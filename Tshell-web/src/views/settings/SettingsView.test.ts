import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { nextTick } from 'vue'

// 把页签组件替换成空壳（工厂内联：vi.mock 提升到文件顶部，且用 render 避免依赖模板编译器）
vi.mock('@/views/config/ServerConfigView.vue', () => ({ default: { name: 'TabStub', render: () => null } }))
vi.mock('@/views/settings/MFAView.vue', () => ({ default: { name: 'TabStub', render: () => null } }))
vi.mock('@/views/server/ServerStatusView.vue', () => ({ default: { name: 'TabStub', render: () => null } }))
vi.mock('@/views/config/UserManagementView.vue', () => ({ default: { name: 'TabStub', render: () => null } }))
vi.mock('@/views/config/KeyManagementView.vue', () => ({ default: { name: 'TabStub', render: () => null } }))
vi.mock('@/views/security/SecurityCenterView.vue', () => ({ default: { name: 'TabStub', render: () => null } }))
vi.mock('@/views/security/HiddenAccessView.vue', () => ({ default: { name: 'TabStub', render: () => null } }))
vi.mock('@/views/logs/LogsView.vue', () => ({ default: { name: 'TabStub', render: () => null } }))
vi.mock('@/views/connection/ConnectionPoolView.vue', () => ({ default: { name: 'TabStub', render: () => null } }))
vi.mock('@/layout/Dialogs/C2ProfileDialog.vue', () => ({ default: { name: 'TabStub', render: () => null } }))

import SettingsView from './SettingsView.vue'
import { usePermissStore } from '@/stores/userpermiss'

// 与 stores/userpermiss.ts 的 defaultList 一致
const ADMIN = ['1', '2', '3', '4', '5', '6', '7', '8', '9', '10', '11', '12', '13', '14', '15', '16']
const OPERATOR = ['1', '2', '3', '4', '5', '6', '7', '8', '9', '10', '11', '12', '13', '15']
const AUDITOR = ['1', '2', '11', '14']

async function mountSettings(permKeys: string[], initialPath = '/setting/config') {
  const pinia = createPinia()
  setActivePinia(pinia)
  usePermissStore().handleSet(permKeys)

  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/setting/:tab?', name: 'Settings', component: SettingsView }],
  })
  await router.push(initialPath)
  await router.isReady()

  const wrapper = mount(SettingsView, { global: { plugins: [pinia, router] } })
  await flushPromises()
  await nextTick()
  return { wrapper, router }
}

describe('SettingsView 页签按角色过滤', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('admin 可见全部 10 个页签', async () => {
    const { wrapper } = await mountSettings(ADMIN)
    const labels = wrapper.findAll('.st-tab').map((t) => t.text())
    expect(labels).toHaveLength(10)
    expect(labels).toContain('服务配置')
    expect(labels).toContain('用户管理')
    expect(labels).toContain('备份') // 备份接口仅 admin（备份含全部凭据）
    expect(labels).toContain('日志')
    expect(labels).toContain('账号安全')
  })

  // 账号安全是**所有登录用户**都能管自己的 MFA，不是管理员专属：
  // 把它做成 adminOnly 会让 operator/auditor 无法开启第二因子。
  it('所有角色都能看到 账号安全', async () => {
    for (const keys of [ADMIN, OPERATOR, AUDITOR]) {
      const { wrapper } = await mountSettings(keys)
      const labels = wrapper.findAll('.st-tab').map((t) => t.text())
      expect(labels).toContain('账号安全')
    }
  })

  // 后端 config/users/keys/security/hidden 是 adminOnly、audit 是 admin+auditor、
  // status/pool 是 protected —— 页签可见性必须与之一致，否则"点得开但必然 403"
  it('operator 能看到 账号安全 / 服务器状态 / 连接池', async () => {
    const { wrapper } = await mountSettings(OPERATOR)
    const labels = wrapper.findAll('.st-tab').map((t) => t.text())
    expect(labels).toEqual(['账号安全', '服务器状态', '连接池'])
  })

  it('auditor 能看到 账号安全 / 服务器状态 / 日志 / 连接池', async () => {
    const { wrapper } = await mountSettings(AUDITOR)
    const labels = wrapper.findAll('.st-tab').map((t) => t.text())
    expect(labels).toEqual(['账号安全', '服务器状态', '日志', '连接池'])
  })

  it('当前页签不可见时回落到第一个可见页签', async () => {
    // operator 用 config（仅 admin）作为不可见页签；回落目标是它的第一个可见页签
    const { router } = await mountSettings(OPERATOR, '/setting/config')
    expect(router.currentRoute.value.path).toBe('/setting/account')
  })

  it('页签可见时不做跳转', async () => {
    const { router } = await mountSettings(ADMIN, '/setting/keys')
    expect(router.currentRoute.value.path).toBe('/setting/keys')
  })
})
