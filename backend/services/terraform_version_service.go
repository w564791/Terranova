package services

import (
	"fmt"
	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// TerraformVersionService Terraform版本服务
type TerraformVersionService struct {
	db *gorm.DB
}

// NewTerraformVersionService 创建Terraform版本服务
func NewTerraformVersionService(db *gorm.DB) *TerraformVersionService {
	return &TerraformVersionService{
		db: db,
	}
}

// List 获取所有Terraform版本
func (s *TerraformVersionService) List(enabled *bool, deprecated *bool) ([]models.TerraformVersion, error) {
	var versions []models.TerraformVersion
	query := s.db.Model(&models.TerraformVersion{})

	if enabled != nil {
		query = query.Where("enabled = ?", *enabled)
	}

	if deprecated != nil {
		query = query.Where("deprecated = ?", *deprecated)
	}

	// 默认版本排在最前面
	err := query.Order("is_default DESC, created_at DESC").Find(&versions).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query terraform versions: %w", err)
	}

	if versions == nil {
		versions = []models.TerraformVersion{}
	}

	return versions, nil
}

// GetDefault 获取默认版本
func (s *TerraformVersionService) GetDefault() (*models.TerraformVersion, error) {
	var version models.TerraformVersion
	err := s.db.Where("is_default = ?", true).First(&version).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("no default version configured")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get default version: %w", err)
	}
	return &version, nil
}

// GetByID 根据ID获取Terraform版本
func (s *TerraformVersionService) GetByID(id int) (*models.TerraformVersion, error) {
	var version models.TerraformVersion
	err := s.db.First(&version, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("terraform version not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get terraform version: %w", err)
	}

	return &version, nil
}

// Create 创建Terraform版本
func (s *TerraformVersionService) Create(req *models.CreateTerraformVersionRequest) (*models.TerraformVersion, error) {
	// 检查版本是否已存在
	var count int64
	s.db.Model(&models.TerraformVersion{}).Where("version = ?", req.Version).Count(&count)
	if count > 0 {
		return nil, fmt.Errorf("version %s already exists", req.Version)
	}

	version := &models.TerraformVersion{
		Version:     req.Version,
		DownloadURL: req.DownloadURL,
		Checksum:    req.Checksum,
		Enabled:     req.Enabled,
		Deprecated:  req.Deprecated,
	}

	err := s.db.Create(version).Error
	if err != nil {
		return nil, fmt.Errorf("failed to create terraform version: %w", err)
	}

	return version, nil
}

// Update 更新Terraform版本
func (s *TerraformVersionService) Update(id int, req *models.UpdateTerraformVersionRequest) (*models.TerraformVersion, error) {
	// 检查版本是否存在
	version, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}

	// 构建更新数据
	updates := make(map[string]interface{})

	if req.DownloadURL != nil {
		updates["download_url"] = *req.DownloadURL
		// 当下载链接变更时，自动重新检测引擎类型
		newEngineType := models.DetectEngineTypeFromURL(*req.DownloadURL)
		updates["engine_type"] = newEngineType
	}

	if req.Checksum != nil {
		updates["checksum"] = *req.Checksum
	}

	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}

	if req.Deprecated != nil {
		updates["deprecated"] = *req.Deprecated
	}

	if len(updates) > 0 {
		err = s.db.Model(version).Updates(updates).Error
		if err != nil {
			return nil, fmt.Errorf("failed to update terraform version: %w", err)
		}
	}

	// 返回更新后的版本
	return s.GetByID(id)
}

// SetDefault 设置默认版本
func (s *TerraformVersionService) SetDefault(id int) (*models.TerraformVersion, error) {
	// 检查版本是否存在
	version, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}

	// 检查版本是否启用
	if !version.Enabled {
		return nil, fmt.Errorf("cannot set disabled version as default")
	}

	// 使用事务确保原子性
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// 1. 取消所有版本的默认状态
		if err := tx.Model(&models.TerraformVersion{}).
			Where("is_default = ?", true).
			Update("is_default", false).Error; err != nil {
			return fmt.Errorf("failed to clear default flags: %w", err)
		}

		// 2. 设置新的默认版本
		if err := tx.Model(&models.TerraformVersion{}).
			Where("id = ?", id).
			Update("is_default", true).Error; err != nil {
			return fmt.Errorf("failed to set default version: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return s.GetByID(id)
}

// Delete 删除Terraform版本
func (s *TerraformVersionService) Delete(id int) error {
	// 检查版本是否存在
	version, err := s.GetByID(id)
	if err != nil {
		return err
	}

	// 不允许删除默认版本
	if version.IsDefault {
		return fmt.Errorf("cannot delete default version, please set another version as default first")
	}

	// 检查是否有workspace在使用该版本
	inUse, err := s.CheckVersionInUse(id)
	if err != nil {
		return err
	}
	if inUse {
		return fmt.Errorf("version is in use by workspaces")
	}

	result := s.db.Delete(&models.TerraformVersion{}, id)
	if result.Error != nil {
		return fmt.Errorf("failed to delete terraform version: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("terraform version not found")
	}

	return nil
}

// CheckVersionInUse 检查版本是否被workspace使用
func (s *TerraformVersionService) CheckVersionInUse(id int) (bool, error) {
	// 获取版本号
	version, err := s.GetByID(id)
	if err != nil {
		return false, err
	}

	// 检查是否有workspace使用该版本
	var count int64
	err = s.db.Model(&models.Workspace{}).Where("terraform_version = ?", version.Version).Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("failed to check version usage: %w", err)
	}

	return count > 0, nil
}
