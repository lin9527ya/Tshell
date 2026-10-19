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

// 右键菜单里的「打开会话」用 router 跳转；测试不挂真路由
const routerPush = vi.fn()
vi.mock('vue-router', () => ({
  useRouter: () => ({ push: routerPush }),
}))

import PivotGraphView from './PivotGraphView.vue'

const SAMPLE = {
  nodes: [
    { id: 'server', type: 'server', label: 'Tshell Server' },
    { id: 'listener_tcp', type: 'listener', label: 'tcp', sub_label: '0.0.0.0:8089' },
    { id: 'client_a', type: 'client', label: 'WEB-01', online: '1', meta: { os: 'windows' } },
    { id: 'client_b', type: 'client', label: 'WEB-02', online: '2', meta: { os: 'linux' } },
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
    routerPush.mockReset()
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

// ---------------------------------------------------------------------------
// 交互：右键菜单 / 聚焦 / 边标签 / 力导向
// ---------------------------------------------------------------------------

describe('PivotGraphView 交互', () => {
  beforeEach(() => {
    getGraphData.mockReset()
    routerPush.mockReset()
    localStorage.clear()
  })

  it('右键节点弹出菜单，点"打开会话"跳到该客户端', async () => {
    // 菜单里的「打开会话」需要 meta.uid
    const data = {
      ...SAMPLE,
      nodes: SAMPLE.nodes.map((n) =>
        n.id === 'client_a' ? { ...n, meta: { uid: 'uid-a' } } : n,
      ),
    }
    getGraphData.mockResolvedValue({ data: { data } })
    const w = mountView()
    await flushPromises()

    const clientA = w.findAll('.pg-node').find((n) => n.text().includes('WEB-01'))!
    await clientA.trigger('contextmenu')
    expect(w.find('.pg-menu').exists()).toBe(true)

    const item = w.findAll('.pg-menu-item').find((b) => b.text().includes('打开会话'))!
    await item.trigger('click')
    expect(routerPush).toHaveBeenCalledWith('/client/uid-a/console')
    expect(w.find('.pg-menu').exists()).toBe(false)
  })

  it('非客户端/非管理页节点不出现跳转项', async () => {
    getGraphData.mockResolvedValue({ data: { data: SAMPLE } })
    const w = mountView()
    await flushPromises()

    const target = w.findAll('.pg-node').find((n) => n.text().includes('10.0.0.5:445'))!
    await target.trigger('contextmenu')
    const labels = w.findAll('.pg-menu-item').map((b) => b.text())
    expect(labels).not.toContain('打开会话')
    expect(labels).not.toContain('监听管理')
    expect(labels).toContain('仅看关联')
    expect(labels).toContain('复制名称')
    // target 节点 label 本身就是地址、没有 sub_label：不出现"复制地址"这种重复项
    expect(labels).not.toContain('复制地址')

    // 有 sub_label 的监听器节点才给"复制地址"
    const listener = w.findAll('.pg-node').find((n) => n.text().includes('0.0.0.0:8089'))!
    await listener.trigger('contextmenu')
    expect(w.findAll('.pg-menu-item').map((b) => b.text())).toContain('复制地址')
  })

  it('"仅看关联"只留该节点与直接邻居', async () => {
    getGraphData.mockResolvedValue({ data: { data: SAMPLE } })
    const w = mountView()
    await flushPromises()
    expect(w.findAll('.pg-node')).toHaveLength(6)

    const clientA = w.findAll('.pg-node').find((n) => n.text().includes('WEB-01'))!
    await clientA.trigger('contextmenu')
    await w.findAll('.pg-menu-item').find((b) => b.text().includes('仅看关联'))!.trigger('click')
    await flushPromises()

    // client_a 的邻居：listener_tcp + tunnel_1
    expect(w.findAll('.pg-node')).toHaveLength(3)
    expect(w.text()).toContain('WEB-01')
    expect(w.text()).toContain('0.0.0.0:8089')
    // 工具条出现"聚焦"胶囊，点击退出
    const chip = w.findAll('.pg-viewbtn').find((b) => b.text().includes('聚焦'))!
    expect(chip.exists()).toBe(true)
    await chip.trigger('click')
    await flushPromises()
    expect(w.findAll('.pg-node')).toHaveLength(6)
  })

  it('后端给了边 label 才画标签', async () => {
    const withLabel = {
      ...SAMPLE,
      edges: SAMPLE.edges.map((e) =>
        e.id === 'e2' ? { ...e, label: 'tcp 回连' } : e,
      ),
    }
    getGraphData.mockResolvedValue({ data: { data: withLabel } })
    const w = mountView()
    await flushPromises()
    const labels = w.findAll('.pg-edge-label')
    expect(labels).toHaveLength(1)
    expect(labels[0].text()).toBe('tcp 回连')
  })

  it('"力导向"按斥力/弹簧重排节点坐标', async () => {
    getGraphData.mockResolvedValue({ data: { data: SAMPLE } })
    const w = mountView()
    await flushPromises()

    const before = w.findAll('.pg-node').map((n) => n.attributes('transform'))
    await w.findAll('.pg-viewbtn').find((b) => b.text().includes('力导向'))!.trigger('click')
    await flushPromises()
    const after = w.findAll('.pg-node').map((n) => n.attributes('transform'))

    expect(after).not.toEqual(before)
    // 重排后仍是 6 个节点、5 条边（没丢数据）
    expect(w.findAll('.pg-node')).toHaveLength(6)
    expect(w.findAll('.pg-edge')).toHaveLength(5)
    // 位置写进 localStorage，刷新后不跳
    expect(localStorage.getItem('tshell-graph-positions')).toBeTruthy()
  })

  it('"分层"清掉力导向/拖拽写入的位置', async () => {
    getGraphData.mockResolvedValue({ data: { data: SAMPLE } })
    const w = mountView()
    await flushPromises()

    await w.findAll('.pg-viewbtn').find((b) => b.text().includes('力导向'))!.trigger('click')
    await flushPromises()
    expect(localStorage.getItem('tshell-graph-positions')).not.toBe('{}')

    await w.findAll('.pg-viewbtn').find((b) => b.text().includes('分层'))!.trigger('click')
    await flushPromises()
    expect(localStorage.getItem('tshell-graph-positions')).toBe('{}')
  })
})

// ---------------------------------------------------------------------------
// 视觉区分：系统图标 / 离线置灰 / 离线虚线 / 导出
// ---------------------------------------------------------------------------

describe('PivotGraphView 辨识度与导出', () => {
  beforeEach(() => {
    getGraphData.mockReset()
    localStorage.clear()
  })

  it('客户端节点画系统图标（Windows / Linux 各自一份 path）', async () => {
    getGraphData.mockResolvedValue({ data: { data: SAMPLE } })
    const w = mountView()
    await flushPromises()

    // 只有客户端画图标：server/listener/tunnel/target 不画
    const icons = w.findAll('.pg-node-os')
    expect(icons).toHaveLength(2)

    const win = w.findAll('.pg-node').find((n) => n.text().includes('WEB-01'))!
    const lin = w.findAll('.pg-node').find((n) => n.text().includes('WEB-02'))!
    const winD = win.find('.pg-node-os').attributes('d')
    const linD = lin.find('.pg-node-os').attributes('d')
    expect(winD).not.toBe(linD)
    expect(winD).toContain('M-5.5 -5.5') // Windows 四方块
    expect(linD!.length).toBeGreaterThan(winD!.length) // Tux 形状更复杂
  })

  it('离线主机置灰：灰虚线空心圈 + 灰色标签', async () => {
    getGraphData.mockResolvedValue({ data: { data: SAMPLE } })
    const w = mountView()
    await flushPromises()

    const offline = w.findAll('.pg-node-offline')
    expect(offline).toHaveLength(1)
    const circle = offline[0].findAll('circle')[1] // [0] 是悬停光晕
    expect(circle.attributes('stroke')).toBe('#9ca3af')
    expect(circle.attributes('fill')).toBe('transparent')
    expect(circle.attributes('stroke-dasharray')).toBe('3 2')

    // 在线客户端仍是类型色实心
    const online = w.findAll('.pg-node').find((n) => n.text().includes('WEB-01'))!
    expect(online.findAll('circle')[1].attributes('fill')).toBe('#3b82f6')
  })

  it('连到离线主机的边画虚线，在线的不画', async () => {
    getGraphData.mockResolvedValue({ data: { data: SAMPLE } })
    const w = mountView()
    await flushPromises()

    // SAMPLE 里只有 e3（client_b→listener_tcp）连到离线主机
    const dashed = w.findAll('.pg-edge-offline')
    expect(dashed).toHaveLength(1)
    expect(dashed[0].attributes('id')).toBeUndefined() // line 上没有 id 属性，用 class 断言数量即可
    expect(w.findAll('.pg-edge')).toHaveLength(5)
  })

  it('缩得太小时边标签只是隐藏（class），DOM 里仍在 —— 导出才不会漏标签', async () => {
    const withLabel = {
      ...SAMPLE,
      edges: SAMPLE.edges.map((e) => (e.id === 'e2' ? { ...e, label: 'tcp 回连' } : e)),
    }
    getGraphData.mockResolvedValue({ data: { data: withLabel } })
    const w = mountView()
    await flushPromises()
    expect(w.find('.pg-edge-label').classes()).not.toContain('is-hidden')

    const minus = w.findAll('.pg-viewbtn').find((b) => b.text().trim() === '−')!
    for (let i = 0; i < 6; i++) await minus.trigger('click')

    const label = w.find('.pg-edge-label')
    expect(label.exists()).toBe(true)
    expect(label.classes()).toContain('is-hidden')
  })

  it('导出菜单给出 PNG / SVG，选 SVG 会下载一份自包含文件', async () => {
    getGraphData.mockResolvedValue({ data: { data: SAMPLE } })
    const w = mountView()
    await flushPromises()

    const created: Blob[] = []
    ;(URL as unknown as { createObjectURL: unknown }).createObjectURL = (b: Blob) => {
      created.push(b)
      return 'blob:fake'
    }
    ;(URL as unknown as { revokeObjectURL: unknown }).revokeObjectURL = () => {}
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})

    await w.findAll('.pg-viewbtn').find((b) => b.text().includes('导出'))!.trigger('click')
    const items = w.findAll('.pg-menu-item').map((b) => b.text())
    expect(items.join('|')).toContain('PNG')
    expect(items.join('|')).toContain('SVG')

    await w.findAll('.pg-menu-item').find((b) => b.text().includes('SVG'))!.trigger('click')
    await flushPromises()

    expect(created).toHaveLength(1)
    expect(created[0].type).toContain('image/svg+xml')
    // jsdom 的 Blob 没有 .text()，用 FileReader 读
    const text = await new Promise<string>((resolve, reject) => {
      const fr = new FileReader()
      fr.onload = () => resolve(String(fr.result))
      fr.onerror = () => reject(fr.error)
      fr.readAsText(created[0])
    })
    expect(text).toContain('<svg')
    expect(text).toContain('class="pg-content"')
    expect(click).toHaveBeenCalled()
    click.mockRestore()
  })
})
