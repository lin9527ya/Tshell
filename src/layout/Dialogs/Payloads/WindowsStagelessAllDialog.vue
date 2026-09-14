<template>
  <el-dialog
    v-model="visible"
    :title="isZh ? '为全部监听器生成无阶段载荷' : 'Windows Stageless (All Listeners)'"
    width="780px"
    append-to-body
    class="cs-dialog"
    :close-on-click-modal="false"
  >
    <div class="cs-dialog-body">
      <div class="cs-dialog-desc">
        {{ isZh ? '为所选监听器逐个生成无阶段 Windows 载荷（后端 GenServer），每个监听器按所选格式下载一份。' : 'Generates a stageless Windows payload for each selected listener (backend GenServer), one file per listener in the chosen format.' }}
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '监听器:' : 'Listeners:' }}</label>
        <el-select
          v-model="form.listeners"
          multiple
          filterable
          size="small"
          class="cs-form-input"
          :placeholder="isZh ? '选择监听器（可多选）' : 'Select listeners (multi)'"
          @visible-change="onDropdown"
        >
          <el-option
            v-for="l in listenerOptions"
            :key="l.name"
            :label="l.name"
            :value="l.name"
          />
        </el-select>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '输出格式:' : 'Output:' }}</label>
        <el-select v-model="form.format" size="small" class="cs-form-input">
          <el-option :label="isZh ? 'exe（可执行程序）' : 'exe (executable)'" value="exe" />
          <el-option :label="isZh ? 'dll（rundll32/regsvr32 加载）' : 'dll (rundll32/regsvr32)'" value="dll" />
          <el-option label="raw" value="raw" />
          <el-option :label="isZh ? 'bin（无阶段 shellcode）' : 'bin (stageless shellcode)'" value="bin" />
          <el-option :label="isZh ? 'hex（shellcode 十六进制）' : 'hex (shellcode hex)'" value="hex" />
          <el-option :label="isZh ? 'c（shellcode C 数组）' : 'c (shellcode C array)'" value="c" />
        </el-select>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">x64:</label>
        <label class="cs-checkbox-wrap">
          <input type="checkbox" v-model="form.x64" />
          <span>{{ isZh ? '使用 64 位 (amd64) 载荷' : 'Use x64 (amd64) payload' }}</span>
        </label>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '上线密码:' : 'Pass:' }}</label>
        <el-input v-model="form.pass" size="small" :placeholder="isZh ? '客户端启动上线密码（可选）' : 'Beacon startup password (optional)'" class="cs-form-input" />
      </div>

      <!-- 高级配置（OPSEC 编译期） -->
      <el-collapse v-model="advOpen" class="cs-adv-collapse">
        <el-collapse-item :title="isZh ? '高级配置（OPSEC）' : 'Advanced (OPSEC)'" name="adv">
          <div class="cs-adv-grid">
            <div class="cs-form-row">
              <label class="cs-form-label">{{ isZh ? '重连基础延迟' : 'Reconnect delay' }}</label>
              <el-input v-model.number="form.reconnectBaseDelay" size="small" type="number" min="1" max="60" style="width: 90px" />
              <span class="cs-adv-unit">s（1-60）</span>
            </div>
            <div class="cs-form-row">
              <label class="cs-form-label">{{ isZh ? '最大重试' : 'Max retries' }}</label>
              <el-input v-model.number="form.reconnectMaxRetries" size="small" type="number" min="5" max="200" style="width: 90px" />
            </div>
            <div class="cs-form-row">
              <label class="cs-form-label">{{ isZh ? '注入默认技术' : 'Inject tech' }}</label>
              <el-select v-model="form.injectTech" size="small" style="width: 150px">
                <el-option label="auto（隐蔽链）" value="auto" />
                <el-option label="NtCreateThreadEx" value="ntc" />
                <el-option label="QueueUserAPC" value="apc" />
                <el-option label="CreateRemoteThread" value="crt" />
              </el-select>
            </div>
            <div class="cs-form-row">
              <label class="cs-form-label">{{ isZh ? 'ETW 补丁' : 'ETW patch' }}</label>
              <el-switch v-model="form.etwPatch" size="small" />
              <span class="cs-adv-unit" style="color: var(--cs-warning)">{{ isZh ? '⚠️ 需虚拟机测试, 关键进程禁用' : '⚠️ VM only, critical PIDs blocked' }}</span>
            </div>
            <div class="cs-form-row">
              <label class="cs-form-label">{{ isZh ? 'AMSI 绕过' : 'AMSI bypass' }}</label>
              <el-switch v-model="form.amsiBypass" size="small" />
              <span class="cs-adv-unit" style="color: var(--cs-warning)">{{ isZh ? '⚠️ 需虚拟机测试' : '⚠️ VM only' }}</span>
            </div>
            <div class="cs-form-row">
              <label class="cs-form-label">{{ isZh ? 'Syscall 模式' : 'Syscall mode' }}</label>
              <el-select v-model="form.syscallMode" size="small" style="width: 150px">
                <el-option :label="isZh ? '常规 ntdll 调用' : 'Normal (ntdll)'" value="none" />
                <el-option :label="isZh ? '直接 syscall (仅x64)' : 'Direct syscall (x64)'" value="direct" />
                <el-option :label="isZh ? '间接 syscall (仅x64)' : 'Indirect syscall (x64)'" value="indirect" />
              </el-select>
            </div>
            <div class="cs-form-row">
              <label class="cs-form-label">{{ isZh ? '上线自动删自身文件' : 'Auto-delete on start' }}</label>
              <el-switch v-model="form.autoDelete" size="small" />
              <span class="cs-adv-unit">{{ isZh ? '改名后立即删除自身(进程保持在线)' : 'unlink self after rename (stay online)' }}</span>
            </div>
            <div class="cs-form-row">
              <label class="cs-form-label">{{ isZh ? '伪装修饰名' : 'Masquerade' }}</label>
              <el-input v-model="form.masqueradeName" size="small" style="width: 130px" :placeholder="isZh ? '留空自动生成' : 'auto if empty'" />
              <span class="cs-adv-unit">{{ isZh ? '持久化/管道/文件前缀' : 'persist/pipe/file prefix' }}</span>
            </div>
            <div class="cs-form-row">
              <label class="cs-checkbox-wrap" style="width: auto">
                <input type="checkbox" v-model="form.quietMode" />
                <span>{{ isZh ? '静默模式（无调试输出）' : 'Quiet mode (no debug output)' }}</span>
              </label>
            </div>
          </div>
          <div class="cs-adv-hint">
            {{ isZh ? 'OPSEC 提示：上线密码通过命令行参数传入（进程命令行可见）；不设密码则无参数启动更隐蔽。' : 'OPSEC: the beacon password is passed via command-line (visible in process args); no password means parameter-less launch (more stealthy).' }}
          </div>
        </el-collapse-item>
      </el-collapse>
    </div>

    <template #footer>
      <div class="cs-dialog-footer-center">
        <button class="cs-btn cs-btn-primary" :disabled="generating || !form.listeners.length" @click="handleGenerate">
          {{ generating ? `${isZh ? '生成中' : 'Generating'} ${generated}/${form.listeners.length}...` : (isZh ? '批量生成' : 'Generate All') }}
        </button>
        <button class="cs-btn" @click="handleHelp">{{ isZh ? '帮助' : 'Help' }}</button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18nStore } from '@/stores/i18n'
import { fetchListenerOptions, generateClientBinary, extractBlobError, type ListenerOption } from './payloadUtil'

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

const listenerOptions = ref<ListenerOption[]>([])
const generating = ref(false)
const generated = ref(0)

const advOpen = ref('')

const form = reactive({
  listeners: [] as string[],
  x64: true,
  pass: '',
  format: 'exe',
  reconnectBaseDelay: 0,
  reconnectMaxRetries: 0,
  injectTech: '',
  masqueradeName: '',
  quietMode: true,
  etwPatch: false,
  amsiBypass: false,
  syscallMode: 'none',
  autoDelete: true,
})

async function onDropdown(visibleState?: boolean) {
  if (visibleState === false) return
  if (!listenerOptions.value.length) {
    listenerOptions.value = await fetchListenerOptions()
  }
}

async function handleGenerate() {
  if (!form.listeners.length) {
    ElMessage.warning(isZh.value ? '请先选择监听器' : 'Select listeners first')
    return
  }
  generating.value = true
  generated.value = 0
  let failed = 0
  for (const listener of form.listeners) {
    try {
      // 从选项列表带出该监听器的完整密钥
      const opt = listenerOptions.value.find((o) => o.name === listener)
      const ok = await generateClientBinary({
        listener,
        osType: 'windows',
        archType: form.x64 ? 'amd64' : '386',
        pass: form.pass,
        format: form.format as any,
        vkey: opt?.vkey || undefined,
        encryptSalt: opt?.encryptSalt || undefined,
        reconnectBaseDelay: form.reconnectBaseDelay > 0 ? String(form.reconnectBaseDelay) : '',
        reconnectMaxRetries: form.reconnectMaxRetries > 0 ? String(form.reconnectMaxRetries) : '',
        injectTech: form.injectTech || undefined,
        masqueradeName: form.masqueradeName.trim() || undefined,
        quietMode: form.quietMode ? '1' : '0',
        etwPatch: form.etwPatch ? '1' : '0',
        amsiBypass: form.amsiBypass ? '1' : '0',
        syscallMode: form.syscallMode,
        autoDelete: form.autoDelete ? '1' : '0',
      })
      if (ok) generated.value++
      else failed++
    } catch (e: any) {
      failed++
      const msg = await extractBlobError(e)
      ElMessage.error(`${listener}: ${msg}`)
    }
  }
  generating.value = false
  if (failed === 0) {
    ElMessage.success(isZh.value ? `已为 ${generated.value} 个监听器生成载荷` : `Generated payloads for ${generated.value} listener(s)`)
    visible.value = false
  } else {
    ElMessage.warning(isZh.value ? `完成：成功 ${generated.value}，失败 ${failed}` : `Done: ${generated.value} ok, ${failed} failed`)
  }
}

function handleHelp() {
  ElMessage.info(isZh.value ? '逐监听器调用后端 GenServer 生成无阶段载荷；失败项会单独提示' : 'Calls backend GenServer per listener; failures are reported individually')
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
  gap: 6px;
  font-size: 12px;
  color: var(--cs-text);
  cursor: pointer;
}
.cs-adv-collapse {
  border: 1px solid var(--cs-border);
  border-radius: 2px;
}
.cs-adv-grid {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.cs-adv-unit {
  font-size: 11px;
  color: var(--cs-text-dim);
  white-space: nowrap;
}
.cs-adv-hint {
  margin-top: 6px;
  font-size: 11px;
  color: var(--cs-text-dim);
  line-height: 1.5;
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
