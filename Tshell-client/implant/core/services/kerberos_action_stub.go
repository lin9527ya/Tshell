//go:build !windows

package services

import "errors"

// runKerberosAction 非 Windows 没有 LSA：动作不可用。
// 文件式 ccache 仍然可以采集（kind=ticket, tech=files），只是不能 list/purge/ptt。
func runKerberosAction(_ kerberosAction) ([]byte, error) {
	return nil, errors.New("Kerberos 票据动作（list/purge/ptt）仅支持 Windows；其它平台可用 kind=ticket, tech=files 采集 ccache")
}
