<template>
  <div class="batch-tasks-view cs-font-ui">
    <!-- CS Toolbar -->
    <div class="bt-toolbar">
      <div class="bt-title-group">
        <span class="bt-title">{{ isZh ? '批量任务进度' : 'Batch Task Progress' }}</span>
        <span class="bt-count">({{ tasks.length }})</span>
      </div>
      <div class="bt-spacer" />
      <button class="cs-btn" :disabled="loading" @click="fetchTasks">{{ isZh ? '刷新' : 'Refresh' }}</button>
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
        <!-- Expandable details row -->
        <el-table-column type="expand" width="36">
          <template #default="{ row }">
            <div class="bt-expand-box">
              <div class="bt-expand-title">{{ isZh ? '各 Beacon 执行明细' : 'Execution Details per Beacon' }}:</div>
              <div class="bt-expand-grid">
                <div
                  v-for="(resultText, uid) in (row.results || {})"
                  :key="uid"
                  class="bt-result-card"
                >
                  <div class="bt-card-header">
                    <span class="bt-card-uid cs-font-mono">{{ String(uid).slice(0, 8) }}</span>
                    <span class="bt-card-badge">{{ isZh ? '已接收' : 'Received' }}</span>
                  </div>
                  <pre class="bt-card-output cs-font-mono">{{ resultText }}</pre>
                </div>
              </div>
              <div v-if="!row.results || !Object.keys(row.results).length" class="bt-empty-hint">
                {{ isZh ? '任务已下发至服务端队列，等待目标 Beacon 拉取执行...' : 'Tasks queued on server, awaiting beacon heartbeat fetch...' }}
              </div>
            </div>
          </template>
        </el-table-column>

        <el-table-column prop="id" label="任务 ID" width="160">
          <template #default="{ row }">
            <span class="cs-font-mono text-dim">{{ row.id }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="command" :label="isZh ? '下发命令' : 'Command'" min-width="200" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="cs-font-mono font-bold">{{ row.command }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="isZh ? '目标数' : 'Targets'" width="90" align="center">
          <template #default="{ row }">
            <el-tag size="small" type="info">{{ Array.isArray(row.uids) ? row.uids.length : 0 }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="status" :label="isZh ? '状态' : 'Status'" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="row.status === 'done' ? 'success' : 'warning'" size="small">
              {{ row.status === 'done' ? (isZh ? '已完成' : 'Done') : (isZh ? '进行中' : 'Running') }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" :label="isZh ? '创建时间' : 'Created At'" width="165">
          <template #default="{ row }">
            <span class="cs-font-mono text-dim">{{ formatTime(row.created_at) }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="isZh ? '执行进度' : 'Progress'" min-width="160">
          <template #default="{ row }">
            <span class="cs-font-mono">
              {{ Object.keys(row.results || {}).length }} / {{ Array.isArray(row.uids) ? row.uids.length : 0 }} {{ isZh ? '已回显' : 'echoed' }}
            </span>
          </template>
        </el-table-column>
      </el-table>

      <EmptyState
        v-if="!tasks.length && !loading"
        variant="overlay"
        :title="isZh ? '暂无批量任务' : 'No batch tasks'"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18nStore } from '@/stores/i18n'
import { useVisibilityInterval } from '@/utils/useVisibilityInterval'
import EmptyState from '@/components/Common/EmptyState.vue'
import ClientAPI from '@/api/clients'

interface TaskRow {
  id: string
  command: string
  uids: string[]
  status: 'running' | 'done'
  created_at: string
  results: Record<string, string>
}

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const tasks = ref<TaskRow[]>([])
const loading = ref(false)

function formatTime(t: string): string {
  if (!t) return '-'
  const d = new Date(t)
  if (isNaN(d.getTime())) return t
  return d.toLocaleString('zh-CN')
}

async function fetchTasks() {
  loading.value = true
  try {
    const res = await ClientAPI.list_tasks()
    if (res.data?.status === 200 && Array.isArray(res.data.data)) {
      // 后端无排序，新的在前
      tasks.value = [...res.data.data].reverse()
    } else {
      tasks.value = []
    }
  } catch {
    tasks.value = []
  } finally {
    loading.value = false
  }
}

// 【性能】5s 轮询接入可见性治理(非活动标签/页面隐藏时暂停)
useVisibilityInterval(fetchTasks, 5000)

onMounted(() => {
  fetchTasks()
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

.bt-table-wrap :deep(.el-tag--info) {
  background: color-mix(in srgb, var(--cs-text-dim) 14%, transparent);
  color: var(--cs-text-dim);
}

/* ===== 展开行：各 Beacon 执行明细 ===== */
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

.bt-expand-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 12px;
}

.bt-result-card {
  background: var(--cs-card-bg);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  overflow: hidden;
}

.bt-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 10px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-card-border);
}

.bt-card-uid {
  font-size: 13px;
  font-weight: 600;
  color: var(--cs-active);
}

.bt-card-badge {
  font-size: 12px;
  color: var(--cs-online);
}

.bt-card-output {
  max-height: 120px;
  margin: 0;
  padding: 8px 10px;
  overflow-y: auto;
  font-family: var(--cs-font-mono);
  font-size: 13px;
  line-height: 1.5;
  color: var(--cs-text);
  background: transparent;
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
