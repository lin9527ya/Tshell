<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import VsIcon from './VsIcon.vue'
import PreferencesDialog from '@/layout/Dialogs/PreferencesDialog.vue'
import ReportsDialog from '@/layout/Dialogs/ReportsDialog.vue'
import { deriveMenu } from '@/router/routes'
import { useI18nStore } from '@/stores/i18n'
import { useThemeStore } from '@/stores/theme'
import { useUserStore } from '@/stores/user'
import { usePermissStore } from '@/stores/userpermiss'
import logoUrl from '@/assets/logo.svg'

const props = defineProps<{ activeKey: string }>()

const i18nStore = useI18nStore()
const themeStore = useThemeStore()
const userStore = useUserStore()
const permissStore = usePermissStore()
const router = useRouter()

const isZh = computed(() => i18nStore.isZh)
const isDark = computed(() => themeStore.isDark)
const userMenuOpen = ref(false)
const isFullscreen = ref(false)
// 顶栏用户菜单内的局部弹窗（不再走全局 workspace store）
const showPreferences = ref(false)
const showReports = ref(false)

// 顶栏 8 项由路由表派生（meta.menu !== false），按权限过滤
const navItems = computed(() =>
  deriveMenu().filter((m) => !m.permiss || permissStore.key.includes(m.permiss)),
)

const label = (m: { title: string; titleEn: string }) => (isZh.value ? m.title : m.titleEn)

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
}

onMounted(() => {
  document.addEventListener('click', onDocClick)
  document.addEventListener('fullscreenchange', onFsChange)
})
onBeforeUnmount(() => {
  document.removeEventListener('click', onDocClick)
  document.removeEventListener('fullscreenchange', onFsChange)
})
</script>

<template>
  <header class="vs-topnav">
    <div class="vs-topnav-logo">
      <img :src="logoUrl" alt="logo" />
      <span class="vs-topnav-title">BladeShell</span>
    </div>

    <nav class="vs-topnav-menu">
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
    <PreferencesDialog v-model="showPreferences" />
    <ReportsDialog v-model="showReports" />
  </header>
</template>
