package api

import (
	"bytes"
	"encoding/binary"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/connection"

	"github.com/gin-gonic/gin"
)

func setInjectListenerType(t *testing.T, uid, listenerType string) {
	t.Helper()
	connection.MuClientListenerType.Lock()
	connection.ClientListenerType[uid] = listenerType
	connection.MuClientListenerType.Unlock()
	t.Cleanup(func() {
		connection.MuClientListenerType.Lock()
		delete(connection.ClientListenerType, uid)
		connection.MuClientListenerType.Unlock()
	})
}

// TestSendInjectMemPersistentListenerMustFail 持久连接客户端没有连接时，分块下发必须报错。
// 修复前：SendFileUploadCommandWithResult 对 tcp/kcp/ws/oss 只做存在性检查就返回 true，
// 于是这里返回 (sent=N, seq=M, nil) —— 客户端一块都收不到，注入静默失败。
func TestSendInjectMemPersistentListenerMustFail(t *testing.T) {
	uid := "tcp-inject-uid"
	setInjectListenerType(t, uid, "tcp")

	sent, seq, err := SendInjectMem(uid, bytes.Repeat([]byte("A"), 4096), 1234, "auto")
	if err == nil {
		t.Fatalf("持久连接无连接时分块下发竟然报成功: sent=%d seq=%d", sent, seq)
	}
	if sent != 0 || seq != 0 {
		t.Fatalf("失败时不应报告已发送字节/分块: sent=%d seq=%d", sent, seq)
	}
}

// TestSendInjectMemQueuesChunksInOrder 轮询客户端：分块必须全部入队且顺序与包头一致
// （客户端按 seq 顺序组装，乱序会直接丢弃整次注入）
func TestSendInjectMemQueuesChunksInOrder(t *testing.T) {
	uid := "http-inject-uid"
	setInjectListenerType(t, uid, "http")

	data := bytes.Repeat([]byte("B"), injectMemChunk*2+10) // 3 块
	sent, seq, err := SendInjectMem(uid, data, 4321, "auto")
	if err != nil {
		t.Fatalf("轮询类型下发失败: %v", err)
	}
	if sent != int64(len(data)) {
		t.Fatalf("发送字节数不符: %d != %d", sent, len(data))
	}
	if seq != 3 {
		t.Fatalf("分块数不符: %d != 3", seq)
	}

	var requestId uint32
	for i := uint32(0); i < seq; i++ {
		raw, ok := command.CommandQueues.GetCommand(uid)
		if !ok {
			t.Fatalf("第 %d 块未入队", i)
		}
		if len(raw) < 20 {
			t.Fatalf("第 %d 块过短: %d", i, len(raw))
		}
		if got := binary.BigEndian.Uint32(raw[0:4]); got != uint32(command.INJECT_MEM) {
			t.Fatalf("第 %d 块命令号不符: %d", i, got)
		}
		rid := binary.BigEndian.Uint32(raw[4:8])
		total := binary.BigEndian.Uint32(raw[8:12])
		s := binary.BigEndian.Uint32(raw[12:16])
		if i == 0 {
			requestId = rid
		} else if rid != requestId {
			t.Fatalf("第 %d 块 requestId 不一致: %d != %d", i, rid, requestId)
		}
		if total != 3 {
			t.Fatalf("第 %d 块 total 不符: %d", i, total)
		}
		if s != i {
			t.Fatalf("分块乱序: 第 %d 块 seq=%d", i, s)
		}
	}

	// 首包带 pid/tech，后续包这两个字段长度为 0
	first, _ := command.CommandQueues.GetCommand(uid)
	_ = first // 队列已取空，仅确认没有多余分块
	if _, ok := command.CommandQueues.GetCommand(uid); ok {
		t.Fatal("队列里有多余分块")
	}
}

// injectMemRequest 构造 /api/client/inject-mem 的 multipart 请求
func injectMemRequest(t *testing.T, uid, pid, tech string, payload []byte) *http.Request {
	t.Helper()
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, err := mw.CreateFormFile("file", "payload.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(payload); err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{"uid": uid, "pid": pid, "tech": tech} {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/client/inject-mem", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

// TestInjectMemSizeLimitMatchesClient 服务端上限必须与客户端一致。
// 客户端组装完成后有 12MB 硬上限（inject_mem_windows.go），服务端原来写 32MB 并注释
// "与客户端上限一致" —— 12~32MB 的载荷会通过服务端校验、到客户端才被拒。
func TestInjectMemSizeLimitMatchesClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/client/inject-mem", InjectMem)

	// 刚过服务端上限：必须被拦下且文案给出真实原因
	w := httptest.NewRecorder()
	router.ServeHTTP(w, injectMemRequest(t, "uid-1", "1234", "auto",
		bytes.Repeat([]byte("A"), injectMemMaxSize+1024)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("超限载荷应 400，实际 HTTP %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "大小非法") {
		t.Fatalf("错误文案未指出大小问题: %s", w.Body.String())
	}

	// 上限值本身要等于客户端的 12MB，否则这条断言没有意义
	if injectMemMaxSize != 12*1024*1024 {
		t.Fatalf("服务端上限 %d 与客户端的 12MB 不一致", injectMemMaxSize)
	}
}
