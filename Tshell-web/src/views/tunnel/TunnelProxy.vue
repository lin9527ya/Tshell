<template>
  <div class="tunnel-proxy-container cs-font-ui">
    <!-- CS Toolbar -->
    <div class="tp-toolbar">
      <div class="tp-title-group">
        <span class="tp-title">{{ isZh ? '隧道代理' : 'Proxy Pivots' }}</span>
        <span class="tp-count">({{ filteredTunnelList.filter(t => t.status === 'active').length }} {{ isZh ? '个活跃' : 'active' }} / {{ filteredTunnelList.length }} {{ isZh ? '个总数' : 'total' }})</span>
      </div>

      <div class="tp-spacer" />

      <el-select v-model="protocolFilter" :placeholder="isZh ? '协议' : 'Protocol'" clearable size="small" style="width: 100px">
        <el-option :label="isZh ? '全部' : 'ALL'" value="" />
        <el-option label="TCP" value="tcp" />
        <el-option label="UDP" value="udp" />
        <el-option label="HTTP" value="http" />
        <el-option label="SOCKS5" value="socks5" />
      </el-select>

      <el-select v-model="statusFilter" :placeholder="isZh ? '状态' : 'Status'" clearable size="small" style="width: 90px">
        <el-option :label="isZh ? '全部' : 'ALL'" value="" />
        <el-option :label="isZh ? '活跃' : 'Active'" value="1" />
        <el-option :label="isZh ? '停止' : 'Stopped'" value="0" />
      </el-select>

      <el-input
        v-model="searchKeyword"
        :placeholder="isZh ? '过滤备注、地址、用户...' : 'Filter note, address, user...'"
        clearable
        size="small"
        class="tp-search"
        @input="handleSearch"
      />

      <button class="cs-btn" @click="refreshTunnelList">
        {{ isZh ? '刷新' : 'Refresh' }}
      </button>
    </div>

    <!-- 隧道列表 -->
    <div class="tp-table-wrap">

      <el-table
        v-loading="loading"
        :data="paginatedTunnelList"
        row-key="id"
        style="width: 100%"
        stripe
        table-layout="auto"
        @selection-change="handleSelectionChange"
      >
        <!-- 批量选择列 -->
        <el-table-column v-if="batchSelectMode" type="selection" width="55" :reserve-selection="true">
          <template #header>
            <span>选择</span>
          </template>
        </el-table-column>
        <el-table-column prop="id" label="ID" width="80" sortable />
        <el-table-column prop="note" label="备注" width="212">
          <template #default="{ row }">
            <div class="note-container">
              <span v-if="!row.isEditingNote" class="note-text">{{ row.note || '-' }}</span>
              <el-input
                v-else
                v-model="row.tempNote"
                @blur="saveNote(row)"
                @keyup.enter="saveNote(row)"
                size="small"
                placeholder="请输入备注"
              />
              <el-button
                v-if="!row.isEditingNote"
                @click="editNote(row)"
                type="text"
                size="small"
                class="edit-note-btn"
              >
                <el-icon><Edit /></el-icon>
              </el-button>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="protocol" label="协议" width="152">
          <template #default="{ row }">
            <el-tag size="small">{{ row.protocol }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="serverPort" label="服务端端口" width="130" />
        <el-table-column prop="targetAddress" label="目标地址" width="200" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="row.targetAddress && (row.protocol === 'tcp' || row.protocol === 'udp')">
              {{ row.targetAddress }}
            </span>
            <span v-else style="color: #c0c4cc;">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="username" label="用户" width="152" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="row.username && (row.protocol === 'socks5' || row.protocol === 'http')">
              {{ row.username }}
            </span>
            <span v-else style="color: #c0c4cc;">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="password" label="密码" width="132">
          <template #default="{ row }">
            <span v-if="row.password && (row.protocol === 'socks5' || row.protocol === 'http')">
              {{ '*'.repeat(Math.min(row.password.length, 8)) }}
            </span>
            <span v-else style="color: #c0c4cc;">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="112">
          <template #default="{ row }">
            <el-tag 
              :type="row.status === 'active' ? 'success' : (row.status === 'loading' ? 'warning' : 'danger')" 
              size="small"
            >
              {{ row.status === 'active' ? '活跃' : (row.status === 'loading' ? '启动中' : '已停止') }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="clientStatus" label="客户端状态" width="132">
          <template #default="{ row }">
            <el-tag :type="row.clientOnline ? 'success' : 'danger'" size="small">
              {{ row.clientOnline ? '在线' : '离线' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="createTime" label="创建时间" width="182">
          <template #default="{ row }">
            <span style="white-space: nowrap">{{ formatTime(row.createTime) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="300" fixed="right">
          <template #default="{ row }">
            <div class="action-buttons">
              <el-button
                type="success"
                size="small"
                @click="startTunnel(row)"
                :disabled="row.status === 'active' || row.status === 'loading'"
                :loading="row.status === 'loading'"
              >
                启动
              </el-button>
              <el-button
                type="warning"
                size="small"
                @click="stopTunnel(row)"
                :disabled="row.status !== 'active' || row.status === 'loading'"
                :loading="row.status === 'loading'"
              >
                停止
              </el-button>
              <el-button
                type="primary"
                size="small"
                @click="editTunnel(row)"
                :disabled="row.status === 'loading'"
              >
                编辑
              </el-button>
              <el-button
                type="danger"
                size="small"
                @click="deleteTunnel(row)"
                :disabled="row.status === 'loading'"
              >
                删除
              </el-button>
            </div>
          </template>
        </el-table-column>

        <template #empty>
          <div class="cs-empty-watermark">
            <div class="empty-title">{{ isZh ? '暂无隧道代理' : 'No Tunnel Proxies Configured' }}</div>
          </div>
        </template>
      </el-table>
    </div>

    <!-- CS Native Inline Tunnel Configuration Panel (No Popup Dialog) -->
    <div v-if="createDialogVisible" class="cs-inline-panel">
      <div class="cs-inline-header">
        <span class="cs-inline-title">{{ isEditMode ? (isZh ? '编辑隧道代理' : 'Edit Proxy Pivot') : (isZh ? '新建隧道代理' : 'New Proxy Pivot') }}</span>
        <button class="cs-btn cs-btn-xs" @click="handleCloseDialog">{{ isZh ? '关闭' : 'Close' }}</button>
      </div>

      <div class="cs-inline-body">
        <div class="cs-form-row">
          <label class="cs-form-label"><span class="req">*</span> {{ isZh ? '选择客户端' : 'Beacon' }}</label>
          <div class="cs-form-input-wrap">
            <el-select 
              v-model="tunnelForm.clientUid" 
              :placeholder="isZh ? '请选择已上线的客户端' : 'Select online beacon'" 
              size="small"
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
          <label class="cs-form-label"><span class="req">*</span> {{ isZh ? '协议模式' : 'Protocol' }}</label>
          <div class="cs-form-input-wrap">
            <el-radio-group v-model="tunnelForm.protocol" size="small" @change="onProtocolChange">
              <el-radio-button label="socks5">Socks5</el-radio-button>
              <el-radio-button label="http">HTTP</el-radio-button>
              <el-radio-button label="tcp">TCP</el-radio-button>
              <el-radio-button label="udp">UDP</el-radio-button>
            </el-radio-group>
          </div>
        </div>

        <div class="cs-form-row">
          <label class="cs-form-label">{{ isZh ? '备注' : 'Note' }}</label>
          <div class="cs-form-input-wrap">
            <el-input v-model="tunnelForm.note" size="small" :placeholder="isZh ? '请输入隧道用途备注' : 'Enter note'" />
          </div>
        </div>

        <div class="cs-form-row">
          <label class="cs-form-label"><span class="req">*</span> {{ isZh ? '服务端端口' : 'Port' }}</label>
          <div class="cs-form-input-wrap">
            <el-input-number v-model="tunnelForm.serverPort" size="small" :min="1" :max="65535" :placeholder="isZh ? '服务端代理端口' : 'Port'" />
          </div>
        </div>

        <div v-if="tunnelForm.protocol === 'tcp' || tunnelForm.protocol === 'udp'" class="cs-form-row">
          <label class="cs-form-label"><span class="req">*</span> {{ isZh ? '目标(IP:端口)' : 'Target' }}</label>
          <div class="cs-form-input-wrap">
            <el-input v-model="tunnelForm.targetAddress" size="small" :placeholder="isZh ? '例如: 192.168.1.100:3306' : 'e.g. 192.168.1.100:3306'" />
          </div>
        </div>

        <div v-if="tunnelForm.protocol === 'socks5' || tunnelForm.protocol === 'http'" class="cs-form-row">
          <label class="cs-form-label">{{ isZh ? '用户名' : 'Username' }}</label>
          <div class="cs-form-input-wrap">
            <el-input v-model="tunnelForm.username" size="small" :placeholder="isZh ? '可选认证用户名' : 'Optional username'" />
          </div>
        </div>

        <div v-if="tunnelForm.protocol === 'socks5' || tunnelForm.protocol === 'http'" class="cs-form-row">
          <label class="cs-form-label">{{ isZh ? '密码' : 'Password' }}</label>
          <div class="cs-form-input-wrap">
            <el-input v-model="tunnelForm.password" type="password" size="small" :placeholder="isZh ? '可选认证密码' : 'Optional password'" show-password />
          </div>
        </div>

        <div class="cs-inline-footer">
          <button class="cs-btn cs-btn-primary" :disabled="createLoading" @click="isEditMode ? updateTunnel() : createTunnel()">
            {{ isEditMode ? (isZh ? '保存更新' : 'Save') : (isZh ? '创建隧道' : 'Create') }}
          </button>
          <button class="cs-btn" @click="resetForm">{{ isZh ? '重置' : 'Reset' }}</button>
          <button class="cs-btn" @click="handleCloseDialog">{{ isZh ? '取消' : 'Cancel' }}</button>
          <button class="cs-btn" @click="handleHelp">{{ isZh ? '帮助' : 'Help' }}</button>
        </div>
      </div>
    </div>

    <!-- Bottom Action Bar (CS Native Button Row) -->
    <div class="tp-bottom-bar cs-font-ui">
      <button class="cs-btn cs-btn-primary" @click="showCreateDialog">
        {{ isZh ? '添加隧道' : 'Add Tunnel' }}
      </button>
      <button class="cs-btn" :disabled="selectedTunnelIds.length === 0" @click="batchStartTunnels">
        {{ isZh ? '启动' : 'Start' }}
      </button>
      <button class="cs-btn" :disabled="selectedTunnelIds.length === 0" @click="batchStopTunnels">
        {{ isZh ? '停止' : 'Stop' }}
      </button>
      <button class="cs-btn cs-btn-danger" :disabled="selectedTunnelIds.length === 0" @click="batchDeleteTunnels">
        {{ isZh ? '删除' : 'Remove' }}
      </button>
      <button class="cs-btn" @click="handleHelp">
        {{ isZh ? '帮助' : 'Help' }}
      </button>

      <div class="tp-spacer" />

      <el-pagination
        v-model:current-page="currentPage"
        v-model:page-size="pageSize"
        :page-sizes="[10, 20, 50, 100]"
        :total="filteredTunnelList.length"
        layout="total, prev, pager, next"
        :small="true"
        @size-change="handleSizeChange"
        @current-change="handleCurrentChange"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18nStore } from '@/stores/i18n'
import { Plus, Edit, Refresh, Select, Delete } from '@element-plus/icons-vue'
import ClientAPI from '@/api/clients'
import TunnelAPI, { type TunnelProxyRequest, type TunnelProxyResponse } from '@/api/tunnel'
import PageHeader from '@/components/Common/PageHeader.vue'

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
const tunnelRules = {
  clientUid: [
    { required: true, message: '请选择客户端', trigger: 'change' }
  ],
  protocol: [
    { required: true, message: '请选择协议类型', trigger: 'change' }
  ],
  serverPort: [
    { required: true, message: '请输入服务端端口', trigger: 'blur' }
  ],
  targetAddress: [
    { 
      validator: (rule: any, value: any, callback: any) => {
        // 只有TCP和UDP协议才需要验证目标地址
        if (tunnelForm.value.protocol === 'tcp' || tunnelForm.value.protocol === 'udp') {
          if (!value || value.trim() === '') {
            callback(new Error('请输入目标地址'))
            return
          }
          // 验证格式: IP:端口 或 域名:端口
          const addressRegex = /^[a-zA-Z0-9.-]+:\d+$/
          if (!addressRegex.test(value.trim())) {
            callback(new Error('目标地址格式不正确，请使用 IP:端口 或 域名:端口 的格式'))
            return
          }
        }
        callback()
      }, 
      trigger: 'blur' 
    }
  ]
}

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
    console.error('获取客户端列表失败:', error)
    ElMessage.error('获取客户端列表失败')
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
  return selectedClient ? getClientDisplayInfo(selectedClient) : '客户端信息不可用'
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
        ElMessage.success('备注更新成功')
        // 移除重复的列表刷新调用，避免触发额外的成功提示
      } else {
        throw new Error(response.data?.error || '更新备注失败')
      }
    } catch (error) {
      console.error('更新备注失败:', error)
      ElMessage.error(error instanceof Error ? error.message : '更新备注失败')
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
    console.error('创建隧道失败:', error)
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
    console.error('更新隧道失败:', error)
    ElMessage.error(error instanceof Error ? error.message : (isZh.value ? '更新隧道失败' : 'Failed to update tunnel'))
  } finally {
    createLoading.value = false
  }
}

const startTunnel = async (tunnel: TunnelItem) => {
  try {
    // 显示加载状态
    tunnel.status = 'loading'
    
    console.log('🚀 尝试启动隧道:', tunnel.id, '客户端:', tunnel.clientUid)
    
    // 调用真实的启动隧道API
    const response = await TunnelAPI.start(tunnel.id)
    if (response.data && response.data.message) {
      tunnel.status = 'active'
      ElMessage.success('隧道启动成功')
      console.log('✅ 隧道启动成功:', tunnel.id)
      // 重新获取隧道列表以更新状态
      await fetchTunnelList()
    } else {
      throw new Error(response.data?.error || '启动隧道失败')
    }
  } catch (error: any) {
    console.error('❌ 启动隧道失败:', error)
    tunnel.status = 'inactive'
    
    // 提供更详细的错误处理
    let errorMessage = '启动隧道失败'
    let detailsMessage = ''
    let suggestionMessage = ''
    
    if (error.response) {
      // HTTP错误响应
      console.log('🔍 HTTP 错误响应:', error.response.status, error.response.data)
      
      if (error.response.status === 500) {
        errorMessage = '服务器内部错误'
        suggestionMessage = '请检查客户端连接状态'
      } else if (error.response.status === 400) {
        errorMessage = error.response.data?.error || '请求参数错误'
        detailsMessage = error.response.data?.details || ''
        suggestionMessage = error.response.data?.suggestion || '请检查配置'
      } else if (error.response.status === 404) {
        errorMessage = '隧道不存在'
        suggestionMessage = '请刷新页面重试'
      } else {
        errorMessage = error.response.data?.error || `HTTP ${error.response.status} 错误`
        detailsMessage = error.response.data?.details || ''
        suggestionMessage = error.response.data?.suggestion || ''
      }
    } else if (error.message) {
      errorMessage = error.message
    }
    
    // 显示详细的错误信息
    let fullErrorMessage = errorMessage
    if (detailsMessage) {
      fullErrorMessage += '\n\n详细信息: ' + detailsMessage
    }
    if (suggestionMessage) {
      fullErrorMessage += '\n\n建议: ' + suggestionMessage
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
      ElMessage.success('隧道已停止')
      // 重新获取隧道列表以更新状态
      await fetchTunnelList()
    } else {
      throw new Error(response.data?.error || '停止隧道失败')
    }
  } catch (error: any) {
    console.error('停止隧道失败:', error)
    tunnel.status = 'active' // 恢复原状态
    
    // 提供更详细的错误处理
    let errorMessage = '停止隧道失败'
    if (error.response) {
      if (error.response.status === 500) {
        errorMessage = '服务器内部错误，隧道可能已停止'
      } else if (error.response.status === 404) {
        errorMessage = '隧道不存在或已停止'
        tunnel.status = 'inactive'
      } else {
        errorMessage = error.response.data?.error || `HTTP ${error.response.status} 错误`
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
      `确定要删除隧道 ${tunnel.id} 吗？`,
      '删除确认',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
    
    // 调用真实的删除隧道API
    const response = await TunnelAPI.delete(tunnel.id)
    if (response.data && response.data.message) {
      ElMessage.success('隧道删除成功')
      // 重新获取隧道列表
      await fetchTunnelList()
    } else {
      throw new Error(response.data?.error || '删除隧道失败')
    }
  } catch (error: any) {
    if (error === 'cancel') {
      // 用户取消删除
      return
    }
    
    console.error('删除隧道失败:', error)
    
    // 提供更详细的错误处理
    let errorMessage = '删除隧道失败'
    if (error.response) {
      if (error.response.status === 500) {
        errorMessage = '服务器内部错误，请稍后重试'
      } else if (error.response.status === 404) {
        errorMessage = '隧道不存在或已被删除'
        // 隧道不存在，直接刷新列表
        await fetchTunnelList()
        return
      } else {
        errorMessage = error.response.data?.error || `HTTP ${error.response.status} 错误`
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
      console.log('没有找到隧道数据，列表为空')
    }
  } catch (error) {
    console.error('获取隧道列表失败:', error)
    // 不显示错误消息，只记录到控制台，设置为空列表
    tunnelList.value = []
  } finally {
    loading.value = false
  }
}

// 刷新隧道列表
const refreshTunnelList = async () => {
  await fetchTunnelList()
  ElMessage.success('列表刷新成功')
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
    ElMessage.warning('请选择要删除的隧道')
    return
  }

  try {
    await ElMessageBox.confirm(
      `确定要删除选中的 ${selectedTunnelIds.value.length} 个隧道吗？`,
      '批量删除确认',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
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
        console.error(`删除隧道 ${tunnelId} 失败:`, result.status === 'rejected' ? result.reason : result.value)
      }
    })
    
    // 显示删除结果
    if (successCount > 0 && failedCount === 0) {
      ElMessage.success(`批量删除完成：成功删除 ${successCount} 个隧道`)
    } else if (successCount > 0 && failedCount > 0) {
      ElMessage.warning(`批量删除完成：成功 ${successCount} 个，失败 ${failedCount} 个。失败ID：${failedIds.join(', ')}`)
    } else {
      ElMessage.error(`批量删除失败：所有 ${failedCount} 个隧道删除失败`)
    }
    
    // 取消批量选择模式并刷新列表
    cancelBatchSelect()
    await fetchTunnelList()
  } catch (error: any) {
    if (error === 'cancel') {
      // 用户取消删除
      return
    }
    
    console.error('批量删除失败:', error)
    ElMessage.error(error instanceof Error ? error.message : '批量删除失败')
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
.tunnel-proxy-container {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-bg);
  overflow: hidden;
}

.tp-toolbar {
  height: 32px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
  flex-shrink: 0;
}

.tp-title-group {
  display: flex;
  align-items: center;
  gap: 6px;
}

.tp-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.tp-count {
  font-size: 11px;
  color: var(--cs-text-dim);
}

.tp-spacer {
  flex: 1;
}

.tp-search {
  width: 200px;
}

.tp-table-wrap {
  flex: 1;
  overflow: hidden;
}

.tp-bottom-bar {
  height: 38px;
  background: var(--cs-panel);
  border-top: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 12px;
  flex-shrink: 0;
}

/* 表格紧凑密度（对齐客户端管理） */
:deep(.el-table) {
  font-size: 13px;
  width: 100% !important;
  background-color: transparent !important;
}

:deep(.el-table__body-wrapper) {
  overflow-x: auto;
}

:deep(.el-table .cell) {
  word-break: break-word;
}

:deep(.el-table th) {
  background-color: var(--table-header-bg, #f6f8fa) !important;
  color: var(--text-primary, #1f2328) !important;
  font-weight: 600;
  padding: 6px 8px !important;
}

:deep(.el-table td) {
  padding: 6px 8px !important;
}

:deep(.el-table .el-table__row:hover > td) {
  background-color: var(--bg-hover, #f0f3f6) !important;
}

:deep(.el-form-item__label) {
  font-weight: 500;
}

/* ===== 顶部工具栏（批量选择 + 搜索筛选合并为一条紧凑横条） ===== */
.tunnel-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  background: var(--bg-secondary, #f6f8fa);
  border-bottom: 1px solid var(--border-color, #e5e5e5);
  border-radius: 10px 10px 0 0;
}

.tunnel-search-input {
  width: 240px;
}

.tunnel-filter-select {
  width: 130px;
}

.toolbar-spacer {
  flex: 1;
}

/* ===== 批量选择 ===== */
.batch-buttons {
  display: flex;
  gap: 8px;
  align-items: center;
}

/* ===== 操作按钮（紧凑） ===== */
.action-buttons {
  display: flex;
  gap: 6px;
  flex-wrap: nowrap;
  align-items: center;
  justify-content: flex-start;
}

.action-buttons .el-button {
  margin: 0;
  min-width: 52px;
  flex-shrink: 0;
  padding: 4px 8px;
  font-size: 12px;
  white-space: nowrap;
}

/* ===== 分页（右对齐紧凑） ===== */
.pagination-container {
  display: flex;
  justify-content: flex-end;
  padding: 8px 12px;
  border-top: 1px solid var(--border-color, #e5e5e5);
}

/* ===== 协议选择 ===== */
.protocol-options {
  width: 100%;
}

.protocol-options .el-radio-button {
  flex: 1;
}

.protocol-options .el-radio-button__inner {
  width: 100%;
  border-radius: 4px;
}

.protocol-options .el-radio-button:first-child .el-radio-button__inner {
  border-top-left-radius: 4px;
  border-bottom-left-radius: 4px;
}

.protocol-options .el-radio-button:last-child .el-radio-button__inner {
  border-top-right-radius: 4px;
  border-bottom-right-radius: 4px;
}

.protocol-description {
  margin-top: 8px;
}

.protocol-description .el-alert {
  border: 1px solid #67c23a;
  background: var(--table-header-bg, #f6f8fa);
}

/* 内联协议选择（对话框内四个选项一行，紧凑） */
.protocol-options-inline {
  display: flex;
  width: 100%;
  gap: 0;
}

.protocol-options-inline .el-radio-button {
  flex: 1;
}

.protocol-options-inline .el-radio-button__inner {
  width: 100%;
  text-align: center;
  border-radius: 0;
  padding: 8px 4px;
  font-size: 13px;
}

.protocol-options-inline .el-radio-button:first-child .el-radio-button__inner {
  border-top-left-radius: 4px;
  border-bottom-left-radius: 4px;
}

.protocol-options-inline .el-radio-button:last-child .el-radio-button__inner {
  border-top-right-radius: 4px;
  border-bottom-right-radius: 4px;
}

/* 对话框表单紧凑 */
:deep(.el-dialog__body) {
  padding: 16px 20px;
}

:deep(.el-dialog__body .el-form-item) {
  margin-bottom: 12px;
}

/* ===== 备注编辑 ===== */
.note-container {
  display: flex;
  align-items: center;
  gap: 8px;
}

.note-text {
  flex: 1;
  word-break: break-word;
}

.edit-note-btn {
  padding: 4px;
  margin-left: 4px;
  min-width: auto;
}

/* CS Native Inline Form Panel */
.cs-inline-panel {
  background: var(--cs-panel);
  border-top: 2px solid var(--cs-active);
  border-bottom: 1px solid var(--cs-border);
  padding: 10px 14px;
}

.cs-inline-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;
  padding-bottom: 6px;
  border-bottom: 1px solid var(--cs-border);
}

.cs-inline-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.cs-inline-body {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-width: 600px;
}

.cs-form-row {
  display: flex;
  align-items: center;
  gap: 12px;
}

.cs-form-label {
  width: 120px;
  text-align: right;
  font-size: 12px;
  color: var(--cs-text-dim);
  flex-shrink: 0;
}

.cs-form-label .req {
  color: #f56c6c;
}

.cs-form-input-wrap {
  flex: 1;
}

.cs-inline-footer {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 8px;
  padding-left: 132px;
}
</style>

