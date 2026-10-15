<template>
  <div class="vs-login">
    <!-- 左上 logo（vshell: top 12px / 高 30px） -->
    <div class="vs-login-logo">
      <img :src="logoUrl" alt="logo" />
      <span>Tshell</span>
    </div>

    <!-- 右上：明暗切换 + 语言 -->
    <div class="vs-login-actions">
      <button class="vs-login-icon" :title="isZh ? '切换主题' : 'Toggle theme'" @click="toggleTheme">
        <VsIcon :name="isDark ? 'sun' : 'moon'" :size="18" />
      </button>
      <button class="vs-login-icon" :title="isZh ? '切换语言' : 'Language'" @click="i18nStore.toggleLocale()">
        <VsIcon name="language" :size="18" />
      </button>
    </div>

    <div class="vs-login-form">
      <h2 class="vs-login-title">{{ isZh ? '登录' : 'Sign in' }}</h2>

      <el-input
        v-model="param.username"
        size="large"
        class="vs-login-input"
        :placeholder="isZh ? '账号' : 'Username'"
        @keyup.enter="submitLogin"
      />
      <el-input
        v-model="param.password"
        type="password"
        size="large"
        show-password
        class="vs-login-input"
        :placeholder="isZh ? '密码' : 'Password'"
        @keyup.enter="submitLogin"
      />

      <button class="vs-login-submit" :disabled="loading" @click="submitLogin">
        {{ loading ? (isZh ? '登录中…' : 'Signing in…') : isZh ? '登录' : 'Sign in' }}
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

const isZh = computed(() => i18nStore.isZh)
const isDark = computed(() => themeStore.isDark)

const loading = ref(false)

const param = reactive({
  username: '',
  password: '',
})

function toggleTheme() {
  themeStore.setTheme(isDark.value ? 'light' : 'dark')
}

function submitLogin() {
  if (!param.username || !param.password) {
    ElMessage.warning('请输入账号与密码')
    return
  }

  loading.value = true
  UserApi.login(param)
    .then((res) => {
      if (res.data.success === true && res.data.code === 0) {
        ElMessage.success('登录成功')
        const responseData = res.data.data
        const role = responseData.role || (responseData.permissions === 1 ? 'admin' : 'user')
        const menuKey = role === 'admin' ? 'admin' : role === 'auditor' ? 'auditor' : 'user'
        const keys = permiss.defaultList[menuKey]
        permiss.handleSet(keys)
        userinfo.setToken(responseData.access_token)
        userinfo.setPermiss(responseData.permissions)
        userinfo.setRefresh(responseData.refresh_token)
        userinfo.setRoles(role === 'admin' ? '超级管理员' : role === 'auditor' ? '审计员' : '操作员')
        userinfo.setUserName(responseData.username)

        // 解锁音频上下文(登录点击=用户交互,此后提示音可播;P1-7)
        primeAudio()

        router.push('/')
      } else {
        ElMessage.error(res.data.message || '登录失败')
      }
    })
    .catch((error) => {
      if (error.response && error.response.status === 401) {
        ElMessage.error('用户名或密码错误')
      } else if (error.response && error.response.status === 429) {
        ElMessage.error('登录尝试过于频繁，请稍后再试')
      } else {
        ElMessage.error('网络错误，请检查 Team Server 服务端连接')
      }
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
</style>
