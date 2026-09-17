import { defineStore } from 'pinia'

export interface CsTab {
  id: string
  title: string
  icon?: string
  component: string // panelRegistry registered name
  params?: Record<string, any>
  closable?: boolean
}

export type TopViewMode = 'table' | 'graph' | 'targets'

export interface DialogState {
  preferences: boolean
  connect: boolean
  about: boolean
  scriptManager: boolean
  reports: boolean
  migrate: { visible: boolean; uid: string; pid: string }
  note: { visible: boolean; uid: string; note: string }
  sleep: { visible: boolean; uid: string; currentSleep: number }
  color: { visible: boolean; uid: string }
  binExecute: { visible: boolean; uid: string; mode: string }
  clientReport: { visible: boolean; uid: string }
  // 1:1 CS Payload Dialogs (Screenshots 4-10)
  htaAttack: boolean
  officeMacro: boolean
  payloadGen: boolean
  payloadGenStageless: { visible: boolean; os: string } // os 预设：windows/linux/darwin
  windowsExe: boolean
  windowsExeStageless: boolean
  windowsStagelessAll: boolean
  // 1:1 CS Attack Dialogs (Screenshots 11-15)
  forwardConnect: boolean
  spearPhish: boolean
  c2Profile: boolean
  // 1:1 CS Site Management Dialogs (Pictures 5-7)
  cloneSite: boolean
  hostFile: boolean
  scriptedWebDelivery: boolean
}

export const useWorkspaceStore = defineStore('cs-workspace', {
  state: () => ({
    // 顶部主视图模式：'table' (Session Table) | 'graph' (Pivot Graph) | 'targets' (Target Table)
    topViewMode: 'table' as TopViewMode,

    // 上下分屏比例 (0-100, 顶部占据的百分比)
    splitRatio: 46,
    isTopCollapsed: false,
    isBottomCollapsed: false,

    // 底部多标签页管理
    tabs: [] as CsTab[],
    activeTabId: 'event-log',

    // 会话高亮颜色映射 (uid -> color hex / class)
    sessionColors: {} as Record<string, string>,

    // 选中的客户端 Uid 列表
    selectedUids: [] as string[],

    // CS 对话框状态
    dialogs: {
      preferences: false,
      connect: false,
      about: false,
      scriptManager: false,
      reports: false,
      note: { visible: false, uid: '', note: '' },
      sleep: { visible: false, uid: '', currentSleep: 5 },
      color: { visible: false, uid: '' },
      binExecute: { visible: false, uid: '', mode: 'execute-assembly' },
      clientReport: { visible: false, uid: '' },
      htaAttack: false,
      officeMacro: false,
      payloadGen: false,
      payloadGenStageless: { visible: false, os: 'windows' },
      windowsExe: false,
      windowsExeStageless: false,
      windowsStagelessAll: false,
      forwardConnect: false,
      spearPhish: false,
      c2Profile: false,
      cloneSite: false,
      hostFile: false,
      scriptedWebDelivery: false,
      migrate: { visible: false, uid: '', pid: '' },
    } as DialogState,
  }),

  getters: {
    hasTab: (s) => (id: string) => s.tabs.some((t) => t.id === id),
    tabById: (s) => (id: string) => s.tabs.find((t) => t.id === id),
    activeTab: (s) => s.tabs.find((t) => t.id === s.activeTabId),
    primarySelectedUid: (s) => (s.selectedUids.length ? s.selectedUids[0] : ''),
  },

  actions: {
    setTopViewMode(mode: TopViewMode) {
      this.topViewMode = mode
    },

    setSplitRatio(ratio: number) {
      this.splitRatio = Math.max(15, Math.min(85, ratio))
      this.isTopCollapsed = false
      this.isBottomCollapsed = false
    },

    toggleTopCollapse() {
      this.isTopCollapsed = !this.isTopCollapsed
      if (this.isTopCollapsed) this.isBottomCollapsed = false
    },

    toggleBottomCollapse() {
      this.isBottomCollapsed = !this.isBottomCollapsed
      if (this.isBottomCollapsed) this.isTopCollapsed = false
    },

    resetSplit() {
      this.splitRatio = 46
      this.isTopCollapsed = false
      this.isBottomCollapsed = false
    },

    setSelectedUids(uids: string[]) {
      this.selectedUids = uids
    },

    setSessionColor(uid: string, color: string) {
      if (!color || color === 'default') {
        delete this.sessionColors[uid]
      } else {
        this.sessionColors[uid] = color
      }
    },

    /** 打开或切换至标签页 */
    openTab(tab: CsTab) {
      const existing = this.tabs.find((t) => t.id === tab.id)
      if (existing) {
        if (tab.params) existing.params = tab.params
        if (tab.title) existing.title = tab.title
      } else {
        this.tabs.push({
          closable: true,
          ...tab,
        })
      }
      this.activeTabId = tab.id
      if (this.isBottomCollapsed) {
        this.isBottomCollapsed = false
      }
    },

    /** 兼容旧版 openPanel 命名 */
    openPanel(panel: { id: string; title: string; component: string; params?: Record<string, any>; icon?: string }) {
      this.openTab({
        id: panel.id,
        title: panel.title,
        component: panel.component,
        params: panel.params,
        icon: panel.icon,
      })
    },

    closeTab(id: string) {
      const idx = this.tabs.findIndex((t) => t.id === id)
      if (idx >= 0) {
        this.tabs.splice(idx, 1)
        if (this.activeTabId === id) {
          if (this.tabs.length > 0) {
            const nextIdx = Math.min(idx, this.tabs.length - 1)
            this.activeTabId = this.tabs[nextIdx].id
          } else {
            this.activeTabId = ''
          }
        }
      }
    },

    closeOtherTabs(currentId: string) {
      this.tabs = this.tabs.filter((t) => t.id === currentId)
      this.activeTabId = currentId
    },

    closeAllTabs() {
      this.tabs = []
      this.activeTabId = ''
    },

    setActiveTab(id: string) {
      this.activeTabId = id
    },

    // Dialog 控制
    openDialog(type: keyof DialogState, payload?: any) {
      if (type === 'note' && payload) {
        this.dialogs.note = { visible: true, uid: payload.uid, note: payload.note || '' }
      } else if (type === 'sleep' && payload) {
        this.dialogs.sleep = { visible: true, uid: payload.uid, currentSleep: payload.sleep || 5 }
      } else if (type === 'color' && payload) {
        this.dialogs.color = { visible: true, uid: payload.uid }
      } else if (type === 'binExecute' && payload) {
        this.dialogs.binExecute = { visible: true, uid: payload.uid || '', mode: payload.mode || 'execute-assembly' }
      } else if (type === 'clientReport' && payload) {
        this.dialogs.clientReport = { visible: true, uid: payload.uid || '' }
      } else if (type === 'payloadGenStageless' && payload) {
        this.dialogs.payloadGenStageless = { visible: true, os: payload.os || 'windows' }
      } else if (type === 'payloadGenStageless') {
        this.dialogs.payloadGenStageless.visible = true
      } else if (type === 'migrate' && payload) {
        this.dialogs.migrate = { visible: true, uid: payload.uid || '', pid: payload.pid || '' }
      } else if (type === 'migrate') {
        this.dialogs.migrate.visible = true
      } else if (type in this.dialogs) {
        (this.dialogs as any)[type] = true
      }
    },

    closeDialog(type: keyof DialogState) {
      if (type === 'note') {
        this.dialogs.note.visible = false
      } else if (type === 'sleep') {
        this.dialogs.sleep.visible = false
      } else if (type === 'color') {
        this.dialogs.color.visible = false
      } else if (type === 'binExecute') {
        this.dialogs.binExecute.visible = false
      } else if (type === 'clientReport') {
        this.dialogs.clientReport.visible = false
      } else if (type === 'payloadGenStageless') {
        this.dialogs.payloadGenStageless.visible = false
      } else if (type in this.dialogs) {
        (this.dialogs as any)[type] = false
      }
    },
  },
  persist: {
    paths: ['splitRatio', 'topViewMode', 'sessionColors'],
  },
})
