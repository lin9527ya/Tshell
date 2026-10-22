import { describe, expect, it } from 'vitest'
import { execModePlatforms, isModeAllowed, normalizeClientOs } from './exec-platform'

describe('normalizeClientOs', () => {
  it('优先使用 Os 字段', () => {
    expect(normalizeClientOs('linux', 'x64', 'DESKTOP-01')).toBe('linux')
    expect(normalizeClientOs('windows', 'x64', 'prod-01')).toBe('windows')
    expect(normalizeClientOs('darwin', 'arm64', 'mac')).toBe('darwin')
  })

  it('Os 缺失时退回 Arch/主机名', () => {
    expect(normalizeClientOs('', 'linux-amd64', 'host')).toBe('linux')
    expect(normalizeClientOs('', '', 'my-linux-box')).toBe('linux')
    expect(normalizeClientOs('', '', 'macbook')).toBe('darwin')
  })

  it('无法判断时返回空（交给服务端兜底）', () => {
    expect(normalizeClientOs('', 'x64', 'DESKTOP-ABC')).toBe('')
  })
})

describe('isModeAllowed', () => {
  it('Windows 模式', () => {
    expect(isModeAllowed('execute-assembly', 'windows')).toBe(true)
    expect(isModeAllowed('execute-assembly', 'linux')).toBe(false)
    expect(isModeAllowed('bof', 'darwin')).toBe(false)
  })

  it('Linux 模式', () => {
    expect(isModeAllowed('script', 'linux')).toBe(true)
    expect(isModeAllowed('binary', 'linux')).toBe(true)
    expect(isModeAllowed('script', 'windows')).toBe(false)
  })

  it('exec-file 支持 Windows/Linux，不支持 macOS', () => {
    expect(isModeAllowed('exec-file', 'windows')).toBe(true)
    expect(isModeAllowed('exec-file', 'linux')).toBe(true)
    expect(isModeAllowed('exec-file', 'darwin')).toBe(false)
  })

  it('未知系统放行（由服务端 400 兜底）', () => {
    expect(isModeAllowed('execute-assembly', '')).toBe(true)
    expect(isModeAllowed('script', '')).toBe(true)
  })

  it('平台表覆盖 7 个模式', () => {
    for (const mode of ['execute-assembly', 'inline-bin', 'shellcode-inject', 'script', 'binary', 'bof', 'exec-file']) {
      expect(execModePlatforms(mode).length).toBeGreaterThan(0)
    }
  })
})
