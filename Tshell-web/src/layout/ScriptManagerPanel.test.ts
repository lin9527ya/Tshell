import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ElementPlus from 'element-plus'

// CNA 执行面五个接口此前全是零调用（只有脚本文件管理那一半接了线）
const listScripts = vi.fn()
const listFiles = vi.fn()
const listMenus = vi.fn()
const execAlias = vi.fn()
const evalScript = vi.fn()
const testScript = vi.fn()
const execMenuItem = vi.fn()
const getClients = vi.fn()

vi.mock('@/api/cna', () => ({
  default: {
    listScripts: (...a: unknown[]) => listScripts(...a),
    listFiles: (...a: unknown[]) => listFiles(...a),
    listMenus: (...a: unknown[]) => listMenus(...a),
    execAlias: (...a: unknown[]) => execAlias(...a),
    evalScript: (...a: unknown[]) => evalScript(...a),
    testScript: (...a: unknown[]) => testScript(...a),
    execMenuItem: (...a: unknown[]) => execMenuItem(...a),
    loadScript: vi.fn(),
    unloadScript: vi.fn(),
    reloadScript: vi.fn(),
    uploadScript: vi.fn(),
  },
}))
vi.mock('@/api/clients', () => ({
  default: { get_clients_list: (...a: unknown[]) => getClients(...a) },
}))

import ScriptManagerPanel from './ScriptManagerPanel.vue'

const mountPanel = () => mount(ScriptManagerPanel, { global: { plugins: [ElementPlus] } })

async function typeAndRun(wrapper: ReturnType<typeof mountPanel>, text: string) {
  const input = wrapper.find('.sm-input')
  await input.setValue(text)
  await input.trigger('keyup.enter')
  await flushPromises()
}

describe('ScriptManagerPanel 的 CNA 执行面', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    listScripts.mockReset().mockResolvedValue({ data: { status: 200, data: [] } })
    listFiles.mockReset().mockResolvedValue({ data: { status: 200, data: [] } })
    listMenus.mockReset().mockResolvedValue({
      data: {
        status: 200,
        data: [{ script: 'demo.cna', kind: 'beacon', items: [{ label: 'Run fscan' }, { label: 'Sub', sub_items: [{ label: 'Inner' }] }] }],
      },
    })
    getClients.mockReset().mockResolvedValue({
      data: { data: { list: [{ Uid: 'uid-1', Computer: 'WIN01', ExternalIP: '10.0.0.5', Online: '1' }] } },
    })
    execAlias.mockReset().mockResolvedValue({ data: { status: 200, hit: false } })
    evalScript.mockReset().mockResolvedValue({ data: { status: 200, output: ['eval-output'] } })
    testScript.mockReset().mockResolvedValue({ data: { status: 200, ok: true, message: '语法检查通过' } })
    execMenuItem.mockReset().mockResolvedValue({ data: { status: 200, hit: true, output: ['menu-output'] } })
  })

  it('挂载时加载脚本、菜单与在线客户端', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    expect(listScripts).toHaveBeenCalled()
    expect(listMenus).toHaveBeenCalled()
    expect(getClients).toHaveBeenCalled()
    expect(wrapper.text()).toContain('Run fscan')
    // el-select 的选项在下拉里（不展开不渲染文本），断言归一化后的状态
    const vm = wrapper.vm as unknown as { clients: { uid: string; label: string }[] }
    expect(vm.clients).toEqual([{ uid: 'uid-1', label: 'WIN01 (10.0.0.5)' }])
  })

  it('输入先按 alias 拦截，命中时不再 eval', async () => {
    execAlias.mockResolvedValue({ data: { status: 200, hit: true, output: ['alias-output'] } })
    const wrapper = mountPanel()
    await flushPromises()
    await typeAndRun(wrapper, 'myalias arg1')

    expect(execAlias).toHaveBeenCalledWith({ uid: '', name: 'myalias', args: ['arg1'] })
    expect(evalScript).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('alias-output')
  })

  it('alias 未命中时回退到 eval', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await typeAndRun(wrapper, 'println("hi")')

    expect(execAlias).toHaveBeenCalled()
    expect(evalScript).toHaveBeenCalledWith('println("hi")', '')
    expect(wrapper.text()).toContain('eval-output')
  })

  it('未选会话时菜单项不执行，选了之后带路径调用', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    await wrapper.find('.sm-menu-item').trigger('click')
    await flushPromises()
    expect(execMenuItem).not.toHaveBeenCalled()

    // 选一个会话再点
    const vm = wrapper.vm as unknown as { execUid: string }
    vm.execUid = 'uid-1'
    await flushPromises()
    await wrapper.find('.sm-menu-item').trigger('click')
    await flushPromises()
    expect(execMenuItem).toHaveBeenCalledWith({ uid: 'uid-1', kind: 'beacon', path: ['Run fscan'] })
    expect(wrapper.text()).toContain('menu-output')
  })

  it('语法检查调用 /cna/test 并把结果写进控制台', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.find('.sm-input').setValue('sub f(){}')
    const btn = wrapper.findAll('button').find((b) => b.text().includes('语法检查'))
    expect(btn).toBeTruthy()
    await btn!.trigger('click')
    await flushPromises()
    expect(testScript).toHaveBeenCalledWith('sub f(){}')
    expect(wrapper.text()).toContain('语法检查通过')
  })
})
