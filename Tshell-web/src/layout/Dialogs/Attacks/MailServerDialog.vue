<template>
  <el-dialog
    v-model="visible"
    :title="i18nStore.tl('邮件服务器配置', 'Mail Server')"
    width="380px"
    append-to-body
    class="cs-dialog"
    :close-on-click-modal="false"
  >
    <div class="cs-dialog-body">
      <div class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('SMTP 主机:', 'SMTP Host:') }}</label>
        <el-input v-model="form.host" size="small" placeholder="smtp.domain.com" class="cs-form-input" />
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('SMTP 端口:', 'SMTP Port:') }}</label>
        <el-input v-model="form.port" size="small" placeholder="25" class="cs-form-input" />
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('用户名:', 'Username:') }}</label>
        <el-input v-model="form.username" size="small" class="cs-form-input" />
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('密码:', 'Password:') }}</label>
        <el-input v-model="form.password" type="password" size="small" class="cs-form-input" />
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('随机延时:', 'Random Delay:') }}</label>
        <el-input v-model="form.delay" size="small" placeholder="0" class="cs-form-input" />
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('连接加密:', 'Connection:') }}</label>
        <el-select v-model="form.connection" size="small" class="cs-form-input">
          <el-option label="Plain" value="Plain" />
          <el-option label="SSL" value="SSL" />
          <el-option label="STARTTLS" value="STARTTLS" />
        </el-select>
      </div>
    </div>

    <template #footer>
      <div class="cs-dialog-footer-center">
        <button class="cs-btn cs-btn-primary" @click="handleSet">{{ i18nStore.tl('保存', 'Set') }}</button>
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
  (e: 'set', config: any): void
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v),
})

const form = reactive({
  host: '',
  port: '25',
  username: '',
  password: '',
  delay: '0',
  connection: 'Plain',
})

function handleSet() {
  emit('set', { ...form })
  visible.value = false
  ElMessage.warning(isZh.value ? '演示功能：配置仅保存在当前表单，后端未实现邮件服务' : 'Demo only: config kept in this form only; no mail service on backend')
}
</script>

