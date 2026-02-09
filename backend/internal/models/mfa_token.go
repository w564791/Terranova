package models

import (
	"time"
)

// MFAToken MFA临时令牌，用于登录时的两步验证
type MFAToken struct {
	ID        uint       `json:"id" gorm:"primaryKey"`
	Token     string     `json:"token" gorm:"uniqueIndex;not null;type:varchar(64)"`
	UserID    string     `json:"user_id" gorm:"column:user_id;not null;type:varchar(20)"`
	IPAddress string     `json:"ip_address" gorm:"column:ip_address;type:varchar(45)"`
	CreatedAt time.Time  `json:"created_at" gorm:"default:now()"`
	ExpiresAt time.Time  `json:"expires_at" gorm:"not null"`
	Used      bool       `json:"used" gorm:"default:false"`
	UsedAt    *time.Time `json:"used_at,omitempty" gorm:"column:used_at"`
}

// TableName 指定表名
func (MFAToken) TableName() string {
	return "mfa_tokens"
}

// IsExpired 检查令牌是否已过期
func (t *MFAToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// IsValid 检查令牌是否有效（未过期且未使用）
func (t *MFAToken) IsValid() bool {
	return !t.IsExpired() && !t.Used
}

// MFABackupCode 备用恢复码结构
type MFABackupCode struct {
	Code   string     `json:"code"`
	Used   bool       `json:"used"`
	UsedAt *time.Time `json:"used_at,omitempty"`
}

// MFAConfig MFA全局配置
type MFAConfig struct {
	Enabled                bool       `json:"enabled"`
	Enforcement            string     `json:"enforcement"` // optional, required_new, required_all
	EnforcementEnabledAt   *time.Time `json:"enforcement_enabled_at,omitempty"`
	Issuer                 string     `json:"issuer"`
	GracePeriodDays        int        `json:"grace_period_days"`
	MaxFailedAttempts      int        `json:"max_failed_attempts"`
	LockoutDurationMinutes int        `json:"lockout_duration_minutes"`
	RequiredBackupCodes    int        `json:"required_backup_codes"` // 使用备用码登录时需要输入的数量，默认1
}

// MFAStatus 用户MFA状态
type MFAStatus struct {
	MFAEnabled        bool       `json:"mfa_enabled"`
	MFAVerifiedAt     *time.Time `json:"mfa_verified_at,omitempty"`
	BackupCodesCount  int        `json:"backup_codes_count"`
	EnforcementPolicy string     `json:"enforcement_policy"`
	IsRequired        bool       `json:"is_required"`
	IsLocked          bool       `json:"is_locked"`
	LockedUntil       *time.Time `json:"locked_until,omitempty"`
}

// MFASetupResponse MFA设置响应
type MFASetupResponse struct {
	Secret      string   `json:"secret"`
	QRCode      string   `json:"qr_code"`
	OTPAuthURI  string   `json:"otpauth_uri"`
	BackupCodes []string `json:"backup_codes"`
}

// MFAStatistics MFA使用统计
type MFAStatistics struct {
	TotalUsers      int `json:"total_users"`
	MFAEnabledUsers int `json:"mfa_enabled_users"`
	MFAPendingUsers int `json:"mfa_pending_users"`
}
