# Workspace Variables 版本控制实施文档

## 实施概述

为 workspace_variables 表添加版本控制功能，支持变量的历史记录追踪和审计。

## 已完成工作

### 1. 数据库迁移 

**文件**: `scripts/add_variable_version_control.sql`

**执行结果**:
-  原表已重命名为 `workspace_variables_backup`
-  创建新表 `workspace_variables`
-  成功迁移 6 条记录
-  所有现有数据设置为 version 1

**新增字段**:
- `variable_id` VARCHAR(20): 变量语义化ID (var-xxxxxxxxxxxxxxxx)
- `version` INT: 版本号（从1开始）
- `is_deleted` BOOLEAN: 软删除标记

**索引**:
- `idx_variable_id`: variable_id 唯一索引
- `idx_variable_id_version`: (variable_id, version) 唯一索引
- `idx_workspace_key_type_version`: (workspace_id, key, variable_type, version) 唯一索引
- `idx_workspace_key_version`: 查询优化索引
- `idx_is_deleted`: 过滤已删除变量
- `idx_created_at`: 按时间查询

### 2. Model 层更新 

**文件**: `backend/internal/models/variable.go`

**更新内容**:
-  添加 `VariableID` 字段
-  添加 `Version` 字段
-  添加 `IsDeleted` 字段
-  实现 `BeforeCreate` hook 自动生成 variable_id
-  更新 `WorkspaceVariableResponse` 结构体
-  更新 `ToResponse()` 方法

## 待完成工作

### 3. Service 层更新 ⏳

**文件**: `backend/services/workspace_variable_service.go`

**需要修改的方法**:

#### 3.1 CreateVariable
```go
// 当前逻辑：直接创建
// 新逻辑：
// - 检查 key 是否存在（查询最新未删除版本）
// - 如果不存在，创建 version 1
// - variable_id 由 BeforeCreate hook 自动生成
```

#### 3.2 UpdateVariable
```go
// 当前逻辑：UPDATE 更新记录
// 新逻辑：
// - 查询当前最新版本
// - 创建新版本记录（version + 1）
// - variable_id 保持不变
// - 支持通过 variable_id 或 id 查询
```

#### 3.3 DeleteVariable
```go
// 当前逻辑：DELETE 删除记录
// 新逻辑：
// - 查询当前最新版本
// - 创建新版本记录（version + 1）
// - 设置 is_deleted = true
```

#### 3.4 ListVariables
```go
// 当前逻辑：查询所有记录
// 新逻辑：
// - 只返回每个 variable_id 的最新版本
// - 过滤 is_deleted = true 的记录
// - 使用子查询或 DISTINCT ON
```

#### 3.5 GetVariable
```go
// 当前逻辑：通过 id 查询
// 新逻辑：
// - 支持通过 id 或 variable_id 查询
// - 返回最新未删除版本
```

#### 3.6 新增方法
```go
// GetVariableVersions - 获取变量的所有历史版本
func (s *WorkspaceVariableService) GetVariableVersions(variableID string) ([]*models.WorkspaceVariable, error)

// GetVariableVersion - 获取变量的指定版本
func (s *WorkspaceVariableService) GetVariableVersion(variableID string, version int) (*models.WorkspaceVariable, error)

// GetVariableByVariableID - 通过 variable_id 获取最新版本
func (s *WorkspaceVariableService) GetVariableByVariableID(variableID string) (*models.WorkspaceVariable, error)
```

### 4. Controller 层更新 ⏳

**文件**: `backend/controllers/workspace_variable_controller.go`

**需要修改的方法**:

#### 4.1 UpdateVariable
```go
// 支持两种ID格式：
// - var-xxxxxxxxxxxxxxxx (variable_id)
// - 数字 (id，向后兼容)
```

#### 4.2 DeleteVariable
```go
// 支持两种ID格式
// 调用 Service 的软删除方法
```

#### 4.3 GetVariable
```go
// 支持两种ID格式
```

#### 4.4 新增方法
```go
// GetVariableVersions - 获取变量版本历史
// GET /api/v1/workspaces/{id}/variables/{var_id}/versions

// GetVariableVersion - 获取指定版本
// GET /api/v1/workspaces/{id}/variables/{var_id}/versions/{version}
```

### 5. Router 更新 ⏳

**文件**: `backend/internal/router/router_workspace.go`

**需要添加的路由**:
```go
variableGroup.GET("/:var_id/versions", variableController.GetVariableVersions)
variableGroup.GET("/:var_id/versions/:version", variableController.GetVariableVersion)
```

### 6. 前端更新 ⏳

**可选功能**:
- 在变量管理页面显示版本历史入口
- 版本历史查看页面
- 版本对比功能

### 7. 测试 ⏳

**测试项**:
- [ ] 创建变量（生成 variable_id 和 version 1）
- [ ] 更新变量（创建新版本）
- [ ] 删除变量（软删除）
- [ ] 查询变量列表（只显示最新版本）
- [ ] 查询版本历史
- [ ] 查询指定版本
- [ ] 加密变量的版本控制
- [ ] API 向后兼容性（支持数字ID）

## 设计要点

### 版本控制策略

1. **创建变量**: 
   - 自动生成 variable_id
   - version = 1
   - is_deleted = false

2. **更新变量**:
   - 保持 variable_id 不变
   - version = 当前版本 + 1
   - 插入新记录

3. **删除变量**:
   - 保持 variable_id 不变
   - version = 当前版本 + 1
   - is_deleted = true
   - 插入新记录

4. **查询当前变量**:
   - 过滤 is_deleted = false
   - 选择每个 variable_id 的最大 version

### 数据一致性

- 使用唯一索引保证 (variable_id, version) 唯一
- 使用唯一索引保证 (workspace_id, key, variable_type, version) 唯一
- 同一 workspace 内，同一 key 只能有一个未删除的最新版本

### 加密处理

- 继续使用现有的 BeforeCreate/BeforeSave/AfterFind hooks
- 每个版本的敏感变量值独立加密
- API 响应时敏感变量值不返回

### 向后兼容

- Controller 层支持两种ID格式：
  - variable_id (var-xxxxxxxxxxxxxxxx)
  - id (数字，向后兼容)
- 现有 API 行为保持不变
- 新增版本历史查询 API

## 回滚方案

如果需要回滚到旧版本：

```sql
-- 删除新表
DROP TABLE workspace_variables;

-- 恢复备份表
ALTER TABLE workspace_variables_backup RENAME TO workspace_variables;
```

## 下一步行动

1. 更新 Service 层（约 2 小时）
2. 更新 Controller 层（约 1.5 小时）
3. 更新 Router（约 0.5 小时）
4. 测试验证（约 2 小时）
5. 编写 API 文档（约 0.5 小时）

**预计剩余时间**: 6.5 小时

## 参考

- State Version 实现: `backend/controllers/state_version_controller.go`
- ID 生成器: `backend/internal/infrastructure/id_generator.go`
- 加密实现: `backend/internal/crypto/variable_crypto.go`
