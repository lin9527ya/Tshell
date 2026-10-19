/**
 * 拓扑图布局（阶段三 §3.11）。
 *
 * 为什么自己算布局而不是引第三方图库：
 * - 这份数据的形状是**确定的层级**（server → listener → client → tunnel → target），
 *   不是需要力导向去"发现"结构的任意图。分层布局算出来比力导向更稳定、更可读，
 *   而且**每次刷新不会跳**（力导向的随机初值会让同一份数据画得不一样）。
 * - 不引入依赖：这个项目对前端依赖一向克制，为一个视图拉进一个几百 KB 的
 *   图库不划算。
 *
 * 纯函数、无 DOM 依赖 —— 布局正确性可以直接单测（见 layout.test.ts）。
 */

/** 服务端返回的节点（与 Go 侧 GraphNode 对齐） */
export interface GraphNode {
  id: string
  type: 'server' | 'listener' | 'client' | 'tunnel' | 'target' | string
  label: string
  sub_label?: string
  /** "1"=在线 "2"=离线（仅 client 节点带） */
  online?: string
  meta?: Record<string, string>
}

/** 服务端返回的边 */
export interface GraphEdge {
  id: string
  source: string
  target: string
  label?: string
}

/** 布局后的节点（含坐标与层级） */
export interface LaidOutNode extends GraphNode {
  x: number
  y: number
  /** 层级序号：0=server，1=listener，2=client，3=tunnel，4=target */
  layer: number
}

/** 布局后的边（含起止坐标，便于直接画线） */
export interface LaidOutEdge extends GraphEdge {
  x1: number
  y1: number
  x2: number
  y2: number
}

export interface GraphLayout {
  nodes: LaidOutNode[]
  edges: LaidOutEdge[]
  width: number
  height: number
}

/** 层级顺序：数据本身是分层的，按这个顺序横向排开 */
export const LAYER_ORDER = ['server', 'listener', 'client', 'tunnel', 'target'] as const

const COLUMN_WIDTH = 210
const ROW_HEIGHT = 68
const MARGIN_X = 90
const MARGIN_Y = 48
/** 同一层节点过多时换列，避免单列长到没法看 */
const MAX_PER_COLUMN = 12

/**
 * layerOf 取节点层级。未知类型排到最后而不是排到 0 ——
 * 排到 0 会和 server 混在一列，看起来像"多了几个服务器"。
 */
export function layerOf(type: string): number {
  const i = (LAYER_ORDER as readonly string[]).indexOf(type)
  return i >= 0 ? i : LAYER_ORDER.length
}

/**
 * computeLayout 分层布局。
 *
 * 坐标约定：x 随层级递增（从左到右），y 在同层内自上而下排列。
 * 同层节点按 (label, id) 排序，保证**同一份数据每次得到同样的图**。
 */
export function computeLayout(nodes: GraphNode[], edges: GraphEdge[]): GraphLayout {
  const byLayer = new Map<number, GraphNode[]>()
  for (const n of nodes) {
    const l = layerOf(n.type)
    const list = byLayer.get(l)
    if (list) list.push(n)
    else byLayer.set(l, [n])
  }

  const laidOut: LaidOutNode[] = []
  const pos = new Map<string, { x: number; y: number }>()
  let maxColumnCount = 1

  // 层号可能不连续（数据里没有 tunnel 时就是 0,1,2,4）——
  // 按"实际出现的层"重新编号，否则会空出一整列
  const presentLayers = [...byLayer.keys()].sort((a, b) => a - b)
  presentLayers.forEach((layerKey, columnIndex) => {
    const group = byLayer.get(layerKey)!
    group.sort((a, b) => a.label.localeCompare(b.label) || a.id.localeCompare(b.id))

    // 超长的一层折成多列：单列 200 个节点画出来是一根没法看的线
    const columns = Math.ceil(group.length / MAX_PER_COLUMN)
    maxColumnCount = Math.max(maxColumnCount, columns)

    group.forEach((n, i) => {
      const col = Math.floor(i / MAX_PER_COLUMN)
      const row = i % MAX_PER_COLUMN
      const x = MARGIN_X + (columnIndex + col) * COLUMN_WIDTH
      const y = MARGIN_Y + row * ROW_HEIGHT
      pos.set(n.id, { x, y })
      laidOut.push({ ...n, x, y, layer: layerKey })
    })
  })

  const laidEdges: LaidOutEdge[] = []
  for (const e of edges) {
    const a = pos.get(e.source)
    const b = pos.get(e.target)
    // 端点缺失的边直接丢弃：服务端可能返回指向已被删除客户端的边，
    // 画一条悬空的线比不画更让人困惑
    if (!a || !b) continue
    laidEdges.push({ ...e, x1: a.x, y1: a.y, x2: b.x, y2: b.y })
  }

  const maxRows = Math.max(
    MAX_PER_COLUMN,
    ...presentLayers.map((l) => Math.min(byLayer.get(l)!.length, MAX_PER_COLUMN)),
  )
  return {
    nodes: laidOut,
    edges: laidEdges,
    width: MARGIN_X * 2 + (presentLayers.length + maxColumnCount - 1) * COLUMN_WIDTH,
    height: MARGIN_Y * 2 + maxRows * ROW_HEIGHT,
  }
}

/** 节点是否在线（只有 client 节点有 online 字段；其它类型一律视为在线） */
export function isOnline(n: GraphNode): boolean {
  if (n.type !== 'client') return true
  return n.online === '1'
}

/** 各类型节点的显示名（图例用） */
export const TYPE_LABELS: Record<string, { zh: string; en: string }> = {
  server: { zh: '服务端', en: 'Server' },
  listener: { zh: '监听器', en: 'Listener' },
  client: { zh: '客户端', en: 'Client' },
  tunnel: { zh: '隧道', en: 'Tunnel' },
  target: { zh: '目标', en: 'Target' },
}

/** 各类型节点的主色（与暗色主题下的对比度一起考虑过） */
export const TYPE_COLORS: Record<string, string> = {
  server: '#7c5cff',
  listener: '#2f9e6e',
  client: '#3b82f6',
  tunnel: '#d97706',
  target: '#6b7280',
}

// ---------------------------------------------------------------------------
// 力导向布局（可选，非默认）
//
// 默认仍是分层布局：同一份数据每次画得一样，server→target 的阅读方向稳定。
// 但分层布局在"某一层特别长"时（例如 40 个客户端）会挤成一根长列，节点标签互相压住。
// 力导向作为**手动触发的一次性整理**：以当前坐标为初值（因此结果可复现，不是每次刷新乱跳），
// 让节点按斥力散开、按边收紧，同时用较弱的"层牵引"把节点拉回它所在的列，
// 保留从左到右的阅读方向。
// ---------------------------------------------------------------------------

export interface ForceLayoutOptions {
  /** 迭代次数上限（节点多时会自动下调，见 iterationsFor） */
  iterations?: number
  /** 节点间斥力强度 */
  repulsion?: number
  /** 边的弹簧强度 */
  spring?: number
  /** 边的理想长度 */
  restLength?: number
  /** 拉回本层列位的强度（0 = 完全自由的力导向） */
  layerPull?: number
  /** 速度衰减：越小越稳、越慢 */
  damping?: number
  /** 单步最大位移，防止初值极端时"炸开" */
  maxStep?: number
}

/** 节点多时下调迭代数：斥力是 O(n²)，200 节点 × 300 次就是 600 万次两两计算 */
function iterationsFor(n: number, want: number): number {
  if (n <= 0) return 0
  return Math.max(40, Math.min(want, Math.floor(40000 / n)))
}

/**
 * forceLayout 返回**全部输入节点**的新坐标（不修改入参）。
 *
 * @param laid   分层布局的结果（同时提供初值与"本层列位"）
 * @param edges  边；端点不在 laid 里的边忽略（与 computeLayout 的丢弃规则一致）
 */
export function forceLayout(
  laid: LaidOutNode[],
  edges: GraphEdge[],
  opts: ForceLayoutOptions = {},
): Map<string, { x: number; y: number }> {
  const wantIter = opts.iterations ?? 300
  const repulsion = opts.repulsion ?? 45000
  const spring = opts.spring ?? 0.02
  const restLength = opts.restLength ?? 170
  const layerPull = opts.layerPull ?? 0.06
  const damping = opts.damping ?? 0.82
  const maxStep = opts.maxStep ?? 24

  const out = new Map<string, { x: number; y: number }>()
  if (!laid.length) return out

  interface P { id: string; x: number; y: number; vx: number; vy: number; col: number }
  const ps: P[] = laid.map((n) => ({ id: n.id, x: n.x, y: n.y, vx: 0, vy: 0, col: n.x }))
  const byId = new Map<string, P>(ps.map((p) => [p.id, p]))
  // 有效边：两端都在图里（否则弹簧会拉向一个不存在的点）
  const links: { a: P; b: P }[] = []
  for (const e of edges) {
    const a = byId.get(e.source)
    const b = byId.get(e.target)
    if (a && b) links.push({ a, b })
  }

  // 纵向重心：只做很弱的回收，避免整张图慢慢飘出视野
  const centerY = ps.reduce((s, p) => s + p.y, 0) / ps.length
  const iterations = iterationsFor(ps.length, wantIter)

  for (let it = 0; it < iterations; it++) {
    // 1) 两两斥力
    for (let i = 0; i < ps.length; i++) {
      const a = ps[i]
      for (let j = i + 1; j < ps.length; j++) {
        const b = ps[j]
        let dx = b.x - a.x
        let dy = b.y - a.y
        let d2 = dx * dx + dy * dy
        if (d2 < 1e-6) {
          // 完全重合：给一个由下标决定的确定性偏移，保证结果可复现
          dx = ((i % 3) - 1) || 1
          dy = ((j % 3) - 1) || -1
          d2 = dx * dx + dy * dy
        }
        const d = Math.sqrt(d2)
        const f = repulsion / d2
        const ux = dx / d
        const uy = dy / d
        a.vx -= ux * f
        a.vy -= uy * f
        b.vx += ux * f
        b.vy += uy * f
      }
    }

    // 2) 边的弹簧
    for (const { a, b } of links) {
      const dx = b.x - a.x
      const dy = b.y - a.y
      const d = Math.sqrt(dx * dx + dy * dy)
      if (d < 1e-6) continue
      const f = spring * (d - restLength)
      const ux = dx / d
      const uy = dy / d
      a.vx += ux * f
      a.vy += uy * f
      b.vx -= ux * f
      b.vy -= uy * f
    }

    // 3) 层牵引（水平）+ 纵向回收（很弱）
    for (const p of ps) {
      p.vx += (p.col - p.x) * layerPull
      p.vy += (centerY - p.y) * layerPull * 0.12
    }

    // 4) 积分 + 冷却
    for (const p of ps) {
      p.x += clampStep(p.vx, maxStep)
      p.y += clampStep(p.vy, maxStep)
      p.vx *= damping
      p.vy *= damping
    }
  }

  for (const p of ps) out.set(p.id, { x: p.x, y: p.y })
  return out
}

function clampStep(v: number, max: number): number {
  if (!Number.isFinite(v)) return 0
  return Math.max(-max, Math.min(max, v))
}

// ---------------------------------------------------------------------------
// 包围盒：画布适配（fitView）与导出共用同一套算法，避免"导出比看到的少一截"
// ---------------------------------------------------------------------------

export interface GraphBounds {
  minX: number
  minY: number
  maxX: number
  maxY: number
}

/** 估算文本宽度：CJK 按整字宽算，其余按 0.56 字宽 —— 只为算包围盒，不需要精确 */
export function estimateTextWidth(text: string, fontSize: number): number {
  let w = 0
  for (const ch of text) {
    const cp = ch.codePointAt(0) ?? 0
    w += cp > 0x2e80 ? fontSize : fontSize * 0.56
  }
  return w
}

/** 节点圆的半径（画布与导出共用） */
export const NODE_RADIUS = 9
/** 标签相对节点的偏移（画布与导出共用） */
export const LABEL_DX = 16

/**
 * graphBounds 算节点（含右侧标签）的包围盒。
 * 右侧必须留出标签宽度：不留的话适配视图时标签会被裁掉半截。
 */
export function graphBounds(
  nodes: { x: number; y: number; label: string; sub_label?: string }[],
  pad = 12,
): GraphBounds {
  if (!nodes.length) return { minX: 0, minY: 0, maxX: 1, maxY: 1 }
  let minX = Infinity
  let minY = Infinity
  let maxX = -Infinity
  let maxY = -Infinity
  for (const n of nodes) {
    const labelW = Math.max(
      estimateTextWidth(n.label || '', 12),
      n.sub_label ? estimateTextWidth(n.sub_label, 10) : 0,
    )
    minX = Math.min(minX, n.x - NODE_RADIUS - 4)
    maxX = Math.max(maxX, n.x + LABEL_DX + labelW)
    minY = Math.min(minY, n.y - NODE_RADIUS - 4)
    maxY = Math.max(maxY, n.y + NODE_RADIUS + 12)
  }
  return { minX: minX - pad, minY: minY - pad, maxX: maxX + pad, maxY: maxY + pad }
}
