<template>
  <div class="event-log cs-font-mono">
    <!-- Event Log Toolbar -->
    <div class="el-toolbar">
      <span class="el-title">{{ isZh ? '团队服务器事件日志' : 'TeamServer Event Log' }}</span>

      <!-- Filter Buttons -->
      <div class="el-filter-group">
        <button
          class="el-filter-btn"
          :class="{ active: currentFilter === 'all' }"
          @click="currentFilter = 'all'"
        >
          {{ isZh ? '全部' : 'All' }}
        </button>
        <button
          class="el-filter-btn"
          :class="{ active: currentFilter === 'client' }"
          @click="currentFilter = 'client'"
        >
          {{ isZh ? '客户端' : 'Beacons' }}
        </button>
        <button
          class="el-filter-btn"
          :class="{ active: currentFilter === 'user' }"
          @click="currentFilter = 'user'"
        >
          {{ isZh ? '操作员' : 'Users' }}
        </button>
        <button
          class="el-filter-btn"
          :class="{ active: currentFilter === 'system' }"
          @click="currentFilter = 'system'"
        >
          {{ isZh ? '系统与服务' : 'System' }}
        </button>
        <button
          class="el-filter-btn"
          :class="{ active: currentFilter === 'chat' }"
          @click="currentFilter = 'chat'"
        >
          {{ isZh ? '聊天' : 'Chat' }}
        </button>
      </div>

      <div class="el-spacer" />

      <button class="cs-btn cs-btn-xs" @click="loadAuditAndClientLogs">{{ isZh ? '刷新' : 'Refresh' }}</button>
      <button class="cs-btn cs-btn-xs" @click="clearLog">{{ isZh ? '清空' : 'Clear' }}</button>
      <button class="cs-btn cs-btn-xs" :class="{ 'cs-btn-primary': follow }" @click="follow = !follow">
        {{ follow ? (isZh ? '自动滚动：开' : 'Auto-Scroll: ON') : (isZh ? '自动滚动：关' : 'Auto-Scroll: OFF') }}
      </button>
      <span class="el-count">{{ filteredLines.length }} / {{ lines.length }} {{ isZh ? '条' : 'events' }}</span>
    </div>

    <!-- Events Body -->
    <div ref="logEl" class="el-body">
      <div
        v-for="e in filteredLines"
        :key="lineKey(e)"
        :class="['el-line', `el-type-${e.type}`]"
      >
        <span class="el-time">[{{ fmtTime(e.time) }}]</span>
        <span v-if="e.user" class="el-user">&lt;{{ e.user }}&gt;</span>
        <span v-else-if="e.type === 'beacon_online' || e.type === 'client_online'" class="el-prefix-online">***</span>
        <span v-else-if="e.type === 'beacon_offline' || e.type === 'client_offline'" class="el-prefix-offline">***</span>
        <span v-else-if="e.type === 'user_login' || e.type === 'user_logout'" class="el-prefix-user">***</span>
        <span v-else-if="e.type === 'cred'" class="el-prefix-cred">***</span>
        <span v-else-if="e.type === 'error'" class="el-prefix-error">***</span>
        <span v-else class="el-prefix">***</span>
        <span class="el-text">{{ e.text }}</span>
      </div>
      <div v-if="!filteredLines.length" class="el-empty">
        {{ isZh ? '暂无匹配的事件日志。' : 'No matching event logs.' }}
      </div>
    </div>

    <!-- Bottom Operator Chat Row -->
    <div class="el-chat-row">
      <span class="chat-prompt">&lt;{{ currentUser }}&gt;</span>
      <input
        v-model="chatInput"
        class="chat-input"
        :placeholder="isZh ? '向团队服务器所有操作员发送消息...' : 'Send message to all operators in Event Log...'"
        @keydown.enter="sendChat"
      />
      <button class="cs-btn cs-btn-xs" @click="sendChat">{{ isZh ? '发送' : 'Send' }}</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useEventsStore } from '@/stores/events'
import { useUserStore } from '@/stores/user'
import { useClientsStore, type ClientData } from '@/stores/clients'
import { useCollabStore } from '@/stores/collab'
import { useI18nStore } from '@/stores/i18n'
import { detectClientOs } from '@/utils/clientUtils'
import AuditAPI from '@/api/audit'

const eventsStore = useEventsStore()
const userStore = useUserStore()
const clientsStore = useClientsStore()
const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const follow = ref(true)
const logEl = ref<HTMLElement | null>(null)
const chatInput = ref('')
const currentFilter = ref<'all' | 'client' | 'user' | 'system' | 'chat'>('all')

const currentUser = computed(() => userStore.username || 'admin')

interface Line {
  id?: string | number
  time: number
  type: string
  category: 'client' | 'user' | 'system' | 'chat'
  user?: string
  text: string
}

const lines = ref<Line[]>([])
const seenIds = new Set<string>()
const clientStates = new Map<string, string>()

// 行去重键(与渲染 key 共用,保证淘汰时能精确回收 seenIds 条目)
function lineKey(line: Line): string {
  return line.id ? String(line.id) : `${line.time}-${line.type}-${line.text.slice(0, 32)}`
}

const filteredLines = computed(() => {
  if (currentFilter.value === 'all') return lines.value
  return lines.value.filter((l) => l.category === currentFilter.value)
})

function addLogLine(line: Line) {
  const dedupeKey = lineKey(line)
  if (seenIds.has(dedupeKey)) return
  seenIds.add(dedupeKey)

  // 【性能】仅在乱序到达时才排序(事件通常近似有序,原实现每条都 O(n log n))
  const last = lines.value[lines.value.length - 1]
  if (last && line.time < last.time) {
    lines.value.push(line)
    lines.value.sort((a, b) => a.time - b.time)
  } else {
    lines.value.push(line)
  }

  if (lines.value.length > 2000) {
    // 【修复】裁剪时同步回收去重集合条目(原 seenIds 只增不减,长时间挂台内存无界)
    const removed = lines.value.splice(0, lines.value.length - 2000)
    for (const r of removed) seenIds.delete(lineKey(r))
  }
  // 双保险:集合规模异常时按现存行重建
  if (seenIds.size > 4000) {
    seenIds.clear()
    for (const l of lines.value) seenIds.add(lineKey(l))
  }
  if (follow.value) scroll()
}

// 格式化时间
function fmtTime(t: number) {
  if (!t) return '--:--:--'
  const d = new Date(t)
  const hh = String(d.getHours()).padStart(2, '0')
  const mm = String(d.getMinutes()).padStart(2, '0')
  const ss = String(d.getSeconds()).padStart(2, '0')
  return `${hh}:${mm}:${ss}`
}

async function clearLog() {
  try {
    await ElMessageBox.confirm(
      isZh.value ? '确定清空事件日志吗？清空后不可恢复。' : 'Clear the event log? This cannot be undone.',
      isZh.value ? '清空事件日志' : 'Clear Event Log',
      {
        confirmButtonText: isZh.value ? '清空' : 'Clear',
        cancelButtonText: isZh.value ? '取消' : 'Cancel',
        type: 'warning',
      },
    )
  } catch {
    return
  }
  lines.value = []
  seenIds.clear()
}

function scroll() {
  nextTick(() => {
    if (logEl.value) logEl.value.scrollTop = logEl.value.scrollHeight
  })
}

function sendChat() {
  const msg = chatInput.value.trim()
  if (!msg) return
  // 走事件流上行、由服务端广播（原实现只本地 echo：其他操作员永远看不到）
  if (!eventsStore.sendChat(msg)) {
    ElMessage.warning(isZh.value ? '事件流未连接，消息未发送' : 'Event stream offline, message not sent')
    return
  }
  chatInput.value = ''
}

// 1. 加载服务端审计日志与客户端初始上线状态（过滤掉纯 API 轮询，仅保留用户、会话与核心系统事件）
async function loadAuditAndClientLogs() {
  try {
    const res = await AuditAPI.getAuditLogs({ page: 1, page_size: 100 })
    if (res.data?.status === 200 && res.data.data?.logs) {
      const logs = res.data.data.logs as any[]
      for (const row of logs) {
        const time = row.timestamp ? new Date(row.timestamp).getTime() : Date.now()
        const cat = (row.category || '').toUpperCase()
        const act = (row.action || '').toUpperCase()
        const user = row.username || ''
        const ip = row.client_ip || ''
        const msg = row.message || ''

        // 过滤掉常规的前端 REST API 轮询流水（如 /api/client/clientslist 等，这属于 Web 日志或审计日志，而非团队事件日志）
        if (
          cat === 'API' ||
          cat === 'HTTP' ||
          act.includes('API') ||
          act.includes('ACCESS') ||
          act.includes('QUERY') ||
          msg.includes('/api/') ||
          msg.includes('GET ') ||
          msg.includes('POST ')
        ) {
          continue
        }

        let logType = 'system'
        let logCategory: Line['category'] = 'system'
        let text = ''

        if (cat === 'AUTH' || act.includes('LOGIN') || act.includes('LOGOUT')) {
          logCategory = 'user'
          if (act.includes('LOGIN')) {
            logType = 'user_login'
            text = isZh.value
              ? `操作员 <${user}> 登录成功 (来源 IP: ${ip || '-'})`
              : `Operator <${user}> logged in from ${ip || '-'}`
          } else {
            logType = 'user_logout'
            text = isZh.value
              ? `操作员 <${user}> 注销退出`
              : `Operator <${user}> logged out`
          }
        } else if (cat === 'CLIENT' || cat === 'BEACON' || act.includes('ONLINE') || act.includes('OFFLINE')) {
          logCategory = 'client'
          if (act.includes('ONLINE')) {
            logType = 'beacon_online'
            text = isZh.value ? `客户端已上线: ${msg || user}` : `Beacon online: ${msg || user}`
          } else {
            logType = 'beacon_offline'
            text = isZh.value ? `客户端已离线: ${msg || user}` : `Beacon offline: ${msg || user}`
          }
        } else if (cat === 'CREDENTIAL' || act.includes('CRED')) {
          logType = 'cred'
          logCategory = 'client'
          text = isZh.value ? `抓取到凭据: ${msg}` : `Credential captured: ${msg}`
        } else if (cat === 'LISTENER' || act.includes('LISTENER')) {
          logType = 'system'
          logCategory = 'system'
          text = isZh.value ? `监听器事件: [${act}] ${msg}` : `Listener event: [${act}] ${msg}`
        } else if (cat === 'PAYLOAD' || cat === 'GEN' || act.includes('PAYLOAD')) {
          logType = 'system'
          logCategory = 'system'
          text = isZh.value ? `载荷生成: ${msg}` : `Payload generated: ${msg}`
        } else if (cat === 'TUNNEL' || cat === 'SOCKS' || act.includes('TUNNEL') || act.includes('SOCKS')) {
          logType = 'system'
          logCategory = 'system'
          text = isZh.value ? `代理隧道: ${msg}` : `Tunnel event: ${msg}`
        } else if (cat === 'SECURITY' || act.includes('SECURITY') || act.includes('BLOCK')) {
          logType = 'error'
          logCategory = 'system'
          text = isZh.value ? `安全中心事件: ${msg}` : `Security event: ${msg}`
        } else {
          // 其他常规系统里程碑
          logType = 'system'
          logCategory = 'system'
          text = `[${cat || 'SYS'}] ${act}: ${msg}`
        }

        addLogLine({
          id: row.id || `${time}-${row.action}-${text}`,
          time,
          type: logType,
          category: logCategory,
          text,
        })
      }
    }
  } catch (e) {
    // 忽略加载错误
  }

  // 检查客户端初始上线状态
  if (clientsStore.clientsList && clientsStore.clientsList.length > 0) {
    for (const c of clientsStore.clientsList) {
      clientStates.set(c.Uid, c.Online)
      if (c.Online === '1') {
        const extIp = c.ExternalIP || '-'
        const intIp = c.InternalIP ? `, 内网: ${c.InternalIP}` : ''
        const arch = c.Arch || 'x64'
        const osType = detectClientOs(c)
        const os = c.Os || (osType === 'windows' ? 'Windows' : (osType === 'linux' ? 'Linux' : (osType === 'darwin' ? 'macOS' : 'Unknown')))
        const proc = c.ProcessName || c.Computer || ''
        addLogLine({
          id: `init-beacon-${c.Uid}`,
          time: Date.now() - 3600000,
          type: 'beacon_online',
          category: 'client',
          text: isZh.value
            ? `会话就绪: ${c.Username || 'user'}@${c.Computer || 'host'} [UID: ${c.Uid.slice(0, 8)}, 外网: ${extIp}${intIp}, PID: ${c.Pid || '?'}, 架构: ${arch}, 系统: ${os}, 监听器: ${c.Listener || 'TCP'}${proc ? `, 进程: ${proc}` : ''}]`
            : `Beacon active: ${c.Username || 'user'}@${c.Computer || 'host'} [UID: ${c.Uid.slice(0, 8)}, Ext: ${extIp}${intIp}, PID: ${c.Pid || '?'}, Arch: ${arch}, OS: ${os}, Listener: ${c.Listener || 'TCP'}${proc ? `, Proc: ${proc}` : ''}]`,
        })
      }
    }
  }
}

// 2. 监听 WebSocket 实时推送事件
// 【性能】增量处理:events 数组为新事件前插(unshift)+ 尾部裁剪,已处理事件用
// WeakSet 标记;从头部遍历,遇到已处理的即 break(其后必然全部已处理)。
// 原实现每次 watch 都遍历全部 100 条并重复做客户端查找与行构造。
// 操作广播的动作文案（服务端 action 值 → 可读标签）；用函数而非常量表，
// 切换语言后新到的事件才会跟着变
function actionLabel(action: string): string {
  const zh: Record<string, string> = {
    lock: '占用了会话', unlock: '释放了会话', command: '下发命令',
    batch: '批量下发', exec: '执行载荷',
  }
  const en: Record<string, string> = {
    lock: 'locked session', unlock: 'released session', command: 'sent command',
    batch: 'batch command', exec: 'executed payload',
  }
  return (isZh.value ? zh : en)[action] || action
}

const processedEvents = new WeakSet<object>()
watch(
  () => eventsStore.events,
  () => {
    for (const e of eventsStore.events) {
      if (processedEvents.has(e)) break // 旧事件都在后面,已处理过
      processedEvents.add(e)
      const raw = e as any
      const time = e.timestamp ? e.timestamp * 1000 : Date.now()
      const t = String(e.type || '').toLowerCase()
      const detail = e.detail || {}

      if (t === 'client_online' || t === 'client_new') {
        const uid = e.uid || detail.uid || raw.uid || ''
        const foundClient = uid ? clientsStore.getClientByUid(uid) : null
        const u = detail.username || raw.username || foundClient?.Username || 'user'
        const comp = detail.computer || raw.computer || foundClient?.Computer || 'host'
        const extIp = detail.external_ip || detail.ip || raw.ip || foundClient?.ExternalIP || '-'
        const intIp = (detail.internal_ip || foundClient?.InternalIP) ? `, 内网: ${detail.internal_ip || foundClient?.InternalIP}` : ''
        const pid = detail.pid || raw.pid || foundClient?.Pid || '?'
        const arch = detail.arch || raw.arch || foundClient?.Arch || 'x64'
        const osType = detectClientOs(foundClient || { Os: detail.os || raw.os, Arch: arch, Computer: comp, Username: u })
        const os = detail.os || raw.os || foundClient?.Os || (osType === 'windows' ? 'Windows' : (osType === 'linux' ? 'Linux' : (osType === 'darwin' ? 'macOS' : 'Unknown')))
        const listener = detail.listener || raw.listener || foundClient?.Listener || 'TCP'
        const proc = detail.process || raw.process || foundClient?.ProcessName || ''

        addLogLine({
          time,
          type: 'beacon_online',
          category: 'client',
          text: isZh.value
            ? `客户端首次上线: ${u}@${comp} [UID: ${uid ? uid.slice(0, 8) : '?'}, 外网: ${extIp}${intIp}, PID: ${pid}, 架构: ${arch}, 系统: ${os}, 监听器: ${listener}${proc ? `, 进程: ${proc}` : ''}]`
            : `Initial Beacon from ${u}@${comp} [UID: ${uid ? uid.slice(0, 8) : '?'}, Ext: ${extIp}${intIp}, PID: ${pid}, Arch: ${arch}, OS: ${os}, Listener: ${listener}${proc ? `, Proc: ${proc}` : ''}]`,
        })
      } else if (t === 'client_offline') {
        const uid = e.uid || detail.uid || raw.uid || ''
        const foundClient = uid ? clientsStore.getClientByUid(uid) : null
        const u = detail.username || raw.username || foundClient?.Username || 'user'
        const comp = detail.computer || raw.computer || foundClient?.Computer || ''
        const extIp = detail.external_ip || detail.ip || raw.ip || foundClient?.ExternalIP || '-'
        const pid = detail.pid || raw.pid || foundClient?.Pid || ''
        addLogLine({
          time,
          type: 'beacon_offline',
          category: 'client',
          text: isZh.value
            ? `客户端已离线: ${u}${comp ? '@' + comp : ''} [UID: ${uid ? uid.slice(0, 8) : '?'}, IP: ${extIp}${pid ? `, PID: ${pid}` : ''}]`
            : `Beacon offline: ${u}${comp ? '@' + comp : ''} [UID: ${uid ? uid.slice(0, 8) : '?'}, IP: ${extIp}${pid ? `, PID: ${pid}` : ''}]`,
        })
      } else if (t === 'user_login' || t === 'operator_login') {
        const op = detail.username || raw.user || raw.operator || 'operator'
        const ip = detail.ip || raw.ip || ''
        addLogLine({
          time,
          type: 'user_login',
          category: 'user',
          text: isZh.value
            ? `操作员 <${op}> 已连接到团队服务器 (来源 IP: ${ip || '-'})`
            : `Operator <${op}> connected to TeamServer (IP: ${ip || '-'})`,
        })
      } else if (t === 'user_logout' || t === 'operator_logout') {
        const op = detail.username || raw.user || raw.operator || 'operator'
        addLogLine({
          time,
          type: 'user_logout',
          category: 'user',
          text: isZh.value
            ? `操作员 <${op}> 已断开连接`
            : `Operator <${op}> disconnected`,
        })
      } else if (t === 'cred' || t === 'cred_new') {
        addLogLine({
          time,
          type: 'cred',
          category: 'client',
          text: isZh.value
            ? `抓取到新凭证: ${detail.username || ''}@${detail.realm || ''} (${detail.source || 'mimikatz'})`
            : `Credential harvested: ${detail.username || ''}@${detail.realm || ''} (${detail.source || 'mimikatz'})`,
        })
      } else if (t === 'listener_open' || t === 'listener_start') {
        addLogLine({
          time,
          type: 'system',
          category: 'system',
          text: isZh.value
            ? `监听器已启动: ${detail.name || detail.type || ''} (地址: ${detail.listenAddress || detail.connectAddress || ''})`
            : `Listener started: ${detail.name || detail.type || ''} (${detail.listenAddress || detail.connectAddress || ''})`,
        })
      } else if (t === 'listener_close' || t === 'listener_stop') {
        addLogLine({
          time,
          type: 'system',
          category: 'system',
          text: isZh.value
            ? `监听器已关闭: ${detail.name || detail.type || ''}`
            : `Listener stopped: ${detail.name || detail.type || ''}`,
        })
      } else if (t === 'operator_action') {
        const op = detail.operator || raw.operator || 'operator'
        const action = String(detail.action || '')
        const target = e.uid ? ` [${e.uid.slice(0, 8)}]` : ''
        const label = actionLabel(action)
        addLogLine({
          time,
          type: 'user_login',
          category: 'user',
          text: isZh.value
            ? `操作员 <${op}> ${label}${target}${detail.detail ? `: ${detail.detail}` : ''}`
            : `Operator <${op}> ${label}${target}${detail.detail ? `: ${detail.detail}` : ''}`,
        })
        // 占用状态可能刚变化，立即刷新（不必等 30s 轮询）
        try { useCollabStore().refresh() } catch { /* ignore */ }
      } else if (t === 'chat') {
        addLogLine({
          time,
          type: 'chat',
          category: 'chat',
          user: raw.user || raw.operator || 'operator',
          text: detail.message || raw.message || raw.text || '',
        })
      } else if (
        t !== 'auth' &&
        t !== 'pong' &&
        !t.includes('api') &&
        !t.includes('http') &&
        !t.includes('poll')
      ) {
        const detailStr = e.detail
          ? Object.entries(e.detail).map(([k, v]) => `${k}=${String(v)}`).join(' ')
          : ''
        const msg = detailStr || raw.message || raw.text || t
        if (
          !msg.includes('/api/') &&
          !msg.includes('GET ') &&
          !msg.includes('POST ') &&
          !msg.includes('API_ACCESS')
        ) {
          addLogLine({
            time,
            type: 'system',
            category: 'system',
            user: raw.operator || raw.user,
            text: `${e.uid ? `[${e.uid.slice(0, 8)}] ` : ''}${msg}`,
          })
        }
      }
    }
  },
  { deep: true },
)

// 3. 监控 clientsStore 状态跃迁（离线->上线 / 上线->离线）
watch(
  () => clientsStore.clientsList,
  (newList) => {
    if (!newList || newList.length === 0) return
    for (const c of newList) {
      const prev = clientStates.get(c.Uid)
      if (prev !== undefined && prev !== c.Online) {
        if (c.Online === '1') {
          const ip = c.ExternalIP || c.InternalIP || '-'
          addLogLine({
            time: Date.now(),
            type: 'beacon_online',
            category: 'client',
            text: isZh.value
              ? `客户端已上线: ${c.Username || 'user'}@${c.Computer || 'host'} (IP: ${ip}, PID: ${c.Pid || '?'}, 监听器: ${c.Listener || 'TCP'})`
              : `Beacon online: ${c.Username || 'user'}@${c.Computer || 'host'} (IP: ${ip}, PID: ${c.Pid || '?'}, Listener: ${c.Listener || 'TCP'})`,
          })
        } else {
          addLogLine({
            time: Date.now(),
            type: 'beacon_offline',
            category: 'client',
            text: isZh.value
              ? `客户端连接断开/离线: ${c.Username || 'user'}@${c.Computer || 'host'} (UID: ${c.Uid.slice(0, 8)})`
              : `Beacon disconnected: ${c.Username || 'user'}@${c.Computer || 'host'} (UID: ${c.Uid.slice(0, 8)})`,
          })
        }
      }
      clientStates.set(c.Uid, c.Online)
    }
  },
  { deep: true },
)

onMounted(async () => {
  addLogLine({
    id: 'sys-start',
    time: Date.now(),
    type: 'system',
    category: 'system',
    text: isZh.value
      ? `团队服务器已连接：${window.location.host || '127.0.0.1:8083'}，系统日志已就绪。`
      : `TeamServer connected at ${window.location.host || '127.0.0.1:8083'}. Ready.`,
  })

  await loadAuditAndClientLogs()
  scroll()
})
</script>

<style scoped>
.event-log {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-panel);
  overflow: hidden;
}

/* ===== 工具条：白底 + 1px 分隔线 ===== */
.el-toolbar {
  flex-shrink: 0;
  min-height: 44px;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  padding: 6px 16px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-card-border);
  user-select: none;
}

.el-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
  white-space: nowrap;
}

/* 分类筛选（分段控件） */
.el-filter-group {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  padding: 2px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-border-strong);
  border-radius: 6px;
}

.el-filter-btn {
  height: 24px;
  padding: 0 10px;
  font-family: var(--cs-font-ui);
  font-size: 13px;
  color: var(--cs-text-dim);
  background: transparent;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  white-space: nowrap;
  transition: background 120ms ease, color 120ms ease;
}

.el-filter-btn:hover {
  color: var(--cs-text);
  background: var(--cs-hover);
}

.el-filter-btn.active {
  color: #ffffff;
  background: var(--cs-active);
}

.el-spacer {
  flex: 1;
}

.el-count {
  font-size: 13px;
  color: var(--cs-text-dim);
  white-space: nowrap;
}

/* ===== 日志区（深色控制台本体保留；令牌随主题） ===== */
.el-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 12px 16px;
  font-family: var(--cs-font-mono);
  font-size: 13px;
  line-height: 1.7;
  background: var(--cs-console);
  color: var(--cs-text);
}

.el-line {
  margin-bottom: 2px;
  white-space: pre-wrap;
  word-break: break-all;
}

.el-time {
  margin-right: 6px;
  color: var(--cs-text-dim);
}

.el-prefix,
.el-prefix-online,
.el-prefix-offline,
.el-prefix-user,
.el-prefix-cred,
.el-prefix-error,
.el-user {
  margin-right: 6px;
  font-weight: 600;
}

.el-prefix { color: var(--cs-active); }
.el-prefix-online { color: var(--cs-online); }
.el-prefix-offline { color: var(--cs-error); }
.el-prefix-user { color: var(--cs-log-cred); }
.el-prefix-cred { color: var(--cs-log-cred); }
.el-prefix-error { color: var(--cs-error); }
.el-user { color: var(--cs-online); }

.el-text {
  color: var(--cs-text);
}

.el-type-beacon_online .el-text { color: var(--cs-online); }
.el-type-beacon_offline .el-text { color: var(--cs-error); }
.el-type-user_login .el-text { color: var(--cs-log-cred); }
.el-type-user_logout .el-text { color: var(--cs-log-cred); }
.el-type-cred .el-text { color: var(--cs-log-cred); }
.el-type-system .el-text { color: var(--cs-log-system); }
.el-type-chat .el-text { color: var(--cs-text); font-weight: 600; }
.el-type-error .el-text { color: var(--cs-error); }

.el-empty {
  margin-top: 40px;
  font-size: 13px;
  text-align: center;
  color: var(--cs-text-dim);
}

/* ===== 底部操作员聊天输入条 ===== */
.el-chat-row {
  flex-shrink: 0;
  min-height: 48px;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 16px;
  background: var(--cs-panel);
  border-top: 1px solid var(--cs-card-border);
}

.chat-prompt {
  font-family: var(--cs-font-mono);
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-online);
  white-space: nowrap;
}

.chat-input {
  flex: 1;
  height: 32px;
  padding: 0 10px;
  font-family: var(--cs-font-ui);
  font-size: 14px;
  color: var(--cs-text);
  background: var(--cs-panel);
  border: 1px solid var(--cs-border-strong);
  border-radius: 6px;
  outline: 0;
  transition: border-color 120ms ease;
}

.chat-input:focus {
  border-color: var(--cs-active);
}

.chat-input::placeholder {
  color: var(--cs-text-dim);
}
</style>
