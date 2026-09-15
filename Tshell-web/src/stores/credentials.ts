import { defineStore } from 'pinia'
import { listLoot, addLoot, deleteLoot, type LootItem } from '@/api/loot'

export interface CredentialItem {
  id: string
  realm: string      // Domain / Realm / Workgroup
  user: string       // Username
  password: string   // Cleartext password
  hash: string       // NTLM hash / Kerberos ticket / other hash
  host: string       // Origin Hostname or IP
  source: string     // 'mimikatz' | 'hashdump' | 'manual' | 'kerberos' | 'vault'
  note?: string
  updatedAt: string
}

export const useCredentialsStore = defineStore('cs-credentials', {
  state: () => ({
    credentials: [] as CredentialItem[],
    selectedIds: [] as string[],
  }),

  getters: {
    totalCount: (s) => s.credentials.length,
    credById: (s) => (id: string) => s.credentials.find((c) => c.id === id),
  },

  actions: {
    addCredential(cred: Omit<CredentialItem, 'id' | 'updatedAt'> & { id?: string; updatedAt?: string }) {
      const now = new Date().toLocaleString()
      const existing = this.credentials.find(
        (c) =>
          c.user.toLowerCase() === cred.user.toLowerCase() &&
          (c.realm || '').toLowerCase() === (cred.realm || '').toLowerCase() &&
          (c.host || '').toLowerCase() === (cred.host || '').toLowerCase(),
      )

      if (existing) {
        if (cred.password) existing.password = cred.password
        if (cred.hash) existing.hash = cred.hash
        if (cred.source) existing.source = cred.source
        if (cred.note) existing.note = cred.note
        existing.updatedAt = now
        return existing
      } else {
        const newItem: CredentialItem = {
          id: cred.id || `cred_${Date.now()}_${Math.random().toString(36).slice(2, 7)}`,
          realm: cred.realm || '',
          user: cred.user || '',
          password: cred.password || '',
          hash: cred.hash || '',
          host: cred.host || '',
          source: cred.source || 'manual',
          note: cred.note || '',
          updatedAt: cred.updatedAt || now,
        }
        this.credentials.unshift(newItem)
        return newItem
      }
    },

    updateCredential(id: string, updates: Partial<CredentialItem>) {
      const idx = this.credentials.findIndex((c) => c.id === id)
      if (idx >= 0) {
        this.credentials[idx] = {
          ...this.credentials[idx],
          ...updates,
          updatedAt: new Date().toLocaleString(),
        }
      }
    },

    removeCredential(id: string) {
      this.credentials = this.credentials.filter((c) => c.id !== id)
      this.selectedIds = this.selectedIds.filter((sid) => sid !== id)
    },

    removeSelected() {
      const set = new Set(this.selectedIds)
      this.credentials = this.credentials.filter((c) => !set.has(c.id))
      this.selectedIds = []
    },

    clearAll() {
      this.credentials = []
      this.selectedIds = []
    },

    setSelectedIds(ids: string[]) {
      this.selectedIds = ids
    },

    /**
     * 解析 Mimikatz / logonpasswords 输出并自动提取凭据
     */
    parseMimikatzOutput(text: string, host: string) {
      if (!text) return
      const lines = text.split('\n')
      let currentDomain = ''
      let currentUser = ''
      let currentPassword = ''
      let currentNtlm = ''

      for (const rawLine of lines) {
        const line = rawLine.trim()
        if (line.includes('Domain') && line.includes(':')) {
          const parts = line.split(':')
          currentDomain = parts[1]?.trim() || ''
        } else if (line.includes('User Name') && line.includes(':')) {
          const parts = line.split(':')
          currentUser = parts[1]?.trim() || ''
        } else if (line.includes('* Password') && line.includes(':')) {
          const parts = line.split(':')
          currentPassword = parts.slice(1).join(':').trim()
          if (currentPassword === '(null)') currentPassword = ''
        } else if (line.includes('* NTLM') && line.includes(':')) {
          const parts = line.split(':')
          currentNtlm = parts[1]?.trim() || ''

          if (currentUser && (currentPassword || currentNtlm)) {
            this.addCredential({
              realm: currentDomain,
              user: currentUser,
              password: currentPassword,
              hash: currentNtlm,
              host,
              source: 'mimikatz',
            })
            currentUser = ''
            currentPassword = ''
            currentNtlm = ''
          }
        }
      }
    },

    /**
     * 解析 Hashdump 输出（格式：user:rid:lmhash:nthash:::）
     */
    parseHashdumpOutput(text: string, host: string) {
      if (!text) return
      const lines = text.split('\n')
      for (const rawLine of lines) {
        const line = rawLine.trim()
        const parts = line.split(':')
        if (parts.length >= 4) {
          const user = parts[0]?.trim()
          const nthash = parts[3]?.trim()
          if (user && nthash && nthash.length === 32) {
            this.addCredential({
              realm: '',
              user,
              password: '',
              hash: `${parts[2] || 'aad3b435b51404eeaad3b435b51404ee'}:${nthash}`,
              host,
              source: 'hashdump',
            })
          }
        }
      }
    },

    // ===== 服务端 Loot 库同步(功能批) =====
    // 服务端为凭据的持久层: 拉取合并到内存 store(去重),本地新增/删除同步写回。
    // 内存态仍在(刷新即重新拉取),与服务端"审计可追溯"要求兼容。

    /** 从服务端 loot 库拉取并合并(uid 维度去重) */
    async fetchFromServer() {
      try {
        const res = await listLoot({ page: 1, size: 1000 })
        const data = res.data?.data?.list
        if (!Array.isArray(data)) return
        for (const item of data as LootItem[]) {
          // uid/user/realm/host 相同视为同一条(与 addCredential 的去重键一致)
          const typeMap: Record<string, string> = { pwd: 'manual', hash: 'hashdump', domain: 'vault', note: 'vault' }
          this.addCredential({
            id: `loot_${item.id}`, // 服务端条目以 loot_ 前缀标识
            realm: item.realm || '',
            user: item.user || '',
            // pwd: secret 即明文密码;hash: secret 即哈希
            password: item.type === 'pwd' ? (item.secret || '') : '',
            hash: item.type === 'hash' || item.type === 'domain' ? (item.secret || '') : '',
            host: item.host || '',
            source: item.source || typeMap[item.type] || 'vault',
            note: item.note || '',
            updatedAt: item.created_at || '',
          })
        }
      } catch {
        // 拉取失败静默(离线/权限),本地内存态仍可用
      }
    },

    /** 新增凭据并写回服务端(uid 来源会话可选) */
    async addCredentialPersistent(cred: Omit<CredentialItem, 'id' | 'updatedAt'> & { id?: string; updatedAt?: string }, uid?: string) {
      const local = this.addCredential(cred)
      // 已是服务端条目则不重复写
      if (local.id.startsWith('loot_')) return local
      try {
        // 密码与哈希分两条存(pwd/hash),域账号归 domain
        const type = cred.password ? 'pwd' : (cred.hash ? (cred.realm && cred.realm !== 'WORKGROUP' ? 'domain' : 'hash') : 'note')
        const res = await addLoot({
          uid: uid || '',
          type,
          user: cred.user || '',
          realm: cred.realm || '',
          host: cred.host || '',
          secret: cred.password || cred.hash || '',
          note: cred.note || '',
          source: cred.source || 'manual',
        })
        const sid = res.data?.data?.id
        if (sid) local.id = `loot_${sid}` // 落库成功后改为服务端 id,后续删除走服务端
      } catch {
        // 写回失败保留本地态
      }
      return local
    },

    /** 删除凭据(服务端条目同步删库;本地条目仅删内存) */
    async removeCredentialPersistent(id: string) {
      this.removeCredential(id)
      if (id.startsWith('loot_')) {
        const sid = parseInt(id.slice(5), 10)
        if (!isNaN(sid)) {
          try { await deleteLoot(sid) } catch { /* 删库失败不影响本地态 */ }
        }
      }
    },
  },

  // OPSEC: 凭证不持久化（内存态，刷新/登出即清空）——明文凭证落 localStorage 是敏感泄露风险
  persist: false,
})
