<template>
  <div class="credentials-view cs-font-ui" @click="closeContextMenu">
    <!-- Toolbar -->
    <div class="cv-toolbar">
      <div class="cv-title-group">
        <el-icon class="cv-title-icon"><Key /></el-icon>
        <span class="cv-title">{{ isZh ? '密码凭据保险箱' : 'Credentials Vault' }}</span>
        <span class="cv-count">({{ filteredList.length }} {{ isZh ? '条凭证' : 'credentials' }})</span>
      </div>

      <!-- Quick Filter Pills -->
      <div class="cv-filter-pills">
        <button
          class="cv-pill"
          :class="{ active: typeFilter === 'all' }"
          @click="typeFilter = 'all'"
        >
          {{ isZh ? '全部' : 'All' }}
        </button>
        <button
          class="cv-pill"
          :class="{ active: typeFilter === 'pwd' }"
          @click="typeFilter = 'pwd'"
        >
          {{ isZh ? '明文密码' : 'Passwords' }}
        </button>
        <button
          class="cv-pill"
          :class="{ active: typeFilter === 'hash' }"
          @click="typeFilter = 'hash'"
        >
          {{ isZh ? 'NTLM / 哈希' : 'Hashes' }}
        </button>
        <button
          class="cv-pill"
          :class="{ active: typeFilter === 'domain' }"
          @click="typeFilter = 'domain'"
        >
          {{ isZh ? '域账号' : 'Domain' }}
        </button>
      </div>

      <div class="cv-spacer" />

      <el-input
        v-model="searchKey"
        :placeholder="isZh ? '过滤用户、域、主机、哈希、备注...' : 'Filter user, realm, host, hash...'"
        size="small"
        clearable
        class="cv-search-input"
      />

      <button class="cs-btn" @click="refresh">
        <el-icon><Refresh /></el-icon>
        <span>{{ isZh ? '刷新' : 'Refresh' }}</span>
      </button>
    </div>

    <!-- Table -->
    <div class="cv-table-wrap">
      <el-table
        ref="tableRef"
        :data="filteredList"
        row-key="id"
        size="small"
        height="100%"
        highlight-current-row
        class="cs-session-table"
        @current-change="onCurrentChange"
        @selection-change="onSelectionChange"
        @row-contextmenu="onContextMenu"
      >
        <el-table-column type="selection" width="38" align="center" :reserve-selection="true" />

        <el-table-column prop="realm" :label="isZh ? '域 / Realm' : 'Realm / Domain'" width="160" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="cs-font-mono font-bold text-bright">{{ row.realm || '-' }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="user" :label="isZh ? '用户名' : 'User'" width="160" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="user-cell font-bold">{{ row.user || '-' }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="password" :label="isZh ? '明文密码' : 'Password'" min-width="180" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <div v-if="row.password" class="pwd-cell-wrap" @click.stop="copyText(row.password, isZh ? '明文密码' : 'Password')">
              <span class="pwd-cell cs-font-mono">{{ showPasswords ? row.password : '••••••••••••' }}</span>
              <el-icon class="copy-icon" :title="isZh ? '点击复制密码' : 'Copy password'"><CopyDocument /></el-icon>
            </div>
            <span v-else class="text-dim">-</span>
          </template>
        </el-table-column>

        <el-table-column prop="hash" :label="isZh ? 'NTLM / 哈希值' : 'NTLM / Hash'" min-width="240" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <div v-if="row.hash" class="hash-cell-wrap" @click.stop="copyText(row.hash, 'Hash')">
              <span class="hash-cell cs-font-mono">{{ row.hash }}</span>
              <el-icon class="copy-icon" :title="isZh ? '点击复制哈希' : 'Copy hash'"><CopyDocument /></el-icon>
            </div>
            <span v-else class="text-dim">-</span>
          </template>
        </el-table-column>

        <el-table-column prop="host" :label="isZh ? '来源主机' : 'Host'" width="150" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="cs-font-mono">{{ row.host || '-' }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="source" :label="isZh ? '捕获来源' : 'Source'" width="110" align="center" sortable>
          <template #default="{ row }">
            <span :class="['source-badge', `source-${row.source || 'manual'}`]">{{ row.source || 'manual' }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="note" :label="isZh ? '备注 / 标签' : 'Note'" min-width="140" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="text-dim">{{ row.note || '-' }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="updatedAt" :label="isZh ? '更新时间' : 'Updated'" width="170" align="center" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="cs-font-mono text-dim">{{ row.updatedAt || '-' }}</span>
          </template>
        </el-table-column>

        <template #empty>
          <div class="cv-empty-state">
            <div class="empty-icon">🔑</div>
            <div class="empty-title">{{ isZh ? '暂无密码凭据' : 'No Harvested Credentials' }}</div>
            <div class="empty-desc">
              {{ isZh ? '在 Beacon 会话中执行 logonpasswords / hashdump，或点击下方「添加」手动录入凭据。' : 'Credentials from logonpasswords or hashdump will appear here.' }}
            </div>
          </div>
        </template>
      </el-table>
    </div>

    <!-- Bottom Action Bar (CS Native Button Row) -->
    <div class="cv-bottom-bar">
      <button class="cs-btn cs-btn-primary" @click="openAddDialog">
        {{ isZh ? '添加凭证 (Add)' : 'Add' }}
      </button>
      <button class="cs-btn" :disabled="!selectedRow" @click="openEditDialog">
        {{ isZh ? '编辑凭据 (Edit)' : 'Edit' }}
      </button>
      <button class="cs-btn" :disabled="!selectedRow || !selectedRow.password" @click="copySelectedPassword">
        {{ isZh ? '复制密码 (Copy Pwd)' : 'Copy Pwd' }}
      </button>
      <button class="cs-btn" :disabled="!selectedRow || !selectedRow.hash" @click="copySelectedHash">
        {{ isZh ? '复制哈希 (Copy Hash)' : 'Copy Hash' }}
      </button>
      <button class="cs-btn" @click="toggleShowPasswords">
        {{ showPasswords ? (isZh ? '隐藏明文' : 'Hide Passwords') : (isZh ? '显示明文' : 'Show Passwords') }}
      </button>

      <div class="cv-bottom-spacer" />

      <button class="cs-btn" :disabled="!credsStore.credentials.length" @click="exportData">
        {{ isZh ? '导出凭证 (Export)' : 'Export' }}
      </button>
      <button class="cs-btn cs-btn-danger" :disabled="!selectedRow" @click="deleteSelected">
        {{ isZh ? '删除选中 (Delete)' : 'Delete' }}
      </button>
      <button class="cs-btn cs-btn-danger" :disabled="!credsStore.credentials.length" @click="clearAll">
        {{ isZh ? '清空全部 (Clear)' : 'Clear All' }}
      </button>
      <button class="cs-btn" @click="showHelp">
        {{ isZh ? '帮助' : 'Help' }}
      </button>
    </div>

    <!-- Add/Edit Credential Dialog -->
    <el-dialog
      v-model="editDialogVisible"
      :title="isEditing ? (isZh ? '编辑凭据' : 'Edit Credential') : (isZh ? '添加凭据' : 'Add Credential')"
      width="480px"
      append-to-body
      class="cs-dialog"
      :close-on-click-modal="false"
    >
      <div class="cs-dialog-form cs-font-ui">
        <div class="form-field">
          <label class="form-label">{{ isZh ? '所属域 / Realm' : 'Realm / Domain' }}:</label>
          <el-input v-model="formCred.realm" :placeholder="isZh ? '例如 CORP 或 WORKGROUP' : 'e.g. CORP or WORKGROUP'" size="small" />
        </div>
        <div class="form-field">
          <label class="form-label">{{ isZh ? '用户名' : 'Username' }}:</label>
          <el-input v-model="formCred.user" :placeholder="isZh ? '例如 Administrator' : 'e.g. Administrator'" size="small" />
        </div>
        <div class="form-field">
          <label class="form-label">{{ isZh ? '明文密码' : 'Password' }}:</label>
          <el-input v-model="formCred.password" :placeholder="isZh ? '明文密码' : 'Cleartext password'" size="small" />
        </div>
        <div class="form-field">
          <label class="form-label">{{ isZh ? 'NTLM 哈希' : 'NTLM Hash' }}:</label>
          <el-input v-model="formCred.hash" :placeholder="isZh ? 'NTLM 哈希（32 位十六进制）' : 'NTLM hash (32 hex characters)'" size="small" />
        </div>
        <div class="form-field">
          <label class="form-label">{{ isZh ? '来源主机 / IP' : 'Host / IP' }}:</label>
          <el-input v-model="formCred.host" :placeholder="isZh ? '来源主机或 IP' : 'Origin host or IP'" size="small" />
        </div>
        <div class="form-field">
          <label class="form-label">{{ isZh ? '备注 / 标签' : 'Note / Tag' }}:</label>
          <el-input v-model="formCred.note" :placeholder="isZh ? '可选备注' : 'Optional notes'" size="small" />
        </div>
      </div>
      <template #footer>
        <div class="cs-dialog-footer-center">
          <button class="cs-btn cs-btn-primary" @click="saveCredential">{{ isZh ? '保存' : 'Save' }}</button>
          <button class="cs-btn" @click="editDialogVisible = false">{{ isZh ? '取消' : 'Cancel' }}</button>
        </div>
      </template>
    </el-dialog>

    <!-- Context Menu -->
    <Teleport to="body">
      <div
        v-if="ctxMenu.visible"
        class="cs-listener-ctx-menu cs-font-ui"
        :style="{ left: ctxMenu.x + 'px', top: ctxMenu.y + 'px' }"
        @mousedown.stop
        @contextmenu.prevent
      >
        <div class="ctx-item" @click="copySelectedUser">{{ isZh ? '复制用户' : 'Copy User' }}</div>
        <div class="ctx-item is-highlight" @click="copySelectedPassword">{{ isZh ? '复制密码' : 'Copy Password' }}</div>
        <div class="ctx-item" @click="copySelectedHash">{{ isZh ? '复制 NTLM 哈希' : 'Copy NTLM Hash' }}</div>
        <div class="ctx-item" @click="copyUserPass">{{ isZh ? '复制（用户:密码）' : 'Copy (User:Password)' }}</div>
        <div class="ctx-divider" />
        <div class="ctx-item" @click="openEditDialog">{{ isZh ? '编辑凭据...' : 'Edit Credential...' }}</div>
        <div class="ctx-item ctx-danger" @click="deleteSelected">{{ isZh ? '删除凭据' : 'Delete Credential' }}</div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Key, Refresh, CopyDocument } from '@element-plus/icons-vue'
import { useCredentialsStore, type CredentialItem } from '@/stores/credentials'
import { useWorkspaceStore } from '@/stores/workspace'
import { useI18nStore } from '@/stores/i18n'

const credsStore = useCredentialsStore()
const workspace = useWorkspaceStore()
const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const searchKey = ref('')
const typeFilter = ref<'all' | 'pwd' | 'hash' | 'domain'>('all')
const selectedRow = ref<CredentialItem | null>(null)
const selectedRows = ref<CredentialItem[]>([])
const showPasswords = ref(true)

const editDialogVisible = ref(false)
const isEditing = ref(false)
const formCred = reactive({
  id: '',
  realm: '',
  user: '',
  password: '',
  hash: '',
  host: '',
  note: '',
})

// Context Menu
const ctxMenu = ref({ visible: false, x: 0, y: 0, item: null as CredentialItem | null })

const filteredList = computed(() => {
  let list = credsStore.credentials

  if (typeFilter.value === 'pwd') {
    list = list.filter(c => !!c.password)
  } else if (typeFilter.value === 'hash') {
    list = list.filter(c => !!c.hash)
  } else if (typeFilter.value === 'domain') {
    list = list.filter(c => !!c.realm && c.realm !== 'WORKGROUP' && c.realm !== '-')
  }

  if (!searchKey.value.trim()) return list
  const q = searchKey.value.toLowerCase().trim()
  return list.filter((c) => {
    return (
      c.user.toLowerCase().includes(q) ||
      c.realm.toLowerCase().includes(q) ||
      c.host.toLowerCase().includes(q) ||
      c.password.toLowerCase().includes(q) ||
      c.hash.toLowerCase().includes(q) ||
      (c.note && c.note.toLowerCase().includes(q))
    )
  })
})

function onCurrentChange(row: CredentialItem | null) {
  selectedRow.value = row
}

function onSelectionChange(selection: CredentialItem[]) {
  selectedRows.value = selection
  credsStore.setSelectedIds(selection.map((s) => s.id))
}

function toggleShowPasswords() {
  showPasswords.value = !showPasswords.value
}

function copyText(text: string, label: string) {
  if (!text) return
  navigator.clipboard.writeText(text).then(() => {
    ElMessage.success(isZh.value ? `已复制 ${label}` : `Copied ${label}`)
  })
}

function copySelectedUser() {
  if (selectedRow.value?.user) copyText(selectedRow.value.user, isZh.value ? '用户名' : 'Username')
}

function copySelectedPassword() {
  if (selectedRow.value?.password) copyText(selectedRow.value.password, isZh.value ? '密码' : 'Password')
}

function copySelectedHash() {
  if (selectedRow.value?.hash) copyText(selectedRow.value.hash, 'Hash')
}

function copyUserPass() {
  if (selectedRow.value) {
    const pair = `${selectedRow.value.realm ? selectedRow.value.realm + '\\' : ''}${selectedRow.value.user}:${selectedRow.value.password || selectedRow.value.hash}`
    copyText(pair, isZh.value ? '用户凭据对' : 'User:Pass')
  }
}

function openAddDialog() {
  isEditing.value = false
  formCred.id = ''
  formCred.realm = ''
  formCred.user = ''
  formCred.password = ''
  formCred.hash = ''
  formCred.host = ''
  formCred.note = ''
  editDialogVisible.value = true
}

function openEditDialog() {
  if (!selectedRow.value) return
  closeContextMenu()
  isEditing.value = true
  formCred.id = selectedRow.value.id
  formCred.realm = selectedRow.value.realm || ''
  formCred.user = selectedRow.value.user || ''
  formCred.password = selectedRow.value.password || ''
  formCred.hash = selectedRow.value.hash || ''
  formCred.host = selectedRow.value.host || ''
  formCred.note = selectedRow.value.note || ''
  editDialogVisible.value = true
}

function saveCredential() {
  if (!formCred.user.trim() && !formCred.password.trim() && !formCred.hash.trim()) {
    ElMessage.warning(isZh.value ? '用户名、密码或哈希不能全部为空' : 'User, password or hash required')
    return
  }
  const now = new Date().toLocaleString()
  if (isEditing.value && formCred.id) {
    credsStore.updateCredential(formCred.id, {
      realm: formCred.realm.trim(),
      user: formCred.user.trim(),
      password: formCred.password.trim(),
      hash: formCred.hash.trim(),
      host: formCred.host.trim(),
      note: formCred.note.trim(),
      updatedAt: now,
    })
    ElMessage.success(isZh.value ? '凭证已更新' : 'Credential updated')
  } else {
    credsStore.addCredential({
      id: `cred_${Date.now()}_${Math.random().toString(36).slice(2, 6)}`,
      realm: formCred.realm.trim(),
      user: formCred.user.trim(),
      password: formCred.password.trim(),
      hash: formCred.hash.trim(),
      host: formCred.host.trim(),
      source: 'manual',
      note: formCred.note.trim(),
      updatedAt: now,
    })
    ElMessage.success(isZh.value ? '凭证已添加' : 'Credential added')
  }
  editDialogVisible.value = false
}

function deleteSelected() {
  closeContextMenu()
  if (!selectedRow.value) return
  const id = selectedRow.value.id
  ElMessageBox.confirm(
    isZh.value ? '确定删除该凭证记录吗？' : 'Delete this credential?',
    isZh.value ? '确认删除' : 'Confirm Delete',
    {
      confirmButtonText: isZh.value ? '删除' : 'Delete',
      cancelButtonText: isZh.value ? '取消' : 'Cancel',
      type: 'warning',
    },
  ).then(() => {
    credsStore.removeCredential(id)
    selectedRow.value = null
    ElMessage.success(isZh.value ? '已删除凭证' : 'Credential deleted')
  }).catch(() => {})
}

function clearAll() {
  ElMessageBox.confirm(
    isZh.value ? '确定清空凭据保险箱中的所有记录吗？' : 'Clear all credentials?',
    isZh.value ? '确认清空' : 'Confirm Clear',
    {
      confirmButtonText: isZh.value ? '清空' : 'Clear',
      cancelButtonText: isZh.value ? '取消' : 'Cancel',
      type: 'warning',
    },
  ).then(() => {
    credsStore.clearAll()
    selectedRow.value = null
    ElMessage.success(isZh.value ? '已清空凭证库' : 'All credentials cleared')
  }).catch(() => {})
}

function exportData() {
  const jsonStr = JSON.stringify(credsStore.credentials, null, 2)
  const blob = new Blob([jsonStr], { type: 'application/json' })
  const link = document.createElement('a')
  link.href = URL.createObjectURL(blob)
  link.download = `credentials_${Date.now()}.json`
  link.click()
  ElMessage.success(isZh.value ? '凭证已导出为 JSON' : 'Exported to JSON')
}

function refresh() {
  ElMessage.success(isZh.value ? '凭据列表已刷新' : 'Credentials refreshed')
}

function showHelp() {
  ElMessageBox.alert(
    isZh.value
      ? '【密码凭证说明】\n1. 记录哈希转储（hashdump）或 Mimikatz 抓取的凭据信息。\n2. 点击密码或哈希值即可快速复制到剪贴板。\n3. 支持按明文/哈希/域账号快捷筛选与批量导出。'
      : '【Credentials Help】\n1. Stores credentials harvested via Mimikatz or hashdump.\n2. Click any password or hash to copy directly.',
    isZh.value ? '密码凭据使用帮助' : 'Credentials Help',
    { confirmButtonText: isZh.value ? '确定' : 'OK' },
  )
}

function onContextMenu(row: CredentialItem, col: any, e: MouseEvent) {
  e.preventDefault()
  selectedRow.value = row
  ctxMenu.value = { visible: true, x: e.clientX, y: e.clientY, item: row }
}

function closeContextMenu() {
  ctxMenu.value.visible = false
}

onMounted(() => {
  window.addEventListener('click', closeContextMenu)
})

onBeforeUnmount(() => {
  window.removeEventListener('click', closeContextMenu)
})
</script>

<style scoped>
.credentials-view {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-panel);
  overflow: hidden;
}

.cv-toolbar {
  height: 36px;
  background: var(--cs-titlebar-bg);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 12px;
  flex-shrink: 0;
}

.cv-title-group {
  display: flex;
  align-items: center;
  gap: 8px;
}

.cv-title-icon {
  font-size: 18px;
  color: #ffc107;
}

.cv-title {
  font-size: 17px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.cv-count {
  font-size: 16px;
  color: var(--cs-text-dim);
}

.cv-filter-pills {
  display: inline-flex;
  align-items: center;
  background: var(--cs-bg, #181818);
  border: 1px solid var(--cs-border, #333333);
  border-radius: 4px;
  padding: 2px;
  gap: 2px;
  margin-left: 6px;
}

.cv-pill {
  height: 24px;
  padding: 0 10px;
  font-size: 14px;
  color: var(--cs-text-dim);
  background: transparent;
  border: none;
  border-radius: 3px;
  cursor: pointer;
  white-space: nowrap;
}

.cv-pill:hover {
  color: var(--cs-text);
  background: var(--cs-hover);
}

.cv-pill.active {
  color: #ffffff;
  background: #1a5c9a;
  font-weight: 600;
}

.cv-spacer { flex: 1; }

.cv-search-input {
  width: 260px;
}

.cv-table-wrap {
  flex: 1;
  overflow: hidden;
  position: relative;
}

.user-cell {
  color: var(--cs-text-bright);
  font-size: 16.5px;
}

.pwd-cell-wrap, .hash-cell-wrap {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
  padding: 2px 4px;
  border-radius: 3px;
  transition: background 120ms ease;
}

.pwd-cell-wrap:hover, .hash-cell-wrap:hover {
  background: var(--cs-hover);
}

.pwd-cell {
  color: #52c41a;
  font-size: 16px;
  font-weight: bold;
}

.hash-cell {
  color: #40a9ff;
  font-size: 15px;
}

.copy-icon {
  font-size: 14px;
  color: var(--cs-text-dim);
}

.copy-icon:hover {
  color: #ffffff;
}

.source-badge {
  display: inline-block;
  padding: 1px 6px;
  border-radius: 3px;
  font-size: 12px;
  font-weight: 600;
}

.source-mimikatz, .source-logonpasswords {
  background: rgba(245, 34, 45, 0.2);
  color: #ff7875;
}

.source-hashdump {
  background: rgba(250, 140, 22, 0.2);
  color: #ffa940;
}

.source-manual {
  background: rgba(24, 144, 255, 0.15);
  color: #40a9ff;
}

.cv-bottom-bar {
  height: 38px;
  padding: 0 12px;
  display: flex;
  align-items: center;
  gap: 8px;
  background: var(--cs-panel);
  border-top: 1px solid var(--cs-border);
  flex-shrink: 0;
}

.cv-bottom-spacer { flex: 1; }

.cv-empty-state {
  padding: 40px 20px;
  text-align: center;
}

.empty-icon {
  font-size: 36px;
  margin-bottom: 8px;
}

.empty-title {
  font-size: 18px;
  font-weight: 600;
  color: var(--cs-text-bright);
  margin-bottom: 6px;
}

.empty-desc {
  font-size: 16px;
  color: var(--cs-text-dim);
}

.cs-dialog-form {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.form-field {
  display: flex;
  align-items: center;
  gap: 10px;
}

.form-label {
  width: 120px;
  font-size: 15px;
  color: var(--cs-text);
  text-align: right;
  flex-shrink: 0;
}

.cs-dialog-footer-center {
  display: flex;
  justify-content: center;
  gap: 12px;
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
  min-width: 180px;
}

.cs-listener-ctx-menu .ctx-item {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 14px;
  font-size: 16.5px;
  color: var(--cs-text);
  cursor: pointer;
}

.cs-listener-ctx-menu .ctx-item:hover {
  background: var(--cs-hover);
  color: var(--cs-text-bright);
}
</style>
