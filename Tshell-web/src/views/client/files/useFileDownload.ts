import { ref, type Ref } from 'vue'
import { ElMessage } from 'element-plus'
import ClientAPI from '@/api/clients'
import TaskAPI from '@/api/tasks'
import { useI18nStore } from '@/stores/i18n'

/**
 * 文件下载的进度轮询与终态判定（REMAINING_WORK §5：拆 ClientFiles.vue）。
 *
 * 抽它的理由：这段是组件里最容易出错的一段**状态机** ——
 * 它同时在追两条线（字节进度 + 任务终态），还有超时兜底和"没有 task_id 时
 * 退回按百分比判定"的兼容分支。原实现里的两次修复都发生在这里：
 *   1. 轮询句柄曾只在 pct>=100 时清理 —— 关掉对话框/卸载组件后
 *      1.5s 轮询会永久跑下去（目标文件被清理时永远到不了 100%）
 *   2. 只盯百分比时，失败（目标离线、连接中断）要等 6 分钟轮询超时才提示
 * 这类"必须显式收敛"的逻辑埋在大组件里、没有测试，很容易改回去。
 *
 * 设计上把**判定**与**调度**分开：
 *   evaluateDownloadPoll  纯函数，决定"继续/成功/失败/超时"，可直接断言
 *   useFileDownload       只负责 setInterval 与把结果写进响应式状态
 * 这样绝大多数边界（终态优先级、超时、无 taskId 兜底）不必依赖假计时器。
 */

/** 轮询间隔（与原实现一致） */
export const POLL_INTERVAL_MS = 1500
/** 轮询次数上限：400 × 1.5s = 10 分钟兜底，防永久轮询 */
export const MAX_POLL_ATTEMPTS = 400

export type DownloadStatus = 'idle' | 'downloading' | 'success' | 'error'

/** 任务终态（后端 kind=download 的任务） */
export type TaskTerminal = 'completed' | 'failed' | 'timeout' | 'cancelled' | ''

export interface PollInput {
  /** 本轮是第几次轮询（从 1 开始） */
  attempts: number
  /** 是否已拿到 task_id */
  hasTask: boolean
  /** 任务状态（无任务或查询失败时为空串） */
  taskStatus: TaskTerminal
  /** 后端回报的下载百分比（0-100） */
  pct: number
}

export type PollAction = 'continue' | 'success' | 'error' | 'timeout'

/**
 * evaluateDownloadPoll 决定本轮轮询该收敛还是继续。
 *
 * 优先级刻意如此（顺序即语义）：
 *   1. **任务终态优先于一切** —— 有任务时以它为准，百分比只用于画进度条。
 *      反过来的话，任务已 failed 而 pct 恰好到 100 会被误判成功。
 *   2. 无任务时才退回按百分比判定（兼容后端未建任务的路径）。
 *   3. 超时是最后兜底：`attempts > maxAttempts`。
 */
export function evaluateDownloadPoll(input: PollInput, maxAttempts = MAX_POLL_ATTEMPTS): PollAction {
  if (input.attempts > maxAttempts) {
    return 'timeout'
  }
  if (input.hasTask) {
    if (input.taskStatus === 'completed') return 'success'
    if (input.taskStatus === 'failed' || input.taskStatus === 'timeout' || input.taskStatus === 'cancelled') {
      return 'error'
    }
    // 任务仍在跑（或本次查询失败）：继续轮询，**不看百分比**
    return 'continue'
  }
  // 没有 task_id：退回按百分比判定
  if (input.pct >= 100) return 'success'
  return 'continue'
}

/** 百分比收口：后端偶尔回超范围或非数字 */
export function normalizePct(raw: unknown): number {
  const n = typeof raw === 'number' ? raw : parseInt(String(raw ?? ''), 10)
  if (!Number.isFinite(n) || n < 0) return 0
  return n > 100 ? 100 : n
}

export interface UseFileDownloadOptions {
  /** 当前客户端 uid（每次轮询都会重新取，避免切换客户端后仍在轮询旧的） */
  uid: () => string
  /** 轮询间隔，测试可调小 */
  intervalMs?: number
  /** 轮询次数上限，测试可调小 */
  maxAttempts?: number
}

/**
 * useFileDownload 下载进度轮询的编排层。
 *
 * 所有退出路径（成功/失败/超时/手动关闭/组件卸载）都必须走 stop()，
 * 否则就会回到"关掉对话框但计时器还在跑"的老问题上。
 */
export function useFileDownload(opts: UseFileDownloadOptions) {
  const i18nStore = useI18nStore()
  const isZh = { get value() { return i18nStore.isZh } }

  const progress: Ref<number> = ref(0)
  const status: Ref<DownloadStatus> = ref('idle')
  const currentName = ref('')
  const dialogVisible = ref(false)

  let timer: ReturnType<typeof setInterval> | null = null

  function stop() {
    if (timer !== null) {
      clearInterval(timer)
      timer = null
    }
  }

  /** 下发下载任务并开始轮询 */
  async function start(file: { name?: string; path: string }) {
    currentName.value = file.name || file.path
    progress.value = 0
    status.value = 'downloading'
    dialogVisible.value = true

    // 重新下载前先停掉上一轮，避免两个计时器同时写状态
    stop()

    try {
      const res = await ClientAPI.download_file({ uid: opts.uid(), filePath: file.path })
      const taskId = Number((res as { data?: { task_id?: number } })?.data?.task_id) || 0
      let attempts = 0
      const maxAttempts = opts.maxAttempts ?? MAX_POLL_ATTEMPTS

      timer = setInterval(async () => {
        attempts++
        let pct = progress.value
        let taskStatus: TaskTerminal = ''

        try {
          const infoRes = await ClientAPI.get_downloads_info({ uid: opts.uid() })
          const downloads = ((infoRes.data as { data?: unknown[] })?.data || []) as Array<{
            filePath?: string
            downloadPart?: unknown
          }>
          const hit = downloads.find((d) => d.filePath === file.path)
          if (hit) {
            pct = normalizePct(hit.downloadPart)
            progress.value = pct
          }
        } catch {
          // 单次查询失败不判定，下一轮重试
        }

        if (taskId) {
          try {
            const taskRes = await TaskAPI.get(taskId)
            const data = (taskRes.data as { data?: { status?: string } })?.data
            const s = data?.status || ''
            if (s === 'completed' || s === 'failed' || s === 'timeout' || s === 'cancelled') {
              taskStatus = s
            }
          } catch {
            // 同上：查询失败不判定
          }
        }

        const action = evaluateDownloadPoll(
          { attempts, hasTask: taskId > 0, taskStatus, pct },
          maxAttempts,
        )

        if (action === 'continue') return
        stop()

        if (action === 'success') {
          progress.value = 100
          status.value = 'success'
          ElMessage.success(
            isZh.value
              ? `文件 ${currentName.value} 已下载至团队服务器`
              : `Downloaded ${currentName.value}`,
          )
          return
        }

        if (action === 'error') {
          status.value = 'error'
          ElMessage.error(isZh.value ? '下载未完成' : 'Download did not finish')
          return
        }

        // timeout
        status.value = 'error'
        ElMessage.warning(
          isZh.value
            ? '等待下载结果超时,请到任务中心查看该任务状态'
            : 'Timed out waiting for the download; check the Task Center',
        )
      }, opts.intervalMs ?? POLL_INTERVAL_MS)
    } catch (e: unknown) {
      status.value = 'error'
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
      ElMessage.error(msg || (isZh.value ? '下发下载任务失败' : 'Download failed'))
    }
  }

  /** 关闭对话框：停止轮询（必须调，否则计时器泄漏） */
  function close() {
    stop()
    dialogVisible.value = false
  }

  return { progress, status, currentName, dialogVisible, start, stop, close }
}
