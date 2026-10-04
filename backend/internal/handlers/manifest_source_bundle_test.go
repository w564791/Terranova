package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	"iac-platform/internal/domain/valueobject"
	"iac-platform/internal/manifestbundle"
	"iac-platform/internal/models"

	"github.com/gin-gonic/gin"
	sqlite3 "github.com/mattn/go-sqlite3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var registerSQLiteNow sync.Once

// openSQLiteWithNow opens an in-memory sqlite DB that also understands the
// PostgreSQL NOW() used by PublishVersion's INSERT ... SELECT.
func openSQLiteWithNow(t *testing.T) *gorm.DB {
	t.Helper()
	registerSQLiteNow.Do(func() {
		sql.Register("sqlite3_with_now", &sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				return conn.RegisterFunc("NOW", func() string { return time.Now().UTC().Format(time.RFC3339) }, false)
			},
		})
	})
	db, err := gorm.Open(sqlite.Dialector{DriverName: "sqlite3_with_now", DSN: "file:" + t.Name() + "?mode=memory&cache=private"},
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestManifestSourceTypeDefaultsNativeAndIsImmutable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupManifestIAMDB(t)
	h := NewManifestHandler(db, nil)
	r := gin.New()
	r.POST("/organizations/:org_id/manifests", withCaller(valueobject.PermissionLevelWrite), h.CreateManifest)
	r.PUT("/organizations/:org_id/manifests/:id", withCaller(valueobject.PermissionLevelAdmin), h.UpdateManifest)
	r.GET("/organizations/:org_id/manifests/:id", withCaller(valueobject.PermissionLevelRead), h.GetManifest)

	// create: always native, exposed in the response
	w := doJSON(r, "POST", "/organizations/1/manifests", `{"name":"new-one","source_type":"git"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var created map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	if created["source_type"] != models.ManifestSourceNative {
		t.Fatalf("created source_type = %v, want native", created["source_type"])
	}
	for _, hidden := range []string{"git_repo_url", "git_subpath", "github_installation_id"} {
		if _, ok := created[hidden]; ok {
			t.Fatalf("%s must not be exposed yet", hidden)
		}
	}

	// existing row (column default) is native and visible on get
	w = doJSON(r, "GET", "/organizations/1/manifests/mf-1", "")
	var got map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got["source_type"] != models.ManifestSourceNative {
		t.Fatalf("get source_type = %v, want native", got["source_type"])
	}

	// changing it is rejected and nothing is persisted
	if w := doJSON(r, "PUT", "/organizations/1/manifests/mf-1", `{"source_type":"git","description":"changed"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("change source_type: got %d want 400 (%s)", w.Code, w.Body.String())
	}
	var m models.Manifest
	db.First(&m, "id = ?", "mf-1")
	if m.SourceType != models.ManifestSourceNative || m.Description == "changed" {
		t.Fatalf("rejected update must not persist: %+v", m)
	}
	// echoing the current value is fine
	if w := doJSON(r, "PUT", "/organizations/1/manifests/mf-1", `{"source_type":"native","description":"ok"}`); w.Code != http.StatusOK {
		t.Fatalf("same source_type: got %d (%s)", w.Code, w.Body.String())
	}
}

func TestPublishVersionRecordsBundleHash(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openSQLiteWithNow(t)
	for _, stmt := range []string{
		`CREATE TABLE manifests (id TEXT PRIMARY KEY, organization_id INTEGER, name TEXT, description TEXT, status TEXT, source_type TEXT NOT NULL DEFAULT 'native', git_repo_url TEXT, git_subpath TEXT, github_installation_id INTEGER, created_by TEXT, created_at DATETIME, updated_at DATETIME)`,
		`INSERT INTO manifests (id, organization_id, name, status, created_by) VALUES ('mf-1', 1, 'm1', 'draft', 'u1')`,
		`CREATE TABLE manifest_versions (id TEXT PRIMARY KEY, manifest_id TEXT, version TEXT, variables TEXT, changelog TEXT, bundle_hash TEXT, bundle_invalid_reason TEXT, source_ref TEXT, created_by TEXT, created_at DATETIME)`,
		`CREATE TABLE manifest_files (id INTEGER PRIMARY KEY AUTOINCREMENT, manifest_id TEXT, version_id TEXT, owner_user_id TEXT, path TEXT, content BLOB, mime TEXT, size INTEGER, is_binary INTEGER, mode INTEGER, created_at DATETIME, updated_at DATETIME)`,
		`INSERT INTO manifest_files (manifest_id, version_id, owner_user_id, path, content, mime, size, is_binary, mode) VALUES
		   ('mf-1', NULL, 'u1', 'main.tf', CAST('variable "x" {}' AS BLOB), 'text/plain', 15, 0, 420),
		   ('mf-1', NULL, 'u1', 'mod/a.tf', X'00FF', 'application/octet-stream', 2, 1, 420),
		   ('mf-1', NULL, 'u2', 'other-user.tf', X'01', 'text/plain', 1, 0, 420)`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("%v\n%s", err, stmt)
		}
	}
	h := NewManifestVersionsHandler(db)
	r := gin.New()
	r.POST("/organizations/:org_id/manifests/:id/v2/versions", withCaller(valueobject.PermissionLevelWrite), h.PublishVersion)
	w := doJSON(r, "POST", "/organizations/1/manifests/mf-1/v2/versions", `{"version":"v1.0.0"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("publish: %d %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	var v models.ManifestVersion
	if err := db.First(&v, "id = ?", resp["id"]).Error; err != nil {
		t.Fatal(err)
	}
	want, _ := manifestbundle.Hash([]manifestbundle.File{
		{Path: "main.tf", Content: []byte(`variable "x" {}`)},
		{Path: "mod/a.tf", Content: []byte{0x00, 0xff}},
	})
	if resp["bundle_hash"] != want {
		t.Fatalf("publish response bundle_hash = %v, want %s", resp["bundle_hash"], want)
	}
	if v.BundleHash == nil || *v.BundleHash != want {
		t.Fatalf("bundle_hash = %v, want %s (caller's draft only)", v.BundleHash, want)
	}
	again, _ := manifestbundle.VersionHash(context.Background(), db, v.ID)
	if again != want {
		t.Fatalf("VersionHash of snapshot = %s, want %s", again, want)
	}
}
