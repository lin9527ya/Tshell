import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'

// 两步登录的分支验证：加了 MFA 之后登录页从"一条路径"变成"两条"，
// 最容易出错的正是这两条路径的分叉与合并（比如第二步忘了落地角色映射）。
const loginMock = vi.fn()
const loginMfaMock = vi.fn()

vi.mock('@/api/user', () => ({
  default: {
    login: (...args: unknown[]) => loginMock(...args),
    loginMfa: (...args: unknown[]) => loginMfaMock(...args),
    mfaStatus: vi.fn(),
    mfaStart: vi.fn(),
    mfaConfirm: vi.fn(),
    mfaDisable: vi.fn(),
    mfaReset: vi.fn(),
    mfaQrUrl: (t: string) => `/api/users/mfa/qr?text=${encodeURIComponent(t)}`,
  },
}))

const pushMock = vi.fn()
vi.mock('vue-router', () => ({ useRouter: () => ({ push: pushMock }) }))

const handleSetMock = vi.fn()
vi.mock('@/stores/userpermiss', () => ({
  usePermissStore: () => ({
    defaultList: { admin: ['1', '2'], user: ['1'], auditor: ['1'] },
    handleSet: handleSetMock,
  }),
}))

const userSetters = {
  setToken: vi.fn(),
  setPermiss: vi.fn(),
  setRefresh: vi.fn(),
  setRoles: vi.fn(),
  setUserName: vi.fn(),
}
vi.mock('@/stores/user', () => ({ useUserStore: () => userSetters }))
// i18n store 替身：必须覆盖组件实际用到的每个方法。
// 只给 isZh/toggleLocale 时，组件改用 tl() 后会以
// "i18nStore.tl is not a function" 失败 —— 而且只在渲染到那一行时才炸。
// tl 这里按 locale 真的分支，而不是恒返回中文：否则语言相关的回归测不出来。
const makeI18nMock = (locale: 'zh' | 'en') => ({
  isZh: locale === 'zh',
  currentLocale: locale,
  setLocale: vi.fn(),
  toggleLocale: vi.fn(),
  t: (key: string, defaultText?: string) => defaultText || key,
  tl: (zh: string, en: string) => (locale === 'zh' ? zh : en),
})
vi.mock('@/stores/i18n', () => ({ useI18nStore: () => makeI18nMock('zh') }))
vi.mock('@/stores/theme', () => ({ useThemeStore: () => ({ isDark: false, setTheme: vi.fn() }) }))
vi.mock('@/utils/notifyAudio', () => ({ primeAudio: vi.fn() }))
vi.mock('@/assets/logo.ico', () => ({ default: 'logo.ico' }))

vi.mock('element-plus', () => ({
  ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
  ElMessageBox: { confirm: vi.fn() },
}))

import LoginView from '@/views/login/LoginView.vue'

const stubs = {
  VsIcon: true,
  'el-input': {
    props: ['modelValue'],
    emits: ['update:modelValue'],
    template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
  },
}

function mountLogin() {
  return mount(LoginView, { global: { stubs } })
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('LoginView 两步登录', () => {
  it('未开 MFA 的账号：一次提交即落地', async () => {
    loginMock.mockResolvedValue({
      data: {
        success: true,
        code: 0,
        data: {
          access_token: 'at',
          refresh_token: 'rt',
          permissions: 1,
          username: 'alice',
          role: 'admin',
        },
      },
    })

    const wrapper = mountLogin()
    const inputs = wrapper.findAll('input')
    await inputs[0].setValue('alice')
    await inputs[1].setValue('pw')
    await wrapper.find('.vs-login-submit').trigger('click')
    await flushPromises()

    expect(loginMock).toHaveBeenCalledWith({ username: 'alice', password: 'pw' })
    expect(loginMfaMock).not.toHaveBeenCalled()
    expect(userSetters.setToken).toHaveBeenCalledWith('at')
    expect(pushMock).toHaveBeenCalledWith('/')
  })

  it('mfa_required：切到第二步，且不落地任何凭据', async () => {
    loginMock.mockResolvedValue({
      data: { success: true, code: 0, data: { mfa_required: true, pending_token: 'pending-1' } },
    })

    const wrapper = mountLogin()
    const inputs = wrapper.findAll('input')
    await inputs[0].setValue('bob')
    await inputs[1].setValue('pw')
    await wrapper.find('.vs-login-submit').trigger('click')
    await flushPromises()

    // 关键：第一步绝不能落地 token / 跳转
    expect(userSetters.setToken).not.toHaveBeenCalled()
    expect(pushMock).not.toHaveBeenCalled()
    // 页面上应出现动态码输入与"返回"
    expect(wrapper.text()).toContain('两步验证')
    expect(wrapper.find('.vs-login-back').exists()).toBe(true)
  })

  it('第二步成功：用 pending_token 换凭据并落地', async () => {
    loginMock.mockResolvedValue({
      data: { success: true, code: 0, data: { mfa_required: true, pending_token: 'pending-2' } },
    })
    loginMfaMock.mockResolvedValue({
      data: {
        success: true,
        code: 0,
        data: {
          access_token: 'at2',
          refresh_token: 'rt2',
          permissions: 1,
          username: 'bob',
          role: 'admin',
        },
      },
    })

    const wrapper = mountLogin()
    let inputs = wrapper.findAll('input')
    await inputs[0].setValue('bob')
    await inputs[1].setValue('pw')
    await wrapper.find('.vs-login-submit').trigger('click')
    await flushPromises()

    inputs = wrapper.findAll('input')
    // 第一步的两个输入被 v-if 移除了，现在只剩动态码一个
    await inputs[0].setValue('123456')
    await wrapper.find('.vs-login-submit').trigger('click')
    await flushPromises()

    expect(loginMfaMock).toHaveBeenCalledWith({ pending_token: 'pending-2', totp_code: '123456' })
    expect(userSetters.setToken).toHaveBeenCalledWith('at2')
    expect(userSetters.setRefresh).toHaveBeenCalledWith('rt2')
    expect(pushMock).toHaveBeenCalledWith('/')
  })

  it('第二步填 10 位串时按恢复码提交', async () => {
    loginMock.mockResolvedValue({
      data: { success: true, code: 0, data: { mfa_required: true, pending_token: 'pending-3' } },
    })
    loginMfaMock.mockResolvedValue({
      data: { success: true, code: 0, data: { access_token: 'at3', refresh_token: 'rt3', username: 'bob' } },
    })

    const wrapper = mountLogin()
    let inputs = wrapper.findAll('input')
    await inputs[0].setValue('bob')
    await inputs[1].setValue('pw')
    await wrapper.find('.vs-login-submit').trigger('click')
    await flushPromises()

    inputs = wrapper.findAll('input')
    await inputs[0].setValue('ABCDE-FGHJK')
    await wrapper.find('.vs-login-submit').trigger('click')
    await flushPromises()

    expect(loginMfaMock).toHaveBeenCalledWith({
      pending_token: 'pending-3',
      recovery_code: 'ABCDE-FGHJK',
    })
  })

  it('中间态过期：把用户送回第一步（而不是卡在失效的 pending 上）', async () => {
    loginMock.mockResolvedValue({
      data: { success: true, code: 0, data: { mfa_required: true, pending_token: 'pending-4' } },
    })
    loginMfaMock.mockRejectedValue({
      response: { status: 401, data: { detail: '登录中间态已失效，请重新输入口令' } },
    })

    const wrapper = mountLogin()
    let inputs = wrapper.findAll('input')
    await inputs[0].setValue('bob')
    await inputs[1].setValue('pw')
    await wrapper.find('.vs-login-submit').trigger('click')
    await flushPromises()

    inputs = wrapper.findAll('input')
    await inputs[0].setValue('123456')
    await wrapper.find('.vs-login-submit').trigger('click')
    await flushPromises()

    // 回到第一步：账号/密码输入框重新出现，"返回"消失
    expect(wrapper.find('.vs-login-back').exists()).toBe(false)
    expect(wrapper.text()).toContain('登录')
  })
})
