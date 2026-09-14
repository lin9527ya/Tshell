package encrypt

import (
	"sync"
)

// PendingNonce 待下发的会话密钥协商消息状态。
// 轮询协议（HTTP/OSS/DNS）在 firstBlood 派生成功后立即重派生会话密钥并暂存
// （RotateSessionKeyWithNonce 返回的初始密钥 + nonce），
// 待客户端下次轮询拉取命令时用初始密钥加密下发（命令队列之后的所有消息用新密钥）。
type PendingNonce struct {
	InitialKey []byte // 初始会话密钥（用于加密 nonce 下发消息）
	Nonce      []byte // 每连接随机 nonce（16 字节）
}

var (
	pendingNonceMu    sync.Mutex
	pendingNonceStore = make(map[string]*PendingNonce)
)

// StorePendingNonce 暂存待下发的 nonce 协商消息
func StorePendingNonce(uid string, p *PendingNonce) {
	pendingNonceMu.Lock()
	defer pendingNonceMu.Unlock()
	pendingNonceStore[uid] = p
}

// HasPendingNonce 是否存在未下发的 nonce 协商消息（防重复旋转重派生）
func HasPendingNonce(uid string) bool {
	pendingNonceMu.Lock()
	defer pendingNonceMu.Unlock()
	_, ok := pendingNonceStore[uid]
	return ok
}

// TakePendingNonce 取出待下发的 nonce 协商消息（幂等：取出后即清除）
func TakePendingNonce(uid string) *PendingNonce {
	pendingNonceMu.Lock()
	defer pendingNonceMu.Unlock()
	p, ok := pendingNonceStore[uid]
	if !ok {
		return nil
	}
	delete(pendingNonceStore, uid)
	return p
}
