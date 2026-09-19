<template>
  <div class="cs-statusbar cs-font-mono">
    <!-- Left: [08/14 15:36:01] neo1 (1:1 CS format) -->
    <div class="sb-left">
      <span class="sb-time-user">
        [{{ dateStr }} {{ now }}] <span class="sb-op-name">{{ userStore.username || 'neo1' }}</span>
      </span>
    </div>

    <!-- Right: [Web: host | Beacons: online/total | lag: --] (1:1 CS format) -->
    <div class="sb-right">
      <span class="sb-cs-status">
        [{{ isZh ? 'Web 地址' : 'Web' }}: <span class="sb-val-ip">{{ hostInfo }}</span> |
        <span
          class="sb-val-beacons sb-clickable"
          :title="isZh ? '点击聚焦会话列表' : 'Click to focus the session list'"
          @click="focusSessionTable"
        >{{ onlineCount }}/{{ totalCount }}</span> |
        <span class="sb-lag-label">{{ isZh ? '延迟' : 'lag' }}:</span>
        <span
          class="sb-val-lag"
          :title="isZh ? '后端未提供延迟数据源' : 'No latency data source on backend'"
        >{{ lag }}</span>]
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useClientsStore } from '@/stores/clients'
import { useUserStore } from '@/stores/user'
import { useI18nStore } from '@/stores/i18n'
import { useWorkspaceStore } from '@/stores/workspace'

const clientsStore = useClientsStore()
const userStore = useUserStore()
const i18nStore = useI18nStore()
const workspace = useWorkspaceStore()
const isZh = computed(() => i18nStore.isZh)

const onlineCount = computed(() => clientsStore.onlineCount ?? 0)
const totalCount = computed(() => clientsStore.totalCount ?? 0)

const hostInfo = computed(() => {
  // 浏览器访问地址（前后端分离部署时并非服务端地址）
  return window.location.hostname || '127.0.0.1'
})

const lag = ref('--') // 后端无延迟数据源，不伪造数值
const now = ref('')
const dateStr = ref('')
let timer: ReturnType<typeof setInterval> | null = null

// 点击会话数 → 聚焦会话列表(与 MenuBar「会话列表」同款编排)
function focusSessionTable() {
  workspace.setTopViewMode('table')
  workspace.isTopCollapsed = false
  workspace.openTab({
    id: 'sessions',
    title: isZh.value ? '会话列表' : 'Beacons',
    component: 'session-table',
  })
}

onMounted(() => {
  const tick = () => {
    const d = new Date()
    const mm = String(d.getMonth() + 1).padStart(2, '0')
    const dd = String(d.getDate()).padStart(2, '0')
    dateStr.value = `${mm}/${dd}`
    now.value = d.toLocaleTimeString('en-GB')
  }
  tick()
  timer = setInterval(tick, 1000)
})

onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})
</script>

<style scoped>
.cs-statusbar {
  height: var(--cs-statusbar-h);
  background: var(--cs-menubar-bg);
  color: var(--cs-text-dim);
  font-size: 16.5px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 12px;
  border-top: 1px solid var(--cs-border);
  user-select: none;
  flex-shrink: 0;
}

.sb-left {
  display: flex;
  align-items: center;
}

.sb-time-user {
  color: var(--cs-text-dim);
}

.sb-op-name {
  color: var(--cs-text-bright);
  font-weight: bold;
}

.sb-right {
  display: flex;
  align-items: center;
}

.sb-cs-status {
  color: var(--cs-text-dim);
}

.sb-val-ip {
  color: var(--cs-text);
  font-weight: 500;
}

.sb-val-beacons {
  color: var(--cs-online);
  font-weight: bold;
}

.sb-clickable {
  cursor: pointer;
}

.sb-clickable:hover {
  text-decoration: underline;
}

.sb-lag-label {
  color: var(--cs-text-dim);
}

.sb-val-lag {
  color: var(--cs-text-dim);
}
</style>
