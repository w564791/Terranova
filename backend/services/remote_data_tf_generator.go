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

// generateToken 生成临时访问token
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

// GenerateRemoteDataTFWithLogging 生成remote_data.tf文件（带日志）
func (g *RemoteDataTFGenerator) GenerateRemoteDataTFWithLogging(
	workspaceID string,
	workDir string,
	taskID *uint,
	logger *TerraformLogger,
) error {
	// 查询workspace的remote data配置
	var remoteDataList []models.WorkspaceRemoteData
	if err := g.db.Where("workspace_id = ?", workspaceID).Find(&remoteDataList).Error; err != nil {
		return fmt.Errorf("failed to get remote data list: %w", err)
	}

	// 如果没有配置remote data，不生成文件
	if len(remoteDataList) == 0 {
		logger.Debug("No remote data configured, skipping remote_data.tf generation")
		return nil
	}

	logger.Info("Generating remote_data.tf with %d remote data references...", len(remoteDataList))

	// 构建TF配置
	tfConfig := make(map[string]interface{})
	dataBlocks := make(map[string]interface{})
	localBlocks := make(map[string]interface{})

	for _, rd := range remoteDataList {
		// 为每个remote data生成临时token
		token, err := g.generateToken(workspaceID, rd.SourceWorkspaceID, taskID)
		if err != nil {
			logger.Warn("Failed to generate token for remote data %s: %v", rd.RemoteDataID, err)
			continue
		}

		logger.Debug("Generated token for remote data %s (expires: %s, max_uses: %d)",
			rd.DataName, token.ExpiresAt.Format(time.RFC3339), token.MaxUses)

		// 生成data "http" block
		dataBlockName := fmt.Sprintf("remote_%s", sanitizeName(rd.DataName))
		url := fmt.Sprintf("%s/api/v1/workspaces/%s/state-outputs/full", g.baseURL, rd.SourceWorkspaceID)

		dataBlocks[dataBlockName] = []map[string]interface{}{
			{
				"url": url,
				"request_headers": map[string]interface{}{
					"Authorization": fmt.Sprintf("Bearer %s", token.Token),
				},
			},
		}

		// 生成local block
		localBlocks[rd.DataName] = fmt.Sprintf("${jsondecode(data.http.%s.response_body).outputs}", dataBlockName)

		logger.Info("✓ Added remote data reference: %s -> %s", rd.DataName, rd.SourceWorkspaceID)
	}

	// 只有当有有效的data blocks时才生成文件
	if len(dataBlocks) == 0 {
		logger.Warn("No valid remote data blocks generated")
		return nil
	}

	// 构建完整的TF配置
	tfConfig["data"] = map[string]interface{}{
		"http": dataBlocks,
	}

	if len(localBlocks) > 0 {
		tfConfig["locals"] = localBlocks
	}

	// 写入文件
	filePath := filepath.Join(workDir, "remote_data.tf.json")
	content, err := json.MarshalIndent(tfConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal remote_data.tf.json: %w", err)
	}

	if err := os.WriteFile(filePath, content, 0644); err != nil {
		return fmt.Errorf("failed to write remote_data.tf.json: %w", err)
	}

	logger.Info("✓ Generated remote_data.tf.json (%.1f KB)", float64(len(content))/1024)
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

// GenerateTokenForAgent 为Agent模式生成临时token
// 这个方法在服务端调用，生成的token会通过API传递给Agent
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
