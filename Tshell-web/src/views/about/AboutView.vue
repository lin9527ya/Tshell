<template>
  <div class="ab-page">
    <div class="ab-card">
      <div class="ab-header">
        <div class="ab-logo">⚡</div>
        <div class="ab-meta">
          <div class="ab-name">Tshell</div>
          <div class="ab-sub">{{ i18nStore.tl('跨平台协同指挥与控制平台（开发版）', 'Tshell C2 Multi-Platform Edition (Dev Build)') }}</div>
        </div>
      </div>

      <div class="ab-body">
        <div class="ab-row">
          <span class="row-k">{{ i18nStore.tl('许可证授权', 'License') }}</span>
          <span class="row-v">{{ i18nStore.tl('开发版（无授权声明）', 'Dev build (no license claim)') }}</span>
        </div>
        <div class="ab-row">
          <span class="row-k">{{ i18nStore.tl('团队服务器', 'TeamServer') }}</span>
          <span class="row-v">{{ hostInfo }}</span>
        </div>
        <div class="ab-row">
          <span class="row-k">{{ i18nStore.tl('操作员', 'Operator') }}</span>
          <span class="row-v">{{ userStore.username || 'admin' }} ({{ userStore.roles || 'SuperAdmin' }})</span>
        </div>
        <div class="ab-row">
          <span class="row-k">{{ i18nStore.tl('支持协议', 'Supported Protocols') }}</span>
          <span class="row-v">TCP, WebSocket, HTTP, HTTPS, KCP, OSS, DNS</span>
        </div>
        <div class="ab-row">
          <span class="row-k">{{ i18nStore.tl('功能模块', 'Modules') }}</span>
          <span class="row-v">{{ i18nStore.tl('Beacon 会话, 隧道代理, Web 投递, 进程注入, SOCKS5', 'Beacon, Tunnels, Web Delivery, Process Inject, SOCKS5') }}</span>
        </div>
      </div>

      <div class="ab-note">
        {{ i18nStore.tl('Tshell 专为红队战术推演、跨平台命令控制与实战攻防设计。', 'Tshell is a cross-platform command & control platform for adversary simulation and red team operations.') }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useUserStore } from '@/stores/user'
import { useI18nStore } from '@/stores/i18n'

defineOptions({ name: 'About' })

const userStore = useUserStore()
const i18nStore = useI18nStore()
const hostInfo = computed(() => `${window.location.protocol}//${window.location.host}`)
</script>

<style scoped>
.ab-page {
  height: 100%;
  padding: 16px;
  background: var(--cs-bg);
  overflow: auto;
  box-sizing: border-box;
  /* 纵向排布（页头在上），卡片自身水平居中，避免宽屏下右侧大片留白 */
  display: flex;
  flex-direction: column;
}

.ab-card {
  max-width: 720px;
  margin: 0 auto;
  width: 100%;
  background: var(--cs-panel);
  border: 1px solid var(--cs-card-border);
  border-radius: 8px;
  padding: 20px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.ab-header {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 12px;
  background: var(--cs-hover);
  border-radius: 8px;
}

.ab-logo {
  font-size: 28px;
  width: 44px;
  height: 44px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--cs-active);
  color: #fff;
  border-radius: 8px;
  flex-shrink: 0;
}

.ab-name {
  font-size: 16px;
  font-weight: 600;
  color: var(--cs-text-bright);
}

.ab-sub {
  font-size: 12px;
  color: var(--cs-text-dim);
  margin-top: 2px;
}

.ab-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
  font-size: 13px;
}

.ab-row {
  display: flex;
  align-items: baseline;
}

.row-k {
  width: 140px;
  flex-shrink: 0;
  color: var(--cs-text-dim);
}

.row-v {
  flex: 1;
  color: var(--cs-text-bright);
  font-family: var(--cs-font-mono);
}

.ab-note {
  font-size: 12px;
  color: var(--cs-text-dim);
  font-style: italic;
  padding-top: 4px;
  border-top: 1px solid var(--cs-card-border);
}
</style>
