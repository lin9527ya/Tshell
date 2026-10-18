import { describe, expect, it } from 'vitest'
import {
  decodeFileContent,
  extractRawContent,
  looksLikeBase64,
  tryDecodeBase64,
} from './decodeContent'

/** 造 base64（UTF-8 安全） */
function b64(s: string): string {
  return btoa(String.fromCharCode(...new TextEncoder().encode(s)))
}

describe('extractRawContent', () => {
  it('裸字符串响应', () => {
    expect(extractRawContent('hello')).toBe('hello')
  })

  it('{ data: "..." }', () => {
    expect(extractRawContent({ data: 'hello' })).toBe('hello')
  })

  it('{ data: { data: "..." } }', () => {
    expect(extractRawContent({ data: { data: 'hello' } })).toBe('hello')
  })

  it('{ data: { data: { content } } }', () => {
    expect(extractRawContent({ data: { data: { content: 'hello' } } })).toBe('hello')
  })

  it('{ data: { content } }', () => {
    expect(extractRawContent({ data: { content: 'hello' } })).toBe('hello')
  })

  it('{ data: { data: { fileContent } } }', () => {
    expect(extractRawContent({ data: { data: { fileContent: 'hello' } } })).toBe('hello')
  })

  it('{ data: { fileContent } }', () => {
    expect(extractRawContent({ data: { fileContent: 'hello' } })).toBe('hello')
  })

  it('对象兜底走美化 JSON，而不是 [object Object]', () => {
    const got = extractRawContent({ data: { data: { a: 1 } } })
    expect(got).toContain('"a": 1')
    expect(got).not.toContain('[object Object]')
  })

  it('优先级：content 比整体 JSON 更具体，必须先命中', () => {
    // 这条守的是最容易写错的顺序问题：若先判"data 是对象"就 JSON.stringify，
    // 编辑器里会出现一整段 {"content":"真正的文件内容"}
    const got = extractRawContent({ data: { data: { content: 'REAL' } } })
    expect(got).toBe('REAL')
    expect(got).not.toContain('{')
  })

  it('空值与缺失字段返回空串（而不是 undefined）', () => {
    expect(extractRawContent(null)).toBe('')
    expect(extractRawContent(undefined)).toBe('')
    expect(extractRawContent({})).toBe('')
    expect(extractRawContent({ data: null })).toBe('')
    expect(extractRawContent({ data: {} })).toBe('')
  })

  it('数字等非字符串 content 也转成字符串', () => {
    expect(extractRawContent({ data: { content: 42 } })).toBe('42')
  })
})

describe('looksLikeBase64', () => {
  const longPlain = 'Hello, World! This is a longer text.'
  const longB64 = b64(longPlain)

  it('识别足够长的 base64', () => {
    expect(longB64.length).toBeGreaterThan(24)
    expect(looksLikeBase64(longB64)).toBe(true)
  })

  it('长度阈值是 24 字符 —— 短串有意不判为 base64（避免巧合命中）', () => {
    // 把阈值本身钉住：'Hello, World!' 的 base64 只有 20 字符，不会被解。
    // 阈值是原实现就有的刻意取舍 —— 误判会把普通文本变成乱码，
    // 那比"漏解一次"严重得多（用户会以为文件坏了）。
    const short = b64('Hello, World!')
    expect(short.length).toBeLessThanOrEqual(24)
    expect(looksLikeBase64(short)).toBe(false)
  })

  it('普通文本不会被误判（误判代价比漏判大）', () => {
    expect(looksLikeBase64('这是一段中文文本内容，不是base64编码')).toBe(false)
    expect(looksLikeBase64('function foo() { return 1 } // some comment')).toBe(false)
    expect(looksLikeBase64('hello world this is plain text here')).toBe(false)
  })

  it('含换行的长文本不判为 base64（真实文本几乎都含换行）', () => {
    const text = `${longB64}\n${longB64}`
    expect(looksLikeBase64(text)).toBe(false)
  })

  it('长度非 4 的倍数不判为 base64', () => {
    // 去掉一个字符，长度就不是 4 的倍数了
    expect(looksLikeBase64(longB64.slice(0, longB64.length - 1))).toBe(false)
  })

  it('太短的串不判为 base64', () => {
    expect(looksLikeBase64('abcd')).toBe(false)
    expect(looksLikeBase64('')).toBe(false)
  })

  it('含非法字符不判为 base64', () => {
    expect(looksLikeBase64(`${longB64}!!!`)).toBe(false)
  })
})

describe('tryDecodeBase64', () => {
  it('解出 UTF-8 文本', () => {
    const plain = 'Hello, World! This is a longer text.'
    expect(tryDecodeBase64(b64(plain))).toBe(plain)
  })

  it('解出非 ASCII（UTF-8 多字节）', () => {
    expect(tryDecodeBase64(b64('中文内容测试'))).toBe('中文内容测试')
  })

  it('解不出时原样返回（而不是抛错或返回半截乱码）', () => {
    expect(tryDecodeBase64('!!!not-base64!!!')).toBe('!!!not-base64!!!')
  })

  it('非法 UTF-8 字节序列原样返回（fatal 解码失败）', () => {
    // 0xff 0xfe 不是合法 UTF-8
    const bad = btoa(String.fromCharCode(0xff, 0xfe))
    expect(tryDecodeBase64(bad)).toBe(bad)
  })
})

describe('decodeFileContent（完整流程）', () => {
  it('普通文本原样返回', () => {
    expect(decodeFileContent({ data: { data: 'line1\nline2' } })).toBe('line1\nline2')
  })

  it('base64 内容被解出来', () => {
    const plain = 'Hello, World! This is a longer text.'
    expect(decodeFileContent({ data: { data: b64(plain) } })).toBe(plain)
  })

  it('中文 base64 内容被正确解出', () => {
    const plain = '这是文件内容，需要足够长才会走 base64 分支。'
    expect(decodeFileContent({ data: { data: { content: b64(plain) } } })).toBe(plain)
  })

  it('空响应返回空串', () => {
    expect(decodeFileContent(null)).toBe('')
    expect(decodeFileContent({})).toBe('')
  })

  it('像 base64 但解不出时退回原串（不产出乱码）', () => {
    const bad = btoa(String.fromCharCode(0xff, 0xfe, 0xfd, 0xfc, 0xfb, 0xfa, 0xf9, 0xf8, 0xf7, 0xf6))
    expect(decodeFileContent({ data: { data: bad } })).toBe(bad)
  })
})
