package handlers

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"time"

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
	db *gorm.DB
}

func NewManifestHandler(db *gorm.DB) *ManifestHandler {
	return &ManifestHandler{db: db}
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
		return
	}

	for i := range manifests {
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

	c.JSON(http.StatusOK, models.ManifestListResponse{
		Items:      manifests,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	})
}

func (h *ManifestHandler) GetManifest(c *gin.Context) {
	orgID := c.Param("org_id")
	id := c.Param("id")

	var manifest models.Manifest
	if err := h.db.Where("id = ? AND organization_id = ?", id, orgID).First(&manifest).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Manifest not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
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

	c.JSON(http.StatusOK, manifest)
}

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
		CreatedBy:      userID,
	}

	// 新模型: 不再创建初始 ManifestVersion (草稿走 manifest_files.version_id IS NULL,按需懒创建)
	if err := h.db.Create(&manifest).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Creation failed: " + err.Error()})
		return
	}

	writeManifestAudit(h.db, auditResourceManifest, "manifest.create", userID, map[string]interface{}{
		"manifest_id":     manifest.ID,
		"organization_id": orgID,
		"name":            manifest.Name,
	})

	c.JSON(http.StatusCreated, manifest)
}

func (h *ManifestHandler) UpdateManifest(c *gin.Context) {
	orgID := c.Param("org_id")
	id := c.Param("id")

	var manifest models.Manifest
	if err := h.db.Where("id = ? AND organization_id = ?", id, orgID).First(&manifest).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Manifest not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
		return
	}

	var req models.UpdateManifestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request parameters: " + err.Error()})
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
		manifest.Status = req.Status
	}

	if err := h.db.Save(&manifest).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Update failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, manifest)
}

func (h *ManifestHandler) DeleteManifest(c *gin.Context) {
	orgID := c.Param("org_id")
	id := c.Param("id")

	var manifest models.Manifest
	if err := h.db.Where("id = ? AND organization_id = ?", id, orgID).First(&manifest).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Manifest not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Deletion failed: " + err.Error()})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
		return
	}

	var label string
	var fileQuery *gorm.DB
	if versionID != "" {
		var version models.ManifestVersion
		if err := h.db.Where("id = ? AND manifest_id = ?", versionID, manifestID).First(&version).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "Version not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
			return
		}
		label = version.Version
		fileQuery = h.db.Where("manifest_id = ? AND version_id = ?", manifestID, versionID)
	} else {
		// 优先最新已发布版本
		var latest models.ManifestVersion
		if err := h.db.Where("manifest_id = ? AND version <> ?", manifestID, "draft").
			Order("created_at DESC").First(&latest).Error; err == nil {
			label = latest.Version
			fileQuery = h.db.Where("manifest_id = ? AND version_id = ?", manifestID, latest.ID)
		} else if userID != "" {
			// 退回当前用户草稿
			label = "draft"
			fileQuery = h.db.Where("manifest_id = ? AND owner_user_id = ? AND version_id IS NULL", manifestID, userID)
		} else {
			c.JSON(http.StatusNotFound, gin.H{"error": "No version or draft to export"})
			return
		}
	}

	var rows []models.ManifestFile
	if err := fileQuery.Order("path ASC").Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query files failed: " + err.Error()})
		return
	}
	if len(rows) == 0 {
		// 私有草稿模型下,无 published 版本时只能导出"自己的"草稿;若调用者没有草稿
		// (常见于他人创建、尚未发布的 manifest),给出可操作的提示而非裸 404。
		msg := "No files to export"
		if label == "draft" {
			msg = "this manifest has no published version and you have no draft of it yet; open it in the editor to create a draft, or publish a version first"
		}
		c.JSON(http.StatusNotFound, gin.H{"error": msg})
		return
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range rows {
		w, err := zw.Create(f.Path)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create ZIP entry: " + err.Error()})
			return
		}
		if _, err := w.Write(f.Content); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write ZIP entry: " + err.Error()})
			return
		}
	}
	if err := zw.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to finalize ZIP: " + err.Error()})
		return
	}

	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.zip"`, manifest.Name, label))
	c.Data(http.StatusOK, "application/zip", buf.Bytes())
}
