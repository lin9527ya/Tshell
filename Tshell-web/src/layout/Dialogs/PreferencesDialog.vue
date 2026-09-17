<template>
  <el-dialog
    v-model="visible"
    :title="isZh ? '偏好设置' : 'Preferences'"
    width="680px"
    :close-on-click-modal="false"
    append-to-body
    class="cs-dialog cs-preferences-dialog"
  >
    <div class="cs-pref-layout cs-font-ui">
      <!-- Left Category List -->
      <div class="cs-pref-sidebar">
        <div
          v-for="cat in computedCategories"
          :key="cat.id"
          class="pref-nav-item"
          :class="{ active: activeCategory === cat.id }"
          @click="activeCategory = cat.id"
        >
          <span class="nav-icon">{{ cat.icon }}</span>
          <span class="nav-label">{{ cat.label }}</span>
        </div>
      </div>

      <!-- Right Option Panel -->
      <div class="cs-pref-content">
        <!-- 1. Cobalt Strike / BladeShell -->
        <div v-if="activeCategory === 'cobalt-strike'" class="pref-panel">
          <div class="panel-desc">{{ isZh ? '配置平台核心参数与界面显示。' : 'Options to configure BladeShell platform.' }}</div>

          <div class="pref-form-grid">
            <div class="form-row">
              <label class="row-label">{{ isZh ? '标签活动颜色:' : 'Tab Activity:' }}</label>
              <div class="color-picker-row">
                <el-input v-model="pref.tabActivity" size="small" class="hex-input" />
                <div
                  class="color-swatch"
                  :style="{ backgroundColor: pref.tabActivity }"
                  @click="cycleColor"
                />
              </div>
            </div>

            <div class="form-row">
              <label class="row-label">{{ isZh ? '快速工具栏:' : 'Toolbar:' }}</label>
              <el-checkbox v-model="pref.showToolbar">{{ isZh ? '在主窗口显示快速工具栏' : 'Show toolbar in main window' }}</el-checkbox>
            </div>

            <div class="form-row">
              <label class="row-label">{{ isZh ? 'VNC 端口范围:' : 'VNC Ports:' }}</label>
              <el-input v-model="pref.vncPorts" size="small" style="width: 240px" />
            </div>

            <div class="form-row">
              <label class="row-label">{{ isZh ? '界面字体:' : 'GUI Font:' }}</label>
              <div class="font-picker-row">
                <el-input v-model="pref.guiFont" size="small" style="width: 240px" />
                <button class="cs-btn cs-btn-xs" @click="pickFont">...</button>
              </div>
            </div>

            <div class="form-row">
              <label class="row-label">{{ isZh ? '界面主题:' : 'GUI Theme:' }}</label>
              <el-select v-model="pref.guiTheme" size="small" style="width: 240px" @change="onThemeChange">
                <el-option :label="isZh ? '暗色 (Darcula)' : 'Dark (Darcula)'" value="dark" />
                <el-option :label="isZh ? '明亮模式' : 'Light'" value="light" />
              </el-select>
            </div>

            <div class="form-row">
              <label class="row-label">{{ isZh ? '主题强调色:' : 'Accent Color:' }}</label>
              <div class="color-presets">
                <button
                  v-for="c in themeStore.presetColors"
                  :key="c"
                  class="color-swatch"
                  :class="{ active: themeStore.config.primaryColor === c }"
                  :style="{ background: c }"
                  :title="c"
                  @click="themeStore.setPrimaryColor(c)"
                />
              </div>
            </div>
          </div>
        </div>

        <!-- 2. Console -->
        <div v-else-if="activeCategory === 'console'" class="pref-panel">
          <div class="panel-desc">{{ isZh ? '配置控制台外观、字体及输出缓冲区。' : 'Options to configure Console appearance and buffers.' }}</div>

          <div class="pref-form-grid">
            <div class="form-row">
              <label class="row-label">{{ isZh ? '控制台字体:' : 'Console Font:' }}</label>
              <el-input v-model="pref.consoleFont" size="small" style="width: 240px" />
            </div>
            <div class="form-row">
              <label class="row-label">{{ isZh ? '字体大小:' : 'Font Size:' }}</label>
              <el-input-number v-model="pref.consoleFontSize" :min="10" :max="24" size="small" />
            </div>
            <div class="form-row">
              <label class="row-label">{{ isZh ? '回滚行数:' : 'Scrollback Lines:' }}</label>
              <el-input-number v-model="pref.scrollback" :min="1000" :max="50000" :step="1000" size="small" />
            </div>
            <div class="form-row">
              <label class="row-label">{{ isZh ? '自动滚动:' : 'Auto-Scroll:' }}</label>
              <el-checkbox v-model="pref.consoleAutoScroll">{{ isZh ? '始终跟随控制台最新输出' : 'Always follow new console output' }}</el-checkbox>
            </div>
            <div class="form-row">
              <label class="row-label">{{ isZh ? '上线提示音:' : 'Online Audio:' }}</label>
              <el-checkbox v-model="pref.audioNotify">{{ isZh ? '客户端上线时播放提示音(浏览器播放,需已登录交互)' : 'Play a chime when a client comes online' }}</el-checkbox>
            </div>
          </div>
        </div>

        <!-- 3. Fingerprints -->
        <div v-else-if="activeCategory === 'fingerprints'" class="pref-panel">
          <div class="panel-desc">{{ isZh ? '团队服务器 SSL / TLS 通信指纹列表。' : 'Saved TeamServer SSH / SSL Fingerprints.' }}</div>
          <div class="pref-empty-note">
            {{ isZh ? '已验证活动团队服务器的 SHA256 安全指纹。' : 'SHA256 Fingerprint cache verified for active servers.' }}
          </div>
        </div>

        <!-- 4. Graph -->
        <div v-else-if="activeCategory === 'graph'" class="pref-panel">
          <div class="panel-desc">{{ isZh ? '拓扑网络图可视化渲染与物理引擎配置。' : 'Pivot Graph visualization rendering settings.' }}</div>
          <div class="pref-form-grid">
            <div class="form-row">
              <label class="row-label">{{ isZh ? '平滑动画:' : 'Animation:' }}</label>
              <el-checkbox v-model="pref.graphAnimation">{{ isZh ? '启用物理引擎与平滑排版' : 'Enable physics and smooth node layout' }}</el-checkbox>
            </div>
            <div class="form-row">
              <label class="row-label">{{ isZh ? '节点尺寸:' : 'Node Size:' }}</label>
              <el-slider v-model="pref.nodeSize" :min="24" :max="64" style="width: 200px" />
            </div>
          </div>
        </div>

        <!-- 5. Reporting -->
        <div v-else-if="activeCategory === 'reporting'" class="pref-panel">
          <div class="panel-desc">{{ isZh ? '活动报告品牌定制与导出选项。' : 'Activity report branding and generation options.' }}</div>
          <div class="pref-form-grid">
            <div class="form-row">
              <label class="row-label">{{ isZh ? '报告 Logo:' : 'Report Logo:' }}</label>
              <el-input v-model="pref.reportLogo" size="small" :placeholder="isZh ? '默认 BladeShell 标志' : 'Default BladeShell Logo'" style="width: 240px" />
            </div>
            <div class="form-row">
              <label class="row-label">{{ isZh ? '作者署名:' : 'Author Name:' }}</label>
              <el-input v-model="pref.reportAuthor" size="small" style="width: 240px" />
            </div>
          </div>
        </div>

        <!-- 6. Statusbar -->
        <div v-else-if="activeCategory === 'statusbar'" class="pref-panel">
          <div class="panel-desc">{{ isZh ? '配置底部状态栏展示指标。' : 'Configure Status bar widgets and metrics.' }}</div>
          <div class="pref-form-grid">
            <div class="form-row">
              <label class="row-label">{{ isZh ? '显示延迟:' : 'Show Lag:' }}</label>
              <el-checkbox v-model="pref.showLag">{{ isZh ? '在状态栏显示团队服务器通信延迟' : 'Display TeamServer latency indicator' }}</el-checkbox>
            </div>
            <div class="form-row">
              <label class="row-label">{{ isZh ? '显示会话数:' : 'Show Beacons Count:' }}</label>
              <el-checkbox v-model="pref.showBeaconsCount">{{ isZh ? '在状态栏显示活动 Beacon 数量' : 'Display active beacon counter' }}</el-checkbox>
            </div>
          </div>
        </div>

        <!-- 7. Team Servers -->
        <div v-else-if="activeCategory === 'team-servers'" class="pref-panel">
          <div class="panel-desc">{{ isZh ? '配置团队服务器连接配置。' : 'Configured Team Server profiles.' }}</div>
          <div class="pref-empty-note">
            {{ isZh ? '活动服务: 127.0.0.1:8083 (已作为 admin 建立连接)' : 'Active: 127.0.0.1:50050 (Connected as admin)' }}
          </div>
        </div>

        <!-- 8. Beacon Health -->
        <div v-else-if="activeCategory === 'beacon-health'" class="pref-panel">
          <div class="panel-desc">{{ isZh ? 'Beacon 心跳监控与告警阈值配置。' : 'Beacon heartbeat monitoring and alert thresholds.' }}</div>
          <div class="pref-form-grid">
            <div class="form-row">
              <label class="row-label">{{ isZh ? '超时心跳告警阈值:' : 'Missed Heartbeats Alert:' }}</label>
              <el-input-number v-model="pref.missedHeartbeats" :min="1" :max="10" size="small" />
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Centered Save Button -->
    <template #footer>
      <div class="cs-preferences-footer">
        <button class="cs-btn cs-btn-primary" @click="savePreferences">
          {{ isZh ? '保存设置' : 'Save' }}
        </button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useWorkspaceStore } from '@/stores/workspace'
import { useThemeStore } from '@/stores/theme'
import { useI18nStore } from '@/stores/i18n'
import { usePreferencesStore } from '@/stores/preferences'

const workspace = useWorkspaceStore()
const themeStore = useThemeStore()
const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const visible = computed({
  get: () => workspace.dialogs.preferences,
  set: (val) => {
    if (!val) workspace.closeDialog('preferences')
  },
})

const activeCategory = ref('cobalt-strike')

const categories = [
  { id: 'cobalt-strike', label: 'BladeShell', zhLabel: '平台核心', icon: '🔧' },
  { id: 'console', label: 'Console', zhLabel: '控制台', icon: '🖥️' },
  { id: 'fingerprints', label: 'Fingerprints', zhLabel: '安全指纹', icon: '🏅' },
  { id: 'graph', label: 'Graph', zhLabel: '拓扑网络', icon: '📈' },
  { id: 'reporting', label: 'Reporting', zhLabel: '报告设置', icon: '📄' },
  { id: 'statusbar', label: 'Statusbar', zhLabel: '状态栏', icon: '💻' },
  { id: 'team-servers', label: 'Team Servers', zhLabel: '团队服务器', icon: '➕' },
  { id: 'beacon-health', label: 'Beacon Health', zhLabel: '会话健康', icon: '🩺' },
]

const computedCategories = computed(() =>
  categories.map((c) => ({
    ...c,
    label: isZh.value ? c.zhLabel : c.label,
  }))
)

const pref = reactive({
  tabActivity: '#0000ff',
  showToolbar: true,
  vncPorts: '5000-9999',
  guiFont: 'Dialog-PLAIN-12',
  guiTheme: (themeStore.isDark ? 'dark' : 'light'),
  consoleFont: "'JetBrains Mono', Consolas, Menlo, monospace",
  consoleFontSize: 13,
  scrollback: 10000,
  consoleAutoScroll: true,
  graphAnimation: true,
  nodeSize: 36,
  reportLogo: '',
  reportAuthor: 'Administrator',
  showLag: true,
  showBeaconsCount: true,
  missedHeartbeats: 3,
  audioNotify: false,
})

// 打开时从持久化偏好回填(localStorage 持久层,P2-12)
const prefStore = usePreferencesStore()
pref.consoleFont = prefStore.consoleFont
pref.consoleFontSize = prefStore.consoleFontSize
pref.scrollback = prefStore.scrollback
pref.consoleAutoScroll = prefStore.consoleAutoScroll
pref.graphAnimation = prefStore.graphAnimation
pref.nodeSize = prefStore.nodeSize
pref.showLag = prefStore.showLag
pref.showBeaconsCount = prefStore.showBeaconsCount
pref.missedHeartbeats = prefStore.missedHeartbeats
pref.audioNotify = prefStore.audioNotify

function cycleColor() {
  const colors = ['#0000ff', '#ff0000', '#00ff00', '#ff9900', '#9900ff']
  const idx = colors.indexOf(pref.tabActivity)
  pref.tabActivity = colors[(idx + 1) % colors.length]
}

function pickFont() {
  ElMessage.info('Font selector: Dialog-PLAIN-12')
}

function onThemeChange(val: string) {
  if (val === 'dark') {
    themeStore.setTheme('dark')
  } else {
    themeStore.setTheme('light')
  }
}

function savePreferences() {
  // 可持久化子集写回 preferences store(localStorage 持久,P2-12);
  // 主题类设置即时生效;tabActivity/vncPorts 等 CS 形态项不持久化
  prefStore.consoleFont = pref.consoleFont
  prefStore.consoleFontSize = pref.consoleFontSize
  prefStore.scrollback = pref.scrollback
  prefStore.consoleAutoScroll = pref.consoleAutoScroll
  prefStore.graphAnimation = pref.graphAnimation
  prefStore.nodeSize = pref.nodeSize
  prefStore.showLag = pref.showLag
  prefStore.showBeaconsCount = pref.showBeaconsCount
  prefStore.missedHeartbeats = pref.missedHeartbeats
  prefStore.audioNotify = pref.audioNotify
  ElMessage.success(isZh.value ? '偏好已保存(控制台/图/提示音设置已持久化)' : 'Preferences saved')
  workspace.closeDialog('preferences')
}
</script>

<style scoped>
.cs-preferences-dialog :deep(.el-dialog__body) {
  padding: 10px 14px;
}

.cs-pref-layout {
  display: flex;
  height: 380px;
  border: 1px solid var(--cs-border);
  background: var(--cs-panel);
}

.cs-pref-sidebar {
  width: 170px;
  border-right: 1px solid var(--cs-border);
  background: var(--cs-bg);
  overflow-y: auto;
  padding: 4px 0;
}

.pref-nav-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 12px;
  font-size: 12px;
  color: var(--cs-text);
  cursor: pointer;
  user-select: none;
}

.pref-nav-item:hover {
  background: var(--cs-hover);
  color: var(--cs-text-bright);
}

.pref-nav-item.active {
  background: #185abc;
  color: #ffffff;
  font-weight: 500;
}

.nav-icon {
  font-size: 13px;
}

.cs-pref-content {
  flex: 1;
  padding: 16px 20px;
  overflow-y: auto;
  background: var(--cs-panel);
}

.panel-desc {
  font-size: 13px;
  color: var(--cs-text-bright);
  margin-bottom: 18px;
}

.pref-form-grid {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.form-row {
  display: flex;
  align-items: center;
  gap: 12px;
}

.row-label {
  width: 110px;
  font-size: 12px;
  color: var(--cs-text);
  text-align: right;
  flex-shrink: 0;
}

.color-picker-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.hex-input {
  width: 240px;
}

.color-swatch {
  width: 22px;
  height: 22px;
  border: 1px solid #000000;
  cursor: pointer;
  box-shadow: 0 0 2px rgba(0, 0, 0, 0.5);
}

.font-picker-row {
  display: flex;
  align-items: center;
  gap: 6px;
}

.pref-empty-note {
  font-size: 12px;
  color: var(--cs-text-dim);
  padding: 12px;
  background: var(--cs-bg);
  border: 1px solid var(--cs-border);
}

.cs-preferences-footer {
  display: flex;
  justify-content: center;
  padding: 8px 0;
}

/* 主题强调色预设 */
.color-presets {
  display: flex;
  align-items: center;
  gap: 8px;
}

.color-swatch {
  width: 22px;
  height: 22px;
  border-radius: 3px;
  border: 2px solid transparent;
  cursor: pointer;
  padding: 0;
  transition: transform 100ms ease, border-color 100ms ease;
}

.color-swatch:hover {
  transform: scale(1.12);
}

.color-swatch.active {
  border-color: var(--cs-text-bright);
  box-shadow: 0 0 0 1px var(--cs-border);
}
</style>
