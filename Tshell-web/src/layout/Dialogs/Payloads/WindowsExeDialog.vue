<template>
  <el-dialog
    v-model="visible"
    :title="isZh ? 'Windows 分阶段可执行程序' : 'Windows Executable (Staged)'"
    width="700px"
    append-to-body
    class="cs-dialog"
    :close-on-click-modal="false"
  >
    <div class="cs-dialog-body">
      <div class="cs-dialog-desc">
        {{ isZh ? '生成 Windows 分阶段可执行程序（stager EXE，运行后从 WebDelivery 拉取完整载荷执行）。架构由所选投递决定。' : 'Generates a staged Windows executable (stager EXE that pulls and runs the full payload from a WebDelivery). Architecture follows the selected delivery.' }}
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
        <label class="cs-form-label">{{ isZh ? '上线密码:' : 'Pass:' }}</label>
        <el-input v-model="form.pass" size="small" :placeholder="isZh ? '客户端启动上线密码（可选）' : 'Beacon startup password (optional)'" class="cs-form-input" />
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
  pass: '',
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
    // 分阶段 stager EXE：后端 /shellcode/stage 按投递架构生成（运行后拉取完整载荷）
    const res = await ClientAPI.StageShellCodeGen({
      listener: field(wd, 'ListenerConfig', 'listener_config') || '',
      port: form.deliveryPort,
      format: 'exe',
    })
    if (res && res.status === 200) {
      if (downloadBlob(res, `staged_stager_${form.deliveryPort}.exe`)) {
        ElMessage.success(isZh.value ? '分阶段可执行程序生成成功' : 'Staged executable generated')
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
  ElMessage.info(isZh.value ? '分阶段 EXE 为 stager 程序：运行后从 WebDelivery 地址拉取并执行完整载荷，架构与投递一致（x86/x64）' : 'Staged EXE is a stager that pulls and runs the full payload from the WebDelivery URL; architecture matches the delivery (x86/x64)')
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
.cs-input-with-btn {
  flex: 1;
  display: flex;
  gap: 6px;
}
.cs-picker-btn {
  width: 28px;
  height: 28px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-border);
  border-radius: 2px;
  color: var(--cs-text);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 14px;
}
.cs-picker-btn:hover {
  background: var(--cs-hover);
  color: var(--cs-text-bright);
}
.cs-checkbox-wrap {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--cs-text);
  cursor: pointer;
}
.cs-dialog-footer-center {
  display: flex;
  justify-content: center;
  gap: 8px;
}
</style>
