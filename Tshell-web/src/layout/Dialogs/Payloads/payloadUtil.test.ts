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
