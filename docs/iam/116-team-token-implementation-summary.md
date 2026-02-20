# 团队Token功能实现总结

## 实现概述
已完成团队Token功能的后端实现，包括数据库表、服务层、处理器和路由配置。

## 已完成的工作

### 1. 数据库层
-  创建 `team_tokens` 表（scripts/create_team_tokens_table.sql）
-  表已存在于数据库中
-  包含所有必需字段和索引

### 2. 后端实现

#### 2.1 模型层
**文件**: `backend/internal/models/team_token.go`
-  TeamToken 模型
-  TeamTokenResponse 响应模型
-  TeamTokenCreateResponse 创建响应模型

#### 2.2 服务层
**文件**: `backend/internal/application/service/team_token_service.go`
-  TeamTokenService 服务
-  GenerateToken - 生成JWT token
-  ListTeamTokens - 列出团队tokens
-  RevokeToken - 吊销token
-  ValidateToken - 验证token
-  GetTokenByID - 获取token详情

**功能特性**:
- JWT格式token
- SHA256哈希存储
- 最多2个有效token限制
- Token过期时间（1年）
- 最后使用时间跟踪

#### 2.3 处理器层
**文件**: `backend/internal/handlers/team_token_handler.go`
-  TeamTokenHandler 处理器
-  CreateTeamToken - POST /api/v1/iam/teams/:id/tokens
-  ListTeamTokens - GET /api/v1/iam/teams/:id/tokens
-  RevokeTeamToken - DELETE /api/v1/iam/teams/:id/tokens/:token_id

#### 2.4 路由配置
**文件**: `backend/internal/router/router.go`
-  添加团队Token路由到IAM组
-  集成IAM权限检查
-  Admin角色绕过检查

### 3. 前端实现

#### 3.1 服务层
**文件**: `frontend/src/services/iam.ts`
-  createTeamToken - 创建token
-  listTeamTokens - 列出tokens
-  revokeTeamToken - 吊销token

## 待实现功能

### 前端UI组件
1. **TeamDetail页面** (`frontend/src/pages/admin/TeamDetail.tsx`)
   - 团队基本信息展示
   - 成员列表和管理
   - Token管理区域
   - 删除团队按钮

2. **Token管理组件**
   - 创建token对话框
   - Token列表展示
   - Token显示对话框（仅创建时显示一次）
   - 复制token功能
   - 吊销确认对话框

3. **路由配置**
   - 在 `frontend/src/App.tsx` 中添加 `/iam/teams/:id` 路由

## JWT Token格式

```json
{
  "team_id": 123,
  "team_name": "developers",
  "token_id": 456,
  "type": "team_token",
  "exp": 1234567890,
  "iat": 1234567890
}
```

## API端点

### 创建Token
```
POST /api/v1/iam/teams/:id/tokens
Body: { "token_name": "string" }
Response: {
  "message": "Token created successfully...",
  "token": {
    "id": 1,
    "team_id": 1,
    "token_name": "api-token",
    "token": "eyJhbGc...",  // JWT token (仅此时返回)
    "created_at": "2025-01-01T00:00:00Z",
    "expires_at": "2026-01-01T00:00:00Z"
  }
}
```

### 列出Tokens
```
GET /api/v1/iam/teams/:id/tokens
Response: {
  "tokens": [
    {
      "id": 1,
      "team_id": 1,
      "token_name": "api-token",
      "is_active": true,
      "created_at": "2025-01-01T00:00:00Z",
      "created_by": 1,
      "last_used_at": "2025-01-02T00:00:00Z",
      "expires_at": "2026-01-01T00:00:00Z"
    }
  ]
}
```

### 吊销Token
```
DELETE /api/v1/iam/teams/:id/tokens/:token_id
Response: {
  "message": "Token revoked successfully"
}
```

## 安全特性

1. **Token存储**: 仅存储SHA256哈希值，不存储明文
2. **一次性显示**: Token创建后仅显示一次
3. **数量限制**: 每个团队最多2个有效token
4. **权限控制**: 需要IAM_TEAMS权限
5. **过期时间**: Token有效期1年
6. **使用跟踪**: 记录最后使用时间

## 下一步工作

1. 创建TeamDetail页面组件
2. 实现Token管理UI
3. 添加前端路由
4. 集成到团队管理页面
5. 测试完整流程

## 注意事项

1. JWT secret key 当前硬编码为 "your-jwt-secret-key"，生产环境需要从配置文件读取
2. Token验证功能已实现但未集成到JWT中间件中
3. 前端需要提供明显的提示，告知用户token仅显示一次
4. 吊销操作需要二次确认
