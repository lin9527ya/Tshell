import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ElementPlus from 'element-plus'

const dump = vi.fn()
const listArtifacts = vi.fn()
const downloadDownloadedFile = vi.fn()

vi.mock('@/api/cred', () => ({
  default: {
    dump: (...a: unknown[]) => dump(...a),
    listArtifacts: (...a: unknown[]) => listArtifacts(...a),
  },
  // 对话框直接用到这几个导出的纯函数，mock 时必须一并给（否则渲染期就是 undefined）
  CRED_TECHS: { lsass: ['auto', 'comsvcs', 'dbgcore'], dpapi: ['keys'], browser: ['all', 'chrome', 'edge', 'firefox', 'cards'], ticket: ['files', 'lsa'] },
  defaultTech: (kind: string) => ({ dpapi: 'keys', browser: 'all' } as Record<string, string>)[kind] || 'auto',
  sourceLabel: (source: string) =>
    ({ lsass: 'LSASS 转储', dpapi: 'DPAPI 主密钥', browser: '浏览器凭据' } as Record<string, string>)[source] || source,
  artifactExt: (source: string) => ({ dpapi: '.zip', browser: '.json' } as Record<string, string>)[source] || '.dmp',
}))

vi.mock('@/api/clients', () => ({
  default: {
    download_downloaded_file: (...a: unknown[]) => downloadDownloadedFile(...a),
  },
}))

vi.mock('@/layout/Dialogs/Payloads/payloadUtil', () => ({ downloadBlob: vi.fn() }))

import CredentialDumpDialog from './CredentialDumpDialog.vue'

const pending = (over: Record<string, unknown> = {}) => ({
  uid: 'uid-win-1',
  kind: 'lsass',
  file_name: 'lsass-DC01-20261018-120000.dmp',
  tech: 'auto',
  task_id: 12,
  operator: 'alice',
  issued_at: '2026-10-18T12:00:00Z',
  expires_at: '2026-10-18T12:30:00Z',
  ...over,
})

const artifact = (over: Record<string, unknown> = {}) => ({
  id: 3,
  uid: 'uid-win-1',
  type: 'dump',
  source: 'lsass',
  secret: '/srv/Downloads/1/lsass-DC01-20261018-120000.dmp',
  note: 'LSASS 内存转储，312.0 MB / 12 个数据流 / 含完整内存；本地解析: pypykatz lsa minidump <文件>',
  created_at: '2026-10-18 12:03:00',
  ...over,
})

// el-dialog 带 append-to-body：内容被 teleport 到 document.body，
// 断言要读 document 而不是 wrapper
const bodyText = () => document.body.textContent || ''
const findButton = (re: RegExp) =>
  Array.from(document.querySelectorAll('button')).find((b) => re.test(b.textContent || '')) as HTMLButtonElement | undefined
const findLink = (re: RegExp) =>
  Array.from(document.querySelectorAll('a.row-link')).find((a) => re.test(a.textContent || '')) as HTMLElement | undefined

const mountDialog = () =>
  mount(CredentialDumpDialog, {
    props: { modelValue: true, uid: 'uid-win-1' },
    global: { plugins: [ElementPlus] },
  })

describe('CredentialDumpDialog（凭据获取）', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })
  beforeEach(() => {
    setActivePinia(createPinia())
    dump.mockReset().mockResolvedValue({ data: { status: 200, task_id: 12, data: { file_name: 'lsass-DC01-20261018-120000.dmp' } } })
    listArtifacts.mockReset().mockResolvedValue({ data: { data: { pending: [], artifacts: [] } } })
    downloadDownloadedFile.mockReset().mockResolvedValue({ status: 200, data: new Blob(['dump']) })
  })

  it('说明"服务端不解析、需本地解析"，避免操作员以为服务端会自动出凭据', async () => {
    const wrapper = mountDialog()
    await flushPromises()
    const text = bodyText()
    expect(text).toContain('不做凭据解析')
    expect(text).toContain('pypykatz lsa minidump')
    expect(text).toContain('从目标磁盘删除')
  })

  it('展示等待回传与已落地制品', async () => {
    listArtifacts.mockResolvedValue({
      data: { data: { pending: [pending()], artifacts: [artifact()] } },
    })
    const wrapper = mountDialog()
    await flushPromises()

    const text = bodyText()
    expect(text).toContain('等待中')
    expect(text).toContain('lsass-DC01-20261018-120000.dmp')
    expect(text).toContain('#12')
    expect(text).toContain('含完整内存')
  })

  it('发起获取时带上 uid/kind/tech', async () => {
    const wrapper = mountDialog()
    await flushPromises()

    const vm = wrapper.vm as any
    vm.form.tech = 'comsvcs'
    await flushPromises()
    const startBtn = findButton(/开始获取|Start/)
    expect(startBtn).toBeTruthy()
    startBtn!.click()
    await flushPromises()

    expect(dump).toHaveBeenCalledWith({ uid: 'uid-win-1', kind: 'lsass', tech: 'comsvcs' })
    // 下发后立即刷新一次（让"等待回传"立刻出现）
    expect(listArtifacts).toHaveBeenCalled()
  })

  it('技术选型切换时给出对应说明（PPL 成败差异）', async () => {
    const wrapper = mountDialog()
    await flushPromises()
    const vm = wrapper.vm as any

    vm.form.tech = 'dbgcore'
    await flushPromises()
    expect(bodyText()).toContain('PPL')

    vm.form.tech = 'comsvcs'
    await flushPromises()
    expect(bodyText()).toContain('rundll32')
  })

  it('取回制品走文件下载接口', async () => {
    listArtifacts.mockResolvedValue({ data: { data: { pending: [], artifacts: [artifact()] } } })
    const wrapper = mountDialog()
    await flushPromises()

    const link = findLink(/取回|Fetch/)
    expect(link).toBeTruthy()
    link!.click()
    await flushPromises()

    expect(downloadDownloadedFile).toHaveBeenCalledWith({
      uid: 'uid-win-1',
      filePath: '/srv/Downloads/1/lsass-DC01-20261018-120000.dmp',
    })
  })

  // ===== 第二批凭据模块（DPAPI / 浏览器）的界面行为 =====

  it('切换类型时技术重置为该类型的默认值（避免跨类型误配）', async () => {
    const wrapper = mountDialog()
    await flushPromises()
    const vm = wrapper.vm as any

    vm.form.tech = 'comsvcs'
    await flushPromises()

    // 模板里的下拉 change 事件内联到渲染函数，不会挂到 vm 上，
    // 所以这里显式走同一个函数：改 kind + 重置 tech
    const changeKind = (k: string) => { vm.form.kind = k; vm.onKindChange(k) }

    changeKind('browser')
    await flushPromises()
    expect(vm.form.tech).toBe('all')

    changeKind('dpapi')
    await flushPromises()
    expect(vm.form.tech).toBe('keys')

    changeKind('lsass')
    await flushPromises()
    expect(vm.form.tech).toBe('auto')
  })

  it('按类型给出对应说明（DPAPI / 浏览器）', async () => {
    const wrapper = mountDialog()
    await flushPromises()
    const vm = wrapper.vm as any

    vm.form.kind = 'browser'
    await flushPromises()
    let text = bodyText()
    expect(text).toContain('内存中完成解密')
    expect(text).toContain('明文不落盘')

    vm.form.kind = 'dpapi'
    await flushPromises()
    text = bodyText()
    expect(text).toContain('DPAPI 主密钥')
    expect(text).toContain('masterkey')
    expect(text).toContain('Credential Manager')
  })

  it('按所选类型下发（browser/chrome）', async () => {
    const wrapper = mountDialog()
    await flushPromises()
    const vm = wrapper.vm as any

    vm.form.kind = 'browser'
    vm.form.tech = 'chrome'
    await flushPromises()

    const startBtn = findButton(/开始获取|Start/)
    startBtn!.click()
    await flushPromises()

    expect(dump).toHaveBeenCalledWith({ uid: 'uid-win-1', kind: 'browser', tech: 'chrome' })
  })

  it('browser 的技术选项包含 Firefox 与 cards（Web Data）', async () => {
    const wrapper = mountDialog()
    await flushPromises()
    const vm = wrapper.vm as any

    vm.form.kind = 'browser'
    await flushPromises()

    const values = (vm.techOptions as Array<{ value: string }>).map((t) => t.value)
    expect(values).toEqual(['all', 'chrome', 'edge', 'firefox', 'cards'])
  })

  it('选定 firefox / cards 时按对应技术下发', async () => {
    const wrapper = mountDialog()
    await flushPromises()
    const vm = wrapper.vm as any
    vm.form.kind = 'browser'

    for (const tech of ['firefox', 'cards']) {
      dump.mockClear()
      vm.form.tech = tech
      await flushPromises()
      findButton(/开始获取|Start/)!.click()
      await flushPromises()
      expect(dump).toHaveBeenCalledWith({ uid: 'uid-win-1', kind: 'browser', tech })
    }
  })

  it('ticket 类型提供 files / lsa 两种技术，lsa 提示 kirbi', async () => {
    const wrapper = mountDialog()
    await flushPromises()
    const vm = wrapper.vm as any

    vm.form.kind = 'ticket'
    await flushPromises()

    const values = (vm.techOptions as Array<{ value: string }>).map((t) => t.value)
    expect(values).toEqual(['files', 'lsa'])

    vm.form.tech = 'lsa'
    await flushPromises()
    expect(bodyText()).toMatch(/kirbi|LSA/i)

    findButton(/开始获取|Start/)!.click()
    await flushPromises()
    expect(dump).toHaveBeenCalledWith({ uid: 'uid-win-1', kind: 'ticket', tech: 'lsa' })
  })

  it('制品列表按来源显示类型徽标', async () => {
    listArtifacts.mockResolvedValue({
      data: {
        data: {
          pending: [],
          artifacts: [
            artifact({ id: 1, source: 'dpapi', secret: '/srv/Downloads/1/dpapi-WS01.zip', note: 'DPAPI 主密钥集' }),
            artifact({ id: 2, source: 'browser', secret: '/srv/Downloads/1/browser-WS01.json', note: '浏览器凭据制品' }),
          ],
        },
      },
    })
    const wrapper = mountDialog()
    await flushPromises()

    const text = bodyText()
    expect(text).toContain('DPAPI 主密钥')
    expect(text).toContain('浏览器凭据')
  })
})
