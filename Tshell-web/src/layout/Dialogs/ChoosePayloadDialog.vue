<template>
  <el-dialog
    v-model="visible"
    :title="title || (isZh ? '选择监听器' : 'Choose a listener')"
    width="780px"
    append-to-body
    class="cs-dialog"
    :close-on-click-modal="false"
  >
    <div class="cs-dialog-body">
      <div class="cs-dialog-desc">
        {{ desc || (isZh ? '选择运行中的监听器。载荷将回连该监听器。' : 'Select an active/running listener. The payload will connect back to this listener.') }}
      </div>

      <div class="cs-table-wrap">
        <el-table
          :data="listeners"
          size="small"
          height="240px"
          highlight-current-row
          v-loading="loading"
          @row-click="onRowClick"
          @row-dblclick="onRowDblClick"
          class="cs-session-table"
        >
          <el-table-column prop="name" :label="isZh ? '连接地址' : 'Connect Address'" min-width="160" sortable show-overflow-tooltip />
          <el-table-column prop="payload" :label="isZh ? '监听地址' : 'Listen Address'" min-width="150" sortable show-overflow-tooltip />
          <el-table-column prop="host" :label="isZh ? '主机' : 'Host'" min-width="110" sortable show-overflow-tooltip />
          <el-table-column prop="port" :label="isZh ? '端口' : 'Port'" width="80" sortable align="right" />
          <el-table-column :label="isZh ? '状态' : 'State'" width="90" align="center">
            <template #default="{ row }">
              <span :class="row.status === 1 ? 'st-online' : 'st-offline'">{{ row.status === 1 ? (isZh ? '运行中' : 'Active') : (isZh ? '未运行' : 'Stopped') }}</span>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <div v-if="!loading && !listeners.length" class="cs-empty-hint">
        {{ isZh ? '暂无运行中的监听器。请点击下方「添加监听器」前往监听器页面创建并启动，或先在「监听器」页面开启监听。' : 'No running listeners found. Click "Add Listener" to open the Listener page and start one.' }}
      </div>
    </div>

    <template #footer>
      <div class="cs-dialog-footer-center">
        <button class="cs-btn cs-btn-primary" :disabled="!selectedRow" @click="handleChoose">
          {{ isZh ? '选择' : 'Choose' }}
        </button>
        <button class="cs-btn" @click="handleAdd">
          {{ isZh ? '添加监听器' : 'Add Listener' }}
        </button>
        <button class="cs-btn" @click="handleRefresh">
          {{ isZh ? '刷新' : 'Refresh' }}
        </button>
        <button class="cs-btn" @click="handleHelp">
          {{ isZh ? '帮助' : 'Help' }}
        </button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18nStore } from '@/stores/i18n'
import { useWorkspaceStore } from '@/stores/workspace'
import { fetchListenerOptions, type ListenerOption } from './Payloads/payloadUtil'

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

export type { ListenerOption }

const props = defineProps<{
  modelValue: boolean
  title?: string
  desc?: string
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', val: boolean): void
  (e: 'choose', item: ListenerOption): void
  (e: 'add'): void
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v),
})

const selectedRow = ref<ListenerOption | null>(null)
const listeners = ref<ListenerOption[]>([])
const loading = ref(false)
const workspace = useWorkspaceStore()

// 打开时拉取真实监听器列表（后端 /client/listener/details）
async function loadListeners() {
  loading.value = true
  try {
    listeners.value = await fetchListenerOptions()
  } catch {
    listeners.value = []
  } finally {
    loading.value = false
  }
}

watch(
  () => props.modelValue,
  (val) => {
    if (val) {
      selectedRow.value = null
      loadListeners()
    }
  },
)

function onRowClick(row: ListenerOption) {
  selectedRow.value = row
}

function onRowDblClick(row: ListenerOption) {
  selectedRow.value = row
  handleChoose()
}

function handleChoose() {
  if (!selectedRow.value) return
  emit('choose', selectedRow.value)
  visible.value = false
}

function handleAdd() {
  // 跳转监听器页面创建新监听器
  workspace.openTab({ id: 'listeners', title: isZh.value ? '监听器' : 'Listeners', component: 'listeners' })
  visible.value = false
}

function handleRefresh() {
  loadListeners()
}

function handleHelp() {
  ElMessage.info(isZh.value ? '列表来自运行中的监听器；双击行可直接选择' : 'Rows come from running listeners; double-click a row to choose')
}
</script>

<style scoped>
.cs-dialog-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.cs-dialog-desc {
  font-size: 12px;
  color: var(--cs-text-dim);
  line-height: 1.4;
}
.cs-table-wrap {
  border: 1px solid var(--cs-border);
}
.cs-empty-hint {
  font-size: 12px;
  color: var(--cs-warning);
  line-height: 1.5;
}
.st-online {
  color: var(--cs-online);
  font-size: 11px;
}
.st-offline {
  color: var(--cs-text-dim);
  font-size: 11px;
}
.cs-dialog-footer-center {
  display: flex;
  justify-content: center;
  gap: 8px;
}
</style>
