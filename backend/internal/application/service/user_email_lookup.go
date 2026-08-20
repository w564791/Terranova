package service

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// DBUserEmailLookup 从 users 表按 user_id 解析邮箱
type DBUserEmailLookup struct {
	db *gorm.DB
}

// NewDBUserEmailLookup 创建邮箱查找器
func NewDBUserEmailLookup(db *gorm.DB) *DBUserEmailLookup {
	return &DBUserEmailLookup{db: db}
}

// GetUserEmail 返回用户主邮箱
func (l *DBUserEmailLookup) GetUserEmail(ctx context.Context, userID string) (string, error) {
	if l == nil || l.db == nil {
		return "", errors.New("email lookup not configured")
	}
	var email string
	// 兼容 user_id 列名
	err := l.db.WithContext(ctx).Table("users").
		Select("email").
		Where("user_id = ?", userID).
		Limit(1).
		Scan(&email).Error
	if err != nil {
		return "", err
	}
	if email == "" {
		return "", fmt.Errorf("email not found for user %s", userID)
	}
	return email, nil
}
