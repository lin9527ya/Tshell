<template>
  <el-dialog
    v-model="visible"
    :title="isEdit ? (isZh ? '编辑监听器' : 'Edit Listener') : (isZh ? '新建监听器' : 'New Listener')"
    width="560px"
    append-to-body
    class="cs-dialog"
    :close-on-click-modal="false"
  >
    <div class="cs-dialog-body cs-font-ui">
      <div class="cs-dialog-desc">{{ isZh ? '配置 C2 监听器。VKey 和 Salt 必须使用自定义值。' : 'Configure a C2 listener. VKey and Salt must be custom values.' }}</div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '协议类型' : 'Type / Protocol' }}:</label>
        <el-select v-model="form.type" size="small" class="cs-form-input">
          <el-option-group v-if="scope === 'reverse'" :label="isZh ? '反向监听' : 'Reverse Listeners'">
            <el-option label="TCP" value="tcp" />
            <el-option label="KCP" value="kcp" />
            <el-option label="WebSocket" value="websocket" />
            <el-option label="HTTP" value="http" />
            <el-option label="HTTPS" value="https" />
            <el-option label="DNS" value="dns" />
            <el-option label="OSS" value="oss" />
          </el-option-group>
          <el-option-group v-else :label="isZh ? '正向监听' : 'Forward Listeners'">
            <el-option label="TCP Forward" value="tcp_forward" />
            <el-option label="KCP Forward" value="kcp_forward" />
            <el-option label="WebSocket Forward" value="ws_forward" />
            <el-option label="HTTP Forward" value="http_forward" />
            <el-option label="HTTPS Forward" value="https_forward" />
          </el-option-group>
        </el-select>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '监听地址' : 'Listen Address' }}:</label>
        <el-input v-model="form.listenAddress" size="small" placeholder="0.0.0.0:8089" class="cs-form-input" />
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '连接地址' : 'Connect Address' }}:</label>
        <el-input v-model="form.connectAddress" size="small" placeholder="public.domain.com:8089" class="cs-form-input" />
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '备注' : 'Note' }}:</label>
        <el-input v-model="form.note" size="small" placeholder="HTTPS C2" class="cs-form-input" />
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '心跳超时' : 'Heartbeat Timeout' }}:</label>
        <el-input v-model="form.retries" size="small" placeholder="30" class="cs-form-input" />
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '心跳间隔' : 'Heartbeat Interval' }}:</label>
        <el-input v-model="form.interval" size="small" placeholder="10" class="cs-form-input" />
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? 'VKey' : 'VKey' }}:</label>
        <div class="cs-input-with-btn">
          <el-input v-model="form.vkey" size="small" show-password :placeholder="isZh ? '监听器主密钥' : 'Listener master key'" class="cs-form-input" />
          <button class="cs-btn cs-btn-xs" type="button" @click="generateVkey">{{ isZh ? '随机' : 'Random' }}</button>
        </div>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '加密盐' : 'Encryption Salt' }}:</label>
        <div class="cs-input-with-btn">
          <el-input v-model="form.encryptionSalt" size="small" show-password :placeholder="isZh ? '流量加密盐' : 'Traffic encryption salt'" class="cs-form-input" />
          <button class="cs-btn cs-btn-xs" type="button" @click="generateSalt">{{ isZh ? '随机' : 'Random' }}</button>
        </div>
      </div>

      <!--
        该监听器绑定的 C2 画像：决定 http/https 注册哪个 URI、websocket 监听哪个路径。
        生成载荷时必须选**同一个**画像，否则服务端会拒绝（而不是生成一个连不上的载荷）。
        运行中不可改（改 DB 而句柄仍挂在旧路径上会变成僵尸监听器），与地址/类型同规则。
      -->
      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? 'C2 画像' : 'C2 Profile' }}:</label>
        <el-select v-model="form.profile" size="small" class="cs-form-input" :disabled="running">
          <el-option :label="isZh ? 'default（跟随默认特征）' : 'default'" value="default" />
          <el-option v-for="p in profileOptions" :key="p" :label="p" :value="p" />
        </el-select>
      </div>
      <div v-if="running" class="cs-dialog-desc">
        {{ isZh ? '监听器运行中，需先停止才能修改画像。' : 'Stop the listener before changing its profile.' }}
      </div>
    </div>

    <template #footer>
      <div class="cs-dialog-footer-center">
        <button class="cs-btn cs-btn-primary" :disabled="saving" @click="handleSave">
          {{ saving ? (isZh ? '保存中...' : 'Saving...') : (isZh ? '保存' : 'Save') }}
        </button>
        <button class="cs-btn" @click="visible = false">{{ isZh ? '取消' : 'Cancel' }}</button>
        <button class="cs-btn" @click="handleHelp">{{ isZh ? '帮助' : 'Help' }}</button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18nStore } from '@/stores/i18n'
import c2profileApi from '@/api/c2profile'

const props = defineProps<{
  modelValue: boolean
  listenerData?: any
  /** reverse: 反向监听(监听器页); forward: 正向监听(正向监听器页) */
  scope?: 'reverse' | 'forward'
  saving?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', val: boolean): void
  (e: 'save', data: any): void
}>()

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v),
})

const isEdit = computed(() => !!props.listenerData?.Id || !!props.listenerData?.id || !!props.listenerData?.ID)

const defaultType = computed(() => (props.scope === 'forward' ? 'tcp_forward' : 'tcp'))

const form = reactive({
  type: 'tcp',
  listenAddress: '',
  connectAddress: '',
  note: '',
  retries: '30',
  interval: '10',
  vkey: '',
  encryptionSalt: '',
  // 该监听器绑定的 C2 画像（空/未选 = default）
  profile: 'default',
})

// 运行中禁止改画像（后端也会拒）：把"停止后才能改"提前告诉用户，
// 而不是让他填完点保存再吃一个 400
const running = computed(
  () => Number(props.listenerData?.Status ?? props.listenerData?.status ?? 0) === 1,
)

// 可选画像列表：default 之外的其它画像
const profileOptions = ref<string[]>([])

onMounted(async () => {
  try {
    const res = await c2profileApi.listProfiles()
    const list: string[] = res.data?.data || res.data?.profiles || []
    profileOptions.value = list.filter((n) => n && n !== 'default')
  } catch {
    // 拉不到就只提供 default：不阻塞编辑，保存时后端还会校验
    profileOptions.value = []
  }
})

function randomHex(bytes: number): string {
  const arr = new Uint8Array(bytes)
  crypto.getRandomValues(arr)
  return Array.from(arr).map((b) => b.toString(16).padStart(2, '0')).join('')
}

function generateVkey() {
  form.vkey = randomHex(32)
}

function generateSalt() {
  form.encryptionSalt = randomHex(16)
}

watch(
  () => props.listenerData,
  (val) => {
    if (val) {
      form.type = val.Type || val.type || defaultType.value
      form.listenAddress = val.ListenAddress || val.listenAddress || ''
      form.connectAddress = val.ConnectAddress || val.connectAddress || ''
      form.note = val.Note || val.note || ''
      form.retries = String(val.HeartbeatTimeout ?? val.Retries ?? val.retries ?? '30')
      form.interval = String(val.HeartbeatInterval ?? val.Interval ?? val.interval ?? '10')
      form.vkey = val.VKey || val.vkey || ''
      form.encryptionSalt = val.EncryptionSalt || val.encryptionSalt || ''
    } else {
      form.type = defaultType.value
      form.listenAddress = ''
      form.connectAddress = ''
      form.note = ''
      form.retries = '30'
      form.interval = '10'
      form.vkey = ''
      form.encryptionSalt = ''
    }
  },
  { immediate: true },
)

function handleSave() {
  if (!form.type.trim()) {
    ElMessage.warning(isZh.value ? '请选择协议类型' : 'Type is required')
    return
  }
  if (!form.listenAddress.trim()) {
    ElMessage.warning(isZh.value ? '请填写监听地址' : 'Listen Address is required')
    return
  }
  emit('save', {
    type: form.type.trim(),
    listenAddress: form.listenAddress.trim(),
    connectAddress: form.connectAddress.trim(),
    note: form.note.trim(),
    retries: form.retries.trim() || '30',
    interval: form.interval.trim() || '10',
    vkey: form.vkey.trim(),
    encryptionSalt: form.encryptionSalt.trim(),
  })
}

function handleHelp() {
  ElMessage.info(isZh.value ? 'C2 监听器配置说明：VKey/Salt 用于会话密钥派生，请勿使用默认值。' : 'C2 Listener Configuration Manual: VKey/Salt are used for session key derivation, do not use default values.')
}
</script>

<style scoped>
.cs-input-with-btn {
  flex: 1;
  display: flex;
  gap: 8px;
}
</style>
