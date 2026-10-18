package services

import "testing"

// 凭据命令的参数校验：文件名会被拼进目标路径，任何路径穿越都必须挡在入口。
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
