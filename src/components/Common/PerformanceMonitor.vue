<template>
  <el-drawer
    v-model="visible"
    title="性能监控"
    direction="rtl"
    size="400px"
    :show-close="true"
  >
    <div class="performance-monitor">
      <!-- 实时性能指标 -->
      <el-card class="metrics-card" shadow="hover">
        <template #header>
          <div class="card-header">
            <span>实时性能</span>
            <el-button 
              type="text" 
              @click="refreshMetrics"
              :loading="refreshing"
            >
              刷新
            </el-button>
          </div>
        </template>
        
        <div class="metrics-grid">
          <div class="metric-item">
            <div class="metric-label">内存使用</div>
            <div class="metric-value">{{ formatBytes(memoryUsage.used) }}</div>
            <div class="metric-total">/ {{ formatBytes(memoryUsage.total) }}</div>
          </div>
          
          <div class="metric-item">
            <div class="metric-label">FPS</div>
            <div class="metric-value">{{ currentFPS }}</div>
            <div class="metric-total">fps</div>
          </div>
          
          <div class="metric-item">
            <div class="metric-label">DOM 节点</div>
            <div class="metric-value">{{ domNodeCount }}</div>
            <div class="metric-total">个</div>
          </div>
          
          <div class="metric-item">
            <div class="metric-label">网络请求</div>
            <div class="metric-value">{{ networkRequests }}</div>
            <div class="metric-total">个</div>
          </div>
        </div>
      </el-card>
      
      <!-- 页面加载时间 -->
      <el-card class="load-times-card" shadow="hover">
        <template #header>
          <span>页面加载时间</span>
        </template>
        
        <div class="load-times">
          <div class="time-item">
            <span>首次内容绘制 (FCP)</span>
            <span>{{ loadTimes.fcp }}ms</span>
          </div>
          <div class="time-item">
            <span>最大内容绘制 (LCP)</span>
            <span>{{ loadTimes.lcp }}ms</span>
          </div>
          <div class="time-item">
            <span>DOM 内容加载</span>
            <span>{{ loadTimes.domContentLoaded }}ms</span>
          </div>
          <div class="time-item">
            <span>完全加载</span>
            <span>{{ loadTimes.loadTime }}ms</span>
          </div>
        </div>
      </el-card>
      
      <!-- 最近性能记录 -->
      <el-card class="recent-metrics-card" shadow="hover">
        <template #header>
          <span>最近记录</span>
        </template>
        
        <div class="recent-metrics">
          <div 
            v-for="(metric, index) in recentMetrics" 
            :key="index"
            class="recent-metric-item"
          >
            <div class="metric-name">{{ metric.name }}</div>
            <div class="metric-time">{{ formatTime(metric.timestamp) }}</div>
            <div class="metric-data">{{ formatMetricData(metric.data) }}</div>
          </div>
        </div>
      </el-card>
      
      <!-- 性能建议 -->
      <el-card class="suggestions-card" shadow="hover">
        <template #header>
          <span>性能建议</span>
        </template>
        
        <div class="suggestions">
          <div 
            v-for="suggestion in performanceSuggestions" 
            :key="suggestion.id"
            class="suggestion-item"
            :class="suggestion.priority"
          >
            <el-icon class="suggestion-icon">
              <component :is="suggestion.icon" />
            </el-icon>
            <div class="suggestion-content">
              <div class="suggestion-title">{{ suggestion.title }}</div>
              <div class="suggestion-desc">{{ suggestion.description }}</div>
            </div>
          </div>
        </div>
      </el-card>
    </div>
  </el-drawer>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { Warning, InfoFilled, SuccessFilled } from '@element-plus/icons-vue'

interface PerformanceMetric {
  name: string
  data: any
  timestamp: number
}

interface PerformanceSuggestion {
  id: string
  title: string
  description: string
  priority: 'low' | 'medium' | 'high'
  icon: any
}

const visible = ref(false)
const refreshing = ref(false)

// 性能数据
const memoryUsage = reactive({
  used: 0,
  total: 0,
  limit: 0
})

const loadTimes = reactive({
  fcp: 0,
  lcp: 0,
  domContentLoaded: 0,
  loadTime: 0
})

const currentFPS = ref(0)
const domNodeCount = ref(0)
const networkRequests = ref(0)
const recentMetrics = ref<PerformanceMetric[]>([])

// FPS 监控
let frameCount = 0
let lastTime = performance.now()
let animationId = 0

// 性能建议
const performanceSuggestions = computed((): PerformanceSuggestion[] => {
  const suggestions: PerformanceSuggestion[] = []
  
  // 内存使用检查
  if (memoryUsage.used / memoryUsage.limit > 0.8) {
    suggestions.push({
      id: 'memory-high',
      title: '内存使用过高',
      description: '当前内存使用超过80%，建议刷新页面或关闭不必要的标签页',
      priority: 'high',
      icon: Warning
    })
  }
  
  // FPS 检查
  if (currentFPS.value < 30) {
    suggestions.push({
      id: 'fps-low',
      title: 'FPS 过低',
      description: '页面帧率较低，可能影响用户体验，建议检查是否有性能瓶颈',
      priority: 'medium',
      icon: InfoFilled
    })
  }
  
  // DOM 节点检查
  if (domNodeCount.value > 10000) {
    suggestions.push({
      id: 'dom-nodes-high',
      title: 'DOM 节点过多',
      description: 'DOM 节点数量超过10000个，建议优化组件结构或使用虚拟滚动',
      priority: 'medium',
      icon: InfoFilled
    })
  }
  
  if (suggestions.length === 0) {
    suggestions.push({
      id: 'performance-good',
      title: '性能状态良好',
      description: '当前页面性能表现良好，继续保持！',
      priority: 'low',
      icon: SuccessFilled
    })
  }
  
  return suggestions
})

// 格式化字节数
const formatBytes = (bytes: number): string => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i]
}

// 格式化时间
const formatTime = (timestamp: number): string => {
  return new Date(timestamp).toLocaleTimeString()
}

// 格式化指标数据
const formatMetricData = (data: any): string => {
  if (typeof data === 'object') {
    if (data.duration) return `${data.duration}ms`
    if (data.value) return `${Math.round(data.value)}ms`
    return JSON.stringify(data).substring(0, 50) + '...'
  }
  return String(data)
}

// FPS 计算
const calculateFPS = () => {
  frameCount++
  const now = performance.now()
  
  if (now - lastTime >= 1000) {
    currentFPS.value = Math.round((frameCount * 1000) / (now - lastTime))
    frameCount = 0
    lastTime = now
  }
  
  animationId = requestAnimationFrame(calculateFPS)
}

// 更新性能指标
const updateMetrics = () => {
  // 更新内存使用
  if ('memory' in performance) {
    const memory = (performance as any).memory
    memoryUsage.used = memory.usedJSHeapSize
    memoryUsage.total = memory.totalJSHeapSize
    memoryUsage.limit = memory.jsHeapSizeLimit
  }
  
  // 更新 DOM 节点数
  domNodeCount.value = document.querySelectorAll('*').length
  
  // 更新网络请求数（从性能条目中获取）
  const resourceEntries = performance.getEntriesByType('resource')
  networkRequests.value = resourceEntries.length
}

// 刷新指标
const refreshMetrics = async () => {
  refreshing.value = true
  
  try {
    updateMetrics()
    
    // 获取性能监控器的数据
    const performanceMonitor = (window as any).performanceMonitor
    if (performanceMonitor) {
      recentMetrics.value = performanceMonitor.getMetrics().slice(-10).reverse()
    }
    
    await new Promise(resolve => setTimeout(resolve, 500)) // 模拟加载
  } finally {
    refreshing.value = false
  }
}

// 获取页面加载时间
const getLoadTimes = () => {
  const navigation = performance.getEntriesByType('navigation')[0] as PerformanceNavigationTiming
  const paint = performance.getEntriesByType('paint')
  
  if (navigation) {
    loadTimes.loadTime = Math.round(navigation.loadEventEnd - navigation.loadEventStart)
    loadTimes.domContentLoaded = Math.round(
      navigation.domContentLoadedEventEnd - navigation.domContentLoadedEventStart
    )
  }
  
  const fcp = paint.find(entry => entry.name === 'first-contentful-paint')
  if (fcp) {
    loadTimes.fcp = Math.round(fcp.startTime)
  }
  
  // LCP 需要通过 PerformanceObserver 获取
  if ('PerformanceObserver' in window) {
    try {
      const observer = new PerformanceObserver((entryList) => {
        const entries = entryList.getEntries()
        const lastEntry = entries[entries.length - 1] as any
        if (lastEntry) {
          loadTimes.lcp = Math.round(lastEntry.startTime)
        }
      })
      observer.observe({ entryTypes: ['largest-contentful-paint'] })
    } catch (error) {
      console.warn('LCP observer not supported:', error)
    }
  }
}

// 暴露显示方法
const show = () => {
  visible.value = true
  refreshMetrics()
}

// 隐藏方法
const hide = () => {
  visible.value = false
}

onMounted(() => {
  // 启动 FPS 监控
  calculateFPS()
  
  // 获取初始加载时间
  getLoadTimes()
  
  // 定期更新指标
  const interval = setInterval(updateMetrics, 5000)
  
  // 将组件方法暴露到全局，方便调用
  ;(window as any).performanceMonitorComponent = { show, hide }
  
  onUnmounted(() => {
    clearInterval(interval)
    if (animationId) {
      cancelAnimationFrame(animationId)
    }
  })
})

defineExpose({
  show,
  hide
})
</script>

<style scoped>
.performance-monitor {
  padding: 0;
}

.metrics-card,
.load-times-card,
.recent-metrics-card,
.suggestions-card {
  margin-bottom: 16px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.metrics-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}

.metric-item {
  text-align: center;
  padding: 16px;
  background: #f8f9fa;
  border-radius: 8px;
}

.metric-label {
  font-size: 12px;
  color: #666;
  margin-bottom: 4px;
}

.metric-value {
  font-size: 18px;
  font-weight: bold;
  color: #409eff;
}

.metric-total {
  font-size: 12px;
  color: #999;
}

.load-times {
  space-y: 12px;
}

.time-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 0;
  border-bottom: 1px solid #eee;
}

.time-item:last-child {
  border-bottom: none;
}

.recent-metrics {
  max-height: 200px;
  overflow-y: auto;
}

.recent-metric-item {
  padding: 8px;
  margin-bottom: 8px;
  background: #f8f9fa;
  border-radius: 4px;
  border-left: 3px solid #409eff;
}

.metric-name {
  font-weight: bold;
  font-size: 12px;
  color: #333;
}

.metric-time {
  font-size: 11px;
  color: #999;
  margin: 2px 0;
}

.metric-data {
  font-size: 11px;
  color: #666;
}

.suggestions {
  space-y: 12px;
}

.suggestion-item {
  display: flex;
  align-items: flex-start;
  padding: 12px;
  border-radius: 8px;
  border: 1px solid #eee;
}

.suggestion-item.high {
  background: #fef0f0;
  border-color: #fbc4c4;
}

.suggestion-item.medium {
  background: #fdf6ec;
  border-color: #f5dab1;
}

.suggestion-item.low {
  background: #f0f9ff;
  border-color: #bae6fd;
}

.suggestion-icon {
  margin-right: 8px;
  margin-top: 2px;
}

.suggestion-content {
  flex: 1;
}

.suggestion-title {
  font-weight: bold;
  font-size: 13px;
  margin-bottom: 4px;
}

.suggestion-desc {
  font-size: 12px;
  color: #666;
  line-height: 1.4;
}
</style>