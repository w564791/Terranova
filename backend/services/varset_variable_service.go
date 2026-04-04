package services

import (
	"fmt"
	"iac-platform/internal/models"
	"time"

	"gorm.io/gorm"
)

// VarsetVariableService 变量集变量服务
type VarsetVariableService struct {
	db *gorm.DB
}

// NewVarsetVariableService 创建变量集变量服务实例
func NewVarsetVariableService(db *gorm.DB) *VarsetVariableService {
	return &VarsetVariableService{db: db}
}

// Create 创建变量集变量
func (s *VarsetVariableService) Create(varsetID, key, value, description string, varType models.VariableType, valFormat models.ValueFormat, sensitive bool, createdBy *string) (*models.VarsetVariable, error) {
	// 检查变量集是否存在
	var varset models.VariableSet
	if err := s.db.Where("varset_id = ? AND is_deleted = ?", varsetID, false).First(&varset).Error; err != nil {
		return nil, fmt.Errorf("变量集不存在: %w", err)
	}

	// 检查是否存在同名的活跃变量（varset_id + key，不区分类型）
	var existing models.VarsetVariable
	err := s.db.Where("varset_id = ? AND key = ? AND is_deleted = ?",
		varsetID, key, false).
		First(&existing).Error
	if err == nil {
		return nil, fmt.Errorf("变量 %s 已存在", key)
	} else if err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("查询变量失败: %w", err)
	}

	variable := &models.VarsetVariable{
		VarsetID:     varsetID,
		Key:          key,
		Value:        value,
		Description:  description,
		VariableType: varType,
		ValueFormat:  valFormat,
		Sensitive:    sensitive,
		Version:      1,
		IsDeleted:    false,
		CreatedBy:    createdBy,
	}

	if err := s.db.Create(variable).Error; err != nil {
		return nil, fmt.Errorf("创建变量失败: %w", err)
	}

	// 重新查询以触发 AfterFind 解密
	var result models.VarsetVariable
	if err := s.db.Where("id = ?", variable.ID).First(&result).Error; err != nil {
		return nil, fmt.Errorf("查询创建的变量失败: %w", err)
	}

	return &result, nil
}

// List 获取变量集的变量列表（只获取每个 variable_id 的最新版本）
func (s *VarsetVariableService) List(varsetID string, varType string) ([]models.VarsetVariable, error) {
	var variables []models.VarsetVariable

	subQuery := s.db.Table("varset_variables").
		Select("variable_id, MAX(version) as max_version").
		Where("varset_id = ? AND is_deleted = false", varsetID).
		Group("variable_id")

	query := s.db.Table("varset_variables").
		Joins("INNER JOIN (?) AS latest ON varset_variables.variable_id = latest.variable_id AND varset_variables.version = latest.max_version", subQuery).
		Where("varset_variables.varset_id = ? AND varset_variables.is_deleted = false", varsetID)

	if varType != "" && varType != "all" {
		query = query.Where("varset_variables.variable_type = ?", varType)
	}

	if err := query.Order("varset_variables.key ASC").Find(&variables).Error; err != nil {
		return nil, fmt.Errorf("获取变量列表失败: %w", err)
	}

	return variables, nil
}

// GetByVariableID 通过 varset_id + variable_id 获取最新版本变量
func (s *VarsetVariableService) GetByVariableID(varsetID, variableID string) (*models.VarsetVariable, error) {
	var variable models.VarsetVariable
	if err := s.db.Where("varset_id = ? AND variable_id = ? AND is_deleted = false", varsetID, variableID).
		Order("version DESC").
		First(&variable).Error; err != nil {
		return nil, fmt.Errorf("变量不存在: %w", err)
	}
	return &variable, nil
}

// Update 更新变量（创建新版本，而非就地修改）
func (s *VarsetVariableService) Update(varsetID, variableID string, value, description *string, sensitive *bool) (*models.VarsetVariable, error) {
	// 获取现有变量（最新版本）
	existing, err := s.GetByVariableID(varsetID, variableID)
	if err != nil {
		return nil, err
	}

	// 安全约束：禁止将敏感变量降级为非敏感
	if sensitive != nil && existing.Sensitive && !*sensitive {
		return nil, fmt.Errorf("不允许将敏感变量降级为非敏感，如需更改请删除后重新创建")
	}

	// Build new version based on existing
	newVar := &models.VarsetVariable{
		VariableID:   existing.VariableID, // Same variable_id
		VarsetID:     existing.VarsetID,
		Key:          existing.Key,          // Immutable
		VariableType: existing.VariableType, // Immutable
		ValueFormat:  existing.ValueFormat,  // Immutable
		Description:  existing.Description,
		Sensitive:    existing.Sensitive,
		CreatedBy:    existing.CreatedBy,
		Value:        existing.Value,
	}

	// Apply changes
	if value != nil {
		newVar.Value = *value
	}
	if description != nil {
		newVar.Description = *description
	}
	if sensitive != nil {
		newVar.Sensitive = *sensitive
	}

	// Calculate next version
	var maxVersion int
	s.db.Model(&models.VarsetVariable{}).
		Where("variable_id = ?", variableID).
		Select("COALESCE(MAX(version), 0)").
		Row().Scan(&maxVersion)
	newVar.Version = maxVersion + 1

	// Create new version row (BeforeCreate won't generate new variable_id since it's non-empty)
	if err := s.db.Create(newVar).Error; err != nil {
		return nil, fmt.Errorf("更新变量失败: %w", err)
	}

	// Re-query to get decrypted value
	var result models.VarsetVariable
	if err := s.db.Where("id = ?", newVar.ID).First(&result).Error; err != nil {
		return nil, fmt.Errorf("查询更新后的变量失败: %w", err)
	}

	return &result, nil
}

// Delete 软删除变量（标记所有版本为已删除）
func (s *VarsetVariableService) Delete(varsetID, variableID string) error {
	result := s.db.Model(&models.VarsetVariable{}).
		Where("varset_id = ? AND variable_id = ? AND is_deleted = ?", varsetID, variableID, false).
		Updates(map[string]interface{}{
			"is_deleted": true,
			"updated_at": time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("删除变量失败: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("变量不存在")
	}

	return nil
}
