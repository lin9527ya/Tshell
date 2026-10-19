<template>
  <el-dialog
    :model-value="visible"
    :title="i18nStore.tl('生成重定向器配置', 'Generate Redirector Config')"
    width="880px"
    :close-on-click-modal="false"
    @update:model-value="(v: boolean) => emit('update:visible', v)"
  >
    <div class="rd-body cs-font-ui">
      <div class="rd-hint">
        {{
          i18nStore.tl(
            '按当前 C2 画像生成配置：画像里的转发路径会原样写进配置。手写重定向器最常见的问题就是与画像漂移 —— 路径对不上时 beacon 会直接失联，而现象看起来像网络问题。',
            'Generated from the current C2 profile: forwarding paths are taken verbatim. The usual failure with hand-written redirectors is drift from the profile — beacons then go silent and it looks like a network problem.',
          )
        }}
      </div>

      <div class="rd-grid">
        <label class="rd-field">
          <span class="rd-label">{{ i18nStore.tl('类型', 'Type') }}</span>
          <el-select v-model="form.type" class="rd-control">
            <el-option label="nginx" value="nginx" />
            <el-option label="apache" value="apache" />
          </el-select>
        </label>

        <label class="rd-field">
          <span class="rd-label">{{ i18nStore.tl('画像', 'Profile') }}</span>
          <el-input v-model="form.profile" class="rd-control" placeholder="default" />
        </label>

        <label class="rd-field">
          <span class="rd-label">{{ i18nStore.tl('后端地址', 'Backend') }}</span>
          <el-input v-model="form.backend" class="rd-control" placeholder="10.0.0.5:8089" />
          <span class="rd-tip">{{ i18nStore.tl('host:port，不要带 http://', 'host:port, no http:// prefix') }}</span>
        </label>

        <label class="rd-field">
          <span class="rd-label">{{ i18nStore.tl('对外域名', 'Domain') }}</span>
          <el-input v-model="form.domain" class="rd-control" placeholder="cdn.example.com" />
        </label>

        <label class="rd-field rd-span2">
          <span class="rd-label">{{ i18nStore.tl('诱饵地址', 'Decoy URL') }}</span>
          <el-input v-model="form.decoy" class="rd-control" placeholder="https://www.example.com" />
          <span class="rd-tip">{{ i18nStore.tl('未命中转发路径时跳到这里（http/https 完整 URL）', 'Visitors not matching a forwarding path land here (full http/https URL)') }}</span>
        </label>

        <label class="rd-field rd-span2">
          <span class="rd-label">
            <el-checkbox v-model="form.full" :disabled="form.type !== 'nginx'">
              {{ i18nStore.tl('生成完整 nginx.conf（自包含 TLS）', 'Full nginx.conf (self-contained TLS)') }}
            </el-checkbox>
          </span>
          <span class="rd-tip">
            {{ i18nStore.tl('只有 nginx 支持完整配置；选它必须填下面两个绝对路径', 'Only nginx supports a full config; it requires the two absolute paths below') }}
          </span>
        </label>

        <template v-if="form.full && form.type === 'nginx'">
          <label class="rd-field">
            <span class="rd-label">{{ i18nStore.tl('证书路径', 'Cert path') }}</span>
            <el-input v-model="form.cert" class="rd-control" placeholder="/etc/ssl/certs/cdn.pem" />
          </label>
          <label class="rd-field">
            <span class="rd-label">{{ i18nStore.tl('私钥路径', 'Key path') }}</span>
            <el-input v-model="form.key" class="rd-control" placeholder="/etc/ssl/private/cdn.key" />
          </label>
        </template>
      </div>

      <div class="rd-actions">
        <button class="cs-btn cs-btn-primary" :disabled="loading || !canGenerate" @click="generate">
          {{ loading ? i18nStore.tl('生成中…', 'Generating…') : i18nStore.tl('生成', 'Generate') }}
        </button>
        <template v-if="result">
          <button class="cs-btn" @click="copyContent">{{ i18nStore.tl('复制', 'Copy') }}</button>
          <button class="cs-btn" @click="downloadContent">{{ i18nStore.tl('下载', 'Download') }}</button>
        </template>
      </div>

      <div v-if="error" class="rd-error">{{ error }}</div>

      <div v-if="result" class="rd-result">
        <div class="rd-result-head">
          <span class="rd-file mono">{{ result.filename }}</span>
          <span class="rd-meta">{{ result.lines }} {{ i18nStore.tl('行', 'lines') }}</span>
          <span class="rd-meta">
            {{ i18nStore.tl('转发路径', 'Forwarded paths') }}: {{ result.uris.length }}
          </span>
        </div>
        <div v-if="result.uris.length" class="rd-uris">
          <code v-for="u in result.uris" :key="u" class="rd-uri mono">{{ u }}</code>
        </div>
        <textarea class="rd-output mono" readonly :value="result.content" />
      </div>
    </div>

    <template #footer>
      <div class="rd-footer">
        <el-button @click="emit('update:visible', false)">{{ i18nStore.tl('关闭', 'Close') }}</el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
// 重定向器配置生成对话框（REMAINING_WORK §5：redirector 此前只有 CLI 与裸接口，
// 没有界面入口）。
//
// 为什么值得做：手写重定向器与 C2 画像漂移是这类部署最常见的失联原因 ——
// 画像里的 http-get/http-post/ws 路径改了，重定向器没跟上，beacon 就静默掉线，
// 而现场看到的现象是"网络不通"。让运维从界面按**当前画像**生成，
// 配置文件里的路径就不可能与人设不一致。
//
// 后端不校验的地方前端也不假装校验：backend/domain 的合法性由服务端
// （redirector.Params.validate）判定，这里只做"必填项没填就别发请求"，
// 两头各写一套规则只会让错误提示互相矛盾。
import { computed, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { getRedirectorTemplate, type RedirectorResult, type RedirectorType } from '@/api/redirector'
import { useI18nStore } from '@/stores/i18n'

defineProps<{ visible: boolean }>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void }>()

const i18nStore = useI18nStore()

const form = reactive({
  type: 'nginx' as RedirectorType,
  profile: 'default',
  backend: '',
  domain: '',
  decoy: '',
  cert: '',
  key: '',
  full: false,
})

const loading = ref(false)
const error = ref('')
const result = ref<RedirectorResult | null>(null)

/** 只挡"必然失败"的情况：后端地址没填。其余交给服务端判定并回显原因 */
const canGenerate = computed(() => form.backend.trim() !== '')

async function generate() {
  loading.value = true
  error.value = ''
  try {
    const res = await getRedirectorTemplate({
      type: form.type,
      profile: form.profile.trim() || 'default',
      backend: form.backend.trim(),
      domain: form.domain.trim(),
      decoy: form.decoy.trim(),
      cert: form.cert.trim(),
      key: form.key.trim(),
      full: form.type === 'nginx' && form.full,
    })
    const data = res?.data?.data
    if (!data || typeof data.content !== 'string') {
      // 形状不符时明确报错，而不是把 undefined 塞进预览框（会显示 "undefined"）
      error.value = i18nStore.tl('服务端返回的内容不符合预期', 'Unexpected response from server')
      result.value = null
      return
    }
    result.value = {
      type: data.type,
      profile: data.profile,
      uris: Array.isArray(data.uris) ? data.uris : [],
      content: data.content,
      lines: typeof data.lines === 'number' ? data.lines : 0,
      full: !!data.full,
      filename: data.filename || `redirector-${form.type}.conf`,
    }
  } catch (e: unknown) {
    // 请求层已按后端 message 弹过提示；这里保留页面内的原因，便于对照参数修改
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    error.value = msg || i18nStore.tl('生成失败，请检查参数', 'Generation failed, check the parameters')
  } finally {
    loading.value = false
  }
}

async function copyContent() {
  if (!result.value) return
  try {
    await navigator.clipboard.writeText(result.value.content)
    ElMessage.success(i18nStore.tl('已复制到剪贴板', 'Copied to clipboard'))
  } catch {
    ElMessage.error(i18nStore.tl('复制失败（浏览器不允许）', 'Copy failed (blocked by browser)'))
  }
}

function downloadContent() {
  if (!result.value) return
  const blob = new Blob([result.value.content], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = result.value.filename
  a.click()
  URL.revokeObjectURL(url)
}
</script>

<style scoped>
.rd-body {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.rd-hint {
  padding: 10px 12px;
  background: var(--cs-bg);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  color: var(--cs-text);
  font-size: 13px;
  line-height: 1.6;
}

.rd-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px 16px;
}

.rd-field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.rd-span2 {
  grid-column: span 2;
}

.rd-label {
  color: var(--cs-text);
  font-size: 13px;
  font-weight: 500;
}

.rd-control {
  width: 100%;
}

.rd-tip {
  color: var(--cs-text-dim);
  font-size: 12px;
}

.rd-actions {
  display: flex;
  gap: 10px;
  align-items: center;
}

.rd-error {
  padding: 10px 12px;
  border: 1px solid #f56c6c;
  border-radius: 8px;
  color: #f56c6c;
  font-size: 13px;
  white-space: pre-wrap;
}

.rd-result {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.rd-result-head {
  display: flex;
  gap: 14px;
  align-items: baseline;
  color: var(--cs-text);
  font-size: 13px;
}

.rd-file {
  font-weight: 600;
}

.rd-meta {
  color: var(--cs-text-dim);
  font-size: 12px;
}

.rd-uris {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.rd-uri {
  padding: 2px 6px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-card-border);
  border-radius: 4px;
  color: var(--cs-text);
  font-size: 12px;
}

.rd-output {
  width: 100%;
  min-height: 260px;
  padding: 10px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  color: var(--cs-text);
  font-size: 12px;
  line-height: 1.5;
  resize: vertical;
}

.rd-footer {
  text-align: right;
}
</style>
