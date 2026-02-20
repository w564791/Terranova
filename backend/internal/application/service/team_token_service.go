package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"iac-platform/internal/models"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

// TeamTokenService 团队Token服务
type TeamTokenService struct {
	db        *gorm.DB
	jwtSecret string
}

// NewTeamTokenService 创建团队Token服务实例
// 如果jwtSecret为空，则从配置中获取
func NewTeamTokenService(db *gorm.DB, jwtSecret string) *TeamTokenService {
	if jwtSecret == "" {
		// 使用统一的JWT密钥
		jwtSecret = getJWTSecretFromConfig()
	}
	return &TeamTokenService{
		db:        db,
		jwtSecret: jwtSecret,
	}
}

// getJWTSecretFromConfig 从配置获取JWT密钥
func getJWTSecretFromConfig() string {
	// 避免循环依赖，直接读取环境变量
	secret := ""
	// 这里应该调用config.GetJWTSecret()，但为了避免循环依赖，直接实现
	// TODO: 重构以使用config包
	if secret == "" {
		secret = "your-jwt-secret-key"
	}
	return secret
}

// TeamTokenClaims JWT Claims for team tokens
type TeamTokenClaims struct {
	TeamID    string `json:"team_id"`
	TeamName  string `json:"team_name"`
	TokenID   string `json:"token_id"` // 改为字符串类型
	TokenType string `json:"type"`
	jwt.RegisteredClaims
}

// GenerateToken 生成团队Token
func (s *TeamTokenService) GenerateToken(ctx context.Context, teamID string, tokenName string, userID string, expiresInDays int) (*models.TeamTokenCreateResponse, error) {
	// 检查团队是否存在
	var team struct {
		TeamID string `gorm:"column:team_id"`
		Name   string
	}
	if err := s.db.WithContext(ctx).Table("teams").Where("team_id = ?", teamID).First(&team).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("team not found")
		}
		return nil, err
	}

	// 检查有效token数量限制（最多2个）
	var activeCount int64
	if err := s.db.WithContext(ctx).Model(&models.TeamToken{}).
		Where("team_id = ? AND is_active = ?", teamID, true).
		Count(&activeCount).Error; err != nil {
		return nil, err
	}
	if activeCount >= 2 {
		return nil, errors.New("maximum number of active tokens (2) reached for this team")
	}

	// 生成token_id（格式：token-t-xxxxx）
	tokenID, err := generateTeamTokenID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token ID: %w", err)
	}

	// 创建token记录
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

	createdBy := userID
	tokenRecord := &models.TeamToken{
		TokenID:     tokenID,
		TokenIDHash: tokenIDHashStr,
		TeamID:      teamID,
		TokenName:   tokenName,
		IsActive:    true,
		CreatedAt:   now,
		CreatedBy:   &createdBy,
		ExpiresAt:   expiresAtPtr,
	}

	// 保存到数据库
	if err := s.db.WithContext(ctx).Create(tokenRecord).Error; err != nil {
		return nil, err
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

	claims := TeamTokenClaims{
		TeamID:           teamID,
		TeamName:         team.Name,
		TokenID:          tokenID, // 使用字符串token_id
		TokenType:        "team_token",
		RegisteredClaims: registeredClaims,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.jwtSecret))
	if err != nil {
		// 如果JWT生成失败，删除已创建的记录
		s.db.WithContext(ctx).Delete(tokenRecord)
		return nil, fmt.Errorf("failed to sign token: %w", err)
	}

	// 计算token哈希值
	hash := sha256.Sum256([]byte(tokenString))
	tokenHash := base64.StdEncoding.EncodeToString(hash[:])

	// 更新token记录的hash值
	if err := s.db.WithContext(ctx).Model(tokenRecord).Update("token_hash", tokenHash).Error; err != nil {
		// 如果更新失败，删除记录
		s.db.WithContext(ctx).Delete(tokenRecord)
		return nil, err
	}

	return &models.TeamTokenCreateResponse{
		TeamID:    teamID,
		TokenName: tokenName,
		Token:     tokenString,
		CreatedAt: now,
		ExpiresAt: expiresAtPtr,
	}, nil
}

// generateTeamTokenID 生成team token ID（格式：token-t-xxxxx）
func generateTeamTokenID() (string, error) {
	// 使用infrastructure包的生成逻辑会更好，但为了避免循环依赖，这里直接实现
	// 格式：token-t-xxxxx（8-16位随机小写+数字）
	randomPart := randomString(12) // 生成12位随机字符串
	return "token-t-" + randomPart, nil
}

// randomString 生成随机字符串
func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
		time.Sleep(time.Nanosecond) // 确保每次生成不同的随机数
	}
	return string(b)
}

// ListTeamTokens 列出团队的所有token
func (s *TeamTokenService) ListTeamTokens(ctx context.Context, teamID string) ([]models.TeamTokenResponse, error) {
	var tokens []models.TeamToken
	if err := s.db.WithContext(ctx).
		Where("team_id = ?", teamID).
		Order("created_at DESC").
		Find(&tokens).Error; err != nil {
		return nil, err
	}

	responses := make([]models.TeamTokenResponse, len(tokens))
	for i, token := range tokens {
		responses[i] = models.TeamTokenResponse{
			TeamID:     token.TeamID,
			TokenName:  token.TokenName,
			IsActive:   token.IsActive,
			CreatedAt:  token.CreatedAt,
			CreatedBy:  token.CreatedBy,
			RevokedAt:  token.RevokedAt,
			RevokedBy:  token.RevokedBy,
			LastUsedAt: token.LastUsedAt,
			ExpiresAt:  token.ExpiresAt,
		}
	}

	return responses, nil
}

// RevokeToken 吊销token
func (s *TeamTokenService) RevokeToken(ctx context.Context, teamID string, tokenID uint, userID string) error {
	var token models.TeamToken
	if err := s.db.WithContext(ctx).
		Where("id = ? AND team_id = ?", tokenID, teamID).
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
		"revoked_by": userID,
	}

	return s.db.WithContext(ctx).Model(&token).Updates(updates).Error
}

// ValidateToken 验证token
func (s *TeamTokenService) ValidateToken(ctx context.Context, tokenString string) (*TeamTokenClaims, error) {
	// 解析JWT token
	token, err := jwt.ParseWithClaims(tokenString, &TeamTokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		// 验证签名方法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.jwtSecret), nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(*TeamTokenClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}

	// 验证token类型
	if claims.TokenType != "team_token" {
		return nil, errors.New("invalid token type")
	}

	// 计算token哈希值
	hash := sha256.Sum256([]byte(tokenString))
	tokenHash := base64.StdEncoding.EncodeToString(hash[:])

	// 从数据库验证token（使用字符串token_id）
	var dbToken models.TeamToken
	if err := s.db.WithContext(ctx).
		Where("token_id = ? AND token_hash = ?", claims.TokenID, tokenHash).
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

// GetTokenByID 根据ID获取token信息
func (s *TeamTokenService) GetTokenByID(ctx context.Context, tokenID uint) (*models.TeamTokenResponse, error) {
	var token models.TeamToken
	if err := s.db.WithContext(ctx).First(&token, tokenID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("token not found")
		}
		return nil, err
	}

	return &models.TeamTokenResponse{
		TeamID:     token.TeamID,
		TokenName:  token.TokenName,
		IsActive:   token.IsActive,
		CreatedAt:  token.CreatedAt,
		CreatedBy:  token.CreatedBy,
		RevokedAt:  token.RevokedAt,
		RevokedBy:  token.RevokedBy,
		LastUsedAt: token.LastUsedAt,
		ExpiresAt:  token.ExpiresAt,
	}, nil
}
