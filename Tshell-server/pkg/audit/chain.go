package audit

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// ===== 审计日志防篡改（哈希链，OPSEC 阶段三）=====
//
// 威胁模型：能改日志的人（入侵者清痕迹、内部人掩盖操作）。仅追加文件本身
// 挡不住"删掉某一行再保存"，但哈希链能让**任何**修改都可定位到具体行号：
// 每条的 Hash = SHA256(PrevHash || 该条规范化 JSON)，改动/删除/重排都会破坏后续链条。
//
// 设计取舍（必须写清，否则会被误当成"完全防篡改"）：
//
//  1. **截断末尾无法被本文件发现**：删掉最后 N 行后，剩余部分依然是一条自洽的链。
//     所以需要 lastHash 外部锚点（见 AnchorFile），或者与库里的副本交叉核对。
//  2. **清理旧日志会破坏跨文件全链验证**：cleanupOldLogs 删掉的早期文件不再参与校验，
//     验证只能覆盖"当前保留窗口"。这不是缺陷，但必须写进文档，不能宣称"全历史可验"。
//  3. 链只能证明"这之后的记录未被改动"，不能证明"记录本身是真实的" ——
//     本进程内的操作是否可信，取决于进程本身未被控制。

// chainHash 计算一条审计记录的链接哈希。
//
// prevHash 为空表示链首。payload 是该条**去掉 hash 字段后**的规范化 JSON
// （Go 的 json.Marshal 对结构体字段有序、对 map 键排序，所以是确定性的）。
func chainHash(prevHash string, entry AuditLog) (string, error) {
	e := entry
	e.Hash = "" // 参与计算时不含自身哈希
	raw, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write([]byte(prevHash))
	h.Write([]byte("|"))
	h.Write(raw)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// VerifyResult 校验结果
type VerifyResult struct {
	// Entries 成功校验的条目数（遇到断点前的条数）
	Entries int
	// BadLine 首个断点的行号（1-based）；0 表示整条链完好
	BadLine int
	// Reason 断点原因
	Reason string
	// LastHash 最后一条的哈希（可作为下一次校验的外部锚点）
	LastHash string
}

// OK 链是否完好
func (r VerifyResult) OK() bool { return r.BadLine == 0 }

// VerifyAuditFile 校验一个审计日志文件的哈希链，定位首个断点。
//
// 兼容历史文件：改动前的记录没有 hash 字段 —— 遇到第一条"无 hash"的记录时
// 停止链式校验并返回 Entries 计数，Reason 说明"此前的记录早于哈希链上线"。
// 这样升级后既不会把老文件判成被篡改，也不会假装它们受保护。
func VerifyAuditFile(path string) (VerifyResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return VerifyResult{}, err
	}
	defer f.Close()

	var res VerifyResult
	prev := ""
	sc := bufio.NewScanner(f)
	// 单行可能很长（详情 JSON），放宽到 8MB
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	lineNo := 0
	seenChain := false

	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e AuditLog
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			res.BadLine = lineNo
			res.Reason = "JSON 解析失败: " + err.Error()
			return res, nil
		}
		if e.Hash == "" {
			// 链上线之前的记录：不参与校验，也不判为篡改
			if !seenChain {
				continue
			}
			res.BadLine = lineNo
			res.Reason = "链中出现无 hash 的记录（疑似被替换为旧格式记录）"
			return res, nil
		}
		if e.PrevHash != prev {
			res.BadLine = lineNo
			res.Reason = fmt.Sprintf("prev_hash 断裂：期望 %q，实际 %q", prev, e.PrevHash)
			return res, nil
		}
		want, herr := chainHash(prev, e)
		if herr != nil {
			res.BadLine = lineNo
			res.Reason = "重算哈希失败: " + herr.Error()
			return res, nil
		}
		if want != e.Hash {
			res.BadLine = lineNo
			res.Reason = "本条内容已被改动（重算哈希不匹配）"
			return res, nil
		}
		prev = e.Hash
		res.Entries++
		res.LastHash = e.Hash
		seenChain = true
	}
	if err := sc.Err(); err != nil {
		return res, err
	}
	return res, nil
}

// recoverLastHash 从已有日志文件末尾恢复链尾哈希（进程重启后接续链条）。
//
// 从后往前找第一条带 hash 的记录：文件可能以半行/空行结尾（进程被杀），
// 从末尾回溯比顺序扫全文件便宜，也更能容忍尾部半行。
func recoverLastHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	defer f.Close()

	const tailBytes = 64 * 1024
	st, err := f.Stat()
	if err != nil {
		return "", err
	}
	size := st.Size()
	if size <= 0 {
		return "", nil
	}
	start := int64(0)
	if size > tailBytes {
		start = size - tailBytes
	}
	if _, err := f.Seek(start, 0); err != nil {
		return "", err
	}
	buf := make([]byte, size-start)
	if _, err := f.Read(buf); err != nil && err.Error() != "EOF" {
		// 读到 EOF 属正常（Read 可能返回 n>0 且 err=EOF）
		if len(buf) == 0 {
			return "", err
		}
	}
	lines := strings.Split(string(buf), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var e AuditLog
		if json.Unmarshal([]byte(line), &e) != nil {
			continue // 尾部可能是半行
		}
		return e.Hash, nil
	}
	return "", nil
}

// ErrAuditChainBroken 链断裂（供调用方判定）
var ErrAuditChainBroken = errors.New("审计日志哈希链断裂")
