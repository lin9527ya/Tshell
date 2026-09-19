<template>
  <div class="targets-view cs-font-ui" @click="closeContextMenu">
    <!-- Toolbar -->
    <div class="tv-toolbar">
      <div class="tv-title-group">
        <span class="tv-title">{{ isZh ? '目标列表' : 'Targets' }}</span>
        <span class="tv-count">({{ filteredTargets.length }} {{ isZh ? '个目标' : 'targets' }})</span>
      </div>

      <!-- View Switcher -->
      <div class="tv-mode-toggle">
        <button
          class="tv-mode-btn"
          :class="{ active: displayMode === 'table' }"
          :title="isZh ? '表格视图' : 'Table View'"
          @click="displayMode = 'table'"
        >
          <svg viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" stroke-width="2">
            <line x1="3" y1="6" x2="21" y2="6" />
            <line x1="3" y1="12" x2="21" y2="12" />
            <line x1="3" y1="18" x2="21" y2="18" />
          </svg>
          <span>{{ isZh ? '表格' : 'Table' }}</span>
        </button>
        <button
          class="tv-mode-btn"
          :class="{ active: displayMode === 'cards' }"
          :title="isZh ? '卡片网格视图' : 'Cards Grid View'"
          @click="displayMode = 'cards'"
        >
          <svg viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" stroke-width="2">
            <rect x="3" y="3" width="7" height="7" rx="1" />
            <rect x="14" y="3" width="7" height="7" rx="1" />
            <rect x="14" y="14" width="7" height="7" rx="1" />
            <rect x="3" y="14" width="7" height="7" rx="1" />
          </svg>
          <span>{{ isZh ? '卡片' : 'Cards' }}</span>
        </button>
      </div>

      <div class="tv-spacer" />

      <el-input
        v-model="searchKey"
        :placeholder="isZh ? '过滤目标 IP、名称、系统、备注...' : 'Filter target IP, name, OS, comment...'"
        size="small"
        clearable
        class="tv-search"
      />

      <button class="cs-btn" :disabled="syncingGraph" @click="syncGraphTargets()">
        {{ syncingGraph ? (isZh ? '同步中...' : 'Syncing...') : (isZh ? '同步拓扑' : 'Sync Graph') }}
      </button>
      <button class="cs-btn" @click="refresh">{{ isZh ? '刷新' : 'Refresh' }}</button>
    </div>

    <!-- Table / Cards View -->
    <div class="tv-table-wrap">
      <el-table
        v-if="displayMode === 'table'"
        :data="filteredTargets"
        size="small"
        height="100%"
        highlight-current-row
        :row-style="getRowStyle"
        class="cs-session-table"
        @current-change="onCurrentChange"
        @row-click="onRowClick"
        @row-dblclick="onRowDblClick"
        @row-contextmenu="onRowContextMenu"
      >
        <el-table-column prop="ip" :label="isZh ? 'IP 地址' : 'IP Address'" width="160" sortable>
          <template #default="{ row }">
            <span class="ip-text">{{ row.ip }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="computer" :label="isZh ? '计算机名称' : 'Name'" min-width="160" sortable show-overflow-tooltip />
        <el-table-column prop="os" :label="isZh ? '操作系统' : 'OS'" width="140" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="target-os-cell">
              <!-- OS 图标单一来源组件 -->
              <OsIcon :os="getOsType(row)" :size="14" class="target-os-svg" />
              <span class="os-text">{{ row.os }}</span>
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="version" :label="isZh ? '版本' : 'Version'" width="100" sortable />
        <el-table-column prop="note" :label="isZh ? '备注' : 'Comment'" min-width="180" sortable show-overflow-tooltip />

        <template #empty>
          <EmptyState
            variant="inline"
            :title="isZh ? '暂无目标主机' : 'No Targets Discovered'"
            :desc="isZh ? '通过【扫描】发现内网资产，或点击【添加】手动录入目标主机。' : 'Scan targets or add hosts manually.'"
          />
        </template>
      </el-table>

      <!-- Cards Grid View -->
      <div v-else class="tv-cards-grid">
        <div
          v-for="target in filteredTargets"
          :key="target.ip"
          class="tv-card"
          :class="{ active: selectedRow?.ip === target.ip }"
          :style="getRowStyle({ row: target })"
          @click="onRowClick(target)"
          @dblclick="onRowDblClick(target)"
          @contextmenu.prevent="onCardContextMenu($event, target)"
        >
          <div class="tv-card-header">
            <div class="tv-card-os">
              <OsIcon :os="getOsType(target)" :size="18" variant="simple" fallback="circle" />
            </div>
            <div class="tv-card-title-wrap">
              <span class="tv-card-ip cs-font-mono">{{ target.ip }}</span>
              <span class="tv-card-computer">{{ target.computer || '-' }}</span>
            </div>
            <span v-if="target.uid" class="tv-beacon-badge">BEACON</span>
          </div>

          <div class="tv-card-body">
            <div class="tv-card-row">
              <span class="tv-k">OS:</span>
              <span class="tv-v">{{ target.os }} {{ target.version }}</span>
            </div>
            <div v-if="target.note" class="tv-card-row">
              <span class="tv-k">{{ isZh ? '备注:' : 'Note:' }}</span>
              <span class="tv-v tv-note">{{ target.note }}</span>
            </div>
          </div>
        </div>

        <EmptyState
          v-if="filteredTargets.length === 0"
          :title="isZh ? '暂无目标主机' : 'No Targets Discovered'"
          :desc="isZh ? '通过【扫描】发现内网资产，或点击【添加】手动录入目标主机。' : 'Scan targets or add hosts manually.'"
        />
      </div>
    </div>

    <!-- Bottom Action Bar (CS Native Button Row) -->
    <div class="tv-bottom-bar">
      <button class="cs-btn cs-btn-primary" :disabled="!selectedRow" @click="openScanDialog">
        {{ isZh ? '扫描' : 'Scan' }}
      </button>
      <button class="cs-btn" :disabled="!selectedRow" @click="openServices">
        {{ isZh ? '服务' : 'Services' }}
      </button>
      <button class="cs-btn" :disabled="!selectedRow" @click="openCustomAction">
        {{ isZh ? '自定义' : 'Custom' }}
      </button>
      <button class="cs-btn" @click="importTargets">
        {{ isZh ? '导入' : 'Import' }}
      </button>
      <button class="cs-btn" @click="openAddTarget">
        {{ isZh ? '添加' : 'Add' }}
      </button>
      <button class="cs-btn cs-btn-danger" :disabled="!selectedRow" @click="deleteTarget">
        {{ isZh ? '删除' : 'Delete' }}
      </button>
      <button class="cs-btn" :disabled="!selectedRow" @click="editComment">
        {{ isZh ? '备注' : 'Note' }}
      </button>
      <button class="cs-btn" @click="handleHelp">
        {{ isZh ? '帮助' : 'Help' }}
      </button>
    </div>

    <!-- Right-Click Context Menu (1:1 CS Target Context Menu) -->
    <Teleport to="body">
      <div
        v-if="ctxMenu.visible"
        class="cs-listener-ctx-menu cs-font-ui"
        :style="{ left: ctxMenu.x + 'px', top: ctxMenu.y + 'px' }"
        @mousedown.stop
        @contextmenu.prevent
      >
        <div v-if="ctxMenu.row?.uid" class="ctx-item is-highlight" @click="openTargetBeacon(ctxMenu.row)">
          <span class="ctx-text">{{ isZh ? '会话交互(I)' : 'Interact (I)' }}</span>
        </div>
        <div class="ctx-item" @click="openScanDialog">{{ isZh ? '扫描...' : 'Scan...' }}</div>
        <div class="ctx-item" @click="openServices">{{ isZh ? '服务' : 'Services' }}</div>
        <div class="ctx-divider" />
        <div
          class="ctx-item has-sub"
          @mouseenter="ctxMenu.sub = 'host'"
          @mouseleave="ctxMenu.sub = null"
        >
          <span class="ctx-text">{{ isZh ? '主机' : 'Host' }}</span>
          <span class="ctx-arrow">▶</span>

          <div v-show="ctxMenu.sub === 'host'" class="ctx-sub-menu">
            <div class="ctx-item" @click="editOs">OS...</div>
            <div class="ctx-item" @click="editComment">{{ isZh ? '备注...' : 'Comment...' }}</div>
            <div class="ctx-divider" />
            <div
              class="ctx-item has-sub"
              @mouseenter="ctxMenu.showColor = true"
              @mouseleave="ctxMenu.showColor = false"
            >
              <span class="ctx-text">{{ isZh ? '颜色' : 'Color' }}</span>
              <span class="ctx-arrow">▶</span>

              <div v-show="ctxMenu.showColor" class="ctx-color-palette">
                <div
                  v-for="c in colorPalette"
                  :key="c.value"
                  class="color-box"
                  :style="{ backgroundColor: c.value }"
                  :title="c.label"
                  @click.stop="setColor(c.value)"
                />
              </div>
            </div>
            <div class="ctx-divider" />
            <div class="ctx-item ctx-danger" @click="deleteTarget">{{ isZh ? '删除' : 'Delete' }}</div>
          </div>
        </div>

        <div
          class="ctx-item has-sub"
          @mouseenter="ctxMenu.sub = 'login'"
          @mouseleave="ctxMenu.sub = null"
        >
          <span class="ctx-text">{{ isZh ? '登录' : 'Login' }}</span>
          <span class="ctx-arrow">▶</span>

          <div v-show="ctxMenu.sub === 'login'" class="ctx-sub-menu">
            <div class="ctx-item" @click="execLogin('psexec')">psexec...</div>
            <div class="ctx-item" @click="execLogin('wmi_exec')">wmi_exec...</div>
            <div class="ctx-item" @click="execLogin('winrm_exec')">winrm_exec...</div>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Scan Dialog (CS Style) -->
    <el-dialog
      v-model="scanDialogVisible"
      width="460px"
      class="cs-dialog"
      :close-on-click-modal="false"
    >
      <template #header>
        <span class="el-dialog__title">{{ isZh ? `端口扫描: ${selectedRow?.ip || '目标'}` : `Port Scan: ${selectedRow?.ip || 'Target'}` }}</span>
        <span class="tv-demo-badge">{{ isZh ? '演示' : 'Demo' }}</span>
      </template>
      <div class="cs-dialog-body cs-font-ui">
        <div class="cs-form-row">
          <label class="cs-form-label">{{ isZh ? '目标 IP / 网段' : 'Targets' }}</label>
          <div class="cs-form-input-wrap">
            <el-input v-model="scanForm.targets" size="small" />
          </div>
        </div>
        <div class="cs-form-row">
          <label class="cs-form-label">{{ isZh ? '端口范围' : 'Ports' }}</label>
          <div class="cs-form-input-wrap">
            <el-input v-model="scanForm.ports" size="small" placeholder="22,80,443,445,3389,8080" />
          </div>
        </div>
        <div class="cs-form-row">
          <label class="cs-form-label">{{ isZh ? '扫描方式' : 'Discovery' }}</label>
          <div class="cs-form-input-wrap">
            <el-select v-model="scanForm.method" size="small" style="width: 100%">
              <el-option label="ARP & ICMP Ping" value="arp" />
              <el-option label="TCP SYN Scan" value="syn" />
              <el-option label="TCP Connect" value="connect" />
            </el-select>
          </div>
        </div>
      </div>
      <template #footer>
        <div class="cs-dialog-footer-center">
          <button class="cs-btn cs-btn-primary" @click="startScan">{{ isZh ? '开始扫描' : 'Scan' }}</button>
           <button class="cs-btn" @click="scanDialogVisible = false">{{ isZh ? '取消' : 'Cancel' }}</button>
        </div>
      </template>
    </el-dialog>

    <!-- Lateral Movement Dialog (CS Style, real backend commands) -->
    <el-dialog
      v-model="lateralDialogVisible"
      :title="isZh ? `横向移动: ${lateralForm.method} → ${lateralForm.target}` : `Lateral Movement: ${lateralForm.method} → ${lateralForm.target}`"
      width="480px"
      class="cs-dialog"
      :close-on-click-modal="false"
    >
      <div class="cs-dialog-body cs-font-ui">
        <div class="cs-form-row">
          <label class="cs-form-label">{{ isZh ? '执行 Beacon' : 'Beacon' }}</label>
          <div class="cs-form-input-wrap">
            <el-select v-model="lateralForm.uid" size="small" style="width: 100%" :placeholder="isZh ? '选择发送执行的会话' : 'Select beacon to execute from'">
              <el-option
                v-for="c in onlineClients"
                :key="c.Uid"
                :label="`${c.Username}@${c.Computer || c.ExternalIP} (${c.Uid.slice(0, 8)})`"
                :value="c.Uid"
              />
            </el-select>
          </div>
        </div>
        <div class="cs-form-row">
          <label class="cs-form-label">{{ isZh ? '用户名' : 'User' }}</label>
          <div class="cs-form-input-wrap">
            <el-input v-model="lateralForm.user" size="small" placeholder="CORP\\administrator" />
          </div>
        </div>
        <div class="cs-form-row">
          <label class="cs-form-label">{{ isZh ? '密码' : 'Pass' }}</label>
          <div class="cs-form-input-wrap">
            <el-input v-model="lateralForm.pass" size="small" type="password" show-password :placeholder="isZh ? '密码（可空，使用当前令牌上下文）' : 'Password (optional, uses current token context)'" />
          </div>
        </div>
        <div class="cs-form-row">
          <label class="cs-form-label">{{ isZh ? '命令' : 'Command' }}</label>
          <div class="cs-form-input-wrap">
            <el-input v-model="lateralForm.command" size="small" :placeholder="isZh ? '远程执行的命令，如 whoami /all' : 'Remote command, e.g. whoami /all'" />
          </div>
        </div>
        <div class="cs-form-hint">
          {{ isZh ? '说明：密码为空时使用执行 Beacon 的当前令牌上下文（配合 make_token/steal_token）。结果回显到该 Beacon 的交互控制台。' : 'Empty password uses the executing beacon token context (pair with make_token/steal_token). Results echo to that beacon Interact console.' }}
        </div>
      </div>
      <template #footer>
        <div class="cs-dialog-footer-center">
          <button class="cs-btn cs-btn-primary" :disabled="!lateralForm.uid || !lateralForm.command" @click="startLateral">
            {{ isZh ? '执行' : 'Execute' }}
          </button>
          <button class="cs-btn" @click="lateralDialogVisible = false">{{ isZh ? '取消' : 'Cancel' }}</button>
        </div>
      </template>
    </el-dialog>

    <!-- Services Dialog (CS Style) -->

    <el-dialog
      v-model="servicesDialogVisible"
      width="580px"
      class="cs-dialog"
      :close-on-click-modal="false"
    >
      <template #header>
        <span class="el-dialog__title">{{ isZh ? `已开放服务列表 - ${selectedRow?.ip || ''}` : `Discovered Services - ${selectedRow?.ip || ''}` }}</span>
        <span class="tv-demo-badge">{{ isZh ? '演示' : 'Demo' }}</span>
      </template>
      <div class="cs-dialog-body cs-font-ui" style="max-height: 360px; overflow-y: auto">
        <div class="cs-form-hint">{{ isZh ? '演示数据，后端无服务探测接口' : 'Demo data — no backend service discovery API' }}</div>
        <el-table :data="discoveredServices" size="small" class="cs-session-table">
          <el-table-column prop="port" :label="isZh ? '端口' : 'Port'" width="80" align="right" />
          <el-table-column prop="proto" :label="isZh ? '协议' : 'Proto'" width="80" align="center" />
          <el-table-column prop="service" :label="isZh ? '服务' : 'Service'" width="120" />
          <el-table-column prop="banner" :label="isZh ? 'Banner 指纹' : 'Banner'" min-width="200" show-overflow-tooltip />
        </el-table>
      </div>
      <template #footer>
        <div class="cs-dialog-footer-center">
          <button class="cs-btn cs-btn-primary" @click="servicesDialogVisible = false">{{ isZh ? '确定' : 'OK' }}</button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useClientsStore } from '@/stores/clients'
import { useWorkspaceStore } from '@/stores/workspace'
import { useI18nStore } from '@/stores/i18n'
import { useTabActive, useVisibilityInterval } from '@/utils/useVisibilityInterval'
import { detectClientOs } from '@/utils/clientUtils'
import ClientAPI from '@/api/clients'
import EmptyState from '@/components/Common/EmptyState.vue'
import { CS_MARKER_COLORS, rowTintStyle } from '@/utils/sessionColors'
import OsIcon from '@/components/Common/OsIcon.vue'

export interface TargetRow {
  uid?: string
  ip: string
  computer: string
  os: string
  version: string
  note: string
  color?: string
}

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const clientsStore = useClientsStore()
const workspace = useWorkspaceStore()
const searchKey = ref('')
const selectedRow = ref<TargetRow | null>(null)
const displayMode = ref<'table' | 'cards'>('table')

function onCardContextMenu(e: MouseEvent, target: TargetRow) {
  selectedRow.value = target
  onRowContextMenu(target, null as any, e)
}

function getOsType(row: TargetRow): string {
  return detectClientOs({
    Os: row.os,
    Arch: row.version,
    Computer: row.computer,
  })
}

const scanDialogVisible = ref(false)
const servicesDialogVisible = ref(false)
const scanForm = ref({
  targets: '192.168.10.0/24',
  ports: '22,80,135,139,443,445,1433,3306,3389,8080',
  method: 'arp',
})

// 示例数据：后端无服务探测 API，服务列表为演示
const discoveredServices = ref([
  { port: 135, proto: 'tcp', service: 'msrpc', banner: 'Microsoft Windows RPC' },
  { port: 139, proto: 'tcp', service: 'netbios-ssn', banner: 'Microsoft NetBIOS' },
  { port: 445, proto: 'tcp', service: 'microsoft-ds', banner: 'Windows Server 2019 SMB' },
  { port: 3389, proto: 'tcp', service: 'ms-wbt-server', banner: 'Remote Desktop Protocol' },
])

// 手动添加的目标列表(localStorage 持久化,P2-12;仅存 IP/备注等非敏感字段)
const manualTargets = ref<TargetRow[]>([])
try {
  const saved = localStorage.getItem('cs-manual-targets')
  if (saved) manualTargets.value = JSON.parse(saved)
} catch { /* 忽略损坏数据 */ }
watch(manualTargets, (v) => {
  try { localStorage.setItem('cs-manual-targets', JSON.stringify(v)) } catch { /* 配额满忽略 */ }
}, { deep: true })

// 后端 /graph/data 同步回来的目标节点（来自隧道代理 target 节点）
const syncedTargets = ref<TargetRow[]>([])
const syncingGraph = ref(false)

async function syncGraphTargets(silent = false) {
  syncingGraph.value = true
  try {
    const res = await ClientAPI.get_graph_data()
    const nodes: any[] = res.data?.data?.nodes || []
    const targets: TargetRow[] = nodes
      .filter((n) => n.type === 'target' && n.label)
      .map((n) => ({
        ip: n.label,
        computer: n.sub_label || n.label,
        os: n.meta?.os || (isZh.value ? '拓扑目标' : 'Pivot Target'),
        version: n.meta?.version || '',
        note: n.meta?.note || (isZh.value ? '来自代理转发的拓扑目标' : 'Target discovered via proxy pivot'),
      }))
    syncedTargets.value = targets
    if (!silent) {
      ElMessage.success(isZh.value ? `已从拓扑图同步 ${targets.length} 个代理目标` : `Synced ${targets.length} pivot target(s) from graph`)
    }
  } catch {
    if (!silent) {
      ElMessage.warning(isZh.value ? '拓扑目标同步失败（需要 operator/admin 权限）' : 'Graph sync failed (requires operator/admin role)')
    }
  } finally {
    syncingGraph.value = false
  }
}

const targetList = computed<TargetRow[]>(() => {
  const clientTargets: TargetRow[] = clientsStore.clientsList.map((c) => {
    const osType = detectClientOs(c)
    const osDisplay = c.Os || (osType === 'windows' ? 'Windows' : (osType === 'linux' ? 'Linux' : (osType === 'darwin' ? 'macOS' : 'Unknown')))
    return {
      uid: c.Uid,
      ip: c.ExternalIP || c.InternalIP || '127.0.0.1',
      computer: c.Computer || 'Host',
      os: osDisplay,
      version: c.Arch || '',
      note: c.Note || '',
      color: workspace.sessionColors[c.Uid] || c.Color || '',
    }
  })

  const combined: TargetRow[] = [...clientTargets]

  // 合并拓扑同步的目标（去重）
  syncedTargets.value.forEach((st) => {
    if (!combined.some((c) => c.ip === st.ip)) {
      combined.push(st)
    }
  })

  // 合并手动录入的目标（去重）
  manualTargets.value.forEach((mt) => {
    if (!combined.some((c) => c.ip === mt.ip)) {
      combined.push(mt)
    }
  })

  return combined
})

const filteredTargets = computed(() => {
  if (!searchKey.value.trim()) return targetList.value
  const q = searchKey.value.toLowerCase().trim()
  return targetList.value.filter(
    (t) =>
      t.ip.toLowerCase().includes(q) ||
      t.computer.toLowerCase().includes(q) ||
      t.os.toLowerCase().includes(q) ||
      t.note.toLowerCase().includes(q),
  )
})

const ctxMenu = ref({
  visible: false,
  x: 0,
  y: 0,
  row: null as TargetRow | null,
  sub: null as string | null,
  showColor: false,
})

const colorPalette = CS_MARKER_COLORS

function getRowStyle({ row }: { row: TargetRow }) {
  const color = row.color || (row.uid ? workspace.sessionColors[row.uid] : '')
  return rowTintStyle(color, { alpha: '2c', borderWidth: 4 })
}

function onCurrentChange(row: TargetRow | null) {
  if (row) selectedRow.value = row
}

function onRowClick(row: TargetRow) {
  selectedRow.value = row
}

function onRowDblClick(row: TargetRow) {
  selectedRow.value = row
  if (row.uid) {
    openTargetBeacon(row)
  } else {
    openServices()
  }
}

function onRowContextMenu(row: TargetRow, col: any, e: MouseEvent) {
  e.preventDefault()
  selectedRow.value = row
  ctxMenu.value = {
    visible: true,
    x: e.clientX,
    y: e.clientY,
    row,
    sub: null,
    showColor: false,
  }
}

function closeContextMenu() {
  ctxMenu.value.visible = false
  ctxMenu.value.sub = null
  ctxMenu.value.showColor = false
}

function setColor(color: string) {
  if (ctxMenu.value.row) {
    ctxMenu.value.row.color = color
    if (ctxMenu.value.row.uid) {
      workspace.setSessionColor(ctxMenu.value.row.uid, color)
    }
  }
  closeContextMenu()
}

function openTargetBeacon(row: any) {
  closeContextMenu()
  if (!row?.uid) {
    ElMessage.info(isZh.value ? `目标 ${row.ip} 暂无活动 Beacon 会话。` : `Target ${row.ip} does not have an active Beacon session yet.`)
    return
  }
  workspace.openTab({
    id: `beacon:${row.uid}`,
    title: isZh.value ? `会话交互: ${row.computer || row.ip}` : `Beacon: ${row.computer || row.ip}`,
    component: 'beacon',
    params: { uid: row.uid },
  })
}

function openScanDialog() {
  closeContextMenu()
  if (selectedRow.value) {
    scanForm.value.targets = selectedRow.value.ip
  }
  scanDialogVisible.value = true
}

function startScan() {
  scanDialogVisible.value = false
  ElMessage.warning(isZh.value ? '演示功能：后端/客户端未实现端口扫描，任务不会实际下发' : 'Demo only: port scanning is not implemented on backend/client; nothing will be dispatched')
}

function openServices() {
  closeContextMenu()
  servicesDialogVisible.value = true
}

function openCustomAction() {
  closeContextMenu()
  ElMessage.info(isZh.value ? '已打开目标自定义后渗透与探测菜单' : 'Custom target action dialog opened')
}

function importTargets() {
  // 粘贴导入(P2-12,纯前端): 每行一个 IP 或 IP/24 网段,支持逗号/空格/分号分隔
  ElMessageBox.prompt(
    isZh.value ? '粘贴目标列表(每行一个 IP 或网段,如 192.168.1.10 / 10.0.0.0/24):' : 'Paste target list (one IP or CIDR per line):',
    isZh.value ? '导入目标' : 'Import Targets',
    {
      confirmButtonText: isZh.value ? '导入' : 'Import',
      cancelButtonText: isZh.value ? '取消' : 'Cancel',
      inputType: 'textarea',
      inputPlaceholder: '192.168.1.10\n192.168.1.11\n10.0.0.0/24',
    },
  ).then(({ value }) => {
    if (!value) return
    const seen = new Set(manualTargets.value.map((t) => t.ip))
    let added = 0
    for (const raw of value.split(/[\n,;]+/)) {
      const ip = raw.trim()
      if (!ip || seen.has(ip)) continue
      // 合法性粗检: IPv4 / CIDR / 主机名
      if (!/^(\d{1,3}\.){3}\d{1,3}(\/\d{1,2})?$|^[a-zA-Z0-9._-]+$/.test(ip)) continue
      manualTargets.value.push({
        ip,
        computer: `HOST-${ip.split('.').slice(-1)[0]}`,
        os: 'Windows',
        version: '',
        note: isZh.value ? '导入' : 'Imported',
      })
      seen.add(ip)
      added++
    }
    if (added > 0) ElMessage.success(isZh.value ? `已导入 ${added} 个目标` : `Imported ${added} targets`)
    else ElMessage.warning(isZh.value ? '未解析到有效目标' : 'No valid targets found')
  }).catch(() => {})
}

function openAddTarget() {
  ElMessageBox.prompt(isZh.value ? '输入目标 IP 地址或网段：' : 'Enter Target IP address:', isZh.value ? '添加目标' : 'Add Target', {
    confirmButtonText: isZh.value ? '添加' : 'Add',
    cancelButtonText: isZh.value ? '取消' : 'Cancel',
  }).then(({ value }) => {
    if (value) {
      const trimmed = value.trim()
      manualTargets.value.push({
        ip: trimmed,
        computer: `HOST-${trimmed.split('.').slice(-1)[0]}`,
        os: 'Windows',
        version: '',
        note: isZh.value ? '手动录入' : 'Manually Added',
      })
      ElMessage.success(isZh.value ? `目标 ${trimmed} 已添加` : `Target ${trimmed} added`)
    }
  }).catch(() => {})
}

function editOs() {
  closeContextMenu()
  ElMessageBox.prompt(isZh.value ? '设置目标操作系统：' : 'Set OS for target:', isZh.value ? '编辑系统' : 'Edit OS', {
    inputValue: selectedRow.value?.os || 'Windows',
  }).then(({ value }) => {
    if (selectedRow.value && value) selectedRow.value.os = value
  }).catch(() => {})
}

function editComment() {
  closeContextMenu()
  if (!selectedRow.value) return
  ElMessageBox.prompt(isZh.value ? '设置目标备注：' : 'Set comment for target:', isZh.value ? '编辑备注' : 'Edit Comment', {
    inputValue: selectedRow.value.note || '',
    confirmButtonText: isZh.value ? '确定' : 'OK',
    cancelButtonText: isZh.value ? '取消' : 'Cancel',
  }).then(({ value }) => {
    if (selectedRow.value) {
      selectedRow.value.note = value || ''
      if (selectedRow.value.uid) {
        const client = clientsStore.getClientByUid(selectedRow.value.uid)
        if (client) client.Note = value || ''
      }
      ElMessage.success(isZh.value ? '备注已更新' : 'Comment updated')
    }
  }).catch(() => {})
}

function deleteTarget() {
  closeContextMenu()
  if (!selectedRow.value) return
  const ip = selectedRow.value.ip
  ElMessageBox.confirm(isZh.value ? `确定移除目标 ${ip} 吗？` : `Remove target ${ip}?`, isZh.value ? '删除目标' : 'Delete Target', {
    type: 'warning',
  }).then(() => {
    const mIdx = manualTargets.value.findIndex((t) => t.ip === ip)
    if (mIdx !== -1) {
      manualTargets.value.splice(mIdx, 1)
      selectedRow.value = null
      ElMessage.success(isZh.value ? '目标已移除' : 'Target removed')
      return
    }
    const sIdx = syncedTargets.value.findIndex((t) => t.ip === ip)
    if (sIdx !== -1) {
      syncedTargets.value.splice(sIdx, 1)
      selectedRow.value = null
      ElMessage.success(isZh.value ? '目标已移除' : 'Target removed')
      return
    }
    ElMessage.warning(isZh.value ? '该目标来自已连接的 Beacon 会话，请通过会话表进行删除' : 'This target comes from an active Beacon session; manage it from the session table')
  }).catch(() => {})
}

function execLogin(method: string) {
  closeContextMenu()
  if (!selectedRow.value) return
  const ip = selectedRow.value.ip
  lateralForm.method = method
  lateralForm.target = ip
  lateralForm.user = ''
  lateralForm.pass = ''
  lateralForm.command = 'whoami /all'
  lateralForm.uid = workspace.primarySelectedUid || onlineClients.value[0]?.Uid || ''
  lateralDialogVisible.value = true
}

const lateralDialogVisible = ref(false)
const lateralForm = reactive({
  method: 'psexec',
  target: '',
  uid: '',
  user: '',
  pass: '',
  command: '',
})

const onlineClients = computed(() => clientsStore.clientsList.filter((c) => c.Online === '1'))

// 实际下发横向移动命令（结果回显到执行 Beacon 的交互控制台）
async function startLateral() {
  if (!lateralForm.uid || !lateralForm.command) return
  const cmd = `${lateralForm.method} ${lateralForm.target}|${lateralForm.user || ''}|${lateralForm.pass || ''}|${lateralForm.command}`
  try {
    await ClientAPI.send_commands({ uid: lateralForm.uid, command: cmd })
    ElMessage.success(isZh.value ? '横向移动指令已下发，结果回显到交互控制台' : 'Lateral movement command sent; results echo to the Interact console')
    lateralDialogVisible.value = false
    workspace.openTab({
      id: `beacon:${lateralForm.uid}`,
      title: `beacon: ${lateralForm.uid.slice(0, 8)}`,
      component: 'beacon',
      params: { uid: lateralForm.uid },
    })
  } catch (e: any) {
    ElMessage.error(isZh.value ? '下发失败: ' + (e?.message || '') : 'Failed: ' + (e?.message || ''))
  }
}

function handleHelp() {
  ElMessage.info(isZh.value ? '目标列表汇集网络资产探测与横向移动渗透目标。' : 'Targets Table Help')
}

// 静默刷新(不带提示),供定时轮询复用
async function refreshSilent() {
  await Promise.all([
    clientsStore.refreshClients().catch(() => {}),
    syncGraphTargets(true).catch(() => {}),
  ])
}

async function refresh() {
  await refreshSilent()
  ElMessage.success(isZh.value ? '目标列表已刷新' : 'Targets refreshed')
}

// 【性能】15s 自动刷新接入可见性治理(非活动标签/页面隐藏时暂停)
const tabActive = useTabActive()
useVisibilityInterval(refreshSilent, 15000, { isActive: () => tabActive.value })

onMounted(() => {
  window.addEventListener('mousedown', closeContextMenu)
  if (!clientsStore.clientsList.length) {
    clientsStore.refreshClients().catch(() => {})
  }
  syncGraphTargets(true)
})

onBeforeUnmount(() => {
  window.removeEventListener('mousedown', closeContextMenu)
})
</script>

<style scoped>
.cs-form-hint {
  margin-top: 4px;
  font-size: 11px;
  color: var(--cs-text-dim);
  line-height: 1.5;
}

.targets-view {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-panel);
  overflow: hidden;
}

.tv-toolbar {
  height: 36px;
  background: var(--cs-titlebar-bg);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 12px;
  flex-shrink: 0;
}

.tv-title-group {
  display: flex;
  align-items: center;
  gap: 8px;
}

.tv-mode-toggle {
  display: flex;
  align-items: center;
  gap: 2px;
  background: var(--cs-bg);
  border: 1px solid var(--cs-border);
  border-radius: 3px;
  padding: 2px;
}

.tv-mode-btn {
  height: 22px;
  padding: 0 8px;
  font-size: 13px;
  color: var(--cs-text-dim);
  background: transparent;
  border: none;
  border-radius: 2px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  transition: all 100ms ease;
}

.tv-mode-btn:hover {
  color: var(--cs-text);
  background: var(--cs-hover);
}

.tv-mode-btn.active {
  color: #ffffff;
  background: var(--cs-active);
  font-weight: 600;
}

.tv-title {
  font-size: 17px;
  font-weight: bold;
  color: var(--cs-text-bright);
}

.tv-count {
  font-size: 16px;
  color: var(--cs-text-dim);
}

.tv-spacer {
  flex: 1;
}

.tv-search {
  width: 280px;
}

.tv-table-wrap {
  flex: 1;
  overflow: hidden;
  position: relative;
}

/* Cards Grid View */
.tv-cards-grid {
  height: 100%;
  overflow-y: auto;
  padding: 12px;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 12px;
  align-content: start;
}

.tv-card {
  background: var(--cs-panel);
  border: 1px solid var(--cs-border);
  border-radius: 4px;
  padding: 10px 12px;
  cursor: pointer;
  transition: all 120ms ease;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.tv-card:hover {
  background: var(--cs-hover);
  border-color: color-mix(in srgb, var(--cs-active) 60%, transparent);
  transform: translateY(-1px);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.2);
}

.tv-card.active {
  border-color: var(--cs-active);
  box-shadow: inset 0 0 0 1px var(--cs-active), 0 2px 8px rgba(0, 0, 0, 0.3);
}

.tv-card-header {
  display: flex;
  align-items: center;
  gap: 8px;
}

.tv-card-os {
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.tv-card-title-wrap {
  flex: 1;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.tv-card-ip {
  font-size: 15px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.tv-card-computer {
  font-size: 12.5px;
  color: var(--cs-text-dim);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tv-beacon-badge {
  background: color-mix(in srgb, var(--cs-online, #4caf50) 20%, transparent);
  border: 1px solid var(--cs-online, #4caf50);
  color: var(--cs-online, #4caf50);
  font-size: 10px;
  font-weight: bold;
  padding: 1px 4px;
  border-radius: 2px;
}

/* 演示徽标 */
.tv-demo-badge {
  display: inline-block;
  margin-left: 8px;
  padding: 1px 6px;
  border-radius: 2px;
  font-size: 11px;
  font-weight: 600;
  vertical-align: middle;
  background: color-mix(in srgb, var(--cs-warning) 18%, transparent);
  border: 1px solid color-mix(in srgb, var(--cs-warning) 45%, transparent);
  color: var(--cs-warning);
}

.tv-card-body {
  font-size: 13px;
  color: var(--cs-text);
  border-top: 1px dashed var(--cs-border);
  padding-top: 6px;
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.tv-card-row {
  display: flex;
  gap: 6px;
  overflow: hidden;
}

.tv-k {
  color: var(--cs-text-dim);
  font-size: 11.5px;
  flex-shrink: 0;
}

.tv-v {
  color: var(--cs-text);
  font-size: 12.5px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tv-note {
  color: var(--cs-accent, #ff9800);
  font-style: italic;
}

.ip-text {
  font-family: var(--cs-font-mono);
  color: var(--cs-text-bright);
  font-size: 16.5px;
}

.os-text {
  color: var(--cs-text);
  font-size: 16.5px;
}

.tv-bottom-bar {
  height: 36px;
  background: var(--cs-bg);
  border-top: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 12px;
  flex-shrink: 0;
}

/* Context Menu */
.cs-listener-ctx-menu {
  position: fixed;
  background: var(--cs-panel);
  border: 1px solid var(--cs-border);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.45);
  border-radius: 2px;
  padding: 4px 0;
  z-index: 9999;
  min-width: 190px;
}

.ctx-sub-menu {
  position: absolute;
  top: -2px;
  left: 100%;
  background: var(--cs-panel);
  border: 1px solid var(--cs-border);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.45);
  border-radius: 2px;
  padding: 4px 0;
  z-index: 10000;
  min-width: 190px;
}

.ctx-item {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 14px;
  height: 30px;
  font-size: 16.5px;
  color: var(--cs-text);
  cursor: pointer;
  white-space: nowrap;
}

.ctx-item:hover {
  background: var(--cs-hover);
  color: var(--cs-text-bright);
}

.ctx-item.is-highlight {
  font-weight: bold;
  color: var(--cs-active);
}

.ctx-text {
  font-size: 16.5px;
}

.ctx-arrow {
  font-size: 12px;
  color: var(--cs-text-dim);
}

.ctx-divider {
  height: 1px;
  background: var(--cs-border);
  margin: 3px 0;
}

.ctx-color-palette {
  position: absolute;
  top: -2px;
  left: 100%;
  display: grid;
  grid-template-columns: repeat(4, 20px);
  gap: 4px;
  padding: 6px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-border);
  border-radius: 2px;
  box-shadow: 0 4px 12px rgba(0,0,0,0.5);
  z-index: 10001;
}

.color-box {
  width: 20px;
  height: 20px;
  border-radius: 2px;
  cursor: pointer;
  border: 1px solid rgba(0,0,0,0.2);
}
.color-box:hover {
  transform: scale(1.15);
  box-shadow: 0 0 4px rgba(255,255,255,0.6);
}

/* Dialog styling */
.cs-dialog-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px 16px;
}

.cs-form-row {
  display: flex;
  align-items: center;
  gap: 12px;
}

.cs-form-label {
  width: 100px;
  font-size: 12px;
  text-align: right;
  color: var(--cs-text-dim);
}

.cs-form-input-wrap {
  flex: 1;
}

.cs-dialog-footer-center {
  display: flex;
  justify-content: center;
  gap: 8px;
  padding: 8px 0;
}

.target-os-cell {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.target-os-svg {
  display: inline-block;
  flex-shrink: 0;
}
</style>
