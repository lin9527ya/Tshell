package sqlite

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRealChromeDatabase（可选）：本机装了 Chrome/Edge 时直接读它的真实库
// （`Login Data` / `Cookies`）—— 真库里有跨页大 BLOB、多配置、真实列结构，
// 是最有说服力的验证。没有浏览器或库被独占时自动跳过，不影响 CI。
func TestRealChromeDatabase(t *testing.T) {
	roots := []string{
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "User Data"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Edge", "User Data"),
	}
	type target struct{ name, path string }
	var found []target
	for _, root := range roots {
		if root == "" {
			continue
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			for _, f := range []string{"Login Data", "Cookies"} {
				p := filepath.Join(root, e.Name(), f)
				if _, err := os.Stat(p); err == nil {
					found = append(found, target{name: filepath.Base(filepath.Dir(root)) + "/" + e.Name() + "/" + f, path: p})
				}
			}
		}
	}
	if len(found) == 0 {
		t.Skip("本机没有 Chrome/Edge 数据文件，跳过")
	}

	checked := 0
	for _, tg := range found {
		t.Run(tg.name, func(t *testing.T) {
			db, err := Open(tg.path)
			if err != nil {
				t.Skipf("打不开（可能被浏览器独占）: %v", err)
			}
			defer db.Close()

			for _, table := range []string{"logins", "cookies"} {
				tb, err := db.Table(table)
				if err != nil {
					continue // 该库不含这张表
				}
				rows, err := tb.Rows(nil)
				if err != nil {
					t.Fatalf("读取 %s 失败: %v", table, err)
				}
				checked++
				blobCol := "password_value"
				if table == "cookies" {
					blobCol = "encrypted_value"
				}
				t.Logf("%s：页大小=%d 有 WAL=%v 行数=%d", table, db.PageSize(), db.HasWAL(), len(rows))
				for i, r := range rows {
					blob, _ := r[blobCol].([]byte)
					if i < 3 {
						t.Logf("  [%d] 密文长度=%d 前缀=% x", i, len(blob), firstN(blob, 3))
					}
					if len(blob) == 0 {
						continue
					}
					// Chrome/Edge 的密文前缀：v10/v20（新格式）或空/01（老格式）
					switch {
					case len(blob) >= 3 && blob[0] == 'v' && (blob[1] == '1' || blob[1] == '2'):
					case blob[0] == 1, blob[0] == 0:
					default:
						t.Errorf("第 %d 行 %s 前缀异常: % x", i, blobCol, firstN(blob, 8))
					}
				}
			}
		})
	}
	if checked == 0 {
		t.Skip("没有可读的表（可能是全新配置）")
	}
}

func firstN(b []byte, n int) []byte {
	if len(b) < n {
		return b
	}
	return b[:n]
}
