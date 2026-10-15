import {createApp} from "vue"
import { ElLoading } from "element-plus"
import * as ElementPlusIconsVue from '@element-plus/icons-vue'

import pinia from "@/stores/index";
import App from "./App.vue"
import router from "./router"
import {usePermissStore} from "@/stores/userpermiss"
import {useUserStore} from "@/stores/user"
import {useThemeStore} from "@/stores/theme"
import piniaPluginPersistedstate from "pinia-plugin-persistedstate"
// Element Plus 按需引入（模板组件由 unplugin-vue-components 自动解析），
// 函数式组件（ElMessage/ElMessageBox/ElLoading）的样式需显式导入
import "element-plus/es/components/message/style/css"
import "element-plus/es/components/message-box/style/css"
import "element-plus/es/components/loading/style/css"
import "element-plus/theme-chalk/dark/css-vars.css"
import "@unocss/reset/tailwind.css"
import "uno.css"
import "./assets/main.css"
import "./assets/theme-variables.css"
import "./assets/cs-theme.css"

import plugins from '@/plugins'
import directives from "@/components/Common/WaterMark";

// 性能监控
class PerformanceMonitor {
  private static instance: PerformanceMonitor;
  private metricsData: any[] = [];
  
  static getInstance(): PerformanceMonitor {
    if (!PerformanceMonitor.instance) {
      PerformanceMonitor.instance = new PerformanceMonitor();
    }
    return PerformanceMonitor.instance;
  }
  
  startMonitoring() {
    // 监控页面加载性能
    window.addEventListener('load', () => {
      setTimeout(() => {
        const navigation = performance.getEntriesByType('navigation')[0] as PerformanceNavigationTiming;
        const paint = performance.getEntriesByType('paint');
        
        const metrics = {
          // 页面加载时间
          loadTime: navigation.loadEventEnd - navigation.loadEventStart,
          // DOM 内容加载时间
          domContentLoaded: navigation.domContentLoadedEventEnd - navigation.domContentLoadedEventStart,
          // 首次内容绘制
          fcp: paint.find(entry => entry.name === 'first-contentful-paint')?.startTime || 0,
          // 最大内容绘制
          lcp: 0, // 需要通过 PerformanceObserver 获取
          // 内存信息（如果可用）
          memory: (performance as any).memory ? {
            used: (performance as any).memory.usedJSHeapSize,
            total: (performance as any).memory.totalJSHeapSize,
            limit: (performance as any).memory.jsHeapSizeLimit
          } : null
        };
        
        this.recordMetric('page-load', metrics);
      }, 0);
    });
    
    // 监控 LCP
    if ('PerformanceObserver' in window) {
      const lcpObserver = new PerformanceObserver((entryList) => {
        const entries = entryList.getEntries();
        const lastEntry = entries[entries.length - 1] as any;
        this.recordMetric('lcp', { value: lastEntry.startTime });
      });
      lcpObserver.observe({ entryTypes: ['largest-contentful-paint'] });
    }
    
    // 监控路由切换性能
    let routeStartTime = Date.now();
    router.beforeEach(() => {
      routeStartTime = Date.now();
    });
    
    router.afterEach(() => {
      const routeTime = Date.now() - routeStartTime;
      this.recordMetric('route-change', { duration: routeTime });
    });
  }
  
  recordMetric(name: string, data: any) {
    this.metricsData.push({
      name,
      data,
      timestamp: Date.now()
    });
    
    // 只保留最近 100 条记录
    if (this.metricsData.length > 100) {
      this.metricsData = this.metricsData.slice(-100);
    }
    
    // 在开发环境输出性能信息
    if (import.meta.env.DEV) {
      console.log(`🚀 Performance [${name}]:`, data);
    }
  }
  
  getMetrics() {
    return this.metricsData;
  }
}

// 资源预加载
class ResourcePreloader {
  private static preloadedRoutes = new Set<string>();
  
  static preloadRoute(routeName: string) {
    if (this.preloadedRoutes.has(routeName)) {
      return Promise.resolve();
    }
    
    this.preloadedRoutes.add(routeName);
    
    // 这里可以根据路由名预加载对应的组件
    // 由于是动态导入，我们可以在用户可能访问前预加载
    const routeImportMap: Record<string, () => Promise<any>> = {
      'Listeners': () => import('@/views/listener/listener.vue'),
    };
    
    const importFn = routeImportMap[routeName];
    if (importFn) {
      return importFn().catch(err => {
        console.warn(`预加载路由 ${routeName} 失败:`, err);
        this.preloadedRoutes.delete(routeName);
      });
    }
    
    return Promise.resolve();
  }
  
  // 预加载关键路由
  static preloadCriticalRoutes() {
    // 延迟预加载，避免影响首屏
    setTimeout(() => {
      this.preloadRoute('Clients');
    }, 2000);
  }
}

// 执行方法得到实例
pinia.use(piniaPluginPersistedstate)

// 创建应用实例
const app = createApp(App)
app.use(pinia)

// 全局注册 Element Plus 图标（ClientView 等组件以字符串 :is="'Monitor'" 方式引用图标，
// 未全局注册时解析失败 → 标签图标消失；SideBar 用 markRaw 组件引用所以此前正常）
for (const [name, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(name, component)
}

// 初始化性能监控
const performanceMonitor = PerformanceMonitor.getInstance();
performanceMonitor.startMonitoring();

// 预加载关键资源
ResourcePreloader.preloadCriticalRoutes();

// 初始化主题系统
const themeStore = useThemeStore()
themeStore.init()

app.use(directives)
app.use(plugins)

// v-loading 指令：按需模式下需手动注册（ElLoading.directive）
app.directive("loading", ElLoading.directive)

// 自定义权限指令
const permiss = usePermissStore()
app.directive("permiss", {
    mounted(el, binding) {
        if (!permiss.key.includes(String(binding.value))) {
            el["hidden"] = true
        }
    }
})

// 设置页面标题的路由守卫
router.beforeEach((to, from, next) => {
    const pageTitle = to.meta.title ? `${to.meta.title} - ` : ''
    document.title = `${pageTitle}Tshell`
    next()
})

app.use(router)

app.mount("#app")
