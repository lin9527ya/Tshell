<template>
  <el-dialog
    v-model="visible"
    :title="isZh ? 'C2 扩展配置画像编辑器' : 'Malleable C2 Profile Editor'"
    width="720px"
    :close-on-click-modal="false"
    append-to-body
    class="cs-dialog"
  >
    <div class="cs-profile-container cs-font-ui">
      <!-- Toolbar -->
      <div class="profile-toolbar">
        <span class="profile-title">{{ isZh ? '画像: ' : 'Profile: ' }}</span>
        <el-select v-model="profileName" size="small" style="width: 160px" @visible-change="onProfileListOpen">
          <el-option v-for="n in profileNames" :key="n" :label="n" :value="n" />
        </el-select>
        <div class="profile-spacer" />
        <button class="cs-btn" @click="loadProfile">{{ isZh ? '加载' : 'Load' }}</button>
        <button class="cs-btn" @click="testProfile">{{ isZh ? '语法校验' : 'Lint / Verify' }}</button>
      </div>

      <!-- Editor Textarea -->
      <div class="profile-editor-wrap">
        <textarea
          v-model="profileContent"
          class="cs-profile-textarea cs-font-mono"
          spellcheck="false"
          :placeholder="isZh ? '# BladeShell C2 扩展通信画像配置...' : '# Cobalt Strike Malleable C2 Profile definition...'"
        />
      </div>
    </div>

    <template #footer>
      <div class="cs-dialog-footer">
        <button class="cs-btn cs-btn-primary" @click="saveProfile">{{ isZh ? '保存画像' : 'Save Profile' }}</button>
        <button class="cs-btn" @click="close">{{ isZh ? '关闭' : 'Close' }}</button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useWorkspaceStore } from '@/stores/workspace'
import { useI18nStore } from '@/stores/i18n'

const workspace = useWorkspaceStore()
const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const visible = computed({
  get: () => workspace.dialogs.c2Profile,
  set: (val) => {
    if (!val) workspace.closeDialog('c2Profile')
  },
})

const defaultC2 = `# ============================================================
# Tshell Malleable C2 Profile（YAML 格式，仿 CS profile 概念）
# 修改保存后 3 秒内自动热加载（仅影响新生成的客户端与新建监听器）
# ============================================================

name: default
description: "默认画像：保留腾讯系接口伪装路径"

# 网络请求 User-Agent（消除 Go-http-client/1.1 特征）
useragent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

# 心跳基准（毫秒，1-60000）
sleeptime_ms: 1000

# 心跳抖动百分比（0-50）
jitter_percent: 20

# HTTP 传输伪装
http:
  get:
    uri: "/tencent/mcp/pc/pcsearch"
    metadata_cookie_prepend: "BDUSS="
  post:
    uri: "/tencent/sensearch/collection/item/check"

# WebSocket 路径
ws:
  path: "/ws"

# TLS 指纹（chrome_120 / firefox_121 / edge_120 / safari_17）
tls:
  ja3: "chrome_120"

# DNS 传输特征
dns:
  beacon_prefix: "update"
  query_type: "TXT"
`

const profileContent = ref(defaultC2)

async function loadDefaultProfile() {
  try {
    const res = await c2profileApi.previewProfile('default')
    if (res?.data?.status === 200 && typeof res.data.data === 'string') {
      profileContent.value = res.data.data
    } else {
      profileContent.value = defaultC2
    }
  } catch {
    profileContent.value = defaultC2
  }
  profileName.value = 'default'
  ElMessage.info(isZh.value ? '已加载默认画像' : 'Loaded default profile')
}

function testProfile() {
  // YAML 语法在保存时由后端校验（yaml.Unmarshal 失败会拒绝保存并提示错误位置）
  if (!profileContent.value.trim()) {
    ElMessage.warning(isZh.value ? '画像内容为空' : 'Profile content is empty')
    return
  }
  ElMessage.info(isZh.value ? '语法在保存时由后端校验（YAML）' : 'Syntax is validated on save (YAML)')
}

const profileName = ref('default')
const profileNames = ref<string[]>(['default'])

async function onProfileListOpen(v?: boolean) {
  if (v === false) return
  try {
    const res = await c2profileApi.listProfiles()
    profileNames.value = (res?.data?.data || []).map((p: any) => p.name)
  } catch { /* 保持默认 */ }
}

async function loadProfile() {
  try {
    const res = await c2profileApi.previewProfile(profileName.value)
    if (res?.data?.status === 200 && typeof res.data.data === 'string') {
      profileContent.value = res.data.data
      ElMessage.success(isZh.value ? `已加载画像 ${profileName.value}` : `Loaded profile ${profileName.value}`)
    }
  } catch {
    ElMessage.error(isZh.value ? '加载画像失败' : 'Failed to load profile')
  }
}

async function saveProfile() {
  if (!profileContent.value.trim()) {
    ElMessage.warning(isZh.value ? '画像内容为空' : 'Profile content is empty')
    return
  }
  try {
    const res = await c2profileApi.saveProfile(profileName.value, profileContent.value)
    if (res?.data?.status === 200) {
      ElMessage.success(isZh.value ? `画像 ${profileName.value} 已保存（3 秒内热加载）` : `Profile ${profileName.value} saved (hot-reload in 3s)`)
    } else {
      ElMessage.error(res?.data?.message || '保存失败')
    }
  } catch (e: any) {
    ElMessage.error(isZh.value ? '保存失败: ' + (e?.message || '') : 'Save failed: ' + (e?.message || ''))
  }
}

function close() {
  workspace.closeDialog('c2Profile')
}
</script>

<style scoped>
.cs-profile-container {
  display: flex;
  flex-direction: column;
  height: 420px;
  gap: 8px;
}

.profile-toolbar {
  height: 30px;
  display: flex;
  align-items: center;
  gap: 8px;
  background: var(--cs-bg);
  padding: 0 8px;
  border: 1px solid var(--cs-border);
}

.profile-title {
  font-size: 12px;
  color: var(--cs-text-bright);
}

.profile-spacer {
  flex: 1;
}

.profile-editor-wrap {
  flex: 1;
  overflow: hidden;
}

.cs-profile-textarea {
  width: 100%;
  height: 100%;
  background: #141414;
  color: #e0e0e0;
  border: 1px solid var(--cs-border);
  padding: 10px;
  font-size: 12px;
  line-height: 1.5;
  resize: none;
  outline: none;
  font-family: var(--cs-font-mono);
  box-sizing: border-box;
}

.cs-profile-textarea:focus {
  border-color: #185abc;
}
</style>
