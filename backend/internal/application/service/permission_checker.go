package service

import (
	"context"
	"fmt"
	"time"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/repository"
	"iac-platform/internal/domain/valueobject"
)

// CheckPermissionRequest 权限检查请求
type CheckPermissionRequest struct {
	UserID        string                      `json:"user_id"`
	ResourceType  valueobject.ResourceType    `json:"resource_type"`
	ScopeType     valueobject.ScopeType       `json:"scope_type"`
	ScopeID       uint                        `json:"scope_id"`     // 保留用于向后兼容
	ScopeIDStr    string                      `json:"scope_id_str"` // 新增：支持语义化ID
	RequiredLevel valueobject.PermissionLevel `json:"required_level"`
}

// CheckPermissionResult 权限检查结果
type CheckPermissionResult struct {
	IsAllowed      bool                        `json:"is_allowed"`
	EffectiveLevel valueobject.PermissionLevel `json:"effective_level"`
	Grants         []*entity.PermissionGrant   `json:"grants,omitempty"`
	DenyReason     string                      `json:"deny_reason,omitempty"`
	Source         string                      `json:"source"` // regular/temporary
	CacheHit       bool                        `json:"cache_hit"`
}

// ScopeInfo 作用域层级信息
type ScopeInfo struct {
	OrgID       uint
	ProjectID   uint
	WorkspaceID uint
}

// PermissionChecker 权限检查器接口
type PermissionChecker interface {
	// CheckPermission 检查用户是否拥有指定权限
	CheckPermission(ctx context.Context, req *CheckPermissionRequest) (*CheckPermissionResult, error)

	// CheckPermissionWithTemporary 检查权限（包含临时权限）
	CheckPermissionWithTemporary(ctx context.Context, req *CheckPermissionRequest, taskID *uint) (*CheckPermissionResult, error)

	// CheckBatchPermissions 批量检查权限
	CheckBatchPermissions(ctx context.Context, reqs []*CheckPermissionRequest) ([]*CheckPermissionResult, error)

	// GetUserTeams 获取用户所属团队
	GetUserTeams(ctx context.Context, userID string) ([]string, error)
}

// PermissionCheckerImpl 权限检查器实现
type PermissionCheckerImpl struct {
	permissionRepo repository.PermissionRepository
	teamRepo       repository.TeamRepository
	orgRepo        repository.OrganizationRepository
	projectRepo    repository.ProjectRepository
	auditRepo      repository.AuditRepository
	// cache          cache.PermissionCache // TODO: 实现缓存
}

// NewPermissionChecker 创建权限检查器实例
func NewPermissionChecker(
	permissionRepo repository.PermissionRepository,
	teamRepo repository.TeamRepository,
	orgRepo repository.OrganizationRepository,
	projectRepo repository.ProjectRepository,
	auditRepo repository.AuditRepository,
) PermissionChecker {
	return &PermissionCheckerImpl{
		permissionRepo: permissionRepo,
		teamRepo:       teamRepo,
		orgRepo:        orgRepo,
		projectRepo:    projectRepo,
		auditRepo:      auditRepo,
	}
}

// CheckPermission 检查权限
func (c *PermissionCheckerImpl) CheckPermission(
	ctx context.Context,
	req *CheckPermissionRequest,
) (*CheckPermissionResult, error) {
	// 0. 如果提供了 ScopeIDStr，需要转换为数字 ID
	if req.ScopeIDStr != "" && req.ScopeID == 0 {
		// 尝试将 ScopeIDStr 解析为数字
		if numID, err := parseUint(req.ScopeIDStr); err == nil {
			req.ScopeID = numID
		} else if req.ScopeType == valueobject.ScopeTypeWorkspace {
			// 如果是 workspace 且不是数字，通过语义化 ID 查询数字 ID
			workspaceID, err := c.projectRepo.GetWorkspaceIDBySemanticID(ctx, req.ScopeIDStr)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve workspace_id '%s': %w", req.ScopeIDStr, err)
			}
			req.ScopeID = workspaceID
		} else {
			return nil, fmt.Errorf("invalid scope_id format: %s", req.ScopeIDStr)
		}
	}

	// 1. 验证请求参数
	if err := c.validateRequest(req); err != nil {
		return nil, err
	}

	// 2. 获取用户所属团队
	userTeams, err := c.GetUserTeams(ctx, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user teams: %w", err)
	}

	// 3. 获取作用域层级信息
	scopeInfo, err := c.getScopeInfo(ctx, req.ScopeType, req.ScopeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get scope info: %w", err)
	}

	// 4. 收集所有权限授予（包括直接授权、团队授权和角色授权）
	allGrants, err := c.collectAllGrants(ctx, req, userTeams, scopeInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to collect grants: %w", err)
	}

	// 5. 计算有效权限等级
	effectiveLevel := c.calculateEffectiveLevel(allGrants)

	// 6. 判断是否允许访问
	isAllowed := effectiveLevel >= req.RequiredLevel && effectiveLevel != valueobject.PermissionLevelNone
	denyReason := ""
	if !isAllowed {
		denyReason = c.getDenyReason(effectiveLevel, req.RequiredLevel)
	}

	result := &CheckPermissionResult{
		IsAllowed:      isAllowed,
		EffectiveLevel: effectiveLevel,
		Grants:         allGrants,
		DenyReason:     denyReason,
		Source:         "regular",
		CacheHit:       false,
	}

	// 注意：不在这里记录访问日志，由 audit_logger 中间件统一记录
	// 这样可以确保记录完整的HTTP请求信息（路径、状态码、IP等）
	// go c.logAccess(context.Background(), req, result, time.Since(startTime))

	return result, nil
}

// CheckPermissionWithTemporary 检查权限（包含临时权限）
func (c *PermissionCheckerImpl) CheckPermissionWithTemporary(
	ctx context.Context,
	req *CheckPermissionRequest,
	taskID *uint,
) (*CheckPermissionResult, error) {
	// 1. 检查常规权限
	regularResult, err := c.CheckPermission(ctx, req)
	if err != nil {
		return nil, err
	}

	// 2. 如果常规权限已允许，直接返回
	if regularResult.IsAllowed {
		return regularResult, nil
	}

	// 3. 如果常规权限拒绝，检查是否有临时权限
	if taskID != nil {
		// TODO: 获取用户邮箱
		userEmail := fmt.Sprintf("user_%s@example.com", req.UserID)

		hasTemp, err := c.checkTemporaryPermission(ctx, *taskID, userEmail, req.ResourceType)
		if err != nil {
			return nil, fmt.Errorf("failed to check temporary permission: %w", err)
		}

		if hasTemp {
			// 临时权限允许
			return &CheckPermissionResult{
				IsAllowed:      true,
				EffectiveLevel: req.RequiredLevel,
				Source:         "temporary",
				CacheHit:       false,
			}, nil
		}
	}

	// 4. 两种权限都不满足，返回拒绝
	return regularResult, nil
}

// CheckBatchPermissions 批量检查权限
func (c *PermissionCheckerImpl) CheckBatchPermissions(
	ctx context.Context,
	reqs []*CheckPermissionRequest,
) ([]*CheckPermissionResult, error) {
	results := make([]*CheckPermissionResult, len(reqs))

	// 简单实现：逐个检查
	// TODO: 优化为批量查询
	for i, req := range reqs {
		result, err := c.CheckPermission(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("failed to check permission at index %d: %w", i, err)
		}
		results[i] = result
	}

	return results, nil
}

// GetUserTeams 获取用户所属团队
func (c *PermissionCheckerImpl) GetUserTeams(ctx context.Context, userID string) ([]string, error) {
	return c.teamRepo.GetUserTeams(ctx, userID)
}

// collectAllGrants 收集所有权限授予（包括直接授权、团队授权和角色授权）
func (c *PermissionCheckerImpl) collectAllGrants(
	ctx context.Context,
	req *CheckPermissionRequest,
	userTeams []string,
	scopeInfo *ScopeInfo,
) ([]*entity.PermissionGrant, error) {
	var allGrants []*entity.PermissionGrant

	// 1. 收集 Organization 级权限
	if scopeInfo.OrgID > 0 {
		// 1.1 直接授权和团队授权
		orgGrants, err := c.collectOrgLevelGrants(ctx, req.UserID, userTeams, req.ResourceType, scopeInfo.OrgID)
		if err != nil {
			return nil, err
		}
		allGrants = append(allGrants, orgGrants...)

		// 1.2 角色授权
		roleGrants, err := c.collectRoleGrants(ctx, req.UserID, valueobject.ScopeTypeOrganization, scopeInfo.OrgID, req.ResourceType)
		if err != nil {
			return nil, err
		}
		allGrants = append(allGrants, roleGrants...)
	}

	// 2. 收集 Project 级权限
	if scopeInfo.ProjectID > 0 {
		// 2.1 直接授权和团队授权
		projGrants, err := c.collectProjectLevelGrants(ctx, req.UserID, userTeams, req.ResourceType, scopeInfo.ProjectID)
		if err != nil {
			return nil, err
		}
		allGrants = append(allGrants, projGrants...)

		// 2.2 角色授权
		roleGrants, err := c.collectRoleGrants(ctx, req.UserID, valueobject.ScopeTypeProject, scopeInfo.ProjectID, req.ResourceType)
		if err != nil {
			return nil, err
		}
		allGrants = append(allGrants, roleGrants...)
	}

	// 3. 收集 Workspace 级权限
	if req.ScopeType == valueobject.ScopeTypeWorkspace {
		// 3.1 直接授权和团队授权
		wsGrants, err := c.collectWorkspaceLevelGrants(ctx, req.UserID, userTeams, req.ResourceType, req.ScopeID)
		if err != nil {
			return nil, err
		}
		allGrants = append(allGrants, wsGrants...)

		// 3.2 角色授权
		roleGrants, err := c.collectRoleGrants(ctx, req.UserID, valueobject.ScopeTypeWorkspace, req.ScopeID, req.ResourceType)
		if err != nil {
			return nil, err
		}
		allGrants = append(allGrants, roleGrants...)
	}

	return allGrants, nil
}

// collectOrgLevelGrants 收集组织级权限
func (c *PermissionCheckerImpl) collectOrgLevelGrants(
	ctx context.Context,
	userID string,
	userTeams []string,
	resourceType valueobject.ResourceType,
	orgID uint,
) ([]*entity.PermissionGrant, error) {
	var grants []*entity.PermissionGrant

	// 用户直接授权
	userPerms, err := c.permissionRepo.QueryOrgPermissions(
		ctx, orgID, valueobject.PrincipalTypeUser, []string{userID}, resourceType,
	)
	if err != nil {
		return nil, err
	}
	for _, perm := range userPerms {
		grants = append(grants, perm.ToPermissionGrant())
	}

	// 团队授权
	if len(userTeams) > 0 {
		teamPerms, err := c.permissionRepo.QueryOrgPermissions(
			ctx, orgID, valueobject.PrincipalTypeTeam, userTeams, resourceType,
		)
		if err != nil {
			return nil, err
		}
		for _, perm := range teamPerms {
			grant := perm.ToPermissionGrant()
			grant.Source = "team"
			grants = append(grants, grant)
		}
	}

	return grants, nil
}

// collectProjectLevelGrants 收集项目级权限
func (c *PermissionCheckerImpl) collectProjectLevelGrants(
	ctx context.Context,
	userID string,
	userTeams []string,
	resourceType valueobject.ResourceType,
	projectID uint,
) ([]*entity.PermissionGrant, error) {
	var grants []*entity.PermissionGrant

	// 用户直接授权
	userPerms, err := c.permissionRepo.QueryProjectPermissions(
		ctx, projectID, valueobject.PrincipalTypeUser, []string{userID}, resourceType,
	)
	if err != nil {
		return nil, err
	}
	for _, perm := range userPerms {
		grants = append(grants, perm.ToPermissionGrant())
	}

	// 团队授权
	if len(userTeams) > 0 {
		teamPerms, err := c.permissionRepo.QueryProjectPermissions(
			ctx, projectID, valueobject.PrincipalTypeTeam, userTeams, resourceType,
		)
		if err != nil {
			return nil, err
		}
		for _, perm := range teamPerms {
			grant := perm.ToPermissionGrant()
			grant.Source = "team"
			grants = append(grants, grant)
		}
	}

	return grants, nil
}

// collectWorkspaceLevelGrants 收集工作空间级权限
func (c *PermissionCheckerImpl) collectWorkspaceLevelGrants(
	ctx context.Context,
	userID string,
	userTeams []string,
	resourceType valueobject.ResourceType,
	workspaceID uint,
) ([]*entity.PermissionGrant, error) {
	var grants []*entity.PermissionGrant

	// 将数字 ID 转换为语义化 ID
	var workspace struct {
		WorkspaceID string `gorm:"column:workspace_id"`
	}
	if err := c.projectRepo.GetDB().Table("workspaces").
		Select("workspace_id").
		Where("id = ?", workspaceID).
		First(&workspace).Error; err != nil {
		fmt.Printf("[DEBUG] Failed to get workspace_id for id=%d: %v\n", workspaceID, err)
		return nil, fmt.Errorf("failed to get workspace_id: %w", err)
	}

	fmt.Printf("[DEBUG] collectWorkspaceLevelGrants: workspaceID=%d, workspace_id=%s, userID=%s, userTeams=%v, resourceType=%s\n",
		workspaceID, workspace.WorkspaceID, userID, userTeams, resourceType)

	// 用户直接授权
	userPerms, err := c.permissionRepo.QueryWorkspacePermissions(
		ctx, workspace.WorkspaceID, valueobject.PrincipalTypeUser, []string{userID}, resourceType,
	)
	if err != nil {
		return nil, err
	}
	fmt.Printf("[DEBUG] User permissions found: %d\n", len(userPerms))
	for _, perm := range userPerms {
		grants = append(grants, perm.ToPermissionGrant())
	}

	// 团队授权
	if len(userTeams) > 0 {
		teamPerms, err := c.permissionRepo.QueryWorkspacePermissions(
			ctx, workspace.WorkspaceID, valueobject.PrincipalTypeTeam, userTeams, resourceType,
		)
		if err != nil {
			return nil, err
		}
		fmt.Printf("[DEBUG] Team permissions found: %d\n", len(teamPerms))
		for _, perm := range teamPerms {
			grant := perm.ToPermissionGrant()
			grant.Source = "team"
			grants = append(grants, grant)
		}
	}

	fmt.Printf("[DEBUG] Total workspace grants collected: %d\n", len(grants))
	return grants, nil
}

// calculateEffectiveLevel 计算有效权限等级
// 规则：
// 1. 如果没有任何有效授权,返回NONE(表示没有权限,不是拒绝)
// 2. 从所有有效授权中取最高权限级别(忽略NONE级别的授权)
// 3. 这样可以确保组织级的WRITE权限不会被角色的READ权限覆盖
// 注意: 当前系统中NONE表示"没有权限",不是"显式拒绝"
func (c *PermissionCheckerImpl) calculateEffectiveLevel(
	grants []*entity.PermissionGrant,
) valueobject.PermissionLevel {
	// 步骤1: 过滤过期权限
	validGrants := c.filterExpiredGrants(grants)

	// 步骤2: 如果没有任何有效授权,返回NONE(没有权限)
	if len(validGrants) == 0 {
		return valueobject.PermissionLevelNone
	}

	// 步骤3: 从所有有效授权中取最高权限级别
	// 注意: 只考虑大于NONE的权限级别
	maxLevel := valueobject.PermissionLevelNone
	for _, grant := range validGrants {
		// 忽略NONE级别的授权(如果存在的话)
		if grant.PermissionLevel > valueobject.PermissionLevelNone && grant.PermissionLevel > maxLevel {
			maxLevel = grant.PermissionLevel
		}
	}

	return maxLevel
}

// getScopeInfo 获取作用域层级信息
func (c *PermissionCheckerImpl) getScopeInfo(
	ctx context.Context,
	scopeType valueobject.ScopeType,
	scopeID uint,
) (*ScopeInfo, error) {
	info := &ScopeInfo{}

	switch scopeType {
	case valueobject.ScopeTypeOrganization:
		info.OrgID = scopeID

	case valueobject.ScopeTypeProject:
		// 获取组织ID
		orgID, err := c.projectRepo.GetOrgIDByProjectID(ctx, scopeID)
		if err != nil {
			return nil, err
		}
		info.OrgID = orgID
		info.ProjectID = scopeID

	case valueobject.ScopeTypeWorkspace:
		// 获取项目ID
		projectID, err := c.projectRepo.GetProjectIDByWorkspaceID(ctx, scopeID)
		if err != nil {
			return nil, err
		}
		info.ProjectID = projectID

		// 获取组织ID
		orgID, err := c.projectRepo.GetOrgIDByProjectID(ctx, projectID)
		if err != nil {
			return nil, err
		}
		info.OrgID = orgID
		info.WorkspaceID = scopeID
	}

	return info, nil
}

// filterByScope 按作用域过滤权限
func (c *PermissionCheckerImpl) filterByScope(
	grants []*entity.PermissionGrant,
	scopeType valueobject.ScopeType,
) []*entity.PermissionGrant {
	var filtered []*entity.PermissionGrant
	for _, grant := range grants {
		if grant.ScopeType == scopeType {
			filtered = append(filtered, grant)
		}
	}
	return filtered
}

// maxLevel 获取权限列表中的最高等级
func (c *PermissionCheckerImpl) maxLevel(
	grants []*entity.PermissionGrant,
) valueobject.PermissionLevel {
	maxLevel := valueobject.PermissionLevelNone
	for _, grant := range grants {
		if grant.PermissionLevel > maxLevel {
			maxLevel = grant.PermissionLevel
		}
	}
	return maxLevel
}

// filterExpiredGrants 过滤过期权限
func (c *PermissionCheckerImpl) filterExpiredGrants(
	grants []*entity.PermissionGrant,
) []*entity.PermissionGrant {
	var valid []*entity.PermissionGrant
	for _, grant := range grants {
		if grant.IsValid() {
			valid = append(valid, grant)
		}
	}
	return valid
}

// checkTemporaryPermission 检查临时权限
func (c *PermissionCheckerImpl) checkTemporaryPermission(
	ctx context.Context,
	taskID uint,
	userEmail string,
	resourceType valueobject.ResourceType,
) (bool, error) {
	// 映射资源类型到临时权限类型
	permType := c.mapResourceToPermType(resourceType)
	if permType == "" {
		return false, nil
	}

	tempPerm, err := c.permissionRepo.CheckTemporaryPermission(ctx, taskID, userEmail, permType)
	if err != nil {
		return false, err
	}

	if tempPerm != nil && tempPerm.IsValid() {
		// 标记为已使用
		_ = c.permissionRepo.MarkTemporaryPermissionUsed(ctx, tempPerm.ID)
		return true, nil
	}

	return false, nil
}

// mapResourceToPermType 映射资源类型到临时权限类型
func (c *PermissionCheckerImpl) mapResourceToPermType(resourceType valueobject.ResourceType) string {
	switch resourceType {
	case valueobject.ResourceTypeWorkspaceExec:
		return "APPLY"
	default:
		return ""
	}
}

// validateRequest 验证请求参数
func (c *PermissionCheckerImpl) validateRequest(req *CheckPermissionRequest) error {
	if req.UserID == "" {
		return fmt.Errorf("user_id is required")
	}
	if !req.ResourceType.IsValid() {
		return fmt.Errorf("invalid resource_type: %s", req.ResourceType)
	}
	if !req.ScopeType.IsValid() {
		return fmt.Errorf("invalid scope_type: %s", req.ScopeType)
	}
	if req.ScopeID == 0 {
		return fmt.Errorf("scope_id is required")
	}
	if !req.RequiredLevel.IsValid() {
		return fmt.Errorf("invalid required_level: %d", req.RequiredLevel)
	}
	return nil
}

// getDenyReason 获取拒绝原因
func (c *PermissionCheckerImpl) getDenyReason(
	effectiveLevel valueobject.PermissionLevel,
	requiredLevel valueobject.PermissionLevel,
) string {
	if effectiveLevel == valueobject.PermissionLevelNone {
		return "Access explicitly denied"
	}
	if effectiveLevel < requiredLevel {
		return fmt.Sprintf("Insufficient permission level: have %s, need %s",
			effectiveLevel.String(), requiredLevel.String())
	}
	return ""
}

// collectRoleGrants 收集角色授予的权限（包括用户角色和团队角色）
// 注意：会查询当前作用域及所有父作用域的角色
func (c *PermissionCheckerImpl) collectRoleGrants(
	ctx context.Context,
	userID string,
	scopeType valueobject.ScopeType,
	scopeID uint,
	resourceType valueobject.ResourceType,
) ([]*entity.PermissionGrant, error) {
	var grants []*entity.PermissionGrant

	// 获取用户所属团队
	userTeams, err := c.GetUserTeams(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user teams: %w", err)
	}

	// 收集所有需要查询的作用域
	scopesToCheck := []struct {
		scopeType valueobject.ScopeType
		scopeID   uint
	}{
		{scopeType, scopeID}, // 当前作用域
	}

	// 如果是WORKSPACE作用域，还需要查询PROJECT和ORGANIZATION作用域的角色
	if scopeType == valueobject.ScopeTypeWorkspace {
		// 获取作用域信息
		scopeInfo, err := c.getScopeInfo(ctx, scopeType, scopeID)
		if err == nil {
			if scopeInfo.ProjectID > 0 {
				scopesToCheck = append(scopesToCheck, struct {
					scopeType valueobject.ScopeType
					scopeID   uint
				}{valueobject.ScopeTypeProject, scopeInfo.ProjectID})
			}
			if scopeInfo.OrgID > 0 {
				scopesToCheck = append(scopesToCheck, struct {
					scopeType valueobject.ScopeType
					scopeID   uint
				}{valueobject.ScopeTypeOrganization, scopeInfo.OrgID})
			}
		}
	} else if scopeType == valueobject.ScopeTypeProject {
		// 如果是PROJECT作用域，还需要查询ORGANIZATION作用域的角色
		scopeInfo, err := c.getScopeInfo(ctx, scopeType, scopeID)
		if err == nil && scopeInfo.OrgID > 0 {
			scopesToCheck = append(scopesToCheck, struct {
				scopeType valueobject.ScopeType
				scopeID   uint
			}{valueobject.ScopeTypeOrganization, scopeInfo.OrgID})
		}
	}

	// 对每个作用域查询角色
	for _, scope := range scopesToCheck {
		// 1. 查询用户在该作用域的角色
		userRoles, err := c.permissionRepo.QueryUserRoles(ctx, userID, scope.scopeType, scope.scopeID)
		if err != nil {
			return nil, fmt.Errorf("failed to query user roles: %w", err)
		}

		// 2. 查询用户所属团队在该作用域的角色
		var teamRoles []*entity.UserRole
		if len(userTeams) > 0 {
			teamRoles, err = c.permissionRepo.QueryTeamRoles(ctx, userTeams, scope.scopeType, scope.scopeID)
			if err != nil {
				return nil, fmt.Errorf("failed to query team roles: %w", err)
			}
		}

		// 合并用户角色和团队角色
		allRoles := append(userRoles, teamRoles...)

		// 3. 对每个角色，查询其包含的权限策略
		for _, role := range allRoles {
			// 检查角色是否有效（未过期）
			if !role.IsValid() {
				continue
			}

			// 查询角色的所有策略（不限制scope_type）
			// 需要查询所有可能的scope_type，因为角色可以包含不同作用域的策略
			allScopeTypes := []valueobject.ScopeType{
				valueobject.ScopeTypeOrganization,
				valueobject.ScopeTypeProject,
				valueobject.ScopeTypeWorkspace,
			}

			for _, policyScopeType := range allScopeTypes {
				policies, err := c.permissionRepo.QueryRolePolicies(ctx, role.RoleID, policyScopeType)
				if err != nil {
					continue // 跳过查询失败的作用域
				}

				// 4. 将角色策略转换为PermissionGrant
				for _, policy := range policies {
					// 只收集匹配resourceType的策略
					if valueobject.ResourceType(policy.ResourceType) == resourceType {
						// 解析权限级别
						permLevel, err := valueobject.ParsePermissionLevel(policy.PermissionLevel)
						if err != nil {
							continue
						}

						// 解析策略的作用域类型
						policyScope, err := valueobject.ParseScopeType(policy.ScopeType)
						if err != nil {
							continue
						}

						grant := &entity.PermissionGrant{
							ScopeType:       policyScope,
							ScopeID:         scope.scopeID,
							PrincipalType:   valueobject.PrincipalTypeUser,
							PrincipalID:     userID,
							PermissionID:    policy.PermissionID,
							PermissionLevel: permLevel,
							GrantedAt:       role.AssignedAt,
							ExpiresAt:       role.ExpiresAt,
							Source:          fmt.Sprintf("role:%s@%s", role.RoleName, scope.scopeType),
						}
						grants = append(grants, grant)
					}
				}
			}
		}
	}

	return grants, nil
}

// parseUint 解析字符串为 uint
func parseUint(s string) (uint, error) {
	val, err := fmt.Sscanf(s, "%d", new(uint))
	if err != nil || val != 1 {
		return 0, fmt.Errorf("invalid uint format")
	}
	var result uint
	fmt.Sscanf(s, "%d", &result)
	return result, nil
}

// logAccess 记录访问日志
// 注意：这个函数主要用于权限检查的审计，不包含HTTP请求的详细信息
// HTTP请求的详细审计由 audit_logger 中间件负责
func (c *PermissionCheckerImpl) logAccess(
	ctx context.Context,
	req *CheckPermissionRequest,
	result *CheckPermissionResult,
	duration time.Duration,
) {
	log := &entity.AccessLog{
		UserID:         req.UserID,
		ResourceType:   string(req.ResourceType),
		ResourceID:     req.ScopeID,
		Action:         req.RequiredLevel.String(),
		IsAllowed:      result.IsAllowed,
		DenyReason:     result.DenyReason,
		EffectiveLevel: &result.EffectiveLevel,
		AccessedAt:     time.Now(),
		DurationMs:     int(duration.Milliseconds()),
		// 以下字段由中间件填充，这里设置默认值
		RequestPath:    "",
		HttpCode:       0,
		IPAddress:      "0.0.0.0",
		UserAgent:      "",
		RequestHeaders: "null",
		RequestBody:    "",
	}

	_ = c.auditRepo.LogResourceAccess(ctx, log)
}
