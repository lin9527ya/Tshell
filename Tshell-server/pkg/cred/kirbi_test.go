package cred

import (
	"os"
	"path/filepath"
	"testing"
)

// ===== kirbi（KRB-CRED）与 ccache 互转，用真实夹具验证 =====
//
// 夹具同 ccache 测试：取自 `fortra/impacket` 的 `tests/data/`（Apache-2.0）。
// `ccache-v4-kirbi` / `ccache-v3-kirbi` 是 kirbi 形态，
// `ccache-v4` 是**同一套测试票据**的 ccache 形态 —— 两组互为对照，
// 因此可以交叉验证：kirbi 解出来的字段必须与 ccache 解出来的对得上。
//
// 这里刻意不断言"字节完全相同"：kirbi 与 ccache 是两种编码，
// 字段集合不同（ccache 多地址/authdata 占位，kirbi 多 EncKrbCredPart 包装），
// 逐字节比对没有意义。有意义的是**语义字段一致** + **往返稳定**。

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("读夹具 %s 失败: %v", name, err)
	}
	return b
}

func TestParseKirbiV4RealTicket(t *testing.T) {
	cc, err := ParseKirbi(readFixture(t, "ccache-v4-kirbi"))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	assertKirbiTicket(t, cc)
}

func TestParseKirbiV3RealTicket(t *testing.T) {
	cc, err := ParseKirbi(readFixture(t, "ccache-v3-kirbi"))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	assertKirbiTicket(t, cc)
}

func assertKirbiTicket(t *testing.T, cc *CCache) {
	t.Helper()
	if len(cc.Credentials) != 1 {
		t.Fatalf("凭据数应为 1，实际 %d", len(cc.Credentials))
	}
	cr := cc.Credentials[0]

	// 与 ccache 夹具的交叉对照：这些值在两份形态里必须一致
	if got := cr.Client.String(); got != wantClient {
		t.Errorf("client 应为 %s，实际 %s", wantClient, got)
	}
	if got := cr.Server.String(); got != wantServer {
		t.Errorf("server 应为 %s，实际 %s", wantServer, got)
	}
	if cr.KeyType != wantKeyType {
		t.Errorf("keytype 应为 %d，实际 %d", wantKeyType, cr.KeyType)
	}
	if len(cr.KeyValue) != wantKeyLen {
		t.Errorf("会话密钥应为 %d 字节，实际 %d", wantKeyLen, len(cr.KeyValue))
	}
	if len(cr.Ticket) < wantTicketLo {
		t.Errorf("票据应 > %d 字节，实际 %d", wantTicketLo, len(cr.Ticket))
	}
	if cr.Ticket[0] != 0x61 {
		t.Errorf("票据应以 APPLICATION 1(0x61) 开头，实际 %#x", firstByte(cr.Ticket))
	}
	// 时间必须解出来且自洽（kirbi 用 GeneralizedTime 字符串，
	// 解析成 time.Time 后与前面对 ccache 的年份断言一致）
	if cr.StartTime.IsZero() || cr.StartTime.Year() != wantEndYear {
		t.Errorf("起始时间年份应为 %d，实际 %v", wantEndYear, cr.StartTime)
	}
	if cr.EndTime.Before(cr.StartTime) {
		t.Errorf("到期不应早于起始：%v < %v", cr.EndTime, cr.StartTime)
	}
	// 缓存主体取首条凭据的 client
	if got := cc.Principal.String(); got != wantClient {
		t.Errorf("缓存主体应为 %s，实际 %s", wantClient, got)
	}
}

// kirbi 与 ccache 两种形态解出的关键字段必须一致（交叉验证）
func TestKirbiMatchesCCacheFields(t *testing.T) {
	fromKirbi, err := ParseKirbi(readFixture(t, "ccache-v4-kirbi"))
	if err != nil {
		t.Fatal(err)
	}
	fromCCache, err := ParseCCache(readFixture(t, "ccache-v4"))
	if err != nil {
		t.Fatal(err)
	}

	k := fromKirbi.Credentials[0]
	c := fromCCache.Credentials[0]
	if k.Client.String() != c.Client.String() {
		t.Errorf("client 不一致：kirbi=%s ccache=%s", k.Client, c.Client)
	}
	if k.Server.String() != c.Server.String() {
		t.Errorf("server 不一致：kirbi=%s ccache=%s", k.Server, c.Server)
	}
	if k.KeyType != c.KeyType {
		t.Errorf("keytype 不一致：kirbi=%d ccache=%d", k.KeyType, c.KeyType)
	}
	if string(k.KeyValue) != string(c.KeyValue) {
		t.Errorf("会话密钥不一致")
	}
	if k.TicketFlags != c.TicketFlags {
		t.Errorf("旗标不一致：kirbi=%08x ccache=%08x", k.TicketFlags, c.TicketFlags)
	}
	// 票据本体必须逐字节相同 —— 它是同一张 Ticket 的 DER
	if string(k.Ticket) != string(c.Ticket) {
		t.Errorf("票据 DER 不一致：kirbi=%d 字节 ccache=%d 字节", len(k.Ticket), len(c.Ticket))
	}
}

// ccache → kirbi → kirbi 解析：字段必须完整保留
func TestCCacheToKirbiRoundTrip(t *testing.T) {
	orig, err := ParseCCache(readFixture(t, "ccache-v4"))
	if err != nil {
		t.Fatal(err)
	}
	kirbi, err := orig.MarshalKirbi()
	if err != nil {
		t.Fatalf("MarshalKirbi 失败: %v", err)
	}
	// 产出必须是合法的 KRB-CRED
	if len(kirbi) < 2 || kirbi[0] != appKRBCred {
		t.Fatalf("产出的不是 KRB-CRED（首字节 %#x）", firstByte(kirbi))
	}

	back, err := ParseKirbi(kirbi)
	if err != nil {
		t.Fatalf("重新解析失败: %v", err)
	}
	if len(back.Credentials) != len(orig.Credentials) {
		t.Fatalf("凭据数不一致：%d vs %d", len(back.Credentials), len(orig.Credentials))
	}
	o, b := orig.Credentials[0], back.Credentials[0]
	if o.Client.String() != b.Client.String() {
		t.Errorf("client 丢失：%s -> %s", o.Client, b.Client)
	}
	if o.Server.String() != b.Server.String() {
		t.Errorf("server 丢失：%s -> %s", o.Server, b.Server)
	}
	if o.KeyType != b.KeyType || string(o.KeyValue) != string(b.KeyValue) {
		t.Errorf("会话密钥丢失")
	}
	if o.TicketFlags != b.TicketFlags {
		t.Errorf("旗标丢失：%08x -> %08x", o.TicketFlags, b.TicketFlags)
	}
	if string(o.Ticket) != string(b.Ticket) {
		t.Errorf("票据 DER 改变：%d -> %d 字节", len(o.Ticket), len(b.Ticket))
	}
	// 时间按秒精度往返（ccache 存秒，kirbi 存 GeneralizedTime 秒）
	if !o.EndTime.Equal(b.EndTime) {
		t.Errorf("到期时间改变：%v -> %v", o.EndTime, b.EndTime)
	}
}

// kirbi → ccache → ccache 解析：能给出可被 MIT 工具链接受的形态
func TestKirbiToCCacheRoundTrip(t *testing.T) {
	cc, err := ParseKirbi(readFixture(t, "ccache-v4-kirbi"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := cc.Marshal()
	if err != nil {
		t.Fatalf("Marshal 失败: %v", err)
	}
	back, err := ParseCCache(raw)
	if err != nil {
		t.Fatalf("产出的 ccache 无法被自己的解析器接受: %v", err)
	}
	if back.Version != 4 {
		t.Errorf("产出应为 v4，实际 %d", back.Version)
	}
	if back.Principal.String() != wantClient {
		t.Errorf("主体丢失：%s", back.Principal)
	}
	if len(back.Credentials) != 1 {
		t.Fatalf("凭据数应为 1，实际 %d", len(back.Credentials))
	}
	cr := back.Credentials[0]
	if cr.Server.String() != wantServer {
		t.Errorf("server 丢失：%s", cr.Server)
	}
	if string(cr.Ticket) != string(cc.Credentials[0].Ticket) {
		t.Error("票据 DER 在转 ccache 后改变")
	}
	if cr.TicketFlags != cc.Credentials[0].TicketFlags {
		t.Errorf("旗标丢失：%08x -> %08x", cc.Credentials[0].TicketFlags, cr.TicketFlags)
	}
}

// 两次转换必须产出相同字节（输出稳定，便于比对与入库去重）
func TestKirbiMarshalIsDeterministic(t *testing.T) {
	cc, err := ParseKirbi(readFixture(t, "ccache-v4-kirbi"))
	if err != nil {
		t.Fatal(err)
	}
	a, err := cc.MarshalKirbi()
	if err != nil {
		t.Fatal(err)
	}
	b, err := cc.MarshalKirbi()
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Error("同样输入两次 MarshalKirbi 结果不同（时间字段按 map 遍历写入时必须排序）")
	}
}

func TestParseKirbiRejectsGarbage(t *testing.T) {
	cases := map[string][]byte{
		"空":                {},
		"普通文本":             []byte("this is not a kirbi file"),
		"只有 APPLICATION 头": {0x76},
		"错误的 APPLICATION":  {0x61, 0x00},
	}
	for name, data := range cases {
		if _, err := ParseKirbi(data); err == nil {
			t.Errorf("%s：应被拒绝", name)
		}
	}
}

// 截断的 kirbi 必须报错而不是 panic
func TestParseKirbiTruncatedDoesNotPanic(t *testing.T) {
	full := readFixture(t, "ccache-v4-kirbi")
	for cut := 0; cut < len(full); cut += 53 {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("截断到 %d 字节时 panic: %v", cut, r)
				}
			}()
			_, _ = ParseKirbi(full[:cut])
		}()
	}
}

// 空的 CCache 不能产出 kirbi（没有票据就没有 KRB-CRED）
func TestMarshalKirbiRejectsEmpty(t *testing.T) {
	if _, err := (&CCache{}).MarshalKirbi(); err == nil {
		t.Error("空 ccache 应拒绝导出 kirbi")
	}
	if _, err := (*CCache)(nil).MarshalKirbi(); err == nil {
		t.Error("nil 应拒绝导出 kirbi")
	}
}

// Kerberos GeneralizedTime 的几种形态都要能解
func TestParseKerberosTime(t *testing.T) {
	ok := []string{"20220209170649Z", "202202091706Z", "2022020917Z"}
	for _, s := range ok {
		got, err := parseKerberosTime(s)
		if err != nil {
			t.Errorf("%q 应可解析: %v", s, err)
		}
		if got.Year() != 2022 {
			t.Errorf("%q 年份应为 2022，实际 %d", s, got.Year())
		}
	}
	bad := []string{"", "not-a-time", "2022-02-09T17:06:49Z"}
	for _, s := range bad {
		if _, err := parseKerberosTime(s); err == nil {
			t.Errorf("%q 应解析失败", s)
		}
	}
}
