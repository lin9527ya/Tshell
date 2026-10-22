<template>
  <div v-if="!editing" class="cs-note-cell" @click.stop="startEdit">
    <span class="cs-cell-note" :title="value">{{ value }}</span>
    <VsIcon name="pen" :size="11" class="cs-note-edit" :title="i18nStore.tl('编辑备注', 'Edit note')" />
  </div>
  <input
    v-else
    ref="inputRef"
    v-model="draft"
    class="cs-note-input"
    :placeholder="placeholder || i18nStore.tl('输入备注…', 'Add a note…')"
    spellcheck="false"
    @click.stop
    @keydown.enter.prevent="commit"
    @keydown.esc.prevent="cancel"
    @blur="commit"
  />
</template>

<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import VsIcon from '@/layout-vshell/VsIcon.vue'
import { useI18nStore } from '@/stores/i18n'

const props = defineProps<{
  value: string
  placeholder?: string
}>()

const emit = defineEmits<{ (e: 'save', value: string): void }>()

const i18nStore = useI18nStore()
const editing = ref(false)
const draft = ref('')
const inputRef = ref<HTMLInputElement | null>(null)

function startEdit() {
  draft.value = props.value || ''
  editing.value = true
  nextTick(() => {
    inputRef.value?.focus()
    inputRef.value?.select()
  })
}

function commit() {
  if (!editing.value) return
  const next = draft.value.trim()
  editing.value = false
  if (next !== (props.value || '')) emit('save', next)
}

function cancel() {
  editing.value = false
}

watch(
  () => props.value,
  (v) => {
    if (!editing.value) draft.value = v || ''
  },
)
</script>
