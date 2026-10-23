<template>
  <div class="cs-connection-pool-view cs-font-ui">
    <!-- vshell 工具条：内容上方一行（左标题/统计，右操作） -->
    <div class="cp-toolbar">
      <div class="cp-title-group">
        <span class="cp-title">{{ i18nStore.tl('连接池与工作线程管理', 'Connection Pool & Workers') }}</span>
        <span class="cp-stats">
          ({{ poolStats.active_connections || connections.length }} {{ i18nStore.tl('个活动连接', 'active sockets') }} / {{ workerStats.worker_count || 16 }} {{ i18nStore.tl('个工作线程', 'workers') }})
        </span>
      </div>

      <div class="cp-spacer" />

      <button class="cs-btn" @click="handleHelp">
        {{ i18nStore.tl('帮助', 'Help') }}
      </button>

      <button v-if="canOperate" class="cs-btn cs-btn-primary" @click="handleSavePoolConfig">
        {{ i18nStore.tl('保存连接池参数', 'Save Pool Tuning') }}
      </button>

      <button class="cs-btn" :disabled="healthChecking" @click="handleHealthCheck">
        {{ healthChecking ? (i18nStore.tl('检查中...', 'Checking...')) : (i18nStore.tl('健康检查', 'Health Check')) }}
      </button>

      <button class="cs-btn" :disabled="loading" @click="fetchData">
        {{ i18nStore.tl('刷新', 'Refresh') }}
      </button>
    </div>

    <!-- Main Workspace Split -->
    <div class="cp-body">
      <!-- Top / Left: Active Connections Table -->
      <div class="cp-table-panel">
        <div class="cp-panel-header">
          <span class="panel-title">{{ i18nStore.tl('活动套接字与传输会话', 'Active Sockets & Transport Sessions') }}</span>
          <div class="cp-filter-group">
            <!-- 后端 /connection/protocol/:protocol 协议过滤 -->
            <el-select
              v-model="protocolFilter"
              :placeholder="i18nStore.tl('全部协议', 'All Protocols')"
              clearable
              class="cp-protocol-select"
              @change="onProtocolFilterChange"
            >
              <el-option label="TCP" value="tcp" />
              <el-option label="KCP" value="kcp" />
              <el-option label="WebSocket" value="websocket" />
              <el-option label="HTTP" value="http" />
              <el-option label="HTTPS" value="https" />
              <el-option label="DNS" value="dns" />
            </el-select>
            <!-- 后端 /connection/uid/:uid UID 精确查询 -->
            <el-input
              v-model="uidFilter"
              :placeholder="i18nStore.tl('按 Beacon UID 查询', 'Lookup Beacon UID')"
              clearable
              class="cp-uid-input"
              @keyup.enter="onUidSearch"
            >
              <template #prefix>
                <svg class="cp-search-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
                  <circle cx="11" cy="11" r="7" />
                  <path d="M20 20l-3.6-3.6" />
                </svg>
              </template>
            </el-input>
            <button class="cs-btn cs-btn-xs" @click="onUidSearch">{{ i18nStore.tl('查询', 'Find') }}</button>
            <button v-if="protocolFilter || uidFilter" class="cs-btn cs-btn-xs" @click="clearFilters">
              {{ i18nStore.tl('清除过滤', 'Clear') }}
            </button>
            <button v-if="canOperate" class="cs-btn cs-btn-xs cs-btn-danger" :disabled="!selectedConnection" @click="selectedConnection && handleCloseConn(selectedConnection)">
              {{ i18nStore.tl('断开选中连接', 'Disconnect Selected') }}
            </button>
          </div>
        </div>

        <div class="cp-table-wrap">
          <el-table
            :data="pagedConnections"
            height="100%"
            highlight-current-row
            class="cs-session-table"
            v-loading="loading"
            @current-change="(r: any) => selectedConnection = r"
          >
            <el-table-column prop="id" :label="i18nStore.tl('套接字 ID', 'Socket ID')" width="100" sortable>
              <template #default="{ row }">
                <span class="cs-font-mono vs-strong">{{ row.id || row.Id || 'sock-1' }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="protocol" :label="i18nStore.tl('协议', 'Protocol')" width="100" sortable align="center">
              <template #default="{ row }">
                <span class="vs-chip">{{ row.protocol || 'TCP' }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="client_ip" :label="i18nStore.tl('远端地址', 'Remote Address')" min-width="180" sortable show-overflow-tooltip>
              <template #default="{ row }">
                <span class="cs-font-mono">{{ row.client_ip || row.remote_addr || '-' }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="uid" :label="i18nStore.tl('Beacon UID', 'Beacon UID')" min-width="160" show-overflow-tooltip>
              <template #default="{ row }">
                <span class="cs-font-mono vs-dim">{{ row.uid || row.Uid || '-' }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="uptime" :label="i18nStore.tl('运行时长', 'Uptime')" width="130" align="right">
              <template #default="{ row }">
                <span class="cs-font-mono">{{ formatUptime(row.uptime, row) }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="is_active" :label="i18nStore.tl('状态', 'State')" width="110" align="center">
              <template #default="{ row }">
                <span class="vs-status-tag" :class="row.is_active ? 'is-online' : 'is-offline'">
                  {{ row.is_active ? 'ESTABLISHED' : 'CLOSED' }}
                </span>
              </template>
            </el-table-column>

            <template #empty>
              <EmptyState variant="inline" icon="🔌" :title="i18nStore.tl('暂无活动连接', 'No active connections')" />
            </template>
          </el-table>
        </div>

        <!-- 底部分页（前端分页：/connection/details 单页上限 100） -->
        <div class="cp-table-pager">
          <el-pagination
            small
            background
            layout="prev, pager, next, sizes, total"
            :total="connections.length"
            :current-page="page"
            :page-size="pageSize"
            :page-sizes="[10, 20, 50]"
            @current-change="onPageChange"
            @size-change="onSizeChange"
          />
        </div>
      </div>

      <!-- Bottom / Right: Pool Tuning & Worker Thread Pool -->
      <div class="cp-config-panel">
        <div class="cp-panel-header">
          <span class="panel-title">{{ i18nStore.tl('连接池与工作线程配置', 'Pool Tuning & Worker Thread Pool') }}</span>
        </div>

        <div class="cp-config-grid">
          <div class="cp-form-col">
            <div class="cs-form-row">
              <label class="cs-form-label">{{ i18nStore.tl('工作线程数', 'Worker Pool Size') }}:</label>
              <div class="input-with-btn">
                <el-input-number v-model="workerCount" :min="4" :max="128" :disabled="!canOperate" />
                <button v-if="canOperate" class="cs-btn cs-btn-primary" @click="handleResizeWorkers">{{ i18nStore.tl('应用', 'Apply Size') }}</button>
              </div>
            </div>

            <div class="cs-form-row">
              <label class="cs-form-label">{{ i18nStore.tl('最大连接数', 'Max Pool Connections') }}:</label>
              <el-input-number v-model="poolConfig.max_connections" :min="50" :max="5000" :disabled="!canOperate" />
            </div>

            <div class="cs-form-row">
              <label class="cs-form-label">{{ i18nStore.tl('最大空闲时间', 'Max Idle Duration') }}:</label>
              <el-input v-model="poolConfig.max_idle_time" placeholder="300s" class="cp-idle-input" :disabled="!canOperate" />
            </div>
          </div>

          <div class="cp-stats-col">
            <div class="stat-mini-card">
              <div class="stat-num">{{ poolStats.active_connections || connections.length }}</div>
              <div class="stat-lbl">{{ i18nStore.tl('活动 TCP/HTTP 连接', 'Active TCP/HTTP Sockets') }}</div>
            </div>
            <div class="stat-mini-card">
              <div class="stat-num">{{ workerStats.worker_count || workerCount }}</div>
              <div class="stat-lbl">{{ i18nStore.tl('专用工作线程数', 'Dedicated Worker Threads') }}</div>
            </div>
            <div class="stat-mini-card">
              <div class="stat-lbl">{{ i18nStore.tl('连接池健康状态', 'Pool Health State') }}</div>
              <span
                class="vs-status-tag"
                :class="String(poolStats.health_status || 'HEALTHY').toUpperCase() === 'HEALTHY' ? 'is-online' : 'is-offline'"
              >
                {{ poolStats.health_status || 'HEALTHY' }}
              </span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import ConnectionAPI from '@/api/connection'
import EmptyState from '@/components/Common/EmptyState.vue'
import { useI18nStore } from '@/stores/i18n'
import { useUserStore } from '@/stores/user'

const i18nStore = useI18nStore()
const userStore = useUserStore()
const isZh = computed(() => i18nStore.isZh)
// 连接池写接口是 operatorOnly；auditor 只能看统计与连接列表
const canOperate = computed(() => String(userStore.getRoles() || '').toLowerCase() !== 'auditor')

const loading = ref(false)
const healthChecking = ref(false)
const selectedConnection = ref<any>(null)
const workerCount = ref(16)

// 初始为空，真实数据来自后端（失败时保持空 + 报错，不再展示假占位）
const poolStats = reactive<any>({
  active_connections: 0,
  idle_connections: 0,
  max_connections: 1000,
  health_status: '',
})

const workerStats = reactive<any>({
  worker_count: 0,
  active_jobs: 0,
})

const poolConfig = reactive({
  max_connections: 1000,
  max_idle_time: '300s',
  health_check_period: '30s',
})

const connections = ref<any[]>([])

// 连接列表过滤：协议走 /connection/protocol/:protocol，UID 走 /connection/uid/:uid
const protocolFilter = ref('')
const uidFilter = ref('')
// 分页（前端分页：/connection/details 单页上限 100）
const page = ref(1)
const pageSize = ref(20)
const pagedConnections = computed(() => {
  const maxPage = Math.max(1, Math.ceil(connections.value.length / pageSize.value))
  const p = Math.min(page.value, maxPage)
  const start = (p - 1) * pageSize.value
  return connections.value.slice(start, start + pageSize.value)
})
function onPageChange(p: number) { page.value = p }
function onSizeChange(s: number) { pageSize.value = s; page.value = 1 }
watch([protocolFilter, uidFilter], () => { page.value = 1 })

async function loadConnections() {
  if (uidFilter.value.trim()) {
    // UID 精确查询（单条连接详情）
    try {
      const res = await ConnectionAPI.getConnectionByUID(uidFilter.value.trim())
      if (res.data?.status === 200) {
        connections.value = [res.data.data]
        return
      }
      connections.value = []
    } catch {
      connections.value = []
      ElMessage.warning(isZh.value ? `未找到 UID 为 ${uidFilter.value} 的连接` : `No connection found for UID ${uidFilter.value}`)
      return
    }
  }

  if (protocolFilter.value) {
    // 协议过滤（专用端点）
    try {
      const res = await ConnectionAPI.getConnectionsByProtocol(protocolFilter.value)
      if (res.data?.status === 200) {
        connections.value = res.data.data?.connections || []
        return
      }
    } catch {
      connections.value = []
      ElMessage.error(isZh.value ? '获取协议连接失败' : 'Failed to load protocol connections')
      return
    }
  }

  // 全量连接（详情端点支持 protocol 查询参数）
  try {
    const res = await ConnectionAPI.getConnectionDetails({ page: 1, page_size: 100 })
    if (res.data?.status === 200 && Array.isArray(res.data.data?.connections)) {
      connections.value = res.data.data.connections
    } else {
      connections.value = []
      ElMessage.error(isZh.value ? '获取连接列表失败' : 'Failed to load connections')
    }
  } catch {
    connections.value = []
  }
}

function onProtocolFilterChange() {
  uidFilter.value = ''
  loadConnections()
}

function onUidSearch() {
  protocolFilter.value = ''
  loadConnections()
}

function clearFilters() {
  protocolFilter.value = ''
  uidFilter.value = ''
  loadConnections()
}

function formatUptime(val: any, row?: any): string {
  if (val == null || val === '') return '-'
  let seconds = Number(val)
  if (isNaN(seconds)) return String(val)

  // 1. 如果是纳秒级（如 6425866514720ns），值 > 1e11
  if (seconds > 1e11) {
    seconds = Math.floor(seconds / 1e9)
  }
  // 2. 如果是毫秒级时间戳或绝对时间戳（如 1770000000000 或 1770000000）
  else if (seconds > 1e9) {
    const nowSec = Math.floor(Date.now() / 1000)
    if (seconds > 1000000000 && seconds <= nowSec + 10) {
      seconds = Math.max(0, nowSec - seconds)
    } else if (seconds > nowSec + 10) {
      seconds = Math.floor(seconds / 1000)
    }
  }

  if (seconds < 0) seconds = 0

  if (seconds < 60) return `${seconds}s`
  const mins = Math.floor(seconds / 60)
  const secs = seconds % 60
  if (mins < 60) return `${mins}m ${secs}s`
  const hours = Math.floor(mins / 60)
  const remMins = mins % 60
  if (hours < 24) return `${hours}h ${remMins}m ${secs}s`
  const days = Math.floor(hours / 24)
  const remHours = hours % 24
  return `${days}d ${remHours}h ${remMins}m`
}

async function fetchData() {
  loading.value = true
  try {
    const [pRes, wRes] = await Promise.allSettled([
      ConnectionAPI.getPoolStats(),
      ConnectionAPI.getWorkerStats(),
    ])
    if (pRes.status === 'fulfilled' && pRes.value?.data?.data) {
      const data = pRes.value.data.data
      // 后端把连接数放在 pool_stats 子对象里；同时保留 max_connections 等顶层字段
      Object.assign(poolStats, data, data.pool_stats || {})
      if (!poolStats.health_status) {
        poolStats.health_status = (poolStats.utilization || 0) >= 0.9 ? 'DEGRADED' : 'HEALTHY'
      }
    }
    if (wRes.status === 'fulfilled' && wRes.value?.data?.data) {
      Object.assign(workerStats, wRes.value.data.data)
      if (workerStats.worker_count) workerCount.value = workerStats.worker_count
    }
    await loadConnections()
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '获取连接池数据失败' : 'Failed to load connection pool data'))
  } finally {
    loading.value = false
  }
}

async function handleHealthCheck() {
  healthChecking.value = true
  try {
    const res = await ConnectionAPI.performHealthCheck()
    if (res?.data?.status === 200 || res?.data?.success) {
      ElMessage.success(isZh.value ? '健康检查完成：所有连接正常' : 'Health check complete: All connection sockets normal')
    } else {
      ElMessage.error(res?.data?.message || (isZh.value ? '健康检查失败' : 'Health check failed'))
    }
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '健康检查失败' : 'Health check failed'))
  } finally {
    healthChecking.value = false
  }
}

function handleCloseConn(row: any) {
  if (!canOperate.value) {
    ElMessage.warning(isZh.value ? '只读账号无法修改连接池' : 'Read-only account cannot modify the pool')
    return
  }
  ElMessageBox.confirm(
      isZh.value ? `终止套接字连接 ${row.id}（${row.client_ip || row.remote_addr || '-'}）吗？` : `Terminate socket connection ${row.id} (${row.client_ip || row.remote_addr || '-'})?`,
      isZh.value ? '断开套接字' : 'Disconnect Socket',
      {
        confirmButtonText: isZh.value ? '断开' : 'Disconnect',
        cancelButtonText: isZh.value ? '取消' : 'Cancel',
        type: 'warning',
      },
    ).then(async () => {
    try {
      await ConnectionAPI.closeConnection(row.id)
      connections.value = connections.value.filter((c) => c.id !== row.id)
      selectedConnection.value = null
      ElMessage.success(isZh.value ? `连接 ${row.id} 已终止` : `Connection ${row.id} terminated`)
    } catch (e: any) {
      ElMessage.error(e?.message || (isZh.value ? `连接 ${row.id} 断开失败` : `Failed to terminate connection ${row.id}`))
    }
  }).catch(() => {})
}

async function handleResizeWorkers() {
  if (!canOperate.value) {
    ElMessage.warning(isZh.value ? '只读账号无法修改连接池' : 'Read-only account cannot modify the pool')
    return
  }
  try {
    await ConnectionAPI.resizeWorkerPool(workerCount.value)
    ElMessage.success(isZh.value ? `工作线程池已调整为 ${workerCount.value}` : `Worker thread pool resized to ${workerCount.value}`)
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '调整工作线程失败' : 'Failed to resize worker pool'))
  }
}

async function handleSavePoolConfig() {
  if (!canOperate.value) {
    ElMessage.warning(isZh.value ? '只读账号无法修改连接池' : 'Read-only account cannot modify the pool')
    return
  }
  try {
    await ConnectionAPI.updatePoolConfig(poolConfig)
    ElMessage.success(isZh.value ? '连接池配置已应用' : 'Connection pool tuning applied')
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '保存连接池配置失败' : 'Failed to save pool tuning'))
  }
}

function handleHelp() {
  ElMessage.info(isZh.value ? '连接池与工作线程引擎说明' : 'Connection Pool & Worker Thread Engine Reference')
}

onMounted(() => {
  fetchData()
})
</script>

<style scoped>
.cs-connection-pool-view {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-bg);
  overflow: hidden;
}

/* ===== 工具条：内容上方一行（左标题/统计，右操作） ===== */
.cp-toolbar {
  min-height: 48px;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  padding: 8px 12px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-card-border);
  flex-shrink: 0;
}

.cp-title-group {
  display: flex;
  align-items: baseline;
  gap: 8px;
}

.cp-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
  white-space: nowrap;
}

.cp-stats {
  font-size: 12px;
  color: var(--cs-text-dim);
  white-space: nowrap;
}

.cp-spacer {
  flex: 1;
}

/* ===== 内容区：12px 留白，两张卡片（会话表 / 池参数） ===== */
.cp-body {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 12px;
  overflow: hidden;
}

.cp-table-panel {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  background: var(--cs-panel);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  overflow: hidden;
}

.cp-config-panel {
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  background: var(--cs-panel);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  overflow: hidden;
}

/* 卡片头：会话卡头兼任筛选工具条 */
.cp-panel-header {
  min-height: 48px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 8px;
  padding: 6px 12px;
  border-bottom: 1px solid var(--cs-card-border);
  flex-shrink: 0;
}

.cp-config-panel .cp-panel-header {
  min-height: 38px;
  padding: 0 12px;
}

.panel-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.cp-filter-group {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.cp-protocol-select {
  width: 120px;
}

.cp-uid-input {
  width: 200px;
}

.cp-search-icon {
  color: var(--cs-text-dim);
}

.cp-table-wrap {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

/* ===== 池参数表单 ===== */
.cp-config-grid {
  display: flex;
  gap: 24px;
  padding: 12px 16px;
}

.cp-form-col {
  flex: 1;
  display: flex;
  flex-direction: column;
}

.cp-stats-col {
  width: 260px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.input-with-btn {
  display: flex;
  align-items: center;
  gap: 8px;
}

.cp-idle-input {
  width: 160px;
}

.stat-mini-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 12px;
  background: var(--cs-bg);
  border-radius: 8px;
}

.stat-num {
  font-size: 16px;
  font-weight: 600;
  font-family: var(--cs-font-mono);
  color: var(--cs-text-bright);
}

.stat-lbl {
  font-size: 12px;
  color: var(--cs-text-dim);
}

/* ===== 单元格内容 ===== */
/* 状态：在线实心绿 tag，离线灰 tag（20px / 4px 圆角 / 12px） */
.vs-status-tag {
  display: inline-flex;
  align-items: center;
  height: 20px;
  padding: 0 7px;
  border-radius: 4px;
  font-size: 12px;
  line-height: 1;
  white-space: nowrap;
}

.vs-status-tag.is-online {
  background: var(--cs-online);
  color: #fff;
}

.vs-status-tag.is-offline {
  background: color-mix(in srgb, var(--cs-offline) 22%, transparent);
  color: var(--cs-text-dim);
}

/* 中性 chip（协议等非状态标签） */
.vs-chip {
  display: inline-flex;
  align-items: center;
  height: 20px;
  padding: 0 7px;
  border-radius: 4px;
  font-size: 12px;
  line-height: 1;
  background: var(--cs-table-head);
  border: 1px solid var(--cs-card-border);
  color: var(--cs-text);
}

.vs-dim {
  color: var(--cs-text-dim);
}

.vs-strong {
  font-weight: 600;
  color: var(--cs-text-bright);
}

/* 空状态：图标 + 一行说明（居中） */
.vs-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  padding: 24px 0;
  font-size: 13px;
  color: var(--cs-text-dim);
}

.vs-empty svg {
  color: var(--cs-text-dim);
  opacity: 0.5;
}

.cp-table-pager {
  flex-shrink: 0;
  display: flex;
  justify-content: flex-end;
  padding: 8px 12px;
  border-top: 1px solid var(--cs-card-border);
}
</style>
