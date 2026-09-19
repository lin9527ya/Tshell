<template>
  <div class="cs-tab-container cs-font-ui">
    <!-- Tab Strip Bar -->
    <div class="tab-strip" @contextmenu.prevent="onStripContextMenu">
      <!-- Scroll Left Button -->
      <button
        v-show="canScrollLeft"
        class="tab-scroll-btn"
        :title="isZh ? '向左滑动标签' : 'Scroll Left'"
        @click="scrollTabs('left')"
      >
        ‹
      </button>

      <div ref="scrollAreaRef" class="tabs-scroll-area" @scroll="checkScrollOverflow">
        <div
          v-for="tab in tabs"
          :key="tab.id"
          class="cs-tab"
          :class="{ active: tab.id === activeTabId }"
          @click="selectTab(tab.id)"
          @contextmenu.prevent.stop="onTabContextMenu($event, tab.id)"
        >
          <!-- Tab Icon -->
          <span class="tab-icon">
            <svg v-if="tab.component === 'beacon' || tab.component === 'script-console' || tab.component === 'client-terminal'" viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
              <polyline points="4 17 10 11 4 5" />
              <line x1="12" y1="19" x2="20" y2="19" />
            </svg>
            <svg v-else-if="tab.component === 'event-log'" viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
            </svg>
            <svg v-else-if="tab.component === 'applications'" viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
              <rect x="3" y="3" width="7" height="7" rx="1" />
              <rect x="14" y="3" width="7" height="7" rx="1" />
              <rect x="14" y="14" width="7" height="7" rx="1" />
              <rect x="3" y="14" width="7" height="7" rx="1" />
            </svg>
            <svg v-else-if="tab.component === 'targets'" viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="12" r="10" />
              <line x1="22" y1="12" x2="18" y2="12" />
              <line x1="6" y1="12" x2="2" y2="12" />
              <line x1="12" y1="6" x2="12" y2="2" />
              <line x1="12" y1="22" x2="12" y2="18" />
            </svg>
            <svg v-else-if="tab.component === 'sessions' || tab.component === 'session-table'" viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
              <rect x="3" y="3" width="18" height="18" rx="2" />
              <line x1="3" y1="9" x2="21" y2="9" />
              <line x1="3" y1="15" x2="21" y2="15" />
              <line x1="9" y1="9" x2="9" y2="21" />
            </svg>
            <svg v-else-if="tab.component === 'sites' || tab.component === 'site-manage' || tab.component === 'webdelivery'" viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="12" r="10" />
              <line x1="2" y1="12" x2="22" y2="12" />
              <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z" />
            </svg>
            <svg v-else-if="tab.component === 'credentials' || tab.component === 'key-management'" viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M21 2l-2 2m-1.5 1.5L14 9a5 5 0 1 0 4.5 4.5l3.5-3.5V6l-4-4z" />
              <circle cx="7.5" cy="16.5" r="1.5" />
            </svg>
            <svg v-else-if="tab.component === 'global-downloads' || tab.component === 'client-downloads'" viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
              <polyline points="7 10 12 15 17 10" />
              <line x1="12" y1="15" x2="12" y2="3" />
            </svg>
            <svg v-else-if="tab.component === 'global-screenshots' || tab.component === 'client-screenshots'" viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M23 19a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4l2-3h6l2 3h4a2 2 0 0 1 2 2z" />
              <circle cx="12" cy="13" r="4" />
            </svg>
            <svg v-else-if="tab.component === 'global-keystrokes'" viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
              <rect x="2" y="4" width="20" height="16" rx="2" />
              <line x1="6" y1="8" x2="6" y2="8" />
              <line x1="10" y1="8" x2="10" y2="8" />
              <line x1="14" y1="8" x2="14" y2="8" />
              <line x1="18" y1="8" x2="18" y2="8" />
              <line x1="6" y1="12" x2="6" y2="12" />
              <line x1="18" y1="12" x2="18" y2="12" />
              <line x1="9" y1="16" x2="15" y2="16" />
            </svg>
            <svg v-else-if="tab.component === 'client-files'" viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z" />
            </svg>
            <svg v-else-if="tab.component === 'client-pids'" viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
              <rect x="4" y="4" width="16" height="16" rx="2" />
              <rect x="9" y="9" width="6" height="6" />
              <line x1="9" y1="1" x2="9" y2="4" />
              <line x1="15" y1="1" x2="15" y2="4" />
              <line x1="9" y1="20" x2="9" y2="23" />
              <line x1="15" y1="20" x2="15" y2="23" />
            </svg>
            <svg v-else-if="tab.component === 'client-monitor'" viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
              <rect x="2" y="3" width="20" height="14" rx="2" />
              <line x1="8" y1="21" x2="16" y2="21" />
              <line x1="12" y1="17" x2="12" y2="21" />
            </svg>
            <svg v-else-if="tab.component === 'client-tools'" viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z" />
            </svg>
            <svg v-else-if="tab.component === 'listeners' || tab.component === 'forward-listeners'" viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M4.9 19.1C1 15.2 1 8.8 4.9 4.9" />
              <path d="M7.8 16.2c-2.3-2.3-2.3-6.1 0-8.5" />
              <circle cx="12" cy="12" r="2" />
              <path d="M16.2 7.8c2.3 2.3 2.3 6.1 0 8.5" />
              <path d="M19.1 4.9C23 8.8 23 15.2 19.1 19.1" />
            </svg>
            <svg v-else-if="tab.component === 'plugins' || tab.component === 'script-manager' || tab.component === 'scripts'" viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83" />
            </svg>
            <svg v-else-if="tab.component === 'tunnel-proxy'" viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="6" y1="3" x2="6" y2="15" />
              <circle cx="18" cy="6" r="3" />
              <circle cx="6" cy="18" r="3" />
              <path d="M18 9a9 9 0 0 1-9 9" />
            </svg>
            <svg v-else-if="tab.component === 'server-gen'" viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z" />
              <polyline points="3.27 6.96 12 12.01 20.73 6.96" />
              <line x1="12" y1="22.08" x2="12" y2="12" />
            </svg>
            <svg v-else-if="tab.component === 'audit' || tab.component === 'audit-logs' || tab.component === 'web-log'" viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
              <polyline points="14 2 14 8 20 8" />
              <line x1="16" y1="13" x2="8" y2="13" />
              <line x1="16" y1="17" x2="8" y2="17" />
              <polyline points="10 9 9 9 8 9" />
            </svg>
            <svg v-else-if="tab.component === 'security-center'" viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
              <polyline points="9 12 11 14 15 10" />
            </svg>
            <svg v-else-if="tab.component === 'hidden-access'" viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24" />
              <line x1="1" y1="1" x2="23" y2="23" />
            </svg>
            <svg v-else-if="tab.component === 'user-management'" viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
              <circle cx="9" cy="7" r="4" />
              <path d="M23 21v-2a4 4 0 0 0-3-3.87" />
              <path d="M16 3.13a4 4 0 0 1 0 7.75" />
            </svg>
            <svg v-else-if="tab.component === 'server-config'" viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="4" y1="21" x2="4" y2="14" />
              <line x1="4" y1="10" x2="4" y2="3" />
              <line x1="12" y1="21" x2="12" y2="12" />
              <line x1="12" y1="8" x2="12" y2="3" />
              <line x1="20" y1="21" x2="20" y2="16" />
              <line x1="20" y1="12" x2="20" y2="3" />
              <line x1="1" y1="14" x2="7" y2="14" />
              <line x1="9" y1="8" x2="15" y2="8" />
              <line x1="17" y1="16" x2="23" y2="16" />
            </svg>
            <svg v-else-if="tab.component === 'connection-pool'" viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
              <rect x="2" y="2" width="20" height="8" rx="2" ry="2" />
              <rect x="2" y="14" width="20" height="8" rx="2" ry="2" />
              <line x1="6" y1="6" x2="6.01" y2="6" />
              <line x1="6" y1="18" x2="6.01" y2="18" />
            </svg>
            <svg v-else-if="tab.component === 'interfaces' || tab.component === 'vpn-interfaces'" viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
              <rect x="16" y="16" width="6" height="6" rx="1" />
              <rect x="2" y="16" width="6" height="6" rx="1" />
              <rect x="9" y="2" width="6" height="6" rx="1" />
              <path d="M5 16v-3a1 1 0 0 1 1-1h12a1 1 0 0 1 1 1v3" />
              <line x1="12" y1="12" x2="12" y2="8" />
            </svg>
            <svg v-else-if="tab.component === 'about'" viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="12" r="10" />
              <line x1="12" y1="16" x2="12" y2="12" />
              <line x1="12" y1="8" x2="12.01" y2="8" />
            </svg>
            <!-- Meaningful Default Window/Tool Icon -->
            <svg v-else viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
              <rect x="3" y="3" width="18" height="18" rx="2" />
              <line x1="3" y1="8" x2="21" y2="8" />
              <line x1="7" y1="5.5" x2="7.01" y2="5.5" stroke-width="3" />
            </svg>
          </span>

          <span class="tab-title" :title="formatTabTitle(tab)">{{ formatTabTitle(tab) }}</span>

          <!-- Close Tab Button -->
          <button
            v-if="tab.closable !== false"
            class="tab-close-btn"
            title="Close Tab"
            @click.stop="closeTab(tab.id)"
          >
            ✕
          </button>
        </div>
      </div>

      <!-- Scroll Right Button -->
      <button
        v-show="canScrollRight"
        class="tab-scroll-btn"
        :title="isZh ? '向右滑动标签' : 'Scroll Right'"
        @click="scrollTabs('right')"
      >
        ›
      </button>

      <!-- Quick Tab Adder / Launcher -->
      <div class="tab-strip-actions">
        <el-dropdown trigger="click" @command="launchToolTab">
          <button class="tab-add-btn" :title="isZh ? '打开工具标签页' : 'Open Tool Tab'">
            ＋
          </button>
          <template #dropdown>
            <el-dropdown-menu class="cs-menu-popper">
              <!-- Group 1: Sessions & Logs -->
              <div class="cs-menu-group-title">{{ isZh ? '会话与日志' : 'Sessions & Logs' }}</div>
              <el-dropdown-item command="event-log">{{ t('menu.view.event_log') }}</el-dropdown-item>
              <el-dropdown-item command="script-console">{{ t('menu.view.script_console') }}</el-dropdown-item>
              <el-dropdown-item command="web-log">{{ t('menu.view.web_log') }}</el-dropdown-item>
              <el-dropdown-item command="audit">{{ t('menu.view.audit_logs') }}</el-dropdown-item>
              <el-dropdown-item command="batch-tasks">{{ isZh ? '批量任务' : 'Batch Tasks' }}</el-dropdown-item>

              <!-- Group 2: Tactical Collection -->
              <div class="cs-menu-group-title">{{ isZh ? '战术收集' : 'Tactical Collection' }}</div>
              <el-dropdown-item command="credentials">{{ t('menu.view.credentials') }}</el-dropdown-item>
              <el-dropdown-item command="global-downloads">{{ t('menu.view.downloaded_files') }}</el-dropdown-item>
              <el-dropdown-item command="global-screenshots">{{ t('menu.view.screenshots') }}</el-dropdown-item>
              <el-dropdown-item command="global-keystrokes">{{ t('menu.view.keystrokes') }}</el-dropdown-item>
              <el-dropdown-item command="applications">{{ isZh ? '应用程序' : 'Applications' }}</el-dropdown-item>

              <!-- Group 3: Listeners & Attacks -->
              <div class="cs-menu-group-title">{{ isZh ? '监听与载荷' : 'Listeners & Attacks' }}</div>
              <el-dropdown-item command="listeners">{{ t('menu.view.listeners') }}</el-dropdown-item>
              <el-dropdown-item command="forward-listeners">{{ t('menu.view.forward_listeners') }}</el-dropdown-item>
              <el-dropdown-item command="webdelivery">{{ isZh ? 'Web投递管理' : 'Web Delivery' }}</el-dropdown-item>
              <el-dropdown-item command="server-gen">{{ t('menu.view.payload_gen') }}</el-dropdown-item>
              <el-dropdown-item command="tunnel-proxy">{{ t('menu.view.proxy_pivots') }}</el-dropdown-item>
              <el-dropdown-item command="plugins">{{ t('ctx.postexp') }}</el-dropdown-item>

              <!-- Group 4: System Config -->
              <div class="cs-menu-group-title">{{ isZh ? '系统与配置' : 'System Config' }}</div>
              <el-dropdown-item command="security-center">{{ isZh ? '安全中心' : 'Security Center' }}</el-dropdown-item>
              <el-dropdown-item command="hidden-access">{{ isZh ? '隐藏访问' : 'Stealth Access' }}</el-dropdown-item>
              <el-dropdown-item command="key-management">{{ t('menu.sysconfig.keys') }}</el-dropdown-item>
              <el-dropdown-item command="user-management">{{ t('menu.sysconfig.users') }}</el-dropdown-item>
              <el-dropdown-item command="server-config">{{ t('menu.sysconfig.config') }}</el-dropdown-item>
              <el-dropdown-item command="server-status">{{ t('menu.sysconfig.status') }}</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </div>
    </div>

    <!-- Tab Panels Area -->
    <div class="tab-content-area">
      <template v-if="tabs.length > 0">
        <div
          v-for="tab in tabs"
          :key="tab.id"
          v-show="tab.id === activeTabId"
          class="tab-pane-wrapper"
        >
          <!-- 【性能】惰性挂载: 首次激活才 mount(此前 v-show 全挂载,开 N 个标签
               即有 N 个 xterm/轮询实例);卸载型关闭由 closeTab 移除列表驱动 -->
          <TabContentHost v-if="activatedTabIds.has(tab.id)" :tab="tab" />
        </div>
      </template>

      <!-- Empty State when no tabs open -->
      <div v-else class="tab-empty-state">
        <div class="empty-watermark">
          <div class="wm-title">Cobalt Strike / BladeShell</div>
          <div class="wm-sub">{{ isZh ? '双击上方任意 Beacon 会话或点击 [ + ] 快捷打开工具标签页。' : 'Double-click any beacon or click [ + ] to open a tool tab.' }}</div>
          <div class="wm-shortcuts">
            <button class="cs-btn" @click="openDefaultEventLog">{{ isZh ? '打开事件日志' : 'Open Event Log' }}</button>
            <button class="cs-btn" @click="openDefaultListeners">{{ isZh ? '打开监听器管理' : 'Open Listeners' }}</button>
            <button class="cs-btn" @click="openDefaultScriptConsole">{{ isZh ? '打开脚本控制台' : 'Open Script Console' }}</button>
          </div>
        </div>
      </div>
    </div>

    <!-- Tab Context Menu -->
    <div
      v-if="ctxMenu.visible"
      class="cs-ctx-menu"
      :style="{ left: ctxMenu.x + 'px', top: ctxMenu.y + 'px' }"
      @mousedown.stop
    >
      <div class="ctx-item" @click="closeTab(ctxMenu.tabId)">{{ isZh ? '关闭标签页' : 'Close Tab' }}</div>
      <div class="ctx-item" @click="closeOtherTabs(ctxMenu.tabId)">{{ isZh ? '关闭其他标签页' : 'Close Other Tabs' }}</div>
      <div class="ctx-divider" />
      <div class="ctx-item ctx-danger" @click="closeAllTabs">{{ isZh ? '关闭所有标签页' : 'Close All Tabs' }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onBeforeUnmount, onMounted, provide, reactive, ref, watch, nextTick } from 'vue'
import { useWorkspaceStore, type CsTab } from '@/stores/workspace'
import { useI18nStore } from '@/stores/i18n'
import { getPanelComponent } from '../Workspace/panelRegistry'

const workspace = useWorkspaceStore()
const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)
const t = (k: string, d?: string) => i18nStore.t(k, d)

const tabs = computed(() => workspace.tabs)
const activeTabId = computed(() => workspace.activeTabId)

// 惰性挂载: 只有被激活过的标签才渲染内容(见模板 TabContentHost v-if)
const activatedTabIds = reactive(new Set<string>())
watch(
  activeTabId,
  (id) => {
    if (id) activatedTabIds.add(id)
  },
  { immediate: true },
)

const scrollAreaRef = ref<HTMLElement | null>(null)
const canScrollLeft = ref(false)
const canScrollRight = ref(false)

function checkScrollOverflow() {
  const el = scrollAreaRef.value
  if (!el) return
  canScrollLeft.value = el.scrollLeft > 2
  canScrollRight.value = el.scrollLeft + el.clientWidth < el.scrollWidth - 2
}

function scrollTabs(direction: 'left' | 'right') {
  const el = scrollAreaRef.value
  if (!el) return
  const offset = direction === 'left' ? -200 : 200
  el.scrollBy({ left: offset, behavior: 'smooth' })
  setTimeout(checkScrollOverflow, 300)
}

watch(
  () => [tabs.value.length, activeTabId.value],
  () => {
    nextTick(() => {
      checkScrollOverflow()
      // Auto-scroll to active tab
      const el = scrollAreaRef.value
      if (!el) return
      const activeTabEl = el.querySelector('.cs-tab.active') as HTMLElement | null
      if (activeTabEl) {
        activeTabEl.scrollIntoView({ behavior: 'smooth', block: 'nearest', inline: 'nearest' })
      }
    })
  },
  { deep: true },
)

// Context Menu
const ctxMenu = ref({ visible: false, x: 0, y: 0, tabId: '' })

// Host component that provides `realUid` for child views
const TabContentHost = defineComponent({
  name: 'TabContentHost',
  props: {
    tab: {
      type: Object as () => CsTab,
      required: true,
    },
  },
  setup(props) {
    const uidRef = computed(() => props.tab.params?.uid || '')
    provide('realUid', uidRef)
    // 【性能】向面板提供"是否为活动标签"——非活动时 useVisibilityInterval
    // 暂停轮询(两个壳都常驻挂载面板,切走的标签此前仍在后台轮询)
    provide('tabActive', computed(() => props.tab.id === workspace.activeTabId))
    return () => {
      const comp = getPanelComponent(props.tab.component)
      if (!comp) {
        return h('div', { class: 'panel-not-found' }, `Panel component [${props.tab.component}] not registered`)
      }
      return h(comp, {
        key: props.tab.id,
        ...(props.tab.params || {}),
      })
    }
  },
})

function selectTab(id: string) {
  workspace.setActiveTab(id)
}

function closeTab(id: string) {
  workspace.closeTab(id)
  closeCtx()
}

function closeOtherTabs(id: string) {
  workspace.closeOtherTabs(id)
  closeCtx()
}

function closeAllTabs() {
  workspace.closeAllTabs()
  closeCtx()
}

function formatTabTitle(tab: any): string {
  if (!tab) return ''
  const t = tab.title || ''
  if (!isZh.value) return t

  const id = tab.id || ''
  const comp = tab.component || ''

  // 独立静态全局页面
  const staticTitles: Record<string, string> = {
    'event-log': '事件日志',
    'script-console': '脚本控制台',
    'credentials': '密码凭证',
    'global-downloads': '下载列表',
    'global-screenshots': '屏幕截图',
    'global-keystrokes': '键盘记录',
    'listeners': '监听器',
    'forward-listeners': '正向监听',
    'webdelivery': 'Web投递',
    'server-gen': '分阶段载荷生成器',
    'tunnel-proxy': '代理转发',
    'socks5': 'SOCKS5 代理',
    'plugins': '内存执行',
    'key-management': '密钥管理',
    'session-keys': '会话密钥',
    'user-management': '用户管理',
    'server-config': '服务配置',
    'server-status': '服务器状态',
    'audit': '审计日志',
    'audit-logs': '审计日志',
    'web-log': 'Web日志',
    'sites': '站点管理',
    'site-manage': '站点管理',
    'batch-tasks': '批量任务',
    'about': '关于',
  }
  if (staticTitles[id]) return staticTitles[id]
  if (staticTitles[comp]) return staticTitles[comp]

  // 带目标标签的客户端工具 Tab（如 Tools: user@host -> 客户端工具: user@host）
  const targetName = t.includes(':') ? t.substring(t.indexOf(':') + 1).trim() : ''

  if (comp === 'client-tools' || id.startsWith('tools:')) {
    return `客户端工具: ${targetName}`
  }
  if (comp === 'client-downloads' || id.startsWith('downloads:')) {
    return `已下载文件: ${targetName}`
  }
  if (comp === 'client-startup' || id.startsWith('startup:')) {
    return `开机启动: ${targetName}`
  }
  if (comp === 'client-monitor' || id.startsWith('monitor:')) {
    return `远程桌面: ${targetName}`
  }
  if (comp === 'client-files' || id.startsWith('files:')) {
    return `文件管理: ${targetName}`
  }
  if (comp === 'client-pids' || id.startsWith('pids:')) {
    return `进程列表: ${targetName}`
  }
  if (comp === 'client-screenshots' || id.startsWith('screenshots:')) {
    return `屏幕截图: ${targetName}`
  }
  if (id.startsWith('win-term:')) {
    return `Windows 终端: ${targetName}`
  }
  if (comp === 'client-terminal' || id.startsWith('term:')) {
    return `Linux 终端: ${targetName}`
  }
  if (comp === 'beacon' || id.startsWith('beacon:')) {
    return `会话交互: ${targetName}`
  }

  // 常规英文前缀替换
  return t
    .replace(/^Tools:\s*/i, '客户端工具: ')
    .replace(/^Downloads:\s*/i, '已下载文件: ')
    .replace(/^Startup:\s*/i, '开机启动: ')
    .replace(/^Desktop:\s*/i, '远程桌面: ')
    .replace(/^Files:\s*/i, '文件管理: ')
    .replace(/^Processes:\s*/i, '进程列表: ')
    .replace(/^Screenshots:\s*/i, '屏幕截图: ')
    .replace(/^Terminal:\s*/i, '终端交互: ')
    .replace(/^Beacon:\s*/i, '会话交互: ')
}

function launchToolTab(cmd: string) {
  const titles: Record<string, string> = {
    'event-log': isZh.value ? '事件日志' : 'Event Log',
    'script-console': isZh.value ? '脚本控制台' : 'Script Console',
    'credentials': isZh.value ? '密码凭证' : 'Credentials',
    'global-downloads': isZh.value ? '下载列表' : 'Downloaded Files',
    'global-screenshots': isZh.value ? '屏幕截图' : 'Screenshots',
    'global-keystrokes': isZh.value ? '键盘记录' : 'Keystrokes',
    'listeners': isZh.value ? '监听器' : 'Listeners',
    'forward-listeners': isZh.value ? '正向监听' : 'Forward Listeners',
    'webdelivery': isZh.value ? 'Web投递' : 'Web Delivery',
    'server-gen': isZh.value ? '分阶段载荷生成器' : 'Payload Generator (Staged)',
    'tunnel-proxy': isZh.value ? '代理转发' : 'Tunnel Proxy',
    'plugins': isZh.value ? '内存执行' : 'Memory Execution',
    'key-management': isZh.value ? '密钥管理' : 'Key Management',
    'session-keys': isZh.value ? '会话密钥' : 'Session Keys',
    'user-management': isZh.value ? '用户管理' : 'User Management',
    'server-config': isZh.value ? '服务配置' : 'Server Config',
    'server-status': isZh.value ? '服务器状态' : 'Server Status',
    'audit': isZh.value ? '审计日志' : 'Audit Logs',
  }
  workspace.openTab({
    id: cmd,
    title: titles[cmd] || cmd,
    component: cmd,
  })
}

function openDefaultEventLog() {
  workspace.openTab({ id: 'event-log', title: isZh.value ? '事件日志' : 'Event Log', component: 'event-log' })
}

function openDefaultListeners() {
  workspace.openTab({ id: 'listeners', title: isZh.value ? '监听器' : 'Listeners', component: 'listeners' })
}

function openDefaultScriptConsole() {
  workspace.openTab({ id: 'script-console', title: isZh.value ? '脚本控制台' : 'Script Console', component: 'script-console' })
}

function onTabContextMenu(e: MouseEvent, tabId: string) {
  ctxMenu.value = {
    visible: true,
    x: e.clientX,
    y: e.clientY,
    tabId,
  }
}

function onStripContextMenu(e: MouseEvent) {
  if (tabs.value.length === 0) return
  ctxMenu.value = {
    visible: true,
    x: e.clientX,
    y: e.clientY,
    tabId: activeTabId.value,
  }
}

function closeCtx() {
  ctxMenu.value.visible = false
}

onMounted(() => {
  window.addEventListener('click', closeCtx)
})

onBeforeUnmount(() => {
  window.removeEventListener('click', closeCtx)
})
</script>

<style scoped>
.cs-tab-container {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-panel);
  overflow: hidden;
  position: relative;
}

/* Tab Strip */
.tab-strip {
  display: flex;
  align-items: stretch;
  height: 36px;
  background: var(--cs-titlebar-bg);
  border-bottom: 1px solid var(--cs-border);
  user-select: none;
  flex-shrink: 0;
  overflow: hidden;
}
.tabs-scroll-area {
  flex: 1;
  display: flex;
  align-items: stretch;
  overflow-x: auto;
  overflow-y: hidden;
  scrollbar-width: none;
}
.tabs-scroll-area::-webkit-scrollbar {
  display: none;
}
.cs-tab {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 14px;
  background: var(--cs-tab-inactive-bg);
  border-right: 1px solid var(--cs-border);
  border-top: 2px solid transparent;
  color: var(--cs-text-dim);
  font-size: 16.5px;
  cursor: pointer;
  max-width: 280px;
  min-width: 120px;
  transition: all 100ms ease;
}
.cs-tab:hover {
  background: var(--cs-tab-hover-bg);
  color: var(--cs-text);
}
.cs-tab.active {
  background: var(--cs-panel);
  border-top-color: var(--cs-active);
  color: var(--cs-text-bright);
  font-weight: 500;
}
.tab-icon {
  display: flex;
  align-items: center;
  opacity: 0.8;
}
.tab-icon svg {
  width: 16px;
  height: 16px;
}
.tab-title {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tab-close-btn {
  width: 18px;
  height: 18px;
  border: 0;
  background: transparent;
  color: var(--cs-text-dim);
  font-size: 14px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  border-radius: 2px;
  padding: 0;
  line-height: 1;
}
.tab-close-btn:hover {
  background: var(--cs-error);
  color: #fff;
}
.tab-scroll-btn {
  width: 20px;
  height: 36px;
  background: var(--cs-titlebar-bg);
  border: none;
  border-right: 1px solid var(--cs-border);
  border-left: 1px solid var(--cs-border);
  color: var(--cs-text-dim);
  font-size: 16px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  transition: all 120ms ease;
  z-index: 10;
}
.tab-scroll-btn:hover {
  background: var(--cs-hover);
  color: var(--cs-text-bright);
}

.tab-strip-actions {
  display: flex;
  align-items: center;
  padding: 0 6px;
  border-left: 1px solid var(--cs-border);
  background: var(--cs-titlebar-bg);
}
.tab-add-btn {
  background: transparent;
  border: 0;
  color: var(--cs-text-dim);
  font-size: 18px;
  width: 28px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  border-radius: 2px;
}
.tab-add-btn:hover {
  background: var(--cs-hover);
  color: var(--cs-text-bright);
}

:deep(.cs-menu-group-title) {
  font-size: 12px;
  font-weight: 600;
  color: var(--cs-text-dim);
  padding: 6px 12px 2px;
  user-select: none;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  opacity: 0.85;
}

/* Tab Panels */
.tab-content-area {
  flex: 1;
  min-height: 0;
  position: relative;
  overflow: hidden;
}
.tab-pane-wrapper {
  width: 100%;
  height: 100%;
  position: absolute;
  inset: 0;
}

/* Empty State */
.tab-empty-state {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--cs-panel);
}
.empty-watermark {
  text-align: center;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
}
.wm-title {
  font-size: 24px;
  font-weight: 600;
  color: var(--cs-text-dim);
  letter-spacing: .04em;
}
.wm-sub {
  font-size: 17px;
  color: var(--cs-text-dim);
  opacity: 0.8;
}
.wm-shortcuts {
  display: flex;
  gap: 12px;
  margin-top: 10px;
}

.panel-not-found {
  padding: 24px;
  color: var(--cs-error);
  font-size: 18px;
}

/* Context Menu */
.cs-ctx-menu {
  position: fixed;
  z-index: 3500;
  min-width: 180px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-border);
  border-radius: 2px;
  padding: 4px 0;
  box-shadow: 2px 4px 14px rgba(0, 0, 0, 0.5);
  user-select: none;
}
.ctx-item {
  padding: 6px 14px;
  font-size: 16.5px;
  color: var(--cs-text);
  cursor: pointer;
}
.ctx-item:hover {
  background: var(--cs-hover);
  color: var(--cs-text-bright);
}
.ctx-item.ctx-danger {
  color: var(--cs-error);
}
.ctx-divider {
  height: 1px;
  background: var(--cs-border);
  margin: 4px 0;
}
</style>
