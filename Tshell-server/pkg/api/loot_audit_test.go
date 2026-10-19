package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"BackendTemplate/pkg/database"

	"github.com/gin-gonic/gin"
)

// ===== OPSEC P0-2：战果的增/查/删都要留痕，且审计里不得出现密钥本体 =====
//
// 之前只有 /cred/* 的"下发提取"被审计（CRED_ACCESS），而"谁把库里的口令
// 翻出来看了"完全没有记录 —— 这是明显的能力缺口：战果库自己成了盲区。
//
// 另一条同样重要：**审计日志不能成为新的泄露面**。如果为了"审计"把 secret
// 直接写进日志，等于把口令又复制到一份长期留存、还会被打包外发的文件里。

type auditCall struct {
	action, result, message string
	details                 map[string]interface{}
}

// captureLootAudit 把战果审计换成同步捕获。
//
// 不用真实审计日志的原因：它是异步批量落盘的，断言要等落盘还会被后台 worker
// 的时序影响；而这里要验的是"记了哪些字段、有没有带上密钥"，
// 同步捕获参数既确定又直击要害。
func captureLootAudit(t *testing.T) func() []auditCall {
	t.Helper()
	old := lootAuditEvent
	var calls []auditCall
	lootAuditEvent = func(action, result, message string, details map[string]interface{}) {
		calls = append(calls, auditCall{action, result, message, details})
	}
	t.Cleanup(func() { lootAuditEvent = old })
	return func() []auditCall { return calls }
}

func lootRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if u := c.GetHeader("X-Test-User"); u != "" {
			c.Set("username", u)
		}
		c.Next()
	})
	r.GET("/loot", ListLoot)
	r.POST("/loot", AddLoot)
	r.DELETE("/loot/:id", DeleteLoot)
	return r
}

func doLoot(t *testing.T, r *gin.Engine, method, path, user, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if user != "" {
		req.Header.Set("X-Test-User", user)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// 把所有审计文本与 details 拍平成一个串，便于断言"是否出现敏感值"
func flattenCalls(calls []auditCall) string {
	var sb strings.Builder
	for _, c := range calls {
		sb.WriteString(c.action + " " + c.result + " " + c.message)
		for k, v := range c.details {
			sb.WriteString(" ")
			sb.WriteString(k)
			sb.WriteString("=")
			sb.WriteString(jsonAny(v))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func jsonAny(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func TestLootCRUDIsAudited(t *testing.T) {
	setupLootTest(t)
	getCalls := captureLootAudit(t)
	r := lootRouter()

	const secret = "AUDIT-MUST-NOT-LEAK-8842"

	// 增
	w := doLoot(t, r, http.MethodPost, "/loot", "alice",
		`{"uid":"uid-1","type":"pwd","user":"bob","realm":"CORP","host":"DC01","secret":"`+secret+`","note":"n"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("录入失败: %d %s", w.Code, w.Body.String())
	}
	// 回包必须回明文（不能把密文 v1:... 当口令回显给前端）
	var created struct {
		Data struct {
			Secret string `json:"secret"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("解析回包失败: %v", err)
	}
	if created.Data.Secret != secret {
		t.Errorf("录入回包应含明文 secret，实际 %q（密文被回显了？）", created.Data.Secret)
	}

	// 查
	if w := doLoot(t, r, http.MethodGet, "/loot?uid=uid-1", "alice", ""); w.Code != http.StatusOK {
		t.Fatalf("查询失败: %d", w.Code)
	}
	// 删
	if w := doLoot(t, r, http.MethodDelete, "/loot/1", "alice", ""); w.Code != http.StatusOK {
		t.Fatalf("删除失败: %d", w.Code)
	}

	calls := getCalls()
	got := map[string]int{}
	for _, c := range calls {
		got[c.action]++
	}
	for _, action := range []string{"LOOT_ADD", "LOOT_LIST", "LOOT_DELETE"} {
		if got[action] == 0 {
			t.Errorf("审计缺少 %s 记录，实际: %v", action, got)
		}
	}
	// 结果要能区分成功/失败
	for _, c := range calls {
		if c.result != "SUCCESS" && c.result != "FAILURE" {
			t.Errorf("%s 的 result 异常: %q", c.action, c.result)
		}
	}
	// 操作者要能追溯
	if !strings.Contains(flattenCalls(calls), "alice") {
		t.Error("审计应记录操作者")
	}
	// 绝不能出现密钥本体 —— 本项最容易被写错的地方
	if strings.Contains(flattenCalls(calls), secret) {
		t.Error("审计里出现了战果密钥本体（审计本身成了泄露面）")
	}
	// 但"是否含密钥"这个事实要记下来，否则审计说不清录的是什么
	if !strings.Contains(flattenCalls(calls), "has_secret") {
		t.Error("审计应记录 has_secret 标记（记布尔值，不记内容）")
	}
}

// 删除要记到具体 id（破坏性操作必须能追溯到"删了哪一条"）
func TestLootDeleteAuditNamesTheID(t *testing.T) {
	setupLootTest(t)
	getCalls := captureLootAudit(t)
	r := lootRouter()

	// 先建一条
	if w := doLoot(t, r, http.MethodPost, "/loot", "alice",
		`{"uid":"uid-1","type":"note","secret":"x"}`); w.Code != http.StatusOK {
		t.Fatalf("录入失败: %d", w.Code)
	}
	if w := doLoot(t, r, http.MethodDelete, "/loot/1", "alice", ""); w.Code != http.StatusOK {
		t.Fatalf("删除失败: %d", w.Code)
	}

	var del auditCall
	for _, c := range getCalls() {
		if c.action == "LOOT_DELETE" && c.result == "SUCCESS" {
			del = c
		}
	}
	if del.action == "" {
		t.Fatal("没有删除成功审计")
	}
	if !strings.Contains(del.message, "id=1") && !strings.Contains(jsonAny(del.details["id"]), "1") {
		t.Errorf("删除审计未点明 id: %s / %v", del.message, del.details)
	}
}

// 录入失败（类型非法）不进库，但值得留痕；且同样不得泄露密钥
func TestLootAddFailureDoesNotLeakSecret(t *testing.T) {
	setupLootTest(t)
	getCalls := captureLootAudit(t)
	r := lootRouter()

	const secret = "SHOULD-NOT-LEAK-1199"
	w := doLoot(t, r, http.MethodPost, "/loot", "alice",
		`{"uid":"uid-1","type":"bogus","secret":"`+secret+`"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("非法 type 应返回 400，实际 %d", w.Code)
	}
	// 校验阶段就不该记"录入失败"（还没到入库那一步），但无论记没记都不能带密钥
	if strings.Contains(flattenCalls(getCalls()), secret) {
		t.Error("任何审计文本都不得出现密钥本体")
	}
	// 确认没有把这条写进库
	var n int64
	if n, _ = database.Engine.Count(new(database.Loot)); n != 0 {
		t.Errorf("非法 type 不应入库，实际 %d 条", n)
	}
}
