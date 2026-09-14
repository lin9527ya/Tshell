<script setup lang="ts">
// 页面统一头部：标题+副标题+右侧操作区
defineProps<{
  title: string
  subtitle?: string
}>()
</script>

<template>
  <div class="page-header">
    <div class="page-header-left">
      <h2 class="page-title">{{ title }}</h2>
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
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 16px 20px;
  background: var(--bg-panel, #161b22);
  border-bottom: 1px solid var(--border-color, #30363d);
  flex-shrink: 0;
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
  gap: 6px;
}

.page-title {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: var(--text-primary, #e6edf3);
  white-space: nowrap;
}

.page-subtitle {
  font-size: 12px;
  color: var(--text-tertiary, #6e7681);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.page-header-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}
</style>
