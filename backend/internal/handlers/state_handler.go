package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"iac-platform/services"

	"github.com/gin-gonic/gin"
)

// StateHandler State API Handler
type StateHandler struct {
	stateService *services.StateService
}

// NewStateHandler 创建 State Handler
func NewStateHandler(stateService *services.StateService) *StateHandler {
	return &StateHandler{
		stateService: stateService,
	}
}

// UploadState uploads state data for a workspace
// @Summary Upload state
// @Description Upload state JSON data for a workspace
// @Tags State
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param request body object true "State upload request (state, force, description)"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/workspaces/{id}/state/upload [post]
// @Security BearerAuth
func (h *StateHandler) UploadState(c *gin.Context) {
	workspaceID := c.Param("id")
	userID := c.GetString("user_id")

	// 解析请求
	var req struct {
		State       map[string]interface{} `json:"state" binding:"required"`
		Force       bool                   `json:"force"`
		Description string                 `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request body",
			"details": err.Error(),
		})
		return
	}

	// 上传 State
	stateVersion, err := h.stateService.UploadState(
		req.State,
		workspaceID,
		userID,
		req.Force,
		req.Description,
	)

	if err != nil {
		// 判断错误类型
		if isValidationError(err) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":      err.Error(),
				"suggestion": "Use force=true to bypass validation",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to upload state",
				"details": err.Error(),
			})
		}
		return
	}

	// 返回成功响应
	warnings := []string{}
	if req.Force {
		warnings = append(warnings, "State uploaded with force=true, validation was bypassed")
		warnings = append(warnings, "Workspace is locked, please verify state before unlocking")
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "State uploaded successfully",
		"version":       stateVersion.Version,
		"warnings":      warnings,
		"state_version": stateVersion,
	})
}

// RollbackState rolls back state to a previous version
// @Summary Rollback state
// @Description Rollback workspace state to a specified target version
// @Tags State
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param request body object true "Rollback request (target_version, reason, force)"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/workspaces/{id}/state/rollback [post]
// @Security BearerAuth
func (h *StateHandler) RollbackState(c *gin.Context) {
	workspaceID := c.Param("id")
	userID := c.GetString("user_id")

	// 解析请求
	var req struct {
		TargetVersion int    `json:"target_version" binding:"required"`
		Reason        string `json:"reason"`
		Force         bool   `json:"force"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request body",
			"details": err.Error(),
		})
		return
	}

	// 回滚 State
	newVersion, err := h.stateService.RollbackState(
		workspaceID,
		req.TargetVersion,
		userID,
		req.Reason,
		req.Force,
	)

	if err != nil {
		// 判断错误类型
		if isValidationError(err) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":      err.Error(),
				"suggestion": "Use force=true to bypass validation",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to rollback state",
				"details": err.Error(),
			})
		}
		return
	}

	// 返回成功响应
	warnings := []string{}
	if req.Force {
		warnings = append(warnings, "State rolled back with force=true, validation was bypassed")
		warnings = append(warnings, "Workspace is locked, please verify state before unlocking")
	}

	c.JSON(http.StatusOK, gin.H{
		"message":               "Rollback successful",
		"new_version":           newVersion.Version,
		"rollback_from_version": req.TargetVersion,
		"description":           newVersion.Description,
		"warnings":              warnings,
	})
}

// GetStateVersions retrieves state version history with pagination
// @Summary Get state version history
// @Description Get paginated list of state versions for a workspace
// @Tags State
// @Produce json
// @Param id path string true "Workspace ID"
// @Param limit query int false "Limit (default: 50, max: 100)"
// @Param offset query int false "Offset (default: 0)"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/workspaces/{id}/state/versions [get]
// @Security BearerAuth
func (h *StateHandler) GetStateVersions(c *gin.Context) {
	workspaceID := c.Param("id")

	// 解析分页参数
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	if limit > 100 {
		limit = 100
	}

	// 获取版本列表（包含用户名）
	versions, total, err := h.stateService.ListStateVersionsWithUsernames(workspaceID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to get state versions",
			"details": err.Error(),
		})
		return
	}

	// 获取当前版本
	currentVersion := 0
	if len(versions) > 0 {
		currentVersion = versions[0].Version
	}

	c.JSON(http.StatusOK, gin.H{
		"versions":        versions,
		"total":           total,
		"current_version": currentVersion,
		"limit":           limit,
		"offset":          offset,
	})
}

// GetStateVersion retrieves metadata for a specific state version (no content)
// @Summary Get state version metadata
// @Description Get metadata for a specific state version (does not include content)
// @Tags State
// @Produce json
// @Param id path string true "Workspace ID"
// @Param version path int true "Version number"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/workspaces/{id}/state/versions/{version} [get]
// @Security BearerAuth
func (h *StateHandler) GetStateVersion(c *gin.Context) {
	workspaceID := c.Param("id")
	versionStr := c.Param("version")

	version, err := strconv.Atoi(versionStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid version number",
		})
		return
	}

	// 获取指定版本
	stateVersion, err := h.stateService.GetStateVersion(workspaceID, version)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "State version not found",
		})
		return
	}

	// 计算资源数和输出数
	resourceCount := h.countResources(stateVersion.Content)
	outputCount := h.countOutputs(stateVersion.Content)

	// 返回元数据（不含 content）
	c.JSON(http.StatusOK, gin.H{
		"id":             stateVersion.ID,
		"workspace_id":   stateVersion.WorkspaceID,
		"version":        stateVersion.Version,
		"checksum":       stateVersion.Checksum,
		"size_bytes":     stateVersion.SizeBytes,
		"task_id":        stateVersion.TaskID,
		"created_by":     stateVersion.CreatedBy,
		"created_at":     stateVersion.CreatedAt,
		"description":    stateVersion.Description,
		"resource_count": resourceCount,
		"output_count":   outputCount,
	})
}

// RetrieveStateVersion retrieves full state content for a specific version
// @Summary Retrieve state version content
// @Description Retrieve full state content including sensitive data (requires WORKSPACE_STATE_SENSITIVE permission)
// @Tags State
// @Produce json
// @Param id path string true "Workspace ID"
// @Param version path int true "Version number"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/workspaces/{id}/state/versions/{version}/retrieve [get]
// @Security BearerAuth
func (h *StateHandler) RetrieveStateVersion(c *gin.Context) {
	workspaceID := c.Param("id")
	versionStr := c.Param("version")
	userID := c.GetString("user_id")

	version, err := strconv.Atoi(versionStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid version number",
		})
		return
	}

	// 获取完整 state（含 content）
	stateVersion, err := h.stateService.GetStateVersion(workspaceID, version)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "State version not found",
		})
		return
	}

	// 记录审计日志
	h.logStateAccess(workspaceID, userID, version)

	// 返回完整内容
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"version": stateVersion.Version,
			"content": stateVersion.Content,
		},
		"audit": gin.H{
			"accessed_at": stateVersion.CreatedAt,
			"accessed_by": userID,
		},
	})
}

// DownloadStateVersion downloads a specific state version as a .tfstate file
// @Summary Download state version
// @Description Download a specific state version as a JSON file
// @Tags State
// @Produce application/json
// @Param id path string true "Workspace ID"
// @Param version path int true "Version number"
// @Success 200 {file} file "State file"
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/workspaces/{id}/state/versions/{version}/download [get]
// @Security BearerAuth
func (h *StateHandler) DownloadStateVersion(c *gin.Context) {
	workspaceID := c.Param("id")
	versionStr := c.Param("version")

	version, err := strconv.Atoi(versionStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid version number",
		})
		return
	}

	// 获取指定版本
	stateVersion, err := h.stateService.GetStateVersion(workspaceID, version)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "State version not found",
		})
		return
	}

	// 转换为 JSON
	stateJSON, err := json.MarshalIndent(stateVersion.Content, "", "  ")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to marshal state",
		})
		return
	}

	// 设置下载响应头
	filename := workspaceID + "-v" + versionStr + ".tfstate"
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Header("Content-Type", "application/json")
	c.Header("Content-Length", strconv.Itoa(len(stateJSON)))

	// 返回文件内容
	c.Data(http.StatusOK, "application/json", stateJSON)
}

// UploadStateFile uploads a state file via multipart form
// @Summary Upload state file
// @Description Upload a state file via multipart/form-data
// @Tags State
// @Accept multipart/form-data
// @Produce json
// @Param id path string true "Workspace ID"
// @Param file formData file true "State file (.tfstate)"
// @Param force formData string false "Force upload (true/false)"
// @Param description formData string false "Upload description"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/workspaces/{id}/state/upload-file [post]
// @Security BearerAuth
func (h *StateHandler) UploadStateFile(c *gin.Context) {
	workspaceID := c.Param("id")
	userID := c.GetString("user_id")

	// 获取表单参数
	force := c.PostForm("force") == "true"
	description := c.PostForm("description")

	// 获取上传的文件
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "No file uploaded",
			"details": err.Error(),
		})
		return
	}

	// 打开文件
	fileContent, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to open file",
			"details": err.Error(),
		})
		return
	}
	defer fileContent.Close()

	// 读取文件内容
	fileBytes, err := io.ReadAll(fileContent)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to read file",
			"details": err.Error(),
		})
		return
	}

	// 解析 JSON
	var stateContent map[string]interface{}
	if err := json.Unmarshal(fileBytes, &stateContent); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid state file format",
			"details": err.Error(),
		})
		return
	}

	// 上传 State
	stateVersion, err := h.stateService.UploadState(
		stateContent,
		workspaceID,
		userID,
		force,
		description,
	)

	if err != nil {
		if isValidationError(err) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":      err.Error(),
				"suggestion": "Use force=true to bypass validation",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to upload state",
				"details": err.Error(),
			})
		}
		return
	}

	// 返回成功响应
	warnings := []string{}
	if force {
		warnings = append(warnings, "State uploaded with force=true, validation was bypassed")
		warnings = append(warnings, "Workspace is locked, please verify state before unlocking")
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "State uploaded successfully",
		"version":       stateVersion.Version,
		"warnings":      warnings,
		"state_version": stateVersion,
	})
}

// ============================================================================
// 辅助函数
// ============================================================================

// isValidationError 判断是否为校验错误
func isValidationError(err error) bool {
	errMsg := err.Error()
	return strings.Contains(errMsg, "lineage mismatch") ||
		strings.Contains(errMsg, "serial must be greater") ||
		strings.Contains(errMsg, "missing required field")
}

// countResources 计算 state 中的资源数量
func (h *StateHandler) countResources(content map[string]interface{}) int {
	if content == nil {
		return 0
	}
	resources, ok := content["resources"].([]interface{})
	if !ok {
		return 0
	}
	return len(resources)
}

// countOutputs 计算 state 中的输出数量
func (h *StateHandler) countOutputs(content map[string]interface{}) int {
	if content == nil {
		return 0
	}
	outputs, ok := content["outputs"].(map[string]interface{})
	if !ok {
		return 0
	}
	return len(outputs)
}

// logStateAccess 记录 state 访问审计日志
func (h *StateHandler) logStateAccess(workspaceID string, userID string, version int) {
	// 调用 StateService 记录审计日志
	h.stateService.LogStateAccess(workspaceID, userID, version)
}
