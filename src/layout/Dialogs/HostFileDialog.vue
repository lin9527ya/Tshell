<template>
  <el-dialog
    :model-value="modelValue"
    :title="isZh ? '文件托管' : 'Host File'"
    width="540px"
    class="cs-dialog"
    :close-on-click-modal="false"
    @update:model-value="$emit('update:modelValue', $event)"
  >
    <div class="cs-dialog-body cs-font-ui">
      <div class="cs-dialog-desc">
        {{ isZh ? '通过 BladeShell 内置 Web 服务器托管并对外分发任意载荷或文件。' : 'Host a file through Cobalt Strike\'s web server.' }}
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label"><span class="req">*</span> {{ isZh ? '托管文件:' : 'File:' }}</label>
        <div class="cs-form-input-wrap has-append">
          <el-input v-model="form.file" size="small" placeholder="/opt/payloads/update.exe" />
          <button class="cs-btn cs-btn-xs" :title="isZh ? '选择文件' : 'Choose File'" @click="chooseFile">📄</button>
        </div>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label"><span class="req">*</span> {{ isZh ? '本地 URI:' : 'Local URI:' }}</label>
        <div class="cs-form-input-wrap">
          <el-input v-model="form.localUri" size="small" placeholder="/download/file.ext" />
        </div>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label"><span class="req">*</span> {{ isZh ? '本地主机:' : 'Local Host:' }}</label>
        <div class="cs-form-input-wrap">
          <el-input v-model="form.localHost" size="small" placeholder="192.168.10.4" />
        </div>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label"><span class="req">*</span> {{ isZh ? '本地端口:' : 'Local Port:' }}</label>
        <div class="cs-form-input-wrap">
          <el-input-number v-model="form.localPort" size="small" :min="1" :max="65535" style="width: 100%" />
        </div>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? 'MIME 类型:' : 'Mime Type:' }}</label>
        <div class="cs-form-input-wrap">
          <el-select v-model="form.mimeType" size="small" style="width: 100%">
            <el-option label="automatic" value="automatic" />
            <el-option label="application/octet-stream" value="application/octet-stream" />
            <el-option label="application/x-msdownload" value="application/x-msdownload" />
            <el-option label="text/html" value="text/html" />
            <el-option label="text/plain" value="text/plain" />
            <el-option label="application/javascript" value="application/javascript" />
            <el-option label="image/png" value="image/png" />
          </el-select>
        </div>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">SSL:</label>
        <div class="cs-form-input-wrap">
          <el-checkbox v-model="form.ssl">{{ isZh ? '启用 SSL (HTTPS)' : 'Enable SSL' }}</el-checkbox>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="cs-dialog-footer-center">
        <button class="cs-btn cs-btn-primary" :disabled="loading" @click="handleLaunch">
          {{ isZh ? '启动托管' : 'Launch' }}
        </button>
        <button class="cs-btn" @click="handleHelp">
          {{ isZh ? '帮助' : 'Help' }}
        </button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18nStore } from '@/stores/i18n'
import { useWorkspaceStore } from '@/stores/workspace'

const props = defineProps<{
  modelValue: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', val: boolean): void
}>()

const i18nStore = useI18nStore()
const workspace = useWorkspaceStore()
const isZh = computed(() => i18nStore.isZh)

const loading = ref(false)
const form = ref({
  file: '/root/payload.exe',
  localUri: '/download/payload.exe',
  localHost: '192.168.10.4',
  localPort: 80,
  mimeType: 'automatic',
  ssl: false,
})

function chooseFile() {
  const input = document.createElement('input')
  input.type = 'file'
  input.onchange = (e: any) => {
    const file = e.target.files?.[0]
    if (file) {
      form.value.file = `/uploads/${file.name}`
      form.value.localUri = `/download/${file.name}`
      ElMessage.success(isZh.value ? `已选择文件: ${file.name}` : `Selected file: ${file.name}`)
    }
  }
  input.click()
}

function handleLaunch() {
  if (!form.value.file) {
    ElMessage.warning(isZh.value ? '请指定要托管的文件' : 'Please specify a file')
    return
  }
  // 演示功能：后端未实现文件托管 Web 服务
  ElMessage.warning(isZh.value ? '演示功能：后端未实现文件托管服务（Web 托管未接入），不会实际启动任何服务' : 'Demo only: file hosting is not implemented on the backend (no web hosting), nothing will actually start')
}

function handleHelp() {
  ElMessage.info(isZh.value ? '文件托管工具用于在 Web 服务器上快速提供文件下载链接。' : 'Host File Manual')
}
</script>

<style scoped>
.cs-dialog-desc {
  font-size: 12px;
  color: var(--cs-text-dim);
  background: var(--cs-bg);
  padding: 8px 12px;
  border: 1px solid var(--cs-border);
  border-radius: 2px;
  margin-bottom: 12px;
  line-height: 1.4;
}

.cs-form-row {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 10px;
}

.cs-form-label {
  width: 130px;
  text-align: right;
  font-size: 12px;
  color: var(--cs-text-dim);
  flex-shrink: 0;
}

.cs-form-label .req {
  color: #f56c6c;
}

.cs-form-input-wrap {
  flex: 1;
}

.cs-form-input-wrap.has-append {
  display: flex;
  gap: 4px;
}

.cs-dialog-footer-center {
  display: flex;
  justify-content: center;
  gap: 10px;
  padding: 6px 0;
}
</style>
