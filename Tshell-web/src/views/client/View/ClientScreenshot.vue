<template>
  <GlobalScreenshotsView :uid="clientUid" />
</template>

<script setup lang="ts">
import { computed, inject } from 'vue'
import type { ComputedRef, Ref } from 'vue'
import { useRoute } from 'vue-router'
import GlobalScreenshotsView from '@/layout/GlobalScreenshotsView.vue'
import { useClientUid } from '@/utils/clientUtils'

const props = withDefaults(
  defineProps<{
    uid?: string
  }>(),
  {
    uid: '',
  },
)

const route = useRoute()
const injectedUid = inject<Ref<string> | ComputedRef<string> | null>('realUid', null)

const clientUid = computed(() => {
  if (props.uid && props.uid.length > 5) return props.uid
  if (injectedUid && injectedUid.value && String(injectedUid.value).length > 5) {
    return String(injectedUid.value)
  }
  const mapped = useClientUid(route)
  return typeof mapped === 'string' && mapped.length > 5 ? mapped : ''
})
</script>
