<script setup lang="ts">
import { onMounted } from 'vue'
import VShellApp from '@/layout-vshell/VShellApp.vue'
import { useThemeStore } from '@/stores/theme'

// 【vshell-ui 分支】Home 渲染 VShell 风格外壳(侧边导航 + 顶栏 + 多标签)。
// CsShell(CS 风格)保留未动:切回 main 分支即恢复 CS 界面。
const themeStore = useThemeStore()
onMounted(() => themeStore.init())
</script>

<template>
  <VShellApp />
</template>
