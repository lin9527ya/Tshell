import { describe, it, expect } from 'vitest'
import {
  buildWgetCommand,
  fileNameFromUrl,
  isSafeRemoteUrl,
  joinRemotePath,
  replaceLastSegment,
  sanitizeFileName,
} from './remotePath'

describe('joinRemotePath', () => {
  it('Windows 用反斜杠拼接并保留盘符', () => {
    expect(joinRemotePath('C:\\Users\\a', 'x.exe', 'windows')).toBe('C:\\Users\\a\\x.exe')
    expect(joinRemotePath('C:\\Users\\a\\', 'x.exe', 'windows')).toBe('C:\\Users\\a\\x.exe')
    expect(joinRemotePath('C:\\', 'x.exe', 'windows')).toBe('C:\\x.exe')
  })

  it('Linux 用斜杠拼接并保留根目录', () => {
    expect(joinRemotePath('/opt/app', 'x.sh', 'linux')).toBe('/opt/app/x.sh')
    expect(joinRemotePath('/opt/app/', 'x.sh', 'linux')).toBe('/opt/app/x.sh')
    expect(joinRemotePath('/', 'x.sh', 'linux')).toBe('/x.sh')
  })

  it('空目录时只返回文件名', () => {
    expect(joinRemotePath('', 'x', 'linux')).toBe('x')
  })
})

describe('replaceLastSegment', () => {
  it('同时识别 "/" 与 "\\"（Windows 重命名不能拿到相对名）', () => {
    expect(replaceLastSegment('C:\\dir\\a.txt', 'b.txt')).toBe('C:\\dir\\b.txt')
    expect(replaceLastSegment('/opt/a.txt', 'b.txt')).toBe('/opt/b.txt')
    expect(replaceLastSegment('a.txt', 'b.txt')).toBe('b.txt')
  })
})

describe('fileNameFromUrl', () => {
  it('去掉 query/fragment 并解码', () => {
    expect(fileNameFromUrl('https://x.com/a/b.zip?token=1#frag')).toBe('b.zip')
    expect(fileNameFromUrl('https://x.com/%E6%8C%87%E4%BB%A4.exe')).toBe('指令.exe')
  })

  it('URL 没有文件名时回退 download', () => {
    expect(fileNameFromUrl('https://x.com/')).toBe('download')
  })

  it('非法字符替换为下划线', () => {
    expect(sanitizeFileName('a:b*c?.txt')).toBe('a_b_c_.txt')
  })
})

describe('isSafeRemoteUrl', () => {
  it('只放行 http/https 且拒绝引号', () => {
    expect(isSafeRemoteUrl('https://example.com/x')).toBe(true)
    expect(isSafeRemoteUrl('ftp://example.com/x')).toBe(false)
    expect(isSafeRemoteUrl('https://example.com/x" & whoami & "')).toBe(false)
    expect(isSafeRemoteUrl("https://example.com/x' ; id")).toBe(false)
  })
})

describe('buildWgetCommand', () => {
  it('Linux 优先 wget，失败退回 curl', () => {
    const cmd = buildWgetCommand({ url: 'https://x.com/a.bin', dir: '/opt', os: 'linux' })
    expect(cmd).toBe("wget -O '/opt/a.bin' 'https://x.com/a.bin' || curl -fL -o '/opt/a.bin' 'https://x.com/a.bin'")
  })

  it('Windows 用 PowerShell Invoke-WebRequest，失败退回 curl.exe', () => {
    const cmd = buildWgetCommand({ url: 'https://x.com/a.bin', dir: 'C:\\Temp', os: 'windows' })
    expect(cmd).toContain('powershell -NoProfile -ExecutionPolicy Bypass -Command')
    expect(cmd).toContain("Invoke-WebRequest -UseBasicParsing -Uri 'https://x.com/a.bin' -OutFile 'C:\\Temp\\a.bin'")
    expect(cmd).toContain("curl.exe -L -o 'C:\\Temp\\a.bin' 'https://x.com/a.bin'")
  })

  it('macOS 走 POSIX 分支', () => {
    const cmd = buildWgetCommand({ url: 'https://x.com/a.bin', dir: '/tmp', os: 'darwin' })
    expect(cmd.startsWith('wget -O ')).toBe(true)
  })

  it('非法 URL 直接抛错（不构造可注入命令）', () => {
    expect(() => buildWgetCommand({ url: 'https://x.com/a" ; rm -rf /', dir: '/', os: 'linux' })).toThrow()
    expect(() => buildWgetCommand({ url: 'file:///etc/passwd', dir: '/', os: 'linux' })).toThrow()
  })

  it('路径里的单引号在 sh 与 PowerShell 中都被转义', () => {
    const sh = buildWgetCommand({ url: 'https://x.com/a.bin', dir: "/tmp/it's", os: 'linux' })
    expect(sh).toContain(`'/tmp/it'\\''s/a.bin'`)
    const ps = buildWgetCommand({ url: 'https://x.com/a.bin', dir: "C:\\T's", os: 'windows' })
    expect(ps).toContain("C:\\T''s\\a.bin")
  })

  it('URL 带单引号直接拒绝（不做转义，避免歧义）', () => {
    expect(() => buildWgetCommand({ url: "https://x.com/it's.bin", dir: '/tmp', os: 'linux' })).toThrow()
  })
})
