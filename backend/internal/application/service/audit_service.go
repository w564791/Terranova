package service

import (
	"context"
	"time"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/repository"
	"iac-platform/internal/domain/valueobject"
)

// AuditService 审计服务
type AuditService struct {
	repo repository.AuditRepository
}

// NewAuditService 创建审计服务实例
func NewAuditService(repo repository.AuditRepository) *AuditService {
	return &AuditService{
		repo: repo,
	}
}

// QueryPermissionHistoryRequest 查询权限历史请求
type QueryPermissionHistoryRequest struct {
	ScopeType valueobject.ScopeType `json:"scope_type" binding:"required"`
	ScopeID   uint                  `json:"scope_id" binding:"required"`
	StartTime time.Time             `json:"start_time"`
	EndTime   time.Time             `json:"end_time"`
	Limit     int                   `json:"limit"`
}

// QueryAccessHistoryRequest 查询访问历史请求
type QueryAccessHistoryRequest struct {
	UserID         string          `json:"user_id"`
	ResourceType   string          `json:"resource_type"`
	Method         string          `json:"method"`
	HttpCodeFilter *HttpCodeFilter `json:"http_code_filter"`
	StartTime      time.Time       `json:"start_time"`
	EndTime        time.Time       `json:"end_time"`
	Limit          int             `json:"limit"`
}

// HttpCodeFilter HTTP状态码筛选条件
type HttpCodeFilter struct {
	Operator string `json:"operator"` // =, !=, >, <, >=, <=
	Value    int    `json:"value"`
}

// QueryDeniedAccessRequest 查询被拒绝访问请求
type QueryDeniedAccessRequest struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Limit     int       `json:"limit"`
}

// QueryPermissionChangesByPrincipalRequest 查询主体权限变更请求
type QueryPermissionChangesByPrincipalRequest struct {
	PrincipalType valueobject.PrincipalType `json:"principal_type" binding:"required"`
	PrincipalID   string                    `json:"principal_id" binding:"required"`
	StartTime     time.Time                 `json:"start_time"`
	EndTime       time.Time                 `json:"end_time"`
	Limit         int                       `json:"limit"`
}

// QueryPermissionChangesByPerformerRequest 查询操作人权限变更请求
type QueryPermissionChangesByPerformerRequest struct {
	PerformerID string    `json:"performer_id" binding:"required"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	Limit       int       `json:"limit"`
}

// QueryPermissionHistory 查询权限变更历史
func (s *AuditService) QueryPermissionHistory(ctx context.Context, req *QueryPermissionHistoryRequest) ([]*entity.PermissionAuditLog, error) {
	// 设置默认值
	if req.Limit <= 0 || req.Limit > 1000 {
		req.Limit = 100
	}
	if req.StartTime.IsZero() {
		req.StartTime = time.Now().AddDate(0, -1, 0) // 默认查询最近1个月
	}
	if req.EndTime.IsZero() {
		req.EndTime = time.Now()
	}

	return s.repo.QueryPermissionHistory(ctx, req.ScopeType, req.ScopeID, req.StartTime, req.EndTime, req.Limit)
}

// QueryAccessHistory 查询资源访问历史
func (s *AuditService) QueryAccessHistory(ctx context.Context, req *QueryAccessHistoryRequest) ([]*entity.AccessLog, error) {
	// 设置默认值
	if req.Limit <= 0 || req.Limit > 1000 {
		req.Limit = 100
	}
	if req.StartTime.IsZero() {
		req.StartTime = time.Now().AddDate(0, -1, 0) // 默认查询最近1个月
	}
	if req.EndTime.IsZero() {
		req.EndTime = time.Now()
	}

	// 处理HTTP状态码筛选
	httpCodeOperator := ""
	httpCodeValue := 0
	if req.HttpCodeFilter != nil {
		httpCodeOperator = req.HttpCodeFilter.Operator
		httpCodeValue = req.HttpCodeFilter.Value
	}

	return s.repo.QueryAccessHistory(ctx, req.UserID, req.ResourceType, req.Method, httpCodeOperator, httpCodeValue, req.StartTime, req.EndTime, req.Limit)
}

// QueryDeniedAccess 查询被拒绝的访问记录
func (s *AuditService) QueryDeniedAccess(ctx context.Context, req *QueryDeniedAccessRequest) ([]*entity.AccessLog, error) {
	// 设置默认值
	if req.Limit <= 0 || req.Limit > 1000 {
		req.Limit = 100
	}
	if req.StartTime.IsZero() {
		req.StartTime = time.Now().AddDate(0, -1, 0) // 默认查询最近1个月
	}
	if req.EndTime.IsZero() {
		req.EndTime = time.Now()
	}

	return s.repo.QueryDeniedAccess(ctx, req.StartTime, req.EndTime, req.Limit)
}

// QueryPermissionChangesByPrincipal 查询指定主体的权限变更历史
func (s *AuditService) QueryPermissionChangesByPrincipal(ctx context.Context, req *QueryPermissionChangesByPrincipalRequest) ([]*entity.PermissionAuditLog, error) {
	// 设置默认值
	if req.Limit <= 0 || req.Limit > 1000 {
		req.Limit = 100
	}
	if req.StartTime.IsZero() {
		req.StartTime = time.Now().AddDate(0, -1, 0) // 默认查询最近1个月
	}
	if req.EndTime.IsZero() {
		req.EndTime = time.Now()
	}

	return s.repo.QueryPermissionChangesByPrincipal(ctx, req.PrincipalType, req.PrincipalID, req.StartTime, req.EndTime, req.Limit)
}

// QueryPermissionChangesByPerformer 查询指定操作人的权限变更历史
func (s *AuditService) QueryPermissionChangesByPerformer(ctx context.Context, req *QueryPermissionChangesByPerformerRequest) ([]*entity.PermissionAuditLog, error) {
	// 设置默认值
	if req.Limit <= 0 || req.Limit > 1000 {
		req.Limit = 100
	}
	if req.StartTime.IsZero() {
		req.StartTime = time.Now().AddDate(0, -1, 0) // 默认查询最近1个月
	}
	if req.EndTime.IsZero() {
		req.EndTime = time.Now()
	}

	return s.repo.QueryPermissionChangesByPerformer(ctx, req.PerformerID, req.StartTime, req.EndTime, req.Limit)
}
