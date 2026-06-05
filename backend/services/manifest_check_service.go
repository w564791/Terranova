package services

import (
	"encoding/json"
	"fmt"
	"iac-platform/internal/models"
	"log"
	"strings"

	"gorm.io/gorm"
)

// ManifestCheckService manifest 草稿检查服务
//
// 与生成流程是不同的 AI 方案：check 无用户描述输入,输出结构化问题列表。
// 但 check 会打包用户「选中的内容」发给 AI,选区中可能藏有 prompt injection,
// 因此同样要走意图断言(对待检查内容做断言),不能跳过。
//
// 流程（4 步）：
//  1. 初始化   - 获取 AI 配置
//  2. 意图断言 - 安全守卫(对待检查内容,复用 AIFormService.AssertIntent)
//  3. 打包     - 组装待检查内容（有选区只打包选区，无选区打包当前文件）
//  4. 组装 + AI 检查 - AssemblePrompt -> callAI -> 解析问题列表 -> LogSkillUsage
type ManifestCheckService struct {
	db             *gorm.DB
	aiFormService  *AIFormService
	configService  *AIConfigService
	skillAssembler *SkillAssembler
}

// NewManifestCheckService 创建 manifest check 服务实例
func NewManifestCheckService(db *gorm.DB) *ManifestCheckService {
	return &ManifestCheckService{
		db:             db,
		aiFormService:  NewAIFormService(db),
		configService:  NewAIConfigService(db),
		skillAssembler: NewSkillAssembler(db),
	}
}

// ManifestCheckResult manifest 检查结果
type ManifestCheckResult struct {
	Status     string          // "complete" | "blocked"
	Issues     []ManifestIssue // 问题列表
	Message    string          // 提示信息（blocked 时为拦截原因）
	UsageLogID string          // Skill 使用日志 ID
}

// getManifestCheckComposition 返回 manifest check 的默认 Skill 组合
func (s *ManifestCheckService) getManifestCheckComposition() *models.SkillComposition {
	return &models.SkillComposition{
		FoundationSkills: []string{
			"json_output_format",
		},
		DomainSkills: []string{
			"terraform_module_best_practices",
		},
		TaskSkill:           "manifest_check_workflow",
		AutoLoadModuleSkill: false,
		DomainSkillMode:     models.DomainSkillModeFixed,
		ConditionalRules:    []models.SkillConditionalRule{},
	}
}

// CheckDraftWithProgress 检查 manifest 草稿内容（带进度回调）
//
// filePath 为待检查内容来源路径；content 为待检查内容（选区或当前文件）；
// startLine 为 content 在文件中的起始行号（选区时>0，整文件为1）。
// 打包时给每行加真实行号前缀，AI 直接引用而非自己数，避免行号漂移。
func (s *ManifestCheckService) CheckDraftWithProgress(
	userID string,
	workspaceID string,
	filePath string,
	content string,
	startLine int,
	progressCallback ProgressCallback,
) (*ManifestCheckResult, error) {
	totalTimer := NewTimer()
	tracker := newManifestProgressTracker(4, manifestCheckStepName, progressCallback)
	reportProgress := tracker.report

	log.Printf("[ManifestCheckService] ========== 开始 manifest 草稿检查 ==========")

	// 步骤 1: 获取 AI 配置
	reportProgress(1, "初始化", "正在获取 AI 配置...")
	configTimer := NewTimer()
	aiConfig, err := s.configService.GetConfigForCapability("manifest_check")
	if err != nil || aiConfig == nil {
		IncAICallCount("manifest_check", "config_error")
		return nil, fmt.Errorf("未找到可用的 AI 配置: %w", err)
	}
	RecordAICallDuration("manifest_check", "get_config", configTimer.ElapsedMs())

	// 步骤 2: 意图断言（安全守卫）
	// check 会打包用户「选中的内容」发给 AI，选区中可能藏有 prompt injection，
	// 因此必须先做意图断言，不能跳过。
	reportProgress(2, "意图断言", "正在检查内容安全性...")
	assertionTimer := NewTimer()
	assertionResult, err := s.aiFormService.AssertIntent(userID, content)
	RecordAICallDuration("manifest_check", "intent_assertion", assertionTimer.ElapsedMs())
	if err != nil {
		log.Printf("[ManifestCheckService] 意图断言服务不可用: %v，继续执行", err)
	} else if assertionResult != nil && !assertionResult.IsSafe {
		IncAICallCount("manifest_check", "blocked")
		return &ManifestCheckResult{
			Status:  "blocked",
			Message: assertionResult.Suggestion,
		}, nil
	}

	// 步骤 3: 打包待检查内容
	reportProgress(3, "打包内容", "正在准备待检查内容...")
	checkContent := buildCheckPayload(filePath, content, startLine)

	// 步骤 4: 组装 Prompt + AI 检查
	reportProgress(4, "AI检查", "正在调用 AI 检查...")
	// 优先用 AIConfig 配置的 Skill 组合(UI 可调),为空才回退硬编码默认。
	composition := &aiConfig.SkillComposition
	if len(composition.FoundationSkills) == 0 && composition.TaskSkill == "" {
		composition = s.getManifestCheckComposition()
	}
	dynamicContext := &DynamicContext{
		ExtraContext: map[string]interface{}{
			"check_content": checkContent,
			"file_path":     filePath,
		},
	}

	assembleResult, err := s.skillAssembler.AssemblePrompt(composition, 0, dynamicContext)
	if err != nil {
		IncAICallCount("manifest_check", "skill_assembly_error")
		return nil, fmt.Errorf("组装 Prompt 失败: %w", err)
	}

	aiTimer := NewTimer()
	aiResult, err := s.aiFormService.callAI(aiConfig, assembleResult.Prompt)
	RecordAICallDuration("manifest_check", "ai_call", aiTimer.ElapsedMs())
	if err != nil {
		IncAICallCount("manifest_check", "ai_error")
		return nil, fmt.Errorf("AI 调用失败: %w", err)
	}

	tracker.addStep("AI检查", int64(aiTimer.ElapsedMs()), assembleResult.UsedSkillNames)

	issues, ok := parseCheckIssues(aiResult, filePath)
	if !ok {
		// AI 响应无法解析为问题列表，不能当作"无问题"。返回错误,由 controller 发 error 事件。
		IncAICallCount("manifest_check", "parse_error")
		return nil, fmt.Errorf("AI 返回的检查结果无法解析，请重试")
	}

	result := &ManifestCheckResult{
		Status:  "complete",
		Issues:  issues,
		Message: fmt.Sprintf("检查完成，发现 %d 个问题", len(issues)),
	}

	// 记录用量日志
	executionTimeMs := int(totalTimer.ElapsedMs())
	RecordAICallDuration("manifest_check", "total", totalTimer.ElapsedMs())
	IncAICallCount("manifest_check", "success")

	inputSnapshot, _ := json.Marshal(map[string]interface{}{
		"file_path":    filePath,
		"content_size": len(content),
	})
	outputSnapshot, _ := json.Marshal(map[string]interface{}{"issues": issues})

	logID, logErr := s.skillAssembler.LogSkillUsage(LogSkillUsageParams{
		SkillIDs:        assembleResult.UsedSkillIDs,
		Capability:      "manifest_check",
		WorkspaceID:     workspaceID,
		UserID:          userID,
		AIModel:         aiConfig.ModelID,
		ExecutionTimeMs: executionTimeMs,
		InputSnapshot:   json.RawMessage(inputSnapshot),
		OutputSnapshot:  json.RawMessage(outputSnapshot),
	})
	if logErr != nil {
		log.Printf("[ManifestCheckService] 记录 Skill 使用日志失败: %v", logErr)
	} else {
		result.UsageLogID = logID
	}

	log.Printf("[ManifestCheckService] ========== manifest 草稿检查完成 ==========")
	return result, nil
}

// buildCheckPayload 组装喂给 AI 的待检查内容。
// 给每行加真实行号前缀(从 startLine 起)，AI 直接引用前缀里的行号，
// 避免它自己数行导致漂移；选区检查时行号天然是文件绝对行号。
func buildCheckPayload(filePath, content string, startLine int) string {
	if filePath == "" {
		filePath = "(unknown)"
	}
	if startLine < 1 {
		startLine = 1
	}
	lines := strings.Split(content, "\n")
	var b strings.Builder
	for i, line := range lines {
		fmt.Fprintf(&b, "%d: %s\n", startLine+i, line)
	}
	return fmt.Sprintf("文件: %s\n每行以「行号: 内容」给出，请用前缀里的行号填 issues[].line：\n\n```hcl\n%s```", filePath, b.String())
}

// checkIssuesEnvelope AI 返回的问题列表 JSON 结构
type checkIssuesEnvelope struct {
	Issues []struct {
		File    string `json:"file"`
		Line    int    `json:"line"`
		Level   string `json:"level"`
		Message string `json:"message"`
	} `json:"issues"`
}

// parseCheckIssues 解析 AI 返回的问题列表
// 返回 (issues, ok)。ok=false 表示 AI 响应无法解析为问题列表(不能当作"无问题"处理)。
func parseCheckIssues(aiResult, defaultFile string) ([]ManifestIssue, bool) {
	jsonStr := extractJSON(aiResult)
	var envelope checkIssuesEnvelope
	if err := json.Unmarshal([]byte(jsonStr), &envelope); err != nil {
		log.Printf("[ManifestCheckService] 解析问题列表失败: %v，原始响应: %s", err, aiResult)
		return nil, false
	}

	issues := make([]ManifestIssue, 0, len(envelope.Issues))
	for _, it := range envelope.Issues {
		level := it.Level
		switch level {
		case "error", "warning", "info":
		default:
			level = "warning"
		}
		file := it.File
		if file == "" {
			file = defaultFile
		}
		issues = append(issues, ManifestIssue{
			File:    file,
			Line:    it.Line,
			Level:   level,
			Message: it.Message,
		})
	}
	return issues, true
}

// manifestCheckStepName 返回检查流程各步骤名称
func manifestCheckStepName(step int) string {
	switch step {
	case 1:
		return "初始化"
	case 2:
		return "意图断言"
	case 3:
		return "打包内容"
	case 4:
		return "AI检查"
	default:
		return ""
	}
}
