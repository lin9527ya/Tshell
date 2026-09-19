<template>
  <el-dialog
    v-model="visible"
    :title="isZh ? '报告' : 'Reports'"
    width="820px"
    :close-on-click-modal="false"
    append-to-body
    class="cs-dialog"
  >
    <div class="reports-container cs-font-ui">
      <el-tabs v-model="activeTab" class="cs-reports-tabs">
        <!-- 威胁指标 IOC -->
        <el-tab-pane :label="isZh ? '威胁指标 (IOC)' : 'Indicators (IOC)'" name="ioc">
          <div class="rp-toolbar">
            <button class="cs-btn cs-btn-primary" @click="loadIOC">{{ isZh ? '生成' : 'Generate' }}</button>
            <button class="cs-btn" :disabled="!ioc" @click="exportIOC">{{ isZh ? '导出' : 'Export' }}</button>
            <span v-if="ioc" class="rp-meta">{{ ioc.generated_at }}</span>
          </div>
          <div v-if="ioc" class="rp-stats">
            <div class="rp-stat-card"><b>{{ ioc.hosts?.length || 0 }}</b><span>{{ isZh ? '会话主机' : 'Hosts' }}</span></div>
            <div class="rp-stat-card"><b>{{ ioc.listener_addresses?.length || 0 }}</b><span>{{ isZh ? '监听地址' : 'Listeners' }}</span></div>
            <div class="rp-stat-card"><b>{{ ioc.blocked_ips?.length || 0 }}</b><span>{{ isZh ? '封禁 IP' : 'Blocked' }}</span></div>
          </div>
          <el-table v-if="ioc" :data="ioc.hosts || []" size="small" height="300" class="cs-session-table">
            <el-table-column prop="host" label="主机" width="150" show-overflow-tooltip />
            <el-table-column prop="user" label="用户" width="110" />
            <el-table-column prop="external_ip" label="外部 IP" width="120" />
            <el-table-column prop="internal_ip" label="内网 IP" width="120" />
            <el-table-column prop="listener" label="监听器" width="90" />
            <el-table-column prop="online" label="状态" width="70">
              <template #default="{ row }">
                <span :class="row.online === '1' ? 'rp-online' : 'rp-offline'">{{ row.online === '1' ? (isZh ? '在线' : 'ON') : (isZh ? '离线' : 'OFF') }}</span>
              </template>
            </el-table-column>
          </el-table>
          <div v-if="ioc" class="rp-extra">
            <div class="rp-extra-title">{{ isZh ? '监听地址:' : 'Listener Addresses:' }}</div>
            <div class="rp-extra-body mono">{{ (ioc.listener_addresses || []).join('  ') || '-' }}</div>
            <div class="rp-extra-title">{{ isZh ? '封禁 IP:' : 'Blocked IPs:' }}</div>
            <div class="rp-extra-body mono">{{ (ioc.blocked_ips || []).join('  ') || '-' }}</div>
          </div>
        </el-tab-pane>

        <!-- 社会工程 Social -->
        <el-tab-pane :label="isZh ? '社会工程' : 'Social Engineering'" name="social">
          <div class="rp-toolbar">
            <button class="cs-btn cs-btn-primary" @click="loadSocial">{{ isZh ? '生成' : 'Generate' }}</button>
            <button class="cs-btn" :disabled="!social" @click="exportSocial">{{ isZh ? '导出' : 'Export' }}</button>
            <span v-if="social" class="rp-meta">{{ social.generated_at }}</span>
          </div>
          <div v-if="social" class="rp-stats">
            <div class="rp-stat-card"><b>{{ social.total_clicks || 0 }}</b><span>{{ isZh ? '投递访问' : 'Clicks' }}</span></div>
            <div class="rp-stat-card"><b>{{ social.unique_ips || 0 }}</b><span>{{ isZh ? '来源 IP' : 'Unique IPs' }}</span></div>
            <div class="rp-stat-card"><b>{{ social.deliveries?.length || 0 }}</b><span>{{ isZh ? '投递项' : 'Deliveries' }}</span></div>
          </div>
          <el-table v-if="social" :data="social.clicks || []" size="small" height="300" class="cs-session-table">
            <el-table-column prop="time" label="时间" width="150" />
            <el-table-column prop="resource" label="资源" width="90" />
            <el-table-column prop="client_ip" label="来源 IP" width="130" />
            <el-table-column prop="user_agent" label="User-Agent" min-width="220" show-overflow-tooltip />
            <el-table-column prop="status_code" label="状态" width="70" align="center" />
          </el-table>
        </el-tab-pane>

        <!-- 战术 TTP -->
        <el-tab-pane :label="isZh ? '战术 TTP' : 'Tactics & TTP'" name="ttp">
          <div class="rp-toolbar">
            <button class="cs-btn cs-btn-primary" @click="loadTTP">{{ isZh ? '生成' : 'Generate' }}</button>
            <button class="cs-btn" :disabled="!ttp" @click="exportTTP">{{ isZh ? '导出' : 'Export' }}</button>
            <span v-if="ttp" class="rp-meta">{{ ttp.generated_at }}</span>
          </div>
          <div v-if="ttp" class="rp-stats">
            <div class="rp-stat-card"><b>{{ ttp.techniques?.length || 0 }}</b><span>{{ isZh ? '识别技术' : 'Techniques' }}</span></div>
            <div class="rp-stat-card"><b>{{ ttp.per_session?.length || 0 }}</b><span>{{ isZh ? '行为条目' : 'Activities' }}</span></div>
          </div>
          <el-table v-if="ttp" :data="ttp.techniques || []" size="small" height="200" class="cs-session-table">
            <el-table-column prop="tactic" label="战术" width="120" />
            <el-table-column prop="technique" label="技术 (MITRE)" min-width="260" />
            <el-table-column prop="count" label="次数" width="80" align="center" />
          </el-table>
          <el-table v-if="ttp && ttp.per_session?.length" :data="ttp.per_session.slice(0, 100)" size="small" height="120" class="cs-session-table">
            <el-table-column prop="host" label="主机" width="140" show-overflow-tooltip />
            <el-table-column prop="command" label="命令" min-width="240" show-overflow-tooltip />
            <el-table-column prop="tactic" label="战术" width="100" />
          </el-table>
        </el-tab-pane>
      </el-tabs>
    </div>

    <template #footer>
      <div class="cs-dialog-footer-center">
        <button class="cs-btn" @click="close">{{ isZh ? '关闭' : 'Close' }}</button>
      </div>
    </template>
  </el-dialog>
</template>

<script lang="ts">
// 【修复】报告菜单三个入口(威胁指标/社会工程/战术TTP)此前都只打开同一个
// 默认 ioc tab,点了不同项看到同一窗口。模块级预设: 菜单在打开前调用
// presetReportsTab('social') 等,对话框打开时消费一次。
// (模块级状态与 export 必须放普通 <script> 块——script setup 内不允许 export)
let pendingReportsTab: string | null = null
export function presetReportsTab(tab: 'ioc' | 'social' | 'ttp') {
  pendingReportsTab = tab
}
function consumeReportsTab(): string | null {
  const t = pendingReportsTab
  pendingReportsTab = null
  return t
}
</script>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useWorkspaceStore } from '@/stores/workspace'
import { useI18nStore } from '@/stores/i18n'
import reportApi from '@/api/report'

const workspace = useWorkspaceStore()
const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const visible = computed({
  get: () => workspace.dialogs.reports,
  set: (v) => {
    if (!v) workspace.closeDialog('reports')
  },
})

const activeTab = ref('ioc')
const ioc = ref<any>(null)
const social = ref<any>(null)
const ttp = ref<any>(null)

// 打开时消费菜单预设的 tab(见上方普通 script 块)
watch(visible, (v) => {
  if (v) {
    const t = consumeReportsTab()
    if (t) activeTab.value = t
  }
})

async function loadIOC() {
  try {
    const res = await reportApi.getIOC()
    ioc.value = res?.data?.data || null
  } catch {
    ElMessage.error(isZh.value ? '生成失败' : 'Generate failed')
  }
}

async function loadSocial() {
  try {
    const res = await reportApi.getSocial()
    social.value = res?.data?.data || null
  } catch {
    ElMessage.error(isZh.value ? '生成失败' : 'Generate failed')
  }
}

async function loadTTP() {
  try {
    const res = await reportApi.getTTP()
    ttp.value = res?.data?.data || null
  } catch {
    ElMessage.error(isZh.value ? '生成失败' : 'Generate failed')
  }
}

function download(name: string, text: string) {
  const blob = new Blob([text], { type: 'text/plain' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = name
  a.click()
  URL.revokeObjectURL(url)
}

function exportIOC() {
  if (!ioc.value) return
  download(`ioc_report_${Date.now()}.json`, JSON.stringify(ioc.value, null, 2))
}

function exportSocial() {
  if (!social.value) return
  download(`social_report_${Date.now()}.json`, JSON.stringify(social.value, null, 2))
}

function exportTTP() {
  if (!ttp.value) return
  download(`ttp_report_${Date.now()}.json`, JSON.stringify(ttp.value, null, 2))
}

function close() {
  workspace.closeDialog('reports')
}
</script>

<style scoped>
.reports-container {
  min-height: 420px;
}
.rp-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}
.rp-meta {
  font-size: 11px;
  color: var(--cs-text-dim);
  font-family: var(--cs-font-mono);
}
.rp-stats {
  display: flex;
  gap: 10px;
  margin-bottom: 8px;
}
.rp-stat-card {
  border: 1px solid var(--cs-border);
  border-radius: 3px;
  padding: 6px 14px;
  display: flex;
  flex-direction: column;
  align-items: center;
  background: var(--cs-panel);
}
.rp-stat-card b {
  font-size: 16px;
  color: var(--cs-text-bright);
}
.rp-stat-card span {
  font-size: 11px;
  color: var(--cs-text-dim);
}
.rp-online { color: #67c23a; }
.rp-offline { color: var(--cs-text-dim); }
.rp-extra {
  margin-top: 10px;
}
.rp-extra-title {
  font-size: 11px;
  color: var(--cs-text-dim);
  margin-top: 6px;
}
.rp-extra-body {
  font-size: 11px;
  color: var(--cs-text);
  word-break: break-all;
}
.cs-dialog-footer-center {
  display: flex;
  justify-content: center;
  gap: 8px;
}
</style>
