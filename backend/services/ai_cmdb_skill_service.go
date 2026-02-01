package services

import (
	"encoding/json"
	"fmt"
	"iac-platform/internal/models"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ========== 优化相关常量 ==========

const (
	ParallelExecutionTimeout = 15 * time.Second // 并行执行总超时
	CMDBQueryTimeout         = 10 * time.Second // CMDB 查询超时
	SkillSelectionTimeout    = 8 * time.Second  // Skill 选择超时
)

// ========== 优化相关数据结构 ==========

// CMDBAssessmentWithQueryPlan CMDB 评估结果（合并判断和查询计划）
type CMDBAssessmentWithQueryPlan struct {
	NeedCMDB      bool                `json:"need_cmdb"`
	Reason        string              `json:"reason"`
	ResourceTypes []string            `json:"resource_types"`
	QueryPlan     []CMDBQueryPlanItem `json:"query_plan"`
}

// CMDBQueryPlanItem CMDB 查询计划项
type CMDBQueryPlanItem struct {
	ResourceType string                 `json:"resource_type"`
	TargetField  string                 `json:"target_field,omitempty"` // 目标字段名，用于区分同类型资源的不同用途
	Filters      map[string]interface{} `json:"filters"`
	Limit        int                    `json:"limit"`
}

// DomainSkillInfo Domain Skill 简要信息（用于 AI 选择）
type DomainSkillInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// DomainSkillSelectionResult AI 选择 Domain Skills 的结果
type DomainSkillSelectionResult struct {
	SelectedSkills []string `json:"selected_skills"`
	Reason         string   `json:"reason"`
}

// ParallelExecutionResult 并行执行结果
type ParallelExecutionResult struct {
	CMDBResults    *CMDBQueryResults
	NeedSelection  bool
	CMDBLookups    []CMDBLookupResult
	CMDBError      error
	SelectedSkills []string
	SkillError     error
}

// AICMDBSkillService AI + CMDB + Skill 集成服务
// 使用 Skill 组合模式替代硬编码的 Prompt
type AICMDBSkillService struct {
	db               *gorm.DB
	aiFormService    *AIFormService
	cmdbService      *CMDBService
	configService    *AIConfigService
	embeddingService *EmbeddingService
	skillAssembler   *SkillAssembler
}

// NewAICMDBSkillService 创建 AI + CMDB + Skill 集成服务实例
func NewAICMDBSkillService(db *gorm.DB) *AICMDBSkillService {
	return &AICMDBSkillService{
		db:               db,
		aiFormService:    NewAIFormService(db),
		cmdbService:      NewCMDBService(db),
		configService:    NewAIConfigService(db),
		embeddingService: NewEmbeddingService(db),
		skillAssembler:   NewSkillAssembler(db),
	}
}

// GenerateConfigWithCMDBSkill 使用 Skill 模式生成配置
func (s *AICMDBSkillService) GenerateConfigWithCMDBSkill(
	userID string,
	moduleID uint,
	userDescription string,
	workspaceID string,
	organizationID string,
	userSelections map[string]interface{}, // 支持 string 或 []string
	currentConfig map[string]interface{},
	mode string,
	resourceInfoMap map[string]interface{}, // 完整的资源信息（包括 ARN）
) (*GenerateConfigWithCMDBResponse, error) {
	totalTimer := NewTimer()
	log.Printf("[AICMDBSkillService] ========== 开始 Skill 模式配置生成 ==========")
	log.Printf("[AICMDBSkillService] 用户 ID: %s, Module ID: %d", userID, moduleID)

	// 1. 获取 AI 配置
	configTimer := NewTimer()
	aiConfig, err := s.configService.GetConfigForCapability("form_generation")
	if err != nil || aiConfig == nil {
		IncAICallCount("form_generation", "config_error")
		return nil, fmt.Errorf("未找到 form_generation 的 AI 配置: %v", err)
	}
	RecordAICallDuration("form_generation", "get_config", configTimer.ElapsedMs())
	log.Printf("[AICMDBSkillService] [耗时] 获取 AI 配置: %.0fms", configTimer.ElapsedMs())

	// 转换 userSelections 为 map[string]string
	convertedSelections := s.convertUserSelections(userSelections)

	// 2. 检查配置模式
	if aiConfig.Mode != "skill" {
		log.Printf("[AICMDBSkillService] AI 配置模式为 '%s'，降级到传统模式", aiConfig.Mode)
		return s.fallbackToLegacyMode(userID, moduleID, userDescription, workspaceID, organizationID, convertedSelections, currentConfig, mode)
	}

	// 3. 获取 Skill 组合配置
	composition := &aiConfig.SkillComposition
	if len(composition.FoundationSkills) == 0 && composition.TaskSkill == "" {
		log.Printf("[AICMDBSkillService] 未配置 SkillComposition，使用默认配置")
		composition = s.getDefaultSkillComposition()
	}

	// 4. 意图断言（使用 Skill 模式）
	assertionTimer := NewTimer()
	log.Printf("[AICMDBSkillService] 步骤 1: 意图断言检查")
	assertionResult, err := s.performIntentAssertion(userID, userDescription)
	RecordAICallDuration("form_generation", "intent_assertion", assertionTimer.ElapsedMs())
	log.Printf("[AICMDBSkillService] [耗时] 步骤 1 意图断言: %.0fms", assertionTimer.ElapsedMs())
	if err != nil {
		log.Printf("[AICMDBSkillService] 意图断言服务不可用: %v，继续执行", err)
	} else if assertionResult != nil && !assertionResult.IsSafe {
		IncAICallCount("form_generation", "blocked")
		return &GenerateConfigWithCMDBResponse{
			Status:  "blocked",
			Message: assertionResult.Suggestion,
		}, nil
	}

	// 5. CMDB 查询（如果需要）
	cmdbTimer := NewTimer()
	var cmdbData string
	var cmdbLookups []CMDBLookupResult
	needSelection := false

	// 如果用户已经选择了资源，直接使用选择的资源，不再查询 CMDB
	if len(convertedSelections) > 0 {
		log.Printf("[AICMDBSkillService] 步骤 2: 使用用户选择的资源（跳过 CMDB 查询）")
		log.Printf("[AICMDBSkillService] 用户选择: %v", convertedSelections)
		cmdbData = s.buildCMDBDataFromSelections(convertedSelections)
		RecordAICallDuration("form_generation", "user_selection", cmdbTimer.ElapsedMs())
		log.Printf("[AICMDBSkillService] [耗时] 步骤 2 构建用户选择数据: %.0fms", cmdbTimer.ElapsedMs())
	} else if s.shouldUseCMDB(userDescription) {
		log.Printf("[AICMDBSkillService] 步骤 2: CMDB 查询")
		cmdbResults, err := s.performCMDBQuery(userID, userDescription, convertedSelections)
		RecordAICallDuration("form_generation", "cmdb_query", cmdbTimer.ElapsedMs())
		log.Printf("[AICMDBSkillService] [耗时] 步骤 2 CMDB 查询: %.0fms", cmdbTimer.ElapsedMs())
		if err != nil {
			log.Printf("[AICMDBSkillService] CMDB 查询失败: %v", err)
		} else {
			needSelection, cmdbLookups = s.checkNeedSelection(cmdbResults)
			if needSelection {
				RecordAICallDuration("form_generation", "total_need_selection", totalTimer.ElapsedMs())
				log.Printf("[AICMDBSkillService] [耗时] 总计（返回需要选择）: %.0fms", totalTimer.ElapsedMs())
				return &GenerateConfigWithCMDBResponse{
					Status:      "need_selection",
					CMDBLookups: cmdbLookups,
					Message:     "找到多个匹配的资源，请选择",
				}, nil
			}
			cmdbData = s.buildCMDBDataString(cmdbResults)
		}
	} else {
		log.Printf("[AICMDBSkillService] [耗时] 步骤 2 跳过 CMDB 查询: %.0fms", cmdbTimer.ElapsedMs())
	}

	// 6. 获取 Schema 数据
	schemaTimer := NewTimer()
	schemaData := s.getSchemaData(moduleID)
	RecordAICallDuration("form_generation", "get_schema", schemaTimer.ElapsedMs())
	log.Printf("[AICMDBSkillService] [耗时] 获取 Schema 数据: %.0fms", schemaTimer.ElapsedMs())

	// 7. 构建动态上下文
	dynamicContext := &DynamicContext{
		UserDescription: userDescription,
		WorkspaceID:     workspaceID,
		OrganizationID:  organizationID,
		ModuleID:        moduleID,
		UseCMDB:         cmdbData != "",
		CurrentConfig:   currentConfig,
		CMDBData:        cmdbData,
		SchemaData:      schemaData,
		ExtraContext: map[string]interface{}{
			"mode": mode,
		},
	}

	// 8. 组装 Prompt
	assembleTimer := NewTimer()
	log.Printf("[AICMDBSkillService] 步骤 3: 组装 Skill Prompt")
	assembleResult, err := s.skillAssembler.AssemblePrompt(composition, moduleID, dynamicContext)
	RecordSkillAssemblyDuration("form_generation", len(assembleResult.UsedSkillNames), assembleTimer.ElapsedMs())
	log.Printf("[AICMDBSkillService] [耗时] 步骤 3 组装 Skill Prompt: %.0fms", assembleTimer.ElapsedMs())
	if err != nil {
		log.Printf("[AICMDBSkillService] Skill 组装失败: %v，降级到传统模式", err)
		return s.fallbackToLegacyMode(userID, moduleID, userDescription, workspaceID, organizationID, convertedSelections, currentConfig, mode)
	}

	log.Printf("[AICMDBSkillService] 使用了 %d 个 Skills: %v", len(assembleResult.UsedSkillNames), assembleResult.UsedSkillNames)

	// 9. 调用 AI 生成配置
	aiTimer := NewTimer()
	log.Printf("[AICMDBSkillService] 步骤 4: 调用 AI 生成配置")
	aiResult, err := s.aiFormService.callAI(aiConfig, assembleResult.Prompt)
	RecordAICallDuration("form_generation", "ai_call", aiTimer.ElapsedMs())
	log.Printf("[AICMDBSkillService] [耗时] 步骤 4 AI 调用: %.0fms", aiTimer.ElapsedMs())
	if err != nil {
		IncAICallCount("form_generation", "ai_error")
		return nil, fmt.Errorf("AI 调用失败: %w", err)
	}

	// 10. 解析 AI 响应
	parseTimer := NewTimer()
	response, err := s.parseAIResponse(aiResult, moduleID)
	RecordAICallDuration("form_generation", "parse_response", parseTimer.ElapsedMs())
	log.Printf("[AICMDBSkillService] [耗时] 解析 AI 响应: %.0fms", parseTimer.ElapsedMs())
	if err != nil {
		IncAICallCount("form_generation", "parse_error")
		return nil, fmt.Errorf("解析 AI 响应失败: %w", err)
	}

	// 11. 添加 CMDB 查询记录
	response.CMDBLookups = cmdbLookups

	// 12. 记录 Skill 使用日志和指标
	executionTimeMs := int(totalTimer.ElapsedMs())
	RecordAICallDuration("form_generation", "total", totalTimer.ElapsedMs())
	IncAICallCount("form_generation", "success")

	if err := s.skillAssembler.LogSkillUsage(
		assembleResult.UsedSkillIDs,
		"form_generation",
		workspaceID,
		userID,
		&moduleID,
		aiConfig.ModelID,
		executionTimeMs,
	); err != nil {
		log.Printf("[AICMDBSkillService] 记录 Skill 使用日志失败: %v", err)
	}

	log.Printf("[AICMDBSkillService] ========== Skill 模式配置生成完成 ==========")
	log.Printf("[AICMDBSkillService] [耗时] 总计: %dms", executionTimeMs)
	return response, nil
}

// getDefaultSkillComposition 获取默认的 Skill 组合配置
func (s *AICMDBSkillService) getDefaultSkillComposition() *models.SkillComposition {
	return &models.SkillComposition{
		FoundationSkills: []string{
			"platform_introduction",
			"output_format_standard",
		},
		DomainSkills: []string{
			"schema_validation_rules",
		},
		TaskSkill:           "resource_generation_workflow",
		AutoLoadModuleSkill: true,
		ConditionalRules: []models.SkillConditionalRule{
			{
				Condition: "use_cmdb == true",
				AddSkills: []string{"cmdb_resource_matching", "cmdb_resource_types", "region_mapping"},
			},
		},
	}
}

// performIntentAssertion 执行意图断言
func (s *AICMDBSkillService) performIntentAssertion(userID string, userDescription string) (*IntentAssertionResult, error) {
	return s.aiFormService.AssertIntent(userID, userDescription)
}

// convertUserSelections 将 map[string]interface{} 转换为 map[string]string
// 支持 string 和 []string 类型的值
func (s *AICMDBSkillService) convertUserSelections(selections map[string]interface{}) map[string]string {
	if selections == nil {
		return nil
	}

	result := make(map[string]string)
	for key, value := range selections {
		switch v := value.(type) {
		case string:
			result[key] = v
		case []interface{}:
			// 数组类型：取第一个元素（单选）或用逗号连接（多选）
			if len(v) == 1 {
				if str, ok := v[0].(string); ok {
					result[key] = str
				}
			} else if len(v) > 1 {
				// 多选情况：用逗号连接
				strs := make([]string, 0, len(v))
				for _, item := range v {
					if str, ok := item.(string); ok {
						strs = append(strs, str)
					}
				}
				result[key] = strings.Join(strs, ",")
			}
		}
	}
	return result
}

// shouldUseCMDB 判断是否需要使用 CMDB
// 使用混合方案：关键词快速检测 + AI 语义分析后备
func (s *AICMDBSkillService) shouldUseCMDB(userDescription string) bool {
	// 第一层：关键词快速检测（快速路径）
	if s.shouldUseCMDBByKeywords(userDescription) {
		log.Printf("[AICMDBSkillService] CMDB 需求判断: 关键词匹配成功")
		return true
	}

	// 第二层：AI 语义分析（后备方案）
	needCMDB, reason := s.shouldUseCMDBByAI(userDescription)
	if needCMDB {
		log.Printf("[AICMDBSkillService] CMDB 需求判断: AI 分析判定需要 CMDB, 原因: %s", reason)
		return true
	}

	log.Printf("[AICMDBSkillService] CMDB 需求判断: 不需要 CMDB 查询")
	return false
}

// shouldUseCMDBByKeywords 通过关键词检测判断是否需要 CMDB
func (s *AICMDBSkillService) shouldUseCMDBByKeywords(userDescription string) bool {
	// 扩展的关键词列表
	cmdbKeywords := []string{
		// 直接 CMDB 引用
		"cmdb", "配置库", "资产库", "资源库",

		// IAM/权限相关
		"role", "角色", "iam", "policy", "策略", "权限",
		"principal", "assume", "trust",

		// AWS 服务名称（可能需要引用现有资源）
		"ec2", "lambda", "ecs", "eks", "rds", "dynamodb",
		"sqs", "sns", "kinesis", "cloudfront",
		"apigateway", "elb", "alb", "nlb",

		// 资源引用表达
		"arn", "引用", "关联", "绑定", "连接",
		"来自", "from", "attach", "associate",

		// 网络相关
		"vpc", "subnet", "security", "安全组", "子网",
		"cidr", "网段", "路由", "route", "gateway", "网关",
		"nat", "igw", "endpoint", "peering",

		// 环境标识
		"existing", "现有", "已有", "使用",
		"exchange", "production", "生产", "开发", "dev", "staging", "test",
		"prod", "uat", "sit",
	}

	lowerDesc := strings.ToLower(userDescription)
	for _, keyword := range cmdbKeywords {
		if strings.Contains(lowerDesc, keyword) {
			log.Printf("[AICMDBSkillService] 关键词匹配: '%s'", keyword)
			return true
		}
	}
	return false
}

// CMDBNeedAssessment CMDB 需求评估结果
type CMDBNeedAssessment struct {
	NeedCMDB      bool     `json:"need_cmdb"`
	Reason        string   `json:"reason"`
	ResourceTypes []string `json:"resource_types"`
}

// shouldUseCMDBByAI 通过 AI 语义分析判断是否需要 CMDB
// 使用 cmdb_need_assessment 能力的 AI 配置，按优先级自动选择
func (s *AICMDBSkillService) shouldUseCMDBByAI(userDescription string) (bool, string) {
	// 通过 GetConfigForCapability 获取配置，它会自动按优先级选择
	aiConfig, err := s.configService.GetConfigForCapability("cmdb_need_assessment")
	if err != nil || aiConfig == nil {
		log.Printf("[AICMDBSkillService] cmdb_need_assessment AI 配置不可用，跳过 AI 判断: %v", err)
		return false, ""
	}
	log.Printf("[AICMDBSkillService] 使用 AI 配置进行 CMDB 需求判断: id=%d, capability=%v, model=%s, priority=%d",
		aiConfig.ID, aiConfig.Capabilities, aiConfig.ModelID, aiConfig.Priority)

	// 构建 Prompt（支持 Skill 模式）
	prompt := s.buildCMDBNeedAssessmentPromptWithSkill(aiConfig, userDescription)

	// 调用 AI
	result, err := s.aiFormService.callAI(aiConfig, prompt)
	if err != nil {
		log.Printf("[AICMDBSkillService] AI 调用失败，跳过 AI 判断: %v", err)
		return false, ""
	}

	// 解析结果
	assessment, err := s.parseCMDBNeedAssessment(result)
	if err != nil {
		log.Printf("[AICMDBSkillService] AI 结果解析失败: %v", err)
		return false, ""
	}

	return assessment.NeedCMDB, assessment.Reason
}

// buildCMDBNeedAssessmentPromptWithSkill 构建 CMDB 需求评估 Prompt（支持 Skill 模式）
func (s *AICMDBSkillService) buildCMDBNeedAssessmentPromptWithSkill(aiConfig *models.AIConfig, userDescription string) string {
	// 检查是否使用 Skill 模式
	if aiConfig.Mode == "skill" {
		log.Printf("[AICMDBSkillService] 使用 Skill 模式构建 CMDB 需求评估 Prompt")

		// 获取 Skill 组合配置
		composition := &aiConfig.SkillComposition
		if len(composition.FoundationSkills) == 0 && composition.TaskSkill == "" {
			// 使用默认的 CMDB 需求评估 Skill 组合
			composition = s.getDefaultCMDBNeedAssessmentSkillComposition()
		}

		// 构建动态上下文
		dynamicContext := &DynamicContext{
			UserDescription: userDescription,
			ExtraContext: map[string]interface{}{
				"capability":   "cmdb_need_assessment",
				"user_request": userDescription,
			},
		}

		// 组装 Prompt
		result, err := s.skillAssembler.AssemblePrompt(composition, 0, dynamicContext)
		if err != nil {
			log.Printf("[AICMDBSkillService] Skill 组装失败: %v，降级到硬编码 Prompt", err)
		} else {
			log.Printf("[AICMDBSkillService] CMDB 需求评估使用了 %d 个 Skills: %v",
				len(result.UsedSkillNames), result.UsedSkillNames)
			return result.Prompt
		}
	}

	// 检查是否有自定义 Prompt
	if aiConfig.CapabilityPrompts != nil {
		if customPrompt, ok := aiConfig.CapabilityPrompts["cmdb_need_assessment"]; ok && customPrompt != "" {
			log.Printf("[AICMDBSkillService] 使用自定义 cmdb_need_assessment Prompt")
			return strings.ReplaceAll(customPrompt, "{user_description}", userDescription)
		}
	}

	// 降级到硬编码 Prompt
	log.Printf("[AICMDBSkillService] 使用硬编码 CMDB 需求评估 Prompt")
	return s.buildCMDBNeedAssessmentPrompt(userDescription)
}

// getDefaultCMDBNeedAssessmentSkillComposition 获取默认的 CMDB 需求评估 Skill 组合配置
// 注意：cmdb_need_assessment 是一个简单的判断任务，Task Skill 已包含完整的判断规则和输出格式
// 不需要额外的 Foundation 和 Domain Skill，因为它们的输出格式与 cmdb_need_assessment 不匹配
func (s *AICMDBSkillService) getDefaultCMDBNeedAssessmentSkillComposition() *models.SkillComposition {
	return &models.SkillComposition{
		FoundationSkills:    []string{}, // 不需要，Task Skill 已定义输出格式
		DomainSkills:        []string{}, // 不需要，这是判断是否需要查询 CMDB，而非实际查询
		TaskSkill:           "cmdb_need_assessment_workflow",
		AutoLoadModuleSkill: false,
		ConditionalRules:    []models.SkillConditionalRule{},
	}
}

// buildCMDBNeedAssessmentPrompt 构建 CMDB 需求评估 Prompt（硬编码版本，作为降级方案）
func (s *AICMDBSkillService) buildCMDBNeedAssessmentPrompt(userDescription string) string {
	return fmt.Sprintf(`你是一个 IaC 平台的资源分析助手。请分析用户的需求描述，判断是否需要从 CMDB（配置管理数据库）查询现有资源。

【需要查询 CMDB 的情况】
1. 用户提到要引用、关联、绑定、连接现有资源
2. 用户提到特定的资源名称、ID、ARN
3. 用户提到权限策略需要允许/拒绝特定服务或角色访问
4. 用户提到要使用现有的 VPC、子网、安全组等网络资源
5. 用户提到要使用现有的 IAM 角色、策略
6. 用户提到 "来自 cmdb"、"现有的"、"已有的" 等表达
7. 用户提到需要查找或匹配某类资源

【不需要查询 CMDB 的情况】
1. 用户只是创建全新的资源，不引用任何现有资源
2. 用户的需求完全自包含，不依赖外部资源

【用户需求】
%s

【输出格式】
请返回 JSON 格式，不要有任何额外文字：
{
  "need_cmdb": true/false,
  "reason": "简短说明判断理由（不超过30字）",
  "resource_types": ["需要查询的资源类型列表，如 aws_iam_role, aws_vpc 等"]
}`, userDescription)
}

// parseCMDBNeedAssessment 解析 CMDB 需求评估结果
func (s *AICMDBSkillService) parseCMDBNeedAssessment(output string) (*CMDBNeedAssessment, error) {
	// 提取 JSON
	jsonStr := extractJSON(output)

	var result CMDBNeedAssessment
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		// 尝试修复不完整的 JSON
		fixedJSON := fixIncompleteJSON(jsonStr)
		if err2 := json.Unmarshal([]byte(fixedJSON), &result); err2 != nil {
			return nil, fmt.Errorf("无法解析 CMDB 需求评估结果: %w", err)
		}
	}

	return &result, nil
}

// performCMDBQuery 执行 CMDB 查询
func (s *AICMDBSkillService) performCMDBQuery(userID string, userDescription string, userSelections map[string]string) (*CMDBQueryResults, error) {
	// 使用现有的 CMDB 服务逻辑
	cmdbService := NewAICMDBService(s.db)

	// 生成查询计划
	queryPlan, err := cmdbService.parseQueryPlan(userDescription)
	if err != nil {
		return nil, err
	}

	// 执行查询
	results, err := cmdbService.executeCMDBQueries(userID, queryPlan)
	if err != nil {
		return nil, err
	}

	// 应用用户选择
	if len(userSelections) > 0 {
		cmdbService.applyUserSelections(results, userSelections)
	}

	return results, nil
}

// checkNeedSelection 检查是否需要用户选择
func (s *AICMDBSkillService) checkNeedSelection(results *CMDBQueryResults) (bool, []CMDBLookupResult) {
	cmdbService := NewAICMDBService(s.db)
	return cmdbService.checkNeedSelection(results)
}

// buildCMDBDataFromSelections 从用户选择构建 CMDB 数据字符串
// 会根据资源 ID 查询完整的资源信息（包括 ARN）
func (s *AICMDBSkillService) buildCMDBDataFromSelections(selections map[string]string) string {
	if len(selections) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("【用户选择的资源 - 请直接使用以下资源信息】\n")

	for key, value := range selections {
		// 检查是否是多选（逗号分隔）
		if strings.Contains(value, ",") {
			ids := strings.Split(value, ",")
			var arnList []string
			for _, id := range ids {
				// 查询资源的完整信息
				resource := s.lookupResourceByID(id)
				if resource != nil && resource.ARN != "" {
					arnList = append(arnList, resource.ARN)
				} else {
					arnList = append(arnList, id) // 降级：使用 ID
				}
			}
			sb.WriteString(fmt.Sprintf("- %s (多个):\n", key))
			for i, arn := range arnList {
				sb.WriteString(fmt.Sprintf("  - [%d] ARN: %s\n", i+1, arn))
			}
		} else {
			// 查询资源的完整信息
			resource := s.lookupResourceByID(value)
			if resource != nil {
				sb.WriteString(fmt.Sprintf("- %s:\n", key))
				sb.WriteString(fmt.Sprintf("  - ID: %s\n", resource.ID))
				sb.WriteString(fmt.Sprintf("  - Name: %s\n", resource.Name))
				if resource.ARN != "" {
					sb.WriteString(fmt.Sprintf("  - ARN: %s\n", resource.ARN))
				}
			} else {
				// 降级：只输出 ID
				sb.WriteString(fmt.Sprintf("- %s: %s\n", key, value))
			}
		}
	}

	sb.WriteString("\n【重要】请在生成配置时直接使用上述 ARN，不要使用占位符！\n")

	return sb.String()
}

// lookupResourceByID 根据资源 ID 查询完整的资源信息
func (s *AICMDBSkillService) lookupResourceByID(resourceID string) *CMDBResourceInfo {
	// 从 resource_index 表查询
	var resource struct {
		CloudResourceID   string `gorm:"column:cloud_resource_id"`
		CloudResourceName string `gorm:"column:cloud_resource_name"`
		CloudResourceARN  string `gorm:"column:cloud_resource_arn"`
		ResourceType      string `gorm:"column:resource_type"`
		CloudRegion       string `gorm:"column:cloud_region"`
	}

	err := s.db.Table("resource_index").
		Where("cloud_resource_id = ?", resourceID).
		First(&resource).Error

	if err != nil {
		// 尝试从 cmdb_external_sources 查询
		var externalResource struct {
			ResourceID   string `gorm:"column:resource_id"`
			ResourceName string `gorm:"column:resource_name"`
			ResourceARN  string `gorm:"column:resource_arn"`
			ResourceType string `gorm:"column:resource_type"`
		}

		err = s.db.Table("cmdb_external_sources").
			Where("resource_id = ?", resourceID).
			First(&externalResource).Error

		if err != nil {
			log.Printf("[AICMDBSkillService] 无法找到资源 ID=%s 的完整信息: %v", resourceID, err)
			return nil
		}

		return &CMDBResourceInfo{
			ID:   externalResource.ResourceID,
			Name: externalResource.ResourceName,
			ARN:  externalResource.ResourceARN,
		}
	}

	return &CMDBResourceInfo{
		ID:     resource.CloudResourceID,
		Name:   resource.CloudResourceName,
		ARN:    resource.CloudResourceARN,
		Region: resource.CloudRegion,
	}
}

// buildCMDBDataFromResourceInfoMap 从前端传递的完整资源信息构建 CMDB 数据字符串
// 这个函数直接使用前端传递的资源信息（包括 ARN），不需要再次查询数据库
func (s *AICMDBSkillService) buildCMDBDataFromResourceInfoMap(resourceInfoMap map[string]interface{}) string {
	if len(resourceInfoMap) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("【用户选择的资源 - 请直接使用以下资源信息】\n")

	for key, value := range resourceInfoMap {
		// 处理单个资源
		if resourceMap, ok := value.(map[string]interface{}); ok {
			id, _ := resourceMap["id"].(string)
			name, _ := resourceMap["name"].(string)
			arn, _ := resourceMap["arn"].(string)

			sb.WriteString(fmt.Sprintf("- %s:\n", key))
			sb.WriteString(fmt.Sprintf("  - ID: %s\n", id))
			sb.WriteString(fmt.Sprintf("  - Name: %s\n", name))
			if arn != "" {
				sb.WriteString(fmt.Sprintf("  - ARN: %s\n", arn))
			}
		} else if resourceList, ok := value.([]interface{}); ok {
			// 处理多个资源（数组）
			sb.WriteString(fmt.Sprintf("- %s (多个):\n", key))
			for i, item := range resourceList {
				if resourceMap, ok := item.(map[string]interface{}); ok {
					id, _ := resourceMap["id"].(string)
					name, _ := resourceMap["name"].(string)
					arn, _ := resourceMap["arn"].(string)

					sb.WriteString(fmt.Sprintf("  - [%d] ID: %s, Name: %s", i+1, id, name))
					if arn != "" {
						sb.WriteString(fmt.Sprintf(", ARN: %s", arn))
					}
					sb.WriteString("\n")
				}
			}
		}
	}

	sb.WriteString("\n【重要】请在生成配置时直接使用上述 ARN，不要使用占位符！\n")

	log.Printf("[AICMDBSkillService] 从前端资源信息构建 CMDB 数据: %s", sb.String())

	return sb.String()
}

// buildCMDBDataString 构建 CMDB 数据字符串
func (s *AICMDBSkillService) buildCMDBDataString(results *CMDBQueryResults) string {
	if results == nil || len(results.Results) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("【CMDB 查询结果 - 请直接使用以下资源 ID】\n")

	for key, result := range results.Results {
		if result.Found && result.Resource != nil {
			sb.WriteString(fmt.Sprintf("- %s: %s (名称: %s)\n",
				key, result.Resource.ID, result.Resource.Name))
		}
	}

	return sb.String()
}

// getSchemaData 获取 Schema 数据
func (s *AICMDBSkillService) getSchemaData(moduleID uint) string {
	var schema models.Schema
	if err := s.db.Where("module_id = ? AND status = ?", moduleID, "active").First(&schema).Error; err != nil {
		return ""
	}

	// 使用 ModuleSkillGenerator 提取约束
	generator := NewModuleSkillGenerator(s.db)
	return generator.ExtractSchemaConstraints(schema.OpenAPISchema)
}

// parseAIResponse 解析 AI 响应
func (s *AICMDBSkillService) parseAIResponse(aiResult string, moduleID uint) (*GenerateConfigWithCMDBResponse, error) {
	// 记录原始响应的前 500 个字符用于调试
	previewLen := 500
	if len(aiResult) < previewLen {
		previewLen = len(aiResult)
	}
	log.Printf("[AICMDBSkillService] AI 原始响应预览 (前 %d 字符): %s", previewLen, aiResult[:previewLen])

	// 提取 JSON
	log.Printf("[AICMDBSkillService] 调用 extractJSON (版本: 2026-01-31-v3)")
	jsonStr := extractJSON(aiResult)
	if jsonStr == "" {
		log.Printf("[AICMDBSkillService] 无法从 AI 响应中提取 JSON，原始响应长度: %d", len(aiResult))
		return &GenerateConfigWithCMDBResponse{
			Status:  "error",
			Message: "无法从 AI 响应中提取 JSON",
		}, nil
	}

	// 记录提取后的 JSON 预览
	jsonPreviewLen := 500
	if len(jsonStr) < jsonPreviewLen {
		jsonPreviewLen = len(jsonStr)
	}
	log.Printf("[AICMDBSkillService] 提取的 JSON 预览 (前 %d 字符): %s", jsonPreviewLen, jsonStr[:jsonPreviewLen])

	// 解析 JSON
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		log.Printf("[AICMDBSkillService] JSON 解析失败: %v", err)
		log.Printf("[AICMDBSkillService] 完整的 JSON 字符串: %s", jsonStr)
		return &GenerateConfigWithCMDBResponse{
			Status:  "error",
			Message: fmt.Sprintf("解析 AI 响应失败: %v", err),
		}, nil
	}

	response := &GenerateConfigWithCMDBResponse{
		Status:  "complete",
		Message: "配置生成成功",
	}

	// 提取状态
	if status, ok := result["status"].(string); ok {
		response.Status = status
	}

	// 提取配置
	if config, ok := result["config"].(map[string]interface{}); ok {
		response.Config = config
	}

	// 提取消息
	if message, ok := result["message"].(string); ok {
		response.Message = message
	}

	return response, nil
}

// fallbackToLegacyMode 降级到传统模式
func (s *AICMDBSkillService) fallbackToLegacyMode(
	userID string,
	moduleID uint,
	userDescription string,
	workspaceID string,
	organizationID string,
	userSelections map[string]string,
	currentConfig map[string]interface{},
	mode string,
) (*GenerateConfigWithCMDBResponse, error) {
	log.Printf("[AICMDBSkillService] 降级到传统 CMDB 模式")
	cmdbService := NewAICMDBService(s.db)
	return cmdbService.GenerateConfigWithCMDB(
		userID, moduleID, userDescription, workspaceID, organizationID,
		userSelections, currentConfig, mode,
	)
}

// ========== Skill 管理方法 ==========

// GetSkillCompositionForCapability 获取指定能力的 Skill 组合配置
func (s *AICMDBSkillService) GetSkillCompositionForCapability(capability string) (*models.SkillComposition, error) {
	aiConfig, err := s.configService.GetConfigForCapability(capability)
	if err != nil {
		return nil, err
	}

	composition := &aiConfig.SkillComposition
	if aiConfig.Mode != "skill" || (len(composition.FoundationSkills) == 0 && composition.TaskSkill == "") {
		return s.getDefaultSkillComposition(), nil
	}

	return composition, nil
}

// PreviewAssembledPrompt 预览组装后的 Prompt（用于调试）
func (s *AICMDBSkillService) PreviewAssembledPrompt(
	capability string,
	moduleID uint,
	userDescription string,
) (string, []string, error) {
	composition, err := s.GetSkillCompositionForCapability(capability)
	if err != nil {
		return "", nil, err
	}

	dynamicContext := &DynamicContext{
		UserDescription: userDescription,
		ModuleID:        moduleID,
		UseCMDB:         s.shouldUseCMDB(userDescription),
		SchemaData:      s.getSchemaData(moduleID),
	}

	result, err := s.skillAssembler.AssemblePrompt(composition, moduleID, dynamicContext)
	if err != nil {
		return "", nil, err
	}

	return result.Prompt, result.UsedSkillNames, nil
}

// ========== 优化方法：AI 智能选择 Domain Skills ==========

// getAllDomainSkillDescriptions 获取所有 Domain Skill 的描述
// 用于 AI 智能选择 Domain Skills
func (s *AICMDBSkillService) getAllDomainSkillDescriptions() ([]DomainSkillInfo, error) {
	var skills []models.Skill
	err := s.db.Where("layer = ? AND is_active = ?", models.SkillLayerDomain, true).
		Select("name", "description").
		Order("priority ASC").
		Find(&skills).Error
	if err != nil {
		return nil, err
	}

	result := make([]DomainSkillInfo, len(skills))
	for i, skill := range skills {
		result[i] = DomainSkillInfo{
			Name:        skill.Name,
			Description: skill.Description,
		}
	}

	return result, nil
}

// getAllDomainSkillNames 获取所有 Domain Skill 名称
func (s *AICMDBSkillService) getAllDomainSkillNames() []string {
	var names []string
	s.db.Model(&models.Skill{}).
		Where("layer = ? AND is_active = ?", models.SkillLayerDomain, true).
		Pluck("name", &names)
	return names
}

// validateSelectedSkills 验证 AI 选择的 Skills 是否存在
func (s *AICMDBSkillService) validateSelectedSkills(selected []string) []string {
	if len(selected) == 0 {
		return selected
	}

	// 获取所有有效的 Domain Skill 名称
	validNames := s.getAllDomainSkillNames()
	validNameSet := make(map[string]bool)
	for _, name := range validNames {
		validNameSet[name] = true
	}

	// 过滤无效的 Skill 名称
	validSkills := make([]string, 0, len(selected))
	for _, name := range selected {
		if validNameSet[name] {
			validSkills = append(validSkills, name)
		} else {
			log.Printf("[AICMDBSkillService] AI 选择了不存在的 Skill: %s，已忽略", name)
		}
	}

	return validSkills
}

// selectDomainSkillsByAI AI 智能选择 Domain Skills
func (s *AICMDBSkillService) selectDomainSkillsByAI(userDescription string) ([]string, error) {
	totalTimer := NewTimer()

	// 1. 获取所有 Domain Skill 描述
	skillInfos, err := s.getAllDomainSkillDescriptions()
	if err != nil {
		RecordDomainSkillSelection(0, "error", totalTimer.ElapsedMs())
		return nil, fmt.Errorf("获取 Domain Skill 描述失败: %w", err)
	}

	if len(skillInfos) == 0 {
		log.Printf("[AICMDBSkillService] 没有可用的 Domain Skills")
		RecordDomainSkillSelection(0, "empty", totalTimer.ElapsedMs())
		return []string{}, nil
	}

	// 2. 获取 AI 配置
	configTimer := NewTimer()
	aiConfig, err := s.configService.GetConfigForCapability("domain_skill_selection")
	RecordAICallDuration("domain_skill_selection", "get_config", configTimer.ElapsedMs())
	if err != nil || aiConfig == nil {
		log.Printf("[AICMDBSkillService] domain_skill_selection AI 配置不可用，降级到标签匹配: %v", err)
		RecordDomainSkillSelection(0, "config_error", totalTimer.ElapsedMs())
		return nil, fmt.Errorf("AI 配置不可用")
	}

	// 3. 构建 Prompt（支持自定义 Prompt）
	promptTimer := NewTimer()
	prompt := s.buildDomainSkillSelectionPromptWithConfig(aiConfig, userDescription, skillInfos)
	RecordAICallDuration("domain_skill_selection", "build_prompt", promptTimer.ElapsedMs())

	// 4. 调用 AI
	aiTimer := NewTimer()
	result, err := s.aiFormService.callAI(aiConfig, prompt)
	RecordAICallDuration("domain_skill_selection", "ai_call", aiTimer.ElapsedMs())
	log.Printf("[AICMDBSkillService] [耗时] Domain Skill 选择 AI 调用: %.0fms", aiTimer.ElapsedMs())
	if err != nil {
		RecordDomainSkillSelection(0, "ai_error", totalTimer.ElapsedMs())
		return nil, fmt.Errorf("AI 调用失败: %w", err)
	}

	// 打印 AI 原始返回数据
	log.Printf("[AICMDBSkillService] Domain Skill 选择 AI 原始返回: %s", result)

	// 5. 解析结果
	parseTimer := NewTimer()
	selection, err := s.parseDomainSkillSelection(result)
	RecordAICallDuration("domain_skill_selection", "parse_response", parseTimer.ElapsedMs())
	if err != nil {
		RecordDomainSkillSelection(0, "parse_error", totalTimer.ElapsedMs())
		return nil, fmt.Errorf("解析 AI 选择结果失败: %w", err)
	}

	// 6. 验证选择的 Skills
	validationTimer := NewTimer()
	validSkills := s.validateSelectedSkills(selection.SelectedSkills)
	RecordAICallDuration("domain_skill_selection", "validation", validationTimer.ElapsedMs())

	// 7. 记录选择结果
	RecordDomainSkillSelection(len(validSkills), "ai", totalTimer.ElapsedMs())
	RecordAICallDuration("domain_skill_selection", "total", totalTimer.ElapsedMs())
	log.Printf("[AICMDBSkillService] [耗时] Domain Skill 选择总计: %.0fms", totalTimer.ElapsedMs())
	log.Printf("[AICMDBSkillService] AI 选择了 %d 个 Domain Skills: %v (原因: %s)",
		len(validSkills), validSkills, selection.Reason)

	return validSkills, nil
}

// buildDomainSkillSelectionPromptWithConfig 构建 Domain Skill 选择 Prompt（支持自定义 Prompt）
// 自定义 Prompt 会自动追加用户需求和 Skills 列表
func (s *AICMDBSkillService) buildDomainSkillSelectionPromptWithConfig(aiConfig *models.AIConfig, userDescription string, skillInfos []DomainSkillInfo) string {
	// 构建 Skill 列表字符串（从数据库动态查询）
	var skillListBuilder strings.Builder
	for i, info := range skillInfos {
		desc := info.Description
		if desc == "" {
			desc = "(无描述)"
		}
		skillListBuilder.WriteString(fmt.Sprintf("%d. %s - %s\n", i+1, info.Name, desc))
	}
	skillList := skillListBuilder.String()

	log.Printf("[AICMDBSkillService] 动态构建 domain_skill_selection Prompt，包含 %d 个可用 Skills", len(skillInfos))

	// 检查是否有自定义 Prompt
	if aiConfig.CapabilityPrompts != nil {
		if customPrompt, ok := aiConfig.CapabilityPrompts["domain_skill_selection"]; ok && customPrompt != "" {
			log.Printf("[AICMDBSkillService] 使用自定义 domain_skill_selection Prompt")

			// 如果自定义 Prompt 包含占位符，替换它们
			if strings.Contains(customPrompt, "{skill_list}") || strings.Contains(customPrompt, "{user_description}") {
				prompt := strings.ReplaceAll(customPrompt, "{user_description}", userDescription)
				prompt = strings.ReplaceAll(prompt, "{skill_list}", skillList)
				return prompt
			}

			// 如果没有占位符，自动追加用户需求和 Skills 列表
			var sb strings.Builder
			sb.WriteString(customPrompt)
			sb.WriteString("\n\n")
			sb.WriteString("【用户需求】\n")
			sb.WriteString(userDescription)
			sb.WriteString("\n\n")
			sb.WriteString("【可用的 Domain Skills】\n")
			sb.WriteString(skillList)
			return sb.String()
		}
	}

	// 检查 custom_prompt 字段
	if aiConfig.CustomPrompt != "" {
		log.Printf("[AICMDBSkillService] 使用 custom_prompt 字段的 domain_skill_selection Prompt")

		if strings.Contains(aiConfig.CustomPrompt, "{skill_list}") || strings.Contains(aiConfig.CustomPrompt, "{user_description}") {
			prompt := strings.ReplaceAll(aiConfig.CustomPrompt, "{user_description}", userDescription)
			prompt = strings.ReplaceAll(prompt, "{skill_list}", skillList)
			return prompt
		}

		// 自动追加
		var sb strings.Builder
		sb.WriteString(aiConfig.CustomPrompt)
		sb.WriteString("\n\n")
		sb.WriteString("【用户需求】\n")
		sb.WriteString(userDescription)
		sb.WriteString("\n\n")
		sb.WriteString("【可用的 Domain Skills】\n")
		sb.WriteString(skillList)
		return sb.String()
	}

	// 使用默认 Prompt
	return s.buildDomainSkillSelectionPrompt(userDescription, skillInfos)
}

// buildDomainSkillSelectionPrompt 构建 Domain Skill 选择 Prompt（硬编码版本）
func (s *AICMDBSkillService) buildDomainSkillSelectionPrompt(userDescription string, skillInfos []DomainSkillInfo) string {
	var sb strings.Builder

	sb.WriteString("你是一个 IaC 平台的 Skill 选择助手。请根据用户需求，从可用的 Domain Skills 中选择需要的 Skills。\n\n")

	sb.WriteString("【用户需求】\n")
	sb.WriteString(userDescription)
	sb.WriteString("\n\n")

	sb.WriteString("【可用的 Domain Skills】\n")
	for i, info := range skillInfos {
		desc := info.Description
		if desc == "" {
			desc = "(无描述)"
		}
		sb.WriteString(fmt.Sprintf("%d. %s - %s\n", i+1, info.Name, desc))
	}
	sb.WriteString("\n")

	sb.WriteString("【选择规则】\n")
	sb.WriteString("1. 只选择与用户需求直接相关的 Skills\n")
	sb.WriteString("2. 通常选择 2-5 个 Skills 即可，不要贪多\n")
	sb.WriteString("3. 如果用户需求涉及 CMDB 资源引用，选择 cmdb_resource_matching\n")
	sb.WriteString("4. 如果用户需求涉及 AWS 策略（IAM/S3/KMS 等），选择对应的策略 Skill\n")
	sb.WriteString("5. 如果用户需求涉及资源标签，选择 aws_resource_tagging\n")
	sb.WriteString("6. 如果用户需求涉及区域选择，选择 region_mapping\n")
	sb.WriteString("\n")

	sb.WriteString("【输出格式】\n")
	sb.WriteString("请返回 JSON 格式，不要有任何额外文字：\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"selected_skills\": [\"skill_name_1\", \"skill_name_2\"],\n")
	sb.WriteString("  \"reason\": \"简短说明选择理由\"\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n")

	return sb.String()
}

// parseDomainSkillSelection 解析 Domain Skill 选择结果
func (s *AICMDBSkillService) parseDomainSkillSelection(output string) (*DomainSkillSelectionResult, error) {
	// 提取 JSON
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		return nil, fmt.Errorf("无法从 AI 响应中提取 JSON")
	}

	var result DomainSkillSelectionResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		// 尝试修复不完整的 JSON
		fixedJSON := fixIncompleteJSON(jsonStr)
		if err2 := json.Unmarshal([]byte(fixedJSON), &result); err2 != nil {
			return nil, fmt.Errorf("无法解析 Domain Skill 选择结果: %w", err)
		}
	}

	return &result, nil
}

// ========== 优化方法：CMDB 判断与查询合并 ==========

// assessCMDBWithQueryPlan AI 判断是否需要 CMDB 并同时生成查询计划
// 合并原来的两次 AI 调用为一次
func (s *AICMDBSkillService) assessCMDBWithQueryPlan(userDescription string) (*CMDBAssessmentWithQueryPlan, error) {
	// 获取 AI 配置
	aiConfig, err := s.configService.GetConfigForCapability("cmdb_need_assessment")
	if err != nil || aiConfig == nil {
		return nil, fmt.Errorf("cmdb_need_assessment AI 配置不可用: %v", err)
	}

	// 构建 Prompt（使用更新后的 Skill，包含 query_plan）
	prompt := s.buildCMDBAssessmentWithQueryPlanPrompt(aiConfig, userDescription)

	// 调用 AI
	result, err := s.aiFormService.callAI(aiConfig, prompt)
	if err != nil {
		return nil, fmt.Errorf("AI 调用失败: %w", err)
	}

	// 解析结果
	assessment, err := s.parseCMDBAssessmentWithQueryPlan(result)
	if err != nil {
		return nil, fmt.Errorf("解析 CMDB 评估结果失败: %w", err)
	}

	return assessment, nil
}

// buildCMDBAssessmentWithQueryPlanPrompt 构建 CMDB 评估 Prompt（包含查询计划）
func (s *AICMDBSkillService) buildCMDBAssessmentWithQueryPlanPrompt(aiConfig *models.AIConfig, userDescription string) string {
	// 检查是否使用 Skill 模式
	if aiConfig.Mode == "skill" {
		composition := s.getDefaultCMDBNeedAssessmentSkillComposition()
		dynamicContext := &DynamicContext{
			UserDescription: userDescription,
			ExtraContext: map[string]interface{}{
				"capability":   "cmdb_need_assessment",
				"user_request": userDescription,
			},
		}

		result, err := s.skillAssembler.AssemblePrompt(composition, 0, dynamicContext)
		if err == nil {
			log.Printf("[AICMDBSkillService] CMDB 评估（含查询计划）使用了 %d 个 Skills: %v",
				len(result.UsedSkillNames), result.UsedSkillNames)
			return result.Prompt
		}
		log.Printf("[AICMDBSkillService] Skill 组装失败: %v，降级到硬编码 Prompt", err)
	}

	// 降级到硬编码 Prompt
	return s.buildCMDBAssessmentWithQueryPlanPromptHardcoded(userDescription)
}

// buildCMDBAssessmentWithQueryPlanPromptHardcoded 硬编码版本的 CMDB 评估 Prompt
func (s *AICMDBSkillService) buildCMDBAssessmentWithQueryPlanPromptHardcoded(userDescription string) string {
	return fmt.Sprintf(`你是一个 IaC 平台的资源分析助手。请分析用户的需求描述，判断是否需要从 CMDB 查询现有资源，并生成查询计划。

【需要查询 CMDB 的情况】
1. 用户提到要引用、关联、绑定、连接现有资源
2. 用户提到特定的资源名称、ID、ARN
3. 用户提到权限策略需要允许/拒绝特定服务或角色访问
4. 用户提到要使用现有的 VPC、子网、安全组等网络资源
5. 用户提到要使用现有的 IAM 角色、策略
6. 用户提到 "来自 cmdb"、"现有的"、"已有的" 等表达

【用户需求】
%s

【输出格式】
请返回 JSON 格式，不要有任何额外文字：
{
  "need_cmdb": true/false,
  "reason": "简短说明判断理由（不超过30字）",
  "resource_types": ["需要查询的资源类型列表"],
  "query_plan": [
    {
      "resource_type": "资源类型，如 aws_iam_role",
      "filters": {
        "name_contains": "名称包含的关键词（可选）",
        "tags": {"标签键": "标签值"}
      },
      "limit": 10
    }
  ]
}`, userDescription)
}

// parseCMDBAssessmentWithQueryPlan 解析 CMDB 评估结果（包含查询计划）
func (s *AICMDBSkillService) parseCMDBAssessmentWithQueryPlan(output string) (*CMDBAssessmentWithQueryPlan, error) {
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		log.Printf("[AICMDBSkillService] CMDB 评估: 无法从 AI 响应中提取 JSON，原始响应: %s", output[:min(len(output), 500)])
		return nil, fmt.Errorf("无法从 AI 响应中提取 JSON")
	}

	// 记录提取的 JSON
	log.Printf("[AICMDBSkillService] CMDB 评估: 提取的 JSON: %s", jsonStr[:min(len(jsonStr), 500)])

	var result CMDBAssessmentWithQueryPlan
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		fixedJSON := fixIncompleteJSON(jsonStr)
		if err2 := json.Unmarshal([]byte(fixedJSON), &result); err2 != nil {
			log.Printf("[AICMDBSkillService] CMDB 评估: JSON 解析失败: %v", err)
			return nil, fmt.Errorf("无法解析 CMDB 评估结果: %w", err)
		}
	}

	// 记录解析结果
	log.Printf("[AICMDBSkillService] CMDB 评估结果: need_cmdb=%v, reason=%s, resource_types=%v, query_plan_count=%d",
		result.NeedCMDB, result.Reason, result.ResourceTypes, len(result.QueryPlan))

	// 如果 query_plan 为空但 resource_types 不为空，自动生成查询计划
	if result.NeedCMDB && len(result.QueryPlan) == 0 && len(result.ResourceTypes) > 0 {
		log.Printf("[AICMDBSkillService] AI 未返回 query_plan，根据 resource_types 自动生成查询计划")
		result.QueryPlan = make([]CMDBQueryPlanItem, len(result.ResourceTypes))
		for i, resourceType := range result.ResourceTypes {
			result.QueryPlan[i] = CMDBQueryPlanItem{
				ResourceType: resourceType,
				Filters:      nil, // 不设置过滤条件，查询所有该类型的资源
				Limit:        10,
			}
		}
		log.Printf("[AICMDBSkillService] 自动生成了 %d 个查询计划项", len(result.QueryPlan))
	}

	return &result, nil
}

// assessAndQueryCMDB 合并 CMDB 判断和查询（优化版）
// 返回: CMDB 查询结果, 是否需要用户选择, 选择列表, 错误
func (s *AICMDBSkillService) assessAndQueryCMDB(
	userID string,
	userDescription string,
) (*CMDBQueryResults, bool, []CMDBLookupResult, error) {
	totalTimer := NewTimer()

	// 1. 关键词快速检测
	keywordTimer := NewTimer()
	keywordMatch := s.shouldUseCMDBByKeywords(userDescription)
	RecordAICallDuration("form_generation_optimized", "cmdb_keyword_check", keywordTimer.ElapsedMs())
	log.Printf("[AICMDBSkillService] [耗时] CMDB 关键词检测: %.0fms, 匹配: %v", keywordTimer.ElapsedMs(), keywordMatch)

	if !keywordMatch {
		log.Printf("[AICMDBSkillService] 关键词检测: 不需要 CMDB")
		RecordCMDBAssessment(false, 0, "keyword", keywordTimer.ElapsedMs())
		return nil, false, nil, nil
	}

	// 2. AI 判断 + 生成查询计划（合并为一次调用）
	assessmentTimer := NewTimer()
	assessment, err := s.assessCMDBWithQueryPlan(userDescription)
	assessmentDuration := assessmentTimer.ElapsedMs()
	RecordAICallDuration("form_generation_optimized", "cmdb_ai_assessment", assessmentDuration)
	log.Printf("[AICMDBSkillService] [耗时] CMDB AI 评估: %.0fms", assessmentDuration)

	if err != nil {
		log.Printf("[AICMDBSkillService] CMDB 评估失败: %v，继续执行（不使用 CMDB）", err)
		RecordCMDBAssessment(false, 0, "ai_error", assessmentDuration)
		return nil, false, nil, nil
	}

	if !assessment.NeedCMDB {
		log.Printf("[AICMDBSkillService] AI 判断: 不需要 CMDB, 原因: %s", assessment.Reason)
		RecordCMDBAssessment(false, 0, "ai", assessmentDuration)
		return nil, false, nil, nil
	}

	// 记录 CMDB 评估结果
	RecordCMDBAssessment(true, len(assessment.ResourceTypes), "ai", assessmentDuration)
	log.Printf("[AICMDBSkillService] AI 判断: 需要 CMDB, 原因: %s, 资源类型: %v",
		assessment.Reason, assessment.ResourceTypes)

	// 3. 执行 CMDB 查询
	queryTimer := NewTimer()
	results, err := s.executeCMDBQueriesFromPlan(userID, assessment.QueryPlan)
	queryDuration := queryTimer.ElapsedMs()
	RecordAICallDuration("form_generation_optimized", "cmdb_query_execution", queryDuration)
	log.Printf("[AICMDBSkillService] [耗时] CMDB 查询执行: %.0fms", queryDuration)

	if err != nil {
		log.Printf("[AICMDBSkillService] CMDB 查询失败: %v，继续执行（不使用 CMDB）", err)
		return nil, false, nil, nil
	}

	// 4. 检查是否需要用户选择，并记录每个资源类型的查询结果
	needSelection, lookups := s.checkNeedSelection(results)

	// 记录每个资源类型的查询结果
	for key, result := range results.Results {
		candidateCount := 0
		if result.Candidates != nil {
			candidateCount = len(result.Candidates)
		}
		IncCMDBQueryCount(key, result.Found, candidateCount)
	}

	// 记录 CMDB 总耗时
	RecordAICallDuration("form_generation_optimized", "cmdb_total", totalTimer.ElapsedMs())
	log.Printf("[AICMDBSkillService] [耗时] CMDB 评估+查询总计: %.0fms", totalTimer.ElapsedMs())

	return results, needSelection, lookups, nil
}

// executeCMDBQueriesFromPlan 根据查询计划执行 CMDB 查询
// 使用 AICMDBService 的向量搜索功能，而非简单的关键词搜索
func (s *AICMDBSkillService) executeCMDBQueriesFromPlan(userID string, queryPlan []CMDBQueryPlanItem) (*CMDBQueryResults, error) {
	if len(queryPlan) == 0 {
		log.Printf("[AICMDBSkillService] 查询计划为空，跳过 CMDB 查询")
		return &CMDBQueryResults{Results: make(map[string]*CMDBQueryResult)}, nil
	}

	log.Printf("[AICMDBSkillService] 执行 CMDB 查询，查询计划包含 %d 个项目", len(queryPlan))

	// 转换查询计划格式：CMDBQueryPlanItem -> CMDBQuery
	cmdbQueryPlan := &CMDBQueryPlan{
		Queries: make([]CMDBQuery, len(queryPlan)),
	}

	for i, item := range queryPlan {
		// 构建搜索关键词
		keyword := ""
		if item.Filters != nil {
			if nameContains, ok := item.Filters["name_contains"].(string); ok {
				keyword = nameContains
			}
		}
		// 如果没有 name_contains，使用 "*" 表示查询所有该类型的资源
		if keyword == "" {
			keyword = "*"
		}

		// 使用 TargetField 区分同类型的多个资源（如 ec2_role 和 lambda_role）
		// 如果 AI 没有指定 target_field，保持为空，让 executeCMDBQueries 使用序号逻辑
		cmdbQueryPlan.Queries[i] = CMDBQuery{
			Type:        item.ResourceType,
			Keyword:     keyword,
			TargetField: item.TargetField, // 直接使用 AI 返回的值，可能为空
		}

		log.Printf("[AICMDBSkillService] 查询项 %d: 类型=%s, 关键词=%s, target_field=%s", i+1, item.ResourceType, keyword, item.TargetField)
	}

	// 使用 AICMDBService 执行查询（支持向量搜索）
	cmdbService := NewAICMDBService(s.db)
	results, err := cmdbService.executeCMDBQueries(userID, cmdbQueryPlan)
	if err != nil {
		log.Printf("[AICMDBSkillService] CMDB 查询失败: %v", err)
		return nil, err
	}

	// 统计结果
	foundCount := 0
	for key, result := range results.Results {
		if result.Found {
			foundCount++
			if result.Resource != nil {
				log.Printf("[AICMDBSkillService] 查询结果: %s -> %s (%s)", key, result.Resource.ID, result.Resource.Name)
			} else if len(result.Candidates) > 0 {
				log.Printf("[AICMDBSkillService] 查询结果: %s -> %d 个候选", key, len(result.Candidates))
			}
		} else {
			log.Printf("[AICMDBSkillService] 查询结果: %s -> 未找到", key)
		}
	}
	log.Printf("[AICMDBSkillService] CMDB 查询完成: %d/%d 个资源类型找到结果", foundCount, len(queryPlan))

	return results, nil
}

// ========== 优化版本：并行执行 ==========

// GenerateConfigWithCMDBSkillOptimized 使用 Skill 模式生成配置（优化版：并行执行）
func (s *AICMDBSkillService) GenerateConfigWithCMDBSkillOptimized(
	userID string,
	moduleID uint,
	userDescription string,
	workspaceID string,
	organizationID string,
	userSelections map[string]interface{},
	currentConfig map[string]interface{},
	mode string,
	resourceInfoMap map[string]interface{}, // 完整的资源信息（包括 ARN）
) (*GenerateConfigWithCMDBResponse, error) {
	totalTimer := NewTimer()
	log.Printf("[AICMDBSkillService] ========== 开始优化版 Skill 模式配置生成 ==========")
	log.Printf("[AICMDBSkillService] 用户 ID: %s, Module ID: %d, 模式: %s", userID, moduleID, mode)

	// 1. 获取 AI 配置
	configTimer := NewTimer()
	aiConfig, err := s.configService.GetConfigForCapability("form_generation")
	if err != nil || aiConfig == nil {
		IncAICallCount("form_generation_optimized", "config_error")
		return nil, fmt.Errorf("未找到 form_generation 的 AI 配置: %v", err)
	}
	RecordAICallDuration("form_generation_optimized", "get_config", configTimer.ElapsedMs())
	log.Printf("[AICMDBSkillService] [耗时] 获取 AI 配置: %.0fms", configTimer.ElapsedMs())

	// 转换 userSelections
	convertedSelections := s.convertUserSelections(userSelections)

	// 2. 检查配置模式
	if aiConfig.Mode != "skill" {
		log.Printf("[AICMDBSkillService] AI 配置模式为 '%s'，降级到传统模式", aiConfig.Mode)
		IncAICallCount("form_generation_optimized", "fallback_legacy")
		return s.fallbackToLegacyMode(userID, moduleID, userDescription, workspaceID, organizationID, convertedSelections, currentConfig, mode)
	}

	// 3. 意图断言
	assertionTimer := NewTimer()
	log.Printf("[AICMDBSkillService] 步骤 1: 意图断言检查")
	assertionResult, err := s.performIntentAssertion(userID, userDescription)
	RecordAICallDuration("form_generation_optimized", "intent_assertion", assertionTimer.ElapsedMs())
	log.Printf("[AICMDBSkillService] [耗时] 步骤 1 意图断言: %.0fms", assertionTimer.ElapsedMs())
	if err != nil {
		log.Printf("[AICMDBSkillService] 意图断言服务不可用: %v，继续执行", err)
	} else if assertionResult != nil && !assertionResult.IsSafe {
		IncAICallCount("form_generation_optimized", "blocked")
		RecordAICallDuration("form_generation_optimized", "total_blocked", totalTimer.ElapsedMs())
		return &GenerateConfigWithCMDBResponse{
			Status:  "blocked",
			Message: assertionResult.Suggestion,
		}, nil
	}

	// 4. 如果用户已选择资源，跳过并行执行
	if len(convertedSelections) > 0 {
		userSelectionTimer := NewTimer()
		log.Printf("[AICMDBSkillService] 步骤 2: 使用用户选择的资源（跳过 CMDB 查询和 Skill 选择）")
		log.Printf("[AICMDBSkillService] 用户选择: %v", convertedSelections)
		var cmdbData string
		// 优先使用前端传递的完整资源信息（包括 ARN）
		if len(resourceInfoMap) > 0 {
			cmdbData = s.buildCMDBDataFromResourceInfoMap(resourceInfoMap)
		} else {
			// 降级：从数据库查询
			cmdbData = s.buildCMDBDataFromSelections(convertedSelections)
		}
		RecordAICallDuration("form_generation_optimized", "user_selection_build", userSelectionTimer.ElapsedMs())
		log.Printf("[AICMDBSkillService] [耗时] 步骤 2 构建用户选择数据: %.0fms", userSelectionTimer.ElapsedMs())
		return s.generateWithCMDBDataAndSkills(
			userID, moduleID, userDescription, workspaceID, organizationID,
			aiConfig, cmdbData, nil, currentConfig, mode, totalTimer,
		)
	}

	// 5. 并行执行 CMDB 查询和 Skill 选择
	parallelTimer := NewTimer()
	log.Printf("[AICMDBSkillService] 步骤 2: 并行执行 CMDB 查询和 Skill 选择")
	SetActiveParallelTasks(2) // 设置活跃并行任务数
	parallelResult := s.executeParallel(userID, userDescription)
	SetActiveParallelTasks(0) // 重置活跃并行任务数
	RecordAICallDuration("form_generation_optimized", "parallel_execution", parallelTimer.ElapsedMs())
	log.Printf("[AICMDBSkillService] [耗时] 步骤 2 并行执行: %.0fms", parallelTimer.ElapsedMs())

	// 6. 处理 CMDB 错误（降级：继续执行，不使用 CMDB）
	var cmdbData string
	if parallelResult.CMDBError != nil {
		log.Printf("[AICMDBSkillService] CMDB 查询失败: %v，继续执行（不使用 CMDB）", parallelResult.CMDBError)
		IncAICallCount("form_generation_optimized", "cmdb_error")
	} else if parallelResult.NeedSelection {
		// 需要用户选择
		RecordAICallDuration("form_generation_optimized", "total_need_selection", totalTimer.ElapsedMs())
		IncAICallCount("form_generation_optimized", "need_selection")
		log.Printf("[AICMDBSkillService] [耗时] 总计（返回需要选择）: %.0fms", totalTimer.ElapsedMs())
		return &GenerateConfigWithCMDBResponse{
			Status:      "need_selection",
			CMDBLookups: parallelResult.CMDBLookups,
			Message:     "找到多个匹配的资源，请选择",
		}, nil
	} else if parallelResult.CMDBResults != nil {
		cmdbData = s.buildCMDBDataString(parallelResult.CMDBResults)
	}

	// 7. 处理 Skill 选择错误（降级：使用标签匹配）
	var selectedSkills []string
	if parallelResult.SkillError != nil {
		log.Printf("[AICMDBSkillService] Skill 选择失败: %v，降级到标签匹配", parallelResult.SkillError)
		IncAICallCount("form_generation_optimized", "skill_selection_error")
	} else {
		selectedSkills = parallelResult.SelectedSkills
		log.Printf("[AICMDBSkillService] AI 选择的 Domain Skills: %v", selectedSkills)
	}

	// 8. 生成配置
	return s.generateWithCMDBDataAndSkills(
		userID, moduleID, userDescription, workspaceID, organizationID,
		aiConfig, cmdbData, selectedSkills, currentConfig, mode, totalTimer,
	)
}

// executeParallel 并行执行 CMDB 查询和 Skill 选择
func (s *AICMDBSkillService) executeParallel(userID string, userDescription string) *ParallelExecutionResult {
	result := &ParallelExecutionResult{}

	// 使用 channel 实现并行
	cmdbDone := make(chan struct{})
	skillDone := make(chan struct{})

	// 记录各任务的耗时
	var cmdbDuration, skillDuration float64

	// 协程 1: CMDB 判断 + 查询
	go func() {
		defer close(cmdbDone)
		cmdbTimer := NewTimer()
		cmdbResults, needSelection, lookups, err := s.assessAndQueryCMDB(userID, userDescription)
		cmdbDuration = cmdbTimer.ElapsedMs()
		result.CMDBResults = cmdbResults
		result.NeedSelection = needSelection
		result.CMDBLookups = lookups
		result.CMDBError = err

		// 记录 CMDB 任务耗时
		status := "success"
		if err != nil {
			status = "error"
		}
		RecordParallelExecutionDuration("cmdb_query", status, cmdbDuration)
		log.Printf("[AICMDBSkillService] [并行] CMDB 任务完成: %.0fms, 状态: %s", cmdbDuration, status)
	}()

	// 协程 2: AI 选择 Domain Skills
	go func() {
		defer close(skillDone)
		skillTimer := NewTimer()
		selectedSkills, err := s.selectDomainSkillsByAI(userDescription)
		skillDuration = skillTimer.ElapsedMs()
		result.SelectedSkills = selectedSkills
		result.SkillError = err

		// 记录 Skill 选择任务耗时
		status := "success"
		if err != nil {
			status = "error"
		}
		RecordParallelExecutionDuration("skill_selection", status, skillDuration)
		log.Printf("[AICMDBSkillService] [并行] Skill 选择任务完成: %.0fms, 状态: %s", skillDuration, status)
	}()

	// 等待两个协程完成（带超时）
	timeout := time.After(ParallelExecutionTimeout)

	select {
	case <-cmdbDone:
		// CMDB 完成，等待 Skill
		select {
		case <-skillDone:
			// 两个都完成
			log.Printf("[AICMDBSkillService] [并行] 两个任务都已完成")
		case <-timeout:
			result.SkillError = fmt.Errorf("Skill 选择超时")
			RecordParallelExecutionDuration("skill_selection", "timeout", float64(ParallelExecutionTimeout.Milliseconds()))
			IncAICallCount("form_generation_optimized", "skill_timeout")
			log.Printf("[AICMDBSkillService] [并行] Skill 选择超时")
		}
	case <-skillDone:
		// Skill 完成，等待 CMDB
		select {
		case <-cmdbDone:
			// 两个都完成
			log.Printf("[AICMDBSkillService] [并行] 两个任务都已完成")
		case <-timeout:
			result.CMDBError = fmt.Errorf("CMDB 查询超时")
			RecordParallelExecutionDuration("cmdb_query", "timeout", float64(ParallelExecutionTimeout.Milliseconds()))
			IncAICallCount("form_generation_optimized", "cmdb_timeout")
			log.Printf("[AICMDBSkillService] [并行] CMDB 查询超时")
		}
	case <-timeout:
		// 总超时
		if result.CMDBResults == nil {
			result.CMDBError = fmt.Errorf("CMDB 查询超时")
			RecordParallelExecutionDuration("cmdb_query", "timeout", float64(ParallelExecutionTimeout.Milliseconds()))
			IncAICallCount("form_generation_optimized", "cmdb_timeout")
		}
		if result.SelectedSkills == nil {
			result.SkillError = fmt.Errorf("Skill 选择超时")
			RecordParallelExecutionDuration("skill_selection", "timeout", float64(ParallelExecutionTimeout.Milliseconds()))
			IncAICallCount("form_generation_optimized", "skill_timeout")
		}
		log.Printf("[AICMDBSkillService] [并行] 总超时")
	}

	return result
}

// generateWithCMDBDataAndSkills 使用 CMDB 数据和选中的 Skills 生成配置
func (s *AICMDBSkillService) generateWithCMDBDataAndSkills(
	userID string,
	moduleID uint,
	userDescription string,
	workspaceID string,
	organizationID string,
	aiConfig *models.AIConfig,
	cmdbData string,
	selectedSkills []string,
	currentConfig map[string]interface{},
	mode string,
	totalTimer *Timer, // 从调用方传入的总计时器
) (*GenerateConfigWithCMDBResponse, error) {
	log.Printf("[AICMDBSkillService] 步骤 3: 生成配置（使用 CMDB 数据和选中的 Skills）")

	// 1. 获取 Skill 组合配置
	composition := &aiConfig.SkillComposition
	if len(composition.FoundationSkills) == 0 && composition.TaskSkill == "" {
		composition = s.getDefaultSkillComposition()
	}

	// 2. 如果有 AI 选择的 Skills，覆盖 Domain Skills
	if len(selectedSkills) > 0 {
		composition.DomainSkills = selectedSkills
		composition.DomainSkillMode = models.DomainSkillModeFixed // 使用固定模式
		log.Printf("[AICMDBSkillService] 使用 AI 选择的 Domain Skills: %v", selectedSkills)
	}

	// 3. 获取 Schema 数据
	schemaTimer := NewTimer()
	schemaData := s.getSchemaData(moduleID)
	RecordAICallDuration("form_generation_optimized", "get_schema", schemaTimer.ElapsedMs())
	log.Printf("[AICMDBSkillService] [耗时] 获取 Schema 数据: %.0fms", schemaTimer.ElapsedMs())

	// 4. 构建动态上下文
	dynamicContext := &DynamicContext{
		UserDescription: userDescription,
		WorkspaceID:     workspaceID,
		OrganizationID:  organizationID,
		ModuleID:        moduleID,
		UseCMDB:         cmdbData != "",
		CurrentConfig:   currentConfig,
		CMDBData:        cmdbData,
		SchemaData:      schemaData,
		ExtraContext: map[string]interface{}{
			"mode": mode,
		},
	}

	// 5. 组装 Prompt
	assembleTimer := NewTimer()
	log.Printf("[AICMDBSkillService] 步骤 4: 组装 Skill Prompt")
	assembleResult, err := s.skillAssembler.AssemblePrompt(composition, moduleID, dynamicContext)
	if err != nil {
		IncAICallCount("form_generation_optimized", "skill_assembly_error")
		return nil, fmt.Errorf("Skill 组装失败: %w", err)
	}
	RecordSkillAssemblyDuration("form_generation_optimized", len(assembleResult.UsedSkillNames), assembleTimer.ElapsedMs())
	log.Printf("[AICMDBSkillService] [耗时] 步骤 4 组装 Skill Prompt: %.0fms", assembleTimer.ElapsedMs())
	log.Printf("[AICMDBSkillService] 使用了 %d 个 Skills: %v", len(assembleResult.UsedSkillNames), assembleResult.UsedSkillNames)

	// 6. 调用 AI 生成配置
	aiTimer := NewTimer()
	log.Printf("[AICMDBSkillService] 步骤 5: 调用 AI 生成配置")
	aiResult, err := s.aiFormService.callAI(aiConfig, assembleResult.Prompt)
	RecordAICallDuration("form_generation_optimized", "ai_call", aiTimer.ElapsedMs())
	log.Printf("[AICMDBSkillService] [耗时] 步骤 5 AI 调用: %.0fms", aiTimer.ElapsedMs())
	if err != nil {
		IncAICallCount("form_generation_optimized", "ai_error")
		return nil, fmt.Errorf("AI 调用失败: %w", err)
	}

	// 7. 解析 AI 响应
	parseTimer := NewTimer()
	response, err := s.parseAIResponse(aiResult, moduleID)
	RecordAICallDuration("form_generation_optimized", "parse_response", parseTimer.ElapsedMs())
	log.Printf("[AICMDBSkillService] [耗时] 解析 AI 响应: %.0fms", parseTimer.ElapsedMs())
	if err != nil {
		IncAICallCount("form_generation_optimized", "parse_error")
		return nil, fmt.Errorf("解析 AI 响应失败: %w", err)
	}

	// 8. 记录总耗时和成功计数
	executionTimeMs := int(totalTimer.ElapsedMs())
	RecordAICallDuration("form_generation_optimized", "total", totalTimer.ElapsedMs())
	IncAICallCount("form_generation_optimized", "success")

	// 9. 记录 Skill 使用日志
	if err := s.skillAssembler.LogSkillUsage(
		assembleResult.UsedSkillIDs,
		"form_generation",
		workspaceID,
		userID,
		&moduleID,
		aiConfig.ModelID,
		executionTimeMs,
	); err != nil {
		log.Printf("[AICMDBSkillService] 记录 Skill 使用日志失败: %v", err)
	}

	log.Printf("[AICMDBSkillService] ========== 优化版配置生成完成 ==========")
	log.Printf("[AICMDBSkillService] [耗时] 总计: %dms", executionTimeMs)

	return response, nil
}
