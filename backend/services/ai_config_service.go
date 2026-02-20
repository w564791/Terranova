package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"iac-platform/internal/models"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"gorm.io/gorm"
)

// AIConfigService AI 配置服务
type AIConfigService struct {
	db *gorm.DB
}

// NewAIConfigService 创建 AI 配置服务实例
func NewAIConfigService(db *gorm.DB) *AIConfigService {
	return &AIConfigService{db: db}
}

// GetConfig 获取 AI 配置（获取第一个配置，兼容旧接口）
func (s *AIConfigService) GetConfig() (*models.AIConfig, error) {
	var config models.AIConfig
	result := s.db.First(&config)
	if result.Error != nil {
		return nil, result.Error
	}
	// 清除敏感信息
	s.sanitizeConfig(&config)
	return &config, nil
}

// GetEnabledConfig 获取第一个启用的 AI 配置（不清除敏感信息，用于内部调用）
func (s *AIConfigService) GetEnabledConfig() (*models.AIConfig, error) {
	var config models.AIConfig
	result := s.db.Where("enabled = ?", true).First(&config)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil // 没有启用的配置，返回 nil 而不是错误
		}
		return nil, result.Error
	}
	return &config, nil
}

// GetConfigForCapability 获取指定能力的配置
// 优先级规则：
// 1. 先查找专用配置（enabled=false，capabilities 包含指定能力），按优先级降序
// 2. 如果没找到，再查找默认配置（enabled=true，capabilities 包含 "*"）
func (s *AIConfigService) GetConfigForCapability(capability string) (*models.AIConfig, error) {
	// 1. 查找专用配置（enabled=false，按优先级降序，ID 升序）
	var configs []models.AIConfig

	// 使用 JSONB 查询操作符 @> 检查数组是否包含指定元素
	err := s.db.Where("enabled = ? AND capabilities @> ?", false,
		fmt.Sprintf(`["%s"]`, capability)).
		Order("priority DESC, id ASC").
		Find(&configs).Error

	if err == nil && len(configs) > 0 {
		return &configs[0], nil
	}

	// 2. 查找默认配置（enabled=true，capabilities 包含 "*"）
	var defaultConfig models.AIConfig
	err = s.db.Where("enabled = ? AND capabilities @> ?", true, `["*"]`).
		First(&defaultConfig).Error

	if err == nil {
		return &defaultConfig, nil
	}

	// 3. 如果都没找到，返回错误
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("未找到支持 %s 的 AI 配置", capability)
	}

	return nil, err
}

// PriorityUpdate 优先级更新结构
type PriorityUpdate struct {
	ID       uint `json:"id"`
	Priority int  `json:"priority"`
}

// BatchUpdatePriorities 批量更新优先级
func (s *AIConfigService) BatchUpdatePriorities(updates []PriorityUpdate) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		for _, update := range updates {
			if err := tx.Model(&models.AIConfig{}).
				Where("id = ?", update.ID).
				Update("priority", update.Priority).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// SetAsDefault 设置为默认配置
func (s *AIConfigService) SetAsDefault(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// 1. 取消其他配置的默认状态（将 capabilities 中的 "*" 移除）
		// 先查找所有包含 "*" 的配置
		var defaultConfigs []models.AIConfig
		if err := tx.Where("id != ? AND capabilities @> ?", id, `["*"]`).
			Find(&defaultConfigs).Error; err != nil {
			return err
		}

		// 将这些配置的 capabilities 设置为空数组
		for _, cfg := range defaultConfigs {
			if err := tx.Model(&models.AIConfig{}).
				Where("id = ?", cfg.ID).
				Update("capabilities", []string{}).Error; err != nil {
				return err
			}
		}

		// 2. 设置当前配置为默认
		if err := tx.Model(&models.AIConfig{}).
			Where("id = ?", id).
			Update("capabilities", []string{"*"}).Error; err != nil {
			return err
		}

		return nil
	})
}

// ListConfigs 获取所有 AI 配置
func (s *AIConfigService) ListConfigs() ([]models.AIConfig, error) {
	var configs []models.AIConfig
	result := s.db.Order("created_at DESC").Find(&configs)
	if result.Error != nil {
		return nil, result.Error
	}
	// 清除所有配置的敏感信息
	for i := range configs {
		s.sanitizeConfig(&configs[i])
	}
	return configs, nil
}

// GetConfigByID 根据 ID 获取配置
func (s *AIConfigService) GetConfigByID(id uint) (*models.AIConfig, error) {
	var config models.AIConfig
	result := s.db.First(&config, id)
	if result.Error != nil {
		return nil, result.Error
	}
	// 清除敏感信息
	s.sanitizeConfig(&config)
	return &config, nil
}

// CreateConfig 创建新的 AI 配置
func (s *AIConfigService) CreateConfig(cfg *models.AIConfig, forceUpdate bool) error {
	// 先测试配置是否有效
	if err := s.TestConfig(cfg); err != nil {
		return fmt.Errorf("配置测试失败: %w", err)
	}

	// 验证 Batch Embedding 配置
	if cfg.EmbeddingBatchEnabled {
		if err := s.validateBatchEmbeddingSupport(cfg.ServiceType, cfg.ModelID); err != nil {
			return err
		}
	}

	// 如果新配置启用，检查是否有其他启用的配置
	if cfg.Enabled {
		var count int64
		s.db.Model(&models.AIConfig{}).Where("enabled = ?", true).Count(&count)
		if count > 0 {
			if !forceUpdate {
				return fmt.Errorf("已有其他 AI 配置处于启用状态，请先禁用其他配置或再次保存以确认")
			}
			// 强制更新：禁用所有其他配置
			if err := s.db.Model(&models.AIConfig{}).Where("enabled = ?", true).Update("enabled", false).Error; err != nil {
				return fmt.Errorf("禁用其他配置失败: %w", err)
			}
		}
	}
	return s.db.Create(cfg).Error
}

// DeleteConfig 删除 AI 配置
func (s *AIConfigService) DeleteConfig(id uint) error {
	return s.db.Delete(&models.AIConfig{}, id).Error
}

// UpdateConfig 更新 AI 配置
func (s *AIConfigService) UpdateConfig(id uint, cfg *models.AIConfig, forceUpdate bool) error {
	var existing models.AIConfig
	result := s.db.First(&existing, id)
	if result.Error != nil {
		return result.Error
	}

	// 创建测试配置（合并现有配置和新配置）
	testCfg := &models.AIConfig{
		ServiceType:         cfg.ServiceType,
		AWSRegion:           cfg.AWSRegion,
		ModelID:             cfg.ModelID,
		BaseURL:             cfg.BaseURL,
		APIKey:              cfg.APIKey,
		UseInferenceProfile: cfg.UseInferenceProfile,
	}

	// 如果没有提供新的 API Key，使用现有的
	if testCfg.APIKey == "" {
		testCfg.APIKey = existing.APIKey
	}

	// 先测试配置是否有效
	if err := s.TestConfig(testCfg); err != nil {
		return fmt.Errorf("配置测试失败: %w", err)
	}

	// 如果要启用此配置，检查是否有其他启用的配置
	if cfg.Enabled && !existing.Enabled {
		var count int64
		s.db.Model(&models.AIConfig{}).Where("id != ? AND enabled = ?", id, true).Count(&count)
		if count > 0 {
			if !forceUpdate {
				return fmt.Errorf("已有其他 AI 配置处于启用状态，请先禁用其他配置或再次保存以确认")
			}
			// 强制更新：禁用所有其他配置
			if err := s.db.Model(&models.AIConfig{}).Where("id != ? AND enabled = ?", id, true).Update("enabled", false).Error; err != nil {
				return fmt.Errorf("禁用其他配置失败: %w", err)
			}
		}
	}

	// 验证 Batch Embedding 配置
	if cfg.EmbeddingBatchEnabled {
		if err := s.validateBatchEmbeddingSupport(cfg.ServiceType, cfg.ModelID); err != nil {
			return err
		}
	}

	// 更新字段
	existing.ServiceType = cfg.ServiceType
	existing.AWSRegion = cfg.AWSRegion
	existing.ModelID = cfg.ModelID
	existing.BaseURL = cfg.BaseURL
	existing.CustomPrompt = cfg.CustomPrompt
	existing.Enabled = cfg.Enabled
	existing.RateLimitSeconds = cfg.RateLimitSeconds
	existing.UseInferenceProfile = cfg.UseInferenceProfile
	existing.Capabilities = cfg.Capabilities
	existing.CapabilityPrompts = cfg.CapabilityPrompts
	existing.Priority = cfg.Priority
	// Skill 模式配置
	existing.Mode = cfg.Mode
	existing.SkillComposition = cfg.SkillComposition
	existing.UseOptimized = cfg.UseOptimized // 优化版开关
	// Vector 搜索配置
	existing.TopK = cfg.TopK
	existing.SimilarityThreshold = cfg.SimilarityThreshold
	existing.EmbeddingBatchEnabled = cfg.EmbeddingBatchEnabled
	existing.EmbeddingBatchSize = cfg.EmbeddingBatchSize

	// 只有当提供了新的 API Key 时才更新（空字符串表示不更新）
	if cfg.APIKey != "" {
		existing.APIKey = cfg.APIKey
	}

	return s.db.Save(&existing).Error
}

// GetAvailableModels 获取指定 Region 的可用模型列表
func (s *AIConfigService) GetAvailableModels(region string) ([]models.BedrockModel, error) {
	// 加载 AWS 配置
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("无法加载 AWS 配置: %w", err)
	}

	// 创建 Bedrock 客户端
	client := bedrock.NewFromConfig(cfg)

	// 列出基础模型（不过滤类型，返回所有模型）
	input := &bedrock.ListFoundationModelsInput{}

	result, err := client.ListFoundationModels(context.TODO(), input)
	if err != nil {
		return nil, fmt.Errorf("无法获取模型列表: %w", err)
	}

	// 转换为我们的模型格式
	availableModels := make([]models.BedrockModel, 0)
	for _, modelSummary := range result.ModelSummaries {
		if modelSummary.ModelId != nil && modelSummary.ModelName != nil && modelSummary.ProviderName != nil {
			availableModels = append(availableModels, models.BedrockModel{
				ID:       aws.ToString(modelSummary.ModelId),
				Name:     aws.ToString(modelSummary.ModelName),
				Provider: aws.ToString(modelSummary.ProviderName),
			})
		}
	}

	return availableModels, nil
}

// sanitizeConfig 清除配置中的敏感信息（API Key）
func (s *AIConfigService) sanitizeConfig(cfg *models.AIConfig) {
	// 清除 API Key，不返回给前端
	if cfg.APIKey != "" {
		cfg.APIKey = "********" // 用星号表示已设置
	}
}

// GetAvailableRegions 获取支持 Bedrock 的 AWS 区域列表
func (s *AIConfigService) GetAvailableRegions() []string {
	// 返回支持 Bedrock 的主要区域
	return []string{
		"us-east-1",
		"us-west-2",
		"ap-southeast-1",
		"ap-northeast-1",
		"eu-central-1",
		"eu-west-1",
	}
}

// AnalysisResult AI 分析结果
type AnalysisResult struct {
	ErrorType  string   `json:"error_type"`
	RootCause  string   `json:"root_cause"`
	Solutions  []string `json:"solutions"`
	Prevention string   `json:"prevention"`
	Severity   string   `json:"severity"`
}

// SaveAnalysis 保存分析结果
func (s *AIConfigService) SaveAnalysis(taskID, userID string, errorMessage string, result *AnalysisResult, duration int) error {
	// 将 solutions 转换为 JSON
	solutionsJSON, err := json.Marshal(result.Solutions)
	if err != nil {
		return fmt.Errorf("无法序列化解决方案: %w", err)
	}

	// 使用 ON CONFLICT 实现真正的 UPSERT
	// 如果 task_id 已存在，则更新；否则插入
	return s.db.Exec(`
		INSERT INTO ai_error_analyses (task_id, user_id, error_message, error_type, root_cause, solutions, prevention, severity, analysis_duration, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NOW())
		ON CONFLICT (task_id) 
		DO UPDATE SET 
			user_id = EXCLUDED.user_id,
			error_message = EXCLUDED.error_message,
			error_type = EXCLUDED.error_type,
			root_cause = EXCLUDED.root_cause,
			solutions = EXCLUDED.solutions,
			prevention = EXCLUDED.prevention,
			severity = EXCLUDED.severity,
			analysis_duration = EXCLUDED.analysis_duration,
			created_at = NOW()
	`, taskID, userID, errorMessage, result.ErrorType, result.RootCause, string(solutionsJSON), result.Prevention, result.Severity, duration).Error
}

// GetAnalysis 获取任务的分析结果
func (s *AIConfigService) GetAnalysis(taskID uint) (*models.AIErrorAnalysis, error) {
	var analysis models.AIErrorAnalysis
	result := s.db.Where("task_id = ?", taskID).First(&analysis)
	if result.Error != nil {
		return nil, result.Error
	}
	return &analysis, nil
}

// TestConfig 测试 AI 配置是否有效
func (s *AIConfigService) TestConfig(cfg *models.AIConfig) error {
	// 构建测试 prompt
	testPrompt := `请返回以下 JSON 格式的测试响应：
{
  "error_type": "配置错误",
  "root_cause": "这是一个测试",
  "solutions": ["测试解决方案1", "测试解决方案2", "测试解决方案3"],
  "prevention": "这是测试预防措施",
  "severity": "low"
}

请直接返回 JSON，不要有任何额外的文字。`

	// 根据服务类型调用不同的 API
	switch cfg.ServiceType {
	case "bedrock":
		return s.testBedrock(cfg.AWSRegion, cfg.ModelID, testPrompt, cfg.UseInferenceProfile)
	case "openai", "azure_openai", "ollama":
		return s.testOpenAICompatible(cfg.BaseURL, cfg.APIKey, cfg.ModelID, testPrompt)
	default:
		return fmt.Errorf("不支持的服务类型: %s", cfg.ServiceType)
	}
}

// testBedrock 测试 Bedrock 配置
func (s *AIConfigService) testBedrock(region, modelID, prompt string, useInferenceProfile bool) error {
	// 加载 AWS 配置
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
	)
	if err != nil {
		return fmt.Errorf("无法加载 AWS 配置: %w", err)
	}

	cfg.RetryMaxAttempts = 1
	client := bedrockruntime.NewFromConfig(cfg)

	var requestBody map[string]interface{}

	// 根据模型类型构建不同的请求体
	if strings.Contains(modelID, "titan-embed") {
		// Amazon Titan Embedding 模型
		requestBody = map[string]interface{}{
			"inputText": "This is a test for embedding model.",
		}
	} else if strings.Contains(modelID, "cohere.embed") {
		// Cohere Embedding 模型
		requestBody = map[string]interface{}{
			"texts":      []string{"This is a test for embedding model."},
			"input_type": "search_document",
		}
	} else {
		// Claude 等对话模型
		requestBody = map[string]interface{}{
			"anthropic_version": "bedrock-2023-05-31",
			"max_tokens":        100,
			"messages": []map[string]string{
				{
					"role":    "user",
					"content": prompt,
				},
			},
		}
	}

	requestBodyJSON, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("无法序列化请求: %w", err)
	}

	// 根据配置决定使用哪个 model ID
	finalModelID := modelID
	if useInferenceProfile {
		if region == "us-east-1" || region == "us-west-2" {
			finalModelID = fmt.Sprintf("us.%s", modelID)
		} else if region == "eu-west-1" || region == "eu-central-1" {
			finalModelID = fmt.Sprintf("eu.%s", modelID)
		} else if region == "ap-southeast-1" || region == "ap-northeast-1" {
			finalModelID = fmt.Sprintf("apac.%s", modelID)
		}
	}

	// 调用模型
	input := &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(finalModelID),
		ContentType: aws.String("application/json"),
		Body:        requestBodyJSON,
	}

	_, err = client.InvokeModel(context.TODO(), input)
	if err != nil {
		return fmt.Errorf("Bedrock API 调用失败: %w", err)
	}

	return nil
}

// testOpenAICompatible 测试 OpenAI Compatible 配置
func (s *AIConfigService) testOpenAICompatible(baseURL, apiKey, modelID, prompt string) error {
	// 构建请求体
	requestBody := map[string]interface{}{
		"model": modelID,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"max_tokens":  100,
		"temperature": 0.7,
	}

	requestBodyJSON, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("无法序列化请求: %w", err)
	}

	// 构建完整的 URL
	url := baseURL
	if url[len(url)-1] != '/' {
		url += "/"
	}
	url += "chat/completions"

	// 创建 HTTP 请求
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBodyJSON))
	if err != nil {
		return fmt.Errorf("无法创建请求: %w", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	// 发送请求
	client := &http.Client{
		Timeout: 30 * time.Second, // 测试时使用较短的超时
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("API 请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("无法读取响应: %w", err)
	}

	// 检查状态码
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API 返回错误状态码 %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// validateBatchEmbeddingSupport 验证模型是否支持 Batch Embedding
func (s *AIConfigService) validateBatchEmbeddingSupport(serviceType, modelID string) error {
	switch serviceType {
	case "openai":
		// OpenAI 全部支持批量 embedding
		return nil
	case "bedrock":
		// Titan V2 和 Cohere Embed 都支持批量
		if strings.Contains(modelID, "titan-embed-text-v2") ||
			strings.Contains(modelID, "cohere.embed") {
			return nil
		}
		return fmt.Errorf("当前模型 %s 不支持 Batch Embedding。支持的 Bedrock 模型：amazon.titan-embed-text-v2:0, cohere.embed-*", modelID)
	default:
		return fmt.Errorf("服务类型 %s 不支持 Batch Embedding。支持的服务类型：openai, bedrock (Titan V2, Cohere Embed)", serviceType)
	}
}

// GetAnalysisWithSolutions 获取任务的分析结果（包含解析后的 solutions）
func (s *AIConfigService) GetAnalysisWithSolutions(taskID uint) (*AnalysisResult, error) {
	analysis, err := s.GetAnalysis(taskID)
	if err != nil {
		return nil, err
	}

	// 解析 solutions JSON
	var solutions []string
	if err := json.Unmarshal([]byte(analysis.Solutions), &solutions); err != nil {
		return nil, fmt.Errorf("无法解析解决方案: %w", err)
	}

	return &AnalysisResult{
		ErrorType:  analysis.ErrorType,
		RootCause:  analysis.RootCause,
		Solutions:  solutions,
		Prevention: analysis.Prevention,
		Severity:   analysis.Severity,
	}, nil
}
