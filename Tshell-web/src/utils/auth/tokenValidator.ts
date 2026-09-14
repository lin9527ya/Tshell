/**
 * JWT Token 验证工具
 * 用于路由守卫中验证token有效性，避免在控制台显示不必要的401错误
 */

import { useUserStore } from '@/stores/user'

interface TokenValidationResult {
  valid: boolean
  needsLogin: boolean
  error?: string
}

/**
 * 验证JWT Token是否有效
 * @param token JWT Token
 * @returns 验证结果
 */
export async function validateToken(token: string): Promise<TokenValidationResult> {
  try {
    // 使用轻量校验端点 /api/auth/check（所有登录用户可访问，无敏感配置）。
    // OPSEC: token 为空时凭 HttpOnly Cookie 认证（页面刷新后登录态保持）。
    const headers: Record<string, string> = { 'Content-Type': 'application/json' }
    if (token) {
      headers['Authorization2'] = 'Bearer ' + token
    }
    const response = await fetch('/api/auth/check', {
      method: 'GET',
      headers,
      credentials: 'include'
    })

    if (response.ok) {
      return { valid: true, needsLogin: false }
    } else if (response.status === 401) {
      return { valid: false, needsLogin: true, error: 'Token已过期或无效' }
    } else {
      return { valid: false, needsLogin: false, error: `服务器错误: ${response.status}` }
    }
  } catch (error) {
    console.error('Token验证网络错误:', error)
    return { valid: false, needsLogin: false, error: '网络错误' }
  }
}

/**
 * 静默验证Token（不产生副作用）
 * 适用于路由守卫等场景
 */
export async function silentValidateToken(token: string): Promise<boolean> {
  const result = await validateToken(token)
  return result.valid
}

/**
 * 带缓存的Token验证
 */
interface TokenCache {
  token: string
  timestamp: number
  valid: boolean
}

let tokenCache: TokenCache | null = null
const CACHE_DURATION = 60 * 1000 // 1分钟缓存

export async function cachedValidateToken(token: string): Promise<boolean> {
  const r = await cachedValidateTokenDetailed(token)
  return r.valid
}

// F9: 区分「token 无效」与「网络/服务器错误」——原网络错误也被当作无效导致
// 服务短暂抖动即全员登出。needsLogin=false 时调用方应放行并由后续请求 401 兜底。
export async function cachedValidateTokenDetailed(token: string): Promise<{ valid: boolean; needsLogin: boolean }> {
  const now = Date.now()
  
  // 检查缓存
  if (tokenCache && 
      tokenCache.token === token && 
      (now - tokenCache.timestamp) < CACHE_DURATION) {
    console.log('使用缓存的Token验证结果:', tokenCache.valid)
    return { valid: tokenCache.valid, needsLogin: !tokenCache.valid }
  }
  
  // 执行验证
  const result = await validateToken(token)
  
  // 更新缓存
  tokenCache = { token, timestamp: now, valid: result.valid }
  
  console.log('Token验证结果:', result.valid ? '有效' : '无效', result.error || '')
  return { valid: result.valid, needsLogin: result.needsLogin }
}

/**
 * 清除Token验证缓存
 */
export function clearTokenCache(): void {
  tokenCache = null
}