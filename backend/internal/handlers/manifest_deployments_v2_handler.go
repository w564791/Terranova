package handlers

import (
	"sort"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"iac-platform/internal/application/service"
	"iac-platform/internal/manifestbundle"
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
// @Description List all deployments for a manifest. Each deployment carries overrides ([]services.OverrideView: key, sensitive, has_value, value); raw variable_overrides are never returned. value is present only for non-sensitive keys when the caller has WORKSPACE_VARIABLES READ on the workspace; a deployment whose sensitive_keys is not yet computed treats every key as sensitive.
// @Tags Manifest Deployments
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} middleware.InternalErrorResponse
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
		_ = c.Error(errors.New("workspace list authorization context is missing"))
		return
	}

	query := h.db.Where("manifest_id = ?", manifestID)
	if !access.FullOrganization {
		// 空切片 => 空结果(IN () 由 gorm 渲染为 NULL 条件,不会退化成全量)
		if len(access.WorkspaceIDs) == 0 {
			c.JSON(http.StatusOK, gin.H{"deployments": []deploymentView{}})
			return
		}
		query = query.Where("workspace_id IN ?", access.WorkspaceIDs)
	}
	rows := make([]models.ManifestDeployment, 0)
	if err := query.Order("created_at DESC").Find(&rows).Error; err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deployments": h.deploymentViews(c, rows)})
}

// deploymentView 部署的对外形态:variable_overrides 不直接输出,统一经
// services.RedactOverrides 输出 overrides。
type deploymentView struct {
	models.ManifestDeployment
	Overrides []services.OverrideView `json:"overrides"`
}

// canReadOverrideValues 覆盖值可见性:与变量预览同一检查(目标 workspace
// WORKSPACE_VARIABLES READ),但不写响应;perm 未配置 => 不可见(失败关闭)。
func (h *ManifestDeploymentsV2Handler) canReadOverrideValues(c *gin.Context, workspaceID string) bool {
	return h.perm.HasWorkspaceResourcePermission(c, workspaceID, "WORKSPACE_VARIABLES", "READ")
}

// redactOverrides 是返回 ManifestDeployment 的唯一出口(详情、列表)。
func (h *ManifestDeploymentsV2Handler) redactOverrides(c *gin.Context, d models.ManifestDeployment, canRead bool) deploymentView {
	return deploymentView{
		ManifestDeployment: d,
		Overrides:          services.RedactOverrides(services.ParseOverrides(d.VariableOverrides), d.SensitiveKeys, canRead),
	}
}

func (h *ManifestDeploymentsV2Handler) deploymentViews(c *gin.Context, rows []models.ManifestDeployment) []deploymentView {
	canRead := map[string]bool{}
	out := make([]deploymentView, 0, len(rows))
	for _, d := range rows {
		ok, cached := canRead[d.WorkspaceID]
		if !cached {
			ok = h.canReadOverrideValues(c, d.WorkspaceID)
			canRead[d.WorkspaceID] = ok
		}
		out = append(out, h.redactOverrides(c, d, ok))
	}
	return out
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
		_ = c.Error(err)
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
		_ = c.Error(err)
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
// @Description Get deployment detail including linked variable sets. Overrides are returned redacted as overrides ([]services.OverrideView), never as raw variable_overrides.
// @Tags Manifest Deployments
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Param deployment_id path string true "Deployment ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} middleware.InternalErrorResponse
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
			_ = c.Error(err)
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
	c.JSON(http.StatusOK, gin.H{"deployment": h.redactOverrides(c, d, h.canReadOverrideValues(c, d.WorkspaceID)), "varsets": varsets})
}

// Install 把指定 published version 装到空 workspace
// @Summary Install manifest deployment
// @Description Install a published version onto an empty workspace. A version without a valid bundle (bundle_hash null) is rejected with 409 bundle_republish_required; so is a version whose stored files no longer match bundle_hash (reason hash_mismatch, recorded on the version and sticky until a new version is published).
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
// @Failure 500 {object} middleware.InternalErrorResponse
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
	// 首装带覆盖或 varset 即变更目标 workspace 的变量
	installVarsets := derefVarsets(req.Varsets) // 首装:缺省与 [] 等价
	if !h.requireVariablesWriteIf(c, req.WorkspaceID, len(req.VariableOverrides) > 0 || len(installVarsets) > 0) {
		return
	}
	if !h.rejectUnmountableVarsets(c, req.WorkspaceID, installVarsets) {
		return
	}

	// 目标校验(与首装变量预览共用):workspace 属于本 org、version 属于本 manifest 且已发布
	version, bundle, ok := h.resolveInstallTarget(c, manifestID, req.WorkspaceID, req.VersionID, "install")
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
		if !subpathExistsInBundle(bundle, effectiveSubpath) {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("subpath %q not found in version %s (must contain at least one .tf)", effectiveSubpath, req.VersionID)})
			return
		}
	}

	// 拉 manifest_files 浅 parse(按 effective subpath)
	subpathPtr := ptrIfNonEmptyStr(effectiveSubpath)
	resourceRefs := shallowParseBundleResources(bundle, subpathPtr)

	deploymentID := generateManifestDeploymentID()

	// 敏感 key = 版本 variable 块 sensitive + 同名敏感变量 + 请求里标记的 sensitive
	sensitive, err := services.ComputeDeploymentSensitiveKeys(h.db, []string{req.VersionID}, req.WorkspaceID, varsetIDsOf(installVarsets))
	if err != nil {
		_ = c.Error(err)
		return
	}
	for k := range req.VariableOverrides.SensitiveFlags() {
		sensitive[k] = true
	}
	// 敏感 key 的空串是掩码占位,不写入(与 upgrade 同一 mergeOverrides 规则)
	overridesJSON, _ := json.Marshal(mergeOverrides(nil, req.VariableOverrides.Values(), sensitive, nil))
	sensitiveJSON := services.EncodeSensitiveKeys(sensitive)

	err = h.db.Transaction(func(tx *gorm.DB) error {
		// 1. 写 manifest_deployments
		now := time.Now()
		dep := models.ManifestDeployment{
			ID:                deploymentID,
			ManifestID:        manifestID,
			VersionID:         req.VersionID,
			WorkspaceID:       req.WorkspaceID,
			VariableOverrides: overridesJSON,
			SensitiveKeys:     sensitiveJSON,
			Status:            models.DeploymentStatusActive,
			DeployedBy:        userID,
			DeployedAt:        &now,
		}
		if err := tx.Create(&dep).Error; err != nil {
			return err
		}

		// 2. 写 varsets
		for _, v := range installVarsets {
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
		_ = c.Error(err)
		return
	}

	writeManifestAudit(h.db, auditResourceManifestDeployment, "deployment.install", userID, map[string]interface{}{
		"manifest_id":     manifestID,
		"deployment_id":   deploymentID,
		"workspace_id":    req.WorkspaceID,
		"version_id":      req.VersionID,
		"version":         version.Version,
		"resources_added": len(resourceRefs),
		"varset_count":    len(installVarsets),
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
// @Description Switch deployment version and varsets; reconcile workspace resources. varsets absent keeps the attached varsets, [] clears them, a list replaces them. The target version must have a valid, intact bundle (409 bundle_republish_required, reason hash_mismatch when its files no longer match); upgrading away from a version without one is allowed.
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
// @Failure 500 {object} middleware.InternalErrorResponse
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
	// 覆盖 / unset / varset 列表(按生效顺序比较)有变化 => 还需 WORKSPACE_VARIABLES WRITE;
	// 原样回传 varset、只换版本沿用上面的规则。
	// varsets 缺省(nil)= 保持不变,不算变化、不重写;[] = 清空;非空 = 替换。
	replaceVarsets := req.Varsets != nil
	effectiveVarsets, err := h.storedVarsetEntries(dep.ID)
	if err != nil {
		_ = c.Error(err)
		return
	}
	variablesChanged := len(req.VariableOverrides) > 0 || len(req.UnsetKeys) > 0
	if replaceVarsets {
		if !variablesChanged {
			changed, err := h.deploymentVarsetsChanged(dep.ID, *req.Varsets)
			if err != nil {
				_ = c.Error(err)
				return
			}
			variablesChanged = changed
		}
		effectiveVarsets = *req.Varsets
	}
	if !h.requireVariablesWriteIf(c, dep.WorkspaceID, variablesChanged) {
		return
	}
	if replaceVarsets && !h.rejectUnmountableVarsets(c, dep.WorkspaceID, *req.Varsets) {
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

	// 只校验目标版本的 bundle(从无合法 bundle 的版本升级到合法版本是允许的)
	targetBundle, ok := h.openDeployableBundle(c, manifestID, req.TargetVersionID, "upgrade_target")
	if !ok {
		return
	}

	// 拉新版本的 resource refs
	var ws models.Workspace
	h.db.Where("workspace_id = ?", dep.WorkspaceID).First(&ws)
	newRefs := shallowParseBundleResources(targetBundle, ws.ManifestSubpath)

	// 覆盖合并(修复 upgrade 清空已有覆盖):缺省 key / 敏感空占位均保留原值,仅 unset_keys 删除
	overrides, sensitive, err := h.mergeDeploymentOverrides(dep, []string{req.TargetVersionID}, effectiveVarsets, req.VariableOverrides, req.UnsetKeys)
	if err != nil {
		_ = c.Error(err)
		return
	}
	overridesJSON, _ := json.Marshal(overrides)

	err = h.db.Transaction(func(tx *gorm.DB) error {
		// 1. 更新 deployment
		if err := tx.Model(&dep).Updates(map[string]interface{}{
			"version_id":         req.TargetVersionID,
			"variable_overrides": overridesJSON,
			"sensitive_keys":     services.EncodeSensitiveKeys(sensitive.stored),
			"deployed_by":        userID,
			"deployed_at":        time.Now(),
		}).Error; err != nil {
			return err
		}

		// 2. 重写 varsets(仅当请求带了 varsets;缺省保持原样)
		if replaceVarsets {
			if err := tx.Where("deployment_id = ?", deploymentID).Delete(&models.ManifestDeploymentVarset{}).Error; err != nil {
				return err
			}
		}
		for _, v := range derefVarsets(req.Varsets) {
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
		_ = c.Error(err)
		return
	}
	writeManifestAudit(h.db, auditResourceManifestDeployment, "deployment.upgrade", userID, map[string]interface{}{
		"manifest_id":     manifestID,
		"deployment_id":   deploymentID,
		"workspace_id":    dep.WorkspaceID,
		"old_version_id":  dep.VersionID,
		"new_version_id":  req.TargetVersionID,
		"new_version":     version.Version,
		"varset_count":    len(effectiveVarsets),
	})
	c.JSON(http.StatusOK, gin.H{
		"deployment_id": deploymentID,
		"version":       version.Version,
	})
}

// overrideSensitivity 一次合并的敏感判定。
//   - stored:写回 sensitive_keys 的集合(粘滞:已存集合只增不减);
//   - display:预览展示 / 空占位判定用的集合 = stored ∪(sensitive_keys 为 NULL 时的全部已存覆盖 key),
//     NULL 行未计算前一律不展示已存值,但不会因此把这些 key 永久标成敏感。
type overrideSensitivity struct {
	stored  map[string]bool
	display map[string]bool
}

// mergeDeploymentOverrides 计算 upgrade 写回(以及预览展示)的覆盖与敏感 key,
// upgrade 与 VariablePreview 共用,保证预览与 upgrade 写入完全一致。
//
// 敏感判定 = ComputeDeploymentSensitiveKeys(当前版本 ∪ 目标版本,已存 varsets ∪ 请求 varsets)
//   - 已存 sensitive_keys 非 NULL:并上已存集合(粘滞)与请求里的 sensitive 标记;
//   - 已存 sensitive_keys 为 NULL:只认版本与 varset 的判定,忽略请求标记。
func (h *ManifestDeploymentsV2Handler) mergeDeploymentOverrides(
	dep models.ManifestDeployment, targetVersionIDs []string, varsets []models.DeploymentVarsetEntry,
	incoming models.OverrideInputs, unset []string,
) (map[string]string, overrideSensitivity, error) {
	stored := services.ParseOverrides(dep.VariableOverrides)
	var varsetIDs []string
	if err := h.db.Model(&models.ManifestDeploymentVarset{}).
		Where("deployment_id = ?", dep.ID).Pluck("varset_id", &varsetIDs).Error; err != nil {
		return nil, overrideSensitivity{}, fmt.Errorf("load deployment varsets: %w", err)
	}
	varsetIDs = append(varsetIDs, varsetIDsOf(varsets)...)
	computed, err := services.ComputeDeploymentSensitiveKeys(h.db, append([]string{dep.VersionID}, targetVersionIDs...), dep.WorkspaceID, varsetIDs)
	if err != nil {
		return nil, overrideSensitivity{}, err
	}
	sens := overrideSensitivity{stored: computed, display: map[string]bool{}}
	storedKeys, known := services.ParseSensitiveKeys(dep.SensitiveKeys)
	if known {
		for k := range storedKeys {
			sens.stored[k] = true
		}
		for k := range incoming.SensitiveFlags() {
			sens.stored[k] = true
		}
	}
	for k := range sens.stored {
		sens.display[k] = true
	}
	if !known {
		for k := range stored {
			sens.display[k] = true
		}
	}
	return mergeOverrides(stored, incoming.Values(), sens.display, unset), sens, nil
}

// requireVariablesWriteIf: changed 时要求目标 workspace WORKSPACE_VARIABLES WRITE
// (与 WORKSPACE_RESOURCES 同一检查实现;403 已写则返回 false)。
func (h *ManifestDeploymentsV2Handler) requireVariablesWriteIf(c *gin.Context, workspaceID string, changed bool) bool {
	if !changed || h.perm == nil {
		return true
	}
	return h.perm.RequireWorkspaceResourcePermission(c, workspaceID, "WORKSPACE_VARIABLES", "WRITE")
}

// deploymentVarsetsChanged 比较请求 varset 列表与已存列表的生效形态:
// 已存行按 priority ASC 读出(同 priority 按写入顺序,即当时请求顺序),请求列表按
// priority 稳定排序后逐项比较 (varset_id, priority)。集合、priority 或同级顺序
// 任一不同即变化。只在请求带了 varsets 时调用(缺省 = 保持不变,不算变化)。
func (h *ManifestDeploymentsV2Handler) deploymentVarsetsChanged(deploymentID string, req []models.DeploymentVarsetEntry) (bool, error) {
	var stored []models.ManifestDeploymentVarset
	if err := h.db.Where("deployment_id = ?", deploymentID).
		Order("priority ASC, id ASC").Find(&stored).Error; err != nil {
		return false, err
	}
	if len(stored) != len(req) {
		return true, nil
	}
	want := make([]models.DeploymentVarsetEntry, len(req))
	copy(want, req)
	sort.SliceStable(want, func(i, j int) bool { return want[i].Priority < want[j].Priority })
	for i := range want {
		if want[i].VarsetID != stored[i].VarsetID || want[i].Priority != stored[i].Priority {
			return true, nil
		}
	}
	return false, nil
}

// storedVarsetEntries 部署已挂的 varset,按生效顺序(priority ASC,同级按写入顺序)。
func (h *ManifestDeploymentsV2Handler) storedVarsetEntries(deploymentID string) ([]models.DeploymentVarsetEntry, error) {
	var rows []models.ManifestDeploymentVarset
	if err := h.db.Where("deployment_id = ?", deploymentID).Order("priority ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load deployment varsets: %w", err)
	}
	out := make([]models.DeploymentVarsetEntry, len(rows))
	for i, r := range rows {
		out[i] = models.DeploymentVarsetEntry{VarsetID: r.VarsetID, Priority: r.Priority}
	}
	return out, nil
}

// derefVarsets nil 指针 => 空列表。
func derefVarsets(p *[]models.DeploymentVarsetEntry) []models.DeploymentVarsetEntry {
	if p == nil {
		return nil
	}
	return *p
}

func varsetIDsOf(entries []models.DeploymentVarsetEntry) []string {
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.VarsetID)
	}
	return ids
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
// @Description Unbind manifest from workspace and clear related workspace resources (does not destroy cloud resources and creates no task). Not blocked when the deployed version has no valid bundle: the bundle is never executed after uninstall, because the follow-up workspace Plan+Apply no longer loads manifest files (the executor refuses to run any version whose bundle_hash is NULL, failing the task with "bundle_republish_required: <reason>").
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
// @Failure 500 {object} middleware.InternalErrorResponse
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
		_ = c.Error(err)
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
// @Description Preview merged variables with a per-variable sensitive flag; sensitive values are always empty. The previewed version (target, else current) must have a valid bundle (409 bundle_republish_required).
// @Tags Manifest Deployments
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Param deployment_id path string true "Deployment ID"
// @Param request body models.DeploymentPreviewRequest true "Optional target version, varsets, overrides and unset_keys; stored overrides are merged as in upgrade"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 409 {object} handlers.BundleRepublishRequiredResponse
// @Failure 500 {object} middleware.InternalErrorResponse
// @Router /api/v1/organizations/{org_id}/manifests/{id}/v2/deployments/{deployment_id}/variable-preview [post]
// @Security BearerAuth
func (h *ManifestDeploymentsV2Handler) VariablePreview(c *gin.Context) {
	deploymentID := c.Param("deployment_id")

	var req models.DeploymentPreviewRequest
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
	var targets []string
	if req.TargetVersionID != "" {
		var n int64
		h.db.Model(&models.ManifestVersion{}).Where("id = ? AND manifest_id = ?", req.TargetVersionID, manifestID).Count(&n)
		if n == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target_version_id"})
			return
		}
		targets = append(targets, req.TargetVersionID)
	}
	previewed := dep.VersionID
	if req.TargetVersionID != "" {
		previewed = req.TargetVersionID
	}
	if _, ok := h.openDeployableBundle(c, manifestID, previewed, "deployment_preview"); !ok {
		return
	}
	// varsets 与 upgrade 同义:缺省 = 部署已挂的 varset
	previewVarsets, err := h.storedVarsetEntries(dep.ID)
	if err != nil {
		_ = c.Error(err)
		return
	}
	if req.Varsets != nil {
		previewVarsets = *req.Varsets
	}
	// 与 upgrade 同一合并:已存覆盖 + 本次覆盖 - unset_keys,敏感值保持空
	h.previewVariables(c, dep.WorkspaceID, previewVarsets, func() (map[string]string, map[string]bool, error) {
		merged, sens, err := h.mergeDeploymentOverrides(dep, targets, previewVarsets, req.VariableOverrides, req.UnsetKeys)
		return merged, sens.display, err
	})
}

// FirstInstallVariablePreview 首次安装前的变量预览(尚无 deployment)。
// 与 VariablePreview 共用 previewVariables;目标校验复用 Install 的 resolveInstallTarget:
// workspace 不属于本 org、version 不属于本 manifest 或为草稿 => 404。
// 响应同 VariablePreview: {"variables":[...]},sensitive 条目 value 恒为空串。
// @Summary Preview variables for a first install
// @Description Preview merged variables for installing a published version into a workspace (no deployment yet); sensitive values are always empty. The version must have a valid bundle (409 bundle_republish_required).
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
// @Failure 409 {object} handlers.BundleRepublishRequiredResponse
// @Failure 500 {object} middleware.InternalErrorResponse
// @Router /api/v1/organizations/{org_id}/manifests/{id}/v2/deployments/variable-preview [post]
// @Security BearerAuth
func (h *ManifestDeploymentsV2Handler) FirstInstallVariablePreview(c *gin.Context) {
	manifestID := c.Param("id")
	var req models.FirstInstallPreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if _, _, ok := h.resolveInstallTarget(c, manifestID, req.WorkspaceID, req.VersionID, "first_install_preview"); !ok {
		return
	}
	h.previewVariables(c, req.WorkspaceID, req.Varsets, func() (map[string]string, map[string]bool, error) {
		sensitive, err := services.ComputeDeploymentSensitiveKeys(h.db, []string{req.VersionID}, req.WorkspaceID, varsetIDsOf(req.Varsets))
		if err != nil {
			return nil, nil, err
		}
		for k := range req.VariableOverrides.SensitiveFlags() {
			sensitive[k] = true
		}
		return req.VariableOverrides.Values(), sensitive, nil
	})
}

// resolveInstallTarget 安装目标校验(Install 与首装预览共用):
//   - workspace 必须属于鉴权 org(WorkspaceService.EnsureWorkspaceInOrg);
//   - version 必须属于本 manifest 且已发布(非草稿)。
// 任一不满足 => 404(不区分不存在与跨 org)。已写响应时返回 false。
func (h *ManifestDeploymentsV2Handler) resolveInstallTarget(c *gin.Context, manifestID, workspaceID, versionID, use string) (models.ManifestVersion, *manifestbundle.Bundle, bool) {
	var version models.ManifestVersion
	orgID, ok := middleware.AuthOrgID(c)
	if !ok || services.NewWorkspaceService(h.db).EnsureWorkspaceInOrg(workspaceID, orgID) != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return version, nil, false
	}
	if err := h.db.Where("id = ? AND manifest_id = ?", versionID, manifestID).First(&version).Error; err != nil ||
		version.Version == "draft" || version.Version == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "version not found"})
		return version, nil, false
	}
	bundle, ok := h.openDeployableBundle(c, manifestID, versionID, use)
	return version, bundle, ok
}

// openDeployableBundle 打开版本的不可变 bundle,供真正使用版本的部署路径
// (install / upgrade 目标 / 两个预览;use 标明是哪一个)。这里是唯一做完整性校验的
// 请求路径(只读接口不重算哈希):
//   - 版本没有合法 bundle(bundle_hash NULL)=> 409 bundle_republish_required,
//     reason 为规则名与路径,或已记录的 hash_mismatch(粘滞,不再重算);
//   - 重算哈希与 bundle_hash 不一致 => 记录 hash_mismatch(尽力而为)、WARN 安全日志与
//     审计行(manifest_id / version_id / request_id),同样 409;
//   - 版本不存在 => 404;数据库错误 => 500。
func (h *ManifestDeploymentsV2Handler) openDeployableBundle(c *gin.Context, manifestID, versionID, use string) (*manifestbundle.Bundle, bool) {
	ctx := c.Request.Context()
	bundle, err := manifestbundle.OpenVersion(ctx, h.db, manifestID, versionID)
	if err == nil {
		err = bundle.RequireValid()
	}
	if err == nil {
		err = manifestbundle.VerifyForUse(ctx, h.db, bundle, manifestbundle.MismatchEvent{
			ManifestID: manifestID, VersionID: versionID, Source: use,
			RequestID: c.GetString(middleware.RequestIDContextKey), UserID: c.GetString("user_id"),
		})
	}
	var invalid *manifestbundle.InvalidError
	switch {
	case err == nil:
		return bundle, true
	case errors.As(err, &invalid):
		c.JSON(http.StatusConflict, BundleRepublishRequiredResponse{
			Error:     "this version has no valid bundle; please republish it",
			Code:      "bundle_republish_required",
			VersionID: versionID,
			Reason:    invalid.Reason,
		})
	case errors.Is(err, manifestbundle.ErrVersionNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "version not found"})
	default:
		_ = c.Error(err)
	}
	return nil, false
}

// previewVariables 两个预览接口的共用实现(路由层已校验 MANIFESTS READ + manifest 属于 org):
//   - 目标 workspace 的 WORKSPACE_VARIABLES READ(预览即读取其合并变量;仅 MANIFESTS READ 不够);
//   - varset 必须可挂载到目标 workspace;
//   - 敏感值恒为空串,带 sensitive 标记。
//
// prepare 在权限与可挂载校验之后才执行,返回要叠加的覆盖与部署层敏感 key。
func (h *ManifestDeploymentsV2Handler) previewVariables(c *gin.Context, workspaceID string, varsets []models.DeploymentVarsetEntry,
	prepare func() (overrides map[string]string, sensitive map[string]bool, err error)) {
	if h.perm == nil || !h.perm.RequireWorkspaceResourcePermission(c, workspaceID, "WORKSPACE_VARIABLES", "READ") {
		if h.perm == nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "permission middleware not configured"})
		}
		return // 403 已写
	}
	if !h.rejectUnmountableVarsets(c, workspaceID, varsets) {
		return
	}
	overrides, sensitive, err := prepare()
	if err != nil {
		_ = c.Error(err)
		return
	}
	resolver := services.NewVariableResolutionService(h.db)
	extraIDs := make([]string, 0, len(varsets))
	for _, v := range varsets {
		extraIDs = append(extraIDs, v.VarsetID)
	}
	values, err := resolver.ResolveDisplayWithExtra(workspaceID, extraIDs, overrides)
	if err != nil {
		_ = c.Error(err)
		return
	}
	// 部署层敏感 key(版本 variable 块 / sensitive_keys / 请求标记)同样不出值
	for i := range values {
		if sensitive[values[i].Key] {
			values[i].Sensitive = true
			values[i].Value = ""
		}
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
// @Failure 500 {object} middleware.InternalErrorResponse
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
		_ = c.Error(err)
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

// subpathExistsInBundle: 校验 subpath 直接下层(非递归)至少有一个 .tf 文件。
//
// 必须与执行/解析的 scope 语义一致: terraform 在 cd subpath 后只读该目录顶层 .tf,
// 不递归子目录。所以这里也只认 subpath 的直接子级 .tf —— 用 ParseManifestResources
// 同款 shouldParse 规则,避免"深层嵌套 .tf 让校验通过、实际 plan 目录却为空"。
func subpathExistsInBundle(b *manifestbundle.Bundle, subpath string) bool {
	sp := strings.TrimSuffix(subpath, "/")
	for _, f := range b.Files {
		if strings.HasSuffix(f.Path, ".tf") && services.IsTopLevelTFUnderSubpath(f.Path, sp) {
			return true
		}
	}
	return false
}

// shallowParseBundleResources 浅 parse 版本 bundle 的 resource/module refs
func shallowParseBundleResources(b *manifestbundle.Bundle, subpath *string) []services.ManifestResourceRef {
	sp := ""
	if subpath != nil {
		sp = *subpath
	}
	return services.ParseManifestResources(b.Scope(), sp)
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
