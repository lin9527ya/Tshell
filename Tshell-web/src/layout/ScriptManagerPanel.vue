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
        <span class="sm-title">{{ i18nStore.tl('脚本管理器', 'Scripts') }}</span>
        <span class="sm-count">({{ i18nStore.tl(`已加载 ${scriptList.length} 个脚本`, `${scriptList.length} loaded`) }})</span>
      </div>

      <div class="sm-spacer" />

      <el-input
        v-model="searchKey"
        :placeholder="i18nStore.tl('过滤脚本名称、路径...', 'Filter script name, path...')"
        size="small"
        clearable
        class="sm-search"
      />

      <button class="cs-btn cs-btn-primary" @click="handleLoad">
        {{ i18nStore.tl('加载', 'Load') }}
      </button>
      <button class="cs-btn cs-btn-danger" :disabled="!selectedRow" @click="handleUnload">
        {{ i18nStore.tl('卸载', 'Unload') }}
      </button>
      <button class="cs-btn" :disabled="!selectedRow" @click="handleReload">
        {{ i18nStore.tl('重载', 'Reload') }}
      </button>
      <button class="cs-btn" @click="handleRefresh">{{ i18nStore.tl('刷新', 'Refresh') }}</button>
      <button class="cs-btn" @click="handleReloadAll">{{ i18nStore.tl('全部重载', 'Reload All') }}</button>
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
        <el-table-column prop="name" :label="i18nStore.tl('脚本', 'Script')" width="240" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="script-name font-bold">{{ row.name }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="path" :label="i18nStore.tl('路径', 'Path')" min-width="300" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="cs-font-mono path-text">{{ row.path }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="i18nStore.tl('事件', 'Events')" width="160" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="cs-font-mono evt-text">{{ (row.events || []).join(', ') || '-' }}</span>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <!-- CNA 执行面：控制台 + 菜单。服务端 pkg/cna 引擎与
         /cna/{eval,test,alias-exec,menus,menu-exec} 五个接口一直都在，
         此前 UI 只接了脚本文件管理那一半，这半边是零调用。 -->
    <div class="sm-exec">
      <div class="sm-console">
        <div class="sm-exec-head">
          <span class="sm-exec-title">{{ i18nStore.tl('CNA 控制台', 'CNA Console') }}</span>
          <el-select
            v-model="execUid"
            size="small"
            clearable
            filterable
            class="sm-uid"
            :placeholder="i18nStore.tl('目标会话（可选）', 'Target session (optional)')"
          >
            <el-option v-for="c in clients" :key="c.uid" :label="c.label" :value="c.uid" />
          </el-select>
          <div class="sm-spacer" />
          <button class="cs-btn cs-btn-xs" :disabled="!consoleInput.trim()" @click="handleSyntaxCheck">
            {{ i18nStore.tl('语法检查', 'Syntax Check') }}
          </button>
          <button class="cs-btn cs-btn-xs" @click="scriptConsoleStore.clear()">
            {{ i18nStore.tl('清空', 'Clear') }}
          </button>
        </div>
        <div class="sm-console-log">
          <div v-if="scriptConsoleStore.lines.length === 0" class="sm-line sm-line-info">
            {{ i18nStore.tl('（暂无输出）', '(no output yet)') }}
          </div>
          <div
            v-for="(l, i) in scriptConsoleStore.lines"
            :key="i"
            class="sm-line"
            :class="`sm-line-${l.type}`"
          >
            <span class="sm-line-time">{{ l.time }}</span>{{ l.prefix }}{{ l.text }}
          </div>
        </div>
        <div class="sm-console-input">
          <input
            v-model="consoleInput"
            class="sm-input cs-font-mono"
            :placeholder="i18nStore.tl('输入别名命令或脚本片段，回车执行', 'Alias command or script snippet, Enter to run')"
            @keyup.enter="handleRunConsole"
          />
          <button class="cs-btn cs-btn-primary" :disabled="running || !consoleInput.trim()" @click="handleRunConsole">
            {{ running ? '…' : (i18nStore.tl('执行', 'Run')) }}
          </button>
        </div>
      </div>

      <div class="sm-menus">
        <div class="sm-exec-head">
          <span class="sm-exec-title">{{ i18nStore.tl('脚本菜单', 'Script Menus') }}</span>
          <div class="sm-spacer" />
          <button class="cs-btn cs-btn-xs" @click="refreshMenus">{{ i18nStore.tl('刷新', 'Refresh') }}</button>
        </div>
        <div class="sm-menu-list">
          <div v-if="menus.length === 0" class="sm-line sm-line-info">
            {{ i18nStore.tl('（没有已注册的菜单）', '(no menus registered)') }}
          </div>
          <template v-for="m in menus" :key="m.kind">
            <div class="sm-menu-kind">{{ m.kind }}<span class="sm-menu-script">— {{ m.script }}</span></div>
            <template v-for="it in m.items" :key="`${m.kind}:${it.label}`">
              <div v-if="!it.separator" class="sm-menu-item" @click="handleExecMenu(m.kind, [it.label])">
                {{ it.label }}
                <span v-for="sub in it.sub_items || []" :key="sub.label" class="sm-menu-subitem" @click.stop="handleExecMenu(m.kind, [it.label, sub.label])">
                  {{ sub.label }}
                </span>
              </div>
            </template>
          </template>
        </div>
      </div>
    </div>

    <!-- 目录中可用脚本 -->
    <div v-if="availableFiles.length > 0" class="sm-avail">
      <span class="sm-avail-title">{{ i18nStore.tl('脚本目录（点击加载）', 'Scripts Folder (click to load)') }}</span>
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
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18nStore } from '@/stores/i18n'
import { useScriptConsoleStore } from '@/stores/scriptConsole'
import cnaApi from '@/api/cna'
import ClientAPI from '@/api/clients'

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

// ===== CNA 执行面 =====
const clients = ref<{ uid: string; label: string }[]>([])
const execUid = ref('')
const consoleInput = ref('')
const running = ref(false)
const menus = ref<{ script: string; kind: string; items: any[] }[]>([])

async function refreshClients() {
  try {
    const res = await ClientAPI.get_clients_list({ page: 1, page_size: 1000 })
    const list = res?.data?.data?.list || res?.data?.list || []
    clients.value = (list as any[])
      .filter((c) => String(c.Online) === '1')
      .map((c) => ({ uid: c.Uid, label: `${c.Computer || c.Uid} (${c.ExternalIP || '-'})` }))
  } catch (err) {
    console.error('加载在线客户端失败:', err)
  }
}

async function refreshMenus() {
  try {
    const res = await cnaApi.listMenus()
    const data = res?.data?.data
    menus.value = Array.isArray(data) ? data : []
  } catch (err) {
    console.error('加载脚本菜单失败:', err)
  }
}

function appendResult(data: any) {
  const out: string[] = Array.isArray(data?.output) ? data.output : []
  out.forEach((line) => scriptConsoleStore.appendLine(line, 'output'))
  if (data?.error) scriptConsoleStore.appendLine(data.error, 'error', '!')
  if (out.length === 0 && !data?.error) {
    scriptConsoleStore.appendLine(isZh.value ? '（无输出）' : '(no output)', 'info')
  }
}

// 控制台执行：先按 alias 拦截（脚本注册的命令优先，与 C/S 控制台语义一致），
// 未命中再作为脚本片段交给 /cna/eval。两个接口此前都是零调用。
async function handleRunConsole() {
  const input = consoleInput.value.trim()
  if (!input || running.value) return
  running.value = true
  scriptConsoleStore.appendLine(input, 'cmd', '>')
  try {
    const [name, ...args] = input.split(/\s+/)
    const aliasRes = await cnaApi.execAlias({ uid: execUid.value, name, args })
    if (aliasRes?.data?.hit) {
      appendResult(aliasRes.data)
    } else {
      const evalRes = await cnaApi.evalScript(input, execUid.value)
      appendResult(evalRes?.data)
    }
    consoleInput.value = ''
  } catch (err: any) {
    scriptConsoleStore.appendLine(err?.message || String(err), 'error', '!')
  } finally {
    running.value = false
  }
}

async function handleSyntaxCheck() {
  const content = consoleInput.value.trim()
  if (!content) return
  try {
    const res = await cnaApi.testScript(content)
    const data = res?.data
    if (data?.ok) {
      scriptConsoleStore.appendLine(isZh.value ? '[+] 语法检查通过' : '[+] Syntax OK', 'info')
    } else {
      scriptConsoleStore.appendLine(`[!] ${data?.message || (isZh.value ? '语法错误' : 'syntax error')}`, 'error')
    }
  } catch (err: any) {
    scriptConsoleStore.appendLine(err?.message || String(err), 'error', '!')
  }
}

async function handleExecMenu(kind: string, path: string[]) {
  if (!execUid.value) {
    ElMessage.warning(isZh.value ? '请先选择目标会话' : 'Select a target session first')
    return
  }
  scriptConsoleStore.appendLine(`${kind} > ${path.join(' > ')}`, 'cmd', '>')
  try {
    const res = await cnaApi.execMenuItem({ uid: execUid.value, kind, path })
    appendResult(res?.data)
  } catch (err: any) {
    scriptConsoleStore.appendLine(err?.message || String(err), 'error', '!')
  }
}

onMounted(() => {
  refreshScripts()
  refreshClients()
  refreshMenus()
})

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
  background: var(--cs-panel);
  overflow: hidden;
}

/* 工具条：白底 + 1px 分隔线 */
.sm-toolbar {
  flex-shrink: 0;
  min-height: 48px;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  padding: 6px 16px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-card-border);
}

.sm-title-group {
  display: flex;
  align-items: center;
  gap: 8px;
}

.sm-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
  white-space: nowrap;
}

.sm-count {
  font-size: 13px;
  color: var(--cs-text-dim);
  white-space: nowrap;
}

.sm-spacer {
  flex: 1;
}

.sm-search {
  width: 220px;
}

/* CNA 执行面：控制台（左）+ 菜单（右），固定高度不吃表格空间 */
.sm-exec {
  flex-shrink: 0;
  height: 240px;
  display: flex;
  gap: 12px;
  padding: 8px 16px 12px;
  border-top: 1px solid var(--cs-card-border);
  background: var(--cs-panel);
}

.sm-console {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  border: 1px solid var(--cs-card-border);
  border-radius: 4px;
  overflow: hidden;
}

.sm-menus {
  width: 280px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  border: 1px solid var(--cs-card-border);
  border-radius: 4px;
  overflow: hidden;
}

.sm-exec-head {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 8px;
  border-bottom: 1px solid var(--cs-card-border);
  background: var(--cs-bg-subtle, transparent);
}

.sm-exec-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--cs-text-bright);
  white-space: nowrap;
}

.sm-uid {
  width: 200px;
}

.sm-console-log {
  flex: 1;
  overflow-y: auto;
  padding: 6px 8px;
  font-family: var(--cs-font-mono, monospace);
  font-size: 12px;
  line-height: 1.6;
}

.sm-line {
  white-space: pre-wrap;
  word-break: break-all;
  color: var(--cs-text);
}

.sm-line-time {
  color: var(--cs-text-dim);
  margin-right: 6px;
}

.sm-line-cmd { color: var(--cs-active, #0960bd); }
.sm-line-error { color: #d9534f; }
.sm-line-warn { color: #d9a441; }
.sm-line-info { color: var(--cs-text-dim); }

.sm-console-input {
  flex-shrink: 0;
  display: flex;
  gap: 6px;
  padding: 6px 8px;
  border-top: 1px solid var(--cs-card-border);
}

.sm-input {
  flex: 1;
  min-width: 0;
  padding: 4px 8px;
  font-size: 12px;
  color: var(--cs-text);
  background: var(--cs-input-bg, transparent);
  border: 1px solid var(--cs-card-border);
  border-radius: 3px;
  outline: none;
}

.sm-menu-list {
  flex: 1;
  overflow-y: auto;
  padding: 6px 8px;
  font-size: 12px;
}

.sm-menu-kind {
  margin: 6px 0 2px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.sm-menu-script {
  margin-left: 6px;
  font-weight: 400;
  color: var(--cs-text-dim);
}

.sm-menu-item {
  padding: 2px 6px;
  border-radius: 3px;
  cursor: pointer;
  color: var(--cs-text);
}

.sm-menu-item:hover {
  background: var(--cs-hover, rgba(128, 128, 128, 0.12));
}

.sm-menu-subitem {
  margin-left: 8px;
  color: var(--cs-text-dim);
}

.sm-menu-subitem:hover {
  color: var(--cs-active, #0960bd);
}

.sm-table-wrap {
  flex: 1;
  overflow: hidden;
  position: relative;
}

.script-name {
  font-weight: 600;
  color: var(--cs-text-bright);
}

.path-text {
  color: var(--cs-text);
}

.evt-text {
  color: var(--cs-text-dim);
}

/* 脚本目录（底部条） */
.sm-avail {
  flex-shrink: 0;
  max-height: 84px;
  overflow-y: auto;
  padding: 8px 16px;
  background: var(--cs-panel);
  border-top: 1px solid var(--cs-card-border);
}

.sm-avail-title {
  display: block;
  margin-bottom: 6px;
  font-size: 13px;
  color: var(--cs-text-dim);
}

.sm-avail-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.sm-avail-item {
  padding: 2px 8px;
  font-size: 13px;
  color: var(--cs-text);
  background: var(--cs-panel);
  border: 1px solid var(--cs-border-strong);
  border-radius: 4px;
  cursor: pointer;
  transition: color 120ms ease, border-color 120ms ease, background 120ms ease;
}

.sm-avail-item:hover {
  color: var(--cs-active);
  border-color: var(--cs-active);
  background: var(--cs-hover);
}
</style>
