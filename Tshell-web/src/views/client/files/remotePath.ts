/**
 * 远程文件路径与 wget 命令构造。
 *
 * 为什么单独抽出来：
 *   - Windows 与 Linux 的路径分隔符不同，重命名/新建/上传都按 "/" 拼路径，
 *     在 Windows 上会把 `C:\dir\a.txt` 拆错（重命名变成只改了个相对名）。
 *   - wget 不是 Windows 自带命令；Linux 也未必装了 wget（curl 更常见）。
 *     原先无条件下发 `wget -O ... ...`，在 Windows 上通过 cmd.exe 执行必然失败。
 *   - URL / 路径最终会被拼进 shell 命令，必须在这里做转义与白名单校验，
 *     否则一个带引号的 URL 就能在下发命令里截断并追加任意命令。
 */

export type RemoteOs = 'windows' | 'linux' | 'darwin' | 'unknown'

/** 路径分隔符：Windows 用反斜杠，其余用斜杠 */
export function remoteSep(os: RemoteOs): '/' | '\\' {
  return os === 'windows' ? '\\' : '/'
}

/** 拼接远程路径（保留根目录的写法："/" + name、`C:\` + name） */
export function joinRemotePath(dir: string, name: string, os: RemoteOs): string {
  const sep = remoteSep(os)
  const d = String(dir ?? '').trim()
  if (!d) return name
  if (/^[\\/]+$/.test(d)) return d + name
  return d.replace(/[\\/]+$/, '') + sep + name
}

/** 替换路径最后一段（重命名用）：同时识别 "/" 与 "\" */
export function replaceLastSegment(path: string, newName: string): string {
  const s = String(path ?? '')
  const idx = Math.max(s.lastIndexOf('/'), s.lastIndexOf('\\'))
  if (idx < 0) return newName
  return s.slice(0, idx + 1) + newName
}

/** 去掉 Windows / POSIX 都不允许的字符 */
export function sanitizeFileName(name: string): string {
  return String(name ?? '')
    .replace(/[<>:"/\\|?*\u0000-\u001f]/g, '_')
    .replace(/^\.+$/, '')
    .trim()
}

/** 从 URL 推文件名：去掉 query/fragment，URL 解码，过滤非法字符 */
export function fileNameFromUrl(url: string): string {
  const raw = String(url ?? '').trim()
  let base = ''
  try {
    const u = new URL(raw)
    const parts = u.pathname.split('/').filter(Boolean)
    base = decodeURIComponent(parts[parts.length - 1] || '')
  } catch {
    const clean = raw.split(/[?#]/)[0]
    const parts = clean.split('/').filter(Boolean)
    try {
      base = decodeURIComponent(parts[parts.length - 1] || '')
    } catch {
      base = parts[parts.length - 1] || ''
    }
  }
  return sanitizeFileName(base) || 'download'
}

/** URL 白名单：仅 http/https，且不含能截断 shell 引号的字符 */
export function isSafeRemoteUrl(url: string): boolean {
  const s = String(url ?? '').trim()
  if (!/^https?:\/\/\S+$/i.test(s)) return false
  return !/["'`\r\n]/.test(s)
}

/** 目标路径白名单：不能含双引号/换行（会截断命令） */
export function isSafeRemotePath(path: string): boolean {
  const s = String(path ?? '')
  return !!s.trim() && !/["\r\n]/.test(s)
}

/** sh 单引号转义：' -> '\'' */
function shQuote(s: string): string {
  return `'${s.replace(/'/g, `'\\''`)}'`
}

/** PowerShell 单引号转义：' -> '' */
function psQuote(s: string): string {
  return `'${s.replace(/'/g, "''")}'`
}

/**
 * 构造跨平台下载命令（下发到目标机执行）：
 *   Windows → PowerShell Invoke-WebRequest，失败退回 curl.exe（Win10 1803+ 自带）
 *   Linux/macOS → wget，未安装时退回 curl
 */
export function buildWgetCommand(opts: { url: string; dir: string; os: RemoteOs }): string {
  const url = String(opts.url ?? '').trim()
  if (!isSafeRemoteUrl(url)) {
    throw new Error('URL 非法：仅支持 http/https，且不能包含引号')
  }
  const target = joinRemotePath(opts.dir, fileNameFromUrl(url), opts.os)
  if (!isSafeRemotePath(target)) {
    throw new Error('保存路径非法：不能包含引号或换行')
  }

  if (opts.os === 'windows') {
    const iwr = `Invoke-WebRequest -UseBasicParsing -Uri ${psQuote(url)} -OutFile ${psQuote(target)}`
    const curl = `curl.exe -L -o ${psQuote(target)} ${psQuote(url)}`
    return `powershell -NoProfile -ExecutionPolicy Bypass -Command "try { ${iwr} } catch { ${curl} }"`
  }

  return `wget -O ${shQuote(target)} ${shQuote(url)} || curl -fL -o ${shQuote(target)} ${shQuote(url)}`
}
