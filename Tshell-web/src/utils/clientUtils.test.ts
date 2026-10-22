import { describe, it, expect } from 'vitest'
import { clientOsIcon, detectClientOs, formatClientTabTitle } from './clientUtils'

describe('clientUtils', () => {
  it('识别系统类型并给出 vshell 同款图标', () => {
    expect(detectClientOs({ Os: 'windows_amd64' })).toBe('windows')
    expect(detectClientOs({ Os: 'linux_amd64' })).toBe('linux')
    expect(detectClientOs({ Os: 'darwin_arm64' })).toBe('darwin')
    expect(clientOsIcon({ Os: 'windows_amd64' })).toBe('devicon-windows8')
    expect(clientOsIcon({ Os: 'linux_amd64' })).toBe('devicon-linux')
    expect(clientOsIcon({ Os: 'darwin_arm64' })).toBe('wpf-macos')
    expect(clientOsIcon(null)).toBe('desktop')
  })

  it('主机管理标签为 ID:3 客户端管理', () => {
    expect(formatClientTabTitle({ func: 'console', id: '3', externalIp: '1.2.3.4', zh: true })).toBe(
      'ID:3 客户端管理',
    )
  })

  it('文件管理标签为 ID值: 外网IP', () => {
    expect(formatClientTabTitle({ func: 'files', id: '3', externalIp: '1.2.3.4', zh: true })).toBe(
      '3: 1.2.3.4',
    )
    // 外网 IP 缺失时不能出现 undefined
    expect(formatClientTabTitle({ func: 'files', id: '3', externalIp: '', zh: true })).toBe('3: -')
  })
})
