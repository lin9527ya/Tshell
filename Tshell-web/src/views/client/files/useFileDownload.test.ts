import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import {
  evaluateDownloadPoll,
  MAX_POLL_ATTEMPTS,
  normalizePct,
  POLL_INTERVAL_MS,
  useFileDownload,
} from './useFileDownload'

// ===== 下载轮询的判定逻辑（纯函数部分，覆盖全部边界）=====

describe('evaluateDownloadPoll', () => {
  it('任务完成 → success', () => {
    expect(
      evaluateDownloadPoll({ attempts: 1, hasTask: true, taskStatus: 'completed', pct: 40 }),
    ).toBe('success')
  })

  it('任务失败/超时/取消 → error', () => {
    for (const s of ['failed', 'timeout', 'cancelled'] as const) {
      expect(evaluateDownloadPoll({ attempts: 1, hasTask: true, taskStatus: s, pct: 40 })).toBe('error')
    }
  })

  it('任务仍在跑 → continue，即使百分比已到 100', () => {
    // 这条是重点：有任务时以任务为准。若按百分比判定，
    // "任务已失败的瞬间 pct 恰好 100" 会被误判成功。
    expect(
      evaluateDownloadPoll({ attempts: 1, hasTask: true, taskStatus: '', pct: 100 }),
    ).toBe('continue')
  })

  it('任务已失败但百分比到 100 → error（不被百分比翻案）', () => {
    expect(
      evaluateDownloadPoll({ attempts: 1, hasTask: true, taskStatus: 'failed', pct: 100 }),
    ).toBe('error')
  })

  it('无 task_id 时退回按百分比判定', () => {
    expect(evaluateDownloadPoll({ attempts: 1, hasTask: false, taskStatus: '', pct: 99 })).toBe('continue')
    expect(evaluateDownloadPoll({ attempts: 1, hasTask: false, taskStatus: '', pct: 100 })).toBe('success')
  })

  it('超过次数上限 → timeout（兜底，防永久轮询）', () => {
    expect(
      evaluateDownloadPoll({ attempts: MAX_POLL_ATTEMPTS, hasTask: true, taskStatus: '', pct: 50 }),
    ).toBe('continue')
    expect(
      evaluateDownloadPoll({ attempts: MAX_POLL_ATTEMPTS + 1, hasTask: true, taskStatus: '', pct: 50 }),
    ).toBe('timeout')
  })

  it('超时优先于任务终态（上限已到就不再等）', () => {
    // 顺序有意义：超过上限时即便任务刚好 completed，也应先收敛为超时，
    // 否则"最后一轮刚好完成"与"永远不到头"两种情况无法区分地混在一起。
    expect(
      evaluateDownloadPoll({ attempts: MAX_POLL_ATTEMPTS + 1, hasTask: true, taskStatus: 'completed', pct: 100 }),
    ).toBe('timeout')
  })

  it('可自定义上限（测试与短超时场景）', () => {
    expect(evaluateDownloadPoll({ attempts: 3, hasTask: true, taskStatus: '', pct: 10 }, 2)).toBe('timeout')
  })
})

describe('normalizePct', () => {
  it('收口到 0..100', () => {
    expect(normalizePct(0)).toBe(0)
    expect(normalizePct(50)).toBe(50)
    expect(normalizePct(100)).toBe(100)
    expect(normalizePct(150)).toBe(100)
    expect(normalizePct(-5)).toBe(0)
  })

  it('接受字符串（后端有时回 "42"）', () => {
    expect(normalizePct('42')).toBe(42)
    expect(normalizePct('')).toBe(0)
  })

  it('非法值归零，不产出 NaN', () => {
    // NaN 会把进度条写成 NaN%，界面直接坏掉
    for (const v of [undefined, null, 'abc', NaN, {}]) {
      expect(normalizePct(v)).toBe(0)
    }
  })
})

describe('常量', () => {
  it('轮询间隔与上限与原实现一致', () => {
    expect(POLL_INTERVAL_MS).toBe(1500)
    expect(MAX_POLL_ATTEMPTS).toBe(400) // 400 × 1.5s = 10 分钟
  })
})

// ===== 编排层：用假计时器验证"必须显式收敛"=====

const downloadFile = vi.fn()
const getDownloadsInfo = vi.fn()
const taskGet = vi.fn()

vi.mock('@/api/clients', () => ({
  default: {
    download_file: (...a: unknown[]) => downloadFile(...a),
    get_downloads_info: (...a: unknown[]) => getDownloadsInfo(...a),
  },
}))
vi.mock('@/api/tasks', () => ({ default: { get: (...a: unknown[]) => taskGet(...a) } }))
vi.mock('@/stores/i18n', () => ({
  useI18nStore: () => ({
    isZh: true,
    currentLocale: 'zh',
    setLocale: vi.fn(),
    toggleLocale: vi.fn(),
    t: (k: string, d?: string) => d || k,
    tl: (zh: string, _e: string) => zh,
  }),
}))
vi.mock('element-plus', async () => {
  const actual = await vi.importActual<typeof import('element-plus')>('element-plus')
  return {
    ...actual,
    ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
  }
})

describe('useFileDownload', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.useFakeTimers()
    downloadFile.mockReset().mockResolvedValue({ data: { task_id: 7 } })
    getDownloadsInfo.mockReset().mockResolvedValue({ data: { data: [] } })
    taskGet.mockReset().mockResolvedValue({ data: { data: { status: '' } } })
  })

  afterEach(() => {
    vi.clearAllTimers()
    vi.useRealTimers()
  })

  it('开始下载后 state 进入 downloading 并打开对话框', async () => {
    const d = useFileDownload({ uid: () => 'uid1', intervalMs: 100 })
    await d.start({ name: 'a.txt', path: 'C:\\a.txt' })
    expect(d.status.value).toBe('downloading')
    expect(d.dialogVisible.value).toBe(true)
    expect(d.progress.value).toBe(0)
    expect(downloadFile).toHaveBeenCalledWith({ uid: 'uid1', filePath: 'C:\\a.txt' })
  })

  it('任务 completed 后收敛为 success 并置进度 100', async () => {
    taskGet.mockResolvedValue({ data: { data: { status: 'completed' } } })
    const d = useFileDownload({ uid: () => 'uid1', intervalMs: 100 })
    await d.start({ name: 'a.txt', path: 'C:\\a.txt' })
    await vi.advanceTimersByTimeAsync(120)
    expect(d.status.value).toBe('success')
    expect(d.progress.value).toBe(100)
  })

  it('任务 failed 后收敛为 error', async () => {
    taskGet.mockResolvedValue({ data: { data: { status: 'failed' } } })
    const d = useFileDownload({ uid: () => 'uid1', intervalMs: 100 })
    await d.start({ name: 'a.txt', path: 'C:\\a.txt' })
    await vi.advanceTimersByTimeAsync(120)
    expect(d.status.value).toBe('error')
  })

  it('超过上限后收敛为 error（不再继续轮询）', async () => {
    const d = useFileDownload({ uid: () => 'uid1', intervalMs: 100, maxAttempts: 2 })
    await d.start({ name: 'a.txt', path: 'C:\\a.txt' })
    await vi.advanceTimersByTimeAsync(150) // 第 1 轮
    expect(d.status.value).toBe('downloading')
    await vi.advanceTimersByTimeAsync(120) // 第 2 轮
    await vi.advanceTimersByTimeAsync(400) // 若还在轮询会继续
    expect(d.status.value).toBe('error')
    const calls = taskGet.mock.calls.length
    await vi.advanceTimersByTimeAsync(400)
    // 收敛后不得再查询
    expect(taskGet.mock.calls.length).toBe(calls)
  })

  it('close() 必须停掉计时器（这正是原实现漏掉的地方）', async () => {
    const d = useFileDownload({ uid: () => 'uid1', intervalMs: 100 })
    await d.start({ name: 'a.txt', path: 'C:\\a.txt' })
    d.close()
    expect(d.dialogVisible.value).toBe(false)
    const before = taskGet.mock.calls.length
    await vi.advanceTimersByTimeAsync(1000)
    expect(taskGet.mock.calls.length).toBe(before)
  })

  it('重复 start 不会留下两个计时器', async () => {
    const d = useFileDownload({ uid: () => 'uid1', intervalMs: 100 })
    await d.start({ name: 'a.txt', path: 'C:\\a.txt' })
    await d.start({ name: 'b.txt', path: 'C:\\b.txt' })
    taskGet.mockResolvedValue({ data: { data: { status: 'completed' } } })
    await vi.advanceTimersByTimeAsync(120)
    // 只会有一个计时器在跑，收敛一次即停
    expect(d.status.value).toBe('success')
  })

  it('下发下载任务本身失败时直接进 error', async () => {
    downloadFile.mockRejectedValue({ response: { data: { error: '目标离线' } } })
    const d = useFileDownload({ uid: () => 'uid1', intervalMs: 100 })
    await d.start({ name: 'a.txt', path: 'C:\\a.txt' })
    expect(d.status.value).toBe('error')
  })
})
