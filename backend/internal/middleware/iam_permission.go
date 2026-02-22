package middleware

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"iac-platform/internal/application/service"
	"iac-platform/internal/domain/valueobject"
	"iac-platform/internal/iam"
)

// IAMPermissionMiddleware IAM权限检查中间件
type IAMPermissionMiddleware struct {
	permissionChecker service.PermissionChecker
}

// NewIAMPermissionMiddleware 创建IAM权限中间件
func NewIAMPermissionMiddleware(db *gorm.DB) *IAMPermissionMiddleware {
	factory := iam.NewServiceFactory(db)
	return &IAMPermissionMiddleware{
		permissionChecker: factory.GetPermissionChecker(),
	}
}

// RequirePermission 要求特定权限的中间件工厂函数
// 用法: router.GET("/path", iamMiddleware.RequirePermission("WORKSPACE_EXECUTION", "ORGANIZATION", "READ"))
func (m *IAMPermissionMiddleware) RequirePermission(
	resourceType string,
	scopeType string,
	requiredLevel string,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 0. 系统管理员直接通过（is_system_admin 仅在系统初始化时设置）
		if isSystemAdmin, _ := c.Get("is_system_admin"); isSystemAdmin == true {
			c.Next()
			return
		}

		// 1. 获取用户ID
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":      401,
				"message":   "User not authenticated",
				"timestamp": time.Now(),
			})
			c.Abort()
			return
		}

		// 2. 从路径参数或查询参数获取scope_id
		var scopeIDUint uint
		var scopeIDStr string

		// 解析scope_type以确定如何获取scope_id
		st, err := valueobject.ParseScopeType(scopeType)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":      400,
				"message":   "Invalid scope type",
				"timestamp": time.Now(),
			})
			c.Abort()
			return
		}

		// 根据scope_type决定如何获取scope_id
		if st == valueobject.ScopeTypeOrganization {
			// 组织级别权限：优先从查询参数获取org_id
			// TODO: 多组织架构时需要移除默认值，要求显式传入 org_id
			scopeID := c.Query("org_id")
			if scopeID == "" {
				scopeID = "1" // 当前单组织默认值
			}
			if _, err := fmt.Sscanf(scopeID, "%d", &scopeIDUint); err != nil || scopeIDUint == 0 {
				scopeIDUint = 1
			}
		} else {
			// 工作空间或项目级别权限：从路径参数获取，必须显式指定
			scopeID := c.Param("id")
			if scopeID == "" {
				scopeID = c.Query("scope_id")
			}
			if scopeID == "" {
				log.Printf("[IAM] Missing scope_id for %s %s, user=%v", c.Request.Method, c.Request.URL.Path, userID)
				c.JSON(http.StatusBadRequest, gin.H{
					"code":      400,
					"message":   "scope_id or path parameter :id is required",
					"timestamp": time.Now(),
				})
				c.Abort()
				return
			}

			// 尝试解析为数字，如果失败则保留为字符串（可能是语义化ID）
			if _, err := fmt.Sscanf(scopeID, "%d", &scopeIDUint); err != nil || scopeIDUint == 0 {
				// 不是数字，可能是语义化ID（如 ws-xxx）
				scopeIDStr = scopeID
				scopeIDUint = 0 // 设为0，让CheckPermission通过ScopeIDStr处理
			}
		}

		// 3. 解析其他参数
		rt, err := valueobject.ParseResourceType(resourceType)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":      400,
				"message":   "Invalid resource type",
				"timestamp": time.Now(),
			})
			c.Abort()
			return
		}

		rl, err := valueobject.ParsePermissionLevel(requiredLevel)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":      400,
				"message":   "Invalid permission level",
				"timestamp": time.Now(),
			})
			c.Abort()
			return
		}

		// 4. 检查权限
		req := &service.CheckPermissionRequest{
			UserID:        userID.(string),
			ResourceType:  rt,
			ScopeType:     st,
			ScopeID:       scopeIDUint,
			ScopeIDStr:    scopeIDStr, // 支持语义化ID
			RequiredLevel: rl,
		}

		result, err := m.permissionChecker.CheckPermission(c.Request.Context(), req)
		if err != nil {
			log.Printf("[IAM] Permission check failed for user %s: %v", userID, err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":      500,
				"message":   "Permission check failed",
				"timestamp": time.Now(),
			})
			c.Abort()
			return
		}

		// 5. 判断是否允许访问
		if !result.IsAllowed {
			// 设置错误信息到context，供审计日志使用
			denyMsg := fmt.Sprintf("Permission denied: %s (required: %s, effective: %s)",
				result.DenyReason, requiredLevel, result.EffectiveLevel.String())
			c.Set("error", denyMsg)

			c.JSON(http.StatusForbidden, gin.H{
				"code":            403,
				"message":         "Permission denied",
				"deny_reason":     result.DenyReason,
				"required_level":  requiredLevel,
				"effective_level": result.EffectiveLevel.String(),
				"timestamp":       time.Now(),
			})
			c.Abort()
			return
		}

		// 6. 权限检查通过，继续处理请求
		c.Set("permission_check_result", result)
		c.Next()
	}
}

// RequireAnyPermission 要求任意一个权限即可（OR逻辑）
func (m *IAMPermissionMiddleware) RequireAnyPermission(
	permissions []PermissionRequirement,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 0. 系统管理员直接通过（is_system_admin 仅在系统初始化时设置）
		if isSystemAdmin, _ := c.Get("is_system_admin"); isSystemAdmin == true {
			c.Next()
			return
		}

		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":      401,
				"message":   "User not authenticated",
				"timestamp": time.Now(),
			})
			c.Abort()
			return
		}

		// 检查是否有任意一个权限满足
		// 注意：scope_id的解析需要根据每个权限要求的scope_type分别处理
		// ORGANIZATION scope使用org_id（默认1），其他scope使用路径参数:id
		for _, perm := range permissions {
			rt, _ := valueobject.ParseResourceType(perm.ResourceType)
			st, _ := valueobject.ParseScopeType(perm.ScopeType)
			rl, _ := valueobject.ParsePermissionLevel(perm.RequiredLevel)

			var scopeIDUint uint
			var scopeIDStr string

			if st == valueobject.ScopeTypeOrganization {
				// 组织级别权限：使用org_id查询参数
				// TODO: 多组织架构时需要移除默认值
				scopeID := c.Query("org_id")
				if scopeID == "" {
					scopeID = "1"
				}
				if _, err := fmt.Sscanf(scopeID, "%d", &scopeIDUint); err != nil || scopeIDUint == 0 {
					scopeIDUint = 1
				}
			} else {
				// 工作空间或项目级别权限：从路径参数获取，必须显式指定
				scopeID := c.Param("id")
				if scopeID == "" {
					scopeID = c.Query("scope_id")
				}
				if scopeID == "" {
					// 跳过此权限检查（RequireAnyPermission 中其他权限可能匹配）
					continue
				}
				if _, err := fmt.Sscanf(scopeID, "%d", &scopeIDUint); err != nil || scopeIDUint == 0 {
					scopeIDStr = scopeID
					scopeIDUint = 0
				}
			}

			req := &service.CheckPermissionRequest{
				UserID:        userID.(string),
				ResourceType:  rt,
				ScopeType:     st,
				ScopeID:       scopeIDUint,
				ScopeIDStr:    scopeIDStr,
				RequiredLevel: rl,
			}

			result, err := m.permissionChecker.CheckPermission(c.Request.Context(), req)
			if err == nil && result.IsAllowed {
				// 有一个权限满足，允许访问
				c.Set("permission_check_result", result)
				c.Next()
				return
			}
		}

		// 所有权限都不满足
		// 设置错误信息到context，供审计日志使用
		c.Set("error", "Permission denied: none of the required permissions are granted")

		c.JSON(http.StatusForbidden, gin.H{
			"code":      403,
			"message":   "Permission denied: none of the required permissions are granted",
			"timestamp": time.Now(),
		})
		c.Abort()
	}
}

// PermissionRequirement 权限要求
type PermissionRequirement struct {
	ResourceType  string
	ScopeType     string
	RequiredLevel string
}

