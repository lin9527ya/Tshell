import request from '@/utils/request'

export interface DashboardStats {
  monitor_count: number
  connection_count: number
  user_count: number
  online_user_count: number
  config_info: {
    log_level: string
    log_file_path: string
    web_port: number
    basic_auth: boolean
    version: string
  }
  system_info: {
    cpu_usage: number
    disk_usage: number
    memory_usage: number
    virtual_memory: number
    tcp_connections: number
    udp_connections: number
    outbound_bw: string
    inbound_bw: string
  }
}

export const getDashboardStats = () => {
  return request({ url: '/dashboard/stats', method: 'GET' })
}
