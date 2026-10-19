import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ElementPlus from 'element-plus'

// ===== 重定向器配置生成对话框 =====
//
// 这个对话框的价值在于"按当前画像生成，避免手写配置与画像漂移"。
// 测试因此盯三件事：请求参数是否真按表单拼、服务端返回的异常形状是否被挡住、
// 以及失败原因是否回显给运维（否则他们只会看到"没反应"）。

const getTemplate = vi.fn()

vi.mock('@/api/redirector', () => ({
  getRedirectorTemplate: (...a: unknown[]) => getTemplate(...a),
}))

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
  }
})

import RedirectorDialog from './RedirectorDialog.vue'

const OK_RESULT = {
  data: {
    status: 200,
    data: {
      type: 'nginx',
      profile: 'default',
      uris: ['/tencent/mcp/pc/pcsearch', '/tencent/sensearch/collection/item/check'],
      content: '# Tshell redirector（nginx）\nserver {\n  listen 443;\n}\n',
      lines: 4,
      full: false,
      filename: 'tshell-redirector.conf',
    },
  },
}

function mountDialog() {
  return mount(RedirectorDialog, {
    props: { visible: true },
    global: { plugins: [ElementPlus] },
  })
}

/** 找到"生成"按钮 */
function generateBtn(w: ReturnType<typeof mountDialog>) {
  return w.findAll('button').find((b) => b.text() === '生成')
}

/** 按 label 文本填 el-input */
async function fillByLabel(w: ReturnType<typeof mountDialog>, label: string, value: string) {
  const field = w.findAll('.rd-field').find((f) => f.text().includes(label))
  expect(field, `未找到字段「${label}」`).toBeTruthy()
  const input = field!.find('input')
  await input.setValue(value)
}

describe('RedirectorDialog', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    getTemplate.mockReset().mockResolvedValue(OK_RESULT)
  })

  it('后端地址为空时生成按钮禁用（只挡必然失败的情况）', async () => {
    const w = mountDialog()
    await flushPromises()
    const btn = generateBtn(w)
    expect(btn, '未找到生成按钮').toBeTruthy()
    expect(btn!.attributes('disabled')).toBeDefined()
  })

  it('填了后端地址后可按表单拼参数请求', async () => {
    const w = mountDialog()
    await flushPromises()
    await fillByLabel(w, '后端地址', '10.0.0.5:8089')
    await fillByLabel(w, '对外域名', 'cdn.example.com')
    await fillByLabel(w, '诱饵地址', 'https://www.example.com')
    await flushPromises()

    await generateBtn(w)!.trigger('click')
    await flushPromises()

    expect(getTemplate).toHaveBeenCalledTimes(1)
    const arg = getTemplate.mock.calls[0][0]
    expect(arg.type).toBe('nginx')
    expect(arg.profile).toBe('default')
    expect(arg.backend).toBe('10.0.0.5:8089')
    expect(arg.domain).toBe('cdn.example.com')
    expect(arg.decoy).toBe('https://www.example.com')
    // apache 不支持完整配置；nginx 未勾选时 full 必须是 false
    expect(arg.full).toBe(false)
  })

  it('生成后渲染配置内容、文件名与转发路径数', async () => {
    const w = mountDialog()
    await flushPromises()
    await fillByLabel(w, '后端地址', '10.0.0.5:8089')
    await generateBtn(w)!.trigger('click')
    await flushPromises()

    expect(w.text()).toContain('tshell-redirector.conf')
    expect(w.text()).toContain('4 行')
    expect(w.text()).toContain('转发路径')
    // 路径清单逐个渲染，便于运维核对是否与画像一致
    expect(w.text()).toContain('/tencent/mcp/pc/pcsearch')
    const out = w.find('.rd-output').element as HTMLTextAreaElement
    expect(out.value).toContain('Tshell redirector')
  })

  it('服务端返回 400 时把原因回显在页面内（而不是只弹一个全局提示）', async () => {
    getTemplate.mockRejectedValue({
      response: { data: { error: '后端地址不要带协议前缀: "http://10.0.0.5:8089"' } },
    })
    const w = mountDialog()
    await flushPromises()
    await fillByLabel(w, '后端地址', 'http://10.0.0.5:8089')
    await generateBtn(w)!.trigger('click')
    await flushPromises()

    expect(w.find('.rd-error').exists()).toBe(true)
    expect(w.find('.rd-error').text()).toContain('不要带协议前缀')
    // 失败时不应残留上一次的结果
    expect(w.find('.rd-output').exists()).toBe(false)
  })

  it('响应形状不符合预期时明确报错，不把 undefined 塞进预览', async () => {
    getTemplate.mockResolvedValue({ data: { status: 200, data: { nope: true } } })
    const w = mountDialog()
    await flushPromises()
    await fillByLabel(w, '后端地址', '10.0.0.5:8089')
    await generateBtn(w)!.trigger('click')
    await flushPromises()

    expect(w.find('.rd-error').exists()).toBe(true)
    expect(w.find('.rd-output').exists()).toBe(false)
    expect(w.text()).not.toContain('undefined')
  })

  it('勾选完整配置时才要求证书路径，且 full 传 true', async () => {
    const w = mountDialog()
    await flushPromises()
    await fillByLabel(w, '后端地址', '10.0.0.5:8089')

    // 勾上「完整 nginx.conf」：证书/私钥字段应出现
    const checkbox = w.find('.rd-field input[type="checkbox"]')
    expect(checkbox.exists()).toBe(true)
    await checkbox.setValue(true)
    await flushPromises()
    expect(w.text()).toContain('证书路径')
    expect(w.text()).toContain('私钥路径')

    await fillByLabel(w, '证书路径', '/etc/ssl/certs/cdn.pem')
    await fillByLabel(w, '私钥路径', '/etc/ssl/private/cdn.key')
    await generateBtn(w)!.trigger('click')
    await flushPromises()

    const arg = getTemplate.mock.calls[0][0]
    expect(arg.full).toBe(true)
    expect(arg.cert).toBe('/etc/ssl/certs/cdn.pem')
    expect(arg.key).toBe('/etc/ssl/private/cdn.key')
  })

  it('关闭按钮把 visible 置回 false', async () => {
    const w = mountDialog()
    await flushPromises()
    const close = w.findAll('button').find((b) => b.text() === '关闭')
    await close!.trigger('click')
    expect(w.emitted('update:visible')?.[0]).toEqual([false])
  })
})
