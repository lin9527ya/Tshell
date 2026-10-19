import { describe, expect, it } from 'vitest'
import { OS_GLYPHS, osGlyphKey } from './os-glyph'
import { pathBounds } from './path-bounds'

describe('osGlyphKey', () => {
  it('windows / linux 各自归一', () => {
    expect(osGlyphKey('windows')).toBe('windows')
    expect(osGlyphKey('Windows')).toBe('windows')
    expect(osGlyphKey('win10')).toBe('windows')
    expect(osGlyphKey('linux')).toBe('linux')
    expect(osGlyphKey('ubuntu')).toBe('linux')
  })

  it('macOS 的各种写法都归到 darwin', () => {
    expect(osGlyphKey('darwin')).toBe('darwin')
    expect(osGlyphKey('macOS')).toBe('darwin')
    expect(osGlyphKey('osx')).toBe('darwin')
  })

  it('认不出的走 other，而不是返回空（空图标与"没数据"分不清）', () => {
    expect(osGlyphKey('freebsd')).toBe('other')
    expect(osGlyphKey('')).toBe('other')
    expect(osGlyphKey(undefined)).toBe('other')
    expect(osGlyphKey('   ')).toBe('other')
  })
})

describe('OS_GLYPHS', () => {
  it('每种都有可画的 path 与显示名', () => {
    for (const key of ['windows', 'linux', 'darwin', 'other'] as const) {
      const g = OS_GLYPHS[key]
      expect(g.d.length).toBeGreaterThan(10)
      expect(g.d).toMatch(/^[Mm]/)
      expect(g.zh).toBeTruthy()
      expect(g.en).toBeTruthy()
    }
  })

  it('path 语法正确（参数个数与命令匹配，写错了在图上就是空白）', () => {
    // 手写的 d 一旦少个参数，浏览器会整条路径不画 —— 这种错只能靠解析发现
    const ARITY: Record<string, number> = {
      M: 2, L: 2, H: 1, V: 1, C: 6, S: 4, Q: 4, T: 2, A: 7, Z: 0,
    }
    for (const key of ['windows', 'linux', 'darwin', 'other'] as const) {
      const tokens = OS_GLYPHS[key].d.match(/[MmLlHhVvCcSsQqTtAaZz]|-?\d*\.?\d+/g) || []
      let i = 0
      let arity = 0
      let cmd = ''
      let sawMove = false
      while (i < tokens.length) {
        if (/[A-Za-z]/.test(tokens[i])) {
          cmd = tokens[i].toUpperCase()
          arity = ARITY[cmd] ?? -1
          if (arity < 0) throw new Error(`${key}: 未知命令 ${tokens[i]}`)
          if (cmd === 'M') sawMove = true
          i++
          if (arity === 0) continue
        }
        if (i + arity > tokens.length) throw new Error(`${key}: ${cmd} 参数不足`)
        for (let k = 0; k < arity; k++) {
          expect(Number.isFinite(Number(tokens[i + k])), `${key}: ${tokens[i + k]} 不是数字`).toBe(true)
        }
        i += arity
      }
      expect(sawMove, `${key}: 缺少 M 起笔`).toBe(true)
    }
  })
})

describe('图形几何：必须落在节点圆（半径 9）内且大致居中', () => {
  const NODE_R = 9
  const cases = (['windows', 'linux', 'darwin', 'other'] as const).map((k) => [k, OS_GLYPHS[k].d] as const)

  it('最远点不超过节点圆半径', () => {
    for (const [name, d] of cases) {
      const b = pathBounds(d)
      expect(b.maxRadius, `${name} 最远点 ${b.maxRadius.toFixed(2)} 超出半径 ${NODE_R}`).toBeLessThanOrEqual(NODE_R)
    }
  })

  it('大致居中（歪掉的图形放进圆里一眼就看得出来）', () => {
    for (const [name, d] of cases) {
      const b = pathBounds(d)
      const cx = (b.minX + b.maxX) / 2
      const cy = (b.minY + b.maxY) / 2
      expect(Math.abs(cx), `${name} 水平偏移 ${cx.toFixed(2)}`).toBeLessThanOrEqual(0.8)
      expect(Math.abs(cy), `${name} 垂直偏移 ${cy.toFixed(2)}`).toBeLessThanOrEqual(0.8)
    }
  })

  it('不是空图形或一条线（宽高都要有量级）', () => {
    for (const [name, d] of cases) {
      const b = pathBounds(d)
      expect(b.maxX - b.minX, name).toBeGreaterThan(6)
      expect(b.maxY - b.minY, name).toBeGreaterThan(4)
    }
  })
})
