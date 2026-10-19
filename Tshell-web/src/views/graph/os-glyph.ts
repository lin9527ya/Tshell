/**
 * 客户端节点的系统图标（Windows / Linux / macOS / 其它）。
 *
 * 用公开图标集的原始路径，不再手绘：
 *  - Windows：simple-icons（正投影的四方块，18px 下仍然清晰；FontAwesome 那版是斜投影，小了会糊）
 *  - Linux：MDI（块面化的企鹅剪影；FontAwesome/simple-icons 的企鹅细节多，小尺寸糊成一团）
 *  - macOS：FontAwesome brands（苹果剪影 + 叶子）
 *  - 其它：项目自带图标集的 desktop（显示器）
 * 各集许可：simple-icons CC0、MDI Apache-2.0、FontAwesome CC BY 4.0（项目图标集同源）。
 */
import { type Glyph } from './glyph'
import { vsIconPath } from '@/layout-vshell/vs-icons'

export type OsGlyphKey = 'windows' | 'linux' | 'darwin' | 'other'

/** simple-icons:windows */
const WINDOWS: Glyph = {
  vb: '0 0 24 24',
  d: 'M0 0h11.377v11.372H0Zm12.623 0H24v11.372H12.623ZM0 12.623h11.377V24H0Zm12.623 0H24V24H12.623',
  zh: 'Windows',
  en: 'Windows',
}

/** mdi:linux */
const LINUX: Glyph = {
  vb: '0 0 24 24',
  d: 'M14.62 8.35c-.42.28-1.75 1.04-1.95 1.19c-.39.31-.75.29-1.14-.01c-.2-.16-1.53-.92-1.95-1.19c-.48-.31-.45-.7.08-.92c1.64-.69 3.28-.64 4.91.03c.49.21.51.6.05.9m7.22 7.28c-.93-2.09-2.2-3.99-3.84-5.66a4.3 4.3 0 0 1-1.06-1.88c-.1-.33-.17-.67-.24-1.01c-.2-.88-.29-1.78-.7-2.61c-.73-1.58-2-2.4-3.84-2.47c-1.81.05-3.16.81-3.95 2.4c-.21.43-.36.88-.46 1.34c-.17.76-.32 1.55-.5 2.32c-.15.65-.45 1.21-.96 1.71c-1.61 1.57-2.9 3.37-3.88 5.35c-.14.29-.28.58-.37.88c-.19.66.29 1.12.99.96c.44-.09.88-.18 1.3-.31c.41-.15.57-.05.67.35c.65 2.15 2.07 3.66 4.24 4.5c4.12 1.56 8.93-.66 9.97-4.58c.07-.27.17-.37.47-.27c.46.14.93.24 1.4.35c.49.09.85-.16.92-.64c.03-.26-.06-.49-.16-.73',
  zh: 'Linux',
  en: 'Linux',
}

/** fa-brands:apple */
const DARWIN: Glyph = {
  vb: '0 0 384 512',
  d: 'M318.7 268.7c-.2-36.7 16.4-64.4 50-84.8c-18.8-26.9-47.2-41.7-84.7-44.6c-35.5-2.8-74.3 20.7-88.5 20.7c-15 0-49.4-19.7-76.4-19.7C63.3 141.2 4 184.8 4 273.5q0 39.3 14.4 81.2c12.8 36.7 59 126.7 107.2 125.2c25.2-.6 43-17.9 75.8-17.9c31.8 0 48.3 17.9 76.4 17.9c48.6-.7 90.4-82.5 102.6-119.3c-65.2-30.7-61.7-90-61.7-91.9m-56.6-164.2c27.3-32.4 24.8-61.9 24-72.5c-24.1 1.4-52 16.4-67.9 34.9c-17.5 19.8-27.8 44.3-25.6 71.9c26.1 2 49.9-11.4 69.5-34.3',
  zh: 'macOS',
  en: 'macOS',
}

const OTHER: Glyph = (() => {
  const p = vsIconPath('desktop')
  return { d: p?.d || '', vb: p?.vb || '0 0 16 16', zh: '其它系统', en: 'Other OS' }
})()

export const OS_GLYPHS: Record<OsGlyphKey, Glyph> = {
  windows: WINDOWS,
  linux: LINUX,
  darwin: DARWIN,
  other: OTHER,
}

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
