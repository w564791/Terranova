package sso

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"iac-platform/internal/crypto"
	"iac-platform/internal/models"
)

// GitHubProvider GitHub OAuth Provider
type GitHubProvider struct {
	config      *models.OAuthConfigData
	providerCfg *models.SSOProvider
}

// NewGitHubProvider 创建 GitHub Provider
func NewGitHubProvider(providerCfg *models.SSOProvider) (*GitHubProvider, error) {
	var oauthConfig models.OAuthConfigData
	if err := json.Unmarshal(providerCfg.OAuthConfig, &oauthConfig); err != nil {
		return nil, fmt.Errorf("failed to parse oauth_config: %w", err)
	}
	return &GitHubProvider{
		config:      &oauthConfig,
		providerCfg: providerCfg,
	}, nil
}

func (p *GitHubProvider) GetType() string {
	return "github"
}

func (p *GitHubProvider) GetAuthorizationURL(state string, redirectURL string) (string, error) {
	scopes := p.config.Scopes
	if len(scopes) == 0 {
		scopes = []string{"user:email", "read:user"}
	}

	params := url.Values{
		"client_id":    {p.config.ClientID},
		"redirect_uri": {redirectURL},
		"scope":        {strings.Join(scopes, " ")},
		"state":        {state},
	}

	authorizeURL := "https://github.com/login/oauth/authorize"
	if p.providerCfg.AuthorizeEndpoint != "" {
		authorizeURL = p.providerCfg.AuthorizeEndpoint
	}

	return authorizeURL + "?" + params.Encode(), nil
}

func (p *GitHubProvider) ExchangeCode(ctx context.Context, code string, redirectURL string) (*OAuthToken, error) {
	// 解密 client_secret
	clientSecret, err := crypto.DecryptValue(p.config.ClientSecretEncrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt client_secret: %w", err)
	}

	tokenURL := "https://github.com/login/oauth/access_token"
	if p.providerCfg.TokenEndpoint != "" {
		tokenURL = p.providerCfg.TokenEndpoint
	}

	data := url.Values{
		"client_id":     {p.config.ClientID},
		"client_secret": {clientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURL},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

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
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	if tokenResp.Error != "" {
		return nil, fmt.Errorf("github token error: %s - %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	return &OAuthToken{
		AccessToken: tokenResp.AccessToken,
		TokenType:   tokenResp.TokenType,
	}, nil
}

func (p *GitHubProvider) GetUserInfo(ctx context.Context, token *OAuthToken) (*StandardUserInfo, error) {
	userinfoURL := "https://api.github.com/user"
	if p.providerCfg.UserinfoEndpoint != "" {
		userinfoURL = p.providerCfg.UserinfoEndpoint
	}

	req, err := http.NewRequestWithContext(ctx, "GET", userinfoURL, nil)
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

	// GitHub 的 user_id 是数字类型
	var providerUserID string
	if id, ok := rawData["id"]; ok {
		switch v := id.(type) {
		case float64:
			providerUserID = strconv.FormatInt(int64(v), 10)
		case string:
			providerUserID = v
		}
	}

	email, _ := rawData["email"].(string)
	name, _ := rawData["name"].(string)
	avatar, _ := rawData["avatar_url"].(string)

	// 如果 login 存在但 name 为空，使用 login 作为 name
	if name == "" {
		if login, ok := rawData["login"].(string); ok {
			name = login
		}
	}

	// GitHub 邮箱可能为 null，需要额外请求 /user/emails
	if email == "" {
		email, _ = p.fetchPrimaryEmail(ctx, token)
	}

	return &StandardUserInfo{
		ProviderUserID: providerUserID,
		Email:          email,
		EmailVerified:  email != "", // GitHub 验证过的邮箱才会返回
		Name:           name,
		Avatar:         avatar,
		RawData:        rawData,
	}, nil
}

// fetchPrimaryEmail 获取 GitHub 用户的主邮箱
func (p *GitHubProvider) fetchPrimaryEmail(ctx context.Context, token *OAuthToken) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user/emails", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.Unmarshal(body, &emails); err != nil {
		return "", err
	}

	// 优先返回 primary + verified 的邮箱
	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}
	// 其次返回任何 verified 的邮箱
	for _, e := range emails {
		if e.Verified {
			return e.Email, nil
		}
	}
	// 最后返回第一个邮箱
	if len(emails) > 0 {
		return emails[0].Email, nil
	}

	return "", nil
}
