package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"iac-platform/internal/application/service"
	"iac-platform/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// JWTOrApplicationAuth 双轨鉴权（选项 A）：
// 1) 若带 X-App-Key + X-App-Secret → Application principal（AgentAuth 语义）
// 2) 否则走 JWT（用户 / Team Token）
//
// Agent 任务执行（锁/state/日志上传）仍应使用 Pool Token 专用路由，不在此合并。
func JWTOrApplicationAuth(db *gorm.DB) gin.HandlerFunc {
	agentService := service.NewAgentService(db)
	jwt := JWTAuth()

	return func(c *gin.Context) {
		appKey := strings.TrimSpace(c.GetHeader("X-App-Key"))
		appSecret := strings.TrimSpace(c.GetHeader("X-App-Secret"))

		if appKey != "" || appSecret != "" {
			if appKey == "" || appSecret == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "X-App-Key and X-App-Secret are both required"})
				c.Abort()
				return
			}
			app, err := agentService.ValidateApplication(appKey, appSecret)
			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
				c.Abort()
				return
			}
			// 可选：绑定 agent 归属
			agentID := c.GetHeader("X-Agent-ID")
			if agentID == "" {
				agentID = c.Param("agent_id")
			}
			if agentID != "" {
				var agent models.Agent
				if err := db.Where("agent_id = ? AND application_id = ?", agentID, app.ID).First(&agent).Error; err != nil {
					c.JSON(http.StatusForbidden, gin.H{"error": "agent not found or does not belong to this application"})
					c.Abort()
					return
				}
				c.Set("agent", agent)
				c.Set("agent_id", agentID)
			}

			principalID := app.AppKey
			if principalID == "" {
				principalID = strconv.FormatUint(uint64(app.ID), 10)
			}
			c.Set("application", app)
			c.Set("application_id", app.ID)
			c.Set("principal_type", "APPLICATION")
			c.Set("principal_id", principalID)
			c.Set("user_id", "app:"+principalID)
			c.Set("username", fmt.Sprintf("app:%s", app.Name))
			if app.OrgID > 0 {
				c.Set("auth_org_id", app.OrgID)
			}
			c.Next()
			return
		}

		// 无 App 密钥 → JWT
		jwt(c)
	}
}

// ApplicationAuthOnly 仅允许 Application 密钥（不接受 JWT）
// 用于 /api/v1/app/* 专用面
func ApplicationAuthOnly(db *gorm.DB) gin.HandlerFunc {
	return AgentAuthMiddleware(db)
}
