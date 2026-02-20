package service

import (
	"context"
	"errors"
	"time"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/repository"
	"iac-platform/internal/infrastructure/persistence"
)

// ApplicationService 应用服务
type ApplicationService struct {
	repo repository.ApplicationRepository
}

// NewApplicationService 创建应用服务实例
func NewApplicationService(repo repository.ApplicationRepository) *ApplicationService {
	return &ApplicationService{
		repo: repo,
	}
}

// CreateApplicationRequest 创建应用请求
type CreateApplicationRequest struct {
	OrgID        uint                   `json:"org_id" binding:"required"`
	Name         string                 `json:"name" binding:"required"`
	Description  string                 `json:"description"`
	CallbackURLs map[string]interface{} `json:"callback_urls"`
	ExpiresAt    *time.Time             `json:"expires_at"`
}

// UpdateApplicationRequest 更新应用请求
type UpdateApplicationRequest struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	CallbackURLs map[string]interface{} `json:"callback_urls"`
	IsActive     *bool                  `json:"is_active"`
	ExpiresAt    *time.Time             `json:"expires_at"`
}

// CreateApplication 创建应用
func (s *ApplicationService) CreateApplication(ctx context.Context, req *CreateApplicationRequest, createdBy string) (*entity.Application, string, error) {
	// 生成AppKey和AppSecret
	appKey := persistence.GenerateAppKey()
	appSecret := persistence.GenerateAppSecret()

	app := &entity.Application{
		OrgID:        req.OrgID,
		Name:         req.Name,
		AppKey:       appKey,
		AppSecret:    appSecret, // 实际应用中应该加密存储
		Description:  req.Description,
		CallbackURLs: req.CallbackURLs,
		IsActive:     true,
		CreatedBy:    &createdBy,
		ExpiresAt:    req.ExpiresAt,
	}

	if err := s.repo.Create(ctx, app); err != nil {
		return nil, "", err
	}

	// 返回明文密钥（仅此一次）
	return app, appSecret, nil
}

// GetApplication 获取应用详情
func (s *ApplicationService) GetApplication(ctx context.Context, id uint) (*entity.Application, error) {
	return s.repo.GetByID(ctx, id)
}

// ListApplications 获取应用列表
func (s *ApplicationService) ListApplications(ctx context.Context, orgID uint, isActive *bool) ([]*entity.Application, error) {
	return s.repo.ListByOrg(ctx, orgID, isActive)
}

// UpdateApplication 更新应用
func (s *ApplicationService) UpdateApplication(ctx context.Context, id uint, req *UpdateApplicationRequest) error {
	app, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// 更新字段
	if req.Name != "" {
		app.Name = req.Name
	}
	if req.Description != "" {
		app.Description = req.Description
	}
	if req.CallbackURLs != nil {
		app.CallbackURLs = req.CallbackURLs
	}
	if req.IsActive != nil {
		app.IsActive = *req.IsActive
	}
	if req.ExpiresAt != nil {
		app.ExpiresAt = req.ExpiresAt
	}

	return s.repo.Update(ctx, app)
}

// DeleteApplication 删除应用
func (s *ApplicationService) DeleteApplication(ctx context.Context, id uint) error {
	return s.repo.Delete(ctx, id)
}

// RegenerateSecret 重新生成密钥
func (s *ApplicationService) RegenerateSecret(ctx context.Context, id uint) (string, error) {
	app, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return "", err
	}

	if !app.IsActive {
		return "", errors.New("cannot regenerate secret for inactive application")
	}

	newSecret := persistence.GenerateAppSecret()
	if err := s.repo.RegenerateSecret(ctx, id, newSecret); err != nil {
		return "", err
	}

	return newSecret, nil
}

// ValidateApplication 验证应用
func (s *ApplicationService) ValidateApplication(ctx context.Context, appKey, appSecret string) (*entity.Application, error) {
	app, err := s.repo.GetByAppKey(ctx, appKey)
	if err != nil {
		return nil, errors.New("invalid application credentials")
	}

	if !app.IsActive {
		return nil, errors.New("application is inactive")
	}

	if app.IsExpired() {
		return nil, errors.New("application has expired")
	}

	if app.AppSecret != appSecret {
		return nil, errors.New("invalid application credentials")
	}

	// 更新最后使用时间
	_ = s.repo.UpdateLastUsed(ctx, app.ID)

	return app, nil
}
