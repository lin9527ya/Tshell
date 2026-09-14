<script setup lang="ts">
import { computed, nextTick, provide, ref } from 'vue'
import { ElConfigProvider } from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import en from 'element-plus/es/locale/lang/en'
import { useI18nStore } from '@/stores/i18n'

const i18nStore = useI18nStore()

const elLocale = computed(() => {
  return i18nStore.currentLocale === 'zh' ? zhCn : en
})

// 局部组件刷新
const isRouterAlive = ref(true)
const reload = () => {
  isRouterAlive.value = false
  nextTick(() => {
    isRouterAlive.value = true
  })
}
provide('reload', reload)
</script>

<template>
  <el-config-provider :locale="elLocale">
    <RouterView v-if="isRouterAlive" />
  </el-config-provider>
</template>

<style scoped>
</style>
