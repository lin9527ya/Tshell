<template>
  <el-dialog
    v-model="visible"
    :title="isZh ? '分阶段载荷生成器' : 'Payload Generator'"
    width="700px"
    append-to-body
    class="cs-dialog"
    :close-on-click-modal="false"
  >
    <div class="cs-dialog-body">
      <div class="cs-dialog-desc">
        {{ isZh ? '基于 WebDelivery 托管的分阶段 Stager 载荷（后端 /shellcode/stage + donut 转换）。需先有运行中的 WebDelivery 投递。' : 'Generates a staged stager payload hosted by WebDelivery (backend /shellcode/stage + donut). Requires a running WebDelivery.' }}
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? 'WebDelivery:' : 'Web Delivery:' }}</label>
        <el-select
          v-model="form.deliveryPort"
          size="small"
          class="cs-form-input"
          :placeholder="isZh ? '选择 WebDelivery 投递' : 'Select a WebDelivery'"
          @visible-change="onDropdown"
        >
          <el-option
            v-for="wd in webDeliveryOptions"
            :key="String(field(wd, 'ListeningPort', 'listening_port'))"
            :label="wdLabel(wd)"
            :value="String(field(wd, 'ListeningPort', 'listening_port'))"
          />
        </el-select>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '输出格式:' : 'Output:' }}</label>
        <el-select v-model="form.format" size="small" class="cs-form-input">
          <el-option label="exe" value="exe" />
          <el-option label="raw" value="raw" />
          <el-option label="bin" value="bin" />
          <el-option label="hex" value="hex" />
          <el-option label="c" value="c" />
        </el-select>
      </div>

      <div class="cs-form-hint">
        {{ isZh ? '格式说明：exe/raw/bin 为二进制载荷，hex 为十六进制文本，c 为 C 语言数组源码。' : 'Formats: exe/raw/bin binary, hex hex text, c C source array.' }}
      </div>
    </div>

    <template #footer>
      <div class="cs-dialog-footer-center">
        <button class="cs-btn cs-btn-primary" :disabled="generating || !form.deliveryPort" @click="handleGenerate">
          {{ generating ? (isZh ? '生成中...' : 'Generating...') : (isZh ? '生成' : 'Generate') }}
        </button>
        <button class="cs-btn" @click="handleHelp">{{ isZh ? '帮助' : 'Help' }}</button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18nStore } from '@/stores/i18n'
import ClientAPI from '@/api/clients'
import { fetchWebDeliveryOptions, wdLabel, field, extractBlobError, downloadBlob, hintNoWebDelivery, type WebDeliveryRow } from './payloadUtil'

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const props = defineProps<{
  modelValue: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', val: boolean): void
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v),
})

const webDeliveryOptions = ref<WebDeliveryRow[]>([])
const generating = ref(false)

const form = reactive({
  deliveryPort: '',
  format: 'bin',
})

watch(
  () => props.modelValue,
  (val) => {
    if (val) {
      form.deliveryPort = ''
      onDropdown(true)
    }
  },
)

async function onDropdown(visibleState?: boolean) {
  if (visibleState === false) return
  webDeliveryOptions.value = await fetchWebDeliveryOptions()
}

async function handleGenerate() {
  if (!form.deliveryPort) {
    ElMessage.warning(isZh.value ? '请选择 WebDelivery' : 'Select a WebDelivery first')
    return
  }
  const wd = webDeliveryOptions.value.find(
    (w) => String(field(w, 'ListeningPort', 'listening_port')) === form.deliveryPort,
  )
  if (!wd) {
    hintNoWebDelivery()
    return
  }
  generating.value = true
  try {
    const res = await ClientAPI.StageShellCodeGen({
      listener: field(wd, 'ListenerConfig', 'listener_config') || '',
      port: form.deliveryPort,
      format: form.format,
    })
    if (res && res.status === 200) {
      const fmt = form.format
      const ext = fmt === 'exe' ? '.exe' : fmt === 'raw' || fmt === 'bin' ? '.bin' : fmt === 'hex' ? '.txt' : '.c'
      if (downloadBlob(res, `stage_shellcode_${form.deliveryPort}${ext}`)) {
        ElMessage.success(isZh.value ? '分阶段载荷生成成功' : 'Staged payload generated')
        visible.value = false
      } else {
        ElMessage.error(isZh.value ? '生成失败：响应不是可下载文件' : 'Generation failed: response is not a downloadable file')
      }
    } else {
      ElMessage.error(isZh.value ? '生成失败，请检查后端日志' : 'Generation failed, check backend logs')
    }
  } catch (e: any) {
    const msg = await extractBlobError(e)
    ElMessage.error(isZh.value ? `生成失败: ${msg}` : `Generation failed: ${msg}`)
  } finally {
    generating.value = false
  }
}

function handleHelp() {
  ElMessage.info(isZh.value ? 'Stager 由后端根据 WebDelivery 的下载地址构建；Windows 载荷经 donut 转为 shellcode' : 'Stager is built by the backend from the WebDelivery download URL; Windows payloads are converted via donut')
}
</script>

<style scoped>
.cs-dialog-body {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.cs-dialog-desc {
  font-size: 15px;
  color: var(--cs-text-dim);
  line-height: 1.6;
}
.cs-form-row {
  display: flex;
  align-items: center;
  gap: 12px;
}
.cs-form-label {
  min-width: 130px;
  max-width: 150px;
  font-size: 15.5px;
  color: var(--cs-text);
  text-align: right;
  flex-shrink: 0;
}
.cs-form-input {
  flex: 1;
}
.cs-form-hint {
  font-size: 14px;
  color: var(--cs-text-dim);
  line-height: 1.6;
}
.cs-dialog-footer-center {
  display: flex;
  justify-content: center;
  gap: 12px;
}
</style>
