package middleware

import (
	"fmt"
	"net/http"
	"strconv"

	"iac-platform/internal/application/service"
	"iac-platform/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AgentAuthMiddleware validates application credentials and agent identity.
// 同时写入 IAM principal 上下文（APPLICATION），供 RequirePermission 等中间件求值。
// 注意：Agent 任务/工作区 API 主路径仍使用 Pool Token；本中间件用于 App 密钥鉴权场景。
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

		// IAM principal 链（选项 A）：与 PermissionChecker APPLICATION 路径对齐
		// principal_id 必须为 app_key（外部稳定标识）；user_id 合成以便 principalFromContext 非空
		principalID := app.AppKey
		if principalID == "" {
			principalID = strconv.FormatUint(uint64(app.ID), 10)
		}
		c.Set("principal_type", "APPLICATION")
		c.Set("principal_id", principalID)
		c.Set("user_id", "app:"+principalID)
		c.Set("username", fmt.Sprintf("app:%s", app.Name))
		if app.OrgID > 0 {
			c.Set("auth_org_id", app.OrgID)
		}

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
