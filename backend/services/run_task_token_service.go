package services

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// RunTaskTokenService handles access token generation and validation
type RunTaskTokenService struct {
	secretKey []byte
}

// NewRunTaskTokenService creates a new token service
func NewRunTaskTokenService(secretKey string) *RunTaskTokenService {
	if secretKey == "" {
		// Generate random key if not provided
		key := make([]byte, 32)
		rand.Read(key)
		secretKey = base64.StdEncoding.EncodeToString(key)
	}
	return &RunTaskTokenService{
		secretKey: []byte(secretKey),
	}
}

// RunTaskTokenClaims represents the JWT claims for run task access token
type RunTaskTokenClaims struct {
	ResultID    string `json:"result_id"`
	TaskID      uint   `json:"task_id"`
	WorkspaceID string `json:"workspace_id"`
	Stage       string `json:"stage"`
	jwt.RegisteredClaims
}

// GenerateAccessToken generates a one-time access token for run task
func (s *RunTaskTokenService) GenerateAccessToken(resultID string, taskID uint, workspaceID string, stage string, expiresIn time.Duration) (string, time.Time, error) {
	if expiresIn == 0 {
		expiresIn = 1 * time.Hour
	}

	expiresAt := time.Now().Add(expiresIn)

	claims := RunTaskTokenClaims{
		ResultID:    resultID,
		TaskID:      taskID,
		WorkspaceID: workspaceID,
		Stage:       stage,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "iac-platform",
			Subject:   resultID,
			ID:        generateTokenID(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.secretKey)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, expiresAt, nil
}

// ValidateAccessToken validates the access token and returns claims
func (s *RunTaskTokenService) ValidateAccessToken(tokenString string) (*RunTaskTokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &RunTaskTokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.secretKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if claims, ok := token.Claims.(*RunTaskTokenClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

// generateTokenID generates a unique token ID
func generateTokenID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}
