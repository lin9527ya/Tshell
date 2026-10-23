<template>
  <div class="batch-tasks-view cs-font-ui">
    <!-- CS Toolbar -->
    <div class="bt-toolbar">
      <div class="bt-title-group">
        <span class="bt-count">({{ total }})</span>
        <el-tag v-if="runningCount" size="small" type="warning" class="bt-running">
          {{ i18nStore.tl(`${runningCount} 进行中`, `${runningCount} running`) }}
        </el-tag>
      </div>

      <div class="bt-filters">
        <el-select v-model="filters.status" size="small" class="bt-filter-status" @change="reload">
          <el-option :label="i18nStore.tl('全部状态', 'All statuses')" value="" />
          <el-option v-for="s in STATUS_OPTIONS" :key="s.value" :label="i18nStore.tl(s.zh, s.en)" :value="s.value" />
        </el-select>
        <el-select v-model="filters.kind" size="small" class="bt-filter-kind" @change="reload">
          <el-option :label="i18nStore.tl('全部类型', 'All kinds')" value="" />
          <el-option v-for="k in KIND_OPTIONS" :key="k.value" :label="i18nStore.tl(k.zh, k.en)" :value="k.value" />
        </el-select>
        <el-input
          v-model="filters.uid"
          size="small"
          clearable
          class="bt-filter-uid"
          :placeholder="i18nStore.tl('按会话 UID 过滤', 'Filter by UID')"
          @keyup.enter="reload"
          @clear="reload"
        />
      </div>

      <div class="bt-spacer" />
      <button class="cs-btn" :disabled="loading" @click="reload">{{ i18nStore.tl('刷新', 'Refresh') }}</button>
    </div>

    <!-- Tasks Table -->
    <div class="bt-table-wrap">
      <el-table
        :data="tasks"
        row-key="id"
        size="small"
        height="100%"
        v-loading="loading"
        class="cs-session-table"
      >
        <!-- 展开：结果与错误全文 -->
        <el-table-column type="expand" width="36">
          <template #default="{ row }">
            <div class="bt-expand-box">
              <div class="bt-expand-title">{{ i18nStore.tl('任务详情', 'Task Detail') }}</div>
              <div class="bt-expand-meta">
                <span><b>{{ i18nStore.tl('会话', 'Session') }}:</b> <code class="cs-font-mono">{{ row.uid }}</code></span>
                <span><b>{{ i18nStore.tl('批次', 'Batch') }}:</b> <code class="cs-font-mono">{{ row.batch_id || '-' }}</code></span>
                <span><b>{{ i18nStore.tl('下发', 'Dispatched') }}:</b> {{ formatTime(row.created_at) }}</span>
                <span v-if="row.finished_at && row.finished_at !== '0001-01-01T00:00:00Z'">
                  <b>{{ i18nStore.tl('结束', 'Finished') }}:</b> {{ formatTime(row.finished_at) }}
                </span>
              </div>
              <div v-if="row.error" class="bt-error-box">
                <div class="bt-error-title">{{ i18nStore.tl('失败原因', 'Error') }}</div>
                <pre class="bt-card-output cs-font-mono">{{ row.error }}</pre>
              </div>
              <div class="bt-expand-title">{{ i18nStore.tl('回显结果', 'Result') }}</div>
              <pre v-if="row.result" class="bt-card-output cs-font-mono">{{ row.result }}</pre>
              <div v-else class="bt-empty-hint">
                {{ row.status === 'dispatched'
                  ? (i18nStore.tl('尚未收到回显（命令可能仍在队列中等待目标拉取）', 'No output yet (may still be queued on the target)'))
                  : (i18nStore.tl('该任务没有回显内容', 'No output recorded')) }}
              </div>
            </div>
          </template>
        </el-table-column>

        <el-table-column prop="id" :label="i18nStore.tl('任务 ID', 'ID')" width="80">
          <template #default="{ row }">
            <span class="cs-font-mono text-dim">{{ row.id }}</span>
          </template>
        </el-table-column>

        <el-table-column :label="i18nStore.tl('状态', 'Status')" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="statusTagType(row.status)" size="small">{{ statusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>

        <el-table-column :label="i18nStore.tl('类型', 'Kind')" width="90" align="center">
          <template #default="{ row }">
            <span class="cs-cell-text">{{ kindLabel(row.kind) }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="uid" :label="i18nStore.tl('目标会话', 'Session')" width="110">
          <template #default="{ row }">
            <span class="cs-font-mono" :title="row.uid">{{ shortUid(row.uid) }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="operator" :label="i18nStore.tl('操作员', 'Operator')" width="110" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="cs-cell-text">{{ row.operator || '-' }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="command" :label="i18nStore.tl('命令', 'Command')" min-width="200" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="cs-font-mono font-bold">{{ row.command }}</span>
          </template>
        </el-table-column>

        <el-table-column :label="i18nStore.tl('耗时', 'Elapsed')" width="90" align="center">
          <template #default="{ row }">
            <span class="cs-font-mono text-dim">{{ elapsed(row) }}</span>
          </template>
        </el-table-column>

        <el-table-column :label="i18nStore.tl('操作', 'Actions')" width="110" fixed="right" align="center">
          <template #default="{ row }">
            <button
              v-if="row.status === 'dispatched'"
              class="cs-btn bt-cancel-btn"
              :title="i18nStore.tl('停止等待回显（已下发的命令无法从目标撤回）', 'Stop waiting (cannot be retracted from target)')"
              @click="cancelTask(row)"
            >
              {{ i18nStore.tl('取消', 'Cancel') }}
            </button>
            <span v-else class="text-dim">—</span>
          </template>
        </el-table-column>
        <template #empty>
          <EmptyState variant="inline" :title="i18nStore.tl('暂无任务', 'No tasks')" />
        </template>
      </el-table>
    </div>

    <div class="bt-pager">
      <el-pagination
        small
        background
        layout="total, sizes, prev, pager, next, jumper"
        :total="total"
        :current-page="page"
        :page-size="size"
        :page-sizes="[50, 100, 200]"
        @current-change="onPageChange"
        @size-change="onSizeChange"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18nStore } from '@/stores/i18n'
import EmptyState from '@/components/Common/EmptyState.vue'
import TaskAPI, { type TaskRow } from '@/api/tasks'

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const tasks = ref<TaskRow[]>([])
const total = ref(0)
const page = ref(1)
const size = ref(50)
const loading = ref(false)
const filters = ref({ status: '', kind: '', uid: '' })

const STATUS_OPTIONS = [
  { value: 'dispatched', zh: '进行中', en: 'Running' },
  { value: 'completed', zh: '已完成', en: 'Completed' },
  { value: 'failed', zh: '失败', en: 'Failed' },
  { value: 'cancelled', zh: '已取消', en: 'Cancelled' },
  { value: 'timeout', zh: '超时', en: 'Timeout' },
]
const KIND_OPTIONS = [
  { value: 'console', zh: '控制台命令', en: 'Console' },
  { value: 'batch', zh: '批量下发', en: 'Batch' },
  { value: 'exec', zh: '载荷执行', en: 'Exec' },
  { value: 'download', zh: '文件下载', en: 'Download' },
]

const runningCount = computed(() => tasks.value.filter((t) => t.status === 'dispatched').length)

function statusLabel(s: string): string {
  const opt = STATUS_OPTIONS.find((o) => o.value === s)
  if (opt) return isZh.value ? opt.zh : opt.en
  return s || '-'
}

function statusTagType(s: string): 'success' | 'warning' | 'danger' | 'info' {
  if (s === 'completed') return 'success'
  if (s === 'dispatched') return 'warning'
  if (s === 'failed' || s === 'timeout') return 'danger'
  return 'info'
}

function kindLabel(k: string): string {
  const opt = KIND_OPTIONS.find((o) => o.value === k)
  return opt ? (isZh.value ? opt.zh : opt.en) : (k || '-')
}

const shortUid = (uid: string) => (uid ? uid.slice(0, 8) : '-')

function formatTime(t: string): string {
  if (!t) return '-'
  const d = new Date(t)
  if (isNaN(d.getTime()) || d.getFullYear() < 1970) return '-'
  return d.toLocaleString('zh-CN')
}

// 耗时：未结束的任务按"到现在"计（避免显示成 0 秒的误导）
function elapsed(row: TaskRow): string {
  const start = new Date(row.created_at).getTime()
  if (!start || isNaN(start)) return '-'
  const endRaw = row.status === 'dispatched' ? Date.now() : new Date(row.finished_at).getTime()
  const end = endRaw && !isNaN(endRaw) && endRaw > 0 ? endRaw : Date.now()
  const sec = Math.max(0, Math.round((end - start) / 1000))
  if (sec < 60) return `${sec}s`
  if (sec < 3600) return `${Math.floor(sec / 60)}m${sec % 60}s`
  return `${Math.floor(sec / 3600)}h${Math.floor((sec % 3600) / 60)}m`
}

async function fetchTasks() {
  loading.value = true
  try {
    const res = await TaskAPI.list({
      page: page.value,
      size: size.value,
      status: filters.value.status || undefined,
      kind: filters.value.kind || undefined,
      uid: filters.value.uid.trim() || undefined,
    })
    const data = res.data?.data
    tasks.value = Array.isArray(data?.list) ? data.list : []
    total.value = Number(data?.total) || 0
  } catch {
    // 请求层已提示；保持上一次数据，避免抖动清空
  } finally {
    loading.value = false
  }
}

function reload() {
  page.value = 1
  fetchTasks()
}

function onPageChange(p: number) {
  page.value = p
  fetchTasks()
}

function onSizeChange(s: number) {
  size.value = s
  page.value = 1
  fetchTasks()
}

async function cancelTask(row: TaskRow) {
  try {
    await ElMessageBox.confirm(
      isZh.value
        ? `取消任务 #${row.id}（${row.command}）？\n\n注意：命令已经下发给目标，服务端无法撤回，这里只是停止等待回显。`
        : `Cancel task #${row.id} (${row.command})?\n\nThe command is already dispatched and cannot be retracted; this only stops waiting for output.`,
      isZh.value ? '取消任务' : 'Cancel Task',
      { confirmButtonText: isZh.value ? '取消任务' : 'Cancel Task', cancelButtonText: isZh.value ? '返回' : 'Back', type: 'warning' },
    )
  } catch {
    return
  }
  try {
    await TaskAPI.cancel(row.id)
    ElMessage.success(isZh.value ? '任务已取消' : 'Task cancelled')
    fetchTasks()
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || (isZh.value ? '取消失败' : 'Cancel failed'))
  }
}

// 5s 轮询（仅在有进行中任务时刷新，避免空转请求）
let pollTimer: number | null = null
function startPolling() {
  if (pollTimer !== null) return
  pollTimer = window.setInterval(() => {
    if (document.visibilityState === 'hidden') return
    if (!tasks.value.some((t) => t.status === 'dispatched')) return
    fetchTasks()
  }, 5000)
}

onMounted(() => {
  fetchTasks()
  startPolling()
})

onBeforeUnmount(() => {
  if (pollTimer !== null) {
    window.clearInterval(pollTimer)
    pollTimer = null
  }
})
</script>

<style scoped>
.batch-tasks-view {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-panel);
  overflow: hidden;
}

/* 工具条：白底 + 1px 分隔线 */
.bt-toolbar {
  flex-shrink: 0;
  min-height: 48px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 16px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-card-border);
}

.bt-title-group {
  display: flex;
  align-items: center;
  gap: 8px;
}

.bt-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.bt-count {
  font-size: 13px;
  color: var(--cs-text-dim);
}

.bt-running {
  border: none;
}

.bt-filters {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-left: 12px;
}

.bt-filter-status { width: 130px; }
.bt-filter-kind { width: 130px; }
.bt-filter-uid { width: 200px; }

.bt-spacer {
  flex: 1;
}

.bt-table-wrap {
  flex: 1;
  overflow: hidden;
  position: relative;
}

/* 状态 tag（实心，20px / 4px 圆角） */
.bt-table-wrap :deep(.el-tag) {
  height: 20px;
  padding: 0 7px;
  font-size: 12px;
  border: none;
  border-radius: 4px;
}

.bt-table-wrap :deep(.el-tag--success) {
  background: var(--cs-online);
  color: #ffffff;
}

.bt-table-wrap :deep(.el-tag--warning) {
  background: var(--cs-warning);
  color: #ffffff;
}

.bt-table-wrap :deep(.el-tag--danger) {
  background: var(--cs-error);
  color: #ffffff;
}

.bt-table-wrap :deep(.el-tag--info) {
  background: color-mix(in srgb, var(--cs-text-dim) 14%, transparent);
  color: var(--cs-text-dim);
}

.bt-cancel-btn {
  height: 22px;
  padding: 0 10px;
  font-size: 12px;
}

.bt-pager {
  flex-shrink: 0;
  display: flex;
  justify-content: flex-end;
  padding: 6px 16px;
  border-top: 1px solid var(--cs-card-border);
}

/* ===== 展开行：任务详情 ===== */
.bt-expand-box {
  padding: 12px 16px;
  background: var(--cs-bg);
  border-bottom: 1px solid var(--cs-card-border);
}

.bt-expand-title {
  margin-bottom: 8px;
  font-size: 13px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.bt-expand-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
  margin-bottom: 10px;
  font-size: 12px;
  color: var(--cs-text-dim);
}

.bt-error-box {
  margin-bottom: 10px;
  border: 1px solid color-mix(in srgb, var(--cs-error) 40%, transparent);
  border-radius: 6px;
  overflow: hidden;
}

.bt-error-title {
  padding: 4px 10px;
  font-size: 12px;
  font-weight: 600;
  color: var(--cs-error);
  background: color-mix(in srgb, var(--cs-error) 10%, transparent);
}

.bt-card-output {
  max-height: 220px;
  margin: 0;
  padding: 8px 10px;
  overflow-y: auto;
  font-family: var(--cs-font-mono);
  font-size: 13px;
  line-height: 1.5;
  color: var(--cs-text);
  background: var(--cs-card-bg);
  white-space: pre-wrap;
  word-break: break-all;
}

.bt-empty-hint {
  padding: 6px 0;
  font-size: 13px;
  font-style: italic;
  color: var(--cs-text-dim);
}
</style>
