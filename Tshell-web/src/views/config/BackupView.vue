<template>
  <div class="bk-page cs-font-ui">
    <div class="bk-toolbar">
      <div class="bk-title-group">
        <span class="bk-title">{{ isZh ? '数据库备份' : 'Database Backup' }}</span>
        <span class="bk-meta">{{ dir }} · {{ isZh ? `保留 ${keep} 份` : `keep ${keep}` }}</span>
      </div>
      <div class="bk-spacer" />
      <button class="cs-btn" :disabled="loading" @click="fetchList">{{ isZh ? '刷新' : 'Refresh' }}</button>
      <button class="cs-btn cs-btn-primary" :disabled="creating" @click="createBackup">
        {{ creating ? (isZh ? '备份中…' : 'Backing up…') : (isZh ? '立即备份' : 'Backup Now') }}
      </button>
      <label class="cs-btn bk-import">
        {{ isZh ? '导入备份' : 'Import' }}
        <input type="file" accept=".db" hidden @change="onImport" />
      </label>
    </div>

    <!-- 恢复说明：不提供按钮是刻意的 —— SQLite 数据库无法在服务端运行时热替换 -->
    <div class="bk-hint">
      <VsIcon name="info-circle" :size="14" />
      <span v-if="isZh">
        备份是在线一致性快照（含 WAL）。<b>恢复需要先停止服务端</b>，在服务器上执行
        <code class="cs-font-mono">./tshell -restore &lt;备份名&gt;</code>，再启动服务端；
        恢复前会自动保存一份 <code class="cs-font-mono">.pre-restore-*</code> 快照。
      </span>
      <span v-else>
        Backups are online consistent snapshots (WAL included). <b>Restoring requires stopping the server</b>,
        then running <code class="cs-font-mono">./tshell -restore &lt;name&gt;</code> on the host and starting it again;
        a <code class="cs-font-mono">.pre-restore-*</code> snapshot is kept automatically.
      </span>
    </div>

    <div class="bk-table-wrap">
      <el-table :data="list" row-key="name" size="small" height="100%" v-loading="loading" class="cs-session-table">
        <el-table-column prop="name" :label="isZh ? '备份文件' : 'File'" min-width="240" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="cs-font-mono">{{ row.name }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="isZh ? '大小' : 'Size'" width="100" align="right">
          <template #default="{ row }">
            <span class="cs-font-mono text-dim">{{ formatSize(row.size) }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="isZh ? '创建时间' : 'Created'" width="170">
          <template #default="{ row }">
            <span class="cs-font-mono text-dim">{{ formatTime(row.created_at) }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="isZh ? '校验' : 'Verified'" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="row.verified ? 'success' : 'danger'" size="small">
              {{ row.verified ? (isZh ? '已校验' : 'Verified') : (isZh ? '未校验' : 'Unverified') }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="note" :label="isZh ? '备注' : 'Note'" min-width="140" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="cs-cell-text">{{ row.note || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="isZh ? '操作' : 'Actions'" width="150" fixed="right" align="center">
          <template #default="{ row }">
            <a class="row-link" @click="download(row)">{{ isZh ? '下载' : 'Download' }}</a>
            <a class="row-link bk-danger" @click="remove(row)">{{ isZh ? '删除' : 'Delete' }}</a>
          </template>
        </el-table-column>
      </el-table>

      <EmptyState v-if="!list.length && !loading" variant="overlay" :title="isZh ? '暂无备份' : 'No backups yet'" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import BackupAPI, { type BackupInfo } from '@/api/backup'
import { useI18nStore } from '@/stores/i18n'
import EmptyState from '@/components/Common/EmptyState.vue'
import VsIcon from '@/layout-vshell/VsIcon.vue'
import { downloadBlob } from '@/layout/Dialogs/Payloads/payloadUtil'

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const list = ref<BackupInfo[]>([])
const dir = ref('')
const keep = ref(0)
const loading = ref(false)
const creating = ref(false)

function formatSize(bytes: number): string {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let v = bytes
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

function formatTime(t: string): string {
  if (!t) return '-'
  const d = new Date(t)
  if (isNaN(d.getTime()) || d.getFullYear() < 1970) return '-'
  return d.toLocaleString('zh-CN')
}

async function fetchList() {
  loading.value = true
  try {
    const res = await BackupAPI.list()
    const data = res.data?.data
    list.value = Array.isArray(data?.list) ? data.list : []
    dir.value = data?.dir || ''
    keep.value = Number(data?.keep) || 0
  } catch {
    // 请求层已提示；保留上一次数据
  } finally {
    loading.value = false
  }
}

async function createBackup() {
  let note = ''
  try {
    const r = await ElMessageBox.prompt(
      isZh.value ? '备份备注（可选，便于日后辨认）:' : 'Backup note (optional):',
      isZh.value ? '立即备份' : 'Backup Now',
      {
        confirmButtonText: isZh.value ? '开始备份' : 'Backup',
        cancelButtonText: isZh.value ? '取消' : 'Cancel',
        inputPlaceholder: isZh.value ? '例如：升级前' : 'e.g. before upgrade',
        inputValue: '',
      },
    )
    note = r.value || ''
  } catch {
    return
  }
  creating.value = true
  try {
    const res = await BackupAPI.create(note)
    const info = res.data?.data
    ElMessage.success(
      isZh.value
        ? `备份完成: ${info?.name || ''}（已通过完整性校验）`
        : `Backup done: ${info?.name || ''} (verified)`,
    )
    await fetchList()
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || (isZh.value ? '备份失败' : 'Backup failed'))
  } finally {
    creating.value = false
  }
}

async function download(row: BackupInfo) {
  try {
    const res = await BackupAPI.download(row.name)
    downloadBlob(res.data, row.name)
  } catch {
    ElMessage.error(isZh.value ? '下载失败' : 'Download failed')
  }
}

async function remove(row: BackupInfo) {
  try {
    await ElMessageBox.confirm(
      isZh.value ? `删除备份 ${row.name}？此操作不可撤销。` : `Delete backup ${row.name}? This cannot be undone.`,
      isZh.value ? '删除备份' : 'Delete Backup',
      { type: 'warning', confirmButtonText: isZh.value ? '删除' : 'Delete', cancelButtonText: isZh.value ? '取消' : 'Cancel' },
    )
  } catch {
    return
  }
  try {
    await BackupAPI.remove(row.name)
    ElMessage.success(isZh.value ? '已删除' : 'Deleted')
    await fetchList()
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || (isZh.value ? '删除失败' : 'Delete failed'))
  }
}

async function onImport(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = '' // 允许重复选择同一个文件
  if (!file) return
  try {
    await BackupAPI.import(file)
    ElMessage.success(isZh.value ? '导入成功（已通过完整性校验）' : 'Imported (verified)')
    await fetchList()
  } catch (err: any) {
    ElMessage.error(err?.response?.data?.error || (isZh.value ? '导入失败' : 'Import failed'))
  }
}

onMounted(fetchList)
</script>

<style scoped>
.bk-page {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-panel);
  overflow: hidden;
}

.bk-toolbar {
  flex-shrink: 0;
  min-height: 48px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 16px;
  border-bottom: 1px solid var(--cs-card-border);
}

.bk-title-group {
  display: flex;
  align-items: baseline;
  gap: 10px;
}

.bk-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.bk-meta {
  font-size: 12px;
  color: var(--cs-text-dim);
}

.bk-spacer {
  flex: 1;
}

.bk-import {
  display: inline-flex;
  align-items: center;
  cursor: pointer;
}

.bk-hint {
  flex-shrink: 0;
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin: 10px 16px 0;
  padding: 8px 12px;
  font-size: 12.5px;
  line-height: 1.6;
  color: var(--cs-text-dim);
  background: color-mix(in srgb, var(--cs-warning) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--cs-warning) 30%, transparent);
  border-radius: 6px;
}

.bk-hint code {
  padding: 1px 4px;
  background: var(--cs-bg);
  border-radius: 3px;
  color: var(--cs-text);
}

.bk-table-wrap {
  flex: 1;
  overflow: hidden;
  position: relative;
  margin-top: 10px;
}

.bk-table-wrap :deep(.el-tag) {
  height: 20px;
  padding: 0 7px;
  font-size: 12px;
  border: none;
  border-radius: 4px;
}

.bk-table-wrap :deep(.el-tag--success) {
  background: var(--cs-online);
  color: #fff;
}

.bk-table-wrap :deep(.el-tag--danger) {
  background: var(--cs-error);
  color: #fff;
}

.row-link {
  color: var(--cs-active);
  font-size: 13px;
  cursor: pointer;
  margin-right: 10px;
}

.row-link:hover {
  text-decoration: underline;
}

.bk-danger {
  color: var(--cs-error);
  margin-right: 0;
}
</style>
