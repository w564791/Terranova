package middleware

import (
	"iac-platform/services"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// StateTokenAuth validates the JWT token from Terraform HTTP backend's Basic Auth.
// The password field carries the JWT token; username is ignored.
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

		workspaceID, taskID, err := tokenService.ValidateToken(password)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or revoked token"})
			return
		}

		// Verify URL workspace_id matches token
		urlWorkspaceID := c.Param("workspace_id")
		if urlWorkspaceID != "" && urlWorkspaceID != workspaceID {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "token workspace mismatch"})
			return
		}

		c.Set("state_workspace_id", workspaceID)
		c.Set("state_task_id", taskID)
		c.Next()
	}
}
