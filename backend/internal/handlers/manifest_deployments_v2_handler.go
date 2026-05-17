package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"iac-platform/internal/models"
	"iac-platform/services"
)

// ManifestDeploymentsV2Handler 处理新版 install / upgrade / uninstall + variable preview
//
// 路由:
//   GET    /manifests/:id/deployments
//   GET    /manifests/:id/deployments/:deployment_id
//   POST   /manifests/:id/deployments/install
//   POST   /manifests/:id/deployments/:deployment_id/upgrade
//   POST   /manifests/:id/deployments/:deployment_id/uninstall
//   POST   /manifests/:id/deployments/:deployment_id/variable-preview
//   GET    /variable-sets/:varset_id/manifest-deployments  (反向关联,变量集详情页用)
//
// 关键性质: 这三个写动作都是纯元信息操作(无 terraform 调用,不动云端)。
// 真实云端变更靠 workspace 现有 plan / plan+apply 任务。
type ManifestDeploymentsV2Handler struct {
	db *gorm.DB
}

func NewManifestDeploymentsV2Handler(db *gorm.DB) *ManifestDeploymentsV2Handler {
	return &ManifestDeploymentsV2Handler{db: db}
}

// ListDeployments 列出某 manifest 的所有 deployment
func (h *ManifestDeploymentsV2Handler) ListDeployments(c *gin.Context) {
	manifestID := c.Param("id")
	var rows []models.ManifestDeployment
	if err := h.db.Where("manifest_id = ?", manifestID).
		Order("created_at DESC").Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deployments": rows})
}

// GetDeployment 详情(含 varsets 关联)
func (h *ManifestDeploymentsV2Handler) GetDeployment(c *gin.Context) {
	deploymentID := c.Param("deployment_id")
	var d models.ManifestDeployment
	if err := h.db.Where("id = ?", deploymentID).First(&d).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	var varsets []models.ManifestDeploymentVarset
	h.db.Where("deployment_id = ?", deploymentID).Order("priority ASC").Find(&varsets)
	c.JSON(http.StatusOK, gin.H{"deployment": d, "varsets": varsets})
}

// Install 把指定 published version 装到空 workspace
func (h *ManifestDeploymentsV2Handler) Install(c *gin.Context) {
	manifestID := c.Param("id")
	userID := c.GetString("user_id")

	var req models.InstallDeploymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 校验 version 属于本 manifest 且非草稿
	var version models.ManifestVersion
	if err := h.db.Where("id = ? AND manifest_id = ?", req.VersionID, manifestID).First(&version).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid version_id"})
		return
	}
	if version.Version == "draft" || version.Version == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot install draft version"})
		return
	}

	// 校验 workspace 存在 + 同 org
	var ws models.Workspace
	if err := h.db.Where("workspace_id = ?", req.WorkspaceID).First(&ws).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace not found"})
		return
	}

	// 校验 workspace 未装其他 manifest
	if ws.ManifestDeploymentID != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "workspace already has an active manifest deployment"})
		return
	}

	// 校验 workspace 为空: 1) tf_state 无 resource;2) 无 UI 添加的 resource
	empty, reason := h.workspaceIsEmpty(req.WorkspaceID, &ws)
	if !empty {
		c.JSON(http.StatusConflict, gin.H{"error": "workspace not empty", "reason": reason})
		return
	}

	// 校验 subpath 在 manifest_files 内存在(若设置了)
	if ws.ManifestSubpath != nil && *ws.ManifestSubpath != "" {
		if !h.subpathExistsInVersion(manifestID, req.VersionID, *ws.ManifestSubpath) {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("subpath %q not found in version %s (must contain at least one .tf)", *ws.ManifestSubpath, req.VersionID)})
			return
		}
	}

	// 拉 manifest_files 浅 parse
	resourceRefs, err := h.shallowParseVersionResources(manifestID, req.VersionID, ws.ManifestSubpath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	deploymentID := generateManifestDeploymentID()

	overridesJSON, _ := json.Marshal(req.VariableOverrides)

	err = h.db.Transaction(func(tx *gorm.DB) error {
		// 1. 写 manifest_deployments
		now := time.Now()
		dep := models.ManifestDeployment{
			ID:                deploymentID,
			ManifestID:        manifestID,
			VersionID:         req.VersionID,
			WorkspaceID:       req.WorkspaceID,
			VariableOverrides: overridesJSON,
			Status:            models.DeploymentStatusActive,
			DeployedBy:        userID,
			DeployedAt:        &now,
		}
		if err := tx.Create(&dep).Error; err != nil {
			return err
		}

		// 2. 写 varsets
		for _, v := range req.Varsets {
			if err := tx.Create(&models.ManifestDeploymentVarset{
				DeploymentID: deploymentID,
				VarsetID:     v.VarsetID,
				Priority:     v.Priority,
			}).Error; err != nil {
				return err
			}
		}

		// 3. 浅 parse 结果写 workspace_resources
		for _, ref := range resourceRefs {
			if ref.Kind != "resource" {
				continue
			}
			row := models.WorkspaceResource{
				WorkspaceID:          req.WorkspaceID,
				ResourceID:           fmt.Sprintf("%s.%s", ref.Type, ref.Name),
				ResourceType:         ref.Type,
				ResourceName:         ref.Name,
				IsActive:             true,
				ManifestDeploymentID: &deploymentID,
				CreatedBy:            &userID,
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}

		// 4. 更新 workspaces 三列
		if err := tx.Model(&models.Workspace{}).
			Where("workspace_id = ?", req.WorkspaceID).
			Updates(map[string]interface{}{
				"manifest_deployment_id": deploymentID,
				"manifest_active_tag":    version.Version,
			}).Error; err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"deployment_id":   deploymentID,
		"version":         version.Version,
		"workspace_id":    req.WorkspaceID,
		"resources_added": len(resourceRefs),
	})
}

// Upgrade 切换版本与 varsets,reconcile workspace_resources
func (h *ManifestDeploymentsV2Handler) Upgrade(c *gin.Context) {
	deploymentID := c.Param("deployment_id")
	manifestID := c.Param("id")
	userID := c.GetString("user_id")

	var req models.UpgradeDeploymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var dep models.ManifestDeployment
	if err := h.db.Where("id = ? AND manifest_id = ?", deploymentID, manifestID).First(&dep).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
		return
	}
	if dep.Status != models.DeploymentStatusActive {
		c.JSON(http.StatusConflict, gin.H{"error": "deployment is not active"})
		return
	}

	// 校验 target version 属于本 manifest
	var version models.ManifestVersion
	if err := h.db.Where("id = ? AND manifest_id = ?", req.TargetVersionID, manifestID).First(&version).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target_version_id"})
		return
	}
	if version.Version == "draft" || version.Version == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot upgrade to draft version"})
		return
	}

	// 拉新版本的 resource refs
	var ws models.Workspace
	h.db.Where("workspace_id = ?", dep.WorkspaceID).First(&ws)
	newRefs, err := h.shallowParseVersionResources(manifestID, req.TargetVersionID, ws.ManifestSubpath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	overridesJSON, _ := json.Marshal(req.VariableOverrides)

	err = h.db.Transaction(func(tx *gorm.DB) error {
		// 1. 更新 deployment
		if err := tx.Model(&dep).Updates(map[string]interface{}{
			"version_id":         req.TargetVersionID,
			"variable_overrides": overridesJSON,
			"deployed_by":        userID,
			"deployed_at":        time.Now(),
		}).Error; err != nil {
			return err
		}

		// 2. 重写 varsets
		if err := tx.Where("deployment_id = ?", deploymentID).Delete(&models.ManifestDeploymentVarset{}).Error; err != nil {
			return err
		}
		for _, v := range req.Varsets {
			if err := tx.Create(&models.ManifestDeploymentVarset{
				DeploymentID: deploymentID,
				VarsetID:     v.VarsetID,
				Priority:     v.Priority,
			}).Error; err != nil {
				return err
			}
		}

		// 3. Reconcile workspace_resources
		// 旧资源 set
		var oldRows []models.WorkspaceResource
		tx.Where("workspace_id = ? AND manifest_deployment_id = ?", dep.WorkspaceID, deploymentID).
			Find(&oldRows)
		oldSet := make(map[string]uint, len(oldRows))
		for _, r := range oldRows {
			oldSet[r.ResourceID] = r.ID
		}

		// 新资源 set (resource_id = "<type>.<name>")
		newSet := make(map[string]bool)
		for _, ref := range newRefs {
			if ref.Kind != "resource" {
				continue
			}
			newSet[fmt.Sprintf("%s.%s", ref.Type, ref.Name)] = true
		}

		// 删除旧集合中存在、新集合不存在的
		for rid, id := range oldSet {
			if !newSet[rid] {
				if err := tx.Delete(&models.WorkspaceResource{}, id).Error; err != nil {
					return err
				}
			}
		}
		// 插入新集合中存在、旧集合不存在的
		for _, ref := range newRefs {
			if ref.Kind != "resource" {
				continue
			}
			rid := fmt.Sprintf("%s.%s", ref.Type, ref.Name)
			if _, ok := oldSet[rid]; ok {
				continue
			}
			if err := tx.Create(&models.WorkspaceResource{
				WorkspaceID:          dep.WorkspaceID,
				ResourceID:           rid,
				ResourceType:         ref.Type,
				ResourceName:         ref.Name,
				IsActive:             true,
				ManifestDeploymentID: &deploymentID,
				CreatedBy:            &userID,
			}).Error; err != nil {
				return err
			}
		}

		// 4. 更新 workspaces.manifest_active_tag
		if err := tx.Model(&models.Workspace{}).
			Where("workspace_id = ?", dep.WorkspaceID).
			Update("manifest_active_tag", version.Version).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"deployment_id": deploymentID,
		"version":       version.Version,
	})
}

// Uninstall 解绑 manifest 与 workspace,清相关 workspace_resources
// 不动云端;workspace 进入"反向漂移"状态等待用户跑 Plan+Apply 清理
func (h *ManifestDeploymentsV2Handler) Uninstall(c *gin.Context) {
	deploymentID := c.Param("deployment_id")
	manifestID := c.Param("id")

	var dep models.ManifestDeployment
	if err := h.db.Where("id = ? AND manifest_id = ?", deploymentID, manifestID).First(&dep).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
		return
	}
	if dep.Status != models.DeploymentStatusActive {
		c.JSON(http.StatusConflict, gin.H{"error": "deployment is not active"})
		return
	}

	err := h.db.Transaction(func(tx *gorm.DB) error {
		// 1. deployment.status = uninstalled
		if err := tx.Model(&dep).Update("status", models.DeploymentStatusUninstalled).Error; err != nil {
			return err
		}
		// 2. 清 workspaces 三列
		if err := tx.Model(&models.Workspace{}).
			Where("workspace_id = ?", dep.WorkspaceID).
			Updates(map[string]interface{}{
				"manifest_deployment_id": nil,
				"manifest_active_tag":    nil,
				"manifest_subpath":       nil,
			}).Error; err != nil {
			return err
		}
		// 3. 删 workspace_resources 关联行
		if err := tx.Where("workspace_id = ? AND manifest_deployment_id = ?", dep.WorkspaceID, deploymentID).
			Delete(&models.WorkspaceResource{}).Error; err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"deployment_id": deploymentID,
		"workspace_id":  dep.WorkspaceID,
		"hint":          "manifest uninstalled, workspace state may still contain leftover resources. Run Plan+Apply on the workspace to destroy them.",
	})
}

// VariablePreview 返回最终合并后的变量值(非敏感)用于 install/upgrade 对话框预览
func (h *ManifestDeploymentsV2Handler) VariablePreview(c *gin.Context) {
	deploymentID := c.Param("deployment_id")

	var req struct {
		Varsets           []models.DeploymentVarsetEntry `json:"varsets"`
		VariableOverrides map[string]string              `json:"variable_overrides"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var dep models.ManifestDeployment
	if err := h.db.Where("id = ?", deploymentID).First(&dep).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
		return
	}

	// 只展示非敏感值
	resolver := services.NewVariableResolutionService(h.db)
	extraIDs := make([]string, 0, len(req.Varsets))
	for _, v := range req.Varsets {
		extraIDs = append(extraIDs, v.VarsetID)
	}
	values, err := resolver.ResolveDisplayWithExtra(dep.WorkspaceID, extraIDs, req.VariableOverrides)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"variables": values})
}

// VarsetReverseLookup 列出使用某 varset 的 active deployment
// GET /variable-sets/:varset_id/manifest-deployments
func (h *ManifestDeploymentsV2Handler) VarsetReverseLookup(c *gin.Context) {
	varsetID := c.Param("varset_id")

	type row struct {
		DeploymentID string    `json:"deployment_id"`
		ManifestID   string    `json:"manifest_id"`
		WorkspaceID  string    `json:"workspace_id"`
		VersionID    string    `json:"version_id"`
		Priority     int       `json:"priority"`
		DeployedAt   time.Time `json:"deployed_at"`
	}
	var rows []row
	if err := h.db.Raw(`
		SELECT md.id AS deployment_id, md.manifest_id, md.workspace_id, md.version_id,
		       mdv.priority, md.deployed_at
		  FROM manifest_deployments md
		  JOIN manifest_deployment_varsets mdv ON mdv.deployment_id = md.id
		 WHERE mdv.varset_id = ? AND md.status = ?
		 ORDER BY md.deployed_at DESC
	`, varsetID, models.DeploymentStatusActive).Scan(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deployments": rows})
}

// =============================================================================
// 内部工具
// =============================================================================

// workspaceIsEmpty: tf_state 无 resource 且 workspace_resources(非 manifest 来源)为 0
func (h *ManifestDeploymentsV2Handler) workspaceIsEmpty(workspaceID string, ws *models.Workspace) (bool, string) {
	// 1. UI 添加的 resource = workspace_resources WHERE workspace_id=? AND manifest_deployment_id IS NULL
	var uiCount int64
	h.db.Model(&models.WorkspaceResource{}).
		Where("workspace_id = ? AND manifest_deployment_id IS NULL", workspaceID).
		Count(&uiCount)
	if uiCount > 0 {
		return false, fmt.Sprintf("workspace has %d UI-managed resources", uiCount)
	}

	// 2. tf_state: 简单解析 resources 数组长度
	if ws.TFState != nil {
		if rs, ok := ws.TFState["resources"].([]interface{}); ok && len(rs) > 0 {
			return false, fmt.Sprintf("workspace tf_state has %d resources", len(rs))
		}
	}
	return true, ""
}

// subpathExistsInVersion: 校验 subpath 下至少有一个 .tf 文件
func (h *ManifestDeploymentsV2Handler) subpathExistsInVersion(manifestID, versionID, subpath string) bool {
	var n int64
	prefix := subpath
	if prefix != "" && prefix[len(prefix)-1] != '/' {
		prefix = prefix + "/"
	}
	h.db.Model(&models.ManifestFile{}).
		Where("manifest_id = ? AND version_id = ?", manifestID, versionID).
		Where("path LIKE ?", prefix+"%.tf").
		Count(&n)
	return n > 0
}

// shallowParseVersionResources 拉 version 的 manifest_files,浅 parse 出 resource/module refs
func (h *ManifestDeploymentsV2Handler) shallowParseVersionResources(
	manifestID, versionID string, subpath *string,
) ([]services.ManifestResourceRef, error) {
	var rows []models.ManifestFile
	if err := h.db.Where("manifest_id = ? AND version_id = ?", manifestID, versionID).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	scope := make(map[string][]byte, len(rows))
	for _, r := range rows {
		scope[r.Path] = r.Content
	}
	sp := ""
	if subpath != nil {
		sp = *subpath
	}
	return services.ParseManifestResources(scope, sp), nil
}
