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

// ModuleSkillGenerator Module Skill 生成器
// 负责从 Module 和 Schema 自动生成 Skill 内容
type ModuleSkillGenerator struct {
	db *gorm.DB
}

// NewModuleSkillGenerator 创建 ModuleSkillGenerator 实例
func NewModuleSkillGenerator(db *gorm.DB) *ModuleSkillGenerator {
	return &ModuleSkillGenerator{db: db}
}

// GenerateSkillContent 生成 Skill 内容
func (g *ModuleSkillGenerator) GenerateSkillContent(module *models.Module, schema *models.Schema) (string, error) {
	if module == nil {
		return "", fmt.Errorf("Module 不能为空")
	}

	var sb strings.Builder

	// 1. 模块基本信息
	sb.WriteString(fmt.Sprintf("## %s 模块配置知识\n\n", module.Name))
	sb.WriteString(fmt.Sprintf("**模块名称**: %s\n", module.Name))
	sb.WriteString(fmt.Sprintf("**Provider**: %s\n", module.Provider))
	if module.Description != "" {
		sb.WriteString(fmt.Sprintf("**描述**: %s\n", module.Description))
	}
	sb.WriteString("\n")

	// 2. Schema 约束
	if schema != nil && schema.OpenAPISchema != nil {
		constraints := g.ExtractSchemaConstraints(schema.OpenAPISchema)
		if constraints != "" {
			sb.WriteString("### 配置约束\n\n")
			sb.WriteString(constraints)
			sb.WriteString("\n")
		}
	}

	// 3. Demo 示例
	demos := g.ExtractDemoExamples(module.ID)
	if demos != "" {
		sb.WriteString("### 配置示例\n\n")
		sb.WriteString(demos)
		sb.WriteString("\n")
	}

	// 4. 生成规则
	sb.WriteString("### 生成规则\n\n")
	sb.WriteString("1. 严格遵循上述 Schema 约束\n")
	sb.WriteString("2. 参考配置示例的格式和风格\n")
	sb.WriteString("3. 必填字段必须提供值\n")
	sb.WriteString("4. 枚举字段只能使用允许的值\n")
	sb.WriteString("5. 数值字段需要在指定范围内\n")

	return sb.String(), nil
}

// ExtractSchemaConstraints 从 OpenAPI Schema 提取约束信息
func (g *ModuleSkillGenerator) ExtractSchemaConstraints(openAPISchema interface{}) string {
	if openAPISchema == nil {
		return ""
	}

	// 将 schema 转换为 map
	var schemaMap map[string]interface{}
	switch v := openAPISchema.(type) {
	case map[string]interface{}:
		schemaMap = v
	case string:
		if err := json.Unmarshal([]byte(v), &schemaMap); err != nil {
			log.Printf("[ModuleSkillGenerator] 解析 Schema 失败: %v", err)
			return ""
		}
	default:
		// 尝试 JSON 序列化再反序列化
		data, err := json.Marshal(openAPISchema)
		if err != nil {
			return ""
		}
		if err := json.Unmarshal(data, &schemaMap); err != nil {
			return ""
		}
	}

	var sb strings.Builder

	// 提取 properties
	if properties, ok := schemaMap["properties"].(map[string]interface{}); ok {
		// 获取必填字段
		requiredFields := make(map[string]bool)
		if required, ok := schemaMap["required"].([]interface{}); ok {
			for _, r := range required {
				if s, ok := r.(string); ok {
					requiredFields[s] = true
				}
			}
		}

		sb.WriteString("| 字段名 | 类型 | 必填 | 约束 | 描述 |\n")
		sb.WriteString("|--------|------|------|------|------|\n")

		for fieldName, fieldSchema := range properties {
			fieldMap, ok := fieldSchema.(map[string]interface{})
			if !ok {
				continue
			}

			// 类型
			fieldType := "string"
			if t, ok := fieldMap["type"].(string); ok {
				fieldType = t
			}

			// 必填
			required := "否"
			if requiredFields[fieldName] {
				required = "是"
			}

			// 约束
			constraints := g.extractFieldConstraints(fieldMap)

			// 描述
			description := ""
			if d, ok := fieldMap["description"].(string); ok {
				description = strings.ReplaceAll(d, "|", "\\|")
				description = strings.ReplaceAll(description, "\n", " ")
				if len(description) > 50 {
					description = description[:50] + "..."
				}
			}

			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
				fieldName, fieldType, required, constraints, description))
		}
	}

	return sb.String()
}

// extractFieldConstraints 提取字段约束
func (g *ModuleSkillGenerator) extractFieldConstraints(fieldMap map[string]interface{}) string {
	var constraints []string

	// 枚举
	if enum, ok := fieldMap["enum"].([]interface{}); ok {
		values := make([]string, 0, len(enum))
		for _, v := range enum {
			values = append(values, fmt.Sprintf("%v", v))
		}
		if len(values) <= 5 {
			constraints = append(constraints, fmt.Sprintf("枚举: %s", strings.Join(values, ", ")))
		} else {
			constraints = append(constraints, fmt.Sprintf("枚举: %s...(共%d个)", strings.Join(values[:5], ", "), len(values)))
		}
	}

	// 最小值
	if min, ok := fieldMap["minimum"].(float64); ok {
		constraints = append(constraints, fmt.Sprintf("最小: %.0f", min))
	}

	// 最大值
	if max, ok := fieldMap["maximum"].(float64); ok {
		constraints = append(constraints, fmt.Sprintf("最大: %.0f", max))
	}

	// 最小长度
	if minLen, ok := fieldMap["minLength"].(float64); ok {
		constraints = append(constraints, fmt.Sprintf("最短: %.0f", minLen))
	}

	// 最大长度
	if maxLen, ok := fieldMap["maxLength"].(float64); ok {
		constraints = append(constraints, fmt.Sprintf("最长: %.0f", maxLen))
	}

	// 正则模式
	if pattern, ok := fieldMap["pattern"].(string); ok {
		if len(pattern) <= 20 {
			constraints = append(constraints, fmt.Sprintf("格式: %s", pattern))
		} else {
			constraints = append(constraints, "有格式要求")
		}
	}

	// 默认值
	if defaultVal, ok := fieldMap["default"]; ok {
		constraints = append(constraints, fmt.Sprintf("默认: %v", defaultVal))
	}

	if len(constraints) == 0 {
		return "-"
	}
	return strings.Join(constraints, "; ")
}

// ExtractDemoExamples 提取 Demo 示例
func (g *ModuleSkillGenerator) ExtractDemoExamples(moduleID uint) string {
	var demos []models.ModuleDemo
	if err := g.db.Where("module_id = ? AND is_active = ?", moduleID, true).
		Preload("CurrentVersion").
		Order("created_at DESC").
		Limit(3).
		Find(&demos).Error; err != nil {
		log.Printf("[ModuleSkillGenerator] 查询 Demo 失败: %v", err)
		return ""
	}

	if len(demos) == 0 {
		return ""
	}

	var sb strings.Builder
	for i, demo := range demos {
		sb.WriteString(fmt.Sprintf("#### 示例 %d: %s\n\n", i+1, demo.Name))
		if demo.Description != "" {
			sb.WriteString(fmt.Sprintf("**场景**: %s\n\n", demo.Description))
		}

		// 格式化配置（从 CurrentVersion.ConfigData 获取）
		if demo.CurrentVersion != nil && demo.CurrentVersion.ConfigData != nil {
			configJSON, err := json.MarshalIndent(demo.CurrentVersion.ConfigData, "", "  ")
			if err == nil {
				sb.WriteString("```json\n")
				sb.WriteString(string(configJSON))
				sb.WriteString("\n```\n\n")
			}
		}
	}

	return sb.String()
}

// ShouldRegenerate 判断是否需要重新生成 Skill
func (g *ModuleSkillGenerator) ShouldRegenerate(skill *models.Skill, moduleID uint) bool {
	if skill == nil {
		return true
	}

	// 检查 Module 更新时间
	var module models.Module
	if err := g.db.First(&module, moduleID).Error; err != nil {
		return false
	}

	// 如果 Module 更新时间晚于 Skill 更新时间，需要重新生成
	if module.UpdatedAt.After(skill.UpdatedAt) {
		return true
	}

	// 检查 Schema 更新时间
	var schema models.Schema
	if err := g.db.Where("module_id = ? AND status = ?", moduleID, "active").First(&schema).Error; err == nil {
		if schema.UpdatedAt.After(skill.UpdatedAt) {
			return true
		}
	}

	// 检查 Demo 更新时间
	var latestDemo models.ModuleDemo
	if err := g.db.Where("module_id = ?", moduleID).Order("updated_at DESC").First(&latestDemo).Error; err == nil {
		if latestDemo.UpdatedAt.After(skill.UpdatedAt) {
			return true
		}
	}

	// 如果 Skill 超过 24 小时未更新，也重新生成
	if time.Since(skill.UpdatedAt) > 24*time.Hour {
		return true
	}

	return false
}

// GenerateSkillFromModule 从 Module 生成完整的 Skill 对象
func (g *ModuleSkillGenerator) GenerateSkillFromModule(moduleID uint) (*models.Skill, error) {
	// 获取 Module
	var module models.Module
	if err := g.db.First(&module, moduleID).Error; err != nil {
		return nil, fmt.Errorf("Module 不存在: %w", err)
	}

	// 获取 Schema
	var schema models.Schema
	if err := g.db.Where("module_id = ? AND status = ?", moduleID, "active").First(&schema).Error; err != nil {
		log.Printf("[ModuleSkillGenerator] Module %d 没有活跃的 Schema", moduleID)
		schema = models.Schema{} // 使用空 Schema
	}

	// 生成内容
	content, err := g.GenerateSkillContent(&module, &schema)
	if err != nil {
		return nil, err
	}

	// 创建 Skill 对象
	skill := &models.Skill{
		Name:           fmt.Sprintf("module_%d_auto", moduleID),
		DisplayName:    fmt.Sprintf("%s 配置知识", module.Name),
		Layer:          models.SkillLayerDomain,
		Content:        content,
		Version:        "1.0.0",
		IsActive:       true,
		Priority:       100,
		SourceType:     models.SkillSourceModuleAuto,
		SourceModuleID: &moduleID,
		Metadata: models.SkillMetadata{
			Tags:        []string{"module", "auto-generated", module.Provider},
			Description: fmt.Sprintf("从 Module %s 自动生成的配置知识", module.Name),
		},
	}

	return skill, nil
}

// PreviewSkillContent 预览 Skill 内容（不保存）
func (g *ModuleSkillGenerator) PreviewSkillContent(moduleID uint) (string, error) {
	// 获取 Module
	var module models.Module
	if err := g.db.First(&module, moduleID).Error; err != nil {
		return "", fmt.Errorf("Module 不存在: %w", err)
	}

	// 获取 Schema
	var schema models.Schema
	g.db.Where("module_id = ? AND status = ?", moduleID, "active").First(&schema)

	return g.GenerateSkillContent(&module, &schema)
}
