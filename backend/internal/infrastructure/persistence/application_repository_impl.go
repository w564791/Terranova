package persistence

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/repository"

	"gorm.io/gorm"
)

type applicationRepositoryImpl struct {
	db *gorm.DB
}

// NewApplicationRepository 创建应用仓储实例
func NewApplicationRepository(db *gorm.DB) repository.ApplicationRepository {
	return &applicationRepositoryImpl{db: db}
}

// Create 创建应用
func (r *applicationRepositoryImpl) Create(ctx context.Context, app *entity.Application) error {
	return r.db.WithContext(ctx).Create(app).Error
}

// GetByID 根据ID获取应用
func (r *applicationRepositoryImpl) GetByID(ctx context.Context, id uint) (*entity.Application, error) {
	var app entity.Application
	err := r.db.WithContext(ctx).
		Preload("Organization").
		First(&app, id).Error
	if err != nil {
		return nil, err
	}
	return &app, nil
}

// GetByAppKey 根据AppKey获取应用
func (r *applicationRepositoryImpl) GetByAppKey(ctx context.Context, appKey string) (*entity.Application, error) {
	var app entity.Application
	err := r.db.WithContext(ctx).
		Where("app_key = ?", appKey).
		First(&app).Error
	if err != nil {
		return nil, err
	}
	return &app, nil
}

// ListByOrg 获取组织下的应用列表
func (r *applicationRepositoryImpl) ListByOrg(ctx context.Context, orgID uint, isActive *bool) ([]*entity.Application, error) {
	var apps []*entity.Application
	query := r.db.WithContext(ctx).Where("org_id = ?", orgID)

	if isActive != nil {
		query = query.Where("is_active = ?", *isActive)
	}

	err := query.Order("created_at DESC").Find(&apps).Error
	if err != nil {
		return nil, err
	}
	return apps, nil
}

// Update 更新应用
func (r *applicationRepositoryImpl) Update(ctx context.Context, app *entity.Application) error {
	return r.db.WithContext(ctx).Save(app).Error
}

// Delete 删除应用
func (r *applicationRepositoryImpl) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&entity.Application{}, id).Error
}

// UpdateLastUsed 更新最后使用时间
func (r *applicationRepositoryImpl) UpdateLastUsed(ctx context.Context, id uint) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&entity.Application{}).
		Where("id = ?", id).
		Update("last_used_at", now).Error
}

// RegenerateSecret 重新生成密钥
func (r *applicationRepositoryImpl) RegenerateSecret(ctx context.Context, id uint, newSecret string) error {
	return r.db.WithContext(ctx).
		Model(&entity.Application{}).
		Where("id = ?", id).
		Update("app_secret", newSecret).Error
}

// GenerateAppKey 生成应用密钥
func GenerateAppKey() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return fmt.Sprintf("app_%s", hex.EncodeToString(bytes))
}

// GenerateAppSecret 生成应用密钥
func GenerateAppSecret() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
