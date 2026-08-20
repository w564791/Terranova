package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"iac-platform/internal/config"
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
		jwtSecret = config.GetJWTSecret()
	}
	return &TeamTokenService{
		db:        db,
		jwtSecret: jwtSecret,
	}
}

// TeamTokenClaims JWT Claims for team tokens
type TeamTokenClaims struct {
	TeamID    string `json:"team_id"`
	TeamName  string `json:"team_name"`
	TokenID   string `json:"token_id"` // 改为字符串类型
	TokenType string `json:"type"`
	jwt.RegisteredClaims
}

// GenerateToken 生成团队Token（事务内配额/同名校验，降低并发突破风险 B-2）
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

	// 规格：Team Token 默认/最长 24h，禁止永不过期（32 号整改 D4）
	now := time.Now()
	days := expiresInDays
	if days <= 0 || days > 1 {
		days = 1
	}
	expiresAt := now.Add(time.Duration(days) * 24 * time.Hour)
	expiresAtPtr := &expiresAt

	tokenID, err := generateTeamTokenID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token ID: %w", err)
	}
	tokenIDHash := sha256.Sum256([]byte(tokenID))
	tokenIDHashStr := base64.StdEncoding.EncodeToString(tokenIDHash[:])
	createdBy := userID

	var tokenString string
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 串行化同 team 的 Generate（PostgreSQL 行锁）；不支持 FOR UPDATE 的驱动忽略错误。
		// 活跃上限 2：事务内 Count+Create 为 best-effort；无 DB 约束时极端并发仍可能 >2。
		_ = tx.Exec(`SELECT team_id FROM teams WHERE team_id = ? FOR UPDATE`, teamID)

		// 清理已过期但仍 is_active 的 token
		_ = tx.Model(&models.TeamToken{}).
			Where("team_id = ? AND is_active = ? AND expires_at IS NOT NULL AND expires_at < ?", teamID, true, now).
			Updates(map[string]interface{}{"is_active": false, "revoked_at": now, "revoked_by": "system:expired"})

		var activeCount int64
		if err := tx.Model(&models.TeamToken{}).
			Where("team_id = ? AND is_active = ?", teamID, true).
			Count(&activeCount).Error; err != nil {
			return err
		}
		if activeCount >= 2 {
			return errors.New("maximum number of active tokens (2) reached for this team")
		}

		var nameCount int64
		if err := tx.Model(&models.TeamToken{}).
			Where("team_id = ? AND token_name = ? AND is_active = ?", teamID, tokenName, true).
			Count(&nameCount).Error; err != nil {
			return err
		}
		if nameCount > 0 {
			return errors.New("active token with this name already exists")
		}

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
		if err := tx.Create(tokenRecord).Error; err != nil {
			// 并发下唯一约束冲突 → 友好错误
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				return errors.New("active token with this name already exists or quota exceeded")
			}
			return err
		}

		claims := TeamTokenClaims{
			TeamID:    teamID,
			TeamName:  team.Name,
			TokenID:   tokenID,
			TokenType: "team_token",
			RegisteredClaims: jwt.RegisteredClaims{
				IssuedAt:  jwt.NewNumericDate(now),
				NotBefore: jwt.NewNumericDate(now),
				ExpiresAt: jwt.NewNumericDate(*expiresAtPtr),
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		signed, err := token.SignedString([]byte(s.jwtSecret))
		if err != nil {
			return fmt.Errorf("failed to sign token: %w", err)
		}
		tokenString = signed
		hash := sha256.Sum256([]byte(signed))
		tokenHash := base64.StdEncoding.EncodeToString(hash[:])
		return tx.Model(tokenRecord).Update("token_hash", tokenHash).Error
	})
	if err != nil {
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

// RevokeToken 吊销 token（兼容旧调用：数字 ID 已废弃）
// 请使用 RevokeTokenByName。
func (s *TeamTokenService) RevokeToken(ctx context.Context, teamID string, tokenID uint, userID string) error {
	return errors.New("revoke by numeric id is no longer supported; use token_name")
}

// RevokeTokenByName 按 token_name 吊销当前活跃 token（仅命中 is_active=true，避免旧吊销记录挡新 token）
func (s *TeamTokenService) RevokeTokenByName(ctx context.Context, teamID string, tokenName string, userID string) error {
	if tokenName == "" {
		return errors.New("token_name is required")
	}
	var token models.TeamToken
	if err := s.db.WithContext(ctx).
		Where("team_id = ? AND token_name = ? AND is_active = ?", teamID, tokenName, true).
		First(&token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("active token not found")
		}
		return err
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

	// 与 JWT 中间件一致：用 token_id 的 hash 作为主键查找
	tokenIDHash := sha256.Sum256([]byte(claims.TokenID))
	tokenIDHashStr := base64.StdEncoding.EncodeToString(tokenIDHash[:])

	// 可选：校验 JWT 明文哈希（Create 时写入 token_hash）
	fullHash := sha256.Sum256([]byte(tokenString))
	tokenHash := base64.StdEncoding.EncodeToString(fullHash[:])

	var dbToken models.TeamToken
	if err := s.db.WithContext(ctx).
		Where("token_id_hash = ?", tokenIDHashStr).
		First(&dbToken).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("token not found in database")
		}
		return nil, err
	}

	// 若库内已有 token_hash，必须匹配（防 JWT 伪造 token_id）
	if dbToken.TokenHash != "" && dbToken.TokenHash != tokenHash {
		return nil, errors.New("token hash mismatch")
	}

	// 检查token是否有效
	if !dbToken.IsActive {
		return nil, errors.New("token has been revoked")
	}

	// 禁止永不过期 + 过期检查
	if dbToken.ExpiresAt == nil {
		return nil, errors.New("token has no expiry")
	}
	if dbToken.ExpiresAt.Before(time.Now()) {
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
