package models

import (
	"time"
)

// StaticOutputResourceName 静态输出的特殊资源名称标识
const StaticOutputResourceName = "__static__"

// WorkspaceOutput 工作空间Output配置模型
type WorkspaceOutput struct {
	ID          uint   `json:"id" gorm:"primaryKey"`
	WorkspaceID string `json:"workspace_id" gorm:"type:varchar(50);not null;index"`
	OutputID    string `json:"output_id" gorm:"type:varchar(50);uniqueIndex"`

	// 资源关联（为空或"__static__"表示静态输出）
	ResourceName string `json:"resource_name" gorm:"type:varchar(200);index"`

	// Output配置
	OutputName  string `json:"output_name" gorm:"type:varchar(200);not null"`
	OutputValue string `json:"output_value" gorm:"type:text;not null"` // 改为text以支持更长的静态值
	Description string `json:"description" gorm:"type:varchar(500)"`
	Sensitive   bool   `json:"sensitive" gorm:"default:false"`

	// 元数据
	CreatedBy *string   `json:"created_by" gorm:"type:varchar(50)"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// 关联
	Workspace *Workspace `json:"workspace,omitempty" gorm:"foreignKey:WorkspaceID;references:WorkspaceID"`
}

// IsStaticOutput 判断是否为静态输出
func (o *WorkspaceOutput) IsStaticOutput() bool {
	return o.ResourceName == "" || o.ResourceName == StaticOutputResourceName
}

// TableName 指定表名
func (WorkspaceOutput) TableName() string {
	return "workspace_outputs"
}

// StateOutputInfo 从State中提取的Output信息
type StateOutputInfo struct {
	CheckResults     interface{}            `json:"check_results"`
	Lineage          string                 `json:"lineage"`
	Outputs          map[string]OutputValue `json:"outputs"`
	Resources        []interface{}          `json:"resources"` // 清空返回
	Serial           int                    `json:"serial"`
	TerraformVersion string                 `json:"terraform_version"`
	Version          int                    `json:"version"`
}

// OutputValue Output值结构
type OutputValue struct {
	Value     interface{} `json:"value"`
	Type      interface{} `json:"type,omitempty"`
	Sensitive bool        `json:"sensitive,omitempty"`
}
