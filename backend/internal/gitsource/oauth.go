package gitsource

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// User-to-server OAuth of the GitHub App, used only to bind an installation
// to a platform organization (setup callback): the code GitHub hands to the
// callback is exchanged for a user access token, which proves that the GitHub
// user who installed the App administers the installation's account. The
// token is used for those few calls, revoked, and never stored or logged.

// ErrOAuthDisabled GITHUB_APP_CLIENT_ID / GITHUB_APP_CLIENT_SECRET not set.
var ErrOAuthDisabled = errors.New("GitHub App user authorization is not configured (GITHUB_APP_CLIENT_ID, GITHUB_APP_CLIENT_SECRET)")

// ErrOAuthCode the authorization code was rejected (expired, used, forged).
var ErrOAuthCode = errors.New("GitHub rejected the authorization code")

// ErrNotInstallationAdmin the GitHub user does not administer the account
// the installation belongs to.
var ErrNotInstallationAdmin = errors.New("the GitHub user is not an admin of the installation's account")

// OAuthClientCredentials the App's OAuth client (client id is not the App
// id). The secret is hidden from fmt.
type OAuthClientCredentials struct {
	ClientID string
	secret   string
}

func (OAuthClientCredentials) String() string     { return "gitsource.OAuthClientCredentials{***}" }
func (c OAuthClientCredentials) GoString() string { return c.String() }

// NewOAuthClientCredentials (tests and custom providers).
func NewOAuthClientCredentials(clientID, secret string) OAuthClientCredentials {
	return OAuthClientCredentials{ClientID: clientID, secret: secret}
}

// OAuthCredentialsFromEnv GITHUB_APP_CLIENT_ID / GITHUB_APP_CLIENT_SECRET
// (read on every call); ErrOAuthDisabled when either is missing.
func OAuthCredentialsFromEnv() (OAuthClientCredentials, error) {
	id := strings.TrimSpace(os.Getenv("GITHUB_APP_CLIENT_ID"))
	secret := strings.TrimSpace(os.Getenv("GITHUB_APP_CLIENT_SECRET"))
	if id == "" || secret == "" {
		return OAuthClientCredentials{}, ErrOAuthDisabled
	}
	return OAuthClientCredentials{ClientID: id, secret: secret}, nil
}

// GitHubUser the authenticated GitHub user of a user access token.
type GitHubUser struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

// OAuthApp performs the user-to-server calls. HTTP may be nil (30s timeout,
// no redirects).
type OAuthApp struct {
	Creds     OAuthClientCredentials
	Endpoints Endpoints
	HTTP      *http.Client
}

// NewOAuthApp from the environment.
func NewOAuthApp() (*OAuthApp, error) {
	c, err := OAuthCredentialsFromEnv()
	if err != nil {
		return nil, err
	}
	e, err := EndpointsFromEnv()
	if err != nil {
		return nil, err
	}
	return &OAuthApp{Creds: c, Endpoints: e}, nil
}

func (o *OAuthApp) client() *http.Client {
	if o.HTTP != nil {
		return o.HTTP
	}
	return noRedirectClient()
}

// send one request; the error never contains a token, secret or code.
func (o *OAuthApp) send(ctx context.Context, op, method, u string, setAuth func(*http.Request), body any, out any) error {
	var rd io.Reader
	contentType := ""
	switch b := body.(type) {
	case nil:
	case url.Values: // OAuth endpoints: standard form body
		rd, contentType = strings.NewReader(b.Encode()), "application/x-www-form-urlencoded"
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			return err
		}
		rd, contentType = bytes.NewReader(raw), "application/json"
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return fmt.Errorf("github %s: invalid request", op)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if setAuth != nil {
		setAuth(req)
	}
	resp, err := o.client().Do(req)
	if err != nil {
		return fmt.Errorf("github %s: request failed", op)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("github %s: %w", op, ErrNotFound)
	}
	if resp.StatusCode/100 != 2 {
		return &APIError{Op: op, Status: resp.StatusCode}
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
		return fmt.Errorf("github %s: invalid response", op)
	}
	return nil
}

func bearer(t *Token) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+t.Secret()) }
}

// ExchangeCode POST <web>/login/oauth/access_token: the code of the setup
// callback for a user access token (client secret in the body, never in the
// URL).
func (o *OAuthApp) ExchangeCode(ctx context.Context, code string) (*Token, error) {
	if code == "" || len(code) > 512 {
		return nil, ErrOAuthCode
	}
	var resp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
		Error       string `json:"error"`
	}
	if err := o.send(ctx, "exchange oauth code", http.MethodPost, o.Endpoints.WebURL+"/login/oauth/access_token", nil,
		url.Values{"client_id": {o.Creds.ClientID}, "client_secret": {o.Creds.secret}, "code": {code}}, &resp); err != nil {
		return nil, err
	}
	if resp.Error != "" || resp.AccessToken == "" {
		return nil, ErrOAuthCode // resp.Error is e.g. bad_verification_code; nothing secret, but not needed
	}
	exp := time.Time{}
	if resp.ExpiresIn > 0 {
		exp = time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second)
	}
	return NewToken(resp.AccessToken, exp), nil
}

// AuthenticatedUser GET /user with the user token.
func (o *OAuthApp) AuthenticatedUser(ctx context.Context, t *Token) (*GitHubUser, error) {
	var u GitHubUser
	if err := o.send(ctx, "get user", http.MethodGet, o.Endpoints.APIURL+"/user", bearer(t), nil, &u); err != nil {
		return nil, err
	}
	if u.ID <= 0 || u.Login == "" {
		return nil, errors.New("github get user: invalid response")
	}
	return &u, nil
}

// OrgMembership GET /user/memberships/orgs/{org} with the user token: the
// user's own membership (state "active" / "pending", role "admin" /
// "member"). ErrNotFound when the user is not a member. Needs the App's
// "Members" organization permission (read).
func (o *OAuthApp) OrgMembership(ctx context.Context, t *Token, org string) (state, role string, err error) {
	var m struct {
		State string `json:"state"`
		Role  string `json:"role"`
	}
	if err := o.send(ctx, "get org membership", http.MethodGet,
		o.Endpoints.APIURL+"/user/memberships/orgs/"+url.PathEscape(org), bearer(t), nil, &m); err != nil {
		return "", "", err
	}
	return m.State, m.Role, nil
}

// RevokeUserToken DELETE /applications/{client_id}/token (basic auth with the
// client credentials, token in the body). Best effort.
func (o *OAuthApp) RevokeUserToken(ctx context.Context, t *Token) {
	if t == nil || t.value == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	_ = o.send(ctx, "revoke user token", http.MethodDelete,
		o.Endpoints.APIURL+"/applications/"+url.PathEscape(o.Creds.ClientID)+"/token",
		func(r *http.Request) { r.SetBasicAuth(o.Creds.ClientID, o.Creds.secret) },
		map[string]string{"access_token": t.value}, nil)
}

// UserAPI the user-token calls VerifyInstallationAdmin needs.
type UserAPI interface {
	AuthenticatedUser(ctx context.Context, t *Token) (*GitHubUser, error)
	OrgMembership(ctx context.Context, t *Token, org string) (state, role string, err error)
}

// VerifyInstallationAdmin proves with the user token that its GitHub user
// may administer the installation's account: for an organization account an
// active admin membership, for a user account the same user (by id). inst
// must come from GitHub with the App JWT (GetInstallation), never from the
// request.
func VerifyInstallationAdmin(ctx context.Context, api UserAPI, t *Token, inst *Installation) (*GitHubUser, error) {
	if inst == nil || inst.AccountLogin == "" {
		return nil, ErrNotInstallationAdmin
	}
	u, err := api.AuthenticatedUser(ctx, t)
	if err != nil {
		return nil, err
	}
	switch inst.AccountType {
	case "User":
		if inst.AccountID > 0 && u.ID == inst.AccountID {
			return u, nil
		}
		return nil, ErrNotInstallationAdmin
	case "Organization":
		state, role, err := api.OrgMembership(ctx, t, inst.AccountLogin)
		if errors.Is(err, ErrNotFound) {
			return nil, ErrNotInstallationAdmin
		}
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusForbidden {
			return nil, ErrNotInstallationAdmin
		}
		if err != nil {
			return nil, err
		}
		if state == "active" && role == "admin" {
			return u, nil
		}
		return nil, ErrNotInstallationAdmin
	default: // Enterprise or unknown account types: not supported
		return nil, ErrNotInstallationAdmin
	}
}
