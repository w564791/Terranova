package services

import (
	"context"
	"log"
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
func (s *CMDBSyncScheduler) Start(ctx context.Context, checkInterval time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		log.Printf("[CMDBSyncScheduler] Already running")
		return
	}

	if checkInterval < time.Minute {
		checkInterval = time.Minute
	}

	s.ticker = time.NewTicker(checkInterval)
	s.running = true

	go func() {
		log.Printf("[CMDBSyncScheduler] Started with check interval: %v", checkInterval)

		// 启动时立即检查一次
		s.checkAndSync()

		for {
			select {
			case <-ctx.Done():
				log.Println("[CMDBSyncScheduler] Stopped: context cancelled")
				return
			case <-s.ticker.C:
				s.checkAndSync()
			case <-s.stopChan:
				log.Println("[CMDBSyncScheduler] Stopped")
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
	log.Println("[CMDBSyncScheduler] Checking for sources to sync...")

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
		log.Printf("[CMDBSyncScheduler] Error querying sources: %v", err)
		return
	}

	if len(sources) == 0 {
		log.Println("[CMDBSyncScheduler] No sources need syncing")
		return
	}

	log.Printf("[CMDBSyncScheduler] Found %d sources to sync", len(sources))

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

	log.Println("[CMDBSyncScheduler] Sync check completed")
}

// syncSource 同步单个数据源
func (s *CMDBSyncScheduler) syncSource(source models.CMDBExternalSource) {
	log.Printf("[CMDBSyncScheduler] Starting sync for source: %s (%s)", source.Name, source.SourceID)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	err := s.externalSourceService.SyncExternalSource(ctx, source.SourceID)
	if err != nil {
		log.Printf("[CMDBSyncScheduler] Sync failed for source %s: %v", source.SourceID, err)
	} else {
		log.Printf("[CMDBSyncScheduler] Sync completed for source: %s", source.SourceID)
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
