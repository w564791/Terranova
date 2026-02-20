package models

import "time"

// TakeoverRequest 资源编辑接管请求
type TakeoverRequest struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	ResourceID       uint      `gorm:"not null;index:idx_takeover_resource_status" json:"resource_id"`
	RequesterUserID  string    `gorm:"type:varchar(255);not null" json:"requester_user_id"`                                                                                                                             // 请求接管的用户ID
	RequesterName    string    `gorm:"type:varchar(255);not null" json:"requester_name"`                                                                                                                                // 请求者用户名
	RequesterSession string    `gorm:"type:varchar(255);not null;index:idx_takeover_requester_session" json:"requester_session"`                                                                                        // 请求者的session_id
	TargetUserID     string    `gorm:"type:varchar(255);not null" json:"target_user_id"`                                                                                                                                // 被接管的用户ID
	TargetSession    string    `gorm:"type:varchar(255);not null;index:idx_takeover_target_session" json:"target_session"`                                                                                              // 被接管的session_id
	Status           string    `gorm:"type:varchar(50);not null;default:'pending';index:idx_takeover_target_session,idx_takeover_requester_session,idx_takeover_resource_status,idx_takeover_expires_at" json:"status"` // pending, approved, rejected, expired
	IsSameUser       bool      `gorm:"not null;default:false" json:"is_same_user"`                                                                                                                                      // 是否同一用户
	ExpiresAt        time.Time `gorm:"not null;index:idx_takeover_expires_at" json:"expires_at"`                                                                                                                        // 30秒后过期
	CreatedAt        time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt        time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

// TableName 指定表名
func (TakeoverRequest) TableName() string {
	return "takeover_requests"
}

// IsExpired 检查请求是否已过期
func (tr *TakeoverRequest) IsExpired() bool {
	return time.Now().After(tr.ExpiresAt)
}

// IsPending 检查请求是否待处理
func (tr *TakeoverRequest) IsPending() bool {
	return tr.Status == "pending" && !tr.IsExpired()
}
