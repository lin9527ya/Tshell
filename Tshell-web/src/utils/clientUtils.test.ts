import { describe, it, expect } from 'vitest'
import { clientOsIcon, detectClientOs, formatClientTabTitle, formatClientTreeRoot } from './clientUtils'

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

  it('标签恒为 "ID: 3 客户端管理"（与 vshell 一致，不随功能页变化）', () => {
    expect(formatClientTabTitle({ func: 'console', id: '3', externalIp: '1.2.3.4', zh: true })).toBe(
      'ID: 3 客户端管理',
    )
    expect(formatClientTabTitle({ func: 'files', id: '3', externalIp: '1.2.3.4', zh: true })).toBe(
      'ID: 3 客户端管理',
    )
    expect(formatClientTabTitle({ func: 'files', id: '3', externalIp: '', zh: false })).toBe(
      'ID: 3 Client Management',
    )
  })

  it('文件树根节点为 "ID: 外网IP"，有备注时追加在 IP 右侧', () => {
    expect(formatClientTreeRoot({ id: '987', externalIp: '172.171.247.253' })).toBe(
      '987: 172.171.247.253',
    )
    // IP 缺失时不能出现 undefined
    expect(formatClientTreeRoot({ id: '987', externalIp: '' })).toBe('987: -')
    // 备注存在 → 追加；空白备注 → 不追加多余空格
    expect(
      formatClientTreeRoot({ id: '988', externalIp: '172.64.200.106', note: 'https://timetrack.wocoo.tv/' }),
    ).toBe('988: 172.64.200.106 https://timetrack.wocoo.tv/')
    expect(formatClientTreeRoot({ id: '988', externalIp: '172.64.200.106', note: '   ' })).toBe(
      '988: 172.64.200.106',
    )
  })
})
