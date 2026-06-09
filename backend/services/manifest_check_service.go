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
	domainSelector *manifestDomainSkillSelector
}

// NewManifestCheckService 创建 manifest check 服务实例
func NewManifestCheckService(db *gorm.DB) *ManifestCheckService {
	return &ManifestCheckService{
		db:             db,
		aiFormService:  NewAIFormService(db),
		configService:  NewAIConfigService(db),
		skillAssembler: NewSkillAssembler(db),
		domainSelector: newManifestDomainSkillSelector(db),
	}
}

// ManifestCheckResult manifest 检查结果
type ManifestCheckResult struct {
	Status         string          // "complete" | "blocked"
	Issues         []ManifestIssue // 问题列表
	Message        string          // 提示信息（blocked 时为拦截原因）
	UsageLogID     string          // Skill 使用日志 ID
	CompletedSteps []CompletedStep // 各步骤耗时 + 加载的 skill(供前端 pipeline 展示)
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

// CheckFileInput 单个待检查文件(跨文件检查时多于一个)
type CheckFileInput struct {
	Path      string
	Content   string
	StartLine int // content 在文件中的起始行(整文件=1,选区=选区起始行)
}

// CheckDraftWithProgress 检查 manifest 草稿内容（带进度回调）
//
// files 为待检查文件集合：单文件检查时长度为 1；跨文件检查时含当前文件 + 关联文件。
// 打包时给每行加真实行号前缀(从各文件 StartLine 起)，AI 直接引用而非自己数，避免行号漂移。
func (s *ManifestCheckService) CheckDraftWithProgress(
	userID string,
	workspaceID string,
	files []CheckFileInput,
	progressCallback ProgressCallback,
) (*ManifestCheckResult, error) {
	totalTimer := NewTimer()
	tracker := newManifestProgressTracker(4, manifestCheckStepName, progressCallback)
	reportProgress := tracker.report

	if len(files) == 0 {
		return nil, fmt.Errorf("没有待检查内容")
	}
	primaryFile := files[0].Path

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
	// 因此必须先做意图断言，不能跳过。对所有待检查文件内容做断言。
	reportProgress(2, "意图断言", "正在检查内容安全性...")
	var assertInput strings.Builder
	for _, f := range files {
		assertInput.WriteString(f.Content)
		assertInput.WriteString("\n")
	}
	assertionTimer := NewTimer()
	assertionResult, err := s.aiFormService.AssertIntent(userID, assertInput.String())
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

	// 步骤 3: 打包待检查内容(多文件分段 + 行号前缀)
	reportProgress(3, "打包内容", "正在准备待检查内容...")
	checkContent := buildCheckPayload(files)

	// 步骤 4: 组装 Prompt + AI 检查
	reportProgress(4, "AI检查", "正在调用 AI 检查...")
	// 优先用 AIConfig 配置的 Skill 组合(UI 可调),为空才回退硬编码默认。
	composition := &aiConfig.SkillComposition
	if len(composition.FoundationSkills) == 0 && composition.TaskSkill == "" {
		composition = s.getManifestCheckComposition()
	}
	// 遵守开关:仅 UseOptimized=true 才跑 domain skill AI 动态选择。
	// 用精简摘要(文件名 + 原始内容,无行号前缀/指令噪声)做选择依据,而非整个打包 payload。
	if aiConfig.UseOptimized {
		if selected, selErr := s.domainSelector.Select(buildSelectionDigest(files)); selErr != nil {
			log.Printf("[ManifestCheckService] domain skill AI 选择失败,降级: %v", selErr)
		} else if len(selected) > 0 {
			compCopy := *composition
			compCopy.DomainSkills = selected
			compCopy.DomainSkillMode = models.DomainSkillModeFixed
			composition = &compCopy
		}
	}
	if moduleSkillNames := s.resolveReferencedModuleSkillNames(files); len(moduleSkillNames) > 0 {
		compCopy := *composition
		compCopy.DomainSkills = mergeSkillNames(compCopy.DomainSkills, moduleSkillNames)
		compCopy.DomainSkillMode = models.DomainSkillModeFixed
		composition = &compCopy
	}
	dynamicContext := &DynamicContext{
		ExtraContext: map[string]interface{}{
			"check_content": checkContent,
			"file_path":     primaryFile,
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

	knownFiles := make(map[string]bool, len(files))
	for _, f := range files {
		knownFiles[f.Path] = true
	}
	issues, ok := parseCheckIssues(aiResult, primaryFile, len(files) > 1, knownFiles)
	if !ok {
		// AI 响应无法解析为问题列表，不能当作"无问题"。返回错误,由 controller 发 error 事件。
		IncAICallCount("manifest_check", "parse_error")
		return nil, fmt.Errorf("AI 返回的检查结果无法解析，请重试")
	}

	result := &ManifestCheckResult{
		Status:         "complete",
		Issues:         issues,
		Message:        fmt.Sprintf("检查完成，发现 %d 个问题", len(issues)),
		CompletedSteps: tracker.steps(),
	}

	// 记录用量日志
	executionTimeMs := int(totalTimer.ElapsedMs())
	RecordAICallDuration("manifest_check", "total", totalTimer.ElapsedMs())
	IncAICallCount("manifest_check", "success")

	inputSnapshot, _ := json.Marshal(map[string]interface{}{
		"file_path":  primaryFile,
		"file_count": len(files),
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

// buildCheckPayload 组装喂给 AI 的待检查内容(多文件分段)。
// 每段一个文件,每行加真实行号前缀(从该文件 StartLine 起),AI 直接引用前缀里的行号,
// 避免它自己数行导致漂移;选区检查时行号天然是文件绝对行号。
// buildSelectionDigest 为 domain skill 选择构建精简摘要:文件名 + 原始内容(无行号前缀/指令噪声)。
// 每个文件内容截断到 ~2000 字符,避免把超大 payload 灌给选择器。
func buildSelectionDigest(files []CheckFileInput) string {
	const maxPerFile = 2000
	var b strings.Builder
	for _, f := range files {
		path := f.Path
		if path == "" {
			path = "(unknown)"
		}
		content := f.Content
		if r := []rune(content); len(r) > maxPerFile {
			content = string(r[:maxPerFile]) + "\n..."
		}
		fmt.Fprintf(&b, "# %s\n%s\n\n", path, content)
	}
	return b.String()
}

func buildCheckPayload(files []CheckFileInput) string {
	var out strings.Builder
	out.WriteString("以下是待检查的文件(可能多个)。每行以「行号: 内容」给出,")
	out.WriteString("请用前缀里的行号填 issues[].line,并用对应 ### 文件名填 issues[].file:\n\n")
	for _, f := range files {
		path := f.Path
		if path == "" {
			path = "(unknown)"
		}
		start := f.StartLine
		if start < 1 {
			start = 1
		}
		fmt.Fprintf(&out, "### 文件: %s\n```hcl\n", path)
		for i, line := range strings.Split(f.Content, "\n") {
			fmt.Fprintf(&out, "%d: %s\n", start+i, line)
		}
		out.WriteString("```\n\n")
	}
	return out.String()
}

// resolveReferencedModuleSkillNames 从被检查的 HCL 内容中精确解析 module source,
// 只加载实际引用到的平台 module skill。module_auto 不进入 AI 自由选择池,避免误选无关模块。
func (s *ManifestCheckService) resolveReferencedModuleSkillNames(files []CheckFileInput) []string {
	scope := make(map[string][]byte, len(files))
	for _, f := range files {
		if strings.TrimSpace(f.Content) == "" {
			continue
		}
		scope[f.Path] = []byte(f.Content)
	}
	sourceByName := ParseManifestModuleSourcesForCheck(scope)
	if len(sourceByName) == 0 {
		return nil
	}

	sourceSet := make(map[string]bool, len(sourceByName))
	sources := make([]string, 0, len(sourceByName))
	for _, source := range sourceByName {
		source = strings.TrimSpace(source)
		if source == "" || sourceSet[source] {
			continue
		}
		sourceSet[source] = true
		sources = append(sources, source)
	}
	if len(sources) == 0 {
		return nil
	}
	log.Printf("[ManifestCheckService] 从 HCL 解析到 module sources: %v", sources)

	modules := s.resolveModulesBySources(sources)
	if len(modules) == 0 {
		log.Printf("[ManifestCheckService] module sources 未匹配到平台 active module: %v", sources)
		return nil
	}

	names := make([]string, 0, len(modules))
	for _, module := range modules {
		name, ok := s.ensureAutoModuleSkillName(module.ID)
		if !ok {
			continue
		}
		names = append(names, name)
	}
	if len(names) > 0 {
		log.Printf("[ManifestCheckService] 根据 HCL module source 精确加载 module skills: %v", names)
	}
	return names
}

func (s *ManifestCheckService) resolveModulesBySources(sources []string) []models.Module {
	moduleByID := make(map[uint]models.Module)
	orderedIDs := make([]uint, 0)
	addModule := func(module models.Module) {
		if _, ok := moduleByID[module.ID]; ok {
			return
		}
		moduleByID[module.ID] = module
		orderedIDs = append(orderedIDs, module.ID)
	}

	var modules []models.Module
	if err := s.db.Where("status = ?", "active").
		Where("module_source IN ? OR source IN ?", sources, sources).
		Order("id ASC").
		Find(&modules).Error; err != nil {
		log.Printf("[ManifestCheckService] 通过 modules 匹配 module source 失败: %v", err)
	} else {
		for _, module := range modules {
			addModule(module)
		}
	}

	var versions []models.ModuleVersion
	if err := s.db.Where("status = ?", "active").
		Where("module_source IN ? OR source IN ?", sources, sources).
		Order("module_id ASC").
		Find(&versions).Error; err != nil {
		log.Printf("[ManifestCheckService] 通过 module_versions 匹配 module source 失败: %v", err)
	} else if len(versions) > 0 {
		moduleIDs := make([]uint, 0, len(versions))
		seen := make(map[uint]bool, len(versions))
		for _, version := range versions {
			if version.ModuleID == 0 || seen[version.ModuleID] {
				continue
			}
			seen[version.ModuleID] = true
			moduleIDs = append(moduleIDs, version.ModuleID)
		}
		if len(moduleIDs) > 0 {
			var versionModules []models.Module
			if err := s.db.Where("status = ?", "active").
				Where("id IN ?", moduleIDs).
				Order("id ASC").
				Find(&versionModules).Error; err != nil {
				log.Printf("[ManifestCheckService] 加载 module_versions 对应 module 失败: %v", err)
			} else {
				for _, module := range versionModules {
					addModule(module)
				}
			}
		}
	}

	result := make([]models.Module, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		result = append(result, moduleByID[id])
	}
	return result
}

func (s *ManifestCheckService) ensureAutoModuleSkillName(moduleID uint) (string, bool) {
	skillName := fmt.Sprintf("module_%d_auto", moduleID)

	var existingSkill models.Skill
	err := s.db.Where("name = ? AND source_module_id = ? AND is_active = ?", skillName, moduleID, true).
		First(&existingSkill).Error
	if err == nil {
		if s.skillAssembler.moduleSkillGen.ShouldRegenerate(&existingSkill, moduleID) {
			skill, regenErr := s.skillAssembler.regenerateModuleSkill(moduleID, &existingSkill)
			if regenErr != nil {
				log.Printf("[ManifestCheckService] 重新生成 module %d auto skill 失败: %v", moduleID, regenErr)
				return "", false
			}
			return skill.Name, true
		}
		return existingSkill.Name, true
	}
	if err != gorm.ErrRecordNotFound {
		log.Printf("[ManifestCheckService] 查询 module %d auto skill 失败: %v", moduleID, err)
		return "", false
	}

	skill, genErr := s.skillAssembler.generateNewModuleSkill(moduleID)
	if genErr != nil {
		log.Printf("[ManifestCheckService] 生成 module %d auto skill 失败: %v", moduleID, genErr)
		return "", false
	}
	if skill == nil || skill.Name == "" {
		return "", false
	}
	return skill.Name, true
}

func mergeSkillNames(base []string, extra []string) []string {
	seen := make(map[string]bool, len(base)+len(extra))
	merged := make([]string, 0, len(base)+len(extra))
	for _, name := range base {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		merged = append(merged, name)
	}
	for _, name := range extra {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		merged = append(merged, name)
	}
	return merged
}

// checkIssuesEnvelope AI 返回的问题列表 JSON 结构
type checkIssuesEnvelope struct {
	Issues []struct {
		File    string `json:"file"`
		Line    int    `json:"line"`
		Level   string `json:"level"`
		Message string `json:"message"`
		Fix     *struct {
			File      string `json:"file"`
			StartLine int    `json:"start_line"`
			EndLine   int    `json:"end_line"`
			NewText   string `json:"new_text"`
		} `json:"fix"`
	} `json:"issues"`
}

// parseCheckIssues 解析 AI 返回的问题列表
// 返回 (issues, ok)。ok=false 表示 AI 响应无法解析为问题列表(不能当作"无问题"处理)。
// multiFile=true 时(跨文件检查),issue/fix 必须明确归属文件,否则不能盲目落到主文件——
// 因为关联文件的绝对行号若被当成主文件行号去替换会改坏主文件。
// isNonIssueNarrative 判断一条 message 是否其实是"已满足/自检叙述",而非真实违规。
// prompt 已禁止 AI 输出这类内容,但模型偶发违反(且常自相矛盾,如先说缺失又说已满足),
// 这里做最后一道防御:命中"合规/无需补充/已满足"等措辞即视为伪 issue 丢弃。
func isNonIssueNarrative(message string) bool {
	m := strings.ToLower(strings.TrimSpace(message))
	// "已满足/无需补充/已齐全/均已包含/无缺失/已合规/符合规范/已完整" 这类是结论性"通过"叙述
	satisfiedPhrases := []string{
		"无需补充", "已满足", "已齐全", "均已包含", "无缺失", "无需补", "已合规",
		"符合规范", "已完整", "标签清单已满足", "已包含所需", "已全部包含", "无需新增",
		"无需添加", "无需修改", "检查通过", "未发现问题", "全部满足",
	}
	for _, p := range satisfiedPhrases {
		if strings.Contains(m, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

func parseCheckIssues(aiResult, defaultFile string, multiFile bool, knownFiles map[string]bool) ([]ManifestIssue, bool) {
	jsonStr := extractJSON(aiResult)
	var envelope checkIssuesEnvelope
	if err := json.Unmarshal([]byte(jsonStr), &envelope); err != nil {
		log.Printf("[ManifestCheckService] 解析问题列表失败: %v，原始响应: %s", err, aiResult)
		return nil, false
	}

	issues := make([]ManifestIssue, 0, len(envelope.Issues))
	for _, it := range envelope.Issues {
		// 防御:空白 message 不是有效问题,跳过(AI 偶尔吐空壳 issue)
		if strings.TrimSpace(it.Message) == "" {
			continue
		}
		// 防御:AI 偶尔违反 prompt,把"自检叙述/已满足"当 issue 输出(且常自相矛盾,
		// 如"缺少X不完整...已满足,无需补充")。这类不是真实违规,丢弃,避免误导用户。
		if isNonIssueNarrative(it.Message) {
			log.Printf("[ManifestCheckService] 丢弃自检叙述类伪 issue: %q", it.Message)
			continue
		}
		level := it.Level
		switch level {
		case "error", "warning", "info":
		default:
			level = "warning"
		}
		// 无 file 时默认归主文件(defaultFile=files[0],检查主体;关联文件只是上下文)。
		// 多文件场景 AI 常省略主文件的 file 字段,不能因此丢弃问题/修复。
		file := it.File
		if file == "" {
			file = defaultFile
		}
		issue := ManifestIssue{
			File:    file,
			Line:    it.Line,
			Level:   level,
			Message: it.Message,
		}
		// fix 行范围合法才考虑。多文件场景下,fix 的行号是「某文件的绝对行号」,
		// 必须能确定归属文件;否则不能落到主文件(会把别的文件的行号打到主文件,改坏内容)。
		if it.Fix != nil && it.Fix.StartLine > 0 && it.Fix.EndLine >= it.Fix.StartLine {
			fixFile := it.Fix.File
			if fixFile == "" && it.File != "" {
				fixFile = it.File // 仅当 issue 显式给了 file 才据此推断 fix 归属
			}
			switch {
			case multiFile && fixFile == "":
				// 多文件且无法确定 fix 归属文件 → 丢弃 fix(只保留问题描述,不给修复按钮)
				log.Printf("[ManifestCheckService] 丢弃 fix:多文件检查但未指明目标文件,无法安全定位")
			case multiFile && !knownFiles[fixFile]:
				log.Printf("[ManifestCheckService] 丢弃 fix:目标文件 %q 不在待检查文件集合内", fixFile)
			default:
				if fixFile == "" {
					fixFile = file // 单文件场景:落到唯一文件
				}
				issue.Fix = &ManifestFix{
					File:      fixFile,
					StartLine: it.Fix.StartLine,
					EndLine:   it.Fix.EndLine,
					NewText:   it.Fix.NewText,
				}
			}
		}
		issues = append(issues, issue)
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
