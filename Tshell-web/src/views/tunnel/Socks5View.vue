<template>
  <div class="socks5-container cs-font-ui">
    <!-- vshell 工具条：左（标题 + 计数），右（筛选 + 操作） -->
    <div class="tp-toolbar">
      <div class="tp-title-group">
        <span class="tp-title">{{ isZh ? 'SOCKS5 反向代理' : 'SOCKS5 Proxies' }}</span>
        <span class="tp-count">({{ activeCount }} {{ isZh ? '个活跃' : 'active' }} / {{ socks5List.length }} {{ isZh ? '个总数' : 'total' }})</span>
      </div>

      <div class="tp-spacer" />

      <el-select v-model="statusFilter" :placeholder="isZh ? '状态' : 'Status'" clearable class="tp-filter-sm">
        <el-option :label="isZh ? '全部' : 'ALL'" value="" />
        <el-option :label="isZh ? '活跃' : 'Active'" value="1" />
        <el-option :label="isZh ? '停止' : 'Stopped'" value="2" />
      </el-select>

      <el-input
        v-model="searchKeyword"
        :placeholder="isZh ? '过滤备注、用户、端口...' : 'Filter note, user, port...'"
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

      <button class="cs-btn" @click="refreshList">
        {{ isZh ? '刷新' : 'Refresh' }}
      </button>

      <span class="tp-toolbar-divider" />

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
    </div>

    <div class="tp-content">
      <!-- 内联配置面板（vshell 卡片：新建/编辑代理，展开在表格上方） -->
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
              <el-input-number v-model="socks5Form.serverPort" :min="1" :max="65535" style="width: 180px" />
              <span class="cs-form-hint">{{ isZh ? '（Agent 端口 = 端口 + 1000）' : '（Agent port = port + 1000）' }}</span>
            </div>
          </div>

          <div class="cs-form-row">
            <label class="cs-form-label">{{ isZh ? '备注' : 'Note' }}</label>
            <div class="cs-form-input-wrap">
              <el-input v-model="socks5Form.note" :placeholder="isZh ? '请输入代理用途备注' : 'Enter note'" />
            </div>
          </div>

          <div class="cs-form-row">
            <label class="cs-form-label">{{ isZh ? '用户名' : 'Username' }}</label>
            <div class="cs-form-input-wrap">
              <el-input v-model="socks5Form.username" :placeholder="isZh ? '可选认证用户名' : 'Optional username'" />
            </div>
          </div>

          <div class="cs-form-row">
            <label class="cs-form-label">{{ isZh ? '密码' : 'Password' }}</label>
            <div class="cs-form-input-wrap">
              <el-input
                v-model="socks5Form.password"
                type="password"

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

      <!-- SOCKS5 列表（卡片：表格铺满 + 分页右下） -->
      <div class="tp-table-card">
        <div class="tp-table-wrap">
          <el-table
            v-loading="loading"
            :data="paginatedSocks5List"
            row-key="id"
            height="100%"
            style="width: 100%"
            table-layout="auto"
            @selection-change="handleSelectionChange"
          >
            <el-table-column type="selection" width="45" :reserve-selection="true" />
            <el-table-column prop="id" label="ID" width="70" sortable />
            <el-table-column prop="note" :label="isZh ? '备注' : 'Note'" width="200">
              <template #default="{ row }">
                <div class="note-container">
                  <span v-if="!row.isEditingNote" class="note-text">{{ row.note || '-' }}</span>
                  <el-input
                    v-else
                    v-model="row.tempNote"
                    @blur="saveNote(row)"
                    @keyup.enter="saveNote(row)"
                    :placeholder="isZh ? '请输入备注' : 'Enter note'"
                  />
                  <button v-if="!row.isEditingNote" class="note-edit-btn" @click="editNote(row)">
                    <el-icon><Edit /></el-icon>
                  </button>
                </div>
              </template>
            </el-table-column>
            <el-table-column prop="protocol" :label="isZh ? '协议' : 'Protocol'" width="110">
              <template #default="{ row }">
                <span class="vs-chip">{{ row.protocol.toUpperCase() }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="serverPort" :label="isZh ? '服务端端口' : 'Server Port'" width="110">
              <template #default="{ row }">
                <span class="cs-font-mono">{{ row.serverPort }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="agentPort" :label="isZh ? 'Agent 端口' : 'Agent Port'" width="110">
              <template #default="{ row }">
                <span class="cs-font-mono">{{ row.agentPort }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="username" :label="isZh ? '用户' : 'User'" width="140" show-overflow-tooltip>
              <template #default="{ row }">
                <span v-if="row.username" class="cs-font-mono">{{ row.username }}</span>
                <span v-else class="vs-dim">-</span>
              </template>
            </el-table-column>
            <el-table-column prop="status" :label="isZh ? '状态' : 'Status'" width="90">
              <template #default="{ row }">
                <span class="vs-status-tag" :class="row.status === 'active' ? 'is-online' : 'is-offline'">
                  {{ row.status === 'active' ? (isZh ? '活跃' : 'Active') : (isZh ? '停止' : 'Stopped') }}
                </span>
              </template>
            </el-table-column>
            <el-table-column prop="clientInfo" :label="isZh ? '客户端' : 'Client'" width="180" show-overflow-tooltip>
              <template #default="{ row }">
                <span class="cs-font-mono">{{ row.clientInfo }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="createTime" :label="isZh ? '创建时间' : 'Created'" width="170">
              <template #default="{ row }">
                <span class="vs-nowrap">{{ formatTime(row.createTime) }}</span>
              </template>
            </el-table-column>
            <el-table-column :label="isZh ? '操作' : 'Actions'" width="320" fixed="right">
              <template #default="{ row }">
                <div class="action-buttons">
                  <button
                    class="cs-btn cs-btn-xs cs-btn-primary"
                    :disabled="row.status === 'active'"
                    @click="openProxy(row)"
                  >
                    {{ isZh ? '启动' : 'Start' }}
                  </button>
                  <button class="cs-btn cs-btn-xs" :disabled="row.status !== 'active'" @click="closeProxy(row)">
                    {{ isZh ? '停止' : 'Stop' }}
                  </button>
                  <button class="cs-btn cs-btn-xs" @click="editProxy(row)">
                    {{ isZh ? '编辑' : 'Edit' }}
                  </button>
                  <button class="cs-btn cs-btn-xs" :disabled="row.status !== 'active'" @click="copyCurlCmd(row)">
                    {{ isZh ? '复制命令' : 'Copy Cmd' }}
                  </button>
                  <button class="cs-btn cs-btn-xs cs-btn-danger" @click="deleteProxy(row)">
                    {{ isZh ? '删除' : 'Delete' }}
                  </button>
                </div>
              </template>
            </el-table-column>

            <template #empty>
              <EmptyState variant="inline" icon="🧦" :title="isZh ? '暂无 SOCKS5 代理' : 'No SOCKS5 Proxies Configured'" />
            </template>
          </el-table>
        </div>

        <!-- 分页：卡片右下（vxe 观感） -->
        <div class="tp-footer">
          <el-pagination
            v-model:current-page="currentPage"
            v-model:page-size="pageSize"
            :page-sizes="[10, 20, 50, 100]"
            :total="filteredList.length"
            layout="total, prev, pager, next"
            @size-change="handleSizeChange"
            @current-change="handleCurrentChange"
          />
        </div>
      </div>
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
import EmptyState from '@/components/Common/EmptyState.vue'

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
  password?: string // 代理认证密码(copyCurlCmd 拼接认证串用;后端返回可缺省)
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
    console.error('Failed to load client list:', error)
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
/* 状态：活跃实心绿 tag，停止灰 tag（20px / 4px 圆角 / 12px） */
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
  max-width: 680px;
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
