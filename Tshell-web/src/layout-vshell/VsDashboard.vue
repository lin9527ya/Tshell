<template>
  <div class="vs-dash">
    <!-- 统计卡行（vshell: 蓝色标题链接 + 右侧彩色大图标） -->
    <div class="vs-dash-cards">
      <div class="vs-card vs-stat-card">
        <div class="vs-stat-title">{{ isZh ? '监听总数' : 'Listeners' }}</div>
        <div class="vs-stat-row">
          <span class="vs-stat-value">{{ stats?.monitor_count ?? 0 }}</span>
          <VsIcon name="magic" :size="40" />
        </div>
      </div>
      <div class="vs-card vs-stat-card">
        <div class="vs-stat-title">{{ isZh ? '在线监听数' : 'Online Listeners' }}</div>
        <div class="vs-stat-row">
          <span class="vs-stat-value">{{ stats?.connection_count ?? 0 }}</span>
          <VsIcon name="headset" :size="40" />
        </div>
      </div>
      <div class="vs-card vs-stat-card">
        <div class="vs-stat-title">{{ isZh ? '客户端总数' : 'Clients' }}</div>
        <div class="vs-stat-row">
          <span class="vs-stat-value">{{ stats?.user_count ?? 0 }}</span>
          <VsIcon name="responsive" :size="40" />
        </div>
      </div>
      <div class="vs-card vs-stat-card">
        <div class="vs-stat-title">{{ isZh ? '在线客户端' : 'Online Clients' }}</div>
        <div class="vs-stat-row">
          <span class="vs-stat-value">{{ stats?.online_user_count ?? 0 }}</span>
          <VsIcon name="pc" :size="40" />
        </div>
      </div>
    </div>

    <!-- 配置信息 / 系统信息 -->
    <div class="vs-dash-panels">
      <div class="vs-card vs-panel">
        <div class="vs-panel-head">{{ isZh ? '配置信息' : 'Config' }}</div>
        <div class="vs-panel-body">
          <div class="vs-kv">
            <span>{{ isZh ? '日志级别' : 'Log level' }}</span>
            <b>{{ cfg?.log_level || '-' }}</b>
          </div>
          <div class="vs-kv">
            <span>{{ isZh ? '日志文件' : 'Log file' }}</span>
            <b class="ellipsis">{{ cfg?.log_file_path || '-' }}</b>
          </div>
          <div class="vs-kv">
            <span>{{ isZh ? 'WEB端口' : 'WEB port' }}</span>
            <b>{{ cfg?.web_port ?? '-' }}</b>
          </div>
          <div class="vs-kv">
            <span>{{ isZh ? 'Basic认证' : 'Basic auth' }}</span>
            <b>{{ cfg?.basic_auth ? (isZh ? '已开启' : 'On') : (isZh ? '未开启' : 'Off') }}</b>
          </div>
          <div class="vs-kv">
            <span>{{ isZh ? '版本' : 'Version' }}</span>
            <b>{{ cfg?.version || '-' }}</b>
          </div>
        </div>
      </div>

      <div class="vs-card vs-panel">
        <div class="vs-panel-head">{{ isZh ? '系统信息' : 'System' }}</div>
        <div class="vs-panel-body">
          <div v-for="g in gauges" :key="g.key" class="vs-gauge-row">
            <span class="vs-gauge-label">{{ g.label }}</span>
            <div class="vs-gauge-track">
              <div class="vs-gauge-fill" :style="{ width: g.value + '%' }" />
            </div>
            <span class="vs-gauge-val">{{ g.value.toFixed(0) }}%</span>
          </div>

          <div class="vs-kv">
            <span>{{ isZh ? 'TCP 通讯 (已建立)' : 'TCP (established)' }}</span>
            <b>{{ sys?.tcp_connections ?? 0 }}</b>
          </div>
          <div class="vs-kv">
            <span>{{ isZh ? 'UDP 通讯 (已建立)' : 'UDP (established)' }}</span>
            <b>{{ sys?.udp_connections ?? 0 }}</b>
          </div>
          <div class="vs-kv">
            <span>{{ isZh ? '流出带宽' : 'Outbound' }}</span>
            <b>{{ sys?.outbound_bw || '-' }}</b>
          </div>
          <div class="vs-kv">
            <span>{{ isZh ? '流入带宽' : 'Inbound' }}</span>
            <b>{{ sys?.inbound_bw || '-' }}</b>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, defineOptions, onBeforeUnmount, onMounted, ref } from 'vue'
import VsIcon from './VsIcon.vue'
import { getDashboardStats } from '@/api/dashboard'
import { useI18nStore } from '@/stores/i18n'

defineOptions({ name: 'Dashboard' })

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

interface DashStats {
  monitor_count?: number
  connection_count?: number
  user_count?: number
  online_user_count?: number
  config_info?: {
    log_level?: string
    log_file_path?: string
    web_port?: number
    basic_auth?: boolean
    version?: string
  }
  system_info?: {
    cpu_usage?: number
    disk_usage?: number
    memory_usage?: number
    virtual_memory?: number
    tcp_connections?: number
    udp_connections?: number
    outbound_bw?: string
    inbound_bw?: string
  }
}

const stats = ref<DashStats | null>(null)
let timer: number | null = null

const cfg = computed(() => stats.value?.config_info)
const sys = computed(() => stats.value?.system_info)

const clamp = (v?: number) => Math.min(100, Math.max(0, v ?? 0))

const gauges = computed(() => [
  { key: 'cpu', label: isZh.value ? '处理器' : 'CPU', value: clamp(sys.value?.cpu_usage) },
  { key: 'disk', label: isZh.value ? '磁盘' : 'Disk', value: clamp(sys.value?.disk_usage) },
  { key: 'mem', label: isZh.value ? '内存' : 'Memory', value: clamp(sys.value?.memory_usage) },
  { key: 'vmem', label: isZh.value ? '虚拟内存' : 'Virtual memory', value: clamp(sys.value?.virtual_memory) },
])

async function fetchStats() {
  try {
    const res = await getDashboardStats()
    stats.value = (res.data?.data || res.data) as DashStats
  } catch {
    /* 仪表盘失败静默,保留上次数据 */
  }
}

onMounted(() => {
  fetchStats()
  timer = window.setInterval(() => {
    if (document.visibilityState !== 'hidden') fetchStats()
  }, 10000) // 与后端 10s 预采 TTL 对齐
})

onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})
</script>

<style scoped>
/* vshell 概览实测：卡片 8px 圆角 / 1px #f0f0f0 边框 / 内容区 padding 16px / 卡头 38px
   自适应：宽度用 4 列网格随视口伸缩；高度上「面板行」撑满剩余空间，避免大屏下方大片空白 */
.vs-dash {
  padding: 16px;
  display: grid;
  grid-template-rows: auto minmax(0, 1fr);
  gap: 16px;
  min-height: 100%;
  box-sizing: border-box;
}

.vs-card {
  background: var(--cs-card-bg);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
}

.vs-dash-cards {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
}

@media (max-width: 1100px) {
  .vs-dash-cards {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

.vs-stat-card {
  padding: 12px;
  min-height: 130px;
  display: flex;
  flex-direction: column;
}

.vs-stat-title {
  font-size: 14px;
  color: var(--cs-active);
  cursor: pointer;
}

.vs-stat-row {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 0;
}

.vs-stat-value {
  font-size: 24px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.vs-dash-panels {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
  align-items: stretch;
  min-height: 0;
}

/* 面板卡片撑满剩余高度（大屏不留大片空白），内容超高时面板内滚动 */
.vs-panel {
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}

@media (max-width: 1100px) {
  .vs-dash-panels {
    grid-template-columns: minmax(0, 1fr);
  }
}

.vs-panel-head {
  height: 38px;
  display: flex;
  align-items: center;
  padding: 0 12px;
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
  border-bottom: 1px solid var(--cs-card-border);
}

.vs-panel-body {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.vs-kv {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  font-size: 14px;
  color: var(--cs-text);
  min-height: 30px;
}

.vs-kv b {
  font-weight: 400;
  color: var(--cs-text-bright);
  font-family: var(--cs-font-mono);
  text-align: right;
}

.vs-kv b.ellipsis {
  max-width: 60%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.vs-gauge-row {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 30px;
  font-size: 14px;
  color: var(--cs-text);
}

.vs-gauge-label {
  width: 76px;
  flex-shrink: 0;
}

.vs-gauge-track {
  flex: 1;
  height: 8px;
  border-radius: 100px;
  background: var(--cs-hover);
  overflow: hidden;
}

.vs-gauge-fill {
  height: 100%;
  border-radius: 100px;
  background: var(--cs-active);
  transition: width 0.3s ease;
}

.vs-gauge-val {
  width: 44px;
  text-align: right;
  flex-shrink: 0;
  color: var(--cs-text);
}
</style>
