package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"iac-platform/internal/middleware"
	"iac-platform/internal/models"
	"iac-platform/internal/sandboxreg"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SandboxHandler registers and lists externally managed sandboxes (§10.2).
// Platform does not create/destroy AgentCore containers.
type SandboxHandler struct {
	db     *gorm.DB
	perm   *middleware.IAMPermissionMiddleware
	ws     *services.WorkspaceService
	tokens *services.StateTokenService
	// harness wakes AgentCore via InvokeHarness (stub by default;
	// AWS SDK when IAC_AGENTCORE_INVOKE_HARNESS=1).
	harness sandboxreg.HarnessInvoker
	// vaultConfigured stubs platform AgentCore vault presence.
	// When false and driver requires vault → 422 vault_credential_required.
	// TODO(step-6): wire real Bot vault lookup for Bedrock/AgentCore access.
	vaultConfigured bool
}

// NewSandboxHandler constructs the registration API handler.
func NewSandboxHandler(db *gorm.DB, perm *middleware.IAMPermissionMiddleware) *SandboxHandler {
	h := &SandboxHandler{
		db:              db,
		perm:            perm,
		ws:              services.NewWorkspaceService(db),
		tokens:          services.NewStateTokenService(db),
		harness:         sandboxreg.NewStubHarnessInvoker(),
		vaultConfigured: true, // stub: treat vault as present until wired; tests can clear
	}
	// When IAC_AGENTCORE_INVOKE_HARNESS=1, always install the returned invoker
	// (AWS or FailingHarnessInvoker). Never keep the default stub on config
	// failure — that would silently skip AWS while operators think it was called.
	if inv, err := sandboxreg.HarnessInvokerFromEnv(context.Background()); inv != nil {
		h.harness = inv
		_ = err // construction error is carried by FailingHarnessInvoker
	}
	return h
}

// WithVaultConfigured overrides the AgentCore vault stub (tests / wiring).
func (h *SandboxHandler) WithVaultConfigured(ok bool) *SandboxHandler {
	h.vaultConfigured = ok
	return h
}

// WithHarness overrides the AgentCore wake invoker (tests / wiring).
func (h *SandboxHandler) WithHarness(inv sandboxreg.HarnessInvoker) *SandboxHandler {
	if inv != nil {
		h.harness = inv
	}
	return h
}

type createSandboxRequest struct {
	WorkspaceID      string  `json:"workspace_id" binding:"required"`
	SandboxRef       string  `json:"sandbox_ref" binding:"required"`
	Driver           string  `json:"driver" binding:"required"`
	Label            *string `json:"label"`
	ImageDigest      *string `json:"image_digest"`
	RuntimeRef       *string `json:"runtime_ref"`
	AssumeRoleARN    *string `json:"assume_role_arn"`
	AssumeExternalID *string `json:"assume_external_id"`
	// NetworkMode optional snapshot (VPC|PUBLIC). 联调 may store PUBLIC;
	// production attach re-checks and rejects PUBLIC (ENV=production).
	NetworkMode *string `json:"network_mode"`
}

// CreateSandbox godoc
// @Summary Register an external sandbox
// @Description Platform-only registration of an externally created AgentCore harness/runtime (or shell_debug). Does not create/destroy remote containers. Permission: target workspace WORKSPACE_RESOURCES WRITE. Cross-org workspace → 404.
// @Tags sandboxes
// @Accept json
// @Produce json
// @Param org_id path int true "Organization ID"
// @Param body body createSandboxRequest true "Registration"
// @Success 201 {object} models.Sandbox
// @Failure 404 {object} map[string]interface{}
// @Failure 422 {object} map[string]interface{}
// @Router /organizations/{org_id}/sandboxes [post]
func (h *SandboxHandler) CreateSandbox(c *gin.Context) {
	orgID, ok := middleware.AuthOrgID(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_org_id", "message": "org_id required"})
		return
	}
	var req createSandboxRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_body", "message": err.Error()})
		return
	}
	if err := sandboxreg.ValidateSandboxRef(req.SandboxRef); err != nil {
		writeSandboxValidation(c, err)
		return
	}
	runtimeRef := ""
	if req.RuntimeRef != nil {
		runtimeRef = *req.RuntimeRef
	}
	if err := sandboxreg.ValidateOptionalRuntimeRef(runtimeRef); err != nil {
		writeSandboxValidation(c, err)
		return
	}
	driver := strings.TrimSpace(req.Driver)
	if driver != models.SandboxDriverAgentCoreExternal && driver != models.SandboxDriverShellDebug {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"code":    "invalid_driver",
			"message": "driver must be agentcore_external or shell_debug",
		})
		return
	}
	var networkMode *string
	rawMode := ""
	if req.NetworkMode != nil {
		rawMode = *req.NetworkMode
	}
	nm, err := sandboxreg.ParseNetworkModeForCreate(rawMode)
	if err != nil {
		writeSandboxValidation(c, err) // invalid_network_mode for bad body values
		return
	}
	if nm != "" {
		// Production refuses PUBLIC (and would refuse empty on attach; empty
		// create is still allowed so mode can be filled after 联调 probe).
		if err := sandboxreg.ValidateNetworkModeForAttach(nm); err != nil {
			writeSandboxValidation(c, err) // network_mode_not_allowed
			return
		}
		networkMode = &nm
	}

	if err := h.ws.EnsureWorkspaceInOrg(req.WorkspaceID, orgID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "Workspace not found"})
		return
	}
	if h.perm != nil && !h.perm.RequireWorkspaceResourcePermission(c, req.WorkspaceID, "WORKSPACE_RESOURCES", "WRITE") {
		return // 401/403 already written
	}
	if driver == models.SandboxDriverAgentCoreExternal && !h.vaultConfigured {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"code":    "vault_credential_required",
			"message": "platform AgentCore vault credential is not configured",
		})
		return
	}

	userID := c.GetString("user_id")
	if userID == "" {
		userID = "unknown"
	}
	status := models.SandboxStatusRegistered
	if h.vaultConfigured {
		status = models.SandboxStatusReady
	}
	row := models.Sandbox{
		ID:               uuid.New().String(),
		OrgID:            orgID,
		WorkspaceID:      req.WorkspaceID,
		SandboxRef:       strings.TrimSpace(req.SandboxRef),
		Label:            req.Label,
		Driver:           driver,
		ImageDigest:      req.ImageDigest,
		AssumeRoleARN:    req.AssumeRoleARN,
		AssumeExternalID: req.AssumeExternalID,
		NetworkMode:      networkMode,
		Status:           status,
		CreatedBy:        userID,
	}
	if runtimeRef != "" {
		r := strings.TrimSpace(runtimeRef)
		row.RuntimeRef = &r
	}
	if err := h.db.Create(&row).Error; err != nil {
		if isUniqueViolation(err) {
			c.JSON(http.StatusConflict, gin.H{"code": "sandbox_ref_exists", "message": "sandbox_ref already registered in this org"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": "failed to register sandbox"})
		return
	}
	c.JSON(http.StatusCreated, row)
}

// ListSandboxes godoc
// @Summary List registered sandboxes in an organization
// @Tags sandboxes
// @Produce json
// @Param org_id path int true "Organization ID"
// @Success 200 {object} map[string]interface{}
// @Router /organizations/{org_id}/sandboxes [get]
func (h *SandboxHandler) ListSandboxes(c *gin.Context) {
	orgID, ok := middleware.AuthOrgID(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_org_id", "message": "org_id required"})
		return
	}
	var rows []models.Sandbox
	q := h.db.Where("org_id = ? AND status <> ?", orgID, models.SandboxStatusUnbound)
	if ws := c.Query("workspace_id"); ws != "" {
		q = q.Where("workspace_id = ?", ws)
	}
	if err := q.Order("created_at DESC").Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": "list failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows, "total": len(rows)})
}

// GetSandbox godoc
// @Summary Get a registered sandbox by id
// @Tags sandboxes
// @Produce json
// @Param org_id path int true "Organization ID"
// @Param id path string true "Sandbox ID"
// @Success 200 {object} models.Sandbox
// @Failure 404 {object} map[string]interface{}
// @Router /organizations/{org_id}/sandboxes/{id} [get]
func (h *SandboxHandler) GetSandbox(c *gin.Context) {
	orgID, ok := middleware.AuthOrgID(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_org_id", "message": "org_id required"})
		return
	}
	id := c.Param("id")
	var row models.Sandbox
	err := h.db.Where("id = ? AND org_id = ?", id, orgID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "Sandbox not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": "get failed"})
		return
	}
	c.JSON(http.StatusOK, row)
}

// DeleteSandbox godoc
// @Summary Unbind a sandbox registration
// @Description Marks the row unbound and revokes unexpired run/shell tokens for that sandbox. Does NOT call AWS to destroy harness/runtime.
// @Tags sandboxes
// @Param org_id path int true "Organization ID"
// @Param id path string true "Sandbox ID"
// @Success 204
// @Failure 404 {object} map[string]interface{}
// @Router /organizations/{org_id}/sandboxes/{id} [delete]
func (h *SandboxHandler) DeleteSandbox(c *gin.Context) {
	orgID, ok := middleware.AuthOrgID(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_org_id", "message": "org_id required"})
		return
	}
	id := c.Param("id")
	var row models.Sandbox
	err := h.db.Where("id = ? AND org_id = ?", id, orgID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "Sandbox not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": "get failed"})
		return
	}
	if h.perm != nil && !h.perm.RequireWorkspaceResourcePermission(c, row.WorkspaceID, "WORKSPACE_RESOURCES", "WRITE") {
		return
	}
	now := time.Now()
	err = h.db.Transaction(func(tx *gorm.DB) error {
		// Revoke typ=shell tokens first, then close sessions, then unbind.
		if err := services.RevokeShellTokensForSandbox(c.Request.Context(), tx, row.ID); err != nil {
			return err
		}
		if err := tx.Model(&models.ShellSession{}).
			Where("sandbox_id = ? AND org_id = ? AND closed_at IS NULL", row.ID, orgID).
			Updates(map[string]interface{}{
				"status":     models.ShellSessionStatusClosed,
				"closed_at":  now,
				"updated_at": now,
			}).Error; err != nil {
			return err
		}
		return tx.Model(&row).Updates(map[string]interface{}{
			"status":     models.SandboxStatusUnbound,
			"updated_at": now,
		}).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": "unbind failed"})
		return
	}
	// TODO(step-6): also revoke unexpired typ=run tokens scoped to this sandbox
	// once run_tokens carries sandbox_id.
	c.Status(http.StatusNoContent)
}

// CreateShellSession creates a shell_sessions row and mints a typ=shell JWT
// (§10.2). Token lifetime = session lifetime, hard-capped at 15 minutes.
// Requires driver=shell_debug and AssertAttachable (production → VPC).
// Wake uses HarnessInvoker (InvokeHarness). TODO(step-6): STS AssumeRole inject.
func (h *SandboxHandler) CreateShellSession(c *gin.Context) {
	orgID, ok := middleware.AuthOrgID(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_org_id", "message": "org_id required"})
		return
	}
	sandboxID := c.Param("id")
	var sb models.Sandbox
	err := h.db.Where("id = ? AND org_id = ? AND status <> ?", sandboxID, orgID, models.SandboxStatusUnbound).First(&sb).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "Sandbox not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": "get failed"})
		return
	}
	if h.perm != nil && !h.perm.RequireWorkspaceResourcePermission(c, sb.WorkspaceID, "WORKSPACE_RESOURCES", "WRITE") {
		return
	}
	if sb.Driver != models.SandboxDriverShellDebug {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"code":    "shell_not_supported",
			"message": "Shell sessions require driver=shell_debug",
		})
		return
	}
	if err := AssertAttachable(&sb); err != nil {
		writeSandboxValidation(c, err)
		return
	}
	userID := c.GetString("user_id")
	if userID == "" {
		userID = "unknown"
	}
	now := time.Now()
	sess := models.ShellSession{
		ID:        uuid.New().String(),
		SandboxID: sb.ID,
		OrgID:     orgID,
		Status:    models.ShellSessionStatusActive,
		ExpiresAt: now.Add(services.MaxShellSessionTTL),
		CreatedBy: userID,
	}
	runtimeRef := ""
	if sb.RuntimeRef != nil {
		runtimeRef = *sb.RuntimeRef
	}
	// Wake via InvokeHarness only (never invoke-agent-runtime). Stub by default;
	// live AWS when IAC_AGENTCORE_INVOKE_HARNESS=1. Fail before minting tokens.
	// Bound InvokeHarness + stream drain with InvokeHarnessTimeout (~60s); do not
	// rely solely on an unbounded request context while draining the event stream.
	if h.harness != nil {
		wakeCtx, wakeCancel := context.WithTimeout(c.Request.Context(), sandboxreg.InvokeHarnessTimeout)
		defer wakeCancel()
		if err := h.harness.InvokeHarness(wakeCtx, sandboxreg.InvokeRequest{
			HarnessARN: sb.SandboxRef,
			RuntimeRef: runtimeRef,
			SessionID:  sess.ID,
			Message:    sandboxreg.DefaultWakeMessage,
			Qualifier:  sandboxreg.DefaultHarnessQualifier,
		}); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"code":    "harness_invoke_failed",
				"message": "InvokeHarness wake failed",
			})
			return
		}
	}
	var tok string
	var exp time.Time
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&sess).Error; err != nil {
			return err
		}
		var err error
		tok, exp, err = h.tokens.IssueShellToken(c.Request.Context(), tx, &sess, &sb)
		return err
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": "create shell session failed"})
		return
	}
	// TODO(step-6): STS AssumeRole inject into the woken session (not on runtime cache).
	c.JSON(http.StatusCreated, gin.H{
		"id":               sess.ID,
		"sandbox_id":       sess.SandboxID,
		"org_id":           sess.OrgID,
		"status":           sess.Status,
		"expires_at":       sess.ExpiresAt,
		"created_by":       sess.CreatedBy,
		"created_at":       sess.CreatedAt,
		"shell_token":      tok,
		"token_expires_at": exp,
	})
}

// DeleteShellSession revokes typ=shell tokens then closes the session in one
// transaction (§10.2). No orphan tokens.
func (h *SandboxHandler) DeleteShellSession(c *gin.Context) {
	orgID, ok := middleware.AuthOrgID(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_org_id", "message": "org_id required"})
		return
	}
	sessID := c.Param("session_id")
	var sess models.ShellSession
	err := h.db.Where("id = ? AND org_id = ?", sessID, orgID).First(&sess).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "Shell session not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": "get failed"})
		return
	}
	var sb models.Sandbox
	if err := h.db.Where("id = ? AND org_id = ?", sess.SandboxID, orgID).First(&sb).Error; err == nil {
		if h.perm != nil && !h.perm.RequireWorkspaceResourcePermission(c, sb.WorkspaceID, "WORKSPACE_RESOURCES", "WRITE") {
			return
		}
	}
	if err := services.EndShellSession(c.Request.Context(), h.db, sess.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": "close failed"})
		return
	}
	c.Status(http.StatusNoContent)
}

// AssertAttachable re-checks stored network_mode before production preview/Deploy.
// Handlers for wake/preview should call this; registration of PUBLIC in non-prod
// must not bypass a later production attach.
func AssertAttachable(sb *models.Sandbox) error {
	if sb == nil {
		return &sandboxreg.ValidationError{Code: "sandbox_required", Message: "sandbox required"}
	}
	if sb.Status == models.SandboxStatusUnbound {
		return &sandboxreg.ValidationError{Code: "sandbox_unbound", Message: "sandbox is unbound"}
	}
	mode := ""
	if sb.NetworkMode != nil {
		mode = *sb.NetworkMode
	}
	return sandboxreg.ValidateNetworkModeForAttach(mode)
}

func writeSandboxValidation(c *gin.Context, err error) {
	var ve *sandboxreg.ValidationError
	if errors.As(err, &ve) {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"code": ve.Code, "message": ve.Message})
		return
	}
	c.JSON(http.StatusUnprocessableEntity, gin.H{"code": "invalid_request", "message": err.Error()})
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "duplicate")
}
