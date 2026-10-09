package pglock

import (
	"os"
	"sync"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// openTestDB connects to PostgreSQL (TEST_PG_DSN, default the local test
// cluster); skips when it is not reachable.
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		dsn = "host=localhost user=postgres password=postgres123 port=5432 dbname=postgres sslmode=disable"
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	sqlDB, _ := db.DB()
	if err := sqlDB.Ping(); err != nil {
		t.Skipf("PostgreSQL not reachable: %v", err)
	}
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

func TestLockerSameProcessIsExclusiveAndUnlockReleases(t *testing.T) {
	db := openTestDB(t)
	l := New(db)
	const key = int64(0x7e57_0001)

	ok, err := l.TryLock(key)
	if err != nil || !ok {
		t.Fatalf("first TryLock: %v %v", ok, err)
	}
	// a pooled session must not re-enter the lock (session-level advisory
	// locks are re-entrant per session)
	for i := 0; i < 8; i++ {
		if ok, err := l.TryLock(key); err != nil || ok {
			t.Fatalf("second TryLock acquired a held lock (attempt %d): %v %v", i, ok, err)
		}
	}
	// a different Locker (another replica) is excluded too
	other := New(db)
	if ok, _ := other.TryLock(key); ok {
		t.Fatal("other locker acquired a held lock")
	}

	released, err := l.Unlock(key)
	if err != nil || !released {
		t.Fatalf("Unlock: %v %v", released, err)
	}
	// really released on the server, whatever pooled session asks next
	if ok, err := other.TryLock(key); err != nil || !ok {
		t.Fatalf("lock not released: %v %v", ok, err)
	}
	other.Unlock(key)

	if released, _ := l.Unlock(key); released {
		t.Fatal("Unlock of a lock not held reported released")
	}
}

func TestLockerConcurrentTryLockSingleWinner(t *testing.T) {
	db := openTestDB(t)
	l := New(db)
	const key = int64(0x7e57_0002)
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, err := l.TryLock(key); err == nil && ok {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("want exactly one winner, got %d", wins)
	}
	if released, err := l.Unlock(key); err != nil || !released {
		t.Fatalf("Unlock: %v %v", released, err)
	}
}

func TestLockerDual(t *testing.T) {
	db := openTestDB(t)
	l := New(db)
	if ok, err := l.TryLockDual(7, 9); err != nil || !ok {
		t.Fatalf("TryLockDual: %v %v", ok, err)
	}
	if ok, _ := l.TryLockDual(7, 9); ok {
		t.Fatal("dual lock re-entered")
	}
	if released, err := l.UnlockDual(7, 9); err != nil || !released {
		t.Fatalf("UnlockDual: %v %v", released, err)
	}
	if ok, _ := l.TryLockDual(7, 9); !ok {
		t.Fatal("dual lock not released")
	}
	l.UnlockDual(7, 9)
}
