package services

import (
	"testing"

	"iac-platform/internal/models"
)

func TestToWorkspaceListItem_RedactsSensitiveConfig(t *testing.T) {
	w := models.Workspace{
		ID:              1,
		WorkspaceID:     "ws-1",
		Name:            "n",
		StateConfig:     models.JSONB{"bucket": "secret-bucket", "key": "k"},
		ProviderConfig:  models.JSONB{"token": "xxx"},
		SystemVariables: models.JSONB{"AWS_SECRET": "s"},
		InitConfig:      models.JSONB{"backend": true},
		LockInfo:        models.JSONB{"who": "me"},
		NotifySettings:  models.JSONB{"webhook": "http://x"},
		LogConfig:       models.JSONB{"level": "debug"},
		Tags:            models.JSONB{"env": "dev"},
	}
	item := toWorkspaceListItem(w)
	if item.Name != "n" || item.WorkspaceID != "ws-1" {
		t.Fatalf("basic fields: %+v", item)
	}
	if item.StateConfig != nil || item.ProviderConfig != nil || item.SystemVariables != nil {
		t.Fatal("sensitive configs must be nil in list item")
	}
	if item.InitConfig != nil || item.LockInfo != nil || item.NotifySettings != nil || item.LogConfig != nil {
		t.Fatal("config blobs must be nil in list item")
	}
	// tags kept (non-secret metadata)
	if item.Tags == nil {
		t.Fatal("tags should remain")
	}
}
