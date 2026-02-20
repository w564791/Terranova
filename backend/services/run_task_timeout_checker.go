package services

import (
	"context"
	"log"
	"time"

	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// RunTaskTimeoutChecker checks for timed out run task results
type RunTaskTimeoutChecker struct {
	db       *gorm.DB
	interval time.Duration
	stopCh   chan struct{}
}

// NewRunTaskTimeoutChecker creates a new timeout checker
func NewRunTaskTimeoutChecker(db *gorm.DB, interval time.Duration) *RunTaskTimeoutChecker {
	if interval == 0 {
		interval = 30 * time.Second
	}
	return &RunTaskTimeoutChecker{
		db:       db,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start starts the timeout checker
func (c *RunTaskTimeoutChecker) Start(ctx context.Context) {
	log.Printf("[RunTaskTimeoutChecker] Starting with interval %v", c.interval)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("[RunTaskTimeoutChecker] Context cancelled, stopping")
			return
		case <-c.stopCh:
			log.Println("[RunTaskTimeoutChecker] Stop signal received, stopping")
			return
		case <-ticker.C:
			c.checkTimeouts()
		}
	}
}

// Stop stops the timeout checker
func (c *RunTaskTimeoutChecker) Stop() {
	close(c.stopCh)
}

// checkTimeouts checks for timed out run task results
func (c *RunTaskTimeoutChecker) checkTimeouts() {
	now := time.Now()

	// Find running results that have timed out (no progress update)
	var timedOutResults []models.RunTaskResult
	err := c.db.Where("status = ? AND timeout_at < ?", models.RunTaskResultRunning, now).
		Find(&timedOutResults).Error
	if err != nil {
		log.Printf("[RunTaskTimeoutChecker] Error finding timed out results: %v", err)
		return
	}

	for _, result := range timedOutResults {
		log.Printf("[RunTaskTimeoutChecker] Result %s timed out (no progress update)", result.ResultID)
		c.markAsTimeout(&result, "No progress update received within timeout period")
	}

	// Find running results that have exceeded max run time
	var maxRunTimeoutResults []models.RunTaskResult
	err = c.db.Where("status = ? AND max_run_timeout_at < ?", models.RunTaskResultRunning, now).
		Find(&maxRunTimeoutResults).Error
	if err != nil {
		log.Printf("[RunTaskTimeoutChecker] Error finding max run timeout results: %v", err)
		return
	}

	for _, result := range maxRunTimeoutResults {
		log.Printf("[RunTaskTimeoutChecker] Result %s exceeded max run time", result.ResultID)
		c.markAsTimeout(&result, "Maximum run time exceeded")
	}

	if len(timedOutResults) > 0 || len(maxRunTimeoutResults) > 0 {
		log.Printf("[RunTaskTimeoutChecker] Processed %d progress timeouts, %d max run timeouts",
			len(timedOutResults), len(maxRunTimeoutResults))
	}
}

// markAsTimeout marks a result as timed out
func (c *RunTaskTimeoutChecker) markAsTimeout(result *models.RunTaskResult, message string) {
	now := time.Now()
	result.Status = models.RunTaskResultTimeout
	result.Message = message
	result.CompletedAt = &now
	result.UpdatedAt = now

	// Clean up access token on timeout
	result.AccessToken = ""
	result.AccessTokenUsed = true

	if err := c.db.Save(result).Error; err != nil {
		log.Printf("[RunTaskTimeoutChecker] Error updating result %s: %v", result.ResultID, err)
	}
}
