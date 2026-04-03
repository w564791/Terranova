package services

import (
	"fmt"
	"iac-platform/internal/crypto"
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

	// 检查是否存在同名的活跃变量（varset_id + key + variable_type）
	var existing models.VarsetVariable
	err := s.db.Where("varset_id = ? AND key = ? AND variable_type = ? AND is_deleted = ?",
		varsetID, key, varType, false).
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

// List 获取变量集的变量列表
func (s *VarsetVariableService) List(varsetID string, varType string) ([]models.VarsetVariable, error) {
	query := s.db.Where("varset_id = ? AND is_deleted = ?", varsetID, false)

	// 可选类型过滤
	if varType != "" && varType != "all" {
		query = query.Where("variable_type = ?", varType)
	}

	var variables []models.VarsetVariable
	if err := query.Order("key ASC").Find(&variables).Error; err != nil {
		return nil, fmt.Errorf("获取变量列表失败: %w", err)
	}

	return variables, nil
}

// GetByVariableID 通过 varset_id + variable_id 获取变量
func (s *VarsetVariableService) GetByVariableID(varsetID, variableID string) (*models.VarsetVariable, error) {
	var variable models.VarsetVariable
	if err := s.db.Where("varset_id = ? AND variable_id = ? AND is_deleted = ?",
		varsetID, variableID, false).
		First(&variable).Error; err != nil {
		return nil, fmt.Errorf("变量不存在: %w", err)
	}
	return &variable, nil
}

// Update 更新变量（仅更新提供的字段）
func (s *VarsetVariableService) Update(varsetID, variableID string, value, description *string, sensitive *bool) (*models.VarsetVariable, error) {
	// 获取现有变量
	existing, err := s.GetByVariableID(varsetID, variableID)
	if err != nil {
		return nil, err
	}

	// 安全约束：禁止将敏感变量降级为非敏感
	if sensitive != nil && existing.Sensitive && !*sensitive {
		return nil, fmt.Errorf("不允许将敏感变量降级为非敏感，如需更改请删除后重新创建")
	}

	// 构建更新字段
	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}
	if description != nil {
		updates["description"] = *description
	}
	if sensitive != nil {
		updates["sensitive"] = *sensitive
	}

	// 判断更新后是否为 sensitive（当前已 sensitive，或本次设为 sensitive）
	willBeSensitive := existing.Sensitive || (sensitive != nil && *sensitive)

	if value != nil {
		val := *value
		// map-based Updates 绕过 GORM hooks，需手动加密
		if willBeSensitive && val != "" {
			encrypted, encErr := crypto.EncryptValue(val)
			if encErr != nil {
				return nil, fmt.Errorf("加密变量失败: %w", encErr)
			}
			val = encrypted
		}
		updates["value"] = val
	} else if !existing.Sensitive && (sensitive != nil && *sensitive) {
		// 未提供新值但切换为 sensitive：加密现有值
		if existing.Value != "" {
			encrypted, encErr := crypto.EncryptValue(existing.Value)
			if encErr != nil {
				return nil, fmt.Errorf("加密变量失败: %w", encErr)
			}
			updates["value"] = encrypted
		}
	}

	if err := s.db.Model(existing).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("更新变量失败: %w", err)
	}

	// 重新查询以获取解密后的值
	var result models.VarsetVariable
	if err := s.db.Where("id = ?", existing.ID).First(&result).Error; err != nil {
		return nil, fmt.Errorf("查询更新后的变量失败: %w", err)
	}

	return &result, nil
}

// Delete 软删除变量
func (s *VarsetVariableService) Delete(varsetID, variableID string) error {
	result := s.db.Model(&models.VarsetVariable{}).
		Where("varset_id = ? AND variable_id = ? AND is_deleted = ?", varsetID, variableID, false).
		Update("is_deleted", true)

	if result.Error != nil {
		return fmt.Errorf("删除变量失败: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("变量不存在")
	}

	return nil
}
