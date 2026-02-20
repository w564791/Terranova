# IAM Role系统实施计划

## 当前状态

 **已完成**：
- 数据库schema创建（iam_roles, iam_role_policies, iam_user_roles）
- 6个系统预定义角色创建
- admin用户自动分配超级管理员角色
- 完整的使用文档

## 下一步实施计划

### 优先级1：更新权限检查逻辑（HIGH）

#### 1.1 添加Repository方法

在 `backend/internal/domain/repository/permission_repository.go` 添加：

```go
// QueryUserRoles 查询用户的角色分配
QueryUserRoles(
    ctx context.Context,
    userID uint,
    scopeType valueobject.ScopeType,
    scopeID uint,
) ([]*entity.UserRole, error)

// QueryRolePolicies 查询角色包含的权限策略
QueryRolePolicies(
    ctx context.Context,
    roleID uint,
    scopeType valueobject.ScopeType,
) ([]*entity.RolePolicy, error)
```

#### 1.2 实现Repository方法

在 `backend/internal/infrastructure/persistence/permission_repository_impl.go` 实现上述方法：

```go
func (r *PermissionRepositoryImpl) QueryUserRoles(
    ctx context.Context,
    userID uint,
    scopeType valueobject.ScopeType,
    scopeID uint,
) ([]*entity.UserRole, error) {
    var roles []*entity.UserRole
    err := r.db.WithContext(ctx).
        Where("user_id = ? AND scope_type = ? AND scope_id = ?", userID, scopeType, scopeID).
        Where("expires_at IS NULL OR expires_at > ?", time.Now()).
        Find(&roles).Error
    return roles, err
}

func (r *PermissionRepositoryImpl) QueryRolePolicies(
    ctx context.Context,
    roleID uint,
    scopeType valueobject.ScopeType,
) ([]*entity.RolePolicy, error) {
    var policies []*entity.RolePolicy
    err := r.db.WithContext(ctx).
        Where("role_id = ? AND scope_type = ?", roleID, scopeType).
        Find(&policies).Error
    return policies, err
}
```

#### 1.3 更新Permission Checker

在 `backend/internal/application/service/permission_checker.go` 的 `collectAllGrants` 方法中添加角色权限收集：

```go
func (c *PermissionCheckerImpl) collectAllGrants(
    ctx context.Context,
    req *CheckPermissionRequest,
    userTeams []uint,
    scopeInfo *ScopeInfo,
) ([]*entity.PermissionGrant, error) {
    var allGrants []*entity.PermissionGrant

    // 1. 收集 Organization 级权限（直接授权）
    if scopeInfo.OrgID > 0 {
        orgGrants, err := c.collectOrgLevelGrants(ctx, req.UserID, userTeams, req.ResourceType, scopeInfo.OrgID)
        if err != nil {
            return nil, err
        }
        allGrants = append(allGrants, orgGrants...)
        
        // 1.1 收集 Organization 级权限（角色授权）
        roleGrants, err := c.collectRoleGrants(ctx, req.UserID, valueobject.ScopeTypeOrganization, scopeInfo.OrgID, req.ResourceType)
        if err != nil {
            return nil, err
        }
        allGrants = append(allGrants, roleGrants...)
    }

    // 2. 收集 Project 级权限（直接授权）
    if scopeInfo.ProjectID > 0 {
        projGrants, err := c.collectProjectLevelGrants(ctx, req.UserID, userTeams, req.ResourceType, scopeInfo.ProjectID)
        if err != nil {
            return nil, err
        }
        allGrants = append(allGrants, projGrants...)
        
        // 2.1 收集 Project 级权限（角色授权）
        roleGrants, err := c.collectRoleGrants(ctx, req.UserID, valueobject.ScopeTypeProject, scopeInfo.ProjectID, req.ResourceType)
        if err != nil {
            return nil, err
        }
        allGrants = append(allGrants, roleGrants...)
    }

    // 3. 收集 Workspace 级权限（直接授权）
    if req.ScopeType == valueobject.ScopeTypeWorkspace {
        wsGrants, err := c.collectWorkspaceLevelGrants(ctx, req.UserID, userTeams, req.ResourceType, req.ScopeID)
        if err != nil {
            return nil, err
        }
        allGrants = append(allGrants, wsGrants...)
        
        // 3.1 收集 Workspace 级权限（角色授权）
        roleGrants, err := c.collectRoleGrants(ctx, req.UserID, valueobject.ScopeTypeWorkspace, req.ScopeID, req.ResourceType)
        if err != nil {
            return nil, err
        }
        allGrants = append(allGrants, roleGrants...)
    }

    return allGrants, nil
}

// collectRoleGrants 收集角色授予的权限
func (c *PermissionCheckerImpl) collectRoleGrants(
    ctx context.Context,
    userID uint,
    scopeType valueobject.ScopeType,
    scopeID uint,
    resourceType valueobject.ResourceType,
) ([]*entity.PermissionGrant, error) {
    var grants []*entity.PermissionGrant

    // 1. 查询用户在该作用域的角色
    userRoles, err := c.permissionRepo.QueryUserRoles(ctx, userID, scopeType, scopeID)
    if err != nil {
        return nil, err
    }

    // 2. 对每个角色，查询其包含的权限策略
    for _, userRole := range userRoles {
        policies, err := c.permissionRepo.QueryRolePolicies(ctx, userRole.RoleID, scopeType)
        if err != nil {
            return nil, err
        }

        // 3. 将角色策略转换为PermissionGrant
        for _, policy := range policies {
            // 只收集匹配resourceType的策略
            if policy.ResourceType == resourceType {
                grant := &entity.PermissionGrant{
                    ScopeType:       scopeType,
                    ScopeID:         scopeID,
                    PrincipalType:   valueobject.PrincipalTypeUser,
                    PrincipalID:     userID,
                    PermissionID:    policy.PermissionID,
                    PermissionLevel: policy.PermissionLevel,
                    GrantedAt:       userRole.AssignedAt,
                    ExpiresAt:       userRole.ExpiresAt,
                    Source:          fmt.Sprintf("role:%s", userRole.RoleName),
                }
                grants = append(grants, grant)
            }
        }
    }

    return grants, nil
}
```

#### 1.4 添加Entity定义

在 `backend/internal/domain/entity/` 添加：

```go
// user_role.go
type UserRole struct {
    ID         uint
    UserID     uint
    RoleID     uint
    RoleName   string // 用于显示
    ScopeType  valueobject.ScopeType
    ScopeID    uint
    AssignedBy *uint
    AssignedAt time.Time
    ExpiresAt  *time.Time
    Reason     string
}

// role_policy.go
type RolePolicy struct {
    ID              uint
    RoleID          uint
    PermissionID    uint
    ResourceType    valueobject.ResourceType
    PermissionLevel valueobject.PermissionLevel
    ScopeType       valueobject.ScopeType
}
```

### 优先级2：创建后端API（MEDIUM）

#### 2.1 角色管理API

创建 `backend/internal/handlers/role_handler.go`：

```go
// ListRoles 列出所有角色
// @Summary 列出所有角色
// @Tags IAM-Roles
// @Produce json
// @Success 200 {object} ListRolesResponse
// @Router /api/iam/roles [get]
func (h *RoleHandler) ListRoles(c *gin.Context)

// GetRole 获取角色详情
// @Summary 获取角色详情
// @Tags IAM-Roles
// @Param id path int true "角色ID"
// @Produce json
// @Success 200 {object} RoleResponse
// @Router /api/iam/roles/{id} [get]
func (h *RoleHandler) GetRole(c *gin.Context)

// CreateRole 创建自定义角色
// @Summary 创建自定义角色
// @Tags IAM-Roles
// @Accept json
// @Produce json
// @Param request body CreateRoleRequest true "角色信息"
// @Success 201 {object} RoleResponse
// @Router /api/iam/roles [post]
func (h *RoleHandler) CreateRole(c *gin.Context)

// UpdateRole 更新角色
// @Summary 更新角色
// @Tags IAM-Roles
// @Param id path int true "角色ID"
// @Accept json
// @Produce json
// @Param request body UpdateRoleRequest true "角色信息"
// @Success 200 {object} RoleResponse
// @Router /api/iam/roles/{id} [put]
func (h *RoleHandler) UpdateRole(c *gin.Context)

// DeleteRole 删除角色（仅非系统角色）
// @Summary 删除角色
// @Tags IAM-Roles
// @Param id path int true "角色ID"
// @Success 204
// @Router /api/iam/roles/{id} [delete]
func (h *RoleHandler) DeleteRole(c *gin.Context)
```

#### 2.2 角色分配API

```go
// AssignRole 为用户分配角色
// @Summary 为用户分配角色
// @Tags IAM-Roles
// @Accept json
// @Produce json
// @Param request body AssignRoleRequest true "分配信息"
// @Success 200 {object} AssignRoleResponse
// @Router /api/iam/users/{user_id}/roles [post]
func (h *RoleHandler) AssignRole(c *gin.Context)

// RevokeRole 撤销用户角色
// @Summary 撤销用户角色
// @Tags IAM-Roles
// @Param user_id path int true "用户ID"
// @Param role_id path int true "角色ID"
// @Success 204
// @Router /api/iam/users/{user_id}/roles/{role_id} [delete]
func (h *RoleHandler) RevokeRole(c *gin.Context)

// ListUserRoles 列出用户的所有角色
// @Summary 列出用户的所有角色
// @Tags IAM-Roles
// @Param user_id path int true "用户ID"
// @Produce json
// @Success 200 {object} ListUserRolesResponse
// @Router /api/iam/users/{user_id}/roles [get]
func (h *RoleHandler) ListUserRoles(c *gin.Context)
```

#### 2.3 添加路由

在 `backend/internal/router/router.go` 添加：

```go
// IAM Role管理路由
iamGroup.GET("/roles", roleHandler.ListRoles)
iamGroup.GET("/roles/:id", roleHandler.GetRole)
iamGroup.POST("/roles", roleHandler.CreateRole)
iamGroup.PUT("/roles/:id", roleHandler.UpdateRole)
iamGroup.DELETE("/roles/:id", roleHandler.DeleteRole)

// 用户角色分配
iamGroup.POST("/users/:user_id/roles", roleHandler.AssignRole)
iamGroup.DELETE("/users/:user_id/roles/:role_id", roleHandler.RevokeRole)
iamGroup.GET("/users/:user_id/roles", roleHandler.ListUserRoles)
```

### 优先级3：前端界面（MEDIUM）

#### 3.1 角色管理页面

创建 `frontend/src/pages/admin/RoleManagement.tsx`：
- 列出所有角色
- 显示每个角色的权限策略数量
- 支持创建/编辑/删除自定义角色
- 查看角色详情和权限策略

#### 3.2 用户角色分配界面

在权限管理页面添加角色分配功能：
- 在用户卡片中显示用户的角色
- 支持为用户分配/撤销角色
- 显示角色来源的权限

#### 3.3 更新路由

在 `frontend/src/App.tsx` 添加：
```tsx
<Route path="admin/roles" element={<RoleManagement />} />
```

### 优先级4：移除Role Bypass（LOW）

在确认所有功能正常后，逐步移除：

1. `backend/internal/middleware/iam_permission.go` 中的 `BypassIAMForAdmin()`
2. `backend/internal/router/router.go` 中所有的 `if role == "admin"` 检查

## 测试计划

### 单元测试
- [ ] 测试角色权限收集逻辑
- [ ] 测试权限合并（直接授权 + 角色授权）
- [ ] 测试权限优先级（workspace > project > org）

### 集成测试
- [ ] 测试admin用户通过角色访问资源
- [ ] 测试普通用户分配角色后的权限
- [ ] 测试角色撤销后权限失效

### E2E测试
- [ ] 测试完整的角色分配流程
- [ ] 测试权限检查在实际API调用中的表现
- [ ] 测试前端角色管理界面

## 预期效果

完成后：
1.  admin用户通过IAM角色拥有完整权限
2.  可以快速为用户分配预定义角色
3.  可以创建自定义角色满足特定需求
4.  权限检查同时考虑直接授权和角色授权
5.  可以逐步移除role bypass逻辑

## 参考资料

- IAM Role使用指南：`docs/iam/iam-roles-guide.md`
- 数据库初始化脚本：`scripts/create_iam_roles.sql`
- 快速分配脚本：`scripts/assign_admin_role.sql`
