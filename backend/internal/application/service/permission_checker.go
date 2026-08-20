package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/repository"
	"iac-platform/internal/domain/valueobject"
)

// CheckPermissionRequest 权限检查请求
type CheckPermissionRequest struct {
	UserID        string                      `json:"user_id"`        // USER 主体时使用；TEAM 可为 team:<id> 兼容
	PrincipalType valueobject.PrincipalType   `json:"principal_type"` // 空则 USER；TEAM / APPLICATION 走专用路径
	PrincipalID   string                      `json:"principal_id"`   // 空则回落 UserID
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

// UserEmailLookup 解析 user_id → 邮箱（临时权限按邮箱匹配）
type UserEmailLookup interface {
	GetUserEmail(ctx context.Context, userID string) (string, error)
}

// PermissionCheckerImpl 权限检查器实现
type PermissionCheckerImpl struct {
	permissionRepo repository.PermissionRepository
	teamRepo       repository.TeamRepository
	orgRepo        repository.OrganizationRepository
	projectRepo    repository.ProjectRepository
	auditRepo      repository.AuditRepository
	userEmails     UserEmailLookup             // 可选；nil 时临时权限无法按真实邮箱匹配
	appAliases     ApplicationPrincipalAliases // 可选；APPLICATION principal_id 展开（app_key ↔ id）
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

// SetUserEmailLookup 注入用户邮箱解析（生产由 factory 设置）
// SetApplicationPrincipalAliases 注入 Application principal_id 展开（选项 A 兼容历史 grant）
func (c *PermissionCheckerImpl) SetApplicationPrincipalAliases(a ApplicationPrincipalAliases) {
	c.appAliases = a
}

func (c *PermissionCheckerImpl) SetUserEmailLookup(lookup UserEmailLookup) {
	if c != nil {
		c.userEmails = lookup
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

	// 1. 解析主体（USER / TEAM / APPLICATION）
	principalType, principalID, teamIDs, err := c.resolvePrincipal(ctx, req)
	if err != nil {
		return nil, err
	}

	// 2. 验证请求参数（主体已解析）
	if err := c.validateRequest(req, principalType, principalID); err != nil {
		return nil, err
	}

	// 3. 获取作用域层级信息
	scopeInfo, err := c.getScopeInfo(ctx, req.ScopeType, req.ScopeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get scope info: %w", err)
	}

	// 4. 收集所有权限授予（按主体类型，不伪造跨主体继承）
	allGrants, err := c.collectAllGrants(ctx, req, principalType, principalID, teamIDs, scopeInfo)
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

	return result, nil
}

// resolvePrincipal 解析鉴权主体。
// TEAM：仅使用该团队的 direct grant + team roles，不展开成员个人权限。
// USER：用户 direct + 所属团队 + user/team roles。
// APPLICATION：仅 Application 级 grant（组织级）。
func (c *PermissionCheckerImpl) resolvePrincipal(
	ctx context.Context,
	req *CheckPermissionRequest,
) (valueobject.PrincipalType, string, []string, error) {
	pt := req.PrincipalType
	if pt == "" {
		pt = valueobject.PrincipalTypeUser
	}
	if !pt.IsValid() {
		return "", "", nil, fmt.Errorf("invalid principal_type: %s", pt)
	}

	switch pt {
	case valueobject.PrincipalTypeUser:
		id := req.PrincipalID
		if id == "" {
			id = req.UserID
		}
		if id == "" {
			return "", "", nil, fmt.Errorf("user_id is required")
		}
		// 防止误把 team: 当作用户
		if len(id) > 5 && id[:5] == "team:" {
			return "", "", nil, fmt.Errorf("principal_type USER cannot use team: principal id; set PrincipalType=TEAM")
		}
		teams, err := c.GetUserTeams(ctx, id)
		if err != nil {
			return "", "", nil, fmt.Errorf("failed to get user teams: %w", err)
		}
		return pt, id, teams, nil

	case valueobject.PrincipalTypeTeam:
		id := req.PrincipalID
		if id == "" && len(req.UserID) > 5 && req.UserID[:5] == "team:" {
			id = req.UserID[5:]
		}
		if id == "" {
			id = req.UserID
		}
		if id == "" {
			return "", "", nil, fmt.Errorf("team principal_id is required")
		}
		return pt, id, []string{id}, nil

	case valueobject.PrincipalTypeApplication:
		id := req.PrincipalID
		if id == "" {
			id = req.UserID
		}
		if id == "" {
			return "", "", nil, fmt.Errorf("application principal_id is required")
		}
		return pt, id, nil, nil

	default:
		return "", "", nil, fmt.Errorf("unsupported principal_type: %s", pt)
	}
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

	// 3. 如果常规权限拒绝，检查是否有临时权限（email 与 user_id 双键）
	if taskID != nil {
		userEmail, emailErr := c.resolveUserEmail(ctx, req.UserID)
		if emailErr != nil {
			log.Printf("[IAM] temporary permission: email resolve failed for %s: %v (will try user_id)", req.UserID, emailErr)
			userEmail = ""
		}
		// req.UserID 作为语义化 user_id（非 app:/team: 前缀时）
		userID := req.UserID
		if len(userID) > 4 && (userID[:4] == "app:" || (len(userID) > 5 && userID[:5] == "team:")) {
			userID = ""
		}
		if userEmail != "" || userID != "" {
			hasTemp, err := c.checkTemporaryPermission(ctx, *taskID, userEmail, userID, req.ResourceType)
			if err != nil {
				return nil, fmt.Errorf("failed to check temporary permission: %w", err)
			}
			if hasTemp {
				return &CheckPermissionResult{
					IsAllowed:      true,
					EffectiveLevel: req.RequiredLevel,
					Source:         "temporary",
					CacheHit:       false,
				}, nil
			}
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
	principalType valueobject.PrincipalType,
	principalID string,
	teamIDs []string,
	scopeInfo *ScopeInfo,
) ([]*entity.PermissionGrant, error) {
	requestedGrants, err := c.collectGrantsForResourceType(
		ctx, req, principalType, principalID, teamIDs, scopeInfo, req.ResourceType,
	)
	if err != nil {
		return nil, err
	}

	// WORKSPACE_MANAGEMENT is an umbrella permission for ordinary workspace
	// capabilities. Collect it through the same target scope and ancestors so
	// an organization/project Role cannot escape its assignment subtree. A
	// fine-grained grant at a given layer remains authoritative at that layer;
	// MANAGEMENT is a fallback, not a way to raise an explicit narrower grant.
	if !req.ResourceType.IsSatisfiedBy(valueobject.ResourceTypeWorkspaceManagement) ||
		req.ResourceType == valueobject.ResourceTypeWorkspaceManagement {
		return requestedGrants, nil
	}

	managementGrants, err := c.collectGrantsForResourceType(
		ctx, req, principalType, principalID, teamIDs, scopeInfo, valueobject.ResourceTypeWorkspaceManagement,
	)
	if err != nil {
		return nil, err
	}

	return mergeWorkspaceManagementFallback(requestedGrants, managementGrants), nil
}

// collectGrantsForResourceType collects grants for one concrete resource type
// at the requested target scope and its ancestors. It deliberately does not
// apply resource implication itself, so callers can preserve the priority of
// an explicit fine-grained grant over the MANAGEMENT fallback at the same
// assignment layer.
func (c *PermissionCheckerImpl) collectGrantsForResourceType(
	ctx context.Context,
	req *CheckPermissionRequest,
	principalType valueobject.PrincipalType,
	principalID string,
	teamIDs []string,
	scopeInfo *ScopeInfo,
	resourceType valueobject.ResourceType,
) ([]*entity.PermissionGrant, error) {
	var allGrants []*entity.PermissionGrant

	// 1. 收集 Organization 级直接/团队/应用授权
	if scopeInfo.OrgID > 0 {
		orgGrants, err := c.collectOrgLevelGrants(ctx, principalType, principalID, teamIDs, resourceType, scopeInfo.OrgID)
		if err != nil {
			return nil, err
		}
		allGrants = append(allGrants, orgGrants...)
	}

	// 2. 收集 Project 级直接/团队授权（Application 不在 project/ws 层）
	if scopeInfo.ProjectID > 0 && principalType != valueobject.PrincipalTypeApplication {
		projGrants, err := c.collectProjectLevelGrants(ctx, principalType, principalID, teamIDs, resourceType, scopeInfo.ProjectID)
		if err != nil {
			return nil, err
		}
		allGrants = append(allGrants, projGrants...)
	}

	// 3. 收集 Workspace 级直接/团队授权
	if req.ScopeType == valueobject.ScopeTypeWorkspace && principalType != valueobject.PrincipalTypeApplication {
		wsGrants, err := c.collectWorkspaceLevelGrants(ctx, principalType, principalID, teamIDs, resourceType, req.ScopeID)
		if err != nil {
			return nil, err
		}
		allGrants = append(allGrants, wsGrants...)
	}

	// 4. Role：USER 取 user+team roles；TEAM 仅 team roles；APPLICATION 取
	// iam_application_roles。Application 的 Role 只允许组织级赋值，即使请求
	// 的业务资源是 project/workspace，也只能读取其所属组织的角色，防御历史
	// 或手工写入的细粒度 Application role 被意外求值。
	roleScopeType, roleScopeID := req.ScopeType, req.ScopeID
	if principalType == valueobject.PrincipalTypeApplication {
		if scopeInfo.OrgID == 0 {
			return nil, fmt.Errorf("application role evaluation requires organization scope")
		}
		roleScopeType = valueobject.ScopeTypeOrganization
		roleScopeID = scopeInfo.OrgID
	}
	roleGrants, err := c.collectRoleGrants(ctx, principalType, principalID, teamIDs, roleScopeType, roleScopeID, resourceType)
	if err != nil {
		return nil, err
	}
	allGrants = append(allGrants, roleGrants...)

	return allGrants, nil
}

// mergeWorkspaceManagementFallback implements the one-way implication
// WORKSPACE_MANAGEMENT -> ordinary workspace resource. The target's exact
// fine-grained grant wins when both exist at the same assignment layer; an
// ancestor MANAGEMENT grant remains available only when no more-specific
// target grant takes precedence through calculateEffectiveLevel.
func mergeWorkspaceManagementFallback(
	fineGrants []*entity.PermissionGrant,
	managementGrants []*entity.PermissionGrant,
) []*entity.PermissionGrant {
	if len(managementGrants) == 0 {
		return fineGrants
	}

	// NONE and expired records are not authorization grants and must not mask a
	// usable MANAGEMENT fallback.
	fineScopes := make(map[valueobject.ScopeType]struct{})
	for _, grant := range fineGrants {
		if grant != nil && grant.IsValid() && grant.PermissionLevel > valueobject.PermissionLevelNone {
			fineScopes[grant.ScopeType] = struct{}{}
		}
	}

	merged := make([]*entity.PermissionGrant, 0, len(fineGrants)+len(managementGrants))
	merged = append(merged, fineGrants...)
	for _, grant := range managementGrants {
		if grant == nil {
			continue
		}
		if _, hasFineGrant := fineScopes[grant.ScopeType]; hasFineGrant {
			continue
		}
		merged = append(merged, grant)
	}
	return merged
}

// collectOrgLevelGrants 收集组织级权限
func (c *PermissionCheckerImpl) collectOrgLevelGrants(
	ctx context.Context,
	principalType valueobject.PrincipalType,
	principalID string,
	teamIDs []string,
	resourceType valueobject.ResourceType,
	orgID uint,
) ([]*entity.PermissionGrant, error) {
	var grants []*entity.PermissionGrant

	// 用户直接授权（仅 USER 主体）
	if principalType == valueobject.PrincipalTypeUser {
		userPerms, err := c.permissionRepo.QueryOrgPermissions(
			ctx, orgID, valueobject.PrincipalTypeUser, []string{principalID}, resourceType,
		)
		if err != nil {
			return nil, err
		}
		for _, perm := range userPerms {
			grants = append(grants, perm.ToPermissionGrant())
		}
	}

	// 团队授权：USER 的所属团队，或 TEAM 主体自身
	if len(teamIDs) > 0 {
		teamPerms, err := c.permissionRepo.QueryOrgPermissions(
			ctx, orgID, valueobject.PrincipalTypeTeam, teamIDs, resourceType,
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

	// 应用授权（principal_id 展开：app_key + 数字 id，兼容历史 grant）
	if principalType == valueobject.PrincipalTypeApplication {
		ids := []string{principalID}
		if c.appAliases != nil {
			if expanded, err := c.appAliases.ExpandApplicationPrincipalIDs(ctx, principalID); err == nil && len(expanded) > 0 {
				ids = expanded
			}
		}
		appPerms, err := c.permissionRepo.QueryOrgPermissions(
			ctx, orgID, valueobject.PrincipalTypeApplication, ids, resourceType,
		)
		if err != nil {
			return nil, err
		}
		for _, perm := range appPerms {
			grant := perm.ToPermissionGrant()
			grant.Source = "application"
			grants = append(grants, grant)
		}
	}

	return grants, nil
}

// collectProjectLevelGrants 收集项目级权限
func (c *PermissionCheckerImpl) collectProjectLevelGrants(
	ctx context.Context,
	principalType valueobject.PrincipalType,
	principalID string,
	teamIDs []string,
	resourceType valueobject.ResourceType,
	projectID uint,
) ([]*entity.PermissionGrant, error) {
	var grants []*entity.PermissionGrant

	if principalType == valueobject.PrincipalTypeUser {
		userPerms, err := c.permissionRepo.QueryProjectPermissions(
			ctx, projectID, valueobject.PrincipalTypeUser, []string{principalID}, resourceType,
		)
		if err != nil {
			return nil, err
		}
		for _, perm := range userPerms {
			grants = append(grants, perm.ToPermissionGrant())
		}
	}

	if len(teamIDs) > 0 {
		teamPerms, err := c.permissionRepo.QueryProjectPermissions(
			ctx, projectID, valueobject.PrincipalTypeTeam, teamIDs, resourceType,
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
	principalType valueobject.PrincipalType,
	principalID string,
	teamIDs []string,
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
		log.Printf("[Permission] Failed to get workspace_id for id=%d: %v", workspaceID, err)
		return nil, fmt.Errorf("failed to get workspace_id: %w", err)
	}

	log.Printf("[Permission] Collecting grants: workspace=%d, workspace_id=%s, principal=%s/%s, resourceType=%s",
		workspaceID, workspace.WorkspaceID, principalType, principalID, resourceType)

	if principalType == valueobject.PrincipalTypeUser {
		userPerms, err := c.permissionRepo.QueryWorkspacePermissions(
			ctx, workspace.WorkspaceID, valueobject.PrincipalTypeUser, []string{principalID}, resourceType,
		)
		if err != nil {
			return nil, err
		}
		for _, perm := range userPerms {
			grants = append(grants, perm.ToPermissionGrant())
		}
	}

	if len(teamIDs) > 0 {
		teamPerms, err := c.permissionRepo.QueryWorkspacePermissions(
			ctx, workspace.WorkspaceID, valueobject.PrincipalTypeTeam, teamIDs, resourceType,
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

	log.Printf("[Permission] Total workspace grants collected: %d", len(grants))
	return grants, nil
}

// calculateEffectiveLevel 计算有效权限等级
// 规则（docs/iam/32-iam-remediation-report.md §2）：
// 1. 无有效授权 → NONE（不授权 = 拒绝）
// 2. 精确作用域优先：WORKSPACE > PROJECT > ORGANIZATION
// 3. 同层取 max(level)；level==NONE 的记录视为无效授权条目
// 4. 不做「显式 NONE grant 覆盖上层 ADMIN」
func (c *PermissionCheckerImpl) calculateEffectiveLevel(
	grants []*entity.PermissionGrant,
) valueobject.PermissionLevel {
	validGrants := c.filterExpiredGrants(grants)

	// 仅保留 level > NONE 的授权
	var effective []*entity.PermissionGrant
	for _, g := range validGrants {
		if g.PermissionLevel > valueobject.PermissionLevelNone {
			effective = append(effective, g)
		}
	}
	if len(effective) == 0 {
		return valueobject.PermissionLevelNone
	}

	// 精确作用域优先
	if ws := c.filterByScope(effective, valueobject.ScopeTypeWorkspace); len(ws) > 0 {
		return c.maxLevel(ws)
	}
	if proj := c.filterByScope(effective, valueobject.ScopeTypeProject); len(proj) > 0 {
		return c.maxLevel(proj)
	}
	if org := c.filterByScope(effective, valueobject.ScopeTypeOrganization); len(org) > 0 {
		return c.maxLevel(org)
	}

	// 无标准作用域标记时回退全局 max（兼容异常数据）
	return c.maxLevel(effective)
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

// resolveUserEmail 解析真实用户邮箱；不伪造 example.com
func (c *PermissionCheckerImpl) resolveUserEmail(ctx context.Context, userID string) (string, error) {
	if userID == "" {
		return "", fmt.Errorf("user_id empty")
	}
	// 已是邮箱形态时直接使用（部分调用方可传入）
	if strings.Contains(userID, "@") {
		return userID, nil
	}
	if c.userEmails == nil {
		return "", fmt.Errorf("user email lookup not configured")
	}
	email, err := c.userEmails.GetUserEmail(ctx, userID)
	if err != nil {
		return "", err
	}
	if email == "" {
		return "", fmt.Errorf("user has no email")
	}
	return email, nil
}

// checkTemporaryPermission 检查临时权限（email / user_id 双键）
func (c *PermissionCheckerImpl) checkTemporaryPermission(
	ctx context.Context,
	taskID uint,
	userEmail string,
	userID string,
	resourceType valueobject.ResourceType,
) (bool, error) {
	// 映射资源类型到临时权限类型
	permType := c.mapResourceToPermType(resourceType)
	if permType == "" {
		return false, nil
	}

	tempPerm, err := c.permissionRepo.CheckTemporaryPermission(ctx, taskID, userEmail, userID, permType)
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

// validateRequest 验证请求参数（主体已由 resolvePrincipal 保证非空）
func (c *PermissionCheckerImpl) validateRequest(
	req *CheckPermissionRequest,
	principalType valueobject.PrincipalType,
	principalID string,
) error {
	if principalID == "" {
		return fmt.Errorf("principal_id is required")
	}
	if !principalType.IsValid() {
		return fmt.Errorf("invalid principal_type: %s", principalType)
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
		return "No permission"
	}
	if effectiveLevel < requiredLevel {
		return fmt.Sprintf("Insufficient permission: have %s, need %s",
			effectiveLevel.String(), requiredLevel.String())
	}
	return ""
}

// collectRoleGrants 收集角色授予的权限
// USER：user roles + 所属 team roles；TEAM：仅该 team 的 roles；不互相伪造。
// 会查询当前作用域及父作用域的角色赋值。
func (c *PermissionCheckerImpl) collectRoleGrants(
	ctx context.Context,
	principalType valueobject.PrincipalType,
	principalID string,
	teamIDs []string,
	scopeType valueobject.ScopeType,
	scopeID uint,
	resourceType valueobject.ResourceType,
) ([]*entity.PermissionGrant, error) {
	var grants []*entity.PermissionGrant

	// 收集所有需要查询的作用域
	scopesToCheck := []struct {
		scopeType valueobject.ScopeType
		scopeID   uint
	}{
		{scopeType, scopeID}, // 当前作用域
	}

	// 如果是WORKSPACE作用域，还需要查询PROJECT和ORGANIZATION作用域的角色
	if scopeType == valueobject.ScopeTypeWorkspace {
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
		var allRoles []*entity.UserRole

		// USER 主体：查询用户角色
		if principalType == valueobject.PrincipalTypeUser {
			userRoles, err := c.permissionRepo.QueryUserRoles(ctx, principalID, scope.scopeType, scope.scopeID)
			if err != nil {
				return nil, fmt.Errorf("failed to query user roles: %w", err)
			}
			allRoles = append(allRoles, userRoles...)
		}

		// APPLICATION：Role 赋值（app_key，兼容数字 id 展开）
		if principalType == valueobject.PrincipalTypeApplication {
			ids := []string{principalID}
			if c.appAliases != nil {
				if expanded, err := c.appAliases.ExpandApplicationPrincipalIDs(ctx, principalID); err == nil && len(expanded) > 0 {
					ids = expanded
				}
			}
			appRoles, err := c.permissionRepo.QueryApplicationRoles(ctx, ids, scope.scopeType, scope.scopeID)
			if err != nil {
				return nil, fmt.Errorf("failed to query application roles: %w", err)
			}
			allRoles = append(allRoles, appRoles...)
		}

		// 团队角色：USER 的所属团队，或 TEAM 主体自身
		if len(teamIDs) > 0 {
			teamRoles, err := c.permissionRepo.QueryTeamRoles(ctx, teamIDs, scope.scopeType, scope.scopeID)
			if err != nil {
				return nil, fmt.Errorf("failed to query team roles: %w", err)
			}
			allRoles = append(allRoles, teamRoles...)
		}

		for _, role := range allRoles {
			if !role.IsValid() {
				continue
			}

			// Policy scope is the Role-assignment layer: a workspace resource may
			// be granted at ORGANIZATION, PROJECT, or WORKSPACE scope. It is not
			// always the native resource scope. Read only policies matching this
			// assignment so a narrow role assignment can never be lifted upward.
			policyScopeType := scope.scopeType
			if !policyScopeType.IsValid() {
				continue
			}
			policies, err := c.permissionRepo.QueryRolePolicies(ctx, role.RoleID, policyScopeType)
			if err != nil {
				continue
			}

			for _, policy := range policies {
				if valueobject.ResourceType(policy.ResourceType) != resourceType {
					continue
				}
				permLevel, err := valueobject.ParsePermissionLevel(policy.PermissionLevel)
				if err != nil {
					continue
				}
				policyScope, err := valueobject.ParseScopeType(policy.ScopeType)
				definitionScope, definitionScopeErr := valueobject.ParseScopeType(policy.PermissionScopeLevel)
				if err != nil || definitionScopeErr != nil ||
					definitionScope != resourceType.GetScopeLevel() ||
					policyScope != policyScopeType ||
					!policyScope.CanHostPolicyFor(definitionScope) {
					continue
				}

				// Role assignment 决定能力覆盖到目标 scope 的哪一层；scopesToCheck
				// 只包含当前请求 scope 及其祖先，故不会把窄 assignment 扩大为 org。
				grant := &entity.PermissionGrant{
					ScopeType:       scope.scopeType,
					ScopeID:         scope.scopeID,
					PrincipalType:   principalType,
					PrincipalID:     principalID,
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
