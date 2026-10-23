<template>
  <div class="cs-hidden-access-view cs-font-ui">
    <!-- 工具条：左侧标题/状态，右侧操作按钮 -->
    <div class="ha-toolbar">
      <div class="ha-title-group">
        <span class="ha-title">{{ i18nStore.tl('隐藏访问网关', 'Stealth Access Gateway') }}</span>
        <span class="ha-status" :class="form.enabled ? 'online' : 'offline'">
          {{ form.enabled ? (i18nStore.tl('隐藏伪装已开启', 'Stealth Cloaking Active')) : (i18nStore.tl('直接访问（已禁用）', 'Direct Access (Disabled)')) }}
        </span>
      </div>

      <div class="ha-spacer" />

      <button class="cs-btn" @click="handleHelp">
        {{ i18nStore.tl('帮助', 'Help') }}
      </button>

      <button class="cs-btn" @click="handleRegenerateKey">
        {{ i18nStore.tl('重新生成密钥', 'Regenerate Secret Key') }}
      </button>

      <button class="cs-btn" :disabled="loading" @click="fetchStatus">
        {{ i18nStore.tl('刷新', 'Refresh') }}
      </button>

      <button class="cs-btn cs-btn-primary" :disabled="saving" @click="handleSave">
        {{ saving ? (i18nStore.tl('保存中...', 'Saving...')) : (i18nStore.tl('应用配置', 'Apply Config')) }}
      </button>
    </div>

    <!-- Body -->
    <div class="ha-body" v-loading="loading">
      <div class="ha-card">
        <div class="ha-card-header">
          <span class="card-title">{{ i18nStore.tl('隐藏访问与蜜罐伪装', 'Stealth Access & Honey Entry Cloaking') }}</span>
        </div>

        <div class="ha-card-body">
          <p class="card-desc">
            {{ i18nStore.tl('通过动态蜜罐令牌与诱饵反向代理路径，隐藏 Team Server 登录入口和 Beacon C2 监听器端点。', 'Obfuscates Team Server login and Beacon C2 listener endpoints behind dynamic honey tokens and decoy reverse-proxy paths.') }}
          </p>

          <div class="cs-form-container">
            <div class="cs-form-row">
              <label class="cs-form-label">{{ i18nStore.tl('启用隐藏访问伪装', 'Enable Stealth Access Cloaking') }}:</label>
              <el-switch v-model="form.enabled" />
            </div>

            <div class="cs-form-row">
              <label class="cs-form-label">{{ i18nStore.tl('访问密钥', 'Secret Access Key') }}:</label>
              <div class="input-with-btn">
                <el-input
                  v-model="form.secret_key"
                  :placeholder="i18nStore.tl('加密访问密钥', 'Cryptographic secret key')"
                  show-password
                  class="cs-input-mono"
                />
                <button class="cs-btn" @click="handleRegenerateKey">{{ i18nStore.tl('重新生成', 'Regenerate') }}</button>
              </div>
            </div>

            <div class="cs-form-row">
              <label class="cs-form-label">{{ i18nStore.tl('允许的隐藏入口路径', 'Allowed Access Paths') }}:</label>
              <div class="ha-field">
                <el-input
                  v-model="pathsText"
                  type="textarea"
                  :rows="3"
                  placeholder="/api/v1/telemetry&#10;/gateway/health&#10;/static/assets/bundle.js"
                  class="cs-input-mono"
                />
                <span class="cs-hint">{{ i18nStore.tl('多个隐藏入口路径请使用换行分隔。', 'Separate multiple stealth entry paths by newlines.') }}</span>
              </div>
            </div>

            <div class="cs-form-row">
              <label class="cs-form-label">{{ i18nStore.tl('验证参数名', 'Query Param Tokens') }}:</label>
              <div class="ha-field">
                <el-input
                  v-model="paramsText"
                  placeholder="sec_token, _k, auth_sig"
                  class="cs-input-mono"
                />
                <span class="cs-hint">{{ i18nStore.tl('网关校验的多个令牌参数，使用逗号分隔。', 'Comma separated token parameters checked by gateway.') }}</span>
              </div>
            </div>

            <div class="cs-form-row">
              <label class="cs-form-label">{{ i18nStore.tl('会话有效期（小时）', 'Stealth Session TTL in Hours') }}:</label>
              <el-input-number v-model="form.session_ttl" :min="1" :max="168" style="width: 140px" />
            </div>

            <!-- 现成入口链接：路径 × 参数 × 当前密钥，重新生成密钥后自动刷新 -->
            <div class="cs-form-row">
              <label class="cs-form-label">{{ i18nStore.tl('访问链接', 'Access URLs') }}:</label>
              <div class="ha-field">
                <div v-if="accessUrls.length" class="ha-url-list">
                  <div v-for="u in accessUrls" :key="u" class="ha-url-item">
                    <code class="ha-url-text">{{ u }}</code>
                    <button class="cs-btn cs-btn-xs" @click="copyUrl(u)">{{ i18nStore.tl('复制', 'Copy') }}</button>
                  </div>
                </div>
                <span v-else class="cs-hint">
                  {{ i18nStore.tl('启用并保存后生成；每个路径 × 参数组合一条链接。', 'Generated after enabling and saving: one link per path × param combination.') }}
                </span>
                <span class="cs-hint">
                  {{ i18nStore.tl('链接带当前访问密钥，等同于凭据，勿外发。', 'Links embed the current secret key — treat them as credentials.') }}
                </span>
              </div>
            </div>

            <div class="ha-action-row">
              <span class="ha-action-desc">
                {{ i18nStore.tl('清空后，当前所有未认证的隐藏访问会话将立即失效。', 'Existing unauthenticated stealth sessions are invalidated immediately.') }}
              </span>
              <button class="cs-btn cs-btn-danger" @click="handleClearSessions">
                {{ i18nStore.tl('清空所有活动隐藏会话', 'Flush All Active Stealth Sessions') }}
              </button>
            </div>
          </div>
        </div>
      </div>
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
  enabled: false,
  access_paths: [] as string[],
  param_names: [] as string[],
  secret_key: '',
  session_ttl: 2,
})

const pathsText = ref('')
const paramsText = ref('')
// 现成入口链接（后端 GetConfig 返回 access_urls，路径相对站点根，展示时补 origin）
const accessUrls = ref<string[]>([])

function normalizeUrl(u: string): string {
  if (!u) return ''
  if (/^https?:\/\//i.test(u)) return u
  return `${window.location.origin}${u.startsWith('/') ? '' : '/'}${u}`
}

function applyAccessUrls(list: unknown) {
  accessUrls.value = Array.isArray(list) ? (list as string[]).map(normalizeUrl).filter(Boolean) : []
}

async function copyUrl(u: string) {
  try {
    await navigator.clipboard.writeText(u)
    ElMessage.success(isZh.value ? '链接已复制' : 'Link copied')
  } catch {
    // 非 HTTPS / 剪贴板被拒时退回手动复制提示
    ElMessage.info(u)
  }
}

async function fetchStatus() {
  loading.value = true
  try {
    const res = await SecurityAPI.getHiddenAccessStatus()
    if (res.data && res.data.data) {
      const d = res.data.data
      Object.assign(form, d)
      if (d.session_ttl == null) form.session_ttl = 2
      if (Array.isArray(d.access_paths)) {
        pathsText.value = d.access_paths.join('\n')
      }
      if (Array.isArray(d.param_names)) {
        paramsText.value = d.param_names.join(', ')
      }
      applyAccessUrls(d.access_urls)
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
      form.session_ttl = 2
    }

    const res = await SecurityAPI.updateHiddenAccessConfig(form)
    if (res?.data?.success || res?.data?.status === 200) {
      ElMessage.success(isZh.value ? '隐藏访问网关配置已更新' : 'Stealth access gateway configuration updated')
      // 配置变更（含密钥/路径/参数）后链接会变，重新拉一次状态
      await fetchStatus()
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
      // 密钥变了，旧链接全部失效：直接展示新链接
      applyAccessUrls(res.data.data.access_urls)
      ElMessage.success(isZh.value ? '密钥已重新生成，旧访问链接已失效' : 'Secret key regenerated; previous access URLs are now invalid')
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
  height: 48px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-card-border);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 16px;
  flex-shrink: 0;
}

.ha-title-group {
  display: flex;
  align-items: center;
  gap: 10px;
}

.ha-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

/* 状态 tag：开启实心绿 / 关闭灰 */
.ha-status {
  display: inline-flex;
  align-items: center;
  height: 20px;
  padding: 0 8px;
  border-radius: 4px;
  font-size: 12px;
  line-height: 1;
  white-space: nowrap;
  color: var(--cs-text-dim);
  background: color-mix(in srgb, var(--cs-offline) 18%, transparent);
}

.ha-status.online {
  color: #ffffff;
  background: var(--cs-online);
}

.ha-status.offline {
  color: var(--cs-text-dim);
  background: color-mix(in srgb, var(--cs-offline) 18%, transparent);
}

.ha-spacer { flex: 1; }

.ha-body {
  flex: 1;
  padding: 16px;
  background: var(--cs-bg);
  overflow-y: auto;
}

.ha-card {
  max-width: 760px;
  margin: 0 auto;
  background: var(--cs-panel);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  overflow: hidden;
}

.ha-card-header {
  height: 38px;
  display: flex;
  align-items: center;
  padding: 0 16px;
  border-bottom: 1px solid var(--cs-card-border);
}

.card-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.ha-card-body {
  padding: 16px;
}

.card-desc {
  margin: 0 0 16px;
  font-size: 13px;
  color: var(--cs-text-dim);
  line-height: 1.6;
}

.cs-form-container {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

/* label 右对齐由全局 .cs-form-label / .cs-form-row 提供，这里只补对齐细节 */
.cs-form-row > .cs-form-label {
  align-self: flex-start;
  line-height: 32px;
}

.input-with-btn {
  display: flex;
  flex: 1;
  gap: 8px;
}

.ha-field {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.cs-input-mono {
  font-family: var(--cs-font-mono);
}

.cs-hint {
  font-size: 12px;
  color: var(--cs-text-dim);
  line-height: 1.6;
}

/* 访问链接列表：等宽 + 单行省略 + 复制按钮 */
.ha-url-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.ha-url-item {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  padding: 4px 8px;
  background: var(--cs-bg);
  border: 1px solid var(--cs-card-border);
  border-radius: 6px;
}

.ha-url-text {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: var(--cs-font-mono);
  font-size: 12px;
  color: var(--cs-text);
}

.ha-action-row {
  margin-top: 12px;
  padding-top: 16px;
  border-top: 1px solid var(--cs-card-border);
  display: flex;
  align-items: center;
  gap: 16px;
}

.ha-action-desc {
  flex: 1;
  font-size: 13px;
  color: var(--cs-text-dim);
  line-height: 1.6;
}
</style>
