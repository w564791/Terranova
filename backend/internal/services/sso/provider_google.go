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

// GoogleProvider Google OAuth Provider
type GoogleProvider struct {
	config      *models.OAuthConfigData
	providerCfg *models.SSOProvider
}

// NewGoogleProvider 创建 Google Provider
func NewGoogleProvider(providerCfg *models.SSOProvider) (*GoogleProvider, error) {
	var oauthConfig models.OAuthConfigData
	if err := json.Unmarshal(providerCfg.OAuthConfig, &oauthConfig); err != nil {
		return nil, fmt.Errorf("failed to parse oauth_config: %w", err)
	}
	return &GoogleProvider{
		config:      &oauthConfig,
		providerCfg: providerCfg,
	}, nil
}

func (p *GoogleProvider) GetType() string {
	return "google"
}

func (p *GoogleProvider) GetAuthorizationURL(state string, redirectURL string) (string, error) {
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
		"access_type":   {"offline"},
		"prompt":        {"consent"},
	}

	authorizeURL := "https://accounts.google.com/o/oauth2/v2/auth"
	if p.providerCfg.AuthorizeEndpoint != "" {
		authorizeURL = p.providerCfg.AuthorizeEndpoint
	}

	return authorizeURL + "?" + params.Encode(), nil
}

func (p *GoogleProvider) ExchangeCode(ctx context.Context, code string, redirectURL string) (*OAuthToken, error) {
	clientSecret, err := crypto.DecryptValue(p.config.ClientSecretEncrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt client_secret: %w", err)
	}

	tokenURL := "https://oauth2.googleapis.com/token"
	if p.providerCfg.TokenEndpoint != "" {
		tokenURL = p.providerCfg.TokenEndpoint
	}

	data := url.Values{
		"client_id":     {p.config.ClientID},
		"client_secret": {clientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURL},
		"grant_type":    {"authorization_code"},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
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
		return nil, fmt.Errorf("google token error: %s - %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	return &OAuthToken{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresIn:    tokenResp.ExpiresIn,
		IDToken:      tokenResp.IDToken,
	}, nil
}

func (p *GoogleProvider) GetUserInfo(ctx context.Context, token *OAuthToken) (*StandardUserInfo, error) {
	userinfoURL := "https://www.googleapis.com/oauth2/v3/userinfo"
	if p.providerCfg.UserinfoEndpoint != "" {
		userinfoURL = p.providerCfg.UserinfoEndpoint
	}

	req, err := http.NewRequestWithContext(ctx, "GET", userinfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)

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

	sub, _ := rawData["sub"].(string)
	email, _ := rawData["email"].(string)
	emailVerified, _ := rawData["email_verified"].(bool)
	name, _ := rawData["name"].(string)
	picture, _ := rawData["picture"].(string)

	return &StandardUserInfo{
		ProviderUserID: sub,
		Email:          email,
		EmailVerified:  emailVerified,
		Name:           name,
		Avatar:         picture,
		RawData:        rawData,
	}, nil
}
