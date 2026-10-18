<template>
  <div class="pg-root">
    <div class="pg-head">
      <span class="pg-title">{{ i18nStore.tl('拓扑网络图', 'Pivot Graph') }}</span>
      <span class="pg-sub">{{ i18nStore.tl('服务端 → 监听器 → 客户端 → 隧道 → 目标', 'Server → Listener → Client → Tunnel → Target') }}</span>
      <div class="pg-actions">
        <span class="pg-stat">
          {{ i18nStore.tl('节点', 'Nodes') }} {{ layout.nodes.length }} ·
          {{ i18nStore.tl('在线', 'Online') }} {{ onlineCount }} / {{ clientCount }}
        </span>
        <button class="cs-btn cs-btn-xs" :disabled="loading" @click="load">
          {{ loading ? (i18nStore.tl('刷新中…', 'Loading…')) : (i18nStore.tl('刷新', 'Refresh')) }}
        </button>
      </div>
    </div>

    <!-- 图例：颜色/实心空心的含义必须在图上说明，否则用户只能猜 -->
    <div class="pg-legend">
      <span v-for="t in legendTypes" :key="t" class="pg-legend-item">
        <i class="pg-dot" :style="{ background: TYPE_COLORS[t] }"></i>
        {{ i18nStore.tl(TYPE_LABELS[t]?.zh || t, TYPE_LABELS[t]?.en || t) }}
      </span>
      <span class="pg-legend-item">
        <i class="pg-dot pg-dot-offline"></i>
        {{ i18nStore.tl('离线客户端（灰色虚线）', 'Offline client (dashed)') }}
      </span>
    </div>

    <div v-if="error" class="pg-error">{{ error }}</div>

    <div v-else-if="!loading && layout.nodes.length === 0" class="pg-empty">
      {{ i18nStore.tl('暂无拓扑数据：先创建监听器并让客户端上线', 'No topology data yet: create a listener and bring a client online') }}
    </div>

    <div v-else class="pg-canvas" @click.self="selected = null">
      <svg
        :width="layout.width"
        :height="layout.height"
        :viewBox="`0 0 ${layout.width} ${layout.height}`"
        role="img"
        :aria-label="i18nStore.tl('拓扑网络图', 'Pivot graph')"
      >
        <!-- 边先画，节点压在上面 -->
        <g class="pg-edges">
          <line
            v-for="e in layout.edges"
            :key="e.id"
            :x1="e.x1"
            :y1="e.y1"
            :x2="e.x2"
            :y2="e.y2"
            :class="['pg-edge', { 'pg-edge-active': isEdgeActive(e) }]"
          />
        </g>

        <g class="pg-nodes">
          <g
            v-for="nd in layout.nodes"
            :key="nd.id"
            :transform="`translate(${nd.x}, ${nd.y})`"
            :class="['pg-node', { 'pg-node-selected': selected === nd.id, 'pg-node-offline': !isOnline(nd) }]"
            role="button"
            tabindex="0"
            @click.stop="selected = selected === nd.id ? null : nd.id"
            @keydown.enter.prevent="selected = selected === nd.id ? null : nd.id"
            @keydown.space.prevent="selected = selected === nd.id ? null : nd.id"
          >
            <circle
              r="9"
              :fill="isOnline(nd) ? TYPE_COLORS[nd.type] || '#6b7280' : 'transparent'"
              :stroke="TYPE_COLORS[nd.type] || '#6b7280'"
              stroke-width="2"
              :stroke-dasharray="isOnline(nd) ? undefined : '3 2'"
            />
            <text class="pg-node-label" x="16" y="4">{{ nd.label }}</text>
            <text v-if="nd.sub_label" class="pg-node-sub" x="16" y="19">{{ nd.sub_label }}</text>
          </g>
        </g>
      </svg>
    </div>

    <!-- 选中节点的详情：图上一眼看不全的信息（meta）放这里 -->
    <div v-if="selectedNode" class="pg-detail">
      <div class="pg-detail-head">
        <strong>{{ selectedNode.label }}</strong>
        <span class="pg-detail-type">
          {{ i18nStore.tl(
            TYPE_LABELS[selectedNode.type]?.zh || selectedNode.type,
            TYPE_LABELS[selectedNode.type]?.en || selectedNode.type,
          ) }}
        </span>
        <span v-if="selectedNode.type === 'client'" :class="['pg-badge', isOnline(selectedNode) ? 'pg-badge-on' : 'pg-badge-off']">
          {{ isOnline(selectedNode) ? (i18nStore.tl('在线', 'Online')) : (i18nStore.tl('离线', 'Offline')) }}
        </span>
        <button class="cs-btn cs-btn-xs" @click="selected = null">{{ i18nStore.tl('关闭', 'Close') }}</button>
      </div>
      <div v-if="selectedNode.sub_label" class="pg-detail-sub">{{ selectedNode.sub_label }}</div>
      <table v-if="metaRows.length" class="pg-meta">
        <tbody>
          <tr v-for="row in metaRows" :key="row.k">
            <td class="pg-meta-k">{{ row.k }}</td>
            <td class="pg-meta-v">{{ row.v }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * Pivot 拓扑网络图（阶段三 §3.11）。
 *
 * 这个视图此前是**空壳**：`api/clients/index.ts` 的 `get_graph_data()` 与请求层
 * 对 `/graph/data` 的静默失败特判都在，i18n 也有 `tb.pivot_graph`，
 * 但**没有任何组件消费** —— 后端算好的关系图没人看。
 *
 * 这里把数据画出来，并刻意做了三件"图能看懂"的事：
 *  1. 分层布局（layout.ts）而不是力导向：这份数据本身是分层的，
 *     而且力导向每次刷新的坐标都不同，同一个拓扑看起来像变了一个网；
 *  2. 离线客户端画成**灰色虚线空心**，与在线实心一眼可分 —— 否则一片点看不出谁掉线；
 *  3. 点击节点在下方列出 meta：图上放不下的地址/密钥指纹等信息集中在这里。
 */
import { computed, onMounted, ref } from 'vue'
import ClientApi from '@/api/clients'
import { useI18nStore } from '@/stores/i18n'
import {
  computeLayout,
  isOnline,
  TYPE_COLORS,
  TYPE_LABELS,
  type GraphEdge,
  type GraphNode,
  type LaidOutEdge,
} from './layout'

const i18nStore = useI18nStore()

const nodes = ref<GraphNode[]>([])
const edges = ref<GraphEdge[]>([])
const loading = ref(false)
const error = ref('')
const selected = ref<string | null>(null)

const layout = computed(() => computeLayout(nodes.value, edges.value))

const legendTypes = ['server', 'listener', 'client', 'tunnel', 'target']

const clientCount = computed(() => nodes.value.filter((n) => n.type === 'client').length)
const onlineCount = computed(() => nodes.value.filter((n) => n.type === 'client' && isOnline(n)).length)

const selectedNode = computed(() => nodes.value.find((n) => n.id === selected.value) || null)
const metaRows = computed(() =>
  Object.entries(selectedNode.value?.meta || {})
    .filter(([, v]) => v !== '')
    .map(([k, v]) => ({ k, v: String(v) })),
)

/** 选中节点相关的边高亮，其余淡出 —— 否则节点一多就分不清谁连谁 */
function isEdgeActive(e: LaidOutEdge): boolean {
  if (!selected.value) return true
  return e.source === selected.value || e.target === selected.value
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await ClientApi.get_graph_data()
    const data = res?.data?.data || {}
    nodes.value = Array.isArray(data.nodes) ? data.nodes : []
    edges.value = Array.isArray(data.edges) ? data.edges : []
    // 选中的节点可能在刷新后消失了（客户端被删），清掉避免详情区停在幽灵节点
    if (selected.value && !nodes.value.some((n) => n.id === selected.value)) {
      selected.value = null
    }
  } catch (e: unknown) {
    // /graph/data 是 operatorOnly：auditor 拿到 403 属正常，不该弹全局报错
    // （请求层已对该 URL 特判静默）。这里给出页面内提示而不是空白页。
    const msg = (e as { response?: { status?: number } })?.response?.status === 403
      ? i18nStore.tl(
          '当前角色无权查看拓扑图（需要 operator 及以上）',
          'Your role cannot view the pivot graph (operator+)',
        )
      : i18nStore.tl('加载拓扑数据失败', 'Failed to load topology data')
    error.value = msg
    nodes.value = []
    edges.value = []
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.pg-root {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  padding: 12px 16px;
  gap: 8px;
  color: var(--vs-text, #e6e6e6);
}
.pg-head {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.pg-title {
  font-size: 15px;
  font-weight: 600;
}
.pg-sub {
  font-size: 12px;
  opacity: 0.6;
}
.pg-actions {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 10px;
}
.pg-stat {
  font-size: 12px;
  opacity: 0.75;
}
.pg-legend {
  display: flex;
  flex-wrap: wrap;
  gap: 14px;
  font-size: 12px;
  opacity: 0.85;
}
.pg-legend-item {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}
.pg-dot {
  width: 9px;
  height: 9px;
  border-radius: 50%;
  display: inline-block;
}
.pg-dot-offline {
  background: transparent;
  border: 2px dashed #6b7280;
}
.pg-error,
.pg-empty {
  padding: 24px;
  font-size: 13px;
  opacity: 0.8;
}
.pg-error {
  color: #f0a020;
}
.pg-canvas {
  flex: 1;
  min-height: 0;
  overflow: auto;
  border: 1px solid var(--vs-border, #333);
  border-radius: 6px;
  background: var(--vs-bg-soft, rgba(255, 255, 255, 0.02));
}
.pg-edge {
  stroke: #4b5563;
  stroke-width: 1.2;
  opacity: 0.55;
}
.pg-edge-active {
  stroke: #8b9dc3;
  opacity: 0.9;
}
.pg-node {
  cursor: pointer;
}
.pg-node-label {
  font-size: 12px;
  fill: var(--vs-text, #e6e6e6);
}
.pg-node-sub {
  font-size: 10px;
  fill: var(--vs-text, #e6e6e6);
  opacity: 0.55;
}
.pg-node-offline .pg-node-label {
  opacity: 0.5;
}
.pg-node-selected .pg-node-label {
  font-weight: 700;
}
.pg-detail {
  border-top: 1px solid var(--vs-border, #333);
  padding-top: 8px;
  font-size: 12px;
}
.pg-detail-head {
  display: flex;
  align-items: center;
  gap: 10px;
}
.pg-detail-type {
  opacity: 0.6;
}
.pg-detail-sub {
  opacity: 0.7;
  margin-top: 2px;
}
.pg-badge {
  padding: 1px 6px;
  border-radius: 8px;
  font-size: 11px;
}
.pg-badge-on {
  background: rgba(47, 158, 110, 0.2);
  color: #4ade80;
}
.pg-badge-off {
  background: rgba(107, 114, 128, 0.25);
  color: #9ca3af;
}
.pg-meta {
  margin-top: 6px;
  border-collapse: collapse;
}
.pg-meta-k {
  opacity: 0.6;
  padding-right: 12px;
  vertical-align: top;
  white-space: nowrap;
}
.pg-meta-v {
  word-break: break-all;
}
</style>
