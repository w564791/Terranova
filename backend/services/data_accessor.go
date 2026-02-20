package services

import (
	"iac-platform/internal/models"
)

// DataAccessor 数据访问接口
// 用于抽象 Local 模式和 Agent 模式的数据访问方式
type DataAccessor interface {
	// Workspace 相关
	GetWorkspace(workspaceID string) (*models.Workspace, error)
	GetWorkspaceResources(workspaceID string) ([]models.WorkspaceResource, error)
	GetWorkspaceVariables(workspaceID string, varType models.VariableType) ([]models.WorkspaceVariable, error)
	LockWorkspace(workspaceID, userID, reason string) error
	UnlockWorkspace(workspaceID string) error
	UpdateWorkspaceFields(workspaceID string, updates map[string]interface{}) error

	// Terraform Lock 文件相关（用于加速 terraform init）
	GetTerraformLockHCL(workspaceID string) (string, error)
	SaveTerraformLockHCL(workspaceID string, lockContent string) error

	// State 相关
	GetLatestStateVersion(workspaceID string) (*models.WorkspaceStateVersion, error)
	SaveStateVersion(version *models.WorkspaceStateVersion) error
	UpdateWorkspaceState(workspaceID string, stateContent map[string]interface{}) error
	GetMaxStateVersion(workspaceID string) (int, error)

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
	UpdateResourceStatus(taskID uint, resourceAddress, status, action string) error

	// Plan parsing
	ParsePlanChanges(taskID uint, planOutput string) error

	// Transaction 支持
	BeginTransaction() (DataAccessor, error)
	Commit() error
	Rollback() error
}
