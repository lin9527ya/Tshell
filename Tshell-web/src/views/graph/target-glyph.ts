/**
 * 目标（target）节点的图标：**按端口推测服务类型**。
 *
 * 目标节点只有一个 IP:端口，没有客户端那样的系统元数据，所以只能从端口推。
 * 这是**推测**而不是事实（8080 也可能是别的服务），图例里明说了这点，
 * 详情面板里也把服务名标成"按端口推测" —— 不能让推测看起来像上报值。
 *
 * 与 os-glyph 同一套画法：以 (0,0) 为中心的 16×16 方框，单色填充，最远点 < 8（节点圆半径 9）。
 * 空心图形（准星、地球）用**反向缠绕的子路径**挖空：nonzero 填充规则下，
 * 外圈顺时针 (+1) 与内圈逆时针 (-1) 叠加为 0，就是洞。改方向会把洞填死。
 */
import type { Glyph } from './os-glyph'

/** Windows 服务（SMB / RDP / WinRM —— 这些都是 Windows 专有端口，推测可靠） */
const WIN_D = 'M-5.5 -5.5h5v5h-5z M0.5 -5.5h5v5h-5z M-5.5 0.5h5v5h-5z M0.5 0.5h5v5h-5z'

/** 远程登录：终端提示符（> 与下划线） */
const TERMINAL_D = [
  // 粗折角：外沿 A→B→C，内沿再折回来（厚度 1.5）
  'M-4.4 -3.5L-0.8 0L-4.4 3.5L-5.45 2.42L-2.95 0L-5.45 -2.42z',
  'M-0.8 3.4h5.6v1.6h-5.6z',
].join('')

/**
 * Web：地球（外圈 + 经线 + 赤道，三条"环"）。
 * 只画一条经线时看着像眼睛，加上赤道才成球；三条环在中心交叉处会互相抵消出一个小孔，
 * 正好是线框地球的样子，不用特意补。
 */
const WEB_D = [
  'M0 -6.3a6.3 6.3 0 1 1 0 12.6a6.3 6.3 0 1 1 0 -12.6z', // 外圈（顺时针）
  'M0 -5.1a5.1 5.1 0 1 0 0 10.2a5.1 5.1 0 1 0 0 -10.2z', // 内圈（逆时针）→ 挖空
  'M0 -5a1.7 5 0 1 1 0 10a1.7 5 0 1 1 0 -10z', // 经线外沿
  'M0 -4.2a1 4.2 0 1 0 0 8.4a1 4.2 0 1 0 0 -8.4z', // 经线内沿 → 挖空
  'M0 -1.7a4.6 1.7 0 1 1 0 3.4a4.6 1.7 0 1 1 0 -3.4z', // 赤道外沿
  'M0 -0.8a4.3 0.8 0 1 0 0 1.6a4.3 0.8 0 1 0 0 -1.6z', // 赤道内沿 → 挖空
].join('')

/** 数据库：三层叠放的圆柱（经典 DB 图标） */
const DB_D = [
  'M-5.2 -5.9h10.4a1.5 1.5 0 0 1 0 3h-10.4a1.5 1.5 0 0 1 0 -3z',
  'M-5.2 -1.9h10.4a1.5 1.5 0 0 1 0 3h-10.4a1.5 1.5 0 0 1 0 -3z',
  'M-5.2 2.1h10.4a1.5 1.5 0 0 1 0 3h-10.4a1.5 1.5 0 0 1 0 -3z',
].join('')

/** 其它端口：准星（环 + 中心点 + 四个刻度） */
const GENERIC_D = [
  'M0 -6a6 6 0 1 1 0 12a6 6 0 1 1 0 -12z', // 环外沿（顺时针）
  'M0 -3.6a3.6 3.6 0 1 0 0 7.2a3.6 3.6 0 1 0 0 -7.2z', // 环内沿（逆时针）→ 挖空
  'M0 -1.3a1.3 1.3 0 1 1 0 2.6a1.3 1.3 0 1 1 0 -2.6z', // 中心点
  'M-0.9 -7.6h1.8v2.4h-1.8z', // 上刻度
  'M-0.9 5.2h1.8v2.4h-1.8z', // 下刻度
  'M-7.6 -0.9h2.4v1.8h-2.4z', // 左刻度
  'M5.2 -0.9h2.4v1.8h-2.4z', // 右刻度
].join('')

export type TargetGlyphKey = 'windows' | 'terminal' | 'web' | 'database' | 'generic'

export const TARGET_GLYPHS: Record<TargetGlyphKey, Glyph> = {
  windows: { d: WIN_D, zh: 'Windows 服务', en: 'Windows service' },
  terminal: { d: TERMINAL_D, zh: '远程登录', en: 'Remote shell' },
  web: { d: WEB_D, zh: 'Web 服务', en: 'Web service' },
  database: { d: DB_D, zh: '数据库', en: 'Database' },
  generic: { d: GENERIC_D, zh: '其它端口', en: 'Other port' },
}

interface PortRule {
  ports: number[]
  key: TargetGlyphKey
  /** 具体服务名（详情面板里展示，标注为"按端口推测"） */
  service: { zh: string; en: string }
}

/**
 * 端口 → 服务。顺序即优先级（同名端口只出现一次，这里靠 port 查找，顺序无关）。
 * 只收录**指向明确**的端口：445 基本就是 SMB，3389 就是 RDP；
 * 拿不准的一律落到 generic（准星），不硬猜。
 */
const PORT_RULES: PortRule[] = [
  { ports: [445, 139], key: 'windows', service: { zh: 'SMB 文件共享', en: 'SMB' } },
  { ports: [135], key: 'windows', service: { zh: 'RPC', en: 'RPC' } },
  { ports: [3389], key: 'windows', service: { zh: 'RDP 远程桌面', en: 'RDP' } },
  { ports: [5985, 5986], key: 'windows', service: { zh: 'WinRM', en: 'WinRM' } },
  { ports: [22, 2222], key: 'terminal', service: { zh: 'SSH', en: 'SSH' } },
  { ports: [23], key: 'terminal', service: { zh: 'Telnet', en: 'Telnet' } },
  { ports: [512, 513, 514], key: 'terminal', service: { zh: 'rsh/rlogin', en: 'rsh/rlogin' } },
  { ports: [80, 8000, 8080, 8888, 7001, 5000, 9000], key: 'web', service: { zh: 'HTTP', en: 'HTTP' } },
  { ports: [443, 8443, 9443], key: 'web', service: { zh: 'HTTPS', en: 'HTTPS' } },
  { ports: [3306], key: 'database', service: { zh: 'MySQL', en: 'MySQL' } },
  { ports: [5432], key: 'database', service: { zh: 'PostgreSQL', en: 'PostgreSQL' } },
  { ports: [1433], key: 'database', service: { zh: 'SQL Server', en: 'SQL Server' } },
  { ports: [1521], key: 'database', service: { zh: 'Oracle', en: 'Oracle' } },
  { ports: [6379], key: 'database', service: { zh: 'Redis', en: 'Redis' } },
  { ports: [27017], key: 'database', service: { zh: 'MongoDB', en: 'MongoDB' } },
  { ports: [9200, 9300], key: 'database', service: { zh: 'Elasticsearch', en: 'Elasticsearch' } },
  { ports: [11211], key: 'database', service: { zh: 'Memcached', en: 'Memcached' } },
  { ports: [21], key: 'generic', service: { zh: 'FTP', en: 'FTP' } },
  { ports: [25, 465, 587], key: 'generic', service: { zh: 'SMTP', en: 'SMTP' } },
  { ports: [110, 995], key: 'generic', service: { zh: 'POP3', en: 'POP3' } },
  { ports: [143, 993], key: 'generic', service: { zh: 'IMAP', en: 'IMAP' } },
  { ports: [161], key: 'generic', service: { zh: 'SNMP', en: 'SNMP' } },
  { ports: [389, 636], key: 'generic', service: { zh: 'LDAP', en: 'LDAP' } },
]

const byPort = new Map<number, PortRule>()
for (const r of PORT_RULES) for (const p of r.ports) byPort.set(p, r)

/**
 * portOf 从目标地址里取端口。地址形如 10.0.0.5:445、[::1]:80、host:8080，
 * 也可能没有端口（纯 IP）——那时返回 null。
 */
export function portOf(address: string): number | null {
  const s = (address || '').trim()
  if (!s) return null
  const m = s.match(/:(\d{1,5})$/)
  if (!m) return null
  const n = Number(m[1])
  return Number.isFinite(n) && n > 0 && n <= 65535 ? n : null
}

/** 目标节点的图标（按端口推测；认不出的端口给准星，不硬猜） */
export function targetGlyphKey(address: string): TargetGlyphKey {
  const port = portOf(address)
  if (port === null) return 'generic'
  return byPort.get(port)?.key ?? 'generic'
}

/** 目标节点的服务名（推测），详情面板用；认不出的端口返回 null */
export function targetService(address: string): { zh: string; en: string } | null {
  const port = portOf(address)
  if (port === null) return null
  return byPort.get(port)?.service ?? null
}
