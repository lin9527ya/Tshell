/**
 * 报告导出（阶段三 §3.11）。
 *
 * 此前三个报告（IOC / 社会工程 / TTP）的"导出"按钮都只是
 * `JSON.stringify(...)` 落一个 .json —— 交付给客户/存档时基本没法用：
 * Excel 打不开、也没法按列筛选。这里补上 CSV。
 *
 * 两个容易被忽略但必须处理的点：
 *
 * 1. **CSV 公式注入**：报告里的字段（主机名、User-Agent、文件名、命令）都来自
 *    被控端/攻击者可控的输入。以 `=` `+` `-` `@` 开头的单元格在 Excel/WPS 里会被
 *    当公式执行（`=cmd|'/c calc'!A0` 这类是经典利用）。导出前必须转义 ——
 *    否则我们等于把被控端的内容当代码喂给分析人员的表格软件。
 * 2. **RFC 4180 转义**：字段含逗号/引号/换行时必须用双引号包裹并把内部引号翻倍。
 *    不做这一步，命令里的换行会把一行拆成两行，表格直接错位。
 */

/** CSV 列定义 */
export interface CsvColumn<T> {
  key: string
  /** 表头文字（导出文件里显示的列名） */
  label: string
  /** 取值；缺省取 row[key] */
  value?: (row: T) => unknown
}

/** 会被表格软件当公式起始的字符 */
const FORMULA_PREFIXES = ['=', '+', '-', '@', '\t', '\r']

/**
 * csvCell 单个单元格转义。
 *
 * 顺序很重要：先防公式注入（加前导单引号），再做 RFC 4180 引号转义。
 * 反过来的话，前导单引号会被包进引号里，Excel 看到的是 `"'=..."`，
 * 反而可能仍按文本里的公式处理（不同版本行为不一致）。
 */
export function csvCell(v: unknown): string {
  if (v === null || v === undefined) return ''
  let s = typeof v === 'string' ? v : String(v)
  if (s === '') return ''
  if (FORMULA_PREFIXES.some((p) => s.startsWith(p))) {
    s = "'" + s
  }
  if (/[",\r\n]/.test(s)) {
    s = '"' + s.replace(/"/g, '""') + '"'
  }
  return s
}

/** toCSV 把行数组渲染成 CSV 文本（含 BOM，Excel 打开中文不乱码） */
export function toCSV<T>(rows: T[], columns: CsvColumn<T>[]): string {
  const header = columns.map((c) => csvCell(c.label)).join(',')
  const body = rows.map((row) =>
    columns
      .map((c) => csvCell(c.value ? c.value(row) : (row as Record<string, unknown>)[c.key]))
      .join(','),
  )
  // 前置 UTF-8 BOM：没有它 Excel 会按本地代码页解码，中文列名/主机名变乱码
  return '\uFEFF' + [header, ...body].join('\r\n') + '\r\n'
}

// ===== 各报告的行/列映射 =====
// 字段名与后端 Go 结构体的 json tag 对齐（pkg/api/report.go）。

export interface IOCReport {
  generated_at?: string
  hosts?: Array<Record<string, unknown>>
  listener_addresses?: string[]
  blocked_ips?: string[]
  artifacts?: Array<Record<string, unknown>>
  login_failures?: string[]
}

export interface SocialReport {
  generated_at?: string
  total_clicks?: number
  unique_ips?: number
  deliveries?: Array<Record<string, unknown>>
  clicks?: Array<Record<string, unknown>>
}

export interface TTPReport {
  generated_at?: string
  techniques?: Array<Record<string, unknown>>
  per_session?: Array<Record<string, unknown>>
}

const zh = (z: string, e: string) => ({ zh: z, en: e })

export const IOC_HOST_COLUMNS: CsvColumn<Record<string, unknown>>[] = [
  { key: 'host', label: zh('主机', 'Host').zh },
  { key: 'user', label: zh('用户', 'User').zh },
  { key: 'external_ip', label: zh('外部 IP', 'External IP').zh },
  { key: 'internal_ip', label: zh('内网 IP', 'Internal IP').zh },
  { key: 'listener', label: zh('监听器', 'Listener').zh },
  {
    key: 'online',
    label: zh('状态', 'Status').zh,
    value: (r) => (r.online === '1' ? '在线' : '离线'),
  },
]

export const SOCIAL_CLICK_COLUMNS: CsvColumn<Record<string, unknown>>[] = [
  { key: 'time', label: zh('时间', 'Time').zh },
  { key: 'resource', label: zh('资源', 'Resource').zh },
  { key: 'client_ip', label: zh('来源 IP', 'Source IP').zh },
  { key: 'user_agent', label: zh('User-Agent', 'User-Agent').zh },
  { key: 'status_code', label: zh('状态码', 'Status').zh },
]

export const TTP_TECHNIQUE_COLUMNS: CsvColumn<Record<string, unknown>>[] = [
  { key: 'tactic', label: zh('战术', 'Tactic').zh },
  { key: 'technique', label: zh('技术 (MITRE)', 'Technique (MITRE)').zh },
  { key: 'count', label: zh('次数', 'Count').zh },
]

export const TTP_SESSION_COLUMNS: CsvColumn<Record<string, unknown>>[] = [
  { key: 'host', label: zh('主机', 'Host').zh },
  { key: 'command', label: zh('命令', 'Command').zh },
  { key: 'tactic', label: zh('战术', 'Tactic').zh },
]

/**
 * iocToCSV 威胁指标报告 → CSV。
 *
 * 会话主机表是主体；监听地址/封禁 IP/登录失败这类**一维列表**放在文件尾部
 * 用单独的区块表示（而不是塞进主机表的一列 —— 那会让行数与主机数对不上）。
 */
export function iocToCSV(r: IOCReport): string {
  let out = toCSV(r.hosts || [], IOC_HOST_COLUMNS)
  const sections: Array<[string, string[] | undefined]> = [
    ['监听地址', r.listener_addresses],
    ['封禁 IP', r.blocked_ips],
    ['登录失败', r.login_failures],
  ]
  for (const [title, list] of sections) {
    if (!list || list.length === 0) continue
    out += '\r\n' + csvCell(title) + '\r\n'
    out += list.map((v) => csvCell(v)).join('\r\n') + '\r\n'
  }
  return out
}

/** socialToCSV 社会工程报告 → CSV（投递项 + 点击明细两段） */
export function socialToCSV(r: SocialReport): string {
  let out = ''
  if (r.deliveries && r.deliveries.length) {
    out += csvCell('投递项') + '\r\n'
    out += toCSV(r.deliveries, [
      { key: 'name', label: '名称' },
      { key: 'url', label: 'URL' },
      { key: 'status', label: '状态' },
    ])
    out += '\r\n'
  }
  out += toCSV(r.clicks || [], SOCIAL_CLICK_COLUMNS)
  return out
}

/** ttpToCSV 战术报告 → CSV（MITRE 技术统计 + 行为明细两段） */
export function ttpToCSV(r: TTPReport): string {
  let out = csvCell('MITRE 技术统计') + '\r\n'
  out += toCSV(r.techniques || [], TTP_TECHNIQUE_COLUMNS)
  if (r.per_session && r.per_session.length) {
    out += '\r\n' + csvCell('行为明细') + '\r\n'
    out += toCSV(r.per_session, TTP_SESSION_COLUMNS)
  }
  return out
}

/** 导出文件名：带时间戳，避免多次导出互相覆盖 */
export function reportFileName(kind: string, ext: string, now = new Date()): string {
  const p = (n: number) => String(n).padStart(2, '0')
  const stamp =
    `${now.getFullYear()}${p(now.getMonth() + 1)}${p(now.getDate())}` +
    `_${p(now.getHours())}${p(now.getMinutes())}${p(now.getSeconds())}`
  return `${kind}_report_${stamp}.${ext}`
}
