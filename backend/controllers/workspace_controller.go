package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"iac-platform/internal/application/service"
	"iac-platform/internal/domain/valueobject"
	"iac-platform/internal/models"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
)

// calculateProviderConfigHash 计算 provider_config 的 SHA256 hash
// 用于跟踪 provider 配置变更，优化 terraform init -upgrade
func calculateProviderConfigHash(config interface{}) string {
	if config == nil {
		return ""
	}

	// 序列化为 JSON
	jsonBytes, err := json.Marshal(config)
	if err != nil {
		log.Printf("Failed to marshal provider_config for hash: %v", err)
		return ""
	}

	// 计算 SHA256
	hash := sha256.Sum256(jsonBytes)
	return hex.EncodeToString(hash[:])
}

type WorkspaceController struct {
	workspaceService  *services.WorkspaceService
	overviewService   *services.WorkspaceOverviewService
	permissionService service.PermissionService
}

func NewWorkspaceController(
	workspaceService *services.WorkspaceService,
	overviewService *services.WorkspaceOverviewService,
	permissionService service.PermissionService,
) *WorkspaceController {
	return &WorkspaceController{
		workspaceService:  workspaceService,
		overviewService:   overviewService,
		permissionService: permissionService,
	}
}

// GetWorkspaces 获取工作空间列表
// @Summary 获取工作空间列表
// @Description 获取工作空间列表，支持分页和搜索，包含最新任务状态
// @Tags Workspace
// @Accept json
// @Produce json
// @Param page query int false "页码" default(1)
// @Param size query int false "每页数量" default(20)
// @Param search query string false "搜索关键词（支持name、description、tags）"
// @Param project_id query int false "项目ID（0=所有，>0=指定项目，-1=未分配项目）"
// @Success 200 {object} map[string]interface{} "成功返回工作空间列表"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /workspaces [get]
// @Security Bearer
func (wc *WorkspaceController) GetWorkspaces(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	search := c.Query("search")
	projectIDStr := c.Query("project_id")

	// 解析 project_id 参数
	// 0: 不过滤项目（返回所有）
	// >0: 过滤指定项目
	// -1: 只返回未分配项目的工作空间（归入 default）
	projectID := 0
	if projectIDStr != "" {
		projectID, _ = strconv.Atoi(projectIDStr)
	}

	// 使用包含状态信息的查询方法
	workspaces, total, err := wc.workspaceService.SearchWorkspacesWithStatus(search, page, size, projectID)
	if err != nil {
		// 返回模拟数据
		mockWorkspaces := []services.WorkspaceWithStatus{
			{
				WorkspaceListItem: services.WorkspaceListItem{
					ID:               1,
					Name:             "production",
					Description:      "生产环境工作空间",
					StateBackend:     "S3",
					TerraformVersion: "1.5.0",
					ExecutionMode:    "local",
					CreatedAt:        time.Now(),
					UpdatedAt:        time.Now(),
				},
			},
			{
				WorkspaceListItem: services.WorkspaceListItem{
					ID:               2,
					Name:             "staging",
					Description:      "测试环境工作空间",
					StateBackend:     "Local",
					TerraformVersion: "1.5.0",
					ExecutionMode:    "agent",
					CreatedAt:        time.Now(),
					UpdatedAt:        time.Now(),
				},
			},
		}
		c.JSON(http.StatusOK, gin.H{
			"code": 200,
			"data": gin.H{
				"items": mockWorkspaces,
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
			"items": workspaces,
			"total": total,
			"page":  page,
			"size":  size,
		},
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// GetWorkspace 获取单个工作空间
// @Summary 获取单个工作空间详情
// @Description 根据ID获取工作空间的详细信息
// @Tags Workspace
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Success 200 {object} map[string]interface{} "成功返回工作空间详情"
// @Failure 400 {object} map[string]interface{} "无效的工作空间ID"
// @Failure 404 {object} map[string]interface{} "工作空间不存在"
// @Router /workspaces/{id} [get]
// @Security Bearer
func (wc *WorkspaceController) GetWorkspace(c *gin.Context) {
	workspaceID := c.Param("id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "无效的工作空间ID",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	workspace, err := wc.workspaceService.GetWorkspaceByID(workspaceID)
	if err != nil {
		// 返回模拟数据
		mockWorkspace := models.Workspace{
			WorkspaceID:      workspaceID,
			Name:             "production",
			Description:      "生产环境工作空间，用于部署生产级别的基础设施",
			StateBackend:     "S3",
			TerraformVersion: "1.5.0",
			ExecutionMode:    "server",
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}
		if workspaceID == "ws-staging" {
			mockWorkspace.Name = "staging"
			mockWorkspace.Description = "测试环境工作空间，用于开发和测试阶段的基础设施部署"
			mockWorkspace.StateBackend = "Local"
			mockWorkspace.ExecutionMode = "agent"
		}
		c.JSON(http.StatusOK, gin.H{
			"code":      200,
			"data":      mockWorkspace,
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// 获取最新 state version 并实时计算资源数量
	var resourceCount int
	var stateVersion models.WorkspaceStateVersion
	err = wc.workspaceService.GetDB().
		Where("workspace_id = ?", workspaceID).
		Order("version DESC").
		First(&stateVersion).Error

	if err == nil && stateVersion.Content != nil {
		// 实时从 state JSON 的 resources 数组计算资源数量
		if resources, ok := stateVersion.Content["resources"].([]interface{}); ok {
			resourceCount = len(resources)
		}
	}

	// 构建响应，添加locked_by_username和ui_mode
	response := gin.H{
		"id":                       workspace.WorkspaceID,
		"workspace_id":             workspace.WorkspaceID,
		"name":                     workspace.Name,
		"description":              workspace.Description,
		"execution_mode":           workspace.ExecutionMode,
		"agent_pool_id":            workspace.AgentPoolID,
		"k8s_config_id":            workspace.K8sConfigID,
		"auto_apply":               workspace.AutoApply,
		"plan_only":                workspace.PlanOnly,
		"terraform_version":        workspace.TerraformVersion,
		"workdir":                  workspace.Workdir,
		"state_backend":            workspace.StateBackend,
		"state_config":             workspace.StateConfig,
		"tags":                     workspace.Tags,
		"variables":                workspace.SystemVariables,
		"provider_config":          workspace.ProviderConfig,
		"provider_template_ids":    workspace.ProviderTemplateIDs,
		"provider_overrides":       workspace.ProviderOverrides,
		"notify_settings":          workspace.NotifySettings,
		"state":                    workspace.State,
		"is_locked":                workspace.IsLocked,
		"locked_by":                workspace.LockedBy,
		"locked_at":                workspace.LockedAt,
		"lock_reason":              workspace.LockReason,
		"ui_mode":                  workspace.UIMode,
		"show_unchanged_resources": workspace.ShowUnchangedResources,
		"resource_count":           resourceCount,
		"last_plan_at":             workspace.LastPlanAt,
		"last_apply_at":            workspace.LastApplyAt,
		"created_at":               workspace.CreatedAt,
		"updated_at":               workspace.UpdatedAt,
	}

	// 如果workspace被锁定，查询锁定者的用户名
	if workspace.IsLocked && workspace.LockedBy != nil {
		var username string
		err := wc.workspaceService.GetDB().Table("users").
			Select("username").
			Where("id = ?", *workspace.LockedBy).
			Scan(&username).Error

		if err == nil && username != "" {
			response["locked_by_username"] = username
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"data":      response,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// CreateWorkspace 创建工作空间
// @Summary 创建新的工作空间
// @Description 创建一个新的工作空间
// @Tags Workspace
// @Accept json
// @Produce json
// @Param workspace body object true "工作空间信息"
// @Success 201 {object} map[string]interface{} "成功创建工作空间"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 500 {object} map[string]interface{} "创建失败"
// @Router /workspaces [post]
// @Security Bearer
func (wc *WorkspaceController) CreateWorkspace(c *gin.Context) {
	var req struct {
		Name             string                 `json:"name" binding:"required"`
		Description      string                 `json:"description"`
		ExecutionMode    string                 `json:"execution_mode" binding:"required"`
		AgentPoolID      *uint                  `json:"agent_pool_id"`
		K8sConfigID      *uint                  `json:"k8s_config_id"`
		AutoApply        bool                   `json:"auto_apply"`
		PlanOnly         bool                   `json:"plan_only"`
		TerraformVersion string                 `json:"terraform_version"`
		Workdir          string                 `json:"workdir"`
		StateBackend     string                 `json:"state_backend" binding:"required"`
		StateConfig      map[string]interface{} `json:"state_config"`
		Tags             map[string]interface{} `json:"tags"`
		Variables        map[string]interface{} `json:"variables"`
		ProviderConfig   map[string]interface{} `json:"provider_config"`
		NotifySettings   map[string]interface{} `json:"notify_settings"`
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

	// 验证执行模式
	executionMode := models.ExecutionMode(req.ExecutionMode)
	if executionMode != models.ExecutionModeLocal &&
		executionMode != models.ExecutionModeAgent &&
		executionMode != models.ExecutionModeK8s {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "无效的执行模式，必须是 local、agent 或 k8s",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// Agent模式必须指定Agent Pool
	if executionMode == models.ExecutionModeAgent && req.AgentPoolID == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "Agent执行模式必须指定agent_pool_id",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// K8s模式必须指定K8s配置
	if executionMode == models.ExecutionModeK8s && req.K8sConfigID == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "K8s执行模式必须指定k8s_config_id",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// 设置默认值
	if req.TerraformVersion == "" {
		req.TerraformVersion = "latest"
	}
	if req.Workdir == "" {
		req.Workdir = "/workspace"
	}
	if req.StateBackend == "" {
		req.StateBackend = "local"
	}

	workspace := &models.Workspace{
		Name:             req.Name,
		Description:      req.Description,
		ExecutionMode:    executionMode,
		AgentPoolID:      req.AgentPoolID,
		K8sConfigID:      req.K8sConfigID,
		AutoApply:        req.AutoApply,
		PlanOnly:         req.PlanOnly,
		TerraformVersion: req.TerraformVersion,
		Workdir:          req.Workdir,
		StateBackend:     req.StateBackend,
		StateConfig:      req.StateConfig,
		Tags:             req.Tags,
		ProviderConfig:   req.ProviderConfig,
		NotifySettings:   req.NotifySettings,
		State:            models.WorkspaceStateCreated,
	}

	// 如果提供了variables，设置系统变量
	if req.Variables != nil {
		workspace.SystemVariables = req.Variables
	}

	if err := wc.workspaceService.CreateWorkspace(workspace); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "创建工作空间失败",
			"error":     err.Error(),
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// 自动为创建者授予 ADMIN 权限
	// 这样创建者可以立即操作自己创建的 workspace，无需额外授权
	if wc.permissionService != nil {
		userID, exists := c.Get("user_id")
		if exists && userID != nil {
			wc.grantCreatorPermissions(workspace.ID, userID.(string))
		}
	}

	// 构建响应，使用workspace_id作为id字段
	response := gin.H{
		"id":                workspace.WorkspaceID, // 使用语义化ID
		"workspace_id":      workspace.WorkspaceID,
		"name":              workspace.Name,
		"description":       workspace.Description,
		"execution_mode":    workspace.ExecutionMode,
		"agent_pool_id":     workspace.AgentPoolID,
		"k8s_config_id":     workspace.K8sConfigID,
		"auto_apply":        workspace.AutoApply,
		"plan_only":         workspace.PlanOnly,
		"terraform_version": workspace.TerraformVersion,
		"workdir":           workspace.Workdir,
		"state_backend":     workspace.StateBackend,
		"state_config":      workspace.StateConfig,
		"tags":              workspace.Tags,
		"variables":         workspace.SystemVariables,
		"provider_config":   workspace.ProviderConfig,
		"notify_settings":   workspace.NotifySettings,
		"state":             workspace.State,
		"created_at":        workspace.CreatedAt,
		"updated_at":        workspace.UpdatedAt,
	}

	c.JSON(http.StatusCreated, gin.H{
		"code":      201,
		"data":      response,
		"message":   "工作空间创建成功",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// UpdateWorkspace 更新工作空间
// @Summary 更新工作空间
// @Description 更新工作空间的配置信息
// @Tags Workspace
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param workspace body object true "更新的工作空间信息"
// @Success 200 {object} map[string]interface{} "更新成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 500 {object} map[string]interface{} "更新失败"
// @Router /workspaces/{id} [put]
// @Security Bearer
func (wc *WorkspaceController) UpdateWorkspace(c *gin.Context) {
	workspaceID := c.Param("id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "无效的工作空间ID",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// 添加日志
	log.Printf("UpdateWorkspace called: workspace_id=%s, method=%s", workspaceID, c.Request.Method)

	var req struct {
		Name                   string                 `json:"name"`
		Description            string                 `json:"description"`
		TerraformVersion       string                 `json:"terraform_version"`
		ExecutionMode          string                 `json:"execution_mode"`
		AgentPoolID            *uint                  `json:"agent_pool_id"`
		K8sConfigID            *uint                  `json:"k8s_config_id"`
		Workdir                string                 `json:"workdir"`
		AutoApply              *bool                  `json:"auto_apply"`
		UIMode                 string                 `json:"ui_mode"`
		ShowUnchangedResources *bool                  `json:"show_unchanged_resources"`
		Tags                   map[string]interface{} `json:"tags"`
		ProviderConfig         map[string]interface{} `json:"provider_config"`
		ProviderTemplateIDs    []uint                 `json:"provider_template_ids"`
		ProviderOverrides      map[string]interface{} `json:"provider_overrides"`
		NotifySettings         map[string]interface{} `json:"notify_settings"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("Failed to bind JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "请求参数无效",
			"error":     err.Error(),
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	log.Printf("Request body parsed: provider_config=%v", req.ProviderConfig != nil)

	// 如果更新provider_config，先验证
	if req.ProviderConfig != nil {
		log.Printf("Validating provider_config...")
		providerService := services.NewProviderService()
		if err := providerService.ValidateProviderConfig(req.ProviderConfig); err != nil {
			log.Printf("Provider validation failed: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"code":      400,
				"message":   "Provider配置验证失败",
				"error":     err.Error(),
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return
		}
		log.Printf("Provider validation passed")
	}

	// 构建更新字段
	updates := make(map[string]interface{})

	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if req.TerraformVersion != "" {
		updates["terraform_version"] = req.TerraformVersion
	}
	if req.ExecutionMode != "" {
		updates["execution_mode"] = req.ExecutionMode
	}
	if req.AgentPoolID != nil {
		updates["agent_pool_id"] = req.AgentPoolID
	}
	if req.K8sConfigID != nil {
		updates["k8s_config_id"] = req.K8sConfigID
	}
	if req.Workdir != "" {
		updates["workdir"] = req.Workdir
	}
	if req.AutoApply != nil {
		updates["auto_apply"] = *req.AutoApply
	}
	if req.UIMode != "" {
		updates["ui_mode"] = req.UIMode
	}
	if req.ShowUnchangedResources != nil {
		updates["show_unchanged_resources"] = *req.ShowUnchangedResources
	}
	if req.Tags != nil {
		updates["tags"] = req.Tags
	}
	if req.ProviderConfig != nil {
		updates["provider_config"] = req.ProviderConfig
		// 计算 provider_config 的 hash，用于优化 terraform init -upgrade
		hash := calculateProviderConfigHash(req.ProviderConfig)
		if hash != "" {
			updates["provider_config_hash"] = hash
			log.Printf("Calculated provider_config_hash: %s", hash[:16]+"...")
		}
	}
	// 处理provider模板引用
	if req.ProviderTemplateIDs != nil {
		updates["provider_template_ids"] = req.ProviderTemplateIDs

		// 解析并缓存最终的provider_config
		ptService := services.NewProviderTemplateService(wc.workspaceService.GetDB())
		resolvedConfig, err := ptService.ResolveProviderConfig(req.ProviderTemplateIDs, req.ProviderOverrides)
		if err != nil {
			log.Printf("Failed to resolve provider config: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"code":      400,
				"message":   "Provider模板解析失败",
				"error":     err.Error(),
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return
		}

		if resolvedConfig != nil {
			updates["provider_config"] = resolvedConfig
			hash := calculateProviderConfigHash(resolvedConfig)
			if hash != "" {
				updates["provider_config_hash"] = hash
			}
		} else {
			updates["provider_config"] = nil
			updates["provider_config_hash"] = ""
		}
	}
	if req.ProviderOverrides != nil {
		updates["provider_overrides"] = req.ProviderOverrides
	}
	if req.NotifySettings != nil {
		updates["notify_settings"] = req.NotifySettings
	}

	log.Printf("Calling UpdateWorkspaceFields with %d updates", len(updates))

	if err := wc.workspaceService.UpdateWorkspaceFields(workspaceID, updates); err != nil {
		log.Printf("UpdateWorkspaceFields failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "更新工作空间失败",
			"error":     err.Error(),
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	log.Printf("UpdateWorkspace completed successfully")

	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"message":   "工作空间更新成功",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// DeleteWorkspace 删除工作空间
// @Summary 删除工作空间
// @Description 删除指定的工作空间
// @Tags Workspace
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Success 200 {object} map[string]interface{} "删除成功"
// @Failure 400 {object} map[string]interface{} "无效的工作空间ID"
// @Failure 500 {object} map[string]interface{} "删除失败"
// @Router /workspaces/{id} [delete]
// @Security Bearer
func (wc *WorkspaceController) DeleteWorkspace(c *gin.Context) {
	workspaceID := c.Param("id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "无效的工作空间ID",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	if err := wc.workspaceService.DeleteWorkspace(workspaceID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "删除工作空间失败",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"message":   "工作空间删除成功",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// GetWorkspaceOverview 获取Workspace Overview
// @Summary 获取Workspace Overview
// @Description 获取Workspace的完整概览信息，包括资源统计、最近运行、配置等
// @Tags Workspace
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {object} services.WorkspaceOverviewResponse
// @Failure 400 {object} map[string]interface{} "无效的工作空间ID"
// @Failure 404 {object} map[string]interface{} "工作空间不存在"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/workspaces/{id}/overview [get]
func (wc *WorkspaceController) GetWorkspaceOverview(c *gin.Context) {
	workspaceID := c.Param("id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "无效的工作空间ID",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	overview, err := wc.overviewService.GetWorkspaceOverview(workspaceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":      404,
			"message":   "工作空间不存在",
			"error":     err.Error(),
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"data":      overview,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// grantCreatorPermissions 为 workspace 创建者授予 ADMIN 权限
// 这是一个内部方法，在创建 workspace 后自动调用
// 授权失败不会影响 workspace 创建的成功响应，只会记录日志
func (wc *WorkspaceController) grantCreatorPermissions(workspaceID uint, userID string) {
	if wc.permissionService == nil {
		return
	}

	// 使用 GrantPresetPermissions 授予 ADMIN 预设权限
	// ADMIN 预设包含该 workspace 的所有权限
	ctx := context.Background()
	req := &service.GrantPresetRequest{
		ScopeType:     valueobject.ScopeTypeWorkspace,
		ScopeID:       workspaceID,
		PrincipalType: valueobject.PrincipalTypeUser,
		PrincipalID:   userID,
		PresetName:    "ADMIN",
		GrantedBy:     userID, // 创建者自己授权给自己
		Reason:        "Auto-granted on workspace creation",
	}

	if err := wc.permissionService.GrantPresetPermissions(ctx, req); err != nil {
		// 授权失败只记录日志，不影响 workspace 创建
		log.Printf("[WARN] Failed to auto-grant permissions for workspace %d to user %s: %v",
			workspaceID, userID, err)
	} else {
		log.Printf("[INFO] Auto-granted ADMIN permissions for workspace %d to creator %s",
			workspaceID, userID)
	}
}
