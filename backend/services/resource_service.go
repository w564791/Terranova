package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// ResourceService 资源管理服务
type ResourceService struct {
	db       *gorm.DB
	executor *TerraformExecutor
}

// GetDB 获取数据库连接
func (s *ResourceService) GetDB() *gorm.DB {
	return s.db
}

// NewResourceService 创建资源服务
func NewResourceService(db *gorm.DB, streamManager *OutputStreamManager) *ResourceService {
	return &ResourceService{
		db:       db,
		executor: NewTerraformExecutor(db, streamManager),
	}
}

// ============================================================================
// 资源CRUD
// ============================================================================

// AddResource 添加新资源到Workspace
func (s *ResourceService) AddResource(
	workspaceID string,
	resourceType string,
	resourceName string,
	tfCode map[string]interface{},
	variables map[string]interface{},
	description string,
	userID string,
) (*models.WorkspaceResource, error) {
	resourceID := fmt.Sprintf("%s.%s", resourceType, resourceName)

	// 检查资源是否已存在
	var existing models.WorkspaceResource
	err := s.db.Where("workspace_id = ? AND resource_id = ?", workspaceID, resourceID).
		First(&existing).Error

	if err == nil {
		return nil, fmt.Errorf("resource %s already exists", resourceID)
	}

	if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	// 创建资源记录
	resource := &models.WorkspaceResource{
		WorkspaceID:  workspaceID,
		ResourceID:   resourceID,
		ResourceType: resourceType,
		ResourceName: resourceName,
		IsActive:     true,
		Description:  description,
		CreatedBy:    &userID,
	}

	return resource, s.db.Transaction(func(tx *gorm.DB) error {
		// 1. 创建资源
		if err := tx.Create(resource).Error; err != nil {
			return err
		}

		// 2. 创建第一个版本
		version := &models.ResourceCodeVersion{
			ResourceID:    resource.ID,
			Version:       1,
			IsLatest:      true,
			TFCode:        tfCode,
			Variables:     variables,
			ChangeType:    "create",
			ChangeSummary: "Initial creation",
			CreatedBy:     &userID,
		}

		if err := tx.Create(version).Error; err != nil {
			return err
		}

		// 3. 更新资源的当前版本
		resource.CurrentVersionID = &version.ID
		return tx.Save(resource).Error
	})
}

// GetResources 获取Workspace的所有资源
func (s *ResourceService) GetResources(workspaceID string, includeInactive bool) ([]models.WorkspaceResource, error) {
	var resources []models.WorkspaceResource
	query := s.db.Where("workspace_id = ?", workspaceID).
		Preload("CurrentVersion").
		Preload("Creator")

	if !includeInactive {
		query = query.Where("is_active = true")
	}

	err := query.Order("created_at DESC").Find(&resources).Error
	return resources, err
}

// ResourceListItem 资源列表项（精简字段）
type ResourceListItem struct {
	ID             uint                    `json:"id"`
	WorkspaceID    string                  `json:"workspace_id"`
	ResourceType   string                  `json:"resource_type"`
	ResourceName   string                  `json:"resource_name"`
	ResourceID     string                  `json:"resource_id"`
	IsActive       bool                    `json:"is_active"`
	CreatedAt      string                  `json:"created_at"`
	UpdatedAt      string                  `json:"updated_at"`
	CurrentVersion *ResourceVersionSummary `json:"current_version,omitempty"`
}

// ResourceVersionSummary 版本摘要信息
type ResourceVersionSummary struct {
	Version       int    `json:"version"`
	IsLatest      bool   `json:"is_latest"`
	ChangeSummary string `json:"change_summary"`
}

// GetResourcesPaginated 获取资源列表（分页）
func (s *ResourceService) GetResourcesPaginated(
	workspaceID string,
	page, pageSize int,
	search, sortBy, sortOrder string,
	includeInactive bool,
) (map[string]interface{}, error) {
	// 构建查询
	query := s.db.Model(&models.WorkspaceResource{}).
		Where("workspace_id = ?", workspaceID)

	// 过滤已删除资源
	if !includeInactive {
		query = query.Where("is_active = ?", true)
	}

	// 搜索过滤
	if search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where(
			"resource_name LIKE ? OR resource_type LIKE ? OR description LIKE ?",
			searchPattern, searchPattern, searchPattern,
		)
	}

	// 计算总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	// 验证排序字段
	validSortFields := map[string]bool{
		"created_at":    true,
		"updated_at":    true,
		"resource_name": true,
		"resource_type": true,
	}
	if !validSortFields[sortBy] {
		sortBy = "created_at"
	}

	// 验证排序方向
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}

	// 排序
	orderClause := sortBy + " " + sortOrder
	query = query.Order(orderClause)

	// 分页
	offset := (page - 1) * pageSize
	query = query.Offset(offset).Limit(pageSize)

	// 只查询必要字段（不包括tf_code和variables）
	var resources []models.WorkspaceResource
	err := query.Select(
		"id", "workspace_id", "resource_type", "resource_name",
		"resource_id", "is_active", "created_at", "updated_at",
		"current_version_id",
	).Preload("CurrentVersion", func(db *gorm.DB) *gorm.DB {
		return db.Select("id", "resource_id", "version", "is_latest", "change_summary")
	}).Find(&resources).Error

	if err != nil {
		return nil, err
	}

	// 转换为列表项格式
	items := make([]ResourceListItem, len(resources))
	for i, r := range resources {
		items[i] = ResourceListItem{
			ID:           r.ID,
			WorkspaceID:  r.WorkspaceID,
			ResourceType: r.ResourceType,
			ResourceName: r.ResourceName,
			ResourceID:   r.ResourceID,
			IsActive:     r.IsActive,
			CreatedAt:    r.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:    r.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}

		if r.CurrentVersion != nil {
			items[i].CurrentVersion = &ResourceVersionSummary{
				Version:       r.CurrentVersion.Version,
				IsLatest:      r.CurrentVersion.IsLatest,
				ChangeSummary: r.CurrentVersion.ChangeSummary,
			}
		}
	}

	// 计算总页数
	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))

	return map[string]interface{}{
		"resources": items,
		"pagination": map[string]interface{}{
			"page":        page,
			"page_size":   pageSize,
			"total":       total,
			"total_pages": totalPages,
		},
	}, nil
}

// GetResource 获取单个资源
func (s *ResourceService) GetResource(resourceID uint) (*models.WorkspaceResource, error) {
	var resource models.WorkspaceResource
	err := s.db.Preload("CurrentVersion").
		Preload("Creator").
		First(&resource, resourceID).Error

	return &resource, err
}

// UpdateResource 更新资源配置
func (s *ResourceService) UpdateResource(
	resourceID uint,
	tfCode map[string]interface{},
	variables map[string]interface{},
	changeSummary string,
	userID string,
) (*models.ResourceCodeVersion, error) {
	// 获取资源
	var resource models.WorkspaceResource
	if err := s.db.First(&resource, resourceID).Error; err != nil {
		return nil, err
	}

	// 获取当前最新版本号
	var maxVersion int
	s.db.Model(&models.ResourceCodeVersion{}).
		Where("resource_id = ?", resourceID).
		Select("COALESCE(MAX(version), 0)").
		Scan(&maxVersion)

	// 获取当前版本用于计算差异
	var currentVersion models.ResourceCodeVersion
	s.db.Where("resource_id = ? AND is_latest = true", resourceID).
		First(&currentVersion)

	diff := s.calculateDiff(currentVersion.TFCode, tfCode)

	// 创建新版本
	newVersion := &models.ResourceCodeVersion{
		ResourceID:       resourceID,
		Version:          maxVersion + 1,
		IsLatest:         true,
		TFCode:           tfCode,
		Variables:        variables,
		ChangeType:       "update",
		ChangeSummary:    changeSummary,
		DiffFromPrevious: diff,
		CreatedBy:        &userID,
	}

	return newVersion, s.db.Transaction(func(tx *gorm.DB) error {
		// 旧版本标记为非最新
		tx.Model(&models.ResourceCodeVersion{}).
			Where("resource_id = ? AND is_latest = true", resourceID).
			Update("is_latest", false)

		// 创建新版本
		if err := tx.Create(newVersion).Error; err != nil {
			return err
		}

		// 更新资源的当前版本
		resource.CurrentVersionID = &newVersion.ID
		return tx.Save(&resource).Error
	})
}

// DeleteResource 删除资源（软删除）
// 同时删除关联的 outputs，避免 Terraform 报 "Reference to undeclared module" 错误
func (s *ResourceService) DeleteResource(resourceID uint, userID string) error {
	return s.DeleteResourceWithOptions(resourceID, userID, false)
}

// HardDeleteResource 硬删除资源（永久删除）
func (s *ResourceService) HardDeleteResource(resourceID uint, userID string) error {
	return s.DeleteResourceWithOptions(resourceID, userID, true)
}

// DeleteResourceWithOptions 删除资源（支持软删除和硬删除）
// hardDelete: true 表示永久删除，false 表示软删除
func (s *ResourceService) DeleteResourceWithOptions(resourceID uint, userID string, hardDelete bool) error {
	// 先获取资源信息，用于删除关联的 outputs
	var resource models.WorkspaceResource
	if err := s.db.First(&resource, resourceID).Error; err != nil {
		return fmt.Errorf("resource not found: %w", err)
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		// 1. 删除关联的 outputs（通过 resource_name 关联）
		if err := tx.Where("workspace_id = ? AND resource_name = ?",
			resource.WorkspaceID, resource.ResourceName).
			Delete(&models.WorkspaceOutput{}).Error; err != nil {
			return fmt.Errorf("failed to delete associated outputs: %w", err)
		}

		if hardDelete {
			// 硬删除：永久删除资源及其所有版本
			// 2a. 删除资源的所有版本
			if err := tx.Where("resource_id = ?", resourceID).
				Delete(&models.ResourceCodeVersion{}).Error; err != nil {
				return fmt.Errorf("failed to delete resource versions: %w", err)
			}

			// 2b. 删除资源依赖关系
			if err := tx.Where("resource_id = ? OR depends_on_resource_id = ?", resourceID, resourceID).
				Delete(&models.ResourceDependency{}).Error; err != nil {
				return fmt.Errorf("failed to delete resource dependencies: %w", err)
			}

			// 2c. 永久删除资源记录
			if err := tx.Delete(&models.WorkspaceResource{}, resourceID).Error; err != nil {
				return fmt.Errorf("failed to hard delete resource: %w", err)
			}
		} else {
			// 软删除：仅标记为非活跃
			if err := tx.Model(&models.WorkspaceResource{}).
				Where("id = ?", resourceID).
				Updates(map[string]interface{}{
					"is_active":  false,
					"updated_at": "NOW()",
				}).Error; err != nil {
				return fmt.Errorf("failed to soft delete resource: %w", err)
			}
		}

		return nil
	})
}

// RestoreResource 恢复已删除的资源
func (s *ResourceService) RestoreResource(resourceID uint) error {
	return s.db.Model(&models.WorkspaceResource{}).
		Where("id = ?", resourceID).
		Updates(map[string]interface{}{
			"is_active":  true,
			"updated_at": "NOW()",
		}).Error
}

// ============================================================================
// 版本管理
// ============================================================================

// GetResourceVersions 获取资源的所有版本
func (s *ResourceService) GetResourceVersions(resourceID uint) ([]models.ResourceCodeVersion, error) {
	var versions []models.ResourceCodeVersion
	err := s.db.Where("resource_id = ?", resourceID).
		Preload("Creator").
		Order("version DESC").
		Find(&versions).Error

	return versions, err
}

// GetResourceVersion 获取资源的特定版本
func (s *ResourceService) GetResourceVersion(resourceID uint, version int) (*models.ResourceCodeVersion, error) {
	var ver models.ResourceCodeVersion
	err := s.db.Where("resource_id = ? AND version = ?", resourceID, version).
		Preload("Creator").
		First(&ver).Error

	return &ver, err
}

// RollbackResource 回滚资源到指定版本
func (s *ResourceService) RollbackResource(
	resourceID uint,
	targetVersion int,
	userID string,
) (*models.ResourceCodeVersion, error) {
	// 获取目标版本
	var targetVer models.ResourceCodeVersion
	if err := s.db.Where("resource_id = ? AND version = ?",
		resourceID, targetVersion).First(&targetVer).Error; err != nil {
		return nil, fmt.Errorf("target version not found: %w", err)
	}

	// 获取资源信息
	var resource models.WorkspaceResource
	if err := s.db.First(&resource, resourceID).Error; err != nil {
		return nil, err
	}

	// 获取当前最大版本号
	var maxVersion int
	s.db.Model(&models.ResourceCodeVersion{}).
		Where("resource_id = ?", resourceID).
		Select("COALESCE(MAX(version), 0)").
		Scan(&maxVersion)

	// 创建新版本（内容是旧版本的）
	newVersion := &models.ResourceCodeVersion{
		ResourceID:    resourceID,
		Version:       maxVersion + 1,
		IsLatest:      true,
		TFCode:        targetVer.TFCode,
		Variables:     targetVer.Variables,
		ChangeType:    "rollback",
		ChangeSummary: fmt.Sprintf("Rollback to version %d", targetVersion),
		CreatedBy:     &userID,
	}

	return newVersion, s.db.Transaction(func(tx *gorm.DB) error {
		// 旧版本标记为非最新
		tx.Model(&models.ResourceCodeVersion{}).
			Where("resource_id = ? AND is_latest = true", resourceID).
			Update("is_latest", false)

		// 创建新版本
		if err := tx.Create(newVersion).Error; err != nil {
			return err
		}

		// 更新资源的当前版本
		resource.CurrentVersionID = &newVersion.ID
		return tx.Save(&resource).Error
	})
}

// CompareVersions 对比两个版本
func (s *ResourceService) CompareVersions(
	resourceID uint,
	fromVersion int,
	toVersion int,
) (map[string]interface{}, error) {
	var from, to models.ResourceCodeVersion

	if err := s.db.Where("resource_id = ? AND version = ?", resourceID, fromVersion).
		First(&from).Error; err != nil {
		return nil, fmt.Errorf("from version not found: %w", err)
	}

	if err := s.db.Where("resource_id = ? AND version = ?", resourceID, toVersion).
		First(&to).Error; err != nil {
		return nil, fmt.Errorf("to version not found: %w", err)
	}

	diff := s.calculateDiff(from.TFCode, to.TFCode)

	return map[string]interface{}{
		"from_version": fromVersion,
		"to_version":   toVersion,
		"diff":         diff,
		"from_code":    from.TFCode,
		"to_code":      to.TFCode,
	}, nil
}

// ============================================================================
// 快照管理
// ============================================================================

// CreateSnapshot 创建资源快照
func (s *ResourceService) CreateSnapshot(
	workspaceID string,
	snapshotName string,
	description string,
	userID string,
) (*models.WorkspaceResourcesSnapshot, error) {
	// 获取所有激活资源的当前版本
	var resources []models.WorkspaceResource
	s.db.Where("workspace_id = ? AND is_active = true", workspaceID).
		Find(&resources)

	// 构建版本映射
	versionsMap := make(map[string]interface{})
	for _, res := range resources {
		if res.CurrentVersionID != nil {
			versionsMap[fmt.Sprintf("%d", res.ID)] = *res.CurrentVersionID
		}
	}

	// 创建快照
	snapshot := &models.WorkspaceResourcesSnapshot{
		WorkspaceID:       workspaceID,
		SnapshotName:      snapshotName,
		ResourcesVersions: versionsMap,
		Description:       description,
		CreatedBy:         &userID,
	}

	return snapshot, s.db.Create(snapshot).Error
}

// GetSnapshots 获取快照列表
func (s *ResourceService) GetSnapshots(workspaceID string) ([]models.WorkspaceResourcesSnapshot, error) {
	var snapshots []models.WorkspaceResourcesSnapshot
	err := s.db.Where("workspace_id = ?", workspaceID).
		Preload("Creator").
		Order("created_at DESC").
		Find(&snapshots).Error

	return snapshots, err
}

// GetSnapshot 获取快照详情
func (s *ResourceService) GetSnapshot(snapshotID uint) (*models.WorkspaceResourcesSnapshot, error) {
	var snapshot models.WorkspaceResourcesSnapshot
	err := s.db.Preload("Creator").
		First(&snapshot, snapshotID).Error

	return &snapshot, err
}

// RestoreSnapshot 恢复快照
func (s *ResourceService) RestoreSnapshot(
	snapshotID uint,
	userID string,
) error {
	// 获取快照
	var snapshot models.WorkspaceResourcesSnapshot
	if err := s.db.First(&snapshot, snapshotID).Error; err != nil {
		return err
	}

	// 恢复每个资源到快照中的版本
	for resourceIDStr, versionIDInterface := range snapshot.ResourcesVersions {
		resourceID, _ := strconv.ParseUint(resourceIDStr, 10, 32)

		// 获取版本ID
		var versionID uint
		switch v := versionIDInterface.(type) {
		case float64:
			versionID = uint(v)
		case int:
			versionID = uint(v)
		case uint:
			versionID = v
		default:
			continue
		}

		// 获取版本信息
		var version models.ResourceCodeVersion
		if err := s.db.First(&version, versionID).Error; err != nil {
			continue
		}

		// 回滚资源
		s.RollbackResource(uint(resourceID), version.Version, userID)
	}

	return nil
}

// DeleteSnapshot 删除快照
func (s *ResourceService) DeleteSnapshot(snapshotID uint) error {
	return s.db.Delete(&models.WorkspaceResourcesSnapshot{}, snapshotID).Error
}

// ============================================================================
// 依赖关系管理
// ============================================================================

// AddDependency 添加资源依赖关系
func (s *ResourceService) AddDependency(
	workspaceID string,
	resourceID uint,
	dependsOnResourceID uint,
	dependencyType string,
) error {
	dep := &models.ResourceDependency{
		WorkspaceID:         workspaceID,
		ResourceID:          resourceID,
		DependsOnResourceID: dependsOnResourceID,
		DependencyType:      dependencyType,
	}

	return s.db.Create(dep).Error
}

// GetResourceDependencies 获取资源的依赖关系
func (s *ResourceService) GetResourceDependencies(resourceID uint) (map[string]interface{}, error) {
	// 获取此资源依赖的资源
	var dependsOn []models.ResourceDependency
	s.db.Where("resource_id = ?", resourceID).
		Preload("DependsOnResource").
		Find(&dependsOn)

	// 获取依赖此资源的资源
	var dependedBy []models.ResourceDependency
	s.db.Where("depends_on_resource_id = ?", resourceID).
		Preload("Resource").
		Find(&dependedBy)

	return map[string]interface{}{
		"depends_on":  dependsOn,
		"depended_by": dependedBy,
	}, nil
}

// GetResourceWithDependencies 获取资源及其所有依赖（递归）
func (s *ResourceService) GetResourceWithDependencies(resourceID uint) ([]string, error) {
	visited := make(map[uint]bool)
	targets := []string{}

	var collectDeps func(uint) error
	collectDeps = func(rid uint) error {
		if visited[rid] {
			return nil
		}
		visited[rid] = true

		// 获取资源
		var resource models.WorkspaceResource
		if err := s.db.First(&resource, rid).Error; err != nil {
			return err
		}

		// 添加到targets
		targets = append(targets, resource.ResourceID)

		// 递归获取依赖
		var deps []models.ResourceDependency
		s.db.Where("resource_id = ?", rid).Find(&deps)

		for _, dep := range deps {
			if err := collectDeps(dep.DependsOnResourceID); err != nil {
				return err
			}
		}

		return nil
	}

	if err := collectDeps(resourceID); err != nil {
		return nil, err
	}

	return targets, nil
}

// UpdateDependencies 更新资源的依赖关系
func (s *ResourceService) UpdateDependencies(
	workspaceID string,
	resourceID uint,
	dependsOnResourceIDs []uint,
) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// 删除现有依赖
		tx.Where("resource_id = ?", resourceID).
			Delete(&models.ResourceDependency{})

		// 添加新依赖
		for _, depID := range dependsOnResourceIDs {
			dep := &models.ResourceDependency{
				WorkspaceID:         workspaceID,
				ResourceID:          resourceID,
				DependsOnResourceID: depID,
				DependencyType:      "explicit",
			}
			if err := tx.Create(dep).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

// ============================================================================
// 辅助函数
// ============================================================================

// calculateDiff 计算两个版本的差异
func (s *ResourceService) calculateDiff(oldCode, newCode map[string]interface{}) string {
	oldJSON, _ := json.MarshalIndent(oldCode, "", "  ")
	newJSON, _ := json.MarshalIndent(newCode, "", "  ")

	// 简单的文本差异（后续可以使用更复杂的diff算法）
	return fmt.Sprintf("Old:\n%s\n\nNew:\n%s", string(oldJSON), string(newJSON))
}

// GenerateMainTFFromResources 从资源聚合生成main.tf.json
func (s *ResourceService) GenerateMainTFFromResources(workspaceID string) (map[string]interface{}, error) {
	// 获取所有激活的资源
	var resources []models.WorkspaceResource
	s.db.Where("workspace_id = ? AND is_active = true", workspaceID).
		Preload("CurrentVersion").
		Find(&resources)

	// 聚合所有资源的TF代码
	mainTF := make(map[string]interface{})

	for _, resource := range resources {
		if resource.CurrentVersion == nil {
			continue
		}

		// 合并资源的TF代码到main.tf
		s.mergeTFCode(mainTF, resource.CurrentVersion.TFCode)
	}

	return mainTF, nil
}

// mergeTFCode 合并TF代码
func (s *ResourceService) mergeTFCode(target, source map[string]interface{}) {
	for key, value := range source {
		if existing, ok := target[key]; ok {
			// 如果key已存在，合并内容
			if existingMap, ok := existing.(map[string]interface{}); ok {
				if sourceMap, ok := value.(map[string]interface{}); ok {
					for k, v := range sourceMap {
						existingMap[k] = v
					}
					continue
				}
			}
		}
		target[key] = value
	}
}

// ImportResourcesFromTF 从Terraform配置导入资源
func (s *ResourceService) ImportResourcesFromTF(
	workspaceID string,
	tfCode map[string]interface{},
	userID string,
) (int, error) {
	count := 0

	// 解析resource块
	if resources, ok := tfCode["resource"].(map[string]interface{}); ok {
		for resourceType, resourcesOfType := range resources {
			if resourceMap, ok := resourcesOfType.(map[string]interface{}); ok {
				for resourceName, resourceConfig := range resourceMap {
					// 为每个资源创建记录
					resourceTFCode := map[string]interface{}{
						"resource": map[string]interface{}{
							resourceType: map[string]interface{}{
								resourceName: resourceConfig,
							},
						},
					}

					_, err := s.AddResource(
						workspaceID,
						resourceType,
						resourceName,
						resourceTFCode,
						nil,
						fmt.Sprintf("Imported from TF code"),
						userID,
					)

					if err == nil {
						count++
					}
				}
			}
		}
	}

	return count, nil
}

// GetResourcesByIDs 根据ID列表获取资源
func (s *ResourceService) GetResourcesByIDs(resourceIDs []uint, resources *[]models.WorkspaceResource) error {
	return s.db.Where("id IN ? AND is_active = true", resourceIDs).
		Find(resources).Error
}

// CreatePlanTaskWithTargets 创建带target的Plan任务
func (s *ResourceService) CreatePlanTaskWithTargets(
	workspaceID string,
	targets []string,
	userID string,
) (*models.WorkspaceTask, error) {
	// 获取workspace
	var workspace models.Workspace
	if err := s.db.Where("workspace_id = ?", workspaceID).First(&workspace).Error; err != nil {
		return nil, fmt.Errorf("workspace not found: %w", err)
	}

	// 检查workspace是否被锁定
	if workspace.IsLocked {
		return nil, fmt.Errorf("workspace is locked")
	}

	// 创建Plan任务
	task := &models.WorkspaceTask{
		WorkspaceID:   workspace.WorkspaceID, // 使用语义化ID
		TaskType:      models.TaskTypePlan,
		Status:        models.TaskStatusPending,
		ExecutionMode: workspace.ExecutionMode,
		CreatedBy:     &userID,
		Stage:         "pending",
		Context: map[string]interface{}{
			"targets": targets,
		},
	}

	if err := s.db.Create(task).Error; err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	// 异步执行Plan任务
	go s.executor.ExecutePlan(context.Background(), task)

	return task, nil
}

// ============================================================================
// HCL导出功能
// ============================================================================

// ExportResourcesAsHCL 导出工作空间资源为HCL格式
func (s *ResourceService) ExportResourcesAsHCL(workspaceID string) (string, error) {
	// 获取所有激活的资源
	var resources []models.WorkspaceResource
	err := s.db.Where("workspace_id = ? AND is_active = true", workspaceID).
		Preload("CurrentVersion").
		Order("resource_type, resource_name").
		Find(&resources).Error

	if err != nil {
		return "", fmt.Errorf("failed to fetch resources: %w", err)
	}

	if len(resources) == 0 {
		return "# No resources found in this workspace\n", nil
	}

	// 预加载所有 modules 用于查找 version（复用 TerraformExecutor 的逻辑）
	moduleVersions := make(map[string]string) // key: provider_name, value: version
	var modules []models.Module
	if err := s.db.Select("name, provider, version").Find(&modules).Error; err == nil {
		for _, m := range modules {
			key := fmt.Sprintf("%s_%s", m.Provider, m.Name)
			if m.Version != "" {
				moduleVersions[key] = m.Version
			}
		}
	}

	var hclBuilder strings.Builder

	// 获取 workspace 的 provider_config
	var workspace models.Workspace
	if err := s.db.Where("workspace_id = ?", workspaceID).First(&workspace).Error; err != nil {
		return "", fmt.Errorf("failed to fetch workspace: %w", err)
	}

	// 添加文件头注释
	hclBuilder.WriteString("# Terraform Resources Export\n")
	hclBuilder.WriteString(fmt.Sprintf("# Workspace: %s\n", workspaceID))
	hclBuilder.WriteString(fmt.Sprintf("# Total Resources: %d\n", len(resources)))
	hclBuilder.WriteString("# Generated by IAC Platform\n\n")

	// 导出 terraform 块（包含 required_providers）
	if workspace.ProviderConfig != nil {
		if terraformBlock, ok := workspace.ProviderConfig["terraform"]; ok {
			hclBuilder.WriteString("# ============================================================================\n")
			hclBuilder.WriteString("# Terraform Configuration\n")
			hclBuilder.WriteString("# ============================================================================\n\n")
			hcl := s.convertTerraformBlockToHCL(terraformBlock)
			hclBuilder.WriteString(hcl)
			hclBuilder.WriteString("\n")
		}

		// 导出 provider 块
		if providerBlock, ok := workspace.ProviderConfig["provider"]; ok {
			hclBuilder.WriteString("# ============================================================================\n")
			hclBuilder.WriteString("# Provider Configuration\n")
			hclBuilder.WriteString("# ============================================================================\n\n")
			hcl := s.convertProviderBlockToHCL(providerBlock)
			hclBuilder.WriteString(hcl)
			hclBuilder.WriteString("\n")
		}
	}

	// 资源分隔
	if len(resources) > 0 {
		hclBuilder.WriteString("# ============================================================================\n")
		hclBuilder.WriteString("# Resources\n")
		hclBuilder.WriteString("# ============================================================================\n\n")
	}

	// 遍历每个资源并转换为HCL
	for _, resource := range resources {
		if resource.CurrentVersion == nil || resource.CurrentVersion.TFCode == nil {
			continue
		}

		// 添加资源注释
		hclBuilder.WriteString(fmt.Sprintf("# Resource: %s\n", resource.ResourceID))
		if resource.Description != "" {
			hclBuilder.WriteString(fmt.Sprintf("# Description: %s\n", resource.Description))
		}

		// 复制 tf_code 以避免修改原始数据
		tfCode := s.copyTFCode(resource.CurrentVersion.TFCode)

		// 注入 version 字段（复用 TerraformExecutor 的逻辑）
		s.ensureModuleVersion(tfCode, resource.ResourceType, moduleVersions)

		// 检查是否有module块
		if moduleBlock, ok := tfCode["module"].(map[string]interface{}); ok {
			hcl := s.convertModuleBlockToHCL(moduleBlock, 0)
			hclBuilder.WriteString(hcl)
		} else if resourceBlock, ok := tfCode["resource"].(map[string]interface{}); ok {
			// 检查是否有resource块
			hcl := s.convertResourceBlockToHCL(resourceBlock, 0)
			hclBuilder.WriteString(hcl)
		} else {
			// 如果没有特定块，尝试直接转换整个tf_code
			hcl := s.convertTFCodeToHCL(tfCode, 0)
			hclBuilder.WriteString(hcl)
		}

		hclBuilder.WriteString("\n")
	}

	// 导出 workspace outputs（复用 exec 流程的 key 格式）
	var outputs []models.WorkspaceOutput
	if err := s.db.Where("workspace_id = ?", workspaceID).Find(&outputs).Error; err == nil && len(outputs) > 0 {
		hclBuilder.WriteString("# ============================================================================\n")
		hclBuilder.WriteString("# Outputs\n")
		hclBuilder.WriteString("# ============================================================================\n\n")

		for _, output := range outputs {
			var outputKey string

			// 判断是否为静态输出（复用 exec 流程的 key 格式）
			if output.IsStaticOutput() {
				// 静态输出的 key 格式: static-{output_name}
				outputKey = fmt.Sprintf("static-%s", output.OutputName)
				hclBuilder.WriteString(fmt.Sprintf("output \"%s\" {\n", outputKey))
				// 静态输出：直接使用值
				hclBuilder.WriteString(fmt.Sprintf("  value = \"%s\"\n", s.escapeHCLString(output.OutputValue)))
			} else {
				// 资源关联输出：从 output_value 中提取 module 名称
				// output_value 格式: module.{module_name}.{output_name}
				moduleName := output.ResourceName // 默认使用 resource_name
				if strings.HasPrefix(output.OutputValue, "module.") {
					parts := strings.Split(output.OutputValue, ".")
					if len(parts) >= 2 {
						moduleName = parts[1] // 提取 module 名称
					}
				}
				// 使用 module_name-output_name 作为 key
				outputKey = fmt.Sprintf("%s-%s", moduleName, output.OutputName)
				hclBuilder.WriteString(fmt.Sprintf("output \"%s\" {\n", outputKey))
				// 资源关联输出：使用表达式引用
				hclBuilder.WriteString(fmt.Sprintf("  value = %s\n", output.OutputValue))
			}

			if output.Description != "" {
				hclBuilder.WriteString(fmt.Sprintf("  description = \"%s\"\n", s.escapeHCLString(output.Description)))
			}

			if output.Sensitive {
				hclBuilder.WriteString("  sensitive = true\n")
			}

			hclBuilder.WriteString("}\n\n")
		}
	}

	return hclBuilder.String(), nil
}

// copyTFCode 深拷贝 tf_code（复用 TerraformExecutor 的逻辑）
func (s *ResourceService) copyTFCode(source map[string]interface{}) map[string]interface{} {
	if source == nil {
		return nil
	}
	result := make(map[string]interface{})
	for k, v := range source {
		switch val := v.(type) {
		case map[string]interface{}:
			result[k] = s.copyTFCode(val)
		case []interface{}:
			newSlice := make([]interface{}, len(val))
			for i, item := range val {
				if itemMap, ok := item.(map[string]interface{}); ok {
					newSlice[i] = s.copyTFCode(itemMap)
				} else {
					newSlice[i] = item
				}
			}
			result[k] = newSlice
		default:
			result[k] = v
		}
	}
	return result
}

// ensureModuleVersion 确保 module 配置中包含 version 字段（复用 TerraformExecutor 的逻辑）
func (s *ResourceService) ensureModuleVersion(tfCode map[string]interface{}, resourceType string, moduleVersions map[string]string) {
	// 获取 module 块
	moduleBlock, ok := tfCode["module"].(map[string]interface{})
	if !ok {
		return
	}

	// 遍历所有 module 定义
	for moduleName, moduleConfig := range moduleBlock {
		// module 配置通常是一个数组
		configArray, ok := moduleConfig.([]interface{})
		if !ok || len(configArray) == 0 {
			continue
		}

		// 获取第一个配置对象
		config, ok := configArray[0].(map[string]interface{})
		if !ok {
			continue
		}

		// 检查是否已有 version 字段
		if _, hasVersion := config["version"]; hasVersion {
			continue
		}

		// 检查是否有 source 字段
		source, hasSource := config["source"].(string)
		if !hasSource || source == "" {
			continue
		}

		// 尝试从 moduleVersions 中查找 version（使用 resourceType 作为 key）
		if version, found := moduleVersions[resourceType]; found && version != "" {
			config["version"] = version
		}

		// 更新 moduleBlock
		moduleBlock[moduleName] = configArray
	}
}

// convertModuleBlockToHCL 将module块转换为HCL格式
func (s *ResourceService) convertModuleBlockToHCL(moduleBlock map[string]interface{}, indent int) string {
	var builder strings.Builder

	// 获取所有模块名称并排序
	moduleNames := make([]string, 0, len(moduleBlock))
	for moduleName := range moduleBlock {
		moduleNames = append(moduleNames, moduleName)
	}
	sort.Strings(moduleNames)

	for _, moduleName := range moduleNames {
		moduleConfig := moduleBlock[moduleName]

		// 处理模块配置 - 可能是数组或对象
		var configMap map[string]interface{}

		switch v := moduleConfig.(type) {
		case []interface{}:
			// 如果是数组，取第一个元素
			if len(v) > 0 {
				if m, ok := v[0].(map[string]interface{}); ok {
					configMap = m
				}
			}
		case map[string]interface{}:
			configMap = v
		}

		if configMap == nil {
			continue
		}

		builder.WriteString(fmt.Sprintf("module \"%s\" {\n", moduleName))
		builder.WriteString(s.convertMapToHCLBody(configMap, 1))
		builder.WriteString("}\n\n")
	}

	return builder.String()
}

// convertTFCodeToHCL 将整个tf_code转换为HCL格式
func (s *ResourceService) convertTFCodeToHCL(tfCode map[string]interface{}, indent int) string {
	var builder strings.Builder

	// 获取所有键并排序
	keys := make([]string, 0, len(tfCode))
	for k := range tfCode {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		value := tfCode[key]

		switch key {
		case "module":
			if moduleBlock, ok := value.(map[string]interface{}); ok {
				builder.WriteString(s.convertModuleBlockToHCL(moduleBlock, indent))
			}
		case "resource":
			if resourceBlock, ok := value.(map[string]interface{}); ok {
				builder.WriteString(s.convertResourceBlockToHCL(resourceBlock, indent))
			}
		case "variable":
			if varBlock, ok := value.(map[string]interface{}); ok {
				builder.WriteString(s.convertSpecialBlockToHCL("variable", varBlock, indent))
			}
		case "output":
			if outputBlock, ok := value.(map[string]interface{}); ok {
				builder.WriteString(s.convertSpecialBlockToHCL("output", outputBlock, indent))
			}
		case "data":
			if dataBlock, ok := value.(map[string]interface{}); ok {
				builder.WriteString(s.convertDataBlockToHCL(dataBlock, indent))
			}
		case "locals":
			if localsBlock, ok := value.(map[string]interface{}); ok {
				builder.WriteString("locals {\n")
				builder.WriteString(s.convertMapToHCLBody(localsBlock, 1))
				builder.WriteString("}\n\n")
			}
		case "provider":
			if providerBlock, ok := value.(map[string]interface{}); ok {
				builder.WriteString(s.convertSpecialBlockToHCL("provider", providerBlock, indent))
			}
		case "terraform":
			if terraformBlock, ok := value.(map[string]interface{}); ok {
				builder.WriteString("terraform {\n")
				builder.WriteString(s.convertMapToHCLBody(terraformBlock, 1))
				builder.WriteString("}\n\n")
			}
		default:
			// 其他顶层属性
			indentStr := strings.Repeat("  ", indent)
			builder.WriteString(fmt.Sprintf("%s%s = %s\n", indentStr, key, s.formatHCLValue(value)))
		}
	}

	return builder.String()
}

// convertDataBlockToHCL 将data块转换为HCL格式
func (s *ResourceService) convertDataBlockToHCL(dataBlock map[string]interface{}, indent int) string {
	var builder strings.Builder

	// 获取所有数据源类型并排序
	dataTypes := make([]string, 0, len(dataBlock))
	for dataType := range dataBlock {
		dataTypes = append(dataTypes, dataType)
	}
	sort.Strings(dataTypes)

	for _, dataType := range dataTypes {
		dataOfType := dataBlock[dataType]
		if dataMap, ok := dataOfType.(map[string]interface{}); ok {
			// 获取所有数据源名称并排序
			dataNames := make([]string, 0, len(dataMap))
			for name := range dataMap {
				dataNames = append(dataNames, name)
			}
			sort.Strings(dataNames)

			for _, dataName := range dataNames {
				config := dataMap[dataName]
				builder.WriteString(fmt.Sprintf("data \"%s\" \"%s\" {\n", dataType, dataName))

				if configMap, ok := config.(map[string]interface{}); ok {
					builder.WriteString(s.convertMapToHCLBody(configMap, 1))
				}

				builder.WriteString("}\n\n")
			}
		}
	}

	return builder.String()
}

// convertResourceBlockToHCL 将resource块转换为HCL格式
func (s *ResourceService) convertResourceBlockToHCL(resourceBlock map[string]interface{}, indent int) string {
	var builder strings.Builder

	// 获取所有资源类型并排序
	resourceTypes := make([]string, 0, len(resourceBlock))
	for resourceType := range resourceBlock {
		resourceTypes = append(resourceTypes, resourceType)
	}
	sort.Strings(resourceTypes)

	for _, resourceType := range resourceTypes {
		resourcesOfType := resourceBlock[resourceType]
		if resourceMap, ok := resourcesOfType.(map[string]interface{}); ok {
			// 获取所有资源名称并排序
			resourceNames := make([]string, 0, len(resourceMap))
			for name := range resourceMap {
				resourceNames = append(resourceNames, name)
			}
			sort.Strings(resourceNames)

			for _, resourceName := range resourceNames {
				config := resourceMap[resourceName]
				builder.WriteString(fmt.Sprintf("resource \"%s\" \"%s\" {\n", resourceType, resourceName))

				if configMap, ok := config.(map[string]interface{}); ok {
					builder.WriteString(s.convertMapToHCLBody(configMap, 1))
				}

				builder.WriteString("}\n\n")
			}
		}
	}

	return builder.String()
}

// convertMapToHCL 将map转换为HCL格式（顶层）
func (s *ResourceService) convertMapToHCL(data map[string]interface{}, indent int) string {
	var builder strings.Builder

	// 获取所有键并排序
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		value := data[key]
		indentStr := strings.Repeat("  ", indent)

		switch v := value.(type) {
		case map[string]interface{}:
			// 检查是否是特殊块（如resource, variable, output等）
			if key == "resource" || key == "variable" || key == "output" || key == "data" || key == "module" || key == "locals" || key == "provider" || key == "terraform" {
				if key == "resource" {
					builder.WriteString(s.convertResourceBlockToHCL(v, indent))
				} else {
					builder.WriteString(s.convertSpecialBlockToHCL(key, v, indent))
				}
			} else {
				builder.WriteString(fmt.Sprintf("%s%s = {\n", indentStr, key))
				builder.WriteString(s.convertMapToHCLBody(v, indent+1))
				builder.WriteString(fmt.Sprintf("%s}\n", indentStr))
			}
		default:
			builder.WriteString(fmt.Sprintf("%s%s = %s\n", indentStr, key, s.formatHCLValue(value)))
		}
	}

	return builder.String()
}

// convertSpecialBlockToHCL 转换特殊块（variable, output等）
func (s *ResourceService) convertSpecialBlockToHCL(blockType string, block map[string]interface{}, indent int) string {
	var builder strings.Builder
	indentStr := strings.Repeat("  ", indent)

	// 获取所有名称并排序
	names := make([]string, 0, len(block))
	for name := range block {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		config := block[name]
		builder.WriteString(fmt.Sprintf("%s%s \"%s\" {\n", indentStr, blockType, name))

		if configMap, ok := config.(map[string]interface{}); ok {
			builder.WriteString(s.convertMapToHCLBody(configMap, indent+1))
		}

		builder.WriteString(fmt.Sprintf("%s}\n\n", indentStr))
	}

	return builder.String()
}

// convertMapToHCLBody 将map转换为HCL块体
func (s *ResourceService) convertMapToHCLBody(data map[string]interface{}, indent int) string {
	var builder strings.Builder
	indentStr := strings.Repeat("  ", indent)

	// 获取所有键并排序
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		value := data[key]

		switch v := value.(type) {
		case map[string]interface{}:
			// 检查是否是嵌套块（如dynamic, lifecycle等）
			if s.isNestedBlock(key) {
				builder.WriteString(fmt.Sprintf("%s%s {\n", indentStr, key))
				builder.WriteString(s.convertMapToHCLBody(v, indent+1))
				builder.WriteString(fmt.Sprintf("%s}\n", indentStr))
			} else {
				builder.WriteString(fmt.Sprintf("%s%s = {\n", indentStr, key))
				builder.WriteString(s.convertMapToHCLBody(v, indent+1))
				builder.WriteString(fmt.Sprintf("%s}\n", indentStr))
			}
		case []interface{}:
			// 检查是否是块列表
			if len(v) > 0 {
				if _, isMap := v[0].(map[string]interface{}); isMap && s.isNestedBlock(key) {
					// 块列表
					for _, item := range v {
						if itemMap, ok := item.(map[string]interface{}); ok {
							builder.WriteString(fmt.Sprintf("%s%s {\n", indentStr, key))
							builder.WriteString(s.convertMapToHCLBody(itemMap, indent+1))
							builder.WriteString(fmt.Sprintf("%s}\n", indentStr))
						}
					}
				} else {
					// 普通列表
					builder.WriteString(fmt.Sprintf("%s%s = %s\n", indentStr, key, s.formatHCLValue(v)))
				}
			} else {
				builder.WriteString(fmt.Sprintf("%s%s = []\n", indentStr, key))
			}
		default:
			builder.WriteString(fmt.Sprintf("%s%s = %s\n", indentStr, key, s.formatHCLValue(value)))
		}
	}

	return builder.String()
}

// isNestedBlock 判断是否是嵌套块
func (s *ResourceService) isNestedBlock(key string) bool {
	nestedBlocks := map[string]bool{
		"dynamic":                              true,
		"lifecycle":                            true,
		"provisioner":                          true,
		"connection":                           true,
		"content":                              true,
		"for_each":                             false, // for_each是属性不是块
		"ingress":                              true,
		"egress":                               true,
		"rule":                                 true,
		"block_device_mappings":                true,
		"ebs_block_device":                     true,
		"root_block_device":                    true,
		"network_interface":                    true,
		"tag":                                  true,
		"tags":                                 false, // tags通常是map
		"metadata_options":                     true,
		"credit_specification":                 true,
		"capacity_reservation":                 true,
		"enclave_options":                      true,
		"hibernation_options":                  true,
		"instance_market_options":              true,
		"launch_template":                      true,
		"maintenance_options":                  true,
		"private_dns_name_options":             true,
		"cors_rule":                            true,
		"versioning":                           true,
		"logging":                              true,
		"website":                              true,
		"server_side_encryption_configuration": true,
		"grant":                                true,
		"replication_configuration":            true,
		"object_lock_configuration":            true,
		"statement":                            true,
		"condition":                            true,
		"principals":                           true,
		"not_principals":                       true,
		"actions":                              false,
		"not_actions":                          false,
		"resources":                            false,
		"not_resources":                        false,
	}

	return nestedBlocks[key]
}

// formatHCLValue 格式化HCL值
func (s *ResourceService) formatHCLValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		// 检查是否是Terraform表达式（变量引用、函数调用等）
		if s.isTerraformExpression(v) {
			return v
		}
		// 普通字符串需要引号
		return fmt.Sprintf("\"%s\"", s.escapeHCLString(v))
	case bool:
		return fmt.Sprintf("%t", v)
	case float64:
		// 检查是否是整数
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%g", v)
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case []interface{}:
		return s.formatHCLList(v)
	case map[string]interface{}:
		return s.formatHCLMap(v)
	case nil:
		return "null"
	default:
		return fmt.Sprintf("\"%v\"", v)
	}
}

// isTerraformExpression 判断是否是Terraform表达式
func (s *ResourceService) isTerraformExpression(str string) bool {
	// 变量引用
	if strings.HasPrefix(str, "var.") || strings.HasPrefix(str, "local.") ||
		strings.HasPrefix(str, "data.") || strings.HasPrefix(str, "module.") ||
		strings.HasPrefix(str, "aws_") || strings.HasPrefix(str, "azurerm_") ||
		strings.HasPrefix(str, "google_") || strings.HasPrefix(str, "kubernetes_") {
		return true
	}

	// 函数调用
	if strings.Contains(str, "(") && strings.Contains(str, ")") {
		return true
	}

	// 插值表达式
	if strings.Contains(str, "${") && strings.Contains(str, "}") {
		return true
	}

	// each引用
	if strings.HasPrefix(str, "each.") {
		return true
	}

	// count引用
	if strings.HasPrefix(str, "count.") {
		return true
	}

	// self引用
	if strings.HasPrefix(str, "self.") {
		return true
	}

	return false
}

// escapeHCLString 转义HCL字符串
func (s *ResourceService) escapeHCLString(str string) string {
	str = strings.ReplaceAll(str, "\\", "\\\\")
	str = strings.ReplaceAll(str, "\"", "\\\"")
	str = strings.ReplaceAll(str, "\n", "\\n")
	str = strings.ReplaceAll(str, "\r", "\\r")
	str = strings.ReplaceAll(str, "\t", "\\t")
	return str
}

// formatHCLList 格式化HCL列表
func (s *ResourceService) formatHCLList(list []interface{}) string {
	if len(list) == 0 {
		return "[]"
	}

	var items []string
	for _, item := range list {
		items = append(items, s.formatHCLValue(item))
	}

	// 如果列表较短，使用单行格式
	result := "[" + strings.Join(items, ", ") + "]"
	if len(result) <= 80 {
		return result
	}

	// 否则使用多行格式
	var builder strings.Builder
	builder.WriteString("[\n")
	for _, item := range items {
		builder.WriteString("    " + item + ",\n")
	}
	builder.WriteString("  ]")
	return builder.String()
}

// convertTerraformBlockToHCL 将 terraform 块转换为 HCL 格式
func (s *ResourceService) convertTerraformBlockToHCL(terraformBlock interface{}) string {
	var builder strings.Builder

	// terraform 块可能是数组或对象
	var configList []interface{}
	switch v := terraformBlock.(type) {
	case []interface{}:
		configList = v
	case map[string]interface{}:
		configList = []interface{}{v}
	default:
		return ""
	}

	for _, item := range configList {
		configMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		builder.WriteString("terraform {\n")

		// 处理 required_providers
		if requiredProviders, ok := configMap["required_providers"]; ok {
			builder.WriteString("  required_providers {\n")

			// required_providers 可能是数组或对象
			var providersList []interface{}
			switch rp := requiredProviders.(type) {
			case []interface{}:
				providersList = rp
			case map[string]interface{}:
				providersList = []interface{}{rp}
			}

			for _, providerItem := range providersList {
				if providerMap, ok := providerItem.(map[string]interface{}); ok {
					for providerName, providerConfig := range providerMap {
						if pc, ok := providerConfig.(map[string]interface{}); ok {
							builder.WriteString(fmt.Sprintf("    %s = {\n", providerName))
							if source, ok := pc["source"].(string); ok {
								builder.WriteString(fmt.Sprintf("      source  = \"%s\"\n", source))
							}
							if version, ok := pc["version"].(string); ok {
								builder.WriteString(fmt.Sprintf("      version = \"%s\"\n", version))
							}
							builder.WriteString("    }\n")
						}
					}
				}
			}

			builder.WriteString("  }\n")
		}

		// 处理其他 terraform 块属性
		for key, value := range configMap {
			if key == "required_providers" {
				continue // 已处理
			}
			builder.WriteString(fmt.Sprintf("  %s = %s\n", key, s.formatHCLValue(value)))
		}

		builder.WriteString("}\n")
	}

	return builder.String()
}

// convertProviderBlockToHCL 将 provider 块转换为 HCL 格式
func (s *ResourceService) convertProviderBlockToHCL(providerBlock interface{}) string {
	var builder strings.Builder

	providerMap, ok := providerBlock.(map[string]interface{})
	if !ok {
		return ""
	}

	// 获取所有 provider 名称并排序
	providerNames := make([]string, 0, len(providerMap))
	for name := range providerMap {
		providerNames = append(providerNames, name)
	}
	sort.Strings(providerNames)

	for _, providerName := range providerNames {
		providerConfig := providerMap[providerName]

		// provider 配置可能是数组或对象
		var configList []interface{}
		switch v := providerConfig.(type) {
		case []interface{}:
			configList = v
		case map[string]interface{}:
			configList = []interface{}{v}
		}

		for _, item := range configList {
			configMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}

			builder.WriteString(fmt.Sprintf("provider \"%s\" {\n", providerName))
			builder.WriteString(s.convertMapToHCLBody(configMap, 1))
			builder.WriteString("}\n\n")
		}
	}

	return builder.String()
}

// formatHCLMap 格式化HCL map
func (s *ResourceService) formatHCLMap(m map[string]interface{}) string {
	if len(m) == 0 {
		return "{}"
	}

	// 获取所有键并排序
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var items []string
	for _, k := range keys {
		v := m[k]
		items = append(items, fmt.Sprintf("%s = %s", k, s.formatHCLValue(v)))
	}

	// 如果map较短，使用单行格式
	result := "{ " + strings.Join(items, ", ") + " }"
	if len(result) <= 80 {
		return result
	}

	// 否则使用多行格式
	var builder strings.Builder
	builder.WriteString("{\n")
	for _, k := range keys {
		v := m[k]
		builder.WriteString(fmt.Sprintf("    %s = %s\n", k, s.formatHCLValue(v)))
	}
	builder.WriteString("  }")
	return builder.String()
}
