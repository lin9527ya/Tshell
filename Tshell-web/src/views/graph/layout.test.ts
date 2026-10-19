import { describe, expect, it } from 'vitest'
import {
  computeLayout,
  estimateTextWidth,
  forceLayout,
  graphBounds,
  isOnline,
  layerOf,
  LAYER_ORDER,
  type GraphEdge,
  type GraphNode,
} from './layout'

function n(id: string, type: string, label = id, extra: Partial<GraphNode> = {}): GraphNode {
  return { id, type, label, ...extra }
}

describe('layerOf', () => {
  it('按 server→listener→client→tunnel→target 排序', () => {
    expect(layerOf('server')).toBe(0)
    expect(layerOf('listener')).toBe(1)
    expect(layerOf('client')).toBe(2)
    expect(layerOf('tunnel')).toBe(3)
    expect(layerOf('target')).toBe(4)
  })

  it('未知类型排到最后而不是排到 0', () => {
    // 排到 0 会和 server 混在一列，看起来像"多了几个服务器"
    expect(layerOf('unknown-type')).toBe(LAYER_ORDER.length)
    expect(layerOf('unknown-type')).toBeGreaterThan(layerOf('target'))
  })
})

describe('computeLayout', () => {
  it('x 随层级递增（从左到右）', () => {
    const { nodes } = computeLayout(
      [n('s', 'server'), n('l', 'listener'), n('c', 'client'), n('t', 'tunnel'), n('g', 'target')],
      [],
    )
    const x = (id: string) => nodes.find((v) => v.id === id)!.x
    expect(x('s')).toBeLessThan(x('l'))
    expect(x('l')).toBeLessThan(x('c'))
    expect(x('c')).toBeLessThan(x('t'))
    expect(x('t')).toBeLessThan(x('g'))
  })

  it('同一份数据每次得到同样的图（按 label/id 稳定排序）', () => {
    const nodes = [n('b', 'client', 'bravo'), n('a', 'client', 'alpha'), n('c', 'client', 'charlie')]
    const first = computeLayout(nodes, []).nodes.map((v) => v.id)
    const shuffled = computeLayout([nodes[2], nodes[0], nodes[1]], []).nodes.map((v) => v.id)
    expect(first).toEqual(shuffled)
    expect(first).toEqual(['a', 'b', 'c'])
  })

  it('层号不连续时按实际出现的层重新编号，不空出整列', () => {
    // 数据里没有 tunnel：层号是 0,1,2,4
    const { nodes } = computeLayout([n('s', 'server'), n('c', 'client'), n('g', 'target')], [])
    const xs = [...new Set(nodes.map((v) => v.x))].sort((a, b) => a - b)
    // 三列应该连续相邻，而不是中间空一列
    expect(xs).toHaveLength(3)
    expect(xs[1] - xs[0]).toBe(xs[2] - xs[1])
  })

  it('同一层超过上限时折成多列，不排成一根长线', () => {
    const many = Array.from({ length: 30 }, (_, i) => n(`c${i}`, 'client', `c${String(i).padStart(2, '0')}`))
    const { nodes } = computeLayout(many, [])
    const xs = [...new Set(nodes.map((v) => v.x))]
    expect(xs.length).toBeGreaterThan(1)
    // 每个节点的 y 都在单列上限之内
    const ys = nodes.map((v) => v.y)
    expect(Math.max(...ys)).toBeLessThan(48 + 12 * 68)
  })

  it('丢弃端点缺失的边（不画悬空线）', () => {
    const edges: GraphEdge[] = [
      { id: 'e1', source: 's', target: 'l' },
      { id: 'e2', source: 's', target: 'ghost' }, // ghost 不在节点里
    ]
    const { edges: out } = computeLayout([n('s', 'server'), n('l', 'listener')], edges)
    expect(out.map((e) => e.id)).toEqual(['e1'])
  })

  it('边带上起止坐标', () => {
    const { nodes, edges } = computeLayout(
      [n('s', 'server'), n('l', 'listener')],
      [{ id: 'e1', source: 's', target: 'l' }],
    )
    const s = nodes.find((v) => v.id === 's')!
    const l = nodes.find((v) => v.id === 'l')!
    expect(edges[0]).toMatchObject({ x1: s.x, y1: s.y, x2: l.x, y2: l.y })
  })

  it('空输入不报错，尺寸仍为正（避免 0 宽 SVG）', () => {
    const { nodes, edges, width, height } = computeLayout([], [])
    expect(nodes).toEqual([])
    expect(edges).toEqual([])
    expect(width).toBeGreaterThan(0)
    expect(height).toBeGreaterThan(0)
  })
})

describe('isOnline', () => {
  it('client 节点按 online 字段判定', () => {
    expect(isOnline(n('c1', 'client', 'c1', { online: '1' }))).toBe(true)
    expect(isOnline(n('c2', 'client', 'c2', { online: '2' }))).toBe(false)
    expect(isOnline(n('c3', 'client', 'c3'))).toBe(false)
  })

  it('非 client 节点一律视为在线（它们没有 online 字段）', () => {
    expect(isOnline(n('s', 'server'))).toBe(true)
    expect(isOnline(n('l', 'listener'))).toBe(true)
    expect(isOnline(n('t', 'tunnel'))).toBe(true)
  })
})

describe('forceLayout', () => {
  // 形参不能叫 n：会遮住上面的节点工厂 n()
  const chain = (count: number) => {
    const nodes: GraphNode[] = [{ id: 's', type: 'server', label: 'S' }]
    const edges: GraphEdge[] = []
    for (let i = 0; i < count; i++) {
      nodes.push(n(`c${i}`, 'client', `C${String(i).padStart(2, '0')}`))
      edges.push({ id: `e${i}`, source: `c${i}`, target: 's' })
    }
    return { nodes, edges }
  }

  it('以当前坐标为初值：同一份输入两次得到同样的结果（不是每次刷新乱跳）', () => {
    const { nodes, edges } = chain(8)
    const laid = computeLayout(nodes, edges).nodes
    const a = forceLayout(laid, edges)
    const b = forceLayout(laid, edges)
    expect([...a.entries()]).toEqual([...b.entries()])
  })

  it('返回全部节点的有限坐标，且不改动入参', () => {
    const { nodes, edges } = chain(6)
    const laid = computeLayout(nodes, edges).nodes
    const before = laid.map((v) => ({ x: v.x, y: v.y }))
    const res = forceLayout(laid, edges)
    expect(res.size).toBe(laid.length)
    for (const p of res.values()) {
      expect(Number.isFinite(p.x)).toBe(true)
      expect(Number.isFinite(p.y)).toBe(true)
    }
    // 入参是布局结果本身，被就地改掉会让"分层布局"跟着一起变
    expect(laid.map((v) => ({ x: v.x, y: v.y }))).toEqual(before)
  })

  it('空输入返回空结果', () => {
    expect(forceLayout([], []).size).toBe(0)
  })

  it('相连的两个节点被弹簧拉近（layerPull=0 时纯看弹簧）', () => {
    // 同层同列、纵向相距很远的两点：只有弹簧能把它们拉近
    const laid = computeLayout([n('a', 'client', 'a'), n('b', 'client', 'b')], []).nodes
    laid[1].y = laid[0].y + 800
    const res = forceLayout(laid, [{ id: 'e', source: 'a', target: 'b' }], { layerPull: 0 })
    const a = res.get('a')!
    const b = res.get('b')!
    expect(Math.hypot(a.x - b.x, a.y - b.y)).toBeLessThan(800)
  })

  it('节点不会重叠（斥力把它们分开）', () => {
    const laid = computeLayout([n('a', 'client', 'a'), n('b', 'client', 'b')], []).nodes
    // 人为把两个节点摆到同一点：斥力必须把它们推开
    laid[1].x = laid[0].x
    laid[1].y = laid[0].y
    const res = forceLayout(laid, [])
    const a = res.get('a')!
    const b = res.get('b')!
    expect(Math.hypot(a.x - b.x, a.y - b.y)).toBeGreaterThan(10)
  })

  it('保留分层阅读方向：server 仍在 target 左边', () => {
    const laid = computeLayout(
      [n('s', 'server', 'S'), n('l', 'listener', 'L'), n('t', 'target', 'T')],
      [
        { id: 'e1', source: 's', target: 'l' },
        { id: 'e2', source: 'l', target: 't' },
      ],
    ).nodes
    const res = forceLayout(laid, [
      { id: 'e1', source: 's', target: 'l' },
      { id: 'e2', source: 'l', target: 't' },
    ])
    expect(res.get('s')!.x).toBeLessThan(res.get('t')!.x)
  })

  it('丢弃端点缺失的边（不把节点拉向不存在的点）', () => {
    const laid = computeLayout([n('a', 'client', 'a')], []).nodes
    const res = forceLayout(laid, [{ id: 'e', source: 'a', target: 'ghost' }])
    expect(res.size).toBe(1)
    expect(Number.isFinite(res.get('a')!.x)).toBe(true)
  })

  it('节点多时自动下调迭代数（斥力是 O(n²)，不能无限迭代）', () => {
    const { nodes, edges } = chain(200)
    const laid = computeLayout(nodes, edges).nodes
    const t0 = Date.now()
    const res = forceLayout(laid, edges)
    // 只是"别卡死主线程"的兜底，阈值放宽到 1.5s
    expect(Date.now() - t0).toBeLessThan(1500)
    expect(res.size).toBe(laid.length)
  })
})

describe('graphBounds', () => {
  it('右侧留出标签宽度（不留的话适配视图会把标签裁掉）', () => {
    const short = graphBounds([{ x: 100, y: 100, label: 'a' }], 0)
    const long = graphBounds([{ x: 100, y: 100, label: 'a-very-long-hostname' }], 0)
    expect(long.maxX).toBeGreaterThan(short.maxX)
  })

  it('中文标签按整字宽估算（单个汉字比单个字母宽）', () => {
    expect(estimateTextWidth('中', 12)).toBeGreaterThan(estimateTextWidth('a', 12))
    expect(estimateTextWidth('中', 12)).toBe(12)
  })

  it('空输入给正尺寸，避免 0 宽画布', () => {
    const b = graphBounds([], 0)
    expect(b.maxX).toBeGreaterThan(b.minX)
    expect(b.maxY).toBeGreaterThan(b.minY)
  })

  it('pad 均匀外扩', () => {
    const a = graphBounds([{ x: 10, y: 10, label: 'x' }], 0)
    const b = graphBounds([{ x: 10, y: 10, label: 'x' }], 20)
    expect(a.minX - b.minX).toBe(20)
    expect(b.maxX - a.maxX).toBe(20)
  })
})
