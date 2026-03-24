package models

import (
	"encoding/json"
	"time"
)

// AIPlanSummary Plan 阶段变更影响分析结果（不可变）
type AIPlanSummary struct {
	ID                string          `gorm:"primaryKey;type:varchar(30)" json:"id"`                        // 语义化 ID: plsm-xxx
	TaskID            uint            `gorm:"not null;uniqueIndex" json:"task_id"`                          // 关联任务
	WorkspaceID       string          `gorm:"type:varchar(50);not null;index" json:"workspace_id"`          // 关联工作空间
	ChangesOverview   string          `gorm:"type:text" json:"changes_overview"`                            // AI 生成的变更概述
	ImpactAnalysis    json.RawMessage `gorm:"type:jsonb" json:"impact_analysis"`                            // 依赖影响分析结果
	AffectedResources json.RawMessage `gorm:"type:jsonb" json:"affected_resources"`                         // 被影响的依赖资源列表
	RiskLevel         string          `gorm:"type:varchar(20)" json:"risk_level"`                           // low/medium/high/critical
	ModuleContext     json.RawMessage `gorm:"type:jsonb" json:"module_context"`                             // 补全后的完整 module 资源快照
	PlanChanges       json.RawMessage `gorm:"type:jsonb" json:"plan_changes"`                               // 当次 plan 的资源变更快照
	CMDBLookups       json.RawMessage `gorm:"type:jsonb" json:"cmdb_lookups"`                               // CMDB 查询审计记录
	ToolCalls         json.RawMessage `gorm:"type:jsonb" json:"tool_calls"`                                 // Agent Loop 所有工具调用记录
	Status            string          `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`    // pending/running/completed/failed
	ErrorMessage      string          `gorm:"type:text" json:"error_message,omitempty"`                     // 失败原因
	Duration          int             `json:"duration"`                                                      // 分析耗时（毫秒）
	CreatedAt         time.Time       `json:"created_at"`                                                    // 创建时间（本地时间）
	// 人机协同决策字段
	RequiresConfirmation bool            `gorm:"default:false" json:"requires_confirmation"`                    // AI 判断是否需要人工确认
	DecisionScenario     string          `gorm:"type:varchar(50)" json:"decision_scenario,omitempty"`           // 决策场景码（V3 旧字段，保留兼容）
	DecisionTitle        string          `gorm:"type:text" json:"decision_title,omitempty"`                     // AI 生成的风险确认标题
	RiskHighlights       json.RawMessage `gorm:"type:jsonb" json:"risk_highlights,omitempty"`                   // AI 生成的关键风险点 ["..."]
	DecisionActions      json.RawMessage `gorm:"type:jsonb" json:"decision_actions,omitempty"`                  // 可选决策项 [{code, label}]
	UserDecisionCode     string          `gorm:"type:text" json:"user_decision_code,omitempty"`                 // 用户选择的决策码（多选时逗号分隔）
	UserDecisionNote     string          `gorm:"type:text" json:"user_decision_note,omitempty"`                 // 用户补充说明
	UserDecisionBy       string          `gorm:"type:varchar(20)" json:"user_decision_by,omitempty"`            // 确认人 user_id
	UserDecisionAt       *time.Time      `json:"user_decision_at,omitempty"`                                    // 确认时间
}

// TableName 指定表名
func (AIPlanSummary) TableName() string {
	return "ai_plan_summaries"
}

// AIApplySummary Apply 阶段执行结果总结（不可变）
type AIApplySummary struct {
	ID                  string          `gorm:"primaryKey;type:varchar(30)" json:"id"`                        // 语义化 ID: apsm-xxx
	TaskID              uint            `gorm:"not null;uniqueIndex" json:"task_id"`                          // 关联任务
	WorkspaceID         string          `gorm:"type:varchar(50);not null;index" json:"workspace_id"`          // 关联工作空间
	ExecutionSummary    string          `gorm:"type:text" json:"execution_summary"`                           // AI 生成的执行结果总结
	ResourceResults     json.RawMessage `gorm:"type:jsonb" json:"resource_results"`                           // 每个资源的最终状态
	ImpactConfirmation  json.RawMessage `gorm:"type:jsonb" json:"impact_confirmation"`                        // 实际影响 vs plan 阶段预测对比
	AffectedResources   json.RawMessage `gorm:"type:jsonb" json:"affected_resources"`                         // 实际被影响的依赖资源
	ApplyChanges        json.RawMessage `gorm:"type:jsonb" json:"apply_changes"`                              // apply 实际变更快照
	CMDBLookups         json.RawMessage `gorm:"type:jsonb" json:"cmdb_lookups"`                               // CMDB 查询审计记录
	ToolCalls           json.RawMessage `gorm:"type:jsonb" json:"tool_calls"`                                 // Agent Loop 所有工具调用记录
	Status              string          `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`    // pending/running/completed/failed
	ErrorMessage        string          `gorm:"type:text" json:"error_message,omitempty"`                     // 失败原因
	Duration            int             `json:"duration"`                                                      // 分析耗时（毫秒）
	CreatedAt           time.Time       `json:"created_at"`                                                    // 创建时间（本地时间）
}

// TableName 指定表名
func (AIApplySummary) TableName() string {
	return "ai_apply_summaries"
}
