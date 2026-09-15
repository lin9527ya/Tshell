import { describe, it, expect } from 'vitest'
import { processTerminalOutput, fixGBKEncoding, smartDecodeContent } from './encoding'

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

describe('gb18030 字节级解码(KNOWN_LIMITATIONS #11)', () => {
  it('GBK 字节误读形态恢复为中文', () => {
    // "中文" 的 GBK 字节 D6 D0 CE C4 被 Latin-1 误读后的形态
    const mojibake = String.fromCharCode(0xd6, 0xd0, 0xce, 0xc4)
    const out = fixGBKEncoding(mojibake)
    expect(out).toContain('中文')
  })

  it('正常中文不受影响', () => {
    const out = fixGBKEncoding('正常的中文输出')
    expect(out).toBe('正常的中文输出')
  })

  it('纯 ASCII 不做转换', () => {
    const out = fixGBKEncoding('whoami\nkali')
    expect(out).toBe('whoami\nkali')
  })

  it('含真实 Unicode 字符的串不误判', () => {
    // 已是正确中文(码点 > 0xFF),字节级还原不应执行
    const out = fixGBKEncoding('目录 中文 测试')
    expect(out).toBe('目录 中文 测试')
  })

  it('smartDecodeContent 端到端', () => {
    // "目录" GBK = C4 BF C2 BC
    const moji = String.fromCharCode(0xc4, 0xbf, 0xc2, 0xbc)
    const out = smartDecodeContent(moji)
    expect(out).toContain('目录')
  })
})
