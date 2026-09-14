import { describe, it, expect } from 'vitest'
import { processTerminalOutput } from './encoding'

describe('processTerminalOutput (F1 XSS 转义)', () => {
  it('转义 HTML 特殊字符', () => {
    const out = processTerminalOutput('<img src=x onerror=alert(1)>')
    expect(out).not.toContain('<img')
    expect(out).toContain('&lt;img')
  })

  it('转义 script 标签', () => {
    const out = processTerminalOutput('<script>alert(1)</script>')
    expect(out).not.toContain('<script>')
  })

  it('保留 ANSI 颜色转换为 span', () => {
    const out = processTerminalOutput('\x1b[31mred\x1b[0m')
    expect(out).toContain('<span')
    expect(out).toContain('color:')
  })

  it('空值安全', () => {
    expect(processTerminalOutput('')).toBe('')
    expect(processTerminalOutput(undefined as any)).toBeUndefined()
  })
})
