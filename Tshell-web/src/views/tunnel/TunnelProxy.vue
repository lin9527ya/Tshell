<template>
  <div class="tunnel-proxy-container cs-font-ui">
    <PageHeader :title="i18nStore.tl('隧道代理', 'Tunnels')" />

    <!-- 页签：隧道 / SOCKS5（vshell：同域功能并页） -->
    <div class="tp-tabs">
      <div class="tp-tab" :class="{ active: view === 'tunnel' }" @click="view = 'tunnel'">
        {{ i18nStore.tl('隧道代理', 'Tunnels') }}
      </div>
      <div class="tp-tab" :class="{ active: view === 'socks5' }" @click="view = 'socks5'">
        {{ i18nStore.tl('SOCKS5 代理', 'SOCKS5') }}
      </div>
    </div>

    <template v-if="view === 'tunnel'">
    <!-- vshell 工具条：左（标题 + 计数），右（筛选 + 操作） -->
    <div class="tp-toolbar">
      <div class="tp-title-group">
        <span class="tp-count">({{ filteredTunnelList.filter(t => t.status === 'active').length }} {{ i18nStore.tl('个活跃', 'active') }} / {{ filteredTunnelList.length }} {{ i18nStore.tl('个总数', 'total') }})</span>
      </div>

      <div class="tp-spacer" />

      <el-select v-model="protocolFilter" :placeholder="i18nStore.tl('协议', 'Protocol')" clearable class="tp-filter">
        <el-option :label="i18nStore.tl('全部', 'ALL')" value="" />
        <el-option label="TCP" value="tcp" />
        <el-option label="UDP" value="udp" />
        <el-option label="HTTP" value="http" />
        <el-option label="SOCKS5" value="socks5" />
      </el-select>

      <el-select v-model="statusFilter" :placeholder="i18nStore.tl('状态', 'Status')" clearable class="tp-filter-sm">
        <el-option :label="i18nStore.tl('全部', 'ALL')" value="" />
        <el-option :label="i18nStore.tl('活跃', 'Active')" value="1" />
        <el-option :label="i18nStore.tl('停止', 'Stopped')" value="0" />
      </el-select>

      <el-input
        v-model="searchKeyword"
        :placeholder="i18nStore.tl('过滤备注、地址、用户...', 'Filter note, address, user...')"
        clearable
        class="tp-search"
        @input="handleSearch"
      >
        <template #prefix>
          <svg class="tp-search-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
            <circle cx="11" cy="11" r="7" />
            <path d="M20 20l-3.6-3.6" />
          </svg>
        </template>
      </el-input>

      <button class="cs-btn" @click="refreshTunnelList">
        {{ i18nStore.tl('刷新', 'Refresh') }}
      </button>

      <span class="tp-toolbar-divider" />

      <button class="cs-btn cs-btn-primary" @click="showCreateDialog">
        {{ i18nStore.tl('添加隧道', 'Add Tunnel') }}
      </button>
      <button class="cs-btn" :disabled="selectedTunnelIds.length === 0" @click="batchStartTunnels">
        {{ i18nStore.tl('启动', 'Start') }}
      </button>
      <button class="cs-btn" :disabled="selectedTunnelIds.length === 0" @click="batchStopTunnels">
        {{ i18nStore.tl('停止', 'Stop') }}
      </button>
      <button class="cs-btn cs-btn-danger" :disabled="selectedTunnelIds.length === 0" @click="batchDeleteTunnels">
        {{ i18nStore.tl('删除', 'Remove') }}
      </button>
      <button class="cs-btn" @click="handleHelp">
        {{ i18nStore.tl('帮助', 'Help') }}
      </button>
    </div>

    <div class="tp-content">
      <!-- 内联配置面板（vshell 卡片：新建/编辑隧道，展开在表格上方） -->
      <div v-if="createDialogVisible" class="cs-inline-panel">
        <div class="cs-inline-header">
          <span class="cs-inline-title">{{ isEditMode ? (i18nStore.tl('编辑隧道代理', 'Edit Proxy Pivot')) : (i18nStore.tl('新建隧道代理', 'New Proxy Pivot')) }}</span>
          <button class="cs-btn cs-btn-xs" @click="handleCloseDialog">{{ i18nStore.tl('关闭', 'Close') }}</button>
        </div>

        <div class="cs-inline-body">
          <div class="cs-form-row">
            <label class="cs-form-label"><span class="req">*</span> {{ i18nStore.tl('选择客户端', 'Beacon') }}</label>
            <div class="cs-form-input-wrap">
              <el-select
                v-model="tunnelForm.clientUid"
                :placeholder="i18nStore.tl('请选择已上线的客户端', 'Select online beacon')"

                style="width: 100%"
                :disabled="isEditMode"
                @change="onClientChange"
              >
                <el-option
                  v-for="client in onlineClients"
                  :key="client.Uid"
                  :label="getClientDisplayInfo(client)"
                  :value="client.Uid"
                />
              </el-select>
            </div>
          </div>

          <div class="cs-form-row">
            <label class="cs-form-label"><span class="req">*</span> {{ i18nStore.tl('协议模式', 'Protocol') }}</label>
            <div class="cs-form-input-wrap">
              <el-radio-group v-model="tunnelForm.protocol" @change="onProtocolChange">
                <el-radio-button label="socks5">Socks5</el-radio-button>
                <el-radio-button label="http">HTTP</el-radio-button>
                <el-radio-button label="tcp">TCP</el-radio-button>
                <el-radio-button label="udp">UDP</el-radio-button>
              </el-radio-group>
            </div>
          </div>

          <div class="cs-form-row">
            <label class="cs-form-label">{{ i18nStore.tl('备注', 'Note') }}</label>
            <div class="cs-form-input-wrap">
              <el-input v-model="tunnelForm.note" :placeholder="i18nStore.tl('请输入隧道用途备注', 'Enter note')" />
            </div>
          </div>

          <div class="cs-form-row">
            <label class="cs-form-label"><span class="req">*</span> {{ i18nStore.tl('服务端端口', 'Port') }}</label>
            <div class="cs-form-input-wrap">
              <el-input-number v-model="tunnelForm.serverPort" :min="1" :max="65535" :placeholder="i18nStore.tl('服务端代理端口', 'Port')" />
            </div>
          </div>

          <div v-if="tunnelForm.protocol === 'tcp' || tunnelForm.protocol === 'udp'" class="cs-form-row">
            <label class="cs-form-label"><span class="req">*</span> {{ i18nStore.tl('目标(IP:端口)', 'Target') }}</label>
            <div class="cs-form-input-wrap">
              <el-input v-model="tunnelForm.targetAddress" :placeholder="i18nStore.tl('例如: 192.168.1.100:3306', 'e.g. 192.168.1.100:3306')" />
            </div>
          </div>

          <div v-if="tunnelForm.protocol === 'socks5' || tunnelForm.protocol === 'http'" class="cs-form-row">
            <label class="cs-form-label">{{ i18nStore.tl('用户名', 'Username') }}</label>
            <div class="cs-form-input-wrap">
              <el-input v-model="tunnelForm.username" :placeholder="i18nStore.tl('可选认证用户名', 'Optional username')" />
            </div>
          </div>

          <div v-if="tunnelForm.protocol === 'socks5' || tunnelForm.protocol === 'http'" class="cs-form-row">
            <label class="cs-form-label">{{ i18nStore.tl('密码', 'Password') }}</label>
            <div class="cs-form-input-wrap">
              <el-input v-model="tunnelForm.password" type="password" :placeholder="i18nStore.tl('可选认证密码', 'Optional password')" show-password />
            </div>
          </div>

          <div class="cs-inline-footer">
            <button class="cs-btn cs-btn-primary" :disabled="createLoading" @click="isEditMode ? updateTunnel() : createTunnel()">
              {{ isEditMode ? (i18nStore.tl('保存更新', 'Save')) : (i18nStore.tl('创建隧道', 'Create')) }}
            </button>
            <button class="cs-btn" @click="resetForm">{{ i18nStore.tl('重置', 'Reset') }}</button>
            <button class="cs-btn" @click="handleCloseDialog">{{ i18nStore.tl('取消', 'Cancel') }}</button>
            <button class="cs-btn" @click="handleHelp">{{ i18nStore.tl('帮助', 'Help') }}</button>
          </div>
        </div>
      </div>

      <!-- 隧道列表（卡片：表格铺满 + 分页右下） -->
      <div class="tp-table-card">
        <div class="tp-table-wrap">
          <el-table
            v-loading="loading"
            :data="paginatedTunnelList"
            row-key="id"
            height="100%"
            style="width: 100%"
            table-layout="auto"
            @selection-change="handleSelectionChange"
          >
            <!-- 批量选择列 -->
            <el-table-column v-if="batchSelectMode" type="selection" width="55" :reserve-selection="true">
              <template #header>
                <span>{{ i18nStore.tl('选择', 'Select') }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="id" label="ID" width="80" sortable />
            <el-table-column prop="note" :label="i18nStore.tl('备注', 'Note')" width="212">
              <template #default="{ row }">
                <div class="note-container">
                  <span v-if="!row.isEditingNote" class="note-text">{{ row.note || '-' }}</span>
                  <el-input
                    v-else
                    v-model="row.tempNote"
                    @blur="saveNote(row)"
                    @keyup.enter="saveNote(row)"
                    :placeholder="i18nStore.tl('请输入备注', 'Enter note')"
                  />
                  <button v-if="!row.isEditingNote" class="note-edit-btn" @click="editNote(row)">
                    <el-icon><Edit /></el-icon>
                  </button>
                </div>
              </template>
            </el-table-column>
            <el-table-column prop="protocol" :label="i18nStore.tl('协议', 'Protocol')" width="152">
              <template #default="{ row }">
                <span class="vs-chip">{{ row.protocol }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="serverPort" :label="i18nStore.tl('服务端端口', 'Port')" width="130" />
            <el-table-column prop="targetAddress" :label="i18nStore.tl('目标地址', 'Target')" width="200" show-overflow-tooltip>
              <template #default="{ row }">
                <span v-if="row.targetAddress && (row.protocol === 'tcp' || row.protocol === 'udp')">
                  {{ row.targetAddress }}
                </span>
                <span v-else class="vs-dim">-</span>
              </template>
            </el-table-column>
            <el-table-column prop="username" :label="i18nStore.tl('用户', 'User')" width="152" show-overflow-tooltip>
              <template #default="{ row }">
                <span v-if="row.username && (row.protocol === 'socks5' || row.protocol === 'http')">
                  {{ row.username }}
                </span>
                <span v-else class="vs-dim">-</span>
              </template>
            </el-table-column>
            <el-table-column prop="password" :label="i18nStore.tl('密码', 'Password')" width="132">
              <template #default="{ row }">
                <span v-if="row.password && (row.protocol === 'socks5' || row.protocol === 'http')">
                  {{ '*'.repeat(Math.min(row.password.length, 8)) }}
                </span>
                <span v-else class="vs-dim">-</span>
              </template>
            </el-table-column>
            <el-table-column prop="status" :label="i18nStore.tl('状态', 'Status')" width="112">
              <template #default="{ row }">
                <span
                  class="vs-status-tag"
                  :class="row.status === 'active' ? 'is-online' : (row.status === 'loading' ? 'is-warn' : 'is-offline')"
                >
                  {{ row.status === 'active' ? (i18nStore.tl('活跃', 'Active')) : (row.status === 'loading' ? (i18nStore.tl('启动中', 'Starting')) : (i18nStore.tl('已停止', 'Stopped'))) }}
                </span>
              </template>
            </el-table-column>
            <el-table-column prop="clientStatus" :label="i18nStore.tl('客户端状态', 'Beacon')" width="132">
              <template #default="{ row }">
                <span class="vs-status-tag" :class="row.clientOnline ? 'is-online' : 'is-offline'">
                  {{ row.clientOnline ? (i18nStore.tl('在线', 'Online')) : (i18nStore.tl('离线', 'Offline')) }}
                </span>
              </template>
            </el-table-column>
            <el-table-column prop="createTime" :label="i18nStore.tl('创建时间', 'Created At')" width="182">
              <template #default="{ row }">
                <span class="vs-nowrap">{{ formatTime(row.createTime) }}</span>
              </template>
            </el-table-column>
            <el-table-column :label="i18nStore.tl('操作', 'Actions')" width="300" fixed="right">
              <template #default="{ row }">
                <div class="action-buttons">
                  <button
                    class="cs-btn cs-btn-xs cs-btn-primary"
                    :disabled="row.status === 'active' || row.status === 'loading'"
                    @click="startTunnel(row)"
                  >
                    <span v-if="row.status === 'loading'" class="cs-btn-spin" />
                    {{ i18nStore.tl('启动', 'Start') }}
                  </button>
                  <button
                    class="cs-btn cs-btn-xs"
                    :disabled="row.status !== 'active' || row.status === 'loading'"
                    @click="stopTunnel(row)"
                  >
                    <span v-if="row.status === 'loading'" class="cs-btn-spin" />
                    {{ i18nStore.tl('停止', 'Stop') }}
                  </button>
                  <button class="cs-btn cs-btn-xs" :disabled="row.status === 'loading'" @click="editTunnel(row)">
                    {{ i18nStore.tl('编辑', 'Edit') }}
                  </button>
                  <button
                    class="cs-btn cs-btn-xs cs-btn-danger"
                    :disabled="row.status === 'loading'"
                    @click="deleteTunnel(row)"
                  >
                    {{ i18nStore.tl('删除', 'Remove') }}
                  </button>
                </div>
              </template>
            </el-table-column>

            <template #empty>
              <EmptyState variant="inline" icon="🔀" :title="i18nStore.tl('暂无隧道代理', 'No Tunnel Proxies Configured')" />
            </template>
          </el-table>
        </div>

        <!-- 分页：卡片右下（vxe 观感） -->
        <div class="tp-footer">
          <el-pagination
            v-model:current-page="currentPage"
            v-model:page-size="pageSize"
            :page-sizes="[10, 20, 50, 100]"
            :total="filteredTunnelList.length"
            layout="total, prev, pager, next"
            @size-change="handleSizeChange"
            @current-change="handleCurrentChange"
          />
        </div>
      </div>
    </div>
    </template>
    <Socks5View v-else />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { defineAsyncComponent } from 'vue'
const Socks5View = defineAsyncComponent(() => import('./Socks5View.vue'))
import { useI18nStore } from '@/stores/i18n'
import { Plus, Edit, Refresh, Select, Delete } from '@element-plus/icons-vue'
import ClientAPI from '@/api/clients'
import TunnelAPI, { type TunnelProxyRequest, type TunnelProxyResponse } from '@/api/tunnel'
import PageHeader from '@/components/Common/PageHeader.vue'
import EmptyState from '@/components/Common/EmptyState.vue'

// 类型定义
interface ClientData {
  ID?: number // 数据库自增ID
  id?: number // 兼容小写字段
  Uid: string
  Computer: string
  ExternalIP: string
  InternalIP: string
  Username: string
  Process: string
  Arch: string
  Online: string
  Listener: string
  Note: string
  FirstStart: string
}

interface TunnelItem {
  id: number
  clientUid: string
  clientInfo: string
  clientOnline: boolean
  protocol: string
  serverPort: number | null
  targetAddress: string // TCP/UDP代理的目标地址
  note: string
  username: string
  password: string
  status: string // 支持 'active' | 'inactive' | 'loading'
  createTime: string
  isEditingNote?: boolean
  tempNote?: string
}

interface TunnelForm {
  clientUid: string
  protocol: string
  serverPort: number | null
  targetAddress: string // TCP/UDP代理的目标地址
  note: string
  username: string
  password: string
}

const i18nStore = useI18nStore()
const view = ref<'tunnel' | 'socks5'>('tunnel')
const isZh = computed(() => i18nStore.isZh)

// 数据状态
const loading = ref(false)
const createLoading = ref(false)
const createDialogVisible = ref(false)
const isEditMode = ref(false)
const editingTunnelId = ref<number | null>(null)
const tunnelList = ref<TunnelItem[]>([])
const onlineClients = ref<ClientData[]>([])
const tunnelFormRef = ref()

// 批量选择相关状态
const batchSelectMode = ref(false)
const selectedTunnelIds = ref<number[]>([])

// 分页相关状态
const currentPage = ref(1)
const pageSize = ref(10)

// 表单数据
const tunnelForm = ref<TunnelForm>({
  clientUid: '',
  protocol: 'socks5',
  serverPort: null,
  targetAddress: '',
  note: '',
  username: '',
  password: ''
})

// 表单验证规则
const tunnelRules = computed(() => ({
  clientUid: [
    { required: true, message: isZh.value ? '请选择客户端' : 'Please select a client', trigger: 'change' }
  ],
  protocol: [
    { required: true, message: isZh.value ? '请选择协议类型' : 'Please select a protocol', trigger: 'change' }
  ],
  serverPort: [
    { required: true, message: isZh.value ? '请输入服务端端口' : 'Please enter the server port', trigger: 'blur' }
  ],
  targetAddress: [
    {
      validator: (rule: any, value: any, callback: any) => {
        // 只有TCP和UDP协议才需要验证目标地址
        if (tunnelForm.value.protocol === 'tcp' || tunnelForm.value.protocol === 'udp') {
          if (!value || value.trim() === '') {
            callback(new Error(isZh.value ? '请输入目标地址' : 'Please enter the target address'))
            return
          }
          // 验证格式: IP:端口 或 域名:端口
          const addressRegex = /^[a-zA-Z0-9.-]+:\d+$/
          if (!addressRegex.test(value.trim())) {
            callback(new Error(isZh.value ? '目标地址格式不正确，请使用 IP:端口 或 域名:端口 的格式' : 'Invalid target address; use IP:port or host:port'))
            return
          }
        }
        callback()
      },
      trigger: 'blur'
    }
  ]
}))

// 方法
// 获取在线客户端列表
const fetchOnlineClients = async () => {
  try {
    const response = await ClientAPI.get_clients_list({ page: 1, page_size: 1000 })
    if (response.data && response.data.status === 200) {
      const clientList = response.data.data?.list || []
      // 过滤出在线的客户端并按上线时间排序
      const allClients = clientList.filter((client: ClientData) => 
        client.Online === '1' || String(client.Online) === '1'
      )
      
      // 按上线时间排序（与客户端管理页面保持一致）
      allClients.sort((a: ClientData, b: ClientData) => {
        const timeA = new Date(a.FirstStart).getTime()
        const timeB = new Date(b.FirstStart).getTime()
        return timeA - timeB // 按上线时间升序排列
      })
      
      onlineClients.value = allClients
    }
  } catch (error) {
    console.error('Failed to fetch clients:', error)
    ElMessage.error(isZh.value ? '获取客户端列表失败' : 'Failed to fetch clients')
  }
}

// 获取客户端显示信息
const getClientDisplayInfo = (client: ClientData) => {
  const protocol = client.Listener || 'tcp'
  
  // 使用数据库自增ID而不是UID
  const clientId = client.ID || client.id || 'N/A'
  
  // 获取系统类型（采用客户端管理页面的成功做法）
  const getSystemType = (client: ClientData) => {
    let os = 'unknown'
    let arch = client.Arch || 'unknown'
    
    // 根据Process字段判断操作系统
    if (client.Process && client.Process.includes('.exe')) {
      os = 'windows'
    } else if (client.Computer && client.Computer.toLowerCase().includes('linux')) {
      os = 'linux'
    } else if (client.Computer && client.Computer.toLowerCase().includes('darwin')) {
      os = 'darwin'
    } else {
      // 默认根据架构推断
      os = 'windows' // 大部分客户端是Windows
    }
    
    return `${os}_${arch}`
  }
  
  const osType = getSystemType(client)
  
  // 格式: ID：X | 协议 | 外网IP | 系统类型_架构 | 计算机名
  return `ID：${clientId} | ${protocol.toUpperCase()} | ${client.ExternalIP} | ${osType} | ${client.Computer}`
}

// 获取选中的客户端信息（编辑模式下使用）
const getSelectedClientInfo = () => {
  const selectedClient = onlineClients.value.find(client => client.Uid === tunnelForm.value.clientUid)
  return selectedClient ? getClientDisplayInfo(selectedClient) : (isZh.value ? '客户端信息不可用' : 'Client info unavailable')
}

// 客户端切换事件处理
const onClientChange = (clientUid: string) => {
  // 所有客户端类型现在都支持SOCKS5隧道，不需要特殊处理
}

// 协议切换事件处理
const onProtocolChange = (protocol: string) => {
  // 切换到TCP或UDP协议时，清空用户名和密码
  if (protocol === 'tcp' || protocol === 'udp') {
    tunnelForm.value.username = ''
    tunnelForm.value.password = ''
  }
  // 切换到SOCKS5或HTTP协议时，清空目标地址
  else if (protocol === 'socks5' || protocol === 'http') {
    tunnelForm.value.targetAddress = ''
  }
}

// 搜索与筛选状态（参考客户端管理）
const searchKeyword = ref('')
const protocolFilter = ref('')
const statusFilter = ref('')

const handleSearch = () => { currentPage.value = 1 }

// 过滤后的隧道列表（搜索 + 协议/状态筛选）
const filteredTunnelList = computed(() => {
  let list = tunnelList.value
  const kw = searchKeyword.value.trim().toLowerCase()
  if (kw) {
    list = list.filter(t =>
      String(t.note || '').toLowerCase().includes(kw) ||
      String(t.targetAddress || '').toLowerCase().includes(kw) ||
      String(t.username || '').toLowerCase().includes(kw) ||
      String(t.id || '').includes(kw)
    )
  }
  if (protocolFilter.value) {
    list = list.filter(t => (t.protocol || '') === protocolFilter.value)
  }
  if (statusFilter.value !== '') {
    const wantOn = statusFilter.value === '1'
    list = list.filter(t => {
      const st = String(t.status ?? '')
      return wantOn ? st === '1' || st === 'running' : !(st === '1' || st === 'running')
    })
  }
  return list
})

// 分页相关计算属性
const paginatedTunnelList = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  const end = start + pageSize.value
  return filteredTunnelList.value.slice(start, end)
})

// 分页事件处理
const handleSizeChange = (val: number) => {
  pageSize.value = val
  currentPage.value = 1
}

const handleCurrentChange = (val: number) => {
  currentPage.value = val
}

const showCreateDialog = () => {
  isEditMode.value = false
  editingTunnelId.value = null
  createDialogVisible.value = true
  resetForm()
  fetchOnlineClients() // 获取最新的在线客户端
}

const editTunnel = async (tunnel: TunnelItem) => {
  isEditMode.value = true
  editingTunnelId.value = tunnel.id
  createDialogVisible.value = true
  
  // 预填表单数据
  tunnelForm.value = {
    clientUid: tunnel.clientUid,
    protocol: tunnel.protocol,
    serverPort: tunnel.serverPort,
    targetAddress: tunnel.targetAddress,
    note: tunnel.note,
    username: tunnel.username,
    password: tunnel.password
  }
  
  await fetchOnlineClients() // 获取最新的在线客户端
}

const handleCloseDialog = () => {
  resetForm()
  createDialogVisible.value = false
}

const resetForm = () => {
  tunnelForm.value = {
    clientUid: '',
    protocol: 'socks5',
    serverPort: null,
    targetAddress: '',
    note: '',
    username: '',
    password: ''
  }
  if (tunnelFormRef.value) {
    tunnelFormRef.value.resetFields()
  }
}

// 编辑备注功能
const editNote = (tunnel: TunnelItem) => {
  tunnel.isEditingNote = true
  tunnel.tempNote = tunnel.note
}

const saveNote = async (tunnel: TunnelItem) => {
  if (tunnel.tempNote !== undefined) {
    try {
      // 调用API更新备注 - 传递完整的隧道信息
      const updateData: Partial<TunnelProxyRequest> = {
        client_uid: tunnel.clientUid,
        protocol: tunnel.protocol,
        server_port: tunnel.serverPort!,
        target_address: tunnel.targetAddress,
        note: tunnel.tempNote, // 只更新备注字段
        username: tunnel.username,
        password: tunnel.password
      }
      
      const response = await TunnelAPI.update(tunnel.id, updateData)
      if (response.data && (response.data.message || response.data.status === 200)) {
        tunnel.note = tunnel.tempNote
        ElMessage.success(isZh.value ? '备注更新成功' : 'Note updated')
        // 移除重复的列表刷新调用，避免触发额外的成功提示
      } else {
        throw new Error(response.data?.error || (isZh.value ? '更新备注失败' : 'Failed to update note'))
      }
    } catch (error) {
      console.error('Failed to update note:', error)
      ElMessage.error(error instanceof Error ? error.message : (isZh.value ? '更新备注失败' : 'Failed to update note'))
      // 恢复原值
      tunnel.tempNote = tunnel.note
    }
  }
  tunnel.isEditingNote = false
  delete tunnel.tempNote
}

const validateTunnelForm = () => {
  if (!tunnelForm.value.clientUid) {
    ElMessage.warning(isZh.value ? '请选择客户端' : 'Please select client')
    return false
  }
  if (!tunnelForm.value.serverPort) {
    ElMessage.warning(isZh.value ? '请输入服务端端口' : 'Please enter server port')
    return false
  }
  if (tunnelForm.value.protocol === 'tcp' || tunnelForm.value.protocol === 'udp') {
    if (!tunnelForm.value.targetAddress || !tunnelForm.value.targetAddress.trim()) {
      ElMessage.warning(isZh.value ? '请输入目标地址 (IP:端口)' : 'Please enter target address')
      return false
    }
  }
  return true
}

const createTunnel = async () => {
  if (!validateTunnelForm()) return
  
  try {
    createLoading.value = true
    
    // 获取选中的客户端信息
    const selectedClient = onlineClients.value.find(client => client.Uid === tunnelForm.value.clientUid)
    if (!selectedClient) {
      ElMessage.error(isZh.value ? '选中的客户端不存在或已离线' : 'Selected beacon not found')
      return
    }
    
    // 构建请求数据
    const requestData: TunnelProxyRequest = {
      client_uid: tunnelForm.value.clientUid,
      protocol: tunnelForm.value.protocol,
      server_port: tunnelForm.value.serverPort!,
      target_address: tunnelForm.value.targetAddress,
      note: tunnelForm.value.note,
      username: tunnelForm.value.username,
      password: tunnelForm.value.password
    }
    
    // 调用真实的创建隧道API
    const response = await TunnelAPI.create(requestData)
    if (response.data && response.data.message) {
      ElMessage.success(isZh.value ? '隧道创建成功' : 'Tunnel proxy created')
      createDialogVisible.value = false
      resetForm()
      // 重新获取隧道列表
      await fetchTunnelList()
    } else {
      throw new Error(response.data?.error || (isZh.value ? '创建隧道失败' : 'Failed to create tunnel'))
    }
  } catch (error) {
    console.error('Failed to create tunnel:', error)
    ElMessage.error(error instanceof Error ? error.message : (isZh.value ? '创建隧道失败' : 'Failed to create tunnel'))
  } finally {
    createLoading.value = false
  }
}

const updateTunnel = async () => {
  if (!editingTunnelId.value || !validateTunnelForm()) return
  
  try {
    createLoading.value = true
    
    // 构建请求数据
    const requestData: Partial<TunnelProxyRequest> = {
      client_uid: tunnelForm.value.clientUid,
      protocol: tunnelForm.value.protocol,
      server_port: tunnelForm.value.serverPort!,
      target_address: tunnelForm.value.targetAddress,
      note: tunnelForm.value.note,
      username: tunnelForm.value.username,
      password: tunnelForm.value.password
    }
    
    // 调用更新API
    const response = await TunnelAPI.update(editingTunnelId.value, requestData)
    if (response.data && response.data.message) {
      ElMessage.success(isZh.value ? '隧道更新成功' : 'Tunnel proxy updated')
      createDialogVisible.value = false
      resetForm()
      // 重新获取隧道列表
      await fetchTunnelList()
    } else {
      throw new Error(response.data?.error || (isZh.value ? '更新隧道失败' : 'Failed to update tunnel'))
    }
  } catch (error) {
    console.error('Failed to update tunnel:', error)
    ElMessage.error(error instanceof Error ? error.message : (isZh.value ? '更新隧道失败' : 'Failed to update tunnel'))
  } finally {
    createLoading.value = false
  }
}

const startTunnel = async (tunnel: TunnelItem) => {
  try {
    // 显示加载状态
    tunnel.status = 'loading'
    
    console.log('🚀 Starting tunnel:', tunnel.id, 'client:', tunnel.clientUid)

    // 调用真实的启动隧道API
    const response = await TunnelAPI.start(tunnel.id)
    if (response.data && response.data.message) {
      tunnel.status = 'active'
      ElMessage.success(isZh.value ? '隧道启动成功' : 'Tunnel proxy started')
      console.log('✅ Tunnel started:', tunnel.id)
      // 重新获取隧道列表以更新状态
      await fetchTunnelList()
    } else {
      throw new Error(response.data?.error || (isZh.value ? '启动隧道失败' : 'Failed to start tunnel'))
    }
  } catch (error: any) {
    console.error('❌ Failed to start tunnel:', error)
    tunnel.status = 'inactive'

    // 提供更详细的错误处理
    let errorMessage = isZh.value ? '启动隧道失败' : 'Failed to start tunnel'
    let detailsMessage = ''
    let suggestionMessage = ''

    if (error.response) {
      // HTTP错误响应
      console.log('🔍 HTTP error response:', error.response.status, error.response.data)

      if (error.response.status === 500) {
        errorMessage = isZh.value ? '服务器内部错误' : 'Internal server error'
        suggestionMessage = isZh.value ? '请检查客户端连接状态' : 'Check the beacon connection state'
      } else if (error.response.status === 400) {
        errorMessage = error.response.data?.error || (isZh.value ? '请求参数错误' : 'Invalid request parameters')
        detailsMessage = error.response.data?.details || ''
        suggestionMessage = error.response.data?.suggestion || (isZh.value ? '请检查配置' : 'Check the configuration')
      } else if (error.response.status === 404) {
        errorMessage = isZh.value ? '隧道不存在' : 'Tunnel does not exist'
        suggestionMessage = isZh.value ? '请刷新页面重试' : 'Refresh the page and retry'
      } else {
        errorMessage = error.response.data?.error || (isZh.value ? `HTTP ${error.response.status} 错误` : `HTTP ${error.response.status} error`)
        detailsMessage = error.response.data?.details || ''
        suggestionMessage = error.response.data?.suggestion || ''
      }
    } else if (error.message) {
      errorMessage = error.message
    }

    // 显示详细的错误信息
    let fullErrorMessage = errorMessage
    if (detailsMessage) {
      fullErrorMessage += (isZh.value ? '\n\n详细信息: ' : '\n\nDetails: ') + detailsMessage
    }
    if (suggestionMessage) {
      fullErrorMessage += (isZh.value ? '\n\n建议: ' : '\n\nSuggestion: ') + suggestionMessage
    }
    
    ElMessage({
      type: 'error',
      message: fullErrorMessage,
      duration: 8000, // 延长显示时间
      showClose: true
    })
  }
}

const stopTunnel = async (tunnel: TunnelItem) => {
  try {
    // 显示加载状态
    tunnel.status = 'loading'
    
    // 调用真实的停止隧道API
    const response = await TunnelAPI.stop(tunnel.id)
    if (response.data && response.data.message) {
      tunnel.status = 'inactive'
      ElMessage.success(isZh.value ? '隧道已停止' : 'Tunnel proxy stopped')
      // 重新获取隧道列表以更新状态
      await fetchTunnelList()
    } else {
      throw new Error(response.data?.error || (isZh.value ? '停止隧道失败' : 'Failed to stop tunnel'))
    }
  } catch (error: any) {
    console.error('Failed to stop tunnel:', error)
    tunnel.status = 'active' // 恢复原状态

    // 提供更详细的错误处理
    let errorMessage = isZh.value ? '停止隧道失败' : 'Failed to stop tunnel'
    if (error.response) {
      if (error.response.status === 500) {
        errorMessage = isZh.value ? '服务器内部错误，隧道可能已停止' : 'Internal server error; the tunnel may already be stopped'
      } else if (error.response.status === 404) {
        errorMessage = isZh.value ? '隧道不存在或已停止' : 'Tunnel not found or already stopped'
        tunnel.status = 'inactive'
      } else {
        errorMessage = error.response.data?.error || (isZh.value ? `HTTP ${error.response.status} 错误` : `HTTP ${error.response.status} error`)
      }
    } else if (error.message) {
      errorMessage = error.message
    }
    
    ElMessage.error(errorMessage)
  }
}

const deleteTunnel = async (tunnel: TunnelItem) => {
  try {
    await ElMessageBox.confirm(
      isZh.value ? `确定要删除隧道 ${tunnel.id} 吗？` : `Delete tunnel ${tunnel.id}?`,
      isZh.value ? '删除确认' : 'Delete Confirm',
      {
        confirmButtonText: isZh.value ? '确定' : 'Delete',
        cancelButtonText: isZh.value ? '取消' : 'Cancel',
        type: 'warning'
      }
    )

    // 调用真实的删除隧道API
    const response = await TunnelAPI.delete(tunnel.id)
    if (response.data && response.data.message) {
      ElMessage.success(isZh.value ? '隧道删除成功' : 'Tunnel proxy deleted')
      // 重新获取隧道列表
      await fetchTunnelList()
    } else {
      throw new Error(response.data?.error || (isZh.value ? '删除隧道失败' : 'Failed to delete tunnel'))
    }
  } catch (error: any) {
    if (error === 'cancel') {
      // 用户取消删除
      return
    }

    console.error('Failed to delete tunnel:', error)

    // 提供更详细的错误处理
    let errorMessage = isZh.value ? '删除隧道失败' : 'Failed to delete tunnel'
    if (error.response) {
      if (error.response.status === 500) {
        errorMessage = isZh.value ? '服务器内部错误，请稍后重试' : 'Internal server error; try again later'
      } else if (error.response.status === 404) {
        errorMessage = isZh.value ? '隧道不存在或已被删除' : 'Tunnel not found or already deleted'
        // 隧道不存在，直接刷新列表
        await fetchTunnelList()
        return
      } else {
        errorMessage = error.response.data?.error || (isZh.value ? `HTTP ${error.response.status} 错误` : `HTTP ${error.response.status} error`)
      }
    } else if (error.message) {
      errorMessage = error.message
    }
    
    ElMessage.error(errorMessage)
  }
}

const formatTime = (timeString: string) => {
  if (!timeString) return '-'
  const date = new Date(timeString)
  return date.toLocaleString('zh-CN')
}

const fetchTunnelList = async () => {
  loading.value = true
  try {
    // 调用真实的获取隧道列表API
    const response = await TunnelAPI.list({ page: 1, page_size: 1000 })
    if (response.data && response.data.data) {
      const apiTunnels = response.data.data || []
      
      // 转换为前端需要的格式
      tunnelList.value = apiTunnels.map((tunnel: TunnelProxyResponse) => {
        let clientInfo = tunnel.client_uid
        
        // 如果有客户端信息，格式化显示
        if (tunnel.client_info) {
          const protocol = tunnel.client_info.listener || 'tcp'
          clientInfo = `${tunnel.client_info.computer} (${tunnel.client_info.external_ip}) [${protocol}]`
        }
        
        // 获取客户端在线状态
        let clientOnline = false
        const onlineClientUIDs = onlineClients.value.map(client => client.Uid)
        clientOnline = onlineClientUIDs.includes(tunnel.client_uid)
        
        return {
          id: tunnel.id,
          clientUid: tunnel.client_uid,
          clientInfo,
          clientOnline,
          protocol: tunnel.protocol,
          serverPort: tunnel.server_port,
          targetAddress: tunnel.target_address || '', // 目标地址
          note: tunnel.note || '',
          username: tunnel.username || '',
          password: tunnel.password || '',
          status: tunnel.status === 1 ? 'active' : 'inactive',
          createTime: tunnel.created_at
        }
      })
    } else {
      // 如果没有数据或者返回空数组，不报错，只是设置为空列表
      tunnelList.value = []
      console.log('No tunnel data found; list is empty')
    }
  } catch (error) {
    console.error('Failed to fetch tunnel list:', error)
    // 不显示错误消息，只记录到控制台，设置为空列表
    tunnelList.value = []
  } finally {
    loading.value = false
  }
}

// 刷新隧道列表
const refreshTunnelList = async () => {
  await fetchTunnelList()
  ElMessage.success(isZh.value ? '列表刷新成功' : 'List refreshed')
}

// 启用批量选择模式
const enableBatchSelect = () => {
  batchSelectMode.value = true
  selectedTunnelIds.value = []
}

// 取消批量选择模式
const cancelBatchSelect = () => {
  batchSelectMode.value = false
  selectedTunnelIds.value = []
}

// 处理选择变化
const handleSelectionChange = (selection: TunnelItem[]) => {
  selectedTunnelIds.value = selection.map(item => item.id)
}

// 批量删除隧道
const batchDeleteTunnels = async () => {
  if (selectedTunnelIds.value.length === 0) {
    ElMessage.warning(isZh.value ? '请选择要删除的隧道' : 'Select the tunnels to delete')
    return
  }

  try {
    await ElMessageBox.confirm(
      isZh.value ? `确定要删除选中的 ${selectedTunnelIds.value.length} 个隧道吗？` : `Delete the ${selectedTunnelIds.value.length} selected tunnel(s)?`,
      isZh.value ? '批量删除确认' : 'Batch Delete Confirm',
      {
        confirmButtonText: isZh.value ? '确定' : 'Delete',
        cancelButtonText: isZh.value ? '取消' : 'Cancel',
        type: 'warning'
      }
    )

    // 调用批量删除API（实际是并发删除多个单独请求）
    const results = await TunnelAPI.batchDelete(selectedTunnelIds.value)
    
    // 统计删除结果
    let successCount = 0
    let failedCount = 0
    const failedIds: number[] = []
    
    results.forEach((result, index) => {
      const tunnelId = selectedTunnelIds.value[index]
      if (result.status === 'fulfilled' && result.value?.status === 200) {
        successCount++
      } else {
        failedCount++
        failedIds.push(tunnelId)
        console.error(`Failed to delete tunnel ${tunnelId}:`, result.status === 'rejected' ? result.reason : result.value)
      }
    })

    // 显示删除结果
    if (successCount > 0 && failedCount === 0) {
      ElMessage.success(isZh.value ? `批量删除完成：成功删除 ${successCount} 个隧道` : `Batch delete done: ${successCount} tunnel(s) removed`)
    } else if (successCount > 0 && failedCount > 0) {
      ElMessage.warning(isZh.value ? `批量删除完成：成功 ${successCount} 个，失败 ${failedCount} 个。失败ID：${failedIds.join(', ')}` : `Batch delete done: ${successCount} succeeded, ${failedCount} failed. Failed IDs: ${failedIds.join(', ')}`)
    } else {
      ElMessage.error(isZh.value ? `批量删除失败：所有 ${failedCount} 个隧道删除失败` : `Batch delete failed: all ${failedCount} tunnel(s) could not be removed`)
    }
    
    // 取消批量选择模式并刷新列表
    cancelBatchSelect()
    await fetchTunnelList()
  } catch (error: any) {
    if (error === 'cancel') {
      // 用户取消删除
      return
    }
    
    console.error('Batch delete failed:', error)
    ElMessage.error(error instanceof Error ? error.message : (isZh.value ? '批量删除失败' : 'Batch delete failed'))
  }
}

const batchStartTunnels = () => {
  ElMessage.info('Started selected tunnels')
}

const batchStopTunnels = () => {
  ElMessage.info('Stopped selected tunnels')
}

const handleHelp = () => {
  ElMessage.info('Proxy Pivots (SOCKS & Port Forwarding) Manual')
}

onMounted(() => {
  fetchTunnelList()
  fetchOnlineClients()
})
</script>

<style scoped>
.tp-tabs {
  height: 40px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 0 16px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-card-border);
}
.tp-tab {
  height: 40px;
  display: inline-flex;
  align-items: center;
  padding: 0 12px;
  font-size: 14px;
  color: var(--cs-text);
  cursor: pointer;
  border-bottom: 2px solid transparent;
  margin-bottom: -1px;
}
.tp-tab:hover { color: var(--cs-active); }
.tp-tab.active { color: var(--cs-active); border-bottom-color: var(--cs-active); }

.tunnel-proxy-container {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-bg);
  overflow: hidden;
}

/* ===== 工具条：内容上方一行（左标题/计数，右筛选 + 操作） ===== */
.tp-toolbar {
  min-height: 48px;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  padding: 8px 12px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-card-border);
  flex-shrink: 0;
}

.tp-title-group {
  display: flex;
  align-items: baseline;
  gap: 8px;
}

.tp-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
  white-space: nowrap;
}

.tp-count {
  font-size: 12px;
  color: var(--cs-text-dim);
  white-space: nowrap;
}

.tp-spacer {
  flex: 1;
}

.tp-filter {
  width: 110px;
}

.tp-filter-sm {
  width: 100px;
}

.tp-search {
  width: 220px;
}

.tp-search-icon {
  color: var(--cs-text-dim);
}

.tp-toolbar-divider {
  width: 1px;
  height: 20px;
  flex-shrink: 0;
  background: var(--cs-card-border);
}

/* ===== 内容区：12px 留白，卡片承载表格与表单 ===== */
.tp-content {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 12px;
  overflow: hidden;
}

/* 表格卡片：白底 + 1px 边 + 8px 圆角 + 无阴影 */
.tp-table-card {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  background: var(--cs-panel);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  overflow: hidden;
}

.tp-table-wrap {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

/* 分页：卡片底部右下（vxe 观感） */
.tp-footer {
  flex-shrink: 0;
  height: 52px;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  padding: 0 12px;
  border-top: 1px solid var(--cs-card-border);
}

/* ===== 单元格内容 ===== */
/* 状态：在线实心绿 tag；离线灰 tag；启动中橙 tag（20px / 4px 圆角 / 12px） */
.vs-status-tag {
  display: inline-flex;
  align-items: center;
  height: 20px;
  padding: 0 7px;
  border-radius: 4px;
  font-size: 12px;
  line-height: 1;
  white-space: nowrap;
}

.vs-status-tag.is-online {
  background: var(--cs-online);
  color: #fff;
}

.vs-status-tag.is-warn {
  background: color-mix(in srgb, var(--cs-warning) 22%, transparent);
  color: var(--cs-elevated);
}

.vs-status-tag.is-offline {
  background: color-mix(in srgb, var(--cs-offline) 22%, transparent);
  color: var(--cs-text-dim);
}

/* 中性 chip（协议等非状态标签） */
.vs-chip {
  display: inline-flex;
  align-items: center;
  height: 20px;
  padding: 0 7px;
  border-radius: 4px;
  font-size: 12px;
  line-height: 1;
  background: var(--cs-table-head);
  border: 1px solid var(--cs-card-border);
  color: var(--cs-text);
}

.vs-dim {
  color: var(--cs-text-dim);
}

.vs-nowrap {
  white-space: nowrap;
}

/* 行内操作：24px 紧凑按钮（危险项红字描边） */
.action-buttons {
  display: flex;
  gap: 6px;
  flex-wrap: nowrap;
  align-items: center;
}

.action-buttons .cs-btn {
  padding: 0 8px;
}

.cs-btn-spin {
  width: 10px;
  height: 10px;
  border: 2px solid currentColor;
  border-top-color: transparent;
  border-radius: 50%;
  animation: vs-spin 0.8s linear infinite;
}

@keyframes vs-spin {
  to {
    transform: rotate(360deg);
  }
}

/* 备注单元格：文本 + 铅笔图标按钮 */
.note-container {
  display: flex;
  align-items: center;
  gap: 8px;
}

.note-text {
  flex: 1;
  word-break: break-word;
}

.note-edit-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  padding: 0;
  flex-shrink: 0;
  font-size: 14px;
  color: var(--cs-text-dim);
  background: transparent;
  border: 0;
  border-radius: 6px;
  cursor: pointer;
  transition: color 120ms ease, background 120ms ease;
}

.note-edit-btn:hover {
  color: var(--cs-active);
  background: var(--cs-hover);
}

/* ===== 内联配置面板（vshell 卡片；表单行样式由全局 .cs-form-row 提供） ===== */
.cs-inline-panel {
  flex-shrink: 0;
  background: var(--cs-panel);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  overflow: hidden;
}

.cs-inline-header {
  height: 38px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 12px;
  border-bottom: 1px solid var(--cs-card-border);
}

.cs-inline-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.cs-inline-body {
  padding: 12px 16px;
}

.cs-form-label .req {
  color: var(--cs-error);
}

.cs-inline-footer {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--cs-card-border);
}
</style>
