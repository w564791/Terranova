package handlers

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"iac-platform/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TaskQueueManagerInterface defines the interface for task queue manager
type TaskQueueManagerInterface interface {
	TryExecuteNextTask(workspaceID string) error
}

// ManifestHandler handles Manifest operations
type ManifestHandler struct {
	db           *gorm.DB
	queueManager TaskQueueManagerInterface
}

// NewManifestHandler creates a new ManifestHandler
func NewManifestHandler(db *gorm.DB) *ManifestHandler {
	return &ManifestHandler{db: db}
}

// SetQueueManager sets the task queue manager
func (h *ManifestHandler) SetQueueManager(qm TaskQueueManagerInterface) {
	h.queueManager = qm
}

// generateRandomID generates a 16-character random lowercase alphanumeric ID
func generateRandomID() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 16)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
		time.Sleep(time.Nanosecond)
	}
	// Use UUID random part to ensure uniqueness
	uuidStr := uuid.New().String()
	for i := 0; i < 16 && i < len(uuidStr); i++ {
		c := uuidStr[i]
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			b[i] = c
		} else if c >= 'A' && c <= 'Z' {
			b[i] = c + 32 // Convert to lowercase
		}
	}
	return string(b)
}

// generateManifestID generates a Manifest ID
func generateManifestID() string {
	return fmt.Sprintf("mf-%s", generateRandomID())
}

// generateManifestVersionID generates a ManifestVersion ID
func generateManifestVersionID() string {
	return fmt.Sprintf("mfv-%s", generateRandomID())
}

// generateManifestDeploymentID generates a ManifestDeployment ID
func generateManifestDeploymentID() string {
	return fmt.Sprintf("mfd-%s", generateRandomID())
}

// generateManifestDeploymentResourceID generates a ManifestDeploymentResource ID
func generateManifestDeploymentResourceID() string {
	return fmt.Sprintf("mdr-%s", generateRandomID())
}

// ========== Manifest CRUD ==========

// ListManifests retrieves the list of Manifests
// @Summary Get Manifest list
// @Tags Manifest
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Page size" default(20)
// @Param status query string false "Status filter"
// @Success 200 {object} models.ManifestListResponse
// @Router /api/v1/organizations/{org_id}/manifests [get]
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

	// Get total count
	query.Count(&total)

	// Paginated query
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&manifests).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
		return
	}

	// Fill additional information
	for i := range manifests {
		// Get latest version (only metadata, not nodes/edges/canvas_data)
		var latestVersion models.ManifestVersion
		if err := h.db.Select("id, manifest_id, version, is_draft, created_by, created_at").
			Where("manifest_id = ?", manifests[i].ID).
			Order("created_at DESC").First(&latestVersion).Error; err == nil {
			manifests[i].LatestVersion = &latestVersion
		}

		// Get deployment count
		var deploymentCount int64
		h.db.Model(&models.ManifestDeployment{}).Where("manifest_id = ?", manifests[i].ID).Count(&deploymentCount)
		manifests[i].DeploymentCount = int(deploymentCount)

		// Get creator name
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

// GetManifest retrieves Manifest details
// @Summary Get Manifest details
// @Tags Manifest
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Success 200 {object} models.Manifest
// @Router /api/v1/organizations/{org_id}/manifests/{id} [get]
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

	// Get latest version
	var latestVersion models.ManifestVersion
	if err := h.db.Where("manifest_id = ?", manifest.ID).
		Order("created_at DESC").First(&latestVersion).Error; err == nil {
		manifest.LatestVersion = &latestVersion
	}

	// Get deployment count
	var deploymentCount int64
	h.db.Model(&models.ManifestDeployment{}).Where("manifest_id = ?", manifest.ID).Count(&deploymentCount)
	manifest.DeploymentCount = int(deploymentCount)

	// Get creator name
	var user models.User
	if err := h.db.Select("username").Where("user_id = ?", manifest.CreatedBy).First(&user).Error; err == nil {
		manifest.CreatedByName = user.Username
	}

	c.JSON(http.StatusOK, manifest)
}

// CreateManifest creates a new Manifest
// @Summary Create Manifest
// @Tags Manifest
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param body body models.CreateManifestRequest true "Create request"
// @Success 201 {object} models.Manifest
// @Router /api/v1/organizations/{org_id}/manifests [post]
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

	// Check if name already exists
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

	// Create Manifest and initial draft version
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&manifest).Error; err != nil {
			return err
		}

		// Create initial draft version
		draftVersion := models.ManifestVersion{
			ID:         generateManifestVersionID(),
			ManifestID: manifest.ID,
			Version:    "draft",
			CanvasData: json.RawMessage(`{}`),
			Nodes:      json.RawMessage(`[]`),
			Edges:      json.RawMessage(`[]`),
			Variables:  json.RawMessage(`[]`),
			IsDraft:    true,
			CreatedBy:  userID,
		}
		return tx.Create(&draftVersion).Error
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Creation failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, manifest)
}

// UpdateManifest updates a Manifest
// @Summary Update Manifest
// @Tags Manifest
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Param body body models.UpdateManifestRequest true "Update request"
// @Success 200 {object} models.Manifest
// @Router /api/v1/organizations/{org_id}/manifests/{id} [put]
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

	// Check if name already exists
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

// DeleteManifest deletes a Manifest
// @Summary Delete Manifest
// @Tags Manifest
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Success 204
// @Router /api/v1/organizations/{org_id}/manifests/{id} [delete]
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

	// Check if there are deployments
	var deploymentCount int64
	h.db.Model(&models.ManifestDeployment{}).Where("manifest_id = ?", id).Count(&deploymentCount)
	if deploymentCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("Manifest has %d deployments, please delete them first", deploymentCount)})
		return
	}

	// Delete Manifest (cascade delete versions)
	if err := h.db.Delete(&manifest).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Deletion failed: " + err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// ========== Version Management ==========

// ListManifestVersions retrieves the list of versions
// @Summary Get version list
// @Tags Manifest
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Success 200 {object} models.ManifestVersionListResponse
// @Router /api/v1/organizations/{org_id}/manifests/{id}/versions [get]
func (h *ManifestHandler) ListManifestVersions(c *gin.Context) {
	orgID := c.Param("org_id")
	manifestID := c.Param("id")

	// Verify Manifest exists
	var manifest models.Manifest
	if err := h.db.Where("id = ? AND organization_id = ?", manifestID, orgID).First(&manifest).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Manifest not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
		return
	}

	var versions []models.ManifestVersion
	var total int64

	query := h.db.Model(&models.ManifestVersion{}).Where("manifest_id = ?", manifestID)
	query.Count(&total)

	if err := query.Order("created_at DESC").Find(&versions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
		return
	}

	// Fill creator name
	for i := range versions {
		var user models.User
		if err := h.db.Select("username").Where("id = ?", versions[i].CreatedBy).First(&user).Error; err == nil {
			versions[i].CreatedByName = user.Username
		}
	}

	c.JSON(http.StatusOK, models.ManifestVersionListResponse{
		Items:      versions,
		Total:      total,
		Page:       1,
		PageSize:   int(total),
		TotalPages: 1,
	})
}

// GetManifestVersion retrieves version details
// @Summary Get version details
// @Tags Manifest
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Param version_id path string true "Version ID"
// @Success 200 {object} models.ManifestVersion
// @Router /api/v1/organizations/{org_id}/manifests/{id}/versions/{version_id} [get]
func (h *ManifestHandler) GetManifestVersion(c *gin.Context) {
	orgID := c.Param("org_id")
	manifestID := c.Param("id")
	versionID := c.Param("version_id")

	// Verify Manifest exists
	var manifest models.Manifest
	if err := h.db.Where("id = ? AND organization_id = ?", manifestID, orgID).First(&manifest).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Manifest not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
		return
	}

	var version models.ManifestVersion
	if err := h.db.Where("id = ? AND manifest_id = ?", versionID, manifestID).First(&version).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Version not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
		return
	}

	// Fill creator name
	var user models.User
	if err := h.db.Select("username").Where("id = ?", version.CreatedBy).First(&user).Error; err == nil {
		version.CreatedByName = user.Username
	}

	c.JSON(http.StatusOK, version)
}

// SaveManifestDraft saves a draft
// @Summary Save draft
// @Tags Manifest
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Param body body models.SaveManifestVersionRequest true "Save request"
// @Success 200 {object} models.ManifestVersion
// @Router /api/v1/organizations/{org_id}/manifests/{id}/draft [put]
func (h *ManifestHandler) SaveManifestDraft(c *gin.Context) {
	orgID := c.Param("org_id")
	manifestID := c.Param("id")
	userID := c.GetString("user_id")

	// Verify Manifest exists
	var manifest models.Manifest
	if err := h.db.Where("id = ? AND organization_id = ?", manifestID, orgID).First(&manifest).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Manifest not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
		return
	}

	var req models.SaveManifestVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request parameters: " + err.Error()})
		return
	}

	// Find or create draft version
	var draftVersion models.ManifestVersion
	err := h.db.Where("manifest_id = ? AND is_draft = ?", manifestID, true).First(&draftVersion).Error
	if err == gorm.ErrRecordNotFound {
		// Create new draft
		draftVersion = models.ManifestVersion{
			ID:         generateManifestVersionID(),
			ManifestID: manifestID,
			Version:    "draft",
			CanvasData: req.CanvasData,
			Nodes:      req.Nodes,
			Edges:      req.Edges,
			Variables:  req.Variables,
			IsDraft:    true,
			CreatedBy:  userID,
		}
		if err := h.db.Create(&draftVersion).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create draft: " + err.Error()})
			return
		}
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
		return
	} else {
		// Update existing draft
		draftVersion.CanvasData = req.CanvasData
		draftVersion.Nodes = req.Nodes
		draftVersion.Edges = req.Edges
		draftVersion.Variables = req.Variables
		if err := h.db.Save(&draftVersion).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save draft: " + err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, draftVersion)
}

// PublishManifestVersion publishes a version
// @Summary Publish version
// @Tags Manifest
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Param body body models.PublishManifestVersionRequest true "Publish request"
// @Success 201 {object} models.ManifestVersion
// @Router /api/v1/organizations/{org_id}/manifests/{id}/versions [post]
func (h *ManifestHandler) PublishManifestVersion(c *gin.Context) {
	orgID := c.Param("org_id")
	manifestID := c.Param("id")
	userID := c.GetString("user_id")

	// Verify Manifest exists
	var manifest models.Manifest
	if err := h.db.Where("id = ? AND organization_id = ?", manifestID, orgID).First(&manifest).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Manifest not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
		return
	}

	var req models.PublishManifestVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request parameters: " + err.Error()})
		return
	}

	// Check if version number already exists
	var count int64
	h.db.Model(&models.ManifestVersion{}).Where("manifest_id = ? AND version = ?", manifestID, req.Version).Count(&count)
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Version number already exists"})
		return
	}

	// Get current draft
	var draftVersion models.ManifestVersion
	if err := h.db.Where("manifest_id = ? AND is_draft = ?", manifestID, true).First(&draftVersion).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No draft to publish"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
		return
	}

	// Create new version
	newVersion := models.ManifestVersion{
		ID:         generateManifestVersionID(),
		ManifestID: manifestID,
		Version:    req.Version,
		CanvasData: draftVersion.CanvasData,
		Nodes:      draftVersion.Nodes,
		Edges:      draftVersion.Edges,
		Variables:  draftVersion.Variables,
		HCLContent: draftVersion.HCLContent,
		IsDraft:    false,
		CreatedBy:  userID,
	}

	err := h.db.Transaction(func(tx *gorm.DB) error {
		// Use raw SQL to insert, ensuring is_draft is correctly set to false
		if err := tx.Exec(`
			INSERT INTO manifest_versions (id, manifest_id, version, canvas_data, nodes, edges, variables, hcl_content, is_draft, created_by, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, false, ?, NOW())
		`, newVersion.ID, newVersion.ManifestID, newVersion.Version, newVersion.CanvasData, newVersion.Nodes, newVersion.Edges, newVersion.Variables, newVersion.HCLContent, newVersion.CreatedBy).Error; err != nil {
			return err
		}

		// Update Manifest status to published
		return tx.Model(&manifest).Update("status", models.ManifestStatusPublished).Error
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Publish failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, newVersion)
}

// ========== Deployment Management ==========

// ListManifestDeployments retrieves the list of deployments
// @Summary Get deployment list
// @Tags Manifest
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Success 200 {object} models.ManifestDeploymentListResponse
// @Router /api/v1/organizations/{org_id}/manifests/{id}/deployments [get]
func (h *ManifestHandler) ListManifestDeployments(c *gin.Context) {
	orgID := c.Param("org_id")
	manifestID := c.Param("id")

	// Verify Manifest exists
	var manifest models.Manifest
	if err := h.db.Where("id = ? AND organization_id = ?", manifestID, orgID).First(&manifest).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Manifest not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
		return
	}

	var deployments []models.ManifestDeployment
	var total int64

	query := h.db.Model(&models.ManifestDeployment{}).Where("manifest_id = ?", manifestID)
	query.Count(&total)

	if err := query.Order("created_at DESC").Find(&deployments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
		return
	}

	// Fill additional information
	for i := range deployments {
		// Get Workspace name and semantic ID
		var workspace models.Workspace
		if err := h.db.Select("name, workspace_id").Where("id = ?", deployments[i].WorkspaceID).First(&workspace).Error; err == nil {
			deployments[i].WorkspaceName = workspace.Name
			deployments[i].WorkspaceSemanticID = workspace.WorkspaceID
		}

		// Get version name
		var version models.ManifestVersion
		if err := h.db.Select("version").Where("id = ?", deployments[i].VersionID).First(&version).Error; err == nil {
			deployments[i].VersionName = version.Version
		}

		// Get deployer name
		var user models.User
		if err := h.db.Select("username").Where("id = ?", deployments[i].DeployedBy).First(&user).Error; err == nil {
			deployments[i].DeployedByName = user.Username
		}
	}

	c.JSON(http.StatusOK, models.ManifestDeploymentListResponse{
		Items:      deployments,
		Total:      total,
		Page:       1,
		PageSize:   int(total),
		TotalPages: 1,
	})
}

// GetManifestDeploymentResources retrieves resources associated with a deployment
// @Summary Get deployment resources
// @Tags Manifest
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Param deployment_id path string true "Deployment ID"
// @Success 200 {array} object
// @Router /api/v1/organizations/{org_id}/manifests/{id}/deployments/{deployment_id}/resources [get]
func (h *ManifestHandler) GetManifestDeploymentResources(c *gin.Context) {
	orgID := c.Param("org_id")
	manifestID := c.Param("id")
	deploymentID := c.Param("deployment_id")

	// Verify Manifest exists
	var manifest models.Manifest
	if err := h.db.Where("id = ? AND organization_id = ?", manifestID, orgID).First(&manifest).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Manifest not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
		return
	}

	// Verify deployment exists
	var deployment models.ManifestDeployment
	if err := h.db.Where("id = ? AND manifest_id = ?", deploymentID, manifestID).First(&deployment).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Deployment not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
		return
	}

	// Get deployment resource associations
	var deploymentResources []models.ManifestDeploymentResource
	if err := h.db.Where("deployment_id = ?", deploymentID).Find(&deploymentResources).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
		return
	}

	// Get Workspace information
	var workspace models.Workspace
	h.db.Where("id = ?", deployment.WorkspaceID).First(&workspace)

	// Build resource detail list
	type ResourceDetail struct {
		NodeID       string `json:"node_id"`
		ResourceID   string `json:"resource_id"`
		ResourceDBID uint   `json:"resource_db_id"` // Database ID for navigation
		ResourceName string `json:"resource_name"`
		ResourceType string `json:"resource_type"`
		IsActive     bool   `json:"is_active"`
		Description  string `json:"description"`
		CreatedAt    string `json:"created_at"`
		IsDrifted    bool   `json:"is_drifted"`   // Whether drifted
		ConfigHash   string `json:"config_hash"`  // Hash at deployment time
		CurrentHash  string `json:"current_hash"` // Current hash
	}

	resources := make([]ResourceDetail, 0, len(deploymentResources))
	for _, dr := range deploymentResources {
		var wr models.WorkspaceResource
		if err := h.db.Where("workspace_id = ? AND resource_id = ?", workspace.WorkspaceID, dr.ResourceID).First(&wr).Error; err == nil {
			// Calculate current config hash
			currentHash := h.calculateResourceConfigHash(&wr)

			// If config_hash is empty, it's historical data, auto-fill
			configHash := dr.ConfigHash
			if configHash == "" && currentHash != "" {
				// Save current hash as baseline on first access
				configHash = currentHash
				h.db.Model(&models.ManifestDeploymentResource{}).
					Where("id = ?", dr.ID).
					Update("config_hash", configHash)
				log.Printf("[Manifest] Auto-filled config_hash for deployment resource %s: %s", dr.ID, configHash)
			}

			// Compare hash to determine if drifted
			isDrifted := configHash != "" && currentHash != "" && configHash != currentHash

			resources = append(resources, ResourceDetail{
				NodeID:       dr.NodeID,
				ResourceID:   dr.ResourceID,
				ResourceDBID: wr.ID,
				ResourceName: wr.ResourceName,
				ResourceType: wr.ResourceType,
				IsActive:     wr.IsActive,
				Description:  wr.Description,
				CreatedAt:    wr.CreatedAt.Format("2006-01-02 15:04:05"),
				IsDrifted:    isDrifted,
				ConfigHash:   configHash,
				CurrentHash:  currentHash,
			})
		} else {
			// Resource may have been deleted
			resources = append(resources, ResourceDetail{
				NodeID:       dr.NodeID,
				ResourceID:   dr.ResourceID,
				ResourceDBID: 0,
				ResourceName: "-",
				ResourceType: "-",
				IsActive:     false,
				Description:  "Resource deleted",
				CreatedAt:    dr.CreatedAt.Format("2006-01-02 15:04:05"),
				IsDrifted:    false,
				ConfigHash:   dr.ConfigHash,
				CurrentHash:  "",
			})
		}
	}

	c.JSON(http.StatusOK, resources)
}

// GetManifestDeployment retrieves deployment details
// @Summary Get deployment details
// @Tags Manifest
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Param deployment_id path string true "Deployment ID"
// @Success 200 {object} models.ManifestDeployment
// @Router /api/v1/organizations/{org_id}/manifests/{id}/deployments/{deployment_id} [get]
func (h *ManifestHandler) GetManifestDeployment(c *gin.Context) {
	orgID := c.Param("org_id")
	manifestID := c.Param("id")
	deploymentID := c.Param("deployment_id")

	// Verify Manifest exists
	var manifest models.Manifest
	if err := h.db.Where("id = ? AND organization_id = ?", manifestID, orgID).First(&manifest).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Manifest not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
		return
	}

	var deployment models.ManifestDeployment
	if err := h.db.Where("id = ? AND manifest_id = ?", deploymentID, manifestID).First(&deployment).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Deployment not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
		return
	}

	// Fill additional information
	var workspace models.Workspace
	if err := h.db.Select("name").Where("id = ?", deployment.WorkspaceID).First(&workspace).Error; err == nil {
		deployment.WorkspaceName = workspace.Name
	}

	var version models.ManifestVersion
	if err := h.db.Select("version").Where("id = ?", deployment.VersionID).First(&version).Error; err == nil {
		deployment.VersionName = version.Version
	}

	var user models.User
	if err := h.db.Select("username").Where("id = ?", deployment.DeployedBy).First(&user).Error; err == nil {
		deployment.DeployedByName = user.Username
	}

	c.JSON(http.StatusOK, deployment)
}

// CreateManifestDeployment creates a deployment
// @Summary Create deployment
// @Tags Manifest
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Param body body models.CreateManifestDeploymentRequest true "Create request"
// @Success 201 {object} models.ManifestDeployment
// @Router /api/v1/organizations/{org_id}/manifests/{id}/deployments [post]
func (h *ManifestHandler) CreateManifestDeployment(c *gin.Context) {
	orgID := c.Param("org_id")
	manifestID := c.Param("id")
	userID := c.GetString("user_id")

	// Verify Manifest exists
	var manifest models.Manifest
	if err := h.db.Where("id = ? AND organization_id = ?", manifestID, orgID).First(&manifest).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Manifest not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
		return
	}

	var req models.CreateManifestDeploymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request parameters: " + err.Error()})
		return
	}

	// Verify version exists
	var version models.ManifestVersion
	if err := h.db.Where("id = ? AND manifest_id = ?", req.VersionID, manifestID).First(&version).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Version not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
		return
	}

	// Verify Workspace exists
	var workspace models.Workspace
	if err := h.db.Where("id = ?", req.WorkspaceID).First(&workspace).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Workspace not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
		return
	}

	// Check if there's already an active deployment (excluding archived)
	var existingDeployment models.ManifestDeployment
	if err := h.db.Where("manifest_id = ? AND workspace_id = ? AND status != ?", manifestID, req.WorkspaceID, models.DeploymentStatusArchived).First(&existingDeployment).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "This Workspace already has an active deployment of this Manifest, please use the update API"})
		return
	}

	now := time.Now()
	deployment := models.ManifestDeployment{
		ID:                generateManifestDeploymentID(),
		ManifestID:        manifestID,
		VersionID:         req.VersionID,
		WorkspaceID:       req.WorkspaceID,
		VariableOverrides: req.VariableOverrides,
		Status:            models.DeploymentStatusPending,
		DeployedBy:        userID,
		DeployedAt:        &now,
	}

	if err := h.db.Create(&deployment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create deployment: " + err.Error()})
		return
	}

	// Trigger actual deployment: create resources based on version nodes
	go h.executeDeployment(deployment, version, workspace, userID)

	c.JSON(http.StatusCreated, deployment)
}

// UpdateManifestDeployment updates a deployment
// @Summary Update deployment
// @Tags Manifest
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Param deployment_id path string true "Deployment ID"
// @Param body body models.UpdateManifestDeploymentRequest true "Update request"
// @Success 200 {object} models.ManifestDeployment
// @Router /api/v1/organizations/{org_id}/manifests/{id}/deployments/{deployment_id} [put]
func (h *ManifestHandler) UpdateManifestDeployment(c *gin.Context) {
	orgID := c.Param("org_id")
	manifestID := c.Param("id")
	deploymentID := c.Param("deployment_id")
	userID := c.GetString("user_id")

	// Verify Manifest exists
	var manifest models.Manifest
	if err := h.db.Where("id = ? AND organization_id = ?", manifestID, orgID).First(&manifest).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Manifest not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
		return
	}

	var deployment models.ManifestDeployment
	if err := h.db.Where("id = ? AND manifest_id = ?", deploymentID, manifestID).First(&deployment).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Deployment not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
		return
	}

	var req models.UpdateManifestDeploymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request parameters: " + err.Error()})
		return
	}

	// Update version
	if req.VersionID != "" && req.VersionID != deployment.VersionID {
		var version models.ManifestVersion
		if err := h.db.Where("id = ? AND manifest_id = ?", req.VersionID, manifestID).First(&version).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Version not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
			return
		}
		deployment.VersionID = req.VersionID
	}

	// Update variable overrides
	if req.VariableOverrides != nil {
		deployment.VariableOverrides = req.VariableOverrides
	}

	now := time.Now()
	deployment.DeployedBy = userID
	deployment.DeployedAt = &now
	deployment.Status = models.DeploymentStatusPending

	if err := h.db.Save(&deployment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update deployment: " + err.Error()})
		return
	}

	// TODO: Trigger actual deployment update

	c.JSON(http.StatusOK, deployment)
}

// DeleteManifestDeployment deletes/uninstalls a deployment
// @Summary Delete deployment (supports soft delete and hard delete/uninstall)
// @Tags Manifest
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Param deployment_id path string true "Deployment ID"
// @Success 200 {object} models.ManifestDeployment
// @Param uninstall query bool false "Whether to uninstall (hard delete resources)"
// @Param force query bool false "Force uninstall (ignore drift warnings, only effective when uninstall=true)"
// @Router /api/v1/organizations/{org_id}/manifests/{id}/deployments/{deployment_id} [delete]
func (h *ManifestHandler) DeleteManifestDeployment(c *gin.Context) {
	orgID := c.Param("org_id")
	manifestID := c.Param("id")
	deploymentID := c.Param("deployment_id")
	userID := c.GetString("user_id")
	uninstall := c.Query("uninstall") == "true"
	force := c.Query("force") == "true"

	// Verify Manifest exists
	var manifest models.Manifest
	if err := h.db.Where("id = ? AND organization_id = ?", manifestID, orgID).First(&manifest).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Manifest not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
		return
	}

	var deployment models.ManifestDeployment
	if err := h.db.Where("id = ? AND manifest_id = ?", deploymentID, manifestID).First(&deployment).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Deployment not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
		return
	}

	// Get Workspace information
	var workspace models.Workspace
	if err := h.db.Where("id = ?", deployment.WorkspaceID).First(&workspace).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get Workspace: " + err.Error()})
		return
	}

	// Get deployment resource associations
	var deploymentResources []models.ManifestDeploymentResource
	if err := h.db.Where("deployment_id = ?", deploymentID).Find(&deploymentResources).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
		return
	}

	if uninstall {
		// Hard delete/uninstall mode: permanently delete resources, then create Plan+Apply task to execute Terraform destroy

		// Check drifted resources
		driftedResources := []string{}
		for _, dr := range deploymentResources {
			var wr models.WorkspaceResource
			if err := h.db.Where("workspace_id = ? AND resource_id = ?", workspace.WorkspaceID, dr.ResourceID).First(&wr).Error; err == nil {
				currentHash := h.calculateResourceConfigHash(&wr)
				if dr.ConfigHash != "" && dr.ConfigHash != currentHash {
					driftedResources = append(driftedResources, wr.ResourceName)
				}
			}
		}

		// If there are drifted resources and not force uninstall, return warning
		if len(driftedResources) > 0 && !force {
			c.JSON(http.StatusConflict, gin.H{
				"error":             "Drifted resources detected, these resources have been manually modified",
				"drifted_resources": driftedResources,
				"message":           "Please confirm if you want to force uninstall these resources. Add &force=true parameter to force uninstall.",
			})
			return
		}

		// Hard delete resources (permanently delete resources and their versions)
		deletedCount := 0
		for _, dr := range deploymentResources {
			var wr models.WorkspaceResource
			if err := h.db.Where("workspace_id = ? AND resource_id = ?", workspace.WorkspaceID, dr.ResourceID).First(&wr).Error; err == nil {
				// Hard delete: delete associated outputs, versions, dependencies and the resource itself
				h.db.Where("workspace_id = ? AND resource_name = ?", workspace.WorkspaceID, wr.ResourceName).
					Delete(&models.WorkspaceOutput{})
				h.db.Where("resource_id = ?", wr.ID).Delete(&models.ResourceCodeVersion{})
				h.db.Where("resource_id = ? OR depends_on_resource_id = ?", wr.ID, wr.ID).
					Delete(&models.ResourceDependency{})
				h.db.Delete(&wr)
				deletedCount++
				log.Printf("[Manifest] Hard deleted resource %s (ID: %d)", dr.ResourceID, wr.ID)
			}
		}

		// Delete deployment resource associations
		h.db.Where("deployment_id = ?", deploymentID).Delete(&models.ManifestDeploymentResource{})

		// Set deployment status to archived
		deployment.Status = models.DeploymentStatusArchived
		h.db.Save(&deployment)

		// Create Plan+Apply task to execute Terraform destroy (since resources are deleted, Plan will generate destroy plan)
		taskID, err := h.createPlanAndApplyTask(workspace, userID, fmt.Sprintf("Manifest uninstall: %s", deployment.ID))
		if err != nil {
			log.Printf("[Manifest] Failed to create plan+apply task for deployment %s: %v", deployment.ID, err)
			// Resources already deleted, return success even if task creation fails
		}

		c.JSON(http.StatusOK, gin.H{
			"message":           "Uninstall task created",
			"deployment_id":     deploymentID,
			"task_id":           taskID,
			"deleted_resources": deletedCount,
			"drifted_resources": driftedResources,
		})
	} else {
		// Soft delete mode: only archive deployment, don't delete resources

		// Set status to archived
		deployment.Status = models.DeploymentStatusArchived
		if err := h.db.Save(&deployment).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to archive deployment: " + err.Error()})
			return
		}

		// Clear manifest_deployment_id from associated resources (unlink, but don't delete resources)
		h.db.Model(&models.WorkspaceResource{}).
			Where("manifest_deployment_id = ?", deploymentID).
			Update("manifest_deployment_id", nil)

		c.JSON(http.StatusOK, deployment)
	}
}

// UninstallManifestDeployment uninstalls a deployment (permanently delete resources) - compatible with old API
// @Summary Uninstall deployment (recommend using DELETE API with uninstall=true parameter)
// @Tags Manifest
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Param deployment_id path string true "Deployment ID"
// @Param force query bool false "Force uninstall (ignore drift warnings)"
// @Success 200 {object} object
// @Router /api/v1/organizations/{org_id}/manifests/{id}/deployments/{deployment_id}/uninstall [post]
func (h *ManifestHandler) UninstallManifestDeployment(c *gin.Context) {
	// Set uninstall parameter and call DeleteManifestDeployment
	c.Request.URL.RawQuery = c.Request.URL.RawQuery + "&uninstall=true"
	h.DeleteManifestDeployment(c)
}

// ========== Workspace Perspective ==========

// GetWorkspaceManifestDeployment retrieves Workspace's Manifest deployment
// @Summary Get Workspace's Manifest deployment
// @Tags Manifest
// @Accept json
// @Produce json
// @Param workspace_id path string true "Workspace ID"
// @Success 200 {object} models.ManifestDeployment
// @Router /api/v1/workspaces/{workspace_id}/manifest-deployment [get]
func (h *ManifestHandler) GetWorkspaceManifestDeployment(c *gin.Context) {
	workspaceID := c.Param("workspace_id")

	var deployment models.ManifestDeployment
	if err := h.db.Where("workspace_id = ?", workspaceID).First(&deployment).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "This Workspace has no Manifest deployment"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
		return
	}

	// Fill additional information
	var manifest models.Manifest
	if err := h.db.Select("name").Where("id = ?", deployment.ManifestID).First(&manifest).Error; err == nil {
		// Can add ManifestName field
	}

	var version models.ManifestVersion
	if err := h.db.Select("version").Where("id = ?", deployment.VersionID).First(&version).Error; err == nil {
		deployment.VersionName = version.Version
	}

	var user models.User
	if err := h.db.Select("username").Where("id = ?", deployment.DeployedBy).First(&user).Error; err == nil {
		deployment.DeployedByName = user.Username
	}

	c.JSON(http.StatusOK, deployment)
}

// ========== Import/Export ==========

// ExportManifestZip exports Manifest as ZIP (contains manifest.json and .tf file)
// @Summary Export Manifest as ZIP
// @Tags Manifest
// @Produce application/zip
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Param version_id query string false "Version ID (default latest version)"
// @Success 200 {file} file "ZIP file"
// @Router /api/v1/organizations/{org_id}/manifests/{id}/export-zip [get]
func (h *ManifestHandler) ExportManifestZip(c *gin.Context) {
	orgID := c.Param("org_id")
	manifestID := c.Param("id")
	versionID := c.Query("version_id")

	// Verify Manifest exists
	var manifest models.Manifest
	if err := h.db.Where("id = ? AND organization_id = ?", manifestID, orgID).First(&manifest).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Manifest not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
		return
	}

	var version models.ManifestVersion
	if versionID != "" {
		if err := h.db.Where("id = ? AND manifest_id = ?", versionID, manifestID).First(&version).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "Version not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
			return
		}
	} else {
		// Prefer draft version
		if err := h.db.Where("manifest_id = ? AND is_draft = ?", manifestID, true).First(&version).Error; err != nil {
			if err := h.db.Where("manifest_id = ? AND is_draft = ?", manifestID, false).Order("created_at DESC").First(&version).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					c.JSON(http.StatusNotFound, gin.H{"error": "No version to export"})
					return
				}
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
				return
			}
		}
	}

	// Generate HCL content
	hclContent := generateHCL(manifest.Name, version)

	// Build manifest.json structure
	manifestJSON := map[string]interface{}{
		"name":        manifest.Name,
		"description": manifest.Description,
		"version":     version.Version,
		"canvas_data": version.CanvasData,
		"nodes":       version.Nodes,
		"edges":       version.Edges,
		"variables":   version.Variables,
		"hcl_content": hclContent,
		"exported_at": time.Now().Format(time.RFC3339),
		"platform":    "iac-platform",
	}

	manifestJSONBytes, err := json.MarshalIndent(manifestJSON, "", "  ")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to serialize manifest: " + err.Error()})
		return
	}

	// Create ZIP in memory
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	// Add manifest.json
	manifestFile, err := zipWriter.Create(fmt.Sprintf("%s.manifest.json", manifest.Name))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create ZIP: " + err.Error()})
		return
	}
	manifestFile.Write(manifestJSONBytes)

	// Add .tf file
	tfFile, err := zipWriter.Create(fmt.Sprintf("%s.tf", manifest.Name))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create ZIP: " + err.Error()})
		return
	}
	tfFile.Write([]byte(hclContent))

	zipWriter.Close()

	// Return ZIP file
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s-%s.zip", manifest.Name, version.Version))
	c.Data(http.StatusOK, "application/zip", buf.Bytes())
}

// ImportManifestJSON imports Manifest from manifest.json
// @Summary Import Manifest from JSON
// @Tags Manifest
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param body body object true "Manifest JSON content"
// @Success 201 {object} models.Manifest
// @Router /api/v1/organizations/{org_id}/manifests/import-json [post]
func (h *ManifestHandler) ImportManifestJSON(c *gin.Context) {
	orgIDStr := c.Param("org_id")
	orgID, err := strconv.Atoi(orgIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid organization ID"})
		return
	}
	userID := c.GetString("user_id")

	var req struct {
		ManifestJSON json.RawMessage `json:"manifest_json" binding:"required"`
		Name         string          `json:"name"` // Optional, override name
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	// Parse manifest JSON
	var manifestData struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Version     string          `json:"version"`
		CanvasData  json.RawMessage `json:"canvas_data"`
		Nodes       json.RawMessage `json:"nodes"`
		Edges       json.RawMessage `json:"edges"`
		Variables   json.RawMessage `json:"variables"`
		HCLContent  string          `json:"hcl_content"`
	}
	if err := json.Unmarshal(req.ManifestJSON, &manifestData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid manifest JSON: " + err.Error()})
		return
	}

	// Use override name if provided
	name := manifestData.Name
	if req.Name != "" {
		name = req.Name
	}

	// Check if name already exists
	var count int64
	h.db.Model(&models.Manifest{}).Where("organization_id = ? AND name = ?", orgID, name).Count(&count)
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Manifest name already exists"})
		return
	}

	// Create Manifest
	manifest := models.Manifest{
		ID:             generateManifestID(),
		OrganizationID: orgID,
		Name:           name,
		Description:    manifestData.Description,
		Status:         models.ManifestStatusDraft,
		CreatedBy:      userID,
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&manifest).Error; err != nil {
			return err
		}

		// Create draft version with imported data
		draftVersion := models.ManifestVersion{
			ID:         generateManifestVersionID(),
			ManifestID: manifest.ID,
			Version:    "draft",
			CanvasData: manifestData.CanvasData,
			Nodes:      manifestData.Nodes,
			Edges:      manifestData.Edges,
			Variables:  manifestData.Variables,
			HCLContent: manifestData.HCLContent,
			IsDraft:    true,
			CreatedBy:  userID,
		}
		return tx.Create(&draftVersion).Error
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create manifest: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, manifest)
}

// ExportManifestHCL exports HCL
// @Summary Export HCL
// @Tags Manifest
// @Accept json
// @Produce text/plain
// @Param org_id path string true "Organization ID"
// @Param id path string true "Manifest ID"
// @Param version_id query string false "Version ID (default latest version)"
// @Success 200 {string} string "HCL content"
// @Router /api/v1/organizations/{org_id}/manifests/{id}/export [get]
func (h *ManifestHandler) ExportManifestHCL(c *gin.Context) {
	orgID := c.Param("org_id")
	manifestID := c.Param("id")
	versionID := c.Query("version_id")

	// Verify Manifest exists
	var manifest models.Manifest
	if err := h.db.Where("id = ? AND organization_id = ?", manifestID, orgID).First(&manifest).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Manifest not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
		return
	}

	var version models.ManifestVersion
	if versionID != "" {
		if err := h.db.Where("id = ? AND manifest_id = ?", versionID, manifestID).First(&version).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "Version not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
			return
		}
	} else {
		// Prefer draft version, if no draft then get latest published version
		if err := h.db.Where("manifest_id = ? AND is_draft = ?", manifestID, true).First(&version).Error; err != nil {
			// No draft, get latest published version
			if err := h.db.Where("manifest_id = ? AND is_draft = ?", manifestID, false).Order("created_at DESC").First(&version).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					c.JSON(http.StatusNotFound, gin.H{"error": "No version to export"})
					return
				}
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Query failed: " + err.Error()})
				return
			}
		}
	}

	// If HCL content already exists, return directly
	if version.HCLContent != "" {
		c.Header("Content-Type", "text/plain")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s-%s.tf", manifest.Name, version.Version))
		c.String(http.StatusOK, version.HCLContent)
		return
	}

	// TODO: Generate HCL based on nodes and edges
	hcl := generateHCL(manifest.Name, version)

	c.Header("Content-Type", "text/plain")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s-%s.tf", manifest.Name, version.Version))
	c.String(http.StatusOK, hcl)
}

// generateHCL generates HCL content
func generateHCL(manifestName string, version models.ManifestVersion) string {
	// Parse nodes and edges
	var nodes []models.ManifestNode
	var variables []models.ManifestVariable

	_ = json.Unmarshal(version.Nodes, &nodes)
	_ = json.Unmarshal(version.Variables, &variables)

	hcl := fmt.Sprintf("# Generated by IaC Platform Manifest Builder\n# Manifest: %s\n# Version: %s\n\n", manifestName, version.Version)

	// Generate variable definitions
	for _, v := range variables {
		hcl += fmt.Sprintf("variable \"%s\" {\n", v.Name)
		hcl += fmt.Sprintf("  type = %s\n", v.Type)
		if v.Description != "" {
			hcl += fmt.Sprintf("  description = \"%s\"\n", v.Description)
		}
		if v.Default != nil {
			hcl += fmt.Sprintf("  default = %v\n", v.Default)
		}
		hcl += "}\n\n"
	}

	// Generate module blocks
	for _, node := range nodes {
		if node.Type != models.NodeTypeModule {
			continue
		}

		hcl += fmt.Sprintf("module \"%s\" {\n", node.InstanceName)
		if node.ModuleSource != "" {
			hcl += fmt.Sprintf("  source  = \"%s\"\n", node.ModuleSource)
		}
		if node.ModuleVersion != "" {
			hcl += fmt.Sprintf("  version = \"%s\"\n", node.ModuleVersion)
		}

		// Add config parameters
		for key, value := range node.Config {
			hcl += fmt.Sprintf("  %s = %v\n", key, formatHCLValue(value))
		}

		hcl += "}\n\n"
	}

	return hcl
}

// formatHCLValue formats HCL value
func formatHCLValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		// Check if it's a Terraform reference (var.xxx, local.xxx, module.xxx, data.xxx)
		// Use "${...}" interpolation syntax for references
		if strings.HasPrefix(v, "var.") || strings.HasPrefix(v, "local.") ||
			strings.HasPrefix(v, "module.") || strings.HasPrefix(v, "data.") {
			return fmt.Sprintf("\"${%s}\"", v) // Use interpolation syntax
		}
		return fmt.Sprintf("\"%s\"", v)
	case bool:
		return fmt.Sprintf("%t", v)
	case float64:
		return fmt.Sprintf("%v", v)
	case int:
		return fmt.Sprintf("%d", v)
	case []interface{}:
		// Array type
		if len(v) == 0 {
			return "[]"
		}
		items := make([]string, len(v))
		for i, item := range v {
			items[i] = formatHCLValue(item)
		}
		return fmt.Sprintf("[%s]", strings.Join(items, ", "))
	case []string:
		// String array
		if len(v) == 0 {
			return "[]"
		}
		items := make([]string, len(v))
		for i, item := range v {
			items[i] = fmt.Sprintf("\"%s\"", item)
		}
		return fmt.Sprintf("[%s]", strings.Join(items, ", "))
	case map[string]interface{}:
		// map/object type - format as multi-line
		if len(v) == 0 {
			return "{}"
		}
		items := make([]string, 0, len(v))
		for key, val := range v {
			items = append(items, fmt.Sprintf("    %s = %s", key, formatHCLValue(val)))
		}
		return fmt.Sprintf("{\n%s\n  }", strings.Join(items, "\n"))
	default:
		// Other types try JSON serialization
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("\"%v\"", v)
		}
		// Try to parse as map or array
		var mapVal map[string]interface{}
		if json.Unmarshal(data, &mapVal) == nil {
			return formatHCLValue(mapVal)
		}
		var arrVal []interface{}
		if json.Unmarshal(data, &arrVal) == nil {
			return formatHCLValue(arrVal)
		}
		return string(data)
	}
}

// ImportManifestHCL imports HCL
// @Summary Import HCL
// @Tags Manifest
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param body body object true "HCL content"
// @Success 201 {object} models.Manifest
// @Router /api/v1/organizations/{org_id}/manifests/import [post]
func (h *ManifestHandler) ImportManifestHCL(c *gin.Context) {
	orgIDStr := c.Param("org_id")
	orgID, err := strconv.Atoi(orgIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid organization ID"})
		return
	}
	userID := c.GetString("user_id")

	var req struct {
		HCLContent string `json:"hcl_content" binding:"required"`
		Name       string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request parameters: " + err.Error()})
		return
	}

	// Check if name already exists
	var count int64
	h.db.Model(&models.Manifest{}).Where("organization_id = ? AND name = ?", orgID, req.Name).Count(&count)
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Manifest name already exists"})
		return
	}

	// Parse HCL content to extract modules, variables, and edges
	nodes, variables, edges, err := h.parseHCLContent(req.HCLContent)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse HCL: " + err.Error()})
		return
	}

	// Create Manifest
	manifest := models.Manifest{
		ID:             generateManifestID(),
		OrganizationID: orgID,
		Name:           req.Name,
		Description:    fmt.Sprintf("Imported from HCL (%d modules, %d variables, %d edges)", len(nodes), len(variables), len(edges)),
		Status:         models.ManifestStatusDraft,
		CreatedBy:      userID,
	}

	// Serialize nodes, variables, and edges
	nodesJSON, _ := json.Marshal(nodes)
	variablesJSON, _ := json.Marshal(variables)
	edgesJSON, _ := json.Marshal(edges)

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&manifest).Error; err != nil {
			return err
		}

		// Create draft version with imported content
		draftVersion := models.ManifestVersion{
			ID:         generateManifestVersionID(),
			ManifestID: manifest.ID,
			Version:    "draft",
			CanvasData: json.RawMessage(`{"viewport":{"x":0,"y":0},"zoom":1}`),
			Nodes:      nodesJSON,
			Edges:      edgesJSON,
			Variables:  variablesJSON,
			HCLContent: req.HCLContent,
			IsDraft:    true,
			CreatedBy:  userID,
		}
		return tx.Create(&draftVersion).Error
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create manifest: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, manifest)
}

// parseHCLContent parses HCL content and extracts modules, variables, and edges
func (h *ManifestHandler) parseHCLContent(hclContent string) ([]models.ManifestNode, []models.ManifestVariable, []models.ManifestEdge, error) {
	nodes := []models.ManifestNode{}
	variables := []models.ManifestVariable{}
	edges := []models.ManifestEdge{}

	lines := strings.Split(hclContent, "\n")

	var currentBlock string
	var currentName string
	var blockContent strings.Builder
	var braceCount int
	var inBlock bool

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// Skip comments and empty lines when not in a block
		if !inBlock {
			if trimmedLine == "" || strings.HasPrefix(trimmedLine, "#") || strings.HasPrefix(trimmedLine, "//") {
				continue
			}
		}

		// Detect block start
		if !inBlock {
			// Match: module "name" {
			if strings.HasPrefix(trimmedLine, "module ") {
				parts := strings.SplitN(trimmedLine, "\"", 3)
				if len(parts) >= 2 {
					currentBlock = "module"
					currentName = parts[1]
					inBlock = true
					braceCount = strings.Count(trimmedLine, "{") - strings.Count(trimmedLine, "}")
					blockContent.Reset()
					blockContent.WriteString(line + "\n")
					continue
				}
			}
			// Match: variable "name" {
			if strings.HasPrefix(trimmedLine, "variable ") {
				parts := strings.SplitN(trimmedLine, "\"", 3)
				if len(parts) >= 2 {
					currentBlock = "variable"
					currentName = parts[1]
					inBlock = true
					braceCount = strings.Count(trimmedLine, "{") - strings.Count(trimmedLine, "}")
					blockContent.Reset()
					blockContent.WriteString(line + "\n")
					continue
				}
			}
		}

		// Inside a block
		if inBlock {
			blockContent.WriteString(line + "\n")
			braceCount += strings.Count(line, "{") - strings.Count(line, "}")

			// Block ended
			if braceCount <= 0 {
				inBlock = false
				content := blockContent.String()

				switch currentBlock {
				case "module":
					node := h.parseModuleBlock(currentName, content, len(nodes))
					nodes = append(nodes, node)
				case "variable":
					variable := h.parseVariableBlock(currentName, content)
					variables = append(variables, variable)
				}

				currentBlock = ""
				currentName = ""
			}
		}
	}

	// Try to link modules to existing Module records in database
	for i := range nodes {
		if nodes[i].Type == models.NodeTypeModule && nodes[i].ModuleSource != "" {
			h.tryLinkModule(&nodes[i])
		}
	}

	// Extract module references and create edges
	edges = h.extractModuleReferences(nodes)

	return nodes, variables, edges, nil
}

// Binding represents a single parameter mapping in an edge
type Binding struct {
	SourceOutput string `json:"sourceOutput"`
	TargetInput  string `json:"targetInput"`
	Expression   string `json:"expression"`
}

// extractModuleReferences scans node configs for module.xxx.xxx references and creates edges
// Two nodes can only have one edge between them, but the edge can contain multiple bindings
func (h *ManifestHandler) extractModuleReferences(nodes []models.ManifestNode) []models.ManifestEdge {
	// Build maps for node lookup
	instanceToNodeID := make(map[string]string)
	nodeIDToNode := make(map[string]*models.ManifestNode)
	for i := range nodes {
		node := &nodes[i]
		if node.Type == models.NodeTypeModule && node.InstanceName != "" {
			instanceToNodeID[node.InstanceName] = node.ID
			nodeIDToNode[node.ID] = node
		}
	}

	// Map to track edges between node pairs: "sourceNodeID->targetNodeID" -> edge
	edgeMap := make(map[string]*models.ManifestEdge)
	// Map to track bindings for each edge
	bindingsMap := make(map[string][]Binding)

	// Regex to match module.xxx.xxx references (with or without ${...} interpolation)
	// Matches: module.xxx.yyy or ${module.xxx.yyy}
	moduleRefRegex := regexp.MustCompile(`\$?\{?module\.([a-zA-Z_][a-zA-Z0-9_-]*)\.([a-zA-Z_][a-zA-Z0-9_]*)\}?`)

	for _, node := range nodes {
		if node.Type != models.NodeTypeModule {
			continue
		}

		// Scan all config values for module references
		for paramName, value := range node.Config {
			valueStr := fmt.Sprintf("%v", value)
			matches := moduleRefRegex.FindAllStringSubmatch(valueStr, -1)

			for _, match := range matches {
				if len(match) >= 3 {
					sourceInstanceName := match[1]
					outputName := match[2]

					// Find source node ID
					sourceNodeID, exists := instanceToNodeID[sourceInstanceName]
					if !exists {
						log.Printf("[Manifest Import] Reference to unknown module: %s", sourceInstanceName)
						continue
					}

					// Create edge key for this node pair
					edgeKey := fmt.Sprintf("%s->%s", sourceNodeID, node.ID)

					// Create binding
					binding := Binding{
						SourceOutput: outputName,
						TargetInput:  paramName,
						Expression:   match[0], // e.g., "module.xxx.yyy"
					}

					// Check if edge already exists for this node pair
					if _, exists := edgeMap[edgeKey]; !exists {
						// Calculate best connection ports based on node positions
						sourcePort, targetPort := h.calculateBestPorts(nodeIDToNode[sourceNodeID], &node)

						// Create new edge with smart port selection
						edge := &models.ManifestEdge{
							ID:   fmt.Sprintf("edge-ref-%s-%s", sourceNodeID, node.ID),
							Type: "variable_binding",
							Source: models.ManifestEdgePoint{
								NodeID: sourceNodeID,
								PortID: sourcePort,
							},
							Target: models.ManifestEdgePoint{
								NodeID: node.ID,
								PortID: targetPort,
							},
						}
						edgeMap[edgeKey] = edge
						bindingsMap[edgeKey] = []Binding{}
					}

					// Add binding to this edge
					bindingsMap[edgeKey] = append(bindingsMap[edgeKey], binding)
					log.Printf("[Manifest Import] Added binding to edge %s: %s.%s -> %s",
						edgeKey, sourceInstanceName, outputName, paramName)
				}
			}
		}
	}

	// Convert map to slice and serialize bindings
	edges := make([]models.ManifestEdge, 0, len(edgeMap))
	for edgeKey, edge := range edgeMap {
		// Serialize bindings to JSON and store in Expression field
		bindings := bindingsMap[edgeKey]
		if len(bindings) > 0 {
			bindingsJSON, _ := json.Marshal(bindings)
			edge.Expression = string(bindingsJSON)
		}
		edges = append(edges, *edge)
		log.Printf("[Manifest Import] Created edge: %s with %d bindings", edge.ID, len(bindings))
	}

	return edges
}

// calculateBestPorts calculates the best connection ports based on node positions
// Returns (sourcePort, targetPort) - the port IDs for source and target nodes
func (h *ManifestHandler) calculateBestPorts(sourceNode, targetNode *models.ManifestNode) (string, string) {
	if sourceNode == nil || targetNode == nil {
		// Default: right-source to left
		return "right-source", "left"
	}

	// Calculate relative position
	dx := targetNode.Position.X - sourceNode.Position.X
	dy := targetNode.Position.Y - sourceNode.Position.Y

	// Determine the best ports based on relative position
	// The idea is to choose ports that result in the shortest, most natural connection

	// If target is to the right of source
	if dx > 50 {
		return "right-source", "left"
	}
	// If target is to the left of source
	if dx < -50 {
		return "left-source", "right"
	}
	// If target is below source (and roughly same X)
	if dy > 50 {
		return "bottom-source", "top"
	}
	// If target is above source (and roughly same X)
	if dy < -50 {
		return "top-source", "bottom"
	}

	// Default: right to left (most common flow direction)
	return "right-source", "left"
}

// tryLinkModule tries to find and link a Module record based on ModuleSource
func (h *ManifestHandler) tryLinkModule(node *models.ManifestNode) {
	if node.ModuleSource == "" {
		return
	}

	// Try to find module by module_source (exact match)
	var module models.Module
	if err := h.db.Where("module_source = ? AND status = ?", node.ModuleSource, "active").First(&module).Error; err == nil {
		// Found matching module - convert uint to int
		moduleID := int(module.ID)
		node.ModuleID = &moduleID
		node.IsLinked = true
		node.LinkStatus = "linked"
		log.Printf("[Manifest Import] Linked node %s to module %d (source: %s)", node.InstanceName, module.ID, node.ModuleSource)
		return
	}

	// Try to find module by source field (for backward compatibility)
	if err := h.db.Where("source = ? AND status = ?", node.ModuleSource, "active").First(&module).Error; err == nil {
		moduleID := int(module.ID)
		node.ModuleID = &moduleID
		node.IsLinked = true
		node.LinkStatus = "linked"
		log.Printf("[Manifest Import] Linked node %s to module %d (source: %s)", node.InstanceName, module.ID, node.ModuleSource)
		return
	}

	// No match found, keep as unlinked
	log.Printf("[Manifest Import] No matching module found for source: %s", node.ModuleSource)
}

// parseModuleBlock parses a module block and returns a ManifestNode
func (h *ManifestHandler) parseModuleBlock(name string, content string, index int) models.ManifestNode {
	node := models.ManifestNode{
		ID:           fmt.Sprintf("node-%s-%d", name, index),
		Type:         models.NodeTypeModule,
		InstanceName: name,
		ResourceName: name,
		Position: models.ManifestNodePosition{
			X: float64(100 + (index%4)*300),
			Y: float64(100 + (index/4)*200),
		},
		Config:         make(map[string]interface{}),
		ConfigComplete: false,
		Ports:          []models.ManifestPort{},
		IsLinked:       false,
		LinkStatus:     "unlinked",
	}

	lines := strings.Split(content, "\n")
	var currentKey string
	var valueBuilder strings.Builder
	var braceDepth int
	var bracketDepth int
	var inMultiLineValue bool

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// If we're collecting a multi-line value
		if inMultiLineValue {
			valueBuilder.WriteString(line + "\n")
			braceDepth += strings.Count(line, "{") - strings.Count(line, "}")
			bracketDepth += strings.Count(line, "[") - strings.Count(line, "]")

			// Check if multi-line value is complete
			if braceDepth <= 0 && bracketDepth <= 0 {
				inMultiLineValue = false
				value := parseHCLValueToInterface(strings.TrimSpace(valueBuilder.String()))
				node.Config[currentKey] = value
				currentKey = ""
				valueBuilder.Reset()
			}
			continue
		}

		// Parse source
		if strings.HasPrefix(trimmedLine, "source") && !strings.HasPrefix(trimmedLine, "source_") {
			value := extractHCLValue(trimmedLine)
			node.ModuleSource = value
			node.RawSource = value
			continue
		}
		// Parse version
		if strings.HasPrefix(trimmedLine, "version") {
			value := extractHCLValue(trimmedLine)
			node.ModuleVersion = value
			node.RawVersion = value
			continue
		}
		// Skip module declaration line
		if strings.HasPrefix(trimmedLine, "module") || trimmedLine == "}" {
			continue
		}
		// Parse other config values
		if strings.Contains(trimmedLine, "=") {
			parts := strings.SplitN(trimmedLine, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				valueStr := strings.TrimSpace(parts[1])

				// Check if this is a multi-line value (starts with { or [)
				braceDepth = strings.Count(valueStr, "{") - strings.Count(valueStr, "}")
				bracketDepth = strings.Count(valueStr, "[") - strings.Count(valueStr, "]")

				if braceDepth > 0 || bracketDepth > 0 {
					// Multi-line value, start collecting
					inMultiLineValue = true
					currentKey = key
					valueBuilder.Reset()
					valueBuilder.WriteString(valueStr + "\n")
				} else {
					// Single-line value
					value := parseHCLValueToInterface(valueStr)
					node.Config[key] = value
				}
			}
		}
	}

	// Store raw config
	node.RawConfig = node.Config

	return node
}

// parseVariableBlock parses a variable block and returns a ManifestVariable
func (h *ManifestHandler) parseVariableBlock(name string, content string) models.ManifestVariable {
	variable := models.ManifestVariable{
		Name:     name,
		Type:     "string", // Default type
		Required: true,
	}

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// Parse type
		if strings.HasPrefix(trimmedLine, "type") {
			value := extractHCLValue(trimmedLine)
			variable.Type = value
		}
		// Parse description
		if strings.HasPrefix(trimmedLine, "description") {
			value := extractHCLValue(trimmedLine)
			variable.Description = value
		}
		// Parse default
		if strings.HasPrefix(trimmedLine, "default") {
			value := parseHCLValueToInterface(strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(trimmedLine, "default")), "="))
			variable.Default = value
			variable.Required = false
		}
		// Parse sensitive
		if strings.HasPrefix(trimmedLine, "sensitive") {
			value := extractHCLValue(trimmedLine)
			variable.Sensitive = value == "true"
		}
	}

	return variable
}

// extractHCLValue extracts the value from an HCL assignment line
func extractHCLValue(line string) string {
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return ""
	}
	value := strings.TrimSpace(parts[1])
	// Remove quotes
	value = strings.Trim(value, "\"")
	return value
}

// parseHCLValueToInterface parses an HCL value string to an interface{}
func parseHCLValueToInterface(value string) interface{} {
	value = strings.TrimSpace(value)

	// Boolean
	if value == "true" {
		return true
	}
	if value == "false" {
		return false
	}

	// Number
	if num, err := strconv.ParseFloat(value, 64); err == nil {
		return num
	}

	// String (quoted)
	if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
		return strings.Trim(value, "\"")
	}

	// Array
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		// Simple array parsing
		inner := strings.Trim(value, "[]")
		if inner == "" {
			return []interface{}{}
		}
		items := strings.Split(inner, ",")
		result := make([]interface{}, 0, len(items))
		for _, item := range items {
			result = append(result, parseHCLValueToInterface(strings.TrimSpace(item)))
		}
		return result
	}

	// Map/Object - parse HCL map syntax
	if strings.HasPrefix(value, "{") && strings.HasSuffix(value, "}") {
		return parseHCLMap(value)
	}

	// Variable reference (e.g., var.xxx, local.xxx, module.xxx)
	if strings.HasPrefix(value, "var.") || strings.HasPrefix(value, "local.") || strings.HasPrefix(value, "module.") {
		return value
	}

	// Default: return as string
	return value
}

// parseHCLMap parses an HCL map/object value like { key1 = "value1" key2 = "value2" }
func parseHCLMap(value string) map[string]interface{} {
	result := make(map[string]interface{})

	// Remove outer braces
	inner := strings.TrimSpace(value[1 : len(value)-1])
	if inner == "" {
		return result
	}

	// Parse key-value pairs
	// HCL format: key = value (can be on same line or multiple lines)
	// We need to handle nested structures and quoted strings

	var currentKey string
	var currentValue strings.Builder
	var inQuote bool
	var braceDepth int
	var bracketDepth int
	var expectingValue bool

	for i := 0; i < len(inner); i++ {
		c := inner[i]

		// Handle quotes
		if c == '"' && (i == 0 || inner[i-1] != '\\') {
			inQuote = !inQuote
			if expectingValue {
				currentValue.WriteByte(c)
			}
			continue
		}

		// Inside quotes, just append
		if inQuote {
			if expectingValue {
				currentValue.WriteByte(c)
			} else {
				currentKey += string(c)
			}
			continue
		}

		// Track brace/bracket depth
		if c == '{' {
			braceDepth++
			if expectingValue {
				currentValue.WriteByte(c)
			}
			continue
		}
		if c == '}' {
			braceDepth--
			if expectingValue && braceDepth >= 0 {
				currentValue.WriteByte(c)
			}
			continue
		}
		if c == '[' {
			bracketDepth++
			if expectingValue {
				currentValue.WriteByte(c)
			}
			continue
		}
		if c == ']' {
			bracketDepth--
			if expectingValue {
				currentValue.WriteByte(c)
			}
			continue
		}

		// Inside nested structure, just append
		if braceDepth > 0 || bracketDepth > 0 {
			if expectingValue {
				currentValue.WriteByte(c)
			}
			continue
		}

		// Handle equals sign
		if c == '=' && !expectingValue {
			currentKey = strings.TrimSpace(currentKey)
			expectingValue = true
			continue
		}

		// Handle newline or end of value
		if (c == '\n' || c == '\r') && expectingValue {
			// Save current key-value pair
			if currentKey != "" {
				valueStr := strings.TrimSpace(currentValue.String())
				if valueStr != "" {
					result[currentKey] = parseHCLValueToInterface(valueStr)
				}
			}
			currentKey = ""
			currentValue.Reset()
			expectingValue = false
			continue
		}

		// Append to current key or value
		if expectingValue {
			currentValue.WriteByte(c)
		} else if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			currentKey += string(c)
		}
	}

	// Handle last key-value pair
	if currentKey != "" && expectingValue {
		valueStr := strings.TrimSpace(currentValue.String())
		if valueStr != "" {
			result[currentKey] = parseHCLValueToInterface(valueStr)
		}
	}

	return result
}

// ========== Deployment Execution ==========

// executeDeployment executes deployment: create resources based on Manifest version
func (h *ManifestHandler) executeDeployment(deployment models.ManifestDeployment, version models.ManifestVersion, workspace models.Workspace, userID string) {
	// Update deployment status to deploying
	h.db.Model(&deployment).Update("status", models.DeploymentStatusDeploying)

	// Get Manifest information
	var manifest models.Manifest
	if err := h.db.Where("id = ?", deployment.ManifestID).First(&manifest).Error; err != nil {
		log.Printf("[Manifest] Failed to get manifest %s: %v", deployment.ManifestID, err)
		h.db.Model(&deployment).Update("status", models.DeploymentStatusFailed)
		return
	}

	// Parse nodes
	var nodes []models.ManifestNode
	if err := json.Unmarshal(version.Nodes, &nodes); err != nil {
		h.db.Model(&deployment).Update("status", models.DeploymentStatusFailed)
		return
	}

	// Iterate nodes, create resource for each module type node
	for _, node := range nodes {
		if node.Type != models.NodeTypeModule {
			continue
		}

		// Generate resource ID
		resourceID := fmt.Sprintf("module.%s", node.InstanceName)

		// Check if resource already exists
		var existingResource models.WorkspaceResource
		if err := h.db.Where("workspace_id = ? AND resource_id = ?", workspace.WorkspaceID, resourceID).First(&existingResource).Error; err == nil {
			// Resource already exists, update manifest_deployment_id and description
			h.db.Model(&existingResource).Updates(map[string]interface{}{
				"manifest_deployment_id": deployment.ID,
				"description":            fmt.Sprintf("Created by Manifest [%s] version [%s] deployment", manifest.Name, version.Version),
			})
			continue
		}

		// Generate TF code
		tfCode := h.generateModuleTFCode(node)

		// Extract resource type from node info
		// Format: {cloudProvider}_{moduleName}
		resourceType := h.extractResourceType(node.ModuleSource, node.ResourceName)

		// Create resource
		resource := models.WorkspaceResource{
			WorkspaceID:          workspace.WorkspaceID,
			ResourceID:           resourceID,
			ResourceType:         resourceType,
			ResourceName:         node.InstanceName,
			IsActive:             true,
			Description:          fmt.Sprintf("Created by Manifest [%s] version [%s] deployment", manifest.Name, version.Version),
			ManifestDeploymentID: &deployment.ID,
			CreatedBy:            &userID,
		}

		if err := h.db.Create(&resource).Error; err != nil {
			continue
		}

		// Create resource code version
		codeVersion := models.ResourceCodeVersion{
			ResourceID:    resource.ID,
			Version:       1,
			IsLatest:      true,
			TFCode:        tfCode,
			Variables:     models.JSONB(node.Config),
			ChangeSummary: fmt.Sprintf("Created by Manifest [%s] version [%s] deployment", manifest.Name, version.Version),
			ChangeType:    "create",
			CreatedBy:     &userID,
		}

		if err := h.db.Create(&codeVersion).Error; err != nil {
			continue
		}

		// Update resource's current version
		h.db.Model(&resource).Update("current_version_id", codeVersion.ID)

		// Re-fetch resource to calculate hash
		var savedResource models.WorkspaceResource
		if err := h.db.Where("workspace_id = ? AND resource_id = ?", workspace.WorkspaceID, resourceID).First(&savedResource).Error; err == nil {
			configHash := h.calculateResourceConfigHash(&savedResource)

			// Create deployment resource association (using semantic ID)
			deploymentResource := models.ManifestDeploymentResource{
				ID:           generateManifestDeploymentResourceID(),
				DeploymentID: deployment.ID,
				NodeID:       node.ID,
				ResourceID:   resourceID, // Use semantic ID (e.g., module.actions-41)
				ConfigHash:   configHash, // Save config hash at deployment time
			}
			h.db.Create(&deploymentResource)
		}
	}

	// Create Plan+Apply task
	taskID, err := h.createPlanAndApplyTask(workspace, userID, fmt.Sprintf("Manifest deployment: %s", deployment.ID))
	if err != nil {
		log.Printf("[Manifest] Failed to create plan+apply task for deployment %s: %v", deployment.ID, err)
		h.db.Model(&deployment).Updates(map[string]interface{}{
			"status": models.DeploymentStatusFailed,
		})
		return
	}

	// Update deployment status to deployed, and record task ID
	h.db.Model(&deployment).Updates(map[string]interface{}{
		"status":       models.DeploymentStatusDeployed,
		"last_task_id": taskID,
	})

	log.Printf("[Manifest] Deployment %s completed, created task %d", deployment.ID, taskID)
}

// createPlanAndApplyTask creates a Plan+Apply task
func (h *ManifestHandler) createPlanAndApplyTask(workspace models.Workspace, userID string, description string) (uint, error) {
	// Create task
	task := &models.WorkspaceTask{
		WorkspaceID:   workspace.WorkspaceID,
		TaskType:      models.TaskTypePlanAndApply,
		Status:        models.TaskStatusPending,
		ExecutionMode: workspace.ExecutionMode,
		CreatedBy:     &userID,
		Stage:         "pending",
		Description:   description,
	}

	if err := h.db.Create(task).Error; err != nil {
		return 0, fmt.Errorf("failed to create task: %w", err)
	}

	log.Printf("[Manifest] Created plan+apply task %d for workspace %s", task.ID, workspace.WorkspaceID)

	// Create task snapshot
	if err := h.createTaskSnapshot(task, &workspace); err != nil {
		log.Printf("[WARN] Failed to create snapshot for task %d: %v", task.ID, err)
	}

	// Trigger task execution (via queue manager)
	if h.queueManager != nil {
		go func() {
			if err := h.queueManager.TryExecuteNextTask(workspace.WorkspaceID); err != nil {
				log.Printf("[Manifest] Failed to trigger task execution for workspace %s: %v", workspace.WorkspaceID, err)
			}
		}()
	} else {
		log.Printf("[WARN] queueManager not set, task %d will be picked up by periodic checker", task.ID)
	}

	return task.ID, nil
}

// createTaskSnapshot creates a task snapshot
func (h *ManifestHandler) createTaskSnapshot(task *models.WorkspaceTask, workspace *models.Workspace) error {
	snapshotTime := time.Now()

	// 1. Snapshot resource versions
	var resources []models.WorkspaceResource
	if err := h.db.Where("workspace_id = ? AND is_active = true", workspace.WorkspaceID).
		Find(&resources).Error; err != nil {
		return fmt.Errorf("failed to get resources: %w", err)
	}

	// Load CurrentVersion for each resource
	for i := range resources {
		if resources[i].CurrentVersionID != nil {
			var version models.ResourceCodeVersion
			if err := h.db.First(&version, *resources[i].CurrentVersionID).Error; err == nil {
				resources[i].CurrentVersion = &version
			}
		}
	}

	resourceVersions := make(map[string]interface{})
	for _, r := range resources {
		if r.CurrentVersion != nil {
			resourceVersions[r.ResourceID] = map[string]interface{}{
				"resource_db_id": r.ID,
				"version_id":     r.CurrentVersion.ID,
				"version":        r.CurrentVersion.Version,
			}
		}
	}

	// 2. Snapshot variables
	var variables []models.WorkspaceVariable
	if err := h.db.Raw(`
		SELECT wv.*
		FROM workspace_variables wv
		WHERE wv.workspace_id = ? 
		  AND wv.is_deleted = false
		  AND wv.version = (
			SELECT MAX(version)
			FROM workspace_variables
			WHERE workspace_id = wv.workspace_id 
			  AND variable_id = wv.variable_id
			  AND is_deleted = false
		  )
	`, workspace.WorkspaceID).Scan(&variables).Error; err != nil {
		return fmt.Errorf("failed to get variables: %w", err)
	}

	variableSnapshots := make([]map[string]interface{}, 0, len(variables))
	for _, v := range variables {
		variableSnapshots = append(variableSnapshots, map[string]interface{}{
			"workspace_id":  v.WorkspaceID,
			"variable_id":   v.VariableID,
			"version":       v.Version,
			"variable_type": string(v.VariableType),
		})
	}

	// 3. Snapshot Provider config
	providerConfig := workspace.ProviderConfig

	// 4. Serialize and save
	resourceVersionsJSON, _ := json.Marshal(models.JSONB(resourceVersions))
	variablesJSON, _ := json.Marshal(variableSnapshots)
	providerConfigJSON, _ := json.Marshal(models.JSONB(providerConfig))

	if err := h.db.Exec(`
		UPDATE workspace_tasks 
		SET snapshot_resource_versions = ?::jsonb,
		    snapshot_variables = ?::jsonb,
		    snapshot_provider_config = ?::jsonb,
		    snapshot_created_at = ?
		WHERE id = ?
	`, resourceVersionsJSON, variablesJSON, providerConfigJSON, snapshotTime, task.ID).Error; err != nil {
		return fmt.Errorf("failed to save snapshot: %w", err)
	}

	return nil
}

// extractResourceType extracts resource type from node info
// Format: {cloudProvider}_{moduleName}
// Examples:
//   - terraform-aws-modules/ec2-instance/aws + resourceName="ec2-ff" → AWS_ec2-ff
func (h *ManifestHandler) extractResourceType(moduleSource string, resourceName string) string {
	// Prefer resourceName (module name)
	moduleName := resourceName
	if moduleName == "" {
		moduleName = "module"
	}

	// Extract cloud provider from moduleSource
	cloudProvider := "AWS" // Default AWS

	if moduleSource != "" {
		parts := strings.Split(moduleSource, "/")
		if len(parts) >= 3 {
			if strings.Contains(parts[0], ".") {
				// Private Registry format: hostname/namespace/name/provider
				// Default to AWS
				cloudProvider = "AWS"
			} else {
				// Terraform Registry format: namespace/name/provider
				provider := parts[2] // provider (e.g., aws, google, azure)
				cloudProvider = strings.ToUpper(provider)
			}
		}
	}

	return fmt.Sprintf("%s_%s", cloudProvider, moduleName)
}

// calculateResourceConfigHash calculates the hash of resource config
func (h *ManifestHandler) calculateResourceConfigHash(resource *models.WorkspaceResource) string {
	if resource == nil || resource.CurrentVersionID == nil {
		return ""
	}

	// Get current version's TF code
	var codeVersion models.ResourceCodeVersion
	if err := h.db.First(&codeVersion, *resource.CurrentVersionID).Error; err != nil {
		return ""
	}

	// Calculate SHA256 hash of TF code
	tfCodeJSON, _ := json.Marshal(codeVersion.TFCode)
	hash := sha256.Sum256(tfCodeJSON)
	return hex.EncodeToString(hash[:])
}

// generateModuleTFCode generates module TF code (Terraform JSON format)
func (h *ManifestHandler) generateModuleTFCode(node models.ManifestNode) models.JSONB {
	// Build module config
	moduleConfig := make(map[string]interface{})
	if node.ModuleSource != "" {
		moduleConfig["source"] = node.ModuleSource
	}
	if node.ModuleVersion != "" {
		moduleConfig["version"] = node.ModuleVersion
	}

	// Add config parameters (with reference conversion)
	for key, value := range node.Config {
		moduleConfig[key] = convertTerraformReferences(value)
	}

	// Return Terraform JSON format
	// Format: { "module": { "instance_name": [{ "source": "...", ... }] } }
	return models.JSONB{
		"module": map[string]interface{}{
			node.InstanceName: []interface{}{moduleConfig},
		},
	}
}

// convertTerraformReferences converts Terraform references to interpolation syntax
// e.g., "module.xxx.yyy" -> "${module.xxx.yyy}"
func convertTerraformReferences(value interface{}) interface{} {
	switch v := value.(type) {
	case string:
		// Check if it's a Terraform reference
		if strings.HasPrefix(v, "var.") || strings.HasPrefix(v, "local.") ||
			strings.HasPrefix(v, "module.") || strings.HasPrefix(v, "data.") {
			return fmt.Sprintf("${%s}", v)
		}
		return v
	case map[string]interface{}:
		// Recursively process map values
		result := make(map[string]interface{})
		for key, val := range v {
			result[key] = convertTerraformReferences(val)
		}
		return result
	case []interface{}:
		// Recursively process array values
		result := make([]interface{}, len(v))
		for i, val := range v {
			result[i] = convertTerraformReferences(val)
		}
		return result
	default:
		return v
	}
}
