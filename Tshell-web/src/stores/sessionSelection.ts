import { defineStore } from 'pinia'

/**
 * 会话多选状态（会话表批量操作 / 需要「当前选中会话」的视图共享）。
 *
 * 历史：本 store 原为 CS 式全局工作区状态机（标签页 + 27 键对话框状态 +
 * openDialog/closeDialog 分派 + 分屏/配色等）。路由化（P1）与页面内弹窗（P3）
 * 完成后，只剩会话多选这一点共享状态，故整体收缩并更名。
 */
export const useSessionSelectionStore = defineStore('session-selection', {
  state: () => ({
    selectedUids: [] as string[],
  }),

  getters: {
    primarySelectedUid: (s) => (s.selectedUids.length ? s.selectedUids[0] : ''),
  },

  actions: {
    setSelectedUids(uids: string[]) {
      this.selectedUids = uids
    },
  },
})
