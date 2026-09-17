// 主题管理Store
// 【全面优化】裁剪为 CS 壳实际使用的配置: mode(明暗) + primaryColor(主色)。
// 原 layout/sidebar/navMode/headerTheme/customTheme 等配置写入的 html class
// 无任何消费方(.modern-header/.main-content 元素已不存在),随本次优化移除。
import { defineStore } from 'pinia'
import { ref, watch, computed } from 'vue'

export type ThemeMode = 'light' | 'dark' | 'auto'

export interface ThemeConfig {
  mode: ThemeMode
  primaryColor: string
}

export const useThemeStore = defineStore('theme', () => {
  const config = ref<ThemeConfig>({
    mode: 'dark',
    // 默认 CS 蓝(与 cs-theme 令牌 fallback 一致;改此值全站强调色随之变化)
    primaryColor: '#007acc',
  })

  // 预设主题色(PreferencesDialog 主色选择器使用)
  const presetColors = [
    '#007acc', // CS 经典蓝(默认)
    '#1a5c9a', // CS 深蓝
    '#2196f3', // 亮蓝
    '#4caf50', // 绿
    '#ff9800', // 橙
    '#f44336', // 红
    '#9c27b0', // 紫
    '#607d8b', // 蓝灰
  ]

  // 从 localStorage 加载配置
  const loadConfig = () => {
    const saved = localStorage.getItem('bladeshell-theme-config')
    if (saved) {
      try {
        const parsed = JSON.parse(saved)
        // 仅采纳已知字段(旧版配置的其余键忽略)
        if (parsed.mode) config.value.mode = parsed.mode
        if (parsed.primaryColor) config.value.primaryColor = parsed.primaryColor
      } catch {
        console.warn('Failed to parse theme config from localStorage')
      }
    }
  }

  const saveConfig = () => {
    localStorage.setItem('bladeshell-theme-config', JSON.stringify(config.value))
  }

  // 应用主题: data-theme 属性驱动 cs-theme.css 双套令牌;
  // 主色写入 --theme-primary,由 cs-theme.css 桥接到 --cs-active(全站强调色)
  const applyTheme = () => {
    const root = document.documentElement
    root.setAttribute('data-theme', config.value.mode)
    if (config.value.mode === 'dark') {
      root.classList.add('dark')
    } else {
      root.classList.remove('dark')
    }
    root.style.setProperty('--theme-primary', config.value.primaryColor)
    root.style.setProperty('--el-color-primary', config.value.primaryColor)
  }

  const toggleTheme = () => {
    config.value.mode = config.value.mode === 'light' ? 'dark' : 'light'
  }

  const setPrimaryColor = (color: string) => {
    config.value.primaryColor = color
  }

  const setTheme = (themeName: string) => {
    if (themeName === 'light' || themeName === 'dark') {
      config.value.mode = themeName as ThemeMode
    }
  }

  const isDark = computed(() => config.value.mode === 'dark')

  const resetConfig = () => {
    config.value = { mode: 'dark', primaryColor: '#007acc' }
  }

  // 配置变化自动应用 + 持久化
  watch(config, () => { applyTheme(); saveConfig() }, { deep: true })

  const init = () => {
    loadConfig()
    applyTheme()
  }

  return {
    config,
    presetColors,
    isDark,
    toggleTheme,
    setPrimaryColor,
    setTheme,
    resetConfig,
    init,
  }
})
