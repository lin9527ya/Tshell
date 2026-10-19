package services

import "time"

// Windows LSA 票据接口的结构体与常量（**跨平台文件**：只有布局，不含 syscall）。
//
// 放在这里而不是 windows 文件里，是为了让"偏移/大小"能在 Linux 上被单测断言 ——
// 这些结构体是直接喂给 LsaCallAuthenticationPackage 的二进制协议，一个字段错位
// 的结果是静默失败或读越界。常量与布局对照 mingw-w64 的 ntsecapi.h（KERB_PROTOCOL_
// MESSAGE_TYPE 枚举顺序与 KERB_* 结构体定义）。
//
// 平台差异：386 上指针 4 字节，布局与 amd64/arm64 不同；布局断言只在 64 位跑。

// KERB_PROTOCOL_MESSAGE_TYPE（按枚举顺序，从 0 起）
const (
	kerbQueryTicketCacheEx2Message   int32 = 20
	kerbRetrieveEncodedTicketMessage int32 = 8
	kerbPurgeTicketCacheMessage      int32 = 6
	kerbSubmitTicketMessage          int32 = 21
)

// lsaUnicodeString 对应 UNICODE_STRING：Length 是**字节数**（不含结尾 NUL）
type lsaUnicodeString struct {
	Length        uint16
	MaximumLength uint16
	Buffer        *uint16
}

// lsaLUID 对应 LUID
type lsaLUID struct {
	LowPart  uint32
	HighPart int32
}

// kerbQueryTktCacheRequest 对应 KERB_QUERY_TKT_CACHE_REQUEST
type kerbQueryTktCacheRequest struct {
	MessageType int32
	LogonId     lsaLUID
}

// kerbTicketCacheInfoEx2 对应 KERB_TICKET_CACHE_INFO_EX2
type kerbTicketCacheInfoEx2 struct {
	ClientName     lsaUnicodeString
	ClientRealm    lsaUnicodeString
	ServerName     lsaUnicodeString
	ServerRealm    lsaUnicodeString
	StartTime      int64 // FILETIME（100ns since 1601）
	EndTime        int64
	RenewTime      int64
	EncryptionType int32
	TicketFlags    uint32
	SessionKeyType uint32
}

// kerbQueryTktCacheEx2Response 对应 KERB_QUERY_TKT_CACHE_EX2_RESPONSE 的**头部**；
// Tickets 是紧随其后的变长数组（按 CountOfTickets 个 kerbTicketCacheInfoEx2 读）。
type kerbQueryTktCacheEx2Response struct {
	MessageType    int32
	CountOfTickets uint32
}

// secHandle 对应 SecHandle（LsaCallAuthenticationPackage 的凭据句柄，未使用时清零）
type secHandle struct {
	Lower uintptr
	Upper uintptr
}

// kerbRetrieveTktRequest 对应 KERB_RETRIEVE_TKT_REQUEST
type kerbRetrieveTktRequest struct {
	MessageType       int32
	LogonId           lsaLUID
	TargetName        lsaUnicodeString
	TicketFlags       uint32
	CacheOptions      uint32
	EncryptionType    int32
	CredentialsHandle secHandle
}

// kerbCryptoKey 对应 KERB_CRYPTO_KEY（KERB_EXTERNAL_TICKET.SessionKey）
type kerbCryptoKey struct {
	KeyType int32
	Length  uint32
	Value   *byte
}

// kerbExternalTicket 对应 KERB_EXTERNAL_TICKET（只声明到 EncodedTicket 为止，
// 前面的名字指针我们不解析 —— Ex2 查询结果里已经有可读的名字了）
type kerbExternalTicket struct {
	ServiceName         *kerbExternalName
	TargetName          *kerbExternalName
	ClientName          *kerbExternalName
	DomainName          lsaUnicodeString
	TargetDomainName    lsaUnicodeString
	AltTargetDomainName lsaUnicodeString
	SessionKey          kerbCryptoKey
	TicketFlags         uint32
	Flags               uint32
	KeyExpirationTime   int64
	StartTime           int64
	EndTime             int64
	RenewUntil          int64
	TimeSkew            int64
	EncodedTicketSize   uint32
	EncodedTicket       *byte
}

// kerbExternalName 对应 KERB_EXTERNAL_NAME（只用于指针类型，不读内容）
type kerbExternalName struct {
	NameType int16
}

// kerbRetrieveTktResponse 对应 KERB_RETRIEVE_TKT_RESPONSE
type kerbRetrieveTktResponse struct {
	Ticket kerbExternalTicket
}

// kerbPurgeTktCacheRequest 对应 KERB_PURGE_TKT_CACHE_REQUEST
// （ServerName/RealmName 都为空表示清空全部票据）
type kerbPurgeTktCacheRequest struct {
	MessageType int32
	LogonId     lsaLUID
	ServerName  lsaUnicodeString
	RealmName   lsaUnicodeString
}

// kerbSubmitTktRequest 对应 KERB_SUBMIT_TKT_REQUEST；KRB_CRED 字节**追加在结构体之后**，
// KerbCredOffset 是它相对缓冲区起始的偏移（= 结构体大小）。
type kerbSubmitTktRequest struct {
	MessageType    int32
	LogonId        lsaLUID
	Flags          uint32
	Key            kerbCryptoKey
	KerbCredSize   uint32
	KerbCredOffset uint32
}

// lsaString LSA_STRING（包名 "Kerberos"），Length 是字节数
type lsaString struct {
	Length        uint16
	MaximumLength uint16
	Buffer        *byte
}

// tokenStatistics 对应 TOKEN_STATISTICS（x/sys/windows 只给了 InfoClass 常量，
// 没有结构体）。只用到 AuthenticationId —— 即登录会话 LUID。
type tokenStatistics struct {
	TokenId            lsaLUID
	AuthenticationId   lsaLUID
	ExpirationTime     int64
	TokenType          int32
	ImpersonationLevel int32
	DynamicCharged     uint32
	DynamicAvailable   uint32
	GroupCount         uint32
	PrivilegeCount     uint32
	ModifiedId         lsaLUID
}

// lsaTicket LSA 缓存里的一张票据（服务端入库用的元数据 + 导出字节）
type lsaTicket struct {
	Client         string // user@REALM
	ClientRealm    string
	Server         string // krbtgt/DOMAIN 或 cifs/host
	ServerRealm    string
	Start          int64 // FILETIME
	End            int64
	Renew          int64
	Flags          uint32
	EncType        int32
	SessionKeyType uint32
	KerbCred       []byte // 导出（KerbRetrieveEncodedTicketMessage）得到的 KRB_CRED
}

// lsaTicketTime 把 Unix 秒渲染成便于阅读的 RFC3339（0 表示未设置）
func lsaTicketTime(sec int64) string {
	if sec <= 0 {
		return ""
	}
	return time.Unix(sec, 0).UTC().Format(time.RFC3339)
}
