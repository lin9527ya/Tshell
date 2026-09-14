<template>
  <div class="shell-wrapper">
    <div class="terminal">
      <!-- 工具栏 -->
      <div class="toolbar">
        <div class="toolbar-group">
          <el-button-group size="small">
            <el-button @click="clearOutput" icon="Delete">清空输出</el-button>
            <el-button @click="GetShellContent" icon="Refresh">刷新</el-button>
            <el-button @click="copyOutput" icon="CopyDocument">复制全部</el-button>
            <el-button @click="exportOutput" icon="Download">导出日志</el-button>
          </el-button-group>
        </div>
        <span class="status-text">
          <span class="status-dot" /> 连接状态: 已连接
        </span>
      </div>

      <!-- 显示历史命令和响应 -->
      <div id="output" class="output" ref="outputElement">
        <pre class="shell-content">{{ formattedShellStr }}</pre>
      </div>

      <!-- 输入命令 -->
      <div class="input-container">
        <el-input
            v-model="command"
            placeholder="输入命令..."
            @keyup.enter="sendCommand"
            @keydown.up.prevent="prevCommand"
            @keydown.down.prevent="nextCommand"
            ref="input"
            class="command-input"
        >
          <template #append>
            <el-button 
              @click="sendCommand" 
              type="primary" 
              :disabled="!command.trim()"
              icon="Right"
            >
              执行
            </el-button>
          </template>
        </el-input>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, computed, nextTick, inject } from 'vue'
import { ElMessage } from 'element-plus'
import { Right, Delete, CopyDocument, Download, Refresh } from '@element-plus/icons-vue'
import ClientAPI from '@/api/clients'
import { useRoute } from "vue-router"
import { useCommandHistoryStore } from '@/stores/shellcommand'
import { useClientUid } from '@/utils/clientUtils'
import { useClientsStore } from '@/stores/clients'
import { processTerminalOutput } from '@/utils/encoding'
import { useVisibilityInterval } from '@/utils/useVisibilityInterval'

const commandHistory = useCommandHistoryStore()
const clientsStore = useClientsStore()

const route = useRoute()

const command = ref('')
const ShellStr = ref('')
const ShellStrTmp = ref('')
const outputElement = ref(null)
// 输出上限（字符）：防止超大输出（如 dir /s）导致 DOM 无限膨胀、整串正则反复处理
const MAX_SHELL_LENGTH = 300 * 1024

// 超限时保留尾部内容（丢弃头部旧输出）
const capOutput = (s) => (s && s.length > MAX_SHELL_LENGTH ? s.slice(-MAX_SHELL_LENGTH) : s)
// 与其他客户端视图一致：优先使用父组件提供的真实 UID
const injectedUid = inject('realUid', null)
const uid = computed(() => {
  const v = injectedUid && injectedUid.value ? String(injectedUid.value) : ''
  if (v && v.length > 10) return v
  // 回落到路由参数
  const fromRoute = useClientUid(route)
  return typeof fromRoute === 'string' && fromRoute.length > 10 ? fromRoute : ''
})

// 计算属性：ShellStr 已在写入时完成编码处理，直接返回——
// 原实现对已处理内容二次 processTerminalOutput：O(n) 正则每轮询重跑且破坏 HTML 转义
const formattedShellStr = computed(() => ShellStr.value || '')

// 发送命令到后端
const sendCommand = async () => {
  if (command.value.trim() === '') return

  const currentCommand = command.value.trim()
  // 清空当前命令
  command.value = ''
  commandHistory.addCommand(uid.value, currentCommand)
  commandHistory.clearCurrentIndex(uid.value)
  
  try {
    const res = await ClientAPI.send_commands({uid: uid.value, command: currentCommand})
    // 对命令响应进行编码处理
    const rawResponse = res.data.data || ''
    const processedResponse = processTerminalOutput(rawResponse)
    
    ShellStrTmp.value = processedResponse
    if (ShellStr.value !== ShellStrTmp.value) {
      ShellStr.value = capOutput(ShellStrTmp.value)
      await nextTick() // 等待DOM更新
      scrollToBottom()
    }
    // 命令发出后立即拉取一次最新 shell 内容，减少等待
    GetShellContent()
    // 【优化】服务端 shell 内容为异步批量落库（500ms flush）+ 客户端执行耗时，
    // 立即拉取通常拿不到结果；改为短轮询（每 500ms 拉一次，最多 3s），
    // 内容有变化即停止——无需手动点击"刷新"
    const prevLen = (ShellStr.value || '').length
    let pollCount = 0
    const pollTimer = setInterval(async () => {
      pollCount++
      if (pollCount >= 6) {
        clearInterval(pollTimer)
        return
      }
      await GetShellContent()
      if ((ShellStr.value || '').length > prevLen) {
        clearInterval(pollTimer)
      }
    }, 500)
  } catch (error) {
    ElMessage.error('命令执行失败')
    console.error('命令执行错误:', error)
  }
}

// 滚动到底部
const scrollToBottom = () => {
  if (outputElement.value) {
    outputElement.value.scrollTop = outputElement.value.scrollHeight
  }
}

// 清空输出
const clearOutput = () => {
  ShellStr.value = ''
  ElMessage.success('输出已清空')
}

// 复制输出内容
const copyOutput = async () => {
  if (!ShellStr.value) {
    ElMessage.warning('没有内容可复制')
    return
  }
  
  try {
    await navigator.clipboard.writeText(ShellStr.value)
    ElMessage.success('已复制全部输出内容')
  } catch (error) {
    // 退回方案：传统复制方法
    const textArea = document.createElement('textarea')
    textArea.value = ShellStr.value
    document.body.appendChild(textArea)
    textArea.select()
    document.execCommand('copy')
    document.body.removeChild(textArea)
    ElMessage.success('已复制全部输出内容')
  }
}

// 导出日志
const exportOutput = () => {
  if (!ShellStr.value) {
    ElMessage.warning('没有内容可导出')
    return
  }
  
  const timestamp = new Date().toISOString().replace(/[:.]/g, '-')
  const filename = `shell-output-${uid.value}-${timestamp}.txt`
  
  const blob = new Blob([ShellStr.value], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
  
  ElMessage.success('日志已导出')
}

const prevCommand = ()=>{
  const prevCmd = commandHistory.getPrevCommand(uid.value);
  if (prevCmd !== undefined) {
    command.value = prevCmd;
  }
}

const nextCommand = () => {
  const nextCmd = commandHistory.getNextCommand(uid.value);
  if (nextCmd !== undefined) {
    command.value = nextCmd;
  }
};

function GetShellContent() {
  ClientAPI.get_shellcontent({uid: uid.value}).then((res) => {
    // 对从服务端获取的数据进行编码处理
    const rawData = res.data.data || ''
    const processedData = processTerminalOutput(rawData)
    
    ShellStrTmp.value = processedData
    if (ShellStr.value !== ShellStrTmp.value) {
      ShellStr.value = capOutput(ShellStrTmp.value)
      nextTick(() => {
        scrollToBottom()
      })
    }
  }).catch((error) => {
    console.error('获取Shell内容失败:', error)
  })
}

onMounted(() => {
  // 打开界面时立即执行命令
  GetShellContent();
  // 【优化】每5秒轮询一次（原2秒），降低 API 频率与限流压力，命令输出仍可感知
  // P1-6: 页面隐藏时暂停轮询（keep-alive 缓存下原轮询在后台持续空转打 API）
  const { stop } = useVisibilityInterval(GetShellContent, 5000);
  onUnmounted(stop);
});
</script>

<style scoped>
.shell-wrapper {
  height: 100%;
  display: flex;
  padding: 14px 16px;
  box-sizing: border-box;
  background: var(--bg-secondary, #f5f5f5);
}

.terminal {
  flex: 1;
  width: 100%;
  max-width: 1100px;
  margin: 0 auto;
  padding: 12px 14px;
  background-color: #1e1e1e;
  color: #ffffff;
  border-radius: 10px;
  border: 1px solid var(--border-color, #30363d);
  display: flex;
  flex-direction: column;
  gap: 10px;
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
  min-height: 0;
}

.output {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 12px;
  background-color: #0d1117;
  border-radius: 8px;
  border: 1px solid #30363d;
  user-select: text;
  -webkit-user-select: text;
  -moz-user-select: text;
  -ms-user-select: text;
}

.shell-content {
  margin: 0;
  padding: 0;
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
  font-size: 14px;
  line-height: 1.5;
  color: #c9d1d9;
  white-space: pre-wrap;
  word-wrap: break-word;
  user-select: text;
  -webkit-user-select: text;
  -moz-user-select: text;
  -ms-user-select: text;
}

.input-container {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}

.command-input {
  flex: 1;
}

.command-input :deep(.el-input__inner) {
  background-color: #21262d;
  border-color: #30363d;
  color: #c9d1d9;
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
}

.command-input :deep(.el-input__wrapper) {
  background-color: #21262d;
  box-shadow: 0 0 0 1px #30363d inset;
}

.command-input :deep(.el-input__wrapper.is-focus) {
  box-shadow: 0 0 0 1px #58a6ff inset;
}

.command-input :deep(.el-input__inner:focus) {
  border-color: #58a6ff;
}

.toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 2px 0;
  border-bottom: 1px solid #30363d;
  padding-bottom: 10px;
  flex-shrink: 0;
}

.toolbar-group {
  display: flex;
  gap: 4px;
}

.status-text {
  font-size: 12px;
  color: #7d8590;
  display: flex;
  align-items: center;
  gap: 6px;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--theme-success, #52c41a);
  display: inline-block;
  box-shadow: 0 0 6px var(--theme-success, #52c41a);
}

/* 滚动条样式 */
.output::-webkit-scrollbar {
  width: 8px;
}

.output::-webkit-scrollbar-track {
  background: #161b22;
  border-radius: 4px;
}

.output::-webkit-scrollbar-thumb {
  background: #30363d;
  border-radius: 4px;
}

.output::-webkit-scrollbar-thumb:hover {
  background: #484f58;
}

/* 支持的文本选择效果 */
.shell-content::selection {
  background-color: #58a6ff;
  color: #ffffff;
}

.shell-content::-moz-selection {
  background-color: #58a6ff;
  color: #ffffff;
}
</style>
