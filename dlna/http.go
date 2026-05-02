package dlna

import (
	"GDLNA/cfg"
	"GDLNA/dlnadb"
	"GDLNA/dlnalogger"
	"GDLNA/fm"
	"GDLNA/getdata"
	"GDLNA/httpdown"
	"GDLNA/system"
	"GDLNA/websource"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/beevik/etree"
	assetfs "github.com/elazarl/go-bindata-assetfs"
	"github.com/labstack/echo/v4"
)

func StaticAssets(root string) *assetfs.AssetFS {
	return &assetfs.AssetFS{
		Asset:     websource.Asset,
		AssetDir:  websource.AssetDir,
		AssetInfo: websource.AssetInfo,
		Prefix:    root,
	}
}

func LoadWebSource(e *echo.Echo) {
	// 1. 修复 API 路由组（必须赋值给变量并使用该变量来注册子路由）
	dlna := e.Group("/dlna")
	// 以下两个 API 不会与 /dlna/AVTransport/desc.xml 产生冲突，因为 Echo 是精准匹配
	dlna.GET("/desc.xml", handleDeviceDesc)         // 实际访问路径：/dlna/desc.xml
	dlna.POST("/AVTransport/action", getRemoteLink) // 实际访问路径：/dlna/AVTransport/action

	// 2. 静态文件处理器（不需要使用 e.Group("/*")，直接在根实例 e 上操作）
	if _, err := os.Stat("www"); err == nil {
		// 【物理目录模式】
		// e.Static("/", "www") 会自动处理所有上面没注册过的 GET 请求。
		// 当你请求 /dlna/AVTransport/desc.xml 时，它会自动去寻找 www/dlna/AVTransport/desc.xml
		e.Static("/", "www")
	} else {
		// 【静态数据模式】
		assetHandler := http.FileServer(StaticAssets("/www/"))
		// 使用根实例的 /* 捕捉所有未匹配路由，直接交给内嵌文件系统
		// 当请求 /dlna/AVTransport/desc.xml 时，FileServer 会在 StaticAssets 中去寻找该路径
		e.GET("/*", echo.WrapHandler(assetHandler))
	}
}

func WebHandleList() {
	e := echo.New()
	e.HideBanner = true // 隐藏ECHO标题
	e.HidePort = true   // 隐藏ECHO端口显示

	//// 动态路由必须先注册，以确保优先级高于静态路由
	//e.GET("/dlna/desc.xml", handleDeviceDesc)
	//e.POST("/dlna/AVTransport/action", getRemoteLink)

	e.GET("/media_get_list", getMediaList)

	e.GET("/media_del", linkDel)

	e.GET("/media_clear_all", linkClearAll)

	e.GET("/save_media", mediaSave)

	e.GET("/file_get_list", getFileList)

	e.GET("/file_del", fileDel)

	e.Static("/movies", "movies")

	e.GET("/openvideo", openVideo)

	// 静态路由必须后注册，以确保动态路由优先级更高
	// 优先加载本地 www 目录，如果不存在则使用编译的静态资源
	LoadWebSource(e)

	//启动http server, 并监听8080端口，冒号（:）前面为空的意思就是绑定网卡所有Ip地址，本机支持的所有ip地址都可以访问。
	go initGOGWebServerHTTP(e)
}

type FileInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Link string `json:"link"`
	Size string `json:"size"`
	Time string `json:"time"`
}

type Files struct {
	Info []FileInfo `json:"files"`
}

func getFileList(c echo.Context) error {
	moviesFolder := "." + system.PathCharacter + "movies" + system.PathCharacter
	webMoviesFolder := "movies/"
	tmpList, err := fm.GetFileList(moviesFolder)
	if err != nil {
		return err
	}
	var allFile Files
	for _, tmpFileInfo := range tmpList {
		allFile.Info = append(allFile.Info, FileInfo{
			Name: tmpFileInfo.Name(),                                  // 文件名
			Path: webMoviesFolder,                                     // 文件夹
			Link: webMoviesFolder + tmpFileInfo.Name(),                // 链接
			Size: strconv.FormatInt(tmpFileInfo.Size(), 10),           // 文件大小
			Time: tmpFileInfo.ModTime().Format("2006-01-02 15:04:05"), // 创建时间
		})
	}

	tmpMarshal, err := json.Marshal(allFile)
	if err != nil {
		return err
	}

	c.Response().Header().Set("Content-Type", "application/json")

	return c.String(200, string(tmpMarshal))
}

func openVideo(c echo.Context) error {
	c.Response().Header().Set("Referrer-Policy", "no-referrer") // 删除 referer
	tmpHTML := "<!DOCTYPE html>\n<html lang=\"zh\">\n" +
		"<style>\n    html, body {\n        height: 100%; /* 确保html和body的高度为100% */\n        margin: 0;    /* 去掉默认的边距 */\n        padding: 0;   /* 去掉默认的内边距 */\n        overflow: hidden; /* 隐藏滚动条 */\n    }\n\n    #videoPlayer {\n        border: none; /* 去掉视频的边框 */\n    }\n</style>" +
		"<head>\n    <title>视频播放器</title>\n    <script>\n        // 获取URL中的查询参数\n        function getQueryParam(param) {\n            const urlParams = new URLSearchParams(window.location.search);\n            return urlParams.get(param);\n        }\n\n        window.onload = function() {\n            const videoUrl = getQueryParam('videoUrl'); // 获取名为'videoUrl'的参数\n            const title = getQueryParam('title'); // 获取名为'title'的参数\n            if (title) {\n                document.title = title; // 替换页面标题\n            }\n            if (videoUrl) {\n                const videoElement = document.getElementById('videoPlayer');\n                videoElement.src = videoUrl; // 设置视频源\n                videoElement.play(); // 播放视频\n            }\n        };\n    </script>\n</head>\n<body>\n\n<video id=\"videoPlayer\" controls style=\"width: 100%; height: 100%;\"></video>\n\n</body>\n</html>"

	return c.HTML(200, tmpHTML)
}

func linkDel(c echo.Context) error {
	tmpIndex := c.QueryParam("id")
	tmpInt, err := strconv.Atoi(tmpIndex)
	if err != nil {
		return c.String(500, "删除参数错误")
	}

	// 检查索引是否越界
	if tmpInt < 0 || tmpInt >= len(dlnadb.ListDlna.MediaList) {
		return c.String(400, "删除的索引不存在")
	}

	// 删除数据库中的值
	isDelDBOK := dlnadb.DeleteDLnaData(dlnadb.ListDlna.MediaList[tmpInt].Link)
	tmpLinkList := append(dlnadb.ListDlna.MediaList[:tmpInt], dlnadb.ListDlna.MediaList[tmpInt+1:]...)
	dlnadb.ListDlna.MediaList = tmpLinkList
	if isDelDBOK {
		return c.String(200, "OK")
	} else {
		return c.String(200, "从数据库中删除失败")
	}
}

func linkClearAll(c echo.Context) error {
	// 清空所有媒体项
	dlnadb.ListDlna.MediaList = []dlnadb.MediaInfo{}
	// 清空数据库中的所有数据
	isClearDBOK := dlnadb.ClearAllDLnaData()
	if isClearDBOK {
		return c.String(200, "已清空所有媒体")
	} else {
		return c.String(200, "清空数据库失败")
	}
}

func fileDel(c echo.Context) error {
	tmpName := c.QueryParam("name")
	moviesFolder := "." + system.PathCharacter + "movies" + system.PathCharacter + tmpName
	// 使用 os.Stat 检查文件是否存在
	_, err := os.Stat(moviesFolder)
	if err == nil { // 文件存在
		err = os.Remove(moviesFolder)
		if err != nil {
			return c.String(200, "文件删除失败:"+err.Error())
		} else {
			return c.String(200, "文件删除成功")
		}
	} else if os.IsNotExist(err) {
		return c.String(200, "文件不存在")
	} else {
		return c.String(200, fmt.Sprintf("检查文件 %s 时出错: %v\n", moviesFolder, err))
	}
}

func mediaSave(c echo.Context) error {
	tmpIndex := c.QueryParam("id")
	tmpInt, err := strconv.Atoi(tmpIndex)
	if err != nil {
		return c.String(500, "保存参数错误")
	}

	// 检查索引是否越界
	if tmpInt < 0 || tmpInt >= len(dlnadb.ListDlna.MediaList) {
		return c.String(400, "保存的索引不存在")
	}

	var tmpMedia dlnadb.MediaInfo
	tmpMedia = dlnadb.ListDlna.MediaList[tmpInt]

	tmpMoviePath := "." + system.PathCharacter + "movies" + system.PathCharacter

	err = httpdown.DownloadFile(tmpMoviePath+tmpMedia.FileName, tmpMedia.Link)
	if err != nil {
		return c.String(200, "无法建立下载任务:"+err.Error())
	}

	return c.String(200, "已建立下载任务")
}

func getMediaList(c echo.Context) error {
	tmpMarshal, err := json.Marshal(dlnadb.ListDlna)
	if err != nil {
		return err
	}

	c.Response().Header().Set("Content-Type", "application/json")

	return c.String(200, string(tmpMarshal))
}

func getRemoteLink(c echo.Context) error {
	r := c.Request() // c: echo.Context

	// 读取请求体内容
	buf, err := io.ReadAll(r.Body)
	if err != nil {
		return c.String(500, "Error reading body")
	}

	// 重新设置请求体，以便后续读取
	r.Body = io.NopCloser(bytes.NewBuffer(buf))

	// 初始化根节点
	doc := etree.NewDocument()
	_, err = doc.ReadFrom(r.Body)
	if err != nil {
		return c.String(500, "Error")
	}

	tmpRoot := doc.SelectElement("s:Envelope")
	if tmpRoot == nil {
		return c.String(404, "Root Not Found")
	}
	tmpElement := tmpRoot.FindElement("./s:Body[0]/u:SetAVTransportURI[0]/CurrentURI") //.Text()
	if tmpElement == nil {
		return c.String(404, "Element Not Found")
	}

	tmpMetaData := tmpRoot.FindElement("./s:Body[0]/u:SetAVTransportURI[0]/CurrentURIMetaData") //.Text()
	if tmpElement == nil {
		return c.String(404, "Element Not Found")
	}

	var getMetaData getdata.MetaData
	// 解析MetaData
	getMetaData = getdata.GetMetaData(tmpMetaData.Text())

	// 先获取链接和标题
	tmpLink := strings.TrimSpace(tmpElement.Text())
	tmpTitle := getMetaData.Title

	dlnalogger.Info(fmt.Sprintf("收到投屏链接: %s, 标题: %s", tmpLink, tmpTitle))

	for _, tmpMediaInfo := range dlnadb.ListDlna.MediaList {
		if tmpLink == tmpMediaInfo.Link {
			dlnalogger.Info(fmt.Sprintf("投屏链接已存在: %s", tmpLink))
			return c.String(200, "已存在投屏数据")
		}
	}

	var tmpMainMedia dlnadb.MediaInfo
	tmpMainMedia.Link = tmpLink
	tmpMainMedia.Title = tmpTitle
	tmpMainMedia.RuneTitle = getMetaData.RuneTitle
	tmpMainMedia.Time = time.Now().Format("2006-01-02 15:04:05")
	tmpMainMedia.FileName = system.RemoveInvalidChars(tmpTitle, tmpMainMedia.Time)

	dlnadb.InsertDLnaData(tmpMainMedia) // 插入数据库
	dlnadb.ListDlna.MediaList = append(dlnadb.ListDlna.MediaList, tmpMainMedia)
	dlnalogger.Info(fmt.Sprintf("投屏链接已保存: %s, 时间: %s", tmpLink, tmpMainMedia.Time))

	return c.String(200, "OK")
}

func initGOGWebServerHTTP(mainWeb *echo.Echo) { // 初始化WEB控制台服务
	err := mainWeb.Start("0.0.0.0:" + cfg.HTTPPort)
	if err != nil {
		dlnalogger.Error(fmt.Sprintf("启动HTTP服务错误! %s", err.Error()))
		return
	}
}

// handleDeviceDesc 动态生成设备描述XML
func handleDeviceDesc(c echo.Context) error {
	// 获取本地IP地址
	localIP := cfg.GetLocalIP()

	// 生成设备描述XML
	xmlContent, err := GenerateDeviceDescXML(localIP)
	if err != nil {
		dlnalogger.Error(fmt.Sprintf("生成设备描述XML失败: %s", err.Error()))
		return c.String(http.StatusInternalServerError, "Internal Server Error")
	}

	// 设置Content-Type并返回XML
	c.Response().Header().Set("Content-Type", "application/xml; charset=utf-8")
	return c.String(http.StatusOK, xmlContent)
}
