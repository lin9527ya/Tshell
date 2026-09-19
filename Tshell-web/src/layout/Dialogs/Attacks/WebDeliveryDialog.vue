<template>
  <el-dialog
    v-model="visible"
    :title="isZh ? '脚本化 Web 投递' : 'Scripted Web Delivery'"
    width="500px"
    append-to-body
    class="cs-dialog"
    :close-on-click-modal="false"
  >
    <div class="cs-dialog-body">
      <div class="cs-dialog-desc">
        {{ isZh ? '在后端 Delivery Hub 创建投递（/webdelivery/start）：托管完整客户端 + Stager + Shellcode，并生成目标机一句话上线命令矩阵。' : 'Creates a delivery on the backend Delivery Hub (/webdelivery/start): hosts full client + stager + shellcode and generates the one-liner command matrix.' }}
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '监听器:' : 'Listener:' }}</label>
        <div class="cs-input-with-btn">
          <el-input v-model="form.listener" size="small" placeholder="tcp://1.2.3.4:8080" class="cs-form-input" />
          <button class="cs-picker-btn" :title="isZh ? '选择监听器' : 'Choose a listener'" @click="showPicker = true">...</button>
        </div>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '目标系统:' : 'Target OS:' }}</label>
        <el-select v-model="form.os" size="small" class="cs-form-input" style="width: 45%">
          <el-option label="windows" value="windows" />
          <el-option label="linux" value="linux" />
        </el-select>
        <label class="cs-checkbox-wrap">
          <input type="checkbox" v-model="form.x64" />
          <span>x64</span>
        </label>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '投递模式:' : 'Bind Mode:' }}</label>
        <el-select v-model="form.bindMode" size="small" class="cs-form-input">
          <el-option :label="isZh ? 'shared（共享主端口，自动分配地址）' : 'shared (main port, auto URL)'" value="shared" />
          <el-option :label="isZh ? 'dedicated（独立端口）' : 'dedicated (own port)'" value="dedicated" />
        </el-select>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '投递端口:' : 'Port:' }}</label>
        <el-input v-model="form.port" size="small" :placeholder="isZh ? 'dedicated 模式必填' : 'Required for dedicated mode'" class="cs-form-input" />
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '上线密码:' : 'Pass:' }}</label>
        <el-input v-model="form.pass" size="small" :placeholder="isZh ? '客户端启动上线密码' : 'Beacon startup password' " class="cs-form-input" />
      </div>
    </div>

    <template #footer>
      <div class="cs-dialog-footer-center">
        <button class="cs-btn cs-btn-primary" :disabled="launching || !form.listener" @click="handleLaunch">
          {{ launching ? (isZh ? '启动中...' : 'Launching...') : (isZh ? '启动投递' : 'Launch') }}
        </button>
        <button class="cs-btn" @click="handleHelp">{{ isZh ? '帮助' : 'Help' }}</button>
      </div>
    </template>

    <ChoosePayloadDialog
      v-model="showPicker"
      :title="isZh ? '选择监听器' : 'Choose a listener'"
      @choose="onChooseListener"
    />
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18nStore } from '@/stores/i18n'
import ChoosePayloadDialog, { type ListenerOption } from '../ChoosePayloadDialog.vue'
import ClientAPI from '@/api/clients'

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
const launching = ref(false)

const form = reactive({
  listener: '',
  os: 'windows',
  x64: true,
  bindMode: 'shared' as 'shared' | 'dedicated',
  port: '',
  pass: '',
})

function onChooseListener(l: ListenerOption) {
  form.listener = l.name
}

async function handleLaunch() {
  if (!form.listener) {
    ElMessage.warning(isZh.value ? '请先选择监听器' : 'Select a listener first')
    return
  }
  if (form.bindMode === 'dedicated' && !form.port) {
    ElMessage.warning(isZh.value ? 'dedicated 模式必须指定端口' : 'Dedicated mode requires a port')
    return
  }
  launching.value = true
  try {
    const res = await ClientAPI.StartWebDelivery({
      listener: form.listener,
      os: form.os,
      arch: form.x64 ? 'amd64' : '386',
      pass: form.pass || 'password',
      port: form.bindMode === 'dedicated' ? form.port : '',
      bindMode: form.bindMode,
      downloadPath: form.os === 'windows' ? 'C:\\Windows\\Temp' : '/tmp',
    })
    if (res.status === 200 && res.data?.status === 200) {
      const payload = res.data.data || {}
      const commands = payload.commands || []
      const urls = payload.urls || {}
      const lines = (Array.isArray(commands) ? commands : [])
        .map((c: any) => `# ${c.name} — ${c.description || ''}\n${c.code}`)
        .join('\n\n')
      const text = `${isZh.value ? '投递已启动，命令矩阵:' : 'Delivery started, command matrix:'}\n${lines}`
      await ElMessageBox.alert(
        `<textarea readonly style="width:100%;height:280px;background:var(--cs-console);color:var(--cs-online);border:1px solid var(--cs-border);padding:8px;font-family:monospace;font-size:11px;line-height:1.6;">${text.replace(/</g, '&lt;').replace(/>/g, '&gt;')}</textarea>`,
        isZh.value ? 'Web Delivery 命令矩阵' : 'Web Delivery Command Matrix',
        {
          dangerouslyUseHTMLString: true,
          confirmButtonText: isZh.value ? '复制并关闭' : 'Copy & Close',
          cancelButtonText: isZh.value ? '取消' : 'Cancel',
          callback: (action: string) => {
            if (action === 'confirm') {
              navigator.clipboard.writeText(text).then(() => {
                ElMessage.success(isZh.value ? '命令矩阵已复制' : 'Command matrix copied')
              })
            }
          },
        },
      )
      visible.value = false
    } else {
      ElMessage.error(res.data?.data || (isZh.value ? '启动投递失败' : 'Failed to start delivery'))
    }
  } catch (e: any) {
    ElMessage.error(isZh.value ? `启动投递失败: ${e?.message || e}` : `Failed to start delivery: ${e?.message || e}`)
  } finally {
    launching.value = false
  }
}

function handleHelp() {
  ElMessage.info(isZh.value ? '投递创建在 Delivery Hub（/webdelivery/start），完整命令矩阵可在「Web Delivery」页再次查看' : 'Delivery is created on the Delivery Hub (/webdelivery/start); the full command matrix is also available in the Web Delivery page')
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
.cs-input-with-btn {
  flex: 1;
  display: flex;
  gap: 6px;
}
.cs-picker-btn {
  width: 28px;
  height: 28px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-border);
  border-radius: 2px;
  color: var(--cs-text);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 14px;
}
.cs-picker-btn:hover {
  background: var(--cs-hover);
  color: var(--cs-text-bright);
}
.cs-checkbox-wrap {
  display: flex;
  align-items: center;
  gap: 8px;
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
