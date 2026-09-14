<template>
  <el-dialog
    v-model="visible"
    :title="isZh ? '自签名 Applet 攻击' : 'Self-signed Applet Attack'"
    width="440px"
    append-to-body
    class="cs-dialog"
    :close-on-click-modal="false"
  >
    <div class="cs-dialog-body">
      <div class="cs-dialog-desc">
        {{ isZh ? '部署自签名 Java Applet 钓鱼载荷。当受害者点击允许运行时触发监听器回连。' : 'This package sets up a self-signed Java applet. This package will spawn the specified listener if the user allows the applet to run.' }}
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '本地 URI:' : 'Local URI:' }}</label>
        <el-input v-model="form.uri" size="small" placeholder="/mPlayer" class="cs-form-input" />
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '本地监听主机:' : 'Local Host:' }}</label>
        <el-input v-model="form.host" size="small" placeholder="192.168.10.4" class="cs-form-input" />
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '本地端口:' : 'Local Port:' }}</label>
        <el-input v-model="form.port" size="small" placeholder="80" class="cs-form-input" />
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '监听器:' : 'Listener:' }}</label>
        <div class="cs-input-with-btn">
          <el-input v-model="form.listener" size="small" placeholder="HTTPS" class="cs-form-input" />
          <button class="cs-picker-btn" :title="isZh ? '选择载荷' : 'Choose a payload'" @click="showPicker = true">...</button>
        </div>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">SSL:</label>
        <label class="cs-checkbox-wrap">
          <input type="checkbox" v-model="form.ssl" />
          <span>{{ isZh ? '启用 SSL / HTTPS 加密' : 'Enable SSL' }}</span>
        </label>
      </div>
    </div>

    <template #footer>
      <div class="cs-dialog-footer-center">
        <button class="cs-btn cs-btn-primary" @click="handleLaunch">{{ isZh ? '启动' : 'Launch' }}</button>
        <button class="cs-btn" @click="handleHelp">{{ isZh ? '帮助' : 'Help' }}</button>
      </div>
    </template>

    <ChoosePayloadDialog
      v-model="showPicker"
      :title="isZh ? '选择载荷' : 'Choose a payload'"
      @choose="onChooseListener"
    />
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18nStore } from '@/stores/i18n'
import ChoosePayloadDialog, { type ListenerOption } from '../ChoosePayloadDialog.vue'

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

const showPicker = ref(false)

const form = reactive({
  uri: '/mPlayer',
  host: '192.168.10.4',
  port: '80',
  listener: 'HTTPS',
  ssl: false,
})

function onChooseListener(l: ListenerOption) {
  form.listener = l.name
}

function handleLaunch() {
  // 演示功能：后端未实现 Applet Web 服务
  ElMessage.warning(isZh.value ? '演示功能：后端未实现该服务（无对应 API），不会实际启动' : 'Demo only: this service is not implemented on the backend (no API), nothing will start')
}

function handleHelp() {
  ElMessage.info('Self-signed Applet Attack Help')
}
</script>

<style scoped>
.cs-dialog-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.cs-dialog-desc {
  font-size: 12px;
  color: var(--cs-text-dim);
  line-height: 1.4;
}
.cs-form-row {
  display: flex;
  align-items: center;
  gap: 10px;
}
.cs-form-label {
  width: 80px;
  font-size: 12px;
  color: var(--cs-text);
  flex-shrink: 0;
}
.cs-form-input {
  flex: 1;
}
.cs-input-with-btn {
  flex: 1;
  display: flex;
  gap: 4px;
}
.cs-picker-btn {
  width: 24px;
  height: 24px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-border);
  border-radius: 2px;
  color: var(--cs-text);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 11px;
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
