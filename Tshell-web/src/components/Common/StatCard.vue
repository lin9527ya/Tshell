<script setup lang="ts">
import { computed } from 'vue'

// 统计卡片：图标 + 数值 + 标签 + 可选色调
const props = withDefaults(defineProps<{
  label: string
  value: number | string
  icon: any
  color?: 'primary' | 'success' | 'warning' | 'danger' | 'info'
  suffix?: string
}>(), {
  color: 'primary',
  suffix: ''
})

const toneClass = computed(() => `tone-${props.color}`)
</script>

<template>
  <div class="stat-card" :class="toneClass">
    <div class="stat-icon">
      <el-icon :size="22"><component :is="icon" /></el-icon>
    </div>
    <div class="stat-body">
      <div class="stat-value">
        {{ value }}<span v-if="suffix" class="stat-suffix">{{ suffix }}</span>
      </div>
      <div class="stat-label">{{ label }}</div>
    </div>
  </div>
</template>

<style scoped>
.stat-card {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 16px;
  background: var(--bg-panel, #161b22);
  border: 1px solid var(--border-color, #30363d);
  border-radius: 10px;
  transition: border-color 0.15s ease, transform 0.15s ease;
}

.stat-card:hover {
  transform: translateY(-1px);
}

.stat-icon {
  width: 46px;
  height: 46px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.tone-primary .stat-icon {
  background: color-mix(in srgb, var(--theme-primary, #1f6feb) 15%, transparent);
  color: var(--theme-primary, #1f6feb);
}

.tone-success .stat-icon {
  background: color-mix(in srgb, var(--theme-success, #3fb950) 15%, transparent);
  color: var(--theme-success, #3fb950);
}

.tone-warning .stat-icon {
  background: color-mix(in srgb, var(--theme-warning, #d29922) 15%, transparent);
  color: var(--theme-warning, #d29922);
}

.tone-danger .stat-icon {
  background: color-mix(in srgb, var(--theme-danger, #f85149) 15%, transparent);
  color: var(--theme-danger, #f85149);
}

.tone-info .stat-icon {
  background: color-mix(in srgb, var(--theme-info, #58a6ff) 15%, transparent);
  color: var(--theme-info, #58a6ff);
}

.stat-body {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.stat-value {
  font-size: 24px;
  font-weight: 700;
  color: var(--text-primary, #e6edf3);
  line-height: 1.1;
  font-family: var(--font-family-mono, monospace);
}

.stat-suffix {
  font-size: 13px;
  font-weight: 400;
  color: var(--text-tertiary, #6e7681);
  margin-left: 2px;
}

.stat-label {
  font-size: 12px;
  color: var(--text-secondary, #8b949e);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
