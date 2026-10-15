<script setup lang="ts">
// vshell 式「主题配置」抽屉（替换原 CS 式偏好设置弹窗）。
// 四项主题设置对齐还原版：主题(明暗) / 导航栏模式 / 顶栏主题 / 菜单主题；
// 另保留两项真正生效的控制台偏好（回滚行数、上线提示音）。
import { computed } from 'vue'
import { Check } from '@element-plus/icons-vue'
import VsIcon from './VsIcon.vue'
import { HEADER_COLORS, MENU_COLORS, useThemeStore, type NavMode } from '@/stores/theme'
import { useI18nStore } from '@/stores/i18n'
import { usePreferencesStore } from '@/stores/preferences'

const props = defineProps<{ modelValue: boolean }>()
const emit = defineEmits<{ (e: 'update:modelValue', val: boolean): void }>()

const theme = useThemeStore()
const i18nStore = useI18nStore()
const prefStore = usePreferencesStore()
const isZh = computed(() => i18nStore.isZh)

const visible = computed({
  get: () => props.modelValue,
  set: (val) => emit('update:modelValue', val),
})

// 顺序与命名对齐 vshell 主题抽屉（左侧菜单 / 顶部菜单混合 / 顶部菜单 / 左侧菜单混合）
const NAV_MODES: { key: NavMode; zh: string; en: string }[] = [
  { key: 'sidebar', zh: '左侧菜单模式', en: 'Left menu' },
  { key: 'mix', zh: '顶部菜单混合模式', en: 'Top mix' },
  { key: 'top-menu', zh: '顶部菜单模式', en: 'Top menu' },
  { key: 'mix-sidebar', zh: '左侧菜单混合模式', en: 'Left mix' },
]
</script>

<template>
  <el-drawer
    v-model="visible"
    :title="isZh ? '主题配置' : 'Theme Configuration'"
    direction="rtl"
    size="360px"
    append-to-body
    class="td-drawer"
  >
    <div class="td-body cs-font-ui">
      <!-- 主题 -->
      <div class="td-sec">{{ isZh ? '主题' : 'Theme' }}</div>
      <div class="td-center">
        <div class="td-dark-switch" :class="{ 'is-dark': theme.isDark }" @click="theme.toggleTheme()">
          <span class="td-knob"><VsIcon :name="theme.isDark ? 'moon' : 'sun'" :size="12" /></span>
        </div>
      </div>

      <!-- 导航栏模式 -->
      <div class="td-sec">{{ isZh ? '导航栏模式' : 'Navigation mode' }}</div>
      <div class="td-center td-navmodes">
        <div
          v-for="m in NAV_MODES"
          :key="m.key"
          class="td-navmode"
          :class="{ active: theme.config.navMode === m.key }"
          :title="isZh ? m.zh : m.en"
          @click="theme.setNavMode(m.key)"
        >
          <!-- 缩略图：四种布局示意（侧栏整高 / 顶栏整宽 / 图标栏 / 无侧栏） -->
          <div class="td-mock" :class="'is-' + m.key">
            <template v-if="m.key === 'sidebar' || m.key === 'mix-sidebar'">
              <div class="td-m-side" :class="{ rail: m.key === 'mix-sidebar' }" />
              <div class="td-m-right">
                <div class="td-m-head" />
                <div class="td-m-body" />
              </div>
            </template>
            <template v-else-if="m.key === 'mix'">
              <div class="td-m-head full" />
              <div class="td-m-row">
                <div class="td-m-side" />
                <div class="td-m-body" />
              </div>
            </template>
            <template v-else>
              <div class="td-m-head full menu" />
              <div class="td-m-body" />
            </template>
          </div>
        </div>
      </div>

      <!-- 顶栏主题 -->
      <div class="td-sec">{{ isZh ? '顶栏主题' : 'Header theme' }}</div>
      <div class="td-swatches">
        <button
          v-for="c in HEADER_COLORS"
          :key="c"
          class="td-swatch"
          :class="{ active: theme.config.headerColor.toLowerCase() === c }"
          :style="{ background: c }"
          :title="c"
          @click="theme.setHeaderColor(c)"
        >
          <el-icon v-if="theme.config.headerColor.toLowerCase() === c" :size="12"><Check /></el-icon>
        </button>
      </div>

      <!-- 菜单主题 -->
      <div class="td-sec">{{ isZh ? '菜单主题' : 'Menu theme' }}</div>
      <div class="td-swatches">
        <button
          v-for="c in MENU_COLORS"
          :key="c"
          class="td-swatch"
          :class="{ active: theme.config.menuColor.toLowerCase() === c }"
          :style="{ background: c }"
          :title="c"
          @click="theme.setMenuColor(c)"
        />
      </div>
      <div class="td-hint">
        {{ isZh ? '菜单主题作用于「左侧菜单」模式下的侧边导航' : 'Applies to the sidebar in side-menu mode' }}
      </div>

      <!-- 控制台 -->
      <div class="td-sec">{{ isZh ? '控制台' : 'Console' }}</div>
      <div class="td-field">
        <label>{{ isZh ? '回滚行数' : 'Scrollback lines' }}</label>
        <el-input-number
          v-model="prefStore.scrollback"
          :min="1000"
          :max="50000"
          :step="1000"
          size="small"
        />
      </div>
      <div class="td-field">
        <label>{{ isZh ? '上线提示音' : 'Online chime' }}</label>
        <el-switch v-model="prefStore.audioNotify" />
      </div>
    </div>
  </el-drawer>
</template>

<style scoped>
.td-body {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding-bottom: 24px;
}

/* 分组标题：vshell 抽屉同款「居中分隔线 + 文字」 */
.td-sec {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 14px 0 6px;
  font-size: 14px;
  color: var(--cs-text);
  white-space: nowrap;
}

.td-sec::before,
.td-sec::after {
  content: '';
  flex: 1;
  height: 1px;
  background: var(--cs-card-border);
}

.td-center {
  display: flex;
  justify-content: center;
}

/* 明暗开关（vshell 的 sun/moon 药丸） */
.td-dark-switch {
  width: 44px;
  height: 22px;
  border-radius: 11px;
  background: var(--cs-border);
  position: relative;
  cursor: pointer;
  transition: background 0.2s;
}

.td-dark-switch.is-dark {
  background: #0960bd;
}

.td-knob {
  position: absolute;
  top: 2px;
  left: 2px;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: #fff;
  color: #666;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: transform 0.2s;
}

.td-dark-switch.is-dark .td-knob {
  transform: translateX(22px);
  color: #0960bd;
}

/* 导航栏模式缩略图 */
.td-navmodes {
  gap: 12px;
}

.td-navmode {
  width: 64px;
  height: 48px;
  padding: 3px;
  border: 1px solid var(--cs-border);
  border-radius: 4px;
  cursor: pointer;
  transition: border-color 0.15s;
}

.td-navmode:hover,
.td-navmode.active {
  border-color: var(--cs-active, #0960bd);
}

.td-mock {
  width: 100%;
  height: 100%;
  display: flex;
  background: #fff;
  border-radius: 2px;
  overflow: hidden;
}

/* 顶栏（浅色条；menu 变体表示顶栏里带菜单项） */
.td-m-head {
  height: 9px;
  flex-shrink: 0;
  background: #e8eef7;
  border-bottom: 1px solid #d5dfeb;
}

.td-m-head.full {
  width: 100%;
}

.td-m-head.menu {
  background: linear-gradient(90deg, #0960bd 0 22%, #e8eef7 22% 100%);
}

/* 侧栏（菜单底色） */
.td-m-side {
  width: 16px;
  flex-shrink: 0;
  background: #001529;
}

.td-m-side.rail {
  width: 9px;
}

.td-m-right,
.td-m-row {
  flex: 1;
  min-width: 0;
  display: flex;
}

.td-m-right {
  flex-direction: column;
}

.td-m-body {
  flex: 1;
  background: #f7f8fa;
}

/* 色板 */
.td-swatches {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  padding: 0 2px;
}

.td-swatch {
  width: 20px;
  height: 20px;
  padding: 0;
  border: 1px solid rgba(0, 0, 0, 0.12);
  border-radius: 2px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 12px;
  line-height: 1;
}

.td-swatch.active {
  box-shadow: 0 0 0 2px var(--cs-active, #0960bd);
}

.td-hint {
  margin-top: 6px;
  font-size: 12px;
  color: var(--cs-text-dim);
}

.td-field {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 6px 2px;
  font-size: 14px;
  color: var(--cs-text);
}
</style>
