import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    // 优雅处理HMR连接错误
    hmr: {
      overlay: true,
      clientPort: undefined,
    },
    // 监听所有网络接口，支持移动端访问
    host: '0.0.0.0',
    port: 5173,
    // 代理配置
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        ws: true, // 支持WebSocket代理
      },
    },
  },
})
