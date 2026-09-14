import { onBeforeUnmount, onMounted } from 'vue'

// P1-6: 轮询 visibility 治理——页面隐藏（切后台/最小化）时暂停 setInterval，
// 恢复可见时立即补一次。用于 Clients/Shell 等多处轮询，降低后台空转 API 压力。
// 返回 { start, stop, isRunning }，组件卸载自动停止。
export function useVisibilityInterval(fn: () => void, intervalMs: number) {
  let timer: ReturnType<typeof setInterval> | null = null
  let stopped = false

  const stop = () => {
    if (timer) {
      clearInterval(timer)
      timer = null
    }
  }

  const start = () => {
    if (stopped) return
    stop()
    timer = setInterval(() => {
      if (document.visibilityState !== 'hidden') fn()
    }, intervalMs)
  }

  const handleVisibility = () => {
    if (document.visibilityState === 'hidden') {
      stop()
    } else if (timer === null && !stopped) {
      fn() // 恢复可见时立即刷新一次
      start()
    }
  }

  onMounted(() => {
    document.addEventListener('visibilitychange', handleVisibility)
    start()
  })

  onBeforeUnmount(() => {
    stopped = true
    stop()
    document.removeEventListener('visibilitychange', handleVisibility)
  })

  return { start, stop }
}
