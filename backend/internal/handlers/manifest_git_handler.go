package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"iac-platform/internal/application/service"
	"iac-platform/internal/gitsource"
	"iac-platform/internal/manifestbundle"
	"iac-platform/internal/middleware"
	"iac-platform/internal/models"
)

// GitApp the GitHub App operations used by the git-source handlers
// (*gitsource.GitHubApp; a fake in tests).
type GitApp interface {
	gitsource.TokenMinter
	gitsource.InstallationLookup
	ListBranches(ctx context.Context, repo gitsource.Repo, t *gitsource.Token) ([]gitsource.Branch, error)
	ListCommits(ctx context.Context, repo gitsource.Repo, ref string, perPage int, t *gitsource.Token) ([]gitsource.Commit, error)
	Revoke(ctx context.Context, t *gitsource.Token)
	GetApp(ctx context.Context) (*gitsource.AppInfo, error)
	MintMetadataToken(ctx context.Context, installationID int64) (*gitsource.Token, error)
	ListInstallationRepos(ctx context.Context, t *gitsource.Token, page, perPage int) ([]gitsource.RepoInfo, int, error)
}

// GitHubOAuth the user-to-server OAuth calls of the setup callback
// (*gitsource.OAuthApp; a fake in tests). The user token is used for the
// proof only, revoked, never stored.
type GitHubOAuth interface {
	gitsource.UserAPI
	ExchangeCode(ctx context.Context, code string) (*gitsource.Token, error)
	RevokeUserToken(ctx context.Context, t *gitsource.Token)
}

// GitSourceDeps resolves the GitHub App, the git fetcher, the endpoints and
// the webhook secret. Each call re-reads the configuration (env); an
// unconfigured App yields gitsource.ErrDisabled.
type GitSourceDeps struct {
	App           func() (GitApp, error)
	Fetcher       func() (*gitsource.Fetcher, error)
	Endpoints     func() (gitsource.Endpoints, error)
	WebhookSecret func() ([]byte, error)
	// OAuth the App's OAuth client (GITHUB_APP_CLIENT_ID / _SECRET);
	// gitsource.ErrOAuthDisabled when not configured.
	OAuth func() (GitHubOAuth, error)
}

// gitDeps is replaced in tests (fake GitHub API / local git server).
var gitDeps = GitSourceDeps{
	App: func() (GitApp, error) {
		app, err := gitsource.NewGitHubApp()
		if err != nil {
			return nil, err
		}
		return app, nil
	},
	Fetcher: func() (*gitsource.Fetcher, error) {
		if _, err := gitsource.Credentials.AppCredentials(); err != nil {
			return nil, err
		}
		e, err := gitsource.EndpointsFromEnv()
		if err != nil {
			return nil, err
		}
		return &gitsource.Fetcher{Endpoints: e}, nil
	},
	Endpoints:     gitsource.EndpointsFromEnv,
	WebhookSecret: func() ([]byte, error) { return gitsource.Credentials.WebhookSecret() },
	OAuth: func() (GitHubOAuth, error) {
		o, err := gitsource.NewOAuthApp()
		if err != nil {
			return nil, err
		}
		return o, nil
	},
}

// errInstallationNotBound the manifest's installation is no longer bound
// (verified) to the manifest's organization.
var errInstallationNotBound = errors.New("the GitHub App installation is not connected to this organization")

// Git-source error codes (JSON "code").
const (
	gitCodeDisabled           = "git_source_disabled"
	gitCodeRepoNotAccessible  = "git_repo_not_accessible"
	gitCodeCommitNotFound     = "git_commit_not_found"
	gitCodeSubpathNotFound    = "git_subpath_not_found"
	gitCodeSubpathInvalid     = "git_subpath_invalid"
	gitCodeSourceImmutable    = "git_source_immutable"
	gitCodeFetchFailed        = "git_fetch_failed"
	gitCodeNotGitSource       = "not_git_source"
	gitCodeReadOnly           = "git_source_read_only"
	gitCodeInstallationNotReg = "github_installation_not_registered"
)

// installationBound whether installationID is bound to orgID through the
// setup callback (verified). Unverified (manually registered) rows do not
// count.
func installationBound(db *gorm.DB, orgID int, installationID int64) (bool, error) {
	var n int64
	err := db.Model(&models.GitHubAppInstallation{}).
		Where("organization_id = ? AND installation_id = ? AND verified_at IS NOT NULL", orgID, installationID).
		Count(&n).Error
	return n > 0, err
}

// respondGitError maps git-source errors to HTTP. Details from git are
// already token-scrubbed; they are logged, never returned.
func respondGitError(c *gin.Context, op string, err error) {
	var apiErr *gitsource.APIError
	switch {
	case errors.Is(err, gitsource.ErrDisabled):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error(), "code": gitCodeDisabled})
	case errors.Is(err, errInstallationNotBound):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error() + " (an organization admin connects it through the GitHub App setup)", "code": gitCodeInstallationNotReg})
	case errors.Is(err, gitsource.ErrNotFound), errors.Is(err, gitsource.ErrAuth),
		errors.As(err, &apiErr) && (apiErr.Status == http.StatusUnauthorized || apiErr.Status == http.StatusForbidden || apiErr.Status == http.StatusUnprocessableEntity):
		log.Printf("[manifest-git] %s: %v", op, err)
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "the repository is not accessible to the GitHub App installation", "code": gitCodeRepoNotAccessible})
	case errors.Is(err, gitsource.ErrCommitNotFound):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "commit not found in the repository", "code": gitCodeCommitNotFound})
	case errors.Is(err, gitsource.ErrSubpathNotFound):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "git_subpath does not exist in the commit", "code": gitCodeSubpathNotFound})
	default:
		log.Printf("[manifest-git] %s: %v", op, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "git operation failed", "code": gitCodeFetchFailed})
	}
}

// gitManifestRepo the parsed repo of a git manifest.
func gitManifestRepo(m *models.Manifest) (gitsource.Repo, gitsource.Endpoints, error) {
	e, err := gitDeps.Endpoints()
	if err != nil {
		return gitsource.Repo{}, e, err
	}
	if m.SourceType != models.ManifestSourceGit || m.GitRepoURL == nil || m.GitHubInstallationID == nil {
		return gitsource.Repo{}, e, errors.New("manifest is not git-sourced")
	}
	repo, err := gitsource.ParseRepoURL(*m.GitRepoURL, e)
	return repo, e, err
}

// withRepoToken mints a token for the manifest's repo (single repo,
// contents:read), runs fn and revokes the token. The installation must still
// be bound (verified) to the manifest's organization.
func withRepoToken(ctx context.Context, db *gorm.DB, m *models.Manifest, fn func(app GitApp, repo gitsource.Repo, tok *gitsource.Token) error) error {
	repo, _, err := gitManifestRepo(m)
	if err != nil {
		return err
	}
	bound, err := installationBound(db.WithContext(ctx), m.OrganizationID, *m.GitHubInstallationID)
	if err != nil {
		return err
	}
	if !bound {
		return errInstallationNotBound
	}
	app, err := gitDeps.App()
	if err != nil {
		return err
	}
	tok, err := app.MintRepoToken(ctx, *m.GitHubInstallationID, repo)
	if err != nil {
		return err
	}
	defer app.Revoke(ctx, tok)
	return fn(app, repo, tok)
}

// fetchGitCommit fetches the pinned commit of a git manifest (publish).
func fetchGitCommit(ctx context.Context, db *gorm.DB, m *models.Manifest, sha string) (*gitsource.Tree, error) {
	fetcher, err := gitDeps.Fetcher()
	if err != nil {
		return nil, err
	}
	subpath := ""
	if m.GitSubpath != nil {
		subpath = *m.GitSubpath
	}
	var tree *gitsource.Tree
	err = withRepoToken(ctx, db, m, func(_ GitApp, repo gitsource.Repo, tok *gitsource.Token) error {
		var ferr error
		tree, ferr = fetcher.Fetch(ctx, repo, sha, subpath, tok)
		return ferr
	})
	if err != nil {
		return nil, err
	}
	for i := range tree.Files {
		tree.Files[i].Mime, tree.Files[i].IsBinary = sniffContent(tree.Files[i].Path, tree.Files[i].Content)
	}
	return tree, nil
}

// gitCommitProblems merges the tree-level problems of a fetched commit with
// the publish rules of its files.
func gitCommitProblems(tree *gitsource.Tree, ruleProblems []manifestbundle.Problem) []manifestbundle.Problem {
	if len(tree.Problems) == 0 {
		return ruleProblems
	}
	return manifestbundle.SortProblems(append(append([]manifestbundle.Problem{}, tree.Problems...), ruleProblems...))
}

// validateGitManifestCreate checks a git manifest create request: App
// configured, repo URL on the GitHub host, installation registered for the
// org and owning the repo, repo reachable with a scoped token.
func validateGitManifestCreate(c *gin.Context, db *gorm.DB, orgID int, req *models.CreateManifestRequest, m *models.Manifest) bool {
	e, err := gitDeps.Endpoints()
	if err != nil {
		respondGitError(c, "endpoints", err)
		return false
	}
	if _, err := gitDeps.App(); err != nil {
		respondGitError(c, "app", err)
		return false
	}
	// git_repo (owner/name) is preferred; git_repo_url is kept for
	// compatibility and must be on the configured host. Either way the host
	// comes from GITHUB_URL, never from the request.
	var repo gitsource.Repo
	switch {
	case req.GitRepo != "" && req.GitRepoURL != "":
		c.JSON(http.StatusBadRequest, gin.H{"error": "give git_repo or git_repo_url, not both"})
		return false
	case req.GitRepo != "":
		repo, err = gitsource.ParseRepoFullName(req.GitRepo)
	default:
		repo, err = gitsource.ParseRepoURL(req.GitRepoURL, e)
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return false
	}
	subpath, err := gitsource.CleanSubpath(req.GitSubpath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": gitCodeSubpathInvalid})
		return false
	}
	if req.GitHubInstallationID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "github_installation_id is required for a git manifest"})
		return false
	}
	var inst models.GitHubAppInstallation
	if err := db.Where("organization_id = ? AND installation_id = ? AND verified_at IS NOT NULL", orgID, req.GitHubInstallationID).Take(&inst).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "the GitHub App installation is not connected to this organization (an organization admin connects it through the GitHub App setup)", "code": gitCodeInstallationNotReg})
		} else {
			_ = c.Error(err)
		}
		return false
	}
	if !strings.EqualFold(inst.AccountLogin, repo.Owner) {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "the repository owner is not the account of the GitHub App installation", "code": gitCodeRepoNotAccessible})
		return false
	}
	url := repo.URL(e)
	instID := req.GitHubInstallationID
	m.SourceType = models.ManifestSourceGit
	m.GitRepoURL = &url
	m.GitHubInstallationID = &instID
	if subpath != "" {
		m.GitSubpath = &subpath
	}
	// the repo must be reachable with a token scoped to it
	if err := withRepoToken(c.Request.Context(), db, m, func(GitApp, gitsource.Repo, *gitsource.Token) error { return nil }); err != nil {
		respondGitError(c, "verify repository", err)
		return false
	}
	return true
}

// ManifestNativeOnly refuses draft writes on git manifests: their editor is
// read-only, changes go through git and publish = pick a commit.
func ManifestNativeOnly(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var st struct{ SourceType string }
		if err := db.Model(&models.Manifest{}).Select("source_type").Where("id = ?", c.Param("id")).Take(&st).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "manifest not found"})
			} else {
				_ = c.Error(err)
				c.Abort()
			}
			return
		}
		if st.SourceType == models.ManifestSourceGit {
			c.AbortWithStatusJSON(http.StatusConflict, gin.H{
				"error": "git manifests are read-only here: change the repository and publish a commit",
				"code":  gitCodeReadOnly,
			})
			return
		}
		c.Next()
	}
}

// ManifestGitHandler the commit picker of git manifests.
type ManifestGitHandler struct{ db *gorm.DB }

func NewManifestGitHandler(db *gorm.DB) *ManifestGitHandler { return &ManifestGitHandler{db: db} }

func (h *ManifestGitHandler) loadGitManifest(c *gin.Context) (*models.Manifest, bool) {
	var m models.Manifest
	if err := h.db.Where("id = ?", c.Param("id")).Take(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "manifest not found"})
		} else {
			_ = c.Error(err)
		}
		return nil, false
	}
	if m.SourceType != models.ManifestSourceGit {
		c.JSON(http.StatusConflict, gin.H{"error": "manifest is not git-sourced", "code": gitCodeNotGitSource})
		return nil, false
	}
	return &m, true
}

// ListBranches branches of a git manifest's repository
// @Summary List git branches of a manifest
// @Description Branches (name, head sha) of the repository of a git-sourced manifest, read with a per-request GitHub App installation token (single repo, contents:read, revoked after use). Requires MANIFESTS WRITE (commit picker of publish). 409 not_git_source for native manifests; 503 git_source_disabled when the GitHub App is not configured; 422 git_repo_not_accessible.
// @Tags Manifest Git
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Success 200 {object} map[string]interface{} "{branches: [{name, sha}]}"
// @Failure 409 {object} map[string]interface{}
// @Failure 422 {object} map[string]interface{}
// @Failure 503 {object} map[string]interface{}
// @Router /api/v1/organizations/{org_id}/manifests/{id}/git/branches [get]
// @Security BearerAuth
func (h *ManifestGitHandler) ListBranches(c *gin.Context) {
	m, ok := h.loadGitManifest(c)
	if !ok {
		return
	}
	var branches []gitsource.Branch
	err := withRepoToken(c.Request.Context(), h.db, m, func(app GitApp, repo gitsource.Repo, tok *gitsource.Token) error {
		var err error
		branches, err = app.ListBranches(c.Request.Context(), repo, tok)
		return err
	})
	if err != nil {
		respondGitError(c, "list branches", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"branches": branches})
}

// ListCommits recent commits of a git manifest's repository
// @Summary List git commits of a manifest
// @Description Recent commits (sha, subject, author_name, author_date; newest first) of a branch / ref of the repository of a git-sourced manifest, for the publish commit picker. Same token and errors as the branch list. Requires MANIFESTS WRITE.
// @Tags Manifest Git
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Param ref query string false "Branch, tag or commit (default: the repository default branch)"
// @Param per_page query int false "1..100, default 30"
// @Success 200 {object} map[string]interface{} "{commits: [{sha, subject, author_name, author_date}]}"
// @Failure 400 {object} map[string]interface{}
// @Failure 409 {object} map[string]interface{}
// @Failure 422 {object} map[string]interface{}
// @Failure 503 {object} map[string]interface{}
// @Router /api/v1/organizations/{org_id}/manifests/{id}/git/commits [get]
// @Security BearerAuth
func (h *ManifestGitHandler) ListCommits(c *gin.Context) {
	ref := c.Query("ref")
	if len(ref) > 255 || strings.IndexFunc(ref, func(r rune) bool { return unicode.IsControl(r) || r == ' ' }) >= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ref"})
		return
	}
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "30"))
	m, ok := h.loadGitManifest(c)
	if !ok {
		return
	}
	var commits []gitsource.Commit
	err := withRepoToken(c.Request.Context(), h.db, m, func(app GitApp, repo gitsource.Repo, tok *gitsource.Token) error {
		var err error
		commits, err = app.ListCommits(c.Request.Context(), repo, ref, perPage, tok)
		return err
	})
	if err != nil {
		respondGitError(c, "list commits", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"commits": commits})
}

const auditResourceGitHubInstallation = "GITHUB_APP_INSTALLATION"

// GitHubAppHandler org-level GitHub App installations (org ADMIN) and the
// App's setup callback (github_app_setup.go).
type GitHubAppHandler struct {
	db   *gorm.DB
	perm service.PermissionChecker
}

func NewGitHubAppHandler(db *gorm.DB) *GitHubAppHandler { return &GitHubAppHandler{db: db} }

// ListInstallations GitHub App installations of the organization
// @Summary List GitHub App installations
// @Description GitHub App installations of the organization (installation_id, account_login, account_type, verified_github_login, verified_at). Only rows with verified_at (bound through the setup callback) are usable by git manifests; rows without it were registered manually by an earlier build and must be connected again. Requires ORGANIZATION ADMIN.
// @Tags Manifest Git
// @Produce json
// @Param org_id path string true "Organization ID"
// @Success 200 {object} map[string]interface{} "{installations: [models.GitHubAppInstallation]}"
// @Router /api/v1/organizations/{org_id}/github-app/installations [get]
// @Security BearerAuth
func (h *GitHubAppHandler) ListInstallations(c *gin.Context) {
	orgID, ok := middleware.AuthOrgID(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "org_id is required"})
		return
	}
	var rows []models.GitHubAppInstallation
	if err := h.db.Where("organization_id = ?", orgID).Order("id").Find(&rows).Error; err != nil {
		_ = c.Error(err)
		return
	}
	if rows == nil {
		rows = []models.GitHubAppInstallation{}
	}
	c.JSON(http.StatusOK, gin.H{"installations": rows})
}

// RegisterInstallation manual registration is disabled
// @Summary Register GitHub App installation (removed)
// @Description Removed: an installation id alone proves nothing about who controls it. Installations are bound only through the GitHub App setup callback (POST /organizations/{org_id}/github-app/connect). Always 410. Requires ORGANIZATION ADMIN.
// @Tags Manifest Git
// @Produce json
// @Param org_id path string true "Organization ID"
// @Failure 410 {object} map[string]interface{}
// @Router /api/v1/organizations/{org_id}/github-app/installations [post]
// @Security BearerAuth
func (h *GitHubAppHandler) RegisterInstallation(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{
		"error": "manual installation registration is disabled; use POST /organizations/{org_id}/github-app/connect",
		"code":  "github_installation_manual_registration_removed",
	})
}

// DeleteInstallation unregisters a GitHub App installation
// @Summary Unregister GitHub App installation
// @Description Remove a registered installation. 409 while git manifests of the organization use it. Does not uninstall the App on GitHub. Requires ORGANIZATION ADMIN.
// @Tags Manifest Git
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param installation_id path int true "GitHub installation id"
// @Success 204
// @Failure 404 {object} map[string]interface{}
// @Failure 409 {object} map[string]interface{}
// @Router /api/v1/organizations/{org_id}/github-app/installations/{installation_id} [delete]
// @Security BearerAuth
func (h *GitHubAppHandler) DeleteInstallation(c *gin.Context) {
	orgID, ok := middleware.AuthOrgID(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "org_id is required"})
		return
	}
	instID, err := strconv.ParseInt(c.Param("installation_id"), 10, 64)
	if err != nil || instID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid installation_id"})
		return
	}
	var inUse int64
	if err := h.db.Model(&models.Manifest{}).
		Where("organization_id = ? AND source_type = ? AND github_installation_id = ?", orgID, models.ManifestSourceGit, instID).
		Count(&inUse).Error; err != nil {
		_ = c.Error(err)
		return
	}
	if inUse > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("%d git manifest(s) use this installation", inUse)})
		return
	}
	res := h.db.Where("organization_id = ? AND installation_id = ?", orgID, instID).Delete(&models.GitHubAppInstallation{})
	if res.Error != nil {
		_ = c.Error(res.Error)
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "installation not registered"})
		return
	}
	writeManifestAudit(h.db, auditResourceGitHubInstallation, "github_installation.delete", c.GetString("user_id"), map[string]interface{}{
		"organization_id": orgID, "installation_id": instID,
	})
	c.Status(http.StatusNoContent)
}

// GitHubWebhookHandler receives GitHub App webhooks.
type GitHubWebhookHandler struct{ db *gorm.DB }

func NewGitHubWebhookHandler(db *gorm.DB) *GitHubWebhookHandler { return &GitHubWebhookHandler{db: db} }

const maxWebhookBody = 5 << 20

type githubPushEvent struct {
	Ref        string `json:"ref"`
	After      string `json:"after"`
	Deleted    bool   `json:"deleted"`
	Repository struct {
		HTMLURL string `json:"html_url"`
	} `json:"repository"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
}

// Receive GitHub App webhook (push => "new commit available")
// @Summary GitHub App webhook
// @Description Receives GitHub App webhooks. The raw body must carry a valid X-Hub-Signature-256 (HMAC-SHA256 with GITHUB_WEBHOOK_SECRET, constant-time compare); unsigned or invalid => 401, no secret configured => 503. A push to a repository of git manifests (matching installation and repository) records git_latest_sha / git_latest_ref / git_latest_at on them as a "new commit available" hint. It never publishes. Other events are acknowledged and ignored. Each X-GitHub-Delivery is processed once (recorded for 72h): a replayed delivery is a 200 no-op ({duplicate: true}); a missing / malformed delivery id is 400. No user authentication (the signature is the authentication).
// @Tags Manifest Git
// @Accept json
// @Produce json
// @Param X-Hub-Signature-256 header string true "sha256=<hex HMAC>"
// @Param X-GitHub-Event header string true "Event name"
// @Param X-GitHub-Delivery header string true "Delivery GUID (replay protection)"
// @Success 200 {object} map[string]interface{}
// @Success 202 {object} map[string]interface{} "{matched: n}"
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 413 {object} map[string]interface{}
// @Failure 503 {object} map[string]interface{}
// @Router /api/v1/webhooks/github [post]
func (h *GitHubWebhookHandler) Receive(c *gin.Context) {
	secret, err := gitDeps.WebhookSecret()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "GitHub webhook is not configured"})
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, maxWebhookBody))
	if err != nil {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "payload too large"})
		return
	}
	if !gitsource.VerifySignature(secret, body, c.GetHeader("X-Hub-Signature-256")) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or missing signature"})
		return
	}
	// replay protection: each X-GitHub-Delivery is processed once. The id is
	// recorded in the same transaction as the effect, so a delivery that
	// failed here (5xx) can be redelivered.
	delivery := strings.TrimSpace(c.GetHeader("X-GitHub-Delivery"))
	if !validDeliveryID(delivery) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing or invalid X-GitHub-Delivery"})
		return
	}
	event := c.GetHeader("X-GitHub-Event")
	if len(event) > 64 {
		event = event[:64]
	}
	maybeCleanupGitHubEphemera(h.db)
	status, resp := http.StatusOK, gin.H{}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		ins := tx.Clauses(clause.OnConflict{DoNothing: true}).
			Create(&models.GitHubWebhookDelivery{DeliveryID: delivery, Event: event, ReceivedAt: time.Now()})
		if ins.Error != nil {
			return ins.Error
		}
		if ins.RowsAffected == 0 {
			status, resp = http.StatusOK, gin.H{"duplicate": true}
			return nil
		}
		status, resp = h.handleEvent(tx, event, body)
		if status >= 500 {
			return errWebhookFailed
		}
		return nil
	})
	if err != nil && !errors.Is(err, errWebhookFailed) {
		_ = c.Error(err)
		return
	}
	c.JSON(status, resp)
}

var errWebhookFailed = errors.New("webhook processing failed")

// validDeliveryID X-GitHub-Delivery is a GUID; accept 1..64 of [A-Za-z0-9-].
func validDeliveryID(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if !(r == '-' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return true
}

// handleEvent the effect of one (new) verified delivery, inside the
// delivery's transaction.
func (h *GitHubWebhookHandler) handleEvent(tx *gorm.DB, event string, body []byte) (int, gin.H) {
	switch event {
	case "ping":
		return http.StatusOK, gin.H{"ok": true}
	case "push":
	default:
		return http.StatusAccepted, gin.H{"ignored": true}
	}
	var ev githubPushEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		return http.StatusBadRequest, gin.H{"error": "invalid push payload"}
	}
	if ev.Deleted || !gitsource.IsCommitSHA(ev.After) || ev.Installation.ID <= 0 || len(ev.Ref) > 255 || !strings.HasPrefix(ev.Ref, "refs/") {
		return http.StatusAccepted, gin.H{"ignored": true}
	}
	e, err := gitDeps.Endpoints()
	if err != nil {
		log.Printf("[manifest-git] webhook endpoints: %v", err)
		return http.StatusInternalServerError, gin.H{"error": "internal error"}
	}
	repo, err := gitsource.ParseRepoURL(ev.Repository.HTMLURL, e)
	if err != nil {
		return http.StatusAccepted, gin.H{"ignored": true}
	}
	// hint only: never publishes, never fetches
	res := tx.Model(&models.Manifest{}).
		Where("source_type = ? AND github_installation_id = ? AND LOWER(git_repo_url) = LOWER(?)",
			models.ManifestSourceGit, ev.Installation.ID, repo.URL(e)).
		Updates(map[string]interface{}{"git_latest_sha": ev.After, "git_latest_ref": ev.Ref, "git_latest_at": time.Now()})
	if res.Error != nil {
		log.Printf("[manifest-git] webhook update: %v", res.Error)
		return http.StatusInternalServerError, gin.H{"error": "internal error"}
	}
	return http.StatusAccepted, gin.H{"matched": res.RowsAffected}
}

// gitSourceUnchanged enforces that the git source of a manifest (repository,
// installation, git_subpath) is immutable after creation: a field may be
// echoed unchanged; a different value is 409 git_source_immutable, an invalid
// git_subpath 400 git_subpath_invalid. Native manifests have no git source,
// so any non-empty value is a change. Writes the response and returns false
// on refusal.
func gitSourceUnchanged(c *gin.Context, m *models.Manifest, req *models.UpdateManifestRequest) bool {
	deref := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	refuse := func(field string) bool {
		c.JSON(http.StatusConflict, gin.H{"error": field + " is immutable after creation (create a new manifest for another repository or directory)", "code": gitCodeSourceImmutable})
		return false
	}
	if req.GitSubpath != nil {
		sub, err := gitsource.CleanSubpath(*req.GitSubpath)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": gitCodeSubpathInvalid})
			return false
		}
		if sub != deref(m.GitSubpath) {
			return refuse("git_subpath")
		}
	}
	if req.GitRepoURL != nil && strings.TrimSpace(*req.GitRepoURL) != deref(m.GitRepoURL) {
		return refuse("git_repo_url")
	}
	if req.GitRepo != nil {
		want := strings.TrimSpace(*req.GitRepo)
		cur := deref(m.GitRepoURL)
		if want != "" || cur != "" {
			r, err := gitsource.ParseRepoFullName(want)
			if err != nil || cur == "" || !strings.HasSuffix(strings.ToLower(cur), "/"+strings.ToLower(r.FullName())) {
				return refuse("git_repo")
			}
		}
	}
	if req.GitHubInstallationID != nil {
		cur := int64(0)
		if m.GitHubInstallationID != nil {
			cur = *m.GitHubInstallationID
		}
		if *req.GitHubInstallationID != cur {
			return refuse("github_installation_id")
		}
	}
	return true
}

// isGitSourceImmutableDBError reports whether err is the BEFORE UPDATE trigger
// that guards git source columns (migration 20261010_16).
func isGitSourceImmutableDBError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "git_source_immutable")
}
