import { describe, expect, it } from 'vitest'
import { GLYPH_HALF, NODE_RADIUS } from './layout'
import { glyphFit, type Glyph } from './glyph'
import { pathBounds } from './path-bounds'
import { OS_GLYPHS } from './os-glyph'
import { TARGET_GLYPHS } from './target-glyph'
import {
  LISTENER_GLYPHS,
  TUNNEL_GLYPHS,
  listenerGlyphKey,
  nodeGlyph,
  tunnelGlyphKey,
} from './node-glyph'

const ALL: [string, Glyph][] = [
  ...Object.entries(OS_GLYPHS).map(([k, g]) => [`os/${k}`, g] as [string, Glyph]),
  ...Object.entries(TARGET_GLYPHS).map(([k, g]) => [`target/${k}`, g] as [string, Glyph]),
  ...Object.entries(LISTENER_GLYPHS).map(([k, g]) => [`listener/${k}`, g] as [string, Glyph]),
  ...Object.entries(TUNNEL_GLYPHS).map(([k, g]) => [`tunnel/${k}`, g] as [string, Glyph]),
]

describe('图标几何：画到节点圆里必须装得下', () => {
  it('每份图标都有 path 与显示名', () => {
    for (const [name, g] of ALL) {
      expect(g.d.length, name).toBeGreaterThan(10)
      expect(g.vb.split(/\s+/), name).toHaveLength(4)
      expect(g.zh, name).toBeTruthy()
      expect(g.en, name).toBeTruthy()
    }
  })

  it('按 glyphFit 变换后不超出节点圆（最"方"的图形也不戳出来）', () => {
    for (const [name, g] of ALL) {
      const b = pathBounds(g.d, 24, glyphFit(g.vb, GLYPH_HALF))
      expect(b.maxRadius, `${name} 最远点 ${b.maxRadius.toFixed(2)} > 半径 ${NODE_RADIUS}`)
        .toBeLessThanOrEqual(NODE_RADIUS)
    }
  })

  it('变换后大致居中（图标集自带留白，容差放宽到 1.5）', () => {
    for (const [name, g] of ALL) {
      const b = pathBounds(g.d, 24, glyphFit(g.vb, GLYPH_HALF))
      const cx = (b.minX + b.maxX) / 2
      const cy = (b.minY + b.maxY) / 2
      expect(Math.abs(cx), `${name} 水平偏移 ${cx.toFixed(2)}`).toBeLessThanOrEqual(1.5)
      expect(Math.abs(cy), `${name} 垂直偏移 ${cy.toFixed(2)}`).toBeLessThanOrEqual(1.5)
    }
  })

  it('有量级（不是空图形或一条线）', () => {
    for (const [name, g] of ALL) {
      const b = pathBounds(g.d, 24, glyphFit(g.vb, GLYPH_HALF))
      expect(b.maxX - b.minX, name).toBeGreaterThan(6)
      expect(b.maxY - b.minY, name).toBeGreaterThan(6)
    }
  })

  it('path 语法正确（参数个数与命令匹配，写错了在图上就是空白）', () => {
    const ARITY: Record<string, number> = { M: 2, L: 2, H: 1, V: 1, C: 6, S: 4, Q: 4, T: 2, A: 7, Z: 0 }
    for (const [name, g] of ALL) {
      const tokens = g.d.match(/[MmLlHhVvCcSsQqTtAaZz]|-?\d*\.?\d+/g) || []
      let i = 0
      let arity = 0
      let cmd = ''
      while (i < tokens.length) {
        if (/[A-Za-z]/.test(tokens[i])) {
          cmd = tokens[i].toUpperCase()
          arity = ARITY[cmd] ?? -1
          if (arity < 0) throw new Error(`${name}: 未知命令 ${tokens[i]}`)
          i++
          if (arity === 0) continue
        }
        if (i + arity > tokens.length) throw new Error(`${name}: ${cmd} 参数不足`)
        i += arity
      }
      expect(tokens.length, name).toBeGreaterThan(0)
    }
  })
})

describe('listenerGlyphKey', () => {
  it('按监听器类型取图标', () => {
    const cases: [string, string][] = [
      ['tcp', 'tcp'],
      ['kcp', 'kcp'],
      ['websocket', 'websocket'],
      ['http', 'http'],
      ['https', 'https'],
      ['dns', 'dns'],
      ['oss', 'oss'],
    ]
    for (const [type, key] of cases) expect(listenerGlyphKey(type), type).toBe(key)
  })

  it('正向连接（*_forward）与对应协议同图标', () => {
    expect(listenerGlyphKey('tcp_forward')).toBe('tcp')
    expect(listenerGlyphKey('https_forward')).toBe('https')
    expect(listenerGlyphKey('ws_forward')).toBe('websocket')
  })

  it('认不出的类型给广播塔，不返回空', () => {
    expect(listenerGlyphKey('mystery')).toBe('other')
    expect(listenerGlyphKey('')).toBe('other')
    expect(listenerGlyphKey(undefined)).toBe('other')
  })
})

describe('tunnelGlyphKey', () => {
  it('按协议取图标（tcp/udp 都是端口转发）', () => {
    expect(tunnelGlyphKey('socks5')).toBe('socks5')
    expect(tunnelGlyphKey('http')).toBe('http')
    expect(tunnelGlyphKey('tcp')).toBe('port')
    expect(tunnelGlyphKey('udp')).toBe('port')
  })

  it('认不出的协议给路标', () => {
    expect(tunnelGlyphKey('gre')).toBe('other')
    expect(tunnelGlyphKey('')).toBe('other')
  })
})

describe('nodeGlyph', () => {
  const nd = (type: string, label: string, meta?: Record<string, string>) =>
    ({ id: 'x', type, label, meta }) as Parameters<typeof nodeGlyph>[0]

  it('五类节点各自取图标的依据不同', () => {
    expect(nodeGlyph(nd('client', 'WEB-01', { os: 'windows' }))).toBe(OS_GLYPHS.windows)
    expect(nodeGlyph(nd('target', '10.0.0.5:445'))).toBe(TARGET_GLYPHS.windows)
    expect(nodeGlyph(nd('listener', 'websocket'))).toBe(LISTENER_GLYPHS.websocket)
    expect(nodeGlyph(nd('tunnel', 'socks5'))).toBe(TUNNEL_GLYPHS.socks5)
  })

  it('服务端不画图标（它是唯一的"自己"，画了反而像还有同类节点）', () => {
    expect(nodeGlyph(nd('server', 'Tshell Server'))).toBeNull()
  })
})
