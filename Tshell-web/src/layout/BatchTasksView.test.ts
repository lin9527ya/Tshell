import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ElementPlus from 'element-plus'

// 任务 API 打桩（响应形状与后端 pkg/api/tasks.go 契约一致）
const list = vi.fn()
const cancel = vi.fn()

vi.mock('@/api/tasks', () => ({
  default: {
    list: (...a: unknown[]) => list(...a),
    get: vi.fn(),
    cancel: (...a: unknown[]) => cancel(...a),
  },
  isTerminal: (s: string) => s !== 'dispatched',
}))

vi.mock('element-plus', async () => {
  const actual = await vi.importActual<any>('element-plus')
  return { ...actual, ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn() } }
})

import BatchTasksView from './BatchTasksView.vue'

const task = (over: Record<string, unknown> = {}) => ({
  id: 1,
  batch_id: 'b-1',
  uid: 'uid-abcdef1234',
  operator: 'alice',
  kind: 'batch',
  command: 'whoami',
  status: 'dispatched',
  result: '',
  error: '',
  created_at: '2026-10-18T10:00:00Z',
  finished_at: '0001-01-01T00:00:00Z',
  ...over,
})

const mountView = () => mount(BatchTasksView, { global: { plugins: [ElementPlus] } })

describe('BatchTasksView（任务中心）', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    list.mockReset().mockResolvedValue({ data: { data: { total: 0, page: 1, size: 50, list: [] } } })
    cancel.mockReset().mockResolvedValue({ data: { status: 200 } })
  })

  it('展示真实任务字段：状态 / 类型 / 操作员 / 目标会话', async () => {
    list.mockResolvedValue({
      data: {
        data: {
          total: 2,
          page: 1,
          size: 50,
          list: [
            task({ id: 7, status: 'completed', kind: 'exec', operator: 'bob', command: 'execute-assembly a.exe', result: 'ok' }),
            task({ id: 8, status: 'dispatched', kind: 'download', command: 'download /tmp/a.bin' }),
          ],
        },
      },
    })
    const wrapper = mountView()
    await flushPromises()

    const text = wrapper.text()
    expect(text).toContain('7')
    expect(text).toContain('已完成')
    expect(text).toContain('进行中')
    expect(text).toContain('bob')
    expect(text).toContain('uid-abcd') // 会话短 UID
    expect(text).toContain('下载')      // kind=download 中文标签
    // 进行中计数
    expect(wrapper.text()).toContain('1 进行中')
  })

  it('只有进行中的任务可取消，取消后调用接口并刷新', async () => {
    list.mockResolvedValue({
      data: { data: { total: 2, page: 1, size: 50, list: [task({ id: 1 }), task({ id: 2, status: 'completed' })] } },
    })
    const wrapper = mountView()
    await flushPromises()

    const buttons = wrapper.findAll('button.bt-cancel-btn')
    expect(buttons).toHaveLength(1) // 已完成的没有取消按钮

    const confirmSpy = vi.spyOn(
      (await import('element-plus')).ElMessageBox as any,
      'confirm',
    ).mockResolvedValue('confirm')
    await buttons[0].trigger('click')
    await flushPromises()

    expect(confirmSpy).toHaveBeenCalled()
    expect(cancel).toHaveBeenCalledWith(1)
  })

  it('过滤条件变化时带上 status/kind/uid 重新查询', async () => {
    const wrapper = mountView()
    await flushPromises()
    expect(list).toHaveBeenCalledWith({ page: 1, size: 50, status: undefined, kind: undefined, uid: undefined })

    const vm = wrapper.vm as any
    vm.filters.status = 'failed'
    vm.filters.kind = 'exec'
    vm.filters.uid = 'uid-1'
    vm.reload()
    await flushPromises()

    expect(list).toHaveBeenLastCalledWith({ page: 1, size: 50, status: 'failed', kind: 'exec', uid: 'uid-1' })
  })

  it('请求失败不清空已有列表（避免轮询抖动把界面刷空）', async () => {
    list.mockResolvedValue({ data: { data: { total: 1, page: 1, size: 50, list: [task({ id: 3 })] } } })
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('3')

    list.mockRejectedValue(new Error('boom'))
    ;(wrapper.vm as any).fetchTasks()
    await flushPromises()
    expect(wrapper.text()).toContain('3')
  })
})
