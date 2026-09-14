<template>
  <el-dialog
    v-model="visible"
    :title="isZh ? '设置会话标记颜色' : 'Set Session Color'"
    width="380px"
    :close-on-click-modal="false"
    append-to-body
    class="cs-dialog"
  >
    <div class="cs-dialog-body">
      <p class="dialog-desc">{{ isZh ? '为选中的会话选择高亮标记颜色：' : 'Choose a highlight color for the selected session:' }}</p>
      <div class="color-grid">
        <button
          v-for="c in computedColors"
          :key="c.name"
          class="color-chip"
          :class="{ active: currentColor === c.value }"
          :style="{ background: c.bg, borderColor: c.border }"
          @click="selectColor(c.value)"
        >
          <span class="chip-label" :style="{ color: c.text }">{{ c.label }}</span>
        </button>
      </div>
    </div>
    <template #footer>
      <div class="cs-dialog-footer">
        <button class="cs-btn" @click="resetColor">{{ isZh ? '清除颜色' : 'Clear Color' }}</button>
        <div class="footer-spacer" />
        <button class="cs-btn cs-btn-primary" @click="close">{{ isZh ? '完成' : 'Done' }}</button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { ElMessage } from 'element-plus'
import { useWorkspaceStore } from '@/stores/workspace'
import { useI18nStore } from '@/stores/i18n'
import ClientAPI from '@/api/clients'

const workspace = useWorkspaceStore()
const i18nStore = useI18nStore()
const isZh = computed(() => i18nStore.isZh)

const visible = computed({
  get: () => workspace.dialogs.color.visible,
  set: (val) => {
    if (!val) workspace.closeDialog('color')
  },
})

const uid = computed(() => workspace.dialogs.color.uid)
const currentColor = computed(() => workspace.sessionColors[uid.value] || '')

const colors = [
  { name: 'Red', zhName: '红色', value: '#d32f2f', bg: '#d32f2f22', border: '#d32f2f', text: '#ef5350' },
  { name: 'Orange', zhName: '橙色', value: '#e65100', bg: '#e6510022', border: '#e65100', text: '#ff9800' },
  { name: 'Yellow', zhName: '黄色', value: '#f57f17', bg: '#f57f1722', border: '#f57f17', text: '#ffee58' },
  { name: 'Green', zhName: '绿色', value: '#2e7d32', bg: '#2e7d3222', border: '#2e7d32', text: '#66bb6a' },
  { name: 'Cyan', zhName: '青色', value: '#00838f', bg: '#00838f22', border: '#00838f', text: '#26c6da' },
  { name: 'Blue', zhName: '蓝色', value: '#1565c0', bg: '#1565c022', border: '#1565c0', text: '#42a5f5' },
  { name: 'Purple', zhName: '紫色', value: '#6a1b9a', bg: '#6a1b9a22', border: '#6a1b9a', text: '#ab47bc' },
  { name: 'Default', zhName: '默认', value: '', bg: 'transparent', border: 'var(--cs-border)', text: 'var(--cs-text)' },
]

const computedColors = computed(() =>
  colors.map((c) => ({
    ...c,
    label: isZh.value ? c.zhName : c.name,
  }))
)

async function applyColor(val: string) {
  const targetUids = workspace.selectedUids.length ? workspace.selectedUids : (uid.value ? [uid.value] : [])
  if (!targetUids.length) return
  // 同步后端 /client/color（与右键调色板行为一致），失败真实报错
  let failed = false
  for (const u of targetUids) {
    try {
      await ClientAPI.select_color({ uid: u, color: val })
      workspace.setSessionColor(u, val)
    } catch (e: any) {
      failed = true
      ElMessage.error(e?.message || (isZh.value ? '设置颜色失败' : 'Failed to set color'))
    }
  }
  if (!failed && !val) close()
}

function selectColor(val: string) {
  applyColor(val)
}

function resetColor() {
  applyColor('')
}

function close() {
  workspace.closeDialog('color')
}
</script>

<style scoped>
.cs-dialog-body {
  padding: 8px 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.dialog-desc {
  font-size: 12px;
  color: var(--cs-text-dim);
}
.color-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 8px;
}
.color-chip {
  height: 36px;
  border-width: 1px;
  border-style: solid;
  border-radius: 3px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: transform 120ms ease;
}
.color-chip:hover {
  transform: scale(1.04);
}
.color-chip.active {
  box-shadow: 0 0 0 2px var(--cs-active);
  font-weight: bold;
}
.chip-label {
  font-size: 11px;
  font-weight: 500;
}
.cs-dialog-footer {
  display: flex;
  align-items: center;
}
.footer-spacer { flex: 1; }
</style>
