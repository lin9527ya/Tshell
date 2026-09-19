// 会话/目标标记色单一来源。
// 此前 ColorDialog(8 色盘)、GlobalScreenshotsView(8 色盘点阵)、
// CsBeaconContextMenu 与 TargetsView(CS 原生 6 色)各持一份，色值与命名易漂移。

export interface SessionColorOption {
  name: string
  zhName: string
  /** 存储值 + 行底色用的深色调（'' 为清除标记） */
  value: string
  bg: string
  border: string
  /** 亮色调：文本/节点描边等以高对比呈现的界面 */
  text: string
}

export const SESSION_COLOR_PALETTE: SessionColorOption[] = [
  { name: 'Red', zhName: '红色', value: '#d32f2f', bg: '#d32f2f22', border: '#d32f2f', text: '#ef5350' },
  { name: 'Orange', zhName: '橙色', value: '#e65100', bg: '#e6510022', border: '#e65100', text: '#ff9800' },
  { name: 'Yellow', zhName: '黄色', value: '#f57f17', bg: '#f57f1722', border: '#f57f17', text: '#ffee58' },
  { name: 'Green', zhName: '绿色', value: '#2e7d32', bg: '#2e7d3222', border: '#2e7d32', text: '#66bb6a' },
  { name: 'Cyan', zhName: '青色', value: '#00838f', bg: '#00838f22', border: '#00838f', text: '#26c6da' },
  { name: 'Blue', zhName: '蓝色', value: '#1565c0', bg: '#1565c022', border: '#1565c0', text: '#42a5f5' },
  { name: 'Purple', zhName: '紫色', value: '#6a1b9a', bg: '#6a1b9a22', border: '#6a1b9a', text: '#ab47bc' },
  { name: 'Default', zhName: '默认', value: '', bg: 'transparent', border: 'var(--cs-border)', text: 'var(--cs-text)' },
]

/** 亮色调点阵（截图管理等直接铺色的界面）；#ffffff 为首项，表示清除标记 */
export const SESSION_MARKER_COLORS = [
  { value: '#ffffff', zh: '默认白色', en: 'White' },
  ...SESSION_COLOR_PALETTE.filter((c) => c.value).map((c) => ({
    value: c.text,
    zh: c.zhName,
    en: c.name,
  })),
]

/** Cobalt Strike 原生右键 6 色（上下文菜单与目标表沿用 CS 视觉） */
export const CS_MARKER_COLORS = [
  { value: '#ffffff', label: 'White' },
  { value: '#c8e6c9', label: 'Light Green' },
  { value: '#ffcdd2', label: 'Light Pink' },
  { value: '#fff59d', label: 'Light Yellow' },
  { value: '#9e9e9e', label: 'Grey' },
  { value: '#3f51b5', label: 'Dark Blue' },
]

export interface RowTintOptions {
  /** 十六进制透明度后缀，各表视觉不同：会话表 3d / 目标表 2c / 截图 30 */
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
