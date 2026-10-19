/**
 * 节点图标的公共模型。
 *
 * 图标来源有两类，都统一成 { d, vb }：
 *  - 品牌图标（Windows/Linux/macOS）：取自公开图标集（simple-icons / MDI / FontAwesome brands），
 *    自己手绘的品牌图形既不标准、细节也经不起放大，看起来"奇怪"；
 *  - 项目自带图标集（src/layout-vshell/vs-icons.ts，FontAwesome 同源）：监听器/隧道/目标这类
 *    "协议与端口"没有品牌图标，用现成的通用图标更清晰，也避免再造一套手绘图形。
 *
 * 坐标系：各家 viewBox 不同（24×24 / 448×512 …），统一用 glyphTransform 映射到
 * 节点圆内的方形区域（边长 2*half），所以图形数据保持原样、不做有损重写。
 */
export interface Glyph {
  /** path 数据（多段子路径用空格连接） */
  d: string
  /** 原始坐标系，如 '0 0 24 24' */
  vb: string
  zh: string
  en: string
}

export interface GlyphFit {
  tx: number
  ty: number
  s: number
}

/**
 * glyphFit 把图标坐标系缩放平移到"以 (0,0) 为中心、半边长 half"的方框里。
 * 按 max(宽,高) 缩放，宽高比不同的图标不会变形（窄的左右留白）。
 */
export function glyphFit(vb: string, half: number): GlyphFit {
  const [minX, minY, w, h] = vb.split(/\s+/).map(Number)
  const s = (half * 2) / Math.max(w || 1, h || 1)
  return { tx: -s * (minX + w / 2), ty: -s * (minY + h / 2), s }
}

/** 与 glyphFit 等价的 SVG transform 字符串（画布与导出都用它） */
export function glyphTransform(vb: string, half: number): string {
  const { tx, ty, s } = glyphFit(vb, half)
  return `translate(${tx.toFixed(4)} ${ty.toFixed(4)}) scale(${s.toFixed(6)})`
}
