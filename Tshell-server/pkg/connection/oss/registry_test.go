package oss

import (
	"BackendTemplate/pkg/connection"
	"bytes"
	"strings"
	"testing"
	"time"
)

// TestInitClientReturnsPerListenerClient 每个监听器拿到自己的 client。
// 原实现把结果写进包级 var Service：第二个 OSS 监听器一启动就覆盖第一个，
// 两个监听器随后都在轮同一个桶、命令也都被写进那个桶。
func TestInitClientReturnsPerListenerClient(t *testing.T) {
	a, err := InitClient("http://oss-cn-hangzhou.aliyuncs.com", "akA", "skA", "bucket-a")
	if err != nil {
		t.Fatalf("InitClient A 失败: %v", err)
	}
	b, err := InitClient("http://oss-cn-beijing.aliyuncs.com", "akB", "skB", "bucket-b")
	if err != nil {
		t.Fatalf("InitClient B 失败: %v", err)
	}
	if a == nil || b == nil {
		t.Fatal("两次初始化都应返回 client")
	}
	if a == b {
		t.Error("两次初始化应得到不同实例（不能共用同一份全局状态）")
	}
	if a.BucketName != "bucket-a" || a.Endpoint != "http://oss-cn-hangzhou.aliyuncs.com" {
		t.Errorf("A 的配置被污染: bucket=%q endpoint=%q", a.BucketName, a.Endpoint)
	}
	if b.BucketName != "bucket-b" || b.Endpoint != "http://oss-cn-beijing.aliyuncs.com" {
		t.Errorf("B 的配置被污染: bucket=%q endpoint=%q", b.BucketName, b.Endpoint)
	}
}

// TestClientRegistryIsolatesListeners 按 uid 取回的 client 必须归属该客户端所在的监听器；
// 关闭一个监听器只能清掉它名下的登记，不能影响另一个（这是多实例互不干扰的关键）。
func TestClientRegistryIsolatesListeners(t *testing.T) {
	const (
		keyA = "ep-a:akA:skA:bucketA"
		keyB = "ep-b:akB:skB:bucketB"
	)
	clientA := &Client{BucketName: "bucket-a"}
	clientB := &Client{BucketName: "bucket-b"}

	RegisterClient("uid-a", keyA, clientA)
	RegisterClient("uid-b", keyB, clientB)

	if got := ClientForUID("uid-a"); got != clientA {
		t.Errorf("uid-a 应取回 A 监听器的 client，实际 %v", got)
	}
	if got := ClientForUID("uid-b"); got != clientB {
		t.Errorf("uid-b 应取回 B 监听器的 client，实际 %v", got)
	}
	if got := ClientForUID("uid-unknown"); got != nil {
		t.Errorf("未登记的 uid 应返回 nil，实际 %v", got)
	}

	// 同一客户端再次上报：登记应被刷新
	clientA2 := &Client{BucketName: "bucket-a2"}
	RegisterClient("uid-a", keyA, clientA2)
	if got := ClientForUID("uid-a"); got != clientA2 {
		t.Errorf("重复登记应刷新为最新 client，实际 %v", got)
	}

	// 关闭 A 监听器：只清 A 名下，B 不受影响
	UnregisterListener(keyA)
	if got := ClientForUID("uid-a"); got != nil {
		t.Errorf("A 关闭后其客户端登记应被清除，实际 %v", got)
	}
	if got := ClientForUID("uid-b"); got != clientB {
		t.Errorf("A 关闭不应影响 B 的客户端，实际 %v", got)
	}

	UnregisterListener(keyB)
}

// TestRegisterClientIgnoresInvalid 空 uid / nil client 不应登记（否则会污染查询结果）
func TestRegisterClientIgnoresInvalid(t *testing.T) {
	RegisterClient("", "key", &Client{BucketName: "b"})
	RegisterClient("uid-nil", "key", nil)
	if got := ClientForUID(""); got != nil {
		t.Errorf("空 uid 不应被登记，实际 %v", got)
	}
	if got := ClientForUID("uid-nil"); got != nil {
		t.Errorf("nil client 不应被登记，实际 %v", got)
	}
}

// TestAbnormalExitNotifiesAndCleans 轮询循环 panic（被 recover）后必须摘客户端登记、
// 删停止通道并回调上报，否则界面显示"监听中"、实际已不再轮询桶。
// 正常停止不得触发回调。
func TestAbnormalExitNotifiesAndCleans(t *testing.T) {
	const stopKey = "ep.example.com:ak:sk:bucket-x"

	connection.MuClientListenerType.Lock()
	connection.StopChan[stopKey] = make(chan bool, 1)
	connection.MuClientListenerType.Unlock()
	RegisterClient("uid-oss", stopKey, &Client{BucketName: "bucket-x"})

	exited := make(chan string, 1)
	// 空 Client（Bucket 为 nil）：轮询到 List 时触发 panic，被 recover 后走异常退出收尾
	go HandleOSSConnection(&Client{}, stopKey, bytes.Repeat([]byte{0xA1}, 32), bytes.Repeat([]byte{0xA2}, 16),
		func(reason string) { exited <- reason })

	select {
	case reason := <-exited:
		if !strings.Contains(reason, "panic") {
			t.Errorf("原因应说明 panic，实际 %q", reason)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("异常退出应触发回调")
	}

	connection.MuClientListenerType.Lock()
	_, hasStop := connection.StopChan[stopKey]
	connection.MuClientListenerType.Unlock()
	if hasStop {
		t.Error("异常退出后应删除停止通道")
	}
	if got := ClientForUID("uid-oss"); got != nil {
		t.Error("异常退出后应清除该监听器名下的客户端登记")
	}
}

// TestNormalStopDoesNotNotifyOSS 正常停止（停止通道）不应触发异常退出回调
func TestNormalStopDoesNotNotifyOSS(t *testing.T) {
	const stopKey = "ep.example.com:ak:sk:bucket-y"
	stopCh := make(chan bool, 1)
	connection.MuClientListenerType.Lock()
	connection.StopChan[stopKey] = stopCh
	connection.MuClientListenerType.Unlock()

	notified := make(chan string, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		HandleOSSConnection(&Client{}, stopKey, bytes.Repeat([]byte{0xA1}, 32), bytes.Repeat([]byte{0xA2}, 16),
			func(reason string) { notified <- reason })
	}()

	// 循环每次先判停止通道再 sleep，这里立即发停止信号
	stopCh <- true

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("停止信号应让轮询循环退出")
	}
	connection.MuClientListenerType.Lock()
	delete(connection.StopChan, stopKey)
	connection.MuClientListenerType.Unlock()

	select {
	case reason := <-notified:
		t.Errorf("正常停止不应触发异常退出回调，实际: %s", reason)
	default:
	}
}
