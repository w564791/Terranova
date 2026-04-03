package controllers

import (
	"net/http"

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
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, v.ToResponse())
}

// List 获取变量集的变量列表
func (c *VarsetVariableController) List(ctx *gin.Context) {
	varsetID := ctx.Param("varset_id")
	varType := ctx.Query("type")

	variables, err := c.service.List(varsetID, varType)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	responses := make([]map[string]interface{}, 0, len(variables))
	for _, v := range variables {
		responses = append(responses, v.ToResponse())
	}

	ctx.JSON(http.StatusOK, responses)
}

// Get 获取单个变量集变量
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
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, v.ToResponse())
}

// Delete 删除变量集变量
func (c *VarsetVariableController) Delete(ctx *gin.Context) {
	varsetID := ctx.Param("varset_id")
	varID := ctx.Param("var_id")

	if err := c.service.Delete(varsetID, varID); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
