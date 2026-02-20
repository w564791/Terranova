package models

import (
	"time"
)

// ResourceLock 资源锁状态
type ResourceLock struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	ResourceID    uint      `gorm:"not null;uniqueIndex:idx_resource_lock_resource_session,priority:1" json:"resource_id"`
	EditingUserID string    `gorm:"type:varchar(20);not null" json:"editing_user_id"`
	SessionID     string    `gorm:"type:varchar(100);not null;uniqueIndex:idx_resource_lock_resource_session,priority:2" json:"session_id"`
	LockType      string    `gorm:"type:varchar(20);not null;default:'optimistic'" json:"lock_type"` // optimistic, pessimistic
	Version       int       `gorm:"not null;default:1" json:"version"`
	LastHeartbeat time.Time `gorm:"not null" json:"last_heartbeat"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	// 关联
	Resource    WorkspaceResource `gorm:"foreignKey:ResourceID" json:"resource,omitempty"`
	EditingUser User              `gorm:"foreignKey:EditingUserID" json:"editing_user,omitempty"`
}

// TableName 指定表名
func (ResourceLock) TableName() string {
	return "resource_locks"
}

// IsExpired 检查锁是否过期（1分钟无心跳）
// 注意：使用本地时间进行比较，因为数据库列是 timestamp without time zone
func (r *ResourceLock) IsExpired() bool {
	return time.Now().Sub(r.LastHeartbeat) > 1*time.Minute
}

// IsWarning 检查是否需要警告（30秒无心跳）
// 注意：使用本地时间进行比较，因为数据库列是 timestamp without time zone
func (r *ResourceLock) IsWarning() bool {
	return time.Now().Sub(r.LastHeartbeat) > 30*time.Second
}

// ResourceDrift 用户编辑草稿状态
type ResourceDrift struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	ResourceID    uint      `gorm:"not null;index:idx_resource_drift_resource" json:"resource_id"`
	UserID        string    `gorm:"type:varchar(20);not null;index:idx_resource_drift_user" json:"user_id"`
	SessionID     string    `gorm:"type:varchar(100);not null" json:"session_id"`
	DriftContent  JSONB     `gorm:"type:jsonb;not null" json:"drift_content"`
	BaseVersion   int       `gorm:"not null" json:"base_version"`
	Status        string    `gorm:"type:varchar(20);not null;default:'active';index:idx_resource_drift_status" json:"status"` // active, expired, submitted
	LastHeartbeat time.Time `gorm:"not null" json:"last_heartbeat"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	// 关联
	Resource WorkspaceResource `gorm:"foreignKey:ResourceID" json:"resource,omitempty"`
	User     User              `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

// TableName 指定表名
func (ResourceDrift) TableName() string {
	return "resource_drifts"
}

// IsExpired 检查草稿是否过期（1分钟无心跳）
// 注意：使用本地时间进行比较，因为数据库列是 timestamp without time zone
func (r *ResourceDrift) IsExpired() bool {
	return time.Now().Sub(r.LastHeartbeat) > 1*time.Minute
}

// IsActive 检查草稿是否活跃
func (r *ResourceDrift) IsActive() bool {
	return r.Status == "active" && !r.IsExpired()
}
