package services

import (
	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// DataAccessor 数据访问接口
// 用于抽象 Local 模式和 Agent 模式的数据访问方式
type DataAccessor interface {
	// Workspace 相关
	GetWorkspace(workspaceID string) (*models.Workspace, error)
	GetWorkspaceResources(workspaceID string) ([]models.WorkspaceResource, error)
	GetWorkspaceVariables(workspaceID string, varType models.VariableType) ([]models.WorkspaceVariable, error)
	LoadSnapshot(vsnapID string, db *gorm.DB) error
	// SetVariableOverrides 设置 manifest deployment 应急覆盖(最高优先级,仅 Terraform 变量),
	// executor 在任务执行前从任务行 variable_overrides 快照注入。空 map 等价于不覆盖。
	SetVariableOverrides(overrides map[string]string)
	LockWorkspace(workspaceID string, lockInfo map[string]interface{}) error
	UnlockWorkspace(workspaceID string) error
	UpdateWorkspaceFields(workspaceID string, updates map[string]interface{}) error

	// Terraform Lock 文件相关（用于加速 terraform init）
	GetTerraformLockHCL(workspaceID string) (string, error)
	SaveTerraformLockHCL(workspaceID string, lockContent string) error

	// Manifest provider schema（post_init 落库；按 manifest+subpath，用 provider 版本指纹跳过重复提取）
	// Meta 含 ProviderVersionsKey；无记录返回 (nil, nil)
	GetManifestProviderSchemaMetaByWorkspace(workspaceID string) (*models.ManifestProviderSchema, error)
	// Upsert：由 workspace 解析 manifest_id+subpath 后写入；row 可不填 ManifestID/Subpath/ID
	UpsertManifestProviderSchemaByWorkspace(workspaceID string, row *models.ManifestProviderSchema) error

	// State 相关
	GetLatestStateVersion(workspaceID string) (*models.WorkspaceStateVersion, error)
	GetMaxStateVersion(workspaceID string) (int, error)

	// State Watcher 相关（temp state 管理）
	UpsertTempState(version *models.WorkspaceStateVersion) error
	PromoteTempState(workspaceID string, recordID uint) error
	CleanupOrphanedTempStates(workspaceID string) error

	// Task 相关
	GetTask(taskID uint) (*models.WorkspaceTask, error)
	GetPlanTask(taskID uint) (*models.WorkspaceTask, error)
	UpdateTask(task *models.WorkspaceTask) error
	SaveTaskLog(taskID uint, phase, content, level string) error
	GetTaskLogs(taskID uint) ([]models.TaskLog, error)

	// Resource 相关
	GetResourceVersion(versionID uint) (*models.ResourceCodeVersion, error)
	CountActiveResources(workspaceID string) (int64, error)
	GetWorkspaceResourcesWithVersions(workspaceID string) ([]models.WorkspaceResource, error)
	GetResourceByVersionID(resourceID string, versionID uint) (*models.WorkspaceResource, error)
	GetResourceByVersion(resourceID string, version int) (*models.WorkspaceResource, error)
	CheckResourceVersionExists(resourceID string, versionID uint) (bool, error)
	CheckResourceVersionExistsByVersion(resourceID string, version int) (bool, error)
	// resourceID 为完成行实时捕获的云端资源 ID，可为空（空时不覆盖已有 resource_id）
	UpdateResourceStatus(taskID uint, resourceAddress, status, action, resourceID string) error

	// Plan parsing
	ParsePlanChanges(taskID uint, planOutput string) error

	// Manifest 相关 (新设计 manifest_files 软链接架构)
	GetManifestFilesByTag(deploymentID, tag string) ([]models.ManifestFile, error)

	// Transaction 支持
	BeginTransaction() (DataAccessor, error)
	Commit() error
	Rollback() error
}
