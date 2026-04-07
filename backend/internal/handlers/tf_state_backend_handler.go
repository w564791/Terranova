package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"iac-platform/internal/models"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const maxStateSize = 50 << 20 // 50MB

type TFStateBackendHandler struct {
	db           *gorm.DB
	tokenService *services.StateTokenService
}

func NewTFStateBackendHandler(db *gorm.DB, tokenService *services.StateTokenService) *TFStateBackendHandler {
	return &TFStateBackendHandler{db: db, tokenService: tokenService}
}

// GetState GET /api/v1/terraform/state/:workspace_id
func (h *TFStateBackendHandler) GetState(c *gin.Context) {
	workspaceID := c.GetString("state_workspace_id")

	// Cross-workspace access: return filtered state with only configured outputs
	if crossWs, exists := c.Get("cross_workspace"); exists && crossWs == true {
		requesterWsID := c.GetString("requester_workspace_id")
		h.getCrossWorkspaceState(c, workspaceID, requesterWsID)
		return
	}

	var stateVersion models.WorkspaceStateVersion
	err := h.db.Where("workspace_id = ?", workspaceID).
		Order("version DESC").
		First(&stateVersion).Error

	if err == gorm.ErrRecordNotFound {
		c.Status(http.StatusNotFound)
		return
	}
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to get state"})
		return
	}

	stateBytes, err := json.Marshal(stateVersion.Content)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to marshal state"})
		return
	}

	c.Data(http.StatusOK, "application/json", stateBytes)
}

// getCrossWorkspaceState returns a filtered state containing only configured outputs.
// Validates sharing permission before returning data.
func (h *TFStateBackendHandler) getCrossWorkspaceState(c *gin.Context, targetWsID, requesterWsID string) {
	// 1. Check sharing permission
	var targetWs models.Workspace
	if err := h.db.Select("workspace_id, outputs_sharing").
		Where("workspace_id = ?", targetWsID).First(&targetWs).Error; err != nil {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return
	}

	allowed := false
	switch models.OutputsSharingMode(targetWs.OutputsSharing) {
	case models.OutputsSharingAll:
		allowed = true
	case models.OutputsSharingSpecific:
		var count int64
		h.db.Model(&models.WorkspaceOutputsAccess{}).
			Where("workspace_id = ? AND allowed_workspace_id = ?", targetWsID, requesterWsID).
			Count(&count)
		allowed = count > 0
	}

	if !allowed {
		log.Printf("[CrossWorkspaceState] Denied: %s -> %s (sharing=%s)", requesterWsID, targetWsID, targetWs.OutputsSharing)
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "workspace does not share outputs with requester"})
		return
	}

	// 2. Get latest state version
	var stateVersion models.WorkspaceStateVersion
	if err := h.db.Where("workspace_id = ?", targetWsID).
		Order("version DESC").First(&stateVersion).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.Status(http.StatusNotFound)
			return
		}
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to get state"})
		return
	}

	// 3. Get configured outputs to build allowed key set
	var configuredOutputs []models.WorkspaceOutput
	h.db.Where("workspace_id = ?", targetWsID).Find(&configuredOutputs)

	allowedKeys := make(map[string]bool, len(configuredOutputs))
	for _, o := range configuredOutputs {
		allowedKeys[o.StateKey()] = true
	}

	// 4. Filter state outputs: only include configured keys
	filteredOutputs := make(map[string]interface{})
	if stateVersion.Content != nil {
		if outputs, ok := stateVersion.Content["outputs"].(map[string]interface{}); ok {
			for key, val := range outputs {
				if allowedKeys[key] {
					filteredOutputs[key] = val
				}
			}
		}
	}

	// 5. Build standard Terraform state format (resources stripped for security)
	filteredState := map[string]interface{}{
		"version":           stateVersion.Content["version"],
		"terraform_version": stateVersion.Content["terraform_version"],
		"serial":            stateVersion.Content["serial"],
		"lineage":           stateVersion.Content["lineage"],
		"outputs":           filteredOutputs,
		"resources":         []interface{}{},
	}

	stateBytes, err := json.Marshal(filteredState)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to marshal filtered state"})
		return
	}

	log.Printf("[CrossWorkspaceState] Allowed: %s -> %s (%d/%d outputs exposed)",
		requesterWsID, targetWsID, len(filteredOutputs), len(allowedKeys))
	c.Data(http.StatusOK, "application/json", stateBytes)
}

// UpdateState POST /api/v1/terraform/state/:workspace_id
// Validates lock ID from ?ID= query param, deduplicates by checksum.
func (h *TFStateBackendHandler) UpdateState(c *gin.Context) {
	workspaceID := c.GetString("state_workspace_id")
	taskID, _ := c.Get("state_task_id")
	taskIDUint := taskID.(uint)

	// Verify lock ID if provided (Terraform appends ?ID=xxx when holding a lock)
	if lockID := c.Query("ID"); lockID != "" {
		var ws models.Workspace
		if err := h.db.Select("lock_id").Where("workspace_id = ?", workspaceID).First(&ws).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to check lock"})
			return
		}
		if ws.LockID == nil || *ws.LockID != lockID {
			c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": "lock ID mismatch"})
			return
		}
	}

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxStateSize))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}
	if len(body) == 0 {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "empty body"})
		return
	}

	var stateContent map[string]interface{}
	if err := json.Unmarshal(body, &stateContent); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
		return
	}

	hash := sha256.Sum256(body)
	checksum := hex.EncodeToString(hash[:])

	lineage, _ := stateContent["lineage"].(string)
	newSerial := 0
	if s, ok := stateContent["serial"].(float64); ok {
		newSerial = int(s)
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		var current struct {
			MaxVersion int
			MaxSerial  int
			Checksum   string
		}
		tx.Model(&models.WorkspaceStateVersion{}).
			Where("workspace_id = ?", workspaceID).
			Select("COALESCE(MAX(version), 0) as max_version, COALESCE(MAX(serial), 0) as max_serial").
			Scan(&current)
		// Get checksum of latest version for dedup
		tx.Model(&models.WorkspaceStateVersion{}).
			Where("workspace_id = ? AND version = ?", workspaceID, current.MaxVersion).
			Select("checksum").Scan(&current.Checksum)

		if newSerial < current.MaxSerial {
			return fmt.Errorf("serial regression: got %d, current is %d", newSerial, current.MaxSerial)
		}

		// Checksum dedup: skip creating new version if content is identical
		if current.Checksum == checksum && current.MaxVersion > 0 {
			log.Printf("State unchanged for workspace %s (checksum match), skipping version creation", workspaceID)
			return nil
		}

		stateVersion := &models.WorkspaceStateVersion{
			WorkspaceID: workspaceID,
			Version:     current.MaxVersion + 1,
			Content:     stateContent,
			Checksum:    checksum,
			SizeBytes:   len(body),
			Lineage:     lineage,
			Serial:      newSerial,
			TaskID:      &taskIDUint,
		}

		var task models.WorkspaceTask
		if tx.Select("created_by").First(&task, taskIDUint).Error == nil {
			stateVersion.CreatedBy = task.CreatedBy
		}

		if err := tx.Create(stateVersion).Error; err != nil {
			return err
		}

		// [2026-04-07] Disabled: tf_state column is write-only (0 reads across entire codebase).
		// State is served from workspace_state_versions table. Commenting out to reduce write IO.
		// Safe to delete after confirming no regressions.
		// return tx.Model(&models.Workspace{}).
		// 	Where("workspace_id = ?", workspaceID).
		// 	Update("tf_state", stateContent).Error
		return nil
	})

	if err != nil {
		log.Printf("ERROR: Failed to save state for workspace %s: %v", workspaceID, err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to save state"})
		return
	}

	log.Printf("State updated for workspace %s via HTTP backend (serial=%d)", workspaceID, newSerial)
	c.Status(http.StatusOK)
}

// LockState POST /api/v1/terraform/state/:workspace_id/lock
func (h *TFStateBackendHandler) LockState(c *gin.Context) {
	workspaceID := c.GetString("state_workspace_id")

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "failed to read lock info"})
		return
	}

	var lockInfo map[string]interface{}
	if err := json.Unmarshal(body, &lockInfo); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid lock info"})
		return
	}

	lockID, _ := lockInfo["ID"].(string)
	if lockID == "" {
		lockID = uuid.New().String()
		lockInfo["ID"] = lockID
	}

	lockInfoJSON := models.JSONB(lockInfo)

	result := h.db.Model(&models.Workspace{}).
		Where("workspace_id = ? AND lock_id IS NULL", workspaceID).
		Updates(map[string]interface{}{
			"lock_id":   lockID,
			"lock_info": lockInfoJSON,
		})

	if result.Error != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "lock failed"})
		return
	}

	if result.RowsAffected == 0 {
		var workspace models.Workspace
		h.db.Select("lock_id, lock_info").Where("workspace_id = ?", workspaceID).First(&workspace)
		c.JSON(http.StatusConflict, workspace.LockInfo)
		return
	}

	c.Status(http.StatusOK)
}

// UnlockState POST /api/v1/terraform/state/:workspace_id/unlock
func (h *TFStateBackendHandler) UnlockState(c *gin.Context) {
	workspaceID := c.GetString("state_workspace_id")

	var lockID string
	body, err := io.ReadAll(c.Request.Body)
	if err == nil && len(body) > 0 {
		var lockInfo map[string]interface{}
		if json.Unmarshal(body, &lockInfo) == nil {
			lockID, _ = lockInfo["ID"].(string)
		}
	}

	var result *gorm.DB
	if lockID != "" {
		result = h.db.Model(&models.Workspace{}).
			Where("workspace_id = ? AND lock_id = ?", workspaceID, lockID).
			Updates(map[string]interface{}{
				"lock_id":   nil,
				"lock_info": nil,
			})
	} else {
		result = h.db.Model(&models.Workspace{}).
			Where("workspace_id = ?", workspaceID).
			Updates(map[string]interface{}{
				"lock_id":   nil,
				"lock_info": nil,
			})
	}

	if result.Error != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "unlock failed"})
		return
	}

	if result.RowsAffected == 0 && lockID != "" {
		var workspace models.Workspace
		h.db.Select("lock_id, lock_info").Where("workspace_id = ?", workspaceID).First(&workspace)
		c.JSON(http.StatusConflict, workspace.LockInfo)
		return
	}

	c.Status(http.StatusOK)
}

// DeleteState DELETE /api/v1/terraform/state/:workspace_id
// Not supported - returns 405 Method Not Allowed
func (h *TFStateBackendHandler) DeleteState(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusMethodNotAllowed, gin.H{
		"error": "state deletion via HTTP backend is not supported, use the UI to manage state versions",
	})
}
