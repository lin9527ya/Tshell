import { describe, it, expect } from 'vitest'
import { formatClientStatus } from './formatUtils'

describe('formatClientStatus', () => {
  it('布尔 true → 在线', () => {
    expect(formatClientStatus(true).text).toBe('在线')
  })
  it('字符串 "1" → 在线', () => {
    expect(formatClientStatus('1').text).toBe('在线')
  })
  it('布尔 false → 离线', () => {
    expect(formatClientStatus(false).text).toBe('离线')
  })
  it('字符串 "2" → 离线', () => {
    expect(formatClientStatus('2').text).toBe('离线')
  })
})
