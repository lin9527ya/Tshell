<template>
  <el-dialog
    v-model="visible"
    :title="isZh ? '无阶段载荷生成器' : 'Payload Generator (Stageless)'"
    width="780px"
    append-to-body
    class="cs-dialog"
    :close-on-click-modal="false"
  >
    <div class="cs-dialog-body">
      <div class="cs-dialog-desc">
        {{ isZh ? '导出无阶段载荷（完整 beacon 单文件/无阶段 shellcode，经后端 GenServer 生成），输出格式与分阶段生成器一致。' : 'Exports a stageless payload (full single-file beacon or stageless shellcode via backend GenServer). Same output formats as the staged generator.' }}
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
        <el-select v-model="form.os" size="small" class="cs-form-input" @change="onOsChange">
          <el-option label="Windows" value="windows" />
          <el-option label="Linux" value="linux" />
          <el-option label="macOS (darwin)" value="darwin" />
        </el-select>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '架构:' : 'Arch:' }}</label>
        <el-select v-model="form.arch" size="small" class="cs-form-input">
          <el-option v-for="a in archOptions" :key="a" :label="a" :value="a" />
        </el-select>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '输出格式:' : 'Output:' }}</label>
        <el-select v-model="form.format" size="small" class="cs-form-input">
          <el-option :label="isZh ? 'exe（可执行程序）' : 'exe (executable)'" value="exe" />
          <el-option v-if="form.os === 'windows'" :label="isZh ? 'dll（rundll32/regsvr32 加载）' : 'dll (rundll32/regsvr32)'" value="dll" />
          <el-option label="raw" value="raw" />
          <el-option v-if="form.os === 'windows'" :label="isZh ? 'bin（无阶段 shellcode）' : 'bin (stageless shellcode)'" value="bin" />
          <el-option v-if="form.os === 'windows'" :label="isZh ? 'hex（shellcode 十六进制）' : 'hex (shellcode hex)'" value="hex" />
          <el-option v-if="form.os === 'windows'" :label="isZh ? 'c（shellcode C 数组）' : 'c (shellcode C array)'" value="c" />
        </el-select>
      </div>

      <div class="cs-form-hint">
        {{ isZh ? '格式说明：exe/raw/dll 为完整 beacon 程序；bin/hex/c 为完整 beacon 经 donut 转换的无阶段 shellcode（仅 Windows）。' : 'exe/raw/dll: full beacon program; bin/hex/c: full beacon as stageless shellcode via donut (Windows only).' }}
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? 'C2 画像:' : 'C2 Profile:' }}</label>
        <el-select v-model="form.profileName" size="small" class="cs-form-input" :placeholder="isZh ? '选择画像（默认 default）' : 'Select profile (default)'" @visible-change="onProfileListOpen">
          <el-option v-for="n in profileNames" :key="n" :label="n" :value="n" />
        </el-select>
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

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '上线密码:' : 'Pass:' }}</label>
        <el-input v-model="form.pass" size="small" :placeholder="isZh ? '客户端启动上线密码（可选）' : 'Beacon startup password (optional)'" class="cs-form-input" />
      </div>
    </div>

    <template #footer>
      <div class="cs-dialog-footer-center">
        <button class="cs-btn cs-btn-primary" :disabled="generating || !form.listener" @click="handleGenerate">
          {{ generating ? (isZh ? '生成中...' : 'Generating...') : (isZh ? '生成' : 'Generate') }}
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
import { computed, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18nStore } from '@/stores/i18n'
import { useWorkspaceStore } from '@/stores/workspace'
import ChoosePayloadDialog, { type ListenerOption } from '../ChoosePayloadDialog.vue'
import { generateClientBinary, extractBlobError } from './payloadUtil'

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)
const workspace = useWorkspaceStore()

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

const form = reactive({
  listener: '',
  os: 'windows',
  arch: 'amd64',
  pass: '',
  format: 'exe',
  vkey: '',
  encryptSalt: '',
  // OPSEC 编译期配置
  reconnectBaseDelay: 0,
  reconnectMaxRetries: 0,
  injectTech: '',
  masqueradeName: '',
  quietMode: true,
  profileName: 'default',
})

const advOpen = ref('')
const profileNames = ref<string[]>(['default'])

async function onProfileListOpen(v?: boolean) {
  if (v === false) return
  try {
    const res = await c2profileApi.listProfiles()
    profileNames.value = (res?.data?.data || []).map((p: any) => p.name)
  } catch { /* 默认 */ }
}

// 有效 OS×Arch 组合（对齐 ServerGen clientTypes：windows=amd64/386，linux=+arm64/arm，darwin=amd64/arm64）
const archByOs: Record<string, string[]> = {
  windows: ['amd64', '386'],
  linux: ['amd64', '386', 'arm64', 'arm'],
  darwin: ['amd64', 'arm64'],
}

const archOptions = computed(() => archByOs[form.os] || ['amd64'])

function onOsChange() {
  if (!archOptions.value.includes(form.arch)) {
    form.arch = archOptions.value[0]
  }
  // 非 windows 目标不支持 dll 与 shellcode 格式
  if (form.os !== 'windows' && (form.format === 'dll' || form.format === 'bin' || form.format === 'hex' || form.format === 'c')) {
    form.format = 'exe'
  }
}

// 菜单入口（有效载荷 → Linux/macOS 无阶段载荷）预设 OS
watch(
  () => props.modelValue,
  (val) => {
    if (val) {
      const preset = workspace.dialogs.payloadGenStageless.os
      if (preset && archByOs[preset]) {
        form.os = preset
        form.arch = archByOs[preset][0]
        if (preset !== 'windows') form.format = 'exe'
      }
    }
  },
)

function onChooseListener(l: ListenerOption) {
  form.listener = l.name
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
      osType: form.os,
      archType: form.arch,
      pass: form.pass,
      format: form.format as any,
      vkey: form.vkey || undefined,
      encryptSalt: form.encryptSalt || undefined,
      reconnectBaseDelay: form.reconnectBaseDelay > 0 ? String(form.reconnectBaseDelay) : '',
      reconnectMaxRetries: form.reconnectMaxRetries > 0 ? String(form.reconnectMaxRetries) : '',
      injectTech: form.injectTech || undefined,
      masqueradeName: form.masqueradeName.trim() || undefined,
      quietMode: form.quietMode ? '1' : '0',
      profileName: form.profileName || undefined,
    })
    if (ok) {
      ElMessage.success(isZh.value ? '无阶段载荷生成成功' : 'Stageless payload generated')
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
  ElMessage.info(isZh.value ? '无阶段载荷为完整 beacon 单文件，生成时自动匹配监听器 VKey/Salt' : 'Stageless payload is a full single-file beacon; listener VKey/Salt matched automatically')
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
