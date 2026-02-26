package services

import (
	"fmt"
	"iac-platform/internal/crypto"
	"iac-platform/internal/infrastructure"
	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// WorkspaceVariableService Workspace变量服务
type WorkspaceVariableService struct {
	db *gorm.DB
}

// GetDB 获取数据库连接
func (s *WorkspaceVariableService) GetDB() *gorm.DB {
	return s.db
}

// NewWorkspaceVariableService 创建变量服务实例
func NewWorkspaceVariableService(db *gorm.DB) *WorkspaceVariableService {
	return &WorkspaceVariableService{db: db}
}

// CreateVariable 创建变量
func (s *WorkspaceVariableService) CreateVariable(variable *models.WorkspaceVariable) error {
	// 检查workspace是否存在（使用workspace_id字段）
	var workspace models.Workspace
	if err := s.db.Where("workspace_id = ?", variable.WorkspaceID).First(&workspace).Error; err != nil {
		return fmt.Errorf("workspace不存在: %w", err)
	}

	// 检查是否存在同名的活跃变量
	var existing models.WorkspaceVariable
	err := s.db.Where("workspace_id = ? AND key = ? AND variable_type = ? AND is_deleted = ?",
		variable.WorkspaceID, variable.Key, variable.VariableType, false).
		First(&existing).Error

	if err == nil {
		return fmt.Errorf("变量 %s 已存在", variable.Key)
	} else if err != gorm.ErrRecordNotFound {
		return fmt.Errorf("查询变量失败: %w", err)
	}

	// 查询该 key 的历史最大版本号（包括已删除的），避免版本号冲突
	var maxVersion int
	s.db.Model(&models.WorkspaceVariable{}).
		Where("workspace_id = ? AND key = ? AND variable_type = ?",
			variable.WorkspaceID, variable.Key, variable.VariableType).
		Select("COALESCE(MAX(version), 0)").
		Scan(&maxVersion)

	// 始终生成全新的 variable_id
	varID, err := infrastructure.GenerateVariableID()
	if err != nil {
		return fmt.Errorf("生成variable_id失败: %w", err)
	}

	variable.VariableID = varID
	variable.Version = maxVersion + 1
	variable.IsDeleted = false

	// 手动处理加密
	if variable.Sensitive && variable.Value != "" && !crypto.IsEncrypted(variable.Value) {
		encrypted, err := crypto.EncryptValue(variable.Value)
		if err != nil {
			return fmt.Errorf("加密失败: %w", err)
		}
		variable.Value = encrypted
	}

	// 使用原生 SQL 插入，避免 GORM hooks
	sql := `
		INSERT INTO workspace_variables (
			variable_id, workspace_id, key, version, value,
			variable_type, value_format, sensitive, description,
			is_deleted, created_at, updated_at, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW(), $11)
		RETURNING id
	`

	var newID uint
	if err := s.db.Raw(sql,
		variable.VariableID,
		variable.WorkspaceID,
		variable.Key,
		variable.Version,
		variable.Value,
		variable.VariableType,
		variable.ValueFormat,
		variable.Sensitive,
		variable.Description,
		variable.IsDeleted,
		variable.CreatedBy,
	).Scan(&newID).Error; err != nil {
		return fmt.Errorf("创建变量失败: %w", err)
	}

	variable.ID = newID

	return nil
}

// GetVariable 获取单个变量
func (s *WorkspaceVariableService) GetVariable(id uint) (*models.WorkspaceVariable, error) {
	var variable models.WorkspaceVariable
	if err := s.db.First(&variable, id).Error; err != nil {
		return nil, fmt.Errorf("变量不存在: %w", err)
	}
	return &variable, nil
}

// GetVariableByKey 根据key获取变量
func (s *WorkspaceVariableService) GetVariableByKey(workspaceID uint, key string) (*models.WorkspaceVariable, error) {
	var variable models.WorkspaceVariable
	if err := s.db.Where("workspace_id = ? AND key = ?", workspaceID, key).First(&variable).Error; err != nil {
		return nil, fmt.Errorf("变量不存在: %w", err)
	}
	return &variable, nil
}

// ListVariables 获取workspace的变量列表（只返回最新未删除版本）
func (s *WorkspaceVariableService) ListVariables(workspaceID string, variableType string) ([]*models.WorkspaceVariable, error) {
	// 修复：先获取所有 variable_id 的最新版本（不过滤 is_deleted）
	// 然后在主查询中过滤 is_deleted = false
	subQuery := s.db.Table("workspace_variables").
		Select("variable_id, MAX(version) as max_version").
		Where("workspace_id = ?", workspaceID).  // 不过滤 is_deleted
		Group("variable_id")

	query := s.db.Table("workspace_variables").
		Joins("INNER JOIN (?) AS latest ON workspace_variables.variable_id = latest.variable_id AND workspace_variables.version = latest.max_version", subQuery).
		Where("workspace_variables.workspace_id = ? AND workspace_variables.is_deleted = ?", workspaceID, false)  // 在这里过滤

	// 如果指定了类型，添加类型过滤
	if variableType != "" && variableType != "all" {
		query = query.Where("workspace_variables.variable_type = ?", variableType)
	}

	var variables []*models.WorkspaceVariable
	if err := query.Order("workspace_variables.key ASC").Find(&variables).Error; err != nil {
		return nil, fmt.Errorf("获取变量列表失败: %w", err)
	}

	return variables, nil
}

// VariableUpdateResult 变量更新结果
type VariableUpdateResult struct {
	VariableID   string `json:"variable_id"`
	OldVersion   int    `json:"old_version"`
	NewVersion   int    `json:"new_version"`
	NewVariable  *models.WorkspaceVariable `json:"-"` // 不序列化到JSON，仅内部使用
}

// UpdateVariable 更新变量（创建新版本，带乐观锁版本检查）
// 返回: variable_id, 旧版本号, 新版本号
// expectedVersion: 客户端期望的当前版本号，用于乐观锁检查
func (s *WorkspaceVariableService) UpdateVariable(id uint, expectedVersion int, updates map[string]interface{}) (*VariableUpdateResult, error) {
	// 查询当前变量
	var current models.WorkspaceVariable
	if err := s.db.First(&current, id).Error; err != nil {
		return nil, fmt.Errorf("变量不存在: %w", err)
	}

	// 乐观锁检查：验证客户端提供的版本号是否与当前版本号一致
	if expectedVersion != current.Version {
		return nil, fmt.Errorf("版本冲突：当前版本为 %d，您提供的版本为 %d，变量已被其他用户修改，请刷新后重试",
			current.Version, expectedVersion)
	}

	// 安全约束：禁止将敏感变量降级为非敏感（sensitive: true → false）
	// 只允许升级（false → true），防止已加密的值以明文存入新版本
	if newSensitive, ok := updates["sensitive"].(bool); ok {
		if current.Sensitive && !newSensitive {
			return nil, fmt.Errorf("不允许将敏感变量降级为非敏感，如需更改请删除后重新创建")
		}
	}

	// 如果更新key，检查新key是否已存在
	if newKey, ok := updates["key"].(string); ok && newKey != current.Key {
		var existing models.WorkspaceVariable
		err := s.db.Where("workspace_id = ? AND key = ? AND variable_type = ? AND is_deleted = ?",
			current.WorkspaceID, newKey, current.VariableType, false).
			Order("version DESC").
			First(&existing).Error
		if err == nil {
			return nil, fmt.Errorf("变量 %s 已存在", newKey)
		}
	}

	// 创建新版本 - 明确构造新对象而不是复制
	newVersion := models.WorkspaceVariable{
		// ID 不设置，让数据库自动生成
		VariableID:   current.VariableID,  // 保持不变
		WorkspaceID:  current.WorkspaceID,
		Key:          current.Key,
		Value:        current.Value,
		VariableType: current.VariableType,
		ValueFormat:  current.ValueFormat,
		Sensitive:    current.Sensitive,
		Description:  current.Description,
		IsDeleted:    false,
		CreatedBy:    current.CreatedBy,
	}

	// 应用更新
	if key, ok := updates["key"].(string); ok {
		newVersion.Key = key
	}
	if value, ok := updates["value"].(string); ok {
		newVersion.Value = value
	}
	if varType, ok := updates["variable_type"].(models.VariableType); ok {
		newVersion.VariableType = varType
	}
	if valueFormat, ok := updates["value_format"].(models.ValueFormat); ok {
		newVersion.ValueFormat = valueFormat
	}
	if sensitive, ok := updates["sensitive"].(bool); ok {
		newVersion.Sensitive = sensitive
	}
	if description, ok := updates["description"].(string); ok {
		newVersion.Description = description
	}

	// 查询所有可能冲突的最大版本号，避免唯一约束冲突
	// idx_variable_id_version: (variable_id, version)
	// idx_workspace_key_type_version: (workspace_id, key, variable_type, version)
	var maxVersion int
	s.db.Model(&models.WorkspaceVariable{}).
		Where("variable_id = ? OR (workspace_id = ? AND key = ? AND variable_type = ?)",
			current.VariableID, newVersion.WorkspaceID, newVersion.Key, newVersion.VariableType).
		Select("COALESCE(MAX(version), 0)").
		Scan(&maxVersion)
	newVersion.Version = maxVersion + 1

	// 手动处理加密（因为要使用原生 SQL）
	if newVersion.Sensitive && newVersion.Value != "" && !crypto.IsEncrypted(newVersion.Value) {
		encrypted, err := crypto.EncryptValue(newVersion.Value)
		if err != nil {
			return nil, fmt.Errorf("加密失败: %w", err)
		}
		newVersion.Value = encrypted
	}

	// 使用原生 SQL 插入，完全避免 GORM hooks
	sql := `
		INSERT INTO workspace_variables (
			variable_id, workspace_id, key, version, value,
			variable_type, value_format, sensitive, description,
			is_deleted, created_at, updated_at, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW(), $11)
		RETURNING id
	`
	
	var newID uint
	if err := s.db.Raw(sql,
		newVersion.VariableID,
		newVersion.WorkspaceID,
		newVersion.Key,
		newVersion.Version,
		newVersion.Value,
		newVersion.VariableType,
		newVersion.ValueFormat,
		newVersion.Sensitive,
		newVersion.Description,
		newVersion.IsDeleted,
		newVersion.CreatedBy,
	).Scan(&newID).Error; err != nil {
		return nil, fmt.Errorf("创建新版本失败: %w", err)
	}
	
	newVersion.ID = newID

	// 返回更新结果
	return &VariableUpdateResult{
		VariableID:  current.VariableID,
		OldVersion:  current.Version,
		NewVersion:  newVersion.Version,
		NewVariable: &newVersion,
	}, nil
}

// UpdateVariableByVariableID 通过 variable_id 更新变量（带版本检查）
func (s *WorkspaceVariableService) UpdateVariableByVariableID(variableID string, expectedVersion int, updates map[string]interface{}) (*VariableUpdateResult, error) {
	// 查询当前最新版本
	var current models.WorkspaceVariable
	if err := s.db.Where("variable_id = ? AND is_deleted = ?", variableID, false).
		Order("version DESC").
		First(&current).Error; err != nil {
		return nil, fmt.Errorf("变量不存在: %w", err)
	}

	// 使用ID更新（传递期望版本号）
	return s.UpdateVariable(current.ID, expectedVersion, updates)
}

// DeleteVariable 删除变量（软删除：标记所有历史版本为已删除，并创建删除版本）
func (s *WorkspaceVariableService) DeleteVariable(id uint) error {
	// 查询当前变量
	var current models.WorkspaceVariable
	if err := s.db.First(&current, id).Error; err != nil {
		return fmt.Errorf("变量不存在: %w", err)
	}

	// 第一步：将该 variable_id 的所有历史版本标记为已删除
	if err := s.db.Model(&models.WorkspaceVariable{}).
		Where("variable_id = ?", current.VariableID).
		Update("is_deleted", true).Error; err != nil {
		return fmt.Errorf("标记历史版本失败: %w", err)
	}

	// 第二步：查询最大版本号，避免唯一约束冲突
	var maxVersion int
	s.db.Model(&models.WorkspaceVariable{}).
		Where("variable_id = ? OR (workspace_id = ? AND key = ? AND variable_type = ?)",
			current.VariableID, current.WorkspaceID, current.Key, current.VariableType).
		Select("COALESCE(MAX(version), 0)").
		Scan(&maxVersion)

	// 创建新的删除版本
	deleteVersion := models.WorkspaceVariable{
		VariableID:   current.VariableID,
		WorkspaceID:  current.WorkspaceID,
		Key:          current.Key,
		Version:      maxVersion + 1,
		Value:        current.Value,
		VariableType: current.VariableType,
		ValueFormat:  current.ValueFormat,
		Sensitive:    current.Sensitive,
		Description:  current.Description,
		IsDeleted:    true,
		CreatedBy:    current.CreatedBy,
	}

	// 手动处理加密
	if deleteVersion.Sensitive && deleteVersion.Value != "" && !crypto.IsEncrypted(deleteVersion.Value) {
		encrypted, err := crypto.EncryptValue(deleteVersion.Value)
		if err != nil {
			return fmt.Errorf("加密失败: %w", err)
		}
		deleteVersion.Value = encrypted
	}

	// 使用原生 SQL 插入删除版本
	sql := `
		INSERT INTO workspace_variables (
			variable_id, workspace_id, key, version, value,
			variable_type, value_format, sensitive, description,
			is_deleted, created_at, updated_at, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW(), $11)
	`
	
	if err := s.db.Exec(sql,
		deleteVersion.VariableID,
		deleteVersion.WorkspaceID,
		deleteVersion.Key,
		deleteVersion.Version,
		deleteVersion.Value,
		deleteVersion.VariableType,
		deleteVersion.ValueFormat,
		deleteVersion.Sensitive,
		deleteVersion.Description,
		deleteVersion.IsDeleted,
		deleteVersion.CreatedBy,
	).Error; err != nil {
		return fmt.Errorf("创建删除版本失败: %w", err)
	}

	return nil
}

// DeleteVariableByVariableID 通过 variable_id 删除变量
func (s *WorkspaceVariableService) DeleteVariableByVariableID(variableID string) error {
	// 查询当前最新版本
	var current models.WorkspaceVariable
	if err := s.db.Where("variable_id = ? AND is_deleted = ?", variableID, false).
		Order("version DESC").
		First(&current).Error; err != nil {
		return fmt.Errorf("变量不存在: %w", err)
	}

	// 使用ID删除
	return s.DeleteVariable(current.ID)
}

// GetVariablesForExecution 获取用于执行的变量（包含敏感变量的值）
func (s *WorkspaceVariableService) GetVariablesForExecution(workspaceID uint) (map[string]string, error) {
	var variables []*models.WorkspaceVariable
	if err := s.db.Where("workspace_id = ?", workspaceID).Find(&variables).Error; err != nil {
		return nil, fmt.Errorf("获取变量失败: %w", err)
	}

	result := make(map[string]string)
	for _, v := range variables {
		result[v.Key] = v.Value
	}

	return result, nil
}

// GetTerraformVariables 获取Terraform变量
func (s *WorkspaceVariableService) GetTerraformVariables(workspaceID uint) (map[string]string, error) {
	var variables []*models.WorkspaceVariable
	if err := s.db.Where("workspace_id = ? AND variable_type = ?",
		workspaceID, models.VariableTypeTerraform).Find(&variables).Error; err != nil {
		return nil, fmt.Errorf("获取Terraform变量失败: %w", err)
	}

	result := make(map[string]string)
	for _, v := range variables {
		result[v.Key] = v.Value
	}

	return result, nil
}

// GetEnvironmentVariables 获取环境变量
func (s *WorkspaceVariableService) GetEnvironmentVariables(workspaceID uint) (map[string]string, error) {
	var variables []*models.WorkspaceVariable
	if err := s.db.Where("workspace_id = ? AND variable_type = ?",
		workspaceID, models.VariableTypeEnvironment).Find(&variables).Error; err != nil {
		return nil, fmt.Errorf("获取环境变量失败: %w", err)
	}

	result := make(map[string]string)
	for _, v := range variables {
		result[v.Key] = v.Value
	}

	return result, nil
}

// BulkCreateVariables 批量创建变量
func (s *WorkspaceVariableService) BulkCreateVariables(variables []*models.WorkspaceVariable) error {
	// 检查workspace是否存在（使用workspace_id字段）
	if len(variables) > 0 {
		var workspace models.Workspace
		if err := s.db.Where("workspace_id = ?", variables[0].WorkspaceID).First(&workspace).Error; err != nil {
			return fmt.Errorf("workspace不存在: %w", err)
		}
	}

	// 批量创建
	if err := s.db.Create(&variables).Error; err != nil {
		return fmt.Errorf("批量创建变量失败: %w", err)
	}

	return nil
}

// GetVariableVersions 获取变量的所有历史版本
func (s *WorkspaceVariableService) GetVariableVersions(variableID string) ([]*models.WorkspaceVariable, error) {
	var versions []*models.WorkspaceVariable
	if err := s.db.Where("variable_id = ?", variableID).
		Order("version DESC").
		Find(&versions).Error; err != nil {
		return nil, fmt.Errorf("获取变量版本历史失败: %w", err)
	}
	return versions, nil
}

// GetVariableVersion 获取变量的指定版本
func (s *WorkspaceVariableService) GetVariableVersion(variableID string, version int) (*models.WorkspaceVariable, error) {
	var variable models.WorkspaceVariable
	if err := s.db.Where("variable_id = ? AND version = ?", variableID, version).
		First(&variable).Error; err != nil {
		return nil, fmt.Errorf("变量版本不存在: %w", err)
	}
	return &variable, nil
}

// GetVariableByVariableID 通过 variable_id 获取最新版本
func (s *WorkspaceVariableService) GetVariableByVariableID(variableID string) (*models.WorkspaceVariable, error) {
	var variable models.WorkspaceVariable
	if err := s.db.Where("variable_id = ? AND is_deleted = ?", variableID, false).
		Order("version DESC").
		First(&variable).Error; err != nil {
		return nil, fmt.Errorf("变量不存在: %w", err)
	}
	return &variable, nil
}
