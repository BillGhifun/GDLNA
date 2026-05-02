package dnslogger

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

var LogPath string     // 日志路径
var LogFileName string // 日志文件名
var LogSwitch bool     // 是否启用日志
var LogPathCharacter string
var LogWriteLevel int // 日志记录级别

var LogContent string // 完整的日志数据

func SetLogPath(folder string, fileName string) { // 获得程序所在的目录
	confirmOS()

	var tmpPath string
	var err error
	if os.Getenv("DEBUG") == "" { // 非调试环境
		tmpPath, err = os.Executable()
		if err != nil {
			// 获得程序所在目录错误
			fmt.Println("无法确认程序所在路径:", err)
			//os.Exit(1001)
			return
		}
		LogFileName = fileName
		LogPath = filepath.Dir(tmpPath) + LogPathCharacter + folder + LogPathCharacter //+ fileName
	} else {
		tmpPath, err = os.Getwd()
		if err != nil {
			// 获得程序所在目录错误
			fmt.Println("无法确认程序所在路径:", err)
			//os.Exit(1001)
			return
		}
		LogFileName = fileName
		LogPath = tmpPath + LogPathCharacter + folder + LogPathCharacter //+ fileName
	}
}

func confirmOS() { // 判断操作系统
	sysType := runtime.GOOS
	if sysType == "linux" { // LINUX系统
		LogPathCharacter = "/"
	}
	if sysType == "windows" { // windows系统
		LogPathCharacter = "\\"
	}
}

func Clear() { // 清空日志
	LogContent = ""
}

func createLogFile(logLevel int, logStr string) bool {
	// 2024-10-14
	if len(LogContent) > 131072 {
		LogContent = "" // 大于128K时则清空日志
	}
	if LogContent == "" {
		LogContent = logStr
	} else {
		LogContent = LogContent + "\r\n" + logStr
	}
	fmt.Println(logStr)
	if LogSwitch == true { // 生成日志文件开关
		if logLevel >= LogWriteLevel { // 当日志级别大于等于设定的值才会进行日志文件写入
			if checkFile() == true {
				// 追加数据
				traceLog(logStr)
				return true
			} else {
				// Creating an empty file
				// Using Create() function
				e := createFile(LogPath)
				if e != nil {
					//log.Fatal(e)
					// 日志文件建立时发生错误
					return false
				}
				//log.Println(logFile)
				logFile, err := os.Create(LogPath + LogFileName)
				if err != nil {
					return false
				}
				logFile.Close()
				// 日志文件建立完毕
				return true
			}
		}
		return false
	}
	return false
}

// 调用os.MkdirAll递归创建文件夹
func createFile(filePath string) error {
	if !isExist(filePath) {
		err := os.MkdirAll(filePath, os.ModePerm)
		return err
	}
	return nil
}

// 判断所给路径文件/文件夹是否存在(返回true是存在)
func isExist(path string) bool {
	_, err := os.Stat(path) //os.Stat获取文件信息
	if err != nil {
		if os.IsExist(err) {
			return true
		}
		return false
	}
	return true
}

// 打印内容到文件中
func traceLog(strContent string) {
	fd, _ := os.OpenFile(LogPath+LogFileName, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	buf := []byte(strContent)
	buf = append(buf, 13, 10)
	_, err := fd.Write(buf)
	if err != nil {
		return
	}
	fd.Close()
}

func checkFile() bool { // 检查日志文件是否存在和[写入权限]
	logFileExists, err := pathExists(LogPath + LogFileName)
	if err != nil {
		return false
	}
	return logFileExists
}

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func getTime() (timeNow string) {
	//now := time.Now().UTC()
	//return now.Format("2006-01-02 15:04:05")
	var cstZone = time.FixedZone("GMT", 8*3600) // 东八
	return time.Now().In(cstZone).Format("2006-01-02 15:04:05")
}

func Debug(msg string, v ...interface{}) string {
	createLogFile(0, getTime()+" [Debug] "+msg)
	return msg
}

// Info 输出 Info 级别的日志信息
func Info(msg string, v ...interface{}) string {
	createLogFile(1, getTime()+" [Info] "+msg)
	return msg
}

// Warning 输出 Warning 级别的日志信息
func Warning(msg string, v ...interface{}) string {
	createLogFile(2, getTime()+" [Warning] "+msg)
	return msg
}

// Error 输出 Error 级别的日志信息
func Error(msg string, v ...interface{}) string {
	createLogFile(3, getTime()+" [Error] "+msg)
	return msg
}
