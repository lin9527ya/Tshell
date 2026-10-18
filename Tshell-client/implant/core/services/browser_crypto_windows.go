//go:build windows

package services

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// cryptUnprotect 调用 DPAPI 解一段当前用户作用域的数据（CryptUnprotectData）。
//
// 这是 dllhost/lsass 之外**唯一**能拿到用户主密钥保护数据的正规途径：
// Chrome/Edge 的浏览器主密钥（Local State 里的 os_crypt.encrypted_key）
// 就是用当前用户的 DPAPI 主密钥保护的，只能在同一用户上下文里解开。
//
// 做成独立函数（而不是内联在采集流程里）是为了：错误语义集中（调用方只需知道
// "这段数据解不开"），以及方便以后加 SYSTEM 上下文下的机器作用域变体。
func cryptUnprotect(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("待解密数据为空")
	}
	in := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, 0, &out); err != nil {
		return nil, fmt.Errorf("CryptUnprotectData 失败: %w", err)
	}
	if out.Data == nil || out.Size == 0 {
		return nil, errors.New("CryptUnprotectData 返回空数据")
	}
	// out.Data 是 LocalAlloc 出来的，必须自己释放
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	buf := unsafe.Slice(out.Data, out.Size)
	res := make([]byte, len(buf))
	copy(res, buf)
	return res, nil
}
