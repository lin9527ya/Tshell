package communication

import (
	"encoding/base64"
	"net/http"
	"time"

	"BackendTemplate/pkg/encrypt"
	"BackendTemplate/pkg/utils/logx"
)

// ===== 会话前 ECDH 密钥交换（HTTP/HTTPS 家族）=====
//
// 位置很关键：这段必须在 **MetaInfo 解密之前** 跑完，因为 `metaMK` 同时用于
// ① 解 MetaInfo、② 派生会话密钥。把它换掉，下游（RegisterClient、nonce 唯一化、
// 命令加解密）全部自动跟随，一行都不用改。
//
// 协议（见 docs/REMAINING_WORK.md §3.3）：
//   请求头 X-Kx: base64(clientPub)
//   响应头 X-Kx: base64(serverPub)
// 公钥是公开信息，走明文头没有问题；**但首包不带任何敏感载荷** ——
// 客户端在拿到 serverPub、派生完主密钥之后才生成 MetaInfo。
// 否则静态密钥仍能解出 MetaInfo，改造就只剩表面效果。
//
// 向后兼容：没有 X-Kx 头 → 返回 nil，调用方走原有静态密钥路径，
// 行为与改造前完全一致。所以老载荷不会因为这次改动失联。

// KxCache 全局握手缓存。TTL 5 分钟而不是 30 秒：
// 客户端的 FirstBlood 带指数退避（最长 31s），30s 会在重试途中过期。
// 5 分钟仍是有界的，且条目数量受容量上限约束。
var KxCache = encrypt.NewKxCache(5*time.Minute, 8192)

// ResolveRequestKey 解析本次请求应使用的主密钥。
//
// 返回值约定：
//   - 返回 nil, false, nil  ：没有 X-Kx，调用方走静态密钥（老行为）
//   - 返回 mk, true, nil    ：用 mk 作 metaMK（可能是新交换的，也可能命中缓存）
//   - 返回 nil, true, err   ：请求声明了 ECDH 但交换失败 —— 必须拒绝，
//     不能"退回静态密钥"，否则攻击者只要发个坏公钥就能把连接降级
//
// 还会把服务端公钥写进响应头（每次请求都写，以便客户端自愈重派生）。
func ResolveRequestKey(w http.ResponseWriter, r *http.Request, listenerSalt []byte) ([]byte, bool, error) {
	kxHeader := r.Header.Get(encrypt.KxHeaderName)
	if kxHeader == "" {
		return nil, false, nil
	}

	peerPub, err := encrypt.ParsePeerPublicKey(kxHeader)
	if err != nil {
		logx.Warnf("⚠️ [KX] 客户端公钥不可用，拒绝连接: %v\n", err)
		return nil, true, err
	}

	// 命中缓存：直接把存下的服务端公钥回给客户端，让它能重新派生（自愈）
	if mk, serverPub, ok := KxCache.GetExchange(peerPub); ok {
		if len(serverPub) > 0 {
			w.Header().Set(encrypt.KxHeaderName, base64.StdEncoding.EncodeToString(serverPub))
		}
		return mk, true, nil
	}

	// 首次交换：生成服务端临时密钥对并派生
	srv, err := encrypt.NewServerKeyExchange()
	if err != nil {
		return nil, true, err
	}
	mk, err := srv.DeriveMasterKey(peerPub, listenerSalt)
	if err != nil {
		// 公钥非法/低阶点：拒绝，不降级
		logx.Warnf("⚠️ [KX] 与客户端交换密钥失败，拒绝连接: %v\n", err)
		return nil, true, err
	}
	if _, err := KxCache.PutExchange(peerPub, srv.PublicKey(), mk); err != nil {
		return nil, true, err
	}
	w.Header().Set(encrypt.KxHeaderName, srv.HeaderValue())
	logx.Infof("🔑 [KX] 完成一次会话前密钥交换（缓存 %d 条）\n", KxCache.Len())
	return mk, true, nil
}

// BindExchangeUID 首条有效消息解出 uid 后，把交换条目绑定到该 uid。
// 绑定只用于观测/重复握手检测，不改变 TTL 语义。
func BindExchangeUID(r *http.Request, uid string) {
	if uid == "" {
		return
	}
	if peerPub, err := encrypt.ParsePeerPublicKey(r.Header.Get(encrypt.KxHeaderName)); err == nil {
		KxCache.Bind(peerPub, uid)
	}
}

// StartKxCacheSweeper 后台清理过期握手条目，返回停止函数。
// 不清理也能靠 Get 时的惰性判断兜住，但条目会一直占着 map 内存。
func StartKxCacheSweeper() func() {
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				if n := KxCache.Sweep(); n > 0 {
					logx.Infof("🧹 [KX] 清理 %d 条过期握手缓存\n", n)
				}
			case <-stop:
				return
			}
		}
	}()
	return func() { close(stop) }
}
