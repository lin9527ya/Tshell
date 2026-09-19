// 用户信息pinia配置信息

import {defineStore} from 'pinia'
import router from '@/router/index'
import { ElMessage } from 'element-plus'
import { useCredentialsStore } from '@/stores/credentials'

// 【single-flight】模块级共享刷新 Promise：并发 401 只触发一次刷新请求，
// 其余请求复用同一结果，避免 N 个并发 401 打 N 次 /users/refresh
let refreshPromise: Promise<boolean> | null = null

export const useUserStore = defineStore('user', {
    state: () => ({
        token: '',
        roles: '',
        username: '',
        permiss: '',
        refresh: '',
        tokenExpiry: 0 // 添加token过期时间
    }),
    actions: {
        // 登录成功后将返回的 token 存起来
        setToken(val: string) {
            this.token = val
            // 计算token过期时间（15分钟）
            this.tokenExpiry = Date.now() + 15 * 60 * 1000
        },
        getToken() {
            // 检查token是否过期
            if (this.token && this.tokenExpiry > 0 && Date.now() > this.tokenExpiry) {
                console.log('Token已过期，清除本地存储')
                this.clearAuth()
                return ''
            }
            return this.token
        },
        setRoles(val: string) {
            this.roles = val
        },
        getRoles() {
            return this.roles
        },
        setPermiss(val: string) {
            this.permiss = val
        },
        getPermiss() {
            return this.permiss
        },
        setUserName(val: string) {
            this.username = val
        },
        getUserName() {
            return this.username
        },
        setRefresh(val: string) {
            this.refresh = val
        },
        getRefresh() {
            return this.refresh
        },
        // 清空认证信息（不跳转）
        clearAuth() {
            this.token = ''
            this.roles = ''
            this.username = ''
            this.permiss = ''
            this.refresh = ''
            this.tokenExpiry = 0
        },
        // 清空 token 和角色并跳转到登录页
        logout() {
            this.clearAuth()
            // OPSEC: 登出同时清空内存态凭证收集（不持久化）
            try {
                const creds = useCredentialsStore()
                creds.$reset()
            } catch { /* 未初始化时忽略 */ }
            router.push('/login')
        },
        // 检查是否已登录
        isLoggedIn() {
            const token = this.getToken()
            return token !== '' && token !== null && token !== undefined
        },
        // 刷新token（single-flight：并发调用共享同一刷新请求）
        async refreshToken() {
            // 不再要求内存里有 refresh token：续期凭 HttpOnly Cookie（credentials: include），
            // 页面刷新后内存 refresh 为空也应能续期——原 guard 会让刷新后的会话直接判定过期
            if (refreshPromise) {
                return refreshPromise
            }
            refreshPromise = this.doRefresh().finally(() => {
                refreshPromise = null
            })
            return refreshPromise
        },
        // 取一个可用的 access token：内存无有效 token（页面刷新/新标签页/已过期）时
        // 先凭 Cookie 续期一次。WS 类端点（终端/HVNC）需要显式 token，靠它避免空串鉴权失败。
        async ensureToken() {
            const token = this.getToken()
            if (token) return token
            await this.refreshToken()
            return this.getToken()
        },
        // 实际刷新逻辑（由 refreshToken 的 single-flight 包装调用）
        async doRefresh() {
            // 10s 超时：避免服务端无响应时挂起所有等待中的请求
            const controller = new AbortController()
            const timer = setTimeout(() => controller.abort(), 10 * 1000)
            try {
                // OPSEC: refresh token 存 HttpOnly Cookie（JS 不可读）。
                // 刷新请求凭 Cookie（credentials include），不再依赖内存 refresh；
                // 页面刷新后仍可续期，避免"过段时间自动退出登录"。
                const response = await fetch('/api/users/refresh', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: '{}',
                    credentials: 'include',
                    signal: controller.signal
                })

                if (response.ok) {
                    const data = await response.json()
                    if (data.code === 200 && data.data) {
                        this.setToken(data.data.access_token || data.data)
                        console.log('Token刷新成功')
                        return true
                    }
                }
            } catch (error) {
                console.error('Token刷新失败:', error)
            } finally {
                clearTimeout(timer)
            }

            console.log('Token刷新失败，清除认证信息')
            this.logout()
            return false
        }
    },
    persist: {
        key: 'bladeshell-user',
        storage: localStorage,
        // OPSEC: access token 不持久化——仅存内存；页面刷新后登录态由 HttpOnly Cookie 维持
        // （服务端 AuthMiddleware 支持 Cookie 认证，JS 无法读取，防 XSS 窃取 token）。
        paths: ['roles', 'username', 'permiss']
    }
})