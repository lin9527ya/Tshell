package main

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"BackendTemplate/pkg/audit"
)

// TestPluginRoutePolicy 守卫插件仓库的路由分组：
// 下载（外带可下发执行的载荷二进制）与上传/删除（写入载荷仓库）必须挂在 operatorOnly，
// 只读元数据（list/details）保留在 protected。
//
// 这里做源码级断言而不是起真实路由：main.go 的路由表依赖完整 DB/配置初始化，
// 单元测试里构造不出同一条中间件链，而"路由挂错组"正是本测试要防的回归。
func TestPluginRoutePolicy(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("读取 main.go 失败: %v", err)
	}

	want := map[string]string{
		"/plugin/list":                 "protected",
		"/plugin/details/:pluginName":  "protected",
		"/plugin/download/:pluginName": "operatorOnly",
		"/plugin/upload":               "operatorOnly",
		"/plugin/delete/:pluginName":   "operatorOnly",
	}

	// 匹配形如 `\toperatorOnly.POST("/plugin/upload", api.UploadPlugin)` 的路由注册行
	re := regexp.MustCompile(`(?m)^\s*(\w+)\.(?:GET|POST|DELETE|PUT)\("(/plugin/[^"]*)"`)
	got := map[string]string{}
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		got[m[2]] = m[1]
	}

	if len(got) != len(want) {
		t.Errorf("main.go 中插件路由数量为 %d（%v），期望 %d", len(got), got, len(want))
	}
	for route, group := range want {
		g, ok := got[route]
		if !ok {
			t.Errorf("main.go 中找不到插件路由 %s", route)
			continue
		}
		if g != group {
			t.Errorf("插件路由 %s 挂在 %s，期望 %s", route, g, group)
		}
	}
}

// TestBodyLimitPolicy 守卫 main.go 里的按路径请求体上限。
// 全局默认是 10MB，任何声明了更大上限的上传接口都必须在这里登记，
// 否则 handler 里的大小检查永远走不到（这正是修复前插件上传 50MB 失效的原因）。
// 下列路径是当前代码里全部使用 c.FormFile 的接口。
func TestBodyLimitPolicy(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("读取 main.go 失败: %v", err)
	}
	text := string(src)

	idx := strings.Index(text, "MaxBodyBytesMiddlewareWithLimits(")
	if idx < 0 {
		t.Fatal("main.go 未使用按路径配置的请求体上限中间件")
	}
	block := text[idx:]
	if end := strings.Index(block, "}))"); end > 0 {
		block = block[:end]
	}

	wantPaths := []string{
		"/api/plugin/upload",
		"/api/bin/execute",
		"/api/client/file/upload",
		"/api/webdelivery/host-file",
		"/api/client/inject-mem",
		"/api/cna/upload",
	}
	re := regexp.MustCompile(`"(/api/[^"]+)"\s*:`)
	found := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(block, -1) {
		found[m[1]] = true
	}
	for _, p := range wantPaths {
		if !found[p] {
			t.Errorf("上传接口 %s 未登记请求体上限（会卡在全局 10MB）", p)
		}
	}
}

// TestFileAuditRoutePolicy 守卫"挂在文件操作审计组上的路由必须被登记"这一不变式。
// FileOperationAuditMiddleware 只对登记过的路径产生日志，而它原来一条都没登记
// （前缀表认的是 /api/upload、/api/file，实际挂载的全是 /api/client/file/*），
// 于是这些路由从来没有产生过文件级审计。新增路由时漏登记会在这里失败。
func TestFileAuditRoutePolicy(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("读取 main.go 失败: %v", err)
	}

	re := regexp.MustCompile(`(?m)^\s*(fileAPI|operatorFile)\.(?:GET|POST|DELETE|PUT)\("(/[^"]*)"`)
	routes := re.FindAllStringSubmatch(string(src), -1)
	if len(routes) == 0 {
		t.Fatal("没有解析到任何挂载在 fileAPI/operatorFile 上的路由（中间件是否已被移除？）")
	}

	for _, m := range routes {
		// main.go 里注册的是组内相对路径（fileAPI/operatorFile 都挂在 /api 下）
		fullPath := "/api" + m[2]
		if !audit.CoversFileOperation(fullPath) {
			t.Errorf("%s.%s 挂在文件操作审计组上，但未在 pkg/audit 的 fileOperationRoutes 里登记（该路由不会产生任何文件操作审计）",
				m[1], fullPath)
		}
	}
	t.Logf("已校验 %d 条文件路由的审计覆盖", len(routes))
}

// TestRBACPolicy 守卫关键路由的角色口径（前后端 RBAC 一致的落点）。
// 这些口径是本轮分析里确定的策略，改回去会直接造成"页签能点开但必然 403"
// 或"只读角色能做写操作"，所以用源码级断言钉住。
func TestRBACPolicy(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("读取 main.go 失败: %v", err)
	}
	text := string(src)

	// 组变量 → RequireRole 允许的角色集合（无 Use(RequireRole) 的组不在此表内）
	roleRe := regexp.MustCompile(`(?m)^\s*(\w+)\.Use\(security\.RequireRole\(([^)]*)\)\)`)
	groupRoles := map[string][]string{}
	groupParent := map[string]string{}
	for _, m := range roleRe.FindAllStringSubmatch(text, -1) {
		var roles []string
		for _, r := range strings.Split(m[2], ",") {
			r = strings.TrimSpace(r)
			r = strings.TrimPrefix(r, "security.")
			// RoleAdmin → admin（rbac.go 里角色字面量就是小写）
			r = strings.TrimPrefix(r, "Role")
			r = strings.ToLower(r)
			if r != "" {
				roles = append(roles, r)
			}
		}
		groupRoles[m[1]] = roles
	}

	// 组前缀链：var := parent.Group("prefix")，迭代到收敛（protected→adminOnly→auditAPI 这种嵌套）
	groupPrefix := map[string]string{"r": ""}
	groupRe := regexp.MustCompile(`(?m)^\s*(\w+)\s*:=\s*(\w+)\.Group\("([^"]*)"\)`)
	groupDefs := groupRe.FindAllStringSubmatch(text, -1)
	for pass := 0; pass < 4; pass++ {
		for _, m := range groupDefs {
			parent, ok := groupPrefix[m[2]]
			if !ok {
				continue
			}
			groupPrefix[m[1]] = parent + m[3]
			groupParent[m[1]] = m[2]
		}
	}

	// 子组继承父组的 RequireRole（如 connectionWrite := operatorOnly.Group(...)）
	rolesOf := func(name string) []string {
		for i := 0; i < 4 && name != ""; i++ {
			if r, ok := groupRoles[name]; ok {
				return r
			}
			name = groupParent[name]
		}
		return nil
	}

	// 路由注册："METHOD /绝对路径" → 组变量
	routeRe := regexp.MustCompile(`(?m)^\s*(\w+)\.(GET|POST|PUT|DELETE|PATCH)\("([^"]+)"`)
	registeredBy := map[string]string{}
	for _, m := range routeRe.FindAllStringSubmatch(text, -1) {
		prefix, ok := groupPrefix[m[1]]
		if !ok {
			continue
		}
		registeredBy[m[2]+" "+prefix+m[3]] = m[1]
	}

	// 策略表：路由 → 期望组变量 → 期望允许角色
	cases := []struct {
		route     string
		wantGroup string
		wantRoles []string
		why       string
	}{
		{"GET /api/audit/logs", "auditRead", []string{"admin", "auditor"}, "auditor 的职责就是看审计"},
		{"GET /api/audit/statistics", "auditRead", []string{"admin", "auditor"}, "同上"},
		{"GET /api/audit/export", "auditAPI", []string{"admin"}, "导出行外带全量日志，仍限 admin"},
		{"DELETE /api/audit/cleanup", "auditAPI", []string{"admin"}, "清理是破坏性操作"},
		{"GET /api/connection/pool/stats", "connectionAPI", nil, "连接状态对全角色可见（无 RequireRole）"},
		{"DELETE /api/connection/:id", "connectionWrite", []string{"admin", "operator"}, "断开连接是写操作，auditor 不可"},
		{"PUT /api/connection/pool/config", "connectionWrite", []string{"admin", "operator"}, "改池配置是写操作"},
		{"PUT /api/connection/worker/resize", "connectionWrite", []string{"admin", "operator"}, "改工作池是写操作"},
		{"POST /api/shellcode/stage", "shellcodeAPI", []string{"admin", "operator"}, "生成载荷与 /client/GenServer 同口径"},
		{"POST /api/shellcode/dynamic-stager", "shellcodeAPI", []string{"admin", "operator"}, "同上"},
		{"POST /api/plugin/upload", "operatorOnly", []string{"admin", "operator"}, "写入攻击载荷仓库"},
		{"DELETE /api/plugin/delete/:pluginName", "operatorOnly", []string{"admin", "operator"}, "同上"},
		{"GET /api/plugin/download/:pluginName", "operatorOnly", []string{"admin", "operator"}, "外带可执行载荷"},
		{"POST /api/bin/execute", "operatorOnly", []string{"admin", "operator"}, "下发载荷到被控端"},
		{"GET /api/client/session-log", "operatorOnly", []string{"admin", "operator"}, "会话日志含完整交互记录"},
	}

	for _, tc := range cases {
		gotGroup, ok := registeredBy[tc.route]
		if !ok {
			t.Errorf("main.go 里找不到路由 %s", tc.route)
			continue
		}
		if gotGroup != tc.wantGroup {
			t.Errorf("%s 挂在 %s，期望 %s（%s）", tc.route, gotGroup, tc.wantGroup, tc.why)
			continue
		}
		gotRoles := rolesOf(gotGroup)
		if len(gotRoles) != len(tc.wantRoles) {
			t.Errorf("%s 所在组 %s 的角色为 %v，期望 %v（%s）", tc.route, gotGroup, gotRoles, tc.wantRoles, tc.why)
			continue
		}
		sort.Strings(gotRoles)
		want := append([]string(nil), tc.wantRoles...)
		sort.Strings(want)
		for i := range want {
			if gotRoles[i] != want[i] {
				t.Errorf("%s 所在组 %s 的角色为 %v，期望 %v（%s）", tc.route, gotGroup, gotRoles, tc.wantRoles, tc.why)
				break
			}
		}
	}
}

// TestWSHandshakeRoleGuards WS 端点不走 RequireRole 中间件，握手里的角色校验是唯一防线。
// xterm 给的是被控端交互式 shell，必须与 /api/shell/:uid、HVNC 一样限定 admin/operator。
func TestWSHandshakeRoleGuards(t *testing.T) {
	for _, f := range []string{
		"pkg/api/xterm_terminal.go",
		"pkg/api/hvnc.go",
		"pkg/api/terminal.go",
	} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("读取 %s 失败: %v", f, err)
		}
		if !strings.Contains(string(src), "security.RoleAdmin") || !strings.Contains(string(src), "security.RoleOperator") {
			t.Errorf("%s 的 WS 握手缺少 admin/operator 角色校验", f)
		}
	}
}

// TestLootIngestWiring 守卫战利品自动入库的接线。
// AddLootIfNotEmpty 此前全仓零调用（注释却写着"供回包识别调用"），识别链路是断的；
// 现在经 pkg/command 的回调在命令回包处调用，并由 main 注入实现。
func TestLootIngestWiring(t *testing.T) {
	replySrc, err := os.ReadFile("pkg/command/reply_handlers.go")
	if err != nil {
		t.Fatalf("读取 reply_handlers.go 失败: %v", err)
	}
	if !strings.Contains(string(replySrc), "var LootIngestFunc") {
		t.Error("pkg/command 缺少 LootIngestFunc 回调声明")
	}
	if !strings.Contains(string(replySrc), "LootIngestFunc(uid") {
		t.Error("回包处理里没有调用 LootIngestFunc —— 命令产出不会自动入库")
	}

	mainSrc, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("读取 main.go 失败: %v", err)
	}
	if !strings.Contains(string(mainSrc), "command.LootIngestFunc =") ||
		!strings.Contains(string(mainSrc), "api.IngestLootFromOutput") {
		t.Error("main.go 没有把 api.IngestLootFromOutput 注入到 command.LootIngestFunc")
	}
}
