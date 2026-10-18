import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

// 协作 API 打桩（响应形状与后端 pkg/api/collab.go 契约一致：{status, data}）
const listOperators = vi.fn()
const listLocks = vi.fn()
const acquireLock = vi.fn()
const releaseLock = vi.fn()

vi.mock('@/api/collab', () => ({
  default: {
    listOperators: (...a: unknown[]) => listOperators(...a),
    listLocks: (...a: unknown[]) => listLocks(...a),
    acquireLock: (...a: unknown[]) => acquireLock(...a),
    releaseLock: (...a: unknown[]) => releaseLock(...a),
  },
}))

vi.mock('@/stores/user', () => ({
  useUserStore: () => ({ username: 'alice' }),
}))

import { useCollabStore } from './collab'

const lock = (uid: string, owner: string) => ({
  uid,
  owner,
  acquired_at: '2026-10-18T10:00:00Z',
  expires_at: '2026-10-18T10:30:00Z',
})

describe('collab store（多操作员协作）', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    listOperators.mockReset().mockResolvedValue({ data: [] })
    listLocks.mockReset().mockResolvedValue({ data: [] })
    acquireLock.mockReset().mockResolvedValue({ data: {} })
    releaseLock.mockReset().mockResolvedValue({ data: {} })
  })

  it('refresh 拉取在线列表与会话锁', async () => {
    listOperators.mockResolvedValue({
      data: [{ username: 'alice', role: 'admin', last_ip: '10.0.0.1', last_seen: '' },
             { username: 'bob', role: 'operator', last_ip: '10.0.0.2', last_seen: '' }],
    })
    listLocks.mockResolvedValue({ data: [lock('uid-1', 'bob')] })

    const store = useCollabStore()
    await store.refresh()

    expect(store.operators.map((o) => o.username)).toEqual(['alice', 'bob'])
    expect(store.lockOf('uid-1')?.owner).toBe('bob')
  })

  it('lockedByOther 只把「他人持有」算作占用', async () => {
    listLocks.mockResolvedValue({ data: [lock('uid-1', 'bob'), lock('uid-2', 'alice')] })
    const store = useCollabStore()
    await store.refresh()

    expect(store.lockedByOther('uid-1')?.owner).toBe('bob') // 别人占用 → 提示
    expect(store.lockedByOther('uid-2')).toBeUndefined()    // 自己占用 → 不算
    expect(store.lockedByOther('uid-9')).toBeUndefined()    // 无锁
  })

  it('接口失败不抛出（协作信息是辅助信息，不能拖垮会话列表）', async () => {
    listOperators.mockRejectedValue(new Error('boom'))
    listLocks.mockRejectedValue(new Error('boom'))
    const store = useCollabStore()

    await expect(store.refresh()).resolves.toBeUndefined()
    expect(store.loaded).toBe(false)
  })

  it('acquire/release 后立即刷新占用状态', async () => {
    const store = useCollabStore()
    listLocks.mockResolvedValue({ data: [lock('uid-1', 'alice')] })
    await store.acquire('uid-1', '排查')
    expect(acquireLock).toHaveBeenCalledWith('uid-1', '排查')
    expect(store.lockOf('uid-1')?.owner).toBe('alice')

    listLocks.mockResolvedValue({ data: [] })
    await store.release('uid-1')
    expect(releaseLock).toHaveBeenCalledWith('uid-1')
    expect(store.lockOf('uid-1')).toBeUndefined()
  })
})
