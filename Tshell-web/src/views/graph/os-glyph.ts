/**
 * 节点上的系统图标（Windows / Linux / 其它）。
 *
 * 为什么是内联 path 而不是图标库或图标字体：
 *  1. 拓扑图要能**导出成自包含的 SVG/PNG** —— 图标字体、<use> 外链在导出后会丢；
 *  2. 这两个形状手写只要几十字节，不值得为此拉一个依赖（本项目对前端依赖一向克制）。
 *
 * 坐标系：以 (0,0) 为中心的 16×16 方框。节点圆半径 9，图形最远点 ~7.9，正好落在圆内。
 * 单色填充：颜色由 CSS 的 fill 决定（在线=白色，离线=灰色），见 PivotGraphView 的 .pg-node-os。
 */
export interface OsGlyph {
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

/** 显示器（未知系统）：与项目里 desktop 图标同形，一眼是"某台机器" */
const OTHER_D = [
  'M-6 -5.5h12a1.5 1.5 0 0 1 1.5 1.5v6a1.5 1.5 0 0 1-1.5 1.5h-12A1.5 1.5 0 0 1-7.5 3.5v-6A1.5 1.5 0 0 1-6 -5.5z',
  'M-1.2 3.5h2.4v2h-2.4z',
  'M-4 5.5h8a.9 .9 0 0 1 .9 .9v.1a.9 .9 0 0 1-.9 .9h-8a.9 .9 0 0 1-.9-.9v-.1a.9 .9 0 0 1 .9-.9z',
].join(' ')

export const OS_GLYPHS: Record<OsGlyphKey, OsGlyph> = {
  windows: { d: WINDOWS_D, zh: 'Windows', en: 'Windows' },
  linux: { d: LINUX_D, zh: 'Linux', en: 'Linux' },
  other: { d: OTHER_D, zh: '其它系统', en: 'Other OS' },
}

export type OsGlyphKey = 'windows' | 'linux' | 'other'

/**
 * osGlyphKey 把服务端上报的 os 归一到三种图标。
 * 认不出的一律走 other（画成显示器），不返回空 —— 空图标在图上和"没数据"分不清。
 */
export function osGlyphKey(os?: string): OsGlyphKey {
  const v = (os || '').trim().toLowerCase()
  if (!v) return 'other'
  if (v.startsWith('win')) return 'windows'
  if (v.includes('linux') || v === 'ubuntu' || v === 'debian' || v === 'centos' || v === 'kali') {
    return 'linux'
  }
  return 'other'
}
