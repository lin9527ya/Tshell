<template>
  <div class="cs-error-page cs-font-ui">
    <div class="cs-error-content">
      <el-icon class="error-icon"><Lock /></el-icon>
      <div class="error-code">401</div>
      <div class="error-desc">Authentication required or session has expired on the Team Server.</div>
      <button class="cs-btn cs-btn-primary" @click="router.push('/login')">
        Connect to Team Server
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import { Lock } from '@element-plus/icons-vue'

const router = useRouter()
</script>

<style scoped>
.cs-error-page {
  display: flex;
  justify-content: center;
  align-items: center;
  width: 100vw;
  height: 100vh;
  background: var(--cs-bg);
}

.cs-error-content {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  padding: 0 24px;
  max-width: 520px;
}

.error-icon {
  font-size: 72px;
  color: var(--cs-text-dim);
  margin-bottom: 16px;
}

.error-code {
  font-size: 40px;
  font-weight: 600;
  color: var(--cs-text-bright);
  line-height: 1.2;
  font-family: var(--cs-font-mono);
}

.error-desc {
  font-size: 14px;
  color: var(--cs-text-dim);
  line-height: 1.6;
  margin: 12px 0 24px;
}
</style>
