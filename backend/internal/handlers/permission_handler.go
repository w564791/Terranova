package handlers

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

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

// CheckPermission checks user permission
// @Summary Check user permission
// @Description Check if the current user has a specific permission in a given scope
// @Tags IAM-Permission
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CheckPermissionRequest true "Permission check request"
// @Success 200 {object} service.CheckPermissionResult
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/permissions/check [post]
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

	// 解析参数（业务权限不再旁路 system_admin，与 IAM 中间件一致）
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

	// 主体：默认 USER；Team Token 等由中间件设置 principal_type/principal_id
	principalType := valueobject.PrincipalTypeUser
	principalID := userID.(string)
	if v, ok := c.Get("principal_type"); ok {
		if s, ok := v.(string); ok && s != "" {
			if pt, err := valueobject.ParsePrincipalType(s); err == nil {
				principalType = pt
			}
		}
	}
	if v, ok := c.Get("principal_id"); ok {
		if s, ok := v.(string); ok && s != "" {
			principalID = s
		}
	}

	checkReq := &service.CheckPermissionRequest{
		UserID:        userID.(string),
		PrincipalType: principalType,
		PrincipalID:   principalID,
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

// ErrDirectGrantRetired 公共 Direct Grant 写路径已下线（D5）；请改用 Role 赋值。
// USER/TEAM → POST /iam/users|teams/:id/roles；APPLICATION → POST /iam/applications/:id/roles
const directGrantRetiredMsg = "Direct Grant is retired; assign a Role instead (POST /iam/users|teams|applications/:id/roles)."

// rejectRetiredDirectGrant 拒绝公共 Direct Grant 写入（USER/TEAM/APPLICATION）。
// 环境变量 IAM_ALLOW_DIRECT_GRANT=1 可临时恢复（仅应急）。
// 内部 service（如 workspace 创建 fallback）不经此路径。
func rejectRetiredDirectGrant(c *gin.Context, principalType valueobject.PrincipalType) bool {
	if os.Getenv("IAM_ALLOW_DIRECT_GRANT") == "1" || os.Getenv("IAM_ALLOW_DIRECT_GRANT") == "true" {
		return false
	}
	_ = principalType // 全部主体类型均退役
	c.JSON(http.StatusGone, gin.H{
		"error":       directGrantRetiredMsg,
		"code":        410,
		"deprecated":  true,
		"alternative": "role_assignment",
		"timestamp":   time.Now(),
	})
	return true
}

// GrantPermission grants a permission
// @Summary Grant permission
// @Description Grant a permission to a principal in a specific scope
// @Tags IAM-Permission
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body GrantPermissionRequest true "Grant permission request"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 409 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/permissions/grant [post]
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

	authOrg, ok := requireAuthOrg(c)
	if !ok {
		return
	}

	// 解析参数
	scopeType, err := valueobject.ParseScopeType(req.ScopeType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 目标 scope 必须落在鉴权 org（防跨 org Direct Grant）
	if err := ensureScopeInAuthOrg(c.Request.Context(), h.db, scopeType, req.ScopeID, authOrg); err != nil {
		respondScopeOutsideAuthOrg(c, err)
		return
	}

	principalType, err := valueobject.ParsePrincipalType(req.PrincipalType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if rejectRetiredDirectGrant(c, principalType) {
		return
	}

	principalID := req.PrincipalID
	// APPLICATION：统一存 app_key（与 AgentAuth principal_id 对齐，选项 A）
	if principalType == valueobject.PrincipalTypeApplication {
		resolved, rerr := resolveApplicationPrincipalID(c.Request.Context(), h.db, principalID)
		if rerr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": rerr.Error()})
			return
		}
		principalID = resolved
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
		PrincipalID:     principalID,
		PermissionID:    req.PermissionID,
		PermissionLevel: permissionLevel,
		GrantedBy:       userID.(string),
		Reason:          req.Reason,
	}
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		expiresAt, err := parseFlexibleTime(*req.ExpiresAt)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid expires_at: " + err.Error()})
			return
		}
		grantReq.ExpiresAt = &expiresAt
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

// parseFlexibleTime 解析 expires_at：
// - 带时区：RFC3339 / RFC3339Nano
// - 无时区 datetime-local：按 UTC 解释（避免服务器本地时区偏移）；拒绝已过去时间
func parseFlexibleTime(s string) (time.Time, error) {
	layoutsTZ := []string{time.RFC3339, time.RFC3339Nano}
	for _, layout := range layoutsTZ {
		if t, err := time.Parse(layout, s); err == nil {
			if t.Before(time.Now().Add(-time.Minute)) {
				return time.Time{}, fmt.Errorf("expires_at must be in the future")
			}
			return t, nil
		}
	}
	// 无时区：UTC
	layoutsLocal := []string{
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	var lastErr error
	for _, layout := range layoutsLocal {
		t, err := time.ParseInLocation(layout, s, time.UTC)
		if err == nil {
			if t.Before(time.Now().UTC().Add(-time.Minute)) {
				return time.Time{}, fmt.Errorf("expires_at must be in the future")
			}
			return t, nil
		}
		lastErr = err
	}
	return time.Time{}, fmt.Errorf("unsupported time format %q: %w", s, lastErr)
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

// BatchGrantPermissions grants permissions in batch
// @Summary Batch grant permissions
// @Description Grant multiple permissions to a principal in a specific scope
// @Tags IAM-Permission
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body BatchGrantPermissionRequest true "Batch grant permission request"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /api/v1/iam/permissions/batch-grant [post]
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

	authOrg, ok := requireAuthOrg(c)
	if !ok {
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
	if rejectRetiredDirectGrant(c, principalType) {
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

	if err := ensureScopeInAuthOrg(c.Request.Context(), h.db, scopeType, scopeID, authOrg); err != nil {
		respondScopeOutsideAuthOrg(c, err)
		return
	}

	principalID := req.PrincipalID
	if principalType == valueobject.PrincipalTypeApplication {
		resolved, rerr := resolveApplicationPrincipalID(c.Request.Context(), h.db, principalID)
		if rerr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": rerr.Error()})
			return
		}
		principalID = resolved
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
			PrincipalID:     principalID,
			PermissionID:    item.PermissionID,
			PermissionLevel: permissionLevel,
			GrantedBy:       userID.(string),
			Reason:          req.Reason,
		}
		if req.ExpiresAt != nil && *req.ExpiresAt != "" {
			expiresAt, expErr := parseFlexibleTime(*req.ExpiresAt)
			if expErr != nil {
				failedCount++
				errors = append(errors, "invalid expires_at: "+expErr.Error())
				continue
			}
			grantReq.ExpiresAt = &expiresAt
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

// GrantPresetPermissions grants preset permissions
// @Summary Grant preset permission set
// @Description Grant a predefined set of permissions (READ/WRITE/ADMIN) to a principal
// @Tags IAM-Permission
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body GrantPresetRequest true "Grant preset permission request"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/permissions/grant-preset [post]
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

	authOrg, ok := requireAuthOrg(c)
	if !ok {
		return
	}

	// 解析参数
	scopeType, err := valueobject.ParseScopeType(req.ScopeType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ensureScopeInAuthOrg(c.Request.Context(), h.db, scopeType, req.ScopeID, authOrg); err != nil {
		respondScopeOutsideAuthOrg(c, err)
		return
	}

	principalType, err := valueobject.ParsePrincipalType(req.PrincipalType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if rejectRetiredDirectGrant(c, principalType) {
		return
	}

	principalID := req.PrincipalID
	if principalType == valueobject.PrincipalTypeApplication {
		resolved, rerr := resolveApplicationPrincipalID(c.Request.Context(), h.db, principalID)
		if rerr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": rerr.Error()})
			return
		}
		principalID = resolved
	}

	// 授予预设权限
	grantReq := &service.GrantPresetRequest{
		ScopeType:     scopeType,
		ScopeID:       req.ScopeID,
		PrincipalType: principalType,
		PrincipalID:   principalID,
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

// RevokePermission revokes a permission
// @Summary Revoke permission
// @Description Revoke a permission assignment
// @Tags IAM-Permission
// @Produce json
// @Security BearerAuth
// @Param scope_type path string true "Scope type"
// @Param id path int true "Permission assignment ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/permissions/{scope_type}/{id} [delete]
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

	authOrg, ok := requireAuthOrg(c)
	if !ok {
		return
	}
	// 加载 assignment 所属 scope 并校验落在鉴权 org
	if err := h.ensureAssignmentInAuthOrg(c, scopeType, uint(id), authOrg); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "permission assignment not found"})
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

// ensureAssignmentInAuthOrg 按 assignment 反查 scope 并校验 org
func (h *PermissionHandler) ensureAssignmentInAuthOrg(c *gin.Context, scopeType valueobject.ScopeType, assignmentID, authOrg uint) error {
	ctx := c.Request.Context()
	switch scopeType {
	case valueobject.ScopeTypeOrganization:
		var orgID uint
		if err := h.db.WithContext(ctx).Table("org_permissions").Select("org_id").Where("id = ?", assignmentID).Scan(&orgID).Error; err != nil || orgID == 0 {
			return fmt.Errorf("not found")
		}
		return ensureScopeInAuthOrg(ctx, h.db, scopeType, orgID, authOrg)
	case valueobject.ScopeTypeProject:
		var projectID uint
		if err := h.db.WithContext(ctx).Table("project_permissions").Select("project_id").Where("id = ?", assignmentID).Scan(&projectID).Error; err != nil || projectID == 0 {
			return fmt.Errorf("not found")
		}
		return ensureScopeInAuthOrg(ctx, h.db, scopeType, projectID, authOrg)
	case valueobject.ScopeTypeWorkspace:
		var wsSem string
		if err := h.db.WithContext(ctx).Table("workspace_permissions").Select("workspace_id").Where("id = ?", assignmentID).Scan(&wsSem).Error; err != nil || wsSem == "" {
			return fmt.Errorf("not found")
		}
		return ensureWorkspaceSemanticInAuthOrg(ctx, h.db, wsSem, authOrg)
	default:
		return fmt.Errorf("unsupported scope")
	}
}

// ListPermissions lists permissions for a scope
// @Summary List permissions by scope
// @Description List all permission assignments for a specific scope
// @Tags IAM-Permission
// @Produce json
// @Security BearerAuth
// @Param scope_type path string true "Scope type"
// @Param scope_id path int true "Scope ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/permissions/{scope_type}/{scope_id} [get]
func (h *PermissionHandler) ListPermissions(c *gin.Context) {
	scopeTypeStr := c.Param("scope_type")
	scopeIDStr := c.Param("scope_id")

	scopeType, err := valueobject.ParseScopeType(scopeTypeStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	authOrg, ok := requireAuthOrg(c)
	if !ok {
		return
	}

	// workspace path 支持数字主键或语义化 ID；其它 scope 仅数字
	var scopeID uint
	if scopeType == valueobject.ScopeTypeWorkspace {
		if parsed, err := strconv.ParseUint(scopeIDStr, 10, 32); err == nil {
			scopeID = uint(parsed)
		} else {
			var workspace struct {
				ID uint `gorm:"column:id"`
			}
			if err := h.db.Table("workspaces").Select("id").Where("workspace_id = ?", scopeIDStr).First(&workspace).Error; err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
				return
			}
			scopeID = workspace.ID
		}
	} else {
		parsed, err := strconv.ParseUint(scopeIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope_id"})
			return
		}
		scopeID = uint(parsed)
	}

	if err := ensureScopeInAuthOrg(c.Request.Context(), h.db, scopeType, scopeID, authOrg); err != nil {
		respondScopeOutsideAuthOrg(c, err)
		return
	}

	// 列出权限（workspace 在 repository 内将数字主键转为语义化 ID 查询）
	permissions, err := h.permissionService.ListPermissions(c.Request.Context(), scopeType, scopeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"permissions": permissions,
		"total":       len(permissions),
	})
}

// ListPermissionDefinitions lists all permission definitions
// @Summary List permission definitions
// @Description List all available permission definitions
// @Tags IAM-Permission
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/permissions/definitions [get]
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

// ListUserPermissions lists all permissions for a user
// @Summary List user permissions
// @Description List all permissions for a user across all scopes
// @Tags IAM-User
// @Produce json
// @Security BearerAuth
// @Param id path string true "User ID (semantic ID)"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/users/{id}/permissions [get]
func (h *PermissionHandler) ListUserPermissions(c *gin.Context) {
	targetUserID := c.Param("id")

	// 获取当前登录用户信息
	currentUserID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	isSystemAdmin, _ := c.Get("is_system_admin")

	// 安全检查：用户只能查询自己的权限，除非是系统管理员
	if isSystemAdmin != true && currentUserID.(string) != targetUserID {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "You can only view your own permissions. System admin access required to view other users' permissions.",
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

// ListTeamPermissions lists all permissions for a team
// @Summary List team permissions
// @Description List all permissions for a team across all scopes
// @Tags IAM-Team
// @Produce json
// @Security BearerAuth
// @Param id path string true "Team ID"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/teams/{id}/permissions [get]
func (h *PermissionHandler) ListTeamPermissions(c *gin.Context) {
	teamID := c.Param("id")

	// 获取当前登录用户信息
	_, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	isSystemAdmin, _ := c.Get("is_system_admin")

	// 安全检查：只有系统管理员才能查询团队权限
	if isSystemAdmin != true {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "System admin access required to view team permissions.",
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
