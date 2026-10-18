import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ElementPlus from 'element-plus'

// ===== 插件上传对话框组件测试 =====
//
// 拆出这个组件的**主要收益**就在这里：以前它的校验规则与"最多 6 个文件"上限
// 只能通过挂载整个 PluginsView 才能触达（而且还要先 mock 一整套插件/客户端 API），
// 现在可以直接挂载对话框本身来测。

const uploadPlugin = vi.fn()
vi.mock('@/api/plugin', () => ({
  default: { upload_plugin: (...a: unknown[]) => uploadPlugin(...a) },
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

import PluginUploadDialog from './PluginUploadDialog.vue'
import { ElMessage } from 'element-plus'
import { formatPluginFileSize } from './formatSize'

/** 造一个 File（jsdom 支持 File 构造） */
function makeFile(name: string, size = 1024, type = ''): File {
  const f = new File(['x'], name, { type })
  Object.defineProperty(f, 'size', { value: size })
  return f
}

function mountDialog(visible = true) {
  return mount(PluginUploadDialog, {
    props: { visible },
    global: { plugins: [ElementPlus] },
  })
}

describe('PluginUploadDialog', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    uploadPlugin.mockReset().mockResolvedValue({ data: { status: 200 } })
    vi.mocked(ElMessage.warning).mockClear()
    vi.mocked(ElMessage.success).mockClear()
    vi.mocked(ElMessage.error).mockClear()
  })

  it('空列表时开始上传会提示先选文件，且不发请求', async () => {
    const w = mountDialog()
    await flushPromises()
    const startBtn = w.findAll('button').find((b) => b.text().includes('开始上传'))
    expect(startBtn, '未找到"开始上传"按钮').toBeTruthy()
    await startBtn!.trigger('click')
    await flushPromises()
    expect(uploadPlugin).not.toHaveBeenCalled()
    expect(ElMessage.warning).toHaveBeenCalled()
  })

  it('关闭时把 visible 置回 false（供 v-model 使用）', async () => {
    const w = mountDialog()
    await flushPromises()
    const closeBtn = w.findAll('button').find((b) => b.text().includes('关闭'))
    await closeBtn!.trigger('click')
    expect(w.emitted('update:visible')?.[0]).toEqual([false])
  })

  it('扩展名白名单拒绝非插件文件（校验在组件内，不再需要挂载整个视图）', async () => {
    const w = mountDialog()
    await flushPromises()
    const input = w.find('input[type="file"]')
    const bad = makeFile('photo.png', 100)
    Object.defineProperty(input.element, 'files', { value: [bad] })
    await input.trigger('change')
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalled()
    // 被拒的文件不应进入列表
    expect(w.text()).not.toContain('photo.png')
  })

  it('超过 50MB 的文件被拒绝', async () => {
    const w = mountDialog()
    await flushPromises()
    const input = w.find('input[type="file"]')
    const big = makeFile('huge.exe', 51 * 1024 * 1024, 'application/x-msdownload')
    Object.defineProperty(input.element, 'files', { value: [big] })
    await input.trigger('change')
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalled()
    expect(w.text()).not.toContain('huge.exe')
  })

  it('合法插件文件进入列表并显示大小', async () => {
    const w = mountDialog()
    await flushPromises()
    const input = w.find('input[type="file"]')
    const ok = makeFile('fscan.exe', 1536, 'application/x-msdownload')
    Object.defineProperty(input.element, 'files', { value: [ok] })
    await input.trigger('change')
    await flushPromises()
    expect(w.text()).toContain('fscan.exe')
    expect(w.text()).toContain('1.5 KB')
  })

  it('最多同时上传 6 个：超出的被截断并提示', async () => {
    const w = mountDialog()
    await flushPromises()
    const input = w.find('input[type="file"]')
    const files = Array.from({ length: 8 }, (_, i) => makeFile(`p${i}.exe`, 1024, 'application/x-msdownload'))
    Object.defineProperty(input.element, 'files', { value: files })
    await input.trigger('change')
    await flushPromises()
    // banner 承诺 6 个，这里必须兑现
    expect(w.text()).toContain('p5.exe')
    expect(w.text()).not.toContain('p6.exe')
    expect(w.text()).not.toContain('p7.exe')
    expect(ElMessage.warning).toHaveBeenCalled()
  })

  it('上传成功后发出 uploaded 事件（父组件据此刷新列表）', async () => {
    const w = mountDialog()
    await flushPromises()
    const input = w.find('input[type="file"]')
    Object.defineProperty(input.element, 'files', {
      value: [makeFile('a.exe', 1024, 'application/x-msdownload')],
    })
    await input.trigger('change')
    await flushPromises()

    const startBtn = w.findAll('button').find((b) => b.text().includes('开始上传'))
    await startBtn!.trigger('click')
    await flushPromises()

    expect(uploadPlugin).toHaveBeenCalledTimes(1)
    expect(w.emitted('uploaded')).toBeTruthy()
  })

  it('单个文件失败不中断其余文件的上传', async () => {
    // 原实现一个 4xx 就跳出整个循环，后面的文件全都不传了
    uploadPlugin
      .mockReset()
      .mockResolvedValueOnce({ data: { status: 500, message: '失败' } })
      .mockResolvedValueOnce({ data: { status: 200 } })

    const w = mountDialog()
    await flushPromises()
    const input = w.find('input[type="file"]')
    Object.defineProperty(input.element, 'files', {
      value: [makeFile('bad.exe', 1024, 'application/x-msdownload'), makeFile('good.exe', 1024, 'application/x-msdownload')],
    })
    await input.trigger('change')
    await flushPromises()

    const startBtn = w.findAll('button').find((b) => b.text().includes('开始上传'))
    await startBtn!.trigger('click')
    await flushPromises()

    // 两个文件都要被尝试
    expect(uploadPlugin).toHaveBeenCalledTimes(2)
    // 有成功即应通知父组件刷新
    expect(w.emitted('uploaded')).toBeTruthy()
  })
})

describe('formatPluginFileSize（与上传对话框显示一致）', () => {
  it('对话框里的 1.5 KB 与共享格式化函数一致', () => {
    // 组件用的是这个函数，不是文件管理器那版 —— 两版在 1024 上结果不同
    expect(formatPluginFileSize(1536)).toBe('1.5 KB')
    expect(formatPluginFileSize(1024)).toBe('1 KB')
  })
})
