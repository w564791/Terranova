package services

import (
	"log"
	"time"

	"iac-platform/internal/application/service"

	"gorm.io/gorm"
)

// AgentCleanupService handles periodic cleanup of agent-related data
type AgentCleanupService struct {
	db           *gorm.DB
	agentService *service.AgentService
	ticker       *time.Ticker
	done         chan bool
}

// NewAgentCleanupService creates a new agent cleanup service
func NewAgentCleanupService(db *gorm.DB) *AgentCleanupService {
	return &AgentCleanupService{
		db:           db,
		agentService: service.NewAgentService(db),
		done:         make(chan bool),
	}
}

// Start begins the cleanup service with the specified interval
func (s *AgentCleanupService) Start(interval time.Duration) {
	log.Printf("[AgentCleanup] Starting agent cleanup service with interval: %v", interval)

	s.ticker = time.NewTicker(interval)

	go func() {
		// Run cleanup immediately on start
		s.runCleanup()

		// Then run periodically
		for {
			select {
			case <-s.ticker.C:
				s.runCleanup()
			case <-s.done:
				log.Println("[AgentCleanup] Stopping agent cleanup service")
				return
			}
		}
	}()
}

// Stop stops the cleanup service
func (s *AgentCleanupService) Stop() {
	if s.ticker != nil {
		s.ticker.Stop()
	}
	s.done <- true
}

// runCleanup performs the actual cleanup operations
func (s *AgentCleanupService) runCleanup() {
	log.Println("[AgentCleanup] Running agent cleanup...")

	// 1. Mark offline agents and delete old ones
	if err := s.agentService.CleanupOfflineAgents(); err != nil {
		log.Printf("[AgentCleanup] Error cleaning up offline agents: %v", err)
	} else {
		log.Println("[AgentCleanup] Successfully cleaned up offline agents")
	}

	// Note: CleanupOrphanedAllowances has been removed.
	// Agent-level authorization tables have been dropped.
	// Pool-level authorization does not require orphaned allowance cleanup.

	log.Println("[AgentCleanup] Cleanup completed")
}
