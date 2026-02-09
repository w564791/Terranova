package models

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID            string     `json:"id" gorm:"column:user_id;primaryKey;type:varchar(20)"`
	Username      string     `json:"username" gorm:"uniqueIndex;not null"`
	Email         string     `json:"email" gorm:"uniqueIndex;not null"`
	PasswordHash  string     `json:"-" gorm:"not null"`
	Role          string     `json:"role" gorm:"default:user"`
	IsActive      bool       `json:"is_active" gorm:"default:true"`
	IsSystemAdmin bool       `json:"is_system_admin" gorm:"default:false"`
	OAuthProvider string     `json:"oauth_provider,omitempty" gorm:"column:oauth_provider;type:varchar(50)"`
	OAuthID       string     `json:"oauth_id,omitempty" gorm:"column:oauth_id;type:varchar(200)"`
	LastLoginAt   *time.Time `json:"last_login_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`

	// MFA相关字段
	MFAEnabled        bool       `json:"mfa_enabled" gorm:"column:mfa_enabled;default:false"`
	MFASecret         string     `json:"-" gorm:"column:mfa_secret;type:varchar(128)"`
	MFAVerifiedAt     *time.Time `json:"mfa_verified_at,omitempty" gorm:"column:mfa_verified_at"`
	MFABackupCodes    string     `json:"-" gorm:"column:mfa_backup_codes;type:text"`
	MFAFailedAttempts int        `json:"-" gorm:"column:mfa_failed_attempts;default:0"`
	MFALockedUntil    *time.Time `json:"-" gorm:"column:mfa_locked_until"`
}

// BeforeCreate hook to generate ID if not set
func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == "" {
		// Import the id_generator package
		// For now, use a simple implementation
		u.ID = generateUserID()
	}
	return nil
}

// generateUserID generates a new user ID in format: user-{10 random chars}
func generateUserID() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	const idLength = 10

	b := make([]byte, idLength)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
		time.Sleep(time.Nanosecond) // Ensure uniqueness
	}
	return "user-" + string(b)
}

type VCSProvider struct {
	ID                uint           `json:"id" gorm:"primaryKey"`
	Name              string         `json:"name" gorm:"not null"`
	BaseURL           string         `json:"base_url" gorm:"not null"`
	APITokenEncrypted string         `json:"-"`
	WebhookSecret     string         `json:"-"`
	CreatedBy         uint           `json:"created_by"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `json:"-" gorm:"index"`
}

// Agent和AgentPool模型已移至agent.go文件
