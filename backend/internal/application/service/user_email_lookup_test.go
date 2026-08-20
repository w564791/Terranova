package service

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestDBUserEmailLookup(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Exec(`CREATE TABLE users (user_id TEXT PRIMARY KEY, email TEXT)`)
	_ = db.Exec(`INSERT INTO users (user_id, email) VALUES ('u1', 'alice@corp.test')`)

	l := NewDBUserEmailLookup(db)
	email, err := l.GetUserEmail(context.Background(), "u1")
	if err != nil || email != "alice@corp.test" {
		t.Fatalf("got %q err=%v", email, err)
	}
	if _, err := l.GetUserEmail(context.Background(), "missing"); err == nil {
		t.Fatal("missing user")
	}
}

func TestResolveUserEmail_EmailShapedUserID(t *testing.T) {
	c := &PermissionCheckerImpl{}
	e, err := c.resolveUserEmail(context.Background(), "bob@example.com")
	if err != nil || e != "bob@example.com" {
		t.Fatalf("%q %v", e, err)
	}
}
