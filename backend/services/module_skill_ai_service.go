package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"iac-platform/internal/models"
	"iac-platform/internal/observability/metrics"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"gorm.io/gorm"
)

// ModuleSkillAIService 使用 AI 生成 Module Skill 的服务
type ModuleSkillAIService struct {
	db             *gorm.DB
	skillAssembler *SkillAssembler
	configService  *AIConfigService
}

// NewModuleSkillAIService 创建服务实例
func NewModuleSkillAIService(db *gorm.DB) *ModuleSkillAIService {
	return &ModuleSkillAIService{
		db:             db,
		skillAssembler: NewSkillAssembler(db),
		configService:  NewAIConfigService(db),
	}
}

// GenerateModuleSkillContent 使用 AI 生成 Module Skill 内容
func (s *ModuleSkillAIService) GenerateModuleSkillContent(module *models.Module, schema *models.Schema) (string, error) {
	log.Printf("[ModuleSkillAIService] 开始为 Module %s 生成 Skill", module.Name)

	// 1. 获取 AI 配置（使用 module_skill_generation 能力）
	aiConfig, err := s.configService.GetConfigForCapability("module_skill_generation")
	if err != nil {
		log.Printf("[ModuleSkillAIService] 获取 AI 配置失败: %v，尝试使用默认配置", err)
		// 尝试使用默认配置
		aiConfig, err = s.configService.GetEnabledConfig()
		if err != nil || aiConfig == nil {
			return "", fmt.Errorf("没有可用的 AI 配置: %w", err)
		}
	}

	// 2. 构建 Skill 组合配置
	composition := s.getSkillComposition(aiConfig)

	// 3. 准备动态上下文
	schemaJSON, _ := json.MarshalIndent(schema.OpenAPISchema, "", "  ")
	dynamicContext := &DynamicContext{
		ModuleID: module.ID,
		ExtraContext: map[string]interface{}{
			"module_name":    module.Name,
			"provider":       module.Provider,
			"description":    module.Description,
			"openapi_schema": string(schemaJSON),
		},
	}

	// 4. 组装 Prompt
	assembleResult, err := s.skillAssembler.AssemblePrompt(composition, module.ID, dynamicContext)
	if err != nil {
		return "", fmt.Errorf("组装 Prompt 失败: %w", err)
	}

	log.Printf("[ModuleSkillAIService] Prompt 组装完成，使用了 %d 个 Skills", len(assembleResult.UsedSkillIDs))

	// 5. 调用 AI
	startTime := time.Now()
	response, err := s.callAI(aiConfig, assembleResult.Prompt)
	if err != nil {
		return "", fmt.Errorf("AI 调用失败: %w", err)
	}
	executionTime := int(time.Since(startTime).Milliseconds())

	// 6. 记录使用日志
	if err := s.skillAssembler.LogSkillUsage(
		assembleResult.UsedSkillIDs,
		"module_skill_generation",
		"",
		"system",
		&module.ID,
		aiConfig.ModelID,
		executionTime,
	); err != nil {
		log.Printf("[ModuleSkillAIService] 记录使用日志失败: %v", err)
	}

	log.Printf("[ModuleSkillAIService] AI 生成完成，耗时 %dms", executionTime)
	return response, nil
}

// getSkillComposition 获取 module_skill_generation 的 Skill 组合配置
func (s *ModuleSkillAIService) getSkillComposition(aiConfig *models.AIConfig) *models.SkillComposition {
	// 如果 AI 配置中有自定义的 Skill 组合，使用它
	if aiConfig.Mode == "skill" && aiConfig.SkillComposition.TaskSkill != "" {
		return &aiConfig.SkillComposition
	}

	// 否则使用默认的 Skill 组合
	return &models.SkillComposition{
		FoundationSkills:    []string{"platform_introduction", "output_format_standard"},
		DomainSkills:        []string{"schema_validation_rules"},
		TaskSkill:           "module_skill_generation_workflow",
		AutoLoadModuleSkill: false, // 生成 Module Skill 时不需要加载已有的 Module Skill
		ConditionalRules:    []models.SkillConditionalRule{},
	}
}

// callAI 调用 AI 服务
func (s *ModuleSkillAIService) callAI(aiConfig *models.AIConfig, prompt string) (string, error) {
	switch aiConfig.ServiceType {
	case "bedrock":
		return s.callBedrock(aiConfig, prompt)
	case "openai", "azure_openai", "ollama":
		return s.callOpenAICompatible(aiConfig, prompt)
	default:
		return "", fmt.Errorf("不支持的服务类型: %s", aiConfig.ServiceType)
	}
}

// callBedrock 调用 AWS Bedrock
func (s *ModuleSkillAIService) callBedrock(aiConfig *models.AIConfig, prompt string) (string, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(aiConfig.AWSRegion),
	)
	if err != nil {
		return "", fmt.Errorf("无法加载 AWS 配置: %w", err)
	}

	client := bedrockruntime.NewFromConfig(cfg)

	// 构建请求体（Claude 格式）
	requestBody := map[string]interface{}{
		"anthropic_version": "bedrock-2023-05-31",
		"max_tokens":        4096,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
	}

	requestBodyJSON, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("无法序列化请求: %w", err)
	}

	// 根据配置决定使用哪个 model ID
	finalModelID := aiConfig.ModelID
	if aiConfig.UseInferenceProfile {
		region := aiConfig.AWSRegion
		if region == "us-east-1" || region == "us-west-2" {
			finalModelID = fmt.Sprintf("us.%s", aiConfig.ModelID)
		} else if region == "eu-west-1" || region == "eu-central-1" {
			finalModelID = fmt.Sprintf("eu.%s", aiConfig.ModelID)
		} else if region == "ap-southeast-1" || region == "ap-northeast-1" {
			finalModelID = fmt.Sprintf("apac.%s", aiConfig.ModelID)
		}
	}

	input := &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(finalModelID),
		ContentType: aws.String("application/json"),
		Body:        requestBodyJSON,
	}

	result, err := client.InvokeModel(context.TODO(), input)
	if err != nil {
		return "", fmt.Errorf("Bedrock API 调用失败: %w", err)
	}

	// 解析响应
	var response struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(result.Body, &response); err != nil {
		return "", fmt.Errorf("无法解析响应: %w", err)
	}

	// 记录 token 用量指标
	if response.Usage.InputTokens > 0 || response.Usage.OutputTokens > 0 {
		metrics.IncAITokens("bedrock", "prompt", float64(response.Usage.InputTokens))
		metrics.IncAITokens("bedrock", "completion", float64(response.Usage.OutputTokens))
	}

	if len(response.Content) == 0 {
		return "", fmt.Errorf("AI 返回空响应")
	}

	return response.Content[0].Text, nil
}

// callOpenAICompatible 调用 OpenAI Compatible API
func (s *ModuleSkillAIService) callOpenAICompatible(aiConfig *models.AIConfig, prompt string) (string, error) {
	requestBody := map[string]interface{}{
		"model": aiConfig.ModelID,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"max_tokens":  4096,
		"temperature": 0.7,
	}

	requestBodyJSON, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("无法序列化请求: %w", err)
	}

	url := aiConfig.BaseURL
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	url += "chat/completions"

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBodyJSON))
	if err != nil {
		return "", fmt.Errorf("无法创建请求: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", aiConfig.APIKey))

	client := &http.Client{
		Timeout: 120 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("API 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("无法读取响应: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API 返回错误状态码 %d: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("无法解析响应: %w", err)
	}

	// 记录 token 用量指标
	if response.Usage.PromptTokens > 0 || response.Usage.CompletionTokens > 0 {
		metrics.IncAITokens("openai", "prompt", float64(response.Usage.PromptTokens))
		metrics.IncAITokens("openai", "completion", float64(response.Usage.CompletionTokens))
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("AI 返回空响应")
	}

	return response.Choices[0].Message.Content, nil
}
