<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import VsIcon from './VsIcon.vue'
import { useI18nStore } from '@/stores/i18n'
import { useTabsStore } from '@/stores/tabs'

const emit = defineEmits<{
  (e: 'refresh'): void
  (e: 'fold', value: boolean): void
}>()

const i18nStore = useI18nStore()
const tabs = useTabsStore()
const router = useRouter()
const isZh = computed(() => i18nStore.isZh)
const folded = ref(false)

const ctx = ref<{ visible: boolean; path: string; x: number; y: number }>({
  visible: false,
  path: '',
  x: 0,
  y: 0,
})

function onTabClick(path: string) {
  ctx.value.visible = false
  if (path !== tabs.activePath) router.push(path)
}

function onClose(e: MouseEvent, path: string) {
  e.stopPropagation()
  const next = tabs.close(path)
  if (next !== tabs.activePath) router.push(next)
}

function toggleFold() {
  folded.value = !folded.value
  emit('fold', folded.value)
}

function onRefresh() {
  emit('refresh')
}

function onContextMenu(e: MouseEvent, path: string) {
  e.preventDefault()
  ctx.value = { visible: true, path, x: e.clientX, y: e.clientY }
}

function closeOthers() {
  tabs.closeOthers(ctx.value.path)
  ctx.value.visible = false
}

function closeRight() {
  tabs.closeRight(ctx.value.path)
  ctx.value.visible = false
}

function closeAll() {
  const next = tabs.closeAll()
  ctx.value.visible = false
  router.push(next)
}

function closeCtxTab() {
  const next = tabs.close(ctx.value.path)
  ctx.value.visible = false
  if (next !== tabs.activePath) router.push(next)
}
</script>

<template>
  <div class="vs-tags" @click="ctx.visible = false">
    <div class="vs-tags-scroll">
      <div
        v-for="tab in tabs.visited"
        :key="tab.path"
        class="vs-tag"
        :class="{ active: tab.path === tabs.activePath }"
        :title="tab.title"
        @click="onTabClick(tab.path)"
        @contextmenu="onContextMenu($event, tab.path)"
      >
        <VsIcon v-if="tab.icon" :name="tab.icon" :size="14" class="vs-tag-icon" />
        <span class="vs-tag-title">{{ tab.title }}</span>
        <span v-if="!tab.affix" class="vs-tag-close" @click="onClose($event, tab.path)">
          <VsIcon name="times" :size="12" />
        </span>
      </div>
    </div>

    <div class="vs-tags-extra">
      <button class="vs-tags-btn" :title="isZh ? '刷新当前页' : 'Refresh'" @click="onRefresh">
        <VsIcon name="redo" :size="14" />
      </button>
      <button class="vs-tags-btn" :title="isZh ? '折叠页头' : 'Collapse header'" @click="toggleFold">
        <VsIcon :name="folded ? 'window-restore' : 'expand'" :size="14" />
      </button>
    </div>

    <!-- 右键菜单就地渲染(不 teleport: 出 .cs-shell 后 --vs-*/--cs-* 令牌失效) -->
    <div
      v-if="ctx.visible"
      class="vs-ctx-menu"
      :style="{ left: ctx.x + 'px', top: ctx.y + 'px' }"
      @click.stop
    >
      <div class="vs-popup-item" @click="onRefresh"><VsIcon name="redo" :size="14" /><span>{{ isZh ? '刷新' : 'Refresh' }}</span></div>
      <div class="vs-popup-item" @click="closeCtxTab"><VsIcon name="times" :size="14" /><span>{{ isZh ? '关闭' : 'Close' }}</span></div>
      <div class="vs-popup-item" @click="closeOthers"><VsIcon name="columns" :size="14" /><span>{{ isZh ? '关闭其他' : 'Close others' }}</span></div>
      <div class="vs-popup-item" @click="closeRight"><VsIcon name="arrow-up" :size="14" /><span>{{ isZh ? '关闭右侧' : 'Close right' }}</span></div>
      <div class="vs-popup-divider" />
      <div class="vs-popup-item" @click="closeAll"><VsIcon name="trash-alt" :size="14" /><span>{{ isZh ? '全部关闭' : 'Close all' }}</span></div>
    </div>
  </div>
</template>
