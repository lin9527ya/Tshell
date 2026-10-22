import { describe, expect, it } from 'vitest'
import { normalizeWebDelivery, normalizeWebDeliveryList } from './webDelivery'

describe('normalizeWebDelivery', () => {
  it('把后端 snake_case 字段补齐为页面使用的 PascalCase', () => {
    const row = normalizeWebDelivery({
      id: 7,
      listening_port: 'shared-abc123',
      os: 'windows',
      arch: 'amd64',
      listener_config: 'https://1.2.3.4:8443',
      status: 1,
      server_address: 'https://cdn.example.com/d/abc/full/update_x.exe',
      stager_url: 'https://cdn.example.com/d/abc/stager',
      shellcode_url: 'https://cdn.example.com/d/abc/woff',
      public_key: 'abc',
      bind_mode: 'shared',
      file_name: 'update_x.exe',
      save_filename: 'update_x.exe',
      crypto_mode: 'aead',
    })

    expect(row.ID).toBe(7)
    expect(row.ListeningPort).toBe('shared-abc123')
    expect(row.OS).toBe('windows')
    expect(row.Arch).toBe('amd64')
    expect(row.ListenerConfig).toBe('https://1.2.3.4:8443')
    expect(row.Status).toBe(1)
    expect(row.StagerURL).toBe('https://cdn.example.com/d/abc/stager')
    expect(row.CryptoMode).toBe('aead')
  })

  it('保留后端已经返回的 PascalCase 字段', () => {
    const row = normalizeWebDelivery({ ListeningPort: '8443', OS: 'linux', Arch: 'arm64', Status: 2 })
    expect(row.ListeningPort).toBe('8443')
    expect(row.OS).toBe('linux')
    expect(row.Status).toBe(2)
  })

  it('列表归一化不修改输入数组，并处理空值', () => {
    const input = [{ listening_port: 8089, os: 'windows' }, null]
    const out = normalizeWebDeliveryList(input as any[])
    expect(out[0].ListeningPort).toBe('8089')
    expect(out[0].Status).toBe(0)
    expect(out[0].BindMode).toBe('shared')
    expect(input[0]).toEqual({ listening_port: 8089, os: 'windows' })
  })
})
