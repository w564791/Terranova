package middleware

import (
	"net/http"

	"iac-platform/internal/application/service"
	"iac-platform/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AgentAuthMiddleware validates application credentials and agent identity
func AgentAuthMiddleware(db *gorm.DB) gin.HandlerFunc {
	agentService := service.NewAgentService(db)

	return func(c *gin.Context) {
		// 1. Get and validate application credentials
		appKey := c.GetHeader("X-App-Key")
		appSecret := c.GetHeader("X-App-Secret")

		if appKey == "" || appSecret == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "missing application credentials",
			})
			c.Abort()
			return
		}

		app, err := agentService.ValidateApplication(appKey, appSecret)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": err.Error(),
			})
			c.Abort()
			return
		}

		// 2. Get agent_id from header or path parameter
		agentID := c.GetHeader("X-Agent-ID")
		if agentID == "" {
			agentID = c.Param("agent_id")
		}

		if agentID != "" {
			// 3. Verify agent belongs to this application
			var agent models.Agent
			err = db.Where("agent_id = ? AND application_id = ?", agentID, app.ID).First(&agent).Error
			if err != nil {
				c.JSON(http.StatusForbidden, gin.H{
					"error": "agent not found or does not belong to this application",
				})
				c.Abort()
				return
			}

			// Store agent in context
			c.Set("agent", agent)
			c.Set("agent_id", agentID)
		}

		// Store application in context
		c.Set("application", app)
		c.Set("application_id", app.ID)

		c.Next()
	}
}

// AgentWorkspaceAuthMiddleware is deprecated.
// Agent-level authorization has been migrated to Pool-level authorization.
// Use Pool-level validation instead: ValidatePoolAccess(poolID, workspaceID)
func AgentWorkspaceAuthMiddleware(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusGone, gin.H{
			"error": "Agent-level authorization is deprecated. Please use Pool-level authorization.",
		})
		c.Abort()
	}
}
