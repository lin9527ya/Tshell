package services

import (
	"runtime"
	"testing"
	"unsafe"
)

// TestLSALayout 断言与 ntsecapi.h 对齐的结构体大小/偏移。
//
// 这些结构体直接作为 LsaCallAuthenticationPackage 的二进制协议：字段错位不会编译
// 报错，只会在 Windows 上静默失败或读到垃圾指针。断言只在 64 位平台跑（386 的
// 指针是 4 字节，布局不同，而 386 模板本来就只做交叉编译验证）。
func TestLSALayout(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("非 64 位平台，布局不同")
	}
	_ = runtime.GOOS

	cases := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"UNICODE_STRING", unsafe.Sizeof(lsaUnicodeString{}), 16},
		{"LUID", unsafe.Sizeof(lsaLUID{}), 8},
		{"KERB_QUERY_TKT_CACHE_REQUEST", unsafe.Sizeof(kerbQueryTktCacheRequest{}), 12},
		{"KERB_TICKET_CACHE_INFO_EX2", unsafe.Sizeof(kerbTicketCacheInfoEx2{}), 104},
		{"KERB_QUERY_TKT_CACHE_EX2_RESPONSE头", unsafe.Sizeof(kerbQueryTktCacheEx2Response{}), 8},
		{"KERB_RETRIEVE_TKT_REQUEST", unsafe.Sizeof(kerbRetrieveTktRequest{}), 64},
		{"KERB_CRYPTO_KEY", unsafe.Sizeof(kerbCryptoKey{}), 16},
		{"KERB_EXTERNAL_TICKET", unsafe.Sizeof(kerbExternalTicket{}), 152},
		{"KERB_PURGE_TKT_CACHE_REQUEST", unsafe.Sizeof(kerbPurgeTktCacheRequest{}), 48},
		{"KERB_SUBMIT_TKT_REQUEST", unsafe.Sizeof(kerbSubmitTktRequest{}), 40},
		{"LSA_STRING", unsafe.Sizeof(lsaString{}), 16},
		{"TOKEN_STATISTICS", unsafe.Sizeof(tokenStatistics{}), 56},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s 大小 %d，期望 %d", c.name, c.got, c.want)
		}
	}

	// 关键偏移：EncodedTicket 必须落在 144（否则导出读到的是别的字段）
	if off := unsafe.Offsetof(kerbExternalTicket{}.EncodedTicket); off != 144 {
		t.Errorf("KERB_EXTERNAL_TICKET.EncodedTicket 偏移 %d，期望 144", off)
	}
	if off := unsafe.Offsetof(tokenStatistics{}.AuthenticationId); off != 8 {
		t.Errorf("TOKEN_STATISTICS.AuthenticationId 偏移 %d，期望 8", off)
	}
	if off := unsafe.Offsetof(kerbRetrieveTktRequest{}.TargetName); off != 16 {
		t.Errorf("KERB_RETRIEVE_TKT_REQUEST.TargetName 偏移 %d，期望 16", off)
	}
	// LUID 与 KERB_CRYPTO_KEY 都是 4 字节对齐（不是 8）—— 这一点用同构 C 结构体
	// 在 x86-64 ABI 下编译核对过：QUERY=12、SUBMIT=40（Key@16）、RETRIEVE=64、
	// EXT=152（EncodedTicket@144）。
	if off := unsafe.Offsetof(kerbSubmitTktRequest{}.Key); off != 16 {
		t.Errorf("KERB_SUBMIT_TKT_REQUEST.Key 偏移 %d，期望 16", off)
	}
}

// TestLSAMessageTypes 锁住协议消息号（来自 ntsecapi.h 枚举，写错就是静默失败）
func TestLSAMessageTypes(t *testing.T) {
	cases := map[string][2]int32{
		"KerbPurgeTicketCacheMessage":      {kerbPurgeTicketCacheMessage, 6},
		"KerbRetrieveEncodedTicketMessage": {kerbRetrieveEncodedTicketMessage, 8},
		"KerbQueryTicketCacheEx2Message":   {kerbQueryTicketCacheEx2Message, 20},
		"KerbSubmitTicketMessage":          {kerbSubmitTicketMessage, 21},
	}
	for name, v := range cases {
		if v[0] != v[1] {
			t.Errorf("%s = %d，期望 %d", name, v[0], v[1])
		}
	}
}
