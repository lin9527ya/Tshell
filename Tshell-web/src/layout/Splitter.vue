<template>
  <div
    ref="splitterEl"
    class="cs-splitter"
    :class="{ dragging: isDragging }"
    @mousedown="startDrag"
    @dblclick="onDoubleClick"
  >
    <div class="splitter-line" />
    <div class="splitter-controls" @mousedown.stop>
      <button
        class="split-btn"
        :title="isTopCollapsed ? (isZh ? '展开顶部视图' : 'Expand Top') : (isZh ? '折叠顶部视图' : 'Collapse Top')"
        @click="toggleTop"
      >
        ▲
      </button>
      <div class="split-grip" title="拖动调整上下分屏 / Drag to resize split">
        <span class="grip-dot" />
        <span class="grip-dot" />
        <span class="grip-dot" />
      </div>
      <button
        class="split-btn"
        :title="isBottomCollapsed ? (isZh ? '展开底部视图' : 'Expand Bottom') : (isZh ? '折叠底部视图' : 'Collapse Bottom')"
        @click="toggleBottom"
      >
        ▼
      </button>
    </div>
    <div class="splitter-line" />

    <!-- 全局拖拽遮罩：防止拖拽过程中被下层的 xterm terminal / iframe / svg 拦截事件 -->
    <teleport to="body">
      <div
        v-if="isDragging"
        class="cs-split-drag-overlay"
        @mousemove="onMouseMove"
        @mouseup="onMouseUp"
      >
        <!-- 拖拽时百分比实时提示 -->
        <div class="split-ratio-bubble cs-font-mono">{{ Math.round(liveRatio) }}%</div>
      </div>
    </teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useWorkspaceStore } from '@/stores/workspace'
import { useI18nStore } from '@/stores/i18n'

const props = defineProps<{
  containerEl?: HTMLElement | null
}>()

const workspace = useWorkspaceStore()
const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const splitterEl = ref<HTMLElement | null>(null)
const isDragging = ref(false)
const liveRatio = ref(46) // 拖拽期间的实时百分比(浮标显示)
let startY = 0
let startRatio = 0

const isTopCollapsed = computed(() => workspace.isTopCollapsed)
const isBottomCollapsed = computed(() => workspace.isBottomCollapsed)

function toggleTop() {
  workspace.toggleTopCollapse()
}

function toggleBottom() {
  workspace.toggleBottomCollapse()
}

function getParentHeight(): number {
  if (props.containerEl) {
    const rect = props.containerEl.getBoundingClientRect()
    return rect.height
  }
  if (splitterEl.value && splitterEl.value.parentElement) {
    const rect = splitterEl.value.parentElement.getBoundingClientRect()
    return rect.height
  }
  return window.innerHeight - 80 // fallback
}

function startDrag(e: MouseEvent) {
  e.preventDefault()
  isDragging.value = true
  startY = e.clientY
  startRatio = workspace.splitRatio
  document.body.style.cursor = 'row-resize'
  document.body.style.userSelect = 'none'
}

function onMouseMove(e: MouseEvent) {
  if (!isDragging.value) return
  const totalH = getParentHeight()
  if (totalH <= 0) return

  const dy = e.clientY - startY
  const dRatio = (dy / totalH) * 100
  // 夹取范围与 workspace.setSplitRatio(15-85)保持一致——原 10-90 会先进入
  // store 的钳制死区(10-14/86-90 拖到也不生效)
  const nextRatio = Math.max(15, Math.min(85, startRatio + dRatio))
  liveRatio.value = nextRatio
  workspace.setSplitRatio(nextRatio)
}

// 双击分屏线复位到 46%(store 的 resetSplit 早已实现但一直无调用点)
function onDoubleClick() {
  workspace.resetSplit()
  liveRatio.value = workspace.splitRatio
}

function onMouseUp() {
  if (isDragging.value) {
    isDragging.value = false
    document.body.style.cursor = ''
    document.body.style.userSelect = ''
  }
}

onMounted(() => {
  window.addEventListener('mousemove', onMouseMove)
  window.addEventListener('mouseup', onMouseUp)
})

onBeforeUnmount(() => {
  window.removeEventListener('mousemove', onMouseMove)
  window.removeEventListener('mouseup', onMouseUp)
  document.body.style.cursor = ''
  document.body.style.userSelect = ''
})
</script>

<style scoped>
.cs-splitter {
  height: 6px;
  background: var(--cs-titlebar-bg);
  border-top: 1px solid var(--cs-border);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: row-resize;
  user-select: none;
  flex-shrink: 0;
  position: relative;
  transition: background 120ms ease;
  z-index: 100;
}
.cs-splitter::before {
  content: '';
  position: absolute;
  top: -4px;
  bottom: -4px;
  left: 0;
  right: 0;
  z-index: 1;
}
.cs-splitter:hover,
.cs-splitter.dragging {
  background: color-mix(in srgb, var(--cs-active) 40%, var(--cs-titlebar-bg));
  border-color: var(--cs-active);
}
.splitter-line {
  flex: 1;
  height: 1px;
}
.splitter-controls {
  position: relative;
  z-index: 2;
  display: flex;
  align-items: center;
  gap: 3px;
  padding: 0 6px;
  background: var(--cs-titlebar-bg);
  border-radius: 2px;
  cursor: default;
}
.split-btn {
  background: transparent;
  border: 0;
  color: var(--cs-text-dim);
  font-size: 8px;
  width: 14px;
  height: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  padding: 0;
  border-radius: 1px;
}
.split-btn:hover {
  background: var(--cs-hover);
  color: var(--cs-text-bright);
}
.split-grip {
  display: flex;
  align-items: center;
  gap: 3px;
  padding: 0 4px;
  cursor: row-resize;
}
.grip-dot {
  width: 3px;
  height: 3px;
  border-radius: 50%;
  background: var(--cs-text-dim);
}
.cs-splitter:hover .grip-dot {
  background: var(--cs-active);
}
</style>

<style>
.cs-split-drag-overlay {
  position: fixed;
  inset: 0;
  z-index: 999999;
  cursor: row-resize !important;
  background: transparent;
  user-select: none;
}

.split-ratio-bubble {
  position: fixed;
  left: 50%;
  top: 50%;
  transform: translate(-50%, -50%);
  background: var(--cs-panel);
  color: var(--cs-text-bright);
  border: 1px solid var(--cs-active);
  border-radius: 3px;
  padding: 2px 10px;
  font-size: 14px;
  pointer-events: none;
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.4);
}
</style>
