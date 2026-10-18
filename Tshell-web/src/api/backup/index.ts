import request from '@/utils/request'

// 数据库备份（后端 pkg/api/backup.go，仅 admin）
// 注意：恢复不在这里 —— SQLite 无法在服务端运行时热替换数据库，
// 恢复走 CLI（./tshell -restore <备份>），见 docs/BACKUP_RESTORE.md。
export interface BackupInfo {
  name: string
  path: string
  size: number
  created_at: string
  verified: boolean
  note?: string
}

export interface BackupListResult {
  dir: string
  keep: number
  list: BackupInfo[]
}

export default {
  list() {
    return request({ url: '/backup/list', method: 'GET' })
  },
  create(note = '') {
    return request({ url: '/backup/create', method: 'POST', data: { note } })
  },
  remove(name: string) {
    return request({ url: `/backup/${encodeURIComponent(name)}`, method: 'DELETE' })
  },
  download(name: string) {
    return request({
      url: `/backup/download/${encodeURIComponent(name)}`,
      method: 'GET',
      responseType: 'blob'
    })
  },
  import(file: File) {
    const form = new FormData()
    form.append('file', file)
    return request({ url: '/backup/import', method: 'POST', data: form })
  }
}
