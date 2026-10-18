<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
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

function onNavClick(path: string) {
  router.push(path)
  userMenuOpen.value = false
}

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
})
onBeforeUnmount(() => {
  document.removeEventListener('click', onDocClick)
  document.removeEventListener('fullscreenchange', onFsChange)
  collabStore.stop()
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
      <button class="vs-action-btn vs-side-toggle" :title="isZh ? '收起/展开菜单' : 'Collapse menu'" @click="emit('toggle-side')">
        <VsIcon name="list-ul" :size="18" />
      </button>
      <span class="vs-topnav-title vs-breadcrumb">{{ activeTitle }}</span>
    </template>

    <nav v-if="props.showMenu" class="vs-topnav-menu">
      <div
        v-for="item in navItems"
        :key="item.name"
        class="vs-nav-item"
        :class="{ active: props.activeKey === item.name }"
        :title="label(item)"
        @click="onNavClick(item.path)"
      >
        <VsIcon v-if="item.icon" :name="item.icon" :size="20" class="vs-nav-icon" />
        <span class="vs-nav-text">{{ label(item) }}</span>
      </div>
    </nav>

    <div class="vs-topnav-actions">
      <button class="vs-action-btn" :title="isZh ? '全屏' : 'Fullscreen'" @click.stop="toggleFullscreen">
        <VsIcon :name="isFullscreen ? 'compress' : 'expand'" :size="16" />
      </button>

      <button class="vs-action-btn" :title="isZh ? '切换语言' : 'Language'" @click.stop="i18nStore.toggleLocale()">
        <VsIcon name="language" :size="16" />
      </button>

      <!-- 拓扑网络图入口：隐藏路由（不进顶栏 8 项），从这里进 -->
      <button
        v-if="canViewGraph"
        class="vs-action-btn"
        :title="isZh ? '拓扑网络图' : 'Pivot Graph'"
        @click.stop="router.push('/graph')"
      >
        <VsIcon name="project-diagram" :size="16" />
      </button>

      <div class="vs-collab-wrap">
        <button
          class="vs-action-btn vs-collab-btn"
          :title="isZh ? '在线操作员与会话占用' : 'Online operators & session locks'"
          @click.stop="collabOpen = !collabOpen"
        >
          <VsIcon name="users" :size="16" />
          <span class="vs-collab-count">{{ collabStore.operators.length }}</span>
        </button>

        <transition name="vs-pop">
          <div v-if="collabOpen" class="vs-nav-popup vs-collab-popup">
            <div class="vs-collab-title">{{ isZh ? '在线操作员' : 'Online operators' }}</div>
            <div v-if="!collabStore.operators.length" class="vs-collab-empty">
              {{ isZh ? '暂无其他在线操作员' : 'No operators online' }}
            </div>
            <div v-for="op in collabStore.operators" :key="op.username" class="vs-collab-row">
              <span class="vs-collab-dot" :class="`vs-role-${op.role}`" />
              <span class="vs-collab-name">{{ op.username }}</span>
              <span class="vs-collab-meta">{{ op.role }}</span>
            </div>

            <div class="vs-collab-title vs-collab-title-locks">
              {{ isZh ? '会话占用' : 'Session locks' }}
            </div>
            <div v-if="!collabStore.locks.length" class="vs-collab-empty">
              {{ isZh ? '当前没有会话被占用' : 'No sessions locked' }}
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
              <VsIcon name="sliders-h" :size="16" /><span>{{ isZh ? '偏好设置' : 'Preferences' }}</span>
            </div>
            <div class="vs-popup-item" @click.stop="onUserCmd('theme')">
              <VsIcon :name="isDark ? 'sun' : 'moon'" :size="16" />
              <span>{{ isDark ? (isZh ? '浅色模式' : 'Light mode') : (isZh ? '深色模式' : 'Dark mode') }}</span>
            </div>
            <div class="vs-popup-item" @click.stop="onUserCmd('reports')">
              <VsIcon name="chart-bar" :size="16" /><span>{{ isZh ? '报告中心' : 'Reports' }}</span>
            </div>
            <div class="vs-popup-item" @click.stop="onUserCmd('about')">
              <VsIcon name="info-circle" :size="16" /><span>{{ isZh ? '关于' : 'About' }}</span>
            </div>
            <div class="vs-popup-divider" />
            <div class="vs-popup-item" @click.stop="onUserCmd('logout')">
              <VsIcon name="sign-out-alt" :size="16" /><span>{{ isZh ? '断开连接' : 'Disconnect' }}</span>
            </div>
          </div>
        </transition>
      </div>

      <button class="vs-action-btn" :title="isZh ? '偏好设置' : 'Preferences'" @click.stop="onUserCmd('preferences')">
        <VsIcon name="gear" :size="16" />
      </button>
    </div>

    <!-- 用户菜单宿主弹窗（局部挂载；append-to-body 渲染到 body） -->
    <ThemeDrawer v-model="showPreferences" />
    <ReportsDialog v-model="showReports" />
  </header>
</template>
