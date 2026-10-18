import { describe, expect, it } from 'vitest'
import {
  applyDrag,
  clampHeight,
  clampWidth,
  dragBaseline,
  MIN_HEIGHT,
  MIN_WIDTH,
  usePanelResize,
} from './usePanelResize'

describe('clampWidth / clampHeight', () => {
  it('低于下限时钳到下限', () => {
    expect(clampWidth(100)).toBe(MIN_WIDTH)
    expect(clampWidth(0)).toBe(MIN_WIDTH)
    expect(clampWidth(-500)).toBe(MIN_WIDTH)
    expect(clampHeight(10)).toBe(MIN_HEIGHT)
    expect(clampHeight(-1)).toBe(MIN_HEIGHT)
  })

  it('不低于下限时原样（取整）', () => {
    expect(clampWidth(401)).toBe(401)
    expect(clampWidth(400.4)).toBe(400)
    expect(clampWidth(400.6)).toBe(401)
    expect(clampHeight(200)).toBe(200)
    expect(clampHeight(333.3)).toBe(333)
  })

  it('下限值本身是合法的（边界包含）', () => {
    expect(clampWidth(MIN_WIDTH)).toBe(MIN_WIDTH)
    expect(clampHeight(MIN_HEIGHT)).toBe(MIN_HEIGHT)
  })
})

describe('dragBaseline', () => {
  const current = { width: 800, height: 400 }

  it('both：两个维度都用渲染尺寸当基准', () => {
    const got = dragBaseline('both', { width: 1234, height: 567 }, current)
    expect(got).toEqual({ width: 1234, height: 567 })
  })

  it('width：只动宽度，高度保持当前值', () => {
    // 这是早先实现踩过的坑：把没在拖的维度也重置了，导致拖宽度时高度跳变
    const got = dragBaseline('width', { width: 1234, height: 567 }, current)
    expect(got).toEqual({ width: 1234, height: current.height })
  })

  it('height：只动高度，宽度保持当前值', () => {
    const got = dragBaseline('height', { width: 1234, height: 567 }, current)
    expect(got).toEqual({ width: current.width, height: 567 })
  })

  it('拿不到渲染尺寸时退回当前值（而不是 0 或跳变）', () => {
    // 面板初始是自适应宽度，state 里的 800 与屏幕上看到的并不一致；
    // 取不到渲染尺寸时至少不能把面板缩成 0
    const got = dragBaseline('both', null, current)
    expect(got).toEqual(current)
  })

  it('按下时只取整、不钳制（与原实现一致，避免窄视口下按下就撑破容器）', () => {
    // 下限的语义是"拖不动了"，不是"按下就跳到 400"。
    // 窄视口渲染宽度可能不足 400，原本允许保持原样、只在拖动时被挡住。
    const got = dragBaseline('both', { width: 350, height: 150 }, current)
    expect(got).toEqual({ width: 350, height: 150 })
  })

  it('按下时的渲染尺寸取整', () => {
    const got = dragBaseline('both', { width: 800.6, height: 400.4 }, current)
    expect(got).toEqual({ width: 801, height: 400 })
  })
})

describe('applyDrag', () => {
  const base = { width: 800, height: 400 }
  const current = { width: 800, height: 400 }

  it('both：两个维度都跟着位移走', () => {
    expect(applyDrag('both', base, current, 100, 50)).toEqual({ width: 900, height: 450 })
  })

  it('width：只改宽度，高度不受 deltaY 影响', () => {
    expect(applyDrag('width', base, current, 100, 999)).toEqual({ width: 900, height: 400 })
  })

  it('height：只改高度，宽度不受 deltaX 影响', () => {
    expect(applyDrag('height', base, current, 999, 50)).toEqual({ width: 800, height: 450 })
  })

  it('负位移可以缩小，但被下限挡住', () => {
    expect(applyDrag('both', base, current, -100, -100)).toEqual({ width: 700, height: 300 })
    // 缩过头 → 钳到下限
    expect(applyDrag('both', base, current, -10000, -10000)).toEqual({
      width: MIN_WIDTH,
      height: MIN_HEIGHT,
    })
  })

  it('位移为 0 时尺寸不变', () => {
    expect(applyDrag('both', base, current, 0, 0)).toEqual(base)
  })
})

describe('usePanelResize', () => {
  it('初始尺寸可配置，默认 800x400', () => {
    const a = usePanelResize()
    expect([a.width.value, a.height.value]).toEqual([800, 400])
    const b = usePanelResize({ initialWidth: 1000, initialHeight: 500 })
    expect([b.width.value, b.height.value]).toEqual([1000, 500])
  })

  it('未开始拖拽时 move 不起作用（避免无谓重排）', () => {
    const r = usePanelResize()
    r.move(9999, 9999)
    expect([r.width.value, r.height.value]).toEqual([800, 400])
  })

  it('完整拖拽：按下 → 移动 → 松开', () => {
    const r = usePanelResize()
    r.start('both', 100, 100, { width: 800, height: 400 })
    expect(r.isResizing.value).toBe(true)
    expect(r.kind.value).toBe('both')
    expect(r.userResized.value).toBe(true)

    r.move(150, 130)
    expect([r.width.value, r.height.value]).toEqual([850, 430])

    r.end()
    expect(r.isResizing.value).toBe(false)
    expect(r.kind.value).toBe('')
    // 松开后再移动不应改变尺寸
    r.move(9999, 9999)
    expect([r.width.value, r.height.value]).toEqual([850, 430])
  })

  it('以按下时的渲染尺寸为基准，而不是 state 里的初始值', () => {
    // 面板初始自适应，屏幕上实际是 1000 宽，而 state 里写着 800。
    // 基准若取 800，按下后移动 0 像素就会把面板缩到 800 —— 视觉上是"跳变"。
    const r = usePanelResize({ initialWidth: 800 })
    r.start('width', 0, 0, { width: 1000, height: 400 })
    expect(r.width.value).toBe(1000)
    r.move(0, 0) // 没动
    expect(r.width.value).toBe(1000) // 不应跳回 800
  })

  it('userResized 只在拖拽开始后置位', () => {
    const r = usePanelResize()
    expect(r.userResized.value).toBe(false)
    r.start('height', 0, 0, null)
    expect(r.userResized.value).toBe(true)
  })

  it('连续两次拖拽各自独立（第二次以新尺寸为基准）', () => {
    const r = usePanelResize()
    r.start('both', 0, 0, { width: 800, height: 400 })
    r.move(100, 0)
    r.end()
    expect(r.width.value).toBe(900)

    r.start('both', 0, 0, { width: 900, height: 400 })
    r.move(-100, 0)
    r.end()
    expect(r.width.value).toBe(800)
  })
})
