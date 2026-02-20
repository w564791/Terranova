package parsers

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// TFVariable represents a Terraform variable
type TFVariable struct {
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Default     interface{}            `json:"default"`
	Description string                 `json:"description"`
	Sensitive   bool                   `json:"sensitive"`
	Nullable    bool                   `json:"nullable"`
	Validation  *TFValidation          `json:"validation,omitempty"`
	RawAttrs    map[string]interface{} `json:"raw_attrs,omitempty"`
}

// TFValidation represents validation block
type TFValidation struct {
	Condition    string `json:"condition"`
	ErrorMessage string `json:"error_message"`
}

// ParseVariablesFile parses a Terraform variables file
func ParseVariablesFile(content string) ([]TFVariable, error) {
	var variables []TFVariable

	// 正则表达式匹配variable块
	// 匹配: variable "name" { ... }
	varBlockRegex := regexp.MustCompile(`variable\s+"([^"]+)"\s*\{([^}]*(?:\{[^}]*\}[^}]*)*)\}`)
	matches := varBlockRegex.FindAllStringSubmatch(content, -1)

	if len(matches) == 0 {
		return nil, fmt.Errorf("no variables found in the file")
	}

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		varName := match[1]
		varBody := match[2]

		variable := TFVariable{
			Name:     varName,
			RawAttrs: make(map[string]interface{}),
		}

		// 解析type
		if typeVal := extractAttribute(varBody, "type"); typeVal != "" {
			variable.Type = typeVal
		} else {
			variable.Type = "string" // 默认类型
		}

		// 解析default
		if defaultVal := extractAttribute(varBody, "default"); defaultVal != "" {
			variable.Default = parseDefaultValue(defaultVal, variable.Type)
		}

		// 解析description
		if desc := extractAttribute(varBody, "description"); desc != "" {
			variable.Description = strings.Trim(desc, `"`)
		}

		// 解析sensitive
		if sensitive := extractAttribute(varBody, "sensitive"); sensitive != "" {
			variable.Sensitive = sensitive == "true"
		}

		// 解析nullable
		if nullable := extractAttribute(varBody, "nullable"); nullable != "" {
			variable.Nullable = nullable == "true"
		}

		// 解析validation块（简化处理）
		if validationBlock := extractValidationBlock(varBody); validationBlock != nil {
			variable.Validation = validationBlock
		}

		variables = append(variables, variable)
	}

	return variables, nil
}

// extractAttribute extracts a simple attribute value from variable body
func extractAttribute(body, attrName string) string {
	// 匹配: attribute = value
	pattern := fmt.Sprintf(`%s\s*=\s*([^\n]+)`, attrName)
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(body)
	if len(matches) > 1 {
		value := strings.TrimSpace(matches[1])
		// 移除末尾的注释
		if idx := strings.Index(value, "#"); idx != -1 {
			value = strings.TrimSpace(value[:idx])
		}
		return value
	}
	return ""
}

// extractValidationBlock extracts validation block
func extractValidationBlock(body string) *TFValidation {
	validationRegex := regexp.MustCompile(`validation\s*\{([^}]+)\}`)
	matches := validationRegex.FindStringSubmatch(body)
	if len(matches) > 1 {
		validationBody := matches[1]
		validation := &TFValidation{}

		if condition := extractAttribute(validationBody, "condition"); condition != "" {
			validation.Condition = condition
		}

		if errorMsg := extractAttribute(validationBody, "error_message"); errorMsg != "" {
			validation.ErrorMessage = strings.Trim(errorMsg, `"`)
		}

		return validation
	}
	return nil
}

// parseDefaultValue parses default value based on type
func parseDefaultValue(value, varType string) interface{} {
	value = strings.TrimSpace(value)

	// 处理null
	if value == "null" {
		return nil
	}

	// 处理字符串
	if strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
		return strings.Trim(value, `"`)
	}

	// 处理布尔值
	if value == "true" {
		return true
	}
	if value == "false" {
		return false
	}

	// 处理数字
	if varType == "number" {
		// 简单处理，实际可能需要更复杂的解析
		return value
	}

	// 处理列表和对象（简化处理）
	if strings.HasPrefix(value, "[") || strings.HasPrefix(value, "{") {
		// 尝试解析为JSON
		var result interface{}
		if err := json.Unmarshal([]byte(value), &result); err == nil {
			return result
		}
	}

	return value
}

// ConvertToSchema converts TFVariable to Schema format
func ConvertToSchema(tfVar TFVariable) map[string]interface{} {
	schema := map[string]interface{}{
		"type":           mapTerraformType(tfVar.Type),
		"required":       false, // 默认不必填
		"description":    tfVar.Description,
		"default":        tfVar.Default,
		"sensitive":      tfVar.Sensitive,
		"hidden_default": true, // 默认隐藏
		"force_new":      false,
		"computed":       false,
	}

	// 添加validation规则（如果存在）
	if tfVar.Validation != nil {
		validationRules := []map[string]interface{}{
			{
				"condition":     tfVar.Validation.Condition,
				"error_message": tfVar.Validation.ErrorMessage,
			},
		}
		schema["validation"] = validationRules
	}

	// 如果没有默认值，可能是必填的
	if tfVar.Default == nil && tfVar.Description != "" {
		// 可以根据描述判断是否必填，这里简化处理
		schema["required"] = false
	}

	return schema
}

// mapTerraformType maps Terraform type to our schema type
func mapTerraformType(tfType string) string {
	tfType = strings.TrimSpace(tfType)

	// 处理基本类型
	switch tfType {
	case "string":
		return "string"
	case "number":
		return "number"
	case "bool", "boolean":
		return "boolean"
	case "list":
		return "list"
	case "map":
		return "map"
	case "set":
		return "set"
	case "object":
		return "object"
	}

	// 处理复杂类型
	if strings.HasPrefix(tfType, "list(") {
		return "list"
	}
	if strings.HasPrefix(tfType, "map(") {
		return "map"
	}
	if strings.HasPrefix(tfType, "set(") {
		return "set"
	}
	if strings.HasPrefix(tfType, "object(") {
		return "object"
	}

	// 默认返回string
	return "string"
}

// ConvertVariablesToSchema converts multiple TFVariables to schema format
func ConvertVariablesToSchema(variables []TFVariable) map[string]interface{} {
	schema := make(map[string]interface{})

	for _, tfVar := range variables {
		schema[tfVar.Name] = ConvertToSchema(tfVar)
	}

	return schema
}
