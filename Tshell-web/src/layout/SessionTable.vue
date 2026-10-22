<template>
  <div class="session-table-container cs-font-ui">
    <!-- Search / Count Toolbar(常驻;接原 searchKey 死代码,Ctrl+F 聚焦) -->
    <div class="cs-session-toolbar">
      <div class="cs-session-search-wrap">
        <input
          ref="searchInputRef"
          v-model="searchKey"
          class="cs-session-search"
          :placeholder="i18nStore.tl('过滤会话(IP/主机/用户/备注...)', 'Filter sessions (IP/host/user/note...)')"
          spellcheck="false"
        />
        <span v-if="searchKey" class="cs-session-search-clear" :title="i18nStore.tl('清空过滤', 'Clear')" @click="searchKey = ''">✕</span>
      </div>
      <span class="cs-session-count" :title="i18nStore.tl('匹配 / 总数', 'matched / total')">
        {{ i18nStore.tl('匹配', 'Matched') }} {{ filteredClients.length }}/{{ clientsStore.clientsList.length }}
      </span>

      <!-- 凭据/战利品库入口：/loot 在路由表里是 menu:false，此前全仓没有任何跳转入口，
           只能手输 URL 打开（命令产出自动入库的 WiFi 密码也在这里查看） -->
      <button class="cs-btn cs-btn-xs" @click="openLoot">{{ i18nStore.tl('凭据库', 'Credentials') }}</button>

      <!-- 正向连接（vshell：客户端列表右上角主按钮；原入口在监听管理-更多里） -->
      <button class="cs-btn cs-btn-xs cs-btn-primary" @click="forwardDlg = true">
        <VsIcon name="plus" :size="11" />
        {{ i18nStore.tl('正向连接', 'Forward Connect') }}
      </button>

      <!-- 批量态（vshell：勾选后操作出现在页头工具条，不再用浮动条） -->
      <template v-if="batchUids.length">
        <span class="batch-count">{{ i18nStore.tl(`已选 ${batchUids.length} 个会话`, `${batchUids.length} selected`) }}</span>
        <div class="batch-input-wrap">
          <input
            v-model="batchCommandInput"
            class="batch-cmd-input cs-font-mono"
            :placeholder="i18nStore.tl('批量下发命令 (回车发送)...', 'Batch command (Enter to send)...')"
            spellcheck="false"
            @keydown.enter="sendBatchCommand"
          />
          <button class="cs-btn cs-btn-xs cs-btn-primary" :disabled="!batchCommandInput.trim() || batchSending" @click="sendBatchCommand">
            {{ batchSending ? (i18nStore.tl('下发中...', 'Sending...')) : (i18nStore.tl('下发', 'Send')) }}
          </button>
        </div>
        <button class="cs-btn cs-btn-xs" @click="batchSetGroup">{{ i18nStore.tl('批量分组', 'Set Group') }}</button>
        <button class="cs-btn cs-btn-xs" @click="openBatchTasks">{{ i18nStore.tl('任务中心', 'Tasks') }}</button>
        <button class="cs-btn cs-btn-xs" :disabled="!hasOfflineSelected" @click="cleanupOfflineBatch">{{ i18nStore.tl('清理离线', 'Clean Offline') }}</button>
        <button class="cs-btn cs-btn-xs" :disabled="!hasOnlineSelected || upgrading" @click="upgradeSelected">{{ upgrading ? (i18nStore.tl('升级中...', 'Upgrading...')) : (i18nStore.tl('升级客户端', 'Upgrade')) }}</button>
        <button class="cs-btn cs-btn-xs" @click="clearTableSelection">{{ i18nStore.tl('取消选择', 'Clear') }}</button>
      </template>

      <!-- vshell 表格工具：刷新 / 密度 / 列设置 -->
      <TableTools
        :columns="columnOptions"
        :visible="colVisible"
        :density="density"
        :loading="manualRefreshing"
        @update:visible="colVisible = $event"
        @update:density="onDensityCmd"
        @refresh="refreshClients"
      />
    </div>

    <!-- Session Table (1:1 CS Column Layout) -->
    <div class="cs-table-wrap">
      <el-table
        ref="tableRef"
        v-loading="loading"
        :data="pagedClients"
        row-key="Uid"
        size="small"
        height="100%"
        highlight-current-row
        :header-cell-style="tableHeaderStyle"
        :row-class-name="getRowClass"
        @row-click="onRowClick"
        @row-dblclick="onRowDblClick"
        @selection-change="onSelectionChange"
        :class="['cs-session-table', density === 'default' ? '' : 'cs-density-' + density]"
      >
        <!-- Multi-select Column -->
        <el-table-column type="selection" width="32" :reserve-selection="true" />

        <!-- ID（数据库自增 ID；缺失时退化为 Uid 前 8 位，保证列永远不空） -->
        <el-table-column
          v-if="colVisible.id"
          prop="ID"
          :label="t('table.id')"
          width="64"
          align="center"
          sortable
          :sort-orders="['ascending', 'descending', null]"
        >
          <template #default="{ row }">
            <span class="cs-font-mono cs-cell-text">{{ clientId(row) }}</span>
            <!-- 协作占用提示：原来挂在最左侧图标列的锁徽标挪到这里，省掉整列 -->
            <VsIcon
              v-if="collabStore.lockedByOther(row.Uid)"
              name="lock"
              :size="11"
              class="beacon-lock-badge"
              :title="i18nStore.tl(`${collabStore.lockedByOther(row.Uid)?.owner} 正在操作该会话`, `${collabStore.lockedByOther(row.Uid)?.owner} is controlling this session`)"
            />
          </template>
        </el-table-column>

        <!-- note (1:1 CS)：单元格内点铅笔即可编辑，不用走右键菜单 -->
        <el-table-column
          v-if="colVisible.note"
          prop="Note"
          :label="t('table.note')"
          width="144"
          sortable
          :sort-orders="['ascending', 'descending', null]"
          show-overflow-tooltip
        >
          <template #default="{ row }">
            <div class="cs-note-cell" @click.stop="openNote(row)">
              <span class="cs-cell-note" :title="row.Note || ''">{{ row.Note || '' }}</span>
              <span v-if="row.Group" class="cs-group-tag" :title="`Group: ${row.Group}`">[{{ row.Group }}]</span>
              <VsIcon name="pen" :size="11" class="cs-note-edit" :title="i18nStore.tl('编辑备注', 'Edit note')" />
            </div>
          </template>
        </el-table-column>

        <!-- listener（连接方式：tcp/https/dns…） -->
        <el-table-column
          v-if="colVisible.listener"
          prop="Listener"
          :label="t('table.listener')"
          width="104"
          sortable
          :sort-orders="['ascending', 'descending', null]"
          show-overflow-tooltip
        >
          <template #default="{ row }">
            <span class="cs-badge-listener">{{ row.Listener || 'HTTPS' }}</span>
          </template>
        </el-table-column>

        <!-- external (1:1 CS) -->
        <el-table-column
          v-if="colVisible.external"
          prop="ExternalIP"
          :label="t('table.external')"
          min-width="136"
          sortable
          :sort-orders="['ascending', 'descending', null]"
          show-overflow-tooltip
        >
          <template #default="{ row }">
            <span class="cs-font-mono cs-cell-text">{{ row.ExternalIP || '127.0.0.1' }}</span>
          </template>
        </el-table-column>

        <!-- address（归属地）：与两侧 IP 列同宽，保证表头落在外网IP/内网IP 正中间 -->
        <el-table-column
          v-if="colVisible.address"
          prop="Address"
          :label="t('table.address')"
          min-width="136"
          sortable
          :sort-orders="['ascending', 'descending', null]"
          show-overflow-tooltip
        >
          <template #default="{ row }">
            <span class="cs-cell-text">{{ row.Address || (row.InternalIP === '127.0.0.1' ? (i18nStore.tl('本机地址', 'Local')) : row.InternalIP) }}</span>
          </template>
        </el-table-column>

        <!-- internal (1:1 CS) -->
        <el-table-column
          v-if="colVisible.internal"
          prop="InternalIP"
          :label="t('table.internal')"
          min-width="136"
          sortable
          :sort-orders="['ascending', 'descending', null]"
          show-overflow-tooltip
        >
          <template #default="{ row }">
            <span class="cs-font-mono cs-cell-text">{{ row.InternalIP || row.ExternalIP || '-' }}</span>
          </template>
        </el-table-column>

        <!-- user (1:1 CS - SYSTEM *) -->
        <el-table-column
          v-if="colVisible.user"
          prop="Username"
          :label="t('table.user')"
          min-width="112"
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
          v-if="colVisible.computer"
          prop="Computer"
          :label="t('table.computer')"
          min-width="110"
          sortable
          :sort-orders="['ascending', 'descending', null]"
          show-overflow-tooltip
        >
          <template #default="{ row }">
            <span class="cs-cell-text">{{ row.Computer || 'UNKNOWN' }}</span>
          </template>
        </el-table-column>

        <!-- 系统类型（windows_amd64 / linux_amd64 / darwin_arm64，一个字段说清系统与架构） -->
        <el-table-column
          v-if="colVisible.osarch"
          prop="Os"
          :label="t('table.osarch')"
          width="160"
          sortable
          :sort-orders="['ascending', 'descending', null]"
          show-overflow-tooltip
        >
          <template #default="{ row }">
            <span class="cs-os-cell">
              <VsIcon :name="osIconName(row)" :size="16" class="cs-os-icon" />
              <span class="cs-font-mono cs-cell-text">{{ osArchLabel(row) }}</span>
            </span>
          </template>
        </el-table-column>

        <!-- process (1:1 CS) -->
        <el-table-column
          v-if="colVisible.process"
          prop="Process"
          :label="t('table.process')"
          min-width="104"
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
          v-if="colVisible.pid"
          prop="Pid"
          :label="t('table.pid')"
          width="64"
          sortable
          :sort-orders="['ascending', 'descending', null]"
          align="right"
        >
          <template #default="{ row }">
            <span class="cs-font-mono cs-cell-text">{{ row.Pid || '-' }}</span>
          </template>
        </el-table-column>

        <!-- 状态（在线 / 离线）：在线只给标签，离线直接给出掉线的时间点 -->
        <el-table-column
          v-if="colVisible.status"
          prop="last_active"
          :label="t('table.status')"
          width="178"
          sortable
          :sort-orders="['ascending', 'descending', null]"
          align="center"
        >
          <template #default="{ row }">
            <span class="cs-status-cell">
              <!-- 用 isClientOnline 判断：Online 是字符串，"2"（离线）也是真值 -->
              <span class="vs-online-tag" :class="isClientOnline(row) ? 'is-online' : 'is-offline'">
                {{ isClientOnline(row) ? (i18nStore.tl('在线', 'ON')) : (i18nStore.tl('离线', 'OFF')) }}
              </span>
              <!-- 在线不再显示"心跳中/下次心跳 Ns"；离线时间才有信息量 -->
              <span
                v-if="!isClientOnline(row)"
                class="cs-cell-last cs-font-mono stale"
                :title="lastActiveTitle(row)"
              >
                {{ offlineSince(row) }}
              </span>
            </span>
          </template>
        </el-table-column>

        <!-- 间隔（回连 sleep，如 "5s +20%"） -->
        <el-table-column
          v-if="colVisible.sleep"
          prop="Sleep"
          :label="t('table.sleep')"
          width="96"
          show-overflow-tooltip
        >
          <template #default="{ row }">
            <span class="cs-cell-text">{{ formatSleep(row) }}</span>
          </template>
        </el-table-column>

        <!-- 操作（vshell：行内链接 + 更多下拉，替代 CS 右键菜单） -->
        <el-table-column :label="i18nStore.tl('操作', 'Actions')" width="172" fixed="right">
          <template #default="{ row }">
            <div class="row-actions">
              <a class="row-link" @click="gotoClient(row.Uid, 'console')">{{ i18nStore.tl('主机管理', 'Manage') }}</a>
              <a class="row-link" @click="gotoClient(row.Uid, 'files')">{{ i18nStore.tl('文件', 'Files') }}</a>
              <el-dropdown trigger="click" @command="(c: string) => onRowCmd(c, row)">
                <a class="row-link">{{ i18nStore.tl('更多', 'More') }} ▾</a>
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item v-if="!collabStore.lockOf(row.Uid)" command="lock">
                      {{ i18nStore.tl('占用会话（协作）', 'Lock Session') }}
                    </el-dropdown-item>
                    <el-dropdown-item v-else-if="collabStore.lockOf(row.Uid)?.owner === userStore.username" command="unlock">
                      {{ i18nStore.tl('释放会话', 'Release Session') }}
                    </el-dropdown-item>
                    <el-dropdown-item v-else disabled>
                      {{ i18nStore.tl('已被 ' + collabStore.lockOf(row.Uid)?.owner + ' 占用', 'Locked by ' + collabStore.lockOf(row.Uid)?.owner) }}
                    </el-dropdown-item>
                    <el-dropdown-item command="note" divided>{{ i18nStore.tl('备注', 'Note') }}</el-dropdown-item>
                    <el-dropdown-item command="sleep">{{ i18nStore.tl('回连间隔', 'Sleep') }}</el-dropdown-item>
                    <el-dropdown-item command="group">{{ i18nStore.tl('设置分组', 'Set Group') }}</el-dropdown-item>
                    <el-dropdown-item command="alive">{{ i18nStore.tl('检查存活', 'Check Alive') }}</el-dropdown-item>
                    <el-dropdown-item v-if="isWindowsSession(row)" command="cred" divided>
                      {{ i18nStore.tl('凭据获取', 'Credential Access') }}
                    </el-dropdown-item>
                    <el-dropdown-item command="clientReport">{{ i18nStore.tl('会话报告', 'Session Report') }}</el-dropdown-item>
                    <el-dropdown-item command="migrate" divided>{{ i18nStore.tl('进程迁移', 'Migrate') }}</el-dropdown-item>
                    <el-dropdown-item command="disconnect">{{ i18nStore.tl('断开连接', 'Disconnect') }}</el-dropdown-item>
                    <el-dropdown-item command="remove">{{ i18nStore.tl('删除进程文件', 'Delete Process File') }}</el-dropdown-item>
                    <el-dropdown-item command="cleanup">{{ i18nStore.tl('清理会话记录', 'Clean Session Record') }}</el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
            </div>
          </template>
        </el-table-column>

        <!-- 空状态放进表格空插槽：避免与 Element Plus 自带「暂无数据」叠加 -->
        <template #empty>
          <EmptyState variant="inline" :title="t('empty.no_sessions')" />
        </template>
      </el-table>
    </div>

    <!-- 底部分页（前端分页：搜索/过滤作用于全量，翻页只切显示窗口） -->
    <div class="cs-table-pager">
      <el-pagination
        small
        background
        layout="prev, pager, next, sizes, total"
        :total="filteredClients.length"
        :current-page="page"
        :page-size="pageSize"
        :page-sizes="[20, 50, 100]"
        @current-change="onPageChange"
        @size-change="onSizeChange"
      />
    </div>

    <!-- P3：页面内局部弹窗（替代原全局 workspace.dialogs 状态机） -->
    <NoteDialog v-model="noteDlg.visible" :uid="noteDlg.uid" :note="noteDlg.note" />
    <SleepDialog v-model="sleepDlg.visible" :uid="sleepDlg.uid" :sleep="sleepDlg.sleep" :uids="workspace.selectedUids" />
    <ProcessMigrateDialog v-model="migrateDlg.visible" :uid="migrateDlg.uid" :pid="migrateDlg.pid" />
    <ClientReportDialog v-model="reportDlg.visible" :uid="reportDlg.uid" />
    <CredentialDumpDialog v-model="credDlg.visible" :uid="credDlg.uid" />
    <ForwardConnectDialog v-model="forwardDlg" />

  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useClientsStore, type ClientData } from '@/stores/clients'
import { useI18nStore } from '@/stores/i18n'
import { useSessionSelectionStore } from '@/stores/sessionSelection'
import { useCollabStore } from '@/stores/collab'
import { useUserStore } from '@/stores/user'
import VsIcon from '@/layout-vshell/VsIcon.vue'
import { useVisibilityInterval } from '@/utils/useVisibilityInterval'
import { clientOsIcon, detectClientOs } from '@/utils/clientUtils'
import ClientAPI from '@/api/clients'
import EmptyState from '@/components/Common/EmptyState.vue'
// P3：会话操作对话框（页面内局部弹窗）
import NoteDialog from './Dialogs/NoteDialog.vue'
import SleepDialog from './Dialogs/SleepDialog.vue'
import ProcessMigrateDialog from './Dialogs/ProcessMigrateDialog.vue'
import ClientReportDialog from './Dialogs/ClientReportDialog.vue'
import CredentialDumpDialog from './Dialogs/CredentialDumpDialog.vue'
import ForwardConnectDialog from './Dialogs/Attacks/ForwardConnectDialog.vue'

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

// ---- 表格工具：刷新 / 密度 / 列设置（vshell 客户端列表右上角） ----
const DENSITY_KEY = 'tshell-client-density'
type Density = 'default' | 'compact' | 'mini'
const density = ref<Density>('default')
try {
  const saved = localStorage.getItem(DENSITY_KEY)
  if (saved === 'default' || saved === 'compact' || saved === 'mini') density.value = saved
} catch { /* 隐私模式不可用 */ }

function onDensityCmd(cmd: string) {
  if (cmd !== 'default' && cmd !== 'compact' && cmd !== 'mini') return
  density.value = cmd
  try { localStorage.setItem(DENSITY_KEY, cmd) } catch { /* 忽略 */ }
}

type ColKey =
  | 'id' | 'note' | 'listener' | 'external' | 'address' | 'internal' | 'user'
  | 'computer' | 'osarch' | 'process' | 'pid' | 'status' | 'sleep'
const COLUMNS_KEY = 'tshell-client-columns'
const DEFAULT_COLS: Record<ColKey, boolean> = {
  id: true, note: true, listener: true, external: true, address: true, internal: true,
  user: true, computer: true, osarch: true, process: true, pid: true, status: true, sleep: true,
}
const colVisible = ref<Record<ColKey, boolean>>({ ...DEFAULT_COLS })
try {
  const raw = localStorage.getItem(COLUMNS_KEY)
  if (raw) {
    const saved = JSON.parse(raw) as Partial<Record<ColKey, boolean>>
    for (const k of Object.keys(DEFAULT_COLS) as ColKey[]) {
      if (typeof saved[k] === 'boolean') colVisible.value[k] = saved[k] as boolean
    }
  }
} catch { /* 损坏的本地配置按默认列显示 */ }
watch(colVisible, (v) => {
  try { localStorage.setItem(COLUMNS_KEY, JSON.stringify(v)) } catch { /* 忽略 */ }
}, { deep: true })

const columnOptions = computed<{ key: ColKey; label: string }[]>(() => [
  { key: 'id', label: i18nStore.tl('ID', 'ID') },
  { key: 'note', label: i18nStore.tl('备注', 'Note') },
  { key: 'listener', label: i18nStore.tl('连接方式', 'Connection') },
  { key: 'external', label: i18nStore.tl('外网IP', 'External IP') },
  { key: 'address', label: i18nStore.tl('归属地', 'Location') },
  { key: 'internal', label: i18nStore.tl('内网IP', 'Internal IP') },
  { key: 'user', label: i18nStore.tl('用户', 'User') },
  { key: 'computer', label: i18nStore.tl('主机名', 'Hostname') },
  { key: 'osarch', label: i18nStore.tl('系统类型', 'OS Type') },
  { key: 'process', label: i18nStore.tl('进程', 'Process') },
  { key: 'pid', label: i18nStore.tl('PID', 'PID') },
  { key: 'status', label: i18nStore.tl('状态', 'Status') },
  { key: 'sleep', label: i18nStore.tl('间隔', 'Sleep') },
])

const manualRefreshing = ref(false)
function refreshClients() {
  if (manualRefreshing.value) return
  manualRefreshing.value = true
  clientsStore.fetchClients().catch(() => {}).finally(() => { manualRefreshing.value = false })
}

// 正向连接（入口从监听管理-更多移到本页工具条）
const forwardDlg = ref(false)

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

// ---- 分页 ----
const page = ref(1)
const pageSize = ref(50)
const pagedClients = computed(() => {
  const maxPage = Math.max(1, Math.ceil(filteredClients.value.length / pageSize.value))
  const p = Math.min(page.value, maxPage)
  const start = (p - 1) * pageSize.value
  return filteredClients.value.slice(start, start + pageSize.value)
})
function onPageChange(p: number) { page.value = p }
function onSizeChange(s: number) { pageSize.value = s; page.value = 1 }
watch(searchKey, () => { page.value = 1 })

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

// 解析后端时间戳（"2006-01-02 15:04:05"）为相对当前的时间差
function parseTs(ts: string | undefined | null): number | null {
  if (!ts) return null
  const t = new Date(ts.replace(' ', 'T'))
  if (isNaN(t.getTime())) return null
  return (Date.now() - t.getTime()) / 1000
}

// 离线时间：直接给掉线的那一刻（"已离线 5m"看不出具体是几点掉的）
function offlineSince(row: RowData): string {
  // 读取 ticker 建立响应式依赖(每秒刷新)
  void nowTick.value
  const ts = row.last_active || row.FirstStart
  if (parseTs(ts) === null) return '-'
  return stampOf(ts)
}

// 掉线时间戳：列表里显示到分钟（悬停给秒级全量）
function stampOf(ts: string | undefined | null, withSeconds = false): string {
  if (!ts) return '-'
  const t = new Date(ts.replace(' ', 'T'))
  if (isNaN(t.getTime())) return ts
  const p = (n: number) => String(n).padStart(2, '0')
  const date = `${t.getFullYear()}-${p(t.getMonth() + 1)}-${p(t.getDate())}`
  const time = `${p(t.getHours())}:${p(t.getMinutes())}${withSeconds ? ':' + p(t.getSeconds()) : ''}`
  return `${date} ${time}`
}

function lastActiveTitle(row: RowData): string {
  const online = isClientOnline(row)
  const ts = row.last_active || row.FirstStart
  const when = stampOf(ts, true)
  if (online) return isZh.value ? `在线，最后心跳 ${when}` : `online, last heartbeat ${when}`
  return isZh.value ? `离线于 ${when}` : `offline since ${when}`
}

// 列表 ID：优先数据库自增 ID，缺失时用 Uid 前 8 位（不显示完整 UUID，避免把列撑爆）
function clientId(row: RowData): string {
  const id = row.ID ?? row.id
  if (id !== undefined && id !== null && String(id) !== '') return String(id)
  return row.Uid ? row.Uid.slice(0, 8) : '-'
}

// 系统类型（系统/架构合并一列）：windows_amd64 / linux_amd64 / darwin_arm64
// OS 取自 detectClientOs（与注入/会话判定同一来源），架构做归一化（x64→amd64、x86→386）
function normalizeArch(raw: string | undefined): string {
  const a = String(raw || '').trim().toLowerCase()
  if (!a) return 'amd64'
  if (a === 'x64' || a === 'x86_64' || a === 'amd64') return 'amd64'
  if (a === 'x86' || a === 'i386' || a === 'i686' || a === '386') return '386'
  if (a === 'aarch64') return 'arm64'
  return a
}

function osArchLabel(row: RowData): string {
  const os = detectClientOs(row) || 'unknown'
  return `${os}_${normalizeArch(row.Arch)}`
}

// 系统类型图标（vshell：devicon windows8 / devicon linux / wpf macos）
function osIconName(row: RowData): string {
  return clientOsIcon(row)
}

function isClientOnline(row: RowData): boolean {
  // Online 可能是 '1'/'true'/true（服务端历史字段类型不一致），统一按字符串比较
  return row.Online === '1' || String(row.Online) === 'true'
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
  fontSize: '15px',
  borderBottom: '1px solid var(--cs-card-border)',
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
const credDlg = ref({ visible: false, uid: '' })

// 备注单元格里的铅笔 / 右键菜单「备注」共用同一个弹窗
function openNote(row: RowData) {
  selectedRow.value = row
  workspace.setSelectedUids([row.Uid])
  noteDlg.value = { visible: true, uid: row.Uid, note: row.Note || '' }
}

function onRowCmd(cmd: string, row: RowData) {
  selectedRow.value = row
  workspace.setSelectedUids([row.Uid])
  if (cmd === 'note') {
    openNote(row)
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
  } else if (cmd === 'cred') {
    credDlg.value = { visible: true, uid: row.Uid }
  }
}

// isWindowsSession 凭据获取当前只支持 Windows（技术路径均为 Windows 专有）
function isWindowsSession(row: RowData): boolean {
  return detectClientOs(row) === 'windows'
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
const hasOnlineSelected = computed(() => selectedRows.value.some((r) => isClientOnline(r)))
const upgrading = ref(false)

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

// 升级客户端：重新生成载荷并下发 self_update（仅独立 EXE payload）
async function upgradeSelected() {
  const uids = selectedRows.value.filter((r) => isClientOnline(r)).map((r) => r.Uid)
  if (!uids.length) {
    ElMessage.warning(isZh.value ? '请选择在线会话' : 'Select online sessions first')
    return
  }
  try {
    await ElMessageBox.confirm(
      isZh.value
        ? `将为 ${uids.length} 个在线会话重新生成载荷并下发 self_update。

仅适用于独立 EXE payload；DLL/注入/迁移形态会被服务端拒绝，旧客户端没有内置保护，误操作可能替换宿主程序文件。

升级载荷不带上线口令，客户端替换后约 2 秒重启。确认继续？`
        : `Regenerate payloads for ${uids.length} online session(s) and send self_update.

Only for standalone EXE payloads; DLL/injected/migrated clients are rejected by the server. Old clients have no built-in guard — a wrong target can overwrite the host process file.

The upgraded payload has no startup password; the client restarts in ~2s. Continue?`,
      isZh.value ? '升级客户端' : 'Upgrade Clients',
      { type: 'warning', confirmButtonText: isZh.value ? '确认升级' : 'Upgrade', cancelButtonText: isZh.value ? '取消' : 'Cancel' },
    )
  } catch {
    return
  }
  upgrading.value = true
  try {
    const res = await ClientAPI.upgradeClients({ uids, confirm: true })
    const data = res.data?.data
    if (res.data?.status === 200 && data) {
      const failed = (data.results || []).filter((r: any) => !r.ok)
      if (failed.length) {
        const detail = failed.map((r: any) => `${r.uid}: ${r.message}`).join('; ')
        ElMessage.warning(
          isZh.value
            ? `已下发 ${data.succeeded} 个，失败 ${data.failed} 个：${detail}`
            : `${data.succeeded} sent, ${data.failed} failed: ${detail}`,
        )
      } else {
        ElMessage.success(isZh.value ? `已向 ${data.succeeded} 个会话下发升级命令` : `Upgrade command sent to ${data.succeeded} session(s)`)
      }
      clientsStore.refreshClients().catch(() => {})
    } else {
      ElMessage.error(res.data?.error || res.data?.data || (isZh.value ? '升级失败' : 'Upgrade failed'))
    }
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || e?.message || (isZh.value ? '升级失败' : 'Upgrade failed'))
  } finally {
    upgrading.value = false
  }
}

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
  display: inline-block;
  margin-left: 4px;
  color: #f0883e;
  vertical-align: -1px;
}

/* 行操作列（vshell：文字链接 + 下拉） */
.row-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.row-link {
  color: var(--cs-active);
  font-size: 15px;
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
  font-size: 15px;
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
  font-size: 15px;
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
  font-size: 13px;
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
  font-size: 14px;
  color: var(--cs-text-dim);
  white-space: nowrap;
}

/* ===== 单元格渲染 ===== */

.cs-cell-text {
  color: var(--cs-text);
}

.cs-os-cell {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  min-width: 0;
}

.cs-os-icon {
  color: var(--cs-text-dim);
  flex-shrink: 0;
}

.cs-cell-text.is-admin {
  font-weight: 600;
  color: var(--cs-elevated);
}

.cs-cell-last {
  font-size: 13px;
  font-weight: 500;
  font-style: italic;
  color: var(--cs-text);
  /* "已离线 101 h" 中间的空格会把 h 挤到下一行，行高被撑高 */
  white-space: nowrap;
}

/* 状态列：标签与离线时间同排，时间用小一号等宽字，避免把 48px 行撑成两行 */
.cs-status-cell {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  white-space: nowrap;
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
  border-radius: 4px;
  font-size: 13px;
  font-style: normal;
  line-height: 1;
  vertical-align: middle;
  flex-shrink: 0;
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
  font-size: 13px;
  background: color-mix(in srgb, var(--cs-active) 12%, transparent);
  color: var(--cs-active);
}

.cs-cell-note {
  font-style: italic;
  color: var(--cs-text-dim);
}

/* 备注单元格：整格可点，铅笔只在悬停时显眼，避免每行都挂个按钮抢视线 */
.cs-note-cell {
  display: flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
  cursor: text;
}

/* 备注文字按内容占宽：短备注不会把分组标签/铅笔推到单元格最右侧，形成空洞 */
.cs-note-cell .cs-cell-note {
  flex: 0 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 备注铅笔常显（弱化），悬停整格时提到全亮，避免"要猜哪里能点" */
.cs-note-edit {
  flex-shrink: 0;
  opacity: 0.65;
  transition: opacity 0.15s;
  color: var(--cs-text-dim);
}

.cs-note-cell:hover .cs-note-edit {
  opacity: 1;
}

.cs-note-edit:hover {
  opacity: 1;
  color: var(--cs-active);
}

.cs-group-tag {
  flex-shrink: 0;
  font-family: var(--cs-font-mono);
  font-size: 13px;
  font-style: normal;
  color: var(--cs-active);
  white-space: nowrap;
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
  font-size: 14px;
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
  font-size: 14px;
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

.cs-table-pager {
  flex-shrink: 0;
  display: flex;
  justify-content: flex-end;
  padding: 8px 16px;
  background: var(--cs-panel);
  border-top: 1px solid var(--cs-card-border);
}
</style>
