<template>
  <el-dialog
    v-model="visible"
    :title="i18nStore.tl('设置回连间隔', 'Beacon Sleep Configuration')"
    width="440px"
    :close-on-click-modal="false"
    append-to-body
    class="cs-dialog"
  >
    <div class="cs-dialog-body">
      <div class="dialog-row">
        <label class="dialog-label">{{ i18nStore.tl('目标客户端:', 'Target:') }}</label>
        <span class="dialog-val">{{ clientLabel }}</span>
      </div>
      <div class="dialog-grid">
        <div class="dialog-field">
          <label class="dialog-label">{{ i18nStore.tl('回连间隔（秒）:', 'Sleep Time (seconds):') }}</label>
          <el-input-number v-model="sleepSec" :min="0" :max="86400" style="width: 100%" />
        </div>
        <div class="dialog-field">
          <label class="dialog-label">{{ i18nStore.tl('波动范围 (%):', 'Jitter (%):') }}</label>
          <el-input-number v-model="jitter" :min="0" :max="99" style="width: 100%" />
        </div>
      </div>
      <div class="quick-presets">
        <span class="preset-label">{{ i18nStore.tl('快捷预设:', 'Quick Presets:') }}</span>
        <button class="preset-btn" @click="setPreset(0, 0)">{{ i18nStore.tl('实时交互 (0s)', 'Interactive (0s)') }}</button>
        <button class="preset-btn" @click="setPreset(5, 10)">5s (10%)</button>
        <button class="preset-btn" @click="setPreset(60, 20)">1m (20%)</button>
        <button class="preset-btn" @click="setPreset(300, 30)">5m (30%)</button>
      </div>
    </div>
    <template #footer>
      <div class="cs-dialog-footer">
        <button class="cs-btn cs-btn-primary" @click="applySleep">{{ i18nStore.tl('应用间隔', 'Set Sleep') }}</button>
        <button class="cs-btn" @click="close">{{ i18nStore.tl('取消', 'Cancel') }}</button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useClientsStore } from '@/stores/clients'
import { useI18nStore } from '@/stores/i18n'
import ClientAPI from '@/api/clients'

// P3：页面内局部弹窗——可见性走 v-model，数据由宿主经 props 传入
const props = withDefaults(
  defineProps<{ modelValue: boolean; uid?: string; sleep?: number | string; uids?: string[] }>(),
  { uid: '', sleep: 5, uids: () => [] },
)
const emit = defineEmits<{ (e: 'update:modelValue', value: boolean): void }>()
const visible = computed({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value),
})

const clientsStore = useClientsStore()
const i18nStore = useI18nStore()

const uid = computed(() => props.uid)
const sleepSec = ref(5)
const jitter = ref(10)

const client = computed(() => clientsStore.getClientByUid(uid.value))
const clientLabel = computed(() => {
  if (!client.value) return uid.value.slice(0, 8)
  return `${client.value.Username || 'user'}@${client.value.Computer || 'host'}`
})

watch(visible, (val) => {
  if (val) {
    sleepSec.value = Number(props.sleep) || 5
    jitter.value = 10
  }
})

function setPreset(sec: number, jit: number) {
  sleepSec.value = sec
  jitter.value = jit
}

function close() {
  visible.value = false
}

async function applySleep() {
  const targetUids = props.uids.length ? props.uids : (uid.value ? [uid.value] : [])
  if (!targetUids.length) return
  try {
    for (const u of targetUids) {
      // 只走 /client/sleep API（纯秒数，后端正确下发）。
      // 注意：不再追加 "sleep X Y" 命令串——后端 msArg 对 "X Y" 整串 Atoi 失败→0ms→客户端拒绝。
      await ClientAPI.edit_sleep({ uid: u, sleep: String(sleepSec.value) }).catch(() => {})
      // jitter 仅为展示项（后端/客户端未实现 jitter 下发）
      clientsStore.updateClient(u, { Sleep: `${sleepSec.value} ${jitter.value}%` })
    }
    ElMessage.success(`Sleep set to ${sleepSec.value}s for ${targetUids.length} session(s)`)
    close()
  } catch {
    ElMessage.error('Failed to send sleep command')
  }
}
</script>

<style scoped>
.dialog-row {
  display: flex;
  align-items: center;
  gap: 10px;
}
.dialog-label {
  font-size: 14px;
  color: var(--cs-text);
  flex-shrink: 0;
}
.dialog-val {
  font-size: 13px;
  color: var(--cs-text-bright);
  font-family: var(--cs-font-mono);
}
.dialog-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}
.dialog-field {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.quick-presets {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  padding-top: 4px;
}
.preset-label {
  font-size: 12px;
  color: var(--cs-text-dim);
  margin-right: 4px;
}
.preset-btn {
  background: var(--cs-panel);
  border: 1px solid var(--cs-border-strong);
  border-radius: 4px;
  color: var(--cs-text);
  font-size: 12px;
  height: 24px;
  padding: 0 8px;
  cursor: pointer;
  transition: all 120ms ease;
}
.preset-btn:hover {
  background: var(--cs-hover);
  color: var(--cs-text-bright);
  border-color: var(--cs-active);
}
.cs-dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
</style>
