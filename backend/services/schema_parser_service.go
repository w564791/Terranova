package services

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// SchemaParserService 提供 Terraform variables.tf 解析服务
type SchemaParserService struct{}

// NewSchemaParserService 创建新的解析服务实例
func NewSchemaParserService() *SchemaParserService {
	return &SchemaParserService{}
}

// TFVariable 表示解析后的 Terraform 变量
type TFVariable struct {
	Name        string
	Type        string
	Description string
	Default     interface{}
	HasDefault  bool
	Sensitive   bool
	Nullable    bool
	Validations []TFValidation
	Annotations map[string]string
}

// TFValidation 表示 Terraform 验证规则
type TFValidation struct {
	Condition    string
	ErrorMessage string
}

// ParseOptions 解析选项
type ParseOptions struct {
	ModuleName string
	Provider   string
	Version    string
	Layout     string // top, left
}

// TFOutput 表示解析后的 Terraform 输出
type TFOutput struct {
	Name        string
	Description string
	Value       string
	Sensitive   bool
	Annotations map[string]string
}

// ParseResult 解析结果
type ParseResult struct {
	OpenAPISchema  map[string]interface{}
	FieldCount     int
	BasicFields    int
	AdvancedFields int
	OutputCount    int
	Warnings       []string
}

// ParseVariablesTF 解析 variables.tf 内容并生成 OpenAPI Schema
func (s *SchemaParserService) ParseVariablesTF(content string, opts ParseOptions) (*ParseResult, error) {
	// 解析变量
	variables, err := s.parseVariables(content)
	if err != nil {
		return nil, fmt.Errorf("解析变量失败: %w", err)
	}

	if len(variables) == 0 {
		return nil, fmt.Errorf("未找到任何变量定义")
	}

	// 生成 OpenAPI Schema
	schema := s.generateOpenAPISchema(variables, opts)

	// 统计字段数量
	basicCount := 0
	advancedCount := 0
	for _, v := range variables {
		level := v.Annotations["level"]
		if level == "basic" {
			basicCount++
		} else {
			advancedCount++
		}
	}

	return &ParseResult{
		OpenAPISchema:  schema,
		FieldCount:     len(variables),
		BasicFields:    basicCount,
		AdvancedFields: advancedCount,
		Warnings:       []string{},
	}, nil
}

// ParseTFWithOutputs 解析 variables.tf 和 outputs.tf 内容并生成 OpenAPI Schema
// 支持只解析 outputs.tf（当 variablesTF 为空时）
func (s *SchemaParserService) ParseTFWithOutputs(variablesTF, outputsTF string, opts ParseOptions) (*ParseResult, error) {
	var variables []TFVariable
	var outputs []TFOutput
	var err error

	// 解析变量（如果有）
	if strings.TrimSpace(variablesTF) != "" && !strings.HasPrefix(strings.TrimSpace(variablesTF), "# Empty") {
		variables, err = s.parseVariables(variablesTF)
		if err != nil {
			return nil, fmt.Errorf("解析变量失败: %w", err)
		}
	}

	// 解析输出（如果有）
	if strings.TrimSpace(outputsTF) != "" {
		outputs, err = s.parseOutputs(outputsTF)
		if err != nil {
			return nil, fmt.Errorf("解析输出失败: %w", err)
		}
	}

	// 如果两者都为空，返回错误
	if len(variables) == 0 && len(outputs) == 0 {
		return nil, fmt.Errorf("未找到任何变量或输出定义")
	}

	// 生成 OpenAPI Schema
	schema := s.generateOpenAPISchemaWithOutputs(variables, outputs, opts)

	// 统计字段数量
	basicCount := 0
	advancedCount := 0
	for _, v := range variables {
		level := v.Annotations["level"]
		if level == "basic" {
			basicCount++
		} else {
			advancedCount++
		}
	}

	return &ParseResult{
		OpenAPISchema:  schema,
		FieldCount:     len(variables),
		BasicFields:    basicCount,
		AdvancedFields: advancedCount,
		OutputCount:    len(outputs),
		Warnings:       []string{},
	}, nil
}

// parseOutputs 解析 outputs.tf 中的输出定义
func (s *SchemaParserService) parseOutputs(content string) ([]TFOutput, error) {
	var outputs []TFOutput

	// 匹配 output 块
	outputBlockRegex := regexp.MustCompile(`(?s)output\s+"([^"]+)"\s*\{((?:[^{}]|\{[^{}]*\})*)\}`)
	matches := outputBlockRegex.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		outputName := match[1]
		outputBody := match[2]

		output := TFOutput{
			Name:        outputName,
			Annotations: make(map[string]string),
		}

		// 解析 description
		descRegex := regexp.MustCompile(`description\s*=\s*"([^"]*)"`)
		if descMatch := descRegex.FindStringSubmatch(outputBody); len(descMatch) > 1 {
			output.Description = descMatch[1]

			// 检查 description 内部是否包含注释
			if strings.Contains(output.Description, "#") {
				parts := strings.SplitN(output.Description, "#", 2)
				output.Description = strings.TrimSpace(parts[0])
				if len(parts) > 1 {
					s.parseOutputAnnotations(strings.TrimSpace(parts[1]), &output)
				}
			}
		}

		// 查找 description 行后的注释
		descLineRegex := regexp.MustCompile(`description\s*=\s*"[^"]*"\s*#\s*(.*)`)
		if descLineMatch := descLineRegex.FindStringSubmatch(outputBody); len(descLineMatch) > 1 {
			s.parseOutputAnnotations(descLineMatch[1], &output)
		}

		// 解析 value
		valueRegex := regexp.MustCompile(`(?s)value\s*=\s*(.+?)(?:\n\s*(?:description|sensitive)\s*=|\n\s*\}|$)`)
		if valueMatch := valueRegex.FindStringSubmatch(outputBody); len(valueMatch) > 1 {
			output.Value = strings.TrimSpace(valueMatch[1])
		}

		// 解析 sensitive
		if strings.Contains(outputBody, "sensitive") {
			sensitiveRegex := regexp.MustCompile(`sensitive\s*=\s*(true|false)`)
			if sensitiveMatch := sensitiveRegex.FindStringSubmatch(outputBody); len(sensitiveMatch) > 1 {
				output.Sensitive = sensitiveMatch[1] == "true"
			}
		}

		outputs = append(outputs, output)
	}

	return outputs, nil
}

// parseOutputAnnotations 解析输出注释中的注解
func (s *SchemaParserService) parseOutputAnnotations(comment string, output *TFOutput) {
	annotationRegex := regexp.MustCompile(`@?(\w+):([^\s]+)`)
	matches := annotationRegex.FindAllStringSubmatch(comment, -1)
	for _, match := range matches {
		if len(match) > 2 {
			key := strings.ToLower(match[1])
			value := match[2]
			output.Annotations[key] = value
		}
	}
}

// generateOpenAPISchemaWithOutputs 生成包含 outputs 的 OpenAPI Schema
func (s *SchemaParserService) generateOpenAPISchemaWithOutputs(variables []TFVariable, outputs []TFOutput, opts ParseOptions) map[string]interface{} {
	// 先生成基础 schema（如果有变量）
	var schema map[string]interface{}
	if len(variables) > 0 {
		schema = s.generateOpenAPISchema(variables, opts)
	} else {
		// 创建空的基础 schema
		moduleName := opts.ModuleName
		if moduleName == "" {
			moduleName = "Module"
		}
		version := opts.Version
		if version == "" {
			version = "1.0.0"
		}
		layout := opts.Layout
		if layout == "" {
			layout = "top"
		}

		schema = map[string]interface{}{
			"openapi": "3.1.0",
			"info": map[string]interface{}{
				"title":           moduleName,
				"version":         version,
				"description":     fmt.Sprintf("Terraform module: %s", moduleName),
				"x-module-source": "",
				"x-provider":      opts.Provider,
			},
			"components": map[string]interface{}{
				"schemas": map[string]interface{}{
					"ModuleInput": map[string]interface{}{
						"type":       "object",
						"properties": map[string]interface{}{},
						"required":   []string{},
					},
				},
			},
			"x-iac-platform": map[string]interface{}{
				"ui": map[string]interface{}{
					"fields": map[string]interface{}{},
					"groups": []map[string]interface{}{
						{"id": "basic", "title": "基础配置", "order": 1, "defaultExpanded": true},
						{"id": "advanced", "title": "高级配置", "order": 2, "defaultExpanded": false},
					},
					"layout": map[string]interface{}{
						"type":     "tabs",
						"position": layout,
					},
				},
				"external": map[string]interface{}{
					"sources": []map[string]interface{}{},
				},
			},
		}
	}

	// 添加 outputs
	if len(outputs) > 0 {
		outputItems := []map[string]interface{}{}
		for _, o := range outputs {
			item := map[string]interface{}{
				"name":        o.Name,
				"type":        "string", // 默认类型
				"description": o.Description,
			}

			if o.Value != "" {
				item["valueExpression"] = o.Value
			}

			if o.Sensitive {
				item["sensitive"] = true
			}

			// 添加注解
			if alias, ok := o.Annotations["alias"]; ok {
				item["alias"] = alias
			}
			if group, ok := o.Annotations["group"]; ok {
				item["group"] = group
			}
			if typeVal, ok := o.Annotations["type"]; ok {
				item["type"] = typeVal
			}

			outputItems = append(outputItems, item)
		}

		// 添加到 x-iac-platform
		if iacPlatform, ok := schema["x-iac-platform"].(map[string]interface{}); ok {
			iacPlatform["outputs"] = map[string]interface{}{
				"description": "Module output list (for smart hints)",
				"items":       outputItems,
			}
		}
	}

	return schema
}

// parseVariables 解析 variables.tf 中的变量定义
func (s *SchemaParserService) parseVariables(content string) ([]TFVariable, error) {
	var variables []TFVariable

	// 匹配 variable 块 - 改进正则以支持嵌套大括号
	varBlockRegex := regexp.MustCompile(`(?s)variable\s+"([^"]+)"\s*\{((?:[^{}]|\{[^{}]*\})*)\}`)
	matches := varBlockRegex.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		varName := match[1]
		varBody := match[2]

		variable := TFVariable{
			Name:        varName,
			Annotations: make(map[string]string),
		}

		// 解析 description（包含行尾注释）- 改进正则以支持多种格式
		// 格式1: description = "xxx" # @level:basic @alias:名称
		// 格式2: description = "xxx # @level:basic" (注释在引号内)
		descRegex := regexp.MustCompile(`description\s*=\s*"([^"]*)"`)
		if descMatch := descRegex.FindStringSubmatch(varBody); len(descMatch) > 1 {
			variable.Description = descMatch[1]

			// 检查 description 内部是否包含注释
			if strings.Contains(variable.Description, "#") {
				parts := strings.SplitN(variable.Description, "#", 2)
				variable.Description = strings.TrimSpace(parts[0])
				if len(parts) > 1 {
					s.parseAnnotations(strings.TrimSpace(parts[1]), &variable)
				}
			}
		}

		// 查找 description 行后的注释
		descLineRegex := regexp.MustCompile(`description\s*=\s*"[^"]*"\s*#\s*(.*)`)
		if descLineMatch := descLineRegex.FindStringSubmatch(varBody); len(descLineMatch) > 1 {
			s.parseAnnotations(descLineMatch[1], &variable)
		}

		// 查找独立的注释行中的注解
		commentRegex := regexp.MustCompile(`#\s*(@\w+:[^\s]+(?:\s+@\w+:[^\s]+)*)`)
		commentMatches := commentRegex.FindAllStringSubmatch(varBody, -1)
		for _, cm := range commentMatches {
			if len(cm) > 1 {
				s.parseAnnotations(cm[1], &variable)
			}
		}

		// 解析 type
		typeRegex := regexp.MustCompile(`type\s*=\s*([^\n]+)`)
		if typeMatch := typeRegex.FindStringSubmatch(varBody); len(typeMatch) > 1 {
			variable.Type = strings.TrimSpace(typeMatch[1])
		}

		// 解析 validation 块 - 支持嵌套大括号（需要先定义，用于移除 validation 块后解析 default）
		validationRegex := regexp.MustCompile(`(?s)validation\s*\{((?:[^{}]|\{[^{}]*\})*)\}`)

		// 解析 default - 需要在解析 validation 之前移除 validation 块
		varBodyWithoutValidation := validationRegex.ReplaceAllString(varBody, "")

		// 改进的 default 解析：支持简单值和复杂值
		defaultRegex := regexp.MustCompile(`(?s)default\s*=\s*(.+?)(?:\n\s*(?:description|type|sensitive|nullable)\s*=|\n\s*\}|$)`)
		if defaultMatch := defaultRegex.FindStringSubmatch(varBodyWithoutValidation); len(defaultMatch) > 1 {
			defaultValue := strings.TrimSpace(defaultMatch[1])
			// 移除尾部可能的换行和空格
			defaultValue = strings.TrimRight(defaultValue, " \t\n\r")
			// 如果默认值以 validation 开头，说明解析错误
			if !strings.HasPrefix(defaultValue, "validation") {
				variable.HasDefault = true
				variable.Default = s.parseDefaultValue(defaultValue)
			}
		}

		// 解析 sensitive
		if strings.Contains(varBody, "sensitive") {
			sensitiveRegex := regexp.MustCompile(`sensitive\s*=\s*(true|false)`)
			if sensitiveMatch := sensitiveRegex.FindStringSubmatch(varBody); len(sensitiveMatch) > 1 {
				variable.Sensitive = sensitiveMatch[1] == "true"
			}
		}

		// 解析 nullable
		if strings.Contains(varBody, "nullable") {
			nullableRegex := regexp.MustCompile(`nullable\s*=\s*(true|false)`)
			if nullableMatch := nullableRegex.FindStringSubmatch(varBody); len(nullableMatch) > 1 {
				variable.Nullable = nullableMatch[1] == "true"
			}
		}

		// 解析 validation 块
		validationMatches := validationRegex.FindAllStringSubmatch(varBody, -1)
		for _, vm := range validationMatches {
			if len(vm) > 1 {
				validation := TFValidation{}
				condRegex := regexp.MustCompile(`condition\s*=\s*(.+)`)
				if condMatch := condRegex.FindStringSubmatch(vm[1]); len(condMatch) > 1 {
					validation.Condition = strings.TrimSpace(condMatch[1])
				}
				errRegex := regexp.MustCompile(`error_message\s*=\s*"([^"]*)"`)
				if errMatch := errRegex.FindStringSubmatch(vm[1]); len(errMatch) > 1 {
					validation.ErrorMessage = errMatch[1]
				}
				if validation.Condition != "" {
					variable.Validations = append(variable.Validations, validation)
				}
			}
		}

		variables = append(variables, variable)
	}

	return variables, nil
}

// parseAnnotations 解析行尾注释中的注解
func (s *SchemaParserService) parseAnnotations(comment string, variable *TFVariable) {
	// 支持的注解格式: @key:value 或 key:value
	annotationRegex := regexp.MustCompile(`@?(\w+):([^\s]+)`)
	matches := annotationRegex.FindAllStringSubmatch(comment, -1)
	for _, match := range matches {
		if len(match) > 2 {
			key := strings.ToLower(match[1])
			value := match[2]
			variable.Annotations[key] = value
		}
	}
}

// parseDefaultValue 解析默认值
func (s *SchemaParserService) parseDefaultValue(value string) interface{} {
	value = strings.TrimSpace(value)

	// null
	if value == "null" {
		return nil
	}

	// boolean
	if value == "true" {
		return true
	}
	if value == "false" {
		return false
	}

	// number
	if matched, _ := regexp.MatchString(`^-?\d+$`, value); matched {
		var num int
		fmt.Sscanf(value, "%d", &num)
		return num
	}
	if matched, _ := regexp.MatchString(`^-?\d+\.\d+$`, value); matched {
		var num float64
		fmt.Sscanf(value, "%f", &num)
		return num
	}

	// string
	if strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
		return value[1 : len(value)-1]
	}

	// empty list
	if value == "[]" {
		return []interface{}{}
	}

	// empty map
	if value == "{}" {
		return map[string]interface{}{}
	}

	return value
}

// generateOpenAPISchema 生成 OpenAPI Schema
func (s *SchemaParserService) generateOpenAPISchema(variables []TFVariable, opts ParseOptions) map[string]interface{} {
	// 构建 properties
	properties := make(map[string]interface{})
	required := []string{}
	uiFields := make(map[string]interface{})

	// 按 order 排序
	sort.Slice(variables, func(i, j int) bool {
		orderI := s.getAnnotationInt(variables[i].Annotations, "order", 999)
		orderJ := s.getAnnotationInt(variables[j].Annotations, "order", 999)
		return orderI < orderJ
	})

	basicOrder := 1
	advancedOrder := 100 // 高级参数从 100 开始，确保基础参数排在前面

	for _, v := range variables {
		prop := s.variableToProperty(v)
		properties[v.Name] = prop

		// 判断是否必填
		if !v.HasDefault {
			required = append(required, v.Name)
		}

		// 生成 UI 配置
		level := v.Annotations["level"]
		if level == "" {
			level = "advanced" // 默认为高级配置
		}

		uiField := map[string]interface{}{
			"group": level,
			"help":  v.Description,
		}

		// 设置 order - 基础参数从 1 开始，高级参数从 100 开始
		if level == "basic" {
			uiField["order"] = basicOrder
			basicOrder++
		} else {
			uiField["order"] = advancedOrder
			advancedOrder++
		}

		// 设置 label
		if alias, ok := v.Annotations["alias"]; ok {
			uiField["label"] = alias
		} else {
			uiField["label"] = s.formatLabel(v.Name)
		}

		// 设置 widget
		if widget, ok := v.Annotations["widget"]; ok {
			uiField["widget"] = widget
		} else {
			uiField["widget"] = s.inferWidget(v)
		}

		// 设置 source（外部数据源）
		if source, ok := v.Annotations["source"]; ok {
			uiField["source"] = source
		}

		// 设置其他 UI 属性
		if placeholder, ok := v.Annotations["placeholder"]; ok {
			uiField["placeholder"] = placeholder
		}
		if v.Annotations["searchable"] == "true" {
			uiField["searchable"] = true
		}
		if v.Annotations["allowcustom"] == "true" {
			uiField["allowCustom"] = true
		}

		uiFields[v.Name] = uiField
	}

	// 构建分组
	groups := []map[string]interface{}{
		{
			"id":              "basic",
			"title":           "基础配置",
			"icon":            "settings",
			"order":           1,
			"defaultExpanded": true,
		},
		{
			"id":              "advanced",
			"title":           "高级配置",
			"icon":            "code",
			"order":           2,
			"defaultExpanded": false,
		},
	}

	// 构建外部数据源
	externalSources := s.buildExternalSources(variables, opts.Provider)

	// 构建完整的 OpenAPI Schema
	moduleName := opts.ModuleName
	if moduleName == "" {
		moduleName = "Module"
	}

	version := opts.Version
	if version == "" {
		version = "1.0.0"
	}

	layout := opts.Layout
	if layout == "" {
		layout = "top"
	}

	schema := map[string]interface{}{
		"openapi": "3.1.0",
		"info": map[string]interface{}{
			"title":           moduleName,
			"version":         version,
			"description":     fmt.Sprintf("Terraform module: %s", moduleName),
			"x-module-source": "",
			"x-provider":      opts.Provider,
		},
		"components": map[string]interface{}{
			"schemas": map[string]interface{}{
				"ModuleInput": map[string]interface{}{
					"type":       "object",
					"properties": properties,
					"required":   required,
				},
			},
		},
		"x-iac-platform": map[string]interface{}{
			"ui": map[string]interface{}{
				"fields": uiFields,
				"groups": groups,
				"layout": map[string]interface{}{
					"type":     "tabs",
					"position": layout,
				},
			},
			"external": map[string]interface{}{
				"sources": externalSources,
			},
		},
	}

	return schema
}

// variableToProperty 将变量转换为 JSON Schema property
func (s *SchemaParserService) variableToProperty(v TFVariable) map[string]interface{} {
	prop := map[string]interface{}{
		"title":       s.formatLabel(v.Name),
		"description": v.Description,
	}

	// 转换类型
	jsonType, format := s.tfTypeToJSONType(v.Type)
	prop["type"] = jsonType
	if format != "" {
		prop["format"] = format
	}

	// 设置默认值
	if v.HasDefault && v.Default != nil {
		prop["default"] = v.Default
	}

	// 处理数组类型
	if jsonType == "array" {
		itemType := s.extractArrayItemType(v.Type)
		if itemType == "object" {
			prop["items"] = map[string]interface{}{"type": "object"}
		} else {
			prop["items"] = map[string]interface{}{"type": itemType}
		}
	}

	// 处理对象类型
	if jsonType == "object" && strings.Contains(v.Type, "map(") {
		prop["additionalProperties"] = map[string]interface{}{"type": "string"}
	}

	// 添加验证规则
	if len(v.Validations) > 0 {
		validations := []map[string]interface{}{}
		for _, val := range v.Validations {
			validation := map[string]interface{}{
				"condition":    val.Condition,
				"errorMessage": val.ErrorMessage,
			}
			// 尝试提取 pattern
			if pattern := s.extractPattern(val.Condition); pattern != "" {
				validation["pattern"] = pattern
				prop["pattern"] = pattern
			}
			validations = append(validations, validation)
		}
		prop["x-validation"] = validations
	}

	// 处理枚举
	if enum, ok := v.Annotations["enum"]; ok {
		prop["enum"] = strings.Split(enum, ",")
	}

	// 处理敏感字段
	if v.Sensitive {
		prop["x-sensitive"] = true
	}

	return prop
}

// tfTypeToJSONType 将 Terraform 类型转换为 JSON Schema 类型
func (s *SchemaParserService) tfTypeToJSONType(tfType string) (string, string) {
	tfType = strings.TrimSpace(tfType)

	switch {
	case tfType == "string":
		return "string", ""
	case tfType == "number":
		return "number", ""
	case tfType == "bool":
		return "boolean", ""
	case strings.HasPrefix(tfType, "list("):
		return "array", ""
	case strings.HasPrefix(tfType, "set("):
		return "array", "x-set"
	case strings.HasPrefix(tfType, "map("):
		return "object", ""
	case strings.HasPrefix(tfType, "object("):
		return "object", ""
	case tfType == "any":
		return "object", ""
	default:
		return "string", ""
	}
}

// extractArrayItemType 提取数组元素类型
func (s *SchemaParserService) extractArrayItemType(tfType string) string {
	// list(string) -> string
	// list(object({...})) -> object
	if strings.HasPrefix(tfType, "list(") || strings.HasPrefix(tfType, "set(") {
		inner := tfType[5 : len(tfType)-1]
		if strings.HasPrefix(inner, "object(") {
			return "object"
		}
		jsonType, _ := s.tfTypeToJSONType(inner)
		return jsonType
	}
	return "string"
}

// extractPattern 从验证条件中提取正则表达式
func (s *SchemaParserService) extractPattern(condition string) string {
	// 匹配 regex("pattern", var.xxx) 或 can(regex("pattern", var.xxx))
	regexPattern := regexp.MustCompile(`regex\s*\(\s*"([^"]+)"`)
	if match := regexPattern.FindStringSubmatch(condition); len(match) > 1 {
		return match[1]
	}
	return ""
}

// inferWidget 推断 widget 类型
func (s *SchemaParserService) inferWidget(v TFVariable) string {
	// 根据注解
	if widget, ok := v.Annotations["widget"]; ok {
		return widget
	}

	// 根据类型推断
	jsonType, _ := s.tfTypeToJSONType(v.Type)

	switch jsonType {
	case "boolean":
		return "switch"
	case "array":
		itemType := s.extractArrayItemType(v.Type)
		if itemType == "object" {
			return "object-list"
		}
		return "tags"
	case "object":
		if strings.Contains(v.Type, "map(") {
			return "key-value"
		}
		return "object"
	default:
		// 检查是否有枚举
		if _, ok := v.Annotations["enum"]; ok {
			return "select"
		}
		// 检查是否有外部数据源
		if _, ok := v.Annotations["source"]; ok {
			return "select"
		}
		return "text"
	}
}

// formatLabel 格式化标签
func (s *SchemaParserService) formatLabel(name string) string {
	// 将 snake_case 转换为 Title Case
	words := strings.Split(name, "_")
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + word[1:]
		}
	}
	return strings.Join(words, " ")
}

// getAnnotationInt 获取注解的整数值
func (s *SchemaParserService) getAnnotationInt(annotations map[string]string, key string, defaultVal int) int {
	if val, ok := annotations[key]; ok {
		var num int
		if _, err := fmt.Sscanf(val, "%d", &num); err == nil {
			return num
		}
	}
	return defaultVal
}

// buildExternalSources 构建外部数据源配置
func (s *SchemaParserService) buildExternalSources(variables []TFVariable, provider string) []map[string]interface{} {
	sources := []map[string]interface{}{}
	sourceMap := make(map[string]bool)

	for _, v := range variables {
		if source, ok := v.Annotations["source"]; ok {
			if !sourceMap[source] {
				sourceMap[source] = true
				sourceConfig := s.getSourceConfig(source, provider)
				if sourceConfig != nil {
					sources = append(sources, sourceConfig)
				}
			}
		}
	}

	return sources
}

// getSourceConfig 获取数据源配置
func (s *SchemaParserService) getSourceConfig(sourceID string, provider string) map[string]interface{} {
	// 预定义的数据源配置
	sourceConfigs := map[string]map[string]interface{}{
		"ami_list": {
			"id":     "ami_list",
			"type":   "api",
			"api":    "/api/v1/aws/ec2/images",
			"method": "GET",
			"params": map[string]interface{}{
				"region": "${providers.aws.region}",
			},
			"cache": map[string]interface{}{
				"ttl": 300,
			},
			"transform": map[string]interface{}{
				"type":       "jmespath",
				"expression": "data.images[*].{value: image_id, label: name, description: description}",
			},
		},
		"instance_types": {
			"id":     "instance_types",
			"type":   "api",
			"api":    "/api/v1/aws/ec2/instance-types",
			"method": "GET",
			"params": map[string]interface{}{
				"region": "${providers.aws.region}",
			},
			"cache": map[string]interface{}{
				"ttl": 3600,
			},
			"transform": map[string]interface{}{
				"type":       "jmespath",
				"expression": "data.instance_types[*].{value: instance_type, label: instance_type, vcpu: vcpu_info.default_vcpus, memory: memory_info.size_in_mib}",
			},
		},
		"availability_zones": {
			"id":     "availability_zones",
			"type":   "api",
			"api":    "/api/v1/aws/ec2/availability-zones",
			"method": "GET",
			"params": map[string]interface{}{
				"region": "${providers.aws.region}",
			},
			"cache": map[string]interface{}{
				"ttl": 3600,
			},
			"transform": map[string]interface{}{
				"type":       "jmespath",
				"expression": "data.availability_zones[*].{value: zone_name, label: zone_name}",
			},
		},
		"subnet_list": {
			"id":     "subnet_list",
			"type":   "api",
			"api":    "/api/v1/aws/ec2/subnets",
			"method": "GET",
			"params": map[string]interface{}{
				"region": "${providers.aws.region}",
				"vpc_id": "${fields.vpc_id}",
			},
			"cache": map[string]interface{}{
				"ttl": 300,
			},
			"transform": map[string]interface{}{
				"type":       "jmespath",
				"expression": "data.subnets[*].{value: subnet_id, label: tags.Name || subnet_id, group: availability_zone}",
			},
			"dependsOn": []string{"vpc_id"},
		},
		"security_groups": {
			"id":     "security_groups",
			"type":   "api",
			"api":    "/api/v1/aws/ec2/security-groups",
			"method": "GET",
			"params": map[string]interface{}{
				"region": "${providers.aws.region}",
				"vpc_id": "${fields.vpc_id}",
			},
			"cache": map[string]interface{}{
				"ttl": 300,
			},
			"transform": map[string]interface{}{
				"type":       "jmespath",
				"expression": "data.security_groups[*].{value: group_id, label: group_name, description: description}",
			},
			"dependsOn": []string{"vpc_id"},
		},
		"key_pairs": {
			"id":     "key_pairs",
			"type":   "api",
			"api":    "/api/v1/aws/ec2/key-pairs",
			"method": "GET",
			"params": map[string]interface{}{
				"region": "${providers.aws.region}",
			},
			"cache": map[string]interface{}{
				"ttl": 300,
			},
			"transform": map[string]interface{}{
				"type":       "jmespath",
				"expression": "data.key_pairs[*].{value: key_name, label: key_name}",
			},
		},
		"iam_instance_profiles": {
			"id":     "iam_instance_profiles",
			"type":   "api",
			"api":    "/api/v1/aws/iam/instance-profiles",
			"method": "GET",
			"cache": map[string]interface{}{
				"ttl": 300,
			},
			"transform": map[string]interface{}{
				"type":       "jmespath",
				"expression": "data.instance_profiles[*].{value: instance_profile_name, label: instance_profile_name, arn: arn}",
			},
		},
	}

	if config, ok := sourceConfigs[sourceID]; ok {
		return config
	}

	// 返回通用配置
	return map[string]interface{}{
		"id":     sourceID,
		"type":   "api",
		"api":    fmt.Sprintf("/api/v1/%s/%s", provider, sourceID),
		"method": "GET",
		"cache": map[string]interface{}{
			"ttl": 300,
		},
	}
}

// ToJSON 将解析结果转换为 JSON 字符串
func (r *ParseResult) ToJSON() (string, error) {
	data, err := json.MarshalIndent(r.OpenAPISchema, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
