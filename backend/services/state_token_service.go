package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

type StateTokenClaims struct {
	WorkspaceID string `json:"workspace_id"`
	TaskID      uint   `json:"task_id"`
	jwt.RegisteredClaims
}

type StateTokenService struct {
	db     *gorm.DB
	secret []byte
}

func NewStateTokenService(db *gorm.DB) *StateTokenService {
	secret := "state:" + os.Getenv("JWT_SECRET")
	return &StateTokenService{db: db, secret: []byte(secret)}
}

// GenerateToken creates a JWT for a task, stores SHA256(token) in workspace_tasks.state_token_hash.
// Token expires in 7 days; lifecycle is also controlled by task status.
func (s *StateTokenService) GenerateToken(workspaceID string, taskID uint) (string, error) {
	now := time.Now()
	claims := StateTokenClaims{
		WorkspaceID: workspaceID,
		TaskID:      taskID,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(7 * 24 * time.Hour)),
			Subject:   fmt.Sprintf("task:%d", taskID),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("failed to sign state token: %w", err)
	}

	hash := sha256Hash(tokenStr)
	result := s.db.Model(&struct{}{}).
		Table("workspace_tasks").
		Where("id = ?", taskID).
		Update("state_token_hash", hash)
	if result.Error != nil {
		return "", fmt.Errorf("failed to save state token hash: %w", result.Error)
	}
	log.Printf("[StateToken] GenerateToken: stored hash for task %d, rows_affected=%d, hash_prefix=%s...", taskID, result.RowsAffected, hash[:16])

	// Verify immediately
	var verifyHash *string
	s.db.Table("workspace_tasks").Where("id = ?", taskID).Select("state_token_hash").Scan(&verifyHash)
	if verifyHash != nil && *verifyHash == hash {
		log.Printf("[StateToken] GenerateToken: VERIFIED hash matches in DB for task %d", taskID)
	} else {
		log.Printf("[StateToken] GenerateToken: MISMATCH! DB hash=%v, expected=%s", verifyHash, hash[:16])
	}

	return tokenStr, nil
}

// ValidateToken verifies JWT signature + expiry, then checks task is still active in DB.
// Falls back to JWT-only validation if DB is temporarily unavailable.
func (s *StateTokenService) ValidateToken(tokenStr string) (string, uint, error) {
	claims := &StateTokenClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil || !token.Valid {
		log.Printf("[StateToken] JWT parse failed for task %d: %v", claims.TaskID, err)
		return "", 0, fmt.Errorf("invalid state token: %w", err)
	}

	log.Printf("[StateToken] JWT valid for task %d, workspace %s", claims.TaskID, claims.WorkspaceID)

	// DB check: verify hash exists and task is still active
	// On DB failure, fall back to JWT-only (already passed signature + expiry check)
	hash := sha256Hash(tokenStr)
	var count int64
	if err := s.db.Table("workspace_tasks").
		Where("id = ? AND state_token_hash = ? AND status IN ('running', 'pending', 'apply_pending')",
			claims.TaskID, hash).
		Count(&count).Error; err != nil {
		log.Printf("WARNING: DB check failed for state token (task %d), falling back to JWT-only: %v", claims.TaskID, err)
		return claims.WorkspaceID, claims.TaskID, nil
	}
	if count == 0 {
		// Debug: check what the actual status and hash are
		var debugStatus string
		var debugHash *string
		s.db.Table("workspace_tasks").Where("id = ?", claims.TaskID).Select("status").Scan(&debugStatus)
		s.db.Table("workspace_tasks").Where("id = ?", claims.TaskID).Select("state_token_hash").Scan(&debugHash)
		hashMatch := debugHash != nil && *debugHash == hash
		dbHashPrefix := "nil"
		if debugHash != nil {
			dbHashPrefix = (*debugHash)[:16] + "..."
		}
		log.Printf("[StateToken] REJECTED task %d: count=0, status=%s, hash_match=%v, request_hash=%s..., db_hash=%s",
			claims.TaskID, debugStatus, hashMatch, hash[:16], dbHashPrefix)
		return "", 0, fmt.Errorf("state token revoked or task not active")
	}

	return claims.WorkspaceID, claims.TaskID, nil
}

// RevokeToken clears the token hash when a task reaches terminal state.
func (s *StateTokenService) RevokeToken(taskID uint) error {
	return s.db.Table("workspace_tasks").
		Where("id = ?", taskID).
		Update("state_token_hash", nil).Error
}

func sha256Hash(data string) string {
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}
