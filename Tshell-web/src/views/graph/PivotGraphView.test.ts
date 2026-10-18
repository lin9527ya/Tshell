import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'

// 组件直接依赖 API 与 i18n store，测试里把 API 打桩：
// 这样能覆盖"加载失败 / 403 / 空数据"这几条真正会出问题的分支，
// 而不用起后端。
const getGraphData = vi.fn()
vi.mock('@/api/clients', () => ({
  default: {
    get_graph_data: (...args: unknown[]) => getGraphData(...args),
  },
}))

import PivotGraphView from './PivotGraphView.vue'

const SAMPLE = {
  nodes: [
    { id: 'server', type: 'server', label: 'Tshell Server' },
    { id: 'listener_tcp', type: 'listener', label: 'tcp', sub_label: '0.0.0.0:8089' },
    { id: 'client_a', type: 'client', label: 'WEB-01', online: '1' },
    { id: 'client_b', type: 'client', label: 'WEB-02', online: '2' },
    { id: 'tunnel_1', type: 'tunnel', label: '1080', sub_label: 'socks5' },
    { id: 'target_1', type: 'target', label: '10.0.0.5:445', meta: { note: 'DC' } },
  ],
  edges: [
    { id: 'e1', source: 'server', target: 'listener_tcp' },
    { id: 'e2', source: 'client_a', target: 'listener_tcp' },
    { id: 'e3', source: 'client_b', target: 'listener_tcp' },
    { id: 'e4', source: 'client_a', target: 'tunnel_1' },
    { id: 'e5', source: 'tunnel_1', target: 'target_1' },
  ],
}

function mountView() {
  setActivePinia(createPinia())
  return mount(PivotGraphView, { global: { stubs: { VsIcon: true } } })
}

describe('PivotGraphView', () => {
  beforeEach(() => {
    getGraphData.mockReset()
  })

  it('把后端返回的节点与边画出来', async () => {
    getGraphData.mockResolvedValue({ data: { data: SAMPLE } })
    const w = mountView()
    await flushPromises()

    // 6 个节点各一个 <g>，5 条边各一条 <line>
    expect(w.findAll('.pg-node')).toHaveLength(6)
    expect(w.findAll('.pg-edge')).toHaveLength(5)
    expect(w.text()).toContain('WEB-01')
    expect(w.text()).toContain('0.0.0.0:8089')
  })

  it('离线客户端标灰（这是图上最容易看错的一处）', async () => {
    getGraphData.mockResolvedValue({ data: { data: SAMPLE } })
    const w = mountView()
    await flushPromises()

    const offline = w.findAll('.pg-node-offline')
    expect(offline).toHaveLength(1)
    expect(offline[0].text()).toContain('WEB-02')
  })

  it('统计在线/客户端数量', async () => {
    getGraphData.mockResolvedValue({ data: { data: SAMPLE } })
    const w = mountView()
    await flushPromises()
    // 2 个客户端，1 个在线
    expect(w.find('.pg-stat').text()).toContain('1 / 2')
  })

  it('点击节点展开 meta 详情', async () => {
    getGraphData.mockResolvedValue({ data: { data: SAMPLE } })
    const w = mountView()
    await flushPromises()

    expect(w.find('.pg-detail').exists()).toBe(false)
    // 点 target_1（带 meta.note）
    const target = w.findAll('.pg-node').find((n) => n.text().includes('10.0.0.5:445'))!
    await target.trigger('click')

    expect(w.find('.pg-detail').exists()).toBe(true)
    expect(w.find('.pg-meta').text()).toContain('note')
    expect(w.find('.pg-meta').text()).toContain('DC')
  })

  it('选中节点后只有相关边高亮', async () => {
    getGraphData.mockResolvedValue({ data: { data: SAMPLE } })
    const w = mountView()
    await flushPromises()

    // 未选中时所有边都算"活跃"
    expect(w.findAll('.pg-edge-active')).toHaveLength(5)

    const clientA = w.findAll('.pg-node').find((n) => n.text().includes('WEB-01'))!
    await clientA.trigger('click')
    // client_a 相关边：e2（client_a→listener）、e4（client_a→tunnel）= 2 条
    expect(w.findAll('.pg-edge-active')).toHaveLength(2)
  })

  it('空数据给出可操作的提示而不是空白页', async () => {
    getGraphData.mockResolvedValue({ data: { data: { nodes: [], edges: [] } } })
    const w = mountView()
    await flushPromises()
    expect(w.find('.pg-empty').exists()).toBe(true)
    expect(w.find('.pg-empty').text()).toContain('监听器')
  })

  it('403 给出"权限不足"的页面内提示，不弹全局报错', async () => {
    getGraphData.mockRejectedValue({ response: { status: 403 } })
    const w = mountView()
    await flushPromises()
    expect(w.find('.pg-error').exists()).toBe(true)
    expect(w.find('.pg-error').text()).toContain('无权')
    expect(w.find('.pg-error').text()).toContain('operator')
  })

  it('其它错误给出通用失败提示', async () => {
    getGraphData.mockRejectedValue(new Error('boom'))
    const w = mountView()
    await flushPromises()
    expect(w.find('.pg-error').exists()).toBe(true)
  })

  it('刷新后选中节点消失时清掉详情（不留幽灵节点）', async () => {
    getGraphData.mockResolvedValue({ data: { data: SAMPLE } })
    const w = mountView()
    await flushPromises()

    const clientB = w.findAll('.pg-node').find((n) => n.text().includes('WEB-02'))!
    await clientB.trigger('click')
    expect(w.find('.pg-detail').exists()).toBe(true)

    // 第二次返回里 WEB-02 已被删除
    const shrunk = { ...SAMPLE, nodes: SAMPLE.nodes.filter((n) => n.id !== 'client_b') }
    getGraphData.mockResolvedValue({ data: { data: shrunk } })
    await w.find('.pg-actions button').trigger('click')
    await flushPromises()

    expect(w.find('.pg-detail').exists()).toBe(false)
  })

  it('节点缺失的边不画悬空线', async () => {
    getGraphData.mockResolvedValue({
      data: {
        data: {
          nodes: [{ id: 'server', type: 'server', label: 's' }],
          edges: [{ id: 'e1', source: 'server', target: 'ghost' }],
        },
      },
    })
    const w = mountView()
    await flushPromises()
    expect(w.findAll('.pg-edge')).toHaveLength(0)
  })
})
