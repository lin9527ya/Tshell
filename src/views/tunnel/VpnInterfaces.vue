<template>
  <div class="vpn-interfaces-view cs-font-ui">
    <!-- CS Toolbar -->
    <div class="vi-toolbar">
      <div class="vi-title-group">
        <span class="vi-title">{{ isZh ? '隐蔽 VPN 网卡' : 'Covert VPN Interfaces' }}</span>
        <span class="vi-count">({{ interfacesList.length }} {{ isZh ? '个网卡' : 'interfaces' }})</span>
      </div>

      <div class="vi-spacer" />

      <el-input
        v-model="searchKey"
        :placeholder="isZh ? '过滤网卡、客户端、MAC...' : 'Filter interface, client, MAC...'"
        size="small"
        clearable
        class="vi-search"
      />

      <button class="cs-btn" @click="refresh">{{ isZh ? '刷新' : 'Refresh' }}</button>
    </div>

    <!-- 1:1 CS Table (Screenshot 18.png) -->
    <div class="vi-table-wrap">
      <el-table
        :data="filteredList"
        size="small"
        height="100%"
        highlight-current-row
        class="cs-session-table"
        @current-change="onCurrentChange"
      >
        <el-table-column prop="interface" :label="isZh ? '网卡' : 'interface'" width="130" sortable>
          <template #default="{ row }">
            <span class="mono-cell font-bold">{{ row.interface }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="channel" :label="isZh ? '通道' : 'channel'" width="120" sortable align="center">
          <template #default="{ row }">
            <span class="arch-badge arch-64">{{ row.channel }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="port" :label="isZh ? '端口' : 'port'" width="90" sortable align="right">
          <template #default="{ row }">
            <span class="mono-cell">{{ row.port }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="mac" :label="isZh ? 'MAC' : 'mac'" min-width="150" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="mono-cell">{{ row.mac }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="client" :label="isZh ? '客户端' : 'client'" min-width="160" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="user-cell">{{ row.client }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="tx" :label="isZh ? '发送' : 'tx'" width="100" sortable align="right">
          <template #default="{ row }">
            <span class="mono-cell">{{ row.tx }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="rx" :label="isZh ? '接收' : 'rx'" width="100" sortable align="right">
          <template #default="{ row }">
            <span class="mono-cell">{{ row.rx }}</span>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <!-- Bottom Buttons: [ Add ] [ Remove ] [ Help ] (Screenshot 18.png) -->
    <div class="vi-bottom-bar">
      <button class="cs-btn cs-btn-primary" @click="showAddDialog = true">
        {{ isZh ? '添加' : 'Add' }}
      </button>
      <button class="cs-btn cs-btn-danger" :disabled="!selectedRow" @click="handleRemove">
        {{ isZh ? '删除' : 'Remove' }}
      </button>
      <button class="cs-btn" @click="handleHelp">
        {{ isZh ? '帮助' : 'Help' }}
      </button>
    </div>

    <!-- Add VPN Interface Dialog -->
    <el-dialog
      v-model="showAddDialog"
      :title="isZh ? '隐蔽 VPN 网卡' : 'Covert VPN Interface'"
      width="460px"
      :close-on-click-modal="false"
      append-to-body
      class="cs-dialog"
    >
      <div class="cs-form-container cs-font-ui">
        <div class="cs-form-row">
          <label class="cs-form-label">{{ isZh ? '网卡' : 'Interface' }}:</label>
          <el-input v-model="form.interface" size="small" placeholder="tap0" />
        </div>
        <div class="cs-form-row">
          <label class="cs-form-label">{{ isZh ? 'MAC 地址' : 'MAC Address' }}:</label>
          <el-input v-model="form.mac" size="small" placeholder="00:15:5d:01:02:03" />
        </div>
        <div class="cs-form-row">
          <label class="cs-form-label">{{ isZh ? '通道' : 'Channel' }}:</label>
          <el-select v-model="form.channel" size="small" style="width: 100%">
            <el-option label="UDP" value="UDP" />
            <el-option :label="isZh ? 'TCP（反向）' : 'TCP (Reverse)'" value="TCP (Reverse)" />
            <el-option :label="isZh ? 'TCP（绑定）' : 'TCP (Bind)'" value="TCP (Bind)" />
            <el-option label="HTTP" value="HTTP" />
            <el-option label="HTTPS (TLS)" value="HTTPS (TLS)" />
            <el-option :label="isZh ? 'ICMP（隐蔽回显）' : 'ICMP (Echo Stealth)'" value="ICMP" />
            <el-option :label="isZh ? 'DNS（隧道）' : 'DNS (Tunneling)'" value="DNS" />
            <el-option label="WebSocket (WSS)" value="WebSocket" />
            <el-option label="KCP (Reliable UDP)" value="KCP" />
            <el-option :label="isZh ? 'SMB（命名管道）' : 'SMB (Named Pipe)'" value="SMB" />
          </el-select>
        </div>
        <div class="cs-form-row">
          <label class="cs-form-label">{{ isZh ? '端口' : 'Port' }}:</label>
          <el-input v-model="form.port" size="small" placeholder="4444" />
        </div>
      </div>
      <template #footer>
        <div class="cs-dialog-footer">
          <button class="cs-btn cs-btn-primary" @click="handleAddInterface">{{ isZh ? '保存' : 'Save' }}</button>
          <button class="cs-btn" @click="showAddDialog = false">{{ isZh ? '取消' : 'Cancel' }}</button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18nStore } from '@/stores/i18n'

interface VpnInterface {
  interface: string
  channel: string
  port: string | number
  mac: string
  client: string
  tx: string
  rx: string
}

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const searchKey = ref('')
const selectedRow = ref<VpnInterface | null>(null)
const showAddDialog = ref(false)

// 示例数据：后端无 VPN API，网卡列表为本地演示
const interfacesList = ref<VpnInterface[]>([
  {
    interface: 'tap0',
    channel: 'UDP',
    port: 4444,
    mac: '00:15:5d:01:2b:88',
    client: '192.168.10.10 (beacon)',
    tx: '1.2 MB',
    rx: '3.4 MB',
  },
])

const form = reactive({
  interface: 'tap1',
  mac: '00:15:5d:02:3c:99',
  channel: 'UDP',
  port: '5555',
})

const filteredList = computed(() => {
  if (!searchKey.value.trim()) return interfacesList.value
  const kw = searchKey.value.toLowerCase().trim()
  return interfacesList.value.filter(
    (i) =>
      i.interface.toLowerCase().includes(kw) ||
      i.client.toLowerCase().includes(kw) ||
      i.mac.toLowerCase().includes(kw),
  )
})

function onCurrentChange(row: VpnInterface) {
  selectedRow.value = row
}

function handleAddInterface() {
  interfacesList.value.push({
    interface: form.interface,
    channel: form.channel,
    port: form.port,
    mac: form.mac,
    client: isZh.value ? '本地 / 团队服务器' : 'Local / TeamServer',
    tx: '0 B',
    rx: '0 B',
  })
  showAddDialog.value = false
  ElMessage.warning(isZh.value ? '演示功能：后端未实现 VPN 接口管理，网卡不会实际创建' : 'Demo only: VPN interface management not implemented on backend; nothing is created')
}

function handleRemove() {
  if (!selectedRow.value) return
  ElMessageBox.confirm(
      isZh.value ? `移除 VPN 网卡 ${selectedRow.value.interface} 吗？` : `Remove VPN interface ${selectedRow.value.interface}?`,
      isZh.value ? '移除网卡' : 'Remove Interface',
      {
        confirmButtonText: isZh.value ? '移除' : 'Remove',
        cancelButtonText: isZh.value ? '取消' : 'Cancel',
        type: 'warning',
      },
    ).then(() => {
    interfacesList.value = interfacesList.value.filter((i) => i.interface !== selectedRow.value?.interface)
    selectedRow.value = null
    ElMessage.warning(isZh.value ? '已从本地示例列表移除（演示功能）' : 'Removed from local demo list (demo only)')
  }).catch(() => {})
}

function handleHelp() {
  ElMessage.info(isZh.value ? '隐蔽 VPN 网卡管理说明' : 'Covert VPN Interfaces Management Help')
}

function refresh() {
  ElMessage.warning(isZh.value ? '演示功能：网卡列表为本地示例，无可刷新数据' : 'Demo only: interface list is local sample data')
}
</script>

<style scoped>
.vpn-interfaces-view {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-bg);
  overflow: hidden;
}

.vi-toolbar {
  height: 32px;
  background: var(--cs-panel);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
  flex-shrink: 0;
}

.vi-title-group {
  display: flex;
  align-items: center;
  gap: 6px;
}

.vi-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.vi-count {
  font-size: 11px;
  color: var(--cs-text-dim);
}

.vi-spacer {
  flex: 1;
}

.vi-search {
  width: 220px;
}

.vi-table-wrap {
  flex: 1;
  overflow: hidden;
}

.vi-bottom-bar {
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

.arch-badge {
  font-size: 10px;
  padding: 1px 4px;
  border-radius: 2px;
  font-family: var(--cs-font-mono);
}

.arch-64 {
  background: rgba(22, 119, 255, 0.2);
  color: #69b1ff;
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
  width: 100px;
  font-size: 12px;
  text-align: right;
  color: var(--cs-text);
  flex-shrink: 0;
}
</style>
