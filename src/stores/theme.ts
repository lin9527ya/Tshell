// 主题管理Store
import { defineStore } from 'pinia'
import { ref, watch, computed } from 'vue'

export type ThemeMode = 'light' | 'dark' | 'auto'
export type LayoutMode = 'default' | 'mix' | 'top'
export type SidebarMode = 'fixed' | 'drawer'
export type NavMode = 'icon' | 'text'
export type HeaderThemeMode = 'follow' | 'dark' | 'light'

export interface ThemeConfig {
  mode: ThemeMode
  layout: LayoutMode
  sidebar: SidebarMode
  primaryColor: string
  sidebarCollapsed: boolean
  showBreadcrumb: boolean
  showTabs: boolean
  fixedHeader: boolean
  enableTransition: boolean
  navMode: NavMode
  headerTheme: HeaderThemeMode
  backgroundColor?: string
  textColor?: string
  borderColor?: string
  sidebarWidth?: number
  headerHeight?: number
  borderRadius?: number
  fontSize?: string
  fontFamily?: string
  backgroundImage?: string
  backgroundOpacity?: number
  animationsEnabled?: boolean
  transitionDuration?: number
  customCSS?: string
}

export const useThemeStore = defineStore('theme', () => {
  // 主题配置
  const config = ref<ThemeConfig>({
    mode: 'dark',
    layout: 'default',
    sidebar: 'fixed',
    primaryColor: '#1a5c9a',
    sidebarCollapsed: false,
    showBreadcrumb: true,
    showTabs: true,
    fixedHeader: true,
    enableTransition: true,
    navMode: 'icon',
    headerTheme: 'follow'
  })

  // 预设主题色
  const presetColors = [
    '#1976d2', // 蓝色
    '#2196f3', // 亮蓝色
    '#4caf50', // 绿色
    '#ff9800', // 橙色
    '#f44336', // 红色
    '#9c27b0', // 紫色
    '#607d8b', // 蓝灰色
    '#795548', // 棕色
  ]

  // 从localStorage加载配置
  const loadConfig = () => {
    const saved = localStorage.getItem('bladeshell-theme-config')
    if (saved) {
      try {
        const parsedConfig = JSON.parse(saved)
        config.value = { ...config.value, ...parsedConfig }
      } catch (error) {
        console.warn('Failed to parse theme config from localStorage')
      }
    }
  }

  // 保存配置到localStorage
  const saveConfig = () => {
    localStorage.setItem('bladeshell-theme-config', JSON.stringify(config.value))
  }

  // 应用主题
  const applyTheme = () => {
    const root = document.documentElement
    
    // 设置主题模式
    root.setAttribute('data-theme', config.value.mode)
    if (config.value.mode === 'dark') {
      root.classList.add('dark')
    } else {
      root.classList.remove('dark')
    }
    
    // 设置主题色及 Element Plus 主题色
    root.style.setProperty('--theme-primary', config.value.primaryColor)
    root.style.setProperty('--el-color-primary', config.value.primaryColor)
    
    // 设置布局类
    root.className = root.className.replace(/layout-\w+/g, '')
    root.classList.add(`layout-${config.value.layout}`)
    
    // 设置侧边栏类
    root.className = root.className.replace(/sidebar-\w+/g, '')
    root.classList.add(`sidebar-${config.value.sidebar}`)

    // 导航显示模式类（icon/text）
    root.className = root.className.replace(/nav-\w+/g, '')
    root.classList.add(`nav-${config.value.navMode}`)

    // 顶栏主题属性（follow 时不设属性，回退跟随主主题）
    if (config.value.headerTheme === 'follow') {
      root.removeAttribute('data-header-theme')
    } else {
      root.setAttribute('data-header-theme', config.value.headerTheme)
    }
    
    // 设置其他类
    if (config.value.sidebarCollapsed) {
      root.classList.add('sidebar-collapsed')
    } else {
      root.classList.remove('sidebar-collapsed')
    }
    
    if (config.value.fixedHeader) {
      root.classList.add('header-fixed')
    } else {
      root.classList.remove('header-fixed')
    }
  }

  // 切换主题模式
  const toggleTheme = () => {
    config.value.mode = config.value.mode === 'light' ? 'dark' : 'light'
  }

  // 切换侧边栏折叠状态
  const toggleSidebar = () => {
    config.value.sidebarCollapsed = !config.value.sidebarCollapsed
  }

  // 设置主题色
  const setPrimaryColor = (color: string) => {
    config.value.primaryColor = color
  }

  // 设置布局模式
  const setLayout = (layout: LayoutMode) => {
    config.value.layout = layout
  }

  // 切换导航显示模式（icon/text）
  const toggleNavMode = () => {
    config.value.navMode = config.value.navMode === 'icon' ? 'text' : 'icon'
  }

  // 设置导航显示模式
  const setNavMode = (mode: NavMode) => {
    config.value.navMode = mode
  }

  // 设置顶栏主题
  const setHeaderTheme = (mode: HeaderThemeMode) => {
    config.value.headerTheme = mode
  }

  // 设置侧边栏模式
  const setSidebarMode = (mode: SidebarMode) => {
    config.value.sidebar = mode
  }

  // 获取当前主题
  const currentTheme = computed(() => config.value.mode)
  
  // 获取可用主题列表
  const getAvailableThemes = computed(() => {
    return {
      light: {
        name: 'light',
        displayName: '浅色主题',
        backgroundColor: '#ffffff',
        primaryColor: config.value.primaryColor,
        textColor: '#333333',
        borderColor: '#e4e7ed'
      },
      dark: {
        name: 'dark', 
        displayName: '深色主题',
        backgroundColor: '#1a1a1a',
        primaryColor: config.value.primaryColor,
        textColor: '#ffffff',
        borderColor: '#4c4d4f'
      }
    }
  })
  
  // 获取当前主题配置
  const getCurrentTheme = computed(() => config.value)
  
  // 更新配置
  const updateConfig = (newConfig: Partial<ThemeConfig>) => {
    Object.assign(config.value, newConfig)
  }
  
  // 设置主题
  const setTheme = (themeName: string) => {
    if (themeName === 'light' || themeName === 'dark') {
      config.value.mode = themeName as ThemeMode
    }
  }
  
  // 检查是否为深色主题
  const isDark = computed(() => config.value.mode === 'dark')
  
  // 保存自定义主题
  const saveCustomTheme = (name: string, themeConfig: ThemeConfig) => {
    const customThemes = JSON.parse(localStorage.getItem('custom-themes') || '{}')
    customThemes[name] = themeConfig
    localStorage.setItem('custom-themes', JSON.stringify(customThemes))
  }
  
  // 导出主题配置
  const exportTheme = () => {
    return JSON.stringify(config.value, null, 2)
  }
  
  // 导入主题配置
  const importTheme = (configString: string): boolean => {
    try {
      const importedConfig = JSON.parse(configString)
      Object.assign(config.value, importedConfig)
      return true
    } catch {
      return false
    }
  }
  
  // 重置主题配置
  const resetConfig = () => {
    config.value = {
      mode: 'light',
      layout: 'default',
      sidebar: 'fixed',
      primaryColor: '#1677ff',
      sidebarCollapsed: false,
      showBreadcrumb: true,
      showTabs: true,
      fixedHeader: true,
      enableTransition: true,
      navMode: 'icon',
      headerTheme: 'follow'
    }
  }

  // 监听配置变化，自动应用主题和保存配置
  watch(
    config,
    () => {
      applyTheme()
      saveConfig()
    },
    { deep: true }
  )

  // 初始化
  const init = () => {
    loadConfig()
    applyTheme()
  }

  return {
    config,
    presetColors,
    currentTheme,
    getAvailableThemes,
    getCurrentTheme,
    isDark,
    toggleTheme,
    toggleSidebar,
    setPrimaryColor,
    setLayout,
    setSidebarMode,
    toggleNavMode,
    setNavMode,
    setHeaderTheme,
    updateConfig,
    setTheme,
    saveCustomTheme,
    exportTheme,
    importTheme,
    resetConfig,
    init
  }
})
