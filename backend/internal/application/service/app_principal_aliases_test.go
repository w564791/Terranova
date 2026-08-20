package service

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestExpandApplicationPrincipalIDs(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:app-alias?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Exec(`CREATE TABLE applications (id INTEGER PRIMARY KEY, app_key TEXT)`)
	_ = db.Exec(`INSERT INTO applications (id, app_key) VALUES (5, 'key_xyz')`)

	a := NewDBApplicationPrincipalAliases(db)
	ids, err := a.ExpandApplicationPrincipalIDs(context.Background(), "key_xyz")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) < 2 {
		t.Fatalf("want key+id, got %v", ids)
	}
	hasKey, hasID := false, false
	for _, id := range ids {
		if id == "key_xyz" {
			hasKey = true
		}
		if id == "5" {
			hasID = true
		}
	}
	if !hasKey || !hasID {
		t.Fatalf("expanded=%v", ids)
	}

	ids2, err := a.ExpandApplicationPrincipalIDs(context.Background(), "5")
	if err != nil {
		t.Fatal(err)
	}
	hasKey, hasID = false, false
	for _, id := range ids2 {
		if id == "key_xyz" {
			hasKey = true
		}
		if id == "5" {
			hasID = true
		}
	}
	if !hasKey || !hasID {
		t.Fatalf("from id expanded=%v", ids2)
	}

	// app: prefix
	ids3, _ := a.ExpandApplicationPrincipalIDs(context.Background(), "app:key_xyz")
	if ids3[0] != "key_xyz" && ids3[0] != "app:key_xyz" {
		// first should be stripped
	}
	found := false
	for _, id := range ids3 {
		if id == "key_xyz" {
			found = true
		}
	}
	if !found {
		t.Fatalf("prefix expand: %v", ids3)
	}
}
