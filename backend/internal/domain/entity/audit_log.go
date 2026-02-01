package entity

import (
	"time"

	"iac-platform/internal/domain/valueobject"
)

// AuditActionType 审计操作类型
type AuditActionType string

const (
	// AuditActionGrant 授予权限
	AuditActionGrant AuditActionType = "GRANT"
	// AuditActionRevoke 撤销权限
	AuditActionRevoke AuditActionType = "REVOKE"
	// AuditActionModify 修改权限
	AuditActionModify AuditActionType = "MODIFY"
	// AuditActionExpire 权限过期
	AuditActionExpire AuditActionType = "EXPIRE"
)

// String 返回操作类型的字符串表示
func (a AuditActionType) String() string {
	return string(a)
}

// PermissionAuditLog 权限变更审计日志
type PermissionAuditLog struct {
	ID            uint                         `json:"id"`
	ActionType    AuditActionType              `json:"action_type"`                           // 操作类型
	ScopeType     valueobject.ScopeType        `json:"scope_type"`                            // 作用域类型
	ScopeID       uint                         `json:"scope_id"`                              // 作用域ID
	PrincipalType valueobject.PrincipalType    `json:"principal_type"`                        // 主体类型
	PrincipalID   string                       `json:"principal_id"`                          // 主体ID
	PermissionID  *string                      `gorm:"type:varchar(32)" json:"permission_id"` // 权限ID（业务语义ID，可为空）
	OldLevel      *valueobject.PermissionLevel `json:"old_level"`                             // 原权限等级
	NewLevel      *valueobject.PermissionLevel `json:"new_level"`                             // 新权限等级
	PerformedBy   string                       `json:"performed_by" gorm:"type:varchar(20)"`  // 操作人user_id
	PerformedAt   time.Time                    `json:"performed_at"`                          // 操作时间
	IPAddress     string                       `json:"ip_address"`                            // IP地址
	UserAgent     string                       `json:"user_agent"`                            // User Agent
	Reason        string                       `json:"reason"`                                // 操作原因
}

// TableName 指定表名
func (PermissionAuditLog) TableName() string {
	return "permission_audit_logs"
}

// AccessLog 资源访问日志
type AccessLog struct {
	ID             uint                         `json:"id"`
	UserID         string                       `json:"user_id" gorm:"type:varchar(20)"`   // 用户ID
	ResourceType   string                       `json:"resource_type"`                     // 资源类型
	ResourceID     uint                         `json:"resource_id"`                       // 资源ID
	Action         string                       `json:"action"`                            // 操作动作
	IsAllowed      bool                         `json:"is_allowed"`                        // 是否允许
	DenyReason     string                       `json:"deny_reason"`                       // 拒绝原因
	EffectiveLevel *valueobject.PermissionLevel `json:"effective_level"`                   // 有效权限等级
	AccessedAt     time.Time                    `json:"accessed_at"`                       // 访问时间
	IPAddress      string                       `json:"ip_address"`                        // IP地址
	UserAgent      string                       `json:"user_agent"`                        // User Agent
	RequestPath    string                       `json:"request_path"`                      // 请求路径
	HttpCode       int                          `json:"http_code"`                         // HTTP状态码
	RequestBody    string                       `json:"request_body"`                      // 请求体
	RequestHeaders string                       `json:"request_headers" gorm:"type:jsonb"` // 请求头（JSON字符串）
	DurationMs     int                          `json:"duration_ms"`                       // 请求耗时（毫秒）
}

// TableName 指定表名
func (AccessLog) TableName() string {
	return "access_logs"
}

// TaskTemporaryPermission 临时任务权限
type TaskTemporaryPermission struct {
	ID             uint                   `json:"id"`
	TaskID         uint                   `json:"task_id"`         // 任务ID
	UserEmail      string                 `json:"user_email"`      // 被授权用户邮箱
	UserID         *uint                  `json:"user_id"`         // 用户ID（可能为空）
	PermissionType string                 `json:"permission_type"` // 权限类型（APPLY/CANCEL）
	GrantedBy      string                 `json:"granted_by"`      // 授权来源
	GrantedAt      time.Time              `json:"granted_at"`      // 授权时间
	ExpiresAt      time.Time              `json:"expires_at"`      // 过期时间
	WebhookPayload map[string]interface{} `json:"webhook_payload"` // Webhook原始数据
	IsUsed         bool                   `json:"is_used"`         // 是否已使用
	UsedAt         *time.Time             `json:"used_at"`         // 使用时间
}

// TableName 指定表名
func (TaskTemporaryPermission) TableName() string {
	return "task_temporary_permissions"
}

// IsExpired 判断临时权限是否过期
func (t *TaskTemporaryPermission) IsExpired() bool {
	return t.ExpiresAt.Before(time.Now())
}

// IsValid 判断临时权限是否有效（未过期且未使用）
func (t *TaskTemporaryPermission) IsValid() bool {
	return !t.IsExpired() && !t.IsUsed
}

// MarkAsUsed 标记为已使用
func (t *TaskTemporaryPermission) MarkAsUsed() {
	t.IsUsed = true
	now := time.Now()
	t.UsedAt = &now
}

// WebhookConfig Webhook配置
type WebhookConfig struct {
	ID          uint                   `json:"id"`
	OrgID       uint                   `json:"org_id"`      // 所属组织ID
	Name        string                 `json:"name"`        // 配置名称
	WebhookURL  string                 `json:"webhook_url"` // Webhook URL
	SecretToken string                 `json:"-"`           // 密钥（不返回）
	EventTypes  map[string]interface{} `json:"event_types"` // 事件类型列表
	IsActive    bool                   `json:"is_active"`   // 是否启用
	CreatedBy   *uint                  `json:"created_by"`  // 创建人
	CreatedAt   time.Time              `json:"created_at"`  // 创建时间

	// 关联
	Organization *Organization `json:"organization,omitempty" gorm:"foreignKey:OrgID"`
}

// TableName 指定表名
func (WebhookConfig) TableName() string {
	return "webhook_configs"
}

// WebhookLog Webhook调用日志
type WebhookLog struct {
	ID              uint                   `json:"id"`
	WebhookConfigID *uint                  `json:"webhook_config_id"` // Webhook配置ID
	EventType       string                 `json:"event_type"`        // 事件类型
	TaskID          *uint                  `json:"task_id"`           // 任务ID
	RequestPayload  map[string]interface{} `json:"request_payload"`   // 请求数据
	ResponsePayload map[string]interface{} `json:"response_payload"`  // 响应数据
	StatusCode      int                    `json:"status_code"`       // HTTP状态码
	ErrorMessage    string                 `json:"error_message"`     // 错误信息
	CreatedAt       time.Time              `json:"created_at"`        // 创建时间

	// 关联
	WebhookConfig *WebhookConfig `json:"webhook_config,omitempty" gorm:"foreignKey:WebhookConfigID"`
}

// TableName 指定表名
func (WebhookLog) TableName() string {
	return "webhook_logs"
}

// IsSuccess 判断Webhook调用是否成功
func (w *WebhookLog) IsSuccess() bool {
	return w.StatusCode >= 200 && w.StatusCode < 300
}
