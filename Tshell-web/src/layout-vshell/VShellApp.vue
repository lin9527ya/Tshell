<template>
  <div class="vs-shell cs-shell cs-font-ui">
    <!-- ============ 侧边导航 ============ -->
    <VsSidebar
      :collapsed="sidebarCollapsed"
      :active-key="activeNavKey"
      @navigate="onNavigate"
    />

    <!-- ============ 主区 ============ -->
    <div class="vs-main">
      <!-- 顶栏 -->
      <div class="vs-header">
        <button class="vs-hamburger" :title="isZh ? '折叠/展开侧栏' : 'Collapse sidebar'" @click="sidebarCollapsed = !sidebarCollapsed">
          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
            <line x1="3" y1="6" x2="21" y2="6" /><line x1="3" y1="12" x2="21" y2="12" /><line x1="3" y1="18" x2="21" y2="18" />
          </svg>
        </button>

        <span class="vs-crumb">
          <b>{{ currentTitle }}</b>
        </span>

        <div class="vs-header-spacer" />

        <span class="vs-header-stat" :title="isZh ? '在线会话' : 'Online sessions'">
          <span class="dot" />{{ onlineCount }} {{ isZh ? '在线' : 'online' }}
        </span>

        <button class="vs-header-btn" :title="isZh ? '切换语言' : 'Switch language'" @click="i18nStore.toggleLocale()">
          {{ isZh ? '中' : 'EN' }}
        </button>
        <button class="vs-header-btn" :title="isZh ? '切换主题' : 'Toggle theme'" @click="toggleTheme">
          {{ isDark ? '☀' : '☾' }}
        </button>

        <span class="vs-user" :title="userStore.username || 'admin'">
          <span class="avatar">{{ (userStore.username || 'A').slice(0, 1).toUpperCase() }}</span>
          {{ userStore.username || 'admin' }}
          <el-dropdown trigger="click" @command="onUserCmd">
            <span class="vs-header-btn" style="margin-left:2px">▾</span>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="preferences">{{ isZh ? '偏好设置' : 'Preferences' }}</el-dropdown-item>
                <el-dropdown-item command="about">{{ isZh ? '关于' : 'About' }}</el-dropdown-item>
                <el-dropdown-item command="logout" divided>{{ isZh ? '断开连接' : 'Disconnect' }}</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </span>
      </div>

      <!-- 标签条 -->
      <div class="vs-tags">
        <div
          v-for="tab in workspace.tabs"
          :key="tab.id"
          class="vs-tag"
          :class="{ active: tab.id === workspace.activeTabId }"
          @click="workspace.setActiveTab(tab.id)"
        >
          <span>{{ tab.title }}</span>
          <span
            v-if="tab.closable !== false"
            class="vs-tag-close"
            @click.stop="workspace.closeTab(tab.id)"
          >✕</span>
        </div>
      </div>

      <!-- 内容区(面板宿主) -->
      <div class="vs-content">
        <div class="vs-panel-host">
          <KeepAlive>
            <component :is="hostComponent" :key="hostKey" />
          </KeepAlive>
        </div>
      </div>
    </div>

    <!-- ============ 对话框(与 CsShell 同款平铺挂载,契约要求) ============ -->
    <NoteDialog />
    <SleepDialog />
    <ColorDialog />
    <BinExecuteDialog />
    <ClientReportDialog />
    <PreferencesDialog />
    <ConnectDialog />
    <AboutDialog />
    <ScriptManagerDialog />
    <ProcessMigrateDialog />
    <ReportsDialog />

    <!-- Payload Generator Dialogs -->
    <HtaAttackDialog v-model="workspace.dialogs.htaAttack" />
    <OfficeMacroDialog v-model="workspace.dialogs.officeMacro" />
    <PayloadGenDialog v-model="workspace.dialogs.payloadGen" />
    <PayloadGenStagelessDialog
      :model-value="workspace.dialogs.payloadGenStageless.visible"
      @update:model-value="(v: boolean) => { if (!v) workspace.closeDialog('payloadGenStageless') }"
    />
    <WindowsExeDialog v-model="workspace.dialogs.windowsExe" />
    <WindowsExeStagelessDialog v-model="workspace.dialogs.windowsExeStageless" />
    <WindowsStagelessAllDialog v-model="workspace.dialogs.windowsStagelessAll" />

    <!-- Attack Dialogs -->
    <ForwardConnectDialog v-model="workspace.dialogs.forwardConnect" />
    <SpearPhishDialog v-model="workspace.dialogs.spearPhish" />
    <C2ProfileDialog v-model="workspace.dialogs.c2Profile" />

    <!-- Site Management Dialogs -->
    <CloneSiteDialog v-model="workspace.dialogs.cloneSite" />
    <HostFileDialog v-model="workspace.dialogs.hostFile" />
    <WebDeliveryDialog v-model="workspace.dialogs.scriptedWebDelivery" />
  </div>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onBeforeUnmount, onMounted, provide, ref } from 'vue'
import VsSidebar from './VsSidebar.vue'
import VsDashboard from './VsDashboard.vue'
import PivotGraph from '../layout/PivotGraph.vue'
import { getPanelComponent, registerPanel } from '../layout/Workspace/panelRegistry'
import './vshell-theme.css'

// Dialogs(与 CsShell 相同组件,统一从 layout/Dialogs 引入)
import NoteDialog from '../layout/Dialogs/NoteDialog.vue'
import SleepDialog from '../layout/Dialogs/SleepDialog.vue'
import ColorDialog from '../layout/Dialogs/ColorDialog.vue'
import BinExecuteDialog from '../layout/Dialogs/BinExecuteDialog.vue'
import ClientReportDialog from '../layout/Dialogs/ClientReportDialog.vue'
import PreferencesDialog from '../layout/Dialogs/PreferencesDialog.vue'
import ConnectDialog from '../layout/Dialogs/ConnectDialog.vue'
import AboutDialog from '../layout/Dialogs/AboutDialog.vue'
import ScriptManagerDialog from '../layout/Dialogs/ScriptManagerDialog.vue'
import ProcessMigrateDialog from '../layout/Dialogs/ProcessMigrateDialog.vue'
import ReportsDialog from '../layout/Dialogs/ReportsDialog.vue'
import C2ProfileDialog from '../layout/Dialogs/C2ProfileDialog.vue'
import HtaAttackDialog from '../layout/Dialogs/Payloads/HtaAttackDialog.vue'
import OfficeMacroDialog from '../layout/Dialogs/Payloads/OfficeMacroDialog.vue'
import PayloadGenDialog from '../layout/Dialogs/Payloads/PayloadGenDialog.vue'
import PayloadGenStagelessDialog from '../layout/Dialogs/Payloads/PayloadGenStagelessDialog.vue'
import WindowsExeDialog from '../layout/Dialogs/Payloads/WindowsExeDialog.vue'
import WindowsExeStagelessDialog from '../layout/Dialogs/Payloads/WindowsExeStagelessDialog.vue'
import WindowsStagelessAllDialog from '../layout/Dialogs/Payloads/WindowsStagelessAllDialog.vue'
import ForwardConnectDialog from '../layout/Dialogs/Attacks/ForwardConnectDialog.vue'
import CloneSiteDialog from '../layout/Dialogs/CloneSiteDialog.vue'
import HostFileDialog from '../layout/Dialogs/HostFileDialog.vue'
import WebDeliveryDialog from '../layout/Dialogs/Attacks/WebDeliveryDialog.vue'

import { useWorkspaceStore } from '@/stores/workspace'
import { useEventsStore } from '@/stores/events'
import { useI18nStore } from '@/stores/i18n'
import { useThemeStore } from '@/stores/theme'
import { useClientsStore } from '@/stores/clients'
import { useUserStore } from '@/stores/user'

const workspace = useWorkspaceStore()
const eventsStore = useEventsStore()
const i18nStore = useI18nStore()
const themeStore = useThemeStore()
const clientsStore = useClientsStore()
const userStore = useUserStore()

const isZh = computed(() => i18nStore.isZh)
const isDark = computed(() => themeStore.isDark)
const sidebarCollapsed = ref(false)

// 注册 vshell 分支专属面板(不动 panelRegistry 原有键)
registerPanel('vs-dashboard', VsDashboard)
registerPanel('vs-topology', PivotGraph)

const onlineCount = computed(
  () => clientsStore.clientsList.filter((c) => c.Online === '1' || c.Online === 'true').length,
)

const currentTitle = computed(() => workspace.activeTab?.title || (isZh.value ? '仪表盘' : 'Dashboard'))

// 当前导航高亮 = 活动标签的 component 键
const activeNavKey = computed(() => workspace.activeTab?.component || 'vs-dashboard')

// 面板宿主: provide('realUid') 契约(CsTabContainer 同款),保证会话类面板可用
const PanelHost = defineComponent({
  name: 'VsPanelHost',
  setup() {
    const uidRef = computed(() => workspace.activeTab?.params?.uid || '')
    provide('realUid', uidRef)
    return () => {
      const tab = workspace.activeTab
      if (!tab) return h('div', { class: 'panel-not-found' }, isZh.value ? '无打开的面板' : 'No open panel')
      const comp = getPanelComponent(tab.component)
      if (!comp) return h('div', { class: 'panel-not-found' }, `Panel component [${tab.component}] not registered`)
      return h(comp, { key: tab.id, ...(tab.params || {}) })
    }
  },
})
const hostComponent = PanelHost
const hostKey = computed(() => workspace.activeTabId || 'empty')

function onNavigate(payload: { panel?: string; dialog?: string; title?: string; open?: (w: typeof workspace) => void }) {
  if (payload.open) { payload.open(workspace); return }
  if (payload.panel) {
    workspace.openTab({
      id: payload.panel,
      title: payload.title || payload.panel,
      component: payload.panel,
    })
  } else if (payload.dialog) {
    workspace.openDialog(payload.dialog as any)
  }
}

function toggleTheme() {
  themeStore.setTheme(isDark.value ? 'light' : 'dark')
}

function onUserCmd(cmd: string) {
  if (cmd === 'preferences') workspace.openDialog('preferences')
  else if (cmd === 'about') workspace.openDialog('about')
  else if (cmd === 'logout') userStore.logout()
}

onMounted(() => {
  eventsStore.connect()
  // 默认页签: 仪表盘(closable:false)
  if (!workspace.tabs.length) {
    workspace.openTab({
      id: 'vs-dashboard',
      title: isZh.value ? '仪表盘' : 'Dashboard',
      component: 'vs-dashboard',
      closable: false,
    })
  }
})

onBeforeUnmount(() => {
  eventsStore.disconnect()
})
</script>
