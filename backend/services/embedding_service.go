package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"iac-platform/internal/models"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/sashabaranov/go-openai"
	"gorm.io/gorm"
)

// EmbeddingService embedding 服务
// 负责生成资源的语义向量，支持 OpenAI 和其他 embedding 模型
type EmbeddingService struct {
	db            *gorm.DB
	configService *AIConfigService
}

// NewEmbeddingService 创建 embedding 服务实例
func NewEmbeddingService(db *gorm.DB) *EmbeddingService {
	return &EmbeddingService{
		db:            db,
		configService: NewAIConfigService(db),
	}
}

// GetConfigService 获取 AI 配置服务
func (s *EmbeddingService) GetConfigService() *AIConfigService {
	return s.configService
}

// GenerateEmbedding 生成文本的 embedding 向量
// 遵循 AI Config 优先级机制
func (s *EmbeddingService) GenerateEmbedding(text string) ([]float32, error) {
	if text == "" {
		return nil, fmt.Errorf("文本不能为空")
	}

	// 1. 获取 embedding 配置（遵循优先级）
	aiConfig, err := s.configService.GetConfigForCapability("embedding")
	if err != nil {
		return nil, fmt.Errorf("未找到 embedding 的 AI 配置: %w", err)
	}

	log.Printf("[EmbeddingService] 使用配置: ID=%d, Model=%s, Priority=%d",
		aiConfig.ID, aiConfig.ModelID, aiConfig.Priority)

	// 2. 根据 service_type 调用对应的 API
	switch aiConfig.ServiceType {
	case "openai":
		return s.callOpenAIEmbedding(aiConfig, text)
	case "bedrock":
		return s.callBedrockEmbedding(aiConfig, text)
	default:
		return nil, fmt.Errorf("不支持的服务类型: %s", aiConfig.ServiceType)
	}
}

// GenerateEmbeddingsBatch 批量生成 embedding
// 一次请求处理多个文本，提高效率
// 会根据 AI Config 的 EmbeddingBatchSize 分批调用 API
func (s *EmbeddingService) GenerateEmbeddingsBatch(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("文本列表不能为空")
	}

	// 过滤空文本
	validTexts := make([]string, 0, len(texts))
	validIndices := make([]int, 0, len(texts))
	for i, text := range texts {
		if text != "" {
			validTexts = append(validTexts, text)
			validIndices = append(validIndices, i)
		}
	}

	if len(validTexts) == 0 {
		return nil, fmt.Errorf("所有文本都为空")
	}

	aiConfig, err := s.configService.GetConfigForCapability("embedding")
	if err != nil {
		return nil, fmt.Errorf("未找到 embedding 的 AI 配置: %w", err)
	}

	// 获取配置的 batch size，默认为 10
	batchSize := aiConfig.EmbeddingBatchSize
	if batchSize <= 0 {
		batchSize = 10
	}

	log.Printf("[EmbeddingService] 批量生成 embedding: %d 个文本, 使用模型: %s, batch_enabled: %v, batch_size: %d",
		len(validTexts), aiConfig.ModelID, aiConfig.EmbeddingBatchEnabled, batchSize)

	var embeddings [][]float32

	// 根据配置决定是否使用批量 API
	if aiConfig.EmbeddingBatchEnabled {
		// 按 batch_size 分批处理
		for i := 0; i < len(validTexts); i += batchSize {
			end := i + batchSize
			if end > len(validTexts) {
				end = len(validTexts)
			}
			batchTexts := validTexts[i:end]

			log.Printf("[EmbeddingService] 处理批次 %d-%d / %d", i+1, end, len(validTexts))

			var batchEmbeddings [][]float32
			switch aiConfig.ServiceType {
			case "openai":
				// OpenAI 原生支持批量
				batchEmbeddings, err = s.callOpenAIEmbeddingBatch(aiConfig, batchTexts)
			case "bedrock":
				// Bedrock Titan V2 和 Cohere Embed 支持批量
				if strings.Contains(aiConfig.ModelID, "titan-embed-text-v2") {
					batchEmbeddings, err = s.callBedrockEmbeddingBatch(aiConfig, batchTexts)
				} else if strings.Contains(aiConfig.ModelID, "cohere.embed") {
					batchEmbeddings, err = s.callCohereEmbeddingBatch(aiConfig, batchTexts)
				} else {
					// 其他 Bedrock 模型不支持批量，回退到逐个调用
					log.Printf("[EmbeddingService] 模型 %s 不支持批量，回退到逐个调用", aiConfig.ModelID)
					batchEmbeddings, err = s.generateEmbeddingsSequentially(aiConfig, batchTexts)
				}
			default:
				// 其他服务类型，逐个调用
				batchEmbeddings, err = s.generateEmbeddingsSequentially(aiConfig, batchTexts)
			}

			if err != nil {
				return nil, fmt.Errorf("批次 %d-%d 处理失败: %w", i+1, end, err)
			}

			embeddings = append(embeddings, batchEmbeddings...)
		}
	} else {
		// 未启用批量，逐个调用
		embeddings, err = s.generateEmbeddingsSequentially(aiConfig, validTexts)
		if err != nil {
			return nil, err
		}
	}

	// 将结果映射回原始索引
	results := make([][]float32, len(texts))
	for i, idx := range validIndices {
		if i < len(embeddings) {
			results[idx] = embeddings[i]
		}
	}

	return results, nil
}

// generateEmbeddingsSequentially 逐个生成 embedding（不使用批量 API）
func (s *EmbeddingService) generateEmbeddingsSequentially(aiConfig *models.AIConfig, texts []string) ([][]float32, error) {
	embeddings := make([][]float32, len(texts))
	for i, text := range texts {
		var embedding []float32
		var err error

		switch aiConfig.ServiceType {
		case "openai":
			embedding, err = s.callOpenAIEmbedding(aiConfig, text)
		case "bedrock":
			embedding, err = s.callBedrockEmbedding(aiConfig, text)
		default:
			return nil, fmt.Errorf("不支持的服务类型: %s", aiConfig.ServiceType)
		}

		if err != nil {
			return nil, fmt.Errorf("生成第 %d 个 embedding 失败: %w", i+1, err)
		}
		embeddings[i] = embedding
	}
	return embeddings, nil
}

// callOpenAIEmbedding 调用 OpenAI embedding API
func (s *EmbeddingService) callOpenAIEmbedding(config *models.AIConfig, text string) ([]float32, error) {
	client := s.createOpenAIClient(config)

	resp, err := client.CreateEmbeddings(context.Background(), openai.EmbeddingRequest{
		Model: openai.EmbeddingModel(config.ModelID),
		Input: []string{text},
	})
	if err != nil {
		return nil, fmt.Errorf("OpenAI embedding 调用失败: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("OpenAI embedding 返回空结果")
	}

	return resp.Data[0].Embedding, nil
}

// callOpenAIEmbeddingBatch 批量调用 OpenAI embedding API
func (s *EmbeddingService) callOpenAIEmbeddingBatch(config *models.AIConfig, texts []string) ([][]float32, error) {
	client := s.createOpenAIClient(config)

	// OpenAI 支持一次请求多个文本
	resp, err := client.CreateEmbeddings(context.Background(), openai.EmbeddingRequest{
		Model: openai.EmbeddingModel(config.ModelID),
		Input: texts,
	})
	if err != nil {
		return nil, fmt.Errorf("OpenAI embedding 批量调用失败: %w", err)
	}

	if len(resp.Data) != len(texts) {
		return nil, fmt.Errorf("OpenAI embedding 返回数量不匹配: 期望 %d, 实际 %d", len(texts), len(resp.Data))
	}

	results := make([][]float32, len(texts))
	for i, data := range resp.Data {
		results[i] = data.Embedding
	}

	return results, nil
}

// createOpenAIClient 创建 OpenAI 客户端
func (s *EmbeddingService) createOpenAIClient(config *models.AIConfig) *openai.Client {
	clientConfig := openai.DefaultConfig(config.APIKey)

	// 如果有自定义 base URL
	if config.BaseURL != "" {
		clientConfig.BaseURL = config.BaseURL
	}

	return openai.NewClientWithConfig(clientConfig)
}

// callBedrockEmbeddingBatch 批量调用 Bedrock Embedding API（Titan V2 支持）
func (s *EmbeddingService) callBedrockEmbeddingBatch(aiConfig *models.AIConfig, texts []string) ([][]float32, error) {
	// 加载 AWS 配置
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(aiConfig.AWSRegion),
	)
	if err != nil {
		return nil, fmt.Errorf("无法加载 AWS 配置: %w", err)
	}

	client := bedrockruntime.NewFromConfig(cfg)
	modelID := aiConfig.ModelID

	// 只有 Titan V2 支持批量
	if !strings.Contains(modelID, "titan-embed-text-v2") {
		return nil, fmt.Errorf("模型 %s 不支持批量 embedding，请使用 titan-embed-text-v2", modelID)
	}

	// Titan V2 批量请求格式: {"inputText": ["text1", "text2", ...]}
	titanRequest := map[string]interface{}{
		"inputText": texts,
	}
	requestBody, err := json.Marshal(titanRequest)
	if err != nil {
		return nil, fmt.Errorf("无法序列化请求: %w", err)
	}

	// 根据配置决定使用哪个 model ID（支持 inference profile）
	finalModelID := modelID
	if aiConfig.UseInferenceProfile {
		region := aiConfig.AWSRegion
		if region == "us-east-1" || region == "us-west-2" {
			finalModelID = fmt.Sprintf("us.%s", modelID)
		} else if region == "eu-west-1" || region == "eu-central-1" {
			finalModelID = fmt.Sprintf("eu.%s", modelID)
		} else if region == "ap-southeast-1" || region == "ap-northeast-1" {
			finalModelID = fmt.Sprintf("apac.%s", modelID)
		}
	}

	log.Printf("[EmbeddingService] 批量调用 Bedrock embedding: model=%s, region=%s, count=%d",
		finalModelID, aiConfig.AWSRegion, len(texts))

	// 调用模型
	input := &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(finalModelID),
		ContentType: aws.String("application/json"),
		Body:        requestBody,
	}

	output, err := client.InvokeModel(context.TODO(), input)
	if err != nil {
		return nil, fmt.Errorf("Bedrock batch embedding API 调用失败: %w", err)
	}

	// Titan V2 批量响应格式: {"embeddings": [[0.1, 0.2, ...], [0.3, 0.4, ...]]}
	var titanResponse struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.Unmarshal(output.Body, &titanResponse); err != nil {
		return nil, fmt.Errorf("无法解析 Titan batch embedding 响应: %w", err)
	}

	if len(titanResponse.Embeddings) != len(texts) {
		return nil, fmt.Errorf("Titan batch embedding 返回数量不匹配: 期望 %d, 实际 %d",
			len(texts), len(titanResponse.Embeddings))
	}

	log.Printf("[EmbeddingService] 批量 embedding 成功: %d 个结果", len(titanResponse.Embeddings))
	return titanResponse.Embeddings, nil
}

// callCohereEmbeddingBatch 批量调用 Cohere Embedding API
func (s *EmbeddingService) callCohereEmbeddingBatch(aiConfig *models.AIConfig, texts []string) ([][]float32, error) {
	// 加载 AWS 配置
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(aiConfig.AWSRegion),
	)
	if err != nil {
		return nil, fmt.Errorf("无法加载 AWS 配置: %w", err)
	}

	client := bedrockruntime.NewFromConfig(cfg)
	modelID := aiConfig.ModelID

	// Cohere 批量请求格式
	// 请求格式: {"texts": ["text1", "text2", ...], "input_type": "search_document", "embedding_types": ["float"]}
	cohereRequest := map[string]interface{}{
		"texts":           texts,
		"input_type":      "search_document",
		"embedding_types": []string{"float"},
	}
	requestBody, err := json.Marshal(cohereRequest)
	if err != nil {
		return nil, fmt.Errorf("无法序列化请求: %w", err)
	}

	// 根据配置决定使用哪个 model ID（支持 inference profile）
	finalModelID := modelID
	if aiConfig.UseInferenceProfile {
		region := aiConfig.AWSRegion
		if region == "us-east-1" || region == "us-west-2" {
			finalModelID = fmt.Sprintf("us.%s", modelID)
		} else if region == "eu-west-1" || region == "eu-central-1" {
			finalModelID = fmt.Sprintf("eu.%s", modelID)
		} else if region == "ap-southeast-1" || region == "ap-northeast-1" {
			finalModelID = fmt.Sprintf("apac.%s", modelID)
		}
	}

	log.Printf("[EmbeddingService] 批量调用 Cohere embedding: model=%s, region=%s, count=%d",
		finalModelID, aiConfig.AWSRegion, len(texts))

	// 调用模型
	input := &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(finalModelID),
		ContentType: aws.String("application/json"),
		Body:        requestBody,
	}

	output, err := client.InvokeModel(context.TODO(), input)
	if err != nil {
		return nil, fmt.Errorf("Cohere batch embedding API 调用失败: %w", err)
	}

	// Cohere 批量响应格式: {"embeddings": {"float": [[0.1, 0.2, ...], [0.3, 0.4, ...]]}}
	// 或者旧格式: {"embeddings": [[0.1, 0.2, ...], [0.3, 0.4, ...]]}
	var cohereResponse struct {
		Embeddings interface{} `json:"embeddings"`
	}
	if err := json.Unmarshal(output.Body, &cohereResponse); err != nil {
		return nil, fmt.Errorf("无法解析 Cohere batch embedding 响应: %w", err)
	}

	// 处理不同的响应格式
	var embeddings [][]float32
	switch v := cohereResponse.Embeddings.(type) {
	case map[string]interface{}:
		// 新格式: {"embeddings": {"float": [[...], [...]]}}
		if floatEmbeddings, ok := v["float"].([]interface{}); ok {
			embeddings = make([][]float32, len(floatEmbeddings))
			for i, emb := range floatEmbeddings {
				if embArray, ok := emb.([]interface{}); ok {
					embeddings[i] = make([]float32, len(embArray))
					for j, val := range embArray {
						if f, ok := val.(float64); ok {
							embeddings[i][j] = float32(f)
						}
					}
				}
			}
		}
	case []interface{}:
		// 旧格式: {"embeddings": [[...], [...]]}
		embeddings = make([][]float32, len(v))
		for i, emb := range v {
			if embArray, ok := emb.([]interface{}); ok {
				embeddings[i] = make([]float32, len(embArray))
				for j, val := range embArray {
					if f, ok := val.(float64); ok {
						embeddings[i][j] = float32(f)
					}
				}
			}
		}
	default:
		return nil, fmt.Errorf("未知的 Cohere embedding 响应格式")
	}

	if len(embeddings) != len(texts) {
		return nil, fmt.Errorf("Cohere batch embedding 返回数量不匹配: 期望 %d, 实际 %d",
			len(texts), len(embeddings))
	}

	log.Printf("[EmbeddingService] Cohere 批量 embedding 成功: %d 个结果", len(embeddings))
	return embeddings, nil
}

// callBedrockEmbedding 调用 Bedrock Embedding API
// 支持 Amazon Titan Embedding 和 Cohere Embedding 模型
func (s *EmbeddingService) callBedrockEmbedding(aiConfig *models.AIConfig, text string) ([]float32, error) {
	// 加载 AWS 配置
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(aiConfig.AWSRegion),
	)
	if err != nil {
		return nil, fmt.Errorf("无法加载 AWS 配置: %w", err)
	}

	client := bedrockruntime.NewFromConfig(cfg)

	// 根据模型类型构建不同的请求体
	var requestBody []byte
	modelID := aiConfig.ModelID

	if strings.Contains(modelID, "titan-embed") {
		// Amazon Titan Embedding 模型
		// 请求格式: {"inputText": "text"}
		titanRequest := map[string]interface{}{
			"inputText": text,
		}
		requestBody, err = json.Marshal(titanRequest)
	} else if strings.Contains(modelID, "cohere.embed") {
		// Cohere Embedding 模型 (v4)
		// 请求格式: {"texts": ["text"], "input_type": "search_query", "embedding_types": ["float"]}
		// 注意：搜索查询使用 "search_query"，文档索引使用 "search_document"
		cohereRequest := map[string]interface{}{
			"texts":           []string{text},
			"input_type":      "search_query", // 搜索时使用 search_query
			"embedding_types": []string{"float"},
		}
		requestBody, err = json.Marshal(cohereRequest)
	} else {
		return nil, fmt.Errorf("不支持的 Bedrock embedding 模型: %s", modelID)
	}

	if err != nil {
		return nil, fmt.Errorf("无法序列化请求: %w", err)
	}

	// 根据配置决定使用哪个 model ID（支持 inference profile）
	finalModelID := modelID
	if aiConfig.UseInferenceProfile {
		region := aiConfig.AWSRegion
		if region == "us-east-1" || region == "us-west-2" {
			finalModelID = fmt.Sprintf("us.%s", modelID)
		} else if region == "eu-west-1" || region == "eu-central-1" {
			finalModelID = fmt.Sprintf("eu.%s", modelID)
		} else if region == "ap-southeast-1" || region == "ap-northeast-1" {
			finalModelID = fmt.Sprintf("apac.%s", modelID)
		}
	}

	log.Printf("[EmbeddingService] 调用 Bedrock embedding: model=%s, region=%s", finalModelID, aiConfig.AWSRegion)

	// 调用模型
	input := &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(finalModelID),
		ContentType: aws.String("application/json"),
		Body:        requestBody,
	}

	output, err := client.InvokeModel(context.TODO(), input)
	if err != nil {
		return nil, fmt.Errorf("Bedrock embedding API 调用失败: %w", err)
	}

	// 解析响应
	if strings.Contains(modelID, "titan-embed") {
		// Titan 响应格式: {"embedding": [0.1, 0.2, ...]}
		var titanResponse struct {
			Embedding []float32 `json:"embedding"`
		}
		if err := json.Unmarshal(output.Body, &titanResponse); err != nil {
			return nil, fmt.Errorf("无法解析 Titan embedding 响应: %w", err)
		}
		return titanResponse.Embedding, nil
	} else if strings.Contains(modelID, "cohere.embed") {
		// Cohere 响应格式（v4 新格式）: {"embeddings": {"float": [[0.1, 0.2, ...]]}}
		// 或者旧格式: {"embeddings": [[0.1, 0.2, ...]]}
		var cohereResponse struct {
			Embeddings interface{} `json:"embeddings"`
		}
		if err := json.Unmarshal(output.Body, &cohereResponse); err != nil {
			return nil, fmt.Errorf("无法解析 Cohere embedding 响应: %w", err)
		}

		// 处理不同的响应格式
		var embedding []float32
		switch v := cohereResponse.Embeddings.(type) {
		case map[string]interface{}:
			// 新格式 (v4): {"embeddings": {"float": [[...]]}}
			if floatEmbeddings, ok := v["float"].([]interface{}); ok && len(floatEmbeddings) > 0 {
				if embArray, ok := floatEmbeddings[0].([]interface{}); ok {
					embedding = make([]float32, len(embArray))
					for j, val := range embArray {
						if f, ok := val.(float64); ok {
							embedding[j] = float32(f)
						}
					}
				}
			}
		case []interface{}:
			// 旧格式: {"embeddings": [[...]]}
			if len(v) > 0 {
				if embArray, ok := v[0].([]interface{}); ok {
					embedding = make([]float32, len(embArray))
					for j, val := range embArray {
						if f, ok := val.(float64); ok {
							embedding[j] = float32(f)
						}
					}
				}
			}
		default:
			return nil, fmt.Errorf("未知的 Cohere embedding 响应格式: %T", cohereResponse.Embeddings)
		}

		if len(embedding) == 0 {
			return nil, fmt.Errorf("Cohere embedding 返回空结果")
		}
		return embedding, nil
	}

	return nil, fmt.Errorf("未知的响应格式")
}

// BuildEmbeddingText 构建用于生成 embedding 的文本
// 从资源的各个字段提取关键信息，组合成语义丰富的文本
// 包含中英文双语关键词，支持跨语言搜索
func (s *EmbeddingService) BuildEmbeddingText(r *models.ResourceIndex) string {
	parts := []string{}

	// 资源名称（添加中文翻译）
	if r.CloudResourceName != "" {
		parts = append(parts, r.CloudResourceName)
		// 添加名称中关键词的中文翻译
		parts = append(parts, translateKeywords(r.CloudResourceName))
	}

	// 描述（添加中文翻译）
	if r.Description != "" {
		parts = append(parts, r.Description)
		parts = append(parts, translateKeywords(r.Description))
	}

	// 重要的 Tags
	if len(r.Tags) > 0 {
		var tags map[string]interface{}
		if err := json.Unmarshal(r.Tags, &tags); err == nil {
			if name, ok := tags["Name"].(string); ok && name != "" {
				parts = append(parts, name)
				parts = append(parts, translateKeywords(name))
			}
			if env, ok := tags["Environment"].(string); ok && env != "" {
				parts = append(parts, env)
				parts = append(parts, translateKeywords(env))
			}
			if team, ok := tags["Team"].(string); ok && team != "" {
				parts = append(parts, team)
			}
			if project, ok := tags["Project"].(string); ok && project != "" {
				parts = append(parts, project)
				parts = append(parts, translateKeywords(project))
			}
		}
	}

	// 资源类型的可读名称（已包含中文）
	parts = append(parts, getResourceTypeDisplayName(r.ResourceType))

	// 区域信息（已包含中文）
	if r.CloudRegion != "" {
		parts = append(parts, r.CloudRegion)
		parts = append(parts, getRegionDisplayName(r.CloudRegion))
	}

	// 云提供商
	if r.CloudProvider != "" {
		parts = append(parts, r.CloudProvider)
	}

	return strings.Join(parts, " ")
}

// BuildEmbeddingTextFromInfo 从 CMDBResourceInfo 构建 embedding 文本
func (s *EmbeddingService) BuildEmbeddingTextFromInfo(name, description, resourceType, region string, tags map[string]string) string {
	parts := []string{}

	if name != "" {
		parts = append(parts, name)
	}

	if description != "" {
		parts = append(parts, description)
	}

	// 重要的 Tags
	if tags != nil {
		if tagName, ok := tags["Name"]; ok && tagName != "" {
			parts = append(parts, tagName)
		}
		if env, ok := tags["Environment"]; ok && env != "" {
			parts = append(parts, env)
		}
		if team, ok := tags["Team"]; ok && team != "" {
			parts = append(parts, team)
		}
	}

	// 资源类型的可读名称
	parts = append(parts, getResourceTypeDisplayName(resourceType))

	// 区域信息
	if region != "" {
		parts = append(parts, region)
		parts = append(parts, getRegionDisplayName(region))
	}

	return strings.Join(parts, " ")
}

// GetConfigStatus 获取 embedding 配置状态
func (s *EmbeddingService) GetConfigStatus() *models.EmbeddingConfigStatus {
	config, err := s.configService.GetConfigForCapability("embedding")

	if err != nil || config == nil {
		return &models.EmbeddingConfigStatus{
			Configured: false,
			Message:    "未配置 embedding 能力的 AI 配置，向量搜索功能不可用",
			Help:       "请在 AI 配置管理界面添加支持 embedding 能力的配置",
		}
	}

	// Bedrock 使用 IAM 认证，不需要 API Key
	// OpenAI 等服务需要 API Key
	hasAPIKey := config.ServiceType == "bedrock" || config.APIKey != ""

	return &models.EmbeddingConfigStatus{
		Configured:  true,
		HasAPIKey:   hasAPIKey,
		ModelID:     config.ModelID,
		ServiceType: config.ServiceType,
		Priority:    config.Priority,
		Message:     "embedding 配置已就绪",
	}
}

// getResourceTypeDisplayName 获取资源类型的可读名称
func getResourceTypeDisplayName(resourceType string) string {
	displayNames := map[string]string{
		"aws_vpc":                  "VPC 虚拟私有云",
		"aws_subnet":               "子网 Subnet",
		"aws_security_group":       "安全组 Security Group",
		"aws_instance":             "EC2 实例",
		"aws_s3_bucket":            "S3 存储桶",
		"aws_iam_role":             "IAM 角色",
		"aws_iam_policy":           "IAM 策略",
		"aws_db_instance":          "RDS 数据库实例",
		"aws_eks_cluster":          "EKS 集群",
		"aws_lambda_function":      "Lambda 函数",
		"aws_eip":                  "弹性 IP",
		"aws_nat_gateway":          "NAT 网关",
		"aws_internet_gateway":     "互联网网关",
		"aws_route_table":          "路由表",
		"aws_lb":                   "负载均衡器",
		"aws_autoscaling_group":    "自动伸缩组",
		"aws_cloudwatch_log_group": "CloudWatch 日志组",
		"aws_sns_topic":            "SNS 主题",
		"aws_sqs_queue":            "SQS 队列",
		"aws_dynamodb_table":       "DynamoDB 表",
	}

	if name, ok := displayNames[resourceType]; ok {
		return name
	}
	return resourceType
}

// getRegionDisplayName 获取区域的可读名称
func getRegionDisplayName(region string) string {
	displayNames := map[string]string{
		"ap-northeast-1":  "东京",
		"ap-northeast-1a": "东京1a",
		"ap-northeast-1c": "东京1c",
		"ap-northeast-1d": "东京1d",
		"ap-northeast-2":  "首尔",
		"ap-southeast-1":  "新加坡",
		"ap-southeast-2":  "悉尼",
		"ap-east-1":       "香港",
		"us-east-1":       "美东弗吉尼亚",
		"us-east-2":       "美东俄亥俄",
		"us-west-1":       "美西加州",
		"us-west-2":       "美西俄勒冈",
		"eu-west-1":       "欧洲爱尔兰",
		"eu-west-2":       "欧洲伦敦",
		"eu-central-1":    "欧洲法兰克福",
		"cn-north-1":      "中国北京",
		"cn-northwest-1":  "中国宁夏",
	}

	if name, ok := displayNames[region]; ok {
		return name
	}
	return ""
}

// translateKeywords 将文本中的英文关键词翻译成中文
// 用于支持跨语言搜索（用户用中文搜索英文资源名称）
func translateKeywords(text string) string {
	// 常见的业务关键词翻译映射
	translations := map[string]string{
		// 业务系统
		"exchange":   "交易所 交易",
		"trading":    "交易 买卖",
		"payment":    "支付 付款",
		"order":      "订单",
		"user":       "用户",
		"account":    "账户 账号",
		"wallet":     "钱包",
		"market":     "市场",
		"price":      "价格",
		"stock":      "股票 库存",
		"finance":    "金融 财务",
		"bank":       "银行",
		"loan":       "贷款",
		"insurance":  "保险",
		"investment": "投资",

		// 环境
		"production":  "生产环境 线上",
		"prod":        "生产环境 线上",
		"staging":     "预发布环境 测试",
		"development": "开发环境",
		"dev":         "开发环境",
		"test":        "测试环境",
		"qa":          "质量保证 测试",

		// 基础设施
		"database": "数据库",
		"db":       "数据库",
		"cache":    "缓存",
		"redis":    "缓存 Redis",
		"queue":    "队列 消息队列",
		"message":  "消息",
		"storage":  "存储",
		"backup":   "备份",
		"log":      "日志",
		"monitor":  "监控",
		"alert":    "告警 报警",

		// 网络
		"network":  "网络",
		"vpc":      "虚拟私有云 VPC",
		"subnet":   "子网",
		"public":   "公有 公共 公网",
		"private":  "私有 内网 私网",
		"gateway":  "网关",
		"firewall": "防火墙",
		"security": "安全",
		"load":     "负载",
		"balancer": "均衡器",
		"proxy":    "代理",
		"dns":      "域名解析",
		"cdn":      "内容分发",

		// 应用
		"web":      "网站 Web",
		"api":      "接口 API",
		"service":  "服务",
		"app":      "应用",
		"frontend": "前端",
		"backend":  "后端",
		"mobile":   "移动端",
		"admin":    "管理后台",

		// 团队
		"ops":      "运维",
		"devops":   "开发运维",
		"platform": "平台",
		"infra":    "基础设施",
		"core":     "核心",
		"common":   "公共 通用",
		"shared":   "共享",
	}

	// 将文本转为小写进行匹配
	lowerText := strings.ToLower(text)
	var result []string

	for keyword, translation := range translations {
		if strings.Contains(lowerText, keyword) {
			result = append(result, translation)
		}
	}

	return strings.Join(result, " ")
}

// VectorToString 将向量转换为 pgvector 格式的字符串
func VectorToString(v []float32) string {
	if len(v) == 0 {
		return ""
	}
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = fmt.Sprintf("%f", f)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
