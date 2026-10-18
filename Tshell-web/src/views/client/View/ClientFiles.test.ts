import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import ElementPlus from 'element-plus'

// ===== ClientFiles 组件级测试 =====
//
// 这是拆 2128 行大文件之前要有的安全网。重点盯**上一轮抽取的 format.ts 接线**：
// 那几个格式化函数自己单测很足（32 项），但组件有没有真的用上、
// 用的是不是同一份实现，只有挂载组件才看得出来。
// 典型漏法：组件里又留了一份本地实现，于是 format.ts 测得很好而页面走的是旧逻辑。

const getDrives = vi.fn()
const getFileTree = vi.fn()
const getDownloadsInfo = vi.fn()

vi.mock('@/api/clients', () => ({
  default: {
    get_drives: (...a: unknown[]) => getDrives(...a),
    get_file_tree: (...a: unknown[]) => getFileTree(...a),
    get_downloads_info: (...a: unknown[]) => getDownloadsInfo(...a),
    send_commands: vi.fn(),
    fetch_file_content: vi.fn(),
    save_file_content: vi.fn(),
    create_file: vi.fn(),
    make_dir: vi.fn(),
    delete_file: vi.fn(),
    rename_file: vi.fn(),
    copy_file: vi.fn(),
    download_file: vi.fn(),
    download_downloaded_file: vi.fn(),
    upload_file: vi.fn(),
    search_files: vi.fn(),
    hide_file: vi.fn(),
    unhide_file: vi.fn(),
    modify_file_time: vi.fn(),
  },
}))

vi.mock('@/api/tasks', () => ({ default: { get: vi.fn(), getTaskResult: vi.fn() } }))

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

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { uid: '0123456789abcdef' }, query: {} }),
}))

vi.mock('element-plus', async () => {
  const actual = await vi.importActual<typeof import('element-plus')>('element-plus')
  return {
    ...actual,
    ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
    ElMessageBox: {
      confirm: vi.fn().mockResolvedValue('confirm'),
      prompt: vi.fn().mockResolvedValue({ value: 'x' }),
    },
  }
})

import ClientFiles from './ClientFiles.vue'

/** 挂载时通过 provide 注入 uid（组件的 getUid 优先用注入值，且要求长度 > 10） */
function mountView() {
  return mount(ClientFiles, {
    global: {
      plugins: [ElementPlus],
      provide: { realUid: ref('0123456789abcdef') },
      stubs: { teleport: true },
    },
  })
}

describe('ClientFiles（文件管理）', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    // get_drives 回的是**对象数组**（每项带 path/name），不是裸字符串数组：
    // 组件会取 `data[0].path || data[0].name` 作为初始路径再去拉文件树。
    // 形状给错时不会报错，只是 path 变 undefined、文件树拉不出来 ——
    // 表现就是"列表是空的"，很容易误判成渲染问题。
    getDrives.mockReset().mockResolvedValue({
      data: {
        status: 200,
        data: [{ name: 'C:', path: 'C:\\' }],
      },
    })
    // get_file_tree 回的是**树**（节点带 children），组件会按 dirPath 找到
    // 对应节点后把它的 children 当作当前列表。回平铺数组时列表会是空的 ——
    // 不报错，只是"看起来没有文件"。
    getFileTree.mockReset().mockResolvedValue({
      data: {
        status: 200,
        data: [
          {
            name: 'C:',
            path: 'C:\\',
            type: 'D',
            children: [
              { name: 'docs', type: 'D', path: 'C:\\docs', size: 0, modifiedTime: '2026-08-17 14:32:00' },
              { name: 'tool.exe', type: 'F', path: 'C:\\tool.exe', size: 1024, modifiedTime: '2026-08-17 14:32:00' },
              { name: 'run.ps1', type: 'F', path: 'C:\\run.ps1', size: 2048, modifiedTime: '2026-08-17 14:32:00' },
              { name: 'notes.txt', type: 'F', path: 'C:\\notes.txt', size: 512, modifiedTime: '2026-08-17 14:32:00' },
            ],
          },
        ],
      },
    })
    getDownloadsInfo.mockReset().mockResolvedValue({ data: { status: 200, data: [] } })
  })

  it('挂载后按注入的 uid 拉取磁盘列表', async () => {
    mountView()
    await flushPromises()
    // get_drives 收的是对象（{ uid }）而不是裸字符串 —— 传错形状时
    // mock 仍会被调用，断言写成裸串就会失败在这种"参数形状"上
    expect(getDrives).toHaveBeenCalledWith({ uid: '0123456789abcdef' })
  })

  it('渲染不报错（i18n/API 替身覆盖了组件用到的接口）', async () => {
    const errors: unknown[] = []
    const spy = vi.spyOn(console, 'error').mockImplementation((...a) => errors.push(a))
    const w = mountView()
    await flushPromises()
    const msg = errors.map(String).join('\n')
    expect(msg).not.toContain('is not a function')
    spy.mockRestore()
    expect(w.exists()).toBe(true)
  })

  it('文件列表用 format.ts 的图标/名称类名（目录与可执行文件区分开）', async () => {
    const w = mountView()
    await flushPromises()

    // 先确认文件列表**真的渲染了** —— 否则下面的类名断言可以"因为没渲染"而空转。
    // （我第一版就是这么写的：if (html.includes(...)) {...} else {跳过}，
    //   结果组件在挂载后根本没渲染列表，那条测试一直是假通过。）
    expect(getFileTree).toHaveBeenCalled()
    const html = w.html()
    expect(html, '文件列表未渲染，类名断言会变成空转').toContain('fm-item-icon')

    // 目录 → icon-folder / name-folder；可执行 → icon-binary / name-binary；
    // 脚本 → icon-script / name-script。这些类名来自 format.ts，
    // 若组件里又留了一份本地实现，类名就会不一致，这条会失败。
    expect(html).toContain('icon-folder')
    expect(html).toContain('name-folder')
    expect(html).toContain('icon-binary')
    expect(html).toContain('name-binary')
    expect(html).toContain('icon-script')
    expect(html).toContain('name-script')
  })

  it('文件大小走 formatFileSize（1024 字节显示为 1.0 KB）', async () => {
    const w = mountView()
    await flushPromises()
    const html = w.html()
    // 1KB / 2KB / 512B 三种量级都应格式化，而不是显示裸字节数
    expect(html).toContain('1.0 KB')
    expect(html).toContain('2.0 KB')
    expect(html).toContain('512 B')
  })
})
