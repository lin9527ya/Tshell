package cred

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ===== Kerberos kirbi（KRB-CRED）解析与互转 =====
//
// kirbi 是 Windows 侧（LSA / Rubeus / mimikatz）交换票据的文件形态；
// ccache 是 MIT 工具链（kinit -c / impacket / Linux）的形态。
// 采集到的票据要能"给 Windows 侧 ptt 用"与"给 Linux 工具链用"，两者必须能互转。
//
// 结构（RFC 4120，IMPLICIT 标记；APPLICATION 22 = 0x76 包一层 SEQUENCE）：
//
//	KRB-CRED ::= [APPLICATION 22] SEQUENCE {
//	    pvno     [0] INTEGER (5),
//	    msg-type [1] INTEGER (22),
//	    tickets  [2] SEQUENCE OF Ticket,
//	    enc-part [3] EncryptedData   -- etype=0，cipher 就是**明文** DER 的 EncKrbCredPart
//	}
//	EncKrbCredPart ::= [APPLICATION 29] SEQUENCE {
//	    ticket-info [0] SEQUENCE OF KrbCredInfo
//	}
//	KrbCredInfo ::= SEQUENCE {
//	    key [0] EncryptionKey,  prealm [1] Realm OPTIONAL,  pname [2] PrincipalName OPTIONAL,
//	    flags [3] BIT STRING OPTIONAL,  authtime [4] … endtime [6] … renew-till [7] …,
//	    srealm [8] Realm OPTIONAL,  sname [9] PrincipalName OPTIONAL
//	}
//
// **一处需要更正的既有说法**：`docs/REMAINING_WORK.md` §1 写「kirbi→ccache 需要密钥解密
// enc-part」。对标准 kirbi 而言这不对 —— enc-part 的 etype 为 0，cipher 里存的就是
// 明文 DER（impacket 的 `fromKRBCRED` 直接 `decoder.decode(cipher, EncKrbCredPart())`，
// 不做任何解密）。实测夹具也印证：`ccache-v4-kirbi` 的 enc-part 是
// `a0 03 02 01 00`（etype=0）+ `a2 … 04 …`（明文 OCTET STRING）。
// 所以本文件两个方向都**不涉及任何密钥**，可以完整验证。
//
// 唯一仍需 Windows 的是**运行时接受度**（`kerberos::ptt` 是否认可产出的票据），
// 那是运行时行为，不是文件格式问题。

var (
	errKirbiNotKRBCred = errors.New("不是合法的 kirbi（缺少 APPLICATION 22 / KRB-CRED 头）")
	errKirbiTruncated  = errors.New("kirbi 数据被截断")
	errKirbiNoInfo     = errors.New("kirbi 里没有 ticket-info（空凭据）")
)

// ---- 最小 DER 读写 ----
//
// 不引第三方 ASN.1 库：需要的只是"按 tag 取子元素"和"拼 TLV"，
// 而 Kerberos 用的是 IMPLICIT 标记 + APPLICATION 包装，用 encoding/asn1 的
// 结构体标签表达既别扭又容易在可选字段上出错。手写一个够用的最小实现更可控，
// 也和 ccache.go 的手写解析风格一致。

const (
	derInteger         = 0x02
	derOctetString     = 0x04
	derBitString       = 0x03
	derGeneralString   = 0x1b
	derGeneralizedTime = 0x18
	derSequence        = 0x30

	appTicket     = 0x61 // [APPLICATION 1]  Ticket
	appKRBCred    = 0x76 // [APPLICATION 22] KRB-CRED
	appEncKrbCred = 0x7d // [APPLICATION 29] EncKrbCredPart
	ctxTagBase    = 0xa0 // 构造型 context tag [0]
	ctxPrimBase   = 0x80 // 原始型 context tag [0]
)

// derElem 一个已解析的 DER 元素
type derElem struct {
	Tag   byte
	Value []byte // 内容（不含 tag/len）
	Full  []byte // 含 tag/len 的完整字节（写回 tickets 时要原样用）
}

// derLen 读 DER 长度字段，返回长度与长度字段占用的字节数
func derLen(b []byte) (int, int, error) {
	if len(b) < 1 {
		return 0, 0, errKirbiTruncated
	}
	first := b[0]
	if first&0x80 == 0 {
		return int(first), 1, nil
	}
	n := int(first & 0x7f)
	if n == 0 || n > 4 {
		return 0, 0, fmt.Errorf("%w: 非法的 DER 长度字段", errKirbiTruncated)
	}
	if len(b) < 1+n {
		return 0, 0, errKirbiTruncated
	}
	length := 0
	for i := 0; i < n; i++ {
		length = length<<8 | int(b[1+i])
	}
	return length, 1 + n, nil
}

// derChildren 把一段 DER 内容切成子元素序列
func derChildren(b []byte) ([]derElem, error) {
	var out []derElem
	off := 0
	for off < len(b) {
		if off+1 >= len(b) {
			return nil, errKirbiTruncated
		}
		tag := b[off]
		length, lenBytes, err := derLen(b[off+1:])
		if err != nil {
			return nil, err
		}
		start := off + 1 + lenBytes
		end := start + length
		if end > len(b) || end < start {
			return nil, errKirbiTruncated
		}
		out = append(out, derElem{Tag: tag, Value: b[start:end], Full: b[off:end]})
		off = end
	}
	return out, nil
}

// findTag 取第一个 tag 匹配的子元素
func findTag(elems []derElem, tag byte) (derElem, bool) {
	for _, e := range elems {
		if e.Tag == tag {
			return e, true
		}
	}
	return derElem{}, false
}

// unwrapOne 取 EXPLICIT context tag 里包裹的唯一子元素。
//
// 这一层很容易漏：Kerberos 的 ASN.1 模块虽是 IMPLICIT TAGS，但 impacket 的
// asn1.py 对多数可选字段用了 explicitTag，于是实际字节是
// `a1 { 1b "REALM" }` 而不是 `81 "REALM"`。按原始型 tag 去找会直接找不到，
// 症状是"字段全丢"而不是报错 —— 所以下面每个读取函数都统一走这里。
func unwrapOne(e derElem) (derElem, bool) {
	kids, err := derChildren(e.Value)
	if err != nil || len(kids) != 1 {
		return derElem{}, false
	}
	return kids[0], true
}

// derTLV 拼一个 TLV
func derTLV(tag byte, content []byte) []byte {
	out := []byte{tag}
	n := len(content)
	switch {
	case n < 0x80:
		out = append(out, byte(n))
	case n < 0x100:
		out = append(out, 0x81, byte(n))
	case n < 0x10000:
		out = append(out, 0x82, byte(n>>8), byte(n))
	default:
		out = append(out, 0x83, byte(n>>16), byte(n>>8), byte(n))
	}
	return append(out, content...)
}

// derInt 编码一个小整数（Kerberos 里全是 32 位以内，够用）
func derInt(v int32) []byte {
	// 去掉多余符号位：正数最高位为 1 时要补一个 0x00
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], uint32(v))
	i := 0
	for i < 3 && buf[i] == 0 && buf[i+1]&0x80 == 0 {
		i++
	}
	return derTLV(derInteger, buf[i:])
}

// parseDERInt 读一个 DER INTEGER（可能带符号位补字节）
func parseDERInt(b []byte) int64 {
	if len(b) == 0 {
		return 0
	}
	var v int64
	for _, c := range b {
		v = v<<8 | int64(c)
	}
	return v
}

// ---- kirbi 解析 ----

// ParseKirbi 把 kirbi（KRB-CRED）解析成 CCache，便于与 ccache 走同一套登记逻辑。
//
// 解析不涉及任何密钥：enc-part.etype 为 0 时 cipher 就是明文 DER 的 EncKrbCredPart。
func ParseKirbi(data []byte) (*CCache, error) {
	top, err := derChildren(data)
	if err != nil {
		return nil, err
	}
	if len(top) != 1 || top[0].Tag != appKRBCred {
		return nil, errKirbiNotKRBCred
	}
	inner, err := derChildren(top[0].Value)
	if err != nil {
		return nil, err
	}
	if len(inner) < 1 || inner[0].Tag != derSequence {
		return nil, errKirbiNotKRBCred
	}
	fields, err := derChildren(inner[0].Value)
	if err != nil {
		return nil, err
	}

	// pvno 必须是 5
	if pvno, ok := findTag(fields, ctxTagBase|0); ok {
		kids, _ := derChildren(pvno.Value)
		if len(kids) > 0 && parseDERInt(kids[0].Value) != 5 {
			return nil, fmt.Errorf("%w: pvno 不是 5", errKirbiNotKRBCred)
		}
	}

	// tickets [2]：SEQUENCE OF Ticket，每张票据的 Full 字节原样保留
	ticketsElem, ok := findTag(fields, ctxTagBase|2)
	if !ok {
		return nil, fmt.Errorf("%w: 缺少 tickets [2]", errKirbiNotKRBCred)
	}
	// tickets [2] 是 EXPLICIT 包裹：a2 { 30 { 61 … } }
	// 注意先判空再索引 —— 直接 ticketSeq[0] 在畸形输入上会 panic，
	// 而票据文件来自不可信来源（对手上传的制品）。
	ticketWrap, err := derChildren(ticketsElem.Value)
	if err != nil {
		return nil, err
	}
	if len(ticketWrap) != 1 || ticketWrap[0].Tag != derSequence {
		return nil, fmt.Errorf("%w: tickets 不是 SEQUENCE", errKirbiNotKRBCred)
	}
	ticketList, err := derChildren(ticketWrap[0].Value)
	if err != nil {
		return nil, err
	}

	// enc-part [3] → cipher [2] → EncKrbCredPart（明文）
	encElem, ok := findTag(fields, ctxTagBase|3)
	if !ok {
		return nil, fmt.Errorf("%w: 缺少 enc-part [3]", errKirbiNotKRBCred)
	}
	// enc-part 的 [3] 是 **EXPLICIT** 包裹：a3 { 30 { a0{02}, a2{04} } }
	// 所以要先拆出那个 SEQUENCE 才能看到 etype/cipher
	encWrap, err := derChildren(encElem.Value)
	if err != nil {
		return nil, err
	}
	if len(encWrap) != 1 || encWrap[0].Tag != derSequence {
		return nil, fmt.Errorf("%w: enc-part 不是 SEQUENCE", errKirbiNotKRBCred)
	}
	encFields, err := derChildren(encWrap[0].Value)
	if err != nil {
		return nil, err
	}
	// etype 必须为 0（0 = 未加密，cipher 为明文 DER）
	if et, ok := findTag(encFields, ctxTagBase|0); ok {
		if inner, ok := unwrapOne(et); ok {
			if etype := parseDERInt(inner.Value); etype != 0 {
				return nil, fmt.Errorf("%w: enc-part.etype 为 %d，本实现只支持 0（明文 kirbi）",
					errKirbiNotKRBCred, etype)
			}
		}
	}
	cipherElem, ok := findTag(encFields, ctxTagBase|2)
	if !ok {
		return nil, fmt.Errorf("%w: 缺少 enc-part.cipher", errKirbiNotKRBCred)
	}
	cipherInner, ok := unwrapOne(cipherElem)
	if !ok || len(cipherInner.Value) < 2 {
		return nil, fmt.Errorf("%w: enc-part.cipher 内容为空", errKirbiNotKRBCred)
	}
	cipher := cipherInner.Value

	// cipher 是一个完整的 DER（APPLICATION 29）
	encKrbElems, err := derChildren(cipher)
	if err != nil {
		return nil, fmt.Errorf("解析 EncKrbCredPart 失败: %w", err)
	}
	if len(encKrbElems) != 1 || encKrbElems[0].Tag != appEncKrbCred {
		return nil, fmt.Errorf("%w: cipher 不是 APPLICATION 29（EncKrbCredPart）", errKirbiNotKRBCred)
	}
	encKrbSeq, err := derChildren(encKrbElems[0].Value)
	if err != nil {
		return nil, err
	}
	if len(encKrbSeq) < 1 || encKrbSeq[0].Tag != derSequence {
		return nil, errKirbiNotKRBCred
	}
	encKrbFields, err := derChildren(encKrbSeq[0].Value)
	if err != nil {
		return nil, err
	}
	infoElem, ok := findTag(encKrbFields, ctxTagBase|0)
	if !ok {
		return nil, errKirbiNoInfo
	}
	infoSeq, err := derChildren(infoElem.Value)
	if err != nil {
		return nil, err
	}
	if len(infoSeq) < 1 {
		return nil, errKirbiNoInfo
	}
	infos, err := derChildren(infoSeq[0].Value)
	if err != nil {
		return nil, err
	}
	if len(infos) == 0 {
		return nil, errKirbiNoInfo
	}

	out := &CCache{Version: 4}
	for i, info := range infos {
		cr, err := parseKrbCredInfo(info, i, ticketList)
		if err != nil {
			return nil, err
		}
		out.Credentials = append(out.Credentials, cr)
	}
	// 缓存主体取第一条凭据的 client（与 impacket 的 fromKRBCRED 一致）
	if len(out.Credentials) > 0 {
		out.Principal = out.Credentials[0].Client
	}
	return out, nil
}

// parseKrbCredInfo 解析一条 KrbCredInfo，并配上同序号的 Ticket
func parseKrbCredInfo(elem derElem, idx int, tickets []derElem) (CCacheCredential, error) {
	var c CCacheCredential
	fields, err := derChildren(elem.Value)
	if err != nil {
		return c, err
	}

	// key [0] → EXPLICIT { SEQUENCE { keytype [0], keyvalue [1] } }
	if keyElem, ok := findTag(fields, ctxTagBase|0); ok {
		if keySeq, ok := unwrapOne(keyElem); ok {
			if keyFields, err := derChildren(keySeq.Value); err == nil {
				if kt, ok := findTag(keyFields, ctxTagBase|0); ok {
					if inner, ok := unwrapOne(kt); ok {
						c.KeyType = uint16(parseDERInt(inner.Value))
					}
				}
				if kv, ok := findTag(keyFields, ctxTagBase|1); ok {
					if inner, ok := unwrapOne(kv); ok {
						c.KeyValue = append([]byte(nil), inner.Value...)
					}
				}
			}
		}
	}

	if p, ok := readRealm(fields, 1); ok {
		c.Client.Realm = p
	}
	if p, ok := readPrincipalName(fields, 2); ok {
		c.Client.Components = p.Components
		if p.NameType != 0 {
			c.Client.NameType = p.NameType
		}
	}
	if f, ok := readBitStringUint32(fields, 3); ok {
		c.TicketFlags = f
	}
	if t, ok := readGeneralizedTime(fields, 4); ok {
		c.AuthTime = t
	}
	if t, ok := readGeneralizedTime(fields, 5); ok {
		c.StartTime = t
	}
	if t, ok := readGeneralizedTime(fields, 6); ok {
		c.EndTime = t
	}
	if t, ok := readGeneralizedTime(fields, 7); ok {
		c.RenewTill = t
	}
	if r, ok := readRealm(fields, 8); ok {
		c.Server.Realm = r
	}
	if p, ok := readPrincipalName(fields, 9); ok {
		c.Server.Components = p.Components
		if p.NameType != 0 {
			c.Server.NameType = p.NameType
		}
	}
	// tickets 与 ticket-info 一一对应；数量不一致时宁可报错也不张冠李戴
	if idx < len(tickets) {
		c.Ticket = append([]byte(nil), tickets[idx].Full...)
	}
	return c, nil
}

func readRealm(fields []derElem, tag byte) (string, bool) {
	e, ok := findTag(fields, ctxTagBase|tag)
	if !ok {
		return "", false
	}
	inner, ok := unwrapOne(e)
	if !ok {
		return "", false
	}
	return string(inner.Value), true
}

func readPrincipalName(fields []derElem, tag byte) (CCachePrincipal, bool) {
	e, ok := findTag(fields, ctxTagBase|tag)
	if !ok {
		return CCachePrincipal{}, false
	}
	var p CCachePrincipal
	seq, ok := unwrapOne(e)
	if !ok {
		return p, false
	}
	inner, err := derChildren(seq.Value)
	if err != nil {
		return p, false
	}
	if nt, ok := findTag(inner, ctxTagBase|0); ok {
		if v, ok := unwrapOne(nt); ok {
			p.NameType = uint32(parseDERInt(v.Value))
		}
	}
	if ns, ok := findTag(inner, ctxTagBase|1); ok {
		seq, ok := unwrapOne(ns)
		if !ok {
			return p, true
		}
		comps, err := derChildren(seq.Value)
		if err != nil {
			return p, true
		}
		for _, c := range comps {
			p.Components = append(p.Components, string(c.Value))
		}
	}
	return p, true
}

// readBitStringUint32 读 32 位旗标（BIT STRING：首字节是"未使用位数"，通常为 0）
func readBitStringUint32(fields []derElem, tag byte) (uint32, bool) {
	e, ok := findTag(fields, ctxTagBase|tag)
	if !ok {
		return 0, false
	}
	inner, ok := unwrapOne(e)
	if !ok {
		return 0, false
	}
	v := inner.Value
	if len(v) < 2 {
		return 0, false
	}
	unused := int(v[0])
	bits := v[1:]
	var out uint32
	for _, b := range bits {
		out = out<<8 | uint32(b)
	}
	// 右移掉未使用位
	if unused > 0 && unused < 32 {
		out >>= uint(unused)
	}
	return out, true
}

// readGeneralizedTime 读 GeneralizedTime（YYYYMMDDHHMMSSZ）
func readGeneralizedTime(fields []derElem, tag byte) (time.Time, bool) {
	e, ok := findTag(fields, ctxTagBase|tag)
	if !ok {
		return time.Time{}, false
	}
	inner, ok := unwrapOne(e)
	if !ok {
		return time.Time{}, false
	}
	t, err := parseKerberosTime(string(inner.Value))
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// parseKerberosTime 解析 Kerberos 的 GeneralizedTime。
// 允许带/不带秒与小数秒（RFC 4120 要求 Z 结尾且不含小数，但现实里两种都见过）。
func parseKerberosTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, errors.New("空时间")
	}
	layouts := []string{
		"20060102150405Z",
		"200601021504Z",
		"2006010215Z",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("无法解析 Kerberos 时间 %q", s)
}

// ---- ccache → kirbi ----

// MarshalKirbi 把 CCache 编码成 kirbi（KRB-CRED，etype=0 明文 EncKrbCredPart）。
//
// 每条 credential 的 Ticket 本身就是一段完整的 DER Ticket，直接放进 tickets [2]。
func (c *CCache) MarshalKirbi() ([]byte, error) {
	if c == nil || len(c.Credentials) == 0 {
		return nil, errKirbiNoInfo
	}

	var tickets [][]byte
	var infos [][]byte
	for _, cr := range c.Credentials {
		if len(cr.Ticket) == 0 {
			continue
		}
		tickets = append(tickets, cr.Ticket)
		infos = append(infos, marshalKrbCredInfo(cr))
	}
	if len(infos) == 0 {
		return nil, errKirbiNoInfo
	}

	var ticketTLVs []byte
	for _, t := range tickets {
		ticketTLVs = append(ticketTLVs, t...)
	}
	ticketSeq := derTLV(derSequence, ticketTLVs)

	var infoTLVs []byte
	for _, i := range infos {
		infoTLVs = append(infoTLVs, i...)
	}
	encKrbPart := derTLV(appEncKrbCred, derTLV(derSequence,
		derTLV(ctxTagBase|0, derTLV(derSequence, infoTLVs))))

	encData := derTLV(derSequence,
		append(derTLV(ctxTagBase|0, derInt(0)), // etype = 0（明文）
			derTLV(ctxTagBase|2, derTLV(derOctetString, encKrbPart))...))

	body := derTLV(derSequence,
		append(append(
			derTLV(ctxTagBase|0, derInt(5)),      // pvno
			derTLV(ctxTagBase|1, derInt(22))...), // msg-type = KRB-CRED
			append(derTLV(ctxTagBase|2, ticketSeq), derTLV(ctxTagBase|3, encData)...)...))

	return derTLV(appKRBCred, body), nil
}

func marshalKrbCredInfo(cr CCacheCredential) []byte {
	var fields []byte

	// key [0] EncryptionKey
	keySeq := derTLV(derSequence,
		append(derTLV(ctxTagBase|0, derInt(int32(cr.KeyType))),
			derTLV(ctxTagBase|1, derTLV(derOctetString, cr.KeyValue))...))
	fields = append(fields, derTLV(ctxTagBase|0, keySeq)...)

	// prealm [1]：**EXPLICIT** —— a1 { 1b "REALM" }。
	// 写成隐式（81 "REALM"）时自家解析器会找不到字段：不报错、只是静默丢失，
	// 产出"能解析但主体为空"的票据 —— 这类错误靠往返测试才抓得住。
	if cr.Client.Realm != "" {
		fields = append(fields, derTLV(ctxTagBase|1, derTLV(derGeneralString, []byte(cr.Client.Realm)))...)
	}
	// pname [2]：a2 { 30 { … } }
	if len(cr.Client.Components) > 0 {
		fields = append(fields, derTLV(ctxTagBase|2, marshalPrincipalName(cr.Client))...)
	}
	// flags [3]：a3 { 03 <unused+4 字节> }
	fields = append(fields, derTLV(ctxTagBase|3, derTLV(derBitString, marshalFlags(cr.TicketFlags)))...)

	// 时间 [4]..[7]：a4 { 18 "…Z" }
	for tag, t := range map[byte]time.Time{
		4: cr.AuthTime, 5: cr.StartTime, 6: cr.EndTime, 7: cr.RenewTill,
	} {
		if !t.IsZero() {
			fields = append(fields, derTLV(ctxTagBase|tag, derTLV(derGeneralizedTime, []byte(formatKerberosTime(t))))...)
		}
	}

	// srealm [8]：a8 { 1b "REALM" }；sname [9]：a9 { 30 { … } }
	if cr.Server.Realm != "" {
		fields = append(fields, derTLV(ctxTagBase|8, derTLV(derGeneralString, []byte(cr.Server.Realm)))...)
	}
	if len(cr.Server.Components) > 0 {
		fields = append(fields, derTLV(ctxTagBase|9, marshalPrincipalName(cr.Server))...)
	}

	// 时间字段是按 map 遍历写入的，顺序不固定 —— DER 的 SET 才要求有序，
	// SEQUENCE 里 context tag 的顺序在解析侧按 tag 查找，功能上无碍；
	// 但为了让输出稳定可复现，下面统一按 tag 排序一次。
	return derTLV(derSequence, sortTLVsByTag(fields))
}

// sortTLVsByTag 把一串 context-tag TLV 按 tag 升序重排，保证输出稳定。
// 解析侧按 tag 查找，不依赖顺序；这里排序只为"同样输入产出同样字节"。
func sortTLVsByTag(b []byte) []byte {
	elems, err := derChildren(b)
	if err != nil {
		return b
	}
	sort.SliceStable(elems, func(i, j int) bool { return elems[i].Tag < elems[j].Tag })
	var out []byte
	for _, e := range elems {
		out = append(out, e.Full...)
	}
	return out
}

func marshalPrincipalName(p CCachePrincipal) []byte {
	var comps []byte
	for _, c := range p.Components {
		comps = append(comps, derTLV(derGeneralString, []byte(c))...)
	}
	// 注意不要重复套 SEQUENCE：inner 本身已经是 SEQUENCE 的 TLV，
	// 再包一层会变成 `30 { 30 { a0, a1 } }`，解析侧按一层拆就找不到组件 ——
	// 症状是"realm 在、用户名没了"，很容易误判成编码问题。
	return derTLV(derSequence, append(
		derTLV(ctxTagBase|0, derInt(int32(p.NameType))),
		derTLV(ctxTagBase|1, derTLV(derSequence, comps))...))
}

// marshalFlags 32 位旗标 → BIT STRING 内容（未使用位数 0 + 4 字节）
func marshalFlags(flags uint32) []byte {
	out := []byte{0x00}
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], flags)
	return append(out, buf[:]...)
}

func formatKerberosTime(t time.Time) string {
	return t.UTC().Format("20060102150405Z")
}

// ---- ccache 序列化（kirbi→ccache 的输出形态）----

// Marshal 把 CCache 编码成 ccache v4 字节。
//
// 只有 v4：它是当前唯一还在产出的版本，v3 已属历史；
// 让"导出 ccache"固定产出 v4 也避免了调用方去选版本。
func (c *CCache) Marshal() ([]byte, error) {
	if c == nil {
		return nil, errors.New("ccache 为空")
	}
	var buf bytes.Buffer
	// 版本 0x0504
	buf.Write([]byte{0x05, 0x04})
	// v4 头部：headerlen(2) + 配置段。这里不写配置段（长度 0），
	// 与"只关心票据"的用途一致；MIT 工具链接受空配置段。
	buf.Write([]byte{0x00, 0x00})

	writePrincipal(&buf, c.Principal)
	for _, cr := range c.Credentials {
		writePrincipal(&buf, cr.Client)
		writePrincipal(&buf, cr.Server)
		// v4 密钥块：keytype(2) + etype(2) + keylen(2) + keyvalue
		writeU16(&buf, cr.KeyType)
		writeU16(&buf, cr.KeyType) // etype，与 keytype 同值
		writeU16(&buf, uint16(len(cr.KeyValue)))
		buf.Write(cr.KeyValue)
		// 时间 4×u32
		writeU32(&buf, unix32(cr.AuthTime))
		writeU32(&buf, unix32(cr.StartTime))
		writeU32(&buf, unix32(cr.EndTime))
		writeU32(&buf, unix32(cr.RenewTill))
		// is_skey(1) + tktflags(4) + num_address(4)
		if cr.IsSKey {
			buf.WriteByte(1)
		} else {
			buf.WriteByte(0)
		}
		writeU32(&buf, cr.TicketFlags)
		writeU32(&buf, 0) // 地址表留空（不导出客户端地址）
		writeU32(&buf, 0) // authdata 留空
		writeCounted(&buf, cr.Ticket)
		writeCounted(&buf, cr.SecondTicket)
	}
	return buf.Bytes(), nil
}

func writePrincipal(buf *bytes.Buffer, p CCachePrincipal) {
	writeU32(buf, p.NameType)
	writeU32(buf, uint32(len(p.Components)))
	writeCounted(buf, []byte(p.Realm))
	for _, c := range p.Components {
		writeCounted(buf, []byte(c))
	}
}

func writeCounted(buf *bytes.Buffer, b []byte) {
	writeU32(buf, uint32(len(b)))
	buf.Write(b)
}

func writeU16(buf *bytes.Buffer, v uint16) {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], v)
	buf.Write(b[:])
}

func writeU32(buf *bytes.Buffer, v uint32) {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	buf.Write(b[:])
}

// unix32 ccache 用 32 位秒数；零值时间编码为 0（表示"未设置"）
func unix32(t time.Time) uint32 {
	if t.IsZero() {
		return 0
	}
	s := t.Unix()
	if s < 0 {
		return 0
	}
	return uint32(s)
}

// 供测试与调试：把 kirbi 的 ticket 片段转成 hex 便于比对
var _ = hex.EncodeToString
