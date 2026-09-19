<template>
  <div class="applications-view cs-font-ui">
    <!-- Toolbar -->
    <div class="av-toolbar">
      <div class="av-title-group">
        <span class="av-title">{{ isZh ? '应用程序' : 'Applications' }}</span>
        <span class="av-count">({{ filteredList.length }} {{ isZh ? '个应用' : 'applications' }})</span>
        <span class="av-demo-badge">{{ isZh ? '演示' : 'Demo' }}</span>
        <span class="av-demo-note">{{ isZh ? '数据为本地演示，后端无 applications 接口' : 'Local demo data — no backend applications API' }}</span>
      </div>
      <div class="av-spacer" />
      <el-input
        v-model="searchKey"
        :placeholder="isZh ? '过滤外网IP、内网IP、应用名称...' : 'Filter IP, application, version...'"
        size="small"
        clearable
        class="av-search-input"
      />
      <button class="av-btn-icon" :title="isZh ? '刷新应用程序列表' : 'Refresh Applications'" @click="refresh">
        <svg viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" stroke-width="2">
          <polyline points="23 4 23 10 17 10" />
          <path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10" />
        </svg>
      </button>
    </div>

    <!-- Table -->
    <div class="av-table-wrap">
      <el-table
        ref="tableRef"
        :data="filteredList"
        row-key="id"
        size="small"
        height="100%"
        highlight-current-row
        class="cs-session-table"
        @current-change="onCurrentChange"
      >
        <el-table-column type="selection" width="30" align="center" :reserve-selection="true" />
        <el-table-column prop="external" :label="isZh ? '外网IP' : 'external'" width="140" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="mono-cell">{{ row.external }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="internal" :label="isZh ? '内网IP' : 'internal'" width="140" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="mono-cell">{{ row.internal }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="application" :label="isZh ? '应用程序' : 'application'" min-width="200" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="app-name font-bold">{{ row.application }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="version" :label="isZh ? '版本' : 'version'" width="140" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="mono-cell">{{ row.version || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="description" :label="isZh ? '说明 / User-Agent' : 'description'" min-width="260" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="desc-cell">{{ row.description }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="updated" :label="isZh ? '发现时间' : 'date'" width="160" sortable align="right">
          <template #default="{ row }">
            <span class="mono-cell">{{ row.updated }}</span>
          </template>
        </el-table-column>

        <template #empty>
          <EmptyState
            variant="inline"
            icon="🖥️"
            :title="isZh ? '暂无探测到的应用程序' : 'No Discovered Applications'"
            :desc="isZh ? '通过 System Profiler、浏览器探针或资产发现获取的应用与插件将汇总展示在此。' : 'Applications detected via System Profiler or browser probes will appear here.'"
          />
        </template>
      </el-table>
    </div>

    <!-- Bottom Action Bar (CS Native Button Row) -->
    <div class="av-bottom-bar">
      <button class="cs-btn cs-btn-primary" :disabled="!selectedRow" @click="handleExportSelected">
        {{ isZh ? '导出' : 'Export' }}
      </button>
      <button class="cs-btn cs-btn-danger" :disabled="!selectedRow" @click="handleDeleteSelected">
        {{ isZh ? '删除' : 'Delete' }}
      </button>
      <div class="av-bottom-spacer" />
      <button class="cs-btn" @click="refresh">
        {{ isZh ? '刷新' : 'Refresh' }}
      </button>
      <button class="cs-btn" @click="showHelp">
        {{ isZh ? '帮助' : 'Help' }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18nStore } from '@/stores/i18n'
import { useClientsStore } from '@/stores/clients'
import EmptyState from '@/components/Common/EmptyState.vue'

interface AppItem {
  id: string
  external: string
  internal: string
  application: string
  version: string
  description: string
  updated: string
}

const i18nStore = useI18nStore()
const clientsStore = useClientsStore()
const isZh = computed(() => i18nStore.isZh)

const searchKey = ref('')
const selectedRow = ref<AppItem | null>(null)
// 示例数据：后端无 applications API，列表为本地演示
const appList = ref<AppItem[]>([
  {
    id: '1',
    external: '192.168.10.10',
    internal: '192.168.10.10',
    application: 'Google Chrome',
    version: '124.0.6367.208',
    description: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36',
    updated: new Date().toLocaleDateString() + ' 10:22:15',
  },
  {
    id: '2',
    external: '192.168.10.15',
    internal: '192.168.10.15',
    application: 'Microsoft Edge',
    version: '124.0.2478.80',
    description: 'Edg/124.0.2478.80 (Windows NT 10.0; Win64; x64)',
    updated: new Date().toLocaleDateString() + ' 11:05:42',
  },
  {
    id: '3',
    external: '192.168.10.22',
    internal: '192.168.10.22',
    application: 'Mozilla Firefox',
    version: '125.0.3',
    description: 'Firefox/125.0 (X11; Ubuntu; Linux x86_64)',
    updated: new Date().toLocaleDateString() + ' 11:40:19',
  },
])

const filteredList = computed(() => {
  if (!searchKey.value.trim()) return appList.value
  const q = searchKey.value.toLowerCase().trim()
  return appList.value.filter(
    (a) =>
      a.external.toLowerCase().includes(q) ||
      a.internal.toLowerCase().includes(q) ||
      a.application.toLowerCase().includes(q) ||
      a.version.toLowerCase().includes(q) ||
      a.description.toLowerCase().includes(q),
  )
})

function onCurrentChange(row: AppItem | null) {
  selectedRow.value = row
}

function refresh() {
  ElMessage.warning(isZh.value ? '演示功能：应用列表为本地示例，无可刷新数据' : 'Demo only: application list is local sample data')

}

function handleExportSelected() {
  if (!selectedRow.value) return
  const blob = new Blob([JSON.stringify(selectedRow.value, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `application_${selectedRow.value.application}_${Date.now()}.json`
  a.click()
  URL.revokeObjectURL(url)
  ElMessage.success(isZh.value ? '应用信息已导出' : 'Application exported')
}

function handleDeleteSelected() {
  if (!selectedRow.value) return
  ElMessageBox.confirm(
    isZh.value ? `确定移除选中的应用记录 "${selectedRow.value.application}" 吗？` : `Remove "${selectedRow.value.application}"?`,
    isZh.value ? '确认删除' : 'Confirm Delete',
    {
      confirmButtonText: isZh.value ? '删除' : 'Delete',
      cancelButtonText: isZh.value ? '取消' : 'Cancel',
      type: 'warning',
    },
  ).then(() => {
    appList.value = appList.value.filter((a) => a.id !== selectedRow.value?.id)
    selectedRow.value = null
    ElMessage.warning(isZh.value ? '已从本地示例列表移除（演示功能，不持久化）' : 'Removed from local demo list (not persisted)')
  }).catch(() => {})
}

function showHelp() {
  ElMessage.info(isZh.value ? '应用程序列表收集各目标主机的客户端软件和浏览器指纹。' : 'Applications view lists target client-side software fingerprints.')
}
</script>

<style scoped>
.applications-view {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-panel);
  overflow: hidden;
}

.av-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 8px;
  background: var(--cs-bg);
  border-bottom: 1px solid var(--cs-border);
  min-height: 28px;
}

.av-title-group {
  display: flex;
  align-items: baseline;
  gap: 6px;
}

.av-title {
  font-weight: bold;
  font-size: 12px;
  color: var(--cs-text-bright);
}

.av-count {
  font-size: 11px;
  color: var(--cs-text-dim);
}

.av-demo-badge {
  align-self: center;
  padding: 1px 6px;
  border-radius: 2px;
  font-size: 10px;
  font-weight: 600;
  background: color-mix(in srgb, var(--cs-warning) 18%, transparent);
  border: 1px solid color-mix(in srgb, var(--cs-warning) 45%, transparent);
  color: var(--cs-warning);
}

.av-demo-note {
  align-self: center;
  font-size: 11px;
  color: var(--cs-text-dim);
}

.av-spacer {
  flex: 1;
}

.av-search-input {
  width: 220px;
}

.av-btn-icon {
  background: var(--cs-panel);
  border: 1px solid var(--cs-border);
  color: var(--cs-text);
  padding: 3px 6px;
  border-radius: 2px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
}
.av-btn-icon:hover {
  background: var(--cs-hover);
  color: var(--cs-active);
}

.av-table-wrap {
  flex: 1;
  overflow: hidden;
  position: relative;
}

.mono-cell {
  font-family: var(--cs-font-mono);
  font-size: 12px;
  color: var(--cs-text);
}

.app-name {
  font-size: 12px;
  color: var(--cs-active);
}

.desc-cell {
  font-size: 12px;
  color: var(--cs-text-dim);
}

.av-bottom-bar {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 8px;
  background: var(--cs-bg);
  border-top: 1px solid var(--cs-border);
  min-height: 28px;
}
.av-bottom-spacer {
  flex: 1;
}
</style>
