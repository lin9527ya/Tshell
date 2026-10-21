<template>
  <div class="cs-listeners-panel cs-font-ui">
    <!-- vshell 工具条：内容上方一行（左：标题 / 下划线页签，右：搜索 + 操作） -->
    <div class="cs-listener-toolbar">
      <!-- 正/反向监听器：下划线页签（激活 = 主色字 + 2px 主色下划线） -->
      <div class="cs-view-tabs">
        <button
          class="cs-view-tab"
          :class="{ active: activeView === 'normal' }"
          @click="switchView('normal')"
        >
          <span class="tab-label">{{ i18nStore.tl('反向监听器', 'Reverse Listeners') }}</span>
          <span class="tab-badge">{{ listenersList.length }}</span>
        </button>
        <button
          class="cs-view-tab"
          :class="{ active: activeView === 'forward' }"
          @click="switchView('forward')"
        >
          <span class="tab-label">{{ i18nStore.tl('正向监听器', 'Forward Listeners') }}</span>
          <span class="tab-badge">{{ forwardCount }}</span>
        </button>
      </div>

      <span v-if="activeView === 'normal'" class="cs-toolbar-hint">
        {{ i18nStore.tl(`共 ${filteredListeners.length} 个反向监听器`, `${filteredListeners.length} reverse listener(s)`) }}
      </span>

      <div class="cs-spacer" />

      <el-input
        v-model="searchKey"
        :placeholder="activeView === 'normal' ? (i18nStore.tl('过滤反向监听器 (名称/载荷/地址/备注)...', 'Filter reverse listeners...')) : (i18nStore.tl('过滤正向监听器 (名称/载荷/地址/备注)...', 'Filter forward listeners...'))"
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

      <button class="cs-btn" :disabled="currentLoading" @click="handleRefresh">
        <el-icon :class="{ 'is-loading': currentLoading }"><Refresh /></el-icon>
        <span>{{ i18nStore.tl('刷新', 'Refresh') }}</span>
      </button>

      <!-- 次要入口收进「更多」；单条启停/编辑/删除在表格行操作列 -->
      <el-dropdown trigger="click" @command="onMoreCmd">
        <button class="cs-btn">{{ i18nStore.tl('更多', 'More') }} ▾</button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item command="phish">{{ i18nStore.tl('钓鱼邮件', 'Spear Phish') }}</el-dropdown-item>
            <el-dropdown-item command="redirector">{{ i18nStore.tl('重定向器配置', 'Redirector Config') }}</el-dropdown-item>
            <el-dropdown-item command="help">{{ i18nStore.tl('帮助', 'Help') }}</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>

      <template v-if="activeView === 'normal'">
        <span class="cs-toolbar-divider" />
        <button class="cs-btn cs-btn-primary" @click="openAddDialog">{{ i18nStore.tl('添加', 'Add') }}</button>
      </template>
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
            class="cs-session-table"
            @row-click="onRowClick"
            @row-dblclick="onRowDblClick"
          >
            <el-table-column prop="name" :label="i18nStore.tl('名称', 'name')" min-width="110" sortable show-overflow-tooltip />
            <el-table-column prop="payload" :label="i18nStore.tl('载荷', 'payload')" min-width="160" sortable show-overflow-tooltip />
            <el-table-column prop="host" :label="i18nStore.tl('主机', 'host')" min-width="140" sortable show-overflow-tooltip />
            <el-table-column prop="port" :label="i18nStore.tl('端口', 'port')" width="85" sortable align="left" />
            <el-table-column prop="bindto" :label="i18nStore.tl('回连地址', 'bindto')" min-width="160" sortable show-overflow-tooltip />
            <el-table-column :label="i18nStore.tl('状态', 'status')" width="115" sortable align="center">
              <template #default="{ row }">
                <span class="vs-status-tag" :class="row.Status === 1 ? 'is-online' : 'is-offline'">
                  {{ row.Status === 1 ? (i18nStore.tl('监听中', 'Listening')) : (i18nStore.tl('已停止', 'Stopped')) }}
                </span>
              </template>
            </el-table-column>
            <el-table-column prop="note" :label="i18nStore.tl('备注', 'note')" min-width="140" sortable show-overflow-tooltip />
            <el-table-column :label="i18nStore.tl('VKey', 'vkey')" min-width="115" show-overflow-tooltip>
              <template #default="{ row }">
                <span class="cs-font-mono">{{ maskSecret(row.VKey) }}</span>
              </template>
            </el-table-column>
            <el-table-column :label="i18nStore.tl('加密盐', 'encryption_salt')" min-width="115" show-overflow-tooltip>
              <template #default="{ row }">
                <span class="cs-font-mono">{{ maskSecret(row.EncryptionSalt) }}</span>
              </template>
            </el-table-column>

            <el-table-column :label="i18nStore.tl('操作', 'Actions')" width="210" fixed="right">
              <template #default="{ row }">
                <div class="row-actions">
                  <a
                    v-if="row.Status === 1"
                    class="row-link"
                    @click.stop="handleStop(row)"
                  >{{ i18nStore.tl('停用', 'Stop') }}</a>
                  <a
                    v-else
                    class="row-link"
                    @click.stop="handleStart(row)"
                  >{{ i18nStore.tl('启用', 'Start') }}</a>
                  <a class="row-link" @click.stop="handleRestart(row)">{{ i18nStore.tl('重启', 'Restart') }}</a>
                  <a class="row-link" @click.stop="openEditDialog(row)">{{ i18nStore.tl('编辑', 'Edit') }}</a>
                  <a class="row-link row-link-danger" @click.stop="handleRemove(row)">{{ i18nStore.tl('删除', 'Remove') }}</a>
                </div>
              </template>
            </el-table-column>

            <template #empty>
              <EmptyState variant="inline" icon="📡" :title="i18nStore.tl('暂无反向监听器', 'No Listeners Configured')" />
            </template>
          </el-table>
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

    <!-- New / Edit Listener Dialog -->
    <EditListenerDialog
      v-model="showEditDialog"
      :listener-data="editingListener"
      scope="reverse"
      :saving="savingListener"
      @save="onListenerSaved"
    />

    <!-- 页面内局部弹窗（入口在本页工具条） -->
    <SpearPhishDialog v-model="showSpearPhish" />
    <!-- 重定向器配置：按当前画像生成，避免手写配置与画像漂移 -->
    <RedirectorDialog v-model:visible="showRedirector" />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import EditListenerDialog from '@/layout/Dialogs/EditListenerDialog.vue'
import SpearPhishDialog from '@/layout/Dialogs/Attacks/SpearPhishDialog.vue'
import ForwardListeners from './ForwardListeners.vue'
import RedirectorDialog from '@/layout/Dialogs/RedirectorDialog.vue'
import ListenerAPI from '@/api/listener'
import { useI18nStore } from '@/stores/i18n'
import EmptyState from '@/components/Common/EmptyState.vue'

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
// 攻击工具弹窗（页面内局部状态）
const showSpearPhish = ref(false)
const showRedirector = ref(false)

const currentLoading = computed(() => (activeView.value === 'normal' ? loading.value : (forwardRef.value?.loading ?? false)))

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

function onMoreCmd(cmd: string) {
  if (cmd === 'phish') showSpearPhish.value = true
  else if (cmd === 'redirector') showRedirector.value = true
  else if (cmd === 'help') handleHelp()
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
}

async function handleStart(item: ListenerItem | null) {
  if (!item) return
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
  // 重启会断开该监听器下全部客户端连接(客户端将自动重连),需确认
  try {
    await ElMessageBox.confirm(
      isZh.value ? `重启监听器 ${item.ListenAddress} 将断开其下所有客户端连接(客户端会自动重连),确定继续?` : `Restarting ${item.ListenAddress} will disconnect all its clients (they will auto-reconnect). Continue?`,
      isZh.value ? '确认重启' : 'Confirm Restart',
      { confirmButtonText: isZh.value ? '重启' : 'Restart', cancelButtonText: isZh.value ? '取消' : 'Cancel', type: 'warning' },
    )
  } catch {
    return
  }
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
        profile: formData.profile || "default",
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
        profile: formData.profile || "default",
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
})

onBeforeUnmount(() => {
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

/* 工具条：白底 + 底部分隔线由 vshell-ep.css 的 .cs-listener-toolbar 接管 */
.cs-listener-toolbar {
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

.cs-title-icon {
  font-size: 16px;
  color: var(--cs-active);
}

.cs-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
  white-space: nowrap;
}

/* 下划线页签（vshell：无填充块；激活 = 主色字 + 2px 主色下划线） */
.cs-view-tabs {
  display: inline-flex;
  align-items: center;
  gap: 2px;
}

.cs-view-tab {
  position: relative;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 32px;
  padding: 0 10px;
  font-size: 14px;
  font-family: var(--cs-font-ui);
  color: var(--cs-text-dim);
  background: transparent;
  border: 0;
  cursor: pointer;
  transition: color 120ms ease;
  white-space: nowrap;
}

.cs-view-tab:hover {
  color: var(--cs-text-bright);
}

.cs-view-tab.active {
  color: var(--cs-active);
  font-weight: 600;
}

.cs-view-tab.active::after {
  content: '';
  position: absolute;
  left: 10px;
  right: 10px;
  bottom: 0;
  height: 2px;
  background: var(--cs-active);
}

.tab-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  font-size: 12px;
  line-height: 1;
  font-family: var(--cs-font-mono);
  border-radius: 9px;
  background: var(--cs-table-head);
  color: var(--cs-text-dim);
  border: 1px solid var(--cs-card-border);
}

.cs-view-tab.active .tab-badge {
  background: color-mix(in srgb, var(--cs-active) 12%, transparent);
  border-color: color-mix(in srgb, var(--cs-active) 30%, transparent);
  color: var(--cs-active);
}

.cs-toolbar-hint {
  font-size: 12px;
  color: var(--cs-text-dim);
  white-space: nowrap;
}

.cs-toolbar-divider {
  width: 1px;
  height: 20px;
  flex-shrink: 0;
  background: var(--cs-card-border);
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
.cs-listener-ctx-menu {
  position: fixed;
  padding: 4px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  box-shadow: 0 3px 6px -4px rgba(0, 0, 0, 0.12), 0 6px 16px 0 rgba(0, 0, 0, 0.08);
  z-index: 9999;
  min-width: 180px;
}

.cs-listener-ctx-menu .ctx-item {
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

.cs-listener-ctx-menu .ctx-item:hover {
  background: var(--cs-hover);
  color: var(--cs-active);
}

.cs-listener-ctx-menu .ctx-item.disabled {
  opacity: 0.45;
}

.cs-listener-ctx-menu .ctx-item.ctx-danger,
.cs-listener-ctx-menu .ctx-item.ctx-danger:hover {
  color: var(--cs-error);
}

.cs-listener-ctx-menu .ctx-item.ctx-danger:hover {
  background: color-mix(in srgb, var(--cs-error) 12%, transparent);
}

.cs-listener-ctx-menu .ctx-divider {
  height: 1px;
  margin: 4px 0;
  background: var(--cs-card-border);
}

.cs-listener-ctx-menu .ctx-arrow {
  font-size: 12px;
  color: var(--cs-text-dim);
  margin-left: 14px;
}

.cs-listener-ctx-menu .ctx-color-palette {
  position: absolute;
  top: -4px;
  left: 100%;
  padding: 6px;
  display: flex;
  gap: 6px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  box-shadow: 0 3px 6px -4px rgba(0, 0, 0, 0.12), 0 6px 16px 0 rgba(0, 0, 0, 0.08);
  z-index: 10000;
}

.color-box {
  width: 24px;
  height: 24px;
  border: 1px solid var(--cs-border-strong);
  border-radius: 4px;
  cursor: pointer;
}

.color-box:hover {
  border-color: var(--cs-active);
}

/* 行内操作（vshell 范式：文字链接） */
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

.row-link-danger {
  color: var(--cs-error, #f56c6c);
}
</style>
