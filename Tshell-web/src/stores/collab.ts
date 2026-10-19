import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import CollabAPI, { type CollabOperator, type SessionLock } from '@/api/collab'
import { useUserStore } from '@/stores/user'

const POLL_INTERVAL = 30000

// 多操作员协作状态：在线列表 + 会话锁。
// 服务端在每次已认证请求与事件流连接上续期在线状态，这里只负责展示与操作。
export const useCollabStore = defineStore('collab', () => {
  const operators = ref<CollabOperator[]>([])
  const locks = ref<SessionLock[]>([])
  const loaded = ref(false)
  let timer: ReturnType<typeof setInterval> | null = null
  let refCount = 0

  const lockByUid = computed(() => {
    const map: Record<string, SessionLock> = {}
    for (const l of locks.value) map[l.uid] = l
    return map
  })

  const lockOf = (uid: string): SessionLock | undefined => lockByUid.value[uid]

  // 被他人占用（自己持有不算占用，前端据此提示/置灰）
  const lockedByOther = (uid: string): SessionLock | undefined => {
    const l = lockByUid.value[uid]
    if (!l) return undefined
    return l.owner === useUserStore().username ? undefined : l
  }

  // 统一解包列表：request 拦截器返回的是完整 axios 响应（res.data 才是响应体），
  // 后端契约 {status, data:[...]}，故列表在 res.data.data；同时兼容直接返回数组的形态。
  const unwrapList = <T,>(res: any): T[] => {
    const body = res?.data
    if (Array.isArray(body)) return body as T[]
    if (Array.isArray(body?.data)) return body.data as T[]
    return []
  }

  const refresh = async () => {
    try {
      const [ops, lks] = await Promise.all([CollabAPI.listOperators(), CollabAPI.listLocks()])
      operators.value = unwrapList<CollabOperator>(ops)
      locks.value = unwrapList<SessionLock>(lks)
      loaded.value = true
    } catch {
      // 协作信息是辅助信息，失败不影响主流程（下一轮重试）
    }
  }

  const acquire = async (uid: string, reason = '') => {
    await CollabAPI.acquireLock(uid, reason)
    await refresh()
  }

  const release = async (uid: string) => {
    await CollabAPI.releaseLock(uid)
    await refresh()
  }

  const start = () => {
    refCount++
    if (refCount > 1) return
    refresh()
    timer = setInterval(refresh, POLL_INTERVAL)
  }

  const stop = () => {
    refCount = Math.max(0, refCount - 1)
    if (refCount > 0) return
    if (timer) { clearInterval(timer); timer = null }
  }

  return { operators, locks, loaded, lockByUid, lockOf, lockedByOther, refresh, acquire, release, start, stop }
})
