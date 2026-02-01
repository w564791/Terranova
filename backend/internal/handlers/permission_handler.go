package handlers

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"iac-platform/internal/application/service"
	"iac-platform/internal/domain/valueobject"
)

// PermissionHandler 权限管理Handler
type PermissionHandler struct {
	permissionService service.PermissionService
	permissionChecker service.PermissionChecker
	teamService       service.TeamService
	db                *gorm.DB
}

// NewPermissionHandler 创建权限管理Handler实例
func NewPermissionHandler(
	permissionService service.PermissionService,
	permissionChecker service.PermissionChecker,
	teamService service.TeamService,
	db *gorm.DB,
) *PermissionHandler {
	return &PermissionHandler{
		permissionService: permissionService,
		permissionChecker: permissionChecker,
		teamService:       teamService,
		db:                db,
	}
}

// CheckPermissionRequest 权限检查请求
type CheckPermissionRequest struct {
	ResourceType  string `json:"resource_type" binding:"required"`
	ScopeType     string `json:"scope_type" binding:"required"`
	ScopeID       string `json:"scope_id" binding:"required"` // 支持语义化ID和数字ID
	RequiredLevel string `json:"required_level" binding:"required"`
}

// CheckPermission 检查权限
// @Summary 检查用户权限
// @Tags Permission
// @Accept json
// @Produce json
// @Param request body CheckPermissionRequest true "权限检查请求"
// @Success 200 {object} service.CheckPermissionResult
// @Router /api/v1/permissions/check [post]
func (h *PermissionHandler) CheckPermission(c *gin.Context) {
	var req CheckPermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取当前用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// 解析参数
	resourceType, err := valueobject.ParseResourceType(req.ResourceType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	scopeType, err := valueobject.ParseScopeType(req.ScopeType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	requiredLevel, err := valueobject.ParsePermissionLevel(req.RequiredLevel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 检查权限
	checkReq := &service.CheckPermissionRequest{
		UserID:        userID.(string),
		ResourceType:  resourceType,
		ScopeType:     scopeType,
		ScopeIDStr:    req.ScopeID, // 使用字符串类型的 scope_id
		RequiredLevel: requiredLevel,
	}

	result, err := h.permissionChecker.CheckPermission(c.Request.Context(), checkReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GrantPermissionRequest 授予权限请求
type GrantPermissionRequest struct {
	ScopeType       string  `json:"scope_type" binding:"required"`
	ScopeID         uint    `json:"scope_id" binding:"required"`
	PrincipalType   string  `json:"principal_type" binding:"required"`
	PrincipalID     string  `json:"principal_id" binding:"required"`
	PermissionID    string  `json:"permission_id" binding:"required"` // 业务语义ID
	PermissionLevel string  `json:"permission_level" binding:"required"`
	ExpiresAt       *string `json:"expires_at,omitempty"`
	Reason          string  `json:"reason,omitempty"`
}

// GrantPermission 授予权限
// @Summary 授予权限
// @Tags Permission
// @Accept json
// @Produce json
// @Param request body GrantPermissionRequest true "授予权限请求"
// @Success 200 {object} map[string]string
// @Router /api/v1/permissions/grant [post]
func (h *PermissionHandler) GrantPermission(c *gin.Context) {
	var req GrantPermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取当前用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// 解析参数
	scopeType, err := valueobject.ParseScopeType(req.ScopeType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	principalType, err := valueobject.ParsePrincipalType(req.PrincipalType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	permissionLevel, err := valueobject.ParsePermissionLevel(req.PermissionLevel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 授予权限
	grantReq := &service.GrantPermissionRequest{
		ScopeType:       scopeType,
		ScopeID:         req.ScopeID,
		PrincipalType:   principalType,
		PrincipalID:     req.PrincipalID,
		PermissionID:    req.PermissionID,
		PermissionLevel: permissionLevel,
		GrantedBy:       userID.(string),
		Reason:          req.Reason,
	}

	if err := h.permissionService.GrantPermission(c.Request.Context(), grantReq); err != nil {
		// 检查是否是权限冲突错误
		errMsg := err.Error()
		if contains(errMsg, "permission already exists") {
			c.JSON(http.StatusConflict, gin.H{"error": errMsg})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Permission granted successfully"})
}

// contains 检查字符串是否包含子串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// BatchGrantPermissionItem 批量授权项
type BatchGrantPermissionItem struct {
	PermissionID    string `json:"permission_id" binding:"required"` // 业务语义ID
	PermissionLevel string `json:"permission_level" binding:"required"`
}

// BatchGrantPermissionRequest 批量授予权限请求
type BatchGrantPermissionRequest struct {
	ScopeType     string                     `json:"scope_type" binding:"required"`
	ScopeID       interface{}                `json:"scope_id" binding:"required"` // 支持 uint 和 string
	PrincipalType string                     `json:"principal_type" binding:"required"`
	PrincipalID   string                     `json:"principal_id" binding:"required"`
	Permissions   []BatchGrantPermissionItem `json:"permissions" binding:"required,min=1"`
	ExpiresAt     *string                    `json:"expires_at,omitempty"`
	Reason        string                     `json:"reason,omitempty"`
}

// PermissionConflict 权限冲突详情
type PermissionConflict struct {
	PermissionID   string `json:"permission_id"`
	PermissionName string `json:"permission_name"`
	ExistingLevel  string `json:"existing_level"`
	RequestedLevel string `json:"requested_level"`
	ErrorMessage   string `json:"error_message"`
}

// BatchGrantPermissions 批量授予权限
// @Summary 批量授予权限
// @Tags Permission
// @Accept json
// @Produce json
// @Param request body BatchGrantPermissionRequest true "批量授予权限请求"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/permissions/batch-grant [post]
func (h *PermissionHandler) BatchGrantPermissions(c *gin.Context) {
	var req BatchGrantPermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取当前用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// 解析参数
	scopeType, err := valueobject.ParseScopeType(req.ScopeType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	principalType, err := valueobject.ParsePrincipalType(req.PrincipalType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 转换 ScopeID（支持 uint 和 string）
	var scopeID uint
	switch v := req.ScopeID.(type) {
	case float64:
		scopeID = uint(v)
	case string:
		// 如果是字符串，尝试解析为数字
		if parsed, err := strconv.ParseUint(v, 10, 32); err == nil {
			scopeID = uint(parsed)
		} else if scopeType == valueobject.ScopeTypeWorkspace {
			// 如果是 workspace 且不是数字，通过语义化 ID 查询数字 ID
			var workspace struct {
				ID uint `gorm:"column:id"`
			}
			if err := h.db.Table("workspaces").
				Select("id").
				Where("workspace_id = ?", v).
				First(&workspace).Error; err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("workspace not found: %s", v)})
				return
			}
			scopeID = workspace.ID
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope_id format"})
			return
		}
	case uint:
		scopeID = v
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope_id type"})
		return
	}

	// 批量授予权限
	successCount := 0
	failedCount := 0
	errors := []string{}
	conflicts := []PermissionConflict{}

	for _, item := range req.Permissions {
		permissionLevel, err := valueobject.ParsePermissionLevel(item.PermissionLevel)
		if err != nil {
			failedCount++
			errors = append(errors, err.Error())
			continue
		}

		grantReq := &service.GrantPermissionRequest{
			ScopeType:       scopeType,
			ScopeID:         scopeID,
			PrincipalType:   principalType,
			PrincipalID:     req.PrincipalID,
			PermissionID:    item.PermissionID,
			PermissionLevel: permissionLevel,
			GrantedBy:       userID.(string),
			Reason:          req.Reason,
		}

		if err := h.permissionService.GrantPermission(c.Request.Context(), grantReq); err != nil {
			failedCount++
			errMsg := err.Error()
			errors = append(errors, errMsg)

			// 检查是否是权限冲突错误，提取结构化信息
			if contains(errMsg, "permission already exists") {
				// 获取权限定义名称
				permDef, defErr := h.permissionService.GetPermissionDefinitionByID(c.Request.Context(), item.PermissionID)
				permName := item.PermissionID
				if defErr == nil && permDef != nil {
					permName = permDef.DisplayName
				}

				// 提取现有权限级别
				existingLevel := "UNKNOWN"
				if levelMatch := extractLevel(errMsg); levelMatch != "" {
					existingLevel = levelMatch
				}

				conflicts = append(conflicts, PermissionConflict{
					PermissionID:   item.PermissionID,
					PermissionName: permName,
					ExistingLevel:  existingLevel,
					RequestedLevel: item.PermissionLevel,
					ErrorMessage:   errMsg,
				})
			}
		} else {
			successCount++
		}
	}

	// 如果所有操作都失败，返回错误状态码
	if successCount == 0 && failedCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"message":       "Batch grant failed",
			"success_count": successCount,
			"failed_count":  failedCount,
			"errors":        errors,
			"conflicts":     conflicts,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "Batch grant completed",
		"success_count": successCount,
		"failed_count":  failedCount,
		"errors":        errors,
		"conflicts":     conflicts,
	})
}

// extractLevel 从错误信息中提取权限级别
func extractLevel(errMsg string) string {
	// 查找 "level: " 的位置
	levelPrefix := "level: "
	idx := -1
	for i := 0; i <= len(errMsg)-len(levelPrefix); i++ {
		if errMsg[i:i+len(levelPrefix)] == levelPrefix {
			idx = i
			break
		}
	}

	if idx == -1 {
		return ""
	}

	// 跳过 "level: " 提取级别值
	start := idx + len(levelPrefix)
	end := start

	// 找到级别值的结束位置（空格、括号或逗号）
	for end < len(errMsg) {
		ch := errMsg[end]
		if ch == ' ' || ch == ')' || ch == ',' {
			break
		}
		end++
	}

	if end > start {
		return errMsg[start:end]
	}

	return ""
}

// GrantPresetRequest 授予预设权限请求
type GrantPresetRequest struct {
	ScopeType     string `json:"scope_type" binding:"required"`
	ScopeID       uint   `json:"scope_id" binding:"required"`
	PrincipalType string `json:"principal_type" binding:"required"`
	PrincipalID   string `json:"principal_id" binding:"required"`
	PresetName    string `json:"preset_name" binding:"required"` // READ/WRITE/ADMIN
	Reason        string `json:"reason,omitempty"`
}

// GrantPresetPermissions 授予预设权限
// @Summary 授予预设权限集
// @Tags Permission
// @Accept json
// @Produce json
// @Param request body GrantPresetRequest true "授予预设权限请求"
// @Success 200 {object} map[string]string
// @Router /api/v1/permissions/grant-preset [post]
func (h *PermissionHandler) GrantPresetPermissions(c *gin.Context) {
	var req GrantPresetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取当前用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// 解析参数
	scopeType, err := valueobject.ParseScopeType(req.ScopeType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	principalType, err := valueobject.ParsePrincipalType(req.PrincipalType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 授予预设权限
	grantReq := &service.GrantPresetRequest{
		ScopeType:     scopeType,
		ScopeID:       req.ScopeID,
		PrincipalType: principalType,
		PrincipalID:   req.PrincipalID,
		PresetName:    req.PresetName,
		GrantedBy:     userID.(string),
		Reason:        req.Reason,
	}

	if err := h.permissionService.GrantPresetPermissions(c.Request.Context(), grantReq); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Preset permissions granted successfully"})
}

// RevokePermission 撤销权限
// @Summary 撤销权限
// @Tags Permission
// @Param scope_type path string true "作用域类型"
// @Param id path int true "权限分配ID"
// @Success 200 {object} map[string]string
// @Router /api/v1/permissions/{scope_type}/{id} [delete]
func (h *PermissionHandler) RevokePermission(c *gin.Context) {
	scopeTypeStr := c.Param("scope_type")
	idStr := c.Param("id")

	scopeType, err := valueobject.ParseScopeType(scopeTypeStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	// 获取当前用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// 撤销权限
	revokeReq := &service.RevokePermissionRequest{
		ScopeType:    scopeType,
		AssignmentID: uint(id),
		RevokedBy:    userID.(string),
	}

	if err := h.permissionService.RevokePermission(c.Request.Context(), revokeReq); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Permission revoked successfully"})
}

// ListPermissions 列出权限
// @Summary 列出指定作用域的所有权限
// @Tags Permission
// @Param scope_type path string true "作用域类型"
// @Param scope_id path int true "作用域ID"
// @Success 200 {array} entity.PermissionGrant
// @Router /api/v1/permissions/{scope_type}/{scope_id} [get]
func (h *PermissionHandler) ListPermissions(c *gin.Context) {
	scopeTypeStr := c.Param("scope_type")
	scopeIDStr := c.Param("scope_id")

	scopeType, err := valueobject.ParseScopeType(scopeTypeStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	scopeID, err := strconv.ParseUint(scopeIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope_id"})
		return
	}

	// 列出权限
	permissions, err := h.permissionService.ListPermissions(c.Request.Context(), scopeType, uint(scopeID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"permissions": permissions,
		"total":       len(permissions),
	})
}

// ListPermissionDefinitions 列出所有权限定义
// @Summary 列出所有权限定义
// @Tags Permission
// @Success 200 {array} entity.PermissionDefinition
// @Router /api/v1/permissions/definitions [get]
func (h *PermissionHandler) ListPermissionDefinitions(c *gin.Context) {
	definitions, err := h.permissionService.ListPermissionDefinitions(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"definitions": definitions,
		"total":       len(definitions),
	})
}

// ListUserPermissions 列出用户的所有权限
// @Summary 列出用户的所有权限（跨所有作用域）
// @Tags Permission
// @Param id path string true "用户ID（语义化ID）"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/iam/users/{id}/permissions [get]
func (h *PermissionHandler) ListUserPermissions(c *gin.Context) {
	targetUserID := c.Param("id")

	// 获取当前登录用户信息
	currentUserID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	currentUserRole, _ := c.Get("role")

	// 安全检查：用户只能查询自己的权限，除非是admin
	if currentUserRole != "admin" && currentUserID.(string) != targetUserID {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "You can only view your own permissions. Admin access required to view other users' permissions.",
		})
		return
	}

	// 列出用户的所有权限
	permissions, err := h.permissionService.ListPermissionsByPrincipal(
		c.Request.Context(),
		valueobject.PrincipalTypeUser,
		targetUserID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  permissions,
		"total": len(permissions),
	})
}

// ListTeamPermissions 列出团队的所有权限
// @Summary 列出团队的所有权限（跨所有作用域）
// @Tags Permission
// @Param id path int true "团队ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/iam/teams/{id}/permissions [get]
func (h *PermissionHandler) ListTeamPermissions(c *gin.Context) {
	teamID := c.Param("id")

	// 获取当前登录用户信息
	_, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	currentUserRole, _ := c.Get("role")

	// 安全检查：只有admin才能查询团队权限
	// 注意：这里不检查团队成员身份，因为权限管理页面需要admin查看所有团队权限
	if currentUserRole != "admin" {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Admin access required to view team permissions.",
		})
		return
	}

	// 列出团队的所有权限
	permissions, err := h.permissionService.ListPermissionsByPrincipal(
		c.Request.Context(),
		valueobject.PrincipalTypeTeam,
		teamID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  permissions,
		"total": len(permissions),
	})
}
