package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"iac-platform/internal/crypto"
	"iac-platform/internal/infrastructure"
	"iac-platform/internal/models"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// SecretHandler 密文处理器
type SecretHandler struct {
	db                    *gorm.DB
	hcpCredentialsService *services.HCPCredentialsService
}

// NewSecretHandler 创建密文处理器
func NewSecretHandler(db *gorm.DB) *SecretHandler {
	return &SecretHandler{
		db:                    db,
		hcpCredentialsService: services.NewHCPCredentialsService(db),
	}
}

// CreateSecret 创建密文
// @Summary 创建密文
// @Description 为指定资源创建加密密文数据
// @Tags Secrets
// @Accept json
// @Produce json
// @Param resourceType path string true "资源类型" Enums(agent_pool, workspace, module, system)
// @Param resourceId path string true "资源ID"
// @Param request body models.CreateSecretRequest true "创建密文请求"
// @Success 201 {object} models.CreateSecretResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/{resourceType}/{resourceId}/secrets [post]
func (h *SecretHandler) CreateSecret(c *gin.Context) {
	resourceType := c.Param("resourceType")
	resourceId := c.Param("resourceId")

	// 验证资源类型
	if !isValidResourceType(resourceType) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resource type"})
		return
	}

	var req models.CreateSecretRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取当前用户ID
	userID, _ := c.Get("user_id")
	userIDStr, _ := userID.(string)

	// 生成Secret ID
	secretID, err := infrastructure.GenerateSecretID()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate secret ID"})
		return
	}

	// 加密value
	encryptedValue, err := crypto.EncryptValue(req.Value)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encrypt value"})
		return
	}

	// 构建metadata
	metadata := models.SecretMetadata{
		Key:         req.Key,
		Description: req.Description,
		Tags:        req.Tags,
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to marshal metadata"})
		return
	}

	// 设置secret_type，默认为hcp
	secretType := req.SecretType
	if secretType == "" {
		secretType = models.SecretTypeHCP
	}

	// 创建Secret记录
	secret := models.Secret{
		SecretID:     secretID,
		SecretType:   secretType,
		ValueHash:    encryptedValue,
		ResourceType: models.ResourceType(resourceType),
		ResourceID:   &resourceId,
		CreatedBy:    &userIDStr,
		UpdatedBy:    &userIDStr,
		ExpiresAt:    req.ExpiresAt,
		IsActive:     true,
		Metadata:     datatypes.JSON(metadataJSON),
	}

	if err := h.db.Create(&secret).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to create secret: %v", err)})
		return
	}

	// If this is an agent_pool HCP secret, trigger credentials file refresh
	if resourceType == string(models.ResourceTypeAgentPool) && secretType == models.SecretTypeHCP {
		go func() {
			// 1. Refresh credentials file (for local mode)
			if err := h.hcpCredentialsService.RefreshCredentialsFile(resourceId); err != nil {
				log.Printf("[HCP Credentials] Failed to refresh credentials after secret creation: %v", err)
			}

			// 2. Send C&C notification to agents (for remote/k8s mode)
			ccNotifier := services.GetCCNotifier()
			if err := ccNotifier.NotifyCredentialsRefresh(resourceId); err != nil {
				log.Printf("[HCP Credentials] Failed to notify agents: %v", err)
			}
		}()
	}

	// 返回响应（不包含value，更安全）
	c.JSON(http.StatusCreated, secret.ToResponse())
}

// ListSecrets 列出密文
// @Summary 列出密文
// @Description 获取指定资源的所有密文列表（不包含value）
// @Tags Secrets
// @Accept json
// @Produce json
// @Param resourceType path string true "资源类型" Enums(agent_pool, workspace, module, system)
// @Param resourceId path string true "资源ID"
// @Param is_active query boolean false "是否仅显示激活的密文"
// @Success 200 {object} models.SecretListResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/{resourceType}/{resourceId}/secrets [get]
func (h *SecretHandler) ListSecrets(c *gin.Context) {
	resourceType := c.Param("resourceType")
	resourceId := c.Param("resourceId")

	// 验证资源类型
	if !isValidResourceType(resourceType) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resource type"})
		return
	}

	query := h.db.Where("resource_type = ? AND resource_id = ?", resourceType, resourceId)

	// 可选过滤：仅显示激活的
	if isActive := c.Query("is_active"); isActive == "true" {
		query = query.Where("is_active = ?", true)
	}

	var secrets []models.Secret
	if err := query.Order("created_at DESC").Find(&secrets).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list secrets"})
		return
	}

	// 转换为响应格式
	responses := make([]models.SecretResponse, len(secrets))
	for i, secret := range secrets {
		responses[i] = *secret.ToResponse()
	}

	c.JSON(http.StatusOK, models.SecretListResponse{
		Secrets: responses,
		Total:   len(responses),
	})
}

// GetSecret 获取密文详情
// @Summary 获取密文详情
// @Description 获取指定密文的详细信息（不包含value）
// @Tags Secrets
// @Accept json
// @Produce json
// @Param resourceType path string true "资源类型" Enums(agent_pool, workspace, module, system)
// @Param resourceId path string true "资源ID"
// @Param secretId path string true "密文ID"
// @Success 200 {object} models.SecretResponse
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/{resourceType}/{resourceId}/secrets/{secretId} [get]
func (h *SecretHandler) GetSecret(c *gin.Context) {
	resourceType := c.Param("resourceType")
	resourceId := c.Param("resourceId")
	secretId := c.Param("secretId")

	var secret models.Secret
	if err := h.db.Where("secret_id = ? AND resource_type = ? AND resource_id = ?",
		secretId, resourceType, resourceId).First(&secret).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Secret not found"})
		return
	}

	// 更新last_used_at
	now := time.Now()
	h.db.Model(&secret).Update("last_used_at", now)

	c.JSON(http.StatusOK, secret.ToResponse())
}

// UpdateSecret 更新密文
// @Summary 更新密文
// @Description 更新密文的metadata（不允许更新value）
// @Tags Secrets
// @Accept json
// @Produce json
// @Param resourceType path string true "资源类型" Enums(agent_pool, workspace, module, system)
// @Param resourceId path string true "资源ID"
// @Param secretId path string true "密文ID"
// @Param request body models.UpdateSecretRequest true "更新密文请求"
// @Success 200 {object} models.SecretResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/{resourceType}/{resourceId}/secrets/{secretId} [put]
func (h *SecretHandler) UpdateSecret(c *gin.Context) {
	resourceType := c.Param("resourceType")
	resourceId := c.Param("resourceId")
	secretId := c.Param("secretId")

	var req models.UpdateSecretRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取当前用户ID
	userID, _ := c.Get("user_id")
	userIDStr, _ := userID.(string)

	var secret models.Secret
	if err := h.db.Where("secret_id = ? AND resource_type = ? AND resource_id = ?",
		secretId, resourceType, resourceId).First(&secret).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Secret not found"})
		return
	}

	// 解析现有metadata
	var metadata models.SecretMetadata
	if secret.Metadata != nil {
		_ = json.Unmarshal(secret.Metadata, &metadata)
	}

	// 更新metadata
	if req.Description != nil {
		metadata.Description = *req.Description
	}
	if req.Tags != nil {
		metadata.Tags = req.Tags
	}

	// 序列化metadata
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to marshal metadata"})
		return
	}

	// 更新记录
	updates := map[string]interface{}{
		"metadata":   datatypes.JSON(metadataJSON),
		"updated_by": userIDStr,
		"updated_at": time.Now(),
	}

	// 如果提供了新的value，则加密并更新
	if req.Value != nil && *req.Value != "" {
		encryptedValue, err := crypto.EncryptValue(*req.Value)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encrypt value"})
			return
		}
		updates["value_hash"] = encryptedValue
	}

	if err := h.db.Model(&secret).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update secret"})
		return
	}

	// 重新加载
	h.db.Where("secret_id = ?", secretId).First(&secret)

	// If this is an agent_pool HCP secret, trigger credentials file refresh
	if resourceType == string(models.ResourceTypeAgentPool) && secret.SecretType == models.SecretTypeHCP {
		go func() {
			// 1. Refresh credentials file (for local mode)
			if err := h.hcpCredentialsService.RefreshCredentialsFile(resourceId); err != nil {
				log.Printf("[HCP Credentials] Failed to refresh credentials after secret update: %v", err)
			}

			// 2. Send C&C notification to agents (for remote/k8s mode)
			ccNotifier := services.GetCCNotifier()
			if err := ccNotifier.NotifyCredentialsRefresh(resourceId); err != nil {
				log.Printf("[HCP Credentials] Failed to notify agents: %v", err)
			}
		}()
	}

	c.JSON(http.StatusOK, secret.ToResponse())
}

// DeleteSecret 删除密文
// @Summary 删除密文
// @Description 删除指定的密文
// @Tags Secrets
// @Accept json
// @Produce json
// @Param resourceType path string true "资源类型" Enums(agent_pool, workspace, module, system)
// @Param resourceId path string true "资源ID"
// @Param secretId path string true "密文ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/{resourceType}/{resourceId}/secrets/{secretId} [delete]
func (h *SecretHandler) DeleteSecret(c *gin.Context) {
	resourceType := c.Param("resourceType")
	resourceId := c.Param("resourceId")
	secretId := c.Param("secretId")

	result := h.db.Where("secret_id = ? AND resource_type = ? AND resource_id = ?",
		secretId, resourceType, resourceId).Delete(&models.Secret{})

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete secret"})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Secret not found"})
		return
	}

	// If this is an agent_pool, trigger credentials file refresh
	// (refresh will remove the file if no HCP secrets remain)
	if resourceType == string(models.ResourceTypeAgentPool) {
		go func() {
			// 1. Refresh credentials file (for local mode)
			if err := h.hcpCredentialsService.RefreshCredentialsFile(resourceId); err != nil {
				log.Printf("[HCP Credentials] Failed to refresh credentials after secret deletion: %v", err)
			}

			// 2. Send C&C notification to agents (for remote/k8s mode)
			ccNotifier := services.GetCCNotifier()
			if err := ccNotifier.NotifyCredentialsRefresh(resourceId); err != nil {
				log.Printf("[HCP Credentials] Failed to notify agents: %v", err)
			}
		}()
	}

	c.JSON(http.StatusOK, gin.H{"message": "Secret deleted successfully"})
}

// isValidResourceType 验证资源类型
func isValidResourceType(resourceType string) bool {
	validTypes := []string{
		string(models.ResourceTypeAgentPool),
		string(models.ResourceTypeWorkspace),
		string(models.ResourceTypeModule),
		string(models.ResourceTypeSystem),
		string(models.ResourceTypeTeam),
		string(models.ResourceTypeUser),
	}

	for _, t := range validTypes {
		if resourceType == t {
			return true
		}
	}
	return false
}
