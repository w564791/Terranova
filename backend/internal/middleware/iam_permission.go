package middleware

import (
	"fmt"
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
		// 0. 检查是否为admin角色,admin直接通过
		role, roleExists := c.Get("role")
		if roleExists && role == "admin" {
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
			// 组织级别权限：优先从查询参数获取org_id，否则使用默认值1
			scopeID := c.Query("org_id")
			if scopeID == "" {
				scopeID = "1" // 默认组织ID
			}
			if _, err := fmt.Sscanf(scopeID, "%d", &scopeIDUint); err != nil || scopeIDUint == 0 {
				scopeIDUint = 1
			}
		} else {
			// 工作空间或项目级别权限：从路径参数获取
			scopeID := c.Param("id")
			if scopeID == "" {
				scopeID = c.Query("scope_id")
			}
			if scopeID == "" {
				scopeID = "1"
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
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":      500,
				"message":   "Permission check failed: " + err.Error(),
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

		// 获取scope_id
		scopeID := c.Param("id")
		if scopeID == "" {
			scopeID = c.Query("scope_id")
		}
		if scopeID == "" {
			scopeID = c.Query("org_id")
		}
		if scopeID == "" {
			scopeID = "1"
		}

		var scopeIDUint uint
		var scopeIDStr string

		// 尝试解析为数字，如果失败则保留为字符串（可能是语义化ID）
		if _, err := fmt.Sscanf(scopeID, "%d", &scopeIDUint); err != nil || scopeIDUint == 0 {
			// 不是数字，可能是语义化ID（如 ws-xxx）
			scopeIDStr = scopeID
			scopeIDUint = 0 // 设为0，让CheckPermission通过ScopeIDStr处理
		}

		// 检查是否有任意一个权限满足
		for _, perm := range permissions {
			rt, _ := valueobject.ParseResourceType(perm.ResourceType)
			st, _ := valueobject.ParseScopeType(perm.ScopeType)
			rl, _ := valueobject.ParsePermissionLevel(perm.RequiredLevel)

			req := &service.CheckPermissionRequest{
				UserID:        userID.(string),
				ResourceType:  rt,
				ScopeType:     st,
				ScopeID:       scopeIDUint,
				ScopeIDStr:    scopeIDStr, // 支持语义化ID
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

// BypassIAMForAdmin 为管理员角色绕过IAM检查（临时方案）
// 注意：这是一个临时解决方案，长期应该完全使用IAM权限
func BypassIAMForAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if exists && role == "admin" {
			// 管理员直接通过
			c.Next()
			return
		}

		// 非管理员用户：拒绝访问（因为IAM权限检查尚未完全集成到所有路由）
		// TODO: 当IAM权限检查完全集成到所有路由后，才能移除此限制
		c.JSON(http.StatusForbidden, gin.H{
			"code":      403,
			"message":   "Access denied: Only administrators can access this resource",
			"hint":      "Please contact your administrator to grant you the necessary permissions",
			"timestamp": time.Now(),
		})
		c.Abort()
	}
}
