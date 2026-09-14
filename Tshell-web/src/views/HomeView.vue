<script setup lang="ts">
import { onMounted } from 'vue'
import CsShell from '@/layout/CsShell.vue'
import { useThemeStore } from '@/stores/theme'

// Home 现在只负责挂载 CS 工作台，不再保留旧的 TopHeader/TagsView/router-view 布局。
const themeStore = useThemeStore()
onMounted(() => themeStore.init())
</script>

<template>
  <CsShell />
</template>
