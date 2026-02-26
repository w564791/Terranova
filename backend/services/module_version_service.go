package services

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"iac-platform/internal/models"
	"log"
	"time"

	"gorm.io/gorm"
)

// generateModuleVersionID 生成模块版本 ID
func generateModuleVersionID() string {
	bytes := make([]byte, 10)
	rand.Read(bytes)
	return "modv-" + hex.EncodeToString(bytes)
}

// ModuleVersionService 模块版本服务
type ModuleVersionService struct {
	db *gorm.DB
}

// NewModuleVersionService 创建模块版本服务
func NewModuleVersionService(db *gorm.DB) *ModuleVersionService {
	return &ModuleVersionService{db: db}
}

// CreateModuleVersionRequest 创建模块版本请求
type CreateModuleVersionRequest struct {
	Version           string `json:"version" binding:"required"`    // 新 TF 版本号
	Source            string `json:"source"`                        // 可选覆盖 source
	ModuleSource      string `json:"module_source"`                 // 可选覆盖 module_source
	InheritSchemaFrom string `json:"inherit_schema_from,omitempty"` // 从哪个版本继承 Schema（版本 ID）
	SetAsDefault      bool   `json:"set_as_default"`                // 是否设为默认版本
}

// UpdateModuleVersionRequest 更新模块版本请求
type UpdateModuleVersionRequest struct {
	Source       string `json:"source,omitempty"`
	ModuleSource string `json:"module_source,omitempty"`
	Status       string `json:"status,omitempty"` // active, deprecated, archived
}

// SetDefaultVersionRequest 设置默认版本请求
type SetDefaultVersionRequest struct {
	VersionID string `json:"version_id" binding:"required"`
}

// InheritDemosRequest 继承 Demos 请求
type InheritDemosRequest struct {
	FromVersionID string `json:"from_version_id" binding:"required"`
	DemoIDs       []uint `json:"demo_ids,omitempty"` // 可选，不传则继承全部
}

// ModuleVersionListResponse 版本列表响应
type ModuleVersionListResponse struct {
	Items      []models.ModuleVersion `json:"items"`
	Total      int64                  `json:"total"`
	Page       int                    `json:"page"`
	PageSize   int                    `json:"page_size"`
	TotalPages int                    `json:"total_pages"`
}

// ListVersions 获取模块的所有版本
func (s *ModuleVersionService) ListVersions(moduleID uint, page, pageSize int) (*ModuleVersionListResponse, error) {
	var total int64

	s.db.Model(&models.ModuleVersion{}).Where("module_id = ?", moduleID).Count(&total)
	log.Printf("[ModuleVersion] ListVersions: module_id=%d, initial count=%d", moduleID, total)

	// 如果没有版本记录，自动为该模块创建一个默认版本
	if total == 0 {
		log.Printf("[ModuleVersion] No versions found for module %d, auto-creating default version", moduleID)
		if err := s.ensureDefaultVersion(moduleID); err != nil {
			log.Printf("[ModuleVersion] Failed to auto-create default version for module %d: %v", moduleID, err)
		} else {
			// 重新计数
			s.db.Model(&models.ModuleVersion{}).Where("module_id = ?", moduleID).Count(&total)
			log.Printf("[ModuleVersion] After auto-create, count=%d", total)
		}
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize
	var versions []models.ModuleVersion
	if err := s.db.Model(&models.ModuleVersion{}).
		Where("module_id = ?", moduleID).
		Order("is_default DESC, created_at DESC").
		Offset(offset).Limit(pageSize).
		Find(&versions).Error; err != nil {
		return nil, err
	}

	log.Printf("[ModuleVersion] ListVersions: found %d versions for module %d", len(versions), moduleID)

	// 填充额外信息
	for i := range versions {
		s.fillVersionDetails(&versions[i])
	}

	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return &ModuleVersionListResponse{
		Items:      versions,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// ensureDefaultVersion 确保模块至少有一个默认版本
// 用于处理在多版本功能上线前创建的旧模块
func (s *ModuleVersionService) ensureDefaultVersion(moduleID uint) error {
	var module models.Module
	if err := s.db.First(&module, moduleID).Error; err != nil {
		return fmt.Errorf("module not found: %w", err)
	}

	versionID := generateModuleVersionID()
	version := module.Version
	if version == "" {
		version = "1.0.0"
	}

	// 核心事务：只创建版本和更新 default_version_id
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// 双重检查：在事务内再次确认没有版本
		var count int64
		tx.Model(&models.ModuleVersion{}).Where("module_id = ?", moduleID).Count(&count)
		if count > 0 {
			log.Printf("[ModuleVersion] ensureDefaultVersion: module %d already has %d versions (race), skipping", moduleID, count)
			return nil
		}

		newVersion := &models.ModuleVersion{
			ID:           versionID,
			ModuleID:     moduleID,
			Version:      version,
			Source:       module.Source,
			ModuleSource: module.ModuleSource,
			IsDefault:    true,
			Status:       models.ModuleVersionStatusActive,
			CreatedBy:    module.CreatedBy,
		}

		if err := tx.Create(newVersion).Error; err != nil {
			return fmt.Errorf("failed to create default version: %w", err)
		}

		// 更新模块的 default_version_id
		if err := tx.Model(&models.Module{}).Where("id = ?", moduleID).Update("default_version_id", versionID).Error; err != nil {
			return fmt.Errorf("failed to set default version: %w", err)
		}

		log.Printf("[ModuleVersion] Auto-created default version %s (v%s) for module %d", versionID, version, moduleID)
		return nil
	})

	if err != nil {
		return err
	}

	// 非关键操作：关联现有无版本的 Schema 和 Demo（失败不影响主流程）
	if err := s.db.Model(&models.Schema{}).
		Where("module_id = ? AND (module_version_id IS NULL OR module_version_id = '')", moduleID).
		Update("module_version_id", versionID).Error; err != nil {
		log.Printf("[ModuleVersion] Warning: failed to associate existing schemas for module %d: %v", moduleID, err)
	}

	if err := s.db.Model(&models.ModuleDemo{}).
		Where("module_id = ? AND (module_version_id IS NULL OR module_version_id = '')", moduleID).
		Update("module_version_id", versionID).Error; err != nil {
		log.Printf("[ModuleVersion] Warning: failed to associate existing demos for module %d: %v", moduleID, err)
	}

	// 如果有 active 的 Schema，设置为版本的 active_schema_id
	var activeSchema models.Schema
	if err := s.db.Where("module_id = ? AND module_version_id = ? AND status = ?", moduleID, versionID, "active").
		Order("created_at DESC").First(&activeSchema).Error; err == nil {
		s.db.Model(&models.ModuleVersion{}).Where("id = ?", versionID).Update("active_schema_id", activeSchema.ID)
		log.Printf("[ModuleVersion] Set active_schema_id=%d for version %s", activeSchema.ID, versionID)
	}

	return nil
}

// GetVersion 获取版本详情
func (s *ModuleVersionService) GetVersion(moduleID uint, versionID string) (*models.ModuleVersion, error) {
	var version models.ModuleVersion
	if err := s.db.Where("id = ? AND module_id = ?", versionID, moduleID).First(&version).Error; err != nil {
		return nil, err
	}

	s.fillVersionDetails(&version)
	return &version, nil
}

// GetVersionByVersion 根据版本号获取版本
func (s *ModuleVersionService) GetVersionByVersion(moduleID uint, versionStr string) (*models.ModuleVersion, error) {
	var version models.ModuleVersion
	if err := s.db.Where("module_id = ? AND version = ?", moduleID, versionStr).First(&version).Error; err != nil {
		return nil, err
	}

	s.fillVersionDetails(&version)
	return &version, nil
}

// GetDefaultVersion 获取默认版本
func (s *ModuleVersionService) GetDefaultVersion(moduleID uint) (*models.ModuleVersion, error) {
	var version models.ModuleVersion
	if err := s.db.Where("module_id = ? AND is_default = ?", moduleID, true).First(&version).Error; err != nil {
		return nil, err
	}

	s.fillVersionDetails(&version)
	return &version, nil
}

// CreateVersion 创建新版本
func (s *ModuleVersionService) CreateVersion(moduleID uint, req *CreateModuleVersionRequest, userID string) (*models.ModuleVersion, error) {
	// 检查模块是否存在
	var module models.Module
	if err := s.db.First(&module, moduleID).Error; err != nil {
		return nil, errors.New("module not found")
	}

	// 检查版本号是否已存在
	var count int64
	s.db.Model(&models.ModuleVersion{}).Where("module_id = ? AND version = ?", moduleID, req.Version).Count(&count)
	if count > 0 {
		return nil, errors.New("version already exists")
	}

	// 创建新版本
	newVersion := &models.ModuleVersion{
		ID:        generateModuleVersionID(),
		ModuleID:  moduleID,
		Version:   req.Version,
		Source:    req.Source,
		IsDefault: false, // 永不自动设为默认
		Status:    models.ModuleVersionStatusActive,
		CreatedBy: &userID,
	}

	// 如果没有指定 source，使用模块的 source
	if newVersion.Source == "" {
		newVersion.Source = module.Source
	}

	// 如果指定了 module_source，使用它；否则使用模块的 module_source
	if req.ModuleSource != "" {
		newVersion.ModuleSource = req.ModuleSource
	} else {
		newVersion.ModuleSource = module.ModuleSource
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		// 创建版本记录
		if err := tx.Create(newVersion).Error; err != nil {
			return err
		}

		// 如果指定了继承来源，复制 Schema
		if req.InheritSchemaFrom != "" {
			if err := s.inheritSchema(tx, moduleID, newVersion.ID, req.InheritSchemaFrom, userID); err != nil {
				return err
			}
			newVersion.InheritedFromVersionID = &req.InheritSchemaFrom
			// 只更新 inherited_from_version_id 字段，避免覆盖 inheritSchema 设置的 active_schema_id
			if err := tx.Model(newVersion).Update("inherited_from_version_id", req.InheritSchemaFrom).Error; err != nil {
				return err
			}
			// 同步内存对象的 active_schema_id
			tx.First(newVersion, "id = ?", newVersion.ID)
		}

		// 如果设为默认版本
		if req.SetAsDefault {
			if err := s.setDefaultVersionTx(tx, moduleID, newVersion.ID); err != nil {
				return err
			}
			newVersion.IsDefault = true
		}

		// 如果这是模块的第一个版本，自动设为默认
		var versionCount int64
		tx.Model(&models.ModuleVersion{}).Where("module_id = ?", moduleID).Count(&versionCount)
		if versionCount == 1 {
			if err := s.setDefaultVersionTx(tx, moduleID, newVersion.ID); err != nil {
				return err
			}
			newVersion.IsDefault = true
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	log.Printf("[ModuleVersion] Created version %s (v%s) for module %d", newVersion.ID, newVersion.Version, moduleID)
	return newVersion, nil
}

// UpdateVersion 更新版本信息
func (s *ModuleVersionService) UpdateVersion(moduleID uint, versionID string, req *UpdateModuleVersionRequest) (*models.ModuleVersion, error) {
	var version models.ModuleVersion
	if err := s.db.Where("id = ? AND module_id = ?", versionID, moduleID).First(&version).Error; err != nil {
		return nil, err
	}

	if req.Source != "" {
		version.Source = req.Source
	}
	if req.ModuleSource != "" {
		version.ModuleSource = req.ModuleSource
	}
	if req.Status != "" {
		version.Status = req.Status
	}

	if err := s.db.Save(&version).Error; err != nil {
		return nil, err
	}

	return &version, nil
}

// DeleteVersion 删除版本（级联删除关联的 Schema 和 Demo）
func (s *ModuleVersionService) DeleteVersion(moduleID uint, versionID string) error {
	var version models.ModuleVersion
	if err := s.db.Where("id = ? AND module_id = ?", versionID, moduleID).First(&version).Error; err != nil {
		return err
	}

	// 检查是否为默认版本
	if version.IsDefault {
		return errors.New("cannot delete default version, please set another version as default first")
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		// 先清除 active_schema_id 外键引用
		if err := tx.Model(&models.ModuleVersion{}).
			Where("id = ?", versionID).
			Update("active_schema_id", nil).Error; err != nil {
			return fmt.Errorf("failed to clear active_schema_id: %w", err)
		}

		// 删除关联的 Schema
		if err := tx.Where("module_version_id = ?", versionID).Delete(&models.Schema{}).Error; err != nil {
			return fmt.Errorf("failed to delete associated schemas: %w", err)
		}

		// 删除关联的 Demo 版本
		var demos []models.ModuleDemo
		if err := tx.Where("module_version_id = ?", versionID).Find(&demos).Error; err != nil {
			return fmt.Errorf("failed to find associated demos: %w", err)
		}

		for _, demo := range demos {
			// 删除 Demo 的版本记录
			if err := tx.Where("demo_id = ?", demo.ID).Delete(&models.ModuleDemoVersion{}).Error; err != nil {
				return fmt.Errorf("failed to delete demo versions: %w", err)
			}
		}

		// 删除关联的 Demo
		if err := tx.Where("module_version_id = ?", versionID).Delete(&models.ModuleDemo{}).Error; err != nil {
			return fmt.Errorf("failed to delete associated demos: %w", err)
		}

		// 删除版本记录
		if err := tx.Delete(&version).Error; err != nil {
			return fmt.Errorf("failed to delete version: %w", err)
		}

		log.Printf("[ModuleVersion] Deleted version %s (v%s) for module %d with all associated data", versionID, version.Version, moduleID)
		return nil
	})
}

// SetDefaultVersion 设置默认版本
func (s *ModuleVersionService) SetDefaultVersion(moduleID uint, versionID string) error {
	// 检查版本是否存在
	var version models.ModuleVersion
	if err := s.db.Where("id = ? AND module_id = ?", versionID, moduleID).First(&version).Error; err != nil {
		return errors.New("version not found")
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		return s.setDefaultVersionTx(tx, moduleID, versionID)
	})
}

// setDefaultVersionTx 在事务中设置默认版本
func (s *ModuleVersionService) setDefaultVersionTx(tx *gorm.DB, moduleID uint, versionID string) error {
	// 取消当前默认版本
	if err := tx.Model(&models.ModuleVersion{}).
		Where("module_id = ? AND is_default = ?", moduleID, true).
		Update("is_default", false).Error; err != nil {
		return err
	}

	// 设置新的默认版本
	if err := tx.Model(&models.ModuleVersion{}).
		Where("id = ? AND module_id = ?", versionID, moduleID).
		Update("is_default", true).Error; err != nil {
		return err
	}

	// 更新 Module 的 default_version_id
	return tx.Model(&models.Module{}).
		Where("id = ?", moduleID).
		Update("default_version_id", versionID).Error
}

// InheritDemos 继承 Demos
func (s *ModuleVersionService) InheritDemos(moduleID uint, targetVersionID string, req *InheritDemosRequest, userID string) (int, error) {
	// 检查目标版本是否存在
	var targetVersion models.ModuleVersion
	if err := s.db.Where("id = ? AND module_id = ?", targetVersionID, moduleID).First(&targetVersion).Error; err != nil {
		return 0, errors.New("target version not found")
	}

	// 检查源版本是否存在
	var sourceVersion models.ModuleVersion
	if err := s.db.Where("id = ? AND module_id = ?", req.FromVersionID, moduleID).First(&sourceVersion).Error; err != nil {
		return 0, errors.New("source version not found")
	}

	// 获取源版本的 Demos
	var sourceDemos []models.ModuleDemo
	query := s.db.Where("module_id = ? AND module_version_id = ?", moduleID, req.FromVersionID)
	if len(req.DemoIDs) > 0 {
		query = query.Where("id IN ?", req.DemoIDs)
	}
	if err := query.Preload("CurrentVersion").Find(&sourceDemos).Error; err != nil {
		return 0, err
	}

	if len(sourceDemos) == 0 {
		return 0, nil
	}

	inheritedCount := 0
	err := s.db.Transaction(func(tx *gorm.DB) error {
		for _, demo := range sourceDemos {
			// 检查目标版本是否已有同名 Demo
			var existingCount int64
			tx.Model(&models.ModuleDemo{}).
				Where("module_id = ? AND module_version_id = ? AND name = ?", moduleID, targetVersionID, demo.Name).
				Count(&existingCount)
			if existingCount > 0 {
				log.Printf("[ModuleVersion] Skipping demo %s, already exists in target version", demo.Name)
				continue
			}

			// 创建新 Demo
			newDemo := models.ModuleDemo{
				ModuleID:            moduleID,
				ModuleVersionID:     &targetVersionID,
				Name:                demo.Name,
				Description:         demo.Description,
				IsActive:            demo.IsActive,
				UsageNotes:          demo.UsageNotes,
				InheritedFromDemoID: &demo.ID,
				CreatedBy:           &userID,
			}

			if err := tx.Create(&newDemo).Error; err != nil {
				return err
			}

			// 如果源 Demo 有当前版本，复制版本数据
			if demo.CurrentVersion != nil {
				newDemoVersion := models.ModuleDemoVersion{
					DemoID:        newDemo.ID,
					Version:       1,
					IsLatest:      true,
					ConfigData:    demo.CurrentVersion.ConfigData,
					ChangeSummary: fmt.Sprintf("Inherited from demo %d (version %s)", demo.ID, sourceVersion.Version),
					ChangeType:    "create",
					CreatedBy:     &userID,
				}

				if err := tx.Create(&newDemoVersion).Error; err != nil {
					return err
				}

				// 更新 Demo 的 current_version_id
				if err := tx.Model(&newDemo).Update("current_version_id", newDemoVersion.ID).Error; err != nil {
					return err
				}
			}

			inheritedCount++
			log.Printf("[ModuleVersion] Inherited demo %s to version %s", demo.Name, targetVersionID)
		}
		return nil
	})

	if err != nil {
		return 0, err
	}

	return inheritedCount, nil
}

// getNextSchemaVersion 获取模块的下一个全局 Schema 版本号
func (s *ModuleVersionService) getNextSchemaVersion(tx *gorm.DB, moduleID uint) string {
	var maxVersion int
	// 查询该模块所有 Schema 的最大版本号（只考虑纯数字版本号，忽略非数字的）
	if err := tx.Model(&models.Schema{}).
		Where("module_id = ? AND version ~ '^[0-9]+$'", moduleID).
		Select("COALESCE(MAX(CAST(version AS INTEGER)), 0)").
		Scan(&maxVersion).Error; err != nil {
		log.Printf("[ModuleVersion] Warning: failed to get max schema version for module %d: %v, defaulting to 1", moduleID, err)
		// 回退：直接 COUNT 作为版本号
		var count int64
		tx.Model(&models.Schema{}).Where("module_id = ?", moduleID).Count(&count)
		return fmt.Sprintf("%d", count+1)
	}
	return fmt.Sprintf("%d", maxVersion+1)
}

// inheritSchema 继承 Schema（创建新的全局版本号）
func (s *ModuleVersionService) inheritSchema(tx *gorm.DB, moduleID uint, targetVersionID, sourceVersionID, userID string) error {
	// 获取源版本
	var sourceVersion models.ModuleVersion
	if err := tx.Where("id = ? AND module_id = ?", sourceVersionID, moduleID).First(&sourceVersion).Error; err != nil {
		return fmt.Errorf("source version not found: %w", err)
	}

	// 获取源版本的 active Schema（优先使用 active_schema_id）
	var sourceSchema models.Schema
	if sourceVersion.ActiveSchemaID != nil {
		if err := tx.First(&sourceSchema, *sourceVersion.ActiveSchemaID).Error; err != nil {
			return fmt.Errorf("source schema not found: %w", err)
		}
	} else {
		// 兼容旧数据：查找 active 状态的 Schema
		if err := tx.Where("module_id = ? AND module_version_id = ? AND status = ?", moduleID, sourceVersionID, "active").
			Order("created_at DESC").First(&sourceSchema).Error; err != nil {
			// 如果没有 active 的，尝试获取任意最新的
			if err := tx.Where("module_id = ? AND module_version_id = ?", moduleID, sourceVersionID).
				Order("created_at DESC").First(&sourceSchema).Error; err != nil {
				log.Printf("[ModuleVersion] No schema found for source version %s, skipping inheritance", sourceVersionID)
				return nil // 没有 Schema 可继承，不是错误
			}
		}
	}

	// 获取下一个全局版本号
	nextVersion := s.getNextSchemaVersion(tx, moduleID)

	// 复制 Schema 数据（使用全局递增版本号）
	newSchema := models.Schema{
		ModuleID:              moduleID,
		ModuleVersionID:       &targetVersionID,
		Version:               nextVersion, // 全局递增版本号
		Status:                "active",    // 新继承的 Schema 直接设为 active
		SchemaData:            sourceSchema.SchemaData,
		AIGenerated:           sourceSchema.AIGenerated,
		SourceType:            sourceSchema.SourceType,
		SchemaVersion:         sourceSchema.SchemaVersion,
		OpenAPISchema:         sourceSchema.OpenAPISchema,
		VariablesTF:           sourceSchema.VariablesTF,
		UIConfig:              sourceSchema.UIConfig,
		InheritedFromSchemaID: &sourceSchema.ID,
		CreatedBy:             &userID,
	}

	if err := tx.Create(&newSchema).Error; err != nil {
		return err
	}

	// 更新目标版本的 active_schema_id
	if err := tx.Model(&models.ModuleVersion{}).
		Where("id = ?", targetVersionID).
		Update("active_schema_id", newSchema.ID).Error; err != nil {
		return fmt.Errorf("failed to update active_schema_id: %w", err)
	}

	log.Printf("[ModuleVersion] Inherited schema %d (v%s) to version %s as schema %d (v%s)",
		sourceSchema.ID, sourceSchema.Version, targetVersionID, newSchema.ID, nextVersion)
	return nil
}

// fillVersionDetails 填充版本详情
func (s *ModuleVersionService) fillVersionDetails(version *models.ModuleVersion) {
	// Schema 数量（该版本关联的所有 Schema）
	var schemaCount int64
	s.db.Model(&models.Schema{}).Where("module_version_id = ?", version.ID).Count(&schemaCount)
	version.SchemaCount = int(schemaCount)

	// Active Schema 版本（优先使用 active_schema_id）
	if version.ActiveSchemaID != nil {
		var activeSchema models.Schema
		if err := s.db.First(&activeSchema, *version.ActiveSchemaID).Error; err == nil {
			version.ActiveSchemaVersion = activeSchema.Version
		}
	} else {
		// 兼容旧数据：查找 active 状态的 Schema
		var activeSchema models.Schema
		if err := s.db.Where("module_version_id = ? AND status = ?", version.ID, "active").
			Order("created_at DESC").First(&activeSchema).Error; err == nil {
			version.ActiveSchemaVersion = activeSchema.Version
		}
	}

	// Demo 数量
	var demoCount int64
	s.db.Model(&models.ModuleDemo{}).Where("module_version_id = ?", version.ID).Count(&demoCount)
	version.DemoCount = int(demoCount)

	// 创建者名称
	if version.CreatedBy != nil {
		var user models.User
		if err := s.db.Select("username").Where("user_id = ?", *version.CreatedBy).First(&user).Error; err == nil {
			version.CreatedByName = user.Username
		}
	}
}

// MigrateExistingModules 迁移现有模块数据到多版本结构
// 为每个现有模块创建一个 ModuleVersion 记录，并关联现有的 Schema 和 Demo
func (s *ModuleVersionService) MigrateExistingModules() error {
	var modules []models.Module
	if err := s.db.Find(&modules).Error; err != nil {
		return err
	}

	for _, module := range modules {
		// 检查是否已有版本记录
		var existingCount int64
		s.db.Model(&models.ModuleVersion{}).Where("module_id = ?", module.ID).Count(&existingCount)
		if existingCount > 0 {
			log.Printf("[Migration] Module %d already has %d versions, skipping", module.ID, existingCount)
			continue
		}

		err := s.db.Transaction(func(tx *gorm.DB) error {
			// 创建版本记录
			versionID := generateModuleVersionID()
			newVersion := &models.ModuleVersion{
				ID:           versionID,
				ModuleID:     module.ID,
				Version:      module.Version,
				Source:       module.Source,
				ModuleSource: module.ModuleSource,
				IsDefault:    true,
				Status:       models.ModuleVersionStatusActive,
				CreatedBy:    module.CreatedBy,
				CreatedAt:    module.CreatedAt,
				UpdatedAt:    time.Now(),
			}

			if err := tx.Create(newVersion).Error; err != nil {
				return err
			}

			// 更新 Module 的 default_version_id
			if err := tx.Model(&module).Update("default_version_id", versionID).Error; err != nil {
				return err
			}

			// 更新现有 Schema 的 module_version_id
			if err := tx.Model(&models.Schema{}).
				Where("module_id = ? AND module_version_id IS NULL", module.ID).
				Update("module_version_id", versionID).Error; err != nil {
				return err
			}

			// 更新现有 Demo 的 module_version_id
			if err := tx.Model(&models.ModuleDemo{}).
				Where("module_id = ? AND module_version_id IS NULL", module.ID).
				Update("module_version_id", versionID).Error; err != nil {
				return err
			}

			log.Printf("[Migration] Created version %s for module %d (v%s)", versionID, module.ID, module.Version)
			return nil
		})

		if err != nil {
			log.Printf("[Migration] Failed to migrate module %d: %v", module.ID, err)
			return err
		}
	}

	return nil
}

// GetVersionSchemas 获取版本的所有 Schema
func (s *ModuleVersionService) GetVersionSchemas(moduleID uint, versionID string) ([]models.Schema, error) {
	var schemas []models.Schema
	if err := s.db.Where("module_id = ? AND module_version_id = ?", moduleID, versionID).
		Order("created_at DESC").Find(&schemas).Error; err != nil {
		return nil, err
	}
	return schemas, nil
}

// GetVersionDemos 获取版本的所有 Demo
func (s *ModuleVersionService) GetVersionDemos(moduleID uint, versionID string) ([]models.ModuleDemo, error) {
	var demos []models.ModuleDemo
	if err := s.db.Where("module_id = ? AND module_version_id = ?", moduleID, versionID).
		Preload("CurrentVersion").
		Order("created_at DESC").Find(&demos).Error; err != nil {
		return nil, err
	}
	return demos, nil
}

// CompareVersions 比较两个版本的 Schema 差异
func (s *ModuleVersionService) CompareVersions(moduleID uint, fromVersionID, toVersionID string) (map[string]interface{}, error) {
	// 获取两个版本的 active Schema
	var fromSchema, toSchema models.Schema

	if err := s.db.Where("module_id = ? AND module_version_id = ? AND status = ?", moduleID, fromVersionID, "active").
		Order("created_at DESC").First(&fromSchema).Error; err != nil {
		return nil, fmt.Errorf("no active schema found for version %s", fromVersionID)
	}

	if err := s.db.Where("module_id = ? AND module_version_id = ? AND status = ?", moduleID, toVersionID, "active").
		Order("created_at DESC").First(&toSchema).Error; err != nil {
		return nil, fmt.Errorf("no active schema found for version %s", toVersionID)
	}

	// 解析 OpenAPI Schema
	var fromOpenAPI, toOpenAPI map[string]interface{}
	if fromSchema.OpenAPISchema != nil {
		fromBytes, _ := json.Marshal(fromSchema.OpenAPISchema)
		json.Unmarshal(fromBytes, &fromOpenAPI)
	}
	if toSchema.OpenAPISchema != nil {
		toBytes, _ := json.Marshal(toSchema.OpenAPISchema)
		json.Unmarshal(toBytes, &toOpenAPI)
	}

	// 简单的字段比较
	fromFields := extractFieldNames(fromOpenAPI)
	toFields := extractFieldNames(toOpenAPI)

	addedFields := []string{}
	removedFields := []string{}
	commonFields := []string{}

	for field := range toFields {
		if _, exists := fromFields[field]; !exists {
			addedFields = append(addedFields, field)
		} else {
			commonFields = append(commonFields, field)
		}
	}

	for field := range fromFields {
		if _, exists := toFields[field]; !exists {
			removedFields = append(removedFields, field)
		}
	}

	return map[string]interface{}{
		"from_version":   fromVersionID,
		"to_version":     toVersionID,
		"from_schema_id": fromSchema.ID,
		"to_schema_id":   toSchema.ID,
		"added_fields":   addedFields,
		"removed_fields": removedFields,
		"common_fields":  commonFields,
		"stats": map[string]int{
			"added":   len(addedFields),
			"removed": len(removedFields),
			"common":  len(commonFields),
		},
	}, nil
}

// extractFieldNames 从 OpenAPI Schema 中提取字段名
func extractFieldNames(schema map[string]interface{}) map[string]bool {
	fields := make(map[string]bool)

	if schema == nil {
		return fields
	}

	// 尝试从 properties 中提取
	if properties, ok := schema["properties"].(map[string]interface{}); ok {
		for name := range properties {
			fields[name] = true
		}
	}

	return fields
}
