<template>
  <div class="audit-container cs-font-ui">
    <!-- 工具条：左侧标题，右侧筛选与操作 -->
    <div class="al-toolbar">
      <div class="al-title-group">
        <span class="al-title">{{ isZh ? '审计日志' : 'Audit Logs' }}</span>
        <span class="al-count">({{ total }} {{ isZh ? '条日志' : 'entries' }})</span>
      </div>

      <div class="al-spacer" />

      <el-select v-model="query.level" :placeholder="isZh ? '级别' : 'Level'" clearable style="width: 110px">
        <el-option label="INFO" value="INFO" />
        <el-option label="WARN" value="WARN" />
        <el-option label="ERROR" value="ERROR" />
        <el-option label="SECURITY" value="SECURITY" />
      </el-select>

      <el-select v-model="query.category" :placeholder="isZh ? '类别' : 'Category'" clearable style="width: 120px">
        <el-option label="AUTH" value="AUTH" />
        <el-option label="API" value="API" />
        <el-option label="FILE" value="FILE" />
        <el-option label="SYSTEM" value="SYSTEM" />
        <el-option label="SECURITY" value="SECURITY" />
        <el-option label="COMMAND" value="COMMAND" />
      </el-select>

      <el-input
        v-model="query.keyword"
        :placeholder="isZh ? '过滤关键字 / 用户...' : 'Filter keyword / user...'"
        clearable
        class="al-search"
        @keyup.enter="loadLogs"
      />

      <button class="cs-btn cs-btn-primary" @click="loadLogs">{{ isZh ? '查询' : 'Query' }}</button>
      <button class="cs-btn" @click="resetQuery">{{ isZh ? '重置' : 'Reset' }}</button>
      <button class="cs-btn" @click="loadLogs">{{ isZh ? '刷新' : 'Refresh' }}</button>
      <button class="cs-btn" @click="handleExport">{{ isZh ? '导出日志' : 'Export Logs' }}</button>
      <button class="cs-btn cs-btn-danger" @click="handleCleanup">{{ isZh ? '清理日志' : 'Cleanup Logs' }}</button>
      <button class="cs-btn" @click="handleHelp">{{ isZh ? '帮助' : 'Help' }}</button>
    </div>

    <!-- 审计统计摘要（后端 /audit/statistics，24h/7d/30d） -->
    <div class="al-stats-strip">
      <div class="al-stats-main">
        <div class="al-stat-cell">
          <div class="al-stat-num">{{ stats.total_logs ?? 0 }}</div>
          <div class="al-stat-lbl">{{ isZh ? '区间日志总数' : 'Total Logs' }}</div>
        </div>
        <div class="al-stat-cell">
          <div class="al-stat-num">{{ stats.today_logs ?? 0 }}</div>
          <div class="al-stat-lbl">{{ isZh ? '今日日志' : 'Today' }}</div>
        </div>
        <div class="al-stat-cell">
          <div class="al-stat-num text-danger">{{ stats.recent_failures ?? 0 }}</div>
          <div class="al-stat-lbl">{{ isZh ? '近 1 小时失败' : 'Failures (1h)' }}</div>
        </div>
        <div class="al-stat-cell">
          <div class="al-stat-num text-online">{{ (stats.top_users || []).length }}</div>
          <div class="al-stat-lbl">{{ isZh ? '活跃操作员' : 'Active Operators' }}</div>
        </div>

        <div class="al-spacer" />

        <span class="al-stat-hint">{{ isZh ? '统计区间' : 'Range' }}:</span>
        <el-select v-model="statsRange" style="width: 96px" @change="loadStatistics">
          <el-option label="24h" value="24h" />
          <el-option label="7d" value="7d" />
          <el-option label="30d" value="30d" />
        </el-select>
        <button class="cs-btn cs-btn-xs" :disabled="statsLoading" @click="loadStatistics">
          {{ statsLoading ? (isZh ? '统计中...' : 'Loading...') : (isZh ? '统计' : 'Stats') }}
        </button>
        <button class="cs-btn cs-btn-xs" @click="statsExpanded = !statsExpanded">
          {{ statsExpanded ? (isZh ? '收起详情' : 'Hide Detail') : (isZh ? '详情' : 'Detail') }}
        </button>
      </div>

      <!-- 分布与 Top 列表（紧凑明细） -->
      <div v-if="statsExpanded" class="al-stats-detail">
        <div class="al-dist-col">
          <div class="al-detail-title">{{ isZh ? '级别分布' : 'By Level' }}</div>
          <div class="al-tags">
            <span v-for="(n, k) in stats.logs_by_level || {}" :key="k" class="al-tag" :class="`lvl-${String(k).toLowerCase()}`">
              {{ k }}: {{ n }}
            </span>
          </div>
          <div class="al-detail-title" style="margin-top: 12px">{{ isZh ? '类别分布' : 'By Category' }}</div>
          <div class="al-tags">
            <span v-for="(n, k) in stats.logs_by_category || {}" :key="k" class="al-tag">{{ k }}: {{ n }}</span>
          </div>
          <div class="al-detail-title" style="margin-top: 12px">{{ isZh ? '结果分布' : 'By Result' }}</div>
          <div class="al-tags">
            <span v-for="(n, k) in stats.logs_by_result || {}" :key="k" class="al-tag" :class="String(k) === 'SUCCESS' ? 'lvl-info' : 'lvl-error'">
              {{ k }}: {{ n }}
            </span>
          </div>
        </div>
        <div class="al-top-col">
          <div class="al-detail-title">{{ isZh ? 'Top 10 操作员' : 'Top 10 Operators' }}</div>
          <div class="al-top-list">
            <div v-for="u in stats.top_users || []" :key="u.username" class="al-top-row">
              <span class="al-top-name">{{ u.username }}</span>
              <span class="al-top-bar" :style="{ width: `${topBarWidth(u.count, stats.top_users)}%` }" />
              <span class="al-top-count">{{ u.count }}</span>
            </div>
          </div>
          <div class="al-detail-title" style="margin-top: 12px">{{ isZh ? 'Top 10 来源 IP' : 'Top 10 Source IPs' }}</div>
          <div class="al-top-list">
            <div v-for="ip in stats.top_ips || []" :key="ip.ip" class="al-top-row">
              <span class="al-top-name mono">{{ ip.ip }}</span>
              <span class="al-top-bar" :style="{ width: `${topBarWidth(ip.count, stats.top_ips)}%` }" />
              <span class="al-top-count">{{ ip.count }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Table -->
    <div class="al-table-wrap">
      <div class="al-table-box">
        <el-table
          :data="logs"
          height="100%"
          highlight-current-row
          class="cs-session-table"
        >
          <el-table-column prop="timestamp" :label="isZh ? '时间' : 'Time'" width="160" sortable>
            <template #default="{ row }">
              <span class="mono-cell">{{ formatTime(row.timestamp) }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="level" :label="isZh ? '级别' : 'Level'" width="90" align="center">
            <template #default="{ row }">
              <span class="arch-badge" :class="`lvl-${row.level?.toLowerCase()}`">{{ row.level }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="category" :label="isZh ? '类别' : 'Category'" width="100" align="center">
            <template #default="{ row }">
              <span class="mono-cell">{{ row.category }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="username" :label="isZh ? '用户' : 'User'" width="110" show-overflow-tooltip />
          <el-table-column prop="client_ip" :label="isZh ? '来源 IP' : 'Source IP'" width="120" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="mono-cell">{{ row.client_ip || '-' }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="action" :label="isZh ? '动作' : 'Action'" width="130" show-overflow-tooltip />
          <el-table-column prop="result" :label="isZh ? '结果' : 'Result'" width="90" align="center">
            <template #default="{ row }">
              <span :class="row.result === 'SUCCESS' ? 'text-online' : 'text-danger'">{{ row.result }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="message" :label="isZh ? '消息' : 'Message'" min-width="260" show-overflow-tooltip />
        </el-table>
      </div>

      <div class="al-pager">
        <el-pagination
          v-model:current-page="query.page"
          v-model:page-size="query.page_size"
          :total="total"
          layout="total, prev, pager, next"
          @change="loadLogs"
        />
      </div>
    </div>

    <!-- Cleanup Dialog -->
    <el-dialog
      v-model="cleanupDialogVisible"
      :title="isZh ? '清理审计日志' : 'Cleanup Audit Logs'"
      width="440px"
      append-to-body
      class="cs-dialog"
      :close-on-click-modal="false"
    >
      <div class="cs-dialog-body">
        <div class="cs-dialog-desc">
          {{ isZh ? '请选择需要清理的时间范围。清理后指定天数之前的日志记录将被彻底删除，不可恢复。' : 'Select the retention threshold. Log entries older than the selected days will be permanently deleted.' }}
        </div>

        <div class="cs-form-row">
          <label class="cs-form-label">{{ isZh ? '快捷选择:' : 'Preset Range:' }}</label>
          <el-select v-model="cleanupDays" style="width: 100%">
            <el-option :label="isZh ? '清理 7 天前的日志' : 'Older than 7 days'" :value="7" />
            <el-option :label="isZh ? '清理 14 天前的日志' : 'Older than 14 days'" :value="14" />
            <el-option :label="isZh ? '清理 30 天前的日志（默认）' : 'Older than 30 days (Default)'" :value="30" />
            <el-option :label="isZh ? '清理 90 天前的日志' : 'Older than 90 days'" :value="90" />
            <el-option :label="isZh ? '清理 180 天前的日志' : 'Older than 180 days'" :value="180" />
            <el-option :label="isZh ? '清理 365 天前的日志' : 'Older than 365 days'" :value="365" />
            <el-option :label="isZh ? '全部清理（清空所有历史日志）' : 'Clear All Logs (0 days)'" :value="0" />
          </el-select>
        </div>

        <div class="cs-form-row" style="margin-top: 10px">
          <label class="cs-form-label">{{ isZh ? '自定义天数:' : 'Custom Days:' }}</label>
          <el-input-number v-model="cleanupDays" :min="0" :max="3650" style="width: 140px" />
          <span class="al-cleanup-hint">
            {{ cleanupDays === 0 ? (isZh ? '（0 表示清空所有日志）' : '(0 = Clear all)') : (isZh ? `（清理 ${cleanupDays} 天前）` : `(Older than ${cleanupDays}d)`) }}
          </span>
        </div>
      </div>

      <template #footer>
        <div class="cs-dialog-footer">
          <button class="cs-btn cs-btn-danger" :disabled="cleanupLoading" @click="confirmCleanup">
            {{ cleanupLoading ? (isZh ? '清理中...' : 'Cleaning...') : (isZh ? '确认清理' : 'Confirm Clean') }}
          </button>
          <button class="cs-btn" @click="cleanupDialogVisible = false">
            {{ isZh ? '取消' : 'Cancel' }}
          </button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import AuditAPI from '@/api/audit'
import { useI18nStore } from '@/stores/i18n'
import { downloadBlob } from '@/layout/Dialogs/Payloads/payloadUtil'

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const logs = ref<Array<Record<string, any>>>([])
const total = ref(0)
const query = ref<Record<string, any>>({
  page: 1,
  page_size: 50,
  level: '',
  category: '',
  username: '',
  keyword: '',
  result: '',
})

// 审计统计摘要（/audit/statistics）
const statsRange = ref('24h')
const statsLoading = ref(false)
const statsExpanded = ref(false)
const stats = ref<Record<string, any>>({})

const loadStatistics = async () => {
  statsLoading.value = true
  try {
    const res = await AuditAPI.getAuditStatistics(statsRange.value)
    if (res.data?.status === 200) {
      stats.value = res.data.data || {}
    } else {
      stats.value = {}
      ElMessage.error(isZh.value ? '获取审计统计失败' : 'Failed to load audit statistics')
    }
  } catch {
    stats.value = {}
    ElMessage.error(isZh.value ? '获取审计统计失败' : 'Failed to load audit statistics')
  } finally {
    statsLoading.value = false
  }
}

const topBarWidth = (count: number, list: Array<{ count: number }>) => {
  const max = Math.max(1, ...(list || []).map((i) => i.count || 0))
  return Math.max(6, Math.round(((count || 0) / max) * 100))
}

const formatTime = (t: string) => (t ? new Date(t).toLocaleString('en-GB') : '-')

const loadLogs = async () => {
  try {
    const params: Record<string, any> = { page: query.value.page, page_size: query.value.page_size }
    for (const k of ['level', 'category', 'username', 'keyword', 'result']) {
      if (query.value[k]) params[k] = query.value[k]
    }
    const res = await AuditAPI.getAuditLogs(params)
    if (res.data?.status === 200) {
      logs.value = res.data.data.logs || []
      total.value = res.data.data.total || 0
    }
  } catch (e) {
    ElMessage.error(isZh.value ? '查询审计日志失败' : 'Failed to query audit logs')
  }
}

const resetQuery = () => {
  query.value = {
    page: 1,
    page_size: 50,
    level: '',
    category: '',
    username: '',
    keyword: '',
    result: '',
  }
  loadLogs()
}

const handleExport = async () => {
  try {
    const res = await AuditAPI.exportAuditLogs({
      level: query.value.level || undefined,
      category: query.value.category || undefined,
      format: 'json',
    })
    // res.data 已是 Blob（api 层 responseType:'blob'），直接下载；JSON 是合法的导出内容
    if (downloadBlob(res, `audit_logs_${Date.now()}.json`, true)) {
      ElMessage.success(isZh.value ? '审计日志已导出' : 'Audit logs exported')
    } else {
      ElMessage.error(isZh.value ? '导出失败：响应不是可下载文件' : 'Export failed: response is not a downloadable file')
    }
  } catch (e) {
    ElMessage.error(isZh.value ? '导出失败' : 'Export failed')
  }
}

const cleanupDialogVisible = ref(false)
const cleanupDays = ref(30)
const cleanupLoading = ref(false)

const handleCleanup = () => {
  cleanupDialogVisible.value = true
}

const confirmCleanup = async () => {
  cleanupLoading.value = true
  try {
    const days = cleanupDays.value
    await AuditAPI.cleanupOldLogs(days)
    ElMessage.success(
      isZh.value
        ? days === 0
          ? '全量旧审计日志已清理'
          : `已清理 ${days} 天前的审计日志`
        : `Cleaned up audit logs older than ${days} days`,
    )
    cleanupDialogVisible.value = false
    loadLogs()
    loadStatistics()
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '清理失败' : 'Cleanup failed'))
  } finally {
    cleanupLoading.value = false
  }
}

const handleHelp = () => {
  ElMessage.info(isZh.value ? '审计日志查看器：提供团队服务器所有操作员行为、系统事件的检索、统计、清理与导出。' : 'Cobalt Strike Audit Logs Viewer Help')
}

onMounted(() => {
  loadLogs()
  loadStatistics()
})
</script>

<style scoped>
.audit-container {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-bg);
  overflow: hidden;
}

.al-toolbar {
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

.al-title-group {
  display: flex;
  align-items: baseline;
  gap: 6px;
}

.al-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.al-count {
  font-size: 12px;
  color: var(--cs-text-dim);
}

.al-spacer {
  flex: 1;
}

.al-search {
  width: 200px;
}

/* 审计统计摘要（卡片式） */
.al-stats-strip {
  margin: 16px 16px 0;
  background: var(--cs-panel);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  padding: 12px 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  flex-shrink: 0;
}

.al-stats-main {
  display: flex;
  align-items: center;
  gap: 24px;
  flex-wrap: wrap;
}

.al-stat-cell {
  display: flex;
  align-items: baseline;
  gap: 8px;
}

.al-stat-num {
  font-family: var(--cs-font-mono);
  font-size: 16px;
  font-weight: 600;
  color: var(--cs-active);
}

.al-stat-lbl {
  font-size: 12px;
  color: var(--cs-text-dim);
}

.al-stat-hint {
  font-size: 12px;
  color: var(--cs-text-dim);
}

.al-stats-detail {
  display: flex;
  gap: 24px;
  border-top: 1px solid var(--cs-card-border);
  padding-top: 12px;
  max-height: 220px;
  overflow-y: auto;
}

.al-dist-col,
.al-top-col {
  flex: 1;
  min-width: 0;
}

.al-detail-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--cs-text-bright);
  margin-bottom: 6px;
}

.al-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.al-tag {
  display: inline-flex;
  align-items: center;
  height: 20px;
  padding: 0 8px;
  font-size: 12px;
  font-family: var(--cs-font-mono);
  border: 1px solid var(--cs-card-border);
  border-radius: 4px;
  color: var(--cs-text);
  background: var(--cs-bg);
}

.al-top-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.al-top-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: var(--cs-text);
}

.al-top-name {
  width: 130px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.al-top-name.mono {
  font-family: var(--cs-font-mono);
}

.al-top-bar {
  height: 6px;
  background: color-mix(in srgb, var(--cs-active) 70%, transparent);
  border-radius: 4px;
}

.al-top-count {
  font-family: var(--cs-font-mono);
  color: var(--cs-text-dim);
}

.al-table-wrap {
  flex: 1;
  overflow: hidden;
  margin: 16px;
  display: flex;
  flex-direction: column;
  background: var(--cs-panel);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
}

.al-table-box {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.al-pager {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  padding: 8px 16px;
  border-top: 1px solid var(--cs-card-border);
  flex-shrink: 0;
}

.mono-cell {
  font-family: var(--cs-font-mono);
  font-size: 13px;
}

.al-cleanup-hint {
  font-size: 12px;
  color: var(--cs-text-dim);
  margin-left: 8px;
}

/* 级别 / 结果 tag */
.arch-badge {
  display: inline-flex;
  align-items: center;
  height: 20px;
  padding: 0 8px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 500;
  background: var(--cs-hover);
  color: var(--cs-text);
}

.lvl-info {
  background: color-mix(in srgb, var(--cs-active) 12%, transparent);
  color: var(--cs-active);
}

.lvl-warn {
  background: color-mix(in srgb, var(--cs-warning) 18%, transparent);
  color: var(--cs-warning);
}

.lvl-error, .lvl-security {
  background: color-mix(in srgb, var(--cs-error) 14%, transparent);
  color: var(--cs-error);
}

.text-online {
  color: var(--cs-online);
}

.text-danger {
  color: var(--cs-error);
}

.cs-dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
