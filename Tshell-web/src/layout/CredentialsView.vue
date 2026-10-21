<template>
  <div class="credentials-view cs-font-ui">
    <!-- Toolbar -->
    <div class="cv-toolbar">
      <div class="cv-title-group">
        <span class="cv-count">({{ filteredList.length }} {{ i18nStore.tl('条凭证', 'credentials') }})</span>
      </div>

      <!-- Quick Filter Pills -->
      <div class="cv-filter-pills">
        <button
          class="cv-pill"
          :class="{ active: typeFilter === 'all' }"
          @click="typeFilter = 'all'"
        >
          {{ i18nStore.tl('全部', 'All') }}
        </button>
        <button
          class="cv-pill"
          :class="{ active: typeFilter === 'pwd' }"
          @click="typeFilter = 'pwd'"
        >
          {{ i18nStore.tl('明文密码', 'Passwords') }}
        </button>
        <button
          class="cv-pill"
          :class="{ active: typeFilter === 'hash' }"
          @click="typeFilter = 'hash'"
        >
          {{ i18nStore.tl('NTLM / 哈希', 'Hashes') }}
        </button>
        <button
          class="cv-pill"
          :class="{ active: typeFilter === 'domain' }"
          @click="typeFilter = 'domain'"
        >
          {{ i18nStore.tl('域账号', 'Domain') }}
        </button>
      </div>

      <div class="cv-spacer" />

      <button class="cs-btn" @click="toggleShowPasswords">
        {{ showPasswords ? (i18nStore.tl('隐藏明文', 'Hide Passwords')) : (i18nStore.tl('显示明文', 'Show Passwords')) }}
      </button>

      <el-input
        v-model="searchKey"
        :placeholder="i18nStore.tl('过滤用户、域、主机、哈希、备注...', 'Filter user, realm, host, hash...')"
        size="small"
        clearable
        class="cv-search-input"
      />

      <button class="cs-btn cs-btn-primary" @click="openAddDialog">
        {{ i18nStore.tl('添加凭证', 'Add') }}
      </button>
      <button class="cs-btn" :disabled="!credsStore.credentials.length" @click="exportData">
        {{ i18nStore.tl('导出凭证', 'Export') }}
      </button>
      <button class="cs-btn cs-btn-danger" :disabled="!selectedRow" @click="deleteSelected">
        {{ i18nStore.tl('删除选中', 'Delete') }}
      </button>
      <button class="cs-btn cs-btn-danger" :disabled="!credsStore.credentials.length" @click="clearAll">
        {{ i18nStore.tl('清空全部', 'Clear All') }}
      </button>
      <button class="cs-btn" @click="showHelp">
        {{ i18nStore.tl('帮助', 'Help') }}
      </button>
      <button class="cs-btn" @click="refresh">
        <el-icon><Refresh /></el-icon>
        <span>{{ i18nStore.tl('刷新', 'Refresh') }}</span>
      </button>
    </div>

    <!-- Table -->
    <div class="cv-table-wrap">
      <el-table
        ref="tableRef"
        :data="pagedList"
        row-key="id"
        size="small"
        height="100%"
        highlight-current-row
        class="cs-session-table"
        @current-change="onCurrentChange"
        @selection-change="onSelectionChange"
      >
        <el-table-column type="selection" width="38" align="center" :reserve-selection="true" />

        <el-table-column prop="realm" :label="i18nStore.tl('域 / Realm', 'Realm / Domain')" width="160" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="cs-font-mono font-bold text-bright">{{ row.realm || '-' }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="user" :label="i18nStore.tl('用户名', 'User')" width="160" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="user-cell font-bold">{{ row.user || '-' }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="password" :label="i18nStore.tl('明文密码', 'Password')" min-width="180" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <div v-if="row.password" class="pwd-cell-wrap" @click.stop="copyText(row.password, i18nStore.tl('明文密码', 'Password'))">
              <span class="pwd-cell cs-font-mono">{{ showPasswords ? row.password : '••••••••••••' }}</span>
              <el-icon class="copy-icon" :title="i18nStore.tl('点击复制密码', 'Copy password')"><CopyDocument /></el-icon>
            </div>
            <span v-else class="text-dim">-</span>
          </template>
        </el-table-column>

        <el-table-column prop="hash" :label="i18nStore.tl('NTLM / 哈希值', 'NTLM / Hash')" min-width="240" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <div v-if="row.hash" class="hash-cell-wrap" @click.stop="copyText(row.hash, 'Hash')">
              <span class="hash-cell cs-font-mono">{{ row.hash }}</span>
              <el-icon class="copy-icon" :title="i18nStore.tl('点击复制哈希', 'Copy hash')"><CopyDocument /></el-icon>
            </div>
            <span v-else class="text-dim">-</span>
          </template>
        </el-table-column>

        <el-table-column prop="host" :label="i18nStore.tl('来源主机', 'Host')" width="150" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="cs-font-mono">{{ row.host || '-' }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="source" :label="i18nStore.tl('捕获来源', 'Source')" width="110" align="center" sortable>
          <template #default="{ row }">
            <span :class="['source-badge', `source-${row.source || 'manual'}`]">{{ row.source || 'manual' }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="note" :label="i18nStore.tl('备注 / 标签', 'Note')" min-width="140" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="text-dim">{{ row.note || '-' }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="updatedAt" :label="i18nStore.tl('更新时间', 'Updated')" width="170" align="center" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="cs-font-mono text-dim">{{ row.updatedAt || '-' }}</span>
          </template>
        </el-table-column>

        <el-table-column :label="i18nStore.tl('操作', 'Actions')" width="220" fixed="right">
          <template #default="{ row }">
            <div class="row-actions">
              <a class="row-link" @click.stop="editRow(row)">{{ i18nStore.tl('编辑', 'Edit') }}</a>
              <a v-if="row.password" class="row-link" @click.stop="copyRowPassword(row)">{{ i18nStore.tl('复制密码', 'Copy Pwd') }}</a>
              <a v-if="row.hash" class="row-link" @click.stop="copyRowHash(row)">{{ i18nStore.tl('复制哈希', 'Copy Hash') }}</a>
              <a class="row-link row-link-danger" @click.stop="deleteRow(row)">{{ i18nStore.tl('删除', 'Delete') }}</a>
            </div>
          </template>
        </el-table-column>

        <template #empty>
          <EmptyState
            variant="inline"
            icon="🔑"
            :title="i18nStore.tl('暂无密码凭据', 'No Harvested Credentials')"
            :desc="i18nStore.tl('在 Beacon 会话中执行 logonpasswords / hashdump，或点击下方「添加」手动录入凭据。', 'Credentials from logonpasswords or hashdump will appear here.')"
          />
        </template>
      </el-table>
    </div>

    <!-- 底部分页（前端分页） -->
    <div class="cs-table-pager">
      <el-pagination
        small
        background
        layout="prev, pager, next, sizes, total"
        :total="filteredList.length"
        :current-page="page"
        :page-size="pageSize"
        :page-sizes="[20, 50, 100]"
        @current-change="onPageChange"
        @size-change="onSizeChange"
      />
    </div>

    <!-- Add/Edit Credential Dialog -->
    <el-dialog
      v-model="editDialogVisible"
      :title="isEditing ? (i18nStore.tl('编辑凭据', 'Edit Credential')) : (i18nStore.tl('添加凭据', 'Add Credential'))"
      width="480px"
      append-to-body
      class="cs-dialog"
      :close-on-click-modal="false"
    >
      <div class="cs-dialog-form cs-font-ui">
        <div class="form-field">
          <label class="form-label">{{ i18nStore.tl('所属域 / Realm', 'Realm / Domain') }}:</label>
          <el-input v-model="formCred.realm" :placeholder="i18nStore.tl('例如 CORP 或 WORKGROUP', 'e.g. CORP or WORKGROUP')" size="small" />
        </div>
        <div class="form-field">
          <label class="form-label">{{ i18nStore.tl('用户名', 'Username') }}:</label>
          <el-input v-model="formCred.user" :placeholder="i18nStore.tl('例如 Administrator', 'e.g. Administrator')" size="small" />
        </div>
        <div class="form-field">
          <label class="form-label">{{ i18nStore.tl('明文密码', 'Password') }}:</label>
          <el-input v-model="formCred.password" :placeholder="i18nStore.tl('明文密码', 'Cleartext password')" size="small" />
        </div>
        <div class="form-field">
          <label class="form-label">{{ i18nStore.tl('NTLM 哈希', 'NTLM Hash') }}:</label>
          <el-input v-model="formCred.hash" :placeholder="i18nStore.tl('NTLM 哈希（32 位十六进制）', 'NTLM hash (32 hex characters)')" size="small" />
        </div>
        <div class="form-field">
          <label class="form-label">{{ i18nStore.tl('来源主机 / IP', 'Host / IP') }}:</label>
          <el-input v-model="formCred.host" :placeholder="i18nStore.tl('来源主机或 IP', 'Origin host or IP')" size="small" />
        </div>
        <div class="form-field">
          <label class="form-label">{{ i18nStore.tl('备注 / 标签', 'Note / Tag') }}:</label>
          <el-input v-model="formCred.note" :placeholder="i18nStore.tl('可选备注', 'Optional notes')" size="small" />
        </div>
      </div>
      <template #footer>
        <div class="cs-dialog-footer-center">
          <button class="cs-btn cs-btn-primary" @click="saveCredential">{{ i18nStore.tl('保存', 'Save') }}</button>
          <button class="cs-btn" @click="editDialogVisible = false">{{ i18nStore.tl('取消', 'Cancel') }}</button>
        </div>
      </template>
    </el-dialog>

    </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh, CopyDocument } from '@element-plus/icons-vue'
import { useCredentialsStore, type CredentialItem } from '@/stores/credentials'
import { useI18nStore } from '@/stores/i18n'
import { useSessionSelectionStore } from '@/stores/sessionSelection'
import EmptyState from '@/components/Common/EmptyState.vue'

const credsStore = useCredentialsStore()
const workspace = useSessionSelectionStore()
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

// ---- 分页（前端分页）----
const page = ref(1)
const pageSize = ref(50)
const pagedList = computed(() => {
  const maxPage = Math.max(1, Math.ceil(filteredList.value.length / pageSize.value))
  const p = Math.min(page.value, maxPage)
  const start = (p - 1) * pageSize.value
  return filteredList.value.slice(start, start + pageSize.value)
})
function onPageChange(p: number) { page.value = p }
function onSizeChange(s: number) { pageSize.value = s; page.value = 1 }
watch([searchKey, typeFilter], () => { page.value = 1 })

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

function copySelectedPassword() {
  if (selectedRow.value?.password) copyText(selectedRow.value.password, isZh.value ? '密码' : 'Password')
}

function copySelectedHash() {
  if (selectedRow.value?.hash) copyText(selectedRow.value.hash, 'Hash')
}

// 行内操作（vshell 范式：文字链接）
function editRow(row: CredentialItem) {
  selectedRow.value = row
  openEditDialog()
}

function copyRowPassword(row: CredentialItem) {
  if (row.password) copyText(row.password, isZh.value ? '密码' : 'Password')
}

function copyRowHash(row: CredentialItem) {
  if (row.hash) copyText(row.hash, 'Hash')
}

function deleteRow(row: CredentialItem) {
  selectedRow.value = row
  deleteSelected()
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
    // 新增: 同步写服务端 loot 库(持久化+审计可追溯)
    credsStore.addCredentialPersistent({
      realm: formCred.realm.trim(),
      user: formCred.user.trim(),
      password: formCred.password.trim(),
      hash: formCred.hash.trim(),
      host: formCred.host.trim(),
      source: 'manual',
      note: formCred.note.trim(),
    })
    ElMessage.success(isZh.value ? '凭证已添加' : 'Credential added')
  }
  editDialogVisible.value = false
}

function deleteSelected() {
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
    credsStore.removeCredentialPersistent(id)
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

onMounted(() => {
  // 从服务端 loot 库拉取凭据(持久层;失败静默保持本地内存态)
  credsStore.fetchFromServer()
})

onBeforeUnmount(() => {
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

/* ===== 工具条：白底 + 1px 分隔线，左标题/筛选，右搜索/操作 ===== */
.cv-toolbar {
  flex-shrink: 0;
  min-height: 48px;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  padding: 6px 16px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-card-border);
}

.cv-title-group {
  display: flex;
  align-items: center;
  gap: 8px;
}

.cv-title-icon {
  font-size: 16px;
  color: var(--cs-warning);
}

.cv-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--cs-text-bright);
  white-space: nowrap;
}

.cv-count {
  font-size: 13px;
  color: var(--cs-text-dim);
  white-space: nowrap;
}

.cv-filter-pills {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  margin-left: 6px;
  padding: 2px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-border-strong);
  border-radius: 6px;
}

.cv-pill {
  height: 26px;
  padding: 0 10px;
  font-family: var(--cs-font-ui);
  font-size: 13px;
  color: var(--cs-text-dim);
  background: transparent;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  white-space: nowrap;
  transition: background 120ms ease, color 120ms ease;
}

.cv-pill:hover {
  color: var(--cs-text);
  background: var(--cs-hover);
}

.cv-pill.active {
  color: #ffffff;
  background: var(--cs-active);
}

.cv-spacer {
  flex: 1;
}

.cv-search-input {
  width: 240px;
}

.cv-table-wrap {
  flex: 1;
  overflow: hidden;
  position: relative;
}

/* ===== 单元格渲染 ===== */
.user-cell {
  color: var(--cs-text);
}

.pwd-cell-wrap,
.hash-cell-wrap {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 6px;
  border-radius: 6px;
  cursor: pointer;
  transition: background 120ms ease;
}

.pwd-cell-wrap:hover,
.hash-cell-wrap:hover {
  background: var(--cs-hover);
}

.pwd-cell {
  font-weight: 600;
  color: var(--cs-online);
}

.hash-cell {
  color: var(--cs-active);
}

.copy-icon {
  font-size: 14px;
  color: var(--cs-text-dim);
}

.copy-icon:hover {
  color: var(--cs-active);
}

.source-badge {
  display: inline-flex;
  align-items: center;
  height: 20px;
  padding: 0 7px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 600;
}

.source-mimikatz,
.source-logonpasswords {
  background: color-mix(in srgb, var(--cs-error) 14%, transparent);
  color: var(--cs-error);
}

.source-hashdump {
  background: color-mix(in srgb, var(--cs-warning) 18%, transparent);
  color: color-mix(in srgb, var(--cs-warning) 75%, var(--cs-text));
}

.source-manual {
  background: color-mix(in srgb, var(--cs-active) 12%, transparent);
  color: var(--cs-active);
}

/* ===== 对话框 ===== */
.cs-dialog-form {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.form-field {
  display: flex;
  align-items: center;
  gap: 12px;
}

.form-label {
  width: 120px;
  flex-shrink: 0;
  font-size: 14px;
  color: var(--cs-text);
  text-align: right;
}

.cs-dialog-footer-center {
  display: flex;
  justify-content: center;
  gap: 12px;
}

/* ===== 右键菜单 ===== */
.cs-listener-ctx-menu {
  position: fixed;
  min-width: 180px;
  padding: 4px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  box-shadow: 0 3px 6px -4px rgba(0, 0, 0, 0.12), 0 6px 16px 0 rgba(0, 0, 0, 0.08);
  z-index: 9999;
}

.cs-listener-ctx-menu .ctx-item {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 32px;
  padding: 0 10px;
  font-size: 14px;
  color: var(--cs-text);
  border-radius: 6px;
  cursor: pointer;
}

.cs-listener-ctx-menu .ctx-item:hover {
  background: var(--cs-hover);
  color: var(--cs-active);
}

.cs-listener-ctx-menu .ctx-item.is-highlight {
  font-weight: 600;
  color: var(--cs-active);
}

.cs-listener-ctx-menu .ctx-item.ctx-danger {
  color: var(--cs-error);
}

.cs-listener-ctx-menu .ctx-item.ctx-danger:hover {
  background: color-mix(in srgb, var(--cs-error) 12%, transparent);
  color: var(--cs-error);
}

.cs-listener-ctx-menu .ctx-divider {
  height: 1px;
  margin: 4px 0;
  background: var(--cs-card-border);
}

.cs-table-pager {
  flex-shrink: 0;
  display: flex;
  justify-content: flex-end;
  padding: 8px 16px;
  background: var(--cs-panel);
  border-top: 1px solid var(--cs-card-border);
}

.row-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.row-link {
  color: var(--cs-active);
  font-size: 14px;
  cursor: pointer;
  white-space: nowrap;
}

.row-link:hover {
  text-decoration: underline;
}

.row-link-danger {
  color: var(--cs-error, #f56c6c);
}
</style>
