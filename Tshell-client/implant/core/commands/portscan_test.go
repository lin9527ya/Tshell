package commands

import (
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

// 端口扫描的两类风险：参数没校验（把扫描变成 DoS / 卡死命令队列）与结果误报。
// 因此这里既测解析边界，也做一次真实的本地扫描。

func TestExpandHosts(t *testing.T) {
	// 单个 IP
	hosts, err := expandHosts("10.0.0.5")
	if err != nil || len(hosts) != 1 || hosts[0] != "10.0.0.5" {
		t.Fatalf("单 IP 展开失败: %v %v", hosts, err)
	}
	// /30：4 个地址，去掉网络号与广播 → 2 台
	hosts, err = expandHosts("192.168.1.0/30")
	if err != nil || len(hosts) != 2 {
		t.Fatalf("/30 应展开 2 台主机，实际 %v (%v)", hosts, err)
	}
	if hosts[0] != "192.168.1.1" || hosts[1] != "192.168.1.2" {
		t.Fatalf("/30 主机范围不符: %v", hosts)
	}
	// /32：单地址
	if hosts, err = expandHosts("10.1.2.3/32"); err != nil || len(hosts) != 1 || hosts[0] != "10.1.2.3" {
		t.Fatalf("/32 展开失败: %v %v", hosts, err)
	}
	// 逗号混合 + 去重
	hosts, err = expandHosts("10.0.0.1,10.0.0.1,192.168.1.0/30")
	if err != nil || len(hosts) != 3 {
		t.Fatalf("逗号混合展开失败: %v (%v)", hosts, err)
	}
	// 大网段必须被拒绝（不静默截断）
	if _, err := expandHosts("10.0.0.0/16"); err == nil {
		t.Fatal("超过上限的网段应被拒绝")
	}
	if _, err := expandHosts("not a host at all /"); err == nil {
		t.Fatal("非法目标应报错")
	}
	if _, err := expandHosts(""); err == nil {
		t.Fatal("空目标应报错")
	}
}

func TestExpandPorts(t *testing.T) {
	ports, err := expandPorts("22,80,443")
	if err != nil || len(ports) != 3 || ports[0] != 22 || ports[2] != 443 {
		t.Fatalf("端口列表展开失败: %v %v", ports, err)
	}
	// 范围 + 去重 + 排序
	ports, err = expandPorts("80,1-5,3")
	if err != nil || len(ports) != 6 {
		t.Fatalf("范围展开失败: %v %v", ports, err)
	}
	for i := 1; i < len(ports); i++ {
		if ports[i-1] >= ports[i] {
			t.Fatalf("端口未排序/去重: %v", ports)
		}
	}
	// top
	ports, err = expandPorts("top")
	if err != nil || len(ports) != len(topPorts) {
		t.Fatalf("top 端口展开失败: %v", err)
	}
	// 非法
	for _, bad := range []string{"0", "65536", "80-70", "abc", "80-", ""} {
		if _, err := expandPorts(bad); err == nil {
			t.Errorf("非法端口 %q 应报错", bad)
		}
	}
	// 超上限拒绝
	if _, err := expandPorts("1-65535"); err == nil {
		t.Fatal("超过端口上限应被拒绝")
	}
}

func TestParseScanSpec(t *testing.T) {
	hosts, ports, timeout, err := parseScanSpec("127.0.0.1 22,80 500")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(hosts) != 1 || len(ports) != 2 || timeout != 500*time.Millisecond {
		t.Fatalf("解析结果不符: %v %v %v", hosts, ports, timeout)
	}
	// 缺省超时
	if _, _, timeout, _ = parseScanSpec("127.0.0.1 80"); timeout != scanDefaultTimeout {
		t.Fatalf("缺省超时应为 %v，实际 %v", scanDefaultTimeout, timeout)
	}
	// 超时钳制
	if _, _, timeout, _ = parseScanSpec("127.0.0.1 80 1"); timeout != scanMinTimeout {
		t.Fatalf("过小超时应钳制到 %v，实际 %v", scanMinTimeout, timeout)
	}
	if _, _, timeout, _ = parseScanSpec("127.0.0.1 80 999999"); timeout != scanMaxTimeout {
		t.Fatalf("过大超时应钳制到 %v，实际 %v", scanMaxTimeout, timeout)
	}
	// 参数不足 / 非法超时
	if _, _, _, err := parseScanSpec("127.0.0.1"); err == nil {
		t.Fatal("缺少端口应报错")
	}
	if _, _, _, err := parseScanSpec("127.0.0.1 80 abc"); err == nil {
		t.Fatal("非法超时应报错")
	}
}

// TestPortScanFindsOpenPort 真实扫描：本机监听一个随机端口，扫描应只报它开放
func TestPortScanFindsOpenPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听失败: %v", err)
	}
	defer ln.Close()
	openPort := ln.Addr().(*net.TCPAddr).Port

	// 找一个几乎肯定关闭的端口：与开放端口不同且未被监听
	closedPort := openPort + 1
	if closedPort > 65535 {
		closedPort = openPort - 1
	}

	out, err := CmdPortScan([]byte("127.0.0.1 " + strconv.Itoa(openPort) + "," + strconv.Itoa(closedPort) + " 300"))
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	text := string(out)
	if !strings.Contains(text, "127.0.0.1  open: "+strconv.Itoa(openPort)) {
		t.Fatalf("应报告开放端口 %d:\n%s", openPort, text)
	}
	if strings.Contains(text, strconv.Itoa(closedPort)) {
		t.Fatalf("不应报告关闭端口 %d:\n%s", closedPort, text)
	}
	if !strings.Contains(text, "[scan]") {
		t.Fatalf("缺少汇总行:\n%s", text)
	}
}

// TestPortScanRejectsBadInput 参数错误必须显式失败（而不是静默扫 0 个端口）
func TestPortScanRejectsBadInput(t *testing.T) {
	for _, spec := range []string{"", "127.0.0.1", "127.0.0.1 notaport", "999.999.999.999 80"} {
		if _, err := CmdPortScan([]byte(spec)); err == nil {
			t.Errorf("CmdPortScan(%q) 应报错", spec)
		}
	}
}
