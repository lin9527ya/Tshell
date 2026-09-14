import request from '@/utils/request'

// 通知配置接口类型定义
export interface NotificationConfig {
  enable: boolean
  enable_audio: boolean
  dingtalk: {
    enable: boolean
    token: string
    keyword: string
    secret: string
  }
  wecom: {
    enable: boolean
    wx_key: string
  }
  feishu: {
    enable: boolean
    webhook: string
    secret: string
    keyword: string
  }
}

// API响应类型
export interface ApiResponse<T = any> {
  success: boolean
  code: number
  message: string
  data?: T
  detail?: string
  request_id?: string
  timestamp: number
}

/**
 * 获取通知配置
 */
export const getNotificationConfig = () => {
  return request.get<ApiResponse<NotificationConfig>>('/notification/config')
}

/**
 * 更新通知配置
 * @param config 通知配置
 */
export const updateNotificationConfig = (config: NotificationConfig) => {
  return request.post<ApiResponse>('/notification/config', config)
}

/**
 * 测试通知
 */
export const testNotification = () => {
  return request.post<ApiResponse>('/notification/test')
}