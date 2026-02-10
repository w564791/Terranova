package models

import "time"

// SSOLoginLog SSO 登录日志
type SSOLoginLog struct {
	ID             int64  `json:"id" gorm:"primaryKey"`
	UserID         string `json:"user_id" gorm:"type:varchar(20);index"`
	IdentityID     *int64 `json:"identity_id"`
	ProviderKey    string `json:"provider_key" gorm:"type:varchar(50);not null;index"`
	ProviderUserID string `json:"provider_user_id" gorm:"type:varchar(255)"`
	ProviderEmail  string `json:"provider_email" gorm:"type:varchar(255)"`

	// 状态: success, failed, user_created, user_linked
	Status       string `json:"status" gorm:"type:varchar(20);not null"`
	ErrorMessage string `json:"error_message" gorm:"type:text"`

	// 请求信息
	IPAddress string `json:"ip_address" gorm:"type:varchar(45)"`
	UserAgent string `json:"user_agent" gorm:"type:text"`

	CreatedAt time.Time `json:"created_at"`
}

func (SSOLoginLog) TableName() string {
	return "sso_login_logs"
}
