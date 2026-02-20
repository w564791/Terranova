package handlers

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"time"

	"iac-platform/internal/application/service"
	"iac-platform/internal/models"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AgentPoolHandler handles agent pool-related HTTP requests
type AgentPoolHandler struct {
	db               *gorm.DB
	poolTokenService *service.PoolTokenService
}

// NewAgentPoolHandler creates a new agent pool handler
func NewAgentPoolHandler(db *gorm.DB) *AgentPoolHandler {
	return &AgentPoolHandler{
		db:               db,
		poolTokenService: service.NewPoolTokenService(db),
	}
}

// CreateAgentPool creates a new agent pool
// @Summary Create agent pool
// @Description Create a new agent pool for organizing agents
// @Tags Agent Pool
// @Accept json
// @Produce json
// @Param request body models.CreateAgentPoolRequest true "Agent pool details"
// @Success 201 {object} models.AgentPool
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agent-pools [post]
func (h *AgentPoolHandler) CreateAgentPool(c *gin.Context) {
	var req models.CreateAgentPoolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
		})
		return
	}

	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		userID = "system"
	}

	// Generate pool ID
	poolID, err := generatePoolID()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to generate pool ID",
		})
		return
	}

	createdBy := userID.(string)
	pool := &models.AgentPool{
		PoolID:      poolID,
		Name:        req.Name,
		Description: req.Description,
		PoolType:    req.PoolType,
		CreatedBy:   &createdBy,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := h.db.Create(pool).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to create agent pool",
		})
		return
	}

	c.JSON(http.StatusCreated, pool)
}

// ListAgentPools retrieves all agent pools
// @Summary List agent pools
// @Description Get list of all agent pools
// @Tags Agent Pool
// @Accept json
// @Produce json
// @Param is_active query boolean false "Filter by active status"
// @Success 200 {object} models.AgentPoolListResponse
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agent-pools [get]
func (h *AgentPoolHandler) ListAgentPools(c *gin.Context) {
	query := h.db.Model(&models.AgentPool{})

	// Filter by status if provided
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	var pools []models.AgentPool
	if err := query.Order("created_at DESC").Find(&pools).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to retrieve agent pools",
		})
		return
	}

	// Get agent count for each pool
	type PoolWithCount struct {
		models.AgentPool
		AgentCount int `json:"agent_count"`
	}

	poolsWithCount := make([]PoolWithCount, 0, len(pools))
	for _, pool := range pools {
		var count int64
		h.db.Model(&models.Agent{}).Where("pool_id = ?", pool.PoolID).Count(&count)
		poolsWithCount = append(poolsWithCount, PoolWithCount{
			AgentPool:  pool,
			AgentCount: int(count),
		})
	}

	c.JSON(http.StatusOK, models.AgentPoolListResponse{
		Pools: poolsWithCount,
		Total: len(poolsWithCount),
	})
}

// GetAgentPool retrieves a specific agent pool
// @Summary Get agent pool
// @Description Get detailed information about a specific agent pool
// @Tags Agent Pool
// @Accept json
// @Produce json
// @Param pool_id path string true "Pool ID"
// @Param include_offline query boolean false "Include offline agents (default: false)"
// @Success 200 {object} models.AgentPoolDetailResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agent-pools/{pool_id} [get]
func (h *AgentPoolHandler) GetAgentPool(c *gin.Context) {
	poolID := c.Param("pool_id")
	if poolID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "pool_id is required",
		})
		return
	}

	var pool models.AgentPool
	if err := h.db.Where("pool_id = ?", poolID).First(&pool).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "agent pool not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to retrieve agent pool",
		})
		return
	}

	// Get agents in this pool
	// By default, exclude offline agents unless include_offline=true
	includeOffline := c.Query("include_offline") == "true"

	query := h.db.Where("pool_id = ?", poolID)
	if !includeOffline {
		query = query.Where("status != ?", models.AgentStatusOffline)
	}

	var agents []models.Agent
	query.Order("status ASC, last_ping_at DESC").Find(&agents)

	// Update agent status in real-time based on last_ping_at
	// An agent is considered offline if last ping was more than 2 minutes ago
	now := time.Now()
	for i := range agents {
		if agents[i].LastPingAt != nil {
			timeSinceLastPing := now.Sub(*agents[i].LastPingAt)
			if timeSinceLastPing > 2*time.Minute {
				// Agent hasn't pinged in over 2 minutes, mark as offline
				agents[i].Status = models.AgentStatusOffline
			}
		} else {
			// No last ping recorded, mark as offline
			agents[i].Status = models.AgentStatusOffline
		}
	}

	c.JSON(http.StatusOK, models.AgentPoolDetailResponse{
		Pool:   pool,
		Agents: agents,
		Total:  len(agents),
	})
}

// UpdateAgentPool updates an agent pool
// @Summary Update agent pool
// @Description Update agent pool information
// @Tags Agent Pool
// @Accept json
// @Produce json
// @Param pool_id path string true "Pool ID"
// @Param request body models.UpdateAgentPoolRequest true "Updated pool details"
// @Success 200 {object} models.AgentPool
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agent-pools/{pool_id} [put]
func (h *AgentPoolHandler) UpdateAgentPool(c *gin.Context) {
	poolID := c.Param("pool_id")
	if poolID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "pool_id is required",
		})
		return
	}

	var req models.UpdateAgentPoolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
		})
		return
	}

	// Check if pool exists
	var pool models.AgentPool
	if err := h.db.Where("pool_id = ?", poolID).First(&pool).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "agent pool not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to retrieve agent pool",
		})
		return
	}

	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		userID = "system"
	}

	updatedBy := userID.(string)
	// Update fields
	updates := map[string]interface{}{
		"updated_at": time.Now(),
		"updated_by": &updatedBy,
	}

	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = req.Description
	}
	if req.IsActive != nil {
		// Map IsActive to Status
		if *req.IsActive {
			updates["status"] = models.AgentPoolStatusActive
		} else {
			updates["status"] = models.AgentPoolStatusInactive
		}
	}

	if err := h.db.Model(&pool).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to update agent pool",
		})
		return
	}

	// Reload pool
	h.db.Where("pool_id = ?", poolID).First(&pool)

	c.JSON(http.StatusOK, pool)
}

// DeleteAgentPool deletes an agent pool
// @Summary Delete agent pool
// @Description Delete an agent pool (only if no agents are assigned)
// @Tags Agent Pool
// @Accept json
// @Produce json
// @Param pool_id path string true "Pool ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 409 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agent-pools/{pool_id} [delete]
func (h *AgentPoolHandler) DeleteAgentPool(c *gin.Context) {
	poolID := c.Param("pool_id")
	if poolID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "pool_id is required",
		})
		return
	}

	// Check if pool has agents
	var count int64
	h.db.Model(&models.Agent{}).Where("pool_id = ?", poolID).Count(&count)
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error":       "cannot delete pool with assigned agents",
			"agent_count": count,
		})
		return
	}

	// Delete pool
	result := h.db.Where("pool_id = ?", poolID).Delete(&models.AgentPool{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to delete agent pool",
		})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "agent pool not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "agent pool deleted successfully",
	})
}

// generatePoolID generates a semantic pool ID using crypto/rand
// Format: pool-{16位随机a-z0-9}
func generatePoolID() (string, error) {
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

	return fmt.Sprintf("pool-%s", string(b)), nil
}

// ===== Pool Token Management Handlers =====

// CreatePoolToken creates a new static token for an agent pool
// @Summary Create pool token
// @Description Create a new static token for agent pool authentication
// @Tags Agent Pool
// @Accept json
// @Produce json
// @Param pool_id path string true "Pool ID"
// @Param request body models.CreatePoolTokenRequest true "Token details"
// @Success 201 {object} models.PoolTokenCreateResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agent-pools/{pool_id}/tokens [post]
func (h *AgentPoolHandler) CreatePoolToken(c *gin.Context) {
	poolID := c.Param("pool_id")
	if poolID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "pool_id is required",
		})
		return
	}

	var req models.CreatePoolTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
		})
		return
	}

	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		userID = "system"
	}

	// Parse expires_at if provided
	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "invalid expires_at format, use ISO 8601",
			})
			return
		}
		expiresAt = &t
	}

	// Generate token
	token, err := h.poolTokenService.GenerateStaticToken(
		c.Request.Context(),
		poolID,
		req.TokenName,
		userID.(string),
		expiresAt,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, token)
}

// ListPoolTokens lists all tokens for an agent pool
// @Summary List pool tokens
// @Description Get list of all tokens for an agent pool
// @Tags Agent Pool
// @Accept json
// @Produce json
// @Param pool_id path string true "Pool ID"
// @Success 200 {object} models.PoolTokenListResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agent-pools/{pool_id}/tokens [get]
func (h *AgentPoolHandler) ListPoolTokens(c *gin.Context) {
	poolID := c.Param("pool_id")
	if poolID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "pool_id is required",
		})
		return
	}

	tokens, err := h.poolTokenService.ListPoolTokens(c.Request.Context(), poolID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to list tokens",
		})
		return
	}

	c.JSON(http.StatusOK, models.PoolTokenListResponse{
		Tokens: tokens,
		Total:  len(tokens),
	})
}

// RevokePoolToken revokes a pool token
// For K8s pools, this will automatically generate a new token, update Secret, and rebuild Pods
// @Summary Revoke pool token
// @Description Revoke a pool token by token name. For K8s pools, automatically generates new token and rebuilds Pods.
// @Tags Agent Pool
// @Accept json
// @Produce json
// @Param pool_id path string true "Pool ID"
// @Param token_name path string true "Token Name"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agent-pools/{pool_id}/tokens/{token_name} [delete]
func (h *AgentPoolHandler) RevokePoolToken(c *gin.Context) {
	poolID := c.Param("pool_id")
	tokenName := c.Param("token_name")

	if poolID == "" || tokenName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "pool_id and token_name are required",
		})
		return
	}

	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		userID = "system"
	}

	// Get pool to check type
	var pool models.AgentPool
	if err := h.db.Where("pool_id = ?", poolID).First(&pool).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "agent pool not found",
		})
		return
	}

	// For K8s pools, revoke is essentially a rotation (revoke old + generate new + update secret + rebuild pods)
	if pool.PoolType == models.AgentPoolTypeK8s {
		// Call K8s deployment service to perform rotation
		k8sDeploymentService, err := services.NewK8sDeploymentService(h.db)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "K8s deployment service not available",
			})
			return
		}

		// Perform rotation (which includes revoke + generate new + update secret + rebuild)
		if err := k8sDeploymentService.ForceRotateToken(c.Request.Context(), &pool, userID.(string)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "token revoked and new token generated, Pods will be rebuilt with new token",
		})
		return
	}

	// For static pools, check if this is the last active token
	var activeTokenCount int64
	if err := h.db.Model(&models.PoolToken{}).
		Where("pool_id = ? AND is_active = ?", poolID, true).
		Count(&activeTokenCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to count active tokens",
		})
		return
	}

	if activeTokenCount <= 1 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "cannot revoke the last active token, please create a new token first",
		})
		return
	}

	// For static pools, just revoke the token
	err := h.poolTokenService.RevokeToken(
		c.Request.Context(),
		poolID,
		tokenName,
		userID.(string),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "token revoked successfully",
	})
}

// UpdateK8sConfig updates the K8s configuration for a pool
// @Summary Update K8s config
// @Description Update K8s Job template configuration for a K8s agent pool
// @Tags Agent Pool
// @Accept json
// @Produce json
// @Param pool_id path string true "Pool ID"
// @Param request body models.UpdateK8sConfigRequest true "K8s configuration"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agent-pools/{pool_id}/k8s-config [put]
func (h *AgentPoolHandler) UpdateK8sConfig(c *gin.Context) {
	poolID := c.Param("pool_id")
	if poolID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "pool_id is required",
		})
		return
	}

	var req models.UpdateK8sConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
		})
		return
	}

	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		userID = "system"
	}

	// Update K8s config in database
	err := h.poolTokenService.UpdateK8sConfig(
		c.Request.Context(),
		poolID,
		req.K8sConfig,
		userID.(string),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Get pool to pass to K8s deployment service
	var pool models.AgentPool
	if err := h.db.Where("pool_id = ?", poolID).First(&pool).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to retrieve updated pool",
		})
		return
	}

	// Rebuild idle pods with new configuration
	// This will delete all idle pods and recreate them with the updated config
	k8sDeploymentService, err := services.NewK8sDeploymentService(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "K8s deployment service not available",
		})
		return
	}

	// Call RebuildIdlePods to delete and recreate idle pods with new config
	if err := k8sDeploymentService.RebuildIdlePods(c.Request.Context(), &pool); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("K8s config updated but failed to rebuild idle pods: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "K8s configuration updated successfully and idle pods are being rebuilt",
	})
}

// GetK8sConfig retrieves the K8s configuration for a pool
// @Summary Get K8s config
// @Description Get K8s Job template configuration for a K8s agent pool
// @Tags Agent Pool
// @Accept json
// @Produce json
// @Param pool_id path string true "Pool ID"
// @Success 200 {object} models.K8sJobTemplateConfig
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agent-pools/{pool_id}/k8s-config [get]
func (h *AgentPoolHandler) GetK8sConfig(c *gin.Context) {
	poolID := c.Param("pool_id")
	if poolID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "pool_id is required",
		})
		return
	}

	config, err := h.poolTokenService.GetK8sConfig(c.Request.Context(), poolID)
	if err != nil {
		// Return 400 for pool type mismatch, 404 for not found, 500 for other errors
		if err.Error() == "pool is not a K8s agent pool" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
		} else if err.Error() == "agent pool not found" {
			c.JSON(http.StatusNotFound, gin.H{
				"error": err.Error(),
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
		}
		return
	}

	c.JSON(http.StatusOK, config)
}

// RotatePoolToken rotates a pool token (for K8s pools)
// @Summary Rotate pool token
// @Description Rotate a pool token, update K8s secret, and restart deployment
// @Tags Agent Pool
// @Accept json
// @Produce json
// @Param pool_id path string true "Pool ID"
// @Param token_name path string true "Token Name"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agent-pools/{pool_id}/tokens/{token_name}/rotate [post]
func (h *AgentPoolHandler) RotatePoolToken(c *gin.Context) {
	poolID := c.Param("pool_id")
	tokenName := c.Param("token_name")

	if poolID == "" || tokenName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "pool_id and token_name are required",
		})
		return
	}

	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		userID = "system"
	}

	// Call K8s deployment service to rotate token
	k8sDeploymentService, err := services.NewK8sDeploymentService(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "K8s deployment service not available",
		})
		return
	}

	// Get pool
	var pool models.AgentPool
	if err := h.db.Where("pool_id = ?", poolID).First(&pool).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "agent pool not found",
		})
		return
	}

	// Verify it's a K8s pool
	if pool.PoolType != models.AgentPoolTypeK8s {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "can only rotate tokens for K8s agent pools",
		})
		return
	}

	// Perform rotation
	if err := k8sDeploymentService.ForceRotateToken(c.Request.Context(), &pool, userID.(string)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "token rotated successfully, deployment will be restarted",
	})
}

// SyncDeploymentConfig syncs the Pod configuration with the latest K8s config
// @Summary Sync Pod config
// @Description Sync K8s Pods with latest configuration from database
// @Tags Agent Pool
// @Accept json
// @Produce json
// @Param pool_id path string true "Pool ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agent-pools/{pool_id}/sync-deployment [post]
func (h *AgentPoolHandler) SyncDeploymentConfig(c *gin.Context) {
	poolID := c.Param("pool_id")
	if poolID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "pool_id is required",
		})
		return
	}

	// Get pool
	var pool models.AgentPool
	if err := h.db.Where("pool_id = ?", poolID).First(&pool).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "agent pool not found",
		})
		return
	}

	// Verify it's a K8s pool
	if pool.PoolType != models.AgentPoolTypeK8s {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "can only sync Pods for K8s agent pools",
		})
		return
	}

	// Call K8s deployment service to sync Pods
	k8sDeploymentService, err := services.NewK8sDeploymentService(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "K8s deployment service not available",
		})
		return
	}

	// Sync Pods with latest configuration
	if err := k8sDeploymentService.EnsurePodsForPool(c.Request.Context(), &pool); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Pod configuration synced successfully",
	})
}

// ActivateOneTimeUnfreeze activates a one-time unfreeze for emergency bypass of freeze schedules
// @Summary Activate one-time unfreeze
// @Description Temporarily bypass freeze schedules for emergency situations (single-use, expires after current day)
// @Tags Agent Pool
// @Accept json
// @Produce json
// @Param pool_id path string true "Pool ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agent-pools/{pool_id}/one-time-unfreeze [post]
func (h *AgentPoolHandler) ActivateOneTimeUnfreeze(c *gin.Context) {
	poolID := c.Param("pool_id")
	if poolID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "pool_id is required",
		})
		return
	}

	// Get pool
	var pool models.AgentPool
	if err := h.db.Where("pool_id = ?", poolID).First(&pool).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "agent pool not found",
		})
		return
	}

	// Verify it's a K8s pool
	if pool.PoolType != models.AgentPoolTypeK8s {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "one-time unfreeze is only available for K8s agent pools",
		})
		return
	}

	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		userID = "system"
	}

	// Set unfreeze until end of current day (23:59:59)
	now := time.Now()
	endOfDay := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location())
	unfreezeBy := userID.(string)

	// Update pool with one-time unfreeze
	updates := map[string]interface{}{
		"one_time_unfreeze_until": endOfDay,
		"one_time_unfreeze_by":    unfreezeBy,
		"one_time_unfreeze_at":    now,
		"updated_at":              now,
		"updated_by":              unfreezeBy,
	}

	if err := h.db.Model(&pool).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to activate one-time unfreeze",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":            "one-time unfreeze activated successfully",
		"unfreeze_until":     endOfDay.Format(time.RFC3339),
		"unfreeze_by":        unfreezeBy,
		"unfreeze_activated": now.Format(time.RFC3339),
	})
}
