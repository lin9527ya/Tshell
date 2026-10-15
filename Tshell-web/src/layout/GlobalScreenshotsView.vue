<template>
  <div class="global-screenshots cs-font-ui" @click="closeContextMenu">
    <!-- Toolbar -->
    <div class="gs-toolbar">
      <div class="gs-title-group">
        <el-icon class="gs-title-icon"><Picture /></el-icon>
        <span class="gs-title">{{ isZh ? (isClientScoped ? `屏幕截图: ${targetClientTitle}` : '屏幕截图管理') : (isClientScoped ? `Screenshots: ${targetClientTitle}` : 'Screenshots') }}</span>
        <span class="gs-count">
          ({{ isClientScoped && scopeFilter === 'current' ? `${filteredScreenshots.length} ${isZh ? '张当前会话' : 'current'}` : `${filteredScreenshots.length} ${isZh ? '张截图' : 'captures'}` }})
        </span>
      </div>

      <!-- Scope Filter Toggle (When in client-scoped mode) -->
      <div v-if="isClientScoped" class="gs-scope-toggle">
        <button
          class="gs-scope-btn"
          :class="{ active: scopeFilter === 'current' }"
          @click="scopeFilter = 'current'"
        >
          {{ isZh ? '当前会话' : 'Current' }}
        </button>
        <button
          class="gs-scope-btn"
          :class="{ active: scopeFilter === 'all' }"
          @click="scopeFilter = 'all'"
        >
          {{ isZh ? '全部会话' : 'All' }}
        </button>
      </div>

      <div class="gs-spacer" />

      <!-- View Mode Buttons -->
      <div class="gs-btn-group">
        <button
          class="gs-btn"
          :class="{ active: viewMode === 'split' }"
          :title="isZh ? '分屏视图' : 'Split View'"
          @click="viewMode = 'split'"
        >
          {{ isZh ? '分屏' : 'Split' }}
        </button>
        <button
          class="gs-btn"
          :class="{ active: viewMode === 'table' }"
          :title="isZh ? '表格视图' : 'Table View'"
          @click="viewMode = 'table'"
        >
          {{ isZh ? '表格' : 'Table' }}
        </button>
        <button
          class="gs-btn"
          :class="{ active: viewMode === 'grid' }"
          :title="isZh ? '网格视图' : 'Grid View'"
          @click="viewMode = 'grid'"
        >
          {{ isZh ? '网格' : 'Grid' }}
        </button>
      </div>

      <!-- Target Client Selector (When in global mode or all-scope) -->
      <el-select
        v-if="!isClientScoped || scopeFilter === 'all'"
        v-model="selectedTargetUid"
        size="small"
        clearable
        style="width: 220px"
        :placeholder="isZh ? '选择截图目标客户端' : 'Select target beacon'"
      >
        <el-option
          v-for="c in onlineClients"
          :key="c.Uid"
          :value="c.Uid"
          :label="`${c.Username || 'user'}@${c.Computer || c.ExternalIP || 'host'}`"
        />
      </el-select>

      <!-- Quality Selector -->
      <div class="gs-quality-group">
        <span class="gs-label">{{ isZh ? '画质:' : 'Quality:' }}</span>
        <el-select v-model="screenshotQuality" size="small" style="width: 100px">
          <el-option :value="0" :label="isZh ? '低画质' : 'Low'" />
          <el-option :value="1" :label="isZh ? '中画质' : 'Medium'" />
          <el-option :value="2" :label="isZh ? '高画质' : 'High'" />
        </el-select>
      </div>

      <!-- Search Input -->
      <el-input
        v-model="searchKey"
        :placeholder="isZh ? '过滤用户/计算机/IP/标题...' : 'Filter user, computer, title...'"
        size="small"
        clearable
        class="gs-search-input"
      />

      <!-- Capture Button in Toolbar -->
      <button class="cs-btn cs-btn-primary" :disabled="isCapturing || !effectiveTargetUid" @click="handleCapture">
        <el-icon :class="{ 'is-loading': isCapturing }"><Camera /></el-icon>
        <span>{{ isCapturing ? (isZh ? '抓取中...' : 'Capturing...') : (isZh ? '屏幕抓取' : 'Capture') }}</span>
      </button>

      <!-- Item Actions (选中项) -->
      <button class="cs-btn" :disabled="!selectedItem" @click="openFullscreen(selectedItem)">
        {{ isZh ? '全屏查看' : 'Fullscreen' }}
      </button>
      <button class="cs-btn" :disabled="!selectedItem" @click="downloadItem(selectedItem)">
        {{ isZh ? '保存/下载' : 'Save' }}
      </button>
      <button class="cs-btn cs-btn-danger" :disabled="!selectedItem" @click="deleteItem(selectedItem)">
        {{ isZh ? '删除' : 'Delete' }}
      </button>
      <button class="cs-btn" :disabled="!filteredScreenshots.length" @click="handleClear">
        {{ isZh ? '清空列表' : 'Clear' }}
      </button>
      <button class="cs-btn" @click="showHelp">
        {{ isZh ? '帮助' : 'Help' }}
      </button>

      <!-- Refresh Button -->
      <button class="gs-btn-icon" :title="isZh ? '刷新截图' : 'Refresh Screenshots'" @click="refreshAll">
        <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2">
          <polyline points="23 4 23 10 17 10" />
          <path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10" />
        </svg>
      </button>

      <span class="gs-status-info">
        {{ isZh ? `当前目标: ${currentTargetDisplay} | 共 ${filteredScreenshots.length} 张截图` : `Target: ${currentTargetDisplay} | ${filteredScreenshots.length} captures` }}
      </span>
    </div>

    <!-- Body Surface (Split / Table / Grid View) -->
    <div class="gs-body" :class="`mode-${viewMode}`">
      <!-- Table Pane (Split / Table Mode) : v-if 而非 v-show——网格模式下整表卸载,
           避免 100 行 base64 图片行的 DOM 常驻(el-table v-show 仍全量渲染) -->
      <div v-if="viewMode === 'split' || viewMode === 'table'" class="gs-table-pane">
        <el-table
          :data="filteredScreenshots"
          size="small"
          height="100%"
          highlight-current-row
          :row-style="getRowStyle"
          class="cs-session-table"
          @current-change="selectItem"
          @row-click="selectItem"
          @row-dblclick="openFullscreen"
          @row-contextmenu="onContextMenu"
        >
          <el-table-column prop="user" :label="isZh ? '用户' : 'user'" width="140" sortable show-overflow-tooltip>
            <template #default="{ row }">
              <span class="user-cell">{{ row.user || 'SYSTEM *' }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="computer" :label="isZh ? '计算机' : 'computer'" width="150" sortable show-overflow-tooltip>
            <template #default="{ row }">
              <span class="host-cell font-bold">{{ row.computer || row.host }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="session" :label="isZh ? '会话' : 'session'" width="80" sortable align="center">
            <template #default="{ row }">
              <span class="mono-cell">{{ row.session || '1' }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="time" :label="isZh ? '截取时间' : 'when'" width="175" sortable show-overflow-tooltip>
            <template #default="{ row }">
              <span class="mono-cell">{{ row.time }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="qualityLabel" :label="isZh ? '画质' : 'quality'" width="95" align="center" sortable>
            <template #default="{ row }">
              <span class="quality-badge" :class="`q-${row.quality ?? 1}`">{{ row.qualityLabel || '中画质' }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="title" :label="isZh ? '窗口标题 / 描述' : 'title'" min-width="200" sortable show-overflow-tooltip>
            <template #default="{ row }">
              <span class="title-cell">{{ row.title || 'BladeShell' }}</span>
            </template>
          </el-table-column>

          <template #empty>
            <EmptyState
              icon="📷"
              variant="inline"
              :title="isZh ? '暂无屏幕截图' : 'No Screenshots'"
              :desc="isZh ? (effectiveTargetUid ? '点击右上角【屏幕抓取】即可捕获当前被控端画面。' : '请先选择在线客户端，然后点击【屏幕抓取】捕获远程画面。') : 'Click [ Capture ] to capture the screen.'"
            />
          </template>
        </el-table>
      </div>

      <!-- Preview Pane (Split Mode / Active Item) -->
      <div v-show="viewMode === 'split'" class="gs-preview-pane">
        <div class="gs-preview-header">
          <span class="gs-preview-title">{{ selectedItem?.title || (selectedItem ? `BladeShell - [ ${selectedItem.computer || selectedItem.host} ]` : (isZh ? '画面预览' : 'Preview')) }}</span>
          <div class="gs-preview-tools">
            <button class="cs-btn cs-btn-xs" :disabled="!selectedItem" @click="zoomReset">{{ isZh ? '1:1' : '1:1' }}</button>
            <button class="cs-btn cs-btn-xs" :disabled="!selectedItem" @click="openFullscreen(selectedItem)">{{ isZh ? '全屏' : 'Fullscreen' }}</button>
            <button class="cs-btn cs-btn-xs cs-btn-primary" :disabled="!selectedItem" @click="downloadItem(selectedItem)">{{ isZh ? '保存' : 'Save' }}</button>
          </div>
        </div>

        <div class="gs-preview-content">
          <div v-if="selectedItem?.dataUrl" class="gs-preview-img-wrap" @wheel.prevent="onWheelZoom" @dblclick="openFullscreen(selectedItem)">
            <img :src="selectedItem.dataUrl" alt="Screenshot" class="gs-preview-img" :style="{ transform: `scale(${zoomLevel})` }" />

            <!-- Floating Zoom Controls -->
            <div class="sc-floating-controls">
              <button class="cs-btn cs-btn-xs" @click.stop="zoomOut">－</button>
              <span class="zoom-text">{{ Math.round(zoomLevel * 100) }}%</span>
              <button class="cs-btn cs-btn-xs" @click.stop="zoomIn">＋</button>
              <button class="cs-btn cs-btn-xs" @click.stop="zoomReset">1:1</button>
            </div>
          </div>
          <div v-else class="gs-preview-placeholder">
            <div class="placeholder-text">{{ isZh ? '请在左侧列表中选择截图以预览，或点击【屏幕抓取】获取新截图' : 'Select a screenshot to preview or click [ Capture ]' }}</div>
          </div>
        </div>
      </div>

      <!-- Grid Gallery Mode -->
      <div v-if="viewMode === 'grid'" class="gs-grid-wrap">
        <div
          v-for="(item, idx) in filteredScreenshots"
          :key="idx"
          class="gs-card"
          :class="{ selected: selectedItem?.id === item.id }"
          @click="selectItem(item)"
          @dblclick="openFullscreen(item)"
          @contextmenu="onContextMenu(item, null, $event)"
        >
          <div class="gs-card-img-wrap">
            <img :src="item.dataUrl" alt="Screenshot" class="gs-card-img" />
            <div class="gs-card-overlay">
              <button class="cs-btn cs-btn-xs" @click.stop="openFullscreen(item)">{{ isZh ? '全屏' : 'Fullscreen' }}</button>
              <button class="cs-btn cs-btn-xs cs-btn-primary" @click.stop="downloadItem(item)">{{ isZh ? '保存' : 'Save' }}</button>
            </div>
          </div>
          <div class="gs-card-footer">
            <div class="gs-card-host" :title="item.title || item.host">{{ item.computer || item.host }}</div>
            <div class="gs-card-time">{{ item.time }}</div>
          </div>
        </div>

        <EmptyState
          v-if="!filteredScreenshots.length"
          icon="📷"
          variant="overlay"
          :title="isZh ? '暂无屏幕截图' : 'No Screenshots'"
        />
      </div>
    </div>

    <!-- Right-Click Context Menu -->
    <Teleport to="body">
      <div
        v-if="ctxMenu.visible"
        class="cs-listener-ctx-menu cs-font-ui"
        :style="{ left: ctxMenu.x + 'px', top: ctxMenu.y + 'px' }"
        @mousedown.stop
        @contextmenu.prevent
      >
        <div class="ctx-item is-highlight" @click="handleInteract(ctxMenu.row)">
          <span class="ctx-text">{{ isZh ? '会话交互(I)' : 'Interact (I)' }}</span>
        </div>
        <div class="ctx-item" @click="recaptureTarget(ctxMenu.row)">
          <span class="ctx-text">{{ isZh ? '重新截图(C)' : 'Recapture (C)' }}</span>
        </div>
        <div
          class="ctx-item has-sub"
          @mouseenter="ctxMenu.showColor = true"
          @mouseleave="ctxMenu.showColor = false"
        >
          <span class="ctx-text">{{ isZh ? '颜色标记(C)' : 'Color (C)' }}</span>
          <span class="ctx-arrow">▶</span>

          <div v-show="ctxMenu.showColor" class="ctx-color-palette">
            <div
              v-for="c in colorPalette"
              :key="c.value"
              class="color-box"
              :style="{ backgroundColor: c.value }"
              :title="isZh ? c.zh : c.en"
              @click.stop="setColor(c.value)"
            />
          </div>
        </div>
        <div class="ctx-divider" />
        <div class="ctx-item" @click="downloadItem(ctxMenu.row)">
          <span class="ctx-text">{{ isZh ? '保存(S)' : 'Save (S)' }}</span>
        </div>
        <div class="ctx-item ctx-danger" @click="deleteItem(ctxMenu.row)">
          <span class="ctx-text">{{ isZh ? '删除(R)' : 'Remove (R)' }}</span>
        </div>
      </div>
    </Teleport>

    <!-- Fullscreen Modal Viewer -->
    <el-dialog
      v-model="fullscreenVisible"
      :title="fullscreenTitle"
      width="92%"
      top="3vh"
      append-to-body
      class="cs-dialog gs-fs-dialog"
      :close-on-click-modal="false"
    >
      <div v-if="selectedItem" class="gs-fs-body" @wheel.prevent="onWheelFsZoom">
        <div class="gs-fs-img-wrap">
          <img
            :src="selectedItem.dataUrl"
            alt="Fullscreen capture"
            class="gs-fs-img"
            :style="{ transform: `scale(${fsZoomFactor})` }"
          />
        </div>
        <div class="gs-fs-controls">
          <button class="cs-btn cs-btn-xs" @click="fsZoomFactor = Math.max(0.2, fsZoomFactor - 0.2)">－</button>
          <span class="zoom-label">{{ Math.round(fsZoomFactor * 100) }}%</span>
          <button class="cs-btn cs-btn-xs" @click="fsZoomFactor = Math.min(4, fsZoomFactor + 0.2)">＋</button>
          <button class="cs-btn cs-btn-xs" @click="fsZoomFactor = 1">1:1</button>
          <button class="cs-btn cs-btn-xs cs-btn-primary" @click="downloadItem(selectedItem)">{{ isZh ? '下载图片' : 'Download' }}</button>
        </div>
      </div>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import type { ComputedRef, Ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Picture, Camera } from '@element-plus/icons-vue'
import { useClientsStore } from '@/stores/clients'
import { useI18nStore } from '@/stores/i18n'
import { useSessionSelectionStore } from '@/stores/sessionSelection'
import { useScreenshotsStore } from '@/stores/screenshots'
import type { ScreenshotItem } from '@/stores/screenshots'
import EmptyState from '@/components/Common/EmptyState.vue'
import { SESSION_MARKER_COLORS, rowTintStyle } from '@/utils/sessionColors'

const router = useRouter()
const props = withDefaults(
  defineProps<{
    uid?: string
  }>(),
  {
    uid: '',
  },
)

const clientsStore = useClientsStore()
const selection = useSessionSelectionStore()
const i18nStore = useI18nStore()
const screenshotsStore = useScreenshotsStore()

const isZh = computed(() => i18nStore.isZh)
const isCapturing = computed(() => screenshotsStore.isCapturing)

// Scope identification: from props, injection or global
const injectedUid = inject<Ref<string> | ComputedRef<string> | null>('realUid', null)
const resolvedUid = computed(() => {
  if (props.uid && props.uid.length > 5) return props.uid
  if (injectedUid && (injectedUid as any).value && String((injectedUid as any).value).length > 5) {
    return String((injectedUid as any).value)
  }
  return ''
})

const isClientScoped = computed(() => !!resolvedUid.value)
const scopeFilter = ref<'current' | 'all'>('current')

const targetClient = computed(() => {
  if (resolvedUid.value) {
    return clientsStore.getClientByUid(resolvedUid.value)
  }
  return null
})

const targetClientTitle = computed(() => {
  const c = targetClient.value
  if (!c) return resolvedUid.value.slice(0, 8) || (isZh.value ? '未指定客户端' : 'No Client')
  return `${c.Username || 'user'}@${c.Computer || c.ExternalIP || 'host'} (${c.InternalIP || c.ExternalIP || '127.0.0.1'})`
})

const viewMode = ref<'split' | 'table' | 'grid'>('split')
const searchKey = ref('')
const selectedItem = ref<ScreenshotItem | null>(null)
const fullscreenVisible = ref(false)
const zoomLevel = ref(1)
const fsZoomFactor = ref(1)
const screenshotQuality = ref(1)

const selectedTargetUid = ref('')
const onlineClients = computed(() =>
  // Online 为后端字符串标记("1" 在线),无 boolean 形态;原 === true 分支恒 false
  clientsStore.clientsList.filter((c) => c.Online === '1' || c.Online === 'true'),
)

// Active target for taking screenshots
const effectiveTargetUid = computed(() => {
  if (isClientScoped.value && scopeFilter.value === 'current') {
    return resolvedUid.value
  }
  if (selectedTargetUid.value) {
    return selectedTargetUid.value
  }
  if (resolvedUid.value) {
    return resolvedUid.value
  }
  if (selection.primarySelectedUid) {
    return selection.primarySelectedUid
  }
  return onlineClients.value.length ? onlineClients.value[0].Uid : ''
})

const currentTargetDisplay = computed(() => {
  const uid = effectiveTargetUid.value
  if (!uid) return isZh.value ? '未选择目标' : 'None'
  const c = clientsStore.getClientByUid(uid)
  return c ? `${c.Username || 'user'}@${c.Computer || c.ExternalIP || 'host'}` : uid.slice(0, 8)
})

const ctxMenu = ref({
  visible: false,
  x: 0,
  y: 0,
  row: null as ScreenshotItem | null,
  showColor: false,
})

const colorPalette = SESSION_MARKER_COLORS

const filteredScreenshots = computed(() => {
  let list = screenshotsStore.screenshots
  if (isClientScoped.value && scopeFilter.value === 'current') {
    list = list.filter((s) => s.uid === resolvedUid.value)
  }
  if (!searchKey.value.trim()) return list
  const q = searchKey.value.toLowerCase().trim()
  return list.filter(
    (s) =>
      s.user.toLowerCase().includes(q) ||
      s.computer.toLowerCase().includes(q) ||
      s.title.toLowerCase().includes(q) ||
      s.host.toLowerCase().includes(q) ||
      String(s.session).includes(q),
  )
})

watch(
  () => filteredScreenshots.value,
  (list) => {
    if (list.length && (!selectedItem.value || !list.some((s) => s.id === selectedItem.value?.id))) {
      selectedItem.value = list[0]
    } else if (!list.length) {
      selectedItem.value = null
    }
  },
  { immediate: true },
)

const fullscreenTitle = computed(() => {
  if (!selectedItem.value) return 'Screenshot Viewer'
  return `Screenshot: ${selectedItem.value.title || selectedItem.value.host} (${selectedItem.value.time})`
})

function getRowStyle({ row }: { row: ScreenshotItem }) {
  return rowTintStyle(row.color, { alpha: '30', borderWidth: 3 })
}

function selectItem(item: ScreenshotItem | null) {
  if (item) {
    selectedItem.value = item
    zoomLevel.value = 1
  }
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
  if (!selectedItem.value) return
  if (e.deltaY < 0) zoomIn()
  else zoomOut()
}

function onWheelFsZoom(e: WheelEvent) {
  const factor = e.deltaY < 0 ? 1.15 : 0.85
  fsZoomFactor.value = Math.max(0.2, Math.min(4, fsZoomFactor.value * factor))
}

function onContextMenu(row: ScreenshotItem, col: any, e: MouseEvent) {
  e.preventDefault()
  selectedItem.value = row
  ctxMenu.value = {
    visible: true,
    x: e.clientX,
    y: e.clientY,
    row,
    showColor: false,
  }
}

function closeContextMenu() {
  ctxMenu.value.visible = false
  ctxMenu.value.showColor = false
}

function setColor(color: string) {
  if (ctxMenu.value.row) {
    ctxMenu.value.row.color = color
  }
  closeContextMenu()
}

function handleInteract(item?: ScreenshotItem | null) {
  closeContextMenu()
  const target = item || selectedItem.value
  if (!target) return
  const client = clientsStore.getClientByUid(target.uid)
  const label = client ? `${client.Username}@${client.Computer || client.ExternalIP}` : target.host
  router.push(`/client/${target.uid}/console`)
}

function openFullscreen(item?: ScreenshotItem | null) {
  const target = item || selectedItem.value
  if (!target) return
  selectedItem.value = target
  fsZoomFactor.value = 1
  fullscreenVisible.value = true
}

function downloadItem(item?: ScreenshotItem | null) {
  closeContextMenu()
  const target = item || selectedItem.value
  if (!target || !target.dataUrl) return
  const link = document.createElement('a')
  link.href = target.dataUrl
  link.download = `screenshot_${(target.computer || target.host).replace(/[^\w]/g, '_')}_${Date.now()}.png`
  link.click()
  ElMessage.success(isZh.value ? '屏幕截图已保存' : 'Screenshot saved')
}

function deleteItem(item?: ScreenshotItem | null) {
  closeContextMenu()
  const target = item || selectedItem.value
  if (!target) return
  ElMessageBox.confirm(
    isZh.value ? '确定删除该屏幕截图记录吗？' : 'Remove this screenshot?',
    isZh.value ? '确认删除' : 'Confirm Delete',
    {
      confirmButtonText: isZh.value ? '删除' : 'Delete',
      cancelButtonText: isZh.value ? '取消' : 'Cancel',
      type: 'warning',
    },
  )
    .then(() => {
      screenshotsStore.removeScreenshot(target.id)
      ElMessage.success(isZh.value ? '已删除截图' : 'Screenshot removed')
    })
    .catch(() => {})
}

function handleClear() {
  ElMessageBox.confirm(
    isZh.value
      ? isClientScoped.value && scopeFilter.value === 'current'
        ? '确定清空当前会话的所有屏幕截图记录吗？'
        : '确定清空所有屏幕截图记录吗？'
      : 'Clear all screenshot records?',
    isZh.value ? '清空确认' : 'Confirm Clear',
    {
      confirmButtonText: isZh.value ? '清空' : 'Clear',
      cancelButtonText: isZh.value ? '取消' : 'Cancel',
      type: 'warning',
    },
  )
    .then(() => {
      if (isClientScoped.value && scopeFilter.value === 'current') {
        screenshotsStore.clearScreenshots(resolvedUid.value)
      } else {
        screenshotsStore.clearScreenshots()
      }
      ElMessage.success(isZh.value ? '已清空截图列表' : 'Screenshots cleared')
    })
    .catch(() => {})
}

async function handleCapture() {
  const uid = effectiveTargetUid.value
  if (!uid) {
    ElMessage.warning(isZh.value ? '请先选择需要截屏的客户端' : 'Please select a client first')
    return
  }
  const client = clientsStore.getClientByUid(uid)
  const clientName = client?.Computer || uid.slice(0, 8)
  const loadingMsg = ElMessage.info({
    message: isZh.value ? `正在抓取 ${clientName} 屏幕画面...` : `Capturing screen for ${clientName}...`,
    duration: 0,
  })

  try {
    const item = await screenshotsStore.capture(uid, screenshotQuality.value, isZh.value)
    loadingMsg.close()
    if (item) {
      selectedItem.value = item
      zoomLevel.value = 1
      ElMessage.success(isZh.value ? `成功捕获 ${clientName} 屏幕截图！` : `Screenshot captured for ${clientName}`)
    }
  } catch (error: any) {
    loadingMsg.close()
    ElMessage.error(error?.message || (isZh.value ? '抓取屏幕失败' : 'Failed to capture screen'))
  }
}

function recaptureTarget(item?: ScreenshotItem | null) {
  closeContextMenu()
  if (item?.uid) {
    selectedTargetUid.value = item.uid
    handleCapture()
  }
}

function refreshAll() {
  if (selectedTargetUid.value) {
    handleCapture()
  } else {
    ElMessage.info(
      isZh.value
        ? '截图列表随抓取实时更新，请先选择目标会话'
        : 'Screenshot list updates with each capture. Select a target session first.',
    )
  }
}

function showHelp() {
  ElMessageBox.alert(
    isZh.value
      ? '【屏幕截图功能说明】\n1. 支持在分屏模式、全表格模式和网格画廊模式之间无缝切换。\n2. 支持滚轮缩放、1:1 还原以及全屏无损查看。\n3. 在分屏预览或列表中双击即可打开全屏模式。\n4. 支持右键菜单会话交互与颜色标记。'
      : '【Screenshots Usage】\n1. Seamlessly switch between Split View, Table View, and Grid Gallery.\n2. Mouse wheel zoom, 1:1 reset, and fullscreen viewer.\n3. Double click on list or preview to open fullscreen modal.',
    isZh.value ? '屏幕截图使用帮助' : 'Screenshots Help',
    { confirmButtonText: isZh.value ? '确定' : 'OK' },
  )
}

onMounted(() => {
  if (isClientScoped.value && !selectedTargetUid.value) {
    selectedTargetUid.value = resolvedUid.value
  }
})
</script>

<style scoped>
.global-screenshots {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-bg);
  overflow: hidden;
  position: relative;
}

/* ===== 工具条：白底 + 1px 分隔线 ===== */
.gs-toolbar {
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

.gs-title-group {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.gs-title-icon {
  font-size: 16px;
  color: var(--cs-active);
}

.gs-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.gs-count {
  font-size: 13px;
  color: var(--cs-text-dim);
  white-space: nowrap;
}

.gs-scope-toggle {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  margin-left: 6px;
  padding: 2px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-border-strong);
  border-radius: 6px;
}

.gs-scope-btn {
  height: 26px;
  padding: 0 10px;
  font-family: var(--cs-font-ui);
  font-size: 13px;
  color: var(--cs-text-dim);
  background: transparent;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  white-space: nowrap;
  transition: background 120ms ease, color 120ms ease;
}

.gs-scope-btn:hover {
  color: var(--cs-text);
  background: var(--cs-hover);
}

.gs-scope-btn.active {
  color: #ffffff;
  background: var(--cs-active);
}

.gs-spacer {
  flex: 1;
}

/* 视图切换（分段控件） */
.gs-btn-group {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  padding: 2px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-border-strong);
  border-radius: 6px;
}

.gs-btn {
  height: 26px;
  padding: 0 10px;
  font-family: var(--cs-font-ui);
  font-size: 13px;
  color: var(--cs-text-dim);
  background: transparent;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  white-space: nowrap;
  transition: background 120ms ease, color 120ms ease;
}

.gs-btn:hover {
  color: var(--cs-text);
  background: var(--cs-hover);
}

.gs-btn.active {
  color: #ffffff;
  background: var(--cs-active);
}

.gs-quality-group {
  display: flex;
  align-items: center;
  gap: 6px;
}

.gs-label {
  font-size: 13px;
  color: var(--cs-text-dim);
  white-space: nowrap;
}

.gs-search-input {
  width: 220px;
}

.gs-btn-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  color: var(--cs-text);
  background: var(--cs-panel);
  border: 1px solid var(--cs-border-strong);
  border-radius: 6px;
  cursor: pointer;
  transition: background 120ms ease, color 120ms ease;
}

.gs-btn-icon:hover {
  background: var(--cs-hover);
  color: var(--cs-active);
}

.gs-status-info {
  font-family: var(--cs-font-mono);
  font-size: 13px;
  color: var(--cs-text-dim);
  white-space: nowrap;
}

/* ===== 主体（分屏 / 表格 / 网格） ===== */
.gs-body {
  flex: 1;
  min-height: 0;
  position: relative;
  overflow: hidden;
}

.gs-body.mode-split {
  display: flex;
}

.gs-table-pane {
  flex: 1;
  min-width: 340px;
  height: 100%;
  overflow: hidden;
  position: relative;
}

.mode-split .gs-table-pane {
  flex: 0 0 52%;
  border-right: 1px solid var(--cs-card-border);
}

/* 预览面板 */
.gs-preview-pane {
  flex: 1;
  display: flex;
  flex-direction: column;
  background: var(--cs-panel);
  overflow: hidden;
}

.gs-preview-header {
  flex-shrink: 0;
  min-height: 44px;
  padding: 0 12px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-card-border);
}

.gs-preview-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.gs-preview-tools {
  display: flex;
  gap: 6px;
}

.gs-preview-content {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 12px;
  background: var(--cs-console);
  overflow: hidden;
  position: relative;
}

.gs-preview-img-wrap {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  position: relative;
  overflow: hidden;
}

.gs-preview-img {
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
  border-radius: 6px;
  box-shadow: 0 3px 6px -4px rgba(0, 0, 0, 0.12), 0 6px 16px 0 rgba(0, 0, 0, 0.08);
  transition: transform 120ms ease-out;
}

.sc-floating-controls {
  position: absolute;
  right: 16px;
  bottom: 16px;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 8px;
  background: color-mix(in srgb, var(--cs-panel) 92%, transparent);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  box-shadow: 0 3px 6px -4px rgba(0, 0, 0, 0.12), 0 6px 16px 0 rgba(0, 0, 0, 0.08);
  backdrop-filter: blur(4px);
  z-index: 10;
}

.zoom-text {
  min-width: 46px;
  font-family: var(--cs-font-mono);
  font-size: 13px;
  text-align: center;
  color: var(--cs-text);
}

.gs-preview-placeholder {
  font-size: 14px;
  text-align: center;
  color: var(--cs-text-dim);
}

/* ===== 网格画廊 ===== */
.gs-grid-wrap {
  position: relative;
  height: 100%;
  overflow-y: auto;
  padding: 16px;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 16px;
  align-content: start;
}

.gs-card {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: var(--cs-card-bg);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  cursor: pointer;
  transition: border-color 120ms ease;
}

.gs-card:hover {
  border-color: color-mix(in srgb, var(--cs-active) 55%, var(--cs-card-border));
}

.gs-card.selected {
  border-color: var(--cs-active);
}

.gs-card-img-wrap {
  position: relative;
  height: 170px;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  background: var(--cs-console);
}

.gs-card-img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.gs-card-overlay {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  background: rgba(0, 0, 0, 0.55);
  opacity: 0;
  transition: opacity 150ms ease;
}

.gs-card:hover .gs-card-overlay {
  opacity: 1;
}

.gs-card-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  background: var(--cs-panel);
  border-top: 1px solid var(--cs-card-border);
}

.gs-card-host {
  max-width: 160px;
  font-size: 13px;
  font-weight: 600;
  color: var(--cs-text-bright);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.gs-card-time {
  font-family: var(--cs-font-mono);
  font-size: 12px;
  color: var(--cs-text-dim);
}

/* ===== 单元格渲染 ===== */
.mono-cell {
  font-family: var(--cs-font-mono);
  color: var(--cs-text);
}

.user-cell {
  font-weight: 600;
  color: var(--cs-text);
}

.host-cell {
  color: var(--cs-text);
}

.title-cell {
  color: var(--cs-text);
}

.quality-badge {
  display: inline-flex;
  align-items: center;
  height: 20px;
  padding: 0 7px;
  border-radius: 4px;
  font-size: 12px;
}

.quality-badge.q-0 {
  background: color-mix(in srgb, var(--cs-text-dim) 16%, transparent);
  color: var(--cs-text-dim);
}

.quality-badge.q-1 {
  background: color-mix(in srgb, var(--cs-active) 12%, transparent);
  color: var(--cs-active);
}

.quality-badge.q-2 {
  background: color-mix(in srgb, var(--cs-online) 14%, transparent);
  color: var(--cs-online);
  font-weight: 600;
}

/* ===== 右键菜单 ===== */
.cs-listener-ctx-menu {
  position: fixed;
  min-width: 180px;
  padding: 4px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  box-shadow: 0 3px 6px -4px rgba(0, 0, 0, 0.12), 0 6px 16px 0 rgba(0, 0, 0, 0.08);
  z-index: 9999;
}

.cs-listener-ctx-menu .ctx-item {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 32px;
  padding: 0 10px;
  font-size: 14px;
  color: var(--cs-text);
  border-radius: 6px;
  cursor: pointer;
}

.cs-listener-ctx-menu .ctx-item:hover {
  background: var(--cs-hover);
  color: var(--cs-active);
}

.cs-listener-ctx-menu .ctx-arrow {
  margin-left: 14px;
  font-size: 12px;
  color: var(--cs-text-dim);
}

.cs-listener-ctx-menu .ctx-color-palette {
  position: absolute;
  top: -2px;
  left: 100%;
  display: flex;
  gap: 6px;
  padding: 6px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  box-shadow: 0 3px 6px -4px rgba(0, 0, 0, 0.12), 0 6px 16px 0 rgba(0, 0, 0, 0.08);
  z-index: 10000;
}

.color-box {
  width: 20px;
  height: 20px;
  border: 1px solid var(--cs-border-strong);
  border-radius: 4px;
  cursor: pointer;
}

.color-box:hover {
  border-color: var(--cs-active);
}

.cs-listener-ctx-menu .ctx-divider {
  height: 1px;
  margin: 4px 0;
  background: var(--cs-card-border);
}

.cs-listener-ctx-menu .ctx-item.is-highlight {
  font-weight: 600;
  color: var(--cs-active);
}

.cs-listener-ctx-menu .ctx-item.ctx-danger {
  color: var(--cs-error);
}

.cs-listener-ctx-menu .ctx-item.ctx-danger:hover {
  background: color-mix(in srgb, var(--cs-error) 12%, transparent);
  color: var(--cs-error);
}

/* ===== 全屏查看对话框 ===== */
.gs-fs-body {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 14px;
}

.gs-fs-img-wrap {
  width: 100%;
  height: 72vh;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  background: var(--cs-console);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
}

.gs-fs-img {
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
  cursor: grab;
  transition: transform 100ms ease-out;
}

.gs-fs-controls {
  display: flex;
  align-items: center;
  gap: 10px;
}

.zoom-label {
  min-width: 56px;
  font-family: var(--cs-font-mono);
  font-size: 14px;
  text-align: center;
  color: var(--cs-text-dim);
}
</style>
