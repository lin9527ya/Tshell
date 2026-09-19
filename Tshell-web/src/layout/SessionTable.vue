<template>
  <div class="session-table-container cs-font-ui">
    <!-- Search / Count Toolbar(常驻;接原 searchKey 死代码,Ctrl+F 聚焦) -->
    <div class="cs-session-toolbar">
      <input
        ref="searchInputRef"
        v-model="searchKey"
        class="cs-session-search cs-font-mono"
        :placeholder="isZh ? '过滤会话(IP/主机/用户/备注...)' : 'Filter sessions (IP/host/user/note...)'"
        spellcheck="false"
      />
      <span v-if="searchKey" class="cs-session-search-clear" :title="isZh ? '清空过滤' : 'Clear'" @click="searchKey = ''">✕</span>
      <span class="cs-session-count" :title="isZh ? '匹配/总数' : 'matched / total'">
        {{ filteredClients.length }}/{{ clientsStore.clientsList.length }}
      </span>
    </div>

    <!-- Session Table (1:1 CS Column Layout) -->
    <div class="cs-table-wrap">
      <!-- Batch Action Bar (C14: 批量命令 / 分组 / 任务进度) — 绝对定位浮层:
           原 v-if 插在表格上方,勾选首行时整表下移 38px(表头跳变) -->
      <div v-if="batchUids.length" class="cs-batch-bar">
        <span class="batch-count">{{ isZh ? `已选 ${batchUids.length} 个会话` : `${batchUids.length} selected` }}</span>
        <div class="batch-input-wrap">
          <input
            v-model="batchCommandInput"
            class="batch-cmd-input cs-font-mono"
            :placeholder="isZh ? '批量下发命令 (回车发送)...' : 'Batch command (Enter to send)...'"
            spellcheck="false"
            @keydown.enter="sendBatchCommand"
          />
          <button class="cs-btn cs-btn-xs cs-btn-primary" :disabled="!batchCommandInput.trim() || batchSending" @click="sendBatchCommand">
            {{ batchSending ? (isZh ? '下发中...' : 'Sending...') : (isZh ? '下发' : 'Send') }}
          </button>
        </div>
        <div class="batch-actions-group">
          <button class="cs-btn cs-btn-xs" @click="batchSetGroup">{{ isZh ? '批量分组' : 'Set Group' }}</button>
          <button class="cs-btn cs-btn-xs" @click="openBatchTasks">{{ isZh ? '任务进度' : 'Tasks' }}</button>
          <button class="cs-btn cs-btn-xs" :disabled="!hasOfflineSelected" @click="cleanupOfflineBatch">{{ isZh ? '清理离线' : 'Clean Offline' }}</button>
          <button class="cs-btn cs-btn-xs" @click="clearTableSelection">{{ isZh ? '取消选择' : 'Clear' }}</button>
        </div>
      </div>

      <el-table
        ref="tableRef"
        v-loading="loading"
        :data="filteredClients"
        row-key="Uid"
        size="small"
        height="100%"
        highlight-current-row
        :header-cell-style="tableHeaderStyle"
        :row-class-name="getRowClass"
        :row-style="getRowColorStyle"
        @row-contextmenu="onRowContextMenu"
        @row-click="onRowClick"
        @row-dblclick="onRowDblClick"
        @selection-change="onSelectionChange"
        class="cs-session-table"
      >
        <!-- Multi-select Column -->
        <el-table-column type="selection" width="32" :reserve-selection="true" />

        <!-- Icon / Status Column (CS OS Icon with Online/Offline Indicator) -->
        <el-table-column width="42" align="center" fixed="left">
          <template #default="{ row }">
            <div class="beacon-os-icon-wrap" :class="{ online: isClientOnline(row), offline: !isClientOnline(row) }" :title="getOsTooltip(row)">
              <!-- Status dot -->
              <span class="beacon-os-dot" :class="{ online: isClientOnline(row) }"></span>

              <!-- Windows Icon -->
              <svg v-if="getClientOsType(row) === 'windows'" viewBox="0 0 18 18" width="16" height="16" class="beacon-os-svg">
                <path d="M1 3.2 L7.8 2.2 L7.8 8.8 L1 8.8 Z" fill="#00adef" />
                <path d="M8.8 2.1 L17 0.9 L17 8.8 L8.8 8.8 Z" fill="#00adef" />
                <path d="M1 9.8 L7.8 9.8 L7.8 16.4 L1 15.4 Z" fill="#00adef" />
                <path d="M8.8 9.8 L17 9.8 L17 17.7 L8.8 16.5 Z" fill="#00adef" />
              </svg>

              <!-- Linux Tux Penguin Icon -->
              <svg v-else-if="getClientOsType(row) === 'linux'" viewBox="0 0 18 18" width="16" height="16" class="beacon-os-svg">
                <path d="M9 1.5 C6.5 1.5, 5 3.5, 5 6.5 C5 8, 4.5 9.5, 3.5 11 C2.8 12.2, 2.8 13.8, 3.8 14.8 C4.8 15.8, 6.5 16.2, 9 16.2 C11.5 16.2, 13.2 15.8, 14.2 14.8 C15.2 13.8, 15.2 12.2, 14.5 11 C13.5 9.5, 13 8, 13 6.5 C13 3.5, 11.5 1.5, 9 1.5 Z" fill="#222" stroke="#111" stroke-width="0.5" />
                <ellipse cx="9" cy="11.2" rx="3.3" ry="3.8" fill="#ffffff" />
                <ellipse cx="7.3" cy="5" rx="1" ry="1.4" fill="#ffffff" />
                <ellipse cx="10.7" cy="5" rx="1" ry="1.4" fill="#ffffff" />
                <circle cx="7.4" cy="5.2" r="0.5" fill="#000000" />
                <circle cx="10.6" cy="5.2" r="0.5" fill="#000000" />
                <polygon points="7.2,6.5 10.8,6.5 9,8.2" fill="#ffb300" stroke="#f57c00" stroke-width="0.3" />
                <ellipse cx="6.2" cy="16.2" rx="2" ry="0.9" fill="#ffb300" />
                <ellipse cx="11.8" cy="16.2" rx="2" ry="0.9" fill="#ffb300" />
              </svg>

              <!-- macOS Apple Icon -->
              <svg v-else-if="getClientOsType(row) === 'darwin'" viewBox="0 0 18 18" width="16" height="16" class="beacon-os-svg">
                <path d="M11.5 2 C12 1.2 12.5 0.5 12.2 0 C11.6 0.1 10.8 0.6 10.3 1.3 C9.8 1.8 9.4 2.6 9.6 3 C10.4 3 11 2.5 11.5 2 Z M13.8 9 C13.8 7.2 15.2 6.3 15.3 6.2 C14.4 5 13.1 4.8 12.6 4.8 C11.4 4.7 10.3 5.5 9.7 5.5 C9.1 5.5 8.2 4.8 7.3 4.8 C6.1 4.8 5 5.5 4.3 6.6 C3 8.8 4 12.1 5.3 14 C5.9 14.8 6.6 15.8 7.7 15.8 C8.7 15.8 9 15.2 10.2 15.2 C11.3 15.2 11.7 15.8 12.7 15.8 C13.8 15.8 14.4 14.9 15.1 14 C15.9 12.9 16.2 11.8 16.2 11.7 C16.1 11.7 13.8 10.8 13.8 9 Z" fill="var(--cs-text-bright)" />
              </svg>

              <!-- Generic / Monitor Icon -->
              <svg v-else viewBox="0 0 24 24" width="16" height="16" fill="currentColor" class="beacon-os-svg">
                <rect x="2" y="3" width="20" height="14" rx="2" stroke="currentColor" stroke-width="1.5" fill="none" />
                <path d="M8 21h8M12 17v4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" />
                <rect v-if="isClientOnline(row)" x="4" y="5" width="16" height="10" rx="1" fill="#4caf50" opacity="0.8" />
                <rect v-else x="4" y="5" width="16" height="10" rx="1" fill="#555" opacity="0.5" />
              </svg>
            </div>
          </template>
        </el-table-column>

        <!-- external (1:1 CS) -->
        <el-table-column
          prop="ExternalIP"
          :label="t('table.external')"
          min-width="105"
          sortable
          :sort-orders="['ascending', 'descending', null]"
          show-overflow-tooltip
        >
          <template #default="{ row }">
            <span class="cs-font-mono cs-cell-text">{{ row.ExternalIP || '127.0.0.1' }}</span>
          </template>
        </el-table-column>

        <!-- internal (1:1 CS) -->
        <el-table-column
          prop="InternalIP"
          :label="t('table.internal')"
          min-width="110"
          sortable
          :sort-orders="['ascending', 'descending', null]"
          show-overflow-tooltip
        >
          <template #default="{ row }">
            <span class="cs-font-mono cs-cell-text">{{ row.InternalIP || row.ExternalIP || '-' }}</span>
          </template>
        </el-table-column>

        <!-- address (1:1 CS - e.g. 本机地址) -->
        <el-table-column
          prop="Address"
          :label="t('table.address')"
          min-width="85"
          sortable
          :sort-orders="['ascending', 'descending', null]"
          show-overflow-tooltip
        >
          <template #default="{ row }">
            <span class="cs-cell-text">{{ row.Address || (row.InternalIP === '127.0.0.1' ? (isZh ? '本机地址' : 'Local') : row.InternalIP) }}</span>
          </template>
        </el-table-column>

        <!-- listener (1:1 CS) -->
        <el-table-column
          prop="Listener"
          :label="t('table.listener')"
          min-width="80"
          sortable
          :sort-orders="['ascending', 'descending', null]"
          show-overflow-tooltip
        >
          <template #default="{ row }">
            <span class="cs-badge-listener">{{ row.Listener || 'HTTPS' }}</span>
          </template>
        </el-table-column>

        <!-- user (1:1 CS - SYSTEM *) -->
        <el-table-column
          prop="Username"
          :label="t('table.user')"
          min-width="100"
          sortable
          :sort-orders="['ascending', 'descending', null]"
          show-overflow-tooltip
        >
          <template #default="{ row }">
            <span class="cs-cell-text" :class="{ 'is-admin': isAdminUser(row) }">
              {{ row.Username }}{{ isAdminUser(row) ? ' *' : '' }}
            </span>
          </template>
        </el-table-column>

        <!-- computer (1:1 CS) -->
        <el-table-column
          prop="Computer"
          :label="t('table.computer')"
          min-width="100"
          sortable
          :sort-orders="['ascending', 'descending', null]"
          show-overflow-tooltip
        >
          <template #default="{ row }">
            <span class="cs-cell-text">{{ row.Computer || 'UNKNOWN' }}</span>
          </template>
        </el-table-column>

        <!-- note (1:1 CS) -->
        <el-table-column
          prop="Note"
          :label="t('table.note')"
          min-width="90"
          sortable
          :sort-orders="['ascending', 'descending', null]"
          show-overflow-tooltip
        >
          <template #default="{ row }">
            <span class="cs-cell-note">{{ row.Note || '' }}</span>
            <span v-if="row.Group" class="cs-group-tag" :title="`Group: ${row.Group}`">[{{ row.Group }}]</span>
          </template>
        </el-table-column>

        <!-- process (1:1 CS) -->
        <el-table-column
          prop="Process"
          :label="t('table.process')"
          min-width="115"
          sortable
          :sort-orders="['ascending', 'descending', null]"
          show-overflow-tooltip
        >
          <template #default="{ row }">
            <span class="cs-cell-text">{{ row.Process || 'beacon.exe' }}</span>
          </template>
        </el-table-column>

        <!-- pid (1:1 CS) -->
        <el-table-column
          prop="Pid"
          :label="t('table.pid')"
          width="65"
          sortable
          :sort-orders="['ascending', 'descending', null]"
          align="right"
        >
          <template #default="{ row }">
            <span class="cs-font-mono cs-cell-text">{{ row.Pid || '-' }}</span>
          </template>
        </el-table-column>

        <!-- arch (1:1 CS) -->
        <el-table-column
          prop="Arch"
          :label="t('table.arch')"
          width="50"
          align="center"
        >
          <template #default="{ row }">
            <span class="cs-font-mono cs-cell-text">{{ row.Arch || 'x64' }}</span>
          </template>
        </el-table-column>

        <!-- last (1:1 CS - 给足 95px 宽度，左对齐展示，确保中文“最后心跳”绝不换行) -->
        <el-table-column
          prop="last_active"
          :label="t('table.last')"
          min-width="95"
          sortable
          :sort-orders="['ascending', 'descending', null]"
          align="left"
        >
          <template #default="{ row }">
            <span class="cs-cell-last cs-font-mono" :class="{ stale: !row.Online }">
              {{ formatLastActive(row) }}
            </span>
          </template>
        </el-table-column>

        <!-- sleep (1:1 CS - e.g. 35 seconds (15% jitter)) -->
        <el-table-column
          prop="Sleep"
          :label="t('table.sleep')"
          min-width="120"
          show-overflow-tooltip
        >
          <template #default="{ row }">
            <span class="cs-cell-text">{{ formatSleep(row) }}</span>
          </template>
        </el-table-column>
      </el-table>

      <!-- Empty State -->
      <div v-if="!filteredClients.length && !loading" class="cs-empty-watermark">
        <div class="empty-title">{{ t('empty.no_sessions') }}</div>
      </div>
    </div>

    <!-- 1:1 Cobalt Strike Right Click Context Menu -->
    <CsBeaconContextMenu
      v-model:visible="ctxMenu.visible"
      :x="ctxMenu.x"
      :y="ctxMenu.y"
      :client="ctxClient"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useClientsStore, type ClientData } from '@/stores/clients'
import { useWorkspaceStore } from '@/stores/workspace'
import { useI18nStore } from '@/stores/i18n'
import { useVisibilityInterval } from '@/utils/useVisibilityInterval'
import { detectClientOs } from '@/utils/clientUtils'
import ClientAPI from '@/api/clients'
import CsBeaconContextMenu from './ContextMenu/CsBeaconContextMenu.vue'

interface RowData extends ClientData {
  last_active?: string
}

const clientsStore = useClientsStore()
const workspace = useWorkspaceStore()
const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)
const t = (key: string, def?: string) => i18nStore.t(key, def)

const loading = computed(() => clientsStore.loading)
const searchKey = ref('')
const searchInputRef = ref<HTMLInputElement | null>(null)

// 全局快捷键(CsShell Ctrl+F)通过 window 事件聚焦搜索框
function onFocusSearchEvent() {
  searchInputRef.value?.focus()
  searchInputRef.value?.select()
}
const selectedRow = ref<RowData | null>(null)
const ctxClient = ref<RowData | null>(null)
const ctxMenu = ref({ visible: false, x: 0, y: 0 })

const filteredClients = computed(() => {
  const list = clientsStore.clientsList as RowData[]
  if (!searchKey.value.trim()) return list
  const q = searchKey.value.toLowerCase().trim()
  return list.filter((c) => {
    return (
      (c.Computer && c.Computer.toLowerCase().includes(q)) ||
      (c.Username && c.Username.toLowerCase().includes(q)) ||
      (c.ExternalIP && c.ExternalIP.toLowerCase().includes(q)) ||
      (c.InternalIP && c.InternalIP.toLowerCase().includes(q)) ||
      (c.Process && c.Process.toLowerCase().includes(q)) ||
      (c.Note && c.Note.toLowerCase().includes(q))
    )
  })
})

function getClientOsType(row: RowData): string {
  return detectClientOs(row)
}

function getOsTooltip(row: RowData): string {
  const type = detectClientOs(row)
  const status = isClientOnline(row) ? (isZh.value ? '在线' : 'Online') : (isZh.value ? '离线' : 'Offline')
  const osName = type === 'windows' ? 'Windows' : (type === 'linux' ? 'Linux' : (type === 'darwin' ? 'macOS' : 'Unknown'))
  return `${osName} (${status})`
}

function isAdminUser(row: RowData): boolean {
  if (!row.Username) return false
  const u = row.Username.toLowerCase()
  return u.includes('system') || u.includes('administrator') || u.includes('root') || row.Username.includes('*')
}

function formatSleep(row: RowData): string {
  // CS 风格："5s +20%"；DB 存纯秒数或 "秒 jitter%"（SleepDialog 写入）
  const raw = row.Sleep && typeof row.Sleep === 'string' ? row.Sleep.trim() : ''
  if (!raw) return '5s'
  if (raw.includes('%')) {
    const parts = raw.split(/\s+/)
    const sec = parseInt(parts[0]) || 5
    const jit = parts.find((p) => p.includes('%')) || ''
    return `${sec}s ${jit ? '+' + jit : ''}`
  }
  const s = parseInt(raw) || 5
  return `${s}s`
}

// 相对时间格式化（秒 → "Ns"/"Nm"/"Nh"）
function relTime(sec: number): string {
  if (sec < 60) return `${Math.max(0, Math.floor(sec))}s`
  if (sec < 3600) return `${Math.floor(sec / 60)}m`
  return `${Math.floor(sec / 3600)}h`
}

// 解析后端时间戳（"2006-01-02 15:04:05"）为相对当前的时间差
function parseTs(ts: string | undefined | null): number | null {
  if (!ts) return null
  const t = new Date(ts.replace(' ', 'T'))
  if (isNaN(t.getTime())) return null
  return (Date.now() - t.getTime()) / 1000
}

function formatLastActive(row: RowData): string {
  const online = isClientOnline(row)
  // 读取 ticker 建立响应式依赖(每秒刷新)
  void nowTick.value
  const ts = row.last_active || (online ? row.FirstStart : row.FirstStart)
  const diff = parseTs(ts)
  if (diff !== null && diff >= 0) {
    if (online) {
      // CS 风格倒计时:距下次心跳的秒数(心跳间隔 - 距最后心跳)
      const interval = heartbeatIntervalSeconds(row)
      const next = Math.max(0, interval - diff)
      if (next <= 0) return isZh ? '心跳中' : 'checking in'
      return isZh ? `下次心跳 ${next}s` : `next ${next}s`
    }
    return (isZh ? '已离线 ' : 'off ') + relTime(diff)
  }
  // 无时间戳回退：按上线时长或离线占位
  if (row.FirstStart) {
    const d = parseTs(row.FirstStart)
    if (d !== null) return online ? relTime(d) : (isZh ? '已离线' : 'offline')
  }
  return online ? '0s' : (isZh ? '已离线' : '1908h')
}

// 心跳间隔(秒):优先 Sleep 字段(客户端 sleep 设置),兜底监听器配置/5
function heartbeatIntervalSeconds(row: RowData): number {
  const raw = String(row.Sleep || '').trim()
  const n = parseInt(raw, 10)
  if (n > 0 && n <= 3600) return n
  const hb = parseInt(String(row.HeartbeatInterval || ''), 10)
  if (hb > 0 && hb <= 3600) return hb
  return 5
}

function isClientOnline(row: RowData): boolean {
  return row.Online === '1' || row.Online === 'true' || row.Online === true as any
}

function getRowClass({ row }: { row: RowData }) {
  const selected = selectedRow.value?.Uid === row.Uid
  const isBatchSelected = workspace.selectedUids.includes(row.Uid)
  const online = isClientOnline(row)
  return [
    'cs-session-row',
    (selected || isBatchSelected) ? 'is-selected' : '',
    online ? 'is-online' : 'is-offline',
  ].filter(Boolean).join(' ')
}

function getRowColorStyle({ row }: { row: RowData }) {
  const color = workspace.sessionColors[row.Uid] || row.Color
  if (color && color !== '#ffffff') {
    return {
      // td 背景读取该变量（见 .cs-table-wrap :deep(.el-table__row td) 样式），
      // 明亮主题下也可见标记色调，避免被纯白底盖住
      '--el-table-tr-bg-color': `${color}3d`,
    }
  }
  return {}
}

const tableHeaderStyle = {
  background: 'var(--cs-table-header)',
  color: 'var(--cs-text)',
  fontWeight: '600',
  fontSize: '17px',
  borderBottom: '1px solid var(--cs-border)',
  height: '32px',
  padding: '4px 0',
}

function onRowClick(row: RowData) {
  selectedRow.value = row
  // 如果当前有多选勾选状态（大于1项），单击单行只切换当前激活查看项，不破坏批量勾选；单选或无勾选时同步更新
  if (selectedRows.value.length <= 1) {
    workspace.setSelectedUids([row.Uid])
  }
}

function onRowDblClick(row: RowData) {
  selectedRow.value = row
  if (selectedRows.value.length <= 1) {
    workspace.setSelectedUids([row.Uid])
  }
  const label = `${row.Username}@${row.Computer || row.ExternalIP}`
  workspace.openTab({
    id: `beacon:${row.Uid}`,
    title: isZh.value ? `会话交互: ${label}` : `Beacon: ${label}`,
    component: 'beacon',
    params: { uid: row.Uid },
  })
}

function onRowContextMenu(row: RowData, column: any, event: MouseEvent) {
  event.preventDefault()
  selectedRow.value = row
  // 如果右键的行不在已勾选的批量列表中，则切换为选中该行；如果在多选列表中，则保留批量多选以支持批量右键操作
  if (!workspace.selectedUids.includes(row.Uid)) {
    workspace.setSelectedUids([row.Uid])
  }
  ctxClient.value = row
  ctxMenu.value = {
    visible: true,
    x: event.clientX,
    y: event.clientY,
  }
}

const selectedRows = ref<RowData[]>([])
const hasOfflineSelected = computed(() => selectedRows.value.some((r) => !isClientOnline(r)))

function onSelectionChange(selection: RowData[]) {
  selectedRows.value = selection
  const uids = selection.map((s) => s.Uid)
  workspace.setSelectedUids(uids)
  if (selection.length > 0) {
    selectedRow.value = selection[selection.length - 1]
  } else {
    selectedRow.value = null
  }
}

// ===== C14 批量命令 / 分组 / 任务进度 =====
const batchUids = computed(() => workspace.selectedUids)
const batchCommandInput = ref('')
const batchSending = ref(false)

// 批量清理离线会话记录(仅删除已离线记录; 在线客户端不受影响)
async function cleanupOfflineBatch() {
  const offline = selectedRows.value.filter((r) => !isClientOnline(r)).map((r) => r.Uid)
  if (!offline.length) {
    ElMessage.info(isZh.value ? '所选会话全部在线，无需清理' : 'All selected sessions are online')
    return
  }
  try {
    await ElMessageBox.confirm(
      isZh.value ? `确定删除 ${offline.length} 个离线客户端的会话记录吗？` : `Remove session records of ${offline.length} offline client(s)?`,
      isZh.value ? '清理离线记录' : 'Clean Up Offline Records',
      { confirmButtonText: isZh.value ? '清理' : 'Clean Up', cancelButtonText: isZh.value ? '取消' : 'Cancel', type: 'warning' },
    )
    const res = await ClientAPI.batch_delete_clients({ uids: offline })
    if (res.data?.status === 200) {
      ElMessage.success(isZh.value ? `已清理 ${offline.length} 个离线会话记录` : `${offline.length} offline session(s) cleaned up`)
    } else {
      ElMessage.error(res.data?.message || res.data?.data || (isZh.value ? '清理失败' : 'Clean up failed'))
    }
    await clientsStore.refreshClients()
    clearTableSelection()
  } catch (e: any) {
    if (e === 'cancel' || e?.message?.includes?.('cancel')) return
    ElMessage.error(e?.message || (isZh.value ? '清理失败' : 'Clean up failed'))
  }
}

async function sendBatchCommand() {
  const uids = batchUids.value
  const cmd = batchCommandInput.value.trim()
  if (!uids.length || !cmd) return
  batchSending.value = true
  try {
    const res = await ClientAPI.batch_command({ uids, command: cmd })
    if (res.data?.status === 200) {
      ElMessage.success(isZh.value ? `已向 ${uids.length} 个会话下发批量命令` : `Batch command sent to ${uids.length} session(s)`)
      batchCommandInput.value = ''
      openBatchTasks()
    } else {
      ElMessage.error(res.data?.data || (isZh.value ? '批量命令下发失败' : 'Batch command failed'))
    }
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '批量命令下发失败' : 'Batch command failed'))
  } finally {
    batchSending.value = false
  }
}

async function batchSetGroup() {
  const uids = batchUids.value
  if (!uids.length) return
  try {
    const { value } = await ElMessageBox.prompt(
      isZh.value ? '输入分组名称（为空则清除分组）：' : 'Enter group name (empty clears group):',
      isZh.value ? '批量设置分组' : 'Batch Set Group',
      { confirmButtonText: isZh.value ? '设置' : 'Set', cancelButtonText: isZh.value ? '取消' : 'Cancel' },
    )
    const res = await ClientAPI.batch_update_client_group({ uids, group: value || '' })
    if (res.data?.status === 200 || res.data?.success) {
      ElMessage.success(isZh.value ? `已为 ${uids.length} 个会话设置分组` : `Group set for ${uids.length} session(s)`)
      clientsStore.refreshClients()
    } else {
      ElMessage.error(res.data?.data || res.data?.message || (isZh.value ? '设置分组失败' : 'Failed to set group'))
    }
  } catch {
    /* 用户取消或失败 */
  }
}

function openBatchTasks() {
  workspace.openTab({ id: 'batch-tasks', title: isZh.value ? '批量任务' : 'Batch Tasks', component: 'batch-tasks' })
}

const tableRef = ref<any>(null)

function clearTableSelection() {
  tableRef.value?.clearSelection()
  selectedRows.value = []
  workspace.setSelectedUids([])
  selectedRow.value = null
}

const { start, stop } = useVisibilityInterval(() => {
  clientsStore.fetchClients()
}, 5000)

// 心跳倒计时:1s ticker 强制"下次心跳"实时刷新(CS 风格)
const nowTick = ref(Date.now())
let tickTimer: number | null = null

onMounted(() => {
  clientsStore.fetchClients()
  start()
  tickTimer = window.setInterval(() => { nowTick.value = Date.now() }, 1000)
  window.addEventListener('cs-focus-session-search', onFocusSearchEvent)
})

onBeforeUnmount(() => {
  stop()
  if (tickTimer) window.clearInterval(tickTimer)
  window.removeEventListener('cs-focus-session-search', onFocusSearchEvent)
})
</script>

<style scoped>
.session-table-container {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-panel);
  overflow: hidden;
}

.cs-table-wrap {
  flex: 1;
  overflow: hidden;
  position: relative;
}

/* 常驻搜索工具栏(接原 searchKey 死代码) */
.cs-session-toolbar {
  height: 32px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 12px;
  background: var(--cs-titlebar-bg);
  border-bottom: 1px solid var(--cs-border);
}

.cs-session-search {
  flex: 1;
  max-width: 340px;
  height: 24px;
  background: var(--cs-bg);
  border: 1px solid var(--cs-border);
  border-radius: 2px;
  color: var(--cs-text);
  padding: 0 8px;
  font-size: 15px;
  outline: none;
}

.cs-session-search:focus {
  border-color: var(--cs-active);
}

.cs-session-search-clear {
  cursor: pointer;
  color: var(--cs-text-dim);
  font-size: 14px;
}

.cs-session-search-clear:hover {
  color: var(--cs-text-bright);
}

.cs-session-count {
  margin-left: auto;
  color: var(--cs-text-dim);
  font-size: 14.5px;
}

.beacon-os-icon-wrap {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
}
.beacon-os-icon-wrap.offline {
  opacity: 0.55;
  filter: grayscale(0.8);
}
.beacon-os-dot {
  position: absolute;
  top: -1px;
  right: -2px;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--cs-offline, #888888);
  border: 1px solid var(--cs-panel, #ffffff);
  box-shadow: 0 0 2px rgba(0, 0, 0, 0.4);
}
.beacon-os-dot.online {
  background: #4caf50;
  box-shadow: 0 0 4px rgba(76, 175, 80, 0.8);
}
.beacon-os-svg {
  display: block;
}

.cs-cell-text {
  font-size: 17px;
  color: var(--cs-text-bright, #111111);
}
.cs-cell-text.is-admin {
  font-weight: 600;
  color: var(--cs-text-bright, #000000);
}

.cs-cell-last {
  font-size: 16px;
  font-weight: bold;
  font-style: italic;
  color: var(--cs-text-bright, #111111);
}
.cs-cell-last.stale {
  font-weight: bold;
  font-style: italic;
  color: var(--cs-text-dim, #777777);
  opacity: 0.8;
}

.cs-badge-listener {
  font-size: 16px;
  padding: 1px 6px;
  border-radius: 2px;
  background: color-mix(in srgb, var(--cs-active) 15%, transparent);
  color: var(--cs-active);
  font-family: var(--cs-font-mono);
}

/* C14 Batch Action Bar */
.cs-batch-bar {
  min-height: 38px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 4px 12px;
  flex-shrink: 0;
  flex-wrap: wrap;
  /* 浮层化: 勾选时覆盖在表格上方,不再把整表下推 38px(表头跳变) */
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  z-index: 6;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.28);
}
.batch-count {
  font-size: 15px;
  color: var(--cs-active);
  font-weight: 600;
  white-space: nowrap;
}
.batch-input-wrap {
  display: flex;
  align-items: center;
  gap: 4px;
  flex: 1;
  min-width: 220px;
  max-width: 520px;
}
.batch-cmd-input {
  flex: 1;
  height: 26px;
  background: var(--cs-bg);
  border: 1px solid var(--cs-border);
  border-radius: 2px;
  color: var(--cs-text);
  font-size: 15px;
  padding: 0 8px;
  outline: none;
}
.batch-cmd-input:focus {
  border-color: var(--cs-active);
}
.batch-actions-group {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}

.cs-cell-note {
  font-size: 16px;
  color: var(--cs-accent, #673ab7);
  font-style: italic;
}

.cs-group-tag {
  margin-left: 6px;
  font-size: 15px;
  font-style: normal;
  color: var(--cs-active);
  font-family: var(--cs-font-mono);
  opacity: 0.85;
}

.cs-empty-watermark {
  position: absolute;
  top: 45%;
  left: 50%;
  transform: translate(-50%, -50%);
  text-align: center;
  pointer-events: none;
}
.empty-title {
  font-size: 18px;
  color: var(--cs-text-dim);
}
</style>

<style>
/* 1:1 Cobalt Strike Session Table Styles */
.cs-session-table {
  background: var(--cs-panel) !important;
  color: var(--cs-text-bright) !important;
  font-size: 17px !important;
}
.cs-session-table .el-table__header th {
  background: var(--cs-table-header) !important;
  color: var(--cs-text) !important;
  padding: 4px 0 !important;
  font-size: 17px !important;
  border-bottom: 1px solid var(--cs-border) !important;
}
.cs-session-table .el-table__header th .cell {
  white-space: nowrap !important;
  word-break: keep-all !important;
  padding: 0 6px !important;
  line-height: 26px !important;
}
.cs-session-table .el-table__row {
  height: 32px !important;
  cursor: default !important;
  color: var(--cs-text-bright) !important;
}
.cs-session-table .el-table__row td {
  padding: 4px 0 !important;
  border-bottom: 1px solid color-mix(in srgb, var(--cs-border) 40%, transparent) !important;
  /* 会话标记色通过行内 --el-table-tr-bg-color 传入；未标记时回落面板底色 */
  background-color: var(--el-table-tr-bg-color, var(--cs-panel)) !important;
  color: var(--cs-text-bright) !important;
}
.cs-session-table .el-table__row td .cell {
  white-space: nowrap !important;
  padding: 0 6px !important;
  line-height: 26px !important;
  color: inherit !important;
}
.cs-session-table .el-table__row:hover td {
  background-color: var(--cs-hover) !important;
}

/* 未选中行明确文字颜色，杜绝白底白字 */
.cs-session-table .el-table__row:not(.current-row):not(.is-selected) .cs-cell-text {
  color: var(--cs-text-bright) !important;
}
.cs-session-table .el-table__row:not(.current-row):not(.is-selected) .cs-cell-last {
  color: var(--cs-text-bright) !important;
}

/* 选中行：CS 经典深蓝底纯白字，保持强对比 */
.cs-session-table .el-table__row.current-row > td,
.cs-session-table .el-table__row.is-selected > td {
  background-color: #1a5c9a !important;
  color: #ffffff !important;
}
.cs-session-table .el-table__row.current-row > td .cs-cell-text,
.cs-session-table .el-table__row.current-row > td .cs-cell-last,
.cs-session-table .el-table__row.current-row > td .cs-cell-note,
.cs-session-table .el-table__row.current-row > td span,
.cs-session-table .el-table__row.is-selected > td .cs-cell-text,
.cs-session-table .el-table__row.is-selected > td .cs-cell-last,
.cs-session-table .el-table__row.is-selected > td .cs-cell-note,
.cs-session-table .el-table__row.is-selected > td span {
  color: #ffffff !important;
}
</style>
