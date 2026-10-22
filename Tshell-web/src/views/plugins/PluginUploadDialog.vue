<template>
  <el-dialog
    :model-value="visible"
    :title="i18nStore.tl('上传插件', 'Upload Plugin')"
    width="700px"
    :close-on-click-modal="false"
    @update:model-value="(v: boolean) => emit('update:visible', v)"
  >
    <div class="upload-dialog-content">
      <div class="upload-info-banner">
        <el-icon class="info-icon"><InfoFilled /></el-icon>
        {{ i18nStore.tl('支持 exe/net/elf/dll/so/dylib/bin/sh/py/ps1/o/obj/coff 格式，单个文件不超过50MB，最多只能同时上传6个文件', 'Supports exe/net/elf/dll/so/dylib/bin/sh/py/ps1/o/obj/coff. Max 50MB per file, up to 6 files at once.') }}
        <el-button type="primary" class="select-file-btn" @click="selectFiles">{{ i18nStore.tl('选择文件', 'Select Files') }}</el-button>
      </div>

      <!-- 文件列表区域 -->
      <div class="file-list-area">
        <div class="file-list-header">
          <div class="header-col thumbnail">{{ i18nStore.tl('缩略图', 'Preview') }}</div>
          <div class="header-col filename">{{ i18nStore.tl('文件名', 'File Name') }}</div>
          <div class="header-col filesize">{{ i18nStore.tl('文件大小', 'Size') }}</div>
          <div class="header-col status">{{ i18nStore.tl('状态', 'Status') }}</div>
          <div class="header-col operation">{{ i18nStore.tl('操作', 'Actions') }}</div>
        </div>

        <div class="file-list-body">
          <div v-if="fileList.length === 0" class="empty-file-list">
            {{ i18nStore.tl('暂无文件', 'No files') }}
          </div>
          <div v-for="(file, index) in fileList" :key="index" class="file-item">
            <div class="file-col thumbnail">
              <div class="file-icon">📄</div>
            </div>
            <div class="file-col filename">{{ file.name }}</div>
            <div class="file-col filesize">{{ formatFileSize(file.size) }}</div>
            <div class="file-col status">{{ i18nStore.tl('待上传', 'Pending') }}</div>
            <div class="file-col operation">
              <el-button type="text" @click="removeFile(index)">{{ i18nStore.tl('删除', 'Remove') }}</el-button>
            </div>
          </div>
        </div>
      </div>

      <!-- 隐藏的文件输入 -->
      <input
        ref="fileInput"
        type="file"
        multiple
        accept=".exe,.net,.elf,.dll,.so,.dylib,.bin,.sh,.py,.ps1,.o,.obj,.coff"
        style="display: none"
        @change="handleFileSelect"
      />
    </div>

    <template #footer>
      <div class="upload-dialog-footer">
        <el-button @click="closeUploadDialog">{{ i18nStore.tl('关闭', 'Close') }}</el-button>
        <el-button type="primary" @click="submitUpload">{{ i18nStore.tl('开始上传', 'Start Upload') }}</el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
// 插件上传对话框（阶段三 §3.11：从 1769 行的 PluginsView.vue 拆出）。
//
// 拆它的理由：这是一块**自带完整状态**的功能单元 —— 文件列表、隐藏 input、
// 选择/去重/数量上限/校验/上传全部只服务于这个对话框，与父组件其余部分无关。
// 拆出去之后它可以单独挂载测试，父组件也少掉约 100 行。
//
// 样式一并搬过来了（原先是父组件的 scoped 样式）。这点很关键：
// Vue 的 scoped 样式只作用于**本组件模板**里的元素，子组件的后代元素不会带上
// 父组件的 scope 属性 —— 只搬模板不搬样式，对话框会静默失去全部样式，
// 而单测与类型检查都发现不了。
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { InfoFilled } from '@element-plus/icons-vue'
import pluginApi from '@/api/plugin'
import { useI18nStore } from '@/stores/i18n'
import { formatPluginFileSize as formatFileSize } from '@/views/plugins/formatSize'

const props = defineProps<{ visible: boolean }>()
const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  /** 至少有一个文件上传成功 —— 父组件据此刷新插件列表 */
  (e: 'uploaded'): void
}>()

const i18nStore = useI18nStore()
const isZh = { get value() { return i18nStore.isZh } }

const fileList = ref<File[]>([])
const fileInput = ref<HTMLInputElement>()

/** 上传对话框 banner 承诺"最多只能同时上传 6 个文件"，这里把它兑现 */
const MAX_UPLOAD_FILES = 6

const beforeUpload = (file: File) => {
  const validTypes = ['application/x-msdownload', 'application/x-executable', 'application/x-sharedlib', 'application/x-so', 'application/x-python', 'text/x-python', 'application/octet-stream']
  // 与服务端 errors.ValidatePluginExtension 的白名单对齐（.bin 是插件目录里
  // fscan.bin/god.bin 这类载荷的实际扩展名，.o/.obj/.coff 用于 BOF）
  const validExtensions = ['.exe', '.net', '.elf', '.dll', '.so', '.dylib', '.bin', '.sh', '.py', '.ps1', '.o', '.obj', '.coff']
  const fileName = file.name.toLowerCase()
  const hasValidExtension = validExtensions.some((ext) => fileName.endsWith(ext))

  if (!validTypes.includes(file.type) && !hasValidExtension) {
    ElMessage.warning(isZh.value ? '只能上传插件文件(可执行载荷或脚本，如 exe/dll/so/bin/sh)' : 'Only plugin files allowed (executable payloads or scripts, e.g. exe/dll/so/bin/sh)')
    return false
  }

  if (file.size > 50 * 1024 * 1024) {
    ElMessage.warning(isZh.value ? '文件大小不能超过50MB' : 'File size cannot exceed 50MB')
    return false
  }

  return true
}

const selectFiles = () => {
  fileInput.value?.click()
}

const handleFileSelect = (event: Event) => {
  const target = event.target as HTMLInputElement
  if (target.files) {
    const newFiles = Array.from(target.files)
    const accepted: File[] = []
    for (const file of newFiles) {
      if (beforeUpload(file)) {
        accepted.push(file)
      }
    }
    const room = Math.max(MAX_UPLOAD_FILES - fileList.value.length, 0)
    if (accepted.length > room) {
      ElMessage.warning(isZh.value
        ? `最多只能同时上传 ${MAX_UPLOAD_FILES} 个文件`
        : `Up to ${MAX_UPLOAD_FILES} files at a time`)
    }
    fileList.value.push(...accepted.slice(0, room))
  }
  target.value = '' // 清空输入
}

const removeFile = (index: number) => {
  fileList.value.splice(index, 1)
}

const closeUploadDialog = () => {
  emit('update:visible', false)
  fileList.value = [] // 关闭时清空文件列表
}

const submitUpload = async () => {
  if (fileList.value.length === 0) {
    ElMessage.warning(isZh.value ? '请选择要上传的文件' : 'Select files to upload')
    return
  }

  let okCount = 0
  for (const file of fileList.value) {
    // 单个文件失败不应中断其余文件的上传（原实现一个 4xx 就跳出整个循环）
    try {
      const response = await pluginApi.upload_plugin(file)
      if (response.data?.status === 200) {
        okCount++
        ElMessage.success(isZh.value ? `${file.name} 上传成功` : `${file.name} uploaded`)
      } else {
        ElMessage.error(response.data?.message || (isZh.value ? `${file.name} 上传失败` : `${file.name} upload failed`))
      }
    } catch (error: any) {
      // 请求层已按后端 message 弹过提示，这里只补上文件名
      console.error(`[PLUGIN] 上传失败: ${file.name}`, error)
      if (!error?.response) {
        ElMessage.error(isZh.value ? `${file.name} 上传失败: ${error?.message || '未知错误'}` : `${file.name} upload failed: ${error?.message || 'unknown error'}`)
      }
    }
  }
  if (okCount > 0) {
    emit('uploaded')
  }
  // 移除自动清空文件列表和关闭对话框，由用户手动关闭
}

// 供测试与父组件使用
defineExpose({ fileList, MAX_UPLOAD_FILES, beforeUpload })
// props 目前只用于 visible，显式引用一次避免 lint 误报
void props
</script>

<style scoped>
/* 上传对话框样式（原先在父组件的 scoped 块里，随模板一并搬来） */
.upload-dialog-content {
  padding: 0;
}

.upload-info-banner {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  background: var(--cs-bg);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  margin-bottom: 16px;
  color: var(--cs-text);
  font-size: 14px;
  line-height: 1.6;
}

.upload-info-banner .info-icon {
  font-size: 16px;
  color: var(--cs-active);
}

.select-file-btn {
  margin-left: auto;
  font-size: 14px;
  flex-shrink: 0;
}

/* 文件列表区域 */
.file-list-area {
  background: var(--cs-panel);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  overflow: hidden;
}

.file-list-header {
  display: flex;
  background: var(--cs-table-head);
  border-bottom: 1px solid var(--cs-card-border);
  font-weight: 600;
  color: var(--cs-text);
  font-size: 14px;
}

.file-list-body {
  min-height: 200px;
  max-height: 300px;
  overflow-y: auto;
}

.header-col,
.file-col {
  height: 48px;
  padding: 0 16px;
  display: flex;
  align-items: center;
}

.header-col.thumbnail,
.file-col.thumbnail {
  width: 80px;
  justify-content: center;
}

.header-col.filename,
.file-col.filename {
  flex: 1;
  min-width: 200px;
}

.header-col.filesize,
.file-col.filesize {
  width: 120px;
}

.header-col.status,
.file-col.status {
  width: 100px;
}

.header-col.operation,
.file-col.operation {
  width: 80px;
  justify-content: center;
}

.file-item {
  display: flex;
  border-bottom: 1px solid var(--cs-card-border);
  transition: background-color 0.3s ease;
}

.file-item:hover {
  background-color: var(--cs-hover);
}

.file-item:last-child {
  border-bottom: none;
}

.file-icon {
  font-size: 20px;
  color: var(--cs-active);
}

.empty-file-list {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 200px;
  color: var(--cs-text-dim);
  font-size: 14px;
}

.upload-dialog-footer {
  text-align: right;
  padding-top: 16px;
  border-top: 1px solid var(--cs-card-border);
  margin-top: 20px;
}

.upload-dialog-footer .el-button {
  margin-left: 12px;
}
</style>
