<template>
  <div class="pg-root">
    <PageHeader
      :title="i18nStore.tl('拓扑网络图', 'Pivot Graph')"
      :subtitle="i18nStore.tl('服务端 → 监听器 → 客户端 → 隧道 → 目标', 'Server → Listener → Client → Tunnel → Target')"
    >
      <div class="pg-actions">
        <span class="pg-stat">
          {{ i18nStore.tl('节点', 'Nodes') }} {{ layout.nodes.length }} ·
          {{ i18nStore.tl('在线', 'Online') }} {{ onlineCount }} / {{ clientCount }}
        </span>
        <button class="cs-btn cs-btn-xs" :disabled="loading" @click="load">
          {{ loading ? (i18nStore.tl('刷新中…', 'Loading…')) : (i18nStore.tl('刷新', 'Refresh')) }}
        </button>
      </div>
    </PageHeader>

    <!-- 图例：颜色/实心空心的含义必须在图上说明，否则用户只能猜 -->
    <div class="pg-legend">
      <span v-for="t in legendTypes" :key="t" class="pg-legend-item">
        <i class="pg-dot" :style="{ background: TYPE_COLORS[t] }"></i>
        {{ i18nStore.tl(TYPE_LABELS[t]?.zh || t, TYPE_LABELS[t]?.en || t) }}
      </span>
      <span class="pg-legend-item">
        <i class="pg-dot pg-dot-offline"></i>
        {{ i18nStore.tl('离线客户端（灰色虚线）', 'Offline client (dashed)') }}
      </span>
    </div>

    <div v-if="error" class="pg-error">{{ error }}</div>

    <div v-else-if="!loading && layout.nodes.length === 0" class="pg-empty">
      {{ i18nStore.tl('暂无拓扑数据：先创建监听器并让客户端上线', 'No topology data yet: create a listener and bring a client online') }}
    </div>

    <div v-else class="pg-canvas" :class="{ 'is-panning': canvasPanning }" :ref="setCanvasRef">
      <svg
        ref="svgEl"
        class="pg-svg"
        role="img"
        :aria-label="i18nStore.tl('拓扑网络图', 'Pivot graph')"
        @wheel.prevent="onWheel"
        @pointerdown="onCanvasPointerDown"
        @click="onCanvasClick"
      >
        <g :transform="`translate(${view.x} ${view.y}) scale(${view.k})`">
          <!-- 边先画，节点压在上面 -->
          <g class="pg-edges">
            <line
              v-for="e in viewEdges"
              :key="e.id"
              :x1="e.x1"
              :y1="e.y1"
              :x2="e.x2"
              :y2="e.y2"
              :class="['pg-edge', { 'pg-edge-active': isEdgeActive(e) }]"
            />
          </g>

          <g class="pg-nodes">
            <g
              v-for="nd in viewNodes"
              :key="nd.id"
              :transform="`translate(${nd.x}, ${nd.y})`"
              :class="['pg-node', { 'pg-node-selected': selected === nd.id, 'pg-node-offline': !isOnline(nd) }]"
              role="button"
              tabindex="0"
              @pointerdown.stop="onNodePointerDown($event, nd)"
              @click.stop="onNodeClick(nd)"
              @keydown.enter.prevent="onNodeClick(nd)"
              @keydown.space.prevent="onNodeClick(nd)"
            >
              <!-- 悬停/选中的光晕（CS 拓扑图同款：节点周围一圈淡色） -->
              <circle class="pg-node-halo" r="16" :fill="TYPE_COLORS[nd.type] || '#6b7280'" opacity="0" />
              <circle
                r="9"
                :fill="isOnline(nd) ? TYPE_COLORS[nd.type] || '#6b7280' : 'transparent'"
                :stroke="TYPE_COLORS[nd.type] || '#6b7280'"
                stroke-width="2"
                :stroke-dasharray="isOnline(nd) ? undefined : '3 2'"
              />
              <text class="pg-node-label" x="16" y="4">{{ nd.label }}</text>
              <text v-if="nd.sub_label" class="pg-node-sub" x="16" y="19">{{ nd.sub_label }}</text>
            </g>
          </g>
        </g>
      </svg>

      <!-- 画布工具条：缩放 / 适配 / 节点复位 -->
      <div class="pg-viewbar" @pointerdown.stop @click.stop>
        <button class="pg-viewbtn" :title="i18nStore.tl('缩小', 'Zoom out')" @click="zoomBy(1 / 1.2)">−</button>
        <span class="pg-zoom">{{ Math.round(view.k * 100) }}%</span>
        <button class="pg-viewbtn" :title="i18nStore.tl('放大', 'Zoom in')" @click="zoomBy(1.2)">+</button>
        <span class="pg-viewsep" />
        <button class="pg-viewbtn" :title="i18nStore.tl('适配视图', 'Fit view')" @click="fitView">
          {{ i18nStore.tl('适配', 'Fit') }}
        </button>
        <button class="pg-viewbtn" :title="i18nStore.tl('节点复位到自动布局', 'Reset to auto layout')" @click="resetLayout">
          {{ i18nStore.tl('自动布局', 'Auto layout') }}
        </button>
      </div>
    </div>

    <!-- 选中节点的详情：图上一眼看不全的信息（meta）放这里 -->
    <div v-if="selectedNode" class="pg-detail">
      <div class="pg-detail-head">
        <strong>{{ selectedNode.label }}</strong>
        <span class="pg-detail-type">
          {{ i18nStore.tl(
            TYPE_LABELS[selectedNode.type]?.zh || selectedNode.type,
            TYPE_LABELS[selectedNode.type]?.en || selectedNode.type,
          ) }}
        </span>
        <span v-if="selectedNode.type === 'client'" :class="['pg-badge', isOnline(selectedNode) ? 'pg-badge-on' : 'pg-badge-off']">
          {{ isOnline(selectedNode) ? (i18nStore.tl('在线', 'Online')) : (i18nStore.tl('离线', 'Offline')) }}
        </span>
        <button class="cs-btn cs-btn-xs" @click="selected = null">{{ i18nStore.tl('关闭', 'Close') }}</button>
      </div>
      <div v-if="selectedNode.sub_label" class="pg-detail-sub">{{ selectedNode.sub_label }}</div>
      <table v-if="metaRows.length" class="pg-meta">
        <tbody>
          <tr v-for="row in metaRows" :key="row.k">
            <td class="pg-meta-k">{{ row.k }}</td>
            <td class="pg-meta-v">{{ row.v }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * Pivot 拓扑网络图（阶段三 §3.11）。
 *
 * 这个视图此前是**空壳**：`api/clients/index.ts` 的 `get_graph_data()` 与请求层
 * 对 `/graph/data` 的静默失败特判都在，i18n 也有 `tb.pivot_graph`，
 * 但**没有任何组件消费** —— 后端算好的关系图没人看。
 *
 * 这里把数据画出来，并刻意做了三件"图能看懂"的事：
 *  1. 分层布局（layout.ts）而不是力导向：这份数据本身是分层的，
 *     而且力导向每次刷新的坐标都不同，同一个拓扑看起来像变了一个网；
 *  2. 离线客户端画成**灰色虚线空心**，与在线实心一眼可分 —— 否则一片点看不出谁掉线；
 *  3. 点击节点在下方列出 meta：图上放不下的地址/密钥指纹等信息集中在这里。
 */
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import PageHeader from '@/components/Common/PageHeader.vue'
import ClientApi from '@/api/clients'
import { useI18nStore } from '@/stores/i18n'
import {
  computeLayout,
  isOnline,
  TYPE_COLORS,
  TYPE_LABELS,
  type GraphEdge,
  type GraphNode,
  type LaidOutEdge,
} from './layout'

const i18nStore = useI18nStore()

const nodes = ref<GraphNode[]>([])
const edges = ref<GraphEdge[]>([])
const loading = ref(false)
const error = ref('')
const selected = ref<string | null>(null)

const layout = computed(() => computeLayout(nodes.value, edges.value))

// ---- 画布交互：平移 / 缩放 / 节点拖拽（CS 拓扑图同款） ----
const svgEl = ref<SVGSVGElement | null>(null)
const canvasPanning = ref(false)
const view = reactive({ x: 0, y: 0, k: 1 })
const MIN_K = 0.3
const MAX_K = 2.5
const hasFitted = ref(false)
const canvasSize = reactive({ w: 0, h: 0 })
let canvasObserver: ResizeObserver | null = null

// 手动拖拽后的节点位置（localStorage 持久化；节点 id 稳定，服务端删掉的 id 自然失效）
const POS_KEY = 'tshell-graph-positions'
const manualPos = reactive<Record<string, { x: number; y: number }>>(loadPositions())

function loadPositions(): Record<string, { x: number; y: number }> {
  try {
    const raw = localStorage.getItem(POS_KEY)
    const parsed = raw ? JSON.parse(raw) : null
    return parsed && typeof parsed === 'object' ? parsed : {}
  } catch {
    return {}
  }
}

function savePositions() {
  try {
    localStorage.setItem(POS_KEY, JSON.stringify(manualPos))
  } catch {
    /* 存储不可用时忽略：仅影响本机展示 */
  }
}

const posOf = computed(() => {
  const m = new Map<string, { x: number; y: number }>()
  for (const n of layout.value.nodes) {
    const o = manualPos[n.id]
    m.set(n.id, o ? { x: o.x, y: o.y } : { x: n.x, y: n.y })
  }
  return m
})

const viewNodes = computed(() => layout.value.nodes.map((n) => ({ ...n, ...(posOf.value.get(n.id) || n) })))
const viewEdges = computed(() =>
  layout.value.edges.map((e) => {
    const sp = posOf.value.get(e.source)
    const tp = posOf.value.get(e.target)
    return { ...e, x1: sp?.x ?? e.x1, y1: sp?.y ?? e.y1, x2: tp?.x ?? e.x2, y2: tp?.y ?? e.y2 }
  }),
)

/** 适配视图：把整张图（含右侧标签宽度）装进画布并居中 */
function fitView() {
  const el = svgEl.value
  const ns = viewNodes.value
  if (!el || !ns.length) return
  const rect = el.getBoundingClientRect()
  const w = rect.width || canvasSize.w
  const h = rect.height || canvasSize.h
  if (!w || !h) return
  const xs = ns.map((n) => n.x)
  const ys = ns.map((n) => n.y)
  const minX = Math.min(...xs) - 40
  const maxX = Math.max(...xs) + 220 // 标签在节点右侧
  const minY = Math.min(...ys) - 30
  const maxY = Math.max(...ys) + 30
  const pad = 28
  const bw = Math.max(1, maxX - minX)
  const bh = Math.max(1, maxY - minY)
  const k = Math.min((w - pad * 2) / bw, (h - pad * 2) / bh)
  view.k = Math.min(MAX_K, Math.max(MIN_K, Math.min(k, 1.4)))
  view.x = w / 2 - ((minX + maxX) / 2) * view.k
  view.y = h / 2 - ((minY + maxY) / 2) * view.k
  hasFitted.value = true
}

function zoomAt(factor: number, cx: number, cy: number) {
  const k2 = Math.min(MAX_K, Math.max(MIN_K, view.k * factor))
  const s = k2 / view.k
  view.x = cx - (cx - view.x) * s
  view.y = cy - (cy - view.y) * s
  view.k = k2
}

function zoomBy(factor: number) {
  const r = svgEl.value?.getBoundingClientRect()
  zoomAt(factor, (r?.width || canvasSize.w) / 2, (r?.height || canvasSize.h) / 2)
}

function onWheel(e: WheelEvent) {
  const r = svgEl.value?.getBoundingClientRect()
  if (!r) return
  zoomAt(e.deltaY < 0 ? 1.12 : 1 / 1.12, e.clientX - r.left, e.clientY - r.top)
}

type Drag =
  | { kind: 'pan'; sx: number; sy: number; ox: number; oy: number }
  | { kind: 'node'; id: string; sx: number; sy: number; ox: number; oy: number; moved: boolean }

let drag: Drag | null = null
let suppressClickUntil = 0

function onCanvasPointerDown(e: PointerEvent) {
  if (e.button !== 0 && e.button !== 1) return
  if ((e.target as Element).closest('.pg-node')) return
  drag = { kind: 'pan', sx: e.clientX, sy: e.clientY, ox: view.x, oy: view.y }
  canvasPanning.value = true
  window.addEventListener('pointermove', onPointerMove)
  window.addEventListener('pointerup', onPointerUp, { once: true })
}

function onNodePointerDown(e: PointerEvent, nd: { id: string; x: number; y: number }) {
  if (e.button !== 0) return
  drag = { kind: 'node', id: nd.id, sx: e.clientX, sy: e.clientY, ox: nd.x, oy: nd.y, moved: false }
  window.addEventListener('pointermove', onPointerMove)
  window.addEventListener('pointerup', onPointerUp, { once: true })
}

function onPointerMove(e: PointerEvent) {
  if (!drag) return
  if (drag.kind === 'pan') {
    view.x = drag.ox + (e.clientX - drag.sx)
    view.y = drag.oy + (e.clientY - drag.sy)
    return
  }
  const dx = (e.clientX - drag.sx) / view.k
  const dy = (e.clientY - drag.sy) / view.k
  if (!drag.moved && Math.abs(e.clientX - drag.sx) + Math.abs(e.clientY - drag.sy) > 4) {
    drag.moved = true
  }
  manualPos[drag.id] = { x: drag.ox + dx, y: drag.oy + dy }
}

function onPointerUp() {
  if (drag?.kind === 'node' && drag.moved) {
    savePositions()
    // 拖拽结束后浏览器还会补一个 click，短暂抑制避免误触发选中/取消
    suppressClickUntil = Date.now() + 150
  }
  drag = null
  canvasPanning.value = false
  window.removeEventListener('pointermove', onPointerMove)
}

function onNodeClick(nd: { id: string }) {
  if (Date.now() < suppressClickUntil) return
  selected.value = selected.value === nd.id ? null : nd.id
}

function onCanvasClick(e: MouseEvent) {
  if (Date.now() < suppressClickUntil) return
  if (!(e.target as Element).closest('.pg-node')) selected.value = null
}

/** 清掉手动位置，节点回到分层自动布局 */
function resetLayout() {
  for (const key of Object.keys(manualPos)) delete manualPos[key]
  savePositions()
  fitView()
}

// 画布按需渲染（空数据/加载中不渲染），函数 ref 在元素出现时挂 ResizeObserver；
// 首次拿到尺寸时自动适配一次
function setCanvasRef(el: any) {
  canvasObserver?.disconnect()
  canvasObserver = null
  if (!el || typeof ResizeObserver === 'undefined') return
  canvasObserver = new ResizeObserver((entries) => {
    const r = entries[0]?.contentRect
    if (!r) return
    canvasSize.w = r.width
    canvasSize.h = r.height
    if (!hasFitted.value && viewNodes.value.length) fitView()
  })
  canvasObserver.observe(el)
}

onBeforeUnmount(() => {
  canvasObserver?.disconnect()
  canvasObserver = null
  window.removeEventListener('pointermove', onPointerMove)
})

const legendTypes = ['server', 'listener', 'client', 'tunnel', 'target']

const clientCount = computed(() => nodes.value.filter((n) => n.type === 'client').length)
const onlineCount = computed(() => nodes.value.filter((n) => n.type === 'client' && isOnline(n)).length)

const selectedNode = computed(() => nodes.value.find((n) => n.id === selected.value) || null)
const metaRows = computed(() =>
  Object.entries(selectedNode.value?.meta || {})
    .filter(([, v]) => v !== '')
    .map(([k, v]) => ({ k, v: String(v) })),
)

/** 选中节点相关的边高亮，其余淡出 —— 否则节点一多就分不清谁连谁 */
function isEdgeActive(e: LaidOutEdge): boolean {
  if (!selected.value) return true
  return e.source === selected.value || e.target === selected.value
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await ClientApi.get_graph_data()
    const data = res?.data?.data || {}
    nodes.value = Array.isArray(data.nodes) ? data.nodes : []
    edges.value = Array.isArray(data.edges) ? data.edges : []
    // 选中的节点可能在刷新后消失了（客户端被删），清掉避免详情区停在幽灵节点
    if (selected.value && !nodes.value.some((n) => n.id === selected.value)) {
      selected.value = null
    }
  } catch (e: unknown) {
    // /graph/data 是 operatorOnly：auditor 拿到 403 属正常，不该弹全局报错
    // （请求层已对该 URL 特判静默）。这里给出页面内提示而不是空白页。
    const msg = (e as { response?: { status?: number } })?.response?.status === 403
      ? i18nStore.tl(
          '当前角色无权查看拓扑图（需要 operator 及以上）',
          'Your role cannot view the pivot graph (operator+)',
        )
      : i18nStore.tl('加载拓扑数据失败', 'Failed to load topology data')
    error.value = msg
    nodes.value = []
    edges.value = []
  } finally {
    loading.value = false
    nextTick(() => {
      if (!hasFitted.value) fitView()
    })
  }
}

onMounted(load)
</script>

<style scoped>
.pg-root {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  padding: 12px 16px;
  gap: 8px;
  color: var(--vs-text, #e6e6e6);
}
.pg-head {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.pg-title {
  font-size: 15px;
  font-weight: 600;
}
.pg-sub {
  font-size: 12px;
  opacity: 0.6;
}
.pg-actions {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 10px;
}
.pg-stat {
  font-size: 12px;
  opacity: 0.75;
}
.pg-legend {
  display: flex;
  flex-wrap: wrap;
  gap: 14px;
  font-size: 12px;
  opacity: 0.85;
}
.pg-legend-item {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}
.pg-dot {
  width: 9px;
  height: 9px;
  border-radius: 50%;
  display: inline-block;
}
.pg-dot-offline {
  background: transparent;
  border: 2px dashed #6b7280;
}
.pg-error,
.pg-empty {
  padding: 24px;
  font-size: 13px;
  opacity: 0.8;
}
.pg-error {
  color: #f0a020;
}
.pg-canvas {
  position: relative;
  flex: 1;
  min-height: 0;
  overflow: hidden;
  border: 1px solid var(--vs-border, #333);
  border-radius: 6px;
  background-color: var(--vs-bg-soft, rgba(255, 255, 255, 0.02));
  /* 点阵网格：拖拽时能看出画布在动 */
  background-image: radial-gradient(circle, rgba(148, 163, 184, 0.22) 1px, transparent 1px);
  background-size: 22px 22px;
  cursor: grab;
  touch-action: none;
}

.pg-canvas.is-panning {
  cursor: grabbing;
}

.pg-svg {
  display: block;
  width: 100%;
  height: 100%;
  user-select: none;
}

/* 画布悬浮工具条：缩放 / 适配 / 节点复位 */
.pg-viewbar {
  position: absolute;
  top: 10px;
  right: 10px;
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 3px;
  z-index: 2;
  border: 1px solid var(--cs-card-border, #333);
  border-radius: 6px;
  background: var(--cs-panel, rgba(20, 24, 31, 0.9));
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.18);
}

.pg-viewbtn {
  min-width: 26px;
  height: 24px;
  padding: 0 7px;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: var(--cs-text, inherit);
  font-size: 12px;
  cursor: pointer;
}

.pg-viewbtn:hover {
  background: var(--cs-hover);
  color: var(--cs-active);
}

.pg-zoom {
  min-width: 40px;
  text-align: center;
  font-size: 12px;
  opacity: 0.75;
}

.pg-viewsep {
  width: 1px;
  height: 14px;
  margin: 0 2px;
  background: var(--cs-card-border, #333);
}
.pg-edge {
  stroke: #4b5563;
  stroke-width: 1.2;
  opacity: 0.55;
}
.pg-edge-active {
  stroke: #8b9dc3;
  opacity: 0.9;
}
.pg-node {
  cursor: grab;
}

.pg-node:active {
  cursor: grabbing;
}

.pg-node:hover .pg-node-halo {
  opacity: 0.16;
}

.pg-node-selected .pg-node-halo {
  opacity: 0.3;
}
.pg-node-label {
  font-size: 12px;
  fill: var(--vs-text, #e6e6e6);
}
.pg-node-sub {
  font-size: 10px;
  fill: var(--vs-text, #e6e6e6);
  opacity: 0.55;
}
.pg-node-offline .pg-node-label {
  opacity: 0.5;
}
.pg-node-selected .pg-node-label {
  font-weight: 700;
}
.pg-detail {
  border-top: 1px solid var(--vs-border, #333);
  padding-top: 8px;
  font-size: 12px;
}
.pg-detail-head {
  display: flex;
  align-items: center;
  gap: 10px;
}
.pg-detail-type {
  opacity: 0.6;
}
.pg-detail-sub {
  opacity: 0.7;
  margin-top: 2px;
}
.pg-badge {
  padding: 1px 6px;
  border-radius: 8px;
  font-size: 11px;
}
.pg-badge-on {
  background: rgba(47, 158, 110, 0.2);
  color: #4ade80;
}
.pg-badge-off {
  background: rgba(107, 114, 128, 0.25);
  color: #9ca3af;
}
.pg-meta {
  margin-top: 6px;
  border-collapse: collapse;
}
.pg-meta-k {
  opacity: 0.6;
  padding-right: 12px;
  vertical-align: top;
  white-space: nowrap;
}
.pg-meta-v {
  word-break: break-all;
}

</style>
