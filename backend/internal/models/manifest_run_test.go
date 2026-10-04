package models

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The model carries the spec-critical CHECKs (mirroring the migration DDL):
// sandbox runs are preview-only and always belong to a session.
func TestManifestRunCheckConstraints(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&ManifestRun{}, &SandboxSession{}, &RunToken{}); err != nil {
		t.Fatal(err)
	}
	session := "ss-1"
	hash := "0000000000000000000000000000000000000000000000000000000000000000"
	run := func(id, runner, purpose string, sess *string) error {
		return db.Create(&ManifestRun{ID: id, ManifestID: "mf-1", BundleHash: hash, WorkspaceID: "ws-a",
			Runner: runner, Purpose: purpose, SessionID: sess, CreatedBy: "u1"}).Error
	}
	for _, tc := range []struct {
		id, runner, purpose string
		sess                *string
		ok                  bool
	}{
		{"r1", ManifestRunRunnerAgent, ManifestRunPurposeApproval, nil, true},
		{"r2", ManifestRunRunnerAgent, ManifestRunPurposePreview, nil, true},
		{"r3", ManifestRunRunnerSandbox, ManifestRunPurposePreview, &session, true},
		{"r4", ManifestRunRunnerSandbox, ManifestRunPurposeApproval, &session, false}, // sandbox => preview
		{"r5", ManifestRunRunnerSandbox, ManifestRunPurposePreview, nil, false},       // sandbox => session
		{"r6", "local", ManifestRunPurposePreview, nil, false},
	} {
		if err := run(tc.id, tc.runner, tc.purpose, tc.sess); (err == nil) != tc.ok {
			t.Fatalf("%s %s/%s: err=%v, want ok=%v", tc.id, tc.runner, tc.purpose, err, tc.ok)
		}
	}
	var r ManifestRun
	db.First(&r, "id = ?", "r1")
	if r.Status != ManifestRunStatusPending {
		t.Fatalf("default status = %q, want pending", r.Status)
	}

	if err := db.Create(&SandboxSession{ID: "ss-2", UserID: "u1", WorkspaceID: "ws-a", Provider: SandboxProviderAgentCore,
		NetworkMode: "public", ExpiresAt: time.Now()}).Error; err == nil {
		t.Fatal("non-VPC sandbox session must be rejected")
	}
	if err := db.Create(&SandboxSession{ID: "ss-3", UserID: "u1", WorkspaceID: "ws-a", Provider: SandboxProviderAgentCore,
		ExpiresAt: time.Now()}).Error; err != nil {
		t.Fatalf("default network mode must be vpc: %v", err)
	}
	if err := db.Create(&RunToken{RunID: "r1", WorkspaceID: "ws-a", Purpose: "admin", TokenHash: hash, ExpiresAt: time.Now()}).Error; err == nil {
		t.Fatal("run token purpose must be preview|approval")
	}
}
