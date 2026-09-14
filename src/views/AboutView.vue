<template>
  <div class="cs-about-view cs-font-ui">
    <div class="cs-about-window">
      <!-- Titlebar -->
      <div class="cs-about-titlebar">
        <span>{{ isZh ? '关于 BladeShell' : 'About BladeShell' }}</span>
      </div>

      <!-- Content -->
      <div class="cs-about-body">
        <div class="cs-about-logo">
          <svg viewBox="0 0 64 64" width="56" height="56">
            <polygon points="32,4 58,16 58,46 32,60 6,16" fill="none" stroke="#69b1ff" stroke-width="3" />
            <polygon points="32,10 52,20 52,42 32,54 12,42 12,20" fill="#177ddc" fill-opacity="0.2" stroke="#177ddc" stroke-width="1.5" />
            <path d="M32 14v36M18 24l28 16M46 24L18 40" stroke="#69b1ff" stroke-width="2" stroke-linecap="round" />
          </svg>
        </div>

        <div class="cs-about-info">
          <h2 class="app-title">BladeShell</h2>
          <div class="app-tagline">{{ isZh ? '高级跨平台红队命令与控制平台' : 'Advanced Cross-Platform Command & Control Platform' }}</div>
          <div class="app-version cs-font-mono">{{ isZh ? '版本 4.9.1 (发布日期 2026.08.15)' : 'Version 4.9.1 (Release 2026.08.15)' }}</div>
          
          <div class="cs-about-desc">
            {{ isZh ? 'BladeShell 提供全方位的红队命令与控制、团队协同、高隐蔽通信画像、隧道穿透及自动化后渗透能力。' : 'BladeShell provides full-spectrum red team command and control, team server collaboration, malleable C2 profiles, covert pivot tunnels, and post-exploitation automation.' }}
          </div>

          <div class="cs-about-grid cs-font-mono">
            <div class="grid-row">
              <span class="grid-label">{{ isZh ? '构建时间:' : 'Build Date:' }}</span>
              <span class="grid-val">{{ systemInfo.buildDate }}</span>
            </div>
            <div class="grid-row">
              <span class="grid-label">{{ isZh ? '服务端通信:' : 'Team Server API:' }}</span>
              <span class="grid-val">v4.9-grpc / ws-tunnel</span>
            </div>
            <div class="grid-row">
              <span class="grid-label">{{ isZh ? '客户端引擎:' : 'Engine Runtime:' }}</span>
              <span class="grid-val">Vue 3 + Vite + TypeScript (Linux/Windows x64)</span>
            </div>
            <div class="grid-row">
              <span class="grid-label">{{ isZh ? '版权所有:' : 'Copyright:' }}</span>
              <span class="grid-val">© 2026 BladeShell Development Team. All rights reserved.</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Footer Buttons -->
      <div class="cs-about-footer">
        <button class="cs-btn cs-btn-primary" @click="handleClose">
          {{ isZh ? '关闭' : 'Close' }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { useI18nStore } from '@/stores/i18n'

const router = useRouter()
const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const systemInfo = reactive({
  version: '4.9.1',
  buildDate: '2026-08-15 00:00:00',
  developer: 'BladeShell Development Team',
})

function handleClose() {
  router.push('/')
}
</script>

<style scoped>
.cs-about-view {
  height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #111111;
  padding: 20px;
}

.cs-about-window {
  width: 580px;
  background: var(--cs-dialog-bg);
  border: 1px solid var(--cs-border);
  box-shadow: 0 12px 36px rgba(0, 0, 0, 0.7);
  display: flex;
  flex-direction: column;
}

.cs-about-titlebar {
  height: 28px;
  background: var(--cs-titlebar-bg);
  border-bottom: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  padding: 0 12px;
  font-size: 12px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.cs-about-body {
  padding: 24px;
  display: flex;
  gap: 20px;
}

.cs-about-logo {
  flex-shrink: 0;
}

.cs-about-info {
  flex: 1;
  display: flex;
  flex-direction: column;
}

.app-title {
  margin: 0;
  font-size: 20px;
  font-weight: 700;
  color: var(--cs-text-bright);
}

.app-tagline {
  font-size: 12px;
  color: var(--cs-text-dim);
  margin-top: 2px;
}

.app-version {
  font-size: 11px;
  color: var(--cs-active);
  font-weight: 600;
  margin-top: 6px;
}

.cs-about-desc {
  font-size: 11px;
  line-height: 1.5;
  color: var(--cs-text);
  margin-top: 14px;
  padding-bottom: 14px;
  border-bottom: 1px solid var(--cs-border);
}

.cs-about-grid {
  margin-top: 12px;
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 11px;
}

.grid-row {
  display: flex;
  align-items: baseline;
  gap: 8px;
}

.grid-label {
  width: 130px;
  color: var(--cs-text-dim);
  flex-shrink: 0;
}

.grid-val {
  color: var(--cs-text-bright);
  word-break: break-all;
}

.cs-about-footer {
  height: 40px;
  background: var(--cs-panel);
  border-top: 1px solid var(--cs-border);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0 16px;
}
</style>