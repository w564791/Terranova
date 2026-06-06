package services

import (
	"encoding/json"
	"fmt"
	"iac-platform/internal/models"
	"log"
	"strings"

	"gorm.io/gorm"
)

// manifestDomainSkillSelector manifest 专用的 Domain Skill AI 选择器。
//
// 与 form 的 selectDomainSkillsByAI 等价(AI 按 skill description 选),但:
//   - 独立实现,不复用 AICMDBSkillService(方案 B:不碰 form,避免回归)
//   - 中性 prompt,不排除 CMDB 资源类 skill(form 的 phase=second 会排除,manifest 不要)
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

// Select 让 AI 根据 description 选择相关 domain skill。
// 返回合法(存在于库)的 skill 名列表。失败/无配置返回 error,调用方降级。
func (s *manifestDomainSkillSelector) Select(description string) ([]string, error) {
	// 1. 取所有 active domain skill 的 name+description
	var skills []models.Skill
	if err := s.db.Where("layer = ? AND is_active = ?", models.SkillLayerDomain, true).
		Select("name", "description").
		Order("priority ASC").
		Find(&skills).Error; err != nil {
		return nil, fmt.Errorf("获取 domain skill 列表失败: %w", err)
	}
	if len(skills) == 0 {
		return []string{}, nil
	}

	// 2. 取 domain_skill_selection 配置
	aiConfig, err := s.configService.GetConfigForCapability("domain_skill_selection")
	if err != nil || aiConfig == nil {
		return nil, fmt.Errorf("domain_skill_selection 配置不可用: %w", err)
	}

	// 3. 构建中性 prompt(不排除任何类别)
	prompt := s.buildPrompt(description, skills)

	// 4. 调 AI
	result, err := s.aiFormService.callAI(aiConfig, prompt)
	if err != nil {
		return nil, fmt.Errorf("AI 调用失败: %w", err)
	}

	// 5. 解析(复用包内 DomainSkillSelectionResult + extractJSON/fixIncompleteJSON)
	jsonStr := extractJSON(result)
	if jsonStr == "" {
		return nil, fmt.Errorf("无法从 AI 响应提取 JSON")
	}
	var selection DomainSkillSelectionResult
	if e := json.Unmarshal([]byte(jsonStr), &selection); e != nil {
		if e2 := json.Unmarshal([]byte(fixIncompleteJSON(jsonStr)), &selection); e2 != nil {
			return nil, fmt.Errorf("解析 domain skill 选择结果失败: %w", e)
		}
	}

	// 6. 用库里合法名过滤
	valid := s.filterValidSkills(selection.SelectedSkills, skills)
	log.Printf("[manifestDomainSkillSelector] AI 选择了 %d 个 domain skill: %v (原因: %s)",
		len(valid), valid, selection.Reason)
	return valid, nil
}

// buildPrompt 构建中性 domain skill 选择 prompt(不排除 CMDB 等任何类别)
func (s *manifestDomainSkillSelector) buildPrompt(description string, skills []models.Skill) string {
	var list strings.Builder
	for i, sk := range skills {
		desc := sk.Description
		if desc == "" {
			desc = "(无描述)"
		}
		fmt.Fprintf(&list, "%d. %s - %s\n", i+1, sk.Name, desc)
	}

	return fmt.Sprintf(`你是 IaC 平台的 Skill 选择助手。根据下面的 Terraform 内容/需求,从可用 Domain Skills 中选出**所有相关**的 skill。

【选择原则】
- 选出与内容相关的所有 domain skill,包括 CMDB 资源类(若内容涉及现有资源引用/匹配)、安全类、策略类等。
- 不要排除任何类别。无关的不选。
- 只能从下面列表里选,用 skill 的 name。

【内容/需求】
%s

【可用 Domain Skills】
%s

【输出】仅输出 JSON,格式:
{"selected_skills": ["skill_name_1", "skill_name_2"], "reason": "选择理由"}`, description, list.String())
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
