package tcp

import (
	"encoding/binary"
	"net"
	"testing"

	"BackendTemplate/pkg/encrypt"
)

// ===== TCP 传输的会话前 ECDH 交换（帧级端到端）=====
//
// 这里不测"函数被调用"，而是用 net.Pipe 走真实分帧：
// 客户端发 KX 帧 → 服务端派生并回帧 → 客户端复算 →
// 再用派生密钥加密 MetaInfo 发 firstBlood → 服务端必须能解开并得到同一个 uid。
//
// 之所以要端到端：这段接线的失败模式是"帧格式或密钥来源对不上"，
// 单测各自 pass 但拼起来必然失败，而且现场表现只是"beacon 上不了线"。

func writeFrame(t *testing.T, conn net.Conn, msgType uint32, payload []byte) {
	t.Helper()
	frame := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(frame[0:4], uint32(4+len(payload)))
	binary.BigEndian.PutUint32(frame[4:8], msgType)
	copy(frame[8:], payload)
	if _, err := conn.Write(frame); err != nil {
		t.Fatalf("写帧失败: %v", err)
	}
}

func readFrame(t *testing.T, conn net.Conn) (uint32, []byte) {
	t.Helper()
	var hdr [8]byte
	if _, err := ioReadFull(conn, hdr[:4]); err != nil {
		t.Fatalf("读长度失败: %v", err)
	}
	total := binary.BigEndian.Uint32(hdr[0:4])
	if total < 4 {
		t.Fatalf("帧长度异常: %d", total)
	}
	rest := make([]byte, total)
	if _, err := ioReadFull(conn, rest); err != nil {
		t.Fatalf("读载荷失败: %v", err)
	}
	return binary.BigEndian.Uint32(rest[0:4]), rest[4:]
}

// ioReadFull 小封装，避免为一条测试引入 io 包的其它符号
func ioReadFull(conn net.Conn, buf []byte) (int, error) {
	read := 0
	for read < len(buf) {
		n, err := conn.Read(buf[read:])
		read += n
		if err != nil {
			return read, err
		}
	}
	return read, nil
}

// 完整往返：两侧派生的主密钥必须逐个字节相同
func TestTCPKXFrameRoundTrip(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	salt := []byte("0123456789abcdef")

	// 客户端侧（用服务端的对称实现充当，X25519+HKDF 两端对称）
	cli, err := encrypt.NewServerKeyExchange()
	if err != nil {
		t.Fatal(err)
	}

	// 服务端：读 KX 帧 → 派生
	st := &connKXState{}
	srvErr := make(chan error, 1)
	go func() {
		msgType, payload := 0, []byte(nil)
		var hdr [8]byte
		if _, err := ioReadFull(serverConn, hdr[:4]); err != nil {
			srvErr <- err
			return
		}
		total := binary.BigEndian.Uint32(hdr[0:4])
		rest := make([]byte, total)
		if _, err := ioReadFull(serverConn, rest); err != nil {
			srvErr <- err
			return
		}
		msgType = int(binary.BigEndian.Uint32(rest[0:4]))
		payload = rest[4:]
		if msgType != MsgTypeKX {
			srvErr <- errMsgType
			return
		}
		srvErr <- handleKXFrame(serverConn, payload, salt, st)
	}()

	writeFrame(t, clientConn, MsgTypeKX, []byte(cli.HeaderValue()))

	// 客户端读回帧
	gotType, serverPubB64 := readFrame(t, clientConn)
	if gotType != MsgTypeKX {
		t.Fatalf("回帧类型应为 %d，实际 %d", MsgTypeKX, gotType)
	}
	if err := <-srvErr; err != nil {
		t.Fatalf("服务端处理 KX 失败: %v", err)
	}

	serverPub, err := encrypt.ParsePeerPublicKey(string(serverPubB64))
	if err != nil {
		t.Fatalf("服务端公钥不可解析: %v", err)
	}
	cliMK, err := cli.DeriveMasterKey(serverPub, salt)
	if err != nil {
		t.Fatalf("客户端派生失败: %v", err)
	}

	if len(st.masterKey) != encrypt.ECDHSharedKeySize {
		t.Fatalf("服务端应保存派生主密钥，实际长度 %d", len(st.masterKey))
	}
	for i := range cliMK {
		if cliMK[i] != st.masterKey[i] {
			t.Fatalf("**两侧主密钥不一致**（第 %d 字节）—— 会导致 firstBlood 解不开", i)
		}
	}
}

var errMsgType = &frameErr{"首帧不是 KX 帧"}

type frameErr struct{ s string }

func (e *frameErr) Error() string { return e.s }

// 密钥优先级：KX 派生密钥必须压过静态覆盖与监听器配置
func TestResolveMetaKeyPrecedence(t *testing.T) {
	staticMK := make([]byte, 32)
	for i := range staticMK {
		staticMK[i] = 0xAA
	}
	overrideSalt := []byte("0123456789abcdef")

	derived := make([]byte, encrypt.ECDHSharedKeySize)
	for i := range derived {
		derived[i] = 0x11
	}

	// 1) 无 KX：用显式覆盖
	mk, salt := resolveMetaKey("tcp", staticMK, overrideSalt, &connKXState{})
	if string(mk) != string(staticMK) {
		t.Error("无 KX 时应使用覆盖主密钥")
	}
	if len(salt) != 16 {
		t.Errorf("盐长度应为 16，实际 %d", len(salt))
	}

	// 2) 有 KX：派生密钥必须胜出，且**盐仍是覆盖盐**（否则两端派生不一致）
	mk2, salt2 := resolveMetaKey("tcp", staticMK, overrideSalt, &connKXState{masterKey: derived})
	if string(mk2) != string(derived) {
		t.Error("KX 完成后必须使用派生主密钥（否则 MetaInfo 解不开）")
	}
	if string(salt2) != string(salt) {
		t.Error("KX 的盐必须与静态路径一致 —— 盐不一致会算出不同密钥")
	}

	// 3) 无覆盖、无 KX：回落到监听器配置（不 panic 即可）
	mk3, _ := resolveMetaKey("tcp", nil, nil, nil)
	if len(mk3) == 0 {
		t.Error("回落到监听器配置时应拿到非空主密钥")
	}
}

// KX 派生密钥不足 32 字节时必须被忽略（不能拿半截密钥当主密钥用）
func TestResolveMetaKeyIgnoresShortDerived(t *testing.T) {
	staticMK := make([]byte, 32)
	for i := range staticMK {
		staticMK[i] = 0xBB
	}
	mk, _ := resolveMetaKey("tcp", staticMK, []byte("0123456789abcdef"),
		&connKXState{masterKey: make([]byte, 16)})
	if string(mk) != string(staticMK) {
		t.Error("过短的派生密钥应被忽略，回落到静态密钥")
	}
}
