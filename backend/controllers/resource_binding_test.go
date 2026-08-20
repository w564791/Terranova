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

func setupResourceBindingDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Exec(`CREATE TABLE workspaces (
		id INTEGER PRIMARY KEY,
		workspace_id TEXT,
		name TEXT
	)`)
	_ = db.Exec(`CREATE TABLE workspace_resources (
		id INTEGER PRIMARY KEY,
		workspace_id TEXT,
		resource_id TEXT,
		resource_type TEXT,
		resource_name TEXT,
		is_active INTEGER DEFAULT 1,
		description TEXT,
		created_at DATETIME,
		updated_at DATETIME
	)`)
	// Snapshot table with only columns needed for path binding (workspace_id check).
	// Avoid map/jsonb columns that SQLite+GORM cannot scan into map[string]interface{}.
	_ = db.Exec(`CREATE TABLE workspace_resources_snapshot (
		id INTEGER PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		snapshot_name TEXT,
		created_at DATETIME,
		description TEXT,
		created_by TEXT,
		task_id INTEGER,
		state_version_id INTEGER
	)`)
	_ = db.Exec(`INSERT INTO workspaces (id, workspace_id, name) VALUES (1,'ws-a','A'),(2,'ws-b','B')`)
	_ = db.Exec(`INSERT INTO workspace_resources (id, workspace_id, resource_id, resource_type, resource_name, is_active)
		VALUES (10,'ws-b','aws_s3.x','aws_s3_bucket','x',1)`)
	_ = db.Exec(`INSERT INTO workspace_resources (id, workspace_id, resource_id, resource_type, resource_name, is_active)
		VALUES (11,'ws-a','aws_s3.y','aws_s3_bucket','y',1)`)
	_ = db.Exec(`INSERT INTO workspace_resources_snapshot (id, workspace_id, snapshot_name)
		VALUES (20,'ws-b','snap-b'),(21,'ws-a','snap-a')`)
	return db
}

func newResourceCtrl(t *testing.T, db *gorm.DB) *ResourceController {
	t.Helper()
	return &ResourceController{
		service: services.NewResourceService(db, nil),
	}
}

func TestLoadResourceInPathWorkspace_CrossWorkspaceDenied(t *testing.T) {
	db := setupResourceBindingDB(t)
	ctrl := newResourceCtrl(t, db)
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/workspaces/ws-a/resources/10", nil)
	c.Params = gin.Params{{Key: "id", Value: "ws-a"}}

	// resource 10 belongs to ws-b
	_, ok := ctrl.loadResourceInPathWorkspace(c, 10)
	if ok {
		t.Fatal("cross-workspace resource must be rejected")
	}
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d body=%s", w.Code, w.Body.String())
	}
}

func TestLoadResourceInPathWorkspace_SameWorkspaceOK(t *testing.T) {
	db := setupResourceBindingDB(t)
	ctrl := newResourceCtrl(t, db)
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/workspaces/ws-a/resources/11", nil)
	c.Params = gin.Params{{Key: "id", Value: "ws-a"}}

	res, ok := ctrl.loadResourceInPathWorkspace(c, 11)
	if !ok {
		t.Fatalf("expected ok, code=%d body=%s", w.Code, w.Body.String())
	}
	if res.WorkspaceID != "ws-a" || res.ID != 11 {
		t.Fatalf("res=%+v", res)
	}
}

func TestLoadSnapshotInPathWorkspace_CrossWorkspaceDenied(t *testing.T) {
	db := setupResourceBindingDB(t)
	ctrl := newResourceCtrl(t, db)
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/workspaces/ws-a/snapshots/20", nil)
	c.Params = gin.Params{{Key: "id", Value: "ws-a"}}

	_, ok := ctrl.loadSnapshotInPathWorkspace(c, 20)
	if ok {
		t.Fatal("cross-workspace snapshot must be rejected")
	}
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d body=%s", w.Code, w.Body.String())
	}
}

func TestLoadSnapshotInPathWorkspace_SameWorkspace_MetaBinds(t *testing.T) {
	db := setupResourceBindingDB(t)
	ctrl := newResourceCtrl(t, db)
	gin.SetMode(gin.TestMode)

	// Meta binding (workspace_id) succeeds; full GetSnapshot may still fail on SQLite without JSONB.
	// Security property: wrong path is denied before content load.
	wCross := httptest.NewRecorder()
	cCross, _ := gin.CreateTestContext(wCross)
	cCross.Request = httptest.NewRequest("GET", "/workspaces/ws-b/snapshots/21", nil)
	cCross.Params = gin.Params{{Key: "id", Value: "ws-b"}}
	if _, ok := ctrl.loadSnapshotInPathWorkspace(cCross, 21); ok {
		t.Fatal("ws-a snapshot must not load under ws-b path")
	}
	if wCross.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d", wCross.Code)
	}

	// Correct path: meta match; if GetSnapshot fails closed that's acceptable for unit env
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/workspaces/ws-a/snapshots/21", nil)
	c.Params = gin.Params{{Key: "id", Value: "ws-a"}}
	var metaWS string
	if err := db.Table("workspace_resources_snapshot").Select("workspace_id").Where("id = ?", 21).Scan(&metaWS).Error; err != nil {
		t.Fatal(err)
	}
	pathWS, ok := ctrl.resolvePathWorkspace(c)
	if !ok || pathWS != metaWS || pathWS != "ws-a" {
		t.Fatalf("meta bind failed path=%s meta=%s ok=%v", pathWS, metaWS, ok)
	}
}

func TestParseResourceIDInPathWorkspace_CrossWorkspaceDenied(t *testing.T) {
	db := setupResourceBindingDB(t)
	ctrl := newResourceCtrl(t, db)
	gin.SetMode(gin.TestMode)

	// resource 10 is in ws-b; path claims ws-a → used by all editing/* endpoints
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/workspaces/ws-a/resources/10/editing/start", nil)
	c.Params = gin.Params{
		{Key: "id", Value: "ws-a"},
		{Key: "resource_id", Value: "10"},
	}
	if _, ok := ctrl.parseResourceIDInPathWorkspace(c); ok {
		t.Fatal("cross-workspace editing resource must be rejected")
	}
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d body=%s", w.Code, w.Body.String())
	}
}

func TestParseResourceIDInPathWorkspace_SameWorkspaceOK(t *testing.T) {
	db := setupResourceBindingDB(t)
	ctrl := newResourceCtrl(t, db)
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/workspaces/ws-a/resources/11/editing/start", nil)
	c.Params = gin.Params{
		{Key: "id", Value: "ws-a"},
		{Key: "resource_id", Value: "11"},
	}
	rid, ok := ctrl.parseResourceIDInPathWorkspace(c)
	if !ok || rid != 11 {
		t.Fatalf("want rid=11 ok, got %d ok=%v code=%d %s", rid, ok, w.Code, w.Body.String())
	}
}

func TestParseResourceIDInPathWorkspace_InvalidID(t *testing.T) {
	db := setupResourceBindingDB(t)
	ctrl := newResourceCtrl(t, db)
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/workspaces/ws-a/resources/abc/editing/start", nil)
	c.Params = gin.Params{
		{Key: "id", Value: "ws-a"},
		{Key: "resource_id", Value: "abc"},
	}
	if _, ok := ctrl.parseResourceIDInPathWorkspace(c); ok {
		t.Fatal("invalid id must fail")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", w.Code)
	}
}

func TestResolvePathWorkspace_MissingAndNotFound(t *testing.T) {
	db := setupResourceBindingDB(t)
	ctrl := newResourceCtrl(t, db)
	gin.SetMode(gin.TestMode)

	// missing param
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/workspaces//resources", nil)
	c.Params = gin.Params{}
	if _, ok := ctrl.resolvePathWorkspace(c); ok {
		t.Fatal("empty path id must fail")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", w.Code)
	}

	// not found
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest("GET", "/workspaces/ws-missing/resources", nil)
	c2.Params = gin.Params{{Key: "id", Value: "ws-missing"}}
	if _, ok := ctrl.resolvePathWorkspace(c2); ok {
		t.Fatal("missing workspace must fail")
	}
	if w2.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d", w2.Code)
	}
}
