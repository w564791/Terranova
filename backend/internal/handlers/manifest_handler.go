package handlers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"iac-platform/internal/domain/valueobject"
	"iac-platform/internal/manifestbundle"
	"iac-platform/internal/middleware"
	"iac-platform/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ManifestHandler 处理 Manifest 顶层 CRUD (List/Get/Create/Update/Delete/ExportZip)。
//
// 文件 / 版本 / 部署相关的写操作已经全部迁移到:
//   - manifest_files_handler.go    草稿与版本文件 CRUD
//   - manifest_versions_handler.go 版本发布/diff/导出
//   - manifest_deployments_v2_handler.go install/upgrade/uninstall
//
// 这里只剩组织级 manifest 自身的元数据 CRUD,以及前端 ManifestManagement 列表的"导出 ZIP"动作。
type ManifestHandler struct {
	db   *gorm.DB
	perm *middleware.IAMPermissionMiddleware // 计算 can_deploy;nil 时 can_deploy=false
}

func NewManifestHandler(db *gorm.DB, perm *middleware.IAMPermissionMiddleware) *ManifestHandler {
	return &ManifestHandler{db: db, perm: perm}
}

// callerCapabilities 计算调用者能力(列表顶层 capabilities 与列表/详情项的 can_*)。
//   - can_read / can_write / can_admin 取自路由上 MANIFESTS 检查得到的有效等级,不做二次评估;
//   - can_deploy 只看 WORKSPACE_RESOURCES WRITE(与 workspace 选择器同一判定),
//     与 MANIFESTS 等级无关 —— MANIFESTS READ 绝不推出 can_deploy。
//
// 评估失败时降级为 false(按钮隐藏;真正的部署接口仍会做服务端校验)。
func (h *ManifestHandler) callerCapabilities(c *gin.Context) models.ManifestCapabilities {
	level := middleware.EffectiveLevelFromContext(c)
	caps := models.ManifestCapabilities{
		CanRead:  level >= valueobject.PermissionLevelRead,
		CanWrite: level >= valueobject.PermissionLevelWrite,
		CanAdmin: level >= valueobject.PermissionLevelAdmin,
	}
	if h.perm != nil {
		ok, err := h.perm.HasWorkspaceCapability(c, valueobject.ResourceTypeWorkspaceResources, valueobject.PermissionLevelWrite)
		if err != nil {
			log.Printf("[Manifest] can_deploy evaluation failed: %v", err)
		}
		caps.CanDeploy = ok && err == nil
	}
	return caps
}

// applyCapabilities 把调用者能力写到单个 manifest 上(仅 list/get)。
func applyCapabilities(m *models.Manifest, caps models.ManifestCapabilities) {
	canWrite, canAdmin, canDeploy := caps.CanWrite, caps.CanAdmin, caps.CanDeploy
	m.CanWrite, m.CanAdmin, m.CanDeploy = &canWrite, &canAdmin, &canDeploy
}

// ========== ID 生成 (供 v2 versions / deployments handler 也调用) ==========

func generateRandomID() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 16)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
		time.Sleep(time.Nanosecond)
	}
	uuidStr := uuid.New().String()
	for i := 0; i < 16 && i < len(uuidStr); i++ {
		c := uuidStr[i]
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			b[i] = c
		} else if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

func generateManifestID() string           { return fmt.Sprintf("mf-%s", generateRandomID()) }
func generateManifestVersionID() string    { return fmt.Sprintf("mfv-%s", generateRandomID()) }
func generateManifestDeploymentID() string { return fmt.Sprintf("mfd-%s", generateRandomID()) }

// ========== Manifest CRUD ==========

// ListManifests lists manifests for an organization
// @Summary List manifests
// @Description List manifests under an organization with optional status filter and pagination
// @Tags Manifest
// @Accept json
// @Produce json
// @Param org_id path int true "Organization ID"
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Page size" default(20)
// @Param status query string false "Filter by status (draft, published, archived)"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/organizations/{org_id}/manifests [get]
// @Security BearerAuth
func (h *ManifestHandler) ListManifests(c *gin.Context) {
	orgIDStr := c.Param("org_id")
	orgID, err := strconv.Atoi(orgIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid organization ID"})
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := c.Query("status")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var manifests []models.Manifest
	var total int64

	query := h.db.Model(&models.Manifest{}).Where("organization_id = ?", orgID)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	query.Count(&total)

	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&manifests).Error; err != nil {
		_ = c.Error(fmt.Errorf("query failed: %w", err))
		return
	}

	caps := h.callerCapabilities(c)
	for i := range manifests {
		applyCapabilities(&manifests[i], caps)

		// 取最新已发布版本 (元数据,不取大字段)
		var latestVersion models.ManifestVersion
		if err := h.db.Select("id, manifest_id, version, created_by, created_at").
			Where("manifest_id = ? AND version <> ?", manifests[i].ID, "draft").
			Order("created_at DESC").First(&latestVersion).Error; err == nil {
			manifests[i].LatestVersion = &latestVersion
		}

		var deploymentCount int64
		h.db.Model(&models.ManifestDeployment{}).
			Where("manifest_id = ? AND status = ?", manifests[i].ID, "active").
			Count(&deploymentCount)
		manifests[i].DeploymentCount = int(deploymentCount)

		var user models.User
		if err := h.db.Select("username").Where("user_id = ?", manifests[i].CreatedBy).First(&user).Error; err == nil {
			manifests[i].CreatedByName = user.Username
		}
	}

	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	if manifests == nil {
		manifests = []models.Manifest{}
	}
	c.JSON(http.StatusOK, models.ManifestListResponse{
		Items:        manifests,
		Capabilities: &caps,
		Total:        total,
		Page:         page,
		PageSize:     pageSize,
		TotalPages:   totalPages,
	})
}

// GetManifest returns a single manifest by ID
// @Summary Get manifest
// @Description Get manifest details including latest version and deployment count
// @Tags Manifest
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/organizations/{org_id}/manifests/{id} [get]
// @Security BearerAuth
func (h *ManifestHandler) GetManifest(c *gin.Context) {
	orgID := c.Param("org_id")
	id := c.Param("id")

	var manifest models.Manifest
	if err := h.db.Where("id = ? AND organization_id = ?", id, orgID).First(&manifest).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Manifest not found"})
			return
		}
		_ = c.Error(fmt.Errorf("query failed: %w", err))
		return
	}

	var latestVersion models.ManifestVersion
	if err := h.db.Where("manifest_id = ? AND version <> ?", manifest.ID, "draft").
		Order("created_at DESC").First(&latestVersion).Error; err == nil {
		manifest.LatestVersion = &latestVersion
	}

	var deploymentCount int64
	h.db.Model(&models.ManifestDeployment{}).
		Where("manifest_id = ? AND status = ?", manifest.ID, "active").
		Count(&deploymentCount)
	manifest.DeploymentCount = int(deploymentCount)

	var user models.User
	if err := h.db.Select("username").Where("user_id = ?", manifest.CreatedBy).First(&user).Error; err == nil {
		manifest.CreatedByName = user.Username
	}

	applyCapabilities(&manifest, h.callerCapabilities(c))

	c.JSON(http.StatusOK, manifest)
}

// CreateManifest creates a new draft manifest
// @Summary Create manifest
// @Description Create a new manifest in draft status under the organization. source_type (immutable afterwards) is native (default: edited in the platform) or git (read-only GitHub source; publish = pick a commit). git requires git_repo_url (<GITHUB_URL>/<owner>/<repo>, no credentials) and github_installation_id (registered for this organization by an org admin, its account must own the repo); git_subpath optionally selects the bundle root directory. The repository is checked with a per-request installation token (single repo, contents:read). Errors: 400 invalid fields; 422 github_installation_not_registered / git_repo_not_accessible; 503 git_source_disabled (GitHub App not configured).
// @Tags Manifest
// @Accept json
// @Produce json
// @Param org_id path int true "Organization ID"
// @Param request body models.CreateManifestRequest true "Manifest create payload"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 409 {object} map[string]interface{}
// @Failure 422 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Failure 503 {object} map[string]interface{}
// @Router /api/v1/organizations/{org_id}/manifests [post]
// @Security BearerAuth
func (h *ManifestHandler) CreateManifest(c *gin.Context) {
	orgIDStr := c.Param("org_id")
	orgID, err := strconv.Atoi(orgIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid organization ID"})
		return
	}
	userID := c.GetString("user_id")

	var req models.CreateManifestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request parameters: " + err.Error()})
		return
	}

	var count int64
	h.db.Model(&models.Manifest{}).Where("organization_id = ? AND name = ?", orgID, req.Name).Count(&count)
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Manifest name already exists"})
		return
	}

	manifest := models.Manifest{
		ID:             generateManifestID(),
		OrganizationID: orgID,
		Name:           req.Name,
		Description:    req.Description,
		Status:         models.ManifestStatusDraft,
		SourceType:     models.ManifestSourceNative, // 创建后不可变
		CreatedBy:      userID,
	}
	switch req.SourceType {
	case "", models.ManifestSourceNative:
		if req.GitRepoURL != "" || req.GitRepo != "" || req.GitSubpath != "" || req.GitHubInstallationID != 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "git_* fields require source_type git"})
			return
		}
	case models.ManifestSourceGit:
		// repo on the GitHub host, installation registered for this org and
		// owning the repo, repo reachable with a scoped token
		if !validateGitManifestCreate(c, h.db, orgID, &req, &manifest) {
			return
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "source_type must be native or git"})
		return
	}

	// 新模型: 不再创建初始 ManifestVersion (草稿走 manifest_files.version_id IS NULL,按需懒创建)
	if err := h.db.Create(&manifest).Error; err != nil {
		_ = c.Error(fmt.Errorf("creation failed: %w", err))
		return
	}

	audit := map[string]interface{}{
		"manifest_id":     manifest.ID,
		"organization_id": orgID,
		"name":            manifest.Name,
		"source_type":     manifest.SourceType,
	}
	if manifest.GitRepoURL != nil {
		audit["git_repo_url"] = *manifest.GitRepoURL
		audit["github_installation_id"] = *manifest.GitHubInstallationID
	}
	writeManifestAudit(h.db, auditResourceManifest, "manifest.create", userID, audit)

	c.JSON(http.StatusCreated, manifest)
}

// UpdateManifest updates manifest metadata
// @Summary Update manifest
// @Description Update manifest name, description, or status
// @Tags Manifest
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Param request body models.UpdateManifestRequest true "Manifest update payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 409 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/organizations/{org_id}/manifests/{id} [put]
// @Security BearerAuth
func (h *ManifestHandler) UpdateManifest(c *gin.Context) {
	orgID := c.Param("org_id")
	id := c.Param("id")

	var manifest models.Manifest
	if err := h.db.Where("id = ? AND organization_id = ?", id, orgID).First(&manifest).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Manifest not found"})
			return
		}
		_ = c.Error(fmt.Errorf("query failed: %w", err))
		return
	}

	var req models.UpdateManifestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request parameters: " + err.Error()})
		return
	}

	// source_type 创建后不可变(spec §1):只接受与当前值相同的回显
	if req.SourceType != "" && req.SourceType != manifest.SourceType {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source_type is immutable after creation"})
		return
	}

	if req.Name != "" && req.Name != manifest.Name {
		var count int64
		h.db.Model(&models.Manifest{}).Where("organization_id = ? AND name = ? AND id != ?", orgID, req.Name, id).Count(&count)
		if count > 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "Manifest name already exists"})
			return
		}
		manifest.Name = req.Name
	}

	if req.Description != "" {
		manifest.Description = req.Description
	}

	if req.Status != "" {
		// 归档 / 取消归档与删除同级: 需 MANIFESTS ADMIN(路由只保证 WRITE)。
		if isArchiveTransition(manifest.Status, req.Status) &&
			middleware.EffectiveLevelFromContext(c) < valueobject.PermissionLevelAdmin {
			c.JSON(http.StatusForbidden, gin.H{
				"code":           403,
				"message":        "Permission denied",
				"deny_reason":    "archiving a manifest requires MANIFESTS ADMIN",
				"required_level": "ADMIN",
			})
			return
		}
		manifest.Status = req.Status
	}

	if err := h.db.Save(&manifest).Error; err != nil {
		_ = c.Error(fmt.Errorf("update failed: %w", err))
		return
	}

	c.JSON(http.StatusOK, manifest)
}

// isArchiveTransition 状态变更是否进入或离开 archived。
func isArchiveTransition(from, to string) bool {
	return from != to && (to == models.ManifestStatusArchived || from == models.ManifestStatusArchived)
}

// DeleteManifest deletes a manifest and its related data
// @Summary Delete manifest
// @Description Delete a manifest (blocked if active deployments exist)
// @Tags Manifest
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Success 204 "No Content"
// @Failure 404 {object} map[string]interface{}
// @Failure 409 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/organizations/{org_id}/manifests/{id} [delete]
// @Security BearerAuth
func (h *ManifestHandler) DeleteManifest(c *gin.Context) {
	orgID := c.Param("org_id")
	id := c.Param("id")

	var manifest models.Manifest
	if err := h.db.Where("id = ? AND organization_id = ?", id, orgID).First(&manifest).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Manifest not found"})
			return
		}
		_ = c.Error(fmt.Errorf("query failed: %w", err))
		return
	}

	// 阻止有 active 部署的删除
	var activeCount int64
	h.db.Model(&models.ManifestDeployment{}).
		Where("manifest_id = ? AND status = ?", id, "active").Count(&activeCount)
	if activeCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("Manifest has %d active deployments, please uninstall them first", activeCount)})
		return
	}

	// 显式清理: manifest_files (按 manifest_id 自带的非 FK 关系)、versions、deployments(已 uninstall 的)
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("manifest_id = ?", id).Delete(&models.ManifestFile{}).Error; err != nil {
			return err
		}
		if err := tx.Where("manifest_id = ?", id).Delete(&models.ManifestDeployment{}).Error; err != nil {
			return err
		}
		if err := tx.Where("manifest_id = ?", id).Delete(&models.ManifestVersion{}).Error; err != nil {
			return err
		}
		return tx.Delete(&manifest).Error
	}); err != nil {
		_ = c.Error(fmt.Errorf("deletion failed: %w", err))
		return
	}

	writeManifestAudit(h.db, auditResourceManifest, "manifest.delete", c.GetString("user_id"), map[string]interface{}{
		"manifest_id":     id,
		"organization_id": orgID,
		"name":            manifest.Name,
	})

	c.Status(http.StatusNoContent)
}

// ========== 导出 ZIP (列表页 "Export ZIP" 动作) ==========
//
// 老版本导出画布 manifest.json + 生成的 .tf,新模型直接走 manifest_files:
//   - 没指定 version_id: 取最新已发布版本
//   - 指定 version_id: 用该版本的文件
//   - 都没有: 用 owner_user_id=current 的草稿(version_id IS NULL)

// ExportManifestZip exports manifest files as a ZIP archive
// @Summary Export manifest ZIP
// @Description Export published version or current user draft as a ZIP of files
// @Tags Manifest
// @Accept json
// @Produce application/zip
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Param version_id query string false "Version ID to export (defaults to latest published, else draft)"
// @Success 200 {file} binary "ZIP archive"
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/organizations/{org_id}/manifests/{id}/export-zip [get]
// @Security BearerAuth
func (h *ManifestHandler) ExportManifestZip(c *gin.Context) {
	orgID := c.Param("org_id")
	manifestID := c.Param("id")
	versionID := c.Query("version_id")
	userID := c.GetString("user_id")

	var manifest models.Manifest
	if err := h.db.Where("id = ? AND organization_id = ?", manifestID, orgID).First(&manifest).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Manifest not found"})
			return
		}
		_ = c.Error(fmt.Errorf("query failed: %w", err))
		return
	}

	// 有已发布版本 => 只导出版本的不可变 bundle(指定 version_id 或最新版本),绝不读草稿;
	// 仅当 manifest 尚无任何已发布版本时,才退回调用者自己的草稿。
	var label string
	var files []manifestbundle.File
	if versionID == "" {
		var latest models.ManifestVersion
		if err := h.db.Where("manifest_id = ? AND version <> ?", manifestID, "draft").
			Order("created_at DESC").First(&latest).Error; err == nil {
			versionID = latest.ID
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			_ = c.Error(fmt.Errorf("query failed: %w", err))
			return
		}
	}
	if versionID != "" {
		bundle, ok := openVersionBundle(c, h.db, manifestID, versionID)
		if !ok {
			return
		}
		var version models.ManifestVersion
		if err := h.db.Select("version").Where("id = ?", versionID).First(&version).Error; err != nil {
			_ = c.Error(fmt.Errorf("query failed: %w", err))
			return
		}
		label, files = version.Version, bundle.Files
	} else if userID != "" {
		draft, err := manifestbundle.NativeDraft{DB: h.db, ManifestID: manifestID, OwnerUserID: userID}.ReadFiles(c.Request.Context())
		if err != nil {
			_ = c.Error(fmt.Errorf("query files failed: %w", err))
			return
		}
		label, files = "draft", draft
	} else {
		c.JSON(http.StatusNotFound, gin.H{"error": "No version or draft to export"})
		return
	}
	if len(files) == 0 {
		// 私有草稿模型下,无 published 版本时只能导出"自己的"草稿;若调用者没有草稿
		// (常见于他人创建、尚未发布的 manifest),给出可操作的提示而非裸 404。
		msg := "No files to export"
		if label == "draft" {
			msg = "this manifest has no published version and you have no draft of it yet; open it in the editor to create a draft, or publish a version first"
		}
		c.JSON(http.StatusNotFound, gin.H{"error": msg})
		return
	}

	buf, err := zipFiles(files)
	if err != nil {
		_ = c.Error(fmt.Errorf("failed to build ZIP: %w", err))
		return
	}

	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.zip"`, manifest.Name, label))
	c.Data(http.StatusOK, "application/zip", buf.Bytes())
}
