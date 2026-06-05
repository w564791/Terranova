package services

import (
	"encoding/json"
	"fmt"
	"iac-platform/internal/models"
	"log"
	"strings"

	"gorm.io/gorm"
)

// ManifestAIService manifest 编辑器的 AI 资源生成/修复服务
//
// 与 form_generation 复用同一套地基（意图断言 / SkillAssembler / callAI / LogSkillUsage），
// 但走 manifest 自己的流程：无 module_id，输出 HCL 文本而非表单 config。
//
// 流程（4 步，与现有 SSE 骨架一致）：
//  1. 初始化  - 获取 AI 配置
//  2. 意图断言 - 安全守卫（复用 AIFormService.AssertIntent）
//  3. Module 召回 - LIKE 搜索候选 module + 输入变量，拼进 prompt 供 AI 择优引用
//  4. 组装 + AI 生成 - AssemblePrompt -> callAI -> 提取 HCL -> LogSkillUsage
type ManifestAIService struct {
	db             *gorm.DB
	aiFormService  *AIFormService
	configService  *AIConfigService
	skillAssembler *SkillAssembler
}

// NewManifestAIService 创建 manifest AI 服务实例
func NewManifestAIService(db *gorm.DB) *ManifestAIService {
	return &ManifestAIService{
		db:             db,
		aiFormService:  NewAIFormService(db),
		configService:  NewAIConfigService(db),
		skillAssembler: NewSkillAssembler(db),
	}
}

// ManifestGenerateResult manifest 资源生成结果
type ManifestGenerateResult struct {
	Status     string // "complete" | "blocked"
	HCL        string // 生成的 HCL 文本
	Message    string // 提示信息（blocked 时为拦截原因）
	UsageLogID string // Skill 使用日志 ID
}

// manifestModuleCandidate Module 召回的候选项（拼进 prompt）
type manifestModuleCandidate struct {
	Name        string                `json:"name"`
	Source      string                `json:"source"`
	Version     string                `json:"version"`
	Description string                `json:"description"`
	Inputs      []manifestModuleInput `json:"inputs"`
}

type manifestModuleInput struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

// getManifestGenerationComposition 返回 manifest 资源生成的默认 Skill 组合
func (s *ManifestAIService) getManifestGenerationComposition() *models.SkillComposition {
	return &models.SkillComposition{
		FoundationSkills: []string{
			"output_format_standard",
		},
		DomainSkills: []string{
			"terraform_module_best_practices",
		},
		TaskSkill:           "manifest_resource_generation_workflow",
		AutoLoadModuleSkill: false,
		DomainSkillMode:     models.DomainSkillModeFixed,
		ConditionalRules:    []models.SkillConditionalRule{},
	}
}

// GenerateResourceWithProgress 生成/修复 manifest 资源（带进度回调）
//
// currentContent 为编辑器当前光标处的上下文（选区或当前文件内容，可为空）。
func (s *ManifestAIService) GenerateResourceWithProgress(
	userID string,
	description string,
	workspaceID string,
	organizationID string,
	currentContent string,
	progressCallback ProgressCallback,
) (*ManifestGenerateResult, error) {
	totalTimer := NewTimer()
	tracker := newManifestProgressTracker(4, manifestGenStepName, progressCallback)
	reportProgress := tracker.report

	log.Printf("[ManifestAIService] ========== 开始 manifest 资源生成 ==========")

	// 步骤 1: 获取 AI 配置
	reportProgress(1, "初始化", "正在获取 AI 配置...")
	configTimer := NewTimer()
	aiConfig, err := s.configService.GetConfigForCapability("manifest_resource_generation")
	if err != nil || aiConfig == nil {
		IncAICallCount("manifest_resource_generation", "config_error")
		return nil, fmt.Errorf("未找到可用的 AI 配置: %w", err)
	}
	RecordAICallDuration("manifest_resource_generation", "get_config", configTimer.ElapsedMs())

	// 步骤 2: 意图断言（安全守卫）
	reportProgress(2, "意图断言", "正在检查请求安全性...")
	assertionTimer := NewTimer()
	assertionResult, err := s.aiFormService.AssertIntent(userID, description)
	RecordAICallDuration("manifest_resource_generation", "intent_assertion", assertionTimer.ElapsedMs())
	if err != nil {
		log.Printf("[ManifestAIService] 意图断言服务不可用: %v，继续执行", err)
	} else if assertionResult != nil && !assertionResult.IsSafe {
		IncAICallCount("manifest_resource_generation", "blocked")
		return &ManifestGenerateResult{
			Status:  "blocked",
			Message: assertionResult.Suggestion,
		}, nil
	}

	// 步骤 3: Module 库召回（LIKE 搜索 + 输入变量）
	reportProgress(3, "Module召回", "正在检索 Module 库...")
	moduleTimer := NewTimer()
	candidates := s.recallModules(description)
	RecordAICallDuration("manifest_resource_generation", "module_recall", moduleTimer.ElapsedMs())
	moduleContext := s.buildModuleContext(candidates)
	log.Printf("[ManifestAIService] Module 召回 %d 个候选", len(candidates))

	// 步骤 4: 组装 Prompt + AI 生成
	reportProgress(4, "AI生成", "正在调用 AI 生成资源...")
	// 优先用 AIConfig 里配置的 Skill 组合(UI 可调),为空才回退硬编码默认。对齐 form_generation。
	composition := &aiConfig.SkillComposition
	if len(composition.FoundationSkills) == 0 && composition.TaskSkill == "" {
		composition = s.getManifestGenerationComposition()
	}
	dynamicContext := &DynamicContext{
		UserDescription: description,
		WorkspaceID:     workspaceID,
		OrganizationID:  organizationID,
		ExtraContext: map[string]interface{}{
			"module_candidates": moduleContext,
			"current_content":   currentContent,
		},
	}

	assembleResult, err := s.skillAssembler.AssemblePrompt(composition, 0, dynamicContext)
	if err != nil {
		IncAICallCount("manifest_resource_generation", "skill_assembly_error")
		return nil, fmt.Errorf("组装 Prompt 失败: %w", err)
	}

	aiTimer := NewTimer()
	aiResult, err := s.aiFormService.callAI(aiConfig, assembleResult.Prompt)
	RecordAICallDuration("manifest_resource_generation", "ai_call", aiTimer.ElapsedMs())
	if err != nil {
		IncAICallCount("manifest_resource_generation", "ai_error")
		return nil, fmt.Errorf("AI 调用失败: %w", err)
	}

	tracker.addStep("AI生成", int64(aiTimer.ElapsedMs()), assembleResult.UsedSkillNames)

	hcl := extractHCL(aiResult)
	if strings.TrimSpace(hcl) == "" {
		IncAICallCount("manifest_resource_generation", "parse_error")
		return nil, fmt.Errorf("AI 未返回有效的 HCL 内容")
	}

	result := &ManifestGenerateResult{
		Status: "complete",
		HCL:    hcl,
	}

	// 记录用量日志
	executionTimeMs := int(totalTimer.ElapsedMs())
	RecordAICallDuration("manifest_resource_generation", "total", totalTimer.ElapsedMs())
	IncAICallCount("manifest_resource_generation", "success")

	inputSnapshot, _ := json.Marshal(map[string]interface{}{
		"description":      description,
		"workspace_id":     workspaceID,
		"module_candidate": len(candidates),
	})
	outputSnapshot, _ := json.Marshal(map[string]interface{}{"hcl": hcl})

	logID, logErr := s.skillAssembler.LogSkillUsage(LogSkillUsageParams{
		SkillIDs:        assembleResult.UsedSkillIDs,
		Capability:      "manifest_resource_generation",
		WorkspaceID:     workspaceID,
		UserID:          userID,
		AIModel:         aiConfig.ModelID,
		ExecutionTimeMs: executionTimeMs,
		InputSnapshot:   json.RawMessage(inputSnapshot),
		OutputSnapshot:  json.RawMessage(outputSnapshot),
	})
	if logErr != nil {
		log.Printf("[ManifestAIService] 记录 Skill 使用日志失败: %v", logErr)
	} else {
		result.UsageLogID = logID
	}

	log.Printf("[ManifestAIService] ========== manifest 资源生成完成 ==========")
	return result, nil
}

// recallModules 用 LIKE 搜索召回候选 module。
//
// 注: 查询逻辑与 internal/handlers/manifest_editor_handler.go 的 ListModules /
// extractModuleInputs 同源,但无法直接复用——handlers 包已 import services,
// services 再反向 import 会形成循环依赖。两处都是只读查询,无行为分叉风险;
// 若 modules/schema 结构变更,需同步两处(handler 用于编辑器补全,此处用于 AI 召回)。
func (s *ManifestAIService) recallModules(description string) []manifestModuleCandidate {
	keywords := extractKeywords(description)
	if len(keywords) == 0 {
		return nil
	}

	type row struct {
		ID          uint   `gorm:"column:id"`
		Name        string `gorm:"column:name"`
		Source      string `gorm:"column:source"`
		Version     string `gorm:"column:version"`
		Description string `gorm:"column:description"`
	}
	var rows []row

	query := s.db.Table("modules m").
		Select(`m.id, m.name,
				COALESCE(NULLIF(m.module_source, ''), m.source) AS source,
				m.version, m.description`).
		Where("m.status = ?", "active")

	// 任一关键词命中 name / module_source / description 即召回
	orClauses := s.db
	for i, kw := range keywords {
		like := "%" + kw + "%"
		cond := "m.name ILIKE ? OR m.module_source ILIKE ? OR m.description ILIKE ?"
		if i == 0 {
			orClauses = orClauses.Where(cond, like, like, like)
		} else {
			orClauses = orClauses.Or(cond, like, like, like)
		}
	}
	query = query.Where(orClauses)

	if err := query.Order("m.name ASC").Limit(5).Scan(&rows).Error; err != nil {
		log.Printf("[ManifestAIService] Module 召回查询失败: %v", err)
		return nil
	}

	candidates := make([]manifestModuleCandidate, 0, len(rows))
	for _, r := range rows {
		candidates = append(candidates, manifestModuleCandidate{
			Name:        r.Name,
			Source:      r.Source,
			Version:     r.Version,
			Description: r.Description,
			Inputs:      s.loadModuleInputs(r.ID),
		})
	}
	return candidates
}

// loadModuleInputs 取 module 活跃 schema 的输入变量（精简版）
func (s *ManifestAIService) loadModuleInputs(moduleID uint) []manifestModuleInput {
	var schema models.Schema
	if err := s.db.Where("module_id = ? AND status = ?", moduleID, "active").
		Order("CASE WHEN schema_version = 'v2' THEN 0 ELSE 1 END, created_at DESC").
		First(&schema).Error; err != nil {
		return nil
	}

	components, ok := schema.OpenAPISchema["components"].(map[string]interface{})
	if !ok {
		return nil
	}
	schemas, ok := components["schemas"].(map[string]interface{})
	if !ok {
		return nil
	}
	moduleInput, ok := schemas["ModuleInput"].(map[string]interface{})
	if !ok {
		return nil
	}

	requiredSet := map[string]bool{}
	if reqArr, ok := moduleInput["required"].([]interface{}); ok {
		for _, r := range reqArr {
			if str, ok := r.(string); ok {
				requiredSet[str] = true
			}
		}
	}

	props, ok := moduleInput["properties"].(map[string]interface{})
	if !ok {
		return nil
	}

	out := make([]manifestModuleInput, 0, len(props))
	for name, raw := range props {
		p, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		field := manifestModuleInput{Name: name, Required: requiredSet[name]}
		if t, ok := p["type"].(string); ok {
			field.Type = t
		}
		if d, ok := p["description"].(string); ok {
			field.Description = d
		}
		out = append(out, field)
	}
	return out
}

// buildModuleContext 把候选 module 拼成喂给 AI 的文本
func (s *ManifestAIService) buildModuleContext(candidates []manifestModuleCandidate) string {
	if len(candidates) == 0 {
		return "（Module 库中无匹配项，请直接生成原生 Terraform 资源）"
	}
	b, err := json.MarshalIndent(candidates, "", "  ")
	if err != nil {
		return "（Module 库序列化失败，请直接生成原生 Terraform 资源）"
	}
	return string(b)
}

// manifestGenStepName 返回生成流程各步骤名称
func manifestGenStepName(step int) string {
	switch step {
	case 1:
		return "初始化"
	case 2:
		return "意图断言"
	case 3:
		return "Module召回"
	case 4:
		return "AI生成"
	default:
		return ""
	}
}
