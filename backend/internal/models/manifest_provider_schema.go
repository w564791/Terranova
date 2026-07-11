package models

import (
	"encoding/json"
	"time"
)

// ManifestProviderSchema 在 execute post_init 后按 (manifest, subpath) 缓存的 provider 类型目录。
//
// 主键语义：同一份 Manifest 源码 + 同一执行子目录共享一行；多 workspace 部署共享。
// ProviderVersionsKey 由 lock 中 source@version 规范化后生成，版本未变则跳过 schema 提取与写库。
type ManifestProviderSchema struct {
	ID                   string          `json:"id" gorm:"primaryKey;size:40"`
	ManifestID           string          `json:"manifest_id" gorm:"type:varchar(36);not null;uniqueIndex:uq_mps_manifest_subpath_kind,priority:1"`
	Subpath              string          `json:"subpath" gorm:"type:varchar(512);not null;default:'';uniqueIndex:uq_mps_manifest_subpath_kind,priority:2"` // 根目录 ""
	SchemaKind           string          `json:"schema_kind" gorm:"type:varchar(16);not null;default:types;uniqueIndex:uq_mps_manifest_subpath_kind,priority:3"`
	Providers            json.RawMessage `json:"providers" gorm:"type:jsonb;not null"` // [{source,version},...]
	ProviderVersionsKey  string          `json:"provider_versions_key" gorm:"type:varchar(128);not null;index"` // 规范化指纹，快速比对
	Resources            json.RawMessage `json:"resources" gorm:"type:jsonb"`                                   // []string type names
	DataSources          json.RawMessage `json:"data_sources" gorm:"type:jsonb"`                                // []string
	ContentHash          string          `json:"content_hash" gorm:"type:varchar(64)"`
	TerraformVersion     string          `json:"terraform_version" gorm:"type:varchar(32)"`
	SourceWorkspaceID    string          `json:"source_workspace_id" gorm:"type:varchar(50)"`
	SourceTaskID         *uint           `json:"source_task_id"`
	CapturedAt           time.Time       `json:"captured_at"`
	CreatedAt            time.Time       `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt            time.Time       `json:"updated_at" gorm:"autoUpdateTime"`
}

func (ManifestProviderSchema) TableName() string {
	return "manifest_provider_schemas"
}

// ProviderVersionRef 单个 provider 坐标 + 解析版本（来自 .terraform.lock.hcl）
type ProviderVersionRef struct {
	Source  string `json:"source"`
	Version string `json:"version"`
}

const ManifestProviderSchemaKindTypes = "types"
