import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import importMetaUrlPlugin from '@codingame/esbuild-import-meta-url-plugin'
import fs from 'fs'
import path from 'path'

const isDev = process.env.NODE_ENV !== 'production'
const useHttps = fs.existsSync(path.resolve(__dirname, '../certs/localhost-key.pem'))

// monaco-vscode-api 包列表 (需要被 vite 显式优化, 否则 worker 重载会引发整页刷新)
const monacoVscodeApiPackages = [
  '@codingame/monaco-vscode-api',
  '@codingame/monaco-vscode-api/extensions',
  '@codingame/monaco-vscode-api/monaco',
  '@codingame/monaco-vscode-configuration-service-override',
  '@codingame/monaco-vscode-keybindings-service-override',
  '@codingame/monaco-vscode-languages-service-override',
  '@codingame/monaco-vscode-textmate-service-override',
  '@codingame/monaco-vscode-theme-service-override',
  '@codingame/monaco-vscode-snippets-service-override',
  '@codingame/monaco-vscode-theme-defaults-default-extension',
  '@codingame/monaco-vscode-extensions-service-override',
  '@codingame/monaco-vscode-files-service-override',
  '@codingame/monaco-vscode-api/extensions',
]

export default defineConfig({
  plugins: [
    react(),
    // 关键: vscode-api 内部用 new URL(..., import.meta.url) 引用资源 (theme json /
    // package.nls.json 等), Vite 默认 dep 预优化会丢失相对路径产生 404,
    // 需要 esbuild 插件保持 import.meta.url 语义
    {
      name: 'vscode-css-as-string',
      enforce: 'pre',
      async resolveId(source, importer, options) {
        const resolved = await this.resolve(source, importer, options)
        if (
          resolved &&
          /node_modules\/(@codingame\/monaco-vscode|vscode|monaco-editor).*\.css$/.test(
            resolved.id,
          )
        ) {
          return { ...resolved, id: resolved.id + '?inline' }
        }
        return undefined
      },
    },
  ],

  worker: {
    // Monaco / vscode-api worker 必须 ES 模块, 否则会报 "Could not create web worker(s)"
    format: 'es',
  },

  optimizeDeps: {
    include: [
      ...monacoVscodeApiPackages,
      'marked',
    ],
    esbuildOptions: {
      plugins: [importMetaUrlPlugin],
    },
  },

  resolve: {
    dedupe: ['vscode', 'monaco-editor', ...monacoVscodeApiPackages],
  },

  build: {
    target: 'esnext',
    chunkSizeWarningLimit: 5000, // monaco-vscode-api 主 chunk 较大,提高警告阈值
    rollupOptions: {
      output: {
        // 把 monaco / vscode-api / antd 拆成独立 chunk:
        //  - 减小单 chunk 体积, 让 rollup 不一次性持有所有 AST,降低 OOM 风险
        //  - 浏览器可并行下载, 缓存命中率更高(只改业务代码不重打 monaco)
        manualChunks(id) {
          if (!id.includes('node_modules')) return undefined
          // React 必须留在 主 chunk,否则 antd 加载时 React 还没初始化会炸
          // ('Cannot set properties of undefined setting Children')。
          if (id.includes('/react/') || id.includes('/react-dom/') ||
              id.includes('/scheduler/') || id.includes('/react-router')) {
            return undefined
          }
          // monaco-vscode-api 系列拆分,避免 rollup 一次性 hold 14MB AST OOM
          if (id.includes('@codingame/monaco-vscode')) return 'vscode-api'
          if (id.includes('vscode-textmate') || id.includes('vscode-oniguruma')) return 'vscode-textmate'
          if (id.includes('monaco-editor')) return 'monaco-editor'
          if (id.includes('@vscode/codicons')) return 'codicons'
          // antd / @ant-design 不再独立拆,跟 React 留同 chunk 避免 hoisting 顺序坑
          return undefined
        },
      },
    },
  },

  server: {
    host: true, // 等价于 0.0.0.0，更语义化
    port: 5173,
    strictPort: true, // 端口被占用就直接报错，避免“连错服务”

    // HTTPS（存在证书才启用，避免新同事 clone 就报错）
    https: useHttps
      ? {
          key: fs.readFileSync(
            path.resolve(__dirname, '../certs/localhost-key.pem')
          ),
          cert: fs.readFileSync(
            path.resolve(__dirname, '../certs/localhost.pem')
          ),
        }
      : false,

    // HMR 配置（HTTPS + 局域网时非常关键）
    hmr: {
      overlay: true,
      protocol: useHttps ? 'wss' : 'ws',
      host: 'localhost',
    },

    // API 代理
    proxy: {
      '/api': {
        target: useHttps ? 'https://localhost:8080' : 'http://localhost:8080',
        changeOrigin: true,
        ws: true,
        secure: false, // 后端是 https 自签名时不炸
        // 注意：不要 rewrite，后端路由已包含 /api/v1 前缀
      },
      '/swagger': {
        target: useHttps ? 'https://localhost:8080' : 'http://localhost:8080',
        changeOrigin: true,
        secure: false,
      },
    },
  },
})
