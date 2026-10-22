<template>
  <el-dialog
    v-model="visible"
    :title="i18nStore.tl('上线命令', 'Launch Commands')"
    width="1100px"
    append-to-body
    class="cs-dialog"
  >
    <div v-if="listenerLabel" class="dc-listener">
      <span class="dc-label">{{ i18nStore.tl('监听器', 'Listener') }}:</span>
      <code>{{ listenerLabel }}</code>
    </div>

    <div v-if="deliveries.length === 0" class="dc-empty">
      <EmptyState
        variant="inline"
        icon="📦"
        :title="i18nStore.tl('该监听器还没有关联的 WebDelivery 投递', 'No WebDelivery linked to this listener')"
      />
      <p class="dc-hint">
        {{ i18nStore.tl(
          '请到「客户端生成 → 投递」创建并开启 WebDelivery，再回来查看上线命令。',
          'Create and start a WebDelivery under Payloads → Delivery, then return here.',
        ) }}
      </p>
    </div>

    <template v-else>
      <div v-if="deliveries.length > 1" class="dc-picker">
        <span class="dc-label">{{ i18nStore.tl('投递', 'Delivery') }}:</span>
        <el-select v-model="selected" size="small" class="dc-select">
          <el-option v-for="d in deliveryOptions" :key="d.key" :label="d.label" :value="d.key" />
        </el-select>
      </div>

      <div v-if="urls" class="dc-urls">
        <div v-if="urls.full">
          <b>{{ i18nStore.tl('完整', 'Full') }}:</b>
          <code>{{ urls.full }}</code>
          <el-button link type="primary" @click="copy(urls.full)">{{ i18nStore.tl('复制', 'Copy') }}</el-button>
        </div>
        <div v-if="urls.stager">
          <b>{{ i18nStore.tl('分阶段', 'Stager') }}:</b>
          <code>{{ urls.stager }}</code>
          <el-button link type="primary" @click="copy(urls.stager)">{{ i18nStore.tl('复制', 'Copy') }}</el-button>
        </div>
        <div v-if="urls.shellcode">
          <b>Shellcode/WOFF:</b>
          <code>{{ urls.shellcode }}</code>
          <el-button link type="primary" @click="copy(urls.shellcode)">{{ i18nStore.tl('复制', 'Copy') }}</el-button>
        </div>
      </div>

      <el-table v-loading="loading" :data="commands" max-height="560" size="small" border>
        <el-table-column prop="name" :label="i18nStore.tl('名称', 'Name')" width="180" show-overflow-tooltip />
        <el-table-column prop="os" label="OS" width="80" />
        <el-table-column prop="description" :label="i18nStore.tl('说明', 'Description')" width="220" show-overflow-tooltip />
        <el-table-column prop="code" :label="i18nStore.tl('命令', 'Command')" min-width="420">
          <template #default="{ row }">
            <code class="dc-cmd">{{ row.code }}</code>
          </template>
        </el-table-column>
        <el-table-column width="64">
          <template #default="{ row }">
            <el-button link type="primary" @click="copy(row.code)">{{ i18nStore.tl('复制', 'Copy') }}</el-button>
          </template>
        </el-table-column>
        <template #empty>
          <EmptyState variant="inline" icon="💻" :title="i18nStore.tl('没有可用的上线命令', 'No launch commands')" />
        </template>
      </el-table>

      <p v-if="launchNote" class="dc-hint">{{ launchNote }}</p>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import ClientAPI from '@/api/clients'
import { useI18nStore } from '@/stores/i18n'
import EmptyState from '@/components/Common/EmptyState.vue'

const props = defineProps<{
  modelValue: boolean
  deliveries: any[]
  listenerLabel?: string
}>()

const emit = defineEmits<{ (e: 'update:modelValue', value: boolean): void }>()

const i18nStore = useI18nStore()
const loading = ref(false)
const selected = ref('')
const commands = ref<any[]>([])
const urls = ref<{ full?: string; stager?: string; shellcode?: string } | null>(null)
const launchNote = ref('')

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v),
})

function field(row: any, ...keys: string[]): any {
  for (const k of keys) {
    const v = row?.[k]
    if (v !== undefined && v !== null && v !== '') return v
  }
  return ''
}

function deliveryKey(d: any): string {
  return String(field(d, 'ID', 'id') || field(d, 'PublicKey', 'public_key') || field(d, 'ListeningPort', 'listening_port'))
}

function deliveryLabel(d: any): string {
  const id = field(d, 'ID', 'id')
  const port = field(d, 'ListeningPort', 'listening_port')
  const os = field(d, 'OS', 'os')
  const note = field(d, 'Note', 'note')
  return [
    id ? `#${id}` : '',
    port ? `${i18nStore.tl('端口', 'Port')} ${port}` : '',
    os || '',
    note || '',
  ].filter(Boolean).join(' · ')
}

const deliveryOptions = computed(() =>
  props.deliveries.map((d) => ({ key: deliveryKey(d), label: deliveryLabel(d) })),
)

const currentDelivery = computed(() =>
  props.deliveries.find((d) => deliveryKey(d) === selected.value) || props.deliveries[0],
)

async function loadCommands() {
  const d = currentDelivery.value
  if (!d) return
  loading.value = true
  try {
    const res = await ClientAPI.GetDeliveryCommands({
      id: field(d, 'ID', 'id') || undefined,
      publicKey: field(d, 'PublicKey', 'public_key') || undefined,
      port: field(d, 'ListeningPort', 'listening_port') || undefined,
    })
    if (res.status === 200 && res.data?.status === 200) {
      const payload = res.data.data || {}
      commands.value = payload.commands || []
      urls.value = payload.urls || {
        full: field(d, 'ServerAddress', 'server_address'),
        stager: field(d, 'StagerURL', 'stager_url'),
        shellcode: field(d, 'ShellcodeURL', 'shellcode_url'),
      }
      launchNote.value = payload.launch_note || ''
    } else {
      commands.value = []
      urls.value = null
      launchNote.value = ''
      ElMessage.error(res.data?.data || i18nStore.tl('获取上线命令失败', 'Failed to load launch commands'))
    }
  } catch (e: any) {
    commands.value = []
    urls.value = null
    launchNote.value = ''
    ElMessage.error(e?.message || i18nStore.tl('获取上线命令失败', 'Failed to load launch commands'))
  } finally {
    loading.value = false
  }
}

watch(
  () => props.modelValue,
  (open) => {
    if (!open) return
    const preferred = props.deliveries.find((d) => Number(field(d, 'Status', 'status')) === 1) || props.deliveries[0]
    const prev = selected.value
    selected.value = preferred ? deliveryKey(preferred) : ''
    if (selected.value === prev) loadCommands()
  },
)

watch(selected, () => {
  if (props.modelValue && selected.value) loadCommands()
})

async function copy(text: string) {
  if (!text) return
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(i18nStore.tl('已复制', 'Copied'))
  } catch {
    ElMessage.error(i18nStore.tl('复制失败', 'Copy failed'))
  }
}
</script>

<style scoped>
.dc-listener {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  font-size: 13px;
  color: var(--cs-text-dim);
}

.dc-listener code {
  color: var(--cs-text-bright);
}

.dc-label {
  color: var(--cs-text-dim);
  white-space: nowrap;
}

.dc-picker {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}

.dc-select {
  width: 360px;
  max-width: 70%;
}

.dc-urls {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px 12px;
  margin-bottom: 12px;
  background: var(--cs-hover);
  border-radius: 6px;
  font-size: 13px;
}

.dc-urls > div {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.dc-urls code {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dc-cmd {
  font-family: var(--cs-font-mono);
  font-size: 12px;
  word-break: break-all;
}

.dc-empty {
  padding: 16px 0 4px;
  text-align: center;
}

.dc-hint {
  margin: 8px 0 0;
  font-size: 13px;
  color: var(--cs-text-dim);
}
</style>
