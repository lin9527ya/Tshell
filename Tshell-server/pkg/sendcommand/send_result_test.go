package sendcommand

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/connection"
	"BackendTemplate/pkg/connection/tcp"
	"BackendTemplate/pkg/encrypt"
)

func setListenerTypeForTest(uid, listenerType string) {
	connection.MuClientListenerType.Lock()
	connection.ClientListenerType[uid] = listenerType
	connection.MuClientListenerType.Unlock()
}

func clearListenerTypeForTest(uid string) {
	connection.MuClientListenerType.Lock()
	delete(connection.ClientListenerType, uid)
	connection.MuClientListenerType.Unlock()
}

// TestSendFileUploadCommandWithResultPersistent 持久连接类型必须真的尝试发送。
// 原实现对 tcp/kcp/ws/oss 只检查"连接是否存在"就返回 true，注释称"实际发送由
// SendInjectMem 的循环保证"，而那个循环里唯一的动作就是调用本函数 ——
// 于是内存注入的分块全部丢失、调用方却拿到成功。
func TestSendFileUploadCommandWithResultPersistent(t *testing.T) {
	if SendFileUploadCommandWithResult("no-such-uid-at-all", []byte("x")) {
		t.Error("未知 uid 应返回 false")
	}

	uid := "tcp-uid-without-connection"
	setListenerTypeForTest(uid, "tcp")
	defer clearListenerTypeForTest(uid)

	if SendFileUploadCommandWithResult(uid, []byte("chunk")) {
		t.Error("tcp 类型在没有连接时返回 true（分块会静默丢失）")
	}
	err := SendFileUploadCommandChecked(uid, []byte("chunk"))
	if err == nil {
		t.Fatal("Checked 未报告失败原因")
	}
	t.Logf("失败原因: %v", err)
}

// TestSendFileUploadCommandCheckedPolling 轮询类型入队，且如实回报结果
func TestSendFileUploadCommandCheckedPolling(t *testing.T) {
	uid := "http-uid-for-queue-test"
	setListenerTypeForTest(uid, "http")
	defer clearListenerTypeForTest(uid)

	payload := []byte("hello-chunk")
	if err := SendFileUploadCommandChecked(uid, payload); err != nil {
		t.Fatalf("轮询类型入队失败: %v", err)
	}
	got, ok := command.CommandQueues.GetCommand(uid)
	if !ok {
		t.Fatal("命令未入队")
	}
	if string(got) != string(payload) {
		t.Fatalf("入队内容不符: %q", got)
	}

	if !SendFileUploadCommandWithResult(uid, []byte("second")) {
		t.Error("轮询类型入队成功时 WithResult 应返回 true")
	}
}

// TestSendFileUploadCommandCheckedWritesToPersistentConn 真实发送验证：
// 把一个 net.Pipe 注册成 tcp 连接，确认载荷真的写到了连接上（而不是像修复前那样
// 只做存在性检查就返回 true）。顺带用同一会话密钥解回来核对内容。
func TestSendFileUploadCommandCheckedWritesToPersistentConn(t *testing.T) {
	if encrypt.GlobalKeyManager == nil {
		if err := encrypt.InitSecureKeyManager(); err != nil {
			t.Skipf("密钥管理器不可用: %v", err)
		}
	}

	uid := "tcp-uid-with-pipe"
	setListenerTypeForTest(uid, "tcp")
	server, peer := net.Pipe()
	tcp.Mutex.Lock()
	tcp.TCPClientManger[uid] = server
	tcp.Mutex.Unlock()
	defer func() {
		server.Close()
		peer.Close()
		tcp.Mutex.Lock()
		delete(tcp.TCPClientManger, uid)
		tcp.Mutex.Unlock()
		clearListenerTypeForTest(uid)
	}()

	payload := []byte("inject-chunk-payload")
	errCh := make(chan error, 1)
	go func() { errCh <- SendFileUploadCommandChecked(uid, payload) }()

	// net.Pipe 没有超时：一旦载荷没真正发出，读取会永久阻塞而不是让用例失败
	if err := peer.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}

	// 协议帧：[4B 大端长度][base64(密文)]
	var lenBuf [4]byte
	if _, err := io.ReadFull(peer, lenBuf[:]); err != nil {
		t.Fatalf("连接上读不到帧长度（载荷没有真正发出）: %v", err)
	}
	frameLen := binary.BigEndian.Uint32(lenBuf[:])
	if frameLen == 0 || frameLen > 1<<20 {
		t.Fatalf("帧长度异常: %d", frameLen)
	}
	frame := make([]byte, frameLen)
	if _, err := io.ReadFull(peer, frame); err != nil {
		t.Fatalf("读取帧内容失败: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("发送返回错误: %v", err)
	}

	// 解回原文，确认内容一致（也验证了分块走的是正常加密通道）
	cipherBytes, err := encrypt.DecodeBase64(frame)
	if err != nil {
		t.Fatalf("帧内容不是 base64: %v", err)
	}
	plain, err := encrypt.DecryptWithSessionKey(cipherBytes, uid)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	if string(plain) != string(payload) {
		t.Fatalf("解密内容不符: %q", plain)
	}
}
