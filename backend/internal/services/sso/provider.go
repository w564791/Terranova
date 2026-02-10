package sso

import (
	"context"
	"iac-platform/internal/models"
)

// StandardUserInfo 标准化的用户信息（不同 Provider 返回的字段统一为此结构）
type StandardUserInfo struct {
	ProviderUserID string                 `json:"provider_user_id"`
	Email          string                 `json:"email"`
	EmailVerified  bool                   `json:"email_verified"`
	Name           string                 `json:"name"`
	Avatar         string                 `json:"avatar"`
	RawData        map[string]interface{} `json:"raw_data"`
}

// OAuthToken OAuth Token 信息
type OAuthToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	IDToken      string `json:"id_token,omitempty"`
}

// Provider SSO Provider 接口
type Provider interface {
	// GetType 返回 Provider 类型标识
	GetType() string

	// GetAuthorizationURL 生成授权 URL
	GetAuthorizationURL(state string, redirectURL string) (string, error)

	// ExchangeCode 用授权码换取 Token
	ExchangeCode(ctx context.Context, code string, redirectURL string) (*OAuthToken, error)

	// GetUserInfo 获取用户信息
	GetUserInfo(ctx context.Context, token *OAuthToken) (*StandardUserInfo, error)
}

// ProviderFactory Provider 工厂接口
type ProviderFactory interface {
	CreateProvider(config *models.SSOProvider) (Provider, error)
}
