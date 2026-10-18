<template>
  <el-dialog
    v-model="visible"
    :title="i18nStore.tl('Windows 可执行程序 (无阶段)', 'Windows Executable (Stageless)')"
    width="780px"
    append-to-body
    class="cs-dialog"
    :close-on-click-modal="false"
  >
    <div class="cs-dialog-body">
      <div class="cs-dialog-desc">
        {{ i18nStore.tl('生成无阶段 Windows 客户端载荷（完整 beacon 单文件，经后端 GenServer 模板 patch），无需分阶段拉取。', 'Generates a stageless Windows beacon payload (single full beacon file, patched by backend GenServer) — no staging required.') }}
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('监听器:', 'Listener:') }}</label>
        <div class="cs-input-with-btn">
          <el-input v-model="form.listener" size="small" placeholder="tcp://1.2.3.4:8080" class="cs-form-input" />
          <button class="cs-picker-btn" :title="i18nStore.tl('选择监听器', 'Choose a listener')" @click="showPicker = true">...</button>
        </div>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('输出格式:', 'Output:') }}</label>
        <el-select v-model="form.output" size="small" class="cs-form-input">
          <el-option :label="i18nStore.tl('Windows EXE（可执行文件）', 'Windows EXE')" value="exe" />
          <el-option :label="i18nStore.tl('Windows DLL（rundll32/regsvr32 加载）', 'Windows DLL')" value="dll" />
        </el-select>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">x64:</label>
        <label class="cs-checkbox-wrap">
          <input type="checkbox" v-model="form.x64" />
          <span>{{ i18nStore.tl('使用 64 位 (amd64) 载荷', 'Use x64 (amd64) payload') }}</span>
        </label>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('上线密码:', 'Pass:') }}</label>
        <el-input v-model="form.pass" size="small" :placeholder="i18nStore.tl('客户端启动上线密码（可选）', 'Beacon startup password (optional)')" class="cs-form-input" />

      <div class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('多传输端点:', 'Multi-transport:') }}</label>
        <el-input
          v-model="form.endpoints"
          size="small"
          class="cs-form-input"
          :placeholder="i18nStore.tl('可选，逗号分隔：tcp://1.2.3.4:443,https://cdn.example.com/api', 'optional, comma-separated: tcp://1.2.3.4:443,https://cdn.example.com/api')"
        />
      </div>
      <div v-if="form.endpoints.trim()" class="cs-form-hint">
        {{ i18nStore.tl('客户端会按顺序尝试这些端点，某个连不上时自动切到下一个（可跨传输类型）。所有端点必须使用与所选监听器相同的 VKey/盐。', 'The beacon tries these endpoints in order and falls back to the next on failure (may switch transport type). All endpoints must share the selected listener\'s VKey/salt.') }}
      </div>
      </div>

      <!-- 高级配置（OPSEC 编译期） -->
      <el-collapse v-model="advOpen" class="cs-adv-collapse">
        <el-collapse-item :title="i18nStore.tl('高级配置（OPSEC）', 'Advanced (OPSEC)')" name="adv">
          <div class="cs-adv-grid">
            <div class="cs-form-row">
              <label class="cs-form-label">{{ i18nStore.tl('重连基础延迟', 'Reconnect delay') }}</label>
              <el-input v-model.number="form.reconnectBaseDelay" size="small" type="number" min="1" max="60" style="width: 90px" />
              <span class="cs-adv-unit">s（1-60）</span>
            </div>
            <div class="cs-form-row">
              <label class="cs-form-label">{{ i18nStore.tl('最大重试', 'Max retries') }}</label>
              <el-input v-model.number="form.reconnectMaxRetries" size="small" type="number" min="5" max="200" style="width: 90px" />
            </div>
            <div class="cs-form-row">
              <label class="cs-form-label">{{ i18nStore.tl('注入默认技术', 'Inject tech') }}</label>
              <el-select v-model="form.injectTech" size="small" style="width: 150px">
                <el-option label="auto（隐蔽链）" value="auto" />
                <el-option label="NtCreateThreadEx" value="ntc" />
                <el-option label="QueueUserAPC" value="apc" />
                <el-option label="CreateRemoteThread" value="crt" />
              </el-select>
            </div>
            <div class="cs-form-row">
              <label class="cs-form-label">{{ i18nStore.tl('ETW 补丁', 'ETW patch') }}</label>
              <el-switch v-model="form.etwPatch" size="small" />
              <span class="cs-adv-unit" style="color: var(--cs-warning)">{{ i18nStore.tl('⚠️ 需虚拟机测试, 关键进程禁用', '⚠️ VM only, critical PIDs blocked') }}</span>
            </div>
            <div class="cs-form-row">
              <label class="cs-form-label">{{ i18nStore.tl('AMSI 绕过', 'AMSI bypass') }}</label>
              <el-switch v-model="form.amsiBypass" size="small" />
              <span class="cs-adv-unit" style="color: var(--cs-warning)">{{ i18nStore.tl('⚠️ 需虚拟机测试', '⚠️ VM only') }}</span>
            </div>
            <div class="cs-form-row">
              <label class="cs-form-label">{{ i18nStore.tl('Syscall 模式', 'Syscall mode') }}</label>
              <el-select v-model="form.syscallMode" size="small" style="width: 150px">
                <el-option :label="i18nStore.tl('常规 ntdll 调用', 'Normal (ntdll)')" value="none" />
                <el-option :label="i18nStore.tl('直接 syscall (仅x64)', 'Direct syscall (x64)')" value="direct" />
                <el-option :label="i18nStore.tl('间接 syscall (仅x64)', 'Indirect syscall (x64)')" value="indirect" />
              </el-select>
            </div>
            <div class="cs-form-row">
              <label class="cs-form-label">{{ i18nStore.tl('上线自动删自身文件', 'Auto-delete on start') }}</label>
              <el-switch v-model="form.autoDelete" size="small" />
              <span class="cs-adv-unit">{{ i18nStore.tl('改名后立即删除自身(进程保持在线)', 'unlink self after rename (stay online)') }}</span>
            </div>
            <div class="cs-form-row">
              <label class="cs-form-label">{{ i18nStore.tl('伪装修饰名', 'Masquerade') }}</label>
              <el-input v-model="form.masqueradeName" size="small" style="width: 130px" :placeholder="i18nStore.tl('留空自动生成', 'auto if empty')" />
              <span class="cs-adv-unit">{{ i18nStore.tl('持久化/管道/文件前缀', 'persist/pipe/file prefix') }}</span>
            </div>
            <div class="cs-form-row">
              <label class="cs-checkbox-wrap" style="width: auto">
                <input type="checkbox" v-model="form.quietMode" />
                <span>{{ i18nStore.tl('静默模式（无调试输出）', 'Quiet mode (no debug output)') }}</span>
              </label>
            </div>
          </div>
          <div class="cs-adv-hint">
            {{ i18nStore.tl('OPSEC 提示：上线密码通过命令行参数传入（进程命令行可见）；不设密码则无参数启动更隐蔽。', 'OPSEC: the beacon password is passed via command-line (visible in process args); no password means parameter-less launch (more stealthy).') }}
          </div>
        </el-collapse-item>
      </el-collapse>
    </div>

    <template #footer>
      <div class="cs-dialog-footer-center">
        <button class="cs-btn cs-btn-primary" :disabled="generating || !form.listener" @click="handleGenerate">
          {{ generating ? (i18nStore.tl('生成中...', 'Generating...')) : (i18nStore.tl('生成', 'Generate')) }}
        </button>
        <button class="cs-btn" @click="handleHelp">{{ i18nStore.tl('帮助', 'Help') }}</button>
      </div>
    </template>

    <ChoosePayloadDialog
      v-model="showPicker"
      :title="i18nStore.tl('选择监听器', 'Choose a listener')"
      @choose="onChooseListener"
    />
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18nStore } from '@/stores/i18n'
import ChoosePayloadDialog, { type ListenerOption } from '../ChoosePayloadDialog.vue'
import { generateClientBinary, extractBlobError } from './payloadUtil'

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
const generating = ref(false)

const advOpen = ref('')

const form = reactive({
  listener: '',
  output: 'exe',
  x64: true,
  pass: '',
  vkey: '',
  encryptSalt: '',
  reconnectBaseDelay: 0,
  reconnectMaxRetries: 0,
  injectTech: '',
  etwPatch: false,
  amsiBypass: false,
  syscallMode: 'none',
  autoDelete: true,
  masqueradeName: '',
  quietMode: true,
  // 多传输端点（可选）：留空=单传输；填写后客户端按顺序在运行时切换传输
  endpoints: '',
})

function onChooseListener(l: ListenerOption) {
  form.listener = l.name
  // 选择器携带监听器完整密钥，直接使用（避免二次匹配失败回退默认密钥）
  form.vkey = l.vkey || ''
  form.encryptSalt = l.encryptSalt || ''
}

async function handleGenerate() {
  if (!form.listener) {
    ElMessage.warning(isZh.value ? '请先选择监听器' : 'Select a listener first')
    return
  }
  generating.value = true
  try {
    const ok = await generateClientBinary({
      listener: form.listener,
      osType: 'windows',
      archType: form.x64 ? 'amd64' : '386',
      pass: form.pass,
      format: form.output as 'exe' | 'dll',
      vkey: form.vkey || undefined,
      encryptSalt: form.encryptSalt || undefined,
      reconnectBaseDelay: form.reconnectBaseDelay > 0 ? String(form.reconnectBaseDelay) : '',
      reconnectMaxRetries: form.reconnectMaxRetries > 0 ? String(form.reconnectMaxRetries) : '',
      injectTech: form.injectTech || undefined,
      masqueradeName: form.masqueradeName.trim() || undefined,
      endpoints: form.endpoints.trim() || undefined,
      quietMode: form.quietMode ? '1' : '0',
      etwPatch: form.etwPatch ? '1' : '0',
      amsiBypass: form.amsiBypass ? '1' : '0',
      syscallMode: form.syscallMode,
      autoDelete: form.autoDelete ? '1' : '0',
    })
    if (ok) {
      ElMessage.success(isZh.value ? `${form.output.toUpperCase()} 无阶段载荷生成成功` : `Stageless ${form.output.toUpperCase()} payload generated`)
      visible.value = false
    } else {
      ElMessage.error(isZh.value ? '生成失败：响应不是可下载文件' : 'Generation failed: response is not a downloadable file')
    }
  } catch (e: any) {
    const msg = await extractBlobError(e)
    ElMessage.error(isZh.value ? `生成失败: ${msg}` : `Generation failed: ${msg}`)
  } finally {
    generating.value = false
  }
}

function handleHelp() {
  ElMessage.info(isZh.value ? '无阶段载荷为完整 beacon 单文件，生成时自动匹配监听器 VKey/Salt' : 'Stageless payload is a full single-file beacon; the listener VKey/Salt is matched automatically')
}
</script>

<style scoped>
/* 多传输端点输入下方的说明（跨传输回退的关键约束） */
.cs-form-hint {
  margin: -4px 0 8px 96px;
  font-size: 12px;
  line-height: 1.6;
  color: var(--cs-text-dim);
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
.cs-checkbox-wrap {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--cs-text);
  cursor: pointer;
}
.cs-adv-collapse {
  border: none;
  border-radius: 8px;
  background: var(--cs-hover);
  overflow: hidden;
}
.cs-adv-collapse :deep(.el-collapse-item__header) {
  height: 40px;
  padding: 0 12px;
  background: transparent;
  border: none;
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text);
}
.cs-adv-collapse :deep(.el-collapse-item__wrap) {
  background: transparent;
  border: none;
}
.cs-adv-collapse :deep(.el-collapse-item__content) {
  padding: 0 12px 12px;
  font-size: 14px;
  color: var(--cs-text);
}
.cs-adv-grid {
  display: flex;
  flex-direction: column;
  gap: 0;
}
.cs-adv-unit {
  font-size: 12px;
  color: var(--cs-text-dim);
  white-space: nowrap;
}
.cs-adv-hint {
  margin-top: 4px;
  font-size: 12px;
  color: var(--cs-text-dim);
  line-height: 1.6;
}
</style>
