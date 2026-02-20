# 工作空间权限整合方案：统一到workspace_management

## 概述

将所有工作空间相关权限整合到单一的`workspace_management`权限下，通过READ/WRITE/ADMIN三个级别控制所有操作。

## 当前权限体系

### 现有的工作空间级权限
1. **task_data_access** (ID: 8) - 任务数据访问
2. **workspace_execution** (ID: 9) - 工作空间执行
3. **workspace_state** (ID: 10) - 状态管理
4. **workspace_variables** (ID: 11) - 变量管理
5. **workspace_resources** (ID: 24) - 资源管理
6. **workspace_management** (ID: 26) - 工作空间管理

## 整合方案

### workspace_management 三级权限设计

#### READ级别 - 查看所有数据
**任务相关（6个API）：**
- GET /:id/tasks - 列出任务
- GET /:id/tasks/:task_id - 查看任务详情
- GET /:id/tasks/:task_id/logs - 查看任务日志
- GET /:id/tasks/:task_id/comments - 查看评论
- GET /:id/tasks/:task_id/resource-changes - 查看资源变更
- GET /:id/tasks/:task_id/state-backup - 下载State备份

**变量相关（2个API）：**
- GET /:id/variables - 列出变量
- GET /:id/variables/:var_id - 查看变量详情

**State相关（5个API）：**
- GET /:id/current-state - 查看当前State
- GET /:id/state-versions - 列出State版本
- GET /:id/state-versions/compare - 对比State版本
- GET /:id/state-versions/:version/metadata - 查看State元数据
- GET /:id/state-versions/:version - 查看特定State版本

**资源相关（10个API）：**
- GET /:id/resources - 列出资源
- GET /:id/resources/:resource_id - 查看资源详情
- GET /:id/resources/:resource_id/versions - 查看资源版本
- GET /:id/resources/:resource_id/versions/compare - 对比资源版本
- GET /:id/resources/:resource_id/versions/:version - 查看特定资源版本
- GET /:id/resources/:resource_id/dependencies - 查看资源依赖
- GET /:id/snapshots - 列出快照
- GET /:id/snapshots/:snapshot_id - 查看快照详情
- GET /:id/resources/:resource_id/editing/status - 查看编辑状态
- GET /:id/resources/:resource_id/drift - 查看Drift

**工作空间信息：**
- GET /:id - 查看工作空间详情
- GET /:id/overview - 查看工作空间概览

#### WRITE级别 - READ + 创建/修改操作

**继承READ级别的所有权限，额外增加：**

**任务操作（2个API）：**
- POST /:id/tasks/plan - 创建Plan任务
- POST /:id/tasks/:task_id/comments - 添加评论

**变量操作（2个API）：**
- POST /:id/variables - 创建变量
- PUT /:id/variables/:var_id - 更新变量
- DELETE /:id/variables/:var_id - 删除变量

**资源操作（14个API）：**
- POST /:id/resources - 创建资源
- POST /:id/resources/import - 导入资源
- POST /:id/resources/deploy - 部署资源
- PUT /:id/resources/:resource_id - 更新资源
- DELETE /:id/resources/:resource_id - 删除资源
- PUT /:id/resources/:resource_id/dependencies - 更新依赖
- POST /:id/resources/:resource_id/restore - 恢复资源
- POST /:id/resources/:resource_id/versions/:version/rollback - 回滚资源版本
- POST /:id/snapshots - 创建快照
- POST /:id/snapshots/:snapshot_id/restore - 恢复快照
- DELETE /:id/snapshots/:snapshot_id - 删除快照
- POST /:id/resources/:resource_id/editing/start - 开始编辑
- POST /:id/resources/:resource_id/editing/heartbeat - 编辑心跳
- POST /:id/resources/:resource_id/editing/end - 结束编辑
- POST /:id/resources/:resource_id/drift/save - 保存Drift
- POST /:id/resources/:resource_id/drift/takeover - 接管编辑
- DELETE /:id/resources/:resource_id/drift - 删除Drift

**State操作（1个API）：**
- POST /:id/state-versions/:version/rollback - 回滚State

#### ADMIN级别 - 所有权限

**继承WRITE级别的所有权限，额外增加：**

**任务管理（6个API）：**
- POST /:id/tasks/:task_id/cancel - 取消任务
- POST /:id/tasks/:task_id/cancel-previous - 取消之前的任务
- POST /:id/tasks/:task_id/confirm-apply - 确认Apply
- PATCH /:id/tasks/:task_id/resource-changes/:resource_id - 更新资源状态
- POST /:id/tasks/:task_id/retry-state-save - 重试State保存
- POST /:id/tasks/:task_id/parse-plan - 手动解析Plan

**State管理（1个API）：**
- DELETE /:id/state-versions/:version - 删除State版本

**工作空间管理（3个API）：**
- PUT /:id - 更新工作空间配置
- PATCH /:id - 部分更新工作空间配置
- DELETE /:id - 删除工作空间
- POST /:id/lock - 锁定工作空间
- POST /:id/unlock - 解锁工作空间

## 整合优势

### 1. 极大简化权限管理
**整合前：** 需要管理6个不同的工作空间权限
**整合后：** 只需管理1个workspace_management权限

### 2. 更符合直觉的权限模型
- READ = 只读访问
- WRITE = 日常操作（开发者/运维）
- ADMIN = 完全控制（管理员）

### 3. 减少授权复杂度
**整合前授予开发者权限：**
```
workspace_execution READ
workspace_execution WRITE
workspace_variables READ
workspace_variables WRITE
workspace_resources READ
workspace_state READ
```

**整合后授予开发者权限：**
```
workspace_management WRITE
```

### 4. 降低权限配置错误风险
- 不会出现"有变量READ但没有State READ"的不一致情况
- 权限配置更加统一和可预测

## 实施计划

### 阶段1：代码修改（需要实施）

修改`backend/internal/router/router.go`中的所有工作空间路由，将权限检查统一为：
- 所有GET操作 → workspace_management READ
- 所有POST/PUT操作（除了管理操作）→ workspace_management WRITE
- 所有DELETE操作和管理操作 → workspace_management ADMIN

### 阶段2：数据迁移

```sql
-- 1. 为所有拥有任何工作空间权限的用户授予workspace_management相应级别
-- 2. 删除旧的工作空间权限
-- 3. 可选：保留task_data_access用于特殊的审计场景
```

### 阶段3：前端更新

- 更新权限管理页面，只显示workspace_management
- 更新权限说明文档
- 更新用户指南

### 阶段4：废弃旧权限

- 标记旧权限为deprecated
- 在一段时间后从数据库中删除旧权限定义

## 权限级别详细说明

### READ级别
**适用角色：** 审计员、观察者、只读用户

**可以做什么：**
- 查看工作空间的所有信息和数据
- 查看任务执行历史和日志
- 查看变量配置（敏感值可能隐藏）
- 查看State版本和内容
- 查看资源配置和依赖关系
- 下载State备份

**不能做什么：**
- 不能创建或修改任何内容
- 不能执行任何操作
- 不能删除任何数据

### WRITE级别
**适用角色：** 开发者、运维人员、日常操作者

**可以做什么：**
- READ级别的所有权限
- 创建和执行Plan任务
- 创建、修改、删除变量
- 创建、修改、删除资源
- 导入和部署资源
- 管理资源快照
- 回滚State和资源版本
- 管理资源编辑会话
- 管理Drift

**不能做什么：**
- 不能取消正在运行的任务
- 不能确认Apply操作
- 不能删除工作空间
- 不能修改工作空间配置
- 不能删除State版本

### ADMIN级别
**适用角色：** 工作空间管理员、高级管理员

**可以做什么：**
- WRITE级别的所有权限
- 取消任务
- 确认Apply操作
- 修改工作空间配置
- 锁定/解锁工作空间
- 删除工作空间
- 删除State版本
- 完全控制工作空间

## 迁移策略

### 权限级别映射

**旧权限 → 新权限映射：**

| 旧权限组合 | 新权限 |
|-----------|--------|
| 任何权限 READ | workspace_management READ |
| workspace_execution WRITE + workspace_variables WRITE | workspace_management WRITE |
| workspace_resources WRITE | workspace_management WRITE |
| workspace_execution ADMIN | workspace_management ADMIN |
| 任何权限 ADMIN | workspace_management ADMIN |

### 迁移规则

1. 如果用户有任何工作空间权限的ADMIN级别 → workspace_management ADMIN
2. 如果用户有任何工作空间权限的WRITE级别 → workspace_management WRITE
3. 如果用户只有READ级别权限 → workspace_management READ

## 风险评估

### 低风险
-  权限只会变得更宽松，不会更严格
-  不会导致用户失去现有权限
-  向后兼容，可以逐步迁移

### 需要注意
-  WRITE级别用户将获得更多权限（如删除变量、删除资源）
-  需要审查现有WRITE级别用户是否应该有这些权限
-  建议先在测试环境验证

### 缓解措施
1. 在迁移前备份权限表
2. 先迁移READ级别权限（风险最低）
3. 逐步迁移WRITE和ADMIN级别
4. 保留旧权限一段时间以便回滚

## 推荐实施步骤

### 第1步：评估现有权限（当前步骤）
-  分析现有权限使用情况
-  设计新的权限模型
-  评估风险和影响

### 第2步：修改代码
- 修改所有路由使用workspace_management权限
- 更新权限检查逻辑

### 第3步：数据迁移
- 为现有用户授予workspace_management权限
- 验证迁移结果

### 第4步：清理
- 废弃旧权限
- 更新文档
- 培训用户

## 建议

**推荐采用此整合方案，理由：**
1. **极大简化**：从6个权限减少到1个权限
2. **更直观**：READ/WRITE/ADMIN三级模型易于理解
3. **减少错误**：不会出现权限配置不一致的情况
4. **易于管理**：授权和撤销都更简单

**需要注意：**
1. WRITE级别权限范围较大，需要谨慎授予
2. 建议先在测试环境验证
3. 需要审查现有用户的权限是否合适

## 下一步

如果确认采用此方案，我将：
1. 修改所有路由的权限检查逻辑
2. 创建完整的数据迁移脚本
3. 更新前端权限管理页面
4. 更新所有相关文档
