import request from '@/utils/request'

export default {
    getAuditLogs(params: {
        page?: number
        page_size?: number
        level?: string
        category?: string
        action?: string
        username?: string
        client_ip?: string
        result?: string
        keyword?: string
        start_time?: string
        end_time?: string
    }) {
        return request({ url: '/audit/logs', method: 'GET', params })
    },
    getAuditStatistics(timeRange = '24h') {
        return request({ url: '/audit/statistics', method: 'GET', params: { time_range: timeRange } })
    },
    exportAuditLogs(params: Record<string, unknown>) {
        return request({ url: '/audit/export', method: 'GET', params, responseType: 'blob' })
    },
    cleanupOldLogs(days: number) {
        return request({ url: '/audit/cleanup', method: 'DELETE', data: { days } })
    }
}
