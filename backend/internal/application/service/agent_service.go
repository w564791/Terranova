package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// AgentService handles agent-related business logic
type AgentService struct {
	db *gorm.DB
}

// NewAgentService creates a new agent service
func NewAgentService(db *gorm.DB) *AgentService {
	return &AgentService{db: db}
}

// GenerateAgentID generates a semantic agent ID
// Format: agent-{16位随机a-z0-9}
func (s *AgentService) GenerateAgentID() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	const length = 16

	b := make([]byte, length)
	rand.Read(b)

	result := make([]byte, length)
	for i := range b {
		result[i] = charset[int(b[i])%len(charset)]
	}

	return "agent-" + string(result)
}

// GenerateTokenHash generates a hash for the agent token
func (s *AgentService) GenerateTokenHash(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// ValidateApplication validates application credentials
func (s *AgentService) ValidateApplication(appKey, appSecret string) (*entity.Application, error) {
	var app entity.Application
	err := s.db.Where("app_key = ? AND is_active = ?", appKey, true).First(&app).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invalid application credentials")
		}
		return nil, err
	}

	// Verify app secret (assuming it's hashed in database)
	// For now, direct comparison - should use proper hash comparison in production
	if app.AppSecret != appSecret {
		return nil, errors.New("invalid application credentials")
	}

	// Check if expired
	if app.IsExpired() {
		return nil, errors.New("application has expired")
	}

	// Update last used time
	now := time.Now()
	app.LastUsedAt = &now
	s.db.Model(&app).Update("last_used_at", now)

	return &app, nil
}

// RegisterAgent registers a new agent
func (s *AgentService) RegisterAgent(app *entity.Application, req *models.AgentRegisterRequest, ipAddress string) (*models.Agent, error) {
	// Generate agent ID
	agentID := s.GenerateAgentID()

	// Generate a token for the agent (this would be returned to agent for future auth)
	token := s.GenerateAgentID() // Reuse the same generation logic
	tokenHash := s.GenerateTokenHash(token)

	agent := &models.Agent{
		AgentID:       agentID,
		ApplicationID: int(app.ID),
		Name:          req.Name,
		TokenHash:     tokenHash,
		Status:        models.AgentStatusIdle,
		IPAddress:     &ipAddress,
		RegisteredAt:  time.Now(),
	}

	if req.Version != "" {
		agent.Version = &req.Version
	}

	if err := s.db.Create(agent).Error; err != nil {
		return nil, fmt.Errorf("failed to register agent: %w", err)
	}

	return agent, nil
}

// PingAgent updates agent heartbeat
func (s *AgentService) PingAgent(agentID string, status string) error {
	now := time.Now()

	result := s.db.Model(&models.Agent{}).
		Where("agent_id = ?", agentID).
		Updates(map[string]interface{}{
			"status":       status,
			"last_ping_at": now,
			"updated_at":   now,
		})

	if result.Error != nil {
		return fmt.Errorf("failed to update agent ping: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return errors.New("agent not found")
	}

	return nil
}

// GetAgent retrieves agent information
func (s *AgentService) GetAgent(agentID string) (*models.Agent, error) {
	var agent models.Agent
	err := s.db.Where("agent_id = ?", agentID).First(&agent).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("agent not found")
		}
		return nil, err
	}
	return &agent, nil
}

// UnregisterAgent removes an agent
func (s *AgentService) UnregisterAgent(agentID string) error {
	result := s.db.Where("agent_id = ?", agentID).Delete(&models.Agent{})
	if result.Error != nil {
		return fmt.Errorf("failed to unregister agent: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return errors.New("agent not found")
	}
	return nil
}

// ValidateAgentAccess is deprecated. Use ValidatePoolAccess instead.
// Agent-level authorization has been migrated to Pool-level authorization.

// ValidatePoolAccess performs pool-level bidirectional validation
// Checks: 1) Pool allows workspace, 2) Workspace has set this pool as current, 3) Pool has online agents
func (s *AgentService) ValidatePoolAccess(poolID string, workspaceID string) (bool, error) {
	// 1. Check if pool has allowed this workspace
	var poolAllow models.PoolAllowedWorkspace
	err := s.db.Where("pool_id = ? AND workspace_id = ? AND status = ?",
		poolID, workspaceID, models.AllowanceStatusActive).First(&poolAllow).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, errors.New("pool has not allowed this workspace")
		}
		return false, err
	}

	// 2. Check if workspace has set this pool as current
	var workspace models.Workspace
	err = s.db.Where("workspace_id = ?", workspaceID).First(&workspace).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, errors.New("workspace not found")
		}
		return false, err
	}

	if workspace.CurrentPoolID == nil || *workspace.CurrentPoolID != poolID {
		return false, errors.New("workspace has not set this pool as current")
	}

	// 3. Check if pool has at least one online agent
	var onlineCount int64
	err = s.db.Model(&models.Agent{}).
		Where("pool_id = ? AND status != ?", poolID, models.AgentStatusOffline).
		Count(&onlineCount).Error
	if err != nil {
		return false, err
	}

	if onlineCount == 0 {
		return false, errors.New("no online agents available in this pool")
	}

	return true, nil
}

// CleanupOfflineAgents marks agents as offline if no heartbeat for 2 minutes
// and deletes agents with no heartbeat for 5 minutes
func (s *AgentService) CleanupOfflineAgents() error {
	// Use database time to avoid timezone issues
	// Mark agents as offline if last_ping_at is more than 2 minutes ago
	result := s.db.Exec(`
		UPDATE agents 
		SET status = ?, updated_at = NOW()
		WHERE last_ping_at < NOW() - INTERVAL '2 minutes' 
		AND status != ?
	`, models.AgentStatusOffline, models.AgentStatusOffline)

	if result.Error != nil {
		return fmt.Errorf("failed to mark agents offline: %w", result.Error)
	}

	if result.RowsAffected > 0 {
		fmt.Printf("[AgentCleanup] Marked %d agents as offline (no heartbeat for 2+ minutes)\n", result.RowsAffected)
	}

	// Delete agents with no heartbeat for 5 minutes
	result = s.db.Exec(`
		DELETE FROM agents 
		WHERE last_ping_at < NOW() - INTERVAL '5 minutes'
	`)

	if result.Error != nil {
		return fmt.Errorf("failed to delete old agents: %w", result.Error)
	}

	if result.RowsAffected > 0 {
		fmt.Printf("[AgentCleanup] Deleted %d old agents (no heartbeat for 5+ minutes)\n", result.RowsAffected)
	}

	return nil
}

// CleanupOrphanedAllowances is deprecated and removed.
// Agent-level authorization tables (agent_allowed_workspaces, workspace_allowed_agents) have been dropped.
// Pool-level authorization does not require orphaned allowance cleanup.
