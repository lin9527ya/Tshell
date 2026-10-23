<template>
  <div class="cs-server-status-view cs-font-ui">
    <div class="sv-toolbar">
      <div class="sv-title-group">
        <span class="sv-title">{{ i18nStore.tl('服务器状态', 'Server Status') }}</span>
        <span class="sv-count">{{ i18nStore.tl('实时系统与连接统计', 'Realtime system & connection stats') }}</span>
      </div>
      <div class="sv-spacer" />
      <button class="cs-btn" :disabled="loading" @click="fetchStats">{{ i18nStore.tl('刷新', 'Refresh') }}</button>
    </div>

    <div class="sv-body" v-loading="loading">
      <div class="sv-stats-grid">
        <div class="sv-stat-card">
          <div class="sv-stat-num">{{ stats.monitor_count }}</div>
          <div class="sv-stat-lbl">{{ i18nStore.tl('活跃监听器', 'Active Listeners') }}</div>
        </div>
        <div class="sv-stat-card">
          <div class="sv-stat-num">{{ stats.connection_count }}</div>
          <div class="sv-stat-lbl">{{ i18nStore.tl('在线连接', 'Online Connections') }}</div>
        </div>
        <div class="sv-stat-card">
          <div class="sv-stat-num">{{ stats.online_user_count }} / {{ stats.user_count }}</div>
          <div class="sv-stat-lbl">{{ i18nStore.tl('在线 / 总会话', 'Online / Total Clients') }}</div>
        </div>
        <div class="sv-stat-card">
          <div class="sv-stat-num">{{ stats.system_info?.cpu_usage?.toFixed?.(1) || '0.0' }}%</div>
          <div class="sv-stat-lbl">CPU</div>
        </div>
        <div class="sv-stat-card">
          <div class="sv-stat-num">{{ stats.system_info?.memory_usage?.toFixed?.(1) || '0.0' }}%</div>
          <div class="sv-stat-lbl">{{ i18nStore.tl('内存', 'Memory') }}</div>
        </div>
        <div class="sv-stat-card">
          <div class="sv-stat-num">{{ stats.system_info?.disk_usage?.toFixed?.(1) || '0.0' }}%</div>
          <div class="sv-stat-lbl">{{ i18nStore.tl('磁盘', 'Disk') }}</div>
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
          <div class="sv-stat-lbl">{{ i18nStore.tl('入站带宽', 'Inbound BW') }}</div>
        </div>
        <div class="sv-stat-card">
          <div class="sv-stat-num">{{ stats.system_info?.outbound_bw || '-' }}</div>
          <div class="sv-stat-lbl">{{ i18nStore.tl('出站带宽', 'Outbound BW') }}</div>
        </div>
      </div>

      <div class="sv-config-panel">
        <div class="sv-panel-header">{{ i18nStore.tl('配置信息', 'Configuration') }}</div>
        <div class="sv-panel-body">
          <div class="sv-config-grid">
            <div><b>{{ i18nStore.tl('版本', 'Version') }}:</b> {{ stats.config_info?.version || '-' }}</div>
            <div><b>{{ i18nStore.tl('日志级别', 'Log Level') }}:</b> {{ stats.config_info?.log_level || '-' }}</div>
            <div><b>{{ i18nStore.tl('Web 端口', 'Web Port') }}:</b> {{ stats.config_info?.web_port || '-' }}</div>
            <div><b>{{ i18nStore.tl('日志路径', 'Log Path') }}:</b> {{ stats.config_info?.log_file_path || '-' }}</div>
            <div><b>Basic Auth:</b> {{ stats.config_info?.basic_auth ? 'On' : 'Off' }}</div>
          </div>
        </div>
      </div>

      <!-- 维护操作（后端 /client/reset-id：把 clients 表主键重排为 1..N 并复位自增） -->
      <div v-if="canOperate" class="sv-config-panel">
        <div class="sv-panel-header">{{ i18nStore.tl('维护操作', 'Maintenance') }}</div>
        <div class="sv-panel-body">
          <div class="sv-maintain-row">
            <span class="sv-maintain-desc">
              {{ i18nStore.tl('把客户端编号按上线顺序重排为 1、2、3……，下一个新会话从 N+1 开始。只动编号，会话本身（Uid、命令、审计）不受影响；但历史记录里按旧编号写的对应关系会对不上，且无法自动还原。', 'Renumber clients to 1, 2, 3… in connect order; the next new session starts at N+1. Only the numbers change — sessions (Uid, commands, audit) are untouched, but historical references to old numbers will no longer match, and this cannot be undone automatically.') }}
            </span>
            <button class="cs-btn cs-btn-danger" :disabled="resettingId" @click="handleResetClientId">
              {{ resettingId ? (i18nStore.tl('重置中...', 'Resetting...')) : (i18nStore.tl('重置会话编号', 'Renumber Sessions')) }}
            </button>
          </div>
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
import { useUserStore } from '@/stores/user'

const i18nStore = useI18nStore()
const userStore = useUserStore()
const isZh = computed(() => i18nStore.isZh)
// 维护写接口是 operatorOnly；auditor 只展示状态
const canOperate = computed(() => String(userStore.getRoles() || '').toLowerCase() !== 'auditor')

const loading = ref(false)
const resettingId = ref(false)
const stats = reactive<Partial<DashboardStats>>({})

async function handleResetClientId() {
  if (!canOperate.value) {
    ElMessage.warning(isZh.value ? '只读账号无法执行维护操作' : 'Read-only account cannot run maintenance')
    return
  }
  ElMessageBox.confirm(
    isZh.value
      ? '把客户端编号按上线顺序重排为 1..N？只改编号，不删会话；历史记录里按旧编号的对应关系会对不上，且无法自动还原。'
      : 'Renumber clients 1..N in connect order? Sessions are not deleted; historical references to old numbers will no longer match, and this cannot be undone automatically.',
    isZh.value ? '重置会话编号' : 'Renumber Sessions',
    {
      confirmButtonText: isZh.value ? '重排' : 'Renumber',
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
          ? `已重排 ${data.renumbered ?? 0} 个会话，下一个编号 ${data.next_id ?? '-'}`
          : `Renumbered ${data.renumbered ?? 0} sessions; next number ${data.next_id ?? '-'}`,
      )
    } catch (e: any) {
      ElMessage.error(e?.message || (isZh.value ? '重置会话编号失败' : 'Failed to renumber sessions'))
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
  height: 48px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-card-border);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 16px;
  flex-shrink: 0;
}
.sv-title-group { display: flex; align-items: baseline; gap: 8px; }
.sv-title { font-size: 14px; font-weight: 600; color: var(--cs-text-bright); }
.sv-count { font-size: 12px; color: var(--cs-text-dim); }
.sv-spacer { flex: 1; }
.sv-body { flex: 1; overflow-y: auto; padding: 16px; display: flex; flex-direction: column; gap: 16px; }
.sv-stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(230px, 1fr));
  gap: 12px;
}
.sv-stat-card {
  background: var(--cs-panel);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.sv-stat-num { font-size: 22px; font-weight: 600; color: var(--cs-active); font-family: var(--cs-font-mono); }
.sv-stat-lbl { font-size: 12px; color: var(--cs-text-dim); }
.sv-config-panel {
  background: var(--cs-panel);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  overflow: hidden;
}
.sv-panel-header {
  height: 38px;
  display: flex;
  align-items: center;
  padding: 0 16px;
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
  border-bottom: 1px solid var(--cs-card-border);
}
.sv-panel-body { padding: 16px; }
.sv-config-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); gap: 8px 16px; font-size: 14px; color: var(--cs-text); }
.sv-config-grid b { color: var(--cs-text-bright); font-weight: 600; }
.sv-maintain-row { display: flex; align-items: center; gap: 16px; }
.sv-maintain-desc { flex: 1; font-size: 13px; color: var(--cs-text-dim); line-height: 1.6; }
</style>
