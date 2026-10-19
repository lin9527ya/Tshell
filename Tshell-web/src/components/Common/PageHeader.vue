<script setup lang="ts">
// 页面统一头部（vshell 骨架）：20px 标题 + 副标题 + 右侧操作。
// 渲染在页面背景上（透明），页签由各页在头部下方自行摆放。
defineProps<{
  title: string
  subtitle?: string
}>()
</script>

<template>
  <div class="page-header">
    <div class="page-header-left">
      <h1 class="page-title">{{ title }}</h1>
      <span v-if="subtitle" class="page-subtitle">{{ subtitle }}</span>
      <div class="page-header-extra">
        <slot name="left-extra" />
      </div>
    </div>
    <div class="page-header-right">
      <slot />
    </div>
  </div>
</template>

<style scoped>
.page-header {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 16px 20px 8px;
  background: transparent;
}

.page-header-left {
  display: flex;
  align-items: baseline;
  gap: 12px;
  min-width: 0;
  flex-wrap: wrap;
}

.page-header-extra {
  display: flex;
  align-items: center;
  gap: 8px;
}

.page-title {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
  line-height: 1.4;
  color: var(--cs-text-bright);
  white-space: nowrap;
}

.page-subtitle {
  font-size: 14px;
  color: var(--cs-text-dim);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
}

.page-header-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

@media (max-width: 1100px) {
  .page-header {
    flex-wrap: wrap;
    row-gap: 8px;
  }
}
</style>
