<template>
  <div class="cs-server-status-view cs-font-ui">
    <div class="sv-toolbar">
      <div class="sv-title-group">
        <span class="sv-title">{{ isZh ? '服务器状态' : 'Server Status' }}</span>
        <span class="sv-count">{{ isZh ? '实时系统与连接统计' : 'Realtime system & connection stats' }}</span>
      </div>
      <div class="sv-spacer" />
      <button class="cs-btn" :disabled="loading" @click="fetchStats">{{ isZh ? '刷新' : 'Refresh' }}</button>
    </div>

    <div class="sv-body" v-loading="loading">
      <div class="sv-stats-grid">
        <div class="sv-stat-card">
          <div class="sv-stat-num">{{ stats.monitor_count }}</div>
          <div class="sv-stat-lbl">{{ isZh ? '活跃监听器' : 'Active Listeners' }}</div>
        </div>
        <div class="sv-stat-card">
          <div class="sv-stat-num">{{ stats.connection_count }}</div>
          <div class="sv-stat-lbl">{{ isZh ? '在线连接' : 'Online Connections' }}</div>
        </div>
        <div class="sv-stat-card">
          <div class="sv-stat-num">{{ stats.online_user_count }} / {{ stats.user_count }}</div>
          <div class="sv-stat-lbl">{{ isZh ? '在线 / 总会话' : 'Online / Total Clients' }}</div>
        </div>
        <div class="sv-stat-card">
          <div class="sv-stat-num">{{ stats.system_info?.cpu_usage?.toFixed?.(1) || '0.0' }}%</div>
          <div class="sv-stat-lbl">CPU</div>
        </div>
        <div class="sv-stat-card">
          <div class="sv-stat-num">{{ stats.system_info?.memory_usage?.toFixed?.(1) || '0.0' }}%</div>
          <div class="sv-stat-lbl">{{ isZh ? '内存' : 'Memory' }}</div>
        </div>
        <div class="sv-stat-card">
          <div class="sv-stat-num">{{ stats.system_info?.disk_usage?.toFixed?.(1) || '0.0' }}%</div>
          <div class="sv-stat-lbl">{{ isZh ? '磁盘' : 'Disk' }}</div>
        </div>
        <div class="sv-stat-card">
          <div class="sv-stat-num">{{ stats.system_info?.tcp_connections || 0 }}</div>
          <div class="sv-stat-lbl">TCP</div>
        </div>
        <div class="sv-stat-card">
          <div class="sv-stat-num">{{ stats.system_info?.udp_connections || 0 }}</div>
          <div class="sv-stat-lbl">UDP</div>
        </div>
        <div class="sv-stat-card">
          <div class="sv-stat-num">{{ stats.system_info?.inbound_bw || '-' }}</div>
          <div class="sv-stat-lbl">{{ isZh ? '入站带宽' : 'Inbound BW' }}</div>
        </div>
        <div class="sv-stat-card">
          <div class="sv-stat-num">{{ stats.system_info?.outbound_bw || '-' }}</div>
          <div class="sv-stat-lbl">{{ isZh ? '出站带宽' : 'Outbound BW' }}</div>
        </div>
      </div>

      <div class="sv-config-panel">
        <div class="sv-panel-header">{{ isZh ? '配置信息' : 'Configuration' }}</div>
        <div class="sv-config-grid">
          <div><b>{{ isZh ? '版本' : 'Version' }}:</b> {{ stats.config_info?.version || '-' }}</div>
          <div><b>{{ isZh ? '日志级别' : 'Log Level' }}:</b> {{ stats.config_info?.log_level || '-' }}</div>
          <div><b>{{ isZh ? 'Web 端口' : 'Web Port' }}:</b> {{ stats.config_info?.web_port || '-' }}</div>
          <div><b>{{ isZh ? '日志路径' : 'Log Path' }}:</b> {{ stats.config_info?.log_file_path || '-' }}</div>
          <div><b>Basic Auth:</b> {{ stats.config_info?.basic_auth ? 'On' : 'Off' }}</div>
        </div>
      </div>

      <!-- 维护操作（后端 /client/reset-id：重排 sessions 自增 ID 序列） -->
      <div class="sv-config-panel">
        <div class="sv-panel-header">{{ isZh ? '维护操作' : 'Maintenance' }}</div>
        <div class="sv-maintain-row">
          <span class="sv-maintain-desc">
            {{ isZh ? '将 sessions 表自增 ID 序列重置为当前最大 ID，下一次上线从 max+1 继续编号。' : 'Reset the sessions AUTOINCREMENT sequence to the current max ID; the next beacon continues from max+1.' }}
          </span>
          <button class="cs-btn cs-btn-danger" :disabled="resettingId" @click="handleResetClientId">
            {{ resettingId ? (isZh ? '重置中...' : 'Resetting...') : (isZh ? '重置会话 ID 序列' : 'Reset Session ID Sequence') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getDashboardStats, type DashboardStats } from '@/api/dashboard'
import ClientAPI from '@/api/clients'
import { useI18nStore } from '@/stores/i18n'

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const loading = ref(false)
const resettingId = ref(false)
const stats = reactive<Partial<DashboardStats>>({})

async function handleResetClientId() {
  ElMessageBox.confirm(
    isZh.value
      ? '重置 sessions 表自增 ID 序列？该操作不会删除任何会话记录，仅让下一次上线从当前 max+1 开始编号。'
      : 'Reset the sessions AUTOINCREMENT sequence? No session records are deleted; the next beacon starts numbering from current max+1.',
    isZh.value ? '重置会话 ID 序列' : 'Reset Session ID Sequence',
    {
      confirmButtonText: isZh.value ? '重置' : 'Reset',
      cancelButtonText: isZh.value ? '取消' : 'Cancel',
      type: 'warning',
    },
  ).then(async () => {
    resettingId.value = true
    try {
      const res = await ClientAPI.reset_client_id()
      const data = res.data?.data || {}
      ElMessage.success(
        isZh.value
          ? `ID 序列已重置：当前最大 ${data.current_max_id ?? '-'}，下一个 ID ${data.next_id ?? '-'}`
          : `ID sequence reset: current max ${data.current_max_id ?? '-'}, next ID ${data.next_id ?? '-'}`,
      )
    } catch (e: any) {
      ElMessage.error(e?.message || (isZh.value ? '重置 ID 序列失败' : 'Failed to reset ID sequence'))
    } finally {
      resettingId.value = false
    }
  }).catch(() => {})
}

async function fetchStats() {
  loading.value = true
  try {
    const res = await getDashboardStats()
    const data = res.data?.data || res.data
    if (data) Object.assign(stats, data)
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '获取服务器状态失败' : 'Failed to fetch server status'))
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchStats()
})
</script>

<style scoped>
.cs-server-status-view {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-bg);
  overflow: hidden;
}
.sv-toolbar {
  height: 32px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
  flex-shrink: 0;
}
.sv-title-group { display: flex; align-items: center; gap: 6px; }
.sv-title { font-size: 12px; font-weight: 600; color: var(--cs-text-bright); }
.sv-count { font-size: 11px; color: var(--cs-text-dim); }
.sv-spacer { flex: 1; }
.sv-body { flex: 1; overflow-y: auto; padding: 12px; display: flex; flex-direction: column; gap: 12px; }
.sv-stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: 10px;
}
.sv-stat-card {
  background: var(--cs-panel);
  border: 1px solid var(--cs-border);
  border-radius: 2px;
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.sv-stat-num { font-size: 22px; font-weight: 600; color: var(--cs-active); font-family: var(--cs-font-mono); }
.sv-stat-lbl { font-size: 11px; color: var(--cs-text-dim); }
.sv-config-panel { background: var(--cs-panel); border: 1px solid var(--cs-border); border-radius: 2px; padding: 10px 12px; }
.sv-panel-header { font-size: 12px; font-weight: 600; color: var(--cs-text-bright); margin-bottom: 8px; }
.sv-config-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); gap: 6px 16px; font-size: 12px; color: var(--cs-text); }
.sv-config-grid b { color: var(--cs-text-bright); }
.sv-maintain-row { display: flex; align-items: center; gap: 16px; }
.sv-maintain-desc { flex: 1; font-size: 12px; color: var(--cs-text); line-height: 1.6; }
</style>
