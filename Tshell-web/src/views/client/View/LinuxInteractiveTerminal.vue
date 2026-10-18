<template>
  <div class="xterm-container cs-font-ui">
    <!-- Toolbar -->
    <div class="term-toolbar">
      <div class="term-title-group">
        <span class="term-title">{{ i18nStore.tl('终端交互: ', 'Terminal: ') }}{{ clientInfo.username }}@{{ clientInfo.computer }}</span>
        <span
          class="term-status"
          :class="isConnected ? 'is-on' : (isConnecting ? 'is-connecting' : 'is-off')"
        >
          {{ isConnected ? (i18nStore.tl('已连接', 'Connected')) : isConnecting ? (i18nStore.tl('连接中...', 'Connecting...')) : (i18nStore.tl('未连接', 'Disconnected')) }}
        </span>
      </div>

      <div class="term-spacer" />

      <button
        class="cs-btn"
        :class="isConnected ? 'cs-btn-danger' : 'cs-btn-primary'"
        :disabled="isConnecting"
        @click="toggleConnection"
      >
        {{ isConnected ? (i18nStore.tl('断开', 'Disconnect')) : (i18nStore.tl('连接', 'Connect')) }}
      </button>
      <button class="cs-btn" :disabled="!isConnected" @click="clearTerminal">
        {{ i18nStore.tl('清屏', 'Clear') }}
      </button>
      <button class="cs-btn" @click="toggleFullscreen">
        {{ isFullscreen ? (i18nStore.tl('退出全屏', 'Exit Fullscreen')) : (i18nStore.tl('全屏查看', 'Fullscreen')) }}
      </button>
      <button class="cs-btn" @click="showSettings = true">
        {{ i18nStore.tl('偏好设置', 'Preferences') }}
      </button>
    </div>

    <!-- 终端状态栏 -->
    <div class="terminal-status">
      <div class="status-left">
        <span class="conn-tag" :class="`is-${connectionStatus.type}`">
          {{ connectionStatus.text }}
        </span>
        <span class="status-info">
          {{ terminalSize.cols }}×{{ terminalSize.rows }}
        </span>
        <span class="status-info" v-if="isConnected">
          延迟: {{ latency }}ms
        </span>
      </div>
      <div class="status-right">
        <span class="encoding-info">UTF-8</span>
        <span class="cursor-info">{{ cursorPosition.x }},{{ cursorPosition.y }}</span>
      </div>
    </div>

    <!-- 终端主体 -->
    <div 
      class="terminal-body" 
      ref="terminalElement"
    >
      <!-- xterm.js 将在这里挂载 -->
    </div>

    <!-- 终端设置对话框 -->
    <el-dialog
      v-model="showSettings"
      title="终端设置"
      width="500px"
      class="cs-dialog"
      :before-close="handleSettingsClose"
    >
      <el-form :model="terminalConfig" label-width="120px">
        <el-form-item label="字体大小">
          <el-slider
            v-model="terminalConfig.fontSize"
            :min="8"
            :max="24"
            :step="1"
            @change="updateTerminalConfig"
          />
          <span class="config-value">{{ terminalConfig.fontSize }}px</span>
        </el-form-item>
        
        <el-form-item label="字体族">
          <el-select 
            v-model="terminalConfig.fontFamily" 
            @change="updateTerminalConfig"
            placeholder="选择字体"
          >
            <el-option label="Consolas" value="Consolas, monospace" />
            <el-option label="Monaco" value="Monaco, monospace" />
            <el-option label="Ubuntu Mono" value="'Ubuntu Mono', monospace" />
            <el-option label="Fira Code" value="'Fira Code', monospace" />
            <el-option label="Source Code Pro" value="'Source Code Pro', monospace" />
          </el-select>
        </el-form-item>

        <el-form-item label="主题">
          <el-select 
            v-model="terminalConfig.theme" 
            @change="updateTerminalConfig"
            placeholder="选择主题"
          >
            <el-option label="默认暗色" value="dark" />
            <el-option label="Kali Linux" value="kali" />
            <el-option label="Ubuntu" value="ubuntu" />
            <el-option label="Matrix" value="matrix" />
            <el-option label="Dracula" value="dracula" />
          </el-select>
        </el-form-item>

        <el-form-item label="光标样式">
          <el-radio-group 
            v-model="terminalConfig.cursorStyle" 
            @change="updateTerminalConfig"
          >
            <el-radio value="block">块状</el-radio>
            <el-radio value="underline">下划线</el-radio>
            <el-radio value="bar">竖线</el-radio>
          </el-radio-group>
        </el-form-item>

        <el-form-item label="光标闪烁">
          <el-switch 
            v-model="terminalConfig.cursorBlink"
            @change="updateTerminalConfig"
          />
        </el-form-item>

        <el-form-item label="快速复制">
          <el-switch 
            v-model="terminalConfig.rightClickSelectsWord"
            @change="updateTerminalConfig"
          />
          <div class="config-help">
            <small>启用后可通过右键快速选择单词</small>
          </div>
        </el-form-item>
      </el-form>
      
      <template #footer>
        <div class="cs-dialog-footer-center">
          <button class="cs-btn cs-btn-primary" @click="showSettings = false">确定</button>
          <button class="cs-btn" @click="resetTerminalConfig">重置默认</button>
        </div>
      </template>
    </el-dialog>

    <!-- 连接错误提示 -->
    <el-dialog
      v-model="showError"
      title="连接错误"
      width="400px"
      class="cs-dialog"
      :show-close="false"
    >
      <div class="error-content">
        <el-icon class="error-icon"><Warning /></el-icon>
        <p>{{ errorMessage }}</p>
      </div>
      <template #footer>
        <div class="cs-dialog-footer-center">
          <button class="cs-btn cs-btn-primary" @click="retryConnection">重试连接</button>
          <button class="cs-btn" @click="showError = false">关闭</button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, onUnmounted, nextTick, watch, computed, inject } from 'vue'
import type { Ref, ComputedRef } from 'vue'
import { Terminal } from 'xterm'
import { FitAddon } from 'xterm-addon-fit'
import { WebLinksAddon } from 'xterm-addon-web-links'
import { WebglAddon } from 'xterm-addon-webgl'
import { Unicode11Addon } from 'xterm-addon-unicode11'
import { ElMessage, ElNotification } from 'element-plus'
import { Connection, Delete, Setting, Warning, FullScreen, Aim } from '@element-plus/icons-vue'
import { useRoute, useRouter } from 'vue-router'
import { useClientUid } from '@/utils/clientUtils'
import { useClientsStore } from '@/stores/clients'
import { useUserStore } from '@/stores/user'
import { useI18nStore } from '@/stores/i18n'
import { usePreferencesStore } from '@/stores/preferences'

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)
import 'xterm/css/xterm.css'

const route = useRoute()
const router = useRouter()
const clientsStore = useClientsStore()
const userStore = useUserStore()
const prefStore = usePreferencesStore()

// 获取UID
const injectedUid = inject<Ref<string> | ComputedRef<string> | null>('realUid', null)
const getUid = () => {
  // injectedUid 的类型是 Ref<string> | ComputedRef<string>，两者都有 .value，无需断言
  const v = injectedUid?.value ? String(injectedUid.value) : ''
  if (v && v.length > 10) return v
  const mapped = useClientUid(route)
  return typeof mapped === 'string' && mapped.length > 10 ? mapped : ''
}

// 终端会话相关变量

// 响应式数据
const terminalElement = ref<HTMLElement>()
const isConnected = ref(false)
const isConnecting = ref(false)
const showSettings = ref(false)
const showError = ref(false)
const errorMessage = ref('')
const latency = ref(0)
const isFullscreen = ref(false)

// 终端实例和插件
let terminal: Terminal | null = null
let fitAddon: FitAddon | null = null
let webLinksAddon: WebLinksAddon | null = null
let webglAddon: WebglAddon | null = null
let unicode11Addon: Unicode11Addon | null = null
let websocket: WebSocket | null = null

// 客户端信息
const clientInfo = reactive({
  uid: getUid(),
  computer: 'kali',
  username: 'root',
  internalIP: '',
  externalIP: '',
  arch: 'linux_amd64',
  process: 'bash'
})

// 终端配置
const terminalConfig = reactive({
  fontSize: 21,
  fontFamily: 'Consolas, monospace',
  theme: 'kali' as keyof typeof terminalThemes,
  cursorStyle: 'block' as 'block' | 'underline' | 'bar',
  cursorBlink: true,
  rightClickSelectsWord: true
})

// 终端状态
const terminalSize = reactive({
  cols: 80,
  rows: 24
})

const cursorPosition = reactive({
  x: 0,
  y: 0
})

const terminalHeight = ref(600)

// 计算属性
const connectionStatus = computed(() => {
  if (isConnecting.value) {
    return { type: 'warning' as const, text: '连接中...' }
  }
  if (isConnected.value) {
    return { type: 'success' as const, text: '已连接' }
  }
  return { type: 'danger' as const, text: '未连接' }
})

// 终端主题配置
const terminalThemes = {
  dark: {
    background: '#1e1e1e',
    foreground: '#ffffff',
    cursor: '#ffffff',
    black: '#000000',
    red: '#cd3131',
    green: '#0dbc79',
    yellow: '#e5e510',
    blue: '#2472c8',
    magenta: '#bc3fbc',
    cyan: '#11a8cd',
    white: '#e5e5e5',
    brightBlack: '#666666',
    brightRed: '#f14c4c',
    brightGreen: '#23d18b',
    brightYellow: '#f5f543',
    brightBlue: '#3b8eea',
    brightMagenta: '#d670d6',
    brightCyan: '#29b8db',
    brightWhite: '#e5e5e5'
  },
  kali: {
    background: '#0c0c0c',
    foreground: '#f0f0f0',
    cursor: '#00ff00',
    black: '#000000',
    red: '#ff5555',
    green: '#50fa7b',
    yellow: '#f1fa8c',
    blue: '#bd93f9',
    magenta: '#ff79c6',
    cyan: '#8be9fd',
    white: '#f8f8f2'
  },
  ubuntu: {
    background: '#2c001e',
    foreground: '#ffffff',
    cursor: '#ffffff',
    black: '#2e3436',
    red: '#cc0000',
    green: '#4e9a06',
    yellow: '#c4a000',
    blue: '#3465a4',
    magenta: '#75507b',
    cyan: '#06989a',
    white: '#d3d7cf'
  },
  matrix: {
    background: '#000000',
    foreground: '#00ff00',
    cursor: '#00ff00',
    black: '#000000',
    red: '#ff0000',
    green: '#00ff00',
    yellow: '#ffff00',
    blue: '#0000ff',
    magenta: '#ff00ff',
    cyan: '#00ffff',
    white: '#ffffff'
  },
  dracula: {
    background: '#282a36',
    foreground: '#f8f8f2',
    cursor: '#f8f8f2',
    black: '#21222c',
    red: '#ff5555',
    green: '#50fa7b',
    yellow: '#f1fa8c',
    blue: '#bd93f9',
    magenta: '#ff79c6',
    cyan: '#8be9fd',
    white: '#f8f8f2'
  }
}

// 生命周期
let resizeObserver: ResizeObserver | null = null

onMounted(async () => {
  await loadClientInfo()
  await nextTick()
  initializeTerminal()
  
  // 监听窗口与容器大小变化
  window.addEventListener('resize', handleResize)

  if (terminalElement.value && typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(() => {
      handleResize()
    })
    resizeObserver.observe(terminalElement.value)
  }
  
  // 监听全屏状态变化
  document.addEventListener('fullscreenchange', handleFullscreenChange)
  document.addEventListener('webkitfullscreenchange', handleFullscreenChange)
  document.addEventListener('msfullscreenchange', handleFullscreenChange)
})

onUnmounted(() => {
  cleanup()
  window.removeEventListener('resize', handleResize)

  if (resizeObserver) {
    resizeObserver.disconnect()
    resizeObserver = null
  }
  
  // 移除全屏状态监听器
  document.removeEventListener('fullscreenchange', handleFullscreenChange)
  document.removeEventListener('webkitfullscreenchange', handleFullscreenChange)
  document.removeEventListener('msfullscreenchange', handleFullscreenChange)
})

// 加载客户端信息
const loadClientInfo = async () => {
  const uid = getUid()
  if (!clientsStore.clientsList || clientsStore.clientsList.length === 0) {
    await clientsStore.fetchClients()
  }
  
  if (uid && clientsStore.clientsList && clientsStore.clientsList.length > 0) {
    const client = clientsStore.clientsList.find((c: any) => c.Uid === uid)
    if (client) {
      clientInfo.computer = client.Computer || 'linux'
      clientInfo.username = client.Username || 'root'
      clientInfo.arch = client.Arch || 'linux_amd64'
      clientInfo.process = client.Process || 'bash'
      clientInfo.internalIP = client.InternalIP || ''
      clientInfo.externalIP = client.ExternalIP || ''
    }
  }
}

// 方法
const initializeTerminal = () => {
  if (!terminalElement.value) {
    console.error('❌ 终端元素不存在')
    return
  }

  console.log('🚀 正在初始化终端...')

  // 创建终端实例
  terminal = new Terminal({
    fontSize: terminalConfig.fontSize,
    fontFamily: terminalConfig.fontFamily,
    theme: terminalThemes[terminalConfig.theme],
    cursorBlink: terminalConfig.cursorBlink,
    cursorStyle: terminalConfig.cursorStyle,
    rightClickSelectsWord: terminalConfig.rightClickSelectsWord,
    allowTransparency: true,
    convertEol: true,
    scrollback: prefStore.scrollback || 5000,
    tabStopWidth: 4,
    allowProposedApi: true
  })

  console.log('✅ Terminal 实例已创建')

  // 创建插件
  fitAddon = new FitAddon()
  webLinksAddon = new WebLinksAddon()
  unicode11Addon = new Unicode11Addon()
  
  try {
    webglAddon = new WebglAddon()
    console.log('✅ WebGL插件已创建')
  } catch (e) {
    console.warn('⚠️ WebGL插件加载失败，将使用Canvas渲染:', e)
  }

  // 加载插件
  console.log('🔌 加载终端插件...')
  terminal.loadAddon(fitAddon)
  terminal.loadAddon(webLinksAddon)
  terminal.loadAddon(unicode11Addon)
  
  if (webglAddon) {
    try {
      webglAddon.onContextLoss(() => {
        webglAddon?.dispose()
        console.warn('⚠️ WebGL 上下文丢失，降级为 Canvas 渲染')
      })
      terminal.loadAddon(webglAddon)
      console.log('✅ WebGL插件已加载')
    } catch (e) {
      console.warn('⚠️ WebGL插件启用失败:', e)
    }
  }

  // 激活Unicode11支持
  terminal.unicode.activeVersion = '11'
  console.log('✅ Unicode11支持已激活')

  // 打开终端
  console.log('📺 正在打开终端...')
  terminal.open(terminalElement.value)
  console.log('✅ 终端已打开')
  
  // 适配大小
  console.log('📏 正在适配终端大小...')
  fitAddon.fit()
  console.log('✅ 终端大小适配完成')
  
  // 更新终端大小
  terminalSize.cols = terminal.cols
  terminalSize.rows = terminal.rows
  console.log(`📊 终端大小: ${terminal.cols}x${terminal.rows}`)

  // 监听终端事件
  console.log('🗣️ 正在注册终端事件监听器...')
  
  // 用户输入直接透传给后端PTY/ConPTY，由远端负责回显（本地不再echo，避免命令重复显示）
  terminal.onData(data => {
    if (websocket && websocket.readyState === WebSocket.OPEN) {
      websocket.send(data)
    }
  })

  terminal.onBinary(data => {
    console.log('📛 二进制输入:', data)
    if (websocket && websocket.readyState === WebSocket.OPEN) {
      sendMessage('binary', data)
    }
  })

  terminal.onResize(size => {
    console.log(`📏 终端大小变化: ${size.cols}x${size.rows}`)
    terminalSize.cols = size.cols
    terminalSize.rows = size.rows
    
    if (websocket && websocket.readyState === WebSocket.OPEN) {
      // 使用示例代码的格式发送终端大小
      const sizeData = {
        cols: size.cols,
        rows: size.rows,
        x: 0,
        y: 0
      }
      const message = '\x01' + JSON.stringify(sizeData)
      websocket.send(message)
      console.log('📏 已发送终端大小调整:', sizeData)
    }
  })

  terminal.onCursorMove(() => {
    cursorPosition.x = terminal?.buffer.active.cursorX || 0
    cursorPosition.y = terminal?.buffer.active.cursorY || 0
  })

  // 显示欢迎信息
  console.log('📝 正在显示欢迎信息...')
  terminal.writeln('🚀 交互式终端已初始化 (XTerm.js + WebSocket + PTY)')
  terminal.writeln('💡 点击上方的“连接”按钮开始终端会话')
  terminal.writeln('✨ 支持: ANSI颜色、光标控制、动态窗口大小调整')
  terminal.writeln('')
  
  console.log('✅ 终端初始化完成！')
}

const connectToTerminal = async () => {
  if (isConnecting.value || isConnected.value) return

  isConnecting.value = true

  try {
    // access token 仅存内存：页面刷新/新标签页后为空，需先凭 HttpOnly Cookie 续期，
    // 否则 WS 首帧发空串，服务端只能回「认证令牌无效」
    const token = await userStore.ensureToken()
    if (!token) {
      ElMessage.error(isZh.value ? '登录态已失效，请重新登录后再打开终端' : 'Session expired — sign in again before opening the terminal')
      isConnecting.value = false
      return
    }

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsUrl = `${protocol}//${window.location.host}/api/xterm/connect?uid=${clientInfo.uid}`

    console.log('🔌 连接到XTerm WebSocket:', wsUrl)
    console.log('🔑 客户端UID:', clientInfo.uid)
    console.log('🔑 JWT Token:', token ? '✓ 存在' : '❌ 缺失')

    websocket = new WebSocket(wsUrl)

    websocket.onopen = () => {
      console.log('🔌 XTerm WebSocket 连接已建立')

      // 发送认证信息
      sendMessage('auth', token)
      
      // 发送终端大小信息
      sendMessage('resize', null, {
        cols: terminalSize.cols,
        rows: terminalSize.rows,
        x: 0,
        y: 0
      })
    }

    websocket.onmessage = (event) => {
      try {
        console.log('📨 收到WebSocket消息:', event.data)
        
        // 优先处理二进制数据（终端输出）
        if (event.data instanceof Blob) {
          console.log('📨 收到Blob数据')
          const reader = new FileReader()
          reader.onload = () => {
            if (terminal) {
              terminal.write(reader.result as string)
              console.log('✅ Blob数据已写入终端')
            }
          }
          reader.readAsText(event.data)
        } else if (event.data instanceof ArrayBuffer) {
          console.log('📨 收到ArrayBuffer数据')
          handleBinaryData(event.data)
        } else if (typeof event.data === 'string') {
          // 尝试解析JSON，如果失败则作为原始文本处理
          try {
            const message = JSON.parse(event.data)
            console.log('📨 收到JSON消息:', message)
            handleMessage(message)
          } catch (jsonError) {
            // 原始文本数据，直接写入终端
            console.log('📨 收到原始文本数据:', event.data.substring(0, 100))
            if (terminal) {
              terminal.write(event.data)
              console.log('✅ 原始文本已写入终端')
            }
          }
        }
      } catch (error) {
        console.error('处理WebSocket消息失败:', error)
      }
    }

    websocket.onclose = (event) => {
      console.log('🔌 XTerm WebSocket 连接已关闭', event.code, event.reason)
      isConnected.value = false
      isConnecting.value = false
      
      if (terminal) {
        terminal.writeln('\r\n💔 终端连接已断开')
      }
      
      if (event.code !== 1000) {
        ElNotification({
          title: '连接断开',
          message: '终端连接意外断开，请检查网络连接',
          type: 'warning'
        })
      }
    }

    websocket.onerror = (error) => {
      console.error('❌ XTerm WebSocket 连接错误:', error)
      isConnecting.value = false
      showErrorDialog('WebSocket连接失败，请检查网络连接')
    }

  } catch (error) {
    console.error('❌ 创建WebSocket连接失败:', error)
    isConnecting.value = false
    showErrorDialog('创建连接失败: ' + (error as Error).message)
  }
}

const handleBinaryData = async (data: ArrayBuffer | Blob) => {
  let arrayBuffer: ArrayBuffer

  if (data instanceof Blob) {
    arrayBuffer = await data.arrayBuffer()
  } else {
    arrayBuffer = data
  }

  const uint8Array = new Uint8Array(arrayBuffer)
  const text = new TextDecoder('utf-8').decode(uint8Array)
  
  console.log('📝 处理二进制数据:', text.substring(0, 100), '...')
  
  if (terminal) {
    terminal.write(text)
    console.log('✅ 已写入终端')
  } else {
    console.log('❌ 终端实例不存在')
  }
}

const handleMessage = (message: any) => {
  switch (message.type) {
    case 'auth_success':
      isConnected.value = true
      isConnecting.value = false
      
      if (terminal) {
        terminal.clear()
        terminal.writeln('\x1b[32m✓ 交互式终端连接成功!\x1b[0m')
        terminal.writeln(`\x1b[36m🖥️  连接到: ${clientInfo.computer}@${clientInfo.username}\x1b[0m`)
        terminal.writeln(`\x1b[33m🌐 内网IP: ${clientInfo.internalIP}\x1b[0m`)
        terminal.writeln(`\x1b[34m🔗 外网IP: ${clientInfo.externalIP}\x1b[0m`)
        terminal.writeln(`\x1b[35m📦 架构: ${clientInfo.arch}\x1b[0m`)
        terminal.writeln('')
        
        // 发送初始终端大小（使用示例代码的格式）
        const sizeData = {
          cols: terminalSize.cols,
          rows: terminalSize.rows,
          x: 0,
          y: 0
        }
        const sizeMessage = '\x01' + JSON.stringify(sizeData)
        websocket?.send(sizeMessage)
        console.log('📏 已发送初始终端大小:', sizeData)
        
        // 注意：交互式终端已在handleXTermSession中自动启动
        // 不需要再次发送启动命令，避免重复启动
        console.log('✅ 交互式终端将自动启动，等待数据输出...')
      }
      
      ElMessage.success('交互式终端连接成功!')
      break

    case 'error':
      isConnecting.value = false
      if (terminal) {
        terminal.writeln(`\x1b[31m❌ 错误: ${message.content}\x1b[0m`)
      }
      showErrorDialog(message.content)
      break

    case 'output':
    case 'terminal_output':
    case 'shell_output':
      if (terminal && message.content) {
        console.log('🖥️ 终端输出:', message.content)
        // 直接写入终端数据，保持ANSI转义序列
        if (typeof message.content === 'string') {
          terminal.write(message.content)
        } else if (message.content instanceof ArrayBuffer) {
          const text = new TextDecoder('utf-8').decode(new Uint8Array(message.content))
          terminal.write(text)
        }
      }
      break

    case 'shell_started':
      if (terminal) {
        terminal.writeln('\x1b[32m✓ 交互式终端已启动\x1b[0m')
        terminal.writeln('')
        terminal.writeln('\x1b[33m🔄 等待终端输出...\x1b[0m')
        
        // 不需要发送回车，等待PTY自然输出命令提示符
        console.log('✅ 交互式终端已启动，等待PTY输出')
      }
      break

    case 'pong': {
      // 计算延迟
      const now = Date.now()
      latency.value = now - (message.content * 1000)
      break
    }

    default:
      console.log('收到未知消息类型:', message.type)
  }
}

const sendMessage = (type: string, content: any, size?: any) => {
  if (websocket && websocket.readyState === WebSocket.OPEN) {
    const message = {
      type,
      content,
      ...(size && { size })
    }
    websocket.send(JSON.stringify(message))
  }
}

const toggleConnection = () => {
  if (isConnected.value) {
    disconnectTerminal()
  } else {
    connectToTerminal()
  }
}

const disconnectTerminal = () => {
  // 清理输入缓冲
  
  if (websocket) {
    websocket.close(1000, 'User disconnected')
    websocket = null
  }
  isConnected.value = false
  isConnecting.value = false
}

const toggleFullscreen = () => {
  if (!isFullscreen.value) {
    enterFullscreen()
  } else {
    exitFullscreen()
  }
}

// 浏览器厂商前缀的全屏 API：TS 的 DOM lib 只声明了标准名，
// 老 Safari/旧 Edge 只认前缀版。集中声明一次，避免下面每处 `as any`
// （那会把整个表达式类型放开，方法名拼错也不报错）。
declare global {
  interface Element {
    webkitRequestFullscreen?: () => Promise<void> | void
    msRequestFullscreen?: () => Promise<void> | void
  }
  interface Document {
    webkitExitFullscreen?: () => Promise<void> | void
    msExitFullscreen?: () => Promise<void> | void
    webkitFullscreenElement?: Element | null
    msFullscreenElement?: Element | null
  }
}

const enterFullscreen = () => {
  const terminalContainer = document.querySelector('.xterm-container') as HTMLElement
  if (terminalContainer) {
    if (terminalContainer.requestFullscreen) {
      terminalContainer.requestFullscreen()
    } else if (terminalContainer.webkitRequestFullscreen) {
      terminalContainer.webkitRequestFullscreen()
    } else if (terminalContainer.msRequestFullscreen) {
      terminalContainer.msRequestFullscreen()
    }
  }
}

const exitFullscreen = () => {
  if (document.exitFullscreen) {
    document.exitFullscreen()
  } else if (document.webkitExitFullscreen) {
    document.webkitExitFullscreen()
  } else if (document.msExitFullscreen) {
    document.msExitFullscreen()
  }
}

const handleFullscreenChange = () => {
  const isCurrentlyFullscreen = !!(document.fullscreenElement || 
    document.webkitFullscreenElement ||
    document.msFullscreenElement)
  
  isFullscreen.value = isCurrentlyFullscreen
  
  // 全屏状态变化时重新适配终端大小
  nextTick(() => {
    handleResize()
  })
}

const clearTerminal = () => {
  if (terminal) {
    terminal.clear()
  }
}

const handleResize = () => {
  if (fitAddon && terminal && terminalElement.value) {
    try {
      if (terminalElement.value.clientHeight > 0 && terminalElement.value.clientWidth > 0) {
        fitAddon.fit()
        terminalSize.cols = terminal.cols
        terminalSize.rows = terminal.rows
      }
    } catch (error) {
      console.warn('终端大小适配失败:', error)
    }
  }
}

const updateTerminalConfig = () => {
  if (!terminal) return

  // 更新终端选项
  terminal.options.fontSize = terminalConfig.fontSize
  terminal.options.fontFamily = terminalConfig.fontFamily
  terminal.options.theme = terminalThemes[terminalConfig.theme]
  terminal.options.cursorStyle = terminalConfig.cursorStyle
  terminal.options.cursorBlink = terminalConfig.cursorBlink
  terminal.options.rightClickSelectsWord = terminalConfig.rightClickSelectsWord

  // 刷新终端
  terminal.refresh(0, terminal.rows - 1)
  
  // 重新适配大小
  if (fitAddon) {
    nextTick(() => {
      fitAddon?.fit()
    })
  }
}

const resetTerminalConfig = () => {
  terminalConfig.fontSize = 14
  terminalConfig.fontFamily = 'Consolas, monospace'
  terminalConfig.theme = 'kali'
  terminalConfig.cursorStyle = 'block'
  terminalConfig.cursorBlink = true
  terminalConfig.rightClickSelectsWord = true
  
  updateTerminalConfig()
}

const handleSettingsClose = () => {
  showSettings.value = false
}

const showErrorDialog = (message: string) => {
  errorMessage.value = message
  showError.value = true
}

const retryConnection = () => {
  showError.value = false
  connectToTerminal()
}

const closeTerminal = () => {
  disconnectTerminal()
}

const minimizeTerminal = () => {
  terminalHeight.value = 200
  nextTick(() => {
    handleResize()
  })
}

const maximizeTerminal = () => {
  terminalHeight.value = window.innerHeight - 200
  nextTick(() => {
    handleResize()
  })
}

const cleanup = () => {
  disconnectTerminal()

  // 清理输入缓冲

  // WebGL addon 需先于 terminal.dispose() 卸载：addon 的还原回调会在终端服务销毁后
  // 再调 _createRenderer()（此时返回 undefined），触发 xterm 的 setRenderer 空指针
  if (webglAddon) {
    try {
      webglAddon.dispose()
    } catch {
      /* addon 卸载异常不影响后续清理 */
    }
    webglAddon = null
  }

  if (terminal) {
    terminal.dispose()
    terminal = null
  }

  fitAddon = null
  webLinksAddon = null
  unicode11Addon = null
}

// 监听配置变化
watch(terminalConfig, () => {
  updateTerminalConfig()
}, { deep: true })

// 定时发送心跳
let heartbeatInterval: number | null = null

watch(isConnected, (connected) => {
  if (connected) {
    heartbeatInterval = window.setInterval(() => {
      if (websocket && websocket.readyState === WebSocket.OPEN) {
        sendMessage('ping', Date.now())
      }
    }, 30000) // 30秒心跳
  } else {
    if (heartbeatInterval) {
      clearInterval(heartbeatInterval)
      heartbeatInterval = null
    }
  }
})
</script>

<style scoped lang="scss">
.xterm-container {
  display: flex;
  flex-direction: column;
  background: var(--cs-bg);
  overflow: hidden;
  height: 100%;
  width: 100%;
  min-height: 0;

  // 全屏模式样式
  &:fullscreen,
  &:-webkit-full-screen,
  &:-moz-full-screen,
  &:-ms-fullscreen {
    border-radius: 0;
    box-shadow: none;
    z-index: 9999;

    .terminal-body {
      flex: 1;
      height: 100%;
      margin: 0;
      border: none;
      border-radius: 0;
    }
  }

  .term-toolbar {
    height: 48px;
    background: var(--cs-panel);
    border-bottom: 1px solid var(--cs-card-border);
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 0 12px;
    flex-shrink: 0;

    .term-title-group {
      display: flex;
      align-items: center;
      gap: 8px;
      min-width: 0;
    }

    .term-title {
      font-size: 14px;
      font-weight: 600;
      color: var(--cs-text-bright);
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }

    // 状态 tag：已连接实心绿底白字 / 空闲灰底
    .term-status {
      display: inline-flex;
      align-items: center;
      height: 20px;
      padding: 0 8px;
      border-radius: 4px;
      font-size: 12px;
      white-space: nowrap;
      background: color-mix(in srgb, var(--cs-text-dim) 12%, transparent);
      color: var(--cs-text-dim);

      &.is-on {
        background: var(--cs-online);
        color: #ffffff;
      }

      &.is-connecting {
        background: color-mix(in srgb, var(--cs-warning) 18%, transparent);
        color: var(--cs-warning);
      }
    }

    .term-spacer {
      flex: 1;
    }
  }

  .terminal-status {
    display: flex;
    justify-content: space-between;
    align-items: center;
    background: var(--cs-panel);
    padding: 4px 12px;
    border-bottom: 1px solid var(--cs-card-border);
    font-size: 12px;
    flex-shrink: 0;

    .status-left {
      display: flex;
      align-items: center;
      gap: 12px;

      .conn-tag {
        display: inline-flex;
        align-items: center;
        height: 20px;
        padding: 0 8px;
        border-radius: 4px;
        font-size: 12px;
        white-space: nowrap;
        background: color-mix(in srgb, var(--cs-text-dim) 12%, transparent);
        color: var(--cs-text-dim);

        &.is-success {
          background: var(--cs-online);
          color: #ffffff;
        }

        &.is-warning {
          background: color-mix(in srgb, var(--cs-warning) 18%, transparent);
          color: var(--cs-warning);
        }

        &.is-danger {
          background: color-mix(in srgb, var(--cs-error) 12%, transparent);
          color: var(--cs-error);
        }
      }

      .status-info {
        color: var(--cs-text-dim);
        font-family: var(--cs-font-mono);
      }
    }

    .status-right {
      display: flex;
      align-items: center;
      gap: 12px;

      .encoding-info,
      .cursor-info {
        color: var(--cs-text-dim);
        font-family: var(--cs-font-mono);
      }
    }
  }

  .terminal-body {
    background: #0c0c0c;
    margin: 12px;
    border: 1px solid var(--cs-card-border);
    border-radius: 8px;
    padding: 2px 4px;
    overflow: hidden;
    flex: 1;
    min-height: 0;
    position: relative;
    box-sizing: border-box;

    :deep(.xterm) {
      height: 100% !important;
      width: 100% !important;
      font-family: 'Consolas', 'Ubuntu Mono', 'Monaco', monospace;
      position: relative;
      box-sizing: border-box;
    }

    :deep(.xterm .xterm-viewport) {
      background-color: transparent !important;
      overflow-y: auto !important;
      width: 100% !important;
    }

    :deep(.xterm .xterm-screen) {
      background-color: transparent !important;
      box-sizing: border-box;
    }

    :deep(.xterm .xterm-rows) {
      background-color: transparent !important;
      z-index: 20;
    }

    :deep(.xterm .xterm-decoration-overview-ruler) {
      right: 0;
    }

    :deep(.xterm-cursor-layer) {
      animation: blink 1s infinite;
      z-index: 30;
    }

    // 确保终端内容在最上层
    :deep(.xterm-helper-textarea) {
      z-index: 30;
    }

    // 隐藏背景水印
    &::before {
      display: none !important;
    }
  }
}

// 设置对话框样式
:deep(.el-dialog) {
  .config-value {
    margin-left: 12px;
    color: var(--cs-active);
    font-weight: 500;
  }

  .config-help {
    margin-top: 4px;
    color: var(--cs-text-dim);
  }
}

// 错误对话框样式
.error-content {
  display: flex;
  align-items: center;
  gap: 12px;

  .error-icon {
    font-size: 24px;
    color: var(--cs-error);
  }

  p {
    margin: 0;
    color: var(--cs-text);
  }
}

// 光标闪烁动画
@keyframes blink {
  0%, 50% {
    opacity: 1;
  }
  51%, 100% {
    opacity: 0;
  }
}

// 响应式样式
@media (max-width: 768px) {
  .terminal-header {
    .header-left .terminal-title {
      display: none;
    }
    
    .header-right .el-button-group {
      .el-button span {
        display: none;
      }
    }
  }

  .terminal-status {
    .status-right {
      display: none;
    }
  }
}
</style>