import request from '@/utils/request'

export default {
    // 安全统计
    getSecurityStats() {
        return request({ url: '/security/stats', method: 'GET' })
    },
    // IP 白名单
    getWhitelist() {
        return request({ url: '/security/whitelist', method: 'GET' })
    },
    addWhitelist(data: { ip: string; network?: string }) {
        return request({ url: '/security/whitelist/add', method: 'POST', data })
    },
    removeWhitelist(data: { ip: string }) {
        return request({ url: '/security/whitelist/remove', method: 'DELETE', data })
    },
    // 封禁 IP
    getBlockedIPs() {
        return request({ url: '/security/blocked-ips', method: 'GET' })
    },
    unblockIP(ip: string) {
        return request({ url: `/security/unblock/${encodeURIComponent(ip)}`, method: 'POST' })
    },
    // 限流配置
    updateRateLimit(data: { login_rate_limit?: number; api_rate_limit?: number; block_duration?: number }) {
        return request({ url: '/security/rate-limit', method: 'PUT', data })
    },
    // 实时统计
    getRealtimeStats() {
        return request({ url: '/security/realtime-stats', method: 'GET' })
    },
    // 隐藏访问
    getHiddenAccessStatus() {
        return request({ url: '/security/hidden-access/status', method: 'GET' })
    },
    updateHiddenAccessConfig(data: {
        enabled?: boolean
        access_paths?: string[]
        param_names?: string[]
        secret_key?: string
        session_ttl?: number
    }) {
        return request({ url: '/security/hidden-access/config', method: 'PUT', data })
    },
    regenerateHiddenAccessKey() {
        return request({ url: '/security/hidden-access/regenerate-key', method: 'POST' })
    },
    clearHiddenAccessSessions() {
        return request({ url: '/security/hidden-access/clear-sessions', method: 'POST' })
    }
}
