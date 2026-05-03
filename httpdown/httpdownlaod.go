package httpdown

import (
	"GDLNA/dlnalogger"
	"fmt"
	"io"
	"net/http"
	"os"
)

var AllMedia []MediaFile

type MediaFile struct {
	IsDownloading bool
	URL           string
	Filename      string
}

func removeStringByValue(slice []MediaFile, target string) []MediaFile {
	result := slice[:0] // Reuse the original slice's memory.
	for _, v := range slice {
		if v.Filename != target { // Keep elements that don't match.
			result = append(result, v)
		}
	}
	return result
}

func DownloadFile(filename string, url string) error {

	// 检查文件是否存在
	if _, err := os.Stat(filename); !os.IsNotExist(err) {
		return fmt.Errorf("文件已存在: %s", filename)
	}

	removeStringByValue(AllMedia, filename) // 删除 已删除本地实体文件但变量列表中还存在的情况

	go func() {
		err := downFile(filename, url)
		if err != nil {
			return
		}
	}()

	return nil
}

func downFile(filename string, url string) error {

	dlnalogger.Info(fmt.Sprintf("开始下载: %s %s", url, filename))
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("HTTP请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("服务器返回错误状态码: %d", resp.StatusCode)
	}

	outFile, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("无法创建文件: %v", err)
	}
	defer outFile.Close()

	var tmpDown MediaFile
	tmpDown.IsDownloading = true
	tmpDown.URL = url
	tmpDown.Filename = filename
	AllMedia = append(AllMedia, tmpDown)

	if _, err = io.Copy(outFile, resp.Body); err != nil {
		return fmt.Errorf("写入文件失败: %v", err)
	}

	for i := range AllMedia { // ✅推荐写法
		if AllMedia[i].Filename == filename {
			AllMedia[i].IsDownloading = false //强制修改原数据
			break                             //找到后提前退出循环
		}
	}
	return nil
}
