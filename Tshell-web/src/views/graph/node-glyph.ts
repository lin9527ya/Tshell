/**
 * 监听器与隧道节点的图标：按**协议**取。
 *
 * 这两类节点没有品牌图标可言（tcp/websocket/socks5 不是某个厂商的东西），
 * 所以用项目自带图标集（src/layout-vshell/vs-icons.ts，FontAwesome 同源）里的通用图标，
 * 而不是继续手绘 —— 手绘的插头/箭头/圆柱在小尺寸下形状"不像那个东西"。
 *
 * 监听器类型来自后端（pkg/api/listener.go）：tcp/kcp/http/https/websocket/dns/oss
 * 以及 *_forward 变体（正向连接，同协议同图标）。
 */
import { type Glyph } from './glyph'
import { vsIconPath } from '@/layout-vshell/vs-icons'
import type { GraphNode } from './layout'
import { OS_GLYPHS, osGlyphKey } from './os-glyph'
import { TARGET_GLYPHS, targetGlyphKey } from './target-glyph'

function fromIconSet(name: string, zh: string, en: string): Glyph {
  const p = vsIconPath(name)
  return { d: p?.d || '', vb: p?.vb || '0 0 16 16', zh, en }
}

/** fa-solid:bolt（项目图标集里没有闪电，单独取一个同源的） */
const BOLT: Glyph = {
  vb: '0 0 320 512',
  d: 'M296 160H180.6l42.6-129.8C227.2 15 215.7 0 200 0H56C44 0 33.8 8.9 32.2 20.8l-32 240C-1.7 275.2 9.5 288 24 288h118.7L96.6 482.5c-3.6 15.2 8 29.5 23.3 29.5c8.4 0 16.4-4.4 20.8-12l176-304c9.3-15.9-2.2-36-20.7-36',
  zh: 'KCP',
  en: 'KCP',
}

export type ListenerGlyphKey = 'tcp' | 'kcp' | 'websocket' | 'http' | 'https' | 'dns' | 'oss' | 'other'

export const LISTENER_GLYPHS: Record<ListenerGlyphKey, Glyph> = {
  tcp: fromIconSet('plug', 'TCP', 'TCP'),
  kcp: BOLT,
  websocket: fromIconSet('stream', 'WebSocket', 'WebSocket'),
  http: fromIconSet('globe', 'HTTP', 'HTTP'),
  https: fromIconSet('lock', 'HTTPS', 'HTTPS'),
  dns: fromIconSet('network-wired', 'DNS', 'DNS'),
  oss: fromIconSet('layer-group', 'OSS', 'OSS'),
  other: fromIconSet('broadcast-tower', '其它', 'Other'),
}

/** 监听器类型 → 图标。正向连接（*_forward）与对应协议同图标 */
export function listenerGlyphKey(type?: string): ListenerGlyphKey {
  const v = (type || '').trim().toLowerCase().replace(/_forward$/, '')
  if (!v) return 'other'
  if (v === 'tcp') return 'tcp'
  if (v === 'kcp') return 'kcp'
  if (v === 'websocket' || v === 'ws') return 'websocket'
  if (v === 'http') return 'http'
  if (v === 'https') return 'https'
  if (v === 'dns') return 'dns'
  if (v === 'oss') return 'oss'
  return 'other'
}

export type TunnelGlyphKey = 'socks5' | 'http' | 'port' | 'other'

export const TUNNEL_GLYPHS: Record<TunnelGlyphKey, Glyph> = {
  // SOCKS5 是"替客户端转发流量"，用交叉箭头（改道）表达
  socks5: fromIconSet('random', 'SOCKS5', 'SOCKS5'),
  http: fromIconSet('globe', 'HTTP', 'HTTP'),
  port: fromIconSet('plug', '端口转发', 'Port fwd'),
  other: fromIconSet('map-signs', '其它', 'Other'),
}

/** 隧道协议 → 图标（tcp/udp 都是端口转发） */
export function tunnelGlyphKey(protocol?: string): TunnelGlyphKey {
  const v = (protocol || '').trim().toLowerCase()
  if (v === 'socks5') return 'socks5'
  if (v === 'http') return 'http'
  if (v === 'tcp' || v === 'udp') return 'port'
  return 'other'
}

/**
 * nodeGlyph 节点 → 图标。返回 null 表示这类节点不画图标。
 *
 * 服务端/监听器/客户端/隧道/目标五类里：
 *  - 客户端看**系统**（os 元数据），目标看**端口**（推测），监听器/隧道看**协议**；
 *  - 服务端是唯一的"自己"，不画 —— 画了反而像还有别的同类节点。
 */
export function nodeGlyph(nd: GraphNode): Glyph | null {
  switch (nd.type) {
    case 'client':
      return OS_GLYPHS[osGlyphKey(nd.meta?.os)]
    case 'target':
      return TARGET_GLYPHS[targetGlyphKey(nd.label)]
    case 'listener':
      return LISTENER_GLYPHS[listenerGlyphKey(nd.label)]
    case 'tunnel':
      return TUNNEL_GLYPHS[tunnelGlyphKey(nd.label)]
    default:
      return null
  }
}
