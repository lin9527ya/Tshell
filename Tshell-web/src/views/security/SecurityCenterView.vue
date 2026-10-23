<template>
  <div class="cs-security-center-view cs-font-ui">
    <!-- 工具条：左侧标题，右侧操作 -->
    <div class="sc-toolbar">
      <div class="sc-title-group">
        <span class="sc-title">{{ i18nStore.tl('安全中心', 'Security Center') }}</span>
        <span class="sc-stats">
          ({{ whitelist.length }} {{ i18nStore.tl('个白名单', 'whitelisted') }} / {{ blockedIPs.length }} {{ i18nStore.tl('个封禁', 'blocked') }})
        </span>
      </div>

      <div class="sc-spacer" />

      <button class="cs-btn cs-btn-primary" @click="showAddWhitelistDialog = true">
        {{ i18nStore.tl('添加白名单 IP', 'Add Whitelist IP') }}
      </button>
      <button class="cs-btn" :disabled="!selectedBlockedRow" @click="selectedBlockedRow && handleUnblock(selectedBlockedRow)">
        {{ i18nStore.tl('解除封禁', 'Unblock IP') }}
      </button>
      <button class="cs-btn" @click="handleHelp">
        {{ i18nStore.tl('帮助', 'Help') }}
      </button>
      <button class="cs-btn" :disabled="loading" @click="fetchData">
        {{ i18nStore.tl('刷新', 'Refresh') }}
      </button>
    </div>

    <!-- Master-Detail Body -->
    <div class="sc-body">
      <!-- Left Navigation -->
      <div class="sc-sidebar">
        <div
          v-for="t in tabs"
          :key="t.id"
          class="sc-nav-item"
          :class="{ active: currentTab === t.id }"
          @click="currentTab = t.id"
        >
          <span class="nav-icon">{{ t.icon }}</span>
          <span class="nav-label">{{ t.name }}</span>
        </div>
      </div>

      <!-- Right Content Panel -->
      <div class="sc-content">
        <!-- 1. IP 白名单 (IP Whitelist) -->
        <div v-if="currentTab === 'whitelist'" class="sc-panel">
          <div class="sc-panel-header">
            <span class="panel-title">{{ i18nStore.tl('可信 IP 白名单', 'Trusted IP Whitelist') }}</span>
            <button class="cs-btn cs-btn-primary" @click="showAddWhitelistDialog = true">{{ i18nStore.tl('添加 IP / CIDR', 'Add IP / CIDR') }}</button>
          </div>
          <div class="sc-table-wrap">
            <el-table
              :data="whitelist"
              height="100%"
              highlight-current-row
              class="cs-session-table"
              @current-change="(r: any) => selectedWhitelistRow = r"
            >
              <el-table-column prop="ip" :label="i18nStore.tl('IP 地址 / 网段', 'IP Address / Network')" min-width="220" sortable show-overflow-tooltip>
                <template #default="{ row }">
                  <span class="cs-font-mono font-bold">{{ row.ip }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="network" :label="i18nStore.tl('子网掩码', 'Subnet Mask')" width="160" show-overflow-tooltip />
              <el-table-column prop="created_at" :label="i18nStore.tl('添加时间', 'Added When')" width="180" show-overflow-tooltip />
              <el-table-column :label="i18nStore.tl('操作', 'Actions')" width="110" align="center">
                <template #default="{ row }">
                  <button class="cs-btn cs-btn-xs cs-btn-danger" @click="handleRemoveWhitelist(row)">{{ i18nStore.tl('移除', 'Remove') }}</button>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </div>

        <!-- 2. 封禁 IP (Blocked IPs) -->
        <div v-else-if="currentTab === 'blocked'" class="sc-panel">
          <div class="sc-panel-header">
            <span class="panel-title">{{ i18nStore.tl('恶意封禁 IP 防御列表', 'Blocked IP Defense List') }}</span>
            <button class="cs-btn cs-btn-danger" :disabled="!selectedBlockedRow" @click="selectedBlockedRow && handleUnblock(selectedBlockedRow)">
              {{ i18nStore.tl('解除选中封禁', 'Unblock Selected') }}
            </button>
          </div>
          <div class="sc-table-wrap">
            <el-table
              :data="blockedIPs"
              height="100%"
              highlight-current-row
              class="cs-session-table"
              @current-change="(r: any) => selectedBlockedRow = r"
            >
              <el-table-column prop="ip" :label="i18nStore.tl('封禁 IP', 'Blocked IP')" min-width="200" sortable show-overflow-tooltip>
                <template #default="{ row }">
                  <span class="cs-font-mono text-danger font-bold">{{ row.ip }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="reason" :label="i18nStore.tl('封禁原因', 'Block Reason')" min-width="180" show-overflow-tooltip />
              <el-table-column prop="blocked_at" :label="i18nStore.tl('封禁时间', 'Blocked At')" width="170" show-overflow-tooltip>
                <template #default="{ row }">
                  <span class="cs-font-mono">{{ row.blocked_at || '-' }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="attempts" :label="i18nStore.tl('失败尝试', 'Attempts')" width="90" align="right" sortable>
                <template #default="{ row }">
                  <span class="cs-font-mono">{{ row.attempts ?? '-' }}</span>
                </template>
              </el-table-column>
              <el-table-column :label="i18nStore.tl('操作', 'Actions')" width="110" align="center">
                <template #default="{ row }">
                  <button class="cs-btn cs-btn-xs" @click="handleUnblock(row)">{{ i18nStore.tl('解除封禁', 'Unblock') }}</button>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </div>

        <!-- 3. 限流与防爆破策略 (Rate Limiting) -->
        <div v-else-if="currentTab === 'rate_limit'" class="sc-panel">
          <div class="sc-panel-header">
            <span class="panel-title">{{ i18nStore.tl('防爆破与请求限流策略', 'Anti-Bruteforce & Rate Limiting Policy') }}</span>
            <button class="cs-btn cs-btn-primary" @click="handleSaveRateLimit">{{ i18nStore.tl('保存策略', 'Save Policy') }}</button>
          </div>

          <div class="sc-panel-body">
            <div class="cs-form-container">
              <div class="cs-form-row">
                <label class="cs-form-label">{{ i18nStore.tl('登录防爆破阈值（次/分钟）', 'Login Max Attempts / min') }}:</label>
                <el-input-number v-model="rateLimitForm.login_rate_limit" :min="1" :max="100" />
              </div>

              <div class="cs-form-row">
                <label class="cs-form-label">{{ i18nStore.tl('API 限流阈值（次/分钟）', 'API Max Requests / min') }}:</label>
                <el-input-number v-model="rateLimitForm.api_rate_limit" :min="10" :max="1000" />
              </div>

              <div class="cs-form-row">
                <label class="cs-form-label">{{ i18nStore.tl('自动封禁时长（分钟）', 'Auto Block Duration in Minutes') }}:</label>
                <el-input-number v-model="rateLimitForm.block_duration" :min="1" :max="1440" />
              </div>
            </div>
          </div>
        </div>

        <!-- 4. 实时防御统计 (Realtime Stats) -->
        <div v-else-if="currentTab === 'stats'" class="sc-panel">
          <div class="sc-panel-header">
            <span class="panel-title">{{ i18nStore.tl('实时威胁防御指标', 'Realtime Threat Defense Metrics') }}</span>
          </div>

          <div class="sc-panel-body">
            <div class="sc-stats-grid">
              <div class="stat-card">
                <div class="stat-num">{{ stats.total_blocked || blockedIPs.length }}</div>
                <div class="stat-lbl">{{ i18nStore.tl('已封禁攻击总数', 'Total Blocked Attacks') }}</div>
              </div>
              <div class="stat-card">
                <div class="stat-num">{{ stats.whitelist_count || whitelist.length }}</div>
                <div class="stat-lbl">{{ i18nStore.tl('白名单条目数', 'Active Whitelist Entries') }}</div>
              </div>
              <div class="stat-card">
                <div class="stat-num">{{ stats.active_defense || 'ON' }}</div>
                <div class="stat-lbl">{{ i18nStore.tl('WAF 与限流状态', 'WAF & Rate Limiter State') }}</div>
              </div>
              <div class="stat-card">
                <div class="stat-num">{{ stats.rate_limit_hits || 0 }}</div>
                <div class="stat-lbl">{{ i18nStore.tl('被限流请求数', 'Rate Limit Throttled Requests') }}</div>
              </div>
            </div>
          </div>
        </div>

        <!-- 5. 近 1 小时实时访问 IP（后端 /security/realtime-stats，5s 自动刷新） -->
        <div v-else class="sc-panel">
          <div class="sc-panel-header">
            <span class="panel-title">{{ i18nStore.tl('近 1 小时活跃访问 IP', 'Active Access IPs (Last 1 Hour)') }}</span>
            <div class="sc-header-actions">
              <span class="sc-rt-hint">{{ realtimeStats.timestamp || (i18nStore.tl('等待数据...', 'Awaiting data...')) }}</span>
              <button class="cs-btn" :disabled="realtimeLoading" @click="fetchRealtimeStats">
                {{ i18nStore.tl('立即刷新', 'Refresh Now') }}
              </button>
            </div>
          </div>

          <div class="sc-panel-body">
            <div class="sc-rt-stats">
              <div class="stat-card">
                <div class="stat-num">{{ realtimeStats.total_active ?? 0 }}</div>
                <div class="stat-lbl">{{ i18nStore.tl('最近活跃来源数', 'Recent Active Sources') }}</div>
              </div>
              <div class="stat-card">
                <div class="stat-num">{{ realtimeStats.timeframe || 'last 1 hour' }}</div>
                <div class="stat-lbl">{{ i18nStore.tl('统计时间窗', 'Timeframe') }}</div>
              </div>
            </div>
          </div>

          <div class="sc-table-wrap">
            <el-table
              :data="realtimeStats.recent_active_ips || []"
              height="100%"
              highlight-current-row
              class="cs-session-table"
              v-loading="realtimeLoading"
            >
              <el-table-column type="index" label="#" width="50" align="center" />
              <el-table-column prop="ip" :label="i18nStore.tl('来源 IP', 'Source IP')" min-width="220" sortable>
                <template #default="{ row }">
                  <span class="cs-font-mono font-bold">{{ row.ip }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="requests" :label="i18nStore.tl('请求次数', 'Requests')" width="120" align="right" sortable>
                <template #default="{ row }">
                  <!-- 后端 GetRecentActiveIPs 的键是 requests（不是 count / request_count） -->
                  <span class="cs-font-mono">{{ row.requests ?? row.count ?? '-' }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="last_seen" :label="i18nStore.tl('最后活跃', 'Last Seen')" width="180" sortable>
                <template #default="{ row }">
                  <span class="cs-font-mono">{{ row.last_seen || row.last_active || '-' }}</span>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </div>
      </div>
    </div>

    <!-- Add Whitelist Dialog -->
    <el-dialog v-model="showAddWhitelistDialog" :title="i18nStore.tl('添加 IP 白名单', 'Add IP Whitelist')" width="420px" class="cs-dialog" append-to-body>
      <div class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('IP 地址 / CIDR', 'IP Address / CIDR') }}:</label>
        <el-input v-model="newWhitelistIP" :placeholder="i18nStore.tl('例如 192.168.1.100 或 10.0.0.0/24', 'e.g. 192.168.1.100 or 10.0.0.0/24')" />
      </div>
      <template #footer>
        <div class="cs-dialog-footer">
          <button class="cs-btn cs-btn-primary" @click="handleAddWhitelist">{{ i18nStore.tl('添加', 'Add') }}</button>
          <button class="cs-btn" @click="showAddWhitelistDialog = false">{{ i18nStore.tl('取消', 'Cancel') }}</button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import SecurityAPI from '@/api/security'
import { useI18nStore } from '@/stores/i18n'

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const currentTab = ref('whitelist')
const loading = ref(false)

const tabs = computed(() => [
  { id: 'whitelist', name: isZh.value ? 'IP 白名单' : 'IP Whitelist', icon: '🛡️' },
  { id: 'blocked', name: isZh.value ? '封禁列表' : 'Blocked IPs', icon: '🚫' },
  { id: 'rate_limit', name: isZh.value ? '限流策略' : 'Rate Limiting', icon: '⚡' },
  { id: 'stats', name: isZh.value ? '防御统计' : 'Defense Stats', icon: '📊' },
  { id: 'realtime', name: isZh.value ? '实时访问' : 'Realtime Access', icon: '🕒' },
])

// 初始为空，真实数据来自后端（失败时保持空 + 报错，不再展示假占位）
const whitelist = ref<any[]>([])

const blockedIPs = ref<any[]>([])

const rateLimitForm = reactive({
  login_rate_limit: 5,
  api_rate_limit: 60,
  block_duration: 30,
})

const stats = reactive<any>({
  total_blocked: 0,
  whitelist_count: 0,
  active_defense: '',
  rate_limit_hits: 0,
})

// 近 1 小时实时访问 IP（/security/realtime-stats）
const realtimeStats = ref<Record<string, any>>({})
const realtimeLoading = ref(false)
let realtimeTimer: ReturnType<typeof setInterval> | null = null

async function fetchRealtimeStats() {
  realtimeLoading.value = true
  try {
    const res = await SecurityAPI.getRealtimeStats()
    if (res.data?.status === 200) {
      realtimeStats.value = res.data.data || {}
    } else {
      realtimeStats.value = {}
    }
  } catch {
    /* 静默失败：轮询接口在切页/服务忙时不应反复弹全局错误 */
  } finally {
    realtimeLoading.value = false
  }
}

const selectedWhitelistRow = ref<any>(null)
const selectedBlockedRow = ref<any>(null)
const showAddWhitelistDialog = ref(false)
const newWhitelistIP = ref('')


function durationMinutes(v: any): number {
  if (typeof v === 'number') return Math.round(v / 60000000000)
  if (typeof v !== 'string') return 0
  const h = /^(\d+)h/.exec(v)
  const m = /^(\d+)m/.exec(v)
  return (h ? Number(h[1]) * 60 : 0) + (m ? Number(m[1]) : 0)
}

async function fetchData() {
  loading.value = true
  try {
    const [wlRes, blRes, stRes] = await Promise.allSettled([
      SecurityAPI.getWhitelist(),
      SecurityAPI.getBlockedIPs(),
      SecurityAPI.getSecurityStats(),
    ])
    if (wlRes.status === 'fulfilled' && wlRes.value?.data?.data) {
      // 后端契约 {whitelist:[string...], count:n}（handlers.go:34-40）；字符串归一化为行对象
      const wlRaw = Array.isArray(wlRes.value.data.data)
        ? wlRes.value.data.data
        : wlRes.value.data.data.whitelist || []
      whitelist.value = wlRaw.map((item: any) =>
        typeof item === 'string'
          ? { ip: item, network: item.includes('/') ? item : '', created_at: '-' }
          : item,
      )
    } else {
      whitelist.value = []
      ElMessage.error(isZh.value ? '获取白名单失败' : 'Failed to load whitelist')
    }
    if (blRes.status === 'fulfilled' && blRes.value?.data?.data) {
      // 后端契约 {blocked_ips:[{ip,blocked_at,attempts,...}], count:n}（handlers.go:125-133）
      const blRaw = Array.isArray(blRes.value.data.data)
        ? blRes.value.data.data
        : blRes.value.data.data.blocked_ips || []
      blockedIPs.value = blRaw.map((item: any) => ({
        ...item,
        reason: item.reason || (isZh.value ? '登录爆破触发自动封禁' : 'Auto-blocked for login bruteforce'),
        expires_at: item.expires_at || '-',
      }))
    } else {
      blockedIPs.value = []
      ElMessage.error(isZh.value ? '获取封禁列表失败' : 'Failed to load blocked IPs')
    }
    if (stRes.status === 'fulfilled' && stRes.value?.data?.data) {
      Object.assign(stats, stRes.value.data.data)
      // /security/stats 已返回当前限流参数；回填表单，避免保存时用硬编码默认值覆盖真实配置
      if (stats.login_rate_limit != null) rateLimitForm.login_rate_limit = Number(stats.login_rate_limit)
      if (stats.api_rate_limit != null) rateLimitForm.api_rate_limit = Number(stats.api_rate_limit)
      const minutes = durationMinutes(stats.block_duration)
      if (minutes > 0) rateLimitForm.block_duration = minutes
      if (stats.enabled != null) stats.active_defense = stats.enabled ? 'ON' : 'OFF'
      if (stats.throttled_requests != null) stats.rate_limit_hits = Number(stats.throttled_requests) || 0
    }
  } catch {} finally {
    loading.value = false
  }
}

async function handleAddWhitelist() {
  const value = newWhitelistIP.value.trim()
  if (!value) return
  const isCIDR = value.includes('/')
  try {
    // 后端 AddNetwork 与 AddIP 是两个分支：CIDR 必须走 network 字段，
    // 否则会被当成普通 IP 字符串写进 map，实际不参与网段匹配。
    await SecurityAPI.addWhitelist(isCIDR ? { ip: value, network: value } : { ip: value })
    whitelist.value.push({ ip: value, network: isCIDR ? value : '', created_at: new Date().toLocaleString() })
    ElMessage.success(isZh.value ? `已将 ${value} 添加到白名单` : `Added ${value} to Whitelist`)
    newWhitelistIP.value = ''
    showAddWhitelistDialog.value = false
  } catch (e: any) {
    ElMessage.error((isZh.value ? '添加失败：' : 'Add failed: ') + (e?.message || (isZh.value ? '错误' : 'Error')))
  }
}

async function handleRemoveWhitelist(row: any) {
  ElMessageBox.confirm(
      isZh.value ? `从白名单移除 ${row.ip} 吗？` : `Remove ${row.ip} from Whitelist?`,
      isZh.value ? '移除白名单' : 'Remove Whitelist',
      {
        confirmButtonText: isZh.value ? '移除' : 'Remove',
        cancelButtonText: isZh.value ? '取消' : 'Cancel',
        type: 'warning',
      },
    ).then(async () => {
    try {
      await SecurityAPI.removeWhitelist({ ip: row.ip })
      whitelist.value = whitelist.value.filter((w) => w.ip !== row.ip)
      ElMessage.success(isZh.value ? `已移除 ${row.ip}` : `Removed ${row.ip}`)
    } catch (e: any) {
      ElMessage.error(e?.message || (isZh.value ? `移除 ${row.ip} 失败` : `Failed to remove ${row.ip}`))
    }
  }).catch(() => {})
}

async function handleUnblock(row: any) {
  ElMessageBox.confirm(
      isZh.value ? `解除封禁 IP ${row.ip} 吗？` : `Unblock IP ${row.ip}?`,
      isZh.value ? '解除封禁' : 'Unblock IP',
      {
        confirmButtonText: isZh.value ? '解除' : 'Unblock',
        cancelButtonText: isZh.value ? '取消' : 'Cancel',
        type: 'info',
      },
    ).then(async () => {
    try {
      await SecurityAPI.unblockIP(row.ip)
      blockedIPs.value = blockedIPs.value.filter((b) => b.ip !== row.ip)
      selectedBlockedRow.value = null
      ElMessage.success(isZh.value ? `已解除封禁 ${row.ip}` : `Unblocked ${row.ip}`)
    } catch (e: any) {
      ElMessage.error(e?.message || (isZh.value ? `解除封禁 ${row.ip} 失败` : `Failed to unblock ${row.ip}`))
    }
  }).catch(() => {})
}

async function handleSaveRateLimit() {
  try {
    const res = await SecurityAPI.updateRateLimit(rateLimitForm)
    if (res?.data?.success || res?.data?.status === 200) {
      ElMessage.success(isZh.value ? '限流策略已更新' : 'Rate limit policy updated')
    } else {
      ElMessage.error(res?.data?.message || (isZh.value ? '更新限流策略失败' : 'Failed to update rate limit'))
    }
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '更新限流策略失败' : 'Failed to update rate limit'))
  }
}

function handleHelp() {
  ElMessage.info(isZh.value ? '安全中心与 IP 白名单说明' : 'Security Center & IP Whitelist Manual')
}

onMounted(() => {
  fetchData()
  fetchRealtimeStats()
  // 实时访问页 5 秒自动刷新；切到该 Tab 时立即抓取一次
  realtimeTimer = setInterval(() => {
    if (currentTab.value === 'realtime') fetchRealtimeStats()
  }, 5000)
})

onBeforeUnmount(() => {
  if (realtimeTimer) {
    clearInterval(realtimeTimer)
    realtimeTimer = null
  }
})
</script>

<style scoped>
.cs-security-center-view {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-bg);
  overflow: hidden;
}

.sc-toolbar {
  min-height: 48px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-card-border);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  flex-shrink: 0;
  flex-wrap: wrap;
}

.sc-title-group {
  display: flex;
  align-items: baseline;
  gap: 6px;
}

.sc-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.sc-stats {
  font-size: 12px;
  color: var(--cs-text-dim);
}

.sc-spacer { flex: 1; }

.sc-body {
  flex: 1;
  display: flex;
  overflow: hidden;
}

.sc-sidebar {
  width: 220px;
  background: var(--cs-panel);
  border-right: 1px solid var(--cs-card-border);
  display: flex;
  flex-direction: column;
  padding: 8px;
  gap: 2px;
}

.sc-nav-item {
  height: 36px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
  border-radius: 6px;
  cursor: pointer;
  color: var(--cs-text);
  font-size: 14px;
  transition: all 120ms ease;
  user-select: none;
}

.sc-nav-item:hover {
  background: var(--cs-hover);
  color: var(--cs-text-bright);
}

.sc-nav-item.active {
  background: color-mix(in srgb, var(--cs-active) 10%, transparent);
  color: var(--cs-active);
  font-weight: 500;
}

.nav-icon {
  font-size: 14px;
}

.nav-label {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sc-content {
  flex: 1;
  background: var(--cs-bg);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  padding: 16px;
}

.sc-panel {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: var(--cs-panel);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
}

.sc-panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-height: 56px;
  padding: 12px 16px;
  border-bottom: 1px solid var(--cs-card-border);
  flex-shrink: 0;
  flex-wrap: wrap;
}

.sc-panel-body {
  padding: 16px;
  overflow-y: auto;
}

.panel-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.sc-table-wrap {
  flex: 1;
  overflow: hidden;
  min-height: 0;
}

.cs-form-container {
  max-width: 560px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.sc-stats-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 16px;
}

.stat-card {
  background: var(--cs-bg);
  border-radius: 8px;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.stat-num {
  font-size: 22px;
  font-weight: 600;
  color: var(--cs-active);
  font-family: var(--cs-font-mono);
  word-break: break-all;
}

.stat-lbl {
  font-size: 12px;
  color: var(--cs-text-dim);
}

.text-danger { color: var(--cs-error); }

.font-bold { font-weight: 600; }

.sc-header-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.sc-rt-hint {
  font-size: 12px;
  color: var(--cs-text-dim);
  font-family: var(--cs-font-mono);
}

.sc-rt-stats {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 16px;
  margin-bottom: 16px;
}

.cs-dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
