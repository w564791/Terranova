package service

import (
	"context"
	"fmt"
	"strings"

	"iac-platform/internal/domain/valueobject"

	"gorm.io/gorm"
)

// WorkspaceListAccessContextKey is the Gin context key shared by the list
// middleware and the workspace controller. The value is *WorkspaceListAccess.
const WorkspaceListAccessContextKey = "workspace_list_access"

// WorkspaceListAccess describes the part of an organization a principal may
// enumerate through GET /workspaces. HasAccess is separate from WorkspaceIDs:
// a principal may hold a valid project-scoped Role while that project currently
// contains zero workspaces, which should return an authorized empty list rather
// than look identical to a principal with no grant at all. A nil/empty
// WorkspaceIDs slice only means "no scoped workspaces" when FullOrganization is
// false; callers must not interpret it as an unrestricted query.
type WorkspaceListAccess struct {
	FullOrganization bool
	HasAccess        bool
	WorkspaceIDs     []string
}

// WorkspaceListAccessRequest identifies the principal and tenant context for a
// workspace list request. OrgID must already be resolved from the request by
// the IAM middleware.
type WorkspaceListAccessRequest struct {
	UserID        string
	PrincipalType valueobject.PrincipalType
	PrincipalID   string
	OrgID         uint
	// Capability optionally narrows the visible list to workspaces where the
	// principal additionally holds Capability.ResourceType at
	// Capability.Level (e.g. WORKSPACE_RESOURCES WRITE for the manifest deploy
	// picker). nil keeps the plain list semantics.
	Capability *WorkspaceCapability
}

// WorkspaceCapability is a workspace-scoped permission requirement used to
// filter a workspace list server-side (GET /workspaces?capability=RES:LEVEL).
type WorkspaceCapability struct {
	ResourceType valueobject.ResourceType
	Level        valueobject.PermissionLevel
}

// ParseWorkspaceCapability parses "RESOURCE_TYPE:LEVEL". The resource type
// must be a known workspace-level resource and the level READ/WRITE/ADMIN;
// anything else is rejected so callers can answer 400 instead of silently
// returning an unfiltered list.
func ParseWorkspaceCapability(raw string) (*WorkspaceCapability, error) {
	parts := strings.Split(raw, ":")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("capability must be RESOURCE_TYPE:LEVEL")
	}
	rt, err := valueobject.ParseResourceType(strings.ToUpper(parts[0]))
	if err != nil {
		return nil, err
	}
	if !rt.IsWorkspaceLevel() {
		return nil, fmt.Errorf("capability resource type %s is not workspace-scoped", rt)
	}
	level, err := valueobject.ParsePermissionLevel(strings.ToUpper(parts[1]))
	if err != nil {
		return nil, err
	}
	if level == valueobject.PermissionLevelNone {
		return nil, fmt.Errorf("capability level must be READ, WRITE or ADMIN")
	}
	return &WorkspaceCapability{ResourceType: rt, Level: level}, nil
}

// WorkspaceListAccessResolver is deliberately small so the HTTP middleware
// can be tested independently and callers cannot accidentally bypass the IAM
// checker with an unscoped workspace query.
type WorkspaceListAccessResolver interface {
	ResolveWorkspaceListAccess(ctx context.Context, req WorkspaceListAccessRequest) (*WorkspaceListAccess, error)
}

// WorkspaceListAccessService resolves list visibility by reusing the exact
// PermissionChecker path used by individual workspace endpoints. This is
// important because role assignments may live at ORGANIZATION, PROJECT, or
// WORKSPACE scope, and because direct grants, team roles, applications, and
// expiry all have one canonical evaluator.
//
// It intentionally starts from the target organization's workspace set and
// evaluates each candidate as a WORKSPACE_MANAGEMENT READ. That prevents an
// authorization record at one workspace/project from broadening the SQL list
// to siblings. A future bulk PermissionChecker implementation can optimize the
// per-workspace evaluations without changing these semantics.
type WorkspaceListAccessService struct {
	db      *gorm.DB
	checker PermissionChecker
}

func NewWorkspaceListAccessService(db *gorm.DB, checker PermissionChecker) *WorkspaceListAccessService {
	return &WorkspaceListAccessService{db: db, checker: checker}
}

func (s *WorkspaceListAccessService) ResolveWorkspaceListAccess(
	ctx context.Context,
	req WorkspaceListAccessRequest,
) (*WorkspaceListAccess, error) {
	access, err := s.resolveReadableWorkspaces(ctx, req)
	if err != nil || req.Capability == nil || access == nil || !access.HasAccess {
		return access, err
	}
	return s.narrowByCapability(ctx, req, access)
}

// narrowByCapability intersects the readable list with the workspaces where
// the principal holds the requested capability, using the same
// PermissionChecker path as the per-workspace endpoints (so ORGANIZATION,
// PROJECT and WORKSPACE scoped Roles and the WORKSPACE_MANAGEMENT umbrella are
// all honoured). HasAccess is preserved: an authorized caller with no capable
// workspace gets an empty list, not a 403.
func (s *WorkspaceListAccessService) narrowByCapability(
	ctx context.Context,
	req WorkspaceListAccessRequest,
	access *WorkspaceListAccess,
) (*WorkspaceListAccess, error) {
	capReq := func(scope valueobject.ScopeType, scopeID uint, scopeIDStr string) *CheckPermissionRequest {
		return &CheckPermissionRequest{
			UserID:        req.UserID,
			PrincipalType: req.PrincipalType,
			PrincipalID:   req.PrincipalID,
			ResourceType:  req.Capability.ResourceType,
			ScopeType:     scope,
			ScopeID:       scopeID,
			ScopeIDStr:    scopeIDStr,
			RequiredLevel: req.Capability.Level,
		}
	}

	orgResult, err := s.checker.CheckPermission(ctx, capReq(valueobject.ScopeTypeOrganization, req.OrgID, ""))
	if err != nil {
		return nil, fmt.Errorf("check organization capability %s: %w", req.Capability.ResourceType, err)
	}
	if orgResult != nil && orgResult.IsAllowed {
		return access, nil
	}

	candidates := access.WorkspaceIDs
	if access.FullOrganization {
		if candidates, err = s.workspaceIDsInOrg(ctx, req.OrgID); err != nil {
			return nil, err
		}
	}
	allowed := make([]string, 0, len(candidates))
	for _, workspaceID := range candidates {
		result, err := s.checker.CheckPermission(ctx, capReq(valueobject.ScopeTypeWorkspace, 0, workspaceID))
		if err != nil {
			return nil, fmt.Errorf("check capability %s for %q: %w", req.Capability.ResourceType, workspaceID, err)
		}
		if result != nil && result.IsAllowed {
			allowed = append(allowed, workspaceID)
		}
	}
	return &WorkspaceListAccess{HasAccess: true, WorkspaceIDs: allowed}, nil
}

// HasAnyWorkspace reports whether the access covers at least one workspace.
// FullOrganization is treated as true without enumerating.
func (a *WorkspaceListAccess) HasAnyWorkspace() bool {
	return a != nil && a.HasAccess && (a.FullOrganization || len(a.WorkspaceIDs) > 0)
}

// AllowsWorkspace reports whether workspaceID is inside the access set.
func (a *WorkspaceListAccess) AllowsWorkspace(workspaceID string) bool {
	if a == nil || !a.HasAccess {
		return false
	}
	if a.FullOrganization {
		return true
	}
	for _, id := range a.WorkspaceIDs {
		if id == workspaceID {
			return true
		}
	}
	return false
}

func (s *WorkspaceListAccessService) resolveReadableWorkspaces(
	ctx context.Context,
	req WorkspaceListAccessRequest,
) (*WorkspaceListAccess, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("workspace list access database is not configured")
	}
	if s.checker == nil {
		return nil, fmt.Errorf("workspace list access permission checker is not configured")
	}
	if req.OrgID == 0 {
		return nil, fmt.Errorf("organization scope is required")
	}

	// Preserve the established fast path: WORKSPACES READ at the selected
	// organization gives a full organization listing. Do not use a system-admin
	// bypass here; business IAM remains the source of truth.
	orgResult, err := s.checker.CheckPermission(ctx, &CheckPermissionRequest{
		UserID:        req.UserID,
		PrincipalType: req.PrincipalType,
		PrincipalID:   req.PrincipalID,
		ResourceType:  valueobject.ResourceTypeAllWorkspaces,
		ScopeType:     valueobject.ScopeTypeOrganization,
		ScopeID:       req.OrgID,
		RequiredLevel: valueobject.PermissionLevelRead,
	})
	if err != nil {
		return nil, fmt.Errorf("check organization workspace-list permission: %w", err)
	}
	if orgResult != nil && orgResult.IsAllowed {
		return &WorkspaceListAccess{FullOrganization: true, HasAccess: true}, nil
	}

	// WORKSPACE_MANAGEMENT assigned at ORGANIZATION scope is also a true
	// organization-wide workspace capability. Checking it here avoids an O(N)
	// per-workspace walk for roles that intentionally use the umbrella resource
	// instead of the metadata-only WORKSPACES definition. It is the same checker
	// semantics used for an individual workspace, merely evaluated at its
	// assignment layer.
	managementResult, err := s.checker.CheckPermission(ctx, &CheckPermissionRequest{
		UserID:        req.UserID,
		PrincipalType: req.PrincipalType,
		PrincipalID:   req.PrincipalID,
		ResourceType:  valueobject.ResourceTypeWorkspaceManagement,
		ScopeType:     valueobject.ScopeTypeOrganization,
		ScopeID:       req.OrgID,
		RequiredLevel: valueobject.PermissionLevelRead,
	})
	if err != nil {
		return nil, fmt.Errorf("check organization workspace-management permission: %w", err)
	}
	if managementResult != nil && managementResult.IsAllowed {
		return &WorkspaceListAccess{FullOrganization: true, HasAccess: true}, nil
	}

	workspaceIDs, err := s.workspaceIDsInOrg(ctx, req.OrgID)
	if err != nil {
		return nil, err
	}

	allowed := make([]string, 0, len(workspaceIDs))
	for _, workspaceID := range workspaceIDs {
		result, err := s.checker.CheckPermission(ctx, &CheckPermissionRequest{
			UserID:        req.UserID,
			PrincipalType: req.PrincipalType,
			PrincipalID:   req.PrincipalID,
			ResourceType:  valueobject.ResourceTypeWorkspaceManagement,
			ScopeType:     valueobject.ScopeTypeWorkspace,
			ScopeIDStr:    workspaceID,
			RequiredLevel: valueobject.PermissionLevelRead,
		})
		if err != nil {
			// A failed evaluation must not be treated as an implicit grant or
			// silently omitted: fail the request closed so operators notice a
			// malformed workspace/role relation.
			return nil, fmt.Errorf("check workspace-list permission for %q: %w", workspaceID, err)
		}
		if result != nil && result.IsAllowed {
			allowed = append(allowed, workspaceID)
		}
	}

	if len(allowed) > 0 {
		return &WorkspaceListAccess{HasAccess: true, WorkspaceIDs: allowed}, nil
	}

	// A project-scoped role can be valid even before its project contains a
	// workspace. Keep that distinct from no authorization so the caller returns
	// a normal empty list (and pagination metadata) for a legitimate empty
	// project. Workspace-scoped roles cannot be valid without a matching
	// candidate above, so no separate orphaned-workspace path is needed.
	hasProjectAccess, err := s.hasReadableWorkspaceProject(ctx, req)
	if err != nil {
		return nil, err
	}
	return &WorkspaceListAccess{HasAccess: hasProjectAccess, WorkspaceIDs: allowed}, nil
}

func (s *WorkspaceListAccessService) workspaceIDsInOrg(ctx context.Context, orgID uint) ([]string, error) {
	workspaceIDs := make([]string, 0)
	err := s.db.WithContext(ctx).
		Table("workspaces AS w").
		Distinct("w.workspace_id").
		Joins("JOIN workspace_project_relations AS wpr ON wpr.workspace_id = w.workspace_id").
		Joins("JOIN projects AS p ON p.id = wpr.project_id").
		Where("p.org_id = ?", orgID).
		Order("w.workspace_id ASC").
		Pluck("w.workspace_id", &workspaceIDs).Error
	if err != nil {
		return nil, fmt.Errorf("list organization workspaces: %w", err)
	}
	return workspaceIDs, nil
}

func (s *WorkspaceListAccessService) hasReadableWorkspaceProject(ctx context.Context, req WorkspaceListAccessRequest) (bool, error) {
	projectIDs := make([]uint, 0)
	if err := s.db.WithContext(ctx).
		Table("projects").
		Where("org_id = ?", req.OrgID).
		Order("id ASC").
		Pluck("id", &projectIDs).Error; err != nil {
		return false, fmt.Errorf("list organization projects for workspace access: %w", err)
	}

	for _, projectID := range projectIDs {
		result, err := s.checker.CheckPermission(ctx, &CheckPermissionRequest{
			UserID:        req.UserID,
			PrincipalType: req.PrincipalType,
			PrincipalID:   req.PrincipalID,
			ResourceType:  valueobject.ResourceTypeWorkspaceManagement,
			ScopeType:     valueobject.ScopeTypeProject,
			ScopeID:       projectID,
			RequiredLevel: valueobject.PermissionLevelRead,
		})
		if err != nil {
			return false, fmt.Errorf("check project workspace-list permission for %d: %w", projectID, err)
		}
		if result != nil && result.IsAllowed {
			return true, nil
		}
	}
	return false, nil
}
