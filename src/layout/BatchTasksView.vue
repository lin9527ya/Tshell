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

      <div v-if="!tasks.length && !loading" class="cs-empty-watermark">
        <div class="empty-title">{{ isZh ? '暂无批量任务' : 'No batch tasks' }}</div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18nStore } from '@/stores/i18n'
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

let timer: number | undefined

onMounted(() => {
  fetchTasks()
  timer = window.setInterval(fetchTasks, 5000)
})

onBeforeUnmount(() => {
  if (timer) window.clearInterval(timer)
})
</script>

<style scoped>
.batch-tasks-view {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-bg);
  overflow: hidden;
}
.bt-toolbar {
  height: 32px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
  flex-shrink: 0;
}
.bt-title-group {
  display: flex;
  align-items: center;
  gap: 6px;
}
.bt-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--cs-text-bright);
}
.bt-count {
  font-size: 11px;
  color: var(--cs-text-dim);
}
.bt-spacer {
  flex: 1;
}
.bt-table-wrap {
  flex: 1;
  overflow: hidden;
}

.bt-expand-box {
  padding: 10px 14px;
  background: var(--cs-bg);
  border-bottom: 1px solid var(--cs-border);
}

.bt-expand-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--cs-text-bright);
  margin-bottom: 8px;
}

.bt-expand-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 10px;
}

.bt-result-card {
  background: var(--cs-panel);
  border: 1px solid var(--cs-border);
  border-radius: 3px;
  overflow: hidden;
}

.bt-card-header {
  padding: 4px 8px;
  background: var(--cs-titlebar-bg);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.bt-card-uid {
  font-size: 11.5px;
  font-weight: bold;
  color: var(--cs-active);
}

.bt-card-badge {
  font-size: 10.5px;
  color: var(--cs-online, #4caf50);
}

.bt-card-output {
  padding: 6px 8px;
  font-size: 11.5px;
  line-height: 1.4;
  color: var(--cs-text);
  background: transparent;
  margin: 0;
  max-height: 120px;
  overflow-y: auto;
  white-space: pre-wrap;
  word-break: break-all;
}

.bt-empty-hint {
  font-size: 12px;
  color: var(--cs-text-dim);
  font-style: italic;
  padding: 6px 0;
}
</style>
