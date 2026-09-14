<template>
  <div class="cs-login-page cs-font-ui">
    <!-- 1:1 Cobalt Strike Connect Dialog Window -->
    <div class="cs-connect-window">
      <div class="cs-window-titlebar">
        <span class="cs-titlebar-icon">
          <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2">
            <rect x="2" y="3" width="20" height="14" rx="2" />
            <line x1="8" y1="21" x2="16" y2="21" />
            <line x1="12" y1="17" x2="12" y2="21" />
          </svg>
        </span>
        <span class="cs-titlebar-text">Connect to Team Server</span>
      </div>

      <div class="cs-connect-body">
        <div class="cs-dialog-form">
          <div class="cs-form-row">
            <label class="cs-form-label">Host:</label>
            <div class="cs-form-input">
              <el-input v-model="connectHost" size="small" placeholder="127.0.0.1" class="cs-mono-input" />
            </div>
          </div>

          <div class="cs-form-row">
            <label class="cs-form-label">Port:</label>
            <div class="cs-form-input">
              <el-input v-model="connectPort" size="small" placeholder="8080" class="cs-mono-input" />
            </div>
          </div>

          <div class="cs-form-row">
            <label class="cs-form-label">User:</label>
            <div class="cs-form-input">
              <el-input v-model="param.username" size="small" placeholder="admin / neo" class="cs-mono-input" />
            </div>
          </div>

          <div class="cs-form-row">
            <label class="cs-form-label">Password:</label>
            <div class="cs-form-input">
              <el-input
                v-model="param.password"
                type="password"
                show-password
                size="small"
                placeholder="password"
                class="cs-mono-input"
                @keyup.enter="submitLogin"
              />
            </div>
          </div>
        </div>
      </div>

      <!-- CS Centered Bottom Action Buttons -->
      <div class="cs-connect-footer">
        <button class="cs-btn cs-btn-primary" :disabled="loading" @click="submitLogin">
          {{ loading ? 'Connecting...' : 'Connect' }}
        </button>
        <button class="cs-btn" @click="handleHelp">
          Help
        </button>
      </div>
    </div>

    <div class="cs-login-status">
      <span>BladeShell C2 · Cross-Platform Team Server Client (v4.9.1)</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { usePermissStore } from '@/stores/userpermiss'
import { useUserStore } from '@/stores/user'
import UserApi from '@/api/user'

const router = useRouter()
const permiss = usePermissStore()
const userinfo = useUserStore()

const connectHost = ref('127.0.0.1')
const connectPort = ref('8080')
const loading = ref(false)

const param = reactive({
  username: '',
  password: '',
})

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

function handleHelp() {
  ElMessage.info('Connect Dialog: 指定 Team Server 的 Host、Port、User 和 Password 连接控制端。')
}
</script>

<style scoped>
.cs-login-page {
  width: 100vw;
  height: 100vh;
  background: var(--cs-bg, #2b2d30);
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  user-select: none;
}

.cs-connect-window {
  width: 360px;
  background: var(--cs-panel, #3c3f41);
  border: 1px solid var(--cs-border, #555555);
  box-shadow: 2px 4px 16px rgba(0, 0, 0, 0.6);
  display: flex;
  flex-direction: column;
}

.cs-window-titlebar {
  height: 26px;
  background: var(--cs-titlebar-bg, #2b2d30);
  border-bottom: 1px solid var(--cs-border, #555555);
  display: flex;
  align-items: center;
  padding: 0 8px;
  gap: 6px;
}

.cs-titlebar-icon {
  display: flex;
  align-items: center;
  color: var(--cs-active, #007acc);
}

.cs-titlebar-text {
  font-size: 12px;
  font-weight: 600;
  color: var(--cs-text-bright, #ffffff);
}

.cs-connect-body {
  padding: 16px 20px;
}

.cs-dialog-form {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.cs-form-row {
  display: flex;
  align-items: center;
}

.cs-form-label {
  width: 75px;
  font-size: 12px;
  color: var(--cs-text, #c0c0c0);
  text-align: right;
  padding-right: 12px;
  flex-shrink: 0;
}

.cs-form-input {
  flex: 1;
}

.cs-mono-input :deep(.el-input__wrapper) {
  background: var(--cs-bg, #1e1f22);
  box-shadow: 0 0 0 1px var(--cs-border, #555555) inset;
  border-radius: 2px;
  height: 24px;
}

.cs-mono-input :deep(.el-input__inner) {
  font-family: var(--cs-font-mono, monospace);
  font-size: 12px;
  color: var(--cs-text-bright, #ffffff);
}

.cs-connect-footer {
  height: 38px;
  background: var(--cs-titlebar-bg, #2b2d30);
  border-top: 1px solid var(--cs-border, #555555);
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 0 12px;
}

.cs-login-status {
  margin-top: 16px;
  font-size: 11px;
  color: var(--cs-text-dim, #777777);
  font-family: var(--cs-font-mono, monospace);
}
</style>
