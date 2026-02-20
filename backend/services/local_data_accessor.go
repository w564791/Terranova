package services

import (
	"fmt"
	"iac-platform/internal/models"
	"time"

	"gorm.io/gorm"
)

// LocalDataAccessor Local 模式的数据访问实现
// 直接访问数据库
type LocalDataAccessor struct {
	db *gorm.DB
	tx *gorm.DB // 用于事务支持
}

// NewLocalDataAccessor 创建 Local 数据访问器
func NewLocalDataAccessor(db *gorm.DB) *LocalDataAccessor {
	return &LocalDataAccessor{
		db: db,
	}
}

// ============================================================================
// Workspace 相关
// ============================================================================

// GetWorkspace 获取 Workspace
func (a *LocalDataAccessor) GetWorkspace(workspaceID string) (*models.Workspace, error) {
	var workspace models.Workspace
	db := a.getDB()

	if err := db.Where("workspace_id = ?", workspaceID).First(&workspace).Error; err != nil {
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	return &workspace, nil
}

// GetWorkspaceResources 获取 Workspace 资源列表
func (a *LocalDataAccessor) GetWorkspaceResources(workspaceID string) ([]models.WorkspaceResource, error) {
	var resources []models.WorkspaceResource
	db := a.getDB()

	if err := db.Where("workspace_id = ? AND is_active = true", workspaceID).
		Find(&resources).Error; err != nil {
		return nil, fmt.Errorf("failed to get workspace resources: %w", err)
	}

	// 手动加载每个资源的 CurrentVersion
	for i := range resources {
		if resources[i].CurrentVersionID != nil {
			var version models.ResourceCodeVersion
			if err := db.First(&version, *resources[i].CurrentVersionID).Error; err == nil {
				resources[i].CurrentVersion = &version
			}
		}
	}

	return resources, nil
}

// GetWorkspaceVariables 获取 Workspace 变量列表（只返回每个变量的最新版本）
func (a *LocalDataAccessor) GetWorkspaceVariables(workspaceID string, varType models.VariableType) ([]models.WorkspaceVariable, error) {
	var variables []models.WorkspaceVariable
	db := a.getDB()

	// 使用子查询只获取每个变量的最新版本
	err := db.Raw(`
		SELECT wv.*
		FROM workspace_variables wv
		INNER JOIN (
			SELECT variable_id, MAX(version) as max_version
			FROM workspace_variables
			WHERE workspace_id = ? AND variable_type = ? AND is_deleted = false
			GROUP BY variable_id
		) latest ON wv.variable_id = latest.variable_id AND wv.version = latest.max_version
		WHERE wv.workspace_id = ? AND wv.variable_type = ? AND wv.is_deleted = false
	`, workspaceID, varType, workspaceID, varType).Scan(&variables).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get workspace variables: %w", err)
	}

	return variables, nil
}

// ============================================================================
// State 相关
// ============================================================================

// GetLatestStateVersion 获取最新的 State 版本
func (a *LocalDataAccessor) GetLatestStateVersion(workspaceID string) (*models.WorkspaceStateVersion, error) {
	var stateVersion models.WorkspaceStateVersion
	db := a.getDB()

	err := db.Where("workspace_id = ?", workspaceID).
		Order("version DESC").
		First(&stateVersion).Error

	if err == gorm.ErrRecordNotFound {
		return nil, nil // 没有 State 版本，返回 nil 而不是错误
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get latest state version: %w", err)
	}

	return &stateVersion, nil
}

// SaveStateVersion 保存 State 版本
func (a *LocalDataAccessor) SaveStateVersion(version *models.WorkspaceStateVersion) error {
	db := a.getDB()

	if err := db.Create(version).Error; err != nil {
		return fmt.Errorf("failed to save state version: %w", err)
	}

	// 更新 workspace 的 resource_count
	if err := db.Model(&models.Workspace{}).
		Where("workspace_id = ?", version.WorkspaceID).
		Update("resource_count", version.ResourceCount).Error; err != nil {
		// 记录错误但不返回，因为 state 已经保存成功
		fmt.Printf("Warning: failed to update workspace resource_count: %v\n", err)
	}

	return nil
}

// UpdateWorkspaceState 更新 Workspace 的 State
func (a *LocalDataAccessor) UpdateWorkspaceState(workspaceID string, stateContent map[string]interface{}) error {
	db := a.getDB()

	if err := db.Model(&models.Workspace{}).
		Where("workspace_id = ?", workspaceID).
		Update("tf_state", stateContent).Error; err != nil {
		return fmt.Errorf("failed to update workspace state: %w", err)
	}

	return nil
}

// ============================================================================
// Task 相关
// ============================================================================

// GetTask 获取任务
func (a *LocalDataAccessor) GetTask(taskID uint) (*models.WorkspaceTask, error) {
	var task models.WorkspaceTask
	db := a.getDB()

	if err := db.First(&task, taskID).Error; err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	return &task, nil
}

// UpdateTask 更新任务
func (a *LocalDataAccessor) UpdateTask(task *models.WorkspaceTask) error {
	db := a.getDB()

	if err := db.Save(task).Error; err != nil {
		return fmt.Errorf("failed to update task: %w", err)
	}

	return nil
}

// SaveTaskLog 保存任务日志
func (a *LocalDataAccessor) SaveTaskLog(taskID uint, phase, content, level string) error {
	db := a.getDB()

	log := &models.TaskLog{
		TaskID:  taskID,
		Phase:   phase,
		Content: content,
		Level:   level,
	}

	if err := db.Create(log).Error; err != nil {
		return fmt.Errorf("failed to save task log: %w", err)
	}

	return nil
}

// ============================================================================
// Resource 相关
// ============================================================================

// GetResourceVersion 获取资源版本
func (a *LocalDataAccessor) GetResourceVersion(versionID uint) (*models.ResourceCodeVersion, error) {
	var version models.ResourceCodeVersion
	db := a.getDB()

	if err := db.First(&version, versionID).Error; err != nil {
		return nil, fmt.Errorf("failed to get resource version: %w", err)
	}

	return &version, nil
}

// CountActiveResources 统计活跃资源数量
func (a *LocalDataAccessor) CountActiveResources(workspaceID string) (int64, error) {
	var count int64
	db := a.getDB()

	if err := db.Model(&models.WorkspaceResource{}).
		Where("workspace_id = ? AND is_active = true", workspaceID).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count active resources: %w", err)
	}

	return count, nil
}

// GetWorkspaceResourcesWithVersions 获取 Workspace 资源列表（包含版本信息）
func (a *LocalDataAccessor) GetWorkspaceResourcesWithVersions(workspaceID string) ([]models.WorkspaceResource, error) {
	// 复用现有的 GetWorkspaceResources 方法，它已经加载了版本信息
	return a.GetWorkspaceResources(workspaceID)
}

// GetResourceByVersionID 根据版本ID获取资源
func (a *LocalDataAccessor) GetResourceByVersionID(resourceID string, versionID uint) (*models.WorkspaceResource, error) {
	var resource models.WorkspaceResource
	db := a.getDB()

	// 获取资源基本信息（resourceID是string类型，如"aws_s3_bucket.my_bucket"）
	if err := db.Where("resource_id = ?", resourceID).First(&resource).Error; err != nil {
		return nil, fmt.Errorf("failed to get resource: %w", err)
	}

	// 获取指定版本
	var version models.ResourceCodeVersion
	if err := db.First(&version, versionID).Error; err != nil {
		return nil, fmt.Errorf("failed to get resource version: %w", err)
	}

	// 验证版本属于该资源（version.ResourceID是uint类型，关联到resource.ID）
	if version.ResourceID != resource.ID {
		return nil, fmt.Errorf("version %d does not belong to resource %s (resource.ID=%d, version.ResourceID=%d)",
			versionID, resourceID, resource.ID, version.ResourceID)
	}

	// 设置版本信息
	resource.CurrentVersion = &version
	resource.CurrentVersionID = &versionID

	return &resource, nil
}

// CheckResourceVersionExists 检查资源版本是否存在
// resourceID: workspace_resources.resource_id (string, 如 "aws_s3_bucket.my_bucket")
// versionID: resource_code_versions.id (uint, 版本记录的主键ID)
func (a *LocalDataAccessor) CheckResourceVersionExists(resourceID string, versionID uint) (bool, error) {
	db := a.getDB()

	// 直接检查版本记录是否存在
	// 注意：这里不需要先查询resource，因为versionID已经是唯一的
	var version models.ResourceCodeVersion
	if err := db.First(&version, versionID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil // 版本不存在
		}
		return false, fmt.Errorf("failed to check resource version: %w", err)
	}

	// 验证版本对应的资源是否匹配（通过resource_id字符串）
	var resource models.WorkspaceResource
	if err := db.Where("id = ?", version.ResourceID).First(&resource).Error; err != nil {
		return false, fmt.Errorf("failed to get resource for version: %w", err)
	}

	// 检查resource_id是否匹配
	if resource.ResourceID != resourceID {
		return false, nil // 版本存在但不属于指定的资源
	}

	return true, nil
}

// ============================================================================
// Workspace Locking 相关
// ============================================================================

// LockWorkspace 锁定 Workspace
func (a *LocalDataAccessor) LockWorkspace(workspaceID, userID, reason string) error {
	db := a.getDB()

	now := time.Now()
	updates := map[string]interface{}{
		"is_locked":   true,
		"locked_by":   userID,
		"locked_at":   now,
		"lock_reason": reason,
	}

	if err := db.Model(&models.Workspace{}).
		Where("workspace_id = ?", workspaceID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to lock workspace: %w", err)
	}

	return nil
}

// UnlockWorkspace 解锁 Workspace
func (a *LocalDataAccessor) UnlockWorkspace(workspaceID string) error {
	db := a.getDB()

	updates := map[string]interface{}{
		"is_locked":   false,
		"locked_by":   nil,
		"locked_at":   nil,
		"lock_reason": "",
	}

	if err := db.Model(&models.Workspace{}).
		Where("workspace_id = ?", workspaceID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to unlock workspace: %w", err)
	}

	return nil
}

// UpdateWorkspaceFields 更新 Workspace 的指定字段
func (a *LocalDataAccessor) UpdateWorkspaceFields(workspaceID string, updates map[string]interface{}) error {
	db := a.getDB()

	if err := db.Model(&models.Workspace{}).
		Where("workspace_id = ?", workspaceID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to update workspace fields: %w", err)
	}

	return nil
}

// GetTerraformLockHCL 获取 Terraform Lock 文件内容
func (a *LocalDataAccessor) GetTerraformLockHCL(workspaceID string) (string, error) {
	var workspace models.Workspace
	db := a.getDB()

	if err := db.Select("terraform_lock_hcl").
		Where("workspace_id = ?", workspaceID).
		First(&workspace).Error; err != nil {
		return "", fmt.Errorf("failed to get terraform lock hcl: %w", err)
	}

	return workspace.TerraformLockHCL, nil
}

// SaveTerraformLockHCL 保存 Terraform Lock 文件内容
func (a *LocalDataAccessor) SaveTerraformLockHCL(workspaceID string, lockContent string) error {
	db := a.getDB()

	if err := db.Model(&models.Workspace{}).
		Where("workspace_id = ?", workspaceID).
		Update("terraform_lock_hcl", lockContent).Error; err != nil {
		return fmt.Errorf("failed to save terraform lock hcl: %w", err)
	}

	return nil
}

// ============================================================================
// State 相关（扩展）
// ============================================================================

// GetMaxStateVersion 获取最大 State 版本号
func (a *LocalDataAccessor) GetMaxStateVersion(workspaceID string) (int, error) {
	var maxVersion int
	db := a.getDB()

	err := db.Model(&models.WorkspaceStateVersion{}).
		Where("workspace_id = ?", workspaceID).
		Select("COALESCE(MAX(version), 0)").
		Scan(&maxVersion).Error

	if err != nil {
		return 0, fmt.Errorf("failed to get max state version: %w", err)
	}

	return maxVersion, nil
}

// ============================================================================
// Task 相关（扩展）
// ============================================================================

// GetPlanTask 获取 Plan 任务
func (a *LocalDataAccessor) GetPlanTask(taskID uint) (*models.WorkspaceTask, error) {
	// 复用 GetTask 方法
	return a.GetTask(taskID)
}

// GetTaskLogs 获取任务日志列表
func (a *LocalDataAccessor) GetTaskLogs(taskID uint) ([]models.TaskLog, error) {
	var logs []models.TaskLog
	db := a.getDB()

	if err := db.Where("task_id = ?", taskID).
		Order("created_at ASC").
		Find(&logs).Error; err != nil {
		return nil, fmt.Errorf("failed to get task logs: %w", err)
	}

	return logs, nil
}

// ============================================================================
// Plan Parsing 相关
// ============================================================================

// ParsePlanChanges 解析 Plan 变更
func (a *LocalDataAccessor) ParsePlanChanges(taskID uint, planOutput string) error {
	// 在 Local 模式下，这个方法由 PlanParserService 直接调用数据库
	// 这里提供一个占位实现，实际解析逻辑在 plan_parser_service.go 中
	// 如果需要，可以在这里调用 PlanParserService
	return fmt.Errorf("ParsePlanChanges should be called through PlanParserService in Local mode")
}

// ============================================================================
// Transaction 支持
// ============================================================================

// BeginTransaction 开始事务
func (a *LocalDataAccessor) BeginTransaction() (DataAccessor, error) {
	tx := a.db.Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}

	return &LocalDataAccessor{
		db: a.db,
		tx: tx,
	}, nil
}

// Commit 提交事务
func (a *LocalDataAccessor) Commit() error {
	if a.tx == nil {
		return fmt.Errorf("no transaction to commit")
	}

	if err := a.tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Rollback 回滚事务
func (a *LocalDataAccessor) Rollback() error {
	if a.tx == nil {
		return fmt.Errorf("no transaction to rollback")
	}

	if err := a.tx.Rollback().Error; err != nil {
		return fmt.Errorf("failed to rollback transaction: %w", err)
	}

	return nil
}

// ============================================================================
// 辅助方法
// ============================================================================

// getDB 获取当前使用的数据库连接（事务或普通连接）
func (a *LocalDataAccessor) getDB() *gorm.DB {
	if a.tx != nil {
		return a.tx
	}
	return a.db
}

// UpdateResourceStatus 更新资源状态
func (a *LocalDataAccessor) UpdateResourceStatus(taskID uint, resourceAddress, status, action string) error {
	db := a.getDB()

	// 查找资源记录
	var resource models.WorkspaceTaskResourceChange
	err := db.Where("task_id = ? AND resource_address = ?", taskID, resourceAddress).
		First(&resource).Error

	if err != nil {
		return fmt.Errorf("resource not found for address %s: %w", resourceAddress, err)
	}

	// 更新状态
	now := time.Now()
	updates := map[string]interface{}{
		"apply_status": status,
		"updated_at":   now,
	}

	if status == "applying" && resource.ApplyStartedAt == nil {
		updates["apply_started_at"] = now
	}

	if status == "completed" {
		updates["apply_completed_at"] = now
	}

	if err := db.Model(&resource).Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to update resource status: %w", err)
	}

	return nil
}

// GetResourceByVersion 根据版本号获取资源
// resourceID: workspace_resources.resource_id (string, 如 "aws_s3_bucket.my_bucket")
// version: resource_code_versions.version (int, 版本号如 1, 2, 3...)
func (a *LocalDataAccessor) GetResourceByVersion(resourceID string, version int) (*models.WorkspaceResource, error) {
	// 【注意】这个方法的设计有问题：resourceID 可能在多个workspace中重复
	// 应该使用 resource_db_id (workspace_resources.id) 来精确查询
	// 但目前接口签名无法修改，所以这里不应该被调用
	// 实际应该使用 GetResourceByDBID 方法
	return nil, fmt.Errorf("GetResourceByVersion should not be used - use snapshot's resource_db_id instead")
}

// CheckResourceVersionExistsByVersion 检查资源版本是否存在（按版本号）
// resourceID: workspace_resources.resource_id (string, 如 "aws_s3_bucket.my_bucket")
// version: resource_code_versions.version (int, 版本号如 1, 2, 3...)
func (a *LocalDataAccessor) CheckResourceVersionExistsByVersion(resourceID string, version int) (bool, error) {
	// 【注意】这个方法的设计有问题：resourceID 可能在多个workspace中重复
	// 应该使用 resource_db_id (workspace_resources.id) 来精确查询
	// 但目前接口签名无法修改，所以这里不应该被调用
	// 实际应该使用 CheckResourceVersionExistsByDBID 方法
	return false, fmt.Errorf("CheckResourceVersionExistsByVersion should not be used - use snapshot's resource_db_id instead")
}
