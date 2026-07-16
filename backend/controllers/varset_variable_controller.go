package controllers

import (
	"log"
	"net/http"
	"strings"

	"iac-platform/internal/models"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// VarsetVariableController 变量集变量控制器
type VarsetVariableController struct {
	service *services.VarsetVariableService
}

// NewVarsetVariableController 创建变量集变量控制器实例
func NewVarsetVariableController(db *gorm.DB) *VarsetVariableController {
	return &VarsetVariableController{
		service: services.NewVarsetVariableService(db),
	}
}

// Create 创建变量集变量
// @Summary Create variable set variable
// @Description Create a new variable in a variable set
// @Tags Variable Set
// @Accept json
// @Produce json
// @Param varset_id path string true "Variable set ID"
// @Param request body object true "Create request"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/variable-sets/{varset_id}/variables [post]
func (c *VarsetVariableController) Create(ctx *gin.Context) {
	varsetID := ctx.Param("varset_id")

	var req struct {
		Key          string              `json:"key" binding:"required"`
		Value        string              `json:"value"`
		Description  string              `json:"description"`
		VariableType models.VariableType `json:"variable_type" binding:"required"`
		ValueFormat  models.ValueFormat  `json:"value_format"`
		Sensitive    bool                `json:"sensitive"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 默认 value_format 为 "string"
	if req.ValueFormat == "" {
		req.ValueFormat = models.ValueFormatString
	}

	uid, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userID := uid.(string)

	v, err := c.service.Create(varsetID, req.Key, req.Value, req.Description, req.VariableType, req.ValueFormat, req.Sensitive, &userID)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "already exists") || strings.Contains(errMsg, "invalid") || strings.Contains(errMsg, "not found") {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
		} else {
			log.Printf("Failed to create varset variable: %v", err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create variable"})
		}
		return
	}

	ctx.JSON(http.StatusCreated, v.ToResponse())
}

// List 获取变量集的变量列表
// @Summary List variable set variables
// @Description Get variables in a variable set, optionally filtered by type
// @Tags Variable Set
// @Accept json
// @Produce json
// @Param varset_id path string true "Variable set ID"
// @Param type query string false "Filter by variable type"
// @Success 200 {array} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/variable-sets/{varset_id}/variables [get]
func (c *VarsetVariableController) List(ctx *gin.Context) {
	varsetID := ctx.Param("varset_id")
	varType := ctx.Query("type")

	variables, err := c.service.List(varsetID, varType)
	if err != nil {
		log.Printf("Failed to list varset variables: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list variables"})
		return
	}

	responses := make([]map[string]interface{}, 0, len(variables))
	for _, v := range variables {
		responses = append(responses, v.ToResponse())
	}

	ctx.JSON(http.StatusOK, responses)
}

// Get 获取单个变量集变量
// @Summary Get variable set variable
// @Description Get a single variable from a variable set
// @Tags Variable Set
// @Accept json
// @Produce json
// @Param varset_id path string true "Variable set ID"
// @Param var_id path string true "Variable ID"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/variable-sets/{varset_id}/variables/{var_id} [get]
func (c *VarsetVariableController) Get(ctx *gin.Context) {
	varsetID := ctx.Param("varset_id")
	varID := ctx.Param("var_id")

	v, err := c.service.GetByVariableID(varsetID, varID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "variable not found"})
		return
	}

	ctx.JSON(http.StatusOK, v.ToResponse())
}

// Update 更新变量集变量
// @Summary Update variable set variable
// @Description Update a variable in a variable set
// @Tags Variable Set
// @Accept json
// @Produce json
// @Param varset_id path string true "Variable set ID"
// @Param var_id path string true "Variable ID"
// @Param request body object true "Update request"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/variable-sets/{varset_id}/variables/{var_id} [put]
func (c *VarsetVariableController) Update(ctx *gin.Context) {
	varsetID := ctx.Param("varset_id")
	varID := ctx.Param("var_id")

	var req struct {
		Value       *string `json:"value"`
		Description *string `json:"description"`
		Sensitive   *bool   `json:"sensitive"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	v, err := c.service.Update(varsetID, varID, req.Value, req.Description, req.Sensitive)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "not found") {
			ctx.JSON(http.StatusNotFound, gin.H{"error": errMsg})
		} else if strings.Contains(errMsg, "cannot") || strings.Contains(errMsg, "invalid") {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
		} else {
			log.Printf("Failed to update varset variable: %v", err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update variable"})
		}
		return
	}

	ctx.JSON(http.StatusOK, v.ToResponse())
}

// Delete 删除变量集变量
// @Summary Delete variable set variable
// @Description Delete a variable from a variable set
// @Tags Variable Set
// @Accept json
// @Produce json
// @Param varset_id path string true "Variable set ID"
// @Param var_id path string true "Variable ID"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/variable-sets/{varset_id}/variables/{var_id} [delete]
func (c *VarsetVariableController) Delete(ctx *gin.Context) {
	varsetID := ctx.Param("varset_id")
	varID := ctx.Param("var_id")

	if err := c.service.Delete(varsetID, varID); err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "not found") {
			ctx.JSON(http.StatusNotFound, gin.H{"error": errMsg})
		} else {
			log.Printf("Failed to delete varset variable: %v", err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete variable"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
