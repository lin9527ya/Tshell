<script setup lang="ts">
// 会话/目标 OS 图标单一来源(此前 SessionTable 与 TargetsView 逐字节重复 3 份)
// variant: detail = 完整 Tux 渐变细节(表格行内); simple = 简化版(卡片/大图标)
// fallback: none 不渲染 / screen 显示器轮廓(可带在线底色) / circle 圆环
withDefaults(
  defineProps<{
    os?: string
    size?: number
    variant?: 'detail' | 'simple'
    fallback?: 'none' | 'screen' | 'circle'
    online?: boolean
  }>(),
  { os: '', size: 16, variant: 'detail', fallback: 'none', online: false },
)
</script>

<template>
  <!-- Windows -->
  <svg v-if="os === 'windows'" viewBox="0 0 18 18" :width="size" :height="size" class="os-icon">
    <path d="M1 3.2 L7.8 2.2 L7.8 8.8 L1 8.8 Z" fill="#00adef" />
    <path d="M8.8 2.1 L17 0.9 L17 8.8 L8.8 8.8 Z" fill="#00adef" />
    <path d="M1 9.8 L7.8 9.8 L7.8 16.4 L1 15.4 Z" fill="#00adef" />
    <path d="M8.8 9.8 L17 9.8 L17 17.7 L8.8 16.5 Z" fill="#00adef" />
  </svg>

  <!-- Linux Tux：detail 保留完整眼部/喙细节 -->
  <svg v-else-if="os === 'linux'" viewBox="0 0 18 18" :width="size" :height="size" class="os-icon">
    <path
      d="M9 1.5 C6.5 1.5, 5 3.5, 5 6.5 C5 8, 4.5 9.5, 3.5 11 C2.8 12.2, 2.8 13.8, 3.8 14.8 C4.8 15.8, 6.5 16.2, 9 16.2 C11.5 16.2, 13.2 15.8, 14.2 14.8 C15.2 13.8, 15.2 12.2, 14.5 11 C13.5 9.5, 13 8, 13 6.5 C13 3.5, 11.5 1.5, 9 1.5 Z"
      fill="#222"
      :stroke="variant === 'detail' ? '#111' : undefined"
      :stroke-width="variant === 'detail' ? 0.5 : undefined"
    />
    <ellipse cx="9" cy="11.2" rx="3.3" ry="3.8" fill="#ffffff" />
    <template v-if="variant === 'detail'">
      <ellipse cx="7.3" cy="5" rx="1" ry="1.4" fill="#ffffff" />
      <ellipse cx="10.7" cy="5" rx="1" ry="1.4" fill="#ffffff" />
      <circle cx="7.4" cy="5.2" r="0.5" fill="#000000" />
      <circle cx="10.6" cy="5.2" r="0.5" fill="#000000" />
    </template>
    <polygon
      points="7.2,6.5 10.8,6.5 9,8.2"
      fill="#ffb300"
      :stroke="variant === 'detail' ? '#f57c00' : undefined"
      :stroke-width="variant === 'detail' ? 0.3 : undefined"
    />
    <ellipse v-if="variant === 'detail'" cx="6.2" cy="16.2" rx="2" ry="0.9" fill="#ffb300" />
    <ellipse v-if="variant === 'detail'" cx="11.8" cy="16.2" rx="2" ry="0.9" fill="#ffb300" />
  </svg>

  <!-- macOS -->
  <svg v-else-if="os === 'darwin'" viewBox="0 0 18 18" :width="size" :height="size" class="os-icon">
    <path
      d="M11.5 2 C12 1.2 12.5 0.5 12.2 0 C11.6 0.1 10.8 0.6 10.3 1.3 C9.8 1.8 9.4 2.6 9.6 3 C10.4 3 11 2.5 11.5 2 Z M13.8 9 C13.8 7.2 15.2 6.3 15.3 6.2 C14.4 5 13.1 4.8 12.6 4.8 C11.4 4.7 10.3 5.5 9.7 5.5 C9.1 5.5 8.2 4.8 7.3 4.8 C6.1 4.8 5 5.5 4.3 6.6 C3 8.8 4 12.1 5.3 14 C5.9 14.8 6.6 15.8 7.7 15.8 C8.7 15.8 9 15.2 10.2 15.2 C11.3 15.2 11.7 15.8 12.7 15.8 C13.8 15.8 14.4 14.9 15.1 14 C15.9 12.9 16.2 11.8 16.2 11.7 C16.1 11.7 13.8 10.8 13.8 9 Z"
      fill="var(--cs-text-bright)"
    />
  </svg>

  <!-- 兜底：显示器轮廓(在线时屏幕亮色) -->
  <svg v-else-if="fallback === 'screen'" viewBox="0 0 24 24" :width="size" :height="size" fill="currentColor" class="os-icon">
    <rect x="2" y="3" width="20" height="14" rx="2" stroke="currentColor" stroke-width="1.5" fill="none" />
    <path d="M8 21h8M12 17v4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" />
    <rect v-if="online" x="4" y="5" width="16" height="10" rx="1" fill="var(--cs-online, #4caf50)" opacity="0.8" />
    <rect v-else x="4" y="5" width="16" height="10" rx="1" fill="currentColor" opacity="0.25" />
  </svg>

  <!-- 兜底：圆环 -->
  <svg v-else-if="fallback === 'circle'" viewBox="0 0 18 18" :width="size" :height="size" class="os-icon">
    <circle cx="9" cy="9" r="8" fill="none" stroke="currentColor" stroke-width="1.5" />
  </svg>
</template>

<style scoped>
.os-icon {
  display: block;
  flex-shrink: 0;
}
</style>
