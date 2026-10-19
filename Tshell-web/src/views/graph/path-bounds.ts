/**
 * 路径几何（**仅供测试**）：把 path 的 d 解析成包围盒与最远点半径。
 *
 * 为什么需要它：图标都画在半径 9 的节点圆里，坐标写错就会溢出去 ——
 * 而 jsdom 没有 getBBox()，光看代码又看不出「这个数字大了」。所以这里自己算：
 *  - 直线/曲线按端点与**控制点**取样（贝塞尔曲线落在控制点凸包内，保守但不会漏）；
 *  - 圆弧按端点参数化真正采样（弧的鼓出量不在端点上，只看端点会漏掉整段弧）。
 *
 * 教训：早先"扫描 d 里的数字、要求都 ≤ 8"的检查是**假检查** ——
 * 圆弧命令里的数字是增量（a6.3 6.3 0 1 1 0 12.6 里的 12.6 是直径），必然误判；
 * 而真正畸形的图形（左侧多出两段鼓包）它反而查不出来。
 */
import { type GlyphFit } from './glyph'
import type { Pt } from './path-bounds-types'

const ARITY: Record<string, number> = {
  M: 2, L: 2, H: 1, V: 1, C: 6, S: 4, Q: 4, T: 2, A: 7, Z: 0,
}

export interface PathBounds {
  minX: number
  minY: number
  maxX: number
  maxY: number
  /** 采样点到原点 (0,0) 的最大距离 —— 节点圆半径就是拿它比的 */
  maxRadius: number
}

/** 圆弧端点参数化 → 圆心参数化，再按角度采样（SVG 规范 F.6.5） */
function sampleArc(from: Pt, rx: number, ry: number, rotDeg: number, largeArc: number, sweep: number, to: Pt, steps: number): Pt[] {
  let crx = Math.abs(rx)
  let cry = Math.abs(ry)
  if (crx === 0 || cry === 0) return [to]
  const phi = (rotDeg * Math.PI) / 180
  const cosP = Math.cos(phi)
  const sinP = Math.sin(phi)
  const dx = (from.x - to.x) / 2
  const dy = (from.y - to.y) / 2
  const x1p = cosP * dx + sinP * dy
  const y1p = -sinP * dx + cosP * dy
  // 半径过小则按规范放大到刚好够
  const lambda = (x1p * x1p) / (crx * crx) + (y1p * y1p) / (cry * cry)
  if (lambda > 1) {
    const s = Math.sqrt(lambda)
    crx *= s
    cry *= s
  }
  const sign = largeArc !== sweep ? 1 : -1
  const num = crx * crx * cry * cry - crx * crx * y1p * y1p - cry * cry * x1p * x1p
  const den = crx * crx * y1p * y1p + cry * cry * x1p * x1p
  const co = sign * Math.sqrt(Math.max(0, num / den))
  const cxp = (co * crx * y1p) / cry
  const cyp = (-co * cry * x1p) / crx
  const cx = cosP * cxp - sinP * cyp + (from.x + to.x) / 2
  const cy = sinP * cxp + cosP * cyp + (from.y + to.y) / 2
  const a1 = Math.atan2((y1p - cyp) / cry, (x1p - cxp) / crx)
  const a2 = Math.atan2((-y1p - cyp) / cry, (-x1p - cxp) / crx)
  let dTheta = a2 - a1
  if (!sweep && dTheta > 0) dTheta -= 2 * Math.PI
  if (sweep && dTheta < 0) dTheta += 2 * Math.PI
  const pts: Pt[] = []
  for (let i = 0; i <= steps; i++) {
    const t = a1 + (dTheta * i) / steps
    pts.push({
      x: cx + crx * Math.cos(t) * cosP - cry * Math.sin(t) * sinP,
      y: cy + crx * Math.cos(t) * sinP + cry * Math.sin(t) * cosP,
    })
  }
  return pts
}

/**
 * pathBounds 解析 path。
 * @param fit 可选：图标的坐标系变换（glyphFit 的结果），给了就返回**变换后**的包围盒 ——
 *            图标原始 viewBox 各不相同（24×24 / 448×512…），不换算没法比"是否落在圆内"。
 */
export function pathBounds(d: string, arcSteps = 24, fit?: GlyphFit): PathBounds {
  const tokens = d.match(/[MmLlHhVvCcSsQqTtAaZz]|-?\d*\.?\d+(?:e[-+]?\d+)?/g) || []
  const pts: Pt[] = []
  let cur: Pt = { x: 0, y: 0 }
  let start: Pt = { x: 0, y: 0 }
  let prevCtrl: Pt | null = null
  let cmd = ''
  let i = 0
  const num = () => Number(tokens[i++])

  while (i < tokens.length) {
    if (/[A-Za-z]/.test(tokens[i])) {
      cmd = tokens[i++]
      if (cmd === 'Z' || cmd === 'z') {
        cur = { ...start }
        pts.push(cur)
        prevCtrl = null
        continue
      }
    }
    const up = cmd.toUpperCase()
    const rel = cmd !== up
    const arity = ARITY[up]
    if (arity === undefined) throw new Error(`未知命令 ${cmd}`)
    const a: number[] = []
    for (let k = 0; k < arity; k++) a.push(num())
    const base = rel ? cur : { x: 0, y: 0 }
    const p = (x: number, y: number): Pt => ({ x: base.x + x, y: base.y + y })

    switch (up) {
      case 'M':
      case 'L': {
        cur = p(a[0], a[1])
        pts.push(cur)
        if (up === 'M') start = { ...cur }
        prevCtrl = null
        break
      }
      case 'H': {
        cur = { x: (rel ? cur.x : 0) + a[0], y: cur.y }
        pts.push(cur)
        prevCtrl = null
        break
      }
      case 'V': {
        cur = { x: cur.x, y: (rel ? cur.y : 0) + a[0] }
        pts.push(cur)
        prevCtrl = null
        break
      }
      case 'C': {
        const c1 = p(a[0], a[1])
        const c2 = p(a[2], a[3])
        const end = p(a[4], a[5])
        pts.push(c1, c2, end)
        prevCtrl = c2
        cur = end
        break
      }
      case 'S': {
        const c1 = prevCtrl ? { x: 2 * cur.x - prevCtrl.x, y: 2 * cur.y - prevCtrl.y } : cur
        const c2 = p(a[0], a[1])
        const end = p(a[2], a[3])
        pts.push(c1, c2, end)
        prevCtrl = c2
        cur = end
        break
      }
      case 'Q': {
        const c1 = p(a[0], a[1])
        const end = p(a[2], a[3])
        pts.push(c1, end)
        prevCtrl = c1
        cur = end
        break
      }
      case 'T': {
        const c1 = prevCtrl ? { x: 2 * cur.x - prevCtrl.x, y: 2 * cur.y - prevCtrl.y } : cur
        const end = p(a[0], a[1])
        pts.push(c1, end)
        prevCtrl = c1
        cur = end
        break
      }
      case 'A': {
        const end = p(a[5], a[6])
        for (const q of sampleArc(cur, a[0], a[1], a[2], a[3], a[4], end, arcSteps)) pts.push(q)
        prevCtrl = null
        cur = end
        break
      }
    }
  }

  if (!pts.length) return { minX: 0, minY: 0, maxX: 0, maxY: 0, maxRadius: 0 }
  const mapped = fit ? pts.map((q) => ({ x: q.x * fit.s + fit.tx, y: q.y * fit.s + fit.ty })) : pts
  const xs = mapped.map((q) => q.x)
  const ys = mapped.map((q) => q.y)
  return {
    minX: Math.min(...xs),
    minY: Math.min(...ys),
    maxX: Math.max(...xs),
    maxY: Math.max(...ys),
    maxRadius: Math.max(...mapped.map((q) => Math.hypot(q.x, q.y))),
  }
}
