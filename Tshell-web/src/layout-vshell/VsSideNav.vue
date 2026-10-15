<script setup lang="ts">
// 左侧菜单栏（导航栏模式 sidebar / mix / mix-sidebar）。
// 结构与 vshell 实测一致：logo 位于侧栏顶部，导航项在其下；
// rail（mix-sidebar）为 80px 图标栏，collapsed 为折叠态（仅图标，64px）。
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import VsIcon from './VsIcon.vue'
import { deriveMenu } from '@/router/routes'
import { useI18nStore } from '@/stores/i18n'
import { usePermissStore } from '@/stores/userpermiss'
import logoUrl from '@/assets/logo.ico'

const props = withDefaults(
  defineProps<{ activeKey: string; collapsed: boolean; rail?: boolean; showLogo?: boolean }>(),
  { rail: false, showLogo: true },
)

const router = useRouter()
const i18nStore = useI18nStore()
const permissStore = usePermissStore()
const isZh = computed(() => i18nStore.isZh)

const condensed = computed(() => props.collapsed || props.rail)

// 与顶栏同源（路由表派生 + 权限过滤）
const items = computed(() =>
  deriveMenu().filter((m) => !m.permiss || permissStore.key.includes(m.permiss)),
)

const label = (m: { title: string; titleEn: string }) => (isZh.value ? m.title : m.titleEn)
</script>

<template>
  <aside class="vs-sidenav" :class="{ 'is-collapsed': props.collapsed, 'is-rail': props.rail }">
    <div v-if="props.showLogo" class="vs-side-logo">
      <img :src="logoUrl" alt="logo" />
      <span v-if="!condensed" class="vs-side-logo-text">Tshell</span>
    </div>

    <div class="vs-side-items">
      <div
        v-for="item in items"
        :key="item.name"
        class="vs-side-item"
        :class="{ active: props.activeKey === item.name }"
        :title="condensed ? label(item) : ''"
        @click="router.push(item.path)"
      >
        <VsIcon v-if="item.icon" :name="item.icon" :size="18" class="vs-side-icon" />
        <span v-if="!condensed" class="vs-side-text">{{ label(item) }}</span>
        <span v-else-if="props.rail" class="vs-side-rail-text">{{ label(item) }}</span>
      </div>
    </div>
  </aside>
</template>
