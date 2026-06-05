package models

import (
	"time"
)

// ManifestFile 草稿与已发布版本快照统一存储
//
// 行级语义:
//   - 草稿区:    version_id IS NULL  AND owner_user_id 非空
//   - 发布快照: version_id 非空     AND owner_user_id IS NULL
//
// 部分唯一索引(在 migration 里):
//   uq_mf_draft     ON (manifest_id, owner_user_id, path) WHERE version_id IS NULL
//   uq_mf_published ON (manifest_id, version_id, path)    WHERE version_id IS NOT NULL
type ManifestFile struct {
	ID          int64     `json:"id" gorm:"primaryKey;column:id;autoIncrement"`
	ManifestID  string    `json:"manifest_id" gorm:"type:varchar(36);not null"`
	VersionID   *string   `json:"version_id,omitempty" gorm:"type:varchar(36)"`        // NULL = 草稿
	OwnerUserID *string   `json:"owner_user_id,omitempty" gorm:"type:varchar(20)"`     // 仅草稿行非空
	Path        string    `json:"path" gorm:"type:varchar(512);not null"`              // POSIX 风格,无前导斜杠
	Content     []byte    `json:"-" gorm:"type:bytea;not null"`                        // 原始字节,不在 JSON 输出(走单独读 API)
	Mime        string    `json:"mime" gorm:"type:varchar(128);not null;default:application/octet-stream"`
	Size        int       `json:"size" gorm:"not null"`
	IsBinary    bool      `json:"is_binary" gorm:"not null;default:false"`
	Mode        int       `json:"mode" gorm:"not null;default:420"`                    // 0644,本期未使用
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

func (ManifestFile) TableName() string {
	return "manifest_files"
}

// IsDraft 是否草稿行
func (mf *ManifestFile) IsDraft() bool {
	return mf.VersionID == nil
}

// ManifestDeploymentVarset deployment 关联的 varset(per-deployment,priority 数字大者优先级高)
type ManifestDeploymentVarset struct {
	ID           int64     `json:"id" gorm:"primaryKey;column:id;autoIncrement"`
	DeploymentID string    `json:"deployment_id" gorm:"type:varchar(36);not null;index"`
	VarsetID     string    `json:"varset_id" gorm:"type:varchar(30);not null;index"`
	Priority     int       `json:"priority" gorm:"not null;default:0"` // 数字大者优先级高
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime"`
}

func (ManifestDeploymentVarset) TableName() string {
	return "manifest_deployment_varsets"
}

// ManifestFileTreeEntry 文件树节点(API 响应)
type ManifestFileTreeEntry struct {
	Path     string `json:"path"`     // 完整路径
	Name     string `json:"name"`     // 文件名
	Type     string `json:"type"`     // file | dir
	Size     int    `json:"size"`     // 字节数
	Mime     string `json:"mime"`     // MIME 类型
	IsBinary bool   `json:"is_binary"`
}

// PublishVersionRequest 发布版本请求
type PublishVersionRequest struct {
	Version   string `json:"version" binding:"required"`   // vMAJOR.MINOR.PATCH
	Changelog string `json:"changelog"`
}

// InstallDeploymentRequest install 请求
type InstallDeploymentRequest struct {
	VersionID         string                  `json:"version_id" binding:"required"`
	WorkspaceID       string                  `json:"workspace_id" binding:"required"` // ws-xxx 语义化ID
	Varsets           []DeploymentVarsetEntry `json:"varsets"`
	VariableOverrides map[string]string       `json:"variable_overrides"`
	// Workdir 可选:terraform 执行子目录(归一化后存入 workspaces.manifest_subpath)。
	// 省略(nil)= 沿用 workspace 记录里已有的 ManifestSubpath(向后兼容);
	// 非 nil(含空串)= 以本次值为准(空串 => 根目录)。
	Workdir *string `json:"workdir"`
}

// UpgradeDeploymentRequest upgrade 请求
type UpgradeDeploymentRequest struct {
	TargetVersionID   string                  `json:"target_version_id" binding:"required"`
	Varsets           []DeploymentVarsetEntry `json:"varsets"`
	VariableOverrides map[string]string       `json:"variable_overrides"`
}

// DeploymentVarsetEntry 部署对话框选中的 varset 条目
type DeploymentVarsetEntry struct {
	VarsetID string `json:"varset_id" binding:"required"`
	Priority int    `json:"priority"`
}

// ManifestExternalFile 用于 workspace plan-only 的临时文件注入(Run 按钮)
//
// executor 看到 ExternalFiles 非空时走"Run 分支":完全忽略
// workspace.ManifestDeploymentID,只用 ExternalFiles 落临时目录跑 plan。
type ManifestExternalFile struct {
	Path       string `json:"path"`
	ContentB64 string `json:"content_b64"`
}
