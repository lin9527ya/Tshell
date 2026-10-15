// 主题管理Store
// 【vshell 主题配置】对齐还原版「主题配置」抽屉：主题(明暗) + 导航栏模式 + 顶栏主题 + 菜单主题。
// 色板取自还原包实机抽取（07-frontend harness 主题抽屉 DOM 内联色值），顺序一致。
import { defineStore } from 'pinia'
import { ref, watch, computed } from 'vue'

export type ThemeMode = 'light' | 'dark'
/** 导航栏模式（对齐 vshell 四模式） */
export type NavMode = 'top-menu' | 'sidebar' | 'mix' | 'mix-sidebar'

export interface ThemeConfig {
  mode: ThemeMode
  primaryColor: string
  navMode: NavMode
  /** 顶栏主题（顶栏底色） */
  headerColor: string
  /** 菜单主题（左侧菜单底色；顶部菜单模式下不生效） */
  menuColor: string
}

/** 顶栏主题色板（vshell 实测 11 色） */
export const HEADER_COLORS = [
  '#ffffff', '#151515', '#009688', '#5172dc', '#018ffb', '#409eff',
  '#e74c3c', '#24292e', '#394664', '#001529', '#383f45',
]

/** 菜单主题色板（vshell 实测 11 色） */
export const MENU_COLORS = [
  '#001529', '#212121', '#273352', '#ffffff', '#191b24', '#191a23',
  '#304156', '#001628', '#28333e', '#344058', '#383f45',
]

const HEADER_DEFAULT: Record<ThemeMode, string> = { light: '#ffffff', dark: '#151515' }
const MENU_DEFAULT = '#001529'

/** 按底色亮度选前景色，保证顶栏/侧栏文字可读 */
export function readableInk(bg: string): string {
  const hex = bg.replace('#', '')
  const r = parseInt(hex.slice(0, 2), 16)
  const g = parseInt(hex.slice(2, 4), 16)
  const b = parseInt(hex.slice(4, 6), 16)
  if ([r, g, b].some((v) => Number.isNaN(v))) return 'rgba(0, 0, 0, 0.88)'
  const lum = (0.299 * r + 0.587 * g + 0.114 * b) / 255
  return lum > 0.6 ? 'rgba(0, 0, 0, 0.88)' : 'rgba(255, 255, 255, 0.92)'
}

export const useThemeStore = defineStore('theme', () => {
  const config = ref<ThemeConfig>({
    // 默认浅色（VShell 观感：浅色内容区 + 白顶栏）；暗色可切换
    mode: 'light',
    // 默认 Vben 蓝(#0960bd)；改此值全站强调色随之变化
    primaryColor: '#0960bd',
    navMode: 'top-menu',
    headerColor: HEADER_DEFAULT.light,
    menuColor: MENU_DEFAULT,
  })

  // 预设主题色（主题配置抽屉的强调色选择器使用）
  const presetColors = [
    '#0960bd', // Vben 蓝(vshell 分支默认)
    '#007acc', // CS 经典蓝
    '#1a5c9a', // CS 深蓝
    '#2196f3', // 亮蓝
    '#4caf50', // 绿
    '#ff9800', // 橙
    '#f44336', // 红
    '#9c27b0', // 紫
  ]

  const loadConfig = () => {
    const saved = localStorage.getItem('tshell-theme-config')
    if (saved) {
      try {
        const parsed = JSON.parse(saved)
        // 仅采纳已知字段(旧版配置的其余键忽略)
        if (parsed.mode) config.value.mode = parsed.mode
        if (parsed.primaryColor) config.value.primaryColor = parsed.primaryColor
        // 旧值兼容：top→top-menu / side→sidebar
        const legacy: Record<string, NavMode> = { top: 'top-menu', side: 'sidebar' }
        const nm = legacy[parsed.navMode] ?? parsed.navMode
        if (nm === 'top-menu' || nm === 'sidebar' || nm === 'mix' || nm === 'mix-sidebar') {
          config.value.navMode = nm
        }
        if (typeof parsed.headerColor === 'string') config.value.headerColor = parsed.headerColor
        if (typeof parsed.menuColor === 'string') config.value.menuColor = parsed.menuColor
      } catch {
        console.warn('Failed to parse theme config from localStorage')
      }
    }
  }

  const saveConfig = () => {
    localStorage.setItem('tshell-theme-config', JSON.stringify(config.value))
  }

  // 应用主题：data-theme 驱动 cs-theme.css 双套令牌；主色写 --theme-primary，
  // 顶栏/菜单底色写 --theme-* 由 vshell-theme.css 的 --vs-header-bg/--vs-menu-bg 消费
  const applyTheme = () => {
    const root = document.documentElement
    root.setAttribute('data-theme', config.value.mode)
    root.setAttribute('data-nav-mode', config.value.navMode)
    if (config.value.mode === 'dark') {
      root.classList.add('dark')
    } else {
      root.classList.remove('dark')
    }
    root.style.setProperty('--theme-primary', config.value.primaryColor)
    root.style.setProperty('--el-color-primary', config.value.primaryColor)
    root.style.setProperty('--theme-header-bg', config.value.headerColor)
    root.style.setProperty('--theme-header-fg', readableInk(config.value.headerColor))
    // 深色顶栏上用主色做选中会看不清，改用顶栏前景色
    root.style.setProperty(
      '--theme-header-active',
      readableInk(config.value.headerColor) === 'rgba(255, 255, 255, 0.92)'
        ? 'rgba(255, 255, 255, 0.95)'
        : config.value.primaryColor,
    )
    root.style.setProperty('--theme-menu-bg', config.value.menuColor)
    root.style.setProperty('--theme-menu-fg', readableInk(config.value.menuColor))
  }

  /** 明暗切换时，若顶栏仍是另一模式的默认色则跟着换（避免暗色模式下顶着白条） */
  const syncHeaderWithMode = () => {
    const other = config.value.mode === 'dark' ? HEADER_DEFAULT.light : HEADER_DEFAULT.dark
    if (config.value.headerColor === other) config.value.headerColor = HEADER_DEFAULT[config.value.mode]
  }

  const toggleTheme = () => {
    config.value.mode = config.value.mode === 'light' ? 'dark' : 'light'
    syncHeaderWithMode()
  }

  const setPrimaryColor = (color: string) => {
    config.value.primaryColor = color
  }

  const setTheme = (themeName: string) => {
    if (themeName === 'light' || themeName === 'dark') {
      config.value.mode = themeName as ThemeMode
      syncHeaderWithMode()
    }
  }

  const setNavMode = (mode: NavMode) => {
    config.value.navMode = mode
  }

  const setHeaderColor = (color: string) => {
    config.value.headerColor = color
  }

  const setMenuColor = (color: string) => {
    config.value.menuColor = color
  }

  const isDark = computed(() => config.value.mode === 'dark')

  const resetConfig = () => {
    config.value = {
      mode: 'light',
      primaryColor: '#0960bd',
      navMode: 'top-menu',
      headerColor: HEADER_DEFAULT.light,
      menuColor: MENU_DEFAULT,
    }
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
    setNavMode,
    setHeaderColor,
    setMenuColor,
    resetConfig,
    init,
  }
})
