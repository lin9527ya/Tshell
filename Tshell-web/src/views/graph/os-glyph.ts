/**
 * 节点上的系统图标（Windows / Linux / macOS / 其它）。
 *
 * 为什么是内联 path 而不是图标库或图标字体：
 *  1. 拓扑图要能**导出成自包含的 SVG/PNG** —— 图标字体、<use> 外链在导出后会丢；
 *  2. 这两个形状手写只要几十字节，不值得为此拉一个依赖（本项目对前端依赖一向克制）。
 *
 * 坐标系：以 (0,0) 为中心的 16×16 方框。节点圆半径 9，图形最远点 ~7.9，正好落在圆内。
 * 单色填充：颜色由 CSS 的 fill 决定（在线=白色，离线=灰色），见 PivotGraphView 的 .pg-node-glyph。
 * 需要"空心"的图形（准星、地球）靠**子路径反向缠绕**实现（nonzero 填充规则下互相抵消），
 * 所以那些路径里同一个图形画两遍（外圈顺时针、内圈逆时针），不能随手改方向。
 */
export interface Glyph {
  /** 单条 path 的 d（多个子路径用空格分隔） */
  d: string
  zh: string
  en: string
}

/** 四个方块（Windows 徽标） */
const WINDOWS_D =
  'M-5.5 -5.5h5v5h-5z M0.5 -5.5h5v5h-5z M-5.5 0.5h5v5h-5z M0.5 0.5h5v5h-5z'

/** 企鹅（Tux）：头 + 身 + 两只脚 + 喙 */
const LINUX_D = [
  'M0 -7.5a2.9 2.9 0 1 1 0 5.8a2.9 2.9 0 1 1 0-5.8z', // 头
  'M-4 1.7a4 4.3 0 1 1 8 0a4 4.3 0 1 1-8 0z', // 身
  'M-4.3 6.6a1.7 .95 0 1 1 3.4 0a1.7 .95 0 1 1-3.4 0z', // 左脚
  'M0.9 6.6a1.7 .95 0 1 1 3.4 0a1.7 .95 0 1 1-3.4 0z', // 右脚
  'M2.4 -5.2L5.8 -4L2.4 -2.8z', // 喙
].join(' ')

/**
 * 苹果（macOS/darwin）：身体 + 右上咬口 + 一片叶子。
 * 咬口是路径里的一段圆弧（A 命令，sweep=1 向内凹），不是"再画个圆去减"——
 * 单色填充下没有减法，evenodd 只会把圆外那半也填上。
 */
const DARWIN_D = [
  'M0 -2.8C-1 -4 -3 -4 -4.2 -2.6C-5.6 -.8 -5.4 2.8 -3.9 4.9C-2.6 6.6 -.9 6.6 0 6.2',
  'C.9 6.6 2.6 6.6 3.9 4.9C5 3.4 5.4 1.2 5.2 .4A3 3 0 0 1 3 -2.6C2.7 -3.6 1.5 -3.9 0 -2.8Z',
  'M.4 -3.6C1.2 -5.2 2.9 -5.9 4.2 -5.8C4 -4.2 2.6 -3.2 .4 -3.6Z', // 叶子
].join('')

/**
 * 显示器（未知系统）：与项目里 desktop 图标同形，一眼是"某台机器"。
 * 屏 + 支架 + 底座三段，左右对称、上下居中（此前左侧两段圆弧写成了鼓包：
 * 图形歪了 0.75、宽到 15.9、还往下偏 1.0 —— 由 path-bounds 的单测兜住）。
 * 底座在下方会让重心偏低，所以整块上移 1 个单位，让包围盒居中。
 */
const OTHER_D = [
  'M-4.5 -6.5h9a1.5 1.5 0 0 1 1.5 1.5v6a1.5 1.5 0 0 1-1.5 1.5h-9a1.5 1.5 0 0 1-1.5-1.5v-6a1.5 1.5 0 0 1 1.5-1.5z',
  'M-1.2 2.5h2.4v2h-2.4z',
  'M-4.4 4.5h8.8v2h-8.8z',
].join(' ')

export const OS_GLYPHS: Record<OsGlyphKey, Glyph> = {
  windows: { d: WINDOWS_D, zh: 'Windows', en: 'Windows' },
  linux: { d: LINUX_D, zh: 'Linux', en: 'Linux' },
  darwin: { d: DARWIN_D, zh: 'macOS', en: 'macOS' },
  other: { d: OTHER_D, zh: '其它系统', en: 'Other OS' },
}

export type OsGlyphKey = 'windows' | 'linux' | 'darwin' | 'other'

/**
 * osGlyphKey 把服务端上报的 os 归一到四种图标。
 * 认不出的一律走 other（画成显示器），不返回空 —— 空图标在图上和"没数据"分不清。
 */
export function osGlyphKey(os?: string): OsGlyphKey {
  const v = (os || '').trim().toLowerCase()
  if (!v) return 'other'
  if (v.startsWith('win')) return 'windows'
  if (v === 'darwin' || v === 'macos' || v === 'osx' || v.startsWith('mac')) return 'darwin'
  if (v.includes('linux') || v === 'ubuntu' || v === 'debian' || v === 'centos' || v === 'kali') {
    return 'linux'
  }
  return 'other'
}
