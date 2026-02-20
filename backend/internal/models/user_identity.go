package models

import (
	"encoding/json"
	"time"
)

// UserIdentity 用户身份关联（支持多 Provider 绑定）
type UserIdentity struct {
	ID int64 `json:"id" gorm:"primaryKey"`

	// 用户关联
	UserID string `json:"user_id" gorm:"type:varchar(20);not null;index"`

	// Provider 信息
	Provider       string `json:"provider" gorm:"type:varchar(50);not null"`
	ProviderUserID string `json:"provider_user_id" gorm:"type:varchar(255);not null"`
	ProviderEmail  string `json:"provider_email" gorm:"type:varchar(255)"`
	ProviderName   string `json:"provider_name" gorm:"type:varchar(255)"`
	ProviderAvatar string `json:"provider_avatar" gorm:"type:varchar(500)"`

	// 元数据
	RawData               json.RawMessage `json:"-" gorm:"type:jsonb"`
	AccessTokenEncrypted  string          `json:"-" gorm:"type:text"`
	RefreshTokenEncrypted string          `json:"-" gorm:"type:text"`
	TokenExpiresAt        *time.Time      `json:"-"`

	// 状态
	IsPrimary  bool       `json:"is_primary" gorm:"default:false"`
	IsVerified bool       `json:"is_verified" gorm:"default:true"`
	LastUsedAt *time.Time `json:"last_used_at"`

	// 审计
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// 关联
	User *User `json:"user,omitempty" gorm:"foreignKey:UserID;references:ID"`
}

func (UserIdentity) TableName() string {
	return "user_identities"
}
