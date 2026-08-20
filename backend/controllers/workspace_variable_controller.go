package controllers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"iac-platform/internal/models"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
)

// WorkspaceVariableController Workspace变量控制器
type WorkspaceVariableController struct {
	variableService *services.WorkspaceVariableService
}

// NewWorkspaceVariableController 创建变量控制器实例
func NewWorkspaceVariableController(variableService *services.WorkspaceVariableService) *WorkspaceVariableController {
	return &WorkspaceVariableController{
		variableService: variableService,
	}
}

// resolvePathWorkspace 解析路径 :id 为语义化 workspace_id（并兼容数字主键）
func (vc *WorkspaceVariableController) resolvePathWorkspace(c *gin.Context) (semanticID string, ok bool) {
	workspaceIDParam := c.Param("id")
	if workspaceIDParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400, "message": "无效的workspace ID",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return "", false
	}
	var workspace models.Workspace
	db := vc.variableService.GetDB()
	if err := db.Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
		if err := db.Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"code": 404, "message": "Workspace不存在",
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return "", false
		}
	}
	return workspace.WorkspaceID, true
}

// CreateVariable 创建变量
// @Summary 创建工作空间变量
// @Description 为指定工作空间创建新的变量（Terraform变量或环境变量）
// @Tags Workspace Variable
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param request body object true "变量信息"
// @Success 201 {object} map[string]interface{} "变量创建成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Router /api/v1/workspaces/{id}/variables [post]
// @Security BearerAuth
func (vc *WorkspaceVariableController) CreateVariable(c *gin.Context) {
	workspaceIDParam := c.Param("id")
	if workspaceIDParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "无效的workspace ID",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// 获取workspace以获取内部ID
	var workspace models.Workspace
	err := vc.variableService.GetDB().Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := vc.variableService.GetDB().Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"code":      404,
				"message":   "Workspace不存在",
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return
		}
	}

	var req struct {
		Key          string              `json:"key" binding:"required"`
		Value        string              `json:"value"`
		VariableType models.VariableType `json:"variable_type" binding:"required"`
		ValueFormat  models.ValueFormat  `json:"value_format"`
		Sensitive    bool                `json:"sensitive"`
		Description  string              `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "请求参数无效",
			"error":     err.Error(),
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// 从JWT token获取用户ID
	uid, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":      401,
			"message":   "未授权",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}
	userID := uid.(string)

	// 设置默认值
	if req.ValueFormat == "" {
		req.ValueFormat = models.ValueFormatString
	}

	variable := &models.WorkspaceVariable{
		WorkspaceID:  workspace.WorkspaceID,
		Key:          req.Key,
		Value:        req.Value,
		VariableType: req.VariableType,
		ValueFormat:  req.ValueFormat,
		Sensitive:    req.Sensitive,
		Description:  req.Description,
		CreatedBy:    &userID,
	}

	if err := vc.variableService.CreateVariable(variable); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   err.Error(),
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"code":      201,
		"data":      variable.ToResponse(),
		"message":   "变量创建成功",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// ListVariables 获取变量列表
// @Summary 获取工作空间变量列表
// @Description 获取指定工作空间的所有变量，支持按类型过滤
// @Tags Workspace Variable
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param type query string false "变量类型过滤（terraform/env/all）" default(all)
// @Success 200 {object} map[string]interface{} "成功返回变量列表"
// @Failure 400 {object} map[string]interface{} "无效的工作空间ID"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/workspaces/{id}/variables [get]
// @Security BearerAuth
func (vc *WorkspaceVariableController) ListVariables(c *gin.Context) {
	workspaceIDParam := c.Param("id")
	if workspaceIDParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "无效的workspace ID",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// 获取workspace以获取内部ID
	var workspace models.Workspace
	err := vc.variableService.GetDB().Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := vc.variableService.GetDB().Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"code":      404,
				"message":   "Workspace不存在",
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return
		}
	}

	// 获取类型过滤参数
	variableType := c.DefaultQuery("type", "all")

	variables, err := vc.variableService.ListVariables(workspace.WorkspaceID, variableType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "获取变量列表失败",
			"error":     err.Error(),
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// 转换为响应格式（处理敏感变量）
	responses := make([]*models.WorkspaceVariableResponse, len(variables))
	for i, v := range variables {
		responses[i] = v.ToResponse()
	}

	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"data":      responses,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// GetVariable 获取单个变量
// @Summary 获取变量详情
// @Description 根据ID获取变量的详细信息
// @Tags Workspace Variable
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param var_id path int true "变量ID"
// @Success 200 {object} map[string]interface{} "成功返回变量详情"
// @Failure 400 {object} map[string]interface{} "无效的变量ID"
// @Failure 404 {object} map[string]interface{} "变量不存在"
// @Router /api/v1/workspaces/{id}/variables/{var_id} [get]
// @Security BearerAuth
func (vc *WorkspaceVariableController) GetVariable(c *gin.Context) {
	workspaceIDParam := c.Param("id")
	if workspaceIDParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "无效的workspace ID",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	varID, err := strconv.ParseUint(c.Param("var_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "无效的变量ID",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// 解析路径 workspace，确保子资源归属该 workspace（防跨 WS IDOR）
	var workspace models.Workspace
	db := vc.variableService.GetDB()
	if err := db.Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
		if err := db.Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"code":      404,
				"message":   "Workspace不存在",
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return
		}
	}

	variable, err := vc.variableService.GetVariableInWorkspace(workspace.WorkspaceID, uint(varID))
	if err != nil {
		// 也尝试数字 workspace 主键（历史数据兼容）
		variable, err = vc.variableService.GetVariableInWorkspace(fmt.Sprintf("%d", workspace.ID), uint(varID))
	}
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":      404,
			"message":   "变量不存在",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"data":      variable.ToResponse(),
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// UpdateVariable 更新变量
// @Summary 更新变量
// @Description 更新变量的配置信息（带版本控制和乐观锁）
// @Tags Workspace Variable
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param var_id path string true "变量ID（支持数字ID或variable_id）"
// @Param request body object true "更新信息（必须包含version字段）"
// @Success 200 {object} map[string]interface{} "更新成功，返回版本变更信息"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 409 {object} map[string]interface{} "版本冲突"
// @Router /api/v1/workspaces/{id}/variables/{var_id} [put]
// @Security BearerAuth
func (vc *WorkspaceVariableController) UpdateVariable(c *gin.Context) {
	wsSemantic, ok := vc.resolvePathWorkspace(c)
	if !ok {
		return
	}
	varIDParam := c.Param("var_id")

	var req struct {
		Version      int                  `json:"version" binding:"required"` // 必须提供当前版本号
		Key          *string              `json:"key"`
		Value        *string              `json:"value"`
		VariableType *models.VariableType `json:"variable_type"`
		ValueFormat  *models.ValueFormat  `json:"value_format"`
		Sensitive    *bool                `json:"sensitive"`
		Description  *string              `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "请求参数无效，必须提供version字段",
			"error":     err.Error(),
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// 构建更新map
	updates := make(map[string]interface{})
	if req.Key != nil {
		updates["key"] = *req.Key
	}
	if req.Value != nil {
		updates["value"] = *req.Value
	}
	if req.VariableType != nil {
		updates["variable_type"] = *req.VariableType
	}
	if req.ValueFormat != nil {
		updates["value_format"] = *req.ValueFormat
	}
	if req.Sensitive != nil {
		updates["sensitive"] = *req.Sensitive
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "没有提供更新字段",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// 判断是数字ID还是variable_id，均绑定路径 workspace
	var result *services.VariableUpdateResult
	var err error

	if _, parseErr := strconv.ParseUint(varIDParam, 10, 32); parseErr == nil {
		varID, _ := strconv.ParseUint(varIDParam, 10, 32)
		result, err = vc.variableService.UpdateVariableInWorkspace(wsSemantic, uint(varID), req.Version, updates)
	} else {
		result, err = vc.variableService.UpdateVariableByVariableIDInWorkspace(wsSemantic, varIDParam, req.Version, updates)
	}
	
	if err != nil {
		// 检查是否是版本冲突错误
		if len(err.Error()) >= 4 && err.Error()[:4] == "版本冲突" {
			c.JSON(http.StatusConflict, gin.H{
				"code":      409,
				"message":   err.Error(),
				"timestamp": time.Now().Format(time.RFC3339),
			})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":      400,
				"message":   err.Error(),
				"timestamp": time.Now().Format(time.RFC3339),
			})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": result.NewVariable.ToResponse(),
		"version_info": gin.H{
			"variable_id": result.VariableID,
			"old_version": result.OldVersion,
			"new_version": result.NewVersion,
		},
		"message":   "变量更新成功",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// DeleteVariable 删除变量
// @Summary 删除变量
// @Description 删除指定的工作空间变量（软删除）
// @Tags Workspace Variable
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param var_id path string true "变量ID（支持数字ID或variable_id）"
// @Success 200 {object} map[string]interface{} "删除成功"
// @Failure 400 {object} map[string]interface{} "无效的变量ID"
// @Router /api/v1/workspaces/{id}/variables/{var_id} [delete]
// @Security BearerAuth
func (vc *WorkspaceVariableController) DeleteVariable(c *gin.Context) {
	wsSemantic, ok := vc.resolvePathWorkspace(c)
	if !ok {
		return
	}
	varIDParam := c.Param("var_id")

	var err error
	if _, parseErr := strconv.ParseUint(varIDParam, 10, 32); parseErr == nil {
		varID, _ := strconv.ParseUint(varIDParam, 10, 32)
		err = vc.variableService.DeleteVariableInWorkspace(wsSemantic, uint(varID))
	} else {
		err = vc.variableService.DeleteVariableByVariableIDInWorkspace(wsSemantic, varIDParam)
	}

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   err.Error(),
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"message":   "变量删除成功",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// GetVariableVersions 获取变量的版本历史
// @Summary 获取变量版本历史
// @Description 获取指定变量的所有历史版本
// @Tags Workspace Variable
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param var_id path string true "变量ID（支持数字ID或variable_id）"
// @Success 200 {object} map[string]interface{} "成功返回版本历史"
// @Failure 400 {object} map[string]interface{} "无效的变量ID"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/workspaces/{id}/variables/{var_id}/versions [get]
// @Security BearerAuth
func (vc *WorkspaceVariableController) GetVariableVersions(c *gin.Context) {
	wsSemantic, ok := vc.resolvePathWorkspace(c)
	if !ok {
		return
	}
	varIDParam := c.Param("var_id")

	var variableID string
	if _, err := strconv.ParseUint(varIDParam, 10, 32); err == nil {
		varID, _ := strconv.ParseUint(varIDParam, 10, 32)
		variable, err := vc.variableService.GetVariableInWorkspace(wsSemantic, uint(varID))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"code":      404,
				"message":   "变量不存在",
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return
		}
		variableID = variable.VariableID
	} else {
		variableID = varIDParam
	}

	versions, err := vc.variableService.GetVariableVersionsInWorkspace(wsSemantic, variableID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "获取版本历史失败",
			"error":     err.Error(),
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}
	
	// 转换为响应格式
	responses := make([]*models.WorkspaceVariableResponse, len(versions))
	for i, v := range versions {
		responses[i] = v.ToResponse()
	}
	
	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"data":      responses,
		"total":     len(responses),
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// GetVariableVersion 获取变量的指定版本
// @Summary 获取变量指定版本
// @Description 获取变量的指定版本详情
// @Tags Workspace Variable
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param var_id path string true "变量ID（支持数字ID或variable_id）"
// @Param version path int true "版本号"
// @Success 200 {object} map[string]interface{} "成功返回版本详情"
// @Failure 400 {object} map[string]interface{} "无效的参数"
// @Failure 404 {object} map[string]interface{} "版本不存在"
// @Router /api/v1/workspaces/{id}/variables/{var_id}/versions/{version} [get]
// @Security BearerAuth
func (vc *WorkspaceVariableController) GetVariableVersion(c *gin.Context) {
	wsSemantic, ok := vc.resolvePathWorkspace(c)
	if !ok {
		return
	}
	varIDParam := c.Param("var_id")
	versionParam := c.Param("version")

	version, err := strconv.Atoi(versionParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "无效的版本号",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	var variableID string
	if _, err := strconv.ParseUint(varIDParam, 10, 32); err == nil {
		varID, _ := strconv.ParseUint(varIDParam, 10, 32)
		variable, err := vc.variableService.GetVariableInWorkspace(wsSemantic, uint(varID))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"code":      404,
				"message":   "变量不存在",
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return
		}
		variableID = variable.VariableID
	} else {
		// 是variable_id：先校验归属路径 workspace
		if _, err := vc.variableService.GetLatestByVariableIDInWorkspace(wsSemantic, varIDParam); err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"code":      404,
				"message":   "变量不存在",
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return
		}
		variableID = varIDParam
	}

	// 获取指定版本（双条件：workspace + variable_id + version）
	variable, err := vc.variableService.GetVariableVersionInWorkspace(wsSemantic, variableID, version)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":      404,
			"message":   "版本不存在",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"data":      variable.ToResponse(),
		"timestamp": time.Now().Format(time.RFC3339),
	})
}
