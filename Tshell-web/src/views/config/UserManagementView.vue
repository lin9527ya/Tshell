<template>
  <div class="cs-user-mgmt-view cs-font-ui">
    <!-- CS Toolbar -->
    <div class="um-toolbar">
      <div class="um-title-group">
        <span class="um-title">{{ isZh ? '用户管理' : 'User Management' }}</span>
        <span class="um-count">({{ total }} {{ isZh ? '个用户' : 'users' }})</span>
      </div>

      <div class="um-spacer" />

      <el-input
        v-model="searchKey"
        :placeholder="isZh ? '过滤用户名...' : 'Filter username...'"
        size="small"
        clearable
        class="um-search"
        @keyup.enter="fetchUsers"
      />

      <button class="cs-btn" :disabled="loading" @click="fetchUsers">{{ isZh ? '刷新' : 'Refresh' }}</button>
      <button class="cs-btn" @click="openChangePasswordDialog">{{ isZh ? '修改我的密码' : 'Change My Password' }}</button>
    </div>

    <!-- Users Table -->
    <div class="um-table-wrap">
      <el-table
        v-loading="loading"
        :data="users"
        size="small"
        height="100%"
        highlight-current-row
        class="cs-session-table"
        @current-change="(r: any) => (selectedRow = r)"
      >
        <el-table-column prop="username" :label="isZh ? '用户名' : 'Username'" min-width="160" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="cs-font-mono font-bold">{{ row.username }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="isZh ? '角色' : 'Role'" width="140" sortable align="center">
          <template #default="{ row }">
            <el-tag size="small" :type="roleTagType(row.role)">{{ roleLabel(row.role) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="isZh ? '锁定' : 'Locked'" width="100" align="center">
          <template #default="{ row }">
            <span :class="row.locked ? 'text-danger' : 'text-ok'">{{ row.locked ? (isZh ? '是' : 'Yes') : (isZh ? '否' : 'No') }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" :label="isZh ? '创建时间' : 'Created At'" width="180" sortable show-overflow-tooltip />
        <el-table-column prop="last_login" :label="isZh ? '最后登录' : 'Last Login'" width="180" sortable show-overflow-tooltip />
        <el-table-column :label="isZh ? '操作' : 'Actions'" width="200" fixed="right" align="center">
          <template #default="{ row }">
            <button class="cs-btn cs-btn-xs" @click="openEditDialog(row)">{{ isZh ? '编辑' : 'Edit' }}</button>
            <button class="cs-btn cs-btn-xs cs-btn-danger" style="margin-left: 4px" @click="handleDelete(row)">{{ isZh ? '删除' : 'Delete' }}</button>
          </template>
        </el-table-column>
      </el-table>

      <div v-if="!users.length && !loading" class="cs-empty-watermark">
        <div class="empty-title">{{ isZh ? '暂无用户' : 'No users found' }}</div>
      </div>
    </div>

    <!-- Bottom Action Bar -->
    <div class="um-bottom-bar">
      <button class="cs-btn cs-btn-primary" @click="openAddDialog">{{ isZh ? '添加用户' : 'Add User' }}</button>
      <button class="cs-btn" :disabled="!selectedRow" @click="selectedRow && openEditDialog(selectedRow)">{{ isZh ? '编辑' : 'Edit' }}</button>
      <button class="cs-btn cs-btn-danger" :disabled="!selectedRow" @click="selectedRow && handleDelete(selectedRow)">{{ isZh ? '删除' : 'Delete' }}</button>
      <button class="cs-btn" @click="handleHelp">{{ isZh ? '帮助' : 'Help' }}</button>
    </div>

    <!-- Add User Dialog -->
    <el-dialog v-model="addDialogVisible" :title="isZh ? '添加用户' : 'Add User'" width="440px" append-to-body class="cs-dialog">
      <div class="cs-form-container">
        <div class="cs-form-row">
          <label class="cs-form-label">{{ isZh ? '用户名' : 'Username' }}:</label>
          <el-input v-model="addForm.username" size="small" :placeholder="isZh ? '新用户名' : 'New operator name'" />
        </div>
        <div class="cs-form-row">
          <label class="cs-form-label">{{ isZh ? '密码' : 'Password' }}:</label>
          <el-input v-model="addForm.password" size="small" type="password" show-password :placeholder="isZh ? '强密码' : 'Strong password'" />
        </div>
        <div class="cs-form-row">
          <label class="cs-form-label">{{ isZh ? '角色' : 'Role' }}:</label>
          <el-select v-model="addForm.role" size="small" style="width: 100%">
            <el-option :label="isZh ? '管理员' : 'admin'" value="admin" />
            <el-option :label="isZh ? '操作员' : 'operator'" value="operator" />
            <el-option :label="isZh ? '审计员' : 'auditor'" value="auditor" />
          </el-select>
        </div>
      </div>
      <template #footer>
        <div class="cs-dialog-footer">
          <button class="cs-btn cs-btn-primary" @click="handleAdd">{{ isZh ? '创建' : 'Create' }}</button>
          <button class="cs-btn" @click="addDialogVisible = false">{{ isZh ? '取消' : 'Cancel' }}</button>
        </div>
      </template>
    </el-dialog>

    <!-- Reset Password Dialog -->
    <el-dialog v-model="resetDialogVisible" :title="isZh ? '编辑用户密码' : 'Edit User Password'" width="440px" append-to-body class="cs-dialog">
      <div class="cs-form-container">
        <div class="cs-form-row">
          <label class="cs-form-label">{{ isZh ? '用户名' : 'Username' }}:</label>
          <el-input :model-value="resetForm.username" size="small" disabled />
        </div>
        <div class="cs-form-row">
          <label class="cs-form-label">{{ isZh ? '新密码' : 'New Password' }}:</label>
          <el-input v-model="resetForm.password" size="small" type="password" show-password :placeholder="isZh ? '新强密码' : 'New strong password'" />
        </div>
      </div>
      <template #footer>
        <div class="cs-dialog-footer">
          <button class="cs-btn cs-btn-primary" @click="handleReset">{{ isZh ? '重置' : 'Reset' }}</button>
          <button class="cs-btn" @click="resetDialogVisible = false">{{ isZh ? '取消' : 'Cancel' }}</button>
        </div>
      </template>
    </el-dialog>
    <!-- Change My Password Dialog -->
    <el-dialog v-model="changePwdDialogVisible" :title="isZh ? '修改我的密码' : 'Change My Password'" width="440px" append-to-body class="cs-dialog">
      <div class="cs-form-container">
        <div class="cs-form-row">
          <label class="cs-form-label">{{ isZh ? '旧密码' : 'Old Password' }}:</label>
          <el-input v-model="changePwdForm.old_password" size="small" type="password" show-password :placeholder="isZh ? '当前登录密码' : 'Current password'" />
        </div>
        <div class="cs-form-row">
          <label class="cs-form-label">{{ isZh ? '新密码' : 'New Password' }}:</label>
          <el-input v-model="changePwdForm.new_password" size="small" type="password" show-password :placeholder="isZh ? '新强密码' : 'New strong password'" />
        </div>
        <div class="cs-form-row">
          <label class="cs-form-label">{{ isZh ? '确认新密码' : 'Confirm' }}:</label>
          <el-input v-model="changePwdForm.confirm" size="small" type="password" show-password :placeholder="isZh ? '再次输入新密码' : 'Repeat new password'" />
        </div>
      </div>
      <template #footer>
        <div class="cs-dialog-footer">
          <button class="cs-btn cs-btn-primary" :disabled="changingPwd" @click="handleChangePassword">
            {{ changingPwd ? (isZh ? '提交中...' : 'Submitting...') : (isZh ? '确认修改' : 'Change') }}
          </button>
          <button class="cs-btn" @click="changePwdDialogVisible = false">{{ isZh ? '取消' : 'Cancel' }}</button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import UserApi from '@/api/user'
import { useI18nStore } from '@/stores/i18n'

interface UserRow {
  username: string
  role: string
  locked: boolean
  created_at: string
  last_login: string
}

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const users = ref<UserRow[]>([])
const total = ref(0)
const loading = ref(false)
const searchKey = ref('')
const selectedRow = ref<UserRow | null>(null)

const addDialogVisible = ref(false)
const resetDialogVisible = ref(false)

const addForm = reactive({
  username: '',
  password: '',
  role: 'operator',
})

const resetForm = reactive({
  username: '',
  password: '',
})

// 修改自己的密码（POST /users/user_setting/ChangePassword，form 绑定）
const changePwdDialogVisible = ref(false)
const changingPwd = ref(false)
const changePwdForm = reactive({
  old_password: '',
  new_password: '',
  confirm: '',
})

function openChangePasswordDialog() {
  changePwdForm.old_password = ''
  changePwdForm.new_password = ''
  changePwdForm.confirm = ''
  changePwdDialogVisible.value = true
}

async function handleChangePassword() {
  if (!changePwdForm.old_password || !changePwdForm.new_password) {
    ElMessage.warning(isZh.value ? '请输入旧密码与新密码' : 'Enter old and new password')
    return
  }
  if (changePwdForm.new_password !== changePwdForm.confirm) {
    ElMessage.warning(isZh.value ? '两次输入的新密码不一致' : 'New passwords do not match')
    return
  }
  changingPwd.value = true
  try {
    const res = await UserApi.rest_password({
      old_password: changePwdForm.old_password,
      new_password: changePwdForm.new_password,
    })
    if (res.data?.code === 200 || res.data?.success) {
      ElMessage.success(isZh.value ? '密码已修改，下次登录生效' : 'Password changed; takes effect on next login')
      changePwdDialogVisible.value = false
    } else {
      ElMessage.error(res.data?.message || (isZh.value ? '修改密码失败' : 'Failed to change password'))
    }
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '修改密码失败' : 'Failed to change password'))
  } finally {
    changingPwd.value = false
  }
}

async function fetchUsers() {
  loading.value = true
  try {
    const res = await UserApi.getUserList({ page: 1, page_size: 100, search: searchKey.value.trim() || undefined })
    if (res.status === 200 && res.data?.status === 200) {
      const data = res.data.data || {}
      users.value = data.list || []
      total.value = data.total || users.value.length
    } else {
      users.value = []
      total.value = 0
    }
  } catch (e: any) {
    users.value = []
    total.value = 0
    ElMessage.error(e?.message || (isZh.value ? '获取用户列表失败' : 'Failed to fetch users'))
  } finally {
    loading.value = false
  }
}

function openAddDialog() {
  addForm.username = ''
  addForm.password = ''
  addForm.role = 'operator'
  addDialogVisible.value = true
}

async function handleAdd() {
  if (!addForm.username.trim() || !addForm.password) {
    ElMessage.warning(isZh.value ? '请输入用户名和密码' : 'Please enter username and password')
    return
  }
  try {
    const res = await UserApi.create_user({
      username: addForm.username.trim(),
      password: addForm.password,
      role: addForm.role,
    })
    if (res.data?.status === 200) {
      ElMessage.success(res.data.message || (isZh.value ? '用户创建成功' : 'User created'))
      addDialogVisible.value = false
      await fetchUsers()
    } else {
      ElMessage.error(res.data?.message || res.data?.data || (isZh.value ? '用户创建失败' : 'Failed to create user'))
    }
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '用户创建失败' : 'Failed to create user'))
  }
}

function openEditDialog(row: UserRow) {
  resetForm.username = row.username
  resetForm.password = ''
  resetDialogVisible.value = true
}

async function handleReset() {
  if (!resetForm.password) {
    ElMessage.warning(isZh.value ? '请输入新密码' : 'Please enter a new password')
    return
  }
  try {
    const res = await UserApi.admin_reset_password({
      username: resetForm.username,
      new_password: resetForm.password,
    })
    if (res.data?.status === 200) {
      ElMessage.success(res.data.message || (isZh.value ? '密码重置成功' : 'Password reset'))
      resetDialogVisible.value = false
    } else {
      ElMessage.error(res.data?.message || res.data?.data || (isZh.value ? '密码重置失败' : 'Failed to reset password'))
    }
  } catch (e: any) {
    ElMessage.error(e?.message || (isZh.value ? '密码重置失败' : 'Failed to reset password'))
  }
}

function handleDelete(row: UserRow) {
  ElMessageBox.confirm(
      isZh.value ? `确定要删除用户 "${row.username}" 吗？其所有会话将被吊销。` : `Delete user "${row.username}"? This will revoke all of their sessions.`,
      isZh.value ? '删除用户' : 'Delete User',
      {
        confirmButtonText: isZh.value ? '删除' : 'Delete',
        cancelButtonText: isZh.value ? '取消' : 'Cancel',
        type: 'warning',
      },
    ).then(async () => {
    try {
      const res = await UserApi.delete_user({ username: row.username })
      if (res.data?.status === 200) {
        ElMessage.success(res.data.message || (isZh.value ? '用户删除成功' : 'User deleted'))
        await fetchUsers()
      } else {
        ElMessage.error(res.data?.message || res.data?.data || (isZh.value ? '用户删除失败' : 'Failed to delete user'))
      }
    } catch (e: any) {
      ElMessage.error(e?.message || (isZh.value ? '用户删除失败' : 'Failed to delete user'))
    }
  }).catch(() => {})
}

function roleLabel(role: string): string {
  if (!isZh.value) return role
  if (role === 'admin') return '管理员'
  if (role === 'operator') return '操作员'
  if (role === 'auditor') return '审计员'
  return role
}

function roleTagType(role: string): 'success' | 'warning' | 'info' | 'primary' | 'danger' {
  if (role === 'admin') return 'danger'
  if (role === 'operator') return 'success'
  if (role === 'auditor') return 'info'
  return 'info'
}

function handleHelp() {
  ElMessage.info(isZh.value ? '用户管理说明：管理员可创建/删除用户并重置密码。' : 'User Management Manual: admin can create/delete users and reset passwords.')
}

onMounted(() => {
  fetchUsers()
})
</script>

<style scoped>
.cs-user-mgmt-view {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-bg);
  overflow: hidden;
}

.um-toolbar {
  height: 32px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
  flex-shrink: 0;
}

.um-title-group {
  display: flex;
  align-items: center;
  gap: 6px;
}

.um-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.um-count {
  font-size: 11px;
  color: var(--cs-text-dim);
}

.um-spacer {
  flex: 1;
}

.um-search {
  width: 200px;
}

.um-table-wrap {
  flex: 1;
  overflow: hidden;
  position: relative;
}

.um-bottom-bar {
  height: 38px;
  background: var(--cs-panel);
  border-top: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 0 12px;
  flex-shrink: 0;
}

.cs-form-container {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.cs-form-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.cs-form-label {
  width: 110px;
  font-size: 12px;
  text-align: right;
  color: var(--cs-text);
  flex-shrink: 0;
}

.cs-dialog-footer {
  display: flex;
  justify-content: center;
  gap: 8px;
}

.cs-empty-watermark {
  position: absolute;
  top: 45%;
  left: 50%;
  transform: translate(-50%, -50%);
  text-align: center;
  pointer-events: none;
}

.empty-title {
  font-size: 13px;
  color: var(--cs-text-dim);
}

.text-danger {
  color: var(--cs-error);
  font-weight: 600;
}

.text-ok {
  color: var(--cs-online);
}

.font-bold {
  font-weight: 600;
}
</style>
