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

    <!-- 图例：颜色/实心空心/图标/线型的含义必须在图上说明，否则用户只能猜 -->
    <div class="pg-legend">
      <div class="pg-legend-row">
        <span class="pg-legend-cap">{{ i18nStore.tl('节点', 'Nodes') }}</span>
        <span v-for="t in legendTypes" :key="t" class="pg-legend-item">
          <i class="pg-dot" :style="{ background: TYPE_COLORS[t] }"></i>
          {{ i18nStore.tl(TYPE_LABELS[t]?.zh || t, TYPE_LABELS[t]?.en || t) }}
        </span>
        <span class="pg-legend-item">
          <i class="pg-dot pg-dot-offline"></i>
          {{ i18nStore.tl('离线主机（置灰 + 虚线连接）', 'Offline host (grey, dashed link)') }}
        </span>
        <span class="pg-hint">
          {{ i18nStore.tl(
            '拖拽节点 / 拖空白平移 / 滚轮缩放 / 右键节点出菜单',
            'Drag nodes, drag canvas to pan, wheel to zoom, right-click a node for actions',
          ) }}
        </span>
      </div>
      <div class="pg-legend-row">
        <span class="pg-legend-cap">{{ i18nStore.tl('客户端图标（按系统）', 'Client icons (by OS)') }}</span>
        <span v-for="k in OS_KEYS" :key="'os-' + k" class="pg-legend-item">
          <svg class="pg-legend-glyph" viewBox="-8 -8 16 16" width="14" height="14" aria-hidden="true">
            <path :d="OS_GLYPHS[k].d" />
          </svg>
          {{ i18nStore.tl(OS_GLYPHS[k].zh, OS_GLYPHS[k].en) }}
        </span>
        <span class="pg-legend-cap">{{ i18nStore.tl('目标图标（按端口推测）', 'Target icons (by port)') }}</span>
        <span v-for="k in TARGET_KEYS" :key="'tg-' + k" class="pg-legend-item">
          <svg class="pg-legend-glyph" viewBox="-8 -8 16 16" width="14" height="14" aria-hidden="true">
            <path :d="TARGET_GLYPHS[k].d" />
          </svg>
          {{ i18nStore.tl(TARGET_GLYPHS[k].zh, TARGET_GLYPHS[k].en) }}
        </span>
      </div>
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
        <g class="pg-content" :transform="`translate(${view.x} ${view.y}) scale(${view.k})`">
          <!-- 边先画，节点压在上面 -->
          <g class="pg-edges">
            <line
              v-for="e in viewEdges"
              :key="e.id"
              :x1="e.x1"
              :y1="e.y1"
              :x2="e.x2"
              :y2="e.y2"
              :class="['pg-edge', { 'pg-edge-active': isEdgeActive(e), 'pg-edge-offline': e.offline }]"
            />
            <!-- 边标签：只有后端给了 label 才画（备注之类的边语义）。
                 缩得太小时用 class 隐藏而不是 v-if —— 否则导出（克隆 DOM）会漏掉标签 -->
            <text
              v-for="e in labeledEdges"
              :key="'lb_' + e.id"
              :class="['pg-edge-label', { 'is-hidden': e.hidden }]"
              :x="(e.x1 + e.x2) / 2"
              :y="(e.y1 + e.y2) / 2 - 4"
            >{{ e.label }}</text>
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
              @contextmenu.prevent="openMenu($event, nd)"
              @keydown.enter.prevent="onNodeClick(nd)"
              @keydown.space.prevent="onNodeClick(nd)"
            >
              <!-- 悬停/选中的光晕（CS 拓扑图同款：节点周围一圈淡色） -->
              <circle class="pg-node-halo" r="16" :fill="TYPE_COLORS[nd.type] || '#6b7280'" opacity="0" />
              <circle
                :r="NODE_RADIUS"
                :fill="nodeFill(nd)"
                :stroke="nodeStroke(nd)"
                stroke-width="2"
                :stroke-dasharray="isOnline(nd) ? undefined : '3 2'"
              />
              <!-- 节点图标：客户端按系统（Windows/Linux/macOS），目标按端口推测服务类型 -->
              <path v-if="nodeGlyph(nd)" class="pg-node-glyph" :d="nodeGlyph(nd)!" />
              <text class="pg-node-label" :x="LABEL_DX" y="4">{{ nd.label }}</text>
              <text v-if="nd.sub_label" class="pg-node-sub" :x="LABEL_DX" y="19">{{ nd.sub_label }}</text>
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
        <button
          class="pg-viewbtn"
          :title="i18nStore.tl('按斥力/弹簧重新排布，保留从左到右的分层方向', 'Force-directed arrange, layers kept left-to-right')"
          @click="applyForce"
        >
          {{ i18nStore.tl('力导向', 'Force') }}
        </button>
        <button class="pg-viewbtn" :title="i18nStore.tl('清除手动位置，回到分层布局', 'Clear manual positions, back to layered')" @click="resetLayout">
          {{ i18nStore.tl('分层', 'Layered') }}
        </button>
        <span class="pg-viewsep" />
        <button
          class="pg-viewbtn"
          :title="i18nStore.tl('导出整张拓扑图（PNG / SVG）', 'Export the whole graph (PNG / SVG)')"
          @click="openExportMenu"
        >
          {{ i18nStore.tl('导出', 'Export') }} ▾
        </button>
        <template v-if="focused">
          <span class="pg-viewsep" />
          <button class="pg-viewbtn pg-focus-chip" :title="i18nStore.tl('退出聚焦，显示全部节点', 'Exit focus, show all nodes')" @click="clearFocus">
            {{ i18nStore.tl('聚焦', 'Focus') }}: {{ focusedLabel }} ✕
          </button>
        </template>
      </div>

      <!-- 节点右键菜单：图上放不下的动作集中在这里（点空白/按 Esc 关闭） -->
      <div
        v-if="menu.open"
        class="pg-menu"
        :style="{ left: menu.x + 'px', top: menu.y + 'px' }"
        @pointerdown.stop
        @click.stop
        @contextmenu.prevent
      >
        <button v-for="it in menuItems" :key="it.key" class="pg-menu-item" @click="runMenu(it.key)">
          {{ it.label }}
        </button>
      </div>

      <div
        v-if="exportMenu.open"
        class="pg-menu"
        :style="{ left: exportMenu.x + 'px', top: exportMenu.y + 'px' }"
        @pointerdown.stop
        @click.stop
      >
        <button class="pg-menu-item" @click="doExport('png')">
          {{ i18nStore.tl('PNG 图片（2 倍像素）', 'PNG image (2x)') }}
        </button>
        <button class="pg-menu-item" @click="doExport('svg')">
          {{ i18nStore.tl('SVG 矢量图', 'SVG vector') }}
        </button>
      </div>

      <!-- 选中节点的详情：图上一眼看不全的信息（meta）放这里 -->
      <div v-if="selectedNode" class="pg-detail" @pointerdown.stop @click.stop @wheel.stop>
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
 *  1. 默认用分层布局（layout.ts）：这份数据本身是分层的，
 *     而且分层布局每次算出来的坐标都一样，同一个拓扑不会看起来像变了一个网。
 *     某一层特别长（几十个客户端）时，可用工具条上的"力导向"手动散开一次 ——
 *     它以当前坐标为初值，所以结果同样可复现；
 *  2. 离线客户端画成**灰色虚线空心**，与在线实心一眼可分 —— 否则一片点看不出谁掉线；
 *  3. 点击节点在下方列出 meta：图上放不下的地址/密钥指纹等信息集中在这里。
 *
 * 画布交互（CS 拓扑图同款）：拖节点、拖空白平移、滚轮缩放、右键节点出菜单
 * （打开会话 / 跳对应管理页 / 仅看关联 / 居中 / 复制）。手动位置与分层布局互不影响：
 * 前者存在 localStorage，清掉即回到分层结果。
 */
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import PageHeader from '@/components/Common/PageHeader.vue'
import ClientApi from '@/api/clients'
import { useI18nStore } from '@/stores/i18n'
import {
  computeLayout,
  forceLayout,
  graphBounds,
  isOnline,
  LABEL_DX,
  NODE_RADIUS,
  TYPE_COLORS,
  TYPE_LABELS,
  type GraphEdge,
  type GraphNode,
  type LaidOutEdge,
} from './layout'
import { OS_GLYPHS, osGlyphKey } from './os-glyph'
import { portOf, TARGET_GLYPHS, targetGlyphKey, targetService } from './target-glyph'
import {
  buildExportSvg,
  downloadBlob,
  exportFilename,
  svgToPngBlob,
  type ExportTheme,
} from './export'

const i18nStore = useI18nStore()
const router = useRouter()

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
/** 离线主机统一用这个灰：节点圈、标签、连线、系统图标都靠它区分 */
const OFFLINE_GRAY = '#9ca3af'
const MIN_K = 0.3
const MAX_K = 2.5
const hasFitted = ref(false)
const canvasSize = reactive({ w: 0, h: 0 })
let canvasObserver: ResizeObserver | null = null

// 聚焦：只显示一个节点和它的直接邻居（大图里找"这个客户端连到哪"最快的方式）
const focused = ref<string | null>(null)

// 节点右键菜单（fixed 定位，坐标是视口坐标）
const menu = reactive<{ open: boolean; x: number; y: number; nodeId: string | null }>({
  open: false,
  x: 0,
  y: 0,
  nodeId: null,
})

// 导出菜单（同一个模式，锚在工具条按钮下方）
const exportMenu = reactive({ open: false, x: 0, y: 0 })

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

// 聚焦集合：节点自身 + 直接邻居；null = 不聚焦
const focusSet = computed<Set<string> | null>(() => {
  const id = focused.value
  if (!id) return null
  const set = new Set<string>([id])
  for (const e of layout.value.edges) {
    if (e.source === id) set.add(e.target)
    if (e.target === id) set.add(e.source)
  }
  return set
})

const visibleNodes = computed(() => {
  const set = focusSet.value
  return set ? layout.value.nodes.filter((n) => set.has(n.id)) : layout.value.nodes
})

const focusedLabel = computed(
  () => layout.value.nodes.find((n) => n.id === focused.value)?.label || '',
)

const posOf = computed(() => {
  const m = new Map<string, { x: number; y: number }>()
  for (const n of visibleNodes.value) {
    const o = manualPos[n.id]
    m.set(n.id, o ? { x: o.x, y: o.y } : { x: n.x, y: n.y })
  }
  return m
})

const viewNodes = computed(() => visibleNodes.value.map((n) => ({ ...n, ...(posOf.value.get(n.id) || n) })))

/** 离线客户端 id 集合：节点置灰、连线画虚线都看它 */
const offlineIds = computed(() => {
  const s = new Set<string>()
  for (const n of visibleNodes.value) if (!isOnline(n)) s.add(n.id)
  return s
})

const viewEdges = computed(() =>
  layout.value.edges
    .filter((e) => posOf.value.has(e.source) && posOf.value.has(e.target))
    .map((e) => {
      const sp = posOf.value.get(e.source)!
      const tp = posOf.value.get(e.target)!
      return {
        ...e,
        x1: sp.x,
        y1: sp.y,
        x2: tp.x,
        y2: tp.y,
        // 连到离线主机的边画虚线：只看线也知道那台掉线了
        offline: offlineIds.value.has(e.source) || offlineIds.value.has(e.target),
      }
    }),
)

/**
 * 边标签：后端给了 label 才画；缩得太小（<55%）时文字会糊成一片。
 * 这里用 hidden 标记而不是过滤掉 —— 导出的 SVG 是克隆 DOM 的，过滤掉就永远导不出标签。
 */
const labeledEdges = computed(() => {
  const hidden = view.k <= 0.55
  return viewEdges.value.filter((e) => !!e.label).map((e) => ({ ...e, hidden }))
})

/** 节点填充：在线=类型色实心；离线=空心（灰虚线描边），一眼看出掉线 */
function nodeFill(nd: { type: string; online?: string }) {
  return isOnline(nd as GraphNode) ? TYPE_COLORS[nd.type] || '#6b7280' : 'transparent'
}

function nodeStroke(nd: { type: string; online?: string }) {
  return isOnline(nd as GraphNode) ? TYPE_COLORS[nd.type] || '#6b7280' : OFFLINE_GRAY
}

/** 适配视图：把整张图（含右侧标签宽度）装进画布并居中 */
function fitView() {
  const el = svgEl.value
  const ns = viewNodes.value
  if (!el || !ns.length) return
  const rect = el.getBoundingClientRect()
  const w = rect.width || canvasSize.w
  const h = rect.height || canvasSize.h
  if (!w || !h) return
  // 包围盒与导出共用同一套算法（标签宽度按字数估），否则"看到的"和"导出的"会差一截
  const b = graphBounds(ns, 26)
  const bw = Math.max(1, b.maxX - b.minX)
  const bh = Math.max(1, b.maxY - b.minY)
  const pad = 26
  const k = Math.min((w - pad * 2) / bw, (h - pad * 2) / bh)
  view.k = Math.min(MAX_K, Math.max(MIN_K, Math.min(k, 1.4)))
  view.x = w / 2 - ((b.minX + b.maxX) / 2) * view.k
  view.y = h / 2 - ((b.minY + b.maxY) / 2) * view.k
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

/**
 * 力导向整理：以当前坐标为初值散开一次（结果可复现），保留分层阅读方向。
 * 只对当前可见节点做 —— 聚焦状态下整理的是子图。
 */
function applyForce() {
  if (!visibleNodes.value.length) return
  const res = forceLayout(visibleNodes.value, layout.value.edges)
  for (const [id, p] of res) manualPos[id] = { x: Math.round(p.x), y: Math.round(p.y) }
  savePositions()
  fitView()
}

/** 把某个节点移到画布中心（缩放不变） */
function centerOn(id: string) {
  const p = posOf.value.get(id)
  const r = svgEl.value?.getBoundingClientRect()
  const w = r?.width || canvasSize.w
  const h = r?.height || canvasSize.h
  if (!p || !w || !h) return
  view.x = w / 2 - p.x * view.k
  view.y = h / 2 - p.y * view.k
}

// ---- 焦点 / 右键菜单 ----

function toggleFocus(id: string) {
  focused.value = focused.value === id ? null : id
  selected.value = id
  nextTick(fitView)
}

function clearFocus() {
  focused.value = null
  nextTick(fitView)
}

interface MenuItem {
  key: string
  label: string
}

const menuNode = computed(() => nodes.value.find((n) => n.id === menu.nodeId) || null)

const menuItems = computed<MenuItem[]>(() => {
  const nd = menuNode.value
  if (!nd) return []
  const zh = (a: string, b: string) => i18nStore.tl(a, b)
  const items: MenuItem[] = []
  if (nd.type === 'client' && nd.meta?.uid) {
    items.push({ key: 'session', label: zh('打开会话', 'Open session') })
  }
  if (nd.type === 'listener') items.push({ key: 'listeners', label: zh('监听管理', 'Listeners page') })
  if (nd.type === 'tunnel') items.push({ key: 'tunnel', label: zh('隧道代理', 'Tunnels page') })
  items.push({
    key: 'focus',
    label: focused.value === nd.id ? zh('退出聚焦', 'Exit focus') : zh('仅看关联', 'Focus on this'),
  })
  items.push({ key: 'center', label: zh('居中显示', 'Center') })
  items.push({ key: 'copyLabel', label: zh('复制名称', 'Copy label') })
  if (nd.sub_label) items.push({ key: 'copySub', label: zh('复制地址', 'Copy address') })
  return items
})

function openMenu(e: MouseEvent, nd: { id: string }) {
  menu.nodeId = nd.id
  selected.value = nd.id
  // 菜单用 fixed 定位（画布是 overflow:hidden，绝对定位会被裁掉）；
  // 先按条目数估高度再夹到视口内，贴着屏幕右下角打开时不会溢出
  const h = Math.max(1, menuItems.value.length) * 28 + 8
  menu.x = Math.max(4, Math.min(e.clientX, window.innerWidth - 156))
  menu.y = Math.max(4, Math.min(e.clientY, window.innerHeight - h))
  menu.open = true
  window.addEventListener('pointerdown', onWindowPointerDown, true)
  window.addEventListener('keydown', onWindowKeyDown)
}

function closeMenu() {
  menu.open = false
  menu.nodeId = null
  exportMenu.open = false
  window.removeEventListener('pointerdown', onWindowPointerDown, true)
  window.removeEventListener('keydown', onWindowKeyDown)
}

/** 导出菜单：锚在按钮下方（fixed，画布 overflow:hidden 裁不到它） */
function openExportMenu(e: MouseEvent) {
  const r = (e.currentTarget as HTMLElement).getBoundingClientRect()
  exportMenu.x = Math.max(4, Math.min(r.left, window.innerWidth - 176))
  exportMenu.y = Math.min(r.bottom + 4, window.innerHeight - 70)
  exportMenu.open = true
  window.addEventListener('pointerdown', onWindowPointerDown, true)
  window.addEventListener('keydown', onWindowKeyDown)
}

function readExportTheme(): Partial<ExportTheme> {
  const el = svgEl.value
  if (!el) return {}
  const cs = getComputedStyle(el)
  const v = (name: string, fb: string) => (cs.getPropertyValue(name) || '').trim() || fb
  return {
    // 卡片底色/正文色跟着主题走，导出图在暗色主题下才不会"白底黑字变白底白字"
    bg: v('--vs-card-bg', '#ffffff'),
    text: v('--vs-text', '#1f2937'),
  }
}

/**
 * 导出整张图（与当前缩放/平移无关）：
 * 缩放状态导出会得到一张小得看不清的图，所以按包围盒重新摆正。
 * 聚焦状态下导出的是当前可见的子图 —— 那是用户正看的东西。
 */
async function doExport(kind: 'png' | 'svg') {
  closeMenu()
  const svg = svgEl.value
  if (!svg || !viewNodes.value.length) return
  try {
    const { text, width, height } = buildExportSvg(svg, graphBounds(viewNodes.value, 0), {
      theme: readExportTheme(),
    })
    if (kind === 'svg') {
      downloadBlob(new Blob([text], { type: 'image/svg+xml;charset=utf-8' }), exportFilename('svg'))
    } else {
      downloadBlob(await svgToPngBlob(text, width, height, 2), exportFilename('png'))
    }
    ElMessage.success(
      i18nStore.tl(`已导出 ${kind.toUpperCase()}`, `Exported ${kind.toUpperCase()}`),
    )
  } catch {
    ElMessage.error(i18nStore.tl('导出失败', 'Export failed'))
  }
}

function onWindowPointerDown(e: PointerEvent) {
  // 捕获阶段先于菜单自身的 click：点在菜单里不算"点外面"
  if ((e.target as Element)?.closest?.('.pg-menu')) return
  closeMenu()
}

function onWindowKeyDown(e: KeyboardEvent) {
  if (e.key === 'Escape') closeMenu()
}

async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(i18nStore.tl('已复制', 'Copied'))
  } catch {
    ElMessage.warning(i18nStore.tl('复制失败，请手动选择', 'Copy failed, select manually'))
  }
}

function runMenu(key: string) {
  const nd = menuNode.value
  closeMenu()
  if (!nd) return
  if (key === 'session' && nd.meta?.uid) {
    router.push(`/client/${nd.meta.uid}/console`)
    return
  }
  if (key === 'listeners') {
    router.push('/listener')
    return
  }
  if (key === 'tunnel') {
    router.push('/tunnel')
    return
  }
  if (key === 'focus') {
    toggleFocus(nd.id)
    return
  }
  if (key === 'center') {
    centerOn(nd.id)
    return
  }
  if (key === 'copyLabel') {
    copyText(nd.label)
    return
  }
  if (key === 'copySub' && nd.sub_label) copyText(nd.sub_label)
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
  closeMenu()
})

const legendTypes = ['server', 'listener', 'client', 'tunnel', 'target']
const OS_KEYS = ['windows', 'linux', 'darwin', 'other'] as const
const TARGET_KEYS = ['windows', 'terminal', 'web', 'database', 'generic'] as const

/**
 * 节点图标：客户端按上报的系统，目标按端口推测（地址里带端口才有意义）。
 * 其它类型（服务端/监听器/隧道）不画 —— 它们没有"是什么系统/什么服务"这层信息。
 */
function nodeGlyph(nd: GraphNode): string | null {
  if (nd.type === 'client') return OS_GLYPHS[osGlyphKey(nd.meta?.os)].d
  if (nd.type === 'target') return TARGET_GLYPHS[targetGlyphKey(nd.label)].d
  return null
}

const clientCount = computed(() => nodes.value.filter((n) => n.type === 'client').length)
const onlineCount = computed(() => nodes.value.filter((n) => n.type === 'client' && isOnline(n)).length)

const selectedNode = computed(() => nodes.value.find((n) => n.id === selected.value) || null)
const metaRows = computed(() => {
  const nd = selectedNode.value
  const rows = Object.entries(nd?.meta || {})
    .filter(([, v]) => v !== '')
    .map(([k, v]) => ({ k, v: String(v) }))
  // 目标节点的服务名是从端口推的，不是上报值 —— 标出来，别让推测看起来像事实
  if (nd?.type === 'target') {
    const svc = targetService(nd.label)
    if (svc) {
      rows.unshift({
        k: 'service',
        v: `${i18nStore.tl(svc.zh, svc.en)}（${i18nStore.tl('按端口推测', 'inferred from port')}）`,
      })
    }
    if (portOf(nd.label) === null) rows.unshift({ k: 'port', v: i18nStore.tl('地址里没有端口', 'no port in address') })
  }
  return rows
})

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
    // 聚焦/右键菜单同理：目标节点没了就退出，否则只剩一个空画布
    if (focused.value && !nodes.value.some((n) => n.id === focused.value)) {
      focused.value = null
    }
    if (menu.open && !nodes.value.some((n) => n.id === menu.nodeId)) {
      closeMenu()
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
  flex-direction: column;
  gap: 4px;
  font-size: 12px;
  opacity: 0.85;
}

.pg-legend-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 14px;
}

/* 分组小标题：图标行里"客户端按系统 / 目标按端口"两组的说明 */
.pg-legend-cap {
  opacity: 0.6;
}
.pg-legend-item {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}
.pg-hint {
  margin-left: auto;
  opacity: 0.5;
}
.pg-dot {
  width: 9px;
  height: 9px;
  border-radius: 50%;
  display: inline-block;
}
.pg-dot-offline {
  background: transparent;
  border: 2px dashed #9ca3af;
}

/* 图例里的系统图标（与节点上的同一份 path） */
.pg-legend-glyph {
  fill: currentColor;
  opacity: 0.7;
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

.pg-focus-chip {
  color: var(--cs-active, #8b9dc3);
}

/* 节点右键菜单 */
.pg-menu {
  position: fixed;
  z-index: 30;
  min-width: 148px;
  padding: 4px;
  border: 1px solid var(--cs-card-border, #333);
  border-radius: 6px;
  background: var(--cs-panel, rgba(20, 24, 31, 0.96));
  box-shadow: 0 6px 18px rgba(0, 0, 0, 0.28);
}

.pg-menu-item {
  display: block;
  width: 100%;
  height: 26px;
  padding: 0 10px;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: var(--cs-text, inherit);
  font-size: 12px;
  text-align: left;
  cursor: pointer;
}

.pg-menu-item:hover {
  background: var(--cs-hover);
  color: var(--cs-active);
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

/* 连到离线主机的边：虚线 + 更淡（必须排在 .pg-edge-active 之后，同权重后者生效） */
.pg-edge-offline {
  stroke: #cbd5e1;
  stroke-dasharray: 5 4;
  opacity: 0.9;
}

/* 边标签描一圈底色，压在线上也读得清 */
.pg-edge-label.is-hidden {
  opacity: 0;
}

.pg-edge-label {
  font-size: 10px;
  fill: var(--vs-text, #e6e6e6);
  opacity: 0.7;
  text-anchor: middle;
  paint-order: stroke;
  stroke: var(--cs-bg, #111);
  stroke-width: 3px;
  stroke-linejoin: round;
  pointer-events: none;
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
/* 离线主机整体置灰：圈是灰虚线空心、标签灰、系统图标灰 */
.pg-node-offline .pg-node-label {
  fill: #9ca3af;
}

.pg-node-offline .pg-node-sub {
  fill: #9ca3af;
  opacity: 0.8;
}

.pg-node-offline .pg-node-glyph {
  fill: #9ca3af;
}

.pg-node-offline:hover .pg-node-halo {
  opacity: 0.1;
}

/* 节点里的图标（系统/服务）：在线白（压在类型色实心圆上） */
.pg-node-glyph {
  fill: #ffffff;
  pointer-events: none;
}
.pg-node-selected .pg-node-label {
  font-weight: 700;
}
/* 详情浮层：放在画布上而不是画布下面 —— 否则一条 meta 就能把画布挤扁 */
.pg-detail {
  position: absolute;
  left: 12px;
  bottom: 12px;
  z-index: 2;
  max-width: min(440px, 62%);
  max-height: 62%;
  overflow: auto;
  padding: 8px 10px;
  border: 1px solid var(--cs-card-border, #333);
  border-radius: 6px;
  background: var(--cs-panel, rgba(20, 24, 31, 0.94));
  box-shadow: 0 4px 14px rgba(0, 0, 0, 0.22);
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
