import { defineStore } from 'pinia'
import { ref } from 'vue'
import { useUserStore } from '@/stores/user'
import { useClientsStore } from '@/stores/clients'
import { usePreferencesStore } from '@/stores/preferences'
import { playOnlineChime } from '@/utils/notifyAudio'

// 事件流（B11）：单一 WebSocket 连接源。
// 原实现多个页面与 EventCenter 各维持一条 /api/events/ws 连接，事件重复接收；
// 统一到本 store 后，所有组件通过 events/connected 订阅同一数据源。
export interface C2Event {
  type: string
  uid?: string
  timestamp?: number
  detail?: Record<string, any>
}

const CLIENT_STATE_EVENTS = ['client_online', 'client_offline', 'client_new']
const RECONNECT_BASE = 5000
const RECONNECT_MAX = 60000

export const useEventsStore = defineStore('events', () => {
  const events = ref<C2Event[]>([])
  const connected = ref(false)

  let ws: WebSocket | null = null
  let reconnectDelay = RECONNECT_BASE
  let refreshTimer: ReturnType<typeof setTimeout> | null = null
  let refCount = 0

  // 客户端状态类事件到达后联动刷新客户端列表（节流 5s）
  const scheduleClientRefresh = () => {
    if (refreshTimer) return
    refreshTimer = setTimeout(() => {
      refreshTimer = null
      useClientsStore().refreshClients()
    }, 5000)
  }

  // 建连核心（不计数）。幂等：已有存活连接时直接复用
  const openSocket = () => {
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
      return
    }
    try {
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const sock = new WebSocket(`${protocol}//${window.location.host}/api/events/ws`)
      ws = sock
      sock.onopen = () => {
        reconnectDelay = RECONNECT_BASE
        sock.send(JSON.stringify({ type: 'auth', content: useUserStore().getToken() || '' }))
      }
      sock.onmessage = (e) => {
        try {
          const msg = JSON.parse(e.data)
          if (msg.type === 'ok') { connected.value = true; return }
          if (msg.type === 'ping') {
            // F8: 回 pong 保活（原仅 return，服务端依赖 pong 时连接会被踢）
            try { sock.send(JSON.stringify({ type: 'pong' })) } catch { /* ignore */ }
            return
          }
          if (msg.type && msg.type !== 'auth') {
            events.value.unshift(msg)
            if (events.value.length > 100) events.value.pop()
            // 上线提示音(P1-7: 前端播放,服务端无头进程播音无意义)
            if (msg.type === 'client_online') {
              try { if (usePreferencesStore().audioNotify) playOnlineChime() } catch { /* ignore */ }
            }
            if (CLIENT_STATE_EVENTS.includes(msg.type)) {
              scheduleClientRefresh()
            }
          }
        } catch { /* ignore */ }
      }
      sock.onclose = () => {
        connected.value = false
        if (ws !== sock) return
        ws = null
        // 指数退避重连（5s 起，上限 60s），页面隐藏时跳过；无订阅者时不再重连
        if (refCount > 0 && document.visibilityState !== 'hidden') {
          setTimeout(openSocket, reconnectDelay)
        }
        reconnectDelay = Math.min(reconnectDelay * 2, RECONNECT_MAX)
      }
      sock.onerror = () => { connected.value = false }
    } catch { /* ignore */ }
  }

  const connect = () => {
    refCount++
    openSocket()
  }

  // 团队频道：消息经事件流上行，服务端广播给所有操作员（含发送者自己），
  // 因此这里不回显本地——本地 echo 会在多端出现重复行。
  const sendChat = (text: string): boolean => {
    const msg = text.trim()
    if (!msg) return false
    if (!ws || ws.readyState !== WebSocket.OPEN) return false
    try {
      ws.send(JSON.stringify({ type: 'chat', content: msg }))
      return true
    } catch {
      return false
    }
  }

  const disconnect = () => {
    refCount = Math.max(0, refCount - 1)
    // 仍有其他订阅者时保留连接，仅最后一个订阅者退出时才真正关闭
    if (refCount > 0) return
    if (ws) { ws.onclose = null; ws.close(); ws = null }
    connected.value = false
  }

  return { events, connected, connect, disconnect, sendChat }
})
