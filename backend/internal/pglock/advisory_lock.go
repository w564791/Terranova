package pglock

import (
	"fmt"

	"gorm.io/gorm"
)

// Locker provides PostgreSQL advisory lock operations.
// Advisory locks are session-level: they are automatically released when the
// database connection is closed, which makes them safe against Pod crashes
// — if a Pod dies, its connection drops and all its advisory locks are freed.
type Locker struct {
	db *gorm.DB
}

// New creates a new Locker backed by the given gorm.DB.
func New(db *gorm.DB) *Locker {
	return &Locker{db: db}
}

// TryLock attempts to acquire a session-level advisory lock identified by key.
// It is non-blocking: it returns immediately with acquired=true if the lock was
// obtained, or acquired=false if another session already holds it.
func (l *Locker) TryLock(key int64) (acquired bool, err error) {
	result := l.db.Raw("SELECT pg_try_advisory_lock(?)", key).Scan(&acquired)
	if result.Error != nil {
		return false, fmt.Errorf("pglock: failed to try advisory lock (key=%d): %w", key, result.Error)
	}
	return acquired, nil
}

// Unlock releases a session-level advisory lock identified by key.
// It returns released=true if the lock was held and successfully released,
// or released=false if the lock was not held by this session.
func (l *Locker) Unlock(key int64) (released bool, err error) {
	result := l.db.Raw("SELECT pg_advisory_unlock(?)", key).Scan(&released)
	if result.Error != nil {
		return false, fmt.Errorf("pglock: failed to unlock advisory lock (key=%d): %w", key, result.Error)
	}
	return released, nil
}

// TryLockDual attempts to acquire a session-level advisory lock identified by
// the composite (key1, key2) pair. This two-key variant is useful when the lock
// identity is naturally split across two dimensions (e.g. resource-type + id).
// It is non-blocking and returns immediately.
func (l *Locker) TryLockDual(key1, key2 int32) (acquired bool, err error) {
	result := l.db.Raw("SELECT pg_try_advisory_lock(?, ?)", key1, key2).Scan(&acquired)
	if result.Error != nil {
		return false, fmt.Errorf("pglock: failed to try advisory lock (key1=%d, key2=%d): %w", key1, key2, result.Error)
	}
	return acquired, nil
}

// UnlockDual releases a session-level advisory lock identified by the composite
// (key1, key2) pair. It returns released=true if the lock was held and
// successfully released, or released=false if the lock was not held by this session.
func (l *Locker) UnlockDual(key1, key2 int32) (released bool, err error) {
	result := l.db.Raw("SELECT pg_advisory_unlock(?, ?)", key1, key2).Scan(&released)
	if result.Error != nil {
		return false, fmt.Errorf("pglock: failed to unlock advisory lock (key1=%d, key2=%d): %w", key1, key2, result.Error)
	}
	return released, nil
}
