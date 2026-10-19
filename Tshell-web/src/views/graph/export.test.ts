import { describe, expect, it } from 'vitest'
import {
  buildExportSvg,
  DEFAULT_EXPORT_THEME,
  exportFilename,
  EXPORT_PAD,
} from './export'
import { graphBounds } from './layout'

const NS = 'http://www.w3.org/2000/svg'

function makeSvg(): SVGSVGElement {
  const svg = document.createElementNS(NS, 'svg')
  svg.setAttribute('class', 'pg-svg')
  svg.setAttribute('style', 'width:100%;height:100%')
  const g = document.createElementNS(NS, 'g')
  g.setAttribute('class', 'pg-content')
  g.setAttribute('transform', 'translate(10 20) scale(0.5)')
  const c = document.createElementNS(NS, 'circle')
  c.setAttribute('r', '9')
  g.appendChild(c)
  svg.appendChild(g)
  return svg
}

const NODES = [
  { x: 90, y: 48, label: 'Tshell Server', sub_label: '' },
  { x: 300, y: 48, label: 'WEB-01', sub_label: '10.0.0.9' },
]

describe('buildExportSvg', () => {
  it('尺寸 = 包围盒 + 四周留白', () => {
    const b = graphBounds(NODES, 0)
    const out = buildExportSvg(makeSvg(), b)
    expect(out.width).toBe(Math.round(b.maxX - b.minX + EXPORT_PAD * 2))
    expect(out.height).toBe(Math.round(b.maxY - b.minY + EXPORT_PAD * 2))
    expect(out.text).toContain(`viewBox="0 0 ${out.width} ${out.height}"`)
    expect(out.text).toContain(`width="${out.width}"`)
  })

  it('把画布的平移缩放换成按包围盒摆正（导出与当前视口无关）', () => {
    const b = graphBounds(NODES, 0)
    const { text } = buildExportSvg(makeSvg(), b)
    expect(text).toContain(`translate(${(EXPORT_PAD - b.minX).toFixed(2)} ${(EXPORT_PAD - b.minY).toFixed(2)})`)
    expect(text).not.toContain('scale(0.5)')
  })

  it('不改动画布里的原节点（克隆后再改）', () => {
    const svg = makeSvg()
    buildExportSvg(svg, graphBounds(NODES, 0))
    expect(svg.querySelector('g.pg-content')!.getAttribute('transform')).toBe(
      'translate(10 20) scale(0.5)',
    )
  })

  it('产出可独立打开的 SVG：XML 头 + 命名空间 + 背景 + 网格', () => {
    const { text } = buildExportSvg(makeSvg(), graphBounds(NODES, 0))
    expect(text.startsWith('<?xml version="1.0" encoding="UTF-8"?>')).toBe(true)
    expect(text).toContain('xmlns="http://www.w3.org/2000/svg"')
    expect(text).toContain(DEFAULT_EXPORT_THEME.bg)
    expect(text).toContain('url(#pg-grid)')
    expect(text).toContain('<style')
  })

  it('主题色可覆盖（暗色主题导出不该是白底黑字）', () => {
    const { text } = buildExportSvg(makeSvg(), graphBounds(NODES, 0), {
      theme: { bg: '#151515', text: '#c9d1d9' },
    })
    expect(text).toContain('#151515')
    expect(text).toContain('#c9d1d9')
  })

  it('导出样式覆盖画布用到的每个类（漏一个导出图就走样）', () => {
    const { text } = buildExportSvg(makeSvg(), graphBounds(NODES, 0))
    for (const cls of [
      'pg-edge',
      'pg-edge-active',
      'pg-edge-offline',
      'pg-edge-label',
      'pg-node-halo',
      'pg-node-label',
      'pg-node-sub',
      'pg-node-os',
      'pg-node-offline',
      'pg-node-selected',
    ]) {
      expect(text, `导出样式缺少 .${cls}`).toContain(`.${cls}`)
    }
  })
})

describe('exportFilename', () => {
  it('带时间戳，扩展名跟着格式走', () => {
    const d = new Date(2026, 9, 19, 14, 30, 12)
    expect(exportFilename('svg', d)).toBe('tshell-topology-20261019-143012.svg')
    expect(exportFilename('png', d)).toBe('tshell-topology-20261019-143012.png')
  })
})
