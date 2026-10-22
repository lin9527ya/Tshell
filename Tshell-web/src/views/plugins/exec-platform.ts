// 插件执行模式的平台约束（纯函数，供 UI gating 与单测使用）。
//
// 服务端 /bin/execute 会按客户端 Os 做同样的校验；这里只负责把不可用的
// 模式提前置灰，避免操作员点了才发现客户端根本不支持。

export type ExecOS = 'windows' | 'linux' | 'darwin' | ''

const MODE_PLATFORMS: Record<string, ExecOS[]> = {
  'execute-assembly': ['windows'],
  'inline-bin': ['windows'],
  'shellcode-inject': ['windows'],
  bof: ['windows'],
  script: ['linux'],
  binary: ['linux'],
  'exec-file': ['windows', 'linux'],
}

/** 归一化客户端 OS：优先 DB 的 Os；缺失时才退回 Arch/主机名启发式。 */
export function normalizeClientOs(rawOs?: string, arch?: string, computer?: string): ExecOS {
  const os = String(rawOs || '').toLowerCase().trim()
  if (os === 'windows' || os === 'linux' || os === 'darwin') return os
  if (os) {
    if (os.includes('win')) return 'windows'
    if (os.includes('linux')) return 'linux'
    if (os.includes('darwin') || os.includes('mac')) return 'darwin'
  }
  const a = String(arch || '').toLowerCase()
  const c = String(computer || '').toLowerCase()
  if (a.includes('linux') || c.includes('linux')) return 'linux'
  if (a.includes('darwin') || a.includes('mac') || c.includes('darwin') || c.includes('mac')) return 'darwin'
  // 未知：不激进禁用（老数据没有 Os），交给服务端 400 兜底
  return ''
}

/** 该模式允许的客户端系统；未知模式返回空数组。 */
export function execModePlatforms(mode: string): ExecOS[] {
  return MODE_PLATFORMS[mode] || []
}

/** 模式是否允许在该系统上执行；os 为空（未知）时放行。 */
export function isModeAllowed(mode: string, os: ExecOS): boolean {
  if (!os) return true
  const allowed = MODE_PLATFORMS[mode]
  if (!allowed || allowed.length === 0) return false
  return allowed.includes(os)
}
