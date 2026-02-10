# SSO 集成开发方案（无代码版）

## 一、Auth0 配置方案

### 1. Auth0 应用创建

**应用类型选择：Single Page Application (SPA)**

**原因：**
- React 是客户端渲染的 SPA
- 使用 Authorization Code Flow with PKCE（无需在前端暴露 Client Secret）
- 更安全，符合现代前端最佳实践

**配置步骤：**
```
1. 注册 Auth0 账号（https://auth0.com）
2. 创建新应用 → 选择 "Single Page Web Applications"
3. 记录关键信息：
   - Domain: dev-xxxxx.us.auth0.com
   - Client ID: abc123...

4. 配置回调 URL：
   - Allowed Callback URLs: http://localhost:3000/callback
   - Allowed Logout URLs: http://localhost:3000
   - Allowed Web Origins: http://localhost:3000

5. 启用社交登录（可选测试）：
   - Authentication → Social
   - 启用 Google / GitHub（使用 Auth0 默认密钥即可）

6. 创建测试用户：
   - User Management → Users → Create User
   - 设置邮箱和密码
```

---

## 二、整体架构方案

### 认证流程设计

```
【前端 React】          【后端 Golang】         【Auth0】
      |                      |                      |
  用户点击登录                |                      |
      |                      |                      |
  跳转到 Auth0 ---------------------------------> 显示登录页
      |                      |                      |
  用户输入凭证                |                  验证用户
      |                      |                      |
  Auth0 验证成功              |                      |
      |                      |                      |
  重定向回前端 /callback      |                      |
  (URL 带 code=xxx)          |                      |
      |                      |                      |
  提取 code，发送给后端        |                      |
      | -----------------> 接收 code                |
      |                      | ----------------> 用 code 换 token
      |                      | <---------------- 返回 access_token
      |                      |                      |
      |                      | ----------------> 获取用户信息
      |                      | <---------------- 返回用户资料
      |                      |                      |
      |                  查询数据库：                |
      |                  - 是否有邀请？             |
      |                  - 用户是否存在？           |
      |                  - 创建/更新用户            |
      |                  - 分配角色                |
      |                      |                      |
      | <----------------- 返回 JWT + 用户信息       |
      |                      |                      |
  存储 JWT 到 localStorage    |                      |
  跳转到首页                  |                      |
```

---

## 三、数据库设计方案

### 核心表结构

**1. users（用户表）**
- 存储平台自己的用户 ID
- 基本信息：邮箱、用户名、头像、状态
- 不直接存储 Auth0 的 ID

**2. user_identities（身份关联表）**
- 关联用户与外部身份提供商
- 关键字段：
  - user_id（外键到 users）
  - provider（auth0 / google / azure）
  - provider_user_id（Auth0 的 sub 字段）
  - provider_email
- 唯一索引：(provider, provider_user_id)

**3. roles（角色表）**
- 预设角色：guest, member, admin, owner
- 系统角色不可删除

**4. user_roles（用户角色关联表）**
- 支持多维度角色分配
- scope_type: global（全局）/ organization（组织）/ team（团队）
- 示例：用户可以是全局 guest，但在某个组织是 member

**5. invitations（邀请表）**
- 存储邀请记录
- 关键字段：
  - email（被邀请人）
  - role_id（预分配角色）
  - invitation_token（唯一令牌）
  - status（pending / accepted / expired）
  - expires_at（7天过期）

---

## 四、后端实现方案

### 项目结构设计

```
backend/
├── cmd/server/          # 主程序入口
├── internal/
│   ├── config/         # 配置加载
│   ├── handlers/       # HTTP 处理器
│   ├── middleware/     # 中间件（JWT 验证）
│   ├── models/         # 数据模型
│   ├── repository/     # 数据库操作
│   └── services/       # 业务逻辑
├── pkg/utils/          # 工具函数（JWT 生成等）
└── .env                # 环境变量
```

### 核心 API 端点设计

**1. POST /api/auth/callback**
- 接收参数：code（授权码）
- 处理流程：
  1. 用 code 向 Auth0 换取 access_token
  2. 用 access_token 获取用户信息（sub, email, name, picture）
  3. 查询 invitations 表：是否有该邮箱的邀请？
  4. 查询 user_identities 表：用户是否已存在？
  5. 决策逻辑：
     - **有邀请 + 新用户** → 创建用户 + 分配邀请中的角色 + 标记邀请已接受
     - **有邀请 + 老用户** → 分配邀请中的角色（追加权限）+ 标记邀请已接受
     - **无邀请 + 新用户** → 创建用户 + 分配默认 guest 角色
     - **无邀请 + 老用户** → 直接登录
  6. 生成平台自己的 JWT token
  7. 返回：{token, user}

**2. GET /api/auth/me**
- 需要 JWT 认证
- 返回当前登录用户信息和角色

**3. POST /api/auth/logout**
- 清除会话（可选实现）
- 返回成功状态

**4. POST /api/invitations**（管理员功能）
- 创建邀请
- 参数：email, role_id, organization_id
- 生成唯一 token
- 可选：发送邮件通知

**5. GET /api/invitations/:token**
- 验证邀请是否有效
- 返回邀请详情（用于前端显示）

### 环境变量配置

```
# Auth0
AUTH0_DOMAIN=dev-xxxxx.us.auth0.com
AUTH0_CLIENT_ID=你的ClientID
AUTH0_AUDIENCE=（可选，API标识符）

# 数据库
DB_HOST=localhost
DB_PORT=5432
DB_NAME=myapp
DB_USER=postgres
DB_PASSWORD=password

# JWT
JWT_SECRET=超长随机字符串
JWT_EXPIRY=24h

# 服务器
PORT=8080
ALLOWED_ORIGINS=http://localhost:3000
```

---

## 五、前端实现方案

### 技术选型

- React 18+
- React Router 6（路由管理）
- Auth0 React SDK（@auth0/auth0-react）
- Axios（HTTP 请求）
- Context API 或 Zustand（状态管理）

### 核心组件设计

**1. AuthProvider（认证上下文）**
- 包裹整个应用
- 提供登录、登出、用户状态
- 管理 JWT token 存储

**2. ProtectedRoute（受保护路由）**
- 检查用户是否登录
- 未登录 → 重定向到登录页
- 已登录但权限不足 → 显示 403

**3. LoginButton（登录按钮）**
- 点击后跳转到 Auth0 登录页
- 使用 Auth0 SDK 的 loginWithRedirect

**4. CallbackPage（回调页面）**
- 路由：/callback
- 接收 Auth0 重定向
- 提取 URL 中的 code
- 调用后端 /api/auth/callback
- 保存 JWT token
- 跳转到首页或原来的页面

**5. UserProfile（用户资料组件）**
- 显示用户信息
- 显示当前角色
- 提供登出按钮

### 路由设计

```
/                    # 首页（公开）
/login               # 登录页（公开）
/callback            # Auth0 回调页（公开）
/dashboard           # 控制台（需要登录）
/profile             # 个人资料（需要登录）
/admin               # 管理后台（需要 admin 角色）
/invitations/:token  # 接受邀请页面（公开）
```

### 状态管理设计

**AuthContext 提供的状态和方法：**
```
状态：
- user: 当前用户信息
- token: JWT token
- isAuthenticated: 是否已登录
- isLoading: 是否加载中
- roles: 用户角色列表

方法：
- login(): 跳转到 Auth0 登录
- logout(): 登出
- refreshUser(): 刷新用户信息
- hasRole(roleName): 检查是否有某个角色
```

### Token 存储方案

**推荐：localStorage**
```
存储内容：
{
  "token": "eyJhbGciOiJ...",
  "user": {
    "user_id": 1001,
    "email": "user@example.com",
    "display_name": "张三"
  }
}

每次 API 请求：
Authorization: Bearer eyJhbGciOiJ...
```

**安全考虑：**
- 设置 token 过期时间（24小时）
- 提供刷新 token 机制（可选）
- 敏感操作二次验证

---

## 六、测试方案

### 1. Auth0 本地测试

**测试场景 A：新用户注册（无邀请）**
```
步骤：
1. 启动后端（localhost:8080）
2. 启动前端（localhost:3000）
3. 点击"登录"按钮
4. 选择"Sign Up"，创建新账号
5. 验证：
   - 回到前端后，localStorage 有 token
   - 数据库 users 表新增一条记录
   - user_identities 表关联了 Auth0 的 sub
   - user_roles 表分配了 guest 角色
6. 刷新页面，验证登录状态保持
```

**测试场景 B：已有用户登录**
```
步骤：
1. 使用已注册的账号登录
2. 验证：
   - 正常登录
   - 数据库没有重复用户
   - user_identities 的 updated_at 更新
```

**测试场景 C：邀请用户登录**
```
准备：
1. 在数据库手动插入一条邀请记录：
   email = "invited@example.com"
   role_id = 2 (member)
   invitation_token = "abc123"
   status = "pending"
   expires_at = 7天后

测试：
1. 用 invited@example.com 登录
2. 验证：
   - 用户创建成功
   - user_roles 表分配了 member 角色（不是 guest）
   - invitations 表状态变为 "accepted"
   - accepted_at 和 accepted_by 更新
```

**测试场景 D：社交登录（Google）**
```
步骤：
1. Auth0 启用 Google 登录
2. 前端点击登录，选择 "Continue with Google"
3. 使用个人 Google 账号登录
4. 验证：
   - user_identities.provider = "google-oauth2"（Auth0 返回的）
   - 头像使用 Google 头像
```

### 2. API 测试

**使用 Postman / curl 测试：**

**测试 1：模拟回调**
```
POST http://localhost:8080/api/auth/callback
Content-Type: application/json

{
  "code": "从 Auth0 获取的真实 code"
}

预期返回：
{
  "token": "eyJhbGc...",
  "user": {
    "user_id": 1001,
    "email": "test@example.com",
    ...
  }
}
```

**测试 2：验证 JWT**
```
GET http://localhost:8080/api/auth/me
Authorization: Bearer eyJhbGc...

预期返回：
{
  "user_id": 1001,
  "email": "test@example.com",
  "roles": ["guest"]
}
```

**测试 3：创建邀请**
```
POST http://localhost:8080/api/invitations
Authorization: Bearer {admin_token}
Content-Type: application/json

{
  "email": "newuser@example.com",
  "role_id": 2,
  "organization_id": 5
}

预期返回：
{
  "invitation_id": 1,
  "invitation_token": "uuid...",
  "expires_at": "2025-02-16T10:00:00Z"
}
```

### 3. 端到端测试流程

**完整用户旅程：**
```
1. 管理员创建邀请
   → POST /api/invitations
   
2. 复制邀请链接
   → http://localhost:3000/invitations/abc123
   
3. 新用户打开链接
   → 显示邀请详情（邀请人、组织名、角色）
   
4. 点击"接受邀请并登录"
   → 跳转到 Auth0
   
5. 用户登录/注册
   → 回调到 /callback
   
6. 后端处理
   → 创建用户 + 分配邀请角色 + 标记邀请已接受
   
7. 前端跳转到对应组织的首页
   → 用户可以立即访问组织资源
```

### 4. 错误场景测试

**测试过期邀请：**
- 修改邀请的 expires_at 为过去时间
- 尝试登录，应该显示"邀请已过期"

**测试重复使用邀请：**
- 邀请已被接受（status = accepted）
- 再次用该邮箱登录，应该忽略邀请，正常登录

**测试无效 code：**
- 发送错误的授权码给后端
- 应该返回 400 错误

**测试 JWT 过期：**
- 使用过期的 JWT token 访问 /api/auth/me
- 应该返回 401 未授权

---

## 七、开发流程建议

### 第一阶段：Auth0 集成（1-2天）
```
后端：
1. 配置 Auth0 连接
2. 实现 /api/auth/callback 端点
3. 实现用户信息获取
4. 生成 JWT token

前端：
1. 安装 @auth0/auth0-react
2. 配置 Auth0Provider
3. 实现登录按钮
4. 实现回调页面
5. 测试登录流程
```

### 第二阶段：用户管理（2-3天）
```
后端：
1. 创建数据库表
2. 实现用户查询/创建逻辑
3. 实现身份关联逻辑
4. 实现 /api/auth/me 端点

前端：
1. 实现 AuthContext
2. 实现受保护路由
3. 显示用户信息
4. 实现登出功能
```

### 第三阶段：角色和邀请（3-4天）
```
后端：
1. 实现角色分配逻辑
2. 实现邀请 CRUD API
3. 实现邀请验证逻辑
4. 集成邀请到登录流程

前端：
1. 实现邀请管理界面（管理员）
2. 实现邀请接受页面
3. 实现角色显示
4. 实现权限检查
```

### 第四阶段：测试和优化（2-3天）
```
1. 完整流程测试
2. 边界情况处理
3. 错误提示优化
4. 性能优化
5. 安全加固
```

---

## 八、常见问题处理方案

### 问题 1：CORS 错误
**现象：**前端调用后端 API 被浏览器拦截

**解决：**
- 后端启用 CORS 中间件
- 配置允许的来源：http://localhost:3000
- 允许携带凭证（credentials）

### 问题 2：Auth0 回调失败
**现象：**登录后白屏或报错

**排查：**
- 检查 Auth0 配置的回调 URL 是否正确
- 检查前端路由 /callback 是否存在
- 查看浏览器控制台错误
- 检查后端日志

### 问题 3：Token 验证失败
**现象：**后端返回 401

**排查：**
- 检查 JWT secret 是否一致
- 检查 token 是否过期
- 检查 Authorization header 格式
- 验证 token 签名

### 问题 4：用户重复创建
**现象：**同一用户多次登录创建多个账号

**原因：**
- provider_user_id 没有正确提取
- 唯一索引未生效

**解决：**
- 确保从 Auth0 返回的 sub 字段正确存储
- 检查数据库唯一索引

---

## 九、安全最佳实践

### 1. Token 安全
- JWT 设置合理过期时间（24小时）
- 不在 token 中存储敏感信息
- 使用强随机 secret
- 生产环境定期轮换 secret

### 2. HTTPS
- 生产环境强制使用 HTTPS
- Auth0 回调 URL 使用 HTTPS
- Cookie 设置 Secure 标志

### 3. CSRF 防护
- State 参数验证（Auth0 SDK 自动处理）
- API 使用 CSRF token（如需要）

### 4. 数据验证
- 所有用户输入验证和清理
- SQL 使用参数化查询（防注入）
- 限制邮箱域名（如需要）

### 5. 日志和监控
- 记录所有登录尝试
- 记录邀请创建和接受
- 异常登录告警
- 定期审计用户权限

---

这个方案覆盖了从 Auth0 配置、数据库设计、前后端实现到测试的完整流程。你可以按照这个方案逐步实施，有任何具体环节需要深入讨论都可以问我。