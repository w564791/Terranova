package persistence

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/repository"
	"iac-platform/internal/domain/valueobject"
)

// AuditRepositoryImpl 审计日志仓储GORM实现
type AuditRepositoryImpl struct {
	db *gorm.DB
}

// NewAuditRepository 创建审计日志仓储实例
func NewAuditRepository(db *gorm.DB) repository.AuditRepository {
	return &AuditRepositoryImpl{db: db}
}

// LogPermissionChange 记录权限变更日志
func (r *AuditRepositoryImpl) LogPermissionChange(ctx context.Context, log *entity.PermissionAuditLog) error {
	if err := r.db.WithContext(ctx).Create(log).Error; err != nil {
		return fmt.Errorf("failed to log permission change: %w", err)
	}
	return nil
}

// LogResourceAccess 记录资源访问日志
func (r *AuditRepositoryImpl) LogResourceAccess(ctx context.Context, log *entity.AccessLog) error {
	// 如果IP地址为空字符串，设置为nil以避免inet类型错误
	if log.IPAddress == "" {
		log.IPAddress = "0.0.0.0"
	}

	// 如果request_headers为空字符串，设置为null以避免jsonb类型错误
	if log.RequestHeaders == "" {
		log.RequestHeaders = "null"
	}

	result := r.db.WithContext(ctx).Create(log)
	if result.Error != nil {
		return fmt.Errorf("failed to log resource access: %w", result.Error)
	}

	// 调试：打印插入结果
	// fmt.Printf("[AuditRepo] Inserted %d rows, Log ID: %d\n", result.RowsAffected, log.ID)

	return nil
}

// QueryPermissionHistory 查询权限变更历史
func (r *AuditRepositoryImpl) QueryPermissionHistory(
	ctx context.Context,
	scopeType valueobject.ScopeType,
	scopeID uint,
	startTime time.Time,
	endTime time.Time,
	limit int,
) ([]*entity.PermissionAuditLog, error) {
	var logs []*entity.PermissionAuditLog

	query := r.db.WithContext(ctx).
		Where("scope_type = ? AND scope_id = ?", scopeType, scopeID).
		Where("performed_at BETWEEN ? AND ?", startTime, endTime)

	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Order("performed_at DESC").Find(&logs).Error; err != nil {
		return nil, fmt.Errorf("failed to query permission history: %w", err)
	}

	return logs, nil
}

// QueryAccessHistory 查询资源访问历史
func (r *AuditRepositoryImpl) QueryAccessHistory(
	ctx context.Context,
	userID string,
	resourceType string,
	method string,
	httpCodeOperator string,
	httpCodeValue int,
	startTime time.Time,
	endTime time.Time,
	limit int,
) ([]*entity.AccessLog, error) {
	// 使用自定义结构体来接收JOIN后的结果
	type AccessLogWithUsername struct {
		entity.AccessLog
		Username string `json:"username"`
	}

	var results []AccessLogWithUsername

	query := r.db.WithContext(ctx).
		Table("access_logs").
		Select("access_logs.*, users.username").
		Joins("LEFT JOIN users ON access_logs.user_id = users.user_id").
		Where("access_logs.accessed_at BETWEEN ? AND ?", startTime, endTime)

	// user_id为可选参数
	if userID != "" {
		query = query.Where("access_logs.user_id = ?", userID)
	}

	if resourceType != "" {
		query = query.Where("access_logs.resource_type = ?", resourceType)
	}

	// Method筛选
	if method != "" {
		query = query.Where("method = ?", method)
	}

	// HTTP状态码筛选（支持比较运算符）
	if httpCodeOperator != "" && httpCodeValue > 0 {
		switch httpCodeOperator {
		case "=":
			query = query.Where("access_logs.http_code = ?", httpCodeValue)
		case "!=":
			query = query.Where("access_logs.http_code != ?", httpCodeValue)
		case ">":
			query = query.Where("access_logs.http_code > ?", httpCodeValue)
		case "<":
			query = query.Where("access_logs.http_code < ?", httpCodeValue)
		case ">=":
			query = query.Where("access_logs.http_code >= ?", httpCodeValue)
		case "<=":
			query = query.Where("access_logs.http_code <= ?", httpCodeValue)
		}
	}

	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Order("access_logs.accessed_at DESC").Find(&results).Error; err != nil {
		return nil, fmt.Errorf("failed to query access history: %w", err)
	}

	// 转换为AccessLog并添加username到UserID字段（临时方案）
	logs := make([]*entity.AccessLog, len(results))
	for i, result := range results {
		logs[i] = &result.AccessLog
		// 如果有username，用username替换user_id显示
		if result.Username != "" {
			logs[i].UserID = result.Username
		}
	}

	return logs, nil
}

// QueryDeniedAccess 查询被拒绝的访问记录
func (r *AuditRepositoryImpl) QueryDeniedAccess(
	ctx context.Context,
	startTime time.Time,
	endTime time.Time,
	limit int,
) ([]*entity.AccessLog, error) {
	// 使用自定义结构体来接收JOIN后的结果
	type AccessLogWithUsername struct {
		entity.AccessLog
		Username string `json:"username"`
	}

	var results []AccessLogWithUsername

	query := r.db.WithContext(ctx).
		Table("access_logs").
		Select("access_logs.*, users.username").
		Joins("LEFT JOIN users ON access_logs.user_id = users.user_id").
		Where("access_logs.is_allowed = ?", false).
		Where("access_logs.accessed_at BETWEEN ? AND ?", startTime, endTime)

	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Order("access_logs.accessed_at DESC").Find(&results).Error; err != nil {
		return nil, fmt.Errorf("failed to query denied access: %w", err)
	}

	// 转换为AccessLog并添加username到UserID字段（临时方案）
	logs := make([]*entity.AccessLog, len(results))
	for i, result := range results {
		logs[i] = &result.AccessLog
		// 如果有username，用username替换user_id显示
		if result.Username != "" {
			logs[i].UserID = result.Username
		}
	}

	return logs, nil
}

// QueryPermissionChangesByPrincipal 查询指定主体的权限变更历史
func (r *AuditRepositoryImpl) QueryPermissionChangesByPrincipal(
	ctx context.Context,
	principalType valueobject.PrincipalType,
	principalID string,
	startTime time.Time,
	endTime time.Time,
	limit int,
) ([]*entity.PermissionAuditLog, error) {
	var logs []*entity.PermissionAuditLog

	query := r.db.WithContext(ctx).
		Where("principal_type = ? AND principal_id = ?", principalType, principalID).
		Where("performed_at BETWEEN ? AND ?", startTime, endTime)

	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Order("performed_at DESC").Find(&logs).Error; err != nil {
		return nil, fmt.Errorf("failed to query permission changes by principal: %w", err)
	}

	return logs, nil
}

// QueryPermissionChangesByPerformer 查询指定操作人的权限变更历史
func (r *AuditRepositoryImpl) QueryPermissionChangesByPerformer(
	ctx context.Context,
	performerID string,
	startTime time.Time,
	endTime time.Time,
	limit int,
) ([]*entity.PermissionAuditLog, error) {
	var logs []*entity.PermissionAuditLog

	query := r.db.WithContext(ctx).
		Where("performed_by = ?", performerID).
		Where("performed_at BETWEEN ? AND ?", startTime, endTime)

	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Order("performed_at DESC").Find(&logs).Error; err != nil {
		return nil, fmt.Errorf("failed to query permission changes by performer: %w", err)
	}

	return logs, nil
}
