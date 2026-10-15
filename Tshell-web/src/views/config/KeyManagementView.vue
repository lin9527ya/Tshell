<template>
  <div class="cs-key-management-view cs-font-ui">
    <!-- 工具条：左侧标题/状态，右侧操作 -->
    <div class="km-toolbar">
      <div class="km-title-group">
        <span class="km-title">{{ isZh ? '密钥管理与通知协同' : 'Key Management & Alerts' }}</span>
        <span class="km-status" :class="notifConfig.enable ? 'online' : 'offline'">
          {{ notifConfig.enable ? (isZh ? '通知已开启' : 'Alerts Enabled') : (isZh ? '通知已禁用' : 'Alerts Disabled') }}
        </span>
      </div>

      <div class="km-spacer" />

      <button class="cs-btn" @click="handleHelp">
        {{ isZh ? '帮助' : 'Help' }}
      </button>

      <button class="cs-btn" :disabled="loading" @click="fetchConfigs">
        {{ isZh ? '刷新' : 'Refresh' }}
      </button>

      <button class="cs-btn" :disabled="testing" @click="handleTestNotification">
        {{ testing ? (isZh ? '发送测试中...' : 'Sending Test...') : (isZh ? '测试通知' : 'Test Notification') }}
      </button>

      <button class="cs-btn cs-btn-primary" :disabled="saving" @click="handleSaveConfig">
        {{ saving ? (isZh ? '保存中...' : 'Saving...') : (isZh ? '保存配置' : 'Save Configuration') }}
      </button>
    </div>

    <!-- Master-Detail Body -->
    <div class="km-body">
      <!-- Left Category Navigation -->
      <div class="km-sidebar">
        <div
          v-for="tab in tabs"
          :key="tab.id"
          class="km-nav-item"
          :class="{ active: currentTab === tab.id }"
          @click="currentTab = tab.id"
        >
          <span class="nav-icon">{{ tab.icon }}</span>
          <span class="nav-label">{{ tab.name }}</span>
          <span v-if="tab.badge" class="nav-badge">{{ tab.badge }}</span>
        </div>
      </div>

      <!-- Right Content Panel -->
      <div class="km-content">
        <!-- 1. 钉钉机器人 (DingTalk) -->
        <div v-if="currentTab === 'dingtalk'" class="km-panel">
          <div class="km-panel-header">
            <span class="km-panel-title">{{ isZh ? '钉钉机器人协同配置' : 'DingTalk Alert Bot Configuration' }}</span>
          </div>
          <div class="km-panel-body">
            <p class="km-panel-desc">
              {{ isZh ? '当新 Beacon 会话上线或执行高危命令时，实时推送通知到指定钉钉群。' : 'Sends real-time push notifications to designated DingTalk group when new Beacon sessions come online or high-risk commands are executed.' }}
            </p>

            <div class="cs-form-container">
              <div class="cs-form-row">
                <label class="cs-form-label">{{ isZh ? '启用钉钉' : 'Enable DingTalk' }}:</label>
                <el-switch v-model="notifConfig.dingtalk.enable" />
              </div>

              <div class="cs-form-row">
                <label class="cs-form-label">{{ isZh ? 'Webhook Token / Access Token' : 'Webhook Token / Access Token' }}:</label>
                <el-input
                  v-model="notifConfig.dingtalk.token"
                  placeholder="e.g. 7c9a6f7b8c..."
                  show-password
                  class="cs-input-mono"
                />
              </div>

              <div class="cs-form-row">
                <label class="cs-form-label">{{ isZh ? '签名密钥（SEC...）' : 'Sign Secret (SEC...)' }}:</label>
                <el-input
                  v-model="notifConfig.dingtalk.secret"
                  placeholder="e.g. SEC1234567..."
                  show-password
                  class="cs-input-mono"
                />
              </div>

              <div class="cs-form-row">
                <label class="cs-form-label">{{ isZh ? '安全关键词' : 'Security Keyword' }}:</label>
                <el-input
                  v-model="notifConfig.dingtalk.keyword"
                  placeholder="e.g. Beacon / BladeShell"
                  style="width: 240px"
                />
              </div>
            </div>
          </div>
        </div>

        <!-- 2. 飞书机器人 (Feishu) -->
        <div v-else-if="currentTab === 'feishu'" class="km-panel">
          <div class="km-panel-header">
            <span class="km-panel-title">{{ isZh ? '飞书机器人协同配置' : 'Feishu / Lark Alert Bot Configuration' }}</span>
          </div>
          <div class="km-panel-body">
            <p class="km-panel-desc">
              {{ isZh ? '接入飞书自定义机器人 Webhook，用于事件推送告警、交互卡片和会话跟踪。' : 'Integrate with Feishu Custom Bot Webhook for event push alerts, interactive cards, and session tracking.' }}
            </p>

            <div class="cs-form-container">
              <div class="cs-form-row">
                <label class="cs-form-label">{{ isZh ? '启用飞书' : 'Enable Feishu' }}:</label>
                <el-switch v-model="notifConfig.feishu.enable" />
              </div>

              <div class="cs-form-row">
                <label class="cs-form-label">{{ isZh ? '自定义机器人 Webhook URL' : 'Custom Bot Webhook URL' }}:</label>
                <el-input
                  v-model="notifConfig.feishu.webhook"
                  placeholder="https://open.feishu.cn/open-apis/bot/v2/hook/..."
                  class="cs-input-mono"
                />
              </div>

              <div class="cs-form-row">
                <label class="cs-form-label">{{ isZh ? '签名校验密钥' : 'Verification Secret' }}:</label>
                <el-input
                  v-model="notifConfig.feishu.secret"
                  :placeholder="isZh ? '可选飞书 Webhook 签名密钥' : 'Optional Feishu webhook signing secret'"
                  show-password
                  class="cs-input-mono"
                />
              </div>

              <div class="cs-form-row">
                <label class="cs-form-label">{{ isZh ? '触发关键词（可选）' : 'Trigger Keyword' }}:</label>
                <el-input
                  v-model="notifConfig.feishu.keyword"
                  placeholder="e.g. BladeShell"
                  style="width: 240px"
                />
              </div>
            </div>
          </div>
        </div>

        <!-- 3. 企业微信 (WeCom) -->
        <div v-else-if="currentTab === 'wecom'" class="km-panel">
          <div class="km-panel-header">
            <span class="km-panel-title">{{ isZh ? '企业微信机器人协同配置' : 'WeCom Bot Configuration' }}</span>
          </div>
          <div class="km-panel-body">
            <p class="km-panel-desc">
              {{ isZh ? '将 Markdown 和文本告警推送到企业微信安全应急响应频道。' : 'Push markdown and text alerts to enterprise WeChat security incident response channels.' }}
            </p>

            <div class="cs-form-container">
              <div class="cs-form-row">
                <label class="cs-form-label">{{ isZh ? '启用企业微信' : 'Enable WeCom' }}:</label>
                <el-switch v-model="notifConfig.wecom.enable" />
              </div>

              <div class="cs-form-row">
                <label class="cs-form-label">{{ isZh ? 'Webhook Key（key=...）' : 'Webhook Key (key=...)' }}:</label>
                <el-input
                  v-model="notifConfig.wecom.wx_key"
                  placeholder="e.g. 693axxx-xxxx-xxxx-xxxx-xxxxxxxx"
                  show-password
                  class="cs-input-mono"
                />
              </div>
            </div>
          </div>
        </div>

        <!-- 4. 监听器加密密钥与加密盐 (Listener Crypto Keys) -->
        <div v-else-if="currentTab === 'listener_keys'" class="km-panel">
          <div class="km-panel-header">
            <span class="km-panel-title">{{ isZh ? '监听器通信密钥（VKey 与加密盐）' : 'Listener VKeys & Cryptographic Salts' }}</span>
          </div>
          <div class="km-panel-body km-panel-body--flush">
            <p class="km-panel-desc km-panel-desc--padded">
              {{ isZh ? '管理每个监听器实例的共享验证密钥（VKey）与 AES/ChaCha20 加密盐。' : 'Manage shared secret verification keys (VKey) and AES/ChaCha20 encryption salts per Listener instance.' }}
            </p>

            <div class="km-table-wrap">
              <el-table
                :data="listenerKeys"
                height="280px"
                highlight-current-row
                class="cs-session-table"
                v-loading="loadingKeys"
              >
                <el-table-column prop="listen_address" :label="isZh ? '监听地址' : 'Listen Address'" width="200" sortable show-overflow-tooltip />
                <el-table-column prop="vkey" :label="isZh ? '验证密钥（VKey）' : 'Verification Key (VKey)'" min-width="220" show-overflow-tooltip>
                  <template #default="{ row }">
                    <span class="cs-font-mono">{{ row.vkey || (isZh ? '默认_VKEY' : 'DEFAULT_VKEY') }}</span>
                  </template>
                </el-table-column>
                <el-table-column prop="encryption_salt" :label="isZh ? '加密盐' : 'Encryption Salt'" width="160" show-overflow-tooltip>
                  <template #default="{ row }">
                    <span class="cs-font-mono">{{ row.encryption_salt || (isZh ? '默认_SALT' : 'DEFAULT_SALT') }}</span>
                  </template>
                </el-table-column>
                <el-table-column :label="isZh ? '操作' : 'Actions'" width="200" align="center">
                  <template #default="{ row }">
                    <button class="cs-btn cs-btn-xs" @click="handleResetKey(row)">{{ isZh ? '重置' : 'Reset' }}</button>
                    <button class="cs-btn cs-btn-xs cs-btn-primary" style="margin-left: 4px" @click="handleEditKey(row)">{{ isZh ? '编辑' : 'Edit' }}</button>
                    <button class="cs-btn cs-btn-xs" style="margin-left: 4px" @click="handleViewKeyDetail(row)">{{ isZh ? '详情' : 'Info' }}</button>
                  </template>
                </el-table-column>
              </el-table>
            </div>
          </div>
        </div>

        <!-- 5. 会话密钥状态 / 轮换 / 健康检查（后端 /key-management/key-status|rotate|health-check|metrics） -->
        <div v-else-if="currentTab === 'session_keys'" class="km-panel">
          <div class="km-panel-header">
            <span class="km-panel-title">{{ isZh ? '会话密钥状态与轮换' : 'Session Key State & Rotation' }}</span>
          </div>
          <div class="km-panel-body">
            <p class="km-panel-desc">
              {{ isZh ? '查看每个 Beacon 会话的当前/上一/下一密钥，手动强制轮换、下发健康检查，并调整自动轮换参数。' : 'Inspect current/previous/next keys per beacon session, force rotation, dispatch health checks, and tune automatic rotation.' }}
            </p>

            <!-- 密钥系统指标卡 -->
            <div class="km-metrics-grid">
              <div class="km-metric-card">
                <div class="km-metric-num">{{ keyMetrics.total_sessions ?? 0 }}</div>
                <div class="km-metric-lbl">{{ isZh ? '密钥会话总数' : 'Total Key Sessions' }}</div>
              </div>
              <div class="km-metric-card">
                <div class="km-metric-num">{{ keyMetrics.active_sessions ?? 0 }}</div>
                <div class="km-metric-lbl">{{ isZh ? '活跃密钥' : 'Active Keys' }}</div>
              </div>
              <div class="km-metric-card">
                <div class="km-metric-num">{{ keyMetrics.expired_sessions ?? 0 }}</div>
                <div class="km-metric-lbl">{{ isZh ? '已过期密钥' : 'Expired Keys' }}</div>
              </div>
              <div class="km-metric-card">
                <div class="km-metric-num">{{ keyMetrics.sessions_with_next ?? 0 }}</div>
                <div class="km-metric-lbl">{{ isZh ? '待切换（Next Key）' : 'Pending Switch (Next)' }}</div>
              </div>
              <div class="km-metric-card">
                <div class="km-metric-num mono">{{ keyMetrics.overlap_duration || '-' }}</div>
                <div class="km-metric-lbl">{{ isZh ? '新旧密钥重叠期' : 'Overlap Duration' }}</div>
              </div>
              <div class="km-metric-card">
                <div class="km-metric-num" :class="keyMetrics.rotation_enabled ? 'text-online' : 'text-danger'">
                  {{ keyMetrics.rotation_enabled ? 'ON' : 'OFF' }}
                </div>
                <div class="km-metric-lbl">{{ isZh ? '自动轮换状态' : 'Auto Rotation State' }}</div>
              </div>
            </div>

            <!-- 轮换参数 -->
            <div class="km-rotation-config">
              <div class="cs-form-row">
                <label class="cs-form-label">{{ isZh ? '自动轮换' : 'Auto Rotation' }}:</label>
                <el-switch v-model="rotationForm.rotation_enabled" />
              </div>
              <div class="cs-form-row">
                <label class="cs-form-label">{{ isZh ? '重叠期（小时）' : 'Overlap (Hours)' }}:</label>
                <el-input-number v-model="rotationForm.overlap_duration_hours" :min="1" :max="168" />
              </div>
              <button class="cs-btn cs-btn-primary" :disabled="savingRotation" @click="handleSaveRotationConfig">
                {{ savingRotation ? (isZh ? '保存中...' : 'Saving...') : (isZh ? '应用轮换配置' : 'Apply Rotation Config') }}
              </button>
            </div>

            <div class="km-table-wrap">
              <el-table
                :data="sessionKeyRows"
                height="240px"
                highlight-current-row
                class="cs-session-table"
                v-loading="loadingSessionKeys"
                @current-change="(r: any) => selectedKeyRow = r"
              >
                <el-table-column prop="client_id" :label="isZh ? '客户端 UID' : 'Client UID'" min-width="160" show-overflow-tooltip>
                  <template #default="{ row }">
                    <span class="cs-font-mono font-bold">{{ row.client_id }}</span>
                  </template>
                </el-table-column>
                <el-table-column prop="exists" :label="isZh ? '状态' : 'State'" width="90" align="center">
                  <template #default="{ row }">
                    <span :class="row.exists ? 'text-online' : 'text-danger'">{{ row.exists ? 'ACTIVE' : 'NONE' }}</span>
                  </template>
                </el-table-column>
                <el-table-column :label="isZh ? '当前密钥' : 'Current Key'" min-width="200" show-overflow-tooltip>
                  <template #default="{ row }">
                    <span v-if="row.current_key" class="cs-font-mono">
                      v{{ row.current_key.version }} / {{ row.current_key.key_id || '-' }}
                    </span>
                    <span v-else class="text-dim">-</span>
                  </template>
                </el-table-column>
                <el-table-column :label="isZh ? '到期时间' : 'Expires At'" width="180" show-overflow-tooltip>
                  <template #default="{ row }">
                    <span class="cs-font-mono">{{ row.current_key?.expires_at || '-' }}</span>
                  </template>
                </el-table-column>
                <el-table-column :label="isZh ? '剩余有效期' : 'Time To Expiry'" width="130" show-overflow-tooltip>
                  <template #default="{ row }">
                    <span class="cs-font-mono">{{ row.current_key?.time_to_expiry || '-' }}</span>
                  </template>
                </el-table-column>
                <el-table-column :label="isZh ? 'Next / Previous' : 'Next / Previous'" width="120" align="center">
                  <template #default="{ row }">
                    <span class="cs-font-mono">
                      {{ row.next_key ? 'N' : '-' }} / {{ row.previous_key ? 'P' : '-' }}
                    </span>
                  </template>
                </el-table-column>
                <el-table-column :label="isZh ? '操作' : 'Actions'" width="170" align="center">
                  <template #default="{ row }">
                    <button class="cs-btn cs-btn-xs" :disabled="!row.exists" @click="handleForceRotate(row)">
                      {{ isZh ? '强制轮换' : 'Rotate' }}
                    </button>
                    <button class="cs-btn cs-btn-xs" style="margin-left: 4px" @click="handleKeyHealthCheck(row)">
                      {{ isZh ? '健康检查' : 'Health Check' }}
                    </button>
                  </template>
                </el-table-column>
              </el-table>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18nStore } from '@/stores/i18n'
import { getNotificationConfig, updateNotificationConfig, testNotification } from '@/api/notification'
import type { NotificationConfig } from '@/api/notification'
import KeyManagementAPI from '@/api/keyManagement'

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const props = defineProps<{
  params?: { tab?: string }
  tab?: string
}>()

const currentTab = ref(props.params?.tab || props.tab || 'dingtalk')

watch(
  () => props.params?.tab || props.tab,
  (newTab) => {
    if (newTab) currentTab.value = newTab
  }
)

const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const loadingKeys = ref(false)
const loadingSessionKeys = ref(false)
const savingRotation = ref(false)

// 会话密钥状态 / 指标 / 轮换配置（后端 /key-management/*）
const keyMetrics = reactive<any>({})
const sessionKeyRows = ref<any[]>([])
const selectedKeyRow = ref<any>(null)
const rotationForm = reactive({
  rotation_enabled: true,
  overlap_duration_hours: 1,
})

const tabs = computed(() => [
  { id: 'dingtalk', name: isZh.value ? '钉钉机器人' : 'DingTalk Bot', icon: '💬', badge: isZh.value ? '启用中' : 'Active' },
  { id: 'feishu', name: isZh.value ? '飞书机器人' : 'Feishu Bot', icon: '🦅', badge: '' },
  { id: 'wecom', name: isZh.value ? '企业微信机器人' : 'WeCom Bot', icon: '🏢', badge: '' },
  { id: 'listener_keys', name: isZh.value ? '监听器密钥' : 'Listener Keys', icon: '🔑', badge: '' },
  { id: 'session_keys', name: isZh.value ? '会话密钥' : 'Session Keys', icon: '🔐', badge: '' },
])

const notifConfig = reactive<NotificationConfig>({
  enable: true,
  enable_audio: true,
  dingtalk: {
    enable: false,
    token: '',
    keyword: 'BladeShell',
    secret: '',
  },
  wecom: {
    enable: false,
    wx_key: '',
  },
  feishu: {
    enable: false,
    webhook: '',
    secret: '',
    keyword: 'BladeShell',
  },
})

const listenerKeys = ref<any[]>([])

async function fetchConfigs() {
  loading.value = true
  try {
    const res = await getNotificationConfig()
    if (res.data && res.data.data) {
      Object.assign(notifConfig, res.data.data)
    }
  } catch {}

  loadingKeys.value = true
  try {
    const res = await KeyManagementAPI.getAllListenerKeyConfigs()
    if (res.data && Array.isArray(res.data.data)) {
      listenerKeys.value = res.data.data
    } else {
      // 不再展示假 VKey 兜底数据
      listenerKeys.value = []
      ElMessage.error(isZh.value ? '获取监听器密钥配置失败' : 'Failed to load listener key configs')
    }
  } catch {
    listenerKeys.value = []
    ElMessage.error(isZh.value ? '获取监听器密钥配置失败' : 'Failed to load listener key configs')
  } finally {
    loading.value = false
    loadingKeys.value = false
  }
}

// ============ 会话密钥状态 / 轮换 / 健康检查 ============
async function fetchSessionKeys() {
  loadingSessionKeys.value = true
  try {
    const [statusRes, metricsRes] = await Promise.allSettled([
      KeyManagementAPI.getAllKeyStatus(),
      KeyManagementAPI.getKeyMetrics(),
    ])
    if (statusRes.status === 'fulfilled' && statusRes.value?.data?.status === 200) {
      const data = statusRes.value.data.data || {}
      sessionKeyRows.value = Object.values(data)
      if (!sessionKeyRows.value.length) {
        // 密钥管理器尚未给任何在线会话派发密钥：状态表为空是真实状态
        console.info('No session key status records yet')
      }
    } else {
      sessionKeyRows.value = []
      ElMessage.error(isZh.value ? '获取会话密钥状态失败' : 'Failed to load session key status')
    }
    if (metricsRes.status === 'fulfilled' && metricsRes.value?.data?.status === 200) {
      Object.assign(keyMetrics, metricsRes.value.data.data || {})
      // 后端只返回 overlap_duration 字符串（如 1h0m0s），解析为小时回填表单
      const hours = parseOverlapHours(keyMetrics.overlap_duration)
      if (hours > 0) rotationForm.overlap_duration_hours = hours
      rotationForm.rotation_enabled = !!keyMetrics.rotation_enabled
    }
  } finally {
    loadingSessionKeys.value = false
  }
}

function parseOverlapHours(duration: any): number {
  if (typeof duration !== 'string') return 0
  const m = /^(\d+)h/.exec(duration)
  return m ? parseInt(m[1], 10) : 0
}

// 使用单客户端端点 /key-management/key-status/:client_id 精确刷新一行
async function refreshKeyRow(clientId: string) {
  try {
    const res = await KeyManagementAPI.getKeyStatus(clientId)
    if (res?.data?.status === 200 && res.data.data) {
      const idx = sessionKeyRows.value.findIndex((r) => r.client_id === clientId)
      if (idx >= 0) {
        sessionKeyRows.value.splice(idx, 1, res.data.data)
        return
      }
    }
  } catch {
    /* 单行刷新失败时回退全量刷新 */
  }
  fetchSessionKeys()
}

async function handleForceRotate(row: any) {
  if (!row?.client_id) return
  ElMessageBox.confirm(
    isZh.value
      ? `强制轮换会话 ${row.client_id} 的密钥？客户端会在下一个可用周期切换至新密钥。`
      : `Force rotate the key for session ${row.client_id}? The beacon will switch to the new key on its next available cycle.`,
    isZh.value ? '强制密钥轮换' : 'Force Key Rotation',
    {
      confirmButtonText: isZh.value ? '轮换' : 'Rotate',
      cancelButtonText: isZh.value ? '取消' : 'Cancel',
      type: 'warning',
    },
  ).then(async () => {
    try {
      const res = await KeyManagementAPI.forceKeyRotation({ client_id: row.client_id })
      if (res?.data?.status === 200) {
        ElMessage.success(isZh.value ? `已启动 ${row.client_id} 的密钥轮换` : `Key rotation started for ${row.client_id}`)
        refreshKeyRow(row.client_id)
      } else {
        ElMessage.error(res?.data?.message || (isZh.value ? '密钥轮换失败' : 'Key rotation failed'))
      }
    } catch (e: any) {
      ElMessage.error(e?.message || (isZh.value ? '密钥轮换失败' : 'Key rotation failed'))
    }
  }).catch(() => {})
}

async function handleKeyHealthCheck(row: any) {
  if (!row?.client_id) return
  try {
    const res = await KeyManagementAPI.sendKeyHealthCheck(row.client_id)
    if (res?.data?.status === 200) {
      ElMessage.success(isZh.value ? `健康检查命令已下发至 ${row.client_id}` : `Health check dispatched to ${row.client_id}`)
      refreshKeyRow(row.client_id)
    } else {
      ElMessage.error(res?.data?.message || (isZh.value ? '健康检查下发失败' : 'Health check dispatch failed'))
    }
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '健康检查下发失败' : 'Health check dispatch failed'))
  }
}

async function handleSaveRotationConfig() {
  if (rotationForm.overlap_duration_hours < 1) {
    ElMessage.warning(isZh.value ? '重叠期至少 1 小时' : 'Overlap duration must be at least 1 hour')
    return
  }
  savingRotation.value = true
  try {
    const res = await KeyManagementAPI.updateRotationConfig({
      overlap_duration_hours: rotationForm.overlap_duration_hours,
      rotation_enabled: rotationForm.rotation_enabled,
    })
    if (res?.data?.status === 200) {
      ElMessage.success(isZh.value ? '密钥轮换配置已应用' : 'Key rotation config applied')
      fetchSessionKeys()
    } else {
      ElMessage.error(res?.data?.message || (isZh.value ? '保存轮换配置失败' : 'Failed to apply rotation config'))
    }
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '保存轮换配置失败' : 'Failed to apply rotation config'))
  } finally {
    savingRotation.value = false
  }
}

async function handleSaveConfig() {
  saving.value = true
  try {
    await updateNotificationConfig(notifConfig)
    ElMessage.success(isZh.value ? '通知与密钥设置已保存' : 'Notification & Key settings saved')
  } catch (e: any) {
    ElMessage.error((isZh.value ? '保存失败：' : 'Save failed: ') + (e?.message || (isZh.value ? '未知错误' : 'Unknown error')))
  } finally {
    saving.value = false
  }
}

async function handleTestNotification() {
  testing.value = true
  try {
    await testNotification()
    ElMessage.success(isZh.value ? '测试告警已发送到配置的 Webhook' : 'Test alert dispatched to configured Webhook')
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '测试通知发送失败' : 'Failed to send test notification'))
  } finally {
    testing.value = false
  }
}

function handleResetKey(row: any) {
  ElMessageBox.confirm(
      isZh.value ? `重置监听器 ${row.listen_address} 的 VKey 与加密盐吗？` : `Reset VKey & Salt for listener ${row.listen_address}?`,
      isZh.value ? '重置密钥' : 'Reset Key',
      {
        confirmButtonText: isZh.value ? '重置' : 'Reset',
        cancelButtonText: isZh.value ? '取消' : 'Cancel',
        type: 'warning',
      },
    ).then(async () => {
    try {
      const res = await KeyManagementAPI.resetListenerKeyConfig(row.listen_address)
      if (res?.data?.status === 200 || res?.data?.success) {
        ElMessage.success(isZh.value ? '密钥重置成功' : 'Key reset successfully')
        fetchConfigs()
      } else {
        ElMessage.error(res?.data?.data || res?.data?.message || (isZh.value ? '密钥重置失败' : 'Key reset failed'))
      }
    } catch (e: any) {
      ElMessage.error(e?.message || (isZh.value ? '密钥重置失败' : 'Key reset failed'))
    }
  }).catch(() => {})
}

function handleEditKey(row: any) {
  ElMessageBox.prompt(
      isZh.value ? '输入新的验证密钥（VKey）：' : 'Enter new Verification Key (VKey):',
      isZh.value ? '编辑 VKey' : 'Edit VKey',
      {
        confirmButtonText: isZh.value ? '保存' : 'Save',
        cancelButtonText: isZh.value ? '取消' : 'Cancel',
        inputValue: row.vkey,
      },
    ).then(async ({ value }) => {
    if (value) {
      row.vkey = value
      try {
        await KeyManagementAPI.updateListenerKeyConfig({ listen_address: row.listen_address, vkey: value })
        ElMessage.success(isZh.value ? 'VKey 已更新' : 'VKey updated')
      } catch {}
    }
  }).catch(() => {})
}

// 查看监听器密钥配置详情（getListenerKeyConfig）
async function handleViewKeyDetail(row: any) {
  try {
    const res = await KeyManagementAPI.getListenerKeyConfig({ address: row.listen_address })
    const data = res?.data?.data || {}
    const lines = [
      isZh.value ? '监听器类型: ' : 'Listener Type: ' + (data.listener_type || '-'),
      isZh.value ? '监听地址: ' : 'Listen Address: ' + (data.listen_address || row.listen_address || '-'),
      isZh.value ? '主密钥长度: ' : 'Master Key Length: ' + (data.master_key_length ?? 0),
      isZh.value ? '加密盐长度: ' : 'Salt Length: ' + (data.salt_length ?? 0),
      (isZh.value ? '自定义配置: ' : 'Custom Config: ') + (data.has_custom_config ? (isZh.value ? '是' : 'Yes') : (isZh.value ? '否' : 'No')),
    ]
    ElMessageBox.alert(lines.join('\n'), isZh.value ? '监听器密钥配置' : 'Listener Key Config', {
      confirmButtonText: isZh.value ? '关闭' : 'Close',
    })
  } catch {
    ElMessage.error(isZh.value ? '获取配置详情失败' : 'Failed to load config detail')
  }
}

function handleHelp() {
  ElMessage.info(isZh.value ? '钉钉、飞书、企业微信、监听器密钥与会话密钥轮换管理说明' : 'DingTalk, Feishu, WeCom, Listener Key & Session Key Rotation Reference')
}

watch(currentTab, (tab) => {
  if (tab === 'session_keys') {
    fetchSessionKeys()
  }
})

onMounted(() => {
  fetchConfigs()
  if (currentTab.value === 'session_keys') {
    fetchSessionKeys()
  }
})
</script>

<style scoped>
.cs-key-management-view {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-bg);
  overflow: hidden;
}

.km-toolbar {
  min-height: 48px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-card-border);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  flex-shrink: 0;
  flex-wrap: wrap;
}

.km-title-group {
  display: flex;
  align-items: center;
  gap: 10px;
}

.km-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

/* 状态 tag：开启实心绿 / 关闭灰 */
.km-status {
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

.km-status.online {
  color: #ffffff;
  background: var(--cs-online);
}

.km-status.offline {
  color: var(--cs-text-dim);
  background: color-mix(in srgb, var(--cs-offline) 18%, transparent);
}

.km-spacer { flex: 1; }

.km-body {
  flex: 1;
  display: flex;
  overflow: hidden;
}

.km-sidebar {
  width: 220px;
  background: var(--cs-panel);
  border-right: 1px solid var(--cs-card-border);
  display: flex;
  flex-direction: column;
  padding: 8px;
  gap: 2px;
}

.km-nav-item {
  height: 36px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
  border-radius: 6px;
  cursor: pointer;
  color: var(--cs-text);
  font-size: 14px;
  transition: all 120ms ease;
  user-select: none;
}

.km-nav-item:hover {
  background: var(--cs-hover);
  color: var(--cs-text-bright);
}

.km-nav-item.active {
  background: color-mix(in srgb, var(--cs-active) 10%, transparent);
  color: var(--cs-active);
  font-weight: 500;
}

.nav-icon {
  font-size: 14px;
}

.nav-label {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.nav-badge {
  margin-left: auto;
  display: inline-flex;
  align-items: center;
  height: 20px;
  padding: 0 8px;
  font-size: 12px;
  background: var(--cs-online);
  color: #ffffff;
  border-radius: 4px;
}

.km-content {
  flex: 1;
  background: var(--cs-bg);
  padding: 16px;
  overflow-y: auto;
}

.km-panel {
  display: flex;
  flex-direction: column;
  background: var(--cs-panel);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  overflow: hidden;
}

.km-panel-header {
  display: flex;
  align-items: center;
  min-height: 38px;
  padding: 0 16px;
  border-bottom: 1px solid var(--cs-card-border);
  flex-shrink: 0;
}

.km-panel-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.km-panel-body {
  padding: 16px;
}

.km-panel-body--flush {
  padding: 0;
}

.km-panel-desc {
  font-size: 13px;
  color: var(--cs-text-dim);
  margin: 0 0 16px;
  line-height: 1.6;
}

.km-panel-desc--padded {
  padding: 16px 16px 0;
  margin-bottom: 12px;
}

.cs-form-container {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-width: 680px;
}

.cs-input-mono {
  font-family: var(--cs-font-mono);
}

/* 会话密钥指标卡 + 轮换配置 */
.km-metrics-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: 12px;
  margin-bottom: 16px;
}

.km-metric-card {
  background: var(--cs-bg);
  border-radius: 8px;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.km-metric-num {
  font-family: var(--cs-font-mono);
  font-size: 22px;
  font-weight: 600;
  color: var(--cs-active);
  word-break: break-all;
}

.km-metric-num.mono {
  font-size: 14px;
}

.km-metric-lbl {
  font-size: 12px;
  color: var(--cs-text-dim);
}

.km-rotation-config {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 16px;
  padding-bottom: 16px;
  margin-bottom: 16px;
  border-bottom: 1px solid var(--cs-card-border);
}

.km-rotation-config .cs-form-row {
  margin-bottom: 0;
}

.text-online {
  color: var(--cs-online);
}

.text-danger {
  color: var(--cs-error);
}

.text-dim {
  color: var(--cs-text-dim);
}

.font-bold {
  font-weight: 600;
}

.km-table-wrap {
  overflow: hidden;
}
</style>
