package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/sha1"
	"crypto/sha256"
	"errors"
	"fmt"
)

// ===== Firefox / NSS 凭据解密所需的密码学原语 =====
//
// 更正一处早先的误判（重要）：本文件此前只有 PKCS#12 PBE（pkcs12.go）。
// 对照参考实现（lclevy/firepwd 的 firepwd.py，NSS 3.49 / Firefox 144 均支持）
// 后确认，**Firefox 实际用的不是 PKCS#12 PBE**：
//
//	路径                          实际算法
//	logins.json（旧版）          裸 des-ede3-cbc：主密钥直接当密钥，IV 取自 ASN.1
//	logins.json（Firefox 144+）  裸 aes-256-cbc：主密钥截断到 32 字节，IV 取自 ASN.1
//	key4.db（NSS 3.49+）         PBES2：PBKDF2-HMAC-SHA256 + aes-256-cbc
//	key4.db（更早）              Mozilla 自有 KDF（本文件 nssMozKDF）+ 3DES
//
// pkcs12.go 保留：它实现的是 RFC 7292 附录 B，本身有已知答案向量、是正确的，
// 但**不要**拿它去解 Firefox 的数据 —— 那正是本轮差点犯的错。
// 是否删除它由后续决定（可能用于其它 PKCS#12 容器），此处不动。
//
// 验证策略（分层，明确各自能证明什么）：
//   - PBKDF2：与**两个独立实现**（stdlib crypto/pbkdf2 与 x/crypto/pbkdf2）
//     做差分测试 —— 这是本文件里证据最强的一项
//   - CBC 解密：往返 + 边界；底层 AES/3DES 是标准库，这里验的是"管道"
//     （密钥截断、IV 取值、去填充）
//   - Mozilla KDF：只能做结构测试。**端到端正确性必须有真实 profile 夹具**，
//     本文件不假装它已被验证

var errNSSDecrypt = errors.New("NSS 解密失败（口令/主密钥错误或数据损坏）")

// pbes2AES256CBCDecrypt PBES2（RFC 8018）解密：PBKDF2-HMAC-SHA256 派生 32 字节密钥，
// 再用 AES-256-CBC + 显式 IV 解密。用于 NSS 3.49+ 的 key4.db。
//
// 注意 password 是**原始字节**而不是字符串：NSS 这一层喂进来的是
// SHA1(globalSalt || masterPassword) 的摘要（20 字节），不是可打印口令。
// 用 string 会因非法 UTF-8 被替换字符破坏 —— 这类 bug 表现为"口令明明对却解不开"。
func pbes2AES256CBCDecrypt(password, salt, iv, ciphertext []byte, iterations int) ([]byte, error) {
	if len(iv) != aes.BlockSize {
		return nil, fmt.Errorf("%w: IV 长度应为 %d，实际 %d", errNSSDecrypt, aes.BlockSize, len(iv))
	}
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("%w: 密文长度 %d 不是块大小 %d 的整数倍",
			errNSSDecrypt, len(ciphertext), aes.BlockSize)
	}
	if iterations < 1 {
		return nil, fmt.Errorf("%w: 迭代次数非法: %d", errNSSDecrypt, iterations)
	}
	key, err := pbkdf2.Key(sha256.New, string(password), salt, iterations, 32)
	if err != nil {
		return nil, fmt.Errorf("%w: PBKDF2 派生失败: %v", errNSSDecrypt, err)
	}
	return aesCBCDecrypt(key, iv, ciphertext)
}

// nssTripleDESCBCDecrypt 裸 3DES-CBC（logins.json 旧格式）。
// key 必须是 24 字节；IV 8 字节，来自条目 ASN.1 里的显式 IV。
func nssTripleDESCBCDecrypt(key, iv, ciphertext []byte) ([]byte, error) {
	if len(key) != 24 {
		return nil, fmt.Errorf("%w: 3DES 密钥应为 24 字节，实际 %d", errNSSDecrypt, len(key))
	}
	if len(iv) != des.BlockSize {
		return nil, fmt.Errorf("%w: IV 长度应为 %d，实际 %d", errNSSDecrypt, des.BlockSize, len(iv))
	}
	if len(ciphertext) == 0 || len(ciphertext)%des.BlockSize != 0 {
		return nil, fmt.Errorf("%w: 密文长度 %d 不是块大小 %d 的整数倍",
			errNSSDecrypt, len(ciphertext), des.BlockSize)
	}
	block, err := des.NewTripleDESCipher(key)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, ciphertext)
	return pkcs12Unpad(out, des.BlockSize)
}

// nssAES256CBCDecrypt 裸 AES-256-CBC（logins.json Firefox 144+ 格式）。
//
// 主密钥可能是 48 字节（3DES 时代）+ 或 32 字节；48 字节时**截断到前 32**，
// 这是参考实现的做法（`final_key = key[:32]`）。不截断会直接因密钥长度报错。
func nssAES256CBCDecrypt(masterKey, iv, ciphertext []byte) ([]byte, error) {
	key := masterKey
	switch len(masterKey) {
	case 48:
		key = masterKey[:32]
	case 32:
		// 已是 AES-256
	default:
		return nil, fmt.Errorf("%w: 主密钥应为 32 或 48 字节，实际 %d", errNSSDecrypt, len(masterKey))
	}
	if len(iv) != aes.BlockSize {
		return nil, fmt.Errorf("%w: IV 长度应为 %d，实际 %d", errNSSDecrypt, aes.BlockSize, len(iv))
	}
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("%w: 密文长度 %d 不是块大小 %d 的整数倍",
			errNSSDecrypt, len(ciphertext), aes.BlockSize)
	}
	return aesCBCDecrypt(key, iv, ciphertext)
}

func aesCBCDecrypt(key, iv, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, ciphertext)
	return pkcs12Unpad(out, aes.BlockSize)
}

// nssIVFromOctetString 复现 NSS 的 IV 取值"怪癖"。
//
// 背景：key4.db 的 PBES2 参数里，IV 以 DER OCTET STRING 存放，内容只有 **14** 字节，
// 而 AES 需要 16 字节。NSS 的实现是**从 DER 的 tag 字节开始读 16 字节** ——
// 也就是把 `04 0e`（OCTET STRING 的 tag + 长度）当成 IV 的前两个字节。
//
// 参考实现（firepwd）里写作 `iv = b'\x04\x0e' + <14 字节内容>`，与上述一致。
//
// 这个细节必须精确复现：写成"补两个 0"或"取 14 字节"都会导致解密失败，
// 而症状只是"解不开"，很难反查到是 IV 差了两个字节。
// 长度已经是 16 的（某些版本直接存 16 字节）原样返回。
func nssIVFromOctetString(raw []byte) ([]byte, error) {
	switch len(raw) {
	case aes.BlockSize:
		return raw, nil
	case 14:
		out := make([]byte, aes.BlockSize)
		out[0] = 0x04 // DER OCTET STRING tag
		out[1] = 0x0e // 长度 14
		copy(out[2:], raw)
		return out, nil
	default:
		return nil, fmt.Errorf("%w: IV 内容应为 14 或 16 字节，实际 %d", errNSSDecrypt, len(raw))
	}
}

// nssMozKDF Mozilla 自有 KDF（key3.db 时代，部分 key4.db 仍在用）。
//
// 与 PBKDF2/PKCS#12 都不同，它是 HMAC-SHA1 的固定组合：
//
//	hp  = SHA1(globalSalt || masterPassword)
//	chp = SHA1(hp || entrySalt)
//	pes = entrySalt 右侧补 0 到 20 字节
//	k1  = HMAC-SHA1(chp, pes || entrySalt)
//	tk  = HMAC-SHA1(chp, pes)
//	k2  = HMAC-SHA1(chp, tk || entrySalt)
//	k   = k1 || k2      （40 字节；前 24 为 3DES 密钥，末 8 为 IV）
//
// 注意 masterPassword 是**原始字节**（通常为空，即"未设主口令"）。
// 返回值按参考实现切分：key = k[:24]，iv = k[len(k)-8:]。
func nssMozKDF(globalSalt, masterPassword, entrySalt []byte) (key, iv []byte) {
	hp := sha1.Sum(concat(globalSalt, masterPassword))
	chp := sha1.Sum(concat(hp[:], entrySalt))

	// pes = entrySalt 补 0 到 20 字节（若 entrySalt 已 >=20 则原样）
	pes := make([]byte, 20)
	copy(pes, entrySalt)

	k1 := hmacSHA1(chp[:], concat(pes, entrySalt))
	tk := hmacSHA1(chp[:], pes)
	k2 := hmacSHA1(chp[:], concat(tk, entrySalt))

	k := concat(k1, k2) // 40 字节
	return k[:24], k[len(k)-8:]
}

func hmacSHA1(key, data []byte) []byte {
	m := hmac.New(sha1.New, key)
	m.Write(data)
	return m.Sum(nil)
}

func concat(parts ...[]byte) []byte {
	n := 0
	for _, p := range parts {
		n += len(p)
	}
	out := make([]byte, 0, n)
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}
