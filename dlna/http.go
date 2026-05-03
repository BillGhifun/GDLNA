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
	"path/filepath"
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
	moviesFolder := filepath.Join(system.MainRootPath, "movies")
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
	moviesFolder := filepath.Join(system.MainRootPath, "movies", tmpName)
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

	tmpMoviePath := filepath.Join(system.MainRootPath, "movies")

	err = httpdown.DownloadFile(filepath.Join(tmpMoviePath, tmpMedia.FileName), tmpMedia.Link)
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

// handleSetAVTransportURI 处理 SetAVTransportURI 动作（核心：捕获投屏链接）
func handleSetAVTransportURI(c echo.Context, doc *etree.Document) error {
	tmpRoot := doc.SelectElement("s:Envelope")
	if tmpRoot == nil {
		return soapError(c, 401, "Invalid request")
	}

	tmpElement := tmpRoot.FindElement("./s:Body[0]/u:SetAVTransportURI[0]/CurrentURI")
	if tmpElement == nil {
		return soapError(c, 402, "Missing CurrentURI")
	}

	tmpMetaData := tmpRoot.FindElement("./s:Body[0]/u:SetAVTransportURI[0]/CurrentURIMetaData")
	if tmpMetaData == nil {
		// 有些控制点不发送 MetaData，允许为空
		tmpMetaData = &etree.Element{}
	}

	var getMetaData getdata.MetaData
	getMetaData = getdata.GetMetaData(tmpMetaData.Text())

	tmpLink := strings.TrimSpace(tmpElement.Text())
	tmpTitle := getMetaData.Title

	dlnalogger.Info(fmt.Sprintf("收到投屏链接: %s, 标题: %s", tmpLink, tmpTitle))

	for _, tmpMediaInfo := range dlnadb.ListDlna.MediaList {
		if tmpLink == tmpMediaInfo.Link {
			dlnalogger.Info(fmt.Sprintf("投屏链接已存在: %s", tmpLink))
			return soapResponse(c, `<u:SetAVTransportURIResponse xmlns:u="urn:schemas-upnp-org:service:AVTransport:1"></u:SetAVTransportURIResponse>`)
		}
	}

	var tmpMainMedia dlnadb.MediaInfo
	tmpMainMedia.Link = tmpLink
	tmpMainMedia.Title = tmpTitle
	tmpMainMedia.RuneTitle = getMetaData.RuneTitle
	tmpMainMedia.Time = time.Now().Format("2006-01-02 15:04:05")
	tmpMainMedia.FileName = system.RemoveInvalidChars(tmpTitle, tmpMainMedia.Time)

	dlnadb.InsertDLnaData(tmpMainMedia)
	dlnadb.ListDlna.MediaList = append(dlnadb.ListDlna.MediaList, tmpMainMedia)
	dlnalogger.Info(fmt.Sprintf("投屏链接已保存: %s, 时间: %s", tmpLink, tmpMainMedia.Time))

	return soapResponse(c, `<u:SetAVTransportURIResponse xmlns:u="urn:schemas-upnp-org:service:AVTransport:1"></u:SetAVTransportURIResponse>`)
}

// handleGetTransportInfo 处理 GetTransportInfo 动作
// UPnP AVTransport:1 标准：返回 CurrentTransportState / CurrentTransportStatus / CurrentSpeed
func handleGetTransportInfo(c echo.Context, instanceID string) error {
	//dlnalogger.Info(fmt.Sprintf("GetTransportInfo: InstanceID=%s", instanceID))
	body := `<u:GetTransportInfoResponse xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">` +
		`<CurrentTransportState>STOPPED</CurrentTransportState>` +
		`<CurrentTransportStatus>OK</CurrentTransportStatus>` +
		`<CurrentSpeed>1</CurrentSpeed>` +
		`</u:GetTransportInfoResponse>`
	return soapResponse(c, body)
}

// handleGetPositionInfo 处理 GetPositionInfo 动作
func handleGetPositionInfo(c echo.Context, instanceID string) error {
	dlnalogger.Info(fmt.Sprintf("GetPositionInfo: InstanceID=%s", instanceID))
	body := `<u:GetPositionInfoResponse xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">` +
		`<Track>0</Track>` +
		`<TrackDuration>0:00:00</TrackDuration>` +
		`<TrackMetaData></TrackMetaData>` +
		`<TrackURI></TrackURI>` +
		`<RelTime>0:00:00</RelTime>` +
		`<AbsTime>0:00:00</AbsTime>` +
		`<RelCount>2147483647</RelCount>` +
		`<AbsCount>2147483647</AbsCount>` +
		`</u:GetPositionInfoResponse>`
	return soapResponse(c, body)
}

// handleGetMediaInfo 处理 GetMediaInfo 动作
func handleGetMediaInfo(c echo.Context, instanceID string) error {
	dlnalogger.Info(fmt.Sprintf("GetMediaInfo: InstanceID=%s", instanceID))
	body := `<u:GetMediaInfoResponse xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">` +
		`<NrTracks>0</NrTracks>` +
		`<MediaDuration>0:00:00</MediaDuration>` +
		`<CurrentURI></CurrentURI>` +
		`<CurrentURIMetaData></CurrentURIMetaData>` +
		`<NextURI></NextURI>` +
		`<NextURIMetaData></NextURIMetaData>` +
		`<PlayMedium>NONE</PlayMedium>` +
		`<RecordMedium>NOT_IMPLEMENTED</RecordMedium>` +
		`<WriteStatus>NOT_IMPLEMENTED</WriteStatus>` +
		`</u:GetMediaInfoResponse>`
	return soapResponse(c, body)
}

// handleGetDeviceCapabilities 处理 GetDeviceCapabilities 动作
func handleGetDeviceCapabilities(c echo.Context, instanceID string) error {
	dlnalogger.Info(fmt.Sprintf("GetDeviceCapabilities: InstanceID=%s", instanceID))
	body := `<u:GetDeviceCapabilitiesResponse xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">` +
		`<PlayMedia>NETWORK</PlayMedia>` +
		`<RecMedia></RecMedia>` +
		`<RecQualityModes></RecQualityModes>` +
		`</u:GetDeviceCapabilitiesResponse>`
	return soapResponse(c, body)
}

// handleStop 处理 Stop 动作
func handleStop(c echo.Context, instanceID string) error {
	dlnalogger.Info(fmt.Sprintf("Stop: InstanceID=%s", instanceID))
	return soapResponse(c, `<u:StopResponse xmlns:u="urn:schemas-upnp-org:service:AVTransport:1"></u:StopResponse>`)
}

// handlePlay 处理 Play 动作
func handlePlay(c echo.Context, instanceID string) error {
	dlnalogger.Info(fmt.Sprintf("Play: InstanceID=%s", instanceID))
	return soapResponse(c, `<u:PlayResponse xmlns:u="urn:schemas-upnp-org:service:AVTransport:1"></u:PlayResponse>`)
}

// handlePause 处理 Pause 动作
func handlePause(c echo.Context, instanceID string) error {
	dlnalogger.Info(fmt.Sprintf("Pause: InstanceID=%s", instanceID))
	return soapResponse(c, `<u:PauseResponse xmlns:u="urn:schemas-upnp-org:service:AVTransport:1"></u:PauseResponse>`)
}

// handleSeek 处理 Seek 动作
func handleSeek(c echo.Context, instanceID string) error {
	dlnalogger.Info(fmt.Sprintf("Seek: InstanceID=%s", instanceID))
	return soapResponse(c, `<u:SeekResponse xmlns:u="urn:schemas-upnp-org:service:AVTransport:1"></u:SeekResponse>`)
}

// handleGetCurrentTransportActions 处理 GetCurrentTransportActions 动作
func handleGetCurrentTransportActions(c echo.Context, instanceID string) error {
	//dlnalogger.Info(fmt.Sprintf("GetCurrentTransportActions: InstanceID=%s", instanceID))
	body := `<u:GetCurrentTransportActionsResponse xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">` +
		`<Actions>Play,Stop,Pause,Seek</Actions>` +
		`</u:GetCurrentTransportActionsResponse>`
	return soapResponse(c, body)
}

// soapResponse 发送标准 SOAP XML 响应
func soapResponse(c echo.Context, bodyContent string) error {
	soap := `<?xml version="1.0" encoding="utf-8"?>` + "\n" +
		`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">` +
		`<s:Body>` + bodyContent + `</s:Body>` +
		`</s:Envelope>`

	c.Response().Header().Set("Content-Type", `text/xml; charset="utf-8"`)
	c.Response().Header().Set("Ext", "")
	return c.String(200, soap)
}

// soapError 返回 UPnP 标准错误响应
func soapError(c echo.Context, errorCode int, errorDesc string) error {
	body := fmt.Sprintf(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">`+
		`<s:Body><s:Fault>`+
		`<faultcode>s:Client</faultcode>`+
		`<faultstring>UPnPError</faultstring>`+
		`<detail><UPnPError xmlns="urn:schemas-upnp-org:control-1-0">`+
		`<errorCode>%d</errorCode>`+
		`<errorDescription>%s</errorDescription>`+
		`</UPnPError></detail>`+
		`</s:Fault></s:Body></s:Envelope>`, errorCode, errorDesc)

	c.Response().Header().Set("Content-Type", `text/xml; charset="utf-8"`)
	return c.String(500, body)
}

func getRemoteLink(c echo.Context) error {
	r := c.Request()

	// 读取请求体内容
	buf, err := io.ReadAll(r.Body)
	if err != nil {
		dlnalogger.Error(fmt.Sprintf("读取请求体失败: %v", err))
		return soapError(c, 401, "Invalid request")
	}

	// 获取 SOAPACTION 头部，格式如: "urn:schemas-upnp-org:service:AVTransport:1#SetAVTransportURI"
	// 注意：头部值可能被双引号包围，需要去掉
	soapAction := r.Header.Get("SOAPACTION")
	soapAction = strings.Trim(soapAction, "\"")
	//dlnalogger.Info(fmt.Sprintf("收到 SOAP 请求: %s", soapAction))

	// 解析 SOAP 动作名称
	actionName := ""
	if idx := strings.LastIndex(soapAction, "#"); idx != -1 {
		actionName = soapAction[idx+1:]
	}

	// 解析请求体 XML（一次），供各 handler 使用
	doc := etree.NewDocument()
	if len(buf) > 0 {
		reader := bytes.NewReader(buf)
		_, err = doc.ReadFrom(reader)
		if err != nil {
			dlnalogger.Warning(fmt.Sprintf("解析 SOAP XML 体失败: %v, action=%s", err, actionName))
			doc = nil
		}
	}

	// 从 XML 中提取 InstanceID（通常为 0）
	instanceID := "0"
	if doc != nil && doc.Root() != nil {
		if el := doc.Root().FindElement(".//InstanceID"); el != nil {
			instanceID = strings.TrimSpace(el.Text())
		}
	}

	// 根据动作名称分派处理
	switch actionName {
	case "SetAVTransportURI":
		if doc == nil {
			return soapError(c, 402, "Missing request body")
		}
		return handleSetAVTransportURI(c, doc)

	case "GetTransportInfo":
		return handleGetTransportInfo(c, instanceID)

	case "GetPositionInfo":
		return handleGetPositionInfo(c, instanceID)

	case "GetMediaInfo":
		return handleGetMediaInfo(c, instanceID)

	case "GetDeviceCapabilities":
		return handleGetDeviceCapabilities(c, instanceID)

	case "Stop":
		return handleStop(c, instanceID)

	case "Play":
		return handlePlay(c, instanceID)

	case "Pause":
		return handlePause(c, instanceID)

	case "Seek":
		return handleSeek(c, instanceID)

	case "GetCurrentTransportActions":
		return handleGetCurrentTransportActions(c, instanceID)

	default:
		// 未知动作，记录日志并返回通用成功响应，避免控制点放弃连接
		//dlnalogger.Info(fmt.Sprintf("收到未处理的 SOAP 动作: %s，返回通用成功响应", actionName))
		return soapResponse(c, fmt.Sprintf(
			`<u:GenericResponse xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">`+
				`<InstanceID>%s</InstanceID>`+
				`</u:GenericResponse>`, instanceID))
	}
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
