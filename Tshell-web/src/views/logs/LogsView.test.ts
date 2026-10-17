import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ElementPlus from 'element-plus'

// 审计 API 打桩（响应形状与后端 pkg/audit/handlers.go 的契约一致）
const getAuditLogs = vi.fn()
const getAuditStatistics = vi.fn()

vi.mock('@/api/audit', () => ({
  default: {
    getAuditLogs: (...args: unknown[]) => getAuditLogs(...args),
    getAuditStatistics: (...args: unknown[]) => getAuditStatistics(...args),
    exportAuditLogs: vi.fn(),
    cleanupOldLogs: vi.fn(),
  },
}))

vi.mock('@/layout/Dialogs/Payloads/payloadUtil', () => ({ downloadBlob: vi.fn() }))

import LogsView from './LogsView.vue'

const mountView = () => mount(LogsView, { global: { plugins: [ElementPlus] } })

describe('LogsView（设置页「日志」页签）', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    getAuditLogs.mockReset().mockResolvedValue({
      data: {
        status: 200,
        data: {
          total: 2,
          page: 1,
          page_size: 50,
          logs: [
            {
              timestamp: '2026-10-18T00:00:00Z',
              level: 'INFO',
              category: 'FILE',
              username: 'alice',
              client_ip: '10.0.0.1',
              action: 'UPLOAD',
              result: 'SUCCESS',
              message: '插件上传成功',
            },
            {
              timestamp: '2026-10-18T00:01:00Z',
              level: 'ERROR',
              category: 'AUTH',
              username: 'bob',
              client_ip: '10.0.0.2',
              action: 'LOGIN',
              result: 'FAILURE',
              message: '登录失败',
            },
          ],
        },
      },
    })
    getAuditStatistics.mockReset().mockResolvedValue({
      data: {
        status: 200,
        data: {
          total_logs: 42,
          today_logs: 7,
          recent_failures: 3,
          logs_by_level: { INFO: 40, ERROR: 2 },
          logs_by_category: { FILE: 20 },
          logs_by_result: { SUCCESS: 39, FAILURE: 3 },
          top_users: [{ username: 'alice', count: 5 }],
          top_ips: [{ ip: '10.0.0.1', count: 4 }],
        },
      },
    })
  })

  it('挂载时按后端契约加载日志与统计', async () => {
    mountView()
    await flushPromises()

    expect(getAuditLogs).toHaveBeenCalledTimes(1)
    expect(getAuditStatistics).toHaveBeenCalledTimes(1)
    // 默认查询参数
    expect(getAuditLogs.mock.calls[0][0]).toMatchObject({ page: 1, page_size: 50 })
    expect(getAuditStatistics.mock.calls[0][0]).toBe('24h')
  })

  it('渲染标题、统计数字与日志行', async () => {
    const wrapper = mountView()
    await flushPromises()

    const text = wrapper.text()
    expect(text).toContain('审计日志')
    // 统计条（纯 div，jsdom 下必定渲染）
    expect(text).toContain('42')
    expect(text).toContain('7')
    expect(text).toContain('3')
    // 表格行
    expect(text).toContain('alice')
    expect(text).toContain('UPLOAD')
    expect(text).toContain('bob')
    expect(text).toContain('FAILURE')
  })

  it('统计接口异常时清空统计且不抛错', async () => {
    getAuditStatistics.mockResolvedValue({ data: { status: 500 } })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('审计日志')
    // 四个统计格子都应回落到 0（stats 被清空）
    const nums = wrapper.findAll('.al-stat-num').map((n) => n.text())
    expect(nums).toEqual(['0', '0', '0', '0'])
  })

  it('日志接口抛错时页面仍可渲染', async () => {
    getAuditLogs.mockRejectedValue(new Error('boom'))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('审计日志')
  })
})
