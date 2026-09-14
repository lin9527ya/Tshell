<template>
  <div class="cs-error-page cs-font-ui">
    <div class="cs-error-window">
      <div class="cs-error-titlebar">
        <span>BladeShell - 404 Not Found</span>
      </div>
      <div class="cs-error-body">
        <div class="error-code cs-font-mono">404</div>
        <div class="error-msg">The requested view or resource was not found on this Team Server.</div>
        <div class="cs-error-buttons">
          <button class="cs-btn cs-btn-primary" @click="router.push('/')">
            Return to Console
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'

const router = useRouter()
</script>

<style scoped>
.cs-error-page {
  display: flex;
  justify-content: center;
  align-items: center;
  width: 100vw;
  height: 100vh;
  background: #111111;
}

.cs-error-window {
  width: 460px;
  background: var(--cs-dialog-bg);
  border: 1px solid var(--cs-border);
  box-shadow: 0 12px 36px rgba(0, 0, 0, 0.7);
}

.cs-error-titlebar {
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

.cs-error-body {
  padding: 32px 24px;
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
}

.error-code {
  font-size: 54px;
  font-weight: 700;
  color: #ff4d4f;
  line-height: 1;
}

.error-msg {
  font-size: 12px;
  color: var(--cs-text);
  margin-top: 14px;
}

.cs-error-buttons {
  margin-top: 24px;
}
</style>
