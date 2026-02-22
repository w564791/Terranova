package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/repository"
	"iac-platform/internal/domain/valueobject"

	"gorm.io/gorm"
)

// GrantPermissionRequest 授予权限请求
type GrantPermissionRequest struct {
	ScopeType       valueobject.ScopeType       `json:"scope_type"`
	ScopeID         uint                        `json:"scope_id"`
	PrincipalType   valueobject.PrincipalType   `json:"principal_type"`
	PrincipalID     string                      `json:"principal_id"`
	PermissionID    string                      `json:"permission_id"` // 业务语义ID
	PermissionLevel valueobject.PermissionLevel `json:"permission_level"`
	GrantedBy       string                      `json:"granted_by"` // user_id
	ExpiresAt       *time.Time                  `json:"expires_at,omitempty"`
	Reason          string                      `json:"reason,omitempty"`
}

// RevokePermissionRequest 撤销权限请求
type RevokePermissionRequest struct {
	ScopeType    valueobject.ScopeType `json:"scope_type"`
	AssignmentID uint                  `json:"assignment_id"`
	RevokedBy    string                `json:"revoked_by"` // user_id
	Reason       string                `json:"reason,omitempty"`
}

// ModifyPermissionRequest 修改权限请求
type ModifyPermissionRequest struct {
	ScopeType    valueobject.ScopeType       `json:"scope_type"`
	AssignmentID uint                        `json:"assignment_id"`
	NewLevel     valueobject.PermissionLevel `json:"new_level"`
	ModifiedBy   string                      `json:"modified_by"` // user_id
	Reason       string                      `json:"reason,omitempty"`
}

// GrantPresetRequest 授予预设权限请求
type GrantPresetRequest struct {
	ScopeType     valueobject.ScopeType     `json:"scope_type"`
	ScopeID       uint                      `json:"scope_id"`
	PrincipalType valueobject.PrincipalType `json:"principal_type"`
	PrincipalID   string                    `json:"principal_id"`
	PresetName    string                    `json:"preset_name"` // READ/WRITE/ADMIN
	GrantedBy     string                    `json:"granted_by"`  // user_id
	Reason        string                    `json:"reason,omitempty"`
}

// PermissionService 权限管理服务接口
type PermissionService interface {
	// GrantPermission 授予权限
	GrantPermission(ctx context.Context, req *GrantPermissionRequest) error

	// RevokePermission 撤销权限
	RevokePermission(ctx context.Context, req *RevokePermissionRequest) error

	// ModifyPermission 修改权限等级
	ModifyPermission(ctx context.Context, req *ModifyPermissionRequest) error

	// GrantPresetPermissions 授予预设权限集（READ/WRITE/ADMIN）
	GrantPresetPermissions(ctx context.Context, req *GrantPresetRequest) error

	// ListPermissions 列出指定作用域的所有权限分配
	ListPermissions(ctx context.Context, scopeType valueobject.ScopeType, scopeID uint) ([]*entity.PermissionGrant, error)

	// ListPermissionDefinitions 列出所有权限定义
	ListPermissionDefinitions(ctx context.Context) ([]*entity.PermissionDefinition, error)

	// ListPermissionsByPrincipal 列出指定主体的所有权限（跨所有作用域）
	ListPermissionsByPrincipal(ctx context.Context, principalType valueobject.PrincipalType, principalID string) ([]*entity.PermissionGrant, error)

	// GetPermissionDefinitionByID 根据语义ID获取权限定义
	GetPermissionDefinitionByID(ctx context.Context, permissionID string) (*entity.PermissionDefinition, error)
}

// PermissionServiceImpl 权限管理服务实现
type PermissionServiceImpl struct {
	permissionRepo repository.PermissionRepository
	auditRepo      repository.AuditRepository
	checker        PermissionChecker
	db             *gorm.DB
	// cache          cache.PermissionCache // TODO: 实现缓存
}

// NewPermissionService 创建权限管理服务实例
func NewPermissionService(
	permissionRepo repository.PermissionRepository,
	auditRepo repository.AuditRepository,
	checker PermissionChecker,
	db *gorm.DB,
) PermissionService {
	return &PermissionServiceImpl{
		permissionRepo: permissionRepo,
		auditRepo:      auditRepo,
		checker:        checker,
		db:             db,
	}
}

// GrantPermission 授予权限
func (s *PermissionServiceImpl) GrantPermission(
	ctx context.Context,
	req *GrantPermissionRequest,
) error {
	// 1. 验证请求参数
	if err := s.validateGrantRequest(req); err != nil {
		return err
	}

	// 2. 验证主体类型和作用域的兼容性
	if !req.PrincipalType.CanBeGrantedAt(req.ScopeType) {
		return fmt.Errorf("principal type %s cannot be granted at scope %s",
			req.PrincipalType, req.ScopeType)
	}

	// 3. 根据作用域类型授予权限
	switch req.ScopeType {
	case valueobject.ScopeTypeOrganization:
		return s.grantOrgPermission(ctx, req)
	case valueobject.ScopeTypeProject:
		return s.grantProjectPermission(ctx, req)
	case valueobject.ScopeTypeWorkspace:
		return s.grantWorkspacePermission(ctx, req)
	default:
		return fmt.Errorf("unsupported scope type: %s", req.ScopeType)
	}
}

// grantOrgPermission 授予组织级权限
func (s *PermissionServiceImpl) grantOrgPermission(
	ctx context.Context,
	req *GrantPermissionRequest,
) error {
	permission := &entity.OrgPermission{
		OrgID:           req.ScopeID,
		PrincipalType:   req.PrincipalType,
		PrincipalID:     req.PrincipalID,
		PermissionID:    req.PermissionID,
		PermissionLevel: req.PermissionLevel,
		GrantedBy:       &req.GrantedBy,
		GrantedAt:       time.Now(),
		ExpiresAt:       req.ExpiresAt,
		Reason:          req.Reason,
	}

	if err := s.permissionRepo.GrantOrgPermission(ctx, permission); err != nil {
		return fmt.Errorf("failed to grant org permission: %w", err)
	}

	// 记录审计日志
	s.logPermissionChange(ctx, entity.AuditActionGrant, req.ScopeType, req.ScopeID,
		req.PrincipalType, req.PrincipalID, req.PermissionID, nil, &req.PermissionLevel, req.GrantedBy, req.Reason)

	// TODO: 使缓存失效

	return nil
}

// grantProjectPermission 授予项目级权限
func (s *PermissionServiceImpl) grantProjectPermission(
	ctx context.Context,
	req *GrantPermissionRequest,
) error {
	permission := &entity.ProjectPermission{
		ProjectID:       req.ScopeID,
		PrincipalType:   req.PrincipalType,
		PrincipalID:     req.PrincipalID,
		PermissionID:    req.PermissionID,
		PermissionLevel: req.PermissionLevel,
		GrantedBy:       &req.GrantedBy,
		GrantedAt:       time.Now(),
		ExpiresAt:       req.ExpiresAt,
		Reason:          req.Reason,
	}

	if err := s.permissionRepo.GrantProjectPermission(ctx, permission); err != nil {
		return fmt.Errorf("failed to grant project permission: %w", err)
	}

	// 记录审计日志
	s.logPermissionChange(ctx, entity.AuditActionGrant, req.ScopeType, req.ScopeID,
		req.PrincipalType, req.PrincipalID, req.PermissionID, nil, &req.PermissionLevel, req.GrantedBy, req.Reason)

	return nil
}

// grantWorkspacePermission 授予工作空间级权限
func (s *PermissionServiceImpl) grantWorkspacePermission(
	ctx context.Context,
	req *GrantPermissionRequest,
) error {
	// 将数字 ID 转换为语义化 ID
	var workspace struct {
		WorkspaceID string `gorm:"column:workspace_id"`
	}
	if err := s.db.Table("workspaces").
		Select("workspace_id").
		Where("id = ?", req.ScopeID).
		First(&workspace).Error; err != nil {
		return fmt.Errorf("failed to get workspace_id: %w", err)
	}

	permission := &entity.WorkspacePermission{
		WorkspaceID:     workspace.WorkspaceID,
		PrincipalType:   req.PrincipalType,
		PrincipalID:     req.PrincipalID,
		PermissionID:    req.PermissionID,
		PermissionLevel: req.PermissionLevel,
		GrantedBy:       &req.GrantedBy,
		GrantedAt:       time.Now(),
		ExpiresAt:       req.ExpiresAt,
		Reason:          req.Reason,
	}

	if err := s.permissionRepo.GrantWorkspacePermission(ctx, permission); err != nil {
		return fmt.Errorf("failed to grant workspace permission: %w", err)
	}

	// 记录审计日志
	s.logPermissionChange(ctx, entity.AuditActionGrant, req.ScopeType, req.ScopeID,
		req.PrincipalType, req.PrincipalID, req.PermissionID, nil, &req.PermissionLevel, req.GrantedBy, req.Reason)

	return nil
}

// RevokePermission 撤销权限
func (s *PermissionServiceImpl) RevokePermission(
	ctx context.Context,
	req *RevokePermissionRequest,
) error {
	// 根据作用域类型撤销权限
	var err error
	switch req.ScopeType {
	case valueobject.ScopeTypeOrganization:
		err = s.permissionRepo.RevokeOrgPermission(ctx, req.AssignmentID)
	case valueobject.ScopeTypeProject:
		err = s.permissionRepo.RevokeProjectPermission(ctx, req.AssignmentID)
	case valueobject.ScopeTypeWorkspace:
		err = s.permissionRepo.RevokeWorkspacePermission(ctx, req.AssignmentID)
	default:
		return fmt.Errorf("unsupported scope type: %s", req.ScopeType)
	}

	if err != nil {
		return fmt.Errorf("failed to revoke permission: %w", err)
	}

	// 记录审计日志
	s.logPermissionChange(ctx, entity.AuditActionRevoke, req.ScopeType, 0,
		"", "", "", nil, nil, req.RevokedBy, req.Reason)

	return nil
}

// ModifyPermission 修改权限等级
func (s *PermissionServiceImpl) ModifyPermission(
	ctx context.Context,
	req *ModifyPermissionRequest,
) error {
	// 根据作用域类型修改权限
	var err error
	switch req.ScopeType {
	case valueobject.ScopeTypeOrganization:
		err = s.permissionRepo.UpdateOrgPermission(ctx, req.AssignmentID, req.NewLevel)
	case valueobject.ScopeTypeProject:
		err = s.permissionRepo.UpdateProjectPermission(ctx, req.AssignmentID, req.NewLevel)
	case valueobject.ScopeTypeWorkspace:
		err = s.permissionRepo.UpdateWorkspacePermission(ctx, req.AssignmentID, req.NewLevel)
	default:
		return fmt.Errorf("unsupported scope type: %s", req.ScopeType)
	}

	if err != nil {
		return fmt.Errorf("failed to modify permission: %w", err)
	}

	// 记录审计日志
	s.logPermissionChange(ctx, entity.AuditActionModify, req.ScopeType, 0,
		"", "", "", nil, &req.NewLevel, req.ModifiedBy, req.Reason)

	return nil
}

// GrantPresetPermissions 授予预设权限集
func (s *PermissionServiceImpl) GrantPresetPermissions(
	ctx context.Context,
	req *GrantPresetRequest,
) error {
	// 1. 验证预设名称
	if req.PresetName != "READ" && req.PresetName != "WRITE" && req.PresetName != "ADMIN" {
		return fmt.Errorf("invalid preset name: %s", req.PresetName)
	}

	// 2. 获取预设包含的权限列表
	presetPerms, err := s.permissionRepo.GetPresetPermissions(ctx, req.PresetName, req.ScopeType)
	if err != nil {
		return fmt.Errorf("failed to get preset permissions: %w", err)
	}

	if len(presetPerms) == 0 {
		return fmt.Errorf("preset %s not found for scope %s", req.PresetName, req.ScopeType)
	}

	// 3. 批量授予权限
	for _, presetPerm := range presetPerms {
		grantReq := &GrantPermissionRequest{
			ScopeType:       req.ScopeType,
			ScopeID:         req.ScopeID,
			PrincipalType:   req.PrincipalType,
			PrincipalID:     req.PrincipalID,
			PermissionID:    presetPerm.PermissionID,
			PermissionLevel: presetPerm.PermissionLevel,
			GrantedBy:       req.GrantedBy,
			Reason:          fmt.Sprintf("Preset: %s - %s", req.PresetName, req.Reason),
		}

		if err := s.GrantPermission(ctx, grantReq); err != nil {
			// 继续授予其他权限，记录错误
			log.Printf("[Permission] Failed to grant permission %s: %v", presetPerm.PermissionID, err)
		}
	}

	return nil
}

// ListPermissions 列出指定作用域的所有权限分配
func (s *PermissionServiceImpl) ListPermissions(
	ctx context.Context,
	scopeType valueobject.ScopeType,
	scopeID uint,
) ([]*entity.PermissionGrant, error) {
	return s.permissionRepo.ListPermissionsByScope(ctx, scopeType, scopeID)
}

// ListPermissionDefinitions 列出所有权限定义
func (s *PermissionServiceImpl) ListPermissionDefinitions(
	ctx context.Context,
) ([]*entity.PermissionDefinition, error) {
	return s.permissionRepo.ListPermissionDefinitions(ctx)
}

// validateGrantRequest 验证授予权限请求
func (s *PermissionServiceImpl) validateGrantRequest(req *GrantPermissionRequest) error {
	if !req.ScopeType.IsValid() {
		return fmt.Errorf("invalid scope_type: %s", req.ScopeType)
	}
	if req.ScopeID == 0 {
		return fmt.Errorf("scope_id is required")
	}
	if !req.PrincipalType.IsValid() {
		return fmt.Errorf("invalid principal_type: %s", req.PrincipalType)
	}
	if req.PrincipalID == "" {
		return fmt.Errorf("principal_id is required")
	}
	if req.PermissionID == "" {
		return fmt.Errorf("permission_id is required")
	}
	if !req.PermissionLevel.IsValid() {
		return fmt.Errorf("invalid permission_level: %d", req.PermissionLevel)
	}
	if req.GrantedBy == "" {
		return fmt.Errorf("granted_by is required")
	}
	return nil
}

// ListPermissionsByPrincipal 列出指定主体的所有权限（跨所有作用域）
func (s *PermissionServiceImpl) ListPermissionsByPrincipal(
	ctx context.Context,
	principalType valueobject.PrincipalType,
	principalID string,
) ([]*entity.PermissionGrant, error) {
	return s.permissionRepo.ListPermissionsByPrincipal(ctx, principalType, principalID)
}

// GetPermissionDefinitionByID 根据语义ID获取权限定义
func (s *PermissionServiceImpl) GetPermissionDefinitionByID(
	ctx context.Context,
	permissionID string,
) (*entity.PermissionDefinition, error) {
	return s.permissionRepo.GetPermissionDefinitionByName(ctx, permissionID)
}

// logPermissionChange 记录权限变更日志（异步）
func (s *PermissionServiceImpl) logPermissionChange(
	ctx context.Context,
	actionType entity.AuditActionType,
	scopeType valueobject.ScopeType,
	scopeID uint,
	principalType valueobject.PrincipalType,
	principalID string,
	permissionID string,
	oldLevel *valueobject.PermissionLevel,
	newLevel *valueobject.PermissionLevel,
	performedBy string,
	reason string,
) {
	go func() {
		var permIDPtr *string
		if permissionID != "" {
			permIDPtr = &permissionID
		}

		log := &entity.PermissionAuditLog{
			ActionType:    actionType,
			ScopeType:     scopeType,
			ScopeID:       scopeID,
			PrincipalType: principalType,
			PrincipalID:   principalID,
			PermissionID:  permIDPtr,
			OldLevel:      oldLevel,
			NewLevel:      newLevel,
			PerformedBy:   performedBy,
			PerformedAt:   time.Now(),
			Reason:        reason,
		}

		_ = s.auditRepo.LogPermissionChange(context.Background(), log)
	}()
}
