# Workspace权限体系分析：workspace_variables vs workspace_management

## 权限定义对比

### workspace_variables (ID: 11)
- **显示名称**：变量管理
- **描述**：管理工作空间变量
- **创建时间**：早期权限（ID较小）
- **作用域**：工作空间级别（WORKSPACE scope）

### workspace_management (ID: 26)
- **显示名称**：工作空间管理
- **描述**：管理工作空间配置（查看、修改、删除工作空间）
- **创建时间**：后期新增权限
- **作用域**：工作空间级别（WORKSPACE scope）

## 当前使用情况

### workspace_variables 使用场景
**变量操作API（5个路由）：**
- `GET /:id/variables` - 列出变量（READ）
- `GET /:id/variables/:var_id` - 查看变量详情（READ）
- `POST /:id/variables` - 创建变量（WRITE）
- `PUT /:id/variables/:var_id` - 更新变量（WRITE）
- `DELETE /:id/variables/:var_id` - 删除变量（ADMIN）

### workspace_management 使用场景
**当前仅在overview API中作为可选权限之一：**
- `GET /:id/overview` - 查看工作空间概览（READ）
  - 可接受的权限包括：WORKSPACES, TASK_DATA_ACCESS, WORKSPACE_EXECUTION, WORKSPACE_MANAGEMENT, WORKSPACE_RESOURCES

**未使用的场景（根据描述应该包括）：**
- 工作空间配置修改（如terraform版本、执行模式等）
- 工作空间锁定/解锁
- 工作空间删除

## 问题分析

### 1. 权限职责重叠
两个权限的职责范围存在模糊地带：
- `workspace_variables`：专注于变量管理
- `workspace_management`：应该包含工作空间的整体配置管理，但变量也是配置的一部分

### 2. 命名不一致
- 旧权限使用小写下划线：`workspace_variables`, `workspace_execution`, `workspace_state`
- 新权限使用大写：`WORKSPACE_MANAGEMENT`, `WORKSPACE_RESOURCES`（在代码中）
- 但数据库中都是小写下划线

### 3. 权限粒度问题
当前设计存在两种可能的理解：

**理解A：workspace_management是更高层级的权限**
- `workspace_management`：管理工作空间本身（创建、删除、配置）
- `workspace_variables`：管理工作空间内的变量
- `workspace_execution`：管理工作空间内的任务执行
- `workspace_resources`：管理工作空间内的资源
- `workspace_state`：管理工作空间内的状态

**理解B：workspace_management包含所有配置管理**
- `workspace_management`：包含变量、设置等所有配置管理
- 其他权限：各自独立的功能域

## 建议方案

### 方案1：保持现状，明确职责边界（推荐）

**workspace_variables**：
- 专门用于变量的CRUD操作
- 保持现有5个API的权限检查

**workspace_management**：
- 用于工作空间级别的配置管理
- 应该控制以下操作：
  - 工作空间设置修改（terraform版本、执行模式、auto_apply等）
  - 工作空间锁定/解锁
  - 工作空间删除
  - 工作空间重命名/描述修改

**优点**：
- 权限粒度更细，符合最小权限原则
- 用户可以只授予变量管理权限，而不授予工作空间配置修改权限
- 向后兼容，不影响现有用户的权限配置

**需要的改动**：
1. 为工作空间配置相关API添加`workspace_management`权限检查
2. 更新文档说明两个权限的明确边界

### 方案2：合并权限

将`workspace_variables`合并到`workspace_management`中：
- 所有工作空间配置相关操作（包括变量）都使用`workspace_management`
- 废弃`workspace_variables`权限

**优点**：
- 权限体系更简洁
- 减少权限管理复杂度

**缺点**：
- 需要数据迁移
- 破坏向后兼容性
- 权限粒度变粗，不符合最小权限原则

### 方案3：重新设计权限层级

引入权限继承机制：
- `workspace_management` ADMIN级别自动包含`workspace_variables`的所有权限
- 但`workspace_variables`可以独立授予

**优点**：
- 灵活性最高
- 既保持细粒度，又提供便捷的高级权限

**缺点**：
- 实现复杂度高
- 需要修改IAM权限检查逻辑

## 推荐实施方案

### 采用方案1：保持现状，明确职责边界

#### 第一步：明确权限边界

**workspace_variables（变量管理）：**
-  GET /workspaces/:id/variables
-  GET /workspaces/:id/variables/:var_id
-  POST /workspaces/:id/variables
-  PUT /workspaces/:id/variables/:var_id
-  DELETE /workspaces/:id/variables/:var_id

**workspace_management（工作空间配置管理）：**
-  PUT /workspaces/:id - 更新工作空间配置
-  PATCH /workspaces/:id - 部分更新工作空间配置
-  DELETE /workspaces/:id - 删除工作空间
-  POST /workspaces/:id/lock - 锁定工作空间
-  POST /workspaces/:id/unlock - 解锁工作空间

#### 第二步：添加缺失的权限检查

为工作空间配置管理API添加`workspace_management`权限检查。

#### 第三步：更新文档

在权限管理页面和文档中明确说明：
- `workspace_variables`：仅用于管理工作空间变量
- `workspace_management`：用于管理工作空间本身的配置和生命周期

## 权限授予建议

### 角色权限映射

**开发者（Developer）：**
- workspace_variables: WRITE（可以创建和修改变量）
- workspace_execution: WRITE（可以创建Plan任务）
- workspace_resources: READ（可以查看资源）
- workspace_state: READ（可以查看状态）

**运维人员（Operator）：**
- workspace_variables: ADMIN（可以删除变量）
- workspace_execution: ADMIN（可以取消任务、确认Apply）
- workspace_resources: WRITE（可以管理资源）
- workspace_state: WRITE（可以管理状态）

**工作空间管理员（Workspace Admin）：**
- workspace_management: ADMIN（可以修改工作空间配置、删除工作空间）
- workspace_variables: ADMIN
- workspace_execution: ADMIN
- workspace_resources: ADMIN
- workspace_state: ADMIN

**审计员（Auditor）：**
- 所有权限: READ（只读访问）

## 总结

1. **保持两个权限独立**：`workspace_variables`专注于变量管理，`workspace_management`专注于工作空间配置管理
2. **补充权限检查**：为工作空间配置相关API添加`workspace_management`权限检查
3. **明确文档说明**：在权限管理界面和文档中清晰说明两个权限的职责边界
4. **向后兼容**：不影响现有用户的权限配置

这样的设计既保持了权限的细粒度控制，又明确了各权限的职责边界，符合最小权限原则和职责分离原则。
