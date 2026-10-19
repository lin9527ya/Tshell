<template>
<div class="plugin-container cs-font-ui">
  <PageHeader :title="i18nStore.tl('插件运行', 'Plugins')" />

  <!-- 页签：插件 / 脚本（vshell：同域功能并页） -->
  <div class="plug-tabs">
    <div class="plug-tab" :class="{ active: view === 'plugins' }" @click="view = 'plugins'">
      {{ i18nStore.tl('插件运行', 'Plugins') }}
    </div>
    <div class="plug-tab" :class="{ active: view === 'scripts' }" @click="view = 'scripts'">
      {{ i18nStore.tl('脚本 (CNA)', 'Scripts (CNA)') }}
    </div>
  </div>

  <template v-if="view === 'plugins'">
  <!-- 工具条：左侧标题/计数，右侧操作 -->
  <div class="plug-toolbar">
    <div class="plug-title-group">
      <span class="plug-count">({{ plugins.length }} {{ i18nStore.tl('个插件可用', 'plugins available') }})</span>
    </div>

    <div class="plug-spacer" />

    <button class="cs-btn cs-btn-primary" @click="showUploadDialog = true">
      {{ i18nStore.tl('上传插件', 'Upload Plugin') }}
    </button>
    <button class="cs-btn" @click="refreshPlugins">
      {{ i18nStore.tl('刷新', 'Refresh') }}
    </button>
  </div>

  <div class="plugin-card">
    <el-form :model="pluginForm" label-width="120px" class="plugin-form">
      <el-form-item :label="i18nStore.tl('选择客户端', 'Select Client')" required>
        <el-select
          v-model="pluginForm.clientUid"
          :placeholder="i18nStore.tl('请选择已上线的客户端', 'Select an online client')"
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
      <el-form-item :label="i18nStore.tl('执行模式', 'Execution Mode')">
        <el-radio-group v-model="executionMode" class="mode-group">
          <el-radio-button
            v-for="m in EXEC_MODES"
            :key="m.value"
            :label="m.value"
            :disabled="modeDisabled(m)"
            :title="modeTitle(m)"
          >{{ m.label }}</el-radio-button>
        </el-radio-group>
        <el-alert v-if="!isWindowsClient && pluginForm.clientUid" type="info" :closable="false" show-icon style="margin-top: 8px;">
          {{ i18nStore.tl('Linux 客户端：使用 Script 或 Binary 模式执行插件', 'Linux client: use Script or Binary mode') }}
        </el-alert>
        <el-alert v-if="isWindowsClient && pluginForm.clientUid" type="warning" :closable="false" show-icon style="margin-top: 8px;">
          {{ i18nStore.tl('Windows 客户端：使用 Execute Assembly / Inline Bin / Shellcode / BOF 模式', 'Windows client: use Execute Assembly / Inline Bin / Shellcode / BOF') }}
        </el-alert>
        <el-alert v-if="!pluginForm.clientUid" type="info" :closable="false" show-icon style="margin-top: 8px;">
          {{ i18nStore.tl('请先选择客户端', 'Select a client first') }}
        </el-alert>
      </el-form-item>

      <el-form-item :label="i18nStore.tl('选择插件', 'Select Plugin')" required>
        <div class="plugin-select-row">
          <el-select v-model="pluginForm.selectedPlugin" :placeholder="i18nStore.tl('请选择插件', 'Select a plugin')" class="full-width" @change="handlePluginChange">
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
          >{{ i18nStore.tl('删除插件', 'Delete Plugin') }}</el-button>
        </div>
      </el-form-item>

      <!-- 插件详情（后端 /plugin/details/:pluginName，1:1 CS 信息条） -->
      <el-form-item v-if="pluginForm.selectedPlugin" :label="i18nStore.tl('插件详情', 'Plugin Details')">
        <div class="plugin-detail-strip cs-font-ui">
          <div v-if="pluginDetailsLoading" class="pd-loading">
            {{ i18nStore.tl('加载详情中...', 'Loading plugin details...') }}
          </div>
          <template v-else-if="pluginDetails">
            <div class="pd-cell">
              <span class="pd-label">{{ i18nStore.tl('名称', 'Name') }}</span>
              <span class="pd-value mono">{{ pluginDetails.name || pluginForm.selectedPlugin }}</span>
            </div>
            <div class="pd-cell">
              <span class="pd-label">{{ i18nStore.tl('类型', 'Type') }}</span>
              <span class="pd-value">{{ pluginDetails.type || pluginDetails.Type || '-' }}</span>
            </div>
            <div class="pd-cell">
              <span class="pd-label">{{ i18nStore.tl('大小', 'Size') }}</span>
              <span class="pd-value mono">{{ formatFileSize(pluginDetails.size ?? pluginDetails.Size ?? 0) }}</span>
            </div>
            <div class="pd-cell">
              <span class="pd-label">{{ i18nStore.tl('修改时间', 'Modified') }}</span>
              <span class="pd-value mono">{{ pluginDetails.last_modified || pluginDetails.LastModified || '-' }}</span>
            </div>
            <div class="pd-cell">
              <span class="pd-label">{{ i18nStore.tl('描述', 'Description') }}</span>
              <span class="pd-value">{{ pluginDetails.description || pluginDetails.Description || '-' }}</span>
            </div>
            <button class="cs-btn cs-btn-xs" @click="downloadPluginFile">
              {{ i18nStore.tl('下载', 'Download') }}
            </button>
          </template>
          <div v-else class="pd-empty">{{ i18nStore.tl('未能获取插件详情', 'Plugin details unavailable') }}</div>
        </div>
      </el-form-item>

      <el-form-item :label="i18nStore.tl('参数', 'Arguments')">
        <el-input
          v-model="pluginForm.arguments"
          :placeholder="i18nStore.tl('请输入参数，例如：-h 192.168.0.0', 'Enter arguments, e.g. -h 192.168.0.0')"
          class="full-width"
        />
      </el-form-item>

      <!-- 插件输出区域 -->
      <div class="output-section">
        <div class="output-header">
          <h4 class="output-title">📜 {{ i18nStore.tl('执行输出', 'Execution Output') }}</h4>
          <div class="output-controls">
            <el-button-group>
              <el-button @click="clearOutput" icon="Delete" :title="i18nStore.tl('清空输出', 'Clear Output')">{{ i18nStore.tl('清空', 'Clear') }}</el-button>
              <el-button @click="copyOutput" icon="CopyDocument" :title="i18nStore.tl('复制全部', 'Copy All')">{{ i18nStore.tl('复制', 'Copy') }}</el-button>
              <el-button @click="exportOutput" icon="Download" :title="i18nStore.tl('导出日志', 'Export Log')">{{ i18nStore.tl('导出', 'Export') }}</el-button>
              <el-button @click="refreshShellContent" icon="Refresh" :title="i18nStore.tl('手动刷新', 'Manual Refresh')" :disabled="!pluginForm.clientUid">{{ i18nStore.tl('刷新', 'Refresh') }}</el-button>
            </el-button-group>

            <button
              class="cs-btn cs-btn-primary"
              @click="executePlugin"
              :disabled="!pluginForm.clientUid || !pluginForm.selectedPlugin || !executionMode || pollingInterval !== null"
            >
              <span v-if="pollingInterval">{{ i18nStore.tl('执行中...', 'Executing...') }}</span>
              <span v-else>{{ i18nStore.tl('执行', 'Execute') }} ({{ getExecutionModeText() }})</span>
            </button>

            <button
              v-if="pollingInterval"
              class="cs-btn cs-btn-danger"
              @click="stopPolling"
            >
              {{ i18nStore.tl('停止轮询', 'Stop Polling') }}
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
    
    <!-- 上传文件模态框（已拆为独立组件，自带文件列表状态与样式） -->
    <PluginUploadDialog v-model:visible="showUploadDialog" @uploaded="refreshPlugins" />
  </template>
  <ScriptManagerPanel v-else />
  </div>
</template>

<script setup lang="ts">
// 输出过滤逻辑已抽到 output-filter.ts（26 项测试）。这里的 filterOpts 负责
// 把"本地化提示文案 + 调试日志出口"注入进去 —— 模块本身保持与语言无关、
// 默认无副作用；组件传入 console.log 以**保持原有的调试日志行为**。
import {
  executionModeText,
  filterShellContentByMode,
  filterShellContentForMimikatz,
  type FilterOptions,
} from '@/views/plugins/output-filter'
import { usePanelResize } from '@/views/plugins/usePanelResize'
import PluginUploadDialog from '@/views/plugins/PluginUploadDialog.vue'
// 详情条与上传对话框共用同一个格式化实现（原先是父组件里的一份被两处用）
import { formatPluginFileSize as formatFileSize } from '@/views/plugins/formatSize'
import { ref, onMounted, computed, onUnmounted, defineAsyncComponent } from 'vue';
const ScriptManagerPanel = defineAsyncComponent(() => import('@/layout/ScriptManagerPanel.vue'));
import { useI18nStore } from '@/stores/i18n';
import { ElMessage, ElMessageBox } from 'element-plus';
import { InfoFilled, UploadFilled, Delete, CopyDocument, Download, Refresh } from '@element-plus/icons-vue';
import pluginApi from '@/api/plugin';
import type { ClientInfo, PluginInfo } from '@/api/plugin';
import ClientAPI from '@/api/clients'
import TaskAPI from '@/api/tasks';
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
// 拖拽缩放规则已抽到 usePanelResize.ts（19 项测试）：最小尺寸钳制、
// 三种拖拽模式各自只改一个维度、以及"以渲染尺寸为基准"这条防跳变规则。
const {
  width: outputWidth,
  height: outputHeight,
  userResized,
  start: startPanelResize,
  move: movePanelResize,
  end: endPanelResize,
} = usePanelResize();
// 默认由 CSS 撑满容器（自适应）；用户拖拽过手柄后才固定为像素尺寸
const outputTextarea = ref<HTMLTextAreaElement>();

// 拖拽调整大小相关

// 上传对话框的开关。文件列表等状态已随对话框移到 PluginUploadDialog 内部。
const showUploadDialog = ref(false);

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
});

// 拖拽调整大小功能。
// 规则都在 usePanelResize 里（最小尺寸钳制、三种模式、以渲染尺寸为基准防跳变），
// 这里只负责把 DOM 事件与元素当前尺寸喂进去。
const renderedSize = () => {
  const el = outputTextarea.value;
  if (!el) return null;
  const r = el.getBoundingClientRect();
  return { width: r.width, height: r.height };
};

const startResize = (e: MouseEvent) => {
  e.preventDefault();
  startPanelResize('both', e.clientX, e.clientY, renderedSize());
};

const startResizeRight = (e: MouseEvent) => {
  e.preventDefault();
  startPanelResize('width', e.clientX, e.clientY, renderedSize());
};

const startResizeBottom = (e: MouseEvent) => {
  e.preventDefault();
  startPanelResize('height', e.clientX, e.clientY, renderedSize());
};

const handleMouseMove = (e: MouseEvent) => {
  movePanelResize(e.clientX, e.clientY);
};

const handleMouseUp = () => {
  endPanelResize();
};

// 设置执行模式
// 模式显示文案已抽到 output-filter.ts（可单测）。这里保留同名薄封装，调用点不用改。
const getExecutionModeText = () => executionModeText(executionMode.value, isZh.value);

// 两个过滤器的"无输出"提示文案**不一样**（原实现如此）：
//   通用版：引导去看当前执行模式
//   mimikatz 版：引导去检查执行是否成功、必要时看完整 Shell 历史
// 传错会让操作员按错误的提示去排查。
const filterOpts = (kind: 'mode' | 'mimikatz'): FilterOptions => ({
  noOutputHint:
    kind === 'mimikatz'
      ? isZh.value
        ? '暂无执行相关输出，请检查执行是否成功。\n\n如果问题持续，请查看完整Shell历史。'
        : 'No execution output found, check whether the run succeeded.\n\nIf the problem persists, review the full shell history.'
      : isZh.value
        ? '暂无当前执行模式的相关输出。\n\n请执行插件后查看结果。'
        : 'No output for the current execution mode.\n\nRun a plugin and check the result.',
  log: (...args: unknown[]) => console.log(...args),
})

// 执行模式：platform = windows / linux / any（any 不随客户端平台禁用）
const EXEC_MODES = computed(() => [
  { value: 'execute-assembly', label: i18nStore.tl('Execute Assembly (.NET)', 'Execute Assembly (.NET)'), platform: 'windows' },
  { value: 'inline-bin', label: i18nStore.tl('Inline Bin (exe)', 'Inline Bin (exe)'), platform: 'windows' },
  { value: 'shellcode-inject', label: i18nStore.tl('Shellcode', 'Shellcode'), platform: 'windows' },
  { value: 'script', label: i18nStore.tl('Script (Linux)', 'Script (Linux)'), platform: 'linux' },
  { value: 'binary', label: i18nStore.tl('Binary (Linux)', 'Binary (Linux)'), platform: 'linux' },
  { value: 'bof', label: 'BOF (COFF)', platform: 'windows' },
  { value: 'exec-file', label: i18nStore.tl('Exec File (临时文件)', 'Exec File (temp)'), platform: 'any' },
])

type ExecMode = { value: string; label: string; platform: string }

function modeDisabled(m: ExecMode): boolean {
  if (m.platform === 'windows') return !isWindowsClient.value
  if (m.platform === 'linux') return isWindowsClient.value
  return false
}

function modeTitle(m: ExecMode): string {
  if (m.platform === 'windows') return isWindowsClient.value ? '' : i18nStore.tl('仅 Windows 客户端可用', 'Windows clients only')
  if (m.platform === 'linux') return isWindowsClient.value ? i18nStore.tl('仅 Linux 客户端可用', 'Linux clients only') : ''
  return ''
}

const setExecutionMode = (mode: string) => {
  executionMode.value = mode;
};

// 获取执行模式文本

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
          ExternalIP: c.external_ip || c.ExternalIP || '-',
          Username: c.username || c.Username || 'user',
          Listener: c.listener || c.Listener || 'tcp',
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
          shellContent = filterShellContentForMimikatz(fullShellContent, executionMode.value, pluginForm.value.selectedPlugin, filterOpts('mimikatz'));
          console.log('🔍 使用 mimikatz 专用过滤器');
        } else {
          shellContent = filterShellContentByMode(fullShellContent, executionMode.value, pluginForm.value.selectedPlugin, filterOpts('mode'));
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
      
      // 任务 ID：由 /bin/execute 建任务时回传，用它轮询真实终态
      // （后端未建任务时 task_id 为 0，此时按输出稳定性兜底）
      const execTaskId = Number(result.task_id) || 0;
      if (!execTaskId) {
        console.warn('未取得任务 ID，回退到输出稳定性判定（任务中心将看不到本次执行）');
      }

      // 开始轮询获取执行结果
      startPollingForResults(clientUid, pluginName, execTaskId);
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

// 专门为 mimikatz 类工具设计的宽松过滤函数
let pollingInterval: ReturnType<typeof setInterval> | null = null;

// 插件执行 = 一条 exec 任务（后端 /bin/execute 建任务并回传 task_id）。
// 完成判定改由**任务终态**给出：原实现靠"输出连续稳定 N 次 + 命中完成标志字符串"猜，
// 分不清"执行完了"与"还在跑但暂时没输出"，只能一路等到超时再强行展示结果。
// 现在：dispatched = 还在跑（实时刷输出），completed/failed/timeout/cancelled = 已结束。
const startPollingForResults = (clientUid: string, pluginName: string, taskId: number) => {
  if (pollingInterval) {
    clearInterval(pollingInterval);
    pollingInterval = null;
  }

  const startedAt = Date.now();
  const maxWaitMs = 10 * 60 * 1000; // 前端等待上限（mimikatz 这类慢工具要留足时间）
  const pollingIntervalMs = 1000;
  const isMimikatzTool = pluginName.toLowerCase().includes('mimikatz') ||
                        pluginName.toLowerCase().includes('sekur') ||
                        pluginName.toLowerCase().includes('creds');

  let pollingInFlight = false;
  let taskStatus = '';
  let taskError = '';
  let lastContent = '';
  let stableReads = 0;

  // 读取并过滤当前 shell 回显（展示用；不再参与"是否执行完"的判定）
  const readShellContent = async (): Promise<string> => {
    const response = await pluginApi.get_shell_history(clientUid);
    if (!(response.data && response.data.status === 200)) return lastContent;
    const fullShellContent = smartDecodeContent(response.data.data || '');
    const filtered = isMimikatzTool
      ? filterShellContentForMimikatz(fullShellContent, executionMode.value, pluginName, filterOpts('mimikatz'))
      : filterShellContentByMode(fullShellContent, executionMode.value, pluginName, filterOpts('mode'));
    lastContent = filtered;
    return filtered;
  };

  const clientLabel = () => {
    const c = onlineClients.value.find(client => client.Uid === clientUid);
    return c ? (c.Computer || c.ExternalIP || clientUid.slice(0, 8)) : clientUid.slice(0, 8);
  };

  const elapsedSec = () => Math.round((Date.now() - startedAt) / 1000);

  const stopPolling = () => {
    if (pollingInterval) {
      clearInterval(pollingInterval);
      pollingInterval = null;
    }
  };

  // 终态展示：ok=成功结束（无论有没有输出），note 为补充说明（失败原因/超时提示）
  const showFinal = (content: string, opts: { ok: boolean; title?: string; note?: string }) => {
    const header = isZh.value
      ? `[${opts.ok ? '✓' : '✗'}] ${opts.title || (opts.ok ? '插件执行完成' : '插件执行未成功')}
模式: ${getExecutionModeText()}
插件: ${pluginName}
参数: ${pluginForm.value.arguments || '无'}
客户端: ${clientLabel()}
执行时间: ${elapsedSec()}秒
显示模式: ${isMimikatzTool ? '完整输出模式（mimikatz优化）' : '过滤输出模式'}`
      : `[${opts.ok ? '✓' : '✗'}] ${opts.title || (opts.ok ? 'Plugin finished' : 'Plugin did not finish')}
Mode: ${getExecutionModeText()}
Plugin: ${pluginName}
Arguments: ${pluginForm.value.arguments || 'none'}
Client: ${clientLabel()}
Elapsed: ${elapsedSec()}s
Display mode: ${isMimikatzTool ? 'full output (mimikatz tuned)' : 'filtered output'}`;

    const body = content && content.trim().length > 0
      ? (isZh.value ? `\n\n=== 最终执行结果 ===\n${content}` : `\n\n=== Final Execution Result ===\n${content}`)
      : (isZh.value ? '\n\n（该任务没有回显内容）' : '\n\n(no output recorded for this task)');

    pluginOutput.value = header + body + (opts.note ? `\n\n${opts.note}` : '');
    stopPolling();
    if (opts.ok) {
      ElMessage.success(isZh.value ? '插件执行完成，完整结果已显示' : 'Plugin finished, full results shown');
    } else {
      ElMessage.error(opts.note || (isZh.value ? '插件执行未成功' : 'Plugin did not finish'));
    }
  };

  // 回显与任务状态更新之间有毫秒级时差：终态后短轮询补齐输出
  const settleRead = async (): Promise<string> => {
    let best = lastContent;
    for (let i = 0; i < 3; i++) {
      await new Promise(r => setTimeout(r, 700));
      try {
        const c = await readShellContent();
        if (c.length > best.length) best = c;
        if (best.trim().length > 0 && i >= 1) break;
      } catch { /* 忽略单次失败 */ }
    }
    return best;
  };

  pollingInterval = setInterval(async () => {
    if (pollingInFlight) return;
    pollingInFlight = true;

    try {
      // 1) 真实终态：任务状态（服务端按回包收敛，见 pkg/api/tasks.go）
      if (taskId) {
        try {
          const taskRes = await TaskAPI.get(taskId);
          const t = taskRes.data?.data;
          if (t && t.status) {
            taskStatus = t.status;
            taskError = t.error || '';
          }
        } catch { /* 单次查询失败不改变判定，下一轮重试 */ }
      }

      // 2) 实时输出（展示用）
      let shellContent = lastContent;
      try {
        shellContent = await readShellContent();
      } catch (e) {
        console.error('获取Shell历史失败:', e);
      }

      if (taskStatus && taskStatus !== 'dispatched') {
        const content = await settleRead();
        if (taskStatus === 'completed') {
          showFinal(content, { ok: true });
        } else if (taskStatus === 'failed') {
          showFinal(content, {
            ok: false,
            title: isZh.value ? '插件执行失败' : 'Plugin execution failed',
            note: (isZh.value ? '失败原因: ' : 'Reason: ') + (taskError || (isZh.value ? '未知（目标未回执）' : 'unknown')),
          });
        } else if (taskStatus === 'cancelled') {
          showFinal(content, {
            ok: false,
            title: isZh.value ? '任务已取消' : 'Task cancelled',
            note: isZh.value ? '已停止等待回显（命令无法从目标撤回）' : 'Stopped waiting for output (command cannot be retracted)',
          });
        } else if (taskStatus === 'timeout') {
          showFinal(content, {
            ok: false,
            title: isZh.value ? '任务超时' : 'Task timed out',
            note: isZh.value ? '服务端判定目标长时间未回执，可能已离线' : 'Target did not respond in time; it may be offline',
          });
        } else {
          showFinal(content, { ok: false, title: isZh.value ? `任务状态: ${taskStatus}` : `Task status: ${taskStatus}` });
        }
        return;
      }

      // 3) 仍在执行：刷新实时视图
      if (shellContent.trim().length > 0) {
        pluginOutput.value = isZh.value
          ? `[⏳] 插件正在执行...
模式: ${getExecutionModeText()}
插件: ${pluginName}
参数: ${pluginForm.value.arguments || '无'}
客户端: ${clientLabel()}
已等待: ${elapsedSec()}秒${taskId ? '' : '（未取得任务状态，按输出稳定性判断）'}

=== 实时执行结果 ===
${shellContent}`
          : `[⏳] Plugin running...
Mode: ${getExecutionModeText()}
Plugin: ${pluginName}
Arguments: ${pluginForm.value.arguments || 'none'}
Client: ${clientLabel()}
Elapsed: ${elapsedSec()}s${taskId ? '' : ' (no task status; judging by output stability)'}

=== Live Execution Result ===
${shellContent}`;
        if (shellContent === lastContent) stableReads++; else stableReads = 0;
      } else {
        pluginOutput.value = isZh.value
          ? `[⏳] 已下发，等待目标回显...
模式: ${getExecutionModeText()}
插件: ${pluginName}
客户端: ${clientLabel()}
已等待: ${elapsedSec()}秒

提示：目标为轮询型客户端时，需要等它下一次回连才会取到命令。`
          : `[⏳] Dispatched, waiting for target output...
Mode: ${getExecutionModeText()}
Plugin: ${pluginName}
Client: ${clientLabel()}
Elapsed: ${elapsedSec()}s

Note: polling transports fetch commands on their next check-in.`;
      }

      // 4) 拿不到任务状态时的兜底（DB 不可用/任务未建成）：输出稳定即视为结束
      if (!taskId && stableReads >= 3 && shellContent.trim().length > 10) {
        showFinal(shellContent, {
          ok: true,
          title: isZh.value ? '插件执行完成（按输出稳定性判断）' : 'Plugin finished (judged by output stability)',
          note: isZh.value ? '未能取得任务状态（数据库不可用？），此处按输出连续稳定判定' : 'No task status available (DB down?); judged by stable output',
        });
        return;
      }

      // 5) 前端等待上限
      if (Date.now() - startedAt > maxWaitMs) {
        showFinal(lastContent, {
          ok: false,
          title: isZh.value ? '等待回显超时' : 'Timed out waiting for output',
          note: isZh.value
            ? `已等待 ${Math.round(maxWaitMs / 60000)} 分钟仍未收到回执。任务仍在服务端任务中心（可查状态或取消），目标可能已离线或命令仍在队列中。`
            : `Waited ${Math.round(maxWaitMs / 60000)} minutes with no response. The task is still listed in the Task Center (check status or cancel); the target may be offline or the command still queued.`,
        });
      }
    } finally {
      pollingInFlight = false;
    }
  }, pollingIntervalMs);
};

// 文件上传前的处理
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
  /* 右下角拖拽手柄：三点抓握，默认低调，悬停高亮 */
  background:
    radial-gradient(circle at 78% 22%, var(--cs-text-dim) 1.2px, transparent 1.5px),
    radial-gradient(circle at 58% 42%, var(--cs-text-dim) 1.2px, transparent 1.5px),
    radial-gradient(circle at 78% 58%, var(--cs-text-dim) 1.2px, transparent 1.5px);
  opacity: 0.55;
  transition: opacity 0.2s ease;
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
  background: var(--cs-border-strong);
  opacity: 0.45;
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
  background: var(--cs-border-strong);
  opacity: 0.45;
  cursor: ns-resize;
  z-index: 9;
  transform: translateX(-50%);
  border-radius: 4px;
  opacity: 0.6;
  transition: opacity 0.3s ease;
}

.resize-handle-right:hover,
.resize-handle-bottom:hover {
  background: var(--cs-active);
  opacity: 0.85;
}

.resize-handle-corner:hover {
  opacity: 1;
}

/* 执行模式选择：下划线页签（激活项主色 + 2px 下划线） */


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

/* 执行模式：Element Plus 分段控件（替代原无边框按钮排） */
.mode-group {
  flex-wrap: wrap;
}
</style>
