package services

import (
	"fmt"
	"iac-platform/internal/models"
	"log"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ModuleVersionSkillService Module 版本 Skill 服务
type ModuleVersionSkillService struct {
	db            *gorm.DB
	moduleSkillAI *ModuleSkillAIService
}

// NewModuleVersionSkillService 创建服务实例
func NewModuleVersionSkillService(db *gorm.DB) *ModuleVersionSkillService {
	return &ModuleVersionSkillService{
		db:            db,
		moduleSkillAI: NewModuleSkillAIService(db),
	}
}

// GetByVersionID 根据版本 ID 获取 Skill
func (s *ModuleVersionSkillService) GetByVersionID(versionID string) (*models.ModuleVersionSkill, error) {
	var skill models.ModuleVersionSkill
	err := s.db.Where("module_version_id = ?", versionID).First(&skill).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil // 没有找到，返回 nil
		}
		return nil, err
	}
	return &skill, nil
}

// GetOrCreate 获取或创建 Skill 记录
func (s *ModuleVersionSkillService) GetOrCreate(versionID string, userID string) (*models.ModuleVersionSkill, error) {
	skill, err := s.GetByVersionID(versionID)
	if err != nil {
		return nil, err
	}

	if skill != nil {
		return skill, nil
	}

	// 创建新记录
	skill = &models.ModuleVersionSkill{
		ID:              uuid.New().String(),
		ModuleVersionID: versionID,
		IsActive:        true,
		CreatedBy:       userID,
	}

	if err := s.db.Create(skill).Error; err != nil {
		return nil, err
	}

	return skill, nil
}

// UpdateCustomContent 更新用户自定义内容
func (s *ModuleVersionSkillService) UpdateCustomContent(versionID string, customContent string, userID string) (*models.ModuleVersionSkill, error) {
	skill, err := s.GetOrCreate(versionID, userID)
	if err != nil {
		return nil, err
	}

	skill.CustomContent = customContent
	skill.UpdatedAt = time.Now()

	if err := s.db.Save(skill).Error; err != nil {
		return nil, err
	}

	return skill, nil
}

// GenerateFromSchema 根据 Schema 生成 Skill（使用 AI）
func (s *ModuleVersionSkillService) GenerateFromSchema(versionID string, userID string) (*models.ModuleVersionSkill, error) {
	// 获取版本信息
	var version models.ModuleVersion
	if err := s.db.First(&version, "id = ?", versionID).Error; err != nil {
		return nil, fmt.Errorf("版本不存在: %w", err)
	}

	// 获取或创建 Skill 记录
	skill, err := s.GetOrCreate(versionID, userID)
	if err != nil {
		return nil, err
	}

	// 获取当前版本的最新 Schema
	latestSchema, err := GetLatestSchema(s.db, versionID)
	if err != nil {
		return nil, fmt.Errorf("查询 Schema 失败: %w", err)
	}
	if latestSchema == nil {
		return nil, fmt.Errorf("该版本没有关联的 Schema，无法生成 Skill")
	}
	schema := *latestSchema

	// 获取 Module 信息
	var module models.Module
	if err := s.db.First(&module, "id = ?", version.ModuleID).Error; err != nil {
		return nil, fmt.Errorf("Module 不存在: %w", err)
	}

	// 使用 AI 生成 Skill 内容
	log.Printf("[ModuleVersionSkillService] 开始使用 AI 生成 Skill，Module: %s, Version: %s", module.Name, version.Version)
	generatedContent, err := s.moduleSkillAI.GenerateModuleSkillContent(&module, &schema)
	if err != nil {
		log.Printf("[ModuleVersionSkillService] AI 生成失败: %v，使用占位内容", err)
		// AI 生成失败时使用占位内容
		generatedContent = s.generatePlaceholderSkill(&version, &schema)
	}

	now := time.Now()
	skill.SchemaGeneratedContent = generatedContent
	skill.SchemaGeneratedAt = &now
	// 解析 schema.Version 字符串为整数
	var schemaVersionInt int
	fmt.Sscanf(schema.Version, "%d", &schemaVersionInt)
	skill.SchemaVersionUsed = &schemaVersionInt
	skill.UpdatedAt = now

	if err := s.db.Save(skill).Error; err != nil {
		return nil, err
	}

	log.Printf("[ModuleVersionSkillService] Skill 生成完成，Version: %s", version.Version)
	return skill, nil
}

// generatePlaceholderSkill 生成占位 Skill 内容
func (s *ModuleVersionSkillService) generatePlaceholderSkill(version *models.ModuleVersion, schema *models.Schema) string {
	// 获取 Module 信息
	var module models.Module
	s.db.First(&module, "id = ?", version.ModuleID)

	content := fmt.Sprintf(`# %s Module Skill

## 模块概述
- **模块名称**: %s
- **Provider**: %s
- **版本**: %s
- **Schema 版本**: v%s

## 使用说明

此 Skill 由系统根据 Schema 自动生成。

### 主要功能
该模块用于管理 %s 相关的基础设施资源。

### 配置要点
请根据 Schema 定义的字段进行配置。

---

*此内容由 AI 自动生成，可能需要人工审核和补充。*
`, module.Name, module.Name, module.Provider, version.Version, schema.Version, module.Provider)

	return content
}

// InheritFromVersion 从其他版本继承 Skill
func (s *ModuleVersionSkillService) InheritFromVersion(targetVersionID string, sourceVersionID string, userID string) (*models.ModuleVersionSkill, error) {
	// 获取源版本的 Skill
	sourceSkill, err := s.GetByVersionID(sourceVersionID)
	if err != nil {
		return nil, err
	}
	if sourceSkill == nil {
		return nil, fmt.Errorf("源版本没有 Skill 记录")
	}

	// 获取或创建目标版本的 Skill
	targetSkill, err := s.GetOrCreate(targetVersionID, userID)
	if err != nil {
		return nil, err
	}

	// 复制内容
	targetSkill.SchemaGeneratedContent = sourceSkill.SchemaGeneratedContent
	targetSkill.SchemaGeneratedAt = sourceSkill.SchemaGeneratedAt
	targetSkill.SchemaVersionUsed = sourceSkill.SchemaVersionUsed
	targetSkill.CustomContent = sourceSkill.CustomContent
	targetSkill.InheritedFromVersionID = &sourceVersionID
	targetSkill.UpdatedAt = time.Now()

	if err := s.db.Save(targetSkill).Error; err != nil {
		return nil, err
	}

	return targetSkill, nil
}

// Delete 删除 Skill
func (s *ModuleVersionSkillService) Delete(versionID string) error {
	return s.db.Where("module_version_id = ?", versionID).Delete(&models.ModuleVersionSkill{}).Error
}

// ListByModuleID 获取模块所有版本的 Skill
func (s *ModuleVersionSkillService) ListByModuleID(moduleID uint) ([]models.ModuleVersionSkill, error) {
	var skills []models.ModuleVersionSkill

	// 先获取模块的所有版本
	var versions []models.ModuleVersion
	if err := s.db.Where("module_id = ?", moduleID).Find(&versions).Error; err != nil {
		return nil, err
	}

	if len(versions) == 0 {
		return skills, nil
	}

	// 获取版本 ID 列表
	versionIDs := make([]string, len(versions))
	for i, v := range versions {
		versionIDs[i] = v.ID
	}

	// 查询 Skill
	if err := s.db.Where("module_version_id IN ?", versionIDs).Find(&skills).Error; err != nil {
		return nil, err
	}

	return skills, nil
}
