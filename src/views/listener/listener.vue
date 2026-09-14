<template>
  <div class="cs-listeners-panel cs-font-ui" @click="closeContextMenu">
    <!-- CS Unified Toolbar -->
    <div class="cs-listener-toolbar">
      <div class="cs-header-left">
        <div class="cs-title-group">
          <el-icon class="cs-title-icon"><Headset /></el-icon>
          <span class="cs-title">{{ isZh ? '监听器管理' : 'Listeners' }}</span>
        </div>

        <!-- 扁平风格正反向监听器分段切换器 -->
        <div class="cs-segmented-toggle">
          <button
            class="cs-segment-btn"
            :class="{ active: activeView === 'normal' }"
            @click="switchView('normal')"
          >
            <span class="seg-icon">⬇</span>
            <span>{{ isZh ? '反向监听器' : 'Reverse Listeners' }}</span>
            <span class="seg-badge">{{ listenersList.length }}</span>
          </button>
          <button
            class="cs-segment-btn"
            :class="{ active: activeView === 'forward' }"
            @click="switchView('forward')"
          >
            <span class="seg-icon">⬆</span>
            <span>{{ isZh ? '正向监听器' : 'Forward Listeners' }}</span>
            <span class="seg-badge">{{ forwardCount }}</span>
          </button>
        </div>
      </div>

      <div class="cs-spacer" />

      <!-- 右侧统一搜索与刷新 -->
      <div class="cs-header-right">
        <el-input
          v-model="searchKey"
          :placeholder="activeView === 'normal' ? (isZh ? '过滤反向监听器 (名称/载荷/地址/备注)...' : 'Filter reverse listeners...') : (isZh ? '过滤正向监听器 (名称/载荷/地址/备注)...' : 'Filter forward listeners...')"
          size="small"
          clearable
          class="cs-search"
        />

        <button class="cs-btn" :disabled="currentLoading" @click="handleRefresh">
          <el-icon :class="{ 'is-loading': currentLoading }"><Refresh /></el-icon>
          <span>{{ isZh ? '刷新' : 'Refresh' }}</span>
        </button>
      </div>
    </div>

    <!-- 主体区域 -->
    <div class="cs-listener-body">
      <!-- 1. 反向监听器面板 -->
      <div v-show="activeView === 'normal'" class="cs-view-pane">
        <!-- Listeners Table (CS Style) -->
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
            <el-table-column prop="name" :label="isZh ? '名称' : 'name'" min-width="110" sortable show-overflow-tooltip />
            <el-table-column prop="payload" :label="isZh ? '载荷' : 'payload'" min-width="160" sortable show-overflow-tooltip />
            <el-table-column prop="host" :label="isZh ? '主机' : 'host'" min-width="140" sortable show-overflow-tooltip />
            <el-table-column prop="port" :label="isZh ? '端口' : 'port'" width="85" sortable align="left" />
            <el-table-column prop="bindto" :label="isZh ? '回连地址' : 'bindto'" min-width="160" sortable show-overflow-tooltip />
            <el-table-column :label="isZh ? '状态' : 'status'" width="115" sortable align="center">
              <template #default="{ row }">
                <span class="cs-status-indicator" :class="row.Status === 1 ? 'online' : 'offline'">
                  <span class="status-dot" />
                  {{ row.Status === 1 ? (isZh ? '监听中' : 'Listening') : (isZh ? '已停止' : 'Stopped') }}
                </span>
              </template>
            </el-table-column>
            <el-table-column prop="note" :label="isZh ? '备注' : 'note'" min-width="140" sortable show-overflow-tooltip />
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
                <div class="empty-title">{{ isZh ? '暂无反向监听器' : 'No Listeners Configured' }}</div>
              </div>
            </template>
          </el-table>
        </div>

        <!-- Bottom Action Buttons (CS Native Button Row) -->
        <div class="cs-bottom-bar">
          <button class="cs-btn cs-btn-primary" @click="openAddDialog">{{ isZh ? '添加' : 'Add' }}</button>
          <button class="cs-btn" :disabled="!selectedRow" @click="openEditDialog()">{{ isZh ? '编辑' : 'Edit' }}</button>
          <button class="cs-btn" :disabled="!selectedRow || selectedRow.Status === 1" @click="() => handleStart(selectedRow)">{{ isZh ? '开启' : 'Start' }}</button>
          <button class="cs-btn" :disabled="!selectedRow || selectedRow.Status !== 1" @click="() => handleStop(selectedRow)">{{ isZh ? '关闭' : 'Stop' }}</button>
          <button class="cs-btn" :disabled="!selectedRow" @click="() => handleRestart(selectedRow)">{{ isZh ? '重启' : 'Restart' }}</button>
          <button class="cs-btn cs-btn-danger" :disabled="!selectedRow" @click="() => handleRemove(selectedRow)">{{ isZh ? '删除' : 'Remove' }}</button>
          <button class="cs-btn" @click="handleHelp">{{ isZh ? '帮助' : 'Help' }}</button>

          <div class="cs-spacer" />
          <span class="cs-status-hint">
            {{ isZh ? `共 ${filteredListeners.length} 个反向监听器` : `${filteredListeners.length} reverse listener(s)` }}
          </span>
        </div>
      </div>

      <!-- 2. 正向监听器面板 (嵌入式) -->
      <div v-show="activeView === 'forward'" class="cs-view-pane">
        <ForwardListeners
          ref="forwardRef"
          :embedded="true"
          :external-search-key="searchKey"
          @count-change="onForwardCountChange"
        />
      </div>
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
        <div class="ctx-item" :class="{ disabled: ctxMenu.row?.Status === 1 }" @click="handleStart(ctxMenu.row)">
          <span class="ctx-text">{{ isZh ? '开启监听' : 'Start Listener' }}</span>
        </div>
        <div class="ctx-item" :class="{ disabled: ctxMenu.row?.Status !== 1 }" @click="handleStop(ctxMenu.row)">
          <span class="ctx-text">{{ isZh ? '关闭监听' : 'Stop Listener' }}</span>
        </div>
        <div class="ctx-item" @click="handleRestart(ctxMenu.row)">
          <span class="ctx-text">{{ isZh ? '重启监听' : 'Restart Listener' }}</span>
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

    <!-- New / Edit Listener Dialog -->
    <EditListenerDialog
      v-model="showEditDialog"
      :listener-data="editingListener"
      scope="reverse"
      :saving="savingListener"
      @save="onListenerSaved"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Headset, Refresh } from '@element-plus/icons-vue'
import EditListenerDialog from '@/layout/Dialogs/EditListenerDialog.vue'
import ForwardListeners from './ForwardListeners.vue'
import ListenerAPI from '@/api/listener'
import { useI18nStore } from '@/stores/i18n'

interface ListenerItem {
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

const forwardRef = ref<InstanceType<typeof ForwardListeners> | null>(null)
const listenersList = ref<ListenerItem[]>([])
const forwardCount = ref(0)
const activeView = ref<'normal' | 'forward'>('normal')
const loading = ref(false)
const savingListener = ref(false)
const searchKey = ref('')
const selectedRow = ref<ListenerItem | null>(null)
const editingListener = ref<ListenerItem | null>(null)
const showEditDialog = ref(false)

const currentLoading = computed(() => (activeView.value === 'normal' ? loading.value : (forwardRef.value?.loading ?? false)))

const ctxMenu = ref({
  visible: false,
  x: 0,
  y: 0,
  row: null as ListenerItem | null,
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
  if (!q) return listenersList.value
  return listenersList.value.filter((l) =>
    [l.Type, l.ListenAddress, l.ConnectAddress, l.Note || '']
      .join(' ')
      .toLowerCase()
      .includes(q),
  )
})

function switchView(view: 'normal' | 'forward') {
  activeView.value = view
  if (view === 'forward') {
    forwardRef.value?.fetchListeners()
  }
}

function handleRefresh() {
  fetchListeners()
  forwardRef.value?.fetchListeners()
}

function onForwardCountChange(cnt: number) {
  forwardCount.value = cnt
}

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

function getRowStyle({ row }: { row: ListenerItem }) {
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
      const allList = res.data.data || []
      const reverseList = allList.filter((item: any) => {
        const t = (item.Type || '').toLowerCase()
        return !t.endsWith('_forward')
      })
      const fList = allList.filter((item: any) => {
        const t = (item.Type || '').toLowerCase()
        return t.endsWith('_forward')
      })
      forwardCount.value = fList.length

      listenersList.value = reverseList.map((item: any) => {
        const addr = parseHostPort(item.ListenAddress || '')
        return {
          Id: item.Id ?? item.ID ?? item.id,
          Type: item.Type || 'tcp',
          ListenAddress: item.ListenAddress || '',
          ConnectAddress: item.ConnectAddress || '',
          Status: item.Status,
          HeartbeatTimeout: item.HeartbeatTimeout,
          HeartbeatInterval: item.HeartbeatInterval,
          Note: item.Note,
          VKey: item.VKey,
          EncryptionSalt: item.EncryptionSalt,
          name: (item.Type || 'tcp').toUpperCase(),
          payload: `beacon_${(item.Type || 'tcp').toLowerCase()}`,
          host: addr.host,
          port: addr.port,
          bindto: item.ConnectAddress || '',
          note: item.Note || '',
          color: item.color || '',
        }
      })
    } else {
      listenersList.value = []
    }
  } catch (e: any) {
    listenersList.value = []
    ElMessage.error(e?.message || (isZh.value ? '获取监听器列表失败' : 'Failed to fetch listeners'))
  } finally {
    loading.value = false
  }
}

function onRowClick(row: ListenerItem) {
  selectedRow.value = row
}

function onRowDblClick(row: ListenerItem) {
  selectedRow.value = row
  openEditDialog(row)
}

function onRowContextMenu(row: ListenerItem, col: any, e: MouseEvent) {
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
  if (ctxMenu.value.row) {
    ctxMenu.value.row.color = color
  }
  closeContextMenu()
}

function openAddDialog() {
  editingListener.value = null
  showEditDialog.value = true
}

function openEditDialog(item?: ListenerItem | null) {
  const target = item || selectedRow.value
  if (!target) return
  editingListener.value = { ...target }
  showEditDialog.value = true
  closeContextMenu()
}

async function handleStart(item: ListenerItem | null) {
  if (!item) return
  closeContextMenu()
  try {
    const res = await ListenerAPI.openListener({ listenAddress: item.ListenAddress })
    if (res.data?.status === 200 || res.status === 200) {
      ElMessage.success(isZh.value ? `监听器 ${item.ListenAddress} 已开启` : `Listener ${item.ListenAddress} started`)
      await fetchListeners()
    } else {
      ElMessage.error(res.data?.data || (isZh.value ? '开启失败' : 'Failed to start'))
    }
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.data || (isZh.value ? '开启失败' : 'Failed to start'))
  }
}

async function handleStop(item: ListenerItem | null) {
  if (!item) return
  closeContextMenu()
  try {
    const res = await ListenerAPI.closeListener({ listenAddress: item.ListenAddress })
    if (res.data?.status === 200 || res.status === 200) {
      ElMessage.success(isZh.value ? `监听器 ${item.ListenAddress} 已关闭` : `Listener ${item.ListenAddress} stopped`)
      await fetchListeners()
    } else {
      ElMessage.error(res.data?.data || (isZh.value ? '关闭失败' : 'Failed to stop'))
    }
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.data || (isZh.value ? '关闭失败' : 'Failed to stop'))
  }
}

async function handleRestart(item: ListenerItem | null) {
  if (!item) return
  closeContextMenu()
  try {
    const res = await ListenerAPI.restartListener({ listenAddress: item.ListenAddress })
    if (res.data?.status === 200 || res.status === 200) {
      ElMessage.success(isZh.value ? `监听器 ${item.ListenAddress} 已重启` : `Listener ${item.ListenAddress} restarted`)
      await fetchListeners()
    } else {
      ElMessage.error(res.data?.data || (isZh.value ? '重启失败' : 'Failed to restart'))
    }
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.data || (isZh.value ? '重启失败' : 'Failed to restart'))
  }
}

function handleRemove(item?: ListenerItem | null) {
  const target = item || selectedRow.value
  if (!target) return
  closeContextMenu()
  ElMessageBox.confirm(
    isZh.value
      ? `确定要删除监听器 "${target.ListenAddress}" 吗？`
      : `Are you sure you want to remove listener "${target.ListenAddress}"?`,
    isZh.value ? '确认删除' : 'Confirm Delete',
    {
      confirmButtonText: isZh.value ? '删除' : 'Remove',
      cancelButtonText: isZh.value ? '取消' : 'Cancel',
      type: 'warning',
    },
  )
    .then(async () => {
      try {
        const res = await ListenerAPI.deleteListener({ listenAddress: target.ListenAddress || '' })
        if (res.data?.status === 200) {
          ElMessage.success(isZh.value ? '监听器已删除' : 'Listener removed')
        } else {
          ElMessage.error(res.data?.data || (isZh.value ? '删除失败' : 'Failed to remove'))
        }
        await fetchListeners()
      } catch (error: any) {
        ElMessage.error(error?.response?.data?.data || (isZh.value ? '删除失败' : 'Failed to remove'))
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
      ElMessage.success(isZh.value ? `监听器 ${formData.listenAddress} 已保存` : `Listener ${formData.listenAddress} saved successfully`)
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
  ElMessage.info(isZh.value ? '监听器管理：用于配置反向回连或正向连接的主控监听端。' : 'Listener Management: configure reverse callbacks and forward connections.')
}

onMounted(() => {
  fetchListeners()
  window.addEventListener('mousedown', closeContextMenu)
})

onBeforeUnmount(() => {
  window.removeEventListener('mousedown', closeContextMenu)
})
</script>

<style scoped>
.cs-listeners-panel {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-bg);
  overflow: hidden;
}

.cs-listener-toolbar {
  height: 36px;
  background: var(--cs-titlebar-bg);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 0 12px;
  flex-shrink: 0;
}

.cs-header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.cs-header-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.cs-title-group {
  display: flex;
  align-items: center;
  gap: 6px;
}

.cs-title-icon {
  font-size: 18px;
  color: #3884ff;
}

.cs-title {
  font-size: 17px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

/* 扁平风格分段切换器 */
.cs-segmented-toggle {
  display: inline-flex;
  align-items: center;
  background: var(--cs-bg, #181818);
  border: 1px solid var(--cs-border, #333333);
  border-radius: 4px;
  padding: 2px;
  gap: 2px;
  user-select: none;
}

.cs-segment-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 26px;
  padding: 0 10px;
  font-size: 15px;
  font-family: var(--cs-font-ui);
  color: var(--cs-text-dim, #888888);
  background: transparent;
  border: none;
  border-radius: 3px;
  cursor: pointer;
  transition: all 120ms ease;
  white-space: nowrap;
}

.cs-segment-btn:hover {
  color: var(--cs-text-bright, #ffffff);
  background: var(--cs-hover, #2a2a2a);
}

.cs-segment-btn.active {
  color: #ffffff;
  background: #1a5c9a;
  font-weight: 600;
}

.seg-icon {
  font-size: 13px;
  line-height: 1;
}

.seg-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  font-size: 12px;
  font-family: var(--cs-font-mono);
  font-weight: bold;
  border-radius: 9px;
  background: rgba(255, 255, 255, 0.2);
  color: inherit;
}

.cs-segment-btn:not(.active) .seg-badge {
  background: var(--cs-panel, #222222);
  color: var(--cs-text-dim, #888888);
  border: 1px solid var(--cs-border, #383838);
}

.cs-spacer {
  flex: 1;
}

.cs-search {
  width: 280px;
}

.cs-listener-body {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}

.cs-view-pane {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
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

.cs-listener-ctx-menu {
  position: fixed;
  background: var(--cs-panel);
  border: 1px solid var(--cs-border);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.45);
  border-radius: 2px;
  padding: 4px 0;
  z-index: 9999;
  min-width: 180px;
}

.cs-listener-ctx-menu .ctx-item {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 14px;
  font-size: 16.5px;
  color: var(--cs-text);
  cursor: pointer;
}

.cs-listener-ctx-menu .ctx-item:hover {
  background: var(--cs-hover);
  color: var(--cs-text-bright);
}

.cs-listener-ctx-menu .ctx-arrow {
  font-size: 12px;
  color: var(--cs-text-dim);
  margin-left: 14px;
}

.cs-listener-ctx-menu .ctx-color-palette {
  position: absolute;
  top: -2px;
  left: 100%;
  background: var(--cs-panel);
  border: 1px solid var(--cs-border);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.45);
  border-radius: 2px;
  padding: 6px;
  display: flex;
  gap: 6px;
  z-index: 10000;
}

.color-box {
  width: 24px;
  height: 24px;
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
  font-size: 18px;
  color: var(--cs-text-dim);
}
</style>
