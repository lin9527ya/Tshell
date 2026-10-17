package cna

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// maxOpenFileBytes open() 单文件读取上限，防一次读入超大文件撑爆服务端内存
const maxOpenFileBytes = 10 << 20

// getenvAllowed 允许脚本读取的环境变量白名单。
// getenv 读的是**服务端进程**的环境（不是被控端的），其中包含监听器密钥、JWT 签名密钥、
// 数据库口令等；按名放行比按名拒绝更可靠（拒绝列表永远漏，如 *_AK / *_DSN）。
var getenvAllowed = map[string]bool{
	"PATH": true, "HOME": true, "USER": true, "USERNAME": true,
	"HOSTNAME": true, "COMPUTERNAME": true, "OS": true, "SHELL": true,
	"TEMP": true, "TMPDIR": true, "TMP": true, "LANG": true, "PWD": true,
}

// builtinFunc 内建函数
type builtinFunc func(ip *Interp, env *Env, args []Value) (Value, error)

// builtinFuncs 内建函数表
var builtinFuncs = map[string]builtinFunc{
	// ---- 输出 ----
	"println":      builtinPrintln,
	"print":        builtinPrintln,
	"message":      builtinPrintln,
	"lprint":       builtinPrintln,
	"lprintln":     builtinPrintln,
	"alert":        builtinAlert,
	"prompt_user":  builtinUnsupported,
	"openScriptConsole": builtinUnsupported,
	"closeScriptConsole": builtinUnsupported,

	// ---- 转换 ----
	"to_string": builtinToString,
	"to_string_quiet": builtinToString,
	"to_int":    builtinToInt,
	"to_long":   builtinToInt,
	"to_number": builtinToNumber,
	"to_bool":   builtinToBool,
	"int":       builtinToInt,
	"strlen":    builtinStrlen,
	"to_upper":  func(ip *Interp, env *Env, args []Value) (Value, error) { return strMap(strings.ToUpper, args) },
	"to_lower":  func(ip *Interp, env *Env, args []Value) (Value, error) { return strMap(strings.ToLower, args) },
	"trim":      func(ip *Interp, env *Env, args []Value) (Value, error) { return strMap(strings.TrimSpace, args) },

	// ---- 字符串 ----
	"substr":    builtinSubstr,
	"substring": builtinSubstr,
	"indexof":   builtinIndexof,
	"split":     builtinSplit,
	"join":      builtinJoin,
	"replace":   builtinReplace,
	"contains":  builtinContains,
	"startsWith": func(ip *Interp, env *Env, args []Value) (Value, error) {
		if len(args) >= 2 {
			return strings.HasPrefix(ToString(args[0]), ToString(args[1])), nil
		}
		return false, nil
	},
	"endsWith": func(ip *Interp, env *Env, args []Value) (Value, error) {
		if len(args) >= 2 {
			return strings.HasSuffix(ToString(args[0]), ToString(args[1])), nil
		}
		return false, nil
	},

	// ---- 数组/字典 ----
	"size": builtinSize,
	"length": builtinSize,
	"push": func(ip *Interp, env *Env, args []Value) (Value, error) {
		if len(args) >= 2 {
			if arr, ok := args[0].([]Value); ok {
				return append(append([]Value{}, arr...), args[1:]...), nil
			}
		}
		return nil, nil
	},
	"join_array": builtinJoin,

	// ---- 环境 ----
	"getenv": builtinGetenv,
	"host":   builtinUnsupported,

	// ---- 系统（服务端上下文，安全起见不执行外部命令）----
	"exec":     builtinExec,
	"shell":    builtinUnsupported,
	"system":   builtinUnsupported,
	"spawn":    builtinUnsupported,
	"inject":   builtinUnsupported,
	"bshell":   builtinBShell,
	"bpython":  builtinUnsupported,
	"bdllspawn": builtinUnsupported,
	"breflectiveinject": builtinUnsupported,
	"bhashdump": builtinUnsupported,
	"binstall":  builtinUnsupported,
	"browserpivot": builtinUnsupported,

	// ---- Beacon 元数据 ----
	"beacon_id":        builtinBeaconID,
	"beacon_arch":      builtinBeaconArch,
	"beacon_os":        builtinBeaconOS,
	"beacon_host":      builtinBeaconHost,
	"beacon_user":      builtinBeaconUser,
	"beacon_ip":        builtinBeaconIP,
	"lhost":            builtinLhost,
	"data_query":       builtinDataQuery,
	"listeners":        builtinListeners,
	"on":               builtinUnsupported,
	"alias":            builtinUnsupported,
	"menu":             builtinUnsupported,

	// ---- 延迟 ----
	"sleep":    builtinSleep,
	"sleepless": builtinUnsupported,

	// ---- 文件 ----
	"open":  builtinOpen,
	"read":  builtinUnsupported,
	"write": builtinUnsupported,
	"close": builtinUnsupported,

	// ---- 其他 ----
	"generate_malleable_c2_profile": builtinNotImplemented("generate_malleable_c2_profile"),
	"artifact_payload":              builtinArtifactPayload,
	"input":                         builtinNotImplemented("input"),
	"value":                         func(ip *Interp, env *Env, args []Value) (Value, error) {
		if len(args) > 0 {
			return args[0], nil
		}
		return nil, nil
	},
	"assert": func(ip *Interp, env *Env, args []Value) (Value, error) {
		if len(args) == 0 || !IsTruthy(args[0]) {
			return nil, runErr("断言失败")
		}
		return nil, nil
	},
	"is_admin":  builtinUnsupported,
	"process_list": builtinUnsupported,
}

// builtinRefs 可被 & 引用的内建（按名字注册即可，实际调用走 builtinFuncs）
var builtinRefs = map[string]bool{}

// ---- 实现 ----

func builtinPrintln(ip *Interp, env *Env, args []Value) (Value, error) {
	parts := make([]string, 0, len(args))
	for _, a := range args {
		parts = append(parts, ToString(a))
	}
	msg := strings.Join(parts, " ")
	if ip.Log != nil {
		ip.Log("[cna] %s", msg)
	}
	return nil, nil
}

func builtinAlert(ip *Interp, env *Env, args []Value) (Value, error) {
	msg := ""
	if len(args) > 0 {
		msg = ToString(args[0])
	}
	if EngineLog != nil {
		EngineLog("alert", msg)
	}
	if ip.Log != nil {
		ip.Log("[cna][alert] %s", msg)
	}
	return nil, nil
}

func builtinToString(ip *Interp, env *Env, args []Value) (Value, error) {
	if len(args) == 0 {
		return "", nil
	}
	return ToString(args[0]), nil
}

func builtinToInt(ip *Interp, env *Env, args []Value) (Value, error) {
	if len(args) == 0 {
		return float64(0), nil
	}
	if n, ok := ToInt(args[0]); ok {
		return float64(n), nil
	}
	return float64(0), nil
}

func builtinToNumber(ip *Interp, env *Env, args []Value) (Value, error) {
	if len(args) == 0 {
		return float64(0), nil
	}
	if n, ok := toNum(args[0]); ok {
		return n, nil
	}
	return float64(0), nil
}

func builtinToBool(ip *Interp, env *Env, args []Value) (Value, error) {
	if len(args) == 0 {
		return false, nil
	}
	if b, ok := ToBool(args[0]); ok {
		return b, nil
	}
	return IsTruthy(args[0]), nil
}

func builtinStrlen(ip *Interp, env *Env, args []Value) (Value, error) {
	if len(args) == 0 {
		return float64(0), nil
	}
	return float64(len(ToString(args[0]))), nil
}

func builtinSubstr(ip *Interp, env *Env, args []Value) (Value, error) {
	if len(args) < 2 {
		return "", nil
	}
	s := ToString(args[0])
	start, ok1 := ToInt(args[1])
	if !ok1 {
		return "", nil
	}
	end := int64(len(s))
	if len(args) > 2 {
		if e, ok := ToInt(args[2]); ok {
			end = e
		}
	}
	if start < 0 {
		start = int64(len(s)) + start
	}
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	if end > int64(len(s)) {
		end = int64(len(s))
	}
	if start >= int64(len(s)) {
		return "", nil
	}
	return s[start:end], nil
}

func builtinIndexof(ip *Interp, env *Env, args []Value) (Value, error) {
	if len(args) < 2 {
		return float64(-1), nil
	}
	return float64(strings.Index(ToString(args[0]), ToString(args[1]))), nil
}

func builtinSplit(ip *Interp, env *Env, args []Value) (Value, error) {
	if len(args) == 0 {
		return []Value{}, nil
	}
	sep := ","
	if len(args) > 1 {
		sep = ToString(args[1])
	}
	var arr []Value
	for _, part := range strings.Split(ToString(args[0]), sep) {
		arr = append(arr, part)
	}
	return arr, nil
}

func builtinJoin(ip *Interp, env *Env, args []Value) (Value, error) {
	sep := " "
	if len(args) > 1 {
		sep = ToString(args[0])
		args = args[1:]
	}
	var parts []string
	if len(args) == 1 {
		if arr, ok := args[0].([]Value); ok {
			for _, e := range arr {
				parts = append(parts, ToString(e))
			}
		} else {
			parts = append(parts, ToString(args[0]))
		}
	} else {
		for _, e := range args {
			parts = append(parts, ToString(e))
		}
	}
	return strings.Join(parts, sep), nil
}

func builtinReplace(ip *Interp, env *Env, args []Value) (Value, error) {
	if len(args) < 3 {
		return "", nil
	}
	return strings.ReplaceAll(ToString(args[0]), ToString(args[1]), ToString(args[2])), nil
}

func builtinContains(ip *Interp, env *Env, args []Value) (Value, error) {
	if len(args) < 2 {
		return false, nil
	}
	switch t := args[0].(type) {
	case []Value:
		for _, e := range t {
			if ToString(e) == ToString(args[1]) {
				return true, nil
			}
		}
		return false, nil
	case map[string]Value:
		_, ok := t[ToString(args[1])]
		return ok, nil
	default:
		return strings.Contains(ToString(args[0]), ToString(args[1])), nil
	}
}

func builtinSize(ip *Interp, env *Env, args []Value) (Value, error) {
	if len(args) == 0 {
		return float64(0), nil
	}
	switch t := args[0].(type) {
	case []Value:
		return float64(len(t)), nil
	case map[string]Value:
		return float64(len(t)), nil
	case string:
		return float64(len(t)), nil
	}
	return float64(0), nil
}

// builtinGetenv getenv("NAME")：仅返回白名单内的服务端环境变量，其余一律空串
// （服务端环境含密钥类变量，原实现无限制返回，等于把进程环境交给任意脚本）
func builtinGetenv(ip *Interp, env *Env, args []Value) (Value, error) {
	if len(args) == 0 {
		return nil, nil
	}
	name := ToString(args[0])
	if !getenvAllowed[strings.ToUpper(name)] {
		return "", nil
	}
	return os.Getenv(name), nil
}

// ---- 桥接相关（引擎回调） ----

// builtinExec exec(beacon_id, "cmd")：向 beacon 发送命令
func builtinExec(ip *Interp, env *Env, args []Value) (Value, error) {
	if len(args) < 2 {
		return nil, runErr("exec 需要 beacon_id 与命令")
	}
	uid := ToString(args[0])
	cmd := ToString(args[1])
	if ExecFunc != nil {
		ExecFunc(uid, cmd)
	}
	return nil, nil
}

// builtinBShell bshell("cmd")：当前 beacon 执行 shell 命令
func builtinBShell(ip *Interp, env *Env, args []Value) (Value, error) {
	if len(args) == 0 {
		return nil, nil
	}
	cmd := ToString(args[0])
	if uid, ok := env.Get("_beacon_id"); ok && ToString(uid) != "" && ExecFunc != nil {
		ExecFunc(ToString(uid), "shell "+cmd)
	}
	return nil, nil
}

func builtinBeaconID(ip *Interp, env *Env, args []Value) (Value, error) {
	if uid, ok := env.Get("_beacon_id"); ok {
		return uid, nil
	}
	return "", nil
}

func beaconMeta(env *Env, key string) Value {
	if v, ok := env.Get(key); ok {
		return v
	}
	return ""
}

func builtinBeaconArch(ip *Interp, env *Env, args []Value) (Value, error) {
	return beaconMeta(env, "_beacon_arch"), nil
}

func builtinBeaconOS(ip *Interp, env *Env, args []Value) (Value, error) {
	return beaconMeta(env, "_beacon_os"), nil
}

func builtinBeaconHost(ip *Interp, env *Env, args []Value) (Value, error) {
	return beaconMeta(env, "_beacon_host"), nil
}

func builtinBeaconUser(ip *Interp, env *Env, args []Value) (Value, error) {
	return beaconMeta(env, "_beacon_user"), nil
}

func builtinBeaconIP(ip *Interp, env *Env, args []Value) (Value, error) {
	return beaconMeta(env, "_beacon_ip"), nil
}

func builtinLhost(ip *Interp, env *Env, args []Value) (Value, error) {
	if LhostFunc != nil {
		return LhostFunc(), nil
	}
	return "", nil
}

// builtinDataQuery 数据查询（子集：返回空数组）
func builtinDataQuery(ip *Interp, env *Env, args []Value) (Value, error) {
	if len(args) == 0 {
		return []Value{}, nil
	}
	q := ToString(args[0])
	if DataQueryFunc != nil {
		return DataQueryFunc(q), nil
	}
	return []Value{}, nil
}

func builtinListeners(ip *Interp, env *Env, args []Value) (Value, error) {
	if ListenersFunc != nil {
		return ListenersFunc(), nil
	}
	return []Value{}, nil
}

func builtinSleep(ip *Interp, env *Env, args []Value) (Value, error) {
	if len(args) == 0 {
		return nil, nil
	}
	if n, ok := ToInt(args[0]); ok {
		if n < 0 {
			n = 0
		}
		if n > 60000 {
			n = 60000
		}
		time.Sleep(time.Duration(n) * time.Millisecond)
	}
	return nil, nil
}

// builtinOpen open("path")：只允许读取脚本目录内的文件。
// 原实现是裸 os.ReadFile(任意路径) —— operator 角色经 /api/cna/eval 即可读
// /etc/shadow、config.yaml、jwt_private.pem、database.db，等于把服务端进程的
// 文件读权限交给脚本。这里用 os.Root 限定根目录：绝对路径按根目录相对解析，
// ".." 与指向根外的符号链接一律被拒。
func builtinOpen(ip *Interp, env *Env, args []Value) (Value, error) {
	if len(args) == 0 {
		return "", nil
	}
	if ip == nil || ip.rootDir == "" {
		return nil, runErr("open 不可用：未绑定脚本目录")
	}
	rel := strings.TrimPrefix(filepath.Clean("/"+ToString(args[0])), "/")
	if rel == "" || rel == "." {
		return nil, runErr("open 需要文件路径")
	}

	root, err := os.OpenRoot(ip.rootDir)
	if err != nil {
		return nil, runErr("open 不可用: %v", err)
	}
	defer root.Close()

	f, err := root.Open(rel)
	if err != nil {
		return nil, runErr("open 拒绝访问 %q（仅限脚本目录内文件）: %v", ToString(args[0]), err)
	}
	defer f.Close()

	if st, err := f.Stat(); err == nil && st.Size() > maxOpenFileBytes {
		return nil, runErr("open 文件过大（上限 %d 字节）: %s", maxOpenFileBytes, rel)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxOpenFileBytes))
	if err != nil {
		return nil, runErr("open 读取失败: %v", err)
	}
	return string(data), nil
}

func builtinUnsupported(ip *Interp, env *Env, args []Value) (Value, error) {
	return nil, nil
}

// builtinNotImplemented 返回明确的未实现错误（脚本/控制台可见）
func builtinNotImplemented(name string) builtinFunc {
	return func(ip *Interp, env *Env, args []Value) (Value, error) {
		return nil, runErr("%s 未实现（此版本不支持），请改用 Web Delivery/手动生成载荷", name)
	}
}

// builtinArtifactPayload artifact_payload(format, arch)：生成可执行载荷 artifact。
// CS 语义：使用当前监听器生成 beacon payload；这里桥接到 Tshell 客户端模板生成。
func builtinArtifactPayload(ip *Interp, env *Env, args []Value) (Value, error) {
	if len(args) < 2 {
		return nil, runErr("artifact_payload 需要 format 与 arch 参数，如 artifact_payload(\"exe\", \"x64\")")
	}
	format := strings.ToLower(ToString(args[0]))
	arch := ToString(args[1])
	if ArtifactPayloadFunc == nil {
		return nil, runErr("artifact_payload 引擎未初始化")
	}
	bin, err := ArtifactPayloadFunc(format, arch)
	if err != nil {
		return nil, runErr("artifact_payload 生成失败: %v", err)
	}
	// 返回原始字节串（Go string 可含任意字节，与 Sleep 字节串语义一致，
	// 脚本可 substr/比较/写盘）
	return string(bin), nil
}

func strMap(fn func(string) string, args []Value) (Value, error) {
	if len(args) == 0 {
		return "", nil
	}
	return fn(ToString(args[0])), nil
}
