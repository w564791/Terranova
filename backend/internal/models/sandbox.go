package models

import "time"

// Sandbox is a platform registration of an externally managed AgentCore
// harness/runtime (or shell_debug driver). The platform does not create or
// destroy the remote container; DELETE only unbinds this row (§10.2).
type Sandbox struct {
	ID               string  `json:"id" gorm:"primaryKey;size:36"`
	OrgID            uint    `json:"org_id" gorm:"not null;index:idx_sandboxes_org;uniqueIndex:uq_sandboxes_org_ref"`
	WorkspaceID      string  `json:"workspace_id" gorm:"type:varchar(50);not null;index:idx_sandboxes_workspace"`
	SandboxRef       string  `json:"sandbox_ref" gorm:"size:256;not null;uniqueIndex:uq_sandboxes_org_ref"`
	RuntimeRef       *string `json:"runtime_ref,omitempty" gorm:"size:256"`
	Label            *string `json:"label,omitempty" gorm:"size:256"`
	Driver           string  `json:"driver" gorm:"size:32;not null"` // agentcore_external | shell_debug
	ImageDigest      *string `json:"image_digest,omitempty" gorm:"size:128"`
	AssumeRoleARN    *string `json:"assume_role_arn,omitempty" gorm:"size:256"`
	AssumeExternalID *string `json:"assume_external_id,omitempty" gorm:"size:128"`
	// NetworkMode is a snapshot (VPC|PUBLIC) when known; list/detail may show
	// 「非 VPC / 仅联调」 for PUBLIC. Production attach requires VPC (empty/PUBLIC refused).
	NetworkMode    *string    `json:"network_mode,omitempty" gorm:"size:16"`
	Status         string     `json:"status" gorm:"size:32;not null;default:registered"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at,omitempty"`
	CreatedBy      string     `json:"created_by" gorm:"size:20;not null"`
	CreatedAt      time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (Sandbox) TableName() string { return "sandboxes" }

const (
	SandboxDriverAgentCoreExternal = "agentcore_external"
	SandboxDriverShellDebug        = "shell_debug"

	SandboxStatusRegistered   = "registered"
	SandboxStatusReady        = "ready"
	SandboxStatusLeaseExpired = "lease_expired"
	SandboxStatusUnbound      = "unbound"
)

// ShellSession is a short-lived Shell binding on a sandbox; credentials share
// its lifetime (§10.2). Create/delete revoke hooks are stubbed until token
// tables gain typ=shell.
type ShellSession struct {
	ID        string     `json:"id" gorm:"primaryKey;size:36"`
	SandboxID string     `json:"sandbox_id" gorm:"size:36;not null;index:idx_shell_sessions_sandbox"`
	OrgID     uint       `json:"org_id" gorm:"not null;index:idx_shell_sessions_org"`
	Status    string     `json:"status" gorm:"size:20;not null;default:active"`
	ExpiresAt time.Time  `json:"expires_at" gorm:"not null"`
	ClosedAt  *time.Time `json:"closed_at,omitempty"`
	CreatedBy string     `json:"created_by" gorm:"size:20;not null"`
	CreatedAt time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (ShellSession) TableName() string { return "shell_sessions" }

const (
	ShellSessionStatusActive = "active"
	ShellSessionStatusClosed = "closed"
)

// ShellToken stores the SHA-256 of a typ=shell JWT (never the plaintext).
// Bound 1:1 to a shell_sessions row; revoked when the session ends (§10.2).
type ShellToken struct {
	ID             int64      `json:"id" gorm:"primaryKey;autoIncrement"`
	ShellSessionID string     `json:"shell_session_id" gorm:"size:36;not null;index:idx_shell_tokens_session"`
	SandboxID      string     `json:"sandbox_id" gorm:"size:36;not null;index:idx_shell_tokens_sandbox"`
	OrgID          uint       `json:"org_id" gorm:"not null"`
	WorkspaceID    string     `json:"workspace_id" gorm:"type:varchar(50);not null"`
	TokenHash      string     `json:"-" gorm:"size:64;not null;uniqueIndex:uq_shell_tokens_token_hash"`
	ExpiresAt      time.Time  `json:"expires_at" gorm:"not null"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at" gorm:"autoCreateTime"`
}

func (ShellToken) TableName() string { return "shell_tokens" }
