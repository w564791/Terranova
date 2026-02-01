# CMDB资源索引功能实施总结

## 功能概述

CMDB（Configuration Management Database）资源索引功能为IaC平台提供了全局资源搜索和树状结构浏览能力，帮助用户快速定位资源所属的workspace。

## 已实现功能

### 1. 后端实现

#### 数据库表
- `resource_index` - 资源索引表，存储从Terraform state解析的资源信息
- `module_hierarchy` - Module层级表，存储module的树状结构

**创建表脚本**: `scripts/create_resource_index_tables.sql`

#### 模型定义
- `backend/internal/models/resource_index.go`
  - `ResourceIndex` - 资源索引模型
  - `ModuleHierarchy` - Module层级模型
  - `ResourceSearchResult` - 搜索结果模型
  - `ResourceTreeNode` - 资源树节点模型
  - `WorkspaceResourceTree` - Workspace资源树响应模型
  - `CMDBStats` - CMDB统计信息模型

#### 服务层
- `backend/services/cmdb_service.go`
  - `SyncWorkspaceResources()` - 同步单个workspace的资源索引
  - `SyncAllWorkspaces()` - 同步所有workspace的资源索引
  - `SearchResources()` - 搜索资源（支持ID、名称、描述）
  - `GetWorkspaceResourceTree()` - 获取workspace的资源树
  - `GetResourceDetail()` - 获取资源详情
  - `GetCMDBStats()` - 获取CMDB统计信息
  - `ResourceNameExtractor` - 智能资源名称提取器

#### API Handler
- `backend/internal/handlers/cmdb_handler.go`
  - `GET /api/v1/cmdb/search` - 搜索资源
  - `GET /api/v1/cmdb/stats` - 获取统计信息
  - `GET /api/v1/cmdb/resource-types` - 获取资源类型列表
  - `GET /api/v1/cmdb/workspaces/:workspace_id/tree` - 获取资源树
  - `GET /api/v1/cmdb/workspaces/:workspace_id/resources` - 获取资源详情
  - `POST /api/v1/cmdb/workspaces/:workspace_id/sync` - 同步单个workspace
  - `POST /api/v1/cmdb/sync-all` - 同步所有workspace

#### 路由配置
- `backend/internal/router/router_cmdb.go` - CMDB路由定义
- `backend/internal/router/router.go` - 主路由注册

### 2. 前端实现

#### API服务
- `frontend/src/services/cmdb.ts`
  - 类型定义（CMDBStats, ResourceSearchResult, ResourceTreeNode等）
  - API调用方法

#### 页面组件
- `frontend/src/pages/CMDB.tsx` - CMDB主页面
  - 统计卡片展示
  - 资源搜索功能（支持关键词和资源类型过滤）
  - 资源树浏览功能
  - 搜索结果展示（带跳转链接）

#### 样式
- `frontend/src/pages/CMDB.module.css` - CMDB页面样式

#### 路由和导航
- `frontend/src/App.tsx` - 添加CMDB路由
- `frontend/src/components/Layout.tsx` - 添加CMDB导航入口

### 3. 权限配置
- `scripts/add_cmdb_permission.sql` - CMDB权限定义（只读）

## 资源名称提取策略

由于不同AWS资源的名称字段不统一，实现了智能提取逻辑：

**提取优先级：**
1. `name` 字段（直接属性）
2. `tags.Name` 或 `tags_all.Name`（标签）
3. `description` 字段（描述信息）
4. 资源类型特定的fallback字段
5. 最终使用资源ID作为fallback

**支持的资源类型fallback规则：**
- EC2: `private_dns`, `private_ip`
- VPC: `cidr_block`
- Subnet: `cidr_block`, `availability_zone`
- Security Group: `name_prefix`
- RDS: `db_instance_identifier`, `endpoint`
- S3: `bucket`
- IAM: `name_prefix`
- EKS: `endpoint`, `node_group_name`
- 等等...

## 树状结构设计

```
workspace
├── module.ec2-ff (module名)
│   ├── aws_instance.abc (资源)
│   │   └── cloud_id: i-1234567890
│   ├── module.sg-module-333 (嵌套module)
│   │   └── aws_security_group.main
│   │       └── cloud_id: sg-1234
│   └── module.sg-module-444
│       └── aws_security_group.main
│           └── cloud_id: sg-5566
└── aws_vpc.main (根级资源)
    └── cloud_id: vpc-abcd
```

## 使用说明

### 1. 执行数据库迁移

```bash
# 创建资源索引表
psql -U postgres -d iac_platform -f scripts/create_resource_index_tables.sql

# 添加CMDB权限定义
psql -U postgres -d iac_platform -f scripts/add_cmdb_permission.sql
```

### 2. 同步资源索引

首次使用需要同步资源索引：
- 通过UI：点击CMDB页面右上角的"同步全部"按钮
- 通过API：`POST /api/v1/cmdb/sync-all`

### 3. 搜索资源

在CMDB页面的搜索框中输入：
- 资源ID（如 `sg-0123456789`）
- 资源名称（如 `web-server-sg`）
- 描述信息

### 4. 浏览资源树

切换到"资源树"标签页，选择一个workspace查看其资源的树状结构。

## 自动同步触发

CMDB同步支持以下触发方式：

### 1. 自动同步（Terraform任务完成后）

**支持所有三种执行模式：**

| 模式 | 触发位置 | 说明 |
|------|----------|------|
| **Local** | `terraform_executor.go` | 在 `SaveNewStateVersion` 成功后异步触发 |
| **Agent** | `agent_handler.go` | 在 `UpdateTaskStatus` 收到 `applied` 状态时异步触发 |
| **K8s Agent** | `agent_handler.go` | 同Agent模式，通过相同的API端点处理 |

**实现代码：**

Local模式（`terraform_executor.go`）:
```go
if saveErr == nil {
    // 异步触发CMDB资源索引同步
    if s.db != nil {
        go func(wsID string) {
            cmdbService := NewCMDBService(s.db)
            cmdbService.SyncWorkspaceResources(wsID)
        }(workspace.WorkspaceID)
    }
}
```

Agent/K8s Agent模式（`agent_handler.go`）:
```go
if req.Status == models.TaskStatusApplied {
    // 异步触发 CMDB 资源索引同步
    go func(wsID string) {
        cmdbService := services.NewCMDBService(h.db)
        cmdbService.SyncWorkspaceResources(wsID)
    }(task.WorkspaceID)
}
```

### 2. 手动同步

- **CMDB页面**：点击workspace行的"Sync"按钮或页面顶部的"Sync All"按钮
- **NewRunDialog**：创建任务时勾选"Refresh CMDB"选项
- **API调用**：
  - `POST /api/v1/cmdb/workspaces/{workspace_id}/sync` - 同步单个workspace
  - `POST /api/v1/cmdb/sync-all` - 同步所有workspace（异步执行）

### 3. 权限要求

- **只读操作**（搜索、查看资源树）：所有认证用户
- **同步操作**：需要 `cmdb:ADMIN` 权限（通常只有admin角色）

## 后续优化建议

1. ~~**实时同步**: 在Terraform apply成功后自动触发同步~~ ✅ 已实现
2. **定时同步**: 添加定时任务定期全量同步
3. **搜索优化**: 添加全文搜索索引提升搜索性能
4. **缓存**: 添加资源树缓存减少数据库查询
5. **权限控制**: 根据用户workspace权限过滤搜索结果

## 文件清单

### 后端
- `backend/internal/models/resource_index.go`
- `backend/services/cmdb_service.go`
- `backend/internal/handlers/cmdb_handler.go`
- `backend/internal/router/router_cmdb.go`
- `backend/internal/router/router.go` (修改)

### 前端
- `frontend/src/services/cmdb.ts`
- `frontend/src/pages/CMDB.tsx`
- `frontend/src/pages/CMDB.module.css`
- `frontend/src/App.tsx` (修改)
- `frontend/src/components/Layout.tsx` (修改)

### 数据库脚本
- `scripts/create_resource_index_tables.sql`
- `scripts/add_cmdb_permission.sql`

### 文档
- `docs/cmdb/cmdb-tree-structure-design.md`
- `docs/cmdb/cmdb-implementation-summary.md`
