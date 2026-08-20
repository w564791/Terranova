package controllers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupModuleTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	// minimal schema if GetModules queries modules table
	_ = db.Exec(`CREATE TABLE IF NOT EXISTS modules (
		id INTEGER PRIMARY KEY,
		name TEXT,
		description TEXT
	)`)
	return db
}

func TestModuleController_GetModules_EmptyDB(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupModuleTestDB(t)
	ctrl := NewModuleController(services.NewModuleService(db))
	r := gin.New()
	r.GET("/modules", ctrl.GetModules)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/modules", nil))
	// 真实 DB 路径：空表应 200 或业务 4xx，绝不能 panic / 无响应 / 5xx 静默
	if w.Code == 0 {
		t.Fatal("no response")
	}
	if w.Code >= 500 {
		t.Fatalf("get modules unexpected 5xx status=%d body=%s", w.Code, w.Body.String())
	}
	if w.Code != http.StatusOK && w.Code < 400 {
		t.Fatalf("get modules unexpected status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestModuleController_GetModule_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupModuleTestDB(t)
	ctrl := NewModuleController(services.NewModuleService(db))
	r := gin.New()
	r.GET("/modules/:id", ctrl.GetModule)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/modules/999", nil))
	if w.Code == http.StatusOK {
		t.Fatalf("GetModule must not 200 for missing id: %s", w.Body.String())
	}
	if w.Code == 0 || w.Code >= 500 {
		t.Fatalf("GetModule unexpected status=%d body=%s", w.Code, w.Body.String())
	}
}
