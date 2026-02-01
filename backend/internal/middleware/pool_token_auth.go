package middleware

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"iac-platform/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PoolTokenAuthMiddleware validates Pool Token for Agent authentication
// This is the basic version without workspace authorization check
func PoolTokenAuthMiddleware(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if this is a WebSocket upgrade request
		isWebSocket := c.GetHeader("Upgrade") == "websocket"

		// Get token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			if isWebSocket {
				// For WebSocket, don't write JSON response, just abort
				c.AbortWithStatus(http.StatusUnauthorized)
			} else {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":    401,
					"message": "Missing authorization header",
				})
				c.Abort()
			}
			return
		}

		// Extract token (format: "Bearer apt_pool-xxx_hash")
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == authHeader {
			if isWebSocket {
				c.AbortWithStatus(http.StatusUnauthorized)
			} else {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":    401,
					"message": "Invalid authorization format",
				})
				c.Abort()
			}
			return
		}

		// Validate token format (apt_pool-xxx_hash)
		if !strings.HasPrefix(token, "apt_") {
			if isWebSocket {
				c.AbortWithStatus(http.StatusUnauthorized)
			} else {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":    401,
					"message": "Invalid token format",
				})
				c.Abort()
			}
			return
		}

		// Calculate token hash (using Base64 to match PoolTokenService)
		hash := sha256.Sum256([]byte(token))
		tokenHash := base64.StdEncoding.EncodeToString(hash[:])

		// Debug logging - SECURITY: Only log token prefix (first 10 chars), never log full token or hash
		if len(token) > 10 {
			log.Printf("[PoolTokenAuth] Token prefix: %s...", token[:10])
		}

		// Query pool_tokens table
		var poolToken models.PoolToken
		err := db.Where("token_hash = ? AND is_active = true", tokenHash).
			First(&poolToken).Error

		if err == gorm.ErrRecordNotFound {
			log.Printf("[PoolTokenAuth] Token not found or inactive")
			if isWebSocket {
				c.AbortWithStatus(http.StatusUnauthorized)
			} else {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":    401,
					"message": "Invalid token",
				})
				c.Abort()
			}
			return
		}

		if err != nil {
			if isWebSocket {
				c.AbortWithStatus(http.StatusInternalServerError)
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{
					"code":    500,
					"message": "Failed to validate token",
				})
				c.Abort()
			}
			return
		}

		// Check if token is expired
		if poolToken.ExpiresAt != nil && poolToken.ExpiresAt.Before(db.NowFunc()) {
			if isWebSocket {
				c.AbortWithStatus(http.StatusUnauthorized)
			} else {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":    401,
					"message": "Token expired",
				})
				c.Abort()
			}
			return
		}

		// Update last_used_at
		db.Model(&poolToken).Update("last_used_at", db.NowFunc())

		// Store pool info in context
		c.Set("pool_id", poolToken.PoolID)
		c.Set("pool_token", poolToken)

		c.Next()
	}
}

// PoolTokenAuthWithTaskCheck validates Pool Token and checks if the pool has access to the task's workspace
// This middleware should be used for Agent API endpoints that access task-specific resources
func PoolTokenAuthWithTaskCheck(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// First, perform standard Pool Token authentication
		if !authenticatePoolToken(c, db) {
			return
		}

		// Get pool_id from context
		poolID, exists := c.Get("pool_id")
		if !exists {
			respondWithError(c, http.StatusInternalServerError, "Pool ID not found in context")
			return
		}

		// Extract task_id from request
		taskIDStr := c.Param("task_id")
		if taskIDStr == "" {
			respondWithError(c, http.StatusBadRequest, "Task ID is required")
			return
		}

		taskID, err := strconv.ParseUint(taskIDStr, 10, 32)
		if err != nil {
			respondWithError(c, http.StatusBadRequest, "Invalid task ID format")
			return
		}

		// Get workspace_id for this task
		var task struct {
			WorkspaceID string `gorm:"column:workspace_id"`
		}
		err = db.Table("workspace_tasks").
			Select("workspace_id").
			Where("id = ?", taskID).
			First(&task).Error

		if err == gorm.ErrRecordNotFound {
			respondWithError(c, http.StatusNotFound, "Task not found")
			return
		}

		if err != nil {
			log.Printf("[PoolTokenAuth] Failed to query task: %v", err)
			respondWithError(c, http.StatusInternalServerError, "Failed to query task")
			return
		}

		// Check if pool has access to this workspace
		if !checkPoolWorkspaceAccess(db, poolID.(string), task.WorkspaceID) {
			log.Printf("[PoolTokenAuth] Pool %s denied access to task %d (workspace %s)", poolID, taskID, task.WorkspaceID)
			respondWithError(c, http.StatusForbidden, "Pool does not have access to this task's workspace")
			return
		}

		// Store task and workspace info in context
		c.Set("authorized_task_id", uint(taskID))
		c.Set("authorized_workspace_id", task.WorkspaceID)
		c.Next()
	}
}

// PoolTokenAuthWithWorkspaceCheck validates Pool Token and checks if the pool has access to the workspace
// This middleware should be used for Agent API endpoints that access workspace-specific resources
func PoolTokenAuthWithWorkspaceCheck(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// First, perform standard Pool Token authentication
		if !authenticatePoolToken(c, db) {
			return
		}

		// Get pool_id from context
		poolID, exists := c.Get("pool_id")
		if !exists {
			respondWithError(c, http.StatusInternalServerError, "Pool ID not found in context")
			return
		}

		// Extract workspace_id from request
		workspaceID := c.Param("workspace_id")
		if workspaceID == "" {
			workspaceID = c.Query("workspace_id")
		}

		if workspaceID == "" {
			respondWithError(c, http.StatusBadRequest, "Workspace ID is required")
			return
		}

		// Check if pool has access to this workspace
		if !checkPoolWorkspaceAccess(db, poolID.(string), workspaceID) {
			log.Printf("[PoolTokenAuth] Pool %s denied access to workspace %s", poolID, workspaceID)
			respondWithError(c, http.StatusForbidden, fmt.Sprintf("Pool does not have access to workspace %s", workspaceID))
			return
		}

		// Store workspace_id in context
		c.Set("authorized_workspace_id", workspaceID)
		c.Next()
	}
}

// Helper functions

// authenticatePoolToken performs the standard Pool Token authentication
// Returns true if authentication succeeds, false otherwise
func authenticatePoolToken(c *gin.Context, db *gorm.DB) bool {
	// Get token from Authorization header
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		respondWithError(c, http.StatusUnauthorized, "Missing authorization header")
		return false
	}

	// Extract token (format: "Bearer apt_pool-xxx_hash")
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == authHeader {
		respondWithError(c, http.StatusUnauthorized, "Invalid authorization format")
		return false
	}

	// Validate token format (apt_pool-xxx_hash)
	if !strings.HasPrefix(token, "apt_") {
		respondWithError(c, http.StatusUnauthorized, "Invalid token format")
		return false
	}

	// Calculate token hash
	hash := sha256.Sum256([]byte(token))
	tokenHash := base64.StdEncoding.EncodeToString(hash[:])

	// Query pool_tokens table
	var poolToken models.PoolToken
	err := db.Where("token_hash = ? AND is_active = true", tokenHash).
		First(&poolToken).Error

	if err == gorm.ErrRecordNotFound {
		log.Printf("[PoolTokenAuth] Token not found or inactive")
		respondWithError(c, http.StatusUnauthorized, "Invalid token")
		return false
	}

	if err != nil {
		log.Printf("[PoolTokenAuth] Database error: %v", err)
		respondWithError(c, http.StatusInternalServerError, "Failed to validate token")
		return false
	}

	// Check if token is expired
	if poolToken.ExpiresAt != nil && poolToken.ExpiresAt.Before(db.NowFunc()) {
		respondWithError(c, http.StatusUnauthorized, "Token expired")
		return false
	}

	// Update last_used_at
	db.Model(&poolToken).Update("last_used_at", db.NowFunc())

	// Store pool info in context
	c.Set("pool_id", poolToken.PoolID)
	c.Set("pool_token", poolToken)

	return true
}

// checkPoolWorkspaceAccess checks if a pool has access to a workspace
func checkPoolWorkspaceAccess(db *gorm.DB, poolID, workspaceID string) bool {
	var count int64
	err := db.Table("pool_allowed_workspaces").
		Where("pool_id = ? AND workspace_id = ?", poolID, workspaceID).
		Count(&count).Error

	if err != nil {
		log.Printf("[PoolTokenAuth] Failed to check workspace access: %v", err)
		return false
	}

	return count > 0
}

// respondWithError sends an error response, handling both regular and WebSocket requests
func respondWithError(c *gin.Context, statusCode int, message string) {
	isWebSocket := c.GetHeader("Upgrade") == "websocket"

	if isWebSocket {
		c.AbortWithStatus(statusCode)
	} else {
		c.JSON(statusCode, gin.H{
			"code":    statusCode,
			"message": message,
		})
		c.Abort()
	}
}
