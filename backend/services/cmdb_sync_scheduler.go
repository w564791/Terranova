package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// CMDBSyncScheduler 外部CMDB数据源定时同步调度器
type CMDBSyncScheduler struct {
	db                    *gorm.DB
	externalSourceService *CMDBExternalSourceService
	ticker                *time.Ticker
	stopChan              chan struct{}
	running               bool
	mu                    sync.Mutex
}

// NewCMDBSyncScheduler 创建同步调度器
func NewCMDBSyncScheduler(db *gorm.DB) *CMDBSyncScheduler {
	return &CMDBSyncScheduler{
		db:                    db,
		externalSourceService: NewCMDBExternalSourceService(db),
		stopChan:              make(chan struct{}),
	}
}

// Start 启动调度器
// checkInterval 是检查需要同步的数据源的间隔时间
func (s *CMDBSyncScheduler) Start(checkInterval time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		fmt.Println("[CMDB Sync Scheduler] Already running")
		return
	}

	if checkInterval < time.Minute {
		checkInterval = time.Minute
	}

	s.ticker = time.NewTicker(checkInterval)
	s.running = true

	go func() {
		fmt.Printf("[CMDB Sync Scheduler] Started with check interval: %v\n", checkInterval)

		// 启动时立即检查一次
		s.checkAndSync()

		for {
			select {
			case <-s.ticker.C:
				s.checkAndSync()
			case <-s.stopChan:
				fmt.Println("[CMDB Sync Scheduler] Stopped")
				return
			}
		}
	}()
}

// Stop 停止调度器
func (s *CMDBSyncScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	s.ticker.Stop()
	close(s.stopChan)
	s.running = false
}

// IsRunning 检查调度器是否正在运行
func (s *CMDBSyncScheduler) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// checkAndSync 检查并同步需要同步的数据源
func (s *CMDBSyncScheduler) checkAndSync() {
	fmt.Println("[CMDB Sync Scheduler] Checking for sources to sync...")

	// 查找需要同步的数据源
	// 条件：
	// 1. is_enabled = true
	// 2. sync_interval_minutes > 0 (0表示手动同步)
	// 3. last_sync_at IS NULL 或 last_sync_at < NOW() - INTERVAL sync_interval_minutes
	// 4. last_sync_status != 'running' (避免重复同步)
	var sources []models.CMDBExternalSource

	err := s.db.Where("is_enabled = ?", true).
		Where("sync_interval_minutes > 0").
		Where("(last_sync_status IS NULL OR last_sync_status != ?)", models.SyncStatusRunning).
		Where(`
			last_sync_at IS NULL OR 
			last_sync_at < NOW() - (sync_interval_minutes || ' minutes')::INTERVAL
		`).
		Find(&sources).Error

	if err != nil {
		fmt.Printf("[CMDB Sync Scheduler] Error querying sources: %v\n", err)
		return
	}

	if len(sources) == 0 {
		fmt.Println("[CMDB Sync Scheduler] No sources need syncing")
		return
	}

	fmt.Printf("[CMDB Sync Scheduler] Found %d sources to sync\n", len(sources))

	// 并发同步各个数据源
	var wg sync.WaitGroup
	for _, source := range sources {
		wg.Add(1)
		go func(src models.CMDBExternalSource) {
			defer wg.Done()
			s.syncSource(src)
		}(source)
	}
	wg.Wait()

	fmt.Println("[CMDB Sync Scheduler] Sync check completed")
}

// syncSource 同步单个数据源
func (s *CMDBSyncScheduler) syncSource(source models.CMDBExternalSource) {
	fmt.Printf("[CMDB Sync Scheduler] Starting sync for source: %s (%s)\n", source.Name, source.SourceID)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	err := s.externalSourceService.SyncExternalSource(ctx, source.SourceID)
	if err != nil {
		fmt.Printf("[CMDB Sync Scheduler] Sync failed for source %s: %v\n", source.SourceID, err)
	} else {
		fmt.Printf("[CMDB Sync Scheduler] Sync completed for source: %s\n", source.SourceID)
	}
}

// TriggerSync 手动触发特定数据源的同步
func (s *CMDBSyncScheduler) TriggerSync(sourceID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	return s.externalSourceService.SyncExternalSource(ctx, sourceID)
}

// GetSchedulerStatus 获取调度器状态
func (s *CMDBSyncScheduler) GetSchedulerStatus() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 获取启用自动同步的数据源数量
	var enabledCount int64
	s.db.Model(&models.CMDBExternalSource{}).
		Where("is_enabled = ? AND sync_interval_minutes > 0", true).
		Count(&enabledCount)

	// 获取正在同步的数据源数量
	var runningCount int64
	s.db.Model(&models.CMDBExternalSource{}).
		Where("last_sync_status = ?", models.SyncStatusRunning).
		Count(&runningCount)

	return map[string]interface{}{
		"running":               s.running,
		"enabled_sources_count": enabledCount,
		"syncing_sources_count": runningCount,
	}
}
