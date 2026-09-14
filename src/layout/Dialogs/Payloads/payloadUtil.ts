// 载荷生成共享工具：真实监听器拉取、GenServer 载荷生成、blob 下载
// 供 Dialogs/Payloads 与 Dialogs/Attacks/WebDeliveryDialog 复用（对齐 ServerGen 的集成方式）
import ClientAPI from '@/api/clients'
import { ElMessage } from 'element-plus'

export interface ListenerOption {
  name: string // protocol://connectAddress（GenServer 需要的 listener 串）
  payload: string // listenAddress（表格展示）
  host: string
  port: number | string
  protocol: string
  connectAddress: string
  id?: number | string
  vkey?: string // /listener/list 返回完整密钥，选择器直接携带
  encryptSalt?: string
  status?: number // 1=运行中 2=未运行
}

export interface WebDeliveryRow {
  ListeningPort?: string
  listening_port?: string
  OS?: string
  os?: string
  Arch?: string
  arch?: string
  ListenerConfig?: string
  listener_config?: string
  Status?: number
  status?: number
  ServerAddress?: string
  server_address?: string
  [key: string]: any
}

// 兼容驼峰/下划线字段（与 WebDelivery.vue 的 field() 一致）
export function field(row: any, camel: string, snake: string): any {
  if (!row) return undefined
  return row[camel] !== undefined ? row[camel] : row[snake]
}

function hostOf(address: string): string {
  const a = String(address || '')
  const idx = a.lastIndexOf(':')
  return idx > 0 ? a.slice(0, idx) : a
}

function portOf(address: string): number | string {
  const a = String(address || '')
  const idx = a.lastIndexOf(':')
  return idx > 0 ? a.slice(idx + 1) : ''
}

// 拉取监听器列表（仅显示运行中的监听器：Status === 1）
export async function fetchListenerOptions(onlyRunning = true): Promise<ListenerOption[]> {
  try {
    const mod = await import('@/api/listener')
    const res = await mod.default.getListenerList()
    if (res.status === 200 && res.data?.status === 200 && Array.isArray(res.data.data)) {
      const list = onlyRunning
        ? res.data.data.filter((l: any) => Number(l.Status) === 1)
        : res.data.data
      return list.map((l: any) => ({
        name: `${l.Type}://${l.ConnectAddress}`,
        payload: l.ListenAddress || '',
        host: hostOf(l.ConnectAddress),
        port: portOf(l.ConnectAddress),
        protocol: l.Type || '',
        connectAddress: l.ConnectAddress || '',
        id: l.Id,
        vkey: l.VKey || undefined,
        encryptSalt: l.EncryptionSalt || undefined,
        status: l.Status,
      }))
    }
  } catch {
    /* 回退 */
  }
  try {
    const res = await ClientAPI.ShowListenerDetails()
    if (res.status === 200 && res.data?.status === 200 && Array.isArray(res.data.data)) {
      const list = onlyRunning
        ? res.data.data.filter((d: any) => d.status === undefined || Number(d.status) === 1 || Number(d.Status) === 1)
        : res.data.data
      return list.map((d: any) => ({
        name: d.url || `${d.protocol}://${d.connectAddress}`,
        payload: d.listenAddress || '',
        host: hostOf(d.connectAddress),
        port: portOf(d.connectAddress),
        protocol: d.protocol || '',
        connectAddress: d.connectAddress || '',
        id: d.id,
        status: 1,
      }))
    }
  } catch {
    /* 无监听器 */
  }
  try {
    const res = await ClientAPI.ShowListener()
    if (res.status === 200 && res.data?.status === 200 && Array.isArray(res.data.data)) {
      return res.data.data.map((u: string) => {
        const [protocol, address] = String(u).split('://')
        return {
          name: u,
          payload: u,
          host: hostOf(address),
          port: portOf(address),
          protocol: protocol || '',
          connectAddress: address || '',
          status: 1,
        }
      })
    }
  } catch {
    /* 无监听器 */
  }
  return []
}

// 手动输入监听器地址时按全量列表匹配 VKey/Salt。
// 匹配不到时直接报错（不再回退公开默认密钥——后端 J4 会拒绝默认密钥，
// 且用默认密钥生成的载荷与服务端会话密钥不一致，上线即解密失败）
export async function fetchListenerKeys(listenerUrl: string): Promise<{ vkey: string; encryptSalt: string }> {
  try {
    const mod = await import('@/api/listener')
    const res = await mod.default.getListenerList()
    if (res.status === 200 && res.data?.status === 200 && Array.isArray(res.data.data)) {
      const [proto, addr] = String(listenerUrl).split('://')
      const l = res.data.data.find(
        (x: any) =>
          String(x.ConnectAddress) === String(addr) &&
          String(x.Type).toLowerCase() === String(proto).toLowerCase(),
      )
      if (l && l.VKey && l.EncryptionSalt) {
        return { vkey: l.VKey, encryptSalt: l.EncryptionSalt }
      }
    }
  } catch {
    /* 落到底部统一报错 */
  }
  throw new Error('无法匹配监听器密钥：请从监听器选择器中选择，或确认该监听器已配置自定义 VKey/加密盐')
}

// blob 下载（后端错误可能以 JSON blob 返回，需区分；allowJson=true 时 JSON 也作为下载内容）
export function downloadBlob(res: any, fallbackName: string, allowJson = false): boolean {
  let fileName = fallbackName
  const contentDisposition = res.headers?.['content-disposition'] || ''
  const matches = /filename[^;=\n]*=((['"]).*?\2|[^;\n]*)/.exec(contentDisposition)
  if (matches && matches.length > 1) {
    fileName = decodeURIComponent(matches[1].replace(/['"]/g, ''))
  }
  const ctype = (res.headers?.['content-type'] || '').toLowerCase()
  if ((ctype.includes('application/json') || ctype.includes('text/json')) && !allowJson) {
    return false
  }
  const blob = new Blob([res.data], { type: res.headers?.['content-type'] || 'application/octet-stream' })
  const url = window.URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = fileName
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  window.URL.revokeObjectURL(url)
  return true
}

export async function extractBlobError(error: any): Promise<string> {
  try {
    const data = error?.response?.data
    if (data instanceof Blob) {
      const text = await data.text()
      try {
        const json = JSON.parse(text)
        return json.data || json.error || json.message || text
      } catch {
        return text || error.message
      }
    }
    if (typeof data === 'object' && data) {
      return data.data || data.error || data.message || error.message
    }
    return error?.message || String(error)
  } catch {
    return error?.message || String(error)
  }
}

// GenServer 全流程：匹配监听器密钥 → 生成 → 下载
// 返回 true=已下载；抛错=失败（错误文案可直接展示）
// format: exe/dll/raw = 完整程序；bin/hex/c = 无阶段 shellcode（仅 windows）
export async function generateClientBinary(opts: {
  listener: string
  osType: string
  archType: string
  pass?: string
  format?: 'exe' | 'dll' | 'raw' | 'bin' | 'hex' | 'c'
  vkey?: string // 选择器携带的密钥（优先），否则按地址匹配
  encryptSalt?: string
  // OPSEC 编译期配置（可选）
  reconnectBaseDelay?: string
  reconnectMaxRetries?: string
  injectTech?: string
  masqueradeName?: string
  quietMode?: string
  etwPatch?: string
  amsiBypass?: string
  syscallMode?: string
  autoDelete?: string
  profileName?: string
}): Promise<boolean> {
  const blob = await generateClientBinaryBlob(opts)
  if (!blob) return false
  const isWin = opts.osType.toLowerCase() === 'windows'
  const extMap: Record<string, string> = {
    exe: isWin ? 'exe' : '',
    dll: 'dll',
    raw: isWin ? 'exe' : '',
    bin: 'bin',
    hex: 'txt',
    c: 'c',
  }
  const ext = extMap[opts.format || 'exe'] || 'bin'
  const proto = String(opts.listener).split('://')[0] || 'client'
  const fname = `${proto}_${opts.osType}_${opts.archType}_${Date.now()}${ext ? '.' + ext : ''}`
  const fakeRes = { data: blob, headers: {} }
  return downloadBlob(fakeRes as any, fname)
}

// generateClientBinaryBlob 生成载荷并返回 Blob（不触发下载；供上传/注入迁移等流程复用）
export async function generateClientBinaryBlob(opts: {
  listener: string
  osType: string
  archType: string
  pass?: string
  format?: 'exe' | 'dll' | 'raw' | 'bin' | 'hex' | 'c'
  vkey?: string
  encryptSalt?: string
  reconnectBaseDelay?: string
  reconnectMaxRetries?: string
  injectTech?: string
  masqueradeName?: string
  quietMode?: string
  profileName?: string
}): Promise<Blob | null> {
  let keys: { vkey: string; encryptSalt: string }
  if (opts.vkey && opts.encryptSalt) {
    keys = { vkey: opts.vkey, encryptSalt: opts.encryptSalt }
  } else {
    keys = await fetchListenerKeys(opts.listener)
  }
  const res = await ClientAPI.GenServer({
    osType: opts.osType,
    archType: opts.archType,
    listener: opts.listener,
    pass: opts.pass || '',
    vkey: keys.vkey,
    encryptSalt: keys.encryptSalt,
    format: opts.format,
    reconnectBaseDelay: opts.reconnectBaseDelay || '',
    reconnectMaxRetries: opts.reconnectMaxRetries || '',
    injectTech: opts.injectTech || '',
    masqueradeName: opts.masqueradeName || '',
    quietMode: opts.quietMode || '',
    profileName: opts.profileName || '',
  })
  if (!res || res.status !== 200) {
    throw new Error('生成失败，请检查后端日志')
  }
  const ctype = (res.headers?.['content-type'] || '').toLowerCase()
  if (ctype.includes('json')) {
    const text = await (res.data as Blob).text()
    let json: any = null
    try {
      json = JSON.parse(text)
    } catch {
      /* 非 JSON */
    }
    throw new Error(json?.data || json?.error || '生成失败')
  }
  return (res.data as Blob) || null
}

// WebDelivery 列表（stage/stager/hta/宏投递的前置条件，仅返回运行中实例）
export async function fetchWebDeliveryOptions(onlyRunning = true): Promise<WebDeliveryRow[]> {
  try {
    const res = await ClientAPI.GetWebDeliveryList()
    if (res.status === 200 && res.data?.status === 200) {
      const list: WebDeliveryRow[] = res.data.data || []
      return onlyRunning
        ? list.filter((w) => Number(field(w, 'Status', 'status')) === 1)
        : list
    }
  } catch {
    /* 空列表 */
  }
  return []
}

export function wdLabel(wd: WebDeliveryRow): string {
  const status = Number(field(wd, 'Status', 'status')) === 1 ? '运行中' : '已关闭'
  const port = field(wd, 'ListeningPort', 'listening_port') || '-'
  const os = `${field(wd, 'OS', 'os') || ''}_${field(wd, 'Arch', 'arch') || ''}`
  const listener = field(wd, 'ListenerConfig', 'listener_config') || '-'
  return `端口 ${port} | ${os} | ${listener} | ${status}`
}

// 公开提示：无运行中监听器 / 无 WebDelivery 时引导去对应页面
export function hintNoListeners(): void {
  ElMessage.warning('暂无运行中的监听器，请先到「监听器」页面创建并启动')
}

export function hintNoWebDelivery(): void {
  ElMessage.warning('暂无 WebDelivery，请先到「Web Delivery」页面创建并启动')
}
