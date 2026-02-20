package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"iac-platform/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// WorkspaceOutputController 工作空间Output控制器
type WorkspaceOutputController struct {
	db *gorm.DB
}

// NewWorkspaceOutputController 创建控制器实例
func NewWorkspaceOutputController(db *gorm.DB) *WorkspaceOutputController {
	return &WorkspaceOutputController{db: db}
}

// generateOutputID 生成Output语义化ID
func generateOutputID() string {
	return fmt.Sprintf("output-%s", uuid.New().String()[:12])
}

// ListOutputs 获取workspace的outputs列表
// @Summary 获取Outputs列表
// @Description 获取工作空间的所有Outputs配置
// @Tags Workspace Output
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Success 200 {object} map[string]interface{} "成功返回Outputs列表"
// @Failure 400 {object} map[string]interface{} "无效的工作空间ID"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/workspaces/{id}/outputs [get]
// @Security Bearer
func (c *WorkspaceOutputController) ListOutputs(ctx *gin.Context) {
	workspaceIDParam := ctx.Param("id")
	if workspaceIDParam == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的workspace ID",
		})
		return
	}

	// 获取workspace
	var workspace models.Workspace
	err := c.db.Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := c.db.Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			ctx.JSON(http.StatusNotFound, gin.H{
				"code":    404,
				"message": "Workspace不存在",
			})
			return
		}
	}

	// 查询outputs列表
	var outputs []models.WorkspaceOutput
	if err := c.db.Where("workspace_id = ?", workspace.WorkspaceID).
		Order("resource_name ASC, output_name ASC").
		Find(&outputs).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取Outputs列表失败",
		})
		return
	}

	// 按资源分组
	resourceOutputs := make(map[string][]models.WorkspaceOutput)
	for _, output := range outputs {
		resourceOutputs[output.ResourceName] = append(resourceOutputs[output.ResourceName], output)
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code":             200,
		"outputs":          outputs,
		"resource_outputs": resourceOutputs,
		"total":            len(outputs),
	})
}

// CreateOutput 创建Output配置
// @Summary 创建Output
// @Description 为工作空间资源创建Output配置，支持资源关联输出和静态值输出
// @Tags Workspace Output
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param request body object true "Output配置"
// @Success 201 {object} map[string]interface{} "创建成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 500 {object} map[string]interface{} "创建失败"
// @Router /api/v1/workspaces/{id}/outputs [post]
// @Security Bearer
func (c *WorkspaceOutputController) CreateOutput(ctx *gin.Context) {
	workspaceIDParam := ctx.Param("id")
	if workspaceIDParam == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的workspace ID",
		})
		return
	}

	// 获取workspace
	var workspace models.Workspace
	err := c.db.Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := c.db.Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			ctx.JSON(http.StatusNotFound, gin.H{
				"code":    404,
				"message": "Workspace不存在",
			})
			return
		}
	}

	userID, _ := ctx.Get("user_id")
	uid := userID.(string)

	var req struct {
		ResourceName string `json:"resource_name"`                  // 资源名称，为空或"__static__"表示静态输出
		OutputName   string `json:"output_name" binding:"required"` // 输出名称
		OutputValue  string `json:"output_value"`                   // 静态输出的值（仅静态输出时使用）
		Description  string `json:"description"`                    // 描述
		Sensitive    bool   `json:"sensitive"`                      // 是否敏感
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	// 判断是否为静态输出
	isStaticOutput := req.ResourceName == "" || req.ResourceName == models.StaticOutputResourceName

	var outputValue string
	var resourceName string

	if isStaticOutput {
		// 静态输出：直接使用用户提供的值
		if req.OutputValue == "" {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "静态输出必须提供output_value",
			})
			return
		}
		outputValue = req.OutputValue
		resourceName = models.StaticOutputResourceName

		// 检查是否已存在相同名称的静态输出
		var existingCount int64
		c.db.Model(&models.WorkspaceOutput{}).
			Where("workspace_id = ? AND resource_name = ? AND output_name = ?",
				workspace.WorkspaceID, models.StaticOutputResourceName, req.OutputName).
			Count(&existingCount)

		if existingCount > 0 {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "已存在相同名称的静态Output",
			})
			return
		}
	} else {
		// 资源关联输出：检查资源是否存在
		resourceName = req.ResourceName

		// 检查是否已存在相同的output
		var existingCount int64
		c.db.Model(&models.WorkspaceOutput{}).
			Where("workspace_id = ? AND resource_name = ? AND output_name = ?",
				workspace.WorkspaceID, req.ResourceName, req.OutputName).
			Count(&existingCount)

		if existingCount > 0 {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "该资源已存在相同名称的Output",
			})
			return
		}

		// 查找资源以获取完整的 resource_id
		var resource models.WorkspaceResource
		if err := c.db.Where("workspace_id = ? AND resource_name = ?",
			workspace.WorkspaceID, req.ResourceName).First(&resource).Error; err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "资源不存在: " + req.ResourceName,
			})
			return
		}

		// 生成output值表达式
		// Terraform module 名称是 resource_id 中的点号替换为下划线
		// 例如: AWS_tesr-ccd.ken-aaa-2025-10-12-02 -> AWS_tesr-ccd_ken-aaa-2025-10-12-02
		moduleName := strings.ReplaceAll(resource.ResourceID, ".", "_")
		outputValue = fmt.Sprintf("module.%s.%s", moduleName, req.OutputName)
	}

	output := &models.WorkspaceOutput{
		WorkspaceID:  workspace.WorkspaceID,
		OutputID:     generateOutputID(),
		ResourceName: resourceName,
		OutputName:   req.OutputName,
		OutputValue:  outputValue,
		Description:  req.Description,
		Sensitive:    req.Sensitive,
		CreatedBy:    &uid,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := c.db.Create(output).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "创建Output失败: " + err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"code":    201,
		"message": "Output创建成功",
		"output":  output,
	})
}

// UpdateOutput 更新Output配置
// @Summary 更新Output
// @Description 更新Output配置
// @Tags Workspace Output
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param output_id path string true "Output ID"
// @Param request body object true "Output配置"
// @Success 200 {object} map[string]interface{} "更新成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 404 {object} map[string]interface{} "Output不存在"
// @Failure 500 {object} map[string]interface{} "更新失败"
// @Router /api/v1/workspaces/{id}/outputs/{output_id} [put]
// @Security Bearer
func (c *WorkspaceOutputController) UpdateOutput(ctx *gin.Context) {
	workspaceIDParam := ctx.Param("id")
	outputIDParam := ctx.Param("output_id")

	if workspaceIDParam == "" || outputIDParam == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的参数",
		})
		return
	}

	// 获取workspace
	var workspace models.Workspace
	err := c.db.Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := c.db.Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			ctx.JSON(http.StatusNotFound, gin.H{
				"code":    404,
				"message": "Workspace不存在",
			})
			return
		}
	}

	// 查找output - 只通过output_id查找
	var output models.WorkspaceOutput
	if err := c.db.Where("workspace_id = ? AND output_id = ?",
		workspace.WorkspaceID, outputIDParam).First(&output).Error; err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "Output不存在",
		})
		return
	}

	var req struct {
		OutputName  string `json:"output_name"`
		Description string `json:"description"`
		Sensitive   bool   `json:"sensitive"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	// 更新字段
	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}

	if req.OutputName != "" && req.OutputName != output.OutputName {
		// 检查是否已存在相同的output
		var existingCount int64
		c.db.Model(&models.WorkspaceOutput{}).
			Where("workspace_id = ? AND resource_name = ? AND output_name = ? AND id != ?",
				workspace.WorkspaceID, output.ResourceName, req.OutputName, output.ID).
			Count(&existingCount)

		if existingCount > 0 {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "该资源已存在相同名称的Output",
			})
			return
		}

		// 查找资源以获取正确的 resource_id
		var resource models.WorkspaceResource
		if err := c.db.Where("workspace_id = ? AND resource_name = ?",
			workspace.WorkspaceID, output.ResourceName).First(&resource).Error; err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "资源不存在: " + output.ResourceName,
			})
			return
		}

		// Terraform module 名称是 resource_id 中的点号替换为下划线
		moduleName := strings.ReplaceAll(resource.ResourceID, ".", "_")

		updates["output_name"] = req.OutputName
		updates["output_value"] = fmt.Sprintf("module.%s.%s", moduleName, req.OutputName)
	}

	updates["description"] = req.Description
	updates["sensitive"] = req.Sensitive

	if err := c.db.Model(&output).Updates(updates).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "更新Output失败: " + err.Error(),
		})
		return
	}

	// 重新查询更新后的数据
	c.db.First(&output, output.ID)

	ctx.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Output更新成功",
		"output":  output,
	})
}

// DeleteOutput 删除Output配置
// @Summary 删除Output
// @Description 删除Output配置
// @Tags Workspace Output
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param output_id path string true "Output ID"
// @Success 200 {object} map[string]interface{} "删除成功"
// @Failure 400 {object} map[string]interface{} "无效的参数"
// @Failure 404 {object} map[string]interface{} "Output不存在"
// @Failure 500 {object} map[string]interface{} "删除失败"
// @Router /api/v1/workspaces/{id}/outputs/{output_id} [delete]
// @Security Bearer
func (c *WorkspaceOutputController) DeleteOutput(ctx *gin.Context) {
	workspaceIDParam := ctx.Param("id")
	outputIDParam := ctx.Param("output_id")

	if workspaceIDParam == "" || outputIDParam == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的参数",
		})
		return
	}

	// 获取workspace
	var workspace models.Workspace
	err := c.db.Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := c.db.Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			ctx.JSON(http.StatusNotFound, gin.H{
				"code":    404,
				"message": "Workspace不存在",
			})
			return
		}
	}

	// 查找并删除output - 只通过output_id查找
	result := c.db.Where("workspace_id = ? AND output_id = ?",
		workspace.WorkspaceID, outputIDParam).Delete(&models.WorkspaceOutput{})

	if result.Error != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "删除Output失败: " + result.Error.Error(),
		})
		return
	}

	if result.RowsAffected == 0 {
		ctx.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "Output不存在",
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Output删除成功",
	})
}

// GetStateOutputs 获取当前State中的Outputs信息（WebUI使用，不返回sensitive数据）
// @Summary 获取State Outputs（WebUI）
// @Description 从当前State中获取Outputs信息（清空resources字段）。Sensitive数据的值会被屏蔽。
// @Tags Workspace Output
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Success 200 {object} models.StateOutputInfo "成功返回State Outputs"
// @Failure 400 {object} map[string]interface{} "无效的工作空间ID"
// @Failure 404 {object} map[string]interface{} "State不存在"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/workspaces/{id}/state-outputs [get]
// @Security Bearer
func (c *WorkspaceOutputController) GetStateOutputs(ctx *gin.Context) {
	c.getStateOutputsInternal(ctx, false)
}

// GetStateOutputsFull 获取当前State中的完整Outputs信息（API使用，包含sensitive数据）
// @Summary 获取完整State Outputs（API）
// @Description 从当前State中获取完整Outputs信息，包含sensitive数据的值。仅供API调用使用。
// @Tags Workspace Output
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Success 200 {object} models.StateOutputInfo "成功返回完整State Outputs"
// @Failure 400 {object} map[string]interface{} "无效的工作空间ID"
// @Failure 404 {object} map[string]interface{} "State不存在"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/workspaces/{id}/state-outputs/full [get]
// @Security Bearer
func (c *WorkspaceOutputController) GetStateOutputsFull(ctx *gin.Context) {
	c.getStateOutputsInternal(ctx, true)
}

// getStateOutputsInternal 内部方法，获取State Outputs
func (c *WorkspaceOutputController) getStateOutputsInternal(ctx *gin.Context, includeSensitive bool) {
	workspaceIDParam := ctx.Param("id")
	if workspaceIDParam == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的workspace ID",
		})
		return
	}

	// 获取workspace
	var workspace models.Workspace
	err := c.db.Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := c.db.Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			ctx.JSON(http.StatusNotFound, gin.H{
				"error": "Workspace不存在",
			})
			return
		}
	}

	// 查询最新的state version记录
	var stateVersion models.WorkspaceStateVersion
	if err := c.db.Where("workspace_id = ?", workspace.WorkspaceID).
		Order("version DESC").
		First(&stateVersion).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// 没有state，返回空outputs（直接返回StateOutputInfo格式）
			ctx.JSON(http.StatusOK, models.StateOutputInfo{
				CheckResults:     nil,
				Lineage:          "",
				Outputs:          make(map[string]models.OutputValue),
				Resources:        []interface{}{}, // 清空resources
				Serial:           0,
				TerraformVersion: "",
				Version:          0,
			})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取state失败",
		})
		return
	}

	// 从state content中提取信息
	stateInfo := models.StateOutputInfo{
		CheckResults:     nil,
		Lineage:          "",
		Outputs:          make(map[string]models.OutputValue),
		Resources:        []interface{}{}, // 清空resources
		Serial:           0,
		TerraformVersion: "",
		Version:          0,
	}

	if stateVersion.Content != nil {
		// 提取lineage
		if lineage, ok := stateVersion.Content["lineage"].(string); ok {
			stateInfo.Lineage = lineage
		}

		// 提取serial
		if serial, ok := stateVersion.Content["serial"].(float64); ok {
			stateInfo.Serial = int(serial)
		}

		// 提取terraform_version
		if tfVersion, ok := stateVersion.Content["terraform_version"].(string); ok {
			stateInfo.TerraformVersion = tfVersion
		}

		// 提取version
		if version, ok := stateVersion.Content["version"].(float64); ok {
			stateInfo.Version = int(version)
		}

		// 提取check_results
		if checkResults, ok := stateVersion.Content["check_results"]; ok {
			stateInfo.CheckResults = checkResults
		}

		// 提取outputs - 只返回值不为null的outputs
		if outputs, ok := stateVersion.Content["outputs"].(map[string]interface{}); ok {
			for key, val := range outputs {
				if outputMap, ok := val.(map[string]interface{}); ok {
					// 检查value是否为null，如果是null则跳过（已删除的output）
					value, hasValue := outputMap["value"]
					if !hasValue || value == nil {
						continue // 跳过已删除的output
					}

					outputValue := models.OutputValue{}

					// 检查是否为sensitive
					isSensitive := false
					if s, ok := outputMap["sensitive"].(bool); ok {
						isSensitive = s
						outputValue.Sensitive = s
					}

					// 如果是sensitive且不包含sensitive数据，则屏蔽值
					if isSensitive && !includeSensitive {
						outputValue.Value = nil // 屏蔽值
					} else {
						outputValue.Value = value
					}

					if t, ok := outputMap["type"]; ok {
						outputValue.Type = t
					}

					stateInfo.Outputs[key] = outputValue
				}
			}
		}
	}

	// 直接返回StateOutputInfo格式，符合用户要求的数据结构
	ctx.JSON(http.StatusOK, stateInfo)
}

// GetResourcesForOutputs 获取可用于创建outputs的资源列表
// @Summary 获取可用资源列表
// @Description 获取workspace中可用于创建outputs的资源列表
// @Tags Workspace Output
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Success 200 {object} map[string]interface{} "成功返回资源列表"
// @Failure 400 {object} map[string]interface{} "无效的工作空间ID"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/workspaces/{id}/outputs/resources [get]
// @Security Bearer
func (c *WorkspaceOutputController) GetResourcesForOutputs(ctx *gin.Context) {
	workspaceIDParam := ctx.Param("id")
	if workspaceIDParam == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的workspace ID",
		})
		return
	}

	// 获取workspace
	var workspace models.Workspace
	err := c.db.Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := c.db.Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			ctx.JSON(http.StatusNotFound, gin.H{
				"code":    404,
				"message": "Workspace不存在",
			})
			return
		}
	}

	// 获取workspace的资源列表
	var resources []models.WorkspaceResource
	if err := c.db.Where("workspace_id = ? AND is_active = ?", workspace.WorkspaceID, true).
		Order("resource_name ASC").
		Find(&resources).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取资源列表失败",
		})
		return
	}

	// 获取每个资源已配置的outputs数量
	type ResourceWithOutputCount struct {
		ResourceID   string `json:"resource_id"`
		ResourceName string `json:"resource_name"`
		ResourceType string `json:"resource_type"`
		OutputCount  int64  `json:"output_count"`
	}

	var result []ResourceWithOutputCount
	for _, resource := range resources {
		var count int64
		c.db.Model(&models.WorkspaceOutput{}).
			Where("workspace_id = ? AND resource_name = ?", workspace.WorkspaceID, resource.ResourceName).
			Count(&count)

		result = append(result, ResourceWithOutputCount{
			ResourceID:   resource.ResourceID,
			ResourceName: resource.ResourceName,
			ResourceType: resource.ResourceType,
			OutputCount:  count,
		})
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code":      200,
		"resources": result,
		"total":     len(result),
	})
}

// GetAvailableOutputs 获取可用的模块输出列表（用于智能提示）
// @Summary 获取可用模块输出
// @Description 获取workspace中所有资源的可用模块输出列表，用于配置outputs时的智能提示
// @Tags Workspace Output
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Success 200 {object} map[string]interface{} "成功返回可用输出列表"
// @Failure 400 {object} map[string]interface{} "无效的工作空间ID"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/workspaces/{id}/available-outputs [get]
// @Security Bearer
func (c *WorkspaceOutputController) GetAvailableOutputs(ctx *gin.Context) {
	workspaceIDParam := ctx.Param("id")
	if workspaceIDParam == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的workspace ID",
		})
		return
	}

	// 获取workspace
	var workspace models.Workspace
	err := c.db.Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := c.db.Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			ctx.JSON(http.StatusNotFound, gin.H{
				"code":    404,
				"message": "Workspace不存在",
			})
			return
		}
	}

	// 获取workspace的活跃资源列表（包含 tf_code）
	var resources []models.WorkspaceResource
	if err := c.db.Where("workspace_id = ? AND is_active = ?", workspace.WorkspaceID, true).
		Order("resource_name ASC").
		Find(&resources).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取资源列表失败",
		})
		return
	}

	// 获取每个资源的最新版本（包含 tf_code）
	resourceVersionMap := make(map[string]*models.ResourceCodeVersion) // resource_id -> version
	for _, resource := range resources {
		var version models.ResourceCodeVersion
		if err := c.db.Where("resource_id = ?", resource.ID).
			Order("version DESC").
			First(&version).Error; err == nil {
			resourceVersionMap[resource.ResourceID] = &version
		}
	}

	// 收集所有 ManifestDeploymentID
	deploymentIDs := make(map[string]bool)
	resourceDeploymentMap := make(map[string]string) // resource_id -> deployment_id
	for _, resource := range resources {
		if resource.ManifestDeploymentID != nil && *resource.ManifestDeploymentID != "" {
			deploymentIDs[*resource.ManifestDeploymentID] = true
			resourceDeploymentMap[resource.ResourceID] = *resource.ManifestDeploymentID
		}
	}

	// 获取 ManifestDeploymentResource 和 ManifestDeployment 信息
	type ResourceModuleInfo struct {
		ModuleID   uint
		ModuleName string
		Schema     *models.Schema
	}
	resourceModuleMap := make(map[string]ResourceModuleInfo) // resource_id -> module info

	for deploymentID := range deploymentIDs {
		// 获取部署信息
		var deployment models.ManifestDeployment
		if err := c.db.Where("id = ?", deploymentID).First(&deployment).Error; err != nil {
			continue
		}

		// 获取 ManifestVersion
		var manifestVersion models.ManifestVersion
		if err := c.db.Where("id = ?", deployment.VersionID).First(&manifestVersion).Error; err != nil {
			continue
		}

		// 解析 ManifestVersion 的 nodes 获取模块信息
		if manifestVersion.Nodes != nil {
			var nodesArray []interface{}
			if err := json.Unmarshal(manifestVersion.Nodes, &nodesArray); err == nil {
				for _, nodeData := range nodesArray {
					if nodeMap, ok := nodeData.(map[string]interface{}); ok {
						nodeID, _ := nodeMap["id"].(string)
						moduleIDFloat, hasModuleID := nodeMap["module_id"].(float64)

						if hasModuleID && moduleIDFloat > 0 {
							moduleID := uint(moduleIDFloat)

							// 查找对应的 ManifestDeploymentResource
							var mdr models.ManifestDeploymentResource
							if err := c.db.Where("deployment_id = ? AND node_id = ?", deploymentID, nodeID).First(&mdr).Error; err == nil {
								// 获取模块信息
								var module models.Module
								if err := c.db.First(&module, moduleID).Error; err == nil {
									// 获取模块的活跃schema (优先v2)
									var schema models.Schema
									err := c.db.Where("module_id = ? AND status = ?", moduleID, "active").
										Order("CASE WHEN schema_version = 'v2' THEN 0 ELSE 1 END, created_at DESC").
										First(&schema).Error

									info := ResourceModuleInfo{
										ModuleID:   moduleID,
										ModuleName: module.Name,
									}
									if err == nil {
										info.Schema = &schema
									}
									resourceModuleMap[mdr.ResourceID] = info
								}
							}
						}
					}
				}
			}
		}
	}

	// 对于没有通过 Manifest 部署的资源，尝试从 tf_code 中提取 module source 并查找对应的 module
	for _, resource := range resources {
		// 如果已经有 module 信息，跳过
		if _, ok := resourceModuleMap[resource.ResourceID]; ok {
			continue
		}

		// 从资源版本的 tf_code 中提取 module source
		version := resourceVersionMap[resource.ResourceID]
		if version == nil || version.TFCode == nil {
			continue
		}

		// 解析 tf_code 获取 module source
		moduleSource := c.extractModuleSourceFromTFCode(version.TFCode)
		if moduleSource == "" {
			continue
		}

		// 根据 module_source 查找对应的 module
		var module models.Module
		if err := c.db.Where("module_source = ? OR source = ?", moduleSource, moduleSource).First(&module).Error; err != nil {
			continue
		}

		// 获取模块的活跃schema (优先v2)
		var schema models.Schema
		err := c.db.Where("module_id = ? AND status = ?", module.ID, "active").
			Order("CASE WHEN schema_version = 'v2' THEN 0 ELSE 1 END, created_at DESC").
			First(&schema).Error

		info := ResourceModuleInfo{
			ModuleID:   module.ID,
			ModuleName: module.Name,
		}
		if err == nil {
			info.Schema = &schema
		}
		resourceModuleMap[resource.ResourceID] = info
	}

	// 构建响应
	type ResourceOutputs struct {
		ResourceName string              `json:"resourceName"`
		ResourceID   string              `json:"resourceId"`
		ResourceType string              `json:"resourceType"`
		ModuleName   string              `json:"moduleName"`
		ModuleID     uint                `json:"moduleId,omitempty"`
		Outputs      []OutputItemForHint `json:"outputs"`
	}

	var result []ResourceOutputs

	for _, resource := range resources {
		resourceOutput := ResourceOutputs{
			ResourceName: resource.ResourceName,
			ResourceID:   resource.ResourceID,
			ResourceType: resource.ResourceType,
			Outputs:      []OutputItemForHint{},
		}

		// Terraform module 名称是 resource_id 中的点号替换为下划线
		moduleName := strings.ReplaceAll(resource.ResourceID, ".", "_")

		if info, ok := resourceModuleMap[resource.ResourceID]; ok {
			resourceOutput.ModuleName = info.ModuleName
			resourceOutput.ModuleID = info.ModuleID

			// 从schema中提取outputs
			if info.Schema != nil && info.Schema.OpenAPISchema != nil {
				outputs := c.extractOutputsFromSchema(info.Schema.OpenAPISchema, moduleName)
				resourceOutput.Outputs = outputs
			}
		}

		result = append(result, resourceOutput)
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code":      200,
		"resources": result,
		"total":     len(result),
	})
}

// extractModuleSourceFromTFCode 从 tf_code 中提取 module source
func (c *WorkspaceOutputController) extractModuleSourceFromTFCode(tfCode models.JSONB) string {
	if tfCode == nil {
		return ""
	}

	// tf_code 格式: { "module": { "module_name": [{ "source": "...", ... }] } }
	moduleMap, ok := tfCode["module"].(map[string]interface{})
	if !ok {
		return ""
	}

	// 遍历所有 module 定义
	for _, moduleConfig := range moduleMap {
		// module 配置是一个数组
		moduleArray, ok := moduleConfig.([]interface{})
		if !ok || len(moduleArray) == 0 {
			continue
		}

		// 获取第一个配置
		config, ok := moduleArray[0].(map[string]interface{})
		if !ok {
			continue
		}

		// 提取 source
		if source, ok := config["source"].(string); ok && source != "" {
			return source
		}
	}

	return ""
}

// OutputItemForHint 用于智能提示的输出项
type OutputItemForHint struct {
	Name        string `json:"name"`
	Alias       string `json:"alias,omitempty"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Sensitive   bool   `json:"sensitive,omitempty"`
	Reference   string `json:"reference"`
}

// extractOutputsFromSchema 从OpenAPI Schema中提取outputs定义
func (c *WorkspaceOutputController) extractOutputsFromSchema(schema models.JSONB, moduleName string) []OutputItemForHint {
	var outputs []OutputItemForHint

	// 方式1: 从 x-iac-platform.outputs.items 提取
	if iacPlatform, ok := schema["x-iac-platform"].(map[string]interface{}); ok {
		if outputsConfig, ok := iacPlatform["outputs"].(map[string]interface{}); ok {
			if items, ok := outputsConfig["items"].([]interface{}); ok {
				for _, item := range items {
					if outputMap, ok := item.(map[string]interface{}); ok {
						output := OutputItemForHint{
							Reference: fmt.Sprintf("module.%s.", moduleName),
						}

						if name, ok := outputMap["name"].(string); ok {
							output.Name = name
							output.Reference = fmt.Sprintf("module.%s.%s", moduleName, name)
						}
						if alias, ok := outputMap["alias"].(string); ok {
							output.Alias = alias
						}
						if t, ok := outputMap["type"].(string); ok {
							output.Type = t
						}
						if desc, ok := outputMap["description"].(string); ok {
							output.Description = desc
						}
						if sensitive, ok := outputMap["sensitive"].(bool); ok {
							output.Sensitive = sensitive
						}

						outputs = append(outputs, output)
					}
				}
			}
		}
	}

	// 方式2: 从 components.schemas.ModuleOutput.properties 提取
	if len(outputs) == 0 {
		if components, ok := schema["components"].(map[string]interface{}); ok {
			if schemas, ok := components["schemas"].(map[string]interface{}); ok {
				if moduleOutput, ok := schemas["ModuleOutput"].(map[string]interface{}); ok {
					if properties, ok := moduleOutput["properties"].(map[string]interface{}); ok {
						for name, prop := range properties {
							propMap, ok := prop.(map[string]interface{})
							if !ok {
								continue
							}

							output := OutputItemForHint{
								Name:      name,
								Reference: fmt.Sprintf("module.%s.%s", moduleName, name),
								Type:      "string", // 默认类型
							}

							if t, ok := propMap["type"].(string); ok {
								output.Type = t
							}
							if desc, ok := propMap["description"].(string); ok {
								output.Description = desc
							}
							if alias, ok := propMap["x-alias"].(string); ok {
								output.Alias = alias
							}
							if sensitive, ok := propMap["x-sensitive"].(bool); ok {
								output.Sensitive = sensitive
							}

							outputs = append(outputs, output)
						}
					}
				}
			}
		}
	}

	return outputs
}

// BatchSaveOutputs 批量保存Outputs配置（支持添加、更新、删除）
// @Summary 批量保存Outputs
// @Description 批量保存Outputs配置，支持一次性添加、更新、删除多个outputs，包括资源关联输出和静态值输出
// @Tags Workspace Output
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param request body object true "批量操作请求"
// @Success 200 {object} map[string]interface{} "保存成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 500 {object} map[string]interface{} "保存失败"
// @Router /api/v1/workspaces/{id}/outputs/batch [post]
// @Security Bearer
func (c *WorkspaceOutputController) BatchSaveOutputs(ctx *gin.Context) {
	workspaceIDParam := ctx.Param("id")
	if workspaceIDParam == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的workspace ID",
		})
		return
	}

	// 获取workspace
	var workspace models.Workspace
	err := c.db.Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := c.db.Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			ctx.JSON(http.StatusNotFound, gin.H{
				"code":    404,
				"message": "Workspace不存在",
			})
			return
		}
	}

	userID, _ := ctx.Get("user_id")
	uid := userID.(string)

	var req struct {
		// 要创建的outputs（资源关联输出）
		Create []struct {
			ResourceName string `json:"resource_name"`
			OutputName   string `json:"output_name"`
			Description  string `json:"description"`
			Sensitive    bool   `json:"sensitive"`
		} `json:"create"`
		// 要创建的静态outputs
		CreateStatic []struct {
			OutputName  string `json:"output_name"`
			OutputValue string `json:"output_value"`
			Description string `json:"description"`
			Sensitive   bool   `json:"sensitive"`
		} `json:"create_static"`
		// 要更新的outputs
		Update []struct {
			OutputID    string `json:"output_id"`
			OutputName  string `json:"output_name"`
			OutputValue string `json:"output_value"` // 仅静态输出可更新此字段
			Description string `json:"description"`
			Sensitive   bool   `json:"sensitive"`
		} `json:"update"`
		// 要删除的output IDs
		Delete []string `json:"delete"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	// 使用事务处理批量操作
	tx := c.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var created, updated, deleted int
	var errors []string

	// 1. 处理删除
	for _, outputID := range req.Delete {
		result := tx.Where("workspace_id = ? AND output_id = ?",
			workspace.WorkspaceID, outputID).Delete(&models.WorkspaceOutput{})
		if result.Error != nil {
			errors = append(errors, fmt.Sprintf("删除 %s 失败: %s", outputID, result.Error.Error()))
		} else if result.RowsAffected > 0 {
			deleted++
		}
	}

	// 2. 处理更新
	for _, item := range req.Update {
		var output models.WorkspaceOutput
		if err := tx.Where("workspace_id = ? AND output_id = ?",
			workspace.WorkspaceID, item.OutputID).First(&output).Error; err != nil {
			errors = append(errors, fmt.Sprintf("更新 %s 失败: 不存在", item.OutputID))
			continue
		}

		updates := map[string]interface{}{
			"description": item.Description,
			"sensitive":   item.Sensitive,
			"updated_at":  time.Now(),
		}

		// 判断是否为静态输出
		isStaticOutput := output.IsStaticOutput()

		if isStaticOutput {
			// 静态输出：可以更新output_name和output_value
			if item.OutputName != "" && item.OutputName != output.OutputName {
				// 检查是否已存在相同名称的静态输出
				var existingCount int64
				tx.Model(&models.WorkspaceOutput{}).
					Where("workspace_id = ? AND resource_name = ? AND output_name = ? AND id != ?",
						workspace.WorkspaceID, models.StaticOutputResourceName, item.OutputName, output.ID).
					Count(&existingCount)

				if existingCount > 0 {
					errors = append(errors, fmt.Sprintf("更新 %s 失败: 已存在相同名称的静态Output", item.OutputID))
					continue
				}
				updates["output_name"] = item.OutputName
			}
			if item.OutputValue != "" {
				updates["output_value"] = item.OutputValue
			}
		} else {
			// 资源关联输出：只能更新output_name
			if item.OutputName != "" && item.OutputName != output.OutputName {
				// 查找资源以获取正确的 resource_id
				var resource models.WorkspaceResource
				if err := tx.Where("workspace_id = ? AND resource_name = ?",
					workspace.WorkspaceID, output.ResourceName).First(&resource).Error; err != nil {
					errors = append(errors, fmt.Sprintf("更新 %s 失败: 资源 %s 不存在", item.OutputID, output.ResourceName))
					continue
				}

				// Terraform module 名称是 resource_id 中的点号替换为下划线
				moduleName := strings.ReplaceAll(resource.ResourceID, ".", "_")

				updates["output_name"] = item.OutputName
				updates["output_value"] = fmt.Sprintf("module.%s.%s", moduleName, item.OutputName)
			}
		}

		if err := tx.Model(&output).Updates(updates).Error; err != nil {
			errors = append(errors, fmt.Sprintf("更新 %s 失败: %s", item.OutputID, err.Error()))
		} else {
			updated++
		}
	}

	// 3. 处理创建（资源关联输出）
	for _, item := range req.Create {
		if item.ResourceName == "" || item.OutputName == "" {
			errors = append(errors, "创建失败: resource_name和output_name不能为空")
			continue
		}

		// 检查是否已存在
		var existingCount int64
		tx.Model(&models.WorkspaceOutput{}).
			Where("workspace_id = ? AND resource_name = ? AND output_name = ?",
				workspace.WorkspaceID, item.ResourceName, item.OutputName).
			Count(&existingCount)

		if existingCount > 0 {
			errors = append(errors, fmt.Sprintf("创建失败: %s.%s 已存在", item.ResourceName, item.OutputName))
			continue
		}

		// 查找资源以获取完整的 resource_id
		var resource models.WorkspaceResource
		if err := tx.Where("workspace_id = ? AND resource_name = ?",
			workspace.WorkspaceID, item.ResourceName).First(&resource).Error; err != nil {
			errors = append(errors, fmt.Sprintf("创建失败: 资源 %s 不存在", item.ResourceName))
			continue
		}

		// Terraform module 名称是 resource_id 中的点号替换为下划线
		moduleName := strings.ReplaceAll(resource.ResourceID, ".", "_")

		output := &models.WorkspaceOutput{
			WorkspaceID:  workspace.WorkspaceID,
			OutputID:     generateOutputID(),
			ResourceName: item.ResourceName,
			OutputName:   item.OutputName,
			OutputValue:  fmt.Sprintf("module.%s.%s", moduleName, item.OutputName),
			Description:  item.Description,
			Sensitive:    item.Sensitive,
			CreatedBy:    &uid,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}

		if err := tx.Create(output).Error; err != nil {
			errors = append(errors, fmt.Sprintf("创建 %s.%s 失败: %s", item.ResourceName, item.OutputName, err.Error()))
		} else {
			created++
		}
	}

	// 4. 处理创建（静态输出）
	for _, item := range req.CreateStatic {
		if item.OutputName == "" {
			errors = append(errors, "创建静态输出失败: output_name不能为空")
			continue
		}
		if item.OutputValue == "" {
			errors = append(errors, fmt.Sprintf("创建静态输出 %s 失败: output_value不能为空", item.OutputName))
			continue
		}

		// 检查是否已存在相同名称的静态输出
		var existingCount int64
		tx.Model(&models.WorkspaceOutput{}).
			Where("workspace_id = ? AND resource_name = ? AND output_name = ?",
				workspace.WorkspaceID, models.StaticOutputResourceName, item.OutputName).
			Count(&existingCount)

		if existingCount > 0 {
			errors = append(errors, fmt.Sprintf("创建静态输出失败: %s 已存在", item.OutputName))
			continue
		}

		output := &models.WorkspaceOutput{
			WorkspaceID:  workspace.WorkspaceID,
			OutputID:     generateOutputID(),
			ResourceName: models.StaticOutputResourceName,
			OutputName:   item.OutputName,
			OutputValue:  item.OutputValue,
			Description:  item.Description,
			Sensitive:    item.Sensitive,
			CreatedBy:    &uid,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}

		if err := tx.Create(output).Error; err != nil {
			errors = append(errors, fmt.Sprintf("创建静态输出 %s 失败: %s", item.OutputName, err.Error()))
		} else {
			created++
		}
	}

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "保存失败: " + err.Error(),
		})
		return
	}

	// 查询更新后的outputs列表
	var outputs []models.WorkspaceOutput
	c.db.Where("workspace_id = ?", workspace.WorkspaceID).
		Order("resource_name ASC, output_name ASC").
		Find(&outputs)

	ctx.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": fmt.Sprintf("操作完成: 创建 %d, 更新 %d, 删除 %d", created, updated, deleted),
		"summary": gin.H{
			"created": created,
			"updated": updated,
			"deleted": deleted,
		},
		"errors":  errors,
		"outputs": outputs,
	})
}
