package repository

import (
	"context"

	"iac-platform/internal/domain/entity"
)

// TeamRepository 团队仓储接口
type TeamRepository interface {
	// CreateTeam 创建团队
	CreateTeam(ctx context.Context, team *entity.Team) error

	// GetTeamByID 根据ID获取团队
	GetTeamByID(ctx context.Context, id string) (*entity.Team, error)

	// GetTeamByName 根据组织ID和名称获取团队
	GetTeamByName(ctx context.Context, orgID uint, name string) (*entity.Team, error)

	// ListTeamsByOrg 列出组织的所有团队
	ListTeamsByOrg(ctx context.Context, orgID uint) ([]*entity.Team, error)

	// UpdateTeam 更新团队信息
	UpdateTeam(ctx context.Context, team *entity.Team) error

	// DeleteTeam 删除团队
	DeleteTeam(ctx context.Context, id string) error

	// AddMember 添加团队成员
	AddMember(ctx context.Context, member *entity.TeamMember) error

	// RemoveMember 移除团队成员
	RemoveMember(ctx context.Context, teamID string, userID string) error

	// UpdateMemberRole 更新成员角色
	UpdateMemberRole(ctx context.Context, teamID string, userID string, role entity.TeamRole) error

	// ListMembers 列出团队成员
	ListMembers(ctx context.Context, teamID string) ([]*entity.TeamMember, error)

	// GetUserTeams 获取用户所属的所有团队ID
	GetUserTeams(ctx context.Context, userID string) ([]string, error)

	// GetUserTeamsInOrg 获取用户在指定组织中的所有团队
	GetUserTeamsInOrg(ctx context.Context, userID string, orgID uint) ([]*entity.Team, error)

	// IsMember 判断用户是否是团队成员
	IsMember(ctx context.Context, teamID string, userID string) (bool, error)

	// IsMaintainer 判断用户是否是团队维护者
	IsMaintainer(ctx context.Context, teamID string, userID string) (bool, error)

	// BatchGetUserTeams 批量获取用户团队（用于性能优化）
	BatchGetUserTeams(ctx context.Context, userIDs []string) (map[string][]string, error)
}
