<template>
  <div class="cs-table-tools">
    <button
      class="cs-icon-btn"
      :class="{ 'is-spinning': loading }"
      :title="i18nStore.tl('刷新', 'Refresh')"
      :disabled="loading"
      @click="emit('refresh')"
    >
      <VsIcon name="sync" :size="14" />
    </button>

    <el-dropdown trigger="click" @command="onDensity">
      <button class="cs-icon-btn" :title="i18nStore.tl('密度', 'Density')">
        <VsIcon name="list-ul" :size="14" />
      </button>
      <template #dropdown>
        <el-dropdown-menu>
          <el-dropdown-item
            v-for="d in DENSITIES"
            :key="d.value"
            :command="d.value"
            :class="{ 'is-active': density === d.value }"
          >
            {{ i18nStore.tl(d.zh, d.en) }}
          </el-dropdown-item>
        </el-dropdown-menu>
      </template>
    </el-dropdown>

    <el-popover trigger="click" placement="bottom-end" :width="190">
      <template #reference>
        <button class="cs-icon-btn" :title="i18nStore.tl('列设置', 'Columns')">
          <VsIcon name="columns" :size="14" />
        </button>
      </template>
      <div class="cs-col-settings">
        <el-checkbox
          v-for="c in columns"
          :key="c.key"
          :model-value="visible[c.key] !== false"
          :label="c.label"
          @update:model-value="(v: any) => emit('update:visible', { ...visible, [c.key]: !!v })"
        />
      </div>
    </el-popover>
  </div>
</template>

<script setup lang="ts">
import VsIcon from '@/layout-vshell/VsIcon.vue'
import { useI18nStore } from '@/stores/i18n'

export type Density = 'default' | 'compact' | 'mini'

const DENSITIES = [
  { value: 'default', zh: '默认', en: 'Default' },
  { value: 'compact', zh: '紧凑', en: 'Compact' },
  { value: 'mini', zh: '迷你', en: 'Mini' },
] as const

withDefaults(
  defineProps<{
    columns: { key: string; label: string }[]
    visible: Record<string, boolean>
    density: Density
    loading?: boolean
  }>(),
  { loading: false },
)

const emit = defineEmits<{
  (e: 'refresh'): void
  (e: 'update:visible', value: Record<string, boolean>): void
  (e: 'update:density', value: Density): void
}>()

const i18nStore = useI18nStore()

function onDensity(cmd: string) {
  if (cmd === 'default' || cmd === 'compact' || cmd === 'mini') {
    emit('update:density', cmd)
  }
}
</script>

<style scoped>
.cs-col-settings {
  display: flex;
  flex-direction: column;
  max-height: 320px;
  overflow-y: auto;
}

.cs-col-settings :deep(.el-checkbox) {
  height: 28px;
  margin-right: 0;
}
</style>
