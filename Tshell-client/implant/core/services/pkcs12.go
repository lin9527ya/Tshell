package services

import (
	"crypto/cipher"
	"crypto/des"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"unicode/utf16"
)

// ===== PKCS#12 PBE（Firefox/NSS 凭据解密所需的密码学核心）=====
//
// 为什么需要它：Firefox 的 `key4.db` 用 `pbeWithSHAAnd3-KeyTripleDES-CBC`
// （OID 1.2.840.113549.1.12.1.3）保护它的主密钥，而 `logins.json` 里的口令
// 又用那把主密钥加密。要读 Firefox 保存的凭据，就必须实现这套 PBE。
//
// 算法出处：RFC 7292 附录 B（"Deriving Keys and IVs from Passwords and Salt"）。
// 该附录的 B.2 给出了完整步骤，本文件按步骤 1:1 实现。
//
// 正确性怎么保证（这一条很关键）：RFC 本身不带测试向量，而"自己加密、自己解密"
// 的往返测试只能证明自洽 —— KDF 理解错了两边会一起错，测试照样通过。
// 因此本文件用的是**已知答案向量（KAT）**，取自 Go 官方 x/crypto 的
// `pkcs12/pbkdf_test.go`（其中 "sesame" 那组是 PKCS#12 规范里的经典向量）：
//
//	向量 1：salt=ffffffffffffffff, password=bmp("sesame"), r=2048, ID=1, n=24
//	        → 7cd9fd3e2b3be7691a44e3bef0f9ea0fb9b897d4e325d9d1
//	向量 2：salt=f37e05b518324b4b, password=0000, r=2048, ID=1, n=24
//	        → 00f759ff47d14dd03665d5943cb3c4a39a2555c02aed66e1
//
// 向量 2 尤其重要：它专门覆盖"I_j 加法后出现前导零"这一情形 —— 用 big.Int 实现
// 会把前导零吃掉，导致块长不足 v 而派生结果错误（x/crypto 曾有过这个 bug）。
// 本实现用**定长字节加法**，从结构上避免该问题。

// pkcs12KDFParams 一次 PBE 派生所需的参数。
// u/v 是哈希的输出/块字节数（SHA-1：u=20, v=64）。
type pkcs12KDFParams struct {
	newHash func() hash.Hash
	u       int
	v       int
}

var sha1KDFParams = pkcs12KDFParams{newHash: sha1.New, u: 20, v: 64}

// errPKCS12 统一的解密失败错误（不区分"口令错"与"数据损坏"，
// 避免把两者区别暴露给调用方用于试探）。
var errPKCS12 = errors.New("PKCS#12 解密失败（口令错误或数据损坏）")

// fillWithRepeats 把 pattern 重复拼接并用最后一次的部分拷贝截断到 n 字节。
//
// 对应 RFC 7292 B.2 步骤 2/3/6B 的"concatenate copies ... the final copy
// may be truncated"。pattern 为空时返回空切片（附录明确：salt/password 为空
// 则 S/P 也为空）。
func fillWithRepeats(pattern []byte, n int) []byte {
	if n <= 0 || len(pattern) == 0 {
		return nil
	}
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		out[i] = pattern[i%len(pattern)]
	}
	return out
}

// addIntoVBitBlocks 对 I 的每个 v 字节块做 I_j = (I_j + B + 1) mod 2^v。
//
// 定长字节加法（带进位），**不用 big.Int**：big.Int 会把前导零吃掉，
// 写回时块长不足 v，正是 x/crypto 曾经踩过的坑（其 pbkdf_test.go 的
// TestThatPBKDFHandlesLeadingZeros 就是为这个回归加的）。
// 这里始终原地修改 v 字节，长度天然守恒。
func addIntoVBitBlocks(I, B []byte, v int) {
	for off := 0; off+v <= len(I); off += v {
		carry := 1 // 步骤 6C 的 "+1"
		// 从最低位（块尾）向最高位加，B 与块等长
		for j := v - 1; j >= 0; j-- {
			sum := int(I[off+j]) + int(B[j]) + carry
			I[off+j] = byte(sum & 0xff)
			carry = sum >> 8
		}
		// 进位溢出丢弃 —— 等价于 mod 2^v
	}
}

// pkcs12PBKDF RFC 7292 附录 B.2 的通用派生过程。
//
// 参数：salt/password 为原始字节（password 需已按 B.1 转成 BMPString）；
// r 为迭代次数；ID 为用途（1=密钥材料，2=IV，3=MAC 密钥）；size 为需要的字节数。
func pkcs12PBKDF(p pkcs12KDFParams, salt, password []byte, r int, id byte, size int) []byte {
	if size <= 0 {
		return nil
	}
	if r < 1 {
		r = 1
	}
	u, v := p.u, p.v

	// 步骤 1：D = v/8 份 ID
	D := make([]byte, v)
	for i := range D {
		D[i] = id
	}
	// 步骤 2/3/4：S、P、I = S || P
	S := fillWithRepeats(salt, v*((len(salt)+v-1)/v))
	P := fillWithRepeats(password, v*((len(password)+v-1)/v))
	I := make([]byte, 0, len(S)+len(P))
	I = append(I, S...)
	I = append(I, P...)

	// 步骤 5：c = ceil(n/u)
	c := (size + u - 1) / u
	out := make([]byte, 0, c*u)

	hashBuf := make([]byte, 0, len(D)+len(I))
	for i := 0; i < c; i++ {
		// 步骤 6A：A_i = H^r(D || I)
		hashBuf = hashBuf[:0]
		hashBuf = append(hashBuf, D...)
		hashBuf = append(hashBuf, I...)
		h := p.newHash()
		h.Write(hashBuf)
		A := h.Sum(nil)
		for k := 1; k < r; k++ {
			h.Reset()
			h.Write(A)
			A = h.Sum(A[:0])
		}

		// 步骤 6B：B = 重复 A_i 到 v 字节
		B := fillWithRepeats(A, v)
		// 步骤 6C：按块累加
		addIntoVBitBlocks(I, B, v)

		out = append(out, A...)
	}

	// 步骤 8：取前 n 字节
	if len(out) < size {
		return out
	}
	return out[:size]
}

// bmpString 按 RFC 7292 B.1 把口令编码为 BMPString（UTF-16BE + 双字节 NULL 结尾）。
//
// 注意结尾是**两个** 0x00 字节（NULL 字符的 UTF-16BE 编码），不是一字节 ——
// 少写一个字节会导致所有派生结果都不对，而症状只是"解密失败"，很难反查。
func bmpString(s string) []byte {
	runes := utf16.Encode([]rune(s))
	out := make([]byte, 0, len(runes)*2+2)
	var tmp [2]byte
	for _, r := range runes {
		binary.BigEndian.PutUint16(tmp[:], r)
		out = append(out, tmp[0], tmp[1])
	}
	return append(out, 0x00, 0x00)
}

// pkcs12PBE3DESDecrypt 用 `pbeWithSHAAnd3-KeyTripleDES-CBC` 解一段密文。
//
// 密钥与 IV 都用同一套 KDF 派生（ID=1 取 24 字节密钥，ID=2 取 8 字节 IV）——
// 这是 PKCS#12 该算法的规定，与"用密钥派生 IV"的其他方案不同。
func pkcs12PBE3DESDecrypt(salt []byte, iterations int, password string, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) == 0 {
		return nil, errPKCS12
	}
	if len(ciphertext)%des.BlockSize != 0 {
		// CBC 要求整块；长度不对说明数据被截断，直接失败而不是硬解出垃圾
		return nil, fmt.Errorf("%w: 密文长度 %d 不是块大小 %d 的整数倍",
			errPKCS12, len(ciphertext), des.BlockSize)
	}
	pw := bmpString(password)
	key := pkcs12PBKDF(sha1KDFParams, salt, pw, iterations, 1, 24)
	iv := pkcs12PBKDF(sha1KDFParams, salt, pw, iterations, 2, des.BlockSize)

	block, err := des.NewTripleDESCipher(key)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, ciphertext)
	return pkcs12Unpad(out, des.BlockSize)
}

// pkcs12Unpad 去掉 PKCS#7 填充。
//
// blockSize 必须由调用方给出（3DES=8，AES=16）——**不能写死**。
// 这里曾经写死 des.BlockSize(8)，导致 AES-CBC 的填充值落在 9..16 时被判非法：
// 也就是明文长度 %16 落在 0..7 时解密必然失败（约占一半的块）。
// 症状是"同一段数据有的能解、有的解不开"，很容易误判成密钥问题。
//
// 校验填充的合法性并返回错误：填充不对说明口令错（或数据损坏），
// 此时返回"解出来的字节"只会让上层把垃圾当明文用。
func pkcs12Unpad(b []byte, blockSize int) ([]byte, error) {
	if len(b) == 0 || blockSize <= 0 || blockSize > 255 {
		return nil, errPKCS12
	}
	n := int(b[len(b)-1])
	if n == 0 || n > blockSize || n > len(b) {
		return nil, errPKCS12
	}
	for _, c := range b[len(b)-n:] {
		if int(c) != n {
			return nil, errPKCS12
		}
	}
	return b[:len(b)-n], nil
}
