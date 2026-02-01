package models

import (
	"time"
)

// UserToken 用户个人Token模型
type UserToken struct {
	TokenID     string     `json:"-" gorm:"-"`                                       // 不保存到数据库，不返回给前端
	TokenIDHash string     `json:"-" gorm:"column:token_id_hash;primaryKey;size:64"` // 使用hash作为主键
	UserID      string     `json:"user_id" gorm:"type:varchar(20);not null;index"`
	TokenName   string     `json:"token_name" gorm:"not null;size:100"`
	TokenHash   string     `json:"-" gorm:"column:token_hash;not null;uniqueIndex;size:255"` // 不返回给前端
	IsActive    bool       `json:"is_active" gorm:"default:true;index"`
	CreatedAt   time.Time  `json:"created_at"`
	RevokedAt   *time.Time `json:"revoked_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	ExpiresAt   *time.Time `json:"expires_at"`
}

// TableName 指定表名
func (UserToken) TableName() string {
	return "user_tokens"
}

// UserTokenResponse 用户Token响应（用于列表展示）
// 不返回token_id，只返回token_name用于识别
type UserTokenResponse struct {
	UserID     string     `json:"user_id"`
	TokenName  string     `json:"token_name"`
	IsActive   bool       `json:"is_active"`
	CreatedAt  time.Time  `json:"created_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

// UserTokenCreateResponse 创建Token时的响应（包含明文token）
type UserTokenCreateResponse struct {
	TokenID   string     `json:"token_id"`
	UserID    string     `json:"user_id"`
	TokenName string     `json:"token_name"`
	Token     string     `json:"token"` // 明文token，仅在创建时返回一次
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}
