package oss

import "testing"

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
