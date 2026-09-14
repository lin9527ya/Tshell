<template>
  <div class="global-downloads cs-font-ui">
    <!-- Toolbar -->
    <div class="gd-toolbar">
      <div class="gd-title-group">
        <el-icon class="gd-title-icon"><FolderOpened /></el-icon>
        <span class="gd-title">
          {{ isZh ? (isClientScoped ? `下载管理: ${targetClientTitle}` : '下载文件管理') : (isClientScoped ? `Downloads: ${targetClientTitle}` : 'Downloaded Files') }}
        </span>
        <span class="gd-count">
          ({{ isClientScoped && scopeFilter === 'current' ? `${filteredList.length} ${isZh ? '个当前文件' : 'current'}` : `${filteredList.length} ${isZh ? '个文件' : 'files'}` }})
        </span>
      </div>

      <!-- Scope Filter Toggle (When in client-scoped mode) -->
      <div v-if="isClientScoped" class="gd-scope-toggle">
        <button
          class="gd-scope-btn"
          :class="{ active: scopeFilter === 'current' }"
          @click="scopeFilter = 'current'"
        >
          {{ isZh ? '当前会话' : 'Current' }}
        </button>
        <button
          class="gd-scope-btn"
          :class="{ active: scopeFilter === 'all' }"
          @click="scopeFilter = 'all'"
        >
          {{ isZh ? '全部会话' : 'All' }}
        </button>
      </div>

      <div class="gd-spacer" />

      <el-input
        v-model="searchKey"
        :placeholder="isZh ? '过滤文件名、主机、远端路径...' : 'Filter file, host, path...'"
        size="small"
        clearable
        class="gd-search-input"
      />

      <button class="cs-btn" :disabled="loading" @click="refreshAll">
        <el-icon :class="{ 'is-loading': loading }"><Refresh /></el-icon>
        <span>{{ isZh ? '刷新' : 'Refresh' }}</span>
      </button>
    </div>

    <!-- Table -->
    <div class="gd-table-wrap">
      <el-table
        ref="tableRef"
        :data="filteredList"
        :row-key="(r: any) => r.uid + ':' + r.filePath"
        size="small"
        height="100%"
        highlight-current-row
        v-loading="loading"
        class="cs-session-table"
        @current-change="onCurrentChange"
        @row-dblclick="handleDownload"
      >
        <el-table-column type="selection" width="38" align="center" :reserve-selection="true" />

        <el-table-column v-if="!isClientScoped || scopeFilter === 'all'" prop="host" :label="isZh ? '会话 / 主机' : 'Session / Host'" width="180" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="host-cell font-bold">{{ row.host || row.uid.slice(0, 8) }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="fileName" :label="isZh ? '文件名' : 'File Name'" min-width="220" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <div class="gd-file-cell">
              <el-icon class="gd-file-icon" :class="getFileIconClass(row.fileName)">
                <Document />
              </el-icon>
              <span class="file-name-cell font-bold">{{ row.fileName }}</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column prop="fileSize" :label="isZh ? '大小' : 'Size'" width="120" sortable align="right">
          <template #default="{ row }">
            <span class="cs-font-mono">{{ row.fileSize || '-' }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="downloadTime" :label="isZh ? '下载日期' : 'Date'" width="180" sortable align="center" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="cs-font-mono text-dim">{{ formatTime(row.downloadTime) }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="downloadPart" :label="isZh ? '传输进度' : 'Progress'" width="140" sortable align="center">
          <template #default="{ row }">
            <div class="progress-cell">
              <el-progress
                :percentage="parseInt(row.downloadPart) || 100"
                :status="parseInt(row.downloadPart) >= 100 ? 'success' : undefined"
                :stroke-width="7"
              />
            </div>
          </template>
        </el-table-column>

        <el-table-column prop="filePath" :label="isZh ? '远端路径' : 'Remote Path'" min-width="260" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="cs-font-mono text-dim" :title="row.filePath">{{ row.filePath }}</span>
          </template>
        </el-table-column>

        <el-table-column :label="isZh ? '操作' : 'Actions'" width="180" align="center">
          <template #default="{ row }">
            <button class="cs-btn cs-btn-xs cs-btn-primary" @click="handleDownload(row)">
              {{ isZh ? '下载到本地' : 'Download' }}
            </button>
            <button class="cs-btn cs-btn-xs cs-btn-danger" style="margin-left: 6px" @click="handleDelete(row)">
              {{ isZh ? '删除' : 'Delete' }}
            </button>
          </template>
        </el-table-column>

        <template #empty>
          <div class="gd-empty-state">
            <div class="empty-icon">📁</div>
            <div class="empty-title">{{ isZh ? '暂无下载的文件' : 'No Downloaded Files' }}</div>
            <div class="empty-desc">
              {{ isZh ? '在文件管理器中右键选择「下载文件」后，下载记录将在此聚合展示。' : 'Files downloaded via File Browser will appear here.' }}
            </div>
          </div>
        </template>
      </el-table>
    </div>

    <!-- Bottom Action Bar (CS Native Button Row) -->
    <div class="gd-bottom-bar">
      <button class="cs-btn cs-btn-primary" :disabled="!selectedRow" @click="handleDownloadSelected">
        {{ isZh ? '下载到本地 (Sync)' : 'Download' }}
      </button>
      <button class="cs-btn cs-btn-danger" :disabled="!selectedRow" @click="handleDeleteSelected">
        {{ isZh ? '删除记录 (Delete)' : 'Delete' }}
      </button>
      <button class="cs-btn" @click="refreshAll">
        {{ isZh ? '刷新列表' : 'Refresh' }}
      </button>
      <button class="cs-btn" @click="showHelp">
        {{ isZh ? '帮助' : 'Help' }}
      </button>

      <div class="gd-bottom-spacer" />

      <span class="gd-status-info">
        {{ isZh ? `共 ${filteredList.length} 个下载项` : `${filteredList.length} download items` }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, onUnmounted, ref } from 'vue'
import type { ComputedRef, Ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { FolderOpened, Refresh, Document } from '@element-plus/icons-vue'
import { useClientsStore } from '@/stores/clients'
import { useWorkspaceStore } from '@/stores/workspace'
import { useI18nStore } from '@/stores/i18n'
import ClientAPI from '@/api/clients'

const props = withDefaults(
  defineProps<{
    uid?: string
  }>(),
  {
    uid: '',
  },
)

interface DownloadRow {
  uid: string
  host: string
  fileName: string
  filePath: string
  fileSize: string
  downloadTime: string
  downloadPart: string
}

const clientsStore = useClientsStore()
const workspace = useWorkspaceStore()
const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

// Scope identification: from props, injection or global
const injectedUid = inject<Ref<string> | ComputedRef<string> | null>('realUid', null)
const resolvedUid = computed(() => {
  if (props.uid && props.uid.length > 5) return props.uid
  if (injectedUid && (injectedUid as any).value && String((injectedUid as any).value).length > 5) {
    return String((injectedUid as any).value)
  }
  return ''
})

const isClientScoped = computed(() => !!resolvedUid.value)
const scopeFilter = ref<'current' | 'all'>('current')

const targetClient = computed(() => {
  if (resolvedUid.value) {
    return clientsStore.getClientByUid(resolvedUid.value)
  }
  return null
})

const targetClientTitle = computed(() => {
  const c = targetClient.value
  if (!c) return resolvedUid.value.slice(0, 8) || (isZh.value ? '未指定客户端' : 'No Client')
  return `${c.Username || 'user'}@${c.Computer || c.ExternalIP || 'host'} (${c.InternalIP || c.ExternalIP || '127.0.0.1'})`
})

const loading = ref(false)
const searchKey = ref('')
const selectedRow = ref<DownloadRow | null>(null)
const downloadItems = ref<DownloadRow[]>([])

const filteredList = computed(() => {
  let list = downloadItems.value
  if (isClientScoped.value && scopeFilter.value === 'current') {
    list = list.filter((d) => d.uid === resolvedUid.value)
  }
  if (!searchKey.value.trim()) return list
  const q = searchKey.value.toLowerCase().trim()
  return list.filter((d) => {
    return (
      d.fileName.toLowerCase().includes(q) ||
      d.filePath.toLowerCase().includes(q) ||
      d.host.toLowerCase().includes(q) ||
      d.uid.toLowerCase().includes(q)
    )
  })
})

function getFileIconClass(fileName: string) {
  const ext = (fileName || '').split('.').pop()?.toLowerCase() || ''
  if (['exe', 'bin', 'dll', 'so', 'elf', 'dylib'].includes(ext)) return 'icon-binary'
  if (['sh', 'ps1', 'bat', 'cmd', 'py', 'vbs'].includes(ext)) return 'icon-script'
  if (['zip', 'tar', 'gz', '7z', 'rar'].includes(ext)) return 'icon-zip'
  return 'icon-doc'
}

function formatTime(timestamp: any) {
  if (!timestamp || timestamp === '-') return '-'
  try {
    const d = new Date(timestamp)
    return isNaN(d.getTime()) ? String(timestamp) : d.toLocaleString()
  } catch {
    return String(timestamp)
  }
}

function onCurrentChange(row: DownloadRow | null) {
  selectedRow.value = row
}

async function refreshAll() {
  loading.value = true
  const list: DownloadRow[] = []
  const clients = isClientScoped.value && scopeFilter.value === 'current'
    ? (targetClient.value ? [targetClient.value] : clientsStore.clientsList.filter(c => c.Uid === resolvedUid.value))
    : clientsStore.clientsList

  try {
    for (const c of clients) {
      try {
        const res = await ClientAPI.get_downloads_info({ uid: c.Uid })
        if (res.data?.status === 200 && Array.isArray(res.data.data)) {
          const hostLabel = `${c.Username || 'user'}@${c.Computer || c.ExternalIP || 'host'}`
          for (const item of res.data.data) {
            list.push({
              uid: c.Uid,
              host: hostLabel,
              fileName: item.fileName,
              filePath: item.filePath,
              fileSize: item.fileSize || '-',
              downloadTime: item.downloadTime || '-',
              downloadPart: item.downloadPart || '100',
            })
          }
        }
      } catch {}
    }
    downloadItems.value = list
  } finally {
    loading.value = false
  }
}

async function handleDownload(row: DownloadRow) {
  const loadingMsg = ElMessage.info({
    message: isZh.value ? `正在准备下载 ${row.fileName}...` : `Preparing download: ${row.fileName}`,
    duration: 0,
  })
  try {
    const res = await ClientAPI.download_downloaded_file({ uid: row.uid, filePath: row.filePath })
    loadingMsg.close()
    if (res.status === 200) {
      const blob = new Blob([res.data], { type: res.headers['content-type'] || 'application/octet-stream' })
      const link = document.createElement('a')
      const href = window.URL.createObjectURL(blob)
      link.href = href
      link.download = row.fileName
      document.body.appendChild(link)
      link.click()
      document.body.removeChild(link)
      window.URL.revokeObjectURL(href)
      ElMessage.success(isZh.value ? '文件已成功保存到本地' : 'File downloaded successfully')
    } else {
      ElMessage.error(isZh.value ? '文件下载失败' : 'Download failed')
    }
  } catch (error: any) {
    loadingMsg.close()
    ElMessage.error(`${isZh.value ? '下载异常' : 'Download error'}: ${error?.message || 'Unknown'}`)
  }
}

function handleDownloadSelected() {
  if (selectedRow.value) handleDownload(selectedRow.value)
}

function handleDelete(row: DownloadRow) {
  ElMessageBox.confirm(
    isZh.value ? `确定从服务端删除下载的文件「${row.fileName}」吗？` : `Delete downloaded file "${row.fileName}" from server?`,
    isZh.value ? '删除确认' : 'Delete Confirmation',
    {
      type: 'warning',
      confirmButtonText: isZh.value ? '删除' : 'Delete',
      cancelButtonText: isZh.value ? '取消' : 'Cancel',
    },
  ).then(async () => {
    try {
      await ClientAPI.delete_downloaded_file({ uid: row.uid, filePath: row.filePath })
      ElMessage.success(isZh.value ? '文件已删除' : 'File deleted from server')
      await refreshAll()
    } catch {
      ElMessage.error(isZh.value ? '删除失败' : 'Delete failed')
    }
  }).catch(() => {})
}

function handleDeleteSelected() {
  if (selectedRow.value) handleDelete(selectedRow.value)
}

function showHelp() {
  ElMessageBox.alert(
    isZh.value
      ? '【下载列表说明】\n1. 所有在文件管理中对远端文件执行下载操作的任务均会在此汇总。\n2. 下载完成后，可点击「下载到本地」将文件从 C2 服务端保存到当前操作机。\n3. 支持通过当前会话/全部会话自由切换查看范围。'
      : '【Downloads Help】\n1. Files downloaded in File Manager are tracked here.\n2. Click Download to save files from C2 server to your local machine.',
    isZh.value ? '下载功能帮助' : 'Downloads Help',
    { confirmButtonText: isZh.value ? '确定' : 'OK' },
  )
}

let pollTimer: any = null

onMounted(() => {
  refreshAll()
  pollTimer = setInterval(() => {
    // 只有存在未完成下载任务时自动轮询
    if (downloadItems.value.some(d => parseInt(d.downloadPart) < 100)) {
      refreshAll()
    }
  }, 3000)
})

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
})
</script>

<style scoped>
.global-downloads {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-panel);
  overflow: hidden;
}

.gd-toolbar {
  height: 36px;
  background: var(--cs-titlebar-bg);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 12px;
  flex-shrink: 0;
}

.gd-title-group {
  display: flex;
  align-items: center;
  gap: 8px;
}

.gd-title-icon {
  font-size: 18px;
  color: #ffc107;
}

.gd-title {
  font-size: 17px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.gd-count {
  font-size: 16px;
  color: var(--cs-text-dim);
}

.gd-scope-toggle {
  display: inline-flex;
  align-items: center;
  background: var(--cs-bg, #181818);
  border: 1px solid var(--cs-border, #333333);
  border-radius: 4px;
  padding: 2px;
  gap: 2px;
  margin-left: 6px;
}

.gd-scope-btn {
  height: 24px;
  padding: 0 10px;
  font-size: 14px;
  color: var(--cs-text-dim);
  background: transparent;
  border: none;
  border-radius: 3px;
  cursor: pointer;
  white-space: nowrap;
}

.gd-scope-btn:hover {
  color: var(--cs-text);
  background: var(--cs-hover);
}

.gd-scope-btn.active {
  color: #ffffff;
  background: #1a5c9a;
  font-weight: 600;
}

.gd-spacer { flex: 1; }

.gd-search-input {
  width: 260px;
}

.gd-table-wrap {
  flex: 1;
  overflow: hidden;
  position: relative;
}

.gd-file-cell {
  display: flex;
  align-items: center;
  gap: 6px;
}

.gd-file-icon {
  font-size: 16px;
  flex-shrink: 0;
}

.icon-binary { color: #52c41a; }
.icon-script { color: #13c2c2; }
.icon-zip { color: #fa8c16; }
.icon-doc { color: #8c8c8c; }

.file-name-cell {
  color: var(--cs-text-bright);
  font-size: 16.5px;
}

.host-cell {
  color: var(--cs-text);
  font-size: 16px;
}

.progress-cell {
  width: 100%;
}

.gd-bottom-bar {
  height: 38px;
  padding: 0 12px;
  display: flex;
  align-items: center;
  gap: 8px;
  background: var(--cs-panel);
  border-top: 1px solid var(--cs-border);
  flex-shrink: 0;
}

.gd-bottom-spacer { flex: 1; }

.gd-status-info {
  font-size: 15px;
  color: var(--cs-text-dim);
  font-family: var(--cs-font-mono);
}

.gd-empty-state {
  padding: 40px 20px;
  text-align: center;
}

.empty-icon {
  font-size: 36px;
  margin-bottom: 8px;
}

.empty-title {
  font-size: 18px;
  font-weight: 600;
  color: var(--cs-text-bright);
  margin-bottom: 6px;
}

.empty-desc {
  font-size: 16px;
  color: var(--cs-text-dim);
}
</style>
