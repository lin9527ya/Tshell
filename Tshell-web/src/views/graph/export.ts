/**
 * 拓扑图导出（SVG / PNG）。
 *
 * 做法是**克隆画布里的 <svg> 再补一份内联样式**，而不是按数据重画一遍：
 * 重画等于把节点、边、系统图标、离线样式的画法复制第二份，迟早与画布不一致。
 * 克隆的代价是 scoped CSS 进不去（导出文件离开页面就没有那些类了），
 * 所以这里手工维护一份导出用的样式 —— 改画布样式时这份也要跟着改。
 *
 * 导出的是**整张图**（按包围盒摆正），与当前缩放/平移无关：
 * 否则"我缩到 40% 导出"会得到一张小得看不清的图。
 */
import { type GraphBounds } from './layout'

const SVG_NS = 'http://www.w3.org/2000/svg'

/** 导出时四周留白 */
export const EXPORT_PAD = 26

export interface ExportTheme {
  /** 背景（不透明：PNG 里透明背景 + 深色文字在聊天工具里会看不清） */
  bg: string
  text: string
  edge: string
  edgeOffline: string
  edgeActive: string
  grid: string
  /** 离线节点的灰 */
  offline: string
}

export const DEFAULT_EXPORT_THEME: ExportTheme = {
  bg: '#ffffff',
  text: '#1f2937',
  edge: '#94a3b8',
  edgeOffline: '#cbd5e1',
  edgeActive: '#8b9dc3',
  grid: 'rgba(148,163,184,0.35)',
  offline: '#9ca3af',
}

export interface ExportedSvg {
  text: string
  width: number
  height: number
}

/** 导出用的样式：与 PivotGraphView 的 scoped CSS 对应 */
function exportCss(t: ExportTheme): string {
  return `
svg { font-family: system-ui, -apple-system, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif; }
.pg-edge { stroke: ${t.edge}; stroke-width: 1.2; opacity: 0.55; }
.pg-edge-active { stroke: ${t.edgeActive}; opacity: 0.9; }
/* 离线主机：虚线 + 更淡 —— 与在线实线一眼可分 */
.pg-edge-offline { stroke: ${t.edgeOffline}; stroke-dasharray: 5 4; opacity: 0.9; }
.pg-edge-label {
  font-size: 10px; fill: ${t.text}; opacity: 0.75; text-anchor: middle;
  paint-order: stroke; stroke: ${t.bg}; stroke-width: 3px; stroke-linejoin: round;
}
/* 导出不画悬停/选中的光晕 */
.pg-node-halo { opacity: 0; }
.pg-node-selected .pg-node-halo { opacity: 0; }
.pg-node-label { font-size: 12px; fill: ${t.text}; }
.pg-node-sub { font-size: 10px; fill: ${t.text}; opacity: 0.55; }
.pg-node-offline .pg-node-label { fill: ${t.offline}; }
.pg-node-offline .pg-node-sub { fill: ${t.offline}; opacity: 0.7; }
.pg-node-glyph { fill: #ffffff; }
.pg-node-offline .pg-node-glyph { fill: ${t.offline}; }
`.trim()
}

/**
 * buildExportSvg 把画布 <svg> 克隆成自包含的 SVG 文本。
 *
 * @param svg    画布里的根 <svg>（内容需在 <g class="pg-content"> 里）
 * @param bounds 内容包围盒（layout.graphBounds），导出按它摆正并算尺寸
 */
export function buildExportSvg(
  svg: SVGSVGElement,
  bounds: GraphBounds,
  opts: { pad?: number; theme?: Partial<ExportTheme> } = {},
): ExportedSvg {
  const pad = opts.pad ?? EXPORT_PAD
  const theme: ExportTheme = { ...DEFAULT_EXPORT_THEME, ...(opts.theme || {}) }
  const width = Math.max(1, Math.round(bounds.maxX - bounds.minX + pad * 2))
  const height = Math.max(1, Math.round(bounds.maxY - bounds.minY + pad * 2))

  const clone = svg.cloneNode(true) as SVGSVGElement
  clone.setAttribute('xmlns', SVG_NS)
  clone.setAttribute('width', String(width))
  clone.setAttribute('height', String(height))
  clone.setAttribute('viewBox', `0 0 ${width} ${height}`)
  clone.removeAttribute('style')

  // 内容组：把画布的"平移+缩放"换成按包围盒摆正（导出与当前视口无关）
  const content = clone.querySelector('g.pg-content')
  if (content) {
    const tx = (pad - bounds.minX).toFixed(2)
    const ty = (pad - bounds.minY).toFixed(2)
    content.setAttribute('transform', `translate(${tx} ${ty})`)
  }

  const style = document.createElementNS(SVG_NS, 'style')
  style.textContent = exportCss(theme)

  // 点阵网格与画布同款（22px 一格），导出图看起来还是这张图
  const defs = document.createElementNS(SVG_NS, 'defs')
  const pattern = document.createElementNS(SVG_NS, 'pattern')
  pattern.setAttribute('id', 'pg-grid')
  pattern.setAttribute('width', '22')
  pattern.setAttribute('height', '22')
  pattern.setAttribute('patternUnits', 'userSpaceOnUse')
  const dot = document.createElementNS(SVG_NS, 'circle')
  dot.setAttribute('cx', '1')
  dot.setAttribute('cy', '1')
  dot.setAttribute('r', '1')
  dot.setAttribute('fill', theme.grid)
  pattern.appendChild(dot)
  defs.appendChild(pattern)

  const bg = document.createElementNS(SVG_NS, 'rect')
  bg.setAttribute('x', '0')
  bg.setAttribute('y', '0')
  bg.setAttribute('width', String(width))
  bg.setAttribute('height', String(height))
  bg.setAttribute('fill', theme.bg)

  const grid = document.createElementNS(SVG_NS, 'rect')
  grid.setAttribute('x', '0')
  grid.setAttribute('y', '0')
  grid.setAttribute('width', String(width))
  grid.setAttribute('height', String(height))
  grid.setAttribute('fill', 'url(#pg-grid)')

  const first = clone.firstChild
  clone.insertBefore(bg, first)
  clone.insertBefore(grid, first)
  clone.insertBefore(defs, first)
  clone.insertBefore(style, first)

  return {
    text: `<?xml version="1.0" encoding="UTF-8"?>\n${new XMLSerializer().serializeToString(clone)}`,
    width,
    height,
  }
}

/** svgToPngBlob 把导出的 SVG 文本栅格化。scale=2 → 2 倍像素，贴到聊天工具里不糊 */
export async function svgToPngBlob(
  text: string,
  width: number,
  height: number,
  scale = 2,
): Promise<Blob> {
  const url = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(text)}`
  const img = new Image()
  img.width = width
  img.height = height
  await new Promise<void>((resolve, reject) => {
    img.onload = () => resolve()
    img.onerror = () => reject(new Error('SVG 解析失败'))
    img.src = url
  })
  const canvas = document.createElement('canvas')
  canvas.width = Math.max(1, Math.round(width * scale))
  canvas.height = Math.max(1, Math.round(height * scale))
  const ctx = canvas.getContext('2d')
  if (!ctx) throw new Error('无法创建画布上下文')
  ctx.drawImage(img, 0, 0, canvas.width, canvas.height)
  return await new Promise<Blob>((resolve, reject) => {
    canvas.toBlob((b) => (b ? resolve(b) : reject(new Error('PNG 编码失败'))), 'image/png')
  })
}

/** downloadBlob 触发浏览器下载（用完立刻回收 objectURL） */
export function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.style.display = 'none'
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

/** 导出文件名：tshell-topology-20261019-143012.svg */
export function exportFilename(ext: 'svg' | 'png', now = new Date()): string {
  const p = (n: number) => String(n).padStart(2, '0')
  const stamp =
    `${now.getFullYear()}${p(now.getMonth() + 1)}${p(now.getDate())}` +
    `-${p(now.getHours())}${p(now.getMinutes())}${p(now.getSeconds())}`
  return `tshell-topology-${stamp}.${ext}`
}
