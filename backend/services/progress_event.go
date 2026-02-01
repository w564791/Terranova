package services

// CompletedStep 已完成的步骤信息
type CompletedStep struct {
	Name       string   `json:"name"`                  // 步骤名称
	ElapsedMs  int64    `json:"elapsed_ms"`            // 该步骤耗时（毫秒）
	UsedSkills []string `json:"used_skills,omitempty"` // 该步骤使用的 Skills（可选）
}

// ProgressEvent 进度事件（用于 SSE 实时推送）
type ProgressEvent struct {
	Type       string `json:"type"`        // 事件类型: "progress" | "complete" | "error" | "need_selection"
	Step       int    `json:"step"`        // 当前步骤（从 1 开始）
	TotalSteps int    `json:"total_steps"` // 总步骤数
	StepName   string `json:"step_name"`   // 步骤名称（中文）
	Message    string `json:"message"`     // 详细消息（可选）
	ElapsedMs  int64  `json:"elapsed_ms"`  // 已耗时（毫秒）

	// 已完成的步骤列表（用于横向进度显示）
	CompletedSteps []CompletedStep `json:"completed_steps,omitempty"`

	// 完成时的数据
	Config      map[string]interface{} `json:"config,omitempty"`       // 生成的配置
	CMDBLookups []CMDBLookupResult     `json:"cmdb_lookups,omitempty"` // CMDB 查询结果

	// 错误时的数据
	Error string `json:"error,omitempty"` // 错误信息
}

// ProgressCallback 进度回调函数类型
type ProgressCallback func(event ProgressEvent)

// ProgressReporter 进度报告器接口
// 用于抽象进度推送方式，便于未来扩展（如 Pipeline 方案使用数据库存储）
type ProgressReporter interface {
	// ReportProgress 报告进度
	ReportProgress(event ProgressEvent)
}

// NilProgressReporter 空进度报告器（不推送进度）
type NilProgressReporter struct{}

func (r *NilProgressReporter) ReportProgress(event ProgressEvent) {
	// 不做任何事情
}

// CallbackProgressReporter 回调进度报告器
type CallbackProgressReporter struct {
	callback ProgressCallback
}

func NewCallbackProgressReporter(callback ProgressCallback) *CallbackProgressReporter {
	return &CallbackProgressReporter{callback: callback}
}

func (r *CallbackProgressReporter) ReportProgress(event ProgressEvent) {
	if r.callback != nil {
		r.callback(event)
	}
}
