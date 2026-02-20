package sso

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"iac-platform/internal/crypto"
	"iac-platform/internal/models"
)

// OIDCProvider 通用 OIDC Provider（支持 Auth0, Azure AD, Okta 等）
type OIDCProvider struct {
	config      *models.OAuthConfigData
	providerCfg *models.SSOProvider
	endpoints   oidcEndpoints
}

type oidcEndpoints struct {
	Authorize string
	Token     string
	UserInfo  string
}

// NewOIDCProvider 创建通用 OIDC Provider
func NewOIDCProvider(providerCfg *models.SSOProvider) (*OIDCProvider, error) {
	var oauthConfig models.OAuthConfigData
	if err := json.Unmarshal(providerCfg.OAuthConfig, &oauthConfig); err != nil {
		return nil, fmt.Errorf("failed to parse oauth_config: %w", err)
	}

	endpoints := resolveEndpoints(providerCfg, &oauthConfig)

	return &OIDCProvider{
		config:      &oauthConfig,
		providerCfg: providerCfg,
		endpoints:   endpoints,
	}, nil
}

// resolveEndpoints 根据 Provider 类型和配置解析端点 URL
func resolveEndpoints(cfg *models.SSOProvider, oauthCfg *models.OAuthConfigData) oidcEndpoints {
	ep := oidcEndpoints{}

	// 优先使用显式配置的端点
	if cfg.AuthorizeEndpoint != "" {
		ep.Authorize = cfg.AuthorizeEndpoint
	}
	if cfg.TokenEndpoint != "" {
		ep.Token = cfg.TokenEndpoint
	}
	if cfg.UserinfoEndpoint != "" {
		ep.UserInfo = cfg.UserinfoEndpoint
	}

	// 根据 Provider 类型推断默认端点
	switch cfg.ProviderType {
	case "auth0":
		domain := oauthCfg.Domain
		if domain != "" {
			if !strings.HasPrefix(domain, "https://") {
				domain = "https://" + domain
			}
			if ep.Authorize == "" {
				ep.Authorize = domain + "/authorize"
			}
			if ep.Token == "" {
				ep.Token = domain + "/oauth/token"
			}
			if ep.UserInfo == "" {
				ep.UserInfo = domain + "/userinfo"
			}
		}
	case "azure_ad":
		tenantID := oauthCfg.TenantID
		if tenantID == "" {
			tenantID = "common"
		}
		base := "https://login.microsoftonline.com/" + tenantID + "/oauth2/v2.0"
		if ep.Authorize == "" {
			ep.Authorize = base + "/authorize"
		}
		if ep.Token == "" {
			ep.Token = base + "/token"
		}
		if ep.UserInfo == "" {
			ep.UserInfo = "https://graph.microsoft.com/v1.0/me"
		}
	case "okta":
		orgURL := oauthCfg.OrgURL
		if orgURL != "" {
			if !strings.HasPrefix(orgURL, "https://") {
				orgURL = "https://" + orgURL
			}
			base := orgURL + "/oauth2/default/v1"
			if ep.Authorize == "" {
				ep.Authorize = base + "/authorize"
			}
			if ep.Token == "" {
				ep.Token = base + "/token"
			}
			if ep.UserInfo == "" {
				ep.UserInfo = base + "/userinfo"
			}
		}
	}

	return ep
}

func (p *OIDCProvider) GetType() string {
	return p.providerCfg.ProviderType
}

func (p *OIDCProvider) GetAuthorizationURL(state string, redirectURL string) (string, error) {
	if p.endpoints.Authorize == "" {
		return "", fmt.Errorf("authorize endpoint not configured for provider %s", p.providerCfg.ProviderKey)
	}

	scopes := p.config.Scopes
	if len(scopes) == 0 {
		scopes = []string{"openid", "profile", "email"}
	}

	params := url.Values{
		"client_id":     {p.config.ClientID},
		"redirect_uri":  {redirectURL},
		"response_type": {"code"},
		"scope":         {strings.Join(scopes, " ")},
		"state":         {state},
	}

	// Auth0 特有参数
	if p.providerCfg.ProviderType == "auth0" && p.config.Audience != "" {
		params.Set("audience", p.config.Audience)
	}

	return p.endpoints.Authorize + "?" + params.Encode(), nil
}

func (p *OIDCProvider) ExchangeCode(ctx context.Context, code string, redirectURL string) (*OAuthToken, error) {
	if p.endpoints.Token == "" {
		return nil, fmt.Errorf("token endpoint not configured for provider %s", p.providerCfg.ProviderKey)
	}

	clientSecret, err := crypto.DecryptValue(p.config.ClientSecretEncrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt client_secret: %w", err)
	}

	data := url.Values{
		"client_id":     {p.config.ClientID},
		"client_secret": {clientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURL},
		"grant_type":    {"authorization_code"},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.endpoints.Token, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		IDToken      string `json:"id_token"`
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	if tokenResp.Error != "" {
		return nil, fmt.Errorf("oidc token error: %s - %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	return &OAuthToken{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresIn:    tokenResp.ExpiresIn,
		IDToken:      tokenResp.IDToken,
	}, nil
}

func (p *OIDCProvider) GetUserInfo(ctx context.Context, token *OAuthToken) (*StandardUserInfo, error) {
	if p.endpoints.UserInfo == "" {
		return nil, fmt.Errorf("userinfo endpoint not configured for provider %s", p.providerCfg.ProviderKey)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", p.endpoints.UserInfo, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read userinfo response: %w", err)
	}

	var rawData map[string]interface{}
	if err := json.Unmarshal(body, &rawData); err != nil {
		return nil, fmt.Errorf("failed to parse userinfo response: %w", err)
	}

	// 使用属性映射提取字段
	mapping := p.getAttributeMapping()

	providerUserID := extractString(rawData, mapping.UserID)
	email := extractString(rawData, mapping.Email)
	name := extractString(rawData, mapping.Name)
	avatar := extractString(rawData, mapping.Avatar)
	emailVerified, _ := rawData["email_verified"].(bool)

	// Azure AD 特殊处理：user_id 字段是 oid 而不是 sub
	if p.providerCfg.ProviderType == "azure_ad" && providerUserID == "" {
		providerUserID = extractString(rawData, "oid")
		if providerUserID == "" {
			providerUserID = extractString(rawData, "id")
		}
	}

	// Azure AD 的 displayName 和 mail 字段
	if p.providerCfg.ProviderType == "azure_ad" {
		if name == "" {
			name = extractString(rawData, "displayName")
		}
		if email == "" {
			email = extractString(rawData, "mail")
			if email == "" {
				email = extractString(rawData, "userPrincipalName")
			}
		}
	}

	return &StandardUserInfo{
		ProviderUserID: providerUserID,
		Email:          email,
		EmailVerified:  emailVerified,
		Name:           name,
		Avatar:         avatar,
		RawData:        rawData,
	}, nil
}

// getAttributeMapping 获取属性映射配置
func (p *OIDCProvider) getAttributeMapping() models.AttributeMappingData {
	mapping := models.AttributeMappingData{
		UserID: "sub",
		Email:  "email",
		Name:   "name",
		Avatar: "picture",
	}

	if p.providerCfg.AttributeMapping != nil {
		var custom models.AttributeMappingData
		if err := json.Unmarshal(p.providerCfg.AttributeMapping, &custom); err == nil {
			if custom.UserID != "" {
				mapping.UserID = custom.UserID
			}
			if custom.Email != "" {
				mapping.Email = custom.Email
			}
			if custom.Name != "" {
				mapping.Name = custom.Name
			}
			if custom.Avatar != "" {
				mapping.Avatar = custom.Avatar
			}
		}
	}

	return mapping
}

// extractString 从 map 中提取字符串值，支持嵌套路径（用 . 分隔）
func extractString(data map[string]interface{}, key string) string {
	if key == "" {
		return ""
	}

	// 简单 key
	if !strings.Contains(key, ".") {
		if v, ok := data[key]; ok {
			switch val := v.(type) {
			case string:
				return val
			case float64:
				return fmt.Sprintf("%.0f", val)
			}
		}
		return ""
	}

	// 嵌套 key（如 "profile.name"）
	parts := strings.SplitN(key, ".", 2)
	if nested, ok := data[parts[0]].(map[string]interface{}); ok {
		return extractString(nested, parts[1])
	}
	return ""
}
