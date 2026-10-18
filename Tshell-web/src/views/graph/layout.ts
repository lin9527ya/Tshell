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
