<template>
  <div class="vs-shell cs-shell cs-font-ui">
    <!-- ============ 顶栏（vshell 风格：logo + 横向导航 + 右侧动作） ============ -->
    <VsTopNav v-show="!folded" :active-key="activeMenuName" />

    <!-- ============ 标签条（路由访问历史） ============ -->
    <VsTags v-show="!folded" @refresh="onRefresh" @fold="onFold" />

    <!-- ============ 内容区（路由页面；keepAlive 由路由 meta 决定） ============ -->
    <div class="vs-content">
      <div class="vs-panel-host">
        <router-view v-slot="{ Component }">
          <KeepAlive :include="tabs.cachedNames">
            <component :is="Component" :key="viewKey" />
          </KeepAlive>
        </router-view>
      </div>
    </div>

  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import VsTopNav from './VsTopNav.vue'
import VsTags from './VsTags.vue'
import './vshell-theme.css'
import './vshell-ep.css'
import { deriveMenu } from '@/router/routes'

import { useEventsStore } from '@/stores/events'
import { useTabsStore } from '@/stores/tabs'

const eventsStore = useEventsStore()
const tabs = useTabsStore()
const route = useRoute()

const folded = ref(false)
const refreshTick = ref(0)

// 刷新当前页：改变 key 强制重挂
// key 只取「路由名 + 身份参数（meta.keyParams）」：页内页签参数（如 /client/:uid/:func）变化时
// 不重挂整页，否则页内状态与 KeepAlive 缓存会被整体丢弃
const viewKey = computed(() => {
  const name = String(route.name ?? route.path)
  const keyParams = (route.meta.keyParams as string[] | undefined) ?? []
  const id = keyParams.map((k) => `${k}=${route.params[k] ?? ''}`).join('&')
  return `${name}|${id}#${refreshTick.value}`
})

// 顶栏高亮：按当前路由匹配菜单项（取最长前缀）
const MENU = deriveMenu()
const activeMenuName = computed(() => {
  const p = route.path
  const hit = [...MENU].sort((a, b) => b.path.length - a.path.length).find((m) => p === m.path || p.startsWith(m.path + '/'))
  return hit?.name || ''
})

function onRefresh() {
  refreshTick.value += 1
}

function onFold(value: boolean) {
  folded.value = value
}

onMounted(() => {
  eventsStore.connect()
})

onBeforeUnmount(() => {
  eventsStore.disconnect()
})
</script>
