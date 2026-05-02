package cfg

import (
	"GDLNA/dnslogger"
	"crypto/md5"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/go-ini/ini"
)

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
	dnslogger.Warning("无法获取MAC地址，使用主机名生成UUID")
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
	Cfg, err := ini.Load("Config.ini")
	if err != nil {
		return errors.New("读取配置文件错误: " + err.Error())
	}
	cfgServer, err := Cfg.GetSection("server")
	if err != nil {
		dnslogger.Info(fmt.Sprintf("读取配置文件错误,无法找到‘server’节点: %s", err.Error()))
		os.Exit(1001)
	}

	MainAddress = cfgServer.Key("ADDRESS").MustString("192.168.1.1")
	HTTPPort = cfgServer.Key("HTTP_PORT").MustString("8181")
	DeviceName = cfgServer.Key("DEVICE_NAME").MustString("")

	// 始终基于MAC地址+HTTP端口生成唯一UUID
	// 这样即使复制整个程序文件夹，不同机器/不同端口的UUID也会不同
	DeviceUUID = GenerateUUID(HTTPPort)
	dnslogger.Info(fmt.Sprintf("基于MAC+端口生成UUID: %s", DeviceUUID))

	// 确保UUID格式正确 (去掉可能已存在的 "uuid:" 前缀，统一处理)
	DeviceUUID = strings.TrimPrefix(DeviceUUID, "uuid:")
	DeviceUUID = "uuid:" + DeviceUUID

	dnslogger.Info(fmt.Sprintf("启动: %s, UUID: %s, HTTP端口: %s", MainAddress, DeviceUUID, HTTPPort))
	return nil
}
