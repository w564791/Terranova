package handlers

import (
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
	db   *gorm.DB
	perm *middleware.IAMPermissionMiddleware
	ws   *services.WorkspaceService
	// vaultConfigured stubs platform AgentCore vault presence.
	// When false and driver requires vault → 422 vault_credential_required.
	// TODO(step-6): wire real Bot vault lookup for Bedrock/AgentCore access.
	vaultConfigured bool
	// revokeTokens records revoke intent on DELETE (stub until typ=shell /
	// sandbox-scoped run tokens are extended).
	revokeIntents []SandboxRevokeIntent
}

// SandboxRevokeIntent is a recorded credential revoke for unbind.
type SandboxRevokeIntent struct {
	SandboxID string
	OrgID     uint
	At        time.Time
	Reason    string
}

// NewSandboxHandler constructs the registration API handler.
func NewSandboxHandler(db *gorm.DB, perm *middleware.IAMPermissionMiddleware) *SandboxHandler {
	return &SandboxHandler{
		db:              db,
		perm:            perm,
		ws:              services.NewWorkspaceService(db),
		vaultConfigured: true, // stub: treat vault as present until wired; tests can clear
	}
}

// WithVaultConfigured overrides the AgentCore vault stub (tests / wiring).
func (h *SandboxHandler) WithVaultConfigured(ok bool) *SandboxHandler {
	h.vaultConfigured = ok
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
	if req.NetworkMode != nil && strings.TrimSpace(*req.NetworkMode) != "" {
		nm := sandboxreg.NormalizeNetworkMode(*req.NetworkMode)
		if nm != sandboxreg.NetworkModeVPC && nm != sandboxreg.NetworkModePublic {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"code":    sandboxreg.ErrNetworkModeNotAllowed,
				"message": "network_mode must be VPC or PUBLIC",
			})
			return
		}
		// Production refuses to register PUBLIC (same code as attach).
		if err := sandboxreg.ValidateNetworkModeForAttach(nm); err != nil {
			writeSandboxValidation(c, err)
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
	h.revokeSandboxCredentials(row.ID, orgID, "sandbox_unbind")
	now := time.Now()
	if err := h.db.Model(&row).Updates(map[string]interface{}{
		"status":     models.SandboxStatusUnbound,
		"updated_at": now,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": "unbind failed"})
		return
	}
	// Close open shell sessions for this sandbox (credential lifecycle).
	_ = h.db.Model(&models.ShellSession{}).
		Where("sandbox_id = ? AND org_id = ? AND closed_at IS NULL", row.ID, orgID).
		Updates(map[string]interface{}{
			"status":     models.ShellSessionStatusClosed,
			"closed_at":  now,
			"updated_at": now,
		})
	c.Status(http.StatusNoContent)
}

// CreateShellSession stubs shell session create + credential issue hooks (§10.2).
// TODO(step-6): issue typ=shell JWT (claim shell_session_id), STS assume_role,
// hard cap ≤15min, heartbeat renew ≤ expires_at. Require shell_debug driver.
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
	if sb.NetworkMode != nil {
		if err := sandboxreg.ValidateNetworkModeForAttach(*sb.NetworkMode); err != nil {
			writeSandboxValidation(c, err)
			return
		}
	}
	// TODO(step-6 data plane): wake via sandboxreg.HarnessInvoker.InvokeHarness only —
	// never invoke-agent-runtime.
	userID := c.GetString("user_id")
	if userID == "" {
		userID = "unknown"
	}
	sess := models.ShellSession{
		ID:        uuid.New().String(),
		SandboxID: sb.ID,
		OrgID:     orgID,
		Status:    models.ShellSessionStatusActive,
		ExpiresAt: time.Now().Add(15 * time.Minute),
		CreatedBy: userID,
	}
	if err := h.db.Create(&sess).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": "create shell session failed"})
		return
	}
	// TODO(step-6): mint typ=shell token bound to sess.ID; inject STS; do not
	// cache tokens on the warm runtime.
	c.JSON(http.StatusCreated, sess)
}

// DeleteShellSession stubs shell session close + credential revoke (§10.2).
// TODO(step-6): revoke all typ=shell credentials for this session before close.
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
	h.revokeSandboxCredentials(sess.SandboxID, orgID, "shell_session_close:"+sess.ID)
	now := time.Now()
	if err := h.db.Model(&sess).Updates(map[string]interface{}{
		"status":     models.ShellSessionStatusClosed,
		"closed_at":  now,
		"updated_at": now,
	}).Error; err != nil {
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
	if sb.NetworkMode != nil {
		return sandboxreg.ValidateNetworkModeForAttach(*sb.NetworkMode)
	}
	return nil
}

func (h *SandboxHandler) revokeSandboxCredentials(sandboxID string, orgID uint, reason string) {
	// TODO(step-6): revoke unexpired run_tokens / typ=shell tokens scoped to
	// this sandbox once token tables carry sandbox_id / shell_session_id.
	h.revokeIntents = append(h.revokeIntents, SandboxRevokeIntent{
		SandboxID: sandboxID,
		OrgID:     orgID,
		At:        time.Now(),
		Reason:    reason,
	})
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
