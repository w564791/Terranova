import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import fs from 'fs'
import path from 'path'

const isDev = process.env.NODE_ENV !== 'production'
const useHttps = fs.existsSync(path.resolve(__dirname, '../certs/localhost-key.pem'))

export default defineConfig({
  plugins: [react()],

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
        target: 'http://localhost:8080',
        changeOrigin: true,
        ws: true,
        secure: false, // 后端是 https 自签名时不炸
        rewrite: (path) => path.replace(/^\/api/, ''),
      },
    },
  },
})
