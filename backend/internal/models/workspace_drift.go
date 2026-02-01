package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

// DriftCheckStatus Drift 检测状态枚举
type DriftCheckStatus string

const (
	DriftCheckStatusPending DriftCheckStatus = "pending"
	DriftCheckStatusRunning DriftCheckStatus = "running"
	DriftCheckStatusSuccess DriftCheckStatus = "success"
	DriftCheckStatusFailed  DriftCheckStatus = "failed"
	DriftCheckStatusSkipped DriftCheckStatus = "skipped"
)

// DriftDetails Drift 详情结构
type DriftDetails struct {
	CheckTime         string          `json:"check_time"`
	TerraformVersion  string          `json:"terraform_version"`
	PlanOutputSummary string          `json:"plan_output_summary"`
	Resources         []DriftResource `json:"resources"`
}

// DriftResource 资源 Drift 信息
type DriftResource struct {
	ResourceID      uint           `json:"resource_id"`
	ResourceName    string         `json:"resource_name"`
	ResourceType    string         `json:"resource_type"`
	HasDrift        bool           `json:"has_drift"`
	DriftedChildren []DriftedChild `json:"drifted_children"`
}

// DriftedChild 子资源 Drift 信息
type DriftedChild struct {
	Address string                       `json:"address"`
	Type    string                       `json:"type"`
	Name    string                       `json:"name"`
	Action  string                       `json:"action"` // create, update, delete, replace
	Changes map[string]DriftChangeDetail `json:"changes"`
}

// DriftChangeDetail 变更详情
type DriftChangeDetail struct {
	Before interface{} `json:"before"`
	After  interface{} `json:"after"`
}

// DriftDetailsJSON 自定义 JSONB 类型用于 DriftDetails
type DriftDetailsJSON DriftDetails

// Value 实现 driver.Valuer 接口
func (d DriftDetailsJSON) Value() (driver.Value, error) {
	return json.Marshal(d)
}

// Scan 实现 sql.Scanner 接口
func (d *DriftDetailsJSON) Scan(value interface{}) error {
	if value == nil {
		*d = DriftDetailsJSON{}
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, d)
}

// WorkspaceDriftResult Workspace Drift 检测结果模型
type WorkspaceDriftResult struct {
	ID             uint              `gorm:"primaryKey" json:"id"`
	WorkspaceID    string            `gorm:"type:varchar(50);not null;uniqueIndex" json:"workspace_id"`
	CurrentTaskID  *uint             `gorm:"index" json:"current_task_id,omitempty"` // 当前正在执行的 drift_check 任务 ID
	HasDrift       bool              `gorm:"default:false" json:"has_drift"`
	DriftCount     int               `gorm:"default:0" json:"drift_count"`
	TotalResources int               `gorm:"default:0" json:"total_resources"`
	DriftDetails   *DriftDetailsJSON `gorm:"type:jsonb" json:"drift_details,omitempty"`
	CheckStatus    DriftCheckStatus  `gorm:"type:varchar(20);default:pending;index" json:"check_status"`
	ErrorMessage   string            `gorm:"type:text" json:"error_message,omitempty"`
	LastCheckAt    *time.Time        `json:"last_check_at,omitempty"`
	LastCheckDate  *time.Time        `gorm:"type:date;index" json:"last_check_date,omitempty"`
	CreatedAt      time.Time         `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time         `gorm:"autoUpdateTime" json:"updated_at"`

	// 继续检测设置
	ContinueOnFailure bool `gorm:"default:false" json:"continue_on_failure"` // 失败后继续检测
	ContinueOnSuccess bool `gorm:"default:false" json:"continue_on_success"` // 成功后继续检测

	// 关联
	Workspace *Workspace `gorm:"foreignKey:WorkspaceID;references:WorkspaceID" json:"workspace,omitempty"`
}

// TableName 指定表名
func (WorkspaceDriftResult) TableName() string {
	return "workspace_drift_results"
}

// ResourceDriftStatus 资源 Drift 状态（用于 API 响应）
type ResourceDriftStatus struct {
	ResourceID           uint       `json:"resource_id"`
	ResourceName         string     `json:"resource_name"`
	Status               string     `json:"drift_status"` // drifted, synced, unapplied
	HasDrift             bool       `json:"has_drift"`
	LastAppliedAt        *time.Time `json:"last_applied_at,omitempty"`
	DriftedChildrenCount int        `json:"drifted_children_count"`
}

// DriftConfigResponse Drift 配置响应
type DriftConfigResponse struct {
	DriftCheckEnabled   bool   `json:"drift_check_enabled"`
	DriftCheckStartTime string `json:"drift_check_start_time"`
	DriftCheckEndTime   string `json:"drift_check_end_time"`
	DriftCheckInterval  int    `json:"drift_check_interval"`
	// 继续检测设置
	ContinueOnFailure bool `json:"continue_on_failure"` // 失败后继续检测
	ContinueOnSuccess bool `json:"continue_on_success"` // 成功后继续检测
}

// DriftConfigRequest Drift 配置请求
type DriftConfigRequest struct {
	DriftCheckEnabled   *bool   `json:"drift_check_enabled,omitempty"`
	DriftCheckStartTime *string `json:"drift_check_start_time,omitempty"`
	DriftCheckEndTime   *string `json:"drift_check_end_time,omitempty"`
	DriftCheckInterval  *int    `json:"drift_check_interval,omitempty"`
	// 继续检测设置
	ContinueOnFailure *bool `json:"continue_on_failure,omitempty"`
	ContinueOnSuccess *bool `json:"continue_on_success,omitempty"`
}

// DriftConfigUpdateRequest Drift 配置更新请求（非指针版本，用于完整更新）
type DriftConfigUpdateRequest struct {
	DriftCheckEnabled   bool   `json:"drift_check_enabled"`
	DriftCheckStartTime string `json:"drift_check_start_time"`
	DriftCheckEndTime   string `json:"drift_check_end_time"`
	DriftCheckInterval  int    `json:"drift_check_interval"`
	// 继续检测设置
	ContinueOnFailure bool `json:"continue_on_failure"`
	ContinueOnSuccess bool `json:"continue_on_success"`
}
