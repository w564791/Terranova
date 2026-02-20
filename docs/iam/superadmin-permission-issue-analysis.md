# 超级管理员权限问题分析报告

## 问题描述

用户反馈：即便是超级管理员，当新建 workspace 后，仍需要为自己授权才能操作这个新建的 workspace 里的任务。

## 问题分析

### 1. 当前权限系统架构

系统采用了两套并行的权限机制：

#### 1.1 基于角色的权限（Role-based）
- 用户模型中有 `Role` 字段，值为 `"admin"` 或 `"user"`
- 用户模型中还有 `IsSystemAdmin` 字段（布尔值），但**完全未被使用**
- 在路由中，通过检查 `role == "admin"` 来绕过 IAM 权限检查

#### 1.2 基于 IAM 的细粒度权限
- 通过 `permission_checker.go` 实现
- 支持组织、项目、工作空间三级作用域
- 支持用户直接授权、团队授权、角色授权
- 权限级别：NONE、READ、WRITE、ADMIN

### 2. 问题根因分析

经过代码审查，发现以下问题：

#### 问题 1：`IsSystemAdmin` 字段未被使用

在 `backend/internal/models/user.go` 中定义了 `IsSystemAdmin` 字段：
```go
IsSystemAdmin bool `json:"is_system_admin" gorm:"default:false"`
```

但在整个后端代码中，这个字段**从未被使用**。搜索结果显示只有模型定义处有这个字段。

#### 问题 2：Admin 角色绕过逻辑不完整

在 `router_workspace.go` 中，每个路由都有类似的检查：
```go
role, _ := c.Get("role")
if role == "admin" {
    workspaceController.CreateWorkspace(c)
    return
}
```

这个检查依赖于 JWT token 中的 `role` 字段。但问题是：

1. **JWT 中间件没有设置 `is_system_admin`**：在 `middleware.go` 中，只设置了 `role`，没有设置 `is_system_admin`
2. **权限检查器没有考虑超级管理员**：`permission_checker.go` 中的 `CheckPermission` 方法没有对超级管理员进行特殊处理

#### 问题 3：Workspace 创建时没有自动授权

在 `workspace_service.go` 的 `CreateWorkspace` 方法中：
```go
func (ws *WorkspaceService) CreateWorkspace(workspace *models.Workspace) error {
    workspaceID, err := infrastructure.GenerateWorkspaceID()
    if err != nil {
        return fmt.Errorf("failed to generate workspace ID: %w", err)
    }
    workspace.WorkspaceID = workspaceID
    return ws.db.Create(workspace).Error
}
```

**没有为创建者自动授予权限**。这意味着即使是 admin 用户创建了 workspace，如果后续的权限检查没有正确绕过，也会失败。

#### 问题 4：权限检查的不一致性

当前系统中存在两种权限检查方式的混用：
1. 路由层面的 `role == "admin"` 检查
2. IAM 中间件的细粒度权限检查

这导致了以下问题：
- 某些路由可能遗漏了 admin 绕过检查
- 权限检查逻辑分散在多处，难以维护

### 3. 具体场景分析

假设用户 A 是 admin（`role = "admin"`）：

1. **创建 Workspace**：
   - 路由检查 `role == "admin"` → 通过
   - 创建成功，但没有为用户 A 授予任何权限

2. **访问新建的 Workspace**：
   - 路由检查 `role == "admin"` → 通过（如果路由正确实现）
   - 但如果某些路由遗漏了 admin 检查，会走 IAM 权限检查
   - IAM 检查发现用户 A 没有该 workspace 的权限 → 拒绝

3. **执行任务（Plan/Apply）**：
   - 同上，如果 admin 检查被遗漏，会被 IAM 拒绝

## 修复方案

### 方案 A：完善 Admin 角色绕过机制（推荐，短期方案）

**优点**：改动小，风险低，快速解决问题
**缺点**：不是长期最佳实践

#### 修改点 1：在 JWT 中间件中设置 `is_system_admin`

修改 `backend/internal/middleware/middleware.go`：
```go
// 在验证 token 后，查询用户的 is_system_admin 字段
var user struct {
    Role          string
    IsSystemAdmin bool
    IsActive      bool
}
if err := globalDB.Table("users").
    Select("role, is_system_admin, is_active").
    Where("user_id = ?", userID).
    First(&user).Error; err == nil {
    c.Set("role", user.Role)
    c.Set("is_system_admin", user.IsSystemAdmin)
}
```

#### 修改点 2：在权限检查器中添加超级管理员绕过

修改 `backend/internal/application/service/permission_checker.go`：
```go
func (c *PermissionCheckerImpl) CheckPermission(
    ctx context.Context,
    req *CheckPermissionRequest,
) (*CheckPermissionResult, error) {
    // 新增：检查是否为超级管理员
    if c.isSystemAdmin(ctx, req.UserID) {
        return &CheckPermissionResult{
            IsAllowed:      true,
            EffectiveLevel: valueobject.PermissionLevelAdmin,
            Source:         "system_admin",
            CacheHit:       false,
        }, nil
    }
    
    // ... 原有逻辑
}

func (c *PermissionCheckerImpl) isSystemAdmin(ctx context.Context, userID string) bool {
    var user struct {
        IsSystemAdmin bool
    }
    if err := c.projectRepo.GetDB().Table("users").
        Select("is_system_admin").
        Where("user_id = ?", userID).
        First(&user).Error; err != nil {
        return false
    }
    return user.IsSystemAdmin
}
```

#### 修改点 3：在 IAM 中间件中添加超级管理员绕过

修改 `backend/internal/middleware/iam_permission.go`：
```go
func (m *IAMPermissionMiddleware) RequirePermission(...) gin.HandlerFunc {
    return func(c *gin.Context) {
        // 检查是否为 admin 角色
        role, roleExists := c.Get("role")
        if roleExists && role == "admin" {
            c.Next()
            return
        }
        
        // 新增：检查是否为超级管理员
        isSystemAdmin, _ := c.Get("is_system_admin")
        if isSystemAdmin == true {
            c.Next()
            return
        }
        
        // ... 原有 IAM 检查逻辑
    }
}
```

### 方案 B：Workspace 创建时自动授权（补充方案）

**优点**：符合最小权限原则
**缺点**：需要更多改动

#### 修改点：在创建 Workspace 时自动为创建者授予 ADMIN 权限

修改 `backend/controllers/workspace_controller.go` 的 `CreateWorkspace` 方法：
```go
func (wc *WorkspaceController) CreateWorkspace(c *gin.Context) {
    // ... 原有创建逻辑
    
    if err := wc.workspaceService.CreateWorkspace(workspace); err != nil {
        // ... 错误处理
    }
    
    // 新增：为创建者授予 ADMIN 权限
    userID, _ := c.Get("user_id")
    if userID != nil {
        // 调用权限服务为创建者授予 WORKSPACE_MANAGEMENT 的 ADMIN 权限
        // 这需要注入 PermissionService
    }
    
    // ... 返回响应
}
```

### 方案 C：统一使用 IAM 权限系统（长期方案）

**优点**：架构清晰，权限管理统一
**缺点**：改动大，需要全面测试

1. 移除所有路由中的 `role == "admin"` 检查
2. 在 IAM 权限检查器中统一处理超级管理员
3. 为所有资源创建时自动授权给创建者
4. 逐步迁移所有权限检查到 IAM 系统

## 推荐实施方案

根据您的反馈，**推荐实施方案 B**：在创建 Workspace 时自动为创建者授予 ADMIN 权限。

这是更合理的做法，因为：
1. 符合最小权限原则
2. 不需要绕过权限检查
3. 创建者自然应该拥有对自己创建资源的完全控制权

### 方案 B 详细实现

#### 修改 1：在 WorkspaceController 中注入 PermissionService

修改 `backend/controllers/workspace_controller.go`：

```go
type WorkspaceController struct {
    workspaceService  *services.WorkspaceService
    overviewService   *services.WorkspaceOverviewService
    permissionService service.PermissionService  // 新增
}

func NewWorkspaceController(
    workspaceService *services.WorkspaceService, 
    overviewService *services.WorkspaceOverviewService,
    permissionService service.PermissionService,  // 新增
) *WorkspaceController {
    return &WorkspaceController{
        workspaceService:  workspaceService,
        overviewService:   overviewService,
        permissionService: permissionService,  // 新增
    }
}
```

#### 修改 2：在 CreateWorkspace 方法中自动授权

修改 `backend/controllers/workspace_controller.go` 的 `CreateWorkspace` 方法：

```go
func (wc *WorkspaceController) CreateWorkspace(c *gin.Context) {
    // ... 原有创建逻辑 ...
    
    if err := wc.workspaceService.CreateWorkspace(workspace); err != nil {
        // ... 错误处理 ...
    }
    
    // 新增：为创建者授予 WORKSPACE_MANAGEMENT 的 ADMIN 权限
    userID, exists := c.Get("user_id")
    if exists && userID != nil {
        // 授予 WORKSPACE_MANAGEMENT 权限
        grantReq := &service.GrantPermissionRequest{
            ScopeType:       valueobject.ScopeTypeWorkspace,
            ScopeID:         workspace.ID,  // 使用数字 ID
            PrincipalType:   valueobject.PrincipalTypeUser,
            PrincipalID:     userID.(string),
            PermissionID:    "WORKSPACE_MANAGEMENT",
            PermissionLevel: valueobject.PermissionLevelAdmin,
            GrantedBy:       userID.(string),
            Reason:          "Auto-granted to workspace creator",
        }
        
        if err := wc.permissionService.GrantPermission(c.Request.Context(), grantReq); err != nil {
            // 记录日志但不阻止创建
            log.Printf("Warning: Failed to auto-grant permission to workspace creator: %v", err)
        }
    }
    
    // ... 返回响应 ...
}
```

#### 修改 3：更新路由初始化

修改 `backend/internal/router/router_workspace.go`：

```go
func setupWorkspaceRoutes(api *gin.RouterGroup, db *gorm.DB, ..., permissionService service.PermissionService) {
    workspaceController := controllers.NewWorkspaceController(
        services.NewWorkspaceService(db),
        services.NewWorkspaceOverviewService(db),
        permissionService,  // 新增
    )
    // ...
}
```

### 需要授予的权限列表

创建 workspace 时，应该为创建者授予以下权限（ADMIN 级别）：

1. `WORKSPACE_MANAGEMENT` - 工作空间管理
2. `WORKSPACE_EXECUTION` - 执行 Plan/Apply
3. `WORKSPACE_VARIABLES` - 变量管理
4. `WORKSPACE_STATE` - 状态管理
5. `WORKSPACE_RESOURCES` - 资源管理
6. `TASK_DATA_ACCESS` - 任务数据访问

或者使用预设权限集 `ADMIN`，一次性授予所有权限。

## 涉及的文件

1. `backend/internal/middleware/middleware.go` - JWT 中间件
2. `backend/internal/middleware/iam_permission.go` - IAM 权限中间件
3. `backend/internal/application/service/permission_checker.go` - 权限检查器
4. `backend/controllers/workspace_controller.go` - Workspace 控制器
5. `backend/services/workspace_service.go` - Workspace 服务

## 测试建议

1. 创建一个 `is_system_admin = true` 的用户
2. 使用该用户创建新的 workspace
3. 验证该用户可以：
   - 查看 workspace 详情
   - 创建 Plan 任务
   - 执行 Apply
   - 管理变量
   - 查看资源
4. 验证普通用户（非 admin）仍然需要授权才能访问

## 数据库检查

确认 `is_system_admin` 字段存在：
```sql
SELECT column_name, data_type, column_default 
FROM information_schema.columns 
WHERE table_name = 'users' AND column_name = 'is_system_admin';
```

设置超级管理员：
```sql
UPDATE users SET is_system_admin = true WHERE user_id = 'your-admin-user-id';
