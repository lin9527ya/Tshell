package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBusinessCodeUsesStorageSeam 业务代码不得直接引用具体存储引擎。
//
// 存储访问统一走 `database.DB()`（Store 接口，见 pkg/database/store.go）：
// 后端可在 SQLite/PostgreSQL 间切换，访问点可统一加日志/重试/指标，替换 ORM 时
// 只需实现该接口。直接引用 `database.Engine` 会让这些能力退化为"改 45 个文件"。
//
// 本用例是"棘轮"：新代码一旦绕开接口就会在 CI 失败；pkg/database 内部与测试文件
// 不受限制（它们本来就需要接触具体引擎）。
func TestBusinessCodeUsesStorageSeam(t *testing.T) {
	root := "."
	var offenders []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel := filepath.ToSlash(path)
		if strings.HasPrefix(rel, "pkg/database/") {
			return nil // 存储层自身
		}
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for i, line := range strings.Split(string(src), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue // 注释里提到不算
			}
			if strings.Contains(line, "database.Engine") {
				offenders = append(offenders, rel+":"+itoa(i+1))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("遍历源码失败: %v", err)
	}
	for _, o := range offenders {
		t.Errorf("%s 直接引用了 database.Engine，请改用 database.DB()（Store 接口）", o)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
