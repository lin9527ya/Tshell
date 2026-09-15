import { defineStore } from 'pinia'

/**
 * 用户偏好设置(P2-12)。
 * 非敏感 UI 偏好持久化到 localStorage;OPSEC 边界: 不得持久化任何凭据类内容
 * (与 credentials.ts persist:false 的口径一致)。
 * PreferencesDialog 保存写此处;消费方(控制台/事件流)读取生效。
 */
export const usePreferencesStore = defineStore('cs-preferences', {
  state: () => ({
    // 控制台
    consoleFontSize: 13,
    consoleFont: 'Consolas, "Courier New", monospace',
    consoleAutoScroll: true,
    scrollback: 5000,
    // 图
    graphAnimation: true,
    nodeSize: 28,
    showLag: true,
    showBeaconsCount: true,
    missedHeartbeats: 3,
    // 通知
    audioNotify: false, // 客户端上线提示音(P1-7)
  }),

  actions: {
    // 组件直接赋值即可(this.xxx = v);persist 插件自动落 localStorage
  },

  persist: true,
})
