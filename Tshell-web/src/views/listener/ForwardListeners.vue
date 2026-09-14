<template>
  <div class="cs-fl-panel cs-font-ui" @click="closeContextMenu">
    <!-- CS Toolbar (Standalone mode only) -->
    <div v-if="!embedded" class="cs-fl-toolbar">
      <div class="cs-title-group">
        <span class="cs-title">{{ isZh ? '正向监听器' : 'Forward Listeners' }}</span>
        <span class="cs-count">({{ forwardList.length }} {{ isZh ? '个' : 'total' }})</span>
      </div>

      <div class="cs-spacer" />

      <el-input
        v-model="localSearchKey"
        :placeholder="isZh ? '过滤类型、地址、备注...' : 'Filter type, address, note...'"
        size="small"
        clearable
        class="cs-search"
      />

      <button class="cs-btn" :disabled="loading" @click="fetchListeners">
        {{ isZh ? '刷新' : 'Refresh' }}
      </button>
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
        class="cs-session-table"
        @row-click="onRowClick"
        @row-dblclick="onRowDblClick"
        @row-contextmenu="onRowContextMenu"
      >
        <el-table-column prop="name" :label="isZh ? '名称' : 'name'" min-width="120" sortable show-overflow-tooltip />
        <el-table-column prop="payload" :label="isZh ? '载荷' : 'payload'" min-width="160" sortable show-overflow-tooltip />
        <el-table-column prop="host" :label="isZh ? '主机' : 'host'" min-width="130" sortable show-overflow-tooltip />
        <el-table-column prop="port" :label="isZh ? '端口' : 'port'" width="85" sortable align="left" />
        <el-table-column prop="bindto" :label="isZh ? '回连地址' : 'bindto'" min-width="170" sortable show-overflow-tooltip />
        <el-table-column :label="isZh ? '状态' : 'status'" width="115" sortable align="center">
          <template #default="{ row }">
            <span class="cs-status-indicator" :class="row.Status === 1 ? 'online' : 'offline'">
              <span class="status-dot" />
              {{ row.Status === 1 ? (isZh ? '监听中' : 'Listening') : (isZh ? '已停止' : 'Stopped') }}
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="note" :label="isZh ? '备注' : 'note'" min-width="130" sortable show-overflow-tooltip />
        <el-table-column :label="isZh ? 'VKey' : 'vkey'" min-width="115" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="cs-font-mono">{{ maskSecret(row.VKey) }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="isZh ? '加密盐' : 'encryption_salt'" min-width="115" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="cs-font-mono">{{ maskSecret(row.EncryptionSalt) }}</span>
          </template>
        </el-table-column>

        <template #empty>
          <div class="cs-empty-watermark">
            <div class="empty-title">{{ isZh ? '暂无正向监听器' : 'No Forward Listeners Configured' }}</div>
          </div>
        </template>
      </el-table>
    </div>

    <!-- Bottom Action Buttons (CS Native Button Row) -->
    <div class="cs-bottom-bar">
      <button class="cs-btn cs-btn-primary" @click="openAddDialog">{{ isZh ? '添加' : 'Add' }}</button>
      <button class="cs-btn" :disabled="!selectedRow" @click="openEditDialog()">{{ isZh ? '编辑' : 'Edit' }}</button>
      <button class="cs-btn" :disabled="!selectedRow" @click="() => handleConnect(selectedRow)">{{ isZh ? '连接' : 'Connect' }}</button>
      <button class="cs-btn" :disabled="!selectedRow || selectedRow.Status === 1" @click="() => handleStart(selectedRow)">{{ isZh ? '开启' : 'Start' }}</button>
      <button class="cs-btn" :disabled="!selectedRow || selectedRow.Status !== 1" @click="() => handleStop(selectedRow)">{{ isZh ? '关闭' : 'Stop' }}</button>
      <button class="cs-btn cs-btn-danger" :disabled="!selectedRow" @click="() => handleRemove(selectedRow)">{{ isZh ? '删除' : 'Remove' }}</button>
      <button class="cs-btn" @click="handleHelp">{{ isZh ? '帮助' : 'Help' }}</button>

      <div class="cs-spacer" v-if="embedded" />
      <span class="cs-status-hint" v-if="embedded">
        {{ isZh ? `共 ${filteredListeners.length} 个正向监听器` : `${filteredListeners.length} forward listener(s)` }}
      </span>
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
          <span class="ctx-text">{{ isZh ? '连接' : 'Connect' }}</span>
        </div>
        <div class="ctx-divider" />
        <div class="ctx-item" :class="{ disabled: ctxMenu.row?.Status === 1 }" @click="handleStart(ctxMenu.row)">
          <span class="ctx-text">{{ isZh ? '开启监听' : 'Start Listener' }}</span>
        </div>
        <div class="ctx-item" :class="{ disabled: ctxMenu.row?.Status !== 1 }" @click="handleStop(ctxMenu.row)">
          <span class="ctx-text">{{ isZh ? '关闭监听' : 'Stop Listener' }}</span>
        </div>
        <div class="ctx-item" @click="openEditDialog(ctxMenu.row)">
          <span class="ctx-text">{{ isZh ? '编辑...' : 'Edit...' }}</span>
        </div>
        <div class="ctx-divider" />
        <div
          class="ctx-item has-sub"
          @mouseenter="ctxMenu.showColor = true"
          @mouseleave="ctxMenu.showColor = false"
        >
          <span class="ctx-text">{{ isZh ? '颜色' : 'Color' }}</span>
          <span class="ctx-arrow">▶</span>

          <div v-show="ctxMenu.showColor" class="ctx-color-palette">
            <div
              v-for="c in colorPalette"
              :key="c.value"
              class="color-box"
              :style="{ backgroundColor: c.value }"
              :title="isZh ? c.zh : c.en"
              @click.stop="setColor(c.value)"
            />
          </div>
        </div>
        <div class="ctx-divider" />
        <div class="ctx-item ctx-danger" @click="handleRemove(ctxMenu.row)">
          <span class="ctx-text">{{ isZh ? '删除' : 'Remove' }}</span>
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
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import EditListenerDialog from '@/layout/Dialogs/EditListenerDialog.vue'
import ListenerAPI from '@/api/listener'
import ClientAPI from '@/api/clients'
import { useI18nStore } from '@/stores/i18n'

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
  color?: string
  name?: string
  payload?: string
  host?: string
  port?: string
  bindto?: string
  note?: string
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
    [l.Type, l.ListenAddress, l.ConnectAddress, l.Note || '']
      .join(' ')
      .toLowerCase()
      .includes(q),
  )
})

function parseHostPort(addr: string): { host: string; port: string } {
  if (!addr) return { host: '', port: '' }
  let a = addr.trim()
  if (a.startsWith(':')) a = '0.0.0.0' + a
  if (!a.includes(':')) return { host: a, port: '' }
  const idx = a.lastIndexOf(':')
  return { host: a.slice(0, idx), port: a.slice(idx + 1) }
}

function maskSecret(secret?: string): string {
  if (!secret) return isZh.value ? '默认' : 'DEFAULT'
  if (secret.length <= 4) return '****'
  return `${secret.slice(0, 4)}****${secret.slice(-2)}`
}

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
      forwardList.value = list.map((item: any) => {
        const addr = parseHostPort(item.ListenAddress || '')
        return {
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
          name: (item.Type || 'tcp_forward').toUpperCase(),
          payload: `beacon_${(item.Type || 'tcp_forward').toLowerCase()}`,
          host: addr.host,
          port: addr.port,
          bindto: item.ConnectAddress || '',
          note: item.Note || '',
          color: item.color || '',
        }
      })
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

async function handleConnect(item: ForwardItem | null) {
  if (!item) return
  closeContextMenu()
  const addr = parseAddr(item.ConnectAddress || item.ListenAddress || '')
  if (!addr.host || !addr.port) {
    ElMessage.warning(isZh.value ? '请先配置连接地址（IP:端口，填写在“回连地址”字段）' : 'Set the connect address (IP:port) first')
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
        retries: formData.retries,
        interval: formData.interval,
        vkey: formData.vkey,
        encryptionSalt: formData.encryptionSalt,
      })
    } else {
      res = await ListenerAPI.addListener({
        type: formData.type,
        listenAddress: formData.listenAddress,
        connectAddress: formData.connectAddress,
        note: formData.note,
        retries: formData.retries,
        interval: formData.interval,
        vkey: formData.vkey,
        encryptionSalt: formData.encryptionSalt,
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

.cs-fl-toolbar {
  height: 36px;
  background: var(--cs-titlebar-bg);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 12px;
  flex-shrink: 0;
}

.cs-title-group {
  display: flex;
  align-items: center;
  gap: 8px;
}

.cs-title {
  font-size: 17px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.cs-count {
  font-size: 16px;
  color: var(--cs-text-dim);
}

.cs-spacer {
  flex: 1;
}

.cs-search {
  width: 260px;
}

.cs-table-container {
  flex: 1;
  overflow: hidden;
  position: relative;
}

.cs-bottom-bar {
  height: 38px;
  background: var(--cs-panel);
  border-top: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  justify-content: flex-start;
  gap: 10px;
  padding: 0 12px;
  flex-shrink: 0;
}

.cs-status-hint {
  font-size: 16px;
  color: var(--cs-text-dim);
  font-family: var(--cs-font-mono);
}

.cs-status-indicator {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 16px;
}

.cs-status-indicator .status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  display: inline-block;
}

.cs-status-indicator.online .status-dot {
  background: var(--cs-online);
  box-shadow: 0 0 4px var(--cs-online);
}

.cs-status-indicator.offline .status-dot {
  background: var(--cs-offline);
}

.cs-fl-ctx-menu {
  position: fixed;
  background: var(--cs-panel);
  border: 1px solid var(--cs-border);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.45);
  border-radius: 2px;
  padding: 4px 0;
  z-index: 9999;
  min-width: 180px;
}

.cs-fl-ctx-menu .ctx-item {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 14px;
  font-size: 16.5px;
  color: var(--cs-text);
  cursor: pointer;
}

.cs-fl-ctx-menu .ctx-item:hover {
  background: var(--cs-hover);
  color: var(--cs-text-bright);
}

.cs-fl-ctx-menu .ctx-arrow {
  font-size: 12px;
  color: var(--cs-text-dim);
  margin-left: 14px;
}

.cs-fl-ctx-menu .ctx-color-palette {
  position: absolute;
  top: -2px;
  left: 100%;
  background: var(--cs-panel);
  border: 1px solid var(--cs-border);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.45);
  border-radius: 2px;
  padding: 6px;
  display: flex;
  gap: 4px;
  z-index: 10000;
}

.color-box {
  width: 20px;
  height: 20px;
  border: 1px solid #444;
  border-radius: 2px;
  cursor: pointer;
}

.color-box:hover {
  border-color: #ffffff;
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
  font-size: 13px;
  color: var(--cs-text-dim);
}
</style>
