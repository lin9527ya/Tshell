/**
 * 目标（target）节点的图标：**按端口推测服务类型**。
 *
 * 目标节点只有一个 IP:端口，没有客户端那样的系统元数据，所以只能从端口推。
 * 这是**推测**而不是事实（8080 也可能是别的服务），图例里明说了这点，
 * 详情面板里也把服务名标成"按端口推测" —— 不能让推测看起来像事实。
 *
 * 图标取自项目图标集（FontAwesome 同源）与公开图标集，不再手绘：
 * 手绘的箭头/地球/圆柱在小尺寸下细节经不起看，形状也"不像那个东西"。
 */
import { type Glyph } from './glyph'
import { vsIconPath } from '@/layout-vshell/vs-icons'
import { OS_GLYPHS } from './os-glyph'

function fromIconSet(name: string, zh: string, en: string, fallback: string): Glyph {
  const p = vsIconPath(name)
  return { d: p?.d || fallback, vb: p?.vb || '0 0 16 16', zh, en }
}

export type TargetGlyphKey = 'windows' | 'terminal' | 'web' | 'database' | 'generic'

export const TARGET_GLYPHS: Record<TargetGlyphKey, Glyph> = {
  // SMB/RDP/WinRM 都是 Windows 专有端口，直接用系统图标
  windows: OS_GLYPHS.windows,
  terminal: fromIconSet('terminal', '远程登录', 'Remote shell', ''),
  web: fromIconSet('globe', 'Web 服务', 'Web service', ''),
  // fa-solid:database（项目图标集里没有数据库图标，单独取一个同源的）
  database: {
    vb: '0 0 448 512',
    d: 'M448 73.143v45.714C448 159.143 347.667 192 224 192S0 159.143 0 118.857V73.143C0 32.857 100.333 0 224 0s224 32.857 224 73.143M448 176v102.857C448 319.143 347.667 352 224 352S0 319.143 0 278.857V176c48.125 33.143 136.208 48.572 224 48.572S399.874 209.143 448 176m0 160v102.857C448 479.143 347.667 512 224 512S0 479.143 0 438.857V336c48.125 33.143 136.208 48.572 224 48.572S399.874 369.143 448 336',
    zh: '数据库',
    en: 'Database',
  },
  generic: fromIconSet('crosshairs', '其它端口', 'Other port', ''),
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
