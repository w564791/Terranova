package repository

import (
	"context"
	"time"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/valueobject"
)

// AuditRepository 审计日志仓储接口
type AuditRepository interface {
	// LogPermissionChange 记录权限变更日志
	LogPermissionChange(ctx context.Context, log *entity.PermissionAuditLog) error

	// LogResourceAccess 记录资源访问日志
	LogResourceAccess(ctx context.Context, log *entity.AccessLog) error

	// QueryPermissionHistory 查询权限变更历史
	QueryPermissionHistory(
		ctx context.Context,
		scopeType valueobject.ScopeType,
		scopeID uint,
		startTime time.Time,
		endTime time.Time,
		limit int,
	) ([]*entity.PermissionAuditLog, error)

	// QueryAccessHistory 查询资源访问历史
	QueryAccessHistory(
		ctx context.Context,
		userID string,
		resourceType string,
		method string,
		httpCodeOperator string,
		httpCodeValue int,
		startTime time.Time,
		endTime time.Time,
		limit int,
	) ([]*entity.AccessLog, error)

	// QueryDeniedAccess 查询被拒绝的访问记录
	QueryDeniedAccess(
		ctx context.Context,
		startTime time.Time,
		endTime time.Time,
		limit int,
	) ([]*entity.AccessLog, error)

	// QueryPermissionChangesByPrincipal 查询指定主体的权限变更历史
	QueryPermissionChangesByPrincipal(
		ctx context.Context,
		principalType valueobject.PrincipalType,
		principalID string,
		startTime time.Time,
		endTime time.Time,
		limit int,
	) ([]*entity.PermissionAuditLog, error)

	// QueryPermissionChangesByPerformer 查询指定操作人的权限变更历史
	QueryPermissionChangesByPerformer(
		ctx context.Context,
		performerID string,
		startTime time.Time,
		endTime time.Time,
		limit int,
	) ([]*entity.PermissionAuditLog, error)
}
