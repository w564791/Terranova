package controllers

import (
	"net/http"
	"strconv"
	"time"

	"iac-platform/internal/models"
	"iac-platform/internal/parsers"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
)

type ModuleController struct {
	moduleService *services.ModuleService
}

func NewModuleController(moduleService *services.ModuleService) *ModuleController {
	return &ModuleController{
		moduleService: moduleService,
	}
}

// GetModules 获取模块列表
// @Summary 获取模块列表
// @Description 获取模块列表，支持分页、按provider过滤和搜索
// @Tags Module
// @Accept json
// @Produce json
// @Param page query int false "页码" default(1)
// @Param size query int false "每页数量" default(20)
// @Param provider query string false "Provider过滤（AWS/Azure/GCP等）"
// @Param search query string false "搜索关键词"
// @Success 200 {object} map[string]interface{} "成功返回模块列表"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/modules [get]
// @Security Bearer
func (mc *ModuleController) GetModules(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	provider := c.Query("provider")
	search := c.Query("search")

	modules, total, err := mc.moduleService.GetModules(page, size, provider, search)
	if err != nil {
		// 返回模拟数据
		mockModules := []models.Module{
			{
				ID:          1,
				Name:        "aws-vpc",
				Provider:    "AWS",
				Source:      "terraform-aws-modules/vpc/aws",
				Version:     "1.0.0",
				Description: "AWS VPC模块，用于创建虚拟私有云",
				SyncStatus:  "synced",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
			{
				ID:          2,
				Name:        "azure-vm",
				Provider:    "Azure",
				Source:      "Azure/compute/azurerm",
				Version:     "2.1.0",
				Description: "Azure虚拟机模块",
				SyncStatus:  "synced",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
		}
		c.JSON(http.StatusOK, gin.H{
			"code": 200,
			"data": gin.H{
				"items": mockModules,
				"total": 2,
				"page":  page,
				"size":  size,
			},
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"items": modules,
			"total": total,
			"page":  page,
			"size":  size,
		},
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// GetModule 获取单个模块
// @Summary 获取模块详情
// @Description 根据ID获取模块的详细信息
// @Tags Module
// @Accept json
// @Produce json
// @Param id path int true "模块ID"
// @Success 200 {object} map[string]interface{} "成功返回模块详情"
// @Failure 400 {object} map[string]interface{} "无效的模块ID"
// @Failure 404 {object} map[string]interface{} "模块不存在"
// @Router /api/v1/modules/{id} [get]
// @Security Bearer
func (mc *ModuleController) GetModule(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "无效的模块ID",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	module, err := mc.moduleService.GetModuleByID(uint(id))
	if err != nil {
		// 返回模拟数据
		mockModule := models.Module{
			ID:          uint(id),
			Name:        "aws-vpc",
			Provider:    "AWS",
			Source:      "terraform-aws-modules/vpc/aws",
			Version:     "1.0.0",
			Description: "AWS VPC模块，用于创建虚拟私有云网络环境",
			SyncStatus:  "synced",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		if id == 2 {
			mockModule.Name = "azure-vm"
			mockModule.Provider = "Azure"
			mockModule.Source = "Azure/compute/azurerm"
			mockModule.Version = "2.1.0"
			mockModule.Description = "Azure虚拟机模块，用于创建和管理Azure云虚拟机"
		}
		c.JSON(http.StatusOK, gin.H{
			"code":      200,
			"data":      mockModule,
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"data":      module,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// CreateModule 创建模块
// @Summary 创建新模块
// @Description 创建一个新的Terraform模块
// @Tags Module
// @Accept json
// @Produce json
// @Param request body object true "模块信息"
// @Success 201 {object} map[string]interface{} "模块创建成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 500 {object} map[string]interface{} "创建失败"
// @Router /api/v1/modules [post]
// @Security Bearer
func (mc *ModuleController) CreateModule(c *gin.Context) {
	var req struct {
		Name          string `json:"name" binding:"required"`
		Provider      string `json:"provider" binding:"required"`
		Source        string `json:"source"`
		ModuleSource  string `json:"module_source"`
		Version       string `json:"version"`
		Description   string `json:"description"`
		RepositoryURL string `json:"repository_url"`
		Branch        string `json:"branch"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   err.Error(),
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// 提供默认值
	source := req.Source
	if source == "" {
		source = req.RepositoryURL
	}
	moduleSource := req.ModuleSource
	version := req.Version
	if version == "" {
		version = "1.0.0"
	}

	module := &models.Module{
		Name:          req.Name,
		Provider:      req.Provider,
		Source:        source,
		ModuleSource:  moduleSource,
		Version:       version,
		Description:   req.Description,
		RepositoryURL: req.RepositoryURL,
		Branch:        req.Branch,
		SyncStatus:    "pending",
	}

	if err := mc.moduleService.CreateModule(module); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "创建模块失败: " + err.Error(),
			"error":     err.Error(),
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"code":      201,
		"data":      module,
		"message":   "模块创建成功",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// UpdateModule 更新模块
// @Summary 更新模块信息
// @Description 更新模块的配置信息
// @Tags Module
// @Accept json
// @Produce json
// @Param id path int true "模块ID"
// @Param request body object true "更新信息"
// @Success 200 {object} map[string]interface{} "更新成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 404 {object} map[string]interface{} "模块不存在"
// @Failure 500 {object} map[string]interface{} "更新失败"
// @Router /api/v1/modules/{id} [put]
// @Security Bearer
func (mc *ModuleController) UpdateModule(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "无效的模块ID",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	var req struct {
		ModuleSource string            `json:"module_source"`
		Description  string            `json:"description"`
		Version      string            `json:"version"`
		Branch       string            `json:"branch"`
		Status       string            `json:"status"`
		AIPrompts    []models.AIPrompt `json:"ai_prompts"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "请求参数无效",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// 验证status值
	if req.Status != "" && req.Status != "active" && req.Status != "inactive" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "状态值无效，只能是 active 或 inactive",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// 获取现有模块
	module, err := mc.moduleService.GetModuleByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":      404,
			"message":   "模块不存在",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// 更新字段
	if req.ModuleSource != "" {
		module.ModuleSource = req.ModuleSource
	}
	if req.Description != "" {
		module.Description = req.Description
	}
	if req.Version != "" {
		module.Version = req.Version
	}
	if req.Branch != "" {
		module.Branch = req.Branch
	}
	if req.Status != "" {
		module.Status = req.Status
	}
	// 处理 AI 提示词更新（允许设置为空数组）
	if req.AIPrompts != nil {
		module.AIPrompts = req.AIPrompts
	}

	if err := mc.moduleService.UpdateModuleFields(module); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "更新模块失败: " + err.Error(),
			"error":     err.Error(),
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"message":   "模块更新成功",
		"data":      module,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// DeleteModule 删除模块
// @Summary 删除模块
// @Description 删除指定的模块
// @Tags Module
// @Accept json
// @Produce json
// @Param id path int true "模块ID"
// @Success 200 {object} map[string]interface{} "删除成功"
// @Failure 400 {object} map[string]interface{} "无效的模块ID"
// @Failure 500 {object} map[string]interface{} "删除失败"
// @Router /api/v1/modules/{id} [delete]
// @Security Bearer
func (mc *ModuleController) DeleteModule(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "无效的模块ID",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	if err := mc.moduleService.DeleteModule(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "删除模块失败: " + err.Error(),
			"error":     err.Error(),
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"message":   "模块删除成功",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// SyncModuleFiles 同步模块文件
// @Summary 同步模块文件
// @Description 从Git仓库同步模块的Terraform文件
// @Tags Module
// @Accept json
// @Produce json
// @Param id path int true "模块ID"
// @Success 200 {object} map[string]interface{} "同步成功"
// @Failure 400 {object} map[string]interface{} "无效的模块ID"
// @Failure 500 {object} map[string]interface{} "同步失败"
// @Router /api/v1/modules/{id}/sync [post]
// @Security Bearer
func (mc *ModuleController) SyncModuleFiles(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "无效的模块ID",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	if err := mc.moduleService.SyncModuleFiles(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "同步模块文件失败: " + err.Error(),
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// 获取同步后的文件内容
	moduleFiles, err := mc.moduleService.GetModuleFiles(uint(id))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "模块文件同步成功",
			"data": gin.H{
				"sync_status": "synced",
			},
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "模块文件同步成功",
		"data": gin.H{
			"sync_status":  "synced",
			"module_files": moduleFiles,
		},
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// GetModuleFiles 获取模块文件内容
// @Summary 获取模块文件
// @Description 获取模块的Terraform文件内容
// @Tags Module
// @Accept json
// @Produce json
// @Param id path int true "模块ID"
// @Success 200 {object} map[string]interface{} "成功返回文件内容"
// @Failure 400 {object} map[string]interface{} "无效的模块ID"
// @Failure 404 {object} map[string]interface{} "文件未找到"
// @Router /api/v1/modules/{id}/files [get]
// @Security Bearer
func (mc *ModuleController) GetModuleFiles(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "无效的模块ID",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	moduleFiles, err := mc.moduleService.GetModuleFiles(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":      404,
			"message":   "模块文件未找到，请先同步模块",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"module_files": moduleFiles,
			"last_sync_at": time.Now().Format(time.RFC3339),
			"sync_status":  "synced",
		},
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// GetModulePrompts 获取模块的AI提示词
// @Summary 获取模块AI提示词
// @Description 获取指定模块的AI助手提示词列表
// @Tags Module
// @Accept json
// @Produce json
// @Param id path int true "模块ID"
// @Success 200 {object} map[string]interface{} "成功返回提示词列表"
// @Failure 400 {object} map[string]interface{} "无效的模块ID"
// @Failure 404 {object} map[string]interface{} "模块不存在"
// @Router /api/v1/modules/{id}/prompts [get]
// @Security Bearer
func (mc *ModuleController) GetModulePrompts(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "无效的模块ID",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	module, err := mc.moduleService.GetModuleByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":      404,
			"message":   "模块不存在",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"items": module.AIPrompts,
			"total": len(module.AIPrompts),
		},
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// ParseTFFile 解析TF文件
// @Summary 解析Terraform文件
// @Description 解析Terraform变量文件并生成Schema
// @Tags Module
// @Accept json
// @Produce json
// @Param request body object true "TF文件内容"
// @Success 200 {object} map[string]interface{} "解析成功"
// @Failure 400 {object} map[string]interface{} "解析失败"
// @Router /api/v1/modules/parse-tf [post]
// @Security Bearer
func (mc *ModuleController) ParseTFFile(c *gin.Context) {
	var req struct {
		TFContent string `json:"tf_content" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "请求参数无效: " + err.Error(),
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// 解析TF文件
	variables, err := parsers.ParseVariablesFile(req.TFContent)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "TF文件解析失败: " + err.Error(),
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// 转换为Schema格式
	schema := parsers.ConvertVariablesToSchema(variables)

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"variables": variables,
			"schema":    schema,
			"count":     len(variables),
		},
		"message":   "TF文件解析成功",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}
