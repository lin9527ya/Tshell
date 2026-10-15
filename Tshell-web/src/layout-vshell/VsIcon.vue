<script setup lang="ts">
import { computed } from 'vue'
import { VS_ICONS } from './vs-icons'

// 内联 Iconify 图标（数据从 vshell 还原包抽取 + 同集合补齐），零运行时依赖
const props = withDefaults(defineProps<{ name: string; size?: number | string }>(), {
  size: 16,
})

const rec = computed(() => VS_ICONS[props.name])
</script>

<template>
  <svg
    v-if="rec"
    class="vs-icon"
    :viewBox="rec.v"
    :width="size"
    :height="size"
    aria-hidden="true"
    v-html="rec.b"
  />
</template>

<style scoped>
.vs-icon {
  display: inline-block;
  vertical-align: -0.15em;
  flex-shrink: 0;
}
</style>
