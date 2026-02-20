# 个人设置功能实现指南

## 概述

本文档描述了个人设置功能的完整实现，包括用户密码修改和个人访问Token管理。

## 功能特性

### 1. 个人设置页面
- 统一的个人设置入口，替代原有的弹窗方式
- 标签页切换设计，包含"修改密码"和"访问Token"两个功能模块
- 访问路径：`/settings`

### 2. 密码修改
- 用户可以修改自己的密码
- 需要验证当前密码
- 新密码长度至少6个字符
- 需要确认新密码

### 3. 访问Token管理
- 用户可以创建个人访问Token用于API调用
- Token使用语义化ID格式：`token-{8-15位随机小写字母+数字}`
- 每个用户最多可创建5个有效Token
- Token支持设置过期时间（30/60/90/180/365天或永不过期）
- Token创建后仅显示一次，需要用户立即保存
- 支持查看Token列表（名称、ID、状态、创建时间、最后使用时间、过期时间）
- 支持吊销Token

## 数据库设计

### user_tokens 表结构

```sql
CREATE TABLE user_tokens (
    token_id VARCHAR(30) PRIMARY KEY,  -- token-{8-15位小写字母+数字}
    user_id VARCHAR(20) NOT NULL,
    token_name VARCHAR(100) NOT NULL,
    token_hash VARCHAR(255) NOT NULL UNIQUE,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at TIMESTAMP,
    last_used_at TIMESTAMP,
    expires_at TIMESTAMP,
    CONSTRAINT fk_user_tokens_user FOREIGN KEY (user_id) REFERENCES users(user_id) ON DELETE CASCADE
);
```

### 索引
- `idx_user_tokens_user_id`: 用户ID索引
- `idx_user_tokens_is_active`: 状态索引
- `idx_user_tokens_token_hash`: Token哈希索引

## 后端实现

### 1. ID生成器 (`backend/internal/infrastructure/id_generator.go`)

新增函数：
```go
// GenerateTokenID 生成Token ID
// 格式: token-{8-15位随机小写字母+数字}
func GenerateTokenID() (string, error)

// ValidateTokenID 验证Token ID格式
func ValidateTokenID(id string) bool
```

### 2. 数据模型 (`backend/internal/models/user_token.go`)

- `UserToken`: 用户Token模型
- `UserTokenResponse`: Token响应模型（列表展示）
- `UserTokenCreateResponse`: Token创建响应（包含明文token）

### 3. 服务层 (`backend/internal/application/service/user_token_service.go`)

主要方法：
- `GenerateToken()`: 生成用户Token
- `ListUserTokens()`: 列出用户的所有Token
- `RevokeToken()`: 吊销Token
- `ValidateToken()`: 验证Token
- `GetTokenByID()`: 根据ID获取Token信息

Token限制：
- 每个用户最多5个有效Token
- Token使用JWT格式，包含用户信息
- Token哈希值使用SHA256算法

### 4. 处理器 (`backend/internal/handlers/user_token_handler.go`)

API端点：
- `POST /api/v1/user/tokens`: 创建Token
- `GET /api/v1/user/tokens`: 列出Token
- `DELETE /api/v1/user/tokens/:token_id`: 吊销Token
- `POST /api/v1/user/change-password`: 修改密码

### 5. 路由配置 (`backend/internal/router/router_user.go`)

所有个人设置相关的路由都在 `/api/v1/user` 路径下。

## 前端实现

### 1. 个人设置页面 (`frontend/src/pages/PersonalSettings.tsx`)

组件特性：
- 使用React Hooks管理状态
- 标签页切换（密码修改/Token管理）
- 表单验证
- 错误处理和成功提示
- Token创建后的安全提示

### 2. 样式文件 (`frontend/src/pages/PersonalSettings.module.css`)

设计特点：
- 响应式布局
- 清晰的视觉层次
- 友好的交互反馈
- 模态框设计
- 表格展示

### 3. 路由配置 (`frontend/src/App.tsx`)

路由路径：`/settings`

## 部署步骤

### 1. 运行数据库迁移

```bash
# 进入项目目录
cd /Users/ken/go/src/iac-platform

# 执行SQL迁移脚本
psql -U postgres -d iac_platform -f scripts/create_user_tokens_table.sql
```

### 2. 重启后端服务

```bash
# 进入backend目录
cd backend

# 重新编译并运行
go run main.go
```

### 3. 重启前端服务

```bash
# 进入frontend目录
cd frontend

# 重新启动开发服务器
npm run dev
```

## 使用说明

### 访问个人设置

1. 登录系统后，点击用户菜单中的"个人设置"选项
2. 或直接访问 `/settings` 路径

### 修改密码

1. 在个人设置页面选择"修改密码"标签
2. 输入当前密码
3. 输入新密码（至少6个字符）
4. 确认新密码
5. 点击"修改密码"按钮

### 创建访问Token

1. 在个人设置页面选择"访问Token"标签
2. 点击"创建新Token"按钮
3. 输入Token名称（例如：CI/CD Pipeline）
4. 选择过期时间
5. 点击"创建Token"
6. **重要**：立即复制并保存显示的Token，关闭后将无法再次查看

### 使用Token进行API调用

```bash
# 使用Token进行API调用示例
curl -H "Authorization: Bearer <your-token>" \
     https://api.example.com/api/v1/workspaces
```

### 吊销Token

1. 在Token列表中找到要吊销的Token
2. 点击"吊销"按钮
3. 确认吊销操作
4. 吊销后的Token将立即失效

## 安全考虑

1. **Token存储**：Token的哈希值存储在数据库中，明文Token仅在创建时返回一次
2. **Token验证**：每次API调用都会验证Token的有效性、是否过期、是否被吊销
3. **密码验证**：修改密码时需要验证当前密码
4. **数量限制**：每个用户最多5个有效Token，防止滥用
5. **过期机制**：Token支持设置过期时间，建议定期更换
6. **审计日志**：Token的创建和吊销操作应该记录到审计日志中

## API文档

### 创建Token

**请求**：
```http
POST /api/v1/user/tokens
Content-Type: application/json

{
  "token_name": "CI/CD Pipeline",
  "expires_in_days": 90
}
```

**响应**：
```json
{
  "message": "Token created successfully",
  "data": {
    "token_id": "token-abc123xyz",
    "user_id": "user-1234567890",
    "token_name": "CI/CD Pipeline",
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "created_at": "2025-10-26T17:00:00Z",
    "expires_at": "2026-01-24T17:00:00Z"
  }
}
```

### 列出Token

**请求**：
```http
GET /api/v1/user/tokens
```

**响应**：
```json
{
  "data": [
    {
      "token_id": "token-abc123xyz",
      "user_id": "user-1234567890",
      "token_name": "CI/CD Pipeline",
      "is_active": true,
      "created_at": "2025-10-26T17:00:00Z",
      "last_used_at": "2025-10-26T18:00:00Z",
      "expires_at": "2026-01-24T17:00:00Z"
    }
  ]
}
```

### 吊销Token

**请求**：
```http
DELETE /api/v1/user/tokens/token-abc123xyz
```

**响应**：
```json
{
  "message": "Token revoked successfully"
}
```

### 修改密码

**请求**：
```http
POST /api/v1/user/change-password
Content-Type: application/json

{
  "old_password": "oldpass123",
  "new_password": "newpass456"
}
```

**响应**：
```json
{
  "message": "Password changed successfully"
}
```

## 故障排查

### Token创建失败

1. 检查是否已达到5个Token的限制
2. 检查Token名称是否为空
3. 检查过期天数是否在有效范围内（0-365）

### Token验证失败

1. 检查Token是否已过期
2. 检查Token是否已被吊销
3. 检查Token格式是否正确
4. 检查JWT密钥配置是否正确

### 密码修改失败

1. 检查当前密码是否正确
2. 检查新密码长度是否至少6个字符
3. 检查新密码和确认密码是否一致

## 未来改进

1. **密码修改功能完善**：当前密码修改功能的后端逻辑需要完善（TODO标记）
2. **Token使用统计**：添加Token使用次数统计
3. **Token权限范围**：支持为Token设置特定的权限范围
4. **多因素认证**：添加MFA支持
5. **密码强度检查**：添加更严格的密码强度验证
6. **会话管理**：添加活跃会话管理功能

## 相关文件

### 后端文件
- `backend/internal/infrastructure/id_generator.go`
- `backend/internal/models/user_token.go`
- `backend/internal/application/service/user_token_service.go`
- `backend/internal/handlers/user_token_handler.go`
- `backend/internal/router/router_user.go`
- `scripts/create_user_tokens_table.sql`

### 前端文件
- `frontend/src/pages/PersonalSettings.tsx`
- `frontend/src/pages/PersonalSettings.module.css`
- `frontend/src/App.tsx`

## 总结

个人设置功能提供了一个统一的用户自助服务入口，用户可以方便地管理自己的密码和访问Token。该功能采用了现代化的设计模式，注重安全性和用户体验，为后续的功能扩展奠定了良好的基础。
