<script setup lang="ts">
import { ref, computed, onMounted, watch, provide, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  Monitor,
  User,
  Connection,
  CircleClose,
  Refresh,
  Back,
  Files,
  VideoCamera,
  Download,
  Document,
  Setting,
  Operation,
  View,
  Position
} from '@element-plus/icons-vue'
import ClientAPI from '@/api/clients'
import { useClientUid, detectClientOs } from '@/utils/clientUtils'
import { useClientsStore } from '@/stores/clients'
// 行内展开模式直接渲染的子组件（路由模式仍走子路由懒加载）
import LinuxInteractiveTerminal from './View/LinuxInteractiveTerminal.vue'
import ClientShell from './View/ClientShell.vue'
import ClientFiles from './View/ClientFiles.vue'
import ClientScreenshot from './View/ClientScreenshot.vue'
import ClientMonitor from './View/ClientMonitor.vue'
import ClientPid from './View/ClientPid.vue'
import ClientStartup from './View/ClientStartup.vue'
import ClientTools from './View/ClientTools.vue'
import ClientDownloads from './View/ClientDownloads.vue'
import ClientNotes from './View/ClientNotes.vue'

const route = useRoute()
const router = useRouter()
const clientsStore = useClientsStore()

// 行内展开模式：父组件（客户端管理表格展开行）传入 uid，不走路由跳转
const props = defineProps<{ uid?: string }>()

// 当前激活的标签页
const activeTab = ref('terminal')

// 获取真实UID（行内模式优先用 prop，否则从路由获取）
const realUid = computed(() => {
  if (props.uid) return props.uid
  return useClientUid(route)
})

// 行内模式标志（无路由上下文时按 prop 判断）
const isInlineMode = computed(() => !!props.uid)

// 客户端信息 - 使用响应式数据源
const clientInfo = computed(() => {
  const currentUid = realUid.value
  const allClients = clientsStore.clientsList
  
  // 尝试多种匹配方式找到对应的客户端
  let targetClient = null
  
  if (currentUid && allClients.length > 0) {
    targetClient = allClients.find(client => client.Uid === currentUid)
    if (!targetClient && currentUid.length >= 8) {
      const shortUid = currentUid.substring(0, 8)
      targetClient = allClients.find(client => client.Uid.startsWith(shortUid))
    }
    if (!targetClient) {
      const uidPart = currentUid.substring(0, Math.min(12, currentUid.length))
      targetClient = allClients.find(client => client.Uid.includes(uidPart) || uidPart.includes(client.Uid.substring(0, 8)))
    }
  }
  
  if (targetClient) {
    return {
      uid: targetClient.Uid,
      computer: targetClient.Computer || '未知主机',
      username: targetClient.Username || '未知用户',
      arch: targetClient.Arch,
      address: targetClient.Address,
      externalIP: targetClient.ExternalIP || '',
      internalIP: targetClient.InternalIP || '',
      online: targetClient.Online === '1',
      firstStart: targetClient.FirstStart,
      sleep: parseInt(targetClient.Sleep) >= 0 ? parseInt(targetClient.Sleep) : 1
    }
  }
  
  return {
    uid: currentUid || 'unknown-uid',
    computer: 'Unknown-Host',
    username: 'Unknown-User',
    arch: 'windows_x64',
    address: '-',
    externalIP: '',
    internalIP: '',
    online: false,
    firstStart: '-',
    sleep: 0
  }
})

// 提供真实UID给子组件
provide('realUid', realUid)
// 提供客户端信息给子组件
provide('clientInfo', clientInfo)

// 操作报告
const reportVisible = ref(false)
const reportLoading = ref(false)
const reportTab = ref('shell')
const reportData = ref<any>({})

const openReportDialog = async () => {
  reportVisible.value = true
  reportLoading.value = true
  reportData.value = {}
  try {
    const res = await ClientAPI.get_client_report(realUid.value)
    if (res.data?.status === 200) {
      reportData.value = res.data.data
    } else {
      ElMessage.error(res.data?.data || '获取报告失败')
    }
  } catch (error: any) {
    ElMessage.error(error.response?.data?.error || '获取报告失败')
  } finally {
    reportLoading.value = false
  }
}

// 检测是否为Linux客户端
const isLinuxClient = computed(() => {
  const osType = detectClientOs({
    Arch: clientInfo.value.arch,
    Computer: clientInfo.value.computer,
    Username: clientInfo.value.username,
  })
  return osType === 'linux'
})

// 动态导航标签页配置 - 统一显示"命令控制"，根据操作系统类型自动跳转
const navigationTabs = computed(() => {
  const terminalTab = {
    name: 'terminal',
    label: '命令控制',
    icon: 'Monitor'
  }
  
  const tabs = [terminalTab]

  tabs.push({
    name: 'shell',
    label: '命令输出',
    icon: 'Monitor'
  })

  tabs.push(
    {
      name: 'files',
      label: '文件管理',
      icon: 'Files'
    },
    {
      name: 'screenshot',
      label: '屏幕截图',
      icon: 'VideoCamera'
    },
    {
      name: 'monitor',
      label: '屏幕监控',
      icon: 'View'
    },
    {
      name: 'pid',
      label: '进程管理',
      icon: 'Operation'
    },
    {
      name: 'startup',
      label: '开机启动',
      icon: 'Setting'
    },
    {
      name: 'tools',
      label: '工具箱',
      icon: 'Setting'
    },
    {
      name: 'downloads',
      label: '下载管理',
      icon: 'Download'
    },
    {
      name: 'notes',
      label: '笔记',
      icon: 'Document'
    }
  )
  
  return tabs
})

// 需要缓存的组件（InteractiveTerminal 已废弃删除，统一走 LinuxInteractiveTerminal）
const keepAliveComponents = computed(() => {
  return ['LinuxInteractiveTerminal', 'ClientFiles', 'ClientDownloads', 'ClientNotes']
})

// 行内展开模式：标签 → 组件映射（路由模式走子路由，行内模式直接渲染）
const inlineViewComponent = computed(() => {
  switch (activeTab.value) {
    case 'terminal':
    case 'linux-terminal':
      return LinuxInteractiveTerminal
    case 'shell':
      return ClientShell
    case 'files':
      return ClientFiles
    case 'screenshot':
      return ClientScreenshot
    case 'monitor':
      return ClientMonitor
    case 'pid':
      return ClientPid
    case 'startup':
      return ClientStartup
    case 'tools':
      return ClientTools
    case 'downloads':
      return ClientDownloads
    case 'notes':
      return ClientNotes
    default:
      return LinuxInteractiveTerminal
  }
})

// 方法实现
const refreshClientInfo = async () => {
  if (realUid.value) {
    await clientsStore.fetchClients()
    ElMessage.success('客户端列表已刷新')
  } else {
    ElMessage.error('无法刷新，客户端ID不可用')
  }
}

const handleTabChange = (tabName: string) => {
  const tab = navigationTabs.value.find(t => t.name === tabName)
  const uid = realUid.value
  
  if (tab && uid) {
    let targetPath = tab.name
    
    if (tabName === 'terminal') {
      targetPath = 'linux-terminal'
    }
    
    if (isInlineMode.value) {
      activeTab.value = targetPath
      return
    }
    router.push({
      path: `/clientManager/${uid}/${targetPath}`
    })
  }
}

const formatFileSize = (size: number) => {
  if (!size && size !== 0) return '-'
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
  return `${(size / 1024 / 1024).toFixed(1)} MB`
}

const downloadSessionLog = async (filename: string) => {
  try {
    const res = await ClientAPI.download_session_log(realUid.value, filename)
    const blob = new Blob([res.data], { type: 'text/plain' })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = filename
    a.click()
    URL.revokeObjectURL(a.href)
  } catch (error: any) {
    ElMessage.error(error.response?.data?.error || '下载失败')
  }
}

// 监听客户端列表变化，确保数据同步
watch(
  () => clientsStore.clientsList,
  (newList) => {
    if (newList.length > 0) {
      nextTick(() => {
        if (isInlineMode.value) return
        const currentPath = route.path
        const uid = route.params.uid as string
        
        if (uid && currentPath.endsWith('/terminal')) {
          const newPath = currentPath.replace('/terminal', '/linux-terminal')
          router.replace(newPath)
        }
      })
    }
  },
  { deep: true, immediate: true }
)

// 监听路由变化，同步激活的标签页（行内模式不依赖路由）
watch(
  () => route.path,
  (newPath) => {
    if (isInlineMode.value) return
    const pathParts = newPath.split('/')
    if (pathParts.length >= 4) {
      const tabName = pathParts[3]
      nextTick(() => {
        let mappedTabName = tabName
        if (tabName === 'linux-terminal') {
          mappedTabName = 'terminal'
        }
        
        const currentTab = navigationTabs.value.find(tab => tab.name === mappedTabName)
        if (currentTab) {
          activeTab.value = currentTab.name
        }
      })
    }
  },
  { immediate: true }
)

// onMounted 时确保客户端列表已加载
onMounted(async () => {
  try {
    await clientsStore.fetchClients()
    nextTick(() => {
      if (isInlineMode.value) return
      const currentPath = route.path
      const uid = route.params.uid as string
      
      if (uid && currentPath.endsWith('/terminal')) {
        const newPath = currentPath.replace('/terminal', '/linux-terminal')
        router.replace(newPath)
      }
    })
  } catch (error) {
    console.error('加载客户端数据失败:', error)
  }
})
</script>

<template>
  <div class="client-detail-container">
    <!-- 客户端信息头部 -->
    <div class="client-header">
      <div class="client-info-card">
        <div class="client-basic-info">
          <div class="client-avatar">
            <el-icon :size="24"><Monitor /></el-icon>
          </div>
          <!-- 左上角信息徽章：IP / User / PC / 状态 -->
          <div class="client-summary">
            <div class="summary-badge badge-ip" title="客户端上线 IP">
              <el-icon :size="13"><Position /></el-icon>
              <span class="badge-key">IP：</span><span class="badge-val">{{ clientInfo.externalIP || '-' }}</span>
            </div>
            <div class="summary-badge badge-user" title="用户">
              <el-icon :size="13"><User /></el-icon>
              <span class="badge-key">User：</span><span class="badge-val">{{ clientInfo.username }}</span>
            </div>
            <div class="summary-badge badge-pc" title="主机名">
              <el-icon :size="13"><Monitor /></el-icon>
              <span class="badge-key">PC：</span><span class="badge-val">{{ clientInfo.computer }}</span>
            </div>
            <div class="summary-badge" :class="clientInfo.online ? 'badge-online' : 'badge-offline'" title="在线状态">
              <el-icon :size="13"><Connection v-if="clientInfo.online" /><CircleClose v-else /></el-icon>
              <span class="badge-key">状态：</span><span class="badge-val">{{ clientInfo.online ? '在线' : '离线' }}</span>
            </div>
          </div>
        </div>
        <div class="header-right-panel">
          <div class="client-actions">
              <el-button-group>
                  <el-button @click="refreshClientInfo" :icon="Refresh" size="small" class="action-btn">刷新</el-button>
                  <el-button @click="openReportDialog" :icon="Document" size="small" class="action-btn">操作报告</el-button>
                  <el-button v-if="!isInlineMode" @click="$router.push('/Clients')" :icon="Back" size="small" class="action-btn">返回</el-button>
              </el-button-group>
          </div>
        </div>
      </div>
    </div>

    <!-- 功能模块标签页 -->
    <div class="client-tabs">
      <el-tabs 
        v-model="activeTab" 
        type="card" 
        class="client-navigation-tabs"
        scrollable
        @tab-change="handleTabChange"
      >
        <el-tab-pane 
          v-for="tab in navigationTabs" 
          :key="tab.name"
          :label="tab.label"
          :name="tab.name"
        >
          <template #label>
            <div class="tab-label">
              <el-icon><component :is="tab.icon" /></el-icon>
              <span>{{ tab.label }}</span>
            </div>
          </template>
        </el-tab-pane>
      </el-tabs>
    </div>

    <!-- 主内容区域 -->
    <div class="client-content">
      <div class="content-wrapper">
        <!-- 行内展开模式：直接渲染当前标签组件（不依赖子路由） -->
        <template v-if="isInlineMode">
          <keep-alive :include="keepAliveComponents">
            <component :is="inlineViewComponent" />
          </keep-alive>
        </template>
        <!-- 路由模式：子路由渲染 -->
        <router-view v-else v-slot="{ Component }">
          <transition name="tab-fade" mode="out-in">
            <keep-alive :include="keepAliveComponents">
              <component :is="Component" />
            </keep-alive>
          </transition>
        </router-view>
      </div>
      
    </div>

    <!-- 操作报告对话框 -->
    <el-dialog v-model="reportVisible" title="操作报告" width="760px" append-to-body>
      <div v-loading="reportLoading" class="report-body">
        <el-tabs v-model="reportTab">
          <el-tab-pane label="Shell 历史" name="shell">
            <pre class="report-pre">{{ reportData.shell_history || '暂无 Shell 历史' }}</pre>
          </el-tab-pane>
          <el-tab-pane label="会话录像" name="sessions">
            <el-table :data="reportData.session_logs || []" size="small" stripe>
              <el-table-column prop="file" label="文件名" min-width="200" show-overflow-tooltip />
              <el-table-column prop="size" label="大小" min-width="80">
                <template #default="{ row }">{{ formatFileSize(row.size) }}</template>
              </el-table-column>
              <el-table-column prop="modified" label="修改时间" min-width="140" />
              <el-table-column label="操作" min-width="80">
                <template #default="{ row }">
                  <el-button size="small" type="primary" @click="downloadSessionLog(row.file)">下载</el-button>
                </template>
              </el-table-column>
              <template #empty>
                <div class="panel-empty">暂无会话录像</div>
              </template>
            </el-table>
          </el-tab-pane>
          <el-tab-pane label="下载记录" name="downloads">
            <el-table :data="reportData.downloads || []" size="small" stripe>
              <el-table-column prop="FileName" label="文件名" min-width="180" show-overflow-tooltip />
              <el-table-column prop="FilePath" label="路径" min-width="200" show-overflow-tooltip />
              <el-table-column prop="CreatedAt" label="时间" min-width="140" />
              <template #empty>
                <div class="panel-empty">暂无下载记录</div>
              </template>
            </el-table>
          </el-tab-pane>
        </el-tabs>
      </div>
      <template #footer>
        <el-button @click="reportVisible = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.client-detail-container {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--theme-bg-primary);
}

/* 客户端信息头部 */
.client-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 16px;
  background: var(--card-bg);
  border-radius: var(--card-radius);
  box-shadow: var(--card-shadow);
  margin-bottom: 8px;
}

.client-info-card {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  flex: 1;
}

.client-basic-info {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
  flex: 1;
}

.client-avatar {
  width: 36px;
  height: 36px;
  background: var(--theme-primary);
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: white;
  flex-shrink: 0;
}

/* 左上角信息徽章 */
.client-summary {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}

.summary-badge {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  height: 24px;
  padding: 0 10px;
  border-radius: 6px;
  font-size: 12px;
  color: #ffffff;
  white-space: nowrap;
  flex-shrink: 0;
}

.badge-key {
  font-weight: 700;
  opacity: 0.9;
}

.badge-val {
  font-weight: 500;
  max-width: 140px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.badge-ip {
  background: #1677ff;
}

.badge-user {
  background: #722ed1;
}

.badge-pc {
  background: #13c2c2;
}

.badge-online {
  background: #52c41a;
}

.badge-offline {
  background: #f5222d;
}

.header-right-panel {
  display: flex;
  align-items: center;
  gap: 16px;
}

.client-status {
  display: flex;
  align-items: center;
  gap: 16px;
  flex-shrink: 0;
}

.status-tag {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  font-weight: 500;
}

.status-info {
  display: flex;
  gap: 12px;
  text-align: left;
}

.status-item {
  display: flex;
  flex-direction: row;
  align-items: center;
  gap: 4px;
  font-size: 12px;
}

.status-item .label {
  color: var(--theme-text-tertiary);
}

.status-item .value {
  color: var(--theme-text-primary);
  font-weight: 500;
}

.client-actions {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}

.client-actions .action-btn {
  height: 30px;
  padding: 0 10px;
  font-size: 12px;
  border-radius: 4px;
}

/* 标签页区域 */
.client-tabs {
  background: var(--card-bg);
  border-radius: var(--card-radius);
  box-shadow: var(--card-shadow);
  margin-bottom: 8px;
  overflow: hidden;
}

.client-navigation-tabs {
  background: var(--card-bg);
}

:deep(.el-tabs__header) {
  margin: 0;
  background: var(--card-header-bg);
  border-bottom: 1px solid var(--theme-border-primary);
}

:deep(.el-tabs__nav) {
  border: none;
}

:deep(.el-tabs__nav-wrap) {
  overflow-x: auto;
  scrollbar-width: thin;
}
:deep(.el-tabs__nav-scroll) {
  overflow: visible;
}
:deep(.el-tabs__nav-wrap::after) {
  display: none;
}

:deep(.el-tabs__item) {
  border: none;
  background: transparent;
  color: var(--theme-text-secondary);
  font-weight: 500;
  transition: all var(--transition-duration-normal);
  padding: 8px 12px;
  height: 38px;
  line-height: 38px;
  position: relative;
  white-space: nowrap;
}

:deep(.el-tabs__item:hover) {
  color: var(--theme-primary);
  background: color-mix(in srgb, var(--theme-primary, #1f6feb) 8%, transparent);
}

:deep(.el-tabs__item.is-active) {
  color: var(--theme-primary);
  background: transparent;
  border-bottom: 2px solid var(--theme-primary);
  font-weight: 600;
}

:deep(.el-tabs__item.is-active::after) {
  content: '';
  position: absolute;
  bottom: 0;
  left: 50%;
  transform: translateX(-50%);
  width: 80%;
  height: 2px;
  background: var(--theme-primary);
  border-radius: 2px 2px 0 0;
}

.tab-label {
  display: flex;
  align-items: center;
  gap: 8px;
}

.tab-label .el-icon {
  width: 16px;
  height: 16px;
}

/* 内容区域 */
.client-content {
  flex: 1;
  background: var(--card-bg);
  border-radius: var(--card-radius);
  box-shadow: var(--card-shadow);
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.content-wrapper {
  flex: 1;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

/* 标签页切换动画 */
.tab-fade-enter-active,
.tab-fade-leave-active {
  transition: all var(--transition-duration-normal) ease;
}

.tab-fade-enter-from {
  opacity: 0;
  transform: translateY(10px);
}

.tab-fade-leave-to {
  opacity: 0;
  transform: translateY(-10px);
}

/* 响应式设计 */
@media (max-width: 768px) {
  .client-header {
    flex-direction: column;
    gap: 16px;
  }
  
  .client-info-card {
    flex-direction: column;
    align-items: flex-start;
  }
  
  .client-basic-info {
    width: 100%;
  }
  
  .client-status {
    width: 100%;
    align-items: flex-start;
  }
  
  :deep(.el-tabs__item) {
    padding: 12px 16px;
    font-size: 14px;
  }
  
  .tab-label span {
    display: none;
  }
}

@media (max-width: 480px) {
  .client-detail-container {
    padding: 16px;
  }
  
  .client-header {
    padding: 16px;
  }
  
  .client-actions {
    width: 100%;
    justify-content: stretch;
  }
  
  .client-actions .el-button {
    flex: 1;
  }
}

.report-body {
  min-height: 300px;
}
.report-pre {
  max-height: 420px;
  overflow: auto;
  font-family: var(--font-family-mono, monospace);
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
  color: var(--text-secondary, #666);
  background: var(--bg-secondary, #f5f5f5);
  padding: 12px;
  border-radius: 6px;
}
.panel-empty {
  padding: 20px 0;
  text-align: center;
  font-size: 13px;
  color: var(--text-tertiary, #999);
}
</style>
