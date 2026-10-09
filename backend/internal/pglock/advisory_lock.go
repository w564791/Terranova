package pglock

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"sync"

	"gorm.io/gorm"
)

// Locker provides PostgreSQL advisory lock operations.
// Advisory locks are session-level: they are automatically released when the
// database connection is closed, which makes them safe against Pod crashes
// — if a Pod dies, its connection drops and all its advisory locks are freed.
//
// Because the lock belongs to a session, every held lock pins its own
// connection out of the pool from TryLock until Unlock. Running the lock and
// the unlock through the shared pool would hand them arbitrary (possibly
// different) sessions: the unlock could miss and leak the lock until that
// connection is recycled, and a second TryLock in the same process could
// re-enter a lock its session already holds (advisory locks are re-entrant
// per session) and report it as acquired twice.
type Locker struct {
	db *gorm.DB

	mu       sync.Mutex
	held     map[int64]*sql.Conn
	heldDual map[[2]int32]*sql.Conn
}

// New creates a new Locker backed by the given gorm.DB.
func New(db *gorm.DB) *Locker {
	return &Locker{db: db, held: map[int64]*sql.Conn{}, heldDual: map[[2]int32]*sql.Conn{}}
}

func (l *Locker) conn(ctx context.Context) (*sql.Conn, error) {
	sqlDB, err := l.db.DB()
	if err != nil {
		return nil, err
	}
	return sqlDB.Conn(ctx)
}

// discard closes c without returning its session to the pool (used when an
// unlock failed, so a session that may still hold a lock is never reused).
func discard(c *sql.Conn) {
	_ = c.Raw(func(any) error { return driver.ErrBadConn })
	_ = c.Close()
}

func (l *Locker) tryLock(query string, args ...any) (*sql.Conn, bool, error) {
	ctx := context.Background()
	c, err := l.conn(ctx)
	if err != nil {
		return nil, false, err
	}
	var acquired bool
	if err := c.QueryRowContext(ctx, query, args...).Scan(&acquired); err != nil {
		discard(c)
		return nil, false, err
	}
	if !acquired {
		_ = c.Close()
		return nil, false, nil
	}
	return c, true, nil
}

func unlockOn(c *sql.Conn, query string, args ...any) (bool, error) {
	var released bool
	if err := c.QueryRowContext(context.Background(), query, args...).Scan(&released); err != nil {
		discard(c)
		return false, err
	}
	if !released {
		discard(c)
		return false, nil
	}
	return true, c.Close()
}

// TryLock attempts to acquire a session-level advisory lock identified by key.
// It is non-blocking: it returns immediately with acquired=true if the lock was
// obtained, or acquired=false if another session (including another caller
// in this process) already holds it.
func (l *Locker) TryLock(key int64) (acquired bool, err error) {
	c, ok, err := l.tryLock("SELECT pg_try_advisory_lock($1)", key)
	if err != nil {
		return false, fmt.Errorf("pglock: failed to try advisory lock (key=%d): %w", key, err)
	}
	if !ok {
		return false, nil
	}
	l.mu.Lock()
	l.held[key] = c
	l.mu.Unlock()
	return true, nil
}

// Unlock releases a session-level advisory lock identified by key on the
// session that acquired it. It returns released=false if this Locker does not
// hold the lock.
func (l *Locker) Unlock(key int64) (released bool, err error) {
	l.mu.Lock()
	c := l.held[key]
	delete(l.held, key)
	l.mu.Unlock()
	if c == nil {
		return false, nil
	}
	released, err = unlockOn(c, "SELECT pg_advisory_unlock($1)", key)
	if err != nil {
		return false, fmt.Errorf("pglock: failed to unlock advisory lock (key=%d): %w", key, err)
	}
	return released, nil
}

// TryLockDual attempts to acquire a session-level advisory lock identified by
// the composite (key1, key2) pair. This two-key variant is useful when the lock
// identity is naturally split across two dimensions (e.g. resource-type + id).
// It is non-blocking and returns immediately.
func (l *Locker) TryLockDual(key1, key2 int32) (acquired bool, err error) {
	c, ok, err := l.tryLock("SELECT pg_try_advisory_lock($1, $2)", key1, key2)
	if err != nil {
		return false, fmt.Errorf("pglock: failed to try advisory lock (key1=%d, key2=%d): %w", key1, key2, err)
	}
	if !ok {
		return false, nil
	}
	l.mu.Lock()
	l.heldDual[[2]int32{key1, key2}] = c
	l.mu.Unlock()
	return true, nil
}

// UnlockDual releases a session-level advisory lock identified by the composite
// (key1, key2) pair. It returns released=true if the lock was held and
// successfully released, or released=false if this Locker does not hold it.
func (l *Locker) UnlockDual(key1, key2 int32) (released bool, err error) {
	k := [2]int32{key1, key2}
	l.mu.Lock()
	c := l.heldDual[k]
	delete(l.heldDual, k)
	l.mu.Unlock()
	if c == nil {
		return false, nil
	}
	released, err = unlockOn(c, "SELECT pg_advisory_unlock($1, $2)", key1, key2)
	if err != nil {
		return false, fmt.Errorf("pglock: failed to unlock advisory lock (key1=%d, key2=%d): %w", key1, key2, err)
	}
	return released, nil
}
