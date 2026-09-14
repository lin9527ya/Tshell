<template>
  <el-dialog
    v-model="visible"
    :title="isZh ? '连接到团队服务器' : 'Connect to TeamServer'"
    width="440px"
    :close-on-click-modal="false"
    append-to-body
    class="cs-dialog"
  >
    <div class="cs-dialog-body">
      <div class="cs-conn-banner">
        <div class="banner-title">BladeShell C2</div>
        <div class="banner-sub">{{ isZh ? '多人协同团队服务器网关' : 'Multi-Player TeamServer Gateway' }}</div>
      </div>
      <div class="dialog-field">
        <label class="dialog-label">{{ isZh ? '主机 / IP 地址:' : 'Host / IP:' }}</label>
        <el-input v-model="form.host" placeholder="127.0.0.1" size="small" />
      </div>
      <div class="dialog-field">
        <label class="dialog-label">{{ isZh ? '端口:' : 'Port:' }}</label>
        <el-input v-model="form.port" placeholder="8083" size="small" />
      </div>
      <div class="dialog-field">
        <label class="dialog-label">{{ isZh ? '操作员用户名:' : 'User / Alias:' }}</label>
        <el-input v-model="form.user" placeholder="admin" size="small" />
      </div>
      <div class="dialog-field">
        <label class="dialog-label">{{ isZh ? '密码:' : 'Password:' }}</label>
        <el-input v-model="form.password" type="password" show-password placeholder="••••••••" size="small" />
      </div>
    </div>
    <template #footer>
      <div class="cs-dialog-footer">
        <button class="cs-btn cs-btn-primary" :disabled="loading" @click="handleConnect">
          {{ loading ? (isZh ? '连接中...' : 'Connecting...') : (isZh ? '连接' : 'Connect') }}
        </button>
        <button class="cs-btn" @click="close">{{ isZh ? '取消' : 'Cancel' }}</button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useWorkspaceStore } from '@/stores/workspace'
import { useUserStore } from '@/stores/user'
import { useI18nStore } from '@/stores/i18n'

const workspace = useWorkspaceStore()
const userStore = useUserStore()
const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const visible = computed({
  get: () => workspace.dialogs.connect,
  set: (val) => {
    if (!val) workspace.closeDialog('connect')
  },
})

const loading = ref(false)
const form = reactive({
  host: window.location.hostname || '127.0.0.1',
  port: window.location.port || '8083',
  user: userStore.username || 'admin',
  password: '',
})

function close() {
  workspace.closeDialog('connect')
}

async function handleConnect() {
  // 当前为 Web 控制台直连模式：浏览器直接通过 HTTP 与后端通信，无需单独连接 TeamServer
  ElMessage.info(isZh.value ? '当前为 Web 控制台直连模式，无需单独连接 TeamServer；会话与监听器状态实时来自后端' : 'This is a direct web console — no separate TeamServer connection needed; sessions and listeners come live from the backend')
  close()
}
</script>

<style scoped>
.cs-dialog-body {
  padding: 6px 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.cs-conn-banner {
  background: var(--cs-titlebar-bg);
  border: 1px solid var(--cs-border);
  padding: 10px;
  border-radius: 2px;
  text-align: center;
}
.banner-title {
  font-size: 13px;
  font-weight: bold;
  color: var(--cs-text-bright);
}
.banner-sub {
  font-size: 11px;
  color: var(--cs-text-dim);
  margin-top: 2px;
}
.dialog-field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.dialog-label {
  font-size: 12px;
  color: var(--cs-text-dim);
  font-weight: 500;
}
.cs-dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
