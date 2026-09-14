import { defineStore } from 'pinia'

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
  },

  // OPSEC: 凭证不持久化（内存态，刷新/登出即清空）——明文凭证落 localStorage 是敏感泄露风险
  persist: false,
})
