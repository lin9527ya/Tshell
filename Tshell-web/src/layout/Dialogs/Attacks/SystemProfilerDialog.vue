<template>
  <el-dialog
    v-model="visible"
    :title="isZh ? '系统资产指纹分析' : 'System Profiler'"
    width="440px"
    append-to-body
    class="cs-dialog"
    :close-on-click-modal="false"
  >
    <div class="cs-dialog-body">
      <div class="cs-dialog-desc">
        {{ isZh ? '客户端探针收集工具。用于探测目标浏览器环境、插件版本及系统指纹。' : 'The system profiler is a client-side reconnaissance tool. It finds common applications (with version numbers) used by target.' }}
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '本地 URI:' : 'Local URI:' }}</label>
        <el-input v-model="form.uri" size="small" placeholder="/" class="cs-form-input" />
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
        <label class="cs-form-label">{{ isZh ? '重定向 URL:' : 'Redirect URL:' }}</label>
        <el-input v-model="form.redirectUrl" size="small" class="cs-form-input" />
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">SSL:</label>
        <label class="cs-checkbox-wrap">
          <input type="checkbox" v-model="form.ssl" />
          <span>{{ isZh ? '启用 SSL / HTTPS 加密' : 'Enable SSL' }}</span>
        </label>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label"></label>
        <label class="cs-checkbox-wrap">
          <input type="checkbox" v-model="form.useApplet" />
          <span>{{ isZh ? '使用 Java Applet 深度收集环境信息' : 'Use Java Applet to get information' }}</span>
        </label>
      </div>
    </div>

    <template #footer>
      <div class="cs-dialog-footer-center">
        <button class="cs-btn cs-btn-primary" @click="handleLaunch">{{ isZh ? '启动探针' : 'Launch' }}</button>
        <button class="cs-btn" @click="handleHelp">{{ isZh ? '帮助' : 'Help' }}</button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, reactive } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18nStore } from '@/stores/i18n'

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

const form = reactive({
  uri: '/',
  host: '192.168.10.4',
  port: '80',
  redirectUrl: '',
  ssl: false,
  useApplet: true,
})

function handleLaunch() {
  // 演示功能：后端未实现 Applet/Profiler Web 服务
  ElMessage.warning(isZh.value ? '演示功能：后端未实现该服务（无对应 API），不会实际启动' : 'Demo only: this service is not implemented on the backend (no API), nothing will start')
}

function handleHelp() {
  ElMessage.info('System Profiler Tool Help')
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
  width: 90px;
  font-size: 12px;
  color: var(--cs-text);
  flex-shrink: 0;
}
.cs-form-input {
  flex: 1;
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
