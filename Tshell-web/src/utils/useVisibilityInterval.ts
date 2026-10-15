import { onActivated, onBeforeUnmount, onDeactivated, onMounted } from 'vue'

// 轮询 visibility 治理——页面隐藏（切后台/最小化）或标签页被 KeepAlive 切走时
// 暂停 setInterval，恢复时立即补一次。用于 Clients/Shell 等多处轮询，降低后台空转。
//
// 标签可见性由组件自身的 onActivated/onDeactivated 维护（路由页在 <keep-alive> 下
// 切走即 onDeactivated），不再依赖宿主 provide('tabActive')——宿主契约已随 CS 壳移除。
// 返回 { start, stop }，组件卸载自动停止。
export function useVisibilityInterval(fn: () => void, intervalMs: number) {
  let timer: ReturnType<typeof setInterval> | null = null
  let stopped = false
  // KeepAlive 标签被切走（onDeactivated）；未纳入 KeepAlive 的组件该钩子不触发
  let deactivated = false

  const active = () => document.visibilityState !== 'hidden' && !deactivated

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
      if (active()) fn()
    }, intervalMs)
  }

  const handleVisibility = () => {
    if (document.visibilityState === 'hidden') {
      stop()
    } else if (timer === null && !stopped) {
      if (active()) fn() // 恢复可见时立即刷新一次
      start()
    }
  }

  onMounted(() => {
    document.addEventListener('visibilitychange', handleVisibility)
    start()
  })

  // 标签切回：立即补一次并恢复轮询（首次挂载时 deactivated 仍为 false，不重复拉取）
  onActivated(() => {
    if (!deactivated) return
    deactivated = false
    if (timer === null && !stopped) {
      fn()
      start()
    }
  })

  onDeactivated(() => {
    deactivated = true
    stop()
  })

  onBeforeUnmount(() => {
    stopped = true
    stop()
    document.removeEventListener('visibilitychange', handleVisibility)
  })

  return { start, stop }
}
