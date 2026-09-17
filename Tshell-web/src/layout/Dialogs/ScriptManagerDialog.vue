<template>
  <el-dialog
    v-model="visible"
    :title="isZh ? '脚本管理器' : 'Scripts'"
    width="760px"
    :close-on-click-modal="false"
    append-to-body
    class="cs-dialog"
  >
    <div class="sm-container cs-font-ui">
      <!-- Hidden file input for Load -->
      <input
        ref="fileInputRef"
        type="file"
        accept=".cna"
        style="display: none"
        @change="onFileSelected"
      />

      <!-- Scripts Table (CS Columns: Script | Path | Events) -->
      <div class="sm-table-wrap">
        <el-table
          :data="scriptList"
          size="small"
          height="280px"
          highlight-current-row
          class="cs-session-table"
          @current-change="onCurrentChange"
        >
          <el-table-column prop="name" :label="isZh ? '脚本' : 'Script'" width="220" sortable show-overflow-tooltip>
            <template #default="{ row }">
              <span class="script-name font-bold">{{ row.name }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="path" :label="isZh ? '路径' : 'Path'" min-width="280" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="cs-font-mono path-text">{{ row.path }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="isZh ? '事件' : 'Events'" width="140" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="cs-font-mono evt-text">{{ (row.events || []).join(', ') || '-' }}</span>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <!-- 目录中可用脚本（未加载） -->
      <div v-if="fileList.length > 0" class="sm-avail">
        <span class="sm-avail-title">{{ isZh ? '脚本目录' : 'Scripts Folder' }}</span>
        <div class="sm-avail-list">
          <span
            v-for="f in fileList"
            :key="f.name"
            class="sm-avail-item cs-font-mono"
            :title="f.path"
            @click="loadFromFolder(f.name)"
          >
            {{ f.name }}
          </span>
        </div>
      </div>
    </div>

    <!-- Centered Bottom Buttons (CS: [ Load ] [ Unload ] [ Reload ] [ Help ]) -->
    <template #footer>
      <div class="cs-dialog-footer justify-center">
        <button class="cs-btn cs-btn-primary" @click="handleLoad">
          {{ isZh ? '加载' : 'Load' }}
        </button>
        <button class="cs-btn cs-btn-danger" :disabled="!selectedRow" @click="handleUnload">
          {{ isZh ? '卸载' : 'Unload' }}
        </button>
        <button class="cs-btn" :disabled="!selectedRow" @click="handleReload">
          {{ isZh ? '重新加载' : 'Reload' }}
        </button>
        <button class="cs-btn" @click="refreshScripts">
          {{ isZh ? '刷新' : 'Refresh' }}
        </button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useWorkspaceStore } from '@/stores/workspace'
import { useI18nStore } from '@/stores/i18n'
import { useScriptConsoleStore } from '@/stores/scriptConsole'
import cnaApi from '@/api/cna'

const workspace = useWorkspaceStore()
const i18nStore = useI18nStore()
const scriptConsoleStore = useScriptConsoleStore()
const isZh = computed(() => i18nStore.isZh)

const visible = computed({
  get: () => workspace.dialogs.scriptManager,
  set: (v) => {
    if (!v) workspace.closeDialog('scriptManager')
  },
})

const loading = ref(false)
const scriptList = ref<any[]>([])
const fileList = ref<any[]>([])
const selectedRow = ref<any | null>(null)
const fileInputRef = ref<HTMLInputElement | null>(null)

watch(visible, (v) => {
  if (v) refreshScripts()
})

async function refreshScripts() {
  loading.value = true
  try {
    const [scriptsRes, filesRes] = await Promise.allSettled([
      cnaApi.listScripts(),
      cnaApi.listFiles(),
    ])
    if (scriptsRes.status === 'fulfilled') {
      const data = scriptsRes.value?.data
      scriptList.value = Array.isArray(data?.data) ? data.data : (Array.isArray(data) ? data : [])
    }
    if (filesRes.status === 'fulfilled') {
      const data = filesRes.value?.data
      const loaded = new Set(scriptList.value.map((s: any) => s.name))
      const allFiles = Array.isArray(data?.data) ? data.data : (Array.isArray(data) ? data : [])
      fileList.value = allFiles.filter((f: any) => !loaded.has(f.name))
    }
  } catch (err) {
    console.error('加载脚本列表失败:', err)
  } finally {
    loading.value = false
  }
}

function onCurrentChange(row: any) {
  selectedRow.value = row
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
</script>

<style scoped>
.sm-container {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.sm-table-wrap {
  border: 1px solid var(--cs-border);
  background: var(--cs-bg);
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
  border: 1px solid var(--cs-border);
  background: var(--cs-bg);
  padding: 6px 8px;
  max-height: 90px;
  overflow-y: auto;
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

.justify-center {
  display: flex;
  justify-content: center;
  gap: 8px;
}
</style>
