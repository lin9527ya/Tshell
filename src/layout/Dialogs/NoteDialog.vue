<template>
  <el-dialog
    v-model="visible"
    :title="isZh ? '会话备注' : 'Session Note'"
    width="420px"
    :close-on-click-modal="false"
    append-to-body
    class="cs-dialog"
  >
    <div class="cs-dialog-body">
      <div class="dialog-row">
        <label class="dialog-label">{{ isZh ? '目标客户端:' : 'Target:' }}</label>
        <span class="dialog-val">{{ clientLabel }}</span>
      </div>
      <div class="dialog-row">
        <label class="dialog-label">{{ isZh ? '备注信息（支持多行）:' : 'Note (multi-line):' }}</label>
        <el-input
          v-model="noteText"
          type="textarea"
          :rows="6"
          :placeholder="isZh ? '输入该会话的完整笔记内容...' : 'Enter full note for this session...'"
        />
      </div>
    </div>
    <template #footer>
      <div class="cs-dialog-footer">
        <button class="cs-btn" @click="clearNote">{{ isZh ? '清空' : 'Clear' }}</button>
        <div class="footer-spacer" />
        <button class="cs-btn cs-btn-primary" @click="saveNote">{{ isZh ? '保存备注' : 'Set Note' }}</button>
        <button class="cs-btn" @click="close">{{ isZh ? '取消' : 'Cancel' }}</button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useWorkspaceStore } from '@/stores/workspace'
import { useClientsStore } from '@/stores/clients'
import { useI18nStore } from '@/stores/i18n'
import ClientAPI from '@/api/clients'

const workspace = useWorkspaceStore()
const clientsStore = useClientsStore()
const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const visible = computed({
  get: () => workspace.dialogs.note.visible,
  set: (val) => {
    if (!val) workspace.closeDialog('note')
  },
})

const uid = computed(() => workspace.dialogs.note.uid)
const noteText = ref('')

const client = computed(() => clientsStore.getClientByUid(uid.value))
const clientLabel = computed(() => {
  if (!client.value) return uid.value.slice(0, 8)
  return `${client.value.Username || 'user'}@${client.value.Computer || 'host'} (${client.value.ExternalIP || client.value.InternalIP || '?'})`
})

watch(
  () => workspace.dialogs.note.visible,
  (val) => {
    if (val) {
      noteText.value = workspace.dialogs.note.note || client.value?.Note || ''
      loadFullNote()
    }
  },
)

// 拉取完整笔记（Notes 表，独立于客户端 Note 快照）
async function loadFullNote() {
  if (!uid.value) return
  try {
    const res = await ClientAPI.get_note({ uid: uid.value })
    const full = res.data?.data
    if (typeof full === 'string' && full !== '') {
      noteText.value = full
    }
  } catch {
    /* 忽略拉取失败，使用已有快照 */
  }
}

function close() {
  workspace.closeDialog('note')
}

async function saveNote() {
  if (!uid.value) return
  try {
    // 完整笔记写入 Notes 表
    await ClientAPI.save_note({ uid: uid.value, noteContent: noteText.value })
    // 同步客户端列表 Note 快照（单行截断展示）
    const snapshot = noteText.value.split('\n')[0].slice(0, 60)
    await ClientAPI.add_uid_note({ uid: uid.value, note: snapshot })
    clientsStore.updateClient(uid.value, { Note: snapshot })
    ElMessage.success(isZh.value ? '备注已保存' : 'Note updated')
    close()
  } catch {
    ElMessage.error(isZh.value ? '保存失败' : 'Failed to set note')
  }
}

async function clearNote() {
  noteText.value = ''
  await saveNote()
}
</script>

<style scoped>
.cs-dialog-body {
  padding: 10px 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.dialog-row {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.dialog-label {
  font-size: 12px;
  color: var(--cs-text-dim);
  font-weight: 500;
}
.dialog-val {
  font-size: 13px;
  color: var(--cs-text-bright);
  font-family: var(--cs-font-mono);
}
.cs-dialog-footer {
  display: flex;
  align-items: center;
  gap: 8px;
}
.footer-spacer { flex: 1; }
</style>
