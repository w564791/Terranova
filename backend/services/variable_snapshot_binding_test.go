package services

import (
	"testing"

	"iac-platform/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestDeleteSnapshot_BindsWorkspace(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.AutoMigrate(&models.VariableSnapshot{})
	_ = db.Create(&models.VariableSnapshot{
		VsnapID: "vsnap-1", WorkspaceID: "ws-a", VariableID: "v1", Version: 1, SourceType: "workspace",
	})

	svc := NewVariableSnapshotService(db)
	// wrong workspace
	if err := svc.DeleteSnapshot("ws-b", "vsnap-1"); err == nil {
		t.Fatal("cross-ws delete must fail")
	}
	ok, err := svc.SnapshotBelongsToWorkspace("ws-a", "vsnap-1")
	if err != nil || !ok {
		t.Fatalf("belongs: %v %v", ok, err)
	}
	if err := svc.DeleteSnapshot("ws-a", "vsnap-1"); err != nil {
		t.Fatal(err)
	}
}
