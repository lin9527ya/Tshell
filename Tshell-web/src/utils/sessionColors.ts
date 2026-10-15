// 标记色单一来源（截图库/进程表等直接铺色的界面）。#ffffff 为首项，表示清除标记。

export const SESSION_MARKER_COLORS = [
  { value: '#ffffff', zh: '默认白色', en: 'White' },
  { value: '#ef5350', zh: '红色', en: 'Red' },
  { value: '#ff9800', zh: '橙色', en: 'Orange' },
  { value: '#ffee58', zh: '黄色', en: 'Yellow' },
  { value: '#66bb6a', zh: '绿色', en: 'Green' },
  { value: '#26c6da', zh: '青色', en: 'Cyan' },
  { value: '#42a5f5', zh: '蓝色', en: 'Blue' },
  { value: '#ab47bc', zh: '紫色', en: 'Purple' },
]

export interface RowTintOptions {
  /** 十六进制透明度后缀，各表视觉不同：截图 30 等 */
  alpha?: string
  /** 传入则加左侧色条 */
  borderWidth?: number
}

/** 行标记底色；#ffffff 与空值视为无标记 */
export function rowTintStyle(color?: string | null, opts: RowTintOptions = {}): Record<string, string> {
  if (!color || color === '#ffffff') return {}
  const tint = `${color}${opts.alpha ?? '3d'}`
  const style: Record<string, string> = {
    '--el-table-tr-bg-color': tint,
    backgroundColor: tint,
  }
  if (opts.borderWidth) style.borderLeft = `${opts.borderWidth}px solid ${color}`
  return style
}
