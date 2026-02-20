package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"iac-platform/internal/infrastructure"
	"iac-platform/internal/models"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

// UserTokenService 用户Token服务
type UserTokenService struct {
	db        *gorm.DB
	jwtSecret string
}

// NewUserTokenService 创建用户Token服务实例
// 如果jwtSecret为空，则从配置中获取
func NewUserTokenService(db *gorm.DB, jwtSecret string) *UserTokenService {
	if jwtSecret == "" {
		jwtSecret = getJWTSecretFromEnv()
	}
	return &UserTokenService{
		db:        db,
		jwtSecret: jwtSecret,
	}
}

// getJWTSecretFromEnv 从环境变量获取JWT密钥
func getJWTSecretFromEnv() string {
	// 这里直接使用os.Getenv，避免循环依赖
	secret := ""
	// 尝试从环境变量读取
	if envSecret := ""; envSecret != "" {
		secret = envSecret
	}
	if secret == "" {
		secret = "your-jwt-secret-key"
	}
	return secret
}

// UserTokenClaims JWT Claims for user tokens
type UserTokenClaims struct {
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	TokenID   string `json:"token_id"`
	TokenType string `json:"type"`
	jwt.RegisteredClaims
}

// GenerateToken 生成用户Token
func (s *UserTokenService) GenerateToken(ctx context.Context, userID string, tokenName string, expiresInDays int) (*models.UserTokenCreateResponse, error) {
	// 检查用户是否存在
	var user struct {
		UserID   string `gorm:"column:user_id"`
		Username string
	}
	if err := s.db.WithContext(ctx).Table("users").Where("user_id = ?", userID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("user not found")
		}
		return nil, err
	}

	// 检查有效token数量限制（最多2个）
	var activeCount int64
	if err := s.db.WithContext(ctx).Model(&models.UserToken{}).
		Where("user_id = ? AND is_active = ?", userID, true).
		Count(&activeCount).Error; err != nil {
		return nil, err
	}
	if activeCount >= 2 {
		return nil, errors.New("maximum number of active tokens (2) reached for this user")
	}

	// 生成Token ID
	tokenID, err := infrastructure.GenerateTokenID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token ID: %w", err)
	}

	// 创建token记录（先不设置token_hash，需要先生成JWT）
	now := time.Now()
	var expiresAtPtr *time.Time
	if expiresInDays > 0 {
		expiresAt := now.Add(time.Duration(expiresInDays) * 24 * time.Hour)
		expiresAtPtr = &expiresAt
	}
	// expiresInDays = 0 表示永不过期，expiresAtPtr = nil

	// 计算token_id的hash
	tokenIDHash := sha256.Sum256([]byte(tokenID))
	tokenIDHashStr := base64.StdEncoding.EncodeToString(tokenIDHash[:])

	tokenRecord := &models.UserToken{
		TokenID:     tokenID,        // 明文token_id（仅用于生成JWT，不保存到数据库）
		TokenIDHash: tokenIDHashStr, // 保存hash值
		UserID:      userID,
		TokenName:   tokenName,
		IsActive:    true,
		CreatedAt:   now,
		ExpiresAt:   expiresAtPtr,
	}

	// 生成JWT token
	registeredClaims := jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now),
	}

	// 如果有过期时间，设置到claims中
	if expiresAtPtr != nil {
		registeredClaims.ExpiresAt = jwt.NewNumericDate(*expiresAtPtr)
	}

	claims := UserTokenClaims{
		UserID:           userID,
		Username:         user.Username,
		TokenID:          tokenID,
		TokenType:        "user_token",
		RegisteredClaims: registeredClaims,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.jwtSecret))
	if err != nil {
		return nil, fmt.Errorf("failed to sign token: %w", err)
	}

	// 计算token哈希值
	hash := sha256.Sum256([]byte(tokenString))
	tokenHash := base64.StdEncoding.EncodeToString(hash[:])
	tokenRecord.TokenHash = tokenHash

	// 保存到数据库
	if err := s.db.WithContext(ctx).Create(tokenRecord).Error; err != nil {
		return nil, err
	}

	return &models.UserTokenCreateResponse{
		TokenID:   tokenID,
		UserID:    userID,
		TokenName: tokenName,
		Token:     tokenString,
		CreatedAt: now,
		ExpiresAt: expiresAtPtr,
	}, nil
}

// ListUserTokens 列出用户的所有token
func (s *UserTokenService) ListUserTokens(ctx context.Context, userID string) ([]models.UserTokenResponse, error) {
	var tokens []models.UserToken
	if err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&tokens).Error; err != nil {
		return nil, err
	}

	responses := make([]models.UserTokenResponse, len(tokens))
	for i, token := range tokens {
		responses[i] = models.UserTokenResponse{
			UserID:     token.UserID,
			TokenName:  token.TokenName,
			IsActive:   token.IsActive,
			CreatedAt:  token.CreatedAt,
			RevokedAt:  token.RevokedAt,
			LastUsedAt: token.LastUsedAt,
			ExpiresAt:  token.ExpiresAt,
		}
	}

	return responses, nil
}

// RevokeToken 吊销token（使用token_name作为标识）
func (s *UserTokenService) RevokeToken(ctx context.Context, userID string, tokenName string) error {
	var token models.UserToken
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND token_name = ?", userID, tokenName).
		First(&token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("token not found")
		}
		return err
	}

	if !token.IsActive {
		return errors.New("token is already revoked")
	}

	now := time.Now()
	updates := map[string]interface{}{
		"is_active":  false,
		"revoked_at": now,
	}

	return s.db.WithContext(ctx).Model(&token).Updates(updates).Error
}

// ValidateToken 验证token
func (s *UserTokenService) ValidateToken(ctx context.Context, tokenString string) (*UserTokenClaims, error) {
	// 解析JWT token
	token, err := jwt.ParseWithClaims(tokenString, &UserTokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		// 验证签名方法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.jwtSecret), nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(*UserTokenClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}

	// 验证token类型
	if claims.TokenType != "user_token" {
		return nil, errors.New("invalid token type")
	}

	// 计算token_id的hash用于查询
	tokenIDHash := sha256.Sum256([]byte(claims.TokenID))
	tokenIDHashStr := base64.StdEncoding.EncodeToString(tokenIDHash[:])

	// 计算token哈希值
	hash := sha256.Sum256([]byte(tokenString))
	tokenHash := base64.StdEncoding.EncodeToString(hash[:])

	// 从数据库验证token（使用token_id_hash作为主键）
	var dbToken models.UserToken
	if err := s.db.WithContext(ctx).
		Where("token_id_hash = ? AND token_hash = ?", tokenIDHashStr, tokenHash).
		First(&dbToken).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("token not found in database")
		}
		return nil, err
	}

	// 检查token是否有效
	if !dbToken.IsActive {
		return nil, errors.New("token has been revoked")
	}

	// 检查是否过期
	if dbToken.ExpiresAt != nil && dbToken.ExpiresAt.Before(time.Now()) {
		return nil, errors.New("token has expired")
	}

	// 更新最后使用时间
	now := time.Now()
	s.db.WithContext(ctx).Model(&dbToken).Update("last_used_at", now)

	return claims, nil
}

// GetTokenByID 根据ID获取token信息（使用token_id_hash）
func (s *UserTokenService) GetTokenByID(ctx context.Context, tokenID string) (*models.UserTokenResponse, error) {
	// 计算token_id的hash
	tokenIDHash := sha256.Sum256([]byte(tokenID))
	tokenIDHashStr := base64.StdEncoding.EncodeToString(tokenIDHash[:])

	var token models.UserToken
	if err := s.db.WithContext(ctx).Where("token_id_hash = ?", tokenIDHashStr).First(&token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("token not found")
		}
		return nil, err
	}

	return &models.UserTokenResponse{
		UserID:     token.UserID,
		TokenName:  token.TokenName,
		IsActive:   token.IsActive,
		CreatedAt:  token.CreatedAt,
		RevokedAt:  token.RevokedAt,
		LastUsedAt: token.LastUsedAt,
		ExpiresAt:  token.ExpiresAt,
	}, nil
}
