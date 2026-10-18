import { describe, expect, it } from 'vitest'
import {
  csvCell,
  iocToCSV,
  reportFileName,
  socialToCSV,
  toCSV,
  ttpToCSV,
  type CsvColumn,
} from './export'

describe('csvCell', () => {
  it('空值输出空串', () => {
    expect(csvCell(null)).toBe('')
    expect(csvCell(undefined)).toBe('')
    expect(csvCell('')).toBe('')
  })

  it('含逗号/引号/换行的字段按 RFC 4180 转义', () => {
    expect(csvCell('a,b')).toBe('"a,b"')
    expect(csvCell('say "hi"')).toBe('"say ""hi"""')
    // 命令里的换行不转义会把一行拆成两行，表格直接错位
    expect(csvCell('line1\nline2')).toBe('"line1\nline2"')
    expect(csvCell('a\r\nb')).toBe('"a\r\nb"')
  })

  it('普通字段不加引号', () => {
    expect(csvCell('WEB-01')).toBe('WEB-01')
    expect(csvCell(42)).toBe('42')
  })

  it('以 = + - @ 开头的值加前导单引号（防 CSV 公式注入）', () => {
    // 这些字段来自被控端/攻击者可控输入，Excel/WPS 会当公式执行
    expect(csvCell('=cmd|\'/c calc\'!A0')).toBe("'=cmd|'/c calc'!A0")
    expect(csvCell('+1+1')).toBe("'+1+1")
    expect(csvCell('-2+3')).toBe("'-2+3")
    expect(csvCell('@SUM(A1)')).toBe("'@SUM(A1)")
    expect(csvCell('\tTAB')).toBe("'\tTAB")
  })

  it('公式注入防护与引号转义叠加时顺序正确', () => {
    // 先加 ' 再包引号：Excel 看到的是文本 "'=..."，不会被当公式
    expect(csvCell('=a,b')).toBe('"\'=a,b"')
  })

  it('负数也会被加引号 —— 宁可当文本也不能让表格执行公式', () => {
    expect(csvCell(-5)).toBe("'-5")
  })
})

describe('toCSV', () => {
  interface Row {
    a: string
    b: number
  }
  const cols: CsvColumn<Row>[] = [
    { key: 'a', label: '列A' },
    { key: 'b', label: '列B' },
  ]

  it('输出表头 + 行，CRLF 分隔，带 UTF-8 BOM', () => {
    const out = toCSV([{ a: 'x', b: 1 }], cols)
    expect(out.startsWith('\uFEFF')).toBe(true) // 没有 BOM 时 Excel 打开中文乱码
    expect(out).toContain('列A,列B')
    expect(out).toContain('x,1')
    expect(out.endsWith('\r\n')).toBe(true)
  })

  it('空行数组只输出表头', () => {
    const out = toCSV<Row>([], cols)
    expect(out).toBe('\uFEFF列A,列B\r\n')
  })

  it('支持自定义取值函数', () => {
    const c: CsvColumn<Row>[] = [{ key: 'a', label: 'A', value: (r) => r.a.toUpperCase() }]
    expect(toCSV([{ a: 'x', b: 1 }], c)).toContain('X')
  })
})

describe('iocToCSV', () => {
  it('主机表 + 一维列表分区块输出', () => {
    const out = iocToCSV({
      generated_at: '2026-10-19 02:00:00',
      hosts: [{ host: 'WEB-01', user: 'alice', online: '1' }],
      listener_addresses: ['1.2.3.4:443'],
      blocked_ips: ['9.9.9.9'],
      login_failures: ['admin@10.0.0.1'],
    })
    expect(out).toContain('主机')
    expect(out).toContain('WEB-01')
    expect(out).toContain('在线') // online='1' 映射为"在线"
    expect(out).toContain('监听地址')
    expect(out).toContain('1.2.3.4:443')
    expect(out).toContain('封禁 IP')
    expect(out).toContain('9.9.9.9')
    expect(out).toContain('登录失败')
  })

  it('离线主机映射为"离线"', () => {
    const out = iocToCSV({ hosts: [{ host: 'X', online: '2' }] })
    expect(out).toContain('离线')
  })

  it('没有列表时不输出多余区块', () => {
    const out = iocToCSV({ hosts: [{ host: 'X' }] })
    expect(out).not.toContain('监听地址')
    expect(out).not.toContain('封禁 IP')
  })

  it('空报告不报错', () => {
    expect(() => iocToCSV({})).not.toThrow()
  })
})

describe('socialToCSV', () => {
  it('投递项与点击明细两段', () => {
    const out = socialToCSV({
      deliveries: [{ name: 'a.exe', url: 'http://x/a.exe', status: '开启' }],
      clicks: [{ time: 't', resource: 'full', client_ip: '1.1.1.1', status_code: 200 }],
    })
    expect(out).toContain('投递项')
    expect(out).toContain('a.exe')
    expect(out).toContain('来源 IP')
    expect(out).toContain('1.1.1.1')
  })

  it('无投递项时只输出点击段', () => {
    const out = socialToCSV({ clicks: [{ client_ip: '1.1.1.1' }] })
    expect(out).not.toContain('投递项')
    expect(out).toContain('1.1.1.1')
  })

  it('恶意 User-Agent 被转义，不会注入公式', () => {
    const out = socialToCSV({ clicks: [{ user_agent: '=HYPERLINK("http://evil")' }] })
    expect(out).toContain("'=HYPERLINK")
  })
})

describe('ttpToCSV', () => {
  it('技术统计与行为明细两段', () => {
    const out = ttpToCSV({
      techniques: [{ tactic: 'Execution', technique: 'T1059', count: 3 }],
      per_session: [{ host: 'WEB-01', command: 'whoami', tactic: 'Discovery' }],
    })
    expect(out).toContain('MITRE 技术统计')
    expect(out).toContain('T1059')
    expect(out).toContain('行为明细')
    expect(out).toContain('whoami')
  })

  it('命令里的逗号/引号不会破坏列结构', () => {
    const out = ttpToCSV({ per_session: [{ host: 'H', command: 'echo "a,b"', tactic: 'X' }] })
    // 含逗号的命令必须被引号包裹
    expect(out).toContain('"echo ""a,b"""')
  })

  it('无行为明细时只输出统计段', () => {
    const out = ttpToCSV({ techniques: [] })
    expect(out).not.toContain('行为明细')
  })
})

describe('reportFileName', () => {
  it('带时间戳，避免多次导出互相覆盖', () => {
    const n = reportFileName('ioc', 'csv', new Date(2026, 9, 19, 2, 5, 7))
    expect(n).toBe('ioc_report_20261019_020507.csv')
  })

  it('补零到两位', () => {
    const n = reportFileName('ttp', 'json', new Date(2026, 0, 3, 4, 5, 6))
    expect(n).toBe('ttp_report_20260103_040506.json')
  })
})
