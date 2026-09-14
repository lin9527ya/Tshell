<template>
  <div class="pivot-graph-container cs-font-ui" @mousedown="onContainerMouseDown">
    <!-- Graph Control Toolbar -->
    <div class="pg-toolbar">
      <span class="pg-title">{{ isZh ? '拓扑网络图' : 'Pivot Graph' }}</span>
      <div class="pg-btn-group">
        <button class="pg-btn" :title="isZh ? '放大' : 'Zoom In'" @click="zoomIn">＋</button>
        <button class="pg-btn" :title="isZh ? '缩小' : 'Zoom Out'" @click="zoomOut">－</button>
        <button class="pg-btn" :title="isZh ? '适应画布' : 'Reset View'" @click="resetView">⤢ {{ isZh ? '居中' : 'Fit' }}</button>
        <button class="pg-btn" :title="isZh ? '自动排列布局' : 'Auto Layout'" @click="autoLayout">⚡ {{ isZh ? '自动排列' : 'Arrange' }}</button>
      </div>

      <div class="pg-spacer" />

      <label class="pg-toggle">
        <input type="checkbox" v-model="showOffline" />
        <span>{{ isZh ? '显示离线节点' : 'Show Offline' }}</span>
      </label>

      <span class="pg-stats">
        {{ activeBeacons.length }} {{ isZh ? '个活动节点' : 'active nodes' }}
      </span>
    </div>

    <!-- SVG Graph Surface -->
    <div
      ref="canvasRef"
      class="pg-canvas"
      :class="{ dragging: isPanning }"
      @wheel.prevent="onWheel"
      @mousedown="startPan"
    >
      <svg class="pg-svg" :style="transformStyle">
        <!-- Arrow Marker Definition -->
        <defs>
          <marker
            id="arrow"
            viewBox="0 0 10 10"
            refX="22"
            refY="5"
            markerWidth="6"
            markerHeight="6"
            orient="auto-start-reverse"
          >
            <path d="M 0 0 L 10 5 L 0 10 z" fill="var(--cs-active)" />
          </marker>
          <linearGradient id="linkGrad" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" stop-color="var(--cs-active)" stop-opacity="0.8" />
            <stop offset="100%" stop-color="var(--cs-active)" stop-opacity="0.2" />
          </linearGradient>
        </defs>

        <!-- Edges / Links -->
        <g class="pg-links">
          <g v-for="link in computedLinks" :key="link.id">
            <path
              :d="link.path"
              class="pg-link-line"
              :class="{ active: link.active }"
              marker-end="url(#arrow)"
            />
          </g>
        </g>

        <!-- Nodes -->
        <g class="pg-nodes">
          <g
            v-for="node in computedNodes"
            :key="node.id"
            :transform="`translate(${node.x}, ${node.y})`"
            class="pg-node-g"
            :class="{
              selected: selectedNodeId === node.id,
              offline: node.online === false,
              server: node.type === 'server',
              listener: node.type === 'listener',
              beacon: node.type === 'beacon',
            }"
            @mousedown.stop="startDragNode($event, node)"
            @dblclick.stop="onNodeDblClick(node)"
            @contextmenu.prevent.stop="onNodeContextMenu($event, node)"
          >
            <!-- Node Background Card -->
            <rect
              :x="-node.w / 2"
              :y="-node.h / 2"
              :width="node.w"
              :height="node.h"
              rx="4"
              class="node-card"
              :style="node.colorStyle"
            />

            <!-- Top Status Indicator -->
            <circle
              v-if="node.type === 'beacon'"
              :cx="-node.w / 2 + 10"
              :cy="-node.h / 2 + 10"
              r="4"
              class="node-status-dot"
              :class="{ online: node.online }"
            />

            <!-- Node Icon -->
            <!-- Node Icon -->
            <g :transform="`translate(${-node.w / 2 + 12}, ${-node.h / 2 + 12})`">
              <!-- Server Icon -->
              <path
                v-if="node.type === 'server'"
                d="M2 4h16v4H2z M2 10h16v4H2z M2 16h16v4H2z"
                fill="none"
                stroke="var(--cs-text-bright)"
                stroke-width="1.5"
              />
              <!-- Listener Icon -->
              <path
                v-else-if="node.type === 'listener'"
                d="M2 14a8 8 0 0 1 16 0 M5 14a5 5 0 0 1 10 0 M10 9v9"
                fill="none"
                stroke="var(--cs-warning)"
                stroke-width="1.5"
              />
              <!-- OS Windows Icon -->
              <g v-else-if="node.os === 'windows'">
                <path d="M1 3.2 L7.8 2.2 L7.8 8.8 L1 8.8 Z" fill="#00adef" />
                <path d="M8.8 2.1 L17 0.9 L17 8.8 L8.8 8.8 Z" fill="#00adef" />
                <path d="M1 9.8 L7.8 9.8 L7.8 16.4 L1 15.4 Z" fill="#00adef" />
                <path d="M8.8 9.8 L17 9.8 L17 17.7 L8.8 16.5 Z" fill="#00adef" />
              </g>
              <!-- OS Linux Tux Penguin Icon -->
              <g v-else-if="node.os === 'linux'">
                <!-- Tux Penguin Body -->
                <path d="M9 1.5 C6.5 1.5, 5 3.5, 5 6.5 C5 8, 4.5 9.5, 3.5 11 C2.8 12.2, 2.8 13.8, 3.8 14.8 C4.8 15.8, 6.5 16.2, 9 16.2 C11.5 16.2, 13.2 15.8, 14.2 14.8 C15.2 13.8, 15.2 12.2, 14.5 11 C13.5 9.5, 13 8, 13 6.5 C13 3.5, 11.5 1.5, 9 1.5 Z" fill="#222" stroke="#111" stroke-width="0.5" />
                <!-- White Belly -->
                <ellipse cx="9" cy="11.2" rx="3.3" ry="3.8" fill="#ffffff" />
                <!-- White Eye Patches -->
                <ellipse cx="7.3" cy="5" rx="1" ry="1.4" fill="#ffffff" />
                <ellipse cx="10.7" cy="5" rx="1" ry="1.4" fill="#ffffff" />
                <circle cx="7.4" cy="5.2" r="0.5" fill="#000000" />
                <circle cx="10.6" cy="5.2" r="0.5" fill="#000000" />
                <!-- Yellow Beak -->
                <polygon points="7.2,6.5 10.8,6.5 9,8.2" fill="#ffb300" stroke="#f57c00" stroke-width="0.3" />
                <!-- Yellow Feet -->
                <ellipse cx="6.2" cy="16.2" rx="2" ry="0.9" fill="#ffb300" />
                <ellipse cx="11.8" cy="16.2" rx="2" ry="0.9" fill="#ffb300" />
              </g>
              <!-- OS macOS Apple Icon -->
              <g v-else-if="node.os === 'darwin'">
                <path d="M11.5 2 C12 1.2 12.5 0.5 12.2 0 C11.6 0.1 10.8 0.6 10.3 1.3 C9.8 1.8 9.4 2.6 9.6 3 C10.4 3 11 2.5 11.5 2 Z M13.8 9 C13.8 7.2 15.2 6.3 15.3 6.2 C14.4 5 13.1 4.8 12.6 4.8 C11.4 4.7 10.3 5.5 9.7 5.5 C9.1 5.5 8.2 4.8 7.3 4.8 C6.1 4.8 5 5.5 4.3 6.6 C3 8.8 4 12.1 5.3 14 C5.9 14.8 6.6 15.8 7.7 15.8 C8.7 15.8 9 15.2 10.2 15.2 C11.3 15.2 11.7 15.8 12.7 15.8 C13.8 15.8 14.4 14.9 15.1 14 C15.9 12.9 16.2 11.8 16.2 11.7 C16.1 11.7 13.8 10.8 13.8 9 Z" fill="var(--cs-text-bright)" />
              </g>
              <!-- Generic / Monitor Icon -->
              <g v-else>
                <rect x="2" y="3" width="14" height="10" rx="1.5" fill="none" stroke="var(--cs-text-bright)" stroke-width="1.2" />
                <path d="M6 16h6M9 13v3" stroke="var(--cs-text-bright)" stroke-width="1.2" stroke-linecap="round" />
              </g>
            </g>

            <!-- Node Text Content -->
            <text
              :x="-node.w / 2 + 36"
              :y="-node.h / 2 + 18"
              class="node-title"
            >
              {{ node.title }}
            </text>

            <text
              :x="-node.w / 2 + 36"
              :y="-node.h / 2 + 32"
              class="node-sub"
            >
              {{ node.sub }}
            </text>

            <text
              v-if="node.ip"
              :x="-node.w / 2 + 36"
              :y="-node.h / 2 + 45"
              class="node-ip"
            >
              {{ node.ip }}
            </text>
          </g>
        </g>
      </svg>
    </div>

    <!-- Full Cobalt Strike Beacon Context Menu -->
    <CsBeaconContextMenu
      v-model:visible="ctxMenu.visible"
      :x="ctxMenu.x"
      :y="ctxMenu.y"
      :client="ctxMenu.node?.raw || null"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useClientsStore, type ClientData } from '@/stores/clients'
import { useWorkspaceStore } from '@/stores/workspace'
import { useEventsStore } from '@/stores/events'
import { useI18nStore } from '@/stores/i18n'
import { detectClientOs } from '@/utils/clientUtils'
import ClientAPI from '@/api/clients'
import ListenerAPI from '@/api/listener'
import CsBeaconContextMenu from './ContextMenu/CsBeaconContextMenu.vue'

interface GraphNode {
  id: string
  type: 'server' | 'listener' | 'beacon'
  title: string
  sub: string
  ip?: string
  os?: 'windows' | 'linux' | 'darwin' | 'unknown'
  online?: boolean
  x: number
  y: number
  w: number
  h: number
  uid?: string
  raw?: ClientData
  colorStyle?: any
}

const clientsStore = useClientsStore()
const workspace = useWorkspaceStore()
const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const canvasRef = ref<HTMLElement | null>(null)
const showOffline = ref(true)
const selectedNodeId = ref('')

// Pan & Zoom
const pan = reactive({ x: 120, y: 80 })
const zoom = ref(1)
const isPanning = ref(false)
let panStart = { x: 0, y: 0 }

// Node dragging
const draggingNode = ref<GraphNode | null>(null)
let dragStart = { x: 0, y: 0, nodeX: 0, nodeY: 0 }

// Nodes positions cache
const nodePositions = reactive<Record<string, { x: number; y: number }>>({})

// Context Menu
const ctxMenu = ref({ visible: false, x: 0, y: 0, node: null as GraphNode | null })
const ctxNodeTitle = computed(() => ctxMenu.value.node?.title || 'Node Target')

const activeBeacons = computed(() => {
  return computedNodes.value.filter((n) => n.type === 'beacon' && n.online === true)
})

// ===== 后端 /graph/data 数据源与 Listener 列表 =====
const graphData = ref<{ nodes: any[]; edges: any[] } | null>(null)
const rawListeners = ref<any[]>([])
const CLIENT_STATE_EVENTS = ['client_online', 'client_offline', 'client_new']

// 解析 Listener 真实外网连接地址（替换 0.0.0.0 为有效回连 IP / 端口，优先匹配活跃在线监听器）
function resolveListenerConnectAddr(rawSubLabel?: string, label?: string, meta?: any): string {
  // 1. 如果 meta 中已有明确的外网连接地址且非 0.0.0.0
  const metaConnect = meta?.connectAddress || meta?.ConnectAddress || meta?.connect_address
  if (metaConnect && !metaConnect.includes('0.0.0.0')) {
    return metaConnect
  }

  const cleanLabel = (label || '').toLowerCase().trim()
  const rawSub = (rawSubLabel || '').toLowerCase().trim()
  const portMatch = (rawSub + ' ' + cleanLabel).match(/:(\d+)/)
  const port = portMatch ? portMatch[1] : ''

  // 2. 从 listener API 列表中查找对应的 Listener（优先匹配 Status === 1 的活动监听器）
  const activeListeners = rawListeners.value.filter((l: any) => l.Status === 1)
  const allListeners = rawListeners.value

  // a. 先在活动监听器中按端口匹配
  let found = port ? activeListeners.find((l: any) => {
    return (l.ConnectAddress && l.ConnectAddress.includes(`:${port}`)) ||
           (l.ListenAddress && l.ListenAddress.includes(`:${port}`))
  }) : null

  // b. 再在活动监听器中按类型/名称匹配
  if (!found && cleanLabel) {
    found = activeListeners.find((l: any) => {
      const lType = (l.Type || '').toLowerCase()
      const lName = (l.name || l.Name || '').toLowerCase()
      return cleanLabel.includes(lType) || (lName && cleanLabel.includes(lName))
    })
  }

  // c. 如果有活动监听器且未找到具体匹配，优先取第一个活动监听器（避免展示已停用的旧监听器）
  if (!found && activeListeners.length > 0) {
    found = activeListeners[0]
  }

  // d. 若没有任何活动监听器，再在全部监听器中按端口或名称降级查找
  if (!found) {
    if (port) {
      found = allListeners.find((l: any) => {
        return (l.ConnectAddress && l.ConnectAddress.includes(`:${port}`)) ||
               (l.ListenAddress && l.ListenAddress.includes(`:${port}`))
      })
    }
    if (!found && cleanLabel) {
      found = allListeners.find((l: any) => {
        const lType = (l.Type || '').toLowerCase()
        const lName = (l.name || l.Name || '').toLowerCase()
        return cleanLabel.includes(lType) || (lName && cleanLabel.includes(lName))
      })
    }
  }

  if (found) {
    if (found.ConnectAddress && !found.ConnectAddress.includes('0.0.0.0')) {
      return found.ConnectAddress
    }
    if (found.bindto && !found.bindto.includes('0.0.0.0')) {
      return found.bindto
    }
    if (found.ListenAddress) {
      let s = found.ListenAddress.trim()
      if (s.startsWith(':')) s = '0.0.0.0' + s
      const host = window.location.hostname !== 'localhost' && window.location.hostname !== '127.0.0.1'
        ? window.location.hostname
        : '127.0.0.1'
      return s.replace(/0\.0\.0\.0/g, host)
    }
  }

  // 3. 处理包含 0.0.0.0 或以 : 开头的地址
  let s = (rawSubLabel || '').trim()
  if (s.startsWith(':')) s = '0.0.0.0' + s

  const activeClient = clientsStore.clientsList.find(
    (c: ClientData) => c.ExternalIP && c.ExternalIP !== '127.0.0.1' && c.ExternalIP !== '0.0.0.0',
  )
  const host =
    activeClient?.ExternalIP ||
    (typeof window !== 'undefined' && window.location.hostname !== 'localhost' && window.location.hostname !== '127.0.0.1'
      ? window.location.hostname
      : '127.0.0.1')

  if (s && s.includes('0.0.0.0')) {
    return s.replace(/0\.0\.0\.0/g, host)
  }

  if (s && s !== 'Active Pivot') {
    return s
  }

  return `${host}:${port || '8080'}`
}

// 后端节点 id → 前端节点 id（保持 nodePositions 拖拽缓存键一致）
function backendIdToFrontend(bid: string): string {
  if (bid === 'server') return 'server'
  if (bid.startsWith('listener_')) return `listener:${bid}`
  if (bid.startsWith('client_')) return `beacon:${bid.slice(7)}`
  return ''
}

function mapOs(archOrClient: any, computer?: string): GraphNode['os'] {
  if (typeof archOrClient === 'object' && archOrClient !== null) {
    return detectClientOs(archOrClient)
  }
  return detectClientOs({ Arch: archOrClient, Computer: computer })
}

function defaultPos(id: string, idx: number) {
  if (id === 'server') return { x: 100, y: 220 }
  if (id.startsWith('listener:')) return { x: 340, y: 120 + idx * 140 }
  return { x: 600 + (idx % 2) * 190, y: 80 + idx * 90 }
}

// 检查 Listener 是否处于活跃在线状态 (Status === 1)
function isListenerActive(n: any): boolean {
  if (rawListeners.value && rawListeners.value.length > 0) {
    const rawId = String(n.id || n.Id || n.ID || '').replace(/^listener_/, '').replace(/^listener:/, '')
    const rawLabel = (n.label || n.title || '').toLowerCase().trim()
    const rawSub = (n.sub_label || n.sub || '').toLowerCase().trim()
    const portMatch = (rawSub + ' ' + rawLabel).match(/:(\d+)/)
    const port = portMatch ? portMatch[1] : ''

    const matched = rawListeners.value.find((l: any) => {
      const lId = String(l.Id ?? l.ID ?? l.id ?? '')
      const lType = (l.Type || '').toLowerCase()
      const lName = (l.name || l.Name || '').toLowerCase()
      if (rawId && lId === rawId) return true
      if (port && ((l.ConnectAddress && l.ConnectAddress.includes(`:${port}`)) || (l.ListenAddress && l.ListenAddress.includes(`:${port}`)))) return true
      if (rawLabel && (lType.includes(rawLabel) || rawLabel.includes(lType) || (lName && (lName.includes(rawLabel) || rawLabel.includes(lName))))) return true
      return false
    })

    if (matched) {
      return matched.Status === 1
    }
  }
  if (n.meta?.Status !== undefined) return n.meta.Status === 1 || n.meta.Status === '1'
  if (n.meta?.status !== undefined) return n.meta.status === 1 || n.meta.status === '1' || n.meta.status === 'active'
  if (n.status !== undefined) return n.status === 1 || n.status === '1' || n.status === 'active'
  if (n.online !== undefined) return n.online === '1' || n.online === true
  return false
}

function mapBackendNode(n: any, idx: number): GraphNode | null {
  if (n.type === 'server') {
    return {
      id: 'server',
      type: 'server',
      title: n.label || 'TeamServer',
      sub: 'Tshell Controller',
      ip: window.location.hostname,
      w: 160,
      h: 56,
      ...(nodePositions['server'] || defaultPos('server', idx)),
    }
  }
  if (n.type === 'listener') {
    // 过滤：仅显示在线运行中的监听器
    if (!isListenerActive(n)) {
      return null
    }
    const id = `listener:${n.id}`
    const addr = resolveListenerConnectAddr(n.sub_label, n.label, n.meta)
    return {
      id,
      type: 'listener',
      title: `Listener: ${n.label || 'TCP'}`,
      sub: addr,
      online: true,
      w: 180,
      h: 56,
      ...(nodePositions[id] || defaultPos(id, idx)),
    }
  }
  if (n.type === 'client') {
    const uid = n.meta?.uid || ''
    const id = `beacon:${uid}`
    const c = clientsStore.clientsList.find((cc: ClientData) => cc.Uid === uid)
    const isClientOnline = c ? c.Online === '1' : (n.online === '1' || n.online === true)
    
    // 过滤：如果不显示离线节点，且当前客户端已离线，则不渲染
    if (!showOffline.value && !isClientOnline) {
      return null
    }

    const color = uid ? workspace.sessionColors[uid] : undefined
    const colorStyle = color ? { stroke: color, strokeWidth: '2px', fill: color + '15' } : {}
    return {
      id,
      type: 'beacon',
      title: n.label || uid.slice(0, 8),
      sub: `${n.meta?.username || 'user'} (PID ${c?.Pid || '?'})`,
      ip: n.sub_label || c?.InternalIP || '-',
      os: mapOs(c || { Arch: n.meta?.os || '', Computer: n.meta?.computer || '' }),
      online: isClientOnline,
      w: 180,
      h: 60,
      uid,
      raw: c || undefined,
      colorStyle,
      ...(nodePositions[id] || defaultPos(id, idx)),
    }
  }
  return null
}

const computedNodes = computed<GraphNode[]>(() => {
  if (graphData.value && graphData.value.nodes && graphData.value.nodes.length > 0) {
    const mapped = graphData.value.nodes
      .map(mapBackendNode)
      .filter((n): n is GraphNode => n !== null)

    const activeListenerFrontendIds = new Set(
      mapped.filter((n) => n.type === 'listener').map((n) => n.id)
    )

    // 仅保留关联到活跃在线监听器的客户端（或无明确监听器但在线的客户端）
    return mapped.filter((n) => {
      if (n.type === 'server' || n.type === 'listener') return true
      if (n.type === 'beacon') {
        if (!showOffline.value && !n.online) return false
        // 如果系统中有在线监听器，确认该 beacon 不属于已关闭的独立监听器
        if (activeListenerFrontendIds.size === 0) return false
        return true
      }
      return true
    })
  }

  // ---- 基于 clientsStore 与 rawListeners 动态构建拓扑 ----
  const result: GraphNode[] = []

  // 1. TeamServer Node (Root)
  const serverPos = nodePositions['server'] || { x: 100, y: 220 }
  result.push({
    id: 'server',
    type: 'server',
    title: 'TeamServer',
    sub: 'Tshell Controller',
    ip: window.location.hostname,
    x: serverPos.x,
    y: serverPos.y,
    w: 160,
    h: 56,
  })

  // 2. Listener Nodes: 仅渲染真实在线活动监听器 (Status === 1)
  const activeListeners = rawListeners.value.filter((l: any) => l.Status === 1)

  activeListeners.forEach((l: any, idx: number) => {
    const lId = `listener:${l.Id ?? l.ID ?? l.id ?? l.Type ?? idx}`
    const lPos = nodePositions[lId] || { x: 340, y: 120 + idx * 140 }
    const addr = resolveListenerConnectAddr(l.ListenAddress, l.Type, l)
    result.push({
      id: lId,
      type: 'listener',
      title: `Listener: ${(l.Type || l.name || 'tcp').toUpperCase()}`,
      sub: addr,
      online: true,
      x: lPos.x,
      y: lPos.y,
      w: 180,
      h: 56,
    })
  })

  // 3. Beacon Nodes: 根据 showOffline 过滤在线状态，且仅在有活动监听器时渲染对应客户端
  if (activeListeners.length > 0) {
    const candidateList = showOffline.value 
      ? clientsStore.clientsList 
      : clientsStore.clientsList.filter((c) => c.Online === '1')

    candidateList.forEach((b, idx) => {
      // 检查 Beacon 是否归属于已开启的活动监听器
      const bListener = (b.Listener || '').toLowerCase().trim()
      const bAddress = (b.Address || '').toLowerCase().trim()
      const portMatch = (bAddress + ' ' + bListener).match(/:(\d+)/)
      const port = portMatch ? portMatch[1] : ''

      const matchedListener = activeListeners.find((l: any) => {
        const lId = String(l.Id ?? l.ID ?? l.id ?? '')
        const lType = (l.Type || '').toLowerCase()
        const lName = (l.name || l.Name || '').toLowerCase()
        if (lId && lId === bListener) return true
        if (port && ((l.ConnectAddress && l.ConnectAddress.includes(`:${port}`)) || (l.ListenAddress && l.ListenAddress.includes(`:${port}`)))) return true
        if (bListener && (lType.includes(bListener) || bListener.includes(lType) || (lName && (lName.includes(bListener) || bListener.includes(lName))))) return true
        return false
      })

      // 如果 Beacon 明确指定了监听器但该监听器已离线/停用，则不在此活动拓扑中显示
      if (bListener && !matchedListener) {
        return
      }

      const bId = `beacon:${b.Uid}`
      const pos = nodePositions[bId] || {
        x: 600 + (idx % 2) * 190,
        y: 80 + idx * 90,
      }

      const color = workspace.sessionColors[b.Uid]
      const colorStyle = color ? { stroke: color, strokeWidth: '2px', fill: color + '15' } : {}

      result.push({
        id: bId,
        type: 'beacon',
        title: b.Computer || b.Username || b.Uid.slice(0, 8),
        sub: `${b.Username || 'user'} (PID ${b.Pid || '?'})`,
        ip: b.ExternalIP || b.InternalIP || '-',
        os: mapOs(b.Arch || '', b.Computer),
        online: b.Online === '1',
        x: pos.x,
        y: pos.y,
        w: 180,
        h: 60,
        uid: b.Uid,
        raw: b,
        colorStyle,
      })
    })
  }

  return result
})

const computedLinks = computed(() => {
  const links: { id: string; path: string; active: boolean }[] = []
  const validNodeIds = new Set(computedNodes.value.map((n) => n.id))

  if (graphData.value && graphData.value.edges && graphData.value.edges.length > 0) {
    const nodeById = new Map(computedNodes.value.map((n) => [n.id, n]))
    for (const e of graphData.value.edges) {
      const sf = backendIdToFrontend(e.source)
      const tf = backendIdToFrontend(e.target)
      if (!validNodeIds.has(sf) || !validNodeIds.has(tf)) continue
      const sn = nodeById.get(sf)
      const tn = nodeById.get(tf)
      if (!sn || !tn) continue
      const isClientEdge = String(e.source).startsWith('client_')
      const from = isClientEdge ? tn : sn
      const to = isClientEdge ? sn : tn
      links.push({
        id: `link:${sf}->${tf}`,
        path: createCurvedPath(from.x + from.w / 2, from.y, to.x - to.w / 2, to.y),
        active: to.type === 'beacon' ? (to.online === true) : true,
      })
    }
    return links
  }

  // ---- 手拼 server → listener → beacon 连线 ----
  const serverNode = computedNodes.value.find((n) => n.id === 'server')
  const listenerNodes = computedNodes.value.filter((n) => n.type === 'listener')

  if (serverNode) {
    listenerNodes.forEach((lNode) => {
      links.push({
        id: `link:server->${lNode.id}`,
        path: createCurvedPath(serverNode.x + serverNode.w / 2, serverNode.y, lNode.x - lNode.w / 2, lNode.y),
        active: lNode.online !== false,
      })
    })
  }

  // 监听器 -> Beacon 连线
  computedNodes.value.filter((n) => n.type === 'beacon').forEach((bNode) => {
    const b = bNode.raw
    const bListener = (b?.Listener || '').toLowerCase().trim()
    const bAddress = (b?.Address || '').toLowerCase().trim()
    const portMatch = (bAddress + ' ' + bListener).match(/:(\d+)/)
    const port = portMatch ? portMatch[1] : ''

    // 1. 优先在在线监听器中按端口匹配
    let targetListener = port
      ? listenerNodes.find((l) => l.sub?.includes(`:${port}`) || l.title?.includes(`:${port}`))
      : undefined

    // 2. 其次在在线监听器中按类型/名称/ID匹配
    if (!targetListener && bListener) {
      targetListener = listenerNodes.find((l) => {
        const titleLower = l.title.toLowerCase()
        const idLower = l.id.toLowerCase()
        return titleLower.includes(bListener) || idLower.includes(bListener) || l.id === `listener:${bListener}`
      })
    }

    // 3. 兜底取第一个在线监听器
    if (!targetListener) {
      targetListener = listenerNodes[0]
    }

    if (targetListener) {
      links.push({
        id: `link:${targetListener.id}->${bNode.id}`,
        path: createCurvedPath(targetListener.x + targetListener.w / 2, targetListener.y, bNode.x - bNode.w / 2, bNode.y),
        active: bNode.online === true,
      })
    }
  })

  return links
})

function createCurvedPath(x1: number, y1: number, x2: number, y2: number) {
  const dx = Math.max(40, (x2 - x1) * 0.5)
  return `M ${x1} ${y1} C ${x1 + dx} ${y1}, ${x2 - dx} ${y2}, ${x2} ${y2}`
}

const transformStyle = computed(() => {
  return {
    transform: `translate(${pan.x}px, ${pan.y}px) scale(${zoom.value})`,
    transformOrigin: '0 0',
  }
})

function zoomIn() {
  zoom.value = Math.min(2.5, zoom.value * 1.2)
}
function zoomOut() {
  zoom.value = Math.max(0.4, zoom.value / 1.2)
}
function resetView() {
  pan.x = 80
  pan.y = 60
  zoom.value = 1
}

function autoLayout() {
  // Server at center left
  nodePositions['server'] = { x: 100, y: 220 }

  // Listeners column
  const listeners = computedNodes.value.filter((n) => n.type === 'listener')
  listeners.forEach((l, i) => {
    nodePositions[l.id] = { x: 340, y: 100 + i * 160 }
  })

  // Beacons grid
  const beacons = computedNodes.value.filter((n) => n.type === 'beacon')
  beacons.forEach((b, i) => {
    const col = Math.floor(i / 4)
    const row = i % 4
    nodePositions[b.id] = {
      x: 600 + col * 220,
      y: 80 + row * 90,
    }
  })
}

function onWheel(e: WheelEvent) {
  const factor = e.deltaY < 0 ? 1.1 : 0.9
  zoom.value = Math.max(0.4, Math.min(2.5, zoom.value * factor))
}

function startPan(e: MouseEvent) {
  if ((e.target as HTMLElement).closest('.pg-node-g')) return
  isPanning.value = true
  panStart = { x: e.clientX - pan.x, y: e.clientY - pan.y }
  closeCtx()
}

function startDragNode(e: MouseEvent, node: GraphNode) {
  selectedNodeId.value = node.id
  if (node.uid) workspace.setSelectedUids([node.uid])
  draggingNode.value = node
  dragStart = { x: e.clientX, y: e.clientY, nodeX: node.x, nodeY: node.y }
}

function onMouseMove(e: MouseEvent) {
  if (isPanning.value) {
    pan.x = e.clientX - panStart.x
    pan.y = e.clientY - panStart.y
  } else if (draggingNode.value) {
    const dx = (e.clientX - dragStart.x) / zoom.value
    const dy = (e.clientY - dragStart.y) / zoom.value
    const newX = dragStart.nodeX + dx
    const newY = dragStart.nodeY + dy
    nodePositions[draggingNode.value.id] = { x: newX, y: newY }
  }
}

function onMouseUp() {
  isPanning.value = false
  draggingNode.value = null
}

function onNodeDblClick(node: GraphNode) {
  if (node.type === 'beacon' && node.uid) {
    workspace.openTab({
      id: `beacon:${node.uid}`,
      title: isZh.value ? `会话交互: ${node.title}` : `Beacon: ${node.title}`,
      component: 'beacon',
      params: { uid: node.uid },
    })
  } else if (node.type === 'listener') {
    workspace.openTab({ id: 'listeners', title: isZh.value ? '监听器' : 'Listeners', component: 'listeners' })
  }
}

function onNodeContextMenu(e: MouseEvent, node: GraphNode) {
  if (node.type !== 'beacon') return
  selectedNodeId.value = node.id
  if (node.uid) workspace.setSelectedUids([node.uid])
  ctxMenu.value = {
    visible: true,
    x: e.clientX,
    y: e.clientY,
    node,
  }
}

function closeCtx() {
  ctxMenu.value.visible = false
}

function onContainerMouseDown() {
  closeCtx()
}

function ctxAction(action: string) {
  const node = ctxMenu.value.node
  closeCtx()
  if (!node || !node.uid) return

  const uid = node.uid
  const label = node.title

  if (action === 'interact') {
    workspace.openTab({ id: `beacon:${uid}`, title: isZh.value ? `会话交互: ${label}` : `Beacon: ${label}`, component: 'beacon', params: { uid } })
  } else if (action === 'files') {
    workspace.openTab({ id: `files:${uid}`, title: isZh.value ? `文件管理: ${label}` : `Files: ${label}`, component: 'client-files', params: { uid } })
  } else if (action === 'pids') {
    workspace.openTab({ id: `pids:${uid}`, title: isZh.value ? `进程列表: ${label}` : `Processes: ${label}`, component: 'client-pids', params: { uid } })
  } else if (action === 'screenshots') {
    workspace.openTab({ id: `screenshots:${uid}`, title: isZh.value ? `屏幕截图: ${label}` : `Screenshots: ${label}`, component: 'client-screenshots', params: { uid } })
  } else if (action === 'note') {
    workspace.openDialog('note', { uid, note: node.raw?.Note || '' })
  } else if (action === 'color') {
    workspace.openDialog('color', { uid })
  } else if (action === 'sleep') {
    workspace.openDialog('sleep', { uid, sleep: parseInt(node.raw?.Sleep || '5') || 5 })
  }
}

// ===== 后端拓扑数据获取（/graph/data，失败静默回退手拼） =====
const eventsStore = useEventsStore()

async function fetchGraphData() {
  try {
    const [graphRes, listenerRes] = await Promise.allSettled([
      ClientAPI.get_graph_data(),
      ListenerAPI.getListenerList(),
    ])

    if (listenerRes.status === 'fulfilled' && listenerRes.value?.data?.data) {
      rawListeners.value = listenerRes.value.data.data
    }

    if (graphRes.status === 'fulfilled' && graphRes.value?.data?.status === 200 && graphRes.value?.data?.data) {
      graphData.value = graphRes.value.data.data
    }
  } catch {
    // 保持 null → computedNodes 回退手拼图
  }
}

let graphTimer: number | undefined

// 客户端状态事件到达时联动刷新（节流 5s）
let lastGraphRefresh = 0
watch(
  () => eventsStore.events.length,
  () => {
    const ev = eventsStore.events[0]
    if (ev && CLIENT_STATE_EVENTS.includes(ev.type)) {
      const now = Date.now()
      if (now - lastGraphRefresh > 5000) {
        lastGraphRefresh = now
        fetchGraphData()
      }
    }
  },
)

onMounted(() => {
  window.addEventListener('mousemove', onMouseMove)
  window.addEventListener('mouseup', onMouseUp)
  fetchGraphData()
  graphTimer = window.setInterval(fetchGraphData, 30000)
})

onBeforeUnmount(() => {
  window.removeEventListener('mousemove', onMouseMove)
  window.removeEventListener('mouseup', onMouseUp)
  if (graphTimer) window.clearInterval(graphTimer)
})
</script>

<style scoped>
.pivot-graph-container {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-panel);
  overflow: hidden;
  position: relative;
  user-select: none;
}
.pg-toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 36px;
  padding: 0 12px;
  background: var(--cs-titlebar-bg);
  border-bottom: 1px solid var(--cs-border);
  flex-shrink: 0;
  z-index: 10;
}
.pg-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--cs-text-bright);
  letter-spacing: .02em;
}
.pg-btn-group {
  display: flex;
  align-items: center;
  gap: 4px;
}
.pg-btn {
  background: var(--cs-panel);
  border: 1px solid var(--cs-border);
  border-radius: 2px;
  color: var(--cs-text);
  font-size: 14px;
  padding: 2px 8px;
  cursor: pointer;
}
.pg-btn:hover {
  background: var(--cs-hover);
  color: var(--cs-text-bright);
}
.pg-spacer { flex: 1; }
.pg-toggle {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 15px;
  color: var(--cs-text);
  cursor: pointer;
}
.pg-stats {
  font-size: 15px;
  color: var(--cs-text-dim);
  font-family: var(--cs-font-mono);
}

/* Canvas Area */
.pg-canvas {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  background: var(--cs-bg);
  cursor: grab;
  position: relative;
  /* CS Canvas subtle grid texture */
  background-image: radial-gradient(circle at 1px 1px, var(--cs-border) 1px, transparent 0);
  background-size: 24px 24px;
}
.pg-canvas.dragging {
  cursor: grabbing;
}
.pg-svg {
  width: 100%;
  height: 100%;
  overflow: visible;
}

/* Links */
.pg-link-line {
  fill: none;
  stroke: var(--cs-border);
  stroke-width: 1.5;
  stroke-dasharray: 4 2;
  transition: stroke 200ms ease;
}
.pg-link-line.active {
  stroke: var(--cs-active);
  stroke-dasharray: none;
  stroke-width: 2;
}

/* Nodes */
.pg-node-g {
  cursor: pointer;
  transition: transform 60ms ease;
}
.node-card {
  fill: var(--cs-panel);
  stroke: var(--cs-border);
  stroke-width: 1.5;
  filter: drop-shadow(0 2px 5px rgba(0, 0, 0, 0.3));
  transition: all 120ms ease;
}
.pg-node-g:hover .node-card {
  stroke: var(--cs-active);
  fill: var(--cs-hover);
}
.pg-node-g.selected .node-card {
  stroke: var(--cs-active);
  stroke-width: 2;
  box-shadow: 0 0 8px var(--cs-active);
}
.pg-node-g.offline .node-card {
  opacity: 0.55;
}
.node-status-dot {
  fill: var(--cs-offline);
}
.node-status-dot.online {
  fill: var(--cs-online);
}
.node-title {
  font-size: 11px;
  font-weight: 600;
  fill: var(--cs-text-bright);
  font-family: var(--cs-font-ui);
}
.node-sub {
  font-size: 10px;
  fill: var(--cs-text-dim);
  font-family: var(--cs-font-mono);
}
.node-ip {
  font-size: 9px;
  fill: var(--cs-text);
  font-family: var(--cs-font-mono);
}

/* Context Menu */
.cs-ctx-menu {
  position: fixed;
  z-index: 3500;
  min-width: 160px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-border);
  border-radius: 2px;
  padding: 3px 0;
  box-shadow: 2px 4px 14px rgba(0, 0, 0, 0.5);
  user-select: none;
}
.ctx-header {
  padding: 4px 10px;
  border-bottom: 1px solid var(--cs-border);
  margin-bottom: 2px;
}
.ctx-target-title {
  font-size: 11px;
  font-weight: 600;
  color: var(--cs-text-dim);
  font-family: var(--cs-font-mono);
}
.ctx-item {
  padding: 4px 10px;
  font-size: 12px;
  color: var(--cs-text);
  cursor: pointer;
}
.ctx-item:hover {
  background: var(--cs-hover);
  color: var(--cs-text-bright);
}
.ctx-item.is-highlight {
  font-weight: 600;
}
.ctx-item.ctx-danger {
  color: var(--cs-error);
}
.ctx-divider {
  height: 1px;
  background: var(--cs-border);
  margin: 3px 0;
}
</style>
