package main

// kxMsgType 会话前 ECDH 交换的帧类型（tcp / kcp 共用）。
//
// 放在**无 build tag** 的文件里：这些传输各自在带 tag 的文件中引用同一个值，
// 若各自定义一份，transport_multi（同时编入所有传输）会因重复声明而编译失败。
//
// 取值必须与服务端 connection/tcp.MsgTypeKX、connection/kcp.MsgTypeKX 一致（200）。
const kxMsgType = 200
