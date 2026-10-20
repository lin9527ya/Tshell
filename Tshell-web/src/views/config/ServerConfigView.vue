<template>
  <div class="cs-server-config-view cs-font-ui">
    <!-- 工具条：页面级操作 -->
    <div class="cfg-toolbar">
      <div class="cfg-title-group">
        <span class="cfg-title">{{ i18nStore.tl('服务配置', 'Server Configuration') }}</span>
        <span class="cfg-status" :class="loaded ? 'online' : 'offline'">{{ loaded ? (i18nStore.tl('已加载', 'Loaded')) : (i18nStore.tl('加载中...', 'Loading...')) }}</span>
      </div>

      <div class="cfg-spacer" />

      <button class="cs-btn" @click="handleEnvVars">{{ i18nStore.tl('环境变量', 'Env Vars') }}</button>
      <button class="cs-btn" @click="handleSchema">{{ i18nStore.tl('配置结构', 'Schema') }}</button>
      <button class="cs-btn" @click="handleHelp">{{ i18nStore.tl('帮助', 'Help') }}</button>
      <button class="cs-btn" @click="handleExport">{{ i18nStore.tl('导出', 'Export') }}</button>
      <button class="cs-btn" :disabled="loading" @click="fetchConfig">{{ i18nStore.tl('刷新', 'Refresh') }}</button>
    </div>

    <!-- Master-Detail Body -->
    <div class="cfg-body">
      <!-- Left Section Navigation -->
      <div class="cfg-sidebar">
        <div
          class="cfg-nav-item"
          :class="{ active: selectedSection === 'all' }"
          @click="selectSection('all')"
        >
          <span class="nav-icon">📄</span>
          <span class="nav-label">{{ i18nStore.tl('全部配置', 'All Sections') }}</span>
        </div>
        <div
          v-for="section in sectionNames"
          :key="section"
          class="cfg-nav-item"
          :class="{ active: selectedSection === section }"
          @click="selectSection(section)"
        >
          <span class="nav-icon">{{ sectionIcon(section) }}</span>
          <span class="nav-label">{{ sectionLabel(section) }}</span>
          <span v-if="SUPPORTED_SECTIONS.includes(section)" class="nav-badge">{{ i18nStore.tl('可编辑', 'editable') }}</span>
        </div>
      </div>

      <!-- Right Content Panel -->
      <div class="cfg-content" v-loading="loading">
        <div class="cfg-panel-header">
          <div class="cfg-panel-heading">
            <span class="panel-title">{{ selectedSection === 'all' ? (i18nStore.tl('完整 JSON 配置', 'Full JSON Configuration')) : (i18nStore.tl(`配置段：${sectionLabel(selectedSection)}`, `Section: ${selectedSection}`)) }}</span>
            <span class="panel-hint">
              {{ selectedSection === 'all'
                ? (i18nStore.tl('在线保存当前仅写入 server / security / logging 三个配置段', 'Online save currently only writes server / security / logging sections'))
                : SUPPORTED_SECTIONS.includes(selectedSection)
                  ? (i18nStore.tl('该配置段支持在线保存', 'This section supports online save'))
                  : (i18nStore.tl('该配置段暂不支持在线保存，请修改 config.yaml 后 Reload', 'This section is read-only online. Edit config.yaml and Reload instead.')) }}
            </span>
          </div>

          <div class="cfg-panel-actions">
            <button v-if="selectedSection !== 'all'" class="cs-btn" @click="syncSectionText">{{ i18nStore.tl('重新加载配置段 JSON', 'Reload Section JSON') }}</button>
            <button class="cs-btn" :disabled="reloading" @click="handleReload">
              {{ reloading ? (i18nStore.tl('重载中...', 'Reloading...')) : (i18nStore.tl('重载', 'Reload')) }}
            </button>
            <button class="cs-btn" :disabled="validating" @click="handleValidate">
              {{ validating ? (i18nStore.tl('校验中...', 'Validating...')) : (i18nStore.tl('校验', 'Validate')) }}
            </button>
            <button class="cs-btn cs-btn-primary" :disabled="saving" @click="handleSave">
              {{ saving ? (i18nStore.tl('保存中...', 'Saving...')) : (i18nStore.tl('保存', 'Save')) }}
            </button>
          </div>
        </div>

        <textarea
          v-model="displayText"
          class="cfg-json-editor cs-font-mono"
          spellcheck="false"
        />
      </div>
    </div>

    <!-- Schema Dialog -->
    <el-dialog v-model="schemaDialogVisible" :title="i18nStore.tl('配置结构说明', 'Configuration Schema')" width="640px" append-to-body class="cs-dialog">
      <textarea v-model="schemaText" class="cfg-json-editor cs-font-mono" spellcheck="false" style="height: 360px" />
      <template #footer>
        <div class="cs-dialog-footer">
          <button class="cs-btn" @click="schemaDialogVisible = false">{{ i18nStore.tl('关闭', 'Close') }}</button>
        </div>
      </template>
    </el-dialog>

    <!-- Environment Variables Dialog -->
    <el-dialog v-model="envDialogVisible" :title="i18nStore.tl('环境变量说明', 'Environment Variable Reference')" width="640px" append-to-body class="cs-dialog">
      <div class="env-table-wrap">
        <el-table :data="envRows" height="360px" class="cs-session-table">
          <template #empty>
            <EmptyState variant="inline" :title="i18nStore.tl('暂无环境变量', 'No environment variables')" />
          </template>
          <el-table-column prop="key" :label="i18nStore.tl('环境变量', 'Variable')" min-width="220" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="cs-font-mono">{{ row.key }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="desc" :label="i18nStore.tl('说明', 'Description')" min-width="260" show-overflow-tooltip />
        </el-table>
      </div>
      <template #footer>
        <div class="cs-dialog-footer">
          <button class="cs-btn" @click="envDialogVisible = false">{{ i18nStore.tl('关闭', 'Close') }}</button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18nStore } from '@/stores/i18n'
import {
  getConfig,
  updateConfig,
  reloadConfig,
  validateConfig,
  exportConfig,
  getConfigEnvVars,
  getConfigSchema,
} from '@/api/config'
import EmptyState from '@/components/Common/EmptyState.vue'

// 可在本页单独编辑并保存的段（其余段只读展示，见后端 applyConfigUpdates 白名单）。
// opsec：载荷生成期开关的团队级默认值（P4）。
const SUPPORTED_SECTIONS: string[] = ['server', 'security', 'logging', 'opsec']

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const configObj = ref<Record<string, any>>({})
const configText = ref('')
const sectionText = ref('')
const selectedSection = ref('all')
const loaded = ref(false)
const loading = ref(false)
const saving = ref(false)
const validating = ref(false)
const reloading = ref(false)

const envDialogVisible = ref(false)
const envRows = ref<{ key: string; desc: string }[]>([])

const schemaDialogVisible = ref(false)
const schemaText = ref('')

const sectionNames = computed(() => Object.keys(configObj.value))

const displayText = computed({
  get: () => (selectedSection.value === 'all' ? configText.value : sectionText.value),
  set: (val: string) => {
    if (selectedSection.value === 'all') {
      configText.value = val
    } else {
      sectionText.value = val
    }
  },
})

function sectionIcon(section: string): string {
  const icons: Record<string, string> = {
    application: '🚀',
    server: '🖥️',
    database: '🗄️',
    jwt: '🔐',
    logging: '📋',
    client: '💤',
    security: '🛡️',
    connection_pool: '🔗',
    worker_pool: '⚙️',
    auth: '👤',
    http: '🌐',
    notifications: '📨',
    delivery: '📦',
    https: '🔒',
  }
  return icons[section] || '📄'
}

const SECTION_LABELS_ZH: Record<string, string> = {
  application: '应用配置',
  server: '服务器配置',
  database: '数据库配置',
  jwt: 'JWT 配置',
  logging: '日志配置',
  client: '客户端默认',
  security: '安全配置',
  connection_pool: '连接池配置',
  worker_pool: '工作池配置',
  auth: '认证配置',
  http: 'HTTP 配置',
  notifications: '通知配置',
  delivery: '投递配置',
  https: 'HTTPS 配置',
}

function sectionLabel(section: string): string {
  return isZh.value ? (SECTION_LABELS_ZH[section] || section) : section
}

async function fetchConfig() {
  loading.value = true
  try {
    const res = await getConfig()
    if (res.status === 200 && res.data?.data) {
      configObj.value = res.data.data
    } else if (res.status === 200 && res.data) {
      configObj.value = res.data
    } else {
      configObj.value = {}
    }
    configText.value = JSON.stringify(configObj.value, null, 2)
    loaded.value = true
    syncSectionText()
  } catch (e: any) {
    loaded.value = false
    ElMessage.error(e?.message || (isZh.value ? '获取服务配置失败' : 'Failed to fetch server config'))
  } finally {
    loading.value = false
  }
}

function selectSection(section: string) {
  selectedSection.value = section
  syncSectionText()
}

function syncSectionText() {
  if (selectedSection.value === 'all') {
    sectionText.value = ''
    return
  }
  sectionText.value = JSON.stringify(configObj.value[selectedSection.value] ?? {}, null, 2)
}

function buildCandidateConfig(): Record<string, any> {
  if (selectedSection.value === 'all') {
    return JSON.parse(configText.value || '{}')
  }
  const cfg = JSON.parse(configText.value || '{}')
  cfg[selectedSection.value] = JSON.parse(sectionText.value || '{}')
  return cfg
}

async function handleSave() {
  try {
    const cfg = buildCandidateConfig()
    let payload: Record<string, any>
    if (selectedSection.value === 'all') {
      payload = {
        server: cfg.server,
        security: cfg.security,
        logging: cfg.logging,
      }
    } else if (SUPPORTED_SECTIONS.includes(selectedSection.value)) {
      payload = { [selectedSection.value]: cfg[selectedSection.value] }
    } else {
      ElMessage.warning(isZh.value ? '该配置段暂不支持在线保存，请修改 config.yaml 后 Reload' : 'This section is read-only online. Edit config.yaml and Reload instead.')
      return
    }

    saving.value = true
    const res = await updateConfig(payload)
    if (res.data?.code === 200 || res.data?.status === 200 || res.data?.success) {
      ElMessage.success(res.data?.message || (isZh.value ? '配置保存成功' : 'Config saved'))
      await fetchConfig()
    } else {
      ElMessage.error(res.data?.message || res.data?.data || (isZh.value ? '配置保存失败' : 'Failed to save config'))
    }
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '配置保存失败，请检查 JSON 格式' : 'Failed to save config, check JSON format'))
  } finally {
    saving.value = false
  }
}

async function handleValidate() {
  try {
    validating.value = true
    const cfg = buildCandidateConfig()
    const res = await validateConfig(cfg)
    if (res.data?.code === 200 || res.data?.status === 200 || res.data?.success) {
      ElMessage.success(res.data?.message || (isZh.value ? '配置验证通过' : 'Config validated'))
    } else {
      ElMessage.error(res.data?.message || res.data?.data || (isZh.value ? '配置验证失败' : 'Config validation failed'))
    }
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '配置验证失败，请检查 JSON 格式' : 'Config validation failed, check JSON format'))
  } finally {
    validating.value = false
  }
}

async function handleReload() {
  try {
    reloading.value = true
    const res = await reloadConfig()
    if (res.data?.code === 200 || res.data?.status === 200 || res.data?.success) {
      ElMessage.success(res.data?.message || (isZh.value ? '配置重新加载成功' : 'Config reloaded'))
      await fetchConfig()
    } else {
      ElMessage.error(res.data?.message || res.data?.data || (isZh.value ? '配置重新加载失败' : 'Failed to reload config'))
    }
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '配置重新加载失败' : 'Failed to reload config'))
  } finally {
    reloading.value = false
  }
}

function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}

async function handleExport() {
  try {
    const res = await exportConfig()
    const data = res.data
    if (data instanceof Blob) {
      const type = (res.headers?.['content-type'] || '').toLowerCase()
      const ext = type.includes('yaml') ? 'yaml' : 'json'
      downloadBlob(data, `config.${ext}`)
      ElMessage.success(isZh.value ? '配置已导出' : 'Config exported')
    } else {
      // Fallback: 后端返回 JSON 对象时，手动导出
      const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
      downloadBlob(blob, 'config.json')
      ElMessage.success(isZh.value ? '配置已导出' : 'Config exported')
    }
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '配置导出失败' : 'Failed to export config'))
  }
}

async function handleEnvVars() {
  try {
    const res = await getConfigEnvVars()
    const data = res.data?.data || res.data || {}
    envRows.value = Object.entries(data).map(([key, desc]) => ({ key, desc: String(desc) }))
    envDialogVisible.value = true
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '获取环境变量说明失败' : 'Failed to fetch env vars'))
  }
}

async function handleSchema() {
  try {
    const res = await getConfigSchema()
    const data = res.data?.data || res.data || {}
    schemaText.value = JSON.stringify(data, null, 2)
    schemaDialogVisible.value = true
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '获取配置 Schema 失败' : 'Failed to fetch config schema'))
  }
}

function handleHelp() {
  ElMessage.info(isZh.value ? '服务配置说明：server/security/logging 支持在线保存，其余字段请编辑 config.yaml 后 Reload。' : 'Server Config Manual: server/security/logging support online save; edit config.yaml and Reload for other fields.')
}

onMounted(() => {
  fetchConfig()
})
</script>

<style scoped>
.cs-server-config-view {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-bg);
  overflow: hidden;
}

.cfg-toolbar {
  min-height: 48px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-card-border);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  flex-shrink: 0;
  flex-wrap: wrap;
}

.cfg-title-group {
  display: flex;
  align-items: center;
  gap: 10px;
}

.cfg-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

/* 状态 tag：已加载实心绿 / 加载中灰 */
.cfg-status {
  display: inline-flex;
  align-items: center;
  height: 20px;
  padding: 0 8px;
  border-radius: 4px;
  font-size: 12px;
  line-height: 1;
  white-space: nowrap;
  color: var(--cs-text-dim);
  background: color-mix(in srgb, var(--cs-offline) 18%, transparent);
}

.cfg-status.online {
  color: #ffffff;
  background: var(--cs-online);
}

.cfg-status.offline {
  color: var(--cs-text-dim);
  background: color-mix(in srgb, var(--cs-offline) 18%, transparent);
}

.cfg-spacer {
  flex: 1;
}

.cfg-body {
  flex: 1;
  display: flex;
  min-height: 0;
  padding: 16px;
  gap: 16px;
}

.cfg-sidebar {
  width: 200px;
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  background: var(--cs-panel);
  overflow-y: auto;
  padding: 8px;
  flex-shrink: 0;
}

.cfg-nav-item {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 36px;
  padding: 0 10px;
  margin-bottom: 2px;
  font-size: 14px;
  color: var(--cs-text);
  border-radius: 6px;
  cursor: pointer;
  user-select: none;
}

.cfg-nav-item:hover {
  background: var(--cs-hover);
  color: var(--cs-text-bright);
}

.cfg-nav-item.active {
  background: color-mix(in srgb, var(--cs-active) 10%, transparent);
  color: var(--cs-active);
  font-weight: 500;
}

.nav-icon {
  font-size: 14px;
}

.nav-label {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.nav-badge {
  font-size: 12px;
  padding: 0 6px;
  height: 20px;
  line-height: 20px;
  border-radius: 4px;
  background: color-mix(in srgb, var(--cs-online) 14%, transparent);
  color: var(--cs-online);
}

.cfg-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
  background: var(--cs-panel);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  overflow: hidden;
}

.cfg-panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 16px;
  border-bottom: 1px solid var(--cs-card-border);
  flex-shrink: 0;
  flex-wrap: wrap;
}

.cfg-panel-heading {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.cfg-panel-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
  flex-wrap: wrap;
}

.panel-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.panel-hint {
  font-size: 12px;
  color: var(--cs-text-dim);
}

.cfg-json-editor {
  flex: 1;
  width: auto;
  margin: 12px 16px 16px;
  resize: none;
  background: var(--cs-console);
  color: var(--cs-text);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  padding: 12px;
  font-size: 13px;
  line-height: 1.6;
  white-space: pre;
  overflow: auto;
}

.cfg-json-editor:focus {
  outline: none;
  border-color: var(--cs-active);
}

.env-table-wrap {
  max-height: 380px;
  overflow: hidden;
}

.cs-dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
