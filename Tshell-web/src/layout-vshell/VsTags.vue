<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
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
const folded = ref(false)

const ctx = ref<{ visible: boolean; path: string; x: number; y: number }>({
  visible: false,
  path: '',
  x: 0,
  y: 0,
})

function onTabClick(path: string) {
  ctx.value.visible = false
  moreOpen.value = false
  if (path !== tabs.activePath) router.push(path)
}

// ---- 横向滚动与溢出菜单 ----
const scrollRef = ref<HTMLElement | null>(null)
const overflowing = ref(false)
const moreOpen = ref(false)

// 纵向滚轮 → 横向滚动（标签条隐藏了滚动条，否则多标签时右侧不可达）
function onWheel(e: WheelEvent) {
  const el = scrollRef.value
  if (!el) return
  if (Math.abs(e.deltaY) <= Math.abs(e.deltaX)) return
  el.scrollLeft += e.deltaY
  e.preventDefault()
}

function updateOverflow() {
  const el = scrollRef.value
  if (!el) return
  overflowing.value = el.scrollWidth > el.clientWidth + 1
}

function scrollToTab(path: string) {
  moreOpen.value = false
  if (path !== tabs.activePath) router.push(path)
  nextTick(() => {
    const el = scrollRef.value?.querySelector(`[data-path="${CSS.escape(path)}"]`) as HTMLElement | null
    el?.scrollIntoView({ inline: 'nearest', block: 'nearest' })
  })
}

watch(() => tabs.visited.length, () => nextTick(updateOverflow))
onMounted(() => {
  nextTick(updateOverflow)
  window.addEventListener('resize', updateOverflow)
})
onBeforeUnmount(() => window.removeEventListener('resize', updateOverflow))

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
  <div class="vs-tags" @click="ctx.visible = false; moreOpen = false">
    <div ref="scrollRef" class="vs-tags-scroll" @wheel="onWheel" @scroll="updateOverflow">
      <div
        v-for="tab in tabs.visited"
        :key="tab.path"
        class="vs-tag"
        :class="{ active: tab.path === tabs.activePath }"
        :title="tab.title"
        :data-path="tab.path"
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
      <!-- 溢出时列出全部标签（点击即跳转并滚动到可见） -->
      <div v-if="overflowing" class="vs-tags-more-wrap">
        <button class="vs-tags-btn" :title="i18nStore.tl('全部标签', 'All tabs')" @click.stop="moreOpen = !moreOpen">
          <VsIcon name="ellipsis-v" :size="14" />
        </button>
        <div v-if="moreOpen" class="vs-nav-popup vs-tags-more" @click.stop>
          <div
            v-for="tab in tabs.visited"
            :key="'more-' + tab.path"
            class="vs-popup-item"
            :class="{ active: tab.path === tabs.activePath }"
            @click="scrollToTab(tab.path)"
          >
            <VsIcon v-if="tab.icon" :name="tab.icon" :size="14" />
            <span>{{ tab.title }}</span>
          </div>
        </div>
      </div>
      <button class="vs-tags-btn" :title="i18nStore.tl('刷新当前页', 'Refresh')" @click="onRefresh">
        <VsIcon name="redo" :size="14" />
      </button>
      <button
        class="vs-tags-btn"
        :title="folded ? i18nStore.tl('展开页头', 'Expand header') : i18nStore.tl('折叠页头', 'Collapse header')"
        @click="toggleFold"
      >
        <VsIcon :name="folded ? 'expand' : 'compress'" :size="14" />
      </button>
    </div>

    <!-- 右键菜单就地渲染(不 teleport: 出 .cs-shell 后 --vs-*/--cs-* 令牌失效) -->
    <div
      v-if="ctx.visible"
      class="vs-ctx-menu"
      :style="{ left: ctx.x + 'px', top: ctx.y + 'px' }"
      @click.stop
    >
      <div class="vs-popup-item" @click="onRefresh"><VsIcon name="redo" :size="14" /><span>{{ i18nStore.tl('刷新', 'Refresh') }}</span></div>
      <div class="vs-popup-item" @click="closeCtxTab"><VsIcon name="times" :size="14" /><span>{{ i18nStore.tl('关闭', 'Close') }}</span></div>
      <div class="vs-popup-item" @click="closeOthers"><VsIcon name="columns" :size="14" /><span>{{ i18nStore.tl('关闭其他', 'Close others') }}</span></div>
      <div class="vs-popup-item" @click="closeRight"><VsIcon name="arrow-up" :size="14" /><span>{{ i18nStore.tl('关闭右侧', 'Close right') }}</span></div>
      <div class="vs-popup-divider" />
      <div class="vs-popup-item" @click="closeAll"><VsIcon name="trash-alt" :size="14" /><span>{{ i18nStore.tl('全部关闭', 'Close all') }}</span></div>
    </div>
  </div>
</template>

<style scoped>

.vs-tags-more-wrap {
  position: relative;
  height: 100%;
  display: flex;
  align-items: center;
}

.vs-tags-more {
  left: auto;
  right: 0;
  min-width: 180px;
  max-height: 60vh;
  overflow-y: auto;
}
</style>
