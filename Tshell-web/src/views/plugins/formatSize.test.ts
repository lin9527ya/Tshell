import { describe, expect, it } from 'vitest'
import { formatPluginFileSize } from './formatSize'
import { formatFileSize as formatClientFileSize } from '@/views/client/files/format'

describe('formatPluginFileSize', () => {
  it('按 1024 进制换算', () => {
    expect(formatPluginFileSize(512)).toBe('512 B')
    expect(formatPluginFileSize(1024)).toBe('1 KB')
    expect(formatPluginFileSize(1536)).toBe('1.5 KB')
    expect(formatPluginFileSize(1024 ** 2)).toBe('1 MB')
    expect(formatPluginFileSize(1024 ** 3)).toBe('1 GB')
  })

  it('两位小数 + 去尾零（这是与文件管理器那版的关键差异）', () => {
    // parseFloat 会把 "1.00" 变成 1、"1.50" 变成 1.5
    expect(formatPluginFileSize(1024)).toBe('1 KB')
    expect(formatPluginFileSize(1024 * 1.5)).toBe('1.5 KB')
    // 需要两位才表达得出来的量级
    expect(formatPluginFileSize(1024 * 1.25)).toBe('1.25 KB')
  })

  it('零与非法值归为 0 B（不产出 "NaN GB"）', () => {
    expect(formatPluginFileSize(0)).toBe('0 B')
    expect(formatPluginFileSize(-1)).toBe('0 B')
    expect(formatPluginFileSize(NaN)).toBe('0 B')
    expect(formatPluginFileSize(Infinity)).toBe('0 B')
  })

  it('超过 GB 时不再进位（单位表上限），而不是产出 undefined', () => {
    const huge = 1024 ** 4 // 1 TB
    const got = formatPluginFileSize(huge)
    expect(got).not.toContain('undefined')
    expect(got).toContain('GB')
  })

  it('与文件管理器那版刻意不一致 —— 这条钉住分歧，防止被"顺手统一"', () => {
    // 统一会改变其中一处的显示，属可见变更，应单独提一次由人确认。
    // 若哪天真的统一了，这条会失败，提醒改的人同步更新文档与两边测试。
    expect(formatPluginFileSize(1024)).toBe('1 KB')
    expect(formatClientFileSize(1024)).toBe('1.0 KB')
    expect(formatPluginFileSize(1536)).toBe('1.5 KB')
    expect(formatClientFileSize(1536)).toBe('1.5 KB')
  })
})
