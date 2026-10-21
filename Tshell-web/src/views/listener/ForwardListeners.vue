<template>
  <div class="cs-fl-panel cs-font-ui" @click="closeContextMenu">
    <!-- vshell 工具条：独立模式带标题/搜索；嵌入式只留操作（标题与搜索由父面板提供） -->
    <div class="cs-fl-toolbar">
      <template v-if="!embedded">
        <div class="cs-title-group">
          <span class="cs-title">{{ i18nStore.tl('正向监听器', 'Forward Listeners') }}</span>
          <span class="cs-count">({{ forwardList.length }} {{ i18nStore.tl('个', 'total') }})</span>
        </div>

        <div class="cs-spacer" />

        <el-input
          v-model="localSearchKey"
          :placeholder="i18nStore.tl('过滤类型、地址、备注...', 'Filter type, address, note...')"
          clearable
          class="cs-search"
        >
          <template #prefix>
            <svg class="cs-search-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
              <circle cx="11" cy="11" r="7" />
              <path d="M20 20l-3.6-3.6" />
            </svg>
          </template>
        </el-input>

        <button class="cs-btn" :disabled="loading" @click="fetchListeners">
          {{ i18nStore.tl('刷新', 'Refresh') }}
        </button>

        <span class="cs-toolbar-divider" />
      </template>

      <button class="cs-btn cs-btn-primary" @click="openAddDialog">{{ i18nStore.tl('添加', 'Add') }}</button>
      <button class="cs-btn" @click="handleHelp">{{ i18nStore.tl('帮助', 'Help') }}</button>

      <div class="cs-spacer" v-if="embedded" />
      <span class="cs-status-hint" v-if="embedded">
        {{ i18nStore.tl(`共 ${filteredListeners.length} 个正向监听器`, `${filteredListeners.length} forward listener(s)`) }}
      </span>

      <!-- vshell 表格工具：刷新 / 密度 / 列设置（正向监听器） -->
      <TableTools
        :columns="columnOptions"
        :visible="colVisible"
        :density="density"
        :loading="manualRefreshing"
        @update:visible="colVisible = $event"
        @update:density="setDensity"
        @refresh="refreshForward"
      />
    </div>

    <!-- Forward Listeners Table (CS Style) -->
    <div class="cs-table-container">
      <el-table
        v-loading="loading"
        :data="filteredListeners"
        size="small"
        height="100%"
        highlight-current-row
        :row-style="getRowStyle"
        :class="['cs-session-table', density === 'default' ? '' : 'cs-density-' + density]"
        @row-click="onRowClick"
        @row-dblclick="onRowDblClick"
        @row-contextmenu="onRowContextMenu"
      >
        <!-- 与反向监听器同一套列：ID / 备注 / 模式 / 监听地址 / 外网连接地址 / 超时心跳 / 心跳间隔 / 间隔 / 状态 / 操作 -->
        <el-table-column v-if="colVisible.id" prop="Id" label="ID" width="64" align="center" sortable :sort-orders="['ascending', 'descending', null]">
          <template #default="{ row }">
            <span class="cs-font-mono cs-cell-text">{{ row.Id ?? '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column v-if="colVisible.note" prop="Note" :label="i18nStore.tl('备注', 'Note')" width="160" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="cs-cell-text">{{ row.Note || '' }}</span>
          </template>
        </el-table-column>
        <el-table-column v-if="colVisible.mode" prop="Type" :label="i18nStore.tl('模式', 'Mode')" width="120" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="cs-cell-text cs-mode-text">{{ row.Type || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column v-if="colVisible.listenAddr" prop="ListenAddress" :label="i18nStore.tl('监听地址', 'Listen Address')" width="180" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="cs-font-mono cs-cell-text">{{ row.ListenAddress || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column v-if="colVisible.connectAddr" prop="ConnectAddress" :label="i18nStore.tl('外网连接地址', 'Connect Address')" width="180" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="row.ConnectAddress" class="cs-font-mono cs-cell-text">{{ row.ConnectAddress }}</span>
            <span v-else class="fl-target-missing">{{ i18nStore.tl('未设置', 'Not set') }}</span>
          </template>
        </el-table-column>
        <el-table-column v-if="colVisible.sleep" prop="SleepSeconds" width="90" sortable align="right" :sort-orders="['ascending', 'descending', null]">
          <template #header>
            <span :title="i18nStore.tl('生效 beacon 节奏（监听器配置 > C2 画像 > 全局默认）', 'Effective beacon rhythm (listener > C2 profile > global)')">{{ i18nStore.tl('间隔', 'Sleep') }}</span>
          </template>
          <template #default="{ row }">
            <span class="cs-cell-text" :title="sleepTitle(row)">{{ sleepLabel(row) }}</span>
          </template>
        </el-table-column>
        <el-table-column v-if="colVisible.status" :label="i18nStore.tl('状态', 'Status')" width="100" sortable align="center">
          <template #default="{ row }">
            <span class="vs-status-tag" :class="row.Status === 1 ? 'is-online' : 'is-offline'">
              {{ row.Status === 1 ? (i18nStore.tl('监听中', 'Listening')) : (i18nStore.tl('已停止', 'Stopped')) }}
            </span>
          </template>
        </el-table-column>

        <!-- vshell 操作列：图标 + 文字；启用/停用常显，不可用的一侧置灰 -->
        <el-table-column :label="i18nStore.tl('操作', 'Actions')" min-width="260">
          <template #default="{ row }">
            <div class="row-actions">
              <a class="row-link" @click.stop="handleConnect(row)">
                <VsIcon name="link" :size="12" /><span>{{ i18nStore.tl('连接', 'Connect') }}</span>
              </a>
              <a class="row-link" :class="{ 'is-disabled': row.Status === 1 }" @click.stop="handleStart(row)">
                <VsIcon name="play" :size="12" /><span>{{ i18nStore.tl('启用', 'Start') }}</span>
              </a>
              <a class="row-link" :class="{ 'is-disabled': row.Status !== 1 }" @click.stop="handleStop(row)">
                <VsIcon name="stop" :size="12" /><span>{{ i18nStore.tl('停用', 'Stop') }}</span>
              </a>
              <a class="row-link" @click.stop="openEditDialog(row)">
                <VsIcon name="pen" :size="12" /><span>{{ i18nStore.tl('编辑', 'Edit') }}</span>
              </a>
              <a class="row-link row-link-danger" @click.stop="handleRemove(row)">
                <VsIcon name="trash-alt" :size="12" /><span>{{ i18nStore.tl('删除', 'Remove') }}</span>
              </a>
            </div>
          </template>
        </el-table-column>

        <template #empty>
          <EmptyState variant="inline" icon="🔌" :title="i18nStore.tl('暂无正向监听器', 'No Forward Listeners Configured')" />
        </template>
      </el-table>
    </div>

    <!-- Right-Click Context Menu -->
    <Teleport to="body">
      <div
        v-if="ctxMenu.visible"
        class="cs-fl-ctx-menu cs-font-ui"
        :style="{ left: ctxMenu.x + 'px', top: ctxMenu.y + 'px' }"
        @mousedown.stop
        @contextmenu.prevent
      >
        <div class="ctx-item" @click="handleConnect(ctxMenu.row)">
          <span class="ctx-text">{{ i18nStore.tl('连接', 'Connect') }}</span>
        </div>
        <div class="ctx-divider" />
        <div class="ctx-item" :class="{ disabled: ctxMenu.row?.Status === 1 }" @click="handleStart(ctxMenu.row)">
          <span class="ctx-text">{{ i18nStore.tl('开启监听', 'Start Listener') }}</span>
        </div>
        <div class="ctx-item" :class="{ disabled: ctxMenu.row?.Status !== 1 }" @click="handleStop(ctxMenu.row)">
          <span class="ctx-text">{{ i18nStore.tl('关闭监听', 'Stop Listener') }}</span>
        </div>
        <div class="ctx-item" @click="openEditDialog(ctxMenu.row)">
          <span class="ctx-text">{{ i18nStore.tl('编辑...', 'Edit...') }}</span>
        </div>
        <div class="ctx-divider" />
        <div
          class="ctx-item has-sub"
          @mouseenter="ctxMenu.showColor = true"
          @mouseleave="ctxMenu.showColor = false"
        >
          <span class="ctx-text">{{ i18nStore.tl('颜色', 'Color') }}</span>
          <span class="ctx-arrow">▶</span>

          <div v-show="ctxMenu.showColor" class="ctx-color-palette">
            <div
              v-for="c in colorPalette"
              :key="c.value"
              class="color-box"
              :style="{ backgroundColor: c.value }"
              :title="i18nStore.tl(c.zh, c.en)"
              @click.stop="setColor(c.value)"
            />
          </div>
        </div>
        <div class="ctx-divider" />
        <div class="ctx-item ctx-danger" @click="handleRemove(ctxMenu.row)">
          <span class="ctx-text">{{ i18nStore.tl('删除', 'Remove') }}</span>
        </div>
      </div>
    </Teleport>

    <!-- New / Edit Forward Listener Dialog -->
    <EditListenerDialog
      v-model="showEditDialog"
      :listener-data="editingListener"
      scope="forward"
      :saving="savingListener"
      @save="onListenerSaved"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import EditListenerDialog from '@/layout/Dialogs/EditListenerDialog.vue'
import ListenerAPI from '@/api/listener'
import ClientAPI from '@/api/clients'
import { useI18nStore } from '@/stores/i18n'
import VsIcon from '@/layout-vshell/VsIcon.vue'
import EmptyState from '@/components/Common/EmptyState.vue'

const props = withDefaults(
  defineProps<{
    embedded?: boolean
    externalSearchKey?: string
  }>(),
  {
    embedded: false,
    externalSearchKey: '',
  },
)

const emit = defineEmits<{
  (e: 'count-change', count: number): void
}>()

interface ForwardItem {
  Id?: number
  ID?: number
  id?: number
  Type: string
  ListenAddress: string
  ConnectAddress: string
  Status: number
  HeartbeatTimeout?: string
  HeartbeatInterval?: string
  Note?: string
  VKey?: string
  EncryptionSalt?: string
  Profile?: string
  SleepSeconds?: number
  BeaconSleepSeconds?: number
  BeaconJitterPercent?: number
  EffectiveSleepSeconds?: number
  EffectiveJitterPercent?: number
  color?: string
}

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const forwardList = ref<ForwardItem[]>([])
const loading = ref(false)
const savingListener = ref(false)
const localSearchKey = ref('')

const searchKey = computed({
  get: () => (props.embedded ? (props.externalSearchKey ?? '') : localSearchKey.value),
  set: (v: string) => { localSearchKey.value = v },
})

const selectedRow = ref<ForwardItem | null>(null)
const editingListener = ref<ForwardItem | null>(null)
const showEditDialog = ref(false)

// ---- vshell 表格工具：密度 / 列设置（正向监听器） ----
const DENSITY_KEY = 'tshell-forward-listener-density'
type Density = 'default' | 'compact' | 'mini'
const density = ref<Density>('default')
try {
  const saved = localStorage.getItem(DENSITY_KEY)
  if (saved === 'default' || saved === 'compact' || saved === 'mini') density.value = saved
} catch { /* 隐私模式不可用 */ }

function setDensity(cmd: string) {
  if (cmd !== 'default' && cmd !== 'compact' && cmd !== 'mini') return
  density.value = cmd
  try { localStorage.setItem(DENSITY_KEY, cmd) } catch { /* 忽略 */ }
}

type ColKey = 'id' | 'note' | 'mode' | 'listenAddr' | 'connectAddr' | 'sleep' | 'status'
const COLUMNS_KEY = 'tshell-forward-listener-columns'
const DEFAULT_COLS: Record<ColKey, boolean> = {
  id: true, note: true, mode: true, listenAddr: true, connectAddr: true,
  sleep: true, status: true,
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
  { key: 'id', label: 'ID' },
  { key: 'note', label: i18nStore.tl('备注', 'Note') },
  { key: 'mode', label: i18nStore.tl('模式', 'Mode') },
  { key: 'listenAddr', label: i18nStore.tl('监听地址', 'Listen Address') },
  { key: 'connectAddr', label: i18nStore.tl('外网连接地址', 'Connect Address') },
  { key: 'sleep', label: i18nStore.tl('间隔', 'Sleep') },
  { key: 'status', label: i18nStore.tl('状态', 'Status') },
])

const manualRefreshing = ref(false)
async function refreshForward() {
  if (manualRefreshing.value) return
  manualRefreshing.value = true
  try { await fetchListeners() } finally { manualRefreshing.value = false }
}

function sleepSeconds(row: ForwardItem): number {
  // 生效值优先（监听器 > 画像 > 全局）；旧服务端没有 Effective* 时回退画像值
  const eff = Number(row.EffectiveSleepSeconds)
  if (Number.isFinite(eff) && eff > 0) return eff
  const sec = Number(row.SleepSeconds)
  return Number.isFinite(sec) && sec > 0 ? sec : 5
}

function sleepJitter(row: ForwardItem): number {
  const eff = Number(row.EffectiveJitterPercent)
  return Number.isFinite(eff) && eff >= 0 ? eff : 0
}

function sleepLabel(row: ForwardItem): string {
  return `${sleepSeconds(row)}s +${sleepJitter(row)}%`
}

function sleepTitle(row: ForwardItem): string {
  const sec = sleepSeconds(row)
  const jit = sleepJitter(row)
  const profile = row.Profile || 'default'
  const source = Number(row.BeaconSleepSeconds) > 0 ? (isZh.value ? '监听器配置' : 'listener') : (isZh.value ? `C2 画像 ${profile}` : `C2 profile ${profile}`)
  return isZh.value
    ? `beacon sleep ${sec}s、抖动 ${jit}%（来源：${source}；优先级 监听器 > 画像 > 全局）`
    : `beacon sleep ${sec}s, jitter ${jit}% (source: ${source}; listener > profile > global)`
}

const ctxMenu = ref({
  visible: false,
  x: 0,
  y: 0,
  row: null as ForwardItem | null,
  showColor: false,
})

const colorPalette = [
  { value: '#ffffff', zh: '默认白色', en: 'White' },
  { value: '#ef5350', zh: '红色', en: 'Red' },
  { value: '#ff9800', zh: '橙色', en: 'Orange' },
  { value: '#ffee58', zh: '黄色', en: 'Yellow' },
  { value: '#66bb6a', zh: '绿色', en: 'Green' },
  { value: '#26c6da', zh: '青色', en: 'Cyan' },
  { value: '#42a5f5', zh: '蓝色', en: 'Blue' },
  { value: '#ab47bc', zh: '紫色', en: 'Purple' },
]

const filteredListeners = computed(() => {
  const q = searchKey.value.toLowerCase().trim()
  if (!q) return forwardList.value
  return forwardList.value.filter((l) =>
    [String(l.Id ?? ''), l.Type, l.ListenAddress, l.ConnectAddress, l.Note || '']
      .join(' ')
      .toLowerCase()
      .includes(q),
  )
})

function getRowStyle({ row }: { row: ForwardItem }) {
  if (row.color && row.color !== '#ffffff') {
    return {
      '--el-table-tr-bg-color': `${row.color}30`,
      backgroundColor: `${row.color}30`,
      borderLeft: `3px solid ${row.color}`,
    }
  }
  return {}
}

async function fetchListeners() {
  loading.value = true
  try {
    const res = await ListenerAPI.getListenerList()
    if (res.status === 200 && res.data?.status === 200) {
      const list = (res.data.data || []).filter((item: any) => {
        const t = (item.Type || '').toLowerCase()
        return t.endsWith('_forward')
      })
      forwardList.value = list.map((item: any) => ({
        Id: item.Id ?? item.ID ?? item.id,
        Type: item.Type || 'tcp_forward',
        ListenAddress: item.ListenAddress || '',
        ConnectAddress: item.ConnectAddress || '',
        Status: item.Status,
        HeartbeatTimeout: item.HeartbeatTimeout,
        HeartbeatInterval: item.HeartbeatInterval,
        Note: item.Note,
        VKey: item.VKey,
        EncryptionSalt: item.EncryptionSalt,
        Profile: item.Profile || '',
        SleepSeconds: Number(item.SleepSeconds) || 0,
        BeaconSleepSeconds: Number(item.BeaconSleepSeconds ?? -1),
        BeaconJitterPercent: Number(item.BeaconJitterPercent ?? -1),
        EffectiveSleepSeconds: Number(item.EffectiveSleepSeconds) || 0,
        EffectiveJitterPercent: Number(item.EffectiveJitterPercent) || 0,
        color: item.color || '',
      }))
      emit('count-change', forwardList.value.length)
    } else {
      forwardList.value = []
      emit('count-change', 0)
    }
  } catch (e: any) {
    forwardList.value = []
    emit('count-change', 0)
    ElMessage.error(e?.message || (isZh.value ? '获取正向监听器列表失败' : 'Failed to fetch forward listeners'))
  } finally {
    loading.value = false
  }
}

function onRowClick(row: ForwardItem) {
  selectedRow.value = row
}

function onRowDblClick(row: ForwardItem) {
  selectedRow.value = row
  openEditDialog(row)
}

function onRowContextMenu(row: ForwardItem, col: any, e: MouseEvent) {
  e.preventDefault()
  selectedRow.value = row
  ctxMenu.value = {
    visible: true,
    x: e.clientX,
    y: e.clientY,
    row,
    showColor: false,
  }
}

function closeContextMenu() {
  ctxMenu.value.visible = false
  ctxMenu.value.showColor = false
}

function setColor(color: string) {
  if (!ctxMenu.value.row) return
  ctxMenu.value.row.color = color
  closeContextMenu()
}

function openAddDialog() {
  editingListener.value = null
  showEditDialog.value = true
}

function openEditDialog(item?: ForwardItem | null) {
  const target = item || selectedRow.value
  if (!target) return
  editingListener.value = { ...target }
  showEditDialog.value = true
  closeContextMenu()
}

function methodOfType(t: string): string {
  const m = (t || '').toLowerCase()
  if (m === 'kcp_forward') return 'KCP/UDP'
  if (m === 'ws_forward') return 'WebSocket'
  if (m === 'http_forward') return 'HTTP'
  if (m === 'https_forward') return 'HTTPS'
  return 'TCP'
}

function parseAddr(addr: string): { host: string; port: string } {
  const s = (addr || '').trim()
  const idx = s.lastIndexOf(':')
  if (idx > 0) {
    return { host: s.slice(0, idx), port: s.slice(idx + 1) }
  }
  return { host: s, port: '' }
}

// 0.0.0.0/:: 是客户端 bind 地址，不是服务端可拨目标（拨过去会连到本机）
function isUnspecifiedForwardHost(host: string): boolean {
  const h = (host || '').trim().toLowerCase().replace(/^\[|\]$/g, '')
  return h === '' || h === '0.0.0.0' || h === '::'
}

async function handleConnect(item: ForwardItem | null) {
  if (!item) return
  closeContextMenu()
  const addr = parseAddr(item.ConnectAddress || '')
  if (!addr.host || !addr.port) {
    ElMessage.warning(isZh.value ? '请先填写回连地址（正向客户端可达的 IP:端口）' : 'Set the connect address (client IP:port) first')
    return
  }
  if (isUnspecifiedForwardHost(addr.host)) {
    ElMessage.warning(isZh.value ? '回连地址不能用 0.0.0.0/::，那是客户端绑定地址' : 'Connect address cannot be 0.0.0.0/:: (client bind address)')
    return
  }
  const loadingMsg = ElMessage.info({
    message: isZh.value ? `正在发起正向连接: ${addr.host}:${addr.port}...` : `Connecting to ${addr.host}:${addr.port}...`,
    duration: 0,
  })
  try {
    const res = await ClientAPI.connect_forward_client({
      isEbpfClient: false,
      connectionMethod: methodOfType(item.Type),
      ipAddress: addr.host,
      port: addr.port,
      password: '',
      vkey: item.VKey || '',
      encryptionSalt: item.EncryptionSalt || '',
    })
    loadingMsg.close()
    if (res?.data?.status === 200 || res?.status === 200) {
      ElMessage.success(isZh.value ? `已连接正向客户端 ${addr.host}:${addr.port}` : `Connected to forward client ${addr.host}:${addr.port}`)
    } else {
      ElMessage.error(res?.data?.message || res?.data?.data || (isZh.value ? '连接失败' : 'Connect failed'))
    }
  } catch (err: any) {
    loadingMsg.close()
    ElMessage.error(err?.message || (isZh.value ? '连接失败' : 'Connect failed'))
  }
}

async function handleStart(item: ForwardItem | null) {
  if (!item) return
  closeContextMenu()
  const target = parseAddr(item.ConnectAddress || '')
  if (!target.host || !target.port || isUnspecifiedForwardHost(target.host)) {
    ElMessage.warning(isZh.value ? '请先填写可拨号的回连地址（IP:端口）再开启自动拨号' : 'Set a dialable connect address (IP:port) before starting auto-dial')
    return
  }
  try {
    const res = await ListenerAPI.openListener({ listenAddress: item.ListenAddress })
    if (res.data?.status === 200 || res.status === 200) {
      ElMessage.success(isZh.value ? `正向监听器 ${item.ListenAddress} 已开启` : `Forward listener ${item.ListenAddress} started`)
      await fetchListeners()
    } else {
      ElMessage.error(res.data?.data || (isZh.value ? '开启失败' : 'Failed to start'))
    }
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '开启失败' : 'Failed to start'))
  }
}

async function handleStop(item: ForwardItem | null) {
  if (!item) return
  closeContextMenu()
  try {
    const res = await ListenerAPI.closeListener({ listenAddress: item.ListenAddress })
    if (res.data?.status === 200 || res.status === 200) {
      ElMessage.success(isZh.value ? `正向监听器 ${item.ListenAddress} 已关闭` : `Forward listener ${item.ListenAddress} stopped`)
      await fetchListeners()
    } else {
      ElMessage.error(res.data?.data || (isZh.value ? '关闭失败' : 'Failed to stop'))
    }
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '关闭失败' : 'Failed to stop'))
  }
}

async function handleRemove(item: ForwardItem | null) {
  if (!item) return
  closeContextMenu()
  ElMessageBox.confirm(
    isZh.value
      ? `确定要删除正向监听器 "${item.ListenAddress}" 吗？`
      : `Are you sure you want to remove forward listener "${item.ListenAddress}"?`,
    isZh.value ? '确认删除' : 'Confirm Delete',
    {
      confirmButtonText: isZh.value ? '删除' : 'Remove',
      cancelButtonText: isZh.value ? '取消' : 'Cancel',
      type: 'warning',
    },
  )
    .then(async () => {
      try {
        const res = await ListenerAPI.deleteListener({ listenAddress: item.ListenAddress || '' })
        if (res.data?.status === 200) {
          ElMessage.success(isZh.value ? '正向监听器已删除' : 'Forward listener removed')
        } else {
          ElMessage.error(res.data?.data || (isZh.value ? '删除失败' : 'Delete failed'))
        }
        selectedRow.value = null
        await fetchListeners()
      } catch (e: any) {
        ElMessage.error(e?.message || (isZh.value ? '删除失败' : 'Delete failed'))
      }
    })
    .catch(() => {})
}

async function onListenerSaved(formData: any) {
  savingListener.value = true
  try {
    const isEdit = !!editingListener.value?.Id || !!editingListener.value?.id || !!editingListener.value?.ID
    const targetId = editingListener.value?.Id ?? editingListener.value?.id ?? editingListener.value?.ID

    let res: any
    if (isEdit && targetId) {
      res = await ListenerAPI.updateListener({
        id: Number(targetId),
        type: formData.type,
        listenAddress: formData.listenAddress,
        connectAddress: formData.connectAddress,
        note: formData.note,
        vkey: formData.vkey,
        encryptionSalt: formData.encryptionSalt,
        profile: formData.profile || "default",
        beaconSleepSeconds: formData.beaconSleepSeconds,
        beaconJitterPercent: formData.beaconJitterPercent,
      })
    } else {
      res = await ListenerAPI.addListener({
        type: formData.type,
        listenAddress: formData.listenAddress,
        connectAddress: formData.connectAddress,
        note: formData.note,
        vkey: formData.vkey,
        encryptionSalt: formData.encryptionSalt,
        profile: formData.profile || "default",
        beaconSleepSeconds: formData.beaconSleepSeconds,
        beaconJitterPercent: formData.beaconJitterPercent,
      })
    }

    if (res.data?.status === 200 || res.status === 200) {
      ElMessage.success(isZh.value ? `正向监听器 ${formData.listenAddress} 已保存` : `Forward listener ${formData.listenAddress} saved successfully`)
      showEditDialog.value = false
      editingListener.value = null
      await fetchListeners()
    } else {
      ElMessage.error(res.data?.data || res.data?.message || (isZh.value ? '保存失败' : 'Failed to save'))
    }
  } catch (err: any) {
    ElMessage.error(err?.response?.data?.data || err?.response?.data?.message || err?.message || (isZh.value ? '保存失败' : 'Failed to save'))
  } finally {
    savingListener.value = false
  }
}

function handleHelp() {
  ElMessage.info(
    isZh.value
      ? '正向监听器使用说明：先在目标机部署正向载荷（ServerGen listen 模式），目标机监听端口等待 C2 主动连接；然后在列表中点击「连接」或使用正向连接对话框发起 forward-connect。'
      : 'Forward Listener Usage: deploy the forward payload on the target (ServerGen listen mode); the target listens and waits for the C2 to connect in; then click "Connect" in the list or use the forward-connect dialog.',
  )
}

defineExpose({
  fetchListeners,
  openAddDialog,
  forwardList,
  loading,
})

onMounted(() => {
  fetchListeners()
  window.addEventListener('mousedown', closeContextMenu)
})

onBeforeUnmount(() => {
  window.removeEventListener('mousedown', closeContextMenu)
})
</script>

<style scoped>
.cs-fl-panel {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-bg);
  overflow: hidden;
}

/* 工具条：白底 + 底部分隔线由 vshell-ep.css 的 .cs-fl-toolbar 接管 */
.cs-fl-toolbar {
  min-height: 48px;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  padding: 8px 12px;
  flex-shrink: 0;
}

.cs-title-group {
  display: flex;
  align-items: center;
  gap: 8px;
}

.cs-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
  white-space: nowrap;
}

.cs-count {
  font-size: 12px;
  color: var(--cs-text-dim);
  white-space: nowrap;
}

.cs-spacer {
  flex: 1;
}

.cs-search {
  width: 240px;
}

.cs-search-icon {
  color: var(--cs-text-dim);
}

.cs-toolbar-divider {
  width: 1px;
  height: 20px;
  flex-shrink: 0;
  background: var(--cs-card-border);
}

.cs-status-hint {
  font-size: 12px;
  color: var(--cs-text-dim);
  white-space: nowrap;
}

/* 表格铺满内容区（单元格底色/字号由全局 .cs-shell .el-table 统一） */
.cs-table-container {
  flex: 1;
  overflow: hidden;
  position: relative;
  background: var(--cs-panel);
}

/* 状态：在线实心绿 tag，离线灰 tag（20px / 4px 圆角 / 12px） */
.vs-status-tag {
  display: inline-flex;
  align-items: center;
  height: 20px;
  padding: 0 7px;
  border-radius: 4px;
  font-size: 12px;
  line-height: 1;
  white-space: nowrap;
}

.vs-status-tag.is-online {
  background: var(--cs-online);
  color: #fff;
}

.vs-status-tag.is-offline {
  background: color-mix(in srgb, var(--cs-offline) 22%, transparent);
  color: var(--cs-text-dim);
}

/* 右键菜单：vshell 弹层观感（8px 圆角 + 柔和投影 + 32px 行高） */
.cs-fl-ctx-menu {
  position: fixed;
  padding: 4px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  box-shadow: 0 3px 6px -4px rgba(0, 0, 0, 0.12), 0 6px 16px 0 rgba(0, 0, 0, 0.08);
  z-index: 9999;
  min-width: 180px;
}

.cs-fl-ctx-menu .ctx-item {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 32px;
  padding: 0 12px;
  font-size: 14px;
  color: var(--cs-text);
  border-radius: 6px;
  cursor: pointer;
}

.cs-fl-ctx-menu .ctx-item:hover {
  background: var(--cs-hover);
  color: var(--cs-active);
}

.cs-fl-ctx-menu .ctx-item.disabled {
  opacity: 0.45;
}

.cs-fl-ctx-menu .ctx-item.ctx-danger,
.cs-fl-ctx-menu .ctx-item.ctx-danger:hover {
  color: var(--cs-error);
}

.cs-fl-ctx-menu .ctx-item.ctx-danger:hover {
  background: color-mix(in srgb, var(--cs-error) 12%, transparent);
}

.cs-fl-ctx-menu .ctx-divider {
  height: 1px;
  margin: 4px 0;
  background: var(--cs-card-border);
}

.cs-fl-ctx-menu .ctx-arrow {
  font-size: 12px;
  color: var(--cs-text-dim);
  margin-left: 14px;
}

.cs-fl-ctx-menu .ctx-color-palette {
  position: absolute;
  top: -4px;
  left: 100%;
  padding: 6px;
  display: flex;
  gap: 4px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  box-shadow: 0 3px 6px -4px rgba(0, 0, 0, 0.12), 0 6px 16px 0 rgba(0, 0, 0, 0.08);
  z-index: 10000;
}

.color-box {
  width: 20px;
  height: 20px;
  border: 1px solid var(--cs-border-strong);
  border-radius: 4px;
  cursor: pointer;
}

.color-box:hover {
  border-color: var(--cs-active);
}

/* 回连地址为空：生成载荷自动建的记录要操作员补全，给一个显眼提示 */
.fl-target-missing {
  font-style: italic;
  color: #e6a23c;
}
</style>
