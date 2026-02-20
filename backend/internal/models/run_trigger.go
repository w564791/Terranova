package models

import (
	"time"
)

// RunTrigger 工作空间触发配置
// 定义当源 workspace 的任务完成后，触发目标 workspace 的 plan+apply 任务
type RunTrigger struct {
	ID                uint      `json:"id" gorm:"primaryKey"`
	SourceWorkspaceID string    `json:"source_workspace_id" gorm:"type:varchar(50);not null;index"`
	TargetWorkspaceID string    `json:"target_workspace_id" gorm:"type:varchar(50);not null;index"`
	Enabled           bool      `json:"enabled" gorm:"default:true"`
	TriggerCondition  string    `json:"trigger_condition" gorm:"type:varchar(50);default:apply_success"` // apply_success
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	CreatedBy         *string   `json:"created_by" gorm:"type:varchar(50)"`

	// 关联
	SourceWorkspace *Workspace `json:"source_workspace,omitempty" gorm:"foreignKey:SourceWorkspaceID;references:WorkspaceID"`
	TargetWorkspace *Workspace `json:"target_workspace,omitempty" gorm:"foreignKey:TargetWorkspaceID;references:WorkspaceID"`
}

// TableName 指定表名
func (RunTrigger) TableName() string {
	return "run_triggers"
}

// TaskTriggerExecution 任务触发执行记录
// 记录每次任务触发的执行情况
type TaskTriggerExecution struct {
	ID                  uint       `json:"id" gorm:"primaryKey"`
	SourceTaskID        uint       `json:"source_task_id" gorm:"not null;index"`
	RunTriggerID        uint       `json:"run_trigger_id" gorm:"not null;index"`
	TargetTaskID        *uint      `json:"target_task_id" gorm:"index"`
	Status              string     `json:"status" gorm:"type:varchar(20);default:pending;index"` // pending, triggered, skipped, failed
	TemporarilyDisabled bool       `json:"temporarily_disabled" gorm:"default:false"`
	DisabledBy          *string    `json:"disabled_by" gorm:"type:varchar(50)"`
	DisabledAt          *time.Time `json:"disabled_at"`
	ErrorMessage        string     `json:"error_message" gorm:"type:text"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`

	// 关联
	SourceTask *WorkspaceTask `json:"source_task,omitempty" gorm:"foreignKey:SourceTaskID"`
	RunTrigger *RunTrigger    `json:"run_trigger,omitempty" gorm:"foreignKey:RunTriggerID"`
	TargetTask *WorkspaceTask `json:"target_task,omitempty" gorm:"foreignKey:TargetTaskID"`
}

// TableName 指定表名
func (TaskTriggerExecution) TableName() string {
	return "task_trigger_executions"
}

// TaskTriggerExecutionStatus 触发执行状态常量
const (
	TriggerStatusPending   = "pending"   // 等待触发
	TriggerStatusTriggered = "triggered" // 已触发
	TriggerStatusSkipped   = "skipped"   // 已跳过（被临时禁用）
	TriggerStatusFailed    = "failed"    // 触发失败
)

// TriggerCondition 触发条件常量
const (
	TriggerConditionApplySuccess = "apply_success" // Apply 成功后触发
)
