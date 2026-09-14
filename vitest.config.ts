import { fileURLToPath, URL } from 'node:url'

import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

// 独立 vitest 配置（不 merge vite.config.ts：其为 async callback 形式，
// vitest 1.x mergeConfig 不支持 callback 配置）。
// 单测仅需要 vue 插件 + jsdom 环境，无需 unocss/自动导入。
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url))
    }
  },
  test: {
    environment: 'jsdom',
    exclude: ['node_modules/**', 'dist/**', 'e2e/*']
  }
})
