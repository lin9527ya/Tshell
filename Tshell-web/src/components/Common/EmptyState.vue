<script setup lang="ts">
// 统一空状态：图标(emoji/字符) + 标题 + 说明
// overlay 变体居中悬浮于容器(父级需 position:relative)，不拦截鼠标；
// inline 变体随文档流排布，可放交互按钮(slot)
withDefaults(
  defineProps<{
    title: string
    icon?: string
    desc?: string
    variant?: 'overlay' | 'inline'
  }>(),
  { variant: 'overlay' },
)
</script>

<template>
  <div class="cs-empty" :class="`cs-empty--${variant}`">
    <div v-if="icon" class="cs-empty-icon">{{ icon }}</div>
    <div class="cs-empty-title">{{ title }}</div>
    <div v-if="desc" class="cs-empty-desc">{{ desc }}</div>
    <slot />
  </div>
</template>

<style scoped>
.cs-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  text-align: center;
}

.cs-empty--overlay {
  position: absolute;
  top: 45%;
  left: 50%;
  transform: translate(-50%, -50%);
  pointer-events: none;
  max-width: 90%;
}

.cs-empty--inline {
  padding: 24px 16px;
}

.cs-empty-icon {
  font-size: 26px;
  line-height: 1;
  opacity: 0.75;
}

.cs-empty-title {
  font-size: 13px;
  color: var(--cs-text-dim, #6e7681);
}

.cs-empty-desc {
  font-size: 11px;
  color: var(--cs-text-dim, #6e7681);
  opacity: 0.85;
  max-width: 420px;
  line-height: 1.6;
}
</style>
