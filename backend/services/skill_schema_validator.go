package services

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// SkillOutputSchema defines the schema for a skill's output validation
type SkillOutputSchema struct {
	RequiredFields []string            // List of required field names
	RequiredOneOf  [][]string          // Groups of fields where at least one must exist (e.g. [["risk_level","risk_evaluation"]])
	EnumFields     map[string][]string // Map of field name to allowed enum values
}

// SchemaValidationResult contains the result of schema validation
type SchemaValidationResult struct {
	Valid        bool     `json:"schema_valid"`
	MissingFields []string `json:"missing_fields"`
	InvalidEnums  []string `json:"invalid_enum_fields"` // Format: "fieldname=actualvalue"
	Score         int      `json:"score"`                // 100 or 0
	Verdict       string   `json:"verdict"`              // pass | fail
}

// SkillSchemaValidator validates skill outputs against registered schemas
type SkillSchemaValidator struct {
	schemas map[string]SkillOutputSchema
	mu      sync.RWMutex
}

// NewSkillSchemaValidator creates a new schema validator instance
func NewSkillSchemaValidator() *SkillSchemaValidator {
	return &SkillSchemaValidator{
		schemas: make(map[string]SkillOutputSchema),
	}
}

// RegisterSchema registers a schema for a specific skill
func (v *SkillSchemaValidator) RegisterSchema(skillName string, schema SkillOutputSchema) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.schemas[skillName] = schema
}

// LoadSchemasFromDB loads output_schema from all Task Skills' metadata
// Skills with output_schema defined in metadata will be registered automatically
func (v *SkillSchemaValidator) LoadSchemasFromDB(db *gorm.DB) int {
	var skills []models.Skill
	db.Where("layer = ? AND is_active = true", models.SkillLayerTask).Find(&skills)

	loaded := 0
	for _, skill := range skills {
		if skill.Metadata.OutputSchema == nil {
			continue
		}
		os := skill.Metadata.OutputSchema
		schema := SkillOutputSchema{
			RequiredFields: os.RequiredFields,
			RequiredOneOf:  os.RequiredOneOf,
			EnumFields:     os.EnumFields,
		}
		v.RegisterSchema(skill.Name, schema)
		loaded++
	}

	if loaded > 0 {
		log.Printf("[SkillSchemaValidator] Loaded %d schemas from DB (Task Skills with output_schema)", loaded)
	}
	return loaded
}

// Validate validates the output against the registered schema for the given skill
func (v *SkillSchemaValidator) Validate(skillName string, output json.RawMessage) SchemaValidationResult {
	v.mu.RLock()
	schema, exists := v.schemas[skillName]
	v.mu.RUnlock()

	// If no schema registered, pass by default (lenient mode)
	if !exists {
		return SchemaValidationResult{
			Valid:         true,
			MissingFields: []string{},
			InvalidEnums:  []string{},
			Score:         100,
			Verdict:       string(models.AssessmentVerdictPass),
		}
	}

	// Parse JSON output
	var data map[string]interface{}
	if err := json.Unmarshal(output, &data); err != nil {
		// Invalid JSON
		return SchemaValidationResult{
			Valid:         false,
			MissingFields: []string{},
			InvalidEnums:  []string{},
			Score:         0,
			Verdict:       string(models.AssessmentVerdictFail),
		}
	}

	// Check required fields
	missingFields := []string{}
	for _, field := range schema.RequiredFields {
		value, exists := data[field]
		if !exists || value == nil {
			missingFields = append(missingFields, field)
		}
	}

	// Check required-one-of groups
	for _, group := range schema.RequiredOneOf {
		found := false
		for _, field := range group {
			if val, ok := data[field]; ok && val != nil {
				found = true
				break
			}
		}
		if !found {
			missingFields = append(missingFields, fmt.Sprintf("one_of(%s)", strings.Join(group, "|")))
		}
	}

	// Check enum fields
	invalidEnums := []string{}
	for field, allowedValues := range schema.EnumFields {
		value, exists := data[field]
		if !exists {
			// Field doesn't exist, skip enum check (will be caught by required field check if applicable)
			continue
		}

		// Convert value to string for comparison
		strValue, ok := value.(string)
		if !ok {
			// Not a string, consider it invalid
			invalidEnums = append(invalidEnums, fmt.Sprintf("%s=%v", field, value))
			continue
		}

		// Check if value is in allowed list
		valid := false
		for _, allowed := range allowedValues {
			if strValue == allowed {
				valid = true
				break
			}
		}

		if !valid {
			invalidEnums = append(invalidEnums, fmt.Sprintf("%s=%s", field, strValue))
		}
	}

	// Determine verdict and score
	valid := len(missingFields) == 0 && len(invalidEnums) == 0
	score := 0
	verdict := string(models.AssessmentVerdictFail)
	if valid {
		score = 100
		verdict = string(models.AssessmentVerdictPass)
	}

	return SchemaValidationResult{
		Valid:         valid,
		MissingFields: missingFields,
		InvalidEnums:  invalidEnums,
		Score:         score,
		Verdict:       verdict,
	}
}
