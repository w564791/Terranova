package service

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/repository"
	"iac-platform/internal/infrastructure/persistence"
)

const appSecretHashPrefix = "sha256:"

// Application 组织隔离错误
var (
	ErrApplicationNotFound     = errors.New("application not found")
	ErrApplicationOrgMismatch  = errors.New("application org mismatch")
	ErrApplicationOrgForbidden = errors.New("application operation forbidden for organization")
)

// hashAppSecret 对明文 secret 做 SHA-256 后存储（不可逆；仅创建/轮换时返回明文）
func hashAppSecret(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return appSecretHashPrefix + hex.EncodeToString(sum[:])
}

// verifyAppSecret 校验明文；兼容历史明文存储与 sha256: 哈希
func verifyAppSecret(stored, plain string) bool {
	if stored == "" || plain == "" {
		return false
	}
	if strings.HasPrefix(stored, appSecretHashPrefix) {
		want := hashAppSecret(plain)
		return subtle.ConstantTimeCompare([]byte(stored), []byte(want)) == 1
	}
	// legacy plaintext
	return subtle.ConstantTimeCompare([]byte(stored), []byte(plain)) == 1
}

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
	OrgID              uint                   `json:"org_id" binding:"required"`
	Name               string                 `json:"name" binding:"required"`
	Description        string                 `json:"description"`
	CallbackURLs       map[string]interface{} `json:"callback_urls"`
	WorkspaceTagFilter map[string]interface{} `json:"workspace_tag_filter"`
	ExpiresAt          *time.Time             `json:"expires_at"`
}

// UpdateApplicationRequest 更新应用请求
type UpdateApplicationRequest struct {
	Name               string                 `json:"name"`
	Description        string                 `json:"description"`
	CallbackURLs       map[string]interface{} `json:"callback_urls"`
	WorkspaceTagFilter map[string]interface{} `json:"workspace_tag_filter"`
	// ClearWorkspaceTagFilter 为 true 时清空 tag 过滤（允许访问 org 内全部，仍需 grant）
	ClearWorkspaceTagFilter bool `json:"clear_workspace_tag_filter"`
	IsActive                *bool      `json:"is_active"`
	ExpiresAt               *time.Time `json:"expires_at"`
}

// CreateApplication 创建应用（调用方须已将 req.OrgID 设为鉴权 org）
func (s *ApplicationService) CreateApplication(ctx context.Context, req *CreateApplicationRequest, createdBy string) (*entity.Application, string, error) {
	if req.OrgID == 0 {
		return nil, "", ErrApplicationOrgForbidden
	}
	// 生成AppKey和AppSecret
	appKey := persistence.GenerateAppKey()
	appSecret := persistence.GenerateAppSecret()

	app := &entity.Application{
		OrgID:              req.OrgID,
		Name:               req.Name,
		AppKey:             appKey,
		AppSecret:          hashAppSecret(appSecret),
		Description:        req.Description,
		CallbackURLs:       req.CallbackURLs,
		WorkspaceTagFilter: NormalizeWorkspaceTagFilter(req.WorkspaceTagFilter),
		IsActive:           true,
		CreatedBy:          &createdBy,
		ExpiresAt:          req.ExpiresAt,
	}

	if err := s.repo.Create(ctx, app); err != nil {
		return nil, "", err
	}

	// 返回明文密钥（仅此一次）；库内仅存哈希
	return app, appSecret, nil
}

// GetApplication 无 org 的获取入口已禁用（C4）；请用 GetApplicationInOrg
func (s *ApplicationService) GetApplication(ctx context.Context, id uint) (*entity.Application, error) {
	return nil, fmt.Errorf("%w: use GetApplicationInOrg", ErrApplicationOrgForbidden)
}

// getInOrg 加载并校验 org 归属
func (s *ApplicationService) getInOrg(ctx context.Context, id, orgID uint) (*entity.Application, error) {
	app, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrApplicationNotFound
	}
	if app.OrgID != orgID {
		return nil, ErrApplicationOrgMismatch
	}
	return app, nil
}

// GetApplicationInOrg 获取应用（绑定鉴权 org）
func (s *ApplicationService) GetApplicationInOrg(ctx context.Context, id, orgID uint) (*entity.Application, error) {
	return s.getInOrg(ctx, id, orgID)
}

// ListApplications 获取应用列表
func (s *ApplicationService) ListApplications(ctx context.Context, orgID uint, isActive *bool) ([]*entity.Application, error) {
	if orgID == 0 {
		return nil, ErrApplicationOrgForbidden
	}
	return s.repo.ListByOrg(ctx, orgID, isActive)
}

// UpdateApplication 无 org 入口已禁用（C4）
func (s *ApplicationService) UpdateApplication(ctx context.Context, id uint, req *UpdateApplicationRequest) error {
	return fmt.Errorf("%w: use UpdateApplicationInOrg", ErrApplicationOrgForbidden)
}

// UpdateApplicationInOrg 在指定 org 内更新应用
func (s *ApplicationService) UpdateApplicationInOrg(ctx context.Context, id, orgID uint, req *UpdateApplicationRequest) error {
	if orgID == 0 {
		return ErrApplicationOrgForbidden
	}
	app, err := s.getInOrg(ctx, id, orgID)
	if err != nil {
		return err
	}

	if req.Name != "" {
		app.Name = req.Name
	}
	if req.Description != "" {
		app.Description = req.Description
	}
	if req.CallbackURLs != nil {
		app.CallbackURLs = req.CallbackURLs
	}
	if req.ClearWorkspaceTagFilter {
		app.WorkspaceTagFilter = nil
	} else if req.WorkspaceTagFilter != nil {
		app.WorkspaceTagFilter = NormalizeWorkspaceTagFilter(req.WorkspaceTagFilter)
	}
	if req.IsActive != nil {
		app.IsActive = *req.IsActive
	}
	if req.ExpiresAt != nil {
		app.ExpiresAt = req.ExpiresAt
	}

	return s.repo.Update(ctx, app)
}

// DeleteApplication 无 org 入口已禁用（C4）
func (s *ApplicationService) DeleteApplication(ctx context.Context, id uint) error {
	return fmt.Errorf("%w: use DeleteApplicationInOrg", ErrApplicationOrgForbidden)
}

// DeleteApplicationInOrg 在指定 org 内删除
func (s *ApplicationService) DeleteApplicationInOrg(ctx context.Context, id, orgID uint) error {
	if orgID == 0 {
		return ErrApplicationOrgForbidden
	}
	if _, err := s.getInOrg(ctx, id, orgID); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

// RegenerateSecret 无 org 入口已禁用（C4）
func (s *ApplicationService) RegenerateSecret(ctx context.Context, id uint) (string, error) {
	return "", fmt.Errorf("%w: use RegenerateSecretInOrg", ErrApplicationOrgForbidden)
}

// RegenerateSecretInOrg 在指定 org 内轮换密钥
func (s *ApplicationService) RegenerateSecretInOrg(ctx context.Context, id, orgID uint) (string, error) {
	if orgID == 0 {
		return "", ErrApplicationOrgForbidden
	}
	app, err := s.getInOrg(ctx, id, orgID)
	if err != nil {
		return "", err
	}

	if !app.IsActive {
		return "", errors.New("cannot regenerate secret for inactive application")
	}

	newSecret := persistence.GenerateAppSecret()
	if err := s.repo.RegenerateSecret(ctx, id, hashAppSecret(newSecret)); err != nil {
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

	if !verifyAppSecret(app.AppSecret, appSecret) {
		return nil, errors.New("invalid application credentials")
	}

	// 更新最后使用时间
	_ = s.repo.UpdateLastUsed(ctx, app.ID)

	return app, nil
}
