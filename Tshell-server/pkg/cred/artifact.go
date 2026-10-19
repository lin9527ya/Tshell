package cred

import (
	"fmt"
	"os"
)

// 制品校验的统一入口。
//
// 本轮新增的两类制品（dpapi / browser）与 LSASS 转储的差别在于：
//   - dpapi 是 ZIP（主密钥集已是明文 masterkey，服务端只看形态是否完整）；
//   - browser 是 JSON（**凭据已经是明文**，服务端要多做一步：解析出来登记进凭据库）。
//
// 三者共用的判定标准只有一条：回传的东西必须"完整且可用"，半截文件一律显式失败。
const (
	// 制品体积上限：正常浏览器凭据 JSON 在数百 KB 量级，8MB 足够覆盖多配置 + 大量 Cookie，
	// 超出说明客户端没有按上限截断（或文件被拼坏）。
	MaxBrowserArtifactBytes = 8 << 20
	// Cookie 条目上限：单机 Cookie 上万条很常见，但不能无上限写库。
	MaxBrowserCookies = 20000
	// ZIP 未压缩总大小上限（防 ZIP 炸弹：WinZip AES 不涉及，但中心目录里的
	// UncompressedSize64 是攻击者可控字段，必须在上限处收口）
	MaxZipUncompressedBytes = 64 << 20
	MaxZipEntries           = 512
)

// ArtifactInfo 制品校验结果（写入任务结果与制品备注）
type ArtifactInfo struct {
	Size         int64  `json:"size"`
	Format       string `json:"format"`
	Entries      int    `json:"entries,omitempty"`       // ZIP：条目数；minidump：数据流数
	HasManifest  bool   `json:"has_manifest,omitempty"`  // ZIP：是否含 manifest.json
	HasMemory    bool   `json:"has_memory,omitempty"`    // minidump：是否含完整内存
	Browsers     int    `json:"browsers,omitempty"`      // 浏览器制品：浏览器配置数
	Credentials  int    `json:"credentials,omitempty"`   // 浏览器制品：明文口令条数
	Cookies      int    `json:"cookies,omitempty"`       // 浏览器制品：Cookie 条数
	TicketCaches int    `json:"ticket_caches,omitempty"` // 票据制品：ccache 个数
	Tickets      int    `json:"tickets,omitempty"`       // 票据制品：票据条目总数
	SkippedItems int    `json:"skipped,omitempty"`       // 客户端上报的采集错误条数
}

// ValidateArtifact 按制品形态校验回传文件。
// format 为空时按 kind 推断（兼容未登记 Format 的旧记录）。
func ValidateArtifact(kind, format, path string) (ArtifactInfo, error) {
	if format == "" {
		format = FormatByKind(kind)
	}
	switch format {
	case FormatMinidump:
		d, err := ValidateMinidump(path)
		if err != nil {
			return ArtifactInfo{Format: format}, err
		}
		return ArtifactInfo{
			Size: d.Size, Format: FormatMinidump, Entries: d.Streams, HasMemory: d.HasMemory,
		}, nil
	case FormatZip:
		if kind == KindTicket {
			art, err := ParseTicketArtifact(path)
			if err != nil {
				return ArtifactInfo{Format: format}, err
			}
			info := art.Info()
			if st, err := os.Stat(path); err == nil {
				info.Size = st.Size()
			}
			return info, nil
		}
		return ValidateZip(path)
	case FormatJSON:
		art, err := ParseBrowserArtifact(path)
		if err != nil {
			return ArtifactInfo{Format: format}, err
		}
		info := art.Info()
		if st, err := os.Stat(path); err == nil {
			info.Size = st.Size()
		}
		return info, nil
	default:
		return ArtifactInfo{}, fmt.Errorf("未知的制品形态: %s", format)
	}
}

// Describe 生成给操作员看的一句话说明（写入制品备注）
func (a ArtifactInfo) Describe() string {
	mb := float64(a.Size) / 1024 / 1024
	switch a.Format {
	case FormatZip:
		manifest := ""
		if a.HasManifest {
			manifest = " / 含 manifest.json"
		}
		if a.TicketCaches > 0 {
			desc := fmt.Sprintf("%.2f MB / %d 个 Kerberos 缓存 / %d 张票据%s",
				mb, a.TicketCaches, a.Tickets, manifest)
			if a.SkippedItems > 0 {
				desc += fmt.Sprintf(" / %d 条解析错误", a.SkippedItems)
			}
			return desc
		}
		return fmt.Sprintf("%.2f MB / ZIP %d 个条目%s", mb, a.Entries, manifest)
	case FormatJSON:
		desc := fmt.Sprintf("%.2f MB / %d 个浏览器配置 / %d 条口令", mb, a.Browsers, a.Credentials)
		if a.Cookies > 0 {
			desc += fmt.Sprintf(" / %d 条 Cookie", a.Cookies)
		}
		if a.SkippedItems > 0 {
			desc += fmt.Sprintf(" / %d 条采集错误（见制品 errors 字段）", a.SkippedItems)
		}
		return desc
	default:
		kind := "仅元数据"
		if a.HasMemory {
			kind = "含完整内存"
		}
		return fmt.Sprintf("%.1f MB / %d 个数据流 / %s", mb, a.Entries, kind)
	}
}
