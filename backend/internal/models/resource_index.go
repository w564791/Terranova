package models

import (
	"encoding/json"
	"time"
)

// ResourceIndex 资源索引表 - 存储从Terraform state解析的资源信息
type ResourceIndex struct {
	ID uint `gorm:"primaryKey" json:"id"`

	// 基本标识
	WorkspaceID      string `gorm:"column:workspace_id;type:varchar(50);not null;index:idx_resource_index_workspace" json:"workspace_id"`
	TerraformAddress string `gorm:"column:terraform_address;type:text;not null" json:"terraform_address"`

	// 资源信息
	ResourceType string `gorm:"column:resource_type;type:varchar(100);not null;index:idx_resource_index_type" json:"resource_type"`
	ResourceName string `gorm:"column:resource_name;type:varchar(100);not null" json:"resource_name"`
	ResourceMode string `gorm:"column:resource_mode;type:varchar(20);not null;default:'managed';index:idx_resource_index_mode" json:"resource_mode"`
	IndexKey     string `gorm:"column:index_key;type:text" json:"index_key,omitempty"`

	// 云资源信息（从attributes提取）
	CloudResourceID   string `gorm:"column:cloud_resource_id;type:varchar(255);index:idx_resource_index_cloud_id" json:"cloud_resource_id,omitempty"`
	CloudResourceName string `gorm:"column:cloud_resource_name;type:varchar(255);index:idx_resource_index_cloud_name" json:"cloud_resource_name,omitempty"`
	CloudResourceARN  string `gorm:"column:cloud_resource_arn;type:text" json:"cloud_resource_arn,omitempty"`
	Description       string `gorm:"column:description;type:text" json:"description,omitempty"`

	// Module层级信息
	ModulePath       string `gorm:"column:module_path;type:text;index:idx_resource_index_module_path" json:"module_path,omitempty"`
	ModuleDepth      int    `gorm:"column:module_depth;default:0" json:"module_depth"`
	ParentModulePath string `gorm:"column:parent_module_path;type:text;index:idx_resource_index_parent_module" json:"parent_module_path,omitempty"`
	RootModuleName   string `gorm:"column:root_module_name;type:varchar(100);index:idx_resource_index_root_module" json:"root_module_name,omitempty"`

	// 属性快照
	Attributes json.RawMessage `gorm:"column:attributes;type:jsonb" json:"attributes,omitempty"`
	Tags       json.RawMessage `gorm:"column:tags;type:jsonb" json:"tags,omitempty"`

	// 元数据
	Provider       string    `gorm:"column:provider;type:varchar(200)" json:"provider,omitempty"`
	StateVersionID *uint     `gorm:"column:state_version_id" json:"state_version_id,omitempty"`
	LastSyncedAt   time.Time `gorm:"column:last_synced_at;default:CURRENT_TIMESTAMP" json:"last_synced_at"`
	CreatedAt      time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"created_at"`

	// 外部数据源相关字段
	SourceType       string `gorm:"column:source_type;type:varchar(20);default:'terraform';index:idx_resource_index_source_type" json:"source_type"`         // terraform 或 external
	ExternalSourceID string `gorm:"column:external_source_id;type:varchar(50);index:idx_resource_index_external_source" json:"external_source_id,omitempty"` // 外部数据源ID
	CloudProvider    string `gorm:"column:cloud_provider;type:varchar(50);index:idx_resource_index_cloud_provider" json:"cloud_provider,omitempty"`          // 云提供商: aws/azure/gcp/aliyun
	CloudAccountID   string `gorm:"column:cloud_account_id;type:varchar(100);index:idx_resource_index_cloud_account" json:"cloud_account_id,omitempty"`      // 云账户ID
	CloudAccountName string `gorm:"column:cloud_account_name;type:varchar(200)" json:"cloud_account_name,omitempty"`                                         // 云账户名称
	CloudRegion      string `gorm:"column:cloud_region;type:varchar(50)" json:"cloud_region,omitempty"`                                                      // 云区域
	PrimaryKeyValue  string `gorm:"column:primary_key_value;type:varchar(500);index:idx_resource_index_primary_key" json:"primary_key_value,omitempty"`      // 主键值（用于唯一标识外部资源）

	// 向量搜索相关字段（CMDB 向量化搜索）
	// 注意：Embedding 字段使用 gorm:"-" 忽略自动映射，因为 GORM 不支持 pgvector 类型的自动序列化/反序列化
	// embedding 的读写使用原生 SQL 操作（如 UPDATE ... SET embedding = ?::vector）
	Embedding          []float32  `gorm:"-" json:"-"`                                                                // 资源的语义向量（1024维，Amazon Titan Embed Text V2）
	EmbeddingText      string     `gorm:"column:embedding_text;type:text" json:"embedding_text,omitempty"`           // 用于生成 embedding 的原始文本
	EmbeddingModel     string     `gorm:"column:embedding_model;type:varchar(100)" json:"embedding_model,omitempty"` // 使用的 embedding 模型 ID
	EmbeddingUpdatedAt *time.Time `gorm:"column:embedding_updated_at" json:"embedding_updated_at,omitempty"`         // embedding 最后更新时间
}

func (ResourceIndex) TableName() string {
	return "resource_index"
}

// ModuleHierarchy Module层级表 - 存储module的树状结构
type ModuleHierarchy struct {
	ID uint `gorm:"primaryKey" json:"id"`

	WorkspaceID string `gorm:"column:workspace_id;type:varchar(50);not null;index:idx_module_hierarchy_workspace" json:"workspace_id"`
	ModulePath  string `gorm:"column:module_path;type:text;not null" json:"module_path"`
	ModuleName  string `gorm:"column:module_name;type:varchar(100);not null" json:"module_name"`
	ModuleKey   string `gorm:"column:module_key;type:text" json:"module_key,omitempty"`
	ParentPath  string `gorm:"column:parent_path;type:text;index:idx_module_hierarchy_parent" json:"parent_path,omitempty"`
	Depth       int    `gorm:"column:depth;default:0;index:idx_module_hierarchy_depth" json:"depth"`

	// 统计信息
	ResourceCount      int `gorm:"column:resource_count;default:0" json:"resource_count"`
	TotalResourceCount int `gorm:"column:total_resource_count;default:0" json:"total_resource_count"`
	ChildModuleCount   int `gorm:"column:child_module_count;default:0" json:"child_module_count"`

	// 元数据
	Source       string    `gorm:"column:source;type:varchar(500)" json:"source,omitempty"`
	LastSyncedAt time.Time `gorm:"column:last_synced_at;default:CURRENT_TIMESTAMP" json:"last_synced_at"`
	CreatedAt    time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"created_at"`
}

func (ModuleHierarchy) TableName() string {
	return "module_hierarchy"
}

// ResourceSearchResult 资源搜索结果
type ResourceSearchResult struct {
	WorkspaceID          string  `json:"workspace_id" gorm:"column:workspace_id"`
	WorkspaceName        string  `json:"workspace_name,omitempty" gorm:"column:workspace_name"`
	TerraformAddress     string  `json:"terraform_address" gorm:"column:terraform_address"`
	ResourceType         string  `json:"resource_type" gorm:"column:resource_type"`
	ResourceName         string  `json:"resource_name" gorm:"column:resource_name"`
	CloudResourceID      string  `json:"cloud_resource_id,omitempty" gorm:"column:cloud_resource_id"`
	CloudResourceName    string  `json:"cloud_resource_name,omitempty" gorm:"column:cloud_resource_name"`
	CloudResourceARN     string  `json:"cloud_resource_arn,omitempty" gorm:"column:cloud_resource_arn"` // 云资源全局标识符（AWS ARN / Azure Resource ID / GCP Resource Name）
	Description          string  `json:"description,omitempty" gorm:"column:description"`
	ModulePath           string  `json:"module_path,omitempty" gorm:"column:module_path"`
	RootModuleName       string  `json:"root_module_name,omitempty" gorm:"column:root_module_name"`
	PlatformResourceID   *uint   `json:"platform_resource_id,omitempty" gorm:"column:platform_resource_id"`
	PlatformResourceName string  `json:"platform_resource_name,omitempty" gorm:"column:platform_resource_name"`
	JumpURL              string  `json:"jump_url,omitempty" gorm:"column:jump_url"`
	MatchRank            float32 `json:"match_rank" gorm:"column:match_rank"`

	// 外部数据源相关字段
	SourceType         string `json:"source_type" gorm:"column:source_type"`                             // terraform 或 external
	ExternalSourceID   string `json:"external_source_id,omitempty" gorm:"column:external_source_id"`     // 外部数据源ID
	ExternalSourceName string `json:"external_source_name,omitempty" gorm:"column:external_source_name"` // 外部数据源名称
	CloudProvider      string `json:"cloud_provider,omitempty" gorm:"column:cloud_provider"`             // 云提供商
	CloudAccountID     string `json:"cloud_account_id,omitempty" gorm:"column:cloud_account_id"`         // 云账户ID
	CloudAccountName   string `json:"cloud_account_name,omitempty" gorm:"column:cloud_account_name"`     // 云账户名称
	CloudRegion        string `json:"cloud_region,omitempty" gorm:"column:cloud_region"`                 // 云区域
}

// ResourceTreeNode 资源树节点
type ResourceTreeNode struct {
	Type               string              `json:"type"` // "module" 或 "resource"
	Name               string              `json:"name"`
	Path               string              `json:"path,omitempty"`
	TerraformAddress   string              `json:"terraform_address,omitempty"`
	TerraformType      string              `json:"terraform_type,omitempty"`
	TerraformName      string              `json:"terraform_name,omitempty"`
	CloudID            string              `json:"cloud_id,omitempty"`
	CloudName          string              `json:"cloud_name,omitempty"`
	CloudARN           string              `json:"cloud_arn,omitempty"` // 云资源全局标识符（AWS ARN / Azure Resource ID / GCP Resource Name）
	Description        string              `json:"description,omitempty"`
	Mode               string              `json:"mode,omitempty"`
	ResourceCount      int                 `json:"resource_count,omitempty"`
	Children           []*ResourceTreeNode `json:"children,omitempty"`
	PlatformResourceID *uint               `json:"platform_resource_id,omitempty"`
	JumpURL            string              `json:"jump_url,omitempty"`
}

// WorkspaceResourceTree Workspace资源树响应
type WorkspaceResourceTree struct {
	WorkspaceID    string              `json:"workspace_id"`
	WorkspaceName  string              `json:"workspace_name"`
	TotalResources int                 `json:"total_resources"`
	Tree           []*ResourceTreeNode `json:"tree"`
}

// CMDBStats CMDB统计信息
type CMDBStats struct {
	TotalWorkspaces   int64              `json:"total_workspaces"`
	TotalResources    int64              `json:"total_resources"`
	TotalModules      int64              `json:"total_modules"`
	ResourceTypeStats []ResourceTypeStat `json:"resource_type_stats"`
	LastSyncedAt      *time.Time         `json:"last_synced_at,omitempty"`
}

// WorkspaceResourceCount workspace资源数量统计
type WorkspaceResourceCount struct {
	WorkspaceID   string     `json:"workspace_id" gorm:"column:workspace_id"`
	WorkspaceName string     `json:"workspace_name" gorm:"column:workspace_name"`
	ResourceCount int        `json:"resource_count" gorm:"column:resource_count"`
	LastSyncedAt  *time.Time `json:"last_synced_at,omitempty" gorm:"column:last_synced_at"`
}

// ResourceTypeStat 资源类型统计
type ResourceTypeStat struct {
	ResourceType string `json:"resource_type"`
	Count        int64  `json:"count"`
}
