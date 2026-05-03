package main

import (
	"GDLNA/cfg"
	"GDLNA/dlna"
	"GDLNA/dlnadb"
	"GDLNA/dlnalogger"
	"GDLNA/system"
	"fmt"
)

// 加入upnp多播组: 239.255.255.250:1900
func main() {
	system.ConfirmOS()
	dlnalogger.LogSwitch = false
	dlnalogger.Info(fmt.Sprintf("GDLNA正在启动..."))

	err := cfg.LoadConfig()
	if err != nil {
		dlnalogger.Error(fmt.Sprintf("启动时错误: %s", err.Error()))
		return
	}

	// 显示DLNA设备名称
	deviceName := dlna.GetDeviceName()
	dlnalogger.Info(fmt.Sprintf("DLNA设备名称: %s", deviceName))

	system.GetRootPath()

	// 检查并创建所需目录
	if err := system.EnsureRequiredDirs(); err != nil {
		dlnalogger.Error(fmt.Sprintf("检查目录时出错: %s", err.Error()))
		return
	}

	dlnadb.LoadMainDB()

	// 先初始化UDP发送连接，再启动各协程
	if err := dlna.InitUDPSendConn(); err != nil {
		dlnalogger.Error(fmt.Sprintf("初始化UDP发送连接失败: %s", err.Error()))
		return
	}

	go dlna.WebHandleList()
	go dlna.StartServer()
	go dlna.StartBroadcastListener() // 启动广播监听
	go dlna.StartTCPServer()         // 启动TCP服务器

	// 初始化UDP 21430端口连接，然后启动监听
	if err := dlna.InitUDP21430Conn(); err != nil {
		dlnalogger.Error(fmt.Sprintf("初始化UDP 21430连接失败: %s", err.Error()))
	} else {
		go dlna.StartUDP21430Listener() // 启动UDP 21430监听（接收订阅者消息）
	}

	// 启动SSDP NOTIFY主动通知协程
	dlna.StartNotifyRoutine()
	dlnalogger.Info(fmt.Sprintf("SSDP NOTIFY主动通知已启动"))

	select {}
}
