package health

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// CheckDatabase verifies that the database is reachable by performing a Ping
// with the given timeout. Returns nil on success or an error describing the failure.
func CheckDatabase(db *gorm.DB, timeout time.Duration) error {
	if db == nil {
		return fmt.Errorf("database connection is nil")
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	return nil
}
