<template>
  <el-dialog
    v-model="visible"
    :title="isZh ? '关于 BladeShell' : 'About Cobalt Strike / BladeShell'"
    width="500px"
    :close-on-click-modal="false"
    append-to-body
    class="cs-dialog"
  >
    <div class="about-container">
      <div class="about-header">
        <div class="about-logo">⚡</div>
        <div class="about-meta">
          <div class="about-name">BladeShell</div>
          <div class="about-sub">{{ isZh ? '跨平台协同指挥与控制平台（开发版）' : 'BladeShell C2 Multi-Platform Edition (Dev Build)' }}</div>
        </div>
      </div>

      <div class="about-body">
        <div class="about-row">
          <span class="row-k">{{ isZh ? '许可证授权:' : 'License:' }}</span>
          <span class="row-v">{{ isZh ? '开发版（无授权声明）' : 'Dev build (no license claim)' }}</span>
        </div>
        <div class="about-row">
          <span class="row-k">{{ isZh ? '团队服务器:' : 'TeamServer:' }}</span>
          <span class="row-v">{{ hostInfo }}</span>
        </div>
        <div class="about-row">
          <span class="row-k">{{ isZh ? '操作员:' : 'Operator:' }}</span>
          <span class="row-v">{{ userStore.username || 'admin' }} ({{ userStore.roles || 'SuperAdmin' }})</span>
        </div>
        <div class="about-row">
          <span class="row-k">{{ isZh ? '支持协议:' : 'Supported Protocols:' }}</span>
          <span class="row-v">TCP, WebSocket, HTTP, HTTPS, KCP, OSS, DNS</span>
        </div>
        <div class="about-row">
          <span class="row-k">{{ isZh ? '功能模块:' : 'Modules:' }}</span>
          <span class="row-v">{{ isZh ? 'Beacon 会话, 拓扑代理, Web 投递, 进程注入, SOCKS5' : 'Beacon, Pivots, Web Delivery, Process Inject, SOCKS5' }}</span>
        </div>
      </div>

      <div class="about-footer-note">
        {{ isZh ? 'BladeShell 专为红队战术推演、跨平台命令控制与实战攻防设计。' : 'Cobalt Strike is software for Adversary Simulations and Red Team Operations.' }}
      </div>
    </div>

    <template #footer>
      <div class="cs-dialog-footer">
        <button class="cs-btn cs-btn-primary" @click="close">{{ isZh ? '确定' : 'OK' }}</button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useWorkspaceStore } from '@/stores/workspace'
import { useUserStore } from '@/stores/user'
import { useI18nStore } from '@/stores/i18n'

const workspace = useWorkspaceStore()
const userStore = useUserStore()
const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const visible = computed({
  get: () => workspace.dialogs.about,
  set: (val) => {
    if (!val) workspace.closeDialog('about')
  },
})

const hostInfo = computed(() => {
  return `${window.location.protocol}//${window.location.host}`
})

function close() {
  workspace.closeDialog('about')
}
</script>

<style scoped>
.about-container {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 4px 0;
}
.about-header {
  display: flex;
  align-items: center;
  gap: 14px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--cs-border);
}
.about-logo {
  font-size: 32px;
  width: 48px;
  height: 48px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--cs-active);
  color: #fff;
  border-radius: 4px;
}
.about-name {
  font-size: 16px;
  font-weight: bold;
  color: var(--cs-text-bright);
}
.about-sub {
  font-size: 12px;
  color: var(--cs-text-dim);
  margin-top: 2px;
}
.about-body {
  display: flex;
  flex-direction: column;
  gap: 8px;
  font-size: 12px;
}
.about-row {
  display: flex;
}
.row-k {
  width: 140px;
  color: var(--cs-text-dim);
  font-weight: 500;
}
.row-v {
  flex: 1;
  color: var(--cs-text-bright);
  font-family: var(--cs-font-mono);
}
.status-ok {
  color: var(--cs-online);
}
.about-footer-note {
  font-size: 11px;
  color: var(--cs-text-dim);
  font-style: italic;
  padding-top: 8px;
  border-top: 1px solid var(--cs-border);
}
.cs-dialog-footer {
  display: flex;
  justify-content: flex-end;
}
</style>
