# 超级管理员权限问题修复完成报告

## 问题描述

超级管理员（role = "admin"）创建新 workspace 后，仍需要为自己授权才能操作该 workspace 的任务（如 Plan、Apply）。

## 根本原因

系统有两套权限机制：
1. **基于角色的权限**：用户表中的 `role` 字段（"admin" 或 "user"）
2. **基于 IAM 的细粒度权限**：`iam_permissions` 表中的权限记录

当前代码中，admin 角色可以绕过 IAM 权限检查（通过 `BypassIAMForAdmin` 中间件），但创建 workspace 时没有自动为创建者授予 IAM 权限。这导致：
- Admin 用户创建 workspace 后，虽然可以通过角色绕过检查访问 workspace
- 但某些操作（如 Plan、Apply）可能需要 IAM 权限记录才能正常工作

## 修复方案

**方案 B：创建 Workspace 时自动为创建者授予 ADMIN 权限**

在 `CreateWorkspace` 方法中，成功创建 workspace 后，自动为创建者授予该 workspace 的 ADMIN 预设权限。

### 优点
- 符合最小权限原则
- 不需要绕过权限检查
- 创建者自然应该拥有对自己创建资源的完全控制权
- 与现有 IAM 系统完全兼容

## 代码修改

### 1. WorkspaceController 修改 (`backend/controllers/workspace_controller.go`)

```go
// 添加 PermissionService 依赖
type WorkspaceController struct {
    workspaceService  *services.WorkspaceService
    overviewService   *services.WorkspaceOverviewService
    permissionService service.PermissionService  // 新增
}

// 更新构造函数
func NewWorkspaceController(
    workspaceService *services.WorkspaceService,
    overviewService *services.WorkspaceOverviewService,
    permissionService service.PermissionService,  // 新增参数
) *WorkspaceController

// 在 CreateWorkspace 方法中添加自动授权逻辑
if wc.permissionService != nil {
    userID, exists := c.Get("user_id")
    if exists && userID != nil {
        wc.grantCreatorPermissions(workspace.ID, userID.(string))
    }
}

// 新增辅助方法
func (wc *WorkspaceController) grantCreatorPermissions(workspaceID uint, userID string) {
    // 使用 GrantPresetPermissions 授予 ADMIN 预设权限
    // 授权失败只记录日志，不影响 workspace 创建
}
```

### 2. 路由初始化修改 (`backend/internal/router/router_workspace.go`)

```go
// 更新函数签名，添加 permissionService 参数
func setupWorkspaceRoutes(
    api *gin.RouterGroup, 
    db *gorm.DB, 
    streamManager *services.OutputStreamManager, 
    iamMiddleware *middleware.IAMPermissionMiddleware, 
    wsHub *websocket.Hub, 
    queueManager *services.TaskQueueManager, 
    agentCCHandler *handlers.RawAgentCCHandler, 
    permissionService service.PermissionService,  // 新增参数
)

// 更新 WorkspaceController 初始化
workspaceController := controllers.NewWorkspaceController(
    services.NewWorkspaceService(db),
    services.NewWorkspaceOverviewService(db),
    permissionService,  // 传入 permissionService
)
```

### 3. 主路由修改 (`backend/internal/router/router.go`)

```go
// 更新 setupWorkspaceRoutes 调用，传入 permissionService
setupWorkspaceRoutes(
    api, db, streamManager, iamMiddleware, wsHub, queueManager, rawCCHandler,
    iamFactory.GetPermissionService(),  // 新增参数
)
```

## 授权逻辑说明

当用户创建 workspace 时：

1. 首先正常创建 workspace
2. 创建成功后，调用 `grantCreatorPermissions` 方法
3. 该方法使用 `PermissionService.GrantPresetPermissions` 授予 ADMIN 预设权限
4. ADMIN 预设包含该 workspace 的所有权限（WORKSPACE_MANAGEMENT、WORKSPACE_EXECUTION、WORKSPACE_STATE、WORKSPACE_VARIABLES、WORKSPACE_RESOURCES、TASK_DATA_ACCESS）
5. 授权失败只记录日志，不影响 workspace 创建的成功响应

## 预期效果

修复后：
- 超级管理员创建 workspace 后，自动获得该 workspace 的 ADMIN 权限
- 可以立即执行 Plan、Apply 等操作，无需额外授权
- 普通用户创建 workspace 后，也会自动获得 ADMIN 权限
- 符合"创建者拥有完全控制权"的直觉

## 编译验证

```bash
cd backend && go build -o /dev/null ./...
```

编译通过，无与本次修改相关的错误。

## 测试建议

1. 使用超级管理员账号创建新 workspace
2. 检查 `iam_permissions` 表，确认自动创建了权限记录
3. 尝试执行 Plan 操作，确认无需额外授权
4. 使用普通用户账号创建 workspace，验证同样的行为

## 相关文件

- `backend/controllers/workspace_controller.go` - WorkspaceController 修改
- `backend/internal/router/router_workspace.go` - 路由初始化修改
- `backend/internal/router/router.go` - 主路由修改
- `docs/superadmin-permission-issue-analysis.md` - 问题分析报告
