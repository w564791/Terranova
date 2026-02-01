package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"iac-platform/internal/application/service"
	"iac-platform/internal/domain/valueobject"

	"github.com/gin-gonic/gin"
)

// AuditHandler 审计处理器
type AuditHandler struct {
	service *service.AuditService
}

// NewAuditHandler 创建审计处理器实例
func NewAuditHandler(service *service.AuditService) *AuditHandler {
	return &AuditHandler{
		service: service,
	}
}

// QueryPermissionHistory 查询权限变更历史
// @Summary 查询权限变更历史
// @Tags IAM-Audit
// @Produce json
// @Param scope_type query string true "作用域类型"
// @Param scope_id query int true "作用域ID"
// @Param start_time query string false "开始时间"
// @Param end_time query string false "结束时间"
// @Param limit query int false "限制数量"
// @Success 200 {object} map[string]interface{}
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

	req := &service.QueryPermissionHistoryRequest{
		ScopeType: valueobject.ScopeType(scopeTypeStr),
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
// @Summary 查询资源访问历史
// @Tags IAM-Audit
// @Produce json
// @Param user_id query int false "用户ID"
// @Param resource_type query string false "资源类型"
// @Param method query string false "请求方法"
// @Param http_code_operator query string false "HTTP状态码运算符"
// @Param http_code_value query int false "HTTP状态码值"
// @Param start_time query string false "开始时间"
// @Param end_time query string false "结束时间"
// @Param limit query int false "限制数量"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/iam/audit/access-history [get]
func (h *AuditHandler) QueryAccessHistory(c *gin.Context) {
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
// @Summary 查询被拒绝的访问记录
// @Tags IAM-Audit
// @Produce json
// @Param start_time query string false "开始时间"
// @Param end_time query string false "结束时间"
// @Param limit query int false "限制数量"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/iam/audit/denied-access [get]
func (h *AuditHandler) QueryDeniedAccess(c *gin.Context) {
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
// @Summary 查询指定主体的权限变更历史
// @Tags IAM-Audit
// @Produce json
// @Param principal_type query string true "主体类型"
// @Param principal_id query int true "主体ID"
// @Param start_time query string false "开始时间"
// @Param end_time query string false "结束时间"
// @Param limit query int false "限制数量"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/iam/audit/permission-changes-by-principal [get]
func (h *AuditHandler) QueryPermissionChangesByPrincipal(c *gin.Context) {
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
		PrincipalID:   string(principalID),
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
// @Summary 查询指定操作人的权限变更历史
// @Tags IAM-Audit
// @Produce json
// @Param performer_id query int true "操作人ID"
// @Param start_time query string false "开始时间"
// @Param end_time query string false "结束时间"
// @Param limit query int false "限制数量"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/iam/audit/permission-changes-by-performer [get]
func (h *AuditHandler) QueryPermissionChangesByPerformer(c *gin.Context) {
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
