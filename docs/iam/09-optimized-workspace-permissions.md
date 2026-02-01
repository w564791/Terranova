# 优化后的工作空间权限体系

## 概述

本文档描述优化后的工作空间权限体系，将所有查看操作统一到`workspace_management` READ权限下，简化权限管理。

## 权限优化方案

### 核心思想

**workspace_management READ权限作为统一的只读权限**
- 拥有此权限的用户可以查看工作空间内的所有数据
- 包括：变量、State、资源、任务等
- 但不能进行任何修改操作

**专项权限用于写入和管理操作**
- workspace_variables: WRITE/ADMIN - 变量的创建、修改、删除
- workspace_state: WRITE/ADMIN - State的回滚、删除
- workspace_resources: WRITE/ADMIN - 资源的创建、修改、删除
- workspace_execution: WRITE/ADMIN - 任务的创建、取消、确认

## 权限映射表

### workspace_management（工作空间管理）

#### READ级别 - 统一只读权限
**变量查看（2个API）：**
- `GET /:id/variables` - 列出变量
- `GET /:id/variables/:var_id` - 查看变量详情

**State查看（5个API）：**
- `GET /:id/current-state` - 查看当前State
- `GET /:id/state-versions` - 列出State版本
- `GET /:id/state-versions/compare` - 对比State版本
- `GET /:id/state-versions/:version/metadata` - 查看State元数据
- `GET /:id/state-versions/:version` - 查看特定State版本

**资源查看（10个API）：**
- `GET /:id/resources` - 列出资源
- `GET /:id/resources/:resource_id` - 查看资源详情
- `GET /:id/resources/:resource_id/versions` - 查看资源版本
- `GET /:id/resources/:resource_id/versions/compare` - 对比资源版本
- `GET /:id/resources/:resource_id/versions/:version` - 查看特定资源版本
- `GET /:id/resources/:resource_id/dependencies` - 查看资源依赖
- `GET /:id/snapshots` - 列出快照
- `GET /:id/snapshots/:snapshot_id` - 查看快照详情
- `GET /:id/resources/:resource_id/editing/status` - 查看编辑状态
- `GET /:id/resources/:resource_id/drift` - 查看Drift

**工作空间概览：**
- `GET /:id/overview` - 查看工作空间概览（也接受其他工作空间级READ权限）

#### WRITE级别
- 修改工作空间配置
- 锁定/解锁工作空间

#### ADMIN级别
- 删除工作空间
- 完全管理工作空间

### workspace_variables（变量管理）

#### WRITE级别
- `POST /:id/variables` - 创建变量
- `PUT /:id/variables/:var_id` - 更新变量

#### ADMIN级别
- `DELETE /:id/variables/:var_id` - 删除变量

### workspace_state（状态管理）

#### WRITE级别
- `POST /:id/state-versions/:version/rollback` - 回滚State

#### ADMIN级别
- `DELETE /:id/state-versions/:version` - 删除State版本

### workspace_resources（资源管理）

#### WRITE级别
- `POST /:id/resources` - 创建资源
- `POST /:id/resources/import` - 导入资源
- `POST /:id/resources/deploy` - 部署资源
- `PUT /:id/resources/:resource_id` - 更新资源
- `PUT /:id/resources/:resource_id/dependencies` - 更新依赖
- `POST /:id/resources/:resource_id/restore` - 恢复资源
- `POST /:id/resources/:resource_id/versions/:version/rollback` - 回滚资源版本
- `POST /:id/snapshots` - 创建快照
- `POST /:id/snapshots/:snapshot_id/restore` - 恢复快照
- `POST /:id/resources/:resource_id/editing/start` - 开始编辑
- `POST /:id/resources/:resource_id/editing/heartbeat` - 编辑心跳
- `POST /:id/resources/:resource_id/editing/end` - 结束编辑
- `POST /:id/resources/:resource_id/drift/save` - 保存Drift
- `POST /:id/resources/:resource_id/drift/takeover` - 接管编辑

#### ADMIN级别
- `DELETE /:id/resources/:resource_id` - 删除资源
- `DELETE /:id/snapshots/:snapshot_id` - 删除快照
- `DELETE /:id/resources/:resource_id/drift` - 删除Drift

### workspace_execution（任务执行）

#### READ级别
- `GET /:id/tasks` - 列出任务
- `GET /:id/tasks/:task_id` - 查看任务详情
- `GET /:id/tasks/:task_id/logs` - 查看任务日志
- `GET /:id/tasks/:task_id/comments` - 查看评论
- `GET /:id/tasks/:task_id/resource-changes` - 查看资源变更
- `GET /:id/tasks/:task_id/state-backup` - 下载State备份

#### WRITE级别
- `POST /:id/tasks/plan` - 创建Plan任务
- `POST /:id/tasks/:task_id/comments` - 添加评论

#### ADMIN级别
- `POST /:id/tasks/:task_id/cancel` - 取消任务
- `POST /:id/tasks/:task_id/cancel-previous` - 取消之前的任务
- `POST /:id/tasks/:task_id/confirm-apply` - 确认Apply
- `PATCH /:id/tasks/:task_id/resource-changes/:resource_id` - 更新资源状态
- `POST /:id/tasks/:task_id/retry-state-save` - 重试State保存
- `POST /:id/tasks/:task_id/parse-plan` - 手动解析Plan

## 优化优势

### 1. 简化权限授予
**优化前：**
- 审计员需要授予：workspace_variables READ + workspace_state READ + workspace_resources READ + workspace_execution READ
- 需要授予4个权限

**优化后：**
- 审计员只需授予：workspace_management READ
- 只需授予1个权限即可查看所有数据

### 2. 清晰的权限层级
```
workspace_management READ (统一只读)
    ├── 查看变量
    ├── 查看State
    ├── 查看资源
    └── 查看任务（通过workspace_execution READ）

workspace_variables WRITE/ADMIN (变量修改)
workspace_state WRITE/ADMIN (State管理)
workspace_resources WRITE/ADMIN (资源管理)
workspace_execution WRITE/ADMIN (任务执行)
```

### 3. 符合实际使用场景

**只读用户（Auditor/Viewer）：**
- 授予：workspace_management READ
- 可以：查看所有数据
- 不能：修改任何内容

**开发者（Developer）：**
- 授予：workspace_management READ + workspace_variables WRITE + workspace_execution WRITE
- 可以：查看所有数据、管理变量、创建任务
- 不能：删除资源、取消任务

**运维人员（Operator）：**
- 授予：workspace_management READ + workspace_variables ADMIN + workspace_execution ADMIN + workspace_resources WRITE
- 可以：查看所有数据、完全管理变量、执行和取消任务、管理资源
- 不能：删除工作空间

**工作空间管理员（Workspace Admin）：**
- 授予：所有权限 ADMIN
- 可以：完全管理工作空间

## 数据迁移建议

### 迁移现有权限

对于已经授予了旧权限的用户，建议进行以下迁移：

```sql
-- 1. 为所有拥有workspace_variables READ的用户授予workspace_management READ
INSERT INTO workspace_permissions (principal_type, principal_id, permission_id, workspace_id, permission_level, granted_by, granted_at, reason)
SELECT 
    principal_type,
    principal_id,
    26, -- workspace_management的ID
    workspace_id,
    1, -- READ级别
    1, -- 系统管理员
    NOW(),
    '权限体系优化：统一只读权限迁移'
FROM workspace_permissions
WHERE permission_id = 11 -- workspace_variables
  AND permission_level = 1 -- READ
  AND NOT EXISTS (
    SELECT 1 FROM workspace_permissions wp2
    WHERE wp2.principal_type = workspace_permissions.principal_type
      AND wp2.principal_id = workspace_permissions.principal_id
      AND wp2.workspace_id = workspace_permissions.workspace_id
      AND wp2.permission_id = 26
  );

-- 2. 为所有拥有workspace_state READ的用户授予workspace_management READ
INSERT INTO workspace_permissions (principal_type, principal_id, permission_id, workspace_id, permission_level, granted_by, granted_at, reason)
SELECT 
    principal_type,
    principal_id,
    26, -- workspace_management的ID
    workspace_id,
    1, -- READ级别
    1, -- 系统管理员
    NOW(),
    '权限体系优化：统一只读权限迁移'
FROM workspace_permissions
WHERE permission_id = 10 -- workspace_state
  AND permission_level = 1 -- READ
  AND NOT EXISTS (
    SELECT 1 FROM workspace_permissions wp2
    WHERE wp2.principal_type = workspace_permissions.principal_type
      AND wp2.principal_id = workspace_permissions.principal_id
      AND wp2.workspace_id = workspace_permissions.workspace_id
      AND wp2.permission_id = 26
  );

-- 3. 为所有拥有workspace_resources READ的用户授予workspace_management READ
INSERT INTO workspace_permissions (principal_type, principal_id, permission_id, workspace_id, permission_level, granted_by, granted_at, reason)
SELECT 
    principal_type,
    principal_id,
    26, -- workspace_management的ID
    workspace_id,
    1, -- READ级别
    1, -- 系统管理员
    NOW(),
    '权限体系优化：统一只读权限迁移'
FROM workspace_permissions
WHERE permission_id = 24 -- workspace_resources
  AND permission_level = 1 -- READ
  AND NOT EXISTS (
    SELECT 1 FROM workspace_permissions wp2
    WHERE wp2.principal_type = workspace_permissions.principal_type
      AND wp2.principal_id = workspace_permissions.principal_id
      AND wp2.workspace_id = workspace_permissions.workspace_id
      AND wp2.permission_id = 26
  );

-- 4. 删除旧的READ权限（可选，建议保留一段时间以防回滚）
-- DELETE FROM workspace_permissions 
-- WHERE permission_id IN (10, 11, 24) -- workspace_state, workspace_variables, workspace_resources
--   AND permission_level = 1; -- READ
```

## 使用示例

### 授予只读权限

```bash
curl -X POST http://localhost:8080/api/v1/iam/permissions/grant \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "principal_type": "USER",
    "principal_id": 3,
    "resource_type": "workspace_management",
    "scope_type": "WORKSPACE",
    "scope_id": 12,
    "permission_level": "READ"
  }'
```

### 授予开发者权限（查看+变量管理+任务执行）

```bash
# 1. 授予统一只读权限
curl -X POST http://localhost:8080/api/v1/iam/permissions/batch-grant \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "principal_type": "USER",
    "principal_id": 3,
    "scope_type": "WORKSPACE",
    "scope_id": 12,
    "permissions": [
      {"permission_id": 26, "permission_level": "READ"},
      {"permission_id": 11, "permission_level": "WRITE"},
      {"permission_id": 9, "permission_level": "WRITE"}
    ],
    "reason": "开发者权限"
  }'
```

## 前端显示

在权限管理页面中，workspace_management权限的说明：

**READ级别：**
- 查看工作空间所有数据（变量、State、资源）- 统一只读权限

**WRITE级别：**
- 修改工作空间配置、锁定/解锁工作空间

**ADMIN级别：**
- 删除工作空间、完全管理工作空间

## 向后兼容性

- 旧的READ权限（workspace_variables READ, workspace_state READ, workspace_resources READ）仍然有效
- 建议通过数据迁移脚本为现有用户授予workspace_management READ权限
- 可以保留旧权限一段时间，确保平滑过渡

## 测试验证

### 测试workspace_management READ权限

1. 授予用户workspace_management READ权限
2. 验证可以访问：
   -  GET /workspaces/12/variables
   -  GET /workspaces/12/current-state
   -  GET /workspaces/12/resources
   -  GET /workspaces/12/overview
3. 验证不能访问：
   - ❌ POST /workspaces/12/variables (403)
   - ❌ POST /workspaces/12/state-versions/1/rollback (403)
   - ❌ POST /workspaces/12/resources (403)

### 测试专项WRITE权限

1. 授予workspace_management READ + workspace_variables WRITE
2. 验证可以：
   -  查看所有数据
   -  创建和修改变量
3. 验证不能：
   - ❌ 删除变量 (403)
   - ❌ 创建资源 (403)

## 相关文件

- `backend/internal/router/router.go` - 路由权限配置
- `frontend/src/pages/admin/PermissionManagement.tsx` - 权限管理UI
- `docs/iam/workspace-permissions-analysis.md` - 权限分析文档
- `docs/iam/workspace-task-permissions.md` - 任务权限文档

## 总结

优化后的权限体系：
1. **简化授权**：只需一个workspace_management READ权限即可查看所有数据
2. **清晰分层**：READ用于查看，WRITE/ADMIN用于修改和管理
3. **灵活控制**：可以精确控制用户能修改哪些类型的数据
4. **向后兼容**：不影响现有权限配置，支持平滑迁移
