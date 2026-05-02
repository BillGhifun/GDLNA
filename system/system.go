package system

import (
	"GDLNA/dnslogger"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var MainRootPath string  // 程序所在目录
var PathCharacter string // 路径符
var OSType int           // windows=0 linux=1

// 错误码
// 8001 初始化设置服务器地址时错误
// 8002 加载指定的配置文件时发生错误

func ConfirmOS() { // 判断操作系统
	sysType := runtime.GOOS
	sysArch := runtime.GOARCH // 架构 Architecture

	if sysType == "linux" { // LINUX系统
		PathCharacter = "/"
		OSType = 1
	}
	if sysType == "windows" { // windows系统
		PathCharacter = "\\"
		OSType = 0
	}
	dnslogger.Info("OS:" + strings.ToUpper(sysType) + "(" + sysArch + ")")
	//dnslogger.Info("Version:" + MainVersionServer + " " + MainVersionSystem)
}

func GetRootPath() { // 获得程序所在的目录
	// 获取当前二进制文件的路径
	var binaryPath string
	var err error
	if os.Getenv("DEBUG") == "" { // 非调试环境
		binaryPath, err = os.Executable()
		MainRootPath = filepath.Dir(binaryPath) + PathCharacter // 获得程序所在目录
	} else {
		binaryPath, err = os.Getwd()
		MainRootPath = binaryPath + PathCharacter // 获得程序所在目录
	}

	//fmt.Println("程序目录: " + MainRootPath)

	if err != nil {
		fmt.Println("无法确认程序所在路径:", err)
		os.Exit(1001)
		return
	}
	// 切片操作去掉文件名部分
	//MainRootPath = filepath.Dir(binaryPath) // 获得程序所在目录
	// 各个改写列表和黑白名单的默认路径  /config/*
}

// RemoveInvalidChars 从文件名中删除不允许的字符
func RemoveInvalidChars(mediaName string, mediaTime string) string {
	if mediaName == "" || mediaName == "投屏视频" {
		mediaName = "投屏视频_" + strings.Replace(strings.Replace(mediaTime, ":", "_", -1), " ", "_", -1) + ".mp4"
		return mediaName
	}
	// 定义不允许的字符
	invalidChars := []string{"<", ">", ":", "\"", "/", "\\", "|", "?", "*"}
	for _, char := range invalidChars {
		mediaName = strings.ReplaceAll(mediaName, char, "")
	}
	mediaName = mediaName + "_" + strings.Replace(strings.Replace(mediaTime, ":", "_", -1), " ", "_", -1) + ".mp4"
	return mediaName
}

func GetRuneName(tmpTitle string) string {
	if strings.TrimSpace(tmpTitle) != "" {
		runes := []rune(tmpTitle)
		if len(runes) > 20 {
			return string(runes[:20]) + "..."
		} else {
			return string(runes) // 如果字符少于20个，保持原样
		}
	} else {
		return "投屏视频"
	}
}
