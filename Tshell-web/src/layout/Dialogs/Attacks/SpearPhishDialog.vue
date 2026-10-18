<template>
  <el-dialog
    v-model="visible"
    :title="i18nStore.tl('鱼叉式邮件钓鱼', 'Spear Phish')"
    width="540px"
    append-to-body
    class="cs-dialog"
    :close-on-click-modal="false"
  >
    <div class="cs-dialog-body">
      <!-- Targets Table -->
      <div class="cs-table-wrap">
        <el-table
          :data="targetsTable"
          size="small"
          height="120px"
          class="cs-session-table"
        >
          <el-table-column prop="to" :label="i18nStore.tl('收件人', 'To')" min-width="140" />
          <el-table-column prop="toName" :label="i18nStore.tl('收件人姓名', 'To_Name')" min-width="140" />
        </el-table>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('目标列表:', 'Targets:') }}</label>
        <div class="cs-input-with-btn">
          <el-input v-model="form.targets" size="small" placeholder="targets.csv" class="cs-form-input" />
          <button class="cs-picker-btn" :title="i18nStore.tl('选择目标文件', 'Choose targets file')" @click="pickFile('targets')">📄</button>
        </div>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('邮件模板:', 'Template:') }}</label>
        <div class="cs-input-with-btn">
          <el-input v-model="form.template" size="small" placeholder="template.html" class="cs-form-input" />
          <button class="cs-picker-btn" :title="i18nStore.tl('选择模板文件', 'Choose template file')" @click="pickFile('template')">📄</button>
        </div>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('附件文件:', 'Attachment:') }}</label>
        <div class="cs-input-with-btn">
          <el-input v-model="form.attachment" size="small" placeholder="invoice.docx" class="cs-form-input" />
          <button class="cs-picker-btn" :title="i18nStore.tl('选择附件文件', 'Choose attachment file')" @click="pickFile('attachment')">📄</button>
        </div>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('嵌入 URL:', 'Embed URL:') }}</label>
        <div class="cs-input-with-btn">
          <el-input v-model="form.embedUrl" size="small" placeholder="http://192.168.10.4/login" class="cs-form-input" />
          <button class="cs-picker-btn" @click="pickEmbedUrl">...</button>
        </div>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('邮件服务器:', 'Mail Server:') }}</label>
        <div class="cs-input-with-btn">
          <el-input v-model="form.mailServer" size="small" placeholder="smtp.domain.com:25" class="cs-form-input" />
          <button class="cs-picker-btn" :title="i18nStore.tl('配置邮件服务器', 'Configure Mail Server')" @click="showMailServerDialog = true">...</button>
        </div>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('退信反弹地址:', 'Bounce To:') }}</label>
        <el-input v-model="form.bounceTo" size="small" placeholder="bounce@domain.com" class="cs-form-input" />
      </div>
    </div>

    <template #footer>
      <div class="cs-dialog-footer-center">
        <button class="cs-btn" @click="handlePreview">{{ i18nStore.tl('预览', 'Preview') }}</button>
        <button class="cs-btn cs-btn-primary" @click="handleSend">{{ i18nStore.tl('发送', 'Send') }}</button>
        <button class="cs-btn" @click="handleHelp">{{ i18nStore.tl('帮助', 'Help') }}</button>
      </div>
    </template>

    <MailServerDialog
      v-model="showMailServerDialog"
      @set="onMailServerSet"
    />
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18nStore } from '@/stores/i18n'
import MailServerDialog from './MailServerDialog.vue'

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

const showMailServerDialog = ref(false)

const targetsTable = ref([
  { to: 'victim@targetcorp.com', toName: 'John Doe' },
  { to: 'finance@targetcorp.com', toName: 'Finance Department' },
])

const form = reactive({
  targets: 'targets.csv',
  template: 'security_alert.html',
  attachment: 'report.pdf',
  embedUrl: 'http://192.168.10.4/login',
  mailServer: 'smtp.internal.local:25',
  bounceTo: 'noreply@internal.local',
})

function pickFile(field: string) {
  ElMessage.info(`Select ${field} file`)
}

function pickEmbedUrl() {
  ElMessage.info('Choose Hosted Site URL')
}

function onMailServerSet(config: any) {
  form.mailServer = `${config.host}:${config.port}`
}

function handlePreview() {
  ElMessage.warning(isZh.value ? '演示功能：后端未实现邮件服务，无预览内容' : 'Demo only: no mail service on backend, nothing to preview')
}

function handleSend() {
  ElMessage.warning(isZh.value ? '演示功能：后端未实现邮件服务，邮件不会实际发送' : 'Demo only: no mail service on backend, emails will not be sent')
}

function handleHelp() {
  ElMessage.info('Spear Phishing Attack Manual')
}
</script>

<style scoped>
.cs-table-wrap {
  border-radius: 8px;
  overflow: hidden;
}
.cs-input-with-btn {
  flex: 1;
  display: flex;
  gap: 8px;
}
.cs-picker-btn {
  width: 24px;
  height: 24px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-border-strong);
  border-radius: 6px;
  color: var(--cs-text);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  transition: all 120ms ease;
}
.cs-picker-btn:hover {
  background: var(--cs-hover);
  color: var(--cs-text-bright);
  border-color: var(--cs-active);
}
</style>
