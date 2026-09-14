/**
 * 通用格式化工具函数
 */

/**
 * 格式化用户名显示，支持完整的域用户名格式
 * @param username 用户名
 * @param computer 计算机名（可选）
 * @returns 格式化后的用户名字符串
 */
export const formatUsername = (username: string, computer?: string): string => {
  if (!username) return '-'
  
  // 检查是否已经是 计算机名\用户名 的格式
  if (username.includes('\\')) {
    return username // 已经是完整格式，直接返回
  }
  
  // 检查是否包含@符号（邮箱格式）
  if (username.includes('@')) {
    return username // 邮箱格式直接返回
  }
  
  // 检查 computer 和 username 是否相同，如果相同则只显示一个
  if (computer && computer === username) {
    return username // 相同时只显示用户名
  }
  
  // 组合成 计算机名\用户名 的格式
  const computerName = computer || 'Unknown'
  return `${computerName}\\${username}`
}

/**
 * 格式化客户端状态显示
 * @param online 是否在线
 * @returns 状态文本和样式类
 */
export const formatClientStatus = (online: boolean | string) => {
  const isOnline = online === true || online === '1'
  return {
    text: isOnline ? '在线' : '离线',
    class: isOnline ? 'status-online' : 'status-offline',
    type: isOnline ? 'success' : 'danger'
  }
}

/**
 * 格式化时间显示
 * @param dateString 时间字符串
 * @returns 格式化后的时间字符串
 */
export const formatDateTime = (dateString: string): string => {
  if (!dateString || dateString === '-') return 'N/A'
  
  const date = new Date(dateString)
  if (isNaN(date.getTime())) {
    return 'Invalid Date'
  }
  
  return date.toLocaleString('zh-CN', {
    year: 'numeric', 
    month: '2-digit', 
    day: '2-digit',
    hour: '2-digit', 
    minute: '2-digit', 
    second: '2-digit'
  })
}

/**
 * 格式化文件大小显示
 * @param bytes 字节数
 * @returns 格式化后的文件大小字符串
 */
export const formatFileSize = (bytes: number): string => {
  if (bytes === 0) return '0 B'
  
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

/**
 * 格式化UID显示（缩短显示）
 * @param uid 完整UID
 * @param length 显示长度，默认8位
 * @returns 缩短后的UID字符串
 */
export const formatUid = (uid: string, length: number = 8): string => {
  if (!uid) return '-'
  if (uid.length <= length) return uid
  return uid.substring(0, length) + '...'
}