import type { RouteLocationNormalizedLoaded } from 'vue-router'

/**
 * 从路由中获取客户端UID
 * 新的路由格式直接在路径中使用UID: /clientManager/:uid
 * 
 * @param route - Vue Router的当前路由对象
 * @returns 客户端UID字符串，如果未找到则返回空字符串
 */
export function useClientUid(route: RouteLocationNormalizedLoaded): string {
  if (!route || !route.params) {
    return ''
  }

  const uidFromParams = route.params.uid

  if (Array.isArray(uidFromParams)) {
    return uidFromParams[0] || ''
  }

  return (uidFromParams as string) || ''
}

/**
 * 智能探测客户端操作系统类型（Windows / Linux / Darwin / Unknown）
 * 综合分析 Os, Arch, Computer, Username 等多维度特征
 */
export function detectClientOs(client: any): 'windows' | 'linux' | 'darwin' | 'unknown' {
  if (!client) return 'unknown'
  const os = String(client.Os || client.os || '').toLowerCase()
  const arch = String(client.Arch || client.arch || '').toLowerCase()
  const comp = String(client.Computer || client.computer || '').toLowerCase()
  const user = String(client.Username || client.username || '').toLowerCase()

  // 1. 明确的 OS 字段优先级最高
  if (
    os.includes('linux') ||
    os.includes('ubuntu') ||
    os.includes('debian') ||
    os.includes('centos') ||
    os.includes('kali') ||
    os.includes('arch') ||
    os.includes('redhat') ||
    os.includes('fedora') ||
    os.includes('alpine')
  ) {
    return 'linux'
  }
  // darwin 必须放在 windows 之前：字符串 "darwin" 本身包含子串 "win"
  if (os.includes('darwin') || os.includes('mac') || os.includes('apple') || os.includes('osx')) {
    return 'darwin'
  }
  if (os.includes('win')) {
    return 'windows'
  }

  // 2. 架构 / 载荷类型判断 (linux_amd64, linux_arm64, elf 等)
  if (arch.includes('linux') || arch.includes('elf') || arch.includes('arm') || arch.includes('aarch64')) {
    return 'linux'
  }
  if (arch.includes('darwin') || arch.includes('macho') || arch.includes('apple')) {
    return 'darwin'
  }
  if (arch.includes('win') || arch.includes('pe') || arch.includes('dll') || arch.includes('exe')) {
    return 'windows'
  }

  // 3. 主机名与用户名特征判断
  if (
    comp.includes('(linux)') ||
    comp.includes('kali') ||
    comp.includes('ubuntu') ||
    comp.includes('debian') ||
    comp.includes('centos') ||
    comp.includes('arch') ||
    comp.includes('linux')
  ) {
    return 'linux'
  }
  if (comp.includes('win') || comp.startsWith('win-') || comp.startsWith('desktop-') || comp.startsWith('laptop-')) {
    return 'windows'
  }
  if (user === 'root' || user.includes('/root') || user.includes('kali') || user.includes('ubuntu')) {
    return 'linux'
  }
  if (user.includes('system') || user.includes('administrator') || user.includes('\\')) {
    return 'windows'
  }

  // 4. 兜底判断
  if (arch.includes('x64') || arch.includes('x86') || arch.includes('386') || arch.includes('amd64')) {
    return 'windows'
  }

  return 'unknown'
}

/** 系统图标名（与 vshell 的 devicon / wpf 图标一致，VsIcon 已内置） */
export function clientOsIcon(client: any): string {
  switch (detectClientOs(client)) {
    case 'windows':
      return 'devicon-windows8'
    case 'linux':
      return 'devicon-linux'
    case 'darwin':
      return 'wpf-macos'
    default:
      return 'desktop'
  }
}

/**
 * 客户端详情的顶部标签标题：
 *   主机管理 → "ID:3 客户端管理"（vshell ClientManagerTab 命名）
 *   文件管理 → "3: 外网IP"（vshell FileTree 节点命名）
 */
export function formatClientTabTitle(opts: {
  func: string
  id: string
  externalIp: string
  zh: boolean
}): string {
  if (opts.func === 'files') return `${opts.id}: ${opts.externalIp || '-'}`
  return opts.zh ? `ID:${opts.id} 客户端管理` : `ID:${opts.id} Client Management`
}
