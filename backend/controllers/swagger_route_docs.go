package controllers

// Docs-only stubs for routes that are wired as inline lambdas in the router.
// These empty functions exist solely so swag can generate OpenAPI entries.
// They are never registered as HTTP handlers.

// LockWorkspaceSwagger documents POST /api/v1/workspaces/{id}/lock
// Real handler: inline lambda in internal/router/router_workspace.go (lifecycleService.LockWorkspace).
// @Summary Lock workspace
// @Description Lock a workspace for exclusive access. Body requires reason.
// @Tags Workspace
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param request body object true "Lock request with reason string"
// @Success 200 {object} map[string]interface{} "Workspace locked successfully"
// @Failure 400 {object} map[string]interface{} "Invalid request"
// @Failure 500 {object} map[string]interface{} "Lock failed"
// @Security BearerAuth
// @Router /api/v1/workspaces/{id}/lock [post]
func LockWorkspaceSwagger() {}

// UnlockWorkspaceSwagger documents POST /api/v1/workspaces/{id}/unlock
// Real handler: inline lambda in internal/router/router_workspace.go (lifecycleService.UnlockWorkspace).
// @Summary Unlock workspace
// @Description Unlock a workspace after exclusive operations complete
// @Tags Workspace
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {object} map[string]interface{} "Workspace unlocked successfully"
// @Failure 500 {object} map[string]interface{} "Unlock failed"
// @Security BearerAuth
// @Router /api/v1/workspaces/{id}/unlock [post]
func UnlockWorkspaceSwagger() {}

// GetEffectiveVariablesSwagger documents GET /api/v1/workspaces/{id}/effective-variables
// Real handler: inline lambda in internal/router/router_workspace.go (VariableResolutionService.ResolveDisplay).
// @Summary Get effective workspace variables
// @Description Get merged variables from variable sets and workspace-level variables for display
// @Tags Workspace Variable
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {object} map[string]interface{} "Resolved effective variables"
// @Failure 500 {object} map[string]interface{} "Failed to resolve effective variables"
// @Security BearerAuth
// @Router /api/v1/workspaces/{id}/effective-variables [get]
func GetEffectiveVariablesSwagger() {}

// GetIAMStatusSwagger documents GET /api/v1/iam/status
// Real handler: inline lambda in internal/router/router_iam.go.
// @Summary Get IAM system status
// @Description Health/status endpoint for the IAM subsystem
// @Tags IAM
// @Produce json
// @Success 200 {object} map[string]interface{} "IAM system status"
// @Security BearerAuth
// @Router /api/v1/iam/status [get]
func GetIAMStatusSwagger() {}

// AgentControlGoneSwagger documents GET /api/v1/agents/control on the main API (8080).
// Real handler: inline lambda in router_agent.go returns 410 Gone.
// Live WebSocket C&C is on standalone server port 8091 (same path).
// @Summary Agent C&C endpoint moved (main API)
// @Description Main API returns 410 Gone. Connect WebSocket to port 8091: ws://host:8091/api/v1/agents/control?agent_id=...
// @Tags Agent C&C
// @Produce json
// @Success 410 {object} map[string]interface{} "WebSocket endpoint has moved to port 8091"
// @Router /api/v1/agents/control [get]
func AgentControlGoneSwagger() {}
