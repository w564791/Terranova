package middleware

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"iac-platform/internal/application/service"
	"iac-platform/internal/domain/valueobject"
	"iac-platform/internal/iam"
)

// IAMPermissionMiddleware IAM权限检查中间件
type IAMPermissionMiddleware struct {
	permissionChecker   service.PermissionChecker
	workspaceListAccess service.WorkspaceListAccessResolver
}

// NewIAMPermissionMiddleware 创建IAM权限中间件
func NewIAMPermissionMiddleware(db *gorm.DB) *IAMPermissionMiddleware {
	factory := iam.NewServiceFactory(db)
	checker := factory.GetPermissionChecker()
	return &IAMPermissionMiddleware{
		permissionChecker:   checker,
		workspaceListAccess: service.NewWorkspaceListAccessService(db, checker),
	}
}

// NewIAMPermissionMiddlewareWithChecker 注入自定义 checker（测试 / 自定义 wiring）
func NewIAMPermissionMiddlewareWithChecker(checker service.PermissionChecker) *IAMPermissionMiddleware {
	return &IAMPermissionMiddleware{permissionChecker: checker}
}

// principalFromContext 从 JWT 上下文解析主体（USER / TEAM / APPLICATION）
// 业务 IAM 不再旁路 system_admin；平台级 API 请使用 RequireSystemAdmin()。
func principalFromContext(c *gin.Context) (userID string, pt valueobject.PrincipalType, pid string, ok bool) {
	raw, exists := c.Get("user_id")
	if !exists {
		return "", "", "", false
	}
	userID, _ = raw.(string)
	if userID == "" {
		return "", "", "", false
	}

	pt = valueobject.PrincipalTypeUser
	if v, exists := c.Get("principal_type"); exists {
		if s, ok := v.(string); ok && s != "" {
			if parsed, err := valueobject.ParsePrincipalType(s); err == nil {
				pt = parsed
			}
		}
	}
	if v, exists := c.Get("principal_id"); exists {
		if s, ok := v.(string); ok && s != "" {
			pid = s
		}
	}
	if pid == "" {
		pid = userID
	}
	return userID, pt, pid, true
}

// resolveOrgScopeID 解析组织 scope_id：path :org_id → path :id(组织路由) → query → context auth_org_id
// 多租户：缺失 org_id 返回 error（400）。
// 单租户：仅当 IAM_SINGLE_TENANT=1|true 时才默认 org=1（C5）。
// Application 鉴权（选项 A）会在 AgentAuth 写入 auth_org_id，可作回退（不得跨 org 覆盖 query）。
func resolveOrgScopeID(c *gin.Context) (uint, error) {
	scopeID := c.Param("org_id")
	if scopeID == "" {
		if strings.Contains(c.FullPath(), "/organizations/") || strings.Contains(c.FullPath(), "/orgs/") {
			scopeID = c.Param("id")
		}
	}
	if scopeID == "" {
		scopeID = c.Query("org_id")
	}
	if scopeID == "" {
		// Application principal：使用密钥所属 org
		if raw, ok := c.Get("auth_org_id"); ok {
			switch v := raw.(type) {
			case uint:
				if v > 0 {
					return v, nil
				}
			case int:
				if v > 0 {
					return uint(v), nil
				}
			case float64:
				if v > 0 {
					return uint(v), nil
				}
			}
		}
	}
	if scopeID == "" {
		if isSingleTenantIAM() {
			log.Printf("[IAM] org scope missing org_id on %s %s, IAM_SINGLE_TENANT defaulting to 1",
				c.Request.Method, c.Request.URL.Path)
			scopeID = "1"
		} else {
			return 0, fmt.Errorf("org_id is required")
		}
	}
	var scopeIDUint uint
	if _, err := fmt.Sscanf(scopeID, "%d", &scopeIDUint); err != nil || scopeIDUint == 0 {
		return 0, fmt.Errorf("invalid org_id")
	}
	// Application：query/path 指定的 org 不得超出 app 所属 org
	if raw, ok := c.Get("auth_org_id"); ok {
		var appOrg uint
		switch v := raw.(type) {
		case uint:
			appOrg = v
		case int:
			if v > 0 {
				appOrg = uint(v)
			}
		case float64:
			if v > 0 {
				appOrg = uint(v)
			}
		}
		if appOrg > 0 && scopeIDUint != appOrg {
			if pt, exists := c.Get("principal_type"); exists {
				if s, ok := pt.(string); ok && strings.EqualFold(s, "APPLICATION") {
					return 0, fmt.Errorf("org_id %d outside application org %d", scopeIDUint, appOrg)
				}
			}
		}
	}
	return scopeIDUint, nil
}

// isSingleTenantIAM 是否启用单租户 org 默认 1（环境变量 IAM_SINGLE_TENANT）
func isSingleTenantIAM() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("IAM_SINGLE_TENANT")))
	return v == "1" || v == "true" || v == "yes"
}

// AuthOrgID returns the organization resolved by an IAM permission middleware.
// Handlers use this value as the data-access boundary; callers must reject a
// missing value instead of falling back to an unscoped query.
func AuthOrgID(c *gin.Context) (uint, bool) {
	raw, ok := c.Get("auth_org_id")
	if !ok {
		return 0, false
	}

	switch v := raw.(type) {
	case uint:
		return v, v > 0
	case int:
		if v > 0 {
			return uint(v), true
		}
	case float64:
		if v > 0 {
			return uint(v), true
		}
	}

	return 0, false
}

// RequirePermission 要求特定权限的中间件工厂函数
// 用法: router.GET("/path", iamMiddleware.RequirePermission("WORKSPACE_EXECUTION", "ORGANIZATION", "READ"))
func (m *IAMPermissionMiddleware) RequirePermission(
	resourceType string,
	scopeType string,
	requiredLevel string,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, principalType, principalID, ok := principalFromContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":      401,
				"message":   "User not authenticated",
				"timestamp": time.Now(),
			})
			c.Abort()
			return
		}

		var scopeIDUint uint
		var scopeIDStr string

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

		if st == valueobject.ScopeTypeOrganization {
			scopeIDUint, err = resolveOrgScopeID(c)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"code":      400,
					"message":   "invalid org_id",
					"timestamp": time.Now(),
				})
				c.Abort()
				return
			}
			// 供 handler 二次绑定（Application / Role assignment 等防跨 org）
			c.Set("auth_org_id", scopeIDUint)
		} else {
			scopeID := c.Param("id")
			if scopeID == "" {
				scopeID = c.Query("scope_id")
			}
			if scopeID == "" {
				log.Printf("[IAM] Missing scope_id for %s %s, principal=%s/%s",
					c.Request.Method, c.Request.URL.Path, principalType, principalID)
				c.JSON(http.StatusBadRequest, gin.H{
					"code":      400,
					"message":   "scope_id or path parameter :id is required",
					"timestamp": time.Now(),
				})
				c.Abort()
				return
			}
			if _, err := fmt.Sscanf(scopeID, "%d", &scopeIDUint); err != nil || scopeIDUint == 0 {
				scopeIDStr = scopeID
				scopeIDUint = 0
			}
		}

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

		req := &service.CheckPermissionRequest{
			UserID:        userID,
			PrincipalType: principalType,
			PrincipalID:   principalID,
			ResourceType:  rt,
			ScopeType:     st,
			ScopeID:       scopeIDUint,
			ScopeIDStr:    scopeIDStr,
			RequiredLevel: rl,
		}

		result, err := m.permissionChecker.CheckPermission(c.Request.Context(), req)
		if err != nil {
			log.Printf("[IAM] Permission check failed for principal %s/%s: %v", principalType, principalID, err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":      500,
				"message":   "Permission check failed",
				"timestamp": time.Now(),
			})
			c.Abort()
			return
		}

		if !result.IsAllowed {
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

		c.Set("permission_check_result", result)
		c.Next()
	}
}

// RequireWorkspaceListAccess authorizes GET /workspaces without assuming that
// every authorized principal has an organization-wide WORKSPACES grant. It
// keeps the organization-level fast path, while scoped Role/direct/team grants
// are converted to an explicit allow-list that the controller must use in its
// SQL query. Thus a project/workspace grant can enumerate its descendants but
// never sibling workspaces.
//
// A principal with no list capability in the selected organization receives the
// same 403 shape as the previous organization-only route. A valid org/project
// grant receives 200 even when its currently visible workspace set is empty.
func (m *IAMPermissionMiddleware) RequireWorkspaceListAccess() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, principalType, principalID, ok := principalFromContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":      401,
				"message":   "User not authenticated",
				"timestamp": time.Now(),
			})
			c.Abort()
			return
		}

		orgID, err := resolveOrgScopeID(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":      400,
				"message":   "invalid org_id",
				"timestamp": time.Now(),
			})
			c.Abort()
			return
		}
		c.Set("auth_org_id", orgID)

		if m.workspaceListAccess == nil {
			log.Printf("[IAM] workspace list access resolver is not configured")
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":      500,
				"message":   "Permission check failed",
				"timestamp": time.Now(),
			})
			c.Abort()
			return
		}

		access, err := m.workspaceListAccess.ResolveWorkspaceListAccess(c.Request.Context(), service.WorkspaceListAccessRequest{
			UserID:        userID,
			PrincipalType: principalType,
			PrincipalID:   principalID,
			OrgID:         orgID,
		})
		if err != nil {
			log.Printf("[IAM] workspace list permission check failed for %s/%s in org %d: %v",
				principalType, principalID, orgID, err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":      500,
				"message":   "Permission check failed",
				"timestamp": time.Now(),
			})
			c.Abort()
			return
		}

		if access == nil || !access.HasAccess {
			c.Set("error", "Permission denied: no readable workspaces in organization")
			c.JSON(http.StatusForbidden, gin.H{
				"code":      403,
				"message":   "Permission denied",
				"timestamp": time.Now(),
			})
			c.Abort()
			return
		}

		c.Set(service.WorkspaceListAccessContextKey, access)
		c.Next()
	}
}

// CheckWorkspaceOrOrgWorkspacesRead 允许：
//   - WORKSPACE_MANAGEMENT READ @ workspace，或
//   - WORKSPACES READ @ organization（鉴权 org）
//
// 用于 Application principal 读 workspace 详情等。失败时写 403/401 并返回 false。
func (m *IAMPermissionMiddleware) CheckWorkspaceOrOrgWorkspacesRead(c *gin.Context, workspaceSemanticID string) bool {
	userID, principalType, principalID, ok := principalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code": 401, "message": "User not authenticated", "timestamp": time.Now(),
		})
		return false
	}
	// 1) workspace 级
	wsReq := &service.CheckPermissionRequest{
		UserID:        userID,
		PrincipalType: principalType,
		PrincipalID:   principalID,
		ResourceType:  valueobject.ResourceTypeWorkspaceManagement,
		ScopeType:     valueobject.ScopeTypeWorkspace,
		ScopeIDStr:    workspaceSemanticID,
		RequiredLevel: valueobject.PermissionLevelRead,
	}
	if m.permissionChecker != nil {
		if r, err := m.permissionChecker.CheckPermission(c.Request.Context(), wsReq); err == nil && r != nil && r.IsAllowed {
			return true
		}
	}
	// 2) org 级 WORKSPACES READ
	orgID, err := resolveOrgScopeID(c)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"code": 403, "message": "Permission denied", "timestamp": time.Now(),
		})
		return false
	}
	orgReq := &service.CheckPermissionRequest{
		UserID:        userID,
		PrincipalType: principalType,
		PrincipalID:   principalID,
		ResourceType:  valueobject.ResourceTypeAllWorkspaces,
		ScopeType:     valueobject.ScopeTypeOrganization,
		ScopeID:       orgID,
		RequiredLevel: valueobject.PermissionLevelRead,
	}
	if m.permissionChecker != nil {
		if r, err := m.permissionChecker.CheckPermission(c.Request.Context(), orgReq); err == nil && r != nil && r.IsAllowed {
			return true
		}
	}
	c.JSON(http.StatusForbidden, gin.H{
		"code": 403, "message": "Permission denied", "timestamp": time.Now(),
	})
	return false
}

// RequireWorkspacePermission 在 handler 内部对"运行时才知道的 workspace"做权限校验。
// 系统管理员不再旁路；需持有对应 Role/grant。
func (m *IAMPermissionMiddleware) RequireWorkspacePermission(
	c *gin.Context, workspaceID string, requiredLevel string,
) bool {
	userID, principalType, principalID, ok := principalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code": 401, "message": "User not authenticated", "timestamp": time.Now(),
		})
		return false
	}

	rt, err := valueobject.ParseResourceType("WORKSPACE_MANAGEMENT")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500, "message": "Invalid resource type", "timestamp": time.Now(),
		})
		return false
	}
	rl, err := valueobject.ParsePermissionLevel(requiredLevel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400, "message": "Invalid permission level", "timestamp": time.Now(),
		})
		return false
	}

	req := &service.CheckPermissionRequest{
		UserID:        userID,
		PrincipalType: principalType,
		PrincipalID:   principalID,
		ResourceType:  rt,
		ScopeType:     valueobject.ScopeTypeWorkspace,
		ScopeIDStr:    workspaceID,
		RequiredLevel: rl,
	}
	result, err := m.permissionChecker.CheckPermission(c.Request.Context(), req)
	if err != nil {
		log.Printf("[IAM] Workspace permission check failed for %s/%s ws %s: %v",
			principalType, principalID, workspaceID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500, "message": "Permission check failed", "timestamp": time.Now(),
		})
		return false
	}
	if !result.IsAllowed {
		denyMsg := fmt.Sprintf("Permission denied on workspace %s (required: %s, effective: %s)",
			workspaceID, requiredLevel, result.EffectiveLevel.String())
		c.Set("error", denyMsg)
		c.JSON(http.StatusForbidden, gin.H{
			"code": 403, "message": "Permission denied", "deny_reason": result.DenyReason,
			"required_level": requiredLevel, "effective_level": result.EffectiveLevel.String(),
			"timestamp": time.Now(),
		})
		return false
	}
	return true
}

// RequireAnyPermission 要求任意一个权限即可（OR逻辑）
func (m *IAMPermissionMiddleware) RequireAnyPermission(
	permissions []PermissionRequirement,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, principalType, principalID, ok := principalFromContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":      401,
				"message":   "User not authenticated",
				"timestamp": time.Now(),
			})
			c.Abort()
			return
		}

		// 预解析 org scope；非法则 400（与 RequirePermission 对齐）
		var orgScopeID uint
		var orgScopeErr error
		needOrg := false
		for _, perm := range permissions {
			if perm.ScopeType == "ORGANIZATION" {
				needOrg = true
				break
			}
		}
		if needOrg {
			orgScopeID, orgScopeErr = resolveOrgScopeID(c)
			if orgScopeErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"code":      400,
					"message":   "invalid org_id",
					"timestamp": time.Now(),
				})
				c.Abort()
				return
			}
			// 与 RequirePermission 对齐：供 handler/二次绑定使用
			c.Set("auth_org_id", orgScopeID)
		}

		for _, perm := range permissions {
			rt, err := valueobject.ParseResourceType(perm.ResourceType)
			if err != nil {
				continue
			}
			st, err := valueobject.ParseScopeType(perm.ScopeType)
			if err != nil {
				continue
			}
			rl, err := valueobject.ParsePermissionLevel(perm.RequiredLevel)
			if err != nil {
				continue
			}

			var scopeIDUint uint
			var scopeIDStr string

			if st == valueobject.ScopeTypeOrganization {
				scopeIDUint = orgScopeID
			} else {
				scopeID := c.Param("id")
				if scopeID == "" {
					scopeID = c.Query("scope_id")
				}
				if scopeID == "" {
					continue
				}
				if _, err := fmt.Sscanf(scopeID, "%d", &scopeIDUint); err != nil || scopeIDUint == 0 {
					scopeIDStr = scopeID
					scopeIDUint = 0
				}
			}

			req := &service.CheckPermissionRequest{
				UserID:        userID,
				PrincipalType: principalType,
				PrincipalID:   principalID,
				ResourceType:  rt,
				ScopeType:     st,
				ScopeID:       scopeIDUint,
				ScopeIDStr:    scopeIDStr,
				RequiredLevel: rl,
			}

			result, err := m.permissionChecker.CheckPermission(c.Request.Context(), req)
			if err == nil && result.IsAllowed {
				c.Set("permission_check_result", result)
				c.Next()
				return
			}
		}

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

// EnforceWorkspaceOrgBinding 对带 path :id 的 workspace 路由强制「资源 ∈ 鉴权 org」。
// 无 :id（列表/创建）时跳过。跨租户返回 404，避免枚举。
// 应挂在 /workspaces 路由组 JWT 之后；自行 resolve org_id 并写入 auth_org_id。
func EnforceWorkspaceOrgBinding(db *gorm.DB) gin.HandlerFunc {
	return EnforceWorkspaceOrgBindingForParam(db, "id")
}

// EnforceWorkspaceOrgBindingForParam is the same tenant boundary as
// EnforceWorkspaceOrgBinding, for routes whose workspace path parameter is
// not named :id (for example CMDB's :workspace_id).
func EnforceWorkspaceOrgBindingForParam(db *gorm.DB, workspaceParam string) gin.HandlerFunc {
	return func(c *gin.Context) {
		workspaceID := c.Param(workspaceParam)
		if workspaceID == "" {
			c.Next()
			return
		}
		if db == nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code": 500, "message": "Workspace authorization is unavailable", "timestamp": time.Now(),
			})
			c.Abort()
			return
		}

		orgID, err := resolveOrgScopeID(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":      400,
				"message":   "invalid org_id",
				"timestamp": time.Now(),
			})
			c.Abort()
			return
		}
		c.Set("auth_org_id", orgID)

		// 数字主键或语义化 ID → 语义化 workspace_id
		semanticID := workspaceID
		var numID uint
		if _, err := fmt.Sscanf(workspaceID, "%d", &numID); err == nil && numID > 0 {
			var sem string
			if err := db.WithContext(c.Request.Context()).
				Table("workspaces").Select("workspace_id").
				Where("id = ?", numID).Scan(&sem).Error; err != nil || sem == "" {
				c.JSON(http.StatusNotFound, gin.H{
					"code": 404, "message": "Workspace not found", "timestamp": time.Now(),
				})
				c.Abort()
				return
			}
			semanticID = sem
		}

		// A workspace must have exactly one project relationship. If corrupted
		// data contains duplicates, fail closed instead of choosing an arbitrary
		// tenant with LIMIT 1.
		var binding struct {
			RelationCount int64 `gorm:"column:relation_count"`
			OrgID         uint  `gorm:"column:org_id"`
		}
		err = db.WithContext(c.Request.Context()).Raw(`
SELECT COUNT(*) AS relation_count, MIN(p.org_id) AS org_id
FROM workspace_project_relations wpr
JOIN projects p ON p.id = wpr.project_id
WHERE wpr.workspace_id = ?`, semanticID).Scan(&binding).Error
		if err != nil || binding.RelationCount != 1 || binding.OrgID == 0 || binding.OrgID != orgID {
			c.JSON(http.StatusNotFound, gin.H{
				"code": 404, "message": "Workspace not found", "timestamp": time.Now(),
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
