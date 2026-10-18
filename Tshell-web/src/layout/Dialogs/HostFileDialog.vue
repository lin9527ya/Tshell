<template>
  <el-dialog
    :model-value="modelValue"
    :title="i18nStore.tl('文件托管', 'Host File')"
    width="540px"
    class="cs-dialog"
    :close-on-click-modal="false"
    @update:model-value="$emit('update:modelValue', $event)"
  >
    <div class="cs-dialog-body cs-font-ui">
      <div class="cs-dialog-desc">
        {{ i18nStore.tl('上传文件经投递通道(/d/)对外分发,支持 MIME 伪装、一次性下载与配额限制;托管记录出现在站点管理列表,可随时关闭/删除。单文件上限 50MB。', 'Host a file via the delivery hub (/d/). 50MB max per file.') }}
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label"><span class="req">*</span> {{ i18nStore.tl('托管文件:', 'File:') }}</label>
        <div class="cs-form-input-wrap has-append">
          <el-input v-model="form.fileName" size="small" :placeholder="i18nStore.tl('点击右侧选择文件…', 'Choose a file…')" readonly />
          <button class="cs-btn cs-btn-xs" :title="i18nStore.tl('选择文件', 'Choose File')" @click="chooseFile">📄</button>
        </div>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('URI 伪装:', 'URI Path:') }}</label>
        <div class="cs-form-input-wrap">
          <el-input v-model="form.uriPath" size="small" placeholder="/download/file.ext" />
        </div>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('MIME 类型:', 'Mime Type:') }}</label>
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
        <label class="cs-form-label">{{ i18nStore.tl('下载限制:', 'Limits:') }}</label>
        <div class="cs-form-input-wrap" style="display:flex; align-items:center; gap:14px;">
          <el-checkbox v-model="form.oneTime">{{ i18nStore.tl('一次性下载', 'One-time') }}</el-checkbox>
          <el-input-number v-model="form.maxDownloads" size="small" :min="0" :max="99999" :placeholder="i18nStore.tl('次数上限(0=不限)', 'Max downloads')" style="width: 170px" />
        </div>
      </div>

      <div v-if="resultUrl" class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('分发链接:', 'URL:') }}</label>
        <div class="cs-form-input-wrap has-append">
          <el-input :model-value="resultUrl" size="small" readonly class="cs-font-mono" />
          <button class="cs-btn cs-btn-xs" @click="copyUrl">{{ i18nStore.tl('复制', 'Copy') }}</button>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="cs-dialog-footer-center">
        <button class="cs-btn cs-btn-primary" :disabled="loading" @click="handleLaunch">
          {{ loading ? (i18nStore.tl('上传中…', 'Uploading…')) : (i18nStore.tl('启动托管', 'Launch')) }}
        </button>
        <button class="cs-btn" @click="handleHelp">
          {{ i18nStore.tl('帮助', 'Help') }}
        </button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18nStore } from '@/stores/i18n'
import ClientAPI from '@/api/clients'

const props = defineProps<{
  modelValue: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', val: boolean): void
}>()

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const loading = ref(false)
const resultUrl = ref('')
const fileObj = ref<File | null>(null)
const form = ref({
  fileName: '',
  uriPath: '',
  mimeType: 'automatic',
  oneTime: false,
  maxDownloads: 0,
})

function chooseFile() {
  const input = document.createElement('input')
  input.type = 'file'
  input.onchange = (e: any) => {
    const file = e.target.files?.[0]
    if (file) {
      fileObj.value = file
      form.value.fileName = file.name
      resultUrl.value = ''
    }
  }
  input.click()
}

async function handleLaunch() {
  if (!fileObj.value) {
    ElMessage.warning(isZh.value ? '请选择要托管的文件' : 'Please choose a file')
    return
  }
  loading.value = true
  try {
    const res = await ClientAPI.host_file({
      file: fileObj.value,
      contentType: form.value.mimeType === 'automatic' ? undefined : form.value.mimeType,
      uriPath: form.value.uriPath || undefined,
      oneTime: form.value.oneTime ? '1' : undefined,
      maxDownloads: form.value.maxDownloads > 0 ? String(form.value.maxDownloads) : undefined,
    })
    const url = res.data?.data?.url
    if (res.data?.status === 200 && url) {
      resultUrl.value = url
      ElMessage.success(isZh.value ? `托管成功: ${url}` : `Hosted: ${url}`)
    } else {
      ElMessage.error(res.data?.error || (isZh.value ? '托管失败' : 'Failed to host file'))
    }
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || (isZh.value ? '托管失败: ' : 'Failed: ') + (e?.message || e))
  } finally {
    loading.value = false
  }
}

function copyUrl() {
  if (!resultUrl.value) return
  navigator.clipboard?.writeText(resultUrl.value).then(() => {
    ElMessage.success(isZh.value ? '已复制' : 'Copied')
  }).catch(() => {})
}

function handleHelp() {
  ElMessage.info(isZh.value ? '文件经投递通道 /d/{key}/full 分发;访问受 UA/令牌控制,一次性下载后链接失效。关闭与删除在「站点管理」列表操作。' : 'Host File Manual')
}
</script>

<style scoped>
.cs-form-label .req {
  color: var(--cs-error);
}
.cs-form-input-wrap.has-append {
  display: flex;
  gap: 8px;
}
</style>
