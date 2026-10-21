<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import VsIcon from './VsIcon.vue'
import ThemeDrawer from '@/layout-vshell/ThemeDrawer.vue'
import ReportsDialog from '@/layout/Dialogs/ReportsDialog.vue'
import { BUSINESS_ROUTES, deriveMenu, type AppRouteMeta } from '@/router/routes'
import { useI18nStore } from '@/stores/i18n'
import { useThemeStore } from '@/stores/theme'
import { useUserStore } from '@/stores/user'
import { usePermissStore } from '@/stores/userpermiss'
import { useCollabStore } from '@/stores/collab'
import logoUrl from '@/assets/logo.ico'

const props = withDefaults(
  defineProps<{ activeKey: string; showMenu?: boolean; variant?: 'full' | 'slim' | 'mix' }>(),
  { showMenu: true, variant: 'full' },
)
const emit = defineEmits<{ (e: 'toggle-side'): void }>()

const i18nStore = useI18nStore()
const themeStore = useThemeStore()
const userStore = useUserStore()
const permissStore = usePermissStore()
const collabStore = useCollabStore()
const router = useRouter()

const isZh = computed(() => i18nStore.isZh)
const isDark = computed(() => themeStore.isDark)
const userMenuOpen = ref(false)
const collabOpen = ref(false)
const isFullscreen = ref(false)
// 顶栏用户菜单内的局部弹窗（不再走全局 workspace store）
const showPreferences = ref(false)
const showReports = ref(false)

// 顶栏 8 项由路由表派生（meta.menu !== false），按权限过滤
const navItems = computed(() =>
  deriveMenu().filter((m) => !m.permiss || permissStore.key.includes(m.permiss)),
)

const label = (m: { title: string; titleEn: string }) => (isZh.value ? m.title : m.titleEn)

// 拓扑图入口的可见性：与 /graph 路由的 permiss 保持一致（后端 /graph/data 是 operatorOnly）。
// 直接读路由表而不是硬编码权限串，避免两处漂移。
const canViewGraph = computed(() => {
  const route = BUSINESS_ROUTES.find((r) => r.name === 'PivotGraph')
  const need = (route?.meta as AppRouteMeta | undefined)?.permiss
  return !need || permissStore.key.includes(need)
})

// 副栏（左侧菜单模式）顶栏的面包屑 = 当前菜单项标题
const activeTitle = computed(() => {
  const hit = navItems.value.find((m) => m.name === props.activeKey)
  return hit ? label(hit) : ''
})

// 顶栏菜单自适应折叠：
//   宽度够 → 全部平铺（整体居中）；
//   宽度不够 → 末尾收成「…」，点开是没显示出来的项；窗口变宽再逐项展开。
// 宽度用隐藏的度量行取（不随折叠状态变化），否则量到的是被折叠后的宽度，会来回抖动。
const menuRef = ref<HTMLElement | null>(null)
const measureRef = ref<HTMLElement | null>(null)
const menuWidth = ref(0)
const itemWidths = ref<number[]>([])
const moreOpen = ref(false)
const MORE_BTN_WIDTH = 48

const visibleCount = computed(() => {
  const total = navItems.value.length
  if (!total) return 0
  if (!menuWidth.value || itemWidths.value.length !== total) return total
  let used = 0
  let count = 0
  for (let i = 0; i < total; i++) {
    const w = itemWidths.value[i] || 0
    // 只要还有项被折叠，就必须给「…」留位
    const reserve = i === total - 1 ? 0 : MORE_BTN_WIDTH
    if (used + w + reserve > menuWidth.value) break
    used += w
    count++
  }
  // 极端窄屏也至少留第一项，否则导航区就空了
  return Math.max(count, Math.min(1, total))
})

const visibleNavItems = computed(() => navItems.value.slice(0, visibleCount.value))
const hiddenNavItems = computed(() => navItems.value.slice(visibleCount.value))
const hiddenCount = computed(() => hiddenNavItems.value.length)

function measureMenu() {
  const host = menuRef.value
  const probe = measureRef.value
  if (!host || !probe) return
  menuWidth.value = host.clientWidth
  const kids = Array.from(probe.children) as HTMLElement[]
  itemWidths.value = kids.map((k) => k.getBoundingClientRect().width)
}

let menuObserver: ResizeObserver | null = null

function scheduleMeasure() {
  nextTick(() => {
    measureMenu()
    // 字体晚到（首次加载图标字体/中文字体回退）会让宽度变一次，补量一拍
    window.setTimeout(measureMenu, 120)
  })
}

watch([() => navItems.value.length, isZh], () => {
  moreOpen.value = false
  scheduleMeasure()
})

function onNavClick(path: string) {
  router.push(path)
  moreOpen.value = false
  userMenuOpen.value = false}

function toggleFullscreen() {
  if (document.fullscreenElement) {
    document.exitFullscreen().catch(() => {})
  } else {
    document.documentElement.requestFullscreen().catch(() => {})
  }
}

function onFsChange() {
  isFullscreen.value = !!document.fullscreenElement
}

function toggleTheme() {
  themeStore.setTheme(isDark.value ? 'light' : 'dark')
  userMenuOpen.value = false
}

function onUserCmd(cmd: string) {
  userMenuOpen.value = false
  if (cmd === 'preferences') showPreferences.value = true
  else if (cmd === 'reports') showReports.value = true
  else if (cmd === 'about') router.push('/about')
  else if (cmd === 'theme') toggleTheme()
  else if (cmd === 'logout') userStore.logout()
}

function onDocClick(e: MouseEvent) {
  const el = e.target as HTMLElement
  if (!el.closest('.vs-user-wrap')) userMenuOpen.value = false
  if (!el.closest('.vs-collab-wrap')) collabOpen.value = false
}

onMounted(() => {
  document.addEventListener('click', onDocClick)
  document.addEventListener('fullscreenchange', onFsChange)
  collabStore.start()
  scheduleMeasure()
  if (typeof ResizeObserver !== 'undefined' && menuRef.value) {
    menuObserver = new ResizeObserver(() => measureMenu())
    menuObserver.observe(menuRef.value)
  }
})
onBeforeUnmount(() => {
  document.removeEventListener('click', onDocClick)
  document.removeEventListener('fullscreenchange', onFsChange)
  collabStore.stop()
  menuObserver?.disconnect()
  menuObserver = null
})

// 会话锁到期时间（本地时区 HH:MM），用于提示"占用到几点"
function lockUntil(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}
</script>

<template>
  <header class="vs-topnav">
    <div v-if="props.variant === 'full'" class="vs-topnav-logo">
      <img :src="logoUrl" alt="logo" />
      <span class="vs-topnav-title">Tshell</span>
    </div>

    <!-- mix 模式：logo 块用菜单底色，嵌在整宽顶栏左端 -->
    <div v-if="props.variant === 'mix'" class="vs-topnav-logo vs-mix-logo">
      <img :src="logoUrl" alt="logo" />
      <span class="vs-topnav-title">Tshell</span>
    </div>

    <!-- 左侧菜单模式的副栏：汉堡（收起/展开侧栏）+ 当前页面标题 -->
    <template v-if="props.variant !== 'full'">
      <button class="vs-action-btn vs-side-toggle" :title="i18nStore.tl('收起/展开菜单', 'Collapse menu')" @click="emit('toggle-side')">
        <VsIcon name="list-ul" :size="18" />
      </button>
      <span class="vs-topnav-title vs-breadcrumb">{{ activeTitle }}</span>
    </template>

    <nav v-if="props.showMenu" ref="menuRef" class="vs-topnav-menu">
      <div
        v-for="item in visibleNavItems"
        :key="item.name"
        class="vs-nav-item"
        :class="{ active: props.activeKey === item.name }"
        :title="label(item)"
        @click="onNavClick(item.path)"
      >
        <VsIcon v-if="item.icon" :name="item.icon" :size="20" class="vs-nav-icon" />
        <span class="vs-nav-text">{{ label(item) }}</span>
      </div>

      <!-- 放不下的项收进「…」，点开列出；窗口变宽会自动逐项展开 -->
      <div
        v-if="hiddenCount > 0"
        class="vs-nav-item vs-nav-more"
        :class="{ open: moreOpen }"
        :title="i18nStore.tl('更多', 'More')"
        @click.stop="moreOpen = !moreOpen"
      >
        <VsIcon name="ellipsis-h" :size="18" class="vs-nav-icon" />
      </div>

      <transition name="vs-pop">
        <div v-if="hiddenCount > 0 && moreOpen" class="vs-nav-popup vs-nav-more-popup" @click.stop>
          <div
            v-for="item in hiddenNavItems"
            :key="item.name"
            class="vs-popup-item"
            :class="{ active: props.activeKey === item.name }"
            :title="label(item)"
            @click="onNavClick(item.path)"
          >
            <VsIcon v-if="item.icon" :name="item.icon" :size="16" />
            <span>{{ label(item) }}</span>
          </div>
        </div>
      </transition>
    </nav>

    <!-- 度量行：只用来量每项宽度，不可见、不占位 -->
    <div ref="measureRef" class="vs-nav-measure" aria-hidden="true">
      <div v-for="item in navItems" :key="`m-${item.name}`" class="vs-nav-item">
        <VsIcon v-if="item.icon" :name="item.icon" :size="20" class="vs-nav-icon" />
        <span class="vs-nav-text">{{ label(item) }}</span>
      </div>
    </div>

    <div class="vs-topnav-actions">
      <button class="vs-action-btn" :title="i18nStore.tl('全屏', 'Fullscreen')" @click.stop="toggleFullscreen">
        <VsIcon :name="isFullscreen ? 'compress' : 'expand'" :size="16" />
      </button>

      <button class="vs-action-btn" :title="i18nStore.tl('切换语言', 'Language')" @click.stop="i18nStore.toggleLocale()">
        <VsIcon name="language" :size="16" />
      </button>

      <!-- 拓扑网络图入口：隐藏路由（不进顶栏 8 项），从这里进 -->
      <button
        v-if="canViewGraph"
        class="vs-action-btn"
        :title="i18nStore.tl('拓扑网络图', 'Pivot Graph')"
        @click.stop="router.push('/graph')"
      >
        <VsIcon name="project-diagram" :size="16" />
      </button>

      <div class="vs-collab-wrap">
        <button
          class="vs-action-btn vs-collab-btn"
          :title="i18nStore.tl('在线操作员与会话占用', 'Online operators & session locks')"
          @click.stop="collabOpen = !collabOpen"
        >
          <VsIcon name="users" :size="16" />
          <span class="vs-collab-count">{{ collabStore.operators.length }}</span>
        </button>

        <transition name="vs-pop">
          <div v-if="collabOpen" class="vs-nav-popup vs-collab-popup">
            <div class="vs-collab-title">{{ i18nStore.tl('在线操作员', 'Online operators') }}</div>
            <div v-if="!collabStore.operators.length" class="vs-collab-empty">
              {{ i18nStore.tl('暂无其他在线操作员', 'No operators online') }}
            </div>
            <div v-for="op in collabStore.operators" :key="op.username" class="vs-collab-row">
              <span class="vs-collab-dot" :class="`vs-role-${op.role}`" />
              <span class="vs-collab-name">{{ op.username }}</span>
              <span class="vs-collab-meta">{{ op.role }}</span>
            </div>

            <div class="vs-collab-title vs-collab-title-locks">
              {{ i18nStore.tl('会话占用', 'Session locks') }}
            </div>
            <div v-if="!collabStore.locks.length" class="vs-collab-empty">
              {{ i18nStore.tl('当前没有会话被占用', 'No sessions locked') }}
            </div>
            <div v-for="l in collabStore.locks" :key="l.uid" class="vs-collab-row">
              <VsIcon name="lock" :size="12" class="vs-collab-lock" />
              <span class="vs-collab-name">{{ l.uid.slice(0, 8) }}</span>
              <span class="vs-collab-meta">{{ l.owner }} · {{ lockUntil(l.expires_at) }}</span>
            </div>
          </div>
        </transition>
      </div>

      <div class="vs-user-wrap">
        <div class="vs-user" @click.stop="userMenuOpen = !userMenuOpen">
          <span class="vs-user-name">{{ userStore.username || 'admin' }}</span>
          <VsIcon name="ellipsis-v" :size="10" class="vs-user-caret" />
        </div>

        <transition name="vs-pop">
          <div v-if="userMenuOpen" class="vs-nav-popup vs-user-popup">
            <div class="vs-popup-item" @click.stop="onUserCmd('preferences')">
              <VsIcon name="sliders-h" :size="16" /><span>{{ i18nStore.tl('偏好设置', 'Preferences') }}</span>
            </div>
            <div class="vs-popup-item" @click.stop="onUserCmd('theme')">
              <VsIcon :name="isDark ? 'sun' : 'moon'" :size="16" />
              <span>{{ isDark ? (i18nStore.tl('浅色模式', 'Light mode')) : (i18nStore.tl('深色模式', 'Dark mode')) }}</span>
            </div>
            <div class="vs-popup-item" @click.stop="onUserCmd('reports')">
              <VsIcon name="chart-bar" :size="16" /><span>{{ i18nStore.tl('报告中心', 'Reports') }}</span>
            </div>
            <div class="vs-popup-item" @click.stop="onUserCmd('about')">
              <VsIcon name="info-circle" :size="16" /><span>{{ i18nStore.tl('关于', 'About') }}</span>
            </div>
            <div class="vs-popup-divider" />
            <div class="vs-popup-item" @click.stop="onUserCmd('logout')">
              <VsIcon name="sign-out-alt" :size="16" /><span>{{ i18nStore.tl('断开连接', 'Disconnect') }}</span>
            </div>
          </div>
        </transition>
      </div>

      <button class="vs-action-btn" :title="i18nStore.tl('偏好设置', 'Preferences')" @click.stop="onUserCmd('preferences')">
        <VsIcon name="gear" :size="16" />
      </button>
    </div>

    <!-- 用户菜单宿主弹窗（局部挂载；append-to-body 渲染到 body） -->
    <ThemeDrawer v-model="showPreferences" />
    <ReportsDialog v-model="showReports" />
  </header>
</template>
