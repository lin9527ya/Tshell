<template>
  <div class="cs-shell cs-font-ui">
    <!-- 1. Top Menu Bar -->
    <MenuBar />

    <!-- 2. Cobalt Strike Icon Toolbar -->
    <Toolbar />

    <!-- 3. Main Split-Pane Workspace -->
    <div ref="workspaceEl" class="cs-main-workspace">
      <!-- Upper Section (Beacons / Pivot Graph / Targets) -->
      <div
        v-show="!isTopCollapsed"
        class="cs-top-pane"
        :style="topPaneStyle"
      >
        <keep-alive>
          <SessionTable v-if="topViewMode === 'table'" key="session-table" />
          <PivotGraph v-else-if="topViewMode === 'graph'" key="pivot-graph" />
          <TargetsView v-else-if="topViewMode === 'targets'" key="targets-view" />
        </keep-alive>
      </div>

      <!-- Splitter Divider -->
      <Splitter :container-el="workspaceEl" />

      <!-- Lower Section (Multi-Tabbed Work Area) -->
      <div
        v-show="!isBottomCollapsed"
        class="cs-bottom-pane"
        :style="bottomPaneStyle"
      >
        <CsTabContainer />
      </div>
    </div>

    <!-- 4. Bottom Status Bar -->
    <StatusBar />

    <!-- 5. Cobalt Strike Dialogs -->
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

    <!-- Payload Generator Dialogs (Screenshots 4-10) -->
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

    <!-- Attack Dialogs (Screenshots 11-15) -->
    <ForwardConnectDialog v-model="workspace.dialogs.forwardConnect" />
    <SpearPhishDialog v-model="workspace.dialogs.spearPhish" />
    <C2ProfileDialog v-model="workspace.dialogs.c2Profile" />

    <!-- Site Management Dialogs (Pictures 6-7) -->
    <CloneSiteDialog v-model="workspace.dialogs.cloneSite" />
    <HostFileDialog v-model="workspace.dialogs.hostFile" />
    <WebDeliveryDialog v-model="workspace.dialogs.scriptedWebDelivery" />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import MenuBar from './MenuBar/MenuBar.vue'
import Toolbar from './Toolbar/Toolbar.vue'
import Splitter from './Splitter.vue'
import SessionTable from './SessionTable.vue'
import PivotGraph from './PivotGraph.vue'
import TargetsView from './TargetsView.vue'
import CsTabContainer from './TabContainer/CsTabContainer.vue'
import StatusBar from './StatusBar/StatusBar.vue'

// Dialogs
import NoteDialog from './Dialogs/NoteDialog.vue'
import SleepDialog from './Dialogs/SleepDialog.vue'
import ColorDialog from './Dialogs/ColorDialog.vue'
import BinExecuteDialog from './Dialogs/BinExecuteDialog.vue'
import ClientReportDialog from './Dialogs/ClientReportDialog.vue'
import PreferencesDialog from './Dialogs/PreferencesDialog.vue'
import ConnectDialog from './Dialogs/ConnectDialog.vue'
import AboutDialog from './Dialogs/AboutDialog.vue'
import ScriptManagerDialog from './Dialogs/ScriptManagerDialog.vue'
import ProcessMigrateDialog from './Dialogs/ProcessMigrateDialog.vue'
import ReportsDialog from './Dialogs/ReportsDialog.vue'
import C2ProfileDialog from './Dialogs/C2ProfileDialog.vue'

// Payloads
import HtaAttackDialog from './Dialogs/Payloads/HtaAttackDialog.vue'
import OfficeMacroDialog from './Dialogs/Payloads/OfficeMacroDialog.vue'
import PayloadGenDialog from './Dialogs/Payloads/PayloadGenDialog.vue'
import PayloadGenStagelessDialog from './Dialogs/Payloads/PayloadGenStagelessDialog.vue'
import WindowsExeDialog from './Dialogs/Payloads/WindowsExeDialog.vue'
import WindowsExeStagelessDialog from './Dialogs/Payloads/WindowsExeStagelessDialog.vue'
import WindowsStagelessAllDialog from './Dialogs/Payloads/WindowsStagelessAllDialog.vue'

// Attacks
import ForwardConnectDialog from './Dialogs/Attacks/ForwardConnectDialog.vue'
import CloneSiteDialog from './Dialogs/CloneSiteDialog.vue'
import HostFileDialog from './Dialogs/HostFileDialog.vue'
import WebDeliveryDialog from './Dialogs/Attacks/WebDeliveryDialog.vue'

import { useWorkspaceStore } from '@/stores/workspace'
import { useEventsStore } from '@/stores/events'
import { useI18nStore } from '@/stores/i18n'

const workspace = useWorkspaceStore()
const eventsStore = useEventsStore()
const i18nStore = useI18nStore()
const workspaceEl = ref<HTMLElement | null>(null)

const isZh = computed(() => i18nStore.isZh)

const topViewMode = computed(() => workspace.topViewMode)
const isTopCollapsed = computed(() => workspace.isTopCollapsed)
const isBottomCollapsed = computed(() => workspace.isBottomCollapsed)

const topPaneStyle = computed(() => {
  if (isBottomCollapsed.value) {
    return { flex: '1 1 100%', height: '100%' }
  }
  return {
    flex: `0 0 ${workspace.splitRatio}%`,
    height: `${workspace.splitRatio}%`,
  }
})

const bottomPaneStyle = computed(() => {
  if (isTopCollapsed.value) {
    return { flex: '1 1 100%', height: '100%' }
  }
  const botRatio = 100 - workspace.splitRatio
  return {
    flex: `0 0 ${botRatio}%`,
    height: `${botRatio}%`,
  }
})

watchDialogOrder() // 同步注册(watch 需在 setup 同步上下文)

onMounted(() => {
  eventsStore.connect()
  window.addEventListener('keydown', onGlobalKeydown)

  // 默认打开 Event Log 标签页
  if (!workspace.tabs.length) {
    workspace.openTab({
      id: 'event-log',
      title: isZh.value ? '事件日志' : 'Event Log',
      component: 'event-log',
      closable: false,
    })
  }
})

onBeforeUnmount(() => {
  eventsStore.disconnect()
  window.removeEventListener('keydown', onGlobalKeydown)
})

// ================= 全局快捷键 =================
// 说明: Chrome 会保留 Ctrl+W/Ctrl+T/Ctrl+Tab/Ctrl+1-9/Ctrl+N(不可拦截),
// 因此同时绑定可拦截的 Alt 变体;两者都注册,浏览器允许哪个就哪个生效。
// 输入区(含 xterm 终端)内除 Esc 外一律不抢占按键。

// 对话框可见性读取(bool 型或 {visible} 对象型两种)
function dialogVisible(v: unknown): boolean {
  if (v === true) return true
  if (v && typeof v === 'object' && 'visible' in (v as Record<string, unknown>)) {
    return !!(v as Record<string, unknown>).visible
  }
  return false
}

// 打开顺序栈(用于 Esc 关"最上层"对话框)
const dialogOrder: string[] = []
function watchDialogOrder() {
  watch(
    () => workspace.dialogs,
    (d) => {
      const openKeys = Object.keys(d).filter((k) => dialogVisible((d as Record<string, unknown>)[k]))
      // 移除已关闭的,保持原有相对顺序
      for (let i = dialogOrder.length - 1; i >= 0; i--) {
        if (!openKeys.includes(dialogOrder[i])) dialogOrder.splice(i, 1)
      }
      // 追加新打开的
      for (const k of openKeys) {
        if (!dialogOrder.includes(k)) dialogOrder.push(k)
      }
    },
    { deep: true },
  )
}

function closeTopDialog(): boolean {
  const top = dialogOrder[dialogOrder.length - 1]
  if (!top) return false
  workspace.closeDialog(top as never)
  return true
}

function cycleTab(delta: number) {
  const tabs = workspace.tabs
  if (tabs.length < 2) return
  const idx = tabs.findIndex((t) => t.id === workspace.activeTabId)
  const next = (idx + delta + tabs.length) % tabs.length
  workspace.setActiveTab(tabs[next].id)
}

function isEditableTarget(t: EventTarget | null): boolean {
  const el = t as HTMLElement | null
  if (!el || typeof el.tagName !== 'string') return false
  const tag = el.tagName.toLowerCase()
  if (tag === 'input' || tag === 'textarea' || tag === 'select') return true
  if (el.isContentEditable) return true
  return typeof el.closest === 'function' && !!el.closest('.xterm')
}

function onGlobalKeydown(e: KeyboardEvent) {
  // Esc: 关闭最上层对话框(输入区中也生效);菜单类 Esc 由各自组件处理
  if (e.key === 'Escape') {
    if (closeTopDialog()) e.preventDefault()
    return
  }
  if (isEditableTarget(e.target)) return // 输入区(含终端)不抢键

  const ctrl = e.ctrlKey || e.metaKey
  const alt = e.altKey

  // 关闭当前标签: Alt+W(可靠) / Ctrl+W(浏览器允许时)
  if ((alt && !ctrl && e.key.toLowerCase() === 'w') || (ctrl && !alt && e.key.toLowerCase() === 'w')) {
    const tab = workspace.activeTab
    if (tab && tab.closable !== false) {
      e.preventDefault()
      workspace.closeTab(tab.id)
    }
    return
  }
  // 标签循环: Ctrl+Alt+←/→(可靠) / Ctrl+Tab(浏览器允许时)
  if ((ctrl && alt && (e.key === 'ArrowLeft' || e.key === 'ArrowRight')) || (ctrl && e.key === 'Tab')) {
    e.preventDefault()
    cycleTab(e.key === 'ArrowLeft' || e.shiftKey ? -1 : 1)
    return
  }
  // 跳转第 N 个标签: Alt+1..9(Chrome 保留 Ctrl+1..9)
  if (alt && !ctrl && /^[1-9]$/.test(e.key)) {
    const idx = parseInt(e.key, 10) - 1
    const tabs = workspace.tabs
    if (idx < tabs.length) {
      e.preventDefault()
      workspace.setActiveTab(tabs[idx].id)
    }
    return
  }
  // 会话搜索: Ctrl+F
  if (ctrl && !alt && e.key.toLowerCase() === 'f') {
    e.preventDefault()
    workspace.setTopViewMode('table')
    workspace.isTopCollapsed = false
    window.dispatchEvent(new CustomEvent('cs-focus-session-search'))
    return
  }
  // 新建连接: Alt+N(Chrome 保留 Ctrl+N)
  if (alt && !ctrl && e.key.toLowerCase() === 'n') {
    e.preventDefault()
    workspace.openDialog('connect')
    return
  }
}
</script>

<style scoped>
.cs-shell {
  display: flex;
  flex-direction: column;
  height: 100vh;
  width: 100vw;
  overflow: hidden;
  background: var(--cs-bg);
  color: var(--cs-text);
}

.cs-main-workspace {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  position: relative;
  overflow: hidden;
  background: var(--cs-bg);
}

.cs-top-pane {
  min-height: 80px;
  position: relative;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.cs-bottom-pane {
  min-height: 80px;
  position: relative;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}
</style>
