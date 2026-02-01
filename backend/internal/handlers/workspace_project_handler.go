package handlers

import (
	"net/http"
	"strconv"

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
// @Summary 获取工作空间所属的项目
// @Tags Workspace Project
// @Param workspace_id path string true "工作空间ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workspaces/{workspace_id}/project [get]
func (h *WorkspaceProjectHandler) GetWorkspaceProject(c *gin.Context) {
	workspaceID := c.Param("id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return
	}

	project, err := h.projectRepo.GetProjectByWorkspaceID(c.Request.Context(), workspaceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 如果没有关联项目，返回 default project
	if project == nil {
		// 获取 org_id=1 的默认项目
		defaultProject, err := h.projectRepo.GetDefaultProject(c.Request.Context(), 1)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"project": nil,
				"message": "workspace not assigned to any project",
			})
			return
		}
		project = defaultProject
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
// @Summary 设置工作空间所属的项目
// @Tags Workspace Project
// @Accept json
// @Produce json
// @Param workspace_id path string true "工作空间ID"
// @Param request body SetWorkspaceProjectRequest true "设置项目请求"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workspaces/{workspace_id}/project [put]
func (h *WorkspaceProjectHandler) SetWorkspaceProject(c *gin.Context) {
	workspaceID := c.Param("id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return
	}

	var req SetWorkspaceProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证项目是否存在
	project, err := h.projectRepo.GetProjectByID(c.Request.Context(), req.ProjectID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	// 分配工作空间到项目
	if err := h.projectRepo.AssignWorkspaceToProject(c.Request.Context(), workspaceID, req.ProjectID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "workspace assigned to project successfully",
		"project": project,
	})
}

// RemoveWorkspaceFromProject 从项目中移除工作空间
// @Summary 从项目中移除工作空间
// @Tags Workspace Project
// @Param workspace_id path string true "工作空间ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workspaces/{workspace_id}/project [delete]
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
// @Summary 列出项目下的所有工作空间
// @Tags Project
// @Param id path int true "项目ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/projects/{id}/workspaces [get]
func (h *WorkspaceProjectHandler) ListProjectWorkspaces(c *gin.Context) {
	projectIDStr := c.Param("id")
	projectID, err := strconv.ParseUint(projectIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project_id"})
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
// @Summary 列出所有项目及其工作空间数量
// @Tags Project
// @Param org_id query int false "组织ID，默认为1"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/projects [get]
func (h *WorkspaceProjectHandler) ListProjectsWithWorkspaceCount(c *gin.Context) {
	orgIDStr := c.DefaultQuery("org_id", "1")
	orgID, err := strconv.ParseUint(orgIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid org_id"})
		return
	}

	// 获取组织的所有项目
	projects, err := h.projectRepo.ListProjectsByOrg(c.Request.Context(), uint(orgID), nil)
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

	// 计算未分配项目的工作空间数量（归入 default）
	var unassignedCount int64
	h.db.Table("workspaces").
		Where("workspace_id NOT IN (SELECT workspace_id FROM workspace_project_relations)").
		Count(&unassignedCount)

	// 将未分配的数量加到 default 项目
	for i, p := range result {
		if p.Project.IsDefault {
			result[i].WorkspaceCount += int(unassignedCount)
			break
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"projects": result,
		"total":    len(result),
	})
}
