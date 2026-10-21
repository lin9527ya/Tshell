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
        :placeholder="activeView === 'normal' ? (i18nStore.tl('过滤反向监听器 (ID/模式/地址/备注)...', 'Filter reverse listeners...')) : (i18nStore.tl('过滤正向监听器 (ID/模式/地址/备注)...', 'Filter forward listeners...'))"
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

      <!-- vshell 表格工具：刷新 / 密度 / 列设置（反向监听器） -->
      <TableTools
        v-if="activeView === 'normal'"
        :columns="columnOptions"
        :visible="colVisible"
        :density="density"
        :loading="manualRefreshing"
        @update:visible="colVisible = $event"
        @update:density="setDensity"
        @refresh="refreshReverse"
      />
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
            :class="['cs-session-table', density === 'default' ? '' : 'cs-density-' + density]"
            @row-click="onRowClick"
            @row-dblclick="onRowDblClick"
          >
            <!-- vshell 监听器列：ID / 备注 / 模式 / 监听地址 / 外网连接地址 / 超时心跳 / 心跳间隔 / 间隔 / 状态 / 操作 -->
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
            <el-table-column v-if="colVisible.mode" prop="Type" :label="i18nStore.tl('模式', 'Mode')" width="100" sortable show-overflow-tooltip>
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
                <span class="cs-font-mono cs-cell-text">{{ row.ConnectAddress || '-' }}</span>
              </template>
            </el-table-column>
            <el-table-column v-if="colVisible.timeout" prop="HeartbeatTimeout" width="120" sortable align="right" :sort-orders="['ascending', 'descending', null]">
              <template #header>
                <span :title="i18nStore.tl('单位：次', 'unit: times')">{{ i18nStore.tl('超时心跳次数', 'Timeout Beats') }}</span>
              </template>
              <template #default="{ row }">
                <span class="cs-cell-text">{{ row.HeartbeatTimeout || '30' }}</span>
              </template>
            </el-table-column>
            <el-table-column v-if="colVisible.interval" prop="HeartbeatInterval" width="110" sortable align="right" :sort-orders="['ascending', 'descending', null]">
              <template #header>
                <span :title="i18nStore.tl('单位：秒', 'unit: seconds')">{{ i18nStore.tl('心跳间隔', 'Heartbeat Interval') }}</span>
              </template>
              <template #default="{ row }">
                <span class="cs-cell-text">{{ row.HeartbeatInterval || '10' }}</span>
              </template>
            </el-table-column>
            <el-table-column v-if="colVisible.sleep" prop="SleepSeconds" width="90" sortable align="right" :sort-orders="['ascending', 'descending', null]">
              <template #header>
                <span :title="i18nStore.tl('默认设置 sleep（取自监听器绑定的 C2 画像）', 'Default sleep (from the listener C2 profile)')">{{ i18nStore.tl('间隔', 'Sleep') }}</span>
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
            <el-table-column :label="i18nStore.tl('操作', 'Actions')" min-width="240">
              <template #default="{ row }">
                <div class="row-actions">
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
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import EditListenerDialog from '@/layout/Dialogs/EditListenerDialog.vue'
import VsIcon from '@/layout-vshell/VsIcon.vue'
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
  Profile?: string
  SleepSeconds?: number
  color?: string
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

// ---- vshell 表格工具：密度 / 列设置（反向监听器） ----
const DENSITY_KEY = 'tshell-listener-density'
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

type ColKey = 'id' | 'note' | 'mode' | 'listenAddr' | 'connectAddr' | 'timeout' | 'interval' | 'sleep' | 'status'
const COLUMNS_KEY = 'tshell-listener-columns'
const DEFAULT_COLS: Record<ColKey, boolean> = {
  id: true, note: true, mode: true, listenAddr: true, connectAddr: true,
  timeout: true, interval: true, sleep: true, status: true,
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
  { key: 'timeout', label: i18nStore.tl('超时心跳次数', 'Timeout Beats') },
  { key: 'interval', label: i18nStore.tl('心跳间隔', 'Heartbeat Interval') },
  { key: 'sleep', label: i18nStore.tl('间隔', 'Sleep') },
  { key: 'status', label: i18nStore.tl('状态', 'Status') },
])

const manualRefreshing = ref(false)
async function refreshReverse() {
  if (manualRefreshing.value) return
  manualRefreshing.value = true
  try { await fetchListeners() } finally { manualRefreshing.value = false }
}

function sleepSeconds(row: ListenerItem): number {
  const sec = Number(row.SleepSeconds)
  return Number.isFinite(sec) && sec > 0 ? sec : 5
}

function sleepLabel(row: ListenerItem): string {
  return `${sleepSeconds(row)}s`
}

function sleepTitle(row: ListenerItem): string {
  const sec = sleepSeconds(row)
  const profile = row.Profile || 'default'
  return isZh.value
    ? `默认设置 sleep：${sec}s（C2 画像 ${profile}）`
    : `Default sleep: ${sec}s (C2 profile ${profile})`
}

const filteredListeners = computed(() => {
  const q = searchKey.value.toLowerCase().trim()
  if (!q) return listenersList.value
  return listenersList.value.filter((l) =>
    [String(l.Id ?? ''), l.Type, l.ListenAddress, l.ConnectAddress, l.Note || '']
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

function onForwardCountChange(cnt: number) {
  forwardCount.value = cnt
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

      listenersList.value = reverseList.map((item: any) => ({
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
        Profile: item.Profile || '',
        SleepSeconds: Number(item.SleepSeconds) || 0,
        color: item.color || '',
      }))
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
