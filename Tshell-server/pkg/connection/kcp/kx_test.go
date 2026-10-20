package kcp

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"BackendTemplate/pkg/encrypt"
)

var errBadFrame = &frameErr{"首帧不是 KX 帧"}

type frameErr struct{ s string }

func (e *frameErr) Error() string { return e.s }

// ===== KCP 的会话前 ECDH 交换 =====
//
// KCP 与 TCP 分帧完全一致（len(4)|msgType(4)|payload），所以同样用 msgType=200。
// 差别只在"结果怎么交回"：KCP 的 metaMasterKey 是所在函数的局部变量，
// 因此 handleKXFrame 直接返回派生密钥，由调用方赋值 —— 不需要连接级状态结构。

// pipeConn 用 net.Pipe 充当 net.Conn（kcp.UDPSession 在真实路径上满足同一接口）
func TestKCPKXFrameRoundTrip(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	salt := []byte("0123456789abcdef")
	cli, err := encrypt.NewServerKeyExchange() // 对称实现充当客户端
	if err != nil {
		t.Fatal(err)
	}

	// 注意：net.Pipe 是**无缓冲**的，服务端写回帧会一直阻塞到有人读。
	// 所以服务端必须跑在 goroutine 里，客户端在主 goroutine 读 ——
	// 反过来（服务端在主 goroutine、先写后读）会自我死锁。
	frame := make([]byte, 8+len(cli.HeaderValue()))
	body := []byte(cli.HeaderValue())
	binary.BigEndian.PutUint32(frame[0:4], uint32(4+len(body)))
	binary.BigEndian.PutUint32(frame[4:8], MsgTypeKX)
	copy(frame[8:], body)
	go func() { _, _ = clientConn.Write(frame) }()

	derivedCh := make(chan []byte, 1)
	srvErr := make(chan error, 1)
	go func() {
		var hdr [4]byte
		if _, err := readFull(serverConn, hdr[:]); err != nil {
			srvErr <- err
			return
		}
		total := binary.BigEndian.Uint32(hdr[:])
		rest := make([]byte, total)
		if _, err := readFull(serverConn, rest); err != nil {
			srvErr <- err
			return
		}
		if binary.BigEndian.Uint32(rest[0:4]) != MsgTypeKX {
			srvErr <- errBadFrame
			return
		}
		mk, err := handleKXFrame(serverConn, rest[4:], salt)
		if err != nil {
			srvErr <- err
			return
		}
		derivedCh <- mk
	}()

	// 客户端读回帧并复算
	var rhdr [4]byte
	if _, err := readFull(clientConn, rhdr[:]); err != nil {
		t.Fatal(err)
	}
	rtotal := binary.BigEndian.Uint32(rhdr[:])
	rbody := make([]byte, rtotal)
	if _, err := readFull(clientConn, rbody); err != nil {
		t.Fatal(err)
	}
	if binary.BigEndian.Uint32(rbody[0:4]) != MsgTypeKX {
		t.Fatal("回帧类型应为 MsgTypeKX")
	}
	serverPub, err := encrypt.ParsePeerPublicKey(string(rbody[4:]))
	if err != nil {
		t.Fatal(err)
	}
	cliMK, err := cli.DeriveMasterKey(serverPub, salt)
	if err != nil {
		t.Fatal(err)
	}
	var derived []byte
	select {
	case derived = <-derivedCh:
	case err := <-srvErr:
		t.Fatalf("服务端处理 KX 失败: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("等待服务端派生超时")
	}
	if len(derived) != encrypt.ECDHSharedKeySize {
		t.Fatalf("派生密钥长度异常: %d", len(derived))
	}
	for i := range cliMK {
		if cliMK[i] != derived[i] {
			t.Fatalf("**两侧主密钥不一致**（第 %d 字节）", i)
		}
	}
}

func readFull(conn net.Conn, buf []byte) (int, error) {
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

func TestKCPKXRejectsBadKey(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	go func() {
		buf := make([]byte, 64)
		_, _ = clientConn.Read(buf) // 排空，避免写阻塞
	}()

	done := make(chan error, 1)
	go func() {
		_, err := handleKXFrame(serverConn, []byte("@@bad@@"), []byte("0123456789abcdef"))
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Error("坏公钥必须返回错误")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("超时")
	}
}

// 常量必须与 tcp 一致（两端共用同一 msgType 语义）
func TestKCPMsgTypeMatchesTCP(t *testing.T) {
	if MsgTypeKX != 200 {
		t.Errorf("MsgTypeKX 必须为 200（与 connection/tcp 保持一致），实际 %d", MsgTypeKX)
	}
}
