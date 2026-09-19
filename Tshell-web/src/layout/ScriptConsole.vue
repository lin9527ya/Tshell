<template>
  <div class="cs-script-console cs-font-mono">
    <div class="sc-toolbar">
      <span class="sc-title">{{ isZh ? '脚本控制台' : 'Script Console' }}</span>
      <button class="sc-btn" @click="clearConsole">{{ isZh ? '清空' : 'Clear' }}</button>
      <button class="sc-btn" @click="loadExample">{{ isZh ? '帮助' : 'Help' }}</button>
      <div class="sc-spacer" />
      <span class="sc-status">● {{ isZh ? '就绪' : 'Ready' }}</span>
    </div>

    <!-- Output Terminal -->
    <div ref="outputRef" class="sc-output">
      <div v-for="(line, idx) in lines" :key="idx" :class="['sc-line', `sc-type-${line.type}`]">
        <span class="sc-time">[{{ line.time }}]</span>
        <span class="sc-prefix">{{ line.prefix }}</span>
        <span class="sc-text">{{ line.text }}</span>
      </div>
      <div v-if="!lines.length" class="sc-empty">
        {{ isZh ? '演示控制台：未接入真实 Aggressor 脚本引擎，仅支持 beacons()/help/clear。' : 'Demo console: no real Aggressor engine attached; only beacons()/help/clear are supported.' }}
      </div>
    </div>

    <!-- Input Bar -->
    <div class="sc-input-bar">
      <span class="sc-prompt">aggressor&gt;</span>
      <input
        ref="inputRef"
        v-model="command"
        class="sc-input"
        :placeholder="isZh ? '输入命令或 Aggressor 脚本片段...' : 'Type command or aggressor script snippet...'"
        spellcheck="false"
        autocomplete="off"
        @keydown.enter="runCommand"
        @keydown.up.prevent="historyUp"
        @keydown.down.prevent="historyDown"
      />
      <button class="sc-run-btn" @click="runCommand">{{ isZh ? '执行' : 'Eval' }}</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useClientsStore } from '@/stores/clients'
import { useScriptConsoleStore } from '@/stores/scriptConsole'
import cnaApi from '@/api/cna'
import { useI18nStore } from '@/stores/i18n'

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const clientsStore = useClientsStore()
const scriptConsoleStore = useScriptConsoleStore()

const command = ref('')
const lines = computed(() => scriptConsoleStore.lines)
const history = ref<string[]>([])
const historyIdx = ref(-1)
const outputRef = ref<HTMLElement | null>(null)
const inputRef = ref<HTMLInputElement | null>(null)

watch(
  () => lines.value.length,
  () => {
    nextTick(() => {
      if (outputRef.value) {
        outputRef.value.scrollTop = outputRef.value.scrollHeight
      }
    })
  },
)

function appendLine(text: string, type: 'info' | 'output' | 'error' | 'warn' | 'cmd' = 'output', prefix = '') {
  scriptConsoleStore.appendLine(text, type, prefix)
}

function clearConsole() {
  scriptConsoleStore.clear()
}

function loadExample() {
  appendLine('Available script commands:', 'info')
  appendLine('  beacons()               - List all active beacon sessions', 'info')
  appendLine('  listeners()             - List active listeners', 'info')
  appendLine('  binput(bid, command)    - Send command to beacon', 'info')
  appendLine('  bsleep(bid, sec, jit)   - Set sleep on beacon', 'info')
  appendLine('  clear                   - Clear script console', 'info')
  appendLine('  help                    - Display this help message', 'info')
}

function runCommand() {
  const cmd = command.value.trim()
  if (!cmd) return

  history.value.push(cmd)
  historyIdx.value = history.value.length
  appendLine(cmd, 'cmd', '>')
  command.value = ''

  const lower = cmd.toLowerCase()
  if (lower === 'clear') {
    clearConsole()
    return
  }
  if (lower === 'help') {
    loadExample()
    return
  }
  if (lower === 'beacons()' || lower === 'beacons') {
    const list = clientsStore.clientsList
    appendLine(`Total Beacons: ${list.length}`, 'info')
    list.forEach((c) => {
      appendLine(`  [${c.Uid.slice(0, 8)}] ${c.Username}@${c.Computer} (${c.ExternalIP || c.InternalIP}) - Online: ${c.Online === '1' ? 'YES' : 'NO'}`, 'output')
    })
    return
  }

  // 真实引擎求值：发送到服务端 CNA 引擎执行
  evalRemote(cmd)
}

async function evalRemote(content: string) {
  appendLine('[*] Eval...', 'info')
  try {
    const res = await cnaApi.evalScript(content, '')
    const data = res?.data
    if (data?.error) {
      appendLine(`[!] ${data.error}`, 'error')
      return
    }
    const output = data?.output || []
    if (output.length === 0) {
      appendLine('(no output)', 'warn')
    }
    for (const line of output) {
      appendLine(line, 'output')
    }
  } catch (err: any) {
    appendLine(`[!] Eval failed: ${err?.message || err}`, 'error')
  }
}

function historyUp() {
  if (history.value.length === 0) return
  if (historyIdx.value > 0) {
    historyIdx.value--
    command.value = history.value[historyIdx.value]
  }
}

function historyDown() {
  if (historyIdx.value < history.value.length - 1) {
    historyIdx.value++
    command.value = history.value[historyIdx.value]
  } else {
    historyIdx.value = history.value.length
    command.value = ''
  }
}

onMounted(() => {
  // 演示控制台：不再输出伪造的 artifact_kit 加载日志
  appendLine(isZh.value ? 'Aggressor 控制台已连接真实脚本引擎，输入脚本表达式或 help 查看支持的命令。' : 'Aggressor console connected to real script engine. Type an expression or help.', 'info', '[*]')
})
</script>

<style scoped>
.cs-script-console {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-console);
  color: var(--cs-text);
  overflow: hidden;
}
.sc-toolbar {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 8px;
  background: var(--cs-titlebar-bg);
  border-bottom: 1px solid var(--cs-border);
  flex-shrink: 0;
  user-select: none;
}
.sc-title {
  font-size: 11px;
  font-weight: 600;
  color: var(--cs-text-dim);
  text-transform: uppercase;
  letter-spacing: .04em;
}
.sc-spacer { flex: 1; }
.sc-status {
  font-size: 11px;
  color: var(--cs-online);
}
.sc-btn {
  background: transparent;
  border: 1px solid var(--cs-border);
  border-radius: 2px;
  color: var(--cs-text);
  font-size: 11px;
  padding: 1px 8px;
  cursor: pointer;
}
.sc-btn:hover {
  background: var(--cs-hover);
  color: var(--cs-text-bright);
}
.sc-output {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 8px 12px;
  font-size: 12px;
  line-height: 1.5;
  background: var(--cs-console);
}
.sc-line {
  white-space: pre-wrap;
  word-break: break-all;
}
.sc-time {
  color: var(--cs-text-dim);
  margin-right: 6px;
}
.sc-prefix {
  color: var(--cs-online);
  font-weight: bold;
}
.sc-type-info { color: var(--cs-active); }
.sc-type-output { color: var(--cs-text); }
.sc-type-error { color: var(--cs-error); }
.sc-type-warn { color: var(--cs-warning); }
.sc-type-cmd { color: var(--cs-active); font-weight: 600; }
.sc-empty {
  color: var(--cs-text-dim);
  text-align: center;
  margin-top: 40px;
  font-size: 12px;
}
.sc-input-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 32px;
  padding: 0 10px;
  background: var(--cs-panel);
  border-top: 1px solid var(--cs-border);
  flex-shrink: 0;
}
.sc-prompt {
  color: var(--cs-active);
  font-size: 12px;
  font-weight: 600;
}
.sc-input {
  flex: 1;
  background: transparent;
  border: 0;
  outline: 0;
  color: var(--cs-text-bright);
  font-family: inherit;
  font-size: 12px;
}
.sc-input::placeholder {
  color: var(--cs-text-dim);
}
.sc-run-btn {
  background: var(--cs-active);
  border: 0;
  border-radius: 2px;
  color: #fff;
  font-size: 11px;
  padding: 3px 10px;
  cursor: pointer;
}
.sc-run-btn:hover {
  background: var(--cs-active-hover);
}
</style>
