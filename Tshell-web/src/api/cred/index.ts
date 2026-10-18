import request from '@/utils/request'

// 凭据获取（后端 pkg/api/cred.go，仅 admin/operator）
// 链路：发起 → 客户端获取内存转储 → 经下载通道回传 → 服务端校验并登记为制品。
export interface CredArtifactRequest {
  uid: string
  kind: string
  file_name: string
  tech: string
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

export default {
  dump(data: { uid: string; kind?: string; tech?: string }) {
    return request({ url: '/cred/dump', method: 'POST', data })
  },
  listArtifacts(uid?: string) {
    return request({ url: '/cred/artifacts', method: 'GET', params: uid ? { uid } : {} })
  }
}
