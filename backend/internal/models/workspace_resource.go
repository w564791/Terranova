package models

import (
	"time"
)

// WorkspaceResource 工作空间资源
type WorkspaceResource struct {
	ID               uint                   `gorm:"primaryKey" json:"id"`
	WorkspaceID      string                 `gorm:"type:varchar(50);not null;index:idx_workspace_resources_workspace" json:"workspace_id"`
	ResourceID       string                 `gorm:"type:varchar(100);not null" json:"resource_id"` // 如 "aws_s3_bucket.my_bucket"
	ResourceType     string                 `gorm:"type:varchar(50);not null;index:idx_workspace_resources_type" json:"resource_type"`
	ResourceName     string                 `gorm:"type:varchar(100);not null" json:"resource_name"`
	CurrentVersionID *uint                  `json:"current_version_id,omitempty"`
	IsActive         bool                   `gorm:"default:true;index:idx_workspace_resources_active" json:"is_active"`
	Description      string                 `gorm:"type:text" json:"description"`
	Tags             map[string]interface{} `gorm:"type:jsonb" json:"tags"`
	CreatedBy        *string                `json:"created_by,omitempty"`
	CreatedAt        time.Time              `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt        time.Time              `gorm:"autoUpdateTime" json:"updated_at"`

	// Apply 状态
	LastAppliedAt *time.Time `gorm:"index:idx_workspace_resources_last_applied" json:"last_applied_at,omitempty"` // 最后一次成功 apply 的时间

	// Manifest 部署关联
	ManifestDeploymentID *string `gorm:"type:varchar(36);index:idx_workspace_resources_manifest_deployment" json:"manifest_deployment_id,omitempty"`

	// 关联
	Workspace      Workspace            `gorm:"foreignKey:WorkspaceID" json:"-"`
	CurrentVersion *ResourceCodeVersion `gorm:"foreignKey:CurrentVersionID;references:ID" json:"current_version,omitempty"`
	Creator        *User                `gorm:"foreignKey:CreatedBy" json:"creator,omitempty"`
}

// TableName 指定表名
func (WorkspaceResource) TableName() string {
	return "workspace_resources"
}

// ResourceCodeVersion 资源代码版本
type ResourceCodeVersion struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	ResourceID       uint      `gorm:"not null;index:idx_resource_code_versions_resource" json:"resource_id"`
	Version          int       `gorm:"not null" json:"version"`
	IsLatest         bool      `gorm:"default:false;index:idx_resource_code_versions_latest" json:"is_latest"`
	TFCode           JSONB     `gorm:"type:jsonb;not null" json:"tf_code"`
	Variables        JSONB     `gorm:"type:jsonb" json:"variables"`
	ChangeSummary    string    `gorm:"type:text" json:"change_summary"`
	ChangeType       string    `gorm:"type:varchar(20)" json:"change_type"` // create, update, delete, rollback
	DiffFromPrevious string    `gorm:"type:text" json:"diff_from_previous"`
	StateVersionID   *uint     `json:"state_version_id,omitempty"`
	TaskID           *uint     `json:"task_id,omitempty"`
	CreatedBy        *string   `json:"created_by,omitempty"`
	CreatedAt        time.Time `gorm:"autoCreateTime" json:"created_at"`

	// 关联
	Resource     WorkspaceResource      `gorm:"foreignKey:ResourceID" json:"-"`
	StateVersion *WorkspaceStateVersion `gorm:"foreignKey:StateVersionID" json:"state_version,omitempty"`
	Task         *WorkspaceTask         `gorm:"foreignKey:TaskID" json:"task,omitempty"`
	Creator      *User                  `gorm:"foreignKey:CreatedBy" json:"creator,omitempty"`
}

// TableName 指定表名
func (ResourceCodeVersion) TableName() string {
	return "resource_code_versions"
}

// WorkspaceResourcesSnapshot 工作空间资源快照
type WorkspaceResourcesSnapshot struct {
	ID                uint                   `gorm:"primaryKey" json:"id"`
	WorkspaceID       string                 `gorm:"type:varchar(50);not null;index:idx_workspace_resources_snapshot_workspace" json:"workspace_id"`
	SnapshotName      string                 `gorm:"type:varchar(100)" json:"snapshot_name"`
	ResourcesVersions map[string]interface{} `gorm:"type:jsonb;not null" json:"resources_versions"` // {"resource_id": version_id}
	TaskID            *uint                  `json:"task_id,omitempty"`
	StateVersionID    *uint                  `json:"state_version_id,omitempty"`
	CreatedBy         *string                `json:"created_by,omitempty"`
	CreatedAt         time.Time              `gorm:"autoCreateTime" json:"created_at"`
	Description       string                 `gorm:"type:text" json:"description"`

	// 关联
	Workspace    Workspace              `gorm:"foreignKey:WorkspaceID" json:"-"`
	Task         *WorkspaceTask         `gorm:"foreignKey:TaskID" json:"task,omitempty"`
	StateVersion *WorkspaceStateVersion `gorm:"foreignKey:StateVersionID" json:"state_version,omitempty"`
	Creator      *User                  `gorm:"foreignKey:CreatedBy" json:"creator,omitempty"`
}

// TableName 指定表名
func (WorkspaceResourcesSnapshot) TableName() string {
	return "workspace_resources_snapshot"
}

// ResourceDependency 资源依赖关系
type ResourceDependency struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	WorkspaceID         string    `gorm:"type:varchar(50);not null" json:"workspace_id"`
	ResourceID          uint      `gorm:"not null;index:idx_resource_dependencies_resource" json:"resource_id"`
	DependsOnResourceID uint      `gorm:"not null;index:idx_resource_dependencies_depends_on" json:"depends_on_resource_id"`
	DependencyType      string    `gorm:"type:varchar(20);default:explicit" json:"dependency_type"` // explicit, implicit
	CreatedAt           time.Time `gorm:"autoCreateTime" json:"created_at"`

	// 关联
	Workspace         Workspace         `gorm:"foreignKey:WorkspaceID" json:"-"`
	Resource          WorkspaceResource `gorm:"foreignKey:ResourceID" json:"-"`
	DependsOnResource WorkspaceResource `gorm:"foreignKey:DependsOnResourceID" json:"depends_on_resource,omitempty"`
}

// TableName 指定表名
func (ResourceDependency) TableName() string {
	return "resource_dependencies"
}
