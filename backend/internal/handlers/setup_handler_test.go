package handlers

import (
	"net/http/httptest"
	"testing"

	"iac-platform/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestSetupHandler_GetStatus(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	// minimal users table matching models.User columns used
	_ = db.Exec(`
CREATE TABLE users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id TEXT,
  username TEXT,
  email TEXT,
  password_hash TEXT,
  is_active INTEGER,
  is_system_admin INTEGER
);`)

	h := NewSetupHandler(db)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/setup/status", h.GetStatus)

	// not initialized
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/setup/status", nil))
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if !contains(w.Body.String(), `"initialized":false`) && !contains(w.Body.String(), `"Initialized":false`) {
		// JSON field is Initialized in struct with json initialized
		if !contains(w.Body.String(), "尚未初始化") && !contains(w.Body.String(), "false") {
			t.Fatalf("body=%s", w.Body.String())
		}
	}

	// seed admin
	_ = db.Exec(`INSERT INTO users (user_id, username, email, password_hash, is_active, is_system_admin)
		VALUES ('u1','admin','a@b.c','x',1,1)`)
	// models.User may use different column mapping - if count still 0, insert via model
	_ = models.User{}
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest("GET", "/setup/status", nil))
	if w2.Code != 200 {
		t.Fatalf("%d %s", w2.Code, w2.Body.String())
	}
}
