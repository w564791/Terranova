package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// ModuleDemoService 模块演示服务
type ModuleDemoService struct {
	db *gorm.DB
}

// NewModuleDemoService 创建模块演示服务实例
func NewModuleDemoService(db *gorm.DB) *ModuleDemoService {
	return &ModuleDemoService{db: db}
}

// CreateDemo 创建新的演示配置
func (s *ModuleDemoService) CreateDemo(demo *models.ModuleDemo, configData map[string]interface{}, userID *string) error {
	db := s.db

	// 开始事务
	return db.Transaction(func(tx *gorm.DB) error {
		// 1. 创建 demo 记录
		demo.CreatedBy = userID
		if err := tx.Create(demo).Error; err != nil {
			return fmt.Errorf("failed to create demo: %w", err)
		}

		// 2. 创建第一个版本
		version := &models.ModuleDemoVersion{
			DemoID:        demo.ID,
			Version:       1,
			IsLatest:      true,
			ConfigData:    configData,
			ChangeSummary: "Initial version",
			ChangeType:    "create",
			CreatedBy:     userID,
		}

		if err := tx.Create(version).Error; err != nil {
			return fmt.Errorf("failed to create demo version: %w", err)
		}

		// 3. 更新 demo 的 current_version_id
		demo.CurrentVersionID = &version.ID
		if err := tx.Model(demo).Update("current_version_id", version.ID).Error; err != nil {
			return fmt.Errorf("failed to update current version: %w", err)
		}

		return nil
	})
}

// UpdateDemo 更新演示配置（创建新版本）
func (s *ModuleDemoService) UpdateDemo(demoID uint, updates map[string]interface{}, configData map[string]interface{}, changeSummary string, userID *string) error {
	db := s.db

	return db.Transaction(func(tx *gorm.DB) error {
		// 1. 获取当前 demo
		var demo models.ModuleDemo
		if err := tx.Preload("CurrentVersion").First(&demo, demoID).Error; err != nil {
			return fmt.Errorf("demo not found: %w", err)
		}

		// 2. 更新 demo 基本信息
		if err := tx.Model(&demo).Updates(updates).Error; err != nil {
			return fmt.Errorf("failed to update demo: %w", err)
		}

		// 3. 获取当前最新版本号
		var maxVersion int
		if err := tx.Model(&models.ModuleDemoVersion{}).
			Where("demo_id = ?", demoID).
			Select("COALESCE(MAX(version), 0)").
			Scan(&maxVersion).Error; err != nil {
			return fmt.Errorf("failed to get max version: %w", err)
		}

		// 4. 计算与上一版本的差异
		var diff string
		if demo.CurrentVersion != nil {
			diffResult, err := s.calculateDiff(demo.CurrentVersion.ConfigData, configData)
			if err == nil {
				diff = diffResult
			}
		}

		// 5. 将之前的版本标记为非最新
		if err := tx.Model(&models.ModuleDemoVersion{}).
			Where("demo_id = ? AND is_latest = ?", demoID, true).
			Update("is_latest", false).Error; err != nil {
			return fmt.Errorf("failed to update previous version: %w", err)
		}

		// 6. 创建新版本
		newVersion := &models.ModuleDemoVersion{
			DemoID:           demoID,
			Version:          maxVersion + 1,
			IsLatest:         true,
			ConfigData:       configData,
			ChangeSummary:    changeSummary,
			ChangeType:       "update",
			DiffFromPrevious: diff,
			CreatedBy:        userID,
		}

		if err := tx.Create(newVersion).Error; err != nil {
			return fmt.Errorf("failed to create new version: %w", err)
		}

		// 7. 更新 demo 的 current_version_id
		if err := tx.Model(&models.ModuleDemo{}).
			Where("id = ?", demoID).
			Update("current_version_id", newVersion.ID).Error; err != nil {
			return fmt.Errorf("failed to update current version: %w", err)
		}

		return nil
	})
}

// GetDemosByModuleID 获取模块的所有演示配置（兼容旧接口）
func (s *ModuleDemoService) GetDemosByModuleID(moduleID uint) ([]models.ModuleDemo, error) {
	return s.GetDemosByModuleIDAndVersion(moduleID, "")
}

// GetDemosByModuleIDAndVersion 获取模块指定版本的演示配置
// 如果 versionID 为空，则返回默认版本的 Demo
func (s *ModuleDemoService) GetDemosByModuleIDAndVersion(moduleID uint, versionID string) ([]models.ModuleDemo, error) {
	db := s.db
	var demos []models.ModuleDemo

	if versionID != "" {
		// 按指定版本过滤
		err := db.Where("module_id = ? AND module_version_id = ? AND is_active = ?", moduleID, versionID, true).
			Preload("CurrentVersion").
			Preload("Creator").
			Order("created_at DESC").
			Find(&demos).Error
		return demos, err
	}

	// 不传 versionID：获取默认版本的 Demo
	var module models.Module
	if err := db.First(&module, moduleID).Error; err != nil {
		return nil, err
	}

	if module.DefaultVersionID != nil && *module.DefaultVersionID != "" {
		// 获取默认版本的 Demo
		err := db.Where("module_id = ? AND module_version_id = ? AND is_active = ?", moduleID, *module.DefaultVersionID, true).
			Preload("CurrentVersion").
			Preload("Creator").
			Order("created_at DESC").
			Find(&demos).Error
		return demos, err
	}

	// 兼容旧数据：如果没有默认版本，返回所有 Demo
	err := db.Where("module_id = ? AND is_active = ?", moduleID, true).
		Preload("CurrentVersion").
		Preload("Creator").
		Order("created_at DESC").
		Find(&demos).Error
	return demos, err
}

// ImportDemosFromVersion 从指定版本导入 Demo 到目标版本
// 返回导入的 Demo 数量
func (s *ModuleDemoService) ImportDemosFromVersion(moduleID uint, targetVersionID, sourceVersionID string, demoIDs []uint, userID *string) (int, error) {
	db := s.db

	// 获取源版本的 Demo
	var sourceDemos []models.ModuleDemo
	query := db.Where("module_id = ? AND module_version_id = ? AND is_active = ?", moduleID, sourceVersionID, true)
	if len(demoIDs) > 0 {
		query = query.Where("id IN ?", demoIDs)
	}
	if err := query.Preload("CurrentVersion").Find(&sourceDemos).Error; err != nil {
		return 0, err
	}

	if len(sourceDemos) == 0 {
		return 0, nil
	}

	importedCount := 0
	err := db.Transaction(func(tx *gorm.DB) error {
		for _, demo := range sourceDemos {
			// 检查目标版本是否已有同名 Demo
			var existingCount int64
			tx.Model(&models.ModuleDemo{}).
				Where("module_id = ? AND module_version_id = ? AND name = ? AND is_active = ?", moduleID, targetVersionID, demo.Name, true).
				Count(&existingCount)
			if existingCount > 0 {
				continue // 跳过已存在的
			}

			// 创建新 Demo
			newDemo := models.ModuleDemo{
				ModuleID:            moduleID,
				ModuleVersionID:     &targetVersionID,
				Name:                demo.Name,
				Description:         demo.Description,
				IsActive:            true,
				UsageNotes:          demo.UsageNotes,
				InheritedFromDemoID: &demo.ID,
				CreatedBy:           userID,
			}

			if err := tx.Create(&newDemo).Error; err != nil {
				return err
			}

			// 如果源 Demo 有当前版本，复制版本数据
			if demo.CurrentVersion != nil {
				newDemoVersion := models.ModuleDemoVersion{
					DemoID:        newDemo.ID,
					Version:       1, // Demo 版本号从 1 开始
					IsLatest:      true,
					ConfigData:    demo.CurrentVersion.ConfigData,
					ChangeSummary: fmt.Sprintf("Imported from demo %d (version %s)", demo.ID, sourceVersionID),
					ChangeType:    "create",
					CreatedBy:     userID,
				}

				if err := tx.Create(&newDemoVersion).Error; err != nil {
					return err
				}

				// 更新 Demo 的 current_version_id
				if err := tx.Model(&newDemo).Update("current_version_id", newDemoVersion.ID).Error; err != nil {
					return err
				}
			}

			importedCount++
		}
		return nil
	})

	return importedCount, err
}

// GetDemoByID 获取演示配置详情
func (s *ModuleDemoService) GetDemoByID(demoID uint) (*models.ModuleDemo, error) {
	db := s.db
	var demo models.ModuleDemo

	err := db.Preload("CurrentVersion").
		Preload("Creator").
		Preload("Module").
		First(&demo, demoID).Error

	if err != nil {
		return nil, err
	}

	return &demo, nil
}

// GetVersionsByDemoID 获取演示配置的所有版本
func (s *ModuleDemoService) GetVersionsByDemoID(demoID uint) ([]models.ModuleDemoVersion, error) {
	db := s.db
	var versions []models.ModuleDemoVersion

	err := db.Where("demo_id = ?", demoID).
		Preload("Creator").
		Order("version DESC").
		Find(&versions).Error

	return versions, err
}

// GetVersionByID 获取特定版本详情
func (s *ModuleDemoService) GetVersionByID(versionID uint) (*models.ModuleDemoVersion, error) {
	db := s.db
	var version models.ModuleDemoVersion

	err := db.Preload("Creator").
		Preload("Demo").
		First(&version, versionID).Error

	if err != nil {
		return nil, err
	}

	return &version, nil
}

// CompareVersions 对比两个版本
func (s *ModuleDemoService) CompareVersions(versionID1, versionID2 uint) (map[string]interface{}, error) {
	db := s.db

	var version1, version2 models.ModuleDemoVersion
	if err := db.First(&version1, versionID1).Error; err != nil {
		return nil, fmt.Errorf("version1 not found: %w", err)
	}
	if err := db.First(&version2, versionID2).Error; err != nil {
		return nil, fmt.Errorf("version2 not found: %w", err)
	}

	// 确保两个版本属于同一个 demo
	if version1.DemoID != version2.DemoID {
		return nil, errors.New("versions belong to different demos")
	}

	diff, err := s.calculateDiff(version1.ConfigData, version2.ConfigData)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"version1":    version1,
		"version2":    version2,
		"diff":        diff,
		"has_changes": diff != "",
	}, nil
}

// RollbackToVersion 回滚到指定版本
func (s *ModuleDemoService) RollbackToVersion(demoID, versionID uint, userID *string) error {
	db := s.db

	return db.Transaction(func(tx *gorm.DB) error {
		// 1. 获取目标版本
		var targetVersion models.ModuleDemoVersion
		if err := tx.First(&targetVersion, versionID).Error; err != nil {
			return fmt.Errorf("target version not found: %w", err)
		}

		// 2. 验证版本属于该 demo
		if targetVersion.DemoID != demoID {
			return errors.New("version does not belong to this demo")
		}

		// 3. 获取当前最新版本号
		var maxVersion int
		if err := tx.Model(&models.ModuleDemoVersion{}).
			Where("demo_id = ?", demoID).
			Select("COALESCE(MAX(version), 0)").
			Scan(&maxVersion).Error; err != nil {
			return fmt.Errorf("failed to get max version: %w", err)
		}

		// 4. 获取当前版本用于计算差异
		var currentVersion models.ModuleDemoVersion
		if err := tx.Where("demo_id = ? AND is_latest = ?", demoID, true).
			First(&currentVersion).Error; err != nil {
			return fmt.Errorf("current version not found: %w", err)
		}

		// 5. 计算差异
		diff, err := s.calculateDiff(currentVersion.ConfigData, targetVersion.ConfigData)
		if err != nil {
			diff = ""
		}

		// 6. 将之前的版本标记为非最新
		if err := tx.Model(&models.ModuleDemoVersion{}).
			Where("demo_id = ? AND is_latest = ?", demoID, true).
			Update("is_latest", false).Error; err != nil {
			return fmt.Errorf("failed to update previous version: %w", err)
		}

		// 7. 创建新版本（回滚版本）
		rollbackVersion := &models.ModuleDemoVersion{
			DemoID:           demoID,
			Version:          maxVersion + 1,
			IsLatest:         true,
			ConfigData:       targetVersion.ConfigData,
			ChangeSummary:    fmt.Sprintf("Rollback to version %d", targetVersion.Version),
			ChangeType:       "rollback",
			DiffFromPrevious: diff,
			CreatedBy:        userID,
		}

		if err := tx.Create(rollbackVersion).Error; err != nil {
			return fmt.Errorf("failed to create rollback version: %w", err)
		}

		// 8. 更新 demo 的 current_version_id
		if err := tx.Model(&models.ModuleDemo{}).
			Where("id = ?", demoID).
			Update("current_version_id", rollbackVersion.ID).Error; err != nil {
			return fmt.Errorf("failed to update current version: %w", err)
		}

		return nil
	})
}

// DeleteDemo 删除演示配置（软删除）
func (s *ModuleDemoService) DeleteDemo(demoID uint) error {
	db := s.db

	return db.Model(&models.ModuleDemo{}).
		Where("id = ?", demoID).
		Update("is_active", false).Error
}

// calculateDiff 计算两个配置的差异
func (s *ModuleDemoService) calculateDiff(oldConfig, newConfig map[string]interface{}) (string, error) {
	// 简单的 JSON diff 实现
	oldJSON, err := json.MarshalIndent(oldConfig, "", "  ")
	if err != nil {
		return "", err
	}

	newJSON, err := json.MarshalIndent(newConfig, "", "  ")
	if err != nil {
		return "", err
	}

	// 如果完全相同，返回空字符串
	if string(oldJSON) == string(newJSON) {
		return "", nil
	}

	// 返回简单的差异描述
	diff := map[string]interface{}{
		"old":       oldConfig,
		"new":       newConfig,
		"timestamp": time.Now().Format(time.RFC3339),
	}

	diffJSON, err := json.MarshalIndent(diff, "", "  ")
	if err != nil {
		return "", err
	}

	return string(diffJSON), nil
}
