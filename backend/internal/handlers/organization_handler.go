package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"iac-platform/internal/application/service"
)

// OrganizationHandler 组织管理Handler
type OrganizationHandler struct {
	orgService     service.OrganizationService
	projectService service.ProjectService
}

// NewOrganizationHandler 创建组织管理Handler实例
func NewOrganizationHandler(
	orgService service.OrganizationService,
	projectService service.ProjectService,
) *OrganizationHandler {
	return &OrganizationHandler{
		orgService:     orgService,
		projectService: projectService,
	}
}

// CreateOrganizationRequest 创建组织请求
type CreateOrganizationRequest struct {
	Name        string                 `json:"name" binding:"required"`
	DisplayName string                 `json:"display_name"`
	Description string                 `json:"description"`
	Settings    map[string]interface{} `json:"settings"`
}

// CreateOrganization 创建组织
// @Summary 创建组织
// @Tags Organization
// @Accept json
// @Produce json
// @Param request body CreateOrganizationRequest true "创建组织请求"
// @Success 200 {object} entity.Organization
// @Router /api/v1/organizations [post]
func (h *OrganizationHandler) CreateOrganization(c *gin.Context) {
	var req CreateOrganizationRequest
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

	// 创建组织
	createReq := &service.CreateOrganizationRequest{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		Settings:    req.Settings,
		CreatedBy:   userID.(string),
	}

	org, err := h.orgService.CreateOrganization(c.Request.Context(), createReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, org)
}

// GetOrganization 获取组织详情
// @Summary 获取组织详情
// @Tags Organization
// @Param id path int true "组织ID"
// @Success 200 {object} entity.Organization
// @Router /api/v1/organizations/{id} [get]
func (h *OrganizationHandler) GetOrganization(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	org, err := h.orgService.GetOrganization(c.Request.Context(), uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, org)
}

// ListOrganizations 列出所有组织
// @Summary 列出所有组织
// @Tags Organization
// @Param is_active query boolean false "是否启用"
// @Success 200 {array} entity.Organization
// @Router /api/v1/organizations [get]
func (h *OrganizationHandler) ListOrganizations(c *gin.Context) {
	var isActive *bool
	if isActiveStr := c.Query("is_active"); isActiveStr != "" {
		val := isActiveStr == "true"
		isActive = &val
	}

	orgs, err := h.orgService.ListOrganizations(c.Request.Context(), isActive)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"organizations": orgs,
		"total":         len(orgs),
	})
}

// UpdateOrganizationRequest 更新组织请求
type UpdateOrganizationRequest struct {
	DisplayName string                 `json:"display_name"`
	Description string                 `json:"description"`
	IsActive    bool                   `json:"is_active"`
	Settings    map[string]interface{} `json:"settings"`
}

// UpdateOrganization 更新组织
// @Summary 更新组织
// @Tags Organization
// @Param id path int true "组织ID"
// @Param request body UpdateOrganizationRequest true "更新组织请求"
// @Success 200 {object} map[string]string
// @Router /api/v1/organizations/{id} [put]
func (h *OrganizationHandler) UpdateOrganization(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req UpdateOrganizationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 更新组织
	updateReq := &service.UpdateOrganizationRequest{
		ID:          uint(id),
		DisplayName: req.DisplayName,
		Description: req.Description,
		IsActive:    req.IsActive,
		Settings:    req.Settings,
	}

	if err := h.orgService.UpdateOrganization(c.Request.Context(), updateReq); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Organization updated successfully"})
}

// DeleteOrganization 删除组织
// @Summary 删除组织
// @Tags Organization
// @Param id path int true "组织ID"
// @Success 200 {object} map[string]string
// @Router /api/v1/organizations/{id} [delete]
func (h *OrganizationHandler) DeleteOrganization(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.orgService.DeleteOrganization(c.Request.Context(), uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Organization deleted successfully"})
}

// CreateProjectRequest 创建项目请求
type CreateProjectRequest struct {
	OrgID       uint                   `json:"org_id" binding:"required"`
	Name        string                 `json:"name" binding:"required"`
	DisplayName string                 `json:"display_name"`
	Description string                 `json:"description"`
	Settings    map[string]interface{} `json:"settings"`
}

// CreateProject 创建项目
// @Summary 创建项目
// @Tags Project
// @Accept json
// @Produce json
// @Param request body CreateProjectRequest true "创建项目请求"
// @Success 200 {object} entity.Project
// @Router /api/v1/projects [post]
func (h *OrganizationHandler) CreateProject(c *gin.Context) {
	var req CreateProjectRequest
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

	// 创建项目
	createReq := &service.CreateProjectRequest{
		OrgID:       req.OrgID,
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		Settings:    req.Settings,
		CreatedBy:   userID.(string),
	}

	project, err := h.projectService.CreateProject(c.Request.Context(), createReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, project)
}

// GetProject 获取项目详情
// @Summary 获取项目详情
// @Tags Project
// @Param id path int true "项目ID"
// @Success 200 {object} entity.Project
// @Router /api/v1/projects/{id} [get]
func (h *OrganizationHandler) GetProject(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	project, err := h.projectService.GetProject(c.Request.Context(), uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, project)
}

// ListProjects 列出组织的所有项目
// @Summary 列出组织的所有项目
// @Tags Project
// @Param org_id query int true "组织ID"
// @Success 200 {array} entity.Project
// @Router /api/v1/projects [get]
func (h *OrganizationHandler) ListProjects(c *gin.Context) {
	orgIDStr := c.Query("org_id")
	if orgIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "org_id is required"})
		return
	}

	orgID, err := strconv.ParseUint(orgIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid org_id"})
		return
	}

	projects, err := h.projectService.ListProjectsByOrg(c.Request.Context(), uint(orgID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"projects": projects,
		"total":    len(projects),
	})
}

// UpdateProjectRequest 更新项目请求
type UpdateProjectRequest struct {
	DisplayName string                 `json:"display_name"`
	Description string                 `json:"description"`
	IsActive    bool                   `json:"is_active"`
	Settings    map[string]interface{} `json:"settings"`
}

// UpdateProject 更新项目
// @Summary 更新项目
// @Tags Project
// @Param id path int true "项目ID"
// @Param request body UpdateProjectRequest true "更新项目请求"
// @Success 200 {object} map[string]string
// @Router /api/v1/projects/{id} [put]
func (h *OrganizationHandler) UpdateProject(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req UpdateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 更新项目
	updateReq := &service.UpdateProjectRequest{
		ID:          uint(id),
		DisplayName: req.DisplayName,
		Description: req.Description,
		IsActive:    req.IsActive,
		Settings:    req.Settings,
	}

	if err := h.projectService.UpdateProject(c.Request.Context(), updateReq); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Project updated successfully"})
}

// DeleteProject 删除项目
// @Summary 删除项目
// @Tags Project
// @Param id path int true "项目ID"
// @Success 200 {object} map[string]string
// @Router /api/v1/projects/{id} [delete]
func (h *OrganizationHandler) DeleteProject(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.projectService.DeleteProject(c.Request.Context(), uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Project deleted successfully"})
}
