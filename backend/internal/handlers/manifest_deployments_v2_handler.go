package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"iac-platform/internal/application/service"
	"iac-platform/internal/middleware"
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
	db   *gorm.DB
	perm *middleware.IAMPermissionMiddleware // 用于 deployment 写动作叠加目标 workspace 权限
}

func NewManifestDeploymentsV2Handler(db *gorm.DB, perm *middleware.IAMPermissionMiddleware) *ManifestDeploymentsV2Handler {
	return &ManifestDeploymentsV2Handler{db: db, perm: perm}
}

// ListDeployments 列出某 manifest 的所有 deployment
// @Summary List manifest deployments
// @Description List all deployments for a manifest
// @Tags Manifest Deployments
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/organizations/{org_id}/manifests/{id}/v2/deployments [get]
// @Security BearerAuth
func (h *ManifestDeploymentsV2Handler) ListDeployments(c *gin.Context) {
	manifestID := c.Param("id")
	if !h.manifestInAuthOrg(c, manifestID) {
		return
	}

	// 路由上 RequireWorkspaceListAccess 已解析出调用者可读的 workspace 集合;
	// 缺失视为配置错误,失败关闭(不退化为不过滤)。
	rawAccess, exists := c.Get(service.WorkspaceListAccessContextKey)
	access, ok := rawAccess.(*service.WorkspaceListAccess)
	if !exists || !ok || access == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "workspace list authorization context is missing"})
		return
	}

	query := h.db.Where("manifest_id = ?", manifestID)
	if !access.FullOrganization {
		// 空切片 => 空结果(IN () 由 gorm 渲染为 NULL 条件,不会退化成全量)
		if len(access.WorkspaceIDs) == 0 {
			c.JSON(http.StatusOK, gin.H{"deployments": []models.ManifestDeployment{}})
			return
		}
		query = query.Where("workspace_id IN ?", access.WorkspaceIDs)
	}
	rows := make([]models.ManifestDeployment, 0)
	if err := query.Order("created_at DESC").Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deployments": rows})
}

// rejectUnmountableVarsets 校验请求里的 varset 都可挂载到目标 workspace
// (与 GET /variable-sets?workspace_id= 同一规则)。不可挂载 => 400,返回 false。
func (h *ManifestDeploymentsV2Handler) rejectUnmountableVarsets(
	c *gin.Context, workspaceID string, entries []models.DeploymentVarsetEntry,
) bool {
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.VarsetID)
	}
	bad, err := services.NewVariableSetService(h.db).UnmountableVarsetIDs(workspaceID, ids)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return false
	}
	if len(bad) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "variable sets not mountable on target workspace", "varset_ids": bad})
		return false
	}
	return true
}

// manifestInAuthOrg 校验 path manifest 属于 IAM 中间件解析出的组织(auth_org_id)。
// 不属于 / 不存在 => 404,避免跨租户枚举。已写响应时返回 false。
func (h *ManifestDeploymentsV2Handler) manifestInAuthOrg(c *gin.Context, manifestID string) bool {
	return manifestInAuthOrg(c, h.db, manifestID)
}

func manifestInAuthOrg(c *gin.Context, db *gorm.DB, manifestID string) bool {
	orgID, ok := middleware.AuthOrgID(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "org_id is required"})
		return false
	}
	var count int64
	if err := db.Model(&models.Manifest{}).
		Where("id = ? AND organization_id = ?", manifestID, orgID).
		Count(&count).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return false
	}
	if count == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "manifest not found"})
		return false
	}
	return true
}

// ManifestInAuthOrg 把同一 manifestInAuthOrg 校验挂成路由中间件,用于
// /organizations/:org_id/manifests/:id/... 下每条路由。必须放在该路由的
// RequirePermission 之后:auth_org_id 由 RequirePermission 写入,且先鉴权可保证
// 无组织权限的调用者只拿到 403,无法用 404/403 差异探测 manifest ID。
func ManifestInAuthOrg(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !manifestInAuthOrg(c, db, c.Param("id")) {
			c.Abort()
			return
		}
		c.Next()
	}
}

// GetDeployment 详情(含 varsets 关联)
// @Summary Get manifest deployment
// @Description Get deployment detail including linked variable sets
// @Tags Manifest Deployments
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Param deployment_id path string true "Deployment ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/organizations/{org_id}/manifests/{id}/v2/deployments/{deployment_id} [get]
// @Security BearerAuth
func (h *ManifestDeploymentsV2Handler) GetDeployment(c *gin.Context) {
	deploymentID := c.Param("deployment_id")
	manifestID := c.Param("id")
	if !h.manifestInAuthOrg(c, manifestID) {
		return
	}
	var d models.ManifestDeployment
	if err := h.db.Where("id = ? AND manifest_id = ?", deploymentID, manifestID).First(&d).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	// 与列表一致: 只返回调用者可读 workspace 上的 deployment
	// (WORKSPACE_MANAGEMENT READ @ workspace 或 WORKSPACES READ @ org)。
	if h.perm != nil && !h.perm.CheckWorkspaceOrOrgWorkspacesRead(c, d.WorkspaceID) {
		return // 403 已写
	}
	var varsets []models.ManifestDeploymentVarset
	h.db.Where("deployment_id = ?", deploymentID).Order("priority ASC").Find(&varsets)
	c.JSON(http.StatusOK, gin.H{"deployment": d, "varsets": varsets})
}

// Install 把指定 published version 装到空 workspace
// @Summary Install manifest deployment
// @Description Install a published version onto an empty workspace
// @Tags Manifest Deployments
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Param request body models.InstallDeploymentRequest true "Install payload"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 409 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/organizations/{org_id}/manifests/{id}/v2/deployments/install [post]
// @Security BearerAuth
func (h *ManifestDeploymentsV2Handler) Install(c *gin.Context) {
	manifestID := c.Param("id")
	userID := c.GetString("user_id")
	if !h.manifestInAuthOrg(c, manifestID) {
		return
	}

	var req models.InstallDeploymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 叠加目标 workspace 权限: 路由只要求 MANIFESTS READ,部署还需对 body 里的目标
	// workspace 显式持有 WORKSPACE_RESOURCES WRITE(WORKSPACE_MANAGEMENT 伞形权限同样满足)。
	if h.perm != nil && !h.perm.RequireWorkspaceResourcePermission(c, req.WorkspaceID, "WORKSPACE_RESOURCES", "WRITE") {
		return // 403 已写
	}
	if !h.rejectUnmountableVarsets(c, req.WorkspaceID, req.Varsets) {
		return
	}

	// 目标校验(与首装变量预览共用):workspace 属于本 org、version 属于本 manifest 且已发布
	version, ok := h.resolveInstallTarget(c, manifestID, req.WorkspaceID, req.VersionID)
	if !ok {
		return
	}

	// 加载 workspace 记录
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

	// 计算 effective subpath(terraform 执行子目录):
	//   req.Workdir 非 nil → 以本次值为准(归一化+校验,绝不信前端原值)
	//   req.Workdir 为 nil → 沿用 workspace 记录里已有的 ManifestSubpath(向后兼容)
	var effectiveSubpath string
	if req.Workdir != nil {
		s, normErr := services.NormalizeManifestSubpath(*req.Workdir)
		if normErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "workdir 非法: " + normErr.Error()})
			return
		}
		effectiveSubpath = s
	} else {
		effectiveSubpath = derefStr(ws.ManifestSubpath)
	}

	// 校验 subpath 在 manifest_files 内存在(若非根)
	if effectiveSubpath != "" {
		if !h.subpathExistsInVersion(manifestID, req.VersionID, effectiveSubpath) {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("subpath %q not found in version %s (must contain at least one .tf)", effectiveSubpath, req.VersionID)})
			return
		}
	}

	// 拉 manifest_files 浅 parse(按 effective subpath)
	subpathPtr := ptrIfNonEmptyStr(effectiveSubpath)
	resourceRefs, err := h.shallowParseVersionResources(manifestID, req.VersionID, subpathPtr)
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

		// 3. 浅 parse 结果写 workspace_resources(resource 块与 module 块都写,
		//    module 实例 resource_id="module.<name>",输出提示端点据此解析 source)
		for _, ref := range resourceRefs {
			row := models.WorkspaceResource{
				WorkspaceID:          req.WorkspaceID,
				ResourceID:           ref.WorkspaceResourceID(),
				ResourceType:         ref.WorkspaceResourceType(),
				ResourceName:         ref.Name,
				IsActive:             true,
				ManifestDeploymentID: &deploymentID,
				CreatedBy:            &userID,
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}

		// 4. 更新 workspaces 三列(含 manifest_subpath:effective 为空写 NULL)
		wsUpdates := map[string]interface{}{
			"manifest_deployment_id": deploymentID,
			"manifest_active_tag":    version.Version,
		}
		if effectiveSubpath == "" {
			wsUpdates["manifest_subpath"] = gorm.Expr("NULL")
		} else {
			wsUpdates["manifest_subpath"] = effectiveSubpath
		}
		if err := tx.Model(&models.Workspace{}).
			Where("workspace_id = ?", req.WorkspaceID).
			Updates(wsUpdates).Error; err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	writeManifestAudit(h.db, auditResourceManifestDeployment, "deployment.install", userID, map[string]interface{}{
		"manifest_id":     manifestID,
		"deployment_id":   deploymentID,
		"workspace_id":    req.WorkspaceID,
		"version_id":      req.VersionID,
		"version":         version.Version,
		"resources_added": len(resourceRefs),
		"varset_count":    len(req.Varsets),
	})

	c.JSON(http.StatusCreated, gin.H{
		"deployment_id":   deploymentID,
		"version":         version.Version,
		"workspace_id":    req.WorkspaceID,
		"resources_added": len(resourceRefs),
	})
}

// Upgrade 切换版本与 varsets,reconcile workspace_resources
// @Summary Upgrade manifest deployment
// @Description Switch deployment version and varsets; reconcile workspace resources
// @Tags Manifest Deployments
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Param deployment_id path string true "Deployment ID"
// @Param request body models.UpgradeDeploymentRequest true "Upgrade payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 409 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/organizations/{org_id}/manifests/{id}/v2/deployments/{deployment_id}/upgrade [post]
// @Security BearerAuth
func (h *ManifestDeploymentsV2Handler) Upgrade(c *gin.Context) {
	deploymentID := c.Param("deployment_id")
	manifestID := c.Param("id")
	userID := c.GetString("user_id")
	if !h.manifestInAuthOrg(c, manifestID) {
		return
	}

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

	// 叠加目标 workspace 权限(workspace 藏在 deployment 记录里,需在此校验):
	// 显式 WORKSPACE_RESOURCES WRITE。
	if h.perm != nil && !h.perm.RequireWorkspaceResourcePermission(c, dep.WorkspaceID, "WORKSPACE_RESOURCES", "WRITE") {
		return // 403 已写
	}
	if !h.rejectUnmountableVarsets(c, dep.WorkspaceID, req.Varsets) {
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

	// 覆盖合并(修复 upgrade 清空已有覆盖):缺省 key / 敏感空占位均保留原值,仅 unset_keys 删除
	overrides, err := h.mergeUpgradeOverrides(dep, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	overridesJSON, _ := json.Marshal(overrides)

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

		// 新资源 set (resource 块 "<type>.<name>" + module 块 "module.<name>")
		newSet := make(map[string]bool)
		for _, ref := range newRefs {
			newSet[ref.WorkspaceResourceID()] = true
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
			rid := ref.WorkspaceResourceID()
			if _, ok := oldSet[rid]; ok {
				continue
			}
			if err := tx.Create(&models.WorkspaceResource{
				WorkspaceID:          dep.WorkspaceID,
				ResourceID:           rid,
				ResourceType:         ref.WorkspaceResourceType(),
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
	writeManifestAudit(h.db, auditResourceManifestDeployment, "deployment.upgrade", userID, map[string]interface{}{
		"manifest_id":     manifestID,
		"deployment_id":   deploymentID,
		"workspace_id":    dep.WorkspaceID,
		"old_version_id":  dep.VersionID,
		"new_version_id":  req.TargetVersionID,
		"new_version":     version.Version,
		"varset_count":    len(req.Varsets),
	})
	c.JSON(http.StatusOK, gin.H{
		"deployment_id": deploymentID,
		"version":       version.Version,
	})
}

// mergeUpgradeOverrides 计算 upgrade 后的 variable_overrides。
// 敏感判定沿用变量预览的同一解析(ResolveDisplayWithExtra 的 sensitive 标记),
// 候选 varset 取新旧两组的并集(偏向保留,绝不因判定不全而清空敏感值)。
func (h *ManifestDeploymentsV2Handler) mergeUpgradeOverrides(dep models.ManifestDeployment, req models.UpgradeDeploymentRequest) (map[string]string, error) {
	stored := map[string]string{}
	if len(dep.VariableOverrides) > 0 {
		if err := json.Unmarshal(dep.VariableOverrides, &stored); err != nil {
			return nil, fmt.Errorf("parse stored variable_overrides: %w", err)
		}
	}
	var varsetIDs []string
	if err := h.db.Model(&models.ManifestDeploymentVarset{}).
		Where("deployment_id = ?", dep.ID).Pluck("varset_id", &varsetIDs).Error; err != nil {
		return nil, fmt.Errorf("load deployment varsets: %w", err)
	}
	for _, v := range req.Varsets {
		varsetIDs = append(varsetIDs, v.VarsetID)
	}
	resolved, err := services.NewVariableResolutionService(h.db).ResolveDisplayWithExtra(dep.WorkspaceID, varsetIDs, nil)
	if err != nil {
		return nil, fmt.Errorf("resolve variable sensitivity: %w", err)
	}
	sensitive := make(map[string]bool, len(resolved))
	for _, v := range resolved {
		if v.Sensitive {
			sensitive[v.Key] = true
		}
	}
	return mergeOverrides(stored, req.VariableOverrides, sensitive, req.UnsetKeys), nil
}

// mergeOverrides: stored 为基础;incoming 中非空值或非敏感 key 覆盖写入;
// 敏感 key 的空串是预览掩码占位 => 不写(保留 stored,stored 没有也不新增空覆盖,
// 否则会用空串遮住 varset/workspace 里的真实敏感值);unset 中的 key 最后删除。
func mergeOverrides(stored, incoming map[string]string, sensitive map[string]bool, unset []string) map[string]string {
	out := make(map[string]string, len(stored)+len(incoming))
	for k, v := range stored {
		out[k] = v
	}
	for k, v := range incoming {
		if v == "" && sensitive[k] {
			continue
		}
		out[k] = v
	}
	for _, k := range unset {
		delete(out, k)
	}
	return out
}

// Uninstall 解绑 manifest 与 workspace,清相关 workspace_resources
// 不动云端;workspace 进入"反向漂移"状态等待用户跑 Plan+Apply 清理
// @Summary Uninstall manifest deployment
// @Description Unbind manifest from workspace and clear related workspace resources (does not destroy cloud resources)
// @Tags Manifest Deployments
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Param deployment_id path string true "Deployment ID"
// @Success 200 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 409 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/organizations/{org_id}/manifests/{id}/v2/deployments/{deployment_id}/uninstall [post]
// @Security BearerAuth
func (h *ManifestDeploymentsV2Handler) Uninstall(c *gin.Context) {
	deploymentID := c.Param("deployment_id")
	manifestID := c.Param("id")
	userID := c.GetString("user_id")
	if !h.manifestInAuthOrg(c, manifestID) {
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

	// 叠加目标 workspace 权限(workspace 藏在 deployment 记录里,需在此校验):
	// 显式 WORKSPACE_RESOURCES WRITE。
	if h.perm != nil && !h.perm.RequireWorkspaceResourcePermission(c, dep.WorkspaceID, "WORKSPACE_RESOURCES", "WRITE") {
		return // 403 已写
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
	writeManifestAudit(h.db, auditResourceManifestDeployment, "deployment.uninstall", userID, map[string]interface{}{
		"manifest_id":   manifestID,
		"deployment_id": deploymentID,
		"workspace_id":  dep.WorkspaceID,
		"version_id":    dep.VersionID,
	})
	c.JSON(http.StatusOK, gin.H{
		"deployment_id": deploymentID,
		"workspace_id":  dep.WorkspaceID,
		"hint":          "manifest uninstalled, workspace state may still contain leftover resources. Run Plan+Apply on the workspace to destroy them.",
	})
}

// VariablePreview 返回最终合并后的变量(结构化,每条带 sensitive)用于 upgrade 对话框预览。
// 响应: {"variables":[{"key","value","sensitive","variable_type","value_format",...}]}
// sensitive=true 的条目 value 恒为空串(前端不得预填)。
// @Summary Preview deployment variables
// @Description Preview merged variables with a per-variable sensitive flag; sensitive values are always empty
// @Tags Manifest Deployments
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Param deployment_id path string true "Deployment ID"
// @Param request body map[string]interface{} true "Varsets and variable_overrides for preview"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/organizations/{org_id}/manifests/{id}/v2/deployments/{deployment_id}/variable-preview [post]
// @Security BearerAuth
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

	manifestID := c.Param("id")
	if !h.manifestInAuthOrg(c, manifestID) {
		return
	}
	var dep models.ManifestDeployment
	if err := h.db.Where("id = ? AND manifest_id = ?", deploymentID, manifestID).First(&dep).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
		return
	}
	h.previewVariables(c, dep.WorkspaceID, req.Varsets, req.VariableOverrides)
}

// FirstInstallVariablePreview 首次安装前的变量预览(尚无 deployment)。
// 与 VariablePreview 共用 previewVariables;目标校验复用 Install 的 resolveInstallTarget:
// workspace 不属于本 org、version 不属于本 manifest 或为草稿 => 404。
// 响应同 VariablePreview: {"variables":[...]},sensitive 条目 value 恒为空串。
// @Summary Preview variables for a first install
// @Description Preview merged variables for installing a published version into a workspace (no deployment yet); sensitive values are always empty
// @Tags Manifest Deployments
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Param request body models.FirstInstallPreviewRequest true "Target workspace, version, varsets and overrides"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/organizations/{org_id}/manifests/{id}/v2/deployments/variable-preview [post]
// @Security BearerAuth
func (h *ManifestDeploymentsV2Handler) FirstInstallVariablePreview(c *gin.Context) {
	manifestID := c.Param("id")
	var req models.FirstInstallPreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if _, ok := h.resolveInstallTarget(c, manifestID, req.WorkspaceID, req.VersionID); !ok {
		return
	}
	h.previewVariables(c, req.WorkspaceID, req.Varsets, req.VariableOverrides)
}

// resolveInstallTarget 安装目标校验(Install 与首装预览共用):
//   - workspace 必须属于鉴权 org(WorkspaceService.EnsureWorkspaceInOrg);
//   - version 必须属于本 manifest 且已发布(非草稿)。
// 任一不满足 => 404(不区分不存在与跨 org)。已写响应时返回 false。
func (h *ManifestDeploymentsV2Handler) resolveInstallTarget(c *gin.Context, manifestID, workspaceID, versionID string) (models.ManifestVersion, bool) {
	var version models.ManifestVersion
	orgID, ok := middleware.AuthOrgID(c)
	if !ok || services.NewWorkspaceService(h.db).EnsureWorkspaceInOrg(workspaceID, orgID) != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return version, false
	}
	if err := h.db.Where("id = ? AND manifest_id = ?", versionID, manifestID).First(&version).Error; err != nil ||
		version.Version == "draft" || version.Version == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "version not found"})
		return version, false
	}
	return version, true
}

// previewVariables 两个预览接口的共用实现(路由层已校验 MANIFESTS READ + manifest 属于 org):
//   - 目标 workspace 的 WORKSPACE_VARIABLES READ(预览即读取其合并变量;仅 MANIFESTS READ 不够);
//   - varset 必须可挂载到目标 workspace;
//   - 敏感值恒为空串,带 sensitive 标记。
func (h *ManifestDeploymentsV2Handler) previewVariables(c *gin.Context, workspaceID string, varsets []models.DeploymentVarsetEntry, overrides map[string]string) {
	if h.perm == nil || !h.perm.RequireWorkspaceResourcePermission(c, workspaceID, "WORKSPACE_VARIABLES", "READ") {
		if h.perm == nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "permission middleware not configured"})
		}
		return // 403 已写
	}
	if !h.rejectUnmountableVarsets(c, workspaceID, varsets) {
		return
	}
	resolver := services.NewVariableResolutionService(h.db)
	extraIDs := make([]string, 0, len(varsets))
	for _, v := range varsets {
		extraIDs = append(extraIDs, v.VarsetID)
	}
	values, err := resolver.ResolveDisplayWithExtra(workspaceID, extraIDs, overrides)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"variables": values})
}

// GetWorkspaceManifestSummary 给 workspace 资源页/banner 用的轻量摘要
// GET /workspaces/:workspace_id/manifest-summary
//
// workspace 视角下,active manifest 软链接已在 workspaces 表;这个端点把
// deployment 与 manifest 的关键展示字段一次性返回,避免前端做 N 次反查。
// @Summary Get workspace manifest summary
// @Description Lightweight active-manifest summary for workspace resource page/banner
// @Tags Workspace
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workspaces/{id}/manifest-summary [get]
// @Security BearerAuth
func (h *ManifestDeploymentsV2Handler) GetWorkspaceManifestSummary(c *gin.Context) {
	// 路由用 :id(与 /workspaces/:id 前缀一致,避免 gin 通配符冲突 panic)
	workspaceID := c.Param("id")

	type result struct {
		WorkspaceID    string  `json:"workspace_id"`
		HasManifest    bool    `json:"has_manifest"`
		DeploymentID   string  `json:"deployment_id,omitempty"`
		VersionID      string  `json:"version_id,omitempty"`
		ActiveTag      string  `json:"active_tag,omitempty"`
		Subpath        *string `json:"subpath,omitempty"`
		ManifestID     string  `json:"manifest_id,omitempty"`
		ManifestName   string  `json:"manifest_name,omitempty"`
		OrgID          int     `json:"org_id,omitempty"`
		Status         string  `json:"status,omitempty"`
	}
	out := result{WorkspaceID: workspaceID, HasManifest: false}

	// 1. 拿 workspace 软链接
	type wsRow struct {
		ManifestDeploymentID *string
		ManifestActiveTag    *string
		ManifestSubpath      *string
	}
	var ws wsRow
	if err := h.db.Table("workspaces").
		Select("manifest_deployment_id, manifest_active_tag, manifest_subpath").
		Where("workspace_id = ?", workspaceID).
		Take(&ws).Error; err != nil {
		c.JSON(http.StatusOK, out) // workspace 不存在或无字段, 静默返回 has_manifest=false
		return
	}

	if ws.ManifestDeploymentID == nil || *ws.ManifestDeploymentID == "" {
		c.JSON(http.StatusOK, out)
		return
	}
	out.HasManifest = true
	out.DeploymentID = *ws.ManifestDeploymentID
	if ws.ManifestActiveTag != nil {
		out.ActiveTag = *ws.ManifestActiveTag
	}
	out.Subpath = ws.ManifestSubpath

	// 2. JOIN manifest_deployments → manifests 拿 manifest 名字与 org
	type joinRow struct {
		ManifestID   string
		ManifestName string
		VersionID    string
		OrgID        int
		Status       string
	}
	var jr joinRow
	if err := h.db.Table("manifest_deployments md").
		Select("md.manifest_id, m.name as manifest_name, md.version_id, m.organization_id as org_id, md.status").
		Joins("JOIN manifests m ON m.id = md.manifest_id").
		Where("md.id = ?", *ws.ManifestDeploymentID).
		Take(&jr).Error; err == nil {
		out.ManifestID = jr.ManifestID
		out.ManifestName = jr.ManifestName
		out.VersionID = jr.VersionID
		out.OrgID = jr.OrgID
		out.Status = jr.Status
	}

	c.JSON(http.StatusOK, out)
}

// VarsetReverseLookup 列出使用某 varset 的 active deployment
// GET /variable-sets/:varset_id/manifest-deployments
// @Summary List deployments using variable set
// @Description List active manifest deployments that reference a variable set
// @Tags Variable Set
// @Accept json
// @Produce json
// @Param varset_id path string true "Variable set ID"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/variable-sets/{varset_id}/manifest-deployments [get]
// @Security BearerAuth
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

// subpathExistsInVersion: 校验 subpath 直接下层(非递归)至少有一个 .tf 文件。
//
// 必须与执行/解析的 scope 语义一致: terraform 在 cd subpath 后只读该目录顶层 .tf,
// 不递归子目录。所以这里也只认 subpath 的直接子级 .tf —— 用 ParseManifestResources
// 同款 shouldParse 规则,避免"深层嵌套 .tf 让校验通过、实际 plan 目录却为空"。
func (h *ManifestDeploymentsV2Handler) subpathExistsInVersion(manifestID, versionID, subpath string) bool {
	var rows []models.ManifestFile
	if err := h.db.Select("path").
		Where("manifest_id = ? AND version_id = ?", manifestID, versionID).
		Where("path LIKE ?", "%.tf").
		Find(&rows).Error; err != nil {
		return false
	}
	sp := strings.TrimSuffix(subpath, "/")
	for _, r := range rows {
		if services.IsTopLevelTFUnderSubpath(r.Path, sp) {
			return true
		}
	}
	return false
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

// derefStr 解引用 *string,nil 返回空串
func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// ptrIfNonEmptyStr 空串 => nil,否则返回指针
func ptrIfNonEmptyStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
