# 第三方CMDB数据同步功能设计方案

## 1. 功能概述

### 1.1 目标
支持从第三方CMDB系统同步资源数据到本平台的CMDB资源索引中，实现统一的资源管理和搜索能力。

### 1.2 核心需求
1. **支持HTTP API同步** - 通过REST API从第三方CMDB拉取数据
2. **Header认证** - 支持自定义Header进行API认证
3. **敏感信息保护** - Header的值需要加密存储，不可查看
4. **灵活配置** - Header的key可以被用户自定义编辑

## 2. 数据存储方案

### 2.1 方案对比

| 方案 | 优点 | 缺点 |
|------|------|------|
| **方案A: 新建独立表** | 结构清晰，易于扩展 | 需要新建表 |
| **方案B: 复用secrets表** | 复用现有加密机制 | 需要额外的配置表 |
| **方案C: 混合方案** | 配置与密钥分离，安全性高 | 需要两个表配合 |

### 2.2 推荐方案：混合方案（方案C）

创建两个表：
1. **`cmdb_external_sources`** - 存储外部数据源配置（非敏感信息）
2. **复用 `secrets` 表** - 存储Header的敏感值（加密）

#### 2.2.1 cmdb_external_sources 表结构

```sql
CREATE TABLE IF NOT EXISTS cmdb_external_sources (
    id SERIAL PRIMARY KEY,
    source_id VARCHAR(50) UNIQUE NOT NULL,           -- 唯一标识: cmdb-src-{随机字符}
    name VARCHAR(100) NOT NULL,                       -- 数据源名称
    description TEXT,                                 -- 描述
    
    -- API配置
    api_endpoint VARCHAR(500) NOT NULL,               -- API端点URL
    http_method VARCHAR(10) DEFAULT 'GET',            -- HTTP方法: GET/POST
    request_body TEXT,                                -- POST请求体模板（可选）
    
    -- 认证配置（Header）
    auth_headers JSONB,                               -- Header配置: [{"key": "X-API-Key", "secret_id": "secret-xxx"}, ...]
    
    -- 数据映射配置
    response_path VARCHAR(200),                       -- 响应数据路径（JSONPath），如 "$.data.resources"
    field_mapping JSONB NOT NULL,                     -- 字段映射配置
    
    -- 主键配置（新增）
    primary_key_field VARCHAR(100) NOT NULL,          -- 主键字段路径，如 "$.id" 或 "$.name"
    
    -- 云环境配置（新增）
    cloud_provider VARCHAR(50),                       -- 云提供商: aws/azure/gcp/aliyun 等（用户输入）
    account_id VARCHAR(100),                          -- 云账户ID（用户输入）
    account_name VARCHAR(200),                        -- 云账户名称（用户输入，可选）
    region VARCHAR(50),                               -- 区域（用户输入，可选）
    
    -- 同步配置
    sync_interval_minutes INT DEFAULT 60,             -- 同步间隔（分钟），0表示手动同步
    is_enabled BOOLEAN DEFAULT true,                  -- 是否启用
    
    -- 过滤配置
    resource_type_filter VARCHAR(100),                -- 资源类型过滤（可选）
    
    -- 元数据
    organization_id VARCHAR(50),                      -- 所属组织（可选，用于多租户）
    created_by VARCHAR(50),
    updated_by VARCHAR(50),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_sync_at TIMESTAMP,                           -- 最后同步时间
    last_sync_status VARCHAR(20),                     -- 最后同步状态: success/failed/running
    last_sync_message TEXT,                           -- 最后同步消息
    last_sync_count INT DEFAULT 0                     -- 最后同步资源数量
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_cmdb_external_sources_org ON cmdb_external_sources(organization_id);
CREATE INDEX IF NOT EXISTS idx_cmdb_external_sources_enabled ON cmdb_external_sources(is_enabled);
CREATE INDEX IF NOT EXISTS idx_cmdb_external_sources_provider ON cmdb_external_sources(cloud_provider);
CREATE INDEX IF NOT EXISTS idx_cmdb_external_sources_account ON cmdb_external_sources(account_id);
```

#### 2.2.2 字段映射配置示例

```json
{
  "field_mapping": {
    "resource_type": "$.type",                    // 资源类型字段路径
    "resource_name": "$.name",                    // 资源名称字段路径
    "cloud_resource_id": "$.id",                  // 云资源ID字段路径
    "cloud_resource_name": "$.displayName",       // 云资源名称字段路径
    "cloud_resource_arn": "$.arn",                // ARN字段路径（可选）
    "description": "$.description",               // 描述字段路径（可选）
    "tags": "$.tags",                             // 标签字段路径（可选）
    "attributes": "$.attributes"                  // 额外属性字段路径（可选）
  }
}
```

#### 2.2.3 认证Header配置示例

```json
{
  "auth_headers": [
    {
      "key": "X-API-Key",
      "secret_id": "secret-abc123def456"          // 引用secrets表中的记录
    },
    {
      "key": "Authorization",
      "secret_id": "secret-xyz789ghi012"
    }
  ]
}
```

### 2.3 secrets表复用

Header的敏感值存储在现有的`secrets`表中：

```sql
-- 示例记录
INSERT INTO secrets (secret_id, value_hash, resource_type, resource_id, metadata) VALUES
('secret-abc123def456', '<AES-256-GCM加密后的值>', 'cmdb_external_source', 'cmdb-src-xxx', '{"key": "X-API-Key", "description": "API认证密钥"}');
```

- `resource_type`: 固定为 `cmdb_external_source`
- `resource_id`: 对应的数据源ID
- `metadata.key`: Header的key名称
- `value_hash`: AES-256-GCM加密后的Header值

## 3. API设计

### 3.1 数据源管理API

#### 创建数据源
```
POST /api/v1/cmdb/external-sources
```

请求体：
```json
{
  "name": "AWS CMDB - Production",
  "description": "从AWS CMDB同步生产环境资源",
  "api_endpoint": "https://cmdb.example.com/api/v1/resources",
  "http_method": "GET",
  "auth_headers": [
    {
      "key": "X-API-Key",
      "value": "your-api-key-here"    // 创建时传入明文，后端加密存储
    },
    {
      "key": "X-Tenant-ID",
      "value": "tenant-123"
    }
  ],
  "response_path": "$.data.items",
  "field_mapping": {
    "resource_type": "$.resourceType",
    "resource_name": "$.name",
    "cloud_resource_id": "$.cloudId",
    "cloud_resource_name": "$.displayName",
    "cloud_resource_arn": "$.arn",
    "description": "$.description",
    "tags": "$.tags"
  },
  "primary_key_field": "$.cloudId",           // 主键字段路径（必填）
  "cloud_provider": "aws",                     // 云提供商（用户输入）
  "account_id": "123456789012",                // 云账户ID（用户输入）
  "account_name": "Production Account",        // 云账户名称（可选）
  "region": "us-east-1",                       // 区域（可选）
  "sync_interval_minutes": 60,
  "resource_type_filter": "aws_security_group" // 可选：只同步特定资源类型
}
```

响应：
```json
{
  "source_id": "cmdb-src-abc123",
  "name": "AWS CMDB - Production",
  "description": "从AWS CMDB同步生产环境资源",
  "api_endpoint": "https://cmdb.example.com/api/v1/resources",
  "http_method": "GET",
  "auth_headers": [
    {
      "key": "X-API-Key",
      "has_value": true              // 只返回是否有值，不返回实际值
    },
    {
      "key": "X-Tenant-ID",
      "has_value": true
    }
  ],
  "response_path": "$.data.items",
  "field_mapping": {
    "resource_type": "$.resourceType",
    "resource_name": "$.name",
    "cloud_resource_id": "$.cloudId",
    "cloud_resource_name": "$.displayName",
    "cloud_resource_arn": "$.arn",
    "description": "$.description",
    "tags": "$.tags"
  },
  "primary_key_field": "$.cloudId",
  "cloud_provider": "aws",
  "account_id": "123456789012",
  "account_name": "Production Account",
  "region": "us-east-1",
  "sync_interval_minutes": 60,
  "resource_type_filter": "aws_security_group",
  "is_enabled": true,
  "created_at": "2026-01-14T16:00:00Z",
  "created_by": "user-xxx"
}
```

#### 获取数据源列表
```
GET /api/v1/cmdb/external-sources
```

响应：
```json
{
  "sources": [
    {
      "source_id": "cmdb-src-abc123",
      "name": "AWS CMDB",
      "api_endpoint": "https://cmdb.example.com/api/v1/resources",
      "auth_headers": [
        {
          "key": "X-API-Key",
          "has_value": true           // 不返回secret_id和实际值
        }
      ],
      "is_enabled": true,
      "last_sync_at": "2026-01-14T15:00:00Z",
      "last_sync_status": "success",
      "last_sync_count": 150
    }
  ]
}
```

#### 更新数据源
```
PUT /api/v1/cmdb/external-sources/:source_id
```

请求体：
```json
{
  "name": "AWS CMDB (Updated)",
  "auth_headers": [
    {
      "key": "X-API-Key",
      "value": "new-api-key"          // 如果提供value，则更新密钥
    },
    {
      "key": "X-Custom-Header",       // 新增Header
      "value": "custom-value"
    }
  ]
}
```

**重要**：
- 如果`auth_headers`中某个Header只提供`key`不提供`value`，则保留原有的密钥值
- 如果提供了`value`（即使是空字符串），则更新密钥值
- 如果要删除某个Header，在更新时不包含该Header即可

#### 删除数据源
```
DELETE /api/v1/cmdb/external-sources/:source_id
```

#### 手动触发同步
```
POST /api/v1/cmdb/external-sources/:source_id/sync
```

#### 测试连接
```
POST /api/v1/cmdb/external-sources/:source_id/test
```

响应：
```json
{
  "success": true,
  "message": "Connection successful",
  "sample_count": 10,
  "sample_data": [...]               // 返回少量样本数据用于验证映射
}
```

### 3.2 同步日志API

```
GET /api/v1/cmdb/external-sources/:source_id/sync-logs
```

响应：
```json
{
  "logs": [
    {
      "id": 1,
      "started_at": "2026-01-14T15:00:00Z",
      "completed_at": "2026-01-14T15:00:30Z",
      "status": "success",
      "resources_synced": 150,
      "resources_added": 10,
      "resources_updated": 5,
      "resources_deleted": 2,
      "error_message": null
    }
  ]
}
```

## 4. 数据模型扩展

### 4.1 ResourceIndex表扩展

在现有的`resource_index`表中添加字段以支持外部数据源：

```sql
-- 数据来源字段
ALTER TABLE resource_index ADD COLUMN IF NOT EXISTS source_type VARCHAR(20) DEFAULT 'terraform';
ALTER TABLE resource_index ADD COLUMN IF NOT EXISTS external_source_id VARCHAR(50);

-- 云环境字段（新增）
ALTER TABLE resource_index ADD COLUMN IF NOT EXISTS cloud_provider VARCHAR(50);      -- 云提供商: aws/azure/gcp/aliyun
ALTER TABLE resource_index ADD COLUMN IF NOT EXISTS cloud_account_id VARCHAR(100);   -- 云账户ID
ALTER TABLE resource_index ADD COLUMN IF NOT EXISTS cloud_account_name VARCHAR(200); -- 云账户名称
ALTER TABLE resource_index ADD COLUMN IF NOT EXISTS cloud_region VARCHAR(50);        -- 区域

-- 主键字段（新增）
ALTER TABLE resource_index ADD COLUMN IF NOT EXISTS primary_key_value VARCHAR(500);  -- 主键值（根据primary_key_field提取）

-- source_type: 'terraform' (默认，从Terraform state同步) 或 'external' (从外部CMDB同步)
-- external_source_id: 外部数据源ID，仅当source_type='external'时有值

CREATE INDEX IF NOT EXISTS idx_resource_index_source_type ON resource_index(source_type);
CREATE INDEX IF NOT EXISTS idx_resource_index_external_source ON resource_index(external_source_id);
CREATE INDEX IF NOT EXISTS idx_resource_index_cloud_provider ON resource_index(cloud_provider);
CREATE INDEX IF NOT EXISTS idx_resource_index_cloud_account ON resource_index(cloud_account_id);
CREATE INDEX IF NOT EXISTS idx_resource_index_primary_key ON resource_index(primary_key_value);
```

### 4.3 主键字段说明

**主键字段（primary_key_field）** 用于指定外部CMDB数据的主要标识字段：

| 资源类型 | 推荐主键字段 | 示例值 |
|---------|-------------|--------|
| `aws_security_group` | `$.id` | `sg-12345678` |
| `aws_iam_role` | `$.name` | `my-role-name` |
| `aws_iam_policy` | `$.arn` | `arn:aws:iam::123456789:policy/MyPolicy` |
| `aws_s3_bucket` | `$.name` | `my-bucket-name` |
| `aws_ec2_instance` | `$.id` | `i-12345678` |
| `aws_vpc` | `$.id` | `vpc-12345678` |
| `aws_subnet` | `$.id` | `subnet-12345678` |
| `aws_rds_instance` | `$.id` | `my-database` |
| `aws_lambda_function` | `$.name` | `my-function` |
| `aws_eks_cluster` | `$.name` | `my-cluster` |

**主键值的用途**：
1. **唯一标识** - 用于增量同步时判断资源是否已存在
2. **搜索优化** - 可以直接通过主键值搜索资源
3. **去重** - 同一数据源内，主键值相同的资源会被更新而不是重复创建

### 4.2 同步日志表

```sql
CREATE TABLE IF NOT EXISTS cmdb_sync_logs (
    id SERIAL PRIMARY KEY,
    source_id VARCHAR(50) NOT NULL,
    started_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP,
    status VARCHAR(20) NOT NULL DEFAULT 'running',    -- running/success/failed
    resources_synced INT DEFAULT 0,
    resources_added INT DEFAULT 0,
    resources_updated INT DEFAULT 0,
    resources_deleted INT DEFAULT 0,
    error_message TEXT,
    
    FOREIGN KEY (source_id) REFERENCES cmdb_external_sources(source_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_cmdb_sync_logs_source ON cmdb_sync_logs(source_id);
CREATE INDEX IF NOT EXISTS idx_cmdb_sync_logs_started ON cmdb_sync_logs(started_at);
```

## 5. 安全设计

### 5.1 Header值的安全处理

1. **存储加密**：Header值使用AES-256-GCM加密后存储在secrets表
2. **传输安全**：API响应中永远不返回Header的实际值
3. **编辑保护**：编辑时只能设置新值，无法查看旧值
4. **审计日志**：记录所有对敏感配置的修改操作

### 5.2 前端显示规则

```typescript
// Header配置的显示
interface AuthHeaderDisplay {
  key: string;           // 可编辑
  hasValue: boolean;     // 是否已设置值
  // value 永远不返回给前端
}

// 编辑时的输入
interface AuthHeaderInput {
  key: string;
  value?: string;        // 可选，如果提供则更新
}
```

### 5.3 前端UI设计

```
┌─────────────────────────────────────────────────────────────────┐
│ 认证Headers                                                      │
├─────────────────────────────────────────────────────────────────┤
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ Key: [X-API-Key        ]  Value: [••••••••] [已设置] [更新] │ │
│ │ Key: [Authorization    ]  Value: [••••••••] [已设置] [更新] │ │
│ │                                                    [+ 添加] │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                                                                 │
│ 💡 说明：Header值已加密存储，无法查看。如需修改请点击"更新"按钮。  │
└─────────────────────────────────────────────────────────────────┘
```

## 6. 同步流程

### 6.1 同步流程图

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  触发同步   │────▶│  获取配置   │────▶│  解密Header │────▶│  调用API    │
└─────────────┘     └─────────────┘     └─────────────┘     └─────────────┘
                                                                   │
                                                                   ▼
┌─────────────┐     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  更新状态   │◀────│  写入数据库 │◀────│  数据映射   │◀────│  解析响应   │
└─────────────┘     └─────────────┘     └─────────────┘     └─────────────┘
```

### 6.2 同步策略

1. **增量同步**：基于资源ID进行增量更新
2. **全量同步**：可选择全量替换模式
3. **冲突处理**：外部数据源的数据不会覆盖Terraform管理的资源

### 6.4 搜索兼容性设计

#### 6.4.1 搜索结果排序规则

现有的CMDB搜索需要兼容外部数据源的数据，搜索结果按以下规则排序：

1. **内部数据（Terraform）优先** - `source_type = 'terraform'` 的数据排在前面
2. **外部数据靠后** - `source_type = 'external'` 的数据排在后面
3. **同类型内按匹配度排序** - 精确匹配 > 前缀匹配 > 包含匹配

#### 6.4.2 搜索结果区分

搜索结果需要明确标识数据来源：

```json
{
  "results": [
    {
      "workspace_id": "ws-abc123",
      "terraform_address": "module.vpc.aws_security_group.main",
      "resource_type": "aws_security_group",
      "cloud_resource_id": "sg-12345678",
      "source_type": "terraform",           // 数据来源
      "external_source_name": null,         // 外部数据源名称（仅外部数据有值）
      "jump_url": "/workspaces/ws-abc123/resources/1",  // 内部数据有跳转链接
      "match_rank": 0.9
    },
    {
      "workspace_id": null,                 // 外部数据可能没有workspace
      "terraform_address": null,            // 外部数据没有terraform地址
      "resource_type": "aws_security_group",
      "cloud_resource_id": "sg-87654321",
      "source_type": "external",            // 外部数据
      "external_source_name": "AWS CMDB",   // 外部数据源名称
      "jump_url": null,                     // 外部数据不支持跳转
      "match_rank": 0.8
    }
  ]
}
```

#### 6.4.3 修改后的搜索SQL

```sql
-- 搜索资源（兼容外部数据源）
SELECT 
    ri.workspace_id,
    w.name as workspace_name,
    ri.terraform_address,
    ri.resource_type,
    ri.resource_name,
    ri.cloud_resource_id,
    ri.cloud_resource_name,
    ri.description,
    ri.source_type,
    es.name as external_source_name,
    CASE 
        WHEN ri.source_type = 'terraform' AND wr.id IS NOT NULL 
        THEN CONCAT('/workspaces/', ri.workspace_id, '/resources/', wr.id)
        ELSE NULL  -- 外部数据不支持跳转
    END as jump_url,
    CASE 
        -- 内部数据基础分数更高
        WHEN ri.source_type = 'terraform' THEN 1.0
        ELSE 0.5
    END * CASE 
        WHEN ri.cloud_resource_id = ? THEN 1.0
        WHEN ri.cloud_resource_name = ? THEN 0.9
        WHEN ri.cloud_resource_id LIKE ? THEN 0.8
        WHEN ri.cloud_resource_name LIKE ? THEN 0.7
        ELSE 0.5
    END as match_rank
FROM resource_index ri
LEFT JOIN workspaces w ON ri.workspace_id = w.workspace_id
LEFT JOIN workspace_resources wr ON ri.workspace_id = wr.workspace_id 
    AND ri.source_type = 'terraform'  -- 只有内部数据才关联平台资源
    AND wr.is_active = true
LEFT JOIN cmdb_external_sources es ON ri.external_source_id = es.source_id
WHERE ri.resource_mode = 'managed'
    AND (
        ri.cloud_resource_id ILIKE ? OR
        ri.cloud_resource_name ILIKE ? OR
        ri.description ILIKE ?
    )
ORDER BY 
    ri.source_type ASC,  -- terraform 排在 external 前面（字母顺序）
    match_rank DESC,
    ri.cloud_resource_name
LIMIT ?;
```

#### 6.4.4 前端搜索结果展示

```
┌─────────────────────────────────────────────────────────────────┐
│ 搜索结果 (共 15 条)                                              │
├─────────────────────────────────────────────────────────────────┤
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ [内部] aws_security_group                    @ workspace-prod │ │
│ │ web-server-sg (sg-12345678)                                 │ │
│ │ module.vpc.aws_security_group.main           [查看详情 →]   │ │
│ └─────────────────────────────────────────────────────────────┘ │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ [内部] aws_security_group                    @ workspace-dev │ │
│ │ database-sg (sg-23456789)                                   │ │
│ │ module.rds.aws_security_group.db             [查看详情 →]   │ │
│ └─────────────────────────────────────────────────────────────┘ │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ [外部] aws_security_group                    来源: AWS CMDB │ │
│ │ legacy-app-sg (sg-87654321)                                 │ │
│ │ Legacy application security group                           │ │
│ └─────────────────────────────────────────────────────────────┘ │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ [外部] aws_security_group                    来源: AWS CMDB │ │
│ │ monitoring-sg (sg-98765432)                                 │ │
│ │ Monitoring tools security group                             │ │
│ └─────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘

图例：
- [内部] = Terraform管理的资源，可点击跳转到资源详情
- [外部] = 外部CMDB同步的资源，仅供参考，不支持跳转
```

#### 6.4.5 搜索建议兼容

搜索建议也需要兼容外部数据，但同样遵循内部优先原则：

```go
// GetSearchSuggestions 获取搜索建议（兼容外部数据）
func (s *CMDBService) GetSearchSuggestions(prefix string, limit int) ([]SearchSuggestion, error) {
    // 1. 先查询内部数据的建议
    internalSuggestions := s.getInternalSuggestions(prefix, limit)
    
    // 2. 如果内部数据不足，补充外部数据
    remaining := limit - len(internalSuggestions)
    if remaining > 0 {
        externalSuggestions := s.getExternalSuggestions(prefix, remaining)
        internalSuggestions = append(internalSuggestions, externalSuggestions...)
    }
    
    return internalSuggestions, nil
}
```

### 6.3 定时同步

使用后台任务定期执行同步：

```go
// 定时任务配置
type SyncScheduler struct {
    db *gorm.DB
    cmdbService *CMDBService
}

func (s *SyncScheduler) Start() {
    ticker := time.NewTicker(1 * time.Minute)
    go func() {
        for range ticker.C {
            s.checkAndSync()
        }
    }()
}

func (s *SyncScheduler) checkAndSync() {
    // 查找需要同步的数据源
    var sources []models.CMDBExternalSource
    s.db.Where("is_enabled = ? AND sync_interval_minutes > 0", true).
        Where("last_sync_at IS NULL OR last_sync_at < NOW() - INTERVAL '1 minute' * sync_interval_minutes").
        Find(&sources)
    
    for _, source := range sources {
        go s.cmdbService.SyncExternalSource(source.SourceID)
    }
}
```

## 7. 实现步骤

### 7.1 后端实现

1. **数据库迁移**
   - 创建 `cmdb_external_sources` 表
   - 创建 `cmdb_sync_logs` 表
   - 扩展 `resource_index` 表

2. **模型定义**
   - `models/cmdb_external_source.go`
   - `models/cmdb_sync_log.go`

3. **服务层**
   - 扩展 `cmdb_service.go` 添加外部数据源同步方法
   - 创建 `cmdb_external_source_service.go` 管理数据源配置

4. **API层**
   - 扩展 `cmdb_handler.go` 添加数据源管理API
   - 添加路由配置

### 7.2 前端实现

1. **服务层**
   - 扩展 `cmdb.ts` 添加数据源管理API调用

2. **页面组件**
   - 在CMDB页面添加"外部数据源"Tab
   - 创建数据源配置表单组件
   - 创建Header编辑组件（支持密钥隐藏）

## 8. 问题讨论

### 8.1 待确认问题

1. **多租户支持**：是否需要按组织隔离外部数据源？
2. **权限控制**：谁可以创建/编辑外部数据源？
3. **同步范围**：外部数据是否需要关联到特定Workspace？
4. **数据冲突**：如果外部CMDB和Terraform管理的资源ID冲突如何处理？

### 8.2 建议

1. **初期简化**：先实现基本的同步功能，不考虑多租户
2. **权限**：仅Admin可以管理外部数据源
3. **隔离**：外部数据源的资源使用`source_type='external'`标识，与Terraform资源区分
4. **冲突**：外部资源使用独立的命名空间，不与Terraform资源冲突

请确认以上方案是否符合您的需求，或者有任何需要调整的地方。

## 9. 对现有逻辑的影响分析

### 9.1 影响范围总结

| 组件 | 影响程度 | 说明 |
|------|----------|------|
| `resource_index` 表 |  低影响 | 新增2个可选字段，不影响现有数据 |
| `secrets` 表 | ✅ 无影响 | 复用现有表，新增resource_type枚举值 |
| CMDB Service |  低影响 | 新增方法，不修改现有方法 |
| CMDB Handler |  低影响 | 新增API端点，不修改现有端点 |
| 前端CMDB页面 |  低影响 | 新增Tab，不修改现有功能 |

### 9.2 详细分析

#### 9.2.1 resource_index 表

**现有使用情况**：
- 仅在 `backend/services/cmdb_service.go` 中使用
- 主要用于：搜索资源、获取资源树、同步Terraform state

**新增字段**：
```sql
ALTER TABLE resource_index ADD COLUMN IF NOT EXISTS source_type VARCHAR(20) DEFAULT 'terraform';
ALTER TABLE resource_index ADD COLUMN IF NOT EXISTS external_source_id VARCHAR(50);
```

**影响分析**：
- ✅ 使用 `DEFAULT 'terraform'`，现有数据自动填充默认值
- ✅ 新字段为可选字段（允许NULL）
- ✅ 现有查询不需要修改，因为默认值确保兼容性
- ✅ 现有的同步逻辑（`SyncWorkspaceResources`）不需要修改

#### 9.2.2 secrets 表

**现有使用情况**：
- 用于存储Agent Pool的HCP凭证
- `resource_type` 枚举值：`agent_pool`, `workspace`, `module`, `system`, `team`, `user`
- `secret_type` 枚举值：`hcp`

**新增内容**：
```go
// 新增 resource_type 枚举值
ResourceTypeCMDBExternalSource ResourceType = "cmdb_external_source"

// 新增 secret_type 枚举值
SecretTypeAPIHeader SecretType = "api_header"
```

**影响分析**：
- ✅ 仅新增枚举值，不修改现有枚举
- ✅ 现有的secrets查询使用 `resource_type` 过滤，不会查到新类型的数据
- ✅ 现有的HCP凭证逻辑完全不受影响
- ✅ 复用现有的加密/解密机制

#### 9.2.3 CMDB Service

**现有方法**（不修改）：
- `SyncWorkspaceResources()` - 从Terraform state同步
- `SearchResources()` - 搜索资源
- `GetWorkspaceResourceTree()` - 获取资源树
- `GetResourceDetail()` - 获取资源详情
- `GetCMDBStats()` - 获取统计信息
- `SyncAllWorkspaces()` - 同步所有workspace
- `GetSearchSuggestions()` - 获取搜索建议

**新增方法**：
- `CreateExternalSource()` - 创建外部数据源
- `UpdateExternalSource()` - 更新外部数据源
- `DeleteExternalSource()` - 删除外部数据源
- `ListExternalSources()` - 列出外部数据源
- `SyncExternalSource()` - 同步外部数据源
- `TestExternalSourceConnection()` - 测试连接

**影响分析**：
- ✅ 所有新增方法独立于现有方法
- ✅ 不修改任何现有方法的签名或逻辑

#### 9.2.4 CMDB Handler

**现有API端点**（不修改）：
- `GET /api/v1/cmdb/search` - 搜索资源
- `GET /api/v1/cmdb/workspaces/:workspace_id/tree` - 获取资源树
- `GET /api/v1/cmdb/workspaces/:workspace_id/resources` - 获取资源详情
- `GET /api/v1/cmdb/stats` - 获取统计信息
- `POST /api/v1/cmdb/workspaces/:workspace_id/sync` - 同步workspace
- `POST /api/v1/cmdb/sync-all` - 同步所有
- `GET /api/v1/cmdb/resource-types` - 获取资源类型
- `GET /api/v1/cmdb/workspace-counts` - 获取workspace资源数量
- `GET /api/v1/cmdb/suggestions` - 获取搜索建议

**新增API端点**：
- `POST /api/v1/cmdb/external-sources` - 创建外部数据源
- `GET /api/v1/cmdb/external-sources` - 列出外部数据源
- `GET /api/v1/cmdb/external-sources/:source_id` - 获取外部数据源详情
- `PUT /api/v1/cmdb/external-sources/:source_id` - 更新外部数据源
- `DELETE /api/v1/cmdb/external-sources/:source_id` - 删除外部数据源
- `POST /api/v1/cmdb/external-sources/:source_id/sync` - 同步外部数据源
- `POST /api/v1/cmdb/external-sources/:source_id/test` - 测试连接
- `GET /api/v1/cmdb/external-sources/:source_id/sync-logs` - 获取同步日志

**影响分析**：
- ✅ 所有新增端点使用独立的URL路径
- ✅ 不修改任何现有端点

#### 9.2.5 前端CMDB页面

**现有功能**（不修改）：
- 资源树Tab（Resource Tree）
- 搜索Tab（Search）
- 统计卡片
- Workspace资源树展开/折叠

**新增功能**：
- 外部数据源Tab（External Sources）
- 数据源配置表单
- Header编辑组件

**影响分析**：
- ✅ 新增独立的Tab，不修改现有Tab
- ✅ 现有的组件和样式不受影响

### 9.3 数据库迁移安全性

```sql
-- 所有迁移都使用 IF NOT EXISTS / IF EXISTS，确保幂等性

-- 1. 新增表（安全）
CREATE TABLE IF NOT EXISTS cmdb_external_sources (...);
CREATE TABLE IF NOT EXISTS cmdb_sync_logs (...);

-- 2. 扩展现有表（安全）
ALTER TABLE resource_index ADD COLUMN IF NOT EXISTS source_type VARCHAR(20) DEFAULT 'terraform';
ALTER TABLE resource_index ADD COLUMN IF NOT EXISTS external_source_id VARCHAR(50);

-- 3. 新增索引（安全）
CREATE INDEX IF NOT EXISTS idx_resource_index_source_type ON resource_index(source_type);
CREATE INDEX IF NOT EXISTS idx_resource_index_external_source ON resource_index(external_source_id);
```

### 9.4 回滚方案

如果需要回滚，可以执行以下操作：

```sql
-- 1. 删除新增的表
DROP TABLE IF EXISTS cmdb_sync_logs;
DROP TABLE IF EXISTS cmdb_external_sources;

-- 2. 删除新增的字段（可选，保留也不影响）
ALTER TABLE resource_index DROP COLUMN IF EXISTS source_type;
ALTER TABLE resource_index DROP COLUMN IF EXISTS external_source_id;

-- 3. 删除新增的secrets记录
DELETE FROM secrets WHERE resource_type = 'cmdb_external_source';
```

### 9.5 结论

**推荐方案对现有逻辑的影响极小**：

1. ✅ **不修改任何现有代码逻辑** - 所有改动都是新增
2. ✅ **不修改任何现有API** - 所有新API使用独立路径
3. ✅ **不影响现有数据** - 新字段使用默认值，现有数据自动兼容
4. ✅ **可安全回滚** - 所有新增内容可独立删除
5. ✅ **复用现有机制** - 复用secrets表的加密机制，无需新建加密逻辑
