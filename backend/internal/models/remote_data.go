package models

import (
	"time"
)

// WorkspaceRemoteData 工作空间远程数据引用配置模型
type WorkspaceRemoteData struct {
	ID           uint   `json:"id" gorm:"primaryKey"`
	WorkspaceID  string `json:"workspace_id" gorm:"type:varchar(50);not null;index"`
	RemoteDataID string `json:"remote_data_id" gorm:"type:varchar(50);uniqueIndex"` // 语义化ID，如 rd-xxxx

	// 远程workspace配置
	SourceWorkspaceID string `json:"source_workspace_id" gorm:"type:varchar(50);not null;index"`
	DataName          string `json:"data_name" gorm:"type:varchar(200);not null"`
	Description       string `json:"description" gorm:"type:varchar(500)"`

	// 元数据
	CreatedBy *string   `json:"created_by" gorm:"type:varchar(50)"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// 关联
	Workspace       *Workspace `json:"workspace,omitempty" gorm:"foreignKey:WorkspaceID;references:WorkspaceID"`
	SourceWorkspace *Workspace `json:"source_workspace,omitempty" gorm:"foreignKey:SourceWorkspaceID;references:WorkspaceID"`
}

// TableName 指定表名
func (WorkspaceRemoteData) TableName() string {
	return "workspace_remote_data"
}

// RemoteDataToken 远程数据访问临时token模型
type RemoteDataToken struct {
	ID      uint   `json:"id" gorm:"primaryKey"`
	TokenID string `json:"token_id" gorm:"type:varchar(50);uniqueIndex"` // 语义化ID，如 rdt-xxxx
	Token   string `json:"token" gorm:"type:varchar(255);uniqueIndex"`   // 实际token值

	// 关联信息
	WorkspaceID          string `json:"workspace_id" gorm:"type:varchar(50);not null;index"`           // 被访问的workspace ID
	RequesterWorkspaceID string `json:"requester_workspace_id" gorm:"type:varchar(50);not null;index"` // 请求方workspace ID
	TaskID               *uint  `json:"task_id" gorm:"index"`                                          // 关联的任务ID（可选）

	// 使用限制
	MaxUses   int       `json:"max_uses" gorm:"default:5"`   // 最大使用次数
	UsedCount int       `json:"used_count" gorm:"default:0"` // 已使用次数
	ExpiresAt time.Time `json:"expires_at" gorm:"not null"`  // 过期时间

	// 元数据
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`

	// 关联
	Workspace          *Workspace `json:"workspace,omitempty" gorm:"foreignKey:WorkspaceID;references:WorkspaceID"`
	RequesterWorkspace *Workspace `json:"requester_workspace,omitempty" gorm:"foreignKey:RequesterWorkspaceID;references:WorkspaceID"`
}

// TableName 指定表名
func (RemoteDataToken) TableName() string {
	return "remote_data_tokens"
}

// IsValid 检查token是否有效
func (t *RemoteDataToken) IsValid() bool {
	// 检查是否过期
	if time.Now().After(t.ExpiresAt) {
		return false
	}
	// 检查使用次数
	if t.UsedCount >= t.MaxUses {
		return false
	}
	return true
}

// WorkspaceOutputsAccess 工作空间Outputs访问控制模型
type WorkspaceOutputsAccess struct {
	ID                 uint   `json:"id" gorm:"primaryKey"`
	WorkspaceID        string `json:"workspace_id" gorm:"type:varchar(50);not null;index"`         // 被访问的workspace ID
	AllowedWorkspaceID string `json:"allowed_workspace_id" gorm:"type:varchar(50);not null;index"` // 允许访问的workspace ID

	// 元数据
	CreatedBy *string   `json:"created_by" gorm:"type:varchar(50)"`
	CreatedAt time.Time `json:"created_at"`

	// 关联
	Workspace        *Workspace `json:"workspace,omitempty" gorm:"foreignKey:WorkspaceID;references:WorkspaceID"`
	AllowedWorkspace *Workspace `json:"allowed_workspace,omitempty" gorm:"foreignKey:AllowedWorkspaceID;references:WorkspaceID"`
}

// TableName 指定表名
func (WorkspaceOutputsAccess) TableName() string {
	return "workspace_outputs_access"
}

// OutputsSharingMode Outputs共享模式
type OutputsSharingMode string

const (
	OutputsSharingNone     OutputsSharingMode = "none"     // 不允许任何访问
	OutputsSharingAll      OutputsSharingMode = "all"      // 允许所有workspace访问
	OutputsSharingSpecific OutputsSharingMode = "specific" // 只允许指定的workspace访问
)

// RemoteDataInfo 远程数据信息（用于API响应）
type RemoteDataInfo struct {
	RemoteDataID        string `json:"remote_data_id"`
	WorkspaceID         string `json:"workspace_id"`
	SourceWorkspaceID   string `json:"source_workspace_id"`
	SourceWorkspaceName string `json:"source_workspace_name,omitempty"`
	DataName            string `json:"data_name"`
	Description         string `json:"description"`
	// 源workspace的outputs信息（用于前端提示）
	AvailableOutputs []OutputKeyInfo `json:"available_outputs,omitempty"`
}

// OutputKeyInfo Output键信息（用于前端自动提示）
type OutputKeyInfo struct {
	Key       string      `json:"key"`
	Type      string      `json:"type,omitempty"`
	Sensitive bool        `json:"sensitive"`
	Value     interface{} `json:"value,omitempty"` // 非sensitive的output会返回value
}
