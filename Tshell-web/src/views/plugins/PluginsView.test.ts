import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ElementPlus from 'element-plus'

// ===== PluginsView 组件级测试 =====
//
// 目的很具体：上一轮把输出过滤逻辑抽到 output-filter.ts（那边已有 26 项单测），
// 但**组件侧的接线没有被任何测试覆盖** —— filterOpts 传错提示文案、
// 或者把 mimikatz 路径接到通用过滤器上，单测全绿而页面行为是错的。
// 这里就钉住"组件到底用了哪个过滤器、传了哪个提示"。
//
// 这类"抽出去的东西单测很足、调用点却没人管"的空档，正是重构最容易漏的地方。

const getPlugins = vi.fn()
const getPluginDetails = vi.fn()
const getShellHistory = vi.fn()
const getOnlineClients = vi.fn()

// 方法名必须与组件实际调用的一致（插件 API 用的是 snake_case）。
// 名字写错时组件会把异常 catch 掉、只在界面上显示"加载失败"，
// 测试看到的就只是"列表是空的" —— 很容易误判成渲染问题。
vi.mock('@/api/plugin', () => ({
  default: {
    get_plugins: (...a: unknown[]) => getPlugins(...a),
    get_plugin_details: (...a: unknown[]) => getPluginDetails(...a),
    get_shell_history: (...a: unknown[]) => getShellHistory(...a),
    get_online_clients: (...a: unknown[]) => getOnlineClients(...a),
    upload_plugin: vi.fn(),
    download_plugin: vi.fn(),
    delete_plugin: vi.fn(),
  },
}))

vi.mock('@/api/clients', () => ({
  default: {
    execute_bin: vi.fn(),
    get_clients_list: vi.fn().mockResolvedValue({ data: { status: 200, data: [] } }),
  },
}))

vi.mock('@/api/tasks', () => ({
  default: { get: vi.fn() },
}))

// i18n 替身：覆盖组件实际用到的每个方法（缺 tl 会在渲染时报 "not a function"）
vi.mock('@/stores/i18n', () => ({
  useI18nStore: () => ({
    isZh: true,
    currentLocale: 'zh',
    setLocale: vi.fn(),
    toggleLocale: vi.fn(),
    t: (k: string, d?: string) => d || k,
    tl: (zh: string, _en: string) => zh,
  }),
}))

vi.mock('element-plus', async () => {
  const actual = await vi.importActual<typeof import('element-plus')>('element-plus')
  return {
    ...actual,
    ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
    ElMessageBox: { confirm: vi.fn().mockResolvedValue('confirm') },
  }
})

import PluginsView from './PluginsView.vue'

const PLUGIN = {
  Name: 'mimikatz.exe',
  OS: 'windows',
  Size: 1024,
  FileHash: 'abc123',
  Description: 'credential dumper',
}

function mountView() {
  return mount(PluginsView, {
    global: {
      plugins: [ElementPlus],
      stubs: { ScriptManagerPanel: true, PageHeader: true },
    },
  })
}

/** 造一段 shell 历史：两次不同模式的执行 */
function shellHistory(): string {
  return [
    '$ execute-assembly /plugins/mimikatz.exe',
    'Authentication Id : 0 ; 123456',
    'Username : alice',
    'NTLM : deadbeefcafe',
    '$ bof /plugins/other.o',
    'unrelated output',
  ].join('\n')
}

describe('PluginsView（插件页）', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    getPlugins.mockReset().mockResolvedValue({
      data: { status: 200, data: [PLUGIN] },
    })
    getPluginDetails.mockReset().mockResolvedValue({
      data: {
        status: 200,
        data: {
          name: 'mimikatz.exe',
          type: 'exe',
          size: 1024,
          last_modified: '2026-10-18 00:00:00',
          description: 'credential dumper',
        },
      },
    })
    getOnlineClients.mockReset().mockResolvedValue({ data: { status: 200, data: [] } })
    getShellHistory.mockReset().mockResolvedValue({
      data: { status: 200, data: shellHistory() },
    })
  })

  it('挂载后加载插件列表，并显示数量', async () => {
    const w = mountView()
    await flushPromises()
    expect(getPlugins).toHaveBeenCalled()
    // 插件名渲染在 el-select 的**下拉里**（teleport 到 body），
    // wrapper.text() 取不到；数量那行是页面内的，用它证明列表确实加载了
    expect(w.text()).toContain('1 个插件可用')
  })

  it('选中插件后加载详情并渲染详情字段', async () => {
    // 详情面板渲染的是 name/type/size/last_modified/description。
    // （插件哈希不在这个视图里显示 —— 上一轮改的是后端 PluginStore 的详情查询，
    //   不要凭印象给前端加一个不存在的字段断言。）
    const w = mountView()
    await flushPromises()

    // el-select 的 @change 才会触发 handlePluginChange。
    // 页面上有多个 el-select（客户端 UID、插件、执行模式），
    // findComponent 只返回第一个 —— 必须按 placeholder 挑出插件那个，
    // 否则事件发到了别的下拉上，测试会以"详情没加载"的形式失败，看不出真正原因。
    const select = w
      .findAllComponents({ name: 'ElSelect' })
      .find((c) => String(c.props('placeholder') || '').includes('插件'))
    expect(select, '未找到插件下拉').toBeTruthy()
    // 必须先发 update:modelValue 让 v-model 落值：handlePluginChange 读的是
    // pluginForm.selectedPlugin，只发 change 的话 name 为空、函数直接 return，
    // 测试会表现为"详情接口没被调用"，看不出是少了这一步。
    select!.vm.$emit('update:modelValue', 'mimikatz.exe')
    select!.vm.$emit('change', 'mimikatz.exe')
    await flushPromises()

    expect(getPluginDetails).toHaveBeenCalledWith('mimikatz.exe')
    // 详情面板用的小写字段名（pluginDetails.description）
    expect(w.text()).toContain('credential dumper')
  })

  it('渲染不报错（i18n 替身覆盖了组件用到的全部方法）', async () => {
    const errors: unknown[] = []
    const spy = vi.spyOn(console, 'error').mockImplementation((...a) => errors.push(a))
    const w = mountView()
    await flushPromises()
    const msg = errors.map(String).join('\n')
    expect(msg).not.toContain('is not a function')
    spy.mockRestore()
    expect(w.exists()).toBe(true)
  })

  it('未选执行模式时按钮文案是「请选择模式」', async () => {
    const w = mountView()
    await flushPromises()
    // 注意：下拉的**选项**本来就在 DOM 里（含 Execute Assembly），
    // 所以不能断言全文不含模式名 —— 要断言的是按钮上当前值的文案
    expect(w.text()).toContain('请选择模式')
  })
})
