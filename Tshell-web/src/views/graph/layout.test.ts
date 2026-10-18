import { describe, expect, it } from 'vitest'
import {
  computeLayout,
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
