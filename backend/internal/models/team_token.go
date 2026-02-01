package models

import (
	"time"
)

// TeamToken 团队Token模型
type TeamToken struct {
	TokenID     string     `json:"-" gorm:"-"`                                       // 不保存到数据库，不返回给前端
	TokenIDHash string     `json:"-" gorm:"column:token_id_hash;primaryKey;size:64"` // 使用hash作为主键
	TeamID      string     `json:"team_id" gorm:"column:team_id;not null"`
	TokenName   string     `json:"token_name" gorm:"column:token_name;not null"`
	TokenHash   string     `json:"-" gorm:"column:token_hash;not null;uniqueIndex"` // 不返回给前端
	IsActive    bool       `json:"is_active" gorm:"column:is_active;not null;default:true"`
	CreatedAt   time.Time  `json:"created_at" gorm:"column:created_at;not null"`
	CreatedBy   *string    `json:"created_by,omitempty" gorm:"column:created_by"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty" gorm:"column:revoked_at"`
	RevokedBy   *string    `json:"revoked_by,omitempty" gorm:"column:revoked_by"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty" gorm:"column:last_used_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty" gorm:"column:expires_at"`
}

// TableName 指定表名
func (TeamToken) TableName() string {
	return "team_tokens"
}

// TeamTokenResponse 团队Token响应（用于列表展示）
// 不返回token_id，只返回token_name用于识别
type TeamTokenResponse struct {
	TeamID     string     `json:"team_id"`
	TokenName  string     `json:"token_name"`
	IsActive   bool       `json:"is_active"`
	CreatedAt  time.Time  `json:"created_at"`
	CreatedBy  *string    `json:"created_by,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	RevokedBy  *string    `json:"revoked_by,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

// TeamTokenCreateResponse 创建Token时的响应（包含明文token）
type TeamTokenCreateResponse struct {
	TeamID    string     `json:"team_id"`
	TokenName string     `json:"token_name"`
	Token     string     `json:"token"` // 明文token，仅在创建时返回一次
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}
