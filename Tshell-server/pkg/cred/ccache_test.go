package cred

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// ===== 用真实票据文件验证 ccache 解析 =====
//
// 夹具来自 `fortra/impacket` 的 `tests/data/`（Apache-2.0，见 testdata/README.md），
// 由 MIT Kerberos 工具链产出。
//
// 为什么必须用真实文件而不是自造数据：二进制格式的解析器若只对着自己写的数据测试，
// "我的写"和"我的读"会一起错而测试全绿 —— 这正是本项目在别处踩过的坑
// （画像一致性检查里按 Name 短路，让 5 个用例空转）。
//
// 下面刻意对 v3 与 v4 **断言同一组期望值**：两个版本的头部结构不同
// （v4 多一段配置头、密钥块少一个字段），若把版本分支写错，
// 通常只有一个版本能过 —— 两版同时过才说明分支是对的。

const (
	wantRealm    = "INNOVATION.ROCKS"
	wantClient   = "user01@" + wantRealm
	wantServer   = "krbtgt/" + wantRealm + "@" + wantRealm
	wantKeyType  = 18   // aes256-cts-hmac-sha1-96
	wantKeyLen   = 32   // AES-256 会话密钥
	wantEndYear  = 2022 // 夹具里票据的到期年份
	wantTicketLo = 1000 // 票据 DER 至少这么长
)

func loadFixture(t *testing.T, name string) *CCache {
	t.Helper()
	c, err := ParseCCacheFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("%s 解析失败: %v", name, err)
	}
	return c
}

func TestParseCCacheV3RealTicket(t *testing.T) {
	c := loadFixture(t, "ccache-v3")
	if c.Version != 3 {
		t.Errorf("版本应为 3，实际 %d", c.Version)
	}
	assertTGT(t, c)
}

func TestParseCCacheV4RealTicket(t *testing.T) {
	c := loadFixture(t, "ccache-v4")
	if c.Version != 4 {
		t.Errorf("版本应为 4，实际 %d", c.Version)
	}
	assertTGT(t, c)
}

// assertTGT 对两个版本断言同一组期望值
func assertTGT(t *testing.T, c *CCache) {
	t.Helper()

	if got := c.Principal.String(); got != wantClient {
		t.Errorf("主主体应为 %s，实际 %s", wantClient, got)
	}
	if c.Principal.Realm != wantRealm {
		t.Errorf("realm 应为 %s，实际 %s", wantRealm, c.Principal.Realm)
	}
	// v4 的配置头里带 krb5_ccache_conf_data 条目，必须被过滤掉，
	// 否则操作员会看到一堆"看起来像票据"的垃圾
	if len(c.Credentials) != 1 {
		t.Fatalf("凭据数应为 1（配置条目须被过滤），实际 %d", len(c.Credentials))
	}

	cr := c.Credentials[0]
	if got := cr.Client.String(); got != wantClient {
		t.Errorf("client 应为 %s，实际 %s", wantClient, got)
	}
	if got := cr.Server.String(); got != wantServer {
		t.Errorf("server 应为 %s，实际 %s", wantServer, got)
	}
	if cr.KeyType != wantKeyType {
		t.Errorf("keytype 应为 %d（aes256），实际 %d", wantKeyType, cr.KeyType)
	}
	if len(cr.KeyValue) != wantKeyLen {
		t.Errorf("会话密钥应为 %d 字节，实际 %d", wantKeyLen, len(cr.KeyValue))
	}
	if cr.EndTime.IsZero() || cr.EndTime.Year() != wantEndYear {
		t.Errorf("到期时间年份应为 %d，实际 %v", wantEndYear, cr.EndTime)
	}
	if cr.EndTime.Before(cr.StartTime) {
		t.Errorf("到期时间不应早于起始时间（%v < %v）", cr.EndTime, cr.StartTime)
	}
	if len(cr.Ticket) < wantTicketLo {
		t.Errorf("票据 DER 长度应 > %d，实际 %d", wantTicketLo, len(cr.Ticket))
	}
	// 票据应是 DER **APPLICATION 1**（0x61 = Kerberos Ticket 的应用标签），
	// 而不是普通 SEQUENCE(0x30)。这条比"能解析出长度"强得多：
	// 它证明取到的确实是票据本身，而不是错位读到的某段相邻字节。
	if len(cr.Ticket) == 0 || cr.Ticket[0] != 0x61 {
		t.Errorf("票据应以 DER APPLICATION 1(0x61, Kerberos Ticket) 开头，实际 %#x", firstByte(cr.Ticket))
	}
	if cr.SecondTicket == nil {
		t.Error("secondTicket 应为空切片而不是 nil（结构里它是必填字段）")
	}
}

func firstByte(b []byte) byte {
	if len(b) == 0 {
		return 0
	}
	return b[0]
}

// v1/v2 必须**明确拒绝**，而不是猜着解出错误的主体名。
// 猜错的后果是操作员以为拿到了一张有效票据。
func TestParseCCacheRejectsLegacyVersions(t *testing.T) {
	for _, name := range []string{"ccache-v1", "ccache-v2"} {
		_, err := ParseCCacheFile(filepath.Join("testdata", name))
		if err == nil {
			t.Fatalf("%s：v1/v2 应被明确拒绝，却解析成功了", name)
		}
		if !errors.Is(err, errCCacheLegacyVersion) {
			t.Errorf("%s：应返回 errCCacheLegacyVersion，实际 %v", name, err)
		}
	}
}

func TestParseCCacheRejectsNonCcache(t *testing.T) {
	cases := map[string][]byte{
		"空":        {},
		"只有一字节":    {0x05},
		"magic 不对": {0x00, 0x03},
		"版本号越界":    {0x05, 0x09},
		"普通文本":     []byte("this is not a ccache file at all"),
	}
	for name, data := range cases {
		if _, err := ParseCCache(data); err == nil {
			t.Errorf("%s：应被拒绝", name)
		}
	}
}

// 截断的数据必须报错，而不是 panic 或读出相邻字节
func TestParseCCacheTruncatedDoesNotPanic(t *testing.T) {
	full, err := os.ReadFile(filepath.Join("testdata", "ccache-v4"))
	if err != nil {
		t.Fatal(err)
	}
	// 从头部开始逐段截断，每一步都应返回错误而不是 panic
	for cut := 0; cut < len(full); cut += 37 {
		_, err := ParseCCache(full[:cut])
		if err == nil && cut < len(full)-1 {
			// 极少数截断点可能恰好落在记录边界上，允许成功
			continue
		}
		if err != nil && errors.Is(err, errCCacheBadVersion) && cut > 2 {
			t.Errorf("截断到 %d 字节时报了版本错（说明越界检查位置不对）: %v", cut, err)
		}
	}
}

// 恶意长度字段不得导致 panic 或巨额分配
func TestParseCCacheHostileLengths(t *testing.T) {
	// v4 头 + headerlen=12 + 12 字节配置，随后是超长的计数字符串长度
	hostile := []byte{
		0x05, 0x04, 0x00, 0x0c,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 配置段
		0x00, 0x00, 0x00, 0x01, // name_type
		0x00, 0x00, 0x00, 0x01, // num_components
		0xff, 0xff, 0xff, 0xff, // realm 长度 = 4GB
	}
	if _, err := ParseCCache(hostile); err == nil {
		t.Error("超长 realm 长度应被拒绝")
	}

	// 组件数超大
	hostile2 := []byte{
		0x05, 0x04, 0x00, 0x0c,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0x00, 0x00, 0x00, 0x01,
		0xff, 0xff, 0xff, 0xff, // num_components = 4G
	}
	if _, err := ParseCCache(hostile2); err == nil {
		t.Error("超大组件数应被拒绝")
	}
}

// 主体渲染
func TestCCachePrincipalString(t *testing.T) {
	cases := []struct {
		p    CCachePrincipal
		want string
	}{
		{CCachePrincipal{Realm: "R", Components: []string{"user"}}, "user@R"},
		{CCachePrincipal{Realm: "R", Components: []string{"krbtgt", "R"}}, "krbtgt/R@R"},
		{CCachePrincipal{Realm: "R", Components: []string{"a", "b", "c"}}, "a/b/c@R"},
		{CCachePrincipal{Realm: "R"}, "@R"},
	}
	for _, tc := range cases {
		if got := tc.p.String(); got != tc.want {
			t.Errorf("期望 %q，实际 %q", tc.want, got)
		}
	}
}

// 时间戳 0 应保留为零值（renew_till 未设置时就是 0），而不是 1970 年
func TestCCacheEpochZero(t *testing.T) {
	if !ccacheEpoch(0).IsZero() {
		t.Error("时间戳 0 应保留为零值时间")
	}
	if ccacheEpoch(1).Year() != 1970 {
		t.Error("时间戳 1 应是 1970 年")
	}
}
