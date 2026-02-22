package database

import (
	"fmt"
	"time"

	"iac-platform/internal/config"
	"iac-platform/internal/observability/metrics"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func Initialize(cfg config.DatabaseConfig) (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=Asia/Singapore",
		cfg.Host, cfg.User, cfg.Password, cfg.Name, cfg.Port, cfg.SSLMode)
	if cfg.SSLRootCert != "" {
		dsn += fmt.Sprintf(" sslrootcert=%s", cfg.SSLRootCert)
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// 跳过自动迁移，使用SQL初始化脚本

	// Register observability GORM callbacks
	metrics.RegisterGORMCallbacks(db)

	// Start DB connection stats collector
	sqlDB, err := db.DB()
	if err == nil {
		metrics.StartDBStatsCollector(sqlDB, 15*time.Second)
	}

	return db, nil
}
