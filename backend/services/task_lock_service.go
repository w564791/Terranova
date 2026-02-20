package services

import (
	"errors"
	"time"

	"iac-platform/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TaskLockService 任务锁服务
type TaskLockService struct {
	db *gorm.DB
}

// NewTaskLockService 创建任务锁服务
func NewTaskLockService(db *gorm.DB) *TaskLockService {
	return &TaskLockService{db: db}
}

// AcquireTask Agent获取任务（带锁）
func (s *TaskLockService) AcquireTask(agentID string, lockDuration int) (*models.WorkspaceTask, error) {
	if lockDuration == 0 {
		lockDuration = 300 // 默认5分钟
	}

	// 使用数据库行锁获取任务
	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var task models.WorkspaceTask

	// 查找pending状态且未锁定或锁已过期的任务
	err := tx.Where("status = ? AND (locked_by IS NULL OR locked_by = '' OR lock_expires_at < ?)",
		models.TaskStatusPending, time.Now()).
		Order("created_at ASC").
		Limit(1).
		Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		First(&task).Error

	if err != nil {
		tx.Rollback()
		if err == gorm.ErrRecordNotFound {
			return nil, errors.New("no available tasks")
		}
		return nil, err
	}

	// 锁定任务
	now := time.Now()
	lockExpiresAt := now.Add(time.Duration(lockDuration) * time.Second)

	err = tx.Model(&task).Updates(map[string]interface{}{
		"locked_by":       agentID,
		"locked_at":       now,
		"lock_expires_at": lockExpiresAt,
		"status":          models.TaskStatusRunning,
	}).Error

	if err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	return &task, nil
}

// RenewLock 续期任务锁
func (s *TaskLockService) RenewLock(taskID uint, agentID string, lockDuration int) error {
	if lockDuration == 0 {
		lockDuration = 300
	}

	lockExpiresAt := time.Now().Add(time.Duration(lockDuration) * time.Second)

	result := s.db.Model(&models.WorkspaceTask{}).
		Where("id = ? AND locked_by = ?", taskID, agentID).
		Update("lock_expires_at", lockExpiresAt)

	if result.RowsAffected == 0 {
		return errors.New("task not locked by this agent")
	}

	return result.Error
}

// ReleaseLock 释放任务锁
func (s *TaskLockService) ReleaseLock(taskID uint, agentID string) error {
	result := s.db.Model(&models.WorkspaceTask{}).
		Where("id = ? AND locked_by = ?", taskID, agentID).
		Updates(map[string]interface{}{
			"locked_by":       nil,
			"locked_at":       nil,
			"lock_expires_at": nil,
		})

	if result.RowsAffected == 0 {
		return errors.New("task not locked by this agent")
	}

	return result.Error
}

// CleanExpiredLocks 清理过期的锁
func (s *TaskLockService) CleanExpiredLocks() error {
	return s.db.Model(&models.WorkspaceTask{}).
		Where("lock_expires_at < ? AND status = ?", time.Now(), models.TaskStatusRunning).
		Updates(map[string]interface{}{
			"locked_by":       nil,
			"locked_at":       nil,
			"lock_expires_at": nil,
			"status":          models.TaskStatusPending,
		}).Error
}
