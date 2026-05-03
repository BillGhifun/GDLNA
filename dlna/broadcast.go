package dlna

import (
	"GDLNA/cfg"
	"GDLNA/dlnadb"
	"GDLNA/dlnalogger"
	"GDLNA/system"
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// SubscriberMessage 订阅者消息
type SubscriberMessage struct {
	Identity     string `json:"identity"`
	Subscribe    bool   `json:"subscribe"`
	PublisherUrl string `json:"publisherUrl"`
	CallbackUrl  string `json:"callbackUrl"`
	Sid          string `json:"sid"`
	NotifyType   string `json:"notifyType"`
	TimeOut      int    `json:"timeOut"`
}

// PublisherMessage 发布者消息
type PublisherMessage struct {
	VdnVersion      string      `json:"vdnVersion"`
	Identity        string      `json:"identity"`
	Subscribe       bool        `json:"subscribe"`
	SubscribeStatus int         `json:"subscribeStatus,omitempty"`
	NotifyType      string      `json:"notifyType"`
	EventType       string      `json:"eventType"`
	Extra           interface{} `json:"extra"`
	EventInfo       EventInfo   `json:"eventInfo"`
}

// ExtraInfo 额外信息
type ExtraInfo struct {
	Channel     string `json:"channel"`
	VersionName string `json:"versionName"`
	IsLogin     bool   `json:"isLogin"`
}

// DiscoverResponse 发现响应 (2字节长度 + JSON)
type DiscoverResponse struct {
	Name string `json:"name"`
	Host string `json:"host"`
	Port int    `json:"port"`
}

// EventInfo 事件信息
type EventInfo struct {
	ErrorCode int    `json:"errorCode"`
	PlayType  int    `json:"playType"`
	Message   string `json:"message"`
	Uuid      string `json:"uuid"`
	Bitrate   string `json:"bitrate"`
}

// udp21430Conn 全局UDP 21430端口连接，用于发送和接收
var udp21430Conn *net.UDPConn

// InitUDP21430Conn 初始化UDP 21430端口连接（复用连接，避免端口占用）
func InitUDP21430Conn() error {
	addr := &net.UDPAddr{
		IP:   net.IPv4zero,
		Port: 21430,
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}
	udp21430Conn = conn
	dlnalogger.Info("UDP 21430 端口连接已初始化")
	return nil
}

// StartBroadcastListener 启动 255.255.255.255 广播消息监听
// 用于接收局域网内广播发现消息（与 DLNA 多播 239.255.255.250 不同）
func StartBroadcastListener() {
	go func() {
		dlnalogger.Info("启动广播监听协程 (255.255.255.255:21416)...")

		// 绑定 0.0.0.0:21416 接收广播消息
		addr := &net.UDPAddr{
			IP:   net.IPv4zero,
			Port: 21416,
		}

		conn, err := net.ListenPacket("udp", addr.String())
		if err != nil {
			dlnalogger.Error("广播监听绑定失败: " + err.Error())
			return
		}
		defer conn.Close()

		buf := make([]byte, 2048)
		for {
			n, peerAddr, err := conn.ReadFrom(buf)
			if err != nil {
				dlnalogger.Warning("广播监听读取错误: " + err.Error())
				continue
			}

			dlnalogger.Info("收到广播消息，来自: " + peerAddr.String() + ", 长度: " + strconv.Itoa(n))

			handleBroadcastMessage(buf[:n], peerAddr, conn)
		}
	}()
}

// extractMessageData 从数据中提取实际消息内容
// 协议格式可能为：2字节长度(大端序) + JSON字符串
// 如果检测到有效长度前缀则跳过前2字节，否则原样返回
func extractMessageData(data []byte) []byte {
	if len(data) < 2 {
		return data
	}
	// 解析前2字节作为长度前缀（大端序）
	expectedLen := int(data[0])<<8 | int(data[1])
	actualLen := len(data) - 2

	// 长度合理（0 < 长度 <= 实际数据长度），则跳过前2字节
	if expectedLen > 0 && expectedLen <= actualLen {
		return data[2 : 2+expectedLen]
	}
	return data
}

// handleBroadcastMessage 处理收到的广播消息
func handleBroadcastMessage(data []byte, peerAddr net.Addr, conn net.PacketConn) {
	// 先提取实际消息内容（跳过可能的2字节长度前缀）
	msgData := extractMessageData(data)
	msg := string(msgData)

	// 检查是否是投屏发现消息
	if strings.Contains(msg, "tv.newtv.screening.udp.discover") {
		dlnalogger.Info("收到投屏发现消息，准备响应...")
		sendScreeningResponse(peerAddr, conn)
		return
	}

	// 检查是否是订阅者消息
	if strings.Contains(msg, `"identity":"subscriber"`) || strings.Contains(msg, `"identity": "subscriber"`) {
		dlnalogger.Info("收到订阅者消息，准备响应...")
		handleSubscriberMessage(msgData, peerAddr, conn)
		return
	}
}

// sendScreeningResponse 发送投屏协议响应
// 协议格式: 2字节长度(大端) + JSON字符串
func sendScreeningResponse(peerAddr net.Addr, conn net.PacketConn) {
	// 构造响应JSON
	deviceName := cfg.DeviceName
	if deviceName == "" {
		deviceName = "GDLNA"
	}

	// 构造JSON: {"name":"设备名","host":"IP","port":端口}
	//jsonStr := fmt.Sprintf(`{"name":"%s","host":"%s","port":%s}`,
	//	deviceName, cfg.MainAddress, cfg.HTTPPort)

	jsonStr := fmt.Sprintf(`{"name":"%s","host":"%s","port":%d}`,
		deviceName, cfg.MainAddress, 16019)

	dlnalogger.Info("构造投屏响应JSON: " + jsonStr)

	jsonBytes := []byte(jsonStr)
	length := len(jsonBytes)

	// 构造响应: 2字节长度(big-endian) + JSON字符串
	response := make([]byte, 2+length)
	response[0] = byte(length >> 8)   // 长度高字节
	response[1] = byte(length & 0xFF) // 长度低字节
	copy(response[2:], jsonBytes)     // JSON字符串

	dlnalogger.Info(fmt.Sprintf("发送投屏响应 (hex): %x", response))

	// 发送响应
	peerUDPAddr, ok := peerAddr.(*net.UDPAddr)
	if !ok {
		dlnalogger.Error("无法解析对端地址")
		return
	}

	_, err := conn.WriteTo(response, peerUDPAddr)
	if err != nil {
		dlnalogger.Error("发送投屏响应失败: " + err.Error())
		return
	}

	dlnalogger.Info("投屏响应已发送")

	// 立即从本地 UDP 21430 端口向源 IP 的 UDP 21420 端口发送发布者消息
	sendPublisherMessageFromPort21430(peerUDPAddr)
}

// handleSubscriberMessage 处理订阅者消息
// 传入的data应是已提取的实际JSON数据（无长度前缀）
func handleSubscriberMessage(data []byte, peerAddr net.Addr, conn net.PacketConn) {
	//// 去除可能的空字节
	//data = bytes.TrimRight(data, "\x00")
	//data = bytes.TrimSpace(data)

	var req SubscriberMessage
	err := json.Unmarshal(data, &req)
	if err != nil {
		dlnalogger.Error("解析订阅者消息失败: " + err.Error())
		dlnalogger.Error(fmt.Sprintf("失败的数据 (hex): %x", data))
		dlnalogger.Error(fmt.Sprintf("失败的数据 (string): %s", string(data)))
		return
	}

	dlnalogger.Info(fmt.Sprintf("订阅者消息: Identity=%s, Subscribe=%v, NotifyType=%s, PublisherUrl=%s, CallbackUrl=%s",
		req.Identity, req.Subscribe, req.NotifyType, req.PublisherUrl, req.CallbackUrl))

	// 根据subscribe字段决定响应类型
	if req.Subscribe {
		// 如果subscribe为true，发送订阅确认响应
		dlnalogger.Info("收到订阅请求，立即发送订阅确认响应")

		// 从callbackUrl解析目标地址
		targetAddr, err := net.ResolveUDPAddr("udp", req.CallbackUrl)
		if err != nil {
			dlnalogger.Error("解析CallbackUrl失败: " + err.Error())
			targetAddr = &net.UDPAddr{
				IP:   peerAddr.(*net.UDPAddr).IP,
				Port: 21420,
			}
		}

		// 立即发送订阅响应（按照抓包顺序）
		sendSubscribeResponseFrom21430(targetAddr)

		// 注意：根据抓包数据，订阅响应发送后，手机会建立TCP连接
		// TCP连接建立过程在StartTCPServer中处理
	} else {
		// 如果subscribe为false，发送普通发布者消息
		//dlnalogger.Info("收到取消订阅请求，发送取消确认响应")
		sendPublisherMessageFromPort21430(peerAddr.(*net.UDPAddr))
	}
}

// sendSubscribeResponseFrom21430 从本地21430端口发送订阅确认响应
// 严格按照抓包数据格式：2字节长度(大端序) + JSON字符串
func sendSubscribeResponseFrom21430(targetAddr *net.UDPAddr) {
	if udp21430Conn == nil {
		dlnalogger.Error("UDP 21430 连接未初始化")
		return
	}

	// 构造订阅确认响应
	resp := PublisherMessage{
		VdnVersion:      "V2",
		Identity:        "publisher",
		Subscribe:       false,
		SubscribeStatus: 200,
		NotifyType:      "tvEvent",
		EventType:       "subscribe",
		Extra:           make(map[string]interface{}), // 空对象 {}
		EventInfo: EventInfo{
			ErrorCode: 0,
			PlayType:  0,
			Message:   "no message",
			Uuid:      "",
			Bitrate:   "",
		},
	}

	// 序列化为JSON
	jsonBytes, err := json.Marshal(resp)
	if err != nil {
		dlnalogger.Error("序列化订阅确认响应失败: " + err.Error())
		return
	}

	dlnalogger.Info("准备发送订阅确认响应: " + string(jsonBytes))

	// 构造响应: 2字节长度(big-endian) + JSON字符串
	length := len(jsonBytes)
	response := make([]byte, 2+length)
	response[0] = byte(length >> 8)   // 长度高字节
	response[1] = byte(length & 0xFF) // 长度低字节
	copy(response[2:], jsonBytes)     // JSON字符串

	dlnalogger.Info(fmt.Sprintf("发送订阅确认响应 (hex): %x", response))

	// 使用全局连接发送响应
	_, err = udp21430Conn.WriteToUDP(response, targetAddr)
	if err != nil {
		dlnalogger.Error("发送订阅确认响应失败: " + err.Error())
		return
	}

	dlnalogger.Info("订阅确认响应已发送到 " + targetAddr.String())
}

// sendPublisherMessageFromPort21430 从本地 UDP 21430 端口向源 IP 的 UDP 21420 端口发送发布者消息
// 严格按照抓包数据格式：2字节长度(大端序) + JSON字符串
func sendPublisherMessageFromPort21430(peerAddr *net.UDPAddr) {
	if udp21430Conn == nil {
		dlnalogger.Error("UDP 21430 连接未初始化")
		return
	}

	// 构造发布者消息
	resp := PublisherMessage{
		VdnVersion: "V2",
		Identity:   "publisher",
		Subscribe:  false,
		NotifyType: "tvEvent",
		EventType:  "position-0-0",
		Extra:      make(map[string]interface{}), // 空对象 {}
		EventInfo: EventInfo{
			ErrorCode: 0,
			PlayType:  0,
			Message:   "no message",
			Uuid:      "",
			Bitrate:   "",
		},
	}

	// 序列化为JSON
	jsonBytes, err := json.Marshal(resp)
	if err != nil {
		dlnalogger.Error("序列化发布者消息失败: " + err.Error())
		return
	}

	dlnalogger.Info("准备从本地 21430 端口发送: " + string(jsonBytes))

	// 构造响应: 2字节长度(big-endian) + JSON字符串
	length := len(jsonBytes)
	response := make([]byte, 2+length)
	response[0] = byte(length >> 8)   // 长度高字节
	response[1] = byte(length & 0xFF) // 长度低字节
	copy(response[2:], jsonBytes)     // JSON字符串

	dlnalogger.Info(fmt.Sprintf("发送发布者消息 (hex): %x", response))

	// 构造对端地址 (源IP的21420端口)
	targetAddr := &net.UDPAddr{
		IP:   peerAddr.IP,
		Port: 21420,
	}

	_, err = udp21430Conn.WriteToUDP(response, targetAddr)
	if err != nil {
		dlnalogger.Error(fmt.Sprintf("从 21430 端口发送数据失败: %v", err))
		return
	}

	dlnalogger.Info(fmt.Sprintf("从本地 21430 端口向 %s 完成数据发送", targetAddr.String()))
}

// StartUDP21430Listener 启动 UDP 21430 端口监听，用于接收订阅者消息
func StartUDP21430Listener() {
	go func() {
		if udp21430Conn == nil {
			dlnalogger.Error("UDP 21430 连接未初始化，请先调用 InitUDP21430Conn()")
			return
		}

		dlnalogger.Info("启动 UDP 21430 端口监听...")

		buf := make([]byte, 2048)
		for {
			n, peerAddr, err := udp21430Conn.ReadFromUDP(buf)
			if err != nil {
				dlnalogger.Warning("UDP 21430 读取错误: " + err.Error())
				continue
			}

			dlnalogger.Info(fmt.Sprintf("收到 UDP 21430 消息，来自: %s, 长度: %d", peerAddr.String(), n))

			// 先提取实际消息内容（跳过可能的2字节长度前缀）
			msgData := extractMessageData(buf[:n])

			// 处理订阅者消息
			handleSubscriberMessage(msgData, peerAddr, udp21430Conn)
		}
	}()
}

// StartTCPServer 启动TCP服务器，监听来自源的POST请求
func StartTCPServer() {
	go func() {
		// 监听16019端口（根据Host头信息）
		addr := ":16019"
		dlnalogger.Info(fmt.Sprintf("启动TCP服务器，监听端口: %s", addr))

		listener, err := net.Listen("tcp", addr)
		if err != nil {
			dlnalogger.Error(fmt.Sprintf("启动TCP服务器失败: %v", err))
			return
		}
		defer listener.Close()

		for {
			conn, err := listener.Accept()
			if err != nil {
				dlnalogger.Error(fmt.Sprintf("接受TCP连接失败: %v", err))
				continue
			}

			go handleTCPConnection(conn)
		}
	}()
}

// handleTCPConnection 处理TCP连接（持续接收消息）
func handleTCPConnection(conn net.Conn) {
	defer conn.Close()

	peerAddr := conn.RemoteAddr().String()
	dlnalogger.Info(fmt.Sprintf("收到TCP连接，来自: %s", peerAddr))

	// 使用 bufio.Reader 来正确读取 HTTP 请求
	reader := bufio.NewReader(conn)

	// 持续读取和处理请求（支持Keep-Alive）
	for {
		// 设置读取超时
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		// 使用 http.ReadRequest 正确解析 HTTP 请求
		request, err := http.ReadRequest(reader)
		if err != nil {
			if err == io.EOF {
				dlnalogger.Info(fmt.Sprintf("TCP连接正常关闭: %s", peerAddr))
			} else {
				dlnalogger.Error(fmt.Sprintf("读取HTTP请求失败: %v", err))
			}
			return
		}

		dlnalogger.Info(fmt.Sprintf("请求行: %s %s %s", request.Method, request.URL.Path, request.Proto))

		// 解析请求头
		dlnalogger.Info("请求头信息:")
		for key, values := range request.Header {
			dlnalogger.Info(fmt.Sprintf("  %s: %s", key, strings.Join(values, ", ")))
		}

		// 检查是否是POST /postCommand请求
		if request.Method == "POST" && strings.Contains(request.URL.Path, "/postCommand") {
			dlnalogger.Info("收到 /postCommand 请求")

			// 正确读取请求体（http.ReadRequest 已经处理了 Content-Length 和 chunked 编码）
			bodyBytes, err := io.ReadAll(request.Body)
			request.Body.Close()
			if err != nil {
				dlnalogger.Error(fmt.Sprintf("读取请求体失败: %v", err))
				return
			}

			dlnalogger.Info(fmt.Sprintf("请求体长度: %d 字节", len(bodyBytes)))
			dlnalogger.Info(fmt.Sprintf("请求体: %s", string(bodyBytes)))

			// 解析JSON数据
			var req PostCommandRequest
			err = json.Unmarshal(bodyBytes, &req)
			if err != nil {
				dlnalogger.Warning(fmt.Sprintf("解析JSON失败: %v", err))
			} else {
				dlnalogger.Info("解析后的请求数据:")
				dlnalogger.Info(fmt.Sprintf("  vdnVersion: %s", req.VdnVersion))
				dlnalogger.Info(fmt.Sprintf("  type: %s", req.Type))
				dlnalogger.Info(fmt.Sprintf("  action: %s", req.Action))
				dlnalogger.Info(fmt.Sprintf("  uuid: %s", req.Uuid))
				dlnalogger.Info(fmt.Sprintf("  startingTime: %d", req.StartingTime))
				dlnalogger.Info(fmt.Sprintf("  bitrate: %s", req.Bitrate))
				if req.Extra != nil {
					dlnalogger.Info(fmt.Sprintf("  extra.userId: %s", req.Extra.UserID))
					dlnalogger.Info(fmt.Sprintf("  extra.hbss: %s", req.Extra.Hbss))
				}

				// 如果 uuid 是一个 URL，则保存到数据库
				if strings.HasPrefix(req.Uuid, "http://") || strings.HasPrefix(req.Uuid, "https://") {
					dlnalogger.Info(fmt.Sprintf("检测到 uuid 为 URL，准备保存到数据库: %s", req.Uuid))

					// 构造媒体信息
					tmpTitle := "YSYY_Video_" + time.Now().Format("20060102_150405")
					var tmpMedia dlnadb.MediaInfo
					tmpMedia.Link = req.Uuid
					tmpMedia.Title = tmpTitle
					tmpMedia.Time = time.Now().Format("2006-01-02 15:04:05")
					tmpMedia.FileName = system.RemoveInvalidChars(tmpTitle, tmpMedia.Time)

					// 原子地检查并添加（内部处理锁和重复检查，防止并发重复添加）
					if dlnadb.AddMediaIfNotExists(tmpMedia) {
						dlnalogger.Info(fmt.Sprintf("已保存 UUID URL 到数据库: %s", req.Uuid))
					} else {
						dlnalogger.Info(fmt.Sprintf("UUID URL 已存在，跳过保存: %s", req.Uuid))
					}
				}

				// 按照抓包顺序：先发送UDP通知，再返回HTTP响应
				peerUDPAddr := &net.UDPAddr{
					IP:   conn.RemoteAddr().(*net.TCPAddr).IP,
					Port: 21420,
				}

				// 根据action类型发送对应的UDP通知
				if req.Action == "tv.newtv.screening.action.video.connect" {
					dlnalogger.Info("按照抓包顺序：先发送UDP connect通知")
					sendConnectMessageFrom21430(peerUDPAddr)
				} else if req.Action == "tv.newtv.screening.action.video.play" {
					dlnalogger.Info("按照抓包顺序：先发送UDP play通知")
					sendPlayMessageFrom21430(peerUDPAddr, req.Uuid, req.Bitrate)
				}

				// 根据不同的action类型进行处理（可选）
				handlePostCommandAction(req, conn)
			}

			// 发送HTTP响应：gzip压缩的"200"
			dlnalogger.Info("发送HTTP响应")
			sendGzipResponse(conn)
		}

		// 检查是否是HTTP/1.0或Connection: close，决定是否继续
		if request.ProtoMajor == 1 && request.ProtoMinor == 0 ||
			strings.ToLower(request.Header.Get("Connection")) == "close" {
			dlnalogger.Info("Connection: close，关闭连接")
			return
		}

		// 继续循环，等待下一个请求（Keep-Alive）
		//dlnalogger.Info("等待下一个请求...")
	}
}

// PostCommandRequest TCP请求中的JSON数据结构
type PostCommandRequest struct {
	VdnVersion     string     `json:"vdnVersion"`
	Type           string     `json:"type"`
	CatgId         string     `json:"catgId"`
	ActionType     string     `json:"actionType"`
	Uuid           string     `json:"uuid"` // 视频URL地址
	SafetyType     int        `json:"safetyType"`
	SourceCms      string     `json:"source_cms"`
	ActionTypeCms  string     `json:"actionType_cms"`
	ContentIdCms   string     `json:"contentId_cms"`
	ContentTypeCms string     `json:"contentType_cms"`
	FocusIdCms     string     `json:"focusId_cms"`
	Action         string     `json:"action"`
	StartingTime   int        `json:"startingTime"`
	Extra          *ExtraData `json:"extra"`
	Bitrate        string     `json:"bitrate"` // 码率
}

// ExtraData 额外数据
type ExtraData struct {
	UserID   string   `json:"userId"`
	Hbss     string   `json:"hbss"`
	Programs []string `json:"programs"`
}

// sendGzipResponse 发送gzip压缩的"200"响应
// 严格按照抓包数据（帧76-78）的格式
func sendGzipResponse(conn net.Conn) {
	// 创建gzip压缩的"200"字符串
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, err := gz.Write([]byte("200"))
	if err != nil {
		dlnalogger.Error(fmt.Sprintf("gzip压缩失败: %v", err))
		return
	}
	err = gz.Close()
	if err != nil {
		dlnalogger.Error(fmt.Sprintf("关闭gzip写入器失败: %v", err))
		return
	}

	gzipData := buf.Bytes()
	//dlnalogger.Info(fmt.Sprintf("gzip压缩后的数据 (hex): %x", gzipData))
	//dlnalogger.Info(fmt.Sprintf("gzip数据长度: %d 字节", len(gzipData)))

	// 构造HTTP响应头（严格按照抓包数据）
	// 注意：抓包数据中"keep-alive"是全小写
	var responseHeaders strings.Builder
	responseHeaders.WriteString("HTTP/1.1 200 OK\r\n")
	responseHeaders.WriteString("Content-Type: text/html\r\n")
	responseHeaders.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().UTC().Format("Sun, 2 Jan 2006 15:04:05 GMT")))
	responseHeaders.WriteString("Access-Control-Allow-Origin: *\r\n")
	responseHeaders.WriteString("Access-Control-Allow-Methods: GET, POST, PUT, DELETE, OPTIONS\r\n")
	responseHeaders.WriteString("Access-Control-Allow-Headers: Content-Type,X-Requested-With,accept,Origin,Access-Control-Request-Method,Access-Control-Request-Headers,token\r\n")
	responseHeaders.WriteString("Connection: keep-alive\r\n")
	responseHeaders.WriteString("Content-Encoding: gzip\r\n")
	responseHeaders.WriteString("Transfer-Encoding: chunked\r\n")
	responseHeaders.WriteString("\r\n")

	// 发送响应头
	_, err = conn.Write([]byte(responseHeaders.String()))
	if err != nil {
		dlnalogger.Error(fmt.Sprintf("发送响应头失败: %v", err))
		return
	}
	//dlnalogger.Info("HTTP响应头已发送")

	// 发送gzip数据（chunked编码）
	// 格式：chunk大小(hex)\r\n + chunk数据\r\n
	// 根据抓包，第一个chunk是gzip头（10字节），第二个chunk是gzip数据（13字节）
	// 但这里我们简化，只发送一个chunk包含完整的gzip数据

	// 第一个chunk: gzip header (10 bytes) - 1f 8b 08 00 00 00 00 00 00 00
	chunk1Size := fmt.Sprintf("%x\r\n", 10)
	_, err = conn.Write([]byte(chunk1Size))
	if err != nil {
		dlnalogger.Error(fmt.Sprintf("发送chunk1大小失败: %v", err))
		return
	}
	_, err = conn.Write(gzipData[:10])
	if err != nil {
		dlnalogger.Error(fmt.Sprintf("发送chunk1数据失败: %v", err))
		return
	}
	_, err = conn.Write([]byte("\r\n"))
	if err != nil {
		dlnalogger.Error(fmt.Sprintf("发送chunk1结束标记失败: %v", err))
		return
	}

	// 第二个chunk: gzip数据剩余部分
	chunk2Size := fmt.Sprintf("%x\r\n", len(gzipData)-10)
	_, err = conn.Write([]byte(chunk2Size))
	if err != nil {
		dlnalogger.Error(fmt.Sprintf("发送chunk2大小失败: %v", err))
		return
	}
	_, err = conn.Write(gzipData[10:])
	if err != nil {
		dlnalogger.Error(fmt.Sprintf("发送chunk2数据失败: %v", err))
		return
	}
	_, err = conn.Write([]byte("\r\n"))
	if err != nil {
		dlnalogger.Error(fmt.Sprintf("发送chunk2结束标记失败: %v", err))
		return
	}

	// 发送结束chunk
	_, err = conn.Write([]byte("0\r\n\r\n"))
	if err != nil {
		dlnalogger.Error(fmt.Sprintf("发送结束chunk失败: %v", err))
		return
	}

	//dlnalogger.Info("gzip压缩的200响应已发送（chunked编码）")
}

// sendDeviceInfo 发送设备信息JSON
func sendDeviceInfo(conn net.Conn) {
	deviceName := cfg.DeviceName
	if deviceName == "" {
		deviceName = "GDLNA"
	}

	// 构造设备信息JSON
	jsonStr := fmt.Sprintf(`{"name":"%s","host":"%s","port":%d}`,
		deviceName, cfg.MainAddress, 16019)

	dlnalogger.Info(fmt.Sprintf("发送设备信息: %s", jsonStr))

	// 直接发送JSON字符串
	_, err := conn.Write([]byte(jsonStr))
	if err != nil {
		dlnalogger.Error(fmt.Sprintf("发送设备信息失败: %v", err))
		return
	}

	dlnalogger.Info("设备信息已发送")
}

// sendConnectMessageFrom21430 从本地21430端口发送连接消息（数据组B）
func sendConnectMessageFrom21430(targetAddr *net.UDPAddr) {
	if udp21430Conn == nil {
		dlnalogger.Error("UDP 21430 连接未初始化")
		return
	}

	// 构造连接消息（eventType为"connect"）
	resp := PublisherMessage{
		VdnVersion: "V2",
		Identity:   "publisher",
		Subscribe:  false,
		NotifyType: "tvEvent",
		EventType:  "connect",
		Extra: ExtraInfo{
			Channel:     "50000323",
			VersionName: "7.2.0",
			IsLogin:     false,
		},
		EventInfo: EventInfo{
			ErrorCode: 0,
			PlayType:  0,
			Message:   "no message",
			Uuid:      "",
			Bitrate:   "",
		},
	}

	sendUDPMessageFrom21430(resp, targetAddr, "连接消息")
}

// handlePostCommandAction 处理不同的postCommand动作
func handlePostCommandAction(req PostCommandRequest, conn net.Conn) {
	peerAddr := conn.RemoteAddr().(*net.TCPAddr)
	targetAddr := &net.UDPAddr{
		IP:   peerAddr.IP,
		Port: 21420,
	}

	dlnalogger.Info(fmt.Sprintf("处理postCommand动作: %s", req.Action))

	switch req.Action {
	case "tv.newtv.screening.action.video.connect":
		// 发送connect消息
		sendConnectMessageFrom21430(targetAddr)

	case "tv.newtv.screening.action.play":
		// 发送play消息
		sendPlayMessageFrom21430(targetAddr, req.Uuid, req.Bitrate)

	case "tv.newtv.screening.action.stop",
		"tv.newtv.screening.action.pause",
		"tv.newtv.screening.action.resume":
		// 可以扩展处理其他动作
		dlnalogger.Info(fmt.Sprintf("收到 %s 动作，暂未实现UDP响应", req.Action))

	default:
		dlnalogger.Warning(fmt.Sprintf("未知的动作类型: %s", req.Action))
	}
}

// sendPlayMessageFrom21430 从本地21430端口发送播放消息
func sendPlayMessageFrom21430(targetAddr *net.UDPAddr, videoUrl string, bitrate string) {
	if udp21430Conn == nil {
		dlnalogger.Error("UDP 21430 连接未初始化")
		return
	}

	// 构造播放消息（eventType为"play"）
	resp := PublisherMessage{
		VdnVersion: "V2",
		Identity:   "publisher",
		Subscribe:  false,
		NotifyType: "tvEvent",
		EventType:  "play",
		Extra: ExtraInfo{
			Channel:     "50000323",
			VersionName: "7.2.0",
			IsLogin:     false,
		},
		EventInfo: EventInfo{
			ErrorCode: 0,
			PlayType:  0,
			Message:   "no message",
			Uuid:      videoUrl,
			Bitrate:   bitrate,
		},
	}

	sendUDPMessageFrom21430(resp, targetAddr, "播放消息")
}

// sendUDPMessageFrom21430 通用函数：从21430端口发送UDP消息
// 严格按照抓包数据格式：2字节长度(大端序) + JSON字符串
func sendUDPMessageFrom21430(resp PublisherMessage, targetAddr *net.UDPAddr, msgType string) {
	if udp21430Conn == nil {
		dlnalogger.Error("UDP 21430 连接未初始化")
		return
	}

	// 序列化为JSON
	jsonBytes, err := json.Marshal(resp)
	if err != nil {
		dlnalogger.Error(fmt.Sprintf("序列化%s失败: %v", msgType, err))
		return
	}

	dlnalogger.Info(fmt.Sprintf("准备发送%s: %s", msgType, string(jsonBytes)))

	// 构造响应: 2字节长度(big-endian) + JSON字符串
	length := len(jsonBytes)
	response := make([]byte, 2+length)
	response[0] = byte(length >> 8)   // 长度高字节
	response[1] = byte(length & 0xFF) // 长度低字节
	copy(response[2:], jsonBytes)     // JSON字符串

	dlnalogger.Info(fmt.Sprintf("发送%s (hex): %x", msgType, response))

	// 使用全局连接发送消息
	_, err = udp21430Conn.WriteToUDP(response, targetAddr)
	if err != nil {
		dlnalogger.Error(fmt.Sprintf("发送%s失败: %v", msgType, err))
		return
	}

	dlnalogger.Info(fmt.Sprintf("%s已发送到 %s", msgType, targetAddr.String()))
}
