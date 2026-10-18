package services

import (
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// ===== 用**真实 Firefox 产物**验证 key4.db + logins.json 全链路 =====
//
// 向量来源：unode/firefox_decrypt 测试套件里的真实 Firefox 配置
// （test_profile_firefox_nopassword_114 / _59），其真值由该套件用 libnss 产出，
// 期望明文取自同套件的 exported_passwords.csv。
//
// 为什么这些字节是可信的外部证据、而不是自造数据：
// 它们是 Firefox 自己写出来的密文与盐。只要我的实现能把它们解成
// csv 里那些已知明文，就说明整条链路（ASN.1 解析 → KDF → CBC → 去填充 →
// a11 主密钥 → 条目解密）与真实 Firefox/NSS 一致。
// 自造夹具做不到这一点：那只能证明"我的加密与我的解密自洽"。
//
// 许可证说明：那套夹具是 GPL-3，**未**随本仓库分发。这里只嵌入从其中抽出的
// 数据字节（盐/密文/期望明文），不含对方任何代码。

// 真实夹具：nopassword_114（无主口令；key4.db 走 PBES2，logins.json 走 AES-256）
const (
	ff114GlobalSalt = "33529692c145bd97095ff23d508a4ab0f66ca062"
	ff114Item2      = "308181306d06092a864886f70d01050d3060304106092a864886f70d01050c30340420b1a87d7e8ad593408152006e8f396694778bdff993bdd3fad26a92d04919b367020101020120300a06082a864886f70d0209301b060960864801650304012a040eb5af06fc6e3a1e7aedcefdee73c4041054c508f57b32f848df901dd4d919d739"
	ff114A11        = "308191306d06092a864886f70d01050d3060304106092a864886f70d01050c303404202a6761cc416db5b01824e7a5f43e98bbe671ad4162c43bc728cec3879eafc021020101020120300a06082a864886f70d0209301b060960864801650304012a040e185b47a464220967f9cc18c4ebd104201b6bdba2d104a108ba655bd4dea41d390d0898881553fa496f0f125dccc27eee"
	ff114User0      = "MDIEEPgAAAAAAAAAAAAAAAAAAAEwFAYIKoZIhvcNAwcECEgmzKOCavAWBAiab7yWG12/Rw=="
	ff114Pass0      = "MDIEEPgAAAAAAAAAAAAAAAAAAAEwFAYIKoZIhvcNAwcECIm+i6iNZ4okBAjDGi5OTiNc9Q=="
	ff114User1      = "MDoEEPgAAAAAAAAAAAAAAAAAAAEwFAYIKoZIhvcNAwcECMmE70ZVP0E4BBCJNgP9zigxQG7wA5Xtf6J/"
	ff114Pass1      = "MDoEEPgAAAAAAAAAAAAAAAAAAAEwFAYIKoZIhvcNAwcECAaKjRlvw62zBBDn4tthVPtEKCqkeOeUQZaM"
	ff114User2      = "MDIEEPgAAAAAAAAAAAAAAAAAAAEwFAYIKoZIhvcNAwcECCb0mK51cDIgBAjToSpG3VVLnQ=="
	ff114Pass2      = "MDoEEPgAAAAAAAAAAAAAAAAAAAEwFAYIKoZIhvcNAwcECB3Oul6W4qDQBBB917xhQ/IHXUnctPwKU6vY"
	ff114User3      = "MDoEEPgAAAAAAAAAAAAAAAAAAAEwFAYIKoZIhvcNAwcECPWy/Wb/zVxsBBCHZJUabIuDWSSPh7IvF4M/"
	ff114Pass3      = "MFIEEPgAAAAAAAAAAAAAAAAAAAEwFAYIKoZIhvcNAwcECBWgpjB1kLSYBCj+3VaCZAPhdWXO40qlQTBj7wzjrAVOCGnug8366ez2dmn4/TpGPxyi"
	ff114User4      = "MDIEEPgAAAAAAAAAAAAAAAAAAAEwFAYIKoZIhvcNAwcECOcZUkTRCDK5BAjDFjezUYVSFg=="
	ff114Pass4      = "MFoEEPgAAAAAAAAAAAAAAAAAAAEwFAYIKoZIhvcNAwcECM8rG1/2s/7uBDDLU1D80YJO5o2tkjh/WaXUbhOWNgLiCvPaaOFQfaI4/iCaS70SD4UrEXJSBm7ph7o="
)

// 真实夹具：nopassword_59（无主口令；key4.db 走 PBES2，logins.json 走 3DES）
const (
	ff59GlobalSalt = "d152c0f4b0c234ce2aebb120c613a7e2d73c91af"
	ff59Item2      = "308181306d06092a864886f70d01050d3060304106092a864886f70d01050c30340420309945829d362154a2337eb4419aa2c310ed079e3a6120caac4227feccb04c91020101020120300a06082a864886f70d0209301b060960864801650304012a040e2502189680c7a25aeb565cf328dd0410ef841d087da52e68b768fab33f1b97cb"
	ff59A11        = "308191306d06092a864886f70d01050d3060304106092a864886f70d01050c303404209102e6348e9b3b311e1babd52ec5953b85c81539e18aba5ee1e748a09685c125020101020120300a06082a864886f70d0209301b060960864801650304012a040ee89167761e2540e4412ed77ee03e042095da190377ba674b9062c2dd4a2dbf1f580a1d5894f6d77e3d430273951d5c5f"
	ff59User0      = "MDoEEPgAAAAAAAAAAAAAAAAAAAEwFAYIKoZIhvcNAwcECDAMJYvxVWmNBBAYOR+4wZeLSB7kqJ/GDhj3"
	ff59Pass0      = "MDoEEPgAAAAAAAAAAAAAAAAAAAEwFAYIKoZIhvcNAwcECBQ0N0EftdcPBBD9CaBvRSe9MhhqBjbd3UG8"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("测试向量 hex 非法: %v", err)
	}
	return b
}

// 口令校验：无主口令的配置，item2 应解出固定的 password-check 串
func TestFirefoxRealItem2PasswordCheck(t *testing.T) {
	for _, tc := range []struct {
		name string
		salt string
		item string
	}{
		{"firefox_nopassword_114", ff114GlobalSalt, ff114Item2},
		{"firefox_nopassword_59", ff59GlobalSalt, ff59Item2},
	} {
		blob, err := parsePBEBlob(mustHex(t, tc.item))
		if err != nil {
			t.Fatalf("%s: 解析 PBE 容器失败: %v", tc.name, err)
		}
		if blob.legacy {
			t.Fatalf("%s: 期望走 PBES2 路径，实际被判定为 legacy", tc.name)
		}
		if blob.keyLength != 32 {
			t.Errorf("%s: 密钥长度应为 32，实际 %d", tc.name, blob.keyLength)
		}
		if len(blob.entrySalt) != 32 {
			t.Errorf("%s: PBES2 salt 应为 32 字节，实际 %d", tc.name, len(blob.entrySalt))
		}
		if len(blob.iv) != 16 {
			t.Errorf("%s: IV 应被补成 16 字节，实际 %d", tc.name, len(blob.iv))
		}
		// NSS 的 IV 怪癖：前两字节必须是 DER 的 04 0e
		if blob.iv[0] != 0x04 || blob.iv[1] != 0x0e {
			t.Errorf("%s: IV 前两字节应为 04 0e（NSS 怪癖），实际 %02x %02x",
				tc.name, blob.iv[0], blob.iv[1])
		}

		clear, err := nssDecryptPBEBlob(blob, mustHex(t, tc.salt), nil)
		if err != nil {
			t.Fatalf("%s: 解密 item2 失败: %v", tc.name, err)
		}
		if string(clear) != string(passwordCheck) {
			t.Fatalf("%s: item2 明文不是 password-check\n期望 %q\n实际 %q",
				tc.name, passwordCheck, clear)
		}
	}
}

// 错口令必须被 password-check 挡住（而不是解出乱码凭据）
func TestFirefoxRealWrongPasswordRejected(t *testing.T) {
	blob, err := parsePBEBlob(mustHex(t, ff114Item2))
	if err != nil {
		t.Fatal(err)
	}
	clear, err := nssDecryptPBEBlob(blob, mustHex(t, ff114GlobalSalt), []byte("wrong-master"))
	if err == nil && string(clear) == string(passwordCheck) {
		t.Fatal("错口令竟然通过了 password-check 校验")
	}
}

// a11 解出的主密钥：长度与形状
func TestFirefoxRealMasterKeyShape(t *testing.T) {
	for _, tc := range []struct {
		name string
		salt string
		a11  string
	}{
		{"firefox_nopassword_114", ff114GlobalSalt, ff114A11},
		{"firefox_nopassword_59", ff59GlobalSalt, ff59A11},
	} {
		blob, err := parsePBEBlob(mustHex(t, tc.a11))
		if err != nil {
			t.Fatalf("%s: 解析 a11 失败: %v", tc.name, err)
		}
		clear, err := nssDecryptPBEBlob(blob, mustHex(t, tc.salt), nil)
		if err != nil {
			t.Fatalf("%s: 解密 a11 失败: %v", tc.name, err)
		}
		// 实测：a11 的明文**就是裸主密钥**（24 字节），并非 SEQUENCE{OCTETSTRING}。
		// unwrapOctetString 仅作为某些版本的兜底，这里允许两种形态。
		key := clear
		if unwrapped, ok := unwrapOctetString(clear); ok {
			key = unwrapped
		}
		if len(key) != 24 && len(key) != 48 {
			t.Errorf("%s: 主密钥长度应为 24 或 48，实际 %d", tc.name, len(key))
		}
		t.Logf("%s: 主密钥 %d 字节", tc.name, len(key))
	}
}

// 端到端：用真实字节解出 exported_passwords.csv 里的已知明文
func TestFirefoxRealLoginsDecrypt(t *testing.T) {
	// —— firefox_nopassword_114 ——
	// 实测其 logins.json 条目声明的 OID 是 des-ede3-cbc（不是 AES），
	// 与 24 字节主密钥一致。算法由条目里的 OID 决定，代码自动选择。
	//
	// 注意：条目的**下标与 exported_passwords.csv 的行序并不对应**。
	// 下面这组 index→明文 映射是逐条实测得到的（不是按顺序猜的）：
	//   login[1] doesntexist / xrbSDzYf94gfk
	//   login[2] onemore     / }]¢öðæ[{
	//   login[3] cömplex     / сЮЛОажс$4vz*VçàhxpfCbmwo
	//   login[4] jãmïe       / Apassword\twithtabs,;colonandsemi'"andquotes
	//   login[0] 是夹具里**故意损坏**的一条（用户名密文填充非法），
	//            与参考套件的 handle_corrupted_passwords.t 对应，单独断言。
	key114 := masterKeyFromVectors(t, ff114GlobalSalt, ff114A11)
	for _, tc := range []struct{ name, user, pass, wantUser, wantPass string }{
		{"login[1]", ff114User1, ff114Pass1, "doesntexist", "xrbSDzYf94gfk"},
		{"login[2]", ff114User2, ff114Pass2, "onemore", "}]¢öðæ[{"},
		{"login[3]", ff114User3, ff114Pass3, "cömplex", "сЮЛОажс$4vz*VçàhxpfCbmwo"},
		// login[4] 的明文含制表符、逗号、分号、单引号、双引号 —— 引号/转义最容易出错
		{"login[4]", ff114User4, ff114Pass4, "jãmïe", "Apassword\twithtabs,;colonandsemi'\"andquotes"},
	} {
		gotUser, err := nssDecryptLoginField(key114, tc.user)
		if err != nil {
			t.Fatalf("%s: 解密用户名失败: %v", tc.name, err)
		}
		gotPass, err := nssDecryptLoginField(key114, tc.pass)
		if err != nil {
			t.Fatalf("%s: 解密口令失败: %v", tc.name, err)
		}
		if gotUser != tc.wantUser {
			t.Errorf("%s: 用户名不符\n期望 %q\n实际 %q", tc.name, tc.wantUser, gotUser)
		}
		if gotPass != tc.wantPass {
			t.Errorf("%s: 口令不符\n期望 %q\n实际 %q", tc.name, tc.wantPass, gotPass)
		}
	}

	// —— firefox_nopassword_59：同样是 3DES，用来确认两条 profile 走同一路径 ——
	key59 := masterKeyFromVectors(t, ff59GlobalSalt, ff59A11)
	gotUser, err := nssDecryptLoginField(key59, ff59User0)
	if err != nil {
		t.Fatalf("59: 解密用户名失败: %v", err)
	}
	gotPass, err := nssDecryptLoginField(key59, ff59Pass0)
	if err != nil {
		t.Fatalf("59: 解密口令失败: %v", err)
	}
	if gotUser != "doesntexist" || gotPass != "xrbSDzYf94gfk" {
		t.Errorf("59: 凭据不符\n期望 doesntexist / xrbSDzYf94gfk\n实际 %q / %q", gotUser, gotPass)
	}
}

// masterKeyFromVectors 用真实向量走一遍 a11 → 主密钥
func masterKeyFromVectors(t *testing.T, saltHex, a11Hex string) []byte {
	t.Helper()
	blob, err := parsePBEBlob(mustHex(t, a11Hex))
	if err != nil {
		t.Fatal(err)
	}
	clear, err := nssDecryptPBEBlob(blob, mustHex(t, saltHex), nil)
	if err != nil {
		t.Fatal(err)
	}
	// a11 明文实测为裸主密钥；unwrap 仅作兜底
	if unwrapped, ok := unwrapOctetString(clear); ok {
		return unwrapped
	}
	return clear
}

// ===== 可选：本机装了 Firefox 时直接跑真机 profile =====
//
// 与 core/sqlite 的 real_chrome_test.go 同一思路：有真机数据时跑最有说服力，
// 没有就跳过，不影响 CI。
func TestFirefoxRealProfileIfPresent(t *testing.T) {
	dirs := nssFirefoxProfileDirs()
	if len(dirs) == 0 {
		t.Skip("本机没有 Firefox profile，跳过")
	}
	tried := 0
	for _, d := range dirs {
		if !nssLooksLikeFirefoxProfile(d) {
			continue
		}
		// 无主口令的 profile 才能直接解；有主口令的会返回 errNSSPasswordBad
		logins, err := nssExtractFirefox(d, nil)
		if err != nil {
			continue
		}
		tried++
		t.Logf("%s: 解出 %d 条凭据", filepath.Base(d), len(logins))
		for _, l := range logins {
			if l.Username == "" && l.Password == "" {
				t.Errorf("%s: 解出空凭据（不应出现）", d)
			}
		}
	}
	if tried == 0 {
		t.Skip("本机 Firefox profile 都设了主口令或不可读，跳过")
	}
}

// 真实文件可用时（通过环境变量指定）跑完整目录级链路。
// CI 不设该变量；本地验证时指向真实 profile 即可。
func TestFirefoxRealProfileDirEnv(t *testing.T) {
	dir := os.Getenv("TSHELL_FIREFOX_PROFILE")
	if dir == "" {
		t.Skip("未设置 TSHELL_FIREFOX_PROFILE，跳过")
	}
	logins, err := nssExtractFirefox(dir, nil)
	if err != nil {
		t.Fatalf("提取失败: %v", err)
	}
	if len(logins) == 0 {
		t.Fatal("未解出任何凭据")
	}
	for _, l := range logins {
		t.Logf("%s | %s | %s", l.URL, l.Username, l.Password)
	}
}

// base64 向量本身要能解码（防止抄错）
func TestFirefoxVectorsAreWellFormed(t *testing.T) {
	for _, v := range []string{ff114User0, ff114Pass0, ff59User0, ff59Pass0} {
		if _, err := base64.StdEncoding.DecodeString(v); err != nil {
			t.Errorf("向量不是合法 base64: %s", v)
		}
	}
}

// 夹具里故意损坏的一条：用户名密文填充非法，但口令能正常解出。
//
// 这条很重要 —— 真实 profile 里确实有这类条目（参考套件专门有
// handle_corrupted_passwords.t）。凭据工具**不能静默丢条目**：
// 丢了就等于丢证据。正确做法是保留条目并标出哪一半没解出来。
func TestFirefoxRealCorruptedEntryIsSurfaced(t *testing.T) {
	key := masterKeyFromVectors(t, ff114GlobalSalt, ff114A11)

	// 用户名解不开
	if _, err := nssDecryptLoginField(key, ff114User0); err == nil {
		t.Error("这条的密文填充是坏的，用户名解密应当失败")
	}
	// 但口令能解开 —— 所以整条不该被丢弃
	pass, err := nssDecryptLoginField(key, ff114Pass0)
	if err != nil {
		t.Fatalf("口令本应能解开: %v", err)
	}
	if pass != "john" {
		t.Errorf("口令应为 john，实际 %q", pass)
	}
}

// nssReadLogins 必须把解不开的字段**标出来**而不是丢掉整条
func TestFirefoxReadLoginsSurfacesPartialFailures(t *testing.T) {
	dir := t.TempDir()
	// 用真实向量拼一个 logins.json：一条完整、一条用户名损坏
	jsonDoc := `{"logins":[` +
		`{"hostname":"https://ok.example","encryptedUsername":"` + ff114User1 + `","encryptedPassword":"` + ff114Pass1 + `"},` +
		`{"hostname":"https://broken.example","encryptedUsername":"` + ff114User0 + `","encryptedPassword":"` + ff114Pass0 + `"}` +
		`]}`
	if err := os.WriteFile(filepath.Join(dir, "logins.json"), []byte(jsonDoc), 0o600); err != nil {
		t.Fatal(err)
	}
	key := masterKeyFromVectors(t, ff114GlobalSalt, ff114A11)
	logins, err := nssReadLogins(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	if len(logins) != 2 {
		t.Fatalf("应返回 2 条（损坏的那条也要保留），实际 %d", len(logins))
	}
	if !logins[0].Complete() {
		t.Errorf("第一条应完整解出，实际 userErr=%q passErr=%q",
			logins[0].UsernameError, logins[0].PasswordError)
	}
	if logins[1].Complete() {
		t.Error("第二条的用户名是坏的，不该报告为完整")
	}
	if logins[1].UsernameError == "" {
		t.Error("第二条应记录用户名解密失败原因")
	}
	if logins[1].Password != "john" {
		t.Errorf("第二条的口令仍应解出，实际 %q", logins[1].Password)
	}
}
