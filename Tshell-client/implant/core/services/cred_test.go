package services

import "testing"

// 凭据命令的参数校验：文件名会被拼进目标路径，任何路径穿越都必须挡在入口；
// 类型与技术必须成对匹配（把 browser 配成 comsvcs 这种组合在客户端也拦一次，
// 省掉一次必然失败的往返）。
func TestCmdCredDumpValidatesCommand(t *testing.T) {
	cases := []string{
		"",                       // 空
		"lsass",                  // 缺字段
		"lsass|auto",             // 缺文件名
		"|auto|x.dmp",            // 缺类型
		"lsass|auto|",            // 空文件名
		"lsass|auto|../evil.dmp", // 路径穿越
		"lsass|auto|a/b.dmp",     // 子路径
		`lsass|auto|a\b.dmp`,     // Windows 分隔符
	}
	for _, c := range cases {
		if _, err := CmdCredDump([]byte(c)); err == nil {
			t.Errorf("CmdCredDump(%q) 应报错", c)
		}
	}
}

// 跨类型的技术误配必须在客户端当场拒绝（服务端也会拒，但这里拒绝更省一次往返）
func TestCmdCredDumpRejectsMismatchedTech(t *testing.T) {
	cases := []string{
		"browser|comsvcs|x.json",
		"browser|dbgcore|x.json",
		"dpapi|auto|x.zip",
		"dpapi|dbgcore|x.zip",
		"lsass|chrome|x.dmp",
		"lsass|keys|x.dmp",
		"sam|auto|x.dmp", // 未知类型
	}
	for _, c := range cases {
		if _, err := CmdCredDump([]byte(c)); err == nil {
			t.Errorf("CmdCredDump(%q) 应报错", c)
		}
	}
}

// 类型/技术分域与默认值必须与服务端 cred.TechsFor / DefaultTech 一致
// （两处不一致的后果：界面能选、下发即失败，或者反之）
func TestCredTechMatrix(t *testing.T) {
	ok := []struct{ kind, tech string }{
		{"lsass", "auto"}, {"lsass", "comsvcs"}, {"lsass", "dbgcore"},
		{"dpapi", "keys"},
		{"browser", "all"}, {"browser", "chrome"}, {"browser", "edge"}, {"browser", "firefox"},
		{"browser", "cards"},
	}
	for _, c := range ok {
		if !credTechAllowed(c.kind, c.tech) {
			t.Errorf("%s/%s 应被支持", c.kind, c.tech)
		}
	}
	if defaultCredTech("lsass") != "auto" || defaultCredTech("dpapi") != "keys" || defaultCredTech("browser") != "all" {
		t.Fatal("默认技术应按类型取")
	}
}
