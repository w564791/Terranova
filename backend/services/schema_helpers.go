package services

import (
	"fmt"
	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// GetLatestSchema 获取指定 ModuleVersion 的最新 Schema（按 updated_at DESC）
// 这是解析"哪个 Schema 是活跃的"的唯一方式
// 使用 updated_at 而非 created_at，确保用户编辑过的 Schema 优先返回
func GetLatestSchema(db *gorm.DB, moduleVersionID string) (*models.Schema, error) {
	var schema models.Schema
	err := db.Where("module_version_id = ?", moduleVersionID).
		Order("updated_at DESC").
		First(&schema).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &schema, nil
}

// GetLatestSchemaV2 同上，但限定 schema_version = 'v2'
func GetLatestSchemaV2(db *gorm.DB, moduleVersionID string) (*models.Schema, error) {
	var schema models.Schema
	err := db.Where("module_version_id = ? AND schema_version = ?", moduleVersionID, "v2").
		Order("updated_at DESC").
		First(&schema).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &schema, nil
}

// GetNextSchemaVersionForModuleVersion 获取 ModuleVersion 的下一个 Schema 版本号
// 作用域: per-ModuleVersion（非全局 per-Module）
func GetNextSchemaVersionForModuleVersion(db *gorm.DB, moduleVersionID string) string {
	var maxVersion int
	if err := db.Model(&models.Schema{}).
		Where("module_version_id = ? AND version ~ '^[0-9]+$'", moduleVersionID).
		Select("COALESCE(MAX(CAST(version AS INTEGER)), 0)").
		Scan(&maxVersion).Error; err != nil {
		var count int64
		db.Model(&models.Schema{}).Where("module_version_id = ?", moduleVersionID).Count(&count)
		return fmt.Sprintf("%d", count+1)
	}
	return fmt.Sprintf("%d", maxVersion+1)
}
