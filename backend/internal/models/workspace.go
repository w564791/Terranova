package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// WorkspaceState 生命周期状态枚举
type WorkspaceState string

const (
	WorkspaceStateCreated      WorkspaceState = "created"
	WorkspaceStatePlanning     WorkspaceState = "planning"
	WorkspaceStatePlanDone     WorkspaceState = "plan_done"
	WorkspaceStateWaitingApply WorkspaceState = "waiting_apply"
	WorkspaceStateApplying     WorkspaceState = "applying"
	WorkspaceStateCompleted    WorkspaceState = "completed"
	WorkspaceStateFailed       WorkspaceState = "failed"
)

// ExecutionMode 执行模式枚举
type ExecutionMode string

const (
	ExecutionModeLocal ExecutionMode = "local"
	ExecutionModeAgent ExecutionMode = "agent"
	ExecutionModeK8s   ExecutionMode = "k8s"
)

// JSONB 自定义JSONB类型
type JSONB map[string]interface{}

// Value 实现 driver.Valuer 接口
func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan 实现 sql.Scanner 接口
func (j *JSONB) Scan(value interface{}) error {
	if value == nil {
		// NULL值时初始化为空map，避免扫描错误
		*j = make(map[string]interface{})
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		*j = make(map[string]interface{})
		return nil
	}

	// 直接解析JSON，可以是map或array
	var data interface{}
	if err := json.Unmarshal(bytes, &data); err != nil {
		return fmt.Errorf("failed to unmarshal JSONB: %w", err)
	}

	// 如果是map，直接使用
	if mapData, ok := data.(map[string]interface{}); ok {
		*j = mapData
		return nil
	}

	// 如果是array，将其存储在"_array"键下（临时方案，用于兼容）
	// 注意：这只是为了让GORM能够读取旧数据，新数据不应该是数组格式
	// 使用 UnwrapArray() 方法取出原始数组
	if arrayData, ok := data.([]interface{}); ok {
		*j = map[string]interface{}{"_array": arrayData}
		return nil
	}

	return fmt.Errorf("JSONB data is neither map nor array")
}

// UnwrapArray 从 JSONB 中提取被 Scan 包装的数组数据。
// DB 中存储的 JSON 数组在 Scan 后变为 {"_array": [...]},
// 此方法将其还原为 []byte JSON 数组，供 json.Unmarshal 使用。
// 如果 JSONB 本身就是 map 格式（无 _array 键），则原样 marshal 返回。
func (j JSONB) UnwrapArray() ([]byte, error) {
	if j == nil {
		return []byte("[]"), nil
	}
	if arr, ok := j["_array"]; ok {
		return json.Marshal(arr)
	}
	return json.Marshal(j)
}

// WorkspaceVariableArray 自定义WorkspaceVariable数组类型（用于JSONB存储）
type WorkspaceVariableArray []WorkspaceVariable

// Value 实现 driver.Valuer 接口
func (w WorkspaceVariableArray) Value() (driver.Value, error) {
	if w == nil {
		return nil, nil
	}
	return json.Marshal(w)
}

// Scan 实现 sql.Scanner 接口
func (w *WorkspaceVariableArray) Scan(value interface{}) error {
	if value == nil {
		*w = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, w)
}

// VariableSnapshot 变量快照引用（只存储必要字段）
type VariableSnapshot struct {
	WorkspaceID  string `json:"workspace_id"`  // Workspace语义化ID
	VariableID   string `json:"variable_id"`   // 变量语义化ID
	Version      int    `json:"version"`       // 版本号
	VariableType string `json:"variable_type"` // 变量类型: terraform或environment
}

// VariableSnapshotArray 变量快照数组类型
type VariableSnapshotArray []VariableSnapshot

// Value 实现 driver.Valuer 接口
func (v VariableSnapshotArray) Value() (driver.Value, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}

// Scan 实现 sql.Scanner 接口
func (v *VariableSnapshotArray) Scan(value interface{}) error {
	if value == nil {
		*v = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, v)
}

// Workspace 工作空间模型
type Workspace struct {
	// 基础字段
	ID          uint      `json:"id" gorm:"primaryKey"`                                                 // 数据库主键(继续对外暴露,兼容)
	WorkspaceID string    `json:"workspace_id" gorm:"column:workspace_id;type:varchar(50);uniqueIndex"` // 语义化ID(新增字段)
	Name        string    `json:"name" gorm:"not null;uniqueIndex"`
	Description string    `json:"description"`
	CreatedBy   *string   `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// 执行模式
	ExecutionMode ExecutionMode `json:"execution_mode" gorm:"default:local"`
	AgentID       *uint         `json:"agent_id"`

	// Apply方法
	AutoApply bool `json:"auto_apply" gorm:"default:false"`
	PlanOnly  bool `json:"plan_only" gorm:"default:false"`

	// Terraform配置
	TerraformVersion string `json:"terraform_version" gorm:"default:latest"`
	Workdir          string `json:"workdir" gorm:"default:/workspace"`

	// 状态后端
	StateBackend string `json:"state_backend" gorm:"not null"`
	StateConfig  JSONB  `json:"state_config" gorm:"type:jsonb"`

	// 锁定状态
	IsLocked   bool       `json:"is_locked" gorm:"default:false"`
	LockedBy   *string    `json:"locked_by" gorm:"type:varchar(20)"`
	LockedAt   *time.Time `json:"locked_at"`
	LockReason string     `json:"lock_reason"`

	// 文件存储
	TFCode  JSONB `json:"tf_code" gorm:"type:jsonb"`
	TFState JSONB `json:"tf_state" gorm:"type:jsonb"`

	// Provider配置
	ProviderConfig JSONB `json:"provider_config" gorm:"type:jsonb"`

	// Provider模板引用
	ProviderTemplateIDs JSONB `json:"provider_template_ids" gorm:"type:jsonb"` // 引用的全局模板ID列表
	ProviderOverrides   JSONB `json:"provider_overrides" gorm:"type:jsonb"`    // 按provider类型的字段覆盖

	// Provider配置变更跟踪（用于优化 terraform init -upgrade）
	ProviderConfigHash       string `json:"provider_config_hash" gorm:"type:varchar(64)"`        // provider_config 的 SHA256 hash
	LastInitHash             string `json:"last_init_hash" gorm:"type:varchar(64)"`              // 上次成功 init 时的 hash
	LastInitTerraformVersion string `json:"last_init_terraform_version" gorm:"type:varchar(20)"` // 上次成功 init 时的 terraform 版本

	// Terraform Lock 文件（用于加速 terraform init，避免重复下载 provider）
	TerraformLockHCL string `json:"terraform_lock_hcl" gorm:"type:text"` // .terraform.lock.hcl 文件内容

	// 初始化配置
	InitConfig JSONB `json:"init_config" gorm:"type:jsonb"`

	// 重试配置
	RetryEnabled bool `json:"retry_enabled" gorm:"default:true"`
	MaxRetries   int  `json:"max_retries" gorm:"default:3"`

	// 通知和日志配置
	NotifySettings JSONB `json:"notify_settings" gorm:"type:jsonb"`
	LogConfig      JSONB `json:"log_config" gorm:"type:jsonb"`

	// 生命周期状态
	State WorkspaceState `json:"state" gorm:"default:created"`

	// Tags
	Tags JSONB `json:"tags" gorm:"type:jsonb"`

	// 系统变量
	SystemVariables JSONB `json:"system_variables" gorm:"type:jsonb"`

	// Overview统计字段
	ResourceCount  int        `json:"resource_count" gorm:"default:0"` // 当前管理的资源数量
	LastPlanAt     *time.Time `json:"last_plan_at" gorm:"index"`       // 最后一次Plan执行时间
	LastApplyAt    *time.Time `json:"last_apply_at" gorm:"index"`      // 最后一次Apply执行时间
	DriftCount     int        `json:"drift_count" gorm:"default:0"`    // Drift资源数量
	LastDriftCheck *time.Time `json:"last_drift_check"`                // 最后一次Drift检测时间

	// Terraform执行引擎字段
	CurrentCodeVersionID   *uint  `json:"current_code_version_id"`                                                 // 当前代码版本ID
	WorkspaceExecutionMode string `json:"workspace_execution_mode" gorm:"type:varchar(20);default:plan_and_apply"` // 执行模式: plan_only或plan_and_apply

	// UI配置
	UIMode string `json:"ui_mode" gorm:"type:varchar(20);default:console"` // UI展示模式: console或structured

	// Plan数据配置
	ShowUnchangedResources bool `json:"show_unchanged_resources" gorm:"default:false"` // 是否显示无变更资源（影响plan_json返回和no-op资源过滤）

	// Outputs共享配置
	OutputsSharing string `json:"outputs_sharing" gorm:"type:varchar(20);default:none"` // Outputs共享模式: none/all/specific

	// Drift 检测配置
	DriftCheckEnabled   bool   `json:"drift_check_enabled" gorm:"default:true"`                  // 是否启用 drift 检测
	DriftCheckStartTime string `json:"drift_check_start_time" gorm:"type:time;default:07:00:00"` // 每天允许检测的开始时间
	DriftCheckEndTime   string `json:"drift_check_end_time" gorm:"type:time;default:22:00:00"`   // 每天允许检测的结束时间
	DriftCheckInterval  int    `json:"drift_check_interval" gorm:"default:1440"`                 // 检测间隔（分钟）

	// CMDB 同步状态（用于前端展示和互斥控制）
	CMDBSyncStatus      string     `json:"cmdb_sync_status" gorm:"type:varchar(20);default:idle"`      // idle, syncing
	CMDBSyncTriggeredBy string     `json:"cmdb_sync_triggered_by" gorm:"type:varchar(20)"`             // auto, manual, rebuild
	CMDBSyncStartedAt   *time.Time `json:"cmdb_sync_started_at"`                                      // 同步开始时间
	CMDBSyncCompletedAt *time.Time `json:"cmdb_sync_completed_at"`                                    // 同步完成时间

	// 关联
	AgentPoolID        *uint                 `json:"agent_pool_id" gorm:"index"`                    // Agent Pool ID (deprecated, use CurrentPoolID)
	CurrentPoolID      *string               `json:"current_pool_id" gorm:"type:varchar(50);index"` // Current Pool ID (pool-level authorization)
	K8sConfigID        *uint                 `json:"k8s_config_id" gorm:"index"`                    // K8s配置ID
	CurrentCodeVersion *WorkspaceCodeVersion `json:"current_code_version,omitempty" gorm:"foreignKey:CurrentCodeVersionID"`
}

// TableName 指定表名
func (Workspace) TableName() string {
	return "workspaces"
}

// CMDB 同步状态常量
const (
	CMDBSyncStatusIdle    = "idle"    // 空闲
	CMDBSyncStatusSyncing = "syncing" // 同步中
)

// CMDB 同步触发来源
const (
	CMDBSyncTriggerAuto    = "auto"    // apply 后自动触发
	CMDBSyncTriggerManual  = "manual"  // 前端手动触发 sync
	CMDBSyncTriggerRebuild = "rebuild" // 前端手动触发 rebuild
)

// TaskType 任务类型枚举
type TaskType string

const (
	TaskTypePlan         TaskType = "plan"
	TaskTypeApply        TaskType = "apply"
	TaskTypePlanAndApply TaskType = "plan_and_apply" // Plan+Apply组合任务
	TaskTypeDriftCheck   TaskType = "drift_check"    // Drift 检测任务
)

// TaskStatus 任务状态枚举
type TaskStatus string

const (
	TaskStatusPending            TaskStatus = "pending"
	TaskStatusWaiting            TaskStatus = "waiting" // 等待前置任务完成
	TaskStatusRunning            TaskStatus = "running"
	TaskStatusApplyPending       TaskStatus = "apply_pending"        // Plan完成，等待用户确认Apply
	TaskStatusPlannedAndFinished TaskStatus = "planned_and_finished" // Plan完成，无需Apply（无变更）
	TaskStatusSuccess            TaskStatus = "success"              // Plan任务成功完成
	TaskStatusApplied            TaskStatus = "applied"              // Apply任务成功完成
	TaskStatusFailed             TaskStatus = "failed"
	TaskStatusCancelled          TaskStatus = "cancelled"
)

// WorkspaceTask 工作空间任务模型
type WorkspaceTask struct {
	// 基础字段
	ID          uint      `json:"id" gorm:"primaryKey"`
	WorkspaceID string    `json:"workspace_id" gorm:"type:varchar(50);not null;index"` // 语义化ID
	CreatedBy   *string   `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// 任务描述
	Description string `json:"description" gorm:"type:text"`

	// 任务类型
	TaskType TaskType `json:"task_type" gorm:"not null"`

	// 任务状态
	Status TaskStatus `json:"status" gorm:"default:pending;index"`

	// 执行信息
	ExecutionMode ExecutionMode `json:"execution_mode" gorm:"not null"`
	AgentID       *string       `json:"agent_id" gorm:"type:varchar(50);index"` // Agent的语义化ID
	K8sConfigID   *uint         `json:"k8s_config_id"`
	K8sPodName    string        `json:"k8s_pod_name" gorm:"index"`
	K8sNamespace  string        `json:"k8s_namespace" gorm:"default:iac-platform"`
	ExecutionNode string        `json:"execution_node"` // 执行节点标识

	// 任务锁（防止多Agent同时获取同一任务）
	LockedBy      string     `json:"locked_by" gorm:"index"`       // Agent ID
	LockedAt      *time.Time `json:"locked_at"`                    // 锁定时间
	LockExpiresAt *time.Time `json:"lock_expires_at" gorm:"index"` // 锁过期时间

	// Terraform输出
	PlanOutput   string `json:"plan_output" gorm:"type:text"`
	ApplyOutput  string `json:"apply_output" gorm:"type:text"`
	ErrorMessage string `json:"error_message" gorm:"type:text"`

	// 执行时间
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	Duration    int        `json:"duration"` // 秒

	// 重试信息
	RetryCount int `json:"retry_count" gorm:"default:0"`
	MaxRetries int `json:"max_retries" gorm:"default:3"`

	// Plan变更统计
	ChangesAdd     int `json:"changes_add" gorm:"default:0"`     // Plan显示的新增资源数
	ChangesChange  int `json:"changes_change" gorm:"default:0"`  // Plan显示的修改资源数
	ChangesDestroy int `json:"changes_destroy" gorm:"default:0"` // Plan显示的删除资源数

	// Terraform执行引擎字段
	PlanTaskID *uint                  `json:"plan_task_id"`                                        // Apply任务关联的Plan任务ID
	PlanData   []byte                 `json:"-" gorm:"type:bytea"`                                 // Plan二进制数据（plan.out文件）
	PlanJSON   JSONB                  `json:"plan_json" gorm:"type:jsonb"`                         // Plan JSON格式数据
	PlanHash   string                 `json:"plan_hash" gorm:"type:varchar(64)"`                   // Plan文件SHA256 hash（用于优化）
	Outputs    JSONB                  `json:"outputs" gorm:"type:jsonb"`                           // Terraform outputs
	Stage      string                 `json:"stage" gorm:"type:varchar(30);default:pending;index"` // 执行阶段
	Context    JSONB                  `json:"context" gorm:"type:jsonb"`                           // 阶段上下文数据

	// Plan+Apply流程字段
	SnapshotID       string `json:"snapshot_id" gorm:"type:varchar(64)"` // 资源版本快照ID（旧版本）
	ApplyDescription string `json:"apply_description" gorm:"type:text"`  // Apply描述

	// Plan+Apply快照字段（新版本，用于修复竞态条件bug）
	SnapshotResourceVersions JSONB      `json:"snapshot_resource_versions" gorm:"type:jsonb"` // 资源版本快照
	SnapshotVariables        JSONB      `json:"snapshot_variables" gorm:"type:jsonb"`         // 变量快照
	SnapshotProviderConfig   JSONB      `json:"snapshot_provider_config" gorm:"type:jsonb"`   // Provider配置快照
	SnapshotCreatedAt        *time.Time `json:"snapshot_created_at"`                          // 快照创建时间

	// Apply确认审计字段（用于追踪谁在什么时间确认了apply）
	ApplyConfirmedBy *string    `json:"apply_confirmed_by" gorm:"type:varchar(255)"` // 确认apply的用户ID
	ApplyConfirmedAt *time.Time `json:"apply_confirmed_at"`                          // 确认apply的时间

	// 后台任务标记（drift_check 等后台任务不显示在任务列表中）
	IsBackground bool `json:"is_background" gorm:"default:false;index"` // 是否为后台任务

	// 关联
	Workspace *Workspace     `json:"workspace,omitempty" gorm:"foreignKey:WorkspaceID"`
	PlanTask  *WorkspaceTask `json:"plan_task,omitempty" gorm:"foreignKey:PlanTaskID"`
}

// TableName 指定表名
func (WorkspaceTask) TableName() string {
	return "workspace_tasks"
}

// WorkspaceStateVersion State版本模型
type WorkspaceStateVersion struct {
	// 基础字段
	ID          uint      `json:"id" gorm:"primaryKey"`
	WorkspaceID string    `json:"workspace_id" gorm:"type:varchar(50);not null;index"` // 语义化ID
	CreatedBy   *string   `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`

	// 文件内容
	Content JSONB `json:"content" gorm:"type:jsonb;not null"`

	// 版本信息
	Version   int    `json:"version" gorm:"not null"`
	Checksum  string `json:"checksum" gorm:"not null"`
	SizeBytes int    `json:"size_bytes"`

	// Terraform State 校验字段
	Lineage string `json:"lineage" gorm:"type:varchar(255);index:idx_state_versions_lineage"` // State lineage ID
	Serial  int    `json:"serial"`                                                            // State serial number

	// 导入标记
	IsImported   bool   `json:"is_imported" gorm:"default:false;index:idx_state_versions_is_imported"` // 是否为用户手动导入
	ImportSource string `json:"import_source" gorm:"type:varchar(50)"`                                 // 来源: user_upload, api, terraform_apply

	// 回滚标记
	IsRollback          bool  `json:"is_rollback" gorm:"default:false;index:idx_state_versions_is_rollback"` // 是否为回滚操作创建
	RollbackFromVersion *uint `json:"rollback_from_version"`                                                 // 回滚源版本号（version 字段值，不是数据库 ID）

	// 描述信息
	Description string `json:"description" gorm:"type:text"` // 版本描述（上传说明或回滚原因）

	// 关联任务和资源统计
	TaskID        *uint `json:"task_id" gorm:"index"`            // 关联的任务ID
	ResourceCount int   `json:"resource_count" gorm:"default:0"` // State中的资源数量

	// 关联
	Workspace         *Workspace             `json:"workspace,omitempty" gorm:"foreignKey:WorkspaceID"`
	Task              *WorkspaceTask         `json:"task,omitempty" gorm:"foreignKey:TaskID"`
	RollbackFromState *WorkspaceStateVersion `json:"rollback_from_state,omitempty" gorm:"foreignKey:RollbackFromVersion"`
}

// TableName 指定表名
func (WorkspaceStateVersion) TableName() string {
	return "workspace_state_versions"
}

// TaskComment 任务评论模型
type TaskComment struct {
	// 基础字段
	ID        uint      `json:"id" gorm:"primaryKey"`
	TaskID    uint      `json:"task_id" gorm:"not null;index"`
	UserID    *string   `json:"user_id" gorm:"type:varchar(20)"`
	Username  string    `json:"username" gorm:"not null"`
	Comment   string    `json:"comment" gorm:"type:text;not null"`
	CreatedAt time.Time `json:"created_at"`

	// 操作类型
	ActionType string `json:"action_type" gorm:"type:varchar(50)"` // comment, confirm_apply, cancel, cancel_previous

	// 关联
	Task *WorkspaceTask `json:"task,omitempty" gorm:"foreignKey:TaskID"`
}

// TableName 指定表名
func (TaskComment) TableName() string {
	return "task_comments"
}

// WorkspaceTaskResourceChange 任务资源变更模型（用于Structured Run Output）
type WorkspaceTaskResourceChange struct {
	// 基础字段
	ID          uint      `json:"id" gorm:"primaryKey"`
	TaskID      uint      `json:"task_id" gorm:"not null;index"`
	WorkspaceID string    `json:"workspace_id" gorm:"type:varchar(50);not null;index"` // 语义化ID
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// 资源标识
	ResourceAddress string `json:"resource_address" gorm:"type:varchar(500);not null"` // 完整地址
	ResourceType    string `json:"resource_type" gorm:"type:varchar(100);not null"`    // 资源类型
	ResourceName    string `json:"resource_name" gorm:"type:varchar(200);not null"`    // 资源名称
	ModuleAddress   string `json:"module_address" gorm:"type:varchar(500)"`            // 模块地址

	// 变更信息
	Action        string `json:"action" gorm:"type:varchar(20);not null;index"` // create/update/delete/replace
	ChangesBefore JSONB  `json:"changes_before" gorm:"type:jsonb"`              // before 数据（完整）
	ChangesAfter  JSONB  `json:"changes_after" gorm:"type:jsonb"`               // after 数据（完整）

	// Apply 阶段状态（用于实时更新）
	ApplyStatus      string     `json:"apply_status" gorm:"type:varchar(20);default:pending;index"` // pending/applying/completed/failed
	ApplyStartedAt   *time.Time `json:"apply_started_at"`
	ApplyCompletedAt *time.Time `json:"apply_completed_at"`
	ApplyError       string     `json:"apply_error" gorm:"type:text"`

	// 资源详情（从terraform state提取）
	ResourceID         *string `json:"resource_id" gorm:"type:varchar(500)"`  // 资源ID（如AWS资源的ID）
	ResourceAttributes JSONB   `json:"resource_attributes" gorm:"type:jsonb"` // 资源属性（如ARN等）

	// 关联
	Task      *WorkspaceTask `json:"task,omitempty" gorm:"foreignKey:TaskID"`
	Workspace *Workspace     `json:"workspace,omitempty" gorm:"foreignKey:WorkspaceID"`
}

// TableName 指定表名
func (WorkspaceTaskResourceChange) TableName() string {
	return "workspace_task_resource_changes"
}
