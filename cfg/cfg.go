package cfg

import (
	"GDLNA/dlnalogger"
	"crypto/md5"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"gopkg.in/ini.v1"
)

// GetLocalIP 获取本机实际用于网络通信的 IP 地址
func GetLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		dlnalogger.Warning("获取本地 IP 失败，使用 127.0.0.1: " + err.Error())
		return "127.0.0.1"
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

var MainAddress string
var DeviceUUID string
var HTTPPort string
var DeviceName string // DLNA设备名称，可在Config.ini中配置

// GenerateUUID 基于MAC地址+HTTP端口生成唯一的UUID (版本3风格 - 确定性)
// 结合MAC地址和HTTP端口，确保同一机器不同实例也能区分
func GenerateUUID(httpPort string) string {
	// 尝试获取MAC地址
	interfaces, err := net.Interfaces()
	if err == nil {
		for _, i := range interfaces {
			// 找到第一个非回环、已启用的网络接口，且有MAC地址
			if i.Flags&net.FlagUp != 0 && i.Flags&net.FlagLoopback == 0 && len(i.HardwareAddr) >= 6 {
				// 使用MAC地址+HTTP端口的MD5哈希生成确定性UUID
				// 这样即使同一机器运行多个实例（不同端口），UUID也不同
				data := i.HardwareAddr.String() + httpPort
				h := md5.Sum([]byte(data))
				// 设置为UUID v3 (MD5哈希) 格式
				h[6] = (h[6] & 0x0F) | 0x30 // version 3
				h[8] = (h[8] & 0x3F) | 0x80 // variant 10
				uuid := fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
					h[0], h[1], h[2], h[3],
					h[4], h[5],
					h[6], h[7],
					h[8], h[9],
					h[10], h[11], h[12], h[13], h[14], h[15])
				return strings.ToLower(uuid)
			}
		}
	}

	// 如果无法获取MAC地址，回退到基于主机名+端口的UUID
	dlnalogger.Warning("无法获取MAC地址，使用主机名生成UUID")
	hostname, _ := os.Hostname()
	data := hostname + httpPort
	h := md5.Sum([]byte(data))
	h[6] = (h[6] & 0x0F) | 0x30 // version 3
	h[8] = (h[8] & 0x3F) | 0x80 // variant 10
	uuid := fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		h[0], h[1], h[2], h[3],
		h[4], h[5],
		h[6], h[7],
		h[8], h[9],
		h[10], h[11], h[12], h[13], h[14], h[15])
	return strings.ToLower(uuid)
}

func LoadConfig() error {
	// 检查配置文件是否存在，不存在则创建默认配置
	if _, err := os.Stat("Config.ini"); os.IsNotExist(err) {
		dlnalogger.Info("配置文件 Config.ini 不存在，正在生成默认配置...")
		defaultConfig := []byte("[server]\nADDRESS     = \nHTTP_PORT   = 8181\nDEVICE_NAME = \n")
		if err := os.WriteFile("Config.ini", defaultConfig, 0644); err != nil {
			return errors.New("无法创建默认配置文件: " + err.Error())
		}
		dlnalogger.Info("已生成默认配置文件 Config.ini")
	}

	Cfg, err := ini.Load("Config.ini")
	if err != nil {
		return errors.New("读取配置文件错误: " + err.Error())
	}
	cfgServer, err := Cfg.GetSection("server")
	if err != nil {
		dlnalogger.Info(fmt.Sprintf("读取配置文件错误,无法找到‘server’节点: %s", err.Error()))
		os.Exit(1001)
	}

	MainAddress = cfgServer.Key("ADDRESS").MustString("")
	HTTPPort = cfgServer.Key("HTTP_PORT").MustString("8181")
	DeviceName = cfgServer.Key("DEVICE_NAME").MustString("")

	// 如果 ADDRESS 为空，自动获取本机 IP
	if MainAddress == "" {
		MainAddress = GetLocalIP()
		dlnalogger.Info("ADDRESS 未配置，自动获取本机 IP: " + MainAddress)
	}

	// 始终基于MAC地址+HTTP端口生成唯一UUID
	// 这样即使复制整个程序文件夹，不同机器/不同端口的UUID也会不同
	DeviceUUID = GenerateUUID(HTTPPort)
	// 基于MAC+端口生成UUID
	dlnalogger.Info(fmt.Sprintf("生成UUID: %s", DeviceUUID))

	// 确保UUID格式正确 (去掉可能已存在的 "uuid:" 前缀，统一处理)
	DeviceUUID = strings.TrimPrefix(DeviceUUID, "uuid:")
	DeviceUUID = "uuid:" + DeviceUUID

	dlnalogger.Info(fmt.Sprintf("启动: %s, UUID: %s, HTTP端口: %s", MainAddress, DeviceUUID, HTTPPort))
	return nil
}
