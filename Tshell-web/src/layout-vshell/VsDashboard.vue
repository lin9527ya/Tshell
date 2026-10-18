<template>
  <div class="vs-dash">
    <!-- 统计卡行（vshell: 蓝色标题链接 + 右侧彩色大图标） -->
    <div class="vs-dash-cards">
      <div class="vs-card vs-stat-card">
        <div class="vs-stat-title">{{ i18nStore.tl('监听总数', 'Listeners') }}</div>
        <div class="vs-stat-row">
          <span class="vs-stat-value">{{ stats?.monitor_count ?? 0 }}</span>
          <VsIcon name="magic" :size="40" />
        </div>
      </div>
      <div class="vs-card vs-stat-card">
        <div class="vs-stat-title">{{ i18nStore.tl('在线监听数', 'Online Listeners') }}</div>
        <div class="vs-stat-row">
          <span class="vs-stat-value">{{ stats?.connection_count ?? 0 }}</span>
          <VsIcon name="headset" :size="40" />
        </div>
      </div>
      <div class="vs-card vs-stat-card">
        <div class="vs-stat-title">{{ i18nStore.tl('客户端总数', 'Clients') }}</div>
        <div class="vs-stat-row">
          <span class="vs-stat-value">{{ stats?.user_count ?? 0 }}</span>
          <VsIcon name="responsive" :size="40" />
        </div>
      </div>
      <div class="vs-card vs-stat-card">
        <div class="vs-stat-title">{{ i18nStore.tl('在线客户端', 'Online Clients') }}</div>
        <div class="vs-stat-row">
          <span class="vs-stat-value">{{ stats?.online_user_count ?? 0 }}</span>
          <VsIcon name="pc" :size="40" />
        </div>
      </div>
    </div>

    <!-- 配置信息 / 系统信息 -->
    <div class="vs-dash-panels">
      <div class="vs-card vs-panel">
        <div class="vs-panel-head">{{ i18nStore.tl('配置信息', 'Config') }}</div>
        <div class="vs-panel-body">
          <div class="vs-kv">
            <span>{{ i18nStore.tl('日志级别', 'Log level') }}</span>
            <b>{{ cfg?.log_level || '-' }}</b>
          </div>
          <div class="vs-kv">
            <span>{{ i18nStore.tl('日志文件', 'Log file') }}</span>
            <b class="ellipsis">{{ cfg?.log_file_path || '-' }}</b>
          </div>
          <div class="vs-kv">
            <span>{{ i18nStore.tl('WEB端口', 'WEB port') }}</span>
            <b>{{ cfg?.web_port ?? '-' }}</b>
          </div>
          <div class="vs-kv">
            <span>{{ i18nStore.tl('Basic认证', 'Basic auth') }}</span>
            <b>{{ cfg?.basic_auth ? (i18nStore.tl('已开启', 'On')) : (i18nStore.tl('未开启', 'Off')) }}</b>
          </div>
          <div class="vs-kv">
            <span>{{ i18nStore.tl('版本', 'Version') }}</span>
            <b>{{ cfg?.version || '-' }}</b>
          </div>
        </div>
      </div>

      <div class="vs-card vs-panel">
        <div class="vs-panel-head">{{ i18nStore.tl('系统信息', 'System') }}</div>
        <div class="vs-panel-body">
          <div v-for="g in gauges" :key="g.key" class="vs-gauge-row">
            <span class="vs-gauge-label">{{ g.label }}</span>
            <div class="vs-gauge-track">
              <div class="vs-gauge-fill" :style="{ width: g.value + '%' }" />
            </div>
            <span class="vs-gauge-val">{{ g.value.toFixed(0) }}%</span>
          </div>

          <div class="vs-kv">
            <span>{{ i18nStore.tl('TCP 通讯 (已建立)', 'TCP (established)') }}</span>
            <b>{{ sys?.tcp_connections ?? 0 }}</b>
          </div>
          <div class="vs-kv">
            <span>{{ i18nStore.tl('UDP 通讯 (已建立)', 'UDP (established)') }}</span>
            <b>{{ sys?.udp_connections ?? 0 }}</b>
          </div>
          <div class="vs-kv">
            <span>{{ i18nStore.tl('流出带宽', 'Outbound') }}</span>
            <b>{{ sys?.outbound_bw || '-' }}</b>
          </div>
          <div class="vs-kv">
            <span>{{ i18nStore.tl('流入带宽', 'Inbound') }}</span>
            <b>{{ sys?.inbound_bw || '-' }}</b>
          </div>
        </div>
      </div>
    </div>

    <!-- 审计活动 / 安全态势 -->
    <div class="vs-dash-panels">
      <div class="vs-card vs-panel">
        <div class="vs-panel-head">
          {{ i18nStore.tl('审计活动（24 小时）', 'Audit activity (24h)') }}
          <span class="vs-panel-sub">{{ i18nStore.tl('按分类', 'by category') }}</span>
        </div>
        <div class="vs-panel-body">
          <div class="vs-triple">
            <div class="vs-triple-item">
              <b>{{ audit?.total_logs ?? 0 }}</b>
              <span>{{ i18nStore.tl('总事件', 'Total') }}</span>
            </div>
            <div class="vs-triple-item">
              <b>{{ audit?.today_logs ?? 0 }}</b>
              <span>{{ i18nStore.tl('今日', 'Today') }}</span>
            </div>
            <div class="vs-triple-item">
              <b :class="{ warn: (audit?.recent_failures ?? 0) > 0 }">{{ audit?.recent_failures ?? 0 }}</b>
              <span>{{ i18nStore.tl('近 1 小时失败', 'Failed 1h') }}</span>
            </div>
          </div>

          <div v-if="auditCategories.length" class="vs-bars">
            <div v-for="c in auditCategories" :key="c.key" class="vs-bar-row">
              <span class="vs-bar-label">{{ c.label }}</span>
              <div class="vs-bar-track">
                <div class="vs-bar-fill" :style="{ width: c.pct + '%' }" />
              </div>
              <span class="vs-bar-val">{{ c.count }}</span>
            </div>
          </div>
          <div v-else class="vs-empty">{{ i18nStore.tl('暂无审计数据', 'No audit data') }}</div>

          <template v-if="audit?.top_ips?.length">
            <div class="vs-sub">{{ i18nStore.tl('活跃来源 IP', 'Top source IPs') }}</div>
            <div v-for="ip in audit.top_ips.slice(0, 4)" :key="ip.ip" class="vs-kv">
              <span class="ellipsis-left">{{ ip.ip }}</span>
              <b>{{ ip.count }}</b>
            </div>
          </template>
        </div>
      </div>

      <div class="vs-card vs-panel">
        <div class="vs-panel-head">{{ i18nStore.tl('安全态势', 'Security') }}</div>
        <div class="vs-panel-body">
          <div class="vs-triple">
            <div class="vs-triple-item">
              <b :class="{ warn: (sec?.blocked_ips?.length ?? 0) > 0 }">{{ sec?.blocked_ips?.length ?? 0 }}</b>
              <span>{{ i18nStore.tl('拦截 IP', 'Blocked IPs') }}</span>
            </div>
            <div class="vs-triple-item">
              <b>{{ sec?.login_attempts_tracked ?? 0 }}</b>
              <span>{{ i18nStore.tl('追踪登录', 'Login tracked') }}</span>
            </div>
            <div class="vs-triple-item">
              <b>{{ realtime?.total_active ?? 0 }}</b>
              <span>{{ i18nStore.tl('近 1 小时活跃 IP', 'Active IPs 1h') }}</span>
            </div>
          </div>

          <div class="vs-sub">{{ i18nStore.tl('被拦截 IP', 'Blocked list') }}</div>
          <div v-if="sec?.blocked_ips?.length">
            <div v-for="ip in sec.blocked_ips.slice(0, 5)" :key="ip" class="vs-kv">
              <span class="ellipsis-left">{{ ip }}</span>
              <VsIcon name="shield-alt" :size="12" />
            </div>
          </div>
          <div v-else class="vs-empty">{{ i18nStore.tl('暂无拦截记录', 'No blocked IPs') }}</div>

          <div class="vs-kv">
            <span>{{ i18nStore.tl('追踪 API 请求', 'API requests tracked') }}</span>
            <b>{{ sec?.api_requests_tracked ?? 0 }}</b>
          </div>
        </div>
      </div>
    </div>

    <!-- 客户端画像 / 实时动态 -->
    <div class="vs-dash-panels">
      <div class="vs-card vs-panel">
        <div class="vs-panel-head">
          {{ i18nStore.tl('客户端画像', 'Client profile') }}
          <span class="vs-panel-sub">{{ onlineClients }}/{{ clientsStore.clientsList.length }} {{ i18nStore.tl('在线', 'online') }}</span>
        </div>
        <div class="vs-panel-body">
          <div v-if="!clientsStore.clientsList.length" class="vs-empty">
            {{ i18nStore.tl('暂无客户端', 'No clients') }}
          </div>
          <template v-else>
            <div class="vs-bar-row">
              <span class="vs-bar-label">{{ i18nStore.tl('在线率', 'Online') }}</span>
              <div class="vs-bar-track">
                <div class="vs-bar-fill" :style="{ width: onlineRate + '%' }" />
              </div>
              <span class="vs-bar-val">{{ onlineRate }}%</span>
            </div>

            <div class="vs-sub">{{ i18nStore.tl('系统 / 架构分布', 'OS / arch') }}</div>
            <div v-for="a in archDist" :key="a.key" class="vs-bar-row">
              <span class="vs-bar-label">{{ a.key }}</span>
              <div class="vs-bar-track">
                <div class="vs-bar-fill" :style="{ width: a.pct + '%' }" />
              </div>
              <span class="vs-bar-val">{{ a.count }}</span>
            </div>

            <div class="vs-sub">{{ i18nStore.tl('监听器分布', 'Listener') }}</div>
            <div v-for="l in listenerDist" :key="l.key" class="vs-bar-row">
              <span class="vs-bar-label">{{ l.key }}</span>
              <div class="vs-bar-track">
                <div class="vs-bar-fill" :style="{ width: l.pct + '%' }" />
              </div>
              <span class="vs-bar-val">{{ l.count }}</span>
            </div>
          </template>
        </div>
      </div>

      <div class="vs-card vs-panel">
        <div class="vs-panel-head">
          {{ i18nStore.tl('实时动态', 'Live events') }}
          <span class="vs-panel-sub">
            <i class="vs-dot" :class="{ on: eventsStore.connected }" />
            {{ eventsStore.connected ? (i18nStore.tl('已连接', 'Connected')) : (i18nStore.tl('未连接', 'Offline')) }}
          </span>
        </div>
        <div class="vs-panel-body">
          <div v-if="!liveEvents.length" class="vs-empty">
            {{ i18nStore.tl('暂无事件（客户端上线/命令输出会实时出现）', 'No events yet') }}
          </div>
          <div v-for="(ev, i) in liveEvents" :key="ev.ts + '-' + i" class="vs-ev">
            <VsIcon :name="ev.icon" :size="14" class="vs-ev-icon" :class="'lv-' + ev.tone" />
            <span class="vs-ev-label">{{ ev.label }}</span>
            <span class="vs-ev-uid">{{ ev.uid }}</span>
            <span class="vs-ev-time">{{ ev.time }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, defineOptions, onBeforeUnmount, onMounted, ref } from 'vue'
import VsIcon from './VsIcon.vue'
import { getDashboardStats } from '@/api/dashboard'
import auditApi from '@/api/audit'
import securityApi from '@/api/security'
import { useI18nStore } from '@/stores/i18n'
import { useEventsStore } from '@/stores/events'
import { useClientsStore } from '@/stores/clients'

defineOptions({ name: 'Dashboard' })

const i18nStore = useI18nStore()
const eventsStore = useEventsStore()
const clientsStore = useClientsStore()
const isZh = computed(() => i18nStore.isZh)

interface DashStats {
  monitor_count?: number
  connection_count?: number
  user_count?: number
  online_user_count?: number
  config_info?: {
    log_level?: string
    log_file_path?: string
    web_port?: number
    basic_auth?: boolean
    version?: string
  }
  system_info?: {
    cpu_usage?: number
    disk_usage?: number
    memory_usage?: number
    virtual_memory?: number
    tcp_connections?: number
    udp_connections?: number
    outbound_bw?: string
    inbound_bw?: string
  }
}

interface AuditStats {
  total_logs?: number
  today_logs?: number
  recent_failures?: number
  logs_by_category?: Record<string, number>
  top_ips?: { ip: string; count: number }[]
}

const stats = ref<DashStats | null>(null)
const audit = ref<AuditStats | null>(null)
const sec = ref<{ blocked_ips?: string[]; login_attempts_tracked?: number; api_requests_tracked?: number } | null>(null)
const realtime = ref<{ total_active?: number } | null>(null)
const timers: number[] = []

const cfg = computed(() => stats.value?.config_info)
const sys = computed(() => stats.value?.system_info)

const clamp = (v?: number) => Math.min(100, Math.max(0, v ?? 0))

const gauges = computed(() => [
  { key: 'cpu', label: isZh.value ? '处理器' : 'CPU', value: clamp(sys.value?.cpu_usage) },
  { key: 'disk', label: isZh.value ? '磁盘' : 'Disk', value: clamp(sys.value?.disk_usage) },
  { key: 'mem', label: isZh.value ? '内存' : 'Memory', value: clamp(sys.value?.memory_usage) },
  { key: 'vmem', label: isZh.value ? '虚拟内存' : 'Virtual memory', value: clamp(sys.value?.virtual_memory) },
])

// ---- 审计活动（按分类，取 Top 6，宽度相对最大值）----
const CATEGORY_LABELS: Record<string, { zh: string; en: string }> = {
  AUTH: { zh: '认证', en: 'Auth' },
  API: { zh: '接口', en: 'API' },
  FILE: { zh: '文件', en: 'File' },
  SYSTEM: { zh: '系统', en: 'System' },
  SECURITY: { zh: '安全', en: 'Security' },
  CONNECTION: { zh: '连接', en: 'Connection' },
  COMMAND: { zh: '命令', en: 'Command' },
}

const auditCategories = computed(() => {
  const map = audit.value?.logs_by_category || {}
  const rows = Object.keys(map)
    .map((k) => ({ key: k, count: map[k] }))
    .sort((a, b) => b.count - a.count)
    .slice(0, 6)
  const max = rows.length ? rows[0].count : 0
  return rows.map((r) => {
    const lab = CATEGORY_LABELS[r.key]
    return {
      ...r,
      label: lab ? (isZh.value ? lab.zh : lab.en) : r.key,
      pct: max > 0 ? Math.max(4, Math.round((r.count / max) * 100)) : 0,
    }
  })
})

// ---- 客户端画像 ----
const onlineClients = computed(() => clientsStore.clientsList.filter((c) => c.Online === '1').length)
const onlineRate = computed(() => {
  const total = clientsStore.clientsList.length
  return total ? Math.round((onlineClients.value / total) * 100) : 0
})

function distribution(pick: (c: { Arch?: string; Os?: string; Listener?: string }) => string | undefined, top = 5) {
  const total = clientsStore.clientsList.length || 1
  const map: Record<string, number> = {}
  for (const c of clientsStore.clientsList) {
    const key = pick(c) || (isZh.value ? '未知' : 'unknown')
    map[key] = (map[key] || 0) + 1
  }
  return Object.keys(map)
    .map((k) => ({ key: k, count: map[k], pct: Math.round((map[k] / total) * 100) }))
    .sort((a, b) => b.count - a.count)
    .slice(0, top)
}

const archDist = computed(() => distribution((c) => c.Os || c.Arch))
const listenerDist = computed(() => distribution((c) => c.Listener, 4))

// ---- 实时动态（事件流 store 已由外壳建立 WS 连接）----
const EVENT_META: Record<string, { zh: string; en: string; icon: string; tone: string }> = {
  client_online: { zh: '客户端上线', en: 'Client online', icon: 'play', tone: 'ok' },
  client_offline: { zh: '客户端离线', en: 'Client offline', icon: 'stop', tone: 'warn' },
  client_new: { zh: '新客户端', en: 'New client', icon: 'plus', tone: 'ok' },
  cmd_output: { zh: '命令输出', en: 'Command output', icon: 'terminal', tone: 'muted' },
  download_done: { zh: '下载完成', en: 'Download done', icon: 'download', tone: 'muted' },
  security_block: { zh: '安全拦截', en: 'Security block', icon: 'shield-alt', tone: 'warn' },
  login_failed: { zh: '登录失败', en: 'Login failed', icon: 'unlock', tone: 'warn' },
}

const liveEvents = computed(() =>
  eventsStore.events.slice(0, 30).map((ev) => {
    const meta = EVENT_META[ev.type] || {
      zh: ev.type,
      en: ev.type,
      icon: 'info-circle',
      tone: 'muted',
    }
    const t = ev.timestamp ? new Date(ev.timestamp * 1000) : new Date()
    const pad = (n: number) => String(n).padStart(2, '0')
    return {
      ts: ev.timestamp || 0,
      label: isZh.value ? meta.zh : meta.en,
      icon: meta.icon,
      tone: meta.tone,
      uid: ev.uid ? ev.uid.slice(0, 8) : '',
      time: `${pad(t.getHours())}:${pad(t.getMinutes())}:${pad(t.getSeconds())}`,
    }
  }),
)

// ---- 拉取 ----
const unwrap = (res: { data?: { data?: unknown } }) => (res.data?.data ?? res.data) as never

async function fetchStats() {
  try {
    stats.value = unwrap(await getDashboardStats())
  } catch {
    /* 失败静默，保留上次数据 */
  }
}

async function fetchAudit() {
  try {
    audit.value = unwrap(await auditApi.getAuditStatistics('24h'))
  } catch {
    /* ignore */
  }
}

async function fetchSecurity() {
  try {
    sec.value = unwrap(await securityApi.getSecurityStats())
  } catch {
    /* ignore */
  }
  try {
    realtime.value = unwrap(await securityApi.getRealtimeStats())
  } catch {
    /* ignore */
  }
}

function visible() {
  return document.visibilityState !== 'hidden'
}

onMounted(() => {
  if (!clientsStore.clientsList.length) clientsStore.fetchClients()
  fetchStats()
  fetchAudit()
  fetchSecurity()

  timers.push(
    window.setInterval(() => { if (visible()) fetchStats() }, 10000), // 与后端 10s 预采 TTL 对齐
    window.setInterval(() => { if (visible()) fetchAudit() }, 60000),
    window.setInterval(() => {
      if (!visible()) return
      fetchSecurity()
      clientsStore.fetchClients()
    }, 30000),
  )
})

onBeforeUnmount(() => {
  timers.forEach((t) => clearInterval(t))
})
</script>

<style scoped>
/* vshell 概览实测：卡片 8px 圆角 / 1px #f0f0f0 边框 / 内容区 padding 16px / 卡头 38px
   自适应：宽度用网格随视口伸缩；面板行自上而下排列，超出视口由本页滚动 */
.vs-dash {
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 16px;
  height: 100%;
  overflow-y: auto;
  box-sizing: border-box;
}

.vs-card {
  background: var(--cs-card-bg);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
}

.vs-dash-cards {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
}

@media (max-width: 1100px) {
  .vs-dash-cards {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

.vs-stat-card {
  padding: 12px;
  min-height: 130px;
  display: flex;
  flex-direction: column;
}

.vs-stat-title {
  font-size: 14px;
  color: var(--cs-active);
  cursor: pointer;
}

.vs-stat-row {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 0;
}

.vs-stat-value {
  font-size: 24px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.vs-dash-panels {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
  align-items: stretch;
}

@media (max-width: 1100px) {
  .vs-dash-panels {
    grid-template-columns: minmax(0, 1fr);
  }
}

.vs-panel {
  display: flex;
  flex-direction: column;
  min-height: 260px;
  overflow: hidden;
}

.vs-panel-head {
  height: 38px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 0 12px;
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
  border-bottom: 1px solid var(--cs-card-border);
}

.vs-panel-sub {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 400;
  color: var(--cs-text-dim);
}

.vs-panel-body {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.vs-kv {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  font-size: 14px;
  color: var(--cs-text);
  min-height: 30px;
}

.vs-kv b {
  font-weight: 400;
  color: var(--cs-text-bright);
  font-family: var(--cs-font-mono);
  text-align: right;
}

.vs-kv b.ellipsis,
.vs-kv span.ellipsis,
.ellipsis {
  max-width: 60%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ellipsis-left {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: var(--cs-font-mono);
}

.vs-gauge-row {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 30px;
  font-size: 14px;
  color: var(--cs-text);
}

.vs-gauge-label {
  width: 76px;
  flex-shrink: 0;
}

.vs-gauge-track {
  flex: 1;
  height: 8px;
  border-radius: 100px;
  background: var(--cs-hover);
  overflow: hidden;
}

.vs-gauge-fill {
  height: 100%;
  border-radius: 100px;
  background: var(--cs-active);
  transition: width 0.3s ease;
}

.vs-gauge-val {
  width: 44px;
  text-align: right;
  flex-shrink: 0;
  color: var(--cs-text);
}

/* ---- 新增面板：三数摘要 / 横向条 / 子标题 / 事件流 ---- */
.vs-triple {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
}

.vs-triple-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  padding: 6px 4px;
  border-radius: 6px;
  background: var(--cs-hover);
}

.vs-triple-item b {
  font-size: 20px;
  font-weight: 600;
  color: var(--cs-text-bright);
  font-family: var(--cs-font-mono);
}

.vs-triple-item b.warn {
  color: #e6a23c;
}

.vs-triple-item span {
  font-size: 12px;
  color: var(--cs-text-dim);
  text-align: center;
}

.vs-bars {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.vs-bar-row {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 13px;
  color: var(--cs-text);
}

.vs-bar-label {
  width: 96px;
  flex-shrink: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.vs-bar-track {
  flex: 1;
  height: 8px;
  border-radius: 100px;
  background: var(--cs-hover);
  overflow: hidden;
}

.vs-bar-fill {
  height: 100%;
  border-radius: 100px;
  background: var(--cs-active);
  transition: width 0.3s ease;
}

.vs-bar-val {
  width: 48px;
  flex-shrink: 0;
  text-align: right;
  font-family: var(--cs-font-mono);
  color: var(--cs-text-bright);
}

.vs-sub {
  margin-top: 4px;
  font-size: 12px;
  color: var(--cs-text-dim);
}

.vs-empty {
  padding: 12px 0;
  font-size: 13px;
  color: var(--cs-text-dim);
}

.vs-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #c0c4cc;
}

.vs-dot.on {
  background: var(--cs-online, #1ab394);
}

.vs-ev {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 26px;
  font-size: 13px;
  color: var(--cs-text);
}

.vs-ev-icon.lv-ok {
  color: var(--cs-online, #1ab394);
}

.vs-ev-icon.lv-warn {
  color: #e6a23c;
}

.vs-ev-icon.lv-muted {
  color: var(--cs-text-dim);
}

.vs-ev-label {
  flex-shrink: 0;
}

.vs-ev-uid {
  font-family: var(--cs-font-mono);
  font-size: 12px;
  color: var(--cs-text-dim);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.vs-ev-time {
  margin-left: auto;
  flex-shrink: 0;
  font-family: var(--cs-font-mono);
  font-size: 12px;
  color: var(--cs-text-dim);
}
</style>
