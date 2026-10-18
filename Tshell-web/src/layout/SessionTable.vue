<template>
  <div class="session-table-container cs-font-ui">
    <!-- Search / Count Toolbar(常驻;接原 searchKey 死代码,Ctrl+F 聚焦) -->
    <div class="cs-session-toolbar">
      <span class="cs-session-title">{{ isZh ? '会话列表' : 'Sessions' }}</span>
      <div class="cs-session-search-wrap">
        <input
          ref="searchInputRef"
          v-model="searchKey"
          class="cs-session-search"
          :placeholder="isZh ? '过滤会话(IP/主机/用户/备注...)' : 'Filter sessions (IP/host/user/note...)'"
          spellcheck="false"
        />
        <span v-if="searchKey" class="cs-session-search-clear" :title="isZh ? '清空过滤' : 'Clear'" @click="searchKey = ''">✕</span>
      </div>
      <span class="cs-session-count" :title="isZh ? '匹配/总数' : 'matched / total'">
        {{ filteredClients.length }}/{{ clientsStore.clientsList.length }}
      </span>

      <!-- 凭据/战利品库入口：/loot 在路由表里是 menu:false，此前全仓没有任何跳转入口，
           只能手输 URL 打开（命令产出自动入库的 WiFi 密码也在这里查看） -->
      <button class="cs-btn cs-btn-xs" @click="openLoot">{{ isZh ? '凭据库' : 'Credentials' }}</button>

      <!-- 批量态（vshell：勾选后操作出现在页头工具条，不再用浮动条） -->
      <template v-if="batchUids.length">
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
        <button class="cs-btn cs-btn-xs" @click="batchSetGroup">{{ isZh ? '批量分组' : 'Set Group' }}</button>
        <button class="cs-btn cs-btn-xs" @click="openBatchTasks">{{ isZh ? '任务中心' : 'Tasks' }}</button>
        <button class="cs-btn cs-btn-xs" :disabled="!hasOfflineSelected" @click="cleanupOfflineBatch">{{ isZh ? '清理离线' : 'Clean Offline' }}</button>
        <button class="cs-btn cs-btn-xs" @click="clearTableSelection">{{ isZh ? '取消选择' : 'Clear' }}</button>
      </template>
    </div>

    <!-- Session Table (1:1 CS Column Layout) -->
    <div class="cs-table-wrap">
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

              <!-- OS Icon(单一来源组件;在线状态用于兜底显示器图标) -->
              <OsIcon :os="getClientOsType(row)" :size="16" fallback="screen" :online="isClientOnline(row)" class="beacon-os-svg" />

              <!-- 多操作员协作：会话被其他操作员占用时给出可见提示 -->
              <VsIcon
                v-if="collabStore.lockedByOther(row.Uid)"
                name="lock"
                :size="11"
                class="beacon-lock-badge"
                :title="`${collabStore.lockedByOther(row.Uid)?.owner} 正在操作该会话`"
              />
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
          width="90"
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
          width="70"
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
          min-width="140"
          sortable
          :sort-orders="['ascending', 'descending', null]"
          align="left"
        >
          <template #default="{ row }">
            <span class="vs-online-tag" :class="row.Online ? 'is-online' : 'is-offline'">
              {{ row.Online ? (isZh ? '在线' : 'ON') : (isZh ? '离线' : 'OFF') }}
            </span>
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

        <!-- 操作（vshell：行内链接 + 更多下拉，替代 CS 右键菜单） -->
        <el-table-column :label="isZh ? '操作' : 'Actions'" width="220" fixed="right">
          <template #default="{ row }">
            <div class="row-actions">
              <a class="row-link" @click="gotoClient(row.Uid, 'console')">{{ isZh ? '主机管理' : 'Manage' }}</a>
              <a class="row-link" @click="gotoClient(row.Uid, 'files')">{{ isZh ? '文件' : 'Files' }}</a>
              <el-dropdown trigger="click" @command="(c: string) => onRowCmd(c, row)">
                <a class="row-link">{{ isZh ? '更多' : 'More' }} ▾</a>
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item v-if="!collabStore.lockOf(row.Uid)" command="lock">
                      {{ isZh ? '占用会话（协作）' : 'Lock Session' }}
                    </el-dropdown-item>
                    <el-dropdown-item v-else-if="collabStore.lockOf(row.Uid)?.owner === userStore.username" command="unlock">
                      {{ isZh ? '释放会话' : 'Release Session' }}
                    </el-dropdown-item>
                    <el-dropdown-item v-else disabled>
                      {{ isZh ? '已被 ' + collabStore.lockOf(row.Uid)?.owner + ' 占用' : 'Locked by ' + collabStore.lockOf(row.Uid)?.owner }}
                    </el-dropdown-item>
                    <el-dropdown-item command="note" divided>{{ isZh ? '备注' : 'Note' }}</el-dropdown-item>
                    <el-dropdown-item command="sleep">{{ isZh ? '回连间隔' : 'Sleep' }}</el-dropdown-item>
                    <el-dropdown-item command="group">{{ isZh ? '设置分组' : 'Set Group' }}</el-dropdown-item>
                    <el-dropdown-item command="alive">{{ isZh ? '检查存活' : 'Check Alive' }}</el-dropdown-item>
                    <el-dropdown-item command="clientReport">{{ isZh ? '会话报告' : 'Session Report' }}</el-dropdown-item>
                    <el-dropdown-item command="migrate" divided>{{ isZh ? '进程迁移' : 'Migrate' }}</el-dropdown-item>
                    <el-dropdown-item command="disconnect">{{ isZh ? '断开连接' : 'Disconnect' }}</el-dropdown-item>
                    <el-dropdown-item command="remove">{{ isZh ? '删除进程文件' : 'Delete Process File' }}</el-dropdown-item>
                    <el-dropdown-item command="cleanup">{{ isZh ? '清理会话记录' : 'Clean Session Record' }}</el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
            </div>
          </template>
        </el-table-column>
      </el-table>

      <!-- Empty State -->
      <EmptyState v-if="!filteredClients.length && !loading" :title="t('empty.no_sessions')" />
    </div>

    <!-- P3：页面内局部弹窗（替代原全局 workspace.dialogs 状态机） -->
    <NoteDialog v-model="noteDlg.visible" :uid="noteDlg.uid" :note="noteDlg.note" />
    <SleepDialog v-model="sleepDlg.visible" :uid="sleepDlg.uid" :sleep="sleepDlg.sleep" :uids="workspace.selectedUids" />
    <ProcessMigrateDialog v-model="migrateDlg.visible" :uid="migrateDlg.uid" :pid="migrateDlg.pid" />
    <ClientReportDialog v-model="reportDlg.visible" :uid="reportDlg.uid" />

  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useClientsStore, type ClientData } from '@/stores/clients'
import { useI18nStore } from '@/stores/i18n'
import { useSessionSelectionStore } from '@/stores/sessionSelection'
import { useCollabStore } from '@/stores/collab'
import { useUserStore } from '@/stores/user'
import VsIcon from '@/layout-vshell/VsIcon.vue'
import { useVisibilityInterval } from '@/utils/useVisibilityInterval'
import { detectClientOs } from '@/utils/clientUtils'
import ClientAPI from '@/api/clients'
import EmptyState from '@/components/Common/EmptyState.vue'
import OsIcon from '@/components/Common/OsIcon.vue'
// P3：会话操作对话框（页面内局部弹窗）
import NoteDialog from './Dialogs/NoteDialog.vue'
import SleepDialog from './Dialogs/SleepDialog.vue'
import ProcessMigrateDialog from './Dialogs/ProcessMigrateDialog.vue'
import ClientReportDialog from './Dialogs/ClientReportDialog.vue'

interface RowData extends ClientData {
  last_active?: string
}

const clientsStore = useClientsStore()
const workspace = useSessionSelectionStore()
const collabStore = useCollabStore()
const userStore = useUserStore()
const router = useRouter()
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
  const ts = row.last_active || row.FirstStart
  const diff = parseTs(ts)
  if (diff !== null && diff >= 0) {
    if (online) {
      // CS 风格倒计时:距下次心跳的秒数(心跳间隔 - 距最后心跳)
      const interval = heartbeatIntervalSeconds(row)
      const next = Math.max(0, interval - diff)
      if (next <= 0) return isZh.value ? '心跳中' : 'checking in'
      return isZh.value ? `下次心跳 ${next}s` : `next ${next}s`
    }
    return (isZh.value ? '已离线 ' : 'off ') + relTime(diff)
  }
  // 无时间戳回退：按上线时长或离线占位
  if (row.FirstStart) {
    const d = parseTs(row.FirstStart)
    if (d !== null) return online ? relTime(d) : (isZh.value ? '已离线' : 'offline')
  }
  return online ? '0s' : (isZh.value ? '已离线' : '1908h')
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

const tableHeaderStyle = {
  background: 'var(--cs-table-head)',
  color: 'var(--cs-text)',
  fontWeight: '600',
  fontSize: '14px',
  borderBottom: '1px solid var(--cs-card-border)',
  height: '48px',
  padding: '0',
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
  // 路由化：进入客户端管理页（命令控制页签）
  router.push(`/client/${row.Uid}/console`)
}

// ---- 行操作列（vshell 风格：行内链接/下拉替代 CS 右键菜单）----
function gotoClient(uid: string, func: string) {
  router.push(`/client/${uid}/${func}`)
}

// ---- 页面内局部弹窗状态（P3：取代 workspace.dialogs 全局状态机）----
const noteDlg = ref({ visible: false, uid: '', note: '' })
const sleepDlg = ref<{ visible: boolean; uid: string; sleep?: number | string }>({ visible: false, uid: '' })
const migrateDlg = ref({ visible: false, uid: '', pid: '' })
const reportDlg = ref({ visible: false, uid: '' })

function onRowCmd(cmd: string, row: RowData) {
  selectedRow.value = row
  workspace.setSelectedUids([row.Uid])
  if (cmd === 'note') {
    noteDlg.value = { visible: true, uid: row.Uid, note: row.Note || '' }
  } else if (cmd === 'sleep') {
    sleepDlg.value = { visible: true, uid: row.Uid, sleep: row.Sleep }
  } else if (cmd === 'group') {
    actSetGroup(row)
  } else if (cmd === 'alive') {
    actAlive(row)
  } else if (cmd === 'clientReport') {
    reportDlg.value = { visible: true, uid: row.Uid }
  } else if (cmd === 'migrate') {
    migrateDlg.value = { visible: true, uid: row.Uid, pid: '' }
  } else if (cmd === 'disconnect') {
    actDisconnect(row)
  } else if (cmd === 'remove') {
    actRemove(row)
  } else if (cmd === 'cleanup') {
    actCleanup(row)
  } else if (cmd === 'lock') {
    actLock(row)
  } else if (cmd === 'unlock') {
    actUnlock(row)
  }
}

// 占用/释放会话：告诉别人"这台我在动"，避免互相踩踏（服务端据此拒绝他人下发）
async function actLock(row: RowData) {
  const { value } = await ElMessageBox.prompt(
    isZh.value ? `占用会话 ${row.Uid.slice(0, 8)}（占用期间其他操作员无法对其下发命令）:` : `Lock session ${row.Uid.slice(0, 8)}:`,
    isZh.value ? '占用会话' : 'Lock Session',
    {
      confirmButtonText: isZh.value ? '占用' : 'Lock',
      cancelButtonText: isZh.value ? '取消' : 'Cancel',
      inputPlaceholder: isZh.value ? '占用原因（可选）' : 'Reason (optional)',
      inputValue: '',
    },
  ).catch(() => ({ value: null as string | null }))
  if (value === null) return
  try {
    await collabStore.acquire(row.Uid, value || '')
    ElMessage.success(isZh.value ? '已占用该会话' : 'Session locked')
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || (isZh.value ? '占用失败' : 'Lock failed'))
  }
}

async function actUnlock(row: RowData) {
  try {
    await collabStore.release(row.Uid)
    ElMessage.success(isZh.value ? '已释放该会话' : 'Session released')
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || (isZh.value ? '释放失败' : 'Release failed'))
  }
}

async function actSetGroup(row: RowData) {
  ElMessageBox.prompt(
    isZh.value ? '设置会话分组（留空清除）:' : 'Set session group (empty clears):',
    isZh.value ? '设置分组' : 'Set Group',
    { confirmButtonText: isZh.value ? '设置' : 'Set', cancelButtonText: isZh.value ? '取消' : 'Cancel', inputValue: row.Group || '' },
  ).then(async ({ value }) => {
    try {
      await ClientAPI.update_client_group({ uid: row.Uid, group: value || '' })
      clientsStore.updateClient(row.Uid, { Group: value || '' })
      ElMessage.success(isZh.value ? '分组已更新' : 'Group updated')
    } catch (e: any) {
      ElMessage.error(e?.message || (isZh.value ? '设置分组失败' : 'Failed to set group'))
    }
  }).catch(() => {})
}

async function actAlive(row: RowData) {
  try {
    const res = await ClientAPI.smart_offline_check({ uid: row.Uid })
    const body = res.data
    const st = String(body?.status || 'unknown')
    const map: Record<string, string> = {
      online: isZh.value ? '在线' : 'Online',
      possibly_busy: isZh.value ? '可能忙碌' : 'Possibly Busy',
      likely_offline: isZh.value ? '疑似离线' : 'Likely Offline',
      offline: isZh.value ? '已离线' : 'Offline',
    }
    if (st === 'offline' || st === 'likely_offline') clientsStore.updateClient(row.Uid, { Online: '2' })
    ElMessageBox.alert(`${row.Username}@${row.Computer || row.ExternalIP}: ${map[st] || st}\n${body?.message || ''}`, isZh.value ? '检查存活' : 'Check Alive', { confirmButtonText: isZh.value ? '确定' : 'OK' })
    clientsStore.refreshClients().catch(() => {})
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '检测失败' : 'Check failed'))
  }
}

function actDisconnect(row: RowData) {
  ElMessageBox.confirm(
    isZh.value ? '确定断开该会话的客户端进程吗？会话记录将保留在列表中。' : 'Kill client process on this session? The session record will be kept.',
    isZh.value ? '断开连接' : 'Disconnect',
    { type: 'warning', confirmButtonText: isZh.value ? '断开' : 'Disconnect', cancelButtonText: isZh.value ? '取消' : 'Cancel' },
  ).then(async () => {
    await ClientAPI.disconnect_client({ uid: row.Uid })
    clientsStore.fetchClients()
    ElMessage.success(isZh.value ? '已发送断开指令' : 'Disconnect sent')
  }).catch(() => {})
}

function actRemove(row: RowData) {
  if (row.Online !== '1') {
    ElMessage.warning(isZh.value ? '仅在线会话可删除进程文件' : 'Process file deletion requires an online session')
    return
  }
  ElMessageBox.confirm(
    isZh.value ? '确定删除被控端进程文件吗？客户端保持在线（Linux unlink / Windows 重命名解锁）。' : 'Delete the client process file? The beacon stays online.',
    isZh.value ? '删除进程文件' : 'Delete Process File',
    { type: 'warning', confirmButtonText: isZh.value ? '删除' : 'Delete', cancelButtonText: isZh.value ? '取消' : 'Cancel' },
  ).then(async () => {
    await ClientAPI.batch_command({ uids: [row.Uid], command: 'selfdelete' })
    ElMessage.success(isZh.value ? '已下发删除指令' : 'Tasked')
  }).catch(() => {})
}

function actCleanup(row: RowData) {
  if (row.Online === '1') {
    ElMessage.info(isZh.value ? '该会话在线，无需清理' : 'Session is online')
    return
  }
  ElMessageBox.confirm(
    isZh.value ? '确定删除该离线会话的记录吗？' : 'Remove this offline session record?',
    isZh.value ? '清理会话记录' : 'Clean Session Record',
    { type: 'warning', confirmButtonText: isZh.value ? '清理' : 'Clean', cancelButtonText: isZh.value ? '取消' : 'Cancel' },
  ).then(async () => {
    await ClientAPI.batch_delete_clients({ uids: [row.Uid] })
    clientsStore.fetchClients()
    ElMessage.success(isZh.value ? '已清理' : 'Cleaned')
  }).catch(() => {})
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
  router.push('/tasks')
}

function openLoot() {
  router.push('/loot')
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
  collabStore.start()
  tickTimer = window.setInterval(() => { nowTick.value = Date.now() }, 1000)
  window.addEventListener('cs-focus-session-search', onFocusSearchEvent)
})

onBeforeUnmount(() => {
  stop()
  collabStore.stop()
  if (tickTimer) window.clearInterval(tickTimer)
  window.removeEventListener('cs-focus-session-search', onFocusSearchEvent)
})
</script>

<style scoped>
/* 会话锁标记（多操作员协作） */
.beacon-lock-badge {
  position: absolute;
  right: -2px;
  bottom: -2px;
  color: #f0883e;
}

/* 行操作列（vshell：文字链接 + 下拉） */
.row-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.row-link {
  color: var(--cs-active);
  font-size: 14px;
  cursor: pointer;
  white-space: nowrap;
}

.row-link:hover {
  text-decoration: underline;
}

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

/* ===== 工具条（vshell：白底 + 1px 分隔线，左标题/搜索，右计数） ===== */
.cs-session-toolbar {
  flex-shrink: 0;
  min-height: 48px;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 0 16px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-card-border);
}

.cs-session-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
  white-space: nowrap;
}

.cs-session-search-wrap {
  position: relative;
  display: flex;
  align-items: center;
  width: 320px;
  max-width: 46vw;
}

.cs-session-search {
  width: 100%;
  height: 32px;
  padding: 0 28px 0 10px;
  font-family: var(--cs-font-ui);
  font-size: 14px;
  color: var(--cs-text);
  background: var(--cs-panel);
  border: 1px solid var(--cs-border-strong);
  border-radius: 6px;
  outline: none;
  transition: border-color 120ms ease;
}

.cs-session-search::placeholder {
  color: var(--cs-text-dim);
}

.cs-session-search:focus {
  border-color: var(--cs-active);
}

.cs-session-search-clear {
  position: absolute;
  right: 9px;
  font-size: 12px;
  line-height: 1;
  color: var(--cs-text-dim);
  cursor: pointer;
}

.cs-session-search-clear:hover {
  color: var(--cs-text);
}

.cs-session-count {
  margin-left: auto;
  font-family: var(--cs-font-mono);
  font-size: 13px;
  color: var(--cs-text-dim);
  white-space: nowrap;
}

/* ===== 单元格渲染 ===== */
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
  background: var(--cs-offline);
  border: 1px solid var(--cs-panel);
}
.beacon-os-dot.online {
  background: var(--cs-online);
}
.beacon-os-svg {
  display: block;
}

.cs-cell-text {
  color: var(--cs-text);
}

.cs-cell-text.is-admin {
  font-weight: 600;
  color: var(--cs-elevated);
}

.cs-cell-last {
  font-weight: 500;
  font-style: italic;
  color: var(--cs-text);
}

.cs-cell-last.stale {
  font-weight: 600;
  font-style: italic;
  color: var(--cs-text-dim);
}

/* 在线/离线标签（vshell 客户端列表观感：#1ab394 实心 tag，20px 高 / 4px 圆角） */
.vs-online-tag {
  display: inline-flex;
  align-items: center;
  height: 20px;
  padding: 0 7px;
  margin-right: 6px;
  border-radius: 4px;
  font-size: 12px;
  font-style: normal;
  line-height: 1;
  vertical-align: middle;
}

.vs-online-tag.is-online {
  background: var(--cs-online);
  color: #fff;
}

.vs-online-tag.is-offline {
  background: color-mix(in srgb, var(--cs-offline) 22%, transparent);
  color: var(--cs-text-dim);
}

.cs-badge-listener {
  padding: 1px 6px;
  border-radius: 4px;
  font-family: var(--cs-font-mono);
  font-size: 12px;
  background: color-mix(in srgb, var(--cs-active) 12%, transparent);
  color: var(--cs-active);
}

.cs-cell-note {
  font-style: italic;
  color: var(--cs-text-dim);
}

.cs-group-tag {
  margin-left: 6px;
  font-family: var(--cs-font-mono);
  font-size: 13px;
  font-style: normal;
  color: var(--cs-active);
}

/* ===== C14 批量操作条（浮层，覆盖在表格上方） ===== */
.cs-batch-bar {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  z-index: 6;
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  min-height: 48px;
  padding: 6px 16px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-card-border);
  box-shadow: 0 3px 6px -4px rgba(0, 0, 0, 0.12), 0 6px 16px 0 rgba(0, 0, 0, 0.08);
}

.batch-count {
  font-size: 13px;
  font-weight: 600;
  color: var(--cs-active);
  white-space: nowrap;
}

.batch-input-wrap {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1;
  min-width: 220px;
  max-width: 520px;
}

.batch-cmd-input {
  flex: 1;
  height: 32px;
  padding: 0 10px;
  font-family: var(--cs-font-mono);
  font-size: 13px;
  color: var(--cs-text);
  background: var(--cs-panel);
  border: 1px solid var(--cs-border-strong);
  border-radius: 6px;
  outline: none;
}

.batch-cmd-input::placeholder {
  color: var(--cs-text-dim);
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
</style>

