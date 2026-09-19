<template>
  <div class="web-log-panel cs-font-ui" @click="closeContextMenu">
    <!-- Toolbar -->
    <div class="wl-toolbar">
      <div class="wl-title-group">
        <span class="wl-title">{{ isZh ? 'Web 日志' : 'Web Log' }}</span>
        <span class="wl-count">({{ filteredLogs.length }} / {{ logs.length }} {{ isZh ? '条访问记录' : 'hits' }})</span>
      </div>

      <!-- View Switcher -->
      <div class="wl-view-switcher">
        <button
          class="wl-mode-btn"
          :class="{ active: viewMode === 'console' }"
          @click="viewMode = 'console'"
        >
          {{ isZh ? '控制台' : 'Console' }}
        </button>
        <button
          class="wl-mode-btn"
          :class="{ active: viewMode === 'table' }"
          @click="viewMode = 'table'"
        >
          {{ isZh ? '数据表' : 'Table' }}
        </button>
      </div>

      <div class="wl-spacer" />

      <!-- Search Input -->
      <el-input
        v-model="searchKey"
        :placeholder="isZh ? '过滤 IP、URI、UA、状态码...' : 'Filter IP, URI, UA, Status...'"
        size="small"
        clearable
        class="wl-search-input"
      />

      <button
        class="cs-btn"
        :class="{ 'cs-btn-primary': autoRefresh }"
        @click="autoRefresh = !autoRefresh"
      >
        {{ autoRefresh ? (isZh ? '自动刷新: 开' : 'Auto: ON') : (isZh ? '自动刷新: 关' : 'Auto: OFF') }}
      </button>
      <button class="cs-btn" :disabled="loading" @click="refreshLogs">
        {{ isZh ? '刷新' : 'Refresh' }}
      </button>
      <button class="cs-btn" @click="clearLogs">
        {{ isZh ? '清空' : 'Clear' }}
      </button>
    </div>

    <!-- Main Content Area -->
    <div class="wl-content-area">
      <!-- 1. Console Stream View -->
      <div
        v-if="viewMode === 'console'"
        ref="outputRef"
        class="wl-output cs-font-mono"
      >
        <div
          v-for="(log, idx) in filteredLogs"
          :key="idx"
          class="wl-line"
          :class="`wl-${log.type}`"
          @contextmenu.prevent.stop="openContextMenu($event, log)"
        >
          <span class="wl-time">[{{ log.fullTime || log.time }}]</span>
          <span class="wl-ip">{{ log.ip }}</span>
          <span class="wl-method" :class="log.method">{{ log.method }}</span>
          <span class="wl-uri">{{ log.uri }}</span>
          <span class="wl-status-code" :class="`status-${log.status}`">{{ log.status }}</span>
          <span class="wl-ua">({{ log.ua }})</span>
        </div>
        <EmptyState
          v-if="!filteredLogs.length"
          variant="inline"
          :title="isZh ? 'Web 服务器正常监听中，暂无外部访问记录。' : 'Web server is active. Awaiting incoming HTTP requests...'"
        />
      </div>

      <!-- 2. Table View -->
      <div v-else class="wl-table-wrap">
        <el-table
          :data="filteredLogs"
          size="small"
          height="100%"
          highlight-current-row
          class="cs-session-table"
          @row-click="onRowClick"
          @row-contextmenu="onRowContextMenu"
        >
          <el-table-column prop="fullTime" :label="isZh ? '时间' : 'Time'" width="160" sortable>
            <template #default="{ row }">
              <span class="cs-font-mono">{{ row.fullTime || row.time }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="ip" :label="isZh ? '来源 IP' : 'Source IP'" width="140" sortable>
            <template #default="{ row }">
              <span class="cs-font-mono font-bold">{{ row.ip }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="method" :label="isZh ? '方法' : 'Method'" width="80" align="center" sortable>
            <template #default="{ row }">
              <span :class="['method-tag', row.method]">{{ row.method }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="uri" :label="isZh ? '请求路径 / URI' : 'URI'" min-width="180" sortable show-overflow-tooltip>
            <template #default="{ row }">
              <span class="cs-font-mono text-bright">{{ row.uri }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="status" :label="isZh ? '状态码' : 'Status'" width="90" align="center" sortable>
            <template #default="{ row }">
              <span :class="['status-tag', `st-${Math.floor(row.status / 100)}xx`]">{{ row.status }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="ua" :label="isZh ? '客户端标识 (User-Agent)' : 'User-Agent'" min-width="260" sortable show-overflow-tooltip>
            <template #default="{ row }">
              <span class="ua-text">{{ row.ua }}</span>
            </template>
          </el-table-column>

          <template #empty>
            <EmptyState
              variant="inline"
              :title="isZh ? '暂无 Web 访问记录' : 'No Web Hits Logged'"
            />
          </template>
        </el-table>
      </div>
    </div>

    <!-- Bottom Action Bar (CS Native Button Row) -->
    <div class="wl-bottom-bar">
      <button class="cs-btn cs-btn-primary" :disabled="loading" @click="refreshLogs">
        {{ isZh ? '刷新' : 'Refresh' }}
      </button>
      <button class="cs-btn" :disabled="!selectedLog" @click="copySelected">
        {{ isZh ? '复制选定' : 'Copy Selected' }}
      </button>
      <button class="cs-btn" :disabled="!logs.length" @click="exportLogs">
        {{ isZh ? '导出日志' : 'Export Logs' }}
      </button>
      <button class="cs-btn cs-btn-danger" :disabled="!logs.length" @click="clearLogs">
        {{ isZh ? '清空' : 'Clear' }}
      </button>
      <button class="cs-btn" @click="handleHelp">
        {{ isZh ? '帮助' : 'Help' }}
      </button>
    </div>

    <!-- Right-Click Context Menu -->
    <Teleport to="body">
      <div
        v-if="ctxMenu.visible"
        class="cs-listener-ctx-menu cs-font-ui"
        :style="{ left: ctxMenu.x + 'px', top: ctxMenu.y + 'px' }"
        @mousedown.stop
        @contextmenu.prevent
      >
        <div class="ctx-item is-highlight" @click="copyLogField('ip')">
          <span class="ctx-text">{{ isZh ? '复制来源 IP' : 'Copy IP' }}</span>
        </div>
        <div class="ctx-item" @click="copyLogField('uri')">
          <span class="ctx-text">{{ isZh ? '复制请求路径 (URI)' : 'Copy URI' }}</span>
        </div>
        <div class="ctx-item" @click="copyLogField('ua')">
          <span class="ctx-text">{{ isZh ? '复制 User-Agent' : 'Copy User-Agent' }}</span>
        </div>
        <div class="ctx-divider" />
        <div class="ctx-item" @click="copyFullLog">
          <span class="ctx-text">{{ isZh ? '复制整行日志' : 'Copy Full Log Line' }}</span>
        </div>
        <div class="ctx-item" @click="exportLogs">
          <span class="ctx-text">{{ isZh ? '导出全部日志' : 'Export All' }}</span>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18nStore } from '@/stores/i18n'
import { useTabActive, useVisibilityInterval } from '@/utils/useVisibilityInterval'
import EmptyState from '@/components/Common/EmptyState.vue'
import ClientAPI from '@/api/clients'

interface WebLogEntry {
  id?: number | string
  time: string
  fullTime: string
  ip: string
  method: string
  uri: string
  status: number
  ua: string
  type: 'get' | 'post' | 'warn' | 'error'
}

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const viewMode = ref<'console' | 'table'>('console')
const searchKey = ref('')
const autoRefresh = ref(true)
const loading = ref(false)
const outputRef = ref<HTMLElement | null>(null)

const logs = ref<WebLogEntry[]>([])
const selectedLog = ref<WebLogEntry | null>(null)

const ctxMenu = ref({
  visible: false,
  x: 0,
  y: 0,
  row: null as WebLogEntry | null,
})

const filteredLogs = computed(() => {
  const q = searchKey.value.trim().toLowerCase()
  if (!q) return logs.value
  return logs.value.filter(
    (l) =>
      l.ip.toLowerCase().includes(q) ||
      l.uri.toLowerCase().includes(q) ||
      l.ua.toLowerCase().includes(q) ||
      String(l.status).includes(q) ||
      l.method.toLowerCase().includes(q),
  )
})

function formatTime(raw: string): string {
  if (!raw) return ''
  const m = raw.match(/(\d{2}:\d{2}:\d{2})/)
  return m ? m[1] : raw
}

async function refreshLogs() {
  loading.value = true
  try {
    const res = await ClientAPI.GetDeliveryAccessLogs({ limit: 300 })
    if (res.status === 200 && res.data.status === 200) {
      logs.value = (res.data.data || []).map((l: any, idx: number) => {
        const fullTime = l.created_at || ''
        const code = l.status_code ?? 200
        return {
          id: l.id || idx,
          time: formatTime(fullTime),
          fullTime,
          ip: l.client_ip || '-',
          method: (l.method || 'GET').toUpperCase(),
          uri: l.resource ? (l.resource.startsWith('/') ? l.resource : `/${l.resource}`) : '/',
          status: code,
          ua: l.user_agent || '-',
          type: code >= 400 ? 'error' : code >= 300 ? 'warn' : 'get',
        }
      })

      if (viewMode.value === 'console') {
        nextTick(() => {
          if (outputRef.value) {
            outputRef.value.scrollTop = outputRef.value.scrollHeight
          }
        })
      }
    }
  } catch {
    // 保底不影响用户当前操作
  } finally {
    loading.value = false
  }
}

function clearLogs() {
  logs.value = []
  selectedLog.value = null
  ElMessage.success(isZh.value ? 'Web 访问日志已清空' : 'Web log cleared')
}

function onRowClick(row: WebLogEntry) {
  selectedLog.value = row
}

function onRowContextMenu(row: WebLogEntry, col: any, e: MouseEvent) {
  openContextMenu(e, row)
}

function openContextMenu(e: MouseEvent, log: WebLogEntry) {
  selectedLog.value = log
  ctxMenu.value = {
    visible: true,
    x: e.clientX,
    y: e.clientY,
    row: log,
  }
}

function closeContextMenu() {
  ctxMenu.value.visible = false
}

async function copyText(text: string, label: string) {
  closeContextMenu()
  if (!text) return
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(isZh.value ? `已复制 ${label}` : `Copied ${label}`)
  } catch {
    ElMessage.error(isZh.value ? '复制失败' : 'Failed to copy')
  }
}

function copyLogField(field: 'ip' | 'uri' | 'ua') {
  const row = ctxMenu.value.row || selectedLog.value
  if (!row) return
  copyText(row[field], field.toUpperCase())
}

function copyFullLog() {
  const row = ctxMenu.value.row || selectedLog.value
  if (!row) return
  const line = `[${row.fullTime || row.time}] ${row.ip} ${row.method} ${row.uri} ${row.status} (${row.ua})`
  copyText(line, isZh.value ? '整行日志' : 'Log line')
}

function copySelected() {
  copyFullLog()
}

function exportLogs() {
  closeContextMenu()
  if (!logs.value.length) {
    ElMessage.warning(isZh.value ? '暂无可导出日志' : 'No logs to export')
    return
  }
  const content = logs.value
    .map((l) => `[${l.fullTime || l.time}] ${l.ip} ${l.method} ${l.uri} ${l.status} (${l.ua})`)
    .join('\n')
  const blob = new Blob([content], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `web_log_${Date.now()}.txt`
  a.click()
  URL.revokeObjectURL(url)
  ElMessage.success(isZh.value ? 'Web 访问日志导出成功' : 'Web log exported successfully')
}

function handleHelp() {
  ElMessage.info(
    isZh.value
      ? 'Web 日志：实时捕获并记录目标对 Web 投递服务、托管文件及克隆站点的外部 HTTP 访问。'
      : 'Web Log: Captures and displays external HTTP requests to Web Delivery, hosted files, and cloned sites.',
  )
}

// 【性能】自动刷新接入可见性治理(非活动标签/页面隐藏时暂停)
const tabActive = useTabActive()
useVisibilityInterval(
  () => {
    if (autoRefresh.value) refreshLogs()
  },
  5000,
  { isActive: () => tabActive.value },
)

onMounted(() => {
  refreshLogs()
})
</script>

<style scoped>
.web-log-panel {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-panel);
  overflow: hidden;
}

.wl-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 30px;
  padding: 0 8px;
  background: var(--cs-titlebar-bg);
  border-bottom: 1px solid var(--cs-border);
  flex-shrink: 0;
  user-select: none;
}

.wl-title-group {
  display: flex;
  align-items: center;
  gap: 6px;
}

.wl-title {
  font-size: 11px;
  font-weight: 600;
  color: var(--cs-text-dim);
  text-transform: uppercase;
  letter-spacing: .04em;
}

.wl-count {
  font-size: 11px;
  color: var(--cs-text-dim);
}

.wl-view-switcher {
  display: flex;
  border: 1px solid var(--cs-border);
  border-radius: 2px;
  overflow: hidden;
  margin-left: 6px;
}

.wl-mode-btn {
  background: transparent;
  border: none;
  color: var(--cs-text-dim);
  font-size: 11px;
  padding: 2px 8px;
  cursor: pointer;
}

.wl-mode-btn.active {
  background: var(--cs-active);
  color: #ffffff;
  font-weight: bold;
}

.wl-search-input {
  width: 220px;
}

.wl-search-input :deep(.el-input__wrapper) {
  height: 22px;
}

.wl-spacer { flex: 1; }

.wl-content-area {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

/* 1. Console View Style */
.wl-output {
  flex: 1;
  padding: 8px 12px;
  overflow-y: auto;
  font-size: 12px;
  line-height: 1.7;
  background: var(--cs-console);
  color: var(--cs-text);
}

.wl-line {
  display: flex;
  align-items: baseline;
  gap: 8px;
  white-space: pre-wrap;
  word-break: break-all;
  padding: 1px 4px;
  border-radius: 2px;
  cursor: pointer;
}

.wl-line:hover {
  background: var(--cs-hover);
}

.wl-time {
  color: var(--cs-text-dim);
  flex-shrink: 0;
}

.wl-ip {
  color: var(--cs-log-system);
  flex-shrink: 0;
  font-weight: bold;
}

.wl-method {
  padding: 0 4px;
  border-radius: 2px;
  font-size: 11px;
  font-weight: 600;
  flex-shrink: 0;
}

.wl-method.GET { color: var(--cs-online); }
.wl-method.POST { color: var(--cs-warning); }

.wl-uri {
  color: var(--cs-text-bright);
  font-weight: 500;
}

.wl-status-code {
  font-weight: 600;
  flex-shrink: 0;
}

.status-200 { color: var(--cs-online); }
.status-302, .status-301 { color: var(--cs-warning); }
.status-404, .status-500 { color: var(--cs-error); }

.wl-ua {
  color: var(--cs-text-dim);
  font-size: 11px;
}

/* 2. Table View Style */
.wl-table-wrap {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.method-tag {
  font-size: 11px;
  font-weight: bold;
  padding: 1px 4px;
}
.method-tag.GET { color: var(--cs-online); }
.method-tag.POST { color: var(--cs-warning); }

.status-tag {
  font-size: 11px;
  font-weight: bold;
}
.status-tag.st-2xx { color: var(--cs-online); }
.status-tag.st-3xx { color: var(--cs-warning); }
.status-tag.st-4xx, .status-tag.st-5xx { color: var(--cs-error); }

.ua-text {
  font-size: 11px;
  color: var(--cs-text-dim);
}

/* Bottom Bar */
.wl-bottom-bar {
  height: 32px;
  background: var(--cs-titlebar-bg);
  border-top: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
  flex-shrink: 0;
}
</style>
