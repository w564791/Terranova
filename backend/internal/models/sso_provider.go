package models

import (
	"encoding/json"
	"time"

	"github.com/lib/pq"
)

// SSOProvider SSO 身份提供商配置
type SSOProvider struct {
	ID int64 `json:"id" gorm:"primaryKey"`

	// 基本信息
	ProviderKey  string `json:"provider_key" gorm:"type:varchar(50);uniqueIndex;not null"`
	ProviderType string `json:"provider_type" gorm:"type:varchar(30);not null"`
	DisplayName  string `json:"display_name" gorm:"type:varchar(100);not null"`
	Description  string `json:"description" gorm:"type:text"`
	Icon         string `json:"icon" gorm:"type:varchar(50)"`

	// OAuth 配置（JSONB 存储，便于不同 Provider 的差异化配置）
	OAuthConfig json.RawMessage `json:"oauth_config" gorm:"column:oauth_config;type:jsonb;not null"`

	// 端点配置（可选，某些 Provider 需要自定义）
	AuthorizeEndpoint string `json:"authorize_endpoint" gorm:"type:varchar(500)"`
	TokenEndpoint     string `json:"token_endpoint" gorm:"type:varchar(500)"`
	UserinfoEndpoint  string `json:"userinfo_endpoint" gorm:"type:varchar(500)"`

	// 回调配置
	CallbackURL         string         `json:"callback_url" gorm:"type:varchar(500);not null"`
	AllowedCallbackURLs pq.StringArray `json:"allowed_callback_urls" gorm:"type:text[]"`

	// 用户管理配置
	AutoCreateUser bool           `json:"auto_create_user" gorm:"default:true"`
	DefaultRole    string         `json:"default_role" gorm:"type:varchar(50);default:user"`
	AllowedDomains pq.StringArray `json:"allowed_domains" gorm:"type:text[]"`

	// 属性映射
	AttributeMapping json.RawMessage `json:"attribute_mapping" gorm:"type:jsonb"`

	// 状态
	IsEnabled      bool   `json:"is_enabled" gorm:"default:true"`
	IsEnterprise   bool   `json:"is_enterprise" gorm:"default:false"`
	OrganizationID string `json:"organization_id" gorm:"type:varchar(20)"`

	// 显示
	DisplayOrder    int  `json:"display_order" gorm:"default:0"`
	ShowOnLoginPage bool `json:"show_on_login_page" gorm:"default:true"`

	// 审计
	CreatedBy string    `json:"created_by" gorm:"type:varchar(20)"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (SSOProvider) TableName() string {
	return "sso_providers"
}

// OAuthConfigData OAuth 配置结构（用于序列化/反序列化 oauth_config JSONB 字段）
type OAuthConfigData struct {
	ClientID              string   `json:"client_id"`
	ClientSecretEncrypted string   `json:"client_secret_encrypted"`
	Domain                string   `json:"domain,omitempty"`    // Auth0 特有
	Audience              string   `json:"audience,omitempty"`  // Auth0 特有
	TenantID              string   `json:"tenant_id,omitempty"` // Azure AD 特有
	OrgURL                string   `json:"org_url,omitempty"`   // Okta 特有
	Scopes                []string `json:"scopes,omitempty"`
}

// AttributeMappingData 属性映射结构
type AttributeMappingData struct {
	UserID string `json:"user_id"` // 默认 "sub"
	Email  string `json:"email"`   // 默认 "email"
	Name   string `json:"name"`    // 默认 "name"
	Avatar string `json:"avatar"`  // 默认 "picture"
}

// SSOProviderPublic 公开的 Provider 信息（用于登录页展示，不含敏感配置）
type SSOProviderPublic struct {
	ProviderKey  string `json:"provider_key"`
	ProviderType string `json:"provider_type"`
	DisplayName  string `json:"display_name"`
	Description  string `json:"description"`
	Icon         string `json:"icon"`
	DisplayOrder int    `json:"display_order"`
}

// ToPublic 转换为公开信息
func (p *SSOProvider) ToPublic() SSOProviderPublic {
	return SSOProviderPublic{
		ProviderKey:  p.ProviderKey,
		ProviderType: p.ProviderType,
		DisplayName:  p.DisplayName,
		Description:  p.Description,
		Icon:         p.Icon,
		DisplayOrder: p.DisplayOrder,
	}
}
