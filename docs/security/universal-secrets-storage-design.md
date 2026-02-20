# 通用Secrets存储系统设计

## 设计理念

设计一个通用的secrets存储表，可以被多个模块（Agent Pool、Workspace、Module等）复用，而不是为每个模块单独创建secrets表。

## 数据库设计

### 核心表: secrets

```sql
CREATE TABLE secrets (
    id SERIAL PRIMARY KEY,                              -- 自增ID（数据库内部使用）
    secret_id VARCHAR(50) UNIQUE NOT NULL,              -- secret-{16位随机小写字母+数字}（对外暴露的ID）
    value_hash TEXT NOT NULL,                           -- AES-256-GCM加密后的hash值
    resource_type VARCHAR(50) NOT NULL,                 -- 资源类型: agent_pool, workspace, module, system等
    resource_id VARCHAR(50),                            -- 关联的资源ID（可为空，用于system级别的secrets）
    created_by VARCHAR(50),                             -- 创建者ID
    updated_by VARCHAR(50),                             -- 最后更新者ID
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,     -- 创建时间
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,     -- 更新时间
    last_used_at TIMESTAMP,                             -- 最后使用时间
    expires_at TIMESTAMP,                               -- 过期时间（可选）
    is_active BOOLEAN DEFAULT true,                     -- 是否激活
    metadata JSONB,                                     -- 元数据（存储key、description等扩展信息）
    
    CONSTRAINT unique_resource_key UNIQUE (resource_type, resource_id, metadata->>'key')
);

-- 索引
CREATE INDEX idx_secrets_secret_id ON secrets(secret_id);
CREATE INDEX idx_secrets_resource ON secrets(resource_type, resource_id);
CREATE INDEX idx_secrets_created_by ON secrets(created_by);
CREATE INDEX idx_secrets_is_active ON secrets(is_active);
CREATE INDEX idx_secrets_expires_at ON secrets(expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX idx_secrets_metadata_gin ON secrets USING GIN(metadata);
```

## 字段说明

### 必需字段

1. **id** (SERIAL PRIMARY KEY)
   - 数据库自增ID
   - 仅用于数据库内部关联和性能优化
   - 不对外暴露

2. **secret_id** (VARCHAR(50) UNIQUE NOT NULL)
   - 格式: `secret-{16位随机小写字母+数字}`
   - 全局唯一标识符
   - 对外暴露的主要ID
   - 示例: `secret-a1b2c3d4e5f6g7h8`

3. **value_hash** (TEXT NOT NULL)
   - AES-256-GCM加密后的密文值
   - 使用base64编码存储
   - 永不对外返回（除创建时返回明文）

4. **resource_type** (VARCHAR(50) NOT NULL)
   - 标识secret所属的资源类型
   - 可选值: `agent_pool`, `workspace`, `module`, `system`, `team`, `user`
   - 用于多租户隔离和权限控制

5. **resource_id** (VARCHAR(50))
   - 关联的资源ID
   - 可为NULL（用于system级别的全局secrets）
   - 示例: `pool-xyz123`, `workspace-abc456`

### 审计字段

6. **created_by** (VARCHAR(50))
   - 创建者的user_id
   - 用于审计追踪
   - 可为NULL（系统创建的secrets）

7. **updated_by** (VARCHAR(50))
   - 最后更新者的user_id
   - 用于审计追踪
   - 每次更新metadata时更新此字段

### 时间字段

8. **created_at** (TIMESTAMP DEFAULT CURRENT_TIMESTAMP)
   - 创建时间
   - 自动设置

9. **updated_at** (TIMESTAMP DEFAULT CURRENT_TIMESTAMP)
   - 最后更新时间
   - 每次更新时自动更新

10. **last_used_at** (TIMESTAMP)
    - 最后使用时间
    - 当secret被读取/使用时更新
    - 用于识别未使用的secrets

11. **expires_at** (TIMESTAMP)
    - 过期时间（可选）
    - 支持临时secrets
    - 过期后自动失效或清理

### 状态和元数据字段

12. **is_active** (BOOLEAN DEFAULT true)
    - 是否激活
    - 支持软删除和临时禁用
    - false时secret不可用但保留记录

13. **metadata** (JSONB)
    - 存储扩展信息的JSON字段
    - 灵活支持不同场景的需求
    - 建议包含的字段:
      ```json
      {
        "key": "AWS_ACCESS_KEY",           // secret的key名称
        "description": "AWS S3 access key", // 描述
        "tags": ["production", "aws"],      // 标签
        "rotation_policy": {                // 轮转策略
          "enabled": true,
          "interval_days": 90
        },
        "access_count": 0,                  // 访问次数
        "last_rotation_at": "2025-01-01T00:00:00Z"
      }
      ```

## 建议添加的字段

基于您提到的字段，我建议以下额外字段可能有用：

### 14. **version** (INTEGER DEFAULT 1)
```sql
version INTEGER DEFAULT 1
```
- 版本号，支持secret的版本管理
- 每次更新value时递增
- 可以保留历史版本（需要额外的history表）

### 15. **rotation_required** (BOOLEAN DEFAULT false)
```sql
rotation_required BOOLEAN DEFAULT false
```
- 标记是否需要轮转
- 配合rotation_policy使用
- 自动化轮转提醒

### 16. **access_policy** (JSONB)
```sql
access_policy JSONB
```
- 访问策略配置
- 定义谁可以访问这个secret
- 示例:
  ```json
  {
    "allowed_users": ["user-123", "user-456"],
    "allowed_teams": ["team-ops"],
    "allowed_roles": ["admin", "operator"],
    "ip_whitelist": ["10.0.0.0/8"]
  }
  ```

## 使用示例

### Agent Pool Secret
```sql
INSERT INTO secrets (
    secret_id, 
    value_hash, 
    resource_type, 
    resource_id, 
    created_by,
    metadata
) VALUES (
    'secret-a1b2c3d4e5f6g7h8',
    'base64_encrypted_value...',
    'agent_pool',
    'pool-xyz123',
    'user-admin',
    '{"key": "AWS_ACCESS_KEY", "description": "AWS access key for agents"}'
);
```

### Workspace Secret
```sql
INSERT INTO secrets (
    secret_id, 
    value_hash, 
    resource_type, 
    resource_id, 
    created_by,
    metadata
) VALUES (
    'secret-b2c3d4e5f6g7h8i9',
    'base64_encrypted_value...',
    'workspace',
    'workspace-abc456',
    'user-dev',
    '{"key": "DB_PASSWORD", "description": "Database password"}'
);
```

### System-level Secret
```sql
INSERT INTO secrets (
    secret_id, 
    value_hash, 
    resource_type, 
    resource_id, 
    created_by,
    metadata
) VALUES (
    'secret-c3d4e5f6g7h8i9j0',
    'base64_encrypted_value...',
    'system',
    NULL,  -- system级别无resource_id
    'user-admin',
    '{"key": "ENCRYPTION_KEY", "description": "System encryption key"}'
);
```

## Model设计

```go
type Secret struct {
    ID            uint       `gorm:"column:id;primaryKey;autoIncrement" json:"-"`
    SecretID      string     `gorm:"column:secret_id;type:varchar(50);uniqueIndex;not null" json:"secret_id"`
    ValueHash     string     `gorm:"column:value_hash;type:text;not null" json:"-"` // 永不序列化
    ResourceType  string     `gorm:"column:resource_type;type:varchar(50);not null;index:idx_resource" json:"resource_type"`
    ResourceID    *string    `gorm:"column:resource_id;type:varchar(50);index:idx_resource" json:"resource_id,omitempty"`
    CreatedBy     *string    `gorm:"column:created_by;type:varchar(50);index" json:"created_by,omitempty"`
    UpdatedBy     *string    `gorm:"column:updated_by;type:varchar(50)" json:"updated_by,omitempty"`
    CreatedAt     time.Time  `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"created_at"`
    UpdatedAt     time.Time  `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updated_at"`
    LastUsedAt    *time.Time `gorm:"column:last_used_at" json:"last_used_at,omitempty"`
    ExpiresAt     *time.Time `gorm:"column:expires_at;index" json:"expires_at,omitempty"`
    IsActive      bool       `gorm:"column:is_active;default:true;index" json:"is_active"`
    Metadata      datatypes.JSON `gorm:"column:metadata;type:jsonb" json:"metadata,omitempty"`
}

// Metadata结构
type SecretMetadata struct {
    Key              string            `json:"key"`
    Description      string            `json:"description,omitempty"`
    Tags             []string          `json:"tags,omitempty"`
    RotationPolicy   *RotationPolicy   `json:"rotation_policy,omitempty"`
    AccessCount      int               `json:"access_count,omitempty"`
    LastRotationAt   *time.Time        `json:"last_rotation_at,omitempty"`
}

type RotationPolicy struct {
    Enabled      bool `json:"enabled"`
    IntervalDays int  `json:"interval_days"`
}
```

## 优势

1. **通用性**: 一个表支持所有类型的secrets
2. **可扩展**: metadata字段支持灵活的扩展
3. **多租户**: resource_type + resource_id 实现资源隔离
4. **权限控制**: 通过resource_type和resource_id实现基于资源的权限控制
5. **审计完整**: 完整的创建/更新/使用追踪
6. **生命周期管理**: 支持过期、轮转、软删除
7. **性能优化**: 合理的索引设计

## 与专用表的对比

| 特性 | 通用表 (secrets) | 专用表 (agent_pool_secrets) |
|------|-----------------|---------------------------|
| 复用性 |  高 - 所有模块共用 | ❌ 低 - 仅agent pool使用 |
| 维护成本 |  低 - 统一管理 | ❌ 高 - 多表维护 |
| 查询复杂度 |  需要过滤resource_type |  简单直接 |
| 扩展性 |  高 - metadata灵活 |  需要修改schema |
| 性能 |  数据量大时需要分区 |  数据量小性能好 |
| 权限控制 |  统一的权限模型 |  每个表独立实现 |

## 建议

基于您的需求，我建议采用**通用表设计**，原因：

1.  符合您"做成通用存储，后面可以复用"的要求
2.  统一的secrets管理和审计
3.  减少代码重复，统一加密/解密逻辑
4.  便于实现跨资源的secrets管理功能
5.  metadata字段提供足够的灵活性

## 总结

**最小必需字段集**（您提到的）：
-  id (自增ID)
-  secret_id (secret-{16位随机字符})
-  value_hash (加密的hash值)
-  created_by
-  updated_by
-  created_at
-  updated_at

**建议添加的字段**（增强功能）：
-  resource_type (资源类型 - 必需)
-  resource_id (资源ID - 必需)
-  last_used_at (使用追踪)
-  expires_at (过期管理)
-  is_active (状态管理)
-  metadata (灵活扩展)

这样的设计既满足了您的基本需求，又提供了足够的扩展性和通用性。
