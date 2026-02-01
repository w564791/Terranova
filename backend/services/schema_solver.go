package services

import (
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"strings"

	"gorm.io/gorm"
)

// ========== 反馈类型定义 ==========

// FeedbackType 反馈类型
type FeedbackType string

const (
	FeedbackTypeError      FeedbackType = "error"      // 错误，必须修复
	FeedbackTypeWarning    FeedbackType = "warning"    // 警告，建议修复
	FeedbackTypeSuggestion FeedbackType = "suggestion" // 建议，可选修复
)

// FeedbackAction AI 需要采取的行动
type FeedbackAction string

const (
	ActionAdjustValue   FeedbackAction = "adjust_value"   // 调整参数值
	ActionRemoveField   FeedbackAction = "remove_field"   // 移除字段
	ActionAddField      FeedbackAction = "add_field"      // 添加字段
	ActionChooseFrom    FeedbackAction = "choose_from"    // 从列表中选择
	ActionProvideReason FeedbackAction = "provide_reason" // 提供选择理由
)

// ========== Schema 字段定义 ==========

// SchemaFieldDef Schema 中的字段定义
type SchemaFieldDef struct {
	Type        string                     `json:"type"`                   // string, number, boolean, array, object, map, json
	Required    bool                       `json:"required"`               // 是否必填
	Default     interface{}                `json:"default,omitempty"`      // 默认值
	Description string                     `json:"description,omitempty"`  // 描述
	Options     []interface{}              `json:"options,omitempty"`      // 枚举值（对应 enum）
	ForceNew    bool                       `json:"force_new,omitempty"`    // 是否强制新建
	MustInclude []string                   `json:"must_include,omitempty"` // map 类型必须包含的键
	Properties  map[string]*SchemaFieldDef `json:"properties,omitempty"`   // object 类型的属性
	Items       *SchemaFieldDef            `json:"items,omitempty"`        // array 类型的元素定义
	MinItems    *int                       `json:"min_items,omitempty"`    // 数组最小元素数
	MaxItems    *int                       `json:"max_items,omitempty"`    // 数组最大元素数
	MinLength   *int                       `json:"min_length,omitempty"`   // 字符串最小长度
	MaxLength   *int                       `json:"max_length,omitempty"`   // 字符串最大长度
	Minimum     *float64                   `json:"minimum,omitempty"`      // 数值最小值
	Maximum     *float64                   `json:"maximum,omitempty"`      // 数值最大值
	Pattern     string                     `json:"pattern,omitempty"`      // 正则表达式

	// 参数关联关系
	ConflictsWith []string `json:"conflicts_with,omitempty"` // 互斥参数
	DependsOn     []string `json:"depends_on,omitempty"`     // 依赖参数
}

// ========== 反馈结构 ==========

// SolverFeedback 反馈信息
type SolverFeedback struct {
	Type         FeedbackType   `json:"type"`                    // 反馈类型
	Action       FeedbackAction `json:"action"`                  // 需要的行动
	Field        string         `json:"field"`                   // 相关字段
	Message      string         `json:"message"`                 // 人类可读的消息
	AIPrompt     string         `json:"ai_prompt"`               // 给 AI 的提示
	CurrentValue interface{}    `json:"current_value,omitempty"` // 当前值
	Constraint   interface{}    `json:"constraint,omitempty"`    // 约束信息
	Context      interface{}    `json:"context,omitempty"`       // 额外上下文
}

// ========== Solver 结果 ==========

// SolverResult 组装结果
type SolverResult struct {
	Success        bool                   `json:"success"`         // 是否成功
	Params         map[string]interface{} `json:"params"`          // 最终参数
	Warnings       []string               `json:"warnings"`        // 警告信息
	AppliedRules   []string               `json:"applied_rules"`   // 应用的规则
	Feedbacks      []*SolverFeedback      `json:"feedbacks"`       // 反馈列表
	NeedAIFix      bool                   `json:"need_ai_fix"`     // 是否需要 AI 修复
	AIInstructions string                 `json:"ai_instructions"` // 给 AI 的完整指令
}

// ========== SchemaSolver 主结构 ==========

// SchemaSolver Schema 组装器
type SchemaSolver struct {
	db          *gorm.DB
	schema      map[string]*SchemaFieldDef
	moduleID    uint
	cmdbService *CMDBService
}

// NewSchemaSolver 创建新的组装器
func NewSchemaSolver(db *gorm.DB, moduleID uint) *SchemaSolver {
	return &SchemaSolver{
		db:          db,
		moduleID:    moduleID,
		cmdbService: NewCMDBService(db),
	}
}

// LoadSchema 加载 Module 的 Schema
func (s *SchemaSolver) LoadSchema() error {
	var schema struct {
		OpenAPISchema map[string]interface{} `gorm:"column:openapi_schema;type:jsonb"`
		SchemaData    string                 `gorm:"column:schema_data;type:jsonb"`
	}

	// 优先使用 openapi_schema，如果没有则使用 schema_data
	err := s.db.Table("schemas").
		Where("module_id = ? AND status = ?", s.moduleID, "active").
		Select("openapi_schema", "schema_data").
		First(&schema).Error

	if err != nil {
		return fmt.Errorf("加载 Schema 失败: %w", err)
	}

	// 解析 Schema
	var schemaMap map[string]interface{}
	if schema.OpenAPISchema != nil && len(schema.OpenAPISchema) > 0 {
		schemaMap = schema.OpenAPISchema
	} else if schema.SchemaData != "" {
		if err := json.Unmarshal([]byte(schema.SchemaData), &schemaMap); err != nil {
			return fmt.Errorf("解析 schema_data 失败: %w", err)
		}
	} else {
		return fmt.Errorf("Schema 为空")
	}

	// 转换为 SchemaFieldDef
	s.schema = make(map[string]*SchemaFieldDef)
	for key, value := range schemaMap {
		if fieldMap, ok := value.(map[string]interface{}); ok {
			s.schema[key] = s.parseFieldDef(fieldMap)
		}
	}

	log.Printf("[SchemaSolver] 加载了 %d 个字段定义", len(s.schema))
	return nil
}

// parseFieldDef 解析字段定义
func (s *SchemaSolver) parseFieldDef(fieldMap map[string]interface{}) *SchemaFieldDef {
	field := &SchemaFieldDef{}

	if t, ok := fieldMap["type"].(string); ok {
		field.Type = t
	}
	if r, ok := fieldMap["required"].(bool); ok {
		field.Required = r
	}
	if d, ok := fieldMap["default"]; ok {
		field.Default = d
	}
	if desc, ok := fieldMap["description"].(string); ok {
		field.Description = desc
	}
	if opts, ok := fieldMap["options"].([]interface{}); ok {
		field.Options = opts
	}
	if fn, ok := fieldMap["force_new"].(bool); ok {
		field.ForceNew = fn
	}
	if mi, ok := fieldMap["must_include"].([]interface{}); ok {
		for _, v := range mi {
			if str, ok := v.(string); ok {
				field.MustInclude = append(field.MustInclude, str)
			}
		}
	}
	if props, ok := fieldMap["properties"].(map[string]interface{}); ok {
		field.Properties = make(map[string]*SchemaFieldDef)
		for k, v := range props {
			if propMap, ok := v.(map[string]interface{}); ok {
				field.Properties[k] = s.parseFieldDef(propMap)
			}
		}
	}
	if items, ok := fieldMap["items"].(map[string]interface{}); ok {
		field.Items = s.parseFieldDef(items)
	}
	if conflicts, ok := fieldMap["conflicts_with"].([]interface{}); ok {
		for _, v := range conflicts {
			if str, ok := v.(string); ok {
				field.ConflictsWith = append(field.ConflictsWith, str)
			}
		}
	}
	if depends, ok := fieldMap["depends_on"].([]interface{}); ok {
		for _, v := range depends {
			if str, ok := v.(string); ok {
				field.DependsOn = append(field.DependsOn, str)
			}
		}
	}

	return field
}

// Solve 执行组装逻辑
func (s *SchemaSolver) Solve(aiParams map[string]interface{}) *SolverResult {
	result := &SolverResult{
		Success:      true,
		Params:       make(map[string]interface{}),
		Warnings:     make([]string, 0),
		AppliedRules: make([]string, 0),
		Feedbacks:    make([]*SolverFeedback, 0),
		NeedAIFix:    false,
	}

	// 如果 Schema 未加载，先加载
	if s.schema == nil {
		if err := s.LoadSchema(); err != nil {
			result.Success = false
			result.Feedbacks = append(result.Feedbacks, &SolverFeedback{
				Type:    FeedbackTypeError,
				Action:  ActionAdjustValue,
				Message: fmt.Sprintf("无法加载 Schema: %v", err),
			})
			return result
		}
	}

	// 复制 AI 参数
	for k, v := range aiParams {
		result.Params[k] = v
	}

	// 第一步: 验证枚举值
	s.validateEnums(result)

	// 第二步: 验证类型
	s.validateTypes(result)

	// 第三步: 验证数组约束
	s.validateArrayConstraints(result)

	// 第四步: 检查互斥条件
	s.checkConflicts(result)

	// 第五步: 检查依赖条件
	s.checkDependencies(result)

	// 第六步: 检查必填字段
	s.checkRequiredFields(result)

	// 第七步: 应用默认值
	s.applyDefaults(result)

	// 第八步: 验证 map 类型的 must_include
	s.validateMapMustInclude(result)

	// 检查是否有错误反馈
	for _, feedback := range result.Feedbacks {
		if feedback.Type == FeedbackTypeError {
			result.NeedAIFix = true
			result.Success = false
			break
		}
	}

	// 生成 AI 指令
	if result.NeedAIFix {
		result.AIInstructions = s.generateAIInstructions(result)
	}

	return result
}

// validateEnums 验证枚举值
func (s *SchemaSolver) validateEnums(result *SolverResult) {
	for key, value := range result.Params {
		field, exists := s.schema[key]
		if !exists || len(field.Options) == 0 {
			continue
		}

		// 检查值是否在枚举列表中
		valid := false
		for _, opt := range field.Options {
			if reflect.DeepEqual(value, opt) {
				valid = true
				break
			}
		}

		if !valid {
			result.Feedbacks = append(result.Feedbacks, &SolverFeedback{
				Type:         FeedbackTypeError,
				Action:       ActionChooseFrom,
				Field:        key,
				Message:      fmt.Sprintf("字段 '%s' 的值 '%v' 不在允许的选项中", key, value),
				AIPrompt:     fmt.Sprintf("字段 '%s' 的值 '%v' 不在允许的选项中。请从以下选项中选择一个: %v。根据用户需求选择最合适的值。", key, value, field.Options),
				CurrentValue: value,
				Constraint: map[string]interface{}{
					"type":           "enum",
					"allowed_values": field.Options,
				},
			})
		}
	}
}

// validateTypes 验证类型
func (s *SchemaSolver) validateTypes(result *SolverResult) {
	for key, value := range result.Params {
		field, exists := s.schema[key]
		if !exists {
			continue
		}

		expectedType := field.Type
		actualType := s.getValueType(value)

		// 类型兼容性检查
		if !s.isTypeCompatible(expectedType, actualType, value) {
			result.Feedbacks = append(result.Feedbacks, &SolverFeedback{
				Type:         FeedbackTypeError,
				Action:       ActionAdjustValue,
				Field:        key,
				Message:      fmt.Sprintf("字段 '%s' 期望类型 '%s'，但得到 '%s'", key, expectedType, actualType),
				AIPrompt:     fmt.Sprintf("字段 '%s' 应该是 '%s' 类型，但你提供的是 '%s' 类型，值为 '%v'。请将此值转换为正确的类型。", key, expectedType, actualType, value),
				CurrentValue: value,
				Constraint: map[string]interface{}{
					"type":          "type_mismatch",
					"expected_type": expectedType,
					"actual_type":   actualType,
				},
			})
		}
	}
}

// getValueType 获取值的类型
func (s *SchemaSolver) getValueType(value interface{}) string {
	if value == nil {
		return "null"
	}

	v := reflect.TypeOf(value)
	switch v.Kind() {
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int64, reflect.Float64, reflect.Float32:
		return "number"
	case reflect.Slice, reflect.Array:
		return "array"
	case reflect.Map:
		return "object" // map 和 object 都用 map 表示
	default:
		return "unknown"
	}
}

// isTypeCompatible 检查类型兼容性
func (s *SchemaSolver) isTypeCompatible(expected, actual string, value interface{}) bool {
	// 直接匹配
	if expected == actual {
		return true
	}

	// 特殊兼容性
	switch expected {
	case "object", "map":
		return actual == "object" || actual == "map"
	case "json":
		// json 类型可以是字符串或对象
		return actual == "string" || actual == "object"
	case "number":
		// 数字类型兼容
		return actual == "number"
	}

	return false
}

// validateArrayConstraints 验证数组约束
func (s *SchemaSolver) validateArrayConstraints(result *SolverResult) {
	for key, value := range result.Params {
		field, exists := s.schema[key]
		if !exists || field.Type != "array" {
			continue
		}

		v := reflect.ValueOf(value)
		if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
			continue
		}

		length := v.Len()

		// 检查最小元素数
		if field.MinItems != nil && length < *field.MinItems {
			result.Feedbacks = append(result.Feedbacks, &SolverFeedback{
				Type:         FeedbackTypeError,
				Action:       ActionAddField,
				Field:        key,
				Message:      fmt.Sprintf("字段 '%s' 有 %d 个元素，但至少需要 %d 个", key, length, *field.MinItems),
				AIPrompt:     fmt.Sprintf("数组 '%s' 当前有 %d 个元素，但需要至少 %d 个元素。请根据上下文添加 %d 个合适的元素。", key, length, *field.MinItems, *field.MinItems-length),
				CurrentValue: value,
				Constraint: map[string]interface{}{
					"type":      "min_items",
					"min_items": *field.MinItems,
				},
			})
		}

		// 检查最大元素数
		if field.MaxItems != nil && length > *field.MaxItems {
			// 获取当前所有元素
			items := make([]interface{}, length)
			for i := 0; i < length; i++ {
				items[i] = v.Index(i).Interface()
			}

			result.Feedbacks = append(result.Feedbacks, &SolverFeedback{
				Type:   FeedbackTypeError,
				Action: ActionProvideReason,
				Field:  key,
				Message: fmt.Sprintf("字段 '%s' 有 %d 个元素，但最多允许 %d 个，需要移除 %d 个",
					key, length, *field.MaxItems, length-*field.MaxItems),
				AIPrompt: fmt.Sprintf(`数组 '%s' 有太多元素（%d 个），最多允许 %d 个。
当前元素: %v

你需要移除 %d 个元素。对于你保留的每个元素，请解释为什么它比被移除的元素更重要。
考虑因素：
- 业务需求
- 安全影响
- 最佳实践
- 其他参数的上下文

请提供：
1. 精简后的列表（最多 %d 个元素）
2. 保留每个元素的原因
3. 移除元素的原因`,
					key, length, *field.MaxItems, items, length-*field.MaxItems, *field.MaxItems),
				CurrentValue: value,
				Constraint: map[string]interface{}{
					"type":      "max_items",
					"max_items": *field.MaxItems,
				},
				Context: map[string]interface{}{
					"current_items":   items,
					"items_to_keep":   *field.MaxItems,
					"items_to_remove": length - *field.MaxItems,
				},
			})
		}
	}
}

// checkConflicts 检查互斥条件
func (s *SchemaSolver) checkConflicts(result *SolverResult) {
	for key := range result.Params {
		field, exists := s.schema[key]
		if !exists || len(field.ConflictsWith) == 0 {
			continue
		}

		conflicts := make([]string, 0)
		for _, conflictKey := range field.ConflictsWith {
			if _, conflictExists := result.Params[conflictKey]; conflictExists {
				conflicts = append(conflicts, conflictKey)
			}
		}

		if len(conflicts) > 0 {
			conflictValues := make(map[string]interface{})
			for _, c := range conflicts {
				conflictValues[c] = result.Params[c]
			}

			result.Feedbacks = append(result.Feedbacks, &SolverFeedback{
				Type:   FeedbackTypeError,
				Action: ActionProvideReason,
				Field:  key,
				Message: fmt.Sprintf("字段 '%s' 与以下字段互斥: %v，只能保留一个",
					key, conflicts),
				AIPrompt: fmt.Sprintf(`你同时提供了 '%s' 和 %v，但这些字段是互斥的。

请选择以下选项之一并解释你的理由：
1. 保留 '%s'（值: %v）- 并移除 %v
2. 移除 '%s' - 并保留 %v

考虑：
- 哪个选项更符合用户需求？
- 有什么权衡？
- 是否有其他参数的依赖？

请提供你的选择和详细理由。`,
					key, conflicts,
					key, result.Params[key], conflicts,
					key, conflicts),
				CurrentValue: result.Params[key],
				Constraint: map[string]interface{}{
					"type":      "conflict",
					"conflicts": conflicts,
				},
				Context: map[string]interface{}{
					"conflicting_fields": conflicts,
					"conflicting_values": conflictValues,
				},
			})
		}
	}
}

// checkDependencies 检查依赖条件
func (s *SchemaSolver) checkDependencies(result *SolverResult) {
	for key := range result.Params {
		field, exists := s.schema[key]
		if !exists || len(field.DependsOn) == 0 {
			continue
		}

		missingDeps := make([]string, 0)
		for _, depKey := range field.DependsOn {
			if _, depExists := result.Params[depKey]; !depExists {
				missingDeps = append(missingDeps, depKey)
			}
		}

		if len(missingDeps) > 0 {
			// 获取缺失依赖的 Schema 信息
			depSchemas := make(map[string]*SchemaFieldDef)
			for _, dep := range missingDeps {
				if schema, ok := s.schema[dep]; ok {
					depSchemas[dep] = schema
				}
			}

			result.Feedbacks = append(result.Feedbacks, &SolverFeedback{
				Type:   FeedbackTypeError,
				Action: ActionAddField,
				Field:  key,
				Message: fmt.Sprintf("字段 '%s' 依赖于缺失的字段: %v",
					key, missingDeps),
				AIPrompt: fmt.Sprintf(`你提供了 '%s'，但它需要以下缺失的字段: %v

对于每个缺失的字段，请根据以下信息提供合适的值：
- 字段的 Schema 定义
- 其他参数的上下文
- 最佳实践和常见配置

如果你无法确定合适的值，考虑移除 '%s'。`,
					key, missingDeps, key),
				CurrentValue: result.Params[key],
				Constraint: map[string]interface{}{
					"type":         "dependency",
					"dependencies": missingDeps,
				},
				Context: map[string]interface{}{
					"missing_dependencies": missingDeps,
					"dependency_schemas":   depSchemas,
				},
			})
		}
	}
}

// checkRequiredFields 检查必填字段
func (s *SchemaSolver) checkRequiredFields(result *SolverResult) {
	for key, field := range s.schema {
		if !field.Required {
			continue
		}

		if _, exists := result.Params[key]; !exists {
			// 如果有默认值，不报错（会在 applyDefaults 中填充）
			if field.Default != nil {
				continue
			}

			enumPrompt := ""
			if len(field.Options) > 0 {
				enumPrompt = fmt.Sprintf("- 允许的值: %v", field.Options)
			}

			result.Feedbacks = append(result.Feedbacks, &SolverFeedback{
				Type:    FeedbackTypeError,
				Action:  ActionAddField,
				Field:   key,
				Message: fmt.Sprintf("必填字段 '%s' 缺失", key),
				AIPrompt: fmt.Sprintf(`必填字段 '%s' 缺失。

字段详情：
- 类型: %s
- 描述: %s
%s

请根据以下信息提供此字段的合适值：
- 用户的原始请求
- 其他参数的上下文
- 最佳实践`,
					key,
					field.Type,
					field.Description,
					enumPrompt),
				Constraint: map[string]interface{}{
					"type":           "required",
					"allowed_values": field.Options,
				},
				Context: map[string]interface{}{
					"field_schema": field,
				},
			})
		}
	}
}

// applyDefaults 应用默认值
func (s *SchemaSolver) applyDefaults(result *SolverResult) {
	for key, field := range s.schema {
		if _, exists := result.Params[key]; !exists && field.Default != nil {
			result.Params[key] = field.Default
			result.AppliedRules = append(result.AppliedRules,
				fmt.Sprintf("应用默认值: %s = %v", key, field.Default))
		}
	}
}

// validateMapMustInclude 验证 map 类型的 must_include
func (s *SchemaSolver) validateMapMustInclude(result *SolverResult) {
	for key, value := range result.Params {
		field, exists := s.schema[key]
		if !exists || len(field.MustInclude) == 0 {
			continue
		}

		// 检查值是否是 map
		valueMap, ok := value.(map[string]interface{})
		if !ok {
			continue
		}

		missingKeys := make([]string, 0)
		for _, requiredKey := range field.MustInclude {
			if _, keyExists := valueMap[requiredKey]; !keyExists {
				missingKeys = append(missingKeys, requiredKey)
			}
		}

		if len(missingKeys) > 0 {
			result.Feedbacks = append(result.Feedbacks, &SolverFeedback{
				Type:   FeedbackTypeError,
				Action: ActionAddField,
				Field:  key,
				Message: fmt.Sprintf("字段 '%s' 必须包含以下键: %v",
					key, missingKeys),
				AIPrompt: fmt.Sprintf(`字段 '%s' 是一个 map/object，必须包含以下键: %v

当前值: %v

请添加缺失的键，并根据上下文提供合适的值。`,
					key, missingKeys, valueMap),
				CurrentValue: value,
				Constraint: map[string]interface{}{
					"type":          "must_include",
					"required_keys": missingKeys,
				},
			})
		}
	}
}

// generateAIInstructions 生成给 AI 的完整指令
func (s *SchemaSolver) generateAIInstructions(result *SolverResult) string {
	var sb strings.Builder

	sb.WriteString("Schema 验证发现以下问题需要你处理：\n\n")

	// 按优先级分组反馈
	errors := make([]*SolverFeedback, 0)
	warnings := make([]*SolverFeedback, 0)

	for _, feedback := range result.Feedbacks {
		switch feedback.Type {
		case FeedbackTypeError:
			errors = append(errors, feedback)
		case FeedbackTypeWarning:
			warnings = append(warnings, feedback)
		}
	}

	// 错误必须修复
	if len(errors) > 0 {
		sb.WriteString("🚨 错误（必须修复）：\n")
		for i, feedback := range errors {
			sb.WriteString(fmt.Sprintf("\n%d. [%s] %s\n", i+1, feedback.Field, feedback.AIPrompt))
			if feedback.Context != nil {
				contextJSON, _ := json.MarshalIndent(feedback.Context, "   ", "  ")
				sb.WriteString(fmt.Sprintf("   上下文: %s\n", contextJSON))
			}
		}
		sb.WriteString("\n")
	}

	// 警告建议修复
	if len(warnings) > 0 {
		sb.WriteString("⚠️ 警告（建议修复）：\n")
		for i, feedback := range warnings {
			sb.WriteString(fmt.Sprintf("\n%d. [%s] %s\n", i+1, feedback.Field, feedback.AIPrompt))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(`
请提供修正后的参数，解决所有错误。
对于你做的每个更改，请解释你的理由。

输出格式：
{
  "corrected_params": { ... },
  "changes": [
    {
      "field": "字段名",
      "action": "你做了什么",
      "reason": "为什么这样做"
    }
  ]
}
`)

	return sb.String()
}

// GetSchema 获取已加载的 Schema
func (s *SchemaSolver) GetSchema() map[string]*SchemaFieldDef {
	return s.schema
}

// GetFieldDef 获取指定字段的定义
func (s *SchemaSolver) GetFieldDef(fieldName string) *SchemaFieldDef {
	if s.schema == nil {
		return nil
	}
	return s.schema[fieldName]
}
