package persistence

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/repository"
)

// TeamRepositoryImpl 团队仓储GORM实现
type TeamRepositoryImpl struct {
	db *gorm.DB
}

// NewTeamRepository 创建团队仓储实例
func NewTeamRepository(db *gorm.DB) repository.TeamRepository {
	return &TeamRepositoryImpl{db: db}
}

// CreateTeam 创建团队
func (r *TeamRepositoryImpl) CreateTeam(ctx context.Context, team *entity.Team) error {
	if err := r.db.WithContext(ctx).Create(team).Error; err != nil {
		return fmt.Errorf("failed to create team: %w", err)
	}
	return nil
}

// GetTeamByID 根据ID获取团队
func (r *TeamRepositoryImpl) GetTeamByID(ctx context.Context, id string) (*entity.Team, error) {
	var team entity.Team
	if err := r.db.WithContext(ctx).Where("team_id = ?", id).First(&team).Error; err != nil {
		return nil, fmt.Errorf("team not found: %w", err)
	}
	return &team, nil
}

// GetTeamByName 根据组织ID和名称获取团队
func (r *TeamRepositoryImpl) GetTeamByName(ctx context.Context, orgID uint, name string) (*entity.Team, error) {
	var team entity.Team
	if err := r.db.WithContext(ctx).
		Where("org_id = ? AND name = ?", orgID, name).
		First(&team).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get team by name: %w", err)
	}
	return &team, nil
}

// ListTeamsByOrg 列出组织的所有团队
func (r *TeamRepositoryImpl) ListTeamsByOrg(ctx context.Context, orgID uint) ([]*entity.Team, error) {
	var teams []*entity.Team
	if err := r.db.WithContext(ctx).
		Where("org_id = ?", orgID).
		Order("is_system DESC, name ASC").
		Find(&teams).Error; err != nil {
		return nil, fmt.Errorf("failed to list teams: %w", err)
	}
	return teams, nil
}

// UpdateTeam 更新团队信息
func (r *TeamRepositoryImpl) UpdateTeam(ctx context.Context, team *entity.Team) error {
	if err := r.db.WithContext(ctx).Save(team).Error; err != nil {
		return fmt.Errorf("failed to update team: %w", err)
	}
	return nil
}

// DeleteTeam 删除团队
func (r *TeamRepositoryImpl) DeleteTeam(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).Where("team_id = ?", id).Delete(&entity.Team{}).Error; err != nil {
		return fmt.Errorf("failed to delete team: %w", err)
	}
	return nil
}

// AddMember 添加团队成员
func (r *TeamRepositoryImpl) AddMember(ctx context.Context, member *entity.TeamMember) error {
	if err := r.db.WithContext(ctx).Create(member).Error; err != nil {
		return fmt.Errorf("failed to add team member: %w", err)
	}
	return nil
}

// RemoveMember 移除团队成员
func (r *TeamRepositoryImpl) RemoveMember(ctx context.Context, teamID string, userID string) error {
	if err := r.db.WithContext(ctx).
		Where("team_id = ? AND user_id = ?", teamID, userID).
		Delete(&entity.TeamMember{}).Error; err != nil {
		return fmt.Errorf("failed to remove team member: %w", err)
	}
	return nil
}

// UpdateMemberRole 更新成员角色
func (r *TeamRepositoryImpl) UpdateMemberRole(ctx context.Context, teamID string, userID string, role entity.TeamRole) error {
	if err := r.db.WithContext(ctx).Model(&entity.TeamMember{}).
		Where("team_id = ? AND user_id = ?", teamID, userID).
		Update("role", role).Error; err != nil {
		return fmt.Errorf("failed to update member role: %w", err)
	}
	return nil
}

// ListMembers 列出团队成员
func (r *TeamRepositoryImpl) ListMembers(ctx context.Context, teamID string) ([]*entity.TeamMember, error) {
	var members []*entity.TeamMember
	if err := r.db.WithContext(ctx).
		Where("team_id = ?", teamID).
		Order("role DESC, joined_at ASC").
		Find(&members).Error; err != nil {
		return nil, fmt.Errorf("failed to list team members: %w", err)
	}
	return members, nil
}

// GetUserTeams 获取用户所属的所有团队ID
func (r *TeamRepositoryImpl) GetUserTeams(ctx context.Context, userID string) ([]string, error) {
	var teamIDs []string
	if err := r.db.WithContext(ctx).
		Model(&entity.TeamMember{}).
		Where("user_id = ?", userID).
		Pluck("team_id", &teamIDs).Error; err != nil {
		return nil, fmt.Errorf("failed to get user teams: %w", err)
	}
	return teamIDs, nil
}

// GetUserTeamsInOrg 获取用户在指定组织中的所有团队
func (r *TeamRepositoryImpl) GetUserTeamsInOrg(ctx context.Context, userID string, orgID uint) ([]*entity.Team, error) {
	var teams []*entity.Team
	if err := r.db.WithContext(ctx).
		Joins("JOIN team_members ON team_members.team_id = teams.team_id").
		Where("team_members.user_id = ? AND teams.org_id = ?", userID, orgID).
		Find(&teams).Error; err != nil {
		return nil, fmt.Errorf("failed to get user teams in org: %w", err)
	}
	return teams, nil
}

// IsMember 判断用户是否是团队成员
func (r *TeamRepositoryImpl) IsMember(ctx context.Context, teamID string, userID string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&entity.TeamMember{}).
		Where("team_id = ? AND user_id = ?", teamID, userID).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to check membership: %w", err)
	}
	return count > 0, nil
}

// IsMaintainer 判断用户是否是团队维护者
func (r *TeamRepositoryImpl) IsMaintainer(ctx context.Context, teamID string, userID string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&entity.TeamMember{}).
		Where("team_id = ? AND user_id = ? AND role = ?", teamID, userID, entity.TeamRoleMaintainer).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to check maintainer status: %w", err)
	}
	return count > 0, nil
}

// BatchGetUserTeams 批量获取用户团队
func (r *TeamRepositoryImpl) BatchGetUserTeams(ctx context.Context, userIDs []string) (map[string][]string, error) {
	if len(userIDs) == 0 {
		return make(map[string][]string), nil
	}

	var members []entity.TeamMember
	if err := r.db.WithContext(ctx).
		Where("user_id IN ?", userIDs).
		Find(&members).Error; err != nil {
		return nil, fmt.Errorf("failed to batch get user teams: %w", err)
	}

	result := make(map[string][]string)
	for _, member := range members {
		result[member.UserID] = append(result[member.UserID], member.TeamID)
	}

	return result, nil
}
