package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CmdFileSearch 文件搜索: 起点目录递归匹配关键字,结果按 FileBrowse 同格式
// (D/F\tsize\ttime\tperm\tname)回传,前端文件管理器可直接渲染。
// 参数格式: <startDir>|<maxResults>|<keyword>(| 分隔;keyword 大小写不敏感,
// 匹配文件/目录名;不含路径分隔符的纯文件名匹配)。
// 防护: 递归深度上限 8、结果数上限 maxResults(≤5000)、总耗时上限 30s——
// 防止超大目录树(如 C:\ 递归)长时间占用命令循环。
func CmdFileSearch(cmdBuf []byte) ([]byte, error) {
	parts := strings.SplitN(string(cmdBuf), "|", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("用法: filesearch <startDir>|<maxResults>|<keyword>")
	}
	startDir := strings.TrimSpace(parts[0])
	if startDir == "" {
		startDir = "."
	}
	var max int
	if _, err := fmt.Sscan(parts[1], &max); err != nil || max <= 0 {
		max = 500
	}
	if max > 5000 {
		max = 5000
	}
	keyword := strings.ToLower(strings.TrimSpace(parts[2]))
	if keyword == "" {
		return nil, fmt.Errorf("搜索关键字为空")
	}

	if _, err := os.Stat(startDir); err != nil {
		return nil, fmt.Errorf("起始目录不存在或无法访问: %v", err)
	}

	var b strings.Builder
	b.Grow(8192)
	b.WriteString(fmt.Sprintf("SEARCH\t%s\t%s", startDir, keyword))
	count := 0
	deadline := time.Now().Add(30 * time.Second)

	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if count >= max || depth > 8 || time.Now().After(deadline) {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return // 无权限/消失的目录直接跳过
		}
		for _, e := range entries {
			if count >= max || time.Now().After(deadline) {
				return
			}
			name := e.Name()
			if strings.Contains(strings.ToLower(name), keyword) {
				info, err := e.Info()
				if err != nil {
					continue
				}
				modTime := info.ModTime().Format("02/01/2006 15:04:05")
				perm := info.Mode().Perm().String()
				full := filepath.Join(dir, name)
				if e.IsDir() {
					b.WriteString(fmt.Sprintf("\nD\t0\t%s\t%s\t%s", modTime, perm, full))
				} else {
					b.WriteString(fmt.Sprintf("\nF\t%d\t%s\t%s\t%s", info.Size(), modTime, perm, full))
				}
				count++
			}
			// 目录递归(符号链接跳过,防环)
			if e.IsDir() {
				walk(filepath.Join(dir, name), depth+1)
			}
		}
	}
	walk(startDir, 0)

	if count == 0 {
		return []byte(fmt.Sprintf("SEARCH\t%s\t%s\nNO_MATCH", startDir, keyword)), nil
	}
	return []byte(b.String()), nil
}
