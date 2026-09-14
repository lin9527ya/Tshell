import request from '@/utils/request'

// 反向 SOCKS5 代理 API 封装（后端 pkg/api/socks5.go）
export interface Socks5Request {
  client_uid: string
  protocol?: string
  server_port: number
  note?: string
  username?: string
  password?: string
}

export interface Socks5Response {
  id: number
  client_uid: string
  protocol: string
  server_port: number
  agent_port: number
  note: string
  username: string
  password: string // 脱敏后
  status: number // 1=开启 2=关闭
  created_at: string
  updated_at: string
}

const Socks5API = {
  // 获取 SOCKS5 代理列表
  list(): Promise<any> {
    return request({
      url: '/socks5/list',
      method: 'get'
    })
  },

  // 创建并启动 SOCKS5 代理
  start(data: Socks5Request): Promise<any> {
    return request({
      url: '/socks5/start',
      method: 'post',
      data
    })
  },

  // 重新开启（按 id）
  open(id: number): Promise<any> {
    return request({
      url: '/socks5/open',
      method: 'post',
      data: { id }
    })
  },

  // 关闭（按 id）
  close(id: number): Promise<any> {
    return request({
      url: '/socks5/close',
      method: 'post',
      data: { id }
    })
  },

  // 删除（按 id）
  delete(id: number): Promise<any> {
    return request({
      url: '/socks5/delete',
      method: 'post',
      data: { id }
    })
  },

  // 批量删除
  batchDelete(ids: number[]): Promise<any> {
    return request({
      url: '/socks5/batch-delete',
      method: 'post',
      data: { ids }
    })
  },

  // 更新（仅更新非空字段；密码留空视为不修改）
  update(id: number, data: Partial<Socks5Request>): Promise<any> {
    return request({
      url: '/socks5/update',
      method: 'put',
      data: { id, ...data }
    })
  }
}

export default Socks5API
