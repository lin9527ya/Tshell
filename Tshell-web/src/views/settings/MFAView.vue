<template>
  <div class="mfa-page">
    <section class="mfa-card">
      <header class="mfa-head">
        <h3>{{ i18nStore.tl('两步验证（TOTP）', 'Two-factor authentication (TOTP)') }}</h3>
        <span class="mfa-badge" :class="enabled ? 'on' : 'off'">
          {{ enabled ? (i18nStore.tl('已开启', 'Enabled')) : i18nStore.tl('未开启', 'Disabled') }}
        </span>
      </header>

      <p class="mfa-desc">
        {{ i18nStore.tl('开启后登录需要「口令 + 认证器动态码」两步。认证器丢失时可用恢复码登录，或用管理员重置。', 'Once enabled, sign-in requires both your password and a code from your authenticator. If you lose the authenticator, use a recovery code or ask an admin to reset.') }}
      </p>

      <p v-if="enabled" class="mfa-meta">
        {{ i18nStore.tl('剩余恢复码', 'Recovery codes left') }}:
        <strong :class="{ warn: recoveryLeft <= 2 }">{{ recoveryLeft }}</strong>
        <span v-if="recoveryLeft <= 2" class="mfa-warn">
          {{ i18nStore.tl('（不多了，建议重新绑定一次以换一批）', '(running low — re-enroll to get a fresh set)') }}
        </span>
      </p>

      <!-- 未开启：口令确认 → 扫码 → 输码确认 -->
      <div v-if="!enabled" class="mfa-body">
        <template v-if="!enroll">
          <el-input
            v-model="password"
            type="password"
            show-password
            :placeholder="i18nStore.tl('当前口令（用于确认身份）', 'Current password')"
          />
          <button class="mfa-btn" :disabled="busy" @click="startEnroll">
            {{ i18nStore.tl('开始绑定', 'Start enrollment') }}
          </button>
        </template>

        <template v-else>
          <ol class="mfa-steps">
            <li>{{ i18nStore.tl('用认证器扫描二维码（或手动输入密钥）', 'Scan the QR code (or enter the key manually)') }}</li>
            <li>{{ i18nStore.tl('抄写下面的恢复码（只显示这一次）', 'Write down the recovery codes (shown once)') }}</li>
            <li>{{ i18nStore.tl('输入认证器里的动态码完成绑定', 'Enter a code from the authenticator to finish') }}</li>
          </ol>

          <div class="mfa-qr">
            <img :src="qrUrl" alt="otpauth QR" />
          </div>
          <div class="mfa-secret">
            <span class="mfa-secret-label">{{ i18nStore.tl('手动密钥', 'Manual key') }}</span>
            <code>{{ enroll.secret }}</code>
            <button class="mfa-copy" @click="copy(enroll.secret)">{{ i18nStore.tl('复制', 'Copy') }}</button>
          </div>

          <div class="mfa-recovery">
            <div class="mfa-recovery-head">
              <strong>{{ i18nStore.tl('恢复码', 'Recovery codes') }}</strong>
              <button class="mfa-copy" @click="copy(enroll.recovery_codes.join('\n'))">
                {{ i18nStore.tl('全部复制', 'Copy all') }}
              </button>
            </div>
            <ul>
              <li v-for="c in enroll.recovery_codes" :key="c"><code>{{ c }}</code></li>
            </ul>
          </div>

          <el-input
            v-model="confirmCode"
            inputmode="numeric"
            :placeholder="i18nStore.tl('认证器中的 6 位动态码', '6-digit code')"
          />
          <div class="mfa-actions">
            <button class="mfa-btn" :disabled="busy" @click="confirmEnroll">
              {{ i18nStore.tl('完成绑定', 'Confirm') }}
            </button>
            <button class="mfa-btn ghost" :disabled="busy" @click="cancelEnroll">
              {{ i18nStore.tl('取消', 'Cancel') }}
            </button>
          </div>
        </template>
      </div>

      <!-- 已开启：关闭（口令 + 动态码/恢复码） -->
      <div v-else class="mfa-body">
        <el-input
          v-model="password"
          type="password"
          show-password
          :placeholder="i18nStore.tl('当前口令', 'Current password')"
        />
        <el-input
          v-model="disableCode"
          :placeholder="i18nStore.tl('动态码或一个恢复码', 'Code or a recovery code')"
        />
        <button class="mfa-btn danger" :disabled="busy" @click="disable">
          {{ i18nStore.tl('关闭两步验证', 'Disable') }}
        </button>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import UserApi from '@/api/user'
import { useI18nStore } from '@/stores/i18n'

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const enabled = ref(false)
const recoveryLeft = ref(0)
const password = ref('')
const disableCode = ref('')
const confirmCode = ref('')
const busy = ref(false)

const enroll = ref<{ secret: string; otpauth_uri: string; recovery_codes: string[] } | null>(null)
const qrUrl = computed(() =>
  enroll.value ? UserApi.mfaQrUrl(enroll.value.otpauth_uri) : ''
)

async function refresh() {
  try {
    const res = await UserApi.mfaStatus()
    if (res.data?.success) {
      enabled.value = !!res.data.data.mfa_enabled
      recoveryLeft.value = res.data.data.recovery_codes_remaining ?? 0
    }
  } catch {
    // 状态拉取失败不阻塞页面：下面的操作各自会再报错
  }
}

onMounted(refresh)

async function startEnroll() {
  if (!password.value) {
    ElMessage.warning(isZh.value ? '请输入当前口令' : 'Enter your password')
    return
  }
  busy.value = true
  try {
    const res = await UserApi.mfaStart({ password: password.value })
    if (res.data?.success) {
      enroll.value = res.data.data
      confirmCode.value = ''
    } else {
      ElMessage.error(res.data?.message || '操作失败')
    }
  } catch (e: unknown) {
    ElMessage.error(errText(e, isZh.value ? '开始绑定失败' : 'Failed'))
  } finally {
    busy.value = false
  }
}

function cancelEnroll() {
  enroll.value = null
  confirmCode.value = ''
  password.value = ''
}

async function confirmEnroll() {
  if (!enroll.value) return
  if (!confirmCode.value.trim()) {
    ElMessage.warning(isZh.value ? '请输入动态码' : 'Enter the code')
    return
  }
  busy.value = true
  try {
    const res = await UserApi.mfaConfirm({
      password: password.value,
      secret: enroll.value.secret,
      totp_code: confirmCode.value.trim(),
      recovery_codes: enroll.value.recovery_codes,
    })
    if (res.data?.success) {
      ElMessage.success(
        isZh.value ? '已开启两步验证，请重新登录' : 'Enabled — please sign in again'
      )
      cancelEnroll()
      await refresh()
    } else {
      ElMessage.error(res.data?.message || '操作失败')
    }
  } catch (e: unknown) {
    ElMessage.error(errText(e, isZh.value ? '动态码不正确' : 'Invalid code'))
  } finally {
    busy.value = false
  }
}

async function disable() {
  if (!password.value) {
    ElMessage.warning(isZh.value ? '请输入当前口令' : 'Enter your password')
    return
  }
  try {
    await ElMessageBox.confirm(
      isZh.value
        ? '关闭后登录只需口令，安全性下降。确认关闭？'
        : 'Disabling removes the second factor. Continue?',
      isZh.value ? '确认关闭' : 'Confirm',
      { type: 'warning' }
    )
  } catch {
    return
  }

  busy.value = true
  try {
    const code = disableCode.value.trim()
    const isRecovery = code.replace(/[-\s]/g, '').length === 10
    const res = await UserApi.mfaDisable(
      isRecovery
        ? { password: password.value, recovery_code: code }
        : { password: password.value, totp_code: code }
    )
    if (res.data?.success) {
      ElMessage.success(isZh.value ? '已关闭两步验证' : 'Disabled')
      password.value = ''
      disableCode.value = ''
      await refresh()
    } else {
      ElMessage.error(res.data?.message || '操作失败')
    }
  } catch (e: unknown) {
    ElMessage.error(errText(e, isZh.value ? '关闭失败' : 'Failed'))
  } finally {
    busy.value = false
  }
}

function errText(e: unknown, fallback: string): string {
  const detail = (e as { response?: { data?: { detail?: string; message?: string } } })?.response?.data
  return detail?.detail || detail?.message || fallback
}

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(isZh.value ? '已复制' : 'Copied')
  } catch {
    ElMessage.warning(isZh.value ? '复制失败，请手动选择' : 'Copy failed')
  }
}
</script>

<style scoped>
.mfa-page {
  padding: 16px;
}

.mfa-card {
  max-width: 560px;
  padding: 18px;
  border: 1px solid var(--cs-border, #e4e7ed);
  border-radius: 8px;
  background: var(--cs-bg-card, #fff);
}

.mfa-head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
}

.mfa-head h3 {
  margin: 0;
  font-size: 15px;
}

.mfa-badge {
  padding: 1px 8px;
  border-radius: 10px;
  font-size: 11px;
}

.mfa-badge.on {
  background: #e7f7ec;
  color: #1a7f37;
}

.mfa-badge.off {
  background: #f2f3f5;
  color: #86909c;
}

.mfa-desc,
.mfa-meta {
  margin: 0 0 12px;
  font-size: 12px;
  line-height: 1.6;
  color: var(--cs-text-dim, #86909c);
}

.mfa-warn {
  color: #d4380d;
}

.mfa-warn,
strong.warn {
  color: #d4380d;
}

.mfa-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.mfa-steps {
  margin: 0;
  padding-left: 18px;
  font-size: 12px;
  line-height: 1.7;
  color: var(--cs-text-dim, #86909c);
}

.mfa-qr {
  display: flex;
  justify-content: center;
  padding: 10px;
  background: #fff;
  border-radius: 6px;
}

.mfa-qr img {
  width: 200px;
  height: 200px;
}

.mfa-secret,
.mfa-recovery {
  padding: 10px;
  background: var(--cs-bg-soft, #f7f8fa);
  border-radius: 6px;
  font-size: 12px;
}

.mfa-secret {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.mfa-secret-label {
  color: var(--cs-text-dim, #86909c);
}

.mfa-secret code {
  word-break: break-all;
}

.mfa-recovery-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 6px;
}

.mfa-recovery ul {
  margin: 0;
  padding-left: 16px;
  columns: 2;
}

.mfa-recovery li {
  line-height: 1.8;
}

.mfa-btn {
  padding: 8px 14px;
  border: none;
  border-radius: 6px;
  background: #0960bd;
  color: #fff;
  font-size: 13px;
  cursor: pointer;
}

.mfa-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.mfa-btn.ghost {
  background: transparent;
  border: 1px solid var(--cs-border, #dcdfe6);
  color: var(--cs-text, #333);
}

.mfa-btn.danger {
  background: #d4380d;
}

.mfa-actions {
  display: flex;
  gap: 8px;
}

.mfa-copy {
  padding: 2px 8px;
  border: 1px solid var(--cs-border, #dcdfe6);
  border-radius: 4px;
  background: transparent;
  font-size: 11px;
  cursor: pointer;
}
</style>
