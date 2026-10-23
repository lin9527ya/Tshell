/**
 * 文件管理器的纯格式化/解析逻辑（阶段三 §3.11：拆 2213 行的 ClientFiles.vue）。
 *
 * 为什么先拆这些、而不是先拆模板：
 *
 * 这个视图此前**没有任何测试**。在没安全网的情况下动 2213 行的文件是危险的；
 * 而下面这些函数是纯函数、边界最多、也最容易出错 —— 它们恰恰是"能先补测试"的部分。
 * 抽出来 + 覆盖测试之后，组件只剩展示与编排，后续再拆模板才有底气。
 *
 * 另一个动机：这些函数被 3 个以上地方各自实现过一遍（`formatFileSize` 与
 * `formatUploadFileSize` 逻辑几乎重复、`parseFileSize` 的逆运算散在别处），
 * 收口到一处才能保证"显示的大小"与"解析出来的字节数"互相对得上。
 */

/** 单位表：KB 起按 1024 进制（与 `parseFileSize` 必须一致，否则显示/解析来回不一致） */
const UNITS: Array<[string, number]> = [
  ['TB', 1024 ** 4],
  ['GB', 1024 ** 3],
  ['MB', 1024 ** 2],
  ['KB', 1024],
  ['B', 1],
]

/**
 * parseFileSize 把 "1.5 KB" / "512B" / "<DIR>" 这类字符串解析成字节数。
 *
 * 解析不出来返回 0 而不是 NaN：调用方用它排序，NaN 会让整个比较链失效
 * （`a - b` 得 NaN 时排序结果不确定，表现为"点了排序但顺序没变"）。
 */
export function parseFileSize(sizeStr: unknown): number {
  if (sizeStr === null || sizeStr === undefined) return 0
  if (typeof sizeStr === 'number') return Number.isFinite(sizeStr) && sizeStr > 0 ? sizeStr : 0
  const s = String(sizeStr).trim()
  if (!s || s === '-' || s === '<DIR>' || s === 'undefined') return 0
  // 纯数字视为字节数（不同后端有时回裸字节）
  if (/^\d+$/.test(s)) return parseInt(s, 10)
  const m = s.match(/^([\d.]+)\s*(B|KB|MB|GB|TB)$/i)
  if (!m) return 0
  const val = parseFloat(m[1])
  if (!Number.isFinite(val)) return 0
  const unit = m[2].toUpperCase()
  const hit = UNITS.find(([u]) => u === unit)
  return val * (hit ? hit[1] : 1)
}

/**
 * formatFileSize 把字节数（或已是 "1.5KB" 形态的字符串）格式化成人读形态。
 *
 * 已经带单位的字符串**原样保留单位**，只补一个空格 —— 后端给的值是它自己算的，
 * 我们重新换算只会引入"后端说 1.5 KB、前端显示 1.4 KB"这类对不上的现象。
 */
export function formatFileSize(size: unknown): string {
  if (typeof size === 'string') {
    const s = size.trim()
    if (s === '' || s === '0' || s === '-' || s === 'undefined') return '0 B'
    if (/^\d+(\.\d+)?(B|KB|MB|GB|TB)$/i.test(s)) {
      return s.replace(/(\d+(?:\.\d+)?)(B|KB|MB|GB|TB)$/i, '$1 $2')
    }
    return s
  }
  const n = typeof size === 'number' ? size : parseInt(String(size), 10)
  if (!Number.isFinite(n) || n <= 0) return '0 B'
  if (n < 1024) return `${n} B`
  if (n < 1024 ** 2) return `${(n / 1024).toFixed(1)} KB`
  if (n < 1024 ** 3) return `${(n / 1024 ** 2).toFixed(1)} MB`
  return `${(n / 1024 ** 3).toFixed(2)} GB`
}

/**
 * formatUploadFileSize 上传进度用的格式化。
 *
 * 与 formatFileSize 的区别是**上限只到 MB**：上传大文件时进度条上的数字
 * 频繁跳动，精度反而不重要；而且上传场景下 "2048.00 MB" 比 "2.00 GB"
 * 更能让人看出"还在传"。
 */
export function formatUploadFileSize(size: number): string {
  if (!size || !Number.isFinite(size) || size <= 0) return '0 B'
  if (size < 1024) return `${size} B`
  if (size < 1024 ** 2) return `${(size / 1024).toFixed(1)} KB`
  return `${(size / 1024 ** 2).toFixed(1)} MB`
}

/**
 * formatPermissions 权限列，统一显示成八进制（如 0755 / 0644）。
 *
 * 后端可能回三种形态：
 *   - 符号式：`rwxr-xr-x`、`-rw-r--r--`、`drwxr-xr-x`（首字符是文件类型）
 *   - 已是八进制：`755`、`0755`、`0644`
 *   - 占位符：`-` / `undefined` / `null` / 空串
 * 符号式里带 s/S/t/T 时，把 setuid/setgid/sticky 折进最高位（4755 / 2755 / 1755）。
 */
export function formatPermissions(perm: unknown): string {
  if (perm === null || perm === undefined) return '-'
  const s = String(perm).trim()
  if (!s || s === '-' || s === 'undefined' || s === 'null') return '-'

  // 已是八进制（3~4 位、只含 0-7）→ 补足 4 位，统一带前导 0
  if (/^[0-7]{3,4}$/.test(s)) return s.padStart(4, '0')

  // 符号式：[文件类型]rwxrwxrwx，允许 setuid/setgid/sticky 的 s/S/t/T
  const m = /^[bcdlps-]?([rwxstST-]{9})$/.exec(s)
  if (!m) return s
  const bits = m[1]

  let special = 0
  if (bits[2] === 's' || bits[2] === 'S') special += 4
  if (bits[5] === 's' || bits[5] === 'S') special += 2
  if (bits[8] === 't' || bits[8] === 'T') special += 1

  let oct = ''
  for (let i = 0; i < 9; i += 3) {
    let v = 0
    if (bits[i] === 'r') v += 4
    if (bits[i + 1] === 'w') v += 2
    // x 与 s/t 都表示可执行位（s/t 只是附加 setuid/setgid/sticky）
    if (bits[i + 2] === 'x' || bits[i + 2] === 's' || bits[i + 2] === 't') v += 1
    oct += String(v)
  }
  return `${special}${oct}`
}

/**
 * parseToDate 把后端各种时间形态解析成 Date。
 *
 * 支持的形态（后端不同实现回的不一样，实测至少有这几种）：
 *   数字/数字串：秒或毫秒（用 1e11 分界 —— 1e11 毫秒 ≈ 1973 年，
 *               而 1e11 秒 ≈ 5138 年，所以大于它的一定是毫秒）
 *   DD/MM/YYYY 或 DD-MM-YYYY
 *   YYYY/MM/DD 或 YYYY-MM-DD
 * 解析不出来返回 null（调用方负责回退成原样显示）。
 */
export function parseToDate(t: unknown): Date | null {
  if (t === null || t === undefined) return null
  if (t instanceof Date) return Number.isNaN(t.getTime()) ? null : t
  if (typeof t === 'number') {
    if (!Number.isFinite(t) || t <= 0) return null
    return new Date(t > 1e11 ? t : t * 1000)
  }
  const s = String(t).trim()
  if (!s || s === '-' || s === 'undefined' || s === 'null') return null
  if (/^\d+$/.test(s)) {
    const n = parseInt(s, 10)
    return new Date(n > 1e11 ? n : n * 1000)
  }
  // DD/MM/YYYY 或 DD-MM-YYYY（如 17/08/2026 14:32:00）
  const dmy = s.match(/^(\d{1,2})[/-](\d{1,2})[/-](\d{4})(?:\s+(\d{1,2}):(\d{1,2})(?::(\d{1,2}))?)?/)
  if (dmy) {
    return new Date(
      parseInt(dmy[3], 10),
      parseInt(dmy[2], 10) - 1,
      parseInt(dmy[1], 10),
      parseInt(dmy[4] || '0', 10),
      parseInt(dmy[5] || '0', 10),
      parseInt(dmy[6] || '0', 10),
    )
  }
  // YYYY/MM/DD 或 YYYY-MM-DD（如 2026-08-17 14:32:00）
  const ymd = s.match(/^(\d{4})[/-](\d{1,2})[/-](\d{1,2})(?:\s+(\d{1,2}):(\d{1,2})(?::(\d{1,2}))?)?/)
  if (ymd) {
    return new Date(
      parseInt(ymd[1], 10),
      parseInt(ymd[2], 10) - 1,
      parseInt(ymd[3], 10),
      parseInt(ymd[4] || '0', 10),
      parseInt(ymd[5] || '0', 10),
      parseInt(ymd[6] || '0', 10),
    )
  }
  const d = new Date(s)
  return Number.isNaN(d.getTime()) ? null : d
}

/**
 * formatDateTime 时间列显示。
 *
 * 入参可以是行对象（不同后端字段名不一：modifiedTime / modified_time / mtime…）
 * 或直接是时间值 —— 这个宽容度是刻意的，调用方散在模板各处，逐个适配不现实。
 * 解析不出来时把 `-` 换成 `/` 原样返回，而不是显示 "Invalid Date"。
 */
export function formatDateTime(rowOrTime: unknown): string {
  let t: unknown = rowOrTime
  if (rowOrTime && typeof rowOrTime === 'object') {
    const r = rowOrTime as Record<string, unknown>
    t = r.modifiedTime ?? r.modified_time ?? r.time ?? r.Time ?? r.mtime ?? r.ModifiedTime
  }
  if (t === null || t === undefined) return '-'
  const s = String(t).trim()
  if (!s || s === '-' || s === 'undefined' || s === 'null') return '-'
  const d = parseToDate(t)
  if (!d) return s.replace(/-/g, '/')
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}/${p(d.getMonth() + 1)}/${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}

const ARCHIVE_EXT = ['zip', 'tar', 'gz', '7z', 'rar']

// 图标与文件名的扩展名表**刻意保持原样不一致**：
//
//   getFileIconClass  二进制含 dylib、脚本含 vbs、有压缩包分支、兜底 icon-doc
//   getFileNameClass  二进制不含 dylib、脚本不含 vbs、无压缩包分支、兜底 name-doc
//
// 这是原实现就有的不一致，不是本次抽取引入的。抽取只做搬运，**不顺手统一** ——
// 统一会让 .dylib / .vbs 在文件名列突然变色，属可见的视觉变更，应该单独提一次、
// 由人确认后再改（而不是夹在一个"重构"里悄悄发生）。
const ICON_BINARY_EXT = ['exe', 'bin', 'dll', 'so', 'elf', 'dylib']
const ICON_SCRIPT_EXT = ['sh', 'ps1', 'bat', 'cmd', 'py', 'vbs', 'cna']
const NAME_BINARY_EXT = ['exe', 'bin', 'dll', 'so', 'elf']
const NAME_SCRIPT_EXT = ['sh', 'ps1', 'bat', 'cmd', 'py', 'cna']

/** 取小写扩展名（无扩展名返回空串） */
export function extOf(name: unknown): string {
  const s = String(name ?? '')
  const i = s.lastIndexOf('.')
  // 开头的点不算扩展名（.bashrc 是隐藏文件而不是 "bashrc" 扩展名）
  if (i <= 0 || i === s.length - 1) return ''
  return s.slice(i + 1).toLowerCase()
}

/** getFileIconClass 列表/树里的图标类名 */
export function getFileIconClass(row: { type?: string; name?: string } | null | undefined): string {
  if (!row) return 'icon-doc'
  if (row.type === 'D') return 'icon-folder'
  const ext = extOf(row.name)
  if (ICON_BINARY_EXT.includes(ext)) return 'icon-binary'
  if (ICON_SCRIPT_EXT.includes(ext)) return 'icon-script'
  if (ARCHIVE_EXT.includes(ext)) return 'icon-zip'
  return 'icon-doc'
}

/** getFileNameClass 文件名着色类名（可执行/脚本高亮，便于一眼扫到） */
export function getFileNameClass(row: { type?: string; name?: string } | null | undefined): string {
  if (!row) return 'name-doc'
  if (row.type === 'D') return 'name-folder'
  const ext = extOf(row.name)
  if (NAME_BINARY_EXT.includes(ext)) return 'name-binary'
  if (NAME_SCRIPT_EXT.includes(ext)) return 'name-script'
  // 兜底必须是 name-doc：CSS 里有 .name-doc 的着色（.name-zip 并不存在）
  return 'name-doc'
}
