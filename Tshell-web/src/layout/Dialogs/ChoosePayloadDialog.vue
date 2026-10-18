<template>
  <el-dialog
    v-model="visible"
    :title="title || (i18nStore.tl('选择监听器', 'Choose a listener'))"
    width="780px"
    append-to-body
    class="cs-dialog"
    :close-on-click-modal="false"
  >
    <div class="cs-dialog-body">
      <div class="cs-dialog-desc">
        {{ desc || (i18nStore.tl('选择运行中的监听器。载荷将回连该监听器。', 'Select an active/running listener. The payload will connect back to this listener.')) }}
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
          <el-table-column prop="name" :label="i18nStore.tl('连接地址', 'Connect Address')" min-width="160" sortable show-overflow-tooltip />
          <el-table-column prop="payload" :label="i18nStore.tl('监听地址', 'Listen Address')" min-width="150" sortable show-overflow-tooltip />
          <el-table-column prop="host" :label="i18nStore.tl('主机', 'Host')" min-width="110" sortable show-overflow-tooltip />
          <el-table-column prop="port" :label="i18nStore.tl('端口', 'Port')" width="80" sortable align="right" />
          <el-table-column :label="i18nStore.tl('状态', 'State')" width="90" align="center">
            <template #default="{ row }">
              <span :class="row.status === 1 ? 'st-online' : 'st-offline'">{{ row.status === 1 ? (i18nStore.tl('运行中', 'Active')) : (i18nStore.tl('未运行', 'Stopped')) }}</span>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <div v-if="!loading && !listeners.length" class="cs-empty-hint">
        {{ i18nStore.tl('暂无运行中的监听器。请点击下方「添加监听器」前往监听器页面创建并启动，或先在「监听器」页面开启监听。', 'No running listeners found. Click "Add Listener" to open the Listener page and start one.') }}
      </div>
    </div>

    <template #footer>
      <div class="cs-dialog-footer-center">
        <button class="cs-btn cs-btn-primary" :disabled="!selectedRow" @click="handleChoose">
          {{ i18nStore.tl('选择', 'Choose') }}
        </button>
        <button class="cs-btn" @click="handleAdd">
          {{ i18nStore.tl('添加监听器', 'Add Listener') }}
        </button>
        <button class="cs-btn" @click="handleRefresh">
          {{ i18nStore.tl('刷新', 'Refresh') }}
        </button>
        <button class="cs-btn" @click="handleHelp">
          {{ i18nStore.tl('帮助', 'Help') }}
        </button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useI18nStore } from '@/stores/i18n'
import { useSessionSelectionStore } from '@/stores/sessionSelection'
import { fetchListenerOptions, type ListenerOption } from './Payloads/payloadUtil'

const i18nStore = useI18nStore()
const router = useRouter()
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
const workspace = useSessionSelectionStore()

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
  router.push('/listener')
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
.cs-table-wrap {
  border-radius: 8px;
  overflow: hidden;
}
.cs-empty-hint {
  font-size: 13px;
  color: var(--cs-warning);
  line-height: 1.6;
}
.st-online {
  color: var(--cs-online);
  font-size: 12px;
}
.st-offline {
  color: var(--cs-text-dim);
  font-size: 12px;
}
</style>
