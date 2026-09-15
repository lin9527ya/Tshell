import request from '@/utils/request'

// Loot 凭据/制品库 API(功能批)
// 服务端表结构: { id, uid, type(pwd/hash/domain/note), user, realm, host, secret, note, source, created_at }

export interface LootItem {
  id: number
  uid: string
  type: string
  user: string
  realm: string
  host: string
  secret: string
  note: string
  source: string
  created_at: string
}

export function listLoot(params: { uid?: string; type?: string; page?: number; size?: number }) {
  return request({
    url: '/loot',
    method: 'get',
    params,
  })
}

export function addLoot(data: { uid?: string; type: string; user?: string; realm?: string; host?: string; secret?: string; note?: string; source?: string }) {
  return request({
    url: '/loot',
    method: 'post',
    data,
  })
}

export function deleteLoot(id: number) {
  return request({
    url: `/loot/${id}`,
    method: 'delete',
  })
}
