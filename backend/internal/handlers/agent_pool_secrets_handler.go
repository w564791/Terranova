package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"iac-platform/internal/crypto"
	"iac-platform/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AgentPoolSecretsHandler handles agent pool secrets retrieval for agents
type AgentPoolSecretsHandler struct {
	db *gorm.DB
}

// NewAgentPoolSecretsHandler creates a new agent pool secrets handler
func NewAgentPoolSecretsHandler(db *gorm.DB) *AgentPoolSecretsHandler {
	return &AgentPoolSecretsHandler{
		db: db,
	}
}

// GetPoolSecrets retrieves HCP secrets for the agent's pool
// This endpoint is called by agents to get credentials for generating credentials.tfrc.json
// @Summary Get pool HCP secrets
// @Description Get decrypted HCP secrets for the agent's pool (agent-only endpoint)
// @Tags Agent
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer {pool_token}"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agents/pool/secrets [get]
func (h *AgentPoolSecretsHandler) GetPoolSecrets(c *gin.Context) {
	// Get pool_id from context (set by PoolTokenAuthMiddleware)
	poolID, exists := c.Get("pool_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "pool_id not found in context",
		})
		return
	}

	poolIDStr := poolID.(string)
	log.Printf("[Agent Pool Secrets] Agent requesting HCP secrets for pool %s", poolIDStr)

	// Query HCP secrets for this agent pool
	var secrets []models.Secret
	err := h.db.Where("resource_type = ? AND resource_id = ? AND secret_type = ? AND is_active = ?",
		models.ResourceTypeAgentPool, poolIDStr, models.SecretTypeHCP, true).
		Find(&secrets).Error

	if err != nil {
		log.Printf("[Agent Pool Secrets] Failed to query secrets: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to query secrets",
		})
		return
	}

	// If no secrets, return empty credentials
	if len(secrets) == 0 {
		log.Printf("[Agent Pool Secrets] No HCP secrets found for pool %s", poolIDStr)
		c.JSON(http.StatusOK, gin.H{
			"credentials": map[string]interface{}{},
		})
		return
	}

	log.Printf("[Agent Pool Secrets] Found %d HCP secrets for pool %s", len(secrets), poolIDStr)

	// Build credentials map with decrypted values
	credentials := make(map[string]interface{})

	for _, secret := range secrets {
		// Parse metadata to get the key (hostname)
		var metadata models.SecretMetadata
		if err := json.Unmarshal(secret.Metadata, &metadata); err != nil {
			log.Printf("[Agent Pool Secrets] Warning: failed to parse metadata for secret %s: %v", secret.SecretID, err)
			continue
		}

		// Decrypt the value
		decryptedValue, err := crypto.DecryptValue(secret.ValueHash)
		if err != nil {
			log.Printf("[Agent Pool Secrets] Warning: failed to decrypt secret %s: %v", secret.SecretID, err)
			continue
		}

		// Add to credentials map
		credentials[metadata.Key] = map[string]interface{}{
			"token": decryptedValue,
		}

		log.Printf("[Agent Pool Secrets] Added credential for key: %s", metadata.Key)
	}

	// Return credentials in Terraform format
	c.JSON(http.StatusOK, gin.H{
		"credentials": credentials,
	})
}
