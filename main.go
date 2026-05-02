package main

import (
	"GDLNA/cfg"
	"GDLNA/dlna"
	"GDLNA/dlnadb"
	"GDLNA/dnslogger"
	"GDLNA/system"
	"fmt"
)

// 加入upnp多播组: 239.255.255.250:1900
func main() {
	system.ConfirmOS()
	dnslogger.LogSwitch = false
	dnslogger.Info(fmt.Sprintf("GDLNA正在启动..."))

	err := cfg.LoadConfig()
	if err != nil {
		dnslogger.Error(fmt.Sprintf("启动时错误: %s", err.Error()))
		return
	}

	// 显示DLNA设备名称
	deviceName := dlna.GetDeviceName()
	dnslogger.Info(fmt.Sprintf("DLNA设备名称: %s", deviceName))

	system.GetRootPath()
	dlnadb.LoadMainDB()

	// 先初始化UDP发送连接，再启动各协程
	if err := dlna.InitUDPSendConn(); err != nil {
		dnslogger.Error(fmt.Sprintf("初始化UDP发送连接失败: %s", err.Error()))
		return
	}

	go dlna.WebHandleList()
	go dlna.StartServer()

	// 启动SSDP NOTIFY主动通知协程
	dlna.StartNotifyRoutine()
	dnslogger.Info(fmt.Sprintf("SSDP NOTIFY主动通知已启动"))

	select {}
}
