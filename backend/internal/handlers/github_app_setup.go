package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"iac-platform/internal/application/service"
	"iac-platform/internal/domain/valueobject"
	"iac-platform/internal/gitsource"
	"iac-platform/internal/keys"
	"iac-platform/internal/middleware"
	"iac-platform/internal/models"
)

// GitHub App installation binding through the App's setup callback.
//
//  1. An org ADMIN calls POST /organizations/{org_id}/github-app/connect and
//     gets the App's install URL carrying a signed state (org, initiating
//     user, nonce; 10 minutes; HS256 with the "ghapp-state" purpose key
//     derived from SIGNING_ROOT_KEY).
//  2. GitHub installs the App and, with "Request user authorization (OAuth)
//     during installation", redirects the browser to the callback URL with
//     code, installation_id, setup_action and state.
//  3. The callback verifies the state (signature, expiry, single use), that
//     the initiating user is still org ADMIN, exchanges the code for a user
//     access token and proves with it that the GitHub user administers the
//     installation's account (active org admin / the same user), using the
//     installation as GitHub reports it to the App JWT. Only then the
//     installation is bound. The user token is revoked and never stored.
//
// The installation id in the callback URL is attacker-controlled; it is
// bound only with that proof, and an installation belongs to at most one
// organization (unique constraint).

const (
	githubStateTTL      = 10 * time.Minute
	githubStateAudience = "terranova/github-app-setup"
	// githubDeliveryTTL how long processed webhook delivery ids are kept.
	githubDeliveryTTL = 72 * time.Hour
	// githubSetupResultPath the frontend page the callback redirects to.
	githubSetupResultPath = "/admin/manifests"
)

// githubSetupNow is replaced in tests.
var githubSetupNow = time.Now

type githubSetupClaims struct {
	OrgID  int    `json:"org"`
	UserID string `json:"uid"`
	jwt.RegisteredClaims
}

var (
	errSetupStateInvalid = errors.New("invalid setup state")
	errSetupStateExpired = errors.New("setup state expired")
)

// signGitHubSetupState a state for (org, user): returns the token, its
// nonce (jti) and expiry.
func signGitHubSetupState(orgID int, userID string) (string, string, time.Time, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", time.Time{}, err
	}
	nonce := base64.RawURLEncoding.EncodeToString(b)
	now := githubSetupNow()
	exp := now.Add(githubStateTTL)
	tok, err := keys.Sign(keys.PurposeGitHubAppState, githubSetupClaims{
		OrgID: orgID, UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        nonce,
			Audience:  jwt.ClaimStrings{githubStateAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}, nil) // no legacy scheme: SIGNING_ROOT_KEY is required
	return tok, nonce, exp, err
}

// parseGitHubSetupState verifies signature (ghapp-state purpose key, kid
// required), audience and expiry.
func parseGitHubSetupState(state string) (*githubSetupClaims, error) {
	if state == "" || len(state) > 2048 {
		return nil, errSetupStateInvalid
	}
	var cl githubSetupClaims
	_, err := jwt.ParseWithClaims(state, &cl, keys.Keyfunc(keys.PurposeGitHubAppState, nil),
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithAudience(githubStateAudience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithTimeFunc(githubSetupNow))
	if errors.Is(err, jwt.ErrTokenExpired) {
		return nil, errSetupStateExpired
	}
	if err != nil || cl.OrgID <= 0 || cl.UserID == "" || cl.ID == "" || len(cl.ID) > 64 || cl.ExpiresAt == nil {
		return nil, errSetupStateInvalid
	}
	return &cl, nil
}

// consumeSetupNonce marks the state's nonce used; false when it already was
// (replay).
func consumeSetupNonce(db *gorm.DB, cl *githubSetupClaims) (bool, error) {
	res := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.GitHubAppSetupNonce{
		Nonce: cl.ID, OrganizationID: cl.OrgID, UserID: cl.UserID,
		ExpiresAt: cl.ExpiresAt.Time, ConsumedAt: githubSetupNow(),
	})
	return res.RowsAffected == 1, res.Error
}

var githubEphemera struct {
	mu   sync.Mutex
	last time.Time
}

// cleanupGitHubEphemera deletes webhook delivery ids older than 72h and
// setup nonces whose state expired (an expired state is rejected anyway).
func cleanupGitHubEphemera(db *gorm.DB, now time.Time) error {
	if err := db.Where("received_at < ?", now.Add(-githubDeliveryTTL)).Delete(&models.GitHubWebhookDelivery{}).Error; err != nil {
		return err
	}
	return db.Where("expires_at < ?", now).Delete(&models.GitHubAppSetupNonce{}).Error
}

// maybeCleanupGitHubEphemera at most every 10 minutes per process.
func maybeCleanupGitHubEphemera(db *gorm.DB) {
	now := time.Now()
	githubEphemera.mu.Lock()
	if now.Sub(githubEphemera.last) < 10*time.Minute {
		githubEphemera.mu.Unlock()
		return
	}
	githubEphemera.last = now
	githubEphemera.mu.Unlock()
	if err := cleanupGitHubEphemera(db, now); err != nil {
		log.Printf("[manifest-git] cleanup of webhook deliveries / setup nonces: %v", err)
	}
}

// WithPermissionChecker the IAM checker used to re-check, in the setup
// callback, that the initiating user is still org ADMIN.
func (h *GitHubAppHandler) WithPermissionChecker(pc service.PermissionChecker) *GitHubAppHandler {
	h.perm = pc
	return h
}

// Connect starts binding a GitHub App installation
// @Summary Connect a GitHub App installation
// @Description Returns the GitHub App install URL with a signed, single-use state (organization, initiating user, nonce; valid 10 minutes). Open it in the browser: after the installation GitHub redirects to the platform's setup callback, which binds the installation to the organization only after user-to-server OAuth proved that the installing GitHub user administers the installation's account. github_url is the configured GITHUB_URL (platform config, never from the request) so the client can check the install_url host. Requires ORGANIZATION ADMIN. 503 when the App, its OAuth client (GITHUB_APP_CLIENT_ID / GITHUB_APP_CLIENT_SECRET) or SIGNING_ROOT_KEY is not configured.
// @Tags Manifest Git
// @Produce json
// @Param org_id path string true "Organization ID"
// @Success 200 {object} map[string]interface{} "{install_url, expires_at, github_url}"
// @Failure 503 {object} map[string]interface{}
// @Router /api/v1/organizations/{org_id}/github-app/connect [post]
// @Security BearerAuth
func (h *GitHubAppHandler) Connect(c *gin.Context) {
	orgID, ok := middleware.AuthOrgID(c)
	userID := c.GetString("user_id")
	if !ok || userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "org_id is required"})
		return
	}
	app, err := gitDeps.App()
	if err != nil {
		respondGitError(c, "app", err)
		return
	}
	if _, err := gitDeps.OAuth(); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gitsource.ErrOAuthDisabled.Error(), "code": gitCodeDisabled})
		return
	}
	e, err := gitDeps.Endpoints()
	if err != nil {
		respondGitError(c, "endpoints", err)
		return
	}
	state, _, exp, err := signGitHubSetupState(int(orgID), userID)
	if err != nil {
		log.Printf("[manifest-git] sign setup state: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "SIGNING_ROOT_KEY is required to connect a GitHub App installation", "code": gitCodeDisabled})
		return
	}
	info, err := app.GetApp(c.Request.Context())
	if err != nil {
		respondGitError(c, "get app", err)
		return
	}
	installURL, err := gitsource.InstallURL(info, e, state)
	if err != nil {
		respondGitError(c, "install url", err)
		return
	}
	maybeCleanupGitHubEphemera(h.db)
	c.Header("Cache-Control", "no-store")
	// github_url comes from GITHUB_URL (platform config), never from the request:
	// the frontend uses its host to check install_url before navigating.
	c.JSON(http.StatusOK, gin.H{"install_url": installURL, "expires_at": exp.UTC(), "github_url": e.WebURL})
}

// setup callback outcomes (?github_app=<result>&reason=<code> on the frontend
// page; nothing secret, never the code or the state).
const (
	setupReasonStateInvalid    = "state_invalid"
	setupReasonStateExpired    = "state_expired"
	setupReasonStateUsed       = "state_used"
	setupReasonOAuthMissing    = "oauth_missing"
	setupReasonOAuthFailed     = "oauth_failed"
	setupReasonInstallation    = "installation_invalid"
	setupReasonNotAdmin        = "not_installation_admin"
	setupReasonForbidden       = "forbidden"
	setupReasonBoundElsewhere  = "installation_bound_elsewhere"
	setupReasonNotConfigured   = "not_configured"
	setupReasonInternal        = "internal_error"
	setupResultConnected       = "connected"
	setupResultRequested       = "requested"
	setupResultError           = "error"
	auditActionGitHubBind      = "github_installation.bind"
	auditActionGitHubBindStale = "github_installation.replace_unverified"
)

func setupRedirect(c *gin.Context, result, reason string, installationID int64) {
	q := url.Values{"github_app": {result}}
	if reason != "" {
		q.Set("reason", reason)
	}
	if installationID > 0 {
		q.Set("installation_id", strconv.FormatInt(installationID, 10))
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	c.Redirect(http.StatusFound, githubSetupResultPath+"?"+q.Encode())
}

func setupFail(c *gin.Context, reason string) { setupRedirect(c, setupResultError, reason, 0) }

// SetupCallback GitHub App setup / OAuth callback
// @Summary GitHub App setup callback
// @Description Callback URL of the GitHub App (with "Request user authorization (OAuth) during installation"). Verifies the signed state (purpose ghapp-state, 10 minutes, single use), re-checks that the initiating user is ORGANIZATION ADMIN, exchanges code for a user access token, and binds installation_id to the state's organization only if that GitHub user administers the installation's account (active org admin membership for organizations, the same user for user accounts; the installation is looked up with the App JWT). The user token is revoked, never stored. An installation verified for another organization is refused. Redirects (302) to /admin/manifests?github_app=connected|requested|error&reason=... . No platform login (the state is the authentication).
// @Tags Manifest Git
// @Param state query string true "Signed state from connect"
// @Param code query string false "OAuth code (user authorization during installation)"
// @Param installation_id query int false "Installation id"
// @Param setup_action query string false "install, update or request"
// @Success 302
// @Router /api/v1/github-app/setup/callback [get]
func (h *GitHubAppHandler) SetupCallback(c *gin.Context) {
	ctx := c.Request.Context()
	maybeCleanupGitHubEphemera(h.db)
	cl, err := parseGitHubSetupState(c.Query("state"))
	switch {
	case errors.Is(err, errSetupStateExpired):
		setupFail(c, setupReasonStateExpired)
		return
	case err != nil:
		setupFail(c, setupReasonStateInvalid)
		return
	}
	if c.Query("setup_action") == "request" {
		// a GitHub org member asked an owner to install; nothing to bind yet
		setupRedirect(c, setupResultRequested, "", 0)
		return
	}
	code := c.Query("code")
	if code == "" {
		setupFail(c, setupReasonOAuthMissing)
		return
	}
	instID, err := strconv.ParseInt(c.Query("installation_id"), 10, 64)
	if err != nil || instID <= 0 {
		setupFail(c, setupReasonInstallation)
		return
	}
	fresh, err := consumeSetupNonce(h.db.WithContext(ctx), cl)
	if err != nil {
		log.Printf("[manifest-git] consume setup nonce: %v", err)
		setupFail(c, setupReasonInternal)
		return
	}
	if !fresh {
		setupFail(c, setupReasonStateUsed)
		return
	}
	if ok, err := h.stillOrgAdmin(ctx, cl); err != nil || !ok {
		if err != nil {
			log.Printf("[manifest-git] setup callback permission check: %v", err)
		}
		setupFail(c, setupReasonForbidden)
		return
	}
	app, err := gitDeps.App()
	if err != nil {
		setupFail(c, setupReasonNotConfigured)
		return
	}
	oauth, err := gitDeps.OAuth()
	if err != nil {
		setupFail(c, setupReasonNotConfigured)
		return
	}
	userTok, err := oauth.ExchangeCode(ctx, code)
	if err != nil {
		log.Printf("[manifest-git] setup callback: %v", err)
		setupFail(c, setupReasonOAuthFailed)
		return
	}
	defer oauth.RevokeUserToken(ctx, userTok)
	inst, err := app.GetInstallation(ctx, instID)
	if err != nil {
		log.Printf("[manifest-git] setup callback get installation %d: %v", instID, err)
		setupFail(c, setupReasonInstallation)
		return
	}
	if inst.ID != instID || inst.AccountID <= 0 {
		setupFail(c, setupReasonInstallation)
		return
	}
	ghUser, err := gitsource.VerifyInstallationAdmin(ctx, oauth, userTok, inst)
	if err != nil {
		log.Printf("[manifest-git] setup callback: installation %d (%s %s): %v", instID, inst.AccountType, inst.AccountLogin, err)
		if errors.Is(err, gitsource.ErrNotInstallationAdmin) {
			setupFail(c, setupReasonNotAdmin)
		} else {
			setupFail(c, setupReasonOAuthFailed)
		}
		return
	}
	reason, err := h.bindInstallation(ctx, cl, inst, ghUser)
	if err != nil {
		log.Printf("[manifest-git] bind installation %d: %v", instID, err)
		setupFail(c, setupReasonInternal)
		return
	}
	if reason != "" {
		setupFail(c, reason)
		return
	}
	setupRedirect(c, setupResultConnected, "", instID)
}

// stillOrgAdmin re-checks ORGANIZATION ADMIN of the initiating user (fail
// closed without a checker).
func (h *GitHubAppHandler) stillOrgAdmin(ctx context.Context, cl *githubSetupClaims) (bool, error) {
	if h.perm == nil {
		return false, errors.New("no permission checker configured")
	}
	res, err := h.perm.CheckPermission(ctx, &service.CheckPermissionRequest{
		UserID:        cl.UserID,
		ResourceType:  valueobject.ResourceTypeOrgSettings,
		ScopeType:     valueobject.ScopeTypeOrganization,
		ScopeID:       uint(cl.OrgID),
		RequiredLevel: valueobject.PermissionLevelAdmin,
	})
	if err != nil {
		return false, err
	}
	return res != nil && res.IsAllowed, nil
}

// bindInstallation writes the verified binding. Returns a reason when it is
// refused (installation verified for another organization).
func (h *GitHubAppHandler) bindInstallation(ctx context.Context, cl *githubSetupClaims, inst *gitsource.Installation, u *gitsource.GitHubUser) (string, error) {
	now := githubSetupNow()
	accountID, accountType := inst.AccountID, inst.AccountType
	ghUserID, ghLogin := u.ID, u.Login
	var refused string
	replacedOrg := 0
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.GitHubAppInstallation
		err := tx.Where("installation_id = ?", inst.ID).Take(&existing).Error
		switch {
		case err == nil && existing.OrganizationID == cl.OrgID:
			return tx.Model(&models.GitHubAppInstallation{}).Where("id = ?", existing.ID).Updates(map[string]interface{}{
				"account_login": inst.AccountLogin, "account_id": accountID, "account_type": accountType,
				"verified_github_user_id": ghUserID, "verified_github_login": ghLogin, "verified_at": now,
			}).Error
		case err == nil && existing.VerifiedAt != nil:
			refused = setupReasonBoundElsewhere
			return nil
		case err == nil:
			// an unverified (manually registered) row of another organization
			// has no standing against a proven binding
			if err := tx.Delete(&models.GitHubAppInstallation{}, existing.ID).Error; err != nil {
				return err
			}
			replacedOrg = existing.OrganizationID
		case !errors.Is(err, gorm.ErrRecordNotFound):
			return err
		}
		row := models.GitHubAppInstallation{
			OrganizationID: cl.OrgID, InstallationID: inst.ID, AccountLogin: inst.AccountLogin,
			AccountID: &accountID, AccountType: &accountType,
			VerifiedGitHubUserID: &ghUserID, VerifiedGitHubLogin: &ghLogin, VerifiedAt: &now,
			CreatedBy: cl.UserID, CreatedAt: now,
		}
		if err := tx.Create(&row).Error; err != nil {
			// lost a race on the unique installation_id
			refused = setupReasonBoundElsewhere
			return errBindConflict
		}
		return nil
	})
	if errors.Is(err, errBindConflict) {
		return refused, nil
	}
	if err != nil {
		return "", err
	}
	if refused == "" && replacedOrg != 0 {
		writeManifestAudit(h.db, auditResourceGitHubInstallation, auditActionGitHubBindStale, cl.UserID, map[string]interface{}{
			"installation_id": inst.ID, "previous_organization_id": replacedOrg, "organization_id": cl.OrgID,
		})
	}
	if refused == "" {
		writeManifestAudit(h.db, auditResourceGitHubInstallation, auditActionGitHubBind, cl.UserID, map[string]interface{}{
			"organization_id": cl.OrgID, "installation_id": inst.ID, "account_login": inst.AccountLogin,
			"account_type": accountType, "github_user_id": ghUserID, "github_login": ghLogin,
		})
	}
	return refused, nil
}

var errBindConflict = errors.New("installation already bound")

// UsableInstallation the read-only view of a bound installation for manifest
// authors: the installation id and the account name only.
type UsableInstallation struct {
	ID      int64  `json:"id"`
	Account string `json:"account"`
}

// loadBoundInstallation the installation path parameter, bound (verified)
// to the caller's organization; 404 otherwise (also for another org's id).
func (h *GitHubAppHandler) loadBoundInstallation(c *gin.Context) (*models.GitHubAppInstallation, bool) {
	orgID, ok := middleware.AuthOrgID(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "org_id is required"})
		return nil, false
	}
	instID, err := strconv.ParseInt(c.Param("installation_id"), 10, 64)
	if err != nil || instID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid installation_id"})
		return nil, false
	}
	var row models.GitHubAppInstallation
	err = h.db.Where("organization_id = ? AND installation_id = ? AND verified_at IS NOT NULL", orgID, instID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "installation not found"})
		return nil, false
	}
	if err != nil {
		_ = c.Error(err)
		return nil, false
	}
	return &row, true
}

// ListUsableInstallations installations a manifest author can pick
// @Summary List usable GitHub App installations
// @Description Installations bound (verified) to the organization, for the git manifest create form: only id (installation id) and account. Requires MANIFESTS WRITE.
// @Tags Manifest Git
// @Produce json
// @Param org_id path string true "Organization ID"
// @Success 200 {object} map[string]interface{} "{installations: [{id, account}]}"
// @Router /api/v1/organizations/{org_id}/github-app/available-installations [get]
// @Security BearerAuth
func (h *GitHubAppHandler) ListUsableInstallations(c *gin.Context) {
	orgID, ok := middleware.AuthOrgID(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "org_id is required"})
		return
	}
	var rows []models.GitHubAppInstallation
	if err := h.db.Where("organization_id = ? AND verified_at IS NOT NULL", orgID).Order("id").Find(&rows).Error; err != nil {
		_ = c.Error(err)
		return
	}
	out := make([]UsableInstallation, 0, len(rows))
	for _, r := range rows {
		out = append(out, UsableInstallation{ID: r.InstallationID, Account: r.AccountLogin})
	}
	c.JSON(http.StatusOK, gin.H{"installations": out})
}

// GetUsableInstallation one usable installation
// @Summary Get a usable GitHub App installation
// @Description id and account of an installation bound to the organization; 404 for unknown, unverified or another organization's installations. Requires MANIFESTS WRITE.
// @Tags Manifest Git
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param installation_id path int true "GitHub installation id"
// @Success 200 {object} handlers.UsableInstallation
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/organizations/{org_id}/github-app/available-installations/{installation_id} [get]
// @Security BearerAuth
func (h *GitHubAppHandler) GetUsableInstallation(c *gin.Context) {
	row, ok := h.loadBoundInstallation(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, UsableInstallation{ID: row.InstallationID, Account: row.AccountLogin})
}

// ListInstallationRepositories repositories of a bound installation
// @Summary List repositories of a GitHub App installation
// @Description Repositories the installation can access (full_name, default_branch, private, html_url), one page. Uses a freshly minted installation token with only metadata:read, revoked right after. html_url is built from the configured GITHUB_URL. With q (trimmed, at most 100 characters, else 400 repo_query_too_long) the installation's repositories are filtered server-side by a case-insensitive substring of full_name: up to 1000 repositories are scanned (truncated=true when the installation has more), page/per_page page through the matches and total_count counts the matches. 404 for an installation not bound to the organization. Requires MANIFESTS WRITE.
// @Tags Manifest Git
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param installation_id path int true "GitHub installation id"
// @Param page query int false "Page (default 1)"
// @Param per_page query int false "1..100, default 30"
// @Param q query string false "Filter: case-insensitive substring of owner/name (max 100 characters)"
// @Success 200 {object} map[string]interface{} "{repositories: [...], total_count, page, per_page, truncated, q?}"
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 503 {object} map[string]interface{}
// @Router /api/v1/organizations/{org_id}/github-app/installations/{installation_id}/repositories [get]
// @Security BearerAuth
func (h *GitHubAppHandler) ListInstallationRepositories(c *gin.Context) {
	row, ok := h.loadBoundInstallation(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "30"))
	if page < 1 || page > 1000 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 30
	}
	q := strings.TrimSpace(c.Query("q"))
	if utf8.RuneCountInString(q) > repoSearchMaxLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("q must be at most %d characters", repoSearchMaxLen), "code": "repo_query_too_long"})
		return
	}
	app, err := gitDeps.App()
	if err != nil {
		respondGitError(c, "app", err)
		return
	}
	ctx := c.Request.Context()
	tok, err := app.MintMetadataToken(ctx, row.InstallationID)
	if err != nil {
		respondGitError(c, "mint metadata token", err)
		return
	}
	defer app.Revoke(context.WithoutCancel(ctx), tok)

	if q == "" {
		repos, total, err := app.ListInstallationRepos(ctx, tok, page, perPage)
		if err != nil {
			respondGitError(c, "list installation repositories", err)
			return
		}
		if repos == nil {
			repos = []gitsource.RepoInfo{}
		}
		c.JSON(http.StatusOK, gin.H{"repositories": repos, "total_count": total, "page": page, "per_page": perPage, "truncated": false})
		return
	}

	matches, truncated, err := searchInstallationRepos(ctx, app, tok, q)
	if err != nil {
		respondGitError(c, "search installation repositories", err)
		return
	}
	start := (page - 1) * perPage
	if start > len(matches) {
		start = len(matches)
	}
	end := start + perPage
	if end > len(matches) {
		end = len(matches)
	}
	c.JSON(http.StatusOK, gin.H{"repositories": matches[start:end], "total_count": len(matches), "page": page, "per_page": perPage, "q": q, "truncated": truncated})
}

// Repository search (q) limits.
const (
	repoSearchMaxLen  = 100  // characters, after trimming
	repoSearchMaxScan = 1000 // repositories scanned per search (10 GitHub pages of 100)
	repoSearchPage    = 100  // GitHub's maximum per_page
)

// searchInstallationRepos filters the installation's repositories by a
// case-insensitive substring of full_name.
//
// GitHub's /installation/repositories has no search parameter. The search
// API (/search/repositories with user:/org:<account>) was rejected: it also
// returns public repositories of the account that the installation was NOT
// granted (selected-repositories installs), so results would not match what
// create/publish accept; it is rate limited to 30 requests/minute, its index
// lags behind new repositories, and q would have to be sanitized against
// search qualifiers (org:other ...). Filtering the installation's own list
// with the same metadata:read token gives exactly the repositories the
// installation can access. At most repoSearchMaxScan repositories are
// scanned (truncated=true when the installation has more); page/per_page
// then page through the matches, and total_count is the number of matches.
func searchInstallationRepos(ctx context.Context, app GitApp, tok *gitsource.Token, q string) ([]gitsource.RepoInfo, bool, error) {
	needle := strings.ToLower(q)
	matches := []gitsource.RepoInfo{}
	scanned := 0
	for p := 1; ; p++ {
		repos, total, err := app.ListInstallationRepos(ctx, tok, p, repoSearchPage)
		if err != nil {
			return nil, false, err
		}
		scanned += len(repos)
		for _, r := range repos {
			if strings.Contains(strings.ToLower(r.FullName), needle) {
				matches = append(matches, r)
			}
		}
		if len(repos) < repoSearchPage || scanned >= total {
			return matches, false, nil
		}
		if scanned >= repoSearchMaxScan {
			return matches, true, nil
		}
	}
}
