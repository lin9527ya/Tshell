<template>
  <div ref="menuBarRef" class="cs-menubar cs-font-ui">
    <!-- Top Level Menus (Click to Toggle / Click to Hide) -->
    <div
      v-for="(menu, mIdx) in menus"
      :key="menu.label"
      class="cs-menu-top-wrap"
      :class="{ 'is-open': activeMenuIndex === mIdx }"
    >
      <button
        class="cs-menu-top-btn"
        :class="{ active: activeMenuIndex === mIdx }"
        @click.stop="onTopMenuClick(mIdx)"
        @mouseenter="onTopMenuHover(mIdx)"
      >
        {{ menu.label }}
      </button>

      <!-- Level 1 Dropdown Menu -->
      <div
        v-if="activeMenuIndex === mIdx"
        class="cs-dropdown-menu"
        @mousedown.stop
      >
        <template v-for="(item, idx) in menu.items" :key="idx">
          <!-- Divider -->
          <div v-if="item.divider" class="cs-dropdown-divider" />

          <!-- Submenu Item (Hover to show sub) -->
          <div
            v-else-if="item.children && item.children.length"
            class="cs-dropdown-item has-sub"
            :class="{ 'sub-active': activeSubIndex === idx, disabled: item.disabled }"
            @mouseenter="activeSubIndex = idx"
            @mouseleave="onSubItemLeave(idx)"
          >
            <span class="mi-label">{{ item.label }}</span>
            <span class="mi-arrow">▶</span>

            <!-- Level 2 Submenu -->
            <div
              v-show="activeSubIndex === idx"
              class="cs-sub-dropdown-menu"
            >
              <div
                v-for="(sub, subIdx) in item.children"
                :key="subIdx"
                class="cs-dropdown-item"
                :class="{ disabled: sub.disabled }"
                @click.stop="handleItemClick(sub)"
              >
                <span class="mi-label">{{ sub.label }}</span>
                <span class="mi-key">{{ sub.key || '' }}</span>
              </div>
            </div>
          </div>

          <!-- Normal Clickable Item -->
          <div
            v-else
            class="cs-dropdown-item"
            :class="{ disabled: item.disabled }"
            @mouseenter="activeSubIndex = null"
            @click.stop="handleItemClick(item)"
          >
            <span class="mi-label">{{ item.label }}</span>
            <span class="mi-key">{{ item.key || '' }}</span>
          </div>
        </template>
      </div>
    </div>

    <div class="mb-spacer" />

    <!-- Right Side Tools: Fullscreen + Lang Switch + Operator Badge + Theme Switch -->
    <div class="mb-right-tools">
      <!-- Fullscreen Toggle Button -->
      <button
        class="mb-fullscreen-btn"
        :title="isFullscreen ? (isZh ? '退出全屏' : 'Exit Fullscreen') : (isZh ? '全屏显示' : 'Fullscreen')"
        @click="toggleFullscreen"
      >
        <svg v-if="!isFullscreen" viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M8 3H5a2 2 0 0 0-2 2v3m18 0V5a2 2 0 0 0-2-2h-3m0 18h3a2 2 0 0 0 2-2v-3M3 16v3a2 2 0 0 0 2 2h3" />
        </svg>
        <svg v-else viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M8 3v3a2 2 0 0 1-2 2H3m18 0h-3a2 2 0 0 1-2-2V3m0 18v-3a2 2 0 0 1 2-2h3M3 16h3a2 2 0 0 1 2 2v3" />
        </svg>
      </button>

      <!-- Language Switcher Button -->
      <button
        class="mb-lang-btn"
        :title="isZh ? '切换为英文' : 'Switch to Chinese'"
        @click="toggleLang"
      >
        <span class="lang-text" :class="{ active: isZh }">中</span>
        <span class="lang-sep">/</span>
        <span class="lang-text" :class="{ active: !isZh }">EN</span>
      </button>

      <!-- Operator Status Badge -->
      <span class="operator-badge" :title="userStore.username || 'admin'">
        <span class="op-dot" />
        {{ userStore.username || 'admin' }}
      </span>

      <!-- Theme Switcher -->
      <button
        class="mb-theme"
        :title="isDark ? 'Switch to Light Theme' : 'Switch to Dark Theme'"
        @click="toggleTheme"
      >
        {{ isDark ? '☀' : '☾' }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useWorkspaceStore } from '@/stores/workspace'
import { useUserStore } from '@/stores/user'
import { useThemeStore } from '@/stores/theme'
import { useClientsStore } from '@/stores/clients'
import { useI18nStore } from '@/stores/i18n'
import { ElMessage } from 'element-plus'

const workspace = useWorkspaceStore()
const userStore = useUserStore()
const themeStore = useThemeStore()
const clientsStore = useClientsStore()
const i18nStore = useI18nStore()

const isDark = computed(() => themeStore.isDark)
const isZh = computed(() => i18nStore.isZh)
const t = (key: string, def?: string) => i18nStore.t(key, def)

const menuBarRef = ref<HTMLElement | null>(null)
const activeMenuIndex = ref<number | null>(null)
const activeSubIndex = ref<number | null>(null)

const isFullscreen = ref(false)

function checkFullscreen() {
  isFullscreen.value = !!document.fullscreenElement
}

function toggleFullscreen() {
  if (!document.fullscreenElement) {
    document.documentElement.requestFullscreen().catch(() => {})
  } else {
    document.exitFullscreen().catch(() => {})
  }
}

onMounted(() => {
  document.addEventListener('fullscreenchange', checkFullscreen)
})

onBeforeUnmount(() => {
  document.removeEventListener('fullscreenchange', checkFullscreen)
})

function onTopMenuClick(index: number) {
  // 点击当前已打开的菜单 -> 隐藏/关闭；点击关闭的菜单 -> 打开
  if (activeMenuIndex.value === index) {
    activeMenuIndex.value = null
    activeSubIndex.value = null
  } else {
    activeMenuIndex.value = index
    activeSubIndex.value = null
  }
}

function onTopMenuHover(index: number) {
  // 当已有任何一个主菜单处于打开状态时，鼠标滑动切换到相邻主菜单（原生桌面软件体验）
  if (activeMenuIndex.value !== null && activeMenuIndex.value !== index) {
    activeMenuIndex.value = index
    activeSubIndex.value = null
  }
}

function onSubItemLeave(idx: number) {
  if (activeSubIndex.value === idx) {
    activeSubIndex.value = null
  }
}

function closeMenu() {
  activeMenuIndex.value = null
  activeSubIndex.value = null
}

function toggleLang() {
  i18nStore.toggleLocale()
}

function toggleTheme() {
  themeStore.setTheme(isDark.value ? 'light' : 'dark')
}

interface MenuItem {
  label?: string
  key?: string
  divider?: boolean
  disabled?: boolean
  action?: () => void
  children?: MenuItem[]
  tab?: { id: string; title: string; component: string; params?: Record<string, any> }
}

const menus = computed<{ label: string; items: MenuItem[] }[]>(() => [
  // 1. BladeShell
  {
    label: t('menu.cs'),
    items: [
      { label: t('menu.cs.new_conn'), action: () => workspace.openDialog('connect') },
      { label: t('menu.cs.preferences'), action: () => workspace.openDialog('preferences') },
      {
        label: t('menu.cs.visualization'),
        children: [
          { label: t('menu.cs.pivot_graph'), action: () => workspace.setTopViewMode('graph') },
          { label: t('menu.cs.session_table'), action: () => workspace.setTopViewMode('table') },
          { label: t('menu.cs.target_table'), action: () => workspace.setTopViewMode('targets') },
        ],
      },
      { divider: true },
      { label: t('menu.cs.listeners'), tab: { id: 'listeners', title: isZh.value ? '监听器' : 'Listeners', component: 'listeners' } },
      { label: t('menu.cs.vpn_interface'), tab: { id: 'interfaces', title: isZh.value ? 'VPN网卡' : 'Interfaces', component: 'interfaces' } },
      { label: t('menu.cs.c2_profile'), action: () => workspace.openDialog('c2Profile') },
      { divider: true },
      { label: t('menu.cs.script_manager'), tab: { id: 'script-manager', title: isZh.value ? '脚本管理器' : 'Scripts', component: 'script-manager' } },
      { label: t('menu.cs.script_console'), tab: { id: 'script-console', title: isZh.value ? '脚本控制台' : 'Script Console', component: 'script-console' } },
      { divider: true },
      { label: t('menu.cs.disconnect'), action: () => userStore.logout() },
    ],
  },

  // 2. 视图(V)
  {
    label: t('menu.view'),
    items: [
      {
        label: t('menu.view.sessions'),
        action: () => {
          workspace.setTopViewMode('table')
          workspace.isTopCollapsed = false
          workspace.openTab({
            id: 'sessions',
            title: isZh.value ? '会话列表' : 'Beacons',
            component: 'session-table',
          })
        },
      },
      { label: t('menu.view.applications'), tab: { id: 'applications', title: isZh.value ? '应用程序' : 'Applications', component: 'applications' } },
      { label: t('menu.view.credentials'), tab: { id: 'credentials', title: isZh.value ? '密码凭证' : 'Credentials', component: 'credentials' } },
      { label: t('menu.view.downloads'), tab: { id: 'global-downloads', title: isZh.value ? '下载列表' : 'Downloaded Files', component: 'global-downloads' } },
      { label: t('menu.view.event_log'), tab: { id: 'event-log', title: isZh.value ? '事件日志' : 'Event Log', component: 'event-log' } },
      { label: t('menu.view.keystrokes'), tab: { id: 'global-keystrokes', title: isZh.value ? '键盘记录' : 'Keystrokes', component: 'global-keystrokes' } },
      { label: t('menu.view.proxy_pivots'), tab: { id: 'tunnel-proxy', title: isZh.value ? '代理转发' : 'Tunnel Proxy', component: 'tunnel-proxy' } },
      { label: t('menu.view.screenshots'), tab: { id: 'global-screenshots', title: isZh.value ? '屏幕截图' : 'Screenshots', component: 'global-screenshots' } },
      {
        label: t('menu.view.targets'),
        action: () => {
          workspace.setTopViewMode('targets')
          workspace.isTopCollapsed = false
          workspace.openTab({
            id: 'targets',
            title: isZh.value ? '目标列表' : 'Targets',
            component: 'targets',
          })
        },
      },
      { label: t('menu.view.web_log'), tab: { id: 'web-log', title: isZh.value ? 'Web日志' : 'Web Log', component: 'web-log' } },
    ],
  },

  // 3. 有效载荷(P)
  {
    label: t('menu.payload'),
    items: [
      { label: t('menu.payload.gen'), action: () => (workspace.dialogs.payloadGen = true) },
      { label: t('menu.payload.stageless_gen'), action: () => workspace.openDialog('payloadGenStageless', { os: 'windows' }) },
      { label: isZh.value ? 'Linux 程序（无阶段）' : 'Linux Program (Stageless)', action: () => workspace.openDialog('payloadGenStageless', { os: 'linux' }) },
      { label: isZh.value ? 'macOS 程序（无阶段）' : 'macOS Program (Stageless)', action: () => workspace.openDialog('payloadGenStageless', { os: 'darwin' }) },
      { label: t('menu.payload.win_exe'), action: () => (workspace.dialogs.windowsExe = true) },
      { label: t('menu.payload.win_exe_stageless'), action: () => (workspace.dialogs.windowsExeStageless = true) },
      { label: t('menu.payload.win_stageless_all'), action: () => (workspace.dialogs.windowsStagelessAll = true) },
    ],
  },

  // 4. 攻击(A)
  {
    label: t('menu.attacks'),
    items: [
      { label: t('menu.attacks.web_delivery'), tab: { id: 'webdelivery', title: isZh.value ? 'Web投递' : 'Web Delivery', component: 'webdelivery' } },
      { label: isZh.value ? '脚本化 Web 投递 (S)' : 'Scripted Web Delivery (S)', action: () => workspace.openDialog('scriptedWebDelivery') },
      { label: t('menu.attacks.forward_connect'), action: () => workspace.openDialog('forwardConnect') },
      { label: t('menu.attacks.spear_phishing'), action: () => (workspace.dialogs.spearPhish = true) },
    ],
  },

  // 5. 站点管理(M)
  {
    label: t('menu.site'),
    items: [
      { label: t('menu.site.manage'), tab: { id: 'sites', title: isZh.value ? '站点管理' : 'Sites', component: 'sites' } },
      { label: t('menu.site.clone'), action: () => (workspace.dialogs.cloneSite = true) },
      { label: t('menu.site.host_file'), action: () => (workspace.dialogs.hostFile = true) },
    ],
  },

  // 6. 系统配置(S)
  {
    label: t('menu.sysconfig'),
    items: [
      { label: t('menu.sysconfig.security'), tab: { id: 'security-center', title: isZh.value ? '安全中心' : 'Security Center', component: 'security-center' } },
      { label: t('menu.sysconfig.stealth'), tab: { id: 'hidden-access', title: isZh.value ? '隐藏访问' : 'Stealth Access', component: 'hidden-access' } },
      { label: t('menu.sysconfig.audit'), tab: { id: 'audit', title: isZh.value ? '审计日志' : 'Audit Logs', component: 'audit' } },
      { label: t('menu.sysconfig.keys'), tab: { id: 'key-management', title: isZh.value ? '密钥管理' : 'Key Management', component: 'key-management' } },
      { label: t('menu.sysconfig.session_keys'), tab: { id: 'session-keys', title: isZh.value ? '会话密钥' : 'Session Keys', component: 'session-keys', params: { tab: 'session_keys' } } },
      { label: t('menu.sysconfig.users'), tab: { id: 'user-management', title: isZh.value ? '用户管理' : 'User Management', component: 'user-management' } },
      { label: t('menu.sysconfig.config'), tab: { id: 'server-config', title: isZh.value ? '服务配置' : 'Server Config', component: 'server-config' } },
      { label: t('menu.sysconfig.status'), tab: { id: 'server-status', title: isZh.value ? '服务器状态' : 'Server Status', component: 'server-status' } },
      { label: t('menu.sysconfig.pool'), tab: { id: 'connection-pool', title: isZh.value ? '连接池' : 'Connection Pool', component: 'connection-pool' } },
    ],
  },

  // 7. 报告(R)
  {
    label: t('menu.reporting'),
    items: [
      { label: t('menu.reporting.activity'), tab: { id: 'audit', title: isZh.value ? '审计日志' : 'Audit Logs', component: 'audit' } },
      { label: t('menu.reporting.hosts'), action: () => workspace.setTopViewMode('targets') },
      { label: t('menu.reporting.ioc'), action: () => (workspace.dialogs.reports = true) },
      {
        label: t('menu.reporting.sessions'),
        action: () => {
          // 生成选中会话的操作报告（/client/report/:uid：shell 历史 + 会话录像 + 下载记录）
          const uid = workspace.primarySelectedUid
          if (!uid) {
            ElMessage.warning(isZh.value ? '请先在会话表中选择一个 Beacon 会话' : 'Select a beacon session in the table first')
            return
          }
          workspace.openDialog('clientReport', { uid })
        },
      },
      { label: t('menu.reporting.social_eng'), action: () => (workspace.dialogs.reports = true) },
      { label: t('menu.reporting.tactics'), action: () => (workspace.dialogs.reports = true) },
      { divider: true },
      { label: t('menu.reporting.clear_data'), action: () => ElMessage.warning(isZh.value ? '清空数据功能在当前环境已禁用' : 'Reset data disabled') },
      {
        label: t('menu.reporting.export_data'),
        action: () => {
          const exportPayload = {
            exportDate: new Date().toISOString(),
            sessions: clientsStore.clientsList,
          }
          const blob = new Blob([JSON.stringify(exportPayload, null, 2)], { type: 'application/json' })
          const url = URL.createObjectURL(blob)
          const a = document.createElement('a')
          a.href = url
          a.download = `cobaltstrike_export_${Date.now()}.json`
          a.click()
          URL.revokeObjectURL(url)
          ElMessage.success(isZh.value ? '数据已导出' : 'Data exported')
        },
      },
    ],
  },

  // 7. 帮助(H)
  {
    label: t('menu.help'),
    items: [
      { label: t('menu.help.home'), action: () => workspace.openDialog('about') },
      { label: t('menu.help.support'), action: () => workspace.openDialog('about') },
      { label: t('menu.help.arsenal'), tab: { id: 'plugins', title: isZh.value ? '插件中心' : 'Plugins', component: 'plugins' } },
      { label: t('menu.help.community'), action: () => workspace.openDialog('scriptManager') },
      { label: t('menu.help.sleep_manual'), tab: { id: 'script-console', title: isZh.value ? '脚本控制台' : 'Script Console', component: 'script-console' } },
      { label: t('menu.help.keybindings'), action: () => ElMessage.warning(isZh.value ? '快捷键功能未实现' : 'Keyboard shortcuts not implemented') },
      { divider: true },
      { label: t('menu.help.sysinfo'), action: () => workspace.openDialog('about') },
      { label: t('menu.help.about'), action: () => workspace.openDialog('about') },
    ],
  },
])

function handleItemClick(item: MenuItem) {
  if (item.disabled) return
  closeMenu()
  if (item.action) {
    item.action()
    return
  }
  if (item.tab) {
    workspace.openTab(item.tab)
  }
}

function onGlobalClick(e: MouseEvent) {
  if (menuBarRef.value && !menuBarRef.value.contains(e.target as Node)) {
    closeMenu()
  }
}

function onKeyDown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    closeMenu()
  }
}

onMounted(() => {
  window.addEventListener('mousedown', onGlobalClick)
  window.addEventListener('keydown', onKeyDown)
})

onBeforeUnmount(() => {
  window.removeEventListener('mousedown', onGlobalClick)
  window.removeEventListener('keydown', onKeyDown)
})
</script>

<style scoped>
.cs-menubar {
  height: var(--cs-titlebar-h);
  background: var(--cs-titlebar-bg);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  padding: 0 4px;
  user-select: none;
  flex-shrink: 0;
  position: relative;
  z-index: 1000;
}

.cs-menu-top-wrap {
  position: relative;
}

.cs-menu-top-btn {
  padding: 4px 10px;
  background: transparent;
  border: 1px solid transparent;
  border-radius: 2px;
  color: var(--cs-text);
  cursor: pointer;
  font-size: 17px;
  font-family: var(--cs-font-ui);
  line-height: 20px;
  transition: all 80ms ease;
}

.cs-menu-top-btn:hover {
  background: var(--cs-menubar-hover);
  color: var(--cs-text-bright);
}

.cs-menu-top-btn.active {
  background: var(--cs-menubar-hover);
  color: var(--cs-text-bright);
  border-color: color-mix(in srgb, var(--cs-border) 70%, transparent);
}

/* Level 1 Dropdown Menu */
.cs-dropdown-menu {
  position: absolute;
  top: 100%;
  left: 0;
  min-width: 240px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-border);
  border-radius: 2px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.45);
  padding: 4px 0;
  z-index: 2000;
}

/* Level 2 Submenu */
.cs-sub-dropdown-menu {
  position: absolute;
  top: -4px;
  left: 100%;
  min-width: 240px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-border);
  border-radius: 2px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.45);
  padding: 4px 0;
  z-index: 2001;
}

.cs-dropdown-item {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 14px;
  height: 32px;
  color: var(--cs-text);
  font-size: 17px;
  cursor: pointer;
  white-space: nowrap;
}

.cs-dropdown-item:hover,
.cs-dropdown-item.sub-active {
  background: var(--cs-hover);
  color: var(--cs-text-bright);
}

.cs-dropdown-item.disabled {
  opacity: 0.45;
  pointer-events: none;
  cursor: default;
}

.mi-label {
  font-size: 17px;
}

.mi-key {
  font-size: 15px;
  color: var(--cs-text-dim);
  font-family: var(--cs-font-mono);
  margin-left: 20px;
}

.mi-arrow {
  font-size: 12px;
  color: var(--cs-text-dim);
  margin-left: 14px;
}

.cs-dropdown-divider {
  height: 1px;
  background: var(--cs-border);
  margin: 4px 0;
}

.mb-spacer {
  flex: 1;
}

.mb-right-tools {
  display: flex;
  align-items: center;
  gap: 8px;
}

.mb-fullscreen-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  background: transparent;
  border: 1px solid var(--cs-border);
  border-radius: 2px;
  color: var(--cs-text);
  padding: 0;
  width: 28px;
  height: 26px;
  cursor: pointer;
  transition: all 120ms ease;
}

.mb-fullscreen-btn:hover {
  background: var(--cs-menubar-hover);
  color: var(--cs-text-bright);
  border-color: var(--cs-active);
}

.mb-lang-btn {
  display: flex;
  align-items: center;
  gap: 3px;
  background: transparent;
  border: 1px solid var(--cs-border);
  border-radius: 2px;
  color: var(--cs-text);
  padding: 2px 8px;
  font-size: 16px;
  font-family: var(--cs-font-ui);
  cursor: pointer;
  height: 26px;
  transition: all 120ms ease;
}

.mb-lang-btn:hover {
  background: var(--cs-menubar-hover);
  color: var(--cs-text-bright);
  border-color: var(--cs-active);
}

.lang-text {
  opacity: 0.6;
}

.lang-text.active {
  opacity: 1;
  font-weight: 600;
  color: var(--cs-active);
}

.lang-sep {
  opacity: 0.4;
  font-size: 14px;
}

.operator-badge {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 16px;
  color: var(--cs-text-dim);
  padding: 2px 8px;
  background: color-mix(in srgb, var(--cs-active) 10%, transparent);
  border: 1px solid var(--cs-border);
  border-radius: 2px;
}

.op-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--cs-online);
}

.mb-theme {
  width: 28px;
  height: 26px;
  margin-right: 4px;
  background: transparent;
  border: 1px solid var(--cs-border);
  border-radius: 2px;
  color: var(--cs-text);
  cursor: pointer;
  font-size: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.mb-theme:hover {
  background: var(--cs-menubar-hover);
  color: var(--cs-text-bright);
}
</style>
