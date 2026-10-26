package commands

import "unicode"

// SplitCommandLine 把一条命令行拆成「程序 + 参数」。
//
// 约定（刻意保持跨平台一致、可预测）：
//   - 双引号 / 单引号包裹的内容作为同一个参数，引号本身不进入结果；
//   - 空白（空格、Tab、换行）在引号外分词；
//   - **反斜杠不做转义**：Windows 路径 `"C:\Windows\System32\x.exe"` 必须原样保留，
//     把 `\` 当转义符会把 `\W`、`\S` 吃掉，程序路径直接失效；
//   - 空引号（`""`）算一个空参数。
//
// 服务端下发的是 `execute "path" arg1 arg2`，客户端两侧都走这套拆分。
func SplitCommandLine(s string) []string {
	args := make([]string, 0, 4)
	var cur []rune
	inSingle, inDouble, hasToken := false, false, false

	flush := func() {
		if hasToken {
			args = append(args, string(cur))
			cur = cur[:0]
			hasToken = false
		}
	}

	for _, r := range s {
		switch {
		case r == '\'' && !inDouble:
			inSingle = !inSingle
			hasToken = true
		case r == '"' && !inSingle:
			inDouble = !inDouble
			hasToken = true
		case unicode.IsSpace(r) && !inSingle && !inDouble:
			flush()
		default:
			cur = append(cur, r)
			hasToken = true
		}
	}
	flush()
	return args
}
