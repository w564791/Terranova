package middleware

import (
	"errors"
	"iac-platform/services"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// StateTokenAuth validates the JWT token from Terraform HTTP backend's Basic Auth.
// The password field carries the JWT token; username is ignored.
//
// Task tokens (typ task, or no typ): unchanged (cross-workspace GET allowed).
// Run tokens (typ run): the run's workspace only; preview runs are read-only
// (POST state / LOCK / UNLOCK / DELETE → 403); a failed database check
// refuses (503).
func StateTokenAuth(tokenService *services.StateTokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Debug: log all incoming requests to state backend
		authHeader := c.GetHeader("Authorization")
		log.Printf("[StateTokenAuth] %s %s | Auth header present: %v (len=%d)", c.Request.Method, c.Request.URL.Path, authHeader != "", len(authHeader))

		_, password, ok := c.Request.BasicAuth()
		if !ok || password == "" {
			log.Printf("[StateTokenAuth] BasicAuth parse failed: ok=%v, password_empty=%v", ok, password == "")
			c.Header("WWW-Authenticate", `Basic realm="Terraform State"`)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		// Log token prefix for debugging (safe: JWT header is not secret)
		tokenPrefix := password
		if len(tokenPrefix) > 30 {
			tokenPrefix = tokenPrefix[:30] + "..."
		}
		log.Printf("[StateTokenAuth] Token prefix: %s", tokenPrefix)

		caller, err := tokenService.Authenticate(c.Request.Context(), password)
		if err != nil {
			if errors.Is(err, services.ErrRunTokenUnavailable) {
				c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "token could not be verified"})
				return
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or revoked token"})
			return
		}
		workspaceID := caller.WorkspaceID
		urlWorkspaceID := c.Param("workspace_id")

		if caller.Type == services.StateTokenTypeRun {
			// run tokens: preview runs read only (no state POST, LOCK, UNLOCK
			// or DELETE) and reach their own workspace only. Approval runs
			// replace the task token of their plan_and_apply task: other
			// workspaces' state is readable exactly as with a task token
			// (GET only, filtered and authorized in getCrossWorkspaceState),
			// state versions are attributed to the task.
			if caller.ReadOnly() && c.Request.Method != http.MethodGet {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "preview run tokens are read-only"})
				return
			}
			target := workspaceID
			if urlWorkspaceID != "" && urlWorkspaceID != workspaceID {
				if caller.ReadOnly() || c.Request.Method != http.MethodGet {
					c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "run token is bound to another workspace"})
					return
				}
				c.Set("cross_workspace", true)
				c.Set("requester_workspace_id", workspaceID)
				target = urlWorkspaceID
			}
			c.Set("state_workspace_id", target)
			c.Set("state_task_id", caller.TaskID)
			c.Set("state_run_id", caller.RunID)
			c.Set("state_run_created_by", caller.CreatedBy)
			c.Next()
			return
		}
		taskID := caller.TaskID

		if urlWorkspaceID != "" && urlWorkspaceID != workspaceID {
			// Cross-workspace access: only GET (read state) is allowed.
			// POST/LOCK/UNLOCK remain forbidden for cross-workspace requests.
			if c.Request.Method != http.MethodGet {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "cross-workspace write not allowed"})
				return
			}
			c.Set("cross_workspace", true)
			c.Set("requester_workspace_id", workspaceID)
		}

		// Always use the URL workspace_id as the target
		targetWorkspaceID := workspaceID
		if urlWorkspaceID != "" {
			targetWorkspaceID = urlWorkspaceID
		}
		c.Set("state_workspace_id", targetWorkspaceID)
		c.Set("state_task_id", taskID)
		c.Next()
	}
}
