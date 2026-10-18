<template>
  <div class="vs-login">
    <!-- 左上 logo（vshell: top 12px / 高 30px） -->
    <div class="vs-login-logo">
      <img :src="logoUrl" alt="logo" />
      <span>Tshell</span>
    </div>

    <!-- 右上：明暗切换 + 语言 -->
    <div class="vs-login-actions">
      <button class="vs-login-icon" :title="i18nStore.tl('切换主题', 'Toggle theme')" @click="toggleTheme">
        <VsIcon :name="isDark ? 'sun' : 'moon'" :size="18" />
      </button>
      <button class="vs-login-icon" :title="i18nStore.tl('切换语言', 'Language')" @click="i18nStore.toggleLocale()">
        <VsIcon name="language" :size="18" />
      </button>
    </div>

    <div class="vs-login-form">
      <h2 class="vs-login-title">
        {{ step === 'mfa' ? (i18nStore.tl('两步验证', 'Two-factor')) : i18nStore.tl('登录', 'Sign in') }}
      </h2>

      <!-- 第一步：账号 + 密码 -->
      <template v-if="step === 'password'">
        <el-input
          v-model="param.username"
          size="large"
          class="vs-login-input"
          :placeholder="i18nStore.tl('账号', 'Username')"
          @keyup.enter="submitLogin"
        />
        <el-input
          v-model="param.password"
          type="password"
          size="large"
          show-password
          class="vs-login-input"
          :placeholder="i18nStore.tl('密码', 'Password')"
          @keyup.enter="submitLogin"
        />
      </template>

      <!-- 第二步：动态码（或恢复码） -->
      <template v-else>
        <p class="vs-login-hint">
          {{ i18nStore.tl('请输入认证器中的 6 位动态码；认证器不可用时填一个恢复码。', 'Enter the 6-digit code from your authenticator, or a recovery code.') }}
        </p>
        <el-input
          v-model="param.mfaCode"
          size="large"
          class="vs-login-input"
          inputmode="numeric"
          autocomplete="one-time-code"
          :placeholder="i18nStore.tl('动态码 / 恢复码', 'Code / recovery code')"
          @keyup.enter="submitMfa"
        />
      </template>

      <button class="vs-login-submit" :disabled="loading" @click="step === 'mfa' ? submitMfa() : submitLogin()">
        {{ loading
          ? (i18nStore.tl('处理中…', 'Working…'))
          : step === 'mfa'
            ? (i18nStore.tl('验证', 'Verify'))
            : i18nStore.tl('登录', 'Sign in') }}
      </button>

      <button v-if="step === 'mfa'" class="vs-login-back" @click="backToPassword">
        {{ i18nStore.tl('返回重新输入账号', 'Back') }}
      </button>

      <div class="vs-login-foot">Tshell C2 · Team Server (v4.9.1)</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import VsIcon from '@/layout-vshell/VsIcon.vue'
import logoUrl from '@/assets/logo.ico'
import { usePermissStore } from '@/stores/userpermiss'
import { useUserStore } from '@/stores/user'
import { useI18nStore } from '@/stores/i18n'
import { useThemeStore } from '@/stores/theme'
import UserApi from '@/api/user'
import { primeAudio } from '@/utils/notifyAudio'

const router = useRouter()
const permiss = usePermissStore()
const userinfo = useUserStore()
const i18nStore = useI18nStore()
const themeStore = useThemeStore()

const isDark = computed(() => themeStore.isDark)

const loading = ref(false)

// 两步登录：'password' 收集账号口令，'mfa' 收集动态码/恢复码。
// 中间态令牌只放在内存里（不入 store），它的有效期只有 5 分钟，
// 落盘没有任何收益，只会多一处泄露面。
const step = ref<'password' | 'mfa'>('password')
const pendingToken = ref('')

const param = reactive({
  username: '',
  password: '',
  mfaCode: '',
})

function toggleTheme() {
  themeStore.setTheme(isDark.value ? 'light' : 'dark')
}

// 两步共用的落地逻辑：绝不能写成两份，否则很容易出现
// "第一步做了 A、第二步忘了做 A"（比如漏掉 permitAudio 或角色映射）
function finishLogin(responseData: Record<string, unknown>) {
  const role = (responseData.role as string) || (responseData.permissions === 1 ? 'admin' : 'user')
  const menuKey = role === 'admin' ? 'admin' : role === 'auditor' ? 'auditor' : 'user'
  const keys = permiss.defaultList[menuKey]
  permiss.handleSet(keys)
  userinfo.setToken(responseData.access_token as string)
  // setPermiss 的签名是 string（store 里按字符串保存），服务端回的是数字，
  // 这里显式转换 —— 两步登录共用这段，别再各写一份转换
  userinfo.setPermiss(String(responseData.permissions ?? ''))
  userinfo.setRefresh(responseData.refresh_token as string)
  userinfo.setRoles(role === 'admin' ? '超级管理员' : role === 'auditor' ? '审计员' : '操作员')
  userinfo.setUserName(responseData.username as string)

  // 解锁音频上下文(登录点击=用户交互,此后提示音可播;P1-7)
  primeAudio()

  router.push('/')
}

function backToPassword() {
  step.value = 'password'
  pendingToken.value = ''
  param.mfaCode = ''
}

function describeError(error: unknown, fallback: string): string {
  const resp = (error as { response?: { status?: number } })?.response
  if (resp?.status === 429) return '登录尝试过于频繁，请稍后再试'
  if (resp?.status === 401) return '用户名或密码错误'
  return fallback
}

function submitLogin() {
  if (!param.username || !param.password) {
    ElMessage.warning('请输入账号与密码')
    return
  }

  loading.value = true
  UserApi.login({ username: param.username, password: param.password })
    .then((res) => {
      if (res.data.success === true && res.data.code === 0) {
        const responseData = res.data.data
        // 开启 MFA 的账号：这一步只拿到中间态，进入第二步
        if (responseData.mfa_required === true) {
          pendingToken.value = responseData.pending_token
          step.value = 'mfa'
          param.mfaCode = ''
          ElMessage.info('该账号已开启两步验证，请输入动态码')
          return
        }
        ElMessage.success('登录成功')
        finishLogin(responseData)
      } else {
        ElMessage.error(res.data.message || '登录失败')
      }
    })
    .catch((error) => {
      ElMessage.error(describeError(error, '网络错误，请检查 Team Server 服务端连接'))
    })
    .finally(() => {
      loading.value = false
    })
}

function submitMfa() {
  const code = param.mfaCode.trim()
  if (!code) {
    ElMessage.warning('请输入动态码')
    return
  }
  // 6 位纯数字 → 动态码；其它形状（10 位字母数字，可能带连字符）→ 恢复码。
  // 与服务端 isRecoveryCodeShape 的判定保持一致，避免用户在两步之间迷失。
  const plain = code.replace(/[-\s]/g, '')
  const isRecovery = plain.length === 10

  loading.value = true
  UserApi.loginMfa(
    isRecovery
      ? { pending_token: pendingToken.value, recovery_code: code }
      : { pending_token: pendingToken.value, totp_code: code }
  )
    .then((res) => {
      if (res.data.success === true && res.data.code === 0) {
        ElMessage.success('登录成功')
        finishLogin(res.data.data)
      } else {
        ElMessage.error(res.data.message || '验证失败')
      }
    })
    .catch((error) => {
      // 中间态过期（5 分钟）时必须把用户送回第一步，否则他会一直对着
      // 一个已经失效的 pending token 反复输码
      const detail = (error as { response?: { data?: { detail?: string } } })?.response?.data?.detail || ''
      if (detail.includes('中间态')) {
        ElMessage.error('验证超时，请重新输入账号密码')
        backToPassword()
        return
      }
      ElMessage.error(describeError(error, detail || '动态码错误'))
    })
    .finally(() => {
      loading.value = false
    })
}
</script>

<style scoped>
/* vshell 登录页实测：底 #f3f3f4 / 标题 30px 粗体 / 输入 400×40 圆角 6 / 按钮全宽 #0960bd */
.vs-login {
  width: 100vw;
  height: 100vh;
  background: var(--cs-bg, #f3f3f4);
  display: flex;
  flex-direction: column;
  align-items: center;
  padding-top: 12vh;
  position: relative;
  user-select: none;
}

.vs-login-logo {
  position: absolute;
  top: 12px;
  left: 16px;
  height: 30px;
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 16px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.vs-login-logo img {
  width: 30px;
  height: 30px;
}

.vs-login-actions {
  position: absolute;
  top: 16px;
  right: 16px;
  display: flex;
  align-items: center;
  gap: 6px;
}

.vs-login-icon {
  width: 32px;
  height: 32px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: transparent;
  border: 0;
  border-radius: 6px;
  color: var(--cs-text);
  cursor: pointer;
  transition: color 0.2s, background 0.2s;
}

.vs-login-icon:hover {
  color: var(--cs-active);
  background: rgba(0, 0, 0, 0.04);
}

.vs-login-form {
  width: 404px;
  max-width: calc(100vw - 32px);
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.vs-login-title {
  margin: 0 0 22px;
  text-align: center;
  font-size: 30px;
  font-weight: 700;
  color: var(--cs-text-bright);
}

.vs-login-input {
  width: 100%;
}

.vs-login-input :deep(.el-input__wrapper) {
  height: 40px;
  border-radius: 6px;
  box-shadow: 0 0 0 1px var(--cs-border-strong) inset;
  background: var(--cs-panel);
  padding: 1px 12px;
}

.vs-login-input :deep(.el-input__inner) {
  font-size: 14px;
  height: 38px;
}

.vs-login-submit {
  height: 40px;
  border: 0;
  border-radius: 6px;
  background: var(--cs-active);
  color: #fff;
  font-size: 14px;
  cursor: pointer;
  transition: background 0.2s;
}

.vs-login-submit:hover:not(:disabled) {
  background: color-mix(in srgb, var(--cs-active) 88%, #fff);
}

.vs-login-submit:disabled {
  opacity: 0.7;
  cursor: not-allowed;
}

.vs-login-foot {
  margin-top: -8px;
  text-align: center;
  font-size: 12px;
  color: var(--cs-text-dim);
}

/* 两步验证：提示文案与"返回"链接（复用登录页既有配色变量） */
.vs-login-hint {
  margin: 0 0 10px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--cs-text-dim);
}

.vs-login-back {
  margin-top: 10px;
  width: 100%;
  background: transparent;
  border: none;
  color: var(--cs-text-dim);
  font-size: 12px;
  cursor: pointer;
}

.vs-login-back:hover {
  text-decoration: underline;
}
</style>
