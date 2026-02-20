// tf2openapi - Terraform variables.tf and outputs.tf to OpenAPI v3 JSON Schema converter
//
// Usage:
//   go run main.go -f /path/to/variables.tf -o output.json
//   go run main.go -f /path/to/variables.tf -outputs /path/to/outputs.tf -o output.json
//   go run main.go -f /path/to/variables.tf -o output.json -name "My Module" -version "1.0.0"
//
// This tool parses Terraform variable and output definitions and generates an OpenAPI v3 compatible
// JSON Schema that can be used with the IAC Platform's dynamic form renderer.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"
)

// Global variable to store file content for type extraction
var fileContent []byte

// OpenAPISchema represents the root OpenAPI v3 schema structure
type OpenAPISchema struct {
	OpenAPI      string        `json:"openapi"`
	Info         Info          `json:"info"`
	Components   Components    `json:"components"`
	XIACPlatform *XIACPlatform `json:"x-iac-platform,omitempty"`
}

// Info contains module metadata
type Info struct {
	Title         string `json:"title"`
	Version       string `json:"version"`
	Description   string `json:"description,omitempty"`
	XModuleSource string `json:"x-module-source,omitempty"`
	XProvider     string `json:"x-provider,omitempty"`
}

// Components contains the schemas
type Components struct {
	Schemas map[string]*JSONSchema `json:"schemas"`
}

// JSONSchema represents a JSON Schema object
type JSONSchema struct {
	Type                 string                 `json:"type,omitempty"`
	Title                string                 `json:"title,omitempty"`
	Description          string                 `json:"description,omitempty"`
	Default              interface{}            `json:"default,omitempty"`
	Enum                 []interface{}          `json:"enum,omitempty"`
	Properties           map[string]*JSONSchema `json:"properties,omitempty"`
	Items                *JSONSchema            `json:"items,omitempty"`
	Required             []string               `json:"required,omitempty"`
	AdditionalProperties interface{}            `json:"additionalProperties,omitempty"`
	MinLength            *int                   `json:"minLength,omitempty"`
	MaxLength            *int                   `json:"maxLength,omitempty"`
	Minimum              *float64               `json:"minimum,omitempty"`
	Maximum              *float64               `json:"maximum,omitempty"`
	Pattern              string                 `json:"pattern,omitempty"`
	Format               string                 `json:"format,omitempty"`
	Nullable             *bool                  `json:"nullable,omitempty"`
	Sensitive            *bool                  `json:"x-sensitive,omitempty"`
	XValidation          []XValidationRule      `json:"x-validation,omitempty"`       // Custom validation rules from Terraform
	XAlias               string                 `json:"x-alias,omitempty"`            // Chinese alias for outputs
	XValueExpression     string                 `json:"x-value-expression,omitempty"` // Value expression for outputs
}

// XValidationRule represents a Terraform validation rule
type XValidationRule struct {
	Condition    string `json:"condition,omitempty"`
	ErrorMessage string `json:"errorMessage,omitempty"`
	Pattern      string `json:"pattern,omitempty"` // Extracted regex pattern if applicable
}

// XIACPlatform contains IAC Platform specific UI configuration
type XIACPlatform struct {
	UI         *UIConfig         `json:"ui,omitempty"`
	Outputs    *OutputsConfig    `json:"outputs,omitempty"`
	Validation *ValidationConfig `json:"validation,omitempty"`
	External   *ExternalConfig   `json:"external,omitempty"`
}

// OutputsConfig contains module output configuration for smart hints
type OutputsConfig struct {
	Description string       `json:"description,omitempty"`
	Items       []OutputItem `json:"items,omitempty"`
}

// OutputItem represents a single output definition
type OutputItem struct {
	Name            string `json:"name"`
	Alias           string `json:"alias,omitempty"`
	Type            string `json:"type"`
	Description     string `json:"description,omitempty"`
	Sensitive       bool   `json:"sensitive,omitempty"`
	ValueExpression string `json:"valueExpression,omitempty"`
	Deprecated      string `json:"deprecated,omitempty"`
	Group           string `json:"group,omitempty"`
}

// ExternalConfig contains external data source configuration
type ExternalConfig struct {
	Sources []ExternalSource `json:"sources,omitempty"`
}

// ExternalSource defines an external data source
type ExternalSource struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"`
	API       string            `json:"api"`
	Method    string            `json:"method,omitempty"`
	Params    map[string]string `json:"params,omitempty"`
	Cache     *CacheConfig      `json:"cache,omitempty"`
	Transform *TransformConfig  `json:"transform,omitempty"`
	DependsOn []string          `json:"dependsOn,omitempty"`
}

// CacheConfig defines cache settings for external data
type CacheConfig struct {
	TTL int    `json:"ttl,omitempty"`
	Key string `json:"key,omitempty"`
}

// TransformConfig defines data transformation rules
type TransformConfig struct {
	Type       string `json:"type,omitempty"`
	Expression string `json:"expression,omitempty"`
}

// UIConfig contains UI rendering configuration
type UIConfig struct {
	Fields map[string]*FieldConfig `json:"fields,omitempty"`
	Groups []GroupConfig           `json:"groups,omitempty"`
	Layout *LayoutConfig           `json:"layout,omitempty"`
}

// FieldConfig contains field-specific UI configuration
type FieldConfig struct {
	Widget            string   `json:"widget,omitempty"`
	Label             string   `json:"label,omitempty"`
	Group             string   `json:"group,omitempty"`
	Order             int      `json:"order,omitempty"`
	HiddenByDefault   bool     `json:"hiddenByDefault,omitempty"`
	Placeholder       string   `json:"placeholder,omitempty"`
	Help              string   `json:"help,omitempty"`
	Source            string   `json:"source,omitempty"`            // External data source ID
	Searchable        bool     `json:"searchable,omitempty"`        // Enable search in select
	AllowCustom       bool     `json:"allowCustom,omitempty"`       // Allow custom input
	CustomPlaceholder string   `json:"customPlaceholder,omitempty"` // Placeholder for custom input
	DependsOn         []string `json:"dependsOn,omitempty"`         // Field dependencies
	GroupBy           string   `json:"groupBy,omitempty"`           // Group options by field
	EmptyText         string   `json:"emptyText,omitempty"`         // Text when no options
	AllowEmpty        bool     `json:"allowEmpty,omitempty"`        // Allow empty selection
	EmptyLabel        string   `json:"emptyLabel,omitempty"`        // Label for empty option
}

// GroupConfig defines a field group
type GroupConfig struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Icon            string `json:"icon,omitempty"`
	Order           int    `json:"order"`
	DefaultExpanded bool   `json:"defaultExpanded,omitempty"`
	HiddenByDefault bool   `json:"hiddenByDefault,omitempty"`
}

// LayoutConfig defines the form layout
type LayoutConfig struct {
	Type     string `json:"type"`
	Position string `json:"position"`
}

// ValidationConfig contains validation rules
type ValidationConfig struct {
	Rules []ValidationRule `json:"rules,omitempty"`
}

// ValidationRule defines a validation rule
type ValidationRule struct {
	ID      string   `json:"id"`
	Type    string   `json:"type"`
	Fields  []string `json:"fields,omitempty"`
	Message string   `json:"message,omitempty"`
}

// VariableAnnotations represents parsed annotations from comments
type VariableAnnotations struct {
	Level         string   // basic/advanced
	Group         string   // custom group ID
	Order         int      // display order
	Hidden        bool     // hidden by default
	Alias         string   // Chinese alias
	Computed      bool     // computed field (readonly)
	ForceNew      bool     // change requires resource recreation
	WriteOnly     bool     // write-only field
	Deprecated    string   // deprecation message
	ConflictsWith []string // conflicting fields
	ExactlyOneOf  []string // exactly one must be set
	AtLeastOneOf  []string // at least one must be set
	RequiredWith  []string // required together
	Widget        string   // UI widget type
	Placeholder   string   // placeholder text
	Prefix        string   // input prefix
	Suffix        string   // input suffix
	Source        string   // External data source ID
	Searchable    bool     // Enable search in select
	AllowCustom   bool     // Allow custom input
	DependsOn     []string // Field dependencies for data source
}

// OutputAnnotations represents parsed annotations from output comments
type OutputAnnotations struct {
	Alias      string // Chinese alias
	Sensitive  bool   // sensitive output
	Deprecated string // deprecation message
	Group      string // group for documentation
}

// TerraformVariable represents a parsed Terraform variable
type TerraformVariable struct {
	Name        string
	Type        string
	Description string
	Default     interface{}
	HasDefault  bool // true if default attribute is defined (even if value is null)
	Sensitive   bool
	Nullable    bool
	Validation  []TerraformValidation
	Level       string              // "basic" or "advanced" (default: "advanced")
	Annotations VariableAnnotations // All parsed annotations
}

// TerraformOutput represents a parsed Terraform output
type TerraformOutput struct {
	Name        string
	Description string
	Value       string // The value expression as string
	Sensitive   bool
	DependsOn   []string
	Annotations OutputAnnotations
}

// TerraformValidation represents a Terraform validation block
type TerraformValidation struct {
	Condition    string
	ErrorMessage string
}

func main() {
	// Parse command line flags
	inputFile := flag.String("f", "", "Input variables.tf file path (required)")
	outputsFile := flag.String("outputs", "", "Input outputs.tf file path (optional)")
	outputFile := flag.String("o", "", "Output JSON file path (required)")
	moduleName := flag.String("name", "", "Module name (optional, defaults to directory name)")
	moduleVersion := flag.String("version", "1.0.0", "Module version")
	moduleSource := flag.String("source", "", "Module source (e.g., terraform-aws-modules/s3-bucket/aws)")
	provider := flag.String("provider", "", "Provider name (e.g., aws, azure, gcp)")
	layoutPosition := flag.String("layout", "top", "Tab layout position: 'top' or 'left'")

	flag.Parse()

	if *inputFile == "" || *outputFile == "" {
		fmt.Println("Usage: tf2openapi -f <variables.tf> -o <output.json> [-outputs <outputs.tf>]")
		fmt.Println("\nOptions:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Read variables file
	content, err := os.ReadFile(*inputFile)
	if err != nil {
		log.Fatalf("Failed to read input file: %v", err)
	}

	// Parse variables
	variables, err := parseVariables(string(content), *inputFile)
	if err != nil {
		log.Fatalf("Failed to parse variables: %v", err)
	}

	fmt.Printf("Parsed %d variables from %s\n", len(variables), *inputFile)

	// Parse outputs if provided
	var outputs []TerraformOutput
	if *outputsFile != "" {
		outputsContent, err := os.ReadFile(*outputsFile)
		if err != nil {
			log.Fatalf("Failed to read outputs file: %v", err)
		}

		outputs, err = parseOutputs(string(outputsContent), *outputsFile)
		if err != nil {
			log.Fatalf("Failed to parse outputs: %v", err)
		}

		fmt.Printf("Parsed %d outputs from %s\n", len(outputs), *outputsFile)
	}

	// Determine module name
	name := *moduleName
	if name == "" {
		name = filepath.Base(filepath.Dir(*inputFile))
		if name == "." || name == "" {
			name = "Terraform Module"
		}
	}

	// Generate OpenAPI schema
	schema := generateOpenAPISchema(variables, outputs, name, *moduleVersion, *moduleSource, *provider, *layoutPosition)

	// Write output
	output, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal JSON: %v", err)
	}

	err = os.WriteFile(*outputFile, output, 0644)
	if err != nil {
		log.Fatalf("Failed to write output file: %v", err)
	}

	fmt.Printf("Generated OpenAPI schema: %s\n", *outputFile)
	fmt.Printf("  - Module: %s v%s\n", name, *moduleVersion)
	fmt.Printf("  - Variables: %d\n", len(variables))
	fmt.Printf("  - Outputs: %d\n", len(outputs))
	fmt.Printf("  - Layout: %s\n", *layoutPosition)
}

// parseVariables parses Terraform variables from HCL content
func parseVariables(content string, filename string) ([]TerraformVariable, error) {
	// Store content globally for type extraction
	fileContent = []byte(content)

	// First, extract all annotations from comments
	allAnnotations := extractAllAnnotations(content)

	parser := hclparse.NewParser()
	file, diags := parser.ParseHCL([]byte(content), filename)
	if diags.HasErrors() {
		return nil, fmt.Errorf("HCL parse error: %s", diags.Error())
	}

	var variables []TerraformVariable

	// Get the body content
	body := file.Body
	bodyContent, _, diags := body.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "variable", LabelNames: []string{"name"}},
		},
	})
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to get body content: %s", diags.Error())
	}

	for _, block := range bodyContent.Blocks {
		if block.Type != "variable" {
			continue
		}

		varName := block.Labels[0]
		variable := TerraformVariable{
			Name:  varName,
			Level: "advanced", // Default to advanced
		}

		// Check if there are annotations for this variable
		if ann, ok := allAnnotations[varName]; ok {
			variable.Annotations = ann
			if ann.Level != "" {
				variable.Level = ann.Level
			}
		}

		// Parse variable attributes
		attrs, diags := block.Body.JustAttributes()
		if diags.HasErrors() {
			// Try partial content for nested blocks
			blockContent, _, _ := block.Body.PartialContent(&hcl.BodySchema{
				Attributes: []hcl.AttributeSchema{
					{Name: "type"},
					{Name: "description"},
					{Name: "default"},
					{Name: "sensitive"},
					{Name: "nullable"},
				},
				Blocks: []hcl.BlockHeaderSchema{
					{Type: "validation"},
				},
			})

			if blockContent != nil {
				for name, attr := range blockContent.Attributes {
					parseVariableAttribute(&variable, name, attr)
				}

				// Parse validation blocks
				for _, valBlock := range blockContent.Blocks {
					if valBlock.Type == "validation" {
						validation := parseValidationBlock(valBlock)
						variable.Validation = append(variable.Validation, validation)
					}
				}
			}
		} else {
			for name, attr := range attrs {
				parseVariableAttribute(&variable, name, attr)
			}
		}

		variables = append(variables, variable)
	}

	// Sort variables by name
	sort.Slice(variables, func(i, j int) bool {
		return variables[i].Name < variables[j].Name
	})

	return variables, nil
}

// parseOutputs parses Terraform outputs from HCL content
func parseOutputs(content string, filename string) ([]TerraformOutput, error) {
	// Store content globally for value extraction
	fileContent = []byte(content)

	// First, extract all output annotations from comments
	allAnnotations := extractAllOutputAnnotations(content)

	parser := hclparse.NewParser()
	file, diags := parser.ParseHCL([]byte(content), filename)
	if diags.HasErrors() {
		return nil, fmt.Errorf("HCL parse error: %s", diags.Error())
	}

	var outputs []TerraformOutput

	// Get the body content
	body := file.Body
	bodyContent, _, diags := body.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "output", LabelNames: []string{"name"}},
		},
	})
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to get body content: %s", diags.Error())
	}

	for _, block := range bodyContent.Blocks {
		if block.Type != "output" {
			continue
		}

		outputName := block.Labels[0]
		output := TerraformOutput{
			Name: outputName,
		}

		// Check if there are annotations for this output
		if ann, ok := allAnnotations[outputName]; ok {
			output.Annotations = ann
		}

		// Parse output attributes
		attrs, diags := block.Body.JustAttributes()
		if diags.HasErrors() {
			// Try partial content
			blockContent, _, _ := block.Body.PartialContent(&hcl.BodySchema{
				Attributes: []hcl.AttributeSchema{
					{Name: "description"},
					{Name: "value"},
					{Name: "sensitive"},
				},
			})

			if blockContent != nil {
				for name, attr := range blockContent.Attributes {
					parseOutputAttribute(&output, name, attr)
				}
			}
		} else {
			for name, attr := range attrs {
				parseOutputAttribute(&output, name, attr)
			}
		}

		outputs = append(outputs, output)
	}

	// Sort outputs by name
	sort.Slice(outputs, func(i, j int) bool {
		return outputs[i].Name < outputs[j].Name
	})

	return outputs, nil
}

// parseOutputAttribute parses a single output attribute
func parseOutputAttribute(output *TerraformOutput, name string, attr *hcl.Attribute) {
	switch name {
	case "description":
		val, _ := attr.Expr.Value(nil)
		if val.Type() == cty.String {
			output.Description = val.AsString()
		}
	case "value":
		// Extract value expression as raw string from source
		rng := attr.Expr.Range()
		if len(fileContent) > 0 && rng.Start.Byte < len(fileContent) && rng.End.Byte <= len(fileContent) {
			output.Value = strings.TrimSpace(string(fileContent[rng.Start.Byte:rng.End.Byte]))
		}
	case "sensitive":
		val, _ := attr.Expr.Value(nil)
		if val.Type() == cty.Bool {
			output.Sensitive = val.True()
		}
	}
}

// extractAllOutputAnnotations extracts all annotations from output comments
func extractAllOutputAnnotations(content string) map[string]OutputAnnotations {
	annotations := make(map[string]OutputAnnotations)

	// Find output blocks with their bodies
	outputPattern := regexp.MustCompile(`output\s+"([^"]+)"\s*\{`)
	outputMatches := outputPattern.FindAllStringSubmatchIndex(content, -1)

	for _, match := range outputMatches {
		if len(match) >= 4 {
			outputName := content[match[2]:match[3]]
			startIdx := match[1] // Position after opening brace

			// Find the matching closing brace
			braceCount := 1
			endIdx := startIdx
			for i := startIdx; i < len(content) && braceCount > 0; i++ {
				if content[i] == '{' {
					braceCount++
				} else if content[i] == '}' {
					braceCount--
				}
				endIdx = i
			}

			if braceCount == 0 {
				outputBody := content[startIdx:endIdx]
				ann := parseOutputAnnotationsFromBody(outputBody)
				if ann.Alias != "" || ann.Sensitive || ann.Deprecated != "" || ann.Group != "" {
					annotations[outputName] = ann
				}
			}
		}
	}

	return annotations
}

// parseOutputAnnotationsFromBody parses annotations from an output body
func parseOutputAnnotationsFromBody(outputBody string) OutputAnnotations {
	ann := OutputAnnotations{}

	// Find all # comments that contain @
	commentPattern := regexp.MustCompile(`#\s*@?([^\n]+)`)
	matches := commentPattern.FindAllStringSubmatch(outputBody, -1)

	for _, match := range matches {
		if len(match) >= 2 {
			commentText := match[1]
			// Parse key:value pairs
			pairPattern := regexp.MustCompile(`@?(\w+):([^\s]+)`)
			pairs := pairPattern.FindAllStringSubmatch(commentText, -1)

			for _, pair := range pairs {
				if len(pair) >= 3 {
					key := strings.ToLower(pair[1])
					value := pair[2]

					switch key {
					case "alias":
						ann.Alias = value
					case "sensitive":
						ann.Sensitive = value == "true"
					case "deprecated":
						ann.Deprecated = value
					case "group":
						ann.Group = value
					}
				}
			}
		}
	}

	return ann
}

// parseVariableAttribute parses a single variable attribute
func parseVariableAttribute(variable *TerraformVariable, name string, attr *hcl.Attribute) {
	switch name {
	case "type":
		// Extract type as string from the expression
		variable.Type = extractTypeString(attr)
	case "description":
		val, _ := attr.Expr.Value(nil)
		if val.Type() == cty.String {
			variable.Description = val.AsString()
		}
	case "default":
		variable.HasDefault = true // Mark that default attribute exists
		variable.Default = extractDefaultValue(attr)
	case "sensitive":
		val, _ := attr.Expr.Value(nil)
		if val.Type() == cty.Bool {
			variable.Sensitive = val.True()
		}
	case "nullable":
		val, _ := attr.Expr.Value(nil)
		if val.Type() == cty.Bool {
			variable.Nullable = val.True()
		}
	}
}

// extractTypeString extracts the type string from a type expression
func extractTypeString(attr *hcl.Attribute) string {
	// Get the source range
	rng := attr.Expr.Range()

	// Extract from global file content using byte offsets
	if len(fileContent) > 0 && rng.Start.Byte < len(fileContent) && rng.End.Byte <= len(fileContent) {
		typeStr := string(fileContent[rng.Start.Byte:rng.End.Byte])
		typeStr = strings.TrimSpace(typeStr)
		return typeStr
	}

	// Fallback: try to evaluate and get type info
	val, diags := attr.Expr.Value(nil)
	if !diags.HasErrors() && val.Type() == cty.String {
		return val.AsString()
	}

	return "string"
}

// extractDefaultValue extracts the default value from an attribute
func extractDefaultValue(attr *hcl.Attribute) interface{} {
	val, diags := attr.Expr.Value(nil)
	if diags.HasErrors() {
		return nil
	}
	return ctyValueToInterface(val)
}

// ctyValueToInterface converts a cty.Value to a Go interface{}
func ctyValueToInterface(val cty.Value) interface{} {
	if val.IsNull() {
		return nil
	}

	switch {
	case val.Type() == cty.String:
		return val.AsString()
	case val.Type() == cty.Number:
		f, _ := val.AsBigFloat().Float64()
		return f
	case val.Type() == cty.Bool:
		return val.True()
	case val.Type().IsListType() || val.Type().IsTupleType():
		var result []interface{}
		for it := val.ElementIterator(); it.Next(); {
			_, v := it.Element()
			result = append(result, ctyValueToInterface(v))
		}
		return result
	case val.Type().IsMapType() || val.Type().IsObjectType():
		result := make(map[string]interface{})
		for it := val.ElementIterator(); it.Next(); {
			k, v := it.Element()
			result[k.AsString()] = ctyValueToInterface(v)
		}
		return result
	default:
		return nil
	}
}

// parseValidationBlock parses a validation block
func parseValidationBlock(block *hcl.Block) TerraformValidation {
	validation := TerraformValidation{}

	attrs, _ := block.Body.JustAttributes()
	for name, attr := range attrs {
		switch name {
		case "error_message":
			val, _ := attr.Expr.Value(nil)
			if val.Type() == cty.String {
				validation.ErrorMessage = val.AsString()
			}
		case "condition":
			// Extract condition as raw string from source
			rng := attr.Expr.Range()
			if len(fileContent) > 0 && rng.Start.Byte < len(fileContent) && rng.End.Byte <= len(fileContent) {
				validation.Condition = string(fileContent[rng.Start.Byte:rng.End.Byte])
			}
		}
	}

	return validation
}

// generateOpenAPISchema generates an OpenAPI v3 schema from parsed variables and outputs
func generateOpenAPISchema(variables []TerraformVariable, outputs []TerraformOutput, moduleName, version, source, provider, layoutPosition string) *OpenAPISchema {
	schema := &OpenAPISchema{
		OpenAPI: "3.1.0",
		Info: Info{
			Title:       moduleName,
			Version:     version,
			Description: fmt.Sprintf("Terraform module: %s", moduleName),
		},
		Components: Components{
			Schemas: make(map[string]*JSONSchema),
		},
	}

	if source != "" {
		schema.Info.XModuleSource = source
	}
	if provider != "" {
		schema.Info.XProvider = provider
	}

	// Create the main input schema
	inputSchema := &JSONSchema{
		Type:       "object",
		Properties: make(map[string]*JSONSchema),
		Required:   []string{},
	}

	// Count basic and advanced variables for statistics
	basicCount := 0
	advancedCount := 0
	for _, v := range variables {
		if v.Level == "basic" {
			basicCount++
		} else {
			advancedCount++
		}
	}
	fmt.Printf("  - Basic variables: %d\n", basicCount)
	fmt.Printf("  - Advanced variables: %d\n", advancedCount)

	// UI configuration with basic/advanced groups
	uiConfig := &UIConfig{
		Fields: make(map[string]*FieldConfig),
		Groups: []GroupConfig{
			{ID: "basic", Title: "基础配置", Icon: "settings", Order: 1, DefaultExpanded: true},
			{ID: "advanced", Title: "高级配置", Icon: "code", Order: 2, DefaultExpanded: false, HiddenByDefault: true},
		},
		Layout: &LayoutConfig{
			Type:     "tabs",
			Position: layoutPosition,
		},
	}

	// Count variables with validation
	validationCount := 0
	for _, v := range variables {
		if len(v.Validation) > 0 {
			validationCount++
		}
	}
	if validationCount > 0 {
		fmt.Printf("  - Variables with validation: %d\n", validationCount)
	}

	// Convert each variable to JSON Schema
	basicOrder := 0
	advancedOrder := 0
	for _, v := range variables {
		propSchema := terraformTypeToJSONSchema(v.Type)
		propSchema.Title = formatTitle(v.Name)
		propSchema.Description = v.Description

		if v.Default != nil {
			propSchema.Default = v.Default
		}

		if v.Sensitive {
			propSchema.Sensitive = &v.Sensitive
		}

		if v.Nullable {
			propSchema.Nullable = &v.Nullable
		}

		// Add validation rules
		if len(v.Validation) > 0 {
			for _, val := range v.Validation {
				rule := XValidationRule{
					Condition:    val.Condition,
					ErrorMessage: val.ErrorMessage,
				}
				// Try to extract regex pattern from condition
				if pattern := extractRegexPattern(val.Condition); pattern != "" {
					rule.Pattern = pattern
					// Also set the standard JSON Schema pattern if it's a simple regex
					if propSchema.Pattern == "" {
						propSchema.Pattern = pattern
					}
				}
				propSchema.XValidation = append(propSchema.XValidation, rule)
			}
		}

		inputSchema.Properties[v.Name] = propSchema

		// Determine if required: only if no default attribute is defined
		// In Terraform, a variable is required only if it has no default value at all
		// Variables with default = null are NOT required
		isRequired := !v.HasDefault
		if isRequired {
			inputSchema.Required = append(inputSchema.Required, v.Name)
		}

		// Generate UI field config
		fieldConfig := &FieldConfig{
			Widget: inferWidget(v.Type, v.Sensitive),
			Label:  formatTitle(v.Name),
		}

		// Apply annotations to field config
		if v.Annotations.Widget != "" {
			fieldConfig.Widget = v.Annotations.Widget
		}
		if v.Annotations.Alias != "" {
			fieldConfig.Label = v.Annotations.Alias
		}
		if v.Annotations.Placeholder != "" {
			fieldConfig.Placeholder = v.Annotations.Placeholder
		}
		if v.Annotations.Source != "" {
			fieldConfig.Source = v.Annotations.Source
		}
		if v.Annotations.Searchable {
			fieldConfig.Searchable = true
		}
		if v.Annotations.AllowCustom {
			fieldConfig.AllowCustom = true
		}
		if len(v.Annotations.DependsOn) > 0 {
			fieldConfig.DependsOn = v.Annotations.DependsOn
		}

		// Assign to group based on @level annotation (default: advanced)
		if v.Level == "basic" {
			basicOrder++
			fieldConfig.Group = "basic"
			fieldConfig.Order = basicOrder
		} else {
			advancedOrder++
			fieldConfig.Group = "advanced"
			fieldConfig.Order = advancedOrder
			fieldConfig.HiddenByDefault = true
		}

		if v.Description != "" {
			fieldConfig.Help = v.Description
		}

		uiConfig.Fields[v.Name] = fieldConfig
	}

	schema.Components.Schemas["ModuleInput"] = inputSchema

	// Create the output schema if outputs are provided
	if len(outputs) > 0 {
		outputSchema := &JSONSchema{
			Type:        "object",
			Description: "Module output definitions (read-only, for smart hints)",
			Properties:  make(map[string]*JSONSchema),
		}

		var outputItems []OutputItem
		for _, o := range outputs {
			// Infer type from value expression (simplified)
			outputType := inferOutputType(o.Value)

			propSchema := &JSONSchema{
				Type:        outputType,
				Description: o.Description,
			}

			// Add alias if available
			if o.Annotations.Alias != "" {
				propSchema.XAlias = o.Annotations.Alias
			}

			// Add value expression
			if o.Value != "" {
				propSchema.XValueExpression = o.Value
			}

			// Add sensitive flag
			if o.Sensitive || o.Annotations.Sensitive {
				sensitive := true
				propSchema.Sensitive = &sensitive
			}

			outputSchema.Properties[o.Name] = propSchema

			// Create output item for x-iac-platform.outputs
			item := OutputItem{
				Name:            o.Name,
				Type:            outputType,
				Description:     o.Description,
				Sensitive:       o.Sensitive || o.Annotations.Sensitive,
				ValueExpression: o.Value,
			}
			if o.Annotations.Alias != "" {
				item.Alias = o.Annotations.Alias
			}
			if o.Annotations.Deprecated != "" {
				item.Deprecated = o.Annotations.Deprecated
			}
			if o.Annotations.Group != "" {
				item.Group = o.Annotations.Group
			}
			outputItems = append(outputItems, item)
		}

		schema.Components.Schemas["ModuleOutput"] = outputSchema

		// Count sensitive outputs
		sensitiveCount := 0
		for _, o := range outputs {
			if o.Sensitive || o.Annotations.Sensitive {
				sensitiveCount++
			}
		}
		if sensitiveCount > 0 {
			fmt.Printf("  - Sensitive outputs: %d\n", sensitiveCount)
		}

		// Add outputs config to x-iac-platform
		schema.XIACPlatform = &XIACPlatform{
			UI: uiConfig,
			Outputs: &OutputsConfig{
				Description: "Module output list (for smart hints)",
				Items:       outputItems,
			},
		}
	} else {
		schema.XIACPlatform = &XIACPlatform{
			UI: uiConfig,
		}
	}

	// Collect all unique data sources referenced by fields
	sourcesMap := make(map[string]bool)
	for _, v := range variables {
		if v.Annotations.Source != "" {
			sourcesMap[v.Annotations.Source] = true
		}
	}

	// Generate external data source definitions
	var externalSources []ExternalSource
	for sourceID := range sourcesMap {
		source := generateExternalSource(sourceID, provider)
		if source != nil {
			externalSources = append(externalSources, *source)
		}
	}

	if len(externalSources) > 0 {
		schema.XIACPlatform.External = &ExternalConfig{
			Sources: externalSources,
		}
		fmt.Printf("  - External data sources: %d\n", len(externalSources))
	}

	return schema
}

// inferOutputType infers the output type from the value expression
func inferOutputType(value string) string {
	value = strings.TrimSpace(value)

	// Check for common patterns
	if strings.HasPrefix(value, "[") || strings.Contains(value, "tolist(") || strings.Contains(value, "list(") {
		return "array"
	}
	if strings.HasPrefix(value, "{") || strings.Contains(value, "tomap(") || strings.Contains(value, "map(") {
		return "object"
	}
	if strings.Contains(value, "tobool(") || value == "true" || value == "false" {
		return "boolean"
	}
	if strings.Contains(value, "tonumber(") {
		return "number"
	}

	// Default to string
	return "string"
}

// generateExternalSource generates an external data source definition based on source ID
func generateExternalSource(sourceID, provider string) *ExternalSource {
	// Define common AWS data sources
	awsSources := map[string]ExternalSource{
		"ami_list": {
			ID:     "ami_list",
			Type:   "api",
			API:    "/api/v1/aws/ec2/images",
			Method: "GET",
			Params: map[string]string{
				"region":       "${providers.aws.region}",
				"owners":       "amazon,self",
				"architecture": "x86_64",
			},
			Cache: &CacheConfig{TTL: 300, Key: "ami_list_${providers.aws.region}"},
			Transform: &TransformConfig{
				Type:       "jmespath",
				Expression: "data.images[*].{value: image_id, label: name, description: description}",
			},
		},
		"instance_types": {
			ID:     "instance_types",
			Type:   "api",
			API:    "/api/v1/aws/ec2/instance-types",
			Method: "GET",
			Params: map[string]string{
				"region": "${providers.aws.region}",
			},
			Cache: &CacheConfig{TTL: 3600},
			Transform: &TransformConfig{
				Type:       "jmespath",
				Expression: "data.instance_types[*].{value: instance_type, label: instance_type, vcpu: vcpu_info.default_vcpus, memory: memory_info.size_in_mib}",
			},
		},
		"availability_zones": {
			ID:     "availability_zones",
			Type:   "api",
			API:    "/api/v1/aws/ec2/availability-zones",
			Method: "GET",
			Params: map[string]string{
				"region": "${providers.aws.region}",
			},
			Cache: &CacheConfig{TTL: 3600},
			Transform: &TransformConfig{
				Type:       "jmespath",
				Expression: "data.availability_zones[*].{value: zone_name, label: zone_name}",
			},
		},
		"vpc_list": {
			ID:     "vpc_list",
			Type:   "api",
			API:    "/api/v1/aws/ec2/vpcs",
			Method: "GET",
			Params: map[string]string{
				"region": "${providers.aws.region}",
			},
			Cache: &CacheConfig{TTL: 300},
			Transform: &TransformConfig{
				Type:       "jmespath",
				Expression: "data.vpcs[*].{value: vpc_id, label: tags.Name || vpc_id}",
			},
		},
		"subnet_list": {
			ID:     "subnet_list",
			Type:   "api",
			API:    "/api/v1/aws/ec2/subnets",
			Method: "GET",
			Params: map[string]string{
				"region": "${providers.aws.region}",
				"vpc_id": "${fields.vpc_id}",
			},
			Cache:     &CacheConfig{TTL: 300},
			DependsOn: []string{"vpc_id"},
			Transform: &TransformConfig{
				Type:       "jmespath",
				Expression: "data.subnets[*].{value: subnet_id, label: tags.Name || subnet_id, group: availability_zone}",
			},
		},
		"security_groups": {
			ID:     "security_groups",
			Type:   "api",
			API:    "/api/v1/aws/ec2/security-groups",
			Method: "GET",
			Params: map[string]string{
				"region": "${providers.aws.region}",
				"vpc_id": "${fields.vpc_id}",
			},
			Cache:     &CacheConfig{TTL: 300},
			DependsOn: []string{"vpc_id"},
			Transform: &TransformConfig{
				Type:       "jmespath",
				Expression: "data.security_groups[*].{value: group_id, label: group_name, description: description}",
			},
		},
		"key_pairs": {
			ID:     "key_pairs",
			Type:   "api",
			API:    "/api/v1/aws/ec2/key-pairs",
			Method: "GET",
			Params: map[string]string{
				"region": "${providers.aws.region}",
			},
			Cache: &CacheConfig{TTL: 300},
			Transform: &TransformConfig{
				Type:       "jmespath",
				Expression: "data.key_pairs[*].{value: key_name, label: key_name}",
			},
		},
		"iam_instance_profiles": {
			ID:     "iam_instance_profiles",
			Type:   "api",
			API:    "/api/v1/aws/iam/instance-profiles",
			Method: "GET",
			Cache:  &CacheConfig{TTL: 300},
			Transform: &TransformConfig{
				Type:       "jmespath",
				Expression: "data.instance_profiles[*].{value: instance_profile_name, label: instance_profile_name, arn: arn}",
			},
		},
		"kms_keys": {
			ID:     "kms_keys",
			Type:   "api",
			API:    "/api/v1/aws/kms/keys",
			Method: "GET",
			Params: map[string]string{
				"region": "${providers.aws.region}",
			},
			Cache: &CacheConfig{TTL: 300},
			Transform: &TransformConfig{
				Type:       "jmespath",
				Expression: "data.keys[*].{value: arn, label: alias}",
			},
		},
	}

	// Return the source if it exists in our predefined list
	if source, ok := awsSources[sourceID]; ok {
		return &source
	}

	// Return a generic placeholder for unknown sources
	return &ExternalSource{
		ID:     sourceID,
		Type:   "api",
		API:    fmt.Sprintf("/api/v1/%s/%s", provider, sourceID),
		Method: "GET",
		Params: map[string]string{
			"region": "${providers." + provider + ".region}",
		},
		Cache: &CacheConfig{TTL: 300},
	}
}

// terraformTypeToJSONSchema converts a Terraform type string to JSON Schema
func terraformTypeToJSONSchema(tfType string) *JSONSchema {
	tfType = strings.TrimSpace(tfType)

	// Handle common Terraform types
	switch {
	case tfType == "" || tfType == "any":
		return &JSONSchema{Type: "string"}
	case tfType == "string":
		return &JSONSchema{Type: "string"}
	case tfType == "number":
		return &JSONSchema{Type: "number"}
	case tfType == "bool":
		return &JSONSchema{Type: "boolean"}
	case strings.HasPrefix(tfType, "list("):
		inner := extractInnerType(tfType, "list(")
		return &JSONSchema{
			Type:  "array",
			Items: terraformTypeToJSONSchema(inner),
		}
	case strings.HasPrefix(tfType, "set("):
		inner := extractInnerType(tfType, "set(")
		return &JSONSchema{
			Type:  "array",
			Items: terraformTypeToJSONSchema(inner),
		}
	case strings.HasPrefix(tfType, "map("):
		inner := extractInnerType(tfType, "map(")
		return &JSONSchema{
			Type:                 "object",
			AdditionalProperties: terraformTypeToJSONSchema(inner),
		}
	case strings.HasPrefix(tfType, "object("):
		// Parse object type - simplified
		return &JSONSchema{
			Type:       "object",
			Properties: make(map[string]*JSONSchema),
		}
	case strings.HasPrefix(tfType, "tuple("):
		return &JSONSchema{
			Type:  "array",
			Items: &JSONSchema{Type: "string"},
		}
	default:
		// Default to string for unknown types
		return &JSONSchema{Type: "string"}
	}
}

// extractInnerType extracts the inner type from a parameterized type like list(string)
func extractInnerType(tfType, prefix string) string {
	inner := strings.TrimPrefix(tfType, prefix)
	inner = strings.TrimSuffix(inner, ")")
	return strings.TrimSpace(inner)
}

// inferWidget infers the appropriate UI widget based on type
func inferWidget(tfType string, sensitive bool) string {
	if sensitive {
		return "password"
	}

	tfType = strings.TrimSpace(tfType)

	switch {
	case tfType == "bool":
		return "switch"
	case tfType == "number":
		return "number"
	case strings.HasPrefix(tfType, "list(") || strings.HasPrefix(tfType, "set("):
		inner := extractInnerType(tfType, "list(")
		if inner == "" {
			inner = extractInnerType(tfType, "set(")
		}
		if strings.HasPrefix(inner, "object(") {
			return "object-list"
		}
		return "tags"
	case strings.HasPrefix(tfType, "map("):
		return "key-value"
	case strings.HasPrefix(tfType, "object("):
		return "object"
	default:
		return "text"
	}
}

// formatTitle converts snake_case to Title Case
func formatTitle(name string) string {
	words := strings.Split(name, "_")
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + word[1:]
		}
	}
	return strings.Join(words, " ")
}

// extractRegexPattern tries to extract a regex pattern from a Terraform validation condition
func extractRegexPattern(condition string) string {
	regexPattern := regexp.MustCompile(`regex\s*\(\s*"([^"]+)"`)
	matches := regexPattern.FindStringSubmatch(condition)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// extractAllAnnotations extracts all annotations from comments in the content
func extractAllAnnotations(content string) map[string]VariableAnnotations {
	annotations := make(map[string]VariableAnnotations)

	// Find variable blocks with their bodies (handle nested braces)
	varPattern := regexp.MustCompile(`variable\s+"([^"]+)"\s*\{`)
	varMatches := varPattern.FindAllStringSubmatchIndex(content, -1)

	for _, match := range varMatches {
		if len(match) >= 4 {
			varName := content[match[2]:match[3]]
			startIdx := match[1] // Position after opening brace

			// Find the matching closing brace
			braceCount := 1
			endIdx := startIdx
			for i := startIdx; i < len(content) && braceCount > 0; i++ {
				if content[i] == '{' {
					braceCount++
				} else if content[i] == '}' {
					braceCount--
				}
				endIdx = i
			}

			if braceCount == 0 {
				varBody := content[startIdx:endIdx]
				ann := parseAnnotationsFromBody(varBody)
				if ann.Level != "" || ann.Widget != "" || ann.Source != "" || ann.Alias != "" {
					annotations[varName] = ann
				}
			}
		}
	}

	return annotations
}

// parseAnnotationsFromBody parses all annotations from a variable body
func parseAnnotationsFromBody(varBody string) VariableAnnotations {
	ann := VariableAnnotations{}

	// Find all # comments that contain @
	commentPattern := regexp.MustCompile(`#\s*@?([^\n]+)`)
	matches := commentPattern.FindAllStringSubmatch(varBody, -1)

	for _, match := range matches {
		if len(match) >= 2 {
			commentText := match[1]
			// Parse key:value pairs (space or @ separated)
			pairPattern := regexp.MustCompile(`@?(\w+):([^\s]+)`)
			pairs := pairPattern.FindAllStringSubmatch(commentText, -1)

			for _, pair := range pairs {
				if len(pair) >= 3 {
					key := strings.ToLower(pair[1])
					value := pair[2]

					switch key {
					case "level":
						ann.Level = value
					case "group":
						ann.Group = value
					case "alias":
						ann.Alias = value
					case "widget":
						ann.Widget = value
					case "placeholder":
						ann.Placeholder = value
					case "source":
						ann.Source = value
					case "searchable":
						ann.Searchable = value == "true"
					case "allowcustom":
						ann.AllowCustom = value == "true"
					case "dependson":
						ann.DependsOn = strings.Split(value, ",")
					case "hidden":
						ann.Hidden = value == "true"
					case "computed":
						ann.Computed = value == "true"
					case "force_new", "forcenew":
						ann.ForceNew = value == "true"
					case "deprecated":
						ann.Deprecated = value
					case "conflicts_with", "conflictswith":
						ann.ConflictsWith = strings.Split(value, ",")
					case "required_with", "requiredwith":
						ann.RequiredWith = strings.Split(value, ",")
					}
				}
			}
		}
	}

	return ann
}
