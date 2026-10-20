package encrypt

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// ===== 握手期临时主密钥缓存（ECDH 阶段一 1b 的共用前置件）=====
//
// 为什么需要它：ECDH 交换发生在 beacon **首次上线**时，而此刻服务端还不知道
// 这个 beacon 的 uid（uid 在加密的 MetaInfo 里）。所以握手结果不能按 uid 存，
// 只能先按"客户端临时公钥"存一个短命条目，等首条有效消息解出 uid 后再绑定。
//
// 三条设计约束（都来自踩过的坑）：
//
//  1. **必须有 TTL**：握手是未认证请求，任何人都能打进来塞条目。
//     没有 TTL 就是一个可以无限撑爆内存的入口。
//  2. **必须有容量上限**：TTL 到期前仍可被灌满，所以再加一层 LRU 淘汰。
//  3. **Get 后不删除**：同一会话的后续请求仍要用这把密钥；
//     删除时机由 TTL/绑定决定，不由读取次数决定。
//     （早期设想"取一次就删"，会让并发请求里第二个直接失败。）
//
// 安全边界要写清：缓存里存的是**派生出的主密钥**，是本进程里最高价值的材料之一。
// 所以有条目过期/淘汰时必须**清零**，而不是交给 GC 等它哪天被回收。

// ErrKxCacheFull 容量耗尽且无可淘汰项（理论上不会发生，除非 max<=0）
var ErrKxCacheFull = errors.New("握手缓存已满")

// KxEntry 一次握手的结果
type KxEntry struct {
	// MasterKey 由 ECDH 共享密钥派生的主密钥（32B）
	MasterKey []byte
	// BeaconUID 首条有效消息解出后绑定；未绑定为空
	BeaconUID string
	// ServerPub 本次交换里服务端那一半的公钥。
	//
	// 为什么要存：客户端可能在第一次 GET 与携带 MetaInfo 的 GET 之间重试
	// （退避最长 30s+），若其间条目过期、服务端重建了密钥对，
	// 客户端手里的主密钥就与新的不匹配 → MetaInfo 解密失败。
	// 所以服务端每次都把公钥回给客户端，客户端每次都用它**重新派生**（自愈）；
	// 而要做到"每次都能回"，公钥就必须留在缓存里。
	ServerPub []byte
	// CreatedAt 入缓存时间
	CreatedAt time.Time

	expiresAt time.Time
}

// KxCache 握手期临时主密钥缓存
type KxCache struct {
	mu      sync.Mutex
	entries map[string]*KxEntry
	order   []string // 插入顺序，用于超容量时淘汰最旧的
	ttl     time.Duration
	max     int
	now     func() time.Time // 便于测试注入时钟
}

// NewKxCache 创建缓存。ttl<=0 用默认 30s，max<=0 用默认 4096。
//
// 默认 30s 的依据：正常网络下 beacon 从握手到首条消息应在数秒内完成；
// 给到 30s 是为了容忍绕行/重传，再长就只是在给攻击者攒内存的时间。
func NewKxCache(ttl time.Duration, max int) *KxCache {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	if max <= 0 {
		max = 4096
	}
	return &KxCache{
		entries: make(map[string]*KxEntry),
		ttl:     ttl,
		max:     max,
		now:     time.Now,
	}
}

// kxKey 由客户端临时公钥算缓存键。
//
// 用公钥本身（而非随机 token）作键的好处：客户端在握手后的每个请求里
// 重复带上同一个公钥头即可命中，不需要额外协商一个"句柄"；
// 而公钥本来就是公开信息，重复出现不泄露任何东西。
func kxKey(clientPub []byte) string {
	sum := sha256.Sum256(clientPub)
	return hex.EncodeToString(sum[:])
}

// Put 存入一次握手结果（会复制并接管 masterKey 的所有权语义：
// 调用方不应再持有/复用它）。
func (c *KxCache) Put(clientPub, masterKey []byte) (*KxEntry, error) {
	if c == nil {
		return nil, errors.New("握手缓存未初始化")
	}
	if len(clientPub) == 0 || len(masterKey) < ECDHSharedKeySize {
		return nil, errors.New("握手缓存的参数不合法")
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	c.sweepLocked(now)

	key := kxKey(clientPub)
	// 同一公钥重复握手：覆盖（幂等），避免同一会话攒出多个条目
	if old, ok := c.entries[key]; ok {
		wipe(old.MasterKey)
		delete(c.entries, key)
	}

	if len(c.entries) >= c.max {
		if !c.evictOldestLocked() {
			return nil, ErrKxCacheFull
		}
	}

	mk := make([]byte, ECDHSharedKeySize)
	copy(mk, masterKey[:ECDHSharedKeySize])
	e := &KxEntry{MasterKey: mk, CreatedAt: now, expiresAt: now.Add(c.ttl)}
	c.entries[key] = e
	c.order = append(c.order, key)
	return e, nil
}

// PutExchange 存入一次完整交换（含服务端公钥）。
// 与 Put 的区别：Put 只存派生结果，适合"不需要再回公钥"的场景。
func (c *KxCache) PutExchange(clientPub, serverPub, masterKey []byte) (*KxEntry, error) {
	e, err := c.Put(clientPub, masterKey)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	e.ServerPub = append([]byte{}, serverPub...)
	c.mu.Unlock()
	return e, nil
}

// GetExchange 取派生结果与服务端公钥（都返回副本）。
func (c *KxCache) GetExchange(clientPub []byte) (masterKey, serverPub []byte, ok bool) {
	if c == nil {
		return nil, nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	key := kxKey(clientPub)
	e, exists := c.entries[key]
	if !exists {
		return nil, nil, false
	}
	if !c.now().Before(e.expiresAt) {
		wipe(e.MasterKey)
		delete(c.entries, key)
		c.removeFromOrderLocked(key)
		return nil, nil, false
	}
	return append([]byte{}, e.MasterKey...), append([]byte{}, e.ServerPub...), true
}

// Get 按客户端公钥取握手结果（不删除，见文件头说明）。
// 返回的是**副本**：调用方拿去用不该能污染缓存里的那那份。
func (c *KxCache) Get(clientPub []byte) ([]byte, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	key := kxKey(clientPub)
	e, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if !c.now().Before(e.expiresAt) {
		// 过期：清零并移除，不返回
		wipe(e.MasterKey)
		delete(c.entries, key)
		c.removeFromOrderLocked(key)
		return nil, false
	}
	return append([]byte{}, e.MasterKey...), true
}

// Bind 把已建立的握手条目绑定到 beacon uid（首条有效消息解出 uid 时调用）。
// 绑定后不改变 TTL 语义：条目仍是短命的，真正长期用的是会话密钥。
func (c *KxCache) Bind(clientPub []byte, uid string) bool {
	if c == nil || uid == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[kxKey(clientPub)]
	if !ok {
		return false
	}
	e.BeaconUID = uid
	return true
}

// LookupByUID 反查该 uid 是否已有握手条目（供"重复握手"检测）
func (c *KxCache) LookupByUID(uid string) ([]byte, bool) {
	if c == nil || uid == "" {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	for _, e := range c.entries {
		if e.BeaconUID == uid && now.Before(e.expiresAt) {
			return append([]byte{}, e.MasterKey...), true
		}
	}
	return nil, false
}

// Sweep 清理过期条目，返回清理数量（供后台定时调用）
func (c *KxCache) Sweep() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sweepLocked(c.now())
}

func (c *KxCache) sweepLocked(now time.Time) int {
	n := 0
	for k, e := range c.entries {
		if !now.Before(e.expiresAt) {
			wipe(e.MasterKey)
			delete(c.entries, k)
			c.removeFromOrderLocked(k)
			n++
		}
	}
	return n
}

func (c *KxCache) evictOldestLocked() bool {
	for len(c.order) > 0 {
		k := c.order[0]
		c.order = c.order[1:]
		if e, ok := c.entries[k]; ok {
			wipe(e.MasterKey)
			delete(c.entries, k)
			return true
		}
	}
	return false
}

func (c *KxCache) removeFromOrderLocked(key string) {
	for i, k := range c.order {
		if k == key {
			c.order = append(c.order[:i], c.order[i+1:]...)
			return
		}
	}
}

// Len 当前条目数（观测/测试用）
func (c *KxCache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// Close 清空并清零全部条目（进程退出/测试清理用）
func (c *KxCache) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.entries {
		wipe(e.MasterKey)
	}
	c.entries = make(map[string]*KxEntry)
	c.order = nil
}

// SetClock 注入时钟（仅测试用）
func (c *KxCache) SetClock(now func() time.Time) {
	if c == nil || now == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = now
}

// GlobalKxCache 进程内共享的会话前握手缓存。
//
// 放在这里而不是各传输包里：HTTP 家族、TCP、KCP、WS、DNS 都要用同一套
// "按客户端公钥/uid 存派生主密钥"的能力，各自建一份会重复且难以统一 TTL。
// 条目以客户端临时公钥的哈希为键，天然按握手隔离，不存在跨传输串用。
//
// TTL 取 5 分钟：客户端的首次注册可能有指数退避重试（最长约 31s），
// 30 秒会在重试途中过期，导致服务端重建密钥对、客户端手里那把随之失效。
var GlobalKxCache = NewKxCache(5*time.Minute, 8192)
