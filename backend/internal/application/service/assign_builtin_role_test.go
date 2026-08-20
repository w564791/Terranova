package service

import (
	"context"
	"testing"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/valueobject"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAssignBuiltinRoleToUser(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:assign-builtin?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&entity.Role{}, &entity.UserRole{}); err != nil {
		t.Fatal(err)
	}
	role := &entity.Role{Name: "workspace_admin", DisplayName: "WS Admin", IsActive: true, IsSystem: true}
	if err := db.Create(role).Error; err != nil {
		t.Fatal(err)
	}

	s := &PermissionServiceImpl{db: db}
	err = s.AssignBuiltinRoleToUser(context.Background(), "u1", "workspace_admin",
		valueobject.ScopeTypeWorkspace, 42, "creator", "auto")
	if err != nil {
		t.Fatal(err)
	}
	// idempotent
	if err := s.AssignBuiltinRoleToUser(context.Background(), "u1", "workspace_admin",
		valueobject.ScopeTypeWorkspace, 42, "creator", "auto"); err != nil {
		t.Fatal(err)
	}
	var n int64
	db.Model(&entity.UserRole{}).Where("user_id = ? AND role_id = ?", "u1", role.ID).Count(&n)
	if n != 1 {
		t.Fatalf("want 1 assignment got %d", n)
	}
	if err := s.AssignBuiltinRoleToUser(context.Background(), "u1", "missing_role",
		valueobject.ScopeTypeWorkspace, 1, "x", ""); err == nil {
		t.Fatal("missing role should error")
	}
}
