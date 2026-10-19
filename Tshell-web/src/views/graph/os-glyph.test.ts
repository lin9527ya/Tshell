import { describe, expect, it } from 'vitest'
import { OS_GLYPHS, osGlyphKey } from './os-glyph'

describe('osGlyphKey', () => {
  it('windows / linux 各自归一', () => {
    expect(osGlyphKey('windows')).toBe('windows')
    expect(osGlyphKey('Windows')).toBe('windows')
    expect(osGlyphKey('win10')).toBe('windows')
    expect(osGlyphKey('linux')).toBe('linux')
    expect(osGlyphKey('ubuntu')).toBe('linux')
  })

  it('认不出的走 other，而不是返回空（空图标与"没数据"分不清）', () => {
    expect(osGlyphKey('darwin')).toBe('other')
    expect(osGlyphKey('')).toBe('other')
    expect(osGlyphKey(undefined)).toBe('other')
    expect(osGlyphKey('   ')).toBe('other')
  })
})

describe('OS_GLYPHS', () => {
  it('每种都有可画的 path 与显示名', () => {
    for (const key of ['windows', 'linux', 'other'] as const) {
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
    for (const key of ['windows', 'linux', 'other'] as const) {
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
