package models

import (
	"encoding/json"
	"fmt"
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
	// Varsets 指针:缺省(nil)与 [] 在首装时等价,都表示不挂 varset。
	Varsets           *[]DeploymentVarsetEntry `json:"varsets"`
	VariableOverrides OverrideInputs          `json:"variable_overrides"`
	// Workdir 可选:terraform 执行子目录(归一化后存入 workspaces.manifest_subpath)。
	// 省略(nil)= 沿用 workspace 记录里已有的 ManifestSubpath(向后兼容);
	// 非 nil(含空串)= 以本次值为准(空串 => 根目录)。
	Workdir *string `json:"workdir"`
}

// UpgradeDeploymentRequest upgrade 请求
type UpgradeDeploymentRequest struct {
	TargetVersionID string `json:"target_version_id" binding:"required"`
	// Varsets 指针,缺省 != []:缺省(nil)= 保持部署已挂的 varset 不变(也不算变量变更);
	// [] = 清空;非空 = 整体替换(按 priority 生效顺序比较是否变化)。
	Varsets *[]DeploymentVarsetEntry `json:"varsets"`
	// VariableOverrides 与已存覆盖合并(不再整体替换):缺省的 key 保留原值;
	// 敏感变量传空串(预览里的掩码占位)视为"不修改",也保留原值。
	VariableOverrides OverrideInputs `json:"variable_overrides"`
	// UnsetKeys 可选:只有列在这里的 key 才会从已存覆盖中删除。
	UnsetKeys []string `json:"unset_keys,omitempty"`
}

// FirstInstallPreviewRequest 首次安装前的变量预览请求(与 Install 同形的目标 + varsets/overrides)
type FirstInstallPreviewRequest struct {
	WorkspaceID       string                  `json:"workspace_id" binding:"required"` // ws-xxx 语义化ID
	VersionID         string                  `json:"version_id" binding:"required"`
	Varsets           []DeploymentVarsetEntry `json:"varsets"`
	VariableOverrides OverrideInputs          `json:"variable_overrides"`
}

// DeploymentPreviewRequest 已有 deployment 的变量预览:与 upgrade 同一合并
// (已存覆盖 + 本次覆盖 - unset_keys),可选 target_version_id 参与敏感判定。
type DeploymentPreviewRequest struct {
	TargetVersionID string `json:"target_version_id,omitempty"`
	// Varsets 与 upgrade 同义:缺省 = 部署已挂的 varset;[] = 无;非空 = 以此为准。
	Varsets           *[]DeploymentVarsetEntry `json:"varsets"`
	VariableOverrides OverrideInputs          `json:"variable_overrides"`
	UnsetKeys         []string                `json:"unset_keys,omitempty"`
}

// OverrideInput 单个覆盖:请求里可写成 "key": "value"(兼容旧格式)或
// "key": {"value": "...", "sensitive": true}。sensitive 一旦记录即粘滞,不能改回普通。
type OverrideInput struct {
	Value     string `json:"value"`
	Sensitive bool   `json:"sensitive,omitempty"`
}

// OverrideInputs variable_overrides 请求体(key -> OverrideInput)
type OverrideInputs map[string]OverrideInput

// UnmarshalJSON 接受字符串或 {value, sensitive} 对象
func (o *OverrideInputs) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return fmt.Errorf("variable_overrides must be an object: %w", err)
	}
	out := make(OverrideInputs, len(raw))
	for k, v := range raw {
		var str string
		if err := json.Unmarshal(v, &str); err == nil {
			out[k] = OverrideInput{Value: str}
			continue
		}
		var in OverrideInput
		if err := json.Unmarshal(v, &in); err != nil {
			return fmt.Errorf("variable_overrides[%q] must be a string or {value, sensitive}", k)
		}
		out[k] = in
	}
	*o = out
	return nil
}

// Values 扁平 key -> value
func (o OverrideInputs) Values() map[string]string {
	out := make(map[string]string, len(o))
	for k, v := range o {
		out[k] = v.Value
	}
	return out
}

// SensitiveFlags 请求里标记 sensitive 的 key
func (o OverrideInputs) SensitiveFlags() map[string]bool {
	out := map[string]bool{}
	for k, v := range o {
		if v.Sensitive {
			out[k] = true
		}
	}
	return out
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
