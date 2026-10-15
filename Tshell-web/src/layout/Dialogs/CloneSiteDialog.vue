<template>
  <el-dialog
    :model-value="modelValue"
    :title="isZh ? '网站克隆' : 'Clone Site'"
    width="540px"
    class="cs-dialog"
    :close-on-click-modal="false"
    @update:model-value="$emit('update:modelValue', $event)"
  >
    <div class="cs-dialog-body cs-font-ui">
      <div class="cs-dialog-desc">
        {{ isZh ? '网站克隆工具用于复制指定目标网站的页面与静态资源，并注入表单键盘记录脚本以捕获凭据。' : 'The site cloner copies a website and fixes the code so images and links work. It also injects a keystroke logger.' }}
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label"><span class="req">*</span> {{ isZh ? '克隆目标 URL:' : 'Clone URL:' }}</label>
        <div class="cs-form-input-wrap">
          <el-input v-model="form.cloneUrl" size="small" placeholder="https://login.microsoftonline.com/" />
        </div>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label"><span class="req">*</span> {{ isZh ? '本地 URI:' : 'Local URI:' }}</label>
        <div class="cs-form-input-wrap">
          <el-input v-model="form.localUri" size="small" placeholder="/" />
        </div>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label"><span class="req">*</span> {{ isZh ? '本地主机:' : 'Local Host:' }}</label>
        <div class="cs-form-input-wrap">
          <el-input v-model="form.localHost" size="small" placeholder="192.168.10.4" />
        </div>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label"><span class="req">*</span> {{ isZh ? '本地端口:' : 'Local Port:' }}</label>
        <div class="cs-form-input-wrap">
          <el-input-number v-model="form.localPort" size="small" :min="1" :max="65535" style="width: 100%" />
        </div>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '攻击载荷 URL:' : 'Attack:' }}</label>
        <div class="cs-form-input-wrap has-append">
          <el-input v-model="form.attack" size="small" placeholder="http://192.168.10.4:80/payload.hta" />
          <button class="cs-btn cs-btn-xs" @click="chooseAttack">...</button>
        </div>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">SSL:</label>
        <div class="cs-form-input-wrap">
          <el-checkbox v-model="form.ssl">{{ isZh ? '启用 SSL (HTTPS)' : 'Enable SSL' }}</el-checkbox>
        </div>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label" />
        <div class="cs-form-input-wrap">
          <el-checkbox v-model="form.logKeystrokes">{{ isZh ? '记录克隆站点的键盘输入 (Log keystrokes)' : 'Log keystrokes on cloned site' }}</el-checkbox>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="cs-dialog-footer-center">
        <button class="cs-btn cs-btn-primary" :disabled="loading" @click="handleClone">
          {{ isZh ? '克隆' : 'Clone' }}
        </button>
        <button class="cs-btn" @click="handleHelp">
          {{ isZh ? '帮助' : 'Help' }}
        </button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18nStore } from '@/stores/i18n'

const props = defineProps<{
  modelValue: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', val: boolean): void
}>()

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const loading = ref(false)
const form = ref({
  cloneUrl: '',
  localUri: '/',
  localHost: '192.168.10.4',
  localPort: 80,
  attack: '',
  ssl: false,
  logKeystrokes: true,
})

function chooseAttack() {
  ElMessage.info(isZh.value ? '选择关联的 WebDelivery 或 HTA 投递地址' : 'Select associated payload URL')
  form.value.attack = `http://${form.value.localHost}:${form.value.localPort}/download/payload.hta`
}

function handleClone() {
  if (!form.value.cloneUrl) {
    ElMessage.warning(isZh.value ? '请输入克隆目标 URL' : 'Please input clone URL')
    return
  }
  // 演示功能：后端未实现站点克隆 Web 服务
  ElMessage.warning(isZh.value ? '演示功能：后端未实现网站克隆服务（Web 托管未接入），不会实际启动任何服务' : 'Demo only: site cloning is not implemented on the backend (no web hosting), nothing will actually start')
}

function handleHelp() {
  ElMessage.info(isZh.value ? '网站克隆工具用于快速克隆认证入口或门户网站进行钓鱼评估。' : 'Clone Site Manual')
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
