<template>
<div class="plugin-container cs-font-ui">
  <!-- 页签：插件 / 脚本（vshell：同域功能并页） -->
  <div class="plug-tabs">
    <div class="plug-tab" :class="{ active: view === 'plugins' }" @click="view = 'plugins'">
      {{ isZh ? '插件运行' : 'Plugins' }}
    </div>
    <div class="plug-tab" :class="{ active: view === 'scripts' }" @click="view = 'scripts'">
      {{ isZh ? '脚本 (CNA)' : 'Scripts (CNA)' }}
    </div>
  </div>

  <template v-if="view === 'plugins'">
  <!-- 工具条：左侧标题/计数，右侧操作 -->
  <div class="plug-toolbar">
    <div class="plug-title-group">
      <span class="plug-title">{{ isZh ? '内存执行与插件模块' : 'Memory Execution & Plugins' }}</span>
      <span class="plug-count">({{ plugins.length }} {{ isZh ? '个插件可用' : 'plugins available' }})</span>
    </div>

    <div class="plug-spacer" />

    <button class="cs-btn cs-btn-primary" @click="showUploadDialog = true">
      {{ isZh ? '上传插件' : 'Upload Plugin' }}
    </button>
    <button class="cs-btn" @click="refreshPlugins">
      {{ isZh ? '刷新' : 'Refresh' }}
    </button>
  </div>

  <div class="plugin-card">
    <el-form :model="pluginForm" label-width="120px" class="plugin-form">
      <el-form-item :label="isZh ? '选择客户端' : 'Select Client'" required>
        <el-select
          v-model="pluginForm.clientUid"
          :placeholder="isZh ? '请选择已上线的客户端' : 'Select an online client'"
          class="full-width"
        >
          <el-option
            v-for="client in onlineClients"
            :key="client.Uid"
            :label="getClientDisplayInfo(client)"
            :value="client.Uid"
          />
        </el-select>
      </el-form-item>

      <!-- Windows内存执行模式选择 -->
      <el-form-item :label="isZh ? '执行模式' : 'Execution Mode'">
        <div class="execution-mode-buttons">
          <el-button
            :type="executionMode === 'execute-assembly' ? 'primary' : 'default'"
            @click="setExecutionMode('execute-assembly')"
            :disabled="!isWindowsClient"
            class="mode-button"
          >
            {{ isZh ? 'Execute Assembly(.NET程序内存执行)' : 'Execute Assembly (.NET in-memory)' }}
          </el-button>
          <el-button
            :type="executionMode === 'inline-bin' ? 'primary' : 'default'"
            @click="setExecutionMode('inline-bin')"
            :disabled="!isWindowsClient"
            class="mode-button"
          >
            {{ isZh ? 'Inline Bin(其他exe程序内存执行)' : 'Inline Bin (other exe in-memory)' }}
          </el-button>
          <el-button
            :type="executionMode === 'shellcode-inject' ? 'primary' : 'default'"
            @click="setExecutionMode('shellcode-inject')"
            :disabled="!isWindowsClient"
            class="mode-button"
          >
            {{ isZh ? 'shellcode执行(执行shellcode,上线其他C2等)' : 'Shellcode (run shellcode, stage other C2)' }}
          </el-button>
          <el-button
            :type="executionMode === 'script' ? 'primary' : 'default'"
            @click="setExecutionMode('script')"
            :disabled="isWindowsClient"
            class="mode-button"
          >
            {{ isZh ? 'Script(Linux脚本内存执行)' : 'Script (Linux script in-memory)' }}
          </el-button>
          <el-button
            :type="executionMode === 'binary' ? 'primary' : 'default'"
            @click="setExecutionMode('binary')"
            :disabled="isWindowsClient"
            class="mode-button"
          >
            {{ isZh ? 'Binary(Linux二进制执行)' : 'Binary (Linux binary exec)' }}
          </el-button>
          <el-button
            :type="executionMode === 'bof' ? 'primary' : 'default'"
            @click="setExecutionMode('bof')"
            :disabled="!isWindowsClient"
            class="mode-button"
          >
            {{ isZh ? 'BOF(COFF内存执行)' : 'BOF (COFF in-memory)' }}
          </el-button>
          <el-button
            :type="executionMode === 'exec-file' ? 'primary' : 'default'"
            @click="setExecutionMode('exec-file')"
            class="mode-button"
          >
            {{ isZh ? 'Exec File(临时文件执行,支持Go/Rust PE)' : 'Exec File (temp file exec, Go/Rust PE)' }}
          </el-button>
        </div>
        <el-alert v-if="!isWindowsClient && pluginForm.clientUid" type="info" :closable="false" show-icon style="margin-top: 8px;">
          {{ isZh ? 'Linux 客户端：使用 Script 或 Binary 模式执行插件' : 'Linux client: use Script or Binary mode' }}
        </el-alert>
        <el-alert v-if="isWindowsClient && pluginForm.clientUid" type="warning" :closable="false" show-icon style="margin-top: 8px;">
          {{ isZh ? 'Windows 客户端：使用 Execute Assembly / Inline Bin / Shellcode / BOF 模式' : 'Windows client: use Execute Assembly / Inline Bin / Shellcode / BOF' }}
        </el-alert>
        <el-alert v-if="!pluginForm.clientUid" type="info" :closable="false" show-icon style="margin-top: 8px;">
          {{ isZh ? '请先选择客户端' : 'Select a client first' }}
        </el-alert>
      </el-form-item>

      <el-form-item :label="isZh ? '选择插件' : 'Select Plugin'" required>
        <div class="plugin-select-row">
          <el-select v-model="pluginForm.selectedPlugin" :placeholder="isZh ? '请选择插件' : 'Select a plugin'" class="full-width" @change="handlePluginChange">
            <el-option
              v-for="plugin in plugins"
              :key="plugin.name || plugin.Name"
              :label="plugin.name || plugin.Name"
              :value="plugin.name || plugin.Name"
            />
          </el-select>
          <el-button
            v-if="pluginForm.selectedPlugin"
            type="danger"
            :icon="Delete"
            @click="handleDeletePlugin"
          >{{ isZh ? '删除插件' : 'Delete Plugin' }}</el-button>
        </div>
      </el-form-item>

      <!-- 插件详情（后端 /plugin/details/:pluginName，1:1 CS 信息条） -->
      <el-form-item v-if="pluginForm.selectedPlugin" :label="isZh ? '插件详情' : 'Plugin Details'">
        <div class="plugin-detail-strip cs-font-ui">
          <div v-if="pluginDetailsLoading" class="pd-loading">
            {{ isZh ? '加载详情中...' : 'Loading plugin details...' }}
          </div>
          <template v-else-if="pluginDetails">
            <div class="pd-cell">
              <span class="pd-label">{{ isZh ? '名称' : 'Name' }}</span>
              <span class="pd-value mono">{{ pluginDetails.name || pluginForm.selectedPlugin }}</span>
            </div>
            <div class="pd-cell">
              <span class="pd-label">{{ isZh ? '类型' : 'Type' }}</span>
              <span class="pd-value">{{ pluginDetails.type || pluginDetails.Type || '-' }}</span>
            </div>
            <div class="pd-cell">
              <span class="pd-label">{{ isZh ? '大小' : 'Size' }}</span>
              <span class="pd-value mono">{{ formatFileSize(pluginDetails.size ?? pluginDetails.Size ?? 0) }}</span>
            </div>
            <div class="pd-cell">
              <span class="pd-label">{{ isZh ? '修改时间' : 'Modified' }}</span>
              <span class="pd-value mono">{{ pluginDetails.last_modified || pluginDetails.LastModified || '-' }}</span>
            </div>
            <div class="pd-cell">
              <span class="pd-label">{{ isZh ? '描述' : 'Description' }}</span>
              <span class="pd-value">{{ pluginDetails.description || pluginDetails.Description || '-' }}</span>
            </div>
            <button class="cs-btn cs-btn-xs" @click="downloadPluginFile">
              {{ isZh ? '下载' : 'Download' }}
            </button>
          </template>
          <div v-else class="pd-empty">{{ isZh ? '未能获取插件详情' : 'Plugin details unavailable' }}</div>
        </div>
      </el-form-item>

      <el-form-item :label="isZh ? '参数' : 'Arguments'">
        <el-input
          v-model="pluginForm.arguments"
          :placeholder="isZh ? '请输入参数，例如：-h 192.168.0.0' : 'Enter arguments, e.g. -h 192.168.0.0'"
          class="full-width"
        />
      </el-form-item>

      <!-- 插件输出区域 -->
      <div class="output-section">
        <div class="output-header">
          <h4 class="output-title">📜 {{ isZh ? '执行输出' : 'Execution Output' }}</h4>
          <div class="output-controls">
            <el-button-group>
              <el-button @click="clearOutput" icon="Delete" :title="isZh ? '清空输出' : 'Clear Output'">{{ isZh ? '清空' : 'Clear' }}</el-button>
              <el-button @click="copyOutput" icon="CopyDocument" :title="isZh ? '复制全部' : 'Copy All'">{{ isZh ? '复制' : 'Copy' }}</el-button>
              <el-button @click="exportOutput" icon="Download" :title="isZh ? '导出日志' : 'Export Log'">{{ isZh ? '导出' : 'Export' }}</el-button>
              <el-button @click="refreshShellContent" icon="Refresh" :title="isZh ? '手动刷新' : 'Manual Refresh'" :disabled="!pluginForm.clientUid">{{ isZh ? '刷新' : 'Refresh' }}</el-button>
            </el-button-group>

            <button
              class="cs-btn cs-btn-primary"
              @click="executePlugin"
              :disabled="!pluginForm.clientUid || !pluginForm.selectedPlugin || !executionMode || pollingInterval !== null"
            >
              <span v-if="pollingInterval">{{ isZh ? '执行中...' : 'Executing...' }}</span>
              <span v-else>{{ isZh ? '执行' : 'Execute' }} ({{ getExecutionModeText() }})</span>
            </button>

            <button
              v-if="pollingInterval"
              class="cs-btn cs-btn-danger"
              @click="stopPolling"
            >
              {{ isZh ? '停止轮询' : 'Stop Polling' }}
            </button>
          </div>
        </div>
        <div class="output-wrapper">
          <textarea
            ref="outputTextarea"
            v-model="pluginOutput"
            placeholder="output"
            class="output-textarea"
            readonly
            :style="userResized ? { height: outputHeight + 'px', width: outputWidth + 'px' } : {}"
          />
          <!-- 调整大小的拖拽手柄 -->
          <div
            class="resize-handle-corner"
            @mousedown="startResize"
          />
          <div
            class="resize-handle-right"
            @mousedown="startResizeRight"
          />
          <div
            class="resize-handle-bottom"
            @mousedown="startResizeBottom"
          />
        </div>
      </div>
    </el-form>
    </div>
    
    <!-- 上传文件模态框 -->
    <el-dialog v-model="showUploadDialog" :title="isZh ? '上传插件' : 'Upload Plugin'" width="700px" :close-on-click-modal="false">
      <div class="upload-dialog-content">
        <div class="upload-info-banner">
          <el-icon class="info-icon"><InfoFilled /></el-icon>
          {{ isZh ? '支持exe,net,elf,dll,so,dylib格式，单个文件不超过50MB，最多只能同时上传6个文件' : 'Supports exe, net, elf, dll, so, dylib. Max 50MB per file, up to 6 files at once.' }}
          <el-button type="primary" class="select-file-btn" @click="selectFiles">{{ isZh ? '选择文件' : 'Select Files' }}</el-button>
        </div>

        <!-- 文件列表区域 -->
        <div class="file-list-area">
          <div class="file-list-header">
            <div class="header-col thumbnail">{{ isZh ? '缩略图' : 'Preview' }}</div>
            <div class="header-col filename">{{ isZh ? '文件名' : 'File Name' }}</div>
            <div class="header-col filesize">{{ isZh ? '文件大小' : 'Size' }}</div>
            <div class="header-col status">{{ isZh ? '状态' : 'Status' }}</div>
            <div class="header-col operation">{{ isZh ? '操作' : 'Actions' }}</div>
          </div>

          <div class="file-list-body">
            <div v-if="fileList.length === 0" class="empty-file-list">
              {{ isZh ? '暂无文件' : 'No files' }}
            </div>
            <div v-for="(file, index) in fileList" :key="index" class="file-item">
              <div class="file-col thumbnail">
                <div class="file-icon">📄</div>
              </div>
              <div class="file-col filename">{{ file.name }}</div>
              <div class="file-col filesize">{{ formatFileSize(file.size) }}</div>
              <div class="file-col status">{{ isZh ? '待上传' : 'Pending' }}</div>
              <div class="file-col operation">
                <el-button type="text" @click="removeFile(index)">{{ isZh ? '删除' : 'Remove' }}</el-button>
              </div>
            </div>
          </div>
        </div>
        
        <!-- 隐藏的文件输入 -->
        <input
          ref="fileInput"
          type="file"
          multiple
          accept=".exe,.net,.elf,.dll,.so,.dylib"
          style="display: none"
          @change="handleFileSelect"
        />
      </div>
      
      <template #footer>
        <div class="upload-dialog-footer">
          <el-button @click="closeUploadDialog">{{ isZh ? '关闭' : 'Close' }}</el-button>
          <el-button type="primary" @click="submitUpload">{{ isZh ? '开始上传' : 'Start Upload' }}</el-button>
        </div>
      </template>
    </el-dialog>
  </template>
  <ScriptManagerPanel v-else />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed, onUnmounted, defineAsyncComponent } from 'vue';
const ScriptManagerPanel = defineAsyncComponent(() => import('@/layout/ScriptManagerPanel.vue'));
import { useI18nStore } from '@/stores/i18n';
import { ElMessage, ElMessageBox } from 'element-plus';
import { InfoFilled, UploadFilled, Delete, CopyDocument, Download, Refresh } from '@element-plus/icons-vue';
import pluginApi from '@/api/plugin';
import type { ClientInfo, PluginInfo } from '@/api/plugin';
import ClientAPI from '@/api/clients';
import type { ClientData } from '@/stores/clients';
import { smartDecodeContent } from '@/utils/encoding';
import PageHeader from '@/components/Common/PageHeader.vue';

// 用户状态

// 表单数据
const i18nStore = useI18nStore();
const view = ref<'plugins' | 'scripts'>('plugins')
const isZh = computed(() => i18nStore.isZh);

const pluginForm = ref({
  clientUid: '',
  selectedPlugin: '',
  arguments: ''
});

// 执行模式
const executionMode = ref('');

// 插件列表
const plugins = ref<PluginInfo[]>([]);

// 插件详情（后端 /plugin/details/:pluginName）
const pluginDetails = ref<any>(null);
const pluginDetailsLoading = ref(false);

// 插件输出
const pluginOutput = ref('');

// 在线客户端
const onlineClients = ref<ClientData[]>([]);

// 跨平台防护：检查选中客户端是否为Windows（execute-assembly/inline-bin/shellcode-inject 仅支持Windows）
const isWindowsClient = computed(() => {
  if (!pluginForm.value.clientUid) return true; // 未选客户端时不禁用
  const client = onlineClients.value.find((c: ClientData) => c.Uid === pluginForm.value.clientUid);
  if (!client) return true;
  const arch = (client.Arch || '').toLowerCase();
  const computer = (client.Computer || '').toLowerCase();
  // 明确是 Linux/Darwin 时返回 false，否则默认 Windows
  return !arch.includes('linux') && !arch.includes('darwin') && !computer.includes('linux');
});

// 输出框尺寸控制
const outputHeight = ref(400);
const outputWidth = ref(800);
// 默认由 CSS 撑满容器（自适应）；用户拖拽过手柄后才固定为像素尺寸
const userResized = ref(false);
const outputTextarea = ref<HTMLTextAreaElement>();

// 拖拽调整大小相关
let isResizing = ref(false);
let resizeType = ref('');
let startX = 0;
let startY = 0;
let startWidth = 0;
let startHeight = 0;

// 上传对话框相关
const showUploadDialog = ref(false);
const fileList = ref<File[]>([]);
const fileInput = ref();

// 在组件挂载时加载数据
onMounted(async () => {
  await refreshPlugins();
  await loadOnlineClients();
  
  // 添加全局鼠标事件监听
  document.addEventListener('mousemove', handleMouseMove);
  document.addEventListener('mouseup', handleMouseUp);
});

// 组件卸载时清理事件监听
onUnmounted(() => {
  document.removeEventListener('mousemove', handleMouseMove);
  document.removeEventListener('mouseup', handleMouseUp);
  
  // 清理轮询计时器和状态
  if (pollingInterval) {
    clearInterval(pollingInterval);
    pollingInterval = null;
  }
  lastShellContentLength = 0;
  contentStableCount = 0;
  hasSeenCompletionMarker = false;
});

// 拖拽调整大小功能
const startResize = (e: MouseEvent) => {
  e.preventDefault();
  isResizing.value = true;
  resizeType.value = 'both';
  startX = e.clientX;
  startY = e.clientY;
  const el = outputTextarea.value;
  if (el) {
    // 自适应状态下以当前渲染尺寸作为拖拽起点，避免从 800x400 跳变
    outputWidth.value = Math.round(el.getBoundingClientRect().width);
    outputHeight.value = Math.round(el.getBoundingClientRect().height);
  }
  userResized.value = true;
  startWidth = outputWidth.value;
  startHeight = outputHeight.value;
};

const startResizeRight = (e: MouseEvent) => {
  e.preventDefault();
  isResizing.value = true;
  resizeType.value = 'width';
  startX = e.clientX;
  const el = outputTextarea.value;
  if (el) outputWidth.value = Math.round(el.getBoundingClientRect().width);
  userResized.value = true;
  startWidth = outputWidth.value;
};

const startResizeBottom = (e: MouseEvent) => {
  e.preventDefault();
  isResizing.value = true;
  resizeType.value = 'height';
  startY = e.clientY;
  const el = outputTextarea.value;
  if (el) outputHeight.value = Math.round(el.getBoundingClientRect().height);
  userResized.value = true;
  startHeight = outputHeight.value;
};

const handleMouseMove = (e: MouseEvent) => {
  if (!isResizing.value) return;
  
  if (resizeType.value === 'both' || resizeType.value === 'width') {
    const deltaX = e.clientX - startX;
    outputWidth.value = Math.max(400, startWidth + deltaX);
  }
  
  if (resizeType.value === 'both' || resizeType.value === 'height') {
    const deltaY = e.clientY - startY;
    outputHeight.value = Math.max(200, startHeight + deltaY);
  }
};

const handleMouseUp = () => {
  isResizing.value = false;
  resizeType.value = '';
};

// 设置执行模式
const setExecutionMode = (mode: string) => {
  executionMode.value = mode;
};

// 获取执行模式文本
const getExecutionModeText = () => {
  switch (executionMode.value) {
    case 'execute-assembly':
      return 'Execute Assembly';
    case 'inline-bin':
      return 'Inline Bin';
    case 'shellcode-inject':
      return isZh.value ? 'Shellcode注入' : 'Shellcode';
    case 'script':
      return 'Linux Script';
    case 'binary':
      return 'Linux Binary';
    case 'bof':
      return 'BOF(COFF)';
    case 'exec-file':
      return 'Exec File';
    default:
      return isZh.value ? '请选择模式' : 'Select Mode';
  }
};

// 获取客户端显示信息（与隧道代理一致）
const getClientDisplayInfo = (client: ClientData) => {
  const protocol = client.Listener || 'tcp'
  
  // 使用数据库自增ID而不是UID的前8位
  const clientId = client.ID || client.id || 'N/A'
  
  // 获取系统类型（采用客户端管理页面的成功做法）
  const getSystemType = (client: ClientData) => {
    let os = 'unknown'
    let arch = client.Arch || 'unknown'
    
    // 根据Process字段判断操作系统
    if (client.Process && client.Process.includes('.exe')) {
      os = 'windows'
    } else if (client.Computer && client.Computer.toLowerCase().includes('linux')) {
      os = 'linux'
    } else if (client.Computer && client.Computer.toLowerCase().includes('darwin')) {
      os = 'darwin'
    } else {
      // 默认根据架构推断
      os = 'windows' // 大部分客户端是Windows
    }
    
    return `${os}_${arch}`
  }
  
  const osType = getSystemType(client)
  
  // 格式: ID：X | 协议 | 外网IP | 系统类型_架构 | 计算机名
  return isZh.value
    ? `ID：${clientId} | ${protocol.toUpperCase()} | ${client.ExternalIP} | ${osType} | ${client.Computer}`
    : `ID: ${clientId} | ${protocol.toUpperCase()} | ${client.ExternalIP} | ${osType} | ${client.Computer}`
}

// 加载在线客户端：优先走后端专用 /client/online-clients（plugin.go），失败回退会话列表
const loadOnlineClients = async () => {
  try {
    const response = await pluginApi.get_online_clients()
    if (response.data && response.data.status === 200 && Array.isArray(response.data.data)) {
      const clients: ClientData[] = response.data.data
        .filter((c: ClientInfo) => (c.online ?? c.Online) === '1' || String(c.online ?? c.Online) === '1')
        .map((c: ClientInfo) => ({
          Uid: c.uid || c.Uid || '',
          Computer: c.computer || c.Computer || 'unknown',
          ExternalIP: (c as any).external_ip || (c as any).ExternalIP || '-',
          Username: c.username || c.Username || 'user',
          Listener: (c as any).listener || (c as any).Listener || 'tcp',
          Online: '1',
        } as ClientData))

      if (clients.length) {
        onlineClients.value = clients
        console.log('✅ 专用 /client/online-clients 加载成功:', clients.length, '个')
        return
      }
    }
    // 专用端点无数据或契约异常时回退 clientslist（保留旧路径）
    const fallback = await ClientAPI.get_clients_list({ page: 1, page_size: 1000 })
    const clientList = fallback.data?.data?.list || []
    onlineClients.value = clientList.filter((c: ClientData) => c.Online === '1' || String(c.Online) === '1')
    if (!onlineClients.value.length) {
      console.warn('在线客户端为空')
    }
  } catch (error) {
    console.error('获取在线客户端失败:', error)
    ElMessage.error(isZh.value ? '获取客户端列表失败' : 'Failed to load client list')
  }
};

// 刷新插件列表
const refreshPlugins = async () => {
  try {
    const response = await pluginApi.get_plugins();
    console.log('🔍 插件API响应:', response);
    
    if (response.data && response.data.status === 200) {
      plugins.value = response.data.data || [];
      console.log('✅ 加载插件成功:', plugins.value.length, '个插件');
      if (plugins.value.length > 0) {
        console.log('📄 插件列表:', plugins.value.map(p => p.Name));
      } else {
        console.log('⚠️ 插件目录为空或没有找到插件文件');
      }
    } else {
      console.error('❌ 插件API返回异常:', response.data);
      plugins.value = [];
      ElMessage.error(isZh.value ? '插件列表加载失败' : 'Failed to load plugin list');
    }
  } catch (error) {
    console.error('❌ 刷新插件列表失败:', error);
    plugins.value = [];
    ElMessage.error(isZh.value ? '刷新插件列表失败' : 'Failed to refresh plugin list');
  }
};

// 当插件选择改变时加载详细信息（后端 /plugin/details/:pluginName）
const handlePluginChange = async () => {
  pluginDetails.value = null
  const name = pluginForm.value.selectedPlugin
  if (!name) return
  pluginDetailsLoading.value = true
  try {
    const response = await pluginApi.get_plugin_details(name)
    if (response.data && response.data.status === 200) {
      pluginDetails.value = response.data.data || null
    } else {
      pluginDetails.value = null
      ElMessage.error(response.data?.message || (isZh.value ? '获取插件详情失败' : 'Failed to load plugin details'))
    }
  } catch {
    pluginDetails.value = null
    ElMessage.error(isZh.value ? '获取插件详情失败' : 'Failed to load plugin details')
  } finally {
    pluginDetailsLoading.value = false
  }
};

// 下载插件文件（后端 /plugin/download/:pluginName，SHA256 完整性校验）
const downloadPluginFile = async () => {
  const name = pluginForm.value.selectedPlugin
  if (!name) return
  try {
    const res = await pluginApi.download_plugin(name)
    const blob = res.data instanceof Blob ? res.data : new Blob([res.data], { type: 'application/octet-stream' })
    const a = document.createElement('a')
    const url = URL.createObjectURL(blob)
    a.href = url
    a.download = name
    a.click()
    URL.revokeObjectURL(url)
    ElMessage.success(isZh.value ? `插件 ${name} 已下载` : `Plugin ${name} downloaded`)
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '下载插件失败' : 'Failed to download plugin'))
  }
};

// 删除插件文件
const handleDeletePlugin = () => {
  const name = pluginForm.value.selectedPlugin
  if (!name) return
  ElMessageBox.confirm(
    isZh.value ? `确定要删除插件 "${name}" 吗？此操作不可恢复。` : `Delete plugin "${name}"? This cannot be undone.`,
    isZh.value ? '删除插件确认' : 'Delete Plugin',
    { confirmButtonText: isZh.value ? '确定' : 'Confirm', cancelButtonText: isZh.value ? '取消' : 'Cancel', type: 'error' }
  ).then(async () => {
    try {
      const response = await pluginApi.delete_plugin(name)
      if (response.data && response.data.status === 200) {
        ElMessage.success(isZh.value ? '插件已删除' : 'Plugin deleted')
        pluginForm.value.selectedPlugin = ''
        pluginDetails.value = null
        await refreshPlugins()
      } else {
        ElMessage.error(response.data?.data || response.data?.message || (isZh.value ? '删除插件失败' : 'Failed to delete plugin'))
      }
    } catch (error: any) {
      ElMessage.error(error.response?.data?.data || (isZh.value ? '删除插件失败' : 'Failed to delete plugin'))
    }
  })
};

// 停止轮询
const stopPolling = () => {
  if (pollingInterval) {
    clearInterval(pollingInterval);
    pollingInterval = null;
    lastShellContentLength = 0;
    contentStableCount = 0;
    hasSeenCompletionMarker = false;
    
    pluginOutput.value += isZh.value
      ? '\n\n[⏹️] 用户手动停止轮询\n提示：您可以稍后手动查看Shell历史记录来确认执行结果'
      : '\n\n[⏹️] Polling stopped by user\nTip: check the shell history later to confirm the result';
    
    ElMessage.info(isZh.value ? '已停止轮询获取执行结果' : 'Stopped polling for results');
  }
};

// 清空输出
const clearOutput = () => {
  pluginOutput.value = '';
  ElMessage.success(isZh.value ? '输出已清空' : 'Output cleared');
};

// 复制输出内容
const copyOutput = async () => {
  if (!pluginOutput.value) {
    ElMessage.warning(isZh.value ? '没有内容可复制' : 'Nothing to copy');
    return;
  }
  
  try {
    await navigator.clipboard.writeText(pluginOutput.value);
    ElMessage.success(isZh.value ? '已复制全部输出内容' : 'Copied all output');
  } catch (error) {
    // 退回方案：传统复制方法
    const textArea = document.createElement('textarea');
    textArea.value = pluginOutput.value;
    document.body.appendChild(textArea);
    textArea.select();
    document.execCommand('copy');
    document.body.removeChild(textArea);
    ElMessage.success(isZh.value ? '已复制全部输出内容' : 'Copied all output');
  }
};

// 导出日志
const exportOutput = () => {
  if (!pluginOutput.value) {
    ElMessage.warning(isZh.value ? '没有内容可导出' : 'Nothing to export');
    return;
  }
  
  const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
  const pluginName = pluginForm.value.selectedPlugin || 'unknown';
  const filename = `plugin-output-${pluginName}-${timestamp}.txt`;
  
  const blob = new Blob([pluginOutput.value], { type: 'text/plain;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
  
  ElMessage.success(isZh.value ? '日志已导出' : 'Log exported');
};

// 手动刷新Shell内容
const refreshShellContent = async () => {
  if (!pluginForm.value.clientUid) {
    ElMessage.warning(isZh.value ? '请先选择客户端' : 'Select a client first');
    return;
  }
  
  try {
    const response = await pluginApi.get_shell_history(pluginForm.value.clientUid);
    if (response.data && response.data.status === 200) {
      // 编码修复：Windows GBK 输出转 UTF-8
      const fullShellContent = smartDecodeContent(response.data.data || '');
      
      // 调试信息：显示原始内容的关键统计
      console.log('🔍 手动刷新 - 原始内容统计:', {
        '总长度': fullShellContent.length,
        '总行数': fullShellContent.split('\n').length,
        '包含mimikatz': fullShellContent.toLowerCase().includes('mimikatz'),
        '包含Authentication': fullShellContent.includes('Authentication Id'),
        '包含CAPTURED_OUTPUT': fullShellContent.includes('=== CAPTURED OUTPUT ==='),
        '包含FINAL_OUTPUT': fullShellContent.includes('=== FINAL OUTPUT DUMP ===')
      });
      
      // 检查是否是 mimikatz 相关工具
      const isMimikatzTool = pluginForm.value.selectedPlugin.toLowerCase().includes('mimikatz') || 
                            pluginForm.value.selectedPlugin.toLowerCase().includes('sekur') ||
                            pluginForm.value.selectedPlugin.toLowerCase().includes('creds');
      
      // 如果有选中的执行模式和插件，则过滤内容
      let shellContent = fullShellContent;
      if (executionMode.value && pluginForm.value.selectedPlugin) {
        if (isMimikatzTool) {
          shellContent = filterShellContentForMimikatz(fullShellContent, executionMode.value, pluginForm.value.selectedPlugin);
          console.log('🔍 使用 mimikatz 专用过滤器');
        } else {
          shellContent = filterShellContentByMode(fullShellContent, executionMode.value, pluginForm.value.selectedPlugin);
          console.log('🔍 使用标准过滤器');
        }
      }
      
      // 显示结果和调试信息
      const yesNo = (b: boolean) => (b ? (isZh.value ? '是' : 'yes') : (isZh.value ? '否' : 'no'));
      const mimikatzModeText = isMimikatzTool
        ? (isZh.value ? 'Mimikatz专用模式（宽松过滤）' : 'Mimikatz mode (loose filter)')
        : (isZh.value ? '标准过滤模式' : 'Standard filter');
      const displayModeLine = executionMode.value
        ? (isZh.value ? `显示模式: ${mimikatzModeText}` : `Display mode: ${mimikatzModeText}`)
        : '';
      const hasCreds = fullShellContent.includes('Authentication Id');
      const hasMarkers = fullShellContent.includes('=== CAPTURED OUTPUT ===') || fullShellContent.includes('=== FINAL OUTPUT DUMP ===');
      pluginOutput.value = isZh.value
        ? `[🔄] 手动刷新Shell历史
时间: ${new Date().toLocaleString()}
客户端: ${pluginForm.value.clientUid}
${displayModeLine}

=== 调试信息 ===
原始内容长度: ${fullShellContent.length} 字符
过滤后长度: ${shellContent.length} 字符
包含 mimikatz 关键字: ${yesNo(fullShellContent.toLowerCase().includes('mimikatz'))}
包含凭据信息: ${yesNo(hasCreds)}
包含输出标记: ${yesNo(hasMarkers)}

=== 最新执行结果 ===
${shellContent}

=== 调试提示 ===
如果上面的结果不完整，请检查：
1. 客户端日志是否显示完整输出
2. 是否选择了正确的执行模式
3. 插件名称是否匹配`
        : `[🔄] Shell History Refreshed
Time: ${new Date().toLocaleString()}
Client: ${pluginForm.value.clientUid}
${displayModeLine}

=== Debug Info ===
Raw length: ${fullShellContent.length} chars
Filtered length: ${shellContent.length} chars
Contains mimikatz keyword: ${yesNo(fullShellContent.toLowerCase().includes('mimikatz'))}
Contains credential info: ${yesNo(hasCreds)}
Contains output markers: ${yesNo(hasMarkers)}

=== Latest Execution Result ===
${shellContent}

=== Debug Tips ===
If the result above is incomplete, check:
1. Whether the client log shows the complete output
2. Whether the correct execution mode is selected
3. Whether the plugin name matches`;
      ElMessage.success(isZh.value ? '已更新Shell历史内容（包含调试信息）' : 'Shell history updated (with debug info)');
    } else {
      ElMessage.error(isZh.value ? '获取Shell历史失败' : 'Failed to load shell history');
    }
  } catch (error) {
    console.error('刷新Shell内容失败:', error);
    ElMessage.error(isZh.value ? '刷新失败：' + (error instanceof Error ? error.message : '网络错误') : 'Refresh failed: ' + (error instanceof Error ? error.message : 'network error'));
  }
};

// 执行插件
const executePlugin = async () => {
  if (!pluginForm.value.clientUid || !pluginForm.value.selectedPlugin || !executionMode.value) {
    ElMessage.warning(isZh.value ? '请选择客户端、插件和执行模式' : 'Select a client, plugin and execution mode');
    return;
  }

  // 检查客户端是否在线
  const selectedClient = onlineClients.value.find(client => client.Uid === pluginForm.value.clientUid);
  if (!selectedClient || selectedClient.Online !== '1') {
    ElMessage.error(isZh.value ? '所选客户端不在线，请选择其他客户端' : 'Selected client is offline, pick another one');
    return;
  }

  try {
    // 显示开始执行的信息
    const pluginName = pluginForm.value.selectedPlugin;
    const clientUid = pluginForm.value.clientUid;
    const args = pluginForm.value.arguments || '';
    
    // 清空之前的输出并显示执行信息
    const pluginArgs = args || (isZh.value ? '无' : 'none');
    pluginOutput.value = isZh.value
      ? `[🔄] 开始执行插件
===================
模式: ${getExecutionModeText()}
插件: ${pluginName}
参数: ${pluginArgs}
客户端: ${selectedClient.Computer} (${selectedClient.ExternalIP})
执行时间: ${new Date().toLocaleTimeString()}
显示模式: 仅显示最新一次 ${executionMode.value} 执行结果
===================

正在下载插件文件...`
      : `[🔄] Plugin Execution Started
===================
Mode: ${getExecutionModeText()}
Plugin: ${pluginName}
Arguments: ${pluginArgs}
Client: ${selectedClient.Computer} (${selectedClient.ExternalIP})
Started at: ${new Date().toLocaleTimeString()}
Display mode: latest ${executionMode.value} run only
===================

Downloading plugin file...`;

    pluginOutput.value += isZh.value ? '\n正在准备执行文件...' : '\nPreparing execution file...';

    // 走统一封装：内部对文件名做 encodeURIComponent（原裸 fetch 直接拼路径，
    // 文件名含空格/括号时会拼出非法 URL），并复用统一的错误处理
    const pluginDownloadRes = await pluginApi.download_plugin(pluginName);
    const pluginBlob = pluginDownloadRes.data instanceof Blob
      ? pluginDownloadRes.data
      : new Blob([pluginDownloadRes.data], { type: 'application/octet-stream' });
    const pluginFile = new File([pluginBlob], pluginName);
    
    pluginOutput.value += isZh.value ? '\n正在发送命令到客户端...' : '\nSending command to client...';
    
    // 走统一封装（api/clients::execute_bin），不再手拼 fetch 与 Authorization2。
    // 差异：非 200 会被请求层 reject（带后端文案），由下方 catch 统一展示。
    const response = await ClientAPI.execute_bin(clientUid, pluginFile, args, executionMode.value);
    const result = response?.data;
    
    if (result.status === 200) {
      pluginOutput.value += isZh.value
        ? '\n命令已成功发送到客户端\n\n正在等待执行结果...'
        : '\nCommand sent to client\n\nWaiting for results...';
      ElMessage.success(isZh.value ? '插件命令已发送，正在等待执行结果' : 'Command sent, waiting for results');
      
      // 重置轮询状态
      lastShellContentLength = 0;
      
      // 开始轮询获取执行结果
      startPollingForResults(clientUid, pluginName);
    } else {
      const errorMsg = result.data || result.message || (isZh.value ? '未知错误' : 'Unknown error');
      pluginOutput.value = isZh.value
        ? `[✗] 执行失败
错误原因: ${errorMsg}

请检查：
1. 客户端是否正常连接
2. 插件文件格式是否正确
3. 执行模式是否与插件类型匹配`
        : `[✗] Execution Failed
Reason: ${errorMsg}

Check:
1. Whether the client is connected
2. Whether the plugin file format is correct
3. Whether the execution mode matches the plugin type`;
      ElMessage.error((isZh.value ? '插件执行失败: ' : 'Plugin execution failed: ') + errorMsg);
    }
  } catch (error: unknown) {
    let errorMessage = isZh.value ? '未知错误' : 'Unknown error';
    
    if (error instanceof Error) {
      errorMessage = error.message;
    } else if (typeof error === 'string') {
      errorMessage = error;
    }
    
    pluginOutput.value = isZh.value
      ? `[✗] 执行失败
错误信息: ${errorMessage}

常见问题解决：
1. 检查网络连接是否正常
2. 确认身份验证未过期
3. 刷新页面后重试
4. 联系管理员获取帮助`
      : `[✗] Execution Failed
Error: ${errorMessage}

Common fixes:
1. Check the network connection
2. Make sure the session has not expired
3. Refresh the page and retry
4. Contact an administrator`;
    
    ElMessage.error((isZh.value ? '执行插件时发生错误: ' : 'Plugin execution error: ') + errorMessage);
    console.error('执行插件时发生错误:', error);
  }
};

// 过滤Shell内容，只保留最新一次执行的相关输出
const filterShellContentByMode = (fullContent: string, currentMode: string, pluginName: string) => {
  if (!fullContent || !currentMode) return fullContent;
  
  const lines = fullContent.split('\n');
  const filteredLines: string[] = [];
  
  // 执行模式列表
  const executionModes = ['execute-assembly', 'inline-bin', 'shellcode-inject', 'script', 'binary', 'bof', 'exec-file'];
  
  // 找到最后一次当前模式的执行命令位置
  let lastCommandIndex = -1;
  for (let i = lines.length - 1; i >= 0; i--) {
    const line = lines[i];
    if (line.startsWith('$ ') && line.includes(currentMode) && line.includes(pluginName)) {
      lastCommandIndex = i;
      break;
    }
  }
  
  // 如果没找到当前模式的命令，返回提示信息
  if (lastCommandIndex === -1) {
    return isZh.value
      ? '暂无当前执行模式的相关输出。\n\n请执行插件后查看结果。'
      : 'No output for the current execution mode.\n\nRun a plugin and check the result.';
  }
  
  // 从最后一次命令开始，收集到下一个执行模式命令之前的所有内容
  let endIndex = lines.length;
  for (let i = lastCommandIndex + 1; i < lines.length; i++) {
    const line = lines[i];
    if (line.startsWith('$ ') && executionModes.some(mode => line.includes(mode))) {
      endIndex = i;
      break;
    }
  }
  
  // 提取最新执行的内容
  const latestExecutionLines = lines.slice(lastCommandIndex, endIndex);
  
  // 特殊处理：确保包含重要的输出内容（如 mimikatz 结果）
  let finalContent = latestExecutionLines.join('\n');
  
  // 如果过滤后的内容很少，可能丢失了重要信息，尝试扩展范围
  if (finalContent.length < 500 && pluginName.toLowerCase().includes('mimikatz')) {
    console.log('🔍 检测到 mimikatz，扩展输出范围以包含更多内容');
    
    // 向前扩展搜索范围，寻找可能的输出内容
    let expandedStartIndex = Math.max(0, lastCommandIndex - 50);
    let expandedLines = lines.slice(expandedStartIndex, endIndex);
    
    // 查找包含凭据信息的行
    const credentialKeywords = [
      'Authentication Id', 'Session', 'Username', 'Domain', 
      'NTLM', 'SHA1', 'Password', 'Kerberos', 'Primary',
      'msv', 'tspkg', 'wdigest', 'kerberos', 'ssp', 'credman',
      '=== CAPTURED OUTPUT ===', '=== FINAL OUTPUT DUMP ==='
    ];
    
    for (let line of expandedLines) {
      if (credentialKeywords.some(keyword => line.toLowerCase().includes(keyword.toLowerCase()))) {
        console.log('🎯 发现凭据相关内容，使用扩展范围');
        finalContent = expandedLines.join('\n');
        break;
      }
    }
  }
  
  console.log('🔍 最新执行内容过滤结果:', {
    '原始总行数': lines.length,
    '最后命令位置': lastCommandIndex,
    '结束位置': endIndex,
    '提取行数': latestExecutionLines.length,
    '最终内容长度': finalContent.length,
    '执行模式': currentMode,
    '插件名': pluginName
  });
  
  return finalContent;
};

// 专门为 mimikatz 类工具设计的宽松过滤函数
const filterShellContentForMimikatz = (fullContent: string, currentMode: string, pluginName: string) => {
  if (!fullContent || !currentMode) return fullContent;
  
  const lines = fullContent.split('\n');
  
  // 执行模式列表
  const executionModes = ['execute-assembly', 'inline-bin', 'shellcode-inject', 'script', 'binary', 'bof', 'exec-file'];
  
  // 找到最后一次当前模式的执行命令位置
  let lastCommandIndex = -1;
  for (let i = lines.length - 1; i >= 0; i--) {
    const line = lines[i];
    if (line.startsWith('$ ') && line.includes(currentMode) && line.includes(pluginName)) {
      lastCommandIndex = i;
      break;
    }
  }
  
  // 如果没找到当前模式的命令，尝试找到任何 mimikatz 相关的执行
  if (lastCommandIndex === -1) {
    for (let i = lines.length - 1; i >= 0; i--) {
      const line = lines[i];
      if (line.startsWith('$ ') && 
          (line.toLowerCase().includes('mimikatz') || 
           line.toLowerCase().includes('sekur') ||
           line.includes(pluginName))) {
        lastCommandIndex = i;
        break;
      }
    }
  }
  
  if (lastCommandIndex === -1) {
    // 如果还是找不到，返回最近的一大段内容以防漏掉重要信息
    const recentLines = lines.slice(Math.max(0, lines.length - 200));
    const recentContent = recentLines.join('\n');
    
    // 检查最近内容是否包含凭据信息
    const credentialKeywords = [
      'Authentication Id', 'Session', 'Username', 'Domain', 
      'NTLM', 'SHA1', 'mimikatz', '=== CAPTURED OUTPUT ==='
    ];
    
    if (credentialKeywords.some(keyword => recentContent.toLowerCase().includes(keyword.toLowerCase()))) {
      console.log('🎯 在最近内容中发现可能的 mimikatz 输出');
      return recentContent;
    }
    
    return isZh.value
      ? '暂无执行相关输出，请检查执行是否成功。\n\n如果问题持续，请查看完整Shell历史。'
      : 'No execution output found, check whether the run succeeded.\n\nIf the problem persists, review the full shell history.';
  }
  
  // 使用更宽松的结束条件：寻找下一个明确的命令，而不是执行模式命令
  let endIndex = lines.length;
  for (let i = lastCommandIndex + 1; i < lines.length; i++) {
    const line = lines[i];
    // 只在遇到明确的新命令时才停止（而不是任何执行模式命令）
    if (line.startsWith('$ ') && 
        !line.includes('[CLIENT-PACKET]') && 
        !line.includes('完整数据包') &&
        executionModes.some(mode => line.includes(mode)) &&
        !line.includes(pluginName)) { // 排除同一个插件的重复执行
      endIndex = i;
      break;
    }
  }
  
  // 提取内容，但给 mimikatz 更大的范围
  let startIndex = Math.max(0, lastCommandIndex - 20); // 向前包含更多上下文
  const extractedLines = lines.slice(startIndex, endIndex);
  
  console.log('🔍 Mimikatz 专用过滤结果:', {
    '原始总行数': lines.length,
    '开始位置': startIndex,
    '最后命令位置': lastCommandIndex,
    '结束位置': endIndex,
    '提取行数': extractedLines.length,
    '执行模式': currentMode,
    '插件名': pluginName
  });
  
  return extractedLines.join('\n');
};
let pollingInterval: ReturnType<typeof setInterval> | null = null;
let lastShellContentLength = 0; // 记录上次Shell内容长度
let executionStartTime = 0; // 记录执行开始时间
let contentStableCount = 0; // 记录内容稳定的次数
let hasSeenCompletionMarker = false; // 记录是否已经看到完成标志

const startPollingForResults = (clientUid: string, pluginName: string) => {
  if (pollingInterval) {
    clearInterval(pollingInterval);
  }
  
  let attempts = 0;
  const maxAttempts = 30; // 增加到30秒超时，给mimikatz更多时间
  executionStartTime = Date.now();
  
  // 重置状态变量
  contentStableCount = 0;
  hasSeenCompletionMarker = false;
  
  // 检查是否是mimikatz或类似工具，使用更快的轮询间隔
  const isMimikatzTool = pluginName.toLowerCase().includes('mimikatz') || 
                        pluginName.toLowerCase().includes('sekur') ||
                        pluginName.toLowerCase().includes('creds');
  // 【优化】轮询统一 1000ms（原 mimikatz 500ms），配合 maxAttempts 超时保护，降低 API 频率
  const pollingIntervalMs = 1000;
  
  let pollingInFlight = false; // 防止并发轮询竞态
  pollingInterval = setInterval(async () => {
    attempts++;
    if (pollingInFlight) return; // 上一次轮询未完成，跳过
    pollingInFlight = true;

    try {
      const response = await pluginApi.get_shell_history(clientUid);
      if (response.data && response.data.status === 200) {
        const rawContent = response.data.data || '';
        // 编码修复：Windows GBK 输出转 UTF-8（"输出总是有问题"的根因）
        const fullShellContent = smartDecodeContent(rawContent);
        
        // 对于mimikatz工具，减少过滤，显示更多内容
        let shellContent;
        if (isMimikatzTool) {
          // mimikatz工具使用宽松的过滤策略
          shellContent = filterShellContentForMimikatz(fullShellContent, executionMode.value, pluginName);
        } else {
          // 其他工具使用标准过滤
          shellContent = filterShellContentByMode(fullShellContent, executionMode.value, pluginName);
        }
        
        // 改进的完成检测逻辑 - 遵循项目规范
        const completionMarkers = [
          '[+] Assembly execution completed successfully',
          'Assembly loaded, methodInfo:',
          'Stdout length:',
          'Assembly execution completed',
          'Assembly executed successfully',
          '[Windows内存执行]',
          '=== FINAL OUTPUT DUMP ===',
          '=== CAPTURED OUTPUT ===',
          '[+] Execution completed successfully',
          'InlineBin execution completed',
          // Linux 模式完成标志
          'Linux script executed',
          'Linux binary executed',
          'script error:',
          'binary error:',
          // BOF 完成标志
          'BOF executed',
          'BOF entry point',
          // exec-file 完成标志
          'file executed',
          'file exec error',
          // mimikatz特有的完成标志
          'Authentication Id',
          'NTLM',
          'SHA1'
        ];
        
        // 检测内容更新
        if (shellContent.length !== lastShellContentLength) {
          contentStableCount = 0;
          lastShellContentLength = shellContent.length;
          
          // 检查是否包含完成标志
          const currentlyHasMarker = completionMarkers.some(marker => 
            shellContent.toLowerCase().includes(marker.toLowerCase()));
          if (currentlyHasMarker && !hasSeenCompletionMarker) {
            hasSeenCompletionMarker = true;
            console.log('🎯 检测到完成标志，继续收集内容...');
          }
          
          // 实时更新显示
          if (shellContent.trim().length > 0) {
            const statusText = hasSeenCompletionMarker
              ? (isZh.value ? '收集输出中' : 'collecting output')
              : (isZh.value ? '正在执行' : 'running');
            pluginOutput.value = isZh.value
              ? `[⏳] 插件${statusText}...
模式: ${getExecutionModeText()}
插件: ${pluginName}
参数: ${pluginForm.value.arguments || '无'}
尝试次数: ${attempts}/${maxAttempts}
显示模式: ${isMimikatzTool ? '完整输出模式（mimikatz优化）' : '过滤输出模式'}

=== 实时执行结果 ===
${shellContent}

${statusText}中...`
              : `[⏳] Plugin ${statusText}...
Mode: ${getExecutionModeText()}
Plugin: ${pluginName}
Arguments: ${pluginForm.value.arguments || 'none'}
Attempt: ${attempts}/${maxAttempts}
Display mode: ${isMimikatzTool ? 'full output (mimikatz tuned)' : 'filtered output'}

=== Live Execution Result ===
${shellContent}

${statusText}...`;
          }
        } else {
          // 内容没有更新
          contentStableCount++;
          console.log(`📄 内容稳定计数: ${contentStableCount}, 已看到完成标志: ${hasSeenCompletionMarker}`);
          
          // 对于mimikatz工具，缩短稳定等待时间
          const requiredStableCount = isMimikatzTool ? 2 : 3;
          
          // 如果已经看到完成标志且内容稳定，则认为执行完成
          if (hasSeenCompletionMarker && contentStableCount >= requiredStableCount && shellContent.trim().length > 10) {
            console.log('🎉 内容已稳定，显示最终结果');
            
            const executionTime = Math.round((Date.now() - executionStartTime) / 1000);
            const selectedClient = onlineClients.value.find(client => client.Uid === clientUid);
            const clientId = selectedClient ? (selectedClient.ID || selectedClient.id || 'N/A') : 'N/A';
            
            // 显示最终结果
            pluginOutput.value = isZh.value
              ? `[✓] 插件执行完成
模式: ${getExecutionModeText()}
插件: ${pluginName}
参数: ${pluginForm.value.arguments || '无'}
客户端: ${clientId}
执行时间: ${executionTime}秒
显示模式: ${isMimikatzTool ? '完整输出模式（mimikatz优化）' : '过滤输出模式'}

=== 最终执行结果 ===
${shellContent}`
              : `[✓] Plugin Execution Finished
Mode: ${getExecutionModeText()}
Plugin: ${pluginName}
Arguments: ${pluginForm.value.arguments || 'none'}
Client: ${clientId}
Elapsed: ${executionTime}s
Display mode: ${isMimikatzTool ? 'full output (mimikatz tuned)' : 'filtered output'}

=== Final Execution Result ===
${shellContent}`;
            
            clearInterval(pollingInterval!);
            pollingInterval = null;
            lastShellContentLength = 0;
            contentStableCount = 0;
            hasSeenCompletionMarker = false;
            ElMessage.success(isZh.value ? '插件执行完成，完整结果已显示' : 'Plugin finished, full results shown');
            return;
          }
        }
        
        // 超时处理
        if (attempts >= maxAttempts) {
          const finalShellContent = shellContent || '';
          
          // 如果有内容且看到了完成标志，就直接显示结果
          if (hasSeenCompletionMarker && finalShellContent.trim().length > 10) {
            const executionTime = Math.round((Date.now() - executionStartTime) / 1000);
            const selectedClient = onlineClients.value.find(client => client.Uid === clientUid);
            const clientId = selectedClient ? (selectedClient.ID || selectedClient.id || 'N/A') : 'N/A';
            
            pluginOutput.value = isZh.value
              ? `[✓] 插件执行完成（超时显示）
模式: ${getExecutionModeText()}
插件: ${pluginName}
参数: ${pluginForm.value.arguments || '无'}
客户端: ${clientId}
执行时间: ${executionTime}秒
显示模式: ${isMimikatzTool ? '完整输出模式（mimikatz优化）' : '过滤输出模式'}

=== 最终执行结果 ===
${finalShellContent}

提示：由于达到${maxAttempts}秒超时限制，系统强制显示结果。`
              : `[✓] Plugin Execution Finished (shown on timeout)
Mode: ${getExecutionModeText()}
Plugin: ${pluginName}
Arguments: ${pluginForm.value.arguments || 'none'}
Client: ${clientId}
Elapsed: ${executionTime}s
Display mode: ${isMimikatzTool ? 'full output (mimikatz tuned)' : 'filtered output'}

=== Final Execution Result ===
${finalShellContent}

Note: the ${maxAttempts}s timeout was reached, so the result is shown as-is.`;
            ElMessage.success(isZh.value ? '插件执行结果已显示（超时显示）' : 'Results shown (timeout)');
          } else {
            // 显示调试信息帮助排查问题
            const recentLines = fullShellContent.split('\n').slice(-30).join('\n');
            pluginOutput.value += isZh.value
              ? `

[⚠️] 轮询超时 (${maxAttempts}秒)

原始Shell输出（最近30行）：
${recentLines}

过滤后内容长度：${finalShellContent.length}

可能原因：
1. 插件执行时间较长
2. 输出内容被过度过滤
3. 客户端网络延迟

建议：请检查客户端Shell历史记录或联系技术支持`
              : `

[⚠️] Polling Timeout (${maxAttempts}s)

Raw shell output (last 30 lines):
${recentLines}

Filtered content length: ${finalShellContent.length}

Possible causes:
1. The plugin takes a long time to run
2. The output was over-filtered
3. Client network latency

Suggestion: check the client shell history or contact support`;
            ElMessage.warning(isZh.value ? '轮询超时，请检查Shell历史记录' : 'Polling timed out, check shell history');
          }
          
          clearInterval(pollingInterval!);
          pollingInterval = null;
          lastShellContentLength = 0;
          contentStableCount = 0;
          hasSeenCompletionMarker = false;
        } else {
          // 更新进度显示
          if (shellContent.trim().length === 0 || (attempts <= 3 && !hasSeenCompletionMarker)) {
            const dots = '.'.repeat((attempts % 4) + 1);
            const progressLines = pluginOutput.value.split('\n');
            
            // 找到最后一行进度信息并更新
            let lastLineIndex = progressLines.length - 1;
            while (lastLineIndex >= 0 && progressLines[lastLineIndex].includes(isZh.value ? '正在等待执行结果' : 'Waiting for results')) {
              progressLines.splice(lastLineIndex, 1);
              lastLineIndex--;
            }

            progressLines.push(isZh.value
              ? `正在等待执行结果${dots} (${attempts}/${maxAttempts}) - 已等待${attempts}秒`
              : `Waiting for results${dots} (${attempts}/${maxAttempts}) - ${attempts}s elapsed`);
            pluginOutput.value = progressLines.join('\n');
          }
        }
      }
    } catch (error) {
      console.error('获取Shell历史失败:', error);
      if (attempts >= maxAttempts) {
        pluginOutput.value += isZh.value
          ? `

[✗] 获取执行结果失败
错误信息: ${error instanceof Error ? error.message : String(error)}
请检查网络连接和客户端状态`
          : `

[✗] Failed to Fetch Execution Result
Error: ${error instanceof Error ? error.message : String(error)}
Check the network connection and client state`;
        clearInterval(pollingInterval!);
        pollingInterval = null;
        lastShellContentLength = 0;
        ElMessage.error(isZh.value ? '获取执行结果失败' : 'Failed to get execution results');
      }
    } finally {
      pollingInFlight = false;
    }
  }, pollingIntervalMs);
};

// 文件上传前的处理
const beforeUpload = (file: File) => {
  const validTypes = ['application/x-msdownload', 'application/x-executable', 'application/x-sharedlib', 'application/x-so', 'application/x-python', 'text/x-python', 'application/octet-stream'];
  // 与服务端 errors.ValidatePluginExtension 的白名单对齐（.bin 是插件目录里
  // fscan.bin/god.bin 这类载荷的实际扩展名，.o/.obj/.coff 用于 BOF）
  const validExtensions = ['.exe', '.net', '.elf', '.dll', '.so', '.dylib', '.bin', '.sh', '.py', '.ps1', '.o', '.obj', '.coff'];
  const fileName = file.name.toLowerCase();
  const hasValidExtension = validExtensions.some(ext => fileName.endsWith(ext));
  
  if (!validTypes.includes(file.type) && !hasValidExtension) {
    ElMessage.warning(isZh.value ? '只能上传插件文件(可执行载荷或脚本，如 exe/dll/so/bin/sh)' : 'Only plugin files allowed (executable payloads or scripts, e.g. exe/dll/so/bin/sh)');
    return false;
  }
  
  if (file.size > 50 * 1024 * 1024) {
    ElMessage.warning(isZh.value ? '文件大小不能超过50MB' : 'File size cannot exceed 50MB');
    return false;
  }
  
  return true;
};

// 提交上传
const submitUpload = async () => {
  if (fileList.value.length === 0) {
    ElMessage.warning(isZh.value ? '请选择要上传的文件' : 'Select files to upload');
    return;
  }
  
  let okCount = 0;
  for (const file of fileList.value) {
    // 单个文件失败不应中断其余文件的上传（原实现一个 4xx 就跳出整个循环）
    try {
      const response = await pluginApi.upload_plugin(file)
      if (response.data?.status === 200) {
        okCount++;
        ElMessage.success(isZh.value ? `${file.name} 上传成功` : `${file.name} uploaded`);
      } else {
        ElMessage.error(response.data?.message || (isZh.value ? `${file.name} 上传失败` : `${file.name} upload failed`));
      }
    } catch (error: any) {
      // 请求层已按后端 message 弹过提示，这里只补上文件名
      console.error(`[PLUGIN] 上传失败: ${file.name}`, error);
      if (!error?.response) {
        ElMessage.error(isZh.value ? `${file.name} 上传失败: ${error?.message || '未知错误'}` : `${file.name} upload failed: ${error?.message || 'unknown error'}`);
      }
    }
  }
  if (okCount > 0) {
    refreshPlugins();
  }
  // 移除自动清空文件列表和关闭对话框，由用户手动关闭
};

// 上传对话框 banner 承诺"最多只能同时上传 6 个文件"，这里把它兑现
const MAX_UPLOAD_FILES = 6;

// 处理文件选择——上传到插件目录
const handleFileSelect = (event: Event) => {
  const target = event.target as HTMLInputElement;
  if (target.files) {
    const newFiles = Array.from(target.files);
    const accepted: File[] = [];
    for (const file of newFiles) {
      if (beforeUpload(file)) {
        accepted.push(file);
      }
    }
    const room = Math.max(MAX_UPLOAD_FILES - fileList.value.length, 0);
    if (accepted.length > room) {
      ElMessage.warning(isZh.value
        ? `最多只能同时上传 ${MAX_UPLOAD_FILES} 个文件`
        : `Up to ${MAX_UPLOAD_FILES} files at a time`);
    }
    fileList.value.push(...accepted.slice(0, room));
  }
  target.value = ''; // 清空输入
};

// 移除文件
const removeFile = (index: number) => {
  fileList.value.splice(index, 1);
};

// 格式化文件大小
const formatFileSize = (bytes: number) => {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
};

// 点击选择文件按钮
const selectFiles = () => {
  fileInput.value?.click();
};

// 关闭上传对话框
const closeUploadDialog = () => {
  showUploadDialog.value = false;
  fileList.value = []; // 关闭时清空文件列表
};
</script>

<style scoped>
.plug-tabs {
  height: 40px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 0 16px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-card-border);
}
.plug-tab {
  height: 40px;
  display: inline-flex;
  align-items: center;
  padding: 0 12px;
  font-size: 14px;
  color: var(--cs-text);
  cursor: pointer;
  border-bottom: 2px solid transparent;
  margin-bottom: -1px;
}
.plug-tab:hover { color: var(--cs-active); }
.plug-tab.active { color: var(--cs-active); border-bottom-color: var(--cs-active); }

.plugin-container {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-bg);
  overflow-y: auto;
}

.plug-toolbar {
  min-height: 48px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-card-border);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  flex-shrink: 0;
  flex-wrap: wrap;
}

.plug-title-group {
  display: flex;
  align-items: baseline;
  gap: 6px;
}

.plug-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.plug-count {
  font-size: 12px;
  color: var(--cs-text-dim);
}

.plug-spacer {
  flex: 1;
}

.plugin-card {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  margin: 16px;
  padding: 16px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  box-sizing: border-box;
}

.plugin-form {
  /* 让输出区能拿到卡片剩余高度（flex 链：card -> form -> output-section -> wrapper） */
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  margin-bottom: 0;
  padding: 0;
  border-radius: 0;
  box-shadow: none;
  border: none;
  background: transparent;
  color: inherit;
}

/* 各表单项内容统一左对齐、上对齐 */
.plugin-form :deep(.el-form-item) {
  align-items: flex-start;
  margin-bottom: 14px;
}

.plugin-form :deep(.el-form-item__content) {
  text-align: left;
}

.full-width {
  width: 100%;
}

.plugin-select-row {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
}

.plugin-select-row .el-select {
  flex: 1;
  min-width: 0;
}

.plugin-select-row .el-button {
  flex-shrink: 0;
}

/* 插件详情信息条 */
.plugin-detail-strip {
  width: 100%;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 20px;
  background: var(--cs-bg);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  padding: 10px 12px;
}

.pd-loading,
.pd-empty {
  font-size: 13px;
  color: var(--cs-text-dim);
}

.pd-cell {
  display: flex;
  align-items: baseline;
  gap: 6px;
  font-size: 13px;
}

.pd-label {
  color: var(--cs-text-dim);
  white-space: nowrap;
}

.pd-value {
  color: var(--cs-text-bright);
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pd-value.mono {
  font-family: var(--cs-font-mono);
}

/* 输出区域样式 */
.output-section {
  position: relative;
  margin-top: 16px;
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.output-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  padding: 8px 16px;
  min-height: 48px;
  background: var(--cs-table-head);
  border: 1px solid var(--cs-card-border);
  border-bottom: none;
  border-radius: 8px 8px 0 0;
}

.output-title {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.output-controls {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.output-wrapper {
  position: relative;
  display: block;
  flex: 1;
  min-height: 0;
}

.output-textarea {
  background: var(--cs-console);
  color: var(--cs-text);
  font-family: var(--cs-font-mono);
  font-size: 13px;
  border: 1px solid var(--cs-card-border);
  border-top: none;
  border-radius: 0 0 8px 8px;
  padding: 16px;
  resize: none;
  outline: none;
  line-height: 1.6;
  min-width: 0;
  min-height: 200px;
  width: 100%;
  height: 100%;
  transition: border-color 0.3s ease;
  user-select: text; /* 确保可以复制文本 */
  -webkit-user-select: text;
  -moz-user-select: text;
  -ms-user-select: text;
  white-space: pre-wrap; /* 保持格式化 */
  word-wrap: break-word;
  overflow-y: auto;
}

.output-textarea:focus {
  border-color: var(--cs-active);
}

/* 滚动条样式（与Shell一致） */
.output-textarea::-webkit-scrollbar {
  width: 8px;
}

.output-textarea::-webkit-scrollbar-track {
  background: var(--cs-panel);
  border-radius: 4px;
}

.output-textarea::-webkit-scrollbar-thumb {
  background: var(--cs-hover);
  border-radius: 4px;
}

.output-textarea::-webkit-scrollbar-thumb:hover {
  background: var(--cs-border);
}

/* 文本选择效果 */
.output-textarea::selection {
  background-color: var(--cs-active);
  color: #ffffff;
}

.output-textarea::-moz-selection {
  background-color: var(--cs-active);
  color: #ffffff;
}

/* 调整大小手柄 */
.resize-handle-corner {
  position: absolute;
  bottom: 0;
  right: 0;
  width: 20px;
  height: 20px;
  background: linear-gradient(-45deg, transparent 0%, transparent 40%, var(--cs-active) 40%, var(--cs-active) 60%, transparent 60%);
  cursor: nw-resize;
  z-index: 10;
  border-bottom-right-radius: 6px;
}

.resize-handle-right {
  position: absolute;
  top: 50%;
  right: 0;
  width: 4px;
  height: 40px;
  background: var(--cs-active);
  cursor: ew-resize;
  z-index: 9;
  transform: translateY(-50%);
  border-radius: 4px;
  opacity: 0.6;
  transition: opacity 0.3s ease;
}

.resize-handle-bottom {
  position: absolute;
  bottom: 0;
  left: 50%;
  width: 40px;
  height: 4px;
  background: var(--cs-active);
  cursor: ns-resize;
  z-index: 9;
  transform: translateX(-50%);
  border-radius: 4px;
  opacity: 0.6;
  transition: opacity 0.3s ease;
}

.resize-handle-right:hover,
.resize-handle-bottom:hover,
.resize-handle-corner:hover {
  opacity: 1;
}

/* 执行模式选择：下划线页签（激活项主色 + 2px 下划线） */
.execution-mode-buttons {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: 0;
  width: 100%;
  border-bottom: 1px solid var(--cs-card-border);
}

.mode-button {
  height: 36px;
  margin: 0 0 -1px 0 !important;
  padding: 0 12px !important;
  min-width: 0;
  font-size: 14px;
  font-weight: 400;
  border: none !important;
  border-bottom: 2px solid transparent !important;
  border-radius: 0 !important;
  background: transparent !important;
  color: var(--cs-text) !important;
  box-shadow: none !important;
  transition: color 120ms ease, border-color 120ms ease;
}

.mode-button + .mode-button {
  margin-left: 4px !important;
}

.mode-button:hover:not(.is-disabled) {
  background: transparent !important;
  color: var(--cs-active) !important;
  transform: none;
  box-shadow: none !important;
}

.mode-button.el-button--primary {
  background: transparent !important;
  border-bottom-color: var(--cs-active) !important;
  color: var(--cs-active) !important;
  font-weight: 500;
}

.mode-button.el-button--primary:hover {
  background: transparent !important;
  color: var(--cs-active) !important;
  transform: none;
  box-shadow: none !important;
}

.mode-button.is-disabled {
  opacity: 0.45;
}

/* 表单项优化 */
:deep(.el-form-item) {
  margin-bottom: 20px;
}

:deep(.el-form-item__label) {
  color: var(--cs-text);
}

/* 响应式适配 */
@media (max-width: 768px) {
  .plugin-card {
    margin: 8px;
    padding: 12px;
  }

  .output-textarea {
    width: 100%;
    min-width: 300px;
  }
}

/* 上传对话框样式 */
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