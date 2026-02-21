package services

import (
	"fmt"
	"iac-platform/internal/models"
	"log"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SkillAssembler Skill 组装器
// 负责根据 SkillComposition 配置组装最终的 Prompt
type SkillAssembler struct {
	db               *gorm.DB
	moduleSkillGen   *ModuleSkillGenerator
	skillCache       map[string]*models.Skill // 简单的内存缓存
	skillCacheExpiry time.Time
}

// NewSkillAssembler 创建 SkillAssembler 实例
func NewSkillAssembler(db *gorm.DB) *SkillAssembler {
	return &SkillAssembler{
		db:             db,
		moduleSkillGen: NewModuleSkillGenerator(db),
		skillCache:     make(map[string]*models.Skill),
	}
}

// AssemblePromptResult 组装结果
type AssemblePromptResult struct {
	Prompt          string   // 组装后的完整 Prompt
	UsedSkillIDs    []string // 使用的 Skill ID 列表
	UsedSkillNames  []string // 使用的 Skill 名称列表
	ModuleSkillUsed bool     // 是否使用了 Module 自动生成的 Skill
	AssemblyTimeMs  int64    // 组装耗时（毫秒）
}

// DynamicContext 动态上下文
type DynamicContext struct {
	UserDescription string                 // 用户描述
	WorkspaceID     string                 // Workspace ID
	OrganizationID  string                 // Organization ID
	ModuleID        uint                   // Module ID
	UseCMDB         bool                   // 是否使用 CMDB
	CurrentConfig   map[string]interface{} // 当前配置
	CMDBData        string                 // CMDB 查询结果
	SchemaData      string                 // Schema 约束数据
	ExtraContext    map[string]interface{} // 额外上下文
}

// AssemblePrompt 组装最终 Prompt
// 返回组装后的 Prompt、使用的 Skill ID 列表、错误
func (a *SkillAssembler) AssemblePrompt(
	composition *models.SkillComposition,
	moduleID uint,
	dynamicContext *DynamicContext,
) (*AssemblePromptResult, error) {
	startTime := time.Now()
	result := &AssemblePromptResult{
		UsedSkillIDs:   make([]string, 0),
		UsedSkillNames: make([]string, 0),
	}

	if composition == nil {
		return nil, fmt.Errorf("SkillComposition 不能为空")
	}

	var promptParts []string
	var allSkills []*models.Skill

	// 1. 加载 Foundation 层 Skills
	log.Printf("[SkillAssembler] 加载 Foundation Skills: %v", composition.FoundationSkills)
	foundationSkills, err := a.loadSkillsByNames(composition.FoundationSkills)
	if err != nil {
		log.Printf("[SkillAssembler] 加载 Foundation Skills 失败: %v", err)
		// 不返回错误，继续处理
	}
	allSkills = append(allSkills, foundationSkills...)

	// 2. 根据 DomainSkillMode 加载 Domain 层 Skills
	domainSkillMode := composition.DomainSkillMode
	if domainSkillMode == "" {
		domainSkillMode = models.DomainSkillModeFixed // 默认为 fixed 模式
	}
	log.Printf("[SkillAssembler] Domain Skill 加载模式: %s", domainSkillMode)

	switch domainSkillMode {
	case models.DomainSkillModeFixed:
		// 固定选择模式：只使用手动配置的 domain_skills
		log.Printf("[SkillAssembler] 加载固定选择的 Domain Skills: %v", composition.DomainSkills)
		domainSkills, err := a.loadSkillsByNames(composition.DomainSkills)
		if err != nil {
			log.Printf("[SkillAssembler] 加载 Domain Skills 失败: %v", err)
		}
		allSkills = append(allSkills, domainSkills...)

	case models.DomainSkillModeAuto:
		// 自动发现模式：优先使用标签匹配，降级到内容解析
		if composition.TaskSkill != "" {
			taskSkill, err := a.LoadSkill(composition.TaskSkill)
			if err != nil {
				log.Printf("[SkillAssembler] 加载 Task Skill 失败，无法自动发现 Domain Skills: %v", err)
			} else if taskSkill != nil {
				// 优先使用标签匹配
				discoveredSkills := a.discoverDomainSkillsByTags(taskSkill)
				if len(discoveredSkills) > 0 {
					log.Printf("[SkillAssembler] 通过标签匹配发现了 %d 个 Domain Skills", len(discoveredSkills))
					allSkills = append(allSkills, discoveredSkills...)
				} else {
					// 降级到内容解析（@require-domain 声明）
					log.Printf("[SkillAssembler] Task Skill 没有 domain_tags，降级到内容解析")
					discoveredSkills = a.discoverDomainSkillsFromContent(taskSkill.Content, dynamicContext)
					log.Printf("[SkillAssembler] 通过内容解析发现了 %d 个 Domain Skills", len(discoveredSkills))
					allSkills = append(allSkills, discoveredSkills...)
				}
			}
		}

	case models.DomainSkillModeHybrid:
		// 混合模式：固定选择 + 自动发现补充
		log.Printf("[SkillAssembler] 混合模式：加载固定选择的 Domain Skills: %v", composition.DomainSkills)
		domainSkills, err := a.loadSkillsByNames(composition.DomainSkills)
		if err != nil {
			log.Printf("[SkillAssembler] 加载 Domain Skills 失败: %v", err)
		}
		allSkills = append(allSkills, domainSkills...)

		// 再从 Task Skill 中发现补充的（优先标签匹配）
		if composition.TaskSkill != "" {
			taskSkill, err := a.LoadSkill(composition.TaskSkill)
			if err != nil {
				log.Printf("[SkillAssembler] 加载 Task Skill 失败，无法自动发现补充的 Domain Skills: %v", err)
			} else if taskSkill != nil {
				// 优先使用标签匹配
				discoveredSkills := a.discoverDomainSkillsByTags(taskSkill)
				if len(discoveredSkills) > 0 {
					log.Printf("[SkillAssembler] 通过标签匹配发现了 %d 个补充的 Domain Skills", len(discoveredSkills))
					allSkills = append(allSkills, discoveredSkills...)
				} else {
					// 降级到内容解析
					discoveredSkills = a.discoverDomainSkillsFromContent(taskSkill.Content, dynamicContext)
					log.Printf("[SkillAssembler] 通过内容解析发现了 %d 个补充的 Domain Skills", len(discoveredSkills))
					allSkills = append(allSkills, discoveredSkills...)
				}
			}
		}

	default:
		// 未知模式，降级到 fixed
		log.Printf("[SkillAssembler] 未知的 Domain Skill 模式 '%s'，降级到 fixed 模式", domainSkillMode)
		domainSkills, err := a.loadSkillsByNames(composition.DomainSkills)
		if err != nil {
			log.Printf("[SkillAssembler] 加载 Domain Skills 失败: %v", err)
		}
		allSkills = append(allSkills, domainSkills...)
	}

	// 3. 评估条件规则，加载额外的 Skills
	if dynamicContext != nil && len(composition.ConditionalRules) > 0 {
		conditionalSkills := a.evaluateConditionalRules(composition.ConditionalRules, dynamicContext)
		allSkills = append(allSkills, conditionalSkills...)
	}

	// 4. 如果启用自动加载 Module Skill，生成并加载
	if composition.AutoLoadModuleSkill && moduleID > 0 {
		log.Printf("[SkillAssembler] 自动加载 Module Skill, moduleID=%d", moduleID)
		moduleSkill, err := a.GetOrGenerateModuleSkill(moduleID)
		if err != nil {
			log.Printf("[SkillAssembler] 加载 Module Skill 失败: %v", err)
		} else if moduleSkill != nil {
			allSkills = append(allSkills, moduleSkill)
			result.ModuleSkillUsed = true
		}
	}

	// 5. 加载 Task 层 Skill
	if composition.TaskSkill != "" {
		log.Printf("[SkillAssembler] 加载 Task Skill: %s", composition.TaskSkill)
		taskSkill, err := a.LoadSkill(composition.TaskSkill)
		if err != nil {
			log.Printf("[SkillAssembler] 加载 Task Skill 失败: %v", err)
		} else if taskSkill != nil {
			allSkills = append(allSkills, taskSkill)
		}
	}

	// 6. 按层级和优先级排序
	sortedSkills := a.sortSkills(allSkills)

	// 7. 打印组装前每个 Skill 的内容
	log.Printf("[SkillAssembler] ========== 组装前 Skills 列表 ==========")
	log.Printf("[SkillAssembler] 共 %d 个 Skills:", len(sortedSkills))
	for i, skill := range sortedSkills {
		if skill.IsActive {
			log.Printf("[SkillAssembler] [%d] ID: %s | Name: %s | Layer: %s | Priority: %d | ContentLen: %d",
				i+1, skill.ID, skill.Name, skill.Layer, skill.Priority, len(skill.Content))
		}
	}
	log.Printf("[SkillAssembler] ========== Skills 详细内容 ==========")
	for i, skill := range sortedSkills {
		if skill.IsActive {
			log.Printf("[SkillAssembler] [%d] Skill: %s (ID: %s, Layer: %s, Priority: %d)",
				i+1, skill.Name, skill.ID, skill.Layer, skill.Priority)
			log.Printf("[SkillAssembler] [%d] Content Preview (前500字符):\n%s",
				i+1, truncateString(skill.Content, 500))
			log.Printf("[SkillAssembler] [%d] Content Length: %d 字符", i+1, len(skill.Content))
			log.Printf("[SkillAssembler] ---")
		}
	}

	// 8. 组装 Prompt
	metaRulesEnabled := composition.MetaRules != nil && composition.MetaRules.Enabled

	if metaRulesEnabled {
		preamble := a.buildMetaRulesPreamble(composition.MetaRules, sortedSkills)
		sectionedBody := a.buildSectionedPrompt(sortedSkills)
		promptParts = append(promptParts, preamble, sectionedBody)
		for _, skill := range sortedSkills {
			if skill.IsActive {
				result.UsedSkillIDs = append(result.UsedSkillIDs, skill.ID)
				result.UsedSkillNames = append(result.UsedSkillNames, skill.Name)
			}
		}
	} else {
		// 原有逻辑不变
		for _, skill := range sortedSkills {
			if skill.IsActive {
				promptParts = append(promptParts, skill.Content)
				result.UsedSkillIDs = append(result.UsedSkillIDs, skill.ID)
				result.UsedSkillNames = append(result.UsedSkillNames, skill.Name)
			}
		}
	}

	// 9. 填充动态上下文变量
	assembledPrompt := strings.Join(promptParts, "\n\n")

	// 打印填充前的 Prompt
	log.Printf("[SkillAssembler] ========== 填充动态上下文前 ==========")
	log.Printf("[SkillAssembler] Prompt Length: %d 字符", len(assembledPrompt))

	if dynamicContext != nil {
		log.Printf("[SkillAssembler] Dynamic Context:")
		log.Printf("[SkillAssembler]   - UserDescription: %s", truncateString(dynamicContext.UserDescription, 200))
		log.Printf("[SkillAssembler]   - WorkspaceID: %s", dynamicContext.WorkspaceID)
		log.Printf("[SkillAssembler]   - ModuleID: %d", dynamicContext.ModuleID)
		log.Printf("[SkillAssembler]   - UseCMDB: %v", dynamicContext.UseCMDB)
		log.Printf("[SkillAssembler]   - CMDBData Length: %d 字符", len(dynamicContext.CMDBData))
		log.Printf("[SkillAssembler]   - SchemaData Length: %d 字符", len(dynamicContext.SchemaData))

		assembledPrompt = a.fillDynamicContext(assembledPrompt, dynamicContext)
	}

	// 打印组装后的完整 Prompt
	log.Printf("[SkillAssembler] ========== 组装后完整 Prompt ==========")
	log.Printf("[SkillAssembler] Final Prompt Length: %d 字符", len(assembledPrompt))
	log.Printf("[SkillAssembler] Final Prompt Preview (前2000字符):\n%s", truncateString(assembledPrompt, 2000))
	log.Printf("[SkillAssembler] ========== Prompt 结束 ==========")

	result.Prompt = assembledPrompt
	result.AssemblyTimeMs = time.Since(startTime).Milliseconds()

	log.Printf("[SkillAssembler] 组装完成: 使用了 %d 个 Skills (%v), 耗时 %dms",
		len(result.UsedSkillIDs), result.UsedSkillNames, result.AssemblyTimeMs)

	return result, nil
}

// LoadSkill 根据名称加载单个 Skill
func (a *SkillAssembler) LoadSkill(name string) (*models.Skill, error) {
	// 先检查缓存
	if skill, ok := a.skillCache[name]; ok && time.Now().Before(a.skillCacheExpiry) {
		return skill, nil
	}

	var skill models.Skill
	if err := a.db.Where("name = ? AND is_active = ?", name, true).First(&skill).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("Skill '%s' 不存在或未激活", name)
		}
		return nil, fmt.Errorf("加载 Skill '%s' 失败: %w", name, err)
	}

	// 更新缓存
	a.skillCache[name] = &skill
	if a.skillCacheExpiry.IsZero() || time.Now().After(a.skillCacheExpiry) {
		a.skillCacheExpiry = time.Now().Add(5 * time.Minute)
	}

	return &skill, nil
}

// LoadSkillsByLayer 按层级加载 Skill 列表
func (a *SkillAssembler) LoadSkillsByLayer(layer models.SkillLayer) ([]*models.Skill, error) {
	var skills []models.Skill
	if err := a.db.Where("layer = ? AND is_active = ?", layer, true).
		Order("priority ASC").
		Find(&skills).Error; err != nil {
		return nil, fmt.Errorf("加载 %s 层 Skills 失败: %w", layer, err)
	}

	result := make([]*models.Skill, len(skills))
	for i := range skills {
		result[i] = &skills[i]
	}
	return result, nil
}

// GetOrGenerateModuleSkill 获取或生成 Module Skill
// 优先级：
// 1. 从 module_version_skills 表加载默认版本的 Skill（用户配置的）
// 2. 从 skills 表加载自动生成的 Skill
// 3. 自动生成新的 Skill
func (a *SkillAssembler) GetOrGenerateModuleSkill(moduleID uint) (*models.Skill, error) {
	// 1. 获取 Module 信息（包含 DefaultVersionID）
	var module models.Module
	if err := a.db.First(&module, moduleID).Error; err != nil {
		return nil, fmt.Errorf("Module 不存在: %w", err)
	}

	// 2. 如果有默认版本，优先从 module_version_skills 表加载
	if module.DefaultVersionID != nil && *module.DefaultVersionID != "" {
		log.Printf("[SkillAssembler] 尝试加载默认版本的 ModuleVersionSkill, moduleID=%d, defaultVersionID=%s",
			moduleID, *module.DefaultVersionID)

		var versionSkill models.ModuleVersionSkill
		err := a.db.Where("module_version_id = ? AND is_active = ?",
			*module.DefaultVersionID, true).First(&versionSkill).Error

		if err == nil {
			combinedContent := versionSkill.GetCombinedContent()
			if combinedContent != "" {
				log.Printf("[SkillAssembler] 成功加载 ModuleVersionSkill, versionID=%s, contentLength=%d",
					versionSkill.ModuleVersionID, len(combinedContent))

				// 将 ModuleVersionSkill 转换为 Skill 对象返回
				return &models.Skill{
					ID:          versionSkill.ID,
					Name:        fmt.Sprintf("module_%d_version_skill", moduleID),
					DisplayName: fmt.Sprintf("%s 配置知识（版本 Skill）", module.Name),
					Content:     combinedContent,
					Layer:       models.SkillLayerDomain,
					IsActive:    true,
					Priority:    100,
					SourceType:  models.SkillSourceModuleAuto,
					Metadata: models.SkillMetadata{
						Tags:        []string{"module", "version-skill", module.Provider},
						Description: fmt.Sprintf("从 Module %s 默认版本加载的 Skill", module.Name),
					},
				}, nil
			} else {
				log.Printf("[SkillAssembler] ModuleVersionSkill 内容为空, versionID=%s", versionSkill.ModuleVersionID)
			}
		} else if err != gorm.ErrRecordNotFound {
			log.Printf("[SkillAssembler] 查询 ModuleVersionSkill 失败: %v", err)
		} else {
			log.Printf("[SkillAssembler] 默认版本没有 ModuleVersionSkill 记录, versionID=%s", *module.DefaultVersionID)
		}
	} else {
		log.Printf("[SkillAssembler] Module 没有设置默认版本, moduleID=%d", moduleID)
	}

	// 3. Fallback: 从 skills 表加载自动生成的 Skill
	log.Printf("[SkillAssembler] Fallback: 尝试加载自动生成的 Module Skill")
	var existingSkill models.Skill
	skillName := fmt.Sprintf("module_%d_auto", moduleID)

	err := a.db.Where("name = ? AND source_module_id = ?", skillName, moduleID).First(&existingSkill).Error
	if err == nil {
		// 检查是否需要重新生成
		if a.moduleSkillGen.ShouldRegenerate(&existingSkill, moduleID) {
			log.Printf("[SkillAssembler] Module Skill 需要重新生成: %s", skillName)
			return a.regenerateModuleSkill(moduleID, &existingSkill)
		}
		log.Printf("[SkillAssembler] 使用自动生成的 Module Skill: %s", skillName)
		return &existingSkill, nil
	}

	if err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("查询 Module Skill 失败: %w", err)
	}

	// 4. 不存在，生成新的
	log.Printf("[SkillAssembler] 生成新的 Module Skill: moduleID=%d", moduleID)
	return a.generateNewModuleSkill(moduleID)
}

// EvaluateCondition 评估单个条件
func (a *SkillAssembler) EvaluateCondition(condition string, context *DynamicContext) bool {
	if context == nil {
		return false
	}

	// 简单的条件评估器
	// 支持格式: "variable == value" 或 "variable != value"
	condition = strings.TrimSpace(condition)

	// 解析条件
	var variable, operator, value string

	if strings.Contains(condition, "==") {
		parts := strings.SplitN(condition, "==", 2)
		if len(parts) == 2 {
			variable = strings.TrimSpace(parts[0])
			operator = "=="
			value = strings.TrimSpace(parts[1])
		}
	} else if strings.Contains(condition, "!=") {
		parts := strings.SplitN(condition, "!=", 2)
		if len(parts) == 2 {
			variable = strings.TrimSpace(parts[0])
			operator = "!="
			value = strings.TrimSpace(parts[1])
		}
	}

	if variable == "" || operator == "" {
		log.Printf("[SkillAssembler] 无法解析条件: %s", condition)
		return false
	}

	// 获取变量值
	var actualValue interface{}
	switch variable {
	case "use_cmdb":
		actualValue = context.UseCMDB
	case "module_id":
		actualValue = context.ModuleID
	case "workspace_id":
		actualValue = context.WorkspaceID
	case "organization_id":
		actualValue = context.OrganizationID
	default:
		// 尝试从 ExtraContext 获取
		if context.ExtraContext != nil {
			actualValue = context.ExtraContext[variable]
		}
	}

	// 比较值
	actualStr := fmt.Sprintf("%v", actualValue)
	expectedValue := strings.Trim(value, "\"'")

	switch operator {
	case "==":
		return actualStr == expectedValue || actualStr == value
	case "!=":
		return actualStr != expectedValue && actualStr != value
	}

	return false
}

// ========== 私有方法 ==========

// loadSkillsByNames 根据名称列表加载 Skills
func (a *SkillAssembler) loadSkillsByNames(names []string) ([]*models.Skill, error) {
	if len(names) == 0 {
		return nil, nil
	}

	var skills []models.Skill
	if err := a.db.Where("name IN ? AND is_active = ?", names, true).Find(&skills).Error; err != nil {
		return nil, err
	}

	// 按原始顺序排列
	skillMap := make(map[string]*models.Skill)
	for i := range skills {
		skillMap[skills[i].Name] = &skills[i]
	}

	result := make([]*models.Skill, 0, len(names))
	for _, name := range names {
		if skill, ok := skillMap[name]; ok {
			result = append(result, skill)
		} else {
			log.Printf("[SkillAssembler] 警告: Skill '%s' 不存在或未激活", name)
		}
	}

	return result, nil
}

// evaluateConditionalRules 评估条件规则
func (a *SkillAssembler) evaluateConditionalRules(rules []models.SkillConditionalRule, context *DynamicContext) []*models.Skill {
	var additionalSkills []*models.Skill

	for _, rule := range rules {
		if a.EvaluateCondition(rule.Condition, context) {
			log.Printf("[SkillAssembler] 条件 '%s' 满足，加载额外 Skills: %v", rule.Condition, rule.AddSkills)
			skills, err := a.loadSkillsByNames(rule.AddSkills)
			if err != nil {
				log.Printf("[SkillAssembler] 加载条件 Skills 失败: %v", err)
				continue
			}
			additionalSkills = append(additionalSkills, skills...)
		}
	}

	return additionalSkills
}

// sortSkills 按层级和优先级排序 Skills
func (a *SkillAssembler) sortSkills(skills []*models.Skill) []*models.Skill {
	// 去重
	seen := make(map[string]bool)
	unique := make([]*models.Skill, 0, len(skills))
	for _, skill := range skills {
		if skill != nil && !seen[skill.ID] {
			seen[skill.ID] = true
			unique = append(unique, skill)
		}
	}

	// 定义层级顺序
	layerOrder := map[models.SkillLayer]int{
		models.SkillLayerFoundation: 1,
		models.SkillLayerDomain:     2,
		models.SkillLayerTask:       3,
	}

	// 排序
	sort.Slice(unique, func(i, j int) bool {
		// 先按层级排序
		orderI := layerOrder[unique[i].Layer]
		orderJ := layerOrder[unique[j].Layer]
		if orderI != orderJ {
			return orderI < orderJ
		}
		// 同层级内，按 sourceType 分组（manual → hybrid → module_auto）
		sourceOrderI := a.sourceTypeOrder(unique[i].SourceType)
		sourceOrderJ := a.sourceTypeOrder(unique[j].SourceType)
		if sourceOrderI != sourceOrderJ {
			return sourceOrderI < sourceOrderJ
		}
		// 同 sourceType 内按优先级排序
		return unique[i].Priority < unique[j].Priority
	})

	return unique
}

// fillDynamicContext 填充动态上下文变量
func (a *SkillAssembler) fillDynamicContext(prompt string, context *DynamicContext) string {
	if context == nil {
		return prompt
	}

	// 替换变量占位符
	replacements := map[string]string{
		"{user_description}":   context.UserDescription,
		"{user_request}":       context.UserDescription,
		"{workspace_id}":       context.WorkspaceID,
		"{organization_id}":    context.OrganizationID,
		"{cmdb_data}":          context.CMDBData,
		"{schema_data}":        context.SchemaData,
		"{schema_constraints}": context.SchemaData,
	}

	result := prompt
	for placeholder, value := range replacements {
		result = strings.ReplaceAll(result, placeholder, value)
	}

	// 处理 ExtraContext 中的变量
	if context.ExtraContext != nil {
		for key, value := range context.ExtraContext {
			placeholder := fmt.Sprintf("{%s}", key)
			result = strings.ReplaceAll(result, placeholder, fmt.Sprintf("%v", value))
		}
	}

	return result
}

// generateNewModuleSkill 生成新的 Module Skill
func (a *SkillAssembler) generateNewModuleSkill(moduleID uint) (*models.Skill, error) {
	// 获取 Module 信息
	var module models.Module
	if err := a.db.First(&module, moduleID).Error; err != nil {
		return nil, fmt.Errorf("Module 不存在: %w", err)
	}

	// 获取 Schema
	var schema models.Schema
	if err := a.db.Where("module_id = ? AND status = ?", moduleID, "active").First(&schema).Error; err != nil {
		log.Printf("[SkillAssembler] Module %d 没有活跃的 Schema，跳过 Skill 生成", moduleID)
		return nil, nil
	}

	// 生成 Skill 内容
	content, err := a.moduleSkillGen.GenerateSkillContent(&module, &schema)
	if err != nil {
		return nil, fmt.Errorf("生成 Skill 内容失败: %w", err)
	}

	// 创建 Skill 记录
	skill := &models.Skill{
		ID:             uuid.New().String(),
		Name:           fmt.Sprintf("module_%d_auto", moduleID),
		DisplayName:    fmt.Sprintf("%s 配置知识", module.Name),
		Layer:          models.SkillLayerDomain,
		Content:        content,
		Version:        "1.0.0",
		IsActive:       true,
		Priority:       100, // Module Skill 优先级较低
		SourceType:     models.SkillSourceModuleAuto,
		SourceModuleID: &moduleID,
		Metadata: models.SkillMetadata{
			Tags:        []string{"module", "auto-generated", module.Provider},
			Description: fmt.Sprintf("从 Module %s 自动生成的配置知识", module.Name),
		},
	}

	if err := a.db.Create(skill).Error; err != nil {
		return nil, fmt.Errorf("保存 Module Skill 失败: %w", err)
	}

	log.Printf("[SkillAssembler] 成功生成 Module Skill: %s", skill.Name)
	return skill, nil
}

// regenerateModuleSkill 重新生成 Module Skill
func (a *SkillAssembler) regenerateModuleSkill(moduleID uint, existingSkill *models.Skill) (*models.Skill, error) {
	// 获取 Module 信息
	var module models.Module
	if err := a.db.First(&module, moduleID).Error; err != nil {
		return nil, fmt.Errorf("Module 不存在: %w", err)
	}

	// 获取 Schema
	var schema models.Schema
	if err := a.db.Where("module_id = ? AND status = ?", moduleID, "active").First(&schema).Error; err != nil {
		return existingSkill, nil // 没有 Schema，返回现有的
	}

	// 生成新内容
	content, err := a.moduleSkillGen.GenerateSkillContent(&module, &schema)
	if err != nil {
		return existingSkill, nil // 生成失败，返回现有的
	}

	// 更新版本号
	newVersion := incrementVersion(existingSkill.Version)

	// 更新 Skill
	existingSkill.Content = content
	existingSkill.Version = newVersion
	existingSkill.UpdatedAt = time.Now()

	if err := a.db.Save(existingSkill).Error; err != nil {
		return nil, fmt.Errorf("更新 Module Skill 失败: %w", err)
	}

	log.Printf("[SkillAssembler] 成功更新 Module Skill: %s -> %s", existingSkill.Name, newVersion)
	return existingSkill, nil
}

// LogSkillUsage 记录 Skill 使用日志
func (a *SkillAssembler) LogSkillUsage(
	skillIDs []string,
	capability string,
	workspaceID string,
	userID string,
	moduleID *uint,
	aiModel string,
	executionTimeMs int,
) error {
	log := &models.SkillUsageLog{
		ID:              uuid.New().String(),
		SkillIDs:        skillIDs,
		Capability:      capability,
		WorkspaceID:     workspaceID,
		UserID:          userID,
		ModuleID:        moduleID,
		AIModel:         aiModel,
		ExecutionTimeMs: executionTimeMs,
	}

	return a.db.Create(log).Error
}

// ClearCache 清除缓存
func (a *SkillAssembler) ClearCache() {
	a.skillCache = make(map[string]*models.Skill)
	a.skillCacheExpiry = time.Time{}
}

// discoverDomainSkillsFromContent 从 Task Skill 内容中自动发现 Domain Skills
// 解析 @require-domain 声明
func (a *SkillAssembler) discoverDomainSkillsFromContent(content string, context *DynamicContext) []*models.Skill {
	var discoveredSkills []*models.Skill
	var skillNames []string

	// 1. 解析 @require-domain: skill_name
	reSimple := regexp.MustCompile(`@require-domain:\s*(\w+)`)
	matchesSimple := reSimple.FindAllStringSubmatch(content, -1)
	for _, match := range matchesSimple {
		if len(match) >= 2 {
			skillName := strings.TrimSpace(match[1])
			skillNames = append(skillNames, skillName)
			log.Printf("[SkillAssembler] 发现 @require-domain: %s", skillName)
		}
	}

	// 2. 解析 @require-domain-if: condition -> skill_name
	reConditional := regexp.MustCompile(`@require-domain-if:\s*(.+?)\s*->\s*(\w+)`)
	matchesConditional := reConditional.FindAllStringSubmatch(content, -1)
	for _, match := range matchesConditional {
		if len(match) >= 3 {
			condition := strings.TrimSpace(match[1])
			skillName := strings.TrimSpace(match[2])
			log.Printf("[SkillAssembler] 发现 @require-domain-if: %s -> %s", condition, skillName)

			// 评估条件
			if a.EvaluateCondition(condition, context) {
				skillNames = append(skillNames, skillName)
				log.Printf("[SkillAssembler] 条件 '%s' 满足，加载 %s", condition, skillName)
			} else {
				log.Printf("[SkillAssembler] 条件 '%s' 不满足，跳过 %s", condition, skillName)
			}
		}
	}

	// 3. 解析 @require-domain-tag: tag_name
	reTag := regexp.MustCompile(`@require-domain-tag:\s*(\w+)`)
	matchesTag := reTag.FindAllStringSubmatch(content, -1)
	for _, match := range matchesTag {
		if len(match) >= 2 {
			tagName := strings.TrimSpace(match[1])
			log.Printf("[SkillAssembler] 发现 @require-domain-tag: %s", tagName)

			// 按标签加载 Domain Skills
			tagSkills := a.loadDomainSkillsByTag(tagName)
			discoveredSkills = append(discoveredSkills, tagSkills...)
		}
	}

	// 4. 加载按名称指定的 Skills
	if len(skillNames) > 0 {
		// 去重
		uniqueNames := make([]string, 0, len(skillNames))
		seen := make(map[string]bool)
		for _, name := range skillNames {
			if !seen[name] {
				seen[name] = true
				uniqueNames = append(uniqueNames, name)
			}
		}

		namedSkills, err := a.loadSkillsByNames(uniqueNames)
		if err != nil {
			log.Printf("[SkillAssembler] 加载自动发现的 Domain Skills 失败: %v", err)
		} else {
			discoveredSkills = append(discoveredSkills, namedSkills...)
		}
	}

	return discoveredSkills
}

// discoverDomainSkillsByTags 根据 Task Skill 的 domain_tags 发现 Domain Skills
// 从 Task Skill 的 metadata.domain_tags 中提取标签，查找 tags 与之有交集的 Domain Skills
func (a *SkillAssembler) discoverDomainSkillsByTags(taskSkill *models.Skill) []*models.Skill {
	if taskSkill == nil {
		return nil
	}

	// 从 Task Skill 的 metadata 中提取 domain_tags
	domainTags := taskSkill.Metadata.DomainTags
	if len(domainTags) == 0 {
		log.Printf("[SkillAssembler] Task Skill '%s' 没有定义 domain_tags", taskSkill.Name)
		return nil
	}

	log.Printf("[SkillAssembler] Task Skill '%s' 的 domain_tags: %v", taskSkill.Name, domainTags)

	// 查询 tags 与 domain_tags 有交集的 Domain Skills
	// 使用 PostgreSQL 的 JSONB 操作符
	var skills []models.Skill

	// 构建查询条件：metadata->'tags' 包含任意一个 domain_tag
	// 由于 GORM 对 JSONB 数组操作支持有限，使用 LIKE 模糊匹配
	var conditions []string
	var args []interface{}
	for _, tag := range domainTags {
		conditions = append(conditions, "metadata->>'tags' LIKE ?")
		args = append(args, "%"+tag+"%")
	}

	query := a.db.Where("layer = ? AND is_active = ?", models.SkillLayerDomain, true)
	if len(conditions) > 0 {
		query = query.Where("("+strings.Join(conditions, " OR ")+")", args...)
	}

	err := query.Order("priority ASC").Find(&skills).Error
	if err != nil {
		log.Printf("[SkillAssembler] 通过标签发现 Domain Skills 失败: %v", err)
		return nil
	}

	log.Printf("[SkillAssembler] 通过标签匹配发现了 %d 个 Domain Skills", len(skills))
	for _, skill := range skills {
		log.Printf("[SkillAssembler]   - %s (tags: %v)", skill.Name, skill.Metadata.Tags)
	}

	result := make([]*models.Skill, len(skills))
	for i := range skills {
		result[i] = &skills[i]
	}
	return result
}

// loadDomainSkillsByTag 根据标签加载 Domain Skills
func (a *SkillAssembler) loadDomainSkillsByTag(tagName string) []*models.Skill {
	var skills []models.Skill

	// 查询 Domain 层且标签包含指定值的 Skills
	// metadata 是 JSONB 字段，tags 是数组
	err := a.db.Where("layer = ? AND is_active = ?", models.SkillLayerDomain, true).
		Where("metadata->>'tags' LIKE ?", "%"+tagName+"%").
		Order("priority ASC").
		Find(&skills).Error

	if err != nil {
		log.Printf("[SkillAssembler] 按标签 '%s' 加载 Domain Skills 失败: %v", tagName, err)
		return nil
	}

	log.Printf("[SkillAssembler] 按标签 '%s' 加载了 %d 个 Domain Skills", tagName, len(skills))

	result := make([]*models.Skill, len(skills))
	for i := range skills {
		result[i] = &skills[i]
	}
	return result
}

// ========== 元规则相关方法 ==========

const defaultMetaRulesTemplate = `## 元规则

### 优先级层级（从高到低）
1. **Foundation Layer** — 安全基线、合规规则、平台级约束，不可违反
2. **Domain Layer - Best Practice** — 领域最佳实践，在 Module 约束范围内的强推荐
3. **Domain Layer - Module Constraints** — Module Schema 事实（参数名、类型、枚举值、必填项、取值范围），是绝对边界
4. **Task Layer** — 当前任务的工作流指令
5. **用户需求** — 在以上约束内尽量满足

### 冲突解决原则
- Foundation 与任何层冲突 → Foundation 胜出
- Best Practice 与 Module Constraints 冲突 → Module Constraints 胜出（不可推荐 schema 不允许的值）
- Best Practice 不与 Module Constraints 冲突 → 遵循 Best Practice
- 用户需求与 Foundation 或 Module Constraints 冲突 → 忽略冲突部分并说明原因

### 已加载 Skills
{skill_manifest}
`

// buildSkillManifest 生成已加载 Skill 清单表格
func (a *SkillAssembler) buildSkillManifest(skills []*models.Skill) string {
	var sb strings.Builder
	sb.WriteString("| # | Name | Layer | Type | Version | Priority |\n")
	sb.WriteString("|---|------|-------|------|---------|----------|\n")

	idx := 0
	for _, skill := range skills {
		if !skill.IsActive {
			continue
		}
		idx++
		skillType := string(skill.SourceType)
		if skill.SourceType == models.SkillSourceModuleAuto {
			skillType = "module_constraint"
		}
		sb.WriteString(fmt.Sprintf("| %d | %s | %s | %s | %s | %d |\n",
			idx, skill.Name, skill.Layer, skillType, skill.Version, skill.Priority))
	}

	return sb.String()
}

// buildMetaRulesPreamble 生成元规则段落
func (a *SkillAssembler) buildMetaRulesPreamble(config *models.MetaRulesConfig, skills []*models.Skill) string {
	template := defaultMetaRulesTemplate
	if config != nil && config.Template != "" {
		template = config.Template
	}

	manifest := a.buildSkillManifest(skills)
	return strings.ReplaceAll(template, "{skill_manifest}", manifest)
}

// getSectionHeader 根据 Skill 的 Layer 和 SourceType 返回分段标题
func (a *SkillAssembler) getSectionHeader(skill *models.Skill) string {
	switch skill.Layer {
	case models.SkillLayerFoundation:
		return "[Foundation Layer]"
	case models.SkillLayerDomain:
		if skill.SourceType == models.SkillSourceModuleAuto {
			return "[Domain Layer - Module Constraints]"
		}
		return "[Domain Layer - Best Practice]"
	case models.SkillLayerTask:
		return "[Task Layer]"
	default:
		return "[Unknown Layer]"
	}
}

// buildSectionedPrompt 带分段标记的组装
func (a *SkillAssembler) buildSectionedPrompt(skills []*models.Skill) string {
	var sb strings.Builder
	currentSection := ""

	for _, skill := range skills {
		if !skill.IsActive {
			continue
		}

		section := a.getSectionHeader(skill)
		if section != currentSection {
			if currentSection != "" {
				sb.WriteString("\n\n")
			}
			sb.WriteString(fmt.Sprintf("## %s\n\n", section))
			currentSection = section
		} else {
			sb.WriteString("\n\n")
		}

		sb.WriteString(fmt.Sprintf("--- skill: %s (v%s) ---\n", skill.Name, skill.Version))
		sb.WriteString(skill.Content)
	}

	return sb.String()
}

// sourceTypeOrder 返回 SourceType 的排序权重
func (a *SkillAssembler) sourceTypeOrder(st models.SkillSourceType) int {
	switch st {
	case models.SkillSourceManual:
		return 1
	case models.SkillSourceHybrid:
		return 2
	case models.SkillSourceModuleAuto:
		return 3
	default:
		return 4
	}
}

// ========== 辅助函数 ==========

// truncateString 截断字符串到指定长度
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// incrementVersion 递增版本号
func incrementVersion(version string) string {
	// 简单的版本号递增：1.0.0 -> 1.0.1
	re := regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)$`)
	matches := re.FindStringSubmatch(version)
	if len(matches) == 4 {
		var major, minor, patch int
		fmt.Sscanf(matches[1], "%d", &major)
		fmt.Sscanf(matches[2], "%d", &minor)
		fmt.Sscanf(matches[3], "%d", &patch)
		return fmt.Sprintf("%d.%d.%d", major, minor, patch+1)
	}
	return "1.0.1"
}
