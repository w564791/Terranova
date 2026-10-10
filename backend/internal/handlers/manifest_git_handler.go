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
}

// GitSourceDeps resolves the GitHub App, the git fetcher, the endpoints and
// the webhook secret. Each call re-reads the configuration (env); an
// unconfigured App yields gitsource.ErrDisabled.
type GitSourceDeps struct {
	App           func() (GitApp, error)
	Fetcher       func() (*gitsource.Fetcher, error)
	Endpoints     func() (gitsource.Endpoints, error)
	WebhookSecret func() ([]byte, error)
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
}

// Git-source error codes (JSON "code").
const (
	gitCodeDisabled           = "git_source_disabled"
	gitCodeRepoNotAccessible  = "git_repo_not_accessible"
	gitCodeCommitNotFound     = "git_commit_not_found"
	gitCodeSubpathNotFound    = "git_subpath_not_found"
	gitCodeFetchFailed        = "git_fetch_failed"
	gitCodeNotGitSource       = "not_git_source"
	gitCodeReadOnly           = "git_source_read_only"
	gitCodeInstallationNotReg = "github_installation_not_registered"
)

// respondGitError maps git-source errors to HTTP. Details from git are
// already token-scrubbed; they are logged, never returned.
func respondGitError(c *gin.Context, op string, err error) {
	var apiErr *gitsource.APIError
	switch {
	case errors.Is(err, gitsource.ErrDisabled):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error(), "code": gitCodeDisabled})
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
// contents:read), runs fn and revokes the token.
func withRepoToken(ctx context.Context, m *models.Manifest, fn func(app GitApp, repo gitsource.Repo, tok *gitsource.Token) error) error {
	repo, _, err := gitManifestRepo(m)
	if err != nil {
		return err
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
func fetchGitCommit(ctx context.Context, m *models.Manifest, sha string) (*gitsource.Tree, error) {
	fetcher, err := gitDeps.Fetcher()
	if err != nil {
		return nil, err
	}
	subpath := ""
	if m.GitSubpath != nil {
		subpath = *m.GitSubpath
	}
	var tree *gitsource.Tree
	err = withRepoToken(ctx, m, func(_ GitApp, repo gitsource.Repo, tok *gitsource.Token) error {
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
	repo, err := gitsource.ParseRepoURL(req.GitRepoURL, e)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return false
	}
	subpath, err := gitsource.CleanSubpath(req.GitSubpath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return false
	}
	if req.GitHubInstallationID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "github_installation_id is required for a git manifest"})
		return false
	}
	var inst models.GitHubAppInstallation
	if err := db.Where("organization_id = ? AND installation_id = ?", orgID, req.GitHubInstallationID).Take(&inst).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "the GitHub App installation is not registered for this organization (an organization admin registers it)", "code": gitCodeInstallationNotReg})
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
	if err := withRepoToken(c.Request.Context(), m, func(GitApp, gitsource.Repo, *gitsource.Token) error { return nil }); err != nil {
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
	err := withRepoToken(c.Request.Context(), m, func(app GitApp, repo gitsource.Repo, tok *gitsource.Token) error {
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
	err := withRepoToken(c.Request.Context(), m, func(app GitApp, repo gitsource.Repo, tok *gitsource.Token) error {
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

// GitHubAppHandler org-level GitHub App installation registry (org ADMIN).
type GitHubAppHandler struct{ db *gorm.DB }

func NewGitHubAppHandler(db *gorm.DB) *GitHubAppHandler { return &GitHubAppHandler{db: db} }

// RegisterGitHubInstallationRequest body of POST .../github-app/installations.
type RegisterGitHubInstallationRequest struct {
	InstallationID int64 `json:"installation_id" binding:"required"`
}

// ListInstallations GitHub App installations of the organization
// @Summary List GitHub App installations
// @Description GitHub App installations registered for the organization (installation_id, account_login). Requires ORGANIZATION ADMIN.
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

// RegisterInstallation registers a GitHub App installation for the org
// @Summary Register GitHub App installation
// @Description Register an installation of the platform's GitHub App for the organization; git manifests of the org can then use repositories of its account. The installation is looked up with the App JWT (account_login comes from GitHub). An installation belongs to one organization: 409 when another organization registered it. Requires ORGANIZATION ADMIN. 503 git_source_disabled when the App is not configured; 422 when GitHub does not know the installation.
// @Tags Manifest Git
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param request body handlers.RegisterGitHubInstallationRequest true "Installation"
// @Success 201 {object} models.GitHubAppInstallation
// @Success 200 {object} models.GitHubAppInstallation "already registered for this organization"
// @Failure 409 {object} map[string]interface{}
// @Failure 422 {object} map[string]interface{}
// @Failure 503 {object} map[string]interface{}
// @Router /api/v1/organizations/{org_id}/github-app/installations [post]
// @Security BearerAuth
func (h *GitHubAppHandler) RegisterInstallation(c *gin.Context) {
	orgID, ok := middleware.AuthOrgID(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "org_id is required"})
		return
	}
	var req RegisterGitHubInstallationRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.InstallationID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "installation_id must be a positive integer"})
		return
	}
	var existing models.GitHubAppInstallation
	err := h.db.Where("installation_id = ?", req.InstallationID).Take(&existing).Error
	if err == nil {
		if existing.OrganizationID == int(orgID) {
			c.JSON(http.StatusOK, existing)
		} else {
			c.JSON(http.StatusConflict, gin.H{"error": "the installation is registered to another organization"})
		}
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		_ = c.Error(err)
		return
	}
	app, err := gitDeps.App()
	if err != nil {
		respondGitError(c, "app", err)
		return
	}
	inst, err := app.GetInstallation(c.Request.Context(), req.InstallationID)
	if err != nil {
		if errors.Is(err, gitsource.ErrNotFound) {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "GitHub App installation not found"})
			return
		}
		respondGitError(c, "get installation", err)
		return
	}
	row := models.GitHubAppInstallation{
		OrganizationID: int(orgID), InstallationID: req.InstallationID, AccountLogin: inst.AccountLogin,
		CreatedBy: c.GetString("user_id"), CreatedAt: time.Now(),
	}
	if err := h.db.Create(&row).Error; err != nil {
		// lost a race on uq_github_app_installations_installation
		c.JSON(http.StatusConflict, gin.H{"error": "the installation is already registered"})
		return
	}
	writeManifestAudit(h.db, auditResourceGitHubInstallation, "github_installation.register", c.GetString("user_id"), map[string]interface{}{
		"organization_id": orgID, "installation_id": row.InstallationID, "account_login": row.AccountLogin,
	})
	c.JSON(http.StatusCreated, row)
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
// @Description Receives GitHub App webhooks. The raw body must carry a valid X-Hub-Signature-256 (HMAC-SHA256 with GITHUB_WEBHOOK_SECRET, constant-time compare); unsigned or invalid => 401, no secret configured => 503. A push to a repository of git manifests (matching installation and repository) records git_latest_sha / git_latest_ref / git_latest_at on them as a "new commit available" hint. It never publishes. Other events are acknowledged and ignored. No user authentication (the signature is the authentication).
// @Tags Manifest Git
// @Accept json
// @Produce json
// @Param X-Hub-Signature-256 header string true "sha256=<hex HMAC>"
// @Param X-GitHub-Event header string true "Event name"
// @Success 200 {object} map[string]interface{}
// @Success 202 {object} map[string]interface{} "{matched: n}"
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
	switch c.GetHeader("X-GitHub-Event") {
	case "ping":
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	case "push":
	default:
		c.JSON(http.StatusAccepted, gin.H{"ignored": true})
		return
	}
	var ev githubPushEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid push payload"})
		return
	}
	if ev.Deleted || !gitsource.IsCommitSHA(ev.After) || ev.Installation.ID <= 0 || len(ev.Ref) > 255 || !strings.HasPrefix(ev.Ref, "refs/") {
		c.JSON(http.StatusAccepted, gin.H{"ignored": true})
		return
	}
	e, err := gitDeps.Endpoints()
	if err != nil {
		_ = c.Error(err)
		return
	}
	repo, err := gitsource.ParseRepoURL(ev.Repository.HTMLURL, e)
	if err != nil {
		c.JSON(http.StatusAccepted, gin.H{"ignored": true})
		return
	}
	// hint only: never publishes, never fetches
	res := h.db.Model(&models.Manifest{}).
		Where("source_type = ? AND github_installation_id = ? AND LOWER(git_repo_url) = LOWER(?)",
			models.ManifestSourceGit, ev.Installation.ID, repo.URL(e)).
		Updates(map[string]interface{}{"git_latest_sha": ev.After, "git_latest_ref": ev.Ref, "git_latest_at": time.Now()})
	if res.Error != nil {
		_ = c.Error(res.Error)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"matched": res.RowsAffected})
}
