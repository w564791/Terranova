package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/repository"
	"iac-platform/internal/infrastructure/persistence"
)

// WorkspaceProjectHandler 工作空间-项目关联Handler
type WorkspaceProjectHandler struct {
	projectRepo repository.ProjectRepository
	db          *gorm.DB
}

// NewWorkspaceProjectHandler 创建工作空间-项目关联Handler实例
func NewWorkspaceProjectHandler(db *gorm.DB) *WorkspaceProjectHandler {
	return &WorkspaceProjectHandler{
		projectRepo: persistence.NewProjectRepository(db),
		db:          db,
	}
}

// GetWorkspaceProject 获取工作空间所属的项目
// @Summary Get workspace project
// @Description Get the project that a workspace belongs to
// @Tags Workspace Project
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workspaces/{id}/project [get]
// @Security BearerAuth
func (h *WorkspaceProjectHandler) GetWorkspaceProject(c *gin.Context) {
	workspaceID := c.Param("id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return
	}

	authOrg, ok := requireAuthOrg(c)
	if !ok {
		return
	}

	project, err := h.projectRepo.GetProjectByWorkspaceID(c.Request.Context(), workspaceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 如果没有关联项目，返回鉴权 org 的 default project（不硬编码 org=1）
	if project == nil {
		defaultProject, err := h.projectRepo.GetDefaultProject(c.Request.Context(), authOrg)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"project": nil,
				"message": "workspace not assigned to any project",
			})
			return
		}
		project = defaultProject
	} else if project.OrgID != authOrg {
		c.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"project": project,
	})
}

// SetWorkspaceProjectRequest 设置工作空间项目请求
type SetWorkspaceProjectRequest struct {
	ProjectID uint `json:"project_id" binding:"required"`
}

// SetWorkspaceProject 设置工作空间所属的项目
// @Summary Set workspace project
// @Description Assign a workspace to a project
// @Tags Workspace Project
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param request body SetWorkspaceProjectRequest true "Set project request"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workspaces/{id}/project [put]
// @Security BearerAuth
func (h *WorkspaceProjectHandler) SetWorkspaceProject(c *gin.Context) {
	workspaceID := c.Param("id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return
	}

	authOrg, ok := requireAuthOrg(c)
	if !ok {
		return
	}

	var req SetWorkspaceProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证项目存在且属于鉴权 org
	project, err := h.projectRepo.GetProjectByID(c.Request.Context(), req.ProjectID)
	if err != nil || project == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}
	if err := ensureProjectBelongsToAuthOrg(project.OrgID, authOrg); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	// Workspace 若已绑定，必须已在同一 org（中间件已拦跨 org；此处防竞态改绑出租户）
	if err := ensureWorkspaceSemanticInAuthOrg(c.Request.Context(), h.db, workspaceID, authOrg); err != nil {
		// 允许「尚未绑定」的 workspace 首次绑定到本 org（仅当无任何 project 关系）
		var cnt int64
		_ = h.db.WithContext(c.Request.Context()).
			Table("workspace_project_relations").
			Where("workspace_id = ?", workspaceID).
			Count(&cnt)
		if cnt > 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
			return
		}
		// 无绑定：须确认 workspace 实体存在
		var exists int64
		_ = h.db.WithContext(c.Request.Context()).
			Table("workspaces").
			Where("workspace_id = ? OR id::text = ?", workspaceID, workspaceID).
			Count(&exists)
		if exists == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
			return
		}
	}

	// 事务内重绑 + 一对一
	err = h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("workspace_id = ?", workspaceID).
			Delete(&entity.WorkspaceProjectRelation{}).Error; err != nil {
			return err
		}
		rel := &entity.WorkspaceProjectRelation{
			WorkspaceID: workspaceID,
			ProjectID:   req.ProjectID,
			CreatedAt:   time.Now(),
		}
		return tx.Create(rel).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "workspace assigned to project successfully",
		"project": project,
	})
}

// RemoveWorkspaceFromProject 从项目中移除工作空间
// @Summary Remove workspace from project
// @Description Remove a workspace from its current project
// @Tags Workspace Project
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workspaces/{id}/project [delete]
// @Security BearerAuth
func (h *WorkspaceProjectHandler) RemoveWorkspaceFromProject(c *gin.Context) {
	workspaceID := c.Param("id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return
	}

	if err := h.projectRepo.RemoveWorkspaceFromProject(c.Request.Context(), workspaceID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "workspace removed from project successfully",
	})
}

// ListProjectWorkspaces 列出项目下的所有工作空间
// @Summary List project workspaces
// @Description List all workspaces belonging to a project
// @Tags Project
// @Produce json
// @Param id path int true "Project ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/projects/{id}/workspaces [get]
// @Security BearerAuth
func (h *WorkspaceProjectHandler) ListProjectWorkspaces(c *gin.Context) {
	projectIDStr := c.Param("id")
	projectID, err := strconv.ParseUint(projectIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project_id"})
		return
	}

	// The route's IAM middleware authorizes a permission in auth_org_id; bind the
	// path resource to that same organization before reading its relationships.
	// Otherwise a caller with Org A permission can enumerate workspaces of a
	// guessed project ID in Org B.
	authOrg, ok := requireAuthOrg(c)
	if !ok {
		return
	}
	project, err := h.projectRepo.GetProjectByID(c.Request.Context(), uint(projectID))
	if err != nil || project == nil || ensureProjectBelongsToAuthOrg(project.OrgID, authOrg) != nil {
		// Return the same result for a foreign and a missing project to avoid
		// turning this endpoint into a project-ID oracle.
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	// 获取项目下的工作空间ID列表
	workspaceIDs, err := h.projectRepo.ListWorkspacesByProject(c.Request.Context(), uint(projectID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 查询工作空间详情
	var workspaces []map[string]interface{}
	if len(workspaceIDs) > 0 {
		rows, err := h.db.Table("workspaces").
			Select("workspace_id, name, description, execution_mode, state, created_at, updated_at").
			Where("workspace_id IN ?", workspaceIDs).
			Rows()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer rows.Close()

		for rows.Next() {
			var ws struct {
				WorkspaceID   string `gorm:"column:workspace_id"`
				Name          string
				Description   string
				ExecutionMode string `gorm:"column:execution_mode"`
				State         string
				CreatedAt     string `gorm:"column:created_at"`
				UpdatedAt     string `gorm:"column:updated_at"`
			}
			if err := h.db.ScanRows(rows, &ws); err != nil {
				continue
			}
			workspaces = append(workspaces, map[string]interface{}{
				"workspace_id":   ws.WorkspaceID,
				"name":           ws.Name,
				"description":    ws.Description,
				"execution_mode": ws.ExecutionMode,
				"state":          ws.State,
				"created_at":     ws.CreatedAt,
				"updated_at":     ws.UpdatedAt,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"workspaces": workspaces,
		"total":      len(workspaces),
	})
}

// ListProjectsWithWorkspaceCount 列出所有项目及其工作空间数量
// @Summary List projects with workspace count
// @Description List all projects with their workspace counts
// @Tags Project
// @Produce json
// @Param org_id query int false "Organization ID (must match authenticated organization)"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/projects [get]
// @Security BearerAuth
func (h *WorkspaceProjectHandler) ListProjectsWithWorkspaceCount(c *gin.Context) {
	authOrg, ok := requireAuthOrg(c)
	if !ok {
		return
	}
	if orgIDStr := c.Query("org_id"); orgIDStr != "" {
		orgID, err := strconv.ParseUint(orgIDStr, 10, 32)
		if err != nil || orgID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid org_id"})
			return
		}
		if uint(orgID) != authOrg {
			// Do not let a caller turn this list endpoint into an organization
			// existence oracle after its IAM grant has been evaluated.
			c.JSON(http.StatusNotFound, gin.H{"error": "organization not found"})
			return
		}
	}

	// 获取组织的所有项目
	projects, err := h.projectRepo.ListProjectsByOrg(c.Request.Context(), authOrg, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 获取每个项目的工作空间数量
	type ProjectWithCount struct {
		*entity.Project
		WorkspaceCount int `json:"workspace_count"`
	}

	var result []ProjectWithCount
	for _, p := range projects {
		workspaceIDs, _ := h.projectRepo.ListWorkspacesByProject(c.Request.Context(), p.ID)
		result = append(result, ProjectWithCount{
			Project:        p,
			WorkspaceCount: len(workspaceIDs),
		})
	}

	// An unassigned workspace has no tenant identity.  Counting every such row
	// under this tenant's default project used to disclose the global total;
	// leave unbound legacy data out until it is explicitly assigned.

	c.JSON(http.StatusOK, gin.H{
		"projects": result,
		"total":    len(result),
	})
}
