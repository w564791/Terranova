package sso

import (
	"fmt"

	"iac-platform/internal/models"
)

// DefaultProviderFactory 默认 Provider 工厂
type DefaultProviderFactory struct{}

// NewProviderFactory 创建工厂实例
func NewProviderFactory() *DefaultProviderFactory {
	return &DefaultProviderFactory{}
}

// CreateProvider 根据配置创建对应的 Provider 实例
func (f *DefaultProviderFactory) CreateProvider(config *models.SSOProvider) (Provider, error) {
	switch config.ProviderType {
	case "github":
		return NewGitHubProvider(config)
	case "google":
		return NewGoogleProvider(config)
	case "auth0", "azure_ad", "okta", "oidc":
		return NewOIDCProvider(config)
	default:
		// 未知类型尝试用通用 OIDC Provider
		return NewOIDCProvider(config)
	}
}

// SupportedProviderTypes 返回支持的 Provider 类型列表
func SupportedProviderTypes() []string {
	return []string{
		"github",
		"google",
		"auth0",
		"azure_ad",
		"okta",
		"oidc",
	}
}

// IsProviderTypeSupported 检查 Provider 类型是否支持
func IsProviderTypeSupported(providerType string) bool {
	for _, t := range SupportedProviderTypes() {
		if t == providerType {
			return true
		}
	}
	return false
}

// ValidateProviderConfig 验证 Provider 配置的基本完整性
func ValidateProviderConfig(config *models.SSOProvider) error {
	if config.ProviderKey == "" {
		return fmt.Errorf("provider_key is required")
	}
	if config.ProviderType == "" {
		return fmt.Errorf("provider_type is required")
	}
	if config.DisplayName == "" {
		return fmt.Errorf("display_name is required")
	}
	if config.CallbackURL == "" {
		return fmt.Errorf("callback_url is required")
	}
	if len(config.OAuthConfig) == 0 {
		return fmt.Errorf("oauth_config is required")
	}
	return nil
}
