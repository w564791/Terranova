package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"iac-platform/internal/application/service"
	"iac-platform/internal/domain/valueobject"
	"iac-platform/internal/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type appCheckMock struct {
	allow map[string]bool // principal_id -> allowed
	last  *service.CheckPermissionRequest
}

func (m *appCheckMock) CheckPermission(ctx context.Context, req *service.CheckPermissionRequest) (*service.CheckPermissionResult, error) {
	m.last = req
	ok := m.allow != nil && m.allow[req.PrincipalID]
	lv := valueobject.PermissionLevelNone
	if ok {
		lv = valueobject.PermissionLevelRead
	}
	return &service.CheckPermissionResult{IsAllowed: ok, EffectiveLevel: lv, DenyReason: "No permission"}, nil
}
func (m *appCheckMock) CheckPermissionWithTemporary(ctx context.Context, req *service.CheckPermissionRequest, taskID *uint) (*service.CheckPermissionResult, error) {
	return m.CheckPermission(ctx, req)
}
func (m *appCheckMock) CheckBatchPermissions(ctx context.Context, reqs []*service.CheckPermissionRequest) ([]*service.CheckPermissionResult, error) {
	return nil, nil
}
func (m *appCheckMock) GetUserTeams(ctx context.Context, userID string) ([]string, error) {
	return nil, nil
}

func TestApplicationPrincipal_WhoAmI_And_Check(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mock := &appCheckMock{allow: map[string]bool{"app_key_1": true}}
	h := NewApplicationPrincipalHandler(mock)

	r := gin.New()
	r.GET("/app/whoami", func(c *gin.Context) {
		c.Set("principal_type", "APPLICATION")
		c.Set("principal_id", "app_key_1")
		c.Set("user_id", "app:app_key_1")
		c.Set("auth_org_id", uint(1))
		c.Set("application_id", uint(1))
		c.Set("username", "app:demo")
		c.Next()
	}, h.WhoAmI)
	r.POST("/app/permissions/check", func(c *gin.Context) {
		c.Set("principal_type", "APPLICATION")
		c.Set("principal_id", "app_key_1")
		c.Set("user_id", "app:app_key_1")
		c.Next()
	}, h.CheckPermission)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/app/whoami", nil))
	if w.Code != 200 {
		t.Fatalf("whoami: %d %s", w.Code, w.Body.String())
	}

	body := `{"resource_type":"WORKSPACES","scope_type":"ORGANIZATION","scope_id":"1","required_level":"READ"}`
	w2 := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/app/permissions/check", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req)
	if w2.Code != 200 {
		t.Fatalf("check: %d %s", w2.Code, w2.Body.String())
	}
	if mock.last == nil || mock.last.PrincipalType != valueobject.PrincipalTypeApplication {
		t.Fatalf("principal type: %+v", mock.last)
	}
	if mock.last.PrincipalID != "app_key_1" {
		t.Fatalf("principal_id want app_key_1 got %s", mock.last.PrincipalID)
	}
}

func TestApplicationAuthOnly_MissingCreds(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:app-auth-miss?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", middleware.ApplicationAuthOnly(db), func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/x", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", w.Code)
	}
}

func TestJWTOrApplicationAuth_PrefersAppHeaders(t *testing.T) {
	// 复用 agent_auth 测试库形态：无真实 Validate 时至少走 app 分支要求双 header
	db, err := gorm.Open(sqlite.Open("file:jwt-or-app?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Exec(`CREATE TABLE applications (
		id INTEGER PRIMARY KEY, org_id INTEGER, name TEXT, app_key TEXT, app_secret TEXT,
		is_active INTEGER, expires_at DATETIME, last_used_at DATETIME, workspace_tag_filter TEXT
	)`)
	// secret 明文兼容路径（verifyAppSecret 支持 legacy）
	_ = db.Exec(`INSERT INTO applications (id, org_id, name, app_key, app_secret, is_active)
		VALUES (1, 3, 'a', 'k1', 's1', 1)`)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/p", middleware.JWTOrApplicationAuth(db), func(c *gin.Context) {
		if c.GetString("principal_type") != "APPLICATION" {
			t.Errorf("want APPLICATION got %s", c.GetString("principal_type"))
		}
		if c.GetString("principal_id") != "k1" {
			t.Errorf("want k1 got %s", c.GetString("principal_id"))
		}
		c.Status(200)
	})
	req := httptest.NewRequest("GET", "/p", nil)
	req.Header.Set("X-App-Key", "k1")
	req.Header.Set("X-App-Secret", "s1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("want 200 got %d %s", w.Code, w.Body.String())
	}
}
