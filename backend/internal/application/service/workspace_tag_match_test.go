package service

import "testing"

func TestWorkspaceTagsMatchFilter_EmptyFilter(t *testing.T) {
	if !WorkspaceTagsMatchFilter(nil, nil) {
		t.Fatal("empty filter should match")
	}
	if !WorkspaceTagsMatchFilter(map[string]interface{}{"env": "prod"}, nil) {
		t.Fatal("nil filter should match")
	}
	if !WorkspaceTagsMatchFilter(map[string]interface{}{"env": "prod"}, map[string]interface{}{}) {
		t.Fatal("empty map filter should match")
	}
}

func TestWorkspaceTagsMatchFilter_AND(t *testing.T) {
	ws := map[string]interface{}{"env": "prod", "team": "platform", "extra": "x"}
	filter := map[string]interface{}{"env": "prod", "team": "platform"}
	if !WorkspaceTagsMatchFilter(ws, filter) {
		t.Fatal("should match all keys")
	}
	if WorkspaceTagsMatchFilter(ws, map[string]interface{}{"env": "staging"}) {
		t.Fatal("env mismatch")
	}
	if WorkspaceTagsMatchFilter(ws, map[string]interface{}{"missing": "1"}) {
		t.Fatal("missing key")
	}
}

func TestWorkspaceTagsMatchFilter_MultiValue(t *testing.T) {
	ws := map[string]interface{}{"env": "staging"}
	filter := map[string]interface{}{"env": []interface{}{"prod", "staging"}}
	if !WorkspaceTagsMatchFilter(ws, filter) {
		t.Fatal("should match multi-value")
	}
	if WorkspaceTagsMatchFilter(map[string]interface{}{"env": "dev"}, filter) {
		t.Fatal("dev not in set")
	}
}

func TestNormalizeWorkspaceTagFilter(t *testing.T) {
	n := NormalizeWorkspaceTagFilter(map[string]interface{}{
		"env": " prod ",
		"":    "x",
		"skip": "",
		"tiers": []interface{}{"a", "", "b"},
	})
	if n["env"] != "prod" {
		t.Fatalf("env: %v", n["env"])
	}
	if _, ok := n[""]; ok {
		t.Fatal("empty key")
	}
	if _, ok := n["skip"]; ok {
		t.Fatal("empty value")
	}
	arr, ok := n["tiers"].([]interface{})
	if !ok || len(arr) != 2 {
		t.Fatalf("tiers: %v", n["tiers"])
	}
}
