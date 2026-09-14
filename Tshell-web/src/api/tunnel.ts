import request from '@/utils/request'

// 隧道代理API接口定义
export interface TunnelProxyRequest {
  client_uid: string
  protocol: string
  server_port: number
  target_address?: string // TCP/UDP代理的目标地址(IP:端口)，对SOCKS5/HTTP隧道可选
  note?: string
  username?: string
  password?: string
}

export interface TunnelProxyResponse {
  id: number
  client_uid: string
  protocol: string
  server_port: number
  target_address: string // TCP/UDP代理的目标地址
  note: string
  username: string
  password: string
  status: number
  created_at: string
  updated_at: string
  client_info?: {
    computer: string
    external_ip: string
    username: string
    online: string
    listener: string
  }
}

export interface TunnelProxyListResponse {
  data: TunnelProxyResponse[]
  total: number
  page: number
  page_size: number
  total_pages: number
}

const TunnelAPI = {
  // 创建隧道代理（使用统一隧道代理API）
  create(data: TunnelProxyRequest): Promise<any> {
    return request({
      url: '/tunnel-proxy/',
      method: 'post',
      data
    })
  },

  // 获取隧道代理列表（使用统一隧道代理API）
  list(params?: {
    page?: number
    page_size?: number
    client_uid?: string
    protocol?: string
    status?: string
  }): Promise<any> {
    return request({
      url: '/tunnel-proxy/',
      method: 'get',
      params
    })
  },

  // 启动隧道代理（使用统一隧道代理API）
  start(id: number): Promise<any> {
    return request({
      url: `/tunnel-proxy/${id}/start`,
      method: 'post'
    })
  },

  // 停止隧道代理（使用统一隧道代理API）
  stop(id: number): Promise<any> {
    return request({
      url: `/tunnel-proxy/${id}/stop`,
      method: 'post'
    })
  },

  // 删除隧道代理（使用统一隧道代理API）
  delete(id: number): Promise<any> {
    return request({
      url: `/tunnel-proxy/${id}`,
      method: 'delete'
    })
  },

  // 批量删除隧道代理（使用Promise.allSettled处理并发删除）
  batchDelete(ids: number[]): Promise<PromiseSettledResult<any>[]> {
    // 统一隧道API不支持批量删除，需要逐个删除
    const deletePromises = ids.map(id => this.delete(id))
    return Promise.allSettled(deletePromises)
  },

  // 更新隧道代理（使用统一隧道代理API）
  update(id: number, data: Partial<TunnelProxyRequest>): Promise<any> {
    return request({
      url: `/tunnel-proxy/${id}`,
      method: 'put',
      data: {
        client_uid: data.client_uid,
        protocol: data.protocol,
        server_port: data.server_port,
        target_address: data.target_address,
        note: data.note,
        username: data.username,
        password: data.password
      }
    })
  }
}

export default TunnelAPI