package models

import (
	"encoding/json"
	"time"
)

// Manifest 可视化编排模板（Organization 级别）
type Manifest struct {
	ID             string    `json:"id" gorm:"primaryKey;size:36"`              // 格式: mf-{ulid}
	OrganizationID int       `json:"organization_id" gorm:"not null;index"`     // 所属组织
	Name           string    `json:"name" gorm:"size:255;not null"`             // 名称
	Description    string    `json:"description" gorm:"type:text"`              // 描述
	Status         string    `json:"status" gorm:"size:20;default:draft;index"` // draft, published, archived
	CreatedBy      string    `json:"created_by" gorm:"size:20;not null"`        // 创建者
	CreatedAt      time.Time `json:"created_at" gorm:"autoCreateTime"`          // 创建时间
	UpdatedAt      time.Time `json:"updated_at" gorm:"autoUpdateTime"`          // 更新时间

	// 关联
	Versions    []ManifestVersion    `json:"versions,omitempty" gorm:"foreignKey:ManifestID"`
	Deployments []ManifestDeployment `json:"deployments,omitempty" gorm:"foreignKey:ManifestID"`

	// 非数据库字段
	LatestVersion   *ManifestVersion `json:"latest_version,omitempty" gorm:"-"`
	DeploymentCount int              `json:"deployment_count,omitempty" gorm:"-"`
	CreatedByName   string           `json:"created_by_name,omitempty" gorm:"-"`
}

func (Manifest) TableName() string {
	return "manifests"
}

// ManifestVersion Manifest 版本
//
// 新模型下版本只是 manifest_files 快照的元信息行,文件内容存 manifest_files
// (version_id = 本行 id)。画布相关旧字段 (canvas_data/nodes/edges/hcl_content/is_draft)
// 已在 PR4 移除。
type ManifestVersion struct {
	ID         string          `json:"id" gorm:"primaryKey;size:36"`              // 格式: mfv-{ulid}
	ManifestID string          `json:"manifest_id" gorm:"size:36;not null;index"` // 所属 Manifest
	Version    string          `json:"version" gorm:"size:50;not null"`           // 版本号，如 v1.0.0
	Variables  json.RawMessage `json:"variables" gorm:"type:jsonb"`               // 该版本声明的 Terraform input variables 元信息(.tf 静态解析)
	Changelog  string          `json:"changelog" gorm:"type:text"`                // 发布说明
	CreatedBy  string          `json:"created_by" gorm:"size:20;not null"`        // 创建者
	CreatedAt  time.Time       `json:"created_at" gorm:"autoCreateTime"`          // 创建时间

	// 非数据库字段
	CreatedByName string `json:"created_by_name,omitempty" gorm:"-"`
}

func (ManifestVersion) TableName() string {
	return "manifest_versions"
}

// ManifestDeployment Manifest 部署记录
type ManifestDeployment struct {
	ID                string          `json:"id" gorm:"primaryKey;size:36"`                              // 格式: mfd-{ulid}
	ManifestID        string          `json:"manifest_id" gorm:"size:36;not null;index"`                 // 所属 Manifest
	VersionID         string          `json:"version_id" gorm:"size:36;not null"`                        // 部署的版本
	WorkspaceID       string          `json:"workspace_id" gorm:"type:varchar(50);not null;index"`       // 目标 Workspace 语义化ID(对齐全平台)
	VariableOverrides json.RawMessage `json:"variable_overrides" gorm:"type:jsonb"`                      // 应急变量覆盖(扁平 key->string,优先级最高)
	Status            string          `json:"status" gorm:"size:20;default:active;index"`                // active, uninstalled
	LastTaskID        *int            `json:"last_task_id" gorm:""`                                      // 最后一次部署的任务 ID
	DeployedBy        string          `json:"deployed_by" gorm:"size:20;not null"`                       // 部署者
	DeployedAt        *time.Time      `json:"deployed_at" gorm:""`                                       // 部署时间
	CreatedAt         time.Time       `json:"created_at" gorm:"autoCreateTime"`                          // 创建时间
	UpdatedAt         time.Time       `json:"updated_at" gorm:"autoUpdateTime"`                          // 更新时间

	// 关联
	Version   *ManifestVersion             `json:"version,omitempty" gorm:"foreignKey:VersionID"`
	Resources []ManifestDeploymentResource `json:"resources,omitempty" gorm:"foreignKey:DeploymentID"`

	// 非数据库字段
	WorkspaceName       string `json:"workspace_name,omitempty" gorm:"-"`
	WorkspaceSemanticID string `json:"workspace_semantic_id,omitempty" gorm:"-"` // ws-xxx 格式
	DeployedByName      string `json:"deployed_by_name,omitempty" gorm:"-"`
	VersionName         string `json:"version_name,omitempty" gorm:"-"`
}

func (ManifestDeployment) TableName() string {
	return "manifest_deployments"
}

// ManifestDeploymentResource 部署资源关联
type ManifestDeploymentResource struct {
	ID           string    `json:"id" gorm:"primaryKey;size:36"`                // 格式: mdr-{ulid}
	DeploymentID string    `json:"deployment_id" gorm:"size:36;not null;index"` // 所属部署
	NodeID       string    `json:"node_id" gorm:"size:50;not null"`             // Manifest 中的节点 ID
	ResourceID   string    `json:"resource_id" gorm:"size:255;not null;index"`  // workspace_resources.resource_id (语义 ID)
	ConfigHash   string    `json:"config_hash" gorm:"size:64"`                  // 部署时的配置 hash，用于漂移检测
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime"`            // 创建时间
}

func (ManifestDeploymentResource) TableName() string {
	return "manifest_deployment_resources"
}

// ========== 请求/响应结构 ==========

// CreateManifestRequest 创建 Manifest 请求
type CreateManifestRequest struct {
	Name        string `json:"name" binding:"required,max=255"`
	Description string `json:"description"`
}

// UpdateManifestRequest 更新 Manifest 请求
type UpdateManifestRequest struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status,omitempty"` // draft, published, archived
}

// PublishManifestVersionRequest 发布版本请求
type PublishManifestVersionRequest struct {
	Version string `json:"version" binding:"required,max=50"` // 如 v1.0.0
}

// CreateManifestDeploymentRequest 创建部署请求
type CreateManifestDeploymentRequest struct {
	VersionID         string          `json:"version_id" binding:"required"`
	WorkspaceID       string          `json:"workspace_id" binding:"required"` // 语义化ID ws-xxx
	VariableOverrides json.RawMessage `json:"variable_overrides"`
	AutoApply         bool            `json:"auto_apply"` // 是否自动 Apply
	PlanOnly          bool            `json:"plan_only"`  // 仅 Plan
}

// UpdateManifestDeploymentRequest 更新部署请求
type UpdateManifestDeploymentRequest struct {
	VersionID         string          `json:"version_id,omitempty"`
	VariableOverrides json.RawMessage `json:"variable_overrides,omitempty"`
	AutoApply         bool            `json:"auto_apply"`
	PlanOnly          bool            `json:"plan_only"`
}

// ManifestListResponse 列表响应
type ManifestListResponse struct {
	Items      []Manifest `json:"items"`
	Total      int64      `json:"total"`
	Page       int        `json:"page"`
	PageSize   int        `json:"page_size"`
	TotalPages int        `json:"total_pages"`
}

// ManifestVersionListResponse 版本列表响应
type ManifestVersionListResponse struct {
	Items      []ManifestVersion `json:"items"`
	Total      int64             `json:"total"`
	Page       int               `json:"page"`
	PageSize   int               `json:"page_size"`
	TotalPages int               `json:"total_pages"`
}

// ManifestDeploymentListResponse 部署列表响应
type ManifestDeploymentListResponse struct {
	Items      []ManifestDeployment `json:"items"`
	Total      int64                `json:"total"`
	Page       int                  `json:"page"`
	PageSize   int                  `json:"page_size"`
	TotalPages int                  `json:"total_pages"`
}

// ========== 常量定义 ==========

const (
	// Manifest 状态
	ManifestStatusDraft     = "draft"
	ManifestStatusPublished = "published"
	ManifestStatusArchived  = "archived"

	// 部署状态(新设计)
	DeploymentStatusActive      = "active"
	DeploymentStatusUninstalled = "uninstalled"

	// 部署状态(旧画布,兼容期保留;新代码不再写入)
	DeploymentStatusPending   = "pending"
	DeploymentStatusDeploying = "deploying"
	DeploymentStatusDeployed  = "deployed"
	DeploymentStatusFailed    = "failed"
	DeploymentStatusArchived  = "archived" // 已废弃

	// 节点类型
	NodeTypeModule   = "module"
	NodeTypeVariable = "variable"

	// 连接类型
	EdgeTypeDependency      = "dependency"
	EdgeTypeVariableBinding = "variable_binding"

	// 端口类型
	PortTypeInput  = "input"
	PortTypeOutput = "output"

	// 关联状态
	LinkStatusLinked   = "linked"
	LinkStatusUnlinked = "unlinked"
	LinkStatusMismatch = "mismatch"
)
