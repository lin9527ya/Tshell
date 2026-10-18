import { describe, expect, it, vi } from 'vitest'
import {
  CREDENTIAL_KEYWORDS,
  EXECUTION_MODES,
  executionModeText,
  filterShellContentByMode,
  filterShellContentForMimikatz,
  MIMIKATZ_CREDENTIAL_KEYWORDS,
} from './output-filter'

const HINT = '__NO_OUTPUT__'

/** 造一段 shell 历史：命令回显 + 输出 */
function history(...parts: string[]): string {
  return parts.join('\n')
}

describe('filterShellContentByMode', () => {
  it('空内容或空模式原样返回（不做无意义的过滤）', () => {
    const o = { noOutputHint: HINT }
    expect(filterShellContentByMode('', 'bof', 'p', o)).toBe('')
    expect(filterShellContentByMode('some output', '', 'p', o)).toBe('some output')
  })

  it('切出最近一次该模式执行的输出', () => {
    const content = history(
      '$ execute-assembly /plugins/a.exe',
      'old output line',
      '$ bof /plugins/a.o',
      'bof result 1',
      'bof result 2',
    )
    const got = filterShellContentByMode(content, 'bof', 'a.o', { noOutputHint: HINT })
    expect(got).toContain('bof result 1')
    expect(got).toContain('bof result 2')
    // 不应把上一次执行的输出带进来
    expect(got).not.toContain('old output line')
  })

  it('多次执行同一插件时只取最后一次', () => {
    const content = history(
      '$ bof /plugins/a.o',
      'first run',
      '$ bof /plugins/a.o',
      'second run',
    )
    const got = filterShellContentByMode(content, 'bof', 'a.o', { noOutputHint: HINT })
    expect(got).toContain('second run')
    expect(got).not.toContain('first run')
  })

  it('遇到下一条任意执行模式命令就停（不把后续输出算进来）', () => {
    const content = history(
      '$ bof /plugins/a.o',
      'bof output',
      '$ script /plugins/b.py',
      'script output',
    )
    const got = filterShellContentByMode(content, 'bof', 'a.o', { noOutputHint: HINT })
    expect(got).toContain('bof output')
    expect(got).not.toContain('script output')
  })

  it('找不到匹配命令时返回提示文案，而不是整段历史', () => {
    // 返回整段历史会让操作员误以为看到的是本次结果
    const content = history('$ bof /plugins/other.o', 'unrelated output')
    const got = filterShellContentByMode(content, 'bof', 'a.o', { noOutputHint: HINT })
    expect(got).toBe(HINT)
  })

  it('非 $ 开头的行不被当作命令边界', () => {
    const content = history(
      '$ bof /plugins/a.o',
      'output mentioning bof inline-bin in text',
      'more output',
    )
    const got = filterShellContentByMode(content, 'bof', 'a.o', { noOutputHint: HINT })
    // 中间的普通输出行虽然含 "inline-bin"，但不该截断
    expect(got).toContain('more output')
  })

  it('mimikatz 且内容过少时向前扩展，取到凭据内容', () => {
    const content = history(
      '$ execute-assembly /plugins/mimikatz.exe',
      'Authentication Id : 0 ; 12345',
      'Username : alice',
      'NTLM : deadbeef',
    )
    const got = filterShellContentByMode(content, 'execute-assembly', 'mimikatz.exe', {
      noOutputHint: HINT,
    })
    expect(got).toContain('Username : alice')
    expect(got).toContain('NTLM : deadbeef')
  })

  it('mimikatz 扩展仅在有凭据关键字时生效（否则保持原切法）', () => {
    const content = history(
      '$ execute-assembly /plugins/mimikatz.exe',
      'nothing sensitive here',
    )
    const got = filterShellContentByMode(content, 'execute-assembly', 'mimikatz.exe', {
      noOutputHint: HINT,
    })
    expect(got).toContain('nothing sensitive here')
  })

  it('非 mimikatz 插件不做扩展', () => {
    const content = history(
      '$ execute-assembly /plugins/other.exe',
      'short output',
    )
    const got = filterShellContentByMode(content, 'execute-assembly', 'other.exe', {
      noOutputHint: HINT,
    })
    expect(got).toContain('short output')
  })

  it('调试日志默认不产生输出（模块默认无副作用）', () => {
    const spy = vi.spyOn(console, 'log').mockImplementation(() => {})
    filterShellContentByMode('$ bof /plugins/a.o\nx', 'bof', 'a.o', { noOutputHint: HINT })
    expect(spy).not.toHaveBeenCalled()
    spy.mockRestore()
  })

  it('传入 logger 时日志照常发生（保持组件原有行为）', () => {
    const log = vi.fn()
    filterShellContentByMode('$ bof /plugins/a.o\nx', 'bof', 'a.o', { noOutputHint: HINT, log })
    expect(log).toHaveBeenCalled()
  })
})

describe('filterShellContentForMimikatz', () => {
  const o = { noOutputHint: HINT }

  it('正常切出当前模式的输出', () => {
    const content = history('$ execute-assembly /plugins/m.exe', 'cred line 1', 'cred line 2')
    const got = filterShellContentForMimikatz(content, 'execute-assembly', 'm.exe', o)
    expect(got).toContain('cred line 1')
    expect(got).toContain('cred line 2')
  })

  it('退路一：找不到当前模式时，认任何 mimikatz 命令', () => {
    const content = history('$ execute-assembly /plugins/mimikatz.exe', 'sekurlsa output')
    // currentMode 不匹配，但命令行含 mimikatz
    const got = filterShellContentForMimikatz(content, 'bof', 'other.o', o)
    expect(got).toContain('sekurlsa output')
  })

  it('退路一也认 sekur 与同名插件', () => {
    expect(
      filterShellContentForMimikatz(history('$ x /plugins/sekurlsa.exe', 'out1'), 'bof', 'zzz', o),
    ).toContain('out1')
    expect(
      filterShellContentForMimikatz(history('$ x /plugins/myplug.exe', 'out2'), 'bof', 'myplug.exe', o),
    ).toContain('out2')
  })

  it('退路二：都没有时，返回最近内容里含凭据关键字的那段', () => {
    const content = history('$ unknown cmd', 'filler', 'Username : bob', 'NTLM : cafe')
    const got = filterShellContentForMimikatz(content, 'bof', 'nope.o', o)
    expect(got).toContain('Username : bob')
    expect(got).toContain('NTLM : cafe')
  })

  it('三条退路都走不通时返回提示文案', () => {
    const content = history('$ unknown cmd', 'totally unrelated text')
    expect(filterShellContentForMimikatz(content, 'bof', 'nope.o', o)).toBe(HINT)
  })

  it('结束边界宽松：遇到另一个插件的执行命令才停', () => {
    const content = history(
      '$ execute-assembly /plugins/m.exe',
      'mimikatz output',
      '$ execute-assembly /plugins/other.exe',
      'other output',
    )
    const got = filterShellContentForMimikatz(content, 'execute-assembly', 'm.exe', o)
    expect(got).toContain('mimikatz output')
    expect(got).not.toContain('other output')
  })

  it('同一插件的重复执行不截断（排除同名）', () => {
    const content = history(
      '$ execute-assembly /plugins/m.exe',
      'line A',
      '$ execute-assembly /plugins/m.exe',
      'line B',
    )
    const got = filterShellContentForMimikatz(content, 'execute-assembly', 'm.exe', o)
    // 同名插件的第二次命令不构成边界，两段都应包含
    expect(got).toContain('line A')
    expect(got).toContain('line B')
  })

  it('[CLIENT-PACKET] / 完整数据包 回显不构成边界', () => {
    const content = history(
      '$ execute-assembly /plugins/m.exe',
      'line A',
      '$ execute-assembly [CLIENT-PACKET] 完整数据包 /plugins/other.exe',
      'line B',
    )
    const got = filterShellContentForMimikatz(content, 'execute-assembly', 'm.exe', o)
    // 这条虽是命令且含执行模式，但被 [CLIENT-PACKET] 排除，不该截断
    expect(got).toContain('line B')
  })

  it('向前多取 20 行上下文', () => {
    const before = Array.from({ length: 30 }, (_, i) => `before-${i}`)
    const content = history(...before, '$ execute-assembly /plugins/m.exe', 'the output')
    const got = filterShellContentForMimikatz(content, 'execute-assembly', 'm.exe', o)
    expect(got).toContain('the output')
    // 至少带回一部分前文（20 行）
    expect(got).toContain('before-29')
    expect(got).not.toContain('before-0')
  })

  it('空内容/空模式原样返回', () => {
    expect(filterShellContentForMimikatz('', 'bof', 'p', o)).toBe('')
    expect(filterShellContentForMimikatz('x', '', 'p', o)).toBe('x')
  })

  it('调试日志默认不产生输出', () => {
    const spy = vi.spyOn(console, 'log').mockImplementation(() => {})
    filterShellContentForMimikatz('$ bof /plugins/m.exe\nx', 'bof', 'm.exe', o)
    expect(spy).not.toHaveBeenCalled()
    spy.mockRestore()
  })
})

describe('executionModeText', () => {
  it('固定英文名与后端模式名一致', () => {
    expect(executionModeText('execute-assembly', true)).toBe('Execute Assembly')
    expect(executionModeText('inline-bin', true)).toBe('Inline Bin')
    expect(executionModeText('script', true)).toBe('Linux Script')
    expect(executionModeText('binary', true)).toBe('Linux Binary')
    expect(executionModeText('bof', true)).toBe('BOF(COFF)')
    expect(executionModeText('exec-file', true)).toBe('Exec File')
  })

  it('需要本地化的两项按语言切换', () => {
    expect(executionModeText('shellcode-inject', true)).toBe('Shellcode注入')
    expect(executionModeText('shellcode-inject', false)).toBe('Shellcode')
    expect(executionModeText('unknown', true)).toBe('请选择模式')
    expect(executionModeText('unknown', false)).toBe('Select Mode')
    expect(executionModeText('', true)).toBe('请选择模式')
  })
})

describe('关键字表', () => {
  it('执行模式表覆盖 7 种（与后端模式名对齐）', () => {
    expect(EXECUTION_MODES).toHaveLength(7)
    expect(EXECUTION_MODES).toContain('execute-assembly')
    expect(EXECUTION_MODES).toContain('exec-file')
  })

  it('两个凭据关键字表刻意不同（原实现如此，抽取未统一）', () => {
    // 通用版认 ssp/credman/tspkg；mimikatz 版不认这些但认 mimikatz 字样。
    // 统一会改变"哪些内容算凭据"，属行为变更，应单独提。
    expect(CREDENTIAL_KEYWORDS).toContain('ssp')
    expect(CREDENTIAL_KEYWORDS).toContain('credman')
    expect(MIMIKATZ_CREDENTIAL_KEYWORDS).not.toContain('ssp')
    expect(MIMIKATZ_CREDENTIAL_KEYWORDS).toContain('mimikatz')
    expect(CREDENTIAL_KEYWORDS.length).toBeGreaterThan(MIMIKATZ_CREDENTIAL_KEYWORDS.length)
  })
})
