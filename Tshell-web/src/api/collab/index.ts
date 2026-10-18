import request from '@/utils/request'

// 多操作员协作：在线状态、会话锁、操作广播（广播走 /api/events/ws，不在这里）
export interface CollabOperator {
  username: string
  role: string
  last_ip: string
  last_seen: string
}

export interface SessionLock {
  uid: string
  owner: string
  reason?: string
  acquired_at: string
  expires_at: string
}

export default {
  listOperators() {
    return request({ url: '/collab/operators', method: 'GET' })
  },
  listLocks() {
    return request({ url: '/collab/locks', method: 'GET' })
  },
  acquireLock(uid: string, reason = '') {
    return request({ url: `/collab/locks/${encodeURIComponent(uid)}`, method: 'POST', data: { reason } })
  },
  releaseLock(uid: string) {
    return request({ url: `/collab/locks/${encodeURIComponent(uid)}`, method: 'DELETE' })
  }
}
