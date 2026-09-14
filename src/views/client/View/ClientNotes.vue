<template>
  <div class="cs-notes-view cs-font-ui">
    <!-- CS Toolbar -->
    <div class="cn-toolbar">
      <div class="cn-title-group">
        <span class="cn-title">{{ isZh ? '会话备忘录 / 笔记' : 'Session Notes' }}</span>
        <span class="cn-hint">{{ isZh ? '为当前 Beacon 会话记录渗透笔记、凭据信息或执行备忘' : 'Keep operational notes and credentials for this Beacon session' }}</span>
      </div>
      <div class="cn-spacer" />
      <button class="cs-btn" :disabled="loading" @click="fetchNote">{{ isZh ? '刷新' : 'Refresh' }}</button>
    </div>

    <!-- Note Editor Body -->
    <div class="cn-body">
      <textarea
        v-model="noteContent"
        class="cn-textarea cs-font-mono"
        :placeholder="isZh ? '在此输入关于该 Beacon 会话的备忘笔记...' : 'Type notes for this session here...'"
        spellcheck="false"
      />
    </div>

    <!-- Bottom Action Bar -->
    <div class="cn-bottom-bar">
      <button class="cs-btn cs-btn-primary" :disabled="loading" @click="saveNote">
        {{ loading ? (isZh ? '保存中...' : 'Saving...') : (isZh ? '保存笔记' : 'Save Note') }}
      </button>
      <button class="cs-btn" @click="fetchNote">
        {{ isZh ? '重载' : 'Reload' }}
      </button>
      <button class="cs-btn cs-btn-danger" :disabled="!noteContent" @click="noteContent = ''">
        {{ isZh ? '清空' : 'Clear' }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, inject, computed } from 'vue'
import { ElMessage } from 'element-plus'
import ClientAPI from '@/api/clients'
import { useRoute } from 'vue-router'
import { useClientUid } from '@/utils/clientUtils'
import { useI18nStore } from '@/stores/i18n'

const route = useRoute()
const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const injectedUid = inject('realUid', null) as any
const getUid = () => {
  const v = injectedUid && injectedUid.value ? String(injectedUid.value) : ''
  if (v && v.length > 10) return v
  const mapped = useClientUid(route)
  return typeof mapped === 'string' && mapped.length > 10 ? mapped : ''
}

const noteContent = ref('')
const loading = ref(false)

const fetchNote = async () => {
  try {
    const uid = getUid()
    if (!uid) {
      ElMessage.warning(isZh.value ? '客户端未就绪' : 'Beacon session not ready')
      return
    }
    const response = await ClientAPI.get_note({ uid })
    noteContent.value = response.data.data || ''
  } catch (error) {
    ElMessage.error(isZh.value ? '加载笔记失败' : 'Failed to load notes')
  }
}

const saveNote = async () => {
  loading.value = true
  try {
    const uid = getUid()
    if (!uid) {
      ElMessage.warning(isZh.value ? '客户端未就绪' : 'Beacon session not ready')
      return
    }
    const res = await ClientAPI.save_note({ uid, noteContent: noteContent.value })
    if (res.data.status === 200) {
      ElMessage.success(isZh.value ? '笔记已保存' : 'Notes saved successfully')
    }
  } catch (error) {
    ElMessage.error(isZh.value ? '保存笔记失败' : 'Failed to save notes')
  } finally {
    loading.value = false
  }
}

onMounted(fetchNote)
</script>

<style scoped>
.cs-notes-view {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-panel);
  overflow: hidden;
}

.cn-toolbar {
  height: 32px;
  background: var(--cs-titlebar-bg);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  padding: 0 12px;
  flex-shrink: 0;
}

.cn-title {
  font-size: 12px;
  font-weight: bold;
  color: var(--cs-text-bright);
}

.cn-hint {
  font-size: 11px;
  color: var(--cs-text-dim);
  margin-left: 12px;
}

.cn-spacer {
  flex: 1;
}

.cn-body {
  flex: 1;
  padding: 8px 12px;
  display: flex;
  overflow: hidden;
}

.cn-textarea {
  flex: 1;
  width: 100%;
  height: 100%;
  background: var(--cs-bg);
  border: 1px solid var(--cs-border);
  border-radius: 2px;
  color: var(--cs-text-bright);
  padding: 8px 10px;
  font-size: 12px;
  line-height: 1.6;
  resize: none;
  outline: none;
  font-family: var(--cs-font-mono);
}

.cn-textarea:focus {
  border-color: var(--cs-active);
}

.cn-bottom-bar {
  height: 34px;
  background: var(--cs-titlebar-bg);
  border-top: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  padding: 0 12px;
  gap: 8px;
  flex-shrink: 0;
}
</style>
