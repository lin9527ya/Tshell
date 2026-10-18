import { describe, expect, it } from 'vitest'
import {
  extOf,
  formatDateTime,
  formatFileSize,
  formatPermissions,
  formatUploadFileSize,
  getFileIconClass,
  getFileNameClass,
  parseFileSize,
  parseToDate,
} from './format'

describe('parseFileSize', () => {
  it('解析带单位的字符串', () => {
    expect(parseFileSize('1 KB')).toBe(1024)
    expect(parseFileSize('1.5 KB')).toBe(1536)
    expect(parseFileSize('2MB')).toBe(2 * 1024 ** 2)
    expect(parseFileSize('1 GB')).toBe(1024 ** 3)
    expect(parseFileSize('1 TB')).toBe(1024 ** 4)
    expect(parseFileSize('512B')).toBe(512)
  })

  it('大小写与空格不敏感', () => {
    expect(parseFileSize('1kb')).toBe(1024)
    expect(parseFileSize('  1  KB  ')).toBe(1024)
  })

  it('纯数字视为字节', () => {
    expect(parseFileSize('4096')).toBe(4096)
    expect(parseFileSize(4096)).toBe(4096)
  })

  it('目录/占位符/空值返回 0（不是 NaN）', () => {
    // 返回 NaN 会让排序链失效，表现为"点了排序但顺序没变"
    expect(parseFileSize('<DIR>')).toBe(0)
    expect(parseFileSize('-')).toBe(0)
    expect(parseFileSize('')).toBe(0)
    expect(parseFileSize('undefined')).toBe(0)
    expect(parseFileSize(null)).toBe(0)
    expect(parseFileSize(undefined)).toBe(0)
    expect(parseFileSize('abc')).toBe(0)
    expect(Number.isNaN(parseFileSize('abc'))).toBe(false)
  })

  it('负数字节视为 0', () => {
    expect(parseFileSize(-1)).toBe(0)
  })
})

describe('formatFileSize', () => {
  it('字节数按 1024 进制换算', () => {
    expect(formatFileSize(0)).toBe('0 B')
    expect(formatFileSize(512)).toBe('512 B')
    expect(formatFileSize(1024)).toBe('1.0 KB')
    expect(formatFileSize(1536)).toBe('1.5 KB')
    expect(formatFileSize(1024 ** 2)).toBe('1.0 MB')
    expect(formatFileSize(1024 ** 3)).toBe('1.00 GB')
  })

  it('已带单位的字符串只补空格，不重新换算', () => {
    // 重新换算会引入"后端说 1.5 KB、前端显示 1.4 KB"这类对不上
    expect(formatFileSize('1.5KB')).toBe('1.5 KB')
    expect(formatFileSize('2 MB')).toBe('2 MB')
  })

  it('非数字串原样返回', () => {
    expect(formatFileSize('weird')).toBe('weird')
  })

  it('占位符归一为 0 B', () => {
    expect(formatFileSize('')).toBe('0 B')
    expect(formatFileSize('0')).toBe('0 B')
    expect(formatFileSize('-')).toBe('0 B')
    expect(formatFileSize('undefined')).toBe('0 B')
    expect(formatFileSize(null)).toBe('0 B')
  })

  it('与 parseFileSize 能来回换算（显示/解析口径一致）', () => {
    // 这两者口径不一致时，列表显示的大小与排序依据会对不上
    for (const n of [1, 1023, 1024, 1024 ** 2, 3 * 1024 ** 2]) {
      expect(parseFileSize(formatFileSize(n))).toBeCloseTo(n, -1)
    }
  })
})

describe('formatUploadFileSize', () => {
  it('上限只到 MB（上传时 "还在传" 比精度更直观）', () => {
    expect(formatUploadFileSize(0)).toBe('0 B')
    expect(formatUploadFileSize(512)).toBe('512 B')
    expect(formatUploadFileSize(2048)).toBe('2.0 KB')
    expect(formatUploadFileSize(1024 ** 2)).toBe('1.0 MB')
    // 2GB 仍显示 MB 而不是 GB
    expect(formatUploadFileSize(2 * 1024 ** 3)).toBe('2048.0 MB')
  })

  it('非法值不产生 NaN 字符串', () => {
    expect(formatUploadFileSize(NaN)).toBe('0 B')
    expect(formatUploadFileSize(-5)).toBe('0 B')
    expect(formatUploadFileSize(Infinity)).toBe('0 B')
  })
})

describe('formatPermissions', () => {
  it('占位符归一为 -', () => {
    expect(formatPermissions('-')).toBe('-')
    expect(formatPermissions('undefined')).toBe('-')
    expect(formatPermissions('null')).toBe('-')
    expect(formatPermissions('')).toBe('-')
    expect(formatPermissions(null)).toBe('-')
    expect(formatPermissions(undefined)).toBe('-')
  })

  it('真实权限原样返回', () => {
    expect(formatPermissions('rwxr-xr-x')).toBe('rwxr-xr-x')
    expect(formatPermissions('-rw-r--r--')).toBe('-rw-r--r--')
  })
})

describe('parseToDate', () => {
  it('秒与毫秒按 1e11 分界', () => {
    // 1e11 毫秒 ≈ 1973 年；1e11 秒 ≈ 5138 年，所以大于它的一定是毫秒
    expect(parseToDate(1700000000)?.getTime()).toBe(1700000000 * 1000)
    expect(parseToDate(1700000000000)?.getTime()).toBe(1700000000000)
    expect(parseToDate('1700000000')?.getTime()).toBe(1700000000 * 1000)
  })

  it('解析 DD/MM/YYYY 与 DD-MM-YYYY', () => {
    const a = parseToDate('17/08/2026 14:32:00')!
    expect(a.getFullYear()).toBe(2026)
    expect(a.getMonth()).toBe(7) // 8 月
    expect(a.getDate()).toBe(17)
    expect(a.getHours()).toBe(14)
    expect(a.getMinutes()).toBe(32)
    const b = parseToDate('17-08-2026')!
    expect(b.getDate()).toBe(17)
    expect(b.getMonth()).toBe(7)
  })

  it('解析 YYYY/MM/DD 与 YYYY-MM-DD', () => {
    const a = parseToDate('2026-08-17 14:32:00')!
    expect(a.getFullYear()).toBe(2026)
    expect(a.getMonth()).toBe(7)
    expect(a.getDate()).toBe(17)
    const b = parseToDate('2026/08/17')!
    expect(b.getDate()).toBe(17)
  })

  it('占位符与非法值返回 null', () => {
    expect(parseToDate('-')).toBeNull()
    expect(parseToDate('undefined')).toBeNull()
    expect(parseToDate('null')).toBeNull()
    expect(parseToDate('')).toBeNull()
    expect(parseToDate(null)).toBeNull()
    expect(parseToDate(undefined)).toBeNull()
    expect(parseToDate(0)).toBeNull()
    expect(parseToDate('not a date at all')).toBeNull()
  })

  it('Date 实例直接返回，Invalid Date 返回 null', () => {
    const d = new Date(2026, 0, 2)
    expect(parseToDate(d)).toBe(d)
    expect(parseToDate(new Date('nope'))).toBeNull()
  })
})

describe('formatDateTime', () => {
  it('按行对象的多种字段名取值', () => {
    // 后端不同实现回的字段名不一样，这个宽容度是刻意的
    const expect1 = '2026/08/17 14:32:00'
    expect(formatDateTime({ modifiedTime: '2026-08-17 14:32:00' })).toBe(expect1)
    expect(formatDateTime({ modified_time: '2026-08-17 14:32:00' })).toBe(expect1)
    expect(formatDateTime({ time: '2026-08-17 14:32:00' })).toBe(expect1)
    expect(formatDateTime({ Time: '2026-08-17 14:32:00' })).toBe(expect1)
    expect(formatDateTime({ mtime: '2026-08-17 14:32:00' })).toBe(expect1)
    expect(formatDateTime({ ModifiedTime: '2026-08-17 14:32:00' })).toBe(expect1)
  })

  it('补零到两位', () => {
    expect(formatDateTime('2026-01-02 03:04:05')).toBe('2026/01/02 03:04:05')
  })

  it('占位符返回 -（而不是 Invalid Date）', () => {
    expect(formatDateTime('-')).toBe('-')
    expect(formatDateTime('undefined')).toBe('-')
    expect(formatDateTime('')).toBe('-')
    expect(formatDateTime(null)).toBe('-')
    expect(formatDateTime(undefined)).toBe('-')
    expect(formatDateTime({})).toBe('-')
  })

  it('解析不了时把 - 换成 / 原样返回', () => {
    // 注意 '2026-08-17-extra' 是**能**解析的（前缀匹配到日期，属刻意的宽容解析），
    // 所以这里要用真正无法解析的串
    expect(formatDateTime('abc-def-ghi')).toBe('abc/def/ghi')
  })

  it('前缀能解析的串按日期显示（宽容解析）', () => {
    // 后端偶尔会带回显后缀，不该因此显示成乱码
    expect(formatDateTime('2026-08-17-extra')).toBe('2026/08/17 00:00:00')
  })
})

describe('extOf', () => {
  it('取小写扩展名', () => {
    expect(extOf('a.EXE')).toBe('exe')
    expect(extOf('archive.tar.gz')).toBe('gz')
    expect(extOf('noext')).toBe('')
  })

  it('开头的点不算扩展名（隐藏文件）', () => {
    expect(extOf('.bashrc')).toBe('')
    expect(extOf('.gitignore')).toBe('')
  })

  it('结尾的点不算扩展名', () => {
    expect(extOf('weird.')).toBe('')
  })
})

describe('getFileIconClass / getFileNameClass', () => {
  it('目录用 folder 类', () => {
    expect(getFileIconClass({ type: 'D', name: 'x' })).toBe('icon-folder')
    expect(getFileNameClass({ type: 'D', name: 'x' })).toBe('name-folder')
  })

  it('按扩展名分类', () => {
    expect(getFileIconClass({ type: 'F', name: 'a.exe' })).toBe('icon-binary')
    expect(getFileIconClass({ type: 'F', name: 'a.sh' })).toBe('icon-script')
    expect(getFileIconClass({ type: 'F', name: 'a.zip' })).toBe('icon-zip')
    expect(getFileIconClass({ type: 'F', name: 'a.txt' })).toBe('icon-doc')
  })

  it('大小写不敏感', () => {
    expect(getFileIconClass({ type: 'F', name: 'A.EXE' })).toBe('icon-binary')
    expect(getFileNameClass({ type: 'F', name: 'A.PS1' })).toBe('name-script')
  })

  it('兜底类必须是 name-doc / icon-doc（CSS 里有对应样式）', () => {
    // 返回空串会让文件名失去着色 —— .name-doc 是有样式的
    expect(getFileNameClass({ type: 'F', name: 'a.txt' })).toBe('name-doc')
    expect(getFileNameClass({ type: 'F', name: 'noext' })).toBe('name-doc')
    expect(getFileNameClass(null)).toBe('name-doc')
    expect(getFileIconClass(null)).toBe('icon-doc')
  })

  it('两个函数的扩展名表刻意不一致（原实现如此，未顺手统一）', () => {
    // .dylib 只影响图标、不影响文件名着色；.vbs 同理。
    // 这是原实现就有的不一致，抽取只做搬运 —— 统一属可见的视觉变更，应单独提。
    expect(getFileIconClass({ type: 'F', name: 'a.dylib' })).toBe('icon-binary')
    expect(getFileNameClass({ type: 'F', name: 'a.dylib' })).toBe('name-doc')
    expect(getFileIconClass({ type: 'F', name: 'a.vbs' })).toBe('icon-script')
    expect(getFileNameClass({ type: 'F', name: 'a.vbs' })).toBe('name-doc')
  })
})
