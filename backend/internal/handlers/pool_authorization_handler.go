package handlers

import (
	"net/http"
	"time"

	"iac-platform/internal/application/service"
	"iac-platform/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PoolAuthorizationHandler handles pool authorization-related HTTP requests
type PoolAuthorizationHandler struct {
	db           *gorm.DB
	agentService *service.AgentService
}

// NewPoolAuthorizationHandler creates a new pool authorization handler
func NewPoolAuthorizationHandler(db *gorm.DB) *PoolAuthorizationHandler {
	return &PoolAuthorizationHandler{
		db:           db,
		agentService: service.NewAgentService(db),
	}
}

// ===== Pool Side APIs =====

// AllowWorkspaces allows a pool to access multiple workspaces
// @Summary Pool allows workspaces
// @Description Pool grants access to multiple workspaces (batch operation)
// @Tags Pool Authorization
// @Accept json
// @Produce json
// @Param pool_id path string true "Pool ID"
// @Param request body models.PoolAllowWorkspacesRequest true "Workspace IDs to allow"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agent-pools/{pool_id}/allow-workspaces [post]
func (h *PoolAuthorizationHandler) AllowWorkspaces(c *gin.Context) {
	poolID := c.Param("pool_id")
	if poolID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "pool_id is required",
		})
		return
	}

	// Verify pool exists
	var pool models.AgentPool
	if err := h.db.Where("pool_id = ?", poolID).First(&pool).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "pool not found",
		})
		return
	}

	// Parse request
	var req models.PoolAllowWorkspacesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
		})
		return
	}

	if len(req.WorkspaceIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "workspace_ids cannot be empty",
		})
		return
	}

	// Get user ID from context (set by auth middleware)
	userID, exists := c.Get("user_id")
	if !exists {
		userID = "system"
	}
	allowedBy := userID.(string)

	// Batch insert allowances
	now := time.Now()
	allowances := make([]models.PoolAllowedWorkspace, 0, len(req.WorkspaceIDs))
	for _, workspaceID := range req.WorkspaceIDs {
		allowances = append(allowances, models.PoolAllowedWorkspace{
			PoolID:      poolID,
			WorkspaceID: workspaceID,
			Status:      models.AllowanceStatusActive,
			AllowedAt:   now,
			AllowedBy:   &allowedBy,
		})
	}

	// Use transaction to ensure atomicity
	err := h.db.Transaction(func(tx *gorm.DB) error {
		// Use ON CONFLICT to handle duplicates
		for i := range allowances {
			if err := tx.Where("pool_id = ? AND workspace_id = ?", allowances[i].PoolID, allowances[i].WorkspaceID).
				Assign(models.PoolAllowedWorkspace{
					Status:    models.AllowanceStatusActive,
					AllowedAt: now,
					AllowedBy: &allowedBy,
				}).
				FirstOrCreate(&allowances[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to allow workspaces",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "workspaces allowed successfully",
		"count":   len(req.WorkspaceIDs),
	})
}

// GetAllowedWorkspaces retrieves workspaces allowed by a pool
// @Summary Get pool's allowed workspaces
// @Description Get list of workspaces that this pool has granted access to
// @Tags Pool Authorization
// @Accept json
// @Produce json
// @Param pool_id path string true "Pool ID"
// @Param status query string false "Filter by status (active/revoked)"
// @Success 200 {object} models.PoolAllowedWorkspacesResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agent-pools/{pool_id}/allowed-workspaces [get]
func (h *PoolAuthorizationHandler) GetAllowedWorkspaces(c *gin.Context) {
	poolID := c.Param("pool_id")
	if poolID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "pool_id is required",
		})
		return
	}

	// Verify pool exists
	var pool models.AgentPool
	if err := h.db.Where("pool_id = ?", poolID).First(&pool).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "pool not found",
		})
		return
	}

	// Get status filter
	status := c.Query("status")

	// Query allowances with workspace names and user names
	type AllowedWorkspaceWithNames struct {
		models.PoolAllowedWorkspace
		WorkspaceName string `json:"workspace_name"`
		AllowedByName string `json:"allowed_by_name"`
	}

	query := h.db.Table("pool_allowed_workspaces").
		Select("pool_allowed_workspaces.*, workspaces.name as workspace_name, users.username as allowed_by_name").
		Joins("LEFT JOIN workspaces ON pool_allowed_workspaces.workspace_id = workspaces.workspace_id").
		Joins("LEFT JOIN users ON pool_allowed_workspaces.allowed_by = users.user_id").
		Where("pool_allowed_workspaces.pool_id = ?", poolID)

	if status != "" {
		query = query.Where("pool_allowed_workspaces.status = ?", status)
	}

	var allowances []AllowedWorkspaceWithNames
	if err := query.Order("pool_allowed_workspaces.allowed_at DESC").Find(&allowances).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to retrieve allowed workspaces",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"pool_id":    poolID,
		"workspaces": allowances,
		"total":      len(allowances),
	})
}

// RevokeWorkspaceAccess revokes pool's access to a workspace
// @Summary Revoke workspace access
// @Description Pool revokes its access to a specific workspace
// @Tags Pool Authorization
// @Accept json
// @Produce json
// @Param pool_id path string true "Pool ID"
// @Param workspace_id path string true "Workspace ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agent-pools/{pool_id}/allowed-workspaces/{workspace_id} [delete]
func (h *PoolAuthorizationHandler) RevokeWorkspaceAccess(c *gin.Context) {
	poolID := c.Param("pool_id")
	workspaceID := c.Param("workspace_id")

	if poolID == "" || workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "pool_id and workspace_id are required",
		})
		return
	}

	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		userID = "system"
	}
	revokedBy := userID.(string)

	// Update status to revoked
	now := time.Now()
	result := h.db.Model(&models.PoolAllowedWorkspace{}).
		Where("pool_id = ? AND workspace_id = ?", poolID, workspaceID).
		Updates(map[string]interface{}{
			"status":     models.AllowanceStatusRevoked,
			"revoked_at": now,
			"revoked_by": revokedBy,
		})

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to revoke workspace access",
		})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "allowance not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "workspace access revoked successfully",
	})
}

// ===== Workspace Side APIs =====

// GetAvailablePools retrieves pools available to a workspace
// @Summary Get available pools for workspace
// @Description Get list of pools that have granted access to this workspace
// @Tags Workspace Pool Authorization
// @Accept json
// @Produce json
// @Param workspace_id path string true "Workspace ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/workspaces/{workspace_id}/available-pools [get]
func (h *PoolAuthorizationHandler) GetAvailablePools(c *gin.Context) {
	workspaceID := c.Param("workspace_id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "workspace_id is required",
		})
		return
	}

	// Verify workspace exists
	var workspace models.Workspace
	if err := h.db.Where("workspace_id = ?", workspaceID).First(&workspace).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "workspace not found",
		})
		return
	}

	// Query pools that have allowed this workspace
	type PoolWithAgentCount struct {
		models.AgentPool
		AllowedAt   time.Time `json:"allowed_at"`
		AgentCount  int       `json:"agent_count"`
		OnlineCount int       `json:"online_count"`
	}

	var pools []PoolWithAgentCount
	err := h.db.Table("agent_pools").
		Select("agent_pools.*, pool_allowed_workspaces.allowed_at, "+
			"COUNT(DISTINCT agents.agent_id) as agent_count, "+
			"COUNT(DISTINCT CASE WHEN agents.status = 'online' THEN agents.agent_id END) as online_count").
		Joins("INNER JOIN pool_allowed_workspaces ON agent_pools.pool_id = pool_allowed_workspaces.pool_id").
		Joins("LEFT JOIN agents ON agent_pools.pool_id = agents.pool_id").
		Where("pool_allowed_workspaces.workspace_id = ? AND pool_allowed_workspaces.status = ?",
			workspaceID, models.AllowanceStatusActive).
		Group("agent_pools.pool_id, pool_allowed_workspaces.allowed_at").
		Order("pool_allowed_workspaces.allowed_at DESC").
		Find(&pools).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to retrieve available pools",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"workspace_id": workspaceID,
		"pools":        pools,
		"total":        len(pools),
	})
}

// SetCurrentPool sets the current pool for a workspace
// @Summary Set current pool
// @Description Set a specific pool as the current pool for this workspace
// @Tags Workspace Pool Authorization
// @Accept json
// @Produce json
// @Param workspace_id path string true "Workspace ID"
// @Param request body map[string]string true "Pool ID to set as current"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/workspaces/{workspace_id}/set-current-pool [post]
func (h *PoolAuthorizationHandler) SetCurrentPool(c *gin.Context) {
	workspaceID := c.Param("workspace_id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "workspace_id is required",
		})
		return
	}

	// Parse request
	var req struct {
		PoolID string `json:"pool_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
		})
		return
	}

	// Verify the pool has allowed this workspace
	var allowance models.PoolAllowedWorkspace
	err := h.db.Where("pool_id = ? AND workspace_id = ? AND status = ?",
		req.PoolID, workspaceID, models.AllowanceStatusActive).First(&allowance).Error
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "pool has not granted access to this workspace",
		})
		return
	}

	// Update workspace's current_pool_id
	result := h.db.Model(&models.Workspace{}).
		Where("workspace_id = ?", workspaceID).
		Update("current_pool_id", req.PoolID)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to set current pool",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      "current pool set successfully",
		"workspace_id": workspaceID,
		"pool_id":      req.PoolID,
	})
}

// GetCurrentPool retrieves the current pool for a workspace
// @Summary Get current pool
// @Description Get the current pool assigned to this workspace
// @Tags Workspace Pool Authorization
// @Accept json
// @Produce json
// @Param workspace_id path string true "Workspace ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/workspaces/{workspace_id}/current-pool [get]
func (h *PoolAuthorizationHandler) GetCurrentPool(c *gin.Context) {
	workspaceID := c.Param("workspace_id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "workspace_id is required",
		})
		return
	}

	// Get workspace with current pool
	var workspace models.Workspace
	err := h.db.Where("workspace_id = ?", workspaceID).First(&workspace).Error
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "workspace not found",
		})
		return
	}

	// Check if current_pool_id is set
	if workspace.CurrentPoolID == nil || *workspace.CurrentPoolID == "" {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "no current pool set for this workspace",
		})
		return
	}

	// Get pool details with agent counts
	type PoolWithAgents struct {
		models.AgentPool
		AgentCount  int `json:"agent_count"`
		OnlineCount int `json:"online_count"`
	}

	var pool PoolWithAgents
	err = h.db.Table("agent_pools").
		Select("agent_pools.*, "+
			"COUNT(DISTINCT agents.agent_id) as agent_count, "+
			"COUNT(DISTINCT CASE WHEN agents.status = 'online' THEN agents.agent_id END) as online_count").
		Joins("LEFT JOIN agents ON agent_pools.pool_id = agents.pool_id").
		Where("agent_pools.pool_id = ?", *workspace.CurrentPoolID).
		Group("agent_pools.pool_id").
		First(&pool).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to retrieve current pool details",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"workspace_id": workspaceID,
		"pool":         pool,
	})
}
