# Workspace ID 语义化实施计划

## 当前状况

### 数据库
-  workspaces 表已添加 workspace_id 字段 (VARCHAR(50))
-  已为现有记录生成语义化 ID
-  已创建触发器自动生成 workspace_id
-  保留了原有的 id 字段 (INTEGER,自增)

### Go 模型
-  Workspace 模型已更新:
  - `ID uint` - 内部主键 (json:"-" 不对外暴露)
  - `WorkspaceID string` - 语义化ID (json:"id" 对外使用)

## 需要修改的代码

### 1. 模型层 (backend/internal/models/)

#### 已修改
-  workspace.go - Workspace 结构体

#### 需要检查的关联模型
这些模型引用了 Workspace,需要确认外键关系:
- WorkspaceTask - 使用 WorkspaceID uint (引用 workspaces.id)
- WorkspaceStateVersion - 使用 WorkspaceID uint (引用 workspaces.id)
- WorkspaceTaskResourceChange - 使用 WorkspaceID uint (引用 workspaces.id)
- WorkspaceVariable - 需要检查
- WorkspaceResource - 需要检查
- WorkspaceMember - 需要检查
- WorkspacePermission - 需要检查

**结论**: 这些模型继续使用 uint 类型的 WorkspaceID 是正确的,因为它们引用的是 workspaces.id (内部主键)

### 2. 服务层 (backend/services/)

需要修改的文件和函数:

#### workspace_service.go
```go
// 当前使用 uint 类型的 ID
func (ws *WorkspaceService) GetWorkspaceByID(id uint) (*models.Workspace, error)
func (ws *WorkspaceService) UpdateWorkspace(id uint, ...) error
func (ws *WorkspaceService) DeleteWorkspace(id uint) error

// 需要添加支持 workspace_id 的方法
func (ws *WorkspaceService) GetWorkspaceByWorkspaceID(workspaceID string) (*models.Workspace, error)
func (ws *WorkspaceService) UpdateWorkspaceByWorkspaceID(workspaceID string, ...) error
func (ws *WorkspaceService) DeleteWorkspaceByWorkspaceID(workspaceID string) error
```

#### workspace_overview_service.go
```go
// 当前
func (s *WorkspaceOverviewService) GetWorkspaceOverview(workspaceID uint) (...)
func (s *WorkspaceOverviewService) UpdateResourceCount(workspaceID uint) error

// 需要支持两种方式
// 方案1: 重载方法
// 方案2: 统一使用 workspace_id (推荐)
```

#### 其他服务
- workspace_variable_service.go
- workspace_lifecycle.go
- terraform_executor.go

### 3. 控制器层 (backend/controllers/)

需要修改的文件:

#### workspace_controller.go
```go
// 当前路由: /api/v1/workspaces/:id
// 参数解析: id, _ := strconv.ParseUint(c.Param("id"), 10, 32)

// 需要改为支持 workspace_id
// 新路由: /api/v1/workspaces/:workspace_id
// 参数解析: workspaceID := c.Param("workspace_id") // 直接使用字符串
```

#### workspace_task_controller.go
- CreatePlanTask
- CreateApplyTask
- GetTask
- GetTasks
- ConfirmApply
- CancelTask
- 等等

#### 其他控制器
- workspace_variable_controller.go
- state_version_controller.go
- dashboard_controller.go

### 4. 路由层 (backend/internal/router/)

需要修改路由定义:
```go
// 当前
workspaces.GET("/:id", controller.GetWorkspace)
workspaces.PUT("/:id", controller.UpdateWorkspace)

// 改为
workspaces.GET("/:workspace_id", controller.GetWorkspace)
workspaces.PUT("/:workspace_id", controller.UpdateWorkspace)
```

## 实施策略

### 方案 A: 渐进式迁移 (推荐)

#### 阶段 1: 添加新方法,保留旧方法
```go
// 保留旧方法 (使用 uint id)
func GetWorkspaceByID(id uint) (*Workspace, error)

// 添加新方法 (使用 string workspace_id)
func GetWorkspaceByWorkspaceID(workspaceID string) (*Workspace, error)
```

#### 阶段 2: 路由同时支持两种格式
```go
// 旧路由 (兼容)
workspaces.GET("/:id", controller.GetWorkspace)

// 新路由
workspaces.GET("/ws/:workspace_id", controller.GetWorkspaceByWorkspaceID)
```

#### 阶段 3: 逐步迁移前端
- 前端逐步改用新的 API
- 验证功能正常

#### 阶段 4: 废弃旧方法
- 删除旧的 API
- 删除旧的方法

### 方案 B: 一次性切换 (风险高)

直接修改所有代码,一次性切换到 workspace_id。

**不推荐**: 风险太高,影响范围太大。

## 关键决策点

### 1. API 路由格式

**选项 A**: 保持路径不变,参数类型改变
```
GET /api/v1/workspaces/:id
// 之前: id 是数字
// 之后: id 是 workspace_id (ws-xxx)
```

**选项 B**: 修改路径,明确使用 workspace_id
```
GET /api/v1/workspaces/:workspace_id
// 明确表示使用 workspace_id
```

**选项 C**: 同时支持两种
```
GET /api/v1/workspaces/:id          // 兼容旧的数字 ID
GET /api/v1/workspaces/ws/:workspace_id  // 新的语义化 ID
```

### 2. 内部查询方式

**当前问题**: 很多地方使用 `Where("id = ?", workspaceID)`

**解决方案**:
```go
// 方案 1: 修改所有查询为使用 workspace_id
db.Where("workspace_id = ?", workspaceID)

// 方案 2: 在服务层统一处理
func (s *Service) getWorkspace(id interface{}) (*Workspace, error) {
    switch v := id.(type) {
    case uint:
        return s.db.Where("id = ?", v).First(&Workspace{}).Error
    case string:
        return s.db.Where("workspace_id = ?", v).First(&Workspace{}).Error
    }
}
```

## 工作量评估

### 如果采用渐进式迁移 (方案 A)

#### 第一阶段: 服务层支持双ID (3天)
- 添加新的查询方法
- 保留旧方法
- 单元测试

#### 第二阶段: 控制器层支持双ID (2天)
- 添加新的路由和处理器
- 保留旧路由
- 集成测试

#### 第三阶段: 前端迁移 (3天)
- 修改 API 调用
- 修改类型定义
- 功能测试

#### 第四阶段: 清理旧代码 (1天)
- 删除旧方法
- 删除旧路由
- 回归测试

**总计**: 9天

## 建议

1. **采用渐进式迁移** (方案 A)
   - 风险低
   - 可以逐步验证
   - 可以随时回退

2. **API 路由使用选项 A**
   - 保持路径不变
   - 参数从数字变为字符串
   - 前端改动最小

3. **优先级**
   - 先完成核心的 workspace CRUD API
   - 再处理 task 相关 API
   - 最后处理其他功能

4. **测试策略**
   - 每个阶段都充分测试
   - 保持旧功能可用
   - 新旧并存一段时间

## 下一步

请确认:
1. 是否采用渐进式迁移?
2. API 路由格式选择哪个?
3. 是否现在开始实施?

如果确认,我将开始修改代码。
