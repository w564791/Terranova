package models

import "time"

// SSOState OAuth2 state 参数存储（CSRF 防护）
type SSOState struct {
	State       string    `json:"state" gorm:"primaryKey;type:varchar(64)"`
	ProviderKey string    `json:"provider_key" gorm:"type:varchar(50);not null"`
	RedirectURL string    `json:"redirect_url" gorm:"type:varchar(500);default:/"`
	Action      string    `json:"action" gorm:"type:varchar(10);not null;default:login"`
	UserID      string    `json:"user_id" gorm:"type:varchar(20);default:''"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at" gorm:"not null;index"`
}

func (SSOState) TableName() string {
	return "sso_states"
}
