package handlers

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupAppPrincipalDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Exec(`CREATE TABLE applications (
		id INTEGER PRIMARY KEY,
		org_id INTEGER,
		name TEXT,
		app_key TEXT,
		app_secret TEXT,
		is_active INTEGER
	)`)
	_ = db.Exec(`INSERT INTO applications (id, org_id, name, app_key, app_secret, is_active)
		VALUES (7, 1, 'ci-app', 'app_key_ci_abc', 'secret', 1)`)
	return db
}

func TestResolveApplicationPrincipalID_FromNumericID(t *testing.T) {
	db := setupAppPrincipalDB(t)
	key, err := resolveApplicationPrincipalID(context.Background(), db, "7")
	if err != nil {
		t.Fatal(err)
	}
	if key != "app_key_ci_abc" {
		t.Fatalf("got %q", key)
	}
}

func TestResolveApplicationPrincipalID_FromAppKey(t *testing.T) {
	db := setupAppPrincipalDB(t)
	key, err := resolveApplicationPrincipalID(context.Background(), db, "app_key_ci_abc")
	if err != nil {
		t.Fatal(err)
	}
	if key != "app_key_ci_abc" {
		t.Fatalf("got %q", key)
	}
}

func TestResolveApplicationPrincipalID_Unknown(t *testing.T) {
	db := setupAppPrincipalDB(t)
	if _, err := resolveApplicationPrincipalID(context.Background(), db, "999"); err == nil {
		t.Fatal("want error for missing id")
	}
	if _, err := resolveApplicationPrincipalID(context.Background(), db, "no_such_key"); err == nil {
		t.Fatal("want error for missing key")
	}
}
