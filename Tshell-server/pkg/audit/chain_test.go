package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ===== 审计日志哈希链（阶段三）=====
//
// 这些用例的重点不是"能算哈希"，而是**能定位篡改**：改一行、删一行、
// 换顺序，都必须被指到具体行号。只验证"正常链通过"是没有意义的
// —— 那种用例在链条完全没生效时也会绿。

// writeChainFile 用链计算逻辑造一个日志文件（模拟真实落盘内容）
func writeChainFile(t *testing.T, dir string, n int) string {
	t.Helper()
	path := filepath.Join(dir, "audit_test.log")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	prev := ""
	for i := 0; i < n; i++ {
		e := AuditLog{
			ID:        "id-" + string(rune('a'+i)),
			Timestamp: time.Unix(int64(1700000000+i), 0).UTC(),
			Level:     LogLevelInfo,
			Category:  CategoryAPI,
			Action:    "TEST_ACTION",
			Result:    "SUCCESS",
			Message:   "message-" + string(rune('a'+i)),
			PrevHash:  prev,
		}
		h, err := chainHash(prev, e)
		if err != nil {
			t.Fatal(err)
		}
		e.Hash = h
		raw, _ := json.Marshal(e)
		if _, err := f.WriteString(string(raw) + "\n"); err != nil {
			t.Fatal(err)
		}
		prev = h
	}
	return path
}

func TestVerifyAuditFileHealthyChain(t *testing.T) {
	dir := t.TempDir()
	path := writeChainFile(t, dir, 5)

	res, err := VerifyAuditFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK() {
		t.Fatalf("完好链不应报错，实际断在第 %d 行: %s", res.BadLine, res.Reason)
	}
	if res.Entries != 5 {
		t.Errorf("应校验 5 条，实际 %d", res.Entries)
	}
	if res.LastHash == "" {
		t.Error("应返回链尾哈希（供外部锚点使用）")
	}
}

// 改动一条记录的内容 → 必须定位到该行
func TestVerifyAuditFileDetectsTamper(t *testing.T) {
	dir := t.TempDir()
	path := writeChainFile(t, dir, 5)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	// 改第 3 行（索引 2）的 message
	lines[2] = strings.Replace(lines[2], "message-c", "message-TAMPERED", 1)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := VerifyAuditFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if res.OK() {
		t.Fatal("被改动的记录必须被检出（否则哈希链等于没生效）")
	}
	if res.BadLine != 3 {
		t.Errorf("断点应定位到第 3 行，实际 %d 行", res.BadLine)
	}
	if res.Entries != 2 {
		t.Errorf("断点前应校验通过 2 条，实际 %d", res.Entries)
	}
}

// 删除中间一条 → 必须定位到断点（后续记录的 prev_hash 对不上）
func TestVerifyAuditFileDetectsDeletion(t *testing.T) {
	dir := t.TempDir()
	path := writeChainFile(t, dir, 5)

	raw, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	// 删掉第 3 行
	lines = append(lines[:2], lines[3:]...)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	res, _ := VerifyAuditFile(path)
	if res.OK() {
		t.Fatal("删除一行必须被检出")
	}
	if res.BadLine != 3 {
		t.Errorf("断点应落在被删位置（第 3 行），实际 %d", res.BadLine)
	}
}

// 交换两行顺序 → 必须被检出
func TestVerifyAuditFileDetectsReorder(t *testing.T) {
	dir := t.TempDir()
	path := writeChainFile(t, dir, 4)

	raw, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	lines[1], lines[2] = lines[2], lines[1]
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	res, _ := VerifyAuditFile(path)
	if res.OK() {
		t.Fatal("重排必须被检出")
	}
}

// 历史记录（无 hash）不参与校验，也不该被判成篡改 —— 否则升级后所有老日志都"被篡改"
func TestVerifyAuditFileToleratesLegacyLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit_legacy.log")

	// 两行老格式（无 hash）
	legacy := `{"id":"old-1","action":"OLD","result":"SUCCESS","message":"m1"}
{"id":"old-2","action":"OLD","result":"SUCCESS","message":"m2"}
`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := VerifyAuditFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK() {
		t.Fatalf("老格式记录不应被判为篡改: %s（第 %d 行）", res.Reason, res.BadLine)
	}
	if res.Entries != 0 {
		t.Errorf("老格式不应计入已校验条目，实际 %d", res.Entries)
	}
}

// 链中出现"无 hash"的记录（链已开始后又插了一条旧的）必须被检出
func TestVerifyAuditFileDetectsInjectedLegacyLine(t *testing.T) {
	dir := t.TempDir()
	path := writeChainFile(t, dir, 3)

	raw, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	// 在第 2 条之后插入一条无 hash 的记录
	inj := `{"id":"inj","action":"INJECTED","result":"SUCCESS","message":"x"}`
	lines = append(lines[:1], append([]string{inj}, lines[1:]...)...)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	res, _ := VerifyAuditFile(path)
	if res.OK() {
		t.Fatal("链中插入无 hash 记录必须被检出")
	}
	if res.BadLine != 2 {
		t.Errorf("断点应为第 2 行，实际 %d", res.BadLine)
	}
}

// 恢复链尾：模拟进程重启后接续
func TestRecoverLastHash(t *testing.T) {
	dir := t.TempDir()
	path := writeChainFile(t, dir, 3)

	h, err := recoverLastHash(path)
	if err != nil {
		t.Fatal(err)
	}
	if h == "" {
		t.Fatal("应能从文件末尾恢复链尾哈希")
	}

	// 用恢复出的哈希再接一条：整条链必须依然完好
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	e := AuditLog{ID: "after-restart", Action: "AFTER", Result: "SUCCESS", Message: "m", PrevHash: h}
	nh, err := chainHash(h, e)
	if err != nil {
		t.Fatal(err)
	}
	e.Hash = nh
	raw, _ := json.Marshal(e)
	f.WriteString(string(raw) + "\n")
	f.Close()

	res, _ := VerifyAuditFile(path)
	if !res.OK() {
		t.Fatalf("重启后接续的链应完好，实际断在第 %d 行: %s", res.BadLine, res.Reason)
	}
	if res.Entries != 4 {
		t.Errorf("应校验 4 条，实际 %d", res.Entries)
	}
}

// 空文件 / 不存在文件不 panic
func TestRecoverLastHashEdgeCases(t *testing.T) {
	dir := t.TempDir()

	// 不存在
	if h, err := recoverLastHash(filepath.Join(dir, "nope.log")); err != nil || h != "" {
		t.Errorf("不存在的文件应返回空哈希且不报错，得到 %q, %v", h, err)
	}
	// 空文件
	empty := filepath.Join(dir, "empty.log")
	os.WriteFile(empty, nil, 0o600)
	if h, err := recoverLastHash(empty); err != nil || h != "" {
		t.Errorf("空文件应返回空哈希，得到 %q, %v", h, err)
	}
	// 尾部半行（进程被杀）不应导致恢复失败
	partial := filepath.Join(dir, "partial.log")
	os.WriteFile(partial, []byte(`{"id":"a"}`+"\n"+`{"id":"b","hash":"`), 0o600)
	if _, err := recoverLastHash(partial); err != nil {
		t.Errorf("尾部半行不应导致恢复失败: %v", err)
	}
}

// ===== 端到端：真实日志器落盘后，链必须可校验 =====
//
// 上面那些用例是手工构造文件；这一条走 NewAuditLogger → Log → Close 的真实路径，
// 确保"写出来的东西"与"校验器期望的东西"一致 —— 两边各自对但拼起来不对，
// 是这类改造最常见的失败方式。
func TestRealLoggerWritesVerifiableChain(t *testing.T) {
	dir := t.TempDir()
	lg, err := NewAuditLogger(dir, 8<<20, 7)
	if err != nil {
		t.Fatalf("创建日志器失败: %v", err)
	}
	// 生产里 GlobalAuditLogger 一定被设置；后台落盘 worker 通过它取实例。
	// 不设置的话批次会被守卫丢弃（那正是防御行为，不是测试想要的）。
	old := GlobalAuditLogger
	GlobalAuditLogger = lg
	t.Cleanup(func() { GlobalAuditLogger = old })
	for i := 0; i < 3; i++ {
		lg.Log(LogLevelInfo, CategoryAPI, "E2E_ACTION", "SUCCESS", "e2e-message", map[string]interface{}{"i": i})
	}
	if err := lg.Close(); err != nil {
		t.Fatalf("关闭日志器失败: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var logPath string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "audit_") {
			logPath = filepath.Join(dir, e.Name())
			break
		}
	}
	if logPath == "" {
		t.Fatal("未找到落盘的审计日志文件")
	}

	res, err := VerifyAuditFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK() {
		t.Fatalf("真实日志器写出的链应完好，实际断在第 %d 行: %s", res.BadLine, res.Reason)
	}
	if res.Entries != 3 {
		t.Errorf("应校验 3 条，实际 %d", res.Entries)
	}
	if res.LastHash == "" {
		t.Error("应得到链尾哈希")
	}
}

// 重启后在同一目录继续写：链条必须接得上（靠 recoverLastHash）
func TestRealLoggerResumesChainAcrossRestart(t *testing.T) {
	dir := t.TempDir()

	lg1, err := NewAuditLogger(dir, 8<<20, 7)
	if err != nil {
		t.Fatal(err)
	}
	old := GlobalAuditLogger
	GlobalAuditLogger = lg1
	t.Cleanup(func() { GlobalAuditLogger = old })
	lg1.Log(LogLevelInfo, CategoryAPI, "BEFORE_RESTART", "SUCCESS", "m1", nil)
	if err := lg1.Close(); err != nil {
		t.Fatal(err)
	}

	// 模拟进程重启：新建日志器（同一天 → 同一文件，走 openLogFile 的恢复分支）
	lg2, err := NewAuditLogger(dir, 8<<20, 7)
	if err != nil {
		t.Fatal(err)
	}
	GlobalAuditLogger = lg2
	lg2.Log(LogLevelInfo, CategoryAPI, "AFTER_RESTART", "SUCCESS", "m2", nil)
	if err := lg2.Close(); err != nil {
		t.Fatal(err)
	}

	entries, _ := os.ReadDir(dir)
	var logPath string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "audit_") {
			logPath = filepath.Join(dir, e.Name())
			break
		}
	}
	res, err := VerifyAuditFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK() {
		t.Fatalf("重启后链条应接续，实际断在第 %d 行: %s", res.BadLine, res.Reason)
	}
	if res.Entries != 2 {
		t.Errorf("应有 2 条，实际 %d", res.Entries)
	}
}
