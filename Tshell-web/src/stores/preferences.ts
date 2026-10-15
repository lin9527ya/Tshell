import { defineStore } from 'pinia'

/**
 * 控制台类偏好（原 CS 式偏好设置已于主题抽屉化时收敛，仅保留真正生效的两项）。
 * 非敏感 UI 偏好持久化到 localStorage；OPSEC 边界: 不得持久化任何凭据类内容
 * (与 credentials.ts persist:false 的口径一致)。
 * 主题类设置见 stores/theme.ts（顶栏/菜单/导航模式）。
 */
export const usePreferencesStore = defineStore('cs-preferences', {
  state: () => ({
    /** 终端回滚行数（LinuxInteractiveTerminal / BeaconConsole 读取） */
    scrollback: 5000,
    /** 客户端上线提示音（stores/events.ts 读取） */
    audioNotify: false,
  }),

  actions: {
    // 组件直接赋值即可(this.xxx = v);persist 插件自动落 localStorage
  },

  persist: true,
})
