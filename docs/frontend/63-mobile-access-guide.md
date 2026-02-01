# 手机访问配置指南

## 问题描述

手机无法登录 admin 账户，提示"用户不存在"，但电脑端（包括浏览器模拟手机模式）登录正常。

## 根本原因

前端 API 配置硬编码为 `http://localhost:8080`，导致：
- **电脑端正常**：localhost 指向本机后端服务器
- **电脑浏览器模拟手机模式正常**：仍然是电脑访问 localhost
- **真实手机失败**：手机上的 localhost 指向手机本身，无法连接到电脑的后端服务

## 解决方案

### 1. 代码修改（已完成）

修改了 `frontend/src/services/api.ts`，实现 API 地址自动检测：

```typescript
const getApiBaseUrl = () => {
  // 优先使用环境变量配置
  if (import.meta.env.VITE_API_BASE_URL) {
    return import.meta.env.VITE_API_BASE_URL;
  }
  
  // 自动根据当前访问的域名/IP构建 API 地址
  const protocol = window.location.protocol;
  const hostname = window.location.hostname;
  const apiPort = window.location.port === '5173' ? '8080' : window.location.port;
  
  return `${protocol}//${hostname}:${apiPort}/api/v1`;
};
```

**自动检测逻辑**：
- 如果访问 `http://localhost:5173` → API 使用 `http://localhost:8080/api/v1`
- 如果访问 `http://172.20.10.2:5173` → API 使用 `http://172.20.10.2:8080/api/v1`
- 如果访问 `https://your-domain.com` → API 使用 `https://your-domain.com/api/v1`

### 2. 手机访问配置步骤（简化版）

**现在无需配置环境变量！** 前端会自动使用你访问的域名/IP来构建 API 地址。

#### 步骤 1：获取电脑 IP 地址

**macOS/Linux:**
```bash
ifconfig | grep "inet " | grep -v 127.0.0.1
```

**Windows:**
```bash
ipconfig
```

找到你的局域网 IP 地址，例如：`192.168.1.100`

#### 步骤 2：启动后端服务器（监听所有网络接口）

后端已配置为默认监听 `0.0.0.0:8080`，支持网络访问：

```bash
cd backend
go run main.go
```

服务器将显示：`Server starting on 0.0.0.0:8080 (accessible from network)`

如果需要自定义监听地址，可以设置环境变量：
```bash
export SERVER_HOST=0.0.0.0
export SERVER_PORT=8080
go run main.go
```

#### 步骤 3：启动前端开发服务器

```bash
cd frontend
npm run dev
```

前端开发服务器也需要监听所有网络接口。在 `package.json` 中确认 dev 脚本：
```json
"dev": "vite --host 0.0.0.0"
```

如果没有 `--host 0.0.0.0`，需要添加它。

#### 步骤 4：确保手机和电脑在同一网络

- 手机和电脑必须连接到同一个 WiFi 网络
- 确保防火墙允许 8080 和 5173 端口访问

#### 步骤 5：手机访问

在手机浏览器中访问：
```
http://192.168.1.100:5173
```

（将 IP 地址替换为你的实际电脑 IP）

### 3. 高级配置（可选）

如果你需要自定义 API 地址（例如 API 在不同的端口或域名），可以创建环境变量文件：

```bash
cd frontend
cp .env.mobile .env.development.local
```

编辑 `.env.development.local`：

```env
VITE_API_BASE_URL=http://custom-api-server:9000/api/v1
```

然后重启前端服务器。

### 4. 生产环境配置

对于生产环境部署，创建 `.env.production` 文件：

```env
# 使用域名或公网 IP
VITE_API_BASE_URL=https://your-domain.com/api/v1
```

构建生产版本：
```bash
npm run build
```

## 环境变量优先级

1. `.env.local` - 本地覆盖（不提交到 Git）
2. `.env.development` - 开发环境
3. `.env.production` - 生产环境
4. 代码中的默认值 - `http://localhost:8080/api/v1`

## 故障排查

### 问题 1：手机仍然无法连接

**检查项：**
1. 确认电脑 IP 地址正确
2. 确认后端服务正在运行（`http://YOUR_IP:8080/health`）
3. 确认防火墙设置
4. 确认手机和电脑在同一网络

**macOS 防火墙设置：**
```bash
# 临时允许端口访问
sudo pfctl -d  # 禁用防火墙（测试用）
```

### 问题 2：CORS 错误

如果遇到 CORS 错误，检查后端 CORS 配置（`backend/internal/middleware/cors.go`）：

```go
config.AddAllowHeaders("Authorization", "Content-Type")
config.AddAllowOrigins("http://192.168.1.100:5173")  // 添加手机访问的源
```

### 问题 3：环境变量未生效

1. 确认 `.env.development.local` 文件在 `frontend` 目录下
2. **完全停止并重启开发服务器**（不是热重载）
   ```bash
   # 按 Ctrl+C 停止
   cd frontend
   npm run dev
   ```
3. 清除 Vite 缓存
   ```bash
   cd frontend
   rm -rf node_modules/.vite
   ```
4. 清除浏览器缓存
5. 检查环境变量是否正确加载：
   ```javascript
   console.log('API Base URL:', import.meta.env.VITE_API_BASE_URL);
   ```

**Vite 环境变量加载优先级**（从高到低）：
1. `.env.development.local` - 开发环境本地配置（**推荐用于手机访问**）
2. `.env.local` - 本地配置
3. `.env.development` - 开发环境配置
4. `.env` - 通用配置

## 测试验证

### 1. 测试后端连接

在手机浏览器中访问：
```
http://YOUR_COMPUTER_IP:8080/health
```

应该返回：
```json
{"status":"ok"}
```

### 2. 测试前端访问

在手机浏览器中访问：
```
http://YOUR_COMPUTER_IP:5173
```

应该能看到登录页面。

### 3. 测试登录功能

使用 admin 账户登录：
- 用户名：`admin`
- 密码：（你设置的密码）

## 安全建议

1. **不要将 `.env.local` 提交到 Git**
   - 已在 `.gitignore` 中配置
   
2. **生产环境使用 HTTPS**
   - 配置 SSL 证书
   - 使用反向代理（Nginx/Caddy）

3. **限制 API 访问**
   - 配置防火墙规则
   - 使用 VPN 或内网访问

## 相关文件

- `frontend/src/services/api.ts` - API 配置
- `frontend/.env.development` - 开发环境配置
- `frontend/.env.mobile` - 手机访问配置模板
- `frontend/.env.development.local` - 本地开发环境配置（手机访问时创建）
- `frontend/package.json` - 前端开发服务器配置
- `backend/main.go` - 后端服务器配置
- `backend/internal/middleware/cors.go` - CORS 配置

## 关键配置说明

### 前端配置
- `frontend/package.json`: `"dev": "vite --host 0.0.0.0"` - 前端监听所有网络接口
- `frontend/src/services/api.ts`: 支持通过 `VITE_API_BASE_URL` 环境变量配置 API 地址

### 后端配置
- `backend/main.go`: 默认监听 `0.0.0.0:8080` - 后端监听所有网络接口
- 可通过 `SERVER_HOST` 和 `SERVER_PORT` 环境变量自定义
