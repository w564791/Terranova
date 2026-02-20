package handlers

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"iac-platform/internal/crypto"
	"iac-platform/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RunTaskHandler handles run task-related HTTP requests
type RunTaskHandler struct {
	db *gorm.DB
}

// NewRunTaskHandler creates a new run task handler
func NewRunTaskHandler(db *gorm.DB) *RunTaskHandler {
	return &RunTaskHandler{
		db: db,
	}
}

// nameRegex validates run task name (only letters, numbers, dashes and underscores)
var nameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// generateRunTaskID generates a semantic run task ID
// Format: rt-{16位随机a-z0-9}
func generateRunTaskID() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	const length = 16

	b := make([]byte, length)
	charsetLen := big.NewInt(int64(len(charset)))

	for i := range b {
		num, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			return "", fmt.Errorf("failed to generate random number: %w", err)
		}
		b[i] = charset[num.Int64()]
	}

	return fmt.Sprintf("rt-%s", string(b)), nil
}

// CreateRunTask creates a new run task
// @Summary Create run task
// @Description Create a new run task for external service integration
// @Tags Run Task
// @Accept json
// @Produce json
// @Param request body models.CreateRunTaskRequest true "Run task details"
// @Success 201 {object} models.RunTaskResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/run-tasks [post]
func (h *RunTaskHandler) CreateRunTask(c *gin.Context) {
	var req models.CreateRunTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
		})
		return
	}

	// Validate name
	if !nameRegex.MatchString(req.Name) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "name can only contain letters, numbers, dashes and underscores",
		})
		return
	}

	// Validate timeout_seconds
	if req.TimeoutSeconds != 0 && (req.TimeoutSeconds < 60 || req.TimeoutSeconds > 600) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "timeout_seconds must be between 60 and 600",
		})
		return
	}

	// Validate max_run_seconds
	if req.MaxRunSeconds != 0 && (req.MaxRunSeconds < 60 || req.MaxRunSeconds > 3600) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "max_run_seconds must be between 60 and 3600",
		})
		return
	}

	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		userID = "system"
	}

	// Generate run task ID
	runTaskID, err := generateRunTaskID()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to generate run task ID",
		})
		return
	}

	// Encrypt HMAC key if provided
	var hmacKeyEncrypted string
	if req.HMACKey != "" {
		encrypted, err := crypto.EncryptValue(req.HMACKey)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "failed to encrypt HMAC key",
			})
			return
		}
		hmacKeyEncrypted = encrypted
	}

	createdBy := userID.(string)

	// 设置全局任务默认值
	globalStages := req.GlobalStages
	if req.IsGlobal && globalStages == "" {
		globalStages = "post_plan" // 默认在 post_plan 阶段执行
	}
	globalEnforcementLevel := req.GlobalEnforcementLevel
	if req.IsGlobal && globalEnforcementLevel == "" {
		globalEnforcementLevel = models.RunTaskEnforcementAdvisory
	}

	runTask := &models.RunTask{
		RunTaskID:              runTaskID,
		Name:                   req.Name,
		Description:            req.Description,
		EndpointURL:            req.EndpointURL,
		HMACKeyEncrypted:       hmacKeyEncrypted,
		Enabled:                true,
		TimeoutSeconds:         req.TimeoutSeconds,
		MaxRunSeconds:          req.MaxRunSeconds,
		IsGlobal:               req.IsGlobal,
		GlobalStages:           globalStages,
		GlobalEnforcementLevel: globalEnforcementLevel,
		OrganizationID:         req.OrganizationID,
		TeamID:                 req.TeamID,
		CreatedBy:              &createdBy,
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
	}

	// Set defaults
	if runTask.TimeoutSeconds == 0 {
		runTask.TimeoutSeconds = 600 // 10 minutes
	}
	if runTask.MaxRunSeconds == 0 {
		runTask.MaxRunSeconds = 3600 // 60 minutes
	}

	if err := h.db.Create(runTask).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to create run task",
		})
		return
	}

	c.JSON(http.StatusCreated, runTask.ToResponse(0))
}

// ListRunTasks retrieves all run tasks
// @Summary List run tasks
// @Description Get list of all run tasks
// @Tags Run Task
// @Accept json
// @Produce json
// @Param organization_id query string false "Filter by organization ID"
// @Param team_id query string false "Filter by team ID"
// @Param enabled query boolean false "Filter by enabled status"
// @Param page query int false "Page number (default: 1)"
// @Param page_size query int false "Page size (default: 20)"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/run-tasks [get]
func (h *RunTaskHandler) ListRunTasks(c *gin.Context) {
	query := h.db.Model(&models.RunTask{})

	// Filter by organization_id
	if orgID := c.Query("organization_id"); orgID != "" {
		query = query.Where("organization_id = ?", orgID)
	}

	// Filter by team_id
	if teamID := c.Query("team_id"); teamID != "" {
		query = query.Where("team_id = ?", teamID)
	}

	// Filter by enabled status
	if enabled := c.Query("enabled"); enabled != "" {
		if enabled == "true" {
			query = query.Where("enabled = ?", true)
		} else if enabled == "false" {
			query = query.Where("enabled = ?", false)
		}
	}

	// Count total
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to count run tasks",
		})
		return
	}

	// Pagination
	page := 1
	pageSize := 20
	if p := c.Query("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}
	if ps := c.Query("page_size"); ps != "" {
		if parsed, err := strconv.Atoi(ps); err == nil && parsed > 0 && parsed <= 100 {
			pageSize = parsed
		}
	}

	offset := (page - 1) * pageSize

	var runTasks []models.RunTask
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&runTasks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to retrieve run tasks",
		})
		return
	}

	// Get workspace count for each run task
	responses := make([]models.RunTaskResponse, 0, len(runTasks))
	for _, rt := range runTasks {
		var count int64
		h.db.Model(&models.WorkspaceRunTask{}).Where("run_task_id = ?", rt.RunTaskID).Count(&count)
		responses = append(responses, rt.ToResponse(int(count)))
	}

	c.JSON(http.StatusOK, gin.H{
		"run_tasks": responses,
		"pagination": gin.H{
			"page":      page,
			"page_size": pageSize,
			"total":     total,
		},
	})
}

// GetRunTask retrieves a specific run task
// @Summary Get run task
// @Description Get detailed information about a specific run task
// @Tags Run Task
// @Accept json
// @Produce json
// @Param run_task_id path string true "Run Task ID"
// @Success 200 {object} models.RunTaskResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/run-tasks/{run_task_id} [get]
func (h *RunTaskHandler) GetRunTask(c *gin.Context) {
	runTaskID := c.Param("run_task_id")
	if runTaskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "run_task_id is required",
		})
		return
	}

	var runTask models.RunTask
	if err := h.db.Where("run_task_id = ?", runTaskID).First(&runTask).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "run task not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to retrieve run task",
		})
		return
	}

	// Get workspace count
	var count int64
	h.db.Model(&models.WorkspaceRunTask{}).Where("run_task_id = ?", runTaskID).Count(&count)

	c.JSON(http.StatusOK, runTask.ToResponse(int(count)))
}

// UpdateRunTask updates a run task
// @Summary Update run task
// @Description Update run task information
// @Tags Run Task
// @Accept json
// @Produce json
// @Param run_task_id path string true "Run Task ID"
// @Param request body models.UpdateRunTaskRequest true "Updated run task details"
// @Success 200 {object} models.RunTaskResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/run-tasks/{run_task_id} [put]
func (h *RunTaskHandler) UpdateRunTask(c *gin.Context) {
	runTaskID := c.Param("run_task_id")
	if runTaskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "run_task_id is required",
		})
		return
	}

	var req models.UpdateRunTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
		})
		return
	}

	// Check if run task exists
	var runTask models.RunTask
	if err := h.db.Where("run_task_id = ?", runTaskID).First(&runTask).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "run task not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to retrieve run task",
		})
		return
	}

	// Build updates
	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}

	if req.Name != nil {
		if !nameRegex.MatchString(*req.Name) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "name can only contain letters, numbers, dashes and underscores",
			})
			return
		}
		updates["name"] = *req.Name
	}

	if req.Description != nil {
		updates["description"] = *req.Description
	}

	if req.EndpointURL != nil {
		updates["endpoint_url"] = *req.EndpointURL
	}

	if req.HMACKey != nil {
		if *req.HMACKey == "" {
			// Clear HMAC key
			updates["hmac_key_encrypted"] = ""
		} else {
			// Encrypt new HMAC key
			encrypted, err := crypto.EncryptValue(*req.HMACKey)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": "failed to encrypt HMAC key",
				})
				return
			}
			updates["hmac_key_encrypted"] = encrypted
		}
	}

	if req.TimeoutSeconds != nil {
		if *req.TimeoutSeconds < 60 || *req.TimeoutSeconds > 600 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "timeout_seconds must be between 60 and 600",
			})
			return
		}
		updates["timeout_seconds"] = *req.TimeoutSeconds
	}

	if req.MaxRunSeconds != nil {
		if *req.MaxRunSeconds < 60 || *req.MaxRunSeconds > 3600 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "max_run_seconds must be between 60 and 3600",
			})
			return
		}
		updates["max_run_seconds"] = *req.MaxRunSeconds
	}

	if req.IsGlobal != nil {
		updates["is_global"] = *req.IsGlobal
	}

	if req.GlobalStages != nil {
		updates["global_stages"] = *req.GlobalStages
	}

	if req.GlobalEnforcementLevel != nil {
		updates["global_enforcement_level"] = *req.GlobalEnforcementLevel
	}

	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}

	if err := h.db.Model(&runTask).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to update run task",
		})
		return
	}

	// Reload run task
	h.db.Where("run_task_id = ?", runTaskID).First(&runTask)

	// Get workspace count
	var count int64
	h.db.Model(&models.WorkspaceRunTask{}).Where("run_task_id = ?", runTaskID).Count(&count)

	c.JSON(http.StatusOK, runTask.ToResponse(int(count)))
}

// DeleteRunTask deletes a run task
// @Summary Delete run task
// @Description Delete a run task (only if no workspaces are using it)
// @Tags Run Task
// @Accept json
// @Produce json
// @Param run_task_id path string true "Run Task ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 409 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/run-tasks/{run_task_id} [delete]
func (h *RunTaskHandler) DeleteRunTask(c *gin.Context) {
	runTaskID := c.Param("run_task_id")
	if runTaskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "run_task_id is required",
		})
		return
	}

	// Check if run task has workspace associations
	var count int64
	h.db.Model(&models.WorkspaceRunTask{}).Where("run_task_id = ?", runTaskID).Count(&count)
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error":           "cannot delete run task with workspace associations",
			"workspace_count": count,
		})
		return
	}

	// Delete run task
	result := h.db.Where("run_task_id = ?", runTaskID).Delete(&models.RunTask{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to delete run task",
		})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "run task not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "run task deleted successfully",
	})
}

// GetDecryptedHMACKey retrieves the decrypted HMAC key for internal use
// This is not exposed as an API endpoint
func (h *RunTaskHandler) GetDecryptedHMACKey(runTaskID string) (string, error) {
	var runTask models.RunTask
	if err := h.db.Where("run_task_id = ?", runTaskID).First(&runTask).Error; err != nil {
		return "", err
	}

	if runTask.HMACKeyEncrypted == "" {
		return "", nil
	}

	return crypto.DecryptValue(runTask.HMACKeyEncrypted)
}

// TestRunTaskRequest 测试 Run Task 连接请求
type TestRunTaskRequest struct {
	EndpointURL string `json:"endpoint_url" binding:"required"`
	HMACKey     string `json:"hmac_key"`
}

// TestRunTask tests the connection to a run task endpoint
// @Summary Test run task connection
// @Description Test the connection to a run task endpoint and verify HMAC configuration
// @Tags Run Task
// @Accept json
// @Produce json
// @Param request body TestRunTaskRequest true "Test connection details"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/run-tasks/test [post]
func (h *RunTaskHandler) TestRunTask(c *gin.Context) {
	var req TestRunTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
		})
		return
	}

	// Build test payload
	testPayload := map[string]interface{}{
		"payload_version":               1,
		"stage":                         "test",
		"access_token":                  "test-token",
		"task_result_id":                "test-result-id",
		"task_result_callback_url":      "https://example.com/callback",
		"task_result_enforcement_level": "advisory",
		"task_id":                       0,
		"task_type":                     "test",
		"task_status":                   "test",
		"task_description":              "Connection test from IaC Platform",
		"task_created_at":               time.Now().Format(time.RFC3339),
		"task_app_url":                  "https://example.com/test",
		"workspace_id":                  "test-workspace",
		"timeout_seconds":               60,
		"is_test":                       true,
		"capabilities": map[string]interface{}{
			"outcomes": true,
		},
	}

	payloadBytes, err := json.Marshal(testPayload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to marshal test payload",
		})
		return
	}

	// Create HTTP request
	httpReq, err := http.NewRequest(http.MethodPost, req.EndpointURL, bytes.NewReader(payloadBytes))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":       false,
			"error":         "invalid endpoint URL",
			"error_details": err.Error(),
		})
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "IaC-Platform/1.0 (Connection Test)")

	// Add HMAC signature if provided
	if req.HMACKey != "" {
		signature := calculateHMACSignature(payloadBytes, req.HMACKey)
		httpReq.Header.Set("X-TFC-Task-Signature", "sha512="+signature)
	}

	// Send request with timeout
	client := &http.Client{Timeout: 10 * time.Second}
	startTime := time.Now()
	resp, err := client.Do(httpReq)
	duration := time.Since(startTime)

	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success":       false,
			"error":         "failed to connect to endpoint",
			"error_details": err.Error(),
			"duration_ms":   duration.Milliseconds(),
		})
		return
	}
	defer resp.Body.Close()

	// Read response body
	respBody, _ := io.ReadAll(resp.Body)

	// Check response status
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		c.JSON(http.StatusOK, gin.H{
			"success":         false,
			"error":           fmt.Sprintf("endpoint returned status %d", resp.StatusCode),
			"status_code":     resp.StatusCode,
			"response_body":   string(respBody),
			"duration_ms":     duration.Milliseconds(),
			"hmac_configured": req.HMACKey != "",
		})
		return
	}

	// Parse response to check for HMAC verification result
	var respData map[string]interface{}
	json.Unmarshal(respBody, &respData)

	// Check if the response indicates HMAC verification failure
	hmacVerified := true
	if hmacError, ok := respData["hmac_error"].(string); ok && hmacError != "" {
		hmacVerified = false
	}

	c.JSON(http.StatusOK, gin.H{
		"success":         true,
		"message":         "Connection test successful",
		"status_code":     resp.StatusCode,
		"response_body":   string(respBody),
		"duration_ms":     duration.Milliseconds(),
		"hmac_configured": req.HMACKey != "",
		"hmac_verified":   hmacVerified,
	})
}

// TestExistingRunTask tests the connection to an existing run task
// @Summary Test existing run task connection
// @Description Test the connection to an existing run task endpoint (appends /test to endpoint URL)
// @Tags Run Task
// @Accept json
// @Produce json
// @Param run_task_id path string true "Run Task ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/run-tasks/{run_task_id}/test [post]
func (h *RunTaskHandler) TestExistingRunTask(c *gin.Context) {
	runTaskID := c.Param("run_task_id")
	if runTaskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "run_task_id is required",
		})
		return
	}

	// Get run task
	var runTask models.RunTask
	if err := h.db.Where("run_task_id = ?", runTaskID).First(&runTask).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "run task not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to retrieve run task",
		})
		return
	}

	// Decrypt HMAC key if present
	var hmacKey string
	if runTask.HMACKeyEncrypted != "" {
		decrypted, err := crypto.DecryptValue(runTask.HMACKeyEncrypted)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "failed to decrypt HMAC key",
			})
			return
		}
		hmacKey = decrypted
	}

	// Build test URL: append /test to endpoint URL
	testURL := runTask.EndpointURL
	if testURL[len(testURL)-1] == '/' {
		testURL = testURL + "test"
	} else {
		testURL = testURL + "/test"
	}

	// Build test payload
	testPayload := map[string]interface{}{
		"payload_version":               1,
		"stage":                         "test",
		"access_token":                  "test-token",
		"task_result_id":                "test-result-id",
		"task_result_callback_url":      "https://example.com/callback",
		"task_result_enforcement_level": "advisory",
		"task_id":                       0,
		"task_type":                     "test",
		"task_status":                   "test",
		"task_description":              "Connection test from IaC Platform",
		"task_created_at":               time.Now().Format(time.RFC3339),
		"task_app_url":                  "https://example.com/test",
		"workspace_id":                  "test-workspace",
		"timeout_seconds":               60,
		"is_test":                       true,
		"capabilities": map[string]interface{}{
			"outcomes": true,
		},
	}

	payloadBytes, err := json.Marshal(testPayload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to marshal test payload",
		})
		return
	}

	// Create HTTP request to test URL
	httpReq, err := http.NewRequest(http.MethodPost, testURL, bytes.NewReader(payloadBytes))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":       false,
			"error":         "invalid endpoint URL",
			"error_details": err.Error(),
		})
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "IaC-Platform/1.0 (Connection Test)")

	// Add HMAC signature if configured
	if hmacKey != "" {
		signature := calculateHMACSignature(payloadBytes, hmacKey)
		httpReq.Header.Set("X-TFC-Task-Signature", "sha512="+signature)
	}

	// Send request with timeout
	client := &http.Client{Timeout: 10 * time.Second}
	startTime := time.Now()
	resp, err := client.Do(httpReq)
	duration := time.Since(startTime)

	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success":       false,
			"error":         "failed to connect to endpoint",
			"error_details": err.Error(),
			"duration_ms":   duration.Milliseconds(),
			"run_task_id":   runTaskID,
			"endpoint_url":  runTask.EndpointURL,
		})
		return
	}
	defer resp.Body.Close()

	// Read response body
	respBody, _ := io.ReadAll(resp.Body)

	// Check response status
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		c.JSON(http.StatusOK, gin.H{
			"success":         false,
			"error":           fmt.Sprintf("endpoint returned status %d", resp.StatusCode),
			"status_code":     resp.StatusCode,
			"response_body":   string(respBody),
			"duration_ms":     duration.Milliseconds(),
			"run_task_id":     runTaskID,
			"endpoint_url":    runTask.EndpointURL,
			"hmac_configured": hmacKey != "",
		})
		return
	}

	// Parse response to check for HMAC verification result
	var respData map[string]interface{}
	json.Unmarshal(respBody, &respData)

	// Check if the response indicates HMAC verification failure
	hmacVerified := true
	if hmacError, ok := respData["hmac_error"].(string); ok && hmacError != "" {
		hmacVerified = false
	}

	c.JSON(http.StatusOK, gin.H{
		"success":         true,
		"message":         "Connection test successful",
		"status_code":     resp.StatusCode,
		"response_body":   string(respBody),
		"duration_ms":     duration.Milliseconds(),
		"run_task_id":     runTaskID,
		"endpoint_url":    runTask.EndpointURL,
		"test_url":        testURL,
		"hmac_configured": hmacKey != "",
		"hmac_verified":   hmacVerified,
	})
}

// calculateHMACSignature calculates HMAC-SHA512 signature
func calculateHMACSignature(payload []byte, key string) string {
	h := hmac.New(sha512.New, []byte(key))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}
