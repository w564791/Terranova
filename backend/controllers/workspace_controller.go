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
	"gorm.io/gorm"
)

// 归一化 manifest 执行子目录的逻辑已搬到 services.NormalizeManifestSubpath,
// 供 deployment install 与 workspace CRUD 共用同一套规则。

// ptrIfNonEmpty 空串 => nil,否则返回指针(对齐 ManifestSubpath *string,空=NULL)
func ptrIfNonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// derefOr 解引用 *string,nil 时返回 fallback
func derefOr(p *string, fallback string) string {
	if p == nil {
		return fallback
	}
	return *p
}

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
// @Summary Get workspace list
// @Description Get workspace list with pagination and search, including latest task status
// @Tags Workspace
// @Accept json
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param size query int false "Page size" default(20)
// @Param search query string false "Search keyword (name, description, tags)"
// @Param project_id query int false "Project ID (0=all, >0=specific, -1=unassigned)"
// @Success 200 {object} map[string]interface{} "Workspace list"
// @Failure 500 {object} map[string]interface{} "Server error"
// @Router /api/v1/workspaces [get]
// @Security BearerAuth
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
// @Summary Get workspace detail
// @Description Get workspace detail by ID
// @Tags Workspace
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {object} map[string]interface{} "Workspace detail"
// @Failure 400 {object} map[string]interface{} "Invalid workspace ID"
// @Failure 404 {object} map[string]interface{} "Workspace not found"
// @Router /api/v1/workspaces/{id} [get]
// @Security BearerAuth
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

	// 动态解析 provider_config：如果有模板实例引用，实时从全局模板解析
	providerConfig := workspace.ProviderConfig
	instances := workspace.ProviderInstances.GetProviderInstances()
	// 保证前端拿到的始终是数组，而不是 JSONB 空 map {}
	if instances == nil {
		instances = []models.ProviderInstance{}
	}
	if len(instances) > 0 {
		ptService := services.NewProviderTemplateService(wc.workspaceService.GetDB())
		resolved, err := ptService.ResolveProviderConfigFromInstances(instances)
		if err != nil {
			log.Printf("Failed to resolve provider config for workspace %s: %v", workspaceID, err)
			// 解析失败时回退到存储的 provider_config
		} else if resolved != nil {
			providerConfig = models.JSONB(resolved)
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
		// 对外把 manifest_subpath 列回显为 workdir(前端字段名),保证表单往返一致
		"workdir":                  derefOr(workspace.ManifestSubpath, ""),
		"manifest_deployment_id":   workspace.ManifestDeploymentID,
		"manifest_active_tag":      workspace.ManifestActiveTag,
		"state_backend":            workspace.StateBackend,
		"state_config":             workspace.StateConfig,
		"tags":                     workspace.Tags,
		"variables":                workspace.SystemVariables,
		"provider_config":          services.FilterTemplateSensitiveInfo(providerConfig),
		"provider_instances":       instances,
		"notify_settings":          workspace.NotifySettings,
		"state":                    workspace.State,
		"lock_id":                  workspace.LockID,
		"lock_info":                workspace.LockInfo,
		"ui_mode":                  workspace.UIMode,
		"show_unchanged_resources": workspace.ShowUnchangedResources,
		"resource_count":           resourceCount,
		"last_plan_at":             workspace.LastPlanAt,
		"last_apply_at":            workspace.LastApplyAt,
		"created_at":               workspace.CreatedAt,
		"updated_at":               workspace.UpdatedAt,
	}

	// 如果workspace被锁定，查询锁定者的用户名并注入 lock_info.who_display
	if workspace.LockID != nil && workspace.LockInfo != nil {
		if who, ok := workspace.LockInfo["who"].(string); ok {
			var username string
			err := wc.workspaceService.GetDB().Table("users").
				Select("username").
				Where("user_id = ?", who).
				Scan(&username).Error

			if err == nil && username != "" {
				response["locked_by_username"] = username
				// Enrich lock_info.who_display for frontend consumption
				enrichedInfo := make(map[string]interface{})
				for k, v := range workspace.LockInfo {
					enrichedInfo[k] = v
				}
				enrichedInfo["who_display"] = username
				response["lock_info"] = enrichedInfo
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"data":      response,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// CreateWorkspace 创建工作空间
// @Summary Create workspace
// @Description Create a new workspace
// @Tags Workspace
// @Accept json
// @Produce json
// @Param workspace body object true "Workspace configuration"
// @Success 201 {object} map[string]interface{} "Workspace created"
// @Failure 400 {object} map[string]interface{} "Invalid request"
// @Failure 500 {object} map[string]interface{} "Creation failed"
// @Router /api/v1/workspaces [post]
// @Security BearerAuth
func (wc *WorkspaceController) CreateWorkspace(c *gin.Context) {
	var req struct {
		Name              string                    `json:"name" binding:"required"`
		Description       string                    `json:"description"`
		ExecutionMode     string                    `json:"execution_mode" binding:"required"`
		AgentPoolID       *uint                     `json:"agent_pool_id"`
		K8sConfigID       *uint                     `json:"k8s_config_id"`
		AutoApply         bool                      `json:"auto_apply"`
		PlanOnly          bool                      `json:"plan_only"`
		TerraformVersion  string                    `json:"terraform_version"`
		Workdir           string                    `json:"workdir"` // 仅 manifest 模式生效:作为 terraform 执行子目录,存入 manifest_subpath 列
		StateBackend      string                    `json:"state_backend" binding:"required"`
		StateConfig       map[string]interface{}    `json:"state_config"`
		Tags              map[string]interface{}    `json:"tags"`
		Variables         map[string]interface{}    `json:"variables"`
		ProviderConfig    map[string]interface{}    `json:"provider_config"`
		ProviderInstances []models.ProviderInstance `json:"provider_instances"`
		NotifySettings    map[string]interface{}    `json:"notify_settings"`
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
	if req.StateBackend == "" {
		req.StateBackend = "local"
	}
	// workdir 仅 manifest 模式生效:作为 terraform 执行子目录,归一化后存入 manifest_subpath 列。
	// (列名沿用 manifest_subpath;对前端/用户呈现为 workdir。空=manifest 根目录)
	subpath, subErr := services.NormalizeManifestSubpath(req.Workdir)
	if subErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400, "message": "workdir 非法: " + subErr.Error(),
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// Provider 配置互斥：create 阶段如果提交 provider_instances，就不允许同时提 provider_config
	if req.ProviderConfig != nil && len(req.ProviderInstances) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "provider_config 与 provider_instances 不能同时提交",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}
	if len(req.ProviderInstances) > 0 {
		ptService := services.NewProviderTemplateService(wc.workspaceService.GetDB())
		if err := ptService.ValidateInstanceAliases(req.ProviderInstances); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":      400,
				"message":   err.Error(),
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return
		}
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
		// workdir 值存进 manifest_subpath 列(executor 实际读取的字段);workdir 列本身留空不用
		ManifestSubpath:  ptrIfNonEmpty(subpath),
		StateBackend:     req.StateBackend,
		StateConfig:      req.StateConfig,
		Tags:             req.Tags,
		ProviderConfig:   req.ProviderConfig,
		NotifySettings:   req.NotifySettings,
		State:            models.WorkspaceStateCreated,
	}

	// provider_instances 需要走 JSONB 自定义类型写入
	if len(req.ProviderInstances) > 0 {
		instancesJSON, _ := json.Marshal(req.ProviderInstances)
		var jb models.JSONB
		_ = jb.Scan(instancesJSON)
		workspace.ProviderInstances = jb
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
		"workdir":           derefOr(workspace.ManifestSubpath, ""),
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
// @Summary Update workspace
// @Description Update workspace configuration (supports both PUT and PATCH)
// @Tags Workspace
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param workspace body object true "Workspace update fields"
// @Success 200 {object} map[string]interface{} "Update successful"
// @Failure 400 {object} map[string]interface{} "Invalid request"
// @Failure 500 {object} map[string]interface{} "Update failed"
// @Router /api/v1/workspaces/{id} [put]
// @Router /api/v1/workspaces/{id} [patch]
// @Security BearerAuth
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
		Name                   string                    `json:"name"`
		Description            string                    `json:"description"`
		TerraformVersion       string                    `json:"terraform_version"`
		ExecutionMode          string                    `json:"execution_mode"`
		AgentPoolID            *uint                     `json:"agent_pool_id"`
		K8sConfigID            *uint                     `json:"k8s_config_id"`
		Workdir                *string                   `json:"workdir"` // 指针区分"未提供"与"设空";仅 manifest 模式生效,值存 manifest_subpath 列;装了 manifest 后拒改
		AutoApply              *bool                     `json:"auto_apply"`
		UIMode                 string                    `json:"ui_mode"`
		ShowUnchangedResources *bool                     `json:"show_unchanged_resources"`
		Tags                   map[string]interface{}    `json:"tags"`
		ProviderConfig         map[string]interface{}    `json:"provider_config"`
		ProviderInstances      []models.ProviderInstance `json:"provider_instances"`
		NotifySettings         map[string]interface{}    `json:"notify_settings"`
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
	// workdir 仅 manifest 模式生效,值存 manifest_subpath 列。
	// spec §3.2: install 后不可改 —— 已装 manifest(manifest_deployment_id 非空)则拒绝(409)。
	if req.Workdir != nil {
		var ws models.Workspace
		if err := wc.workspaceService.GetDB().
			Select("manifest_deployment_id").
			Where("workspace_id = ? OR id = ?", workspaceID, workspaceID).
			First(&ws).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"code": 404, "message": "工作空间不存在",
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return
		}
		if ws.ManifestDeploymentID != nil && *ws.ManifestDeploymentID != "" {
			c.JSON(http.StatusConflict, gin.H{
				"code": 409, "message": "该 workspace 已装 manifest,工作目录不可更改;需 uninstall 后重新 install",
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return
		}
		subpath, subErr := services.NormalizeManifestSubpath(*req.Workdir)
		if subErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code": 400, "message": "workdir 非法: " + subErr.Error(),
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return
		}
		if subpath == "" {
			updates["manifest_subpath"] = gorm.Expr("NULL")
		} else {
			updates["manifest_subpath"] = subpath
		}
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
	// Provider 配置三种模式互斥：template（instances）、custom（config）、none
	// 前端约定：
	//   template → 提交 {"provider_instances": [{...}, ...]}
	//   custom   → 提交 {"provider_config": {...}}
	//   none     → 提交 {"provider_instances": []}（空数组触发清空 provider_config）
	// 后端在写入其中一个字段时主动清空另一个，避免数据残留。
	if req.ProviderConfig != nil && req.ProviderInstances != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "provider_config 与 provider_instances 不能同时提交",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}
	// 注意：互斥清空用 gorm.Expr("NULL")，不依赖 GORM 对 map nil 值的行为（版本敏感）
	if req.ProviderConfig != nil {
		pcJSON, _ := json.Marshal(req.ProviderConfig)
		updates["provider_config"] = gorm.Expr("?::jsonb", string(pcJSON))
		hash := calculateProviderConfigHash(req.ProviderConfig)
		if hash != "" {
			updates["provider_config_hash"] = hash
			log.Printf("Calculated provider_config_hash: %s", hash[:16]+"...")
		} else {
			updates["provider_config_hash"] = ""
		}
		// 切到 custom 模式，清空 template 模式的实例
		updates["provider_instances"] = gorm.Expr("NULL")
	}
	if req.ProviderInstances != nil {
		// 校验同 type 下 alias 唯一性
		ptService := services.NewProviderTemplateService(wc.workspaceService.GetDB())
		if err := ptService.ValidateInstanceAliases(req.ProviderInstances); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":      400,
				"message":   err.Error(),
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return
		}
		instancesJSON, _ := json.Marshal(req.ProviderInstances)
		updates["provider_instances"] = gorm.Expr("?::jsonb", string(instancesJSON))
		// 切到 template 或 none 模式，清空 custom 模式的配置和 hash
		updates["provider_config"] = gorm.Expr("NULL")
		updates["provider_config_hash"] = ""
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
// @Summary Delete workspace
// @Description Delete a workspace by ID
// @Tags Workspace
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {object} map[string]interface{} "Deletion successful"
// @Failure 400 {object} map[string]interface{} "Invalid workspace ID"
// @Failure 500 {object} map[string]interface{} "Deletion failed"
// @Router /api/v1/workspaces/{id} [delete]
// @Security BearerAuth
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
// @Summary Get workspace overview
// @Description Get workspace overview including resource stats, recent runs, and configuration
// @Tags Workspace
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {object} services.WorkspaceOverviewResponse
// @Failure 400 {object} map[string]interface{} "Invalid workspace ID"
// @Failure 404 {object} map[string]interface{} "Workspace not found"
// @Failure 500 {object} map[string]interface{} "Server error"
// @Router /api/v1/workspaces/{id}/overview [get]
// @Security BearerAuth
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
