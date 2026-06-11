package services

import (
	"encoding/json"
	"fmt"
	"iac-platform/internal/models"
	"log"
	"sort"
	"strings"
	"sync"

	"gorm.io/gorm"
)

// manifestSchemaValidateConcurrency schema 校验的最大并发 goroutine 数
const manifestSchemaValidateConcurrency = 4

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
	domainSelector *manifestDomainSkillSelector
}

// NewManifestAIService 创建 manifest AI 服务实例
func NewManifestAIService(db *gorm.DB) *ManifestAIService {
	return &ManifestAIService{
		db:             db,
		aiFormService:  NewAIFormService(db),
		configService:  NewAIConfigService(db),
		skillAssembler: NewSkillAssembler(db),
		domainSelector: newManifestDomainSkillSelector(db),
	}
}

// ManifestGenerateResult manifest 资源生成结果
type ManifestGenerateResult struct {
	Status         string          // "complete" | "blocked"
	HCL            string          // 生成的 HCL 文本
	Message        string          // 提示信息（blocked 时为拦截原因）
	UsageLogID     string          // Skill 使用日志 ID
	Warnings       []string        // schema 校验发现的问题(仅 module 块,引用仓库已有 module 时)
	CompletedSteps []CompletedStep // 各步骤耗时 + 使用的 skill(供前端 pipeline 展示)
}

// manifestModuleCandidate Module 库候选项(拼进 prompt 供 AI 挑选)
type manifestModuleCandidate struct {
	moduleID    uint     // 内部用(加载 module skill),不进 prompt
	Name        string   `json:"name"`
	Source      string   `json:"source"`
	Versions    []string `json:"versions,omitempty"` // 可用版本列表(从 module_versions 表)
	Description string   `json:"description"`
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
	tracker := newManifestProgressTracker(5, manifestGenStepName, progressCallback)
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

	// 步骤 3: 列出全部 module 库候选(精简字段),交给 AI 按描述挑选
	reportProgress(3, "Module候选", "正在列出 Module 库...")
	moduleTimer := NewTimer()
	candidates := s.listAllModules()
	RecordAICallDuration("manifest_resource_generation", "module_list", moduleTimer.ElapsedMs())
	moduleContext := s.buildModuleContext(candidates)
	log.Printf("[ManifestAIService] Module 库候选 %d 个", len(candidates))

	// 优先用 AIConfig 里配置的 Skill 组合(UI 可调),为空才回退硬编码默认。对齐 form_generation。
	composition := &aiConfig.SkillComposition
	if len(composition.FoundationSkills) == 0 && composition.TaskSkill == "" {
		composition = s.getManifestGenerationComposition()
	}

	// 步骤 4: Domain Skill + Module 选择(AI 同时选择最佳实践 skill 和相关 module)
	moduleSkills := ""
	var moduleSkillNames []string
	reportProgress(4, "Skill选择", "正在选择相关 Skill...")
	if aiConfig.UseOptimized {
		// 构建 module 信息列表传给 AI
		moduleInfos := make([]manifestModuleInfo, len(candidates))
		for i, c := range candidates {
			moduleInfos[i] = manifestModuleInfo{
				Name:        c.Name,
				Source:      c.Source,
				Versions:    c.Versions,
				Description: c.Description,
			}
		}

		selectedSkills, selectedModules, selErr := s.domainSelector.Select(description, moduleInfos)
		if selErr != nil {
			log.Printf("[ManifestAIService] skill AI 选择失败,降级: %v", selErr)
		} else {
			if len(selectedSkills) > 0 {
				compCopy := *composition
				compCopy.DomainSkills = selectedSkills
				compCopy.DomainSkillMode = models.DomainSkillModeFixed
				composition = &compCopy
			}
			// AI 选中的 modules → 加载 module_version_skills
			if composition.AutoLoadModuleSkill && len(selectedModules) > 0 {
				moduleSkills, moduleSkillNames = s.loadModuleSkillsByNames(selectedModules, candidates)
			}
		}
	}

	// 步骤 5: 组装 Prompt + AI 生成
	reportProgress(5, "AI生成", "正在调用 AI 生成资源...")
	dynamicContext := &DynamicContext{
		UserDescription: description,
		WorkspaceID:     workspaceID,
		OrganizationID:  organizationID,
		ExtraContext: map[string]interface{}{
			"module_candidates": moduleContext,
			"current_content":   currentContent,
			"module_skills":     moduleSkills,
		},
	}

	assembleResult, err := s.skillAssembler.AssemblePrompt(composition, 0, dynamicContext)
	if err != nil {
		IncAICallCount("manifest_resource_generation", "skill_assembly_error")
		return nil, fmt.Errorf("组装 Prompt 失败: %w", err)
	}
	// 把精确匹配加载的 module skill 名追加到 UsedSkillNames(pipeline 展示)
	assembleResult.UsedSkillNames = append(assembleResult.UsedSkillNames, moduleSkillNames...)

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

	// 固定流程:对生成 HCL 里引用了「仓库已有 module」的块做 schema 校验(并发)。
	// resource 块不校验;仓库不存在的 module 跳过;任何失败降级,不阻断生成。
	warnings := s.validateModuleBlocks(hcl)

	result := &ManifestGenerateResult{
		Status:         "complete",
		HCL:            hcl,
		Warnings:       warnings,
		CompletedSteps: tracker.steps(),
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

// loadModuleSkillsByNames 按 AI 选中的 module(含版本) 加载 module_version_skills 内容。
// 从 candidates 找到 module_id → 优先指定版本 → 回退默认版本 → module_version_skills。
// 返回 (skill 内容文本, skill 名列表)。
func (s *ManifestAIService) loadModuleSkillsByNames(selectedModules []selectedModule, candidates []manifestModuleCandidate) (string, []string) {
	if len(selectedModules) == 0 {
		return "", nil
	}

	// 构建 name → moduleID 映射
	nameToID := make(map[string]uint, len(candidates))
	for _, c := range candidates {
		nameToID[c.Name] = c.moduleID
	}

	mvSvc := NewModuleVersionService(s.db)
	var b strings.Builder
	var names []string

	for _, sel := range selectedModules {
		moduleID, ok := nameToID[sel.Name]
		if !ok || moduleID == 0 {
			log.Printf("[ManifestAIService] AI 选中 module %q 不在候选中,跳过", sel.Name)
			continue
		}

		var mv *models.ModuleVersion
		var err error
		versionUsed := ""

		// 优先使用 AI 指定的版本
		if sel.Version != "" {
			mv, err = mvSvc.GetVersionByVersion(moduleID, sel.Version)
			if err != nil || mv == nil {
				log.Printf("[ManifestAIService] module %s 指定版本 %s 不存在,回退默认版本", sel.Name, sel.Version)
			} else {
				versionUsed = sel.Version
			}
		}

		// 回退到默认版本
		if mv == nil {
			mv, err = mvSvc.GetDefaultVersion(moduleID)
			if err != nil || mv == nil {
				log.Printf("[ManifestAIService] module %s 无默认版本,跳过", sel.Name)
				continue
			}
			versionUsed = mv.Version
		}

		type versionSkill struct {
			SchemaGeneratedContent string `gorm:"column:schema_generated_content"`
			CustomContent          string `gorm:"column:custom_content"`
		}
		var vs versionSkill
		if err := s.db.Table("module_version_skills").
			Where("module_version_id = ?", mv.ID).
			Select("schema_generated_content", "custom_content").
			First(&vs).Error; err != nil {
			log.Printf("[ManifestAIService] module %s version %s 无 skill 记录,跳过", sel.Name, versionUsed)
			continue
		}
		// 组合 schema_generated_content + custom_content
		combinedContent := vs.SchemaGeneratedContent
		if vs.CustomContent != "" {
			if combinedContent != "" {
				combinedContent += "\n\n---\n\n## 用户自定义补充\n\n"
			}
			combinedContent += vs.CustomContent
		}
		if combinedContent == "" {
			log.Printf("[ManifestAIService] module %s version %s skill 内容为空,跳过", sel.Name, versionUsed)
			continue
		}

		skillName := fmt.Sprintf("module_%d_version_skill", moduleID)
		names = append(names, skillName)
		fmt.Fprintf(&b, "## Module: %s (version: %s)\n%s\n\n", sel.Name, versionUsed, combinedContent)
		log.Printf("[ManifestAIService] 精确加载 module skill: %s (%s %s)", skillName, sel.Name, versionUsed)
	}

	return b.String(), names
}

// validateModuleBlocks 对生成 HCL 里引用了「仓库已有 module」的 module 块做 schema 校验。
// 每个 module 块一个 goroutine,信号量限制最大并发。resource 块不在此处理(调用方只传 module 块)。
// 返回人类可读的校验警告;任何失败/查不到的 module 跳过,不阻断生成。
func (s *ManifestAIService) validateModuleBlocks(hcl string) []string {
	blocks := ParseManifestModuleBlocks(hcl)
	if len(blocks) == 0 {
		return nil
	}

	var (
		mu       sync.Mutex
		warnings []string
		wg       sync.WaitGroup
		sem      = make(chan struct{}, manifestSchemaValidateConcurrency)
	)

	for _, blk := range blocks {
		if blk.Source == "" {
			continue // 无 source,无法定位仓库 module,跳过
		}
		wg.Add(1)
		go func(b ParsedModuleBlock) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			ws := s.validateOneModuleBlock(b)
			if len(ws) > 0 {
				mu.Lock()
				warnings = append(warnings, ws...)
				mu.Unlock()
			}
		}(blk)
	}
	wg.Wait()

	sort.Strings(warnings) // 稳定输出(goroutine 完成顺序不定)
	return warnings
}

// validateOneModuleBlock 校验单个 module 块:source→module,version→module_version(无则默认),
// 取对应 schema 跑 SchemaSolver。仓库不存在该 module / 无 schema 则跳过(返回 nil)。
func (s *ManifestAIService) validateOneModuleBlock(b ParsedModuleBlock) []string {
	prefix := fmt.Sprintf("module.%s (%s):", b.InstanceName, b.Source)
	var out []string

	// 1. 按 source 找仓库 module(稳定排序,避免同 source 多 module 时随机选)
	var module models.Module
	if err := s.db.Where("module_source = ? OR source = ?", b.Source, b.Source).
		Order("id ASC").
		First(&module).Error; err != nil {
		return nil // 仓库无此 module(可能是外部 module),跳过
	}

	// 2. version → module_version_id(无 version 用默认/最新版)
	moduleVersionID := ""
	mvSvc := NewModuleVersionService(s.db)
	if b.Version != "" {
		if mv, err := mvSvc.GetVersionByVersion(module.ID, b.Version); err == nil && mv != nil {
			moduleVersionID = mv.ID
		} else {
			// 指定了 version 但仓库查不到 → 明确报警,再回退默认版校验
			out = append(out, fmt.Sprintf("%s 版本 %q 在 Module 库中不存在,已按默认版本校验", prefix, b.Version))
		}
	}
	if moduleVersionID == "" {
		if mv, err := mvSvc.GetDefaultVersion(module.ID); err == nil && mv != nil {
			moduleVersionID = mv.ID
		}
	}

	// 3. 取 schema 跑 SchemaSolver
	solver := NewSchemaSolverWithVersion(s.db, module.ID, moduleVersionID)
	if err := solver.LoadSchema(); err != nil {
		log.Printf("[ManifestAIService] module %q(%s) 无可用 schema,跳过校验: %v", b.InstanceName, b.Source, err)
		return out
	}

	// 把"出现过但不可静态求值"的参数(如 vpc_id = var.x)也算作"已提供",
	// 避免必填检查把引用变量的参数误报为缺失(#1)。占位值仅用于 exists 判定。
	params := make(map[string]interface{}, len(b.PresentParams))
	for name := range b.PresentParams {
		params[name] = "<unevaluable>"
	}
	for k, v := range b.Parameters {
		params[k] = v // 可静态求值的覆盖占位,参与值约束校验
	}

	res := solver.Solve(params)
	if res.Success {
		return out
	}

	// 4. 汇总该块的校验问题为警告
	fbCount := 0
	for _, fb := range res.Feedbacks {
		if fb != nil && fb.Message != "" {
			out = append(out, prefix+" "+fb.Message)
			fbCount++
		}
	}
	if fbCount == 0 {
		out = append(out, prefix+" 参数未通过 schema 校验")
	}
	return out
}

// listAllModules 列出全部 active module(精简字段 + 可用版本),交给 AI 自行按描述挑选。
//
// 不做关键词/向量召回筛选:module 库常用量级 ≤100,name+source+description 全列进
// prompt 的 token 成本可控,而"中文描述↔module 语义匹配"正是 LLM 最擅长的,比 LIKE
// 分词/向量阈值都更准(LIKE 对"s3桶"这类中文夹英文会 0 命中)。
//
// 不在此加载每个 module 的变量 schema(避免 N 次查询 + prompt 膨胀);被选中 module 的
// 参数知识由 module skill 提供(AutoLoadModuleSkill)。
func (s *ManifestAIService) listAllModules() []manifestModuleCandidate {
	type row struct {
		ID          uint   `gorm:"column:id"`
		Name        string `gorm:"column:name"`
		Source      string `gorm:"column:source"`
		Description string `gorm:"column:description"`
	}
	var rows []row

	if err := s.db.Table("modules m").
		Select(`m.id, m.name,
				COALESCE(NULLIF(m.module_source, ''), m.source) AS source,
				m.description`).
		Where("m.status = ?", "active").
		Order("m.name ASC").
		Scan(&rows).Error; err != nil {
		log.Printf("[ManifestAIService] 列出 module 失败: %v", err)
		return nil
	}

	// 查询所有 module 的可用版本(从 module_versions 表)
	type versionRow struct {
		ModuleID uint   `gorm:"column:module_id"`
		Version  string `gorm:"column:version"`
	}
	var versions []versionRow
	if len(rows) > 0 {
		moduleIDs := make([]uint, len(rows))
		for i, r := range rows {
			moduleIDs[i] = r.ID
		}
		if err := s.db.Table("module_versions").
			Select("module_id, version").
			Where("module_id IN ? AND status = ?", moduleIDs, "active").
			Order("module_id, version DESC").
			Scan(&versions).Error; err != nil {
			log.Printf("[ManifestAIService] 查询 module versions 失败: %v", err)
		}
	}

	// 按 module_id 分组版本
	versionsByModule := make(map[uint][]string)
	for _, v := range versions {
		versionsByModule[v.ModuleID] = append(versionsByModule[v.ModuleID], v.Version)
	}

	candidates := make([]manifestModuleCandidate, 0, len(rows))
	for _, r := range rows {
		candidates = append(candidates, manifestModuleCandidate{
			moduleID:    r.ID,
			Name:        r.Name,
			Source:      r.Source,
			Versions:    versionsByModule[r.ID],
			Description: r.Description,
		})
	}
	return candidates
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
		return "Module候选"
	case 4:
		return "Skill选择"
	case 5:
		return "AI生成"
	default:
		return ""
	}
}
