package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"iac-platform/internal/application/service"
	"iac-platform/internal/domain/valueobject"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AuditHandler 审计处理器
type AuditHandler struct {
	service *service.AuditService
	db      *gorm.DB
}

// NewAuditHandler 创建审计处理器实例
func NewAuditHandler(service *service.AuditService) *AuditHandler {
	return &AuditHandler{service: service}
}

// NewAuditHandlerWithDB 带 db 的审计处理器（用于 scope→org 绑定）
func NewAuditHandlerWithDB(svc *service.AuditService, db *gorm.DB) *AuditHandler {
	return &AuditHandler{service: svc, db: db}
}

// QueryPermissionHistory 查询权限变更历史
// @Summary Query permission change history
// @Tags IAM-Audit
// @Produce json
// @Security BearerAuth
// @Param scope_type query string true "Scope type"
// @Param scope_id query int true "Scope ID"
// @Param start_time query string false "Start time (RFC3339)"
// @Param end_time query string false "End time (RFC3339)"
// @Param limit query int false "Result limit"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/v1/iam/audit/permission-history [get]
func (h *AuditHandler) QueryPermissionHistory(c *gin.Context) {
	scopeTypeStr := c.Query("scope_type")
	scopeIDStr := c.Query("scope_id")

	if scopeTypeStr == "" || scopeIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scope_type and scope_id are required"})
		return
	}

	scopeID, err := strconv.ParseUint(scopeIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope_id"})
		return
	}

	authOrg, ok := requireAuthOrg(c)
	if !ok {
		return
	}
	scopeType := valueobject.ScopeType(scopeTypeStr)
	// 需要 db 做 scope 归属校验 — AuditHandler 仅有 service；scope org 校验在无 db 时退化为 org 级仅允许 authOrg
	if scopeType == valueobject.ScopeTypeOrganization {
		if uint(scopeID) != authOrg {
			c.JSON(http.StatusForbidden, gin.H{"error": "scope outside authenticated organization"})
			return
		}
	} else if h.db != nil {
		if err := ensureScopeInAuthOrg(c.Request.Context(), h.db, scopeType, uint(scopeID), authOrg); err != nil {
			respondScopeOutsideAuthOrg(c, err)
			return
		}
	} else {
		// 无 db：非 org scope 拒绝（fail-closed）
		c.JSON(http.StatusInternalServerError, gin.H{"error": "audit org binding not configured"})
		return
	}

	req := &service.QueryPermissionHistoryRequest{
		ScopeType: scopeType,
		ScopeID:   uint(scopeID),
	}

	// 解析时间参数（保持UTC时间，因为数据库存储的是UTC时间）
	if startTimeStr := c.Query("start_time"); startTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, startTimeStr); err == nil {
			req.StartTime = t
		}
	}
	if endTimeStr := c.Query("end_time"); endTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, endTimeStr); err == nil {
			req.EndTime = t
		}
	}
	if limitStr := c.Query("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			req.Limit = limit
		}
	}

	logs, err := h.service.QueryPermissionHistory(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":  logs,
		"total": len(logs),
	})
}

// QueryAccessHistory 查询资源访问历史
// @Summary Query resource access history
// @Tags IAM-Audit
// @Produce json
// @Security BearerAuth
// @Param user_id query int false "User ID"
// @Param resource_type query string false "Resource type"
// @Param method query string false "HTTP method"
// @Param http_code_operator query string false "HTTP status code operator"
// @Param http_code_value query int false "HTTP status code value"
// @Param start_time query string false "Start time (RFC3339)"
// @Param end_time query string false "End time (RFC3339)"
// @Param limit query int false "Result limit"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/iam/audit/access-history [get]
func (h *AuditHandler) QueryAccessHistory(c *gin.Context) {
	// access_logs 无 org 列：全局读仅平台超管（防 org-A 审计读全站访问记录）
	if !requireSystemAdmin(c) {
		return
	}
	req := &service.QueryAccessHistoryRequest{}

	if userIDStr := c.Query("user_id"); userIDStr != "" {
		if userID, err := strconv.ParseUint(userIDStr, 10, 32); err == nil {
			req.UserID = fmt.Sprintf("%d", userID)
		}
	}

	req.ResourceType = c.Query("resource_type")
	req.Method = c.Query("method")

	// 解析HTTP状态码筛选
	httpCodeOperator := c.Query("http_code_operator")
	httpCodeValueStr := c.Query("http_code_value")
	if httpCodeOperator != "" && httpCodeValueStr != "" {
		if httpCodeValue, err := strconv.Atoi(httpCodeValueStr); err == nil && httpCodeValue > 0 {
			req.HttpCodeFilter = &service.HttpCodeFilter{
				Operator: httpCodeOperator,
				Value:    httpCodeValue,
			}
		}
	}

	// 解析时间参数并转换为本地时区（因为数据库存储的是本地时间）
	loc, _ := time.LoadLocation("Asia/Singapore")
	if startTimeStr := c.Query("start_time"); startTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, startTimeStr); err == nil {
			// 将UTC时间转换为本地时区
			req.StartTime = t.In(loc)
		}
	}
	if endTimeStr := c.Query("end_time"); endTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, endTimeStr); err == nil {
			// 将UTC时间转换为本地时区
			req.EndTime = t.In(loc)
		}
	}
	if limitStr := c.Query("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			req.Limit = limit
		}
	}

	logs, err := h.service.QueryAccessHistory(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":  logs,
		"total": len(logs),
	})
}

// QueryDeniedAccess 查询被拒绝的访问记录
// @Summary Query denied access records
// @Tags IAM-Audit
// @Produce json
// @Security BearerAuth
// @Param start_time query string false "Start time (RFC3339)"
// @Param end_time query string false "End time (RFC3339)"
// @Param limit query int false "Result limit"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/iam/audit/denied-access [get]
func (h *AuditHandler) QueryDeniedAccess(c *gin.Context) {
	// 全局拒绝记录无 org 过滤：仅平台超管
	if !requireSystemAdmin(c) {
		return
	}
	req := &service.QueryDeniedAccessRequest{}

	// 解析时间参数并转换为本地时区（因为数据库存储的是本地时间）
	loc, _ := time.LoadLocation("Asia/Singapore")
	if startTimeStr := c.Query("start_time"); startTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, startTimeStr); err == nil {
			// 将UTC时间转换为本地时区
			req.StartTime = t.In(loc)
		}
	}
	if endTimeStr := c.Query("end_time"); endTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, endTimeStr); err == nil {
			// 将UTC时间转换为本地时区
			req.EndTime = t.In(loc)
		}
	}
	if limitStr := c.Query("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			req.Limit = limit
		}
	}

	logs, err := h.service.QueryDeniedAccess(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":  logs,
		"total": len(logs),
	})
}

// QueryPermissionChangesByPrincipal 查询指定主体的权限变更历史
// @Summary Query permission changes by principal
// @Tags IAM-Audit
// @Produce json
// @Security BearerAuth
// @Param principal_type query string true "Principal type"
// @Param principal_id query int true "Principal ID"
// @Param start_time query string false "Start time (RFC3339)"
// @Param end_time query string false "End time (RFC3339)"
// @Param limit query int false "Result limit"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/v1/iam/audit/permission-changes-by-principal [get]
func (h *AuditHandler) QueryPermissionChangesByPrincipal(c *gin.Context) {
	// 跨 scope 主体查询暂无 org 过滤：仅平台超管
	if !requireSystemAdmin(c) {
		return
	}
	principalTypeStr := c.Query("principal_type")
	principalIDStr := c.Query("principal_id")

	if principalTypeStr == "" || principalIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "principal_type and principal_id are required"})
		return
	}

	principalID, err := strconv.ParseUint(principalIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid principal_id"})
		return
	}

	req := &service.QueryPermissionChangesByPrincipalRequest{
		PrincipalType: valueobject.PrincipalType(principalTypeStr),
		PrincipalID:   strconv.FormatUint(principalID, 10),
	}

	// 解析时间参数并转换为本地时区（因为数据库存储的是本地时间）
	loc, _ := time.LoadLocation("Asia/Singapore")
	if startTimeStr := c.Query("start_time"); startTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, startTimeStr); err == nil {
			// 将UTC时间转换为本地时区
			req.StartTime = t.In(loc)
		}
	}
	if endTimeStr := c.Query("end_time"); endTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, endTimeStr); err == nil {
			// 将UTC时间转换为本地时区
			req.EndTime = t.In(loc)
		}
	}
	if limitStr := c.Query("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			req.Limit = limit
		}
	}

	logs, err := h.service.QueryPermissionChangesByPrincipal(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":  logs,
		"total": len(logs),
	})
}

// QueryPermissionChangesByPerformer 查询指定操作人的权限变更历史
// @Summary Query permission changes by performer
// @Tags IAM-Audit
// @Produce json
// @Security BearerAuth
// @Param performer_id query int true "Performer ID"
// @Param start_time query string false "Start time (RFC3339)"
// @Param end_time query string false "End time (RFC3339)"
// @Param limit query int false "Result limit"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/v1/iam/audit/permission-changes-by-performer [get]
func (h *AuditHandler) QueryPermissionChangesByPerformer(c *gin.Context) {
	// 跨 scope 操作人查询暂无 org 过滤：仅平台超管
	if !requireSystemAdmin(c) {
		return
	}
	performerIDStr := c.Query("performer_id")

	if performerIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "performer_id is required"})
		return
	}

	performerID, err := strconv.ParseUint(performerIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid performer_id"})
		return
	}

	req := &service.QueryPermissionChangesByPerformerRequest{
		PerformerID: fmt.Sprintf("%d", performerID),
	}

	// 解析时间参数并转换为本地时区（因为数据库存储的是本地时间）
	loc, _ := time.LoadLocation("Asia/Singapore")
	if startTimeStr := c.Query("start_time"); startTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, startTimeStr); err == nil {
			// 将UTC时间转换为本地时区
			req.StartTime = t.In(loc)
		}
	}
	if endTimeStr := c.Query("end_time"); endTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, endTimeStr); err == nil {
			// 将UTC时间转换为本地时区
			req.EndTime = t.In(loc)
		}
	}
	if limitStr := c.Query("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			req.Limit = limit
		}
	}

	logs, err := h.service.QueryPermissionChangesByPerformer(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":  logs,
		"total": len(logs),
	})
}
