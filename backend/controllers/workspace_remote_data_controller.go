package controllers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"sort"
	"time"

	"iac-platform/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// WorkspaceRemoteDataController 工作空间远程数据控制器
type WorkspaceRemoteDataController struct {
	db *gorm.DB
}

// NewWorkspaceRemoteDataController 创建控制器实例
func NewWorkspaceRemoteDataController(db *gorm.DB) *WorkspaceRemoteDataController {
	return &WorkspaceRemoteDataController{db: db}
}

// generateRemoteDataID 生成远程数据语义化ID
func generateRemoteDataID() string {
	return fmt.Sprintf("rd-%s", uuid.New().String()[:12])
}

// generateTokenID 生成Token语义化ID
func generateTokenID() string {
	return fmt.Sprintf("rdt-%s", uuid.New().String()[:12])
}

// generateSecureToken 生成安全的随机token
func generateSecureToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// ListRemoteData 获取workspace的远程数据引用列表
// @Summary 获取远程数据引用列表
// @Description 获取工作空间的所有远程数据引用配置
// @Tags Workspace Remote Data
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Success 200 {object} map[string]interface{} "成功返回远程数据列表"
// @Failure 400 {object} map[string]interface{} "无效的工作空间ID"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/workspaces/{id}/remote-data [get]
// @Security Bearer
func (c *WorkspaceRemoteDataController) ListRemoteData(ctx *gin.Context) {
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
		ctx.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "Workspace不存在",
		})
		return
	}

	// 查询远程数据列表
	var remoteDataList []models.WorkspaceRemoteData
	if err := c.db.Where("workspace_id = ?", workspace.WorkspaceID).
		Preload("SourceWorkspace").
		Order("created_at DESC").
		Find(&remoteDataList).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取远程数据列表失败",
		})
		return
	}

	// 构建响应，包含源workspace的outputs信息
	var result []models.RemoteDataInfo
	for _, rd := range remoteDataList {
		info := models.RemoteDataInfo{
			RemoteDataID:      rd.RemoteDataID,
			WorkspaceID:       rd.WorkspaceID,
			SourceWorkspaceID: rd.SourceWorkspaceID,
			DataName:          rd.DataName,
			Description:       rd.Description,
		}
		if rd.SourceWorkspace != nil {
			info.SourceWorkspaceName = rd.SourceWorkspace.Name
		}
		result = append(result, info)
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code":        200,
		"remote_data": result,
		"total":       len(result),
	})
}

// CreateRemoteData 创建远程数据引用
// @Summary 创建远程数据引用
// @Description 为工作空间创建远程数据引用配置
// @Tags Workspace Remote Data
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param request body object true "远程数据配置"
// @Success 201 {object} map[string]interface{} "创建成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 403 {object} map[string]interface{} "无权访问源workspace"
// @Failure 500 {object} map[string]interface{} "创建失败"
// @Router /api/v1/workspaces/{id}/remote-data [post]
// @Security Bearer
func (c *WorkspaceRemoteDataController) CreateRemoteData(ctx *gin.Context) {
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
		ctx.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "Workspace不存在",
		})
		return
	}

	userID, _ := ctx.Get("user_id")
	uid := userID.(string)

	var req struct {
		SourceWorkspaceID string `json:"source_workspace_id" binding:"required"`
		DataName          string `json:"data_name" binding:"required"`
		Description       string `json:"description"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	// 检查源workspace是否存在
	var sourceWorkspace models.Workspace
	if err := c.db.Where("workspace_id = ?", req.SourceWorkspaceID).First(&sourceWorkspace).Error; err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "源Workspace不存在",
		})
		return
	}

	// 检查是否有权限访问源workspace的outputs
	if !c.canAccessOutputs(workspace.WorkspaceID, sourceWorkspace.WorkspaceID) {
		ctx.JSON(http.StatusForbidden, gin.H{
			"code":    403,
			"message": "无权访问源Workspace的outputs，请联系源Workspace管理员开启共享",
		})
		return
	}

	// 检查是否已存在相同的远程数据引用
	var existingCount int64
	c.db.Model(&models.WorkspaceRemoteData{}).
		Where("workspace_id = ? AND source_workspace_id = ? AND data_name = ?",
			workspace.WorkspaceID, req.SourceWorkspaceID, req.DataName).
		Count(&existingCount)

	if existingCount > 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "已存在相同的远程数据引用",
		})
		return
	}

	remoteData := &models.WorkspaceRemoteData{
		WorkspaceID:       workspace.WorkspaceID,
		RemoteDataID:      generateRemoteDataID(),
		SourceWorkspaceID: req.SourceWorkspaceID,
		DataName:          req.DataName,
		Description:       req.Description,
		CreatedBy:         &uid,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if err := c.db.Create(remoteData).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "创建远程数据引用失败: " + err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"code":        201,
		"message":     "远程数据引用创建成功",
		"remote_data": remoteData,
	})
}

// UpdateRemoteData 更新远程数据引用
// @Summary 更新远程数据引用
// @Description 更新远程数据引用的名称和描述
// @Tags Workspace Remote Data
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param remote_data_id path string true "远程数据ID"
// @Param request body object true "更新内容"
// @Success 200 {object} map[string]interface{} "更新成功"
// @Failure 400 {object} map[string]interface{} "无效的参数"
// @Failure 404 {object} map[string]interface{} "远程数据不存在"
// @Failure 500 {object} map[string]interface{} "更新失败"
// @Router /api/v1/workspaces/{id}/remote-data/{remote_data_id} [put]
// @Security Bearer
func (c *WorkspaceRemoteDataController) UpdateRemoteData(ctx *gin.Context) {
	workspaceIDParam := ctx.Param("id")
	remoteDataIDParam := ctx.Param("remote_data_id")

	if workspaceIDParam == "" || remoteDataIDParam == "" {
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
		ctx.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "Workspace不存在",
		})
		return
	}

	var req struct {
		DataName    string `json:"data_name"`
		Description string `json:"description"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	// 查找远程数据引用
	var remoteData models.WorkspaceRemoteData
	if err := c.db.Where("workspace_id = ? AND remote_data_id = ?",
		workspace.WorkspaceID, remoteDataIDParam).First(&remoteData).Error; err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "远程数据引用不存在",
		})
		return
	}

	// 更新字段
	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}
	if req.DataName != "" {
		updates["data_name"] = req.DataName
	}
	updates["description"] = req.Description

	if err := c.db.Model(&remoteData).Updates(updates).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "更新远程数据引用失败: " + err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code":        200,
		"message":     "远程数据引用更新成功",
		"remote_data": remoteData,
	})
}

// DeleteRemoteData 删除远程数据引用
// @Summary 删除远程数据引用
// @Description 删除远程数据引用配置
// @Tags Workspace Remote Data
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param remote_data_id path string true "远程数据ID"
// @Success 200 {object} map[string]interface{} "删除成功"
// @Failure 400 {object} map[string]interface{} "无效的参数"
// @Failure 404 {object} map[string]interface{} "远程数据不存在"
// @Failure 500 {object} map[string]interface{} "删除失败"
// @Router /api/v1/workspaces/{id}/remote-data/{remote_data_id} [delete]
// @Security Bearer
func (c *WorkspaceRemoteDataController) DeleteRemoteData(ctx *gin.Context) {
	workspaceIDParam := ctx.Param("id")
	remoteDataIDParam := ctx.Param("remote_data_id")

	if workspaceIDParam == "" || remoteDataIDParam == "" {
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
		ctx.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "Workspace不存在",
		})
		return
	}

	// 删除远程数据引用
	result := c.db.Where("workspace_id = ? AND remote_data_id = ?",
		workspace.WorkspaceID, remoteDataIDParam).Delete(&models.WorkspaceRemoteData{})

	if result.Error != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "删除远程数据引用失败: " + result.Error.Error(),
		})
		return
	}

	if result.RowsAffected == 0 {
		ctx.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "远程数据引用不存在",
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "远程数据引用删除成功",
	})
}

// GetSourceWorkspaceOutputs 获取源workspace的outputs列表（用于前端自动提示）
// @Summary 获取源workspace的outputs
// @Description 获取指定workspace的outputs列表，用于配置远程数据引用时的自动提示。同时返回已配置的outputs和state中的outputs，并标记状态。
// @Tags Workspace Remote Data
// @Accept json
// @Produce json
// @Param id path string true "当前工作空间ID"
// @Param source_workspace_id query string true "源工作空间ID"
// @Success 200 {object} map[string]interface{} "成功返回outputs列表"
// @Failure 400 {object} map[string]interface{} "无效的参数"
// @Failure 403 {object} map[string]interface{} "无权访问"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/workspaces/{id}/remote-data/source-outputs [get]
// @Security Bearer
func (c *WorkspaceRemoteDataController) GetSourceWorkspaceOutputs(ctx *gin.Context) {
	workspaceIDParam := ctx.Param("id")
	sourceWorkspaceID := ctx.Query("source_workspace_id")

	if workspaceIDParam == "" || sourceWorkspaceID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的参数",
		})
		return
	}

	// 获取当前workspace
	var workspace models.Workspace
	err := c.db.Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "Workspace不存在",
		})
		return
	}

	// 检查源workspace是否存在
	var sourceWorkspace models.Workspace
	if err := c.db.Where("workspace_id = ?", sourceWorkspaceID).First(&sourceWorkspace).Error; err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "源Workspace不存在",
		})
		return
	}

	// 检查是否有权限访问源workspace的outputs
	if !c.canAccessOutputs(workspace.WorkspaceID, sourceWorkspace.WorkspaceID) {
		ctx.JSON(http.StatusForbidden, gin.H{
			"code":    403,
			"message": "无权访问源Workspace的outputs",
		})
		return
	}

	// 1. 获取源workspace配置的outputs（从workspace_outputs表）
	var configuredOutputs []models.WorkspaceOutput
	c.db.Where("workspace_id = ?", sourceWorkspace.WorkspaceID).Find(&configuredOutputs)

	// 2. 获取源workspace的state outputs（从workspace_state_versions表）
	stateOutputsMap := make(map[string]map[string]interface{}) // key -> output info
	var stateVersion models.WorkspaceStateVersion
	if err := c.db.Where("workspace_id = ?", sourceWorkspace.WorkspaceID).
		Order("version DESC").
		First(&stateVersion).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			log.Printf("Warning: failed to get state version for workspace %s: %v", sourceWorkspaceID, err)
		}
	} else if stateVersion.Content != nil {
		if outputs, ok := stateVersion.Content["outputs"].(map[string]interface{}); ok {
			for key, val := range outputs {
				if outputMap, ok := val.(map[string]interface{}); ok {
					// 检查value是否为null
					value, hasValue := outputMap["value"]
					if hasValue && value != nil {
						stateOutputsMap[key] = outputMap
					}
				}
			}
		}
	}

	// 3. 合并结果，标记状态
	// OutputKeyInfoWithStatus 扩展 OutputKeyInfo，增加状态字段
	type OutputKeyInfoWithStatus struct {
		Key         string      `json:"key"`
		Type        string      `json:"type,omitempty"`
		Sensitive   bool        `json:"sensitive,omitempty"`
		Value       interface{} `json:"value,omitempty"`
		Status      string      `json:"status"` // "available" = 已apply有值, "pending" = 已配置但未apply
		Description string      `json:"description,omitempty"`
	}

	outputsMap := make(map[string]*OutputKeyInfoWithStatus)
	pendingCount := 0
	availableCount := 0

	// 先处理配置的outputs
	for _, configOutput := range configuredOutputs {
		outputName := configOutput.OutputName

		// 对于静态输出（resource_name = "__static__"），state中的key是 "static-{output_name}"
		// 对于资源关联输出，state中的key是 "{resource_name}-{output_name}"
		var stateKey string
		if configOutput.ResourceName == "__static__" {
			stateKey = "static-" + outputName
		} else {
			stateKey = configOutput.ResourceName + "-" + outputName
		}

		outputInfo := &OutputKeyInfoWithStatus{
			Key:         stateKey, // 使用state中的key作为显示key，这样用户引用时使用正确的名称
			Sensitive:   configOutput.Sensitive,
			Description: configOutput.Description,
			Status:      "pending", // 默认为pending
		}

		// 检查是否在state中存在
		if stateOutput, exists := stateOutputsMap[stateKey]; exists {
			outputInfo.Status = "available"
			availableCount++

			// 获取类型信息
			if t, ok := stateOutput["type"].(string); ok {
				outputInfo.Type = t
			}

			// 非sensitive的output返回value
			if !outputInfo.Sensitive {
				outputInfo.Value = stateOutput["value"]
			}

			// 标记这个state output已经被配置匹配
			delete(stateOutputsMap, stateKey)
		} else {
			pendingCount++
		}

		outputsMap[stateKey] = outputInfo
	}

	// 再处理state中存在但未在配置中的outputs（可能是手动在tf文件中定义的）
	for key, stateOutput := range stateOutputsMap {
		if _, exists := outputsMap[key]; !exists {
			outputInfo := &OutputKeyInfoWithStatus{
				Key:    key,
				Status: "available",
			}
			availableCount++

			// 检查是否为sensitive
			if s, ok := stateOutput["sensitive"].(bool); ok {
				outputInfo.Sensitive = s
			}

			// 获取类型信息
			if t, ok := stateOutput["type"].(string); ok {
				outputInfo.Type = t
			}

			// 非sensitive的output返回value
			if !outputInfo.Sensitive {
				outputInfo.Value = stateOutput["value"]
			}

			outputsMap[key] = outputInfo
		}
	}

	// 转换为数组并按key排序
	var outputKeys []OutputKeyInfoWithStatus
	for _, v := range outputsMap {
		outputKeys = append(outputKeys, *v)
	}
	sort.Slice(outputKeys, func(i, j int) bool {
		return outputKeys[i].Key < outputKeys[j].Key
	})

	// 构建响应消息
	var message string
	if pendingCount > 0 && availableCount == 0 {
		message = fmt.Sprintf("该 workspace 有 %d 个 output 尚未 apply，引用后需要源 workspace 执行 apply 才能获取实际值", pendingCount)
	} else if pendingCount > 0 {
		message = fmt.Sprintf("该 workspace 有 %d 个 output 尚未 apply", pendingCount)
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code":                  200,
		"source_workspace_id":   sourceWorkspaceID,
		"source_workspace_name": sourceWorkspace.Name,
		"outputs":               outputKeys,
		"has_pending_outputs":   pendingCount > 0,
		"pending_count":         pendingCount,
		"available_count":       availableCount,
		"message":               message,
	})
}

// GetAccessibleWorkspaces 获取可访问的workspace列表
// @Summary 获取可访问的workspace列表
// @Description 获取当前workspace可以引用outputs的workspace列表
// @Tags Workspace Remote Data
// @Accept json
// @Produce json
// @Param id path string true "当前工作空间ID"
// @Success 200 {object} map[string]interface{} "成功返回可访问的workspace列表"
// @Failure 400 {object} map[string]interface{} "无效的参数"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/workspaces/{id}/remote-data/accessible-workspaces [get]
// @Security Bearer
func (c *WorkspaceRemoteDataController) GetAccessibleWorkspaces(ctx *gin.Context) {
	workspaceIDParam := ctx.Param("id")

	if workspaceIDParam == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的workspace ID",
		})
		return
	}

	// 获取当前workspace
	var workspace models.Workspace
	err := c.db.Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "Workspace不存在",
		})
		return
	}

	// 查询所有允许访问的workspace
	// 1. outputs_sharing = 'all' 的workspace
	// 2. outputs_sharing = 'specific' 且在 workspace_outputs_access 中允许当前workspace访问的
	var accessibleWorkspaces []models.Workspace

	// 查询 outputs_sharing = 'all' 的workspace（排除自己）
	c.db.Where("outputs_sharing = ? AND workspace_id != ?", "all", workspace.WorkspaceID).
		Find(&accessibleWorkspaces)

	// 查询 outputs_sharing = 'specific' 且允许当前workspace访问的
	var specificWorkspaceIDs []string
	c.db.Model(&models.WorkspaceOutputsAccess{}).
		Where("allowed_workspace_id = ?", workspace.WorkspaceID).
		Pluck("workspace_id", &specificWorkspaceIDs)

	if len(specificWorkspaceIDs) > 0 {
		var specificWorkspaces []models.Workspace
		c.db.Where("workspace_id IN ? AND outputs_sharing = ?", specificWorkspaceIDs, "specific").
			Find(&specificWorkspaces)
		accessibleWorkspaces = append(accessibleWorkspaces, specificWorkspaces...)
	}

	// 构建响应
	type WorkspaceInfo struct {
		WorkspaceID string `json:"workspace_id"`
		Name        string `json:"name"`
	}

	var result []WorkspaceInfo
	for _, ws := range accessibleWorkspaces {
		result = append(result, WorkspaceInfo{
			WorkspaceID: ws.WorkspaceID,
			Name:        ws.Name,
		})
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code":       200,
		"workspaces": result,
		"total":      len(result),
	})
}

// UpdateOutputsSharing 更新workspace的outputs共享设置
// @Summary 更新outputs共享设置
// @Description 更新workspace的outputs共享模式
// @Tags Workspace Remote Data
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param request body object true "共享设置"
// @Success 200 {object} map[string]interface{} "更新成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 500 {object} map[string]interface{} "更新失败"
// @Router /api/v1/workspaces/{id}/outputs-sharing [put]
// @Security Bearer
func (c *WorkspaceRemoteDataController) UpdateOutputsSharing(ctx *gin.Context) {
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
		ctx.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "Workspace不存在",
		})
		return
	}

	var req struct {
		SharingMode         string   `json:"sharing_mode" binding:"required"` // none, all, specific
		AllowedWorkspaceIDs []string `json:"allowed_workspace_ids"`           // 当 sharing_mode = specific 时使用
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	// 验证sharing_mode
	if req.SharingMode != "none" && req.SharingMode != "all" && req.SharingMode != "specific" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的sharing_mode，必须是 none, all 或 specific",
		})
		return
	}

	userID, _ := ctx.Get("user_id")
	uid := userID.(string)

	// 使用事务更新
	err = c.db.Transaction(func(tx *gorm.DB) error {
		// 更新workspace的outputs_sharing字段
		if err := tx.Model(&workspace).Update("outputs_sharing", req.SharingMode).Error; err != nil {
			return err
		}

		// 如果是specific模式，更新允许访问的workspace列表
		if req.SharingMode == "specific" {
			// 删除现有的访问权限
			if err := tx.Where("workspace_id = ?", workspace.WorkspaceID).
				Delete(&models.WorkspaceOutputsAccess{}).Error; err != nil {
				return err
			}

			// 添加新的访问权限
			for _, allowedID := range req.AllowedWorkspaceIDs {
				// 验证workspace是否存在
				var count int64
				tx.Model(&models.Workspace{}).Where("workspace_id = ?", allowedID).Count(&count)
				if count == 0 {
					continue
				}

				access := &models.WorkspaceOutputsAccess{
					WorkspaceID:        workspace.WorkspaceID,
					AllowedWorkspaceID: allowedID,
					CreatedBy:          &uid,
					CreatedAt:          time.Now(),
				}
				if err := tx.Create(access).Error; err != nil {
					return err
				}
			}
		} else {
			// 如果不是specific模式，清空访问权限列表
			if err := tx.Where("workspace_id = ?", workspace.WorkspaceID).
				Delete(&models.WorkspaceOutputsAccess{}).Error; err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "更新共享设置失败: " + err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "共享设置更新成功",
	})
}

// GetOutputsSharing 获取workspace的outputs共享设置
// @Summary 获取outputs共享设置
// @Description 获取workspace的outputs共享模式和允许访问的workspace列表
// @Tags Workspace Remote Data
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Success 200 {object} map[string]interface{} "成功返回共享设置"
// @Failure 400 {object} map[string]interface{} "无效的参数"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/workspaces/{id}/outputs-sharing [get]
// @Security Bearer
func (c *WorkspaceRemoteDataController) GetOutputsSharing(ctx *gin.Context) {
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
		ctx.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "Workspace不存在",
		})
		return
	}

	// 获取允许访问的workspace列表
	var allowedAccess []models.WorkspaceOutputsAccess
	c.db.Where("workspace_id = ?", workspace.WorkspaceID).
		Preload("AllowedWorkspace").
		Find(&allowedAccess)

	type AllowedWorkspaceInfo struct {
		WorkspaceID string `json:"workspace_id"`
		Name        string `json:"name"`
	}

	var allowedWorkspaces []AllowedWorkspaceInfo
	for _, access := range allowedAccess {
		if access.AllowedWorkspace != nil {
			allowedWorkspaces = append(allowedWorkspaces, AllowedWorkspaceInfo{
				WorkspaceID: access.AllowedWorkspaceID,
				Name:        access.AllowedWorkspace.Name,
			})
		}
	}

	// 获取outputs_sharing字段值，如果为空则默认为none
	sharingMode := workspace.OutputsSharing
	if sharingMode == "" {
		sharingMode = "none"
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code":               200,
		"sharing_mode":       sharingMode,
		"allowed_workspaces": allowedWorkspaces,
	})
}

// canAccessOutputs 检查是否有权限访问目标workspace的outputs
func (c *WorkspaceRemoteDataController) canAccessOutputs(requesterWorkspaceID, targetWorkspaceID string) bool {
	var targetWorkspace models.Workspace
	if err := c.db.Where("workspace_id = ?", targetWorkspaceID).First(&targetWorkspace).Error; err != nil {
		return false
	}

	// 检查共享模式
	switch targetWorkspace.OutputsSharing {
	case "all":
		return true
	case "specific":
		// 检查是否在允许列表中
		var count int64
		c.db.Model(&models.WorkspaceOutputsAccess{}).
			Where("workspace_id = ? AND allowed_workspace_id = ?", targetWorkspaceID, requesterWorkspaceID).
			Count(&count)
		return count > 0
	default:
		return false
	}
}

// GenerateRemoteDataToken 生成远程数据访问token（内部方法，供terraform执行时调用）
func (c *WorkspaceRemoteDataController) GenerateRemoteDataToken(
	requesterWorkspaceID string,
	targetWorkspaceID string,
	taskID *uint,
) (*models.RemoteDataToken, error) {
	// 检查是否有权限访问
	if !c.canAccessOutputs(requesterWorkspaceID, targetWorkspaceID) {
		return nil, fmt.Errorf("no permission to access outputs of workspace %s", targetWorkspaceID)
	}

	// 生成token
	tokenValue, err := generateSecureToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	token := &models.RemoteDataToken{
		TokenID:              generateTokenID(),
		Token:                tokenValue,
		WorkspaceID:          targetWorkspaceID,
		RequesterWorkspaceID: requesterWorkspaceID,
		TaskID:               taskID,
		MaxUses:              5,
		UsedCount:            0,
		ExpiresAt:            time.Now().Add(30 * time.Minute),
		CreatedAt:            time.Now(),
	}

	if err := c.db.Create(token).Error; err != nil {
		return nil, fmt.Errorf("failed to create token: %w", err)
	}

	return token, nil
}

// ValidateAndUseToken 验证并使用token（供state-outputs API调用）
func (c *WorkspaceRemoteDataController) ValidateAndUseToken(tokenValue string) (*models.RemoteDataToken, error) {
	var token models.RemoteDataToken
	if err := c.db.Where("token = ?", tokenValue).First(&token).Error; err != nil {
		return nil, fmt.Errorf("invalid token")
	}

	// 检查token是否有效
	if !token.IsValid() {
		return nil, fmt.Errorf("token expired or exceeded max uses")
	}

	// 更新使用次数
	now := time.Now()
	c.db.Model(&token).Updates(map[string]interface{}{
		"used_count":   token.UsedCount + 1,
		"last_used_at": now,
	})

	return &token, nil
}
