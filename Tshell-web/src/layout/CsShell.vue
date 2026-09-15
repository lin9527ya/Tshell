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
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
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

onMounted(() => {
  eventsStore.connect()

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
})
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
