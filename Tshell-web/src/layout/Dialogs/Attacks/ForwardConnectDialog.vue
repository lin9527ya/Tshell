<template>
  <el-dialog
    v-model="visible"
    :title="i18nStore.tl('正向连接客户端', 'Forward Connect Client')"
    width="520px"
    append-to-body
    class="cs-dialog"
    :close-on-click-modal="false"
  >
    <div class="cs-dialog-body cs-font-ui">
      <div class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('连接方式', 'Connection Method') }}:</label>
        <el-select v-model="form.connectionMethod" size="small" class="cs-form-input">
          <el-option label="TCP" value="TCP" />
          <el-option label="KCP/UDP" value="KCP/UDP" />
          <el-option label="WebSocket" value="WebSocket" />
          <el-option label="HTTP" value="HTTP" />
          <el-option label="HTTPS" value="HTTPS" />
        </el-select>
      </div>
      <div class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('IP / 域名', 'IP / Domain') }}:</label>
        <el-input v-model="form.ipAddress" class="cs-form-input" :placeholder="i18nStore.tl('正向客户端监听 IP 或域名', 'Forward client listener IP / domain')" />
      </div>
      <div class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('端口', 'Port') }}:</label>
        <el-input v-model="form.port" class="cs-form-input" placeholder="8080" />
      </div>
      <div class="cs-form-row">
        <label class="cs-form-label">VKey:</label>
        <el-input v-model="form.vkey" class="cs-form-input" show-password :placeholder="i18nStore.tl('可选，会话密钥', 'Optional session key')" />
      </div>
      <div class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('加密盐', 'Encryption Salt') }}:</label>
        <el-input v-model="form.encryptionSalt" class="cs-form-input" show-password :placeholder="i18nStore.tl('可选，流量加密盐', 'Optional traffic salt')" />
      </div>
      <div class="cs-form-row">
        <label class="cs-form-label">eBPF:</label>
        <el-switch v-model="form.isEbpfClient" />
      </div>
    </div>
    <template #footer>
      <div class="cs-dialog-footer-center">
        <button class="cs-btn cs-btn-primary" @click="handleConnect">{{ i18nStore.tl('连接', 'Connect') }}</button>
        <button class="cs-btn" @click="visible = false">{{ i18nStore.tl('取消', 'Cancel') }}</button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, reactive } from 'vue'
import { ElMessage } from 'element-plus'
import ClientAPI from '@/api/clients'
import { useI18nStore } from '@/stores/i18n'

const props = defineProps<{ modelValue: boolean }>()
const emit = defineEmits<{ (e: 'update:modelValue', val: boolean): void }>()

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v),
})

const form = reactive({
  connectionMethod: 'TCP',
  ipAddress: '',
  port: '',
  vkey: '',
  encryptionSalt: '',
  isEbpfClient: false,
})

async function handleConnect() {
  if (!form.ipAddress.trim() || !form.port.trim()) {
    ElMessage.warning(isZh.value ? '请填写 IP 和端口' : 'IP and port are required')
    return
  }
  // 0.0.0.0/:: 是客户端 bind 地址，拨过去只会连到服务端本机
  const host = form.ipAddress.trim().toLowerCase().replace(/^\[|\]$/g, '')
  if (host === '0.0.0.0' || host === '::') {
    ElMessage.warning(isZh.value ? '回连地址不能用 0.0.0.0/::，请填客户端可达的 IP 或域名' : 'Use the client reachable IP/domain, not 0.0.0.0/::')
    return
  }
  try {
    const res = await ClientAPI.connect_forward_client({
      isEbpfClient: form.isEbpfClient,
      connectionMethod: form.connectionMethod,
      ipAddress: form.ipAddress.trim(),
      port: form.port.trim(),
      password: '',
      vkey: form.vkey.trim(),
      encryptionSalt: form.encryptionSalt.trim(),
    })
    if (res.data?.status === 200) {
      ElMessage.success(res.data?.message || (isZh.value ? '正向连接建立成功' : 'Forward connection established'))
      visible.value = false
      form.ipAddress = ''
      form.port = ''
      form.vkey = ''
      form.encryptionSalt = ''
    } else {
      ElMessage.error(res.data?.message || res.data?.error || (isZh.value ? '正向连接失败' : 'Forward connection failed'))
    }
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '正向连接失败' : 'Forward connection failed'))
  }
}
</script>

