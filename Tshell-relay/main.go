// tshell-relay 多级中继节点。
//
// 拓扑：
//
//	beacon → 中继1 → 中继2 → … → 团队服务器
//
// 中继只做字节搬运（TCP/UDP），不解析 C2 协议、不持有任何密钥：beacon 与服务端
// 之间的会话加密是端到端的，中继被拿下也解不开指令内容。传输类型对中继透明，
// 因此它可以同时承接 TCP/WebSocket/HTTP(S)，UDP 部分承接 KCP。
//
// 用法：
//
//	./tshell-relay -config relay.json
//
// 配置示例见 config.example.json 与 docs/RELAY.md。
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var Version = "1.0.0"

func main() {
	cfgPath := flag.String("config", "relay.json", "配置文件路径")
	showVersion := flag.Bool("version", false, "打印版本并退出")
	flag.Parse()

	if *showVersion {
		fmt.Println(Version)
		return
	}

	log.SetFlags(log.LstdFlags)
	log.SetPrefix("[relay] ")

	cfg, err := LoadConfig(*cfgPath)
	if err != nil {
		log.Fatalf("❌ 配置错误: %v", err)
	}

	limiter := newConnLimiter(cfg.MaxConns, cfg.MaxConnsPerIP)
	tcpR, err := newTCPRelay(cfg, limiter)
	if err != nil {
		log.Fatalf("❌ TCP 中继初始化失败: %v", err)
	}

	var udpR *udpRelay
	if cfg.UDPListen != "" {
		udpR = newUDPRelay(cfg)
		go func() {
			if err := udpR.Serve(); err != nil {
				log.Printf("❌ UDP 中继退出: %v", err)
			}
		}()
	}

	var healthSrv *http.Server
	if cfg.HealthListen != "" {
		h := &healthServer{tcp: tcpR, udp: udpR, started: time.Now(), version: Version}
		ln, err := net.Listen("tcp", cfg.HealthListen)
		if err != nil {
			log.Fatalf("❌ 健康检查监听失败: %v", err)
		}
		healthSrv = h.serve(ln)
		log.Printf("🩺 健康检查: http://%s/healthz", cfg.HealthListen)
	}

	// 信号处理：优雅退出
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		log.Printf("🛑 收到退出信号，正在关闭…")
		if healthSrv != nil {
			_ = healthSrv.Close()
		}
		if udpR != nil {
			_ = udpR.Close()
		}
		// TCP 监听在 Serve 的 defer 关闭；这里退出进程即可
		os.Exit(0)
	}()

	if err := tcpR.Serve(); err != nil {
		log.Fatalf("❌ TCP 中继退出: %v", err)
	}
}
