<template>
  <div class="vs-shell cs-shell cs-font-ui" :class="'vs-navmode-' + navMode">
    <!-- 顶部菜单（top-menu）：整宽顶栏 = logo + 8 项导航 + 动作；
         mix：整宽顶栏只放 logo（菜单底色）+ 汉堡/标题 + 动作 -->
    <VsTopNav
      v-if="!sideBySide"
      v-show="!folded"
      :active-key="activeMenuName"
      :variant="navMode === 'mix' ? 'mix' : 'full'"
      :show-menu="isTop"
      @toggle-side="sideCollapsed = !sideCollapsed"
    />

    <!-- 主体：左侧菜单模式插入侧栏；sidebar/mix-sidebar 的副栏顶栏在内容列内（顶栏从侧栏右侧起） -->
    <div v-if="hasSidebar" class="vs-side-layout">
      <VsSideNav
        :active-key="activeMenuName"
        :collapsed="sideCollapsed"
        :rail="isRail"
        :show-logo="sideBySide"
      />

      <div class="vs-shell-main">
        <VsTopNav
          v-if="sideBySide"
          v-show="!folded"
          :active-key="activeMenuName"
          variant="slim"
          :show-menu="false"
          @toggle-side="sideCollapsed = !sideCollapsed"
        />
        <VsTags @refresh="onRefresh" @fold="onFold" />

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
    </div>

    <!-- 顶部菜单模式：无侧栏。折叠只收起顶栏；标签条保留（vshell 同款：
         折叠按钮在标签条上，否则收起后没有任何入口能恢复页头） -->
    <template v-else>
      <VsTags @refresh="onRefresh" @fold="onFold" />
      <div class="vs-content">
        <div class="vs-panel-host">
          <router-view v-slot="{ Component }">
            <KeepAlive :include="tabs.cachedNames">
              <component :is="Component" :key="viewKey" />
            </KeepAlive>
          </router-view>
        </div>
      </div>
    </template>
  </div>
</template>


<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import VsTopNav from './VsTopNav.vue'
import VsSideNav from './VsSideNav.vue'
import VsTags from './VsTags.vue'
import './vshell-theme.css'
import './vshell-ep.css'
import { deriveMenu } from '@/router/routes'

import { useEventsStore } from '@/stores/events'
import { useTabsStore } from '@/stores/tabs'
import { useThemeStore } from '@/stores/theme'

const eventsStore = useEventsStore()
const tabs = useTabsStore()
const themeStore = useThemeStore()
const route = useRoute()

// 顶部菜单模式下折叠页头（藏顶栏+标签条）
const folded = ref(false)
// 左侧菜单模式下折叠侧栏为图标条
const sideCollapsed = ref(false)
const refreshTick = ref(0)

const navMode = computed(() => themeStore.config.navMode)
const isTop = computed(() => navMode.value === 'top-menu')
/** 有侧栏（除顶部菜单外三种） */
const hasSidebar = computed(() => !isTop.value)
/** sidebar / mix-sidebar：侧栏整高、顶栏从侧栏右侧起（logo 在侧栏顶部） */
const sideBySide = computed(() => navMode.value === 'sidebar' || navMode.value === 'mix-sidebar')
/** mix-sidebar：80px 图标栏（vshell 的「左侧菜单混合模式」） */
const isRail = computed(() => navMode.value === 'mix-sidebar')

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
  // 左侧菜单模式下「折叠页头」改为收起侧栏（顶栏保持可见，否则无法再展开）
  if (isTop.value) folded.value = value
  else sideCollapsed.value = value
}

onMounted(() => {
  eventsStore.connect()
})

onBeforeUnmount(() => {
  eventsStore.disconnect()
})
</script>
