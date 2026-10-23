<template>
  <el-dialog
    v-model="visible"
    :title="i18nStore.tl('会话操作报告', 'Session Report')"
    width="720px"
    :close-on-click-modal="false"
    append-to-body
    class="cs-dialog"
  >
    <div class="cs-dialog-body">
      <div v-loading="loading" class="report-wrap">
        <template v-if="report">
          <div class="report-meta">
            <span class="dialog-label">{{ i18nStore.tl('会话:', 'Session:') }}</span>
            <span class="dialog-val cs-font-mono">{{ report.uid }}</span>
            <span class="dialog-label report-meta-gap">{{ i18nStore.tl('生成时间:', 'Generated:') }}</span>
            <span class="dialog-val cs-font-mono">{{ report.generated_at }}</span>
          </div>

          <!-- 会话录像 -->
          <div class="report-section">
            <div class="report-section-title">{{ i18nStore.tl('会话录像 (Session Logs)', 'Session Logs') }}</div>
            <div v-if="report.session_logs && report.session_logs.length" class="report-log-list">
              <div v-for="log in report.session_logs" :key="log.file" class="report-log-row cs-font-mono">
                <span class="log-file">{{ log.file }}</span>
                <span class="log-size">{{ formatSize(log.size) }}</span>
                <span class="log-time">{{ log.modified }}</span>
                <button
                  v-if="log.downloadable"
                  class="cs-btn cs-btn-xs"
                  :disabled="downloadingFile === log.file"
                  @click="downloadLog(log.file)"
                >
                  {{ downloadingFile === log.file ? (i18nStore.tl('下载中...', 'Downloading...')) : (i18nStore.tl('下载', 'Download')) }}
                </button>
              </div>
            </div>
            <div v-else class="report-empty">{{ i18nStore.tl('暂无会话录像', 'No session logs') }}</div>
          </div>

          <!-- 下载记录 -->
          <div class="report-section">
            <div class="report-section-title">{{ i18nStore.tl('下载记录 (Downloads)', 'Download History') }}</div>
            <div v-if="report.downloads && report.downloads.length" class="report-log-list">
              <div v-for="(d, idx) in report.downloads" :key="idx" class="report-log-row cs-font-mono">
                <span class="log-file">{{ d.file_name ?? d.FileName }}</span>
                <span class="log-size">{{ formatSize(d.downloaded_size ?? d.DownloadedSize) }} / {{ formatSize(d.file_size ?? d.FileSize) }}</span>
                <span class="log-time">{{ d.file_path ?? d.FilePath }}</span>
              </div>
            </div>
            <div v-else class="report-empty">{{ i18nStore.tl('暂无下载记录', 'No download history') }}</div>
          </div>

          <!-- Shell 历史 -->
          <div class="report-section">
            <div class="report-section-title">{{ i18nStore.tl('Shell 历史', 'Shell History') }}</div>
            <pre class="report-shell cs-font-mono">{{ report.shell_history || (i18nStore.tl('（空）', '(empty)')) }}</pre>
          </div>
        </template>

        <div v-else-if="!loading" class="report-empty">
          {{ i18nStore.tl('无法获取报告，请检查会话是否有效', 'Failed to load report') }}
        </div>
      </div>
    </div>

    <template #footer>
      <div class="cs-dialog-footer">
        <button class="cs-btn" @click="close">{{ i18nStore.tl('关闭', 'Close') }}</button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18nStore } from '@/stores/i18n'
import ClientAPI from '@/api/clients'

interface SessionLogInfo {
  file: string
  size: number
  modified: string
  downloadable: boolean
}

interface ReportData {
  uid: string
  generated_at: string
  shell_history: string
  session_logs: SessionLogInfo[]
  downloads: any[]
}

// P3：页面内局部弹窗——可见性走 v-model，数据由宿主经 props 传入
const props = withDefaults(defineProps<{ modelValue: boolean; uid?: string }>(), { uid: '' })
const emit = defineEmits<{ (e: 'update:modelValue', value: boolean): void }>()
const visible = computed({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value),
})

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const uid = computed(() => props.uid)

const loading = ref(false)
const report = ref<ReportData | null>(null)
const downloadingFile = ref('')

function formatSize(size: number): string {
  if (!size && size !== 0) return '-'
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
  return `${(size / 1024 / 1024).toFixed(2)} MB`
}

watch(visible, (val) => {
  if (val) {
    loading.value = true
    report.value = null
    downloadingFile.value = ''
    ClientAPI.get_client_report(uid.value)
      .then((res: any) => {
        if (res.data && res.data.status === 200) {
          report.value = res.data.data
        }
      })
      .catch(() => {
        ElMessage.error(isZh.value ? '获取报告失败' : 'Failed to load report')
      })
      .finally(() => {
        loading.value = false
      })
  }
})

async function downloadLog(file: string) {
  try {
    downloadingFile.value = file
    const res = await ClientAPI.download_session_log(uid.value, file)
    const blob = new Blob([res.data])
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = file
    a.click()
    URL.revokeObjectURL(url)
    ElMessage.success(isZh.value ? '录像下载已开始' : 'Session log download started')
  } catch {
    ElMessage.error(isZh.value ? '录像下载失败' : 'Failed to download session log')
  } finally {
    downloadingFile.value = ''
  }
}

function close() {
  visible.value = false
}
</script>

<style scoped>
.report-wrap {
  min-height: 200px;
  max-height: 60vh;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.report-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.report-meta-gap {
  margin-left: 16px;
}
.dialog-label {
  font-size: 13px;
  color: var(--cs-text-dim);
}
.dialog-val {
  font-size: 13px;
  color: var(--cs-text-bright);
}
.report-section {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.report-section-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text);
}
.report-log-list {
  max-height: 160px;
  overflow-y: auto;
}
.report-log-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 5px 4px;
  font-size: 12px;
  border-bottom: 1px solid var(--cs-card-border);
}
.report-log-row:last-child {
  border-bottom: none;
}
.log-file {
  flex: 1;
  color: var(--cs-text);
  word-break: break-all;
}
.log-size {
  color: var(--cs-text-dim);
  white-space: nowrap;
}
.log-time {
  color: var(--cs-text-dim);
  white-space: nowrap;
}
.report-shell {
  margin: 0;
  padding: 12px;
  max-height: 220px;
  overflow-y: auto;
  font-size: 13px;
  line-height: 1.6;
  color: var(--cs-text);
  white-space: pre-wrap;
  word-break: break-all;
  background: var(--cs-console);
  border: 1px solid var(--cs-border);
  border-radius: 6px;
}
.report-empty {
  padding: 12px;
  font-size: 12px;
  color: var(--cs-text-dim);
  font-style: italic;
}
.cs-dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
</style>
