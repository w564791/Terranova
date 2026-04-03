package controllers

import (
	"net/http"
	"strconv"

	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// VariableSetController 变量集控制器
type VariableSetController struct {
	service *services.VariableSetService
}

// NewVariableSetController 创建变量集控制器实例
func NewVariableSetController(db *gorm.DB) *VariableSetController {
	return &VariableSetController{
		service: services.NewVariableSetService(db),
	}
}

// Create 创建变量集
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
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, varset)
}

// List 获取变量集列表
func (c *VariableSetController) List(ctx *gin.Context) {
	scope := ctx.Query("scope")

	varsets, err := c.service.List(scope)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 为每个变量集添加 variable_count 和 assignment_count
	type varsetWithCounts struct {
		ID              uint   `json:"id"`
		VarsetID        string `json:"varset_id"`
		Name            string `json:"name"`
		Description     string `json:"description"`
		Scope           string `json:"scope"`
		IsDeleted       bool   `json:"is_deleted"`
		CreatedAt       interface{} `json:"created_at"`
		UpdatedAt       interface{} `json:"updated_at"`
		CreatedBy       *string `json:"created_by"`
		VariableCount   int64  `json:"variable_count"`
		AssignmentCount int64  `json:"assignment_count"`
	}

	items := make([]varsetWithCounts, 0, len(varsets))
	for _, vs := range varsets {
		varCount, _ := c.service.GetVariableCount(vs.VarsetID)
		assignCount, _ := c.service.GetAssignmentCount(vs.VarsetID)
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
			VariableCount:   varCount,
			AssignmentCount: assignCount,
		})
	}

	ctx.JSON(http.StatusOK, gin.H{"items": items})
}

// Get 获取单个变量集
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
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, varset)
}

// UpdateScope 更新变量集 scope
func (c *VariableSetController) UpdateScope(ctx *gin.Context) {
	varsetID := ctx.Param("varset_id")

	var req struct {
		Scope string `json:"scope" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	varset, err := c.service.UpdateScope(varsetID, req.Scope)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, varset)
}

// Delete 删除变量集
func (c *VariableSetController) Delete(ctx *gin.Context) {
	varsetID := ctx.Param("varset_id")

	if err := c.service.Delete(varsetID); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// ListAssignments 获取变量集分配列表
func (c *VariableSetController) ListAssignments(ctx *gin.Context) {
	varsetID := ctx.Param("varset_id")

	assignments, err := c.service.ListAssignments(varsetID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"items": assignments})
}

// CreateAssignment 创建变量集分配
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

	assignment, err := c.service.CreateAssignment(varsetID, req.ScopeType, req.ProjectID, req.WorkspaceID, &userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, assignment)
}

// DeleteAssignment 删除变量集分配
func (c *VariableSetController) DeleteAssignment(ctx *gin.Context) {
	assignmentIDStr := ctx.Param("assignment_id")
	assignmentID, err := strconv.ParseUint(assignmentIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid assignment_id"})
		return
	}

	if err := c.service.DeleteAssignment(uint(assignmentID)); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
