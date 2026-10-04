package controllers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"iac-platform/internal/middleware"
	"iac-platform/internal/models"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// VariableSetController 变量集控制器
type VariableSetController struct {
	service *services.VariableSetService

	// AuthorizeAssignmentTarget validates the target workspace/project of an
	// assignment being created or removed (in the auth org, caller may write its
	// variables). It writes the 403/404 response and returns false on deny. Nil
	// fails closed (403).
	AuthorizeAssignmentTarget func(ctx *gin.Context, scopeType string, projectID *int, workspaceID *string) bool
}

// VarsetInAuthOrg binds /variable-sets/:varset_id routes to the caller's org.
// Placed after RequirePermission. A varset not visible in the auth org
// (VarsetVisibleInOrg) is 404 for every route; on write routes a visible but
// not writable varset (VarsetWritableInOrg: assigned to another org, someone
// else's unassigned set, or a global set written by a non-superadmin) is 403.
func (c *VariableSetController) VarsetInAuthOrg(write bool) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		varsetID := ctx.Param("varset_id")
		orgID, ok := middleware.AuthOrgID(ctx)
		if !ok {
			ctx.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "variable set not found"})
			return
		}
		userID := ctx.GetString("user_id")
		visible, err := c.service.VarsetVisibleInOrg(varsetID, orgID, userID)
		if err != nil {
			_ = ctx.Error(fmt.Errorf("varset org binding: %w", err))
			ctx.Abort()
			return
		}
		if !visible {
			ctx.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "variable set not found"})
			return
		}
		if write {
			writable, err := c.service.VarsetWritableInOrg(varsetID, orgID, userID, isSystemAdmin(ctx))
			if err != nil {
				_ = ctx.Error(fmt.Errorf("varset org binding: %w", err))
				ctx.Abort()
				return
			}
			if !writable {
				ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "variable set is read-only in this organization"})
				return
			}
		}
		ctx.Next()
	}
}

func isSystemAdmin(ctx *gin.Context) bool {
	v, _ := ctx.Get("is_system_admin")
	b, _ := v.(bool)
	return b
}

func (c *VariableSetController) authorizeTarget(ctx *gin.Context, scopeType string, projectID *int, workspaceID *string) bool {
	if c.AuthorizeAssignmentTarget == nil {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "assignment target authorization not configured"})
		return false
	}
	return c.AuthorizeAssignmentTarget(ctx, scopeType, projectID, workspaceID)
}

// NewVariableSetController 创建变量集控制器实例
func NewVariableSetController(db *gorm.DB) *VariableSetController {
	return &VariableSetController{
		service: services.NewVariableSetService(db),
	}
}

// Create 创建变量集
// @Summary Create variable set
// @Description Create a new variable set
// @Tags Variable Set
// @Accept json
// @Produce json
// @Param request body object true "Create request"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/variable-sets [post]
func (c *VariableSetController) Create(ctx *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
		Scope       string `json:"scope" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	uid, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userID := uid.(string)

	varset, err := c.service.Create(req.Name, req.Description, req.Scope, &userID)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "already exists") || strings.Contains(errMsg, "invalid") {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
		} else {
			_ = ctx.Error(fmt.Errorf("Failed to create variable set: %w", err))
		}
		return
	}

	ctx.JSON(http.StatusCreated, varset)
}

// List 获取变量集列表
// @Summary List variable sets
// @Description Get list of variable sets, optionally filtered by scope
// @Tags Variable Set
// @Accept json
// @Produce json
// @Param scope query string false "Filter by scope"
// @Param workspace_id query string false "Only variable sets this workspace can mount (global + assigned to the workspace or its project); caller must be able to read the workspace"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/variable-sets [get]
func (c *VariableSetController) List(ctx *gin.Context) {
	scope := ctx.Query("scope")

	// workspace_id: 仅返回该 workspace 可挂载的变量集(路由层已校验 workspace 可读且属于鉴权 org)
	var varsets []models.VariableSet
	var err error
	if workspaceID := ctx.Query("workspace_id"); workspaceID != "" {
		varsets, err = c.service.ListMountableForWorkspace(workspaceID)
	} else {
		// 无 workspace_id: 只返回调用组织可见的变量集(VariableSet 无 org_id,见 ListForOrg)
		orgID, ok := middleware.AuthOrgID(ctx)
		if !ok {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "org_id is required"})
			return
		}
		varsets, err = c.service.ListForOrg(scope, orgID, ctx.GetString("user_id"))
	}
	if err != nil {
		_ = ctx.Error(fmt.Errorf("Failed to list variable sets: %w", err))
		return
	}

	// 为每个变量集添加 variable_count 和 assignment_count
	type varsetWithCounts struct {
		ID              uint        `json:"id"`
		VarsetID        string      `json:"varset_id"`
		Name            string      `json:"name"`
		Description     string      `json:"description"`
		Scope           string      `json:"scope"`
		IsDeleted       bool        `json:"is_deleted"`
		CreatedAt       interface{} `json:"created_at"`
		UpdatedAt       interface{} `json:"updated_at"`
		CreatedBy       *string     `json:"created_by"`
		VariableCount   int64       `json:"variable_count"`
		AssignmentCount int64       `json:"assignment_count"`
	}

	// Collect varset IDs
	varsetIDs := make([]string, len(varsets))
	for i, s := range varsets {
		varsetIDs[i] = s.VarsetID
	}

	// Batch count queries (2 queries total instead of 2N)
	varCounts, err := c.service.GetVariableCounts(varsetIDs)
	if err != nil {
		log.Printf("Failed to get variable counts: %v", err)
		varCounts = make(map[string]int64)
	}
	assignCounts, err := c.service.GetAssignmentCounts(varsetIDs)
	if err != nil {
		log.Printf("Failed to get assignment counts: %v", err)
		assignCounts = make(map[string]int64)
	}

	items := make([]varsetWithCounts, 0, len(varsets))
	for _, vs := range varsets {
		items = append(items, varsetWithCounts{
			ID:              vs.ID,
			VarsetID:        vs.VarsetID,
			Name:            vs.Name,
			Description:     vs.Description,
			Scope:           vs.Scope,
			IsDeleted:       vs.IsDeleted,
			CreatedAt:       vs.CreatedAt,
			UpdatedAt:       vs.UpdatedAt,
			CreatedBy:       vs.CreatedBy,
			VariableCount:   varCounts[vs.VarsetID],
			AssignmentCount: assignCounts[vs.VarsetID],
		})
	}

	ctx.JSON(http.StatusOK, gin.H{"items": items})
}

// Get 获取单个变量集
// @Summary Get variable set
// @Description Get a variable set by ID
// @Tags Variable Set
// @Accept json
// @Produce json
// @Param varset_id path string true "Variable set ID"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/variable-sets/{varset_id} [get]
func (c *VariableSetController) Get(ctx *gin.Context) {
	varsetID := ctx.Param("varset_id")

	varset, err := c.service.GetByVarsetID(varsetID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "variable set not found"})
		return
	}

	ctx.JSON(http.StatusOK, varset)
}

// Update 更新变量集
// @Summary Update variable set
// @Description Update a variable set's name and description
// @Tags Variable Set
// @Accept json
// @Produce json
// @Param varset_id path string true "Variable set ID"
// @Param request body object true "Update request"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/variable-sets/{varset_id} [put]
func (c *VariableSetController) Update(ctx *gin.Context) {
	varsetID := ctx.Param("varset_id")

	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	varset, err := c.service.Update(varsetID, req.Name, req.Description)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "not found") {
			ctx.JSON(http.StatusNotFound, gin.H{"error": errMsg})
		} else if strings.Contains(errMsg, "already exists") {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
		} else {
			_ = ctx.Error(fmt.Errorf("Failed to update variable set: %w", err))
		}
		return
	}

	ctx.JSON(http.StatusOK, varset)
}

// UpdateScope 更新变量集 scope
// @Summary Update variable set scope
// @Description Update a variable set's scope
// @Tags Variable Set
// @Accept json
// @Produce json
// @Param varset_id path string true "Variable set ID"
// @Param request body object true "Update scope request"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/variable-sets/{varset_id}/scope [put]
func (c *VariableSetController) UpdateScope(ctx *gin.Context) {
	varsetID := ctx.Param("varset_id")

	var req struct {
		Scope string `json:"scope" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// global 变量集注入所有组织的 workspace:只有平台超管可以把变量集设为 global
	if req.Scope == "global" && !isSystemAdmin(ctx) {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "only a system admin can make a variable set global"})
		return
	}

	varset, err := c.service.UpdateScope(varsetID, req.Scope)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "not found") {
			ctx.JSON(http.StatusNotFound, gin.H{"error": errMsg})
		} else if strings.Contains(errMsg, "invalid") {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
		} else {
			_ = ctx.Error(fmt.Errorf("Failed to update variable set scope: %w", err))
		}
		return
	}

	ctx.JSON(http.StatusOK, varset)
}

// Delete 删除变量集
// @Summary Delete variable set
// @Description Delete a variable set by ID
// @Tags Variable Set
// @Accept json
// @Produce json
// @Param varset_id path string true "Variable set ID"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/variable-sets/{varset_id} [delete]
func (c *VariableSetController) Delete(ctx *gin.Context) {
	varsetID := ctx.Param("varset_id")

	if err := c.service.Delete(varsetID); err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "not found") {
			ctx.JSON(http.StatusNotFound, gin.H{"error": errMsg})
		} else {
			_ = ctx.Error(fmt.Errorf("Failed to delete variable set: %w", err))
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// ListAssignments 获取变量集分配列表
// @Summary List variable set assignments
// @Description Get assignment list for a variable set
// @Tags Variable Set
// @Accept json
// @Produce json
// @Param varset_id path string true "Variable set ID"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/variable-sets/{varset_id}/assignments [get]
func (c *VariableSetController) ListAssignments(ctx *gin.Context) {
	varsetID := ctx.Param("varset_id")

	assignments, err := c.service.ListAssignments(varsetID)
	if err != nil {
		_ = ctx.Error(fmt.Errorf("Failed to list assignments: %w", err))
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"items": assignments})
}

// CreateAssignment 创建变量集分配
// @Summary Create variable set assignment
// @Description Assign a variable set to a project or workspace
// @Tags Variable Set
// @Accept json
// @Produce json
// @Param varset_id path string true "Variable set ID"
// @Param request body object true "Create assignment request"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/variable-sets/{varset_id}/assignments [post]
func (c *VariableSetController) CreateAssignment(ctx *gin.Context) {
	varsetID := ctx.Param("varset_id")

	var req struct {
		ScopeType   string  `json:"scope_type" binding:"required"`
		ProjectID   *int    `json:"project_id"`
		WorkspaceID *string `json:"workspace_id"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	uid, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userID := uid.(string)

	if !c.authorizeTarget(ctx, req.ScopeType, req.ProjectID, req.WorkspaceID) {
		return
	}

	assignment, err := c.service.CreateAssignment(varsetID, req.ScopeType, req.ProjectID, req.WorkspaceID, &userID)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "not found") {
			ctx.JSON(http.StatusNotFound, gin.H{"error": errMsg})
		} else if strings.Contains(errMsg, "cannot") || strings.Contains(errMsg, "invalid") || strings.Contains(errMsg, "required") {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
		} else {
			_ = ctx.Error(fmt.Errorf("Failed to create assignment: %w", err))
		}
		return
	}

	ctx.JSON(http.StatusCreated, assignment)
}

// DeleteAssignment 删除变量集分配
// @Summary Delete variable set assignment
// @Description Remove an assignment from a variable set
// @Tags Variable Set
// @Accept json
// @Produce json
// @Param varset_id path string true "Variable set ID"
// @Param assignment_id path string true "Assignment ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/variable-sets/{varset_id}/assignments/{assignment_id} [delete]
func (c *VariableSetController) DeleteAssignment(ctx *gin.Context) {
	assignmentIDStr := ctx.Param("assignment_id")
	assignmentID, err := strconv.ParseUint(assignmentIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid assignment_id"})
		return
	}

	varsetID := ctx.Param("varset_id")
	// 解除分配同样改变目标的变量:按分配记录上的目标做与创建相同的校验
	existing, err := c.service.GetAssignment(varsetID, uint(assignmentID))
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "assignment not found"})
		return
	}
	if !c.authorizeTarget(ctx, existing.ScopeType, existing.ProjectID, existing.WorkspaceID) {
		return
	}
	if err := c.service.DeleteAssignment(varsetID, uint(assignmentID)); err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "not found") {
			ctx.JSON(http.StatusNotFound, gin.H{"error": errMsg})
		} else {
			_ = ctx.Error(fmt.Errorf("Failed to delete assignment: %w", err))
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
