# Workspace模块 - API规范

> **文档版本**: v1.0  
> **创建日期**: 2025-10-09  
> **状态**: 完整设计

## 📘 概述

本文档定义Workspace模块的所有REST API接口规范，包括请求/响应格式、错误码和认证方式。

## 🔐 认证

**方式**: Bearer Token

```http
Authorization: Bearer <token>
```

## 🆔 资源ID格式规范

所有资源ID采用统一的字符串格式：`{type}-{20位随机字符}`

示例：
- Workspace: `ws-cx34lzcp313z23u0z1mc`
- Task/Run: `run-dx45maaq424a34v1a2nd`
- State Version: `sv-fx67occs646c56x3c4pf`
- Agent: `agent-hx89qeeu868e78z5e6rh`
- Agent Pool: `apool-gx78pddt757d67y4d5qg`

详细规范请参考：[资源ID规范文档](../id-specification.md)

## 📊 通用响应格式

### 成功响应

```json
{
  "success": true,
  "data": {},
  "message": "Operation successful"
}
```

### 错误响应

```json
{
  "success": false,
  "error": {
    "code": "WORKSPACE_NOT_FOUND",
    "message": "Workspace not found",
    "details": {}
  }
}
```

## 🔗 API端点

### Workspace管理

#### 1. 创建Workspace

```http
POST /api/v1/workspaces
```

**请求体**:
```json
{
  "name": "production-infra",
  "description": "Production infrastructure",
  "execution_mode": "local",
  "terraform_version": "1.6.0",
  "auto_apply": false,
  "tags": ["production", "aws"]
}
```

**响应**: `201 Created`
```json
{
  "success": true,
  "data": {
    "id": 1,
    "name": "production-infra",
    "state": "created",
    "created_at": "2025-10-09T10:00:00Z"
  }
}
```

#### 2. 获取Workspace列表

```http
GET /api/v1/workspaces?page=1&limit=20&state=created
```

#### 3. 获取Workspace详情

```http
GET /api/v1/workspaces/:id
```

#### 4. 更新Workspace

```http
PUT /api/v1/workspaces/:id
```

#### 5. 删除Workspace

```http
DELETE /api/v1/workspaces/:id
```

#### 6. 锁定/解锁Workspace

```http
POST /api/v1/workspaces/:id/lock
POST /api/v1/workspaces/:id/unlock
```

### 任务管理

#### 1. 创建Plan任务

```http
POST /api/v1/workspaces/:id/tasks/plan
```

**请求体**:
```json
{
  "message": "Update security group rules",
  "variables": {
    "environment": "production"
  }
}
```

#### 2. 创建Apply任务

```http
POST /api/v1/workspaces/:id/tasks/apply
```

#### 3. 获取任务列表

```http
GET /api/v1/workspaces/:id/tasks?status=success&limit=50
```

#### 4. 获取任务详情

```http
GET /api/v1/workspaces/:id/tasks/:task_id
```

#### 5. 取消任务

```http
POST /api/v1/workspaces/:id/tasks/:task_id/cancel
```

### State版本管理

#### 1. 获取当前State

```http
GET /api/v1/workspaces/:id/current-state
```

#### 2. 获取State版本列表

```http
GET /api/v1/workspaces/:id/state-versions
```

#### 3. 获取指定版本

```http
GET /api/v1/workspaces/:id/state-versions/:version_id
```

#### 4. 回滚到指定版本

```http
POST /api/v1/workspaces/:id/state-versions/:version_id/rollback
```

#### 5. 对比版本

```http
GET /api/v1/workspaces/:id/state-versions/compare?from=1&to=2
```

### Agent管理

#### 1. 创建Agent

```http
POST /api/v1/agents
```

**请求体**:
```json
{
  "name": "agent-01",
  "agent_type": "remote",
  "labels": ["production", "us-west"],
  "endpoint": "https://agent-01.example.com"
}
```

#### 2. 获取Agent列表

```http
GET /api/v1/agents?status=online&labels=production
```

#### 3. Agent心跳

```http
POST /api/v1/agents/:id/heartbeat
```

#### 4. 重新生成Token

```http
POST /api/v1/agents/:id/regenerate-token
```

### Agent Pool管理

#### 1. 创建Pool

```http
POST /api/v1/agent-pools
```

**请求体**:
```json
{
  "name": "production-pool",
  "pool_type": "static",
  "selection_strategy": "least_busy",
  "required_labels": ["production"]
}
```

#### 2. 添加Agent到Pool

```http
POST /api/v1/agent-pools/:id/agents
```

**请求体**:
```json
{
  "agent_id": "agent-01"
}
```

### K8s配置管理

#### 1. 创建K8s配置

```http
POST /api/v1/k8s-configs
```

**请求体**:
```json
{
  "name": "prod-k8s",
  "namespace": "terraform",
  "pod_template": {
    "image": "hashicorp/terraform:1.6.0",
    "resources": {
      "requests": {"cpu": "500m", "memory": "512Mi"}
    }
  }
}
```

#### 2. 测试K8s连接

```http
POST /api/v1/k8s-configs/:id/test
```

#### 3. 设置为默认配置

```http
POST /api/v1/k8s-configs/:id/set-default
```

## 📝 错误码

| 错误码 | HTTP状态码 | 说明 |
|--------|-----------|------|
| WORKSPACE_NOT_FOUND | 404 | Workspace不存在 |
| WORKSPACE_LOCKED | 423 | Workspace已锁定 |
| INVALID_STATE_TRANSITION | 400 | 无效的状态转换 |
| TASK_NOT_FOUND | 404 | 任务不存在 |
| AGENT_NOT_FOUND | 404 | Agent不存在 |
| AGENT_OFFLINE | 503 | Agent离线 |
| UNAUTHORIZED | 401 | 未授权 |
| FORBIDDEN | 403 | 无权限 |
| INTERNAL_ERROR | 500 | 内部错误 |

## 🔄 分页

**请求参数**:
- `page`: 页码（从1开始）
- `limit`: 每页数量（默认20，最大100）

**响应**:
```json
{
  "data": [],
  "pagination": {
    "page": 1,
    "limit": 20,
    "total": 100,
    "total_pages": 5
  }
}
```

## 🔍 过滤和排序

**过滤**: 使用查询参数
```http
GET /api/v1/workspaces?state=completed&tags=production
```

**排序**: 使用`sort`参数
```http
GET /api/v1/workspaces?sort=-created_at
```
- `-created_at`: 降序
- `created_at`: 升序

## 📊 批量操作

**批量删除**:
```http
POST /api/v1/workspaces/batch-delete
```

**请求体**:
```json
{
  "ids": [1, 2, 3]
}
```

---

**相关文档**:
- [00-overview.md](./00-overview.md) - 总览和架构
- [08-database-design.md](./08-database-design.md) - 数据库设计
- [10-implementation-guide.md](./10-implementation-guide.md) - 实现指导
