import { beforeEach, describe, expect, it, vi } from 'vitest'

const genServer = vi.fn()

vi.mock('@/api/clients', () => ({
  default: { GenServer: (...a: unknown[]) => genServer(...a) },
}))

import { generateClientBinaryBlob } from './payloadUtil'

// 多传输端点必须原样进请求体：后端据此选用 multi 模板并 patch 端点列表。
// 漏传时后端会生成一个单传输载荷——操作员以为配了回退链，实际没有。
describe('generateClientBinaryBlob 多传输端点', () => {
  beforeEach(() => {
    genServer.mockReset().mockResolvedValue({
      status: 200,
      data: new Blob(['MZ']),
      headers: { 'content-type': 'application/octet-stream' },
    })
  })

  it('转发 endpoints 到 GenServer 请求体', async () => {
    await generateClientBinaryBlob({
      listener: 'tcp://1.2.3.4:443',
      osType: 'windows',
      archType: 'amd64',
      vkey: 'v'.repeat(32),
      encryptSalt: 's'.repeat(16),
      endpoints: 'tcp://1.2.3.4:443,https://cdn.example.com/api',
    })
    const body = genServer.mock.calls[0][0]
    expect(body.endpoints).toBe('tcp://1.2.3.4:443,https://cdn.example.com/api')
  })

  it('未填端点时传空串（后端据此使用单传输模板）', async () => {
    await generateClientBinaryBlob({
      listener: 'tcp://1.2.3.4:443',
      osType: 'linux',
      archType: 'amd64',
      vkey: 'v'.repeat(32),
      encryptSalt: 's'.repeat(16),
    })
    expect(genServer.mock.calls[0][0].endpoints).toBe('')
  })
})

// OPSEC 开关必须原样到达后端：这里曾经漏传 etwPatch/amsiBypass/syscallMode/autoDelete
// （blob 路径与 exe 路径的 opts 类型各写一份、逐渐漂移），表现是"界面勾了、载荷里还是 0"。
describe('generateClientBinaryBlob OPSEC 开关', () => {
  beforeEach(() => {
    genServer.mockReset().mockResolvedValue({
      status: 200,
      data: new Blob(['MZ']),
      headers: { 'content-type': 'application/octet-stream' },
    })
  })

  it('转发全部 9 个 OPSEC 字段', async () => {
    await generateClientBinaryBlob({
      listener: 'tcp://1.2.3.4:443',
      osType: 'windows',
      archType: 'amd64',
      vkey: 'v'.repeat(32),
      encryptSalt: 's'.repeat(16),
      etwPatch: '1',
      amsiBypass: '0',
      syscallMode: 'direct',
      autoDelete: '0',
      sleepMaskHide: '1',
      stackSpoof: '1',
      ppidSpoof: '0',
      blockDLLs: '1',
      unhookOnStart: '1',
    })
    const body = genServer.mock.calls[0][0]
    expect(body.etwPatch).toBe('1')
    expect(body.amsiBypass).toBe('0')
    expect(body.syscallMode).toBe('direct')
    expect(body.autoDelete).toBe('0')
    expect(body.sleepMaskHide).toBe('1')
    expect(body.stackSpoof).toBe('1')
    expect(body.ppidSpoof).toBe('0')
    expect(body.blockDLLs).toBe('1')
    expect(body.unhookOnStart).toBe('1')
  })

  it('未提供的开关传空串，由后端回落默认', async () => {
    await generateClientBinaryBlob({
      listener: 'tcp://1.2.3.4:443',
      osType: 'windows',
      archType: 'amd64',
      vkey: 'v'.repeat(32),
      encryptSalt: 's'.repeat(16),
    })
    const body = genServer.mock.calls[0][0]
    // 空串在后端被 normalizeOnOff 判成"未提供" → 走"opsec 配置 → 内置默认"回落
    expect(body.etwPatch).toBe('')
    expect(body.stackSpoof).toBe('')
  })
})
