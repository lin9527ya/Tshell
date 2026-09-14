import { defineStore } from 'pinia'
import { ref } from 'vue'

/**
 * UIStore（CS 壳全局 UI 状态）
 * 与 前端重构.md 十六节一致: sidebarWidth / consoleHeight / activeTab / theme
 */
export const useUIStore = defineStore('cs-ui', {
  state: () => ({
    sidebarWidth: 220,     // 左侧导航树宽度(可拖拽调整)
    navigatorVisible: false, // CS 默认不显示资源导航树，避免挤占会话/工作区
    consoleHeight: 180,    // 底部控制台高度(可拖拽调整)
    sidebarCollapsed: false,
    activePanel: '' as string, // 当前激活的 dock 面板 id
    shellEnabled: true,    // 是否使用新 CS 壳(渐进切换开关,默认启用)
  }),
  getters: {
    sidebarWidthPx: (s) => (s.sidebarCollapsed ? 0 : s.sidebarWidth),
  },
  actions: {
    setSidebarWidth(w: number) {
      this.sidebarWidth = Math.max(160, Math.min(480, w))
    },
    setConsoleHeight(h: number) {
      this.consoleHeight = Math.max(100, Math.min(600, h))
    },
    toggleSidebar() {
      this.sidebarCollapsed = !this.sidebarCollapsed
    },
    toggleNavigator() {
      this.navigatorVisible = !this.navigatorVisible
    },
    setActivePanel(id: string) {
      this.activePanel = id
    },
    setShellEnabled(v: boolean) {
      this.shellEnabled = v
    },
  },
  persist: {
    key: 'bladeshell-cs-ui',
    storage: localStorage,
    paths: ['sidebarWidth', 'consoleHeight', 'sidebarCollapsed', 'navigatorVisible', 'shellEnabled'],
  },
})
