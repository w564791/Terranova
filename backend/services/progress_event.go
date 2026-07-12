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
	UsageLogID  string                 `json:"usage_log_id,omitempty"` // Skill 使用日志 ID（用于前端行为上报）

	// Manifest AI 完成时的数据
	HCL      string          `json:"hcl,omitempty"`      // manifest 资源生成结果（HCL 文本）
	Issues   []ManifestIssue `json:"issues,omitempty"`   // manifest check 结果（问题列表）
	Warnings []string        `json:"warnings,omitempty"` // 生成结果的 schema 校验警告

	// CMDB 搜索结果 AI 解读完成时的数据
	SearchSummary *SearchSummaryResult `json:"search_summary,omitempty"`

	// 错误时的数据
	Error string `json:"error,omitempty"` // 错误信息
}

// ManifestIssue manifest check 发现的单条问题
type ManifestIssue struct {
	File    string       `json:"file"`          // 文件路径（打包内容来源）
	Line    int          `json:"line"`          // 行号（从 1 开始，0 表示无法定位）
	Level   string       `json:"level"`         // 严重级别: error | warning | info
	Message string       `json:"message"`       // 问题描述
	Fix     *ManifestFix `json:"fix,omitempty"` // 可修复项才有：结构化修复
}

// ManifestFix AI 给出的结构化修复：按行范围替换目标文件内容
type ManifestFix struct {
	File      string `json:"file"`       // 目标文件路径
	StartLine int    `json:"start_line"` // 起始行（1-based，含）
	EndLine   int    `json:"end_line"`   // 结束行（1-based，含）
	NewText   string `json:"new_text"`   // 替换后的文本
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
