import { describe, expect, it } from 'vitest'
import {
  OPSEC_CAPABILITIES,
  OPSEC_PRESETS,
  SYSCALL_MODES,
  capabilityDisabledReason,
  defaultOpsecForm,
  normalizeForPlatform,
  toGenServerBody,
  type OpsecForm,
} from './opsecPresets'

const findCap = (key: string) => OPSEC_CAPABILITIES.find((c) => c.key === key)!

describe('opsecPresets 定义自洽', () => {
  it('每个能力都有唯一 key 与中英文名', () => {
    const keys = OPSEC_CAPABILITIES.map((c) => c.key)
    expect(new Set(keys).size).toBe(keys.length)
    for (const c of OPSEC_CAPABILITIES) {
      expect(c.labelZh.length).toBeGreaterThan(0)
      expect(c.labelEn.length).toBeGreaterThan(0)
    }
  })

  it('预设字段与表单模型一一对应', () => {
    const modelKeys = Object.keys(defaultOpsecForm()).sort()
    for (const p of OPSEC_PRESETS) {
      expect(Object.keys(p.form).sort()).toEqual(modelKeys)
    }
  })

  it('syscall 三个取值都有标签', () => {
    expect(SYSCALL_MODES.map((m) => m.value)).toEqual(['none', 'direct', 'indirect'])
  })

  it('默认值与服务端一致：对抗能力全关，autoDelete 开', () => {
    const d = defaultOpsecForm()
    expect(d.autoDelete).toBe(true)
    for (const k of ['etwPatch', 'amsiBypass', 'sleepMaskHide', 'stackSpoof', 'ppidSpoof', 'blockDLLs', 'unhookOnStart'] as const) {
      expect(d[k]).toBe(false)
    }
    expect(d.syscallMode).toBe('none')
  })
})

describe('normalizeForPlatform', () => {
  const allOn: OpsecForm = {
    etwPatch: true,
    amsiBypass: true,
    syscallMode: 'direct',
    autoDelete: true,
    sleepMaskHide: true,
    stackSpoof: true,
    ppidSpoof: true,
    blockDLLs: true,
    unhookOnStart: true,
  }

  it('linux 上关掉所有 windows-only 能力', () => {
    const out = normalizeForPlatform(allOn, 'linux', 'amd64')
    expect(out.etwPatch).toBe(false)
    expect(out.amsiBypass).toBe(false)
    expect(out.syscallMode).toBe('none')
    expect(out.sleepMaskHide).toBe(false)
    expect(out.stackSpoof).toBe(false)
    expect(out.unhookOnStart).toBe(false)
    expect(out.ppidSpoof).toBe(false)
    expect(out.blockDLLs).toBe(false)
    // 自删是跨平台能力，不该被平台归一关掉
    expect(out.autoDelete).toBe(true)
  })

  it('windows/386 上退化成 none 并关掉 x64 能力', () => {
    const out = normalizeForPlatform(allOn, 'windows', '386')
    expect(out.syscallMode).toBe('none')
    expect(out.sleepMaskHide).toBe(false)
    expect(out.stackSpoof).toBe(false)
    expect(out.unhookOnStart).toBe(false)
    // PPID/BlockDLLs 在 386 上有实现，保留
    expect(out.ppidSpoof).toBe(true)
    expect(out.blockDLLs).toBe(true)
    expect(out.etwPatch).toBe(true)
  })

  it('syscallMode 非 direct 时强制关掉伪栈', () => {
    const out = normalizeForPlatform({ ...allOn, syscallMode: 'indirect' }, 'windows', 'amd64')
    expect(out.stackSpoof).toBe(false)
    const out2 = normalizeForPlatform(allOn, 'windows', 'amd64')
    expect(out2.stackSpoof).toBe(true)
  })
})

describe('capabilityDisabledReason', () => {
  const form = defaultOpsecForm()

  it('非 windows 上报 platform', () => {
    expect(capabilityDisabledReason(findCap('etwPatch'), form, 'linux')).toBe('platform')
  })

  it('windows/386 上报 arch', () => {
    expect(capabilityDisabledReason(findCap('sleepMaskHide'), form, 'windows', '386')).toBe('arch')
    // PPID 不依赖 x64 代码
    expect(capabilityDisabledReason(findCap('ppidSpoof'), form, 'windows', '386')).toBe('none')
  })

  it('伪栈在 syscallMode=none 上报 requires', () => {
    expect(capabilityDisabledReason(findCap('stackSpoof'), form, 'windows', 'amd64')).toBe('requires')
    const direct = { ...form, syscallMode: 'direct' as const }
    expect(capabilityDisabledReason(findCap('stackSpoof'), direct, 'windows', 'amd64')).toBe('none')
  })
})

describe('toGenServerBody', () => {
  it('把表单转成 1/0 与枚举字符串', () => {
    const body = toGenServerBody(
      { ...defaultOpsecForm(), etwPatch: true, stackSpoof: true, syscallMode: 'direct' },
      'windows',
      'amd64',
    )
    expect(body).toEqual({
      etwPatch: '1',
      amsiBypass: '0',
      syscallMode: 'direct',
      autoDelete: '1',
      sleepMaskHide: '0',
      stackSpoof: '1',
      ppidSpoof: '0',
      blockDLLs: '0',
      unhookOnStart: '0',
    })
  })

  it('非 windows 载荷上不输出"勾了但不生效"的开关', () => {
    const body = toGenServerBody(
      { ...defaultOpsecForm(), etwPatch: true, syscallMode: 'direct', blockDLLs: true },
      'linux',
      'amd64',
    )
    expect(body.etwPatch).toBe('0')
    expect(body.syscallMode).toBe('none')
    expect(body.blockDLLs).toBe('0')
    expect(body.autoDelete).toBe('1')
  })

  it('每项都显式发送，不留空串让后端猜默认', () => {
    const body = toGenServerBody(defaultOpsecForm(), 'windows', 'amd64')
    for (const v of Object.values(body)) {
      expect(v).not.toBe('')
    }
  })
})
