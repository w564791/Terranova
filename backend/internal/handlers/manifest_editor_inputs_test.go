package handlers

import (
	"testing"

	"iac-platform/internal/models"
)

func TestExtractModuleInputs_Types(t *testing.T) {
	schema := models.JSONB{
		"components": map[string]interface{}{
			"schemas": map[string]interface{}{
				"ModuleInput": map[string]interface{}{
					"type":     "object",
					"required": []interface{}{"vpc_id", "name"},
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Name",
						},
						"vpc_id": map[string]interface{}{
							"type": "string",
						},
						"create": map[string]interface{}{
							"type":    "boolean",
							"default": true,
						},
						"count": map[string]interface{}{
							"type": "integer",
						},
						"tags": map[string]interface{}{
							"type": "object",
							"additionalProperties": map[string]interface{}{
								"type": "string",
							},
							"default": map[string]interface{}{},
						},
						"ingress_rules": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"type": "string",
							},
						},
						"complex": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"type": "object",
							},
						},
						"env": map[string]interface{}{
							"type": "string",
							"enum": []interface{}{"dev", "prod"},
						},
					},
				},
			},
		},
	}

	fields := extractModuleInputs(schema)
	if len(fields) != 8 {
		t.Fatalf("want 8 fields, got %d", len(fields))
	}

	// required first
	if !fields[0].Required || !fields[1].Required {
		t.Fatalf("required should sort first: %+v %+v", fields[0], fields[1])
	}

	byName := map[string]ModuleInputField{}
	for _, f := range fields {
		byName[f.Name] = f
	}

	assertLabel := func(name, wantType, wantLabel string) {
		f, ok := byName[name]
		if !ok {
			t.Fatalf("missing field %s", name)
		}
		if f.Type != wantType || f.TypeLabel != wantLabel {
			t.Fatalf("%s: type=%q label=%q want type=%q label=%q", name, f.Type, f.TypeLabel, wantType, wantLabel)
		}
	}

	assertLabel("name", "string", "string")
	assertLabel("create", "boolean", "bool")
	assertLabel("count", "number", "number")
	assertLabel("tags", "object", "map(string)")
	assertLabel("ingress_rules", "array", "list(string)")
	assertLabel("complex", "array", "list(object)")
	assertLabel("env", "string", "string")

	if len(byName["env"].Enum) != 2 {
		t.Fatalf("env enum: %+v", byName["env"].Enum)
	}
	if byName["create"].Default != "true" {
		t.Fatalf("create default: %q", byName["create"].Default)
	}
}

func TestExtractModuleInputs_Empty(t *testing.T) {
	if len(extractModuleInputs(nil)) != 0 {
		t.Fatal("nil schema")
	}
	if len(extractModuleInputs(models.JSONB{})) != 0 {
		t.Fatal("empty schema")
	}
}
