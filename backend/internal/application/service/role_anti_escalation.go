package service

import (
	"context"
	"errors"
	"fmt"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/valueobject"

	"gorm.io/gorm"
)

// 防提权错误（handler 映射 HTTP 状态）
var (
	// ErrPrivilegeEscalation 目标 Role/策略超出 actor 在该 scope 的有效权限
	ErrPrivilegeEscalation = errors.New("privilege escalation denied")
	// ErrSystemRoleRestricted 系统特权 Role（如 admin）仅平台超管可分配
	ErrSystemRoleRestricted = errors.New("system privileged role assignment restricted")
	// ErrSystemRolePolicyReadonly 系统 Role 策略仅超管可改
	ErrSystemRolePolicyReadonly = errors.New("system role policies are read-only")
	// ErrRoleNotFound 角色不存在
	ErrRoleNotFound = errors.New("role not found")
	// ErrPermissionDefNotFound 权限定义不存在
	ErrPermissionDefNotFound = errors.New("permission definition not found")
	// ErrScopeOutsideAuthOrg assignment scope 不在鉴权 org 子树
	ErrScopeOutsideAuthOrg = errors.New("assignment scope outside authorized organization")
	// ErrAntiEscalationMisconfigured checker 未注入（fail-closed）
	ErrAntiEscalationMisconfigured = errors.New("anti-escalation checker not configured")
)

// rolePolicySpec Role 策略展开后的单条能力需求
type rolePolicySpec struct {
	PermissionID    string
	PermissionLevel valueobject.PermissionLevel
	ResourceType    valueobject.ResourceType
	PolicyScopeType string
}

// RoleAntiEscalationService Role 赋权/改策略防提权（docs/iam/32 §3.3）
// 规则：
//  1. 目标 Role 的 policy 闭包 ⊆ actor 在赋值 scope 上的有效权限
//  2. 系统特权 Role（is_system && name=admin）仅 is_system_admin 可分配
//  3. 新增单条 policy 时，actor 必须已持有该 resource_type@level
type RoleAntiEscalationService struct {
	db      *gorm.DB
	checker PermissionChecker
}

// NewRoleAntiEscalationService 创建防提权服务
func NewRoleAntiEscalationService(db *gorm.DB, checker PermissionChecker) *RoleAntiEscalationService {
	return &RoleAntiEscalationService{db: db, checker: checker}
}

// EnsureCanAssignRole 校验 actor 是否可在指定 scope 将 role 赋给用户/团队
func (s *RoleAntiEscalationService) EnsureCanAssignRole(
	ctx context.Context,
	actorUserID string,
	isSystemAdmin bool,
	roleID uint,
	scopeType valueobject.ScopeType,
	scopeID uint,
) error {
	if s == nil || s.checker == nil {
		return ErrAntiEscalationMisconfigured
	}
	if actorUserID == "" {
		return fmt.Errorf("%w: actor required", ErrPrivilegeEscalation)
	}

	var role entity.Role
	if err := s.db.WithContext(ctx).First(&role, roleID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrRoleNotFound
		}
		return err
	}

	// 系统特权 Role：仅平台超管可分配（与业务 IAM 旁路分离）
	if isPrivilegedSystemRole(&role) {
		if !isSystemAdmin {
			return fmt.Errorf("%w: only system admin may assign role %q", ErrSystemRoleRestricted, role.Name)
		}
		return nil
	}

	// 其它 is_system Role：非超管仍须闭包校验（不得静默放宽）。
	// Role 可以同时保存多个 assignment 层的 policy，只有与本次
	// assignment 同层的 policy 才会生效，也才应参与闭包校验。
	specs, err := s.loadRolePolicySpecsAtScope(ctx, roleID, scopeType)
	if err != nil {
		return err
	}
	// 空策略 Role：禁止非超管赋值（避免后续加策略突然变强）
	if len(specs) == 0 && !isSystemAdmin {
		return fmt.Errorf("%w: empty-policy role assignment requires system admin", ErrPrivilegeEscalation)
	}
	return s.ensureActorCoversSpecs(ctx, actorUserID, scopeType, scopeID, specs)
}

// EnsureAssignmentScopeInAuthOrg assignment scope 必须落在鉴权 org 子树内
func (s *RoleAntiEscalationService) EnsureAssignmentScopeInAuthOrg(
	ctx context.Context,
	scopeType valueobject.ScopeType,
	scopeID uint,
	authOrgID uint,
) error {
	if authOrgID == 0 {
		return fmt.Errorf("%w: auth org missing", ErrScopeOutsideAuthOrg)
	}
	switch scopeType {
	case valueobject.ScopeTypeOrganization:
		if scopeID != authOrgID {
			return fmt.Errorf("%w: org scope %d != auth %d", ErrScopeOutsideAuthOrg, scopeID, authOrgID)
		}
		return nil
	case valueobject.ScopeTypeProject:
		var orgID uint
		err := s.db.WithContext(ctx).Table("projects").Select("org_id").Where("id = ?", scopeID).Scan(&orgID).Error
		if err != nil || orgID == 0 {
			return fmt.Errorf("%w: project %d not found", ErrScopeOutsideAuthOrg, scopeID)
		}
		if orgID != authOrgID {
			return fmt.Errorf("%w: project %d belongs to org %d", ErrScopeOutsideAuthOrg, scopeID, orgID)
		}
		return nil
	case valueobject.ScopeTypeWorkspace:
		// scope_id 为 workspace 数字主键时：经 workspace_project_relations 溯源 org
		var wsSem string
		_ = s.db.WithContext(ctx).Table("workspaces").Select("workspace_id").Where("id = ?", scopeID).Scan(&wsSem)
		if wsSem == "" {
			// 也可能调用方传语义化 id 的 hash 不匹配 — 尝试把 scopeID 当不存在
			return fmt.Errorf("%w: workspace id %d not found", ErrScopeOutsideAuthOrg, scopeID)
		}
		var orgID uint
		err := s.db.WithContext(ctx).Raw(`
SELECT p.org_id FROM workspace_project_relations wpr
JOIN projects p ON p.id = wpr.project_id
WHERE wpr.workspace_id = ? LIMIT 1`, wsSem).Scan(&orgID).Error
		if err != nil || orgID == 0 {
			// 无项目关联：拒绝跨 org 猜测（单租户默认 org1 可放宽需配置，默认 fail-closed）
			return fmt.Errorf("%w: workspace %s has no project/org binding", ErrScopeOutsideAuthOrg, wsSem)
		}
		if orgID != authOrgID {
			return fmt.Errorf("%w: workspace belongs to org %d", ErrScopeOutsideAuthOrg, orgID)
		}
		return nil
	default:
		return fmt.Errorf("%w: unsupported scope", ErrScopeOutsideAuthOrg)
	}
}

// EnsureCanAddRolePolicy 校验 actor 是否可向 Role 添加一条策略
// checkScope：通常为 ORGANIZATION + 当前 org_id（与 IAM_ROLES 中间件一致）
func (s *RoleAntiEscalationService) EnsureCanAddRolePolicy(
	ctx context.Context,
	actorUserID string,
	isSystemAdmin bool,
	roleID uint,
	permissionID string,
	permissionLevel string,
	policyScopeType valueobject.ScopeType,
	checkScopeType valueobject.ScopeType,
	checkScopeID uint,
) error {
	if s == nil || s.checker == nil {
		return ErrAntiEscalationMisconfigured
	}
	if actorUserID == "" {
		return fmt.Errorf("%w: actor required", ErrPrivilegeEscalation)
	}

	var role entity.Role
	if err := s.db.WithContext(ctx).First(&role, roleID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrRoleNotFound
		}
		return err
	}
	// 系统 Role 策略：非超管禁止增删改（A-2）
	if role.IsSystem && !isSystemAdmin {
		return fmt.Errorf("%w: role %q", ErrSystemRolePolicyReadonly, role.Name)
	}

	spec, err := s.resolvePolicySpec(ctx, permissionID, permissionLevel, policyScopeType)
	if err != nil {
		return err
	}

	return s.ensureActorCoversSpecs(ctx, actorUserID, checkScopeType, checkScopeID, []rolePolicySpec{spec})
}

// EnsureCanMutateSystemRolePolicies 删除策略前校验系统 Role
func (s *RoleAntiEscalationService) EnsureCanMutateSystemRolePolicies(
	ctx context.Context,
	isSystemAdmin bool,
	roleID uint,
) error {
	if s == nil {
		return ErrAntiEscalationMisconfigured
	}
	var role entity.Role
	if err := s.db.WithContext(ctx).First(&role, roleID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrRoleNotFound
		}
		return err
	}
	if role.IsSystem && !isSystemAdmin {
		return fmt.Errorf("%w: role %q", ErrSystemRolePolicyReadonly, role.Name)
	}
	return nil
}

// EnsureCanCloneRole 克隆源 Role 的策略闭包不得超出 actor 能力
func (s *RoleAntiEscalationService) EnsureCanCloneRole(
	ctx context.Context,
	actorUserID string,
	isSystemAdmin bool,
	sourceRoleID uint,
	checkScopeType valueobject.ScopeType,
	checkScopeID uint,
) error {
	if s == nil || s.checker == nil {
		return ErrAntiEscalationMisconfigured
	}
	var role entity.Role
	if err := s.db.WithContext(ctx).First(&role, sourceRoleID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrRoleNotFound
		}
		return err
	}
	// 克隆 admin 系统角色：禁止非超管（即使只读策略也避免复制特权模板）
	if isPrivilegedSystemRole(&role) && !isSystemAdmin {
		return fmt.Errorf("%w: cannot clone privileged system role %q", ErrSystemRoleRestricted, role.Name)
	}
	specs, err := s.loadRolePolicySpecs(ctx, sourceRoleID)
	if err != nil {
		return err
	}
	return s.ensureActorCoversSpecs(ctx, actorUserID, checkScopeType, checkScopeID, specs)
}

func isPrivilegedSystemRole(role *entity.Role) bool {
	// R1-4：所有 is_system Role 仅超管可 Assign/Clone（与策略只读对齐）
	return role != nil && role.IsSystem
}

func (s *RoleAntiEscalationService) loadRolePolicySpecs(ctx context.Context, roleID uint) ([]rolePolicySpec, error) {
	return s.loadRolePolicySpecsForScope(ctx, roleID, "")
}

// loadRolePolicySpecsAtScope returns only policies activated by an assignment
// at assignmentScope. Policies configured for another layer are intentionally
// retained for their own assignments; they must not make this assignment
// stronger or cause an unrelated false denial.
func (s *RoleAntiEscalationService) loadRolePolicySpecsAtScope(
	ctx context.Context,
	roleID uint,
	assignmentScope valueobject.ScopeType,
) ([]rolePolicySpec, error) {
	if !assignmentScope.IsValid() {
		return nil, fmt.Errorf("%w: invalid assignment scope %q", ErrPrivilegeEscalation, assignmentScope)
	}
	return s.loadRolePolicySpecsForScope(ctx, roleID, string(assignmentScope))
}

func (s *RoleAntiEscalationService) loadRolePolicySpecsForScope(
	ctx context.Context,
	roleID uint,
	onlyPolicyScope string,
) ([]rolePolicySpec, error) {
	type row struct {
		PermissionID    string
		PermissionLevel string
		ScopeType       string
		ScopeLevel      string
		ResourceType    string
	}
	var rows []row
	query := s.db.WithContext(ctx).Table("iam_role_policies AS rp").
		Select("rp.permission_id, rp.permission_level, rp.scope_type, COALESCE(pd.scope_level, '') AS scope_level, COALESCE(pd.resource_type, '') AS resource_type").
		Joins("LEFT JOIN permission_definitions AS pd ON pd.id = rp.permission_id").
		Where("rp.role_id = ?", roleID)
	if onlyPolicyScope != "" {
		query = query.Where("rp.scope_type = ?", onlyPolicyScope)
	}
	err := query.Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	out := make([]rolePolicySpec, 0, len(rows))
	for _, r := range rows {
		level, err := valueobject.ParsePermissionLevel(r.PermissionLevel)
		if err != nil {
			return nil, fmt.Errorf("role policy %s: invalid level %q: %w", r.PermissionID, r.PermissionLevel, err)
		}
		rt := valueobject.ResourceType(r.ResourceType)
		if r.ResourceType == "" || !rt.IsValid() {
			// 定义缺失时拒绝赋权，避免静默跳过提权策略
			return nil, fmt.Errorf("%w: permission %s has no valid resource_type", ErrPrivilegeEscalation, r.PermissionID)
		}
		policyScope, err := valueobject.ParseScopeType(r.ScopeType)
		if err != nil {
			return nil, fmt.Errorf("%w: permission %s has invalid policy scope %q", ErrPrivilegeEscalation, r.PermissionID, r.ScopeType)
		}
		definitionScope, err := valueobject.ParseScopeType(r.ScopeLevel)
		if err != nil || definitionScope != rt.GetScopeLevel() || !policyScope.CanHostPolicyFor(definitionScope) {
			return nil, fmt.Errorf("%w: permission %s cannot be granted at policy scope %q for definition scope %q", ErrPrivilegeEscalation, r.PermissionID, r.ScopeType, r.ScopeLevel)
		}
		if level == valueobject.PermissionLevelNone {
			continue // NONE 策略不赋予能力，跳过
		}
		out = append(out, rolePolicySpec{
			PermissionID:    r.PermissionID,
			PermissionLevel: level,
			ResourceType:    rt,
			PolicyScopeType: string(policyScope),
		})
	}
	return out, nil
}

func (s *RoleAntiEscalationService) resolvePolicySpec(
	ctx context.Context,
	permissionID string,
	levelStr string,
	policyScopeType valueobject.ScopeType,
) (rolePolicySpec, error) {
	level, err := valueobject.ParsePermissionLevel(levelStr)
	if err != nil {
		return rolePolicySpec{}, err
	}
	if !policyScopeType.IsValid() {
		return rolePolicySpec{}, fmt.Errorf("%w: invalid policy scope %q", ErrPrivilegeEscalation, policyScopeType)
	}
	var def entity.PermissionDefinition
	if err := s.db.WithContext(ctx).Where("id = ?", permissionID).First(&def).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return rolePolicySpec{}, ErrPermissionDefNotFound
		}
		return rolePolicySpec{}, err
	}
	if !def.ResourceType.IsValid() {
		return rolePolicySpec{}, fmt.Errorf("%w: permission %s has invalid resource_type", ErrPrivilegeEscalation, permissionID)
	}
	if !def.ScopeLevel.IsValid() || def.ScopeLevel != def.ResourceType.GetScopeLevel() || !policyScopeType.CanHostPolicyFor(def.ScopeLevel) {
		return rolePolicySpec{}, fmt.Errorf("%w: permission %s cannot be granted at policy scope %s for definition scope %s", ErrPrivilegeEscalation, permissionID, policyScopeType, def.ScopeLevel)
	}
	return rolePolicySpec{
		PermissionID:    permissionID,
		PermissionLevel: level,
		ResourceType:    def.ResourceType,
		PolicyScopeType: string(policyScopeType),
	}, nil
}

func (s *RoleAntiEscalationService) ensureActorCoversSpecs(
	ctx context.Context,
	actorUserID string,
	scopeType valueobject.ScopeType,
	scopeID uint,
	specs []rolePolicySpec,
) error {
	// 空策略 Role 不扩大权限，允许（仍受路由 IAM 管理权限约束）
	for _, p := range specs {
		req := &CheckPermissionRequest{
			UserID:        actorUserID,
			PrincipalType: valueobject.PrincipalTypeUser,
			PrincipalID:   actorUserID,
			ResourceType:  p.ResourceType,
			ScopeType:     scopeType,
			ScopeID:       scopeID,
			RequiredLevel: p.PermissionLevel,
		}
		result, err := s.checker.CheckPermission(ctx, req)
		if err != nil {
			return fmt.Errorf("permission check failed for %s: %w", p.ResourceType, err)
		}
		if result == nil || !result.IsAllowed {
			have := valueobject.PermissionLevelNone
			if result != nil {
				have = result.EffectiveLevel
			}
			return fmt.Errorf("%w: cannot grant %s@%s (actor has %s on %s at %s/%d)",
				ErrPrivilegeEscalation,
				p.ResourceType, p.PermissionLevel.String(),
				have.String(), p.ResourceType, scopeType, scopeID,
			)
		}
	}
	return nil
}

// IsPrivilegeEscalationError 是否为防提权类错误（403）
func IsPrivilegeEscalationError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrPrivilegeEscalation) ||
		errors.Is(err, ErrSystemRoleRestricted) ||
		errors.Is(err, ErrSystemRolePolicyReadonly) ||
		errors.Is(err, ErrScopeOutsideAuthOrg)
}
