<template>
  <div class="scripts-manager-view cs-font-ui">
    <!-- Hidden file input for Load -->
    <input
      ref="fileInputRef"
      type="file"
      accept=".cna"
      style="display: none"
      @change="onFileSelected"
    />

    <!-- CS Toolbar -->
    <div class="sm-toolbar">
      <div class="sm-title-group">
        <span class="sm-title">{{ isZh ? '脚本管理器' : 'Scripts' }}</span>
        <span class="sm-count">({{ isZh ? `已加载 ${scriptList.length} 个脚本` : `${scriptList.length} loaded` }})</span>
      </div>

      <div class="sm-spacer" />

      <el-input
        v-model="searchKey"
        :placeholder="isZh ? '过滤脚本名称、路径...' : 'Filter script name, path...'"
        size="small"
        clearable
        class="sm-search"
      />

      <button class="cs-btn" @click="handleRefresh">{{ isZh ? '刷新' : 'Refresh' }}</button>
      <button class="cs-btn" @click="handleReloadAll">{{ isZh ? '全部重载' : 'Reload All' }}</button>
    </div>

    <!-- CS Table (Columns: Script | Path | Events) -->
    <div class="sm-table-wrap">
      <el-table
        :data="filteredList"
        size="small"
        height="100%"
        highlight-current-row
        class="cs-session-table"
        @current-change="onCurrentChange"
      >
        <el-table-column prop="name" :label="isZh ? '脚本' : 'Script'" width="240" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="script-name font-bold">{{ row.name }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="path" :label="isZh ? '路径' : 'Path'" min-width="300" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="cs-font-mono path-text">{{ row.path }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="isZh ? '事件' : 'Events'" width="160" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="cs-font-mono evt-text">{{ (row.events || []).join(', ') || '-' }}</span>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <!-- 目录中可用脚本 -->
    <div v-if="availableFiles.length > 0" class="sm-avail">
      <span class="sm-avail-title">{{ isZh ? '脚本目录（点击加载）' : 'Scripts Folder (click to load)' }}</span>
      <div class="sm-avail-list">
        <span
          v-for="f in availableFiles"
          :key="f.name"
          class="sm-avail-item cs-font-mono"
          :title="f.path"
          @click="loadFromFolder(f.name)"
        >
          {{ f.name }}
        </span>
      </div>
    </div>

    <!-- Bottom Buttons: [ Load ] [ Unload ] [ Reload ] -->
    <div class="sm-bottom-bar">
      <button class="cs-btn cs-btn-primary" @click="handleLoad">
        {{ isZh ? '加载' : 'Load' }}
      </button>
      <button class="cs-btn cs-btn-danger" :disabled="!selectedRow" @click="handleUnload">
        {{ isZh ? '卸载' : 'Unload' }}
      </button>
      <button class="cs-btn" :disabled="!selectedRow" @click="handleReload">
        {{ isZh ? '重载' : 'Reload' }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18nStore } from '@/stores/i18n'
import { useScriptConsoleStore } from '@/stores/scriptConsole'
import cnaApi from '@/api/cna'

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)
const scriptConsoleStore = useScriptConsoleStore()

interface ScriptItem {
  id: string
  name: string
  path: string
  events: string[]
  aliases: string[]
}

const fileInputRef = ref<HTMLInputElement | null>(null)
const selectedRow = ref<ScriptItem | null>(null)
const searchKey = ref('')
const scriptList = ref<ScriptItem[]>([])
const availableFiles = ref<{ name: string; path: string }[]>([])
const loading = ref(false)

async function refreshScripts() {
  loading.value = true
  try {
    const [scriptsRes, filesRes] = await Promise.allSettled([
      cnaApi.listScripts(),
      cnaApi.listFiles(),
    ])
    if (scriptsRes.status === 'fulfilled') {
      const data = scriptsRes.value?.data
      const scripts = Array.isArray(data?.data) ? data.data : (Array.isArray(data) ? data : [])
      scriptList.value = scripts.map((s: any) => ({ ...s, id: s.name }))
    }
    if (filesRes.status === 'fulfilled') {
      const data = filesRes.value?.data
      const files = Array.isArray(data?.data) ? data.data : (Array.isArray(data) ? data : [])
      const loaded = new Set(scriptList.value.map((s: any) => s.name))
      availableFiles.value = files.filter((f: any) => !loaded.has(f.name))
    }
  } catch (err) {
    console.error('加载脚本列表失败:', err)
  } finally {
    loading.value = false
  }
}

onMounted(refreshScripts)

const filteredList = computed(() => {
  if (!searchKey.value.trim()) return scriptList.value
  const kw = searchKey.value.toLowerCase().trim()
  return scriptList.value.filter(
    (s) => s.name.toLowerCase().includes(kw) || s.path.toLowerCase().includes(kw),
  )
})

function onCurrentChange(row: any) {
  selectedRow.value = row
}

function handleRefresh() {
  refreshScripts()
  ElMessage.success(isZh.value ? '已刷新' : 'Refreshed')
}

function handleLoad() {
  fileInputRef.value?.click()
}

async function onFileSelected(e: Event) {
  const target = e.target as HTMLInputElement
  if (!target.files || target.files.length === 0) return
  const file = target.files[0]
  try {
    const res = await cnaApi.uploadScript(file)
    if (res?.data?.status === 200) {
      const name = res.data.data?.name || file.name
      await cnaApi.loadScript(name)
      scriptConsoleStore.logScriptLoaded(name, file.name)
      ElMessage.success(isZh.value ? `已加载 ${name}` : `Loaded ${name}`)
      await refreshScripts()
    } else {
      ElMessage.error(res?.data?.message || '上传失败')
    }
  } catch (err: any) {
    scriptConsoleStore.logScriptError(file.name, err?.message || '')
    ElMessage.error(isZh.value ? '加载失败: ' + (err?.message || '') : 'Load failed: ' + (err?.message || ''))
  }
  target.value = ''
}

async function loadFromFolder(name: string) {
  try {
    await cnaApi.loadScript(name)
    scriptConsoleStore.logScriptLoaded(name)
    ElMessage.success(isZh.value ? `已加载 ${name}` : `Loaded ${name}`)
    await refreshScripts()
  } catch (err: any) {
    scriptConsoleStore.logScriptError(name, err?.message || '')
    ElMessage.error(isZh.value ? '加载失败: ' + (err?.message || '') : 'Load failed: ' + (err?.message || ''))
  }
}

function handleUnload() {
  if (!selectedRow.value) return
  const item = selectedRow.value
  ElMessageBox.confirm(
    isZh.value ? `卸载脚本 "${item.name}"？` : `Unload script "${item.name}"?`,
    isZh.value ? '卸载脚本' : 'Unload Script',
    {
      confirmButtonText: 'Unload',
      cancelButtonText: 'Cancel',
      type: 'warning',
    },
  ).then(async () => {
    try {
      await cnaApi.unloadScript(item.name)
      scriptConsoleStore.logScriptUnloaded(item.name)
      ElMessage.success(isZh.value ? `已卸载 ${item.name}` : `Unloaded ${item.name}`)
      selectedRow.value = null
      await refreshScripts()
    } catch (err: any) {
      scriptConsoleStore.logScriptError(item.name, err?.message || '')
      ElMessage.error(isZh.value ? '卸载失败: ' + (err?.message || '') : 'Unload failed: ' + (err?.message || ''))
    }
  }).catch(() => {})
}

function handleReload() {
  if (!selectedRow.value) return
  const item = selectedRow.value
  cnaApi.reloadScript(item.name).then(() => {
    scriptConsoleStore.logScriptReloaded(item.name)
    ElMessage.success(isZh.value ? `已重载 ${item.name}` : `Reloaded ${item.name}`)
    refreshScripts()
  }).catch((err: any) => {
    scriptConsoleStore.logScriptError(item.name, err?.message || '')
    ElMessage.error(isZh.value ? '重载失败: ' + (err?.message || '') : 'Reload failed: ' + (err?.message || ''))
  })
}

function handleReloadAll() {
  const names = scriptList.value.map((s) => s.name)
  if (names.length === 0) {
    ElMessage.warning(isZh.value ? '没有已加载的脚本' : 'No scripts loaded')
    return
  }
  Promise.all(names.map((n) => cnaApi.reloadScript(n).then(() => scriptConsoleStore.logScriptReloaded(n)).catch(() => {}))).then(() => {
    ElMessage.success(isZh.value ? `已重载 ${names.length} 个脚本` : `Reloaded ${names.length} scripts`)
    refreshScripts()
  })
}
</script>

<style scoped>
.scripts-manager-view {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-bg);
  overflow: hidden;
}

.sm-toolbar {
  height: 32px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
  flex-shrink: 0;
}

.sm-title-group {
  display: flex;
  align-items: center;
  gap: 6px;
}

.sm-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.sm-count {
  font-size: 11px;
  color: var(--cs-text-dim);
}

.sm-spacer {
  flex: 1;
}

.sm-search {
  width: 220px;
}

.sm-table-wrap {
  flex: 1;
  overflow: hidden;
}

.script-name {
  color: var(--cs-text-bright);
}

.path-text {
  font-size: 11px;
  color: var(--cs-text);
}

.evt-text {
  font-size: 11px;
  color: var(--cs-accent);
}

.sm-avail {
  flex-shrink: 0;
  max-height: 84px;
  overflow-y: auto;
  border-top: 1px solid var(--cs-border);
  background: var(--cs-panel);
  padding: 6px 10px;
}

.sm-avail-title {
  font-size: 11px;
  color: var(--cs-text-dim);
  display: block;
  margin-bottom: 4px;
}

.sm-avail-list {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.sm-avail-item {
  font-size: 11px;
  padding: 1px 6px;
  border: 1px solid var(--cs-border);
  border-radius: 2px;
  color: var(--cs-text);
  cursor: pointer;
}

.sm-avail-item:hover {
  border-color: var(--cs-accent);
  color: var(--cs-accent);
}

.sm-bottom-bar {
  height: 38px;
  background: var(--cs-panel);
  border-top: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 0 12px;
  flex-shrink: 0;
}
</style>
