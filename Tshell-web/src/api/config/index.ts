import request from '@/utils/request'

// 服务配置管理 API
export const getConfig = () => {
  return request({
    url: '/config/current',
    method: 'GET'
  })
}

export const updateConfig = (data: any) => {
  return request({
    url: '/config/update',
    method: 'PUT',
    data
  })
}

export const reloadConfig = () => {
  return request({
    url: '/config/reload',
    method: 'POST'
  })
}

export const getConfigSchema = () => {
  return request({
    url: '/config/schema',
    method: 'GET'
  })
}

export const validateConfig = (data: any) => {
  return request({
    url: '/config/validate',
    method: 'POST',
    data
  })
}

export const exportConfig = () => {
  return request({
    url: '/config/export',
    method: 'GET',
    responseType: 'blob'
  })
}

export const getConfigEnvVars = () => {
  return request({
    url: '/config/env-vars',
    method: 'GET'
  })
}
