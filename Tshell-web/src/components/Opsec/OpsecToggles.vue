<!--
  OpsecToggles：生成期 OPSEC 开关面板（载荷生成 / 服务端生成页共用）。

  设计要点
  1. 每项都标注"层"：生成期（编译进占位符，运行期改不了）还是
     "生成期 + 运行期可切"（编译进载荷后还能用 evasion 命令热切换）。
     不标就会让人以为同一个开关在两边都能动。
  2. 不可用项置灰并给出原因（平台/架构/依赖），而不是隐藏——隐藏会让人
     以为"上次勾的东西被吞了"。
  3. 面板只负责模型；提交时走 opsecPresets.toGenServerBody()，保证
     "看到的 = 载荷里的"。
-->
<template>
  <div class="cs-opsec">
    <div v-if="showPresets && !readonly" class="cs-opsec-presets">
      <span class="cs-opsec-presets-label">{{ i18nStore.tl('预设', 'Preset') }}</span>
      <button
        v-for="p in OPSEC_PRESETS"
        :key="p.id"
        class="cs-btn cs-opsec-preset"
        :class="{ 'is-active': activePreset === p.id }"
        :title="i18nStore.tl(p.descZh, p.descEn)"
        @click="applyPreset(p)"
      >
        {{ i18nStore.tl(p.labelZh, p.labelEn) }}
      </button>
      <span class="cs-opsec-presets-hint">{{ i18nStore.tl('仅预填，不改变任何门控语义', 'prefills only, no behavior change') }}</span>
    </div>

    <div v-for="g in groups" :key="g.id" class="cs-opsec-group">
      <div class="cs-opsec-group-title">{{ i18nStore.tl(g.titleZh, g.titleEn) }}</div>

      <div v-for="cap in g.caps" :key="cap.key" class="cs-opsec-row">
        <label class="cs-opsec-label">
          {{ i18nStore.tl(cap.labelZh, cap.labelEn) }}
          <span class="cs-opsec-layer" :class="cap.layer === 'runtime' ? 'is-runtime' : 'is-build'">
            {{ cap.layer === 'runtime' ? i18nStore.tl('运行期可切', 'runtime switchable') : i18nStore.tl('生成期', 'build time') }}
          </span>
        </label>

        <div class="cs-opsec-control">
          <el-select
            v-if="cap.kind === 'enum'"
            :model-value="syscallMode"
            size="small"
            style="width: 190px"
            :disabled="readonly || disabledReason(cap) !== 'none'"
            @update:model-value="onSyscallMode"
          >
            <el-option
              v-for="m in SYSCALL_MODES"
              :key="m.value"
              :label="i18nStore.tl(m.labelZh, m.labelEn)"
              :value="m.value"
            />
          </el-select>

          <el-switch
            v-else
            :model-value="boolValue(cap.key)"
            size="small"
            :disabled="readonly || disabledReason(cap) !== 'none'"
            @update:model-value="(v: any) => onBool(cap.key, v)"
          />

          <span v-if="reasonText(cap)" class="cs-opsec-blocked">{{ reasonText(cap) }}</span>
          <span v-else-if="cap.hintZh" class="cs-opsec-hint" :class="{ 'is-warn': cap.risk === 'warn' }">
            {{ i18nStore.tl(cap.hintZh, cap.hintEn || cap.hintZh) }}
          </span>
        </div>
      </div>
    </div>

    <div v-if="readonly" class="cs-opsec-readonly">
      {{ i18nStore.tl('该流程固定使用最小对抗配置（不补丁 ETW/AMSI、不开伪栈/摘链），不可修改。', 'This flow is pinned to a minimal-evasion profile (no ETW/AMSI patch, no spoof/unlink) and cannot be changed.') }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18nStore } from '@/stores/i18n'
import {
  OPSEC_CAPABILITIES,
  OPSEC_PRESETS,
  SYSCALL_MODES,
  capabilityDisabledReason,
  defaultOpsecForm,
  type OpsecCapability,
  type OpsecForm,
  type OpsecKey,
  type OpsecPreset,
  type SyscallMode,
} from './opsecPresets'

const i18nStore = useI18nStore()

const props = withDefaults(
  defineProps<{
    modelValue: OpsecForm
    osType?: string
    archType?: string
    readonly?: boolean
    showPresets?: boolean
  }>(),
  {
    osType: 'windows',
    archType: 'amd64',
    readonly: false,
    showPresets: true,
  },
)

const emit = defineEmits<{
  (e: 'update:modelValue', v: OpsecForm): void
}>()

const syscallMode = computed<SyscallMode>(() => props.modelValue.syscallMode)

const groups = computed(() => {
  const titles: Record<string, { titleZh: string; titleEn: string }> = {
    evasion: { titleZh: '对抗与伪装', titleEn: 'Evasion & masquerading' },
    spawn: { titleZh: '子进程属性', titleEn: 'Child process attributes' },
    hygiene: { titleZh: '清理与残留', titleEn: 'Cleanup & residue' },
  }
  return ['evasion', 'spawn', 'hygiene']
    .map((id) => ({
      id,
      ...titles[id],
      caps: OPSEC_CAPABILITIES.filter((c) => c.group === id),
    }))
    .filter((g) => g.caps.length > 0)
})

// 当前模型恰好等于某个预设时高亮它（不存状态，避免和手改打架）
const activePreset = computed(() => {
  const hit = OPSEC_PRESETS.find((p) => JSON.stringify(p.form) === JSON.stringify(props.modelValue))
  return hit ? hit.id : ''
})

function boolValue(key: OpsecKey): boolean {
  return Boolean(props.modelValue[key])
}

function onBool(key: OpsecKey, v: boolean | string | number) {
  if (props.readonly) return
  emit('update:modelValue', { ...props.modelValue, [key]: Boolean(v) } as OpsecForm)
}

function onSyscallMode(v: SyscallMode) {
  if (props.readonly) return
  const mode = v || 'none'
  const next: OpsecForm = { ...props.modelValue, syscallMode: mode }
  // 依赖项跟着走：非 direct 时强制关掉 stackSpoof，避免留下"勾了但不生效"的状态
  if (mode !== 'direct') next.stackSpoof = false
  emit('update:modelValue', next)
}

function disabledReason(cap: OpsecCapability): string {
  return capabilityDisabledReason(cap, props.modelValue, props.osType, props.archType)
}

function reasonText(cap: OpsecCapability): string {
  const r = disabledReason(cap)
  if (r === 'none') return ''
  if (r === 'platform') return i18nStore.tl('仅 Windows 载荷可用', 'Windows payloads only')
  if (r === 'arch') return i18nStore.tl('仅 x64 实现', 'x64 only')
  if (r === 'requires' && cap.requires) {
    return i18nStore.tl(cap.requires.reasonZh, cap.requires.reasonEn)
  }
  return ''
}

function applyPreset(p: OpsecPreset) {
  if (props.readonly) return
  emit('update:modelValue', { ...p.form })
}

defineExpose({ reset: () => emit('update:modelValue', defaultOpsecForm()) })
</script>

<style scoped>
.cs-opsec {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.cs-opsec-presets {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  padding-bottom: 6px;
  border-bottom: 1px solid var(--cs-border);
  margin-bottom: 4px;
}

.cs-opsec-presets-label {
  font-size: 13px;
  color: var(--cs-text-dim);
}

.cs-opsec-preset {
  padding: 2px 10px;
  font-size: 12px;
}

.cs-opsec-preset.is-active {
  border-color: var(--cs-primary, #409eff);
  color: var(--cs-primary, #409eff);
}

.cs-opsec-presets-hint {
  font-size: 12px;
  color: var(--cs-text-dim);
  opacity: 0.8;
}

.cs-opsec-group {
  margin-bottom: 4px;
}

.cs-opsec-group-title {
  font-size: 12px;
  color: var(--cs-text-dim);
  margin: 6px 0 4px;
  letter-spacing: 0.3px;
}

.cs-opsec-row {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 3px 0;
}

.cs-opsec-label {
  min-width: 150px;
  max-width: 190px;
  font-size: 13px;
  color: var(--cs-text);
  text-align: right;
  flex-shrink: 0;
  line-height: 20px;
}

.cs-opsec-layer {
  display: inline-block;
  margin-left: 6px;
  padding: 0 5px;
  font-size: 11px;
  border-radius: 3px;
  border: 1px solid var(--cs-border);
  color: var(--cs-text-dim);
  transform: scale(0.95);
}

.cs-opsec-layer.is-runtime {
  color: var(--cs-success, #67c23a);
  border-color: currentColor;
}

.cs-opsec-control {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  min-height: 20px;
}

.cs-opsec-hint {
  font-size: 12px;
  color: var(--cs-text-dim);
  line-height: 1.5;
}

.cs-opsec-hint.is-warn {
  color: var(--cs-warning, #e6a23c);
}

.cs-opsec-blocked {
  font-size: 12px;
  color: var(--cs-text-dim);
  opacity: 0.75;
}

.cs-opsec-readonly {
  font-size: 12px;
  color: var(--cs-warning, #e6a23c);
  line-height: 1.6;
  padding-top: 4px;
}
</style>
