package services

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// RemoteDataTFGenerator 远程数据TF文件生成器
type RemoteDataTFGenerator struct {
	db      *gorm.DB
	baseURL string
}

// NewRemoteDataTFGenerator 创建远程数据TF文件生成器
func NewRemoteDataTFGenerator(db *gorm.DB, baseURL string) *RemoteDataTFGenerator {
	return &RemoteDataTFGenerator{
		db:      db,
		baseURL: baseURL,
	}
}

// Deprecated: generateToken is no longer used. Remote data now uses terraform_remote_state
// with TF_HTTP_* env var auth. Kept for cleanup of legacy tokens.
func (g *RemoteDataTFGenerator) generateToken(
	workspaceID string,
	sourceWorkspaceID string,
	taskID *uint,
) (*models.RemoteDataToken, error) {
	// 生成随机token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}
	tokenStr := hex.EncodeToString(tokenBytes)

	// 生成唯一的token_id
	tokenIDBytes := make([]byte, 8)
	if _, err := rand.Read(tokenIDBytes); err != nil {
		return nil, fmt.Errorf("failed to generate token_id: %w", err)
	}
	tokenID := fmt.Sprintf("rdt-%s", hex.EncodeToString(tokenIDBytes))

	// 创建token记录
	// WorkspaceID = 被访问的workspace（source）
	// RequesterWorkspaceID = 请求方workspace
	token := &models.RemoteDataToken{
		TokenID:              tokenID,
		Token:                tokenStr,
		WorkspaceID:          sourceWorkspaceID, // 被访问的workspace
		RequesterWorkspaceID: workspaceID,       // 请求方workspace
		TaskID:               taskID,
		ExpiresAt:            time.Now().Add(30 * time.Minute), // 30分钟有效
		MaxUses:              5,                                // 最多使用5次
		UsedCount:            0,
	}

	if err := g.db.Create(token).Error; err != nil {
		return nil, fmt.Errorf("failed to create token: %w", err)
	}

	return token, nil
}

// GenerateRemoteDataTFWithLogging generates remote_data.tf.json using terraform_remote_state.
// Credentials are provided by TF_HTTP_USERNAME/TF_HTTP_PASSWORD env vars (same as own state backend).
func (g *RemoteDataTFGenerator) GenerateRemoteDataTFWithLogging(
	workspaceID string,
	workDir string,
	taskID *uint,
	logger *TerraformLogger,
) error {
	var remoteDataList []models.WorkspaceRemoteData
	if err := g.db.Where("workspace_id = ?", workspaceID).Find(&remoteDataList).Error; err != nil {
		return fmt.Errorf("failed to get remote data list: %w", err)
	}

	if len(remoteDataList) == 0 {
		logger.Debug("No remote data configured, skipping remote_data.tf generation")
		return nil
	}

	logger.Info("Generating remote_data.tf with %d remote data references...", len(remoteDataList))

	tfConfig := make(map[string]interface{})
	remoteStateBlocks := make(map[string]interface{})
	localBlocks := make(map[string]interface{})

	for _, rd := range remoteDataList {
		stateURL := fmt.Sprintf("%s/api/v1/terraform/state/%s", g.baseURL, rd.SourceWorkspaceID)
		blockName := fmt.Sprintf("remote_%s", sanitizeName(rd.DataName))

		remoteStateBlocks[blockName] = map[string]interface{}{
			"backend": "http",
			"config": map[string]interface{}{
				"address": stateURL,
			},
		}

		localBlocks[rd.DataName] = fmt.Sprintf(
			"${data.terraform_remote_state.%s.outputs}", blockName)

		logger.Info("Added remote data reference: %s -> %s", rd.DataName, rd.SourceWorkspaceID)
	}

	if len(remoteStateBlocks) == 0 {
		logger.Warn("No valid remote data blocks generated")
		return nil
	}

	tfConfig["data"] = map[string]interface{}{
		"terraform_remote_state": remoteStateBlocks,
	}
	if len(localBlocks) > 0 {
		tfConfig["locals"] = localBlocks
	}

	filePath := filepath.Join(workDir, "remote_data.tf.json")
	content, err := json.MarshalIndent(tfConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal remote_data.tf.json: %w", err)
	}

	if err := os.WriteFile(filePath, content, 0644); err != nil {
		return fmt.Errorf("failed to write remote_data.tf.json: %w", err)
	}

	logger.Info("Generated remote_data.tf.json (%.1f KB)", float64(len(content))/1024)
	return nil
}

// sanitizeName 清理名称，使其符合Terraform命名规范
func sanitizeName(name string) string {
	result := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return '_'
	}, name)

	if len(result) > 0 && result[0] >= '0' && result[0] <= '9' {
		result = "_" + result
	}

	return result
}

// Deprecated: GenerateTokenForAgent is no longer used. Remote data now uses terraform_remote_state
// with TF_HTTP_* env var auth. Kept for cleanup of legacy tokens.
func (g *RemoteDataTFGenerator) GenerateTokenForAgent(workspaceID, sourceWorkspaceID string, taskID *uint) (string, error) {
	token, err := g.generateToken(workspaceID, sourceWorkspaceID, taskID)
	if err != nil {
		return "", err
	}
	return token.Token, nil
}

// CleanupExpiredTokens 清理过期的token
func (g *RemoteDataTFGenerator) CleanupExpiredTokens() error {
	result := g.db.Where("expires_at < ? OR used_count >= max_uses", time.Now()).
		Delete(&models.RemoteDataToken{})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected > 0 {
		log.Printf("Cleaned up %d expired remote data tokens", result.RowsAffected)
	}

	return nil
}
