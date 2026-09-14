<template>
  <div class="socks5-container cs-font-ui">
    <!-- CS Toolbar -->
    <div class="tp-toolbar">
      <div class="tp-title-group">
        <span class="tp-title">{{ isZh ? 'SOCKS5 反向代理' : 'SOCKS5 Proxies' }}</span>
        <span class="tp-count">({{ activeCount }} {{ isZh ? '个活跃' : 'active' }} / {{ socks5List.length }} {{ isZh ? '个总数' : 'total' }})</span>
      </div>

      <div class="tp-spacer" />

      <el-select v-model="statusFilter" :placeholder="isZh ? '状态' : 'Status'" clearable size="small" style="width: 100px">
        <el-option :label="isZh ? '全部' : 'ALL'" value="" />
        <el-option :label="isZh ? '活跃' : 'Active'" value="1" />
        <el-option :label="isZh ? '停止' : 'Stopped'" value="2" />
      </el-select>

      <el-input
        v-model="searchKeyword"
        :placeholder="isZh ? '过滤备注、用户、端口...' : 'Filter note, user, port...'"
        clearable
        size="small"
        class="tp-search"
        @input="handleSearch"
      />

      <button class="cs-btn" @click="refreshList">
        {{ isZh ? '刷新' : 'Refresh' }}
      </button>
    </div>

    <!-- SOCKS5 列表 -->
    <div class="tp-table-wrap">
      <el-table
        v-loading="loading"
        :data="paginatedSocks5List"
        row-key="id"
        style="width: 100%"
        stripe
        table-layout="auto"
        @selection-change="handleSelectionChange"
      >
        <el-table-column type="selection" width="45" :reserve-selection="true" />
        <el-table-column prop="id" label="ID" width="70" sortable />
        <el-table-column prop="note" label="备注" width="200">
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
              <el-button v-if="!row.isEditingNote" @click="editNote(row)" type="text" size="small" class="edit-note-btn">
                <el-icon><Edit /></el-icon>
              </el-button>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="protocol" label="协议" width="110">
          <template #default="{ row }">
            <el-tag size="small">{{ row.protocol.toUpperCase() }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="serverPort" label="服务端端口" width="110">
          <template #default="{ row }">
            <span class="cs-font-mono">{{ row.serverPort }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="agentPort" label="Agent 端口" width="110">
          <template #default="{ row }">
            <span class="cs-font-mono">{{ row.agentPort }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="username" label="用户" width="140" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="row.username" class="cs-font-mono">{{ row.username }}</span>
            <span v-else style="color: #c0c4cc;">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="row.status === 'active' ? 'success' : 'danger'" size="small">
              {{ row.status === 'active' ? (isZh ? '活跃' : 'Active') : (isZh ? '停止' : 'Stopped') }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="clientInfo" label="客户端" width="180" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="cs-font-mono">{{ row.clientInfo }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="createTime" label="创建时间" width="170">
          <template #default="{ row }">
            <span style="white-space: nowrap">{{ formatTime(row.createTime) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="290" fixed="right">
          <template #default="{ row }">
            <div class="action-buttons">
              <el-button
                type="success"
                size="small"
                @click="openProxy(row)"
                :disabled="row.status === 'active'"
              >
                {{ isZh ? '启动' : 'Start' }}
              </el-button>
              <el-button
                type="warning"
                size="small"
                @click="closeProxy(row)"
                :disabled="row.status !== 'active'"
              >
                {{ isZh ? '停止' : 'Stop' }}
              </el-button>
              <el-button type="primary" size="small" @click="editProxy(row)">
                {{ isZh ? '编辑' : 'Edit' }}
              </el-button>
              <el-button size="small" :disabled="row.status !== 'active'" @click="copyCurlCmd(row)">
                {{ isZh ? '复制命令' : 'Copy Cmd' }}
              </el-button>
              <el-button type="danger" size="small" @click="deleteProxy(row)">
                {{ isZh ? '删除' : 'Delete' }}
              </el-button>
            </div>
          </template>
        </el-table-column>

        <template #empty>
          <div class="cs-empty-watermark">
            <div class="empty-title">{{ isZh ? '暂无 SOCKS5 代理' : 'No SOCKS5 Proxies Configured' }}</div>
          </div>
        </template>
      </el-table>
    </div>

    <!-- CS Native Inline Configuration Panel -->
    <div v-if="createDialogVisible" class="cs-inline-panel">
      <div class="cs-inline-header">
        <span class="cs-inline-title">{{ isEditMode ? (isZh ? '编辑 SOCKS5 代理' : 'Edit SOCKS5 Proxy') : (isZh ? '新建 SOCKS5 代理' : 'New SOCKS5 Proxy') }}</span>
        <button class="cs-btn cs-btn-xs" @click="handleCloseDialog">{{ isZh ? '关闭' : 'Close' }}</button>
      </div>

      <div class="cs-inline-body">
        <div class="cs-form-row">
          <label class="cs-form-label"><span class="req">*</span> {{ isZh ? '选择客户端' : 'Beacon' }}</label>
          <div class="cs-form-input-wrap">
            <el-select
              v-model="socks5Form.clientUid"
              :placeholder="isZh ? '请选择已上线的客户端' : 'Select online beacon'"
              size="small"
              style="width: 100%"
              :disabled="isEditMode"
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
          <label class="cs-form-label"><span class="req">*</span> {{ isZh ? '服务端端口' : 'Port' }}</label>
          <div class="cs-form-input-wrap">
            <el-input-number v-model="socks5Form.serverPort" size="small" :min="1" :max="65535" style="width: 180px" />
            <span class="cs-form-hint">{{ isZh ? '（Agent 端口 = 端口 + 1000）' : '（Agent port = port + 1000）' }}</span>
          </div>
        </div>

        <div class="cs-form-row">
          <label class="cs-form-label">{{ isZh ? '备注' : 'Note' }}</label>
          <div class="cs-form-input-wrap">
            <el-input v-model="socks5Form.note" size="small" :placeholder="isZh ? '请输入代理用途备注' : 'Enter note'" />
          </div>
        </div>

        <div class="cs-form-row">
          <label class="cs-form-label">{{ isZh ? '用户名' : 'Username' }}</label>
          <div class="cs-form-input-wrap">
            <el-input v-model="socks5Form.username" size="small" :placeholder="isZh ? '可选认证用户名' : 'Optional username'" />
          </div>
        </div>

        <div class="cs-form-row">
          <label class="cs-form-label">{{ isZh ? '密码' : 'Password' }}</label>
          <div class="cs-form-input-wrap">
            <el-input
              v-model="socks5Form.password"
              type="password"
              size="small"
              :placeholder="isEditMode ? (isZh ? '留空保持不变' : 'Leave empty to keep') : (isZh ? '可选认证密码' : 'Optional password')"
              show-password
            />
          </div>
        </div>

        <div class="cs-inline-footer">
          <button class="cs-btn cs-btn-primary" :disabled="createLoading" @click="isEditMode ? updateProxy() : createProxy()">
            {{ isEditMode ? (isZh ? '保存更新' : 'Save') : (isZh ? '创建代理' : 'Create') }}
          </button>
          <button class="cs-btn" @click="resetForm">{{ isZh ? '重置' : 'Reset' }}</button>
          <button class="cs-btn" @click="handleCloseDialog">{{ isZh ? '取消' : 'Cancel' }}</button>
          <button class="cs-btn" @click="handleHelp">{{ isZh ? '帮助' : 'Help' }}</button>
        </div>
      </div>
    </div>

    <!-- Bottom Action Bar -->
    <div class="tp-bottom-bar cs-font-ui">
      <button class="cs-btn cs-btn-primary" @click="showCreateDialog">
        {{ isZh ? '添加代理' : 'Add Proxy' }}
      </button>
      <button class="cs-btn" :disabled="selectedIds.length === 0" @click="batchOpen">
        {{ isZh ? '启动' : 'Start' }}
      </button>
      <button class="cs-btn" :disabled="selectedIds.length === 0" @click="batchClose">
        {{ isZh ? '停止' : 'Stop' }}
      </button>
      <button class="cs-btn cs-btn-danger" :disabled="selectedIds.length === 0" @click="batchDelete">
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
        :total="filteredList.length"
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
import { Edit } from '@element-plus/icons-vue'
import ClientAPI from '@/api/clients'
import Socks5API, { type Socks5Response } from '@/api/socks5'

interface ClientData {
  ID?: number
  id?: number
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

interface Socks5Item {
  id: number
  clientUid: string
  clientInfo: string
  protocol: string
  serverPort: number
  agentPort: number
  note: string
  username: string
  status: 'active' | 'inactive'
  createTime: string
  isEditingNote?: boolean
  tempNote?: string
}

interface Socks5Form {
  clientUid: string
  serverPort: number | null
  note: string
  username: string
  password: string
}

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const loading = ref(false)
const createLoading = ref(false)
const createDialogVisible = ref(false)
const isEditMode = ref(false)
const editingId = ref<number | null>(null)
const socks5List = ref<Socks5Item[]>([])
const onlineClients = ref<ClientData[]>([])
const selectedIds = ref<number[]>([])

const currentPage = ref(1)
const pageSize = ref(10)

const searchKeyword = ref('')
const statusFilter = ref('')

const socks5Form = ref<Socks5Form>({
  clientUid: '',
  serverPort: null,
  note: '',
  username: '',
  password: ''
})

const activeCount = computed(() => socks5List.value.filter(s => s.status === 'active').length)

const filteredList = computed(() => {
  let list = socks5List.value
  const kw = searchKeyword.value.trim().toLowerCase()
  if (kw) {
    list = list.filter(s =>
      String(s.note || '').toLowerCase().includes(kw) ||
      String(s.username || '').toLowerCase().includes(kw) ||
      String(s.serverPort || '').includes(kw) ||
      String(s.id || '').includes(kw)
    )
  }
  if (statusFilter.value !== '') {
    const wantOn = statusFilter.value === '1'
    list = list.filter(s => (s.status === 'active') === wantOn)
  }
  return list
})

const paginatedSocks5List = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  return filteredList.value.slice(start, start + pageSize.value)
})

const handleSearch = () => { currentPage.value = 1 }
const handleSizeChange = (val: number) => { pageSize.value = val; currentPage.value = 1 }
const handleCurrentChange = (val: number) => { currentPage.value = val }

const handleSelectionChange = (selection: Socks5Item[]) => {
  selectedIds.value = selection.map(item => item.id)
}

// 后端错误统一为 HTTP 200 + {status:400, data:"..."} 包装，统一提取错误文案
function getErrorMsg(res: any, fallback: string): string {
  const body = res?.data || res
  if (body && typeof body === 'object' && body.data && typeof body.data === 'string') return body.data
  if (body?.error) return body.error
  if (body?.message && body.status !== 200) return body.message
  return fallback
}

// 获取在线客户端列表（与隧道代理面板一致）
const fetchOnlineClients = async () => {
  try {
    const response = await ClientAPI.get_clients_list({ page: 1, page_size: 1000 })
    if (response.data && response.data.status === 200) {
      const clientList = response.data.data?.list || []
      const allClients = clientList.filter((client: ClientData) =>
        client.Online === '1' || String(client.Online) === '1'
      )
      allClients.sort((a: ClientData, b: ClientData) => {
        return new Date(a.FirstStart).getTime() - new Date(b.FirstStart).getTime()
      })
      onlineClients.value = allClients
    }
  } catch (error) {
    console.error('获取客户端列表失败:', error)
  }
}

const getClientDisplayInfo = (client: ClientData) => {
  const protocol = client.Listener || 'tcp'
  const clientId = client.ID || client.id || 'N/A'
  let os = 'unknown'
  const arch = client.Arch || 'unknown'
  if (client.Process && client.Process.includes('.exe')) {
    os = 'windows'
  } else if (client.Computer && client.Computer.toLowerCase().includes('linux')) {
    os = 'linux'
  } else if (client.Computer && client.Computer.toLowerCase().includes('darwin')) {
    os = 'darwin'
  } else {
    os = 'windows'
  }
  return `ID：${clientId} | ${protocol.toUpperCase()} | ${client.ExternalIP} | ${os}_${arch} | ${client.Computer}`
}

const showCreateDialog = () => {
  isEditMode.value = false
  editingId.value = null
  createDialogVisible.value = true
  resetForm()
  fetchOnlineClients()
}

const editProxy = (item: Socks5Item) => {
  isEditMode.value = true
  editingId.value = item.id
  createDialogVisible.value = true
  socks5Form.value = {
    clientUid: item.clientUid,
    serverPort: item.serverPort,
    note: item.note,
    username: item.username,
    password: '' // 密码不回显（后端 update 仅非空才更新，留空保持原密码）
  }
  fetchOnlineClients()
}

const handleCloseDialog = () => {
  resetForm()
  createDialogVisible.value = false
}

const resetForm = () => {
  socks5Form.value = {
    clientUid: '',
    serverPort: null,
    note: '',
    username: '',
    password: ''
  }
}

const editNote = (item: Socks5Item) => {
  item.isEditingNote = true
  item.tempNote = item.note
}

const saveNote = async (item: Socks5Item) => {
  if (item.tempNote === undefined) return
  try {
    const response = await Socks5API.update(item.id, { note: item.tempNote })
    if (response.data && response.data.status === 200) {
      item.note = item.tempNote
      ElMessage.success(isZh.value ? '备注更新成功' : 'Note updated')
    } else {
      throw new Error(getErrorMsg(response, isZh.value ? '更新备注失败' : 'Failed to update note'))
    }
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : isZh.value ? '更新备注失败' : 'Failed to update note')
    item.tempNote = item.note
  }
  item.isEditingNote = false
  delete item.tempNote
}

const validateForm = () => {
  if (!socks5Form.value.clientUid) {
    ElMessage.warning(isZh.value ? '请选择客户端' : 'Please select client')
    return false
  }
  if (!socks5Form.value.serverPort || socks5Form.value.serverPort <= 0 || socks5Form.value.serverPort > 65535) {
    ElMessage.warning(isZh.value ? '请输入有效的服务端端口 (1-65535)' : 'Enter a valid server port (1-65535)')
    return false
  }
  return true
}

const createProxy = async () => {
  if (!validateForm()) return
  try {
    createLoading.value = true
    const response = await Socks5API.start({
      client_uid: socks5Form.value.clientUid,
      protocol: 'socks5',
      server_port: socks5Form.value.serverPort!,
      note: socks5Form.value.note,
      username: socks5Form.value.username,
      password: socks5Form.value.password
    })
    if (response.data && response.data.status === 200) {
      ElMessage.success(isZh.value ? 'SOCKS5 代理启动成功' : 'SOCKS5 proxy started')
      createDialogVisible.value = false
      resetForm()
      await fetchList()
    } else {
      ElMessage.error(getErrorMsg(response, isZh.value ? '启动 SOCKS5 代理失败' : 'Failed to start SOCKS5 proxy'))
    }
  } catch (error) {
    console.error('创建 SOCKS5 代理失败:', error)
    ElMessage.error(error instanceof Error ? error.message : isZh.value ? '启动 SOCKS5 代理失败' : 'Failed to start SOCKS5 proxy')
  } finally {
    createLoading.value = false
  }
}

const updateProxy = async () => {
  if (!editingId.value || !validateForm()) return
  try {
    createLoading.value = true
    const response = await Socks5API.update(editingId.value, {
      client_uid: socks5Form.value.clientUid,
      protocol: 'socks5',
      server_port: socks5Form.value.serverPort!,
      note: socks5Form.value.note,
      username: socks5Form.value.username,
      password: socks5Form.value.password
    })
    if (response.data && response.data.status === 200) {
      ElMessage.success(isZh.value ? 'SOCKS5 代理更新成功' : 'SOCKS5 proxy updated')
      createDialogVisible.value = false
      resetForm()
      await fetchList()
    } else {
      ElMessage.error(getErrorMsg(response, isZh.value ? '更新 SOCKS5 代理失败' : 'Failed to update SOCKS5 proxy'))
    }
  } catch (error) {
    console.error('更新 SOCKS5 代理失败:', error)
    ElMessage.error(error instanceof Error ? error.message : isZh.value ? '更新 SOCKS5 代理失败' : 'Failed to update SOCKS5 proxy')
  } finally {
    createLoading.value = false
  }
}

const openProxy = async (item: Socks5Item) => {
  try {
    const response = await Socks5API.open(item.id)
    if (response.data && response.data.status === 200) {
      ElMessage.success(isZh.value ? 'SOCKS5 代理已启动' : 'SOCKS5 proxy started')
    } else {
      ElMessage.error(getErrorMsg(response, isZh.value ? '启动失败' : 'Failed to start'))
    }
    await fetchList()
  } catch (error) {
    console.error('启动 SOCKS5 代理失败:', error)
    ElMessage.error(error instanceof Error ? error.message : isZh.value ? '启动失败' : 'Failed to start')
  }
}

const closeProxy = async (item: Socks5Item) => {
  try {
    const response = await Socks5API.close(item.id)
    if (response.data && response.data.status === 200) {
      ElMessage.success(isZh.value ? 'SOCKS5 代理已停止' : 'SOCKS5 proxy stopped')
    } else {
      ElMessage.error(getErrorMsg(response, isZh.value ? '停止失败' : 'Failed to stop'))
    }
    await fetchList()
  } catch (error) {
    console.error('停止 SOCKS5 代理失败:', error)
    ElMessage.error(error instanceof Error ? error.message : isZh.value ? '停止失败' : 'Failed to stop')
  }
}

const deleteProxy = async (item: Socks5Item) => {
  try {
    await ElMessageBox.confirm(
      isZh.value ? `确定要删除 SOCKS5 代理 ${item.id}（端口 ${item.serverPort}）吗？` : `Delete SOCKS5 proxy ${item.id} (port ${item.serverPort})?`,
      isZh.value ? '删除确认' : 'Delete Confirmation',
      { confirmButtonText: isZh.value ? '确定' : 'Delete', cancelButtonText: isZh.value ? '取消' : 'Cancel', type: 'warning' }
    )
    const response = await Socks5API.delete(item.id)
    if (response.data && response.data.status === 200) {
      ElMessage.success(isZh.value ? 'SOCKS5 代理删除成功' : 'SOCKS5 proxy deleted')
    } else {
      ElMessage.error(getErrorMsg(response, isZh.value ? '删除失败' : 'Failed to delete'))
    }
    await fetchList()
  } catch (error: any) {
    if (error === 'cancel' || error === 'Cancel') return
    console.error('删除 SOCKS5 代理失败:', error)
    ElMessage.error(error instanceof Error ? error.message : isZh.value ? '删除失败' : 'Failed to delete')
  }
}

const batchOpen = async () => {
  for (const id of selectedIds.value) {
    try {
      const response = await Socks5API.open(id)
      if (response.data && response.data.status !== 200) {
        ElMessage.error(getErrorMsg(response, isZh.value ? `启动 #${id} 失败` : `Failed to start #${id}`))
      }
    } catch { /* 单条失败不阻断批量 */ }
  }
  ElMessage.success(isZh.value ? '批量启动请求已下发' : 'Batch start requested')
  await fetchList()
}

const batchClose = async () => {
  for (const id of selectedIds.value) {
    try {
      const response = await Socks5API.close(id)
      if (response.data && response.data.status !== 200) {
        ElMessage.error(getErrorMsg(response, isZh.value ? `停止 #${id} 失败` : `Failed to stop #${id}`))
      }
    } catch { /* 单条失败不阻断批量 */ }
  }
  ElMessage.success(isZh.value ? '批量停止请求已下发' : 'Batch stop requested')
  await fetchList()
}

const batchDelete = async () => {
  try {
    await ElMessageBox.confirm(
      isZh.value ? `确定要删除选中的 ${selectedIds.value.length} 个 SOCKS5 代理吗？` : `Delete ${selectedIds.value.length} selected SOCKS5 proxy(ies)?`,
      isZh.value ? '批量删除确认' : 'Batch Delete Confirmation',
      { confirmButtonText: isZh.value ? '确定' : 'Delete', cancelButtonText: isZh.value ? '取消' : 'Cancel', type: 'warning' }
    )
    const response = await Socks5API.batchDelete(selectedIds.value)
    if (response.data && response.data.status === 200) {
      const msg = response.data.message || (isZh.value ? '批量删除完成' : 'Batch delete done')
      ElMessage.success(String(msg))
    } else {
      ElMessage.error(getErrorMsg(response, isZh.value ? '批量删除失败' : 'Batch delete failed'))
    }
    await fetchList()
  } catch (error: any) {
    if (error === 'cancel' || error === 'Cancel') return
    console.error('批量删除失败:', error)
    ElMessage.error(error instanceof Error ? error.message : isZh.value ? '批量删除失败' : 'Batch delete failed')
  }
}

const formatTime = (timeString: string) => {
  if (!timeString) return '-'
  return timeString
}

const fetchList = async () => {
  loading.value = true
  try {
    const response = await Socks5API.list()
    if (response.data && response.data.status === 200) {
      const list = response.data.data || []
      socks5List.value = list.map((s: Socks5Response) => {
        const onlineClient = onlineClients.value.find(c => c.Uid === s.client_uid)
        const clientInfo = onlineClient
          ? `${onlineClient.Computer} (${onlineClient.ExternalIP})`
          : s.client_uid.slice(0, 8)
        return {
          id: s.id,
          clientUid: s.client_uid,
          clientInfo,
          protocol: s.protocol || 'socks5',
          serverPort: s.server_port,
          agentPort: s.agent_port,
          note: s.note || '',
          username: s.username || '',
          status: s.status === 1 ? 'active' : 'inactive',
          createTime: s.created_at
        }
      })
    } else {
      socks5List.value = []
    }
  } catch (error) {
    console.error('获取 SOCKS5 列表失败:', error)
    socks5List.value = []
  } finally {
    loading.value = false
  }
}

const refreshList = async () => {
  await fetchList()
  ElMessage.success(isZh.value ? '列表刷新成功' : 'List refreshed')
}

const handleHelp = () => {
  ElMessage.info(isZh.value ? 'SOCKS5 反向代理：服务端监听端口 + Agent 端口(端口+1000)，经客户端回连提供内网代理' : 'SOCKS5 reverse proxy: server listens on port, agent on port+1000, reachable via beacon callback')
}

onMounted(() => {
  fetchList()
  fetchOnlineClients()
})
const copyCurlCmd = (row: Socks5Item) => {
  const auth = row.username && row.password ? `${row.username}:${row.password}@` : ''
  const curlCmd = `curl --socks5-hostname ${auth}127.0.0.1:${row.serverPort} http://myip.ipip.net`
  navigator.clipboard.writeText(curlCmd).then(() => {
    ElMessage.success(isZh.value ? `已复制 SOCKS5 测试命令:\n${curlCmd}` : `Copied SOCKS5 test command:\n${curlCmd}`)
  })
}
</script>

<style scoped>
.socks5-container {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-bg);
  overflow: hidden;
}

.tp-toolbar {
  height: 36px;
  background: var(--cs-titlebar-bg);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 12px;
  flex-shrink: 0;
}

.tp-title-group {
  display: flex;
  align-items: center;
  gap: 8px;
}

.tp-title {
  font-size: 17px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.tp-count {
  font-size: 16px;
  color: var(--cs-text-dim);
}

.tp-spacer {
  flex: 1;
}

.tp-search {
  width: 240px;
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

:deep(.el-table) {
  font-size: 16.5px;
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

.action-buttons {
  display: flex;
  gap: 6px;
  flex-wrap: nowrap;
  align-items: center;
}

.action-buttons .el-button {
  margin: 0;
  min-width: 52px;
  flex-shrink: 0;
  padding: 4px 8px;
  font-size: 12px;
  white-space: nowrap;
}

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
  max-width: 640px;
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

.cs-form-hint {
  margin-left: 8px;
  font-size: 11px;
  color: var(--cs-text-dim);
}

.cs-inline-footer {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 8px;
  padding-left: 132px;
}
</style>
