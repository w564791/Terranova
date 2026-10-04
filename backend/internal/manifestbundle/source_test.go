package manifestbundle

import (
	"context"
	"errors"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func sourceDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		`CREATE TABLE manifest_files (id INTEGER PRIMARY KEY AUTOINCREMENT, manifest_id TEXT, version_id TEXT, owner_user_id TEXT, path TEXT, content BLOB, mime TEXT, size INTEGER, is_binary INTEGER, mode INTEGER, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE manifest_versions (id TEXT PRIMARY KEY, manifest_id TEXT, version TEXT, bundle_hash TEXT, bundle_invalid_reason TEXT, created_at DATETIME)`,
		`INSERT INTO manifest_versions (id, manifest_id, version) VALUES ('mfv-1', 'mf-1', 'v1'), ('mfv-bad', 'mf-1', 'v0')`,
		`UPDATE manifest_versions SET bundle_invalid_reason = 'denylisted_file @ a.tfvars' WHERE id = 'mfv-bad'`,
		`INSERT INTO manifest_files (manifest_id, version_id, path, content) VALUES ('mf-1', 'mfv-bad', 'a.tfvars', X'00')`,
		`INSERT INTO manifest_files (manifest_id, version_id, owner_user_id, path, content) VALUES ('mf-1', NULL, 'u1', 'main.tf', CAST('draft' AS BLOB)), ('mf-1', NULL, 'u2', 'other.tf', CAST('x' AS BLOB))`,
	} {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("%v: %s", err, s)
		}
	}
	return db
}

func TestPackStoreOpen_RoundTrip(t *testing.T) {
	ctx := context.Background()
	db := sourceDB(t)
	b, problems, err := Pack(ctx, NativeDraft{DB: db, ManifestID: "mf-1", OwnerUserID: "u1"})
	if err != nil || problems != nil || len(b.Files) != 1 {
		t.Fatalf("Pack: %+v %v %v", b, problems, err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error { return Store(ctx, tx, "mf-1", "mfv-1", b) }); err != nil {
		t.Fatal(err)
	}
	got, err := OpenVersion(ctx, db, "mf-1", "mfv-1")
	if err != nil || got.Hash != b.Hash || string(got.Scope()["main.tf"]) != "draft" || got.RequireValid() != nil {
		t.Fatalf("OpenVersion: %+v %v", got, err)
	}
	if byHash, err := OpenBundle(ctx, db, b.Hash); err != nil || byHash.VersionID != "mfv-1" {
		t.Fatalf("OpenBundle: %+v %v", byHash, err)
	}
	if _, err := OpenBundle(ctx, db, "nope"); !errors.Is(err, ErrVersionNotFound) {
		t.Fatalf("OpenBundle unknown: %v", err)
	}
	// wrong manifest / unknown version
	if _, err := OpenVersion(ctx, db, "mf-other", "mfv-1"); !errors.Is(err, ErrVersionNotFound) {
		t.Fatalf("cross-manifest open: %v", err)
	}
	// draft edits after Store do not change the snapshot
	db.Exec(`UPDATE manifest_files SET content = CAST('edited' AS BLOB) WHERE version_id IS NULL`)
	if again, err := OpenVersion(ctx, db, "", "mfv-1"); err != nil || string(again.Scope()["main.tf"]) != "draft" {
		t.Fatalf("snapshot changed: %+v %v", again, err)
	}
	// tampering is detected
	db.Exec(`UPDATE manifest_files SET content = CAST('evil' AS BLOB) WHERE version_id = 'mfv-1'`)
	if _, err := OpenVersion(ctx, db, "", "mfv-1"); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("tamper: %v", err)
	}
}

func TestOpenVersion_NullHashIsLenientButNotValid(t *testing.T) {
	b, err := OpenVersion(context.Background(), sourceDB(t), "mf-1", "mfv-bad")
	if err != nil || b.Hash != "" || len(b.Files) != 1 {
		t.Fatalf("OpenVersion: %+v %v", b, err)
	}
	var inv *InvalidError
	if err := b.RequireValid(); !errors.As(err, &inv) || inv.Reason != "denylisted_file @ a.tfvars" {
		t.Fatalf("RequireValid: %v", err)
	}
}

func TestPack_ReportsProblemsAndStoresNothing(t *testing.T) {
	b, problems, err := PackFiles([]File{f("main.tf", "x"), f("prod.tfvars", "x")})
	if err != nil || b != nil || len(problems) != 1 || problems[0].Rule != RuleDenylistedFile {
		t.Fatalf("PackFiles: %+v %v %v", b, problems, err)
	}
	hash, problems, err := CheckVersion(context.Background(), sourceDB(t), "mfv-bad")
	if err != nil || hash != "" || len(problems) != 1 {
		t.Fatalf("CheckVersion: %q %v %v", hash, problems, err)
	}
}

func TestGitCommitSourceNotImplemented(t *testing.T) {
	if _, _, err := Pack(context.Background(), GitCommit{}); !errors.Is(err, ErrGitSourceNotImplemented) {
		t.Fatalf("want ErrGitSourceNotImplemented, got %v", err)
	}
}

func TestStore_ModeIsNormalizedHashedAndVerified(t *testing.T) {
	ctx := context.Background()
	db := sourceDB(t)
	db.Exec(`INSERT INTO manifest_files (manifest_id, version_id, owner_user_id, path, content, mode) VALUES ('mf-1', NULL, 'u1', 'run.sh', CAST('echo' AS BLOB), 509)`)
	b, problems, err := Pack(ctx, NativeDraft{DB: db, ManifestID: "mf-1", OwnerUserID: "u1"})
	if err != nil || problems != nil {
		t.Fatalf("Pack: %v %v", problems, err)
	}
	flat := make([]File, len(b.Files))
	for i, f := range b.Files {
		flat[i] = File{Path: f.Path, Content: f.Content, Mode: ModeRegular}
	}
	if h, _ := Hash(flat); h == b.Hash {
		t.Fatal("executable bit must be part of the bundle hash")
	}
	if err := db.Transaction(func(tx *gorm.DB) error { return Store(ctx, tx, "mf-1", "mfv-1", b) }); err != nil {
		t.Fatal(err)
	}
	var mode int
	db.Raw(`SELECT mode FROM manifest_files WHERE version_id = 'mfv-1' AND path = 'run.sh'`).Scan(&mode)
	if mode != ModeExecutable {
		t.Fatalf("stored mode %o, want 755", mode)
	}
	// flipping only the stored mode is detected like a content change
	db.Exec(`UPDATE manifest_files SET mode = 420 WHERE version_id = 'mfv-1' AND path = 'run.sh'`)
	if _, err := OpenVersion(ctx, db, "", "mfv-1"); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("mode tamper: %v", err)
	}
}
