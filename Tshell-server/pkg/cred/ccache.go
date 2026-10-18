package cred

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"time"
)

// ===== Kerberos 凭据缓存（MIT ccache）解析 =====
//
// 为什么需要它：`docs/CREDENTIAL_ACCESS.md` 把票据文件定义为**制品**
// （"列出/导出/注入/清除票据 | 票据文件即制品"）。采集到的票据要能列出、
// 导出、复用，就必须先能解析这些文件。
//
// 格式出处：MIT Kerberos ccache_file_format，实现对齐 `fortra/impacket` 的
// `impacket/krb5/ccache.py`（该实现与下面的测试夹具同源）。
//
// **所有整数都是大端**（Impacket 里全是 `!` 前缀）。这一条必须记牢：
// 按小端解会得到看似"合理"但完全错误的长度，进而把整个文件读成垃圾。
//
// 版本支持范围（刻意划定）：
//   - v3 / v4：**已实现**，并用真实票据文件验证（见 ccache_test.go）
//   - v1 / v2：**明确拒绝**，返回 errCCacheLegacyVersion
//
// v1/v2 的处理方式值得说明：Impacket 自己也只抛 NotImplementedError。
// 它们是 2003 年前的格式，而 v1 的 principal **没有组件计数**字段
// （结构上就无法与后续字节无歧义地切分）。与其"猜着解"出错误的主体名 ——
// 那会让操作员以为拿到了一张有效票据 —— 不如明确报"不支持这个版本"。

var (
	errCCacheLegacyVersion = errors.New("不支持 ccache v1/v2（2003 年前的格式，v1 的 principal 无组件计数字段，无法无歧义解析）")
	errCCacheTruncated     = errors.New("ccache 数据被截断")
	errCCacheBadVersion    = errors.New("不是合法的 ccache 文件（版本号不是 0x0501-0x0504）")
)

// CCachePrincipal Kerberos 主体
type CCachePrincipal struct {
	NameType   uint32
	Realm      string
	Components []string
}

// String 渲染成 `comp1/comp2@REALM`
func (p CCachePrincipal) String() string {
	s := ""
	for i, c := range p.Components {
		if i > 0 {
			s += "/"
		}
		s += c
	}
	return s + "@" + p.Realm
}

// CCacheCredential 一条票据（含会话密钥）
type CCacheCredential struct {
	Client       CCachePrincipal
	Server       CCachePrincipal
	KeyType      uint16
	KeyValue     []byte
	AuthTime     time.Time
	StartTime    time.Time
	EndTime      time.Time
	RenewTill    time.Time
	IsSKey       bool
	TicketFlags  uint32
	Ticket       []byte
	SecondTicket []byte
}

// CCache 一个解析后的凭据缓存
type CCache struct {
	Version     int
	Principal   CCachePrincipal
	Credentials []CCacheCredential
}

// ccacheReader 带边界检查的顺序读取器。
//
// 每一处读取都检查剩余长度：票据文件可能来自不可信来源（对手上传的制品），
// 越界读取会 panic 或读到相邻字段的字节并当成合法值 —— 两者都比报错更糟。
type ccacheReader struct {
	b   []byte
	off int
}

func (r *ccacheReader) remaining() int { return len(r.b) - r.off }

func (r *ccacheReader) u8() (byte, error) {
	if r.remaining() < 1 {
		return 0, errCCacheTruncated
	}
	v := r.b[r.off]
	r.off++
	return v, nil
}

func (r *ccacheReader) u16() (uint16, error) {
	if r.remaining() < 2 {
		return 0, errCCacheTruncated
	}
	v := binary.BigEndian.Uint16(r.b[r.off:])
	r.off += 2
	return v, nil
}

func (r *ccacheReader) u32() (uint32, error) {
	if r.remaining() < 4 {
		return 0, errCCacheTruncated
	}
	v := binary.BigEndian.Uint32(r.b[r.off:])
	r.off += 4
	return v, nil
}

// bytesN 读 n 字节。n 会先与实际剩余长度比对 —— 直接切片会 panic。
func (r *ccacheReader) bytesN(n int) ([]byte, error) {
	if n < 0 || r.remaining() < n {
		return nil, errCCacheTruncated
	}
	v := r.b[r.off : r.off+n]
	r.off += n
	return v, nil
}

// countedString 计数字符串：长度(4) + 数据
func (r *ccacheReader) countedString() (string, error) {
	n, err := r.u32()
	if err != nil {
		return "", err
	}
	// 先做上限检查再切片：恶意长度（如 0xFFFFFFFF）会直接 panic
	if int(n) > r.remaining() {
		return "", errCCacheTruncated
	}
	b, err := r.bytesN(int(n))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (r *ccacheReader) countedBytes() ([]byte, error) {
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	if int(n) > r.remaining() {
		return nil, errCCacheTruncated
	}
	return r.bytesN(int(n))
}

// readPrincipal 读一个主体：name_type(4) + num_components(4) + realm + components
func (r *ccacheReader) readPrincipal() (CCachePrincipal, error) {
	var p CCachePrincipal
	var err error
	if p.NameType, err = r.u32(); err != nil {
		return p, err
	}
	n, err := r.u32()
	if err != nil {
		return p, err
	}
	// 组件数上限：每个组件至少 4 字节长度前缀，用剩余长度做上界
	if int(n) > r.remaining()/4 {
		return p, fmt.Errorf("%w: 组件数 %d 超出剩余数据可能", errCCacheTruncated, n)
	}
	if p.Realm, err = r.countedString(); err != nil {
		return p, err
	}
	for i := uint32(0); i < n; i++ {
		c, err := r.countedString()
		if err != nil {
			return p, err
		}
		p.Components = append(p.Components, c)
	}
	return p, nil
}

// ccacheEpoch 把 ccache 的 32 位秒数转成 time.Time。
// 0 表示"未设置"（如 renew_till），保留为零值时间而不是 1970 年。
func ccacheEpoch(sec uint32) time.Time {
	if sec == 0 {
		return time.Time{}
	}
	return time.Unix(int64(sec), 0).UTC()
}

// ParseCCache 解析 ccache 数据（v3 / v4）。
func ParseCCache(data []byte) (*CCache, error) {
	if len(data) < 2 {
		return nil, errCCacheBadVersion
	}
	// 版本字节形如 05 03 / 05 04
	if data[0] != 0x05 {
		return nil, errCCacheBadVersion
	}
	version := int(data[1])
	if version < 1 || version > 4 {
		return nil, errCCacheBadVersion
	}
	if version == 1 || version == 2 {
		return nil, errCCacheLegacyVersion
	}

	r := &ccacheReader{b: data}
	// 跳过 2 字节版本
	if _, err := r.bytesN(2); err != nil {
		return nil, err
	}

	out := &CCache{Version: version}

	if version == 4 {
		// v4：版本(2) + headerlen(2)，随后 headerlen 字节的配置段。
		// 配置段里是 `krb5_ccache_conf_data/...` 这类条目，跳过即可 ——
		// 它们不是票据，混进凭据列表会让操作员看到一堆"看起来像票据"的垃圾。
		headerLen, err := r.u16()
		if err != nil {
			return nil, err
		}
		if int(headerLen) > r.remaining() {
			return nil, errCCacheTruncated
		}
		if _, err := r.bytesN(int(headerLen)); err != nil {
			return nil, err
		}
	}

	p, err := r.readPrincipal()
	if err != nil {
		return nil, err
	}
	out.Principal = p

	for r.remaining() > 0 {
		c, err := readCredential(r, version)
		if err != nil {
			return nil, err
		}
		// 跳过配置条目（v4 的 conf_data 会以"凭据"形态出现）
		if isConfData(c.Server) {
			continue
		}
		out.Credentials = append(out.Credentials, c)
	}
	return out, nil
}

// isConfData v4 把配置项也存成凭据形态，主体名里带 krb5_ccache_conf_data
func isConfData(p CCachePrincipal) bool {
	for _, c := range p.Components {
		if c == "krb5_ccache_conf_data" {
			return true
		}
	}
	return false
}

func readCredential(r *ccacheReader, version int) (CCacheCredential, error) {
	var c CCacheCredential
	var err error
	if c.Client, err = r.readPrincipal(); err != nil {
		return c, err
	}
	if c.Server, err = r.readPrincipal(); err != nil {
		return c, err
	}

	// 密钥块：v3 多一个重复的 etype 字段（4 字段），v4 是 3 字段。
	// 这是两个版本**结构上**的差异，不是冗余 —— 按同一套读会把后续字段全部错位。
	if c.KeyType, err = r.u16(); err != nil {
		return c, err
	}
	if _, err = r.u16(); err != nil { // etype（与 keytype 同值）
		return c, err
	}
	if version == 3 {
		if _, err = r.u16(); err != nil { // v3 独有的重复 etype
			return c, err
		}
	}
	// 注意：密钥长度是 **2 字节**（不是计数字符串的 4 字节长度）。
	// 用 countedBytes() 会多读 2 字节，后续所有字段整体错位，
	// 最终以"数据被截断"的形式报错 —— 症状离原因很远。
	keyLen, err := r.u16()
	if err != nil {
		return c, err
	}
	if c.KeyValue, err = r.bytesN(int(keyLen)); err != nil {
		return c, err
	}

	authTime, err := r.u32()
	if err != nil {
		return c, err
	}
	startTime, err := r.u32()
	if err != nil {
		return c, err
	}
	endTime, err := r.u32()
	if err != nil {
		return c, err
	}
	renewTill, err := r.u32()
	if err != nil {
		return c, err
	}
	c.AuthTime = ccacheEpoch(authTime)
	c.StartTime = ccacheEpoch(startTime)
	c.EndTime = ccacheEpoch(endTime)
	c.RenewTill = ccacheEpoch(renewTill)

	skey, err := r.u8()
	if err != nil {
		return c, err
	}
	c.IsSKey = skey != 0

	if c.TicketFlags, err = r.u32(); err != nil {
		return c, err
	}
	numAddr, err := r.u32()
	if err != nil {
		return c, err
	}
	if int(numAddr) > r.remaining()/6 {
		return c, fmt.Errorf("%w: 地址数 %d 超出剩余数据可能", errCCacheTruncated, numAddr)
	}
	for i := uint32(0); i < numAddr; i++ {
		if _, err := r.u16(); err != nil { // addrtype
			return c, err
		}
		if _, err := r.countedBytes(); err != nil { // addrdata
			return c, err
		}
	}

	numAuth, err := r.u32()
	if err != nil {
		return c, err
	}
	if int(numAuth) > r.remaining()/6 {
		return c, fmt.Errorf("%w: authdata 数 %d 超出剩余数据可能", errCCacheTruncated, numAuth)
	}
	for i := uint32(0); i < numAuth; i++ {
		if _, err := r.u16(); err != nil { // authtype
			return c, err
		}
		if _, err := r.countedBytes(); err != nil { // authdata
			return c, err
		}
	}

	if c.Ticket, err = r.countedBytes(); err != nil {
		return c, err
	}
	if c.SecondTicket, err = r.countedBytes(); err != nil {
		return c, err
	}
	return c, nil
}

// ParseCCacheFile 从文件读取并解析
func ParseCCacheFile(path string) (*CCache, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseCCache(data)
}
