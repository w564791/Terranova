package models

import (
	"time"
)

// PoolToken represents a token for agent pool authentication
type PoolToken struct {
	TokenHash    string     `json:"-" gorm:"column:token_hash;primaryKey;size:64"` // SHA-256 hash as primary key
	TokenName    string     `json:"token_name" gorm:"column:token_name;not null;size:100"`
	TokenType    string     `json:"token_type" gorm:"column:token_type;not null;size:20"` // static or k8s_temporary
	PoolID       string     `json:"pool_id" gorm:"column:pool_id;not null;size:50;index"`
	IsActive     bool       `json:"is_active" gorm:"column:is_active;default:true;index"`
	CreatedAt    time.Time  `json:"created_at" gorm:"column:created_at;default:CURRENT_TIMESTAMP"`
	CreatedBy    *string    `json:"created_by,omitempty" gorm:"column:created_by;size:50"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty" gorm:"column:revoked_at"`
	RevokedBy    *string    `json:"revoked_by,omitempty" gorm:"column:revoked_by;size:50"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty" gorm:"column:last_used_at"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty" gorm:"column:expires_at"`
	K8sJobName   *string    `json:"k8s_job_name,omitempty" gorm:"column:k8s_job_name;size:255"`
	K8sPodName   *string    `json:"k8s_pod_name,omitempty" gorm:"column:k8s_pod_name;size:255"`
	K8sNamespace string     `json:"k8s_namespace" gorm:"column:k8s_namespace;size:100;default:'terraform'"`
}

// TableName specifies the table name for PoolToken model
func (PoolToken) TableName() string {
	return "pool_tokens"
}

// PoolTokenType constants
const (
	PoolTokenTypeStatic       = "static"
	PoolTokenTypeK8sTemporary = "k8s_temporary"
)

// ===== Pool Token Request/Response Models =====

// CreatePoolTokenRequest represents the request to create a pool token
type CreatePoolTokenRequest struct {
	TokenName string  `json:"token_name" binding:"required,max=100"`
	ExpiresAt *string `json:"expires_at,omitempty"` // ISO 8601 format, optional
}

// PoolTokenResponse represents a pool token in list view (without sensitive data)
type PoolTokenResponse struct {
	TokenName    string     `json:"token_name"`
	TokenType    string     `json:"token_type"`
	PoolID       string     `json:"pool_id"`
	IsActive     bool       `json:"is_active"`
	CreatedAt    time.Time  `json:"created_at"`
	CreatedBy    *string    `json:"created_by,omitempty"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
	RevokedBy    *string    `json:"revoked_by,omitempty"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	K8sJobName   *string    `json:"k8s_job_name,omitempty"`
	K8sPodName   *string    `json:"k8s_pod_name,omitempty"`
	K8sNamespace string     `json:"k8s_namespace,omitempty"`
}

// PoolTokenCreateResponse represents the response when creating a token (includes plaintext token)
type PoolTokenCreateResponse struct {
	Token     string     `json:"token"` // Plaintext token, only returned once
	TokenName string     `json:"token_name"`
	TokenType string     `json:"token_type"`
	PoolID    string     `json:"pool_id"`
	CreatedAt time.Time  `json:"created_at"`
	CreatedBy *string    `json:"created_by,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// PoolTokenListResponse represents the response for listing pool tokens
type PoolTokenListResponse struct {
	Tokens []PoolTokenResponse `json:"tokens"`
	Total  int                 `json:"total"`
}

// K8sJobTemplateConfig represents the K8s Job template configuration
type K8sJobTemplateConfig struct {
	Image           string            `json:"image" binding:"required"`
	ImagePullPolicy string            `json:"image_pull_policy,omitempty"`
	Command         []string          `json:"command,omitempty"`
	Args            []string          `json:"args,omitempty"`
	Env             map[string]string `json:"env,omitempty"`
	Resources       *K8sResources     `json:"resources,omitempty"`
	RestartPolicy   string            `json:"restart_policy,omitempty"`
	BackoffLimit    *int              `json:"backoff_limit,omitempty"`
	TTLSecondsAfter *int              `json:"ttl_seconds_after_finished,omitempty"`
	MinReplicas     int               `json:"min_replicas,omitempty"`
	MaxReplicas     int               `json:"max_replicas,omitempty"`
	FreezeSchedules []FreezeSchedule  `json:"freeze_schedules,omitempty"`
}

// FreezeSchedule represents a time window when the pool should be frozen
type FreezeSchedule struct {
	FromTime string `json:"from_time"` // HH:MM format, e.g., "23:00"
	ToTime   string `json:"to_time"`   // HH:MM format, e.g., "02:00" (can span across days)
	Weekdays []int  `json:"weekdays"`  // 1=Monday, 2=Tuesday, ..., 7=Sunday
}

// K8sResources represents K8s resource requirements
type K8sResources struct {
	Requests map[string]string `json:"requests,omitempty"`
	Limits   map[string]string `json:"limits,omitempty"`
}

// CreateK8sJobRequest represents the request to create a K8s job
type CreateK8sJobRequest struct {
	JobName     string `json:"job_name" binding:"required,max=63"`
	WorkspaceID string `json:"workspace_id" binding:"required"`
	TaskID      string `json:"task_id,omitempty"`
}

// CreateK8sJobResponse represents the response when creating a K8s job
type CreateK8sJobResponse struct {
	JobName      string     `json:"job_name"`
	Token        string     `json:"token"` // Temporary token for the job
	TokenName    string     `json:"token_name"`
	K8sNamespace string     `json:"k8s_namespace"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// UpdateK8sConfigRequest represents the request to update K8s configuration
type UpdateK8sConfigRequest struct {
	K8sConfig K8sJobTemplateConfig `json:"k8s_config" binding:"required"`
}
