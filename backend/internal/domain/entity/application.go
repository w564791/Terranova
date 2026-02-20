package entity

import (
	"time"
)

// Application 应用实体（Agent/外部系统）
type Application struct {
	ID           uint                   `json:"id"`
	OrgID        uint                   `json:"org_id"`        // 所属组织ID
	Name         string                 `json:"name"`          // 应用名称
	AppKey       string                 `json:"app_key"`       // API Key
	AppSecret    string                 `json:"-"`             // API Secret（加密存储，不返回）
	Description  string                 `json:"description"`   // 描述
	CallbackURLs map[string]interface{} `json:"callback_urls"` // 回调URL列表
	IsActive     bool                   `json:"is_active"`     // 是否启用
	CreatedBy    *string                `json:"created_by"`    // 创建人user_id
	CreatedAt    time.Time              `json:"created_at"`    // 创建时间
	ExpiresAt    *time.Time             `json:"expires_at"`    // 过期时间
	LastUsedAt   *time.Time             `json:"last_used_at"`  // 最后使用时间

	// 关联
	Organization *Organization `json:"organization,omitempty" gorm:"foreignKey:OrgID"`
}

// TableName 指定表名
func (Application) TableName() string {
	return "applications"
}

// IsValid 验证应用数据是否有效
func (a *Application) IsValid() bool {
	return a.OrgID > 0 && a.Name != "" && a.AppKey != ""
}

// IsExpired 判断应用是否过期
func (a *Application) IsExpired() bool {
	if a.ExpiresAt == nil {
		return false
	}
	return a.ExpiresAt.Before(time.Now())
}

// UpdateLastUsed 更新最后使用时间
func (a *Application) UpdateLastUsed() {
	now := time.Now()
	a.LastUsedAt = &now
}
