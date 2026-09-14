package proxy

import (
	"bytes"
	"crypto/tls"
	"errors"
	"io"
	"log"
	"time"

	"math"
	"net"
	"sync"

	"BackendTemplate/pkg/proxy/bufferpool"
	"BackendTemplate/pkg/proxy/mux"
	"BackendTemplate/pkg/proxy/statute"
)

var MuSocks5Server sync.Mutex
var Socks5Server = make(map[string]net.Listener)

// CloseSocks5Server 加锁取出并关闭指定监听器，返回是否存在（锁内取出，锁外 Close，避免持锁做网络 IO）
func CloseSocks5Server(addr string) bool {
	MuSocks5Server.Lock()
	ln, exists := Socks5Server[addr]
	if exists {
		delete(Socks5Server, addr)
	}
	MuSocks5Server.Unlock()
	if exists && ln != nil {
		ln.Close()
	}
	return exists
}

var (
	bufferPool = bufferpool.NewPool(math.MaxUint16)
	// 固定的64字节魔法包
	magicPacket = [64]byte{
		0x6a, 0x1d, 0x3e, 0x74, 0x8b, 0x99, 0x5a, 0x7f,
		0xca, 0xef, 0x33, 0x88, 0xac, 0x44, 0x52, 0xbd,
		0x1e, 0x5f, 0x39, 0xd4, 0x6c, 0xb3, 0x72, 0xf8,
		0x21, 0x9d, 0x54, 0x68, 0x91, 0xab, 0x43, 0xee,
		0x4c, 0x7a, 0x90, 0x26, 0xf7, 0x35, 0x9b, 0x5e,
		0xd1, 0x88, 0x4f, 0xbc, 0x2a, 0x67, 0x91, 0xe4,
		0xaf, 0x34, 0xcd, 0x89, 0x60, 0x18, 0xa2, 0xde,
		0x77, 0x93, 0xfb, 0x02, 0x6e, 0x11, 0xc3, 0xf0,
	}
)

func ReverseSocksServer(agentListenAddress, socksListenAddress, psk, certFile, keyFile, username, password string) {
	log.Printf("[SOCKS5-SERVER] 🚀 启动反向SOCKS服务器")
	log.Printf("[SOCKS5-SERVER] 📋 配置信息:")
	log.Printf("[SOCKS5-SERVER]    - Agent监听地址: %s", agentListenAddress)
	log.Printf("[SOCKS5-SERVER]    - SOCKS监听地址: %s", socksListenAddress)
	log.Printf("[SOCKS5-SERVER]    - PSK长度: %d", len(psk))
	log.Printf("[SOCKS5-SERVER]    - 证书文件: %s", certFile)
	log.Printf("[SOCKS5-SERVER]    - 密钥文件: %s", keyFile)
	log.Printf("[SOCKS5-SERVER]    - 用户名: %s", username)
	log.Printf("[SOCKS5-SERVER]    - 密码长度: %d", len(password))

	usingTLS := false
	var cert tls.Certificate
	var err error

	if certFile != "" && keyFile != "" {
		log.Printf("[SOCKS5-SERVER] 🔐 尝试加载TLS证书...")
		cert, err = tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			log.Printf("[SOCKS5-SERVER] ⚠️ 证书加载失败，使用TCP监听: %v", err)
		} else {
			usingTLS = true
			log.Printf("[SOCKS5-SERVER] ✅ TLS证书加载成功")
		}
	} else {
		log.Printf("[SOCKS5-SERVER] 📝 未提供证书文件，使用TCP监听")
	}

	if len(password) == 0 {
		log.Printf("[SOCKS5-SERVER] ⚠️ 警告: 未配置密码，任何人都可以连接到SOCKS5服务器")
	} else {
		log.Printf("[SOCKS5-SERVER] 🔐 已配置用户名密码认证")
	}

	log.Printf("[SOCKS5-SERVER] 🔍 开始监听Agent连接: %s", agentListenAddress)

	var ln net.Listener
	if usingTLS {
		config := &tls.Config{
			PreferServerCipherSuites: true,
			CurvePreferences:         []tls.CurveID{tls.X25519, tls.CurveP256},
			Certificates:             []tls.Certificate{cert},
		}
		ln, err = tls.Listen("tcp", agentListenAddress, config)
		log.Printf("[SOCKS5-SERVER] 🔐 TLS监听器已创建")
	} else {
		ln, err = net.Listen("tcp", agentListenAddress)
		log.Printf("[SOCKS5-SERVER] 🌐 TCP监听器已创建")
	}

	// 注册必须在 err 检查之后（避免失败时把 nil 监听器写入 map 导致后续 Close panic）
	if err != nil {
		log.Printf("[SOCKS5-SERVER] ❌ 监听器创建失败: %v", err)
		return
	}
	MuSocks5Server.Lock()
	Socks5Server[agentListenAddress] = ln
	MuSocks5Server.Unlock()

	log.Printf("[SOCKS5-SERVER] ✅ 监听器创建成功，等待Agent连接...")

	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				log.Printf("[SOCKS5-SERVER] 🔚 监听器已关闭，退出主循环")
				break
			} else {
				log.Printf("[SOCKS5-SERVER] ❌ 接受连接失败: %v", err)
				continue
			}
		}

		log.Printf("[SOCKS5-SERVER] 🔗 Agent已连接: %s", conn.RemoteAddr().String())

		// 验证魔法包
		deadline := time.Now().Add(time.Second * 5)
		conn.SetReadDeadline(deadline)
		buffer := make([]byte, 64)
		log.Printf("[SOCKS5-SERVER] ⏱️ 等待魔法包验证（5秒超时）...")
		// 用 io.ReadFull 读取完整 64 字节，避免 TCP 短读导致验证失败
		if _, err := io.ReadFull(conn, buffer); err != nil {
			log.Printf("[SOCKS5-SERVER] ❌ 读取魔法包失败: %v", err)
			conn.Close()
			continue
		}
		magic := DeriveMagicPacket(psk)
		if bytes.Equal(buffer, magic[:]) {
			log.Printf("[SOCKS5-SERVER] ✅ 魔法包验证成功！")
		} else {
			log.Printf("[SOCKS5-SERVER] ❌ 魔法包验证失败")
			conn.Close()
			continue
		}
		conn.SetReadDeadline(time.Time{})

		log.Printf("[SOCKS5-SERVER] 🔀 创建多路复用会话...")
		session := mux.Client(conn, psk)
		log.Printf("[SOCKS5-SERVER] ✅ 多路复用会话已创建，启动隧道服务器...")

		go func() {
			TunnelServer(socksListenAddress, username, password, session)
			log.Printf("[SOCKS5-SERVER] 🔚 隧道服务器已关闭")
			session.Close()
		}()
	}
}

// Accepts connections and tunnels the traffic to the SOCKS server running on the client.
func TunnelServer(listenAddress, username, password string, session *mux.Mux) {
	log.Printf("[SOCKS5] 启动隧道服务器: %s", listenAddress)

	ln, err := net.Listen("tcp", listenAddress)
	if err != nil {
		log.Printf("[SOCKS5] 启动失败: %v", err)
		return
	}
	MuSocks5Server.Lock()
	Socks5Server[listenAddress] = ln
	MuSocks5Server.Unlock()
	// agent 断开后 TunnelServer 返回时关闭 SOCKS5 监听器并从 map 移除，避免端口/监听器泄漏（否则重连会端口占用）
	defer func() {
		ln.Close()
		MuSocks5Server.Lock()
		delete(Socks5Server, listenAddress)
		MuSocks5Server.Unlock()
		log.Printf("[SOCKS5] 隧道监听器已关闭: %s", listenAddress)
	}()

	authMethod := Authenticator(&NoAuthAuthenticator{})

	if len(password) > 0 {
		log.Printf("[SOCKS5] 启用认证")
		authMethod = &UserPassAuthenticator{username, password}
	}

	log.Printf("[SOCKS5] 等待客户端连接...")

	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				log.Printf("[SOCKS5] 服务器关闭")
				break
			} else {
				log.Printf("[SOCKS5] 连接失败: %v", err)
				continue
			}
		}

		stream, err := session.OpenStream()
		if err != nil {
			log.Printf("[SOCKS5] 流创建失败: %v", err)
			conn.Close()
			break
		}

		go handleSocksClient(conn, stream, authMethod)
	}
}

func doauth(conn net.Conn, authMethod Authenticator) error {
	// Check its a really SOCKS5 connection
	mr, err := statute.ParseMethodRequest(conn)
	if err != nil {
		log.Printf("[SOCKS5] 认证失败: %v", err)
		return err
	}
	if mr.Ver != statute.VersionSocks5 {
		log.Printf("[SOCKS5] 不支持的版本: %d", mr.Ver)
		return statute.ErrNotSupportVersion
	}

	// Select a usable method
	for _, method := range mr.Methods {
		if authMethod.GetCode() == method {
			return authMethod.Authenticate(conn)
		}
	}

	// No usable method found
	log.Printf("[SOCKS5] 无支持的认证方法")
	conn.Write([]byte{statute.VersionSocks5, statute.MethodNoAcceptable}) //nolint: errcheck
	return statute.ErrNoSupportedAuth
}

func handleSocksClient(conn net.Conn, stream net.Conn, authMethod Authenticator) {
	defer conn.Close()
	defer stream.Close()

	if err := doauth(conn, authMethod); err != nil {
		log.Printf("[SOCKS5] 认证失败: %v", err.Error())
		return
	}

	// Proxy the data
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		buf := bufferPool.Get()
		defer bufferPool.Put(buf)

		io.CopyBuffer(conn, stream, buf[:cap(buf)])
		// 本方向(stream->conn)结束:关闭自己的dst(conn)=对端的src,解除对端Read阻塞,避免半关闭泄漏
		conn.Close()
	}()
	go func() {
		defer wg.Done()
		buf := bufferPool.Get()
		defer bufferPool.Put(buf)

		io.CopyBuffer(stream, conn, buf[:cap(buf)])
		// 本方向(conn->stream)结束:关闭自己的dst(stream)=对端的src
		stream.Close()
	}()

	wg.Wait()
}

// StopSocks5Server 停止SOCKS5服务器
func StopSocks5Server(listenAddress string) error {
	log.Printf("[SOCKS5-SERVER] 🚫 停止SOCKS5服务器: %s", listenAddress)

	MuSocks5Server.Lock()
	defer MuSocks5Server.Unlock()

	// 查找并关闭监听器
	if listener, exists := Socks5Server[listenAddress]; exists {
		err := listener.Close()
		delete(Socks5Server, listenAddress)
		if err != nil {
			log.Printf("[SOCKS5-SERVER] ⚠️ 关闭监听器失败: %v", err)
			return err
		}
		log.Printf("[SOCKS5-SERVER] ✅ 监听器已关闭: %s", listenAddress)
	} else {
		log.Printf("[SOCKS5-SERVER] ⚠️ 监听器不存在: %s", listenAddress)
	}

	return nil
}
