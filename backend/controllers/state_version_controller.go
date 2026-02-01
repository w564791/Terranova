package controllers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"iac-platform/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// StateVersionController State版本控制器
type StateVersionController struct {
	db *gorm.DB
}

// NewStateVersionController 创建控制器实例
func NewStateVersionController(db *gorm.DB) *StateVersionController {
	return &StateVersionController{
		db: db,
	}
}

// GetStateVersions 获取workspace的state版本列表
// @Summary 获取State版本列表
// @Description 获取工作空间的所有State版本
// @Tags State Version
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Success 200 {object} map[string]interface{} "成功返回版本列表"
// @Failure 400 {object} map[string]interface{} "无效的工作空间ID"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/workspaces/{id}/state-versions [get]
// @Security Bearer
func (svc *StateVersionController) GetStateVersions(c *gin.Context) {
	workspaceIDParam := c.Param("id")
	if workspaceIDParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "无效的workspace ID",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// 获取workspace
	var workspace models.Workspace
	err := svc.db.Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := svc.db.Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"code":      404,
				"message":   "Workspace不存在",
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return
		}
	}

	// 查询版本列表，包含content字段以提取详细信息
	var versions []models.WorkspaceStateVersion
	if err := svc.db.Where("workspace_id = ?", workspace.WorkspaceID).
		Order("version DESC").
		Find(&versions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "获取版本列表失败",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// 获取总数
	var count int64
	svc.db.Model(&models.WorkspaceStateVersion{}).
		Where("workspace_id = ?", workspace.WorkspaceID).
		Count(&count)

	// 收集所有用户 ID，用于批量查询用户名
	userIDs := make([]string, 0)
	for _, v := range versions {
		if v.CreatedBy != nil && *v.CreatedBy != "" {
			userIDs = append(userIDs, *v.CreatedBy)
		}
	}

	// 批量查询用户名
	userNameMap := make(map[string]string)
	if len(userIDs) > 0 {
		var users []models.User
		svc.db.Where("user_id IN ?", userIDs).Select("user_id", "username").Find(&users)
		for _, u := range users {
			userNameMap[u.ID] = u.Username
		}
	}

	// 格式化版本列表，提取content中的详细信息
	type VersionResponse struct {
		ID                  uint    `json:"id"`
		WorkspaceID         string  `json:"workspace_id"` // 已改为 string
		Version             string  `json:"version"`
		Serial              int     `json:"serial"`
		TerraformVersion    string  `json:"terraform_version"`
		ResourcesCount      int     `json:"resources_count"`
		Checksum            string  `json:"checksum"`
		SizeBytes           int     `json:"size_bytes"`
		TaskID              *uint   `json:"task_id"`
		CreatedAt           string  `json:"created_at"`
		CreatedBy           *string `json:"created_by"`
		CreatedByName       string  `json:"created_by_name"` // 新增：用户名
		IsCurrent           bool    `json:"is_current"`
		IsImported          bool    `json:"is_imported"`
		ImportSource        string  `json:"import_source"`
		IsRollback          bool    `json:"is_rollback"`
		RollbackFromVersion *uint   `json:"rollback_from_version"`
		Description         string  `json:"description"`
	}

	var formattedVersions []VersionResponse
	for i, v := range versions {
		var terraformVersion string
		var resourcesCount int
		var serial int

		if v.Content != nil {
			if tfVer, ok := v.Content["terraform_version"].(string); ok {
				terraformVersion = tfVer
			}
			if resources, ok := v.Content["resources"].([]interface{}); ok {
				resourcesCount = len(resources)
			}
			if ser, ok := v.Content["serial"].(float64); ok {
				serial = int(ser)
			}
		}

		// 获取用户名
		createdByName := "System"
		if v.CreatedBy != nil && *v.CreatedBy != "" {
			if name, ok := userNameMap[*v.CreatedBy]; ok && name != "" {
				createdByName = name
			} else {
				// 如果找不到用户名，显示用户 ID 的前 8 位
				if len(*v.CreatedBy) > 8 {
					createdByName = (*v.CreatedBy)[:8] + "..."
				} else {
					createdByName = *v.CreatedBy
				}
			}
		}

		formattedVersions = append(formattedVersions, VersionResponse{
			ID:                  v.ID,
			WorkspaceID:         v.WorkspaceID,
			Version:             strconv.Itoa(v.Version),
			Serial:              serial,
			TerraformVersion:    terraformVersion,
			ResourcesCount:      resourcesCount,
			Checksum:            v.Checksum,
			SizeBytes:           int(v.SizeBytes),
			TaskID:              v.TaskID,
			CreatedAt:           v.CreatedAt.Format(time.RFC3339),
			CreatedBy:           v.CreatedBy,
			CreatedByName:       createdByName,
			IsCurrent:           i == 0, // 第一个（最新的）是当前版本
			IsImported:          v.IsImported,
			ImportSource:        v.ImportSource,
			IsRollback:          v.IsRollback,
			RollbackFromVersion: v.RollbackFromVersion,
			Description:         v.Description,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"items":     formattedVersions,
		"total":     count,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// GetStateVersionMetadata 获取指定版本的state元数据
// @Summary 获取State版本元数据
// @Description 获取指定版本的State元数据信息
// @Tags State Version
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param version path int true "版本号"
// @Success 200 {object} map[string]interface{} "成功返回元数据"
// @Failure 400 {object} map[string]interface{} "无效的参数"
// @Failure 404 {object} map[string]interface{} "版本不存在"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/workspaces/{id}/state-versions/{version}/metadata [get]
// @Security Bearer
func (svc *StateVersionController) GetStateVersionMetadata(c *gin.Context) {
	workspaceIDParam := c.Param("id")
	if workspaceIDParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "无效的workspace ID",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	version, err := strconv.Atoi(c.Param("version"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "无效的版本号",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// 获取workspace
	var workspace models.Workspace
	err = svc.db.Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := svc.db.Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"code":      404,
				"message":   "Workspace不存在",
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return
		}
	}

	// 查询版本元数据
	var stateVersion models.WorkspaceStateVersion
	if err := svc.db.Where("workspace_id = ? AND version = ?", workspace.WorkspaceID, version).
		First(&stateVersion).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"code":      404,
				"message":   "版本不存在",
				"timestamp": time.Now().Format(time.RFC3339),
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":      500,
				"message":   "获取版本失败",
				"timestamp": time.Now().Format(time.RFC3339),
			})
		}
		return
	}

	// 从content中提取详细信息
	var terraformVersion string
	var resourcesCount int
	var serial int

	if stateVersion.Content != nil {
		if tfVer, ok := stateVersion.Content["terraform_version"].(string); ok {
			terraformVersion = tfVer
		}
		if resources, ok := stateVersion.Content["resources"].([]interface{}); ok {
			resourcesCount = len(resources)
		}
		if ser, ok := stateVersion.Content["serial"].(float64); ok {
			serial = int(ser)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"id":                    stateVersion.ID,
			"workspace_id":          stateVersion.WorkspaceID,
			"version":               strconv.Itoa(stateVersion.Version),
			"serial":                serial,
			"terraform_version":     terraformVersion,
			"resources_count":       resourcesCount,
			"checksum":              stateVersion.Checksum,
			"size_bytes":            stateVersion.SizeBytes,
			"task_id":               stateVersion.TaskID,
			"created_at":            stateVersion.CreatedAt,
			"created_by":            stateVersion.CreatedBy,
			"is_imported":           stateVersion.IsImported,
			"import_source":         stateVersion.ImportSource,
			"is_rollback":           stateVersion.IsRollback,
			"rollback_from_version": stateVersion.RollbackFromVersion,
			"description":           stateVersion.Description,
		},
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// GetStateVersion 获取指定版本的state内容
// @Summary 下载State版本文件
// @Description 下载指定版本的State文件内容
// @Tags State Version
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param version path int true "版本号"
// @Success 200 {file} file "State文件"
// @Failure 400 {object} map[string]interface{} "无效的参数"
// @Failure 404 {object} map[string]interface{} "版本不存在"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/workspaces/{id}/state-versions/{version} [get]
// @Security Bearer
func (svc *StateVersionController) GetStateVersion(c *gin.Context) {
	workspaceIDParam := c.Param("id")
	if workspaceIDParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "无效的workspace ID",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	version, err := strconv.Atoi(c.Param("version"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "无效的版本号",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// 获取workspace
	var workspace models.Workspace
	err = svc.db.Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := svc.db.Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"code":      404,
				"message":   "Workspace不存在",
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return
		}
	}

	// 查询版本，明确选择content字段
	var stateVersion models.WorkspaceStateVersion
	if err := svc.db.Where("workspace_id = ? AND version = ?", workspace.WorkspaceID, version).
		Select("id", "workspace_id", "version", "content", "checksum", "size_bytes", "task_id", "created_at", "created_by").
		First(&stateVersion).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"code":      404,
				"message":   "版本不存在",
				"timestamp": time.Now().Format(time.RFC3339),
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":      500,
				"message":   "获取版本失败",
				"timestamp": time.Now().Format(time.RFC3339),
			})
		}
		return
	}

	// 返回文件内容用于下载
	c.Header("Content-Type", "application/json")
	c.Header("Content-Disposition", "attachment; filename=terraform-state-v"+strconv.Itoa(version)+".json")

	// 将content转换为JSON字节并直接写入响应
	if stateVersion.Content == nil {
		c.Data(http.StatusOK, "application/json", []byte("{}"))
	} else {
		// 使用json.Marshal将content转换为JSON字节
		jsonData, err := json.Marshal(stateVersion.Content)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":      500,
				"message":   "序列化state内容失败",
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return
		}
		c.Data(http.StatusOK, "application/json", jsonData)
	}
}

// GetCurrentState 获取workspace当前的state
// @Summary 获取当前State
// @Description 获取工作空间当前的State信息
// @Tags State Version
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Success 200 {object} map[string]interface{} "成功返回当前State"
// @Failure 400 {object} map[string]interface{} "无效的工作空间ID"
// @Failure 404 {object} map[string]interface{} "State不存在"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/workspaces/{id}/current-state [get]
// @Security Bearer
func (svc *StateVersionController) GetCurrentState(c *gin.Context) {
	workspaceIDParam := c.Param("id")
	if workspaceIDParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "无效的workspace ID",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// 获取workspace
	var workspace models.Workspace
	err := svc.db.Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := svc.db.Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"code":      404,
				"message":   "Workspace不存在",
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return
		}
	}

	// 查询最新的state version记录
	var stateVersion models.WorkspaceStateVersion
	if err := svc.db.Where("workspace_id = ?", workspace.WorkspaceID).
		Order("version DESC").
		First(&stateVersion).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"code":      404,
				"message":   "没有找到state版本",
				"timestamp": time.Now().Format(time.RFC3339),
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":      500,
				"message":   "获取state失败",
				"timestamp": time.Now().Format(time.RFC3339),
			})
		}
		return
	}

	// 从content中提取terraform_version和resources_count
	var terraformVersion string
	var resourcesCount int
	var serial int

	if stateVersion.Content != nil {
		if tfVer, ok := stateVersion.Content["terraform_version"].(string); ok {
			terraformVersion = tfVer
		}
		if resources, ok := stateVersion.Content["resources"].([]interface{}); ok {
			resourcesCount = len(resources)
		}
		if ser, ok := stateVersion.Content["serial"].(float64); ok {
			serial = int(ser)
		}
	}

	// 返回格式化的响应
	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"id":                stateVersion.ID,
			"version":           strconv.Itoa(stateVersion.Version),
			"serial":            serial,
			"terraform_version": terraformVersion,
			"resources_count":   resourcesCount,
			"created_at":        stateVersion.CreatedAt,
			"is_current":        true,
		},
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// RollbackState 回滚到指定版本
// @Summary 回滚State版本
// @Description 将工作空间State回滚到指定版本
// @Tags State Version
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param version path int true "目标版本号"
// @Success 200 {object} map[string]interface{} "回滚成功"
// @Failure 400 {object} map[string]interface{} "无效的参数或workspace已锁定"
// @Failure 404 {object} map[string]interface{} "版本不存在"
// @Failure 500 {object} map[string]interface{} "回滚失败"
// @Router /api/v1/workspaces/{id}/state-versions/{version}/rollback [post]
// @Security Bearer
func (svc *StateVersionController) RollbackState(c *gin.Context) {
	workspaceIDParam := c.Param("id")
	if workspaceIDParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "无效的workspace ID",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	version, err := strconv.Atoi(c.Param("version"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "无效的版本号",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// 获取workspace
	var workspace models.Workspace
	err = svc.db.Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := svc.db.Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"code":      404,
				"message":   "Workspace不存在",
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return
		}
	}

	if workspace.IsLocked {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "workspace已锁定，无法回滚",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// 查询目标版本
	var stateVersion models.WorkspaceStateVersion
	if err := svc.db.Where("workspace_id = ? AND version = ?", workspace.WorkspaceID, version).
		First(&stateVersion).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"code":      404,
				"message":   "版本不存在",
				"timestamp": time.Now().Format(time.RFC3339),
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":      500,
				"message":   "获取版本失败",
				"timestamp": time.Now().Format(time.RFC3339),
			})
		}
		return
	}

	// 回滚state
	if err := svc.db.Model(&models.Workspace{}).
		Where("workspace_id = ?", workspace.WorkspaceID).
		Update("tf_state", stateVersion.Content).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "回滚失败",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "回滚成功",
		"data": gin.H{
			"version":  version,
			"checksum": stateVersion.Checksum,
		},
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// CompareVersions 比较两个版本的差异
// @Summary 对比State版本
// @Description 对比两个State版本的差异
// @Tags State Version
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param from query int true "源版本号"
// @Param to query int true "目标版本号"
// @Success 200 {object} map[string]interface{} "成功返回对比结果"
// @Failure 400 {object} map[string]interface{} "无效的参数"
// @Failure 404 {object} map[string]interface{} "版本不存在"
// @Failure 500 {object} map[string]interface{} "对比失败"
// @Router /api/v1/workspaces/{id}/state-versions/compare [get]
// @Security Bearer
func (svc *StateVersionController) CompareVersions(c *gin.Context) {
	workspaceIDParam := c.Param("id")
	if workspaceIDParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "无效的workspace ID",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	fromVersion, err := strconv.Atoi(c.Query("from"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "无效的from版本号",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	toVersion, err := strconv.Atoi(c.Query("to"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "无效的to版本号",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// 获取workspace
	var workspace models.Workspace
	err = svc.db.Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := svc.db.Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"code":      404,
				"message":   "Workspace不存在",
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return
		}
	}

	// 查询两个版本
	var versions []models.WorkspaceStateVersion
	if err := svc.db.Where("workspace_id = ? AND version IN ?", workspace.WorkspaceID, []int{fromVersion, toVersion}).
		Find(&versions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "获取版本失败",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	if len(versions) != 2 {
		c.JSON(http.StatusNotFound, gin.H{
			"code":      404,
			"message":   "版本不存在",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// 简单返回两个版本的内容，前端可以做diff
	var fromState, toState models.JSONB
	for _, v := range versions {
		if v.Version == fromVersion {
			fromState = v.Content
		} else {
			toState = v.Content
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"from_version": fromVersion,
			"to_version":   toVersion,
			"from_state":   fromState,
			"to_state":     toState,
		},
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// DeleteStateVersion 删除指定版本（软删除，保留记录但清空内容）
// @Summary 删除State版本
// @Description 删除指定的State版本（软删除）
// @Tags State Version
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param version path int true "版本号"
// @Success 200 {object} map[string]interface{} "删除成功"
// @Failure 400 {object} map[string]interface{} "无效的参数或不能删除最新版本"
// @Failure 500 {object} map[string]interface{} "删除失败"
// @Router /api/v1/workspaces/{id}/state-versions/{version} [delete]
// @Security Bearer
func (svc *StateVersionController) DeleteStateVersion(c *gin.Context) {
	workspaceIDParam := c.Param("id")
	if workspaceIDParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "无效的workspace ID",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	version, err := strconv.Atoi(c.Param("version"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "无效的版本号",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// 获取workspace
	var workspace models.Workspace
	err = svc.db.Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := svc.db.Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"code":      404,
				"message":   "Workspace不存在",
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return
		}
	}

	// 获取最新版本号
	var maxVersion int
	svc.db.Model(&models.WorkspaceStateVersion{}).
		Where("workspace_id = ?", workspace.WorkspaceID).
		Select("COALESCE(MAX(version), 0)").
		Scan(&maxVersion)

	// 不允许删除最新版本
	if version == maxVersion {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "不能删除最新版本",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// 软删除：清空content但保留记录
	if err := svc.db.Model(&models.WorkspaceStateVersion{}).
		Where("workspace_id = ? AND version = ?", workspace.WorkspaceID, version).
		Update("content", nil).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "删除版本失败",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"message":   "版本已删除",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}
