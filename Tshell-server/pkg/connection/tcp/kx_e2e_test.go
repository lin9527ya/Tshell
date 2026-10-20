package tcp

import (
	"bufio"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/hkdf"

	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/encrypt"
)

// ===== 真实协议端到端：KX → firstBlood → 会话密钥必须两侧一致 =====
//
// 这是 2026-10-20 线上故障的直接守卫。当时的 bug：
//   MetaInfo 用 KX 派生密钥解密（能解开 → 客户端正常上线），
//   但会话密钥是用 **静态监听器密钥** 派生的（firstBlood 分支里又取了一次），
//   于是客户端用派生密钥加密、服务端用静态密钥解密 → 命令/结果全部失败。
//
// 现象极具迷惑性：**能上线，但所有功能无效**。
// 所以断言必须是"服务端为该 uid 存的会话密钥 == 客户端按派生主密钥算出的"，
// 而不是"客户端注册成功了" —— 后者在 bug 存在时同样会通过。

func testKeys() (mk, salt []byte) {
	mk = make([]byte, 32)
	for i := range mk {
		mk[i] = byte(0x10 + i)
	}
	salt = make([]byte, 16)
	for i := range salt {
		salt[i] = byte(0x40 + i)
	}
	return mk, salt
}

func setupDB(t *testing.T) {
	t.Helper()
	old := database.DatabaseName
	database.DatabaseName = filepath.Join(t.TempDir(), "kx_e2e.db")
	database.ConnectDateBase()
	database.DatabaseName = old
	t.Cleanup(func() {
		if database.Engine != nil {
			database.Engine.Close()
		}
	})
}

// buildMetaInfo 按客户端格式造 MetaInfo 明文：
// pid(4) | flag(1) | ipLE(4) | "host\tuser\tproc\tos"
func buildMetaInfo(host string) []byte {
	out := make([]byte, 9)
	binary.BigEndian.PutUint32(out[0:4], 4242)
	out[4] = 1
	binary.LittleEndian.PutUint32(out[5:9], 0x0100007F) // 127.0.0.1
	return append(out, []byte(host+"\tuser\tproc\ttestos")...)
}

// clientSessionKey 复刻客户端公式：HKDF-SHA256(masterKey, salt, info+clientID)。
// info 必须取 encrypt.SessionKeyInfoProvider()（main 会从配置注入），
// 否则在这个测试里两边都用默认值也可能掩盖差异。
func clientSessionKey(t *testing.T, masterKey, salt []byte, clientID string) []byte {
	t.Helper()
	r := hkdf.New(sha256.New, masterKey, salt, []byte(encrypt.SessionKeyInfoProvider()+clientID))
	out := make([]byte, 32)
	if _, err := io.ReadFull(r, out); err != nil {
		t.Fatalf("客户端侧派生会话密钥失败: %v", err)
	}
	return out
}

func frame(msgType uint32, payload []byte) []byte {
	body := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(body[:4], msgType)
	copy(body[4:], payload)
	out := make([]byte, 4+len(body))
	binary.BigEndian.PutUint32(out[:4], uint32(len(body)))
	copy(out[4:], body)
	return out
}

func TestTCPKXSessionKeyMatchesClient(t *testing.T) {
	setupDB(t)
	if err := encrypt.InitSecureKeyManager(); err != nil {
		t.Fatalf("初始化密钥管理器失败: %v", err)
	}
	masterKey, salt := testKeys()

	srvConn, cliConn := net.Pipe()
	defer cliConn.Close()

	go HandleTcpConnectionForWithKeys(srvConn, "tcp", masterKey, salt)

	// ---- 客户端：临时密钥对 + 发送 KX 帧 ----
	curve := ecdh.X25519()
	priv, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cliConn.Write(frame(MsgTypeKX,
		[]byte(base64.StdEncoding.EncodeToString(priv.PublicKey().Bytes())))); err != nil {
		t.Fatalf("发送 KX 帧失败: %v", err)
	}

	// ---- 读服务端 KX 应答并派生主密钥 ----
	rd := bufio.NewReader(cliConn)
	_ = cliConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(rd, lenBuf); err != nil {
		t.Fatalf("读 KX 应答长度失败: %v", err)
	}
	total := int(binary.BigEndian.Uint32(lenBuf))
	if total < 4 {
		t.Fatalf("KX 应答长度异常: %d", total)
	}
	body := make([]byte, total)
	if _, err := io.ReadFull(rd, body); err != nil {
		t.Fatalf("读 KX 应答失败: %v", err)
	}
	if got := binary.BigEndian.Uint32(body[:4]); got != MsgTypeKX {
		t.Fatalf("应答帧类型应为 %d，实际 %d", MsgTypeKX, got)
	}
	serverPubRaw, err := base64.StdEncoding.DecodeString(string(body[4:]))
	if err != nil {
		t.Fatalf("服务端公钥不是合法 base64: %v", err)
	}
	serverPub, err := curve.NewPublicKey(serverPubRaw)
	if err != nil {
		t.Fatalf("服务端公钥非法: %v", err)
	}
	shared, err := priv.ECDH(serverPub)
	if err != nil {
		t.Fatalf("ECDH 失败: %v", err)
	}
	derivedMK := make([]byte, 32)
	if _, err := io.ReadFull(hkdf.New(sha256.New, shared, salt, []byte(encrypt.ECDHKeyInfo)), derivedMK); err != nil {
		t.Fatalf("派生主密钥失败: %v", err)
	}

	// ---- 用**派生**主密钥加密 MetaInfo，发 firstBlood ----
	plain := buildMetaInfo("kx-e2e-host")
	cipherText, err := encrypt.EncryptWithMasterKeyRaw(plain, derivedMK)
	if err != nil {
		t.Fatalf("加密 MetaInfo 失败: %v", err)
	}
	if _, err := cliConn.Write(frame(1,
		[]byte(base64.StdEncoding.EncodeToString(cipherText)))); err != nil {
		t.Fatalf("发送 firstBlood 失败: %v", err)
	}

	uid := encrypt.BytesToMD5(plain)
	wantKey := clientSessionKey(t, derivedMK, salt, uid)

	// ---- 判定：能否用客户端自算的会话密钥解开服务端下发的 nonce 消息 ----
	//
	// 为什么不直接比对 GlobalKeyManager.GetSessionKey(uid)：
	// firstBlood 派生完会话密钥后会**立即用 nonce 轮换**（type=4 下发），
	// 轮换后的密钥与 initialKey 不同。轮询 GetSessionKey 会与轮换竞争，
	// 结果时好时坏 —— 那种测试比没有更糟。
	//
	// 而 type=4 消息是用 **initialKey** 加密的（客户端此刻仍是 initialKey），
	// 所以"能用客户端自算的 key 解开它"就是密钥一致的确定性证据。
	_ = uid
	_ = cliConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	frameLen := make([]byte, 4)
	if _, err := io.ReadFull(rd, frameLen); err != nil {
		t.Fatalf("读 nonce 帧长度失败（firstBlood 未被处理？）: %v", err)
	}
	n := int(binary.BigEndian.Uint32(frameLen))
	if n <= 0 || n > 8192 {
		t.Fatalf("nonce 帧长度异常: %d", n)
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(rd, payload); err != nil {
		t.Fatalf("读 nonce 帧失败: %v", err)
	}
	nonceMsg, err := base64.StdEncoding.DecodeString(string(payload))
	if err != nil {
		t.Fatalf("nonce 帧不是合法 base64: %v", err)
	}
	decrypted, err := encrypt.SecureDecrypt(nonceMsg, wantKey)
	if err != nil {
		serverKey, _ := encrypt.GlobalKeyManager.GetSessionKey(uid)
		t.Fatalf("用客户端自算的会话密钥解不开服务端的 nonce 消息: %v\n"+
			"  客户端 key = %x\n  服务端 key = %x\n"+
			"这正是「能上线但所有功能失效」的根因 —— firstBlood 必须复用 "+
			"resolveMetaKey 解析出的派生主密钥派生会话密钥，不能重新取静态监听器密钥",
			err, wantKey, serverKey)
	}
	if len(decrypted) < 4 || binary.BigEndian.Uint32(decrypted[:4]) != 4 {
		t.Fatalf("解出的消息类型不是 4（nonce 协商）: %x", decrypted)
	}
}
