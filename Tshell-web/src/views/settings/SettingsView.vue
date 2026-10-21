<template>
  <div class="st-page">
    <div class="st-tabs">
      <div
        v-for="t in visibleTabs"
        :key="t.key"
        class="st-tab"
        :class="{ active: t.key === tab }"
        @click="switchTo(t.key)"
      >
        {{ i18nStore.tl(t.zh, t.en) }}
      </div>
    </div>
    <div class="st-body">
      <!-- 服务配置页签：页面级 C2 扩展配置入口（局部弹窗） -->
      <div v-if="tab === 'config'" class="st-subbar">
        <button class="cs-btn" @click="showC2Profile = true">{{ i18nStore.tl('C2 扩展配置', 'C2 Profile') }}</button>
      </div>
      <KeepAlive>
        <component v-if="comp" :is="comp" :key="tab" />
      </KeepAlive>
      <C2ProfileDialog v-model="showC2Profile" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, markRaw, shallowReactive, ref, watch, type Component } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { SETTING_TABS, type SettingTab } from '@/router/routes'
import C2ProfileDialog from '@/layout/Dialogs/C2ProfileDialog.vue'
import { useI18nStore } from '@/stores/i18n'
import { usePermissStore } from '@/stores/userpermiss'
import { Perm } from '@/utils/permiss'

defineOptions({ name: 'Settings' })

const route = useRoute()
const router = useRouter()
const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const tab = computed<SettingTab>(() => {
  const t = String(route.params.tab || 'config') as SettingTab
  return (SETTING_TABS as readonly string[]).includes(t) ? t : 'config'
})

// permiss 必须与后端分组一致，否则会出现"页签点得开、接口必然 403"：
//   config/users/keys/security/hidden → 后端 adminOnly          → '16'(AdminOnly，仅 admin)
//   logs                              → 后端 admin+auditor       → '14'(Audit，admin+auditor)
//   status/pool                       → 后端 protected（全角色）→ '1'(SystemView)
const TABS: { key: SettingTab; zh: string; en: string; permiss: string }[] = [
  { key: 'config', zh: '服务配置', en: 'Config', permiss: Perm.AdminOnly },
  // 账号安全：**所有登录用户**都能管自己的 MFA，不是管理员专属
  { key: 'account', zh: '账号安全', en: 'Account', permiss: Perm.SystemView },
  { key: 'status', zh: '服务器状态', en: 'Status', permiss: Perm.SystemView },
  { key: 'users', zh: '用户管理', en: 'Users', permiss: Perm.AdminOnly },
  { key: 'keys', zh: '密钥管理', en: 'Keys', permiss: Perm.AdminOnly },
  { key: 'security', zh: '安全中心', en: 'Security', permiss: Perm.AdminOnly },
  { key: 'hidden', zh: '隐蔽访问', en: 'Hidden Access', permiss: Perm.AdminOnly },
  { key: 'backup', zh: '备份', en: 'Backup', permiss: Perm.AdminOnly },
  { key: 'logs', zh: '日志', en: 'Logs', permiss: Perm.Audit },
  { key: 'pool', zh: '连接池', en: 'Connections', permiss: Perm.SystemView },
]

const permissStore = usePermissStore()

// 按角色过滤页签（原实现无条件渲染全部 8 个，operator/auditor 点开 6 个必然 403）
const visibleTabs = computed(() => TABS.filter((t) => permissStore.key.includes(t.permiss)))

// 当前页签不可见时落到第一个可见页签（例如 operator 直接访问 /setting/config）
watch(visibleTabs, (tabs) => {
  if (tabs.length > 0 && !tabs.some((t) => t.key === tab.value)) {
    router.replace('/setting/' + tabs[0].key)
  }
}, { immediate: true })

// 同 ClientDetail：先解析再交给 KeepAlive（Vue 3.3 不能缓存 async 包装器）
const LOADERS: Record<SettingTab, () => Promise<{ default: Component }>> = {
  config: () => import('@/views/config/ServerConfigView.vue'),
  account: () => import('@/views/settings/MFAView.vue'),
  status: () => import('@/views/server/ServerStatusView.vue'),
  users: () => import('@/views/config/UserManagementView.vue'),
  keys: () => import('@/views/config/KeyManagementView.vue'),
  security: () => import('@/views/security/SecurityCenterView.vue'),
  hidden: () => import('@/views/security/HiddenAccessView.vue'),
  backup: () => import('@/views/config/BackupView.vue'),
  logs: () => import('@/views/logs/LogsView.vue'),
  pool: () => import('@/views/connection/ConnectionPoolView.vue'),
}
// 同 ClientDetail：按页签缓存已解析组件，未就绪前不渲染
const resolved = shallowReactive<Record<string, Component>>({})
const comp = computed<Component | null>(() => resolved[tab.value] ?? null)
watch(tab, async (t) => {
  if (resolved[t]) return
  const mod = await LOADERS[t]()
  resolved[t] = markRaw(mod.default)
}, { immediate: true })

// C2 扩展配置弹窗（页面内局部状态）
const showC2Profile = ref(false)

function switchTo(t: SettingTab) {
  router.replace(`/setting/${t}`)
}
</script>

<style scoped>
.st-page {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-bg);
  overflow: hidden;
}

.st-tabs {
  height: 40px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 0 16px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-card-border);
  overflow-x: auto;
}

.st-tab {
  height: 40px;
  display: inline-flex;
  align-items: center;
  padding: 0 12px;
  font-size: 14px;
  color: var(--cs-text);
  cursor: pointer;
  white-space: nowrap;
  border-bottom: 2px solid transparent;
  margin-bottom: -1px;
  transition: color 0.15s;
}

.st-tab:hover {
  color: var(--cs-active);
}

.st-tab.active {
  color: var(--cs-active);
  border-bottom-color: var(--cs-active);
}

.st-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.st-body > * {
  flex: 1;
  min-height: 0;
}

/* 页签级工具条：不参与 st-body 的 flex:1 拉伸（与上面规则同权重，靠顺序生效） */
.st-subbar {
  flex: 0 0 auto;
  min-height: 0;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  padding: 8px 16px 0;
}
</style>
