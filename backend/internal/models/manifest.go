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
type ManifestVersion struct {
	ID         string          `json:"id" gorm:"primaryKey;size:36"`              // 格式: mfv-{ulid}
	ManifestID string          `json:"manifest_id" gorm:"size:36;not null;index"` // 所属 Manifest
	Version    string          `json:"version" gorm:"size:50;not null"`           // 版本号，如 v1.0.0, draft
	CanvasData json.RawMessage `json:"canvas_data" gorm:"type:jsonb;not null"`    // 画布数据
	Nodes      json.RawMessage `json:"nodes" gorm:"type:jsonb;not null"`          // 节点配置
	Edges      json.RawMessage `json:"edges" gorm:"type:jsonb;not null"`          // 连接关系
	Variables  json.RawMessage `json:"variables" gorm:"type:jsonb"`               // 可配置变量
	HCLContent string          `json:"hcl_content" gorm:"type:text"`              // 生成的 HCL
	IsDraft    bool            `json:"is_draft" gorm:"default:true;index"`        // 是否为草稿
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
	ID                string          `json:"id" gorm:"primaryKey;size:36"`                // 格式: mfd-{ulid}
	ManifestID        string          `json:"manifest_id" gorm:"size:36;not null;index"`   // 所属 Manifest
	VersionID         string          `json:"version_id" gorm:"size:36;not null"`          // 部署的版本
	WorkspaceID       int             `json:"workspace_id" gorm:"not null;index"`          // 目标 Workspace
	VariableOverrides json.RawMessage `json:"variable_overrides" gorm:"type:jsonb"`        // 变量覆盖
	Status            string          `json:"status" gorm:"size:20;default:pending;index"` // pending, deploying, deployed, failed
	LastTaskID        *int            `json:"last_task_id" gorm:""`                        // 最后一次部署的任务 ID
	DeployedBy        string          `json:"deployed_by" gorm:"size:20;not null"`         // 部署者
	DeployedAt        *time.Time      `json:"deployed_at" gorm:""`                         // 部署时间
	CreatedAt         time.Time       `json:"created_at" gorm:"autoCreateTime"`            // 创建时间
	UpdatedAt         time.Time       `json:"updated_at" gorm:"autoUpdateTime"`            // 更新时间

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

// ========== JSONB 数据结构 ==========

// ManifestNode 节点配置
type ManifestNode struct {
	ID             string                 `json:"id"`                       // 节点唯一标识
	Type           string                 `json:"type"`                     // module, variable
	ModuleID       *int                   `json:"module_id,omitempty"`      // 关联的平台 Module ID
	IsLinked       bool                   `json:"is_linked"`                // 是否已关联平台 Module
	LinkStatus     string                 `json:"link_status"`              // linked, unlinked, mismatch
	ModuleSource   string                 `json:"module_source,omitempty"`  // Module source
	ModuleVersion  string                 `json:"module_version,omitempty"` // Module 版本
	InstanceName   string                 `json:"instance_name"`            // 实例名称
	ResourceName   string                 `json:"resource_name"`            // 资源名称
	RawSource      string                 `json:"raw_source,omitempty"`     // 原始 source
	RawVersion     string                 `json:"raw_version,omitempty"`    // 原始 version
	RawConfig      map[string]interface{} `json:"raw_config,omitempty"`     // 原始配置
	Position       ManifestNodePosition   `json:"position"`                 // 位置信息
	Config         map[string]interface{} `json:"config"`                   // 配置参数
	ConfigComplete bool                   `json:"config_complete"`          // 参数是否完整
	Ports          []ManifestPort         `json:"ports"`                    // 暴露的端口
}

// ManifestNodePosition 节点位置
type ManifestNodePosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// ManifestPort 端口定义
type ManifestPort struct {
	ID          string `json:"id"`                    // 端口 ID
	Type        string `json:"type"`                  // input, output
	Name        string `json:"name"`                  // 变量名
	DataType    string `json:"data_type,omitempty"`   // 数据类型
	Description string `json:"description,omitempty"` // 描述
}

// ManifestEdge 连接关系
type ManifestEdge struct {
	ID         string            `json:"id"`                   // 连接 ID
	Type       string            `json:"type"`                 // dependency, variable_binding
	Source     ManifestEdgePoint `json:"source"`               // 源节点
	Target     ManifestEdgePoint `json:"target"`               // 目标节点
	Expression string            `json:"expression,omitempty"` // 变量绑定表达式
}

// ManifestEdgePoint 连接端点
type ManifestEdgePoint struct {
	NodeID string `json:"node_id"`           // 节点 ID
	PortID string `json:"port_id,omitempty"` // 端口 ID
}

// ManifestVariable 可配置变量
type ManifestVariable struct {
	Name        string      `json:"name"`                  // 变量名
	Type        string      `json:"type"`                  // string, number, bool, list, map
	Description string      `json:"description,omitempty"` // 描述
	Default     interface{} `json:"default,omitempty"`     // 默认值
	Required    bool        `json:"required"`              // 是否必填
	Sensitive   bool        `json:"sensitive,omitempty"`   // 是否敏感
}

// ManifestCanvasData 画布数据
type ManifestCanvasData struct {
	Viewport ManifestViewport `json:"viewport"` // 视口
	Zoom     float64          `json:"zoom"`     // 缩放比例
}

// ManifestViewport 视口
type ManifestViewport struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
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

// SaveManifestVersionRequest 保存版本请求
type SaveManifestVersionRequest struct {
	CanvasData json.RawMessage `json:"canvas_data" binding:"required"`
	Nodes      json.RawMessage `json:"nodes" binding:"required"`
	Edges      json.RawMessage `json:"edges" binding:"required"`
	Variables  json.RawMessage `json:"variables"`
}

// PublishManifestVersionRequest 发布版本请求
type PublishManifestVersionRequest struct {
	Version string `json:"version" binding:"required,max=50"` // 如 v1.0.0
}

// CreateManifestDeploymentRequest 创建部署请求
type CreateManifestDeploymentRequest struct {
	VersionID         string          `json:"version_id" binding:"required"`
	WorkspaceID       int             `json:"workspace_id" binding:"required"`
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

	// 部署状态
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
