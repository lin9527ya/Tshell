import { defineAsyncComponent, markRaw } from 'vue'
import type { Component } from 'vue'
import EventLogPanel from '../EventLogPanel.vue'
import BeaconConsole from '../BeaconConsole.vue'
import ScriptConsole from '../ScriptConsole.vue'
import CredentialsView from '../CredentialsView.vue'
import GlobalDownloadsView from '../GlobalDownloadsView.vue'
import GlobalScreenshotsView from '../GlobalScreenshotsView.vue'
import GlobalKeystrokesView from '../GlobalKeystrokesView.vue'
import WebLogPanel from '../WebLogPanel.vue'
import BatchTasksView from '../BatchTasksView.vue'

const panelRegistry: Record<string, Component> = {
  // Core Cobalt Strike Panels
  'event-log': markRaw(EventLogPanel),
  'beacon': markRaw(BeaconConsole),
  'script-console': markRaw(ScriptConsole),
  'web-log': markRaw(WebLogPanel),
  'credentials': markRaw(CredentialsView),
  'global-downloads': markRaw(GlobalDownloadsView),
  'global-screenshots': markRaw(GlobalScreenshotsView),
  'global-keystrokes': markRaw(GlobalKeystrokesView),
  'batch-tasks': markRaw(BatchTasksView),
  'applications': defineAsyncComponent(() => import('../ApplicationsView.vue')),
  'targets': defineAsyncComponent(() => import('../TargetsView.vue')),
  'session-table': defineAsyncComponent(() => import('../SessionTable.vue')),
  'sites': defineAsyncComponent(() => import('../SitesView.vue')),

  // Beacon Exploration Panels
  'client-files': defineAsyncComponent(() => import('@/views/client/View/ClientFiles.vue')),
  'client-pids': defineAsyncComponent(() => import('@/views/client/View/ClientPid.vue')),
  'client-screenshots': defineAsyncComponent(() => import('@/views/client/View/ClientScreenshot.vue')),
  'client-downloads': defineAsyncComponent(() => import('@/views/client/View/ClientDownloads.vue')),
  'client-monitor': defineAsyncComponent(() => import('@/views/client/View/ClientMonitor.vue')),
  'client-tools': defineAsyncComponent(() => import('@/views/client/View/ClientTools.vue')),
  'client-terminal': defineAsyncComponent(() => import('@/views/client/View/LinuxInteractiveTerminal.vue')),
  'client-startup': defineAsyncComponent(() => import('@/views/client/View/ClientStartup.vue')),

  // Listeners & Attacks
  'listeners': defineAsyncComponent(() => import('@/views/listener/listener.vue')),
  'forward-listeners': defineAsyncComponent(() => import('@/views/listener/ForwardListeners.vue')),
  'webdelivery': defineAsyncComponent(() => import('@/views/server/WebDelivery.vue')),
  'server-gen': defineAsyncComponent(() => import('@/views/server/ServerGen.vue')),
  'tunnel-proxy': defineAsyncComponent(() => import('@/views/tunnel/TunnelProxy.vue')),
  'socks5': defineAsyncComponent(() => import('@/views/tunnel/Socks5View.vue')),
  'plugins': defineAsyncComponent(() => import('@/views/plugins/PluginsView.vue')),
  'script-manager': defineAsyncComponent(() => import('../ScriptManagerPanel.vue')),
  'audit': defineAsyncComponent(() => import('@/views/server/AuditLogsView.vue')),
  'security-center': defineAsyncComponent(() => import('@/views/security/SecurityCenterView.vue')),
  'hidden-access': defineAsyncComponent(() => import('@/views/security/HiddenAccessView.vue')),
  'key-management': defineAsyncComponent(() => import('@/views/config/KeyManagementView.vue')),
  'session-keys': defineAsyncComponent(() => import('@/views/config/KeyManagementView.vue')),
  'user-management': defineAsyncComponent(() => import('@/views/config/UserManagementView.vue')),
  'server-config': defineAsyncComponent(() => import('@/views/config/ServerConfigView.vue')),
  'server-status': defineAsyncComponent(() => import('@/views/server/ServerStatusView.vue')),
  'connection-pool': defineAsyncComponent(() => import('@/views/connection/ConnectionPoolView.vue')),
}

export function registerPanel(name: string, component: Component) {
  panelRegistry[name] = markRaw(component)
}

export function getPanelComponent(name: string): Component | undefined {
  return panelRegistry[name]
}

export default panelRegistry
