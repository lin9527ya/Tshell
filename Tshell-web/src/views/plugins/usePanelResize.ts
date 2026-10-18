import { ref, type Ref } from 'vue'

/**
 * 插件输出面板的拖拽缩放（阶段三 §3.11：拆 1793 行的 PluginsView.vue）。
 *
 * 抽它的理由：这块有**三处容易写错、且错了不容易看出来**的规则 ——
 *   1. 最小尺寸钳制（宽 ≥ 400、高 ≥ 200）
 *   2. 三种拖拽模式（both / width / height）各自只改一个维度，且另一个维度的
 *      起点值必须保留（早先的实现把没在拖的维度也重置了）
 *   3. 拖拽起点用**当前渲染尺寸**而不是 state 里的值 —— 面板初始是自适应宽度，
 *      state 里还写着 800，直接用 800 当基准会让面板在按下鼠标的瞬间跳变
 *
 * 这些规则原先埋在组件里、没有任何测试；抽出来之后可以直接断言，
 * 不必去 mount 一个 1793 行的组件再模拟鼠标。
 */

/** 面板最小尺寸（再小就看不清输出内容了） */
export const MIN_WIDTH = 400
export const MIN_HEIGHT = 200

/** 拖拽模式：both=右下角手柄，width=右边框，height=下边框 */
export type ResizeKind = 'both' | 'width' | 'height'

export interface PanelSize {
  width: number
  height: number
}

/** 把尺寸钳制到合法范围（纯函数，便于直接断言边界） */
export function clampWidth(w: number): number {
  return Math.max(MIN_WIDTH, Math.round(w))
}

export function clampHeight(h: number): number {
  return Math.max(MIN_HEIGHT, Math.round(h))
}

/**
 * 按下鼠标时的基准尺寸。
 *
 * rendered 为元素的**当前渲染尺寸**（可能为 null —— 元素还没挂载）。
 * 有渲染尺寸时以它为准，否则退回 state 里的值：
 * 面板初始宽度是自适应的，state 里的 800 与屏幕上看到的并不一致，
 * 用 state 当基准会让面板在按下瞬间跳变。
 *
 * 注意这里**只取整、不钳制**（与原实现一致）。按下时钳制看似更"正确"，
 * 但会改变行为：窄视口下渲染宽度可能不足 400，原本允许它保持原样、
 * 只在拖动时才被下限挡住；按下就钳到 400 反而可能撑破容器。
 * 下限的作用是"拖不动了"，不是"按下就跳"。
 */
export function dragBaseline(kind: ResizeKind, rendered: PanelSize | null, current: PanelSize): PanelSize {
  const out: PanelSize = { width: current.width, height: current.height }
  if (kind === 'both' || kind === 'width') {
    out.width = rendered ? Math.round(rendered.width) : current.width
  }
  if (kind === 'both' || kind === 'height') {
    out.height = rendered ? Math.round(rendered.height) : current.height
  }
  return out
}

/** 按位移算出新尺寸（只改该模式负责的维度，另一维原样保留） */
export function applyDrag(
  kind: ResizeKind,
  base: PanelSize,
  current: PanelSize,
  deltaX: number,
  deltaY: number,
): PanelSize {
  const out: PanelSize = { width: current.width, height: current.height }
  if (kind === 'both' || kind === 'width') {
    out.width = clampWidth(base.width + deltaX)
  }
  if (kind === 'both' || kind === 'height') {
    out.height = clampHeight(base.height + deltaY)
  }
  return out
}

export interface UsePanelResizeOptions {
  initialWidth?: number
  initialHeight?: number
}

/**
 * 组合式封装：把上面的纯计算接到 Vue 响应式状态上。
 *
 * 组件只负责把 DOM 事件与元素尺寸喂进来，规则本身在这里。
 */
export function usePanelResize(opts: UsePanelResizeOptions = {}) {
  const width: Ref<number> = ref(opts.initialWidth ?? 800)
  const height: Ref<number> = ref(opts.initialHeight ?? 400)
  const isResizing = ref(false)
  const kind = ref<ResizeKind | ''>('')
  /** 用户是否手动调整过（调整过就不再自适应） */
  const userResized = ref(false)

  let startX = 0
  let startY = 0
  let base: PanelSize = { width: width.value, height: height.value }

  /** 开始拖拽。rendered 传元素的当前渲染尺寸（取不到就传 null） */
  function start(k: ResizeKind, clientX: number, clientY: number, rendered: PanelSize | null) {
    isResizing.value = true
    kind.value = k
    startX = clientX
    startY = clientY
    base = dragBaseline(k, rendered, { width: width.value, height: height.value })
    width.value = base.width
    height.value = base.height
    userResized.value = true
  }

  /** 拖拽中 */
  function move(clientX: number, clientY: number) {
    if (!isResizing.value || !kind.value) return
    const next = applyDrag(kind.value, base, { width: width.value, height: height.value }, clientX - startX, clientY - startY)
    width.value = next.width
    height.value = next.height
  }

  /** 结束拖拽 */
  function end() {
    isResizing.value = false
    kind.value = ''
  }

  return { width, height, isResizing, kind, userResized, start, move, end }
}
