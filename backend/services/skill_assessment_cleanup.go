package services

import (
	"context"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"
)

// AssessmentCleanup 定期清理过期的评估快照数据
// 保留策略（来自设计文档 8.3）：
//   - input_snapshot / output_snapshot: 90 天
//   - assessment_raw_output: 30 天
//   - user_modification_diff: 90 天
//   - skill_content_snapshot: 永久（每个 hash 仅一条）
//   - skill_assessment_results 结构化字段: 永久
type AssessmentCleanup struct {
	db      *gorm.DB
	running bool
	mu      sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewAssessmentCleanup(db *gorm.DB) *AssessmentCleanup {
	return &AssessmentCleanup{db: db}
}

// Start 启动每日清理定时器
func (c *AssessmentCleanup) Start(ctx context.Context) {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return
	}
	c.running = true
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.mu.Unlock()

	go func() {
		// 启动时执行一次
		c.run()

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
				c.run()
			}
		}
	}()

	log.Println("[AssessmentCleanup] Started (daily)")
}

func (c *AssessmentCleanup) run() {
	now := time.Now()
	days90 := now.AddDate(0, 0, -90)
	days30 := now.AddDate(0, 0, -30)

	// 1. 清空 90 天前的 input/output snapshot
	result := c.db.Exec(`
		UPDATE skill_usage_logs
		SET input_snapshot = NULL, output_snapshot = NULL, user_modification_diff = NULL
		WHERE created_at < ? AND (input_snapshot IS NOT NULL OR output_snapshot IS NOT NULL OR user_modification_diff IS NOT NULL)
	`, days90)
	if result.RowsAffected > 0 {
		log.Printf("[AssessmentCleanup] Cleared snapshots for %d logs older than 90 days", result.RowsAffected)
	}

	// 2. 清空 30 天前的 assessment_raw_output
	result = c.db.Exec(`
		UPDATE skill_assessment_results
		SET assessment_raw_output = NULL
		WHERE assessed_at < ? AND assessment_raw_output IS NOT NULL
	`, days30)
	if result.RowsAffected > 0 {
		log.Printf("[AssessmentCleanup] Cleared raw_output for %d assessments older than 30 days", result.RowsAffected)
	}
}
