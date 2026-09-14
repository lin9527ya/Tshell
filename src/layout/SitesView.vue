<template>
  <div class="sites-view cs-font-ui" @click="closeContextMenu">
    <!-- Toolbar -->
    <div class="sites-toolbar">
      <div class="sites-title-group">
        <span class="sites-title">{{ isZh ? '站点管理' : 'Sites' }}</span>
        <span class="sites-count">({{ filteredSites.length }} {{ isZh ? '个站点' : 'sites' }})</span>
      </div>

      <div class="sites-spacer" />

      <el-input
        v-model="searchKey"
        :placeholder="isZh ? '过滤 URI、主机、端口、类型、描述...' : 'Filter URI, Host, Port, Type, Description...'"
        size="small"
        clearable
        class="sites-search-input"
      />

      <button class="cs-btn" @click="refreshSites">
        {{ isZh ? '刷新' : 'Refresh' }}
      </button>
    </div>

    <!-- Table (1:1 CS Columns from Picture 5) -->
    <div class="sites-table-wrap">
      <el-table
        :data="filteredSites"
        size="small"
        height="100%"
        highlight-current-row
        class="cs-session-table"
        @current-change="onCurrentChange"
        @row-click="onRowClick"
        @row-dblclick="handleOpenUrl"
        @row-contextmenu="onRowContextMenu"
      >
        <el-table-column prop="uri" label="URI" min-width="180" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="cs-font-mono uri-text">{{ row.uri }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="host" :label="isZh ? '主机' : 'Host'" width="160" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="cs-font-mono">{{ row.host || defaultHost }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="port" :label="isZh ? '端口' : 'Port'" width="90" sortable align="left">
          <template #default="{ row }">
            <span class="cs-font-mono">{{ row.port }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="type" :label="isZh ? '类型' : 'Type'" width="120" sortable align="center">
          <template #default="{ row }">
            <span :class="['site-badge', `site-${row.type}`]">{{ row.type }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="description" :label="isZh ? '描述' : 'Description'" min-width="260" sortable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="desc-text">{{ row.description }}</span>
          </template>
        </el-table-column>

        <template #empty>
          <div class="sites-empty-watermark">
            <div class="empty-title">{{ isZh ? '暂无托管的站点与服务' : 'No Hosted Web Sites' }}</div>
            <div class="empty-desc">{{ isZh ? '通过顶栏【站点管理 -> 网站克隆】或【文件托管】创建托管服务。' : 'Create hosted sites via Site Management menu.' }}</div>
          </div>
        </template>
      </el-table>
    </div>

    <!-- Bottom Action Bar (1:1 CS Picture 5: Centered Copy URL | Kill | Help) -->
    <div class="sites-bottom-bar">
      <button class="cs-btn" :disabled="!selectedRow" @click="handleCopyUrl">
        {{ isZh ? '复制 URL' : 'Copy URL' }}
      </button>
      <button class="cs-btn cs-btn-danger" :disabled="!selectedRow" @click="handleKill">
        {{ isZh ? '终止' : 'Kill' }}
      </button>
      <button class="cs-btn" @click="handleHelp">
        {{ isZh ? '帮助' : 'Help' }}
      </button>
    </div>

    <!-- Right-Click Context Menu -->
    <Teleport to="body">
      <div
        v-if="ctxMenu.visible"
        class="cs-listener-ctx-menu cs-font-ui"
        :style="{ left: ctxMenu.x + 'px', top: ctxMenu.y + 'px' }"
        @mousedown.stop
        @contextmenu.prevent
      >
        <div class="ctx-item is-highlight" @click="handleCopyUrl">
          <span class="ctx-text">{{ isZh ? '复制 URL' : 'Copy URL' }}</span>
        </div>
        <div class="ctx-item" @click="handleOpenUrl">
          <span class="ctx-text">{{ isZh ? '在浏览器打开' : 'Open in Browser' }}</span>
        </div>
        <div class="ctx-divider" />
        <div class="ctx-item ctx-danger" @click="handleKill">
          <span class="ctx-text">{{ isZh ? '终止站点 (Kill)' : 'Kill Site' }}</span>
        </div>
        <div class="ctx-divider" />
        <div class="ctx-item" @click="handleHelp">
          <span class="ctx-text">{{ isZh ? '帮助' : 'Help' }}</span>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18nStore } from '@/stores/i18n'
import ClientAPI from '@/api/clients'

interface SiteItem {
  id: string | number
  uri: string
  host: string
  port: number | string
  type: string
  description: string
  ssl?: boolean
  raw?: any
}

const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const searchKey = ref('')
const selectedRow = ref<SiteItem | null>(null)
const defaultHost = window.location.hostname || '127.0.0.1'
const loading = ref(false)

const ctxMenu = ref({
  visible: false,
  x: 0,
  y: 0,
  row: null as SiteItem | null,
})

const sitesList = ref<SiteItem[]>([])

const filteredSites = computed(() => {
  if (!searchKey.value.trim()) return sitesList.value
  const q = searchKey.value.toLowerCase().trim()
  return sitesList.value.filter(
    (s) =>
      s.uri.toLowerCase().includes(q) ||
      String(s.host || '').toLowerCase().includes(q) ||
      String(s.port).includes(q) ||
      s.type.toLowerCase().includes(q) ||
      s.description.toLowerCase().includes(q),
  )
})

function onCurrentChange(row: SiteItem | null) {
  if (row) selectedRow.value = row
}

function onRowClick(row: SiteItem) {
  selectedRow.value = row
}

function onRowContextMenu(row: SiteItem, col: any, e: MouseEvent) {
  e.preventDefault()
  selectedRow.value = row
  ctxMenu.value = {
    visible: true,
    x: e.clientX,
    y: e.clientY,
    row,
  }
}

function closeContextMenu() {
  ctxMenu.value.visible = false
}

function buildFullUrl(site: SiteItem): string {
  const host = site.host || defaultHost
  const portNum = Number(site.port) || 80
  const scheme = site.ssl || portNum === 443 ? 'https' : 'http'
  const uriPath = site.uri.startsWith('/') ? site.uri : `/${site.uri}`
  const portStr = (scheme === 'http' && portNum === 80) || (scheme === 'https' && portNum === 443) ? '' : `:${portNum}`
  return `${scheme}://${host}${portStr}${uriPath}`
}

function handleCopyUrl() {
  closeContextMenu()
  const target = ctxMenu.value.row || selectedRow.value
  if (!target) return
  const url = buildFullUrl(target)
  navigator.clipboard.writeText(url).then(() => {
    ElMessage.success(isZh.value ? `已复制 URL: ${url}` : `Copied URL: ${url}`)
  }).catch(() => {
    ElMessage.info(url)
  })
}

function handleOpenUrl() {
  closeContextMenu()
  const target = ctxMenu.value.row || selectedRow.value
  if (!target) return
  const url = buildFullUrl(target)
  window.open(url, '_blank')
}

async function refreshSites() {
  loading.value = true
  try {
    const res = await ClientAPI.GetWebDeliveryList()
    if (res.status === 200 && res.data?.status === 200 && Array.isArray(res.data.data)) {
      sitesList.value = res.data.data.map((item: any) => {
        const id = item.id || item.ID || item.publicKey || item.port || String(Math.random())
        const port = item.ListeningPort || item.listening_port || item.port || 80
        const uri = item.URI || item.uri || item.resource || item.Resource || '/'
        const host = item.ServerAddress || item.server_address || item.Host || item.host || defaultHost
        const mode = item.mode || item.Mode || item.type || 'web delivery'
        const desc = item.Description || item.description || item.desc || (item.stager_url ? `Stager (${item.stager_url})` : `WebDelivery (${mode})`)

        return {
          id,
          uri: uri.startsWith('/') ? uri : `/${uri}`,
          host: host.replace(/^https?:\/\//, '').split(':')[0] || defaultHost,
          port,
          type: mode,
          description: desc,
          ssl: item.ssl || item.SSL || false,
          raw: item,
        }
      })
    } else {
      sitesList.value = []
    }
  } catch {
    // 保底不影响界面渲染
  } finally {
    loading.value = false
  }
}

async function handleKill() {
  closeContextMenu()
  const target = ctxMenu.value.row || selectedRow.value
  if (!target) return
  ElMessageBox.confirm(
    isZh.value ? `确定终止并关闭托管服务 "${target.uri}" (端口: ${target.port}) 吗？` : `Kill hosted site "${target.uri}" (port: ${target.port})?`,
    isZh.value ? '确认终止' : 'Confirm Kill',
    {
      confirmButtonText: isZh.value ? '终止' : 'Kill',
      cancelButtonText: isZh.value ? '取消' : 'Cancel',
      type: 'warning',
    },
  ).then(async () => {
    try {
      const raw = target.raw || {}
      await ClientAPI.DeleteWebDelivery({
        id: raw.id || raw.ID,
        port: String(target.port),
        publicKey: raw.publicKey || raw.public_key,
      })
      ElMessage.success(isZh.value ? `已成功终止服务 ${target.uri}` : `Terminated hosted service ${target.uri}`)
      selectedRow.value = null
      refreshSites()
    } catch (e: any) {
      ElMessage.error(e?.message || (isZh.value ? '终止失败' : 'Failed to kill service'))
    }
  }).catch(() => {})
}

function handleHelp() {
  closeContextMenu()
  ElMessage.info(isZh.value ? '站点管理：实时展示并管理已启动的 Web 投递服务、托管文件与网站克隆端点。' : 'Site Management: View and manage hosted Web Delivery endpoints, files, and cloned sites.')
}

defineExpose({
  refreshSites,
})

let autoTimer: number | undefined

onMounted(() => {
  window.addEventListener('mousedown', closeContextMenu)
  refreshSites()
  autoTimer = window.setInterval(refreshSites, 8000)
})

onBeforeUnmount(() => {
  window.removeEventListener('mousedown', closeContextMenu)
  if (autoTimer) window.clearInterval(autoTimer)
})
</script>

<style scoped>
.sites-view {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--cs-panel);
  overflow: hidden;
}

.sites-toolbar {
  height: 28px;
  background: var(--cs-bg);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 8px;
  flex-shrink: 0;
}

.sites-title-group {
  display: flex;
  align-items: center;
  gap: 6px;
}

.sites-title {
  font-size: 12px;
  font-weight: bold;
  color: var(--cs-text-bright);
}

.sites-count {
  font-size: 11px;
  color: var(--cs-text-dim);
}

.sites-spacer {
  flex: 1;
}

.sites-search-input {
  width: 240px;
}

.sites-table-wrap {
  flex: 1;
  overflow: hidden;
  position: relative;
}

.uri-text {
  font-size: 12px;
  color: var(--cs-active);
  font-weight: 500;
}

.desc-text {
  font-size: 12px;
  color: var(--cs-text);
}

.site-badge {
  font-size: 11px;
  padding: 1px 6px;
  border-radius: 2px;
  font-family: var(--cs-font-mono);
  background: color-mix(in srgb, var(--cs-active) 15%, transparent);
  color: var(--cs-active);
}
.site-badge.site-clone {
  background: color-mix(in srgb, #ff9800 20%, transparent);
  color: #ff9800;
}
.site-badge.site-file {
  background: color-mix(in srgb, #4caf50 20%, transparent);
  color: #4caf50;
}

.sites-empty-watermark {
  padding: 40px 20px;
  text-align: center;
}
.empty-title {
  font-size: 13px;
  font-weight: 500;
  color: var(--cs-text-bright);
  margin-bottom: 4px;
}
.empty-desc {
  font-size: 12px;
  color: var(--cs-text-dim);
}

/* Bottom Bar - Center aligned as in Screenshot 5 */
.sites-bottom-bar {
  height: 34px;
  background: var(--cs-bg);
  border-top: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 0 12px;
  flex-shrink: 0;
}

/* Context Menu */
.cs-listener-ctx-menu {
  position: fixed;
  background: var(--cs-panel);
  border: 1px solid var(--cs-border);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.45);
  border-radius: 2px;
  padding: 2px 0;
  z-index: 9999;
  min-width: 150px;
}

.ctx-item {
  display: flex;
  align-items: center;
  padding: 4px 12px;
  height: 24px;
  font-size: 12px;
  color: var(--cs-text);
  cursor: pointer;
  white-space: nowrap;
}
.ctx-item:hover {
  background: var(--cs-hover);
  color: var(--cs-text-bright);
}
.ctx-item.is-highlight {
  font-weight: bold;
  color: var(--cs-active);
}
.ctx-item.ctx-danger {
  color: #f56c6c;
}
.ctx-item.ctx-danger:hover {
  background: color-mix(in srgb, #f56c6c 20%, transparent);
  color: #f56c6c;
}
.ctx-divider {
  height: 1px;
  background: var(--cs-border);
  margin: 3px 0;
}
</style>
