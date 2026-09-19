import { computed, inject, onBeforeUnmount, onMounted } from 'vue'
import type { ComputedRef } from 'vue'

// P1-6: 轮询 visibility 治理——页面隐藏（切后台/最小化）时暂停 setInterval，
// 恢复可见时立即补一次。用于 Clients/Shell 等多处轮询，降低后台空转 API 压力。
//
// 【第二轮增强】第三参数 isActive: 宿主(标签页)不可见时也暂停轮询。
// 两个壳(CsTabContainer 的 v-show 全挂载 / VShellApp 的 KeepAlive)都会让
// 非活动面板常驻,原实现只在 document 隐藏时暂停 → 切走的标签页仍在后台轮询。
// 面板用法: useVisibilityInterval(fetch, 5000, { isActive: () => tabActive.value })
// 其中 tabActive 来自 useTabActive()(宿主 provide 'tabActive';未提供时恒 true)。
// 返回 { start, stop }，组件卸载自动停止。
export function useVisibilityInterval(
  fn: () => void,
  intervalMs: number,
  opts?: { isActive?: () => boolean },
) {
  let timer: ReturnType<typeof setInterval> | null = null
  let stopped = false

  const active = () => {
    if (document.visibilityState === 'hidden') return false
    if (opts?.isActive && !opts.isActive()) return false
    return true
  }

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

  onBeforeUnmount(() => {
    stopped = true
    stop()
    document.removeEventListener('visibilitychange', handleVisibility)
  })

  return { start, stop }
}

/**
 * 当前面板是否为活动标签页。
 * 宿主(CsTabContainer.TabContentHost / VShellApp.PanelHost)通过 provide('tabActive')
 * 注入;未提供时恒为 true(组件不在标签体系内使用时的保守回退)。
 */
export function useTabActive(): ComputedRef<boolean> {
  return inject<ComputedRef<boolean>>(
    'tabActive',
    computed(() => true),
  )
}
