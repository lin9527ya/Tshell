<template>
  <div class="cs-toolbar cs-font-ui">
    <!-- Group 1: Connection (Connect / Disconnect) -->
    <div class="tb-group">
      <button class="tb-btn" :title="t('tb.connect')" @click="openConnect">
        <svg class="tb-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="9" />
          <line x1="12" y1="8" x2="12" y2="16" />
          <line x1="8" y1="12" x2="16" y2="12" />
        </svg>
      </button>

      <button class="tb-btn tb-danger" :title="t('tb.disconnect')" @click="handleDisconnect">
        <svg class="tb-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="9" />
          <line x1="8" y1="12" x2="16" y2="12" />
        </svg>
      </button>
    </div>

    <div class="tb-divider" />

    <!-- Group 2: Core Views Switcher (Table / Graph / Targets) -->
    <div class="tb-group tb-segmented">
      <button
        class="tb-btn"
        :class="{ active: topViewMode === 'table' }"
        :title="t('tb.session_table')"
        @click="setViewMode('table')"
      >
        <svg class="tb-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <line x1="3" y1="6" x2="21" y2="6" />
          <line x1="3" y1="12" x2="21" y2="12" />
          <line x1="3" y1="18" x2="21" y2="18" />
        </svg>
      </button>

      <button
        class="tb-btn"
        :class="{ active: topViewMode === 'graph' }"
        :title="t('tb.pivot_graph')"
        @click="setViewMode('graph')"
      >
        <svg class="tb-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="6" cy="6" r="3" />
          <circle cx="18" cy="6" r="3" />
          <circle cx="12" cy="18" r="3" />
          <path d="M8.5 7.5l7 7M15.5 7.5l-7 7M9 6h6" />
        </svg>
      </button>

      <button
        class="tb-btn"
        :class="{ active: topViewMode === 'targets' }"
        :title="t('tb.targets')"
        @click="setViewMode('targets')"
      >
        <svg class="tb-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <rect x="2" y="3" width="20" height="14" rx="2" />
          <line x1="8" y1="21" x2="16" y2="21" />
          <line x1="12" y1="17" x2="12" y2="21" />
        </svg>
      </button>
    </div>

    <div class="tb-divider" />

    <!-- Group 3: Listeners & Attacks (Listeners / Web Delivery / Payload Gen) -->
    <div class="tb-group">
      <button class="tb-btn" :title="t('tb.listeners')" @click="openListeners">
        <svg class="tb-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M3 18v-6a9 9 0 0 1 18 0v6" />
          <path d="M21 19a2 2 0 0 1-2 2h-1a2 2 0 0 1-2-2v-3a2 2 0 0 1 2-2h3zM3 19a2 2 0 0 0 2 2h1a2 2 0 0 0 2-2v-3a2 2 0 0 0-2-2H3z" />
        </svg>
      </button>

      <button class="tb-btn" :title="t('tb.web_delivery')" @click="openWebDelivery">
        <svg class="tb-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="10" />
          <line x1="2" y1="12" x2="22" y2="12" />
          <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z" />
        </svg>
      </button>

      <button class="tb-btn" :title="t('tb.payload_gen')" @click="openServerGen">
        <svg class="tb-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="6" cy="12" r="1.5" fill="currentColor" />
          <circle cx="12" cy="12" r="1.5" fill="currentColor" />
          <circle cx="18" cy="12" r="1.5" fill="currentColor" />
          <rect x="2" y="6" width="20" height="12" rx="2" />
        </svg>
      </button>
    </div>

    <div class="tb-divider" />

    <!-- Group 4: Tactical Pivoting & Data Collection (Tunnel / Keystrokes / Screenshots) -->
    <div class="tb-group">
      <button class="tb-btn" :title="t('tb.tunnel')" @click="openTunnel">
        <svg class="tb-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M18 10h-1.26A8 8 0 1 0 9 20h9a5 5 0 0 0 0-10z" />
        </svg>
      </button>

      <button class="tb-btn" :title="t('tb.keystrokes')" @click="openKeystrokes">
        <svg class="tb-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <rect x="2" y="4" width="20" height="16" rx="2" />
          <line x1="6" y1="8" x2="6" y2="8" />
          <line x1="10" y1="8" x2="10" y2="8" />
          <line x1="14" y1="8" x2="14" y2="8" />
          <line x1="18" y1="8" x2="18" y2="8" />
          <line x1="6" y1="12" x2="6" y2="12" />
          <line x1="18" y1="12" x2="18" y2="12" />
          <line x1="9" y1="16" x2="15" y2="16" />
        </svg>
      </button>

      <button class="tb-btn" :title="t('tb.screenshots')" @click="openScreenshots">
        <svg class="tb-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M23 19a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4l2-3h6l2 3h4a2 2 0 0 1 2 2z" />
          <circle cx="12" cy="13" r="4" />
        </svg>
      </button>
    </div>

    <div class="tb-spacer" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useWorkspaceStore, type TopViewMode } from '@/stores/workspace'
import { useUserStore } from '@/stores/user'
import { useI18nStore } from '@/stores/i18n'
import UserAPI from '@/api/user'

const workspace = useWorkspaceStore()
const userStore = useUserStore()
const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)
const t = (key: string, def?: string) => i18nStore.t(key, def)

const topViewMode = computed(() => workspace.topViewMode)

function setViewMode(mode: TopViewMode) {
  workspace.setTopViewMode(mode)
}

function openConnect() {
  workspace.openDialog('connect')
}

async function handleDisconnect() {
  try {
    await UserAPI.logout({ refresh: userStore.refresh || '' })
  } catch {
    /* 忽略：本地登出兜底 */
  }
  userStore.logout()
}

function openListeners() {
  workspace.openTab({ id: 'listeners', title: isZh.value ? '监听器' : 'Listeners', component: 'listeners' })
}

function openWebDelivery() {
  workspace.openTab({ id: 'webdelivery', title: isZh.value ? 'Web投递' : 'Web Delivery', component: 'webdelivery' })
}

function openServerGen() {
  workspace.openTab({ id: 'server-gen', title: isZh.value ? '分阶段载荷生成器' : 'Payload Generator (Staged)', component: 'server-gen' })
}

function openTunnel() {
  workspace.openTab({ id: 'tunnel-proxy', title: isZh.value ? '代理转发' : 'Tunnel Proxy', component: 'tunnel-proxy' })
}

function openKeystrokes() {
  workspace.openTab({ id: 'global-keystrokes', title: isZh.value ? '键盘记录' : 'Keystrokes', component: 'global-keystrokes' })
}

function openScreenshots() {
  workspace.openTab({ id: 'global-screenshots', title: isZh.value ? '屏幕截图' : 'Screenshots', component: 'global-screenshots' })
}
</script>

<style scoped>
.cs-toolbar {
  height: 36px;
  background: var(--cs-titlebar-bg);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  padding: 0 8px;
  gap: 6px;
  user-select: none;
  flex-shrink: 0;
}

.tb-group {
  display: flex;
  align-items: center;
  gap: 3px;
}

.tb-divider {
  width: 1px;
  height: 18px;
  background: var(--cs-border);
  margin: 0 4px;
  opacity: 0.8;
}

.tb-btn {
  width: 30px;
  height: 26px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: transparent;
  border: 1px solid transparent;
  border-radius: 2px;
  color: var(--cs-text);
  cursor: pointer;
  padding: 0;
  transition: all 120ms ease;
}

.tb-btn:hover {
  background: var(--cs-hover);
  border-color: color-mix(in srgb, var(--cs-border) 80%, transparent);
  color: var(--cs-text-bright);
}

.tb-btn.active {
  background: color-mix(in srgb, var(--cs-active) 20%, transparent);
  border-color: var(--cs-active);
  color: var(--cs-active);
  box-shadow: inset 0 0 0 1px var(--cs-active);
}

.tb-btn.tb-danger:hover {
  background: var(--cs-error);
  border-color: var(--cs-error);
  color: #fff;
}

.tb-icon {
  width: 17px;
  height: 17px;
}

.tb-spacer {
  flex: 1;
}
</style>
