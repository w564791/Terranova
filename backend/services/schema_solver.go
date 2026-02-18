package services

import (
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"regexp"
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

	// 隐含规则: 当字段值满足条件时，自动设置其他字段
	// 例如: high_availability=true 时自动设置 multi_az=true
	Implies *ImpliesRule `json:"implies,omitempty"`

	// 条件规则: if-else 逻辑
	Conditional *ConditionalRule `json:"conditional,omitempty"`

	// 数据源配置
	Source       string                 `json:"source,omitempty"`        // cmdb, output, variable
	SourceConfig map[string]interface{} `json:"source_config,omitempty"` // 数据源配置
}

// ImpliesRule 隐含规则: 当字段值满足条件时，自动设置其他字段
type ImpliesRule struct {
	When interface{}            `json:"when"` // 触发条件的值
	Then map[string]interface{} `json:"then"` // 要设置的字段和值
}

// ConditionalRule 条件规则: if-else 逻辑
type ConditionalRule struct {
	If   *Condition        `json:"if"`             // 条件
	Then *FieldRequirement `json:"then,omitempty"` // 满足条件时的要求
	Else *FieldRequirement `json:"else,omitempty"` // 不满足条件时的要求
}

// Condition 条件定义
type Condition struct {
	Field    string      `json:"field"`    // 字段名
	Operator string      `json:"operator"` // 操作符: exists, equals, in, not_exists, not_equals
	Value    interface{} `json:"value"`    // 比较值
}

// FieldRequirement 字段要求
type FieldRequirement struct {
	Required  []string               `json:"required,omitempty"`   // 必须存在的字段
	Forbidden []string               `json:"forbidden,omitempty"`  // 必须不存在的字段
	SetValues map[string]interface{} `json:"set_values,omitempty"` // 自动设置的值
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
	// 使用 []byte 来接收 JSONB 数据，避免 GORM 扫描问题
	var schema struct {
		OpenAPISchema []byte `gorm:"column:openapi_schema;type:jsonb"`
		SchemaData    []byte `gorm:"column:schema_data;type:jsonb"`
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

	// 优先使用 openapi_schema
	if len(schema.OpenAPISchema) > 0 {
		if err := json.Unmarshal(schema.OpenAPISchema, &schemaMap); err != nil {
			log.Printf("[SchemaSolver] 解析 openapi_schema 失败: %v", err)
			// 继续尝试 schema_data
		}
	}

	// 如果 openapi_schema 解析失败或为空，尝试 schema_data
	if schemaMap == nil && len(schema.SchemaData) > 0 {
		if err := json.Unmarshal(schema.SchemaData, &schemaMap); err != nil {
			return fmt.Errorf("解析 schema_data 失败: %w", err)
		}
	}

	if schemaMap == nil {
		return fmt.Errorf("Schema 为空")
	}

	// 检测 Schema 格式并提取字段定义
	propertiesMap := s.extractPropertiesFromSchema(schemaMap)
	if propertiesMap == nil {
		return fmt.Errorf("无法从 Schema 中提取字段定义")
	}

	// 提取 required 字段列表
	requiredFields := s.extractRequiredFields(schemaMap)

	// 转换为 SchemaFieldDef
	s.schema = make(map[string]*SchemaFieldDef)
	for key, value := range propertiesMap {
		if fieldMap, ok := value.(map[string]interface{}); ok {
			fieldDef := s.parseFieldDef(fieldMap)
			// 检查是否在 required 列表中
			for _, req := range requiredFields {
				if req == key {
					fieldDef.Required = true
					break
				}
			}
			s.schema[key] = fieldDef
		}
	}

	log.Printf("[SchemaSolver] 加载了 %d 个字段定义", len(s.schema))
	return nil
}

// extractPropertiesFromSchema 从 Schema 中提取 properties
// 支持多种格式：
// 1. OpenAPI 3.x: components.schemas.ModuleInput.properties
// 2. 简单格式: 直接是 properties map
func (s *SchemaSolver) extractPropertiesFromSchema(schemaMap map[string]interface{}) map[string]interface{} {
	// 尝试 OpenAPI 3.x 格式: components.schemas.ModuleInput.properties
	if components, ok := schemaMap["components"].(map[string]interface{}); ok {
		if schemas, ok := components["schemas"].(map[string]interface{}); ok {
			if moduleInput, ok := schemas["ModuleInput"].(map[string]interface{}); ok {
				if properties, ok := moduleInput["properties"].(map[string]interface{}); ok {
					log.Printf("[SchemaSolver] 检测到 OpenAPI 3.x 格式")
					return properties
				}
			}
		}
	}

	// 尝试直接 properties 格式
	if properties, ok := schemaMap["properties"].(map[string]interface{}); ok {
		log.Printf("[SchemaSolver] 检测到直接 properties 格式")
		return properties
	}

	// 尝试简单格式（直接是字段定义）
	// 检查是否有 type 字段，如果没有，可能是简单格式
	hasTypeField := false
	for _, value := range schemaMap {
		if fieldMap, ok := value.(map[string]interface{}); ok {
			if _, hasType := fieldMap["type"]; hasType {
				hasTypeField = true
				break
			}
		}
	}

	if hasTypeField {
		log.Printf("[SchemaSolver] 检测到简单格式（直接字段定义）")
		return schemaMap
	}

	return nil
}

// extractRequiredFields 从 Schema 中提取 required 字段列表
func (s *SchemaSolver) extractRequiredFields(schemaMap map[string]interface{}) []string {
	var required []string

	// 尝试 OpenAPI 3.x 格式
	if components, ok := schemaMap["components"].(map[string]interface{}); ok {
		if schemas, ok := components["schemas"].(map[string]interface{}); ok {
			if moduleInput, ok := schemas["ModuleInput"].(map[string]interface{}); ok {
				if reqList, ok := moduleInput["required"].([]interface{}); ok {
					for _, r := range reqList {
						if str, ok := r.(string); ok {
							required = append(required, str)
						}
					}
				}
			}
		}
	}

	// 尝试直接 required 格式
	if reqList, ok := schemaMap["required"].([]interface{}); ok {
		for _, r := range reqList {
			if str, ok := r.(string); ok {
				required = append(required, str)
			}
		}
	}

	return required
}

// parseFieldDef 解析字段定义
// 支持 OpenAPI 3.x 格式和自定义格式
func (s *SchemaSolver) parseFieldDef(fieldMap map[string]interface{}) *SchemaFieldDef {
	field := &SchemaFieldDef{}

	// 基本字段
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

	// 枚举值 - 支持 OpenAPI 的 "enum" 和自定义的 "options"
	if opts, ok := fieldMap["enum"].([]interface{}); ok {
		field.Options = opts
	} else if opts, ok := fieldMap["options"].([]interface{}); ok {
		field.Options = opts
	}

	// 正则表达式 - OpenAPI 的 "pattern"
	if pattern, ok := fieldMap["pattern"].(string); ok {
		field.Pattern = pattern
	}

	// 字符串长度约束 - OpenAPI 的 "minLength" 和 "maxLength"
	if minLen, ok := fieldMap["minLength"].(float64); ok {
		minLenInt := int(minLen)
		field.MinLength = &minLenInt
	}
	if maxLen, ok := fieldMap["maxLength"].(float64); ok {
		maxLenInt := int(maxLen)
		field.MaxLength = &maxLenInt
	}

	// 数值约束 - OpenAPI 的 "minimum" 和 "maximum"
	if min, ok := fieldMap["minimum"].(float64); ok {
		field.Minimum = &min
	}
	if max, ok := fieldMap["maximum"].(float64); ok {
		field.Maximum = &max
	}

	// 数组约束 - OpenAPI 的 "minItems" 和 "maxItems"
	if minItems, ok := fieldMap["minItems"].(float64); ok {
		minItemsInt := int(minItems)
		field.MinItems = &minItemsInt
	}
	if maxItems, ok := fieldMap["maxItems"].(float64); ok {
		maxItemsInt := int(maxItems)
		field.MaxItems = &maxItemsInt
	}

	// 自定义字段
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

	// 嵌套对象属性
	if props, ok := fieldMap["properties"].(map[string]interface{}); ok {
		field.Properties = make(map[string]*SchemaFieldDef)
		for k, v := range props {
			if propMap, ok := v.(map[string]interface{}); ok {
				field.Properties[k] = s.parseFieldDef(propMap)
			}
		}
	}

	// 数组元素定义
	if items, ok := fieldMap["items"].(map[string]interface{}); ok {
		field.Items = s.parseFieldDef(items)
	}

	// 参数关联关系
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
func (s *SchemaSolver) Solve(aiParams map[string]interface{}) (result *SolverResult) {
	// 初始化结果
	result = &SolverResult{
		Success:      true,
		Params:       make(map[string]interface{}),
		Warnings:     make([]string, 0),
		AppliedRules: make([]string, 0),
		Feedbacks:    make([]*SolverFeedback, 0),
		NeedAIFix:    false,
	}

	// panic 恢复机制 - 确保任何 panic 都不会导致程序崩溃
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[SchemaSolver] Solve 发生 panic: %v", r)
			result.Success = false
			result.NeedAIFix = false // panic 时不要求 AI 修复
			result.Feedbacks = append(result.Feedbacks, &SolverFeedback{
				Type:    FeedbackTypeError,
				Action:  ActionAdjustValue,
				Message: fmt.Sprintf("Schema 验证过程中发生内部错误: %v", r),
			})
		}
	}()

	// 检查 SchemaSolver 是否正确初始化
	if s == nil || s.db == nil {
		result.Success = false
		result.Feedbacks = append(result.Feedbacks, &SolverFeedback{
			Type:    FeedbackTypeError,
			Action:  ActionAdjustValue,
			Message: "SchemaSolver 未正确初始化",
		})
		return result
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

	// 再次检查 schema 是否加载成功
	if s.schema == nil {
		result.Success = false
		result.Feedbacks = append(result.Feedbacks, &SolverFeedback{
			Type:    FeedbackTypeError,
			Action:  ActionAdjustValue,
			Message: "Schema 加载后仍为空",
		})
		return result
	}

	// 处理空参数的情况
	if aiParams == nil {
		aiParams = make(map[string]interface{})
	}

	// 复制 AI 参数（深拷贝以避免修改原始数据）
	for k, v := range aiParams {
		result.Params[k] = s.deepCopyValue(v)
	}

	// 第一步: 验证枚举值
	s.validateEnums(result)

	// 第二步: 验证类型
	s.validateTypes(result)

	// 第三步: 验证字符串约束（最小长度、最大长度、正则表达式）
	s.validateStringConstraints(result)

	// 第四步: 验证数值约束（最小值、最大值）
	s.validateNumberConstraints(result)

	// 第五步: 验证数组约束
	s.validateArrayConstraints(result)

	// 第六步: 检查互斥条件
	s.checkConflicts(result)

	// 第七步: 检查依赖条件
	s.checkDependencies(result)

	// 第八步: 检查必填字段
	s.checkRequiredFields(result)

	// 第九步: 应用隐含规则 (Implies)
	s.applyImpliesRules(result)

	// 第十步: 应用条件规则 (Conditional)
	s.applyConditionalRules(result)

	// 第十一步: 验证 map 类型的 must_include
	// 注意：不再自动填充默认值，这应该由 AI 来决定
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

// validateStringConstraints 验证字符串约束（最小长度、最大长度、正则表达式）
func (s *SchemaSolver) validateStringConstraints(result *SolverResult) {
	for key, value := range result.Params {
		field, exists := s.schema[key]
		if !exists || field.Type != "string" {
			continue
		}

		strValue, ok := value.(string)
		if !ok {
			continue
		}

		length := len(strValue)

		// 检查最小长度
		if field.MinLength != nil && length < *field.MinLength {
			result.Feedbacks = append(result.Feedbacks, &SolverFeedback{
				Type:         FeedbackTypeError,
				Action:       ActionAdjustValue,
				Field:        key,
				Message:      fmt.Sprintf("字段 '%s' 的长度为 %d，但最小长度要求为 %d", key, length, *field.MinLength),
				AIPrompt:     fmt.Sprintf("字符串 '%s' 的长度为 %d，但需要至少 %d 个字符。当前值: '%s'。请提供一个更长的值。", key, length, *field.MinLength, strValue),
				CurrentValue: value,
				Constraint: map[string]interface{}{
					"type":       "min_length",
					"min_length": *field.MinLength,
					"actual":     length,
				},
			})
		}

		// 检查最大长度
		if field.MaxLength != nil && length > *field.MaxLength {
			result.Feedbacks = append(result.Feedbacks, &SolverFeedback{
				Type:         FeedbackTypeError,
				Action:       ActionAdjustValue,
				Field:        key,
				Message:      fmt.Sprintf("字段 '%s' 的长度为 %d，但最大长度限制为 %d", key, length, *field.MaxLength),
				AIPrompt:     fmt.Sprintf("字符串 '%s' 的长度为 %d，但最多只能有 %d 个字符。当前值: '%s'。请缩短这个值。", key, length, *field.MaxLength, strValue),
				CurrentValue: value,
				Constraint: map[string]interface{}{
					"type":       "max_length",
					"max_length": *field.MaxLength,
					"actual":     length,
				},
			})
		}

		// 检查正则表达式
		if field.Pattern != "" {
			matched, err := regexp.MatchString(field.Pattern, strValue)
			if err != nil {
				log.Printf("[SchemaSolver] 正则表达式错误: %v", err)
			} else if !matched {
				result.Feedbacks = append(result.Feedbacks, &SolverFeedback{
					Type:         FeedbackTypeError,
					Action:       ActionAdjustValue,
					Field:        key,
					Message:      fmt.Sprintf("字段 '%s' 的值不匹配要求的格式", key),
					AIPrompt:     fmt.Sprintf("字符串 '%s' 的值 '%s' 不匹配要求的格式（正则: %s）。请提供一个符合格式要求的值。", key, strValue, field.Pattern),
					CurrentValue: value,
					Constraint: map[string]interface{}{
						"type":    "pattern",
						"pattern": field.Pattern,
					},
				})
			}
		}
	}
}

// validateNumberConstraints 验证数值约束（最小值、最大值）
// 支持 number 和 integer 类型
func (s *SchemaSolver) validateNumberConstraints(result *SolverResult) {
	for key, value := range result.Params {
		field, exists := s.schema[key]
		if !exists {
			continue
		}

		// 支持 number 和 integer 类型
		if field.Type != "number" && field.Type != "integer" {
			continue
		}

		// 转换为 float64
		var numValue float64
		var isValidNumber bool
		switch v := value.(type) {
		case float64:
			numValue = v
			isValidNumber = true
		case float32:
			numValue = float64(v)
			isValidNumber = true
		case int:
			numValue = float64(v)
			isValidNumber = true
		case int64:
			numValue = float64(v)
			isValidNumber = true
		case int32:
			numValue = float64(v)
			isValidNumber = true
		case json.Number:
			// JSON 解析时可能返回 json.Number
			if f, err := v.Float64(); err == nil {
				numValue = f
				isValidNumber = true
			}
		}

		if !isValidNumber {
			continue
		}

		// 检查最小值
		if field.Minimum != nil && numValue < *field.Minimum {
			result.Feedbacks = append(result.Feedbacks, &SolverFeedback{
				Type:         FeedbackTypeError,
				Action:       ActionAdjustValue,
				Field:        key,
				Message:      fmt.Sprintf("字段 '%s' 的值为 %v，但最小值要求为 %v", key, numValue, *field.Minimum),
				AIPrompt:     fmt.Sprintf("数值 '%s' 的值为 %v，但需要至少为 %v。请提供一个更大的值。", key, numValue, *field.Minimum),
				CurrentValue: value,
				Constraint: map[string]interface{}{
					"type":    "minimum",
					"minimum": *field.Minimum,
					"actual":  numValue,
				},
			})
		}

		// 检查最大值
		if field.Maximum != nil && numValue > *field.Maximum {
			result.Feedbacks = append(result.Feedbacks, &SolverFeedback{
				Type:         FeedbackTypeError,
				Action:       ActionAdjustValue,
				Field:        key,
				Message:      fmt.Sprintf("字段 '%s' 的值为 %v，但最大值限制为 %v", key, numValue, *field.Maximum),
				AIPrompt:     fmt.Sprintf("数值 '%s' 的值为 %v，但最多只能为 %v。请提供一个更小的值。", key, numValue, *field.Maximum),
				CurrentValue: value,
				Constraint: map[string]interface{}{
					"type":    "maximum",
					"maximum": *field.Maximum,
					"actual":  numValue,
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
	case "integer":
		// integer 类型兼容 number（JSON 中整数也是 number）
		return actual == "number"
	case "boolean":
		// boolean 类型
		return actual == "boolean"
	case "string":
		// string 类型
		return actual == "string"
	case "array":
		// array 类型
		return actual == "array"
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

// applyImpliesRules 应用隐含规则
// 例如: high_availability=true 时自动设置 multi_az=true
func (s *SchemaSolver) applyImpliesRules(result *SolverResult) {
	for key, value := range result.Params {
		field, exists := s.schema[key]
		if !exists || field.Implies == nil {
			continue
		}

		// 检查是否满足触发条件
		if reflect.DeepEqual(value, field.Implies.When) {
			for impliedKey, impliedValue := range field.Implies.Then {
				// 只在目标字段不存在时设置
				if _, exists := result.Params[impliedKey]; !exists {
					result.Params[impliedKey] = impliedValue
					result.AppliedRules = append(result.AppliedRules,
						fmt.Sprintf("隐含规则: %s=%v → %s=%v", key, value, impliedKey, impliedValue))
				}
			}
		}
	}
}

// applyConditionalRules 应用条件规则 (if-else 逻辑)
func (s *SchemaSolver) applyConditionalRules(result *SolverResult) {
	for key, field := range s.schema {
		if field.Conditional == nil {
			continue
		}

		condition := field.Conditional
		conditionMet := s.evaluateCondition(condition.If, result.Params)

		var requirement *FieldRequirement
		var branch string
		if conditionMet {
			requirement = condition.Then
			branch = "then"
		} else if condition.Else != nil {
			requirement = condition.Else
			branch = "else"
		}

		if requirement != nil {
			// 检查必需字段
			for _, requiredField := range requirement.Required {
				if _, exists := result.Params[requiredField]; !exists {
					result.Feedbacks = append(result.Feedbacks, &SolverFeedback{
						Type:   FeedbackTypeError,
						Action: ActionAddField,
						Field:  requiredField,
						Message: fmt.Sprintf("条件规则要求字段 '%s' 必须存在（当 %s 时）",
							requiredField, s.describeCondition(condition.If)),
						AIPrompt: fmt.Sprintf(`基于字段 '%s' 的条件规则：
- 条件: %s
- 分支: %s
- 必需字段: '%s' 缺失

请为 '%s' 提供一个合适的值，考虑：
- 触发的条件
- 其他参数的上下文
- 最佳实践`,
							key, s.describeCondition(condition.If), branch, requiredField, requiredField),
						Constraint: map[string]interface{}{
							"type": "conditional_required",
						},
					})
				}
			}

			// 检查禁止字段
			for _, forbiddenField := range requirement.Forbidden {
				if _, exists := result.Params[forbiddenField]; exists {
					result.Feedbacks = append(result.Feedbacks, &SolverFeedback{
						Type:   FeedbackTypeError,
						Action: ActionRemoveField,
						Field:  forbiddenField,
						Message: fmt.Sprintf("条件规则禁止字段 '%s' 存在（当 %s 时）",
							forbiddenField, s.describeCondition(condition.If)),
						AIPrompt: fmt.Sprintf(`基于字段 '%s' 的条件规则：
- 条件: %s
- 分支: %s
- 字段 '%s' 必须不存在

你提供了 '%s' = %v，但这违反了条件规则。

请移除此字段或调整其他参数以避免触发此条件。
解释你选择的方法的理由。`,
							key, s.describeCondition(condition.If), branch,
							forbiddenField, forbiddenField, result.Params[forbiddenField]),
						CurrentValue: result.Params[forbiddenField],
						Constraint: map[string]interface{}{
							"type": "conditional_forbidden",
						},
					})
				}
			}

			// 自动设置值
			for setKey, setValue := range requirement.SetValues {
				if _, exists := result.Params[setKey]; !exists {
					result.Params[setKey] = setValue
					result.AppliedRules = append(result.AppliedRules,
						fmt.Sprintf("条件自动设置: %s=%v (条件: %s, 分支: %s)",
							setKey, setValue, s.describeCondition(condition.If), branch))
				}
			}
		}
	}
}

// evaluateCondition 评估条件
func (s *SchemaSolver) evaluateCondition(cond *Condition, params map[string]interface{}) bool {
	if cond == nil {
		return false
	}

	value, exists := params[cond.Field]

	switch cond.Operator {
	case "exists":
		return exists
	case "not_exists":
		return !exists
	case "equals":
		return exists && reflect.DeepEqual(value, cond.Value)
	case "not_equals":
		return !exists || !reflect.DeepEqual(value, cond.Value)
	case "in":
		if !exists {
			return false
		}
		valueList, ok := cond.Value.([]interface{})
		if !ok {
			return false
		}
		for _, v := range valueList {
			if reflect.DeepEqual(value, v) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// describeCondition 描述条件（用于日志和反馈）
func (s *SchemaSolver) describeCondition(cond *Condition) string {
	if cond == nil {
		return "无条件"
	}
	return fmt.Sprintf("%s %s %v", cond.Field, cond.Operator, cond.Value)
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
		sb.WriteString(" 警告（建议修复）：\n")
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

// deepCopyValue 深拷贝值，避免修改原始数据
func (s *SchemaSolver) deepCopyValue(value interface{}) interface{} {
	if value == nil {
		return nil
	}

	// 使用 JSON 序列化/反序列化实现深拷贝
	// 这种方式虽然性能不是最优，但最安全可靠
	switch v := value.(type) {
	case map[string]interface{}:
		// 对于 map，进行深拷贝
		result := make(map[string]interface{})
		for k, val := range v {
			result[k] = s.deepCopyValue(val)
		}
		return result
	case []interface{}:
		// 对于 slice，进行深拷贝
		result := make([]interface{}, len(v))
		for i, val := range v {
			result[i] = s.deepCopyValue(val)
		}
		return result
	case string, int, int64, int32, float64, float32, bool:
		// 基本类型直接返回（值类型，不需要拷贝）
		return v
	default:
		// 对于其他复杂类型，使用 JSON 序列化/反序列化
		data, err := json.Marshal(v)
		if err != nil {
			// 如果序列化失败，返回原值
			return v
		}
		var result interface{}
		if err := json.Unmarshal(data, &result); err != nil {
			// 如果反序列化失败，返回原值
			return v
		}
		return result
	}
}
