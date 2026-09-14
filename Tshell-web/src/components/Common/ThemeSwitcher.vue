<template>
  <div class="theme-switcher">
    <!-- 快速主题切换 -->
    <el-dropdown @command="handleThemeChange" trigger="click">
      <el-button :icon="isDark ? 'Moon' : 'Sunny'" circle size="large" />
      <template #dropdown>
        <el-dropdown-menu>
          <el-dropdown-item 
            v-for="[name, theme] in Object.entries(availableThemes)" 
            :key="name"
            :command="name"
            :class="{ 'is-active': currentTheme === name }"
          >
            <div class="theme-option">
              <div 
                class="theme-preview" 
                :style="getThemePreviewStyle(theme)"
              />
              <span>{{ getThemeDisplayName(name) }}</span>
            </div>
          </el-dropdown-item>
        </el-dropdown-menu>
      </template>
    </el-dropdown>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useThemeStore } from '@/stores/theme'
import { ElMessage } from 'element-plus'
import { Sunny, Moon } from '@element-plus/icons-vue'

const themeStore = useThemeStore()

// 计算属性
const currentTheme = computed(() => themeStore.currentTheme)
const availableThemes = computed(() => themeStore.getAvailableThemes)
const isDark = computed(() => themeStore.isDark)

// 方法
const handleThemeChange = (themeName: string) => {
  setTheme(themeName)
}

const setTheme = (themeName: string) => {
  themeStore.setTheme(themeName)
  ElMessage.success(`已切换到 ${getThemeDisplayName(themeName)} 主题`)
}

const getThemeDisplayName = (name: string): string => {
  const nameMap: Record<string, string> = {
    light: '浅色主题',
    dark: '深色主题'
  }
  return nameMap[name] || name
}

const getThemePreviewStyle = (theme: any) => {
  return {
    background: `linear-gradient(45deg, ${theme.backgroundColor} 50%, ${theme.primaryColor} 50%)`,
    border: `1px solid ${theme.borderColor || '#e4e7ed'}`
  }
}
</script>

<style scoped>
.theme-switcher {
  display: inline-block;
}

.theme-option {
  display: flex;
  align-items: center;
  gap: 8px;
}

.theme-preview {
  width: 16px;
  height: 16px;
  border-radius: 2px;
}

.is-active {
  background-color: var(--el-color-primary-light-9);
}
</style>