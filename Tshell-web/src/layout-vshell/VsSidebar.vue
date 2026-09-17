<template>
  <div class="vs-sidebar" :class="{ collapsed }">
    <div class="vs-logo">
      <span class="vs-logo-dot" />
      <span v-if="!collapsed">BladeShell</span>
    </div>

    <nav class="vs-nav">
      <template v-for="group in navGroups" :key="group.label">
        <div class="vs-nav-group-label">{{ collapsed ? '·' : group.label }}</div>
        <button
          v-for="item in group.items"
          :key="item.key"
          class="vs-nav-item"
          :class="{ active: !collapsed && activeKey === item.key }"
          :title="collapsed ? item.label : ''"
          @click="$emit('navigate', item)"
        >
          <span class="vs-nav-icon">
            <component :is="item.icon" v-if="item.icon" />
          </span>
          <span class="vs-nav-label">{{ item.label }}</span>
        </button>
      </template>
    </nav>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18nStore } from '@/stores/i18n'
import { useWorkspaceStore } from '@/stores/workspace'

defineProps<{ collapsed: boolean; activeKey: string }>()
defineEmits<{ (e: 'navigate', payload: NavPayload): void }>()

interface NavPayload {
  key: string
  label: string
  icon?: string
  panel?: string
  dialog?: string
  title?: string
  open?: (w: ReturnType<typeof useWorkspaceStore>) => void
}

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

// 导航树: 分组语义对齐 VShell/Vben(资产/基础设施/载荷/运维/系统/报告)。
// panel = 打开标签面板(经 panelRegistry);dialog = 打开对话框;open = 自定义编排。
const navGroups = computed<{ label: string; items: NavPayload[] }[]>(() => [
  {
    label: isZh.value ? '总览' : 'Overview',
    items: [
      { key: 'vs-dashboard', label: isZh.value ? '仪表盘' : 'Dashboard', icon: 'Odometer', panel: 'vs-dashboard' },
    ],
  },
  {
    label: isZh.value ? '资产' : 'Assets',
    items: [
      { key: 'session-table', label: isZh.value ? '会话管理' : 'Sessions', icon: 'Monitor', panel: 'session-table' },
      { key: 'targets', label: isZh.value ? '目标列表' : 'Targets', icon: 'Aim', panel: 'targets' },
      { key: 'vs-topology', label: isZh.value ? '拓扑图谱' : 'Topology', icon: 'Share', panel: 'vs-topology' },
      { key: 'credentials', label: isZh.value ? '密码凭证' : 'Credentials', icon: 'Key', panel: 'credentials' },
      { key: 'applications', label: isZh.value ? '应用程序' : 'Applications', icon: 'Grid', panel: 'applications' },
    ],
  },
  {
    label: isZh.value ? '基础设施' : 'Infrastructure',
    items: [
      { key: 'listeners', label: isZh.value ? '监听管理' : 'Listeners', icon: 'Connection', panel: 'listeners' },
      { key: 'forward-listeners', label: isZh.value ? '正向监听' : 'Forward', icon: 'Sort', panel: 'forward-listeners' },
      { key: 'webdelivery', label: isZh.value ? '站点投递' : 'WebDelivery', icon: 'Link', panel: 'webdelivery' },
      { key: 'sites', label: isZh.value ? '站点管理' : 'Sites', icon: 'Files', panel: 'sites' },
      { key: 'tunnel-proxy', label: isZh.value ? '隧道代理' : 'Tunnels', icon: 'Guide', panel: 'tunnel-proxy' },
      { key: 'socks5', label: isZh.value ? 'SOCKS5 代理' : 'SOCKS5', icon: 'Switch', panel: 'socks5' },
    ],
  },
  {
    label: isZh.value ? '载荷' : 'Payloads',
    items: [
      { key: 'server-gen', label: isZh.value ? '客户端生成' : 'Generate', icon: 'MagicStick', panel: 'server-gen' },
      { key: 'dlg-payloadGen', label: isZh.value ? '载荷生成' : 'Payload', icon: 'Box', dialog: 'payloadGen' },
      { key: 'dlg-windowsExe', label: isZh.value ? 'Windows EXE' : 'Windows EXE', icon: 'Platform', dialog: 'windowsExe' },
      { key: 'dlg-windowsExeStageless', label: isZh.value ? 'Windows 无阶段' : 'Win Stageless', icon: 'Platform', dialog: 'windowsExeStageless' },
      { key: 'dlg-htaAttack', label: isZh.value ? 'HTA 攻击' : 'HTA Attack', icon: 'Document', dialog: 'htaAttack' },
      { key: 'dlg-officeMacro', label: isZh.value ? 'Office 宏' : 'Office Macro', icon: 'Document', dialog: 'officeMacro' },
      { key: 'dlg-scriptedWebDelivery', label: isZh.value ? '脚本化投递' : 'Scripted Delivery', icon: 'Link', dialog: 'scriptedWebDelivery' },
      { key: 'dlg-cloneSite', label: isZh.value ? '克隆站点' : 'Clone Site', icon: 'CopyDocument', dialog: 'cloneSite' },
      { key: 'dlg-hostFile', label: isZh.value ? '文件托管' : 'Host File', icon: 'Upload', dialog: 'hostFile' },
      { key: 'dlg-spearPhish', label: isZh.value ? '钓鱼邮件' : 'Spear Phish', icon: 'Message', dialog: 'spearPhish' },
    ],
  },
  {
    label: isZh.value ? '运维' : 'Operations',
    items: [
      { key: 'event-log', label: isZh.value ? '事件日志' : 'Event Log', icon: 'Bell', panel: 'event-log' },
      { key: 'web-log', label: isZh.value ? '投递日志' : 'Delivery Log', icon: 'Tickets', panel: 'web-log' },
      { key: 'global-downloads', label: isZh.value ? '全局下载' : 'Downloads', icon: 'Download', panel: 'global-downloads' },
      { key: 'global-screenshots', label: isZh.value ? '屏幕截图' : 'Screenshots', icon: 'Camera', panel: 'global-screenshots' },
      { key: 'global-keystrokes', label: isZh.value ? '键盘记录' : 'Keystrokes', icon: 'EditPen', panel: 'global-keystrokes' },
      { key: 'batch-tasks', label: isZh.value ? '批量任务' : 'Batch Tasks', icon: 'List', panel: 'batch-tasks' },
      { key: 'plugins', label: isZh.value ? '插件中心' : 'Plugins', icon: 'Suitcase', panel: 'plugins' },
      { key: 'script-manager', label: isZh.value ? '脚本管理器' : 'Script Manager', icon: 'FolderOpened', panel: 'script-manager' },
      { key: 'script-console', label: isZh.value ? '脚本控制台' : 'Script Console', icon: 'Cpu', panel: 'script-console' },
    ],
  },
  {
    label: isZh.value ? '系统' : 'System',
    items: [
      { key: 'server-status', label: isZh.value ? '服务器状态' : 'Server Status', icon: 'DataLine', panel: 'server-status' },
      { key: 'security-center', label: isZh.value ? '安全中心' : 'Security', icon: 'Lock', panel: 'security-center' },
      { key: 'hidden-access', label: isZh.value ? '隐藏访问' : 'Hidden Access', icon: 'Hide', panel: 'hidden-access' },
      { key: 'key-management', label: isZh.value ? '密钥管理' : 'Keys', icon: 'Key', panel: 'key-management' },
      { key: 'session-keys', label: isZh.value ? '会话密钥' : 'Session Keys', icon: 'Unlock', panel: 'session-keys' },
      { key: 'user-management', label: isZh.value ? '用户管理' : 'Users', icon: 'User', panel: 'user-management' },
      { key: 'server-config', label: isZh.value ? '服务配置' : 'Config', icon: 'Setting', panel: 'server-config' },
      { key: 'audit', label: isZh.value ? '审计日志' : 'Audit', icon: 'Notebook', panel: 'audit' },
      { key: 'connection-pool', label: isZh.value ? '连接池' : 'Connections', icon: 'Coin', panel: 'connection-pool' },
    ],
  },
  {
    label: isZh.value ? '报告' : 'Reporting',
    items: [
      { key: 'dlg-reports', label: isZh.value ? '报告中心' : 'Reports', icon: 'DataAnalysis', dialog: 'reports' },
      { key: 'dlg-c2Profile', label: isZh.value ? 'C2 扩展配置' : 'C2 Profile', icon: 'SetUp', dialog: 'c2Profile' },
    ],
  },
])
</script>
