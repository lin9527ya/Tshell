import { describe, expect, it } from 'vitest'
import {
  portOf,
  TARGET_GLYPHS,
  targetGlyphKey,
  targetService,
  type TargetGlyphKey,
} from './target-glyph'

describe('portOf', () => {
  it('取地址末尾的端口', () => {
    expect(portOf('10.0.0.5:445')).toBe(445)
    expect(portOf('host.example.com:8080')).toBe(8080)
    expect(portOf('[::1]:443')).toBe(443)
  })

  it('没有端口时返回 null（纯 IP、空串）', () => {
    expect(portOf('10.0.0.5')).toBeNull()
    expect(portOf('')).toBeNull()
    expect(portOf('   ')).toBeNull()
  })

  it('越界端口不算数（:0 / :70000 不是端口）', () => {
    expect(portOf('10.0.0.5:0')).toBeNull()
    expect(portOf('10.0.0.5:70000')).toBeNull()
  })
})

describe('targetGlyphKey', () => {
  it('按端口归类到对应图标', () => {
    const cases: [string, TargetGlyphKey][] = [
      ['10.0.0.5:445', 'windows'],
      ['10.0.0.5:139', 'windows'],
      ['10.0.0.5:3389', 'windows'],
      ['10.0.0.5:5985', 'windows'],
      ['10.0.0.5:22', 'terminal'],
      ['10.0.0.5:23', 'terminal'],
      ['10.0.0.5:80', 'web'],
      ['10.0.0.5:8443', 'web'],
      ['10.0.0.5:3306', 'database'],
      ['10.0.0.5:6379', 'database'],
    ]
    for (const [addr, key] of cases) expect(targetGlyphKey(addr), addr).toBe(key)
  })

  it('认不出的端口给准星，不硬猜', () => {
    expect(targetGlyphKey('10.0.0.5:12345')).toBe('generic')
    expect(targetGlyphKey('10.0.0.5')).toBe('generic')
    expect(targetGlyphKey('')).toBe('generic')
  })
})

describe('targetService', () => {
  it('给出具体服务名', () => {
    expect(targetService('10.0.0.5:445')?.zh).toBe('SMB 文件共享')
    expect(targetService('10.0.0.5:3306')?.zh).toBe('MySQL')
  })

  it('认不出的端口返回 null（详情面板就不显示这一行）', () => {
    expect(targetService('10.0.0.5:12345')).toBeNull()
    expect(targetService('10.0.0.5')).toBeNull()
  })
})

describe('TARGET_GLYPHS', () => {
  const ARITY: Record<string, number> = { M: 2, L: 2, H: 1, V: 1, C: 6, S: 4, Q: 4, T: 2, A: 7, Z: 0 }

  it('每种都有 path 与显示名', () => {
    for (const key of Object.keys(TARGET_GLYPHS) as TargetGlyphKey[]) {
      const g = TARGET_GLYPHS[key]
      expect(g.d.length).toBeGreaterThan(10)
      expect(g.zh).toBeTruthy()
      expect(g.en).toBeTruthy()
    }
  })
  // 注：「图形是否落在节点圆内（半径 9）」不做数字扫描 —— 圆弧命令里的数字是增量
  // （如 a6.3 6.3 0 1 1 0 12.6 的 12.6 是直径），扫数字必然误判。
  // 这条几何约束用浏览器 getBBox() 实测（见 verify 记录）。

  it('path 语法正确（参数个数与命令匹配，写错了在图上就是空白）', () => {
    for (const key of Object.keys(TARGET_GLYPHS) as TargetGlyphKey[]) {
      const tokens = TARGET_GLYPHS[key].d.match(/[MmLlHhVvCcSsQqTtAaZz]|-?\d*\.?\d+/g) || []
      let i = 0
      let arity = 0
      let cmd = ''
      while (i < tokens.length) {
        if (/[A-Za-z]/.test(tokens[i])) {
          cmd = tokens[i].toUpperCase()
          arity = ARITY[cmd] ?? -1
          if (arity < 0) throw new Error(`${key}: 未知命令 ${tokens[i]}`)
          i++
          if (arity === 0) continue
        }
        if (i + arity > tokens.length) throw new Error(`${key}: ${cmd} 参数不足`)
        i += arity
      }
      expect(tokens.length, key).toBeGreaterThan(0)
    }
  })

})
