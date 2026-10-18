// Package collab 多操作员协作：在线状态与会话锁。
//
// 内存态实现（单实例部署的当前形态）：存在性是"最近一次已认证请求的时间"，
// 锁带自动过期，避免操作员掉线后永久占锁。多实例/水平扩展时换 DB 或 Redis，
// 接口保持不变（阶段三）。
package collab

import (
	"sort"
	"sync"
	"time"
)

// Operator 在线操作员
type Operator struct {
	Username string    `json:"username"`
	Role     string    `json:"role"`
	LastIP   string    `json:"last_ip"`
	LastSeen time.Time `json:"last_seen"`
}

// Lock 会话锁（同一 beacon 同一时刻只建议一个操作员操作）
type Lock struct {
	UID        string    `json:"uid"`
	Owner      string    `json:"owner"`
	Reason     string    `json:"reason,omitempty"`
	AcquiredAt time.Time `json:"acquired_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

const (
	// PresenceTTL 超过该时长没有任何已认证请求 → 视为离线
	PresenceTTL = 5 * time.Minute
	// DefaultLockTTL 会话锁默认有效期（到期自动释放）
	DefaultLockTTL = 30 * time.Minute
)

// Registry 在线状态与会话锁的注册表（并发安全）
type Registry struct {
	mu    sync.Mutex
	now   func() time.Time // 可注入时钟（测试用）
	ops   map[string]*Operator
	locks map[string]*Lock
}

// NewRegistry 创建注册表
func NewRegistry() *Registry {
	return &Registry{
		now:   time.Now,
		ops:   make(map[string]*Operator),
		locks: make(map[string]*Lock),
	}
}

// Global 全局注册表（main 启动后由中间件与 handler 使用）
var Global = NewRegistry()

// Touch 记录一次已认证请求：在线状态的事实来源（任一 API 请求都会续期）。
func (r *Registry) Touch(username, role, ip string) {
	if username == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	op, ok := r.ops[username]
	if !ok {
		op = &Operator{Username: username}
		r.ops[username] = op
	}
	op.Role = role
	op.LastIP = ip
	op.LastSeen = r.now()
}

// Online 返回在线操作员（按用户名排序）；同时惰性清理超时条目。
func (r *Registry) Online() []Operator {
	r.mu.Lock()
	defer r.mu.Unlock()
	cutoff := r.now().Add(-PresenceTTL)
	out := make([]Operator, 0, len(r.ops))
	for name, op := range r.ops {
		if op.LastSeen.Before(cutoff) {
			delete(r.ops, name)
			continue
		}
		out = append(out, *op)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Username < out[j].Username })
	return out
}

// Acquire 获取会话锁：无人持有 / 已过期 / 同一持有人 → 成功（同持有人为续期）；
// 否则返回 (当前锁, false, 持有人)。
func (r *Registry) Acquire(uid, owner, reason string) (Lock, bool, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	if l, ok := r.locks[uid]; ok && now.Before(l.ExpiresAt) {
		if l.Owner == owner {
			l.ExpiresAt = now.Add(DefaultLockTTL)
			l.Reason = reason
			return *l, true, owner
		}
		return *l, false, l.Owner
	}
	l := &Lock{UID: uid, Owner: owner, Reason: reason,
		AcquiredAt: now, ExpiresAt: now.Add(DefaultLockTTL)}
	r.locks[uid] = l
	return *l, true, owner
}

// Release 释放会话锁（仅持有人；已过期视为无人持有，返回 false）。
func (r *Registry) Release(uid, owner string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	l, ok := r.locks[uid]
	if !ok || !r.now().Before(l.ExpiresAt) || l.Owner != owner {
		return false
	}
	delete(r.locks, uid)
	return true
}

// Holder 返回当前持有人（无人持有/已过期 → 空串）。
func (r *Registry) Holder(uid string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	l, ok := r.locks[uid]
	if !ok || !r.now().Before(l.ExpiresAt) {
		return ""
	}
	return l.Owner
}

// Locks 列出未过期的会话锁（按 UID 排序），并惰性清理过期项。
func (r *Registry) Locks() []Lock {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	out := make([]Lock, 0, len(r.locks))
	for uid, l := range r.locks {
		if !now.Before(l.ExpiresAt) {
			delete(r.locks, uid)
			continue
		}
		out = append(out, *l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UID < out[j].UID })
	return out
}
