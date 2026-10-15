import { defineStore } from 'pinia'
import type { RouteLocationNormalized } from 'vue-router'
import type { AppRouteMeta } from '@/router/routes'

/**
 * 顶栏标签条 = 路由访问历史（vshell 模型：标签是"最近访问"，不是工作区）。
 * - 打开路由（router.afterEach）→ 追加/激活标签
 * - 关闭标签 → 移除并跳回相邻标签（由 VsTags 处理跳转）
 * - visited 持久化：刷新后标签保留（修掉旧实现"刷新回仪表盘"）
 * - cachedNames：keepAlive 路由的组件名列表，喂给 <keep-alive :include>
 */
export interface VisitedTab {
  name: string
  path: string
  title: string
  icon?: string
  /** 固定标签（仪表盘）：不可关闭 */
  affix?: boolean
  keepAlive?: boolean
}

export const useTabsStore = defineStore('app-tabs', {
  state: () => ({
    visited: [] as VisitedTab[],
    activePath: '',
  }),

  getters: {
    /** 给 <keep-alive :include> 的组件名列表（去重） */
    cachedNames(state): string[] {
      return [...new Set(state.visited.filter((t) => t.keepAlive).map((t) => t.name))]
    },
    /** 固定标签（仪表盘） */
    affixTabs(state): VisitedTab[] {
      return state.visited.filter((t) => t.affix)
    },
  },

  actions: {
    /** 路由进入时调用（router.afterEach） */
    addVisited(route: RouteLocationNormalized) {
      const name = String(route.name || '')
      if (!name) return
      const meta = (route.meta || {}) as unknown as AppRouteMeta
      const title = meta.title || name
      const path = route.fullPath
      const exist = this.visited.find((t) => t.path === path)
      if (exist) {
        exist.title = title
        exist.keepAlive = !!meta.keepAlive
      } else {
        // 同一路由的「同级切换」（客户端功能页签 / 设置页签）→ 更新原标签而非新增
        const parent = path.replace(/\/[^/]*$/, '')
        const sibling = this.visited.find((t) => t.name === name && t.path.replace(/\/[^/]*$/, '') === parent)
        if (sibling) {
          sibling.path = path
          sibling.title = title
          sibling.keepAlive = !!meta.keepAlive
        } else {
          this.visited.push({
            name,
            path,
            title,
            icon: meta.icon,
            affix: name === 'Dashboard',
            keepAlive: !!meta.keepAlive,
          })
        }
      }
      this.activePath = path
    },

    /** 页面加载后可细化标题（如客户端详情补主机名） */
    setTitle(path: string, title: string) {
      const t = this.visited.find((x) => x.path === path)
      if (t) t.title = title
    },

    /** 关闭一个标签；返回关闭后应跳转的路径（由调用方 router.push） */
    close(path: string): string {
      const idx = this.visited.findIndex((t) => t.path === path)
      if (idx < 0) return this.activePath
      const tab = this.visited[idx]
      if (tab.affix) return this.activePath
      this.visited.splice(idx, 1)
      if (this.activePath !== path) return this.activePath
      // 激活标签被关：优先右邻，否则左邻，都没有则回仪表盘
      const next = this.visited[idx] || this.visited[idx - 1]
      return next ? next.path : '/dashboard'
    },

    closeOthers(path: string) {
      this.visited = this.visited.filter((t) => t.path === path || t.affix)
    },

    closeRight(path: string) {
      const idx = this.visited.findIndex((t) => t.path === path)
      if (idx < 0) return
      this.visited = this.visited.filter((t, i) => i <= idx || t.affix)
    },

    closeAll() {
      this.visited = this.visited.filter((t) => t.affix)
      return this.visited.length ? this.visited[0].path : '/dashboard'
    },

    /** 退出登录等场景清空（保留固定标签） */
    reset() {
      this.visited = []
      this.activePath = ''
    },
  },

  persist: {
    paths: ['visited'],
  },
})
