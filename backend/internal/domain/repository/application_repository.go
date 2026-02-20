package repository

import (
	"context"

	"iac-platform/internal/domain/entity"
)

// ApplicationRepository 应用仓储接口
type ApplicationRepository interface {
	// Create 创建应用
	Create(ctx context.Context, app *entity.Application) error

	// GetByID 根据ID获取应用
	GetByID(ctx context.Context, id uint) (*entity.Application, error)

	// GetByAppKey 根据AppKey获取应用
	GetByAppKey(ctx context.Context, appKey string) (*entity.Application, error)

	// ListByOrg 获取组织下的应用列表
	ListByOrg(ctx context.Context, orgID uint, isActive *bool) ([]*entity.Application, error)

	// Update 更新应用
	Update(ctx context.Context, app *entity.Application) error

	// Delete 删除应用
	Delete(ctx context.Context, id uint) error

	// UpdateLastUsed 更新最后使用时间
	UpdateLastUsed(ctx context.Context, id uint) error

	// RegenerateSecret 重新生成密钥
	RegenerateSecret(ctx context.Context, id uint, newSecret string) error
}
