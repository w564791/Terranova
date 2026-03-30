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

// CreateOrganization creates an organization
// @Summary Create organization
// @Description Create a new organization
// @Tags IAM-Organization
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateOrganizationRequest true "Create organization request"
// @Success 200 {object} entity.Organization
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/organizations [post]
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

// GetOrganization gets organization details
// @Summary Get organization details
// @Description Get detailed information for a specific organization
// @Tags IAM-Organization
// @Produce json
// @Security BearerAuth
// @Param id path int true "Organization ID"
// @Success 200 {object} entity.Organization
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/iam/organizations/{id} [get]
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

// ListOrganizations lists all organizations
// @Summary List organizations
// @Description List all organizations with optional active status filter
// @Tags IAM-Organization
// @Produce json
// @Security BearerAuth
// @Param is_active query boolean false "Filter by active status"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/organizations [get]
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

// UpdateOrganization updates an organization
// @Summary Update organization
// @Description Update organization information
// @Tags IAM-Organization
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Organization ID"
// @Param request body UpdateOrganizationRequest true "Update organization request"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/organizations/{id} [put]
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

// DeleteOrganization deletes an organization
// @Summary Delete organization
// @Description Delete an organization
// @Tags IAM-Organization
// @Produce json
// @Security BearerAuth
// @Param id path int true "Organization ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/organizations/{id} [delete]
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

// CreateProject creates a project
// @Summary Create project
// @Description Create a new project within an organization
// @Tags IAM-Project
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateProjectRequest true "Create project request"
// @Success 200 {object} entity.Project
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/projects [post]
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

// GetProject gets project details
// @Summary Get project details
// @Description Get detailed information for a specific project
// @Tags IAM-Project
// @Produce json
// @Security BearerAuth
// @Param id path int true "Project ID"
// @Success 200 {object} entity.Project
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/iam/projects/{id} [get]
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

// ListProjects lists all projects for an organization
// @Summary List projects
// @Description List all projects belonging to a specific organization
// @Tags IAM-Project
// @Produce json
// @Security BearerAuth
// @Param org_id query int true "Organization ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/projects [get]
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

// UpdateProject updates a project
// @Summary Update project
// @Description Update project information
// @Tags IAM-Project
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Project ID"
// @Param request body UpdateProjectRequest true "Update project request"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/projects/{id} [put]
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

// DeleteProject deletes a project
// @Summary Delete project
// @Description Delete a project
// @Tags IAM-Project
// @Produce json
// @Security BearerAuth
// @Param id path int true "Project ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/projects/{id} [delete]
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
