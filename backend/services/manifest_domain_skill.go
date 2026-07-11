package services

import (
	"encoding/json"
	"fmt"
	"iac-platform/internal/models"
	"log"
	"strings"

	"gorm.io/gorm"
)

// manifestModuleInfo 传给 AI 选择的 module 信息
type manifestModuleInfo struct {
	Name        string   `json:"name"`
	Source      string   `json:"source"`
	Versions    []string `json:"versions,omitempty"`
	Description string   `json:"description,omitempty"`
}

// selectedModule AI 选中的 module(含版本)
type selectedModule struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// domainAndModuleSelectionResult AI 同时选择 domain skills + modules 的结果
type domainAndModuleSelectionResult struct {
	SelectedSkills  []string         `json:"selected_skills"`
	SelectedModules []selectedModule `json:"selected_modules"`
	Reason          string           `json:"reason"`
}

// manifestDomainSkillSelector manifest 专用的 Domain Skill + Module AI 选择器。
//
// 与 form 的 selectDomainSkillsByAI 等价(AI 按 skill description 选),但:
//   - 独立实现,不复用 AICMDBSkillService(方案 B:不碰 form,避免回归)
//   - 中性 prompt,不排除 CMDB 资源类 skill(form 的 phase=second 会排除,manifest 不要)
//   - 同时选择 domain skills 和 modules,一次 AI 调用完成两个选择。
//
// 仅在 aiConfig.UseOptimized==true 时由调用方启用(遵守开关)。
type manifestDomainSkillSelector struct {
	db            *gorm.DB
	configService *AIConfigService
	aiFormService *AIFormService
}

func newManifestDomainSkillSelector(db *gorm.DB) *manifestDomainSkillSelector {
	return &manifestDomainSkillSelector{
		db:            db,
		configService: NewAIConfigService(db),
		aiFormService: NewAIFormService(db),
	}
}

// Select 让 AI 根据 description 同时选择相关 domain skill 和 module。
// modules 为可选的 module 候选列表,传 nil 则只选择 domain skill。
// 返回 (domainSkillNames, selectedModules, error)。失败/无配置返回 error,调用方降级。
func (s *manifestDomainSkillSelector) Select(description string, modules []manifestModuleInfo) ([]string, []selectedModule, error) {
	// 1. 取所有 active domain skill 的 name+description。
	var skills []models.Skill
	if err := s.db.Where("layer = ? AND is_active = ?", models.SkillLayerDomain, true).
		Where("source_type <> ?", models.SkillSourceModuleAuto).
		Select("name", "description").
		Order("priority ASC").
		Find(&skills).Error; err != nil {
		return nil, nil, fmt.Errorf("获取 domain skill 列表失败: %w", err)
	}
	if len(skills) == 0 && len(modules) == 0 {
		return []string{}, []selectedModule{}, nil
	}

	// 2. 取 domain_skill_selection 配置
	aiConfig, err := s.configService.GetConfigForCapability("domain_skill_selection")
	if err != nil || aiConfig == nil {
		return nil, nil, fmt.Errorf("domain_skill_selection 配置不可用: %w", err)
	}

	// 3. 构建 prompt(domain skills + modules)
	prompt := s.buildPrompt(description, skills, modules)

	// 4. 调 AI
	result, err := s.aiFormService.callAIForCapability("domain_skill_selection", aiConfig, prompt)
	if err != nil {
		return nil, nil, fmt.Errorf("AI 调用失败: %w", err)
	}

	// 5. 解析
	jsonStr := extractJSON(result)
	if jsonStr == "" {
		return nil, nil, fmt.Errorf("无法从 AI 响应提取 JSON")
	}
	var selection domainAndModuleSelectionResult
	if e := json.Unmarshal([]byte(jsonStr), &selection); e != nil {
		if e2 := json.Unmarshal([]byte(fixIncompleteJSON(jsonStr)), &selection); e2 != nil {
			return nil, nil, fmt.Errorf("解析选择结果失败: %w", e)
		}
	}

	// 6. 用库里合法名过滤
	validSkills := s.filterValidSkills(selection.SelectedSkills, skills)
	validModules := filterValidModules(selection.SelectedModules, modules)
	log.Printf("[manifestDomainSkillSelector] AI 选择了 %d 个 domain skill: %v, %d 个 module: %v (原因: %s)",
		len(validSkills), validSkills, len(validModules), formatSelectedModules(validModules), selection.Reason)
	return validSkills, validModules, nil
}

// buildPrompt 构建 domain skill + module 选择 prompt
func (s *manifestDomainSkillSelector) buildPrompt(description string, skills []models.Skill, modules []manifestModuleInfo) string {
	var skillList strings.Builder
	for i, sk := range skills {
		desc := sk.Description
		if desc == "" {
			desc = "(无描述)"
		}
		fmt.Fprintf(&skillList, "%d. %s - %s\n", i+1, sk.Name, desc)
	}

	var moduleSection string
	if len(modules) > 0 {
		var moduleList strings.Builder
		for i, m := range modules {
			desc := m.Description
			if desc == "" {
				desc = "(无描述)"
			}
			versions := ""
			if len(m.Versions) > 0 {
				versions = fmt.Sprintf(", 可用版本: %s", strings.Join(m.Versions, ", "))
			}
			fmt.Fprintf(&moduleList, "%d. %s (source: %s%s) - %s\n", i+1, m.Name, m.Source, versions, desc)
		}
		moduleSection = fmt.Sprintf(`

【可用 Modules】
%s
从上面 Modules 中选出用户需求涉及的 module。只选用户需求明确涉及的,无关的不选。
如果用户指定了版本号,在 version 字段填写;未指定则留空(将使用默认版本)。`, moduleList.String())
	}

	outputFormat := `{"selected_skills": ["skill_name_1"], "selected_modules": [{"name": "module_name_1", "version": "5.14.0"}], "reason": "选择理由"}`
	if len(modules) == 0 {
		outputFormat = `{"selected_skills": ["skill_name_1"], "reason": "选择理由"}`
	}

	return fmt.Sprintf(`你是 IaC 平台的 Skill 选择助手。根据下面的 Terraform 内容/需求,从可用 Domain Skills 中选出**所有相关**的 skill。%s

【Domain Skill 选择原则】
- 选出与内容相关的所有 domain skill,包括 CMDB 资源类(若内容涉及现有资源引用/匹配)、安全类、策略类等。
- 不要排除任何类别。无关的不选。
- 只能从下面列表里选,用 skill 的 name。

【内容/需求】
%s

【可用 Domain Skills】
%s

【输出】仅输出 JSON,格式:
%s`, moduleSection, description, skillList.String(), outputFormat)
}

// filterValidSkills 只保留库里存在的 skill 名
func (s *manifestDomainSkillSelector) filterValidSkills(selected []string, all []models.Skill) []string {
	nameSet := make(map[string]bool, len(all))
	for _, sk := range all {
		nameSet[sk.Name] = true
	}
	out := make([]string, 0, len(selected))
	for _, n := range selected {
		if nameSet[n] {
			out = append(out, n)
		}
	}
	return out
}

// filterValidModules 只保留候选列表里存在的 module(校验 name,保留 version)
func filterValidModules(selected []selectedModule, modules []manifestModuleInfo) []selectedModule {
	nameSet := make(map[string]bool, len(modules))
	for _, m := range modules {
		nameSet[m.Name] = true
	}
	out := make([]selectedModule, 0, len(selected))
	for _, s := range selected {
		if nameSet[s.Name] {
			out = append(out, s)
		}
	}
	return out
}

// formatSelectedModules 格式化 selectedModule 列表用于日志
func formatSelectedModules(modules []selectedModule) string {
	if len(modules) == 0 {
		return "[]"
	}
	parts := make([]string, len(modules))
	for i, m := range modules {
		if m.Version != "" {
			parts[i] = fmt.Sprintf("%s@%s", m.Name, m.Version)
		} else {
			parts[i] = m.Name
		}
	}
	return fmt.Sprintf("[%s]", strings.Join(parts, ", "))
}
