package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// PoolTokenService handles pool token operations
type PoolTokenService struct {
	db *gorm.DB
}

// NewPoolTokenService creates a new pool token service instance
func NewPoolTokenService(db *gorm.DB) *PoolTokenService {
	return &PoolTokenService{
		db: db,
	}
}

// GenerateStaticToken generates a static token for an agent pool
func (s *PoolTokenService) GenerateStaticToken(ctx context.Context, poolID string, tokenName string, createdBy string, expiresAt *time.Time) (*models.PoolTokenCreateResponse, error) {
	// Verify pool exists
	var pool models.AgentPool
	if err := s.db.WithContext(ctx).Where("pool_id = ?", poolID).First(&pool).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("agent pool not found")
		}
		return nil, err
	}

	// Allow static tokens for both static and k8s pools
	// K8s pools use static tokens for deployment authentication
	if pool.PoolType != models.AgentPoolTypeStatic && pool.PoolType != models.AgentPoolTypeK8s {
		return nil, errors.New("can only create static tokens for static or k8s agent pools")
	}

	// Generate random token (32 bytes = 64 hex characters)
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random token: %w", err)
	}
	tokenString := fmt.Sprintf("apt_%s_%s", poolID, hex.EncodeToString(tokenBytes))

	// Calculate token hash
	hash := sha256.Sum256([]byte(tokenString))
	tokenHash := base64.StdEncoding.EncodeToString(hash[:])

	// Create token record
	now := time.Now()
	token := &models.PoolToken{
		TokenHash:    tokenHash,
		TokenName:    tokenName,
		TokenType:    models.PoolTokenTypeStatic,
		PoolID:       poolID,
		IsActive:     true,
		CreatedAt:    now,
		CreatedBy:    &createdBy,
		ExpiresAt:    expiresAt,
		K8sNamespace: "terraform",
	}

	if err := s.db.WithContext(ctx).Create(token).Error; err != nil {
		return nil, fmt.Errorf("failed to create token: %w", err)
	}

	return &models.PoolTokenCreateResponse{
		Token:     tokenString,
		TokenName: tokenName,
		TokenType: models.PoolTokenTypeStatic,
		PoolID:    poolID,
		CreatedAt: now,
		CreatedBy: &createdBy,
		ExpiresAt: expiresAt,
	}, nil
}

// GenerateK8sTemporaryToken generates a temporary token for K8s job
func (s *PoolTokenService) GenerateK8sTemporaryToken(ctx context.Context, poolID string, jobName string, podName string, createdBy string, expiresAt time.Time) (*models.PoolTokenCreateResponse, error) {
	// Verify pool exists and is k8s type
	var pool models.AgentPool
	if err := s.db.WithContext(ctx).Where("pool_id = ?", poolID).First(&pool).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("agent pool not found")
		}
		return nil, err
	}

	if pool.PoolType != models.AgentPoolTypeK8s {
		return nil, errors.New("can only create K8s temporary tokens for K8s agent pools")
	}

	// Generate random token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random token: %w", err)
	}
	tokenString := fmt.Sprintf("apt_%s_%s", poolID, hex.EncodeToString(tokenBytes))

	// Calculate token hash
	hash := sha256.Sum256([]byte(tokenString))
	tokenHash := base64.StdEncoding.EncodeToString(hash[:])

	// Create token record
	now := time.Now()
	tokenName := fmt.Sprintf("k8s-job-%s", jobName)
	token := &models.PoolToken{
		TokenHash:    tokenHash,
		TokenName:    tokenName,
		TokenType:    models.PoolTokenTypeK8sTemporary,
		PoolID:       poolID,
		IsActive:     true,
		CreatedAt:    now,
		CreatedBy:    &createdBy,
		ExpiresAt:    &expiresAt,
		K8sJobName:   &jobName,
		K8sPodName:   &podName,
		K8sNamespace: "terraform",
	}

	if err := s.db.WithContext(ctx).Create(token).Error; err != nil {
		return nil, fmt.Errorf("failed to create token: %w", err)
	}

	return &models.PoolTokenCreateResponse{
		Token:     tokenString,
		TokenName: tokenName,
		TokenType: models.PoolTokenTypeK8sTemporary,
		PoolID:    poolID,
		CreatedAt: now,
		CreatedBy: &createdBy,
		ExpiresAt: &expiresAt,
	}, nil
}

// ListPoolTokens lists the most recent tokens for a pool (limited to 5)
func (s *PoolTokenService) ListPoolTokens(ctx context.Context, poolID string) ([]models.PoolTokenResponse, error) {
	var tokens []models.PoolToken
	if err := s.db.WithContext(ctx).
		Where("pool_id = ?", poolID).
		Order("created_at DESC").
		Limit(5).
		Find(&tokens).Error; err != nil {
		return nil, err
	}

	responses := make([]models.PoolTokenResponse, len(tokens))
	for i, token := range tokens {
		responses[i] = models.PoolTokenResponse{
			TokenName:    token.TokenName,
			TokenType:    token.TokenType,
			PoolID:       token.PoolID,
			IsActive:     token.IsActive,
			CreatedAt:    token.CreatedAt,
			CreatedBy:    token.CreatedBy,
			RevokedAt:    token.RevokedAt,
			RevokedBy:    token.RevokedBy,
			LastUsedAt:   token.LastUsedAt,
			ExpiresAt:    token.ExpiresAt,
			K8sJobName:   token.K8sJobName,
			K8sPodName:   token.K8sPodName,
			K8sNamespace: token.K8sNamespace,
		}
	}

	return responses, nil
}

// RevokeToken revokes a token by token name
func (s *PoolTokenService) RevokeToken(ctx context.Context, poolID string, tokenName string, revokedBy string) error {
	var token models.PoolToken
	if err := s.db.WithContext(ctx).
		Where("pool_id = ? AND token_name = ? AND is_active = ?", poolID, tokenName, true).
		Order("created_at DESC").
		First(&token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("token not found or already revoked")
		}
		return err
	}

	now := time.Now()
	updates := map[string]interface{}{
		"is_active":  false,
		"revoked_at": now,
		"revoked_by": revokedBy,
	}

	return s.db.WithContext(ctx).Model(&token).Updates(updates).Error
}

// ValidateToken validates a pool token
func (s *PoolTokenService) ValidateToken(ctx context.Context, tokenString string) (*models.PoolToken, error) {
	// Calculate token hash
	hash := sha256.Sum256([]byte(tokenString))
	tokenHash := base64.StdEncoding.EncodeToString(hash[:])

	// Find token in database
	var token models.PoolToken
	if err := s.db.WithContext(ctx).
		Where("token_hash = ?", tokenHash).
		First(&token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invalid token")
		}
		return nil, err
	}

	// Check if token is active
	if !token.IsActive {
		return nil, errors.New("token has been revoked")
	}

	// Check if token is expired
	if token.ExpiresAt != nil && token.ExpiresAt.Before(time.Now()) {
		return nil, errors.New("token has expired")
	}

	// Update last used time
	now := time.Now()
	s.db.WithContext(ctx).Model(&token).Update("last_used_at", now)

	return &token, nil
}

// CleanupExpiredTokens removes expired tokens
func (s *PoolTokenService) CleanupExpiredTokens(ctx context.Context) (int64, error) {
	now := time.Now()

	// Delete expired tokens
	result := s.db.WithContext(ctx).
		Where("expires_at IS NOT NULL AND expires_at < ?", now).
		Delete(&models.PoolToken{})

	if result.Error != nil {
		return 0, result.Error
	}

	return result.RowsAffected, nil
}

// UpdateK8sConfig updates the K8s configuration for a pool
func (s *PoolTokenService) UpdateK8sConfig(ctx context.Context, poolID string, config models.K8sJobTemplateConfig, updatedBy string) error {
	// Verify pool exists and is k8s type
	var pool models.AgentPool
	if err := s.db.WithContext(ctx).Where("pool_id = ?", poolID).First(&pool).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("agent pool not found")
		}
		return err
	}

	if pool.PoolType != models.AgentPoolTypeK8s {
		return errors.New("can only update K8s config for K8s agent pools")
	}

	// Serialize config to JSON
	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to serialize K8s config: %w", err)
	}

	configStr := string(configJSON)
	log.Printf("[PoolTokenService] Updating K8s config for pool %s, config length: %d bytes", poolID, len(configStr))
	log.Printf("[PoolTokenService] Config JSON: %s", configStr)

	updates := map[string]interface{}{
		"k8s_config": &configStr,
		"updated_at": time.Now(),
		"updated_by": &updatedBy,
	}

	result := s.db.WithContext(ctx).Model(&pool).Updates(updates)
	if result.Error != nil {
		log.Printf("[PoolTokenService] Failed to update K8s config: %v", result.Error)
		return result.Error
	}

	log.Printf("[PoolTokenService] Successfully updated K8s config for pool %s, rows affected: %d", poolID, result.RowsAffected)

	if result.RowsAffected == 0 {
		log.Printf("[PoolTokenService] Warning: No rows affected when updating K8s config for pool %s", poolID)
	}

	return nil
}

// GetK8sConfig retrieves the K8s configuration for a pool
func (s *PoolTokenService) GetK8sConfig(ctx context.Context, poolID string) (*models.K8sJobTemplateConfig, error) {
	var pool models.AgentPool
	if err := s.db.WithContext(ctx).Where("pool_id = ?", poolID).First(&pool).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("agent pool not found")
		}
		return nil, err
	}

	if pool.PoolType != models.AgentPoolTypeK8s {
		return nil, errors.New("pool is not a K8s agent pool")
	}

	if pool.K8sConfig == nil || *pool.K8sConfig == "" {
		// Return empty config instead of error for better UX
		return &models.K8sJobTemplateConfig{
			Image:           "",
			ImagePullPolicy: "IfNotPresent",
			MinReplicas:     1,
			MaxReplicas:     10,
		}, nil
	}

	var config models.K8sJobTemplateConfig
	if err := json.Unmarshal([]byte(*pool.K8sConfig), &config); err != nil {
		return nil, fmt.Errorf("failed to parse K8s config: %w", err)
	}

	return &config, nil
}
