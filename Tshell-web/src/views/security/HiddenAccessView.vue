<template>
  <div class="cs-hidden-access-view cs-font-ui">
    <!-- CS Toolbar -->
    <div class="ha-toolbar">
      <div class="ha-title-group">
        <span class="ha-title">{{ isZh ? '隐藏访问网关' : 'Stealth Access Gateway' }}</span>
        <span class="ha-status" :class="form.enabled ? 'online' : 'offline'">
          ● {{ form.enabled ? (isZh ? '隐藏伪装已开启' : 'Stealth Cloaking Active') : (isZh ? '直接访问（已禁用）' : 'Direct Access (Disabled)') }}
        </span>
      </div>

      <div class="ha-spacer" />

      <button class="cs-btn cs-btn-primary" :disabled="saving" @click="handleSave">
        {{ saving ? (isZh ? '保存中...' : 'Saving...') : (isZh ? '应用配置' : 'Apply Config') }}
      </button>

      <button class="cs-btn" :disabled="loading" @click="fetchStatus">
        {{ isZh ? '刷新' : 'Refresh' }}
      </button>
    </div>

    <!-- Body -->
    <div class="ha-body">
      <div class="ha-card">
        <div class="ha-card-header">
          <span class="card-title">{{ isZh ? '隐藏访问与蜜罐伪装' : 'Stealth Access & Honey Entry Cloaking' }}</span>
          <span class="card-desc">
            {{ isZh ? '通过动态蜜罐令牌与诱饵反向代理路径，隐藏 Team Server 登录入口和 Beacon C2 监听器端点。' : 'Obfuscates Team Server login and Beacon C2 listener endpoints behind dynamic honey tokens and decoy reverse-proxy paths.' }}
          </span>
        </div>

        <div class="cs-form-container">
          <div class="cs-form-row">
            <label class="cs-form-label">{{ isZh ? '启用隐藏访问伪装' : 'Enable Stealth Access Cloaking' }}:</label>
            <el-switch v-model="form.enabled" size="small" />
          </div>

          <div class="cs-form-row">
            <label class="cs-form-label">{{ isZh ? '访问密钥' : 'Secret Access Key' }}:</label>
            <div class="input-with-btn">
              <el-input
                v-model="form.secret_key"
                :placeholder="isZh ? '加密访问密钥' : 'Cryptographic secret key'"
                size="small"
                show-password
                class="cs-input-mono"
              />
              <button class="cs-btn" @click="handleRegenerateKey">{{ isZh ? '重新生成' : 'Regenerate' }}</button>
            </div>
          </div>

          <div class="cs-form-row">
            <label class="cs-form-label">{{ isZh ? '允许的隐藏入口路径' : 'Allowed Access Paths' }}:</label>
            <el-input
              v-model="pathsText"
              type="textarea"
              :rows="3"
              placeholder="/api/v1/telemetry&#10;/gateway/health&#10;/static/assets/bundle.js"
              class="cs-input-mono"
            />
            <span class="cs-hint">{{ isZh ? '多个隐藏入口路径请使用换行分隔。' : 'Separate multiple stealth entry paths by newlines.' }}</span>
          </div>

          <div class="cs-form-row">
            <label class="cs-form-label">{{ isZh ? '验证参数名' : 'Query Param Tokens' }}:</label>
            <el-input
              v-model="paramsText"
              placeholder="sec_token, _k, auth_sig"
              size="small"
              class="cs-input-mono"
            />
            <span class="cs-hint">{{ isZh ? '网关校验的多个令牌参数，使用逗号分隔。' : 'Comma separated token parameters checked by gateway.' }}</span>
          </div>

          <div class="cs-form-row">
            <label class="cs-form-label">{{ isZh ? '会话有效期（分钟）' : 'Stealth Session TTL in Minutes' }}:</label>
            <el-input-number v-model="form.session_ttl" :min="5" :max="1440" size="small" style="width: 140px" />
          </div>

          <div class="ha-action-row">
            <button class="cs-btn cs-btn-danger" @click="handleClearSessions">
              {{ isZh ? '清空所有活动隐藏会话' : 'Flush All Active Stealth Sessions' }}
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Bottom Action Bar (CS Native Button Row) -->
    <div class="ha-bottom-bar">
      <button class="cs-btn cs-btn-primary" :disabled="saving" @click="handleSave">
        {{ isZh ? '应用配置' : 'Apply Configuration' }}
      </button>
      <button class="cs-btn" @click="handleRegenerateKey">
        {{ isZh ? '重新生成密钥' : 'Regenerate Secret Key' }}
      </button>
      <button class="cs-btn cs-btn-danger" @click="handleClearSessions">
        {{ isZh ? '清空会话' : 'Flush Sessions' }}
      </button>
      <button class="cs-btn" @click="handleHelp">
        {{ isZh ? '帮助' : 'Help' }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import SecurityAPI from '@/api/security'
import { useI18nStore } from '@/stores/i18n'

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const loading = ref(false)
const saving = ref(false)

const form = reactive({
  enabled: true,
  access_paths: ['/api/v1/telemetry', '/gateway/health'],
  param_names: ['sec_token', '_k'],
  secret_key: 'SecBlade2026x9948FaK',
  session_ttl: 60,
})

const pathsText = ref('/api/v1/telemetry\n/gateway/health')
const paramsText = ref('sec_token, _k')

async function fetchStatus() {
  loading.value = true
  try {
    const res = await SecurityAPI.getHiddenAccessStatus()
    if (res.data && res.data.data) {
      const d = res.data.data
      Object.assign(form, d)
      if (d.session_ttl == null) form.session_ttl = 60
      if (Array.isArray(d.access_paths)) {
        pathsText.value = d.access_paths.join('\n')
      }
      if (Array.isArray(d.param_names)) {
        paramsText.value = d.param_names.join(', ')
      }
    }
  } catch {} finally {
    loading.value = false
  }
}

async function handleSave() {
  saving.value = true
  try {
    // 换行拆分路径
    form.access_paths = pathsText.value
      .split('\n')
      .map((s) => s.trim())
      .filter(Boolean)

    // 逗号拆分参数
    form.param_names = paramsText.value
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean)

    if (form.session_ttl == null || isNaN(form.session_ttl) || form.session_ttl <= 0) {
      form.session_ttl = 60
    }

    const res = await SecurityAPI.updateHiddenAccessConfig(form)
    if (res?.data?.success || res?.data?.status === 200) {
      ElMessage.success(isZh.value ? '隐藏访问网关配置已更新' : 'Stealth access gateway configuration updated')
    } else {
      ElMessage.error(res?.data?.message || (isZh.value ? '更新配置失败' : 'Failed to update configuration'))
    }
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '更新配置失败' : 'Failed to update configuration'))
  } finally {
    saving.value = false
  }
}

async function handleRegenerateKey() {
  try {
    const res = await SecurityAPI.regenerateHiddenAccessKey()
    // 后端返回 data:{new_secret_key, access_urls, message}
    if (res.data && res.data.data?.new_secret_key) {
      form.secret_key = res.data.data.new_secret_key
      ElMessage.success(isZh.value ? '密钥已重新生成' : 'Secret key regenerated')
    } else {
      ElMessage.error(res.data?.message || (isZh.value ? '重新生成密钥失败' : 'Failed to regenerate key'))
    }
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '重新生成密钥失败' : 'Failed to regenerate key'))
  }
}

async function handleClearSessions() {
  ElMessageBox.confirm(isZh.value ? '清空所有活动的隐藏访问会话？现有未认证连接将全部失效。' : 'Flush all active stealth sessions? Existing unauthenticated connections will be invalidated.', isZh.value ? '清空会话' : 'Flush Sessions', {
    confirmButtonText: isZh.value ? '清空' : 'Flush',
    cancelButtonText: isZh.value ? '取消' : 'Cancel',
    type: 'warning',
  }).then(async () => {
    try {
      const res = await SecurityAPI.clearHiddenAccessSessions()
      if (res?.data?.success || res?.data?.status === 200) {
        ElMessage.success(isZh.value ? '隐藏访问会话已清空' : 'Stealth sessions cleared')
      } else {
        ElMessage.error(res?.data?.message || (isZh.value ? '清空会话失败' : 'Failed to flush sessions'))
      }
    } catch (e: any) {
      ElMessage.error(e?.message || (isZh.value ? '清空会话失败' : 'Failed to flush sessions'))
    }
  }).catch(() => {})
}

function handleHelp() {
  ElMessage.info('Stealth Access Gateway & Cloaking Manual')
}

onMounted(() => {
  fetchStatus()
})
</script>

<style scoped>
.cs-hidden-access-view {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-bg);
  overflow: hidden;
}

.ha-toolbar {
  height: 32px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
  flex-shrink: 0;
}

.ha-title-group {
  display: flex;
  align-items: center;
  gap: 6px;
}

.ha-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.ha-status {
  font-size: 11px;
  color: var(--cs-text-dim);
}

.ha-status.online { color: var(--cs-online); }
.ha-status.offline { color: var(--cs-offline); }

.ha-spacer { flex: 1; }

.ha-body {
  flex: 1;
  padding: 16px 20px;
  background: var(--cs-bg);
  overflow-y: auto;
}

.ha-card {
  max-width: 680px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-border);
  padding: 16px 20px;
}

.ha-card-header {
  margin-bottom: 16px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.card-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.card-desc {
  font-size: 11px;
  color: var(--cs-text-dim);
  line-height: 1.4;
}

.cs-form-container {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.cs-form-row {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.cs-form-label {
  font-size: 11px;
  color: var(--cs-text);
  font-weight: 500;
}

.input-with-btn {
  display: flex;
  gap: 8px;
}

.cs-input-mono {
  font-family: var(--cs-font-mono);
}

.cs-hint {
  font-size: 10px;
  color: var(--cs-text-dim);
}

.ha-action-row {
  margin-top: 8px;
  padding-top: 12px;
  border-top: 1px solid var(--cs-border);
}

.ha-bottom-bar {
  height: 38px;
  background: var(--cs-panel);
  border-top: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 0 12px;
  flex-shrink: 0;
}
</style>
