# Agent Pool Secrets Implementation Plan

## 需求分析

### 功能需求
为Agent Pool添加密文数据存储功能，支持K/V配置管理，用于存储敏感信息（如API密钥、数据库密码等）。

### 核心要求
1. **K/V存储**: 支持键值对配置
2. **加密存储**: 使用hash加密存储密文数据
3. **独立表**: 密文数据存储在单独的表中
4. **全局唯一ID**: 每个密文数据有唯一标识符，格式为 `secret-{16位随机小写字母+数字}`
5. **安全性**: 
   - 前端不显示value值
   - API不返回value值（仅在创建时返回一次）
   - 使用AES-256-GCM加密

## 技术设计

### 1. 数据库设计

#### 新表: agent_pool_secrets

```sql
CREATE TABLE agent_pool_secrets (
    secret_id VARCHAR(50) PRIMARY KEY,  -- secret-{16位随机字符}
    pool_id VARCHAR(50) NOT NULL,
    key VARCHAR(100) NOT NULL,
    value_encrypted TEXT NOT NULL,  -- AES-256-GCM加密后的值
    description TEXT,
    created_by VARCHAR(50),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP,
    
    CONSTRAINT fk_pool FOREIGN KEY (pool_id) REFERENCES agent_pools(pool_id) ON DELETE CASCADE,
    CONSTRAINT unique_pool_key UNIQUE (pool_id, key)
);

CREATE INDEX idx_pool_secrets_pool_id ON agent_pool_secrets(pool_id);
CREATE INDEX idx_pool_secrets_key ON agent_pool_secrets(key);
CREATE INDEX idx_pool_secrets_created_at ON agent_pool_secrets(created_at);
```

### 2. 后端实现

#### 2.1 Model层 (backend/internal/models/agent_pool_secret.go)

```go
type AgentPoolSecret struct {
    SecretID       string    `gorm:"column:secret_id;primaryKey;type:varchar(50)" json:"secret_id"`
    PoolID         string    `gorm:"column:pool_id;type:varchar(50);not null;index" json:"pool_id"`
    Key            string    `gorm:"column:key;type:varchar(100);not null" json:"key"`
    ValueEncrypted string    `gorm:"column:value_encrypted;type:text;not null" json:"-"` // 不序列化到JSON
    Description    *string   `gorm:"column:description;type:text" json:"description,omitempty"`
    CreatedBy      *string   `gorm:"column:created_by;type:varchar(50)" json:"created_by,omitempty"`
    CreatedAt      time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"created_at"`
    UpdatedAt      time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updated_at"`
    LastUsedAt     *time.Time `gorm:"column:last_used_at" json:"last_used_at,omitempty"`
}

// 创建时的请求
type CreateSecretRequest struct {
    Key         string  `json:"key" binding:"required,max=100"`
    Value       string  `json:"value" binding:"required"`
    Description *string `json:"description"`
}

// 创建时的响应（仅此时返回明文value）
type CreateSecretResponse struct {
    SecretID    string    `json:"secret_id"`
    PoolID      string    `json:"pool_id"`
    Key         string    `json:"key"`
    Value       string    `json:"value"` // 仅创建时返回
    Description *string   `json:"description,omitempty"`
    CreatedAt   time.Time `json:"created_at"`
}

// 列表响应（不包含value）
type SecretResponse struct {
    SecretID    string     `json:"secret_id"`
    PoolID      string     `json:"pool_id"`
    Key         string     `json:"key"`
    Description *string    `json:"description,omitempty"`
    CreatedBy   *string    `json:"created_by,omitempty"`
    CreatedAt   time.Time  `json:"created_at"`
    UpdatedAt   time.Time  `json:"updated_at"`
    LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
}

// 更新请求（不允许更新value，只能删除重建）
type UpdateSecretRequest struct {
    Description *string `json:"description"`
}
```

#### 2.2 加密工具 (复用 backend/internal/crypto/variable_crypto.go)

现有的加密工具已经实现了AES-256-GCM加密，可以直接复用：
- `EncryptValue(plaintext string) (string, error)`
- `DecryptValue(ciphertext string) (string, error)`
- `IsEncrypted(value string) bool`

#### 2.3 ID生成工具 (backend/internal/utils/id_generator.go)

```go
func GenerateSecretID() string {
    const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
    const length = 16
    
    b := make([]byte, length)
    for i := range b {
        b[i] = charset[rand.Intn(len(charset))]
    }
    
    return "secret-" + string(b)
}
```

#### 2.4 Handler层 (backend/internal/handlers/agent_pool_secret_handler.go)

```go
// API端点:
// POST   /api/v1/agent-pools/:poolId/secrets          - 创建密文
// GET    /api/v1/agent-pools/:poolId/secrets          - 列出密文（不含value）
// GET    /api/v1/agent-pools/:poolId/secrets/:secretId - 获取密文详情（不含value）
// PUT    /api/v1/agent-pools/:poolId/secrets/:secretId - 更新密文（仅description）
// DELETE /api/v1/agent-pools/:poolId/secrets/:secretId - 删除密文
```

### 3. 前端实现

#### 3.1 API Service (frontend/src/services/agent.ts)

```typescript
export interface AgentPoolSecret {
  secret_id: string;
  pool_id: string;
  key: string;
  description?: string;
  created_by?: string;
  created_at: string;
  updated_at: string;
  last_used_at?: string;
}

export interface CreateSecretRequest {
  key: string;
  value: string;
  description?: string;
}

export interface CreateSecretResponse extends AgentPoolSecret {
  value: string; // 仅创建时返回
}

export const poolSecretAPI = {
  list: (poolId: string) => 
    api.get<{ secrets: AgentPoolSecret[] }>(`/agent-pools/${poolId}/secrets`),
  
  create: (poolId: string, data: CreateSecretRequest) => 
    api.post<CreateSecretResponse>(`/agent-pools/${poolId}/secrets`, data),
  
  update: (poolId: string, secretId: string, data: { description?: string }) => 
    api.put(`/agent-pools/${poolId}/secrets/${secretId}`, data),
  
  delete: (poolId: string, secretId: string) => 
    api.delete(`/agent-pools/${poolId}/secrets/${secretId}`),
};
```

#### 3.2 UI组件 (在 AgentPoolDetail.tsx 中添加)

新增一个 "Secrets" section，包含：
1. **列表展示**: 显示key、description、创建时间等（不显示value）
2. **创建对话框**: 
   - 输入key、value、description
   - 创建成功后显示value（仅此一次）
   - 提示用户复制保存
3. **删除确认**: 删除前二次确认
4. **安全提示**: 明确告知用户value仅在创建时显示一次

### 4. 安全考虑

#### 4.1 加密方式
- 使用AES-256-GCM加密算法
- 密钥来源: JWT_SECRET的SHA256哈希
- 每次加密使用随机nonce
- 加密后使用base64编码存储

#### 4.2 访问控制
- 仅pool的创建者和管理员可以管理secrets
- API需要验证用户权限
- 审计日志记录所有操作

#### 4.3 数据保护
- value字段在model中标记为`json:"-"`，不会被序列化
- API响应中不包含value字段（除创建时）
- 前端不缓存value值
- 创建成功后的value显示使用一次性对话框

### 5. 使用场景

Agent Pool Secrets可用于：
1. **K8s环境变量**: 在K8s配置中引用secrets作为环境变量
2. **Agent认证**: 存储第三方服务的API密钥
3. **数据库连接**: 存储数据库密码
4. **云服务凭证**: 存储AWS/Azure/GCP等云服务凭证

### 6. 实现步骤

1. **数据库迁移** (scripts/create_agent_pool_secrets_table.sql)
   - 创建agent_pool_secrets表
   - 创建索引
   - 添加外键约束

2. **后端实现**
   - 创建Model (agent_pool_secret.go)
   - 实现ID生成工具
   - 实现Handler (agent_pool_secret_handler.go)
   - 添加路由
   - 添加权限验证

3. **前端实现**
   - 扩展API service
   - 在AgentPoolDetail页面添加Secrets section
   - 实现创建/删除对话框
   - 添加安全提示

4. **测试**
   - 单元测试: 加密/解密功能
   - 集成测试: API端点
   - E2E测试: 前端操作流程
   - 安全测试: 验证value不会泄露

### 7. 注意事项

1. **不支持更新value**: 如需修改value，必须删除后重新创建
2. **value仅显示一次**: 创建成功后立即显示，之后无法再次查看
3. **删除不可恢复**: 删除secret后无法恢复，需要二次确认
4. **加密密钥管理**: JWT_SECRET必须妥善保管，泄露会导致所有加密数据不安全
5. **审计日志**: 所有操作都应记录到审计日志

## 与现有功能的对比

### Workspace Variables vs Agent Pool Secrets

| 特性 | Workspace Variables | Agent Pool Secrets |
|------|-------------------|-------------------|
| 作用域 | Workspace级别 | Agent Pool级别 |
| 用途 | Terraform变量/环境变量 | Agent配置/凭证 |
| 加密 | 敏感变量加密 | 全部加密 |
| 查看 | 非敏感变量可查看 | 创建后不可查看 |
| 更新 | 支持更新value | 不支持更新value |
| 类型 | terraform/environment | 统一为secret |

## API文档

### 创建Secret

```
POST /api/v1/agent-pools/:poolId/secrets
Content-Type: application/json

Request:
{
  "key": "AWS_ACCESS_KEY",
  "value": "AKIAIOSFODNN7EXAMPLE",
  "description": "AWS access key for S3"
}

Response (201):
{
  "secret_id": "secret-a1b2c3d4e5f6g7h8",
  "pool_id": "pool-xyz123",
  "key": "AWS_ACCESS_KEY",
  "value": "AKIAIOSFODNN7EXAMPLE",  // 仅此时返回
  "description": "AWS access key for S3",
  "created_at": "2025-01-04T11:30:00Z"
}
```

### 列出Secrets

```
GET /api/v1/agent-pools/:poolId/secrets

Response (200):
{
  "secrets": [
    {
      "secret_id": "secret-a1b2c3d4e5f6g7h8",
      "pool_id": "pool-xyz123",
      "key": "AWS_ACCESS_KEY",
      "description": "AWS access key for S3",
      "created_by": "user-123",
      "created_at": "2025-01-04T11:30:00Z",
      "updated_at": "2025-01-04T11:30:00Z",
      "last_used_at": null
    }
  ],
  "total": 1
}
```

### 删除Secret

```
DELETE /api/v1/agent-pools/:poolId/secrets/:secretId

Response (200):
{
  "message": "Secret deleted successfully"
}
```

## 总结

该设计方案：
1.  满足K/V配置需求
2.  使用AES-256-GCM加密存储
3.  独立表存储
4.  全局唯一ID (secret-{16位随机字符})
5.  前端不显示value
6.  API不返回value（除创建时）
7.  复用现有加密基础设施
8.  遵循现有代码规范和架构模式
