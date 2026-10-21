import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import ClientAPI from '@/api/clients'

export interface ClientData {
  ID?: number // 数据库自增ID
  id?: number // 兼容小写字段
  Uid: string
  FirstStart: string
  ExternalIP: string
  InternalIP: string
  Listener: string
  Username: string
  Computer: string
  Process: string
  Pid: string
  Address: string
  Arch: string
  Note: string
  Sleep: string
  Online: string
  Color: string
  Group?: string
  HeartbeatTimeout?: string // 心跳超时次数，默认30次
  HeartbeatInterval?: string // 心跳检测间隔（秒），默认10秒
  // 【类型补全】以下字段由后端 returns/事件 payload 携带,此前类型缺失导致多处 TS 报错
  Os?: string // 操作系统(后端 clients.Os)
  ProcessName?: string // 进程名(事件 payload)
  Session?: string // 会话号(Session 0/1/2,默认 "1")
  LastActive?: string // 最后活跃时间(列表接口附加)
}

export const useClientsStore = defineStore('clients', () => {
  // 状态
  const clientsList = ref<ClientData[]>([])
  const loading = ref(false)
  const lastFetchTime = ref<Date | null>(null)

  // 计算属性
  const totalCount = computed(() => clientsList.value.length)
  const onlineCount = computed(() => 
    clientsList.value.filter(client => client.Online === '1').length
  )

  // 根据显示ID获取客户端数据
  const getClientByDisplayId = computed(() => {
    return (displayId: string) => {
      // 首先尝试直接匹配UID
      const directMatch = clientsList.value.find(client => client.Uid === displayId)
      if (directMatch) {
        return directMatch
      }
      
      // 如果不是直接匹配，可能是序号，尝试按索引获取
      const index = parseInt(displayId) - 1
      if (!isNaN(index) && index >= 0 && index < clientsList.value.length) {
        return clientsList.value[index]
      }
      
      return null
    }
  })

  // 根据显示ID获取真实UID
  const getUidByDisplayId = computed(() => {
    return (displayId: string) => {
      const client = getClientByDisplayId.value(displayId)
      return client?.Uid || displayId
    }
  })

  // 获取客户端列表
  const fetchClients = async (page: number = 1, pageSize: number = 100) => {
    loading.value = true
    try {
      const response = await ClientAPI.get_clients_list({ page, page_size: pageSize })

      // 【统一响应格式】后端契约 {success, code, message, data, timestamp}，
      // 拦截器已归一化为 {status, data}；data 恒为 {list, total}，无需多格式猜测。
      // 仅对旧服务端（未部署包装中间件）保留兜底兼容。
      let clientsData = null
      if (response.data.status === 200 && response.data.data && response.data.data.list) {
        clientsData = response.data.data.list
      } else if (Array.isArray(response.data)) {
        clientsData = response.data
      } else if (response.data.list && Array.isArray(response.data.list)) {
        clientsData = response.data.list
      } else {
        console.warn('无法识别的客户端数据格式')
        clientsList.value = []
        return
      }
      
      // 验证数据是否为数组
      if (!Array.isArray(clientsData)) {
        console.warn('客户端数据格式错误')
        clientsList.value = []
        return
      }
      
      // 设置客户端列表（优化：保持现有客户端对象引用稳定，避免定时拉取时触发全量重新挂载或丢失状态）
      if (clientsList.value.length === 0) {
        clientsList.value = clientsData
      } else {
        const oldMap = new Map(clientsList.value.map((c: ClientData) => [c.Uid, c]))
        clientsList.value = clientsData.map((newClient: ClientData) => {
          const oldClient = oldMap.get(newClient.Uid)
          if (oldClient) {
            Object.assign(oldClient, newClient)
            return oldClient
          }
          return newClient
        })
      }
      lastFetchTime.value = new Date()
      
      if (clientsData.length === 0) {
        console.warn('客户端列表为空')
      }
    } catch (error: any) {
      console.error('获取客户端列表失败:', error.message || error)
      clientsList.value = []
      throw error
    } finally {
      loading.value = false
    }
  }

  // 刷新客户端列表
  const refreshClients = async () => {
    await fetchClients()
  }

  // 根据UID获取客户端
  const getClientByUid = (uid: string) => {
    return clientsList.value.find(client => client.Uid === uid)
  }

  // 根据UID更新客户端信息（就地合并，保持对象引用稳定：
  // fetchClients 特意复用旧对象避免表格整行重挂载，单行更新也不应替换引用）
  const updateClient = (uid: string, updates: Partial<ClientData>) => {
    const index = clientsList.value.findIndex(client => client.Uid === uid)
    if (index >= 0) {
      Object.assign(clientsList.value[index], updates)
    }
  }

  // 清空客户端列表
  const clearClients = () => {
    clientsList.value = []
    lastFetchTime.value = null
  }

  return {
    // 状态
    clientsList,
    loading,
    lastFetchTime,
    
    // 计算属性
    totalCount,
    onlineCount,
    getClientByDisplayId,
    getUidByDisplayId,
    
    // 方法
    fetchClients,
    refreshClients,
    getClientByUid,
    updateClient,
    clearClients
  }
})