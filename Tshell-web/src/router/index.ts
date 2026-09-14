import { createRouter, createWebHashHistory } from 'vue-router'
import Home from '../views/HomeView.vue'
import {cancelRequest} from "@/utils/request"
import { usePermissStore } from '@/stores/userpermiss'
import { Perm } from '@/utils/permiss'
import { useUserStore } from '@/stores/user'
import { cachedValidateToken, cachedValidateTokenDetailed, clearTokenCache } from '@/utils/auth/tokenValidator'

// 绑定路由和vue
const router = createRouter({
  history: createWebHashHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      name: 'Home',
      component: Home,
      meta: {
        requiresAuth: true,
        title: 'Console'
      },
      children: []
    },
    {
      path: '/login',
      name: '登陆',
      meta: {
        title: '登陆'
      },
      component: () => import('@/views/login/LoginView.vue')
    },
    {
      path: '/404',
      name: '404',
      meta: {
        title: '404'
      },
      component: () => import('@/views/error/404View.vue')
    },
    {
      path: '/401',
      name: '401',
      meta: {
        title: '401'
      },
      component: () => import('@/views/error/401View.vue')
    }
  ]
})

// 路由导航守卫 - JWT认证（优化版本）
router.beforeEach(async (to, from, next) => {
  const userStore = useUserStore()
  
  // 公开路由列表
  const publicRoutes = ['/login', '/404', '/401']
  
  // 如果是公开路由，直接通过
  if (publicRoutes.includes(to.path)) {
    // 如果用户已登录且访问登录页，重定向到 Sessions
    if (to.path === '/login' && userStore.getToken()) {
      const token = userStore.getToken()
      
      try {
        // 验证token是否仍然有效 - 使用更轻量的请求
        const isValid = await cachedValidateToken(token)
        
        if (isValid) {
          console.log('用户已登录，重定向到 Sessions')
          next('/')
          return
        } else {
          console.log('Token验证失败，清除无效token')
          userStore.logout()
          clearTokenCache()
        }
      } catch (error) {
        console.log('Token验证失败，清除无效token:', error)
        userStore.logout()
        clearTokenCache()
      }
    }
    next()
    return
  }
  
  // 检查是否需要认证
  if (to.meta.requiresAuth !== false) {
    const token = userStore.getToken()
    
    // OPSEC: 无内存 token 时凭 HttpOnly Cookie 验证（页面刷新后登录态保持，
    // token 不持久化 localStorage）；Cookie 也无效才跳登录页。
    const check = await cachedValidateTokenDetailed(token || '')
    
    if (check.valid) {
      console.log('JWT令牌验证通过，允许访问:', to.path)
    } else if (check.needsLogin) {
      console.log('JWT令牌验证失败，跳转到登录页')
      userStore.logout()
      clearTokenCache()
      next('/login')
      return
    }
    // 网络错误：放行，避免误登出
  }

  // F2: RBAC 路由层权限校验——meta.permiss 存在时必须命中用户权限集合，
  // 否则跳 401（原 meta.permiss 无任何消费方，直接输 URL 即可越权访问页面）
  const permissStore = usePermissStore()
  const requiredPerm = to.meta?.permiss as string | undefined
  // 【安全修复】permiss 为空（未加载/被清空）时拒绝而非放行——原实现
  // `key.length > 0` 条件使清空 localStorage 后凭有效 token 即可绕过路由层校验
  if (requiredPerm && (permissStore.key.length === 0 || !permissStore.key.includes(requiredPerm))) {
    console.log(`🚫 [RBAC] 用户无权访问 ${to.path}（需要 permiss ${requiredPerm}）`)
    next('/401')
    return
  }
  
  // 如果不需要认证，直接通过
  next()
})

export default router
