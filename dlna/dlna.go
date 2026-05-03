package dlna

import (
	"GDLNA/cfg"
	"GDLNA/dlnalogger"
	"bufio"
	"bytes"
	"encoding/xml"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// 日志去重：记录每个(IP, 搜索类型)组合的最后日志时间
var lastLogTime = make(map[string]time.Time)
var logMutex sync.Mutex

// shouldLog 检查是否应该记录日志（相同IP+搜索类型在5秒内只记录一次）
func shouldLog(key string) bool {
	logMutex.Lock()
	defer logMutex.Unlock()

	now := time.Now()
	lastTime, exists := lastLogTime[key]

	if !exists || now.Sub(lastTime) > 2*time.Second {
		lastLogTime[key] = now
		return true
	}
	return false
}

// DeviceDesc 设备描述XML结构
type DeviceDesc struct {
	XMLName     xml.Name    `xml:"root"`
	Xmlns       string      `xml:"xmlns,attr"`
	SpecVersion SpecVersion `xml:"specVersion"`
	Device      Device      `xml:"device"`
}

type SpecVersion struct {
	Major int `xml:"major"`
	Minor int `xml:"minor"`
}

type Device struct {
	DeviceType       string      `xml:"deviceType"`
	FriendlyName     string      `xml:"friendlyName"`
	Manufacturer     string      `xml:"manufacturer"`
	ManufacturerURL  string      `xml:"manufacturerURL"`
	ModelDescription string      `xml:"modelDescription"`
	ModelName        string      `xml:"modelName"`
	ModelNumber      string      `xml:"modelNumber"`
	ModelURL         string      `xml:"modelURL"`
	SerialNumber     string      `xml:"serialNumber"`
	UDN              string      `xml:"UDN"`
	PresentationURL  string      `xml:"presentationURL"`
	ServiceList      ServiceList `xml:"serviceList"`
}

type ServiceList struct {
	Services []Service `xml:"service"`
}

type Service struct {
	ServiceType string `xml:"serviceType"`
	ServiceId   string `xml:"serviceId"`
	ControlURL  string `xml:"controlURL"`
	EventSubURL string `xml:"eventSubURL"`
	SCPDURL     string `xml:"SCPDURL"`
}

// GenerateDeviceDescXML 生成设备描述XML
func GenerateDeviceDescXML(localIP string) (string, error) {
	desc := DeviceDesc{
		Xmlns: "urn:schemas-upnp-org:device-1-0",
		SpecVersion: SpecVersion{
			Major: 1,
			Minor: 0,
		},
		Device: Device{
			DeviceType:       "urn:schemas-upnp-org:device:MediaRenderer:1",
			FriendlyName:     GetDeviceName(),
			Manufacturer:     "Ghifun",
			ManufacturerURL:  "http://127.0.0.1",
			ModelDescription: "Ghifun DLNA Test Server",
			ModelName:        "DLNATestServer",
			ModelNumber:      "0.0.0.1(20211224)",
			ModelURL:         "http://127.0.0.1",
			SerialNumber:     "GDLNA-" + cfg.HTTPPort,
			UDN:              cfg.DeviceUUID,
			PresentationURL:  "http://" + localIP + ":" + cfg.HTTPPort + "/",
			ServiceList: ServiceList{
				Services: []Service{
					{
						ServiceType: "urn:schemas-upnp-org:service:AVTransport:1",
						ServiceId:   "urn:upnp-org:serviceId:AVTransport",
						ControlURL:  "/dlna/AVTransport/action",
						EventSubURL: "/dlna/AVTransport/event",
						SCPDURL:     "/dlna/AVTransport/desc.xml",
					},
					{
						ServiceType: "urn:schemas-upnp-org:service:RenderingControl:1",
						ServiceId:   "urn:upnp-org:serviceId:RenderingControl",
						ControlURL:  "/dlna/RenderingControl/action",
						EventSubURL: "/dlna/RenderingControl/event",
						SCPDURL:     "/dlna/RenderingControl/desc.xml",
					},
					{
						ServiceType: "urn:schemas-upnp-org:service:ConnectionManager:1",
						ServiceId:   "urn:upnp-org:serviceId:ConnectionManager",
						ControlURL:  "/dlna/ConnectionManager/action",
						EventSubURL: "/dlna/ConnectionManager/event",
						SCPDURL:     "/dlna/ConnectionManager/desc.xml",
					},
				},
			},
		},
	}

	output, err := xml.MarshalIndent(desc, "", "    ")
	if err != nil {
		return "", err
	}

	// 添加XML头
	xmlHeader := []byte(`<?xml version="1.0" encoding="utf-8" standalone="yes"?>` + "\n")
	output = append(xmlHeader, output...)

	return string(output), nil
}

// GetDeviceName 获取DLNA设备名称
func GetDeviceName() string {
	// 如果配置文件中设置了设备名称，则使用配置的名称
	if cfg.DeviceName != "" {
		return cfg.DeviceName
	}

	// 如果未配置，则使用默认名称 "GDLNA IP:端口"
	localIP := cfg.GetLocalIP()
	return "GDLNA " + localIP + ":" + cfg.HTTPPort
}

type Info struct {
	ClientAddress string
}

func handler(r *http.Request) {

	//log.Println("-->收到远程数据:", r.RemoteAddr, r.Method)
	//// 打印所有 Header
	//log.Println("---------- Headers ----------")
	//for key, values := range r.Header {
	//	log.Printf("%s: %s", key, strings.Join(values, ", "))
	//}
	//
	//// 打印 Query 参数
	//log.Println("---------- Query Params ----------")
	//for key, values := range r.URL.Query() {
	//	log.Printf("%s: %s", key, strings.Join(values, ", "))
	//}

	// 验证请求方法
	if r.Method != "M-SEARCH" {
		return
	}

	st := r.Header.Get("ST")
	if st == "" {
		return
	}

	userAgent := r.Header.Get("User-Agent")
	//if userAgent == "" {
	//	return
	//}

	// 验证Man头（M-SEARCH请求必须包含）
	man := r.Header.Get("Man")
	if man != "\"ssdp:discover\"" {
		return
	}

	// 记录收到的 M-SEARCH 请求（相同IP+搜索类型在5秒内只记录一次）
	logKey := fmt.Sprintf("%s|%s", r.RemoteAddr, st)
	if shouldLog(logKey) {
		dlnalogger.Info(fmt.Sprintf("收到 M-SEARCH 请求来自：%s, 搜索类型：%s", r.RemoteAddr, st))
	}

	// 获取本地实际 IP 地址用于响应
	localIP := cfg.GetLocalIP()

	// 根据搜索类型确定响应的USN后缀
	var usnSuffix string
	switch st {
	case "ssdp:all":
		usnSuffix = "upnp:rootdevice"
	case "upnp:rootdevice":
		usnSuffix = "upnp:rootdevice"
	case cfg.DeviceUUID:
		usnSuffix = cfg.DeviceUUID
	case "urn:schemas-upnp-org:device:MediaRenderer:1":
		usnSuffix = "urn:schemas-upnp-org:device:MediaRenderer:1"
	case "urn:schemas-upnp-org:service:AVTransport:1":
		usnSuffix = "urn:schemas-upnp-org:service:AVTransport:1"
	case "urn:schemas-upnp-org:service:RenderingControl:1":
		usnSuffix = "urn:schemas-upnp-org:service:RenderingControl:1"
	case "urn:schemas-upnp-org:service:ConnectionManager:1":
		usnSuffix = "urn:schemas-upnp-org:service:ConnectionManager:1"
	default:
		// 未知类型，默认按根设备响应
		usnSuffix = "upnp:rootdevice"
	}

	//"UPnP/1.0 TencentVideoDlna/NewDLNA/1.0 QR/4098\\r\\n"
	var serverType = "Normal"
	if strings.Contains(userAgent, "TencentVideoDlna") {
		serverType = "TencentVideoDlna"
	}

	switch serverType {
	case "Normal":
		serverType = "Linux/5.4 UPnP/1.1 GhifunDLNA/1.0\r\n"
	case "TencentVideoDlna":
		dlnalogger.Info("--> TencentVideoDlna")
		serverType = "Linux/4.4.146 UPnP/1.0 QQLiveTV/1.0\r\n"
	}

	// 使用 strings.Builder 高效构建响应
	var buf strings.Builder
	buf.WriteString("HTTP/1.1 200 OK\r\n")
	buf.WriteString("CACHE-CONTROL: max-age=1800\r\n")
	buf.WriteString(fmt.Sprintf("USN: %s::%s\r\n", cfg.DeviceUUID, usnSuffix))
	buf.WriteString(fmt.Sprintf("LOCATION: http://%s:%s/dlna/desc.xml\r\n", localIP, cfg.HTTPPort))
	//buf.WriteString("SERVER: Linux/5.4 UPnP/1.1 GhifunDLNA/1.0\r\n")
	buf.WriteString("SERVER: " + serverType)
	buf.WriteString("EXT: \r\n")
	//buf.WriteString("DATE: " + time.Now().Format(time.RFC1123) + "\r\n")
	buf.WriteString(fmt.Sprintf("DATE: %s\r\n", time.Now().UTC().Format(time.RFC1123)))
	buf.WriteString(fmt.Sprintf("ST: %s\r\n", st))
	buf.WriteString("BOOTID.UPNP.ORG: 1\r\n")
	buf.WriteString("CONFIGID.UPNP.ORG: 1\r\n")
	buf.WriteString("\r\n")

	err := SendUDP(r.RemoteAddr, buf.String())
	if err != nil {
		dlnalogger.Error(fmt.Sprintf("发送 UDP 数据包失败：%v", err))
	}
}

// 全局UDP socket用于发送响应
var udpSendConn *net.UDPConn

// InitUDPSendConn 初始化UDP发送连接（导出，供main.go调用）
func InitUDPSendConn() error {
	addr := &net.UDPAddr{
		IP:   net.IPv4zero,
		Port: 0,
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}
	udpSendConn = conn
	dlnalogger.Info(fmt.Sprintf("UDP发送连接已初始化"))
	return nil
}

// SendUDP 向指定的客户端地址发送UDP数据包
func SendUDP(clientAddr string, sendData string) error {
	if udpSendConn == nil {
		dlnalogger.Error(fmt.Sprintf("UDP发送连接未初始化"))
		return fmt.Errorf("UDP发送连接未初始化")
	}

	// 解析客户端地址
	addr, err := net.ResolveUDPAddr("udp", clientAddr)
	if err != nil {
		dlnalogger.Error(fmt.Sprintf("解析地址错误: %s", err.Error()))
		return err
	}

	// 使用全局UDP socket发送数据
	_, err = udpSendConn.WriteToUDP([]byte(sendData), addr)
	if err != nil {
		dlnalogger.Error(fmt.Sprintf("发送数据失败: %s", err))
		return err
	}
	return nil
}

func StartServer() {
	dlnalogger.Info(fmt.Sprintf("启动多播侦听协程..."))

	// UDP发送连接已在main.go中初始化
	if udpSendConn == nil {
		dlnalogger.Error(fmt.Sprintf("UDP发送连接未初始化，请先在main.go中调用InitUDPSendConn()"))
		return
	}

	var addr *net.UDPAddr
	var err error

	if addr, err = net.ResolveUDPAddr("udp", "239.255.255.250:1900"); err != nil {
		dlnalogger.Error(fmt.Sprintf("无法解析多播地址: %v", err))
		return
	}

	var conn net.PacketConn
	// net.Interface is nil, call net.joinIPv4Group
	if conn, err = net.ListenMulticastUDP("udp", nil, addr); err != nil {
		dlnalogger.Error(fmt.Sprintf("无法侦听多播地址: %v", err))
		return
	}
	defer conn.Close()

	buf := make([]byte, 2048)
	for {
		n, peerAddr, err := conn.ReadFrom(buf)
		if err != nil {
			dlnalogger.Error(fmt.Sprintf("read-from error: %s", err.Error()))
			return
		}
		reqbytes := buf[:n]
		req, err := http.ReadRequest(bufio.NewReader(bytes.NewBuffer(reqbytes)))
		if err != nil {
			dlnalogger.Error(fmt.Sprintf("Failed to parse request: %s", err.Error()))
			continue
		}
		req.RemoteAddr = peerAddr.String()
		handler(req)
	}
}

// SendNotify 发送SSDP NOTIFY消息（主动通知）
func SendNotify() {
	dlnalogger.Info(fmt.Sprintf("开始发送SSDP NOTIFY消息..."))

	localIP := cfg.GetLocalIP()

	notifyAddr := "239.255.255.250:1900"
	location := fmt.Sprintf("http://%s:%s/dlna/desc.xml", localIP, cfg.HTTPPort)
	baseMsg := "NOTIFY * HTTP/1.1\r\n" +
		"HOST: 239.255.255.250:1900\r\n" +
		"CACHE-CONTROL: max-age=1800\r\n" +
		"LOCATION: " + location + "\r\n" +
		"NTS: ssdp:alive\r\n" +
		"SERVER: Linux/5.4 UPnP/1.1 GhifunDLNA/1.0\r\n" +
		"BOOTID.UPNP.ORG: 1\r\n" +
		"CONFIGID.UPNP.ORG: 1\r\n"

	// 发送 upnp:rootdevice 通知
	//dlnalogger.Info(fmt.Sprintf("发送NOTIFY: upnp:rootdevice"))
	sendOneNotify(notifyAddr, baseMsg, "upnp:rootdevice", cfg.DeviceUUID+"::upnp:rootdevice")

	// 发送 uuid 通知
	//dlnalogger.Info(fmt.Sprintf("发送NOTIFY: uuid"))
	sendOneNotify(notifyAddr, baseMsg, "uuid:"+cfg.DeviceUUID, cfg.DeviceUUID)

	// 发送各服务类型通知
	serviceTypes := []string{
		"urn:schemas-upnp-org:device:MediaRenderer:1",
		"urn:schemas-upnp-org:service:AVTransport:1",
		"urn:schemas-upnp-org:service:RenderingControl:1",
		"urn:schemas-upnp-org:service:ConnectionManager:1",
	}
	for _, nt := range serviceTypes {
		//dlnalogger.Info(fmt.Sprintf("发送NOTIFY: %s", nt))
		sendOneNotify(notifyAddr, baseMsg, nt, cfg.DeviceUUID+"::"+nt)
	}

	dlnalogger.Info(fmt.Sprintf("SSDP NOTIFY消息已全部发送，共%d条", 2+len(serviceTypes)))
}

// sendOneNotify 发送单条NOTIFY消息
func sendOneNotify(targetAddr string, baseMsg string, nt string, usn string) {
	msg := baseMsg + "NT: " + nt + "\r\n" + "USN: " + usn + "\r\n" + "\r\n"
	err := sendUDPNotify(targetAddr, msg)
	if err != nil {
		dlnalogger.Error(fmt.Sprintf("发送NOTIFY消息失败(NT=%s): %v", nt, err))
	}
}

// sendUDPNotify 发送UDP通知消息
func sendUDPNotify(targetAddr string, msg string) error {
	if udpSendConn == nil {
		dlnalogger.Error(fmt.Sprintf("UDP发送连接未初始化"))
		return fmt.Errorf("UDP发送连接未初始化")
	}

	// 解析目标地址
	addr, err := net.ResolveUDPAddr("udp", targetAddr)
	if err != nil {
		return err
	}

	// 使用全局UDP socket发送数据
	_, err = udpSendConn.WriteToUDP([]byte(msg), addr)
	return err
}

// StartNotifyRoutine 启动定期发送NOTIFY消息的协程
func StartNotifyRoutine() {
	go func() {
		// 立即发送一次通知
		SendNotify()

		// 每隔60秒发送一次通知（保持设备在客户端中的活跃状态）
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			SendNotify()
		}
	}()
}
