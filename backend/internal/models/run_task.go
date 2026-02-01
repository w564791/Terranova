package models

import (
	"time"
)

// RunTaskStage 执行阶段
type RunTaskStage string

const (
	RunTaskStagePrePlan   RunTaskStage = "pre_plan"
	RunTaskStagePostPlan  RunTaskStage = "post_plan"
	RunTaskStagePreApply  RunTaskStage = "pre_apply"
	RunTaskStagePostApply RunTaskStage = "post_apply"
)

// RunTaskEnforcementLevel 执行级别
type RunTaskEnforcementLevel string

const (
	RunTaskEnforcementAdvisory  RunTaskEnforcementLevel = "advisory"
	RunTaskEnforcementMandatory RunTaskEnforcementLevel = "mandatory"
)

// RunTaskResultStatus 执行结果状态
type RunTaskResultStatus string

const (
	RunTaskResultPending    RunTaskResultStatus = "pending"
	RunTaskResultRunning    RunTaskResultStatus = "running"
	RunTaskResultPassed     RunTaskResultStatus = "passed"
	RunTaskResultFailed     RunTaskResultStatus = "failed"
	RunTaskResultError      RunTaskResultStatus = "error"
	RunTaskResultTimeout    RunTaskResultStatus = "timeout"
	RunTaskResultSkipped    RunTaskResultStatus = "skipped"
	RunTaskResultOverridden RunTaskResultStatus = "overridden" // 被用户手动覆盖
)

// RunTask 全局 Run Task 定义
// 在组织/团队级别定义的外部服务集成，包含名称、Endpoint URL、HMAC密钥等配置
type RunTask struct {
	// 基础字段
	ID          uint      `json:"id" gorm:"primaryKey"`
	RunTaskID   string    `json:"run_task_id" gorm:"column:run_task_id;type:varchar(50);uniqueIndex"` // 语义化ID，如 "rt-security-scan"
	Name        string    `json:"name" gorm:"type:varchar(100);not null"`                             // 名称，只能包含字母、数字、破折号和下划线
	Description string    `json:"description" gorm:"type:text"`                                       // 描述（可选）
	EndpointURL string    `json:"endpoint_url" gorm:"type:varchar(500);not null"`                     // Endpoint URL，Run Tasks 会 POST 到这个 URL
	Enabled     bool      `json:"enabled" gorm:"default:true"`                                        // 是否启用
	CreatedBy   *string   `json:"created_by" gorm:"type:varchar(50)"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// HMAC密钥（加密存储，不返回给前端）
	HMACKeyEncrypted string `json:"-" gorm:"column:hmac_key_encrypted;type:text"` // HMAC密钥（AES-256加密存储）

	// 超时配置（符合 TFE 规范）
	TimeoutSeconds int `json:"timeout_seconds" gorm:"default:600"`  // 进度更新超时（秒），默认10分钟，范围60-600
	MaxRunSeconds  int `json:"max_run_seconds" gorm:"default:3600"` // 最大运行时间（秒），默认60分钟，范围60-3600

	// 全局任务配置
	IsGlobal bool `json:"is_global" gorm:"default:false"` // 是否为全局任务（自动应用于所有 Workspace）

	// 全局任务默认配置（仅当 IsGlobal=true 时有效）
	GlobalStages           string                  `json:"global_stages" gorm:"type:varchar(100);default:'post_plan'"`        // 全局任务默认执行阶段，逗号分隔，如 "post_plan,pre_apply"
	GlobalEnforcementLevel RunTaskEnforcementLevel `json:"global_enforcement_level" gorm:"type:varchar(20);default:advisory"` // 全局任务默认执行级别

	// 组织/团队归属
	OrganizationID *string `json:"organization_id" gorm:"type:varchar(50);index"` // 组织ID（可选）
	TeamID         *string `json:"team_id" gorm:"type:varchar(50);index"`         // 团队ID（可选）
}

// TableName 指定表名
func (RunTask) TableName() string {
	return "run_tasks"
}

// RunTaskResponse Run Task API 响应结构（用于隐藏敏感字段并添加计算字段）
type RunTaskResponse struct {
	ID                     uint                    `json:"id"`
	RunTaskID              string                  `json:"run_task_id"`
	Name                   string                  `json:"name"`
	Description            string                  `json:"description"`
	EndpointURL            string                  `json:"endpoint_url"`
	HMACKeySet             bool                    `json:"hmac_key_set"` // 是否设置了HMAC密钥
	Enabled                bool                    `json:"enabled"`
	TimeoutSeconds         int                     `json:"timeout_seconds"`
	MaxRunSeconds          int                     `json:"max_run_seconds"`
	IsGlobal               bool                    `json:"is_global"`
	GlobalStages           string                  `json:"global_stages,omitempty"`            // 全局任务默认执行阶段
	GlobalEnforcementLevel RunTaskEnforcementLevel `json:"global_enforcement_level,omitempty"` // 全局任务默认执行级别
	OrganizationID         *string                 `json:"organization_id"`
	TeamID                 *string                 `json:"team_id"`
	WorkspaceCount         int                     `json:"workspace_count"` // 关联的 Workspace 数量
	CreatedBy              *string                 `json:"created_by"`
	CreatedAt              time.Time               `json:"created_at"`
	UpdatedAt              time.Time               `json:"updated_at"`
}

// ToResponse 将 RunTask 转换为 API 响应结构
func (r *RunTask) ToResponse(workspaceCount int) RunTaskResponse {
	return RunTaskResponse{
		ID:                     r.ID,
		RunTaskID:              r.RunTaskID,
		Name:                   r.Name,
		Description:            r.Description,
		EndpointURL:            r.EndpointURL,
		HMACKeySet:             r.HMACKeyEncrypted != "",
		Enabled:                r.Enabled,
		TimeoutSeconds:         r.TimeoutSeconds,
		MaxRunSeconds:          r.MaxRunSeconds,
		IsGlobal:               r.IsGlobal,
		GlobalStages:           r.GlobalStages,
		GlobalEnforcementLevel: r.GlobalEnforcementLevel,
		OrganizationID:         r.OrganizationID,
		TeamID:                 r.TeamID,
		WorkspaceCount:         workspaceCount,
		CreatedBy:              r.CreatedBy,
		CreatedAt:              r.CreatedAt,
		UpdatedAt:              r.UpdatedAt,
	}
}

// WorkspaceRunTask Workspace 关联的 Run Task
// 将全局 Run Task 应用到特定 Workspace，配置执行阶段和执行级别
type WorkspaceRunTask struct {
	// 基础字段
	ID                 uint      `json:"id" gorm:"primaryKey"`
	WorkspaceRunTaskID string    `json:"workspace_run_task_id" gorm:"column:workspace_run_task_id;type:varchar(50);uniqueIndex"` // 语义化ID
	WorkspaceID        string    `json:"workspace_id" gorm:"type:varchar(50);not null;index"`                                    // 关联的 Workspace ID
	RunTaskID          string    `json:"run_task_id" gorm:"type:varchar(50);not null;index"`                                     // 关联的 Run Task ID
	CreatedBy          *string   `json:"created_by" gorm:"type:varchar(50)"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`

	// 执行配置
	Stage            RunTaskStage            `json:"stage" gorm:"type:varchar(20);not null"`                     // 执行阶段: pre_plan, post_plan, pre_apply, post_apply
	EnforcementLevel RunTaskEnforcementLevel `json:"enforcement_level" gorm:"type:varchar(20);default:advisory"` // 执行级别: advisory, mandatory

	// 状态
	Enabled bool `json:"enabled" gorm:"default:true"`

	// 关联
	RunTask   *RunTask   `json:"run_task,omitempty" gorm:"foreignKey:RunTaskID;references:RunTaskID"`
	Workspace *Workspace `json:"workspace,omitempty" gorm:"foreignKey:WorkspaceID;references:WorkspaceID"`
}

// TableName 指定表名
func (WorkspaceRunTask) TableName() string {
	return "workspace_run_tasks"
}

// WorkspaceRunTaskResponse Workspace Run Task API 响应结构
type WorkspaceRunTaskResponse struct {
	ID                 uint                    `json:"id"`
	WorkspaceRunTaskID string                  `json:"workspace_run_task_id"`
	WorkspaceID        string                  `json:"workspace_id"`
	RunTaskID          string                  `json:"run_task_id"`
	RunTaskName        string                  `json:"run_task_name"`        // Run Task 名称
	RunTaskDescription string                  `json:"run_task_description"` // Run Task 描述
	Stage              RunTaskStage            `json:"stage"`
	EnforcementLevel   RunTaskEnforcementLevel `json:"enforcement_level"`
	Enabled            bool                    `json:"enabled"`
	CreatedBy          *string                 `json:"created_by"`
	CreatedAt          time.Time               `json:"created_at"`
	UpdatedAt          time.Time               `json:"updated_at"`
}

// ToResponse 将 WorkspaceRunTask 转换为 API 响应结构
func (w *WorkspaceRunTask) ToResponse() WorkspaceRunTaskResponse {
	resp := WorkspaceRunTaskResponse{
		ID:                 w.ID,
		WorkspaceRunTaskID: w.WorkspaceRunTaskID,
		WorkspaceID:        w.WorkspaceID,
		RunTaskID:          w.RunTaskID,
		Stage:              w.Stage,
		EnforcementLevel:   w.EnforcementLevel,
		Enabled:            w.Enabled,
		CreatedBy:          w.CreatedBy,
		CreatedAt:          w.CreatedAt,
		UpdatedAt:          w.UpdatedAt,
	}
	if w.RunTask != nil {
		resp.RunTaskName = w.RunTask.Name
		resp.RunTaskDescription = w.RunTask.Description
	}
	return resp
}

// RunTaskResult Run Task 执行结果
// 存储每次 Run Task 调用的结果
type RunTaskResult struct {
	// 基础字段
	ID        uint      `json:"id" gorm:"primaryKey"`
	ResultID  string    `json:"result_id" gorm:"column:result_id;type:varchar(50);uniqueIndex"` // 语义化ID
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// 关联
	TaskID             uint    `json:"task_id" gorm:"not null;index"`                       // 关联的 workspace_task ID
	WorkspaceRunTaskID *string `json:"workspace_run_task_id" gorm:"type:varchar(50);index"` // 关联的 workspace_run_task ID（Workspace 级别 Run Task）
	RunTaskID          *string `json:"run_task_id" gorm:"type:varchar(50);index"`           // 关联的 run_task ID（全局 Run Task）

	// 执行信息
	Stage  RunTaskStage        `json:"stage" gorm:"type:varchar(20);not null"`         // 执行阶段
	Status RunTaskResultStatus `json:"status" gorm:"type:varchar(20);default:pending"` // 状态

	// 一次性 Access Token（用于 Run Task 平台获取数据和回调）
	AccessToken          string     `json:"-" gorm:"type:varchar(500)"`              // 一次性验证令牌（JWT格式，不返回给前端）
	AccessTokenExpiresAt *time.Time `json:"-" gorm:"column:access_token_expires_at"` // Token过期时间
	AccessTokenUsed      bool       `json:"-" gorm:"default:false"`                  // Token是否已使用

	// 请求/响应
	RequestPayload  JSONB  `json:"request_payload" gorm:"type:jsonb"`     // 发送给外部服务的请求
	ResponsePayload JSONB  `json:"response_payload" gorm:"type:jsonb"`    // 外部服务的响应（回调数据）
	CallbackURL     string `json:"callback_url" gorm:"type:varchar(500)"` // 回调URL

	// 结果详情
	Message string `json:"message" gorm:"type:text"`     // 结果消息
	URL     string `json:"url" gorm:"type:varchar(500)"` // 详情链接（外部服务提供）

	// 超时配置（符合 TFE 规范）
	TimeoutSeconds  int        `json:"timeout_seconds" gorm:"default:600"`  // 进度更新超时（秒）
	MaxRunSeconds   int        `json:"max_run_seconds" gorm:"default:3600"` // 最大运行时间（秒）
	LastHeartbeatAt *time.Time `json:"last_heartbeat_at"`                   // 最后一次进度更新时间
	TimeoutAt       *time.Time `json:"timeout_at" gorm:"index"`             // 进度更新超时时间点
	MaxRunTimeoutAt *time.Time `json:"max_run_timeout_at" gorm:"index"`     // 最大运行超时时间点

	// 时间
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`

	// Override 相关字段（仅对 Advisory 类型有效）
	IsOverridden bool       `json:"is_overridden" gorm:"default:false"` // 是否已被 Override
	OverrideBy   *string    `json:"override_by" gorm:"type:varchar(50)"`
	OverrideAt   *time.Time `json:"override_at"`

	// 关联
	Task             *WorkspaceTask    `json:"task,omitempty" gorm:"foreignKey:TaskID"`
	WorkspaceRunTask *WorkspaceRunTask `json:"workspace_run_task,omitempty" gorm:"foreignKey:WorkspaceRunTaskID;references:WorkspaceRunTaskID"`
	Outcomes         []RunTaskOutcome  `json:"outcomes,omitempty" gorm:"foreignKey:RunTaskResultID;references:ResultID"`
}

// TableName 指定表名
func (RunTaskResult) TableName() string {
	return "run_task_results"
}

// RunTaskResultResponse Run Task Result API 响应结构
type RunTaskResultResponse struct {
	ID                 uint                    `json:"id"`
	ResultID           string                  `json:"result_id"`
	TaskID             uint                    `json:"task_id"`
	WorkspaceRunTaskID string                  `json:"workspace_run_task_id,omitempty"`
	RunTaskID          string                  `json:"run_task_id,omitempty"`
	RunTaskName        string                  `json:"run_task_name"`
	Stage              RunTaskStage            `json:"stage"`
	EnforcementLevel   RunTaskEnforcementLevel `json:"enforcement_level"`
	Status             RunTaskResultStatus     `json:"status"`
	Message            string                  `json:"message"`
	URL                string                  `json:"url"`
	StartedAt          *time.Time              `json:"started_at"`
	CompletedAt        *time.Time              `json:"completed_at"`
	CreatedAt          time.Time               `json:"created_at"`
	Outcomes           []RunTaskOutcome        `json:"outcomes,omitempty"`
	IsGlobal           bool                    `json:"is_global"` // 是否为全局 Run Task
}

// ToResponse 将 RunTaskResult 转换为 API 响应结构
func (r *RunTaskResult) ToResponse() RunTaskResultResponse {
	resp := RunTaskResultResponse{
		ID:          r.ID,
		ResultID:    r.ResultID,
		TaskID:      r.TaskID,
		Stage:       r.Stage,
		Status:      r.Status,
		Message:     r.Message,
		URL:         r.URL,
		StartedAt:   r.StartedAt,
		CompletedAt: r.CompletedAt,
		CreatedAt:   r.CreatedAt,
		Outcomes:    r.Outcomes,
	}

	// 处理 WorkspaceRunTaskID（可能为 nil）
	if r.WorkspaceRunTaskID != nil {
		resp.WorkspaceRunTaskID = *r.WorkspaceRunTaskID
	}

	// 处理 RunTaskID（可能为 nil）
	if r.RunTaskID != nil {
		resp.RunTaskID = *r.RunTaskID
		resp.IsGlobal = true
	}

	if r.WorkspaceRunTask != nil {
		resp.RunTaskID = r.WorkspaceRunTask.RunTaskID
		resp.EnforcementLevel = r.WorkspaceRunTask.EnforcementLevel
		if r.WorkspaceRunTask.RunTask != nil {
			resp.RunTaskName = r.WorkspaceRunTask.RunTask.Name
		}
	}
	return resp
}

// RunTaskOutcome Run Task Outcome（详细检查结果，符合 TFE 规范）
type RunTaskOutcome struct {
	// 基础字段
	ID        uint      `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"created_at"`

	// 关联
	RunTaskResultID string `json:"run_task_result_id" gorm:"type:varchar(50);not null;index"` // 关联的 run_task_result ID

	// Outcome 标识（第三方服务提供）
	OutcomeID string `json:"outcome_id" gorm:"type:varchar(100);not null"` // 第三方服务提供的唯一标识，如 "PRTNR-CC-TF-127"

	// 描述
	Description string `json:"description" gorm:"type:varchar(500);not null"` // 一行描述
	Body        string `json:"body" gorm:"type:text"`                         // Markdown 格式的详细内容（建议 < 1MB，最大 5MB）
	URL         string `json:"url" gorm:"type:varchar(500)"`                  // 详情链接

	// 标签（JSON 格式，支持 severity 和 status 特殊处理）
	// 格式: {"Status": [{"label": "Failed", "level": "error"}], "Severity": [...]}
	// level 可选值: none(默认), info(蓝色), warning(黄色), error(红色)
	Tags JSONB `json:"tags" gorm:"type:jsonb"`
}

// TableName 指定表名
func (RunTaskOutcome) TableName() string {
	return "run_task_outcomes"
}

// RunTaskWebhookPayload Run Task Webhook 请求体（发送给第三方服务）
type RunTaskWebhookPayload struct {
	PayloadVersion int    `json:"payload_version"` // 固定为 1
	Stage          string `json:"stage"`           // pre_plan, post_plan, pre_apply, post_apply
	AccessToken    string `json:"access_token"`    // 一次性 Bearer Token

	// 平台能力声明
	Capabilities struct {
		Outcomes bool `json:"outcomes"` // 平台是否支持详细的 outcomes 结果
	} `json:"capabilities"`

	// Run Task 结果相关
	TaskResultID               string `json:"task_result_id"`
	TaskResultCallbackURL      string `json:"task_result_callback_url"`
	TaskResultEnforcementLevel string `json:"task_result_enforcement_level"`

	// Task 信息
	TaskID          uint   `json:"task_id"`
	TaskType        string `json:"task_type"`
	TaskStatus      string `json:"task_status"`
	TaskDescription string `json:"task_description"`
	TaskCreatedAt   string `json:"task_created_at"`
	TaskCreatedBy   string `json:"task_created_by"`
	TaskAppURL      string `json:"task_app_url"`

	// Workspace 信息
	WorkspaceID               string `json:"workspace_id"`
	WorkspaceName             string `json:"workspace_name"`
	WorkspaceWorkdir          string `json:"workspace_workdir"`
	WorkspaceTerraformVersion string `json:"workspace_terraform_version"`
	WorkspaceExecutionMode    string `json:"workspace_execution_mode"`
	WorkspaceAppURL           string `json:"workspace_app_url"`

	// 团队信息
	TeamID string `json:"team_id,omitempty"`

	// Plan 数据 URL（仅 post_plan/pre_apply/post_apply 阶段）
	PlanJSONAPIURL string `json:"plan_json_api_url,omitempty"`

	// Plan 变更统计（仅 post_plan/pre_apply/post_apply 阶段）
	PlanChanges *struct {
		Add     int `json:"add"`
		Change  int `json:"change"`
		Destroy int `json:"destroy"`
	} `json:"plan_changes,omitempty"`

	// 资源变更列表 URL（仅 post_plan/pre_apply/post_apply 阶段）
	ResourceChangesAPIURL string `json:"resource_changes_api_url,omitempty"`

	// 超时时间
	TimeoutSeconds int `json:"timeout_seconds"`
}

// RunTaskCallbackPayload Run Task 回调请求体（第三方服务返回）
// 符合 JSON:API 规范
type RunTaskCallbackPayload struct {
	Data struct {
		Type       string `json:"type"` // "task-results"
		Attributes struct {
			Status  string `json:"status"`  // running, passed, failed
			Message string `json:"message"` // 结果消息
			URL     string `json:"url"`     // 详情链接
		} `json:"attributes"`
		Relationships *struct {
			Outcomes *struct {
				Data []struct {
					Type       string `json:"type"` // "task-result-outcomes"
					Attributes struct {
						OutcomeID   string `json:"outcome-id"`
						Description string `json:"description"`
						Body        string `json:"body,omitempty"`
						URL         string `json:"url,omitempty"`
						Tags        JSONB  `json:"tags,omitempty"`
					} `json:"attributes"`
				} `json:"data"`
			} `json:"outcomes,omitempty"`
		} `json:"relationships,omitempty"`
	} `json:"data"`
}

// CreateRunTaskRequest 创建 Run Task 请求
type CreateRunTaskRequest struct {
	Name                   string                  `json:"name" binding:"required"`         // 名称
	Description            string                  `json:"description"`                     // 描述
	EndpointURL            string                  `json:"endpoint_url" binding:"required"` // Endpoint URL
	HMACKey                string                  `json:"hmac_key"`                        // HMAC密钥（可选）
	TimeoutSeconds         int                     `json:"timeout_seconds"`                 // 超时时间（秒）
	MaxRunSeconds          int                     `json:"max_run_seconds"`                 // 最大运行时间（秒）
	IsGlobal               bool                    `json:"is_global"`                       // 是否为全局任务
	GlobalStages           string                  `json:"global_stages"`                   // 全局任务默认执行阶段
	GlobalEnforcementLevel RunTaskEnforcementLevel `json:"global_enforcement_level"`        // 全局任务默认执行级别
	OrganizationID         *string                 `json:"organization_id"`                 // 组织ID
	TeamID                 *string                 `json:"team_id"`                         // 团队ID
}

// UpdateRunTaskRequest 更新 Run Task 请求
type UpdateRunTaskRequest struct {
	Name                   *string                  `json:"name"`                     // 名称
	Description            *string                  `json:"description"`              // 描述
	EndpointURL            *string                  `json:"endpoint_url"`             // Endpoint URL
	HMACKey                *string                  `json:"hmac_key"`                 // HMAC密钥（可选，空字符串表示清除）
	TimeoutSeconds         *int                     `json:"timeout_seconds"`          // 超时时间（秒）
	MaxRunSeconds          *int                     `json:"max_run_seconds"`          // 最大运行时间（秒）
	IsGlobal               *bool                    `json:"is_global"`                // 是否为全局任务
	GlobalStages           *string                  `json:"global_stages"`            // 全局任务默认执行阶段
	GlobalEnforcementLevel *RunTaskEnforcementLevel `json:"global_enforcement_level"` // 全局任务默认执行级别
	Enabled                *bool                    `json:"enabled"`                  // 是否启用
}

// CreateWorkspaceRunTaskRequest 创建 Workspace Run Task 请求
type CreateWorkspaceRunTaskRequest struct {
	RunTaskID        string                  `json:"run_task_id" binding:"required"` // Run Task ID
	Stage            RunTaskStage            `json:"stage" binding:"required"`       // 执行阶段
	EnforcementLevel RunTaskEnforcementLevel `json:"enforcement_level"`              // 执行级别，默认 advisory
}

// UpdateWorkspaceRunTaskRequest 更新 Workspace Run Task 请求
type UpdateWorkspaceRunTaskRequest struct {
	Stage            *RunTaskStage            `json:"stage"`             // 执行阶段
	EnforcementLevel *RunTaskEnforcementLevel `json:"enforcement_level"` // 执行级别
	Enabled          *bool                    `json:"enabled"`           // 是否启用
}
