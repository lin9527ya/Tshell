import request from '@/utils/request'

// 任务中心（后端 pkg/api/tasks.go 契约）
// 状态机：dispatched → completed | failed | cancelled | timeout
export type TaskStatus = 'dispatched' | 'completed' | 'failed' | 'cancelled' | 'timeout'
export type TaskKind = 'console' | 'batch' | 'exec' | 'download'

export interface TaskRow {
  id: number
  batch_id: string
  uid: string
  operator: string
  kind: TaskKind | string
  command: string
  status: TaskStatus | string
  result: string
  error: string
  created_at: string
  finished_at: string
}

export interface TaskListParams {
  page?: number
  size?: number
  uid?: string
  status?: string
  kind?: string
}

// 终态判定：只有 dispatched 还在进行中
export const isTerminal = (status: string): boolean => status !== 'dispatched'

export default {
  list(params: TaskListParams) {
    return request({ url: '/tasks', method: 'GET', params })
  },
  get(id: number | string) {
    return request({ url: `/tasks/${id}`, method: 'GET' })
  },
  cancel(id: number | string) {
    return request({ url: `/tasks/${id}/cancel`, method: 'POST' })
  }
}
