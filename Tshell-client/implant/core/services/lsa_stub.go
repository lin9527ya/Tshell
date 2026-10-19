//go:build !windows

package services

import "errors"

// LSA 票据采集依赖 Windows 的 LsaCallAuthenticationPackage；其它平台只有文件式
// ccache（tech=files）可用。显式报错，不做静默降级。
func credDumpLSATickets(_ string) ([]byte, error) {
	return nil, errors.New("LSA 票据采集仅支持 Windows（其它平台请用 tech=files 采集 ccache 文件）")
}
