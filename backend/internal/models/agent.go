package models

import (
	"encoding/json"
	"time"
)

// Agent represents an agent instance in the system
type Agent struct {
	AgentID       string     `gorm:"column:agent_id;primaryKey;type:varchar(50)" json:"agent_id"`
	ApplicationID int        `gorm:"column:application_id;not null" json:"application_id"`
	PoolID        *string    `gorm:"column:pool_id;type:varchar(50)" json:"pool_id,omitempty"`
	Name          string     `gorm:"column:name;type:varchar(100)" json:"name"`
	TokenHash     string     `gorm:"column:token_hash;type:varchar(255);not null" json:"-"` // 不返回给客户端
	Status        string     `gorm:"column:status;type:varchar(20);default:'idle'" json:"status"`
	IPAddress     *string    `gorm:"column:ip_address;type:varchar(50)" json:"ip_address,omitempty"`
	Version       *string    `gorm:"column:version;type:varchar(50)" json:"version,omitempty"`
	LastPingAt    *time.Time `gorm:"column:last_ping_at" json:"last_ping_at,omitempty"`
	ConnectedPod  *string    `gorm:"column:connected_pod;type:varchar(100)" json:"connected_pod"`
	Capabilities  *string    `gorm:"column:capabilities;type:jsonb" json:"capabilities,omitempty"`
	Metadata      *string    `gorm:"column:metadata;type:jsonb" json:"metadata,omitempty"`
	RegisteredAt  time.Time  `gorm:"column:registered_at;not null;default:CURRENT_TIMESTAMP" json:"registered_at"`
	CreatedBy     *string    `gorm:"column:created_by;type:varchar(50)" json:"created_by,omitempty"`
	UpdatedBy     *string    `gorm:"column:updated_by;type:varchar(50)" json:"updated_by,omitempty"`
	CreatedAt     time.Time  `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	// TokenGeneration agent tokens carry it (gen claim); revocation bumps it.
	TokenGeneration int `gorm:"column:token_generation;not null;default:0" json:"-"`
	// RevokedAt set when the agent's tokens were revoked (agent tokens and
	// its run tokens are refused from then on).
	RevokedAt *time.Time `gorm:"column:revoked_at" json:"revoked_at,omitempty"`
	// PoolTokenHash the pool token the agent registered with; its agent
	// tokens are valid only while that pool token is active.
	PoolTokenHash *string `gorm:"column:pool_token_hash;type:varchar(64)" json:"-"`
}

// TableName specifies the table name for Agent model
func (Agent) TableName() string {
	return "agents"
}

// AgentStatus constants
const (
	AgentStatusIdle    = "idle"
	AgentStatusBusy    = "busy"
	AgentStatusOffline = "offline"
)

// IsOnline checks if the agent is online (last ping within 2 minutes)
func (a *Agent) IsOnline() bool {
	if a.LastPingAt == nil {
		return false
	}
	return time.Since(*a.LastPingAt) < 2*time.Minute
}

// AgentRegisterRequest represents the request body for agent registration
type AgentRegisterRequest struct {
	Name    string `json:"name" binding:"omitempty,max=100"`
	Version string `json:"version" binding:"omitempty,max=50"`
	// Capabilities the agent build supports (AgentCapability*). Unknown
	// values are ignored; an agent that sends none (older builds) has none.
	Capabilities []string `json:"capabilities" binding:"omitempty,max=32,dive,max=64"`
}

// Agent capabilities (agents.capabilities, reported at registration).
const (
	// AgentCapabilityManifestBundleV1 the agent unpacks the verified manifest
	// bundle / Run files from task data (manifest_bundle, external_files with
	// bundle_hash) and re-checks the hash before terraform init.
	AgentCapabilityManifestBundleV1 = "manifest_bundle_v1"
	// AgentCapabilityTaskDataOverridesV1 the agent applies the deployment
	// variable overrides (and their sensitivity) from task data.
	AgentCapabilityTaskDataOverridesV1 = "task_data_overrides_v1"
	// AgentCapabilityAgentTokenV1 the agent authenticates every call after
	// registration (task / workspace API, C&C WebSocket) with its per-agent
	// JWT (typ agent) and renews it; the agent ID comes from the token. The
	// platform refuses pool-token calls on the tasks of such an agent.
	// (Replaces the X-Agent-ID header of agent_identity_header_v1, which is
	// no longer trusted or known.)
	AgentCapabilityAgentTokenV1 = "agent_token_v1"
)

// SupportedAgentCapabilities the capabilities of this build (sent by the
// agent at registration, accepted by the platform).
func SupportedAgentCapabilities() []string {
	return []string{AgentCapabilityManifestBundleV1, AgentCapabilityTaskDataOverridesV1, AgentCapabilityAgentTokenV1}
}

// ManifestAgentCapabilities what an agent needs to run a task of a
// manifest-bound workspace (or a manifest Run task).
func ManifestAgentCapabilities() []string {
	return []string{AgentCapabilityManifestBundleV1, AgentCapabilityTaskDataOverridesV1, AgentCapabilityAgentTokenV1}
}

// HasCapability reports whether the agent reported capability c.
func (a *Agent) HasCapability(c string) bool {
	return len(a.MissingCapabilities([]string{c})) == 0
}

// KnownAgentCapabilities filters reported capabilities to the known ones
// (deduplicated, in SupportedAgentCapabilities order).
func KnownAgentCapabilities(reported []string) []string {
	have := make(map[string]bool, len(reported))
	for _, c := range reported {
		have[c] = true
	}
	var out []string
	for _, c := range SupportedAgentCapabilities() {
		if have[c] {
			out = append(out, c)
		}
	}
	return out
}

// MissingCapabilities the capabilities of required the agent did not report.
func (a *Agent) MissingCapabilities(required []string) []string {
	have := map[string]bool{}
	if a != nil && a.Capabilities != nil {
		var caps []string
		if err := json.Unmarshal([]byte(*a.Capabilities), &caps); err == nil {
			for _, c := range caps {
				have[c] = true
			}
		}
	}
	var missing []string
	for _, c := range required {
		if !have[c] {
			missing = append(missing, c)
		}
	}
	return missing
}

// AgentRegisterResponse represents the response for agent registration
type AgentRegisterResponse struct {
	AgentID      string    `json:"agent_id"`
	Status       string    `json:"status"`
	RegisteredAt time.Time `json:"registered_at"`
}

// AgentPingRequest represents the request body for agent ping
type AgentPingRequest struct {
	Status       string        `json:"status" binding:"required,oneof=idle busy"`
	CPUUsage     float64       `json:"cpu_usage"`      // CPU使用率 0-100
	MemoryUsage  float64       `json:"memory_usage"`   // 内存使用率 0-100
	RunningTasks []RunningTask `json:"running_tasks"`  // 当前运行的任务
}

// RunningTask 运行中的任务信息
type RunningTask struct {
	TaskID      uint   `json:"task_id"`
	TaskType    string `json:"task_type"`
	WorkspaceID string `json:"workspace_id"`
	StartedAt   string `json:"started_at"`
}

// AgentPingResponse represents the response for agent ping
type AgentPingResponse struct {
	Message    string    `json:"message"`
	LastPingAt time.Time `json:"last_ping_at"`
}

// Note: Agent-level authorization has been deprecated and migrated to Pool-level authorization.
// All agent authorization related types have been removed.
// Please use Pool-level authorization APIs instead.
