<template>
  <div class="cs-connection-pool-view cs-font-ui">
    <!-- CS Toolbar -->
    <div class="cp-toolbar">
      <div class="cp-title-group">
        <span class="cp-title">{{ isZh ? '连接池与工作线程管理' : 'Connection Pool & Workers' }}</span>
        <span class="cp-stats">
          ({{ poolStats.active_connections || connections.length }} {{ isZh ? '个活动连接' : 'active sockets' }} / {{ workerStats.worker_count || 16 }} {{ isZh ? '个工作线程' : 'workers' }})
        </span>
      </div>

      <div class="cp-spacer" />

      <button class="cs-btn cs-btn-primary" :disabled="healthChecking" @click="handleHealthCheck">
        {{ healthChecking ? (isZh ? '检查中...' : 'Checking...') : (isZh ? '健康检查' : 'Health Check') }}
      </button>

      <button class="cs-btn" :disabled="loading" @click="fetchData">
        {{ isZh ? '刷新' : 'Refresh' }}
      </button>
    </div>

    <!-- Main Workspace Split -->
    <div class="cp-body">
      <!-- Top / Left: Active Connections Table -->
      <div class="cp-table-panel">
        <div class="cp-panel-header">
          <span class="panel-title">{{ isZh ? '活动套接字与传输会话' : 'Active Sockets & Transport Sessions' }}</span>
          <div class="cp-filter-group">
            <!-- 后端 /connection/protocol/:protocol 协议过滤 -->
            <el-select
              v-model="protocolFilter"
              :placeholder="isZh ? '全部协议' : 'All Protocols'"
              clearable
              size="small"
              style="width: 110px"
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
              :placeholder="isZh ? '按 Beacon UID 查询' : 'Lookup Beacon UID'"
              size="small"
              clearable
              class="cp-uid-input"
              @keyup.enter="onUidSearch"
            />
            <button class="cs-btn cs-btn-xs" @click="onUidSearch">{{ isZh ? '查询' : 'Find' }}</button>
            <button v-if="protocolFilter || uidFilter" class="cs-btn cs-btn-xs" @click="clearFilters">
              {{ isZh ? '清除过滤' : 'Clear' }}
            </button>
            <button class="cs-btn cs-btn-xs cs-btn-danger" :disabled="!selectedConnection" @click="selectedConnection && handleCloseConn(selectedConnection)">
              {{ isZh ? '断开选中连接' : 'Disconnect Selected' }}
            </button>
          </div>
        </div>

        <div class="cp-table-wrap">
          <el-table
            :data="connections"
            size="small"
            height="100%"
            highlight-current-row
            class="cs-session-table"
            v-loading="loading"
            @current-change="(r: any) => selectedConnection = r"
          >
            <el-table-column prop="id" :label="isZh ? '套接字 ID' : 'Socket ID'" width="100" sortable>
              <template #default="{ row }">
                <span class="cs-font-mono font-bold">{{ row.id || row.Id || 'sock-1' }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="protocol" :label="isZh ? '协议' : 'Protocol'" width="100" sortable align="center">
              <template #default="{ row }">
                <span class="arch-badge arch-64">{{ row.protocol || 'TCP' }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="client_ip" :label="isZh ? '远端地址' : 'Remote Address'" min-width="180" sortable show-overflow-tooltip>
              <template #default="{ row }">
                <span class="cs-font-mono">{{ row.client_ip || row.remote_addr || '-' }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="uid" :label="isZh ? 'Beacon UID' : 'Beacon UID'" min-width="160" show-overflow-tooltip>
              <template #default="{ row }">
                <span class="cs-font-mono text-dim">{{ row.uid || row.Uid || '-' }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="uptime" :label="isZh ? '运行时长' : 'Uptime'" width="130" align="right">
              <template #default="{ row }">
                <span class="cs-font-mono">{{ formatUptime(row.uptime, row) }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="is_active" :label="isZh ? '状态' : 'State'" width="110" align="center">
              <template #default="{ row }">
                <span class="text-online">● {{ row.is_active ? 'ESTABLISHED' : 'CLOSED' }}</span>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </div>

      <!-- Bottom / Right: Pool Tuning & Worker Thread Pool -->
      <div class="cp-config-panel">
        <div class="cp-panel-header">
          <span class="panel-title">{{ isZh ? '连接池与工作线程配置' : 'Pool Tuning & Worker Thread Pool' }}</span>
        </div>

        <div class="cp-config-grid">
          <div class="cp-form-col">
            <div class="cs-form-row">
              <label class="cs-form-label">{{ isZh ? '工作线程数' : 'Worker Pool Size' }}:</label>
              <div class="input-with-btn">
                <el-input-number v-model="workerCount" :min="4" :max="128" size="small" />
                <button class="cs-btn cs-btn-primary" @click="handleResizeWorkers">{{ isZh ? '应用' : 'Apply Size' }}</button>
              </div>
            </div>

            <div class="cs-form-row" style="margin-top: 10px">
              <label class="cs-form-label">{{ isZh ? '最大连接数' : 'Max Pool Connections' }}:</label>
              <el-input-number v-model="poolConfig.max_connections" :min="50" :max="5000" size="small" />
            </div>

            <div class="cs-form-row" style="margin-top: 10px">
              <label class="cs-form-label">{{ isZh ? '最大空闲时间' : 'Max Idle Duration' }}:</label>
              <el-input v-model="poolConfig.max_idle_time" placeholder="300s" size="small" style="width: 140px" />
            </div>
          </div>

          <div class="cp-stats-col">
            <div class="stat-mini-card">
              <div class="stat-num">{{ poolStats.active_connections || connections.length }}</div>
              <div class="stat-lbl">{{ isZh ? '活动 TCP/HTTP 连接' : 'Active TCP/HTTP Sockets' }}</div>
            </div>
            <div class="stat-mini-card">
              <div class="stat-num">{{ workerStats.worker_count || workerCount }}</div>
              <div class="stat-lbl">{{ isZh ? '专用工作线程数' : 'Dedicated Worker Threads' }}</div>
            </div>
            <div class="stat-mini-card">
              <div class="stat-num">{{ poolStats.health_status || 'HEALTHY' }}</div>
              <div class="stat-lbl">{{ isZh ? '连接池健康状态' : 'Pool Health State' }}</div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Bottom Action Bar (CS Native Button Row) -->
    <div class="cp-bottom-bar">
      <button class="cs-btn cs-btn-primary" @click="handleSavePoolConfig">
        {{ isZh ? '保存连接池参数' : 'Save Pool Tuning' }}
      </button>
      <button class="cs-btn cs-btn-danger" :disabled="!selectedConnection" @click="selectedConnection && handleCloseConn(selectedConnection)">
        {{ isZh ? '强制断开连接' : 'Terminate Connection' }}
      </button>
      <button class="cs-btn" @click="handleHealthCheck">
        {{ isZh ? '执行健康检查' : 'Perform Health Check' }}
      </button>
      <button class="cs-btn" @click="handleHelp">
        {{ isZh ? '帮助' : 'Help' }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import ConnectionAPI from '@/api/connection'
import { useI18nStore } from '@/stores/i18n'

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

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
      Object.assign(poolStats, pRes.value.data.data)
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
  try {
    await ConnectionAPI.resizeWorkerPool(workerCount.value)
    ElMessage.success(isZh.value ? `工作线程池已调整为 ${workerCount.value}` : `Worker thread pool resized to ${workerCount.value}`)
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '调整工作线程失败' : 'Failed to resize worker pool'))
  }
}

async function handleSavePoolConfig() {
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

.cp-toolbar {
  height: 32px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
  flex-shrink: 0;
}

.cp-title-group {
  display: flex;
  align-items: center;
  gap: 6px;
}

.cp-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.cp-stats {
  font-size: 11px;
  color: var(--cs-text-dim);
}

.cp-spacer { flex: 1; }

.cp-body {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  gap: 1px;
  background: var(--cs-border);
}

.cp-table-panel {
  flex: 1;
  background: var(--cs-bg);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.cp-config-panel {
  height: 170px;
  background: var(--cs-panel);
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
}

.cp-panel-header {
  height: 26px;
  background: var(--cs-titlebar-bg);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 10px;
  font-size: 11px;
  font-weight: 600;
  color: var(--cs-text-dim);
}

.panel-title {
  color: var(--cs-text-bright);
}

.cp-filter-group {
  display: flex;
  align-items: center;
  gap: 6px;
}

.cp-uid-input {
  width: 170px;
}

.cp-table-wrap {
  flex: 1;
  overflow: hidden;
}

.cp-config-grid {
  flex: 1;
  display: flex;
  padding: 10px 14px;
  gap: 20px;
  overflow-y: auto;
}

.cp-form-col {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.cp-stats-col {
  width: 260px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.cs-form-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.cs-form-label {
  font-size: 11px;
  color: var(--cs-text);
  min-width: 180px;
}

.input-with-btn {
  display: flex;
  gap: 6px;
}

.stat-mini-card {
  background: var(--cs-bg);
  border: 1px solid var(--cs-border);
  padding: 6px 10px;
  display: flex;
  align-items: baseline;
  justify-content: space-between;
}

.stat-num {
  font-size: 14px;
  font-weight: 700;
  color: var(--cs-active);
  font-family: var(--cs-font-mono);
}

.stat-lbl {
  font-size: 11px;
  color: var(--cs-text-dim);
}

.text-online { color: var(--cs-online); }
.text-dim { color: var(--cs-text-dim); }

.cp-bottom-bar {
  height: 38px;
  background: var(--cs-panel);
  border-top: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 0 12px;
  flex-shrink: 0;
}
</style>
