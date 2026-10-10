package models

import (
	"encoding/json"
	"time"
)

// Manifest run / sandbox session / run token (docs/manifest/manifest-sandbox-spec.md §2-§3, §6.1).
// Schema: migration 20261004_02_manifest_sandbox_schema (authoritative; the
// application never AutoMigrates). gorm allows one check tag per field, so the
// tags mirror only the spec-critical CHECKs of the same name (runner values,
// sandbox => preview, sandbox => session) for model-level tests.

const (
	ManifestRunRunnerAgent   = "agent"   // 远程 K8s 驱动:preview 与 approval
	ManifestRunRunnerSandbox = "sandbox" // 云 sandbox(AgentCore):仅 preview

	ManifestRunPurposePreview  = "preview"  // 只读 state、-lock=false、不可审批
	ManifestRunPurposeApproval = "approval" // 仅 agent、加锁、可审批

	ManifestRunStatusPending   = "pending"
	ManifestRunStatusRunning   = "running"
	ManifestRunStatusSucceeded = "succeeded"
	ManifestRunStatusFailed    = "failed"
	ManifestRunStatusCancelled = "cancelled"

	SandboxProviderAgentCore = "agentcore"
	SandboxNetworkModeVPC    = "vpc" // 唯一允许的网络模式(spec §6.2)

	SandboxSessionStatusActive  = "active"
	SandboxSessionStatusClosed  = "closed"
	SandboxSessionStatusExpired = "expired"
)

// ManifestRun 对不可变 bundle 的一次 plan 执行。
type ManifestRun struct {
	ID           string          `json:"id" gorm:"primaryKey;size:36"` // 格式: mfr-{id}
	ManifestID   string          `json:"manifest_id" gorm:"size:36;not null"`
	VersionID    *string         `json:"version_id,omitempty" gorm:"size:36"` // native 草稿预览为 NULL
	BundleHash   string          `json:"bundle_hash" gorm:"size:64;not null"`
	WorkspaceID  string          `json:"workspace_id" gorm:"type:varchar(50);not null"`
	Runner       string          `json:"runner" gorm:"size:16;not null;check:chk_manifest_runs_runner,runner IN ('agent', 'sandbox')"`
	Purpose      string          `json:"purpose" gorm:"size:16;not null;check:chk_manifest_runs_sandbox_preview_only,runner <> 'sandbox' OR purpose = 'preview'"`
	Status       string          `json:"status" gorm:"size:20;not null;default:pending"`
	PlanHash     *string         `json:"plan_hash,omitempty" gorm:"size:64"`        // 脱敏后 plan 的哈希
	PlanRedacted json.RawMessage `json:"plan_redacted,omitempty" gorm:"type:jsonb"` // 脱敏后的 plan JSON
	StateSerial  *int64          `json:"state_serial,omitempty"`                    // preview 基准 state serial
	SessionID    *string         `json:"session_id,omitempty" gorm:"size:36;check:chk_manifest_runs_sandbox_session,runner <> 'sandbox' OR session_id IS NOT NULL"`
	AgentID      *string         `json:"agent_id,omitempty" gorm:"size:50"` // runner=agent:被指派的 agent,只有它能用 agent token 换取 run token
	CreatedBy    string          `json:"created_by" gorm:"size:20;not null"`
	CreatedAt    time.Time       `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt    time.Time       `json:"updated_at" gorm:"autoUpdateTime"`
}

func (ManifestRun) TableName() string { return "manifest_runs" }

// SandboxSession 用户 + workspace 绑定的 sandbox 会话;不存任何 token / 凭证。
type SandboxSession struct {
	ID          string     `json:"id" gorm:"primaryKey;size:36"`
	UserID      string     `json:"user_id" gorm:"size:20;not null"`
	WorkspaceID string     `json:"workspace_id" gorm:"type:varchar(50);not null"`
	Provider    string     `json:"provider" gorm:"size:32;not null"`
	NetworkMode string     `json:"network_mode" gorm:"size:16;not null;default:vpc;check:chk_sandbox_sessions_network_mode,network_mode = 'vpc'"`
	Status      string     `json:"status" gorm:"size:20;not null;default:active"`
	ExpiresAt   time.Time  `json:"expires_at" gorm:"not null"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (SandboxSession) TableName() string { return "sandbox_sessions" }

// RunToken 按 run 签发的平台 token(只存 SHA-256 哈希,同 workspace_tasks.state_token_hash)。
// DB 外键保证 (run_id, workspace_id, purpose) 与所属 run 一致、session 与 workspace 一致。
type RunToken struct {
	ID          int64      `json:"id" gorm:"primaryKey;autoIncrement"`
	RunID       string     `json:"run_id" gorm:"size:36;not null"`
	SessionID   *string    `json:"session_id,omitempty" gorm:"size:36"`
	WorkspaceID string     `json:"workspace_id" gorm:"type:varchar(50);not null"`
	Purpose     string     `json:"purpose" gorm:"size:16;not null;check:chk_run_tokens_purpose,purpose IN ('preview', 'approval')"`
	TokenHash   string     `json:"-" gorm:"size:64;not null;uniqueIndex:uq_run_tokens_token_hash"`
	ExpiresAt   time.Time  `json:"expires_at" gorm:"not null"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	AgentID     *string    `json:"agent_id,omitempty" gorm:"size:50"` // 换取该 token 的 agent;agent 撤销时一并撤销
	CreatedAt   time.Time  `json:"created_at" gorm:"autoCreateTime"`
}

func (RunToken) TableName() string { return "run_tokens" }
