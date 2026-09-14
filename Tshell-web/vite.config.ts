import { fileURLToPath, URL } from 'node:url'

import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import vueJsx from '@vitejs/plugin-vue-jsx'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers'
import * as ElementPlusIcons from '@element-plus/icons-vue'

// 图标按需解析：模板中使用的 <Plus/>/<EditPen/> 等图标组件名
// 若命中 @element-plus/icons-vue 导出集，则自动按需导入，替代全量全局注册
const ElementPlusIconsResolver = (name: string) => {
  if (name && (ElementPlusIcons as any)[name]) {
    return { name, from: '@element-plus/icons-vue' }
  }
  return undefined
}

// https://vitejs.dev/config/
export default defineConfig(async () => {
  const UnoCSS = (await import('@unocss/vite')).default
  
  return {
    server: {
      /** 是否开启 HTTPS */
      https: false,
      /** 设置 host: true 才可以使用 Network 的形式，以 IP 访问项目 */
      host: true, // host: "0.0.0.0"
      /** 端口号 */
      port: 3333,
      /** 是否自动打开浏览器 */
      open: false,
      /** 跨域设置允许 */
      cors: true,
      /** 端口被占用时，是否直接退出 */
      strictPort: false,
      /*
       * 连接后端服务时使用 server.proxy 设置代理
       * 当请求路径以 api 开头，说明调用的是真实后端服务
       */
      proxy: {
        '^/api': {
          target: 'http://127.0.0.1:8089/',
          // target: 'http://172.18.6.232:8000/api/v1',
          //rewrite: (path) => path.replace(/^\/api/, ''),
          ws: true,
          /** 是否允许跨域 */
          changeOrigin: true
        }
      }
    },
    /** 打包相关操作 */
    build: {
      /** 启用/禁用 CSS 代码拆分 */
      cssCodeSplit: true,
      /** 构建后是否生成 source map 文件 */
      sourcemap: false,
      /** 单个 chunk 文件的大小超过 2048KB 时发出警告 */
      chunkSizeWarningLimit: 2048,
      /** 禁用 gzip 压缩大小报告 */
      reportCompressedSize: false,
      /** 打包后静态资源目录 */
      assetsDir: 'static',
      /** 小于此阈值的导入或引用资源将内联为 base64 编码 */
      assetsInlineLimit: 4096,
      rollupOptions: {
        output: {
          /**
           * 分块策略
           * 1. 注意这些包名必须存在，否则打包会报错
           * 2. 如果你不想自定义 chunk 分割策略，可以直接移除这段配置
           */
          manualChunks: {
            vue: ['vue', 'vue-router', 'pinia'],
            element: ['element-plus', '@element-plus/icons-vue'],
            xterm: ['xterm', 'xterm-addon-fit', 'xterm-addon-web-links', 'xterm-addon-webgl', 'xterm-addon-unicode11'],
            utils: ['axios', 'vue-clipboard3'],
            dock: ['dockview'],
            charts: ['echarts', 'vue-echarts']
          },
          /** 用于从入口点创建的块的打包输出格式[name]表示文件名,[hash]表示该文件内容hash值 */
          entryFileNames: 'js/[name].[hash].js',
          /** 用于命名代码拆分时创建的共享块的输出命名 */
          chunkFileNames: 'js/[name].[hash].js',
          /** 用于输出静态资源的命名，[ext]表示文件扩展名 */
          assetFileNames: (assetInfo) => {
            const info = assetInfo.name.split('.')
            let extType = info[info.length - 1]
            // console.log('文件信息', assetInfo.name)
            if (/\.(mp4|webm|ogg|mp3|wav|flac|aac)(\?.*)?$/i.test(assetInfo.name)) {
              extType = 'media'
            } else if (/\.(png|jpe?g|gif|svg)(\?.*)?$/i.test(assetInfo.name)) {
              extType = 'img'
            } else if (/\.(woff2?|eot|ttf|otf)(\?.*)?$/i.test(assetInfo.name)) {
              extType = 'fonts'
            }
            return `${extType}/[name].[hash].[ext]`
          }
        },
        /** 外部化依赖 */
        external: [],
      }
    },
    /** 混淆器 */
    esbuild: {
      /** 打包时移除 console.log */
      pure: ['console.log'],
      /** 打包时移除 debugger */
      drop: ['debugger'],
      /** 打包时移除所有注释 */
      legalComments: 'none'
    },
    plugins: [
      vue(),
      vueJsx(),
      UnoCSS(),
      AutoImport({
        resolvers: [ElementPlusResolver()]
      }),
      Components({
        resolvers: [ElementPlusResolver(), ElementPlusIconsResolver as any]
      })
    ],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url))
      }
    }
  }
})