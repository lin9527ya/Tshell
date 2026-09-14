<template>
  <div class="cs-statusbar cs-font-mono">
    <!-- Left: [08/14 15:36:01] neo1 (1:1 CS format) -->
    <div class="sb-left">
      <span class="sb-time-user">
        [{{ dateStr }} {{ now }}] <span class="sb-op-name">{{ userStore.username || 'neo1' }}</span>
      </span>
    </div>

    <!-- Right: [Web: 172.17.46.75 | Beacons: 109 | lag: --] (1:1 CS format) -->
    <div class="sb-right">
      <span class="sb-cs-status">
        [{{ isZh ? 'Web 地址' : 'Web' }}: <span class="sb-val-ip">{{ hostInfo }}</span> | {{ isZh ? '会话数' : 'Beacons' }}: <span class="sb-val-beacons">{{ totalCount }}</span> | {{ isZh ? '延迟' : 'lag' }}: <span class="sb-val-lag">{{ lag }}</span>]
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useEventsStore } from '@/stores/events'
import { useClientsStore } from '@/stores/clients'
import { useUserStore } from '@/stores/user'
import { useI18nStore } from '@/stores/i18n'

const eventsStore = useEventsStore()
const clientsStore = useClientsStore()
const userStore = useUserStore()
const i18nStore = useI18nStore()
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
  height: 30px;
  background: #000000;
  color: #c0c0c0;
  font-size: 16.5px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 12px;
  border-top: 1px solid #222222;
  user-select: none;
  flex-shrink: 0;
}

.sb-left {
  display: flex;
  align-items: center;
}

.sb-time-user {
  color: #cccccc;
}

.sb-op-name {
  color: #ffffff;
  font-weight: bold;
}

.sb-right {
  display: flex;
  align-items: center;
}

.sb-cs-status {
  color: #aaaaaa;
}

.sb-val-ip {
  color: #d1a8e6;
  font-weight: 500;
}

.sb-val-beacons {
  color: #ff5252;
  font-weight: bold;
}

.sb-val-lag {
  color: #ffffff;
}
</style>
