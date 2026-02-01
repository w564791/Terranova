package models

import "time"

// LoginSession 登录会话模型
type LoginSession struct {
	SessionID  string     `gorm:"column:session_id;primaryKey" json:"session_id"`
	UserID     string     `gorm:"column:user_id;not null" json:"user_id"`
	Username   string     `gorm:"column:username;not null" json:"username"`
	CreatedAt  time.Time  `gorm:"column:created_at;not null" json:"created_at"`
	ExpiresAt  time.Time  `gorm:"column:expires_at;not null" json:"expires_at"`
	LastUsedAt *time.Time `gorm:"column:last_used_at" json:"last_used_at,omitempty"`
	IPAddress  string     `gorm:"column:ip_address" json:"ip_address,omitempty"`
	UserAgent  string     `gorm:"column:user_agent" json:"user_agent,omitempty"`
	IsActive   bool       `gorm:"column:is_active;not null;default:true" json:"is_active"`
	RevokedAt  *time.Time `gorm:"column:revoked_at" json:"revoked_at,omitempty"`
}

// TableName 指定表名
func (LoginSession) TableName() string {
	return "login_sessions"
}
