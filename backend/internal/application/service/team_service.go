package service

import (
	"context"
	"fmt"
	"time"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/repository"
)

// CreateTeamRequest 创建团队请求
type CreateTeamRequest struct {
	OrgID       uint   `json:"org_id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	CreatedBy   string `json:"created_by"` // user_id
}

// AddTeamMemberRequest 添加团队成员请求
type AddTeamMemberRequest struct {
	TeamID  string          `json:"team_id"`
	UserID  string          `json:"user_id"`
	Role    entity.TeamRole `json:"role"`
	AddedBy string          `json:"added_by"` // user_id
}

// TeamService 团队管理服务接口
type TeamService interface {
	// CreateTeam 创建团队
	CreateTeam(ctx context.Context, req *CreateTeamRequest) (*entity.Team, error)

	// GetTeam 获取团队详情
	GetTeam(ctx context.Context, teamID string) (*entity.Team, error)

	// ListTeamsByOrg 列出组织的所有团队
	ListTeamsByOrg(ctx context.Context, orgID uint) ([]*entity.Team, error)

	// UpdateTeam 更新团队信息
	UpdateTeam(ctx context.Context, team *entity.Team) error

	// DeleteTeam 删除团队
	DeleteTeam(ctx context.Context, teamID string, deletedBy string) error

	// AddTeamMember 添加团队成员
	AddTeamMember(ctx context.Context, req *AddTeamMemberRequest) error

	// RemoveTeamMember 移除团队成员
	RemoveTeamMember(ctx context.Context, teamID string, userID string, removedBy string) error

	// UpdateMemberRole 更新成员角色
	UpdateMemberRole(ctx context.Context, teamID string, userID string, role entity.TeamRole, updatedBy string) error

	// ListTeamMembers 列出团队成员
	ListTeamMembers(ctx context.Context, teamID string) ([]*entity.TeamMember, error)

	// ListUserTeams 列出用户所属的所有团队
	ListUserTeams(ctx context.Context, userID string, orgID uint) ([]*entity.Team, error)
}

// TeamServiceImpl 团队管理服务实现
type TeamServiceImpl struct {
	teamRepo  repository.TeamRepository
	orgRepo   repository.OrganizationRepository
	auditRepo repository.AuditRepository
	// cache     cache.PermissionCache // TODO: 实现缓存
}

// NewTeamService 创建团队管理服务实例
func NewTeamService(
	teamRepo repository.TeamRepository,
	orgRepo repository.OrganizationRepository,
	auditRepo repository.AuditRepository,
) TeamService {
	return &TeamServiceImpl{
		teamRepo:  teamRepo,
		orgRepo:   orgRepo,
		auditRepo: auditRepo,
	}
}

// CreateTeam 创建团队
func (s *TeamServiceImpl) CreateTeam(
	ctx context.Context,
	req *CreateTeamRequest,
) (*entity.Team, error) {
	// 1. 验证请求参数
	if err := s.validateCreateTeamRequest(req); err != nil {
		return nil, err
	}

	// 2. 验证组织是否存在
	_, err := s.orgRepo.GetOrganizationByID(ctx, req.OrgID)
	if err != nil {
		return nil, fmt.Errorf("organization not found: %w", err)
	}

	// 3. 检查团队名称是否已存在
	existing, _ := s.teamRepo.GetTeamByName(ctx, req.OrgID, req.Name)
	if existing != nil {
		return nil, fmt.Errorf("team name already exists in organization")
	}

	// 4. 创建团队
	team := &entity.Team{
		OrgID:       req.OrgID,
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		IsSystem:    false,
		CreatedBy:   &req.CreatedBy,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.teamRepo.CreateTeam(ctx, team); err != nil {
		return nil, fmt.Errorf("failed to create team: %w", err)
	}

	return team, nil
}

// GetTeam 获取团队详情
func (s *TeamServiceImpl) GetTeam(ctx context.Context, teamID string) (*entity.Team, error) {
	return s.teamRepo.GetTeamByID(ctx, teamID)
}

// ListTeamsByOrg 列出组织的所有团队
func (s *TeamServiceImpl) ListTeamsByOrg(ctx context.Context, orgID uint) ([]*entity.Team, error) {
	return s.teamRepo.ListTeamsByOrg(ctx, orgID)
}

// UpdateTeam 更新团队信息
func (s *TeamServiceImpl) UpdateTeam(ctx context.Context, team *entity.Team) error {
	// 验证团队是否存在
	existing, err := s.teamRepo.GetTeamByID(ctx, team.ID)
	if err != nil {
		return fmt.Errorf("team not found: %w", err)
	}

	// 系统团队不允许修改名称
	if existing.IsSystem && existing.Name != team.Name {
		return fmt.Errorf("cannot modify system team name")
	}

	team.UpdatedAt = time.Now()
	return s.teamRepo.UpdateTeam(ctx, team)
}

// DeleteTeam 删除团队
func (s *TeamServiceImpl) DeleteTeam(ctx context.Context, teamID string, deletedBy string) error {
	// 1. 验证团队是否存在
	team, err := s.teamRepo.GetTeamByID(ctx, teamID)
	if err != nil {
		return fmt.Errorf("team not found: %w", err)
	}

	// 2. 系统团队不允许删除
	if team.IsSystem {
		return fmt.Errorf("cannot delete system team")
	}

	// 3. 删除团队（会级联删除成员关系和权限）
	if err := s.teamRepo.DeleteTeam(ctx, teamID); err != nil {
		return fmt.Errorf("failed to delete team: %w", err)
	}

	// TODO: 使团队相关缓存失效

	return nil
}

// AddTeamMember 添加团队成员
func (s *TeamServiceImpl) AddTeamMember(
	ctx context.Context,
	req *AddTeamMemberRequest,
) error {
	// 1. 验证请求参数
	if err := s.validateAddMemberRequest(req); err != nil {
		return err
	}

	// 2. 验证团队是否存在
	_, err := s.teamRepo.GetTeamByID(ctx, req.TeamID)
	if err != nil {
		return fmt.Errorf("team not found: %w", err)
	}

	// 3. 检查用户是否已是成员
	isMember, err := s.teamRepo.IsMember(ctx, req.TeamID, req.UserID)
	if err != nil {
		return fmt.Errorf("failed to check membership: %w", err)
	}
	if isMember {
		return fmt.Errorf("user is already a team member")
	}

	// 4. 添加成员
	member := &entity.TeamMember{
		TeamID:   req.TeamID,
		UserID:   req.UserID,
		Role:     req.Role,
		JoinedAt: time.Now(),
		JoinedBy: &req.AddedBy,
	}

	if err := s.teamRepo.AddMember(ctx, member); err != nil {
		return fmt.Errorf("failed to add team member: %w", err)
	}

	// TODO: 使用户权限缓存失效

	return nil
}

// RemoveTeamMember 移除团队成员
func (s *TeamServiceImpl) RemoveTeamMember(
	ctx context.Context,
	teamID string,
	userID string,
	removedBy string,
) error {
	// 1. 验证团队是否存在
	_, err := s.teamRepo.GetTeamByID(ctx, teamID)
	if err != nil {
		return fmt.Errorf("team not found: %w", err)
	}

	// 2. 检查用户是否是成员
	isMember, err := s.teamRepo.IsMember(ctx, teamID, userID)
	if err != nil {
		return fmt.Errorf("failed to check membership: %w", err)
	}
	if !isMember {
		return fmt.Errorf("user is not a team member")
	}

	// 3. 移除成员
	if err := s.teamRepo.RemoveMember(ctx, teamID, userID); err != nil {
		return fmt.Errorf("failed to remove team member: %w", err)
	}

	// TODO: 使用户权限缓存失效

	return nil
}

// UpdateMemberRole 更新成员角色
func (s *TeamServiceImpl) UpdateMemberRole(
	ctx context.Context,
	teamID string,
	userID string,
	role entity.TeamRole,
	updatedBy string,
) error {
	// 1. 验证角色是否有效
	if !role.IsValid() {
		return fmt.Errorf("invalid role: %s", role)
	}

	// 2. 验证用户是否是成员
	isMember, err := s.teamRepo.IsMember(ctx, teamID, userID)
	if err != nil {
		return fmt.Errorf("failed to check membership: %w", err)
	}
	if !isMember {
		return fmt.Errorf("user is not a team member")
	}

	// 3. 更新角色
	if err := s.teamRepo.UpdateMemberRole(ctx, teamID, userID, role); err != nil {
		return fmt.Errorf("failed to update member role: %w", err)
	}

	return nil
}

// ListTeamMembers 列出团队成员
func (s *TeamServiceImpl) ListTeamMembers(ctx context.Context, teamID string) ([]*entity.TeamMember, error) {
	return s.teamRepo.ListMembers(ctx, teamID)
}

// ListUserTeams 列出用户所属的所有团队
func (s *TeamServiceImpl) ListUserTeams(ctx context.Context, userID string, orgID uint) ([]*entity.Team, error) {
	return s.teamRepo.GetUserTeamsInOrg(ctx, userID, orgID)
}

// validateCreateTeamRequest 验证创建团队请求
func (s *TeamServiceImpl) validateCreateTeamRequest(req *CreateTeamRequest) error {
	if req.OrgID == 0 {
		return fmt.Errorf("org_id is required")
	}
	if req.Name == "" {
		return fmt.Errorf("name is required")
	}
	if len(req.Name) > 100 {
		return fmt.Errorf("name too long (max 100 characters)")
	}
	if req.CreatedBy == "" {
		return fmt.Errorf("created_by is required")
	}
	return nil
}

// validateAddMemberRequest 验证添加成员请求
func (s *TeamServiceImpl) validateAddMemberRequest(req *AddTeamMemberRequest) error {
	if req.TeamID == "" {
		return fmt.Errorf("team_id is required")
	}
	if req.UserID == "" {
		return fmt.Errorf("user_id is required")
	}
	if !req.Role.IsValid() {
		return fmt.Errorf("invalid role: %s", req.Role)
	}
	if req.AddedBy == "" {
		return fmt.Errorf("added_by is required")
	}
	return nil
}
