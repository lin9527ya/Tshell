import { defineConfig, presetUno, presetAttributify, presetIcons, presetTypography } from 'unocss'
import { presetMini } from '@unocss/preset-mini'

export default defineConfig({
  // 扫描文件路径
  content: {
    filesystem: [
      './index.html',
      './src/**/*.{vue,js,ts,jsx,tsx}',
    ],
  },
  
  // 预设
  presets: [
    presetUno({ darkMode: 'class' }),
    presetMini({ darkMode: 'class' }),
    presetAttributify(), // 属性模式预设
    presetIcons({
      // 图标预设配置
      scale: 1.2,
      warn: true,
      collections: {
        // 可以添加自定义图标集
      },
    }),
    presetTypography(), // 排版预设
  ],
  
  // 自定义规则
  rules: [
    // 自定义动画
    ['animate-fade-in', { 
      animation: 'fadeIn 0.3s ease-in-out' 
    }],
    ['animate-slide-in', { 
      animation: 'slideIn 0.3s ease-in-out' 
    }],
    ['animate-bounce-in', { 
      animation: 'bounceIn 0.5s ease-in-out' 
    }],
    
    // 自定义阴影
    ['shadow-glow', { 
      'box-shadow': '0 0 20px rgba(64, 158, 255, 0.3)' 
    }],
    ['shadow-glow-success', { 
      'box-shadow': '0 0 20px rgba(103, 194, 58, 0.3)' 
    }],
    ['shadow-glow-warning', { 
      'box-shadow': '0 0 20px rgba(230, 162, 60, 0.3)' 
    }],
    ['shadow-glow-danger', { 
      'box-shadow': '0 0 20px rgba(245, 108, 108, 0.3)' 
    }],
    
    // 渐变背景
    ['bg-gradient-primary', { 
      background: 'linear-gradient(135deg, #409eff 0%, #67c23a 100%)' 
    }],
    ['bg-gradient-dark', { 
      background: 'linear-gradient(135deg, #303133 0%, #606266 100%)' 
    }],
    ['bg-gradient-cyberpunk', { 
      background: 'linear-gradient(135deg, #00ffff 0%, #ff00ff 100%)' 
    }],
    ['bg-gradient-ocean', { 
      background: 'linear-gradient(135deg, #0077be 0%, #00a8cc 100%)' 
    }],
    
    // 玻璃形态效果
    ['glassmorphism', {
      background: 'rgba(255, 255, 255, 0.1)',
      'backdrop-filter': 'blur(10px)',
      border: '1px solid rgba(255, 255, 255, 0.2)',
    }],
    ['glassmorphism-dark', {
      background: 'rgba(0, 0, 0, 0.1)',
      'backdrop-filter': 'blur(10px)',
      border: '1px solid rgba(255, 255, 255, 0.1)',
    }],
  ],
  
  // 自定义快捷方式
  shortcuts: {
    // 按钮样式
    'btn': 'px-4 py-2 rounded transition-all duration-200 cursor-pointer select-none',
    'btn-primary': 'btn bg-blue-500 hover:bg-blue-600 text-white',
    'btn-success': 'btn bg-green-500 hover:bg-green-600 text-white',
    'btn-warning': 'btn bg-yellow-500 hover:bg-yellow-600 text-white',
    'btn-danger': 'btn bg-red-500 hover:bg-red-600 text-white',
    'btn-ghost': 'btn border border-gray-300 hover:bg-gray-100 text-gray-700',
    
    // 卡片样式
    'card': 'bg-white dark:bg-gray-800 rounded-lg shadow-lg border border-gray-200 dark:border-gray-700',
    'card-header': 'px-6 py-4 border-b border-gray-200 dark:border-gray-700',
    'card-body': 'px-6 py-4',
    'card-footer': 'px-6 py-4 border-t border-gray-200 dark:border-gray-700',
    
    // 表单样式
    'input': 'px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 dark:bg-gray-700 dark:text-white',
    'label': 'block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2',
    
    // 布局
    'flex-center': 'flex items-center justify-center',
    'flex-between': 'flex items-center justify-between',
    'flex-col-center': 'flex flex-col items-center justify-center',
    
    // 文本
    'text-primary': 'text-gray-900 dark:text-white',
    'text-secondary': 'text-gray-600 dark:text-gray-400',
    'text-muted': 'text-gray-500 dark:text-gray-500',
    
    // 仪表盘特定样式
    'dashboard-card': 'card p-6 hover:shadow-xl transition-shadow duration-300',
    'stat-card': 'dashboard-card bg-gradient-to-br from-white to-gray-50 dark:from-gray-800 dark:to-gray-900',
    'stat-value': 'text-3xl font-bold text-primary mb-2',
    'stat-label': 'text-sm text-secondary uppercase tracking-wide',
    
    // 侧边栏样式
    'sidebar': 'w-64 bg-white dark:bg-gray-800 border-r border-gray-200 dark:border-gray-700 transition-all duration-300',
    'sidebar-collapsed': 'w-16',
    'menu-item': 'flex items-center px-4 py-3 text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors duration-200',
    'menu-item-active': 'menu-item bg-blue-50 dark:bg-blue-900 text-blue-600 dark:text-blue-300',
    
    // 表格样式
    'table-header': 'bg-gray-50 dark:bg-gray-900 text-gray-700 dark:text-gray-300 font-semibold',
    'table-row': 'hover:bg-gray-50 dark:hover:bg-gray-700 transition-colors duration-150',
  },
  
  // 主题色彩
  theme: {
    colors: {
      primary: {
        50: '#eff6ff',
        100: '#dbeafe',
        200: '#bfdbfe',
        300: '#93c5fd',
        400: '#60a5fa',
        500: '#3b82f6', // 主色
        600: '#2563eb',
        700: '#1d4ed8',
        800: '#1e40af',
        900: '#1e3a8a',
      },
      success: {
        50: '#f0fdf4',
        100: '#dcfce7',
        200: '#bbf7d0',
        300: '#86efac',
        400: '#4ade80',
        500: '#22c55e', // 成功色
        600: '#16a34a',
        700: '#15803d',
        800: '#166534',
        900: '#14532d',
      },
      warning: {
        50: '#fffbeb',
        100: '#fef3c7',
        200: '#fde68a',
        300: '#fcd34d',
        400: '#fbbf24',
        500: '#f59e0b', // 警告色
        600: '#d97706',
        700: '#b45309',
        800: '#92400e',
        900: '#78350f',
      },
      danger: {
        50: '#fef2f2',
        100: '#fee2e2',
        200: '#fecaca',
        300: '#fca5a5',
        400: '#f87171',
        500: '#ef4444', // 错误色
        600: '#dc2626',
        700: '#b91c1c',
        800: '#991b1b',
        900: '#7f1d1d',
      },
    },
    breakpoints: {
      'xs': '480px',
      'sm': '640px',
      'md': '768px',
      'lg': '1024px',
      'xl': '1280px',
      '2xl': '1536px',
    },
    animation: {
      keyframes: {
        fadeIn: '{from{opacity:0}to{opacity:1}}',
        slideIn: '{from{transform:translateX(-100%)}to{transform:translateX(0)}}',
        bounceIn: '{0%{transform:scale(0.3);opacity:0}50%{transform:scale(1.1)}70%{transform:scale(0.9)}100%{transform:scale(1);opacity:1}}',
      },
      durations: {
        fadeIn: '0.3s',
        slideIn: '0.3s',
        bounceIn: '0.5s',
      },
    },
  },
  
  // 变换器
  transformers: [],
  
  // 安全列表 - 确保这些类不会被清除
  safelist: [
    'bg-primary-500',
    'text-white',
    'shadow-lg',
    'rounded-lg',
    'transition-all',
    'duration-300',
  ],
})