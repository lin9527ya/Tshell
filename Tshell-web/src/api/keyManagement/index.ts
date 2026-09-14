import request from '@/utils/request'

export default {
    getListenerKeyConfig(params: { type?: string; address?: string }) {
        return request({ url: '/key-management/listener-config', method: 'GET', params })
    },
    getAllListenerKeyConfigs() {
        return request({ url: '/key-management/all-configs', method: 'GET' })
    },
    updateListenerKeyConfig(data: { listen_address: string; vkey?: string; encryption_salt?: string }) {
        return request({ url: '/key-management/update', method: 'POST', data })
    },
    resetListenerKeyConfig(address: string) {
        return request({ url: `/key-management/reset/${encodeURIComponent(address)}`, method: 'POST' })
    },

    // ===== 会话密钥状态 / 轮换 / 健康检查 / 统计 / 轮换配置（backend key_management.go） =====
    getAllKeyStatus() {
        return request({ url: '/key-management/key-status', method: 'GET' })
    },
    getKeyStatus(clientId: string) {
        return request({ url: `/key-management/key-status/${encodeURIComponent(clientId)}`, method: 'GET' })
    },
    forceKeyRotation(data: { client_id: string; force?: boolean }) {
        return request({ url: '/key-management/rotate', method: 'POST', data })
    },
    sendKeyHealthCheck(clientId: string) {
        return request({ url: `/key-management/health-check/${encodeURIComponent(clientId)}`, method: 'POST' })
    },
    getKeyMetrics() {
        return request({ url: '/key-management/metrics', method: 'GET' })
    },
    updateRotationConfig(data: { overlap_duration_hours: number; rotation_enabled: boolean }) {
        return request({ url: '/key-management/rotation-config', method: 'PUT', data })
    }
}
