<template>
  <div class="vs-dashboard">
    <!-- 统计卡行 -->
    <div class="vs-dash-cards">
      <div class="vs-card">
        <div class="vs-stat-label">{{ isZh ? '客户端总数' : 'Total Clients' }}</div>
        <div class="vs-stat-value">{{ stats?.user_count ?? '-' }}</div>
      </div>
      <div class="vs-card">
        <div class="vs-stat-label">{{ isZh ? '在线客户端' : 'Online' }}</div>
        <div class="vs-stat-value" style="color: var(--cs-online)">{{ stats?.online_user_count ?? '-' }}</div>
      </div>
      <div class="vs-card">
        <div class="vs-stat-label">{{ isZh ? '活跃监听器' : 'Listeners' }}</div>
        <div class="vs-stat-value">{{ stats?.monitor_count ?? '-' }}</div>
      </div>
      <div class="vs-card">
        <div class="vs-stat-label">{{ isZh ? '服务端口' : 'Web Port' }}</div>
        <div class="vs-stat-value">
          {{ stats?.config_info?.web_port ?? '-' }}
          <span class="unit">v{{ stats?.config_info?.version || '' }}</span>
        </div>
      </div>
    </div>

    <!-- 资源仪表 + 连接信息 -->
    <div class="vs-dash-row">
      <div class="vs-card">
        <div class="vs-dash-title">{{ isZh ? '服务器资源' : 'Server Resources' }}</div>
        <div class="vs-gauge-wrap">
          <div class="vs-gauge">
            <div class="vs-gauge-ring" :style="ringStyle(cpu, '#0960bd')">
              <span class="val">{{ fmt(cpu) }}%</span>
            </div>
            <span class="vs-gauge-name">CPU</span>
          </div>
          <div class="vs-gauge">
            <div class="vs-gauge-ring" :style="ringStyle(mem, '#4caf50')">
              <span class="val">{{ fmt(mem) }}%</span>
            </div>
            <span class="vs-gauge-name">{{ isZh ? '内存' : 'Memory' }}</span>
          </div>
          <div class="vs-gauge">
            <div class="vs-gauge-ring" :style="ringStyle(disk, '#ff9800')">
              <span class="val">{{ fmt(disk) }}%</span>
            </div>
            <span class="vs-gauge-name">{{ isZh ? '磁盘' : 'Disk' }}</span>
          </div>
        </div>
      </div>

      <div class="vs-card">
        <div class="vs-dash-title">{{ isZh ? '连接与带宽' : 'Connections & Bandwidth' }}</div>
        <div class="vs-kv-list">
          <div class="vs-kv"><span>{{ isZh ? 'TCP 连接' : 'TCP Conns' }}</span><b>{{ sys?.tcp_connections ?? '-' }}</b></div>
          <div class="vs-kv"><span>{{ isZh ? 'UDP 连接' : 'UDP Conns' }}</span><b>{{ sys?.udp_connections ?? '-' }}</b></div>
          <div class="vs-kv"><span>{{ isZh ? '流入带宽' : 'Inbound' }}</span><b>{{ sys?.inbound_bw || '-' }}</b></div>
          <div class="vs-kv"><span>{{ isZh ? '流出带宽' : 'Outbound' }}</span><b>{{ sys?.outbound_bw || '-' }}</b></div>
          <div class="vs-kv"><span>{{ isZh ? '运行日志级别' : 'Log Level' }}</span><b>{{ stats?.config_info?.log_level || '-' }}</b></div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { getDashboardStats } from '@/api/dashboard'
import { useI18nStore } from '@/stores/i18n'

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

interface DashStats {
  monitor_count?: number
  connection_count?: number
  user_count?: number
  online_user_count?: number
  config_info?: { log_level?: string; web_port?: number; version?: string }
  system_info?: {
    cpu_usage?: number
    disk_usage?: number
    memory_usage?: number
    tcp_connections?: number
    udp_connections?: number
    outbound_bw?: string
    inbound_bw?: string
  }
}

const stats = ref<DashStats | null>(null)
let timer: number | null = null

const sys = computed(() => stats.value?.system_info)
const cpu = computed(() => Math.min(100, Math.max(0, sys.value?.cpu_usage ?? 0)))
const mem = computed(() => Math.min(100, Math.max(0, sys.value?.memory_usage ?? 0)))
const disk = computed(() => Math.min(100, Math.max(0, sys.value?.disk_usage ?? 0)))

function fmt(v: number) {
  return v.toFixed(0)
}

function ringStyle(pct: number, color: string) {
  return {
    '--pct': String(pct),
    '--ring-color': pct > 85 ? 'var(--cs-error)' : color,
  }
}

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
.vs-kv-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding-top: 2px;
}
.vs-kv {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 13px;
  color: var(--vs-text-normal);
}
.vs-kv b {
  color: var(--vs-text-strong);
  font-weight: 600;
  font-family: var(--cs-font-mono);
}
</style>
