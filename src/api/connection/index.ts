import request from '@/utils/request'

export default {
    getPoolStats() {
        return request({ url: '/connection/pool/stats', method: 'GET' })
    },
    getWorkerStats() {
        return request({ url: '/connection/worker/stats', method: 'GET' })
    },
    getConnectionDetails(params: { protocol?: string; page?: number; page_size?: number }) {
        return request({ url: '/connection/details', method: 'GET', params })
    },
    closeConnection(id: string) {
        return request({ url: `/connection/${encodeURIComponent(id)}`, method: 'DELETE' })
    },
    updatePoolConfig(data: {
        max_connections?: number
        max_idle_time?: string
        health_check_period?: string
        conn_timeout?: string
        read_timeout?: string
        write_timeout?: string
    }) {
        return request({ url: '/connection/pool/config', method: 'PUT', data })
    },
    resizeWorkerPool(workerCount: number) {
        return request({ url: '/connection/worker/resize', method: 'PUT', data: { worker_count: workerCount } })
    },
    getConnectionsByProtocol(protocol: string) {
        return request({ url: `/connection/protocol/${encodeURIComponent(protocol)}`, method: 'GET' })
    },
    performHealthCheck() {
        return request({ url: '/connection/health-check', method: 'POST' })
    },
    getConnectionByUID(uid: string) {
        return request({ url: `/connection/uid/${encodeURIComponent(uid)}`, method: 'GET' })
    }
}
