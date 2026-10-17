package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
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
