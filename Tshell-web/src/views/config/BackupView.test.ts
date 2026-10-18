import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ElementPlus from 'element-plus'

const list = vi.fn()
const create = vi.fn()
const remove = vi.fn()
const download = vi.fn()
const doImport = vi.fn()

vi.mock('@/api/backup', () => ({
  default: {
    list: (...a: unknown[]) => list(...a),
    create: (...a: unknown[]) => create(...a),
    remove: (...a: unknown[]) => remove(...a),
    download: (...a: unknown[]) => download(...a),
    import: (...a: unknown[]) => doImport(...a),
  },
}))

vi.mock('@/layout/Dialogs/Payloads/payloadUtil', () => ({ downloadBlob: vi.fn() }))

import BackupView from './BackupView.vue'

const info = (over: Record<string, unknown> = {}) => ({
  name: 'tshell-backup-20261018-101530.000.db',
  path: '/srv/backups/tshell-backup-20261018-101530.000.db',
  size: 3145728,
  created_at: '2026-10-18T10:15:30Z',
  verified: true,
  note: 'manual:admin',
  ...over,
})

const mountView = () => mount(BackupView, { global: { plugins: [ElementPlus] } })

describe('BackupView（设置页「备份」页签）', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    list.mockReset().mockResolvedValue({ data: { data: { dir: './backups', keep: 7, list: [] } } })
    create.mockReset().mockResolvedValue({ data: { data: info() } })
    remove.mockReset().mockResolvedValue({ data: { status: 200 } })
    download.mockReset().mockResolvedValue({ data: new Blob(['x']) })
    doImport.mockReset().mockResolvedValue({ data: { status: 200 } })
  })

  it('展示备份目录、保留份数与备份条目', async () => {
    list.mockResolvedValue({
      data: { data: { dir: './backups', keep: 7, list: [info(), info({ name: 'tshell-backup-x.db', verified: false, size: 1024 })] } },
    })
    const wrapper = mountView()
    await flushPromises()

    const text = wrapper.text()
    expect(text).toContain('./backups')
    expect(text).toContain('保留 7 份')
    expect(text).toContain('tshell-backup-20261018-101530.000.db')
    expect(text).toContain('3.0 MB')
    expect(text).toContain('已校验')
    expect(text).toContain('未校验') // 未通过校验的备份要一眼看出来
  })

  it('恢复必须走 CLI：页面给出命令而不是"恢复"按钮', async () => {
    const wrapper = mountView()
    await flushPromises()

    // 这是刻意的设计（SQLite 无法热替换），用例守住它不被改成假按钮
    expect(wrapper.text()).toContain('./tshell -restore')
    expect(wrapper.text()).toContain('停止服务端')
    expect(wrapper.findAll('button').filter((b) => /恢复|Restore/.test(b.text()))).toHaveLength(0)
  })

  it('删除需二次确认，确认后调用接口并刷新', async () => {
    list.mockResolvedValue({ data: { data: { dir: './backups', keep: 7, list: [info()] } } })
    const wrapper = mountView()
    await flushPromises()

    const ep: any = await import('element-plus')
    const confirmSpy = vi.spyOn(ep.ElMessageBox, 'confirm').mockResolvedValue('confirm')
    const delLink = wrapper.findAll('a.row-link').find((a) => /删除|Delete/.test(a.text()))
    expect(delLink).toBeTruthy()
    await delLink!.trigger('click')
    await flushPromises()

    expect(confirmSpy).toHaveBeenCalled()
    expect(remove).toHaveBeenCalledWith('tshell-backup-20261018-101530.000.db')
  })

  it('取消删除时不调用接口', async () => {
    list.mockResolvedValue({ data: { data: { dir: './backups', keep: 7, list: [info()] } } })
    const wrapper = mountView()
    await flushPromises()

    const ep: any = await import('element-plus')
    vi.spyOn(ep.ElMessageBox, 'confirm').mockRejectedValue('cancel')
    const delLink = wrapper.findAll('a.row-link').find((a) => /删除|Delete/.test(a.text()))
    await delLink!.trigger('click')
    await flushPromises()

    expect(remove).not.toHaveBeenCalled()
  })
})
