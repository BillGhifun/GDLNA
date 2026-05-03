package system

import (
	"GDLNA/dlnalogger"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var MainRootPath string // 程序所在目录
var OSType int          // windows=0 linux=1

// 错误码
// 8001 初始化设置服务器地址时错误
// 8002 加载指定的配置文件时发生错误

func ConfirmOS() { // 判断操作系统
	sysType := runtime.GOOS
	sysArch := runtime.GOARCH // 架构 Architecture

	if sysType == "linux" { // LINUX系统
		OSType = 1
	}
	if sysType == "windows" { // windows系统
		OSType = 0
	}
	dlnalogger.Info("OS:" + strings.ToUpper(sysType) + "(" + sysArch + ")")
	//dlnalogger.Info("Version:" + MainVersionServer + " " + MainVersionSystem)
}

func GetRootPath() { // 获得程序所在的目录
	var rootPath string
	var err error

	execPath, execErr := os.Executable()
	if execErr != nil {
		fmt.Println("无法确认程序所在路径:", execErr)
		os.Exit(1001)
		return
	}

	// 检查是否在GoLand/IDE临时目录中运行
	// GoLand会将可执行文件输出到临时目录，如 C:\Users\xxx\AppData\Local\JetBrains\GoLand2026.1\tmp\GoLand\
	inGoLandTemp := strings.Contains(execPath, "GoLand") ||
		strings.Contains(execPath, "JetBrains") ||
		strings.Contains(execPath, "Temp") ||
		strings.Contains(execPath, "tmp")

	if inGoLandTemp || os.Getenv("DEBUG") != "" { // 在GoLand中运行或调试环境
		// 使用当前工作目录（通常是项目目录）
		rootPath, err = os.Getwd()
		if err != nil {
			fmt.Println("无法确认程序所在路径:", err)
			os.Exit(1001)
			return
		}
		MainRootPath = rootPath
		dlnalogger.Info("开发环境: 使用工作目录: " + MainRootPath)
	} else { // 非调试环境，使用可执行文件所在目录
		MainRootPath = filepath.Dir(execPath)
		dlnalogger.Info("生产环境: 使用可执行文件目录: " + MainRootPath)
	}
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

// EnsureRequiredDirs 检查并创建程序所需的目录
func EnsureRequiredDirs() error {
	// 需要检查的目录列表
	dirs := []string{"db", "movies"}

	for _, dir := range dirs {
		dirPath := filepath.Join(MainRootPath, dir)
		if _, err := os.Stat(dirPath); os.IsNotExist(err) {
			// 目录不存在，创建它
			err := os.MkdirAll(dirPath, 0755)
			if err != nil {
				dlnalogger.Error(fmt.Sprintf("创建目录失败 %s: %s", dirPath, err.Error()))
				return err
			}
			dlnalogger.Info(fmt.Sprintf("已创建目录: %s", dirPath))
		} else {
			dlnalogger.Info(fmt.Sprintf("目录已存在: %s", dirPath))
		}
	}
	return nil
}
